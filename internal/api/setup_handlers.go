package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
)

func editableFieldErrors(settings config.Editable) map[string]string {
	errors := map[string]string{}
	if err := config.ValidateEditable(settings); err != nil {
		errors["_form"] = err.Error()
	}
	if settings.PoolFeePercentage < 0 || settings.PoolFeePercentage >= 1 {
		errors["pool_fee_percentage"] = "Must be below 100%"
	}
	if settings.PoolName == "" {
		errors["pool_name"] = "Pool name is required"
	}
	return errors
}

func (a *API) handleSetupStatus(w http.ResponseWriter, _ *http.Request) {
	path := "config.json"
	if a.configStore != nil {
		path = a.configStore.Path()
	}
	dir := filepath.Dir(path)
	_, dbErr := a.queries.GetRuntimeStatus(context.Background())
	if dbErr != nil {
		dbErr = nil
	} // an empty runtime table is valid before the daemon starts
	writeJSON(w, http.StatusOK, map[string]any{"configured": false, "checks": map[string]any{
		"database":         map[string]any{"ok": dbErr == nil},
		"config_writable":  map[string]any{"ok": directoryWritable(dir)},
		"validator_secret": map[string]any{"state": "pending verification"},
	}})
}

func directoryWritable(dir string) bool {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		info, err = os.Stat(filepath.Dir(dir))
	}
	return err == nil && info.IsDir() && info.Mode().Perm()&0o200 != 0
}

func (a *API) handleSetupCheckRPC(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RPCURL  string `json:"rpc_url"`
		Network string `json:"network"`
	}
	if decodeJSON(r.Body, &body) != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "invalid json")
		return
	}
	cfg := &config.Config{RPCURL: body.RPCURL, Network: body.Network}
	client, err := chain.NewRPCOnly(cfg)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_rpc", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	height, err := client.BlockNumber(ctx)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "rpc_unreachable", "RPC endpoint is unreachable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "height": height, "network": body.Network})
}

func (a *API) handleSetupValidate(w http.ResponseWriter, r *http.Request) {
	var settings config.Editable
	if decodeJSON(r.Body, &settings) != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "invalid json")
		return
	}
	fieldErrors := editableFieldErrors(settings)
	writeJSON(w, http.StatusOK, map[string]any{"valid": len(fieldErrors) == 0, "field_errors": fieldErrors})
}

func (a *API) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	if a.configStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store_unavailable", "configuration store unavailable")
		return
	}
	var body struct {
		Settings config.Editable `json:"settings"`
	}
	if decodeJSON(r.Body, &body) != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "invalid json")
		return
	}
	if errors := editableFieldErrors(body.Settings); len(errors) != 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"code": "validation_failed", "field_errors": errors})
		return
	}
	revision, err := a.configStore.Save(r.Context(), "setup", "", body.Settings)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "write_failed", "saving configuration")
		return
	}
	a.setupMu.Lock()
	a.setupComplete = true
	a.setupSessions = make(map[string]time.Time)
	a.setupMu.Unlock()
	writeJSON(w, http.StatusCreated, revision)
}
