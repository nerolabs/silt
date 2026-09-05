package credit

// R2.9a RE-CERT — M-2, closed at the package boundary.
//
// From RESEARCH CERTIFICATION
// R2.9a-minR-floor-RECERT-sybil-pad-and-estimand-steerability (2026-09-05) §5.4.
//
// WHAT WENT WRONG BEFORE. The reviewed build kept the raw census exported and pinned
// "nothing outside core/credit calls it" with a test that read TWO HARD-CODED FILE PATHS
// while its failure text claimed a whole-tree property. A reviewer ablated past it in
// five lines by adding a second unfloored export to a third file that already held a live
// ledger: build clean, gate green, source-gate lint green.
//
// WHY WALKING THE TREE WOULD NOT HAVE CLOSED IT. The consuming seam is DUCK-TYPED —
// core/node asserts an anonymous interface on a METHOD NAME — so any type carrying that
// method satisfies it, and a name-based gate has to exclude core/credit (where the method
// lives), which is exactly where a second exported reader would be invisible.
//
// SO THE CLOSE IS THE TYPE SYSTEM: bBootstrapSnapshot is unexported, and BBootstrapPublish
// (which floors) is the only exported route. This gate closes the one hole the compiler
// cannot: a SECOND exported reader added inside this package. It parses EVERY non-test
// .go file of core/credit and requires that the only exported functions returning a
// BBootstrapHistogram are the sanctioned two. Unlike its predecessor it checks exactly
// the scope its failure text names — one package, every file in it, found by reading the
// directory rather than by listing paths.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// r29aSanctionedHistogramExports are the ONLY exported functions in core/credit allowed
// to return a BBootstrapHistogram.
//
//   - BBootstrapPublish is the floored route out of the package.
//   - WithMinRequesterFloor takes a histogram and returns one; it can only ever FLOOR, so
//     it cannot be used to obtain a census a caller did not already hold.
//
// Adding a name here is a privacy decision and has to be argued as one.
var r29aSanctionedHistogramExports = map[string]bool{
	"BBootstrapPublish":     true,
	"WithMinRequesterFloor": true,
}

// TestR29aBBootstrapHasOneExportedRoute is the package-scope source gate behind the
// minimum-requester floor's structural claim (G-BB-11′ / M-2).
//
// RUNTIME GATE: cmd/silt's TestR29aBB20BelowFloorBlockIsAFunctionOfTheClockAlone observes
// the resulting property on the published bytes. This gate closes only the "is there a
// second door out of this package" question, which no runtime test can answer, and it
// closes it for the histogram TYPE — a future export returning census-derived scalars of
// some other type is the stated residual R-BB-EXPORT-SCALAR-BYPASS, not covered here.
func TestR29aBBootstrapHasOneExportedRoute(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	scanned := 0
	var offenders []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Type.Results == nil || !fn.Name.IsExported() {
				continue
			}
			if r29aSanctionedHistogramExports[fn.Name.Name] {
				continue
			}
			for _, res := range fn.Type.Results.List {
				if !r29aMentionsHistogram(res.Type) {
					continue
				}
				offenders = append(offenders, name+":"+fn.Name.Name)
			}
		}
	}
	// The gate must have read something, or a rename of the package directory would
	// silently make it vacuous.
	if scanned < 3 {
		t.Fatalf("SOURCE GATE: scanned only %d non-test .go files in core/credit — the directory walk found nothing to check and this gate is vacuous", scanned)
	}
	if len(offenders) > 0 {
		t.Fatalf("SOURCE GATE: %v are exported function(s) of core/credit returning a BBootstrapHistogram, and they are not in the sanctioned set %v. The RAW census must not leave this package: bBootstrapSnapshot is unexported and BBootstrapPublish is the one route out, because it applies the minimum-requester floor (G-BB-11'). A second exported reader here would be invisible to any name-based gate in another package, since the consuming seam in core/node is duck-typed on the method name. If this export is intended, it must floor",
			offenders, r29aSanctionedHistogramExports)
	}
}

// r29aMentionsHistogram reports whether a result type is, points to, or contains a
// BBootstrapHistogram — so a pointer, slice or map return is caught too.
func r29aMentionsHistogram(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "BBootstrapHistogram" {
			found = true
		}
		return !found
	})
	return found
}

// TestR29aExportRouteGateHasTeeth is the positive control: the same predicate, run over a
// synthetic declaration of the exact shape the reviewer used to ablate the old gate. If
// the walk's predicate could not flag it, the gate above would be decoration.
//
// RUNTIME GATE: TestR29aBBootstrapHasOneExportedRoute.
func TestR29aExportRouteGateHasTeeth(t *testing.T) {
	const bypass = `package credit
func (l *Ledger) Census() BBootstrapHistogram { return l.bBootstrapSnapshot() }
func (l *Ledger) CensusPtr() *BBootstrapHistogram { return nil }
func (l *Ledger) unexportedIsFine() BBootstrapHistogram { return BBootstrapHistogram{} }
func (l *Ledger) BBootstrapPublishLookalike() int { return 0 }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bypass.go", bypass, 0)
	if err != nil {
		t.Fatal(err)
	}
	var flagged []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Type.Results == nil || !fn.Name.IsExported() || r29aSanctionedHistogramExports[fn.Name.Name] {
			continue
		}
		for _, res := range fn.Type.Results.List {
			if r29aMentionsHistogram(res.Type) {
				flagged = append(flagged, fn.Name.Name)
			}
		}
	}
	want := []string{"Census", "CensusPtr"}
	if len(flagged) != len(want) {
		t.Fatalf("the export-route predicate flagged %v, want exactly %v: it must catch a value return AND a pointer return, ignore unexported methods, and ignore an exported method that does not return the histogram", flagged, want)
	}
	for i := range want {
		if flagged[i] != want[i] {
			t.Fatalf("the export-route predicate flagged %v, want %v", flagged, want)
		}
	}
}
