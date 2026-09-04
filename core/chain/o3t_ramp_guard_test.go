package chain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// =============================================================================
// O3 DIRECTION T — GATE (c): R-FORKCHOICE-RAMP-GUARD, second half
// =============================================================================
//
// Binding spec: O3-Direction-T-I5-restatement-and-divergence-RESEARCH-CERTIFICATION-2026-09-04.md
// §8.3. ROADMAP marked R-FORKCHOICE-RAMP-GUARD DONE after PR #716 deleted the `Weight() > 0`
// assertion at forkchoice_ramp357_test.go:62-67 — but its TWIN at modelcheck_i5_357_test.go:92
// (`if c.Weight() <= 0`) is still live, in the model-check tier that gates every graded field run.
// The cert's pass condition: with Weight() deleted, NO file in the tree references the retired
// term, or the deletion leaves a dangling site (a compile break in test code the Builder may not
// run, or a call kept alive by a fixture nobody reads).
//
// This gate FAILS while any Go file under core/, cmd/, sim/ or e2e/ (test or non-test) references
// the retired surface IN CODE:
//
//	<expr>.Weight()          — the method call (a zero-arg call named Weight; the field `Weight`
//	                           on statehash/floorbox member structs is NOT the chain term)
//	blockWeight, anchorWeight — unique identifiers anywhere
//	AnchorWeight             — the Config field: selector or composite-literal key
//
// Comments and string literals are NOT failures (forkchoice_ramp357_test.go's deletion comment
// legitimately names the deleted assertion); they are LOGGED as an inventory so the Builder can
// true them up. RED at 59509b1 (chain.go definitions + 9 test call sites); GREEN after T.
//
// SOURCE GATE: this test parses Go source under the repo's core/, cmd/, sim/, e2e/ roots. It sees
// identifiers, not behaviour.
// RUNTIME GATE: TestO3T_CertificateVariantNeverRanksHeavier (fork-choice ranks by height/hash only).

var o3tRetiredIdents = map[string]bool{"blockWeight": true, "anchorWeight": true, "AnchorWeight": true}

type o3tRetiredHit struct {
	File string
	Line int
	What string
}

// o3tWalkRetiredWeightRefs returns the CODE references to the retired surface in one parsed file,
// and the comment/string mentions separately. Shared with the teeth test.
func o3tWalkRetiredWeightRefs(fset *token.FileSet, rel string, af *ast.File, src []byte) (code, prose []o3tRetiredHit) {
	ast.Inspect(af, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Weight" && len(x.Args) == 0 {
				code = append(code, o3tRetiredHit{rel, fset.Position(x.Pos()).Line, ".Weight() call"})
			}
		case *ast.FuncDecl:
			if x.Name.Name == "Weight" && x.Recv != nil && len(x.Recv.List) == 1 && chainReceiver(x.Recv.List[0].Type) {
				code = append(code, o3tRetiredHit{rel, fset.Position(x.Pos()).Line, "func (c *Chain) Weight declared"})
			}
		case *ast.Ident:
			if o3tRetiredIdents[x.Name] {
				code = append(code, o3tRetiredHit{rel, fset.Position(x.Pos()).Line, x.Name})
			}
		}
		return true
	})
	for _, cg := range af.Comments {
		for _, c := range cg.List {
			for _, w := range []string{"Weight()", "blockWeight", "anchorWeight", "AnchorWeight"} {
				if strings.Contains(c.Text, w) {
					prose = append(prose, o3tRetiredHit{rel, fset.Position(c.Pos()).Line, "comment mentions " + w})
					break
				}
			}
		}
	}
	// String literals: cheap scan over the source for the unique names (a comment-free literal).
	ast.Inspect(af, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			for _, w := range []string{"Weight()", "blockWeight", "anchorWeight", "AnchorWeight"} {
				if strings.Contains(lit.Value, w) {
					prose = append(prose, o3tRetiredHit{rel, fset.Position(lit.Pos()).Line, "string literal mentions " + w})
					break
				}
			}
		}
		return true
	})
	_ = src
	return code, prose
}

