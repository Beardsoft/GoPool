package db

import (
	"database/sql"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3" // Import the SQLite driver
)

// InitDB initializes and returns a *sql.DB connection to an SQLite database.
func InitDB(dataSourceName string) (*sql.DB, error) {

	schemaFilePath := "schema/scheme.sql"

	// Open the SQLite database file.
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	// Verify the connection is valid
	if err := db.Ping(); err != nil {
		return nil, err
	}

	// Execute the schema SQL to create tables if they don't exist
	if err := executeSchema(db, schemaFilePath); err != nil {
		return nil, err
	}

	return db, nil
}

// executeSchema reads and executes SQL schema from a file
func executeSchema(db *sql.DB, schemaFilePath string) error {
	schema, err := os.ReadFile(schemaFilePath)
	if err != nil {
		return err
	}

	// Split the schema into individual SQL statements
	statements := strings.Split(string(schema), ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			_, err := db.Exec(stmt)
			if err != nil {
				log.Printf("Error executing statement: %s\nError: %v", stmt, err)
				return err
			}
		}
	}

	return nil
}
