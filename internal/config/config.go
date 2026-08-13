package config

import (
	"fmt"

	"github.com/Beardsoft/GoPool/internal/logger"
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
