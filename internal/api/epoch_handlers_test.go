package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Beardsoft/GoPool/internal/db"
)

func TestHandleListEpochs(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 1, NumStakers: 1, Balance: 10, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 2, NumStakers: 1, Balance: 20, Status: "in_progress"}); err != nil {
		t.Fatal(err)
	}

	a := &API{queries: q}
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/epochs", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got []epochResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Number != 2 { // ORDER BY number DESC
		t.Errorf("got %+v", got)
	}
}

func TestHandleGetEpoch(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 5, NumStakers: 1, Balance: 50, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertStaker(ctx, db.InsertStakerParams{EpochNumber: 5, Address: "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E", Stake: 50, Percentage: 100}); err != nil {
		t.Fatal(err)
	}

	a := &API{queries: q}

	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/epochs/5", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got epochDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Number != 5 || len(got.Stakers) != 1 {
		t.Errorf("got %+v", got)
	}

	rec2 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/epochs/999", nil))
	if rec2.Code != http.StatusNotFound {
		t.Errorf("unknown epoch: status = %d, want 404", rec2.Code)
	}
}
