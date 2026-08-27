// Package depcheck enforces the architecture rules with a test, so a
// violation is a red build, not a code-review hope:
//
//  1. core/* and ports never import adapters (hexagonal dependency rule).
//  2. core/* and ports never import effectful stdlib packages — no
//     networking, no filesystem, no wall clock, no ambient randomness.
//     All effects must arrive through injected interfaces; that is what
//     makes sim runs deterministic and replayable by seed.
//  3. No cmd/ entry point constructs the credit-Gated registry
//     (registry.NewGated): it hard-requires a durable Publisher and has no
//     token path, so it is sim/test-only and must never back a persistent
//     network (#99, M0 privacy). The production registry is the chain.
//
// Test files are exempt: tests may use os, math/rand, etc.
package depcheck

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var forbiddenExact = map[string]string{
	"os":           "filesystem access must come through a ChunkStore or other port",
	"time":         "wall-clock time must come through the Clock port",
	"math/rand":    "randomness must be injected (io.Reader or seeded source)",
	"math/rand/v2": "randomness must be injected (io.Reader or seeded source)",
	"crypto/rand":  "randomness must be injected so sims are deterministic",
	"net":          "core has no networking; that's what adapters are for",
}

var forbiddenPrefixes = map[string]string{
	"github.com/nerolabs/silt/adapters/": "core must not import adapters (hexagonal rule)",
	"net/":                               "core has no networking; that's what adapters are for",
	"os/":                                "filesystem access must come through a port",
}

