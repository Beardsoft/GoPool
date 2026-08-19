package notifier

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	discordColorError   = 0xFF1A1A // bright red
	discordColorWarning = 0xF0B429
	discordColorInfo    = 0x5865F2
	discordMaxTitle     = 256
	discordMaxDesc      = 4096
	discordMaxField     = 1024
)

type discordPayloadBody struct {
	Username string         `json:"username,omitempty"`
	Embeds   []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
	Footer      *discordFooter `json:"footer,omitempty"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordFooter struct {
	Text string `json:"text"`
}

func discordPayload(alert Alert, network, poolName string) discordPayloadBody {
	name := strings.TrimSpace(poolName)
	if name == "" {
		name = "GoPool"
	}
	embed := discordEmbed{
		Title:       clipRunes(alertTitle(alert), discordMaxTitle),
		Description: clipRunes(alert.Message, discordMaxDesc),
		Color:       discordColor(alert.Level),
		Timestamp:   alert.Time,
		Footer:      &discordFooter{Text: name},
	}
	for _, field := range alert.Fields {
		embed.Fields = append(embed.Fields, discordField{
			Name:   clipRunes(field.Name, discordMaxTitle),
			Value:  clipRunes(formatFieldValue(network, field.Value), discordMaxField),
			Inline: true,
		})
	}
	return discordPayloadBody{Username: name, Embeds: []discordEmbed{embed}}
}

func alertTitle(alert Alert) string {
	if title := strings.TrimSpace(alert.Title); title != "" {
		return title
	}
	return "GoPool alert"
}

func discordColor(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error":
		return discordColorError
	case "warning":
		return discordColorWarning
	default:
		return discordColorInfo
	}
}

func formatFieldValue(network, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}
	if url := txURL(network, value); url != "" {
		return fmt.Sprintf("[%s](%s)", shortHash(value), url)
	}
	if url := accountURL(network, value); url != "" {
		return fmt.Sprintf("[%s](%s)", shortAddress(value), url)
	}
	return value
}

func explorerBase(network string) string {
	n := strings.ToLower(network)
	switch {
	case strings.Contains(n, "main"):
		return "https://nimiqscan.com"
	case strings.Contains(n, "test"):
		return "https://testnet.nimiqscan.com"
	default:
		return ""
	}
}

func txURL(network, hash string) string {
	if !isTxHash(hash) {
		return ""
	}
	if base := explorerBase(network); base != "" {
		return base + "/transaction/" + hash
	}
	return ""
}

func accountURL(network, address string) string {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(address)), "NQ") {
		return ""
	}
	if base := explorerBase(network); base != "" {
		return base + "/account/" + url.PathEscape(address)
	}
	return ""
}

func isTxHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func shortHash(hash string) string {
	if len(hash) < 16 {
		return hash
	}
	return hash[:8] + "…" + hash[len(hash)-8:]
}

func shortAddress(addr string) string {
	parts := strings.Fields(addr)
	if len(parts) < 4 {
		return addr
	}
	return parts[0] + " " + parts[1] + " … " + parts[len(parts)-1]
}

func clipRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string([]rune(s)[:max-1]) + "…"
}
