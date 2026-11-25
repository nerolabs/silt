package chain

import (
	"bytes"
	"errors"

	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// FalseRepairProof is self-verifying evidence of a false repair claim: the claimant's
// own signature on "shard ShardID is leaf LeafIndex of object Root", beside a Merkle
// path from that Root showing the leaf at LeafIndex is a different shard. The object's
// root commits its leaves, so the claim is refuted by the root alone, and the claimant
// signed it. No knowledge of the object's geometry, manifest or holders is needed, so
// every replica (a floor box included) reaches the same verdict.
//
// It rides in a Block's Slashes as an Equivocation whose FalseRepair field is set and
// whose two blocks are empty, so the slash pipeline that evicts a double-signer (the
// size cap, the digest, the culprit's witness, the eviction itself) evicts a false
// claimant unchanged. A judge produces one when a claim names a shard id the manifest
// does not commit at the claimed leaf (core/node judgeRepairClaim). An honest caretaker
// never signs such a claim: it reads the id from a manifest it has already checked
// against the root.
type FalseRepairProof struct {
	Claim repairproof.SignedClaim `cbor:"1,keyasint"`
	// Leaf is the shard the object really commits at Claim.Claim.LeafIndex, and
	// Total and Path the inclusion proof of it under Claim.Claim.Root.
	Leaf  ports.Hash   `cbor:"2,keyasint"`
	Total int          `cbor:"3,keyasint"`
	Path  []ports.Hash `cbor:"4,keyasint"`
}

// ErrNotFalseRepair is evidence that does not prove a false repair claim.
var ErrNotFalseRepair = errors.New("chain: false-repair proof does not prove a false claim")

// checkFalseRepair verifies e as a false-repair slash under chainID. A conjunct of
// every condition below is required; any one failing refuses the slash, so a forged
// accusation can never evict an honest caretaker.
func checkFalseRepair(e *Equivocation, chainID ports.Hash) error {
	p := e.FalseRepair
	// The record carries a claim, not two blocks: blocks here would be a second
	// accusation hiding inside the first.
	empty := Encode(&Block{})
	if !bytes.Equal(Encode(&e.A), empty) || !bytes.Equal(Encode(&e.B), empty) {
		return ErrNotFalseRepair
	}
	// The culprit named is the key that signed the claim.
	if !bytes.Equal(e.Culprit, p.Claim.Pub) {
		return ErrNotFalseRepair
	}
	if err := p.Claim.Verify(chainID); err != nil {
		return ErrNotFalseRepair
	}
	c := p.Claim.Claim
	proof := manifest.Proof{Index: c.LeafIndex, Total: p.Total, Path: p.Path}
	if !manifest.VerifyProof(c.Root, p.Leaf, proof) {
		return ErrNotFalseRepair
	}
	if p.Leaf == ports.Hash(c.ShardID) {
		return ErrNotFalseRepair // the claim names the committed leaf: true, not false
	}
	return nil
}
