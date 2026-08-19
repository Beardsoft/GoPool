package pool

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"
	"github.com/NimMiniApps/nimiq-go/signer"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
)

const bootstrapKeyHex = "6927eb8de74e8ea06a8afae5a66db176a7031f742b656651ac53bddb8a4ad3f3"

func TestRunBootstrapRegistersWhenFunded(t *testing.T) {
	priv, err := signer.ParsePrivateKeyHex(bootstrapKeyHex)
	if err != nil {
		t.Fatal(err)
	}
	addr := priv.Address().String()

	var creates atomic.Int32
	wallet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			ID     int    `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "isConsensusEstablished":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": true})
		case "importRawKey":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": addr})
		case "unlockAccount":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": true})
		case "sendNewValidatorTransaction":
			creates.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": "regtx"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": nil})
		}
	}))
	t.Cleanup(wallet.Close)

	pool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			ID     int    `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "getPolicyConstants":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{
					"blocksPerBatch": 60, "batchesPerEpoch": 720, "blocksPerEpoch": 43200,
					"validatorDeposit": 10_000_000_000, "minimumStake": 10_000_000,
				},
			})
		case "getAccountByAddress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"address": addr, "balance": 10_100_000_000, "type": "basic"},
			})
		case "getValidatorByAddress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32601, "message": "Validator not found"},
			})
		case "getStakerByAddress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32601, "message": "Staker not found"},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": nil})
		}
	}))
	t.Cleanup(pool.Close)

	walletPath := filepath.Join(t.TempDir(), "wallet.json")
	if err := os.WriteFile(walletPath, []byte(`{"signing_private_key":"sign","voting_secret_key":"vote"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rpcClient, err := rpc.New(pool.URL, rpc.WithNetwork(nimiq.NetworkTestAlbatross))
	if err != nil {
		t.Fatal(err)
	}
	wRPC, err := chain.NewWalletRPC(wallet.URL)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{
		chain:   &chain.Chain{RPC: rpcClient, Signer: priv, Network: nimiq.NetworkTestAlbatross, Wallet: wRPC},
		queries: newLifecycleTestQueries(t),
		cfg: &config.Config{
			Network: "test-albatross", PrivateKey: bootstrapKeyHex,
			ValidatorAddress: addr, WalletJSONFile: walletPath,
		},
		policy: &rpc.Policy{ValidatorDeposit: 10_000_000_000, MinimumStake: 10_000_000},
	}
	m.runBootstrap(t.Context())
	if creates.Load() != 1 {
		t.Fatalf("CreateValidator calls = %d", creates.Load())
	}
}

func TestRunBootstrapDryRunDoesNotRegister(t *testing.T) {
	priv, err := signer.ParsePrivateKeyHex(bootstrapKeyHex)
	if err != nil {
		t.Fatal(err)
	}
	var creates atomic.Int32
	wallet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			ID     int    `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "sendNewValidatorTransaction" {
			creates.Add(1)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": true})
	}))
	t.Cleanup(wallet.Close)
	pool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			ID     int    `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
			"address": priv.Address().String(), "balance": 10_100_000_000, "type": "basic",
		}})
	}))
	t.Cleanup(pool.Close)
	rpcClient, _ := rpc.New(pool.URL, rpc.WithNetwork(nimiq.NetworkTestAlbatross))
	wRPC, _ := chain.NewWalletRPC(wallet.URL)
	m := &Manager{
		chain:   &chain.Chain{RPC: rpcClient, Signer: priv, Wallet: wRPC},
		queries: newLifecycleTestQueries(t),
		cfg:     &config.Config{Network: "test-albatross", DryRun: true, WalletJSONFile: "x"},
		policy:  &rpc.Policy{ValidatorDeposit: 10_000_000_000},
	}
	m.runBootstrap(t.Context())
	if creates.Load() != 0 {
		t.Fatal("dry-run must not register")
	}
}

func TestEnsureReadyWaitingCopy(t *testing.T) {
	priv, err := signer.ParsePrivateKeyHex(bootstrapKeyHex)
	if err != nil {
		t.Fatal(err)
	}
	addr := priv.Address().String()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			ID     int    `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "getPolicyConstants":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"blocksPerBatch": 60, "batchesPerEpoch": 720, "blocksPerEpoch": 43200, "validatorDeposit": 10_000_000_000},
			})
		case "getValidatorByAddress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32601, "message": "Validator not found"},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": nil})
		}
	}))
	t.Cleanup(srv.Close)
	client, _ := rpc.New(srv.URL, rpc.WithNetwork(nimiq.NetworkTestAlbatross))
	m := &Manager{
		chain:    &chain.Chain{RPC: client, Signer: priv},
		queries:  newLifecycleTestQueries(t),
		cfg:      &config.Config{ValidatorAddress: addr, Network: "test-albatross"},
		recorder: nil,
	}
	err = m.ensureReady(t.Context())
	if err == nil || !strings.Contains(err.Error(), "Waiting for 100000 NIM to register the validator") {
		t.Fatalf("err = %v", err)
	}
}
