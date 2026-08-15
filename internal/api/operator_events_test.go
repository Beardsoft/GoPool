package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOperatorEventsRequiresSession(t *testing.T) {
	a, _, _ := operatorTestAPI(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operator/events", nil)

	a.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}
