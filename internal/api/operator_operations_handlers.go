package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/pool"
)

func (a *API) registerOperatorOperationsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/operator/payouts", a.requireOperator(a.handleOperatorPayouts))
	mux.HandleFunc("GET /api/operator/actions", a.requireOperator(a.handleOperatorActions))
	mux.HandleFunc("POST /api/operator/actions", a.requireOperator(a.handleOperatorActionCreate))
	mux.HandleFunc("POST /api/operator/actions/", a.requireOperator(a.handleOperatorActionSub))
	mux.HandleFunc("POST /api/operator/payouts/", a.requireOperator(a.handleOperatorPayoutSub))
}

type operatorActionResponse struct {
	ID            int64   `json:"id"`
	Action        string  `json:"action"`
	State         string  `json:"state"`
	RequestedAt   string  `json:"requested_at,omitempty"`
	UpdatedAt     string  `json:"updated_at,omitempty"`
	TxHash        *string `json:"tx_hash,omitempty"`
	ErrorSummary  *string `json:"error_summary,omitempty"`
	CorrelationID *string `json:"correlation_id,omitempty"`
}

type operatorPayoutResponse struct {
	Hash            string `json:"hash"`
	Address         string `json:"address"`
	Amount          int64  `json:"amount"`
	Status          string `json:"status"`
	SubmittedAt     string `json:"submitted_at,omitempty"`
	SubmittedHeight int64  `json:"submitted_height"`
	Stuck           bool   `json:"stuck"`
	EpochFrom       *int64 `json:"epoch_from,omitempty"`
	EpochTo         *int64 `json:"epoch_to,omitempty"`
}

var errDuplicateValidatorAction = errors.New("validator action already requested or in flight")
var errInvalidPayoutRetry = errors.New("payout is not a failed group without an active transaction")

func nullableString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func nullableInt64(v any) *int64 {
	switch n := v.(type) {
	case nil:
		return nil
	case int64:
		return &n
	case int:
		x := int64(n)
		return &x
	case float64:
		x := int64(n)
		return &x
	default:
		return nil
	}
}

func actionResponseFromRow(row db.ListValidatorActionsRow) operatorActionResponse {
	response := operatorActionResponse{
		ID:            row.ID,
		Action:        row.Action,
		TxHash:        nullableString(row.TxHash),
		ErrorSummary:  nullableString(row.ErrorSummary),
		CorrelationID: nullableString(row.CorrelationID),
	}
	if row.State.Valid {
		response.State = row.State.String
	}
	if row.RequestedAt.Valid {
		response.RequestedAt = row.RequestedAt.Time.UTC().Format(time.RFC3339)
	}
	if row.UpdatedAt.Valid {
		response.UpdatedAt = row.UpdatedAt.Time.UTC().Format(time.RFC3339)
	}
	return response
}

func actionResponseFromModel(row db.ValidatorAction) operatorActionResponse {
	response := operatorActionResponse{
		ID:            row.ID,
		Action:        row.Action,
		State:         row.Outcome,
		TxHash:        nullableString(row.TxHash),
		ErrorSummary:  nullableString(row.ErrorSummary),
		CorrelationID: nullableString(row.CorrelationID),
	}
	if row.State.Valid {
		response.State = row.State.String
	}
	if row.RequestedAt.Valid {
		response.RequestedAt = row.RequestedAt.Time.UTC().Format(time.RFC3339)
	}
	if row.UpdatedAt.Valid {
		response.UpdatedAt = row.UpdatedAt.Time.UTC().Format(time.RFC3339)
	}
	return response
}

