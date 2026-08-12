package pool

import (
	"context"
	"database/sql"
	"errors"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

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
// finalize their payslips, failed ones reset their payslips to pending for
// retry next payout pass. It then rolls any epoch whose every payslip is
// settled into "completed".
func (m *Manager) runConfirmations(ctx context.Context) error {
	pending, err := m.queries.GetPendingTransactions(ctx)
	if err != nil {
		return err
	}

	for _, tx := range pending {
		conf, err := m.chain.RPC.CheckTransaction(ctx, tx.Hash)
		if err != nil {
			// Broadcast txs are often not indexed until they land in a block.
			// nimiq-go documents ErrNotFound as "not yet", not failure.
			if errors.Is(err, rpc.ErrNotFound) {
				continue
			}
			logger.Logger.Error("checking transaction", zap.String("hash", tx.Hash), zap.Error(err))
			continue
		}
		switch confirmationOutcome(conf) {
		case outcomePending:
			continue
		case outcomeSucceeded:
			if err := m.queries.SetTransactionStatus(ctx, db.SetTransactionStatusParams{Status: "completed", Hash: tx.Hash}); err != nil {
				return err
			}
			if err := m.queries.FinalizePayslips(ctx, sql.NullString{String: tx.Hash, Valid: true}); err != nil {
				return err
			}
			logger.Logger.Info("payout confirmed", zap.String("hash", tx.Hash))
		case outcomeFailed:
			if err := m.queries.SetTransactionStatus(ctx, db.SetTransactionStatusParams{Status: "failed", Hash: tx.Hash}); err != nil {
				return err
			}
			if err := m.queries.ResetPayslipsToPending(ctx, sql.NullString{String: tx.Hash, Valid: true}); err != nil {
				return err
			}
			logger.Logger.Warn("payout failed, reset for retry", zap.String("hash", tx.Hash))
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
