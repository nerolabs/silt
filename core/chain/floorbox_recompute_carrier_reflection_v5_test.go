package chain

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
)

// =============================================================================
// R-CARRIER-REFLECTION — the fold-input carrier reflection pin (Boulder 1, owed before R1.8)
// =============================================================================
//
// THE RESIDUAL THIS CLOSES. The R1.4 witness-soundness certification
// (floorbox-R1.3-refutation-R1.4-witness-soundness-RESEARCH-CERTIFICATION-2026-09-01, §Q2 and
// §Residuals) held R-CARRIER-REFLECTION as BOUNDED-BUT-OPEN: `TestAdversarialRootCoverageIsComplete`
// reflects only the VALUE/PREDICATE carriers (StateRootAttScreen, StateRootRotateMember,
// StateRootBondRegScreen, plus StateRootRotateScalar added by R1.6), NOT the FOLD-INPUT carriers.
// The certifier verified BY HAND that the fold-input carriers are all prevStateRoot- or
// committedStateRoot-anchored, and wrote: "It is not pinned: a future field added to a fold-input
// carrier that ALSO decides a branch would escape the walk. Lift = extend the walk to the
// fold-input carriers with an 'already-anchored' classification, so a new value-bearing field on
// them reddens. Owed before the flip."
//
// Hand-verification is not a gate. This file is the gate.
//
// WHAT IS PINNED — the CLOSURE, not a hand list. `reachableCarriers` walks the TRANSITIVE struct
// closure from the state-root fold's witness roots (StateRootWitness, plus SeenSetWitness which is
// also reached through Maturity.SeenSet), descending through pointers / slices / arrays / maps into
// every named struct type declared in package chain. The reachable set is compared for EXACT
// EQUALITY against the declared coverage — the union of `r12CoverageTable` (the R1.2/R1.4
// value/predicate rows, unchanged by this file) and `foldInputCoverageTable` (the fold-input rows
// added here). Three ways to go RED:
//
//	1. a NEW CARRIER TYPE reachable from the fold's witness bundle with no table entry;
//	2. a NEW FIELD on any reachable carrier with no classification row;
//	3. a STALE ROW naming a type or field that no longer exists (renamed / removed).
//
// Because the enumeration is derived by reflection over the real production types, an added or
// renamed carrier cannot slip past the coverage table silently — the failure mode the residual
// names.
//
// TEETH. `TestFoldInputCarrierCoverageHasTeeth` drives the SAME walk (`carrierCoverageGaps`, not a
// re-implementation of it) over an injected violation in each of the three directions and requires
// the walk to report it. A meta-assertion with no demonstrated red is a comment that compiles.
//
// WHAT THIS PIN DOES NOT CLAIM. It pins the COVERAGE OBLIGATION (every carrier field is classified
// with a named anchor), not the anchoring itself. The anchoring is proven by the driven gates: the
// 10 per-field FIX gates + the class-M cross-class gate (adversarialroot_v5_test.go), the
// per-scalar suppression gates (`scalarSuppressObligations`), and the fold's own root equality.
// This pin makes a NEW carrier field impossible to add without stating which of those anchors
// covers it.
//
// SCOPE BOUNDARIES — deliberate and named, not oversights:
//
//   - `statehash.Witness` / `statehash.FoldSibling` are OPAQUE leaves. The walk descends only into
//     structs declared in package chain, so the proof primitives are not enumerated field-by-field.
//     They are the verification primitives (`core/statehash/witness.go` Resolve, `fold.go`
//     FoldChangedPaths), certified in their own package, not floor-box carriers.
//   - The SIBLING recompute witnesses — `EpochSetWitness`, `MemberWeightWitness`,
//     `BondedSetWitness`, `QualifiedCountWitness`, `QualifiedMemberWitness` — are OUT OF SCOPE.
//     They feed the root-only predicate recomputes (requireEpochWeightQuorum,
//     requireDeMatureSuperQuorum, qualifiedCount), not the state-root FOLD, and they are not
//     reachable from StateRootWitness. R-CARRIER-REFLECTION is scoped to the fold-input carriers.
//     If one ever becomes reachable from the fold's witness bundle, this pin reddens until it is
//     classified — the boundary is enforced by the closure, not by trust.
//   - The READ-SET side (`readset_v5.go`) emits `[]statehash.ReadEntry`, not a chain-local carrier
//     struct. Its completeness is pinned by the EXECUTION-DERIVED drift guard
//     (`readset_v5_drift_test.go`), which is the stronger instrument for that surface: it derives
//     ground truth from the real recompute's leaf-touch, not from a table.

