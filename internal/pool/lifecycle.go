package pool

import (
	"context"
	"database/sql"
	"fmt"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/metrics"
	"github.com/Beardsoft/GoPool/internal/ops"

	"go.uber.org/zap"
)

// shouldReactivate reports whether a jailed validator's cooldown has elapsed.
// A nil jailedFrom means the validator is not jailed at all.
func shouldReactivate(jailedFrom *uint32, height uint32) bool {
	if jailedFrom == nil {
		return false
	}
	return height >= *jailedFrom
}

const actionReactivate = "reactivate"

// runAutoReactivate sends a ReactivateValidator transaction once, the first
// tick after a jail cooldown elapses. Guarded against resending every tick
// by checking for an unconfirmed action row first — see the "pending"
// outcome recorded below and cleared once the daemon observes the
// validator is no longer jailed.
func (m *Manager) runAutoReactivate(ctx context.Context) error {
	addr := m.chain.Address()
	validator, err := m.chain.RPC.GetValidator(ctx, addr)
	if err != nil {
		return nil // not a validator (yet); nothing to reactivate
	}

	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		metrics.RPCErrors.WithLabelValues("reactivate").Inc()
		return err
	}
	m.observeValidator(validator, head)
	if !shouldReactivate(validator.JailedFrom, head) {
		return nil
	}

	pending, err := m.queries.HasPendingValidatorAction(ctx, actionReactivate)
	if err != nil {
		return err
	}
	if pending {
		return nil
	}

	// Operator deactivate/retire (requested or pending) must win: both
	// steps run in the same tick, and a reactivate would undo them.
	for _, action := range []string{"deactivate", "retire"} {
		outstanding, err := m.queries.HasOutstandingValidatorAction(ctx, action)
		if err != nil {
			return err
		}
		if outstanding {
			return nil
		}
	}

	tx, err := nimiq.NewReactivateValidatorTransaction(addr, addr, 0, head, m.chain.Network)
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
		Action:  actionReactivate,
		TxHash:  sql.NullString{String: hash, Valid: hash != ""},
		Outcome: outcome,
	}); insertErr != nil {
		return insertErr
	}
	if err != nil {
		metrics.RPCErrors.WithLabelValues("reactivate").Inc()
		return err
	}
	logger.Logger.Info("validator reactivation submitted", zap.String("tx", hash))
	if m.recorder != nil {
		_ = m.recorder.RecordEvent(ctx, ops.EventInput{
			Severity: "info",
			Category: "validator",
			Source:   "daemon",
			Type:     "reactivation_submitted",
			Summary:  "Validator reactivation submitted",
			Context: map[string]any{
				"txHash": hash,
			},
		})
	}
	return nil
}

// Deactivate and Retire are operator-triggered: from the CLI in cmd/main.go,
// or queued via the API as validator_actions rows (outcome = "requested")
// and executed by ProcessRequestedActions. Delete stays CLI-only — it needs
// a recipient and deposit value the request row has no place for.

// Deactivate submits a DeactivateValidator transaction for this pool.
func (m *Manager) Deactivate(ctx context.Context) (string, error) {
	addr := m.chain.Address()
	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return "", err
	}
	tx, err := nimiq.NewDeactivateValidatorTransaction(addr, addr, 0, head, m.chain.Network)
	if err != nil {
		return "", err
	}
	if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
		return "", err
	}
	return m.chain.RPC.SendTransaction(ctx, tx)
}

// Retire submits a RetireValidator transaction for this pool.
func (m *Manager) Retire(ctx context.Context) (string, error) {
	addr := m.chain.Address()
	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return "", err
	}
	tx, err := nimiq.NewRetireValidatorTransaction(addr, 0, head, m.chain.Network)
	if err != nil {
		return "", err
	}
	if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
		return "", err
	}
	return m.chain.RPC.SendTransaction(ctx, tx)
}