func operationCorrelationID(r *http.Request, prefix string) string {
	if correlationID := strings.TrimSpace(r.Header.Get("X-Correlation-ID")); correlationID != "" {
		return correlationID
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}

func operatorOperationEvent(eventType, summary, actor, correlationID string, eventCtx map[string]any) db.InsertOperatorEventParams {
	ctxJSON := ""
	if b, err := json.Marshal(eventCtx); err == nil {
		ctxJSON = string(b)
	}
	return db.InsertOperatorEventParams{
		Severity:      "info",
		Category:      "operator",
		Source:        "api",
		EventType:     eventType,
		Summary:       summary,
		ContextJson:   sql.NullString{String: ctxJSON, Valid: ctxJSON != ""},
		ActorAddress:  sql.NullString{String: actor, Valid: actor != ""},
		CorrelationID: sql.NullString{String: correlationID, Valid: correlationID != ""},
	}
}

func queueValidatorAction(ctx context.Context, q *db.Queries, action, actor, correlationID string) (db.ValidatorAction, error) {
	if action != "deactivate" && action != "retire" {
		return db.ValidatorAction{}, fmt.Errorf("unsupported validator action %q", action)
	}
	id, err := q.InsertRequestedValidatorAction(ctx, db.InsertRequestedValidatorActionParams{
		Action:        action,
		RequestedBy:   sql.NullString{String: actor, Valid: actor != ""},
		CorrelationID: sql.NullString{String: correlationID, Valid: correlationID != ""},
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return db.ValidatorAction{}, errDuplicateValidatorAction
		}
		return db.ValidatorAction{}, err
	}
	return q.GetValidatorAction(ctx, id)
}

func (a *API) queueAndAuditValidatorAction(ctx context.Context, action, actor, correlationID string) (db.ValidatorAction, error) {
	var queued db.ValidatorAction
	err := a.queries.InTx(ctx, func(q *db.Queries) error {
		var err error
		queued, err = queueValidatorAction(ctx, q, action, actor, correlationID)
		if err != nil {
			return err
		}
		_, err = q.InsertOperatorEvent(ctx, operatorOperationEvent("validator_action_requested", "Validator action requested", actor, correlationID, map[string]any{"action": action}))
		return err
	})
	return queued, err
}

func (a *API) handleOperatorActions(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	cursorStr := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	cursor := 0
	if cursorStr != "" {
		if v, err := strconv.Atoi(cursorStr); err == nil && v >= 0 {
			cursor = v
		}
	}
	ctx := r.Context()
	rows, err := a.queries.ListValidatorActions(ctx, db.ListValidatorActionsParams{
		Status: sql.NullString{String: status, Valid: status != ""},
		Limit:  int64(limit),
		Offset: int64(cursor),
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "loading actions")
		return
	}
	items := make([]operatorActionResponse, len(rows))
	for i, row := range rows {
		items[i] = actionResponseFromRow(row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": cursor + len(items),
	})
}

func (a *API) handleOperatorActionCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action        string `json:"action"`
		CorrelationID string `json:"correlation_id,omitempty"`
	}
	if err := decodeJSON(r.Body, &body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "invalid json")
		return
	}
	ctx := r.Context()
	addr, ok := addressFromContext(ctx)
	if !ok {
		writeAPIError(w, http.StatusForbidden, "operator_only", "operator only")
		return
	}
	correlationID := body.CorrelationID
	if correlationID == "" {
		correlationID = operationCorrelationID(r, "validator-action")
	}
	action, err := a.queueAndAuditValidatorAction(ctx, body.Action, addr.String(), correlationID)
	if errors.Is(err, errDuplicateValidatorAction) {
		writeAPIError(w, http.StatusConflict, "duplicate_action", "action already requested or in flight")
		return
	}
	if err != nil {
		if body.Action == "" || body.Action != "deactivate" && body.Action != "retire" {
			writeAPIError(w, http.StatusBadRequest, "invalid_action", "action must be deactivate or retire")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "db_error", "queueing action")
		return
	}
	writeJSON(w, http.StatusCreated, actionResponseFromModel(action))
}

func (a *API) handleOperatorActionSub(w http.ResponseWriter, r *http.Request) {
	// POST /api/operator/actions/{id}/cancel
	path := strings.TrimPrefix(r.URL.Path, "/api/operator/actions/")
	if path == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_path", "invalid path")
		return
	}
	parts := strings.Split(path, "/")
	idStr := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	if action == "cancel" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_id", "invalid id")
			return
		}
		ctx := r.Context()
		addr, _ := addressFromContext(ctx)
		correlationID := operationCorrelationID(r, "validator-action-cancel")
		var stored db.ValidatorAction
		err = a.queries.InTx(ctx, func(q *db.Queries) error {
			if _, err := q.CancelRequestedValidatorAction(ctx, id); err != nil {
				return err
			}
			var err error
			stored, err = q.GetValidatorAction(ctx, id)
			if err != nil {
				return err
			}
			if stored.CorrelationID.Valid {
				correlationID = stored.CorrelationID.String
			}
			_, err = q.InsertOperatorEvent(ctx, operatorOperationEvent("validator_action_cancelled", "Validator action cancelled", addr.String(), correlationID, map[string]any{"action_id": id, "action": stored.Action}))
			return err
		})
		if err != nil {
			if err == sql.ErrNoRows {
				writeAPIError(w, http.StatusConflict, "invalid_transition", "action cannot be cancelled")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "db_error", "cancelling action")
			return
		}
		writeJSON(w, http.StatusOK, actionResponseFromModel(stored))
		return
	}
	writeAPIError(w, http.StatusNotFound, "not_found", "unknown action")
}

