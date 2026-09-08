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

// foldLiveStateSiteAllowed is the narrow, SITE-SCOPED exception list: selector → the enclosing
// functions where that read is permitted, each with the reason. It exists for exactly one selector.
//
// `verifyBond` is the injected-wiring read the cert requires be asserted LOUDLY at the box entry
// (R-VERIFYBOND-WIRING, Q4 row 3: the #572 replay shape — objective()/epochsEnabled() silently take
// the legacy branch on an unwired box). Asserting it there is the fix; BRANCHING on it anywhere else
// in a fold file is the defect. Two sites read it, neither branches on it:
//   - assembleStateRootRecomputeOps: the recompute entry's loud non-nil assertion (ErrRecomputeBoxWiring);
//   - (*Box).view (floorbox_box_v5.go): THREADS the verifier into provenView as its class-3
//     VerifyBond capability. NewBox already refused a chain whose verifier is unwired (objective()
//     requires verifyBond != nil, ErrBoxLegacyMode), so the value is asserted non-nil at
//     construction and view() only carries it; the composition calls it through StateView.
//
// Every listed site must still perform its read (the stale-site check below), so a deleted entry
// assertion reddens rather than silently going missing.
var foldLiveStateSiteAllowed = map[string]map[string]string{
	"verifyBond": {
		"assembleStateRootRecomputeOps": "the recompute entry asserts the injected verifier is wired (R-VERIFYBOND-WIRING)",
		"view":                          "(*Box).view threads the verifier, asserted wired by NewBox, into provenView.VerifyBond (class 3); no branch",
	},
}

// foldFileGlob is the set of non-test floor-box files the pin covers. Widened 2026-09-03
// (R-AST-PIN-GLOB): the earlier `floorbox_recompute_*_v5.go` missed `floorbox_recompute_v5.go`
// and four others — the files three of five box defeats lived in — so an AST gate failed by
// scope. Widened AGAIN 2026-09-08 (NG-4 tail, floor-box structure round 1A step 11):
// `floorbox_*_v5.go` still missed `floorbox_v5.go` — the file the pre-structure door lives in.
// Every non-test `floorbox_*.go` is now in, and TestNG4_FoldFileGlobCoversEveryFloorboxFile
// asserts the glob and the directory listing agree.
const foldFileGlob = "floorbox_*.go"

