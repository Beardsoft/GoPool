package pool

import (
	"strings"
	"testing"
	"time"
)

func TestNextBootstrap(t *testing.T) {
	base := bootstrapSnapshot{
		Network: "test-albatross", Deposit: 10_000_000_000, MinStake: 10_000_000,
		HasWalletJSON: true, LocalConsensus: true, FaucetEnabled: true,
	}
	cases := []struct {
		name string
		mod  func(*bootstrapSnapshot)
		want bootstrapAction
	}{
		{"testnet low balance faucets", func(s *bootstrapSnapshot) { s.Balance = 0 }, bootstrapFaucet},
		{"mainnet low balance waits", func(s *bootstrapSnapshot) { s.Network = "main-albatross"; s.FaucetEnabled = false; s.Balance = 0 }, bootstrapWait},
		{"deposit met skips faucet and registers", func(s *bootstrapSnapshot) { s.Balance = 10_000_000_000 }, bootstrapRegister},
		{"testnet at 101k registers", func(s *bootstrapSnapshot) { s.Balance = 10_100_000_000 }, bootstrapRegister},
		{"funded but no wallet json waits", func(s *bootstrapSnapshot) {
			s.Balance = 10_100_000_000
			s.HasWalletJSON = false
		}, bootstrapWait},
		{"funded but no local consensus waits", func(s *bootstrapSnapshot) {
			s.Balance = 10_100_000_000
			s.LocalConsensus = false
		}, bootstrapWait},
		{"pending register waits", func(s *bootstrapSnapshot) {
			s.Balance = 10_100_000_000
			s.PendingRegister = true
		}, bootstrapWait},
		{"validator exists self-stakes leftover", func(s *bootstrapSnapshot) {
			s.ValidatorExists = true
			s.Balance = 100_000_000
		}, bootstrapSelfStake},
		{"leftover below min+reserve waits", func(s *bootstrapSnapshot) {
			s.ValidatorExists = true
			s.Balance = 1_000_000
		}, bootstrapWait},
		{"already a staker is ready", func(s *bootstrapSnapshot) {
			s.ValidatorExists = true
			s.HasSelfStaker = true
			s.Balance = 100_000_000
		}, bootstrapReady},
		{"dry-run never submits", func(s *bootstrapSnapshot) {
			s.Balance = 10_100_000_000
			s.DryRun = true
		}, bootstrapWait},
		{"mainnet with deposit registers", func(s *bootstrapSnapshot) {
			s.Network = "main-albatross"
			s.FaucetEnabled = false
			s.Balance = 10_100_000_000
		}, bootstrapRegister},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			tc.mod(&s)
			if got := nextBootstrap(s); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestSelfStakeAmount(t *testing.T) {
	got := selfStakeAmount(100_000_000, 10_000_000)
	if got != 99_000_000 {
		t.Fatalf("got %d", got)
	}
	if selfStakeAmount(1_000_000, 10_000_000) != 0 {
		t.Fatal("reserve must block stake")
	}
}

func TestBootstrapWaitingErrorAsksToSelfFund(t *testing.T) {
	msg := bootstrapWaitingError(bootstrapSnapshot{
		Balance: 0, Deposit: 10_000_000_000,
		Address:       "NQ60 64GU RB0H SH2S MFBB B2HS HK8D 35LD 8TDU",
		FaucetBlocked: true, FaucetRetryAt: time.Date(2026, 8, 20, 13, 0, 0, 0, time.UTC),
	})
	for _, want := range []string{
		"Waiting for 100000 NIM to register the validator (have 0 NIM)",
		"Send at least 101000 NIM",
		"NQ60 64GU RB0H SH2S MFBB B2HS HK8D 35LD 8TDU",
		"faucet",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("missing %q in %q", want, msg)
		}
	}
}
