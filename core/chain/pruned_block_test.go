package chain

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// mkBondRegBlock builds a height-1 block carrying one bond registration with a heavy
// ~Answer, for the pruning tests. Not signed unless the test signs it.
func mkBondRegBlock(answerLen int) Block {
	var root ports.Hash
	root[0] = 9
	return Block{
		Version: BlockVersion,
		Height:  1,
		BondRegs: []BondReg{{
			Validator: []byte("validator-key"),
			Root:      root,
			Size:      1 << 30,
			Answer:    bytes.Repeat([]byte{0xAB}, answerLen), // the heavy space-time proof
			Sig:       []byte("reg-sig"),
			Domain:    7,
		}},
	}
}

// TestBlockPrunePreservesHashAndDropsAnswer pins the Opt 1 pruned-block representation
// (PE ruling pruned-block-representation-ruling-PE-2026-08-18): a payload-selective
// prune drops the heavy BondReg.Answer (~1.5 MB) while KEEPING the block's hash (so it
// still links) and the light BondReg fields (Validator/Root/Size/Sig/Domain — needed
// by STATE + slashing). Because Hash() commits BondRegs, the pruned block must carry
// its pre-prune hash; Hash() returns that stored value.
func TestBlockPrunePreservesHashAndDropsAnswer(t *testing.T) {
	full := mkBondRegBlock(4096)
	if full.IsPruned() {
		t.Fatal("a freshly built block must not report as pruned")
	}
	want := full.Hash()

	pruned := full.Prune()

	if !pruned.IsPruned() {
		t.Fatal("Prune() must mark the block pruned")
	}
	if got := pruned.Hash(); got != want {
		t.Fatalf("pruned hash %x != full hash %x — pruning must preserve the hash (linkage/sigs depend on it)", got, want)
	}
	if pruned.BondRegs[0].Answer != nil {
		t.Fatalf("Prune() must drop the heavy BondReg.Answer, got %d bytes", len(pruned.BondRegs[0].Answer))
	}
	// The light fields the STATE/slashing paths read must survive.
	if pruned.BondRegs[0].Root != full.BondRegs[0].Root ||
		pruned.BondRegs[0].Size != full.BondRegs[0].Size ||
		!bytes.Equal(pruned.BondRegs[0].Validator, full.BondRegs[0].Validator) ||
		pruned.BondRegs[0].Domain != full.BondRegs[0].Domain {
		t.Fatal("Prune() must keep the light BondReg fields (Validator/Root/Size/Domain)")
	}
	// The original must be untouched (Prune returns a copy, does not mutate).
	if full.BondRegs[0].Answer == nil {
		t.Fatal("Prune() must not mutate the source block's Answer")
	}
}

// TestPrunedBlockSignatureStillVerifies is the linkage/accountability property: the
// proposer signature (made over the full block's hash) must still verify against the
// pruned block, because Hash() returns the same value. This is what lets a pruned
// block remain valid slashing evidence and keep chaining.
func TestPrunedBlockSignatureStillVerifies(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	full := mkBondRegBlock(2048)
	Sign(&full, priv)

	pruned := full.Prune()

	h := pruned.Hash()
	if !ed25519.Verify(pub, h[:], pruned.ProposerSig) {
		t.Fatal("proposer signature must still verify against the pruned block (Hash preserved)")
	}
	if !bytes.Equal(pruned.Proposer, pub) {
		t.Fatal("pruned block must keep the proposer key")
	}
}

