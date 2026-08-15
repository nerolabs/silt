package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Era-2 (#432 rounds) chain-layer rules, per the research certification:
// a v2 commit carries TWO quorums at the same (Height, CommitRound) — the
// prepare-QC and the precommit certificate — each at the full commit
// threshold; signatures at the wrong phase or round are refused (that refusal
// is what excludes the S1 delayed-quorum and S2 replay shapes at the
// validation layer); the round is NOT part of block identity (re-proposal
// across rounds preserves the hash); and equivocation is scoped to the same
// (height, round, phase).

// roundsWorld builds a 4-anchor objective launch chain (the field shape:
// strict anchor majority 3-of-4, #402) with an era-1 genesis, plus the anchor
// keys. Genesis stays v1 — a v2 block extends a v1 history (the upgrade path).
func roundsWorld(t *testing.T) (*Chain, []ed25519.PrivateKey, *Block) {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range keys {
		keys[i] = key(int64(11000 + i))
		anchors[idOf(keys[i])] = true
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	return c, keys, g
}

// v2Block builds an era-2 block at height 1 with a full two-phase certificate
// at the given round: proposer keys[0], whose REQUIRED round-scoped
// self-prepare (requireProposerPrepare) and count-neutral self-precommit lead
// each set; prepare + precommit signatures from the three non-proposer anchors
// (3-of-4 = the strict anchor majority with the proposer counted; non-proposer
// sigs alone also reach 3 only with all three).
func v2Block(g *Block, keys []ed25519.PrivateKey, round uint64) *Block {
	b := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(9)}}
	Sign(b, keys[0])
	b.CommitRound = round
	b.PrepareQC = append(b.PrepareQC, AttestAt(b, keys[0], round, PhasePrepare))
	for _, k := range keys[1:] {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, round, PhasePrepare))
	}
	b.Atts = append(b.Atts, AttestAt(b, keys[0], round, PhasePrecommit))
	for _, k := range keys[1:] {
		b.Atts = append(b.Atts, AttestAt(b, k, round, PhasePrecommit))
	}
	return b
}

func TestV2CommitRequiresBothQuorums(t *testing.T) {
	c, keys, g := roundsWorld(t)

	// The full two-phase certificate validates.
	if err := c.ValidateCommit(v2Block(g, keys, 0)); err != nil {
		t.Fatalf("full v2 certificate must validate: %v", err)
	}
	// So does one at a NON-ZERO round — the whole point of #432 is that a
	// later round can commit.
	if err := c.ValidateCommit(v2Block(g, keys, 3)); err != nil {
		t.Fatalf("v2 certificate at round 3 must validate: %v", err)
	}

	// No prepare-QC ⇒ no commit, no matter how many precommits: the prepare
	// phase is what makes at most one value preparable per round (S2's block).
	// The structural author-prepare check fires first (an empty QC lacks that
	// too); the quorum layer's refusal on a thin QC is asserted just below.
	noPrep := v2Block(g, keys, 0)
	noPrep.PrepareQC = nil
	if err := c.ValidateCommit(noPrep); !errors.Is(err, ErrProposerPrepare) {
		t.Fatalf("v2 commit without a prepare-QC must be refused, got: %v", err)
	}

	// A prepare-QC below the commit threshold ⇒ refused (POL threshold IS the
	// commit threshold — certification §4). One counted (non-author) signature
	// trips the count quorum first (the stack refuses at its first unmet
	// layer); the author's self-prepare stays — it satisfies
	// requireProposerPrepare but counts toward nothing.
	thinPrep := v2Block(g, keys, 0)
	thinPrep.PrepareQC = thinPrep.PrepareQC[:2]
	if err := c.ValidateCommit(thinPrep); !errors.Is(err, ErrNoQuorum) {
		t.Fatalf("v2 commit with a thin prepare-QC must fail ErrNoQuorum, got: %v", err)
	}
}

func TestV2RefusesCrossPhaseAndCrossRound(t *testing.T) {
	c, keys, g := roundsWorld(t)

	// Prepare signatures cannot serve as the precommit certificate (phase
	// replay — the S2 shape at the validation layer).
	phased := v2Block(g, keys, 0)
	phased.Atts = phased.PrepareQC
	if err := c.ValidateCommit(phased); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("prepare sigs in the precommit slot must fail ErrBadSignature, got: %v", err)
	}

	// A signature from round 1 cannot complete a round-0 certificate (round
	// replay — the S1 delayed-quorum shape at the validation layer). Index 1:
	// a COUNTED attester slot (index 0 is the author's exempt self-precommit).
	crossRound := v2Block(g, keys, 0)
	crossRound.Atts[1] = AttestAt(crossRound, keys[1], 1, PhasePrecommit)
	if err := c.ValidateCommit(crossRound); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a round-1 sig in a round-0 certificate must fail ErrBadSignature, got: %v", err)
	}

	// A legacy bare-hash attestation carries no (round, phase) scope and is
	// refused inside an era-2 certificate.
	legacyAtt := v2Block(g, keys, 0)
	legacyAtt.Atts[1] = Attest(legacyAtt, keys[1])
	if err := c.ValidateCommit(legacyAtt); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a legacy attestation in an era-2 certificate must fail ErrBadSignature, got: %v", err)
	}
}

