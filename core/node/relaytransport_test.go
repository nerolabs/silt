package node

// PoD §7.3 transport Batch 2 — the wire protocol + settle-at-close (steps 3+4),
// failing-first. These tests drive the LIVE path: a fetcher node opens a paid
// session on a relay node over the in-memory transport (MsgRelayOpen), pays
// increments (MsgRelayPay), and the relay settles at close (RedeemRelayCredit).
//
// The four properties pinned here:
//
//  3. FULL SESSION SETTLEMENT: open → pay N → settle redeems exactly
//     min(N × increment, Σ face) via the verifier's monotonic count (R2.14: the
//     session is anchored; the R0.7 interim's pay-0 re-specification is retired).
//
//  4. M0 ON THE LIVE PATH: the settlement log line carries NO durable or
//     cross-session-stable field (design §6 residual). Asserted against a
//     capturing logger.
//
//  5. LIVE-PATH GUARD REUSE: the Batch-1 M0 guards and the #644 S-clamp FIRE when
//     driven through MsgRelayOpen — the wire path does not bypass them.
//
// R2.14 (2026-09-04): TestRelayFullSessionConservedSettlement is RE-SPECIFIED
// back to a paying settle — a PRESCRIBED goalpost move, recorded here, not silent
// (the R0.7 interim, 2026-09-03, had re-specified it to "pays 0"). See
// core/node/r214_relay_anchor_test.go for the anchor gates and
// docs/thinking/2026-09-04-r2.14-relay-prepayment-anchor-design.md.

import (
	"bytes"
	"crypto/ed25519"
	"errors"
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
// with the relay accepting PayWord chains, holding a ledger for settlement, and —
// R2.14 — holding a chain-committed demand key_0 so anchors it signs verify under
// its own keyset. The fetcher node IS the session ephemeral (its signer commits
// the open; sha256(pub) == its NodeID). No balance is pre-funded anywhere: the
// only money that ever reaches this ledger is what an anchor purchase burns.
func relayPairForTest(t *testing.T, relayLog ports.Logger) (fetcher, relay *Node, ledger *credit.Ledger, sched *simclock.Scheduler) {
	t.Helper()
	sched = simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())

	fID := identity.FromSeed(7001) // the fetcher's FRESH EPHEMERAL identity
	rID := identity.FromSeed(7002) // the relay operator

	fetcher = New(fID.NodeID(), DefaultConfig(), sched, net.Endpoint(fID.NodeID()), memstore.New())
	fetcher.SetSigner(fID.Signer())
	relay = New(rID.NodeID(), DefaultConfig(), sched, net.Endpoint(rID.NodeID()), memstore.New())
	if relayLog != nil {
		relay.SetLogger(relayLog)
	}

	ledger = credit.New(50_000, 0)
	ledger.Register(rID.NodeID()) // the relay's own account (see newAnchorCluster)
	relay.SetLedger(ledger)
	commitSelfDemandKey(t, relay, rID, cachedRSAKey(t, 0))
	relay.EnableRelayAccept()

	// Route the fetcher to the relay so `request` can reach it.
	fetcher.Bootstrap([]ports.NodeID{rID.NodeID()}, func() {})
	relay.Bootstrap([]ports.NodeID{fID.NodeID()}, func() {})
	sched.Run()
	return fetcher, relay, ledger, sched
}

