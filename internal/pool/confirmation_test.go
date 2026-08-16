package pool

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NimMiniApps/nimiq-go/rpc"
	_ "github.com/mattn/go-sqlite3"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
)

func TestConfirmationOutcome(t *testing.T) {
	cases := []struct {
		name string
		conf *rpc.Confirmation
		want confirmOutcome
	}{
		{"not yet final", &rpc.Confirmation{Included: true, Final: false, Succeeded: true}, outcomePending},
		{"final and succeeded", &rpc.Confirmation{Included: true, Final: true, Succeeded: true}, outcomeSucceeded},
		{"final but failed", &rpc.Confirmation{Included: true, Final: true, Succeeded: false}, outcomeFailed},
	}
	for _, c := range cases {
		if got := confirmationOutcome(c.conf); got != c.want {
			t.Errorf("%s: confirmationOutcome = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLegacyRequestedValidatorActionCanBeClaimed(t *testing.T) {
	q := newLifecycleTestQueries(t)
	if err := q.InsertValidatorAction(t.Context(), db.InsertValidatorActionParams{Action: "retire", Outcome: "requested"}); err != nil {
		t.Fatal(err)
	}
	requested, err := q.GetRequestedValidatorActions(t.Context())
	if err != nil || len(requested) != 1 {
		t.Fatalf("requested = %+v, err = %v", requested, err)
	}
	if _, err := q.MarkValidatorActionProcessing(t.Context(), requested[0].ID); err != nil {
		t.Fatalf("claim legacy requested action: %v", err)
	}
	stored, err := q.GetValidatorAction(t.Context(), requested[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.State.Valid || stored.State.String != "processing" {
		t.Fatalf("state = %+v, want processing", stored.State)
	}
}

func TestValidatorActionTransitionsAndLegacySubmittedReconciliation(t *testing.T) {
	q := newLifecycleTestQueries(t)

	requestedID, err := q.InsertValidatorActionWithState(t.Context(), db.InsertValidatorActionWithStateParams{
		Action: "retire", State: sql.NullString{String: "requested", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.MarkValidatorActionProcessing(t.Context(), requestedID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SubmitValidatorAction(t.Context(), db.SubmitValidatorActionParams{
		ID: requestedID, TxHash: sql.NullString{String: "new-submitted", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	if err := q.InsertValidatorAction(t.Context(), db.InsertValidatorActionParams{
		Action: "deactivate", TxHash: sql.NullString{String: "legacy-submitted", Valid: true}, Outcome: "pending",
	}); err != nil {
		t.Fatal(err)
	}

	submitted, err := q.ListValidatorActions(t.Context(), db.ListValidatorActionsParams{
		Status: sql.NullString{String: "submitted", Valid: true},
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(submitted) != 2 {
		t.Fatalf("submitted = %+v, want both legacy and current rows", submitted)
	}
	for _, action := range submitted {
		if !action.State.Valid || action.State.String != "submitted" {
			t.Fatalf("exposed state = %+v, want submitted", action.State)
		}
		terminalState := "confirmed"
		if action.TxHash.String == "legacy-submitted" {
			terminalState = "failed"
		}
		if _, err := q.CompleteSubmittedValidatorAction(t.Context(), db.CompleteSubmittedValidatorActionParams{
			ID: action.ID, State: sql.NullString{String: terminalState, Valid: true},
		}); err != nil {
			t.Fatalf("complete action %d: %v", action.ID, err)
		}
		stored, err := q.GetValidatorAction(t.Context(), action.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !stored.State.Valid || stored.State.String != terminalState {
			t.Fatalf("terminal state = %+v, want %s", stored.State, terminalState)
		}
	}
}

func TestCancelledContextReleasesClaimedValidatorAction(t *testing.T) {
	q := newLifecycleTestQueries(t)
	id, err := q.InsertValidatorActionWithState(t.Context(), db.InsertValidatorActionWithStateParams{
		Action: "retire", State: sql.NullString{String: "requested", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.MarkValidatorActionProcessing(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	m := &Manager{queries: q}
	if err := m.recoverClaimedValidatorAction(ctx, id); err != nil {
		t.Fatal(err)
	}
	stored, err := q.GetValidatorAction(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.State.Valid || stored.State.String != "requested" {
		t.Fatalf("state = %+v, want requested", stored.State)
	}
}

func newLifecycleTestQueries(t *testing.T) *db.Queries {
	t.Helper()
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Migrate(sqlDB); err != nil {
		t.Fatal(err)
	}
	return db.New(sqlDB)
}

func TestInsertAndReadSubmittedHeight(t *testing.T) {
	q := newLifecycleTestQueries(t)
	if err := q.InsertTransaction(t.Context(), db.InsertTransactionParams{
		Hash: "h1", Address: "A", Amount: 100, Status: "awaiting_confirmation",
		SubmittedHeight: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	pending, err := q.GetPendingTransactions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].SubmittedHeight != 1000 {
		t.Fatalf("pending = %+v, want one row with submitted_height 1000", pending)
	}
}

func TestEligiblePayoutExcludesActiveAttempt(t *testing.T) {
	q := newLifecycleTestQueries(t)
	ctx := t.Context()
	for batch, address := range []string{"A", "B", "C"} {
		if err := q.InsertPayslip(ctx, db.InsertPayslipParams{
			BatchNumber: int64(batch + 1), Address: address, Amount: 100, Status: "pending",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
		ActionType: "payout", Address: "A", Amount: 100, Kind: "delegate", Status: "approved",
	}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertTransaction(ctx, db.InsertTransactionParams{
		Hash: "active-b", Address: "B", Amount: 100, Status: "awaiting_confirmation", SubmittedHeight: 10,
	}); err != nil {
		t.Fatal(err)
	}

	eligible, err := q.GetEligibleForPayout(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(eligible) != 1 || eligible[0].Address != "C" {
		t.Fatalf("eligible = %+v, want only C", eligible)
	}
}

func TestPersistPayoutSubmissionClaimsAuditExactlyOnce(t *testing.T) {
	q := newLifecycleTestQueries(t)
	ctx := t.Context()
	if err := q.InsertReward(ctx, db.InsertRewardParams{BatchNumber: 1, EpochNumber: 1, Amount: 100, PoolFee: 0, NumStakers: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertPayslip(ctx, db.InsertPayslipParams{BatchNumber: 1, Address: "A", Amount: 100, Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	id, err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{ActionType: "payout", Address: "A", Amount: 100, Kind: "delegate", Status: "approved"})
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{queries: q}
	log := db.ListApprovedAuditLogsRow{ID: id, Address: "A", Amount: 100, Kind: "delegate", Status: "approved"}
	if err := m.persistPayoutSubmission(ctx, log, "hash-a", 123); err != nil {
		t.Fatal(err)
	}
	if err := m.persistPayoutSubmission(ctx, log, "hash-b", 124); err == nil {
		t.Fatal("second claim unexpectedly succeeded")
	}
	pending, err := q.GetPendingTransactions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Hash != "hash-a" || pending[0].SubmittedHeight != 123 {
		t.Fatalf("pending = %+v, want one atomically tracked hash-a", pending)
	}
	approved, err := q.ListApprovedAuditLogs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(approved) != 0 {
		t.Fatalf("approved logs = %+v, want claimed audit removed from retry queue", approved)
	}
}

func TestRunConfirmationsFailsStuckPayout(t *testing.T) {
	q := newLifecycleTestQueries(t)
	const blocksPerEpoch = uint32(43200)
	const submitted = int64(1000)
	const head = submitted + 4*int64(blocksPerEpoch) // 4 epochs behind -> stuck (threshold 3)

	if err := q.InsertReward(t.Context(), db.InsertRewardParams{BatchNumber: 1, EpochNumber: 1, Amount: 100, PoolFee: 0, NumStakers: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertPayslip(t.Context(), db.InsertPayslipParams{BatchNumber: 1, Address: "A", Amount: 100, Status: "out_for_payment"}); err != nil {
		t.Fatal(err)
	}
	if err := q.SetPayslipsTransaction(t.Context(), db.SetPayslipsTransactionParams{TxHash: sql.NullString{String: "stuck1", Valid: true}, Address: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertTransaction(t.Context(), db.InsertTransactionParams{Hash: "stuck1", Address: "A", Amount: 100, Status: "awaiting_confirmation", SubmittedHeight: submitted}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertTransaction(t.Context(), db.InsertTransactionParams{Hash: "fresh1", Address: "B", Amount: 50, Status: "awaiting_confirmation", SubmittedHeight: head}); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "getBlockNumber":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":%d}`, head)))
		case "getTransactionByHash":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"not found"}}`))
		default:
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`))
		}
	}))
	defer srv.Close()
	client, err := rpc.New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{
		queries: q,
		chain:   &chain.Chain{RPC: client},
		cfg:     &config.Config{StuckPayoutEpochs: 3},
		policy:  &rpc.Policy{BlocksPerEpoch: blocksPerEpoch},
	}
	if err := m.runConfirmations(t.Context()); err != nil {
		t.Fatal(err)
	}

	pending, err := q.GetPendingTransactions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Hash != "fresh1" {
		t.Fatalf("pending = %+v, want only fresh1 (stuck1 failed)", pending)
	}
	payslips, err := q.GetPayslipsForAddress(t.Context(), "A")
	if err != nil {
		t.Fatal(err)
	}
	if len(payslips) != 1 || payslips[0].Status != "failed" {
		t.Fatalf("payslips = %+v, want one failed", payslips)
	}
}

func TestStuckFailedPayoutCanBeRetried(t *testing.T) {
	q := newLifecycleTestQueries(t)
	ctx := t.Context()
	if err := q.InsertReward(ctx, db.InsertRewardParams{BatchNumber: 1, EpochNumber: 1, Amount: 1000, PoolFee: 0, NumStakers: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertPayslip(ctx, db.InsertPayslipParams{BatchNumber: 1, Address: "A", Amount: 1000, Status: "out_for_payment"}); err != nil {
		t.Fatal(err)
	}
	if err := q.SetPayslipsTransaction(ctx, db.SetPayslipsTransactionParams{TxHash: sql.NullString{String: "h1", Valid: true}, Address: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertTransaction(ctx, db.InsertTransactionParams{Hash: "h1", Address: "A", Amount: 1000, Status: "awaiting_confirmation", SubmittedHeight: 1000}); err != nil {
		t.Fatal(err)
	}
	// Simulate the stuck-detection failure path.
	if err := q.SetTransactionStatus(ctx, db.SetTransactionStatusParams{Status: "failed", Hash: "h1"}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdatePayslipStatusFailed(ctx, sql.NullString{String: "h1", Valid: true}); err != nil {
		t.Fatal(err)
	}
	payslips, err := q.GetPayslipsForAddress(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	if len(payslips) != 1 || payslips[0].Status != "failed" {
		t.Fatalf("before retry payslips = %+v, want failed", payslips)
	}
	// Operator retries the failed payout group.
	if _, err := q.RetryFailedPayoutPayslips(ctx, sql.NullString{String: "h1", Valid: true}); err != nil {
		t.Fatal(err)
	}
	payslips, err = q.GetPayslipsForAddress(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	if len(payslips) != 1 || payslips[0].Status != "pending" || payslips[0].TxHash.Valid {
		t.Fatalf("after retry payslips = %+v, want pending with cleared tx", payslips)
	}
}

func TestStakerPreferenceUpsertAndGet(t *testing.T) {
	q := newLifecycleTestQueries(t)
	ctx := t.Context()

	if _, err := q.GetStakerPreference(ctx, "A"); err != sql.ErrNoRows {
		t.Fatalf("absent preference err = %v, want sql.ErrNoRows", err)
	}

	if err := q.UpsertStakerPreference(ctx, db.UpsertStakerPreferenceParams{Address: "A", Compound: 1}); err != nil {
		t.Fatal(err)
	}
	pref, err := q.GetStakerPreference(ctx, "A")
	if err != nil || pref != 1 {
		t.Fatalf("after upsert pref = %d, err = %v, want 1", pref, err)
	}

	if err := q.UpsertStakerPreference(ctx, db.UpsertStakerPreferenceParams{Address: "A", Compound: 0}); err != nil {
		t.Fatal(err)
	}
	pref, err = q.GetStakerPreference(ctx, "A")
	if err != nil || pref != 0 {
		t.Fatalf("after overwrite pref = %d, err = %v, want 0", pref, err)
	}
}