// The era-2 analogue of the structural ProposerSig: a v2 commit REQUIRES the
// author's round-scoped prepare in its PrepareQC (requireProposerPrepare).
// Without this rule a double-proposer that withholds its own signatures forks
// the launch chain with ZERO slashable evidence — it counts toward both
// anchor quorums by authorship alone (the omit-loophole; era 1 never had it
// because ProposerSig is structural). With it, wanting a commit forces the
// author to leave the same-(h, r, prepare) signature the slash rule catches.
func TestV2CommitRequiresProposerPrepare(t *testing.T) {
	c, keys, g := roundsWorld(t)

	// Strip the author's self-prepare: a certificate that satisfies BOTH
	// quorums still refuses to commit — the omit-loophole closed.
	omit := v2Block(g, keys, 0)
	omit.PrepareQC = omit.PrepareQC[1:]
	if err := c.ValidateCommit(omit); !errors.Is(err, ErrProposerPrepare) {
		t.Fatalf("a v2 commit without the author's prepare must fail ErrProposerPrepare, got: %v", err)
	}

	// The carried re-proposal shape: block authored (and self-prepared) at
	// round 0, committed at round 2 by a fresh certificate — the author's
	// round-0 prepare rides in the fresh QC (hash excludes the round; the
	// author is exempt from round-exactness) and satisfies the rule at
	// Round ≤ CommitRound. This is the dead-author view-change escape: the
	// rule must never re-wedge what #432 unwedged.
	carried := v2Block(g, keys, 2)
	carried.PrepareQC[0] = AttestAt(carried, keys[0], 0, PhasePrepare)
	if err := c.ValidateCommit(carried); err != nil {
		t.Fatalf("an author prepare at a LOWER round than CommitRound must satisfy the rule (the carried lock): %v", err)
	}

	// An author prepare only at a HIGHER round than the commit round does not
	// endorse this commit (nothing at ≤ CommitRound) — refused.
	future := v2Block(g, keys, 0)
	future.PrepareQC[0] = AttestAt(future, keys[0], 3, PhasePrepare)
	if err := c.ValidateCommit(future); !errors.Is(err, ErrProposerPrepare) {
		t.Fatalf("an author prepare only at round > CommitRound must fail ErrProposerPrepare, got: %v", err)
	}

	// A self-prepare in the PRECOMMIT set is the wrong phase/slot — the rule
	// reads PrepareQC only.
	misplaced := v2Block(g, keys, 0)
	misplaced.PrepareQC = misplaced.PrepareQC[1:]
	misplaced.Atts = append(misplaced.Atts, AttestAt(misplaced, keys[0], 0, PhasePrepare))
	if err := c.ValidateCommit(misplaced); !errors.Is(err, ErrProposerPrepare) {
		t.Fatalf("an author prepare outside PrepareQC must not satisfy the rule, got: %v", err)
	}
}

// The proposer's self-signatures are COUNT-NEUTRAL (#402 arithmetic
// preserved): the proposer is counted by authorship exactly once
// (countAnchorSupport / the weight quorum), and its certificate signatures
// add nothing — self-signatures can never substitute for a missing attester,
// no matter how many are stapled on.
func TestV2ProposerSelfSigsAreCountNeutral(t *testing.T) {
	c, keys, g := roundsWorld(t)

	// Baseline sanity: the full certificate (3 non-author anchors) commits.
	if err := c.ValidateCommit(v2Block(g, keys, 0)); err != nil {
		t.Fatalf("full certificate must validate: %v", err)
	}

	// One counted attester + proposer = 2 anchors < the strict majority 3
	// (one attester short of the tolerated 2+proposer minimum). Pad both sets
	// back to full length with duplicate author self-signatures: the quorum
	// math must still see exactly one attester and refuse — identical to no
	// padding at all.
	short := v2Block(g, keys, 0)
	short.PrepareQC = append(short.PrepareQC[:2], AttestAt(short, keys[0], 0, PhasePrepare), AttestAt(short, keys[0], 0, PhasePrepare))
	short.Atts = append(short.Atts[:2], AttestAt(short, keys[0], 0, PhasePrecommit), AttestAt(short, keys[0], 0, PhasePrecommit))
	padded := c.ValidateCommit(short)
	if padded == nil {
		t.Fatal("author self-signatures must never substitute for a missing attester (#402: size-set == membership-set)")
	}
	bare := v2Block(g, keys, 0)
	bare.PrepareQC = bare.PrepareQC[:2]
	bare.Atts = bare.Atts[:2]
	unpadded := c.ValidateCommit(bare)
	if unpadded == nil || padded.Error() != unpadded.Error() {
		t.Fatalf("padding with self-signatures must change nothing: padded=%v, unpadded=%v", padded, unpadded)
	}
	// And the tolerated minimum still commits (2 attesters + proposer = 3):
	// the rule must not RAISE the quorum either — fault tolerance unchanged.
	min3 := v2Block(g, keys, 0)
	min3.PrepareQC = min3.PrepareQC[:3]
	min3.Atts = min3.Atts[:3]
	if err := c.ValidateCommit(min3); err != nil {
		t.Fatalf("2 attesters + proposer = the strict anchor majority must still commit: %v", err)
	}
}

