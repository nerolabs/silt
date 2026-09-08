package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// ── G-H43-8, arm 8c — the mature-epoch count floor drops to 0 ──────────────
//
// TestG_H43_8c_MatureEpochWeightAloneAdmitsZeroAttestationCommit is arm 8c,
// per the Researcher's predicate certification
// `CONSENSUS-380-quorum-floor-direction-1-PREDICATE-AND-CERTIFICATION-2026-09-08.md`
// §1 row (b) and §5 arm 8c: "In a mature epoch: (i) a commit whose coalition
// carries >⅔ frozen weight with ZERO non-proposer attestations is ACCEPTED;
// (ii) the same block with the coalition below ⅔ weight is still REFUSED by
// name (ErrNoQuorumWeight), so floor 0 did not delete the bar."
//
// THE PREDICATE (§1, row (b)): under direction (1), `RequiredQuorum()`
// (chain.go:1710) returns 0 in the mature regime — not `cfg.Quorum` verbatim
// (today's behaviour) — because "the Byzantine bar here IS
// `requireEpochWeightQuorum` (chain.go:3093-3124): proposer + seen must carry
// `3·support > 2·total` of the frozen epoch weight; two such coalitions
// overlap in >⅓ weight, hence in honest bond." A count floor of 0 does not
// remove that bar — it removes a REDUNDANT, non-replicated, weaker one (a
// constant floor of ≥1 is already IMPLIED by the weight rule in every
// non-concentrated epoch; it binds only when one identity alone holds >⅔ of
// frozen weight, the "honest whale" corner this test exercises directly).
//
// THE FIXTURE reaches `c.matureEpoch == true` at genesis (no `Anchors`
// configured, `MatureValidators: 0` sheds the training wheels immediately),
// mirroring `sim/maturequorum_test.go`'s own construction and its own
// premise check (`ch.RequiredQuorum() == cfg.Quorum` right after genesis —
// confirmed again below, since that equality is what makes this arm's HEAD
// behaviour ErrNoQuorum rather than something else).
//
// (i) RED-FIRST (at e443548, before direction (1) landed): a block proposed by
// the WHALE (>⅔ of frozen weight alone) with ZERO attestations failed the old
// count floor (`RequiredQuorum() = cfg.Quorum = 1 > len(seen) = 0`) with
// `ErrNoQuorum` before `requireEpochWeightQuorum` (which accepts it) was ever
// reached. GREEN once regime (b) returns 0: the commit is ACCEPTED. Ablation:
// restore `return q` (the old regime-(b) leg) in RequiredQuorum ⇒ RED here,
// by name (ErrNoQuorum), and the premise below reddens first.
//
// (ii) CONTROL, GREEN before and after: a block proposed by a SMALL validator
// with one small-validator attester (support well under ⅔ of frozen weight)
// clears the count floor either way (1 before, 0 after) and is refused BY
// NAME — `ErrNoQuorumWeight` (chain.go:3116-3117) — proving floor 0 does not
// delete the weight bar.
func TestG_H43_8c_MatureEpochWeightAloneAdmitsZeroAttestationCommit(t *testing.T) {
	whale, v1, v2 := key(4801), key(4802), key(4803)
	const (
		whaleWeight = int64(10) << 20
		smallWeight = int64(1) << 20
	)
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 0, EpochBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs,
		bondReg(whale, whaleWeight, ports.Hash{}),
		bondReg(v1, smallWeight, ports.Hash{}),
		bondReg(v2, smallWeight, ports.Hash{}))
	Sign(g, whale)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("append genesis: %v", err)
	}

	// Premise: the epoch is mature at genesis (no Anchors configured,
	// MatureValidators: 0). The RequiredQuorum()/cfg.Quorum premise is checked
	// AFTER the accept below, so an ablation reddens on the mechanism first.
	if !c.matureEpoch {
		t.Fatalf("premise: expected c.matureEpoch == true at genesis (MatureValidators: 0, no Anchors)")
	}
	total := whaleWeight + 2*smallWeight
	if 3*whaleWeight <= 2*total {
		t.Fatalf("premise: the whale's weight (%d) must exceed 2/3 of the frozen total (%d) alone", whaleWeight, total)
	}

	// (i) — the whale proposes, ZERO non-proposer attestations.
	prev := g.Hash()
	whaleBlock := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(whaleBlock, whale)
	// No Atts at all — the coalition is {whale} alone.
	err := c.Append(*whaleBlock)
	// The healthy assertion: this commit is ACCEPTED. A RED here by name
	// (ErrNoQuorum) is the count floor shadowing the weight rule again — the
	// old regime-(b) leg restored.
	if err != nil {
		t.Fatalf("G-H43-8 arm 8c(i) REGRESSED: a commit whose sole proposer (the whale) alone carries %d of "+
			"%d frozen weight (%.1f%%, > 2/3) was REFUSED (%v) instead of accepted — RequiredQuorum() must "+
			"return 0 in the mature regime (predicate certification §1 row (b)) so a local, "+
			"non-replicated count floor never shadows the weight rule that carries the Byzantine bar",
			whaleWeight, total, 100*float64(whaleWeight)/float64(total), err)
	}
	if _, h := c.Head(); h != 2 {
		t.Fatalf("G-H43-8 arm 8c(i): the whale block was accepted but the head is %d, want 2", h)
	}
	// Premise, after the fact: RequiredQuorum() is 0 in this regime while the
	// local cfg.Quorum is HIGHER — so the accept above is attributable to the
	// derived floor, not to a config that already asked for nothing.
	if got := c.RequiredQuorum(); got != 0 || cfg.Quorum <= 0 {
		t.Fatalf("premise: RequiredQuorum() in the mature regime must be 0 (#380 regime (b)) with cfg.Quorum (%d) above it; got %d", cfg.Quorum, got)
	}
	t.Logf("G-H43-8 arm 8c(i): zero-attestation, >2/3-frozen-weight commit ACCEPTED in the mature regime (RequiredQuorum()=0, cfg.Quorum=%d)", cfg.Quorum)
}

