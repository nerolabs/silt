package node

import (
	"testing"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// #572, round 2 — the field mechanism named by the shipped diagnostic
// (474718e-deep: 507 warns; 391 on val-d, all `suffix stop: immature network
// requires anchor attestations: 2 of required 3` at h32 with suffix-appends=0).
//
// THE ARITHMETIC THAT PINS IT: val-d's replica demanded need=3 anchors for the
// majority's h32 — so its replayed state was PRE-handoff — while the live
// majority COMMITTED h32 carrying only 2 anchor attestations — so their
// handedOff() was already true when h32 validated (their everMature latched at
// or before the h24 rotation). Identical committed prefix, divergent regime
// state ⇒ the everMature latch is NOT replay-deterministic (the #558 class:
// live validation ≠ replay validation, this time for the maturity regime
// rather than era signatures).
//
// THE ORACLE: a chain whose LIVE history tripped the latch must reproduce the
// latch when REPLAYED into a fresh replica — and a replica that replays the
// pre-handoff prefix must then ADOPT the post-handoff suffix. RED = the fresh
// replica under-latches (EverMature false after replaying the same blocks) or
// refuses the suffix with ErrAnchorRequired.
func TestLatchSurvivesReplay_572(t *testing.T) {
	nodes, ids, _, _, _ := matureWorld12(t)
	src := nodes[0].Chain()
	if !src.EverMature() {
		t.Fatal("premise: the live world must be latched (matureWorld12 latches during its drive)")
	}

	// The exact persisted/wire representation a restarted validator replays.
	blocks, err := chain.DecodeBlocks(chain.EncodeBlocks(src.Blocks(0)))
	if err != nil {
		t.Fatalf("roundtrip: %v", err)
	}

	anchors := map[ports.NodeID]bool{}
	for i := 0; i < 4; i++ {
		anchors[ids[i].NodeID()] = true
	}
	fresh := chain.New(chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, MatureValidators: 2, OperatorMargin: 1, EpochBlocks: 8},
		func(ports.NodeID) int64 { return 0 })
	fresh.SetBondVerifier(mcStubVerify)
	n, err := fresh.Reload(blocks)
	if err != nil || n != len(blocks) {
		t.Fatalf("#572 REPRODUCED (replay refuses live history): Reload restored %d of %d: %v", n, len(blocks), err)
	}
	if src.EverMature() && !fresh.EverMature() {
		t.Fatalf("#572 REPRODUCED (the latch is not replay-deterministic): the live chain latched everMature but replaying its own %d committed blocks did NOT — a restarted/behind validator wakes PRE-handoff in a POST-handoff world and refuses every mature commit with ErrAnchorRequired (474718e-deep val-d, 391 warns at h32)", len(blocks))
	}
}
