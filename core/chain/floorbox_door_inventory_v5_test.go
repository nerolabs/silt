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
	gates, twins := 0, 0
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
			twins++
		}
	}
	if gates < 20 {
		t.Fatalf("SOURCE GATE: G-4 VACUOUS — only %d Test* declarations found across %v", gates, twinGateFiles)
	}
	if twins != gates {
		t.Fatalf("SOURCE GATE: G-4 — %d gates, %d with a twin", gates, twins)
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
