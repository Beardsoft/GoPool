package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Beardsoft/GoPool/internal/db"
)

func configuredOperatorAPI(t *testing.T) (*API, *http.Cookie) {
	t.Helper()
	a, cookie, _ := operatorTestAPI(t)
	return a, cookie
}

func seedRuntimeStatus(t *testing.T, q *db.Queries, validatorState string, lastProcessed, chainHead int64, rpcOk bool) {
	t.Helper()
	rpc := int64(0)
	if rpcOk {
		rpc = 1
	}
	err := q.UpsertRuntimeStatus(t.Context(), db.UpsertRuntimeStatusParams{
		HeartbeatAt:             time.Now().UTC(),
		DaemonVersion:           "v0.1",
		ConfigHash:              "abc",
		DerivedValidatorAddress: testAddr,
		ValidatorState:          validatorState,
		LastProcessedHeight:     lastProcessed,
		ChainHead:               chainHead,
		LastTickMs:              1000,
		RpcOk:                   rpc,
		ReadinessError:          sql.NullString{},
	})
	if err != nil {
		t.Fatalf("seedRuntimeStatus: %v", err)
	}
}

func seedFailedPayout(t *testing.T, q *db.Queries) {
	t.Helper()
	_, err := q.InsertOperatorEvent(t.Context(), db.InsertOperatorEventParams{
		Severity:    "error",
		Category:    "payout",
		Source:      "daemon",
		EventType:   "payout_failed",
		Summary:     "Payout submission failed",
		ContextJson: sql.NullString{String: `{"address":"NQ..."}`, Valid: true},
	})
	if err != nil {
		t.Fatalf("seedFailedPayout: %v", err)
	}
}

func seedAction(t *testing.T, q *db.Queries, action, state string) int64 {
	t.Helper()
	id, err := q.InsertValidatorActionWithState(t.Context(), db.InsertValidatorActionWithStateParams{
		Action:      action,
		State:       sql.NullString{String: state, Valid: true},
		RequestedBy: sql.NullString{String: "test", Valid: true},
	})
	if err != nil {
		t.Fatalf("seedAction: %v", err)
	}
	return id
}

func assertPostStatus(t *testing.T, a *API, cookie *http.Cookie, path string, want int) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("POST %s: got %d, want %d, body: %s", path, rec.Code, want, rec.Body.String())
	}
}

func postJSON(t *testing.T, mux http.Handler, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func putJSON(t *testing.T, mux http.Handler, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}
