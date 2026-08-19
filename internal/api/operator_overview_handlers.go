package api

import (
	"context"
	"database/sql"
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
	mux.HandleFunc("GET /api/operator/events", a.requireOperator(a.handleOperatorEvents))
	mux.HandleFunc("GET /api/operator/overview", a.requireOperator(a.handleOperatorOverview))
	mux.HandleFunc("GET /api/operator/readiness", a.requireOperator(a.handleOperatorReadiness))
	mux.HandleFunc("GET /api/operator/telemetry", a.requireOperator(a.handleOperatorTelemetry))
	mux.HandleFunc("GET /api/operator/activity", a.requireOperator(a.handleOperatorActivity))
	mux.HandleFunc("GET /api/operator/activity/export", a.requireOperator(a.handleOperatorActivityExport))
}

type operatorOverviewResponse struct {
	Status             string                 `json:"status"`
	ChainLag           int64                  `json:"chain_lag"`
	WalletRunwayDays   *int                   `json:"wallet_runway_days,omitempty"`
	Readiness          string                 `json:"readiness"`
	PayoutSummary      json.RawMessage        `json:"payout_summary"`
	ValidatorSummary   validatorSummary       `json:"validator_summary"`
	Attention          []attentionItem        `json:"attention"`
	Events             []eventSummary         `json:"events"`
	EpochParticipation epochParticipationJSON `json:"epoch_participation"`
}

type validatorSummary struct {
	Address             string `json:"address"`
	State               string `json:"state"`
	LastProcessedHeight int64  `json:"last_processed_height"`
	LastTickMs          int64  `json:"last_tick_ms"`
}

type epochParticipationJSON struct {
	Epoch      *int64 `json:"epoch"`
	Elected    *bool  `json:"elected"`
	SlotCount  *int64 `json:"slot_count"`
	SlotsTotal *int64 `json:"slots_total"`
}

