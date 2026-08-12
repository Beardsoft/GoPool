package pool

import (
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
)

func TestStakerPercentage(t *testing.T) {
	got := stakerPercentage(nimiq.Luna(25_000_00000), nimiq.Luna(100_000_00000))
	if got < 24.99 || got > 25.01 {
		t.Errorf("percentage = %v, want ~25", got)
	}
}

func TestStakerPercentageZeroTotal(t *testing.T) {
	if got := stakerPercentage(0, 0); got != 0 {
		t.Errorf("percentage of a zero-balance validator = %v, want 0", got)
	}
}
