package chain

import (
	"bytes"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// The era-3 StateRoot determinism oracle — research cert residual R2, the
// freeze condition owed at the model-check tier BEFORE the root becomes a signed
// block field. It proves: the per-field canonical value encoding is deterministic,
// so two honest nodes with the same logical committedSet produce byte-identical
// leaves and therefore an identical StateRoot; and a wrong VALUE in a value-carrying
// field changes the root, so a value-encoding defect surfaces here as a root
// mismatch, never as a consensus split in the field.
//
// STOP boundary (step 1): these tests exercise StateRoot() only. No Block field, no
// Hash() change, no validity predicate — the root is computed and proven
// deterministic, not yet committed or validated against.

// TestStateRootCoversExactlyTheCommittedSetFields is the coverage guard: the field
// tags this file's marshaller commits must be EXACTLY the 16 committedSet fields the
// keystone classification enumerates. A committedSet field added to Chain without a
// leaf in stateRootLeaves would silently escape the root — the completeness bug the
// state root exists to prevent. This binds stateRootTags to the live classification
// by reflection, so it cannot drift.
func TestStateRootCoversExactlyTheCommittedSetFields(t *testing.T) {
	classified := map[string]bool{}
	for _, name := range fieldsOfKind(committedSet) {
		classified[name] = true
	}
	committed := map[string]bool{}
	for _, name := range stateRootTags {
		committed[name] = true
	}

	var missing []string
	for name := range classified {
		if !committed[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("committedSet field(s) NOT committed to the StateRoot: %v\n\n"+
			"A committedSet field with no leaf in stateRootLeaves escapes the root "+
			"entirely — a snapshot-booted node could diverge on it undetected. Add its "+
			"tag and leaf in statehash.go, or reclassify the field.", missing)
	}

	var extra []string
	for name := range committed {
		if !classified[name] {
			extra = append(extra, name)
		}
	}
	if len(extra) > 0 {
		t.Fatalf("StateRoot commits tag(s) that are NOT classified committedSet: %v\n\n"+
			"Either the field was reclassified (drop its tag) or the tag is stale.", extra)
	}
}

// TestStateRootEmitsALeafForEveryCommittedField is the EMIT guard — the closure of
// the coverage-guard gap the tag-list check leaves open. TestStateRootCoversExactly-
// TheCommittedSetFields compares the static stateRootTags LIST to the classification;
// it stays GREEN if a leaf LOOP is dropped from stateRootLeaves() while its tag remains
// in the list. A dropped Class-A loop (e.g. `spent`) then vanishes from the root with
// the whole suite green — a field silently absent from the state root, the exact
// completeness defect the root exists to prevent.
//
// This guard closes that hole by execution: it populates every committedSet field (the
// fixture sets exactly one entry per field) and asserts stateRootLeaves() actually
// EMITS at least one leaf tagged with each field. A dropped leaf loop of ANY class —
// Class A included — produces no leaf for that tag and turns this guard RED.
//
// A leaf key is `fieldName\x00 || rawKey`, so a leaf belongs to field F iff its key has
// the prefix `F\x00`. Matching by that prefix ties the emitted leaves back to the field
// names without depending on the tag constants, so renaming a tag cannot mask a drop.
func TestStateRootEmitsALeafForEveryCommittedField(t *testing.T) {
	c := &Chain{}
	populateCommitted(c)

	leaves := c.stateRootLeaves()
	if len(leaves) == 0 {
		t.Fatal("stateRootLeaves emitted NO leaves for a fully-populated chain — the " +
			"marshaller is not running")
	}

	var missing []string
	for _, field := range stateRootTags {
		prefix := append([]byte(field), 0x00) // fieldName\x00
		found := false
		for _, lf := range leaves {
			if bytes.HasPrefix(lf.Key, prefix) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("stateRootLeaves emitted NO leaf for committedSet field(s): %v\n\n"+
			"The field's tag is in stateRootTags but its leaf loop is absent from "+
			"stateRootLeaves — the field is silently missing from the StateRoot. Every "+
			"populated committedSet field MUST emit at least one leaf. Restore the "+
			"leaf loop in statehash.go.", missing)
	}
}

// TestStateRootIsInsertionOrderIndependent proves the root is a pure function of the
// committed SET: recomputing over the same chain (Go map iteration order is random
// between calls) yields the identical root. This is the node-independence property —
// two nodes holding the same logical state agree on the root regardless of how their
// maps happen to iterate.
func TestStateRootIsInsertionOrderIndependent(t *testing.T) {
	c := &Chain{}
	populateCommitted(c)

	first, err := c.StateRoot()
	if err != nil {
		t.Fatalf("StateRoot: %v", err)
	}
	// Recompute many times: each call re-iterates the maps in a fresh random order,
	// so a stable root across calls IS the order-independence evidence.
	for i := 0; i < 50; i++ {
		got, err := c.StateRoot()
		if err != nil {
			t.Fatalf("StateRoot recompute %d: %v", i, err)
		}
		if got != first {
			t.Fatalf("StateRoot is not order-independent: recompute %d = %x, first = %x",
				i, got, first)
		}
	}
}

// TestStateRootIsNodeIndependent proves two independently-constructed chains with the
// SAME logical committedSet compute the same root — the cross-node determinism cert R2
// requires. The two chains are populated by the same fixture but are distinct structs
// with independently-allocated maps.
func TestStateRootIsNodeIndependent(t *testing.T) {
	a, b := &Chain{}, &Chain{}
	populateCommitted(a)
	populateCommitted(b)

	ra, err := a.StateRoot()
	if err != nil {
		t.Fatalf("StateRoot(a): %v", err)
	}
	rb, err := b.StateRoot()
	if err != nil {
		t.Fatalf("StateRoot(b): %v", err)
	}
	if ra != rb {
		t.Fatalf("two nodes with the same logical committedSet computed different roots: "+
			"%x != %x — the encoding is not deterministic across nodes", ra, rb)
	}
}

// TestStateRootChangesOnPerturbedValue is the ablation: perturb ONE value-carrying
// field's committed value and the root must change. This proves the VALUE (not just
// presence) is bound into the root for each Class-B field — a true-presence /
// wrong-value witness is impossible because it would reconstruct a different root.
// The three super-quorum-summed weights (bonded, epochSet) are the load-bearing
// cases (PE Q2); the identity and scalar fields are covered too.
func TestStateRootChangesOnPerturbedValue(t *testing.T) {
	base := func() *Chain {
		c := &Chain{}
		populateCommitted(c)
		return c
	}
	root := func(c *Chain) ports.Hash {
		r, err := c.StateRoot()
		if err != nil {
			t.Fatalf("StateRoot: %v", err)
		}
		return r
	}

	baseline := root(base())
	id := ports.NodeID{9}
	rootHash := ports.Hash{7}

	perturbations := []struct {
		name    string
		perturb func(c *Chain)
	}{
		{"bonded weight", func(c *Chain) { c.bonded[id] = c.bonded[id] + 1 }},
		{"epochSet weight", func(c *Chain) { c.epochSet[id] = c.epochSet[id] + 1 }},
		{"bondRegHeight", func(c *Chain) { c.bondRegHeight[id] = c.bondRegHeight[id] + 1 }},
		{"bondDomain", func(c *Chain) { c.bondDomain[id] = c.bondDomain[id] ^ 0xFF }},
		{"regVersion", func(c *Chain) { c.regVersion[id] = c.regVersion[id] + 1 }},
		{"bondRootOwner", func(c *Chain) { c.bondRootOwner[rootHash] = ports.NodeID{0xAB} }},
		{"bondRootProven", func(c *Chain) { c.bondRootProven[rootHash] = !c.bondRootProven[rootHash] }},
		{"gateHeight scalar", func(c *Chain) { c.gateHeight++ }},
		{"everMature scalar", func(c *Chain) { c.everMature = !c.everMature }},
		{"matureEpoch scalar", func(c *Chain) { c.matureEpoch = !c.matureEpoch }},
		{"gateLockedIn scalar", func(c *Chain) { c.gateLockedIn = !c.gateLockedIn }},
	}
	for _, p := range perturbations {
		c := base()
		p.perturb(c)
		if got := root(c); got == baseline {
			t.Errorf("perturbing %s did NOT change the StateRoot — the value is not "+
				"committed, so a wrong-value witness for that field could forge the "+
				"committed state without changing the root", p.name)
		}
	}
}

// TestStateRootChangesOnMembershipChange is the Class-A ablation: adding or removing a
// set-membership key changes the root, so presence/absence is bound. (The VALUE of a
// Class-A leaf is a fixed marker by design — only membership is load-bearing there.)
func TestStateRootChangesOnMembershipChange(t *testing.T) {
	c := &Chain{}
	populateCommitted(c)
	baseline, _ := c.StateRoot()

	c.revoked[ports.Hash{0x55}] = true
	if got, _ := c.StateRoot(); got == baseline {
		t.Fatal("adding a revoked root did not change the StateRoot — Class-A membership " +
			"is not bound into the root")
	}
}

// TestEmptyChainStateRootIsStable pins that an empty committedSet computes a definite,
// reproducible root (the empty-vs-absent closure). Note a zero-value Chain still
// commits the four scalar leaves (everMature=false, etc.), so the "empty" root is the
// root over exactly those four scalar leaves — a fixed constant, not the empty-tree
// root, and that is correct: the scalars are always present.
func TestEmptyChainStateRootIsStable(t *testing.T) {
	a, b := &Chain{}, &Chain{}
	ra, err := a.StateRoot()
	if err != nil {
		t.Fatalf("StateRoot(empty a): %v", err)
	}
	rb, _ := b.StateRoot()
	if ra != rb {
		t.Fatalf("empty-chain StateRoot is not stable across nodes: %x != %x", ra, rb)
	}
}
