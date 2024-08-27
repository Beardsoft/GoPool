package pool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	RPCURL      string  `json:"rpc_url"`
	APIKey      string  `json:"api_key"`
	PoolAddress string  `json:"pool_address"`
	PoolFee     float64 `json:"pool_fee"`
	PrivateKey  string  `json:"private_key"`
}

func LoadConfig() (*Config, error) {
	// First, check if config.json exists in the current directory
	defaultConfigPath := filepath.Join(".", "config.json")
	if _, err := os.Stat(defaultConfigPath); err == nil {
		return loadConfigFromFile(defaultConfigPath)
	}

	// If not found, fall back to the environment variable
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		return nil, fmt.Errorf("CONFIG_FILE environment variable not set, and config.json not found in current directory")
	}

	return loadConfigFromFile(configFile)
}

func loadConfigFromFile(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
