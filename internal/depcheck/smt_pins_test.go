package depcheck

// the gate.
//
// -: the shipped V2 gate pins a PROXY for SI-1 — NewSparseMerkleSumTrie calls
// NewTrieSpec(hasher, true) and WithValueHasher(nil) INSIDE the library, so a silt sum-trie
// call would violate SI-1 and SI-2 with V2 green. This gate asserts zero
// NewSparseMerkleSumTrie / ImportSparseMerkleSumTrie sites in non-test code, and that every
// NewSparseMerkleTrie / ImportSparseMerkleTrie site passes sha256.New as its hasher.
// - (SI-7): the go.mod pin for pokt-network/smt is exactly the version the
// research was written against, so a bump is a red test and a forced re-read of (the
// audit's scope commit is not the v1.0.0 tag; provenance is carried in the record).
//
// Same AST-walk convention as the sibling depcheck tests (not os.ReadFile of a literal.go
// path, so scripts/check_source_gates.py's marker discipline does not mechanically apply).
// RUNTIME GATE: none — these are structural conditions of an argument, not behaviours.

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const r31CertifiedSMTVersion = "github.com/pokt-network/smt v1.0.0"

func TestNoSumTrieAndEverySMTUsesSHA256(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	type site struct{ file, src string }
	var sumSites, badHasher []site
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
			render := func(n ast.Node) string {
				var sb strings.Builder
				if e := format.Node(&sb, fset, n); e != nil {
					return "<unrenderable>"
				}
				return sb.String()
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CallExpr:
					sel, ok := node.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					switch sel.Sel.Name {
					case "NewSparseMerkleSumTrie", "ImportSparseMerkleSumTrie":
						sumSites = append(sumSites, site{rel, render(node)})
					case "NewSparseMerkleTrie", "ImportSparseMerkleTrie":
						if len(node.Args) < 2 || render(node.Args[1]) != "sha256.New()" {
							badHasher = append(badHasher, site{rel, render(node)})
						}
					}
				case *ast.SelectorExpr:
					if node.Sel.Name == "NewSparseMerkleSumTrie" || node.Sel.Name == "ImportSparseMerkleSumTrie" {
						sumSites = append(sumSites, site{rel, render(node)})
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
	if len(sumSites) != 0 {
		t.Fatalf("a SUM trie is constructed or imported in non-test code — the library sets sumTrie=true and WithValueHasher(nil) internally, which the V2 gate cannot see: %v", sumSites)
	}
	if len(badHasher) != 0 {
		t.Fatalf("an SMT is constructed with a hasher other than sha256.New — the disjoint-preimage argument is written against SHA-256 (32-byte path and value digests): %v", badHasher)
	}
}

func TestSMTModuleIsPinnedToAnExactVersion(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(string(mod), "\n") {
		if strings.Contains(line, "github.com/pokt-network/smt ") {
			found = true
			if strings.TrimSpace(line) != r31CertifiedSMTVersion {
				t.Fatalf("go.mod pins %q, but the disjoint-preimage argument was against %q — bumping the SMT library re-opens (re-read the node encodings, the verifier's recomputation order and the audit's scope commit before changing this constant)", strings.TrimSpace(line), r31CertifiedSMTVersion)
			}
		}
	}
	if !found {
		t.Fatalf("go.mod has no pokt-network/smt requirement")
	}
}
