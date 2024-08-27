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
            payout_tx_hash TEXT,
            paid_out INTEGER DEFAULT 0
        );
        CREATE TABLE IF NOT EXISTS stakers (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            epoch_id INTEGER NOT NULL,
            address TEXT NOT NULL,
            stake FLOAT NOT NULL,
            FOREIGN KEY (epoch_id) REFERENCES epochs(id)
        );
    `)
	if err != nil {
		return nil, err
	}

	return db, nil
}
