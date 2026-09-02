package chain

// R0.4b C3 re-break — BREAK 1 (F1), the WRONG-ACCEPT direction. Inversion of the red-team
// probe rt_c3_prune_accept_test.go (BREAK-D).
//
// The old defect: the box did not reproduce the height-driven pruneIssuerKeyCommit, so its
// recomputed post-root was the root of a state in which the pruned leaf SURVIVED. An attacker
// committing exactly that root produced a block a FULL NODE rejects and the BOX agrees with —
// a latent wrong-Accept the R1.8 flip would turn live.
//
// Post-close, the honest apply of a zero-registration block leaves the keyspace untouched, so
// the same forgery is expressed the other way round: the attacker commits a root in which the
// leaf was DELETED. Both tiers must reject it, and neither may reject the honest one.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestRTC3_ForgedPrunedRootIsRefusedByTheBox: a committed root whose only difference from the
// honest post-state is a DELETED issuerKeyCommit leaf — precisely the state the old
// height-driven prune produced — must not be agreed with.
func TestRTC3_ForgedPrunedRootIsRefusedByTheBox(t *testing.T) {
	f := buildPruneFixture(t, true)
	b := f.boundaryBlock(nil)

	honest := f.applyAndCommittedRoot(t, b)

	// The FORGED post-state: identical to the honest apply EXCEPT the epoch-0 issuerKeyCommit
	// leaf is gone (what the pre-fix height-driven prune did on this very block).
	forge := f.c.cloneForDryRun()
	forge.apply(b)
	if forge.issuerKeyCommit[0] == nil {
		t.Fatalf("BREAK-D REOPENED: the honest apply of a 0-registration block PRUNED the "+
			"epoch-0 bucket (height %d)", b.Height)
	}
	delete(forge.issuerKeyCommit, 0)
	forged, err := forge.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion: %v", err)
	}
	if forged == honest {
		t.Fatalf("probe bug: forged root equals the honest root")
	}

	w := f.witnessForBoundary(t, b)
	if err := f.c.RecomputeStateRootEntriesRevocations(f.prevRoot, forged, b, w); err == nil {
		t.Fatalf("BREAK-D REOPENED (WRONG-ACCEPT): the floor-box recompute AGREED with a "+
			"committed StateRoot %x that a full node rejects", forged[:8])
	}
}

// TestRTC3_ForgedRootDiffersOnlyInIssuerKeyCommit keeps the red-team's measurement of the
// attack's surface: the forged post-state differs from the honest one in exactly one leaf, so
// the gate above is testing the issuerKeyCommit blind spot and nothing else.
func TestRTC3_ForgedRootDiffersOnlyInIssuerKeyCommit(t *testing.T) {
	f := buildPruneFixture(t, true)
	b := f.boundaryBlock(nil)
	honestC := f.c.cloneForDryRun()
	honestC.apply(b)
	forgeC := f.c.cloneForDryRun()
	forgeC.apply(b)
	delete(forgeC.issuerKeyCommit, 0)
	tags := map[string]int{}
	for k := range committedLeafDiff(honestC, forgeC) {
		tags[tagOfKey(k)]++
	}
	if len(tags) != 1 || tags["issuerKeyCommit"] != 1 {
		t.Fatalf("expected the delta to be exactly one issuerKeyCommit leaf, got %v", tags)
	}
	var _ ports.Hash
}
