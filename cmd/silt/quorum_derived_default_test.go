package main

import (
	"testing"

	"github.com/nerolabs/silt/core/chain"
)

// TestDerivedGatherTargetTracksTheByzantineBar pins the delegated owner call of
// 2026-09-08: on the untrusted objective path an unset -quorum DERIVES to the same
// Byzantine bar the commit already demands, instead of the shipped literal 3.
//
// The defect it closes, measured by the blind PE on the #380 review: at a four-anchor
// launch the literal asks for all three peers, so the swarm tolerates f = 0 — while the
// project publishes its liveness bound (D-CONSENSUS-ARMING (19), amended (21)) at f = 1.
// The field topology already overrode it to 2, which is how the published number and the
// shipped default came apart without anyone tripping over it.
//
// This can only ever LOWER the ask. gatherTwoPhase gathers
// max(caller floor, ConfigQuorum(), RequiredQuorum()), so the derived Byzantine bar sits
// underneath whatever this returns, and -quorum is not a validity term on this path
// (#380). ABLATION: return `explicit, false` from the objective branch of
// effectiveQuorum and the four-anchor arm goes RED at 3.
func TestDerivedGatherTargetTracksTheByzantineBar(t *testing.T) {
	const shipped = 3

	// The case that motivated the call: four anchors, nothing set.
	if got, defaulted := effectiveQuorum(false, shipped, true, true, 4); !defaulted || got != chain.ByzantineThreshold(4) {
		t.Fatalf("four-anchor objective launch: gather target %d (defaulted=%v), want %d — the shipped literal asks every peer to attest, so the swarm tolerates f=0 against a published bound of f=1",
			got, defaulted, chain.ByzantineThreshold(4))
	}
	if chain.ByzantineThreshold(4) != 2 {
		t.Fatalf("premise moved: ByzantineThreshold(4) = %d, want 2", chain.ByzantineThreshold(4))
	}

	// An explicit choice always wins, including one ABOVE the derived bar: an operator
	// may demand more attestations than safety needs, and this must not quietly undo it.
	if got, defaulted := effectiveQuorum(true, 3, true, true, 4); defaulted || got != 3 {
		t.Fatalf("explicit -quorum 3 at four anchors: got %d (defaulted=%v), want 3 and no derivation", got, defaulted)
	}

	// THE REGIME THE FIRST CUT GOT WRONG (blind PE, ruling on 91e7eb2). With
	// -byzantine-quorum=false, RequiredQuorum returns cfg.Quorum VERBATIM — the local
	// floor IS the validity bar there, so deriving it downward makes the node ACCEPT a
	// 2-attestation block it previously refused, on three accept-side predicates. It is
	// also the exact boundary #380's ratification drew: the trusted opt-out and legacy
	// mode keep cfg.Quorum unchanged. The regime is live in-tree (integration/sybil's
	// compose file runs it, safe only incidentally because it also passes -quorum).
	// ABLATION: drop `|| !byzantineSizing` from the guard and this arm goes RED.
	if got, defaulted := effectiveQuorum(false, shipped, true, false /*byzantineSizing*/, 4); defaulted || got != shipped {
		t.Fatalf("objective with Byzantine sizing OFF: got %d (defaulted=%v), want the literal untouched — there RequiredQuorum returns cfg.Quorum verbatim, so deriving it downward LOWERS a validity bar and the node accepts blocks it used to refuse", got, defaulted)
	}

	// Off the untrusted objective path there is nothing to derive from.
	if got, defaulted := effectiveQuorum(false, shipped, false, true, 4); defaulted || got != shipped {
		t.Fatalf("non-objective: got %d (defaulted=%v), want the shipped literal untouched", got, defaulted)
	}

	// A launch set too small to derive a real bar keeps the literal rather than
	// deriving a self-commit: ByzantineThreshold(1) = 0 would ask for zero attestations.
	for _, anchors := range []int{0, 1} {
		if got, defaulted := effectiveQuorum(false, shipped, true, true, anchors); defaulted || got != shipped {
			t.Fatalf("%d anchor(s): got %d (defaulted=%v), want the literal — deriving here would ask for zero attestations", anchors, got, defaulted)
		}
	}

	// It scales with the launch set rather than pinning a constant.
	for _, tc := range []struct{ anchors, want int }{{4, 2}, {7, 4}, {13, 8}} {
		if got, _ := effectiveQuorum(false, shipped, true, true, tc.anchors); got != tc.want {
			t.Fatalf("%d anchors: gather target %d, want %d (the Byzantine bar over the launch set)", tc.anchors, got, tc.want)
		}
	}
}
