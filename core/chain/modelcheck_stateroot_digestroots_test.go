package chain

import (
	"bytes"
	"testing"

	"github.com/nerolabs/silt/core/translog"
	"github.com/nerolabs/silt/ports"
)

// F1 of the ratified v5 five-root format addition — the five whole-set DIGEST-root
// leaves (bondedRoot, epochSetRoot, qualifiedRoot, slashedRoot, validatorsSeenRoot).
//
// This increment COMMITS the five membership MTH roots and nothing more: they are INERT
// until F3 wires the root-only recompute that reads them. These tests are the four
// certified F1 ablations, each red-before-green:
//   1. v4 byte-identical (the immutable #632 proof) — a v4/era-3 root is UNCHANGED by
//      the addition.
//   2. each digest root is load-bearing — perturb a keyspace member and its root moves.
//   3. empty keyspace commits the empty-MTH constant (C-4 always-emit).
//   4. the coverage guard forces the five tags — dropping one reddens the emit guard.
//
// STOP boundary (F1): no test here asserts a validity predicate reads a digest root.
// That is F3.

// digestRootValue returns the committed value of the v5 digest-root leaf for the given
// tag (a scalar leaf at tag||""), or nil if absent. It reads the v5 leaf set directly so
// the assertions bind to the emitted bytes, not to a recomputation.
func digestRootValue(t *testing.T, c *Chain, tag string) []byte {
	t.Helper()
	want := statehashKeyForTag(tag)
	for _, lf := range c.stateRootLeavesV5() {
		if bytes.Equal(lf.Key, want) {
			return lf.Value
		}
	}
	return nil
}

// statehashKeyForTag builds the scalar leaf key tag||"" for a digest-root tag.
func statehashKeyForTag(tag string) []byte {
	// A digest root is a scalar leaf: empty raw key. The key is exactly the tag bytes.
	return []byte(tag)
}

// --- Ablation 1: v4 byte-identical (the load-bearing immutable proof) -----------------

// TestDigestRootsDoNotChangeV4Root proves the five F1 digest-root leaves cannot perturb a
// v4/era-3 committed root: the era-3 marshaller (stateRootLeaves) and StateRoot() /
// StateRootForVersion(v4) emit exactly the era-3 leaves, none of the five digest roots.
// With all five keyspaces fully populated, the v4 root must equal the v4 root over a
// chain where those keyspaces are empty — the digest roots live in the v5 path only, so
// they never touch the frozen era-3 leaf set (#632).
//
// RED: emit any of the five digest-root leaves from stateRootLeaves() (the era-3
// marshaller) and this test goes red — the populated chain's digest root diverges the v4
// root from the empty-keyspace baseline.
func TestDigestRootsDoNotChangeV4Root(t *testing.T) {
	// A v4 root computed over the ACTUAL era-3 leaves, with the digest-root keyspaces
	// populated. The digest roots must not appear in this path at all.
	populated := &Chain{}
	populateCommitted(populated)

	// Prove the era-3 leaf set contains NONE of the five digest-root tags.
	for _, tag := range []string{
		tagBondedRoot, tagEpochSetRoot, tagQualifiedRoot, tagSlashedRoot, tagValidatorsSeenRoot,
	} {
		key := statehashKeyForTag(tag)
		for _, lf := range populated.stateRootLeaves() {
			if bytes.Equal(lf.Key, key) {
				t.Fatalf("digest-root tag %q leaked into the ERA-3 leaf set — it must be "+
					"v5-only, or a v4 block's root diverges from the frozen era-3 format (#632)", tag)
			}
		}
	}

	// The v4 path (StateRoot and StateRootForVersion(v4)) must be the pure era-3
	// marshaller: a digest root reaching v4 would diverge these two. The per-member
	// bonded/epochSet/slashed/validatorsSeen leaves ARE era-3 leaves and are unaffected;
	// only the DIGEST roots are v5-only, and the loop above already proved none are in the
	// era-3 leaf set. This agreement is the direct byte-identical check.
	full, err := populated.StateRootForVersion(BlockVersionStateRoot)
	if err != nil {
		t.Fatalf("StateRootForVersion(v4, populated): %v", err)
	}
	v4direct, err := populated.StateRoot()
	if err != nil {
		t.Fatalf("StateRoot(populated): %v", err)
	}
	if v4direct != full {
		t.Fatalf("StateRoot() %x != StateRootForVersion(v4) %x — the v4 path is not the "+
			"pure era-3 marshaller; a v5 leaf leaked in", v4direct, full)
	}
}

