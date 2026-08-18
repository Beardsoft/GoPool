package pool

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"
	"github.com/NimMiniApps/nimiq-go/signer"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
)

func TestStakerPercentage(t *testing.T) {
	got := stakerPercentage(nimiq.Luna(25_000_00000), nimiq.Luna(100_000_00000))
	if got < 24.99 || got > 25.01 {
		t.Errorf("percentage = %v, want ~25", got)
	}
}

func TestStakerPercentageZeroTotal(t *testing.T) {
	if got := stakerPercentage(0, 0); got != 0 {
		t.Errorf("percentage of a zero-balance validator = %v, want 0", got)
	}
}

// The genesis election must record its snapshot as epoch 0 as well: the first
// checkpoint attributes batch 0's reward to epoch 0, and without the row the
// rewards foreign key fails and the replay loop stalls on the first batch
// boundary forever.
func TestGenesisElectionRecordsEpochZeroForFirstCheckpoint(t *testing.T) {
	priv, err := signer.ParsePrivateKeyHex("6927eb8de74e8ea06a8afae5a66db176a7031f742b656651ac53bddb8a4ad3f3")
	if err != nil {
		t.Fatal(err)
	}
	addr := priv.Address().String()
	const staker = "NQ17 VERV F3MQ 283T NRSR FPJG 55BJ PMHC N8MD"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			ID     int    `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "getPolicyConstants":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"blocksPerBatch": 60, "batchesPerEpoch": 4, "blocksPerEpoch": 240, "genesisBlockNumber": 0},
			})
		case "getValidatorByAddress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"address": addr, "rewardAddress": addr, "balance": 10100000000, "numStakers": 1},
			})
		case "getStakersByValidatorAddress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": []map[string]any{{"address": staker, "balance": 100000000}},
			})
		case "getInherentsByBlockNumber":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": []map[string]any{{"type": "reward", "blockNumber": 60, "validatorAddress": addr, "value": 1000000}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": nil})
		}
	}))
	t.Cleanup(srv.Close)

	client, err := rpc.New(srv.URL, rpc.WithNetwork(nimiq.NetworkDevAlbatross))
	if err != nil {
		t.Fatal(err)
	}
	q := newLifecycleTestQueries(t)
	m := &Manager{
		chain:   &chain.Chain{RPC: client, Signer: priv},
		queries: q,
		cfg:     &config.Config{ValidatorAddress: addr, PoolFeePercentage: 1},
		policy:  &rpc.Policy{BlocksPerBatch: 60, BatchesPerEpoch: 4, BlocksPerEpoch: 240, GenesisBlockNumber: 0},
	}

	if err := m.handleElection(t.Context(), 0); err != nil {
		t.Fatalf("handleElection(genesis): %v", err)
	}
	for _, ep := range []int64{0, 1} {
		exists, err := q.EpochExists(t.Context(), ep)
		if err != nil || !exists {
			t.Fatalf("epoch %d exists=%v err=%v, want recorded", ep, exists, err)
		}
		stakers, err := q.GetStakersForEpoch(t.Context(), ep)
		if err != nil || len(stakers) != 1 {
			t.Fatalf("stakers for epoch %d = %d, err=%v, want 1", ep, len(stakers), err)
		}
	}

	if err := m.handleCheckpoint(t.Context(), 60); err != nil {
		t.Fatalf("handleCheckpoint(60): %v", err)
	}
	rewardExists, err := q.RewardExists(t.Context(), 0)
	if err != nil || !rewardExists {
		t.Fatalf("reward batch 0 exists=%v err=%v, want recorded", rewardExists, err)
	}
}
