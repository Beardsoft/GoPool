package configstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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
	if _, err := store.Save(context.Background(), "actor", "", validEditable()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(store.Path())
	_, err := store.Save(context.Background(), "actor", "stale", validEditable())
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("%v", err)
	}
	after, _ := os.ReadFile(store.Path())
	if !bytes.Equal(before, after) {
		t.Fatal("file changed")
	}
}
