package statehash

import (
	"crypto/sha256"
	"errors"

	"github.com/nerolabs/silt/ports"
	"github.com/pokt-network/smt"
	"github.com/pokt-network/smt/kvstore"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// Change sets one leaf of a Tree: Key to Value, or removes Key when Value is nil.
type Change struct {
	Key   []byte
	Value []byte
}

// Tree is a committed state tree kept between blocks, so a block's root costs
// one path per changed leaf instead of a rebuild from every leaf. Its root is
// the root Root computes over the same leaf set: the SMT is history
// independent, so the order changes arrive in does not matter.
//
// The nodes live in an in-memory store addressed by digest; Apply persists a
// set of changes and RootWith computes the root a set of changes would give
// without writing anything. A Tree is not safe for concurrent use.
type Tree struct {
	nodes kvstore.MapStore
	trie  *smt.SMT
}

// NewTree builds a Tree over leaves, refusing a duplicate key or an empty value
// exactly as Root does.
func NewTree(leaves []Leaf) (*Tree, error) {
	t := &Tree{nodes: simplemap.NewSimpleMap()}
	t.trie = smt.NewSparseMerkleTrie(t.nodes, sha256.New())
	seen := make(map[string]struct{}, len(leaves))
	for _, lf := range leaves {
		if _, dup := seen[string(lf.Key)]; dup {
			return nil, &DuplicateKeyError{Key: append([]byte(nil), lf.Key...)}
		}
		seen[string(lf.Key)] = struct{}{}
		if len(lf.Value) == 0 {
			return nil, &EmptyValueError{Key: append([]byte(nil), lf.Key...)}
		}
		if err := t.trie.Update(lf.Key, lf.Value); err != nil {
			return nil, err
		}
	}
	if err := t.commit(); err != nil {
		return nil, err
	}
	return t, nil
}

// Root is the committed root.
func (t *Tree) Root() ports.Hash {
	var h ports.Hash
	copy(h[:], t.trie.Root())
	return h
}

// RootWith is the root the tree would have after changes, computed on a view
// of the committed nodes. The tree is left as it was.
func (t *Tree) RootWith(changes []Change) (ports.Hash, error) {
	view := smt.ImportSparseMerkleTrie(t.nodes, sha256.New(), t.trie.Root())
	if err := applyChanges(view, changes); err != nil {
		return ports.Hash{}, err
	}
	var h ports.Hash
	copy(h[:], view.Root())
	return h, nil
}

// Apply commits changes to the tree.
func (t *Tree) Apply(changes []Change) error {
	if err := applyChanges(t.trie, changes); err != nil {
		return err
	}
	return t.commit()
}

// commit persists the dirty nodes, then reopens the trie lazily over the store
// so the committed nodes are held once, encoded, rather than also as a decoded
// tree in memory.
func (t *Tree) commit() error {
	if err := t.trie.Commit(); err != nil {
		return err
	}
	t.trie = smt.ImportSparseMerkleTrie(t.nodes, sha256.New(), t.trie.Root())
	return nil
}

func applyChanges(trie *smt.SMT, changes []Change) error {
	for _, ch := range changes {
		if ch.Value == nil {
			// Removing an absent key leaves the set, and so the root, unchanged.
			if err := trie.Delete(ch.Key); err != nil && !errors.Is(err, smt.ErrKeyNotFound) {
				return err
			}
			continue
		}
		if len(ch.Value) == 0 {
			return &EmptyValueError{Key: append([]byte(nil), ch.Key...)}
		}
		if err := trie.Update(ch.Key, ch.Value); err != nil {
			return err
		}
	}
	return nil
}
