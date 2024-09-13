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
	queries *db.Queries
	config  *config.Config
	db      *sql.DB // This field needs to be added if you intend to use it directly
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

	// Fetch policy constants from the database at startup
	policyConstants, err := pm.GetPolicyConstants()
	if err != nil {
		logger.Logger.Fatal("Error fetching policy constants", zap.Error(err))
		return
	}

	blocksPerEpoch := int64(policyConstants.BlocksPerEpoch)
	blocksPerBatch := int64(policyConstants.BlocksPerBatch)
	genesisBlockNumber := int64(policyConstants.GenesisBlockNumber)

	// Retrieve the last processed checkpoint from the database
	lastProcessedCheckpoint, err := pm.GetLastProcessedCheckpoint()
	if err != nil {
		logger.Logger.Error("Error fetching last processed checkpoint", zap.Error(err))
		lastProcessedCheckpoint = genesisBlockNumber - blocksPerBatch // Initialize to a block before genesis if not found
	}

	for {
		// Fetch the current block number
		currentBlock, err := rpc.GetCurrentBlockNumber(pm.config)
		if err != nil {
			logger.Logger.Error("Error fetching current block number", zap.Error(err))
			time.Sleep(1 * time.Minute)
			continue
		}

		// Calculate the next election block
		nextElectionBlock := genesisBlockNumber + (((currentBlock-genesisBlockNumber)/blocksPerEpoch)+1)*blocksPerEpoch

		// Calculate the last completed checkpoint block
		lastCompletedCheckpoint := genesisBlockNumber + ((currentBlock-genesisBlockNumber)/blocksPerBatch)*blocksPerBatch

		// Calculate the next checkpoint block
		nextCheckpointBlock := lastCompletedCheckpoint + blocksPerBatch

		logger.Logger.Info("Block status", zap.Int64("block", currentBlock), zap.Int64("checkpoint", nextCheckpointBlock), zap.Int64("election", nextElectionBlock))

		// Check if we're approaching an election block
		if currentBlock >= nextElectionBlock-5 && currentBlock < nextElectionBlock {
			logger.Logger.Info("Approaching election block", zap.Int64("block", nextElectionBlock))
			err = pm.PrepareForElectionBlock(nextElectionBlock)
			if err != nil {
				logger.Logger.Error("Error preparing for election block", zap.Error(err))
			}
		}

		// Check if we've completed a new checkpoint block
		if lastCompletedCheckpoint > lastProcessedCheckpoint {
			logger.Logger.Info("New checkpoint block completed", zap.Int64("block", lastCompletedCheckpoint))
			err = pm.HandleCheckpointBlock(lastCompletedCheckpoint)
			if err != nil {
				logger.Logger.Error("Error handling checkpoint block", zap.Error(err))
			} else {
				// Only update if handling was successful
				err = pm.UpdateLastProcessedCheckpoint(lastCompletedCheckpoint)
				if err != nil {
					logger.Logger.Error("Error updating last processed checkpoint", zap.Error(err))
				} else {
					lastProcessedCheckpoint = lastCompletedCheckpoint
				}
			}
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (pm *PoolManager) PrepareForElectionBlock(blockNumber int64) error {
	// Implement steps from section A of the design
	validator, err := rpc.GetValidatorByAddress(pm.config, pm.config.PoolAddress)
	if err != nil {
		return fmt.Errorf("failed to get validator: %w", err)
	}

	// Check validator status
	if validator == nil {
		logger.Logger.Warn("Not a validator")
		return nil
	}

	if validator.InactivityFlag != nil && *validator.InactivityFlag > blockNumber {
		logger.Logger.Info("Validator is inactive", zap.Int64("block", *validator.InactivityFlag))
		return nil
	}

	if validator.Retired {
		logger.Logger.Info("Validator is retired.")
		return nil
	}

	if validator.JailedFrom != nil && *validator.JailedFrom > blockNumber {
		logger.Logger.Info("Validator is jailed", zap.Int64("block", *validator.JailedFrom))
		return nil
	}

	// Handle stakers if any
	if validator.NumStakers > 0 {
		stakers, err := rpc.GetStakersByValidatorAddress(pm.config, pm.config.PoolAddress)
		if err != nil {
			return fmt.Errorf("failed to get stakers: %w", err)
		}

		totalStake := validator.Balance
		stakerPercentages := make(map[string]float64)

		for address, balance := range stakers {
			percentage := float64(balance) / float64(totalStake)
			stakerPercentages[address] = percentage
		}

		// Store stake percentages in the database
		epochID, err := rpc.GetEpochNumber(pm.config)
		if err != nil {
			return fmt.Errorf("failed to get epoch ID: %w", err)
		}

		for address, stake := range stakers {
			percentage := float64(stake) / float64(totalStake)
			err := pm.queries.InsertStakerWithPercentage(context.Background(), db.InsertStakerWithPercentageParams{
				EpochID:    epochID,
				Address:    address,
				Stake:      int64(stake),
				Percentage: percentage,
			})
			if err != nil {
				return fmt.Errorf("failed to store staker percentage: %w", err)
			}
		}

		logger.Logger.Info("Stored stake percentages for stakers",
			zap.Int("stakerCount", len(stakerPercentages)),
			zap.Int64("totalStake", totalStake))
	}

	return nil
}

func (pm *PoolManager) GetLastProcessedCheckpoint() (int64, error) {
	ctx := context.Background()
	policyConstants, err := pm.GetPolicyConstants()
	if err != nil {
		logger.Logger.Fatal("Error fetching policy constants", zap.Error(err))
	}
	checkpoint, err := pm.queries.GetLastProcessedCheckpoint(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			// If no checkpoint is found, return a default value
			return policyConstants.GenesisBlockNumber - policyConstants.BlocksPerBatch, nil
		}
		return 0, fmt.Errorf("error fetching last processed checkpoint: %w", err)
	}
	return checkpoint, nil
}

func (pm *PoolManager) UpdateLastProcessedCheckpoint(checkpoint int64) error {
	ctx := context.Background()
	err := pm.queries.UpdateLastProcessedCheckpoint(ctx, checkpoint)
	if err != nil {
		return fmt.Errorf("error updating last processed checkpoint: %w", err)
	}
	return nil
}

func (pm *PoolManager) GetPolicyConstants() (*models.PolicyConstants, error) {
	constants, err := pm.queries.GetLatestPolicyConstants(context.Background())
	if err != nil {
		return nil, err
	}

	return &models.PolicyConstants{
		StakingContractAddress:    constants.StakingContractAddress,
		CoinbaseAddress:           constants.CoinbaseAddress,
		TransactionValidityWindow: constants.TransactionValidityWindow,
		MaxSizeMicroBody:          constants.MaxSizeMicroBody,
		Version:                   constants.Version,
		Slots:                     constants.Slots,
		BlocksPerBatch:            constants.BlocksPerBatch,
		BatchesPerEpoch:           constants.BatchesPerEpoch,
		BlocksPerEpoch:            constants.BlocksPerEpoch,
		ValidatorDeposit:          constants.ValidatorDeposit,
		MinimumStake:              constants.MinimumStake,
		TotalSupply:               constants.TotalSupply,
		BlockSeparationTime:       constants.BlockSeparationTime,
		JailEpochs:                constants.JailEpochs,
		GenesisBlockNumber:        constants.GenesisBlockNumber,
	}, nil
}

func (pm *PoolManager) HandleElectionBlock(currentBlock int64) error {
	// Step 1: Fetch the validator state for the new epoch
	validator, err := rpc.GetValidatorByAddress(pm.config, pm.config.PoolAddress)
	if err != nil {
		return fmt.Errorf("error fetching validator state: %v", err)
	}

	// Step 2: Check if the validator is active
	if validator == nil {
		logger.Logger.Info("Validator not found, you are not a validator.")
		return nil
	}

	if validator.InactivityFlag != nil && *validator.InactivityFlag > currentBlock {
		logger.Logger.Info("Validator is inactive", zap.Int64("block", *validator.InactivityFlag))
		return nil
	}

	if validator.Retired {
		logger.Logger.Info("Validator is retired.")
		return nil
	}

	if validator.JailedFrom != nil && *validator.JailedFrom > currentBlock {
		logger.Logger.Info("Validator is jailed", zap.Int64("block", *validator.JailedFrom))
		return nil
	}

	// Step 3: Process stakers if the validator is active
	if validator.NumStakers > 0 {
		stakers, err := rpc.GetStakersByValidatorAddress(pm.config, validator.Address)
		if err != nil {
			return fmt.Errorf("error fetching stakers: %v", err)
		}

		totalStake := validator.Balance
		stakerMap := make(map[string]int64)

		for address, balance := range stakers {
			stakerMap[address] = int64(balance)
		}

		// Insert stakers with their calculated percentage
		err = pm.InsertStakersWithPercentage(currentBlock, stakerMap, totalStake)
		if err != nil {
			return fmt.Errorf("error storing stakers: %v", err)
		}

		logger.Logger.Info("Stakers stored for new epoch", zap.Int("numStakers", int(validator.NumStakers)))
	} else {
		logger.Logger.Info("No stakers found for the validator.")
	}

	return nil
}

func (pm *PoolManager) InsertStakersWithPercentage(epochID int64, stakers map[string]int64, totalStake int64) error {
	for address, stake := range stakers {
		percentage := float64(stake) / float64(totalStake)
		err := pm.queries.InsertStakerWithPercentage(context.Background(), db.InsertStakerWithPercentageParams{
			EpochID:    epochID,
			Address:    address,
			Stake:      stake,
			Percentage: percentage,
		})
		if err != nil {
			return err
		}
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

func StorePolicyConstants(config *config.Config, queries *db.Queries) error {
	// Check if constants already exist
	_, err := queries.GetLatestPolicyConstants(context.Background())
	if err == nil {
		// Constants already exist, no need to insert
		return nil
	}
	if err != sql.ErrNoRows {
		// An error occurred other than "no rows"
		return fmt.Errorf("error checking existing policy constants: %v", err)
	}

	// Fetch and insert new constants
	policyConstants, err := rpc.GetPolicyConstants(config)
	if err != nil {
		return fmt.Errorf("failed to fetch policy constants: %v", err)
	}

	err = queries.InsertPolicyConstants(context.Background(), db.InsertPolicyConstantsParams{
		StakingContractAddress:    policyConstants.StakingContractAddress,
		CoinbaseAddress:           policyConstants.CoinbaseAddress,
		TransactionValidityWindow: policyConstants.TransactionValidityWindow,
		MaxSizeMicroBody:          policyConstants.MaxSizeMicroBody,
		Version:                   policyConstants.Version,
		Slots:                     policyConstants.Slots,
		BlocksPerBatch:            policyConstants.BlocksPerBatch,
		BatchesPerEpoch:           policyConstants.BatchesPerEpoch,
		BlocksPerEpoch:            policyConstants.BlocksPerEpoch,
		ValidatorDeposit:          policyConstants.ValidatorDeposit,
		MinimumStake:              policyConstants.MinimumStake,
		TotalSupply:               policyConstants.TotalSupply,
		BlockSeparationTime:       policyConstants.BlockSeparationTime,
		JailEpochs:                policyConstants.JailEpochs,
		GenesisBlockNumber:        policyConstants.GenesisBlockNumber,
	})
	if err != nil {
		return fmt.Errorf("failed to insert policy constants: %v", err)
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
