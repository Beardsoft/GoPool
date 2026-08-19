package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Beardsoft/GoPool/internal/configstore"
)

func setupSessionCookie(t *testing.T, mux http.Handler) *http.Cookie {
	t.Helper()
	session := postJSON(t, mux, "/api/setup/session", map[string]string{"token": "bootstrap"}, nil)
	if session.Code != http.StatusOK {
		t.Fatalf("session: %d %s", session.Code, session.Body.String())
	}
	for _, c := range session.Result().Cookies() {
		if c.Name == setupCookieName {
			return c
		}
	}
	t.Fatal("no setup cookie")
	return nil
}

func TestSetupStatusReturnsHintsFromFile(t *testing.T) {
	sum := sha256.Sum256([]byte("bootstrap"))
	dir := t.TempDir()
	hints := `{
  "validator_address": "` + testAddr + `",
  "pool_fee_wallet": "` + testAddr + `",
  "network": "main-albatross",
  "rpc_url": "https://rpc-mainnet.nimiqscan.com"
}`
	if err := os.WriteFile(filepath.Join(dir, "setup-hints.json"), []byte(hints), 0o644); err != nil {
		t.Fatal(err)
	}
	q := newTestDB(t)
	store := configstore.New(filepath.Join(dir, "config.json"), q)
	a := New(nil, q, nil, WithSetup(hex.EncodeToString(sum[:]), store))
	cookie := setupSessionCookie(t, a.Mux())

	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Hints map[string]string `json:"hints"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Hints["validator_address"] != testAddr {
		t.Fatalf("validator_address hint = %q, want %q", body.Hints["validator_address"], testAddr)
	}
	if body.Hints["network"] != "main-albatross" {
		t.Fatalf("network hint = %q", body.Hints["network"])
	}
	if body.Hints["rpc_url"] != "https://rpc-mainnet.nimiqscan.com" {
		t.Fatalf("rpc_url hint = %q", body.Hints["rpc_url"])
	}
	if body.Hints["pool_fee_wallet"] != testAddr {
		t.Fatalf("pool_fee_wallet hint = %q", body.Hints["pool_fee_wallet"])
	}
}

func TestSetupStatusOmitsHintsWhenFileMissing(t *testing.T) {
	sum := sha256.Sum256([]byte("bootstrap"))
	q := newTestDB(t)
	store := configstore.New(filepath.Join(t.TempDir(), "config.json"), q)
	a := New(nil, q, nil, WithSetup(hex.EncodeToString(sum[:]), store))
	cookie := setupSessionCookie(t, a.Mux())

	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"hints"`) {
		t.Fatalf("status included hints without a file: %s", rec.Body.String())
	}
}

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

func TestSetupCompleteActivatesPoolRoutes(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "session-secret")
	if err := os.WriteFile(secretPath, []byte("test-session-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POOL_SESSION_SECRET_FILE", secretPath)

	sum := sha256.Sum256([]byte("bootstrap"))
	q := newTestDB(t)
	store := configstore.New(filepath.Join(t.TempDir(), "config.json"), q)
	a := New(nil, q, nil, WithSetup(hex.EncodeToString(sum[:]), store))
	mux := a.Mux()

	session := postJSON(t, mux, "/api/setup/session", map[string]string{"token": "bootstrap"}, nil)
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
	}
	complete := postJSON(t, mux, "/api/setup/complete", map[string]any{"settings": draft}, cookie)
	if complete.Code != http.StatusCreated {
		t.Fatalf("complete: %d %s", complete.Code, complete.Body.String())
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/pool", nil))
	if rec.Code == http.StatusNotFound || rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("pool still unavailable after setup: %d %s", rec.Code, rec.Body.String())
	}

	home := httptest.NewRecorder()
	mux.ServeHTTP(home, httptest.NewRequest(http.MethodGet, "/", nil))
	if home.Code == http.StatusFound && home.Header().Get("Location") == "/setup" {
		t.Fatal("GET / still redirects to /setup after complete")
	}
}
