package main

import (
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/pool"
	"go.uber.org/zap"
)

func main() {

	logger.InitLogger()
	defer logger.Sync() // Ensure all logs are flushed before exiting

	// Load the config
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Logger.Fatal("Failed to load config: %v", zap.Error(err))
	}

	// Initialize the database
	sqlDB, err := db.InitDB("pool.db")
	if err != nil {
		logger.Logger.Fatal("Failed to initialize the database: %v", zap.Error(err))
	}
	defer sqlDB.Close()

	// Create a new Queries instance using the sqlc-generated code
	queries := db.New(sqlDB)
	// Optionally, use context for queries
	// Ensure the pool address is set up
	poolAddress, err := pool.EnsurePoolAddress(cfg)
	if err != nil {
		logger.Logger.Fatal("Failed to ensure pool address: %v", zap.Error(err))
	}
	logger.Logger.Info("Pool Address ready: %s", zap.String("pool_address", poolAddress))

	// Start the pool manager and process epochs continuously
	manager := pool.NewPoolManager(sqlDB, queries, cfg) // Pass `queries` to the manager
	manager.ProcessEpochs()
}
