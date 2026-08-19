package chain

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWalletJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wallet.json")
	raw := `{
	  "validator_address": "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E",
	  "address_private_key": "aa",
	  "signing_private_key": "bb",
	  "fee_private_key": "cc",
	  "voting_secret_key": "dd",
	  "voting_public_key": "ee"
	}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := LoadWalletJSON(path)
	if err != nil {
		t.Fatal(err)
	}
	if w.SigningPrivateKey != "bb" || w.VotingSecretKey != "dd" {
		t.Fatalf("%+v", w)
	}
}

func TestLoadWalletJSONMissingKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wallet.json")
	_ = os.WriteFile(path, []byte(`{"validator_address":"NQ20"}`), 0o600)
	if _, err := LoadWalletJSON(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadWalletJSONAbsent(t *testing.T) {
	_, err := LoadWalletJSON(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}
