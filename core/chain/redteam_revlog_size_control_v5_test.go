package chain

import (
	"crypto/sha256"
	"testing"

	"github.com/nerolabs/silt/core/translog"
	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// G-D9 — the m = 1 right-spine CONTROL for tagRevLogSize (R3.4), pre-registered
// =============================================================================
//
// THIS IS A RED-TEAM-STYLE CONTROL, NOT A REGRESSION GATE. It asserts that the UNSOUND construction
// is unsound, so nobody re-derives it a third time (P-table delta certification §3.3 and §9 item 6:
// the certifier derived "a witness-supplied m is safe under MTH size-injectivity" once, refuted it
// against the verifier's own code, and pre-registered this control). It ships with the P13b k ≥ 1
// STALL (provenView.CommittedRoots) and travels with tagRevLogSize when that leaf lands.
//
// THE CONSTRUCTION UNDER TEST: verify-not-recompute of a revocation-bearing block's LogRoot —
// derive n := m + k, check translog.VerifyConsistency(L_p, m, b.LogRoot, n, π) and
// VerifyInclusion(leaf_j, m+j, n, b.LogRoot, π_j) for each derived leaf — with m TAKEN FROM THE
// WITNESS. The two degeneracies in the verifier (translog.go): `m == 0` returns true without
// reading either root; and for `m` an exact power of two the path is seeded with oldRoot and at
// m = 1 the accumulator `fr` is never updated, so `fr == oldRoot` is vacuous and the only binding
// constraint is that newRoot folds up from oldRoot with attacker-chosen siblings — and newRoot is
// b.LogRoot, which the attacker chooses.
//
// RUNTIME GATE (the composition's stall, the round-1A disposition): TestGD8_RevocationBearingBlock-
// StallsOnTheProvenView. This control is what that stall is FOR.

// rfc6962Node is the RFC 6962 interior-node hash HASH(0x01 ‖ left ‖ right), re-derived here because
// translog does not export it; pinned below against translog.MTH over a two-element list.
func rfc6962Node(left, right ports.Hash) ports.Hash {
	b := append(append([]byte{0x01}, left[:]...), right[:]...)
	return ports.Hash(sha256.Sum256(b))
}

func rfc6962Leaf(entry ports.Hash) ports.Hash {
	return ports.Hash(sha256.Sum256(append([]byte{0x00}, entry[:]...)))
}

// verifyLogExtension is the verify-not-recompute construction, parameterised on WHERE m comes
// from. It is the shape T-LOGEXT (delta certification §3.3) conditions on: sound iff m is
// authenticated. `m` is the caller's claim of the parent log size; `leaves` are the block's derived
// revocation leaves in order; the proofs are witness-supplied.
func verifyLogExtension(parentRoot ports.Hash, m int, newRoot ports.Hash, leaves []ports.Hash,
	consistency []ports.Hash, inclusion [][]ports.Hash) bool {
	n := m + len(leaves)
	if !translog.VerifyConsistency(parentRoot, m, newRoot, n, consistency) {
		return false
	}
	for j, leaf := range leaves {
		if !translog.VerifyInclusion(leaf, m+j, n, newRoot, inclusion[j]) {
			return false
		}
	}
	return true
}

// TestGD9_WitnessSuppliedLogSizeIsUnsound_Control. The parent log has THREE entries (m_true = 3,
// not a power of two, so the honest path exercises the general branch). The attacker claims
// m = 1, forges b.LogRoot = node(L_p, leafHash(leaf)) with n = 2, and supplies the one-element
// proofs that make both legs pass. With m authenticated as 3, the identical input is REFUSED.
func TestGD9_WitnessSuppliedLogSizeIsUnsound_Control(t *testing.T) {
	// Pin the re-derived node hash against the package's own MTH before relying on it.
	a, b := ports.HashBytes([]byte("a")), ports.HashBytes([]byte("b"))
	if got, want := rfc6962Node(rfc6962Leaf(a), rfc6962Leaf(b)), translog.MTH([]ports.Hash{a, b}); got != want {
		t.Fatalf("CONTROL BROKEN: the re-derived RFC 6962 node hash does not match translog.MTH (%x != %x)", got[:8], want[:8])
	}

	// The honest parent log: three revocation leaves, derived the way the node derives them.
	log := translog.New()
	for i, r := range []string{"r0", "r1", "r2"} {
		log.Append(RevocationLeaf(RevOp, ports.HashBytes([]byte(r)), uint64(i+1)))
	}
	parentRoot, mTrue := log.Root(), log.Size()
	if mTrue != 3 {
		t.Fatalf("fixture: want a 3-entry parent log, got %d", mTrue)
	}
	// The block's one derived leaf (k = 1).
	leaf := RevocationLeaf(RevOp, ports.HashBytes([]byte("r3")), 4)

	// CONTROL, honest path: with the TRUE m the true extension verifies (so the construction is
	// not vacuously refusing everything).
	honest := log.Clone()
	honest.Append(leaf)
	cons, err := honest.ConsistencyProof(mTrue, honest.Size())
	if err != nil {
		t.Fatal(err)
	}
	incl, err := honest.InclusionProof(mTrue, honest.Size())
	if err != nil {
		t.Fatal(err)
	}
	if !verifyLogExtension(parentRoot, mTrue, honest.Root(), []ports.Hash{leaf}, cons, [][]ports.Hash{incl}) {
		t.Fatal("CONTROL BROKEN: the honest extension does not verify with the true m")
	}

	// THE ATTACK. The attacker claims m = 1 and publishes b.LogRoot = node(L_p, leafHash(leaf)):
	// a two-element "tree" whose left leaf hash happens to be the parent's 3-entry MTH. The
	// consistency proof is the single sibling leafHash(leaf); the inclusion proof for the derived
	// leaf at index 1 of size 2 is the single sibling L_p.
	forgedRoot := rfc6962Node(parentRoot, rfc6962Leaf(leaf))
	mClaimed := 1
	forgedCons := []ports.Hash{rfc6962Leaf(leaf)}
	forgedIncl := [][]ports.Hash{{parentRoot}}
	if forgedRoot == honest.Root() {
		t.Fatal("fixture VACUOUS: the forged root equals the honest post-root")
	}

	// (1) WITNESS-SUPPLIED m: the forgery PASSES both legs. This is the wrong-accept the
	// certifier's first derivation missed; it is the whole reason tagRevLogSize is a SAFETY leaf.
	if !verifyLogExtension(parentRoot, mClaimed, forgedRoot, []ports.Hash{leaf}, forgedCons, forgedIncl) {
		t.Fatal("CONTROL BROKEN: with a witness-supplied m = 1 the forged LogRoot must PASS VerifyConsistency + " +
			"the inclusion leg (translog.go: the isPow2 seeding leaves fr == oldRoot vacuous at m = 1). If this " +
			"now refuses, the verifier changed — re-read §3.3 before touching the stall.")
	}
	// The m == 0 short-circuit is the other degeneracy: it returns true without reading a root.
	if !translog.VerifyConsistency(parentRoot, 0, forgedRoot, 1, nil) {
		t.Fatal("CONTROL BROKEN: VerifyConsistency(m = 0) must return true with an empty proof, whatever the roots")
	}

	// (2) THE ABLATION, built in: authenticate m from the committed leaf (m = 3) and the SAME
	// input is REFUSED — n becomes 4, the forged proofs no longer fold, and the forged root is not
	// an extension of the parent's log.
	if verifyLogExtension(parentRoot, mTrue, forgedRoot, []ports.Hash{leaf}, forgedCons, forgedIncl) {
		t.Fatal("G-D9 VIOLATED: with m AUTHENTICATED the forged LogRoot must be REFUSED")
	}
	// And the honest proofs do not rescue the forged root either.
	if verifyLogExtension(parentRoot, mTrue, forgedRoot, []ports.Hash{leaf}, cons, [][]ports.Hash{incl}) {
		t.Fatal("G-D9 VIOLATED: the honest proofs must not verify the forged root")
	}
}
