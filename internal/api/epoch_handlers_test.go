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

func TestHandleGetEpochRewardsReturnsRewardBatchesAsJSON(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	for _, reward := range []db.InsertRewardParams{
		{BatchNumber: 4, EpochNumber: 12, Amount: 490_00000, PoolFee: 10_00000, NumStakers: 2},
		{BatchNumber: 5, EpochNumber: 12, Amount: 735_00000, PoolFee: 15_00000, NumStakers: 3},
	} {
		if err := q.InsertReward(ctx, reward); err != nil {
			t.Fatalf("seeding reward: %v", err)
		}
	}

	a := &API{queries: q}
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/epochs/12/rewards", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("content type = %q, want application/json; body: %s", contentType, rec.Body.String())
	}
	type rewardBatch struct {
		BatchNumber int64 `json:"batch_number"`
		EpochNumber int64 `json:"epoch_number"`
		AmountLuna  int64 `json:"amount_luna"`
		PoolFeeLuna int64 `json:"pool_fee_luna"`
		NumStakers  int64 `json:"num_stakers"`
	}
	var got []rewardBatch
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	want := []rewardBatch{
		{BatchNumber: 4, EpochNumber: 12, AmountLuna: 490_00000, PoolFeeLuna: 10_00000, NumStakers: 2},
		{BatchNumber: 5, EpochNumber: 12, AmountLuna: 735_00000, PoolFeeLuna: 15_00000, NumStakers: 3},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("reward batch %d = %+v, want %+v", i, got[i], want[i])
		}
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
