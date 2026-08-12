package pool

import (
	"testing"

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
