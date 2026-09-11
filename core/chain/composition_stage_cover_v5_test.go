package chain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
// RUNTIME GATE: TestGD5_IssuerKeysOnlyBlockParity and TestGD6_LegacyRepLegOracle
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
	{"v5RequiredQuorum", "RequiredQuorum", "Q1's count floor by regime (#380 M-380-3: the row Q1 carries Node \"\" because RequiredQuorum returns int, not error, so arm A cannot derive it; this row is what makes G-D13 pin its body)"},
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
	assertHonestTwinAccepts(t, f.c, f.mkBlock(t, nil)) // arm D is the honest twin (NG-2)
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
const nodeAcceptPathDigest = "7af9cc783e4921ddbac0158f99ebcd47f7219b3430b0842e99763ed2906eb279"

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
// RUNTIME GATE: TestGD6_LegacyRepLegOracle (an invented stage that refuses would break parity).
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

// ---------------------------------------------------------------------------
// G-D13 — PER-ROW NODE-BODY DIGESTS (M-1A-5; research certification
// FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e §3.1, option (2))
// ---------------------------------------------------------------------------

// nodeBodyDigests pins the sha256 of the comment-free body of EVERY node function that nodeStages
// (validate_v5.go) or compositionHelpers names as mirrored by the composition — the ~34 bodies
// arm B's three-function scope does not see. One digest per node function; the failure names
// every table row that mirrors it, so a red says exactly which mirror to re-derive.
//
// THE SAME-COMMIT RULE. A change inside any of these bodies is an accept-path change for a v5
// block that the composition does NOT carry until its mirror is re-derived. The correct action on
// a red is: re-derive the named mirror against the new body line by line, then update the digest
// in the SAME commit. Updating the digest alone is the era-keyed consensus-rule split this gate
// exists to catch (R-PTABLE-DRIFT, re-scoped 2026-09-08 to "~34 bodies covered by nothing").
//
// The table lives here, beside arm B's digest, not in the production nodeStages table: a
// production table carries no test pins, and the attribution the certification asked for is
// preserved because the gate names the rows.
var nodeBodyDigests = map[string]string{
	"ValidateEntry":              "f6844363fb7212b625f60ca726f3d361b7b3eaca7f4af22efb33703c2f5bf3e5",
	"RequiredQuorum":             "c3a413ba707abae6cfdd40806fe5c195a79a2c4a7e4fa2939d8ee1cb92257454",
	"ValidateProposal":           "752294cf1e8cbb2ab9c9830ac4e35f68eca9f9a2389da5f2e1e3ea322e1bc3de",
	"attesterQualifiedAt":        "fd5de208dcecba70d1fcaf32172ef185c20b12020413ba7fff0ddd04fef3723f",
	"collectQuorumSigs":          "c0b0a3aa3f475f6bcd0e7621b07d0005d575a21fadcc7e3f6aa457b0d634e603",
	"effectiveEpochSet":          "8da6023fc1b8b0e5482b8f8c275ad9d93a7db8edf304750fdd15e6bfca4d292a",
	"epochsEnabled":              "d60a5734d37abe1722fcc7f4dc6f42a510eaa6bd04d56b0ab4975fa48ed50f18",
	"era3Active":                 "883666c754351a281949e98123de50205e93d7114e02ea4226e56448cc8206d2",
	"handedOff":                  "07e38df939a4eadebcb2decd5d90589fe3b8d409502125fb677a17aea77a53b1",
	"matureNow":                  "1ae3776705326b6589ed476873070414d3f733d9df6f9480098e7bfda1d8a41e",
	"postApplyRoots":             "cf7fec5d6d0ec420f664095e1352cf3b1700998284b23b3cae8137874ba2d531",
	"recentBondRegNonces":        "ba7a00623b0f6c0d94b95b73921a0e91f66b983d2f4d42e29cd779a46d42454d",
	"regGateActive":              "4d369be6273149966e31991f923749eef5d1b2162b0d865f78a10ed4a0a5ee72",
	"requireDeMatureSuperQuorum": "0470237c9c4760af6a9059e9da5fdd526855d7dc0ad1fa658c4789bb22a001e1",
	"requireEpochWeightQuorum":   "11ea038dd07b4b1a3bf05f9b744b6e4dbddebdf199d2617f552abcb5bb5c7982",
	"requireProposerPrepare":     "bac6ab02cdbaf5dda5b8af8f68d251c9e31e1c870b4210d002dab9d64bb8313b",
	"requireQuorumStack":         "b5553c3f61494bdd91506782f56bd25ddb778a7bf522eee10dd6af7932633138",
	"restoresHeldStanding":       "1dd120f427f6eb4c3f93103afa61770b4bf2f03153c5bfc97596d04e1263d499",
	"validateBondReg":            "595b695e1d665885b4de70585fe94ed4b09654ce956b6b08fe712665b80f3bf6",
	"validateBondRegWindow":      "cceaf238101c9a751ebba1bfcb406ce01c90387d03fc62fdd8015b5e98ad7745",
	"validateBondRegs":           "44a7b37852f162e47a7b409222f86251d846c959163659f07ac2d758fb39c08d",
	"validateCarrier":            "e5cf4078a9d633fca3e977457e5c40fb203c2f6b49ab634ff9c8bc83faf2fbe2",
	"validateEra3Roots":          "a7ec421d2ea91224cec48dc7cfe34f935908fdf0bc46d8e3dcbd8addf6823b92",
	"validateEra3Version":        "1ff98cdc43e91fb10dc080c41a0c788beb6b8ddc06a87461a7fceea55fe4dc0b",
	"validateEra4Version":        "9a1a52aa6d4d5182afddeae6b3b3da3fb6b1b93e1a517b6b026c687afd33edac",
	"validateIssuerKeys":         "801f04536b8794a5fca36adb65e76c58589c677da378e42da613fed6af75f324",
	"validateSlashes":            "c432026ba5fd3dc175f3d0c5026dfe28021921706baa6d5e18ead30184730e57",
	"validateTakedowns":          "4250eff489a8869f843815a411c1351ff9ba616c31c4c0a3a7f5d7637e1d52aa",
	"validatorSetSize":           "393c775422214e528015d9833602d1ee7b6a58e4e5da45c2f339d604f4aae919",
}

