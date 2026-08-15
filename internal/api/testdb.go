package api

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3" // SQLite driver for in-memory handler tests

	"github.com/Beardsoft/GoPool/internal/db"
)

// newTestDB opens an in-memory sqlite database and applies the project
// schema, for handler tests. It locates schema/scheme.sql relative to this
// source file (not the process CWD), so it works no matter which package
// directory `go test` runs from.
func newTestDB(t *testing.T) *db.Queries {
	t.Helper()
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := db.Migrate(sqlDB); err != nil {
		t.Fatalf("migrating test db: %v", err)
	}
	return db.New(sqlDB)
}
