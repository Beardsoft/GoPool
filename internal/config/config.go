package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	nimiq "github.com/NimMiniApps/nimiq-go"
)

// Config holds the pool daemon's runtime configuration.
type Config struct {
	RPCURL            string  `json:"rpc_url"`
	Network           string  `json:"network"`
	PoolFeeWallet     string  `json:"pool_fee_wallet"`
	PoolFeePercentage float64 `json:"pool_fee_percentage"`
	PrivateKey        string  `json:"private_key,omitempty"`
	PayoutMode        string  `json:"payout_mode"`
	MinPayoutLuna     uint64  `json:"min_payout_luna"`
	StuckPayoutEpochs int     `json:"stuck_payout_epochs"`
	AutoReactivate    bool    `json:"auto_reactivate"`
	DryRun            bool    `json:"dry_run,omitempty"`
	APIAddr           string  `json:"api_addr"`
	SessionSecret     string  `json:"session_secret,omitempty"`
	ValidatorAddress  string  `json:"validator_address"`
	OperatorAddresses string  `json:"operator_addresses"`
	MetricsAddr       string  `json:"metrics_addr"`

	AlertTelegramEnabled     bool   `json:"alert_telegram_enabled,omitempty"`
	AlertTelegramDestination string `json:"alert_telegram_destination,omitempty"`
	AlertTelegramToken       string `json:"alert_telegram_token,omitempty"`
	AlertWebhookEnabled      bool   `json:"alert_webhook_enabled,omitempty"`
	AlertWebhookURL          string `json:"alert_webhook_url,omitempty"`

	PoolName        string `json:"pool_name"`
	PoolDescription string `json:"pool_description,omitempty"`
	ContactURL      string `json:"contact_url,omitempty"`
	Disclosure      string `json:"disclosure,omitempty"`
}

// Editable is the operator-editable subset of Config exposed via the API.
type Editable struct {
	RPCURL                   string  `json:"rpc_url"`
	Network                  string  `json:"network"`
	PoolFeeWallet            string  `json:"pool_fee_wallet"`
	PoolFeePercentage        float64 `json:"pool_fee_percentage"`
	PayoutMode               string  `json:"payout_mode"`
	MinPayoutLuna            uint64  `json:"min_payout_luna"`
	AutoReactivate           bool    `json:"auto_reactivate"`
	APIAddr                  string  `json:"api_addr"`
	ValidatorAddress         string  `json:"validator_address"`
	OperatorAddresses        string  `json:"operator_addresses"`
	MetricsAddr              string  `json:"metrics_addr"`
	AlertTelegramEnabled     bool    `json:"alert_telegram_enabled"`
	AlertTelegramDestination string  `json:"alert_telegram_destination,omitempty"`
	AlertWebhookEnabled      bool    `json:"alert_webhook_enabled"`
	PoolName                 string  `json:"pool_name"`
	PoolDescription          string  `json:"pool_description,omitempty"`
	ContactURL               string  `json:"contact_url,omitempty"`
	Disclosure               string  `json:"disclosure,omitempty"`
}

// AlertSecrets is the secret subset of alert delivery config. Values are
// accepted on save (empty means keep current) but never returned by the API.
type AlertSecrets struct {
	TelegramToken string `json:"alert_telegram_token,omitempty"`
	WebhookURL    string `json:"alert_webhook_url,omitempty"`
}

// AlertSecrets extracts the secret subset of the config.
func (c *Config) AlertSecrets() AlertSecrets {
	return AlertSecrets{TelegramToken: c.AlertTelegramToken, WebhookURL: c.AlertWebhookURL}
}

// ApplySecrets merges provided secrets onto c; empty fields keep the current value.
func (c *Config) ApplySecrets(s AlertSecrets) {
	if s.TelegramToken != "" {
		c.AlertTelegramToken = s.TelegramToken
	}
	if s.WebhookURL != "" {
		c.AlertWebhookURL = s.WebhookURL
	}
}

// SecretPresence reports which optional secrets are set, without exposing them.
type SecretPresence struct {
	ValidatorKey  bool `json:"validator_key"`
	SessionSecret bool `json:"session_secret"`
	TelegramToken bool `json:"telegram_token"`
	WebhookURL    bool `json:"webhook_url"`
}

// Redacted is the API-safe view of config: editable settings plus secret presence.
type Redacted struct {
	Settings Editable       `json:"settings"`
	Secrets  SecretPresence `json:"secrets"`
}

// Editable returns the operator-editable subset of the config.
func (c *Config) Editable() Editable {
	return Editable{RPCURL: c.RPCURL, Network: c.Network, PoolFeeWallet: c.PoolFeeWallet, PoolFeePercentage: c.PoolFeePercentage,
		PayoutMode: c.PayoutMode, MinPayoutLuna: c.MinPayoutLuna, AutoReactivate: c.AutoReactivate, APIAddr: c.APIAddr,
		ValidatorAddress: c.ValidatorAddress, OperatorAddresses: c.OperatorAddresses, MetricsAddr: c.MetricsAddr,
		AlertTelegramEnabled: c.AlertTelegramEnabled, AlertTelegramDestination: c.AlertTelegramDestination,
		AlertWebhookEnabled: c.AlertWebhookEnabled, PoolName: c.PoolName, PoolDescription: c.PoolDescription, ContactURL: c.ContactURL, Disclosure: c.Disclosure}
}

