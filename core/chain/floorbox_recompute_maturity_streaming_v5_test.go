package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Tests for the STREAMING class-M maturity recompute (RecomputeMatureNowStreaming,
// floorbox_recompute_maturity_v5.go). The streaming path pulls each member's proof witness on
// demand (SeenSetStreamWitness.Member) and lets that member's proof heap be freed before the next
// member is verified, cutting resident witness from O(N·depth) to O(depth). It is CERTIFIED
// soundness-neutral ONLY under these conditions, which these tests hold the line on:
//
//   - FOLD-EQUIVALENCE: the streamed verdict == the resident-map verdict == the full-node
//     matureNow() verdict, over the SAME committed state, mature and immature. Streaming changes
//     HOW memory is held, never WHAT is verified.
//   - R-M-STREAM-COMPLETENESS (the load-bearing RED ablation): a SHORT / truncated id-list must
//     still STALL. Streaming frees per-member PROOF heaps, NEVER the id-list; the completeness MTH
//     nodeSetMTH(w.IDs) still consumes the full list. If a member is dropped from IDs, the
//     reconstructed MTH differs from the committed validatorsSeenRoot and the box stalls
//     ErrRecomputeSeenSetIncomplete. This test feeds a short id-list to the STREAMING path and
//     asserts the stall bites — the residual R-M-STREAM-COMPLETENESS the cert names.
//   - FORGED-VALUE-UNDER-STREAMING: a member pulled by the provider with a forged bonded weight
//     still fails its Resolve against the committed root and stalls ErrRecomputeMemberStateUnproven.
//     Streaming does not weaken the per-member anchor.

// streamWitnessFor converts a resident SeenSetWitness fixture into the streaming SeenSetStreamWitness
// form: the SAME id-list, digest proof, and per-member witnesses, but delivered through a pull
// provider over the fixture's Members map. This is the test analogue of the witness-delivery seam
// that would fetch/decode a member's proof on demand.
func streamWitnessFor(w SeenSetWitness) SeenSetStreamWitness {
	return SeenSetStreamWitness{
		IDs:             w.IDs,
		SeenRootWitness: w.SeenRootWitness,
		SeenRootValue:   w.SeenRootValue,
		Member: func(id ports.NodeID) (MemberStateWitness, bool) {
			mw, ok := w.Members[id]
			return mw, ok
		},
	}
}

// TestRecomputeMatureNowStreaming_MatchesResidentAndFullNode is the equivalence anchor for the
// streaming path: over the SAME committed state, the streamed verdict equals BOTH the resident-map
// RecomputeMatureNow verdict AND the full-node matureNow() verdict — for a mature config (low bar)
// and an immature config (high bar), and across the diverse / mixed-domain / slashed fixtures. This
// proves the streaming refactor reproduces the predicate byte-for-byte, not merely that it runs.
func TestRecomputeMatureNowStreaming_MatchesResidentAndFullNode(t *testing.T) {
	cases := []struct {
		name             string
		matureValidators int
		operatorMargin   int
		bonds            []maturityBond
		wantMature       bool
	}{
		{"diverse-mature", 2, 1, diverseBonds(), true},
		{"diverse-immature", 99, 1, diverseBonds(), false},
		{"mixed-domain-mature", 2, 1, mixedDomainBonds(), true},
		{"mixed-domain-immature", 99, 1, mixedDomainBonds(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := buildMaturityFixture(t, tc.matureValidators, tc.operatorMargin, tc.bonds)
			w := f.witnessFor(t)

			// Resident path.
			resident, rErr := f.c.RecomputeMatureNow(f.root, w)
			if rErr != nil {
				t.Fatalf("resident RecomputeMatureNow stalled: %v", rErr)
			}

			// Streaming path.
			streamed, sErr := f.c.RecomputeMatureNowStreaming(f.root, streamWitnessFor(w))
			if sErr != nil {
				t.Fatalf("streaming RecomputeMatureNowStreaming stalled: %v", sErr)
			}

			// Full node.
			full := f.c.matureNow()

			if streamed != resident {
				t.Fatalf("streamed verdict %v != resident verdict %v", streamed, resident)
			}
			if streamed != full {
				t.Fatalf("streamed verdict %v != full-node matureNow %v", streamed, full)
			}
			if streamed != tc.wantMature {
				t.Fatalf("streamed verdict %v != expected %v", streamed, tc.wantMature)
			}
		})
	}
}

// TestRecomputeMatureNowStreaming_ShortIDListStalls is the R-M-STREAM-COMPLETENESS RED ablation.
// It drops ONE member from the streamed id-list (a truncated/short list) and asserts the streaming
// path still STALLS ErrRecomputeSeenSetIncomplete. This proves the completeness check bites under
// streaming: the box reconstructs nodeSetMTH over the SHORT list, which differs from the committed
// validatorsSeenRoot, so it never folds a partial set. Streaming freed per-member proof heaps, NOT
// the id-list — the load-bearing completeness commitment is untouched.
//
// RED-BEFORE-GREEN: with the full id-list the same fixture reaches a verdict (proven by the
// equivalence test above); dropping a member is the single injected change that flips it to a stall.
func TestRecomputeMatureNowStreaming_ShortIDListStalls(t *testing.T) {
	f := buildMaturityFixture(t, 2, 1, diverseBonds())
	w := f.witnessFor(t)
	sw := streamWitnessFor(w)

	if len(sw.IDs) < 2 {
		t.Fatalf("fixture precondition: need >=2 ids to drop one, got %d", len(sw.IDs))
	}

	// Sanity: the FULL streamed id-list reaches a verdict (no stall). This is the "before" of the
	// red-before-green: the ablation is the ONLY change that flips it.
	if _, err := f.c.RecomputeMatureNowStreaming(f.root, sw); err != nil {
		t.Fatalf("precondition: full streamed id-list should reach a verdict, got stall: %v", err)
	}

	// Ablation: drop the last member from the id-list. The Member provider still HAS every member's
	// witness (streaming did not prune it) — only the id-LIST is short. This is exactly the
	// "streaming shortened the completeness input" hazard R-M-STREAM-COMPLETENESS guards.
	shortIDs := make([]ports.NodeID, len(sw.IDs)-1)
	copy(shortIDs, sw.IDs[:len(sw.IDs)-1])
	sw.IDs = shortIDs

	mature, err := f.c.RecomputeMatureNowStreaming(f.root, sw)
	if err == nil {
		t.Fatalf("short id-list must STALL (R-M-STREAM-COMPLETENESS), got mature=%v nil error", mature)
	}
	if !errors.Is(err, ErrRecomputeSeenSetIncomplete) {
		t.Fatalf("short id-list stalled with the wrong reason: got %v, want ErrRecomputeSeenSetIncomplete", err)
	}
	if mature {
		t.Fatalf("a stalled recompute must return mature=false, got true")
	}
}

// TestRecomputeMatureNowStreaming_ForgedMemberValueStalls asserts the per-member anchor survives
// streaming: a member pulled by the provider with a FORGED bonded weight fails its Resolve against
// the committed root and stalls ErrRecomputeMemberStateUnproven. Freeing proof heaps mid-fold does
// not change that every value is anchored to committedStateRoot.
func TestRecomputeMatureNowStreaming_ForgedMemberValueStalls(t *testing.T) {
	f := buildMaturityFixture(t, 2, 1, diverseBonds())
	w := f.witnessFor(t)

	// Pick a real seated member and forge its bonded weight in the delivered witness. Its BondedProof
	// still proves the COMMITTED weight, so the forged claim fails Resolve.
	target := f.members[0]
	forgedMembers := make(map[ports.NodeID]MemberStateWitness, len(w.Members))
	for id, mw := range w.Members {
		if id == target {
			mw.Bonded += 1 << 40 // forge the claimed weight; proof still commits the real value
		}
		forgedMembers[id] = mw
	}

	sw := SeenSetStreamWitness{
		IDs:             w.IDs,
		SeenRootWitness: w.SeenRootWitness,
		SeenRootValue:   w.SeenRootValue,
		Member: func(id ports.NodeID) (MemberStateWitness, bool) {
			mw, ok := forgedMembers[id]
			return mw, ok
		},
	}

	mature, err := f.c.RecomputeMatureNowStreaming(f.root, sw)
	if err == nil {
		t.Fatalf("forged bonded weight must STALL, got mature=%v nil error", mature)
	}
	if !errors.Is(err, ErrRecomputeMemberStateUnproven) {
		t.Fatalf("forged weight stalled with the wrong reason: got %v, want ErrRecomputeMemberStateUnproven", err)
	}
	if mature {
		t.Fatalf("a stalled recompute must return mature=false, got true")
	}
}

// TestRecomputeMatureNowStreaming_MissingMemberStalls asserts a member present in the id-list but
// NOT deliverable by the provider (Member returns ok=false) stalls exactly as a missing map entry
// does in the resident form — the box cannot verify that member's state, so it never folds a partial
// set. This holds the streaming provider to the same completeness obligation as the resident map.
func TestRecomputeMatureNowStreaming_MissingMemberStalls(t *testing.T) {
	f := buildMaturityFixture(t, 2, 1, diverseBonds())
	w := f.witnessFor(t)

	target := f.members[0]
	sw := SeenSetStreamWitness{
		IDs:             w.IDs,
		SeenRootWitness: w.SeenRootWitness,
		SeenRootValue:   w.SeenRootValue,
		Member: func(id ports.NodeID) (MemberStateWitness, bool) {
			if id == target {
				return MemberStateWitness{}, false // provider cannot deliver this member
			}
			mw, ok := w.Members[id]
			return mw, ok
		},
	}

	mature, err := f.c.RecomputeMatureNowStreaming(f.root, sw)
	if err == nil {
		t.Fatalf("undeliverable member must STALL, got mature=%v nil error", mature)
	}
	if !errors.Is(err, ErrRecomputeMemberStateUnproven) {
		t.Fatalf("undeliverable member stalled with the wrong reason: got %v, want ErrRecomputeMemberStateUnproven", err)
	}
	if mature {
		t.Fatalf("a stalled recompute must return mature=false, got true")
	}
}

// TestRecomputeMatureNowStreaming_NilProviderStalls asserts a streaming witness with a nil Member
// provider stalls (never-Accept) rather than panicking. A nil provider can deliver no member, so the
// safe default is to stall exactly as a resident witness with no Members map would. This holds even
// though the completeness MTH passed — the box must never proceed to a verdict it cannot anchor.
func TestRecomputeMatureNowStreaming_NilProviderStalls(t *testing.T) {
	f := buildMaturityFixture(t, 2, 1, diverseBonds())
	w := f.witnessFor(t)

	sw := SeenSetStreamWitness{
		IDs:             w.IDs,
		SeenRootWitness: w.SeenRootWitness,
		SeenRootValue:   w.SeenRootValue,
		Member:          nil, // no provider
	}

	mature, err := f.c.RecomputeMatureNowStreaming(f.root, sw)
	if err == nil {
		t.Fatalf("nil Member provider must STALL, got mature=%v nil error", mature)
	}
	if !errors.Is(err, ErrRecomputeMemberStateUnproven) {
		t.Fatalf("nil provider stalled with the wrong reason: got %v, want ErrRecomputeMemberStateUnproven", err)
	}
	if mature {
		t.Fatalf("a stalled recompute must return mature=false, got true")
	}
}
