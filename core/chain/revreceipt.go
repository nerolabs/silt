package chain

import (
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/core/translog"
	"github.com/nerolabs/silt/ports"
)

// RevocationReceipt is what an honoured removal carries: the takedown's own leaf in
// the revocation transparency log, an inclusion proof of that leaf against the log
// root a committed block signs, and a consistency proof that the log the removal is
// read from extends an earlier checkpoint of the same log.
//
// Anyone holding the anchoring block's header can check it, with no other state:
// the leaf names exactly one root and the height that revoked it, the inclusion
// proof ties it to the header's LogRoot, and the header's hash ties that root to the
// chain. That is what makes a removal accountable rather than merely true: an
// operator who honours a takedown can show it was the network's committed record,
// and an auditor holding two receipts can show the log only grew between them, so
// no removal was quietly added to the past or dropped from it. The receipt proves
// what was removed; it says nothing about who else still serves the root, which is
// the other half of non-globality and is observed, not proven.
type RevocationReceipt struct {
	Root   ports.Hash `cbor:"1,keyasint"`
	Height uint64     `cbor:"2,keyasint"` // the block that committed the revocation
	Index  uint64     `cbor:"3,keyasint"` // its leaf's position in the log

	At        uint64       `cbor:"4,keyasint"` // the block whose LogRoot the proof is against
	BlockHash ports.Hash   `cbor:"5,keyasint"` // that block's hash
	LogRoot   ports.Hash   `cbor:"6,keyasint"` // that block's committed LogRoot
	Size      uint64       `cbor:"7,keyasint"` // the log size LogRoot commits
	Inclusion []ports.Hash `cbor:"8,keyasint"`

	// The earlier checkpoint, and the proof the log at Size extends it. A zero
	// PrevSize is the empty log, which every log extends.
	PrevSize    uint64       `cbor:"9,keyasint,omitempty"`
	PrevLogRoot ports.Hash   `cbor:"10,keyasint"`
	Consistency []ports.Hash `cbor:"11,keyasint,omitempty"`
	// Curator is the curator whose listing this is; zero for a takedown committed before
	// curators. The log leaf names it, so the receipt proves who asked for the removal.
	Curator ports.NodeID `cbor:"12,keyasint,omitempty"`
}

// Listing names one takedown listing: a curator's listing of a root, or, for a takedown committed
// before curators, the bare root (a zero Curator).
type Listing struct {
	Curator ports.NodeID
	Root    ports.Hash
}

// key is the listing's revoked-keyspace key.
func (l Listing) key() ports.Hash {
	if l.Curator == (ports.NodeID{}) {
		return l.Root
	}
	return ListingKey(l.Curator, l.Root)
}

func (t takedownOp) listing() Listing { return Listing{Curator: t.Curator, Root: t.Root} }

// Listed reports whether a listing is live: listed and not since lifted.
func (c *Chain) Listed(l Listing) bool { return c.revoked[l.key()] }

// ListedBy reports whether any of the given curators has a live listing of root. It is the
// question an operator asks of the curators it follows; a curator it does not follow lists
// nothing, as far as that operator is concerned.
func (c *Chain) ListedBy(root ports.Hash, curators []ports.NodeID) bool {
	for _, cur := range curators {
		if c.revoked[ListingKey(cur, root)] {
			return true
		}
	}
	return false
}

var (
	ErrNotRevoked = errors.New("chain: root is not currently revoked")
	ErrNoLogRoot  = errors.New("chain: no committed revocation log root at or above the revoking block")
)

// RevocationReceiptFor builds the receipt for a live listing, anchored at the revoking block's
// own LogRoot or, on a block from before the log root was committed, at the nearest later block
// that commits one. It walks back from the head, reading only the blocks above the revocation.
func (c *Chain) RevocationReceiptFor(l Listing) (RevocationReceipt, error) {
	if !c.revoked[l.key()] {
		return RevocationReceipt{}, ErrNotRevoked
	}
	return buildReceipt(c.revLog, c.blocks.Len(), c.blocks.At, l)
}

// ExtendFrom attaches the proof that the receipt's log extends the checkpoint at
// prevSize of this chain's own log. The checkpoint is typically the previous receipt
// the same operator issued, so its receipts form one chain of consistency proofs.
func (c *Chain) ExtendFrom(r *RevocationReceipt, prevSize uint64) error {
	return extendFrom(c.revLog, r, prevSize)
}

// ReceiptFromBlocks is RevocationReceiptFor for an auditor holding only a replica's
// blocks: it rebuilds the revocation log from the blocks' own takedown records, then
// builds the listing's receipt and, when prevSize > 0, its consistency from that checkpoint.
// The receipt is then checked against the block it anchors at, so a replica whose
// committed LogRoot disagrees with its own records yields an error, never a receipt.
func ReceiptFromBlocks(blocks []Block, l Listing, prevSize uint64) (RevocationReceipt, error) {
	log := translog.New()
	revoked := false
	for i := range blocks {
		for _, t := range takedownOps(&blocks[i]) {
			log.Append(t.leaf(blocks[i].Height))
			if t.listing() == l {
				revoked = t.Op == RevOp
			}
		}
	}
	if !revoked {
		return RevocationReceipt{}, ErrNotRevoked
	}
	r, err := buildReceipt(log, len(blocks), func(i int) *Block { return &blocks[i] }, l)
	if err != nil {
		return RevocationReceipt{}, err
	}
	if err := extendFrom(log, &r, prevSize); err != nil {
		return RevocationReceipt{}, err
	}
	if err := VerifyRevocationReceipt(r, blocks[r.At]); err != nil {
		return RevocationReceipt{}, fmt.Errorf("chain: this replica's blocks do not prove their own takedown record: %w", err)
	}
	return r, nil
}