func TestCoreImportsNoAdaptersAndNoEffects(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"core", "ports"} {
		root := filepath.Join(repoRoot, dir)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imp := range f.Imports {
				pkg := strings.Trim(imp.Path.Value, `"`)
				rel, _ := filepath.Rel(repoRoot, path)
				if why, bad := forbiddenExact[pkg]; bad {
					t.Errorf("%s imports %q — %s", rel, pkg, why)
				}
				for prefix, why := range forbiddenPrefixes {
					if strings.HasPrefix(pkg, prefix) {
						t.Errorf("%s imports %q — %s", rel, pkg, why)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestGatedRegistryFencedOffFromProduction fails the build if any cmd/ file
// references registry.NewGated. The credit-Gated registry records a durable
// Publisher on every entry (no token path), so it is the non-M0, sim/test
// path and must never be wired into a persistent daemon (#99).
func TestGatedRegistryFencedOffFromProduction(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmdRoot := filepath.Join(repoRoot, "cmd")
	err = filepath.WalkDir(cmdRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(repoRoot, path)
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if ok && pkg.Name == "registry" && sel.Sel.Name == "NewGated" {
				t.Errorf("%s constructs registry.NewGated — the credit-Gated registry is sim/test-only (records a durable Publisher, no token path); a persistent daemon must use the chain (#99)", rel)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// transitiveEffectAllowed records third-party effects that are reachable from
// core's import graph but NOT reachable in silt's configuration or usage. Each
// entry is a CLAIM, reviewable and falsifiable — the same bar the state-field
// classification uses: every exclusion states what it is asserting.
//
// This is a RATCHET, not a proof. It does not establish that the listed effects
// are harmless in general; it establishes that someone looked, wrote down why,
// and that the NEXT third-party effect to appear cannot arrive silently. That
// is the property worth having, and it is the one that was missing.
var transitiveEffectAllowed = map[string]string{
	"github.com/fxamacker/cbor/v2 math/rand": "VERIFIED unreachable in silt's " +
		"configuration: the only rand use is `start = rand.Intn(len(flds))` in " +
		"encodeStruct (encode.go:1532), guarded by `em.sort == SortFastShuffle`. " +
		"silt encodes with cbor.CanonicalEncOptions() (chain.go:498/:502), which " +
		"sorts canonically, so the shuffle branch is never taken. If an encode mode " +
		"is ever changed to SortFastShuffle, block encoding becomes nondeterministic " +
		"and consensus breaks — this entry is the tripwire for that.",
	"github.com/fxamacker/cbor/v2 time": "time is used for CBOR tag-0/1 time.Time " +
		"support. silt's committed structs carry no time.Time (heights and TTLs are " +
		"uint64), so the path is not exercised by block encoding.",
	"github.com/klauspost/cpuid/v2 os": "host CPU-feature detection at init, used by " +
		"reedsolomon to select a SIMD implementation. It reads host capability, never " +
		"consensus state. NOTE THE ASSUMPTION THIS RESTS ON: the SIMD and generic " +
		"Reed-Solomon paths must produce identical bytes, or erasure output would vary " +
		"by host — recorded here because it is an assumption, not a proof.",
	"golang.org/x/sys/unix time": "syscall wrappers pulled in by cpuid's capability " +
		"detection; the same host-capability path as above, not a consensus input.",
	"golang.org/x/sys/cpu time": "as above — CPU feature detection.",
}

// TestNoThirdPartyEffectsReachableFromCore closes the transitive gap the
// keystone node-store consult exposed, per the PE ruling
// (RULING-keystone-node-store-dependency-2026-08-27.md, Q3: "close it now — this
// is the change that makes the gap live").
//
// The test above inspects DIRECT imports only. That was honest while core
// imported nothing third-party. It no longer is: a pure-looking third-party
// package that itself reaches the filesystem, the clock, the network, or
// ambient randomness could enter core without tripping the hexagonal guard,
// because the forbidden import sits one hop away. The guard would report green
// while the property it exists to protect — injected effects, deterministic
// replay by seed — had quietly gone. A green check that no longer verifies its
// property is worse than no check: it manufactures confidence.
//
// SCOPE (the PE's note, and it matters for false positives): the boundary is
// about THIRD-PARTY purity, not the standard library. Stdlib uses `os`
// internally by design — `fmt` does — so flagging stdlib would make this
// unrunnable. So: walk every non-stdlib, non-silt package transitively
// reachable from core/ports, and flag any that imports the forbidden set.
func TestNoThirdPartyEffectsReachableFromCore(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	// `go list -deps` resolves the module-aware transitive closure — the same
	// graph the compiler builds, rather than one this test re-derives.
	cmd := exec.Command("go", "list", "-deps", "-json", "./core/...", "./ports/...")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("go list unavailable (%v) — the direct-import guard above still runs", err)
	}

	type pkg struct {
		ImportPath string
		Standard   bool
		Imports    []string
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var thirdParty []pkg
	for {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			break
		}
		if p.Standard || strings.HasPrefix(p.ImportPath, "github.com/nerolabs/silt") {
			continue
		}
		thirdParty = append(thirdParty, p)
	}

	for _, p := range thirdParty {
		for _, imp := range p.Imports {
			if reason, ok := transitiveEffectAllowed[p.ImportPath+" "+imp]; ok {
				t.Logf("allowed: %s imports %q — %s", p.ImportPath, imp, reason)
				continue
			}
			if why, bad := forbiddenExact[imp]; bad {
				t.Errorf("third-party package %q is reachable from core/ports and imports %q — %s\n\n"+
					"The direct-import guard cannot see this: the forbidden import is one "+
					"hop away, so core stays green while the injected-effects property is "+
					"gone. Either keep this dependency out of core (put it behind a port "+
					"and an adapter, the ChunkStore/ProofStore pattern), or establish that "+
					"the effect is not reachable and record why.", p.ImportPath, imp, why)
			}
			for prefix, why := range forbiddenPrefixes {
				// Only the effect prefixes apply here; the adapters rule is a
				// silt-internal concern already covered above.
				if strings.HasPrefix(prefix, "github.com/nerolabs/silt") {
					continue
				}
				if strings.HasPrefix(imp, prefix) {
					t.Errorf("third-party package %q is reachable from core/ports and imports %q — %s",
						p.ImportPath, imp, why)
				}
			}
		}
	}

	if len(thirdParty) == 0 {
		t.Log("no third-party packages reachable from core/ports")
	} else {
		names := make([]string, 0, len(thirdParty))
		for _, p := range thirdParty {
			names = append(names, p.ImportPath)
		}
		t.Logf("checked %d third-party package(s) reachable from core/ports: %s",
			len(thirdParty), strings.Join(names, ", "))
	}
}
