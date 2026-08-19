package pool

import (
	"testing"

	nimiq "github.com/NimMiniApps/nimiq-go"
	"github.com/NimMiniApps/nimiq-go/rpc"
)

func TestSlotShareNotInSet(t *testing.T) {
	vs := []rpc.Validator{{Address: "NQ01 AAAA", Balance: 100}}
	elected, count := slotShare("NQ02 BBBB", vs, 512)
	if elected || count != 0 {
		t.Fatalf("elected=%v count=%d", elected, count)
	}
}

func TestSlotShareEmptySet(t *testing.T) {
	elected, count := slotShare("NQ01 AAAA", nil, 512)
	if elected || count != 0 {
		t.Fatalf("elected=%v count=%d", elected, count)
	}
}

func TestSlotShareIgnoresAddressSpaces(t *testing.T) {
	vs := []rpc.Validator{{Address: "NQ01 AAAA BBBB", Balance: nimiq.Luna(512)}}
	elected, count := slotShare("NQ01AAAABBBB", vs, 512)
	if !elected || count != 512 {
		t.Fatalf("elected=%v count=%d", elected, count)
	}
}

func TestSlotShareLargestRemainder(t *testing.T) {
	vs := []rpc.Validator{
		{Address: "NQ01 A", Balance: 50},
		{Address: "NQ02 B", Balance: 30},
		{Address: "NQ03 C", Balance: 20},
	}
	_, a := slotShare("NQ01 A", vs, 5)
	_, b := slotShare("NQ02 B", vs, 5)
	_, c := slotShare("NQ03 C", vs, 5)
	if a+b+c != 5 {
		t.Fatalf("sum=%d want 5 (a=%d b=%d c=%d)", a+b+c, a, b, c)
	}
	if a != 3 || b != 1 || c != 1 {
		t.Fatalf("a=%d b=%d c=%d want 3,1,1", a, b, c)
	}
}
