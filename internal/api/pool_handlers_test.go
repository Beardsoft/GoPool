package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/config"
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

func TestHandlePoolIncludesPublicSocials(t *testing.T) {
	q := newTestDB(t)
	a := &API{queries: q, cfg: &config.Config{
		PoolName:          "GoPool",
		ContactURL:        "https://github.com/Beardsoft/GoPool",
		TelegramURL:       "https://t.me/gopool",
		DiscordURL:        "https://discord.gg/gopool",
		XURL:              "https://x.com/gopool",
		Network:           "test-albatross",
		PoolFeePercentage: 0.01,
	}}
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
	if got.ContactURL != "https://github.com/Beardsoft/GoPool" || got.TelegramURL != "https://t.me/gopool" || got.DiscordURL != "https://discord.gg/gopool" || got.XURL != "https://x.com/gopool" {
		t.Errorf("public profile = %+v", got)
	}
}

func TestHandlePoolIncludesEpochClockFromPolicy(t *testing.T) {
	q := newTestDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		var result any
		switch req.Method {
		case "getBlockNumber":
			result = 1241
		case "getPolicyConstants":
			result = map[string]any{
				"blocksPerBatch": 60, "batchesPerEpoch": 4, "blocksPerEpoch": 240,
				"blockSeparationTime": 1000, "genesisBlockNumber": 1000,
			}
		default:
			result = nil
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	t.Cleanup(srv.Close)
	client, err := rpc.New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	a := &API{queries: q, rpc: client}
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
	if got.EpochClock == nil {
		t.Fatalf("epoch_clock missing: %s", rec.Body.String())
	}
	clock := got.EpochClock
	if clock.Epoch != 2 || clock.Head != 1241 || clock.BlocksPerEpoch != 240 || clock.BlockSeparationMs != 1000 || clock.RemainingBlocks != 239 || clock.RemainingMs != 239_000 {
		t.Errorf("epoch_clock = %+v", clock)
	}
}

func TestHandlePoolEmpty(t *testing.T) {
	q := newTestDB(t)
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
	if got.CurrentEpoch != 0 || got.EpochStatus != "pending" || got.TotalRewardsLuna != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestHandlePoolRewardsReturnsAggregatedJSON(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	for _, epoch := range []db.InsertEpochParams{
		{Number: 7, NumStakers: 2, Balance: 10_000_00000, Status: "completed"},
		{Number: 8, NumStakers: 3, Balance: 12_000_00000, Status: "in_progress"},
	} {
		if err := q.InsertEpoch(ctx, epoch); err != nil {
			t.Fatalf("seeding epoch: %v", err)
		}
	}
	for _, reward := range []db.InsertRewardParams{
		{BatchNumber: 1, EpochNumber: 7, Amount: 240_00000, PoolFee: 10_00000, NumStakers: 2},
		{BatchNumber: 2, EpochNumber: 7, Amount: 350_00000, PoolFee: 15_00000, NumStakers: 2},
		{BatchNumber: 3, EpochNumber: 8, Amount: 480_00000, PoolFee: 20_00000, NumStakers: 3},
		{BatchNumber: 4, EpochNumber: 9, Amount: 100_00000, PoolFee: 5_00000, NumStakers: 3},
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
		EpochNumber    int64 `json:"epoch_number"`
		TotalAmount    int64 `json:"total_amount"`
		TotalFee       int64 `json:"total_fee"`
		Batches        int64 `json:"batches"`
		NumStakers     int64 `json:"num_stakers"`
		TotalStakeLuna int64 `json:"total_stake_luna"`
	}
	var got []rewardPoint
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	want := []rewardPoint{
		{EpochNumber: 7, TotalAmount: 590_00000, TotalFee: 25_00000, Batches: 2, NumStakers: 2, TotalStakeLuna: 10_000_00000},
		{EpochNumber: 8, TotalAmount: 480_00000, TotalFee: 20_00000, Batches: 1, NumStakers: 3, TotalStakeLuna: 12_000_00000},
		{EpochNumber: 9, TotalAmount: 100_00000, TotalFee: 5_00000, Batches: 1, NumStakers: 0, TotalStakeLuna: 0},
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
