package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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
