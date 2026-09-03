package chain

import (
	"bytes"
	"sort"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
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
// tags this file's marshaller commits must be EXACTLY the 18 committedSet fields the
// keystone classification enumerates. A committedSet field added to Chain without a
// leaf in stateRootLeaves would silently escape the root — the completeness bug the
// state root exists to prevent. This binds stateRootTags to the live classification
// by reflection, so it cannot drift.
func TestStateRootCoversExactlyTheCommittedSetFields(t *testing.T) {
	classified := map[string]bool{}
	for _, name := range fieldsOfKind(committedSet) {
		classified[name] = true
	}
	// A committedSet field is committed if it is in EITHER the era-3 tag set
	// (stateRootTags — emitted for every v4+ root) OR the era-4 v5 tag set
	// (stateRootTagsV5 — emitted ONLY for a v5 root). The v5 fields are deliberately
	// absent from stateRootTags so a v4 block's root stays byte-identical to era-3
	// (hazard-1); they escape the root ONLY if absent from BOTH lists.
	committed := map[string]bool{}
	for _, name := range stateRootTags {
		committed[name] = true
	}
	v5 := map[string]bool{}
	for _, name := range stateRootTagsV5 {
		committed[name] = true
		v5[name] = true
	}

	var missing []string
	for name := range classified {
		if !committed[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("committedSet field(s) NOT committed to the StateRoot: %v\n\n"+
			"A committedSet field with no leaf in stateRootLeaves/stateRootLeavesV5 escapes "+
			"the root entirely — a snapshot-booted node could diverge on it undetected. Add "+
			"its tag and leaf in statehash.go (stateRootTagsV5 for a v5-only field), or "+
			"reclassify the field.", missing)
	}

	// The era-3 and v5 tag sets must be DISJOINT: a v5-only field in stateRootTags would
	// leak into the v4 root and break the era-3 byte-identical freeze.
	for name := range v5 {
		for _, e3 := range stateRootTags {
			if name == e3 {
				t.Fatalf("field %q is in BOTH stateRootTags and stateRootTagsV5 — a v5-only "+
					"field emitted into the era-3 root breaks the byte-identical freeze (hazard-1)", name)
			}
		}
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

// TestStateRootV5CoversExactlyTheV5Fields is the CURRENT-ERA (v5) counterpart of the
// era-3 coverage guard above, and the guard statehash.go's stateRootTagsV5 comment cites.
// The era-3 guard binds the era-3 TAG LIST to the field classification; it says nothing
// about what the v5 marshaller actually EMITS. This closes the other side: the tag set
// stateRootLeavesV5 emits — on a fully-populated chain, over the exact leaves the v5 root
// is computed from — must be EXACTLY the union of the three declared v5 tag lists:
//
//   - stateRootTags         — the 18 era-3 committedSet fields (a v5 root is a superset);
//   - stateRootTagsV5       — the five era-4 maintenance-spine committedSet fields;
//   - stateRootDigestTagsV5 — the five F1 whole-set digest roots (derived, not fields).
//
// Both directions are load-bearing and NEITHER is covered by the existing guards:
//
//   - MISSING (declared, not emitted): TestStateRootV5EmitsALeafForEveryV5Field and
//     TestStateRootV5EmitsEveryDigestRoot each cover ONE list, and the era-3 emit guard
//     runs against stateRootLeaves. Nothing checks the era-3 leaves ON THE V5 PATH, so a
//     v5-only regression dropping an era-3 leaf loop from stateRootLeavesV5 stays green.
//   - EXTRA (emitted, not declared): nothing covers this at all. A leaf added to
//     stateRootLeavesV5 under a tag on no list is invisible to every list-driven guard —
//     the classification binding, both emit guards, and the leaf-diff coverage
//     meta-assertion all iterate a LIST — so it enters the consensus root unreviewed, and
//     no witness or recompute knows it is there.
//
// Both sides are derived by construction: the emitted side from the live marshaller (a
// leaf key is tag\x00||rawKey, so the tag is the key up to the first NUL), the declared
// side from the tag lists themselves. Neither is a hand copy, so neither can drift.
//
// The final assertion ties the enumerated leaves to the ROOT: the SMT over
// stateRootLeavesV5() must equal StateRootForVersion(v5), so "covered" means covered by
// the root a v5 block commits, not merely present in a slice some other code path built.
//
// RED (demonstrated, 2026-09-02): drop `add(tagEpochStart, ...)` from stateRootLeavesV5
// and this test names epochStart MISSING; add an unlisted `add("shadow\x00", ...)` leaf
// and it names shadow EXTRA.
func TestStateRootV5CoversExactlyTheV5Fields(t *testing.T) {
	// DECLARED: the union of the three v5 tag lists, built from the lists themselves.
	declared := map[string]struct{}{}
	for _, list := range [][]string{stateRootTags, stateRootTagsV5, stateRootDigestTagsV5} {
		for _, tag := range list {
			declared[tag] = struct{}{}
		}
	}
	if len(declared) == 0 {
		t.Fatal("the v5 tag lists are all empty — this guard would pass vacuously")
	}

	// EMITTED: the tags the live v5 marshaller actually produces on a chain where every
	// committedSet field is populated (populateCommitted's completeness against the
	// classification is itself pinned, by TestAdoptCopiesEveryCommittedField).
	//
	// v5EmittedLeafTags, NOT v5EmittableLeafTags: this guard asks what a v5 block COMMITS,
	// so it takes no exclusions. The sibling helper subtracts leafDiffOutOfScopeTags, which
	// is the different question of what the diff-minus-fold guard can drive — using it here
	// would report a genuinely-committed keyspace as absent from the root.
	emitted := v5EmittedLeafTags(t)
	if len(emitted) == 0 {
		t.Fatal("stateRootLeavesV5 emitted NO leaves for a fully-populated chain — the v5 " +
			"marshaller is not running, so this guard would pass vacuously")
	}

	var missing, extra []string
	for tag := range declared {
		if _, ok := emitted[tag]; !ok {
			missing = append(missing, tag)
		}
	}
	for tag := range emitted {
		if _, ok := declared[tag]; !ok {
			extra = append(extra, tag)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("the v5 state root does NOT cover declared tag(s): %v\n\n"+
			"The tag is in stateRootTags / stateRootTagsV5 / stateRootDigestTagsV5 but "+
			"stateRootLeavesV5 emits no leaf for it on a fully-populated chain — the keyspace "+
			"is silently absent from the root a v5 block commits, so two replicas can diverge "+
			"on it undetected. Restore the leaf loop in statehash.go.\n"+
			"  declared(%d): %v\n  emitted(%d): %v",
			missing, len(declared), sortedSet(declared), len(emitted), sortedSet(emitted))
	}
	if len(extra) > 0 {
		t.Errorf("the v5 state root covers tag(s) on NO declared list: %v\n\n"+
			"stateRootLeavesV5 emits a leaf under a tag that is in none of stateRootTags, "+
			"stateRootTagsV5, stateRootDigestTagsV5. Every other coverage guard iterates one of "+
			"those LISTS, so an unlisted tag is invisible to all of them and enters the consensus "+
			"root unreviewed. Add the tag to the list its class belongs to (a committedSet field "+
			"to stateRootTagsV5, a derived whole-set digest to stateRootDigestTagsV5), or drop "+
			"the leaf.\n  declared(%d): %v\n  emitted(%d): %v",
			extra, len(declared), sortedSet(declared), len(emitted), sortedSet(emitted))
	}

	// The enumerated leaves must BE the v5 root's preimage. Without this the set equality
	// above could hold over a leaf slice the committed root is not actually computed from.
	c := &Chain{}
	populateCommitted(c)
	want, err := statehash.Root(c.stateRootLeavesV5())
	if err != nil {
		t.Fatalf("statehash.Root over stateRootLeavesV5: %v", err)
	}
	got, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion(v5): %v", err)
	}
	if got != want {
		t.Fatalf("StateRootForVersion(v5) = %x but the SMT over stateRootLeavesV5() = %x — the "+
			"v5 root is NOT computed from the leaf set this guard enumerates, so its coverage "+
			"assertion says nothing about the committed root", got, want)
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

// TestStateRootV5EmitsALeafForEveryV5Field is the era-4 EMIT guard: the v5 marshaller
// (stateRootLeavesV5) must emit at least one leaf for each of the three v5-only
// committedSet fields. A dropped v5 leaf loop makes that field silently absent from the
// v5 root — the completeness defect the state root exists to prevent, now for era-4.
// Matched by the fieldName\x00 key prefix, so renaming a tag cannot mask a drop.
func TestStateRootV5EmitsALeafForEveryV5Field(t *testing.T) {
	c := &Chain{}
	populateCommitted(c)

	leaves := c.stateRootLeavesV5()
	var missing []string
	for _, field := range stateRootTagsV5 {
		prefix := append([]byte(field), 0x00)
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
		t.Fatalf("stateRootLeavesV5 emitted NO leaf for v5 committedSet field(s): %v\n\n"+
			"The field is in stateRootTagsV5 but its leaf loop is absent from "+
			"stateRootLeavesV5 — the field is silently missing from the v5 StateRoot. "+
			"Restore the leaf loop in statehash.go.", missing)
	}
}

// TestEra3RootByteIdenticalWithV5KeyspacesPresent is the hazard-1 gate: with the era-4
// maintenance-spine maps FULLY POPULATED, the era-3 (v4) root — StateRoot(), and
// StateRootForVersion(v4) — must be byte-identical to the root over a chain with those
// maps EMPTY. The v5 keyspaces must not leak into the v4 root, or the era-3
// byte-identical freeze (ratified 2026-08-29) breaks and every deployed v4 node
// diverges.
//
// RED (the ablation, demonstrated in the 4b report): route the era-3 path through the
// v5 marshaller (emit the v5 leaves into the v4 root) and this test goes red — the two
// roots diverge because the populated chain's v5 leaves change the root.
func TestEra3RootByteIdenticalWithV5KeyspacesPresent(t *testing.T) {
	withV5 := &Chain{}
	populateCommitted(withV5) // sets qualified, dueBucket, epochStart among others

	withoutV5 := &Chain{}
	populateCommitted(withoutV5)
	withoutV5.qualified = map[ports.NodeID]int64{}
	withoutV5.dueBucket = map[uint64]map[ports.NodeID]struct{}{}
	withoutV5.epochStart = 0
	// Zero the era-4 latch scalars too. populateCommitted sets these in BOTH
	// baselines, so a v5-only scalar (era4LockedIn/era4Height) leaked into the v4
	// marshaller — INCLUDING under a fresh unregistered tag — would appear
	// identically in withV5 and withoutV5 and CANCEL, leaving this freeze guard
	// green on a real leak. Zeroing them here makes any such leak diverge the two
	// v4 roots, so the guard reddens (the hazard-1 / era-3 FREEZE, #632).
	withoutV5.era4LockedIn = false
	withoutV5.era4Height = 0

	// The era-3 entry point must ignore the v5 maps entirely.
	a, err := withV5.StateRoot()
	if err != nil {
		t.Fatalf("StateRoot(withV5): %v", err)
	}
	b, err := withoutV5.StateRoot()
	if err != nil {
		t.Fatalf("StateRoot(withoutV5): %v", err)
	}
	if a != b {
		t.Fatalf("era-3 StateRoot() is NOT byte-identical with v5 keyspaces present: %x != %x — "+
			"the v5 leaves leaked into the v4 root, breaking the era-3 freeze (hazard-1)", a, b)
	}

	// StateRootForVersion(v4) must agree with StateRoot() and be v5-invariant too.
	av4, err := withV5.StateRootForVersion(BlockVersionStateRoot)
	if err != nil {
		t.Fatalf("StateRootForVersion(v4, withV5): %v", err)
	}
	if av4 != a {
		t.Fatalf("StateRootForVersion(v4) %x != StateRoot() %x — the v4 path must be the era-3 marshaller", av4, a)
	}

	// The v5 root MUST differ (the v5 leaves ARE committed there) — proves the gate is
	// a real era switch, not a no-op that would make 4c/4d meaningless.
	av5, err := withV5.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion(v5, withV5): %v", err)
	}
	if av5 == a {
		t.Fatalf("v5 StateRoot equals v4 StateRoot with populated v5 maps — the v5 keyspaces " +
			"are NOT committed under the v5 root; the era gate is a no-op")
	}
}

// TestStateRootV5IsOrderIndependent proves the v5 root is a pure function of the
// committed set: recomputing over the same chain (random map order, and a due-bucket
// whose id set is committed as an MTH over the CANONICAL sorted list) yields the
// identical root. A non-canonical (map-order) bucket encoding would make this flap.
func TestStateRootV5IsOrderIndependent(t *testing.T) {
	c := &Chain{}
	populateCommitted(c)
	// A multi-id bucket is the case where canonical ordering actually bites.
	c.dueBucket = map[uint64]map[ports.NodeID]struct{}{
		43: {ports.NodeID{9}: {}, ports.NodeID{3}: {}, ports.NodeID{7}: {}, ports.NodeID{1}: {}},
	}
	c.qualified = map[ports.NodeID]int64{
		{9}: 1 << 21, {3}: 1 << 21, {7}: 1 << 21,
	}

	first, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("v5 StateRoot: %v", err)
	}
	for i := 0; i < 50; i++ {
		got, err := c.StateRootForVersion(BlockVersionWitnessable)
		if err != nil {
			t.Fatalf("v5 StateRoot recompute %d: %v", i, err)
		}
		if got != first {
			t.Fatalf("v5 StateRoot is not order-independent: recompute %d = %x, first = %x", i, got, first)
		}
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
// commits the six scalar leaves (everMature=false, etc.), so the "empty" root is the
// root over exactly those six scalar leaves — a fixed constant, not the empty-tree
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
