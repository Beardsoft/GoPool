package pool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/metrics"
	"github.com/Beardsoft/GoPool/internal/notifier"
	"github.com/Beardsoft/GoPool/internal/ops"

	"go.uber.org/zap"
)

type confirmOutcome int

const (
	outcomePending confirmOutcome = iota
	outcomeSucceeded
	outcomeFailed
)

// confirmationOutcome classifies a Confirmation. Only a macro-final result is
// decisive — a micro-block-included transaction under Albatross can still be
// reverted, so "pending" covers included-but-not-final as well as
// not-yet-included.
func confirmationOutcome(conf *rpc.Confirmation) confirmOutcome {
	if !conf.Final {
		return outcomePending
	}
	if conf.Succeeded {
		return outcomeSucceeded
	}
	return outcomeFailed
}

// runConfirmations settles every pending payout transaction: succeeded ones
// finalize their payslips, while failed ones remain visibly failed until an
// operator explicitly retries the payout group. It then rolls any epoch whose
// every payslip is settled into "completed".
func (m *Manager) runConfirmations(ctx context.Context) error {
	pending, err := m.queries.GetPendingTransactions(ctx)
	if err != nil {
		return err
	}
	head, _ := m.chain.RPC.BlockNumber(ctx)
	now := time.Now()

	for _, tx := range pending {
		submittedAt := time.Time{}
		if tx.SubmittedAt.Valid {
			submittedAt = tx.SubmittedAt.Time
		}
		stuck := IsStuck(tx.SubmittedHeight, head, m.cfg.StuckPayoutEpochs, m.policy.BlocksPerEpoch, m.policy.BlockSeparationTime, submittedAt, now)
		conf, err := m.chain.RPC.CheckTransaction(ctx, tx.Hash)
		if err != nil {
			// Broadcast txs are often not indexed until they land in a block.
			// nimiq-go documents ErrNotFound as "not yet", not failure.
			if errors.Is(err, rpc.ErrNotFound) {
				if stuck {
					if ferr := m.failStuckPayout(ctx, tx.Hash, tx.Address); ferr != nil {
						logger.Logger.Error("failing stuck payout", zap.String("hash", tx.Hash), zap.Error(ferr))
					}
				}
				continue
			}
			logger.Logger.Error("checking transaction", zap.String("hash", tx.Hash), zap.Error(err))
			metrics.RPCErrors.WithLabelValues("confirm").Inc()
			continue
		}
		switch confirmationOutcome(conf) {
		case outcomePending:
			if stuck {
				if ferr := m.failStuckPayout(ctx, tx.Hash, tx.Address); ferr != nil {
					logger.Logger.Error("failing stuck payout", zap.String("hash", tx.Hash), zap.Error(ferr))
				}
			}
			continue
		case outcomeSucceeded:
			if err := m.queries.SetTransactionStatus(ctx, db.SetTransactionStatusParams{Status: "completed", Hash: tx.Hash}); err != nil {
				return err
			}
			if err := m.queries.FinalizePayslips(ctx, sql.NullString{String: tx.Hash, Valid: true}); err != nil {
				return err
			}
			logger.Logger.Info("payout confirmed", zap.String("hash", tx.Hash))
			metrics.PayoutsConfirmed.Inc()
			if m.recorder != nil {
				_ = m.recorder.RecordEvent(ctx, ops.EventInput{
					Severity: "info",
					Category: "payout",
					Source:   "daemon",
					Type:     "payout_confirmed",
					Summary:  "Payout confirmed",
					Context: map[string]any{
						"txHash":  tx.Hash,
						"address": tx.Address,
					},
				})
			}
		case outcomeFailed:
			if err := m.queries.SetTransactionStatus(ctx, db.SetTransactionStatusParams{Status: "failed", Hash: tx.Hash}); err != nil {
				return err
			}
			if err := m.queries.UpdatePayslipStatusFailed(ctx, sql.NullString{String: tx.Hash, Valid: true}); err != nil {
				return err
			}
			logger.Logger.Warn("payout failed, marked payslips failed", zap.String("hash", tx.Hash))
			metrics.PayoutsFailed.Inc()
			if m.recorder != nil {
				_ = m.recorder.RecordEvent(ctx, ops.EventInput{
					Severity: "error",
					Category: "payout",
					Source:   "daemon",
					Type:     "payout_failed",
					Summary:  "Payout failed",
					Context: map[string]any{
						"txHash":  tx.Hash,
						"address": tx.Address,
					},
				})
			}
		}
	}

	completed, err := m.queries.FinalizeCompletedEpochs(ctx)
	if err != nil {
		return err
	}
	for _, number := range completed {
		logger.Logger.Info("epoch finalized", zap.Int64("epoch", number))
	}
	return nil
}

// failStuckPayout marks a pending payout that has been unconfirmable for more
// than the configured number of epochs as failed, so the operator can retry it.
// It never auto-retries.
func (m *Manager) failStuckPayout(ctx context.Context, hash, address string) error {
	if err := m.queries.SetTransactionStatus(ctx, db.SetTransactionStatusParams{Status: "failed", Hash: hash}); err != nil {
		return err
	}
	if err := m.queries.UpdatePayslipStatusFailed(ctx, sql.NullString{String: hash, Valid: true}); err != nil {
		return err
	}
	logger.Logger.Warn("payout stuck, marked failed for operator retry",
		zap.String("hash", hash), zap.String("address", address))
	metrics.PayoutsFailed.Inc()
	if m.recorder != nil {
		_ = m.recorder.RecordEvent(ctx, ops.EventInput{
			Severity: "error",
			Category: "payout",
			Source:   "daemon",
			Type:     "payout_stuck",
			Summary:  "Payout stuck, marked failed for operator retry",
			Context: map[string]any{
				"txHash":  hash,
				"address": address,
			},
		})
	}
	if m.notifier != nil {
		m.notifier.Send(ctx, notifier.Alert{
			Level:   "error",
			Type:    "payout_stuck",
			Title:   "Payout stuck, marked failed",
			Message: fmt.Sprintf("Unconfirmable for more than %d epochs. Retry it from the operator console.", m.cfg.StuckPayoutEpochs),
			Fields: []notifier.AlertField{
				{Name: "Recipient", Value: address},
				{Name: "Tx", Value: hash},
			},
		})
	}
	return nil
}
