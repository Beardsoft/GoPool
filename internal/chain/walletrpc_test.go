package chain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWalletRPCCreateValidator(t *testing.T) {
	var importKey, unlockAddr, sender, signing, voting string
	var createCalls atomic.Int32
	wallet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			ID     int             `json:"id"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "isConsensusEstablished":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": true})
		case "importRawKey":
			var p []any
			_ = json.Unmarshal(req.Params, &p)
			if len(p) > 0 {
				importKey, _ = p[0].(string)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": "NQ20"})
		case "unlockAccount":
			var p []any
			_ = json.Unmarshal(req.Params, &p)
			if len(p) > 0 {
				unlockAddr, _ = p[0].(string)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": true})
		case "sendNewValidatorTransaction":
			createCalls.Add(1)
			var p []any
			_ = json.Unmarshal(req.Params, &p)
			if len(p) >= 4 {
				sender, _ = p[0].(string)
				signing, _ = p[2].(string)
				voting, _ = p[3].(string)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": "txhash"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": nil})
		}
	}))
	defer wallet.Close()

	var poolCreates atomic.Int32
	pool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			ID     int    `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "sendNewValidatorTransaction" {
			poolCreates.Add(1)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": nil})
	}))
	defer pool.Close()

	client, err := NewWalletRPC(wallet.URL)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := client.ConsensusEstablished(context.Background())
	if err != nil || !ok {
		t.Fatalf("consensus = %v %v", ok, err)
	}
	hash, err := client.CreateValidator(context.Background(), "NQ20", "addrkey", "signkey", "votekey")
	if err != nil {
		t.Fatal(err)
	}
	if hash != "txhash" {
		t.Fatalf("hash = %q", hash)
	}
	if importKey != "addrkey" || unlockAddr != "NQ20" || sender != "NQ20" || signing != "signkey" || voting != "votekey" {
		t.Fatalf("import=%s unlock=%s sender=%s signing=%s voting=%s", importKey, unlockAddr, sender, signing, voting)
	}
	if createCalls.Load() != 1 {
		t.Fatalf("wallet create calls = %d", createCalls.Load())
	}
	if poolCreates.Load() != 0 {
		t.Fatalf("pool RPC must not receive sendNewValidatorTransaction, got %d", poolCreates.Load())
	}
}
