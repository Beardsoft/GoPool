package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Beardsoft/GoPool/internal/config"
)

const maxResponseSummary = 512

const discordMaxContent = 2000

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
	results := make([]DeliveryResult, 0, 2)
	if n.cfg.AlertTelegramEnabled && n.cfg.AlertTelegramToken != "" {
		results = append(results, n.sendTelegram(ctx, alert))
	}
	if n.cfg.AlertWebhookEnabled && n.cfg.AlertWebhookURL != "" {
		results = append(results, n.sendWebhook(ctx, alert))
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
	return n.sendHTTP(ctx, alert, "webhook", RedactWebhookURL(n.cfg.AlertWebhookURL), n.cfg.AlertWebhookURL, webhookBody(alert, n.cfg.AlertWebhookURL))
}

// webhookBody shapes the payload per target: Discord webhooks require a
// non-empty "content" field (long URLs there are auto-truncated by Discord),
// anything else receives the raw alert JSON.
func webhookBody(alert Alert, rawURL string) []byte {
	if isDiscordWebhook(rawURL) {
		body, _ := json.Marshal(map[string]string{"content": discordContent(alert)})
		return body
	}
	body, _ := json.Marshal(alert)
	return body
}

func discordContent(alert Alert) string {
	content := fmt.Sprintf("**%s** %s\n%s", strings.ToUpper(alert.Level), alert.Title, alert.Message)
	if strings.TrimSpace(content) == "" {
		content = "GoPool alert"
	}
	if runes := []rune(content); len(runes) > discordMaxContent {
		content = string(runes[:discordMaxContent-1]) + "…"
	}
	return content
}

func isDiscordWebhook(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && isDiscordHost(u.Hostname())
}

func isDiscordHost(host string) bool {
	host = strings.ToLower(host)
	return host == "discord.com" || host == "discordapp.com" || strings.HasSuffix(host, ".discord.com") || strings.HasSuffix(host, ".discordapp.com")
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

// RedactWebhookURL returns a display-safe webhook URL. Discord webhook
// tokens live in the path (/api/webhooks/<id>/<token>), so the last segment
// is redacted there too.
func RedactWebhookURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "configured webhook"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	if isDiscordHost(u.Hostname()) {
		segments := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segments) >= 4 && segments[0] == "api" && segments[1] == "webhooks" {
			segments[len(segments)-1] = "[redacted]"
			u.Path = "/" + strings.Join(segments, "/")
		}
	}
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
	secrets := append([]string{cfg.AlertTelegramToken}, webhookSecrets(cfg.AlertWebhookURL)...)
	for _, secret := range secrets {
		if secret != "" {
			summary = strings.ReplaceAll(summary, secret, "[redacted]")
		}
	}
	return capSummary(summary)
}

func webhookSecrets(raw string) []string {
	if raw == "" {
		return nil
	}
	secrets := []string{raw}
	u, err := url.Parse(raw)
	if err != nil {
		return secrets
	}
	if u.User != nil {
		secrets = append(secrets, u.User.Username())
		if password, ok := u.User.Password(); ok {
			secrets = append(secrets, password)
		}
	}
	for _, values := range u.Query() {
		secrets = append(secrets, values...)
	}
	if isDiscordHost(u.Hostname()) {
		segments := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segments) > 0 {
			secrets = append(secrets, segments[len(segments)-1])
		}
	}
	return secrets
}

func capSummary(summary string) string {
	if len(summary) > maxResponseSummary {
		return summary[:maxResponseSummary]
	}
	return summary
}
