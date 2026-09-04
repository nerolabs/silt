package depcheck

// R3.1 V2 — the SMT second-preimage / domain-separation scope gate (verification half).
//
// Binding spec: docs/thinking/2026-09-01-smt-domain-separation-close-design.md, Part 2 (the
// three scope invariants) and Part 3 "V2". The Researcher certifies the disjoint-preimage
// ARGUMENT separately (core/statehash's V1 pins the math the argument rests on); this gate pins
// the SOURCE CONDITIONS the argument needs to keep holding as the code evolves:
//
//   - SI-1 (non-sum): exactly one smt.NewTrieSpec construction site in non-test code, and it is
//     (sha256.New(), false).
//   - SI-2 (default value hasher): zero WithValueHasher references in non-test code.
//   - SI-3 (no closest proof): zero references to ProveClosest, VerifyClosestProof,
//     SparseMerkleClosestProof, nilPathHasher, or newNilPathHasher in non-test code — the
//     unaudited path the Thesis Defense audit's Issue #3 was about.
//
// THE INVENTORY IS DERIVED, NOT HAND-WRITTEN (scar-inventory-gate-is-a-hand-list, 2026-09-03):
// this walks core/, cmd/, adapters/, internal/ from the repo root by filepath.WalkDir, the same
// method TestCoreImportsNoAdaptersAndNoEffects and TestGatedRegistryFencedOffFromProduction
// above already use, and parses each file's AST rather than grepping raw text — a doc comment
// that MENTIONS one of these symbols (to explain why silt does not use it) must not trip the
// gate, and a method call through an arbitrary receiver (trie.ProveClosest(...), not just
// smt.ProveClosest(...)) must.
//
// This file uses go/parser.ParseFile (like the tests above), not os.ReadFile of a literal path,
// so scripts/check_source_gates.py's SOURCE GATE / RUNTIME GATE marker discipline (which fires
// on os.ReadFile("....go")) does not mechanically apply here — the existing depcheck tests in
// this package follow the same unmarked AST-walk convention. RUNTIME GATE: none; UNGATED: the
// disjoint-preimage argument itself (Researcher-certified separately, core/statehash V1 pins its
// math). This gate is structural: it can see call sites and literal argument shapes, not runtime
// behaviour.

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// smtSite is one located reference to a symbol this gate tracks.
type smtSite struct {
	file string // repo-relative path
	line int
	src  string // the call/selector rendered back to source, for a readable failure
}

// walkSMTSites parses every non-test .go file under core/, cmd/, adapters/, internal/ (repo
// root, derived by filepath.WalkDir — never a hand-written file list) and returns:
//
//   - trieSpecCalls: every smt.NewTrieSpec(...) call site, with its two argument strings.
//   - valueHasherSites: every WithValueHasher reference (call or plain selector/ident).
//   - closestProofSites: every ProveClosest / VerifyClosestProof / SparseMerkleClosestProof /
//     nilPathHasher / newNilPathHasher reference (call, selector, or plain identifier — a
//     locally re-implemented helper under one of these names is caught too, not just an import).
func walkSMTSites(t *testing.T) (trieSpecCalls []smtSite, trieSpecArgs [][2]string, valueHasherSites, closestProofSites []smtSite) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	closestProofNames := map[string]bool{
		"ProveClosest":             true,
		"VerifyClosestProof":       true,
		"SparseMerkleClosestProof": true,
		"nilPathHasher":            true,
		"newNilPathHasher":         true,
	}

	for _, dir := range []string{"core", "cmd", "adapters", "internal"} {
		root := filepath.Join(repoRoot, dir)
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return perr
			}
			rel, _ := filepath.Rel(repoRoot, path)

			renderNode := func(n ast.Node) string {
				var sb strings.Builder
				if fmtErr := format.Node(&sb, fset, n); fmtErr != nil {
					return "<unrenderable>"
				}
				return sb.String()
			}

			ast.Inspect(f, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CallExpr:
					if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
						line := fset.Position(node.Pos()).Line
						switch sel.Sel.Name {
						case "NewTrieSpec":
							trieSpecCalls = append(trieSpecCalls, smtSite{file: rel, line: line, src: renderNode(node)})
							var a0, a1 string
							if len(node.Args) > 0 {
								a0 = renderNode(node.Args[0])
							}
							if len(node.Args) > 1 {
								a1 = renderNode(node.Args[1])
							}
							trieSpecArgs = append(trieSpecArgs, [2]string{a0, a1})
						case "WithValueHasher":
							valueHasherSites = append(valueHasherSites, smtSite{file: rel, line: line, src: renderNode(node)})
						}
						if closestProofNames[sel.Sel.Name] {
							closestProofSites = append(closestProofSites, smtSite{file: rel, line: line, src: renderNode(node)})
						}
					}
				case *ast.SelectorExpr:
					// A non-call reference (e.g. a type name smt.SparseMerkleClosestProof used
					// in a var/field/param declaration, or a function value passed without a
					// call). CallExpr above already covers the call-shaped uses; this covers the
					// rest so the gate is not fooled by "pass the func, don't call it here".
					if closestProofNames[node.Sel.Name] {
						line := fset.Position(node.Pos()).Line
						closestProofSites = append(closestProofSites, smtSite{file: rel, line: line, src: renderNode(node)})
					}
					if node.Sel.Name == "WithValueHasher" {
						line := fset.Position(node.Pos()).Line
						valueHasherSites = append(valueHasherSites, smtSite{file: rel, line: line, src: renderNode(node)})
					}
				case *ast.Ident:
					// A bare (unqualified) identifier matching a closest-proof symbol name: a
					// locally-declared function/type/var reusing the library's private name
					// (e.g. a copy-pasted nilPathHasher), not just an import of it.
					if closestProofNames[node.Name] {
						line := fset.Position(node.Pos()).Line
						closestProofSites = append(closestProofSites, smtSite{file: rel, line: line, src: node.Name})
					}
				}
				return true
			})
			return nil
		})
		if walkErr != nil {
			t.Fatal(walkErr)
		}
	}
	return
}

