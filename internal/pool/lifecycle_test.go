package pool

import (
	"testing"

	"github.com/NimMiniApps/nimiq-go/rpc"
)

func TestShouldReactivate(t *testing.T) {
	jailedUntil := uint32(1000)
	cases := []struct {
		name       string
		jailedFrom *uint32
		height     uint32
		want       bool
	}{
		{"not jailed", nil, 2000, false},
		{"still jailed", &jailedUntil, 500, false},
		{"cooldown passed", &jailedUntil, 1000, true},
		{"well past cooldown", &jailedUntil, 5000, true},
	}
	for _, c := range cases {
		if got := shouldReactivate(c.jailedFrom, c.height); got != c.want {
			t.Errorf("%s: shouldReactivate = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestValidatorLiveState(t *testing.T) {
	height := uint32(1000)
	future := uint32(2000)
	past := uint32(500)
	cases := []struct {
		name string
		v    rpc.Validator
		want string
	}{
		{"active", rpc.Validator{}, "active"},
		{"retired", rpc.Validator{Retired: true}, "retired"},
		{"inactive", rpc.Validator{InactivityFlag: &future}, "inactive"},
		{"inactive expired", rpc.Validator{InactivityFlag: &past}, "active"},
		{"jailed", rpc.Validator{JailedFrom: &future}, "jailed"},
		{"jail expired", rpc.Validator{JailedFrom: &past}, "active"},
	}
	for _, c := range cases {
		if got := validatorLiveState(&c.v, height); got != c.want {
			t.Errorf("%s: validatorLiveState = %q, want %q", c.name, got, c.want)
		}
	}
}
