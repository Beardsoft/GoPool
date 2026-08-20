package pool

import (
	"sort"
	"strings"

	"github.com/NimMiniApps/nimiq-go/rpc"
)

func normalizeAddr(s string) string {
	return strings.ToUpper(strings.ReplaceAll(s, " ", ""))
}

// slotShare reports whether addr holds any of `slots` in this set, and how
// many, using Hamilton / largest-remainder over the set's balances.
// Membership with a zero assignment is not elected.
func slotShare(addr string, validators []rpc.Validator, slots uint32) (elected bool, count uint32) {
	want := normalizeAddr(addr)
	if want == "" || slots == 0 || len(validators) == 0 {
		return false, 0
	}

	type entry struct {
		addr      string
		stake     uint64
		assigned  uint32
		remainder uint64
	}
	entries := make([]entry, 0, len(validators))
	var total uint64
	inSet := false
	for _, v := range validators {
		n := normalizeAddr(v.Address)
		if n == "" {
			continue
		}
		e := entry{addr: n, stake: uint64(v.Balance)}
		if n == want {
			inSet = true
		}
		total += e.stake
		entries = append(entries, e)
	}
	if !inSet || total == 0 {
		return false, 0
	}

	var used uint32
	for i := range entries {
		prod := entries[i].stake * uint64(slots)
		entries[i].assigned = uint32(prod / total)
		entries[i].remainder = prod % total
		used += entries[i].assigned
	}
	leftover := slots - used
	order := make([]int, len(entries))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := entries[order[i]], entries[order[j]]
		if a.remainder != b.remainder {
			return a.remainder > b.remainder
		}
		return a.addr < b.addr
	})
	for i := uint32(0); i < leftover; i++ {
		entries[order[i]].assigned++
	}

	for _, e := range entries {
		if e.addr == want {
			return e.assigned > 0, e.assigned
		}
	}
	return false, 0
}
