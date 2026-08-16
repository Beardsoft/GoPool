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

func payoutFitsBalance(balance, amount, fee nimiq.Luna) bool {
	total, err := amount.Add(fee)
	return err == nil && balance >= total
}

func markHold(alerted map[string]bool, staker string) bool {
	if alerted[staker] {
		return false
	}
	alerted[staker] = true
	return true
}

func pruneHolds(held, alerted map[string]bool) {
	for staker := range alerted {
		if !held[staker] {
			delete(alerted, staker)
		}
	}
}

// markFeeFloorHold records that a staker is in a fee-floor hold, returning
// true on the first tick of the hold so the caller sends a single alert
// instead of one per ~2s tick.
func markFeeFloorHold(alerted map[string]bool, staker string) bool {
	return markHold(alerted, staker)
}

// pruneFeeFloorHolds drops stakers that are no longer held this tick, so a
// future hold re-alerts instead of staying silent.
func pruneFeeFloorHolds(held, alerted map[string]bool) {
	pruneHolds(held, alerted)
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
	available, err := m.chain.RPC.GetBalance(ctx, m.chain.Address())
	if err != nil {
		return fmt.Errorf("loading pool wallet balance: %w", err)
	}

	if m.feeFloorAlerted == nil {
		m.feeFloorAlerted = make(map[string]bool)
	}
	if m.balanceHoldAlerted == nil {
		m.balanceHoldAlerted = make(map[string]bool)
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
		if !payoutFitsBalance(available, amount, fee) {
			if markHold(m.balanceHoldAlerted, row.Address) && m.notifier != nil {
				m.notifier.Send(ctx, notifier.Alert{
					Level: "error", Type: "insufficient_payout_balance", Title: "Pool wallet needs funding",
					Message: fmt.Sprintf("Payout to %s held: pool wallet has %d luna available but needs %d luna plus %d luna fee", row.Address, available, amount, fee),
				})
			}
			logger.Logger.Warn("holding payout because pool wallet balance is insufficient",
				zap.String("staker", row.Address), zap.Uint64("available", uint64(available)),
				zap.Uint64("amount", uint64(amount)), zap.Uint64("fee", uint64(fee)))
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
		total, _ := amount.Add(fee)
		available -= total
		delete(m.balanceHoldAlerted, row.Address)

		if m.cfg.DryRun {
			logger.Logger.Info("dry-run payout logged", zap.String("staker", row.Address), zap.Uint64("amount", uint64(amount)))
			continue
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
	available, err := m.chain.RPC.GetBalance(ctx, m.chain.Address())
	if err != nil {
		return fmt.Errorf("loading pool wallet balance: %w", err)
	}
	if m.balanceHoldAlerted == nil {
		m.balanceHoldAlerted = make(map[string]bool)
	}

	for _, log := range logs {
		addr, err := nimiq.ParseAddress(log.Address)
		if err != nil {
			logger.Logger.Error("unparseable address in audit log", zap.Int64("id", log.ID), zap.Error(err))
			continue
		}
		amount := nimiq.Luna(uint64(log.Amount))
		fee := nimiq.Luna(uint64(log.Fee))
		if !payoutFitsBalance(available, amount, fee) {
			if markHold(m.balanceHoldAlerted, log.Address) && m.notifier != nil {
				m.notifier.Send(ctx, notifier.Alert{
					Level: "error", Type: "insufficient_payout_balance", Title: "Pool wallet needs funding",
					Message: fmt.Sprintf("Approved payout to %s held: pool wallet has %d luna available but needs %d luna plus %d luna fee", log.Address, available, amount, fee),
				})
			}
			continue
		}

		kind := payoutTransfer
		if log.Kind == "delegate" {
			kind = payoutDelegate
		}

		tx, err := m.buildPayoutTx(addr, amount, fee, kind, head)
		if err != nil {
			logger.Logger.Error("building payout tx from audit log", zap.Int64("id", log.ID), zap.Error(err))
			continue
		}

		if err := m.signPayout(ctx, tx, kind); err != nil {
			logger.Logger.Error("sign approved payout", zap.Int64("id", log.ID), zap.Error(err))
			continue
		}
		hash := tx.Hash().String()
		if err := m.persistPayoutSubmission(ctx, log, hash, head); err != nil {
			logger.Logger.Error("persist payout before broadcast", zap.Int64("id", log.ID), zap.Error(err))
			continue
		}
		total, _ := amount.Add(fee)
		available -= total
		delete(m.balanceHoldAlerted, log.Address)

		if _, err := m.chain.RPC.SendTransaction(ctx, tx); err != nil {
			// The node may have accepted the transaction even if the response was
			// lost. Keep the locally-computed hash awaiting confirmation rather
			// than rebuilding and risking a second payment.
			logger.Logger.Error("broadcast approved payout; tracking for confirmation", zap.Int64("id", log.ID), zap.String("tx", hash), zap.Error(err))
			if m.recorder != nil {
				_ = m.recorder.RecordEvent(ctx, ops.EventInput{
					Severity: "error",
					Category: "payout",
					Source:   "daemon",
					Type:     "payout_broadcast_uncertain",
					Summary:  "Payout broadcast result is uncertain; tracking hash without automatic retry",
					Context: map[string]any{
						"address": log.Address,
						"amount":  log.Amount,
						"txHash":  hash,
						"error":   err.Error(),
					},
				})
			}
			continue
		}

		logger.Logger.Info("approved payout executed", zap.Int64("audit_id", log.ID), zap.String("address", log.Address), zap.String("tx", hash))
		metrics.PayoutsSubmitted.WithLabelValues(log.Kind).Inc()
		if log.CreatedAt.Valid {
			latency := time.Since(log.CreatedAt.Time).Seconds()
			metrics.PayoutLatency.Observe(latency)
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

func newPayoutTransaction(hash, address string, amount int64, head uint32) db.InsertTransactionParams {
	return db.InsertTransactionParams{
		Hash: hash, Address: address, Amount: amount, Status: "awaiting_confirmation", SubmittedHeight: int64(head),
	}
}

func (m *Manager) persistPayoutSubmission(ctx context.Context, log db.ListApprovedAuditLogsRow, hash string, head uint32) error {
	return m.queries.InTx(ctx, func(q *db.Queries) error {
		claimed, err := q.ClaimApprovedAuditLog(ctx, db.ClaimApprovedAuditLogParams{TxHash: sql.NullString{String: hash, Valid: true}, ID: log.ID})
		if err != nil {
			return err
		}
		if claimed != 1 {
			return fmt.Errorf("audit log %d is no longer approved", log.ID)
		}
		if err := q.MarkPayslipsOutForPayment(ctx, log.Address); err != nil {
			return err
		}
		if err := q.SetPayslipsTransaction(ctx, db.SetPayslipsTransactionParams{
			TxHash: sql.NullString{String: hash, Valid: true}, Address: log.Address,
		}); err != nil {
			return err
		}
		return q.InsertTransaction(ctx, newPayoutTransaction(hash, log.Address, log.Amount, head))
	})
}

func (m *Manager) buildPayoutTx(recipient nimiq.Address, amount, fee nimiq.Luna, kind payoutKind, head uint32) (*nimiq.Transaction, error) {
	sender := m.chain.Address()
	if kind == payoutDelegate {
		return nimiq.NewAddStakeTransaction(sender, recipient, amount, fee, head, m.chain.Network)
	}
	return nimiq.NewBasicTransaction(sender, recipient, amount, fee, head, m.chain.Network)
}

func (m *Manager) signPayout(ctx context.Context, tx *nimiq.Transaction, kind payoutKind) error {
	if kind == payoutDelegate {
		if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
			return err
		}
	} else if err := nimiq.SignTransaction(ctx, m.chain.Signer, tx); err != nil {
		return err
	}
	return nil
}
