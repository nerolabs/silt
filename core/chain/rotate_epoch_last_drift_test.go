package chain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// R-ROTATE-EPOCH-LAST — the drift guard for the load-bearing coupling that #620
// leans on (RULING-620-mature-epoch-order-independence-2026-08-28, "Couplings the
// consult should carry forward").
//
// The claim #620 discharges — `epochSet` is an order-INVARIANT read of the two
// final maps — holds ONLY BY CONSTRUCTION, and that construction is two facts about
// production code, NEITHER guarded before this file:
//
//  1. rotateEpoch runs LAST in apply(chain.go), AFTER every bonded/slashed mutation
//     and the maturity latch. So the freeze reads the block's POST-APPLY state, and
//     `epochSet = liveQualifiedSet(bonded, slashed)` is a deterministic function of
//     the two FINAL maps — order-invariant, because those maps' own order-independence
//     is covered elsewhere (#617/#618). Move rotateEpoch before slash/bond application
//     and it freezes a PRE-final (mid-apply) set, reopening the I3 mid-epoch-churn fork.
//  2. liveQualifiedSet reads ONLY bonded, slashed, and cfg.MinBond — no history
//     (blocks/revLog/bondRegHeight/…). Make it read history and `epochSet` becomes
//     history-DEPENDENT, silently breaking the SMT history-independence premise for
//     the frozen set even if rotate still runs last.
//
// Both facts are STRUCTURAL, so the guards are structural — a purely behavioral
// fixture can pass through a refactor that reorders statements when the scenario does
// not happen to distinguish them (the exact scope-honesty gap the #620 ruling names:
// "convergent by construction," not "a divergence-capable ordering was tried"). These
// tests pin the construction itself, then a behavioral fixture confirms the effect.
//
// RED-on-injection (session-7 rule — a green check with no demonstrated red is a
// comment that compiles): each guard was ablated red before it was trusted. The
// injections and their reddening are recorded in this file's doc comments and the
// R-ROTATE-EPOCH-LAST report. This file adds NO production logic (a _test.go only);
// the class-P fix that will touch apply()/rotateEpoch is separate and gated.

// parseChainAST parses core/chain/chain.go into an AST, located relative to this test
// file so it does not depend on the working directory (the readChainSource pattern).
func parseChainAST(t *testing.T) (*token.FileSet, *ast.File) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate the source directory")
	}
	dir := thisFile[:strings.LastIndex(thisFile, "/")]
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, dir+"/chain.go", nil, 0)
	if err != nil {
		t.Fatalf("parse chain.go: %v", err)
	}
	return fset, f
}

// chainMethod returns the *ast.FuncDecl for method `(c *Chain) name` in chain.go.
func chainMethod(t *testing.T, f *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name || fn.Recv == nil {
			continue
		}
		return fn
	}
	t.Fatalf("method %q not found in chain.go — was it renamed? this guard pins a real coupling", name)
	return nil
}

// callName returns the selector name of a call expression `x.Name(...)`, or "".
func callName(e ast.Expr) string {
	ce, ok := e.(*ast.CallExpr)
	if !ok {
		return ""
	}
	se, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return se.Sel.Name
}

// TestRotateEpochIsLastInApply is the STRUCTURAL half of guard (1): rotateEpoch is
// the LAST thing apply() does. The freeze's order-invariance depends on it reading the
// block's post-apply state, which holds iff no bonded/slashed mutation runs after it.
//
// The guard asserts, from the AST of apply(): the final top-level statement is the
// epoch-rotation gate, and its ONLY body statement is the c.rotateEpoch(...) call.
//
// RED-on-injection (verified, then restored): moving the `if c.epochsEnabled() && … {
// c.rotateEpoch(b.Height) }` gate above the slash loop in apply() makes the last
// statement the slash loop (an *ast.RangeStmt, not the rotate gate), so this test
// fatals with "rotateEpoch is NOT last". GREEN on current code.
func TestRotateEpochIsLastInApply(t *testing.T) {
	_, f := parseChainAST(t)
	apply := chainMethod(t, f, "apply")

	body := apply.Body.List
	if len(body) == 0 {
		t.Fatal("apply() has an empty body — impossible")
	}
	last := body[len(body)-1]

	ifs, ok := last.(*ast.IfStmt)
	if !ok {
		t.Fatalf("the LAST statement of apply() is %T, not the epoch-rotation gate (*ast.IfStmt) — "+
			"rotateEpoch is NOT last, so the boundary freeze may read a PRE-final (mid-apply) set "+
			"(I3 mid-epoch-churn divergence; #620 order-invariance premise broken)", last)
	}
	if len(ifs.Body.List) != 1 {
		t.Fatalf("the final gate of apply() has %d body statements, want exactly 1 (the rotateEpoch call) — "+
			"an extra statement in the boundary gate is not the guarded shape", len(ifs.Body.List))
	}
	es, ok := ifs.Body.List[0].(*ast.ExprStmt)
	if !ok || callName(es.X) != "rotateEpoch" {
		t.Fatalf("the final gate of apply() does not call c.rotateEpoch(...) as its sole body statement — "+
			"got %T; rotateEpoch is NOT the last thing apply() does (the freeze may read a stale set)", ifs.Body.List[0])
	}
}