// foldInputCoverageTable classifies every field of every FOLD-INPUT carrier reachable from the
// state-root fold's witness bundle: the carriers R1.4 §Q2 verified by hand. It is DISJOINT from
// `r12CoverageTable` (the value/predicate carriers) — `declaredCarrierCoverage` fails if a type
// appears in both, so ownership of a carrier is never ambiguous.
//
// Every row here is `already-anchored` with a SPECIFIC anchor named. That is the point: a new field
// on one of these carriers cannot be waved through, because adding the row forces the author to
// state which anchor covers it — or to classify it FIX and build a driven gate (the R1.2 pattern).
var foldInputCoverageTable = map[string]map[string]r12Disposition{
	// ---- the top-level bundle: each slot's SET-COMPLETENESS is payload-derived, never witness-chosen ----
	"StateRootWitness": {
		"ChangedLeaves":  {"already-anchored", "derived-set: the box derives the changed-key set from the payload and requires a matching verified witness per key; a missing witness stalls the fold (values classified under StateRootChangedLeafWitness)"},
		"DueBucketProof": {"already-anchored", "the TTL scope-gate non-membership proof, Resolved against prevStateRoot; a membership or failed proof stalls the scope gate"},
		"DigestPreSets":  {"already-anchored", "derived-set: touched digests are derived from the payload and matched by Tag; a non-derived tag is ignored (values classified under StateRootDigestWitness)"},
		"TTLSweep":       {"already-anchored", "presence is own-cfg + height gated (BondTTLBlocks, dueBucket[h]), never witness-decided (C-6); values classified under StateRootTTLWitness"},
		"BondRegScreens": {"already-anchored", "derived-set: one screen per payload bond-reg Root; values classified under StateRootBondRegScreen (R1.2 FIX gates)"},
		"BondRegBuckets": {"already-anchored", "derived-set: one entry per payload-derived affected due-height; values classified under StateRootBucketWitness"},
		"AttScreens":     {"already-anchored", "derived-set: one screen per carried non-parent-proposer signer in b.LastCommit — the HASH-COVERED carrier (R-BOX-ATTESTS O1), so the derived set is a pure function of signed content; values classified under StateRootAttScreen (R1.2 FIX gates)"},
		// R-BOX-ATTESTS O1 (2026-09-03). The carrier transition excludes id == parent.ProposerID(),
		// and the parent's proposer identity is NOT a committed leaf, so it cannot be Resolved against
		// prevStateRoot like every other class-A screen input. It is anchored instead by the PARENT'S
		// OWN PROPOSER SIGNATURE over the hash-covered b.Prev. Both fields decide a branch (the skip),
		// so both are FIX with driven gates. The residual the anchor leaves — it proves "this key
		// signed b.Prev", not "this key IS the parent's proposer", so a signer can drop its OWN seat —
		// is R-CARRIER-PARENTPROPOSER, bounded to the downward-only discretion O1 already discloses.
		// See carrierParentProposerFromWitness (carrier.go).
		"ParentProposer":    {"FIX", "TestAdversarialRoot_ClassA_ForgedParentProposer"},
		"ParentProposerSig": {"FIX", "TestAdversarialRoot_ClassA_MissingParentProposerSig"},
		"Rotate":            {"already-anchored", "presence is own-cfg gated (epochsEnabled && h%EpochBlocks==0), never witness-decided (C-6); values classified under StateRootRotateWitness"},
		"Maturity":          {"already-anchored", "REQUIRED on every v5 block — maturityLatchOps STALLS on a nil Maturity, so its presence is not attacker-optional; values classified under StateRootMaturityWitness"},
	},
	// ---- class E/R: the payload changed-leaf carrier ----
	"StateRootChangedLeafWitness": {
		"Key":            {"already-anchored", "derived-key: the box derives the expected key set from the payload; a witness for a non-derived key is ignored (the derived set is authoritative)"},
		"OldValue":       {"already-anchored", "the fold OldValue — FoldChangedPaths verifies the pre-state claim against prevStateRoot before folding; a false claim stalls"},
		"Proof":          {"already-anchored", "the pre-state inclusion/non-membership proof of Key, verified against prevStateRoot by the fold"},
		"DeleteSiblings": {"already-anchored", "delete off-path siblings, anchored by the fold's final root equality"},
	},
	// ---- class S: the whole-set digest pre-set carrier ----
	"StateRootDigestWitness": {
		"Tag":    {"already-anchored", "derived-key: the touched-digest tag set is derived from the payload; a witness for a non-derived tag is ignored"},
		"PreIDs": {"already-anchored", "completeness-anchored: nodeSetMTH(PreIDs) must equal the committed pre-digest, which is itself the FoldOp OldValue verified against prevStateRoot; a short or padded id-list stalls"},
		"Proof":  {"already-anchored", "the digest leaf inclusion proof, routed as the FoldOp OldValue and verified against prevStateRoot (R-anchor-prevroot)"},
	},
	// ---- class T: the TTL-sweep accelerator carrier ----
	"StateRootTTLWitness": {
		"Height":               {"already-anchored", "derived-key: the sweep height is b.Height from the block header, not a witness choice"},
		"Members":              {"already-anchored", "completeness-anchored (the CRUX): dueBucketMTH(Members) must equal the committed bucket MTH carried as the FoldOp OldValue; a short or padded expired set stalls"},
		"BucketProof":          {"already-anchored", "the dueBucket leaf inclusion proof, routed as the bucket FoldOp OldValue and verified against prevStateRoot"},
		"BucketDeleteSiblings": {"already-anchored", "bucket-delete off-path siblings, anchored by the fold's final root equality"},
	},
	// ---- class B: the per-due-height bucket carrier ----
	"StateRootBucketWitness": {
		"DueHeight":      {"already-anchored", "derived-key: affected due-heights are derived from the payload's bond regs plus own-cfg BondTTLBlocks (C-6), never witness-chosen"},
		"PreMembers":     {"already-anchored", "completeness-anchored: dueBucketMTH(PreMembers) must equal the committed bucket MTH, or the bucket is proven absent; a short or padded pre-set stalls"},
		"Proof":          {"already-anchored", "the bucket leaf inclusion or non-membership proof against prevStateRoot"},
		"DeleteSiblings": {"already-anchored", "bucket-emptying delete off-path siblings, anchored by the fold's final root equality"},
	},
	// ---- class P: the epoch-boundary bundle (its MEMBER carrier is r12-classified; its SCALARS are split-classified) ----
	"StateRootRotateWitness": {
		"Members":       {"already-anchored", "cross-checked against the entry-threaded post-qualified id-set (the anchored pre-qualified set plus this block's S/B/T deltas); a short or padded member list mismatches the derived set and stalls (per-member values: StateRootRotateMember, R1.2 FIX gates)"},
		"PriorEpochSet": {"already-anchored", "per-leaving-member epochSet DELETE proofs against prevStateRoot; per-member values classified under StateRootRotateMember"},
		"EpochStart":    {"already-anchored", "class-P scalar, EMIT-ANCHORED: scalarSuppressObligations[tagEpochStart] (height strictly advances, so the emit always fires and the fold verifies OldValue); fields under StateRootRotateScalar"},
		"MatureEpoch":   {"already-anchored", "class-P scalar, SUPPRESS-ANCHORED: scalarSuppressObligations[tagMatureEpoch] (Direction A pre-state anchor plus a driven suppression gate); fields under StateRootRotateScalar"},
		"GateLockedIn":  {"already-anchored", "class-P scalar, SUPPRESS-ANCHORED: scalarSuppressObligations[tagGateLockedIn]; fields under StateRootRotateScalar"},
		"GateHeight":    {"already-anchored", "class-P scalar riding its lock-in bool (classP-anchoring cert 2026-09-02 §1b): suppressing GateLockedIn suppresses the pair, so anchoring the bool closes it; keeps its emit-time fold anchor"},
		"Era3LockedIn":  {"already-anchored", "class-P scalar, SUPPRESS-ANCHORED: scalarSuppressObligations[tagEra3LockedIn]; fields under StateRootRotateScalar"},
		"Era3Height":    {"already-anchored", "class-P scalar riding Era3LockedIn (cert §1b); keeps its emit-time fold anchor"},
		"Era4LockedIn":  {"already-anchored", "class-P scalar, SUPPRESS-ANCHORED: scalarSuppressObligations[tagEra4LockedIn]; fields under StateRootRotateScalar"},
		"Era4Height":    {"already-anchored", "class-P scalar riding Era4LockedIn (cert §1b); keeps its emit-time fold anchor"},
	},
	// ---- class M: the everMature latch bundle ----
	"StateRootMaturityWitness": {
		"EverMature": {"already-anchored", "class-M scalar, SUPPRESS-ANCHORED: scalarSuppressObligations[tagEverMature] (Direction A pre-state anchor against prevStateRoot plus a driven suppression gate); fields under StateRootRotateScalar"},
		// R-FOLD-LIVE-STATE-READS (cert 2026-09-02, Q3 step 1): the class-A screen's BRANCH SELECTOR.
		// Homed on this carrier because it is REQUIRED on every block, while the class-P rotate witness
		// is nil off-boundary. Anchored UNCONDITIONALLY by handoffPreState before any class dispatches.
		"MatureEpoch": {"already-anchored", "class-M/A scalar, SUPPRESS-ANCHORED: scalarSuppressObligations[tagMatureEpoch] (Direction A pre-state anchor in handoffPreState against prevStateRoot, plus the driven both-polarity gate TestColdBox_D1_ForgedMatureEpochOldValueStalls); fields under StateRootRotateScalar"},
		"SeenSet":     {"already-anchored", "the maturity witness RecomputeMatureNow verifies against committedStateRoot; read only when the pre-latch everMature is false, and a missing/forged one stalls (values classified under SeenSetWitness)"},
	},
	// ---- class M: the validatorsSeen set witness ----
	"SeenSetWitness": {
		"IDs":             {"already-anchored", "completeness-anchored: nodeSetMTH(IDs) must equal the committed validatorsSeenRoot; a member omitted or injected yields a different MTH and stalls"},
		"SeenRootWitness": {"already-anchored", "the validatorsSeenRoot leaf inclusion proof, Resolved against committedStateRoot"},
		"SeenRootValue":   {"already-anchored", "the value SeenRootWitness proves (Resolve-anchored); the comparand nodeSetMTH(IDs) must equal"},
		"Members":         {"already-anchored", "every id in IDs must have an entry or the recompute stalls; per-member values classified under MemberStateWitness (C-1 Resolve-anchored)"},
	},
	// ---- class M: the per-member committed-state witness (C-1: every field Resolve-anchored) ----
	"MemberStateWitness": {
		"Bonded":        {"already-anchored", "C-1 Resolve-anchored: the bonded[id] leaf must prove EncodeInt64(Bonded) against committedStateRoot; a forged weight fails to verify and the member is unproven"},
		"BondedProof":   {"already-anchored", "the anchoring proof for Bonded (verified via Resolve)"},
		"Domain":        {"already-anchored", "C-1 Resolve-anchored: bondDomain[id] must prove EncodeUint64(Domain) when DomainPresent; a forged domain fails to verify"},
		"DomainPresent": {"already-anchored", "C-1 Resolve-anchored: selects inclusion vs non-inclusion for DomainProof — claiming present-when-absent or absent-when-committed fails to verify"},
		"DomainProof":   {"already-anchored", "the anchoring proof for Domain/DomainPresent (verified via Resolve)"},
		"Slashed":       {"already-anchored", "C-1 Resolve-anchored: the slashed[id] bit is proven either way (inclusion or non-inclusion), so a slashed member cannot be silently dropped to shrink the tally"},
		"SlashedProof":  {"already-anchored", "the anchoring proof for Slashed (verified via Resolve)"},
	},
}

