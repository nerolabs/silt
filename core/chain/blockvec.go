package chain

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
type blockVec struct {
	root  *bvNode
	tail  []Block // the last 1..32 blocks; shared, never written in place
	n     int
	shift uint // bit offset of the root's index digit
}

// bvNode is a leaf (leaf holds exactly 32 blocks) or an internal node.
type bvNode struct {
	kids *[32]*bvNode
	leaf []Block
}

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
// history share: callers read it and never write through it.
func (v *blockVec) At(i int) *Block {
	if i < 0 || i >= v.n {
		panic("chain: block index out of range")
	}
	if i >= v.tailOffset() {
		return &v.tail[i-v.tailOffset()]
	}
	node := v.root
	for level := v.shift; level > 0; level -= bvBits {
		node = node.kids[(i>>level)&bvMask]
	}
	return &node.leaf[i&bvMask]
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

// Set replaces the block at index i.
func (v *blockVec) Set(i int, b Block) {
	if i < 0 || i >= v.n {
		panic("chain: block index out of range")
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
