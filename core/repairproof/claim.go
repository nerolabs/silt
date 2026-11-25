package repairproof

// RepairClaim is the wire announcement a caretaker broadcasts after it
// reconstructs a lost shard and places the rebuilt copy on a fresh storage node
// (the paramedic split — design doc §8b). It names WHAT was repaired and WHO now
// holds it, so the object's other caretakers can independently verify and settle
// the bounty. It deliberately does NOT carry the survivor set or the code
// parameters: a verifying caretaker takes those from the manifest it already holds
// and picks its own k survivors to recompute — so a claimant cannot bias the
// correctness check by dictating which survivors to use.
//
// The claim is not trusted on its face: correctness is re-derived (VerifyByRecompute
// against the manifest-committed ShardID) and retrievability is re-challenged
// against Holder under an identity-bound seed. The claim only points the verifiers
// at the work; the proofs are recomputed.
//
// ADVERSARY-SHAPE: capability=ClaimantChosenSurvivorSet UNCOVERED: no fixture GRANTS AND CONTROLS FOR a claimant any influence over which survivors the judge fetches. TestSurvivorFetchIsUnboundedPerSender_PINNED_DEFECT measures what a claim costs the judge; the out-of-range influence it used to show is now screened before the fetch and the width is budgeted to k, but neither is a control that removes claimant influence, so this is still not declared as cover.

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/ports"
)

// RepairClaim identifies one repaired shard and its new holder.
type RepairClaim struct {
	// Root is the object whose durability escrow pays the bounty.
	Root ports.Hash `cbor:"1,keyasint"`
	// Stripe is the stripe index within the file, ShardPos the position within
	// the stripe (0.n-1: data 0.k-1, parity k.n-1). Together with the manifest
	// they pin which codeword coordinate was repaired.
	Stripe   int `cbor:"2,keyasint"`
	ShardPos int `cbor:"3,keyasint"`
	// ShardID is the manifest-committed content ID of the repaired shard — the
	// correctness target a verifier recomputes to.
	ShardID ports.ChunkID `cbor:"4,keyasint"`
	// Holder is the fresh storage node now holding the rebuilt shard — the bounty
	// payee and the target of the retrievability challenge (design §8b).
	Holder ports.NodeID `cbor:"5,keyasint"`
	// LeafIndex is the shard's leaf in the object's Merkle tree (manifest.Leaves): the
	// claim says "ShardID is leaf LeafIndex under Root". A judge checks it against its
	// own manifest; and because it names the leaf, one Merkle path from Root refutes a
	// false claim with no knowledge of the object's geometry, so any validator can.
	LeafIndex int `cbor:"6,keyasint"`
}

// Marshal serializes the claim to canonical CBOR for the wire.
func (c RepairClaim) Marshal() ([]byte, error) {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, err
	}
	return enc.Marshal(c)
}

// UnmarshalClaim parses a wire claim. A caller should treat every field as
// untrusted input to be re-verified, never as established fact.
func UnmarshalClaim(b []byte) (RepairClaim, error) {
	var c RepairClaim
	if err := cbor.Unmarshal(b, &c); err != nil {
		return RepairClaim{}, err
	}
	return c, nil
}

// SignedClaim is a claim its claimant signed. The signature is what makes a false claim
// attributable: a claim naming a shard the object does not commit at that leaf is a
// lie in the claimant's own hand, and the chain slashes the claimant's bond for it.
type SignedClaim struct {
	Claim RepairClaim `cbor:"1,keyasint"`
	Pub   []byte      `cbor:"2,keyasint"` // the claimant's ed25519 public key
	Sig   []byte      `cbor:"3,keyasint"`
}

// claimDomain separates a claim signature from every other signature an identity key
// makes, so no other signed message can be presented as a claim.
const claimDomain = "silt/repair-claim/v1"

// ErrBadClaimSignature is a claim whose signature does not verify.
var ErrBadClaimSignature = errors.New("repairproof: claim signature does not verify")

// ClaimSigBytes is the message a claimant signs: the domain, the network's chain id
// (a claim signed on one network is no claim on another), and the canonical claim.
func ClaimSigBytes(chainID ports.Hash, c RepairClaim) ([]byte, error) {
	raw, err := c.Marshal()
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(claimDomain)+len(chainID)+len(raw))
	out = append(out, claimDomain...)
	out = append(out, chainID[:]...)
	return append(out, raw...), nil
}

// SignClaim signs c with the claimant's identity key.
func SignClaim(priv ed25519.PrivateKey, chainID ports.Hash, c RepairClaim) (SignedClaim, error) {
	msg, err := ClaimSigBytes(chainID, c)
	if err != nil {
		return SignedClaim{}, err
	}
	pub := priv.Public().(ed25519.PublicKey)
	return SignedClaim{Claim: c, Pub: append([]byte(nil), pub...), Sig: ed25519.Sign(priv, msg)}, nil
}

// Claimant is the NodeID of the key that signed the claim.
func (s SignedClaim) Claimant() ports.NodeID { return sha256.Sum256(s.Pub) }

// Verify checks the signature under chainID.
func (s SignedClaim) Verify(chainID ports.Hash) error {
	if len(s.Pub) != ed25519.PublicKeySize || len(s.Sig) != ed25519.SignatureSize {
		return ErrBadClaimSignature
	}
	msg, err := ClaimSigBytes(chainID, s.Claim)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(s.Pub), msg, s.Sig) {
		return ErrBadClaimSignature
	}
	return nil
}

// Marshal serializes the signed claim to canonical CBOR for the wire.
func (s SignedClaim) Marshal() ([]byte, error) {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, err
	}
	return enc.Marshal(s)
}

// UnmarshalSignedClaim parses a wire claim. Every field is untrusted until Verify and
// the judge's own checks pass.
func UnmarshalSignedClaim(b []byte) (SignedClaim, error) {
	var s SignedClaim
	if err := cbor.Unmarshal(b, &s); err != nil {
		return SignedClaim{}, err
	}
	return s, nil
}
