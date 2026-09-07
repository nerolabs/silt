package chain

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// =============================================================================
// THE STAGE-COVER GATE — the composition covers every node validity stage (R-PTABLE-DRIFT teeth)
// =============================================================================
//
// P-table delta certification §4 (CERTIFIED, classified BOUNDED):
// /Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-P-TABLE-DRIFT-DELTA-CERTIFICATION-e963034-2026-09-07.md
//
// WHY. The node's v5 stage table was hand-copied into a certification on 2026-09-03 and had
// drifted by 2026-09-07: main's ValidateProposal had gained validateIssuerKeys (v5-ONLY) and a
// sixth P5 clause, and the composition built from the table had neither — a validity rule the
// node enforces today, silently dropped from the path that replaces it. This gate DERIVES the
// node's stage list from ValidateProposal / ValidateCommit / requireQuorumStack by AST walk and
// asserts the nodeStages table (validate_v5.go) and the composition cover it.
//
// THE HONEST BOUND. Four arms, and they are NOT a completeness proof:
//   - arm A DERIVES cover for the stages that are error-returning CALLS (14 of 24);
//   - arm B is a CHANGE DETECTOR (a body digest) over the three functions whose INLINE predicates
//     and stage ORDER the composition mirrors by hand (P1–P5, Q1–Q2 live there);
//   - arm C is the reverse cover (no invented stage in the composition);
//   - arm D is the fixture's non-vacuity, and it runs FIRST: a v2 fixture makes A–C vacuous.
//
// Neither arm sees an inline predicate change INSIDE a called stage (a new clause inside
// validateBondRegs). The composition MIRRORS those bodies over StateView rather than calling them
// (they are *Chain methods), so the mirrored bodies are inside the bound too. That is
// R-PTABLE-DRIFT: BOUNDED, not closed.
//
// SOURCE GATE — all of A, B, C read the project's own .go source (parsed comment-free: a
// `c.<x>` in prose is not a call). They see names, order and text, never behaviour.
// RUNTIME GATE: TestGD5_IssuerKeysOnlyBlockParity and TestGD6_LegacyModeParity
// (redteam_floorbox_structure_gate_test.go) drive the stages the drift actually hit.

// nodeStageFiles are the files the node's stage list is derived over (certification §4 arm A
// step 1). A stage that moves to another file must be added here, or arm A reddens on the
// missing FuncDecl — that is the intended failure.
var nodeStageFiles = []string{"chain.go", "era3validity.go", "issuerkey.go", "carrier.go"}

// compositionFiles are the files the composition's calls are derived over. The composition index
// is built over these PLUS nodeStageFiles, so the one DIRECT call into the node's files
// (validateCarrier, receiverless) resolves; a composition function shadowing a node name would
// redden parseIndex's duplicate check.
var compositionFiles = []string{"validate_v5.go", "validate_v5_predicates.go", "validate_v5_quorum.go"}

func compositionIndex(t *testing.T) funcIndex {
	t.Helper()
	return parseIndex(t, append(append([]string(nil), compositionFiles...), nodeStageFiles...))
}

// compositionHelpers are the error-returning functions the composition calls that are NOT stage
// mirrors: the mirrors of node helpers that return a bool/int/map (and so are not stages by the
// arm-A rule), the two box-owned steps 0/0b, the scalar decoders, and stall. Arm C admits a
// composition call iff it is a stage mirror, one of these, or the proposal entry itself. Each
// entry names the node function it mirrors ("" for a utility with no node analogue), and arm C
// asserts the named node function EXISTS in the node files — so a renamed node helper reddens.
var compositionHelpers = []struct{ helper, node, why string }{
	{"ValidateProposalV5", "ValidateProposal", "the proposal entry, called by ValidateCommitV5 (M-2)"},
	{"v5VersionPartition", "", "step 0: the L1 version partition (box-owned scope; node: no-op)"},
	{"v5CheckBudget", "", "step 0b: the BG-3 frame budget (box-owned; node: unlimited)"},
	{"stall", "", "the named-read stall constructor"},
	{"v5ScalarBool", "", "committed scalar decoder"},
	{"v5ScalarUint64", "", "committed scalar decoder"},
	{"v5HandedOff", "handedOff", "the young→mature handoff, read from committed latch scalars"},
	{"v5MatureEpochRegime", "epochsEnabled", "the (epochsEnabled && matureEpoch) regime selector"},
	{"v5EffectiveEpochSet", "effectiveEpochSet", "the #535 substitution rule, IN the composition"},
	{"v5AttesterQualifiedAt", "attesterQualifiedAt", "the attester filter, both modes"},
	{"v5ValidateEntry", "ValidateEntry", "the per-entry rule inside the P9 loop"},
	{"v5EraActive", "era3Active", "era3Active / era4Active"},
	{"v5RecentBondRegNonces", "recentBondRegNonces", "the bounded header-window nonces"},
	{"v5RegGateActive", "regGateActive", "the #506 gate activation"},
	{"v5RestoresHeldStanding", "restoresHeldStanding", "the #506 R-interval exemption"},
	{"v5ValidatorSetSize", "validatorSetSize", "N for the Byzantine threshold"},
	{"v5MatureNow", "matureNow", "the objective maturity metric"},
}

