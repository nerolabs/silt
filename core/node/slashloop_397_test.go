package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// #397 Q4(i): detection of the SAME equivocation on every reconcile sweep must
// apply the local ledger slash (and fire the callback / warn log) exactly ONCE
// per culprit — not once per sweep. On the wire (run b88245d-3496) the missing
// latch re-slashed and re-logged both culprits every ~2s indefinitely.
func TestSlashEquivocatorsIsIdempotentAcrossSweeps397(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ledger := credit.New(50_000, 0)

	ndi := identity.FromSeed(1)
	nd := New(ndi.NodeID(), DefaultConfig(), sched, net.Endpoint(ndi.NodeID()), memstore.New())
	nd.SetLedger(ledger)

	prop := identity.FromSeed(2).Signer()
	v := identity.FromSeed(3)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	chain.Sign(g, prop)
	fork := func(name string) *chain.Block {
		b := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry(name)}}
		chain.Sign(b, prop)
		b.Atts = append(b.Atts, chain.Attest(b, v.Signer()))
		return b
	}
	a, b := fork("A"), fork("B")

	slashes := map[ports.NodeID]int{}
	nd.OnSlash(func(culprit ports.NodeID, _ uint64) { slashes[culprit]++ })

	// The same live fork is re-observed on three consecutive reconcile sweeps —
	// exactly what a wedged chain does until it heals.
	nd.slashEquivocators([]chain.Block{*g, *a}, []chain.Block{*g, *b})
	nd.slashEquivocators([]chain.Block{*g, *a}, []chain.Block{*g, *b})
	nd.slashEquivocators([]chain.Block{*g, *a}, []chain.Block{*g, *b})

	if got := slashes[v.NodeID()]; got != 1 {
		t.Fatalf("culprit slashed %d times across 3 sweeps of the same fork — the local slash must be idempotent-once (#397 Q4-i)", got)
	}
}

// #397 Q4(ii): a pending on-chain slash record that does NOT land in a
// committed block (the proposal failed to gather quorum) must stay queued and
// be retried in the next proposal — not silently dropped after one attempt.
// (`still` was built but never appended to, so `n.pendingSlashes = still`
// zeroed the queue; masked in the field only by Q4(i)'s re-detection.)
func TestPendingSlashRequeuedUntilCommitted397(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ledger := credit.New(50_000, 0)
	repFn := func(n ports.NodeID) int64 { return ledger.Reputation(n) }

	ti := identity.FromSeed(1)
	tid := ti.NodeID()
	tn := New(tid, DefaultConfig(), sched, net.Endpoint(tid), memstore.New())
	tn.SetLedger(ledger)

	ch := chain.New(chain.DefaultConfig(), repFn)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	prop := identity.FromSeed(2)
	chain.Sign(g, prop.Signer())
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	tn.EnableChain(ch, ti.Signer())
	ledger.RecordBondChallenge(tid, ports.HashBytes([]byte{1}), 64<<20, true, 1)

	// A self-verifying equivocation proof by a third validator.
	v := identity.FromSeed(3)
	fork := func(name string) *chain.Block {
		b := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry(name)}}
		chain.Sign(b, prop.Signer())
		b.Atts = append(b.Atts, chain.Attest(b, v.Signer()))
		return b
	}
	evs := chain.FindEquivocations([]chain.Block{*g, *fork("A")}, []chain.Block{*g, *fork("B")})
	if len(evs) == 0 {
		t.Fatal("setup: expected an equivocation proof")
	}
	tn.pendingSlashes = []chain.Equivocation{evs[0]}

	// Propose to an attester that never replies: the gather fails (no quorum),
	// the block does not commit, and the slash record must remain queued.
	deadID := identity.FromSeed(9).NodeID()
	_ = net.Endpoint(deadID)
	blk := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("carrier")}}
	var perr error
	tn.proposeBlock(blk, []ports.NodeID{deadID}, nil, 1, func(err error) { perr = err })
	sched.Run()

	if perr == nil {
		t.Fatal("setup: the proposal should have failed (no quorum from a dead attester)")
	}
	if len(tn.pendingSlashes) != 1 {
		t.Fatalf("pending on-chain slash record dropped after one failed proposal (len=%d) — it must requeue until a commit confirms it (#397 Q4-ii)", len(tn.pendingSlashes))
	}
}
