package pool

import (
	"context"
	"time"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

type blockKind int

const (
	blockMicro blockKind = iota
	blockCheckpoint
	blockElection
)

const cursorName = "last_processed_height"

// Manager is the pool daemon: it replays chain heights and runs payouts.
type Manager struct {
	chain   *chain.Chain
	queries *db.Queries
	cfg     *config.Config
	policy  *rpc.Policy
}

// NewManager builds a Manager. Policy is loaded on Run.
func NewManager(c *chain.Chain, q *db.Queries, cfg *config.Config) *Manager {
	return &Manager{chain: c, queries: q, cfg: cfg}
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

// Run is the daemon's main loop: replay every height between the last
// processed cursor and the current chain head, in order, then sleep.
func (m *Manager) Run(ctx context.Context) error {
	policy, err := m.chain.RPC.GetPolicy(ctx)
	if err != nil {
		return err
	}
	m.policy = policy

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		head, err := m.chain.RPC.BlockNumber(ctx)
		if err != nil {
			logger.Logger.Error("fetching block number", zap.Error(err))
			continue
		}

		cursor, err := m.queries.GetCursor(ctx, cursorName)
		if err != nil {
			cursor = int64(m.policy.GenesisBlockNumber)
		}

		for h := uint32(cursor) + 1; h <= head; h++ {
			if err := m.processHeight(ctx, h); err != nil {
				logger.Logger.Error("processing height", zap.Uint32("height", h), zap.Error(err))
				break
			}
			if err := m.queries.UpsertCursor(ctx, db.UpsertCursorParams{Name: cursorName, Height: int64(h)}); err != nil {
				logger.Logger.Error("advancing cursor", zap.Error(err))
				break
			}
		}
	}
}

// processHeight dispatches a single height to election/checkpoint handling.
// Micro blocks with no election/checkpoint significance are a no-op — the
// cursor still advances past them so the loop never reprocesses a height.
func (m *Manager) processHeight(ctx context.Context, height uint32) error {
	switch m.classify(height) {
	case blockElection:
		return m.handleElection(ctx, height)
	case blockCheckpoint:
		return m.handleCheckpoint(ctx, height)
	}
	return nil
}

// handleCheckpoint is filled in by checkpoint.go; this stub keeps the package
// compiling until that file lands.
func (m *Manager) handleCheckpoint(ctx context.Context, height uint32) error {
	return nil
}
