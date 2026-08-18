package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/configstore"
)

func validTestEditable() config.Editable {
	return config.Editable{RPCURL: "https://rpc-testnet.nimiqscan.com", Network: "test-albatross", PoolFeeWallet: testAddr, PoolFeePercentage: .01, PayoutMode: "delegate", MinPayoutLuna: 100_000, APIAddr: ":8080", ValidatorAddress: testAddr, PoolName: "GoPool"}
}

func TestSettingsSaveRequiresCurrentHash(t *testing.T) {
	a, cookie := configuredOperatorAPI(t)
	store := configstore.New(filepath.Join(t.TempDir(), "config.json"), a.queries)
	a.configStore = store
	settings := validTestEditable()
	if _, err := store.Save(t.Context(), testAddr, "", settings, config.AlertSecrets{}); err != nil {
		t.Fatal(err)
	}
	rec := putJSON(t, a.Mux(), "/api/operator/settings", map[string]any{"expected_hash": "stale", "settings": settings}, cookie)
	if rec.Code != http.StatusConflict {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsSavePersistsSecretsRedacted(t *testing.T) {
	a, cookie := configuredOperatorAPI(t)
	store := configstore.New(filepath.Join(t.TempDir(), "config.json"), a.queries)
	a.configStore = store
	rec := putJSON(t, a.Mux(), "/api/operator/settings", map[string]any{
		"expected_hash": "",
		"settings":      validTestEditable(),
		"secrets": map[string]any{
			"alert_telegram_token": "tg-secret-token",
			"alert_webhook_url":    "https://discord.com/api/webhooks/123456/wh-secret-token",
		},
	}, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
	raw, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tg-secret-token", "wh-secret-token"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config file missing %q", want)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/operator/settings", nil)
	req.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	a.Mux().ServeHTTP(getRec, req)
	body := getRec.Body.String()
	if strings.Contains(body, "tg-secret-token") || strings.Contains(body, "wh-secret-token") {
		t.Fatalf("settings response leaked a secret: %s", body)
	}
	for _, want := range []string{`"telegram_token":"configured"`, `"webhook_url":"configured"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings response missing %s", want)
		}
	}
}
