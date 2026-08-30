package node

// PoD §7.3 transport Batch 2 — the wire protocol + settle-at-close (steps 3+4),
// failing-first. These tests drive the LIVE path: a fetcher node opens a paid
// session on a relay node over the in-memory transport (MsgRelayOpen), pays
// increments (MsgRelayPay), and the relay settles at close (RedeemRelayCredit).
//
// The four properties pinned here:
//
//  3. FULL SESSION + CONSERVED SETTLEMENT: open → pay N → settle redeems exactly
//     N × increment (≤ budget) via the verifier's monotonic count; conservation
//     holds (the fetcher's balance falls by exactly what the relay's rises).
//
//  4. M0 ON THE LIVE PATH: the settlement log line carries NO durable or
//     cross-session-stable field (design §6 residual). Asserted against a
//     capturing logger.
//
//  5. LIVE-PATH GUARD REUSE: the Batch-1 M0 guards and the #644 S-clamp FIRE when
//     driven through MsgRelayOpen — the wire path does not bypass them.

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

// capturingLogger records every log record so a test can audit the fields the
// settlement line emits (M0 residual, design §6).
type capturingLogger struct {
	records []logRecord
}

type logRecord struct {
	event string
	kv    []any
}

func (c *capturingLogger) Enabled(ports.LogLevel) bool { return true }

func (c *capturingLogger) Log(_ ports.LogLevel, event string, kv ...any) {
	c.records = append(c.records, logRecord{event: event, kv: append([]any(nil), kv...)})
}

// relayPairForTest wires a fetcher node and a relay node over one sim transport,
// with the relay accepting PayWord chains and holding a ledger for settlement. It
// funds the fetcher's paid-in blind credit so settlement has a source to draw from.
func relayPairForTest(t *testing.T, relayLog ports.Logger, fetcherBudget int64) (fetcher, relay *Node, ledger *credit.Ledger, sched *simclock.Scheduler) {
	t.Helper()
	sched = simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())

	fID := identity.FromSeed(7001) // the fetcher's FRESH EPHEMERAL identity
	rID := identity.FromSeed(7002) // the relay operator

	fetcher = New(fID.NodeID(), DefaultConfig(), sched, net.Endpoint(fID.NodeID()), memstore.New())
	relay = New(rID.NodeID(), DefaultConfig(), sched, net.Endpoint(rID.NodeID()), memstore.New())
	if relayLog != nil {
		relay.SetLogger(relayLog)
	}

	ledger = credit.New(50_000, 0)
	// Seed the fetcher's paid-in blind credit (RecordServe credits the "server"
	// arg — here the fetcher — 1 credit/byte, standing in for the withdrawal it
	// already paid for).
	ledger.RecordServe(fID.NodeID(), rID.NodeID(), ports.ChunkID{}, fetcherBudget)
	relay.SetLedger(ledger)
	relay.EnableRelayAccept()

	// Route the fetcher to the relay so `request` can reach it.
	fetcher.Bootstrap([]ports.NodeID{rID.NodeID()}, func() {})
	relay.Bootstrap([]ports.NodeID{fID.NodeID()}, func() {})
	sched.Run()
	return fetcher, relay, ledger, sched
}

// TestRelayFullSessionConservedSettlement is failing-first test (3): a full live
// session over the wire settles for exactly count × increment, conserved.
func TestRelayFullSessionConservedSettlement(t *testing.T) {
	const S = 6
	fetcher, relay, ledger, sched := relayPairForTest(t, nil, 1_000)

	tip := []byte("full-session-fresh-random-tip-32b")[:32]
	chain, _ := relaypay.BuildChain(tip, S)

	relayID := relay.id
	fetcherID := fetcher.id
	relayBalBefore := ledger.Balance(relayID)
	fetcherBalBefore := ledger.Balance(fetcherID)

	// Open the session over the wire.
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

	// Pay all S increments over the wire, one MsgRelayPay each.
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

	// Settle at close: redeem the highest held preimage ONCE.
	paid := relay.SettleRelaySession(handle)
	want := int64(S) * relaypay.RelayIncrementCredit
	if paid != want {
		t.Fatalf("settlement paid %d, want count × increment = %d", paid, want)
	}
	// Conservation: relay up by exactly `paid`, fetcher down by exactly `paid`.
	if got := ledger.Balance(relayID) - relayBalBefore; got != paid {
		t.Fatalf("relay balance rose by %d, want the settled %d", got, paid)
	}
	if got := fetcherBalBefore - ledger.Balance(fetcherID); got != paid {
		t.Fatalf("fetcher balance fell by %d, want the settled %d", got, paid)
	}
	// A second settle is a no-op (single settlement at close — the session is gone).
	if again := relay.SettleRelaySession(handle); again != 0 {
		t.Fatalf("a second settle of the same handle paid %d, want 0 — single-settlement-at-close is violated", again)
	}
}

