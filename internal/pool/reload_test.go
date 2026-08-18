package pool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldReloadWaitsForInFlightPayout(t *testing.T) {
	if shouldReload("aaa", "bbb", true) {
		t.Fatal("reload while payout in flight")
	}
	if !shouldReload("aaa", "bbb", false) {
		t.Fatal("reload when idle and hash changed")
	}
	if shouldReload("aaa", "aaa", false) {
		t.Fatal("reload when hash unchanged")
	}
}

func TestCurrentFileHashChangesWhenFileRewritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h1 := currentFileHash(path)
	if h1 == "" {
		t.Fatal("empty hash")
	}
	if err := os.WriteFile(path, []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h2 := currentFileHash(path)
	if h1 == h2 {
		t.Fatal("hash unchanged after rewrite")
	}
}
