package pool

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestEffectivePayoutKind(t *testing.T) {
	cases := []struct {
		name string
		mode string
		pref *bool
		dep  bool // stillDelegated
		want payoutKind
	}{
		{"global delegate, no pref, delegated", "delegate", nil, true, payoutDelegate},
		{"global delegate, no pref, undelegated", "delegate", nil, false, payoutTransfer},
		{"global transfer, no pref", "transfer", nil, true, payoutTransfer},
		{"pref compound beats global transfer", "transfer", boolPtr(true), true, payoutDelegate},
		{"pref compound but undelegated falls back", "transfer", boolPtr(true), false, payoutTransfer},
		{"pref cashout beats global delegate", "delegate", boolPtr(false), true, payoutTransfer},
	}
	for _, c := range cases {
		if got := effectivePayoutKind(c.mode, c.pref, c.dep); got != c.want {
			t.Errorf("%s: effectivePayoutKind = %v, want %v", c.name, got, c.want)
		}
	}
}
