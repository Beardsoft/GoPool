package notifier

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strconv"
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

type Notifier struct {
	cfg      *config.Config
	client   *http.Client
	recorder DeliveryRecorder
}

// New builds a Notifier. recorder may be nil to skip delivery recording.
func New(cfg *config.Config, recorder DeliveryRecorder) *Notifier {
	return &Notifier{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}, recorder: recorder}
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
	if n.cfg.AlertTelegramEnabled && n.cfg.AlertTelegramToken != "" {
		results = append(results, n.sendTelegram(ctx, alert))
	}
	if n.cfg.AlertWebhookEnabled && n.cfg.AlertWebhookURL != "" {
		results = append(results, n.sendWebhook(ctx, alert))
	}
	if n.cfg.AlertEmailEnabled && n.cfg.AlertEmailTo != "" {
		results = append(results, n.sendEmail(ctx, alert))
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
	chatID := strings.TrimSpace(n.cfg.AlertTelegramDestination)
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

func (n *Notifier) sendEmail(ctx context.Context, alert Alert) DeliveryResult {
	destination := redactDestination(n.cfg.AlertEmailTo)
	fail := func(err error) DeliveryResult {
		return n.result(alert, "email", destination, "failed", sanitizeSummary(err.Error(), n.cfg))
	}
	from := n.cfg.AlertEmailFrom
	if from == "" {
		from = n.cfg.AlertEmailUsername
	}
	if from == "" {
		return n.result(alert, "email", destination, "failed", "smtp from address is not configured")
	}
	host := n.cfg.AlertEmailSMTPHost
	if host == "" {
		return n.result(alert, "email", destination, "failed", "smtp host is not configured")
	}
	port := n.cfg.AlertEmailSMTPPort
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var client *smtp.Client
	if port == 465 {
		dialer := net.Dialer{Timeout: 10 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fail(err)
		}
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fail(err)
		}
		client, err = smtp.NewClient(tlsConn, host)
		if err != nil {
			return fail(err)
		}
	} else {
		var err error
		client, err = smtp.Dial(addr)
		if err != nil {
			return fail(err)
		}
		if port == 587 {
			if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return fail(err)
			}
		}
	}
	defer client.Close()
	if n.cfg.AlertEmailUsername != "" {
		if err := client.Auth(smtp.PlainAuth("", n.cfg.AlertEmailUsername, n.cfg.AlertEmailPassword, host)); err != nil {
			return fail(err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fail(err)
	}
	if err := client.Rcpt(n.cfg.AlertEmailTo); err != nil {
		return fail(err)
	}
	w, err := client.Data()
	if err != nil {
		return fail(err)
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: [%s] %s\r\n\r\n%s\r\n", from, n.cfg.AlertEmailTo, strings.ToUpper(alert.Level), alert.Title, alert.Message)
	if _, err := w.Write([]byte(message)); err != nil {
		return fail(err)
	}
	if err := w.Close(); err != nil {
		return fail(err)
	}
	return n.result(alert, "email", destination, "sent", "message accepted by smtp server")
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
