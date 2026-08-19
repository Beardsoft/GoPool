package pool

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/ops"

	"go.uber.org/zap"
)

const (
	faucetTargetLuna = nimiq.Luna(10_100_000_000) // 101k NIM
	feeReserveLuna   = nimiq.Luna(1_000_000)      // 10 NIM
	actionCreate     = "create"
	actionSelfStake  = "self_stake"
	lunaPerNIM       = 100_000
	registerCooldown = 60 * time.Second
)

type bootstrapAction int

const (
	bootstrapWait bootstrapAction = iota
	bootstrapFaucet
	bootstrapRegister
	bootstrapSelfStake
	bootstrapReady
)

type bootstrapSnapshot struct {
	Network          string
	Balance          nimiq.Luna
	Deposit          nimiq.Luna
	MinStake         nimiq.Luna
	FaucetEnabled    bool
	HasWalletJSON    bool
	LocalConsensus   bool
	ValidatorExists  bool
	HasSelfStaker    bool
	PendingRegister  bool
	PendingSelfStake bool
	DryRun           bool
	Address          string
	FaucetBlocked    bool
	FaucetRetryAt    time.Time
}

func nextBootstrap(s bootstrapSnapshot) bootstrapAction {
	if s.DryRun {
		return bootstrapWait
	}
	if s.ValidatorExists && s.HasSelfStaker {
		return bootstrapReady
	}
	if s.ValidatorExists {
		if !s.PendingSelfStake && selfStakeAmount(s.Balance, s.MinStake) > 0 {
			return bootstrapSelfStake
		}
		return bootstrapWait
	}
	if s.HasWalletJSON && s.LocalConsensus && s.Balance >= s.Deposit && !s.PendingRegister {
		return bootstrapRegister
	}
	if s.FaucetEnabled && s.Balance < s.Deposit {
		return bootstrapFaucet
	}
	return bootstrapWait
}

func selfStakeAmount(balance, minStake nimiq.Luna) nimiq.Luna {
	if balance <= feeReserveLuna {
		return 0
	}
	amount := balance - feeReserveLuna
	if amount < minStake {
		return 0
	}
	return amount
}

func bootstrapWaitingError(s bootstrapSnapshot) string {
	have := int64(s.Balance) / lunaPerNIM
	if s.ValidatorExists && !s.HasSelfStaker {
		return fmt.Sprintf("Waiting to self-stake leftover NIM (have %d NIM)", have)
	}
	need := int64(s.Deposit) / lunaPerNIM
	if need == 0 {
		need = int64(faucetTargetLuna) / lunaPerNIM
	}
	msg := fmt.Sprintf("Waiting for %d NIM to register the validator (have %d NIM)", need, have)
	if !s.FaucetBlocked {
		return msg
	}
	want := int64(faucetTargetLuna) / lunaPerNIM
	msg += fmt.Sprintf(". The testnet faucet cannot fund this address now; it will be retried after %s. Send at least %d NIM to %s yourself if you cannot wait.",
		s.FaucetRetryAt.UTC().Format(time.RFC3339), want, strings.TrimSpace(s.Address))
	return msg
}

func (m *Manager) snapshotBootstrap(ctx context.Context) (bootstrapSnapshot, error) {
	s := bootstrapSnapshot{
		Network:       m.cfg.Network,
		Deposit:       m.policy.ValidatorDeposit,
		MinStake:      m.policy.MinimumStake,
		FaucetEnabled: m.cfg.FaucetURL != "" && m.cfg.Network == "test-albatross",
		DryRun:        m.cfg.DryRun,
		Address:       m.chain.Address().String(),
		FaucetBlocked: m.faucetBlocked,
		FaucetRetryAt: m.faucetRetryAt,
	}
	if m.cfg.WalletJSONFile != "" {
		if _, err := os.Stat(m.cfg.WalletJSONFile); err == nil {
			s.HasWalletJSON = true
		}
	}
	if m.chain.Wallet != nil {
		ok, err := m.chain.Wallet.ConsensusEstablished(ctx)
		if err == nil {
			s.LocalConsensus = ok
		}
	}
	bal, err := m.chain.RPC.GetBalance(ctx, m.chain.Address())
	if err == nil {
		s.Balance = bal
	}
	if _, err := m.chain.RPC.GetValidator(ctx, m.chain.Address()); err == nil {
		s.ValidatorExists = true
	}
	if _, err := m.chain.RPC.GetStaker(ctx, m.chain.Address()); err == nil {
		s.HasSelfStaker = true
	}
	if pending, err := m.queries.HasPendingValidatorAction(ctx, actionCreate); err == nil {
		s.PendingRegister = pending
	}
	if pending, err := m.queries.HasPendingValidatorAction(ctx, actionSelfStake); err == nil {
		s.PendingSelfStake = pending
	}
	return s, nil
}