// FromEditable builds a Config from an editable subset.
func FromEditable(e Editable) *Config {
	return &Config{RPCURL: e.RPCURL, Network: e.Network, PoolFeeWallet: e.PoolFeeWallet, PoolFeePercentage: e.PoolFeePercentage,
		PayoutMode: e.PayoutMode, MinPayoutLuna: e.MinPayoutLuna, AutoReactivate: e.AutoReactivate, APIAddr: e.APIAddr,
		ValidatorAddress: e.ValidatorAddress, OperatorAddresses: e.OperatorAddresses, MetricsAddr: e.MetricsAddr,
		AlertTelegramEnabled: e.AlertTelegramEnabled, AlertTelegramDestination: e.AlertTelegramDestination,
		AlertWebhookEnabled: e.AlertWebhookEnabled, PoolName: e.PoolName, PoolDescription: e.PoolDescription, ContactURL: e.ContactURL, Disclosure: e.Disclosure}
}

// Redact builds the API-safe representation of a config.
func Redact(c *Config) Redacted {
	return Redacted{Settings: c.Editable(), Secrets: SecretPresence{ValidatorKey: c.PrivateKey != "", SessionSecret: c.SessionSecret != "", TelegramToken: c.AlertTelegramToken != "", WebhookURL: c.AlertWebhookURL != ""}}
}

// ConfigHash returns a stable hash of the editable settings plus alert
// secrets, so a secret change also flags the daemon for restart.
func ConfigHash(editable Editable, secrets AlertSecrets) string {
	data, _ := json.Marshal(struct {
		Editable
		AlertSecrets
	}{editable, secrets})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Validate checks invariants that must hold for the daemon to run.
func (c *Config) Validate() error {
	if c.PayoutMode != "delegate" && c.PayoutMode != "transfer" {
		return fmt.Errorf("config: payout_mode must be delegate or transfer")
	}
	if c.PoolFeePercentage < 0 || c.PoolFeePercentage >= 1 {
		return fmt.Errorf("config: pool_fee_percentage must be in [0, 1)")
	}
	return nil
}

// ValidateEditable checks the operator-editable settings before saving.
func ValidateEditable(e Editable) error {
	if strings.TrimSpace(e.PoolName) == "" {
		return fmt.Errorf("pool_name is required")
	}
	u, err := url.Parse(e.RPCURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("rpc_url must be an HTTP(S) URL")
	}
	if e.Network != "main" && e.Network != "main-albatross" && e.Network != "test-albatross" && e.Network != "dev-albatross" {
		return fmt.Errorf("unsupported network")
	}
	if e.PoolFeePercentage < 0 || e.PoolFeePercentage >= 1 {
		return fmt.Errorf("pool_fee_percentage must be in [0, 1)")
	}
	if e.PayoutMode != "delegate" && e.PayoutMode != "transfer" {
		return fmt.Errorf("payout_mode must be delegate or transfer")
	}
	if e.MinPayoutLuna == 0 {
		return fmt.Errorf("min_payout_luna must be greater than zero")
	}
	for name, value := range map[string]string{"pool_fee_wallet": e.PoolFeeWallet, "validator_address": e.ValidatorAddress} {
		if _, err := nimiq.ParseAddress(value); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	for _, value := range strings.Split(e.OperatorAddresses, ",") {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, err := nimiq.ParseAddress(strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("operator_addresses: %w", err)
		}
	}
	return nil
}

// ValidateAPI checks API-required settings and normalizes operator addresses.
func ValidateAPI(c *Config) error {
	if c.APIAddr == "" {
		return fmt.Errorf("config: api_addr is required")
	}
	if strings.TrimSpace(c.SessionSecret) == "" {
		return fmt.Errorf("config: session_secret is required")
	}
	if c.ValidatorAddress == "" {
		return fmt.Errorf("config: validator_address is required")
	}
	addr, err := nimiq.ParseAddress(c.ValidatorAddress)
	if err != nil {
		return fmt.Errorf("config: validator_address: %w", err)
	}
	c.ValidatorAddress = addr.String()

	var operators []string
	for _, value := range strings.Split(c.OperatorAddresses, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		op, err := nimiq.ParseAddress(value)
		if err != nil {
			return fmt.Errorf("config: operator_addresses: %w", err)
		}
		operators = append(operators, op.String())
	}
	c.OperatorAddresses = strings.Join(operators, ",")
	return nil
}

func configPath(path string) string {
	if path != "" {
		return path
	}
	if value := os.Getenv("CONFIG_FILE"); value != "" {
		return value
	}
	return "config.json"
}

// LoadOptional loads config from path if present, returning (nil, false, nil) when absent.
func LoadOptional(path string) (*Config, bool, error) {
	data, err := os.ReadFile(configPath(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, false, err
	}
	cfg.PrivateKey = ""
	cfg.SessionSecret = ""
	return &cfg, true, nil
}

// LoadDaemon loads and validates the daemon config, applying defaults and env overrides.
func LoadDaemon(path string) (*Config, error) {
	data, err := os.ReadFile(configPath(path))
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.PayoutMode == "" {
		cfg.PayoutMode = "delegate"
	}
	if cfg.MetricsAddr == "" {
		cfg.MetricsAddr = ":9100"
	}
	if cfg.StuckPayoutEpochs == 0 {
		cfg.StuckPayoutEpochs = 3
	}
	if keyPath := os.Getenv("POOL_PRIVATE_KEY_FILE"); keyPath != "" {
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("read validator key: %w", err)
		}
		cfg.PrivateKey = strings.TrimSpace(string(key))
	}
	if dry := os.Getenv("POOL_DRY_RUN"); dry != "" {
		cfg.DryRun = dry == "1" || dry == "true" || dry == "TRUE"
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// WaitForDaemon loads daemon config, retrying while the file is missing.
// Other LoadDaemon errors (invalid JSON, validation) are returned immediately.
func WaitForDaemon(ctx context.Context, path string, retry time.Duration) (*Config, error) {
	for {
		cfg, err := LoadDaemon(path)
		if err == nil {
			return cfg, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retry):
		}
	}
}