func buildReceipt(log *translog.Log, n int, at func(int) *Block, l Listing) (RevocationReceipt, error) {
	size := uint64(log.Size()) // events through the block being visited
	var anchor *Block
	var anchorSize uint64
	for h := n - 1; h >= 0; h-- {
		b := at(h)
		ops := takedownOps(b)
		start := size - uint64(len(ops))
		if b.LogRoot != nil {
			anchor, anchorSize = b, size
		}
		for j := len(ops) - 1; j >= 0; j-- {
			if ops[j].Op != RevOp || ops[j].listing() != l {
				continue
			}
			if anchor == nil {
				return RevocationReceipt{}, fmt.Errorf("%w (revoked at height %d)", ErrNoLogRoot, b.Height)
			}
			idx := start + uint64(j)
			proof, err := log.InclusionProof(int(idx), int(anchorSize))
			if err != nil {
				return RevocationReceipt{}, err
			}
			return RevocationReceipt{
				Root: l.Root, Curator: l.Curator, Height: b.Height, Index: idx,
				At: anchor.Height, BlockHash: anchor.Hash(), LogRoot: *anchor.LogRoot, Size: anchorSize,
				Inclusion: proof,
			}, nil
		}
		size = start
	}
	return RevocationReceipt{}, fmt.Errorf("chain: %s is listed but no block lists it", l.Root)
}

func extendFrom(log *translog.Log, r *RevocationReceipt, prevSize uint64) error {
	if prevSize > r.Size {
		return fmt.Errorf("chain: checkpoint size %d is past the receipt's log size %d", prevSize, r.Size)
	}
	prevRoot, err := log.RootAt(int(prevSize))
	if err != nil {
		return err
	}
	proof, err := log.ConsistencyProof(int(prevSize), int(r.Size))
	if err != nil {
		return err
	}
	r.PrevSize, r.PrevLogRoot, r.Consistency = prevSize, prevRoot, proof
	return nil
}

// VerifyRevocationReceipt checks a receipt against the header of the block it is
// anchored at. The header's own validity (its quorum) is the caller's: a verifier
// reads it from its own replica, or from a checkpoint it trusts.
func VerifyRevocationReceipt(r RevocationReceipt, anchor Block) error {
	switch {
	case anchor.Height != r.At:
		return fmt.Errorf("receipt is anchored at height %d, header is height %d", r.At, anchor.Height)
	case anchor.Hash() != r.BlockHash:
		return fmt.Errorf("receipt names block %s at height %d, the header hashes to %s", r.BlockHash, r.At, anchor.Hash())
	case anchor.LogRoot == nil:
		return fmt.Errorf("block %d commits no revocation log root", r.At)
	case *anchor.LogRoot != r.LogRoot:
		return fmt.Errorf("receipt's log root %s is not block %d's committed %s", r.LogRoot, r.At, *anchor.LogRoot)
	case r.Height > r.At:
		return fmt.Errorf("receipt revokes at height %d, above its anchor %d", r.Height, r.At)
	case r.Index >= r.Size:
		return fmt.Errorf("leaf %d is outside a log of size %d", r.Index, r.Size)
	}
	leaf := RevocationLeaf(RevOp, r.Root, r.Height)
	if r.Curator != (ports.NodeID{}) {
		leaf = CuratedRevocationLeaf(RevOp, r.Curator, r.Root, r.Height)
	}
	if !translog.VerifyInclusion(leaf, int(r.Index), int(r.Size), r.LogRoot, r.Inclusion) {
		return fmt.Errorf("the revocation of %s at height %d is not leaf %d of the log block %d commits", r.Root, r.Height, r.Index, r.At)
	}
	if !translog.VerifyConsistency(r.PrevLogRoot, int(r.PrevSize), r.LogRoot, int(r.Size), r.Consistency) {
		return fmt.Errorf("the log at size %d does not extend the checkpoint at size %d", r.Size, r.PrevSize)
	}
	return nil
}

// EncodeReceipt and DecodeReceipt are the receipt's wire form, the one an operator
// publishes and an auditor checks.
func EncodeReceipt(r RevocationReceipt) []byte {
	raw, err := encMode.Marshal(r)
	if err != nil {
		panic(err)
	}
	return raw
}

func DecodeReceipt(raw []byte) (RevocationReceipt, error) {
	var r RevocationReceipt
	if err := cbor.Unmarshal(raw, &r); err != nil {
		return RevocationReceipt{}, fmt.Errorf("chain: decode revocation receipt: %w", err)
	}
	return r, nil
}

// RevocationsSince lists, in log order, the listings made by log events at index seen and above,
// walking back from the head only as far as those events reach. Lifted listings are not listed;
// whether one is still live is Listed's to say.
func (c *Chain) RevocationsSince(seen uint64) []Listing {
	size := uint64(c.revLog.Size())
	var out []Listing
	for h := c.blocks.Len() - 1; h >= 0 && size > seen; h-- {
		b := c.blocks.At(h)
		ops := takedownOps(b)
		start := size - uint64(len(ops))
		for j := len(ops) - 1; j >= 0; j-- {
			if ops[j].Op == RevOp && start+uint64(j) >= seen {
				out = append(out, ops[j].listing())
			}
		}
		size = start
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
