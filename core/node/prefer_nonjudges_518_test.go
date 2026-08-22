package node

// #518 regression (unit leg): rebuilt-shard placement prefers holders outside
// the caretaker-judge quorum, stably — a claim excludes both the paramedic and
// the named holder from judging, so a 2-caretaker deployment whose rebuilt
// shard lands on the other caretaker has zero eligible judges and the bounty
// starves silently (captured: all four of a repair's claims naming the other
// caretaker, quorum=2). The wire leg is the e2e bounty test itself, whose
// flake mode this closes.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

func TestPreferNonJudgesStablePartition(t *testing.T) {
	id := func(b byte) ports.NodeID { var n ports.NodeID; n[0] = b; return n }
	a, b, c, d := id(1), id(2), id(3), id(4)

	judges := map[ports.NodeID]bool{b: true, d: true}
	got := preferNonJudges([]ports.NodeID{a, b, c, d}, judges)
	want := []ports.NodeID{a, c, b, d} // civilians first, each class in original order
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("#518: order[%d] = %v, want %v (civilians first, stable within class)", i, got[i], want[i])
		}
	}

	// No judges → untouched slice order.
	got = preferNonJudges([]ports.NodeID{d, c, b, a}, nil)
	want = []ports.NodeID{d, c, b, a}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("no-judges order[%d] = %v, want %v", i, got[i], want[i])
		}
	}

	// All judges → still all present, order preserved (preference, not veto:
	// a shard on a judge beats a shard nowhere).
	got = preferNonJudges([]ports.NodeID{b, d}, judges)
	if len(got) != 2 || got[0] != b || got[1] != d {
		t.Fatalf("all-judges must keep every candidate in order, got %v", got)
	}
}
