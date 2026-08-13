package pool

import (
	"context"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/metrics"

	"go.uber.org/zap"
)

// splitReward divides a reward into the staker share and the pool fee, in
// integer Luna. feePercentage is converted to basis points (x10000) so the
// division happens on integers, never on floats — the fix for the precision
// bug in the amount predating this rewrite.
func splitReward(reward nimiq.Luna, feePercentage float64) (afterFee, fee nimiq.Luna) {
	basisPoints := uint64(feePercentage * 10000)
	fee = nimiq.Luna(uint64(reward) * basisPoints / 10000)
	return reward - fee, fee
}

// handleCheckpoint runs at every checkpoint (batch boundary): it fetches the
// reward inherent for the batch that just closed one batch size back (reward
// inherents for batch N land in batch N+1, per Nimiq's Albatross reward
// timing), splits it, and fans out a payslip per staker using the epoch's
// stored percentages.
func (m *Manager) handleCheckpoint(ctx context.Context, height uint32) error {
	currentBatch := batchAt(m.policy, height)
	if currentBatch == 0 {
		return nil
	}
	rewardBatch := currentBatch - 1

	exists, err := m.queries.RewardExists(ctx, int64(rewardBatch))
	if err != nil {
		return err
	}
	if exists != 0 {
		return nil
	}

	// Reward inherents for the previous batch are in this checkpoint block,
	// not in the previous batch's macro (which is empty — verified on devlab).
	inherents, err := m.chain.RPC.GetInherentsByBlockNumber(ctx, height)
	if err != nil {
		return err
	}

	addr := m.chain.Address().String()
	var reward nimiq.Luna
	for _, in := range inherents {
		if in.IsReward() && in.ValidatorAddress == addr {
			reward += in.Value
		}
	}

	epoch := epochAt(m.policy, height-m.policy.BlocksPerBatch)
	stakers, err := m.queries.GetStakersForEpoch(ctx, int64(epoch))
	if err != nil {
		return err
	}

	afterFee, fee := splitReward(reward, m.cfg.PoolFeePercentage)
	if err := m.queries.InsertReward(ctx, db.InsertRewardParams{
		BatchNumber: int64(rewardBatch), EpochNumber: int64(epoch),
		Amount: int64(afterFee), PoolFee: int64(fee), NumStakers: int64(len(stakers)),
	}); err != nil {
		return err
	}
	metrics.RewardsTotal.Add(float64(reward))
	metrics.PoolFeeTotal.Add(float64(fee))

	if reward == 0 || len(stakers) == 0 {
		return nil
	}

	var totalStake int64
	for _, s := range stakers {
		totalStake += s.Stake
	}
	if totalStake == 0 {
		return nil
	}

	for _, s := range stakers {
		// Integer stake ratio, not the stored percentage float: this is the
		// actual payout amount, so it must not go through a float division.
		share := nimiq.Luna(uint64(afterFee) * uint64(s.Stake) / uint64(totalStake))
		if share == 0 {
			continue
		}
		if err := m.queries.InsertPayslip(ctx, db.InsertPayslipParams{
			BatchNumber: int64(rewardBatch), Address: s.Address, Amount: int64(share), Status: "pending",
		}); err != nil {
			return err
		}
	}

	logger.Logger.Info("reward collected",
		zap.Int64("batch", int64(rewardBatch)), zap.Uint64("reward", uint64(reward)), zap.Uint64("fee", uint64(fee)))
	return nil
}
