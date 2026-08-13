package api

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
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
	_, thisFile, _, _ := runtime.Caller(0)
	schemaPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "schema", "scheme.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("reading schema: %v", err)
	}

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if _, err := sqlDB.Exec(string(schema)); err != nil {
		t.Fatalf("applying schema: %v", err)
	}
	return db.New(sqlDB)
}
