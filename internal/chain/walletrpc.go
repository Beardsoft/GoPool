package chain

import (
	"context"
	"fmt"

	"github.com/NimMiniApps/nimiq-go/rpc"
)

// WalletRPC talks to a validator node's wallet methods. It must be pointed
// only at that node — never at the pool's public RPC.
type WalletRPC struct {
	rpc *rpc.Client
}

func NewWalletRPC(url string) (*WalletRPC, error) {
	if url == "" {
		return nil, fmt.Errorf("chain: validator RPC URL is required")
	}
	c, err := rpc.New(url)
	if err != nil {
		return nil, err
	}
	return &WalletRPC{rpc: c}, nil
}

func (w *WalletRPC) ConsensusEstablished(ctx context.Context) (bool, error) {
	var ok bool
	if err := w.rpc.Call(ctx, "isConsensusEstablished", nil, &ok); err != nil {
		return false, err
	}
	return ok, nil
}

func (w *WalletRPC) CreateValidator(ctx context.Context, address, addressKey, signingKey, votingKey string) (string, error) {
	var imported string
	if err := w.rpc.Call(ctx, "importRawKey", []any{addressKey, ""}, &imported); err != nil {
		return "", fmt.Errorf("importRawKey: %w", err)
	}
	var unlocked bool
	if err := w.rpc.Call(ctx, "unlockAccount", []any{address, "", 0}, &unlocked); err != nil {
		return "", fmt.Errorf("unlockAccount: %w", err)
	}
	params := []any{address, address, signingKey, votingKey, address, "", 500, "+0"}
	var hash string
	if err := w.rpc.Call(ctx, "sendNewValidatorTransaction", params, &hash); err != nil {
		return "", fmt.Errorf("sendNewValidatorTransaction: %w", err)
	}
	return hash, nil
}
