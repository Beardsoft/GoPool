package configstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Beardsoft/GoPool/internal/config"
	"github.com/Beardsoft/GoPool/internal/db"
)

func validEditable() config.Editable {
	return config.Editable{RPCURL: "https://rpc-testnet.nimiqscan.com", Network: "test-albatross", PoolFeeWallet: "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E", PoolFeePercentage: .01, PayoutMode: "delegate", MinPayoutLuna: 100_000, AutoReactivate: true, APIAddr: ":8080", ValidatorAddress: "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E", PoolName: "GoPool"}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	sqlDB, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return New(filepath.Join(t.TempDir(), "config.json"), db.New(sqlDB))
}

func TestStoreRejectsStaleHashWithoutChangingFile(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.Save(context.Background(), "actor", "", validEditable(), config.AlertSecrets{}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(store.Path())
	_, err := store.Save(context.Background(), "actor", "stale", validEditable(), config.AlertSecrets{})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("%v", err)
	}
	after, _ := os.ReadFile(store.Path())
	if !bytes.Equal(before, after) {
		t.Fatal("file changed")
	}
}

func TestStoreSaveMergesSecretsAndCarriesUnmanagedFields(t *testing.T) {
	store := newTestStore(t)
	seed := `{"rpc_url":"https://rpc-testnet.nimiqscan.com","network":"test-albatross","pool_fee_wallet":"NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E","pool_fee_percentage":0.01,"payout_mode":"delegate","min_payout_luna":100000,"auto_reactivate":true,"api_addr":":8080","validator_address":"NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E","pool_name":"GoPool","private_key":"legacy-key-value","session_secret":"legacy-session-value","alert_telegram_token":"old-token","stuck_payout_epochs":7,"faucet_url":"https://faucet.example","validator_rpc_url":"http://127.0.0.1:8648"}`
	if err := os.WriteFile(store.Path(), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	_, hash, err := store.current()
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Save(context.Background(), "actor", hash, validEditable(), config.AlertSecrets{TelegramToken: "tg-new-token", WebhookURL: "https://discord.com/api/webhooks/1/wh-new-token"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(store.Path())
	for _, want := range []string{"legacy-key-value", "legacy-session-value", "tg-new-token", "wh-new-token", `"stuck_payout_epochs": 7`, `"faucet_url": "https://faucet.example"`, `"validator_rpc_url": "http://127.0.0.1:8648"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config file missing %q: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), "old-token") {
		t.Fatalf("provided secret did not override: %s", raw)
	}
	// Re-save with empty secrets must keep the existing values.
	_, err = store.Save(context.Background(), "actor", revision.Hash, validEditable(), config.AlertSecrets{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(store.Path())
	if !strings.Contains(string(raw), "tg-new-token") || !strings.Contains(string(raw), "wh-new-token") {
		t.Fatalf("empty secrets dropped existing values: %s", raw)
	}
	// Revisions are redacted: no secret values in the snapshots.
	revisions, err := store.ListRevisions(context.Background())
	if err != nil || len(revisions) != 2 {
		t.Fatalf("revisions=%d err=%v", len(revisions), err)
	}
	for _, rev := range revisions {
		before, _ := json.Marshal(rev.Before)
		after, _ := json.Marshal(rev.After)
		for _, leaked := range []string{"old-token", "tg-new-token", "legacy-key-value", "legacy-session-value", "wh-new-token"} {
			if strings.Contains(string(before), leaked) || strings.Contains(string(after), leaked) {
				t.Fatalf("revision leaked %q", leaked)
			}
		}
	}
}
