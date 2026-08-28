package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// The covering probe for the certified same-root intra-block bond-registration
// contention fix (Research certification
// same-root-intrablock-bondreg-contention-2026-08-28, resolution (a); closes its
// residual R2). This is the case the #618 disjoint-root fixture never built.
//
// The defect (RED, pre-fix): validateBondRegs deduped per-ValidatorID (seenReg)
// but never per-root, so a block carrying two PROVEN registrations from DISTINCT
// identities on the SAME root was ADMITTED. apply() (chain.go:2780-2790) then
// resolved the winner by intra-block SLICE ORDER: the first reg in the slice
// claimed the root, the second hit the already-claimed branch and earned
// nothing. Two honest replicas applying the identical block in a different
// BondReg order therefore committed a DIFFERENT bonded / bondRootOwner state —
// an order-dependent commit the era-3 SMT root cannot tolerate.
//
// The fix (GREEN, post-fix): a seenRoot dedup in validateBondRegs, a sibling of
// seenReg, run UNCONDITIONALLY (not behind the #506 gate). It rejects the block
// with ErrSharedRootInBlock, so the divergent input can never commit. There is
// then nothing order-dependent left to hash.
//
// The two binding caveats of the certification are exercised here:
//   - the guard is NOT gate-gated (this world configures no RegGateActivationHeight,
//     so regGateActive is false — the collision must still be rejected);
//   - a validator RE-registering its OWN root (same ID, same root — renew/resize,
//     legal per F1) must still be ADMITTED (the negative control below).

// sameRootWorld builds a launch/objective world with a two-anchor quorum (a1
// proposes, a2 attests, era-1 blocks), configuring NO RegGateActivationHeight so
// regGateActive is false on the validated path — proving the seenRoot guard runs
// unconditionally. Returns the chain and the genesis block.
func sameRootWorld(t *testing.T) (*Chain, *Block, ed25519.PrivateKey, ed25519.PrivateKey) {
	t.Helper()
	const minBond = int64(2) << 20
	a1, a2 := key(9001), key(9002)
	id1, id2 := idOf(a1), idOf(a2)
	cfg := Config{Quorum: 1, MinBond: minBond,
		Anchors: map[ports.NodeID]bool{id1: true, id2: true}, MatureValidators: 100}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, a1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return c, g, a1, a2
}

// sameRootContentionBlock builds a height-1 era-1 block with two PROVEN bond
// registrations on rootShared from two DISTINCT identities, in the requested
// slice order, quorum-attested by anchor a2. Returns the chain, the block, and
// the two claimant ids.
func sameRootContentionBlock(t *testing.T, aFirst bool) (*Chain, *Block, ports.NodeID, ports.NodeID) {
	t.Helper()
	const minBond = int64(2) << 20
	rootShared := ports.HashBytes([]byte("same-root-intrablock-shared-plot"))

	c, g, a1, a2 := sameRootWorld(t)

	// Two DISTINCT identities, both proving the SAME root. Under the stub verifier
	// both proofs "verify" — the exact regime the freeze's proof engine runs in,
	// and the exact block a Byzantine proposer can construct regardless of real proofs.
	claimantA, claimantB := key(9101), key(9102)
	regA := bondRegAt(claimantA, rootShared, minBond, g.Hash())
	regB := bondRegAt(claimantB, rootShared, minBond, g.Hash())

	regs := []BondReg{regA, regB}
	if !aFirst {
		regs = []BondReg{regB, regA}
	}
	b := &Block{Version: 1, Height: 1, Prev: g.Hash(), BondRegs: regs}
	Sign(b, a1)                            // proposer = anchor a1 (launch-eligible while immature)
	b.Atts = append(b.Atts, Attest(b, a2)) // attester = anchor a2
	return c, b, idOf(claimantA), idOf(claimantB)
}

