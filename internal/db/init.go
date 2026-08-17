package db

import (
	"database/sql"
	"strings"

	_ "github.com/mattn/go-sqlite3" // Import the SQLite driver
)

// InitDB initializes and returns a *sql.DB connection to an SQLite database.
func InitDB(dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dsnWithPragmas(dataSourceName))
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

// dsnWithPragmas appends the pragmas every pooled connection needs. They go in
// the DSN (applied on each new connection) rather than a one-off Exec, which
// would only reach a single connection in the pool. WAL lets the daemon's
// frequent writes coexist with API reads on the same file; busy_timeout makes
// a blocked reader wait instead of erroring; foreign_keys enforces the schema
// (off by default in SQLite).
func dsnWithPragmas(dataSourceName string) string {
	sep := "?"
	if strings.Contains(dataSourceName, "?") {
		sep = "&"
	}
	return dataSourceName + sep + "_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000&_foreign_keys=1"
}