// TestFullBlockHashIgnoresUnsetPrunedField guards backward compatibility: an unpruned
// block (Pruned unset) hashes from its body exactly as before the field existed — the
// new cbor field is omitempty and excluded from the Hash preimage, so existing blocks
// and signatures are unaffected. (The full-block-with-forged-Pruned decode guard lands
// with the Q2 gate in the consensus-adjacent slice.)
func TestFullBlockHashIgnoresUnsetPrunedField(t *testing.T) {
	full := mkBondRegBlock(1024)
	h1 := full.Hash()
	// Recomputing must be stable and body-derived while unpruned.
	if h2 := full.Hash(); h1 != h2 {
		t.Fatal("unpruned Hash() must be deterministic from the body")
	}
	// Dropping the Answer WITHOUT the stored-hash marker must change the hash — proving
	// the hash genuinely commits the Answer (so pruning could not just recompute).
	stripped := full
	stripped.BondRegs = []BondReg{full.BondRegs[0]}
	stripped.BondRegs[0].Answer = nil
	stripped.hashMemoSet = false // the copy keeps full's memo (#555); a stripped wire block decodes without one
	if stripped.Hash() == h1 {
		t.Fatal("Hash must commit BondReg.Answer — a bare strip should change it (else no stored hash would be needed)")
	}
}

// TestPrunedBlockHashDoesNotCoverCarrierOrStateRoot DOCUMENTS a property that holds TODAY. It is
// not a fix and it asserts no desired behaviour — it pins the measured truth so the stamp-raise /
// era-4-freeze checklist can find it by name.
//
// PRE-FREEZE / PRE-STAMP-RAISE CHECKLIST ITEM. Red-team RT-CARRIER-2; PE ruling
// RULING-floorbox-predicate-rederivation-structure-2026-09-03.md §6(b) ("test the invariant before
// the stamp raise; it is not a flip gate"). Do NOT change Hash() or Prune() to make this test go
// the other way: covering pruned bodies with the hash defeats pruning, which exists for
// build-immutable #8.
//
// WHAT IS TRUE, MEASURED HERE:
//
//	(1) Prune() drops ONLY BondReg.Answer. It KEEPS LastCommit and StateRoot.
//	(2) Hash() short-circuits for a pruned block and returns the stored b.Pruned.
//	(3) Therefore, on a pruned block, mutating LastCommit or StateRoot changes NOTHING that any
//	    signature covers: the proposer signature and every attester signature still verify. The
//	    Pruned field is a LINKAGE TOKEN, not a content commitment.
//	(4) The carrier is nonetheless the best-protected member of that set: validateCarrier runs on
//	    the reload disk-write path with no IsPruned skip and verifies over b.Prev, which pruning does
//	    not touch — so FABRICATING an entry still needs a real key. Only DROPPING entries is free.
//
// THE INVARIANT THIS DOCUMENTS (stated at chain.go's Hash()): a pruned block's integrity rests on
// the recompute chain to the first NON-pruned descendant plus trustFloor. NO CONSENSUS DECISION MAY
// DEPEND ON RE-READING THE BODY OF A PRUNED BLOCK.
func TestPrunedBlockHashDoesNotCoverCarrierOrStateRoot(t *testing.T) {
	prop, attester := key(91001), key(91002)

	parent := Block{Version: BlockVersionWitnessable, Height: 4, Entries: []ports.Entry{entry(1)}}
	Sign(&parent, prop)

	b := mkBondRegBlock(4096) // a heavy Answer: retention.go only prunes blocks with something to shed
	b.Version = BlockVersionWitnessable
	b.Height = 5
	b.Prev = parent.Hash()
	root := ports.HashBytes([]byte("committed-state-root"))
	b.StateRoot = &root
	b.LastCommit = []Attestation{AttestAt(&parent, attester, 0, PhasePrecommit)}
	Sign(&b, prop)
	b.Atts = []Attestation{AttestAt(&b, attester, 0, PhasePrecommit)}

	fullHash := b.Hash()
	if err := validateCarrier(&b); err != nil {
		t.Fatalf("fixture: the honest carrier must be valid, got %v", err)
	}

	pruned := b.Prune()

	// (1) Prune keeps both.
	if len(pruned.LastCommit) != len(b.LastCommit) {
		t.Fatalf("Prune() dropped LastCommit (%d -> %d) — the documented property has changed; re-read "+
			"RT-CARRIER-2 before adjusting this test", len(b.LastCommit), len(pruned.LastCommit))
	}
	if pruned.StateRoot == nil || *pruned.StateRoot != root {
		t.Fatal("Prune() dropped StateRoot — the documented property has changed")
	}
	if pruned.BondRegs[0].Answer != nil {
		t.Fatal("Prune() must still drop the heavy BondReg.Answer")
	}

	// (2)+(3) Mutating the pruned block's carrier and committed root leaves the hash and EVERY
	// signature verifying. This is the property; it is not a defect to fix here.
	forged := pruned
	forged.LastCommit = []Attestation{
		{PubKey: append([]byte(nil), pubOf(attester)...), Sig: make([]byte, ed25519.SignatureSize),
			Round: 0, Phase: PhasePrecommit},
	}
	other := ports.HashBytes([]byte("some-other-root"))
	forged.StateRoot = &other

	if got := forged.Hash(); got != fullHash {
		t.Fatalf("PROPERTY CHANGED: a mutated pruned block's Hash() moved (%x != %x). If Hash() now "+
			"covers pruned bodies, RT-CARRIER-2 is closed and this test should be replaced by the "+
			"stronger assertion — do not just delete it", got[:8], fullHash[:8])
	}
	fh := forged.Hash()
	if !ed25519.Verify(forged.Proposer, fh[:], forged.ProposerSig) {
		t.Fatal("PROPERTY CHANGED: the proposer signature no longer verifies over a mutated pruned block")
	}
	for i, a := range forged.Atts {
		if !verifyAtt(a, forged.Hash()) {
			t.Fatalf("PROPERTY CHANGED: attester signature %d no longer verifies over a mutated pruned block", i)
		}
	}

	// (4) The carrier's own validity rule still runs on a pruned block and still binds to b.Prev,
	// so a ZERO-SIGNATURE entry is refused. That is ALL it refuses: an entry harvested from the
	// parent's published Atts is a genuine precommit over b.Prev and is ACCEPTED — adding is as
	// free as dropping (RT2-CARRIER-14). An earlier version of this clause read the refusal below
	// as "fabricating an entry needs a real key"; it does not. See TestGD11_… for both sides.
	if err := validateCarrier(&forged); err == nil {
		t.Fatal("PROPERTY CHANGED: validateCarrier accepted a ZERO-signature carrier entry on a pruned block")
	}
}

