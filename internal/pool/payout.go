package pool

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/metrics"
	"github.com/Beardsoft/GoPool/internal/notifier"
	"github.com/Beardsoft/GoPool/internal/ops"

	"go.uber.org/zap"
)

type payoutKind int

const (
	payoutTransfer payoutKind = iota
	payoutDelegate
)

func payoutKindLabel(k payoutKind) string {
	if k == payoutDelegate {
		return "delegate"
	}
	return "transfer"
}

// feePayoutMultiple is how many times the tx fee the pending amount must
// cover before we send. The pool pays the fee on top; this keeps that cost
// from eating the payout.
const feePayoutMultiple = 10

// payoutWorthSending reports whether amount is large enough to justify
// paying fee. A zero fee (current Nimiq default) always passes — the SQL
// min_payout_luna gate has already run.
func payoutWorthSending(amount, fee nimiq.Luna) bool {
	if fee == 0 {
		return true
	}
	return amount >= fee*feePayoutMultiple
}

// markFeeFloorHold records that a staker is in a fee-floor hold, returning
// true on the first tick of the hold so the caller sends a single alert
// instead of one per ~2s tick.
func markFeeFloorHold(alerted map[string]bool, staker string) bool {
	if alerted[staker] {
		return false
	}
	alerted[staker] = true
	return true
}

// pruneFeeFloorHolds drops stakers that are no longer held this tick, so a
// future hold re-alerts instead of staying silent.
func pruneFeeFloorHolds(held, alerted map[string]bool) {
	for s := range alerted {
		if !held[s] {
			delete(alerted, s)
		}
	}
}

// choosePayoutTx picks the transaction shape for a payout. "delegate" mode
// restakes (compounding) only while the staker is still delegated to this
// validator; if they undelegated, it falls back to a plain transfer instead
// of restaking into a stale relationship — the fix for zpool's known TODO,
// which always restakes without checking.
func choosePayoutTx(mode string, stillDelegated bool) payoutKind {
	if mode == "delegate" && stillDelegated {
		return payoutDelegate
	}
	return payoutTransfer
}

// runPayouts sums pending payslips per staker, and for every staker whose
// total has crossed the configured minimum, builds audit intent before signing.
func (m *Manager) runPayouts(ctx context.Context) error {
	eligible, err := m.queries.GetEligibleForPayout(ctx, int64(m.cfg.MinPayoutLuna))
	if err != nil {
		return err
	}

	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}

	if m.feeFloorAlerted == nil {
		m.feeFloorAlerted = make(map[string]bool)
	}
	held := make(map[string]bool)

	for _, row := range eligible {
		addr, err := nimiq.ParseAddress(row.Address)
		if err != nil {
			logger.Logger.Error("unparseable staker address, skipping", zap.String("address", row.Address), zap.Error(err))
			continue
		}
		amount := nimiq.Luna(uint64(row.Total))

		pref, perr := m.queries.GetStakerPreference(ctx, row.Address)
		var prefPtr *bool
		if perr == nil {
			b := pref == 1
			prefPtr = &b
		}
		// Only query the chain for delegation when the resolved mode is
		// delegate; a transfer-mode staker never needs it.
		kind := payoutTransfer
		if effectivePayoutKind(m.cfg.PayoutMode, prefPtr, true) == payoutDelegate {
			staker, err := m.chain.RPC.GetStaker(ctx, addr)
			stillDelegated := err == nil && staker.Delegation == m.chain.Address().String()
			kind = effectivePayoutKind(m.cfg.PayoutMode, prefPtr, stillDelegated)
		}

		tx, err := m.buildPayoutTx(addr, amount, 0, kind, head)
		if err != nil {
			logger.Logger.Error("building payout tx", zap.String("address", row.Address), zap.Error(err))
			continue
		}
		fee, err := m.chain.RPC.EstimateFee(ctx, tx)
		if err != nil {
			logger.Logger.Error("estimating payout fee", zap.String("address", row.Address), zap.Error(err))
			continue
		}
		if !payoutWorthSending(amount, fee) {
			held[row.Address] = true
			if markFeeFloorHold(m.feeFloorAlerted, row.Address) && m.notifier != nil {
				m.notifier.Send(ctx, notifier.Alert{Level: "warning", Type: "fee_floor", Title: "Fee floor breach",
					Message: fmt.Sprintf("Payout to %s held: %d luna pending is below 10x the %d luna tx fee", row.Address, amount, fee)})
			}
			logger.Logger.Debug("holding payout until amount covers 10× fee",
				zap.String("staker", row.Address),
				zap.Uint64("amount", uint64(amount)),
				zap.Uint64("fee", uint64(fee)))
			continue
		}

		// Build intent for audit log
		intent := map[string]any{
			"address": row.Address,
			"amount":  int64(amount),
			"fee":     int64(fee),
			"kind":    payoutKindLabel(kind),
			"head":    head,
		}
		intentBytes, _ := json.Marshal(intent)
		status := "approved"
		if m.cfg.DryRun {
			status = "dry_run"
		}

		_, err = m.queries.InsertAuditLog(ctx, db.InsertAuditLogParams{
			ActionType: "payout",
			Address:    row.Address,
			Amount:     int64(amount),
			Fee:        int64(fee),
			Kind:       payoutKindLabel(kind),
			Status:     status,
			IntentData: sql.NullString{String: string(intentBytes), Valid: true},
		})
		if err != nil {
			logger.Logger.Error("inserting audit log", zap.String("address", row.Address), zap.Error(err))
			continue
		}

		if m.cfg.DryRun {
			logger.Logger.Info("dry-run payout logged", zap.String("staker", row.Address), zap.Uint64("amount", uint64(amount)))
			continue
		}

		if m.broadcaster != nil {
			m.broadcaster.Publish(PoolEvent{
				Type:      "payout_scheduled",
				Timestamp: time.Now().UnixMilli(),
				Data: mustMarshal(map[string]any{
					"address": row.Address,
					"amount":  int64(amount),
					"fee":     int64(fee),
					"kind":    payoutKindLabel(kind),
				}),
			})
		}
	}

	pruneFeeFloorHolds(held, m.feeFloorAlerted)
	return nil
}

