// Package chain builds the nimiq-go RPC client and signer this pool uses to
// talk to a node — the one place that turns config into a live connection.
package chain

import (
	"fmt"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"
	"github.com/NimMiniApps/nimiq-go/signer"

	"github.com/Beardsoft/GoPool/internal/config"
)

// Chain is the pool's RPC client, signer, and network identity.
type Chain struct {
	RPC     *rpc.Client
	Signer  *signer.PrivateKey
	Network nimiq.NetworkID
}

func networkFromString(s string) (nimiq.NetworkID, error) {
	switch s {
	case "main-albatross":
		return nimiq.NetworkMainAlbatross, nil
	case "test-albatross":
		return nimiq.NetworkTestAlbatross, nil
	case "dev-albatross":
		return nimiq.NetworkDevAlbatross, nil
	case "unit-albatross":
		return nimiq.NetworkUnitAlbatross, nil
	}
	return 0, fmt.Errorf("chain: unknown network %q", s)
}

// New builds an RPC client and signer from cfg.
func New(cfg *config.Config) (*Chain, error) {
	network, err := networkFromString(cfg.Network)
	if err != nil {
		return nil, err
	}
	key, err := signer.ParsePrivateKeyHex(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("chain: parsing private key: %w", err)
	}
	client, err := rpc.New(cfg.RPCURL, rpc.WithNetwork(network))
	if err != nil {
		return nil, fmt.Errorf("chain: %w", err)
	}
	return &Chain{RPC: client, Signer: key, Network: network}, nil
}

// Address is the pool's own validator/fee-paying address, derived from the
// configured private key — no importRawKey/unlockAccount RPC dance needed,
// the SDK signs offline.
func (c *Chain) Address() nimiq.Address {
	return c.Signer.Address()
}