// TestLiveQualifiedSetReadsOnlyFinalMaps is the STRUCTURAL half of guard (2):
// liveQualifiedSet reads ONLY the two final maps and MinBond — never history. If it
// read blocks/revLog/bondRegHeight/… the frozen epochSet would become history-
// dependent, breaking the SMT history-independence premise even with rotate still last.
//
// The guard walks liveQualifiedSet's body and collects every `c.<field>` reference,
// then asserts the set is a subset of the allowlist {bonded, slashed, cfg}. cfg is the
// MinBond floor; any other c.<field> is a NEW read — reddens.
//
// RED-on-injection (verified, then restored): adding a reference to a history field in
// liveQualifiedSet, e.g. `_ = len(c.blocks)` or gating on `c.bondRegHeight[id]`, makes
// "blocks"/"bondRegHeight" appear in the reference set and this test fatals with the
// disallowed-field name. GREEN on current code (references are exactly bonded/slashed/cfg).
func TestLiveQualifiedSetReadsOnlyFinalMaps(t *testing.T) {
	_, f := parseChainAST(t)
	lqs := chainMethod(t, f, "liveQualifiedSet")

	// The ONLY receiver fields the freeze read is allowed to touch. bonded/slashed are
	// the two final maps; cfg supplies MinBond. Adding ANY other field is a history read
	// (or a new coupling) and must be re-certified, not slipped in silently.
	allowed := map[string]bool{"bonded": true, "slashed": true, "cfg": true}

	refs := map[string]bool{}
	ast.Inspect(lqs.Body, func(n ast.Node) bool {
		se, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := se.X.(*ast.Ident)
		if !ok || id.Name != "c" {
			return true
		}
		refs[se.Sel.Name] = true
		return true
	})

	if len(refs) == 0 {
		t.Fatal("liveQualifiedSet references NO receiver field — it no longer reads bonded/slashed at all; " +
			"the freeze source moved, the #620 premise no longer describes this function")
	}
	for field := range refs {
		if !allowed[field] {
			t.Fatalf("liveQualifiedSet reads c.%s — NOT one of the two final maps {bonded, slashed} or cfg. "+
				"If it reads history (blocks/revLog/bondRegHeight/…), the frozen epochSet becomes "+
				"history-DEPENDENT and the SMT history-independence premise for the mature set (#620) breaks. "+
				"This is a research-gated change, not a refactor.", field)
		}
	}
	// The two maps must BOTH still be read — a freeze that dropped `slashed` would admit
	// a slashed member; one that dropped `bonded` would freeze an empty set.
	for _, must := range []string{"bonded", "slashed"} {
		if !refs[must] {
			t.Fatalf("liveQualifiedSet no longer reads c.%s — the freeze predicate changed; "+
				"the frozen epochSet is no longer filter(bonded, slashed, MinBond)", must)
		}
	}
}

// TestLiveQualifiedSetIsPureFunctionOfFinalMaps is the BEHAVIORAL companion to the
// structural read-set guard: liveQualifiedSet's OUTPUT is a function of (bonded,
// slashed, MinBond) alone. It mutates a HISTORY field (c.blocks) and asserts the output
// is unchanged; then mutates a final map (c.slashed) and asserts the output DOES change.
// A refactor that read history would make the first assertion redden.
//
// RED-on-injection (verified, then restored): if liveQualifiedSet filtered on
// len(c.blocks), appending a block would change the output and the "history must not
// change the output" assertion fatals. GREEN on current code.
func TestLiveQualifiedSetIsPureFunctionOfFinalMaps(t *testing.T) {
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	a, b := key(87001), key(87002)
	c.bonded = map[ports.NodeID]int64{idOf(a): 4 << 20, idOf(b): 4 << 20}
	c.slashed = map[ports.NodeID]bool{}

	before := c.liveQualifiedSet()
	if len(before) != 2 {
		t.Fatalf("setup: expected 2 qualified, got %d", len(before))
	}

	// Mutate HISTORY (append a fabricated block). The output must NOT move.
	c.blocks = append(c.blocks, Block{Height: 999})
	afterHistory := c.liveQualifiedSet()
	if !reflect.DeepEqual(before, afterHistory) {
		t.Fatalf("liveQualifiedSet changed after a HISTORY mutation (appended block): %v -> %v — "+
			"it is reading history, not just the two final maps; the frozen epochSet is history-dependent",
			before, afterHistory)
	}

	// Mutate a FINAL MAP (slash a member). The output MUST move — proves the test is
	// not vacuously green because liveQualifiedSet ignores everything.
	c.slashed[idOf(a)] = true
	afterSlash := c.liveQualifiedSet()
	if reflect.DeepEqual(before, afterSlash) {
		t.Fatal("liveQualifiedSet did NOT change after slashing a member — the guard is vacuous; " +
			"the function is not actually reading c.slashed")
	}
	if _, in := afterSlash[idOf(a)]; in {
		t.Fatal("liveQualifiedSet still returns a SLASHED member — the freeze predicate is broken")
	}
}

