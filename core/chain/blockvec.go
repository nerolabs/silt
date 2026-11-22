package chain

import (
	"crypto/sha256"
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// blockVec is the chain's block history as a persistent vector: a 32-way trie of
// full 32-block leaves plus a tail of up to 32 blocks, the structure of Clojure's
// PersistentVector (Bagwell, "Ideal Hash Trees"; Hickey). Nodes and leaves are never
// modified once built: an append copies the tail (and, when the tail fills, the path
// it joins), and a Set copies the path to one leaf. Copying a blockVec value is
// therefore O(1), and the copy and the original change independently.
//
// Validating a block applies it to a dry-run copy of the chain, and that copy used
// to duplicate the whole history: a cost proportional to the chain's height on every
// block, and a quadratic one when a restarting validator re-validates its history
// block by block. Shared, the copy costs nothing.
//
// The trie's shape depends only on the length, so two blockVecs holding the same
// blocks are built from equal nodes whatever order of appends and Sets made them.
//
// Old leaves can be released from memory once a durable archive holds their blocks
// (release). A released leaf keeps a digest of each block's encoding and whether
// its heavy proofs were shed, and a read of one of its blocks fetches it from the
// archive, sheds the same proofs, and checks the digest: what comes back is exactly
// the block that was released, signatures included. A block's hash would not do:
// it covers the body, which the signatures sign, and not the signatures. Alongside the blocks the vector keeps what would otherwise need a walk
// of the whole history: an index from block hash to height, and the era census.
type blockVec struct {
	root  *bvNode
	tail  []Block // the last 1..32 blocks; shared, never written in place
	n     int
	shift uint // bit offset of the root's index digit

	byHash pmap[ports.Hash, int]
	census vecCensus
	// archive returns the blocks of released leaves; hotFrom is the index of the
	// first block after them, or 0 when none is released. The first leaf, which
	// holds genesis, is never released.
	archive BlockArchive
	hotFrom int
}

// bvNode is a leaf (leaf holds exactly 32 blocks), a released leaf (cold), or an
// internal node.
type bvNode struct {
	kids *[32]*bvNode
	leaf []Block
	cold *coldLeaf
}

// coldLeaf is what a released leaf keeps of its 32 blocks.
type coldLeaf struct {
	digests [bvWidth]ports.Hash // sha256 of each block's encoding
	shed    uint32              // bit j: block j's heavy proofs were shed in memory
}

func encodingDigest(b *Block) ports.Hash { return sha256.Sum256(Encode(b)) }

// BlockArchive returns committed blocks from durable storage: the chain's own log,
// which holds every block the chain has committed, from genesis.
type BlockArchive interface {
	// Durable is how many blocks, from genesis, the archive holds.
	Durable() uint64
	// Block returns the block at height, below Durable, as it was written.
	Block(height uint64) (Block, error)
}

// vecCensus is the era census folded block by block, so reporting it costs nothing
// however long the chain. The maps are copied on write: blockVec values share them.
type vecCensus struct {
	counts        map[uint64]int
	first         map[uint64]uint64
	maxVersion    uint64
	maxAtts       int
	maxAttsHeight uint64
}

func (s vecCensus) with(b *Block) vecCensus {
	counts := make(map[uint64]int, len(s.counts)+1)
	for k, v := range s.counts {
		counts[k] = v
	}
	counts[b.Version]++
	s.counts = counts
	if _, seen := s.first[b.Version]; !seen {
		first := make(map[uint64]uint64, len(s.first)+1)
		for k, v := range s.first {
			first[k] = v
		}
		first[b.Version] = b.Height
		s.first = first
	}
	if b.Version > s.maxVersion {
		s.maxVersion = b.Version
	}
	if n := len(b.Atts); n > s.maxAtts {
		s.maxAtts, s.maxAttsHeight = n, b.Height
	}
	return s
}

// Census is CensusOf over every block, without reading any.
func (v *blockVec) Census() VersionCensus {
	out := VersionCensus{Blocks: v.n, Counts: map[uint64]int{}, FirstHeights: map[uint64]uint64{}}
	if v.n == 0 {
		return out
	}
	head := v.Last()
	out.HeadHeight, out.HeadVersion = head.Height, head.Version
	out.AttsMeasured = true
	for k, c := range v.census.counts {
		out.Counts[k] = c
	}
	for k, h := range v.census.first {
		out.FirstHeights[k] = h
	}
	out.MaxVersion = v.census.maxVersion
	out.MaxAtts, out.MaxAttsHeight = v.census.maxAtts, v.census.maxAttsHeight
	out.MaxVersionFirstHeight = out.FirstHeights[out.MaxVersion]
	return out
}

// IndexOf returns the index of the block whose hash is h.
func (v *blockVec) IndexOf(h ports.Hash) (int, bool) { return v.byHash.Get(h) }

const (
	bvBits  = 5
	bvWidth = 1 << bvBits
	bvMask  = bvWidth - 1
)

// newBlockVec builds a blockVec holding bs, in order.
func newBlockVec(bs []Block) blockVec {
	var v blockVec
	for i := range bs {
		v.Append(bs[i])
	}
	return v
}

// Len is the number of blocks.
func (v *blockVec) Len() int { return v.n }

func (v *blockVec) tailOffset() int { return v.n - len(v.tail) }

// At returns the block at index i. It points into storage that other copies of the
// history share: callers read it and never write through it. A released block is
// read back from the archive; a block the archive cannot return is a node whose own
// log has failed, and At panics rather than answer with anything else.
func (v *blockVec) At(i int) *Block {
	if i < 0 || i >= v.n {
		panic("chain: block index out of range")
	}
	if i >= v.tailOffset() {
		return &v.tail[i-v.tailOffset()]
	}
	node := v.leafNode(i)
	if node.cold != nil {
		return v.readCold(i, node.cold)
	}
	return &node.leaf[i&bvMask]
}

func (v *blockVec) leafNode(i int) *bvNode {
	node := v.root
	for level := v.shift; level > 0; level -= bvBits {
		node = node.kids[(i>>level)&bvMask]
	}
	return node
}

func (v *blockVec) readCold(i int, cold *coldLeaf) *Block {
	b, err := v.archive.Block(uint64(i))
	if err != nil {
		panic(fmt.Sprintf("chain: block %d was released from memory and the chain log cannot return it: %v", i, err))
	}
	j := i & bvMask
	if cold.shed&(1<<j) != 0 {
		b = b.Prune()
	}
	if d := encodingDigest(&b); d != cold.digests[j] || b.Height != uint64(i) {
		panic(fmt.Sprintf("chain: the chain log returned block %d as height %d digest %s, but the block released was %s", i, b.Height, d, cold.digests[j]))
	}
	return &b
}

// Released reports whether block i is held only in the archive.
func (v *blockVec) Released(i int) bool { return i >= bvWidth && i < v.hotFrom }

// HotFrom is the index of the first block held in memory after the released
// ones: 0 when none is released.
func (v *blockVec) HotFrom() int { return v.hotFrom }

// release drops from memory every whole leaf below index below, except the first,
// once archive holds it. keep reports whether a block may be released; a leaf with
// any block it refuses stays, and so does every leaf after it. Each block is read
// back from archive and checked against the one in memory before its leaf goes, so
// an archive that disagrees with the chain is found here, while the chain still
// holds the truth. It returns how many blocks it released.
func (v *blockVec) release(below int, archive BlockArchive, keep func(*Block) bool) (int, error) {
	if d := int(archive.Durable()); below > d {
		below = d
	}
	below = min(below, v.tailOffset()) &^ bvMask
	from := max(v.hotFrom, bvWidth)
	if below <= from {
		return 0, nil
	}
	if v.archive != nil && v.archive != archive {
		return 0, fmt.Errorf("chain: blocks are already released to another archive")
	}
	released := 0
	for start := from; start < below; start += bvWidth {
		node := v.leafNode(start)
		cold := &coldLeaf{}
		for j := range node.leaf {
			b := &node.leaf[j]
			if !keep(b) {
				return released, nil
			}
			cold.digests[j] = encodingDigest(b)
			if b.HeavyProofsShed() {
				cold.shed |= 1 << j
			}
		}
		for j := range node.leaf {
			got, err := archive.Block(uint64(start + j))
			if err != nil {
				return released, fmt.Errorf("chain: archive block %d: %w", start+j, err)
			}
			if cold.shed&(1<<j) != 0 {
				got = got.Prune()
			}
			if encodingDigest(&got) != cold.digests[j] {
				return released, fmt.Errorf("chain: archive block %d is not the committed block", start+j)
			}
		}
		v.root = bvSetNode(v.shift, v.root, start, &bvNode{cold: cold})
		v.archive, v.hotFrom = archive, start+bvWidth
		released += bvWidth
	}
	return released, nil
}

// bvSetNode returns a copy of node with the leaf holding index i replaced by leaf.
func bvSetNode(level uint, node *bvNode, i int, leaf *bvNode) *bvNode {
	if level == 0 {
		return leaf
	}
	kids := *node.kids
	sub := (i >> level) & bvMask
	kids[sub] = bvSetNode(level-bvBits, node.kids[sub], i, leaf)
	return &bvNode{kids: &kids}
}

// Last returns the final block, or nil when there is none.
func (v *blockVec) Last() *Block {
	if v.n == 0 {
		return nil
	}
	return &v.tail[len(v.tail)-1]
}

// Append adds b at the end.
func (v *blockVec) Append(b Block) {
	v.byHash.Set(b.Hash(), v.n)
	v.census = v.census.with(&b)
	if len(v.tail) < bvWidth {
		tail := make([]Block, len(v.tail)+1, bvWidth)
		copy(tail, v.tail)
		tail[len(v.tail)] = b
		v.tail = tail
		v.n++
		return
	}
	full := &bvNode{leaf: v.tail}
	switch {
	case v.root == nil:
		v.root, v.shift = &bvNode{kids: new([32]*bvNode)}, bvBits
		v.root.kids[0] = full
	case (v.n >> bvBits) > (1 << v.shift):
		top := &bvNode{kids: new([32]*bvNode)}
		top.kids[0] = v.root
		top.kids[1] = bvPath(v.shift, full)
		v.root, v.shift = top, v.shift+bvBits
	default:
		v.root = v.pushTail(v.shift, v.root, full)
	}
	tail := make([]Block, 1, bvWidth)
	tail[0] = b
	v.tail = tail
	v.n++
}

// pushTail returns a copy of parent with the full leaf joined at the position that
// follows the last leaf already in the trie.
func (v *blockVec) pushTail(level uint, parent, leaf *bvNode) *bvNode {
	kids := *parent.kids
	sub := ((v.n - 1) >> level) & bvMask
	if level == bvBits {
		kids[sub] = leaf
	} else if child := parent.kids[sub]; child != nil {
		kids[sub] = v.pushTail(level-bvBits, child, leaf)
	} else {
		kids[sub] = bvPath(level-bvBits, leaf)
	}
	return &bvNode{kids: &kids}
}

// bvPath wraps leaf in internal nodes down from level.
func bvPath(level uint, leaf *bvNode) *bvNode {
	if level == 0 {
		return leaf
	}
	n := &bvNode{kids: new([32]*bvNode)}
	n.kids[0] = bvPath(level-bvBits, leaf)
	return n
}

// Set replaces the block at index i. A released block is final and cannot be
// replaced. The census assumes a replacement keeps the block's version and
// attestations, which pruning, its one caller, does.
func (v *blockVec) Set(i int, b Block) {
	if i < 0 || i >= v.n {
		panic("chain: block index out of range")
	}
	if v.Released(i) {
		panic(fmt.Sprintf("chain: block %d was released from memory and cannot be replaced", i))
	}
	old := v.At(i)
	if old.Version != b.Version || len(old.Atts) != len(b.Atts) {
		panic(fmt.Sprintf("chain: replacing block %d would change the era census", i))
	}
	if oh, nh := old.Hash(), b.Hash(); oh != nh {
		v.byHash.Delete(oh)
		v.byHash.Set(nh, i)
	}
	if i >= v.tailOffset() {
		tail := make([]Block, len(v.tail), bvWidth)
		copy(tail, v.tail)
		tail[i-v.tailOffset()] = b
		v.tail = tail
		return
	}
	v.root = bvSet(v.shift, v.root, i, b)
}

func bvSet(level uint, node *bvNode, i int, b Block) *bvNode {
	if level == 0 {
		leaf := make([]Block, bvWidth)
		copy(leaf, node.leaf)
		leaf[i&bvMask] = b
		return &bvNode{leaf: leaf}
	}
	kids := *node.kids
	sub := (i >> level) & bvMask
	kids[sub] = bvSet(level-bvBits, node.kids[sub], i, b)
	return &bvNode{kids: &kids}
}

// Slice returns a fresh copy of the blocks from index from to the end.
func (v *blockVec) Slice(from int) []Block {
	if from < 0 {
		from = 0
	}
	if from >= v.n {
		return nil
	}
	out := make([]Block, 0, v.n-from)
	for i := from; i < v.n; i++ {
		out = append(out, *v.At(i))
	}
	return out
}
