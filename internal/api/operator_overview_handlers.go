package api

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Beardsoft/GoPool/internal/db"
)

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}

func (a *API) registerOperatorOverviewRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/operator/overview", a.requireOperator(a.handleOperatorOverview))
	mux.HandleFunc("GET /api/operator/readiness", a.requireOperator(a.handleOperatorReadiness))
	mux.HandleFunc("GET /api/operator/telemetry", a.requireOperator(a.handleOperatorTelemetry))
	mux.HandleFunc("GET /api/operator/activity", a.requireOperator(a.handleOperatorActivity))
	mux.HandleFunc("GET /api/operator/activity/export", a.requireOperator(a.handleOperatorActivityExport))
}

type operatorOverviewResponse struct {
	Status           string          `json:"status"`
	ChainLag         int64           `json:"chain_lag"`
	WalletRunwayDays *int            `json:"wallet_runway_days,omitempty"`
	Readiness        string          `json:"readiness"`
	PayoutSummary    json.RawMessage `json:"payout_summary"`
	ValidatorSummary json.RawMessage `json:"validator_summary"`
	Attention        []attentionItem `json:"attention"`
	Events           []eventSummary  `json:"events"`
}

type attentionItem struct {
	ID       int64  `json:"id"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	Summary  string `json:"summary"`
}

type eventSummary struct {
	ID       int64  `json:"id"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	Summary  string `json:"summary"`
	CreatedAt string `json:"created_at"`
}

func (a *API) handleOperatorOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	statusRow, err := a.queries.GetRuntimeStatus(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "loading runtime status")
		return
	}
	chainLag := int64(0)
	if statusRow.ChainHead >= int64(statusRow.LastProcessedHeight) {
		chainLag = int64(statusRow.ChainHead) - int64(statusRow.LastProcessedHeight)
	}
	// Determine overall status
	overall := "healthy"
	readiness := "ok"
	if statusRow.ReadinessError.Valid && statusRow.ReadinessError.String != "" {
		readiness = "error"
		overall = "attention"
	} else if statusRow.RpcOk == 0 {
		readiness = "degraded"
		overall = "attention"
	}
	// Attention from recent events
	eventsRows, _ := a.queries.ListOperatorEvents(ctx, db.ListOperatorEventsParams{Limit: 20, Offset: 0})
	attention := []attentionItem{}
	events := []eventSummary{}
	for _, e := range eventsRows {
		created := ""
		if e.CreatedAt.Valid {
			created = e.CreatedAt.Time.Format(time.RFC3339)
		}
		events = append(events, eventSummary{
			ID: e.ID, Severity: e.Severity, Category: e.Category, Summary: e.Summary, CreatedAt: created,
		})
		if e.Severity == "error" || e.Severity == "warning" {
			attention = append(attention, attentionItem{
				ID: e.ID, Severity: e.Severity, Category: e.Category, Summary: e.Summary,
			})
		}
	}
	if len(attention) > 0 {
		overall = "attention"
	}
	// Wallet runway (simplified)
	walletRunway := computeWalletRunway(ctx, a.queries)

	resp := operatorOverviewResponse{
		Status:           overall,
		ChainLag:         chainLag,
		WalletRunwayDays: walletRunway,
		Readiness:        readiness,
		PayoutSummary:    json.RawMessage(`{}`),
		ValidatorSummary: json.RawMessage(`{}`),
		Attention:        attention,
		Events:           events,
	}
	writeJSON(w, http.StatusOK, resp)
}

func computeWalletRunway(ctx interface{}, q *db.Queries) *int {
	// Simplified: return nil (no confirmed payouts)
	return nil
}

func (a *API) handleOperatorReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	statusRow, err := a.queries.GetRuntimeStatus(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "loading runtime status")
		return
	}
	resp := map[string]any{
		"rpc_ok": statusRow.RpcOk == 1,
		"readiness_error": nil,
		"validator_state": statusRow.ValidatorState,
		"chain_lag": int64(statusRow.ChainHead) - int64(statusRow.LastProcessedHeight),
	}
	if statusRow.ReadinessError.Valid {
		resp["readiness_error"] = statusRow.ReadinessError.String
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleOperatorTelemetry(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if metric == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_metric", "metric required")
		return
	}
	allowed := map[string]bool{
		"wallet_balance": true,
		"live_stake": true,
		"staker_count": true,
		"pending_payout_luna": true,
		"chain_lag": true,
	}
	if !allowed[metric] {
		writeAPIError(w, http.StatusBadRequest, "unknown_metric", "unknown metric")
		return
	}
	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_range", "invalid from")
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_range", "invalid to")
		return
	}
	if to.Sub(from) > 31*24*time.Hour {
		writeAPIError(w, http.StatusBadRequest, "range_too_large", "range >31 days")
		return
	}
	// Simplified telemetry: return empty points (max 500)
	writeJSON(w, http.StatusOK, []any{})
}

func (a *API) handleOperatorActivity(w http.ResponseWriter, r *http.Request) {
	severity := r.URL.Query().Get("severity")
	category := r.URL.Query().Get("category")
	cursorStr := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil {
			if v > 100 {
				v = 100
			}
			limit = v
		}
	}
	cursor := 0
	if cursorStr != "" {
		if v, err := strconv.Atoi(cursorStr); err == nil {
			cursor = v
		}
	}
	// Fetch events
	events, err := a.queries.ListOperatorEvents(r.Context(), db.ListOperatorEventsParams{Limit: int64(limit), Offset: int64(cursor)})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "loading events")
		return
	}
	// Simple filter
	filtered := []eventSummary{}
	for _, e := range events {
		if severity != "" && e.Severity != severity {
			continue
		}
		if category != "" && e.Category != category {
			continue
		}
		created := ""
		if e.CreatedAt.Valid {
			created = e.CreatedAt.Time.Format(time.RFC3339)
		}
		filtered = append(filtered, eventSummary{
			ID: e.ID, Severity: e.Severity, Category: e.Category, Summary: e.Summary, CreatedAt: created,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": filtered,
		"next_cursor": cursor + len(filtered),
	})
}

func (a *API) handleOperatorActivityExport(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	events, err := a.queries.ListOperatorEvents(r.Context(), db.ListOperatorEventsParams{Limit: 10000, Offset: 0})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "loading events")
		return
	}
	if format == "json" {
		writeJSON(w, http.StatusOK, events)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="activity.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id","severity","category","source","event_type","summary","created_at"})
	for _, e := range events {
		created := ""
		if e.CreatedAt.Valid {
			created = e.CreatedAt.Time.Format(time.RFC3339)
		}
		_ = cw.Write([]string{
			strconv.FormatInt(e.ID,10),
			e.Severity,
			e.Category,
			e.Source,
			e.EventType,
			e.Summary,
			created,
		})
	}
	cw.Flush()
}
