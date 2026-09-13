package main

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"regexp"
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
//
// THIS IS THE PRE-v5 LEG, AND SAYING SO IS THE POINT. Its fixture is a v1 chain,
// where Prune() sets `Pruned` and IsPruned() therefore answers the question. That
// fixture is exactly why this test stayed GREEN while the shipped counter was
// structurally zero on the v5 chain the field harness actually drives — see
// TestChainStatusPrunedCountIsSoundOnAV5Chain for the era leg.
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
	// Both counters read 1 here, and the reason is EQUALITY, not the subset direction:
	// validateD3Digests refuses a pre-v5 AnswerDigest outright, so on v1/v2/v4 the only way
	// HeavyProofsShed() can fire is the IsPruned() short-circuit and the two predicates are
	// the same predicate. That is precisely why this leg cannot gate the second line's
	// choice of predicate, and why the v5 leg has to.
	for _, want := range []string{
		"pruned:       1 blocks have shed their heavy bond proofs below the retention horizon",
		"of those:     1 declare a pre-v5 non-recomputable identity",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("chain-status must report the pruned count from persisted state;\nwant line %q in:\n%s", want, out)
		}
	}
}

// TestChainStatusPrunedCountIsSoundOnAV5Chain is the era leg, and it drives the defect the
// graded run `869ad9a-deep` surfaced: row 12b-deep-prune failed with a UNIFORM ZERO across
// all four validators (`0pruned` on val-a…val-d) on a chain of 1 x v2 + 137 x v5.
//
// THE MECHANISM. `chain-status` counted `Block.IsPruned()`, which is `b.Pruned != Hash{}`.
// (d-3) retired `Pruned` for v5 — a pruned v5 block's preimage folds AnswerDigest in place of
// Answer, so the pruned body still reproduces its own hash and Prune() sets no token. The
// counter is therefore STRUCTURALLY zero on a v5 chain and the harness gate (`pruned: N >= 1`)
// fails by construction, whatever the node actually did. The consensus readers were re-keyed to
// `HeavyProofsShed()` by the (d-3) sweep; this reader was missed.
//
// The rig-error arms are the anti-vacuity anchor: they pin that the fixture really is in the
// post-(d-3) regime (no declared token, proofs demonstrably shed), so this cannot pass for the
// pre-v5 reason the sibling test covers.
func TestChainStatusPrunedCountIsSoundOnAV5Chain(t *testing.T) {
	dir := t.TempDir()

	// Mirror the graded chain's shape: a pre-v5 genesis under v5 history.
	g := chain.Block{Version: chain.BlockVersionRounds, Height: 0,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("g"))}}}
	answer := []byte("heavy-proof-bytes")
	// A v5 registration commits its proof by digest whether the proof is carried or pruned —
	// validateD3Digests refuses one that does not, so a fixture without this is not a legal
	// v5 block and would prove nothing about a real chain.
	digest := ports.Hash(sha256.Sum256(answer))
	b1 := chain.Block{Version: chain.BlockVersionWitnessable, Height: 1, Prev: g.Hash(),
		BondRegs: []chain.BondReg{{Answer: answer, AnswerDigest: &digest}}}
	pruned := b1.Prune()

	if pruned.IsPruned() {
		t.Fatal("rig error: (d-3) retires `Pruned` for v5 — a pruned v5 block must carry no declared token; " +
			"if this fires, the era premise moved and this test is measuring the wrong regime")
	}
	if !pruned.HeavyProofsShed() || b1.HeavyProofsShed() {
		t.Fatal("rig error: Prune() must shed the copy's heavy proofs and leave the original carrying them")
	}
	if err := chainstore.Save(filepath.Join(dir, "chain.cbor"), []chain.Block{g, pruned}); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := cmdChainStatus([]string{"-store", dir}); err != nil {
			t.Fatal(err)
		}
	})
	// The `pruned:` label and its spacing are the field harness's scrape surface
	// (integration/cloudtest/scenarios.sh reads `pruned:[[:space:]]*[0-9]+`), so the shape is
	// pinned here as well as the number.
	//
	// THE SECOND LINE'S PREDICATE IS PINNED HERE TOO, AND v5 IS THE ONLY ERA THAT CAN PIN IT.
	// chainstatus.go asserts in a comment that the identity counter never exceeds the
	// possession one; on v1/v2/v4 the two predicates are IDENTICAL (validateD3Digests refuses
	// a pre-v5 AnswerDigest outright, so HeavyProofsShed can only fire through the IsPruned
	// short-circuit), which is why the pre-v5 leg cannot separate them and why swapping this
	// counter to HeavyProofsShed() left the WHOLE cmd/silt package green. Under that swap the
	// output reads "of those: 1" in the same sentence that narrates 0 as EXPECTED — a false
	// statement contradicting its own narration. This want is what executes the claim.
	for _, want := range []string{
		"pruned:       1 blocks have shed their heavy bond proofs below the retention horizon",
		"of those:     0 declare a pre-v5 non-recomputable identity",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("chain-status must count a pruned v5 block under HeavyProofsShed() and report ZERO declared identities under IsPruned();\na v5 chain reads 0 for the first by construction, and a non-zero second line contradicts its own narration.\nwant line %q in:\n%s", want, out)
		}
	}

	// THE HARNESS'S OWN READ, RUN. The comment in chainstatus.go that says "no other line
	// may carry this shape" is an assertion, so it is executed rather than trusted: this is
	// byte-for-byte the expression integration/cloudtest/scenarios.sh applies to this output
	// for row 12b-deep-prune, and it takes the LAST match. Two matches would silently hand
	// the gate a different number than the one pinned above — which is the failure class that
	// a second `of those:` line could reintroduce. This test is the reason that line is safe.
	scrape := regexp.MustCompile(`pruned:[[:space:]]*[0-9]+`)
	hits := scrape.FindAllString(out, -1)
	if len(hits) != 1 {
		t.Fatalf("the field harness scrapes `pruned:[[:space:]]*[0-9]+` and keeps the LAST match; "+
			"exactly one line may carry that shape, got %d: %q\nin:\n%s", len(hits), hits, out)
	}
	if got := strings.TrimSpace(strings.TrimPrefix(hits[0], "pruned:")); got != "1" {
		t.Fatalf("the number the field gate would read is %q, want \"1\" (it asserts >= 1)", got)
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
