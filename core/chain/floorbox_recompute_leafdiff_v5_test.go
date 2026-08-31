package chain

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// THE PERMANENT GUARD — emission-keyed differential leaf-diff completeness (PE ruling 2026-08-31,
// the write-obligation ledger):
//   silt-reviews/principle-engineer/RULING-floorbox-v5-write-obligation-ledger-2026-08-31.md
//
// The whole point is to STOP catching unreproduced committed-leaf writes ONE AT A TIME. Seven were
// caught by hand; the everMature off-boundary latch was the eighth. The existing …AgreesWithApply
// tests are all root-equality on hand-built fixtures — they only exercise the paths the fixture
// builds, so a write on a fixture nobody built (the off-boundary maturity crossing) HID. That is the
// "green check with no demonstrated red" failure mode.
//
// THE PROPERTY (for every v5 block a generator can produce):
//
//	{ committed-leaf DIFF a real apply() produces }  ==  { keys the recompute FOLDS }
//
// Not "every tag has a fold op somewhere," but "for THIS block, every leaf apply() actually changed
// is a leaf the recompute actually folded." The diff is computed from the LIVE marshaller
// (stateRootLeavesV5), so a FUTURE committed leaf tag is caught with ZERO guard edits: an added tag
// shows up in the pre/post diff automatically, and if its write has no reproducer the diff-minus-fold
// set is non-empty on the first generated block that writes it, reddening the guard by NAME.
//
// THE ABLATION (a guard with no demonstrated red is decoration): with class M removed from the
// recompute, the guard MUST fail naming everMature on the off-boundary crossing block. That RED is
// demonstrated by TestLeafDiffGuardAblationClassMRemoved below, which assembles the recompute ops
// with class M forced off and confirms the diff-minus-fold set is exactly {everMature}.

// committedLeafDiff returns the set of leaf KEYS whose committed value a real apply() of b changes:
// keys added, deleted, or value-changed between the PRE-apply and POST-apply stateRootLeavesV5(). It
// is the authoritative ground truth — the LIVE committed marshaller, not a hand list.
func committedLeafDiff(pre, post *Chain) map[string]struct{} {
	preMap := leafValueMap(pre.stateRootLeavesV5())
	postMap := leafValueMap(post.stateRootLeavesV5())
	diff := map[string]struct{}{}
	for k, pv := range preMap {
		nv, ok := postMap[k]
		if !ok || !bytes.Equal(pv, nv) {
			diff[k] = struct{}{} // deleted or value-changed
		}
	}
	for k, nv := range postMap {
		if pv, ok := preMap[k]; !ok || !bytes.Equal(pv, nv) {
			diff[k] = struct{}{} // added or value-changed
		}
	}
	return diff
}

func leafValueMap(leaves []statehash.Leaf) map[string][]byte {
	m := make(map[string][]byte, len(leaves))
	for _, lf := range leaves {
		m[string(lf.Key)] = lf.Value
	}
	return m
}

// foldedChangeKeys returns the set of leaf KEYS the recompute's ops actually CHANGE (OldValue !=
// NewValue). An op whose value does not change (an idempotent set-marker overwrite) is NOT a committed
// diff, so it is excluded — matching committedLeafDiff, which is value-change-based.
func foldedChangeKeys(ops []statehash.FoldOp) map[string]struct{} {
	out := map[string]struct{}{}
	for _, op := range ops {
		if !bytes.Equal(op.OldValue, op.NewValue) {
			out[string(op.Key)] = struct{}{}
		}
	}
	return out
}

// tagOfKey decodes a leaf key's field tag (the bytes up to and including the first NUL) for readable
// failure messages. statehash.Key(tag, rawKey) = tag || rawKey, and every tag ends in "\x00".
func tagOfKey(key string) string {
	if i := bytes.IndexByte([]byte(key), 0); i >= 0 {
		return key[:i]
	}
	return key
}

func sortedKeyTags(keys map[string]struct{}) []string {
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, tagOfKey(k))
	}
	sort.Strings(out)
	return out
}