// TestG_H43_8c_ControlBelowWeightBarStillRefusedByName is arm 8c's CONTROL
// half (ii): floor 0 must not delete requireEpochWeightQuorum's bar. GREEN
// today and after the predicate change lands — this is the "the fix did not
// also delete the thing it was never supposed to touch" check the
// certification requires alongside (i).
func TestG_H43_8c_ControlBelowWeightBarStillRefusedByName(t *testing.T) {
	whale, v1, v2 := key(4811), key(4812), key(4813)
	const (
		whaleWeight = int64(10) << 20
		smallWeight = int64(1) << 20
	)
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 0, EpochBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs,
		bondReg(whale, whaleWeight, ports.Hash{}),
		bondReg(v1, smallWeight, ports.Hash{}),
		bondReg(v2, smallWeight, ports.Hash{}))
	Sign(g, whale)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("append genesis: %v", err)
	}
	if !c.matureEpoch {
		t.Fatalf("premise: expected c.matureEpoch == true at genesis")
	}

	// Two SMALL validators propose/attest — a coalition well under 2/3 of the
	// frozen total (2 of 12 MiB), but with len(seen) = 1, clearing BOTH the
	// count floor of 1 (today) and 0 (after the fix), so this reaches the
	// weight check under either predicate.
	prev := g.Hash()
	smallBlock := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(2)}}
	Sign(smallBlock, v1)
	smallBlock.Atts = []Attestation{Attest(smallBlock, v2)}
	err := c.Append(*smallBlock)
	total := whaleWeight + 2*smallWeight
	support := 2 * smallWeight
	if 3*support > 2*total {
		t.Fatalf("premise: the two-small-validator coalition (%d) must be BELOW 2/3 of the frozen total (%d)", support, total)
	}
	if err == nil {
		t.Fatalf("G-H43-8 arm 8c(ii) CONTROL VIOLATION: a %d-of-%d (%.1f%%, < 2/3) weight coalition was "+
			"ACCEPTED — the weight bar (requireEpochWeightQuorum) must refuse this regardless of the count "+
			"floor's value", support, total, 100*float64(support)/float64(total))
	}
	if !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("G-H43-8 arm 8c(ii) CONTROL: got %v, want ErrNoQuorumWeight (chain.go:3116-3117) — refused "+
			"for the wrong reason, so this control is not actually pinning the weight bar", err)
	}
	t.Logf("G-H43-8 arm 8c(ii) control confirmed (GREEN, as required today and after): %v", err)
}

