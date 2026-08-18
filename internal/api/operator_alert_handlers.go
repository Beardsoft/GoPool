package api

import (
	"context"
	"database/sql"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/notifier"
)

// diskConfig returns the config as written to disk (what the daemon will load
// after a restart), falling back to the API's startup config.
func (a *API) diskConfig() *config.Config {
	if a.configStore != nil {
		if cfg, configured, err := config.LoadOptional(a.configStore.Path()); err == nil && configured {
			return cfg
		}
	}
	return a.cfg
}

type alertDeliveryRecorder struct{ q *db.Queries }

func (r alertDeliveryRecorder) RecordDelivery(ctx context.Context, result notifier.DeliveryResult) error {
	_, err := r.q.InsertAlertDelivery(ctx, db.InsertAlertDeliveryParams{
		Channel: result.Channel, AlertType: result.AlertType, Destination: result.Destination, State: result.State,
		ResponseSummary: sql.NullString{String: result.ResponseSummary, Valid: result.ResponseSummary != ""},
		CorrelationID:   sql.NullString{String: result.CorrelationID, Valid: result.CorrelationID != ""},
	})
	return err
}

func (a *API) registerOperatorAlertRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/operator/alerts", a.requireOperator(a.handleOperatorAlerts))
	mux.HandleFunc("POST /api/operator/alerts/", a.requireOperator(a.handleOperatorAlertTest))
	mux.HandleFunc("GET /api/operator/alerts/deliveries", a.requireOperator(a.handleOperatorAlertDeliveries))
}

func destinationHint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "configured"
	}
	return "…" + value[len(value)-4:]
}

func (a *API) handleOperatorAlerts(w http.ResponseWriter, _ *http.Request) {
	cfg := a.diskConfig()
	if cfg == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "config_unavailable", "loading configuration")
		return
	}
	telegramConfigured := cfg.AlertTelegramEnabled && cfg.AlertTelegramToken != ""
	webhookConfigured := cfg.AlertWebhookEnabled && cfg.AlertWebhookURL != ""
	writeJSON(w, http.StatusOK, map[string]any{"channels": map[string]any{
		"telegram": map[string]any{"enabled": telegramConfigured, "configured": telegramConfigured, "destination_hint": destinationHint(cfg.AlertTelegramDestination), "state": configuredState(telegramConfigured, false)},
		"webhook":  map[string]any{"enabled": webhookConfigured, "configured": webhookConfigured, "destination_hint": notifier.RedactWebhookURL(cfg.AlertWebhookURL), "state": configuredState(webhookConfigured, false)},
	}})
}

func configuredState(configured, unavailable bool) string {
	if !configured {
		return "missing"
	}
	if unavailable {
		return "unavailable"
	}
	return "configured"
}

func (a *API) handleOperatorAlertTest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/operator/alerts/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "test" {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown alert action")
		return
	}
	channel := parts[0]
	if channel != "telegram" && channel != "webhook" {
		writeAPIError(w, http.StatusBadRequest, "invalid_channel", "unknown alert channel")
		return
	}
	if a.alertTestAt == nil {
		a.alertTestAt = make(map[string]time.Time)
	}
	a.alertTestMu.Lock()
	last := a.alertTestAt[channel]
	if time.Since(last) < 30*time.Second {
		a.alertTestMu.Unlock()
		writeAPIError(w, http.StatusTooManyRequests, "rate_limited", "wait 30 seconds before testing this channel again")
		return
	}
	a.alertTestAt[channel] = time.Now()
	a.alertTestMu.Unlock()

	cfgCopy := *a.diskConfig()
	if channel != "telegram" {
		cfgCopy.AlertTelegramToken = ""
	}
	if channel != "webhook" {
		cfgCopy.AlertWebhookURL = ""
	}
	n := notifier.New(&cfgCopy, alertDeliveryRecorder{q: a.queries})
	correlationID := operationCorrelationID(r, "alert-test")
	results := n.Send(r.Context(), notifier.Alert{Level: "info", Type: "test", Title: "GoPool test alert", Message: "Operator requested a delivery test", CorrelationID: correlationID})
	if len(results) == 0 {
		writeAPIError(w, http.StatusConflict, "channel_not_configured", "alert channel is not configured")
		return
	}
	writeJSON(w, http.StatusOK, results[0])
}

func (a *API) handleOperatorAlertDeliveries(w http.ResponseWriter, r *http.Request) {
	cursor := int64(math.MaxInt64)
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			writeAPIError(w, http.StatusBadRequest, "invalid_cursor", "invalid cursor")
			return
		}
		cursor = parsed
	}
	limit := int64(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 || parsed > 100 {
			writeAPIError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	rows, err := a.queries.ListAlertDeliveries(r.Context(), db.ListAlertDeliveriesParams{ID: cursor, Limit: limit})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "loading alert deliveries")
		return
	}
	next := int64(0)
	if len(rows) == int(limit) && len(rows) > 0 {
		next = rows[len(rows)-1].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows, "next_cursor": next})
}
