package notifier

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// fakeSMTP speaks just enough SMTP to accept one message.
func fakeSMTP(t *testing.T) (host string, port int, received *string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ = strconv.Atoi(portStr)
	received = new(string)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		fmt.Fprintln(conn, "220 fake smtp")
		inData := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case inData:
				*received += line + "\n"
				if line == "." {
					inData = false
					fmt.Fprintln(conn, "250 queued")
				}
			case strings.HasPrefix(line, "EHLO"):
				fmt.Fprintln(conn, "250 fake")
			case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
				fmt.Fprintln(conn, "250 OK")
			case strings.HasPrefix(line, "DATA"):
				inData = true
				fmt.Fprintln(conn, "354 go ahead")
			case strings.HasPrefix(line, "QUIT"):
				fmt.Fprintln(conn, "221 bye")
				return
			}
		}
	}()
	return "127.0.0.1", port, received
}

func TestNotifierSendEmailOverSMTP(t *testing.T) {
	host, port, received := fakeSMTP(t)
	recorder := &fakeDeliveryRecorder{}
	n := New(&config.Config{AlertEmailEnabled: true, AlertEmailTo: "ops@example.com", AlertEmailSMTPHost: host, AlertEmailSMTPPort: port, AlertEmailFrom: "pool@example.com"}, recorder)
	got := n.Send(context.Background(), Alert{Level: "error", Type: "test", Title: "Test alert", Message: "Delivery check"})
	if len(got) != 1 || got[0].State != "sent" {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(*received, "Subject: [ERROR] Test alert") || !strings.Contains(*received, "Delivery check") {
		t.Fatalf("smtp server received: %s", *received)
	}
	if strings.Contains(recorder.last.Destination, "ops@") {
		t.Fatalf("destination not redacted: %s", recorder.last.Destination)
	}
}

func TestNotifierSendEmailWithoutHostFails(t *testing.T) {
	n := New(&config.Config{AlertEmailEnabled: true, AlertEmailTo: "ops@example.com", AlertEmailFrom: "pool@example.com"}, nil)
	got := n.Send(context.Background(), Alert{Level: "info", Type: "test", Title: "t", Message: "m"})
	if len(got) != 1 || got[0].State != "failed" || !strings.Contains(got[0].ResponseSummary, "smtp host") {
		t.Fatalf("%+v", got)
	}
}
