package chain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// =============================================================================
// G-6 — the exported box-door inventory (step 9) · G-4 — every gate carries an honest twin (step 10)
// =============================================================================

// exportedBoxDoors is the EXACT set of exported *Chain methods a floor-box file may declare. Main
// had nine; eight were second doors — a caller could reach one reproduced predicate with none of
// P1–P4 in front of it (N1 is exactly that shape). The box's door is (*Box).Validate, which is not
// a *Chain method and runs the ONE composition. The two that stay:
//   - WitnessValidateV5: the pre-structure never-Accept scaffold (three-parameter under 1A, delta
//     certification §6);
//   - WitnessReadSetV5: the read-set PRODUCER, which expresses no verdict (readset_v5.go).
var exportedBoxDoors = []string{"WitnessReadSetV5", "WitnessValidateV5"}

// boxDoorFiles are the files the inventory is derived over: every non-test floorbox_*.go plus the
// read-set producer's file.
func boxDoorFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("floorbox_*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			out = append(out, f)
		}
	}
	return append(out, "readset_v5.go")
}

// TestG6_ExportedBoxDoorInventory derives the exported *Chain methods declared in the box files
// and asserts they are exactly exportedBoxDoors. Ablation (G-6): declare a tenth exported *Chain
// method in any floorbox_*.go ⇒ RED.
// SOURCE GATE: an AST inventory of FuncDecls. RUNTIME GATE: TestWitnessReadSetV5_LegacyFence (the
// producer's fence) and TestBoxDoor_HonestBlockReachesTheDowngrade (the one real door).
func TestG6_ExportedBoxDoorInventory(t *testing.T) {
	fset := token.NewFileSet()
	var got []string
	for _, f := range boxDoorFiles(t) {
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("SOURCE GATE: parse %s: %v", f, err)
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 || !chainReceiver(fd.Recv.List[0].Type) {
				continue
			}
			if ast.IsExported(fd.Name.Name) {
				got = append(got, fd.Name.Name)
			}
		}
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(exportedBoxDoors, ",") {
		t.Fatalf("SOURCE GATE: G-6 — the box files declare exported *Chain methods %v; exactly %v are permitted.\n"+
			"  A new exported *Chain method in a floorbox_*.go is a SECOND DOOR: a caller can reach one reproduced\n"+
			"  predicate with none of P1–P4 in front of it. Unexport it and route through (*Box).Validate.", got, exportedBoxDoors)
	}
}

// TestWitnessReadSetV5_LegacyFence is the runtime cover of the read-set producer's S2 mode fence: a
// legacy (non-objective) chain yields NO read-set for a v5 block, because a read-set produced under
// rep(id) qualification names keys that mean nothing to a box. An objective chain yields one.
func TestWitnessReadSetV5_LegacyFence(t *testing.T) {
	lf := buildLegacyFixture(t)
	b := lf.mkBlock(t)
	if rs := lf.c.WitnessReadSetV5(b); rs != nil {
		t.Fatalf("the read-set producer must return nil on a legacy chain (the S2 mode fence); got %d entries", len(rs))
	}
	f := buildStructFixture(t)
	if rs := f.c.WitnessReadSetV5(f.mkBlock(t, nil)); len(rs) == 0 {
		t.Fatal("the read-set producer must produce a read-set on an objective chain (the fence must not over-fire)")
	}
}

// twinHelpers are the honest-twin assertions (NG-2). Every Test* in twinGateFiles must call one of
// them DIRECTLY: a gate whose refusal arm is green while its twin fails is refusing for the wrong
// reason, and a box that stalls on everything satisfies box.Accept ⇒ node.Accept trivially.
var twinHelpers = map[string]string{
	"assertHonestTwinAgrees":       "the carrier gates: the box agrees with the node on the honest block, warm and cold",
	"assertHonestTwinAccepts":      "the structure gates: the node and the composition over liveView Accept the honest block",
	"assertBoxReachesTheDowngrade": "the door gates: the honest block runs the composition through the box to the R1.8 downgrade",
	"requireV5Fixture":             "the stage-cover arms: arm D, which calls assertHonestTwinAccepts",
}

// twinGateFiles are the gate files under the NG-2 rule.
var twinGateFiles = []string{
	"redteam_carrier_boxsplit_gate_test.go",
	"redteam_floorbox_structure_gate_test.go",
	"composition_stage_cover_v5_test.go",
	"floorbox_box_v5_test.go",
}

