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
