package pool

import "testing"

func TestIsStuck(t *testing.T) {
	blocksPerEpoch := uint32(43200)
	cases := []struct {
		name string
		sub  int64
		head uint32
		eps  int
		want bool
	}{
		{"unknown submitted height never auto-fails", 0, 500_000, 3, false},
		{"just submitted", 1000, 1000, 3, false},
		{"within threshold", 1000, 100_000, 3, false},
		{"exactly at threshold not stuck", 1000, 130_600, 3, false},
		{"past threshold is stuck", 1000, 130_601, 3, true},
		{"one epoch threshold", 1000, 44_201, 1, true},
	}
	for _, c := range cases {
		if got := isStuck(c.sub, c.head, c.eps, blocksPerEpoch); got != c.want {
			t.Errorf("%s: isStuck = %v, want %v", c.name, got, c.want)
		}
	}
}
