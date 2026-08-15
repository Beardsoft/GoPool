package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type operatorOverviewResponseTest struct {
	Status   string `json:"status"`
	ChainLag int64  `json:"chain_lag"`
	Attention []struct {
		ID int64 `json:"id"`
	} `json:"attention"`
}

func TestOperatorOverviewReturnsHealthAndAttention(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	seedRuntimeStatus(t, a.queries, "active", 120, 122, true)
	seedFailedPayout(t, a.queries)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/overview", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
	var got operatorOverviewResponseTest
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "attention" || got.ChainLag != 2 || len(got.Attention) != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestOperatorReadinessReturnsOK(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	seedRuntimeStatus(t, a.queries, "active", 100, 100, true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/readiness", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTelemetryRejectsUnknownMetric(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/telemetry?metric=unknown&from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	var errResp map[string]string
	json.NewDecoder(rec.Body).Decode(&errResp)
	if errResp["code"] != "unknown_metric" {
		t.Fatalf("unexpected code %s", errResp["code"])
	}
}

func TestTelemetryRejectsLargeRange(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/telemetry?metric=wallet_balance&from=2026-01-01T00:00:00Z&to=2026-03-01T00:00:00Z", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestActivityCapsLimit(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/activity?limit=200", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestActivityExportReturnsCSV(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/activity/export", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "text/csv" {
		t.Fatalf("expected csv")
	}
}

func TestNonOperatorForbidden(t *testing.T) {
	a, _, stakerCookie := operatorTestAPI(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/overview", nil)
	req.AddCookie(stakerCookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
