package pool

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/models"
	"github.com/Beardsoft/GoPool/internal/rpc"

	_ "github.com/mattn/go-sqlite3" // Import the SQLite driver

	"go.uber.org/zap"
)

type PoolManager struct {
	queries         *db.Queries
	config          *config.Config
	db              *sql.DB // This field needs to be added if you intend to use it directly
	previousStakers map[string]float64
}

// Adjust your constructor to accept and store the *sql.DB:
func NewPoolManager(db *sql.DB, queries *db.Queries, config *config.Config) *PoolManager {
	return &PoolManager{
		db:      db,
		queries: queries,
		config:  config,
	}
}

func (pm *PoolManager) MainLoop() {
	logger.Logger.Info("Starting the MainLoop...")

	for {
		// Step 1: Fetch the current block number and determine if it's an election block or checkpoint block.
		currentBlock, err := rpc.GetCurrentBlockNumber(pm.config)
		if err != nil {
			logger.Logger.Error("Error fetching current block number", zap.Error(err))
			time.Sleep(1 * time.Minute)
			continue
		}

		policyConstants, err := rpc.GetPolicyConstants(pm.config)
		if err != nil {
			logger.Logger.Error("Error fetching policy constants", zap.Error(err))
			time.Sleep(1 * time.Minute)
			continue
		}

		blocksPerEpoch := policyConstants["blocksPerEpoch"].(float64)
		electionBlock := int64(blocksPerEpoch)

		// If election block, handle validator state for the new epoch
		if currentBlock%electionBlock == 0 {
			logger.Logger.Info("Election block detected", zap.Int64("block", currentBlock))
			err = pm.HandleElectionBlock(currentBlock)
			if err != nil {
				logger.Logger.Error("Error handling election block", zap.Error(err))
			}
		}

		// If checkpoint block, handle reward payout for the previous epoch
		if currentBlock%60 == 0 { // assuming checkpoint block occurs every 60 blocks
			logger.Logger.Info("Checkpoint block detected", zap.Int64("block", currentBlock))
			err = pm.HandleCheckpointBlock(currentBlock)
			if err != nil {
				logger.Logger.Error("Error handling checkpoint block", zap.Error(err))
			}
		}

		// Sleep a short time before checking again
		time.Sleep(10 * time.Second)
	}
}

func (pm *PoolManager) HandleElectionBlock(currentBlock int64) error {
	validator, err := rpc.GetValidatorByAddress(pm.config, pm.config.PoolAddress)
	if err != nil {
		return fmt.Errorf("error fetching validator state: %v", err)
	}

	// Fetch stakers if the validator is active
	if validator.NumStakers > 0 {
		stakers, err := rpc.GetStakersByValidatorAddress(pm.config, validator.Address)
		if err != nil {
			return fmt.Errorf("error fetching stakers: %v", err)
		}

		// Convert stakers from float64 to int64 if necessary
		stakersInt64 := make(map[string]int64)
		for address, balance := range stakers {
			stakersInt64[address] = int64(balance)
		}

		// Insert stakers using the converted map
		err = pm.InsertStakers(currentBlock, stakersInt64)
		if err != nil {
			return fmt.Errorf("error storing stakers: %v", err)
		}

		logger.Logger.Info("Stakers stored for new epoch", zap.Int("numStakers", int(validator.NumStakers)))
	}

	return nil
}

func (pm *PoolManager) HandleCheckpointBlock(currentBlock int64) error {
	inherents, err := rpc.GetInherentsByBlockNumber(pm.config, currentBlock-60)
	if err != nil {
		return fmt.Errorf("error fetching inherents for block %d: %v", currentBlock-60, err)
	}

	for _, inherent := range inherents {
		logger.Logger.Info("Processing reward",
			zap.String("validatorAddress", inherent.ValidatorAddress),
			zap.String("rewardAddress", inherent.Target),
			zap.Float64("rewardValue", float64(inherent.Value)))

		// Convert Inherent to Reward
		reward := models.InherentToReward(inherent)

		// Process payout with the converted Reward
		err = pm.PayoutRewards(reward, 0)
		if err != nil {
			logger.Logger.Error("Error processing reward payout", zap.Error(err))
		}
	}

	return nil
}

