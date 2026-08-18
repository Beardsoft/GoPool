package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/db"
)

func TestCancelValidatorActionOnlyWhileRequested(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	requestedID := seedAction(t, a.queries, "retire", "requested")
	assertPostStatus(t, a, cookie, fmt.Sprintf("/api/operator/actions/%d/cancel", requestedID), http.StatusOK)
	if err := a.queries.InsertValidatorAction(t.Context(), db.InsertValidatorActionParams{Action: "retire", Outcome: "requested"}); err != nil {
		t.Fatal(err)
	}
	legacyRequested, err := a.queries.GetRequestedValidatorActions(t.Context())
	if err != nil || len(legacyRequested) != 1 {
		t.Fatalf("legacy requested = %+v, err = %v", legacyRequested, err)
	}
	assertPostStatus(t, a, cookie, fmt.Sprintf("/api/operator/actions/%d/cancel", legacyRequested[0].ID), http.StatusOK)
	processingID := seedAction(t, a.queries, "deactivate", "processing")
	assertPostStatus(t, a, cookie, fmt.Sprintf("/api/operator/actions/%d/cancel", processingID), http.StatusConflict)
}

func TestCreateValidatorActionReturnsServerTruthAndAudits(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	rec := postJSON(t, a.Mux(), "/api/operator/actions", map[string]string{
		"action": "retire", "correlation_id": "operator-request-1",
	}, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got operatorActionResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID == 0 || got.State != "requested" || got.Action != "retire" || got.CorrelationID == nil || *got.CorrelationID != "operator-request-1" {
		t.Fatalf("create response = %+v", got)
	}
	stored, err := a.queries.GetValidatorAction(t.Context(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Outcome != "requested" || !stored.State.Valid || stored.State.String != "requested" {
		t.Fatalf("stored action = %+v", stored)
	}
	events, err := a.queries.ListOperatorEvents(t.Context(), db.ListOperatorEventsParams{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventType != "validator_action_requested" || events[0].CorrelationID.String != "operator-request-1" {
		t.Fatalf("events = %+v", events)
	}

	duplicate := postJSON(t, a.Mux(), "/api/operator/actions", map[string]string{"action": "retire"}, cookie)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, body: %s", duplicate.Code, duplicate.Body.String())
	}
}

func TestOperatorActionListMapsLegacyPendingToSubmitted(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	if err := a.queries.InsertValidatorAction(t.Context(), db.InsertValidatorActionParams{
		Action: "retire", TxHash: sql.NullString{String: "legacy-submitted", Valid: true}, Outcome: "pending",
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.queries.InsertValidatorAction(t.Context(), db.InsertValidatorActionParams{
		Action: "deactivate", Outcome: "requested",
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/actions?status=submitted", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Items []operatorActionResponse `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].State != "submitted" || got.Items[0].Action != "retire" {
		t.Fatalf("actions = %+v", got.Items)
	}
}

func TestRetryPayoutOnlyResetsFailedGroupWithoutActiveTransaction(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	const address = "NQ00 0000 0000 0000 0000 0000 0000 0000 0001"
	seedFailedPayoutGroup(t, a.queries, "failed-payout", address)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/operator/payouts/failed-payout/retry", nil)
	req.Header.Set("X-Correlation-ID", "payout-retry-1")
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("retry status = %d, body: %s", rec.Code, rec.Body.String())
	}
	payslips, err := a.queries.GetPayslipsForAddress(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	if len(payslips) != 1 || payslips[0].Status != "pending" || payslips[0].TxHash.Valid {
		t.Fatalf("payslips = %+v", payslips)
	}
	events, err := a.queries.ListOperatorEvents(t.Context(), db.ListOperatorEventsParams{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventType != "payout_retry_requested" || events[0].CorrelationID.String != "payout-retry-1" {
		t.Fatalf("events = %+v", events)
	}
}

func TestRetryPayoutRejectsGroupWhenAddressHasActiveTransaction(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	const address = "NQ00 0000 0000 0000 0000 0000 0000 0000 0001"
	seedFailedPayoutGroup(t, a.queries, "failed-payout", address)
	if err := a.queries.InsertTransaction(t.Context(), db.InsertTransactionParams{
		Hash: "active-payout", Address: address, Amount: 1, Status: "awaiting_confirmation",
	}); err != nil {
		t.Fatal(err)
	}
	assertPostStatus(t, a, cookie, "/api/operator/payouts/failed-payout/retry", http.StatusConflict)
}

func seedFailedPayoutGroup(t *testing.T, q *db.Queries, hash, address string) {
	t.Helper()
	if err := q.InsertEpoch(t.Context(), db.InsertEpochParams{Number: 1, NumStakers: 1, Balance: 1, Status: "in_progress"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertReward(t.Context(), db.InsertRewardParams{BatchNumber: 1, EpochNumber: 1, Amount: 1, PoolFee: 0, NumStakers: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertPayslip(t.Context(), db.InsertPayslipParams{BatchNumber: 1, Address: address, Amount: 1, Status: "out_for_payment"}); err != nil {
		t.Fatal(err)
	}
	if err := q.SetPayslipsTransaction(t.Context(), db.SetPayslipsTransactionParams{TxHash: sql.NullString{String: hash, Valid: true}, Address: address}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdatePayslipStatusFailed(t.Context(), sql.NullString{String: hash, Valid: true}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertTransaction(t.Context(), db.InsertTransactionParams{Hash: hash, Address: address, Amount: 1, Status: "failed"}); err != nil {
		t.Fatal(err)
	}
}

func TestOperatorPayoutsIncludeHeightAndStuckFlag(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "getBlockNumber":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":540}`))
		case "getPolicyConstants":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"blocksPerBatch":60,"batchesPerEpoch":4,"blocksPerEpoch":100,"blockSeparationTime":1000,"genesisBlockNumber":0}}`))
		default:
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`))
		}
	}))
	defer srv.Close()
	client, err := rpc.New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	a.rpc = client
	a.cfg.StuckPayoutEpochs = 3

	const address = "NQ00 0000 0000 0000 0000 0000 0000 0001"
	const freshAddress = "NQ32 EGL6 H9C8 0JJB PH4S 7RYY ULRC 5B6N 56RE"
	for _, tx := range []db.InsertTransactionParams{
		{Hash: "old-pending", Address: address, Amount: 1, Status: "awaiting_confirmation", SubmittedHeight: 100},
		{Hash: "fresh-pending", Address: freshAddress, Amount: 1, Status: "awaiting_confirmation", SubmittedHeight: 500},
		{Hash: "done", Address: address, Amount: 1, Status: "completed", SubmittedHeight: 100},
	} {
		if err := a.queries.InsertTransaction(t.Context(), tx); err != nil {
			t.Fatal(err)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/payouts?limit=10", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("payouts status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Items []operatorPayoutResponse `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 3 {
		t.Fatalf("items = %+v, want 3", got.Items)
	}
	byHash := map[string]operatorPayoutResponse{}
	for _, it := range got.Items {
		byHash[it.Hash] = it
	}
	if old := byHash["old-pending"]; !old.Stuck || old.SubmittedHeight != 100 {
		t.Errorf("old-pending = %+v, want stuck=true height=100", old)
	}
	if fresh := byHash["fresh-pending"]; fresh.Stuck || fresh.SubmittedHeight != 500 {
		t.Errorf("fresh-pending = %+v, want stuck=false height=500", fresh)
	}
	if done := byHash["done"]; done.Stuck {
		t.Errorf("done = %+v, want stuck=false (terminal)", done)
	}
}

func TestOperatorPayoutsIncludeEpochRange(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	const address = "NQ00 0000 0000 0000 0000 0000 0000 0001"
	for _, epoch := range []int64{12, 13} {
		if err := a.queries.InsertEpoch(t.Context(), db.InsertEpochParams{Number: epoch, NumStakers: 1, Balance: 1, Status: "completed"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.queries.InsertReward(t.Context(), db.InsertRewardParams{BatchNumber: 10, EpochNumber: 12, Amount: 50, PoolFee: 0, NumStakers: 1}); err != nil {
		t.Fatal(err)
	}
	if err := a.queries.InsertReward(t.Context(), db.InsertRewardParams{BatchNumber: 11, EpochNumber: 13, Amount: 50, PoolFee: 0, NumStakers: 1}); err != nil {
		t.Fatal(err)
	}
	if err := a.queries.InsertPayslip(t.Context(), db.InsertPayslipParams{BatchNumber: 10, Address: address, Amount: 50, Status: "out_for_payment"}); err != nil {
		t.Fatal(err)
	}
	if err := a.queries.InsertPayslip(t.Context(), db.InsertPayslipParams{BatchNumber: 11, Address: address, Amount: 50, Status: "out_for_payment"}); err != nil {
		t.Fatal(err)
	}
	if err := a.queries.SetPayslipsTransaction(t.Context(), db.SetPayslipsTransactionParams{
		TxHash: sql.NullString{String: "bundled-payout", Valid: true}, Address: address,
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.queries.InsertTransaction(t.Context(), db.InsertTransactionParams{
		Hash: "bundled-payout", Address: address, Amount: 100, Status: "awaiting_confirmation", SubmittedHeight: 540,
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.queries.InsertTransaction(t.Context(), db.InsertTransactionParams{
		Hash: "orphan-payout", Address: address, Amount: 1, Status: "completed", SubmittedHeight: 100,
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/payouts?limit=10", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("payouts status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Items []struct {
			Hash      string `json:"hash"`
			EpochFrom *int64 `json:"epoch_from"`
			EpochTo   *int64 `json:"epoch_to"`
		} `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	byHash := map[string]struct {
		EpochFrom *int64
		EpochTo   *int64
	}{}
	for _, it := range got.Items {
		byHash[it.Hash] = struct {
			EpochFrom *int64
			EpochTo   *int64
		}{it.EpochFrom, it.EpochTo}
	}
	bundled := byHash["bundled-payout"]
	if bundled.EpochFrom == nil || *bundled.EpochFrom != 12 || bundled.EpochTo == nil || *bundled.EpochTo != 13 {
		t.Fatalf("bundled-payout epochs = %+v, want 12–13", bundled)
	}
	orphan := byHash["orphan-payout"]
	if orphan.EpochFrom != nil || orphan.EpochTo != nil {
		t.Fatalf("orphan-payout epochs = %+v, want none", orphan)
	}
}

func TestOperatorPayoutsPaginatesWithHasMore(t *testing.T) {
	a, cookie, _ := operatorTestAPI(t)
	const address = "NQ00 0000 0000 0000 0000 0000 0001"
	for i := 0; i < 51; i++ {
		if err := a.queries.InsertTransaction(t.Context(), db.InsertTransactionParams{
			Hash: fmt.Sprintf("tx-%02d", i), Address: address, Amount: 1, Status: "completed",
		}); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/payouts", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var first struct {
		Items      []operatorPayoutResponse `json:"items"`
		NextCursor int                      `json:"next_cursor"`
		HasMore    bool                     `json:"has_more"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 50 || !first.HasMore || first.NextCursor != 50 {
		t.Fatalf("first page: items=%d has_more=%v next_cursor=%d", len(first.Items), first.HasMore, first.NextCursor)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/operator/payouts?cursor=50", nil)
	req.AddCookie(cookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var second struct {
		Items   []operatorPayoutResponse `json:"items"`
		HasMore bool                     `json:"has_more"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.HasMore {
		t.Fatalf("second page: items=%d has_more=%v", len(second.Items), second.HasMore)
	}
}
