package config

import (
	"fmt"
	"strings"

	"github.com/Beardsoft/GoPool/internal/logger"
	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Config holds pool daemon settings loaded from config.json / POOL_* env vars.
type Config struct {
	RPCURL            string  `mapstructure:"rpc_url"`
	Network           string  `mapstructure:"network"`
	PoolFeeWallet     string  `mapstructure:"pool_fee_wallet"`
	PoolFeePercentage float64 `mapstructure:"pool_fee_percentage"`
	PrivateKey        string  `mapstructure:"private_key"`
	PayoutMode        string  `mapstructure:"payout_mode"`
	MinPayoutLuna     uint64  `mapstructure:"min_payout_luna"`
	AutoReactivate    bool    `mapstructure:"auto_reactivate"`
	// APIAddr, SessionSecret, and ValidatorAddress are only used by cmd/api.
	// The daemon (cmd) ignores them.
	APIAddr          string `mapstructure:"api_addr"`
	SessionSecret    string `mapstructure:"session_secret"`
	ValidatorAddress string `mapstructure:"validator_address"`
	MetricsAddr       string  `mapstructure:"metrics_addr"`
}

// Validate reports whether payout_mode and pool_fee_percentage are in range.
func (c *Config) Validate() error {
	if c.PayoutMode != "delegate" && c.PayoutMode != "transfer" {
		return fmt.Errorf("config: payout_mode must be \"delegate\" or \"transfer\", got %q", c.PayoutMode)
	}
	if c.PoolFeePercentage < 0 || c.PoolFeePercentage >= 1 {
		return fmt.Errorf("config: pool_fee_percentage must be in [0, 1), got %v", c.PoolFeePercentage)
	}
	return nil
}

// ValidateAPI reports whether the API-only config fields are present and
// well-formed. The daemon's LoadConfig/Validate does not require these —
// only cmd/api calls this, right after loading config.
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

// LoadConfig reads config.json (and POOL_* overrides) and validates it.
func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("POOL")
viper.SetDefault("payout_mode", "delegate")
viper.SetDefault("auto_reactivate", true)
viper.SetDefault("metrics_addr", ":9100")

	if err := viper.ReadInConfig(); err != nil {
		logger.Logger.Error("Error reading config file", zap.Error(err))
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		logger.Logger.Error("Unable to decode config into struct", zap.Error(err))
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	logger.Logger.Info("Config loaded successfully",
		zap.String("rpc_url", cfg.RPCURL),
		zap.String("payout_mode", cfg.PayoutMode),
		zap.Float64("pool_fee_percentage", cfg.PoolFeePercentage))

	return &cfg, nil
}
