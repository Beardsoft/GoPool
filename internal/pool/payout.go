package pool

import (
	"context"
	"database/sql"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

type payoutKind int

const (
	payoutTransfer payoutKind = iota
	payoutDelegate
)

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
// total has crossed the configured minimum, builds and submits one payout
// transaction covering the whole total.
func (m *Manager) runPayouts(ctx context.Context) error {
	eligible, err := m.queries.GetEligibleForPayout(ctx, int64(m.cfg.MinPayoutLuna))
	if err != nil {
		return err
	}

	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}

	for _, row := range eligible {
		addr, err := nimiq.ParseAddress(row.Address)
		if err != nil {
			logger.Logger.Error("unparseable staker address, skipping", zap.String("address", row.Address), zap.Error(err))
			continue
		}
		amount := nimiq.Luna(uint64(row.Total))

		if err := m.queries.MarkPayslipsOutForPayment(ctx, row.Address); err != nil {
			return err
		}

		kind := payoutTransfer
		if m.cfg.PayoutMode == "delegate" {
			staker, err := m.chain.RPC.GetStaker(ctx, addr)
			stillDelegated := err == nil && staker.Delegation == m.chain.Address().String()
			kind = choosePayoutTx(m.cfg.PayoutMode, stillDelegated)
		}

		hash, err := m.submitPayout(ctx, addr, amount, kind, head)
		if err != nil {
			logger.Logger.Error("payout submission failed, will retry next tick", zap.String("address", row.Address), zap.Error(err))
			continue
		}

		if err := m.queries.SetPayslipsTransaction(ctx, db.SetPayslipsTransactionParams{
			TxHash:  sql.NullString{String: hash, Valid: true},
			Address: row.Address,
		}); err != nil {
			return err
		}
		if err := m.queries.InsertTransaction(ctx, db.InsertTransactionParams{
			Hash: hash, Address: row.Address, Amount: int64(amount), Status: "awaiting_confirmation",
		}); err != nil {
			return err
		}
		logger.Logger.Info("payout submitted", zap.String("staker", row.Address), zap.Uint64("amount", uint64(amount)), zap.String("tx", hash))
	}
	return nil
}

func (m *Manager) submitPayout(ctx context.Context, recipient nimiq.Address, amount nimiq.Luna, kind payoutKind, head uint32) (string, error) {
	sender := m.chain.Address()
	if kind == payoutDelegate {
		tx, err := nimiq.NewAddStakeTransaction(sender, recipient, amount, 0, head, m.chain.Network)
		if err != nil {
			return "", err
		}
		if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
			return "", err
		}
		return m.chain.RPC.SendTransaction(ctx, tx)
	}
	tx, err := nimiq.NewBasicTransaction(sender, recipient, amount, 0, head, m.chain.Network)
	if err != nil {
		return "", err
	}
	if err := nimiq.SignTransaction(ctx, m.chain.Signer, tx); err != nil {
		return "", err
	}
	return m.chain.RPC.SendTransaction(ctx, tx)
}
