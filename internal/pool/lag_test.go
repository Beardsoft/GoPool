package pool

import (
	"testing"
	"time"
)

func TestChainLagFloor(t *testing.T) {
	if got := chainLag(10, 12); got != 0 {
		t.Fatalf("processed ahead of head: %d", got)
	}
	if got := chainLag(40, 10); got != 30 {
		t.Fatalf("lag = %d", got)
	}
}

func TestLagAlertWarnsThenEscalatesIfNotRecovered(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	var s lagAlertState

	s, kind := nextLagAlert(s, 10, now)
	if kind != lagAlertNone {
		t.Fatalf("below warn threshold: %v", kind)
	}

	s, kind = nextLagAlert(s, 30, now)
	if kind != lagAlertWarning || !s.warningSent {
		t.Fatalf("first crossing: kind=%v state=%+v", kind, s)
	}

	s, kind = nextLagAlert(s, 40, now.Add(time.Minute))
	if kind != lagAlertNone {
		t.Fatalf("still within escalate window: %v", kind)
	}

	s, kind = nextLagAlert(s, 25, now.Add(lagEscalateAfter))
	if kind != lagAlertError || !s.errorSent {
		t.Fatalf("did not recover: kind=%v state=%+v", kind, s)
	}

	s, kind = nextLagAlert(s, 80, now.Add(lagEscalateAfter+time.Minute))
	if kind != lagAlertNone {
		t.Fatalf("already escalated: %v", kind)
	}

	s, kind = nextLagAlert(s, 1, now.Add(3*time.Minute))
	if kind != lagAlertNone || s.warningSent || s.errorSent {
		t.Fatalf("recovered state = %+v kind=%v", s, kind)
	}

	s, kind = nextLagAlert(s, 30, now.Add(4*time.Minute))
	if kind != lagAlertWarning {
		t.Fatalf("re-warn after recover: %v", kind)
	}
}

func TestLagAlertDoesNotEscalateIfCaughtUp(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	s, _ := nextLagAlert(lagAlertState{}, 50, now)
	s, kind := nextLagAlert(s, 0, now.Add(lagEscalateAfter))
	if kind != lagAlertNone || s.warningSent {
		t.Fatalf("caught up should reset: kind=%v state=%+v", kind, s)
	}
}
