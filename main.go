package main

import (
	"log"

	"go.uber.org/zap"

	"github.com/Beardsoft/GoPool/pool"
)

var logger *zap.Logger

func initLogger() {
	var err error
	logger, err = zap.NewProduction() // For production logging
	if err != nil {
		panic(err)
	}
	defer logger.Sync() // flushes buffer, if any
}

func main() {
	// Load the config
	config, err := pool.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize the database
	db, err := pool.InitDB("pool.db")
	if err != nil {
		log.Fatalf("Failed to initialize the database: %v", err)
	}
	defer db.Close()

	// Ensure the pool address is set up
	poolAddress, err := pool.EnsurePoolAddress(config)
	if err != nil {
		log.Fatalf("Failed to ensure pool address: %v", err)
	}
	log.Printf("Pool Address ready: %s", poolAddress)

	// Start the pool manager and process epochs continuously
	manager := pool.NewPoolManager(db, config)
	manager.ProcessEpochs()
}
