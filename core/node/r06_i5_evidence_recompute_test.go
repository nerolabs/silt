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

// TestForgedPeerForkDoesNotQueueAPendingSlash is T-5 (I5-cross-height-pruned-slash-forgery-
// FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md §10, reproducing §8.1). §8.1's finding
// was ANALYTIC, not reproduced, at certification time: the attack needs NO Byzantine
// proposer. A Byzantine PEER — no proposer role, no quorum, no key material of its own —
// serves a "fork" whose block at OUR OWN honest height H is a forgery: Pruned set to the
// hash of some OTHER honest block the culprit genuinely signed at a DIFFERENT height H',
// carrying that block's real attestation. slashEquivocators runs on DETECTION
// (core/node/chainrole.go:1252-1288), BEFORE and INDEPENDENT of adoption, so the honest node
// convicts its own honest validator purely from a peer-served forgery it never adopts, and
// queues the forged proof for on-chain inclusion (chainrole.go:1273-1282,
// n.pendingSlashes).
//
// G-5 (cert §11): this test FIRST reproduces §8.1 by construction — the fixture below is
// exactly the shape described there — then gates the post-fix property. RED today: the
// forgery IS caught (slashedLocal set, a proof queued in pendingSlashes) exactly as §8.1
// predicts.
func TestForgedPeerForkDoesNotQueueAPendingSlash(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ledger := credit.New(50_000, 0)

	ndi := identity.FromSeed(1)
	nd := New(ndi.NodeID(), DefaultConfig(), sched, net.Endpoint(ndi.NodeID()), memstore.New())
	nd.SetLedger(ledger)

	prop := identity.FromSeed(2).Signer()
	culprit := identity.FromSeed(3) // honest, active validator — the forgery's victim
	culpritSigner := culprit.Signer()
	culpritPub := []byte(culpritSigner.Public().(ed25519.PublicKey))
	culpritID := culprit.NodeID()

	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	chain.Sign(g, prop)

	ch := chain.New(chain.DefaultConfig(), func(id ports.NodeID) int64 { return ledger.Reputation(id) })
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	nd.EnableChain(ch, ndi.Signer())

	// Our own HONEST height-1 block, genuinely attested by the culprit.
	ownA := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("A")}}
	chain.Sign(ownA, prop)
	ownA.Atts = append(ownA.Atts, chain.Attest(ownA, culpritSigner))

	// A SEPARATE honest block at a DIFFERENT height, also genuinely signed by the culprit —
	// the ordinary "sequential proposing" every active validator does, never equivocation.
	realOther := &chain.Block{Version: 1, Height: 2, Prev: ownA.Hash(), Entries: []ports.Entry{mkEntry("other")}}
	realOtherAtt := chain.Attest(realOther, culpritSigner)
	realOther.Atts = append(realOther.Atts, realOtherAtt)

	if chain.VerifyEquivocation(&chain.Equivocation{Culprit: culpritPub, A: *ownA, B: *realOther}) {
		t.Fatal("precondition broken: two honest, different-height blocks must not verify as equivocation")
	}

	// FORGE: a Byzantine PEER's fork whose block at OUR height 1 carries no key material of
	// its own — just realOther's genuine hash-and-signature, relabeled via Pruned.
	forged := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Pruned: realOther.Hash(),
		Atts: []chain.Attestation{realOtherAtt}}

	// §8.1 REPRODUCTION: exactly what an honest node runs on every sync sweep, BEFORE and
	// independent of adoption (chainrole.go SyncChain -> slashEquivocators).
	nd.slashEquivocators([]chain.Block{*g, *ownA}, []chain.Block{*g, *forged})

	// --- Post-fix gate (RED today; see the reproduction note above) ---
	if len(nd.pendingSlashes) != 0 {
		t.Fatalf("T-5 RED (expected): a peer-served forged fork queued %d pending on-chain "+
			"slash(es) against an honest validator — a Byzantine PEER with no proposer role and "+
			"no key material weaponised an honest validator's own real signatures (cert §8.1)",
			len(nd.pendingSlashes))
	}
	if nd.slashedLocal[culpritID] {
		t.Fatal("T-5: the honest validator was locally slashed by a proof-free peer-served forgery")
	}
}
