package node

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// TestSeam7_LosingForkEquivocatorIsSlashedOnDetection pins the red-team seam-7 fix:
// slash a double-sign on DETECTION, not only on adoption. A validator that attests
// the canonical (heavier) head AND signs a conflicting block on a doomed/lighter
// fork is provably guilty, but the slash used to fire ONLY when a node RECONCILED
// ONTO a heavier competing fork — so a double-sign onto a fork nobody adopts (to
// confuse late joiners, split gossip, or bait a partition) cost the actor nothing.
//
// Here A holds the heavier chain [g, W@1, X@2] where the culprit attested W@1;
// B serves the LIGHTER fork [g, L@1] where the culprit signed L@1 (conflicting with
// W@1 at height 1). A syncs B: it must NOT adopt the lighter fork, yet it MUST slash
// the cross-fork double-signer.
func TestSeam7_LosingForkEquivocatorIsSlashedOnDetection(t *testing.T) {
	const bondSize = int64(2) << 20
	sched := simclock.New()
	net := simnet.New(sched, 7, simnet.DefaultConfig())

	idA := identity.FromSeed(701)
	culprit := identity.FromSeed(703) // genesis-bonded anchor that double-signs
	h2 := identity.FromSeed(705)      // honest anchor, signs only the losing fork
	pub := func(id *identity.Identity) []byte {
		return append([]byte(nil), id.Signer().Public().(ed25519.PublicKey)...)
	}

	// THREE launch anchors (idA, culprit, h2). #402 makes launch finality a STRICT
	// ANCHOR MAJORITY ⌊A/2⌋+1: for A=3 that is 2, and two 2-of-3 anchor sets can share
	// EXACTLY ONE anchor — so a single Byzantine anchor (the culprit) can still form a
	// valid competing minority fork by co-signing both sides, with the honest anchors
	// cleanly split (idA on the winner, h2 on the loser). That lone cross-fork
	// double-signer is the seam-7 target. (An EVEN anchor set would force the two
	// majorities to overlap in ≥2, so a valid competing fork would need ≥2 colluding
	// double-signers — a different, stronger-collusion scenario. A=3 keeps this test's
	// lone-culprit property under the #402 rule; detection itself runs pre-Reconcile,
	// independent of fork validity — see slashEquivocators.)
	anchors := map[ports.NodeID]bool{idA.NodeID(): true, culprit.NodeID(): true, h2.NodeID(): true}
	g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("g")},
		BondRegs: []chain.BondReg{
			{Validator: pub(idA), Root: ports.HashBytes(pub(idA)), Size: bondSize},
			{Validator: pub(culprit), Root: ports.HashBytes(pub(culprit)), Size: bondSize},
			{Validator: pub(h2), Root: ports.HashBytes(pub(h2)), Size: bondSize},
		}}
	chain.Sign(g, idA.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, MatureValidators: 99}

	// A's WINNING chain: [g, W@1 (idA proposes; culprit attests → 2 of 3 anchors),
	// X@2 (idA proposes; culprit attests)]. Heavier (h=2). The culprit rides the
	// winning chain — and ALSO co-signs the losing fork below.
	W := chain.Block{Version: chain.BlockVersion, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("win")}}
	chain.Sign(&W, idA.Signer())
	W.Atts = []chain.Attestation{chain.Attest(&W, culprit.Signer())}
	X := chain.Block{Version: chain.BlockVersion, Height: 2, Prev: W.Hash(), Entries: []ports.Entry{mkEntry("win2")}}
	chain.Sign(&X, idA.Signer())
	X.Atts = []chain.Attestation{chain.Attest(&X, culprit.Signer())}

	// B's LOSING fork: [g, L@1 (h2 proposes; culprit attests → 2 of 3 anchors)].
	// Lighter (h=1), and it conflicts with W@1 — the culprit ATTESTED both W@1 and L@1
	// at height 1. idA (winner) and h2 (loser) are cleanly split; only the culprit
	// signs both.
	L := chain.Block{Version: chain.BlockVersion, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("lose")}}
	chain.Sign(&L, h2.Signer())
	L.Atts = []chain.Attestation{chain.Attest(&L, culprit.Signer())}

	mk := func(id *identity.Identity, blocks []chain.Block) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), bondSize)
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		if len(blocks) > 0 {
			if ok, err := ch.Reconcile(blocks); !ok {
				t.Fatalf("reconcile setup blocks: %v", err)
			}
		}
		return nd
	}
	a := mk(idA, []chain.Block{*g, W, X})
	b := mk(culprit, []chain.Block{*g, L})
	_ = b
	a.Bootstrap([]ports.NodeID{culprit.NodeID()}, func() {})
	b.Bootstrap([]ports.NodeID{idA.NodeID()}, func() {})
	sched.Run()

	if a.Chain().Len() != 3 {
		t.Fatalf("setup: A should hold the heavier 3-block chain, got len=%d", a.Chain().Len())
	}

	var slashed bool
	a.OnSlash(func(c ports.NodeID, _ uint64) {
		if c == culprit.NodeID() {
			slashed = true
		}
	})

	// A syncs B's LOSING fork.
	syncDone := false
	a.SyncChain([]ports.NodeID{culprit.NodeID()}, func(int, error) { syncDone = true })
	sched.Run()
	if !syncDone {
		t.Fatal("A's sync from B did not complete")
	}

	// The heavier chain is unchanged — the lighter fork was NOT adopted.
	if a.Chain().Len() != 3 {
		t.Fatalf("A must not adopt the lighter fork (len=%d, want 3)", a.Chain().Len())
	}
	// ...yet the equivocator that double-signed onto that losing fork IS slashed.
	if !slashed {
		t.Fatal("seam-7: a validator that double-signed onto a LOSING (never-adopted) fork must be slashed on detection, not left uncosted")
	}
}
