package pool

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/NimMiniApps/nimiq-go/rpc"
)

func TestValidatePoolWallet(t *testing.T) {
	const wallet = "NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E"
	const other = "NQ46 U66M JNLD 0DJ7 0E9P Q7XR V9KV H976 813A"

	tests := []struct {
		name       string
		configured string
		derived    string
		validator  *rpc.Validator
		wantError  string
	}{
		{name: "all identities use one wallet", configured: wallet, derived: wallet, validator: &rpc.Validator{Address: wallet, RewardAddress: wallet}},
		{name: "configured validator differs", configured: other, derived: wallet, validator: &rpc.Validator{Address: wallet, RewardAddress: wallet}, wantError: "configured validator address"},
		{name: "live validator differs", configured: wallet, derived: wallet, validator: &rpc.Validator{Address: other, RewardAddress: wallet}, wantError: "on-chain validator address"},
		{name: "reward wallet differs", configured: wallet, derived: wallet, validator: &rpc.Validator{Address: wallet, RewardAddress: other}, wantError: "reward address"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePoolWallet(tt.configured, tt.derived, tt.validator)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("validatePoolWallet() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("validatePoolWallet() error = %v, want text %q", err, tt.wantError)
			}
		})
	}
}

func TestPoolWalletReadinessHeartbeatPersistsFailure(t *testing.T) {
	err := fmt.Errorf("reward wallet mismatch")
	hb := poolWalletReadinessHeartbeat("NQ20 TSB0 DFSM UH9C 15GQ GAGJ TTE4 D3MA 859E", err, false)
	if hb.ReadinessError != err.Error() || hb.ValidatorState != "unready" || hb.RPCOk {
		t.Fatalf("heartbeat = %+v", hb)
	}
}

func testPolicy() *rpc.Policy {
	return &rpc.Policy{
		BlocksPerBatch:     60,
		BatchesPerEpoch:    720,
		BlocksPerEpoch:     43200,
		GenesisBlockNumber: 0,
	}
}

func TestClassify(t *testing.T) {
	m := &Manager{policy: testPolicy()}
	cases := []struct {
		height uint32
		want   blockKind
	}{
		{0, blockElection},
		{60, blockCheckpoint},
		{43200, blockElection},
		{999, blockMicro},
		{120, blockCheckpoint},
	}
	for _, c := range cases {
		if got := m.classify(c.height); got != c.want {
			t.Errorf("classify(%d) = %v, want %v", c.height, got, c.want)
		}
	}
}

func TestEpochBatchAtMatchesNode(t *testing.T) {
	p := &rpc.Policy{BlocksPerBatch: 60, BlocksPerEpoch: 240, GenesisBlockNumber: 0}
	cases := []struct {
		height, epoch, batch uint32
	}{
		{0, 0, 0},
		{1, 1, 1},
		{60, 1, 1},
		{61, 1, 2},
		{120, 1, 2},
		{240, 1, 4},
		{241, 2, 5},
	}
	for _, c := range cases {
		if got := epochAt(p, c.height); got != c.epoch {
			t.Errorf("epochAt(%d) = %d, want %d", c.height, got, c.epoch)
		}
		if got := batchAt(p, c.height); got != c.batch {
			t.Errorf("batchAt(%d) = %d, want %d", c.height, got, c.batch)
		}
	}
}

func TestEpochClockAtUsesPolicyNotGuesses(t *testing.T) {
	p := &rpc.Policy{BlocksPerEpoch: 240, GenesisBlockNumber: 1000, BlockSeparationTime: 1000}
	cases := []struct {
		height, epoch, into, remaining, remainingMs uint32
	}{
		{1000, 0, 0, 0, 0},
		{1001, 1, 1, 239, 239_000},
		{1240, 1, 240, 0, 0},
		{1241, 2, 1, 239, 239_000},
	}
	for _, c := range cases {
		got := EpochClockAt(p, c.height)
		if got.Epoch != c.epoch || got.Head != c.height || got.BlocksIntoEpoch != c.into || got.RemainingBlocks != c.remaining || got.RemainingMs != uint64(c.remainingMs) {
			t.Errorf("EpochClockAt(%d) = %+v, want epoch=%d into=%d remaining=%d ms=%d", c.height, got, c.epoch, c.into, c.remaining, c.remainingMs)
		}
		if got.BlocksPerEpoch != 240 || got.BlockSeparationMs != 1000 {
			t.Errorf("EpochClockAt(%d) dropped policy: %+v", c.height, got)
		}
	}
}

func TestShouldRefreshGauges(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if !shouldRefreshGauges(time.Time{}, now) {
		t.Error("zero last must refresh")
	}
	if shouldRefreshGauges(now.Add(-29*time.Second), now) {
		t.Error("29s is too soon")
	}
	if !shouldRefreshGauges(now.Add(-30*time.Second), now) {
		t.Error("30s must refresh")
	}
}
