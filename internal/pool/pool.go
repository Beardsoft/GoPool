package pool

import (
	"context"
	"time"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/metrics"
	"github.com/Beardsoft/GoPool/internal/notifier"
	"github.com/Beardsoft/GoPool/internal/ops"

	"go.uber.org/zap"
)

type blockKind int

const (
	blockMicro blockKind = iota
	blockCheckpoint
	blockElection
)

// CursorName is the daemon's last-processed-height cursor. Exported so the
// API health endpoint can read the same row without duplicating the literal.
const CursorName = "last_processed_height"

const chainGaugeInterval = 30 * time.Second

// Manager is the pool daemon: it replays chain heights and runs payouts.
type Manager struct {
	chain       *chain.Chain
	queries     *db.Queries
	cfg         *config.Config
	policy      *rpc.Policy
	broadcaster *Broadcaster
	notifier    *notifier.Notifier
	recorder    *ops.Recorder
	// feeFloorAlerted tracks stakers currently in a fee-floor hold so the
	// operator gets one alert per hold episode, not one per ~2s tick.
	feeFloorAlerted       map[string]bool
	lastChainGaugeRefresh time.Time
	lastValidatorObserve  time.Time
}

// NewManager builds a Manager. Policy is loaded on Run.
func NewManager(c *chain.Chain, q *db.Queries, cfg *config.Config, opts ...func(*Manager)) *Manager {
	m := &Manager{chain: c, queries: q, cfg: cfg, broadcaster: GetBroadcaster(), notifier: notifier.New(cfg), feeFloorAlerted: make(map[string]bool)}
	for _, o := range opts {
		o(m)
	}
	return m
}

func WithRecorder(r *ops.Recorder) func(*Manager) {
	return func(m *Manager) { m.recorder = r }
}

// classify reports what kind of block a height is, relative to the cached
// policy constants. A height that is both an epoch and a batch boundary is
// an election block, which takes priority over checkpoint.
func (m *Manager) classify(height uint32) blockKind {
	offset := height - m.policy.GenesisBlockNumber
	if offset%m.policy.BlocksPerEpoch == 0 {
		return blockElection
	}
	if offset%m.policy.BlocksPerBatch == 0 {
		return blockCheckpoint
	}
	return blockMicro
}

// epochAt / batchAt match core-rs-albatross Policy::epoch_at / batch_at:
// ceil division after genesis, with genesis itself as epoch/batch 0.
// nimiq-go's Policy.EpochAt/BatchAt use truncating division, which numbers
// the first epoch as 0 and would look up the wrong staker snapshot.
func epochAt(p *rpc.Policy, height uint32) uint32 {
	if height <= p.GenesisBlockNumber {
		return 0
	}
	return divCeil(height-p.GenesisBlockNumber, p.BlocksPerEpoch)
}

func batchAt(p *rpc.Policy, height uint32) uint32 {
	if height <= p.GenesisBlockNumber {
		return 0
	}
	return divCeil(height-p.GenesisBlockNumber, p.BlocksPerBatch)
}

func divCeil(n, d uint32) uint32 {
	return (n + d - 1) / d
}

func shouldRefreshGauges(last, now time.Time) bool {
	return last.IsZero() || now.Sub(last) >= chainGaugeInterval
}

func (m *Manager) observeValidator(v *rpc.Validator, height uint32) {
	metrics.LiveStake.Set(float64(v.Balance))
	metrics.LiveStakers.Set(float64(v.NumStakers))
	metrics.SetValidatorState(validatorLiveState(v, height))
	m.lastValidatorObserve = time.Now()
}

