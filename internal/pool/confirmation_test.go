package pool

import (
	"context"
	"database/sql"
	"testing"

	"github.com/NimMiniApps/nimiq-go/rpc"
	_ "github.com/mattn/go-sqlite3"

	"github.com/Beardsoft/GoPool/internal/db"
)

func TestConfirmationOutcome(t *testing.T) {
	cases := []struct {
		name string
		conf *rpc.Confirmation
		want confirmOutcome
	}{
		{"not yet final", &rpc.Confirmation{Included: true, Final: false, Succeeded: true}, outcomePending},
		{"final and succeeded", &rpc.Confirmation{Included: true, Final: true, Succeeded: true}, outcomeSucceeded},
		{"final but failed", &rpc.Confirmation{Included: true, Final: true, Succeeded: false}, outcomeFailed},
	}
	for _, c := range cases {
		if got := confirmationOutcome(c.conf); got != c.want {
			t.Errorf("%s: confirmationOutcome = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLegacyRequestedValidatorActionCanBeClaimed(t *testing.T) {
	q := newLifecycleTestQueries(t)
	if err := q.InsertValidatorAction(t.Context(), db.InsertValidatorActionParams{Action: "retire", Outcome: "requested"}); err != nil {
		t.Fatal(err)
	}
	requested, err := q.GetRequestedValidatorActions(t.Context())
	if err != nil || len(requested) != 1 {
		t.Fatalf("requested = %+v, err = %v", requested, err)
	}
	if _, err := q.MarkValidatorActionProcessing(t.Context(), requested[0].ID); err != nil {
		t.Fatalf("claim legacy requested action: %v", err)
	}
	stored, err := q.GetValidatorAction(t.Context(), requested[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.State.Valid || stored.State.String != "processing" {
		t.Fatalf("state = %+v, want processing", stored.State)
	}
}

func TestValidatorActionTransitionsAndLegacySubmittedReconciliation(t *testing.T) {
	q := newLifecycleTestQueries(t)

	requestedID, err := q.InsertValidatorActionWithState(t.Context(), db.InsertValidatorActionWithStateParams{
		Action: "retire", State: sql.NullString{String: "requested", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.MarkValidatorActionProcessing(t.Context(), requestedID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SubmitValidatorAction(t.Context(), db.SubmitValidatorActionParams{
		ID: requestedID, TxHash: sql.NullString{String: "new-submitted", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	if err := q.InsertValidatorAction(t.Context(), db.InsertValidatorActionParams{
		Action: "deactivate", TxHash: sql.NullString{String: "legacy-submitted", Valid: true}, Outcome: "pending",
	}); err != nil {
		t.Fatal(err)
	}

	submitted, err := q.ListValidatorActions(t.Context(), db.ListValidatorActionsParams{
		Status: sql.NullString{String: "submitted", Valid: true},
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(submitted) != 2 {
		t.Fatalf("submitted = %+v, want both legacy and current rows", submitted)
	}
	for _, action := range submitted {
		if !action.State.Valid || action.State.String != "submitted" {
			t.Fatalf("exposed state = %+v, want submitted", action.State)
		}
		terminalState := "confirmed"
		if action.TxHash.String == "legacy-submitted" {
			terminalState = "failed"
		}
		if _, err := q.CompleteSubmittedValidatorAction(t.Context(), db.CompleteSubmittedValidatorActionParams{
			ID: action.ID, State: sql.NullString{String: terminalState, Valid: true},
		}); err != nil {
			t.Fatalf("complete action %d: %v", action.ID, err)
		}
		stored, err := q.GetValidatorAction(t.Context(), action.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !stored.State.Valid || stored.State.String != terminalState {
			t.Fatalf("terminal state = %+v, want %s", stored.State, terminalState)
		}
	}
}

func TestCancelledContextReleasesClaimedValidatorAction(t *testing.T) {
	q := newLifecycleTestQueries(t)
	id, err := q.InsertValidatorActionWithState(t.Context(), db.InsertValidatorActionWithStateParams{
		Action: "retire", State: sql.NullString{String: "requested", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.MarkValidatorActionProcessing(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	m := &Manager{queries: q}
	if err := m.recoverClaimedValidatorAction(ctx, id); err != nil {
		t.Fatal(err)
	}
	stored, err := q.GetValidatorAction(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.State.Valid || stored.State.String != "requested" {
		t.Fatalf("state = %+v, want requested", stored.State)
	}
}

func newLifecycleTestQueries(t *testing.T) *db.Queries {
	t.Helper()
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Migrate(sqlDB); err != nil {
		t.Fatal(err)
	}
	return db.New(sqlDB)
}
