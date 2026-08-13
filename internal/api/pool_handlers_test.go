package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Beardsoft/GoPool/internal/db"
)

func TestHandlePool(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 3, NumStakers: 2, Balance: 100_000_00000, Status: "in_progress"}); err != nil {
		t.Fatalf("seeding epoch: %v", err)
	}
	if err := q.InsertReward(ctx, db.InsertRewardParams{BatchNumber: 1, EpochNumber: 2, Amount: 990_00000, PoolFee: 10_00000, NumStakers: 2}); err != nil {
		t.Fatalf("seeding reward: %v", err)
	}

	a := &API{queries: q}
	req := httptest.NewRequest(http.MethodGet, "/api/pool", nil)
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	var got poolResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.CurrentEpoch != 3 || got.NumStakers != 2 || got.TotalStakeLuna != 100_000_00000 || got.TotalRewardsLuna != 990_00000 {
		t.Errorf("got %+v", got)
	}
}
