package notifier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	n := New(&config.Config{AlertWebhookURL: server.URL + "?token=secret"}, WithDeliveryRecorder(recorder))
	got := n.Send(context.Background(), Alert{Level: "error", Type: "test", Title: "Test alert", Message: "Delivery check"})
	if len(got) != 1 || got[0].State != "failed" || strings.Contains(recorder.last.Destination, "secret") || strings.Contains(recorder.last.ResponseSummary, "secret") {
		t.Fatalf("%+v", got)
	}
}

func TestBuildEmail(t *testing.T) {
	a := Alert{
		Level:   "warning",
		Title:   "Fee floor breach",
		Message: "Payout to NQ12... held",
		Time:    "2026-08-15T10:00:00Z",
	}
	subject, msg := buildEmail(a, "gopool@example.com", "ops@example.com")

	if subject != "[GoPool warning] Fee floor breach" {
		t.Errorf("subject = %q", subject)
	}
	for _, want := range []string{
		"From: gopool@example.com",
		"To: ops@example.com",
		"Subject: [GoPool warning] Fee floor breach",
		"Content-Type: text/plain; charset=UTF-8",
		"Payout to NQ12... held",
		"Level: warning",
		"Time:  2026-08-15T10:00:00Z",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n got: %s", want, msg)
		}
	}
	// Header/body separator must be present and use CRLF line endings.
	if !strings.Contains(msg, "\r\n\r\n") {
		t.Error("message missing blank CRLF header/body separator")
	}
}