// TestGD11_PrunedCarrierRewriteIsCaughtOnlyByTheDescendant is the TWO-SIDED pruned-carrier gate
// (P-table delta certification §5.2 / G-D11; floor-box structure round 1A step 12).
//
// THE CLAIM IT PINS AS FALSE: "fabricating a carrier entry still needs a real key; only dropping
// is free" (chain.go, Hash(), three times). validateCarrier accepts an entry iff it is a genuine
// PhasePrecommit over b.Prev — the PARENT's hash — and the parent's own published Atts are exactly
// that, on every replica's disk. So an attacker rewrites a PRUNED block's LastCommit with entries
// harvested from the parent's Atts, recomputes its (uncovered) StateRoot with the real apply(),
// and every signature still verifies because Hash() returns b.Pruned unchanged. No key material.
//
// Side (i): validateCarrier ACCEPTS the harvested-Atts rewrite.
// Side (ii): a node replaying the rewritten history (Reload → appendStructural) accepts the pruned
// block, applies the forged seating, and refuses the FIRST NON-PRUNED DESCENDANT on its hash-
// covered StateRoot — the only catch — leaving its head SILENTLY TRUNCATED at the rewritten block
// with the forged seat live in validatorsSeen.
// Ablation (G-D11): remove validateEra3Roots from appendStructural ⇒ the forgery survives ⇒ RED.
// R-CARRIER-PRUNED-HASH stays OPEN; this gate bounds it, it does not close it.
func TestGD11_PrunedCarrierRewriteIsCaughtOnlyByTheDescendant(t *testing.T) {
	// A ValidateCommit-driven era-4 world: heights 1 and 2 commit with EMPTY carriers and the
	// victim (bonded, qualified, never seated) signs every certificate — so the parent's published
	// Atts carry the victim's genuine precommit, which is the harvest.
	w := rtGateWorldWith(t, 0, 1, 68000)
	victim := idOf(w.victims[0])
	h3 := w.mintEmptyCarrier(t) // the block the attacker will prune and rewrite
	mustAppend(t, w.c, h3)
	h4 := w.mintEmptyCarrier(t) // the first non-pruned descendant: its StateRoot is hash-covered
	mustAppend(t, w.c, h4)
	if w.c.validatorsSeen[victim] {
		t.Fatal("fixture VACUOUS: the victim is already seated; a forged seat would be idempotent")
	}
	honest := w.c.Blocks(0)
	if len(honest) != 5 {
		t.Fatalf("fixture: want genesis..h4, got %d blocks", len(honest))
	}
	parent := honest[2] // h2, the parent of h3

	// THE ATTACK. Prune h3, harvest the parent's real precommits into its carrier, recompute its
	// (now uncovered) StateRoot with the real apply() over the honest h2 state.
	pruned := honest[3].Prune()
	var harvested []Attestation
	for _, a := range parent.Atts {
		if a.Phase == PhasePrecommit && a.AttesterID() == victim {
			harvested = append(harvested, a)
		}
	}
	if len(harvested) != 1 {
		t.Fatalf("fixture: the parent's Atts must carry the victim's precommit, got %d", len(harvested))
	}
	rewritten := pruned
	rewritten.LastCommit = harvested
	at2 := New(w.cfg, func(ports.NodeID) int64 { return 0 })
	at2.SetBondVerifier(objectiveVerify)
	if n, err := at2.Reload(honest[:3]); err != nil || n != 3 {
		t.Fatalf("fixture: replay to h2: %d %v", n, err)
	}
	forgedState, forgedLog, err := at2.postApplyRoots(rewritten)
	if err != nil {
		t.Fatal(err)
	}
	rewritten.StateRoot, rewritten.LogRoot = &forgedState, &forgedLog
	if *rewritten.StateRoot == *honest[3].StateRoot {
		t.Fatal("fixture VACUOUS: the harvested carrier did not move the committed root (the victim was not seated)")
	}
	if rewritten.Hash() != honest[3].Hash() {
		t.Fatal("PROPERTY CHANGED: a mutated pruned block's Hash() moved — RT-CARRIER-2 is closed; rewrite this gate")
	}

	// SIDE (i): the carrier rule ACCEPTS the harvested rewrite. No key material was used.
	if err := validateCarrier(&rewritten); err != nil {
		t.Fatalf("G-D11 (i): validateCarrier must ACCEPT a carrier harvested from the parent's real Atts — "+
			"that is what makes adding as free as dropping; got %v", err)
	}

	// SIDE (ii): a node replaying [g, h1, h2, rewritten h3, h4] accepts the rewrite and is caught
	// ONLY at h4 — on the hash-covered StateRoot — with its head silently truncated at h3 and the
	// forged seat live.
	replay := New(w.cfg, func(ports.NodeID) int64 { return 0 })
	replay.SetBondVerifier(objectiveVerify)
	history := append(append([]Block{}, honest[:3]...), rewritten, honest[4])
	n, err := replay.Reload(history)
	if n != 4 || !errors.Is(err, ErrEra3StateRootMismatch) {
		t.Fatalf("G-D11 (ii): the rewritten pruned ancestor must be ACCEPTED and the first non-pruned descendant "+
			"REFUSED on its committed StateRoot (want n=4, ErrEra3StateRootMismatch); got n=%d err=%v", n, err)
	}
	if _, next := replay.Head(); next != 4 {
		t.Fatalf("G-D11 (ii): the head must be SILENTLY TRUNCATED at the rewritten block (next height 4); got %d", next)
	}
	if !replay.validatorsSeen[victim] {
		t.Fatal("G-D11 (ii): the forged seat must be LIVE in the replayed state — the descendant catch does not undo it")
	}
	// The honest history replays whole: the truncation is the forgery's, not the fixture's.
	clean := New(w.cfg, func(ports.NodeID) int64 { return 0 })
	clean.SetBondVerifier(objectiveVerify)
	if n, err := clean.Reload(honest); err != nil || n != 5 {
		t.Fatalf("control: the honest history must replay whole; got n=%d err=%v", n, err)
	}
}
