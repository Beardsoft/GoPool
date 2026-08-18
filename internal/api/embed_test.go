package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupOnlyAPI(t *testing.T) *API {
	t.Helper()
	sum := sha256.Sum256([]byte("bootstrap"))
	return New(nil, newTestDB(t), nil, WithSetup(hex.EncodeToString(sum[:]), nil))
}

func TestRegisterStaticRoutesServesSPA(t *testing.T) {
	a := &API{queries: newTestDB(t)}

	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<title>GoPool</title>") {
		t.Errorf("GET /: body missing GoPool title: %s", rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/epochs/5", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET /epochs/5: status = %d, body: %s", rec2.Code, rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), "<title>GoPool</title>") {
		t.Errorf("SPA fallback: body missing GoPool title: %s", rec2.Body.String())
	}
}

func TestRegisterStaticRoutesDoesNotServeSPAForUnknownAPI(t *testing.T) {
	a := &API{queries: newTestDB(t)}
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/missing", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("content type = %q, want application/json; body: %s", contentType, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("unknown API returned the SPA document: %s", rec.Body.String())
	}
}

func TestSetupOnlyRedirectsPublicPagesToSetup(t *testing.T) {
	a := setupOnlyAPI(t)
	for _, path := range []string{"/", "/epochs", "/operator"} {
		rec := httptest.NewRecorder()
		a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusFound {
			t.Fatalf("GET %s: status = %d, want 302; body: %s", path, rec.Code, rec.Body.String())
		}
		if loc := rec.Header().Get("Location"); loc != "/setup" {
			t.Fatalf("GET %s: Location = %q, want /setup", path, loc)
		}
	}
}

func TestSetupOnlyServesSetupWizard(t *testing.T) {
	a := setupOnlyAPI(t)
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /setup: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<title>GoPool</title>") {
		t.Errorf("GET /setup: body missing GoPool title: %s", rec.Body.String())
	}
}

func TestSetupOnlyUnknownAPIReturnsSetupRequired(t *testing.T) {
	a := setupOnlyAPI(t)
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/pool", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body: %s", err, rec.Body.String())
	}
	if body.Code != "setup_required" {
		t.Fatalf("code = %q, want setup_required", body.Code)
	}
}
