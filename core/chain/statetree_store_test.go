package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// trackedNodes is a node store that records being closed, and any use after.
type trackedNodes struct {
	statehash.NodeStore
	stores *nodeStores
	closed bool
}

var errNodesClosed = errors.New("node store used after close")

func (s *trackedNodes) Get(k []byte) ([]byte, error) {
	if s.closed {
		s.stores.usedAfterClose++
		return nil, errNodesClosed
	}
	return s.NodeStore.Get(k)
}

func (s *trackedNodes) Set(k, v []byte) error {
	if s.closed {
		s.stores.usedAfterClose++
		return errNodesClosed
	}
	return s.NodeStore.Set(k, v)
}

func (s *trackedNodes) Close() error {
	if !s.closed {
		s.closed = true
		s.stores.closed++
	}
	return nil
}

// nodeStores opens trackedNodes and counts what happens to them.
type nodeStores struct {
	opened, closed, usedAfterClose int
}

func (n *nodeStores) open() (statehash.NodeStore, error) {
	n.opened++
	return &trackedNodes{NodeStore: simplemap.NewSimpleMap(), stores: n}, nil
}

// A chain whose state trees live in a supplied store reaches exactly the roots its
// blocks commit, which were computed by a chain that kept its tree in memory: where
// the nodes live changes cost, never a root. Reload checks every block's root, and
// the kept-root cross-check (on in tests) compares each with a rebuild from every leaf.
func TestStateTreeRootsAreTheSameInAnyNodeStore(t *testing.T) {
	built, _ := scaleChain(t, 40, 10)
	var stores nodeStores
	c := New(built.cfg, built.rep)
	c.SetBondVerifier(objectiveVerify)
	c.SetStateNodeStore(stores.open)
	if n, err := c.Reload(built.Blocks(0)); err != nil {
		t.Fatalf("reload in a supplied store failed at block %d: %v", n, err)
	}
	if stores.opened == 0 {
		t.Fatal("the chain never opened a store: the reload proved nothing about one")
	}
	want, err := built.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil || got != want {
		t.Fatalf("state root %s (%v) in a supplied store, %s in memory", got, err, want)
	}
}

// Every store a chain opens is closed once its tree is replaced or discarded, and
// none is used after: the scratch chain of a fork that loses, the tree a reorg
// replaces, and a tree rebuilt from every leaf. Only the live tree's store stays
// open. A store left open is a file left on a validator's disk.
func TestChainClosesEveryStateTreeStoreItOpens(t *testing.T) {
	built, _ := scaleChain(t, 30, 5)
	full := built.Blocks(0)
	var stores nodeStores
	c := New(built.cfg, built.rep)
	c.SetBondVerifier(objectiveVerify)
	c.SetStateNodeStore(stores.open)
	if _, err := c.Reload(full[:20]); err != nil {
		t.Fatal(err)
	}
	live := func(step string) {
		t.Helper()
		if open := stores.opened - stores.closed; open != 1 {
			t.Fatalf("%s: %d stores open (%d opened, %d closed), want only the live tree's", step, open, stores.opened, stores.closed)
		}
		if stores.usedAfterClose != 0 {
			t.Fatalf("%s: a closed store was used %d times", step, stores.usedAfterClose)
		}
	}
	live("reload")

	// A fork no heavier than ours: the scratch chain replays it, loses, and goes.
	if adopted, err := c.Reconcile(full[:20]); err != nil || adopted {
		t.Fatalf("reconcile with our own history: adopted %v, %v", adopted, err)
	}
	live("a fork that loses")

	// A taller fork: adopted, and our old tree goes.
	if adopted, err := c.Reconcile(full); err != nil || !adopted {
		t.Fatalf("reconcile with a taller fork: adopted %v, %v", adopted, err)
	}
	live("a fork that wins")

	// A rebuild from every leaf replaces the tree.
	if !c.buildRootCache() {
		t.Fatal("rebuild failed")
	}
	live("a rebuild")
	if _, err := c.StateRootForVersion(BlockVersionWitnessable); err != nil {
		t.Fatal(err)
	}
	live("a root after the rebuild")
}

// A store that cannot be opened leaves the tree in memory: the chain still has its
// roots, and the next tree tries the store again.
func TestAStoreThatWillNotOpenKeepsTheTreeInMemory(t *testing.T) {
	built, _ := scaleChain(t, 10, 5)
	tries := 0
	c := New(built.cfg, built.rep)
	c.SetBondVerifier(objectiveVerify)
	c.SetStateNodeStore(func() (statehash.NodeStore, error) {
		tries++
		return nil, errors.New("disk full")
	})
	if n, err := c.Reload(built.Blocks(0)); err != nil {
		t.Fatalf("reload with a failing store failed at block %d: %v", n, err)
	}
	if tries == 0 {
		t.Fatal("the chain never tried its store")
	}
}
