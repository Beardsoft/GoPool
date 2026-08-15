package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	nimiq "github.com/NimMiniApps/nimiq-go"
)

type Config struct {
	RPCURL            string  `json:"rpc_url" mapstructure:"rpc_url"`
	Network           string  `json:"network" mapstructure:"network"`
	PoolFeeWallet     string  `json:"pool_fee_wallet" mapstructure:"pool_fee_wallet"`
	PoolFeePercentage float64 `json:"pool_fee_percentage" mapstructure:"pool_fee_percentage"`
	PrivateKey        string  `json:"private_key,omitempty" mapstructure:"private_key"`
	PayoutMode        string  `json:"payout_mode" mapstructure:"payout_mode"`
	MinPayoutLuna     uint64  `json:"min_payout_luna" mapstructure:"min_payout_luna"`
	AutoReactivate    bool    `json:"auto_reactivate" mapstructure:"auto_reactivate"`
	DryRun            bool    `json:"dry_run,omitempty" mapstructure:"dry_run"`
	APIAddr           string  `json:"api_addr" mapstructure:"api_addr"`
	SessionSecret     string  `json:"session_secret,omitempty" mapstructure:"session_secret"`
	ValidatorAddress  string  `json:"validator_address" mapstructure:"validator_address"`
	OperatorAddresses string  `json:"operator_addresses" mapstructure:"operator_addresses"`
	MetricsAddr       string  `json:"metrics_addr" mapstructure:"metrics_addr"`

	AlertTelegramEnabled     bool   `json:"alert_telegram_enabled,omitempty" mapstructure:"alert_telegram_enabled"`
	AlertTelegramDestination string `json:"alert_telegram_destination,omitempty" mapstructure:"alert_telegram_destination"`
	AlertTelegramToken       string `json:"alert_telegram_token,omitempty" mapstructure:"alert_telegram_token"`
	AlertWebhookEnabled      bool   `json:"alert_webhook_enabled,omitempty" mapstructure:"alert_webhook_enabled"`
	AlertWebhookURL          string `json:"alert_webhook_url,omitempty" mapstructure:"alert_webhook_url"`
	AlertEmailEnabled        bool   `json:"alert_email_enabled,omitempty" mapstructure:"alert_email_enabled"`
	AlertEmailTo             string `json:"alert_email_to,omitempty" mapstructure:"alert_email_to"`
	AlertEmailSmtpHost       string `json:"alert_email_smtp_host,omitempty" mapstructure:"alert_email_smtp_host"`
	AlertEmailSmtpPort       int    `json:"alert_email_smtp_port,omitempty" mapstructure:"alert_email_smtp_port"`
	AlertEmailUsername       string `json:"alert_email_username,omitempty" mapstructure:"alert_email_username"`
	AlertEmailPassword       string `json:"alert_email_password,omitempty" mapstructure:"alert_email_password"`
	AlertEmailFrom           string `json:"alert_email_from,omitempty" mapstructure:"alert_email_from"`

	PoolName        string `json:"pool_name" mapstructure:"pool_name"`
	PoolDescription string `json:"pool_description,omitempty" mapstructure:"pool_description"`
	ContactURL      string `json:"contact_url,omitempty" mapstructure:"contact_url"`
	Disclosure      string `json:"disclosure,omitempty" mapstructure:"disclosure"`
}

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
	AlertWebhookURL          string  `json:"alert_webhook_url,omitempty"`
	AlertEmailEnabled        bool    `json:"alert_email_enabled"`
	AlertEmailTo             string  `json:"alert_email_to,omitempty"`
	PoolName                 string  `json:"pool_name"`
	PoolDescription          string  `json:"pool_description,omitempty"`
	ContactURL               string  `json:"contact_url,omitempty"`
	Disclosure               string  `json:"disclosure,omitempty"`
}

type SecretPresence struct {
	ValidatorKey  bool `json:"validator_key"`
	SessionSecret bool `json:"session_secret"`
	TelegramToken bool `json:"telegram_token"`
	EmailPassword bool `json:"email_password"`
}

