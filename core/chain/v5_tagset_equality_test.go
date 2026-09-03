package chain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// R-V5-TAGSET-EQUALITY (ROADMAP; composed-direction cert CD-0, 2026-09-03).
//
// statehash.go DECLARES the committed leaf tags (the `tag*` string constants, each
// mirrored by name in stateRootTags / stateRootTagsV5 / stateRootDigestTagsV5). The
// floor-box side — the read-set producer (readset_v5.go) and the recomputes
// (floorbox*.go) — CONSUMES them by identifier. There is no producer-side table: the
// producer references tags ad hoc, so "the set the box knows" is DERIVED here as the
// set of `tag*` identifiers those files reference. The carrier branch's statehash.go
// (base 1adca0f) has 24 tag constants and a five-entry stateRootTagsV5; main has 29
// and six (issuerKeyCommit joined in #711). A merge that resolves statehash.go toward
// the carrier drops a committed keyspace from the declared set while the emit-guard
// list and the box's references still name it — this gate reddens on that.
//
// Existing pins this one composes with rather than duplicates:
//   - issuerkey_rollout_gate_test.go:57 pins "issuerKeyCommit" ∈ stateRootTagsV5;
//   - TestStateRootV5CoversExactlyTheV5Fields binds the three runtime lists to what
//     stateRootLeavesV5 EMITS (runtime);
//   - readset_v5_drift_test.go v5CommittedKeyspaceTags() is the test-side closed set
//     of the 23 member keyspaces the execution-derived read-set guard iterates.
//
// v5TagsNotReadByBox is the TIGHT allowlist of declared tags no box-side file may
// reference. An entry whose tag becomes referenced must be removed (the test fails
// until it is), and an entry naming an undeclared tag fails too.
var v5TagsNotReadByBox = map[string]string{
	"tagIssuerKey": "R0.4b per-epoch issuer-key leaf: nothing folds this keyspace and no validity predicate, quorum or fork-choice rule reads it (statehash.go tagIssuerKey doc; chain.go applyIssuerKeys comment). A redeemer resolves ONE leaf by inclusion proof, outside the box.",
}

// v5TagSetSides is everything the gate compares, so the teeth can run the same
// comparison over edited inputs.
type v5TagSetSides struct {
	declared map[string]string // statehash.go const name → tag value (with trailing NUL)
	runtime  map[string]bool   // tag values (without NUL) from the three runtime lists
	boxRefs  map[string]bool   // tag const names referenced by box-side files
	allow    map[string]string // v5TagsNotReadByBox
}

// v5TagSetGaps is the pure comparison. Every returned string is one failure.
func v5TagSetGaps(s v5TagSetSides) []string {
	var gaps []string
	// (i) declared consts ⇔ runtime lists, by value.
	declaredValues := map[string]string{}
	for name, val := range s.declared {
		declaredValues[strings.TrimSuffix(val, "\x00")] = name
	}
	for val := range declaredValues {
		if !s.runtime[val] {
			gaps = append(gaps, "declared tag const "+declaredValues[val]+" ("+strconv.Quote(val)+") is in none of stateRootTags / stateRootTagsV5 / stateRootDigestTagsV5")
		}
	}
	for val := range s.runtime {
		if _, ok := declaredValues[val]; !ok {
			gaps = append(gaps, "runtime list names "+strconv.Quote(val)+" but statehash.go declares no tag constant with that value")
		}
	}
	// (ii) box references ⊆ declared (no phantom tag on the box side).
	for name := range s.boxRefs {
		if _, ok := s.declared[name]; !ok {
			gaps = append(gaps, "box-side file references "+name+", which statehash.go does not declare (a leaf the box reads that the root never commits)")
		}
	}
	// (iii) declared \ boxRefs == allow, exactly.
	for name := range s.declared {
		if s.boxRefs[name] {
			if _, listed := s.allow[name]; listed {
				gaps = append(gaps, "allowlist entry "+name+" is STALE: a box-side file now references it; delete the entry")
			}
			continue
		}
		if _, listed := s.allow[name]; !listed {
			gaps = append(gaps, "declared tag "+name+" is referenced by NO box-side file and is not in v5TagsNotReadByBox — either the producer lost a read, or add the entry with its reason")
		}
	}
	for name := range s.allow {
		if _, ok := s.declared[name]; !ok {
			gaps = append(gaps, "allowlist entry "+name+" names a tag statehash.go does not declare")
		}
	}
	sort.Strings(gaps)
	return gaps
}

