package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — tier 1, #357 launch replay (fork-choice / no-reorg-of-final).
//
// #357 (research cert 2026-08-13): the "reorged onto a heavier fork, new head height 0"
// was a zero-weight tiebreak degeneracy — during the drain window anchor attesters have
// bonded=0, so a committed chain weighed ≈0 and `heavier()` fell to a height-blind hash
// tiebreak that a bare-genesis fork could win, dropping committed blocks. The fix that
// stands: a quorum-finality gate makes a super-quorum-committed block irreversible —
// fork-choice chooses only among descendants of the finalized head (D-1) — and the §1b
// height preference, now the PRIMARY fork-choice term (height → head-hash, O3 Direction T;
// the anchor bootstrap weight that shipped alongside them is retired).
//
// This oracle asserts the invariant that closes #357: over adversarial competing forks
// (shorter, genesis, and even TALLER conflicting), a super-quorum-finalized launch block
// is NEVER reorged, and the outcome is order-independent (fork-choice is a pure function).
// Chain-level, deterministic.
//
// FAILING-FIRST (verified by controlled revert): with finalityQuorumActive forced false
// (the pre-#357 no-finality-gate state), the taller conflicting fork is adopted by
// heavier() and the finalized chain is reorged — RED. With the shipped gate — GREEN.

// ramp357 builds a committed anchor-attested launch chain [g, b1@1, b2@2] (4 anchors,
// zero real bonds — the drain-window regime) and returns it plus the anchor keys, so a
// test can construct valid competing forks. Head is at height 3 (Head()==3).
func ramp357(t *testing.T) (*Chain, []ed25519.PrivateKey, *Block) {
	t.Helper()
	ak := make([]ed25519.PrivateKey, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range ak {
		ak[i] = key(int64(9200 + i))
		anchors[idOf(ak[i])] = true
	}
	cfg := Config{Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, MatureValidators: 99}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, ak[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	prev := g.Hash()
	for h := byte(1); h <= 2; h++ {
		b := &Block{Version: 1, Height: uint64(h), Prev: prev, Entries: []ports.Entry{entry(h)}}
		Sign(b, ak[0])
		b.Atts = []Attestation{Attest(b, ak[1]), Attest(b, ak[2])}
		if err := c.Append(*b); err != nil {
			t.Fatalf("commit ramp block %d: %v", h, err)
		}
		prev = b.Hash()
	}
	return c, ak, g
}

// anchorFork builds a competing anchor-attested fork of `height` blocks from genesis g,
// with distinct entries (tag) so it conflicts with the committed chain. Valid on its own
// (each block anchor-proposed + 2 anchor attesters), so only the finality gate — not
// validation — can refuse it.
func anchorFork(g *Block, ak []ed25519.PrivateKey, height int, tag byte) []Block {
	out := []Block{*g}
	prev := g.Hash()
	for h := 1; h <= height; h++ {
		b := Block{Version: 1, Height: uint64(h), Prev: prev, Entries: []ports.Entry{entry(tag + byte(h))}}
		Sign(&b, ak[0])
		b.Atts = []Attestation{Attest(&b, ak[1]), Attest(&b, ak[2])}
		out = append(out, b)
		prev = b.Hash()
	}
	return out
}

// TestModelCheck_357_NoReorgOfFinalizedLaunchBlock is the #357 oracle: a super-quorum-
// finalized launch chain is never reorged by any competing fork, and the result is
// order-independent.
func TestModelCheck_357_NoReorgOfFinalizedLaunchBlock(t *testing.T) {
	c, ak, g := ramp357(t)
	finalHead, finalHeight := c.Head()
	if finalHeight != 3 {
		t.Fatalf("setup: committed head must be at height 3 (Head()==3), got %d", finalHeight)
	}
	// (The "#357 weight face" positive control — `Weight() > 0` — that stood here was
	// DELETED 2026-09-04 with the weight term (O3 Direction T; R-FORKCHOICE-RAMP-GUARD's
	// twin site). It asserted a property production does not have: green only because this
	// fixture hand-builds Version: 1 blocks with era-1 attestations. The oracle below is the
	// part that guards #357 and it stands.)

	// Adversarial competing forks, each re-Reconciled against the finalized chain: a bare
	// genesis, a shorter conflict, an EQUAL-height conflict, and a TALLER conflict (the
	// sharpest — a heavier/longer fork must STILL not reorg a finalized block; D-1).
	forks := map[string][]Block{
		"bare-genesis":   {*g},
		"shorter-h1":     anchorFork(g, ak, 1, 100),
		"equal-h2":       anchorFork(g, ak, 2, 110),
		"taller-h3":      anchorFork(g, ak, 3, 120),
		"much-taller-h5": anchorFork(g, ak, 5, 130),
	}
	for name, fork := range forks {
		adopted, err := c.Reconcile(fork)
		if adopted {
			t.Fatalf("#357 VIOLATION — the %q fork reorged a super-quorum-finalized launch block (D-1: finalized history is irreversible)", name)
		}
		if !errors.Is(err, ErrPreFinalityReorg) && err != nil && !errors.Is(err, ErrForeignGenesis) {
			// Refusal is expected; only a SILENT adopt (adopted==true) is the #357 failure.
			// A non-finality error (e.g. re-validation) is also a refusal — fine.
		}
		if hh, h := c.Head(); h != finalHeight || hh != finalHead {
			t.Fatalf("#357 VIOLATION — head moved after reconciling the %q fork: height %d (want %d) — finalized block was dropped", name, h, finalHeight)
		}
	}
}

// TestModelCheck_357_ForkChoiceIsOrderIndependent asserts I5 determinism: reconciling
// the same set of competing forks in different orders yields the same head — fork-choice
// is a pure function, never a hash-luck tiebreak that depends on arrival order (#357).
func TestModelCheck_357_ForkChoiceIsOrderIndependent(t *testing.T) {
	run := func(order []int) (ports.Hash, uint64) {
		c, ak, g := ramp357(t)
		forks := [][]Block{
			anchorFork(g, ak, 5, 130),
			{*g},
			anchorFork(g, ak, 2, 110),
			anchorFork(g, ak, 3, 120),
		}
		for _, i := range order {
			c.Reconcile(forks[i])
		}
		return c.Head()
	}
	h1, ht1 := run([]int{0, 1, 2, 3})
	h2, ht2 := run([]int{3, 2, 1, 0})
	h3, ht3 := run([]int{2, 0, 3, 1})
	if h1 != h2 || h1 != h3 || ht1 != ht2 || ht1 != ht3 {
		t.Fatalf("#357 I5 VIOLATION — fork-choice is order-dependent: heads %x@%d / %x@%d / %x@%d", h1, ht1, h2, ht2, h3, ht3)
	}
}