// ---------------------------------------------------------------------------
// the shared AST machinery
// ---------------------------------------------------------------------------

type funcIndex struct {
	fset  *token.FileSet
	decls map[string]*ast.FuncDecl // package-level FuncDecls by name (methods and functions)
}

func parseIndex(t *testing.T, files []string) funcIndex {
	t.Helper()
	idx := funcIndex{fset: token.NewFileSet(), decls: map[string]*ast.FuncDecl{}}
	for _, f := range files {
		af, err := parser.ParseFile(idx.fset, f, nil, 0) // no comments: a c.<x> in prose is not a call
		if err != nil {
			t.Fatalf("SOURCE GATE: parse %s: %v", f, err)
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if prev, dup := idx.decls[fd.Name.Name]; dup {
				// A method and a function may share a name only if one is not a *Chain member;
				// the stage functions are unique by name, so a collision is a real ambiguity.
				t.Fatalf("SOURCE GATE: %s declared twice (%s and %s) — the name-keyed derivation cannot tell them apart",
					fd.Name.Name, idx.fset.Position(prev.Pos()), idx.fset.Position(fd.Pos()))
			}
			idx.decls[fd.Name.Name] = fd
		}
	}
	return idx
}

// lastResultIsError is the arm-A stage rule: a call is a stage iff its callee's LAST result is
// the identifier `error`.
func lastResultIsError(fd *ast.FuncDecl) bool {
	if fd.Type.Results == nil || len(fd.Type.Results.List) == 0 {
		return false
	}
	last := fd.Type.Results.List[len(fd.Type.Results.List)-1]
	id, ok := last.Type.(*ast.Ident)
	return ok && id.Name == "error"
}

// calledStages returns, in source order, the names of the error-returning package-level
// functions called in body as `c.<name>(...)` or bare `<name>(...)`, with duplicates kept.
func (idx funcIndex) calledStages(body *ast.BlockStmt) []string {
	type hit struct {
		name string
		pos  token.Pos
	}
	var hits []hit
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var name string
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if x, ok := fn.X.(*ast.Ident); ok && x.Name == "c" {
				name = fn.Sel.Name
			}
		case *ast.Ident:
			name = fn.Name
		}
		if name == "" {
			return true
		}
		fd, ok := idx.decls[name]
		if !ok || !lastResultIsError(fd) {
			return true
		}
		hits = append(hits, hit{name, call.Pos()})
		return true
	})
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.name)
	}
	return out
}

// closure walks the transitive call closure from root under the same rule, returning every
// distinct stage reached (root excluded), in first-reached order.
func (idx funcIndex) closure(root string) []string {
	var out []string
	seen := map[string]bool{root: true}
	var walk func(name string)
	walk = func(name string) {
		fd := idx.decls[name]
		if fd == nil || fd.Body == nil {
			return
		}
		for _, callee := range idx.calledStages(fd.Body) {
			if seen[callee] {
				continue
			}
			seen[callee] = true
			out = append(out, callee)
			walk(callee)
		}
	}
	walk(root)
	return out
}