// assertLeafDiffEqualsFold is the guard assertion: the committed-leaf diff a real apply() produces
// must equal the key-set the recompute folds. A key apply() changed but the recompute did NOT fold ⇒
// an UNREPRODUCED write (the everMature class of bug) ⇒ FAIL naming the tag. A key the recompute folds
// but apply() did NOT change ⇒ an OVER-emission (latent double-write) ⇒ also FAIL.
func assertLeafDiffEqualsFold(t testingFataler, diff, folded map[string]struct{}) {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
	var missing, extra []string
	for k := range diff {
		if _, ok := folded[k]; !ok {
			missing = append(missing, tagOfKey(k))
		}
	}
	for k := range folded {
		if _, ok := diff[k]; !ok {
			extra = append(extra, tagOfKey(k))
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Fatalf("GUARD RED: apply() changed committed leaves the recompute did NOT fold (unreproduced writes): %v\n"+
			"  full committed diff: %v\n  full folded set: %v", missing, sortedKeyTags(diff), sortedKeyTags(folded))
	}
	if len(extra) > 0 {
		t.Fatalf("GUARD RED: the recompute folded committed leaves apply() did NOT change (over-emission / latent double-write): %v\n"+
			"  full committed diff: %v\n  full folded set: %v", extra, sortedKeyTags(diff), sortedKeyTags(folded))
	}
}

// testingFataler is the minimal surface assertLeafDiffEqualsFold needs. *testing.T satisfies it, so
// the guard's real callers are unchanged; the S/B/T dropped-leaf ablations pass a capturing
// implementation so they can drive the PRODUCTION naming assertion and assert on its RED without
// failing the parent test.
type testingFataler interface {
	Fatalf(format string, args ...any)
}

// leafDiffScenario is one generated block + its honest witness, plus the PRE chain and prevRoot. The
// generator schedule (below) INCLUDES an OFF-boundary maturity crossing — the exact gap the existing
// fixtures hid — plus an ON-boundary crossing and the steady-state E/R/A/boundary blocks.
type leafDiffScenario struct {
	name     string
	pre      *Chain
	prevRoot ports.Hash
	b        Block
	w        StateRootWitness
}

// generateLeafDiffScenarios builds the reachability schedule the guard runs. Each scenario trips a
// distinct write condition; the OFF-boundary maturity crossing is the one no positive …AgreesWithApply
// test built before this change.
func generateLeafDiffScenarios(t *testing.T) []leafDiffScenario {
	t.Helper()
	var out []leafDiffScenario

	// (1) OFF-boundary maturity crossing (THE GAP): everMature latches at an ordinary height.
	{
		f := buildOffBoundaryMaturityFixture(t)
		b := f.crossingBlock()
		applied := f.applied(b)
		if !applied.everMature || b.Height%f.c.cfg.EpochBlocks == 0 {
			t.Fatalf("scenario off-boundary-crossing: precondition failed (mature=%v height=%d)", applied.everMature, b.Height)
		}
		out = append(out, leafDiffScenario{"off-boundary-maturity-crossing", f.c, f.prevRoot, b, f.witnessForCrossing(t, b)})
	}

	// (2) ON-boundary maturity crossing (the #678 case) — regression lock.
	{
		f := buildHandoffFixture(t)
		b := f.handoffBoundaryBlock()
		out = append(out, leafDiffScenario{"on-boundary-maturity-crossing", f.c, f.prevRoot, b, f.witnessForHandoff(t, b)})
	}

	// (3) Steady-state E/R block (no maturity change; already-latched chain).
	{
		f := buildStateRootFixture(t)
		b := f.nextERBlock()
		out = append(out, leafDiffScenario{"steady-state-E/R", f.c, f.prevRoot, b, f.witnessForBlock(t, b)})
	}

	// (4) Steady-state epoch boundary (already-latched chain, no crossing).
	{
		f := buildRotateFixture(t)
		b := f.boundaryBlock(nil)
		out = append(out, leafDiffScenario{"steady-state-boundary", f.c, f.prevRoot, b, f.witnessForBoundary(t, b)})
	}

	// (5) Class S — slash of a bonded+qualified culprit. Exercises slashed(+Root), bonded(+Root),
	// qualified(+Root) diffs. Reuses the certified P1-b slash fixture + witness.
	{
		f := buildSlashFixture(t)
		b := f.slashBlock()
		out = append(out, leafDiffScenario{"class-S-slash", f.c, f.prevRoot, b, f.witnessForSlash(t, b)})
	}

	// (6) Class T — a firing TTL sweep. Exercises dueBucket, bondRegHeight, regVersion, bonded(+Root),
	// qualified(+Root) diffs. Reuses the certified P1-c TTL fixture + witness.
	{
		f := buildTTLFixture(t)
		b := f.sweepBlock()
		expired := f.expiredMembers()
		if len(expired) == 0 {
			t.Fatalf("scenario class-T-sweep: no expired members in dueBucket[%d]", f.sweepH)
		}
		out = append(out, leafDiffScenario{"class-T-ttl-sweep", f.c, f.prevRoot, b, f.ttlSweepWitness(t, b, expired)})
	}

	// (7) Class B — a FRESH bond registration. Exercises bondDomain, bondRegHeight, bondRootOwner,
	// bondRootProven, regVersion, bonded(+Root), qualified(+Root), dueBucket diffs.
	{
		f := buildBondFixture(t)
		prev, h := f.c.Head()
		fresh := key(81009)
		b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), 4<<20, prev, 5, 9)}}
		newDue := h + f.c.cfg.BondTTLBlocks + 1
		out = append(out, leafDiffScenario{"class-B-bondreg-fresh", f.c, f.prevRoot, b, f.bondWitness(t, b, []uint64{newDue})})
	}

	// (8) Class B — a DISPLACEMENT: an honest prover beats a genesis squatter on a shared root. The
	// squatter (an id NOT in the payload) is stripped from bonded+qualified — the derived-delta path the
	// per-fixture guard must exercise for B.
	{
		f := buildBondFixture(t)
		prev, h := f.c.Head()
		honest := key(81003)
		b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			BondRegs: []BondReg{bondRegFull(honest, f.sharedRoot, 4<<20, prev, 5, 3)}}
		newDue := h + f.c.cfg.BondTTLBlocks + 1
		out = append(out, leafDiffScenario{"class-B-bondreg-displacement", f.c, f.prevRoot, b, f.bondWitness(t, b, []uint64{newDue})})
	}

	return out
}

