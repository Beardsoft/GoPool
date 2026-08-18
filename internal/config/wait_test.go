package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForDaemonReturnsOnceFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.WriteFile(path, []byte(`{"payout_mode":"delegate","pool_fee_percentage":0.01,"min_payout_luna":1}`+"\n"), 0o600)
	}()

	cfg, err := WaitForDaemon(ctx, path, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForDaemon: %v", err)
	}
	if cfg.PayoutMode != "delegate" {
		t.Fatalf("payout_mode = %q", cfg.PayoutMode)
	}
}

func TestWaitForDaemonStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := WaitForDaemon(ctx, filepath.Join(t.TempDir(), "missing.json"), time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
