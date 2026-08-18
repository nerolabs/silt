package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Slice 3 — the Q2 pruned-tolerance gate (the C1/M0 merge gate). A payload-pruned
// (Answer-less) block cannot have its space-time proof re-verified, so a node must
// trust one ONLY strictly below its OWN finalized/checkpoint anchor (trustFloor) and
// REJECT one at/above it — else a peer strips Answer to skip verification and forge
// standing (a no-discount break). These oracles are RED against the current tree (no
// gate: a pruned block's nil Answer fails verifyBond with ErrBadBondReg, the wrong
// reason, and below the floor is rejected when it should be trusted) and GREEN with the
// gate. Plan: docs/thinking/2026-08-18-slice3-q2-gate-plan.md. PE ruling:
// principle-engineer/pruned-block-representation-ruling-PE-2026-08-18.md.

// q2Chain builds an objective replica with its pruned-tolerance floor pinned to `floor`
// (the node's OWN anchor). objective() is true (MinBond>0 + a bond verifier), so
// validateBondRegs runs the per-reg space-time check on a full block and the Q2 gate on
// a pruned one.
func q2Chain(floor uint64) *Chain {
	c := New(Config{Quorum: 1, MinBond: 1 << 20}, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	f := floor
	c.trustFloorOverride = &f
	return c
}

// prunedRegBlockAt builds a height-h block carrying one verifier-accepted bond reg, then
// payload-prunes it (drops the heavy Answer, stores the pre-prune hash). IsPruned true,
// Answer nil.
func prunedRegBlockAt(h uint64) Block {
	v := key(int64(7000) + int64(h))
	b := Block{Version: BlockVersion, Height: h,
		BondRegs: []BondReg{bondReg(v, twoMiB, ports.Hash{})}}
	return b.Prune()
}

// TestQ2_PrunedAtOrAboveFloorRejected is the C1 catch: a pruned block presented AT or
// ABOVE the node's trust floor must be refused with ErrPrunedAboveHorizon — trusting it
// would skip the space-time proof at a height the node has not itself finalized.
func TestQ2_PrunedAtOrAboveFloorRejected(t *testing.T) {
	c := q2Chain(10)

	atFloor := prunedRegBlockAt(10) // height == floor → NOT strictly below → reject
	if err := c.validateBondRegs(&atFloor); !errors.Is(err, ErrPrunedAboveHorizon) {
		t.Fatalf("pruned block AT the trust floor must be rejected with ErrPrunedAboveHorizon, got %v", err)
	}

	aboveFloor := prunedRegBlockAt(25) // height > floor → reject
	if err := c.validateBondRegs(&aboveFloor); !errors.Is(err, ErrPrunedAboveHorizon) {
		t.Fatalf("pruned block ABOVE the trust floor must be rejected with ErrPrunedAboveHorizon, got %v", err)
	}
}

// TestQ2_PrunedBelowFloorAccepted: strictly below the floor the reg is finalized-trusted,
// so the space-time re-verify is legitimately SKIPPED and the block is accepted (the nil
// Answer must NOT reach verifyBond). RED today (verifyBond(nil) → ErrBadBondReg).
func TestQ2_PrunedBelowFloorAccepted(t *testing.T) {
	c := q2Chain(100)
	below := prunedRegBlockAt(50) // strictly below the floor
	if err := c.validateBondRegs(&below); err != nil {
		t.Fatalf("pruned block strictly below the trust floor must be accepted (re-verify skipped), got %v", err)
	}
}

// TestQ2_FreshNodeTrustsNoPrunedBlock: a node with no finality and no checkpoint has
// trustFloor 0, so it trusts NO pruned block — even at height 0, nothing is strictly
// below 0. The safe default (a fresh node re-verifies everything it did not finalize).
func TestQ2_FreshNodeTrustsNoPrunedBlock(t *testing.T) {
	c := q2Chain(0)
	b := prunedRegBlockAt(0)
	if err := c.validateBondRegs(&b); !errors.Is(err, ErrPrunedAboveHorizon) {
		t.Fatalf("a fresh node (floor 0) must trust no pruned block, got %v", err)
	}
}

// TestQ2_MalformedPrunedWithAnswerRejected is the decode-invariant belt: a block marked
// pruned (Pruned set) that STILL carries a BondReg.Answer is malformed — a full block
// cannot smuggle a forged stored-hash past the skip. Rejected with ErrMalformedPruned
// even below the floor. RED today (no belt).
func TestQ2_MalformedPrunedWithAnswerRejected(t *testing.T) {
	c := q2Chain(100)
	b := prunedRegBlockAt(50)          // pruned, below floor (would otherwise be accepted)
	b.BondRegs[0].Answer = []byte("x") // smuggle an Answer back in
	if err := c.validateBondRegs(&b); !errors.Is(err, ErrMalformedPruned) {
		t.Fatalf("a pruned block carrying a BondReg.Answer must be rejected with ErrMalformedPruned, got %v", err)
	}
}

// TestQ2_FullBlockUnaffectedAtAnyHeight: a full (unpruned) block is verified in full at
// any height — the gate only diverts the IsPruned() path, so normal validation is
// unchanged. A verifier-accepted reg passes; a bad one still fails ErrBadBondReg.
func TestQ2_FullBlockUnaffectedAtAnyHeight(t *testing.T) {
	c := q2Chain(10)
	v := key(4242)
	good := Block{Version: BlockVersion, Height: 5,
		BondRegs: []BondReg{bondReg(v, twoMiB, ports.Hash{})}}
	if err := c.validateBondRegs(&good); err != nil {
		t.Fatalf("a full verifier-accepted reg must pass at any height, got %v", err)
	}
	bad := good
	bad.BondRegs = []BondReg{bondReg(v, twoMiB, ports.Hash{})}
	bad.BondRegs[0].Answer = []byte("not-valid") // objectiveVerify rejects
	if err := c.validateBondRegs(&bad); !errors.Is(err, ErrBadBondReg) {
		t.Fatalf("a full reg with a bad proof must fail ErrBadBondReg, got %v", err)
	}
}

// TestQ2_ReconcileGatesPrunedInReplay proves the gate is wired into the Reconcile replay
// AND uses the RECEIVER's floor: a receiver whose own floor is 0 (trusts nothing pruned)
// must refuse a fork that contains a pruned block — the peer cannot get its Answer-less
// block trusted no matter what the fork claims. RED today (Reconcile's tmp replay hits
// verifyBond(nil) → adopts-or-errors on the wrong basis).
func TestQ2_ReconcileGatesPrunedInReplay(t *testing.T) {
	prop := key(1)
	vals := []ed25519.PrivateKey{key(2), key(3), key(4), key(5)}
	c, g := objectiveChain(prop, vals, func(ports.NodeID) int64 { return 0 })

	// Pin the receiver's floor to 0 (a fresh node trusts no pruned history). Reconcile
	// must thread THIS into the tmp replica, so the fork's own (replayed) state cannot
	// raise the height at which a pruned block is trusted.
	zero := uint64(0)
	c.trustFloorOverride = &zero

	// A height-1 block carrying a NEW validator's bond reg — the exact payload a pruned
	// representation lets a peer skip verifying. Signed + attested so it passes every
	// check EXCEPT the space-time re-verify the prune removes (a FULL such block would
	// validate; only the dropped Answer distinguishes it).
	newv := key(6001)
	full := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)},
		BondRegs: []BondReg{bondReg(newv, twoMiB, g.Hash())}}
	Sign(full, prop)
	for _, val := range vals[:3] {
		full.Atts = append(full.Atts, Attest(full, val))
	}
	pruned := full.Prune()
	fork := []Block{*g, pruned}

	_, err := c.Reconcile(fork)
	if !errors.Is(err, ErrPrunedAboveHorizon) {
		t.Fatalf("Reconcile must gate a pruned block in the replay against the RECEIVER's floor "+
			"(ErrPrunedAboveHorizon), got %v", err)
	}
}