// TestGD13_PerRowNodeBodyDigests: every node function named by a nodeStages row (transitively) or
// by a compositionHelpers entry has a pinned digest, the digest matches, and there are at least 24
// of them (so nobody empties the table). Ablation (G-D13): add `&& true` to a clause inside
// validateBondRegs ⇒ RED naming P7 / v5ValidateBondRegs.
// SOURCE GATE: a body digest is a CHANGE DETECTOR, not a cover proof. RUNTIME GATE:
// TestM1A3_V4V5ParityOracle (the v4 bodies against the v5 mirrors across the regimes).
func TestGD13_PerRowNodeBodyDigests(t *testing.T) {
	requireV5Fixture(t)
	node := parseIndex(t, nodeStageFiles)

	// node function → the rows / helpers that mirror it.
	mirroredBy := map[string][]string{}
	var collect func(rows []nodeStage, path string)
	collect = func(rows []nodeStage, path string) {
		for _, row := range rows {
			label := path + row.ID
			if row.ID == "" {
				label = path + "nested:" + row.Node
			}
			if row.Node != "" {
				what := row.Mirror
				if row.Substituted != "" {
					what = "substituted → StateView.CommittedRoots"
				}
				mirroredBy[row.Node] = append(mirroredBy[row.Node], label+" ("+what+")")
			}
			collect(row.Nested, label+"/")
		}
	}
	collect(nodeStages, "")
	for _, h := range compositionHelpers {
		if h.node != "" {
			mirroredBy[h.node] = append(mirroredBy[h.node], "helper "+h.helper)
		}
	}

	var names []string
	for n := range mirroredBy {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) < 24 {
		t.Fatalf("SOURCE GATE: G-D13 VACUOUS — only %d mirrored node functions derived from nodeStages + compositionHelpers (floor 24)", len(names))
	}
	var problems []string
	for _, n := range names {
		got := nodeBodyDigest(t, node, n)
		want, pinned := nodeBodyDigests[n]
		switch {
		case !pinned:
			problems = append(problems, fmt.Sprintf("  %s: NOT PINNED — mirrored by %v; add\n      %q: %q,", n, mirroredBy[n], n, got))
		case got != want:
			problems = append(problems, fmt.Sprintf("  %s: body CHANGED — hashes to %s, pinned %s; mirrored by %v", n, got[:16], want[:16], mirroredBy[n]))
		}
	}
	for n := range nodeBodyDigests {
		if _, ok := mirroredBy[n]; !ok {
			problems = append(problems, fmt.Sprintf("  %s: pinned but no nodeStages row or compositionHelpers entry mirrors it — delete the pin or restore the row", n))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("SOURCE GATE: G-D13 — a node body the composition MIRRORS changed (or is unpinned):\n%s\n\n"+
			"  Re-derive the named mirror(s) against the new body line by line (P-table delta certification §1;\n"+
			"  composed-diff certification §3.1), then update nodeBodyDigests in the SAME commit. Updating the\n"+
			"  digest alone splits the v5 accept path from the v2/v4 path — the era-keyed consensus-rule split\n"+
			"  R-PTABLE-DRIFT names.", strings.Join(problems, "\n"))
	}
}
