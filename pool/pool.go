package pool

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type PoolManager struct {
	db              *sql.DB
	config          *Config
	previousStakers map[string]float64 // Keyed by staker address
}

func NewPoolManager(db *sql.DB, config *Config) *PoolManager {
	return &PoolManager{
		db:              db,
		config:          config,
		previousStakers: make(map[string]float64),
	}
}

func (pm *PoolManager) ProcessEpochs() {
	// Store the current epoch and balance at startup
	err := pm.StoreEpochAndBalanceAtStartup()
	if err != nil {
		log.Fatalf("Failed to store epoch and balance at startup: %v", err)
	}

	for {
		currentEpoch, err := GetEpochNumber(pm.config)
		if err != nil {
			log.Printf("Error getting epoch number: %v", err)
			time.Sleep(1 * time.Minute)
			continue
		}

		previousEpoch := currentEpoch - 1
		log.Printf("Processing Epoch: %d", previousEpoch)

		var epochID int64
		err = pm.db.QueryRow("SELECT id FROM epochs WHERE epoch_number = ?", previousEpoch).Scan(&epochID)
		if err == sql.ErrNoRows {
			log.Printf("Epoch %d has not been processed yet", previousEpoch)
			time.Sleep(1 * time.Minute)
			continue
		}

		rewards, err := pm.GetEpochRewards(epochID)
		if err != nil {
			log.Printf("Error getting rewards for epoch %d: %v", previousEpoch, err)
			time.Sleep(1 * time.Minute)
			continue
		}

		if rewards > 0 {
			err = pm.CalculateAndPayRewards(epochID, rewards)
			if err != nil {
				log.Printf("Error paying rewards for epoch %d: %v", previousEpoch, err)
				time.Sleep(1 * time.Minute)
				continue
			}

			err = pm.MarkEpochAsPaid(epochID)
			if err != nil {
				log.Printf("Error marking epoch %d as paid: %v", previousEpoch, err)
			}

			// Update the balance for the current epoch
			err = pm.UpdateEpochBalance(previousEpoch)
			if err != nil {
				log.Printf("Error updating balance for epoch %d: %v", previousEpoch, err)
			}
		} else {
			log.Printf("No rewards received for epoch %d. Skipping payout.", previousEpoch)
		}

		// Sleep until the next epoch
		sleepDuration, err := CalculateTimeUntilNextEpoch(pm.config)
		if err != nil {
			log.Printf("Error calculating time until next epoch: %v", err)
			time.Sleep(1 * time.Minute)
			continue
		}

		log.Printf("Sleeping until next epoch: %v", sleepDuration)
		Countdown(sleepDuration)
	}
}

func (pm *PoolManager) InsertPayout(epochID int64, stakerAddress, txHash string) error {
	_, err := pm.db.Exec(`
        INSERT INTO payouts (epoch_id, staker_address, payout_tx_hash, payout_completed) 
        VALUES (?, ?, ?, 1)
    `, epochID, stakerAddress, txHash)
	return err
}

func (pm *PoolManager) RecordPoolPayout(epochID int64, totalAmount, feePercentage, feeAmount float64, feeTxHash string) error {
	_, err := pm.db.Exec(`
        INSERT INTO pool_payouts (epoch_id, amount, fee_percentage, fee_amount, fee_tx_hash) 
        VALUES (?, ?, ?, ?, ?)
    `, epochID, totalAmount, feePercentage, feeAmount, feeTxHash)
	if err != nil {
		return fmt.Errorf("error recording pool payout: %v", err)
	}

	log.Printf("Recorded pool payout for epoch %d: amount=%.4f, fee=%.4f (%.2f%%), tx_hash=%s",
		epochID, totalAmount, feeAmount, feePercentage*100, feeTxHash)
	return nil
}

