package pool

import (
	"context"
	"database/sql"
	"fmt"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

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
		return err
	}
	if !shouldReactivate(validator.JailedFrom, head) {
		return nil
	}

	pending, err := m.queries.HasPendingValidatorAction(ctx, actionReactivate)
	if err != nil {
		return err
	}
	if pending != 0 {
		return nil
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
		return err
	}
	logger.Logger.Info("validator reactivation submitted", zap.String("tx", hash))
	return nil
}

// Deactivate, Retire, and Delete are operator-triggered, run from the CLI in
// cmd/main.go — never from the daemon loop. Deactivating, retiring, or
// deleting a validator affects delegator funds and pool operation; a human
// must choose to do it.

// Deactivate submits a DeactivateValidator transaction for this pool.
func (m *Manager) Deactivate(ctx context.Context) error {
	addr := m.chain.Address()
	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}
	tx, err := nimiq.NewDeactivateValidatorTransaction(addr, addr, 0, head, m.chain.Network)
	if err != nil {
		return err
	}
	if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
		return err
	}
	hash, err := m.chain.RPC.SendTransaction(ctx, tx)
	if err != nil {
		return err
	}
	fmt.Printf("deactivate transaction submitted: %s\n", hash)
	return nil
}

// Retire submits a RetireValidator transaction for this pool.
func (m *Manager) Retire(ctx context.Context) error {
	addr := m.chain.Address()
	head, err := m.chain.RPC.BlockNumber(ctx)
	if err != nil {
		return err
	}
	tx, err := nimiq.NewRetireValidatorTransaction(addr, 0, head, m.chain.Network)
	if err != nil {
		return err
	}
	if err := nimiq.SignStakingTransaction(ctx, tx, m.chain.Signer, m.chain.Signer); err != nil {
		return err
	}
	hash, err := m.chain.RPC.SendTransaction(ctx, tx)
	if err != nil {
		return err
	}
	fmt.Printf("retire transaction submitted: %s\n", hash)
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
