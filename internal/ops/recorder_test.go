package ops

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/Beardsoft/GoPool/internal/db"
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

func testQueries(t *testing.T) *db.Queries {
	t.Helper()
	sqlDB := openMemoryDB(t)
	if err := db.Migrate(sqlDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db.New(sqlDB)
}

func listEvents(t *testing.T, q *db.Queries) []db.OperatorEvent {
	t.Helper()
	rows, err := q.ListOperatorEvents(context.Background(), db.ListOperatorEventsParams{
		Limit:  100,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	return rows
}

func TestRecorderRedactsSecretContext(t *testing.T) {
	q := testQueries(t)
	r := NewRecorder(q, "")
	err := r.RecordEvent(context.Background(), EventInput{
		Severity: "warning",
		Category: "payout",
		Source:   "daemon",
		Type:     "payout_failed",
		Summary:  "Payout submission failed",
		Context:  map[string]any{"address": "NQ…", "private_key": "must-not-store"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := listEvents(t, q)
	if len(got) == 0 {
		t.Fatal("no events recorded")
	}
	ctxJSON := ""
	if got[0].ContextJson.Valid {
		ctxJSON = got[0].ContextJson.String
	}
	if strings.Contains(ctxJSON, "must-not-store") {
		t.Fatal("secret persisted")
	}
	if !strings.Contains(ctxJSON, "[redacted]") {
		t.Fatal("redaction missing")
	}
}