// TestG4_EveryGateCarriesAnHonestTwin counts, per Test* declaration in twinGateFiles, the direct
// calls to a twin helper, and requires at least one each: call sites == gate count, as the brief
// states it. Ablation (G-4): make the box stall unconditionally (the recompute for the carrier
// gates; the composition's step 0b for the structure gates) ⇒ EVERY gate in these files goes RED
// through its twin; then delete one twin call ⇒ this gate goes RED.
// SOURCE GATE: an AST count of call sites. RUNTIME GATE: every twin call in twinGateFiles.
func TestG4_EveryGateCarriesAnHonestTwin(t *testing.T) {
	fset := token.NewFileSet()
	gates := 0
	for _, f := range twinGateFiles {
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("SOURCE GATE: parse %s: %v", f, err)
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || !strings.HasPrefix(fd.Name.Name, "Test") {
				continue
			}
			gates++
			calls := 0
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var name string
				switch fn := call.Fun.(type) {
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				case *ast.Ident:
					name = fn.Name
				}
				if _, ok := twinHelpers[name]; ok {
					calls++
				}
				return true
			})
			if calls == 0 {
				t.Errorf("SOURCE GATE: G-4 — %s (%s) carries NO honest twin: it must call one of %v directly", fd.Name.Name, f, twinHelperNames())
			}
		}
	}
	if gates < 20 {
		t.Fatalf("SOURCE GATE: G-4 VACUOUS — only %d Test* declarations found across %v", gates, twinGateFiles)
	}
}