// carrierClosureRoots returns the roots of the fold-input carrier closure: the witness bundle the
// state-root fold consumes, plus the class-M maturity set witness (also reached through
// StateRootWitness.Maturity.SeenSet — listed explicitly so the closure does not silently narrow if
// the class-M homing changes).
func carrierClosureRoots() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf(StateRootWitness{}),
		reflect.TypeOf(SeenSetWitness{}),
	}
}

// reachableCarriers walks the transitive struct closure from roots, descending through pointers,
// slices, arrays and maps (both key and element), and collects every NAMED struct type declared in
// package chain. External types (statehash.Witness, ports.NodeID, ...) are opaque leaves — see the
// SCOPE BOUNDARIES note above. The returned map is keyed by type name, which is what the coverage
// tables key on.
func reachableCarriers(roots []reflect.Type) map[string]reflect.Type {
	pkg := reflect.TypeOf(StateRootWitness{}).PkgPath()
	out := map[string]reflect.Type{}
	var visit func(reflect.Type)
	visit = func(ty reflect.Type) {
		// Unwrap containers down to the underlying element type; visit map keys too (a carrier
		// used as a map key would otherwise escape).
		for {
			switch ty.Kind() {
			case reflect.Ptr, reflect.Slice, reflect.Array:
				ty = ty.Elem()
			case reflect.Map:
				visit(ty.Key())
				ty = ty.Elem()
			default:
				goto unwrapped
			}
		}
	unwrapped:
		if ty.Kind() != reflect.Struct || ty.Name() == "" || ty.PkgPath() != pkg {
			return
		}
		if _, dup := out[ty.Name()]; dup {
			return // already walked (also breaks any recursive carrier cycle)
		}
		out[ty.Name()] = ty
		for i := 0; i < ty.NumField(); i++ {
			visit(ty.Field(i).Type)
		}
	}
	for _, r := range roots {
		visit(r)
	}
	return out
}