func distinctInOrder(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// requireV5Fixture is arm D as a PRECONDITION of arms A–C: the world every gate in this round
// reasons about must actually commit a v5 block and reach the composition. Forcing
// b.Version = BlockVersionRounds in the fixture must fail this, and with it A, B and C.
func requireV5Fixture(t *testing.T) {
	t.Helper()
	f := buildStructFixture(t)
	committed := f.c.Blocks(1)
	if len(committed) == 0 || committed[len(committed)-1].Version != BlockVersionWitnessable {
		t.Fatalf("SOURCE GATE: arm D — the fixture did not commit a v%d block; arms A–C would be vacuous", BlockVersionWitnessable)
	}
	b := f.mkBlock(t, nil)
	if out, err := ValidateCommitV5(liveView{f.c}, &b); out != Accept {
		t.Fatalf("SOURCE GATE: arm D — the fixture's block does not reach Accept in the composition (%s / %v); arms A–C would be vacuous", out, err)
	}
}

// ---------------------------------------------------------------------------
// arm A — CALL COVER, derived, ordered, transitive (G-D2)
// ---------------------------------------------------------------------------

// TestStageCover_ArmA_DerivedCallCover derives the node's stage list and asserts nodeStages equals
// it — element for element, including order — then asserts the composition carries every row.
// Ablation (G-D2): delete one stage call from the composition ⇒ RED.
// RUNTIME GATE: TestGD5_IssuerKeysOnlyBlockParity (the stage the drift dropped, driven).
func TestStageCover_ArmA_DerivedCallCover(t *testing.T) {
	requireV5Fixture(t)
	node := parseIndex(t, nodeStageFiles)
	comp := compositionIndex(t)

	for _, root := range []string{"ValidateProposal", "ValidateCommit", "requireQuorumStack"} {
		fd := node.decls[root]
		if fd == nil || fd.Recv == nil || !chainReceiver(fd.Recv.List[0].Type) {
			t.Fatalf("SOURCE GATE: root %s is not a *Chain method in %v", root, nodeStageFiles)
		}
	}

	// ---- the P section: ValidateProposal's direct stages, distinct, in order ----
	derivedP := distinctInOrder(node.calledStages(node.decls["ValidateProposal"].Body))
	if len(derivedP) < 9 {
		t.Fatalf("SOURCE GATE: arm A VACUOUS — derived only %d error-returning stages from ValidateProposal (floor 9): %v", len(derivedP), derivedP)
	}
	var tableP []string
	for _, row := range nodeStages {
		if strings.HasPrefix(row.ID, "P") && row.Node != "" {
			tableP = append(tableP, row.Node)
		}
	}
	tableP = distinctInOrder(tableP)
	if !equalStrings(derivedP, tableP) {
		t.Fatalf("SOURCE GATE: arm A — the node's ValidateProposal stage list (derived) differs from nodeStages (declared).\n"+
			"  derived:  %v\n  declared: %v\n"+
			"  The node's accept path changed. Re-derive the P-table (certification §1), update the composition in\n"+
			"  validate_v5.go, then update nodeStages in the SAME commit.", derivedP, tableP)
	}

	// ---- the C section: ValidateCommit's call SITES, in order, duplicates kept ----
	derivedC := node.calledStages(node.decls["ValidateCommit"].Body)
	if len(derivedC) == 0 || derivedC[0] != "ValidateProposal" {
		t.Fatalf("SOURCE GATE: arm A — ValidateCommit's first stage must be ValidateProposal (C0); derived %v", derivedC)
	}
	var tableC []string
	for _, row := range nodeStages {
		if strings.HasPrefix(row.ID, "C") {
			tableC = append(tableC, row.Node)
		}
	}
	rest := derivedC[1:]
	if len(rest) < len(tableC) || !equalStrings(rest[:len(tableC)], tableC) {
		t.Fatalf("SOURCE GATE: arm A — ValidateCommit's stage sites (after ValidateProposal) do not start with nodeStages C1..C5.\n"+
			"  derived:  %v\n  declared: %v", rest, tableC)
	}
	// The era-1 leg (v1 only, unreachable for v5) may only re-use the C-row stages. Anything else
	// after the v2 branch is a stage the composition does not carry.
	cNames := map[string]bool{}
	for _, n := range tableC {
		cNames[n] = true
	}
	for _, n := range rest[len(tableC):] {
		if !cNames[n] {
			t.Fatalf("SOURCE GATE: arm A — ValidateCommit calls %s outside the C1..C5 rows (the era-1 leg may only re-use them); derived %v", n, rest)
		}
	}
	// Distinct error-returning stages reachable from ValidateCommit besides ValidateProposal.
	var besides []string
	for _, n := range node.closure("ValidateCommit") {
		if n != "ValidateProposal" && !contains(node.closure("ValidateProposal"), n) {
			besides = append(besides, n)
		}
	}
	if len(besides) < 5 {
		t.Fatalf("SOURCE GATE: arm A VACUOUS — only %d distinct stages reachable from ValidateCommit besides ValidateProposal (floor 5): %v", len(besides), besides)
	}

	// ---- the Q section: requireQuorumStack's nested stages ----
	derivedQ := distinctInOrder(node.calledStages(node.decls["requireQuorumStack"].Body))
	var tableQ []string
	for _, row := range nodeStages {
		if strings.HasPrefix(row.ID, "Q") && row.Node != "" {
			tableQ = append(tableQ, row.Node)
		}
	}
	if !equalStrings(derivedQ, tableQ) {
		t.Fatalf("SOURCE GATE: arm A — requireQuorumStack's stage list (derived) differs from nodeStages Q3/Q4.\n  derived: %v\n  declared: %v", derivedQ, tableQ)
	}

	// ---- transitive: each row's Nested equals the node function's derived nested closure ----
	var checkNested func(rows []nodeStage, path string)
	checkNested = func(rows []nodeStage, path string) {
		for _, row := range rows {
			if row.Node == "" {
				continue
			}
			want := make([]string, 0, len(row.Nested))
			for _, n := range row.Nested {
				want = append(want, n.Node)
			}
			got := distinctInOrder(node.calledStages(node.decls[row.Node].Body))
			if !equalStrings(got, want) {
				t.Fatalf("SOURCE GATE: arm A — %s%s (%s) reaches nested stages %v; nodeStages declares %v", path, row.ID, row.Node, got, want)
			}
			checkNested(row.Nested, path+row.ID+"/")
		}
	}
	checkNested(nodeStages, "")

	// ---- cover: every row is carried by the composition ----
	compReach := map[string]bool{}
	for _, n := range comp.closure("ValidateCommitV5") {
		compReach[n] = true
	}
	compReach["ValidateProposalV5"] = true
	var substituted []string
	var checkCover func(rows []nodeStage)
	checkCover = func(rows []nodeStage) {
		for _, row := range rows {
			switch {
			case row.Substituted != "":
				substituted = append(substituted, row.Node)
			case row.Mirror != "":
				if !compReach[row.Mirror] {
					t.Fatalf("SOURCE GATE: arm A COVER — node stage %s (%s) is not carried: the composition never calls its mirror %s "+
						"(transitively from ValidateCommitV5). A validity rule the node enforces is missing from the v5 path.",
						row.ID, row.Node, row.Mirror)
				}
			case row.Node != "":
				// A derived node stage with neither a mirror nor a substitution — only legal as the
				// nested internals of the substituted step.
				if row.Node != "postApplyRoots" {
					t.Fatalf("SOURCE GATE: arm A — nodeStages row %s (%s) has no Mirror and no Substituted", row.ID, row.Node)
				}
			}
			checkCover(row.Nested)
		}
	}
	checkCover(nodeStages)
	if ds := distinctInOrder(substituted); len(ds) != 1 || ds[0] != "validateEra3Roots" {
		t.Fatalf("SOURCE GATE: arm A — exactly ONE node stage may be substituted (validateEra3Roots → StateView.CommittedRoots); found %v. "+
			"A second substitution is a research-gated event, not a table edit.", ds)
	}
	if !callsViewMethod(comp.decls["ValidateProposalV5"].Body, "CommittedRoots") {
		t.Fatal("SOURCE GATE: arm A — ValidateProposalV5 does not call v.CommittedRoots(b): the substituted step is not wired")
	}
	if len(nodeStages) != 24 {
		t.Fatalf("SOURCE GATE: arm A — nodeStages has %d rows; the certification's table has 24", len(nodeStages))
	}
}

func containsName(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func callsViewMethod(body *ast.BlockStmt, method string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "v" && sel.Sel.Name == method {
				found = true
			}
		}
		return true
	})
	return found
}

