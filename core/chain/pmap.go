package chain

import "hash/maphash"

// pmap is a persistent hash map: a 16-way trie over each key's 64-bit hash whose
// nodes are never modified once built. A write copies the nodes on the path to the
// changed key and shares every other node, so copying a pmap value is O(1) and the
// copy and the original then change independently.
//
// The chain keeps one entry per published file and one per spent token serial, and
// validating a block first copies the committed state so the block can be applied
// to the copy. With Go maps that copy cost time in proportion to every file ever
// published, on the validator's event loop, for every block. With pmap it costs
// nothing, and applying the block costs its own writes.
//
// The trie's shape depends only on the set of keys: a key's leaf sits at the
// shallowest depth where no other key shares its hash prefix, and a delete collapses
// what that rule no longer needs. Two pmaps holding the same entries are therefore
// built from equal nodes, whatever order the entries arrived in.
//
// Hashes are seeded per process, like Go's own maps, so a key chosen to collide
// with another (a token serial is chosen by its publisher) cannot be aimed at a
// trie path in advance.
type pmap[K comparable, V any] struct {
	root *pnode[K, V]
	n    int
}

// pnode is a leaf when kvs is non-empty, and an internal node otherwise. An
// internal node's children sit behind a pointer so that leaves, which outnumber
// internal nodes many times over, do not each carry an unused child array.
type pnode[K comparable, V any] struct {
	kids *[16]*pnode[K, V]
	kvs  []pkv[K, V] // a leaf's entries, which share one full hash
	hash uint64      // a leaf's full hash
}

type pkv[K comparable, V any] struct {
	k K
	v V
}

const pmapDepth = 16 // nibbles in a 64-bit hash

var pmapSeed = maphash.MakeSeed()

func pmapHash[K comparable](k K) uint64 { return maphash.Comparable(pmapSeed, k) }

func nibble(h uint64, d int) int { return int(h>>(4*d)) & 15 }

func (n *pnode[K, V]) isLeaf() bool { return len(n.kvs) > 0 }

// Len is the number of entries.
func (m *pmap[K, V]) Len() int { return m.n }

// Get returns the value stored for k.
func (m *pmap[K, V]) Get(k K) (V, bool) {
	h := pmapHash(k)
	n := m.root
	for d := 0; n != nil; d++ {
		if n.isLeaf() {
			if n.hash == h {
				for _, kv := range n.kvs {
					if kv.k == k {
						return kv.v, true
					}
				}
			}
			break
		}
		n = n.kids[nibble(h, d)]
	}
	var zero V
	return zero, false
}

// Has reports whether k is present.
func (m *pmap[K, V]) Has(k K) bool {
	_, ok := m.Get(k)
	return ok
}

// Set stores v for k.
func (m *pmap[K, V]) Set(k K, v V) {
	root, added := pset(m.root, 0, pmapHash(k), k, v)
	m.root = root
	if added {
		m.n++
	}
}

func pset[K comparable, V any](n *pnode[K, V], d int, h uint64, k K, v V) (*pnode[K, V], bool) {
	if n == nil {
		return &pnode[K, V]{kvs: []pkv[K, V]{{k, v}}, hash: h}, true
	}
	if n.isLeaf() {
		if n.hash == h {
			kvs := make([]pkv[K, V], len(n.kvs), len(n.kvs)+1)
			copy(kvs, n.kvs)
			for i := range kvs {
				if kvs[i].k == k {
					kvs[i].v = v
					return &pnode[K, V]{kvs: kvs, hash: h}, false
				}
			}
			return &pnode[K, V]{kvs: append(kvs, pkv[K, V]{k, v}), hash: h}, true
		}
		// Two hashes now share this prefix: push the existing leaf one level
		// down and insert beside it. Distinct 64-bit hashes differ in some
		// nibble, so this ends before d reaches pmapDepth.
		split := &pnode[K, V]{kids: new([16]*pnode[K, V])}
		split.kids[nibble(n.hash, d)] = n
		return pset(split, d, h, k, v)
	}
	i := nibble(h, d)
	kid, added := pset(n.kids[i], d+1, h, k, v)
	kids := *n.kids
	kids[i] = kid
	return &pnode[K, V]{kids: &kids}, added
}

// Delete removes k.
func (m *pmap[K, V]) Delete(k K) {
	root, removed := pdel(m.root, 0, pmapHash(k), k)
	m.root = root
	if removed {
		m.n--
	}
}

func pdel[K comparable, V any](n *pnode[K, V], d int, h uint64, k K) (*pnode[K, V], bool) {
	if n == nil {
		return nil, false
	}
	if n.isLeaf() {
		if n.hash != h {
			return n, false
		}
		for i := range n.kvs {
			if n.kvs[i].k == k {
				if len(n.kvs) == 1 {
					return nil, true
				}
				kvs := make([]pkv[K, V], 0, len(n.kvs)-1)
				kvs = append(kvs, n.kvs[:i]...)
				kvs = append(kvs, n.kvs[i+1:]...)
				return &pnode[K, V]{kvs: kvs, hash: h}, true
			}
		}
		return n, false
	}
	i := nibble(h, d)
	kid, removed := pdel(n.kids[i], d+1, h, k)
	if !removed {
		return n, false
	}
	kids := *n.kids
	kids[i] = kid
	// An internal node left holding a single leaf and nothing else gives way to
	// that leaf, which keeps the shape a function of the key set alone.
	var only *pnode[K, V]
	count := 0
	for _, c := range kids {
		if c != nil {
			count++
			only = c
		}
	}
	switch {
	case count == 0:
		return nil, true
	case count == 1 && only.isLeaf():
		return only, true
	}
	return &pnode[K, V]{kids: &kids}, true
}

// Range calls f for every entry until f returns false. The order is the trie's,
// which depends on the per-process hash seed: callers must not depend on it.
func (m *pmap[K, V]) Range(f func(K, V) bool) {
	prange(m.root, f)
}

func prange[K comparable, V any](n *pnode[K, V], f func(K, V) bool) bool {
	if n == nil {
		return true
	}
	if n.isLeaf() {
		for _, kv := range n.kvs {
			if !f(kv.k, kv.v) {
				return false
			}
		}
		return true
	}
	for _, c := range n.kids {
		if !prange(c, f) {
			return false
		}
	}
	return true
}
