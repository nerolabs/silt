package node

// The relay interim gate, re-specified as
// UNANCHORED ABLATION GUARDS — a recorded goalpost move, not a silent one. The
// interim pinned "an unanchored session settles for 0"; under an unanchored
// session is never ADMITTED: "v1 open at a v2 relay: len(Anchors) == 0 ⇒
// errRelayNoAnchor", so the property moves one step earlier: the wire refusal
// names the missing anchor, no session exists to settle, no settlement line is
// emitted, and the relay's ledger does not move. The shipped-grant topology
// (credit.New(50_000, 500_000), the shape) is kept for the same reason the
// gives.
//
// RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION- step 1 (the interim);
// (the refusal).

import (
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

// relayPairShippedGrant wires a fetcher and a relay node exactly like
// relayPairForTest (relaytransport_test.go), except the ledger carries the
// SHIPPED grant (500,000, daemon.go) instead of 0. The fetcher's fresh ephemeral
// is a total stranger to the relay's ledger — the topology.
func relayPairShippedGrant(t *testing.T, relayLog ports.Logger) (fetcher, relay *Node, ledger *credit.Ledger, sched *simclock.Scheduler) {
	t.Helper()
	sched = simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())

	fID := identity.FromSeed(7101) // the fetcher's FRESH EPHEMERAL identity
	rID := identity.FromSeed(7102) // the relay operator

	fetcher = New(fID.NodeID(), DefaultConfig(), sched, net.Endpoint(fID.NodeID()), memstore.New())
	relay = New(rID.NodeID(), DefaultConfig(), sched, net.Endpoint(rID.NodeID()), memstore.New())
	if relayLog != nil {
		relay.SetLogger(relayLog)
	}

	ledger = credit.New(50_000, 500_000) // the shipped grant, deliberately — the rule
	ledger.Register(rID.NodeID())
	relay.SetLedger(ledger)
	fetcher.SetSigner(fID.Signer())
	commitSelfDemandKey(t, relay, rID, cachedRSAKey(t, 1))
	relay.EnableRelayAccept()

	fetcher.Bootstrap([]ports.NodeID{rID.NodeID()}, func() {})
	relay.Bootstrap([]ports.NodeID{fID.NodeID()}, func() {})
	sched.Run()
	return fetcher, relay, ledger, sched
}

// TestSettleRelaySessionPaysZeroUntilAnchor is the gate re-specified for: an
// UNANCHORED open over the wire (the v1 payload shape) is refused with the named
// reason, no session exists to settle, and the relay's ledger — every account on
// it — is exactly as it was. Ablation: admit an open with no anchors → the open
// succeeds and this reddens.
func TestSettleRelaySessionPaysZeroUntilAnchor(t *testing.T) {
	const S = 6
	fetcher, relay, ledger, sched := relayPairShippedGrant(t, nil)

	tip := []byte("g-ri-1b-fresh-random-tip-32-bytes")[:32]
	chain, err := relaypay.BuildChain(tip, S)
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	relayID := relay.id
	relayBalBefore := ledger.Balance(relayID)
	accountsBefore := len(ledger.Balances())

	var handle uint64
	var openErr error
	got := false
	fetcher.OpenRelaySessionRemote(relayID, chain.Root(), S, FundingEphemeralBlind, nil, func(h uint64, e error) {
		handle, openErr, got = h, e, true
	})
	sched.Run()
	if !got {
		t.Fatal("open over wire: no reply")
	}
	if openErr == nil || handle != 0 {
		t.Fatalf("an UNANCHORED open was admitted over the wire (handle=%d) —  admits nothing without a spent anchor", handle)
	}
	if !strings.Contains(openErr.Error(), errRelayNoAnchor.Error()) {
		t.Fatalf("unanchored open refused for %q, want the named reason %q", openErr, errRelayNoAnchor)
	}
	if got := relay.relayLiveSessionCountForTest(); got != 0 {
		t.Fatalf("%d live sessions after a refused unanchored open, want 0", got)
	}
	if paid := relay.SettleRelaySession(handle); paid != 0 {
		t.Fatalf("settling the refused handle paid %d, want 0", paid)
	}
	if got := ledger.Balance(relayID); got != relayBalBefore {
		t.Fatalf("relay balance moved %d -> %d on a refused unanchored open", relayBalBefore, got)
	}
	if got := len(ledger.Balances()); got != accountsBefore {
		t.Fatalf("the account set changed %d -> %d — the fetcher's ephemeral was registered by a refused open", accountsBefore, got)
	}
}

// TestSettleRelaySessionLogCarriesNoAnchorReason is the gate re-specified for:
// an unanchored session never settles, so NO "relay session settled" line is
// emitted for it; the anchor's absence is named in the wire refusal instead (the
// S5 `no-anchor` registry entry is retired for `anchored`, cmd/silt/observable_contract.go).
func TestSettleRelaySessionLogCarriesNoAnchorReason(t *testing.T) {
	const S = 4
	log := &capturingLogger{}
	fetcher, relay, _, sched := relayPairShippedGrant(t, log)

	tip := []byte("g-ri-2-fresh-random-tip-32-bytes!")[:32]
	chain, err := relaypay.BuildChain(tip, S)
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}

	var handle uint64
	var openErr error
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), S, FundingEphemeralBlind, nil, func(h uint64, e error) { handle, openErr = h, e })
	sched.Run()
	if openErr == nil || !strings.Contains(openErr.Error(), "no prepayment anchor") {
		t.Fatalf("unanchored open: err=%v, want a refusal naming the missing anchor", openErr)
	}
	relay.SettleRelaySession(handle)

	for _, rec := range log.records {
		if rec.event == "relay session settled" {
			t.Fatalf("a 'relay session settled' line was emitted for an unanchored open (%+v) — nothing was admitted, so nothing may settle", rec.kv)
		}
	}
}
