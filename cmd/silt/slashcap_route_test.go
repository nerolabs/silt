package main

import (
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/node"
)

// TestG_SLASHCAP_3_ShippedFlagDefaultsAndTheRefusalText drives the daemon's OWN flag default CONSTANTS through the predicate, so
// that changing a default in daemon.go changes this result. It does NOT boot the daemon:
// the wiring and order are covered by the source gate below, and core/node G-SLASHCAP-1
// drives the boundary itself. Named for what it checks — the shipped defaults and the
// operator-facing refusal text — rather than for a daemon run it does not perform.
func TestG_SLASHCAP_3_ShippedFlagDefaultsAndTheRefusalText(t *testing.T) {
	const mib = 1 << 20

	// The daemon's ACTUAL flag defaults, by the same constants cmdDaemon passes to
	// fs.Int64 — not a literal restated here. Moving a default in daemon.go therefore moves
	// this assertion, instead of leaving a green test beside a binary that will not start
	// (PE ruling B-4, ablation A5).
	shipped := node.Config{
		MaxBondRegBytesPerBlock: defaultMaxBondRegBytesPerBlock,
		MaxEntryBytesPerBlock:   defaultMaxEntryBytesPerBlock,
	}
	if err := node.CheckSlashEvidenceHeadroom(shipped); err != nil {
		t.Fatalf("the SHIPPED flag defaults are refused by the gate wired into daemon.go — the daemon cannot start: %v", err)
	}

	// The operator route the owner ordered closed: raise the local bond-reg budget and
	// a real double-signer's evidence no longer fits the consensus cap.
	overCap := node.Config{MaxBondRegBytesPerBlock: 8 * mib, MaxEntryBytesPerBlock: 64 << 10}
	err := node.CheckSlashEvidenceHeadroom(overCap)
	if err == nil {
		t.Fatal("ROUTE OPEN: -max-bondreg-bytes-per-block=8MiB was ACCEPTED. A validator on this config makes " +
			"its own equivocation unprovable — the evidence pair exceeds SlashesBytesCap, so the cap rejects it " +
			"before CheckEquivocation runs and the equivocator keeps its seat.")
	}
	for _, must := range []string{"SlashesBytesCap", "-max-bondreg-bytes-per-block", "keeps its seat"} {
		if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(must)) {
			t.Fatalf("the refusal must name %q so an operator can act on it, and must say what is lost; got: %v", must, err)
		}
	}

	// And the unbounded posture the flag help advertises.
	if err := node.CheckSlashEvidenceHeadroom(node.Config{MaxBondRegBytesPerBlock: 0, MaxEntryBytesPerBlock: 64 << 10}); err == nil {
		t.Fatal("ROUTE OPEN: -max-bondreg-bytes-per-block=0 (documented `0 = unbounded`) was ACCEPTED")
	}
}

// SOURCE GATE: this test reads cmd/silt/daemon.go as TEXT. It can see only strings and
// their ORDER — that the call exists, that it sits after the two cfg assignments it
// validates, and that it returns rather than warns. It observes no behaviour.
//
// RUNTIME GATE: TestG_SLASHCAP_3_ShippedFlagDefaultsAndTheRefusalText (above)
// observes the refusal itself, and core/node TestG_SLASHCAP_1_ConfigCannotDefeatSlashEvidence
// drives the boundary both ways. What this source gate adds is the WIRING and the ORDER:
// a check placed before the assignments would validate a zero value and be vacuous, and a
// check that warned instead of returning would let the node start and the seat survive
// anyway — neither is visible to a runtime test of the predicate alone.
func TestG_SLASHCAP_4_TheRefusalIsWiredAtStartup_Source(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot read daemon.go as text, so the wiring is unverified: %v", err)
	}
	s := string(src)

	call := strings.Index(s, "node.CheckSlashEvidenceHeadroom(cfg)")
	if call < 0 {
		t.Fatal("SOURCE GATE: the string \"node.CheckSlashEvidenceHeadroom(cfg)\" is absent from daemon.go, so the consensus cap's derivation " +
			"is once again unbound to the configuration in force, which is the route the owner ordered closed")
	}
	assign := strings.Index(s, "cfg.MaxEntryBytesPerBlock = *maxEntryBytes")
	if assign < 0 {
		t.Fatal("SOURCE GATE: the string \"cfg.MaxEntryBytesPerBlock = *maxEntryBytes\" is absent from daemon.go; this gate checks an ORDER against an anchor that no longer exists — re-home it")
	}
	if call < assign {
		t.Fatal("SOURCE GATE: by string OFFSET the headroom call precedes the budget assignments in daemon.go, so it validates a zero value " +
			"and is vacuous")
	}
	// It must refuse, not warn.
	tail := s[call:min(call+240, len(s))]
	if !strings.Contains(tail, "return fmt.Errorf") {
		t.Fatalf("SOURCE GATE: the 240 bytes of text following the headroom call contain no \"return fmt.Errorf\" — a warning lets the node start and the seat "+
			"survives anyway; got: %q", tail)
	}
}
