package pool

import "testing"

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
