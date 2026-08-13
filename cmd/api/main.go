package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Beardsoft/GoPool/internal/api"
	"github.com/Beardsoft/GoPool/internal/chain"
	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"

	"go.uber.org/zap"
)

func main() {
	logger.InitLogger()
	defer logger.Sync()

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Logger.Fatal("failed to load config", zap.Error(err))
	}
	if err := config.ValidateAPI(cfg); err != nil {
		logger.Logger.Fatal("invalid API config", zap.Error(err))
	}

	rpcClient, err := chain.NewRPCOnly(cfg)
	if err != nil {
		logger.Logger.Fatal("failed to set up RPC client", zap.Error(err))
	}

	sqlDB, err := db.InitDB("pool.db")
	if err != nil {
		logger.Logger.Fatal("failed to initialize the database", zap.Error(err))
	}
	defer sqlDB.Close()
	queries := db.New(sqlDB)

	a := api.New(cfg, queries, rpcClient)
	srv := &http.Server{Addr: cfg.APIAddr, Handler: a.Mux()}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.Logger.Info("API listening", zap.String("addr", cfg.APIAddr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Logger.Fatal("API server stopped", zap.Error(err))
	}
}