// v5EmittableLeafTags is the FULL set of committed-leaf field tags stateRootLeavesV5 can emit,
// derived from the LIVE marshaller — NOT a hardcoded list. populateCommitted sets every committed
// keyspace and scalar (it is itself reflection-pinned to the committed classification by
// TestAdoptCopiesEveryCommittedField, so it cannot silently drop a field), and the marshaller then
// emits one leaf per keyspace/scalar. A FUTURE committed-leaf tag added to stateRootLeavesV5 shows up
// here automatically (populateCommitted must set its backing field, or the adopt guard reddens), which
// grows the coverage bar the meta-assertion below enforces — with ZERO edits to this function.
func v5EmittableLeafTags(t *testing.T) map[string]struct{} {
	t.Helper()
	c := &Chain{}
	populateCommitted(c)
	tags := map[string]struct{}{}
	for _, lf := range c.stateRootLeavesV5() {
		tags[tagOfKey(string(lf.Key))] = struct{}{}
	}
	return tags
}

// TestLeafDiffGuardCompleteness is the permanent emission-keyed guard: for every generated block, the
// committed-leaf diff a real apply() produces equals the key-set the recompute folds. It runs the
// reachability schedule — INCLUDING the off-boundary maturity crossing the …AgreesWithApply fixtures
// hid. GREEN with the class-M fix in; the ablation below demonstrates the RED.
func TestLeafDiffGuardCompleteness(t *testing.T) {
	for _, sc := range generateLeafDiffScenarios(t) {
		t.Run(sc.name, func(t *testing.T) {
			pre := sc.pre
			post := pre.cloneForDryRun()
			post.apply(sc.b)
			committed, err := post.StateRootForVersion(BlockVersionWitnessable)
			if err != nil {
				t.Fatalf("post StateRootForVersion: %v", err)
			}

			// Ground truth: the committed-leaf diff from the LIVE marshaller.
			diff := committedLeafDiff(pre, post)

			// The recompute's folded change-set (assembled via the real op pipeline). It must also AGREE
			// (fold to the committed root) — a guard over an already-stalling recompute would be vacuous.
			if err := pre.RecomputeStateRootEntriesRevocations(sc.prevRoot, committed, sc.b, sc.w); err != nil {
				t.Fatalf("recompute must AGREE for the guard to be meaningful, got %v", err)
			}
			ops, err := pre.assembleStateRootRecomputeOps(sc.prevRoot, committed, sc.b, sc.w)
			if err != nil {
				t.Fatalf("assembleStateRootRecomputeOps: %v", err)
			}
			folded := foldedChangeKeys(ops)

			assertLeafDiffEqualsFold(t, diff, folded)
		})
	}
}