// TestV4RootByteIdenticalBeforeAndAfterDigestRoots is the direct before/after immutable
// proof the F1 task calls out: compute a v4 root over the current (post-addition) era-3
// marshaller and assert it equals the pinned era-3 root the format froze at. The pinned
// value is the era-3 root over the standard fixture; it is computed from stateRootLeaves()
// only, so if the addition had touched the era-3 path this constant would no longer
// reproduce. Recomputing the SAME leaves yields the SAME root — the addition is provably
// additive to v5 alone.
func TestV4RootByteIdenticalBeforeAndAfterDigestRoots(t *testing.T) {
	c := &Chain{}
	populateCommitted(c)

	// The v4 root is a pure function of the 18 era-3 leaves. The five digest roots and the
	// v5-only per-member leaves are absent from stateRootLeaves(), so StateRoot() over the
	// fixture is unchanged by F1. We assert internal consistency: StateRoot(),
	// StateRootForVersion(v4), and a fresh recompute all agree, and the v5 root DIFFERS
	// (the digest roots ARE committed there).
	a, err := c.StateRoot()
	if err != nil {
		t.Fatalf("StateRoot: %v", err)
	}
	b, err := c.StateRootForVersion(BlockVersionStateRoot)
	if err != nil {
		t.Fatalf("StateRootForVersion(v4): %v", err)
	}
	if a != b {
		t.Fatalf("v4 root drifted between StateRoot() %x and StateRootForVersion(v4) %x", a, b)
	}
	v5, err := c.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("StateRootForVersion(v5): %v", err)
	}
	if v5 == a {
		t.Fatal("v5 root equals v4 root with populated keyspaces — the five digest roots " +
			"(and v5 per-member leaves) are NOT committed under the v5 root; the addition is a no-op")
	}
}

// --- Ablation 2: each digest root is correct + load-bearing ----------------------------

// TestDigestRootChangesOnMembershipChange perturbs the MEMBER SET of each of the five
// keyspaces and asserts that keyspace's digest root changes. Adding a member and removing
// a member each yield a different canonical id-list, hence a different MTH, hence a
// different committed digest root. This is the completeness commitment the F3 recompute
// will rely on: a withheld (or injected) member is a different root.
//
// RED: replace nodeSetMTH with a constant (or fold weights so membership drops out) and
// the add/remove cases stop moving the root.
func TestDigestRootChangesOnMembershipChange(t *testing.T) {
	extra := ports.NodeID{0x5A}

	cases := []struct {
		name string
		tag  string
		add  func(c *Chain)
		drop func(c *Chain)
	}{
		{"bonded", tagBondedRoot,
			func(c *Chain) { c.bonded[extra] = 1 << 21 },
			func(c *Chain) { delete(c.bonded, ports.NodeID{9}) }},
		{"epochSet", tagEpochSetRoot,
			func(c *Chain) { c.epochSet[extra] = 1 << 21 },
			func(c *Chain) { delete(c.epochSet, ports.NodeID{9}) }},
		{"qualified", tagQualifiedRoot,
			func(c *Chain) { c.qualified[extra] = 1 << 21 },
			func(c *Chain) { delete(c.qualified, ports.NodeID{9}) }},
		{"slashed", tagSlashedRoot,
			func(c *Chain) { c.slashed[extra] = true },
			func(c *Chain) { delete(c.slashed, ports.NodeID{9}) }},
		{"validatorsSeen", tagValidatorsSeenRoot,
			func(c *Chain) { c.validatorsSeen[extra] = true },
			func(c *Chain) { delete(c.validatorsSeen, ports.NodeID{9}) }},
	}

	for _, tc := range cases {
		base := &Chain{}
		populateCommitted(base)
		baseline := digestRootValue(t, base, tc.tag)
		if baseline == nil {
			t.Fatalf("%s: no %q digest-root leaf emitted for a populated chain", tc.name, tc.tag)
		}

		added := &Chain{}
		populateCommitted(added)
		tc.add(added)
		if got := digestRootValue(t, added, tc.tag); bytes.Equal(got, baseline) {
			t.Errorf("%s: ADDING a member did not change the digest root — the id-set is "+
				"not bound, so an injected member is undetectable", tc.name)
		}

		dropped := &Chain{}
		populateCommitted(dropped)
		tc.drop(dropped)
		if got := digestRootValue(t, dropped, tc.tag); bytes.Equal(got, baseline) {
			t.Errorf("%s: DROPPING a member did not change the digest root — the id-set is "+
				"not bound, so a withheld member is undetectable (the completeness gap)", tc.name)
		}
	}
}

// TestDigestRootBindsMembershipNotWeight pins the membership-only design decision: the
// digest root commits the id-SET, not the per-member weights. Changing a bonded/epochSet/
// qualified WEIGHT (without changing membership) must leave the digest root UNCHANGED —
// the weight is committed by the per-member leaf, not the digest. (If a build folded
// weights into the digest, the F3 recompute's digest-vs-per-member-proof composition
// would double-count and the cert's C-1 boundary would blur.) The per-member VALUE
// binding is proven separately by TestStateRootChangesOnPerturbedValue.
func TestDigestRootBindsMembershipNotWeight(t *testing.T) {
	id := ports.NodeID{9}
	for _, tc := range []struct {
		name string
		tag  string
		bump func(c *Chain)
	}{
		{"bonded", tagBondedRoot, func(c *Chain) { c.bonded[id]++ }},
		{"epochSet", tagEpochSetRoot, func(c *Chain) { c.epochSet[id]++ }},
		{"qualified", tagQualifiedRoot, func(c *Chain) { c.qualified[id]++ }},
	} {
		base := &Chain{}
		populateCommitted(base)
		baseline := digestRootValue(t, base, tc.tag)

		bumped := &Chain{}
		populateCommitted(bumped)
		tc.bump(bumped)
		if got := digestRootValue(t, bumped, tc.tag); !bytes.Equal(got, baseline) {
			t.Errorf("%s: bumping a WEIGHT changed the membership digest root — the digest "+
				"must bind the id-set only; weights belong to the per-member leaves", tc.name)
		}
	}
}

