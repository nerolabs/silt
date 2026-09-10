package chain

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// R-HASH-LITERAL-PIN (ROADMAP; composed-direction cert CD-0, 2026-09-03).
//
// Block.bodyHash builds the signed preimage from a hand-written struct literal
// (`unsigned := Block{...}`). That literal is the ONE place a Block field becomes
// hash-covered: a field omitted there is attacker-writable in transit under a valid
// proposer signature. The LastCommit carrier branch (base 1adca0f) folds LastCommit
// and not IssuerKeys; main folds IssuerKeys and not LastCommit; the cbor keys are
// disjoint (17 / 18) so the FIELD merge is clean and the LITERAL merge is where the
// hole opens. This pin makes that hole a red test instead of a review finding.
//
// The five deliberate exclusions are the fields set on the COMMITTED copy after
// Hash-identity has done its work (CommitRound, PrepareQC, Atts, ProposerSig — see the
// Block field docs) plus Pruned (the pre-prune hash itself). The allowlist is TIGHT:
// an exclusion naming a field that no longer exists is itself a failure.
var hashPreimageExclusions = map[string]string{
	"Atts":        "the precommit certificate, set on the committed copy after Hash-identity (Block.Atts doc)",
	"CommitRound": "the round is not part of the value's identity (Block.CommitRound doc)",
	"PrepareQC":   "the prepare certificate, excluded from Hash like Atts (Block.PrepareQC doc)",
	"ProposerSig": "the signature over the hash cannot be inside the hash",
	"Pruned":      "the pre-prune Hash carried by a pruned block; Hash() returns it, bodyHash never reads it",
}

// hashCoveredFields is the set of exported Block fields that MUST appear in the
// bodyHash literal: every exported field minus the exclusions. Derived by reflection
// so a newly added field is in the expected set the moment it is declared.
func hashCoveredFields(t *testing.T) []string {
	t.Helper()
	bt := reflect.TypeOf(Block{})
	exported := map[string]bool{}
	var out []string
	for i := 0; i < bt.NumField(); i++ {
		f := bt.Field(i)
		if !f.IsExported() {
			continue // hashMemo / hashMemoSet: never on the wire, never in the preimage
		}
		exported[f.Name] = true
		if _, excluded := hashPreimageExclusions[f.Name]; excluded {
			continue
		}
		out = append(out, f.Name)
	}
	for name := range hashPreimageExclusions {
		if !exported[name] {
			t.Errorf("SOURCE GATE: hashPreimageExclusions names %q, which is not an exported Block field — a stale exclusion would let a renamed field silently drop out of the preimage; remove or rename the entry", name)
		}
	}
	sort.Strings(out)
	return out
}

// hashLiteralKeySets parses Go source text and returns ONE key set PER composite literal
// assigned to `unsigned` inside `func (b *Block) bodyHash`.
//
// PER-LITERAL, NOT UNIONED — this is the (d-3) re-point (2026-09-10, owner precondition;
// D3-ANSWERDIGEST-TWO-LEVEL-HASH-DELTA-RESEARCH-CERTIFICATION-2026-09-10). bodyHash becomes
// VERSION-DEPENDENT: a v5 preimage substituting AnswerDigest/SlashesDigest, and the pre-v5
// preimage unchanged. The earlier implementation accumulated every literal's keys into one flat
// slice, so a field folded in the pre-v5 literal and DROPPED from the v5 one read as covered in
// both — the gate would have stayed GREEN straight through the change it exists to police. Each
// literal is now checked on its own.
func hashLiteralKeySets(src string) ([][]string, bool) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "chain.go", src, 0)
	if err != nil {
		return nil, false
	}
	var sets [][]string
	found := false
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "bodyHash" || fd.Recv == nil || fd.Body == nil {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				return true
			}
			id, ok := as.Lhs[0].(*ast.Ident)
			if !ok || id.Name != "unsigned" {
				return true
			}
			cl, ok := as.Rhs[0].(*ast.CompositeLit)
			if !ok {
				return true
			}
			found = true
			var keys []string
			for _, e := range cl.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if k, ok := kv.Key.(*ast.Ident); ok {
					keys = append(keys, k.Name)
				}
			}
			sort.Strings(keys)
			sets = append(sets, keys)
			return false
		})
	}
	return sets, found
}