// TestRelayFullSessionConservedSettlement: a full live session over the wire —
// anchored with one credential — settles for exactly min(count × increment, face)
// into the relay's balance, moves no other account, never registers the ephemeral,
// and a second settle is a no-op. (Under the R0.7 interim this pinned "pays 0";
// R2.14 retires that.)
func TestRelayFullSessionConservedSettlement(t *testing.T) {
	const S = 6
	fetcher, relay, ledger, sched := relayPairForTest(t, nil)

	tip := []byte("full-session-fresh-random-tip-32b")[:32]
	chain, _ := relaypay.BuildChain(tip, S)

	relayID := relay.id
	relayBalBefore := ledger.Balance(relayID)
	accountsBefore := len(ledger.Balances())

	// Open the session over the wire, anchored with one credential.
	var handle uint64
	var openErr error
	got := false
	fetcher.OpenRelaySessionRemote(relayID, chain.Root(), S, FundingEphemeralBlind, mintAnchorsFor(t, relay, 0, 1), func(h uint64, e error) {
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

	// Settle at close: redeem the highest held preimage ONCE, bounded by the anchor.
	paid := relay.SettleRelaySession(handle)
	if want := int64(S) * relaypay.RelayIncrementCredit; paid != want {
		t.Fatalf("settlement paid %d for an anchored session with S=%d paid increments, want min(count × inc, face) = %d", paid, S, want)
	}
	if got := ledger.Balance(relayID); got != relayBalBefore+paid {
		t.Fatalf("relay balance %d, want %d + settled %d", got, relayBalBefore, paid)
	}
	// The ephemeral is never registered: only the relay's account may move.
	if got := len(ledger.Balances()); got != accountsBefore {
		t.Fatalf("settlement changed the account set %d → %d — acct(ephID) was touched", accountsBefore, got)
	}
	// A second settle is still a no-op (single settlement at close — the session is gone).
	if again := relay.SettleRelaySession(handle); again != 0 {
		t.Fatalf("a second settle of the same handle paid %d, want 0 — single-settlement-at-close is violated", again)
	}
}

// TestRelaySettlementLogCarriesNoDurableField is failing-first test (4), the M0
// residual (design §6): the settlement log line must carry NO durable or
// cross-session-stable field. It asserts the "relay session settled" record's keys
// and values contain neither the fetcher's ephemeral NodeID, the relay's NodeID,
// nor the chain root — only per-session, non-correlatable values.
//
// The line also carries `reason=anchored` (R2.14; the R0.7 interim's `no-anchor`
// retired — the recorded S5 goalpost move, cmd/silt/observable_contract.go), so
// the kv count below is 6, not 4. The reason is a constant string, not a
// per-session value, so it adds no correlatable field; the forbidden-value scan
// above still covers it.
func TestRelaySettlementLogCarriesNoDurableField(t *testing.T) {
	const S = 4
	log := &capturingLogger{}
	fetcher, relay, _, sched := relayPairForTest(t, log)

	tip := []byte("m0-audit-session-fresh-random-tip")[:32]
	chain, _ := relaypay.BuildChain(tip, S)

	var handle uint64
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), S, FundingEphemeralBlind, mintAnchorsFor(t, relay, 0, 1), func(h uint64, _ error) { handle = h })
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
	// Also assert the keys are exactly the three we intend: the two per-session
	// values and the constant S5 reason.
	if len(rec.kv) != 6 { // "increments", <n>, "credit", <n>, "reason", "anchored"
		t.Fatalf("settlement log has %d kv items, want 6 (increments, credit, reason) — audit the log line for extra fields", len(rec.kv))
	}
}