// declaredCarrierCoverage merges the two coverage tables into the single declared set the closure is
// compared against. A type declared in BOTH tables is itself a failure: ownership of a carrier must
// be unambiguous, else a field could be "classified" in one table and stale in the other.
func declaredCarrierCoverage(t *testing.T) map[string]map[string]r12Disposition {
	t.Helper()
	merged := map[string]map[string]r12Disposition{}
	for name, rows := range r12CoverageTable {
		merged[name] = rows
	}
	for name, rows := range foldInputCoverageTable {
		if _, dup := merged[name]; dup {
			t.Fatalf("AMBIGUOUS COVERAGE: carrier %s is declared in BOTH r12CoverageTable and "+
				"foldInputCoverageTable. Each carrier has exactly one owning table — a field classified "+
				"in one and stale in the other is a silent hole.", name)
		}
		merged[name] = rows
	}
	return merged
}

// validCarrierDispositions is the closed vocabulary a coverage row may use. It matches the
// dispositions TestAdversarialRootCoverageIsComplete accepts, so the two walks cannot diverge on
// what counts as classified.
var validCarrierDispositions = map[string]struct{}{
	"FIX":              {},
	"FIX-OPEN":         {},
	"SUPPRESS-SPLIT":   {},
	"already-anchored": {},
}

