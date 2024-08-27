package main

import (
	"log"
	"time"

	"github.com/Beardsoft/GoPool/pool"
)

func main() {
	// Initialize the database
	db, err := pool.InitDB("pool.db")
	if err != nil {
		log.Fatalf("Failed to initialize the database: %v", err)
	}
	defer db.Close()

	// Start the pool manager
	manager := pool.NewPoolManager(db)
	for {
		manager.ProcessEpoch()
		time.Sleep(24 * time.Hour) // wait for next epoch
	}
}
