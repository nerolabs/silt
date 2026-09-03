package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #357 — fork-choice oscillation during the anchor→bonded ramp. The blind
// multi-region field run committed blocks then reorged them back to height 0.
// Research root-caused it (docs/reviews/... RESEARCH-RESPONSE): during bootstrap
// every fork's Weight() is ≈0 because anchors are attesterQualified but contribute
// bonded[id]=0, so heavier() falls through to its HEIGHT-BLIND head-hash tiebreak —
// and a genesis-only fork whose head hash sorts below the committed head is adopted,
// dropping committed blocks. The mature-regime sim never hit this (it tests stable,
// nonzero weights), which is why the two-substrate immutable caught it in the field.
//
// This is the deterministic bootstrap-ramp repro (research Ask 4). It asserts the
// §1 properties: (§1a) a committed anchor-attested chain carries real fork-choice
// weight during the ramp, and (invariant D) reconciling a genesis-only fork never
// drops a committed chain. Fails before the §1 fix (anchor bootstrap weight +
// height-aware tiebreak); passes after.
func commitRampChain(t *testing.T) (*Chain, *Block, []any) {
	t.Helper()
	a1, a2, a3, a4 := key(11), key(12), key(13), key(14)
	ids := map[ports.NodeID]bool{idOf(a1): true, idOf(a2): true, idOf(a3): true, idOf(a4): true}
	cfg := Config{Quorum: 2, MinBond: 1 << 20, Anchors: ids, MatureValidators: 99, ByzantineQuorum: true}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis, anchor-proposed, NO real bonds — the ramp regime (anchors are the
	// only trust; qualifiedCount()==0).
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, a1)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("append genesis: %v", err)
	}
	// Two anchor-attested committed blocks (quorum 2, anchors zero-bond).
	b1 := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)}}
	Sign(b1, a1)
	b1.Atts = []Attestation{Attest(b1, a2), Attest(b1, a3)}
	if err := c.Append(*b1); err != nil {
		t.Fatalf("commit block 1 (anchor-attested): %v", err)
	}
	b2 := &Block{Version: 1, Height: 2, Prev: b1.Hash(), Entries: []ports.Entry{entry(2)}}
	Sign(b2, a1)
	b2.Atts = []Attestation{Attest(b2, a2), Attest(b2, a3)}
	if err := c.Append(*b2); err != nil {
		t.Fatalf("commit block 2 (anchor-attested): %v", err)
	}
	_ = a4
	return c, g, nil
}

func TestForkChoiceRampCommittedChainOutweighsGenesis357(t *testing.T) {
	c, g, _ := commitRampChain(t)
	if _, h := c.Head(); h != 3 {
		t.Fatalf("setup: head should be at block-height 2 (Head()==3), got %d", h)
	}

	// §1a (the `Weight() > 0` assertion) was DELETED 2026-09-03 (R-FORKCHOICE-RAMP-GUARD,
	// O3 research recommendation §3.5 item 3): it asserted a property production does not
	// have and was green only because this fixture hand-builds Version: 1 blocks with era-1
	// attestations inside an objective config — a guard that passes only under an
	// attestation era no node mints is not a guard. Invariant D below is the part that
	// guards #357 and it stands. Under O3 Direction T the weight term itself is retired.

	// §2: the validator set N the Byzantine quorum is sized against must be the FIXED
	// anchor set during the ramp (4), NOT the live, zero-then-climbing qualifiedCount —
	// so RequiredQuorum can't shift block-to-block as registrations drain in (the
	// "0 of 2 gathered" stall). bftThreshold(4)=2 matches the field's "2 attestations".
	if n := c.validatorSetSize(); n != 4 {
		t.Fatalf("#357 §2: ramp quorum must be sized against the fixed anchor set (4), got %d", n)
	}
	if rq := c.RequiredQuorum(); rq != 2 {
		t.Fatalf("#357 §2: ramp RequiredQuorum should be bftThreshold(4)=2 (stable), got %d", rq)
	}

	// Invariant D (the direct #357 symptom): reconciling a genesis-only fork must
	// NEVER displace 2 committed blocks. Before the fix, if g.Hash() sorts below the
	// committed head hash, the zero-weight head-hash tiebreak adopts genesis (head → 0).
	adopted, err := c.Reconcile([]Block{*g})
	if adopted {
		t.Fatal("#357: a genesis-only fork must NOT displace committed blocks (adopted=true)")
	}
	// The §3 finality gate refuses it outright (ErrPreFinalityReorg); §1's weight would
	// also reject it as lighter. Either refusal is correct — the head must be preserved.
	if err != nil && !errors.Is(err, ErrPreFinalityReorg) {
		t.Fatalf("reconcile genesis-only fork: unexpected error %v (want nil or ErrPreFinalityReorg)", err)
	}
	if _, h := c.Head(); h != 3 {
		t.Fatalf("#357: committed head must be preserved at block-height 2 (Head()==3), got %d — the reorg-to-0 defect", h)
	}
}
