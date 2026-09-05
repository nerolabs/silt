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
