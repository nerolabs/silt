package chain

import (
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

// hashLiteralKeys parses Go source text and returns the field names named by the
// composite literal assigned to `unsigned` inside `func (b *Block) bodyHash`.
func hashLiteralKeys(src string) ([]string, bool) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "chain.go", src, 0)
	if err != nil {
		return nil, false
	}
	var keys []string
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
			for _, e := range cl.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if k, ok := kv.Key.(*ast.Ident); ok {
					keys = append(keys, k.Name)
				}
			}
			return false
		})
	}
	sort.Strings(keys)
	return keys, found
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
	keys, found := hashLiteralKeys(string(src))
	if !found {
		t.Fatal("SOURCE GATE: no `unsigned := Block{...}` composite literal inside func (b *Block) bodyHash in chain.go — the pin has lost its anchor; if the preimage construction moved, move this gate with it")
	}
	want := hashCoveredFields(t)
	missing, extra := hashLiteralGaps(keys, want)
	if len(missing) > 0 {
		t.Errorf("SOURCE GATE: bodyHash's `unsigned` literal does NOT fold exported Block field(s) %v — every field outside the five exclusions is a HOLE in the signed body (CD-0: the carrier merge must list BOTH IssuerKeys and LastCommit). Literal keys: %v", missing, keys)
	}
	if len(extra) > 0 {
		t.Errorf("SOURCE GATE: bodyHash's `unsigned` literal names %v, which is either a deliberate exclusion (folding it changes every committed hash) or not a Block field at all", extra)
	}
	// The two CD-0 fields by name, so the failure text is unambiguous on the rebase.
	for _, must := range []string{"IssuerKeys"} {
		if !contains(keys, must) {
			t.Errorf("SOURCE GATE: %q is not in the bodyHash literal (main folds it; the carrier base did not)", must)
		}
	}
	if _, ok := reflect.TypeOf(Block{}).FieldByName("LastCommit"); ok && !contains(keys, "LastCommit") {
		t.Errorf("SOURCE GATE: Block has a LastCommit field but the bodyHash literal does not fold it — the carrier is unsigned (CD-0)")
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
	base, found := hashLiteralKeys(string(src))
	if !found {
		t.Fatal("SOURCE GATE: precondition — the real literal must parse")
	}
	if m, x := hashLiteralGaps(base, want); len(m)+len(x) != 0 {
		t.Fatalf("SOURCE GATE: precondition — the real literal must be gap-free before the teeth are meaningful (missing %v extra %v)", m, x)
	}

	// Teeth 1: drop one folded key (the CD-0 shape: a merge loses IssuerKeys).
	dropped := strings.Replace(string(src), ", IssuerKeys: b.IssuerKeys", "", 1)
	if dropped == string(src) {
		t.Fatal("SOURCE GATE: teeth fixture — `, IssuerKeys: b.IssuerKeys` not found verbatim in the literal; update the teeth to the literal's current spelling")
	}
	keys, found := hashLiteralKeys(dropped)
	if !found {
		t.Fatal("SOURCE GATE: teeth fixture — edited literal no longer parses")
	}
	if missing, _ := hashLiteralGaps(keys, want); !contains(missing, "IssuerKeys") {
		t.Errorf("SOURCE GATE: TEETH FAILED — removing IssuerKeys from the literal was not reported missing (got %v)", missing)
	}

	// Teeth 2: fold an exclusion (Atts) — must be reported extra. Injected at the HEAD of the
	// literal so the fixture does not depend on which field the literal happens to end with
	// (it ended with IssuerKeys before the LastCommit carrier landed, LastCommit after).
	injected := strings.Replace(string(src), "unsigned := Block{", "unsigned := Block{Atts: b.Atts, ", 1)
	if injected == string(src) {
		t.Fatal("SOURCE GATE: teeth fixture — `unsigned := Block{` not found verbatim in bodyHash; update the teeth to the literal's current spelling")
	}
	keys, found = hashLiteralKeys(injected)
	if !found {
		t.Fatal("SOURCE GATE: teeth fixture — injected literal no longer parses")
	}
	if _, extra := hashLiteralGaps(keys, want); !contains(extra, "Atts") {
		t.Errorf("SOURCE GATE: TEETH FAILED — folding the excluded Atts was not reported extra (got %v)", extra)
	}

	// Teeth 3: a new Block field the literal does not know (the "field added, not
	// folded" shape) — must be reported missing.
	if missing, _ := hashLiteralGaps(base, append(append([]string{}, want...), "LastCommitPhantom")); !contains(missing, "LastCommitPhantom") {
		t.Errorf("SOURCE GATE: TEETH FAILED — a new field absent from the literal was not reported missing (got %v)", missing)
	}
}

// TestHashLiteralPinRuntimePair is the RUNTIME half of R-HASH-LITERAL-PIN: for every
// exported Block field, set it to a non-zero value on a copy of a real block and
// observe bodyHash(). A hash-covered field MUST move the digest; an excluded field MUST
// NOT. This observes the behaviour the source gate can only read, on the live code
// path Sign/verify use, with no allowlist beyond the same five exclusions.
func TestHashLiteralPinRuntimePair(t *testing.T) {
	base := Block{Version: 1, Height: 7, Prev: ports.HashBytes([]byte("prev")), Entries: []ports.Entry{entry(1)}}
	Sign(&base, key(41000))
	h0 := base.bodyHash()
	bt := reflect.TypeOf(Block{})
	for i := 0; i < bt.NumField(); i++ {
		f := bt.Field(i)
		if !f.IsExported() {
			continue
		}
		b := base
		fv := reflect.ValueOf(&b).Elem().Field(i)
		if !setNonZero(fv) {
			t.Fatalf("runtime pair: no non-zero constructor for Block.%s (%s) — extend setNonZero", f.Name, f.Type)
		}
		moved := b.bodyHash() != h0
		_, excluded := hashPreimageExclusions[f.Name]
		switch {
		case excluded && moved:
			t.Errorf("Block.%s is a declared exclusion but mutating it MOVED bodyHash — the literal folds an excluded field", f.Name)
		case !excluded && !moved:
			t.Errorf("Block.%s is hash-covered by declaration but mutating it did NOT move bodyHash — a HOLE in the signed body", f.Name)
		}
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
