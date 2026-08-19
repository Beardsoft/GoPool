package pool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"
	"github.com/NimMiniApps/nimiq-go/signer"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/ops"
)

func TestReadinessRetryable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("load policy for pool wallet readiness: timeout"), true},
		{fmt.Errorf("load validator for pool wallet readiness: %w", rpc.ErrNotFound), true},
		{fmt.Errorf("on-chain reward address X does not match pool wallet Y"), true},
		{context.Canceled, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := readinessRetryable(c.err); got != c.want {
			t.Errorf("readinessRetryable(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestEnsureReadyRecordsHeartbeatThenSucceeds(t *testing.T) {
	priv, err := signer.ParsePrivateKeyHex("6927eb8de74e8ea06a8afae5a66db176a7031f742b656651ac53bddb8a4ad3f3")
	if err != nil {
		t.Fatal(err)
	}
	addr := priv.Address().String()

	var validatorCalls atomic.Int32
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
				"result": map[string]any{"blocksPerBatch": 60, "batchesPerEpoch": 720, "blocksPerEpoch": 43200},
			})
		case "getValidatorByAddress":
			n := validatorCalls.Add(1)
			if n <= 2 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": req.ID,
					"error": map[string]any{"code": -32601, "message": "Validator not found"},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"address": addr, "rewardAddress": addr, "balance": 0, "numStakers": 0},
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
		chain:    &chain.Chain{RPC: client, Signer: priv},
		queries:  q,
		cfg:      &config.Config{ValidatorAddress: addr},
		recorder: ops.NewRecorder(q, "testhash"),
	}

	err = m.ensureReady(t.Context())
	if err == nil || !readinessRetryable(err) {
		t.Fatalf("first ensureReady err = %v, want retryable", err)
	}
	status, statusErr := q.GetRuntimeStatus(t.Context())
	if statusErr != nil {
		t.Fatalf("heartbeat: %v", statusErr)
	}
	if !status.ReadinessError.Valid || status.ReadinessError.String == "" {
		t.Fatalf("expected readiness_error on heartbeat, got %+v", status)
	}

	if err := m.ensureReady(t.Context()); err != nil {
		t.Fatalf("second ensureReady: %v", err)
	}
	if m.policy == nil || m.policy.BlocksPerBatch != 60 {
		t.Fatalf("policy = %+v", m.policy)
	}
}

func TestReadinessRetryStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitReady(ctx, func(context.Context) error {
		return fmt.Errorf("load validator for pool wallet readiness: down")
	}, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