// TestRelaySettlementLogCarriesNoDurableField is failing-first test (4), the M0
// residual (design §6): the settlement log line must carry NO durable or
// cross-session-stable field. It asserts the "relay session settled" record's keys
// and values contain neither the fetcher's ephemeral NodeID, the relay's NodeID,
// nor the chain root — only per-session, non-correlatable values (the increment
// count and the paid credit).
func TestRelaySettlementLogCarriesNoDurableField(t *testing.T) {
	const S = 4
	log := &capturingLogger{}
	fetcher, relay, _, sched := relayPairForTest(t, log, 1_000)

	tip := []byte("m0-audit-session-fresh-random-tip")[:32]
	chain, _ := relaypay.BuildChain(tip, S)

	var handle uint64
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), S, FundingEphemeralBlind, func(h uint64, _ error) { handle = h })
	sched.Run()
	for k := 1; k <= S; k++ {
		fetcher.SubmitRelayPay(relay.id, handle, chain.Preimage(k), k, func(int, error) {})
		sched.Run()
	}
	relay.SettleRelaySession(handle)

	// Find the settlement record.
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

	// The forbidden correlatable values: any identity or the chain root.
	forbidden := map[string][]byte{
		"fetcher ephemeral NodeID": fetcher.id[:],
		"relay NodeID":             relay.id[:],
		"chain root":               chain.Root(),
	}
	for i := 0; i < len(rec.kv); i++ {
		val := fmt.Sprintf("%v", rec.kv[i])
		for name, secret := range forbidden {
			// Compare against the raw-byte string and the hex/%x rendering.
			if bytes.Contains([]byte(val), secret) ||
				val == fmt.Sprintf("%x", secret) ||
				val == fmt.Sprintf("%v", secret) {
				t.Fatalf("settlement log leaks a cross-session-stable field (%s) in kv[%d]=%v — M0 residual (design §6) violated", name, i, rec.kv[i])
			}
		}
	}
	// Also assert the keys are exactly the two non-durable ones we intend.
	if len(rec.kv) != 4 { // "increments", <n>, "credit", <n>
		t.Fatalf("settlement log has %d kv items, want 4 (increments, credit) — audit the log line for extra fields", len(rec.kv))
	}
}

// TestRelayWireGuardsFireOnLivePath is failing-first test (5): the Batch-1 M0
// guards and the #644 S-clamp FIRE when driven through MsgRelayOpen. The wire path
// routes through OpenRelaySession, so a durable-funded open, a reused ephemeral
// identity, and an oversized S are all REFUSED over the wire (OK=false), not
// bypassed.
func TestRelayWireGuardsFireOnLivePath(t *testing.T) {
	fetcher, relay, _, sched := relayPairForTest(t, nil, 1_000)
	tip := []byte("live-guard-session-fresh-tip-32by")[:32]
	chain, _ := relaypay.BuildChain(tip, 8)

	// Guard (i): a DURABLE-funded open must be refused over the wire.
	var derr error
	done := false
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), 8, FundingDurableAccount, func(_ uint64, e error) { derr, done = e, true })
	sched.Run()
	if !done || derr == nil {
		t.Fatalf("guard (i) bypassed on the wire: a durable-funded open was accepted (done=%v err=%v)", done, derr)
	}

	// #644 clamp: an oversized S must be refused over the wire.
	done = false
	var serr error
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), relaypay.MaxChainLength+1, FundingEphemeralBlind, func(_ uint64, e error) { serr, done = e, true })
	sched.Run()
	if !done || serr == nil {
		t.Fatalf("#644 clamp bypassed on the wire: an oversized S=%d open was accepted", relaypay.MaxChainLength+1)
	}

	// A valid open succeeds — establishing the fetcher's ephemeral identity as seen.
	done = false
	var handle uint64
	var oerr error
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), 8, FundingEphemeralBlind, func(h uint64, e error) { handle, oerr, done = h, e, true })
	sched.Run()
	if !done || oerr != nil || handle == 0 {
		t.Fatalf("a valid ephemeral-blind open must succeed on the wire: done=%v err=%v handle=%d", done, oerr, handle)
	}

	// Guard (ii): the SAME ephemeral identity (the fetcher node) reusing a session
	// with a FRESH chain must be refused over the wire.
	chain2, _ := relaypay.BuildChain([]byte("live-guard-session-two-fresh-tip!")[:32], 8)
	done = false
	var rerr error
	fetcher.OpenRelaySessionRemote(relay.id, chain2.Root(), 8, FundingEphemeralBlind, func(_ uint64, e error) { rerr, done = e, true })
	sched.Run()
	if !done || rerr == nil {
		t.Fatalf("guard (ii) bypassed on the wire: a reused ephemeral identity opened a second session")
	}
}
