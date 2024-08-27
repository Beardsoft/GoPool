package pool

import (
	"database/sql"
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
	for {
		currentEpoch, err := GetEpochNumber(pm.config)
		if err != nil {
			log.Printf("Error getting epoch number: %v", err)
			time.Sleep(1 * time.Minute)
			continue
		}

		log.Printf("Processing Epoch: %d", currentEpoch)

		// Check if this epoch has already been processed
		var epochID int64
		err = pm.db.QueryRow("SELECT id FROM epochs WHERE epoch_number = ?", currentEpoch).Scan(&epochID)
		if err == sql.ErrNoRows {
			// Insert the new epoch and store stakers
			epochID, err = pm.InsertEpoch(currentEpoch)
			if err != nil {
				log.Printf("Error inserting new epoch: %v", err)
				continue
			}

			stakers, err := GetStakersByValidatorAddress(pm.config, pm.config.PoolAddress)
			if err != nil {
				log.Printf("Error getting stakers: %v", err)
				continue
			}
			err = pm.InsertStakers(epochID, stakers)
			if err != nil {
				log.Printf("Error inserting stakers: %v", err)
				continue
			}

			newStakers, leftStakers := pm.CompareStakers(stakers)
			log.Printf("New Stakers: %v", newStakers)
			log.Printf("Left Stakers: %v", leftStakers)

			// Perform payout operations and record the tx hash
			txHash := "example_tx_hash" // Replace with actual tx hash from PerformPayout
			err = pm.MarkEpochAsPaid(currentEpoch, txHash)
			if err != nil {
				log.Printf("Error marking epoch as paid: %v", err)
			}

			pm.previousStakers = stakers

		} else if err != nil {
			log.Printf("Error checking epoch: %v", err)
		} else {
			log.Printf("Epoch %d already processed.", currentEpoch)
		}

		// Calculate time until the next epoch
		sleepDuration, err := CalculateTimeUntilNextEpoch(pm.config)
		if err != nil {
			log.Printf("Error calculating time until next epoch: %v", err)
			time.Sleep(1 * time.Minute)
			continue
		}

		// Log and count down to the next epoch
		log.Printf("Sleeping until next epoch: %v", sleepDuration)
		Countdown(sleepDuration)
	}
}

func (pm *PoolManager) InsertEpoch(epochNumber int64) (int64, error) {
	result, err := pm.db.Exec("INSERT INTO epochs (epoch_number) VALUES (?)", epochNumber)
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

func (pm *PoolManager) MarkEpochAsPaid(epochNumber int64, txHash string) error {
	_, err := pm.db.Exec("UPDATE epochs SET paid_out = 1, payout_tx_hash = ? WHERE epoch_number = ?", txHash, epochNumber)
	return err
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