func (m *Manager) observeTickGauges(ctx context.Context, head uint32, cursor int64, tickStart time.Time) {
	metrics.ChainHead.Set(float64(head))
	metrics.LastProcessedHeight.Set(float64(cursor))
	metrics.TickDuration.Set(time.Since(tickStart).Seconds())

	stats, err := m.queries.GetPayslipStats(ctx)
	if err == nil {
		metrics.PayslipsPending.Set(float64(stats.PendingCount))
		metrics.PayslipsPendingLuna.Set(float64(stats.PendingLuna))
		metrics.PayslipsStuck.Set(float64(stats.StuckCount))
	}
	snap, err := m.queries.GetCurrentEpochSnapshot(ctx)
	if err == nil {
		metrics.Stakers.Set(float64(snap.NumStakers))
		metrics.DelegatedStake.Set(float64(snap.Balance))
	}

	now := time.Now()
	if !shouldRefreshGauges(m.lastChainGaugeRefresh, now) {
		return
	}
	m.lastChainGaugeRefresh = now

	if shouldRefreshGauges(m.lastValidatorObserve, now) {
		v, err := m.chain.RPC.GetValidator(ctx, m.chain.Address())
		if err != nil {
			metrics.RPCErrors.WithLabelValues("validator").Inc()
		} else {
			m.observeValidator(v, head)
		}
	}

	bal, err := m.chain.RPC.GetBalance(ctx, m.chain.Address())
	if err != nil {
		metrics.RPCErrors.WithLabelValues("balance").Inc()
		return
	}
	metrics.WalletBalance.WithLabelValues("payout").Set(float64(bal))

	if m.cfg.PoolFeeWallet == "" {
		return
	}
	rewardAddr, err := nimiq.ParseAddress(m.cfg.PoolFeeWallet)
	if err != nil || rewardAddr == m.chain.Address() {
		return
	}
	rbal, err := m.chain.RPC.GetBalance(ctx, rewardAddr)
	if err != nil {
		metrics.RPCErrors.WithLabelValues("balance").Inc()
		return
	}
	metrics.WalletBalance.WithLabelValues("reward").Set(float64(rbal))
}

func (m *Manager) recordHeartbeat(ctx context.Context, head uint32, processed int64, tickStart time.Time) error {
	if m.recorder == nil {
		return nil
	}
	// best-effort validator state
	validatorState := "unknown"
	if v, err := m.chain.RPC.GetValidator(ctx, m.chain.Address()); err == nil {
		validatorState = validatorLiveState(v, head)
	}
	hb := ops.Heartbeat{
		HeartbeatAt:             time.Now().UTC(),
		DaemonVersion:           "",
		ConfigHash:              "",
		DerivedValidatorAddress: m.chain.Address().String(),
		ValidatorState:          validatorState,
		LastProcessedHeight:     processed,
		ChainHead:               int64(head),
		LastTickMs:              int64(time.Since(tickStart).Milliseconds()),
		RPCOk:                   true,
		ReadinessError:          "",
	}
	return m.recorder.RecordHeartbeat(ctx, hb)
}

func (m *Manager) recordSnapshot(ctx context.Context, head uint32, processed int64, tickStart time.Time) error {
	if m.recorder == nil {
		return nil
	}
	stats, _ := m.queries.GetPayslipStats(ctx)
	snap, _ := m.queries.GetCurrentEpochSnapshot(ctx)
	bal, _ := m.chain.RPC.GetBalance(ctx, m.chain.Address())
	validatorState := "unknown"
	if v, err := m.chain.RPC.GetValidator(ctx, m.chain.Address()); err == nil {
		validatorState = validatorLiveState(v, head)
	}
	s := ops.Snapshot{
		RecordedAt:          time.Now().UTC(),
		ChainHead:           int64(head),
		ProcessedHeight:     processed,
		TickMs:              int64(time.Since(tickStart).Milliseconds()),
		ValidatorState:      validatorState,
		LiveStake:           0,
		StakerCount:         snap.NumStakers,
		PendingPayoutCount:  stats.PendingCount,
		PendingPayoutLuna:   stats.PendingLuna,
		StuckPayoutCount:    stats.StuckCount,
		StuckPayoutLuna:     0,
		WalletBalance:       int64(bal),
		RPCOk:               true,
	}
	return m.recorder.RecordSnapshot(ctx, s)
}

