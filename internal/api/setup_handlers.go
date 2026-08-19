package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	hints := loadSetupHints(dir)
	body := map[string]any{"configured": false, "checks": map[string]any{
		"database":         map[string]any{"ok": dbErr == nil},
		"config_writable":  map[string]any{"ok": directoryWritable(dir)},
		"validator_secret": validatorSecretCheck(hints),
	}}
	if hints != nil {
		body["hints"] = hints
	}
	writeJSON(w, http.StatusOK, body)
}

func validatorSecretCheck(hints map[string]string) map[string]any {
	if hints != nil && strings.TrimSpace(hints["validator_address"]) != "" {
		return map[string]any{"ok": true, "state": "present"}
	}
	return map[string]any{"ok": false, "state": "missing"}
}

type setupHints struct {
	ValidatorAddress string `json:"validator_address,omitempty"`
	PoolFeeWallet    string `json:"pool_fee_wallet,omitempty"`
	Network          string `json:"network,omitempty"`
	RPCURL           string `json:"rpc_url,omitempty"`
}

func loadSetupHints(dir string) map[string]string {
	raw, err := os.ReadFile(filepath.Join(dir, "setup-hints.json"))
	if err != nil {
		return nil
	}
	var hints setupHints
	if json.Unmarshal(raw, &hints) != nil {
		return nil
	}
	out := map[string]string{}
	if hints.ValidatorAddress != "" {
		out["validator_address"] = hints.ValidatorAddress
	}
	if hints.PoolFeeWallet != "" {
		out["pool_fee_wallet"] = hints.PoolFeeWallet
	}
	if hints.Network != "" {
		out["network"] = hints.Network
	}
	if hints.RPCURL != "" {
		out["rpc_url"] = hints.RPCURL
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

// setupDraft decodes the flat setup form: editable settings plus alert secrets.
type setupDraft struct {
	config.Editable
	config.AlertSecrets
}

func (a *API) handleSetupValidate(w http.ResponseWriter, r *http.Request) {
	var draft setupDraft
	if decodeJSON(r.Body, &draft) != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "invalid json")
		return
	}
	fieldErrors := editableFieldErrors(draft.Editable)
	writeJSON(w, http.StatusOK, map[string]any{"valid": len(fieldErrors) == 0, "field_errors": fieldErrors})
}

func (a *API) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	if a.configStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store_unavailable", "configuration store unavailable")
		return
	}
	var body struct {
		Settings setupDraft `json:"settings"`
	}
	if decodeJSON(r.Body, &body) != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "invalid json")
		return
	}
	if errors := editableFieldErrors(body.Settings.Editable); len(errors) != 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"code": "validation_failed", "field_errors": errors})
		return
	}
	revision, err := a.configStore.Save(r.Context(), "setup", "", body.Settings.Editable, body.Settings.AlertSecrets)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "write_failed", "saving configuration")
		return
	}
	a.setupMu.Lock()
	a.setupComplete = true
	a.setupSessions = make(map[string]time.Time)
	a.setupMu.Unlock()
	_ = a.tryActivateFromStore()
	writeJSON(w, http.StatusCreated, revision)
}

func (a *API) tryActivateFromStore() error {
	if a.configStore == nil {
		return nil
	}
	cfg, configured, err := config.LoadOptional(a.configStore.Path())
	if err != nil {
		return err
	}
	if !configured {
		return nil
	}
	if path := os.Getenv("POOL_SESSION_SECRET_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		cfg.SessionSecret = strings.TrimSpace(string(data))
	}
	if err := config.ValidateAPI(cfg); err != nil {
		return err
	}
	rpcClient, err := chain.NewRPCOnly(cfg)
	if err != nil {
		return err
	}
	a.ActivateConfigured(cfg, rpcClient)
	return nil
}
