package pool

import (
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Config struct {
	RPCURL            string  `mapstructure:"rpc_url"`
	APIKey            string  `mapstructure:"api_key"`
	PoolAddress       string  `mapstructure:"pool_address"`
	PoolFeeWallet     string  `mapstructure:"pool_fee_wallet"`
	PoolFeePercentage float64 `mapstructure:"pool_fee_percentage"`
	PrivateKey        string  `mapstructure:"private_key"`
}

func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("POOL")

	if err := viper.ReadInConfig(); err != nil {
		logger.Logger.Error("Error reading config file", zap.Error(err))
		return nil, err
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		logger.Logger.Error("Unable to decode config into struct", zap.Error(err))
		return nil, err
	}

	logger.Logger.Info("Config loaded successfully",
		zap.String("rpc_url", config.RPCURL),
		zap.Float64("pool_fee_percentage", config.PoolFeePercentage),
		zap.String("pool_fee_wallet", config.PoolFeeWallet))

	return &config, nil
}