// Run is the daemon's main loop: replay every height between the last
// processed cursor and the current chain head, in order, then sleep.
func (m *Manager) Run(ctx context.Context) error {
	policy, err := m.chain.RPC.GetPolicy(ctx)
	if err != nil {
		return err
	}
	m.policy = policy

	// Readiness check: validator address must match config
	derived := m.chain.Address().String()
	if m.cfg.ValidatorAddress != "" && derived != m.cfg.ValidatorAddress {
		if m.recorder != nil {
			_ = m.recorder.RecordEvent(ctx, ops.EventInput{
				Severity: "error",
				Category: "readiness",
				Source:   "daemon",
				Type:     "validator_address_mismatch",
				Summary:  "Derived validator address does not match config",
				Context: map[string]any{
					"derived":  derived,
					"expected": m.cfg.ValidatorAddress,
				},
			})
		}
		return nil
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		tickStart := time.Now()
		head, err := m.chain.RPC.BlockNumber(ctx)
		if err != nil {
			logger.Logger.Error("fetching block number", zap.Error(err))
			metrics.RPCErrors.WithLabelValues("block_number").Inc()
			if m.recorder != nil {
				_ = m.recorder.RecordEvent(ctx, ops.EventInput{
					Severity: "error",
					Category: "rpc",
					Source:   "daemon",
					Type:     "rpc_failure",
					Summary:  "Failed to fetch block number",
					Context:  map[string]any{"error": err.Error()},
				})
			}
			continue
		}

		// Missing cursor means "never started": include the genesis height
		// itself. Genesis is an election block, and skipping it (the old
		// cursor+1 start) left epoch 1 with no staker snapshot on a
		// genesis-0 devnet.
		start := m.policy.GenesisBlockNumber
		cursor, err := m.queries.GetCursor(ctx, CursorName)
		if err == nil {
			start = uint32(cursor) + 1
		}

		for h := start; h <= head; h++ {
			if err := m.processHeight(ctx, h); err != nil {
				logger.Logger.Error("processing height", zap.Uint32("height", h), zap.Error(err))
				metrics.RPCErrors.WithLabelValues("height").Inc()
				break
			}
			if err := m.queries.UpsertCursor(ctx, db.UpsertCursorParams{Name: CursorName, Height: int64(h)}); err != nil {
				logger.Logger.Error("advancing cursor", zap.Error(err))
				break
			}
		}

		if err := m.runPayouts(ctx); err != nil {
			logger.Logger.Error("running payouts", zap.Error(err))
		}
		if err := m.processApprovedPayouts(ctx); err != nil {
			logger.Logger.Error("processing approved payouts", zap.Error(err))
		}
		if err := m.runConfirmations(ctx); err != nil {
			logger.Logger.Error("running confirmations", zap.Error(err))
		}
		if err := m.ProcessRequestedActions(ctx); err != nil {
			logger.Logger.Error("processing requested validator actions", zap.Error(err))
		}
		if m.cfg.AutoReactivate {
			if err := m.runAutoReactivate(ctx); err != nil {
				logger.Logger.Error("running auto-reactivate", zap.Error(err))
			}
		}

		processed := int64(head)
		if start <= head {
			if c, err := m.queries.GetCursor(ctx, CursorName); err == nil {
				processed = c
			}
		}
		m.observeTickGauges(ctx, head, processed, tickStart)

		if m.recorder != nil {
			_ = m.recordHeartbeat(ctx, head, processed, tickStart)
			_ = m.recordSnapshot(ctx, head, processed, tickStart)
		}
	}
}

// processHeight dispatches a single height to election/checkpoint handling.
// Micro blocks with no election/checkpoint significance are a no-op — the
// cursor still advances past them so the loop never reprocesses a height.
func (m *Manager) processHeight(ctx context.Context, height uint32) error {
	switch m.classify(height) {
	case blockElection:
		if err := m.handleElection(ctx, height); err != nil {
			return err
		}
		// Election blocks are also batch boundaries. Collect the closing
		// batch's reward; classify() prefers election so this would
		// otherwise skip one batch per epoch.
		return m.handleCheckpoint(ctx, height)
	case blockCheckpoint:
		return m.handleCheckpoint(ctx, height)
	}
	return nil
}
