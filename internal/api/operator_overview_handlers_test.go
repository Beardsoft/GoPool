package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Beardsoft/GoPool/internal/db"
)

type operatorOverviewResponseTest struct {
	Status           string `json:"status"`
	ChainLag         int64  `json:"chain_lag"`
	WalletRunwayDays *int   `json:"wallet_runway_days"`
	ValidatorSummary struct {
		Address             string `json:"address"`
		State               string `json:"state"`
		LastProcessedHeight int64  `json:"last_processed_height"`
		LastTickMs          int64  `json:"last_tick_ms"`
	} `json:"validator_summary"`
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
	if got.ValidatorSummary.Address == "" || got.ValidatorSummary.State != "active" || got.ValidatorSummary.LastProcessedHeight != 120 {
		t.Fatalf("validator summary is incomplete: %+v", got.ValidatorSummary)
	}
	if got.WalletRunwayDays != nil {
		t.Fatalf("wallet_runway_days = %d, want null (no wallet snapshots)", *got.WalletRunwayDays)
	}
}

func TestOperatorOverviewWalletRunway(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	seedRuntimeStatus(t, a.queries, "active", 100, 100, true)
	seedWalletRunway(t, a.queries, 3000, 300)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/overview", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got operatorOverviewResponseTest
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.WalletRunwayDays == nil || *got.WalletRunwayDays != 300 {
		t.Fatalf("wallet_runway_days = %v, want 300 (3000 balance / 10 avg daily)", got.WalletRunwayDays)
	}
}

func TestOperatorOverviewWalletRunwayNullWithoutConfirmedPayouts(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	seedRuntimeStatus(t, a.queries, "active", 100, 100, true)
	seedWalletRunway(t, a.queries, 3000, 0)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/overview", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got operatorOverviewResponseTest
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.WalletRunwayDays != nil {
		t.Fatalf("wallet_runway_days = %d, want null (no confirmed payouts)", *got.WalletRunwayDays)
	}
}

func seedSnapshot(t *testing.T, q *db.Queries, at time.Time, chainHead, processed int64) {
	t.Helper()
	_, err := q.InsertHealthSnapshot(t.Context(), db.InsertHealthSnapshotParams{
		RecordedAt:      at,
		ChainHead:       chainHead,
		ProcessedHeight: processed,
		ValidatorState:  "active",
		RpcOk:           1,
	})
	if err != nil {
		t.Fatalf("seedSnapshot: %v", err)
	}
}

