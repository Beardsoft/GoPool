package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	nimiq "github.com/NimMiniApps/nimiq-go"

	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/metrics"
	"github.com/Beardsoft/GoPool/internal/ops"
	"github.com/Beardsoft/GoPool/internal/pool"

	"go.uber.org/zap"
)

func main() {
	defer logger.Sync()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.WaitForDaemon(ctx, "", time.Second)
	if err != nil {
		logger.Logger.Fatal("failed to load config", zap.Error(err))
	}

	dbPath := os.Getenv("SQLITE_DB")
	if dbPath == "" {
		dbPath = "pool.db"
	}
	sqlDB, err := db.InitDB(dbPath)
	if err != nil {
		logger.Logger.Fatal("failed to initialize the database", zap.Error(err))
	}
	defer sqlDB.Close()
	queries := db.New(sqlDB)

	if len(os.Args) > 1 && os.Args[1] == "validator" {
		c, err := chain.New(cfg)
		if err != nil {
			logger.Logger.Fatal("failed to set up chain client", zap.Error(err))
		}
		recorder := ops.NewRecorder(queries, config.ConfigHash(cfg.Editable(), cfg.AlertSecrets()))
		manager := pool.NewManager(c, queries, cfg, recorder)
		if err := runValidatorCLI(ctx, manager, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	if cfg.MetricsAddr != "" {
		go func() {
			if err := metrics.Serve(ctx, cfg.MetricsAddr); err != nil {
				logger.Logger.Error("metrics server", zap.Error(err))
			}
		}()
	}

	for {
		c, err := chain.New(cfg)
		if err != nil {
			logger.Logger.Fatal("failed to set up chain client", zap.Error(err))
		}
		recorder := ops.NewRecorder(queries, config.ConfigHash(cfg.Editable(), cfg.AlertSecrets()))
		manager := pool.NewManager(c, queries, cfg, recorder)
		err = manager.Run(ctx)
		if errors.Is(err, pool.ErrConfigChanged) {
			next, loadErr := config.WaitForDaemon(ctx, "", time.Second)
			if loadErr != nil {
				logger.Logger.Fatal("failed to load config", zap.Error(loadErr))
			}
			logger.Logger.Info("reloading pool manager after config change")
			cfg = next
			continue
		}
		if err != nil {
			logger.Logger.Error("pool manager stopped", zap.Error(err))
		}
		return
	}
}

func runValidatorCLI(ctx context.Context, m *pool.Manager, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: gopool validator deactivate|retire|delete <recipient> <value-luna>")
	}
	switch args[0] {
	case "deactivate":
		hash, err := m.Deactivate(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("deactivate transaction submitted: %s\n", hash)
		return nil
	case "retire":
		hash, err := m.Retire(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("retire transaction submitted: %s\n", hash)
		return nil
	case "delete":
		if len(args) != 3 {
			return fmt.Errorf("usage: gopool validator delete <recipient> <value-luna>")
		}
		recipient, err := nimiq.ParseAddress(args[1])
		if err != nil {
			return err
		}
		var value uint64
		if _, err := fmt.Sscanf(args[2], "%d", &value); err != nil {
			return fmt.Errorf("invalid value %q: %w", args[2], err)
		}
		return m.Delete(ctx, recipient, nimiq.Luna(value))
	}
	return fmt.Errorf("unknown validator action %q", args[0])
}