// TestQ2_PrunedBlockStillSlashable is the I5 / accountable-safety corner Opt 1 exists to
// protect: payload-selective pruning keeps the header + consensus signatures, so a pruned
// block remains valid late-reveal equivocation evidence. A culprit who double-signed
// cannot escape the slash by having one (or both) conflicting blocks pruned — Hash()
// returns the stored pre-prune value and the sigs are preserved, so VerifyEquivocation
// still fires.
func TestQ2_PrunedBlockStillSlashable(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()
	a, b := w.conflicting(g, w.prop, w.vals[3], []ed25519.PrivateKey{w.vals[0]}, []ed25519.PrivateKey{w.vals[0]})

	if !VerifyEquivocation(&Equivocation{Culprit: pubOf(w.vals[0]), A: *a, B: *b}) {
		t.Fatal("precondition: the full double-sign must be provable")
	}
	// Both sides pruned — the heavy payload is gone, the proof still verifies.
	pa, pb := a.Prune(), b.Prune()
	if !VerifyEquivocation(&Equivocation{Culprit: pubOf(w.vals[0]), A: pa, B: pb}) {
		t.Fatal("a pruned block must remain valid late-reveal slashing evidence (I5) — " +
			"payload-selective pruning keeps header + sigs")
	}
	// The realistic late-reveal: one side already pruned, the accuser reveals the full other.
	if !VerifyEquivocation(&Equivocation{Culprit: pubOf(w.vals[0]), A: *a, B: pb}) {
		t.Fatal("a mixed full/pruned equivocation pair must still verify")
	}
}

