package manifest

import (
	"crypto/sha256"
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// Tree is a precomputed RFC 6962 Merkle tree over a FIXED leaf set. Building it
// is O(n) once; each Prove is then O(log n), because it reads cached internal
// subtree hashes instead of recomputing merkleTreeHash over half the leaves on
// every call — which is what the standalone Prove(leaves, index) does, making
// that O(n) per proof. The recompute is invisible on a small file manifest, but
// on a large, frequently-challenged bond plot (up to ~16k leaves, O(k) proofs
// every audit epoch) it dominates the consensus loop (#340). The construction is
// bit-for-bit the standalone one, so Tree.Root() == MerkleRoot(leaves) and
// Tree.Prove(i) returns the SAME Proof as Prove(leaves, i): a proof from either
// verifies against the same root. Use it where many proofs are drawn from one
// stable leaf set; a one-off proof over a fresh slice should still use Prove.
type Tree struct {
	leaves []ports.Hash
	root   *treeNode
}

// treeNode caches the merkleTreeHash of one contiguous leaf range. split is the
// index at which the right subtree begins (== largestPowerOfTwoBelow offset); a
// leaf node has left == nil and split == 0.
type treeNode struct {
	split       int
	hash        ports.Hash
	left, right *treeNode
}

// BuildTree precomputes the Merkle tree over leaves. The slice is retained by
// reference (callers hold it stable for the tree's life), matching how the bond
// Commitment already keeps its leaves resident.
func BuildTree(leaves []ports.Hash) *Tree {
	t := &Tree{leaves: leaves}
	if len(leaves) > 0 {
		t.root = buildNode(leaves, 0, len(leaves))
	}
	return t
}

// buildNode mirrors merkleTreeHash's split so the cached hashes are identical.
func buildNode(leaves []ports.Hash, lo, hi int) *treeNode {
	if hi-lo == 1 {
		return &treeNode{hash: leafHash(leaves[lo])}
	}
	k := largestPowerOfTwoBelow(hi - lo)
	left := buildNode(leaves, lo, lo+k)
	right := buildNode(leaves, lo+k, hi)
	return &treeNode{split: lo + k, hash: nodeHash(left.hash, right.hash), left: left, right: right}
}

// Root returns the tree root; equals MerkleRoot(leaves) (empty tree = SHA-256 of
// nothing, exactly as merkleTreeHash defines it).
func (t *Tree) Root() ports.Hash {
	if t.root == nil {
		return sha256.Sum256(nil)
	}
	return t.root.hash
}

// Prove builds the inclusion proof for leaves[index] in O(log n), returning the
// SAME Proof as Prove(t.leaves, index).
func (t *Tree) Prove(index int) (Proof, error) {
	if index < 0 || index >= len(t.leaves) {
		return Proof{}, fmt.Errorf("merkle: index %d out of range for %d leaves", index, len(t.leaves))
	}
	return Proof{Index: index, Total: len(t.leaves), Path: t.auditPath(t.root, index)}, nil
}

// auditPath mirrors the standalone auditPath recursion — the deeper half first,
// then this split's sibling appended — but reads cached subtree hashes instead
// of rehashing, so the path is byte-identical and produced in O(log n).
func (t *Tree) auditPath(n *treeNode, index int) []ports.Hash {
	if n.left == nil { // leaf
		return nil
	}
	if index < n.split {
		return append(t.auditPath(n.left, index), n.right.hash)
	}
	return append(t.auditPath(n.right, index), n.left.hash)
}
