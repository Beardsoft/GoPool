package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
)

const testAddr = "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"

func seedStakerWithPayslip(t *testing.T, q *db.Queries) {
	t.Helper()
	ctx := context.Background()
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 1, NumStakers: 1, Balance: 100, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertStaker(ctx, db.InsertStakerParams{EpochNumber: 1, Address: testAddr, Stake: 100, Percentage: 100}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertReward(ctx, db.InsertRewardParams{BatchNumber: 1, EpochNumber: 1, Amount: 5, PoolFee: 0, NumStakers: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertPayslip(ctx, db.InsertPayslipParams{BatchNumber: 1, Address: testAddr, Amount: 5, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleGetStaker(t *testing.T) {
	q := newTestDB(t)
	seedStakerWithPayslip(t, q)

	a := &API{queries: q}

	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/stakers/"+url.PathEscape(testAddr), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got stakerDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.StakeLuna != 100 || len(got.Payslips) != 1 {
		t.Errorf("got %+v", got)
	}
	if got.Payslips[0].EpochNumber != 1 {
		t.Errorf("payslip epoch = %d, want 1", got.Payslips[0].EpochNumber)
	}

	rec2 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/stakers/"+url.PathEscape("NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE"), nil))
	if rec2.Code != http.StatusNotFound {
		t.Errorf("unknown staker: status = %d, want 404", rec2.Code)
	}

	rec3 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/api/stakers/not-an-address", nil))
	if rec3.Code != http.StatusBadRequest {
		t.Errorf("malformed address: status = %d, want 400", rec3.Code)
	}
}

func TestHandleMe(t *testing.T) {
	q := newTestDB(t)
	ctx := context.Background()
	if err := q.InsertEpoch(ctx, db.InsertEpochParams{Number: 1, NumStakers: 1, Balance: 100, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertStaker(ctx, db.InsertStakerParams{EpochNumber: 1, Address: testAddr, Stake: 100, Percentage: 100}); err != nil {
		t.Fatal(err)
	}

	a := &API{queries: q, cfg: &config.Config{SessionSecret: "test-secret"}}
	addr, err := nimiq.ParseAddress(testAddr)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: a.issueSession(addr)})
	a.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got stakerDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.StakeLuna != 100 {
		t.Errorf("got %+v", got)
	}

	rec2 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("no cookie: status = %d, want 401", rec2.Code)
	}
}

func TestStakerPayslipsCSV(t *testing.T) {
	q := newTestDB(t)
	seedStakerWithPayslip(t, q)

	a := &API{queries: q}

	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/stakers/"+url.PathEscape(testAddr)+"/payslips.csv", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv" {
		t.Errorf("content-type = %q, want text/csv", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"epoch_number,batch_number,amount_luna,amount_nim,status,tx_hash", "1,1,5,0.00005,completed,"} {
		if !strings.Contains(body, want) {
			t.Errorf("csv missing %q in %q", want, body)
		}
	}

	rec404 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec404, httptest.NewRequest(http.MethodGet, "/api/stakers/"+url.PathEscape("NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE")+"/payslips.csv", nil))
	if rec404.Code != http.StatusNotFound {
		t.Errorf("unknown staker csv: status = %d, want 404", rec404.Code)
	}
}

func TestMePayslipsCSV(t *testing.T) {
	q := newTestDB(t)
	seedStakerWithPayslip(t, q)

	a := &API{queries: q, cfg: &config.Config{SessionSecret: "test-secret"}}
	addr, err := nimiq.ParseAddress(testAddr)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me/payslips.csv", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: a.issueSession(addr)})
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "1,1,5,0.00005,completed,") {
		t.Errorf("csv body = %q", rec.Body.String())
	}

	rec401 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec401, httptest.NewRequest(http.MethodGet, "/api/me/payslips.csv", nil))
	if rec401.Code != http.StatusUnauthorized {
		t.Errorf("no cookie: status = %d, want 401", rec401.Code)
	}
}
