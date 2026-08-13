package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/pool"
)

func operatorTestAPI(t *testing.T) (*API, *http.Cookie, *http.Cookie) {
	t.Helper()
	q := newTestDB(t)
	cfg := &config.Config{SessionSecret: "test-secret", ValidatorAddress: testAddr}
	a := &API{queries: q, cfg: cfg}

	operatorAddr, err := nimiq.ParseAddress(testAddr)
	if err != nil {
		t.Fatal(err)
	}
	stakerAddr, err := nimiq.ParseAddress("NQ00 0000 0000 0000 0000 0000 0000 0000 0001")
	if err != nil {
		stakerAddr, err = nimiq.ParseAddress("NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE")
	}
	if err != nil {
		t.Fatal(err)
	}
	operatorCookie := &http.Cookie{Name: sessionCookieName, Value: a.issueSession(operatorAddr)}
	stakerCookie := &http.Cookie{Name: sessionCookieName, Value: a.issueSession(stakerAddr)}
	return a, operatorCookie, stakerCookie
}

func TestRequireOperatorRejectsNonOperator(t *testing.T) {
	a, _, stakerCookie := operatorTestAPI(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/health", nil)
	req.AddCookie(stakerCookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestHandleOperatorDeactivate(t *testing.T) {
	a, operatorCookie, _ := operatorTestAPI(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/operator/validator/deactivate", nil)
	req.AddCookie(operatorCookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}

	rows, err := a.queries.GetRequestedValidatorActions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Action != "deactivate" {
		t.Fatalf("got %+v", rows)
	}

	// A second request while one is outstanding is rejected, not duplicated.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/operator/validator/deactivate", nil)
	req2.AddCookie(operatorCookie)
	a.Mux().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Errorf("duplicate request: status = %d, want 409", rec2.Code)
	}
}

func TestHandleOperatorHealth(t *testing.T) {
	a, operatorCookie, _ := operatorTestAPI(t)
	ctx := context.Background()
	if err := a.queries.UpsertCursor(ctx, db.UpsertCursorParams{Name: pool.CursorName, Height: 100}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/health", nil)
	req.AddCookie(operatorCookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.LastProcessedHeight != 100 {
		t.Errorf("got %+v", got)
	}
}
