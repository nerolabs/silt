package statehash

// R3.1 — the two substantive gates of the Researcher's certification
// (R3.1-SMT-domain-separation-disjoint-preimage-RESEARCH-CERTIFICATION-2026-09-06, §7):
// G-R31-1 the fold BINDS every delete sibling (digest == SHA-256(preimage)) before seeding the
// library's node store; G-R31-2 a proof whose NonMembershipLeafData does not begin 0x00 is
// refused BEFORE VerifyProof, because the library enforces the prefix with panic and core/ has
// no recover(). Each carries its positive control: the raw library call panics on the witness,
// and the fold accepts the unbound sibling with the check removed.

import (
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
	"github.com/pokt-network/smt"
)

// TestR31UnboundDeleteSiblingStallsTheFold (G-R31-1): a delete op whose DeleteSiblings carries a
// (Digest, Preimage) pair where Digest != SHA-256(Preimage) stalls the fold with
// ErrFoldSiblingUnbound, before any library surgery. Control: the same op with the pair BOUND
// (Digest = SHA-256(Preimage)) is accepted by the binding (it may still stall downstream on the
// root equality — that is the pre-existing catch this gate sits in front of).
func TestR31UnboundDeleteSiblingStallsTheFold(t *testing.T) {
	leaves := []Leaf{{Key: Key("a\x00", []byte("k1")), Value: []byte("v1")}, {Key: Key("a\x00", []byte("k2")), Value: []byte("v2")}}
	root, err := Root(leaves)
	if err != nil {
		t.Fatal(err)
	}
	prover, err := NewProver(leaves)
	if err != nil {
		t.Fatal(err)
	}
	proof, _, err := prover.ProveWithSiblings(leaves[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	forged := []byte{0x01}
	forged = append(forged, make([]byte, 64)...) // a plausible 65-byte inner preimage
	bogusDigest := make([]byte, 32)
	bogusDigest[0] = 0xEE // NOT sha256(forged)
	op := FoldOp{Key: leaves[0].Key, OldValue: leaves[0].Value, NewValue: nil, Proof: proof,
		DeleteSiblings: []FoldSibling{{Digest: bogusDigest, Preimage: forged}}}
	_, err = FoldChangedPaths(root, []FoldOp{op})
	if !errors.Is(err, ErrFoldSiblingUnbound) {
		t.Fatalf("an UNBOUND delete sibling was seeded into the node store (err %v) — that is the audit's Issue #2 on the verify side: a forged node with zero hash work", err)
	}
	// Control: bound pair passes the binding.
	sum := sha256.Sum256(forged)
	op.DeleteSiblings = []FoldSibling{{Digest: sum[:], Preimage: forged}}
	_, err = FoldChangedPaths(root, []FoldOp{op})
	if errors.Is(err, ErrFoldSiblingUnbound) {
		t.Fatalf("a BOUND sibling was refused by the binding: %v", err)
	}
	// A short digest is unbound too.
	op.DeleteSiblings = []FoldSibling{{Digest: sum[:16], Preimage: forged}}
	if _, err = FoldChangedPaths(root, []FoldOp{op}); !errors.Is(err, ErrFoldSiblingUnbound) {
		t.Fatalf("a 16-byte digest passed the binding: %v", err)
	}
}

// TestR31MalformedLeafPrefixIsRefusedNotPanicked (G-R31-2): the 33-byte witness — a proof whose
// NonMembershipLeafData begins 0x01 — makes the RAW library call panic (the positive control,
// captured with recover), and Resolve / FoldChangedPaths refuse it as NoWitness / a stall
// without reaching the library.
func TestR31MalformedLeafPrefixIsRefusedNotPanicked(t *testing.T) {
	var root ports.Hash
	root[0] = 0x5a
	witness := &smt.SparseMerkleProof{NonMembershipLeafData: append([]byte{0x01}, make([]byte, 32)...)}
	key := Key("a\x00", []byte("absent"))

	// Positive control: the library itself PANICS on this shape.
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = smt.VerifyProof(witness, root[:], key, nil, verifySpec())
	}()
	if !panicked {
		t.Fatalf("control: the raw library VerifyProof did not panic on a 0x01-prefixed NonMembershipLeafData — if the library now returns an error instead, this gate's premise changed; re-read R3.1 §4.7")
	}

	// The product path refuses it BEFORE the library: NoWitness, no panic.
	if r := Resolve(root, key, nil, NewWitness(witness)); r.outcome != NoWitness {
		t.Fatalf("Resolve returned %v on the panic-shaped witness, want NoWitness", r.outcome)
	}
	op := FoldOp{Key: key, OldValue: nil, NewValue: []byte("v"), Proof: NewWitness(witness)}
	if _, err := FoldChangedPaths(root, []FoldOp{op}); !errors.Is(err, ErrFoldProofShape) {
		t.Fatalf("FoldChangedPaths did not stall with ErrFoldProofShape on the panic-shaped proof: %v", err)
	}
	// And a WELL-formed absence proof is not caught by the shape check (it proceeds to
	// VerifyProof, which fails against this bogus root → NoWitness, not a shape error).
	good := &smt.SparseMerkleProof{NonMembershipLeafData: append([]byte{0x00}, make([]byte, 64)...)}
	if _, err := FoldChangedPaths(root, []FoldOp{{Key: key, NewValue: []byte("v"), Proof: NewWitness(good)}}); errors.Is(err, ErrFoldProofShape) {
		t.Fatalf("a 0x00-prefixed leaf blob was refused by the shape check")
	}
}

// TestR31MalformedSiblingDataIsRefusedNotPanicked (PE S1): the panic also reaches through
// SiblingData — validateBasic hashes it unbounded when SideNodes is non-empty. Three shapes
// panic the raw library (controls captured with recover): an empty SiblingData, and a 0x02
// (extension) SiblingData shorter than 35 bytes (one byte, and 34 bytes). Resolve, the fold
// and IngestBlockWitnesses each refuse them without reaching the library; a 35-byte 0x02
// SiblingData is NOT refused by the shape check (the library parses it).
func TestR31MalformedSiblingDataIsRefusedNotPanicked(t *testing.T) {
	var root ports.Hash
	root[0] = 0x5a
	key := Key("a\x00", []byte("absent"))
	side := [][]byte{make([]byte, 32)}
	shapes := map[string][]byte{
		"empty":        {},
		"ext-1-byte":   {0x02},
		"ext-34-bytes": append([]byte{0x02}, make([]byte, 33)...),
	}
	for name, sib := range shapes {
		w := &smt.SparseMerkleProof{SideNodes: side, SiblingData: sib}
		panicked := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					panicked = true
				}
			}()
			_, _ = smt.VerifyProof(w, root[:], key, nil, verifySpec())
		}()
		if !panicked {
			t.Fatalf("control (%s): the raw library did not panic on this SiblingData shape — the gate's premise changed; re-read R3.1", name)
		}
		if r := Resolve(root, key, nil, NewWitness(w)); r.outcome != NoWitness {
			t.Fatalf("%s: Resolve returned %v, want NoWitness", name, r.outcome)
		}
		if _, err := FoldChangedPaths(root, []FoldOp{{Key: key, NewValue: []byte("v"), Proof: NewWitness(w)}}); !errors.Is(err, ErrFoldProofShape) {
			t.Fatalf("%s: fold did not stall with ErrFoldProofShape: %v", name, err)
		}
		enc, err := w.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		res := IngestBlockWitnesses(root, []ReadEntry{{Key: key, Kind: QueryAbsent}}, []RawWitness{{Key: key, Encoded: enc}})
		if r, ok := res.Results[string(key)]; !ok || r.outcome != NoWitness {
			t.Fatalf("%s: IngestBlockWitnesses resolved %v, want NoWitness", name, r.outcome)
		}
	}
	// 35 bytes of 0x02 SiblingData is parsable: the shape check must not refuse it.
	ok35 := &smt.SparseMerkleProof{SideNodes: side, SiblingData: append([]byte{0x02}, make([]byte, 34)...)}
	if !proofShapeParsable(ok35) {
		t.Fatalf("a 35-byte extension SiblingData was refused by the shape check; the library parses it")
	}
}

