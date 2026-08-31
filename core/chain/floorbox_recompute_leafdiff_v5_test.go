package chain

import (
	"bytes"
	"sort"
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
func assertLeafDiffEqualsFold(t *testing.T, diff, folded map[string]struct{}) {
	t.Helper()
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

	return out
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
