package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const repoRoot = "../.."

// checkContract is the presence check, factored so the teeth test can run it over a
// synthetic source with the marker removed.
func checkContract(e ContractedString, src string) (ok bool, why string) {
	if !strings.Contains(src, e.Marker) {
		return false, "SOURCE GATE: the announced marker " + strconvQuote(e.Marker) + " is no longer in " + e.File +
			" — it is an S5 observable contract (" + e.Why + "); renaming it breaks the operator interface and the " +
			"spawned-process tier that asserts it. Preserve the literal; never move the goalposts."
	}
	return true, ""
}

func strconvQuote(s string) string { return "\"" + s + "\"" }

// TestObservableContractStringsAreStillEmitted: every registered marker is still present in
// its emitting file (a SOURCE-text check), the file exists, no (marker, file) pair repeats,
// no marker is a strict substring of another (that would let the presence check pass for
// the wrong reason), and every named Asserter resolves to a real `func TestX(` in the tree.
//
// SOURCE GATE: this reads the product source as TEXT; it cannot see whether the line is
// still REACHABLE. RUNTIME GATE: the per-entry Asserter (an e2e test) observes the line
// from a spawned process.
func TestObservableContractStringsAreStillEmitted(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range ObservableContract {
		key := e.Marker + "\x00" + e.File
		if seen[key] {
			t.Errorf("SOURCE GATE: duplicate registry entry %q in %s", e.Marker, e.File)
		}
		seen[key] = true
		src, err := os.ReadFile(filepath.Join(repoRoot, e.File))
		if err != nil {
			t.Errorf("SOURCE GATE: %s (emitting %q) is missing — a marker that moved packages is how a rename hides: %v", e.File, e.Marker, err)
			continue
		}
		if ok, why := checkContract(e, string(src)); !ok {
			t.Error(why)
		}
	}
	for i, a := range ObservableContract {
		for j, b := range ObservableContract {
			if i != j && a.Marker != b.Marker && strings.Contains(b.Marker, a.Marker) {
				t.Errorf("SOURCE GATE: marker %q is a strict substring of %q — the presence check for the shorter one can pass for the wrong reason", a.Marker, b.Marker)
			}
		}
	}
	tests := testFuncNames(t)
	for _, e := range ObservableContract {
		if e.Asserter != "" && !tests[e.Asserter] {
			t.Errorf("SOURCE GATE: entry %q names asserter %s, but no `func %s(` exists in the tree — a cited runtime gate that does not exist is worse than none", e.Marker, e.Asserter, e.Asserter)
		}
	}
}

// TestObservableContractHasTeeth: the SAME presence check over a synthetic source with the
// marker removed must FAIL, so the registry cannot silently degrade to a no-op.
func TestObservableContractHasTeeth(t *testing.T) {
	e := ObservableContract[0]
	src, err := os.ReadFile(filepath.Join(repoRoot, e.File))
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := checkContract(e, string(src)); !ok {
		t.Fatalf("precondition: %q must be present in %s", e.Marker, e.File)
	}
	ablated := strings.ReplaceAll(string(src), e.Marker, "RENAMED-MARKER")
	if ok, _ := checkContract(e, ablated); ok {
		t.Fatalf("SOURCE GATE: the presence check passed with %q removed — the registry has no teeth", e.Marker)
	}
}

var testFuncRe = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]*)\(`)

// testFuncNames walks the tree for every `func TestX(` in a _test.go file.
func testFuncNames(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	err := filepath.WalkDir(repoRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "website", "node_modules", "archive":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for _, m := range testFuncRe.FindAllStringSubmatch(string(b), -1) {
			out[m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
