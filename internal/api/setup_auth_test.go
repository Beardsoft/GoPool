package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Beardsoft/GoPool/internal/configstore"
)

func TestSetupTokenExchangeDisabledAfterCompletion(t *testing.T) {
	sum := sha256.Sum256([]byte("bootstrap"))
	q := newTestDB(t)
	store := configstore.New(filepath.Join(t.TempDir(), "config.json"), q)
	a := New(nil, q, nil, WithSetup(hex.EncodeToString(sum[:]), store))
	a.setupComplete = true
	rec := postJSON(t, a.Mux(), "/api/setup/session", map[string]string{"token": "bootstrap"}, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
}

func TestSetupSessionCookieSecureBehindProxy(t *testing.T) {
	sum := sha256.Sum256([]byte("bootstrap"))
	q := newTestDB(t)
	store := configstore.New(filepath.Join(t.TempDir(), "config.json"), q)
	a := New(nil, q, nil, WithSetup(hex.EncodeToString(sum[:]), store))
	req := httptest.NewRequest(http.MethodPost, "/api/setup/session", strings.NewReader(`{"token":"bootstrap"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == setupCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no setup cookie")
	}
	if !cookie.Secure {
		t.Fatal("setup cookie must be Secure when X-Forwarded-Proto is https")
	}
}
