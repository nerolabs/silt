package node

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// The confirm-run 54003f7-91159 drain-pace repro (docs/thinking/2026-08-16-441-
// confirm-run-latch-late.md): the 3rd maturer's FIRST-TIME registration sat in
// the designees' queues for 22 minutes while lower-ID renewals banked every
// block — because the reg fold sorts `fresh` by validator ID and the ~2 MiB
// budget admits one plot-sized reg per block, ID order is a strict PRIORITY:
// the highest-ID first-timer loses to ANY lower-ID renewal, every block, for
// as long as renewal traffic flows. This is the exact starvation class the
// #441 certification closed for entries with FIFO (Addition 2: no fees ⇒ no
// priority order that can defer indefinitely) — still live on the reg side
// (#429 had named it: "ID-sorted packing makes order seed-luck").
//
// FAILING-FIRST: RED under today's ID-sorted fold — the low-ID renewal wins
// every block and the high-ID first-timer never banks within the budget.
// GREEN with FIFO-by-arrival (the first-timer, queued first, banks first).
func TestBondRegFIFONoIDSortStarvation(t *testing.T) {
	nodes, ids, net, g, _ := tier2AnchorNet(t, 4)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	proposer := nodes[0]
	// One reg folds per block (≥1 always folds) — the field's plot-sized-reg
	// budget in miniature (a ~1.5 MB proof fills the ~2 MiB cap).
	proposer.cfg.MaxBondRegBytesPerBlock = 1

	// Cast by ID order from a probe pool: the FIRST-TIMER must sort ABOVE the
	// RENEWER so today's ID-sorted fold always prefers the renewer.
	var firstTimer, renewer *identity.Identity
	idOfC := func(c *identity.Identity) []byte { id := c.NodeID(); return id[:] }
	for s := int64(9300); s < 9330; s++ {
		c := identity.FromSeed(s)
		if firstTimer == nil || bytes.Compare(idOfC(c), idOfC(firstTimer)) > 0 {
			firstTimer = c
		}
	}
	for s := int64(9300); s < 9330; s++ {
		c := identity.FromSeed(s)
		if c.NodeID() != firstTimer.NodeID() && (renewer == nil || bytes.Compare(idOfC(c), idOfC(renewer)) < 0) {
			renewer = c
		}
	}
	mkReg := func(id *identity.Identity, prev ports.Hash) chain.BondReg {
		sgn := id.Signer()
		pub := append([]byte(nil), sgn.Public().(ed25519.PublicKey)...)
		return chain.NewBondReg(sgn, ports.HashBytes(pub), 2<<20, []byte("stub"), prev, 0)
	}

	// The first-timer's reg arrives FIRST; the renewer's stream refills after
	// it every block (the field shape: a fresh renewal resubmission lands each
	// sweep while the first-timer waits).
	attesters := []ports.NodeID{all[1], all[2], all[3]}
	prev, h := proposer.chain.Head()
	proposer.queuePendingBondReg(mkReg(firstTimer, prev))
	const blocks = 4
	for i := 0; i < blocks; i++ {
		prev, h = proposer.chain.Head()
		proposer.queuePendingBondReg(mkReg(renewer, prev)) // the lower-ID renewal, freshly re-signed
		b := &chain.Block{Version: chain.BlockVersion, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry("ft-" + string(rune('a'+i)))}}
		var done bool
		var perr error
		proposer.proposeBlock(b, attesters, all, 0, func(err error) { done, perr = true, err })
		drainHeld(t, net, fifo)
		if !done || perr != nil {
			t.Fatalf("block %d: done=%v err=%v", i, done, perr)
		}
		if proposer.chain.BondedSize(firstTimer.NodeID()) > 0 {
			return // banked — FIFO holds, no starvation
		}
	}
	_ = g
	t.Fatalf("ID-SORT STARVATION: the high-ID FIRST-TIME reg never banked across %d blocks while the lower-ID renewal stream won every one-reg budget slot — the reg fold needs FIFO-by-arrival (the #441 Addition-2 rule, reg side)", blocks)
}
