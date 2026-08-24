package node

// #183 red-team F-1 (the rate-gate half) — the MsgSubmitEntry CPU gate, the
// same #424/allowBondSubmit shape one message kind over. The entry-submit path
// had NO per-sender bound: under -require-tokens, ValidateEntry runs an RSA
// verify per token signature, so one authenticated peer floods entry submits
// and rides per-message crypto onto the single consensus loop. The gate: a
// per-sender window budget charged BEFORE decode+validate (a refusal is a map
// lookup, zero amplification), mirroring the bond-reg submit gate.

import (
	"fmt"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// entryWorld builds a validator whose chain accepts well-formed token-less
// entries (tokenQuorum 0), so a submit that clears the gate reaches the mempool
// — the count of QUEUED entries is the observable the gate must bound. (The
// gate is regime-independent; in -require-tokens mode each queued submit would
// additionally have paid the RSA verify the gate exists to bound.)
func entryWorld(t *testing.T) (*Node, *simclock.Scheduler) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 5, simnet.DefaultConfig())
	id := identity.FromSeed(9501)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-entry-gate")}}
	chain.Sign(g, id.Signer())
	ch := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1000 })
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	nd.EnableChain(ch, id.Signer())
	return nd, sched
}

// TestEntrySubmitFloodBoundedPerSender: N distinct valid entry submits from ONE
// sender inside one window must reach the mempool at most entrySubmitBurst
// times; the budget refills next window; and a DIFFERENT sender proceeds while
// the flooder is capped. RED pre-gate (all N reach validate+queue).
func TestEntrySubmitFloodBoundedPerSender(t *testing.T) {
	nd, sched := entryWorld(t)

	flooder := identity.FromSeed(9502)
	const flood = 100
	for i := 0; i < flood; i++ {
		e := mkEntry(fmt.Sprintf("flood-%d", i)) // distinct roots (dedup would otherwise mask it)
		nd.handleChain(flooder.NodeID(), ports.Message{Kind: ports.MsgSubmitEntry, Data: entryEncode(e)})
	}
	if got := len(nd.pendingEntries); got > entrySubmitBurst {
		t.Fatalf("one sender queued %d entries in one window (budget %d) — the submit flood is unbounded", got, entrySubmitBurst)
	}

	// A second sender is NOT starved by the flooder's spent budget.
	before := len(nd.pendingEntries)
	other := identity.FromSeed(9503)
	nd.handleChain(other.NodeID(), ports.Message{Kind: ports.MsgSubmitEntry, Data: entryEncode(mkEntry("other-entry"))})
	if len(nd.pendingEntries) != before+1 {
		t.Fatalf("an honest second sender was starved: queued %d → %d (want +1)", before, len(nd.pendingEntries))
	}

	// The window turns over → the budget refills (client resubmit heals).
	window := nd.cfg.ChainSyncInterval
	sched.AfterFunc(window+1, func() {})
	sched.Run()
	before = len(nd.pendingEntries)
	nd.handleChain(flooder.NodeID(), ports.Message{Kind: ports.MsgSubmitEntry, Data: entryEncode(mkEntry("flood-post-window"))})
	if len(nd.pendingEntries) != before+1 {
		t.Fatalf("the flooder's budget did not refill after the window: queued %d → %d (want +1)", before, len(nd.pendingEntries))
	}
}
