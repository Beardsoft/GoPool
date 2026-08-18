package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorAlertsNeverExposeSecrets(t *testing.T) {
	a, cookie := configuredOperatorAPI(t)
	a.cfg.AlertTelegramToken = "telegram-secret-value"
	a.cfg.AlertWebhookURL = "https://user:pass@example.com/hook?token=webhook-secret"
	req := httptest.NewRequest(http.MethodGet, "/api/operator/alerts", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, secret := range []string{"telegram-secret-value", "webhook-secret", "user:pass"} {
		if strings.Contains(body, secret) {
			t.Fatalf("response leaks %q: %s", secret, body)
		}
	}
	a.cfg.AlertWebhookURL = "https://discord.com/api/webhooks/123456/discord-path-token"
	rec2 := httptest.NewRecorder()
	a.Mux().ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec2.Code, rec2.Body.String())
	}
	if strings.Contains(rec2.Body.String(), "discord-path-token") {
		t.Fatalf("response leaks discord path token: %s", rec2.Body.String())
	}
}

func TestOperatorAlertTestIsRateLimited(t *testing.T) {
	a, cookie := configuredOperatorAPI(t)
	a.cfg.AlertWebhookEnabled = true
	a.cfg.AlertWebhookURL = "http://127.0.0.1:9/hook"
	first := postJSON(t, a.Mux(), "/api/operator/alerts/webhook/test", map[string]any{}, cookie)
	if first.Code != http.StatusOK {
		t.Fatalf("first status %d: %s", first.Code, first.Body.String())
	}
	second := postJSON(t, a.Mux(), "/api/operator/alerts/webhook/test", map[string]any{}, cookie)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status %d: %s", second.Code, second.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(first.Body.Bytes(), &body)
	if body["state"] != "failed" || !strings.Contains(body["response_summary"].(string), "connect") {
		t.Fatalf("state = %v", body["state"])
	}
}
