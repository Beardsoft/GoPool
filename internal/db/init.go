package db

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // Import the SQLite driver
)

// InitDB initializes and returns a *sql.DB connection to an SQLite database.
func InitDB(dataSourceName string) (*sql.DB, error) {
	// Open the SQLite database file.
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	// Verify the connection is valid
	if err := db.Ping(); err != nil {
		return nil, err
	}

	// Run migrations
	if err := Migrate(db); err != nil {
		return nil, err
	}

	return db, nil
}