// ── G-H43-8, arm 8d (M-380-1) — Reload symmetry ─────────────────────────────
//
// TestG_H43_8d_ReloadMustAcceptWhatTheDerivedFloorAccepted is arm 8d, per the
// predicate certification §2 row `chain.go:3388` and §5 arm 8d: "A block
// accepted by ValidateCommit under the derived floor on a node whose
// cfg.Quorum is higher must Reload on that same node. Ablation: leave
// chain.go:3388 at cfg.Quorum ⇒ RED (the replay fails; with D-RC-SCOPE-S3 the
// daemon then refuses to start)." M-380-1 is a MERGE-BLOCKING condition on
// direction (1), not an optional follow-up: "Without it, direction (1)
// creates a live-accept / Reload-reject split... a new failure mode
// manufactured by this change."
//
// THE SPLIT: `Append`/`ValidateCommit` (chain.go:3226, :2911) reach quorum
// through `requireQuorumStack` → `RequiredQuorum()` — the function direction
// (1) changes. `Reload` (chain.go:3242) instead calls `appendStructural` →
// `validateStructural` (chain.go:3340), whose OWN, SEPARATE quorum check
// (chain.go:3388-3389) reads the BARE `c.cfg.Quorum` and has NEVER called
// `RequiredQuorum()` at all — not even today, and direction (1) does not
// touch this line by itself. This test does not need the predicate fix
// itself to exist: it isolates `validateStructural`'s formula directly, by
// asking it to replay a block shaped EXACTLY as the fixed `ValidateCommit`
// would accept (arm 8c(i)'s own whale block: mature epoch, proposer alone
// holding >⅔ of frozen weight, ZERO non-proposer attestations, cfg.Quorum: 1
// — already higher than the derived floor of 0 arm 8c argues for).
//
// RED at HEAD: `Reload` refuses the whale block at `validateStructural`
// (chain.go:3388-3389, `valid(0) < c.cfg.Quorum(1)`) with `ErrNoQuorum` —
// exactly the split M-380-1 names. Per `D-RC-SCOPE-S3`, a failed replay is
// what makes the daemon refuse to start (`daemon.go:881-886`) unless the
// operator passes `-accept-chain-loss` — so this is not merely a test
// failure shape, it is the manufactured OUTAGE M-380-1 exists to prevent.
func TestG_H43_8d_ReloadMustAcceptWhatTheDerivedFloorAccepted(t *testing.T) {
	whale, v1, v2 := key(4821), key(4822), key(4823)
	const (
		whaleWeight = int64(10) << 20
		smallWeight = int64(1) << 20
	)
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 0, EpochBlocks: 4}

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs,
		bondReg(whale, whaleWeight, ports.Hash{}),
		bondReg(v1, smallWeight, ports.Hash{}),
		bondReg(v2, smallWeight, ports.Hash{}))
	Sign(g, whale)

	whaleBlock := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)}}
	Sign(whaleBlock, whale)
	// No Atts — the coalition is {whale} alone, exactly arm 8c(i)'s shape:
	// the block a FIXED ValidateCommit accepts (>2/3 weight, floor 0 in the
	// mature regime) but which cfg.Quorum: 1 (higher than 0) still names as
	// "need 1" in validateStructural's bare check.

	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	n, err := c.Reload([]Block{*g, *whaleBlock})
	if err != nil {
		if n != 1 {
			t.Fatalf("premise: expected Reload to fail exactly at block index 1 (the whale block), failed at index %d", n)
		}
		if !errors.Is(err, ErrNoQuorum) {
			t.Fatalf("G-H43-8 arm 8d mechanism pin FAILED: got %v, want ErrNoQuorum (chain.go:3388-3389) — "+
				"this RED is not attributable to the validateStructural/RequiredQuorum split", err)
		}
		t.Logf("G-H43-8 arm 8d mechanism pin confirmed (validateStructural's count leg refused by name — either it "+
			"still reads the bare cfg.Quorum, or RequiredQuorum() is not 0 in the mature regime): %v", err)
		t.Fatalf("G-H43-8 arm 8d REPRODUCED (M-380-1): Reload refused a block shaped exactly as the FIXED "+
			"ValidateCommit would accept (mature epoch, whale proposer alone >2/3 frozen weight, 0 "+
			"attestations) — got %v after replaying %d/%d blocks. validateStructural (chain.go:3388) must "+
			"move to RequiredQuorum() in the SAME commit as the predicate change, or direction (1) creates "+
			"a live-accept / Reload-reject split that (per D-RC-SCOPE-S3) refuses the daemon's own restart",
			err, n, 2)
		return
	}
	if n != 2 {
		t.Fatalf("G-H43-8 arm 8d: Reload reported success but restored %d blocks, want 2", n)
	}
	t.Logf("G-H43-8 arm 8d: Reload accepted the whale block — validateStructural already tracks RequiredQuorum().")
}
