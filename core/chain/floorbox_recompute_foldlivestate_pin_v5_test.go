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
// THE FOLD-FILE LIVE-STATE ALLOWLIST PIN (R-FOLD-LIVE-STATE-READS recurrence teeth)
// =============================================================================
//
// Research cert: floorbox-R-FOLD-LIVE-STATE-READS-RESEARCH-CERTIFICATION-2026-09-02.md §Q3 last
// section. PE ruling: RULING-R-CARRIER-REFLECTION-pin-2026-09-02.md §"The coupling the consult
// missed" item 1.
//
// WHAT IT PINS. The recompute's contract is "Accept iff the recomputed post-root equals b.StateRoot
// (== what a full node would accept)". That makes the verdict a function of
// (prevStateRoot, committedStateRoot, b, w, own-cfg) and NOTHING else. Every witness-carried value
// is reflection-pinned by the R1.2 coverage table. A `c.<liveState>` read escapes that pin BY
// CONSTRUCTION — it is not a carrier field, so no reflection walk sees it. That is exactly how
// `c.matureEpoch` decided the class-A screen's branch for four sub-increments without any coverage
// test noticing.
//
// HOW. Parse every non-test fold file and flag any `c.<selector>` whose Sel is not allowlisted.
//
// THE ALLOWLIST IS DELIBERATELY NARROW (the cert CORRECTS the ruling's proposal here — the ruling's
// list included `matureEpoch` and `launchAnchor`, which would have PINNED THE DEFECT IN PLACE):
//
//	cfg / epochsEnabled / objective / operatorMargin   — own-cfg (C-6) + the injected verifier
//	launchAnchorGiven                                  — the shared predicate, handoff SUPPLIED
//	<methods declared on Chain in the fold files>      — self-dispatch
//
// EXPLICITLY OUTSIDE IT, and asserted so: matureEpoch, everMature, launchAnchor, handedOff, plus
// every committed map (bonded / slashed / epochSet / validatorsSeen / qualified / bondDomain) and
// the legacy rep(). Those are the box's own applied-history view — which a box that replays no
// apply() does not have.

// foldLiveStateAllowed is the allowlist of `c.<sel>` names a fold file may read, beyond the
// self-dispatch methods the walk derives. Each entry states WHY it cannot make the verdict depend on
// applied history.
var foldLiveStateAllowed = map[string]string{
	"cfg":            "own-cfg (C-6): genesis/operator configuration the box is trusted to hold",
	"objective":      "cfg.MinBond>0 && verifyBond!=nil — own-cfg + the INJECTED verifier, asserted wired at the box entry (ErrRecomputeBoxWiring)",
	"epochsEnabled":  "cfg.EpochBlocks>0 && objective() — own-cfg + the injected verifier",
	"operatorMargin": "cfg.OperatorMargin accessor — own-cfg",
	"launchAnchorGiven": "the SHARED launch-anchor predicate with the handoff bool SUPPLIED by the caller " +
		"(the box supplies the ANCHORED committed pre-state). Reads cfg.Anchors only; its body is walked below.",
}

// foldLiveStateDenied is the explicit denylist: names that MUST NOT be allowlisted, each with the
// reason a fold file reading it is a soundness defect. It is asserted against the allowlist so a
// future edit cannot quietly re-admit one.
var foldLiveStateDenied = map[string]string{
	"matureEpoch":    "the class-A branch selector; written only by apply→rotateEpoch and adopt — a cold box never sets it",
	"everMature":     "the maturity latch; same writers, same cold-box hole",
	"handedOff":      "reads matureEpoch/everMature",
	"launchAnchor":   "reads handedOff()",
	"bonded":         "a committed MAP — must be witness-Resolved against prevStateRoot, never read live",
	"slashed":        "a committed MAP — must be witness-Resolved",
	"epochSet":       "a committed MAP — must be witness-Resolved",
	"validatorsSeen": "a committed MAP — must be witness-Resolved",
	"qualified":      "a committed MAP — must be witness-Resolved",
	"bondDomain":     "a committed MAP — must be witness-Resolved",
	"rep":            "the LEGACY reputation view; not a committed leaf at all (the class-A screen asserts objective mode and stalls)",
	"head":           "applied-history chain state",
	"blocks":         "applied-history chain state",
	"verifyBond":     "a fold file must not BRANCH on the injected verifier — it is asserted once, at the entry (see foldLiveStateSiteAllowed)",
}

