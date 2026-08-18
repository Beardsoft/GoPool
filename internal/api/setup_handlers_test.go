package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Beardsoft/GoPool/internal/configstore"
)

func TestSetupCompletePersistsAlertSecrets(t *testing.T) {
	sum := sha256.Sum256([]byte("bootstrap"))
	q := newTestDB(t)
	store := configstore.New(filepath.Join(t.TempDir(), "config.json"), q)
	a := New(nil, q, nil, WithSetup(hex.EncodeToString(sum[:]), store))

	session := postJSON(t, a.Mux(), "/api/setup/session", map[string]string{"token": "bootstrap"}, nil)
	if session.Code != http.StatusOK {
		t.Fatalf("session: %d %s", session.Code, session.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range session.Result().Cookies() {
		if c.Name == setupCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no setup cookie")
	}

	draft := map[string]any{
		"rpc_url": "https://rpc-testnet.nimiqscan.com", "network": "test-albatross",
		"pool_fee_wallet": testAddr, "pool_fee_percentage": .01, "payout_mode": "delegate",
		"min_payout_luna": 100000, "api_addr": ":8080", "validator_address": testAddr, "pool_name": "GoPool",
		"alert_telegram_enabled": true, "alert_telegram_destination": "12345",
		"alert_telegram_token":  "tg-setup-token",
		"alert_webhook_enabled": true,
		"alert_webhook_url":     "https://discord.com/api/webhooks/123456/wh-setup-token",
	}
	rec := postJSON(t, a.Mux(), "/api/setup/complete", map[string]any{"settings": draft}, cookie)
	if rec.Code != http.StatusCreated {
		t.Fatalf("complete: %d %s", rec.Code, rec.Body.String())
	}
	raw, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tg-setup-token", "wh-setup-token"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config file missing %q", want)
		}
	}
}
