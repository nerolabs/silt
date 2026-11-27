package depcheck

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const module = "github.com/nerolabs/silt/"

// The credit ledger (core/credit) holds the balance lane: credit earned by serving,
// relaying and repair. Consensus standing must never be bought with it.
//
// On the shipped, objective path the chain decides standing from the bonded size it
// commits, and nothing else. The one bridge from the ledger into core/chain is the
// reputation function a daemon passes to chain.New, which only the legacy (non-objective)
// qualification reads; core/credit's Invariant A tests hold that its formula never reads
// a balance. Every other path is closed here: core/chain, and every package it reaches,
// must not import core/credit, nor the packages that settle into it.
var creditSide = map[string]bool{
	module + "core/credit":   true,
	module + "core/relaypay": true,
	module + "core/node":     true,
	// Witnessed demand (RC item 2, the demand axis): self-dealt demand must not buy standing, and
	// the structural half of that is that the chain cannot read a witnessed increment at all.
	module + "core/demand": true,
}

// moduleImports returns the in-module packages the non-test files of pkg import.
func moduleImports(t *testing.T, repoRoot, pkg string) []string {
	t.Helper()
	dir := filepath.Join(repoRoot, strings.TrimPrefix(pkg, module))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if p := strings.Trim(imp.Path.Value, `"`); strings.HasPrefix(p, module) {
				seen[p] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func TestConsensusNeverReadsTheCreditLedger(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	start := module + "core/chain"
	via := map[string]string{start: ""}
	queue := []string{start}
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		for _, imp := range moduleImports(t, repoRoot, pkg) {
			if _, done := via[imp]; done {
				continue
			}
			via[imp] = pkg
			if creditSide[imp] {
				path := imp
				for p := pkg; p != ""; p = via[p] {
					path = p + " → " + path
				}
				t.Errorf("consensus reaches the balance lane: %s", path)
				continue
			}
			queue = append(queue, imp)
		}
	}
	// Anti-vacuity: the walk must have left core/chain, or a renamed module path would
	// make it pass on nothing.
	if len(via) < 3 {
		t.Fatalf("the walk from %s reached only %d package(s) — the guard is not looking", start, len(via))
	}
}
