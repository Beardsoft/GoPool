package pool

import (
	"testing"
	"time"
)

func TestIsStuck(t *testing.T) {
	blocksPerEpoch := uint32(43200)
	sepMs := uint32(10_000)
	zero := time.Time{}
	cases := []struct {
		name string
		sub  int64
		head uint32
		eps  int
		want bool
	}{
		{"just submitted", 1000, 1000, 3, false},
		{"within threshold", 1000, 100_000, 3, false},
		{"exactly at threshold not stuck", 1000, 130_600, 3, false},
		{"past threshold is stuck", 1000, 130_601, 3, true},
		{"one epoch threshold", 1000, 44_201, 1, true},
	}
	for _, c := range cases {
		if got := IsStuck(c.sub, c.head, c.eps, blocksPerEpoch, sepMs, zero, zero); got != c.want {
			t.Errorf("%s: IsStuck = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsStuckTimeFallback(t *testing.T) {
	blocksPerEpoch := uint32(240)
	sepMs := uint32(1000) // 1s/block -> 1 epoch = 240s = 4min; 3 epochs = 12min
	now := time.Date(2026, 8, 16, 22, 0, 0, 0, time.UTC)
	head := uint32(540)
	cases := []struct {
		name        string
		sub         int64
		submittedAt time.Time
		want        bool
	}{
		{"unknown height, no timestamp never stuck", 0, time.Time{}, false},
		{"unknown height, fresh tx not stuck", 0, now.Add(-5 * time.Minute), false},
		{"unknown height, exactly at threshold not stuck", 0, now.Add(-12 * time.Minute), false},
		{"unknown height, past threshold stuck", 0, now.Add(-13 * time.Minute), true},
		{"known height takes precedence over timestamp", 1000, now.Add(-10 * time.Hour), false},
	}
	for _, c := range cases {
		if got := IsStuck(c.sub, head, 3, blocksPerEpoch, sepMs, c.submittedAt, now); got != c.want {
			t.Errorf("%s: IsStuck = %v, want %v", c.name, got, c.want)
		}
	}
}
