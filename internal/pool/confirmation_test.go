package pool

import (
	"testing"

	"github.com/NimMiniApps/nimiq-go/rpc"
)

func TestConfirmationOutcome(t *testing.T) {
	cases := []struct {
		name string
		conf *rpc.Confirmation
		want confirmOutcome
	}{
		{"not yet final", &rpc.Confirmation{Included: true, Final: false, Succeeded: true}, outcomePending},
		{"final and succeeded", &rpc.Confirmation{Included: true, Final: true, Succeeded: true}, outcomeSucceeded},
		{"final but failed", &rpc.Confirmation{Included: true, Final: true, Succeeded: false}, outcomeFailed},
	}
	for _, c := range cases {
		if got := confirmationOutcome(c.conf); got != c.want {
			t.Errorf("%s: confirmationOutcome = %v, want %v", c.name, got, c.want)
		}
	}
}
