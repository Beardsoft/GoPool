package chain

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// WalletJSON is the signing and voting material from onboarding wallet.json.
// The daemon already has the address key via POOL_PRIVATE_KEY_FILE.
type WalletJSON struct {
	SigningPrivateKey string `json:"signing_private_key"`
	VotingSecretKey   string `json:"voting_secret_key"`
}

// LoadWalletJSON reads signing and voting keys from an onboarding wallet file.
func LoadWalletJSON(path string) (WalletJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WalletJSON{}, err
	}
	var w WalletJSON
	if err := json.Unmarshal(data, &w); err != nil {
		return WalletJSON{}, fmt.Errorf("chain: wallet.json: %w", err)
	}
	w.SigningPrivateKey = strings.TrimSpace(w.SigningPrivateKey)
	w.VotingSecretKey = strings.TrimSpace(w.VotingSecretKey)
	if w.SigningPrivateKey == "" || w.VotingSecretKey == "" {
		return WalletJSON{}, fmt.Errorf("chain: wallet.json missing signing_private_key or voting_secret_key")
	}
	return w, nil
}
