package api

import (
	"database/sql"
	"net/http"

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
		if !ok || addr.String() != a.cfg.ValidatorAddress {
			writeError(w, http.StatusForbidden, "operator only")
			return
		}
		next(w, r)
	})
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
	mux.HandleFunc("POST /api/operator/validator/deactivate", a.requireOperator(a.handleOperatorAction("deactivate")))
	mux.HandleFunc("POST /api/operator/validator/retire", a.requireOperator(a.handleOperatorAction("retire")))
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
		outstanding, err := a.queries.HasOutstandingValidatorAction(r.Context(), action)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "checking outstanding actions")
			return
		}
		if outstanding != 0 {
			writeError(w, http.StatusConflict, action+" is already requested or in flight")
			return
		}
		if err := a.queries.InsertValidatorAction(r.Context(), db.InsertValidatorActionParams{
			Action: action, TxHash: sql.NullString{}, Outcome: "requested",
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "queuing "+action)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "requested"})
	}
}
