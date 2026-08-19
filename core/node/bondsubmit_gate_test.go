package node

// Phase 1.2 — the MsgSubmitBondReg CPU gate (the #424 shape, one message kind
// over). The submit path had NO per-sender bound: every well-formed, self-signed
// reg forces up to one VerifySpaceTime (~ms of single-loop CPU, measured in
// core/bond/verifycost_bench_test.go), so one authenticated identity holding a
// pipe keeps the loop at a permanent duty cycle for free — and a third party
// replaying a captured VALID reg re-pays full verify per message (queue dedup
// sits after the verify). The gate: a per-sender window budget charged BEFORE
// decode (refusal = a map lookup, zero amplification), plus a sender-binding
// check (a submit is always the sender's OWN renewal — SubmitBondRenewal
// self-submits) refusing third-party relays before any crypto.
// Deliberation: docs/thinking/2026-08-19-bondreg-submit-cpu-gate.md

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// submitWorld builds an objective validator whose bond verifier COUNTS the
// expensive verifies — the quantity the gate exists to bound.
func submitWorld(t *testing.T) (*Node, *chain.Chain, *simclock.Scheduler, *int) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 5, simnet.DefaultConfig())
	id := identity.FromSeed(9401)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-submit-gate")}}
	chain.Sign(g, id.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, MatureValidators: 99}
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
	verifies := 0
	ch.SetBondVerifier(func([]byte, ports.Hash, int64, uint64, []byte) bool {
		verifies++
		return false // garbage floods never validate; the COUNT is the cost
	})
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	nd.EnableChain(ch, id.Signer())
	return nd, ch, sched, &verifies
}

// selfSignedReg builds a reg that clears decode + signature + size for the
// current head — exactly what a flooding attacker mints for free — so the next
// cost is the expensive verify the gate must bound.
func selfSignedReg(ch *chain.Chain, who *identity.Identity) chain.BondReg {
	head, _ := ch.Head()
	var root ports.Hash
	nid := who.NodeID()
	copy(root[:], nid[:16])
	return chain.NewBondReg(who.Signer(), root, 1<<20, []byte("answer"), head, 1)
}

// TestBondSubmitFloodBoundedPerSender: N submits from ONE sender inside one
// window must reach the expensive verify at most bondSubmitBurst times; the
// budget refills in the next window; and a DIFFERENT sender proceeds while the
// flooder is capped. RED pre-gate (all N reach the verify).
func TestBondSubmitFloodBoundedPerSender(t *testing.T) {
	nd, ch, sched, verifies := submitWorld(t)

	flooder := identity.FromSeed(9402)
	reg := selfSignedReg(ch, flooder)
	raw := bondRegEncode(reg)
	const flood = 100
	for i := 0; i < flood; i++ {
		nd.handleChain(flooder.NodeID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: raw})
	}
	if *verifies > bondSubmitBurst {
		t.Fatalf("one sender forced %d expensive verifies in one window (budget %d) — the submit flood is unbounded", *verifies, bondSubmitBurst)
	}

	// A second sender is NOT starved by the flooder's spent budget.
	before := *verifies
	other := identity.FromSeed(9403)
	nd.handleChain(other.NodeID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: bondRegEncode(selfSignedReg(ch, other))})
	if *verifies != before+1 {
		t.Fatalf("an honest second sender was starved: verifies %d → %d (want +1)", before, *verifies)
	}

	// The window turns over → the budget refills (honest resubmit-next-sweep heals).
	window := nd.cfg.ChainSyncInterval
	sched.AfterFunc(window+1, func() {})
	sched.Run()
	before = *verifies
	nd.handleChain(flooder.NodeID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: raw})
	if *verifies != before+1 {
		t.Fatalf("the flooder's budget did not refill after the window: verifies %d → %d (want +1)", before, *verifies)
	}
}

// TestBondSubmitRefusesThirdPartyRelay: a VALID reg signed by one identity but
// delivered by a different transport identity is refused before any crypto —
// the replay hole (re-verifying a captured 1.5 MB reg per message) closed.
// RED pre-gate (the relayed reg reaches the verify).
func TestBondSubmitRefusesThirdPartyRelay(t *testing.T) {
	nd, ch, _, verifies := submitWorld(t)

	victim := identity.FromSeed(9404) // the reg's real owner
	relay := identity.FromSeed(9405)  // the replaying third party
	raw := bondRegEncode(selfSignedReg(ch, victim))
	nd.handleChain(relay.NodeID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: raw})
	if *verifies != 0 {
		t.Fatalf("a third-party relayed reg reached the expensive verify (%d) — replay is not refused", *verifies)
	}
	if len(nd.pendingBondRegs) != 0 {
		t.Fatalf("a third-party relayed reg was queued")
	}

	// The owner submitting the SAME reg itself passes the binding and verifies.
	nd.handleChain(victim.NodeID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: raw})
	if *verifies != 1 {
		t.Fatalf("the owner's own submit did not reach validation (verifies=%d)", *verifies)
	}
}