// hashLiteralKeys is the UNION view, kept only for the synthetic-source teeth test, which
// edits a single-literal fixture. Never use it to judge the real chain.go: unioning is the
// defect hashLiteralKeySets exists to remove.
func hashLiteralKeys(src string) ([]string, bool) {
	sets, found := hashLiteralKeySets(src)
	var all []string
	for _, ks := range sets {
		all = append(all, ks...)
	}
	sort.Strings(all)
	return all, found
}

// hashLiteralGaps compares the literal's keys against the expected hash-covered set.
// missing = expected fields the literal does not fold (a HOLE in the signed body);
// extra = literal keys that are not expected (an exclusion folded by mistake, or a
// key naming a field that no longer exists).
func hashLiteralGaps(keys, want []string) (missing, extra []string) {
	have := map[string]bool{}
	for _, k := range keys {
		have[k] = true
	}
	wantSet := map[string]bool{}
	for _, w := range want {
		wantSet[w] = true
		if !have[w] {
			missing = append(missing, w)
		}
	}
	for _, k := range keys {
		if !wantSet[k] {
			extra = append(extra, k)
		}
	}
	return missing, extra
}

// TestHashLiteralPinsEveryHashCoveredField is the R-HASH-LITERAL-PIN source gate.
//
// SOURCE GATE: it reads chain.go as text and checks that the `unsigned := Block{...}`
// literal in bodyHash names exactly the exported Block fields minus the five
// exclusions. It cannot see whether bodyHash is the function Sign/verify actually
// call. RUNTIME GATE: TestHashLiteralPinRuntimePair (below) mutates every field on a
// real block and observes bodyHash() move or hold; the signature-verification tests
// (TestCommitAndReplay, TestEquivocationProof, the appendStructural reload tests)
// observe that a moved hash breaks the proposer signature.
func TestHashLiteralPinsEveryHashCoveredField(t *testing.T) {
	src, err := os.ReadFile("chain.go")
	if err != nil {
		t.Fatal(err)
	}
	sets, found := hashLiteralKeySets(string(src))
	if !found {
		t.Fatal("SOURCE GATE: no `unsigned := Block{...}` composite literal inside func (b *Block) bodyHash in chain.go — the pin has lost its anchor; if the preimage construction moved, move this gate with it")
	}
	want := hashCoveredFields(t)

	// EVERY literal is judged on its own. When bodyHash becomes version-dependent for (d-3),
	// one literal per era, a hole in ANY of them is a hole in that era's signed body — and a
	// union check cannot see it.
	for i, keys := range sets {
		which := fmt.Sprintf("literal %d of %d", i+1, len(sets))
		missing, extra := hashLiteralGaps(keys, want)
		if len(missing) > 0 {
			t.Errorf("SOURCE GATE: bodyHash's `unsigned` %s does NOT fold exported Block field(s) %v — every field outside the exclusions is a HOLE in the signed body for the era that literal serves (CD-0: the carrier merge must list BOTH IssuerKeys and LastCommit). Literal keys: %v", which, missing, keys)
		}
		if len(extra) > 0 {
			t.Errorf("SOURCE GATE: bodyHash's `unsigned` %s names %v, which is either a deliberate exclusion (folding it changes every committed hash) or not a Block field at all", which, extra)
		}
		for _, must := range []string{"IssuerKeys"} {
			if !contains(keys, must) {
				t.Errorf("SOURCE GATE: %q is not in bodyHash's %s (main folds it; the carrier base did not)", must, which)
			}
		}
		if _, ok := reflect.TypeOf(Block{}).FieldByName("LastCommit"); ok && !contains(keys, "LastCommit") {
			t.Errorf("SOURCE GATE: Block has a LastCommit field but bodyHash's %s does not fold it — the carrier is unsigned (CD-0)", which)
		}
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// TestHashLiteralPinHasTeeth drives the SAME parse+compare over synthetic sources:
// one folded key removed must report it missing; an exclusion injected must report it
// extra; a field the literal has never heard of must report it missing. If any of
// these passes, the pin is a no-op.
//
// SOURCE GATE: exercises hashLiteralKeys/hashLiteralGaps on edited text only.
// RUNTIME GATE: TestHashLiteralPinRuntimePair.
func TestHashLiteralPinHasTeeth(t *testing.T) {
	src, err := os.ReadFile("chain.go")
	if err != nil {
		t.Fatal(err)
	}
	want := hashCoveredFields(t)
	sets, found := hashLiteralKeySets(string(src))
	if !found {
		t.Fatal("SOURCE GATE: precondition — the real literal must parse")
	}
	if len(sets) == 0 {
		t.Fatal("SOURCE GATE: precondition — at least one `unsigned` literal must be found")
	}
	// Teeth run against the FIRST literal. Every literal must be gap-free (the main gate asserts
	// that per-literal), so any one of them is a valid fixture.
	base := sets[0]
	for i, ks := range sets {
		if m, x := hashLiteralGaps(ks, want); len(m)+len(x) != 0 {
			t.Fatalf("SOURCE GATE: precondition — every real literal must be gap-free before the teeth are meaningful (literal %d: missing %v extra %v)", i+1, m, x)
		}
	}

	// Teeth 1: drop one folded key (the CD-0 shape: a merge loses IssuerKeys).
	dropped := strings.Replace(string(src), ", IssuerKeys: b.IssuerKeys", "", 1)
	if dropped == string(src) {
		t.Fatal("SOURCE GATE: teeth fixture — `, IssuerKeys: b.IssuerKeys` not found verbatim in the literal; update the teeth to the literal's current spelling")
	}
	dropSets, found := hashLiteralKeySets(dropped)
	if !found {
		t.Fatal("SOURCE GATE: teeth fixture — edited literal no longer parses")
	}
	// PER-LITERAL, and this is the teeth test proving its own re-point: the drop above edits ONE
	// occurrence, so a UNION view still sees IssuerKeys via the other literal and reports nothing
	// missing. That is precisely the defect hashLiteralKeySets removes — the teeth failed here on
	// the first run after bodyHash became version-dependent, which is the shape working.
	sawMissing := false
	for _, ks := range dropSets {
		if missing, _ := hashLiteralGaps(ks, want); contains(missing, "IssuerKeys") {
			sawMissing = true
		}
	}
	if !sawMissing {
		t.Errorf("SOURCE GATE: TEETH FAILED — removing IssuerKeys from a literal was not reported missing by any literal's check")
	}

	// Teeth 2: fold an exclusion (Atts) — must be reported extra. Injected at the HEAD of the
	// literal so the fixture does not depend on which field the literal happens to end with
	// (it ended with IssuerKeys before the LastCommit carrier landed, LastCommit after).
	// (d-3) made bodyHash version-dependent, so the literal is spelled `unsigned = Block{` inside
	// a branch rather than `unsigned := Block{`. Try both, and fail loudly if neither is present —
	// a teeth fixture that silently matches nothing is the decoration this file exists to prevent.
	injected := string(src)
	for _, spelling := range []string{"unsigned = Block{", "unsigned := Block{"} {
		if strings.Contains(injected, spelling) {
			injected = strings.Replace(injected, spelling, spelling+"Atts: b.Atts, ", 1)
			break
		}
	}
	if injected == string(src) {
		t.Fatal("SOURCE GATE: teeth fixture — neither `unsigned = Block{` nor `unsigned := Block{` found in bodyHash; update the teeth to the literal's current spelling")
	}
	injSets, found := hashLiteralKeySets(injected)
	if !found {
		t.Fatal("SOURCE GATE: teeth fixture — injected literal no longer parses")
	}
	sawExtra := false
	for _, ks := range injSets {
		if _, extra := hashLiteralGaps(ks, want); contains(extra, "Atts") {
			sawExtra = true
		}
	}
	if !sawExtra {
		t.Errorf("SOURCE GATE: TEETH FAILED — folding the excluded Atts into a literal was not reported extra")
	}

	// Teeth 3: a new Block field the literal does not know (the "field added, not
	// folded" shape) — must be reported missing.
	if missing, _ := hashLiteralGaps(base, append(append([]string{}, want...), "LastCommitPhantom")); !contains(missing, "LastCommitPhantom") {
		t.Errorf("SOURCE GATE: TEETH FAILED — a new field absent from the literal was not reported missing (got %v)", missing)
	}
}

// v5DigestCommitted names the fields the v5 preimage commits by DIGEST rather than by value
// ((d-3)). On v5 these must NOT move bodyHash when mutated — the preimage folds AnswerDigest and
// SlashesDigest instead — and their coverage is TRANSITIVE, enforced by the validity rules that
// require each digest to equal sha256 of its content. Mutating one makes the block INVALID, not
// differently hashed.
//
// THIS IS A NARROWING OF THE PIN AND IT IS DANGEROUS IF LEFT UNGUARDED, so it is guarded twice:
// the case below asserts these fields DO move on pre-v5 (where they are folded by value), and it
// asserts they do NOT move on v5 (so a v5 literal that quietly folds the field itself, forfeiting
// future prunability, reddens). The digest fields themselves are NOT listed here — AnswerDigest
// lives on BondReg, and SlashesDigest is folded by value and must move.
//
// RUNTIME COVER for the transitive half: the digest-consistency validity gates (G-D3-*).
var v5DigestCommitted = map[string]string{
	"Slashes": "committed by SlashesDigest on v5; validity requires SlashesDigest == sha256(canonical(Slashes))",
}

// TestHashLiteralPinRuntimePair is the RUNTIME half of R-HASH-LITERAL-PIN: for every
// exported Block field, set it to a non-zero value on a copy of a real block and
// observe bodyHash(). A hash-covered field MUST move the digest; an excluded field MUST
// NOT. This observes the behaviour the source gate can only read, on the live code
// path Sign/verify use, with no allowlist beyond the same five exclusions.
func TestHashLiteralPinRuntimePair(t *testing.T) {
	// EVERY ERA, not just one. (d-3) re-point, 2026-09-10 (owner precondition): bodyHash becomes
	// VERSION-DEPENDENT, and this gate's fixture was `Version: 1` — so it exercised exactly one
	// branch and a hole in the v5 preimage would have been invisible to it. Each era version gets
	// the full field sweep. Add a version here the moment a new era mints one.
	for _, ver := range []uint64{BlockVersionRounds, BlockVersionStateRoot, BlockVersionWitnessable} {
		t.Run(fmt.Sprintf("v%d", ver), func(t *testing.T) {
			base := Block{Version: ver, Height: 7, Prev: ports.HashBytes([]byte("prev")), Entries: []ports.Entry{entry(1)}}
			Sign(&base, key(41000))
			h0 := base.bodyHash()
			bt := reflect.TypeOf(Block{})
			for i := 0; i < bt.NumField(); i++ {
				f := bt.Field(i)
				if !f.IsExported() {
					continue
				}
				if f.Name == "Version" {
					continue // the loop variable IS the version; mutating it changes the era under test
				}
				b := base
				fv := reflect.ValueOf(&b).Elem().Field(i)
				if !setNonZero(fv) {
					t.Fatalf("runtime pair: no non-zero constructor for Block.%s (%s) — extend setNonZero", f.Name, f.Type)
				}
				moved := b.bodyHash() != h0
				_, excluded := hashPreimageExclusions[f.Name]
				_, transitive := v5DigestCommitted[f.Name]
				transitive = transitive && ver >= BlockVersionWitnessable
				switch {
				case excluded && moved:
					t.Errorf("v%d: Block.%s is a declared exclusion but mutating it MOVED bodyHash — this era's literal folds an excluded field", ver, f.Name)
				case transitive && moved:
					t.Errorf("v%d: Block.%s is declared DIGEST-COMMITTED on v5, so mutating it must NOT move bodyHash — the preimage folds its digest, not the field. It moved, so the v5 literal is folding the field itself and the payload is no longer prunable-by-digest", ver, f.Name)
				case !excluded && !transitive && !moved:
					t.Errorf("v%d: Block.%s is hash-covered by declaration but mutating it did NOT move bodyHash — a HOLE in this era's signed body", ver, f.Name)
				}
			}
		})
	}
}

// setNonZero writes a value of v's type that DIFFERS from v's current value. Returns
// false for a kind it cannot construct.
func setNonZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Uint64, reflect.Uint8, reflect.Uint32:
		v.SetUint(v.Uint() + 1) // DIFFERENT from the current value, not merely non-zero
		return true
	case reflect.Int64, reflect.Int:
		v.SetInt(v.Int() + 1)
		return true
	case reflect.Bool:
		v.SetBool(!v.Bool())
		return true
	case reflect.Array:
		if v.Len() == 0 {
			return false
		}
		return setNonZero(v.Index(0))
	case reflect.Slice:
		n := v.Len() + 1 // a length the current value does not have
		s := reflect.MakeSlice(v.Type(), n, n)
		if s.Index(0).Kind() == reflect.Uint8 {
			s.Index(0).SetUint(1)
		}
		v.Set(s)
		return true
	case reflect.Ptr:
		p := reflect.New(v.Type().Elem())
		if p.Elem().Kind() == reflect.Array {
			setNonZero(p.Elem())
		}
		v.Set(p)
		return true
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
		return true
	default:
		return false
	}
}