type attentionItem struct {
	ID       int64  `json:"id"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	Summary  string `json:"summary"`
}

type eventSummary struct {
	ID          int64  `json:"id"`
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Summary     string `json:"summary"`
	ContextJson string `json:"context_json,omitempty"`
	CreatedAt   string `json:"created_at"`
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
			ID: e.ID, Severity: e.Severity, Category: e.Category, Summary: e.Summary, ContextJson: e.ContextJson.String, CreatedAt: created,
		})
		// Readiness events are transient by design (the daemon retries until
		// the validator is staked); the current state is the readiness field,
		// so they stay in the event feed without flagging attention.
		if (e.Severity == "error" || e.Severity == "warning") && e.Category != "readiness" {
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
		ValidatorSummary: validatorSummary{
			Address:             statusRow.DerivedValidatorAddress,
			State:               statusRow.ValidatorState,
			LastProcessedHeight: statusRow.LastProcessedHeight,
			LastTickMs:          statusRow.LastTickMs,
		},
		Attention:          attention,
		Events:             events,
		EpochParticipation: epochParticipationFromStatus(statusRow),
	}
	writeJSON(w, http.StatusOK, resp)
}

func epochParticipationFromStatus(row db.RuntimeStatus) epochParticipationJSON {
	out := epochParticipationJSON{}
	if row.EpochNumber.Valid {
		v := row.EpochNumber.Int64
		out.Epoch = &v
	}
	if row.EpochElected.Valid {
		v := row.EpochElected.Int64 != 0
		out.Elected = &v
	}
	if row.SlotCount.Valid {
		v := row.SlotCount.Int64
		out.SlotCount = &v
	}
	if row.SlotsTotal.Valid {
		v := row.SlotsTotal.Int64
		out.SlotsTotal = &v
	}
	return out
}

// computeWalletRunway estimates how many days the payout wallet can keep
// paying out at its recent pace: latest wallet balance divided by the
// greater of one luna or the average confirmed payout per day over the
// previous 30 complete UTC days. Returns nil when there is no recorded
// balance or no confirmed payouts in the window.
func computeWalletRunway(ctx context.Context, q *db.Queries) *int {
	balance, err := q.GetLatestWalletBalance(ctx)
	if err != nil {
		return nil
	}
	windowStart := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -30)
	total, err := q.SumCompletedPayoutsSince(ctx, sql.NullTime{Time: windowStart, Valid: true})
	if err != nil || total == 0 {
		return nil
	}
	avgDaily := float64(total) / 30
	if avgDaily < 1 {
		avgDaily = 1
	}
	days := int(float64(balance) / avgDaily)
	return &days
}

func (a *API) handleOperatorReadiness(w http.ResponseWriter, r *http.Request) {
	a.writeReadiness(w, r)
}

func (a *API) handlePublicReadiness(w http.ResponseWriter, r *http.Request) {
	a.writeReadiness(w, r)
}

func (a *API) writeReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	statusRow, err := a.queries.GetRuntimeStatus(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"rpc_ok": false, "readiness_error": "Waiting for daemon heartbeat", "validator_state": "",
		})
		return
	}
	resp := map[string]any{
		"rpc_ok":          statusRow.RpcOk == 1,
		"readiness_error": nil,
		"validator_state": statusRow.ValidatorState,
		"chain_lag":       int64(statusRow.ChainHead) - int64(statusRow.LastProcessedHeight),
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
		"wallet_balance":      true,
		"live_stake":          true,
		"staker_count":        true,
		"pending_payout_luna": true,
		"chain_lag":           true,
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
	bucket := time.Duration(0)
	if bucketStr := r.URL.Query().Get("bucket"); bucketStr != "" {
		bucket, err = time.ParseDuration(bucketStr)
		if err != nil || bucket <= 0 {
			writeAPIError(w, http.StatusBadRequest, "invalid_bucket", "invalid bucket")
			return
		}
	}
	rows, err := a.queries.ListHealthSnapshotsBetween(r.Context(), db.ListHealthSnapshotsBetweenParams{RecordedAt: from, RecordedAt_2: to})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "loading telemetry")
		return
	}
	points := make([]telemetryPoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, telemetryPoint{t: row.RecordedAt, value: metricValue(metric, row)})
	}
	if bucket > 0 {
		points = bucketize(points, bucket)
	}
	if n := len(points); n > 500 {
		out := make([]telemetryPoint, 500)
		for i := range out {
			out[i] = points[i*n/500]
		}
		points = out
	}
	resp := make([]map[string]any, 0, len(points))
	for _, p := range points {
		resp = append(resp, map[string]any{"ts": p.t.Format(time.RFC3339), "value": p.value})
	}
	writeJSON(w, http.StatusOK, resp)
}

type telemetryPoint struct {
	t     time.Time
	value float64
}

func metricValue(metric string, row db.ListHealthSnapshotsBetweenRow) float64 {
	switch metric {
	case "chain_lag":
		if row.ChainHead >= row.ProcessedHeight {
			return float64(row.ChainHead - row.ProcessedHeight)
		}
		return 0
	case "wallet_balance":
		return float64(row.WalletBalance)
	case "live_stake":
		return float64(row.LiveStake)
	case "staker_count":
		return float64(row.StakerCount)
	default:
		return float64(row.PendingPayoutLuna)
	}
}

// bucketize collapses ordered points into fixed buckets, keeping the last
// sample of each bucket (gauge semantics) and the bucket start as the label.
func bucketize(points []telemetryPoint, bucket time.Duration) []telemetryPoint {
	out := make([]telemetryPoint, 0, len(points))
	for i := range points {
		start := points[i].t.Truncate(bucket)
		if n := len(out); n > 0 && out[n-1].t.Equal(start) {
			out[n-1] = telemetryPoint{t: start, value: points[i].value}
			continue
		}
		out = append(out, telemetryPoint{t: start, value: points[i].value})
	}
	return out
}

func (a *API) handleOperatorActivity(w http.ResponseWriter, r *http.Request) {
	severity := r.URL.Query().Get("severity")
	category := r.URL.Query().Get("category")
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
	// Fetch one extra row to detect whether another page exists.
	events, err := a.queries.ListOperatorEvents(r.Context(), db.ListOperatorEventsParams{Limit: int64(limit + 1), Offset: int64(cursor)})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "loading events")
		return
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
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
			ID: e.ID, Severity: e.Severity, Category: e.Category, Summary: e.Summary, ContextJson: e.ContextJson.String, CreatedAt: created,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       filtered,
		"next_cursor": cursor + len(events),
		"has_more":    hasMore,
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
	_ = cw.Write([]string{"id", "severity", "category", "source", "event_type", "summary", "created_at"})
	for _, e := range events {
		created := ""
		if e.CreatedAt.Valid {
			created = e.CreatedAt.Time.Format(time.RFC3339)
		}
		_ = cw.Write([]string{
			strconv.FormatInt(e.ID, 10),
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