// foldLiveStateSiteAllowed is the narrow, SITE-SCOPED exception list: selector → the single
// enclosing function where that read is permitted. It exists for exactly one read.
//
// `c.verifyBond` is the injected-wiring read the cert requires be asserted LOUDLY at the box entry
// (R-VERIFYBOND-WIRING, Q4 row 3: the #572 replay shape — objective()/epochsEnabled() silently take
// the legacy branch on an unwired box). Asserting it there is the fix; BRANCHING on it anywhere else
// in a fold file is the defect. Scoping the allowance to the one entry function keeps both true.
var foldLiveStateSiteAllowed = map[string]string{
	"verifyBond": "assembleStateRootRecomputeOps",
}

// foldFileGlob is the set of non-test recompute fold files the pin covers.
const foldFileGlob = "floorbox_recompute_*_v5.go"

// TestFoldFilesReadNoLiveBoxState is the pin. Any `c.<sel>` in a fold file that is neither
// allowlisted nor a self-dispatch method declared in a fold file reddens it.
func TestFoldFilesReadNoLiveBoxState(t *testing.T) {
	for name, why := range foldLiveStateDenied {
		if _, bad := foldLiveStateAllowed[name]; bad {
			t.Fatalf("PIN CORRUPTED: %q is on the BLANKET allowlist but it is a DENIED live-state read (%s).\n"+
				"  Allowlisting it would pin the R-FOLD-LIVE-STATE-READS defect in place — the exact\n"+
				"  correction the research cert made to the PE ruling's proposed allowlist.", name, why)
		}
	}
	for name := range foldLiveStateSiteAllowed {
		if _, bad := foldLiveStateAllowed[name]; bad {
			t.Fatalf("PIN CORRUPTED: %q is BOTH blanket-allowed and site-scoped — the site scope is then vacuous", name)
		}
	}
	for _, denied := range []string{"matureEpoch", "everMature", "launchAnchor", "handedOff"} {
		if _, bad := foldLiveStateSiteAllowed[denied]; bad {
			t.Fatalf("PIN CORRUPTED: %q was given a site-scoped exception — the two handoff latch fields and\n"+
				"  their predicates have NO permitted read site in a fold file. Anchor them (handoffPreState).", denied)
		}
	}

	files, err := filepath.Glob(filepath.Join(".", foldFileGlob))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	var foldFiles []string
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			foldFiles = append(foldFiles, f)
		}
	}
	if len(foldFiles) < 8 {
		t.Fatalf("PIN VACUOUS: only %d fold files matched %q — the glob has drifted from the file naming",
			len(foldFiles), foldFileGlob)
	}

	fset := token.NewFileSet()
	parsed := make([]*ast.File, 0, len(foldFiles))
	selfMethods := map[string]struct{}{}
	for _, f := range foldFiles {
		af, pErr := parser.ParseFile(fset, f, nil, 0) // no comments: a c.<x> in prose is not a read
		if pErr != nil {
			t.Fatalf("parse %s: %v", f, pErr)
		}
		parsed = append(parsed, af)
		for _, decl := range af.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
				continue
			}
			if chainReceiver(fd.Recv.List[0].Type) {
				selfMethods[fd.Name.Name] = struct{}{}
			}
		}
	}

	var violations []string
	siteHits := map[string]int{}
	for i, af := range parsed {
		file := foldFiles[i]
		for _, decl := range af.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			enclosing := fd.Name.Name
			ast.Inspect(fd, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok || ident.Name != "c" {
					return true
				}
				name := sel.Sel.Name
				if _, ok := foldLiveStateAllowed[name]; ok {
					return true
				}
				if site, scoped := foldLiveStateSiteAllowed[name]; scoped && site == enclosing {
					siteHits[name]++
					return true
				}
				if _, ok := selfMethods[name]; ok {
					return true
				}
				pos := fset.Position(sel.Pos())
				why := foldLiveStateDenied[name]
				if why == "" {
					why = "not classified — an unrecognised box-own read"
				}
				violations = append(violations, "  "+filepath.Base(file)+":"+itoa(pos.Line)+"  c."+name+
					"  (in "+enclosing+")  — "+why)
				return true
			})
		}
	}
	// A site-scoped allowance whose read has DISAPPEARED means the entry assertion was deleted or
	// moved. That is the R-VERIFYBOND-WIRING gate going silently missing, so it reddens too.
	for name, site := range foldLiveStateSiteAllowed {
		if siteHits[name] == 0 {
			t.Fatalf("SITE ALLOWANCE STALE: c.%s is scoped to %s but no such read exists any more.\n"+
				"  If the entry assertion moved, move the scope with it; if it was deleted, the\n"+
				"  R-VERIFYBOND-WIRING gate is gone and must be restored.", name, site)
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("FOLD FILES READ LIVE BOX STATE (%d site(s)):\n%s\n\n"+
			"  The recompute's verdict must be a function of (prevStateRoot, committedStateRoot, b, w,\n"+
			"  own-cfg) ONLY. A box-own field is not one of those: the deployment target holds no\n"+
			"  registry and replays no apply(), so its accelerator fields are never written and a read\n"+
			"  of one silently screens under the wrong rule (R-FOLD-LIVE-STATE-READS, 2026-09-02).\n"+
			"  Anchor the value: Resolve the committed leaf against prevStateRoot (Direction A) and\n"+
			"  thread it in, as handoffPreState does for everMature/matureEpoch. Do NOT add the name to\n"+
			"  foldLiveStateAllowed.", len(violations), strings.Join(violations, "\n"))
	}
}

