package db

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openMemoryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("pragma foreign_keys: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func applyLegacySchema(t *testing.T, db *sql.DB) {
	t.Helper()
	// Load legacy schema from migration 001_initial.sql
	data, err := os.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatalf("read legacy schema: %v", err)
	}
	execSQL(t, db, string(data))
}

func assertTableExists(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var count int
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`
	if err := db.QueryRow(query, name).Scan(&count); err != nil {
		t.Fatalf("query table %s: %v", name, err)
	}
	if count != 1 {
		t.Fatalf("table %s does not exist", name)
	}
}

func execSQL(t *testing.T, db *sql.DB, sqlText string) {
	t.Helper()
	stmts := strings.Split(sqlText, ";")
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec sql %q: %v", stmt, err)
		}
	}
}

func TestMigrateFreshAndLegacyDatabases(t *testing.T) {
	for _, tc := range []struct {
		name   string
		legacy bool
	}{
		{"fresh", false},
		{"legacy", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openMemoryDB(t)
			if tc.legacy {
				applyLegacySchema(t, db)
			}
			if err := Migrate(db); err != nil {
				t.Fatal(err)
			}
			for _, table := range []string{"runtime_status", "health_snapshots", "operator_events", "alert_deliveries", "config_revisions"} {
				assertTableExists(t, db, table)
			}
		})
	}
}
