package pool

import (
	"context"
	"fmt"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/notifier"
	"github.com/Beardsoft/GoPool/internal/ops"

	"go.uber.org/zap"
)

// stakerPercentage is a display/estimation ratio only — actual payout
// amounts are computed from integer stake ratios directly in payout.go, not
// from this float, so its precision does not affect any money path.
func stakerPercentage(stake, total nimiq.Luna) float64 {
	if total == 0 {
		return 0
	}
	return float64(stake) / float64(total) * 100
}

// handleElection runs at the last micro block before an election, or the
// election block itself: it snapshots the validator's stakers and their
// share of the total delegated balance for the upcoming epoch. If the
// validator cannot earn rewards next epoch (inactive, retired, jailed, no
// stakers), the epoch is recorded with that status and no snapshot is taken.
func (m *Manager) handleElection(ctx context.Context, height uint32) error {
	nextEpoch := epochAt(m.policy, height) + 1

	// The genesis election closes epoch 0, whose stakers are exactly this
	// snapshot. The first checkpoint attributes batch 0's reward to epoch 0,
	// so without an epoch-0 row the rewards foreign key fails and the replay
	// loop is stuck forever on the first batch boundary.
	epochs := []int64{int64(nextEpoch)}
	if height == m.policy.GenesisBlockNumber {
		epochs = append(epochs, int64(epochAt(m.policy, height)))
	}

	for _, ep := range epochs {
		exists, err := m.queries.EpochExists(ctx, ep)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}

	addr := m.chain.Address()
	validator, err := m.chain.RPC.GetValidator(ctx, addr)
	if err != nil {
		var insertErr error
		for _, ep := range epochs {
			if insertErr = m.queries.InsertEpoch(ctx, db.InsertEpochParams{
				Number: ep, NumStakers: 0, Balance: 0, Status: "not_elected",
			}); insertErr != nil {
				break
			}
		}
		if m.notifier != nil {
			m.notifier.Send(ctx, notifier.Alert{Level: "warning", Type: "missed_election", Title: "Missed election",
				Message: fmt.Sprintf("Could not confirm validator election for epoch %d: %v", nextEpoch, err),
				Fields:  []notifier.AlertField{{Name: "Epoch", Value: fmt.Sprintf("%d", nextEpoch)}}})
		}
		return insertErr
	}
	m.observeValidator(validator, height)

	status := ""
	switch {
	case validator.Retired:
		status = "retired"
	case validator.InactivityFlag != nil && *validator.InactivityFlag > height:
		status = "inactive"
	case validator.JailedFrom != nil && *validator.JailedFrom > height:
		status = "jailed"
	case validator.NumStakers == 0:
		status = "no_stakers"
	}
	if status != "" {
		var insertErr error
		for _, ep := range epochs {
			if insertErr = m.queries.InsertEpoch(ctx, db.InsertEpochParams{
				Number: ep, NumStakers: 0, Balance: int64(validator.Balance), Status: status,
			}); insertErr != nil {
				break
			}
		}
		if insertErr != nil {
			return insertErr
		}
		if m.notifier != nil {
			m.notifier.Send(ctx, notifier.Alert{
				Level: "warning", Type: "validator_state", Title: "Validator state",
				Message: "Validator is " + status + " for epoch " + fmt.Sprintf("%d", nextEpoch),
				Fields: []notifier.AlertField{
					{Name: "Epoch", Value: fmt.Sprintf("%d", nextEpoch)},
					{Name: "Status", Value: status},
				},
			})
		}
		if m.recorder != nil {
			_ = m.recorder.RecordEvent(ctx, ops.EventInput{
				Severity: "warning",
				Category: "validator",
				Source:   "daemon",
				Type:     "validator_state_change",
				Summary:  "Validator state change",
				Context: map[string]any{
					"epoch":  nextEpoch,
					"height": height,
					"status": status,
				},
			})
		}
		return nil
	}

	stakers, err := m.chain.RPC.GetStakersByValidatorAddress(ctx, addr)
	if err != nil {
		return err
	}

	for _, ep := range epochs {
		if err := m.queries.InsertEpoch(ctx, db.InsertEpochParams{
			Number: ep, NumStakers: int64(len(stakers)), Balance: int64(validator.Balance), Status: "in_progress",
		}); err != nil {
			return err
		}
	}

	for _, ep := range epochs {
		for _, s := range stakers {
			if s.Balance == 0 {
				continue
			}
			if err := m.queries.InsertStaker(ctx, db.InsertStakerParams{
				EpochNumber: ep,
				Address:     s.Address,
				Stake:       int64(s.Balance),
				Percentage:  stakerPercentage(s.Balance, validator.Balance),
			}); err != nil {
				return err
			}
		}
	}

	logger.Logger.Info("validator elected for next epoch",
		zap.Uint32("epoch", nextEpoch), zap.Int("stakers", len(stakers)))
	if m.recorder != nil {
		_ = m.recorder.RecordEvent(ctx, ops.EventInput{
			Severity: "info",
			Category: "election",
			Source:   "daemon",
			Type:     "epoch_started",
			Summary:  "Validator elected for next epoch",
			Context: map[string]any{
				"epoch":      nextEpoch,
				"height":     height,
				"numStakers": len(stakers),
				"balance":    int64(validator.Balance),
			},
		})
	}
	return nil
}