func (m *Manager) runBootstrap(ctx context.Context) bootstrapSnapshot {
	s, err := m.snapshotBootstrap(ctx)
	if err != nil {
		return s
	}
	switch nextBootstrap(s) {
	case bootstrapFaucet:
		if !m.lastFaucetAt.IsZero() && time.Now().Before(m.faucetRetryAt) {
			break
		}
		m.lastFaucetAt = time.Now()
		result := fundAddress(ctx, nil, m.cfg.FaucetURL, m.chain.Address().String())
		retry := result.RetryAfter
		if retry <= 0 {
			retry = defaultFaucetRetry
		}
		m.faucetRetryAt = m.lastFaucetAt.Add(retry)
		if result.OK {
			m.faucetBlocked = false
			logger.Logger.Info("requested testnet faucet funds", zap.String("address", m.chain.Address().String()), zap.Duration("retry_after", retry))
			m.recordBootstrapEvent(ctx, "info", "faucet_requested", "Requested testnet faucet funds", nil)
			break
		}
		m.faucetBlocked = true
		logger.Logger.Warn("testnet faucet did not fund", zap.String("address", m.chain.Address().String()), zap.Bool("rate_limited", result.RateLimited), zap.Duration("retry_after", retry), zap.String("detail", result.Message))
		m.recordBootstrapEvent(ctx, "warning", "faucet_unavailable", "Testnet faucet did not fund; send NIM to the validator address or wait to retry", map[string]any{
			"error": result.Message, "retryAfter": retry.String(), "rateLimited": result.RateLimited,
		})
	case bootstrapRegister:
		if time.Since(m.lastRegisterAt) < registerCooldown {
			break
		}
		m.lastRegisterAt = time.Now()
		if err := m.registerValidator(ctx); err != nil {
			logger.Logger.Error("validator registration", zap.Error(err))
			m.recordBootstrapEvent(ctx, "error", "validator_registration_failed", "Validator registration failed", map[string]any{"error": err.Error()})
		}
	case bootstrapSelfStake:
		if err := m.selfStake(ctx, selfStakeAmount(s.Balance, s.MinStake)); err != nil {
			logger.Logger.Error("validator self-stake", zap.Error(err))
			m.recordBootstrapEvent(ctx, "error", "validator_self_stake_failed", "Validator self-stake failed", map[string]any{"error": err.Error()})
		}
	}
	s.FaucetBlocked = m.faucetBlocked
	s.FaucetRetryAt = m.faucetRetryAt
	return s
}

func (m *Manager) registerValidator(ctx context.Context) error {
	if m.chain.Wallet == nil {
		return fmt.Errorf("validator RPC is not configured")
	}
	keys, err := chain.LoadWalletJSON(m.cfg.WalletJSONFile)
	if err != nil {
		return err
	}
	addr := m.chain.Address().String()
	hash, err := m.chain.Wallet.CreateValidator(ctx, addr, m.cfg.PrivateKey, keys.SigningPrivateKey, keys.VotingSecretKey)
	outcome := "pending"
	if err != nil {
		outcome = "failed"
	}
	if insertErr := m.queries.InsertValidatorAction(ctx, db.InsertValidatorActionParams{
		Action:  actionCreate,
		TxHash:  sql.NullString{String: hash, Valid: hash != ""},
		Outcome: outcome,
	}); insertErr != nil {
		return insertErr
	}
	if err != nil {
		return err
	}
	logger.Logger.Info("validator registration submitted", zap.String("tx", hash))
	m.recordBootstrapEvent(ctx, "info", "validator_registration_submitted", "Validator registration submitted", map[string]any{"txHash": hash})
	return nil
}

func (m *Manager) selfStake(ctx context.Context, amount nimiq.Luna) error {
	addr := m.chain.Address()
	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}
	probe, err := nimiq.NewCreateStakerTransaction(addr, &addr, amount, 0, head, m.chain.Network)
	if err != nil {
		return err
	}
	fee, err := m.chain.RPC.EstimateFee(ctx, probe)
	if err != nil {
		fee = 0
	}
	tx, err := nimiq.NewCreateStakerTransaction(addr, &addr, amount, fee, head, m.chain.Network)
	if err != nil {
		return err
	}
	if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
		return err
	}
	hash, err := m.chain.RPC.SendTransaction(ctx, tx)
	outcome := "pending"
	if err != nil {
		outcome = "failed"
	}
	if insertErr := m.queries.InsertValidatorAction(ctx, db.InsertValidatorActionParams{
		Action:  actionSelfStake,
		TxHash:  sql.NullString{String: hash, Valid: hash != ""},
		Outcome: outcome,
	}); insertErr != nil {
		return insertErr
	}
	if err != nil {
		return err
	}
	logger.Logger.Info("validator self-stake submitted", zap.String("tx", hash))
	m.recordBootstrapEvent(ctx, "info", "validator_self_stake_submitted", "Validator self-stake submitted", map[string]any{"txHash": hash})
	return nil
}

func (m *Manager) recordBootstrapEvent(ctx context.Context, severity, typ, summary string, contextMap map[string]any) {
	if m.recorder == nil {
		return
	}
	_ = m.recorder.RecordEvent(ctx, ops.EventInput{
		Severity: severity, Category: "validator", Source: "daemon", Type: typ, Summary: summary, Context: contextMap,
	})
}