type Redacted struct {
	Settings Editable       `json:"settings"`
	Secrets  SecretPresence `json:"secrets"`
}

func (c *Config) Editable() Editable {
	return Editable{RPCURL: c.RPCURL, Network: c.Network, PoolFeeWallet: c.PoolFeeWallet, PoolFeePercentage: c.PoolFeePercentage,
		PayoutMode: c.PayoutMode, MinPayoutLuna: c.MinPayoutLuna, AutoReactivate: c.AutoReactivate, APIAddr: c.APIAddr,
		ValidatorAddress: c.ValidatorAddress, OperatorAddresses: c.OperatorAddresses, MetricsAddr: c.MetricsAddr,
		AlertTelegramEnabled: c.AlertTelegramEnabled, AlertTelegramDestination: c.AlertTelegramDestination,
		AlertWebhookEnabled: c.AlertWebhookEnabled, AlertWebhookURL: c.AlertWebhookURL, AlertEmailEnabled: c.AlertEmailEnabled,
		AlertEmailTo: c.AlertEmailTo, PoolName: c.PoolName, PoolDescription: c.PoolDescription, ContactURL: c.ContactURL, Disclosure: c.Disclosure}
}

func FromEditable(e Editable) *Config {
	return &Config{RPCURL: e.RPCURL, Network: e.Network, PoolFeeWallet: e.PoolFeeWallet, PoolFeePercentage: e.PoolFeePercentage,
		PayoutMode: e.PayoutMode, MinPayoutLuna: e.MinPayoutLuna, AutoReactivate: e.AutoReactivate, APIAddr: e.APIAddr,
		ValidatorAddress: e.ValidatorAddress, OperatorAddresses: e.OperatorAddresses, MetricsAddr: e.MetricsAddr,
		AlertTelegramEnabled: e.AlertTelegramEnabled, AlertTelegramDestination: e.AlertTelegramDestination,
		AlertWebhookEnabled: e.AlertWebhookEnabled, AlertWebhookURL: e.AlertWebhookURL, AlertEmailEnabled: e.AlertEmailEnabled,
		AlertEmailTo: e.AlertEmailTo, PoolName: e.PoolName, PoolDescription: e.PoolDescription, ContactURL: e.ContactURL, Disclosure: e.Disclosure}
}

func Redact(c *Config) Redacted {
	return Redacted{Settings: c.Editable(), Secrets: SecretPresence{ValidatorKey: c.PrivateKey != "", SessionSecret: c.SessionSecret != "", TelegramToken: c.AlertTelegramToken != "", EmailPassword: c.AlertEmailPassword != ""}}
}

func EditableHash(editable Editable) string {
	data, _ := json.Marshal(editable)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (c *Config) Validate() error {
	if c.PayoutMode != "delegate" && c.PayoutMode != "transfer" {
		return fmt.Errorf("config: payout_mode must be delegate or transfer")
	}
	if c.PoolFeePercentage < 0 || c.PoolFeePercentage >= 1 {
		return fmt.Errorf("config: pool_fee_percentage must be in [0, 1)")
	}
	return nil
}

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

func LoadOptional(path string) (*Config, bool, error) {
	data, err := os.ReadFile(configPath(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var editable Editable
	if err := json.Unmarshal(data, &editable); err != nil {
		return nil, false, err
	}
	cfg := FromEditable(editable)
	var extras struct {
		DryRun bool `json:"dry_run"`
	}
	_ = json.Unmarshal(data, &extras)
	cfg.DryRun = extras.DryRun
	return cfg, true, nil
}

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
	if keyPath := os.Getenv("POOL_PRIVATE_KEY_FILE"); keyPath != "" {
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("read validator key: %w", err)
		}
		cfg.PrivateKey = strings.TrimSpace(string(key))
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func LoadConfig() (*Config, error) { return LoadDaemon("") }
