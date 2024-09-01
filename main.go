package main

import (
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/pool"
	"go.uber.org/zap"
)

func main() {

	logger.InitLogger()
	defer logger.Sync() // Ensure all logs are flushed before exiting

	// Load the config
	config, err := pool.LoadConfig()
	if err != nil {
		logger.Logger.Fatal("Failed to load config: %v", zap.Error(err))
	}

	// Initialize the database
	db, err := pool.InitDB("pool.db")
	if err != nil {
		logger.Logger.Fatal("Failed to initialize the database: %v", zap.Error(err))
	}
	defer db.Close()

	// Ensure the pool address is set up
	poolAddress, err := pool.EnsurePoolAddress(config)
	if err != nil {
		logger.Logger.Fatal("Failed to ensure pool address: %v", zap.Error(err))
	}
	logger.Logger.Info("Pool Address ready: %s", zap.String("pool_address", poolAddress))

	// Start the pool manager and process epochs continuously
	manager := pool.NewPoolManager(db, config)
	manager.ProcessEpochs()
}
