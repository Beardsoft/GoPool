package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
