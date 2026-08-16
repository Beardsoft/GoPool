package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/signer"

	"github.com/Beardsoft/GoPool/internal/config"
)

func TestAuthChallengeVerifyRoundTrip(t *testing.T) {
	key, err := signer.Generate()
	if err != nil {
		t.Fatal(err)
	}
	addr := key.Address()

	a := &API{queries: newTestDB(t), cfg: &config.Config{SessionSecret: "test-secret"}, nonces: newNonceStore()}

	// 1. challenge
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/challenge", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("challenge status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var ch challengeResponse
	if err := json.NewDecoder(rec.Body).Decode(&ch); err != nil {
		t.Fatal(err)
	}

	// 2. sign it exactly as Hub's signMessage would
	ctx := context.Background()
	signed, err := nimiq.SignMessage(ctx, key, []byte(ch.Challenge))
	if err != nil {
		t.Fatal(err)
	}

	// 3. verify
	body := strings.NewReader(`{"nonce":"` + ch.Nonce + `","address":"` + addr.String() + `","public_key":"` +
		base64.StdEncoding.EncodeToString(signed.PublicKey) + `","signature":"` +
		base64.StdEncoding.EncodeToString(signed.Signature) + `"}`)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/verify", body)
	req2.Header.Set("Content-Type", "application/json")
	a.Mux().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("verify status = %d, body: %s", rec2.Code, rec2.Body.String())
	}
	cookies := rec2.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName {
		t.Fatalf("expected one %s cookie, got %+v", sessionCookieName, cookies)
	}

	// 4. reuse fails (single-use nonce)
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/api/auth/verify", strings.NewReader(`{"nonce":"`+ch.Nonce+`","address":"`+addr.String()+`","public_key":"`+
		base64.StdEncoding.EncodeToString(signed.PublicKey)+`","signature":"`+base64.StdEncoding.EncodeToString(signed.Signature)+`"}`))
	req3.Header.Set("Content-Type", "application/json")
	a.Mux().ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusBadRequest {
		t.Errorf("nonce reuse: status = %d, want 400", rec3.Code)
	}

	// 5. the session cookie authenticates requireSession
	protected := a.requireSession(func(w http.ResponseWriter, r *http.Request) {
		got, ok := addressFromContext(r.Context())
		if !ok || got != addr {
			t.Errorf("addressFromContext = %v, %v; want %v, true", got, ok, addr)
		}
		w.WriteHeader(http.StatusOK)
	})
	rec4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req4.AddCookie(cookies[0])
	protected(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Errorf("requireSession: status = %d, want 200", rec4.Code)
	}
}

func TestSessionEndpoint(t *testing.T) {
	a, operatorCookie, stakerCookie := operatorTestAPI(t)

	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("signed out: status = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(operatorCookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("operator: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var body sessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Address != testAddr || !body.Operator {
		t.Errorf("operator session = %+v, want address %s operator true", body, testAddr)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(stakerCookie)
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("staker: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	body = sessionResponse{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Operator {
		t.Errorf("staker session = %+v, want operator false", body)
	}
}

func TestAuthLogoutClearsCookie(t *testing.T) {
	a := &API{cfg: &config.Config{SessionSecret: "test-secret"}}
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].MaxAge != -1 {
		t.Fatalf("cookies = %+v, want expired %s cookie", cookies, sessionCookieName)
	}
}

func TestRequireSessionRejectsMissingCookie(t *testing.T) {
	a := &API{cfg: &config.Config{SessionSecret: "test-secret"}}
	protected := a.requireSession(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	rec := httptest.NewRecorder()
	protected(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