// actionForRequest maps a validator_actions.action string to the Manager
// method the poll loop should call. Only deactivate/retire are pollable —
// delete needs an operator-supplied recipient and deposit value the request
// row has no place for, so it stays CLI-only (see cmd/main.go).
func actionForRequest(action string) (func(*Manager, context.Context) (string, error), error) {
	switch action {
	case "deactivate":
		return (*Manager).Deactivate, nil
	case "retire":
		return (*Manager).Retire, nil
	}
	return nil, fmt.Errorf("pool: action %q is not requestable, only deactivate/retire are", action)
}

// ProcessRequestedActions submits every validator_actions row an operator
// queued via the API (outcome = "requested"), then records the outcome.
// This is the only bridge between the API (which never holds the validator's
// key) and an actual signed transaction — the API writes the request, the
// daemon (which does hold the key) executes it.
func (m *Manager) ProcessRequestedActions(ctx context.Context) error {
	requests, err := m.queries.GetRequestedValidatorActions(ctx)
	if err != nil {
		return err
	}
	for _, req := range requests {
		fn, err := actionForRequest(req.Action)
		if err != nil {
			logger.Logger.Error("skipping unrequestable validator action", zap.String("action", req.Action), zap.Error(err))
			if setErr := m.queries.SetValidatorActionOutcome(ctx, db.SetValidatorActionOutcomeParams{
				Outcome: "failed", ID: req.ID,
			}); setErr != nil {
				return setErr
			}
			continue
		}

		// Leave the row as "requested" on shutdown/timeout so a restart retries.
		if err := ctx.Err(); err != nil {
			return err
		}
		hash, err := fn(m, ctx)
		if err := ctx.Err(); err != nil {
			return err
		}
		outcome := "pending"
		if err != nil {
			outcome = "failed"
			logger.Logger.Error("requested validator action failed", zap.String("action", req.Action), zap.Error(err))
		} else {
			logger.Logger.Info("requested validator action submitted", zap.String("action", req.Action), zap.String("tx", hash))
		}
		if setErr := m.queries.SetValidatorActionOutcome(ctx, db.SetValidatorActionOutcomeParams{
			Outcome: outcome, TxHash: sql.NullString{String: hash, Valid: hash != ""}, ID: req.ID,
		}); setErr != nil {
			return setErr
		}
		if m.recorder != nil {
			_ = m.recorder.RecordEvent(ctx, ops.EventInput{
				Severity:  "info",
				Category:  "validator",
				Source:    "daemon",
				Type:      "validator_action",
				Summary:   "Validator action processed",
				Context: map[string]any{
					"action": req.Action,
					"outcome": outcome,
					"txHash": hash,
				},
				CorrelationID: fmt.Sprintf("validator-action-%d", req.ID),
			})
		}
	}
	return nil
}

// Delete submits a DeleteValidator transaction, sending the deposit to recipient.
func (m *Manager) Delete(ctx context.Context, recipient nimiq.Address, value nimiq.Luna) error {
	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}
	tx, err := nimiq.NewDeleteValidatorTransaction(recipient, value, 0, head, m.chain.Network)
	if err != nil {
		return err
	}
	if err := nimiq.SignTransaction(ctx, m.chain.Signer, tx); err != nil {
		return err
	}
	hash, err := m.chain.RPC.SendTransaction(ctx, tx)
	if err != nil {
		return err
	}
	fmt.Printf("delete transaction submitted: %s\n", hash)
	return nil
}

// validatorLiveState maps GetValidator fields to the 1/0 gauge enum.
// Inactive/jailed tests match handleElection.
func validatorLiveState(v *rpc.Validator, height uint32) string {
	switch {
	case v.Retired:
		return "retired"
	case v.InactivityFlag != nil && *v.InactivityFlag > height:
		return "inactive"
	case v.JailedFrom != nil && *v.JailedFrom > height:
		return "jailed"
	}
	return "active"
}
