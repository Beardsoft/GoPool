package pool

import "testing"

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
