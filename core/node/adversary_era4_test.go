package node

import (
	"testing"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// TestPlaceConflictingSignedIsEra4Aware is the regression gate for a defect that took the ONLY
// driven proof of equivocation-over-the-real-wire dark.
//
// THE FAILURE. `PlaceConflictingSigned` scans committed blocks for this node's own prepare and
// matched it against the canonical constant `chain.PhasePrepare`. Era 4 renames the two steps on
// the wire (`AttPhase`: PhasePrepare -> PhasePrepareV5), and `Era4ActivationHeight` now defaults
// to 1, so on a fresh network EVERY committed block above the genesis carries the v5 form. The
// scan matched nothing, the function returned "this node has no prepare in any committed block
// yet" on every retry, the daemon's 1-second retry loop never emitted `adversary: equivocation
// complete`, and `TestEquivocatorSlashedOverTCP` timed out after 121 s. It failed on the M1
// branch and passed on `origin/main` — that discriminator is the attribution.
//
// WHY THE EXISTING NODE-TIER GATE COULD NOT SEE IT. TestModelCheck_184_PlaceConflictingSignedSlashedOverSync
// drives the same primitive, but its fixture hard-codes `Version: chain.BlockVersionRounds` on the
// block it commits, so production's `MintVersion` never chooses the era and the v5 form is never
// reached. A fixture that pins the quantity whose production value IS the defect is blind to it.
// This test therefore lets the chain mint, and asserts the era it got.
//
// THE ABLATION. Restore `att.Phase == chain.PhasePrepare` in PlaceConflictingSigned: RED here with
// "no prepare in any committed block". Restore `Version: chain.BlockVersionRounds` on L: the
// pair still convicts (canonicalStep folds the two forms onto one slot) but the fork is no longer
// era-faithful, which the v5 assertion below names explicitly.
func TestPlaceConflictingSignedIsEra4Aware(t *testing.T) {
	nodes, ids, net, g := era4CarrierNet(t, 4, 0)
	byz := nodes[0]
	attesters := []ports.NodeID{ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}
	all := []ports.NodeID{ids[0].NodeID(), ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}

	// The Byzantine validator proposes the honest head at height 1, so its own round-scoped
	// prepare is on-chain in W.PrepareQC (requireProposerPrepare) — the signature the detection
	// matches against. The VERSION is chosen by the chain, never by this fixture.
	W := &chain.Block{Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("honest-win")}}
	var done bool
	var commitErr error
	byz.proposeBlock(W, attesters, all, 1, func(err error) { done, commitErr = true, err })
	drainHeld(t, net, fifo)
	if !done || commitErr != nil {
		t.Fatalf("the honest v5 commit did not land (done=%v): %v", done, commitErr)
	}

	// ---- ANTI-VACUITY: the regime this test claims to be in, MEASURED. ----
	head := byz.chain.Blocks(1)[0]
	if head.Version != chain.BlockVersionWitnessable {
		t.Fatalf("fixture VACUOUS: the committed head is v%d, not v5 — this gate only distinguishes "+
			"the two era forms when the chain actually minted the later one", head.Version)
	}
	selfPub := pubOf(ids[0])
	var sawV5Form, sawCanonicalForm bool
	for _, a := range head.PrepareQC {
		if string(a.PubKey) != string(selfPub) {
			continue
		}
		switch a.Phase {
		case chain.PhasePrepareV5:
			sawV5Form = true
		case chain.PhasePrepare:
			sawCanonicalForm = true
		}
	}
	if !sawV5Form || sawCanonicalForm {
		t.Fatalf("fixture VACUOUS: the proposer's own prepare must be on-chain in the V5 WIRE FORM "+
			"and in no other (v5=%v, canonical=%v). If the canonical form were present the old "+
			"scan would have found it and this gate would pass without the fix", sawV5Form, sawCanonicalForm)
	}

	// ---- THE ACT. ----
	h, err := byz.PlaceConflictingSigned()
	if err != nil {
		t.Fatalf("PlaceConflictingSigned REFUSED on an era-4 chain: %v.\n"+
			"The scan must match the prepare in the block's OWN era form (chain.AttPhase(w.Version, "+
			"chain.PhasePrepare)), not the canonical constant. With this refusal the daemon's "+
			"-equivocate loop retries forever and never double-signs, so the e2e slash gate proves "+
			"nothing about a network that is era-4 from height 1 — which every fresh network now is.", err)
	}
	if h != 1 {
		t.Fatalf("double-sign height %d, want 1", h)
	}

	// ---- THE CONSEQUENCE: the served fork is a PROVABLE double-sign, not merely a served block. ----
	fork := byz.equivServedFork
	if len(fork) == 0 {
		t.Fatal("PlaceConflictingSigned returned a height but served no fork")
	}
	L := fork[len(fork)-1]
	if L.Height != head.Height {
		t.Fatalf("the served L is at height %d, want the forked height %d", L.Height, head.Height)
	}
	if L.Version != head.Version {
		t.Fatalf("L is v%d but W is v%d — the forged fork must be minted in the era of the block it "+
			"forks, or the adversary is signing a block no validator on this network could have signed",
			L.Version, head.Version)
	}
	e := chain.Equivocation{Culprit: append([]byte(nil), selfPub...), A: head, B: L}
	if err := chain.CheckEquivocation(&e, byz.chain.ChainID(), byz.chain.EraFloor()); err != nil {
		t.Fatalf("the served fork is NOT slashable evidence: %v.\n"+
			"W and L are two different blocks at one height, both carrying this key's prepare at the "+
			"same round — if that does not convict, the harness announces a double-sign no honest "+
			"replica can act on.", err)
	}

	// The chain id is load-bearing from era 4: the same pair under ANOTHER network's id must not
	// convict, or the evidence would be portable across silt networks (the owner-call-A break).
	if err := chain.CheckEquivocation(&e, ports.HashBytes([]byte("some other silt network")), byz.chain.EraFloor()); err == nil {
		t.Fatal("the v5-form pair convicted under a FOREIGN chain id — the era-4 preimage binds the " +
			"network, so evidence minted here must be inert elsewhere")
	}
}
