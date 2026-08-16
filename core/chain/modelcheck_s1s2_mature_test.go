package chain

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — the #432 S1/S2 faces in the MATURE (weight-quorum)
// regime, at the validation layer (certification §5.4/§5.5: "over BOTH the
// launch count-quorum and the mature weight-quorum"; §4: "the POL threshold IS
// the commit threshold in both regimes").
//
// The full adversarial S1/S2 delivery schedules run at tier 2 over the launch
// regime (core/node/modelcheck_s1s2_test.go) — the locking code paths are
// regime-independent; what changes in the mature phase is the quorum
// PREDICATE. These tests pin that predicate's era-2 face over the real frozen
// weighted epoch (the B2 world): a weight-short prepare-QC can never justify a
// precommit no matter its head count (S2's forged/packed lock, in weight), and
// a cross-round signature can never complete a certificate (S1's delayed
// quorum, in weight). The DYNAMIC node-level mature schedules (a live epoch
// network under held delivery) are a named residual — see
// docs/thinking/2026-08-16-432-s1-s2-oracles.md — not a silent gap.

// TestModelCheck_S2Face_MatureWeightShortPrepareQCRefused: in a mature epoch,
// a prepare-QC packed with the head-count-rich, weight-poor sybil cohort must
// be refused at the commit gate even when the PRECOMMIT certificate carries
// full honest weight — the POL threshold is the commit threshold in WEIGHT, so
// a Byzantine cohort cannot manufacture a "prepared" state the weight rule
// would never commit (S2's misreport face, weighted).
func TestModelCheck_S2Face_MatureWeightShortPrepareQCRefused(t *testing.T) {
	c, honest, sybil := matureWeightedEpoch(t)
	prev, h := c.Head()

	mk := func() *Block {
		b := &Block{Version: BlockVersionRounds, Height: h, Prev: prev, Entries: []ports.Entry{entry(0xE1)}}
		Sign(b, honest[0])
		b.CommitRound = 0
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, honest[0], 0, PhasePrepare))
		return b
	}

	// Control (the setup check): honest weight in BOTH quorums validates —
	// proposer (20 MiB) + two honest attesters (40 MiB) = 60 MiB > ⅔·76.
	good := mk()
	for _, k := range honest[1:] {
		good.PrepareQC = append(good.PrepareQC, AttestAt(good, k, 0, PhasePrepare))
		good.Atts = append(good.Atts, AttestAt(good, k, 0, PhasePrecommit))
	}
	good.Atts = append(good.Atts, AttestAt(good, honest[0], 0, PhasePrecommit))
	if err := c.ValidateCommit(good); err != nil {
		t.Fatalf("setup: an honest-weight two-quorum v2 commit must validate in the mature epoch: %v", err)
	}

	// S2 face: the prepare-QC is the FULL SYBIL COHORT (8 heads, 16 MiB — a
	// head-count quorum by the pre-B2 rule) while the precommits carry full
	// honest weight. The weight-short POL must be refused.
	packed := mk()
	for _, k := range sybil {
		packed.PrepareQC = append(packed.PrepareQC, AttestAt(packed, k, 0, PhasePrepare))
	}
	for _, k := range honest[1:] {
		packed.Atts = append(packed.Atts, AttestAt(packed, k, 0, PhasePrecommit))
	}
	packed.Atts = append(packed.Atts, AttestAt(packed, honest[0], 0, PhasePrecommit))
	if err := c.ValidateCommit(packed); !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("a weight-short (sybil-packed) prepare-QC must fail ErrNoQuorumWeight — the POL threshold is the commit threshold in WEIGHT (certification §4), got: %v", err)
	}

	// And the same weight-short set can never verify as a round-change lock
	// (S2's forged-lock death in the mature regime — verifyRoundChange leans on
	// exactly this check).
	if err := c.VerifyPrepareQC(packed, packed.PrepareQC, 0); !errors.Is(err, ErrNoQuorumWeight) {
		t.Fatalf("a weight-short prepare-QC must never verify as a lock (VerifyPrepareQC), got: %v", err)
	}
}

// TestModelCheck_S1Face_MatureCrossRoundSignatureRefused: a signature from a
// LOWER round can never complete a higher-round certificate in the mature
// regime — S1's delayed-quorum shape at the weighted validation layer. The
// author's own carried self-prepare is the one deliberate exemption (it counts
// toward nothing).
func TestModelCheck_S1Face_MatureCrossRoundSignatureRefused(t *testing.T) {
	c, honest, _ := matureWeightedEpoch(t)
	prev, h := c.Head()

	b := &Block{Version: BlockVersionRounds, Height: h, Prev: prev, Entries: []ports.Entry{entry(0xE2)}}
	Sign(b, honest[0])
	b.CommitRound = 1
	// Author self-prepare at round 0 — the CARRIED re-proposal shape: exempt,
	// count-neutral, and required (requireProposerPrepare, round ≤ CommitRound).
	b.PrepareQC = append(b.PrepareQC, AttestAt(b, honest[0], 0, PhasePrepare))
	for _, k := range honest[1:] {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, 1, PhasePrepare))
		b.Atts = append(b.Atts, AttestAt(b, k, 1, PhasePrecommit))
	}
	if err := c.ValidateCommit(b); err != nil {
		t.Fatalf("setup: the round-1 certificate with the carried author prepare must validate: %v", err)
	}

	// Replace one counted round-1 precommit with the same signer's ROUND-0
	// precommit (the delayed lower-round signature): the certificate must be
	// refused — a stale-round signature never completes a mature commit.
	b.Atts[0] = AttestAt(b, honest[1], 0, PhasePrecommit)
	if err := c.ValidateCommit(b); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a round-0 precommit inside a round-1 certificate must fail ErrBadSignature (S1's delayed-quorum face, weighted), got: %v", err)
	}
}
