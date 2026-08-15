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
// at the given round: proposer keys[0]; prepare + precommit signatures from
// the three non-proposer anchors (3-of-4 = the strict anchor majority with the
// proposer counted; non-proposer sigs alone also reach 3 only with all three).
func v2Block(g *Block, keys []ed25519.PrivateKey, round uint64) *Block {
	b := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(9)}}
	Sign(b, keys[0])
	b.CommitRound = round
	for _, k := range keys[1:] {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, round, PhasePrepare))
	}
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
	noPrep := v2Block(g, keys, 0)
	noPrep.PrepareQC = nil
	if err := c.ValidateCommit(noPrep); !errors.Is(err, ErrNoQuorum) {
		t.Fatalf("v2 commit without a prepare-QC must fail ErrNoQuorum, got: %v", err)
	}

	// A prepare-QC below the commit threshold ⇒ refused (POL threshold IS the
	// commit threshold — certification §4). One signature trips the count
	// quorum first (the stack refuses at its first unmet layer).
	thinPrep := v2Block(g, keys, 0)
	thinPrep.PrepareQC = thinPrep.PrepareQC[:1]
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
	// replay — the S1 delayed-quorum shape at the validation layer).
	crossRound := v2Block(g, keys, 0)
	crossRound.Atts[0] = AttestAt(crossRound, keys[1], 1, PhasePrecommit)
	if err := c.ValidateCommit(crossRound); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a round-1 sig in a round-0 certificate must fail ErrBadSignature, got: %v", err)
	}

	// A legacy bare-hash attestation carries no (round, phase) scope and is
	// refused inside an era-2 certificate.
	legacyAtt := v2Block(g, keys, 0)
	legacyAtt.Atts[0] = Attest(legacyAtt, keys[1])
	if err := c.ValidateCommit(legacyAtt); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a legacy attestation in an era-2 certificate must fail ErrBadSignature, got: %v", err)
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

	// Mixed eras are never evidence (fail-safe at the upgrade boundary).
	v1 := Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(5)}}
	Sign(&v1, keys[0])
	v1.Atts = append(v1.Atts, Attest(&v1, culprit))
	mixed := &Equivocation{Culprit: culprit.Public().(ed25519.PublicKey), A: v1, B: mk(6, 0, PhasePrecommit)}
	if VerifyEquivocation(mixed) {
		t.Fatal("a cross-era signature pair must never be slashable (fail-safe)")
	}
}