// statehashDeclaredTags parses statehash.go text and returns name → value for every
// `tag*` string constant.
func statehashDeclaredTags(src string) (map[string]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "statehash.go", src, 0)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, n := range vs.Names {
				if !strings.HasPrefix(n.Name, "tag") || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					return nil, err
				}
				out[n.Name] = val
			}
		}
	}
	return out, nil
}

// boxSideTagRefs parses each source and returns the set of `tag[A-Z]…` identifiers
// referenced (comments are not identifiers, so a mention in prose does not count).
func boxSideTagRefs(srcs map[string]string) (map[string]bool, error) {
	out := map[string]bool{}
	for name, src := range srcs {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			return nil, err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && len(id.Name) > 3 && strings.HasPrefix(id.Name, "tag") && id.Name[3] >= 'A' && id.Name[3] <= 'Z' {
				out[id.Name] = true
			}
			return true
		})
	}
	return out, nil
}

// boxSideFiles enumerates the box-side sources by DIRECTORY LISTING, not a glob
// pattern: every non-test .go file whose name starts with "floorbox" or "readset".
// (scar-ast-pin-glob-misses-the-defect-file: a hand glob covered 10 of 15 files.)
func boxSideFiles(t *testing.T) map[string]string {
	t.Helper()
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		if !strings.HasPrefix(n, "floorbox") && !strings.HasPrefix(n, "readset") {
			continue
		}
		src, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		out[n] = string(src)
	}
	for _, must := range []string{"readset_v5.go", "floorbox_recompute_v5.go", "floorbox_recompute_stateroot_v5.go"} {
		if _, ok := out[must]; !ok {
			t.Fatalf("SOURCE GATE: box-side enumeration did not find %s — the file set this gate scans has lost its anchor", must)
		}
	}
	return out
}

func runtimeTagValues() map[string]bool {
	out := map[string]bool{}
	for _, list := range [][]string{stateRootTags, stateRootTagsV5, stateRootDigestTagsV5} {
		for _, v := range list {
			out[v] = true
		}
	}
	return out
}

func liveV5TagSetSides(t *testing.T) v5TagSetSides {
	t.Helper()
	src, err := os.ReadFile("statehash.go")
	if err != nil {
		t.Fatal(err)
	}
	declared, err := statehashDeclaredTags(string(src))
	if err != nil {
		t.Fatal(err)
	}
	refs, err := boxSideTagRefs(boxSideFiles(t))
	if err != nil {
		t.Fatal(err)
	}
	return v5TagSetSides{declared: declared, runtime: runtimeTagValues(), boxRefs: refs, allow: v5TagsNotReadByBox}
}

// TestV5TagSetEqualityAcrossStatehashAndBox is the R-V5-TAGSET-EQUALITY source gate.
//
// SOURCE GATE: it reads statehash.go and the floorbox*/readset* sources as text and
// compares (i) the declared tag constants to the three runtime tag lists by value,
// (ii) the box side's referenced tag identifiers to the declared set, and (iii) the
// declared-but-unreferenced remainder to the tight v5TagsNotReadByBox allowlist. It
// cannot see whether a referenced tag is read on a live path. RUNTIME GATE:
// TestStateRootV5CoversExactlyTheV5Fields (the leaves stateRootLeavesV5 emits equal the
// lists) and TestWitnessReadSetV5ExecutionDerivedGuard (the producer's reads equal the
// recompute's execution-derived read-set).
func TestV5TagSetEqualityAcrossStatehashAndBox(t *testing.T) {
	s := liveV5TagSetSides(t)
	for _, g := range v5TagSetGaps(s) {
		t.Error("SOURCE GATE: " + g)
	}
	// The CD-0 tag by name: issuerKeyCommit must be declared AND in stateRootTagsV5
	// (issuerkey_rollout_gate_test.go pins the list side; this pins the const side).
	if v, ok := s.declared["tagIssuerKey"]; !ok || v != "issuerKeyCommit\x00" {
		t.Errorf("SOURCE GATE: statehash.go no longer declares tagIssuerKey = \"issuerKeyCommit\\x00\" (got %q, present=%v) — the carrier base predates #711 and a merge toward it drops the keyspace", v, ok)
	}
	if !s.runtime["issuerKeyCommit"] {
		t.Error("SOURCE GATE: \"issuerKeyCommit\" is in none of the runtime tag lists — stateRootTagsV5 was resolved toward the carrier's five-entry list")
	}
	// The drift test's closed member set ∪ the digest roots must equal what the box
	// references, so that test-side enumeration cannot drift from the production side.
	closed := map[string]bool{}
	nameByValue := map[string]string{}
	for name, val := range s.declared {
		nameByValue[val] = name
	}
	for _, tag := range v5CommittedKeyspaceTags() {
		closed[nameByValue[tag]] = true
	}
	for _, d := range stateRootDigestTagsV5 {
		closed[nameByValue[d+"\x00"]] = true
	}
	for name := range s.boxRefs {
		if !closed[name] {
			t.Errorf("SOURCE GATE: box-side files reference %s but readset_v5_drift_test.go's closed set (v5CommittedKeyspaceTags ∪ stateRootDigestTagsV5) does not include it — the red-on-drop guard never iterates that keyspace", name)
		}
	}
	for name := range closed {
		if name != "" && !s.boxRefs[name] {
			t.Errorf("SOURCE GATE: the drift test's closed set names %s but no box-side file references it", name)
		}
	}
	t.Logf("declared=%d runtime=%d boxRefs=%d allow=%d", len(s.declared), len(s.runtime), len(s.boxRefs), len(s.allow))
}

