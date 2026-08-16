package pool

import (
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
)

func TestChoosePayoutTx(t *testing.T) {
	cases := []struct {
		mode           string
		stillDelegated bool
		want           payoutKind
	}{
		{"delegate", true, payoutDelegate},
		{"delegate", false, payoutTransfer}, // fixes zpool's TODO: fall back when undelegated
		{"transfer", true, payoutTransfer},
		{"transfer", false, payoutTransfer},
	}
	for _, c := range cases {
		if got := choosePayoutTx(c.mode, c.stillDelegated); got != c.want {
			t.Errorf("choosePayoutTx(%q, %v) = %v, want %v", c.mode, c.stillDelegated, got, c.want)
		}
	}
}

func TestPayoutWorthSending(t *testing.T) {
	cases := []struct {
		amount, fee nimiq.Luna
		want        bool
	}{
		{1, 0, true},    // zero-fee network: min_payout already applied in SQL
		{99, 10, false}, // 10×10 = 100, hold
		{100, 10, true}, // exactly 10×
		{9, 1, false},
		{10, 1, true},
	}
	for _, c := range cases {
		if got := payoutWorthSending(c.amount, c.fee); got != c.want {
			t.Errorf("payoutWorthSending(%d, %d) = %v, want %v", c.amount, c.fee, got, c.want)
		}
	}
}

func TestPayoutKindLabel(t *testing.T) {
	if got := payoutKindLabel(payoutDelegate); got != "delegate" {
		t.Errorf("delegate label = %q", got)
	}
	if got := payoutKindLabel(payoutTransfer); got != "transfer" {
		t.Errorf("transfer label = %q", got)
	}
}

func TestMarkFeeFloorHold(t *testing.T) {
	alerted := map[string]bool{}
	if !markFeeFloorHold(alerted, "A") {
		t.Error("first hold for A must alert")
	}
	if markFeeFloorHold(alerted, "A") {
		t.Error("repeat hold for A must not re-alert")
	}
	if !markFeeFloorHold(alerted, "B") {
		t.Error("first hold for B must alert")
	}
}

func TestPruneFeeFloorHolds(t *testing.T) {
	held := map[string]bool{"A": true}
	alerted := map[string]bool{"A": true, "B": true}
	pruneFeeFloorHolds(held, alerted)
	if !alerted["A"] {
		t.Error("A still held, must stay alerted")
	}
	if alerted["B"] {
		t.Error("B no longer held, must be pruned so a future hold re-alerts")
	}
}

func TestFeeFloorHoldReAlertsAfterResolution(t *testing.T) {
	alerted := map[string]bool{}
	// Tick 1: A held -> alert.
	if !markFeeFloorHold(alerted, "A") {
		t.Fatal("tick 1 must alert")
	}
	// Tick 2: A still held -> no alert.
	if markFeeFloorHold(alerted, "A") {
		t.Fatal("tick 2 must not re-alert")
	}
	// Tick 3: A resolved (not held) -> pruned.
	pruneFeeFloorHolds(map[string]bool{}, alerted)
	if alerted["A"] {
		t.Fatal("resolved staker must be pruned")
	}
	// Tick 4: A held again -> re-alert.
	if !markFeeFloorHold(alerted, "A") {
		t.Fatal("tick 4 must re-alert after resolution")
	}
}