// TestRotateEpochFreezesPostApplySetNotPreBlock is the BEHAVIORAL confirmation of guard
// (1): at a boundary block that ALSO slashes a member, the frozen epochSet equals the
// POST-apply recompute and DIFFERS from the PRE-block set. This makes the ordering
// DISTINGUISHABLE — the exact non-vacuity the #620 ruling flags as missing from the
// original fixture ("a divergence-capable ordering was tried and converged"): here the
// pre- and post-freeze sets genuinely differ, so rotate-LAST is load-bearing, not moot.
//
// RED-on-injection (verified, then restored): sourcing the freeze from a snapshot of
// qualified taken BEFORE the slash loop (i.e. rotate reading pre-final state) freezes
// the victim into epochSet, so the "epochSet excludes the slashed member" and "epochSet
// == post-apply recompute" assertions fatal. GREEN on current code.
func TestRotateEpochFreezesPostApplySetNotPreBlock(t *testing.T) {
	a, b, victim := key(88001), key(88002), key(88003)
	// MatureValidators=0 hands maturity off at the genesis boundary, so rotateEpoch
	// freezes a real set without driving the full latch (the #535/stale-capture pattern).
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis seats a, b, victim and hands off maturity (boundary h0).
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = []BondReg{
		bondReg(a, 4<<20, ports.Hash{}), bondReg(b, 4<<20, ports.Hash{}), bondReg(victim, 4<<20, ports.Hash{})}
	Sign(g, a)
	c.apply(*g)

	// h1: an ordinary mid-epoch block — all three still qualified going into the boundary.
	prev := g.Hash()
	b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(b1, a)
	c.apply(*b1)

	// Snapshot the PRE-boundary-block qualified set: this is what a rotate that ran
	// BEFORE the block's slash would freeze. The victim is IN it here.
	preBlockSet := c.liveQualifiedSet()
	if _, in := preBlockSet[idOf(victim)]; !in {
		t.Fatal("setup: victim must be qualified BEFORE the boundary block, or the ordering is not distinguishable")
	}

	// h2: the BOUNDARY block slashes the victim. rotate-LAST must freeze the POST-slash
	// set ({a, b}), NOT the pre-block set ({a, b, victim}).
	prev = b1.Hash()
	b2 := &Block{Version: 1, Height: 2, Prev: prev, Entries: []ports.Entry{entry(2)},
		Slashes: []Equivocation{slashProof(victim, prev, 0x71, 0x72)}}
	Sign(b2, a)
	c.apply(*b2)

	if !c.matureEpoch {
		t.Fatal("fixture did not mature — the boundary freeze never ran, the guard is vacuous")
	}

	// The freeze must equal the POST-apply recompute over the two final maps.
	if !reflect.DeepEqual(c.epochSet, recomputeQualified(c)) {
		t.Fatalf("frozen epochSet %v != post-apply filter(bonded, slashed) %v — rotate did NOT read this "+
			"block's POST-apply set; it is no longer LAST in apply()", c.epochSet, recomputeQualified(c))
	}
	// The freeze must EXCLUDE the slashed victim (post-final state).
	if _, in := c.epochSet[idOf(victim)]; in {
		t.Fatal("STALE CAPTURE: the boundary froze the slashed victim into epochSet — rotate read a " +
			"PRE-final (pre-slash) set; rotate-LAST is broken (I3 mid-epoch-churn divergence, #620 premise gone)")
	}
	// The freeze must DIFFER from the pre-block set — proving the ordering is genuinely
	// distinguishable (the #620 non-vacuity gap: pre != post here, so rotate-LAST matters).
	if reflect.DeepEqual(c.epochSet, preBlockSet) {
		t.Fatal("frozen epochSet EQUALS the pre-boundary-block set — the ordering is not distinguishable in " +
			"this fixture, so it cannot witness rotate-LAST; strengthen the fixture (the #620 scope-honesty gap)")
	}
}