// carrierCoverageGaps is THE walk. It reports every way the declared coverage and the reflected
// carrier closure disagree. Both the pin (TestFoldInputCarrierCoverageIsComplete) and its teeth
// (TestFoldInputCarrierCoverageHasTeeth) call THIS function — the teeth exercise the real detector,
// not a re-implementation of it, so a walk that stops detecting cannot pass its own teeth test.
//
// Returned gaps are sorted so failures are stable and diffable.
func carrierCoverageGaps(reachable map[string]reflect.Type, declared map[string]map[string]r12Disposition) []string {
	var gaps []string

	for name, ty := range reachable {
		rows, ok := declared[name]
		if !ok {
			gaps = append(gaps, fmt.Sprintf("UNCLASSIFIED CARRIER: %s is reachable from the state-root fold's witness bundle "+
				"but has no coverage table entry. Classify every field of it in foldInputCoverageTable "+
				"(already-anchored, with the specific anchor named) or in r12CoverageTable (FIX, with a driven gate).", name))
			continue
		}
		seen := map[string]struct{}{}
		for i := 0; i < ty.NumField(); i++ {
			f := ty.Field(i)
			seen[f.Name] = struct{}{}
			disp, classified := rows[f.Name]
			if !classified {
				gaps = append(gaps, fmt.Sprintf("UNCLASSIFIED FIELD: %s.%s is a fold-input carrier field with no coverage row. "+
					"State which anchor covers it (fold OldValue against prevStateRoot / Resolve / payload-derived set), "+
					"or classify it FIX and build a driven adversarial-root gate.", name, f.Name))
				continue
			}
			if _, valid := validCarrierDispositions[disp.kind]; !valid {
				gaps = append(gaps, fmt.Sprintf("INVALID DISPOSITION: %s.%s has kind %q (want one of FIX, FIX-OPEN, SUPPRESS-SPLIT, already-anchored)", name, f.Name, disp.kind))
			}
			if disp.detail == "" {
				gaps = append(gaps, fmt.Sprintf("EMPTY DETAIL: %s.%s classified %q with no anchor named — a classification with no reason is not coverage", name, f.Name, disp.kind))
			}
		}
		for field := range rows {
			if _, ok := seen[field]; !ok {
				gaps = append(gaps, fmt.Sprintf("STALE ROW: the coverage table lists %s.%s but the struct has no such field (renamed or removed?)", name, field))
			}
		}
	}

	for name := range declared {
		if _, ok := reachable[name]; !ok {
			gaps = append(gaps, fmt.Sprintf("STALE CARRIER: the coverage table lists %s but it is not reachable from the "+
				"state-root fold's witness bundle (renamed, removed, or unwired?). Remove the entry, or re-root the closure.", name))
		}
	}

	sort.Strings(gaps)
	return gaps
}