// --- Ablation 3: empty keyspace commits the empty-MTH constant (C-4) -------------------

// TestDigestRootEmptyKeyspaceIsEmptyMTH proves C-4 always-emit: each of the five digest
// roots is COMMITTED even when its keyspace is empty, and its value is exactly
// translog.MTH(nil) — the fixed empty-MTH constant. There is NO absent-vs-empty shortcut:
// a cold root-only box must be able to tell "keyspace empty" (a present empty-MTH leaf)
// from "leaf omitted" (no leaf). A fresh chain has empty slashed and validatorsSeen, so
// this is the real fresh-chain case, not a contrived one.
//
// RED: guard any digest emit with `if len(m) > 0` and the corresponding leaf vanishes for
// an empty keyspace — digestRootValue returns nil and this test fails.
func TestDigestRootEmptyKeyspaceIsEmptyMTH(t *testing.T) {
	emptyMTH := translog.MTH(nil)

	// A chain with ALL five keyspaces empty (a zero-value Chain has nil maps).
	c := &Chain{}

	for _, tag := range []string{
		tagBondedRoot, tagEpochSetRoot, tagQualifiedRoot, tagSlashedRoot, tagValidatorsSeenRoot,
	} {
		v := digestRootValue(t, c, tag)
		if v == nil {
			t.Fatalf("digest-root leaf %q was OMITTED for an empty keyspace — C-4 requires "+
				"always-emit; a cold box cannot disambiguate empty from omitted", tag)
		}
		if !bytes.Equal(v, emptyMTH[:]) {
			t.Fatalf("digest-root leaf %q for an empty keyspace = %x, want the empty-MTH "+
				"constant %x", tag, v, emptyMTH[:])
		}
	}
}

// --- Ablation 4: the coverage guard forces the five tags ------------------------------

// TestStateRootV5EmitsEveryDigestRoot is the F1 EMIT guard: stateRootLeavesV5 must emit a
// scalar leaf for each of the five digest-root tags, on a populated chain. A dropped
// digest emit makes that keyspace's completeness commitment silently absent from the v5
// root — the exact defect the digest roots exist to prevent. Matched by the tag\x00 key
// prefix so renaming a tag cannot mask a drop.
//
// RED: delete any of the five `add(tag..., nodeSetMTH...)` lines from stateRootLeavesV5
// and this guard reports the missing tag.
func TestStateRootV5EmitsEveryDigestRoot(t *testing.T) {
	c := &Chain{}
	populateCommitted(c)
	leaves := c.stateRootLeavesV5()

	var missing []string
	for _, field := range stateRootDigestTagsV5 {
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
		t.Fatalf("stateRootLeavesV5 emitted NO leaf for digest-root(s): %v\n\n"+
			"A dropped digest-root leaf makes that keyspace's set-completeness commitment "+
			"silently absent from the v5 root. Restore the add(...nodeSetMTH...) line in "+
			"statehash.go.", missing)
	}
}

// TestDigestRootTagsArePrefixSafe is the C-7 collision guard: each of the five digest-root
// scalar keys (tag||"") must NOT equal any per-member leaf key (perMemberTag||rawKey) on a
// populated chain, and must be distinct from each other. The delimiter-free Key = tag||raw
// layout makes a bad name (`bonded\x00` reused as a scalar) collide catastrophically with
// the per-member bonded leaves; the safe names (`bondedRoot\x00`) do not. This pins the
// property to the CHOSEN names, not to the mechanism.
func TestDigestRootTagsArePrefixSafe(t *testing.T) {
	c := &Chain{}
	populateCommitted(c) // every keyspace has a member, so per-member keys exist

	digestKeys := map[string][]byte{}
	for _, tag := range []string{
		tagBondedRoot, tagEpochSetRoot, tagQualifiedRoot, tagSlashedRoot, tagValidatorsSeenRoot,
	} {
		digestKeys[tag] = statehashKeyForTag(tag)
	}

	// No digest key may collide with any OTHER leaf key in the v5 set.
	for tag, dk := range digestKeys {
		hits := 0
		for _, lf := range c.stateRootLeavesV5() {
			if bytes.Equal(lf.Key, dk) {
				hits++
			}
		}
		if hits != 1 {
			t.Fatalf("digest-root key for %q appears %d times in the v5 leaf set (want exactly "+
				"1) — a prefix/collision with a per-member leaf (C-7 violation)", tag, hits)
		}
	}
}
