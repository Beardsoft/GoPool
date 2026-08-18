package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/configstore"
)

func (a *API) registerOperatorSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/operator/settings", a.requireOperator(a.handleOperatorSettings))
	mux.HandleFunc("POST /api/operator/settings/validate", a.requireOperator(a.handleOperatorSettingsValidate))
	mux.HandleFunc("PUT /api/operator/settings", a.requireOperator(a.handleOperatorSettingsSave))
	mux.HandleFunc("GET /api/operator/settings/revisions", a.requireOperator(a.handleOperatorSettingsRevisions))
	mux.HandleFunc("POST /api/operator/settings/revisions/", a.requireOperator(a.handleOperatorSettingsRestore))
}

func (a *API) handleOperatorSettings(w http.ResponseWriter, r *http.Request) {
	if a.configStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store_unavailable", "configuration store unavailable")
		return
	}
	cfg, configured, err := config.LoadOptional(a.configStore.Path())
	if err != nil || !configured {
		writeAPIError(w, http.StatusInternalServerError, "config_unavailable", "loading configuration")
		return
	}
	hash := config.ConfigHash(cfg.Editable(), cfg.AlertSecrets())
	runtimeHash := ""
	validatorPresent := false
	if runtime, err := a.queries.GetRuntimeStatus(r.Context()); err == nil {
		runtimeHash = runtime.ConfigHash
		validatorPresent = runtime.DerivedValidatorAddress != ""
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "loading daemon state")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active_hash": hash, "daemon_hash": runtimeHash, "restart_required": runtimeHash != hash,
		"settings": cfg.Editable(), "secrets": map[string]string{"validator_key": presenceState(validatorPresent), "session_secret": presenceState(a.cfg != nil && a.cfg.SessionSecret != ""),
			"telegram_token": presenceState(cfg.AlertTelegramToken != ""), "webhook_url": presenceState(cfg.AlertWebhookURL != "")}})
}

func presenceState(present bool) string {
	if present {
		return "configured"
	}
	return "missing"
}

func (a *API) handleOperatorSettingsValidate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Settings config.Editable `json:"settings"`
	}
	if decodeJSON(r.Body, &body) != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "invalid json")
		return
	}
	errors := editableFieldErrors(body.Settings)
	writeJSON(w, http.StatusOK, map[string]any{"valid": len(errors) == 0, "field_errors": errors})
}

func (a *API) handleOperatorSettingsSave(w http.ResponseWriter, r *http.Request) {
	if a.configStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store_unavailable", "configuration store unavailable")
		return
	}
	var body struct {
		ExpectedHash string              `json:"expected_hash"`
		Settings     config.Editable     `json:"settings"`
		Secrets      config.AlertSecrets `json:"secrets"`
	}
	if decodeJSON(r.Body, &body) != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "invalid json")
		return
	}
	addr, _ := addressFromContext(r.Context())
	revision, err := a.configStore.Save(r.Context(), addr.String(), body.ExpectedHash, body.Settings, body.Secrets)
	if errors.Is(err, configstore.ErrRevisionConflict) {
		writeAPIError(w, http.StatusConflict, "revision_conflict", "settings changed; reload before saving")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error())
		return
	}
	a.applyLiveConfig()
	writeJSON(w, http.StatusOK, revision)
}

func (a *API) handleOperatorSettingsRevisions(w http.ResponseWriter, r *http.Request) {
	if a.configStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store_unavailable", "configuration store unavailable")
		return
	}
	revisions, err := a.configStore.ListRevisions(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "db_error", "loading revisions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": revisions})
}

func (a *API) handleOperatorSettingsRestore(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/operator/settings/revisions/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "restore" {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown revision action")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_id", "invalid revision id")
		return
	}
	var body struct {
		ExpectedHash string `json:"expected_hash"`
	}
	if decodeJSON(r.Body, &body) != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "invalid json")
		return
	}
	addr, _ := addressFromContext(r.Context())
	revision, err := a.configStore.Restore(r.Context(), addr.String(), body.ExpectedHash, id)
	if errors.Is(err, configstore.ErrRevisionConflict) {
		writeAPIError(w, http.StatusConflict, "revision_conflict", "settings changed; reload before restoring")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, "restore_failed", err.Error())
		return
	}
	a.applyLiveConfig()
	writeJSON(w, http.StatusOK, revision)
}

func (a *API) applyLiveConfig() {
	if a.configStore == nil {
		return
	}
	cfg, configured, err := config.LoadOptional(a.configStore.Path())
	if err != nil || !configured || cfg == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg != nil {
		cfg.SessionSecret = a.cfg.SessionSecret
	}
	rpcChanged := a.cfg == nil || a.cfg.RPCURL != cfg.RPCURL || a.cfg.Network != cfg.Network
	a.cfg = cfg
	if rpcChanged {
		if rpcClient, err := chain.NewRPCOnly(cfg); err == nil {
			a.rpc = rpcClient
		}
	}
}
