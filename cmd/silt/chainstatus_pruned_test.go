package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/chainstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// The deep-sheet exit gate (ROADMAP Phase 3) asserts the retention prune from
// REAL persisted state: chain-status must count payload-stripped blocks so an
// operator (and the field harness) can confirm the prune engaged without
// depending on a debug log line. This pins the count against a store holding
// a pruned and an un-pruned block.
func TestChainStatusReportsPrunedBlocks(t *testing.T) {
	dir := t.TempDir()

	g := chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{{Root: ports.HashBytes([]byte("g"))}}}
	b1 := chain.Block{Version: 1, Height: 1, Prev: g.Hash(),
		BondRegs: []chain.BondReg{{Answer: []byte("heavy-proof-bytes")}}}
	pruned := b1.Prune()
	if !pruned.IsPruned() || b1.IsPruned() {
		t.Fatal("rig error: Prune() must mark the copy and leave the original unpruned")
	}
	if err := chainstore.Save(filepath.Join(dir, "chain.cbor"), []chain.Block{g, pruned}); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := cmdChainStatus([]string{"-store", dir}); err != nil {
			t.Fatal(err)
		}
	})
	want := "pruned:       1 blocks payload-stripped below the retention horizon"
	if !strings.Contains(out, want) {
		t.Fatalf("chain-status must report the pruned count from persisted state;\nwant line %q in:\n%s", want, out)
	}
}

func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	f()
	w.Close()
	buf := make([]byte, 1<<16)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

