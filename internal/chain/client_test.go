package chain

import (
	"testing"

	"github.com/Beardsoft/GoPool/internal/config"
)

const testKeyHex = "6927eb8de74e8ea06a8afae5a66db176a7031f742b656651ac53bddb8a4ad3f3"

func TestNewAttachesValidatorRPCFallback(t *testing.T) {
	cfg := &config.Config{
		RPCURL:          "http://pool.example:8648",
		ValidatorRPCURL: "http://gopool-validator:8648",
		Network:         "test-albatross",
		PrivateKey:      testKeyHex,
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := c.RPC.Endpoints()
	if len(got) != 2 || got[0] != cfg.RPCURL || got[1] != cfg.ValidatorRPCURL {
		t.Fatalf("endpoints = %v", got)
	}
	if c.Wallet == nil {
		t.Fatal("expected wallet RPC client")
	}
}

func TestNewRPCOnlyOmitsValidatorFallback(t *testing.T) {
	cfg := &config.Config{
		RPCURL:          "http://pool.example:8648",
		ValidatorRPCURL: "http://gopool-validator:8648",
		Network:         "test-albatross",
	}
	c, err := NewRPCOnly(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := c.Endpoints()
	if len(got) != 1 || got[0] != cfg.RPCURL {
		t.Fatalf("endpoints = %v", got)
	}
}

func TestNewWithoutValidatorRPC(t *testing.T) {
	cfg := &config.Config{
		RPCURL:     "http://pool.example:8648",
		Network:    "main-albatross",
		PrivateKey: testKeyHex,
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.RPC.Endpoints()) != 1 {
		t.Fatalf("endpoints = %v", c.RPC.Endpoints())
	}
	if c.Wallet != nil {
		t.Fatal("wallet must be nil without validator_rpc_url")
	}
}
