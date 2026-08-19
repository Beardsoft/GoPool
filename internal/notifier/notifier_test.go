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

func TestDiscordWebhookBodyIsEmbedNotRawJSON(t *testing.T) {
	body := webhookBody(Alert{
		Level: "error", Type: "payout_stuck", Title: "Payout stuck, marked failed",
		Message: "Unconfirmable for more than 3 epochs. Retry it from the operator console.",
		Fields: []AlertField{
			{Name: "Recipient", Value: "NQ95 HH5Q QT81 0VE5 V9SA LCNY CV37 K6Q6 XMPM"},
			{Name: "Tx", Value: "8e687a53f2e52f328609f4b9d3a412ae63094985bf6239ea1392022f58f5a922"},
		},
		Time: "2026-08-18T21:00:00Z",
	}, "https://discord.com/api/webhooks/123/token", "main-albatross", "GoPool")
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["username"] != "GoPool" {
		t.Fatalf("username: %s", body)
	}
	if _, ok := payload["title"]; ok {
		t.Fatalf("discord payload leaked raw alert fields: %s", body)
	}
	embeds, _ := payload["embeds"].([]any)
	if len(embeds) != 1 {
		t.Fatalf("embeds: %s", body)
	}
	embed, _ := embeds[0].(map[string]any)
	if embed["title"] != "Payout stuck, marked failed" {
		t.Fatalf("title: %s", body)
	}
	if int(embed["color"].(float64)) != discordColorError {
		t.Fatalf("error color want %d: %s", discordColorError, body)
	}
	fields, _ := embed["fields"].([]any)
	if len(fields) != 2 {
		t.Fatalf("fields: %s", body)
	}
	tx, _ := fields[1].(map[string]any)
	value, _ := tx["value"].(string)
	if !strings.Contains(value, "8e687a53…58f5a922") || !strings.Contains(value, "nimiqscan.com/transaction/") {
		t.Fatalf("tx field: %s", value)
	}
}

func TestDiscordEmbedColorAndFallbackTitle(t *testing.T) {
	if discordColor("error") != discordColorError {
		t.Fatal("error should be bright red")
	}
	if discordColor("warning") != discordColorWarning {
		t.Fatal("warning should be amber")
	}
	body := webhookBody(Alert{Level: "info"}, "https://discord.com/api/webhooks/123/token", "", "")
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	embed := payload["embeds"].([]any)[0].(map[string]any)
	if embed["title"] != "GoPool alert" {
		t.Fatalf("fallback title: %s", body)
	}
	long := webhookBody(Alert{Level: "error", Title: "t", Message: strings.Repeat("x", 5000)}, "https://discord.com/api/webhooks/123/token", "", "")
	var longPayload discordPayloadBody
	if err := json.Unmarshal(long, &longPayload); err != nil {
		t.Fatal(err)
	}
	if utf8.RuneCountInString(longPayload.Embeds[0].Description) > discordMaxDesc {
		t.Fatalf("description too long")
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
