package pool

import (
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
)

func TestSplitReward(t *testing.T) {
	after, fee := splitReward(nimiq.Luna(100_000_00000), 0.01) // 1% fee
	if fee != 1_000_00000 {
		t.Errorf("fee = %d, want %d", fee, 1_000_00000)
	}
	if after != 99_000_00000 {
		t.Errorf("after = %d, want %d", after, 99_000_00000)
	}
	if after+fee != nimiq.Luna(100_000_00000) {
		t.Error("after + fee must equal the original reward exactly")
	}
}

func TestSplitRewardZeroFee(t *testing.T) {
	after, fee := splitReward(nimiq.Luna(500), 0)
	if fee != 0 || after != 500 {
		t.Errorf("after=%d fee=%d, want after=500 fee=0", after, fee)
	}
}
