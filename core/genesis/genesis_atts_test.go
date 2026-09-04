package genesis_test

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/genesis"
)

// G7 — TestProductionGenesisCarriesNoAtts, the sibling of
// TestProductionGenesisCarriesNoBondRegs (genesis_sameroot_premise_test.go).
//
// The genesis attestation seating rule (genesis-atts-seating-rule-RESEARCH-CERTIFICATION-
// 2026-09-04.md §4.4; owner-ratified 2026-09-04) ships with NO era gate. That is sound
// only because the production genesis carries zero attestations: on every real network
// the rule "seat only the attestations that verify over the genesis hash" is then an
// identity transform, so no persisted chain, archival fixture, or field-test replica
// changes state on upgrade. This test is the pin that makes the no-era-gate claim hold.
// The archival half (each committed fixture's block 0 carries no Atts) lives in
// core/chain (TestArchivalFixturesGenesisCarriesNoAtts) — genesis imports chain, so a
// chain test cannot live here.
//
// Teeth: if a genesis builder ever attests height 0 (a "seed the launch anchors at
// genesis" change, or a certificate riding on the genesis block), this reddens, and the
// seating rule stops being an identity transform on production — re-derive §4.4 before
// shipping.
func TestProductionGenesisCarriesNoAtts(t *testing.T) {
	gb1, _, _, err1 := genesis.Build(memstore.New())
	gb2, _, _, err2 := genesis.Build(memstore.New())
	if err1 != nil || err2 != nil {
		t.Fatalf("genesis.Build failed: %v / %v", err1, err2)
	}
	if gb1.Hash() != gb2.Hash() {
		t.Fatalf("PREMISE CHANGED: two nodes' genesis.Build produced DIFFERENT block hashes (%x vs %x)", gb1.Hash(), gb2.Hash())
	}
	for i, gb := range []struct {
		atts, pqc, lc int
	}{{len(gb1.Atts), len(gb1.PrepareQC), len(gb1.LastCommit)}, {len(gb2.Atts), len(gb2.PrepareQC), len(gb2.LastCommit)}} {
		if gb.atts != 0 || gb.pqc != 0 {
			t.Fatalf("PREMISE CHANGED (build %d): the production genesis now carries %d Atts / %d PrepareQC. "+
				"The genesis seating rule (seat only verified attestations) is no longer an identity "+
				"transform on production, so its no-era-gate claim (cert §4.4) must be re-derived "+
				"before this ships; anchors seat at height >= 1 through the founding drain, never at 0.",
				i+1, gb.atts, gb.pqc)
		}
		if gb.lc != 0 {
			t.Fatalf("PREMISE CHANGED (build %d): the production genesis carries %d LastCommit entries — "+
				"a genesis carrier is REFUSED by rule (O1, TestGenesisLastCommitIsRefused); no daemon could boot", i+1, gb.lc)
		}
	}
	t.Logf("premise pinned: production genesis %x carries 0 Atts, 0 PrepareQC, 0 LastCommit (version %d)", gb1.Hash(), gb1.Version)
}