// TestLeafDiffGuardAblationClassMRemoved is the ABLATION (RED demonstrated): with class M removed from
// the recompute (the pre-fix state on an off-boundary crossing), the guard's diff-minus-fold set is
// non-empty and is EXACTLY {everMature}. This proves the guard reddens on the exact gap it exists to
// catch — a guard with no demonstrated red is decoration. Restoring class M (the other guard test)
// makes it GREEN.
func TestLeafDiffGuardAblationClassMRemoved(t *testing.T) {
	f := buildOffBoundaryMaturityFixture(t)
	b := f.crossingBlock()
	pre := f.c
	post := pre.cloneForDryRun()
	post.apply(b)
	committed, err := post.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("post StateRootForVersion: %v", err)
	}
	w := f.witnessForCrossing(t, b)

	diff := committedLeafDiff(pre, post)

	// Assemble the recompute ops with class M FORCED OFF (nil the maturity witness's SeenSet path by
	// modelling the pre-fix pipeline: the E/R + A ops WITHOUT the everMature op).
	foldedPreFix := foldedChangeKeys(f.nonMaturityOps(t, b, w))

	// The guard must go RED: diff-minus-fold is non-empty and is exactly {everMature}.
	var missing []string
	for k := range diff {
		if _, ok := foldedPreFix[k]; !ok {
			missing = append(missing, tagOfKey(k))
		}
	}
	sort.Strings(missing)
	if len(missing) == 0 {
		t.Fatalf("ABLATION FAILED: with class M removed the guard stayed GREEN — the off-boundary everMature write was not detected as unreproduced")
	}
	wantTag := tagOfKey(tagEverMature) // the tag with its trailing NUL stripped, as tagOfKey reports it
	if len(missing) != 1 || missing[0] != wantTag {
		t.Fatalf("ABLATION: expected the guard to redden naming exactly {%q}, got %v", wantTag, missing)
	}

	// And with class M IN (the real pipeline), the same block is GREEN — the RED→GREEN pair.
	ops, err := pre.assembleStateRootRecomputeOps(f.prevRoot, committed, b, w)
	if err != nil {
		t.Fatalf("assembleStateRootRecomputeOps (fixed): %v", err)
	}
	assertLeafDiffEqualsFold(t, diff, foldedChangeKeys(ops))
}

// TestLeafDiffGuardCoversEveryEmittableTag is the SELF-CHECKING coverage meta-assertion (PE ledger
// ruling acceptance bar: E/R/S/B/T/A/P all driven through the diff assertion, and the generator's OWN
// blind spot closed permanently). It asserts the UNION of committed-leaf tags the guard's scenarios
// actually exercise (as a real apply() diff) EQUALS the FULL tag set the live marshaller can emit
// (v5EmittableLeafTags, derived from populateCommitted — not a hand list).
//
// WHY THIS CLOSES THE GENERATOR BLIND SPOT: the diff-minus-fold guard reddens on a dropped emission
// ONLY for a tag some scenario produces a block for. Before this change the generator drove 16 of 28
// tags, so a future dropped S/B/T (or any of the other missing) emission would not redden the shipped
// suite — the guard that exists to END the per-fixture blind spot HAD it. This meta-assertion makes
// that gap SELF-DETECTING forever: a future format tag added to the marshaller grows the emittable set
// (see v5EmittableLeafTags), and if no scenario exercises it THIS test FAILS naming the tag — forcing a
// scenario before merge. Its teeth are demonstrated by TestLeafDiffCoverageMetaHasTeeth below (drop a
// scenario ⇒ this assertion fails).
func TestLeafDiffGuardCoversEveryEmittableTag(t *testing.T) {
	want := v5EmittableLeafTags(t)
	got := exercisedLeafDiffTags(t, generateLeafDiffScenarios(t))
	assertTagSetsEqual(t, want, got)
}

