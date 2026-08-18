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
	if stripped.Hash() == h1 {
		t.Fatal("Hash must commit BondReg.Answer — a bare strip should change it (else no stored hash would be needed)")
	}
}