// TestRelayWireGuardsFireOnLivePath is failing-first test (5): the Batch-1 M0
// guards and the #644 S-clamp FIRE when driven through MsgRelayOpen. The wire path
// routes through OpenRelaySession, so a durable-funded open, a reused ephemeral
// identity, and an oversized S are all REFUSED over the wire (OK=false), not
// bypassed.
func TestRelayWireGuardsFireOnLivePath(t *testing.T) {
	fetcher, relay, _, sched := relayPairForTest(t, nil)
	tip := []byte("live-guard-session-fresh-tip-32by")[:32]
	chain, _ := relaypay.BuildChain(tip, 8)
	anchors := mintAnchorsFor(t, relay, 0, 1)

	// Guard (i): a DURABLE-funded open must be refused over the wire.
	var derr error
	done := false
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), 8, FundingDurableAccount, anchors, func(_ uint64, e error) { derr, done = e, true })
	sched.Run()
	if !done || derr == nil {
		t.Fatalf("guard (i) bypassed on the wire: a durable-funded open was accepted (done=%v err=%v)", done, derr)
	}

	// #644 clamp: an oversized S must be refused over the wire.
	done = false
	var serr error
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), relaypay.MaxChainLength+1, FundingEphemeralBlind, anchors, func(_ uint64, e error) { serr, done = e, true })
	sched.Run()
	if !done || serr == nil {
		t.Fatalf("#644 clamp bypassed on the wire: an oversized S=%d open was accepted", relaypay.MaxChainLength+1)
	}

	// A valid open succeeds — establishing the fetcher's ephemeral identity as seen.
	done = false
	var handle uint64
	var oerr error
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), 8, FundingEphemeralBlind, anchors, func(h uint64, e error) { handle, oerr, done = h, e, true })
	sched.Run()
	if !done || oerr != nil || handle == 0 {
		t.Fatalf("a valid ephemeral-blind open must succeed on the wire: done=%v err=%v handle=%d", done, oerr, handle)
	}

	// Guard (ii): the SAME ephemeral identity (the fetcher node) reusing a session
	// with a FRESH chain must be refused over the wire.
	chain2, _ := relaypay.BuildChain([]byte("live-guard-session-two-fresh-tip!")[:32], 8)
	done = false
	var rerr error
	fetcher.OpenRelaySessionRemote(relay.id, chain2.Root(), 8, FundingEphemeralBlind, mintAnchorsFor(t, relay, 0, 1), func(_ uint64, e error) { rerr, done = e, true })
	sched.Run()
	if !done || rerr == nil {
		t.Fatalf("guard (ii) bypassed on the wire: a reused ephemeral identity opened a second session")
	}
}

// floodOpenMsg builds a synthetic UNANCHORED MsgRelayOpen for a fresh-identity
// attacker open: a distinct ephemeral `from` and a distinct chain root, funding-
// blind, S valid, no anchors. It is the exact wire input handleRelayOpen decodes.
func floodOpenMsg(t *testing.T, seq uint64) (ports.NodeID, ports.Message) {
	t.Helper()
	from := ports.HashBytes([]byte(fmt.Sprintf("flood-eph-%d", seq)))
	root := make([]byte, 32)
	binaryPutUint64(root, seq)
	open := relaypay.RelayOpen{Root: root, S: 8, Funding: int(FundingEphemeralBlind)}
	blob, err := open.Marshal()
	if err != nil {
		t.Fatalf("marshal flood open %d: %v", seq, err)
	}
	return from, ports.Message{Kind: ports.MsgRelayOpen, Data: blob}
}

func binaryPutUint64(b []byte, v uint64) {
	for i := 0; i < 8; i++ {
		b[i] = byte(v >> (8 * uint(i)))
	}
}