// The round is NOT part of block identity: the same value re-proposed at a
// higher round (after a view-change) must hash identically — the view-change's
// "re-propose the locked value" depends on it (Tendermint separates the vote's
// (h, r, block-id) from the block content for exactly this reason).
func TestRoundNotInBlockIdentity(t *testing.T) {
	_, keys, g := roundsWorld(t)
	a := v2Block(g, keys, 0)
	b := v2Block(g, keys, 7)
	if a.Hash() != b.Hash() {
		t.Fatal("CommitRound/certificates must be excluded from Hash — the same value at different rounds changed identity, which breaks re-proposal after a view-change")
	}
}

// Era-2 equivocation is (height, round, phase)-scoped: the same validator
// precommitting two different values at the SAME (h, r) is slashable; the same
// two signatures across DIFFERENT rounds (an honest POL-justified lock-change)
// or different phases are NOT; and a proposer's bare-hash authorship signature
// alone is never consensus evidence in era 2.
func TestV2EquivocationRoundScoped(t *testing.T) {
	_, keys, g := roundsWorld(t)
	culprit := keys[1]

	mk := func(tag byte, round uint64, phase uint8) Block {
		b := Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(tag)}}
		Sign(&b, keys[0])
		b.Atts = append(b.Atts, AttestAt(&b, culprit, round, phase))
		return b
	}

	samePR := &Equivocation{Culprit: culprit.Public().(ed25519.PublicKey), A: mk(1, 2, PhasePrecommit), B: mk(2, 2, PhasePrecommit)}
	if !VerifyEquivocation(samePR) {
		t.Fatal("two different-hash precommits at the same (h, r) must be slashable equivocation")
	}
	crossRound := &Equivocation{Culprit: culprit.Public().(ed25519.PublicKey), A: mk(1, 2, PhasePrecommit), B: mk(2, 3, PhasePrecommit)}
	if VerifyEquivocation(crossRound) {
		t.Fatal("different-hash precommits at DIFFERENT rounds are an honest lock-change, never slashable (I5 under #432)")
	}
	crossPhase := &Equivocation{Culprit: culprit.Public().(ed25519.PublicKey), A: mk(1, 2, PhasePrepare), B: mk(2, 2, PhasePrecommit)}
	if VerifyEquivocation(crossPhase) {
		t.Fatal("a prepare and a precommit at one (h, r) are two phases of one honest flow, never slashable")
	}

	// Proposer authorship alone (no consensus signature by the culprit):
	// re-proposing fresh values across rounds is honest in era 2.
	pa := Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(3)}}
	Sign(&pa, culprit)
	pb := Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(4)}}
	Sign(&pb, culprit)
	author := &Equivocation{Culprit: culprit.Public().(ed25519.PublicKey), A: pa, B: pb}
	if VerifyEquivocation(author) {
		t.Fatal("era-2 authorship signatures alone must not be slashable (the consensus vote is the phase-scoped attestation)")
	}

	// But a COMMITTED double-proposal always carries the author's required
	// self-prepares (requireProposerPrepare), and two of those at one (h, r)
	// over different hashes ARE the slash evidence — the #345/#378 drill
	// shape, restored in era 2 via the round-scoped prepare.
	pa.PrepareQC = append(pa.PrepareQC, AttestAt(&pa, culprit, 0, PhasePrepare))
	pb.PrepareQC = append(pb.PrepareQC, AttestAt(&pb, culprit, 0, PhasePrepare))
	if !VerifyEquivocation(&Equivocation{Culprit: culprit.Public().(ed25519.PublicKey), A: pa, B: pb}) {
		t.Fatal("a double-proposer's same-(h, r) self-prepares over different hashes must be slashable (#345/#378 in era 2)")
	}

	// The I5 mirror: an honest proposer whose round-0 value died re-proposes
	// FRESH at round 1 — self-prepares at DIFFERENT rounds, never slashable.
	pc := Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(7)}}
	Sign(&pc, culprit)
	pc.PrepareQC = append(pc.PrepareQC, AttestAt(&pc, culprit, 1, PhasePrepare))
	if VerifyEquivocation(&Equivocation{Culprit: culprit.Public().(ed25519.PublicKey), A: pa, B: pc}) {
		t.Fatal("an honest cross-round re-proposal (self-prepares at different rounds) must never be slashable (I5)")
	}

	// Mixed eras are never evidence (fail-safe at the upgrade boundary).
	v1 := Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(5)}}
	Sign(&v1, keys[0])
	v1.Atts = append(v1.Atts, Attest(&v1, culprit))
	mixed := &Equivocation{Culprit: culprit.Public().(ed25519.PublicKey), A: v1, B: mk(6, 0, PhasePrecommit)}
	if VerifyEquivocation(mixed) {
		t.Fatal("a cross-era signature pair must never be slashable (fail-safe)")
	}
}
