package chain

import (
	"bytes"
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// The v5 state root is a sparse Merkle tree over every committed leaf, and the
// leaves grow by one per published file and one per spent token. Rebuilding it
// for each check made a block's validation, and each submitted publish, cost
// time and memory in proportion to the chain's whole history. The chain instead
// keeps the committed tree between blocks and computes a block's post-apply
// root by applying only the leaves that changed. The tree is a cache: dropping
// it changes cost, never a root, because the SMT is history independent and the
// root over the same leaf set is the same however it was reached.
//
// Changed leaves come from two places. The growing keyspaces (byRoot, spent,
// revoked) are journaled where apply writes them, so finding their changes never
// scans them. Everything else is bounded by the validator set, so its leaves are
// recomputed and compared with the tree's copy.

// Growing keyspaces, in the order their sizes are recorded.
const (
	spaceByRoot = iota
	spaceSpent
	spaceRevoked
	growingSpaces
)

var growingTags = [growingSpaces]string{spaceByRoot: tagByRoot, spaceSpent: tagSpent, spaceRevoked: tagRevoked}

// stateTreeCache is a chain's kept v5 state tree and what has changed since.
type stateTreeCache struct {
	tree *statehash.Tree
	// bounded holds the tree's leaves outside the growing keyspaces, by key.
	bounded map[string][]byte
	// grown holds the growing-keyspace leaves written since the tree was last
	// brought up to date, by key.
	grown map[string]grownLeaf
	// sizes are the growing keyspaces' sizes when the tree was last brought up
	// to date. With grown they predict the current sizes, which catches a write
	// that bypassed the journal.
	sizes [growingSpaces]int
	// owned is false on a dry-run clone, which reads its parent's tree and
	// never changes it.
	owned bool
	// base is the tree's root when a clone took it. A parent that has since
	// moved its tree on leaves the clone nothing to build on.
	base ports.Hash
}

type grownLeaf struct {
	space   int
	present bool
	inTree  bool // whether the tree holds the key: its presence before the first write
}

// noteGrown records a write to a growing keyspace: key is now present or absent,
// and had says whether it was present just before the write.
func (rc *stateTreeCache) noteGrown(space int, rawKey []byte, had, present bool) {
	if rc == nil {
		return
	}
	k := string(statehash.Key(growingTags[space], rawKey))
	g, seen := rc.grown[k]
	if !seen {
		g = grownLeaf{space: space, inTree: had}
	}
	g.present = present
	rc.grown[k] = g
}

// sizesMatch reports whether the journal accounts for every change in the
// growing keyspaces' sizes since the tree was brought up to date.
func (rc *stateTreeCache) sizesMatch(c *Chain) bool {
	want := rc.sizes
	for _, g := range rc.grown {
		if g.present && !g.inTree {
			want[g.space]++
		}
		if !g.present && g.inTree {
			want[g.space]--
		}
	}
	return want == [growingSpaces]int{c.byRoot.Len(), c.spent.Len(), len(c.revoked)}
}

// changes lists what the tree must change to describe c, and c's bounded leaves.
func (rc *stateTreeCache) changes(c *Chain) ([]statehash.Change, map[string][]byte) {
	var out []statehash.Change
	for k, g := range rc.grown {
		switch {
		case g.present && !g.inTree:
			out = append(out, statehash.Change{Key: []byte(k), Value: statehash.Present})
		case !g.present && g.inTree:
			out = append(out, statehash.Change{Key: []byte(k)})
		}
	}
	bounded := boundedByKey(c)
	for k, v := range bounded {
		if old, ok := rc.bounded[k]; !ok || !bytes.Equal(old, v) {
			out = append(out, statehash.Change{Key: []byte(k), Value: v})
		}
	}
	for k := range rc.bounded {
		if _, ok := bounded[k]; !ok {
			out = append(out, statehash.Change{Key: []byte(k)})
		}
	}
	return out, bounded
}

func boundedByKey(c *Chain) map[string][]byte {
	leaves := c.boundedLeavesV5()
	m := make(map[string][]byte, len(leaves))
	for _, lf := range leaves {
		m[string(lf.Key)] = lf.Value
	}
	return m
}

func growingSizes(c *Chain) [growingSpaces]int {
	return [growingSpaces]int{c.byRoot.Len(), c.spent.Len(), len(c.revoked)}
}

// buildRootCache builds c's tree from every leaf: the one full rebuild, after
// which each block costs only its changes.
func (c *Chain) buildRootCache() bool {
	tree, err := statehash.NewTree(c.stateRootLeavesV5())
	if err != nil {
		c.rootCache = nil
		return false
	}
	c.rootCache = &stateTreeCache{tree: tree, bounded: boundedByKey(c),
		grown: map[string]grownLeaf{}, sizes: growingSizes(c), owned: true}
	return true
}

// syncRootCache brings c's own tree up to date with c's state, building it the
// first time. It reports whether c now has a current tree.
func (c *Chain) syncRootCache() bool {
	rc := c.rootCache
	if rc == nil || !rc.owned {
		return c.buildRootCache()
	}
	if !rc.sizesMatch(c) {
		return c.buildRootCache()
	}
	changes, bounded := rc.changes(c)
	if len(changes) > 0 {
		if err := rc.tree.Apply(changes); err != nil {
			return c.buildRootCache()
		}
	}
	rc.bounded, rc.grown, rc.sizes = bounded, map[string]grownLeaf{}, growingSizes(c)
	return true
}

// dryRunRootCache is the cache a dry-run clone of c starts with: c's tree,
// brought up to date first if c owns it, and the changes c has not applied to it
// when c is itself a clone.
func (c *Chain) dryRunRootCache() *stateTreeCache {
	rc := c.rootCache
	if rc == nil || rc.owned {
		if !c.syncRootCache() {
			return nil
		}
		rc = c.rootCache
	}
	grown := make(map[string]grownLeaf, len(rc.grown))
	for k, g := range rc.grown {
		grown[k] = g
	}
	return &stateTreeCache{tree: rc.tree, bounded: rc.bounded, grown: grown, sizes: rc.sizes, base: rc.tree.Root()}
}

// checkKeptRoots makes every root taken from a kept tree also be rebuilt from
// every leaf, and panics on a difference. Tests set it, so each state any test
// reaches checks the cache against the definition.
var checkKeptRoots = false

// keptRootV5 is c's v5 state root from the kept tree, or false when c has no
// tree to start from or the journal cannot account for c's state, in which
// case the caller rebuilds from every leaf.
func (c *Chain) keptRootV5() (ports.Hash, bool) {
	r, ok := c.keptRootV5Unchecked()
	if ok && checkKeptRoots {
		if full, err := statehash.Root(c.stateRootLeavesV5()); err == nil && full != r {
			panic(fmt.Sprintf("chain: kept state tree root %s, rebuild %s", r, full))
		}
	}
	return r, ok
}

func (c *Chain) keptRootV5Unchecked() (ports.Hash, bool) {
	rc := c.rootCache
	if rc == nil {
		return ports.Hash{}, false
	}
	if rc.owned {
		if !c.syncRootCache() {
			return ports.Hash{}, false
		}
		return c.rootCache.tree.Root(), true
	}
	if rc.tree.Root() != rc.base || !rc.sizesMatch(c) {
		return ports.Hash{}, false
	}
	changes, _ := rc.changes(c)
	r, err := rc.tree.RootWith(changes)
	if err != nil {
		return ports.Hash{}, false
	}
	return r, true
}
