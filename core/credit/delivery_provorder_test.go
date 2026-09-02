package credit

// REGRESSION GATE for the provOrder unbounded-growth / desync break found by
// red-team 2026-09-01 (RT-DELIV-1). Drop this into core/credit/ as
// delivery_provorder_test.go. It reddens on commit 9d50437 and must go green
// once RedeemDeliveryCredit removes the redeemed key from provOrder (and the
// in-window reversal keeps the two structures in sync).
//
// Three assertions, each an independent facet of the break:
//   (1) BOUND: after N serve+redeem cycles the provOrder slice is bounded by
//       the same cap that bounds the map (build-immutable #8). RED at 9d50437:
//       provOrder == N (no ceiling).
//   (2) SYNC: len(provOrder) never exceeds len(provisional) by more than the
//       cap — the two structures track. RED at 9d50437: they diverge without
//       bound.
//   (3) NO-DUP: a redeem-then-reserve of the same key does not leave a stale
//       duplicate in provOrder that later reverses a LIVE re-served lane before
//       any redeem. RED at 9d50437: duplicate present, live lane destroyed.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestProvOrderStaysBoundedAcrossRedeems is the primary gate: the FIFO order
// slice must be bounded, not just the provisional map. A stream of witnessed
// deliveries (serve+redeem) of distinct (fetcher,object) lanes must not grow
// unbounded state on the floor box.
func TestProvOrderStaysBoundedAcrossRedeems(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	node := id(1)

	const cycles = 50_000
	for i := 0; i < cycles; i++ {
		req := ports.NodeID(ports.HashBytes([]byte{'u', byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)}))
		obj := ports.HashBytes([]byte{'v', byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)})
		l.RecordServeToObject(node, req, obj, id(9), 8)
		l.RedeemDeliveryCredit(node, req, obj, nil, 0, 0)
	}

	// The map is bounded (all redeemed → ~0). The order slice MUST be bounded too.
	if len(l.provOrder) > maxProvisional {
		t.Fatalf("provOrder grew to %d after %d serve+redeem cycles, cap is %d — "+
			"unbounded FIFO state on the floor box (build-immutable #8). "+
			"RedeemDeliveryCredit deletes from the map but not from provOrder.",
			len(l.provOrder), cycles, maxProvisional)
	}
	// And it must stay in sync with the live map.
	if len(l.provOrder) > len(l.provisional)+maxProvisional {
		t.Fatalf("provOrder (%d) and provisional (%d) desynced by more than the cap %d",
			len(l.provOrder), len(l.provisional), maxProvisional)
	}
}

// TestRedeemDoesNotLeaveDuplicateOrderEntry is the desync/correctness gate: a
// redeem must not leave a stale provOrder key that a later re-serve turns into a
// duplicate, whose stale front position then reverses the LIVE re-served lane's
// self-mint before that lane is ever redeemed.
func TestRedeemDoesNotLeaveDuplicateOrderEntry(t *testing.T) {
	const fee = 50_000
	l := New(fee, 500_000)
	node := id(1)
	req := ports.NodeID(ports.HashBytes([]byte("dup-req")))
	obj := ports.HashBytes([]byte("dup-obj"))
	kx := provKey{server: node, requester: req, root: obj}

	count := func() int {
		c := 0
		for _, o := range l.provOrder {
			if o != nil && *o == kx { // provOrder holds pointers; nil is a tombstone
				c++
			}
		}
		return c
	}

	const bytes = 1 << 12
	l.RecordServeToObject(node, req, obj, id(9), bytes) // lane + order entry #1
	l.RedeemDeliveryCredit(node, req, obj, nil, 0, 0)   // deletes map entry; MUST also drop order entry
	if count() != 0 {
		t.Fatalf("after redeem, key still in provOrder %d time(s) — the redeem left a stale FIFO entry", count())
	}
	l.RecordServeToObject(node, req, obj, id(9), bytes) // re-serve → new lane
	if got := count(); got != 1 {
		t.Fatalf("after redeem+reserve, key appears %d times in provOrder, want exactly 1 — duplicate stale entry", got)
	}
}
