package pool

import (
	"testing"

	"github.com/NimMiniApps/nimiq-go/rpc"
)

func TestApplyActiveSetSnapshot(t *testing.T) {
	var got epochParticipation
	vs := []rpc.Validator{{Address: "NQ01 A", Balance: 100}}
	applyActiveSet(&got, "NQ01 A", vs, 2, 512)
	if !got.valid || got.epoch != 2 || !got.elected || got.slotCount != 512 || got.slotsTotal != 512 {
		t.Fatalf("%+v", got)
	}
	applyActiveSet(&got, "NQ99 Z", vs, 3, 512)
	if !got.valid || got.elected || got.slotCount != 0 || got.epoch != 3 {
		t.Fatalf("unelected %+v", got)
	}
}

func TestShouldAlertNotElectedOncePerEpoch(t *testing.T) {
	unelected := epochParticipation{valid: true, epoch: 2, elected: false, slotsTotal: 512}
	if !shouldAlertNotElected(false, 0, unelected) {
		t.Fatal("first unelected epoch should alert")
	}
	if shouldAlertNotElected(true, 2, unelected) {
		t.Fatal("same epoch should not re-alert")
	}
	next := unelected
	next.epoch = 3
	if !shouldAlertNotElected(true, 2, next) {
		t.Fatal("new unelected epoch should alert")
	}
	elected := epochParticipation{valid: true, epoch: 4, elected: true, slotCount: 12, slotsTotal: 512}
	if shouldAlertNotElected(true, 3, elected) {
		t.Fatal("elected should not alert")
	}
}

func TestKeepLastParticipationOnRPCFailure(t *testing.T) {
	var got epochParticipation
	vs := []rpc.Validator{{Address: "NQ01 A", Balance: 100}}
	applyActiveSet(&got, "NQ01 A", vs, 2, 512)
	// RPC failure is modeled by not calling applyActiveSet.
	if !got.valid || got.epoch != 2 || !got.elected {
		t.Fatalf("lost snapshot %+v", got)
	}
}