// foldFileFloor is the vacuity floor: the number of non-test floorbox_*.go files measured when the
// glob was last widened (13 on 2026-09-08). A count below it means the glob drifted from the
// naming, not that files were deleted — a deletion must lower this number in the same commit.
const foldFileFloor = 13

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
	for name, sites := range foldLiveStateSiteAllowed {
		if _, bad := foldLiveStateAllowed[name]; bad {
			t.Fatalf("PIN CORRUPTED: %q is BOTH blanket-allowed and site-scoped — the site scope is then vacuous", name)
		}
		for site, why := range sites {
			if why == "" {
				t.Fatalf("PIN CORRUPTED: the site-scoped allowance %s.%s carries no reason", site, name)
			}
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
	if len(foldFiles) < foldFileFloor {
		t.Fatalf("PIN VACUOUS: only %d fold files matched %q (floor %d) — the glob has drifted from the file naming",
			len(foldFiles), foldFileGlob, foldFileFloor)
	}

	fset := token.NewFileSet()
	parsed := make([]*ast.File, 0, len(foldFiles))
	for _, f := range foldFiles {
		af, pErr := parser.ParseFile(fset, f, nil, 0) // no comments: a c.<x> in prose is not a read
		if pErr != nil {
			t.Fatalf("parse %s: %v", f, pErr)
		}
		parsed = append(parsed, af)
	}
	idx := newFoldPinIndex(parsed)
	var violations []string
	siteHits := map[string]map[string]int{}
	for i, af := range parsed {
		file := foldFiles[i]
		for _, hit := range idx.liveReads(af) {
			name, enclosing := hit.sel, hit.enclosing
			if _, ok := foldLiveStateAllowed[name]; ok {
				continue
			}
			if sites, scoped := foldLiveStateSiteAllowed[name]; scoped {
				if _, ok := sites[enclosing]; ok {
					if siteHits[name] == nil {
						siteHits[name] = map[string]int{}
					}
					siteHits[name][enclosing]++
					continue
				}
			}
			if _, ok := idx.selfMethods[name]; ok {
				continue
			}
			pos := fset.Position(hit.pos)
			why := foldLiveStateDenied[name]
			if why == "" {
				why = "not classified — an unrecognised box-own read"
			}
			violations = append(violations, "  "+filepath.Base(file)+":"+itoa(pos.Line)+"  "+hit.path+"."+name+
				"  (in "+enclosing+")  — "+why)
		}
	}
	// A site-scoped allowance whose read has DISAPPEARED means the entry assertion was deleted or
	// moved. That is the R-VERIFYBOND-WIRING gate going silently missing, so it reddens too.
	for name, sites := range foldLiveStateSiteAllowed {
		for site := range sites {
			if siteHits[name][site] == 0 {
				t.Fatalf("SITE ALLOWANCE STALE: %s is scoped to %s but no such read exists any more.\n"+
					"  If the entry assertion moved, move the scope with it; if it was deleted, the\n"+
					"  R-VERIFYBOND-WIRING gate is gone and must be restored.", name, site)
			}
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

// TestFoldLiveStatePinHasTeeth proves the walk bites. It runs the SAME classifier
// (foldPinIndex.liveReads) over a synthetic fold file that re-injects the exact defect in every
// shape the pin must see — a `c.matureEpoch` branch selector on a *Chain method, a `s.c.epochSet`
// read through a RECEIVER-FIELD ALIAS on a box-owned struct (PE ruling F-1, 2026-09-08: the
// matcher keyed on the identifier `c`, so a Box method reading s.c.<map> was invisible), a `ch.<map>`
// read through a *Chain PARAMETER, and a read through a local `x := s.c` — and asserts each is
// flagged. Without this, a walk that silently matched nothing would look green forever — the
// decoration-green trap.
func TestFoldLiveStatePinHasTeeth(t *testing.T) {
	const injected = `package chain

type injectedBox struct {
	c    *Chain
	head HeadRef
}

func (c *Chain) reInjectedScreen(sc StateRootAttScreen) bool {
	if c.epochsEnabled() && c.matureEpoch { // the defect, re-injected
		return sc.InEpochSet
	}
	return sc.BondedSize >= c.cfg.MinBond || c.launchAnchor(sc.Attester)
}

func (s *injectedBox) reInjectedAliasRead() (bool, int) { return s.c.everMature, len(s.c.epochSet) }

func reInjectedParamRead(ch *Chain) int { return len(ch.bonded) }

func (s *injectedBox) reInjectedLocalRead() bool {
	x := s.c
	return x.handedOff()
}
`
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, "injected_fold_v5.go", injected, 0)
	if err != nil {
		t.Fatalf("parse synthetic: %v", err)
	}
	idx := newFoldPinIndex([]*ast.File{af})
	var flagged []string
	for _, hit := range idx.liveReads(af) {
		if _, ok := foldLiveStateAllowed[hit.sel]; ok {
			continue
		}
		if _, ok := idx.selfMethods[hit.sel]; ok {
			continue
		}
		flagged = append(flagged, hit.enclosing+":"+hit.path+"."+hit.sel)
	}
	sort.Strings(flagged)
	want := []string{
		"reInjectedAliasRead:s.c.epochSet",
		"reInjectedAliasRead:s.c.everMature",
		"reInjectedLocalRead:x.handedOff",
		"reInjectedParamRead:ch.bonded",
		"reInjectedScreen:c.launchAnchor",
		"reInjectedScreen:c.matureEpoch",
	}
	if strings.Join(flagged, ",") != strings.Join(want, ",") {
		t.Fatalf("PIN HAS NO TEETH: re-injecting live reads in four shapes into a fold file flagged\n  %v\nwant\n  %v",
			flagged, want)
	}
}

// foldPinIndex is what the pin knows about the fold files as a set: the *Chain methods they declare
// (self-dispatch, permitted) and, for every struct type they declare, the fields typed *Chain — the
// receiver-field aliases a method may reach live state through (Box.c).
type foldPinIndex struct {
	selfMethods map[string]struct{}
	chainFields map[string]map[string]bool // struct type name → field names typed Chain / *Chain
}

func newFoldPinIndex(files []*ast.File) foldPinIndex {
	idx := foldPinIndex{selfMethods: map[string]struct{}{}, chainFields: map[string]map[string]bool{}}
	for _, af := range files {
		for _, decl := range af.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil && len(d.Recv.List) > 0 && chainReceiver(d.Recv.List[0].Type) {
					idx.selfMethods[d.Name.Name] = struct{}{}
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, fld := range st.Fields.List {
						if !chainReceiver(fld.Type) {
							continue
						}
						if idx.chainFields[ts.Name.Name] == nil {
							idx.chainFields[ts.Name.Name] = map[string]bool{}
						}
						for _, n := range fld.Names {
							idx.chainFields[ts.Name.Name][n.Name] = true
						}
					}
				}
			}
		}
	}
	return idx
}

// liveRead is one selector that reaches a *Chain: `path.sel` inside `enclosing`.
type liveRead struct {
	enclosing string
	path      string // the chain-reaching expression as written: c, s.c, ch, x
	sel       string
	pos       token.Pos
}

// liveReads walks every FuncDecl in af and returns each `<chain>.<sel>` where <chain> is an
// expression that reaches a *Chain: the receiver of a *Chain method; a parameter typed *Chain or
// Chain; `recv.<field>` where recv is the receiver of a method on a fold-file struct and <field> is
// one of that struct's *Chain fields; or a local defined as `x := <one of the above>`. The name
// matters nowhere — the TYPE reachability does. A read through a shape this walk does not resolve
// (a chain returned from a call, a chain stored in a map) is outside the pin; keep the shapes here
// in step with what the fold files actually write.
func (idx foldPinIndex) liveReads(af *ast.File) []liveRead {
	var out []liveRead
	for _, decl := range af.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		// The chain-reaching roots of this function: name → true for bare idents; the
		// receiver-field aliases are (receiver name, field name) pairs.
		roots := map[string]bool{}
		aliases := map[string]map[string]bool{}
		if fd.Recv != nil && len(fd.Recv.List) > 0 && len(fd.Recv.List[0].Names) > 0 {
			recv := fd.Recv.List[0].Names[0].Name
			rt := fd.Recv.List[0].Type
			if star, ok := rt.(*ast.StarExpr); ok {
				rt = star.X
			}
			if chainReceiver(rt) {
				roots[recv] = true
			} else if id, ok := rt.(*ast.Ident); ok {
				if fields := idx.chainFields[id.Name]; len(fields) > 0 {
					aliases[recv] = fields
				}
			}
		}
		for _, p := range fd.Type.Params.List {
			if chainReceiver(p.Type) {
				for _, n := range p.Names {
					roots[n.Name] = true
				}
			}
		}
		// reaches reports whether e is a chain-reaching expression, and how it is written.
		var reaches func(e ast.Expr) (string, bool)
		reaches = func(e ast.Expr) (string, bool) {
			switch x := e.(type) {
			case *ast.Ident:
				if roots[x.Name] {
					return x.Name, true
				}
			case *ast.SelectorExpr:
				if base, ok := x.X.(*ast.Ident); ok && aliases[base.Name][x.Sel.Name] {
					return base.Name + "." + x.Sel.Name, true
				}
			case *ast.ParenExpr:
				return reaches(x.X)
			}
			return "", false
		}
		// Locals defined from a chain-reaching expression become roots (x := s.c).
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || as.Tok != token.DEFINE || len(as.Lhs) != len(as.Rhs) {
				return true
			}
			for i := range as.Lhs {
				if _, ok := reaches(as.Rhs[i]); ok {
					if id, ok := as.Lhs[i].(*ast.Ident); ok {
						roots[id.Name] = true
					}
				}
			}
			return true
		})
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if path, ok := reaches(sel.X); ok {
				out = append(out, liveRead{enclosing: fd.Name.Name, path: path, sel: sel.Sel.Name, pos: sel.Pos()})
			}
			return true
		})
	}
	return out
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

