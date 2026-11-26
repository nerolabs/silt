package chain

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// Curator-signed takedowns (ADVERSARY.md claim 2).
//
// A takedown names a root and a CURATOR: an ed25519 key that signs the takedown and is the party
// accountable for it. Anyone may be a curator; operators choose which curators they follow, so the
// effect of a list is proportional to who trusts it. On a witnessable block every revocation and
// un-revocation carries its curator's signature and, on a network that requires publish tokens, a
// paid token — the fee that prices a takedown the way a publish is priced. A validator adds no
// takedown of its own: a takedown with no curator is refused.
//
// The chain keeps one listing per (curator, root): two curators list the same root independently,
// and only the curator that listed a root can lift its own listing. The transparency log records
// the curator in every leaf, so every honoured removal names who asked for it.

// TakedownAuth is one takedown's authority: the curator's key and signature, and the fee.
//
// Nonce makes each authority single-use. The chain spends an authority's digest when it commits
// (AuthDigest, in the same spent set as token serials), so nobody can replay a curator's signature:
// not to re-list a root the curator lifted, nor to lift one it re-listed. A curator that lists the
// same root again signs with a fresh nonce.
type TakedownAuth struct {
	Curator []byte              `cbor:"1,keyasint"` // ed25519 public key
	Sig     []byte              `cbor:"2,keyasint"`
	Token   *ports.PublishToken `cbor:"3,keyasint,omitempty"`
	Nonce   uint64              `cbor:"4,keyasint"`
}

// CuratorID is the curator's identity: the hash of its public key, as a node id is of its key.
func (a TakedownAuth) CuratorID() ports.NodeID { return ports.HashBytes(a.Curator) }

var (
	// ErrTakedownUnauthorized is a takedown on a witnessable block with no curator authority.
	ErrTakedownUnauthorized = errors.New("chain: takedown carries no curator authority")
	// ErrTakedownSig is a takedown whose curator signature does not verify.
	ErrTakedownSig = errors.New("chain: takedown curator signature does not verify")
	// ErrAlreadyListed is a curator listing a root it already lists.
	ErrAlreadyListed = errors.New("chain: the curator already lists this root")
	// ErrTakedownReplayed is a curator authority the chain has already spent.
	ErrTakedownReplayed = errors.New("chain: takedown authority already used")
	// ErrTakedownAuthEra is takedown authority on a block below the witnessable version.
	ErrTakedownAuthEra = errors.New("chain: takedown authority rides only a witnessable block")
)

const takedownDomain = "silt/takedown/v1"

// TakedownSigBytes is what a curator signs: the domain, the network (a takedown for one network
// is none on another), the operation, the root and the nonce.
func TakedownSigBytes(chainID ports.Hash, op byte, root ports.Hash, nonce uint64) []byte {
	out := make([]byte, 0, len(takedownDomain)+len(chainID)+1+len(root)+8)
	out = append(out, takedownDomain...)
	out = append(out, chainID[:]...)
	out = append(out, op)
	out = append(out, root[:]...)
	for i := 0; i < 8; i++ {
		out = append(out, byte(nonce>>(8*(7-i))))
	}
	return out
}

// SignTakedown makes a curator's authority for one operation on root under a fresh nonce (the
// token, the fee, is attached by the caller).
func SignTakedown(priv ed25519.PrivateKey, chainID ports.Hash, op byte, root ports.Hash, nonce uint64) TakedownAuth {
	return TakedownAuth{
		Curator: append([]byte(nil), priv.Public().(ed25519.PublicKey)...),
		Sig:     ed25519.Sign(priv, TakedownSigBytes(chainID, op, root, nonce)),
		Nonce:   nonce,
	}
}

func (a TakedownAuth) verify(chainID ports.Hash, op byte, root ports.Hash) bool {
	return len(a.Curator) == ed25519.PublicKeySize && len(a.Sig) == ed25519.SignatureSize &&
		ed25519.Verify(ed25519.PublicKey(a.Curator), TakedownSigBytes(chainID, op, root, a.Nonce), a.Sig)
}

// AuthDigest is the key under which a committed authority is spent: a digest of the curator and the
// message it signed. It is derived from the signed message, never from the signature bytes, so no
// re-encoding of a signature makes a second spendable authority. Its domain keeps it apart from
// every token serial in the same spent set.
func AuthDigest(op byte, root ports.Hash, a TakedownAuth) []byte {
	b := make([]byte, 0, 32+len(a.Curator)+1+len(root)+8)
	b = append(b, "silt/takedown-auth-spent/v1"...)
	b = append(b, a.Curator...)
	b = append(b, op)
	b = append(b, root[:]...)
	for i := 0; i < 8; i++ {
		b = append(b, byte(a.Nonce>>(8*(7-i))))
	}
	h := ports.HashBytes(b)
	return h[:]
}