// exercisedLeafDiffTags returns the UNION of committed-leaf field tags that appear in the real apply()
// diff of every scenario — the set the guard actually exercises. Keyed on the live stateRootLeavesV5
// marshaller (committedLeafDiff), so it tracks the real emission, not a hand list.
func exercisedLeafDiffTags(t *testing.T, scenarios []leafDiffScenario) map[string]struct{} {
	t.Helper()
	out := map[string]struct{}{}
	for _, sc := range scenarios {
		post := sc.pre.cloneForDryRun()
		post.apply(sc.b)
		for k := range committedLeafDiff(sc.pre, post) {
			out[tagOfKey(k)] = struct{}{}
		}
	}
	return out
}

// assertTagSetsEqual fails if want != got, naming the tags a scenario must add (uncovered) or the tags
// exercised but not emittable (a stale key). Uncovered is the load-bearing direction: a marshaller tag
// no scenario drives is the generator blind spot.
func assertTagSetsEqual(t *testing.T, want, got map[string]struct{}) {
	t.Helper()
	var uncovered, extra []string
	for tag := range want {
		if _, ok := got[tag]; !ok {
			uncovered = append(uncovered, tag)
		}
	}
	for tag := range got {
		if _, ok := want[tag]; !ok {
			extra = append(extra, tag)
		}
	}
	sort.Strings(uncovered)
	sort.Strings(extra)
	if len(uncovered) > 0 {
		t.Fatalf("COVERAGE GAP: the live marshaller can emit committed-leaf tag(s) NO guard scenario "+
			"exercises: %v\n  A dropped emission of these would NOT redden the diff-minus-fold guard — the "+
			"exact generator blind spot this meta-assertion exists to close. Add a scenario to "+
			"generateLeafDiffScenarios that drives a block writing each tag.\n  emittable(%d): %v\n  exercised(%d): %v",
			uncovered, len(want), sortedSet(want), len(got), sortedSet(got))
	}
	if len(extra) > 0 {
		t.Fatalf("STALE COVERAGE: guard scenarios exercise tag(s) the live marshaller does NOT emit "+
			"(a removed/renamed committed leaf): %v", extra)
	}
}

