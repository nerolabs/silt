package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — tier 1 (in-package, chain-level, deterministic).
//
// This is the harness `docs/design/consensus-model-check.md` calls the "small-N
// exhaustive core": it drives the REAL Chain finality predicate (ValidateCommit under
// finalityQuorumActive) over an EXHAUSTIVE enumeration of adversarial signer coalitions
// and asserts the I1–I5 oracle (`docs/design/consensus-invariants.md`) holds for EVERY
// schedule — the complement of `integration/consensus` (which checks that specific
// scenarios converge). Where the field discovered #357/B2/#397/#402 one region at a
// time, this enumerates the same perimeter on a laptop in milliseconds.
//
// SCOPE — this file is the **I1 launch oracle**: the exhaustive
// no-two-disjoint-anchor-coalitions-finalize property over the N∈{3,4,5} launch regime,
// the invariant all four scars (#357/B2/#397/#402) violated at their core. The rest of
// the I1–I5 perimeter named in `docs/thinking/2026-08-15-406-model-check-approach.md` is
// now BUILT in sibling oracles (paths relative to the repo root):
//   - I1 in the MATURE weight-quorum regime (the B2 catch) —
//     core/chain/modelcheck_s1s2_mature_test.go, core/node/modelcheck_s1s2_mature_node_test.go;
//   - I3 set-constancy (mid-epoch bank adds no quorum weight) — core/chain/modelcheck_i3_test.go;
//   - I5 fork-choice determinism + honest-never-slashed (the #357/#397 catches) —
//     core/chain/modelcheck_i5_accountable_test.go, core/chain/modelcheck_i5_357_test.go,
//     core/node/modelcheck_tier2_test.go (through the real gather);
//   - I2 across a real restart — core/node/modelcheck_i2_exhaustive_test.go.
// The cross-cutting step-monitor over an arbitrary schedule is
// core/node/modelcheck_unified_oracle_test.go (#406).
// In-package so the oracle reads unexported state (finalityQuorumActive) directly — no
// production API change, not research-gated.
//
// FAILING-FIRST (DoD #2): temporarily reverting requiredLaunchAnchors to the pre-#402
// rule (objective launch needs 1 anchor) makes TestModelCheck_I1_LaunchNoDisjointFinality
// go RED — "disjoint anchor coalitions [0] and [1] BOTH finalize" — the exact #402 defect,
// caught on a laptop in milliseconds. At/after the fix (derived ⌊A/2⌋+1) it is GREEN. That
// controlled revert is the recorded proof it would have caught the scar without a field run.

// anchorSubsets returns every subset of [0,n) as a slice of index slices (2^n of them).
func anchorSubsets(n int) [][]int {
	out := make([][]int, 0, 1<<n)
	for mask := 0; mask < (1 << n); mask++ {
		var s []int
		for i := 0; i < n; i++ {
			if mask&(1<<i) != 0 {
				s = append(s, i)
			}
		}
		out = append(out, s)
	}
	return out
}

func disjoint(a, b []int) bool {
	seen := map[int]bool{}
	for _, x := range a {
		seen[x] = true
	}
	for _, x := range b {
		if seen[x] {
			return false
		}
	}
	return true
}

// finalAnchorCoalitions enumerates, at the contested height, EVERY (proposer, anchor-
// attester-set) over the A anchors — with ALL sybils also attesting — and returns the
// anchor-signer set of each coalition whose block PASSES ValidateCommit (i.e. would
// finalize). Sybils are included in every candidate precisely to prove they cannot
// substitute for anchor support: if a sybil-padded coalition finalizes, its anchor
// signer set is what we record, and the I1 check below is over anchors only.
func finalAnchorCoalitions(t *testing.T, c *Chain, ak, sk []ed25519.PrivateKey, prev ports.Hash, contentSeed byte) [][]int {
	t.Helper()
	var final [][]int
	for p := range ak { // proposer — anchor-only launch proposing (encoding B)
		others := make([]int, 0, len(ak)-1)
		for i := range ak {
			if i != p {
				others = append(others, i)
			}
		}
		for _, attMask := range anchorSubsets(len(others)) {
			b := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(contentSeed)}}
			Sign(b, ak[p])
			signers := []int{p}
			for _, oi := range attMask {
				ai := others[oi]
				b.Atts = append(b.Atts, Attest(b, ak[ai]))
				signers = append(signers, ai)
			}
			for _, s := range sk { // every sybil attests — must not help finality
				b.Atts = append(b.Atts, Attest(b, s))
			}
			if c.ValidateCommit(b) == nil {
				final = append(final, signers)
			}
		}
	}
	return final
}

// TestModelCheck_I1_LaunchNoDisjointFinality is the I1 oracle for the launch regime:
// over EVERY adversarial anchor coalition, no two DISJOINT anchor-signer sets may both
// finalize a block at one height — because two disjoint coalitions share no honest
// anchor, so both blocks would finalize and I1 (at most one final block per height) is
// broken. This single property subsumes the launch face of #357/#397/#402.
func TestModelCheck_I1_LaunchNoDisjointFinality(t *testing.T) {
	for _, A := range []int{4, 5, 3} { // even (the ⌈A/2⌉ off-by-one trap) and odd
		c, ak, sk := launch402(t, A, 8, 0) // AnchorQuorum=0: the derived rule must carry it alone
		prev, _ := c.Head()
		final := finalAnchorCoalitions(t, c, ak, sk, prev, 1)
		if len(final) == 0 {
			t.Fatalf("A=%d: no coalition finalized — the launch gate is unsatisfiable, not intersecting", A)
		}
		for i := 0; i < len(final); i++ {
			for j := i + 1; j < len(final); j++ {
				if disjoint(final[i], final[j]) {
					t.Fatalf("A=%d: I1 VIOLATION — disjoint anchor coalitions %v and %v BOTH finalize at one height "+
						"(two conflicting blocks can be committed → permanent partition). The launch quorum does not intersect.",
						A, final[i], final[j])
				}
			}
		}
		// Positive control: the honest 3-of-4-style majority DID finalize (the set is
		// non-empty AND intersecting), so this is intersection, not an unsatisfiable gate.
		t.Logf("A=%d: %d finalizing coalitions, all pairwise-intersecting (min anchor support = strict majority ⌊A/2⌋+1=%d)",
			A, len(final), A/2+1)
	}
}
