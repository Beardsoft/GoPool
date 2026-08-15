package pool

import (
	"context"
	"fmt"
	"time"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
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

	exists, err := m.queries.EpochExists(ctx, int64(nextEpoch))
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	addr := m.chain.Address()
	validator, err := m.chain.RPC.GetValidator(ctx, addr)
	if err != nil {
		insertErr := m.queries.InsertEpoch(ctx, db.InsertEpochParams{
			Number: int64(nextEpoch), NumStakers: 0, Balance: 0, Status: "not_elected",
		})
		if m.notifier != nil {
			m.notifier.Send("warning", "Missed election",
				fmt.Sprintf("Could not confirm validator election for epoch %d: %v", nextEpoch, err))
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
		if err := m.queries.InsertEpoch(ctx, db.InsertEpochParams{
			Number: int64(nextEpoch), NumStakers: 0, Balance: int64(validator.Balance), Status: status,
		}); err != nil {
			return err
		}
		if m.notifier != nil {
			m.notifier.Send("warning", "Validator state", "Validator is "+status+" for epoch "+fmt.Sprintf("%d", nextEpoch))
		}
		if m.broadcaster != nil {
			m.broadcaster.Publish(PoolEvent{
				Type:      "validator_state_change",
				Timestamp: time.Now().UnixMilli(),
				Data: mustMarshal(map[string]any{
					"epoch":  nextEpoch,
					"height": height,
					"status": status,
				}),
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

	if err := m.queries.InsertEpoch(ctx, db.InsertEpochParams{
		Number: int64(nextEpoch), NumStakers: int64(len(stakers)), Balance: int64(validator.Balance), Status: "in_progress",
	}); err != nil {
		return err
	}

	if m.broadcaster != nil {
		m.broadcaster.Publish(PoolEvent{
			Type:      "epoch_started",
			Timestamp: time.Now().UnixMilli(),
			Data: mustMarshal(map[string]any{
				"epoch":      nextEpoch,
				"height":     height,
				"numStakers": len(stakers),
				"balance":    int64(validator.Balance),
			}),
		})
	}

	for _, s := range stakers {
		if s.Balance == 0 {
			continue
		}
		if err := m.queries.InsertStaker(ctx, db.InsertStakerParams{
			EpochNumber: int64(nextEpoch),
			Address:     s.Address,
			Stake:       int64(s.Balance),
			Percentage:  stakerPercentage(s.Balance, validator.Balance),
		}); err != nil {
			return err
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