func twinHelperNames() []string {
	out := make([]string, 0, len(twinHelpers))
	for n := range twinHelpers {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// =============================================================================
// G-6b — the exported PACKAGE-LEVEL surface of the box files (PE ruling F-2, 2026-09-08)
// =============================================================================

// packageSurfaceFiles are the files the wide inventory is derived over: the box files, the
// composition, the views and the read-set producer.
var packageSurfaceFiles = []string{"floorbox_*.go", "validate_v5*.go", "stateview*_v5.go", "readset_v5.go"}

// exportedPackageSurface is the EXACT exported package-level surface of packageSurfaceFiles —
// every exported func, every exported method on an EXPORTED receiver type (a method on an
// unexported receiver such as liveView/provenView is not a surface: the type cannot be named
// outside the package), every exported type, and every exported const — each with the one-line
// reason it is permitted. Exported VARS are excluded by rule: every one is an Err* sentinel, a
// value with no behaviour, and a new sentinel is not a door.
//
// G-6 closed nine exported *Chain methods; the same commit exported ValidateCommitV5 over an
// exported StateView, which was a SECOND door around (*Box).Validate's downgrade until StateView
// was sealed (stateview_v5.go). This inventory is what makes a new exported entry a REVIEWED
// event: it reddens until the entry is listed here with its reason.
var exportedPackageSurface = map[string]string{
	// ---- funcs ----
	"func ValidateProposalV5": "the ONE composition's proposal entry; the chain.go dispatch calls it over liveView. It ACCEPTS an honest block — never-Accept is (*Box).Validate's property. Drivable only by the two sealed views",
	"func ValidateCommitV5":   "the ONE composition's commit entry; the node's dispatch and the box door both call it. Same seal",
	"func NewBox":             "the box constructor: refuses a legacy chain, an unset budget, a pruned or unsigned parent; the head record is derived from the parent block (BG-2)",
	"func ByteBudget":         "the only bounded Budget constructor; refuses a non-positive ceiling (M-4)",
	"func UnlimitedBudget":    "the node's Budget; only liveView returns it (G-D10)",
	// ---- methods on exported receivers ----
	"method Box.Validate":            "THE DOOR: budget → recovery → pruned stall → ValidateCommitV5(provenView) → the one-line R1.8 downgrade",
	"method Box.Head":                "the box's own head record, read-only",
	"method Budget.Check":            "the ONE budget comparison; the zero Budget stalls by name",
	"method Budget.Unlimited":        "accessor",
	"method Budget.IsZero":           "accessor",
	"method Budget.MaxBytes":         "accessor",
	"method FloorBoxOutcome.String":  "rendering",
	"method Availability.String":     "rendering",
	"method Chain.WitnessValidateV5": "the pre-structure never-Accept scaffold (G-6; delta certification §6, three-parameter under 1A)",
	"method Chain.WitnessReadSetV5":  "the read-set PRODUCER, fenced to objective mode; expresses no verdict (G-6)",
	// ---- types ----
	"type Box":               "the box: config-bearing chain, derived head, derived budget, delivery seam",
	"type BoxConfig":         "box-owned operator config: the byte ceiling and the #535 directive",
	"type StateView":         "the composition's read interface — SEALED by an unexported method, so only liveView/provenView implement it",
	"type WitnessSource":     "the witness DELIVERY seam a witness server implements; it returns leaves, never a verdict",
	"type HeadRef":           "the view's own position (M-3)",
	"type Budget":            "the BG-3 byte ceiling; unexported fields, two constructors",
	"type Params":            "the view's own consensus configuration (class 3)",
	"type Availability":      "the three-valued read result; NoWitness is the zero",
	"type FloorBoxOutcome":   "the three-valued verdict",
	"type RecoveryDirective": "the box-local #535 recovery directive",
	// witness carriers: data the box is HANDED, measured by witnessBytes; none expresses a verdict
	"type StateRootWitness":            "witness carrier",
	"type StateRootChangedLeafWitness": "witness carrier",
	"type StateRootAttScreen":          "witness carrier",
	"type StateRootBondRegScreen":      "witness carrier",
	"type StateRootBucketWitness":      "witness carrier",
	"type StateRootDigestWitness":      "witness carrier",
	"type StateRootMaturityWitness":    "witness carrier",
	"type StateRootRotateMember":       "witness carrier",
	"type StateRootRotateScalar":       "witness carrier",
	"type StateRootRotateWitness":      "witness carrier",
	"type StateRootTTLWitness":         "witness carrier",
	"type EpochSetWitness":             "witness carrier",
	"type MemberWeightWitness":         "witness carrier",
	"type MemberStateWitness":          "witness carrier",
	"type SeenSetWitness":              "witness carrier",
	"type SeenSetStreamWitness":        "witness carrier",
	"type BondedSetWitness":            "witness carrier",
	"type QualifiedMemberWitness":      "witness carrier",
	"type QualifiedCountWitness":       "witness carrier",
	// ---- consts ----
	"const Accept":                   "FloorBoxOutcome value",
	"const Reject":                   "FloorBoxOutcome value",
	"const IndeterminateTrustlessly": "FloorBoxOutcome value",
	"const NoWitness":                "Availability value (the zero)",
	"const ProvenAbsent":             "Availability value",
	"const Present":                  "Availability value",
}

// TestG6b_ExportedPackageSurfaceInventory derives the exported package-level surface of
// packageSurfaceFiles by AST and asserts it equals exportedPackageSurface exactly, both ways, with
// a reason on every entry. It also asserts StateView carries its unexported seal method.
// Ablation (G-6b): declare `func ExportedAblationDoor()` in validate_v5.go ⇒ RED naming it; delete
// sealedStateView from the interface ⇒ RED.
// SOURCE GATE: an AST inventory of exported declarations. RUNTIME GATE:
// TestBoxDoor_HonestBlockReachesTheDowngrade (the one door, driven to its downgrade) and
// TestGD12_BothNodeEntryPointsDispatchToTheComposition (the two sealed callers of the composition).
func TestG6b_ExportedPackageSurfaceInventory(t *testing.T) {
	var files []string
	for _, g := range packageSurfaceFiles {
		m, err := filepath.Glob(g)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range m {
			if !strings.HasSuffix(f, "_test.go") {
				files = append(files, f)
			}
		}
	}
	if len(files) < 20 {
		t.Fatalf("SOURCE GATE: G-6b VACUOUS — only %d non-test files matched %v", len(files), packageSurfaceFiles)
	}
	fset := token.NewFileSet()
	got := map[string]string{}
	sealed := false
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("SOURCE GATE: parse %s: %v", f, err)
		}
		for _, d := range af.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if !ast.IsExported(d.Name.Name) {
					continue
				}
				if d.Recv == nil {
					got["func "+d.Name.Name] = f
					continue
				}
				rt := d.Recv.List[0].Type
				if star, ok := rt.(*ast.StarExpr); ok {
					rt = star.X
				}
				if id, ok := rt.(*ast.Ident); ok && ast.IsExported(id.Name) {
					got["method "+id.Name+"."+d.Name.Name] = f
				}
			case *ast.GenDecl:
				for _, sp := range d.Specs {
					switch sp := sp.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(sp.Name.Name) {
							got["type "+sp.Name.Name] = f
						}
						if sp.Name.Name == "StateView" {
							if it, ok := sp.Type.(*ast.InterfaceType); ok {
								for _, m := range it.Methods.List {
									for _, n := range m.Names {
										if n.Name == "sealedStateView" {
											sealed = true
										}
									}
								}
							}
						}
					case *ast.ValueSpec:
						if d.Tok != token.CONST {
							continue // exported vars are Err* sentinels: values, not doors
						}
						for _, n := range sp.Names {
							if ast.IsExported(n.Name) {
								got["const "+n.Name] = f
							}
						}
					}
				}
			}
		}
	}
	if !sealed {
		t.Fatal("SOURCE GATE: G-6b — StateView must carry the unexported seal method sealedStateView, or any package can implement it and take Accept out of ValidateCommitV5 with no downgrade in front (PE F-2)")
	}
	var extra, missing []string
	for k, f := range got {
		if _, ok := exportedPackageSurface[k]; !ok {
			extra = append(extra, k+" ("+f+")")
		}
	}
	for k, why := range exportedPackageSurface {
		if _, ok := got[k]; !ok {
			missing = append(missing, k)
		}
		if why == "" {
			t.Fatalf("SOURCE GATE: G-6b — %s is listed with no reason", k)
		}
	}
	sort.Strings(extra)
	sort.Strings(missing)
	if len(extra) > 0 || len(missing) > 0 {
		t.Fatalf("SOURCE GATE: G-6b — the exported package-level surface of the box files differs from exportedPackageSurface.\n"+
			"  NEW (not listed): %v\n  LISTED but gone: %v\n"+
			"  A new exported func, method, type or const in these files is a new entry point onto the composition or the box.\n"+
			"  Either unexport it, or list it here with the one-line reason it cannot be a second door.", extra, missing)
	}
}