func (pm *PoolManager) PayoutRewards(reward models.Reward, epochID int64) error {
	// Retrieve stored stakers for the previous epoch
	stakers, err := pm.queries.GetStakersForEpoch(context.Background(), epochID)
	if err != nil {
		return fmt.Errorf("error retrieving stakers for payout: %v", err)
	}

	// Calculate total stake and payout per staker
	var totalStake int64
	stakerMap := make(map[string]int64)
	for _, staker := range stakers {
		totalStake += staker.Stake
		stakerMap[staker.StakerAddress] = staker.Stake
	}

	// Deduct the pool fee from the total reward
	poolFee := float64(reward.Value) * pm.config.PoolFeePercentage
	rewardsAfterFee := float64(reward.Value) - poolFee

	// Send pool fee to configured wallet
	err = rpc.SendPoolFee(pm.config, poolFee)
	if err != nil {
		return fmt.Errorf("error sending pool fee: %v", err)
	}

	// Pay each staker their share
	for stakerAddress, stake := range stakerMap {
		stakerReward := (float64(stake) / float64(totalStake)) * rewardsAfterFee
		err = rpc.PayOutStake(pm.config, stakerAddress, stakerReward)
		if err != nil {
			return fmt.Errorf("error paying staker: %v", err)
		}

		logger.Logger.Info("Staker payout successful", zap.String("staker", stakerAddress), zap.Float64("amount", stakerReward))
	}

	// Record pool payout in the database
	err = pm.RecordPoolPayout(epochID, float64(reward.Value), pm.config.PoolFeePercentage, poolFee, reward.Hash)
	if err != nil {
		return fmt.Errorf("error recording pool payout: %v", err)
	}

	return nil
}

func (pm *PoolManager) InsertPayout(epochID int64, stakerAddress, txHash string) error {
	err := pm.queries.InsertPayout(context.Background(), db.InsertPayoutParams{
		EpochID:       epochID,
		StakerAddress: stakerAddress,
		PayoutTxHash:  txHash,
	})
	return err
}

func (pm *PoolManager) RecordPoolPayout(epochID int64, totalAmount, feePercentage, feeAmount float64, feeTxHash string) error {
	err := pm.queries.InsertPoolPayout(context.Background(), db.InsertPoolPayoutParams{
		EpochID:       epochID,
		Amount:        totalAmount,
		FeePercentage: feePercentage,
		FeeAmount:     feeAmount,
		FeeTxHash:     feeTxHash,
	})
	if err != nil {
		return fmt.Errorf("error recording pool payout: %v", err)
	}

	logger.Logger.Info("Recorded pool payout", zap.Int64("epoch", epochID), zap.Float64("amount", totalAmount), zap.Float64("feeAmount", feeAmount), zap.Float64("feePercentage", feePercentage*100), zap.String("txHash", feeTxHash))
	return nil
}

