package pool

import (
	"context"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

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
	nextEpoch := m.policy.EpochAt(height) + 1

	exists, err := m.queries.EpochExists(ctx, int64(nextEpoch))
	if err != nil {
		return err
	}
	if exists != 0 {
		return nil
	}

	addr := m.chain.Address()
	validator, err := m.chain.RPC.GetValidator(ctx, addr)
	if err != nil {
		return m.queries.InsertEpoch(ctx, db.InsertEpochParams{
			Number: int64(nextEpoch), NumStakers: 0, Balance: 0, Status: "not_elected",
		})
	}

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
		return m.queries.InsertEpoch(ctx, db.InsertEpochParams{
			Number: int64(nextEpoch), NumStakers: 0, Balance: int64(validator.Balance), Status: status,
		})
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
	return nil
}