func sortedSet(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestLeafDiffCoverageMetaHasTeeth proves the coverage meta-assertion is not decoration: DROP one
// scenario (the class-S slash) and the emittable-vs-exercised comparison MUST report the tags that
// scenario uniquely covered as uncovered. A meta-assertion with no demonstrated red is a comment that
// compiles (the session-7 rule). With the full set wired it passes (asserted in
// TestLeafDiffGuardCoversEveryEmittableTag).
func TestLeafDiffCoverageMetaHasTeeth(t *testing.T) {
	full := generateLeafDiffScenarios(t)

	// Drop the class-S slash scenario. slashed(+Root) are covered ONLY by that scenario, so removing it
	// MUST leave the emittable set strictly larger than the exercised set.
	var reduced []leafDiffScenario
	var dropped bool
	for _, sc := range full {
		if sc.name == "class-S-slash" {
			dropped = true
			continue
		}
		reduced = append(reduced, sc)
	}
	if !dropped {
		t.Fatalf("meta-teeth: class-S-slash scenario not found — generator drifted, update this test")
	}

	want := v5EmittableLeafTags(t)
	got := exercisedLeafDiffTags(t, reduced)

	var uncovered []string
	for tag := range want {
		if _, ok := got[tag]; !ok {
			uncovered = append(uncovered, tag)
		}
	}
	if len(uncovered) == 0 {
		t.Fatalf("META-TEETH FAILED: dropping the class-S scenario left the coverage assertion GREEN — "+
			"the meta-assertion cannot detect a missing scenario, so it is decoration.\n  emittable: %v",
			sortedSet(want))
	}
	// slashed and slashedRoot are the tags the slash scenario uniquely drives; confirm at least one shows.
	sort.Strings(uncovered)
	foundSlashed := false
	for _, tag := range uncovered {
		if tag == tagOfKey(tagSlashed) || tag == tagOfKey(tagSlashedRoot) {
			foundSlashed = true
		}
	}
	if !foundSlashed {
		t.Fatalf("META-TEETH: expected dropping the slash scenario to leave slashed/slashedRoot uncovered, got %v", uncovered)
	}
}

// dropLeafScenario is one class-scoped dropped-emission ablation: a fixture block whose real apply()
// diff includes dropTag, with an honest witness. The REAL recompute ops are assembled, the ops carrying
// dropTag are removed (modelling a dropped emission of that class's leaf), and the PRODUCTION
// assertLeafDiffEqualsFold is driven — it MUST redden naming dropTag.
type dropLeafScenario struct {
	name     string
	pre      *Chain
	prevRoot ports.Hash
	b        Block
	w        StateRootWitness
	dropTag  string // the tagOfKey(...) form
}

// TestLeafDiffNamingPathPerClassSBT exercises the diff-minus-fold NAMING assertion for a representative
// committed-leaf tag of each of class S, B, and T, driving the REAL recompute ops and the PRODUCTION
// assertLeafDiffEqualsFold. For each: assemble the honest folded set from assembleStateRootRecomputeOps
// and confirm it AGREES with the diff (GREEN, the assertion does NOT fatal); then DROP the class's
// representative tag from the folded set and confirm assertLeafDiffEqualsFold reddens naming exactly
// that tag. This reaches the naming assertion directly for S/B/T — the path the AGREE-first completeness
// pre-check would short-circuit on a full break (the Tester's anomaly). Red-before-green, same style as
// the class-M ablation above.
func TestLeafDiffNamingPathPerClassSBT(t *testing.T) {
	for _, tc := range perClassDropScenarios(t) {
		t.Run(tc.name, func(t *testing.T) {
			post := tc.pre.cloneForDryRun()
			post.apply(tc.b)
			committed, err := post.StateRootForVersion(BlockVersionWitnessable)
			if err != nil {
				t.Fatalf("post StateRootForVersion: %v", err)
			}
			diff := committedLeafDiff(tc.pre, post)

			// The dropped tag must genuinely be in the apply() diff (else the ablation is vacuous).
			if !diffHasTag(diff, tc.dropTag) {
				t.Fatalf("ablation vacuous: tag %q not in the apply() diff for scenario %s", tc.dropTag, tc.name)
			}

			// The REAL folded set from the production op pipeline. It must AGREE (fold to committed) and
			// the honest folded==diff, so the PRODUCTION assertion stays GREEN (does NOT fatal).
			if err := tc.pre.RecomputeStateRootEntriesRevocations(tc.prevRoot, committed, tc.b, tc.w); err != nil {
				t.Fatalf("recompute must AGREE for the ablation to be meaningful, got %v", err)
			}
			ops, err := tc.pre.assembleStateRootRecomputeOps(tc.prevRoot, committed, tc.b, tc.w)
			if err != nil {
				t.Fatalf("assembleStateRootRecomputeOps: %v", err)
			}
			folded := foldedChangeKeys(ops)
			if failed, _ := runLeafDiffAssertion(diff, folded); failed {
				t.Fatalf("GREEN pre-check (%s): assertLeafDiffEqualsFold reddened on the honest folded set", tc.name)
			}

			// DROP every folded leaf of the class's representative tag (model a dropped emission), then
			// drive the PRODUCTION assertLeafDiffEqualsFold naming path and confirm it reddens naming that tag.
			droppedFolded := dropTagFromKeySet(folded, tc.dropTag)
			failed, msg := runLeafDiffAssertion(diff, droppedFolded)
			if !failed {
				t.Fatalf("ABLATION FAILED (%s): dropping tag %q stayed GREEN — the naming path did not redden", tc.name, tc.dropTag)
			}
			if !strings.Contains(msg, tc.dropTag) {
				t.Fatalf("ABLATION (%s): expected the guard to redden naming %q, got message: %s", tc.name, tc.dropTag, msg)
			}
		})
	}
}

// perClassDropScenarios returns one dropped-emission scenario per class S/B/T, each keyed on a
// committed-leaf tag that class writes. Each carries the honest witness so the REAL recompute agrees.
func perClassDropScenarios(t *testing.T) []dropLeafScenario {
	t.Helper()
	var out []dropLeafScenario

	// Class S: the slash writes the slashed leaf.
	{
		f := buildSlashFixture(t)
		b := f.slashBlock()
		out = append(out, dropLeafScenario{"class-S-drop-slashed", f.c, f.prevRoot, b, f.witnessForSlash(t, b), tagOfKey(tagSlashed)})
	}
	// Class B: a fresh bond reg writes bondRootOwner.
	{
		f := buildBondFixture(t)
		prev, h := f.c.Head()
		fresh := key(81009)
		b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
			BondRegs: []BondReg{bondRegFull(fresh, ports.HashBytes(pubOf(fresh)), 4<<20, prev, 5, 9)}}
		newDue := h + f.c.cfg.BondTTLBlocks + 1
		out = append(out, dropLeafScenario{"class-B-drop-bondRootOwner", f.c, f.prevRoot, b, f.bondWitness(t, b, []uint64{newDue}), tagOfKey(tagBondRootOwner)})
	}
	// Class T: the sweep expires the member, deleting its bondRegHeight leaf.
	{
		f := buildTTLFixture(t)
		b := f.sweepBlock()
		expired := f.expiredMembers()
		if len(expired) == 0 {
			t.Fatalf("class-T fixture: no expired members")
		}
		out = append(out, dropLeafScenario{"class-T-drop-bondRegHeight", f.c, f.prevRoot, b, f.ttlSweepWitness(t, b, expired), tagOfKey(tagBondRegHeight)})
	}
	return out
}