func (a *API) handleOperatorPayouts(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	address := r.URL.Query().Get("address")
	epochStr := r.URL.Query().Get("epoch")
	cursorStr := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	cursor := 0
	if cursorStr != "" {
		if v, err := strconv.Atoi(cursorStr); err == nil && v >= 0 {
			cursor = v
		}
	}
	epoch := int64(0)
	hasEpoch := false
	if epochStr != "" {
		if v, err := strconv.ParseInt(epochStr, 10, 64); err == nil && v >= 0 {
			epoch = v
			hasEpoch = true
		}
	}
	ctx := r.Context()
	column1 := interface{}(nil)
	if status != "" {
		column1 = 1
	}
	column3 := interface{}(nil)
	if address != "" {
		column3 = 1
	}
	column5 := interface{}(nil)
	if hasEpoch {
		column5 = 1
	}
	// Fetch one extra row to detect whether another page exists.
	rows, err := a.queries.ListPayoutTransactions(ctx, db.ListPayoutTransactionsParams{
		Column1:     column1,
		Status:      status,
		Column3:     column3,
		Column4:     sql.NullString{String: address, Valid: true},
		Column5:     column5,
		EpochNumber: epoch,
		Limit:       int64(limit + 1),
		Offset:      int64(cursor),
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "loading payouts")
		return
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	var head uint32
	var policy *rpc.Policy
	if a.rpc != nil {
		head, _ = a.rpc.BlockNumber(ctx)
		policy, _ = a.rpc.GetPolicy(ctx)
	}
	now := time.Now()
	stuckEpochs := 0
	if a.cfg != nil {
		stuckEpochs = a.cfg.StuckPayoutEpochs
	}

	items := make([]operatorPayoutResponse, len(rows))
	for i, r := range rows {
		submittedAt := time.Time{}
		submittedAtStr := ""
		if r.SubmittedAt.Valid {
			submittedAt = r.SubmittedAt.Time
			submittedAtStr = submittedAt.UTC().Format(time.RFC3339)
		}
		stuck := false
		if policy != nil && r.Status != "completed" && r.Status != "failed" {
			stuck = pool.IsStuck(r.SubmittedHeight, head, stuckEpochs, policy.BlocksPerEpoch, policy.BlockSeparationTime, submittedAt, now)
		}
		items[i] = operatorPayoutResponse{
			Hash:            r.Hash,
			Address:         r.Address,
			Amount:          r.Amount,
			Status:          r.Status,
			SubmittedAt:     submittedAtStr,
			SubmittedHeight: r.SubmittedHeight,
			Stuck:           stuck,
			EpochFrom:       nullableInt64(r.EpochFrom),
			EpochTo:         nullableInt64(r.EpochTo),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": cursor + len(items),
		"has_more":    hasMore,
	})
}

func (a *API) handleOperatorPayoutSub(w http.ResponseWriter, r *http.Request) {
	// POST /api/operator/payouts/{id}/retry
	path := strings.TrimPrefix(r.URL.Path, "/api/operator/payouts/")
	if path == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_path", "invalid path")
		return
	}
	parts := strings.Split(path, "/")
	idStr := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	if action != "retry" {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown action")
		return
	}
	ctx := r.Context()
	addr, _ := addressFromContext(ctx)
	correlationID := operationCorrelationID(r, "payout-retry")
	var retried []int64
	err := a.queries.InTx(ctx, func(q *db.Queries) error {
		var err error
		retried, err = q.RetryFailedPayoutPayslips(ctx, sql.NullString{String: idStr, Valid: true})
		if err != nil {
			return err
		}
		if len(retried) == 0 {
			return errInvalidPayoutRetry
		}
		_, err = q.InsertOperatorEvent(ctx, operatorOperationEvent("payout_retry_requested", "Failed payout group returned to pending", addr.String(), correlationID, map[string]any{"txHash": idStr, "payslips_retried": len(retried)}))
		return err
	})
	if errors.Is(err, errInvalidPayoutRetry) {
		writeAPIError(w, http.StatusConflict, "invalid_retry", "payout is not a failed group without an active transaction")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "retrying payout")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hash":             idStr,
		"status":           "pending",
		"payslips_retried": len(retried),
		"correlation_id":   correlationID,
	})
}
