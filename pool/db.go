package pool

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func InitDB(filepath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", filepath)
	if err != nil {
		return nil, err
	}

	// Initialize tables
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS epochs (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            epoch_number INTEGER UNIQUE NOT NULL,
            validator_balance BIGINT NOT NULL,
            processed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            paid_out INTEGER DEFAULT 0
        );

        CREATE TABLE IF NOT EXISTS staker_history (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            staker_address TEXT NOT NULL,
            epoch_id INTEGER NOT NULL,
            stake FLOAT NOT NULL,
            joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            left_at DATETIME,
            change_type TEXT NOT NULL,
            FOREIGN KEY (epoch_id) REFERENCES epochs(id)
        );

        CREATE TABLE IF NOT EXISTS payouts (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            epoch_id INTEGER NOT NULL,
            staker_address TEXT NOT NULL,
            payout_tx_hash TEXT,
            payout_completed INTEGER DEFAULT 0,
            payout_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (epoch_id) REFERENCES epochs(id),
            FOREIGN KEY (staker_address) REFERENCES staker_history(staker_address)
        );
        
        CREATE TABLE IF NOT EXISTS pool_payouts (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            epoch_id INTEGER NOT NULL,
            amount FLOAT NOT NULL,
            fee_percentage FLOAT NOT NULL,
            fee_amount FLOAT NOT NULL,
            fee_tx_hash TEXT,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (epoch_id) REFERENCES epochs(id)
        );
    `)

	if err != nil {
		return nil, err
	}

	return db, nil
}