func (pm *PoolManager) AreAllPayoutsCompleted(epochID int64) (bool, error) {
	var count int
	err := pm.db.QueryRow(`
        SELECT COUNT(*) FROM payouts 
        WHERE epoch_id = ? AND payout_completed = 0
    `, epochID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

func (pm *PoolManager) MarkEpochAsPaid(epochID int64) error {
	_, err := pm.db.Exec("UPDATE epochs SET paid_out = 1 WHERE id = ?", epochID)
	return err
}

func (pm *PoolManager) InsertEpoch(epochNumber int64) (int64, error) {
	balance, err := GetValidatorBalance(pm.config, pm.config.PoolAddress)
	if err != nil {
		return 0, err
	}

	result, err := pm.db.Exec(`
        INSERT INTO epochs (epoch_number, validator_balance) 
        VALUES (?, ?)
    `, epochNumber, balance)
	if err != nil {
		return 0, err
	}
	epochID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return epochID, nil
}

func (pm *PoolManager) InsertStakers(epochID int64, stakers map[string]float64) error {
	tx, err := pm.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO stakers (epoch_id, address, stake) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for address, stake := range stakers {
		_, err := stmt.Exec(epochID, address, stake)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (pm *PoolManager) StoreEpochAndBalanceAtStartup() error {
	currentEpoch, err := GetEpochNumber(pm.config)
	if err != nil {
		return fmt.Errorf("error getting current epoch number: %v", err)
	}

	// Check if the current epoch is already stored
	var epochID int64
	err = pm.db.QueryRow("SELECT id FROM epochs WHERE epoch_number = ?", currentEpoch).Scan(&epochID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error checking epoch existence: %v", err)
	}

	if err == sql.ErrNoRows {
		// If the epoch is not stored, fetch the current balance and store it
		balance, err := GetValidatorBalance(pm.config, pm.config.PoolAddress)
		if err != nil {
			return fmt.Errorf("error getting validator balance: %v", err)
		}

		_, err = pm.db.Exec(`
            INSERT INTO epochs (epoch_number, validator_balance) 
            VALUES (?, ?)
        `, currentEpoch, balance)
		if err != nil {
			return fmt.Errorf("error inserting epoch and balance: %v", err)
		}

		log.Printf("Stored epoch %d and balance %d at startup.", currentEpoch, balance)
	} else {
		log.Printf("Epoch %d is already stored. Skipping storage at startup.", currentEpoch)
	}

	return nil
}

func (pm *PoolManager) UpdateEpochBalance(epochNumber int64) error {
	balance, err := GetValidatorBalance(pm.config, pm.config.PoolAddress)
	if err != nil {
		return fmt.Errorf("error getting validator balance: %v", err)
	}

	_, err = pm.db.Exec(`
        UPDATE epochs 
        SET validator_balance = ? 
        WHERE epoch_number = ?
    `, balance, epochNumber)
	if err != nil {
		return fmt.Errorf("error updating epoch balance: %v", err)
	}

	log.Printf("Updated balance for epoch %d to %d.", epochNumber, balance)
	return nil
}

func (pm *PoolManager) CalculateAndPayRewards(epochID int64, totalRewards float64) error {
	// Retrieve all stakers for the given epoch
	rows, err := pm.db.Query("SELECT staker_address, stake FROM staker_history WHERE epoch_id = ?", epochID)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Calculate the total stake
	var totalStake float64
	stakerMap := make(map[string]float64)
	for rows.Next() {
		var address string
		var stake float64
		if err := rows.Scan(&address, &stake); err != nil {
			return err
		}
		totalStake += stake
		stakerMap[address] = stake
	}

	// Calculate the pool fee
	poolFee := totalRewards * pm.config.PoolFeePercentage
	rewardsAfterFee := totalRewards - poolFee

	// Send the pool fee to the configured wallet
	err = pm.SendPoolFee(poolFee)
	if err != nil {
		return err
	}

	// Pay out each staker their portion of the rewards
	for address, stake := range stakerMap {
		reward := (stake / totalStake) * rewardsAfterFee
		err = pm.PayOutStake(address, reward)
		if err != nil {
			return err
		}
	}

	// Record the pool payout in the database
	err = pm.RecordPoolPayout(epochID, totalRewards, pm.config.PoolFeePercentage, poolFee, "example_fee_tx_hash")
	if err != nil {
		return err
	}

	return nil
}

func (pm *PoolManager) SendPoolFee(amount float64) error {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "sendBasicTransaction",
		"params": []interface{}{
			pm.config.PoolAddress,   // Sender address
			pm.config.PoolFeeWallet, // Receiver address (pool fee wallet)
			amount,                  // Amount in Luna
			0,                       // Fee in Luna (can be 0)
			"+0",                    // Validity start height
		},
	}

	response, err := sendRPCRequest(pm.config, payload)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	if err != nil {
		return err
	}

	if txHash, ok := result["result"].(string); ok {
		log.Printf("Sent pool fee of %.4f to %s. Transaction hash: %s", amount, pm.config.PoolFeeWallet, txHash)
		return nil
	}

	return fmt.Errorf("unexpected response structure: %v", result)
}

func (pm *PoolManager) PayOutStake(stakerAddress string, amount float64) error {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "sendStakeTransaction",
		"params": []interface{}{
			pm.config.PoolAddress, // Sender address (pool address)
			stakerAddress,         // Staker address
			amount,                // Amount in Luna
			0,                     // Fee in Luna (can be 0)
			"+0",                  // Validity start height
		},
	}

	response, err := sendRPCRequest(pm.config, payload)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	err = json.Unmarshal(response, &result)
	if err != nil {
		return err
	}

	if txHash, ok := result["result"].(string); ok {
		log.Printf("Paid out %.4f to %s. Transaction hash: %s", amount, stakerAddress, txHash)
		return nil
	}

	return fmt.Errorf("unexpected response structure: %v", result)
}

func (pm *PoolManager) InsertStakerHistory(epochID int64, stakers map[string]float64, changeType string) error {
	tx, err := pm.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
        INSERT INTO staker_history (staker_address, epoch_id, stake, change_type) 
        VALUES (?, ?, ?, ?)
    `)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for address, stake := range stakers {
		_, err := stmt.Exec(address, epochID, stake, changeType)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (pm *PoolManager) GetEpochRewards(epochID int64) (float64, error) {
	var prevEpochBalance, currEpochBalance int64

	err := pm.db.QueryRow(`
        SELECT validator_balance 
        FROM epochs 
        WHERE id = ?
    `, epochID).Scan(&currEpochBalance)
	if err != nil {
		return 0, err
	}

	err = pm.db.QueryRow(`
        SELECT validator_balance 
        FROM epochs 
        WHERE id = ?
    `, epochID-1).Scan(&prevEpochBalance)
	if err != nil {
		return 0, err
	}

	rewards := float64(currEpochBalance - prevEpochBalance)
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

func EnsurePoolAddress(config *Config) (string, error) {
	log.Println("Ensuring pool address is set up...")

	// Step 1: Import the private key
	poolAddress, err := ImportPrivateKey(config)
	if err != nil {
		return "", fmt.Errorf("failed to import private key: %w", err)
	}
	log.Printf("Pool Address: %s", poolAddress)

	// Step 2: Check if the account is imported
	isImported, err := IsAccountImported(config, poolAddress)
	if err != nil {
		return "", fmt.Errorf("failed to check if account is imported: %w", err)
	}

	if !isImported {
		return "", fmt.Errorf("account was not imported correctly")
	}
	log.Println("Account is imported successfully.")

	// Step 3: Unlock the account
	err = UnlockAccount(config, poolAddress)
	if err != nil {
		return "", fmt.Errorf("failed to unlock account: %w", err)
	}
	log.Println("Account unlocked successfully.")

	return poolAddress, nil
}

func CalculateTimeUntilNextEpoch(config *Config) (time.Duration, error) {
	// Fetch the policy constants
	policyConstants, err := GetPolicyConstants(config)
	if err != nil {
		return 0, err
	}

	blocksPerEpoch := policyConstants["blocksPerEpoch"].(float64)
	blockSeparationTime := policyConstants["blockSeparationTime"].(float64) / 1000 // Convert milliseconds to seconds

	// Fetch the current block number
	currentBlockNumber, err := GetCurrentBlockNumber(config)
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
				log.Printf("Time until next epoch: %v", remaining)
			}
		case <-time.After(remaining):
			return
		}
	}
}