func (pm *PoolManager) AreAllPayoutsCompleted(epochID int64) (bool, error) {
	count, err := pm.queries.CountIncompletePayouts(context.Background(), epochID)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

func (pm *PoolManager) MarkEpochAsPaid(epochID int64) error {
	err := pm.queries.MarkEpochAsPaid(context.Background(), epochID)
	return err
}

func (pm *PoolManager) InsertEpoch(epochNumber int64) (int64, error) {
	balance, err := rpc.GetValidatorBalance(pm.config, pm.config.PoolAddress)
	if err != nil {
		return 0, err
	}

	// InsertEpoch returns (int64, error)
	epochID, err := pm.queries.InsertEpoch(context.Background(), db.InsertEpochParams{
		EpochNumber:      epochNumber,
		ValidatorBalance: balance,
	})
	if err != nil {
		return 0, err
	}

	return epochID, nil
}

func (pm *PoolManager) StoreEpochAndBalanceAtStartup() error {
	// Get the current epoch number
	currentEpoch, err := rpc.GetEpochNumber(pm.config)
	if err != nil {
		return fmt.Errorf("error getting current epoch number: %v", err)
	}

	// Check if the epoch already exists
	_, err = pm.queries.GetEpochID(context.Background(), currentEpoch)
	if err == sql.ErrNoRows {
		// If the epoch doesn't exist, get the validator balance
		balance, err := rpc.GetValidatorBalance(pm.config, pm.config.PoolAddress)
		if err != nil {
			return fmt.Errorf("error getting validator balance: %v", err)
		}

		// Insert the new epoch and balance
		_, err = pm.queries.InsertEpoch(context.Background(), db.InsertEpochParams{
			EpochNumber:      currentEpoch, // Use currentEpoch here, not the erroneous "epoch"
			ValidatorBalance: balance,
		})
		if err != nil {
			return fmt.Errorf("error inserting epoch and balance: %v", err)
		}

		logger.Logger.Info("Stored epoch and balance at startup", zap.Int64("epoch", currentEpoch), zap.Int64("balance", balance))
	} else if err != nil {
		// If there's an error other than no rows found, return it
		return fmt.Errorf("error checking epoch existence: %v", err)
	} else {
		// Epoch already exists, so we skip the insertion
		logger.Logger.Info("Epoch is already stored. Skipping storage at startup", zap.Int64("epoch", currentEpoch))
	}

	return nil
}

func (pm *PoolManager) InsertStakers(epochID int64, stakers map[string]int64) error {
	// Start a new transaction using the *sql.DB instance from PoolManager
	tx, err := pm.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Use sqlc's WithTx method to create a transaction-specific Queries instance
	qtx := pm.queries.WithTx(tx)

	// Insert each staker
	for address, stake := range stakers {
		err := qtx.InsertStaker(context.Background(), db.InsertStakerParams{
			EpochID: epochID,
			Address: address,
			Stake:   stake, // Ensure Stake is an int64
		})
		if err != nil {
			return err
		}
	}

	// Commit the transaction
	return tx.Commit()
}

func (pm *PoolManager) UpdateEpochBalance(epochNumber int64) error {
	balance, err := rpc.GetValidatorBalance(pm.config, pm.config.PoolAddress)
	if err != nil {
		return fmt.Errorf("error getting validator balance: %v", err)
	}

	err = pm.queries.UpdateEpochBalance(context.Background(), db.UpdateEpochBalanceParams{
		ValidatorBalance: balance,
		EpochNumber:      epochNumber,
	})
	if err != nil {
		return fmt.Errorf("error updating epoch balance: %v", err)
	}

	logger.Logger.Info("Updated balance for epoch", zap.Int64("epoch", epochNumber), zap.Int64("balance", balance))
	return nil
}

func (pm *PoolManager) CalculateAndPayRewards(epochID int64, totalRewards float64) error {
	// Start a transaction
	tx, err := pm.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Create a new Queries instance that uses the transaction
	qtx := pm.queries.WithTx(tx)

	// Retrieve all stakers for the given epoch using sqlc-generated query
	stakers, err := qtx.GetStakersForEpoch(context.Background(), epochID)
	if err != nil {
		return err
	}

	// Calculate the total stake
	var totalStake int64
	stakerMap := make(map[string]int64)
	for _, staker := range stakers {
		totalStake += staker.Stake
		stakerMap[staker.StakerAddress] = staker.Stake
	}

	// Calculate the pool fee in floating-point terms
	poolFee := totalRewards * pm.config.PoolFeePercentage
	rewardsAfterFee := totalRewards - poolFee

	// Send the pool fee to the configured wallet
	err = rpc.SendPoolFee(pm.config, poolFee)
	if err != nil {
		return err
	}

	// Pay out each staker their portion of the rewards
	for address, stake := range stakerMap {
		reward := (float64(stake) / float64(totalStake)) * rewardsAfterFee
		err = rpc.PayOutStake(pm.config, address, reward)
		if err != nil {
			return err
		}
	}

	// Record the pool payout in the database
	err = qtx.InsertPoolPayout(context.Background(), db.InsertPoolPayoutParams{
		EpochID:       epochID,
		Amount:        totalRewards,
		FeePercentage: pm.config.PoolFeePercentage,
		FeeAmount:     poolFee,
		FeeTxHash:     "example_fee_tx_hash",
	})
	if err != nil {
		return err
	}

	// Commit the transaction
	return tx.Commit()
}

func (pm *PoolManager) GetEpochRewards(epochID int64) (float64, error) {
	currBalance, err := pm.queries.GetValidatorBalance(context.Background(), epochID)
	if err != nil {
		return 0, err
	}

	prevBalance, err := pm.queries.GetValidatorBalance(context.Background(), epochID-1)
	if err != nil {
		return 0, err
	}

	rewards := float64(currBalance - prevBalance)
	return rewards, nil
}

func (pm *PoolManager) CompareStakers(currentStakers map[string]float64) (map[string]float64, map[string]float64) {
	newStakers := make(map[string]float64)
	leftStakers := make(map[string]float64)

	for address, stake := range currentStakers {
		if _, exists := pm.previousStakers[address]; !exists {
			newStakers[address] = stake
		}
	}

	for address := range pm.previousStakers {
		if _, exists := currentStakers[address]; !exists {
			leftStakers[address] = pm.previousStakers[address]
		}
	}

	return newStakers, leftStakers
}

func EnsurePoolAddress(config *config.Config) (string, error) {
	logger.Logger.Info("Ensuring pool address is set up...")

	// Step 1: Import the private key
	poolAddress, err := rpc.ImportPrivateKey(config)
	if err != nil {
		return "", fmt.Errorf("failed to import private key: %w", err)
	}
	logger.Logger.Info("Pool Address", zap.String("address", poolAddress))

	// Step 2: Check if the account is imported
	isImported, err := rpc.IsAccountImported(config, poolAddress)
	if err != nil {
		return "", fmt.Errorf("failed to check if account is imported: %w", err)
	}

	if !isImported {
		return "", fmt.Errorf("account was not imported correctly")
	}
	logger.Logger.Info("Account is imported successfully.")

	// Step 3: Unlock the account
	err = rpc.UnlockAccount(config, poolAddress)
	if err != nil {
		return "", fmt.Errorf("failed to unlock account: %w", err)
	}
	logger.Logger.Info("Account unlocked successfully.")

	return poolAddress, nil
}

func CalculateTimeUntilNextEpoch(config *config.Config) (time.Duration, error) {
	// Fetch the policy constants
	policyConstants, err := rpc.GetPolicyConstants(config)
	if err != nil {
		return 0, err
	}

	blocksPerEpoch := policyConstants["blocksPerEpoch"].(float64)
	blockSeparationTime := policyConstants["blockSeparationTime"].(float64) / 1000 // Convert milliseconds to seconds

	// Fetch the current block number
	currentBlockNumber, err := rpc.GetCurrentBlockNumber(config)
	if err != nil {
		return 0, err
	}

	// Calculate how many blocks are left in the current epoch
	blocksRemaining := blocksPerEpoch - float64(currentBlockNumber%int64(blocksPerEpoch))

	// Calculate the time remaining until the next epoch
	timeUntilNextEpoch := blocksRemaining * blockSeparationTime

	if timeUntilNextEpoch < 0 {
		return 0, fmt.Errorf("time calculation error: time until next epoch is negative")
	}

	return time.Duration(timeUntilNextEpoch) * time.Second, nil
}

func Countdown(duration time.Duration) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	remaining := duration

	for remaining > 0 {
		select {
		case <-ticker.C:
			remaining -= 1 * time.Minute
			if remaining > 0 {
				logger.Logger.Info("Time until next epoch", zap.Duration("remaining", remaining))
			}
		case <-time.After(remaining):
			return
		}
	}
}