// TestO3T_NoWeightTermReferenceSurvives is gate (c).
func TestO3T_NoWeightTermReferenceSurvives(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, sErr := os.Stat(filepath.Join(root, "go.mod")); sErr != nil {
		t.Fatalf("SOURCE GATE: repo root not found at %s (no go.mod): %v", root, sErr)
	}
	self, _ := filepath.Abs("o3t_ramp_guard_test.go")
	fset := token.NewFileSet()
	var code, prose []o3tRetiredHit
	files := 0
	for _, top := range []string{"core", "cmd", "sim", "e2e"} {
		dir := filepath.Join(root, top)
		if _, sErr := os.Stat(dir); sErr != nil {
			continue
		}
		wErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "testdata", "vendor", ".git", ".claude":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || path == self {
				return nil
			}
			src, rErr := os.ReadFile(path)
			if rErr != nil {
				return rErr
			}
			af, pErr := parser.ParseFile(fset, path, src, parser.ParseComments)
			if pErr != nil {
				return pErr
			}
			files++
			rel, _ := filepath.Rel(root, path)
			c, p := o3tWalkRetiredWeightRefs(fset, rel, af, src)
			code = append(code, c...)
			prose = append(prose, p...)
			return nil
		})
		if wErr != nil {
			t.Fatalf("walk %s: %v", dir, wErr)
		}
	}
	if files < 100 {
		t.Fatalf("SOURCE GATE: GATE VACUOUS — only %d Go files walked under core/ cmd/ sim/ e2e/", files)
	}
	sortHits := func(h []o3tRetiredHit) {
		sort.Slice(h, func(i, j int) bool {
			if h[i].File != h[j].File {
				return h[i].File < h[j].File
			}
			return h[i].Line < h[j].Line
		})
	}
	sortHits(code)
	sortHits(prose)
	fmtHits := func(h []o3tRetiredHit) string {
		var out []string
		for _, x := range h {
			out = append(out, "  "+x.File+":"+itoa(x.Line)+"  "+x.What)
		}
		return strings.Join(out, "\n")
	}
	if len(prose) > 0 {
		t.Logf("advisory — %d comment/string mention(s) of the retired surface (NOT failures; true up the prose):\n%s", len(prose), fmtHits(prose))
	}
	if len(code) > 0 {
		t.Fatalf("SOURCE GATE: R-FORKCHOICE-RAMP-GUARD — %d CODE reference(s) to the retired fork-choice weight surface "+
			"(Weight() / blockWeight / anchorWeight / Config.AnchorWeight) across %d files:\n%s\n\n"+
			"  O3 Direction T deletes the term. Every reference goes with it — the cert §8.3 dispositions:\n"+
			"  re-ground on the preserved property (height -> head-hash; r1.Head() == r2.Head()), retire with the\n"+
			"  term, and NEVER re-ground onto an equal-height head-hash tiebreak.", len(code), files, fmtHits(code))
	}
}

// TestO3T_RampGuardHasTeeth proves the walk bites on each of the four shapes and does NOT bite on
// the statehash member field `Weight` (a field, not the chain method).
func TestO3T_RampGuardHasTeeth(t *testing.T) {
	const injected = `package chain

// a comment naming Weight() is prose, not code
func (c *Chain) Weight() int64 { return c.blockWeight(nil) + c.anchorWeight() }

func use(c *Chain, m struct{ Weight int64 }) int64 {
	cfg := Config{AnchorWeight: 1}
	_ = cfg.AnchorWeight
	_ = "blockWeight in a string"
	return c.Weight() + m.Weight
}
`
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, "x.go", injected, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	code, prose := o3tWalkRetiredWeightRefs(fset, "x.go", af, []byte(injected))
	var whats []string
	for _, h := range code {
		whats = append(whats, h.What)
	}
	joined := strings.Join(whats, ",")
	for _, want := range []string{"func (c *Chain) Weight declared", "blockWeight", "anchorWeight", "AnchorWeight", ".Weight() call"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("SOURCE GATE: GATE HAS NO TEETH — %q not flagged in the synthetic file; flagged: %s", want, joined)
		}
	}
	// m.Weight is a FIELD read (no call): must not be flagged. Exactly one .Weight() call in the file.
	calls := 0
	for _, h := range code {
		if h.What == ".Weight() call" {
			calls++
		}
	}
	if calls != 1 {
		t.Fatalf("SOURCE GATE: GATE OVER-BITES — %d .Weight() call(s) flagged, want 1 (m.Weight is a field, not the chain method)", calls)
	}
	if len(prose) != 2 {
		t.Fatalf("SOURCE GATE: prose inventory wrong — %d mention(s), want 2 (one comment, one string)", len(prose))
	}
}