// TestFoldInputCarrierCoverageIsComplete is the R-CARRIER-REFLECTION pin: the reflected transitive
// carrier closure of the state-root fold's witness bundle EQUALS the declared coverage set, field
// for field. See the file header for what this does and does not claim.
func TestFoldInputCarrierCoverageIsComplete(t *testing.T) {
	reachable := reachableCarriers(carrierClosureRoots())
	declared := declaredCarrierCoverage(t)

	if gaps := carrierCoverageGaps(reachable, declared); len(gaps) != 0 {
		msg := fmt.Sprintf("R-CARRIER-REFLECTION: %d coverage gap(s) between the reflected fold-input carrier closure "+
			"and the declared coverage table:\n", len(gaps))
		for _, g := range gaps {
			msg += "  - " + g + "\n"
		}
		t.Fatal(msg)
	}

	// Report the enumeration so the numbers are visible in the run record (the cert's carrier count
	// is a DOC count; the reflection is authoritative — R1.4 §Q2).
	fields := 0
	names := make([]string, 0, len(reachable))
	for name, ty := range reachable {
		fields += ty.NumField()
		names = append(names, name)
	}
	sort.Strings(names)
	t.Logf("fold-input carrier closure: %d carrier types / %d classified fields\n  %v", len(reachable), fields, names)
}

