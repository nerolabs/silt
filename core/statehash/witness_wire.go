package statehash

import (
	"errors"
	"fmt"

	"github.com/pokt-network/smt"
)

// Wire encoding for a Witness.
//
// A Witness wraps an untrusted sparse-Merkle proof, and the wrap is deliberate: the proof
// is reachable only through Resolve, against a specific committed root. That is exactly the
// property a witness-serving transport must not break, so the encoding here is byte-level
// only — it moves a proof between two processes and grants no new way to read one. A decoded
// witness is no more trusted than an encoded one: both are verified by Resolve against the
// root the reader already holds, and a forged or corrupted proof fails there rather than
// here.
//
// The underlying proof is three byte slices (side nodes, non-membership leaf data, sibling
// data) and carries its own Marshal/Unmarshal, so this is a delegation rather than a second
// format. Keeping it a delegation matters: a hand-rolled encoding here could drift from what
// the verifier expects and turn a real proof into a stall.

// ErrWitnessProofTooLarge is UnmarshalWitness's refusal of an encoded proof above the
// supplied ceiling. A witness arrives from an untrusted server, so its size is an
// attacker-chosen quantity: decoding it allocates, and a floor box that allocated whatever
// a peer claimed would have handed that peer its memory ceiling. The caller supplies the
// bound because only the caller knows its budget.
var ErrWitnessProofTooLarge = errors.New("statehash: encoded witness proof exceeds the supplied byte ceiling — refused before decoding")

// MarshalBinary encodes the witness's proof. A nil (absent) witness encodes as an empty
// slice, which UnmarshalWitness returns as an absent witness again — "I have no witness" is
// a legal, expected answer that must survive the round trip as itself, not as an error.
func (w Witness) MarshalBinary() ([]byte, error) {
	if w.proof == nil {
		return nil, nil
	}
	b, err := w.proof.Marshal()
	if err != nil {
		return nil, fmt.Errorf("statehash: marshal witness proof: %w", err)
	}
	return b, nil
}

// UnmarshalWitness decodes a witness proof, refusing anything above maxBytes before it
// allocates. maxBytes must be positive; a non-positive ceiling is refused rather than read
// as "unlimited", for the same reason the floor box's byte budget has no infinite value.
//
// An empty input decodes to an absent witness (Witness.IsNil), which resolves to NoWitness
// and stalls the read that wanted it. That is the correct outcome for a server that does not
// hold the leaf, and it is why an empty input is not an error.
func UnmarshalWitness(b []byte, maxBytes int) (Witness, error) {
	if maxBytes <= 0 {
		return Witness{}, fmt.Errorf("%w: ceiling must be positive, got %d", ErrWitnessProofTooLarge, maxBytes)
	}
	if len(b) > maxBytes {
		return Witness{}, fmt.Errorf("%w: %d bytes against a %d-byte ceiling", ErrWitnessProofTooLarge, len(b), maxBytes)
	}
	if len(b) == 0 {
		return Witness{}, nil // absent witness — the expected "I have no witness" answer
	}
	var p smt.SparseMerkleProof
	if err := p.Unmarshal(b); err != nil {
		return Witness{}, fmt.Errorf("statehash: unmarshal witness proof: %w", err)
	}
	return NewWitness(&p), nil
}