// ---------------------------------------------------------------------------
// arm B — BODY DIGEST, change detection over exactly three functions (G-D3)
// ---------------------------------------------------------------------------

// nodeAcceptPathDigest pins the comment-free bodies of ValidateProposal, ValidateCommit and
// requireQuorumStack — the three functions whose INLINE predicates (P1–P5, Q1–Q2) and stage
// ORDER the composition mirrors by hand. Widening it to the closure would redden on an
// error-message tweak inside validateBondRegs and be turned off within a month; that
// calibration is the certification's, and its cost (an inline change INSIDE a called stage is
// unseen) is R-PTABLE-DRIFT's stated bound.
const nodeAcceptPathDigest = "42b27dc5d6485dca09653d6df8969aa2e5df2e9c241a3c7bdc01bdea4172319a"

// TestStageCover_ArmB_NodeBodyDigest is a CHANGE DETECTOR, not a cover proof.
// Ablation (G-D3): add `&& true` to the P5 clause chain in ValidateProposal ⇒ RED.
// RUNTIME GATE: TestGD5_IssuerKeysOnlyBlockParity (the P5 clause, driven).
func TestStageCover_ArmB_NodeBodyDigest(t *testing.T) {
	requireV5Fixture(t)
	node := parseIndex(t, nodeStageFiles)
	got := nodeBodyDigest(t, node, "ValidateProposal", "ValidateCommit", "requireQuorumStack")
	if got != nodeAcceptPathDigest {
		t.Fatalf("SOURCE GATE: arm B — the comment-free bodies of ValidateProposal + ValidateCommit + requireQuorumStack "+
			"hash to %s, pinned %s.\n"+
			"  The node's accept path changed — re-derive the P-table (FLOORBOX-STRUCTURE-P-TABLE-DRIFT-DELTA-CERTIFICATION "+
			"§1), update the composition (core/chain/validate_v5.go and its mirrors), then update this digest in the SAME commit.",
			got, nodeAcceptPathDigest)
	}
}