// TestR31ForgedExtensionSiblingDoesNotBind is the false-ACCEPTANCE direction of G-R31-1 for the
// extension arm (PE code ruling, gate-coverage hole): an HONEST extension sibling, taken from
// a real trie's ProveWithSiblings, binds; the same preimage with a mutated child digest, or
// mutated bounds, does not. Without this, weakening the arm to "accept any 0x02 preimage under
// any digest" left the whole suite green.
func TestR31ForgedExtensionSiblingDoesNotBind(t *testing.T) {
	// Search small tries until a delete's siblings include an extension preimage (0x02).
	var honest FoldSibling
	found := false
	for seed := byte(1); seed < 200 && !found; seed++ {
		var leaves []Leaf
		for i := byte(0); i < 6; i++ {
			leaves = append(leaves, Leaf{Key: Key("t\x00", []byte{seed, i}), Value: []byte{i + 1}})
		}
		prover, err := NewProver(leaves)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range leaves {
			_, sibs, err := prover.ProveWithSiblings(l.Key)
			if err != nil {
				t.Fatal(err)
			}
			for _, sib := range sibs {
				if len(sib.Preimage) > 0 && sib.Preimage[0] == 0x02 {
					honest, found = sib, true
					break
				}
			}
			if found {
				break
			}
		}
	}
	if !found {
		t.Fatalf("fixture: no extension sibling found in 200 small tries")
	}
	if foldDigestMismatch(honest.Digest, honest.Preimage) {
		t.Fatalf("an HONEST extension sibling (bounds %v) does not bind — the expansion replica is wrong", honest.Preimage[1:3])
	}
	forgedChild := append([]byte(nil), honest.Preimage...)
	forgedChild[len(forgedChild)-1] ^= 0x01
	if !foldDigestMismatch(honest.Digest, forgedChild) {
		t.Fatalf("an extension preimage with a FORGED child digest bound to the honest digest")
	}
	forgedBounds := append([]byte(nil), honest.Preimage...)
	forgedBounds[2]++ // end bound +1
	if !foldDigestMismatch(honest.Digest, forgedBounds) {
		t.Fatalf("an extension preimage with mutated bounds bound to the honest digest")
	}
	// And a bare 0x02 preimage under a random digest never binds.
	junk := append([]byte{0x02, 0, 4}, make([]byte, 64)...)
	if !foldDigestMismatch(honest.Digest, junk) {
		t.Fatalf("an arbitrary 0x02 preimage bound to an honest digest")
	}
}