// ListingKey is the revoked-keyspace key of a curator's listing of root. It is domain-separated
// from a bare root, which is the key of a takedown committed before curators (older eras).
func ListingKey(curator ports.NodeID, root ports.Hash) ports.Hash {
	b := make([]byte, 0, 16+len(curator)+len(root))
	b = append(b, "silt/listing/v1"...)
	b = append(b, curator[:]...)
	b = append(b, root[:]...)
	return ports.HashBytes(b)
}

// CuratedRevocationLeaf is the transparency-log leaf of a curator's takedown: the operation, the
// curator, the root and the height that committed it.
func CuratedRevocationLeaf(op byte, curator ports.NodeID, root ports.Hash, height uint64) ports.Hash {
	b := make([]byte, 0, 24+1+len(curator)+len(root)+8)
	b = append(b, []byte("silt/revlog/curated/v1")...)
	b = append(b, op)
	b = append(b, curator[:]...)
	b = append(b, root[:]...)
	for i := 0; i < 8; i++ {
		b = append(b, byte(height>>(8*(7-i))))
	}
	return ports.HashBytes(b)
}

// takedownOp is one takedown in a block, in log order (every revocation, then every un-revocation).
type takedownOp struct {
	Op      byte
	Root    ports.Hash
	Key     ports.Hash    // its revoked-keyspace key
	Curator ports.NodeID  // zero for a takedown committed before curators
	Auth    *TakedownAuth // nil before curators
}

// spentKey is the op's authority digest in the spent set (nil before curators).
func (t takedownOp) spentKey() []byte {
	if t.Auth == nil {
		return nil
	}
	return AuthDigest(t.Op, t.Root, *t.Auth)
}

// curated reports whether a block's takedowns carry curator authority: every witnessable block's
// do, and no older block's.
func curated(b *Block) bool { return b.Version >= BlockVersionWitnessable }

// CarriesTakedownAuthority reports whether b's version is one whose takedowns carry curator
// authority (a proposer drops the authority from an older era's block, whose rules refuse it).
func CarriesTakedownAuthority(b *Block) bool { return curated(b) }

// takedownOps lists a block's takedowns in log order with their keys. On a witnessable block the
// i-th op takes the i-th TakedownAuth; a missing one leaves Auth nil, which validity refuses.
func takedownOps(b *Block) []takedownOp {
	out := make([]takedownOp, 0, len(b.Revocations)+len(b.Unrevocations))
	add := func(op byte, r ports.Hash) {
		t := takedownOp{Op: op, Root: r, Key: r}
		if curated(b) {
			if i := len(out); i < len(b.TakedownAuths) {
				a := b.TakedownAuths[i]
				t.Auth, t.Curator = &a, a.CuratorID()
				t.Key = ListingKey(t.Curator, r)
			}
		}
		out = append(out, t)
	}
	for _, r := range b.Revocations {
		add(RevOp, r)
	}
	for _, r := range b.Unrevocations {
		add(UnrevOp, r)
	}
	return out
}

// leaf is the op's transparency-log leaf at height h.
func (t takedownOp) leaf(h uint64) ports.Hash {
	if t.Auth != nil {
		return CuratedRevocationLeaf(t.Op, t.Curator, t.Root, h)
	}
	return RevocationLeaf(t.Op, t.Root, h)
}

// checkTakedownAuthority is the structural half of the takedown rule shared by the full node and
// the floor box: a witnessable block's takedowns each carry a verifying curator signature for
// this network, and an older block's carry none. The state half (existence, listing, the fee)
// is checked by each path against its own view of the state.
func checkTakedownAuthority(b *Block, chainID ports.Hash) error {
	if !curated(b) {
		if len(b.TakedownAuths) > 0 {
			return fmt.Errorf("%w: block version %d", ErrTakedownAuthEra, b.Version)
		}
		return nil
	}
	if n := len(b.Revocations) + len(b.Unrevocations); len(b.TakedownAuths) != n {
		return fmt.Errorf("%w: %d takedown(s), %d authorities", ErrTakedownUnauthorized, n, len(b.TakedownAuths))
	}
	for i, t := range takedownOps(b) {
		if !t.Auth.verify(chainID, t.Op, t.Root) {
			return fmt.Errorf("%w (takedown %d, root %s)", ErrTakedownSig, i, t.Root)
		}
	}
	return nil
}
