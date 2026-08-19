package pool

import (
	"testing"
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
