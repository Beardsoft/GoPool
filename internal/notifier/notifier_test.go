package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Beardsoft/GoPool/internal/config"
)

type fakeDeliveryRecorder struct{ last DeliveryResult }

func (r *fakeDeliveryRecorder) RecordDelivery(_ context.Context, result DeliveryResult) error {
	r.last = result
	return nil
}

func TestNotifierRecordsWebhookFailureWithoutSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rejected token secret", http.StatusInternalServerError)
	}))
	defer server.Close()
	recorder := &fakeDeliveryRecorder{}
	n := New(&config.Config{AlertWebhookEnabled: true, AlertWebhookURL: server.URL + "?token=secret"}, recorder)
	got := n.Send(context.Background(), Alert{Level: "error", Type: "test", Title: "Test alert", Message: "Delivery check"})
	if len(got) != 1 || got[0].State != "failed" || strings.Contains(recorder.last.Destination, "secret") || strings.Contains(recorder.last.ResponseSummary, "secret") {
		t.Fatalf("%+v", got)
	}
}

func TestNotifierTelegramRequiresConfigDestination(t *testing.T) {
	n := New(&config.Config{AlertTelegramEnabled: true, AlertTelegramToken: "token"}, nil)
	got := n.Send(context.Background(), Alert{Level: "info", Type: "test", Title: "t", Message: "m"})
	if len(got) != 1 || got[0].State != "failed" || !strings.Contains(got[0].ResponseSummary, "destination") {
		t.Fatalf("%+v", got)
	}
}

func TestDiscordWebhookBodyIsContentNotRawJSON(t *testing.T) {
	body := webhookBody(Alert{Level: "error", Type: "test", Title: "Test alert", Message: "Delivery check"}, "https://discord.com/api/webhooks/123/token")
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["content"] == "" {
		t.Fatalf("discord payload must carry content: %s", body)
	}
	if _, ok := payload["title"]; ok {
		t.Fatalf("discord payload leaked raw alert fields: %s", body)
	}
}

func TestDiscordContentIsNeverEmptyAndStaysUnderLimit(t *testing.T) {
	if strings.TrimSpace(discordContent(Alert{Level: "info"})) == "" {
		t.Fatal("discord content must not be empty")
	}
	long := discordContent(Alert{Level: "error", Title: "t", Message: strings.Repeat("x", 5000)})
	if count := utf8.RuneCountInString(long); count > discordMaxContent {
		t.Fatalf("content length %d exceeds discord limit", count)
	}
}

func TestRedactWebhookURLHidesDiscordPathToken(t *testing.T) {
	if got := RedactWebhookURL("https://discord.com/api/webhooks/123456/secret-token"); strings.Contains(got, "secret-token") {
		t.Fatal(got)
	}
	if got := RedactWebhookURL("https://hooks.example.com/hook?token=query-secret"); strings.Contains(got, "query-secret") {
		t.Fatal(got)
	}
}

func TestSanitizeSummaryRedactsDiscordPathToken(t *testing.T) {
	cfg := &config.Config{AlertWebhookURL: "https://discord.com/api/webhooks/123456/secret-token"}
	if got := sanitizeSummary("POST https://discord.com/api/webhooks/123456/secret-token 403 Forbidden", cfg); strings.Contains(got, "secret-token") {
		t.Fatal(got)
	}
}