// processApprovedPayouts signs and sends payouts whose audit log is approved.
func (m *Manager) processApprovedPayouts(ctx context.Context) error {
	if m.cfg.DryRun {
		return nil
	}

	logs, err := m.queries.ListApprovedAuditLogs(ctx)
	if err != nil {
		return err
	}

	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}

	for _, log := range logs {
		addr, err := nimiq.ParseAddress(log.Address)
		if err != nil {
			logger.Logger.Error("unparseable address in audit log", zap.Int64("id", log.ID), zap.Error(err))
			continue
		}
		amount := nimiq.Luna(uint64(log.Amount))
		fee := nimiq.Luna(uint64(log.Fee))

		kind := payoutTransfer
		if log.Kind == "delegate" {
			kind = payoutDelegate
		}

		tx, err := m.buildPayoutTx(addr, amount, fee, kind, head)
		if err != nil {
			logger.Logger.Error("building payout tx from audit log", zap.Int64("id", log.ID), zap.Error(err))
			continue
		}

		// Mark payslips out for payment before signing
		if err := m.queries.MarkPayslipsOutForPayment(ctx, log.Address); err != nil {
			logger.Logger.Error("mark payslips out", zap.Int64("id", log.ID), zap.Error(err))
			continue
		}

		hash, err := m.signAndSend(ctx, tx, kind)
		if err != nil {
			logger.Logger.Error("sign and send approved payout", zap.Int64("id", log.ID), zap.Error(err))
			_ = m.queries.ResetPayslipsOutForPayment(ctx, log.Address)
			if m.recorder != nil {
				_ = m.recorder.RecordEvent(ctx, ops.EventInput{
					Severity: "error",
					Category: "payout",
					Source:   "daemon",
					Type:     "payout_failed",
					Summary:  "Payout submission failed",
					Context: map[string]any{
						"address": log.Address,
						"amount":  log.Amount,
						"error":   err.Error(),
					},
				})
			}
			continue
		}

		// Update audit log
		if err := m.queries.UpdateAuditLogStatus(ctx, db.UpdateAuditLogStatusParams{
			Status: "executed",
			ID:     log.ID,
		}); err != nil {
			logger.Logger.Error("update audit log status", zap.Int64("id", log.ID), zap.Error(err))
		}

		// Update payslips and transactions
		_ = m.queries.SetPayslipsTransaction(ctx, db.SetPayslipsTransactionParams{
			TxHash:  sql.NullString{String: hash, Valid: true},
			Address: log.Address,
		})
		_ = m.queries.InsertTransaction(ctx, db.InsertTransactionParams{
			Hash:    hash,
			Address: log.Address,
			Amount:  log.Amount,
			Status:  "awaiting_confirmation",
		})

		logger.Logger.Info("approved payout executed", zap.Int64("audit_id", log.ID), zap.String("address", log.Address), zap.String("tx", hash))
		metrics.PayoutsSubmitted.WithLabelValues(log.Kind).Inc()
		if log.CreatedAt.Valid {
			latency := time.Since(log.CreatedAt.Time).Seconds()
			metrics.PayoutLatency.Observe(latency)
		}

		if m.broadcaster != nil {
			m.broadcaster.Publish(PoolEvent{
				Type:      "payout_sent",
				Timestamp: time.Now().UnixMilli(),
				Data: mustMarshal(map[string]any{
					"address": log.Address,
					"amount":  log.Amount,
					"fee":     log.Fee,
					"txHash":  hash,
					"kind":    log.Kind,
				}),
			})
		}
		if m.recorder != nil {
			_ = m.recorder.RecordEvent(ctx, ops.EventInput{
				Severity: "info",
				Category: "payout",
				Source:   "daemon",
				Type:     "payout_submitted",
				Summary:  "Payout submitted",
				Context: map[string]any{
					"address": log.Address,
					"amount":  log.Amount,
					"fee":     log.Fee,
					"txHash":  hash,
					"kind":    log.Kind,
				},
			})
		}
	}
	return nil
}

func (m *Manager) buildPayoutTx(recipient nimiq.Address, amount, fee nimiq.Luna, kind payoutKind, head uint32) (*nimiq.Transaction, error) {
	sender := m.chain.Address()
	if kind == payoutDelegate {
		return nimiq.NewAddStakeTransaction(sender, recipient, amount, fee, head, m.chain.Network)
	}
	return nimiq.NewBasicTransaction(sender, recipient, amount, fee, head, m.chain.Network)
}

func (m *Manager) signAndSend(ctx context.Context, tx *nimiq.Transaction, kind payoutKind) (string, error) {
	if kind == payoutDelegate {
		if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
			return "", err
		}
	} else if err := nimiq.SignTransaction(ctx, m.chain.Signer, tx); err != nil {
		return "", err
	}
	return m.chain.RPC.SendTransaction(ctx, tx)
}