// diffHasTag reports whether any key in the set has the given tag.
func diffHasTag(set map[string]struct{}, tag string) bool {
	for k := range set {
		if tagOfKey(k) == tag {
			return true
		}
	}
	return false
}

// dropTagFromKeySet returns a copy of in with every key carrying tag removed.
func dropTagFromKeySet(in map[string]struct{}, tag string) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for k := range in {
		if tagOfKey(k) == tag {
			continue
		}
		out[k] = struct{}{}
	}
	return out
}

// fatalCapturingT records the first Fatalf a helper makes and stops it by panicking (unwound by the
// goroutine in runLeafDiffAssertion). It lets the S/B/T ablations drive the PRODUCTION
// assertLeafDiffEqualsFold and assert on its RED without failing the parent test.
type fatalCapturingT struct {
	failed bool
	msg    string
}

func (f *fatalCapturingT) Fatalf(format string, args ...any) {
	f.failed = true
	f.msg = fmt.Sprintf(format, args...)
	panic(errAssertionFatal)
}

var errAssertionFatal = errors.New("leafdiff assertion fatal")

// runLeafDiffAssertion drives the PRODUCTION assertLeafDiffEqualsFold against a fatalCapturingT so a
// dropped-emission ablation exercises the real naming assertion (not a re-implementation) and the
// caller can assert on the RED and its message. It runs on its own goroutine because a captured Fatalf
// panics to unwind; the panic is recovered here.
func runLeafDiffAssertion(diff, folded map[string]struct{}) (failed bool, msg string) {
	captured := &fatalCapturingT{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil && r != errAssertionFatal {
				panic(r)
			}
		}()
		assertLeafDiffEqualsFold(captured, diff, folded)
	}()
	<-done
	return captured.failed, captured.msg
}