// TestV5TagSetEqualityHasTeeth runs the SAME comparison over edited inputs: the
// carrier-shaped statehash.go (tagIssuerKey deleted), a box file referencing a
// phantom tag, and a stale allowlist entry. Each must produce a gap.
//
// SOURCE GATE: exercises v5TagSetGaps on edited text only. RUNTIME GATE:
// TestStateRootV5CoversExactlyTheV5Fields.
func TestV5TagSetEqualityHasTeeth(t *testing.T) {
	live := liveV5TagSetSides(t)
	if g := v5TagSetGaps(live); len(g) != 0 {
		t.Fatalf("SOURCE GATE: precondition — the live sides must be gap-free before the teeth are meaningful: %v", g)
	}
	src, _ := os.ReadFile("statehash.go")

	// Teeth 1: the carrier shape — drop the tagIssuerKey constant.
	edited := strings.Replace(string(src), "tagIssuerKey = \"issuerKeyCommit\\x00\"", "", 1)
	if edited == string(src) {
		t.Fatal("SOURCE GATE: teeth fixture — tagIssuerKey declaration not found verbatim; update the teeth")
	}
	declared, err := statehashDeclaredTags(edited)
	if err != nil {
		t.Fatal(err)
	}
	s := live
	s.declared = declared
	if g := v5TagSetGaps(s); !anyContains(g, "issuerKeyCommit") || !anyContains(g, "tagIssuerKey") {
		t.Errorf("SOURCE GATE: TEETH FAILED — deleting tagIssuerKey from statehash.go produced no gap naming it: %v", g)
	}

	// Teeth 2: a box file referencing an undeclared tag (CD-1's forbidden tagPrevHash).
	refs, err := boxSideTagRefs(map[string]string{"phantom.go": "package chain\nvar _ = tagPrevHash\n"})
	if err != nil {
		t.Fatal(err)
	}
	s = live
	s.boxRefs = map[string]bool{}
	for k := range live.boxRefs {
		s.boxRefs[k] = true
	}
	for k := range refs {
		s.boxRefs[k] = true
	}
	if g := v5TagSetGaps(s); !anyContains(g, "tagPrevHash") {
		t.Errorf("SOURCE GATE: TEETH FAILED — a box-side reference to undeclared tagPrevHash produced no gap: %v", g)
	}

	// Teeth 3: a stale allowlist entry (the tag becomes referenced).
	s = live
	s.boxRefs = map[string]bool{}
	for k := range live.boxRefs {
		s.boxRefs[k] = true
	}
	s.boxRefs["tagIssuerKey"] = true
	if g := v5TagSetGaps(s); !anyContains(g, "STALE") {
		t.Errorf("SOURCE GATE: TEETH FAILED — a referenced tag still on the allowlist produced no STALE gap: %v", g)
	}

	// Teeth 4: a declared tag the box stops referencing (a lost producer read).
	s = live
	s.boxRefs = map[string]bool{}
	for k := range live.boxRefs {
		if k != "tagBonded" {
			s.boxRefs[k] = true
		}
	}
	if g := v5TagSetGaps(s); !anyContains(g, "tagBonded") {
		t.Errorf("SOURCE GATE: TEETH FAILED — dropping every box-side tagBonded reference produced no gap: %v", g)
	}
}

func anyContains(xs []string, sub string) bool {
	for _, x := range xs {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}