// TestFoldInputCarrierCoverageHasTeeth drives the SAME walk over an injected violation in each of
// the three directions the pin claims to catch, and requires the walk to report it. Direction 1
// synthesizes a REAL new struct field with reflect.StructOf (not a table edit), so it exercises the
// reflection path itself, which is the path a future carrier change actually takes.
func TestFoldInputCarrierCoverageHasTeeth(t *testing.T) {
	baseReachable := reachableCarriers(carrierClosureRoots())
	baseDeclared := declaredCarrierCoverage(t)
	if gaps := carrierCoverageGaps(baseReachable, baseDeclared); len(gaps) != 0 {
		t.Fatalf("TEETH SETUP: the un-injected walk must be clean, got %v", gaps)
	}

	// --- Direction 1: a NEW FIELD on an existing carrier, with no coverage row. ---
	// Synthesize StateRootDigestWitness + an unclassified field via reflect.StructOf and swap it
	// into the reachable set under the same name.
	orig := reflect.TypeOf(StateRootDigestWitness{})
	fields := make([]reflect.StructField, 0, orig.NumField()+1)
	for i := 0; i < orig.NumField(); i++ {
		fields = append(fields, orig.Field(i))
	}
	fields = append(fields, reflect.StructField{Name: "ForgedNewCarrierField", Type: reflect.TypeOf(int64(0))})
	withNewField := reflect.StructOf(fields)

	mutated := map[string]reflect.Type{}
	for k, v := range baseReachable {
		mutated[k] = v
	}
	mutated["StateRootDigestWitness"] = withNewField
	gaps := carrierCoverageGaps(mutated, baseDeclared)
	if !gapsContain(gaps, "UNCLASSIFIED FIELD: StateRootDigestWitness.ForgedNewCarrierField") {
		t.Fatalf("TEETH FAILED (new field): adding an unclassified field to a fold-input carrier must be reported.\n"+
			"  If this does not redden, a future added carrier field slips the coverage table silently — the exact\n"+
			"  R-CARRIER-REFLECTION failure mode. gaps=%v", gaps)
	}

	// --- Direction 2: a STALE ROW — a covered carrier's field removed from the struct. ---
	// Synthesize StateRootTTLWitness with its Members field dropped; the table row for Members must
	// be reported stale.
	origTTL := reflect.TypeOf(StateRootTTLWitness{})
	kept := make([]reflect.StructField, 0, origTTL.NumField())
	for i := 0; i < origTTL.NumField(); i++ {
		if origTTL.Field(i).Name == "Members" {
			continue
		}
		kept = append(kept, origTTL.Field(i))
	}
	if len(kept) != origTTL.NumField()-1 {
		t.Fatalf("TEETH SETUP: StateRootTTLWitness.Members should have been dropped exactly once")
	}
	mutated = map[string]reflect.Type{}
	for k, v := range baseReachable {
		mutated[k] = v
	}
	mutated["StateRootTTLWitness"] = reflect.StructOf(kept)
	gaps = carrierCoverageGaps(mutated, baseDeclared)
	if !gapsContain(gaps, "STALE ROW: the coverage table lists StateRootTTLWitness.Members") {
		t.Fatalf("TEETH FAILED (stale row): a coverage row for a removed field must be reported stale.\n"+
			"  If this does not redden, a renamed carrier field leaves a row that certifies a field that no longer\n"+
			"  exists, while the real (renamed) field is unclassified. gaps=%v", gaps)
	}

	// --- Direction 3: a WHOLE CARRIER TYPE with no table entry. ---
	// Drop the SeenSetWitness classification (simulating a newly added carrier type) and require the
	// walk to report the carrier unclassified.
	reducedDeclared := map[string]map[string]r12Disposition{}
	for k, v := range baseDeclared {
		if k == "SeenSetWitness" {
			continue
		}
		reducedDeclared[k] = v
	}
	gaps = carrierCoverageGaps(baseReachable, reducedDeclared)
	if !gapsContain(gaps, "UNCLASSIFIED CARRIER: SeenSetWitness") {
		t.Fatalf("TEETH FAILED (new carrier type): a reachable carrier with no table entry must be reported.\n"+
			"  If this does not redden, a whole new carrier type can be wired into the fold's witness bundle with\n"+
			"  zero coverage. gaps=%v", gaps)
	}

	// --- Direction 3b: the reverse — a table entry for a carrier no longer reachable. ---
	reducedReachable := map[string]reflect.Type{}
	for k, v := range baseReachable {
		if k == "StateRootBucketWitness" {
			continue
		}
		reducedReachable[k] = v
	}
	gaps = carrierCoverageGaps(reducedReachable, baseDeclared)
	if !gapsContain(gaps, "STALE CARRIER: the coverage table lists StateRootBucketWitness") {
		t.Fatalf("TEETH FAILED (stale carrier): a table entry for an unreachable carrier must be reported.\n"+
			"  If this does not redden, an unwired carrier keeps a coverage row that certifies nothing. gaps=%v", gaps)
	}

	// --- Direction 4: an empty anchor detail is not coverage. ---
	blankDeclared := map[string]map[string]r12Disposition{}
	for k, v := range baseDeclared {
		rows := map[string]r12Disposition{}
		for f, d := range v {
			if k == "MemberStateWitness" && f == "Bonded" {
				d.detail = ""
			}
			rows[f] = d
		}
		blankDeclared[k] = rows
	}
	gaps = carrierCoverageGaps(baseReachable, blankDeclared)
	if !gapsContain(gaps, "EMPTY DETAIL: MemberStateWitness.Bonded") {
		t.Fatalf("TEETH FAILED (empty detail): a row with no anchor named must be reported — a classification with\n"+
			"  no reason is a comment, not coverage. gaps=%v", gaps)
	}
}

// gapsContain reports whether any gap string starts with prefix.
func gapsContain(gaps []string, prefix string) bool {
	for _, g := range gaps {
		if len(g) >= len(prefix) && g[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