// TestNG4_FoldFileGlobCoversEveryFloorboxFile (NG-4 tail, step 11). MEASURED twice on the donor
// branch and once here: a `floorbox_recompute_*_v5.go` glob covered 10 of 15 files and missed
// floorbox_recompute_v5.go (where F1, G-I and N1 live); `floorbox_*_v5.go` covered 12 of 13 and
// missed floorbox_v5.go (the pre-structure door). The gate built to be a defect family's recurrence
// teeth did not read the file the family lives in. This asserts the glob matches EVERY non-test
// floorbox_*.go in the package and that the floor equals the count.
// SOURCE GATE: a directory listing compared with a glob. RUNTIME GATE: TestFoldFilesReadNoLiveBoxState
// (the pin itself, over the widened set). Ablation (NG-4): revert the glob to floorbox_*_v5.go ⇒ RED.
func TestNG4_FoldFileGlobCoversEveryFloorboxFile(t *testing.T) {
	all, err := filepath.Glob("floorbox_*.go")
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, f := range all {
		if !strings.HasSuffix(f, "_test.go") {
			want = append(want, f)
		}
	}
	got, err := filepath.Glob(filepath.Join(".", foldFileGlob))
	if err != nil {
		t.Fatal(err)
	}
	var covered []string
	for _, f := range got {
		if !strings.HasSuffix(f, "_test.go") {
			covered = append(covered, f)
		}
	}
	sort.Strings(want)
	sort.Strings(covered)
	if strings.Join(want, ",") != strings.Join(covered, ",") {
		t.Fatalf("SOURCE GATE: NG-4 — foldFileGlob %q covers %d of %d non-test floorbox_*.go files.\n  covered: %v\n  all:     %v",
			foldFileGlob, len(covered), len(want), covered, want)
	}
	if len(want) != foldFileFloor {
		t.Fatalf("SOURCE GATE: NG-4 — %d non-test floorbox_*.go files, foldFileFloor is %d; move the floor with the file set in the same commit",
			len(want), foldFileFloor)
	}
}