// dedupeSites collapses sites that were recorded twice for the same file:line (the CallExpr and
// SelectorExpr cases above can both match a call like x.ProveClosest(...) — once as the call,
// once as its Fun selector). Keyed on file:line:src.
func dedupeSites(sites []smtSite) []smtSite {
	seen := map[string]bool{}
	out := make([]smtSite, 0, len(sites))
	for _, s := range sites {
		key := s.file + ":" + itoa(s.line) + ":" + s.src
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestSI1SoleNonSumTrieSpecConstruction pins SI-1: exactly one smt.NewTrieSpec construction site
// in non-test code across core/, cmd/, adapters/, internal/, and it constructs the non-sum
// SHA-256 spec (sha256.New(), false) — the spec statehash.Root's trie (smt.NewSparseMerkleTrie)
// implicitly matches. A second construction site, or a different argument pair, is exactly the
// drift the domain-separation argument (core/statehash V1) does not cover: the argument is about
// silt's FIXED-WIDTH, non-sum leaf encoding specifically.
func TestSI1SoleNonSumTrieSpecConstruction(t *testing.T) {
	sites, args, _, _ := walkSMTSites(t)
	sites = dedupeSites(sites)

	if len(sites) != 1 {
		for _, s := range sites {
			t.Logf("NewTrieSpec site: %s:%d: %s", s.file, s.line, s.src)
		}
		t.Fatalf("SI-1 violated: found %d smt.NewTrieSpec construction site(s) in non-test code, want exactly 1 "+
			"(core/statehash/witness.go's verifySpec) — a second site re-opens the domain-separation "+
			"argument's scope, which is pinned to silt's single fixed-width non-sum encoding", len(sites))
	}
	got := sites[0]
	if got.file != "core/statehash/witness.go" {
		t.Fatalf("SI-1 violated: the sole NewTrieSpec site moved to %s:%d (want core/statehash/witness.go)", got.file, got.line)
	}
	a0, a1 := args[0][0], args[0][1]
	if a0 != "sha256.New()" {
		t.Fatalf("SI-1 violated: %s:%d NewTrieSpec's first argument is %q, want \"sha256.New()\" (non-default hasher "+
			"changes the leaf/inner preimage widths the domain-separation argument depends on)", got.file, got.line, a0)
	}
	if a1 != "false" {
		t.Fatalf("SI-1 violated: %s:%d NewTrieSpec's second argument (sumTrie) is %q, want \"false\" — a sum trie uses "+
			"digestSumLeafNode/digestSumInnerNode, which are OUTSIDE the domain-separation argument's scope "+
			"(design doc Part 1, \"where it does NOT hold\")", got.file, got.line, a1)
	}
}

// TestSI2NoValueHasherOverride pins SI-2: no non-test code ever calls smt.WithValueHasher. silt
// always uses the DEFAULT value hasher, which is what makes every leaf's value slot a fixed
// 32-byte SHA-256 valueHash — the fixed width the domain-separation argument's disjoint-preimage
// claim (leaf and inner are both exactly 65 bytes, differing only in the prefix byte) depends on.
func TestSI2NoValueHasherOverride(t *testing.T) {
	_, _, sites, _ := walkSMTSites(t)
	sites = dedupeSites(sites)
	if len(sites) != 0 {
		for _, s := range sites {
			t.Logf("WithValueHasher site: %s:%d: %s", s.file, s.line, s.src)
		}
		t.Fatalf("SI-2 violated: found %d WithValueHasher reference(s) in non-test code, want 0 — a non-default "+
			"value hasher changes the leaf preimage width, which must be re-reviewed against the "+
			"domain-separation argument before it ships (design doc SI-2)", len(sites))
	}
}

// TestSI3NoClosestProofOrNilPathHasherInProduction pins SI-3: no non-test code references
// ProveClosest, VerifyClosestProof, SparseMerkleClosestProof, nilPathHasher, or
// newNilPathHasher — the closest-proof path the Thesis Defense audit's Issue #3 was about, and
// which silt's domain-separation close explicitly declines to use or analyze.
func TestSI3NoClosestProofOrNilPathHasherInProduction(t *testing.T) {
	_, _, _, sites := walkSMTSites(t)
	sites = dedupeSites(sites)
	if len(sites) != 0 {
		for _, s := range sites {
			t.Logf("closest-proof site: %s:%d: %s", s.file, s.line, s.src)
		}
		t.Fatalf("SI-3 violated: found %d closest-proof / nilPathHasher reference(s) in non-test code, want 0 — "+
			"this is the unaudited path (Thesis Defense audit Issue #3); silt's standard Prove/VerifyProof-only "+
			"scope (design doc SI-3) does not cover it", len(sites))
	}
}