// TestSameRootDistinctIDIntraBlockRejected is the GREEN half: after the fix the
// block is REJECTED by validateBondRegs with ErrSharedRootInBlock, in BOTH
// intra-block orderings. Because the block never commits, the order-dependent
// divergence the RED capture exhibits can no longer occur.
func TestSameRootDistinctIDIntraBlockRejected(t *testing.T) {
	for _, tc := range []struct {
		name   string
		aFirst bool
	}{{"claimantA-first", true}, {"claimantB-first", false}} {
		t.Run(tc.name, func(t *testing.T) {
			c, b, _, _ := sameRootContentionBlock(t, tc.aFirst)
			err := c.Append(*b)
			if err == nil {
				_, next := c.Head()
				t.Fatalf("block with two distinct-ID proven regs on ONE root was ADMITTED — "+
					"the certified same-root dedup did not reject it. Head next-height now %d", next)
			}
			if !errors.Is(err, ErrSharedRootInBlock) {
				t.Fatalf("block was rejected, but not with ErrSharedRootInBlock: %v", err)
			}
			t.Logf("[%s] block rejected at the validity layer: %v", tc.name, err)
		})
	}
}

// TestSameRootDistinctIDNoDivergentCommit is the standing trip-wire proving the
// fix is load-bearing: with the seenRoot guard active BOTH orderings are rejected
// identically, so neither can commit a divergent bonded/bondRootOwner state. The
// contended root ends with NO owner and NEITHER claimant bonded in EITHER chain —
// order-independence restored by REJECTION, not by a tie-break.
//
// If a future change removed the guard, validateBondRegs would admit the block
// and apply() would commit different state across the two orderings — the exact
// divergence captured RED in the PR.
func TestSameRootDistinctIDNoDivergentCommit(t *testing.T) {
	cA, bA, idA, idB := sameRootContentionBlock(t, true)
	cB, bB, _, _ := sameRootContentionBlock(t, false)

	errA := cA.Append(*bA)
	errB := cB.Append(*bB)
	if errA == nil || errB == nil {
		t.Fatalf("a same-root distinct-ID block committed (errA=%v errB=%v) — the guard is not "+
			"rejecting it, so bonded/bondRootOwner can diverge by slice order", errA, errB)
	}

	if _, ok := cA.bonded[idA]; ok {
		t.Fatalf("claimantA is bonded after a rejected block (chain A)")
	}
	if _, ok := cA.bonded[idB]; ok {
		t.Fatalf("claimantB is bonded after a rejected block (chain A)")
	}
	if len(cA.bonded) != 0 || len(cB.bonded) != 0 {
		t.Fatalf("bonded not empty after rejection (A=%d B=%d) — no reg should have committed",
			len(cA.bonded), len(cB.bonded))
	}
}

// TestSameRootSameIDRenewAdmitted is the NEGATIVE CONTROL mandated by the
// certification: the dedup is on (root × DISTINCT-ID), NOT on root alone. A
// validator re-registering (renewing / resizing) its OWN root within one block —
// same ValidatorID, same root — is legitimate (F1) and MUST still be ADMITTED.
// If this block were rejected, the guard would be over-broad and would break a
// legal renewal.
func TestSameRootSameIDRenewAdmitted(t *testing.T) {
	const minBond = int64(2) << 20
	rootOwn := ports.HashBytes([]byte("same-root-same-id-renew"))

	c, g, a1, a2 := sameRootWorld(t)

	// One identity, TWO regs on its OWN root in one block: an initial claim and a
	// resize to a larger size. Both sign over the parent nonce. This is a legal
	// renew/resize (same ID), not a distinct-ID collision.
	owner := key(9202)
	ownerID := idOf(owner)
	reg1 := bondRegAt(owner, rootOwn, minBond, g.Hash())
	reg2 := bondRegAt(owner, rootOwn, minBond*2, g.Hash())
	b := &Block{Version: 1, Height: 1, Prev: g.Hash(), BondRegs: []BondReg{reg1, reg2}}
	Sign(b, a1)
	b.Atts = append(b.Atts, Attest(b, a2))

	if err := c.Append(*b); err != nil {
		t.Fatalf("same-ID same-root renew/resize was REJECTED — the seenRoot dedup is "+
			"OVER-BROAD; it must fire only on DISTINCT-ID collisions (F1 renew is legal): %v", err)
	}
	if c.bondRootOwner[rootOwn] != ownerID {
		t.Fatalf("owner does not hold its own root after a legal renew (owner=%x)", ownerID[:6])
	}
	if c.bonded[ownerID] != minBond*2 {
		t.Fatalf("resize did not take: bonded=%d want %d", c.bonded[ownerID], minBond*2)
	}
}