// anchorObjectiveChain builds a 2-anchor objective chain committed up to head height
// topHeight (BondTTLBlocks 1 ⇒ safetyDepth 2), returning the chain and its blocks
// (genesis..topHeight) so a fork can be rebuilt with a chosen block pruned. Finality is
// active throughout, so RetentionHorizon/trustFloor are positive once deep enough (probed:
// finalizedHead 7 ⇒ floor 5).
func anchorObjectiveChain(t *testing.T, topHeight uint64) (*Chain, []Block) {
	t.Helper()
	a1, a2 := key(9000), key(9001)
	anchors := map[ports.NodeID]bool{idOf(a1): true, idOf(a2): true}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 1,
		MatureValidators: 99, BondTTLBlocks: 1}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, a1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	blocks := []Block{*g}
	prev := g.Hash()
	for h := uint64(1); h <= topHeight; h++ {
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		Sign(b, a1)
		b.Atts = []Attestation{Attest(b, a2)}
		if err := c.Append(*b); err != nil {
			t.Fatalf("append h%d: %v", h, err)
		}
		blocks = append(blocks, *b)
		prev = b.Hash()
	}
	return c, blocks
}

// TestQ2_ReconcileHonorsPositiveReceiverFloor exercises the gate AND the threading on the
// Reconcile path with a POSITIVE receiver floor (not just the fresh-node floor-0 case).
// Receiver: a 2-anchor chain at head 6 → trustFloor 5 (safetyDepth 2). The threading pins
// tmp's floor to THIS fixed 5 — NOT the incremental head-so-far during replay, which is
// what makes part (a) fail without the threading (proven load-bearing by removing the two
// threading lines: part (a) then RED).
func TestQ2_ReconcileHonorsPositiveReceiverFloor(t *testing.T) {
	c, blocks := anchorObjectiveChain(t, 6)
	if got := c.trustFloor(); got != 5 {
		t.Fatalf("precondition: receiver trustFloor = %d, want 5", got)
	}

	// (a) THREADING/liveness: a fork that legitimately prunes a block BELOW the floor
	// (h3 < 5) must NOT be gate-rejected during replay. WITH threading tmp uses the
	// receiver's fixed floor 5 (3 < 5 ⇒ trust); WITHOUT it, tmp's INCREMENTAL floor at h3
	// is 2 (3 ≥ 2 ⇒ reject) — so this assertion is the threading's load-bearing test.
	// Safety here rests on the finality gate: the pruned block's hash equals the receiver's
	// own finalized h3, so trusting it re-trusts already-verified history, not a peer claim.
	forkA := append([]Block(nil), blocks...)
	forkA[3] = blocks[3].Prune()
	if _, err := c.Reconcile(forkA); errors.Is(err, ErrPrunedAboveHorizon) {
		t.Fatalf("a fork pruning a block BELOW the receiver's fixed floor must NOT be gate-rejected "+
			"(threading pins tmp to the receiver's anchor, not its incremental replay head), got %v", err)
	}

	// (b) SECURITY: a fork pruning a block AT/ABOVE the floor (h5 ≥ 5) must be rejected —
	// the node keeps the last safetyDepth of full proofs and will not trust a pruned one there.
	forkB := append([]Block(nil), blocks...)
	forkB[5] = blocks[5].Prune()
	if _, err := c.Reconcile(forkB); !errors.Is(err, ErrPrunedAboveHorizon) {
		t.Fatalf("a fork pruning a block AT/ABOVE the receiver's floor must be rejected "+
			"with ErrPrunedAboveHorizon, got %v", err)
	}
}

