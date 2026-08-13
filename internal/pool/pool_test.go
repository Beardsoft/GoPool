package pool

import (
	"testing"
	"time"

	"github.com/NimMiniApps/nimiq-go/rpc"
)

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
