package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Beardsoft/GoPool/internal/api"
	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/configstore"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

func main() {
	logger.InitLogger()
	defer logger.Sync()

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
	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "config.json"
	}
	store := configstore.New(configPath, queries)
	cfg, configured, err := config.LoadOptional(configPath)
	if err != nil {
		logger.Logger.Fatal("failed to load config", zap.Error(err))
	}

	var a *api.API
	addr := os.Getenv("POOL_API_ADDR")
	if !configured {
		if addr == "" {
			addr = ":8080"
		}
		token, err := readSecretFile("POOL_SETUP_TOKEN_FILE", true)
		if err != nil {
			logger.Logger.Fatal("failed to load setup token", zap.Error(err))
		}
		sum := sha256.Sum256([]byte(token))
		a = api.New(nil, queries, nil, api.WithSetup(hex.EncodeToString(sum[:]), store))
	} else {
		if addr != "" {
			cfg.APIAddr = addr
		}
		cfg.SessionSecret, err = readSecretFile("POOL_SESSION_SECRET_FILE", true)
		if err != nil {
			logger.Logger.Fatal("failed to load session secret", zap.Error(err))
		}
		cfg.AlertTelegramToken, _ = readSecretFile("POOL_ALERT_TELEGRAM_TOKEN_FILE", false)
		cfg.AlertEmailPassword, _ = readSecretFile("POOL_ALERT_EMAIL_PASSWORD_FILE", false)
		if err := config.ValidateAPI(cfg); err != nil {
			logger.Logger.Fatal("invalid API config", zap.Error(err))
		}
		rpcClient, err := chain.NewRPCOnly(cfg)
		if err != nil {
			logger.Logger.Fatal("failed to set up RPC client", zap.Error(err))
		}
		a = api.New(cfg, queries, rpcClient, api.WithConfigStore(store))
		addr = cfg.APIAddr
	}

	srv := &http.Server{Addr: addr, Handler: a.Mux()}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.Logger.Info("API listening", zap.String("addr", addr), zap.Bool("setup_mode", !configured))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Logger.Fatal("API server stopped", zap.Error(err))
	}
}

func readSecretFile(envName string, required bool) (string, error) {
	path := os.Getenv(envName)
	if path == "" {
		if required {
			return "", os.ErrNotExist
		}
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