// TestLaunchAnchorGivenReadsNoLiveState walks the ONE allowlisted predicate that lives OUTSIDE the
// fold files (chain.go). Allowlisting it by name would otherwise be a hole: a future edit could make
// its body read c.handedOff() and the fold-file walk would never see it.
func TestLaunchAnchorGivenReadsNoLiveState(t *testing.T) {
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, "chain.go", nil, 0)
	if err != nil {
		t.Fatalf("parse chain.go: %v", err)
	}
	var body *ast.FuncDecl
	for _, decl := range af.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if ok && fd.Name.Name == "launchAnchorGiven" && fd.Recv != nil {
			body = fd
		}
	}
	if body == nil {
		t.Fatalf("launchAnchorGiven not found in chain.go — the fold files allowlist it by name; if it " +
			"moved or was renamed, update foldLiveStateAllowed and this walk together")
	}
	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "c" && sel.Sel.Name != "cfg" {
			t.Fatalf("launchAnchorGiven reads c.%s at %s — it is allowlisted for the fold files ONLY "+
				"because it reads own-cfg (Anchors) and takes the handoff predicate as a PARAMETER. "+
				"A live-state read here re-opens R-FOLD-LIVE-STATE-READS through the allowlisted door.",
				sel.Sel.Name, fset.Position(sel.Pos()))
		}
		return true
	})
}

// TestFoldLiveStatePinHasTeeth proves the walk bites. It runs the SAME classification over a
// synthetic fold file that re-injects the exact defect (a `c.matureEpoch` branch selector) and
// asserts it is flagged. Without this, a walk that silently matched nothing would look green
// forever — the decoration-green trap.
func TestFoldLiveStatePinHasTeeth(t *testing.T) {
	const injected = `package chain

func (c *Chain) reInjectedScreen(sc StateRootAttScreen) bool {
	if c.epochsEnabled() && c.matureEpoch { // the defect, re-injected
		return sc.InEpochSet
	}
	return sc.BondedSize >= c.cfg.MinBond || c.launchAnchor(sc.Attester)
}
`
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, "injected_fold_v5.go", injected, 0)
	if err != nil {
		t.Fatalf("parse synthetic: %v", err)
	}
	selfMethods := map[string]struct{}{"reInjectedScreen": {}}
	var flagged []string
	ast.Inspect(af, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "c" {
			return true
		}
		name := sel.Sel.Name
		if _, ok := foldLiveStateAllowed[name]; ok {
			return true
		}
		if _, ok := selfMethods[name]; ok {
			return true
		}
		flagged = append(flagged, name)
		return true
	})
	sort.Strings(flagged)
	want := []string{"launchAnchor", "matureEpoch"}
	if len(flagged) != len(want) {
		t.Fatalf("PIN HAS NO TEETH: re-injecting c.matureEpoch + c.launchAnchor into a fold file flagged %v, want %v",
			flagged, want)
	}
	for i := range want {
		if flagged[i] != want[i] {
			t.Fatalf("PIN HAS NO TEETH: flagged %v, want %v", flagged, want)
		}
	}
}

func chainReceiver(t ast.Expr) bool {
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	ident, ok := t.(*ast.Ident)
	return ok && ident.Name == "Chain"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
