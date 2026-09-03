package node

// R0.7 relay interim — RED-first gates G-RI-1b and G-RI-2 (Tester, 2026-09-03).
//
// Binding spec: RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md
// §9 step 1; docs/thinking/2026-09-03-r0.7-relay-interim-design.md §2 step 1
// ("Named reason on the log line (`relay session settled` gains
// `reason=no-anchor`), pinned as an S5 entry in the registry").
//
// G-RI-1b drives the SAME RT-RELAY-1 mint shape as core/credit's G-RI-1
// (TestRelayRedeemPaysZeroUntilAnchor) but through the live node path
// (SettleRelaySession, relaytransport.go:96), against the SHIPPED grant
// (credit.New(50_000, 500_000), cmd/silt/daemon.go:622) — deliberately not
// the grant=0 the package's existing relayPairForTest fixture uses, for the
// same reason the certification's T-1 gives: grant=0 would pass this gate for
// the wrong reason once R2.12 lands (a zero-balance phantom debited to
// negative looks similar to "paid 0" even without a real fix). A fresh
// fixture (relayPairShippedGrant) is used instead of editing the existing
// relayPairForTest so no other test's ledger topology changes underneath it.
//
// G-RI-2 pins the S5 observable-log-contract addition: the settlement log
// line must carry a named `reason` field with value `no-anchor`. Reused
// fixture: capturingLogger (relaytransport_test.go:37), already scoped to
// exactly this settlement line and already wired into relayPairForTest's
// optional relayLog param. (core/node/repair_sweep_duration_501_test.go:33's
// captureLog helper was considered — it captures the same shape (event + kv +
// timestamp) but is built for the #501 phase-narration rig and pulls in
// erasure/manifest/pipeline/registry dependencies unrelated to this gate;
// capturingLogger is the closer, already-relay-scoped fixture, so it is
// reused rather than duplicated.)

import (
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
// relayPairForTest (relaytransport_test.go:55), except the ledger carries the
// SHIPPED grant (500,000, daemon.go:622) instead of 0, and the fetcher's
// fresh ephemeral is left COMPLETELY UNTOUCHED on the relay's ledger — no
// RecordServe pre-funding call. That is the real RT-RELAY-1 topology: in
// production the fetcher's actual payment landed on a demand-token issuer's
// ledger, never the relay's, so the relay's ledger has never seen this
// identity before settlement.
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

	ledger = credit.New(50_000, 500_000) // the shipped grant, deliberately — cert T-1
	// Deliberately NO ledger.RecordServe(...) pre-funding call: the fetcher's
	// fresh ephemeral must be a total stranger to this ledger, matching the
	// red-team's reproduced RT-RELAY-1 shape.
	relay.SetLedger(ledger)
	relay.EnableRelayAccept()

	fetcher.Bootstrap([]ports.NodeID{rID.NodeID()}, func() {})
	relay.Bootstrap([]ports.NodeID{fID.NodeID()}, func() {})
	sched.Run()
	return fetcher, relay, ledger, sched
}

// TestSettleRelaySessionPaysZeroUntilAnchor is G-RI-1b: a full paid session,
// driven over the wire exactly like TestRelayFullSessionConservedSettlement,
// settles through SettleRelaySession for 0, and the relay's ledger balance is
// unchanged. TODAY (main): the session settles for S x RelayIncrementCredit
// and the relay's balance rises by exactly that — RED.
func TestSettleRelaySessionPaysZeroUntilAnchor(t *testing.T) {
	const S = 6
	fetcher, relay, ledger, sched := relayPairShippedGrant(t, nil)

	tip := []byte("g-ri-1b-fresh-random-tip-32-bytes")[:32]
	chain, err := relaypay.BuildChain(tip, S)
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}

	relayID := relay.id
	fetcherID := fetcher.id
	relayBalBefore := ledger.Balance(relayID)

	var handle uint64
	var openErr error
	got := false
	fetcher.OpenRelaySessionRemote(relayID, chain.Root(), S, FundingEphemeralBlind, func(h uint64, e error) {
		handle, openErr, got = h, e, true
	})
	sched.Run()
	if !got || openErr != nil {
		t.Fatalf("open over wire: got=%v err=%v", got, openErr)
	}
	if handle == 0 {
		t.Fatalf("relay returned a zero session handle")
	}

	for k := 1; k <= S; k++ {
		var authorized int
		var payErr error
		done := false
		fetcher.SubmitRelayPay(relayID, handle, chain.Preimage(k), k, func(a int, e error) {
			authorized, payErr, done = a, e, true
		})
		sched.Run()
		if !done || payErr != nil {
			t.Fatalf("pay increment %d over wire: done=%v err=%v", k, done, payErr)
		}
		if authorized != k {
			t.Fatalf("after paying increment %d the relay authorized %d, want %d", k, authorized, k)
		}
	}

	paid := relay.SettleRelaySession(handle)
	if paid != 0 {
		t.Fatalf("settlement paid %d for an unanchored session (S=%d paid increments), want 0 — the RT-RELAY-1 mint on the live SettleRelaySession path", paid, S)
	}
	if got := ledger.Balance(relayID); got != relayBalBefore {
		t.Fatalf("relay balance moved %d -> %d on an unanchored (pay-0) settlement", relayBalBefore, got)
	}
	// The fetcher's fresh ephemeral must still read as the untouched shipped
	// grant: if it were debited by even 1 credit, this would no longer equal
	// the grant (the ledger auto-registers on first Balance() touch too, so
	// "untouched" here means "reads exactly the grant", not "reads 0").
	const grant = 500_000
	if got := ledger.Balance(fetcherID); got != grant {
		t.Fatalf("fetcher balance reads %d, want the untouched grant %d — RedeemRelayCredit debited a phantom on an unanchored settlement", got, grant)
	}
}

// TestSettleRelaySessionLogCarriesNoAnchorReason is G-RI-2 (S5): the
// settlement log line names WHY it paid 0. Once the interim ships, "relay
// session settled" must carry a `reason` key with value `no-anchor`. TODAY
// (main): the log line carries only "increments" and "credit" — no `reason`
// key at all — RED.
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
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), S, FundingEphemeralBlind, func(h uint64, _ error) { handle = h })
	sched.Run()
	for k := 1; k <= S; k++ {
		fetcher.SubmitRelayPay(relay.id, handle, chain.Preimage(k), k, func(int, error) {})
		sched.Run()
	}
	relay.SettleRelaySession(handle)

	var rec *logRecord
	for i := range log.records {
		if log.records[i].event == "relay session settled" {
			rec = &log.records[i]
			break
		}
	}
	if rec == nil {
		t.Fatalf("no 'relay session settled' log record captured")
	}

	reasonFound := false
	for i := 0; i+1 < len(rec.kv); i += 2 {
		key, _ := rec.kv[i].(string)
		if key == "reason" {
			reasonFound = true
			if val, _ := rec.kv[i+1].(string); val != "no-anchor" {
				t.Fatalf("settlement log 'reason' = %q, want %q", rec.kv[i+1], "no-anchor")
			}
		}
	}
	if !reasonFound {
		t.Fatalf("settlement log record %+v carries no 'reason' key — the S5 no-anchor disclosure (design doc §2 step 1) is missing", rec.kv)
	}
}
