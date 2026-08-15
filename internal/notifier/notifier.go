package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Beardsoft/GoPool/internal/config"
)

const maxResponseSummary = 512

type DeliveryResult struct {
	Channel         string    `json:"channel"`
	AlertType       string    `json:"alert_type"`
	Destination     string    `json:"destination"`
	State           string    `json:"state"`
	ResponseSummary string    `json:"response_summary,omitempty"`
	AttemptedAt     time.Time `json:"attempted_at"`
	CorrelationID   string    `json:"correlation_id,omitempty"`
}

type DeliveryRecorder interface {
	RecordDelivery(context.Context, DeliveryResult) error
}

type Option func(*Notifier)

func WithDeliveryRecorder(recorder DeliveryRecorder) Option {
	return func(n *Notifier) { n.recorder = recorder }
}
func WithHTTPClient(client *http.Client) Option { return func(n *Notifier) { n.client = client } }

type Notifier struct {
	cfg      *config.Config
	client   *http.Client
	recorder DeliveryRecorder
}

func New(cfg *config.Config, options ...Option) *Notifier {
	n := &Notifier{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
	for _, option := range options {
		option(n)
	}
	return n
}

type Alert struct {
	Level         string `json:"level"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Time          string `json:"time,omitempty"`
}

func (n *Notifier) Send(ctx context.Context, alert Alert) []DeliveryResult {
	if alert.Time == "" {
		alert.Time = time.Now().UTC().Format(time.RFC3339)
	}
	results := make([]DeliveryResult, 0, 3)
	if n.cfg.AlertTelegramToken != "" {
		results = append(results, n.sendTelegram(ctx, alert))
	}
	if n.cfg.AlertWebhookURL != "" {
		results = append(results, n.sendWebhook(ctx, alert))
	}
	if n.cfg.AlertEmailTo != "" {
		results = append(results, n.unavailableEmail(alert))
	}
	for _, result := range results {
		if n.recorder != nil {
			_ = n.recorder.RecordDelivery(ctx, result)
		}
	}
	return results
}

func (n *Notifier) result(alert Alert, channel, destination, state, summary string) DeliveryResult {
	return DeliveryResult{Channel: channel, AlertType: alert.Type, Destination: destination, State: state,
		ResponseSummary: capSummary(summary), AttemptedAt: time.Now().UTC(), CorrelationID: alert.CorrelationID}
}

func (n *Notifier) sendTelegram(ctx context.Context, alert Alert) DeliveryResult {
	chatID := getEnv("TELEGRAM_CHAT_ID", "")
	if chatID == "" {
		return n.result(alert, "telegram", "", "failed", "chat destination is not configured")
	}
	payload, _ := json.Marshal(map[string]string{"chat_id": chatID, "text": fmt.Sprintf("*%s*: %s\n%s", alert.Level, alert.Title, alert.Message), "parse_mode": "Markdown"})
	endpoint := "https://api.telegram.org/bot" + n.cfg.AlertTelegramToken + "/sendMessage"
	return n.sendHTTP(ctx, alert, "telegram", redactDestination(chatID), endpoint, payload)
}

func (n *Notifier) sendWebhook(ctx context.Context, alert Alert) DeliveryResult {
	body, _ := json.Marshal(alert)
	return n.sendHTTP(ctx, alert, "webhook", redactURL(n.cfg.AlertWebhookURL), n.cfg.AlertWebhookURL, body)
}

func (n *Notifier) sendHTTP(ctx context.Context, alert Alert, channel, destination, endpoint string, body []byte) DeliveryResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return n.result(alert, channel, destination, "failed", sanitizeSummary(err.Error(), n.cfg))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return n.result(alert, channel, destination, "failed", sanitizeSummary(err.Error(), n.cfg))
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSummary+1))
	summary := strings.TrimSpace(string(responseBody))
	if summary == "" {
		summary = resp.Status
	}
	summary = sanitizeSummary(summary, n.cfg)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return n.result(alert, channel, destination, "failed", summary)
	}
	return n.result(alert, channel, destination, "sent", summary)
}

func (n *Notifier) unavailableEmail(alert Alert) DeliveryResult {
	return n.result(alert, "email", redactDestination(n.cfg.AlertEmailTo), "unavailable", "email delivery is unavailable")
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "configured webhook"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func redactDestination(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "configured"
	}
	return "…" + value[len(value)-4:]
}

func sanitizeSummary(summary string, cfg *config.Config) string {
	secrets := []string{cfg.AlertTelegramToken, cfg.AlertWebhookURL, cfg.AlertEmailPassword}
	if webhook, err := url.Parse(cfg.AlertWebhookURL); err == nil {
		if webhook.User != nil {
			secrets = append(secrets, webhook.User.Username())
			if password, ok := webhook.User.Password(); ok {
				secrets = append(secrets, password)
			}
		}
		for _, values := range webhook.Query() {
			secrets = append(secrets, values...)
		}
	}
	for _, secret := range secrets {
		if secret != "" {
			summary = strings.ReplaceAll(summary, secret, "[redacted]")
		}
	}
	return capSummary(summary)
}

func capSummary(summary string) string {
	if len(summary) > maxResponseSummary {
		return summary[:maxResponseSummary]
	}
	return summary
}

func buildEmail(a Alert, from, to string) (subject, msg string) {
	subject = "[GoPool " + a.Level + "] " + a.Title
	body := a.Message + "\n\nLevel: " + a.Level + "\nTime:  " + a.Time + "\n"
	msg = "From: " + from + "\r\nTo: " + to + "\r\nSubject: " + subject + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body
	return subject, msg
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
