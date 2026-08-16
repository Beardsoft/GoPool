package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/pool"
)

// requireOperator wraps requireSession: only the address configured as
// cfg.ValidatorAddress may pass. There is no separate operator secret — this
// is "controls the validator's private key," the same identity the daemon
// itself already uses to sign transactions.
func (a *API) requireOperator(next http.HandlerFunc) http.HandlerFunc {
	return a.requireSession(func(w http.ResponseWriter, r *http.Request) {
		addr, ok := addressFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusForbidden, "operator only")
			return
		}
		addrStr := addr.String()
		if addrStr == a.cfg.ValidatorAddress || a.isOperatorAllowed(addrStr) {
			next(w, r)
			return
		}
		writeError(w, http.StatusForbidden, "operator only")
	})
}

func (a *API) isOperatorAllowed(addr string) bool {
	if a.cfg.OperatorAddresses == "" {
		return false
	}
	for _, op := range strings.Split(a.cfg.OperatorAddresses, ",") {
		if strings.TrimSpace(op) == addr {
			return true
		}
	}
	return false
}

type stuckPayslipResponse struct {
	BatchNumber int64  `json:"batch_number"`
	Address     string `json:"address"`
	AmountLuna  int64  `json:"amount_luna"`
	Status      string `json:"status"`
	TxHash      string `json:"tx_hash,omitempty"`
}

type healthResponse struct {
	LastProcessedHeight int64                  `json:"last_processed_height"`
	ChainHead           uint32                 `json:"chain_head"`
	StuckPayslips       []stuckPayslipResponse `json:"stuck_payslips"`
}

func (a *API) registerOperatorRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/operator/health", a.requireOperator(a.handleOperatorHealth))
	mux.HandleFunc("GET /api/operator/audit", a.requireOperator(a.handleAuditList))
	mux.HandleFunc("POST /api/operator/audit/approve", a.requireOperator(a.handleAuditApprove))
	mux.HandleFunc("POST /api/operator/audit/skip", a.requireOperator(a.handleAuditSkip))
	mux.HandleFunc("POST /api/operator/validator/deactivate", a.requireOperator(a.handleOperatorAction("deactivate")))
	mux.HandleFunc("POST /api/operator/validator/retire", a.requireOperator(a.handleOperatorAction("retire")))
	a.registerOperatorOverviewRoutes(mux)
	a.registerOperatorOperationsRoutes(mux)
	a.registerOperatorAlertRoutes(mux)
}

type auditLogResponse struct {
	ID         int64  `json:"id"`
	ActionType string `json:"action_type"`
	Address    string `json:"address"`
	Amount     int64  `json:"amount"`
	Fee        int64  `json:"fee"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	IntentData string `json:"intent_data,omitempty"`
	CreatedAt  string `json:"created_at"`
}

func (a *API) handleAuditList(w http.ResponseWriter, r *http.Request) {
	logs, err := a.queries.ListPendingAuditLogs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading audit logs")
		return
	}
	resp := make([]auditLogResponse, len(logs))
	for i, l := range logs {
		created := ""
		if l.CreatedAt.Valid {
			created = l.CreatedAt.Time.String()
		}
		intent := ""
		if l.IntentData.Valid {
			intent = l.IntentData.String
		}
		resp[i] = auditLogResponse{
			ID:         l.ID,
			ActionType: l.ActionType,
			Address:    l.Address,
			Amount:     l.Amount,
			Fee:        l.Fee,
			Kind:       l.Kind,
			Status:     l.Status,
			IntentData: intent,
			CreatedAt:  created,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleAuditApprove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int64 `json:"id"`
	}
	if err := decodeJSON(r.Body, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	updated, err := a.queries.ApprovePendingAuditLog(r.Context(), body.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "updating audit log")
		return
	}
	if updated != 1 {
		writeError(w, http.StatusConflict, "audit log is not pending")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (a *API) handleAuditSkip(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int64 `json:"id"`
	}
	if err := decodeJSON(r.Body, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := a.queries.UpdateAuditLogStatus(r.Context(), db.UpdateAuditLogStatusParams{
		Status: "skipped",
		ID:     body.ID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "updating audit log")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "skipped"})
}

func (a *API) handleOperatorHealth(w http.ResponseWriter, r *http.Request) {
	cursor, err := a.queries.GetCursor(r.Context(), pool.CursorName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading cursor")
		return
	}
	var head uint32
	if a.rpc != nil {
		head, err = a.rpc.BlockNumber(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, "querying chain head")
			return
		}
	}
	stuckRows, err := a.queries.GetStuckPayslips(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading stuck payslips")
		return
	}
	stuck := make([]stuckPayslipResponse, len(stuckRows))
	for i, s := range stuckRows {
		stuck[i] = stuckPayslipResponse{BatchNumber: s.BatchNumber, Address: s.Address, AmountLuna: s.Amount, Status: s.Status, TxHash: s.TxHash.String}
	}
	writeJSON(w, http.StatusOK, healthResponse{LastProcessedHeight: cursor, ChainHead: head, StuckPayslips: stuck})
}

// handleOperatorAction returns a handler that queues a deactivate/retire
// request for the daemon's ProcessRequestedActions to pick up and sign — the
// API itself never touches the validator's private key.
func (a *API) handleOperatorAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		addr, ok := addressFromContext(ctx)
		if !ok {
			writeError(w, http.StatusForbidden, "operator only")
			return
		}
		correlationID := operationCorrelationID(r, "validator-action")
		queued, err := a.queueAndAuditValidatorAction(ctx, action, addr.String(), correlationID)
		if errors.Is(err, errDuplicateValidatorAction) {
			writeError(w, http.StatusConflict, action+" is already requested or in flight")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "queuing "+action)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"id":     queued.ID,
			"status": queued.State.String,
		})
	}
}