// TestRelayOpenFloodStaysBounded is the Batch-2 leak-fix test (PE ruling
// 2026-08-30), RE-SPECIFIED for R2.14 (recorded goalpost move): admission is now
// PRICED at ≥ one anchor face per session, so a flood of free fresh-identity opens
// cannot fill the table at all. Three bounds hold it:
//
//  0. PRICED ADMISSION (R2.14; RT-RELAY-3's "sessions are free" half, closed): N
//     unanchored wire opens are ALL refused — the table stays EMPTY.
//     (Ablation: admit an open with no anchors → the table grows and this reddens.)
//  1. HARD CAP: with the table at relayMaxLiveSessions live sessions, a further
//     ANCHORED open is refused (errRelaySessionCap) BEFORE its anchor is spent —
//     the same anchor then opens once a slot frees. (Ablation: remove the cap
//     check → the table exceeds the cap.)
//  2. EPOCH SWEEP: after an epoch advance past the retention window, stale
//     unsettled sessions are reaped. (Ablation: remove the session loop in
//     sweepRelaySeen → the pre-advance sessions survive.)
//
// The table is filled to the cap by DIRECT insertion (relaySessions is package
// state) rather than 4,096 anchored wire opens: at ~2 ms of RSA per anchored open
// that flood would cost the short tier ~10 s, and the cap check does not read
// how a session got into the table.
func TestRelayOpenFloodStaysBounded(t *testing.T) {
	relay := newAnchoredRelayTestNode(t, 7002)
	relay.EnableRelayAccept()

	var epoch uint64
	relay.setRelayEpochFnForTest(func() uint64 { return epoch })

	// (0) PRICED ADMISSION: an unanchored flood admits nothing.
	const N = 256
	epoch = 0
	for i := uint64(0); i < N; i++ {
		from, msg := floodOpenMsg(t, i)
		relay.handleRelayOpen(from, msg)
	}
	if got := relay.relayLiveSessionCountForTest(); got != 0 {
		t.Fatalf("session table holds %d sessions after %d UNANCHORED wire opens, want 0 — admission is free again (R2.14 regression)", got, N)
	}

	// (1) HARD CAP: fill the table to the cap, then one anchored open is refused.
	for i := 0; i < relayMaxLiveSessions; i++ {
		relay.relaySessionSeq++
		relay.relaySessions[relay.relaySessionSeq] = &RelaySession{admitEpoch: 0, signal: make(chan struct{}, 1)}
	}
	c, _ := relaypay.BuildChain([]byte("flood-cap-anchored-open-fresh-tip")[:32], 8)
	e := newEphemeral(70021)
	anchors := mintAnchorsFor(t, relay, 0, 1)
	sig := ed25519.Sign(e.priv, relayOpenCommitment(relay.id, c.Root(), 8, anchors))
	if _, err := relay.OpenRelaySession(e.id, c.Root(), 8, FundingEphemeralBlind, anchors, e.pub, sig); !errors.Is(err, errRelaySessionCap) {
		t.Fatalf("an anchored open at the cap returned err=%v, want errRelaySessionCap — the hard cap is missing", err)
	}
	if got := relay.relayLiveSessionCountForTest(); got != relayMaxLiveSessions {
		t.Fatalf("table holds %d after a refused open at the cap, want exactly the cap %d", got, relayMaxLiveSessions)
	}

	// (2) EPOCH SWEEP: advance past retention; the SAME ephemeral, root and anchor
	// now open (the cap refusal recorded nothing and spent nothing), and the stale
	// sessions are gone.
	epoch = relayRetentionEpochs + 1
	if _, err := relay.OpenRelaySession(e.id, c.Root(), 8, FundingEphemeralBlind, anchors, e.pub, sig); err != nil {
		t.Fatalf("after the epoch advance the refused-at-cap open did not succeed: %v — the cap refusal spent the anchor or recorded the identity", err)
	}
	// OpenRelaySession returns the session without inserting it (handleRelayOpen
	// does the insert), so the table now holds exactly the survivors of the sweep.
	if got := relay.relayLiveSessionCountForTest(); got != 0 {
		t.Fatalf("after an epoch advance past retention the table holds %d sessions, want 0 — the epoch sweep did not reap the stale unsettled sessions", got)
	}
}

// TestRelaySettledSessionRemovedPromptly pins the drain side of the fix: a
// legitimately-settled session is removed from the table immediately at settlement
// (the existing single-settle path), independent of any sweep. This guards against
// a sweep-only fix that would leave settled sessions lingering until the next epoch.
func TestRelaySettledSessionRemovedPromptly(t *testing.T) {
	const S = 4
	fetcher, relay, _, sched := relayPairForTest(t, nil)
	chain, _ := relaypay.BuildChain([]byte("prompt-removal-fresh-random-tip!!")[:32], S)

	var handle uint64
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), S, FundingEphemeralBlind, mintAnchorsFor(t, relay, 0, 1), func(h uint64, _ error) { handle = h })
	sched.Run()
	if relay.relayLiveSessionCountForTest() != 1 {
		t.Fatalf("after one open the table holds %d sessions, want 1", relay.relayLiveSessionCountForTest())
	}
	relay.SettleRelaySession(handle)
	if got := relay.relayLiveSessionCountForTest(); got != 0 {
		t.Fatalf("after settlement the table holds %d sessions, want 0 — settled sessions must be removed promptly, not left for the sweep", got)
	}
}
