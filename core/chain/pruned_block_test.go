package chain

import (
	"bytes"
	"crypto/ed25519"
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

	// (4) The live containment: the carrier's own validity rule still runs on a pruned block and
	// still binds to b.Prev, so a FABRICATED entry is refused without a real key.
	if err := validateCarrier(&forged); err == nil {
		t.Fatal("CONTAINMENT LOST: validateCarrier accepted a fabricated carrier entry on a pruned " +
			"block. The reload path (appendStructural) relies on this: pruning must not make " +
			"carrier entries forgeable, only droppable.")
	}
}
