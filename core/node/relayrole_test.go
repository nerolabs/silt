package node

// PoD relay lane — the relay-accept role and its two M0 guards
// (docs/design/pod.md §7.3, certified 2026-08-30). A relay opts into accepting
// sender-funded PayWord chains behind a flag (mirror of --accept-delivery-
// receipts / EnableDemandBank). The two guards are bright-line, non-negotiable
// M0 access-privacy constraints (immutable Don't-#3): the relay must reject any
// chain funded by a durable-account credit, and any session that reuses an
// ephemeral identity or a chain across sessions.

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

func relayTestID(b byte) ports.NodeID { return ports.HashBytes([]byte{b}) }

// newRelayTestNode builds a node with a sim transport so New does not panic on a
// nil transport. The relay guards are pure per-node state logic; the transport
// is only there to satisfy the constructor.
func newRelayTestNode(idSeed int64) *Node {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ident := identity.FromSeed(idSeed)
	return New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
}

// TestRelayAcceptOffByDefault: a node does not accept PayWord chains until the
// gate is enabled — the mirror of the delivery-receipt gate being off by
// default. OpenRelaySession returns an error while the gate is off.
func TestRelayAcceptOffByDefault(t *testing.T) {
	n := newRelayTestNode(1)
	c, _ := relaypay.BuildChain([]byte("a-fresh-random-tip-for-the-session"), 8)
	eph := relayTestID(9)
	_, err := n.OpenRelaySession(eph, c.Root(), 8, FundingEphemeralBlind)
	if err == nil {
		t.Fatalf("OpenRelaySession succeeded with the relay-accept gate OFF — it must be gated")
	}
}

// TestRelayGuardI_DurableFundingRejected is M0 guard (i), failing-first: a chain
// root funded by a DURABLE-account credit (not the ephemeral blind path) must be
// REJECTED. Binding a chain to a durable identity turns the relay into a party
// that can tie the fetcher's durable identity to what it fetched — an M0
// access-privacy violation, not permitted at any performance price.
func TestRelayGuardI_DurableFundingRejected(t *testing.T) {
	n := newRelayTestNode(1)
	n.EnableRelayAccept()
	c, _ := relaypay.BuildChain([]byte("a-fresh-random-tip-for-the-session"), 8)
	eph := relayTestID(9)

	// A durable-funded chain MUST be rejected.
	if _, err := n.OpenRelaySession(eph, c.Root(), 8, FundingDurableAccount); err == nil {
		t.Fatalf("guard (i) missing: a chain funded by a DURABLE-account credit was accepted — M0 Don't-#3 violation")
	}
	// The ephemeral-blind path is the ONLY accepted funding source.
	if _, err := n.OpenRelaySession(eph, c.Root(), 8, FundingEphemeralBlind); err != nil {
		t.Fatalf("the ephemeral-blind funding path must be accepted: %v", err)
	}
}

// TestRelayGuardII_EphemeralReuseRejected is M0 guard (ii), failing-first: a
// session-open reusing an ephemeral identity OR a chain across sessions must be
// REJECTED. Reuse upgrades the relay from a per-session observer to a
// longitudinal one — a real Don't-#3 regression.
func TestRelayGuardII_EphemeralReuseRejected(t *testing.T) {
	n := newRelayTestNode(1)
	n.EnableRelayAccept()

	eph := relayTestID(9)
	c1, _ := relaypay.BuildChain([]byte("session-one-fresh-random-tip-value"), 8)
	c2, _ := relaypay.BuildChain([]byte("session-two-fresh-random-tip-value"), 8)

	if _, err := n.OpenRelaySession(eph, c1.Root(), 8, FundingEphemeralBlind); err != nil {
		t.Fatalf("first session for a fresh ephemeral identity must open: %v", err)
	}
	// Reusing the SAME ephemeral identity for a second session must be rejected.
	if _, err := n.OpenRelaySession(eph, c2.Root(), 8, FundingEphemeralBlind); err == nil {
		t.Fatalf("guard (ii) missing: a REUSED ephemeral identity opened a second session — longitudinal-linkage regression")
	}

	// And reusing the SAME chain root under a fresh ephemeral identity must also
	// be rejected — a chain must not span sessions.
	fresh := relayTestID(10)
	if _, err := n.OpenRelaySession(fresh, c1.Root(), 8, FundingEphemeralBlind); err == nil {
		t.Fatalf("guard (ii) missing: a REUSED chain root opened a second session — a chain must not span sessions")
	}
}

// TestRelaySessionAdvancesAndSettles: the happy path. A session opened under a
// fresh ephemeral identity + fresh chain accepts preimages in order and the
// verifier tracks the settled count.
func TestRelaySessionAdvancesAndSettles(t *testing.T) {
	n := newRelayTestNode(1)
	n.EnableRelayAccept()

	const S = 8
	c, _ := relaypay.BuildChain([]byte("the-happy-path-session-random-tip!"), S)
	eph := relayTestID(9)
	sess, err := n.OpenRelaySession(eph, c.Root(), S, FundingEphemeralBlind)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for k := 1; k <= S; k++ {
		if err := sess.Pay(c.Preimage(k)); err != nil {
			t.Fatalf("increment %d: %v", k, err)
		}
	}
	if sess.Count() != S {
		t.Fatalf("settled count %d, want %d", sess.Count(), S)
	}
}
