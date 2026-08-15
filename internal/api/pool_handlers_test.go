package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHandlePoolRewardsReturnsAggregatedJSON(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	for _, reward := range []db.InsertRewardParams{
		{BatchNumber: 1, EpochNumber: 7, Amount: 240_00000, PoolFee: 10_00000, NumStakers: 2},
		{BatchNumber: 2, EpochNumber: 7, Amount: 350_00000, PoolFee: 15_00000, NumStakers: 2},
		{BatchNumber: 3, EpochNumber: 8, Amount: 480_00000, PoolFee: 20_00000, NumStakers: 3},
	} {
		if err := q.InsertReward(ctx, reward); err != nil {
			t.Fatalf("seeding reward: %v", err)
		}
	}

	a := &API{queries: q}
	req := httptest.NewRequest(http.MethodGet, "/api/pool/rewards", nil)
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("content type = %q, want application/json; body: %s", contentType, rec.Body.String())
	}
	type rewardPoint struct {
		EpochNumber int64 `json:"epoch_number"`
		TotalAmount int64 `json:"total_amount"`
		TotalFee    int64 `json:"total_fee"`
		Batches     int64 `json:"batches"`
	}
	var got []rewardPoint
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	want := []rewardPoint{
		{EpochNumber: 7, TotalAmount: 590_00000, TotalFee: 25_00000, Batches: 2},
		{EpochNumber: 8, TotalAmount: 480_00000, TotalFee: 20_00000, Batches: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("reward point %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