// seedWalletRunway records a wallet-balance snapshot and, when completedLuna
// is non-zero, drives one payout through the real status flow to completed.
func seedWalletRunway(t *testing.T, q *db.Queries, balance, completedLuna int64) {
	t.Helper()
	_, err := q.InsertHealthSnapshot(t.Context(), db.InsertHealthSnapshotParams{
		RecordedAt:     time.Now().UTC(),
		ValidatorState: "active",
		WalletBalance:  balance,
		RpcOk:          1,
	})
	if err != nil {
		t.Fatalf("seedWalletRunway snapshot: %v", err)
	}
	if completedLuna == 0 {
		return
	}
	if err := q.InsertReward(t.Context(), db.InsertRewardParams{BatchNumber: 1, EpochNumber: 1, Amount: completedLuna, NumStakers: 1}); err != nil {
		t.Fatalf("seedWalletRunway reward: %v", err)
	}
	if err := q.InsertPayslip(t.Context(), db.InsertPayslipParams{BatchNumber: 1, Address: testAddr, Amount: completedLuna, Status: "out_for_payment"}); err != nil {
		t.Fatalf("seedWalletRunway payslip: %v", err)
	}
	if err := q.InsertTransaction(t.Context(), db.InsertTransactionParams{Hash: "0xrunway", Address: testAddr, Amount: completedLuna, Status: "awaiting_confirmation", SubmittedHeight: 1}); err != nil {
		t.Fatalf("seedWalletRunway tx: %v", err)
	}
	if err := q.SetPayslipsTransaction(t.Context(), db.SetPayslipsTransactionParams{TxHash: sql.NullString{String: "0xrunway", Valid: true}, Address: testAddr}); err != nil {
		t.Fatalf("seedWalletRunway set tx: %v", err)
	}
	if err := q.FinalizePayslips(t.Context(), sql.NullString{String: "0xrunway", Valid: true}); err != nil {
		t.Fatalf("seedWalletRunway finalize: %v", err)
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

func TestTelemetryReturnsBucketizedChainLag(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	seedRuntimeStatus(t, a.queries, "active", 100, 100, true)
	bucketStart := time.Now().UTC().Truncate(15 * time.Minute)
	// Two samples in the previous 15m bucket (lag 5 then 2), one in the current (lag 7)
	seedSnapshot(t, a.queries, bucketStart.Add(-5*time.Minute), 1005, 1000)
	seedSnapshot(t, a.queries, bucketStart.Add(-1*time.Minute), 1002, 1000)
	seedSnapshot(t, a.queries, bucketStart.Add(5*time.Minute), 1007, 1000)
	rec := httptest.NewRecorder()
	url := "/api/operator/telemetry?metric=chain_lag&from=" + bucketStart.Add(-time.Hour).Format(time.RFC3339) +
		"&to=" + bucketStart.Add(15*time.Minute).Format(time.RFC3339) + "&bucket=15m"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got []struct {
		Ts    string  `json:"ts"`
		Value float64 `json:"value"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("points = %+v, want 2 buckets", got)
	}
	if got[0].Value != 2 || got[1].Value != 7 {
		t.Fatalf("values = [%g, %g], want [2, 7] (last sample per bucket)", got[0].Value, got[1].Value)
	}
	if want := bucketStart.Add(-15 * time.Minute).Format(time.RFC3339); got[0].Ts != want {
		t.Fatalf("first ts = %s, want %s", got[0].Ts, want)
	}
}

func TestTelemetryCapsPointsAt500(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	seedRuntimeStatus(t, a.queries, "active", 100, 100, true)
	base := time.Now().UTC().Truncate(time.Hour).Add(-6 * time.Hour)
	for i := 0; i < 501; i++ {
		seedSnapshot(t, a.queries, base.Add(time.Duration(i)*time.Minute), int64(1000+i), 1000)
	}
	rec := httptest.NewRecorder()
	url := "/api/operator/telemetry?metric=chain_lag&from=" + base.Add(-time.Hour).Format(time.RFC3339) +
		"&to=" + base.Add(9*time.Hour).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got []struct {
		Ts    string  `json:"ts"`
		Value float64 `json:"value"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 500 {
		t.Fatalf("points = %d, want 500", len(got))
	}
	if got[0].Value != 0 {
		t.Fatalf("first value = %g, want 0", got[0].Value)
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

func TestActivityPaginatesWithHasMore(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	for i := 0; i < 55; i++ {
		if _, err := a.queries.InsertOperatorEvent(t.Context(), db.InsertOperatorEventParams{
			Severity: "info", Category: "test", Source: "test", EventType: "test", Summary: fmt.Sprintf("event %d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/activity", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var first struct {
		Items      []map[string]any `json:"items"`
		NextCursor int              `json:"next_cursor"`
		HasMore    bool             `json:"has_more"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 50 || !first.HasMore || first.NextCursor != 50 {
		t.Fatalf("first page: items=%d has_more=%v next_cursor=%d", len(first.Items), first.HasMore, first.NextCursor)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/operator/activity?cursor=50", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var second struct {
		Items   []map[string]any `json:"items"`
		HasMore bool             `json:"has_more"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 5 || second.HasMore {
		t.Fatalf("second page: items=%d has_more=%v", len(second.Items), second.HasMore)
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