// TestQ2_ReconcileFloorIsReceiversNotForks is the PE's merge-gate invariant (slice5
// ruling): the pruned-tolerance floor Reconcile uses is ALWAYS the receiver's OWN
// trustFloor, NEVER derived from the (peer-supplied) fork. A TALLER peer fork — whose own
// replayed history would imply a HIGHER floor — must not get a pruned block above the
// RECEIVER's floor accepted. Here receiver floor = 5, a height-12 peer fork's own floor
// would be 11; a pruned block at height 8 (5 ≤ 8 < 11) must be REJECTED (receiver's 5
// governs), not accepted (fork's 11). If the override were ever fork-derived this is the
// Q1 C1 break — a peer inflates the floor and slips pruned forgeries under it.
func TestQ2_ReconcileFloorIsReceiversNotForks(t *testing.T) {
	receiver, _ := anchorObjectiveChain(t, 6)
	if got := receiver.trustFloor(); got != 5 {
		t.Fatalf("precondition: receiver trustFloor = %d, want 5", got)
	}
	peer, peerBlocks := anchorObjectiveChain(t, 12) // deterministic ⇒ shares receiver's genesis+prefix
	if got := peer.trustFloor(); got <= 8 {
		t.Fatalf("precondition: taller peer's own floor = %d, want > 8 (so it would accept h8 pruned)", got)
	}

	fork := append([]Block(nil), peerBlocks...)
	fork[8] = peerBlocks[8].Prune() // pruned block ABOVE the receiver's floor, BELOW the peer's

	if _, err := receiver.Reconcile(fork); !errors.Is(err, ErrPrunedAboveHorizon) {
		t.Fatalf("a taller fork's pruned block above the RECEIVER's floor must be rejected "+
			"(the floor is the receiver's own, never the fork's), got %v", err)
	}
}

// TestReloadPrunedBlockRoundTrips guards the one-site finding (plan §load-bearing): the
// Reload/own-disk path (validateStructural) NEVER re-verifies bonds — it checks the
// proposer/attester sigs against Hash() (which returns the stored pre-prune hash for a
// pruned block) — so a pruned own-disk block replays with NO gate change there. This is
// a regression guard (expected GREEN pre- and post-gate); it fails only if someone adds a
// bond re-verify to the Reload path that would choke on the dropped Answer.
func TestReloadPrunedBlockRoundTrips(t *testing.T) {
	prop := key(1)
	vals := []ed25519.PrivateKey{key(2), key(3), key(4), key(5)}
	_, g := objectiveChain(prop, vals, func(ports.NodeID) int64 { return 0 })

	// A height-1 block carrying a real bond reg (the heavy payload). If validateStructural
	// re-verified bonds, the dropped Answer would fail it; it does not (Reload trusts own
	// history) — this is the one-site finding the gate relies on.
	newv := key(6001)
	full := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)},
		BondRegs: []BondReg{bondReg(newv, twoMiB, g.Hash())}}
	Sign(full, prop)
	for _, val := range vals[:3] {
		full.Atts = append(full.Atts, Attest(full, val))
	}

	// Replay THIS node's own history with the height-1 block pruned, into a fresh replica.
	pruned := full.Prune()
	fresh, _ := objectiveChain(prop, vals, func(ports.NodeID) int64 { return 0 })
	// objectiveChain already appended genesis; Reload the suffix onto it via appendStructural.
	if err := fresh.appendStructural(pruned); err != nil {
		t.Fatalf("a pruned own-disk block must replay through the structural (Reload) path "+
			"without a bond re-verify: %v", err)
	}
	// Head() returns the NEXT expected height; genesis(0) + the height-1 block ⇒ 2.
	if _, next := fresh.Head(); next != 2 {
		t.Fatalf("after replaying the pruned height-1 block the next height must be 2, got %d", next)
	}
}