func nodeBodyDigest(t *testing.T, idx funcIndex, names ...string) string {
	t.Helper()
	h := sha256.New()
	for _, name := range names {
		fd := idx.decls[name]
		if fd == nil {
			t.Fatalf("SOURCE GATE: arm B — %s not found in %v", name, nodeStageFiles)
		}
		var buf strings.Builder
		if err := printer.Fprint(&buf, idx.fset, fd.Body); err != nil {
			t.Fatalf("SOURCE GATE: arm B — print %s: %v", name, err)
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write([]byte(buf.String()))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ---------------------------------------------------------------------------
// arm C — REVERSE COVER (G-D4)
// ---------------------------------------------------------------------------

// TestStageCover_ArmC_ReverseCover: every error-returning call the composition makes, transitively
// from ValidateCommitV5, is a stage mirror in nodeStages, a listed helper mirror of a node function
// that exists, or the proposal entry. Ablation (G-D4): add an invented stage call ⇒ RED.
// RUNTIME GATE: TestGD6_LegacyModeParity (an invented stage that refuses would break parity).
func TestStageCover_ArmC_ReverseCover(t *testing.T) {
	requireV5Fixture(t)
	node := parseIndex(t, nodeStageFiles)
	comp := compositionIndex(t)

	admitted := map[string]string{}
	var collect func(rows []nodeStage)
	collect = func(rows []nodeStage) {
		for _, row := range rows {
			if row.Mirror != "" {
				admitted[row.Mirror] = "stage mirror of " + row.Node
			}
			collect(row.Nested)
		}
	}
	collect(nodeStages)
	for _, h := range compositionHelpers {
		if h.node != "" {
			if _, ok := node.decls[h.node]; !ok {
				t.Fatalf("SOURCE GATE: arm C — helper %s claims to mirror node function %s, which does not exist in %v", h.helper, h.node, nodeStageFiles)
			}
		}
		admitted[h.helper] = h.why
	}
	reached := comp.closure("ValidateCommitV5")
	if len(reached) < 20 {
		t.Fatalf("SOURCE GATE: arm C VACUOUS — only %d error-returning calls reachable from ValidateCommitV5: %v", len(reached), reached)
	}
	for _, name := range reached {
		if _, ok := admitted[name]; !ok {
			t.Fatalf("SOURCE GATE: arm C — the composition calls %s, which is neither a stage mirror in nodeStages nor a listed "+
				"helper (compositionHelpers). An INVENTED stage would make the box refuse where the node accepts — legal by the "+
				"implication, but it is not the node's rule and it is not in the certified table. Name it or remove it.", name)
		}
	}
}
