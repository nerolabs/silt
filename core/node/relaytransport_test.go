package node

// PoD §7.3 transport Batch 2 — the wire protocol + settle-at-close (steps 3+4),
// failing-first. These tests drive the LIVE path: a fetcher node opens a paid
// session on a relay node over the in-memory transport (MsgRelayOpen), pays
// increments (MsgRelayPay), and the relay settles at close (RedeemRelayCredit).
//
// The four properties pinned here:
//
//  3. FULL SESSION SETTLEMENT: open → pay N → settle redeems exactly
//     N × increment (≤ budget) via the verifier's monotonic count, RE-SPECIFIED
//     below to the R0.7 interim (pays 0 until R2.14).
//
//  4. M0 ON THE LIVE PATH: the settlement log line carries NO durable or
//     cross-session-stable field (design §6 residual). Asserted against a
//     capturing logger.
//
//  5. LIVE-PATH GUARD REUSE: the Batch-1 M0 guards and the #644 S-clamp FIRE when
//     driven through MsgRelayOpen — the wire path does not bypass them.
//
// R0.7 INTERIM (2026-09-03): TestRelayFullSessionConservedSettlement is
// RE-SPECIFIED to "pays 0 until R2.14" per
// RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md
// §9 step 1 — a PRESCRIBED goalpost move, recorded here, not silent. See also
// core/node/r07_relay_interim_test.go (G-RI-1b, G-RI-2) and
// docs/thinking/2026-09-03-r0.7-relay-interim-design.md.

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
// funds the fetcher's paid-in blind credit so settlement has a source to draw from
// (pre-interim shape; under the R0.7 interim the redeem no longer draws on it, but
// the pre-funding is left in place so this fixture keeps modelling "an honest
// fetcher who really did pay somewhere" for tests that only care about the M0
// guards / wire plumbing, not the settlement amount).
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

// TestRelayFullSessionConservedSettlement is RE-SPECIFIED to the R0.7 interim
// (cert §9 step 1): a full live session over the wire settles for 0, and moves
// no balance on either side. Before R0.7 this pinned "settles for exactly
// count × increment, conserved" — that shape is refuted
// (RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md
// §2: settlement runs on the relay's own ledger and debits an account —
// `sess.ephID` — that ledger has never seen; no anchor exists to justify any
// nonzero payout). RED now: main still pays count × increment.
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
	if paid != 0 {
		t.Fatalf("settlement paid %d for an unanchored session (S=%d paid increments), want 0 (interim: no anchor type exists — R2.14)", paid, S)
	}
	// Nothing moves on either side: the interim performs no ledger mutation.
	if got := ledger.Balance(relayID); got != relayBalBefore {
		t.Fatalf("relay balance moved %d → %d on an unanchored settlement, want unchanged", relayBalBefore, got)
	}
	if got := ledger.Balance(fetcherID); got != fetcherBalBefore {
		t.Fatalf("fetcher balance moved %d → %d on an unanchored settlement, want unchanged", fetcherBalBefore, got)
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
// R0.7 interim (2026-09-03): the line also carries `reason=no-anchor` (G-RI-2,
// core/node/r07_relay_interim_test.go), so the kv count below is 6, not 4. The
// reason is a constant string, not a per-session value, so it adds no
// correlatable field; the forbidden-value scan above still covers it.
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
	// Also assert the keys are exactly the three we intend: the two per-session
	// values and the constant S5 reason.
	if len(rec.kv) != 6 { // "increments", <n>, "credit", <n>, "reason", "no-anchor"
		t.Fatalf("settlement log has %d kv items, want 6 (increments, credit, reason) — audit the log line for extra fields", len(rec.kv))
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

// floodOpenMsg builds a synthetic MsgRelayOpen for a fresh-identity attacker open:
// a distinct ephemeral `from` and a distinct chain root, funding-blind, S valid. It
// is the exact wire input handleRelayOpen decodes on every guard-passing open.
func floodOpenMsg(t *testing.T, seq uint64) (ports.NodeID, ports.Message) {
	t.Helper()
	// Distinct fresh ephemeral identity per open (guard (ii) rejects reuse, so a
	// flood MUST use fresh identities — this mirrors the attacker).
	from := ports.HashBytes([]byte(fmt.Sprintf("flood-eph-%d", seq)))
	// Distinct fresh chain root per open (guard (ii) rejects root reuse too).
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

// TestRelayOpenFloodStaysBounded is the Batch-2 leak-fix failing-first test (PE
// ruling 2026-08-30). A flood of fresh-identity MsgRelayOpen messages with NO
// settlement must NOT grow relaySessions without bound. Two bounds hold it:
//
//  1. HARD CAP: within a single epoch, once the table is at relayMaxLiveSessions
//     every further open is refused (OK=false), so the table never exceeds the cap.
//     (Ablation: remove the cap check in OpenRelaySession → the table grows to N and
//     this assertion reddens.)
//  2. EPOCH SWEEP: after an epoch advance past the retention window, stale unsettled
//     sessions are reaped, so the table does not carry a full epoch's admits forever.
//     (Ablation: remove the session loop in sweepRelaySeen → the pre-advance
//     sessions survive and the post-sweep assertion reddens.)
//
// Removing BOTH the cap and the sweep turns this test RED (unbounded growth to N).
func TestRelayOpenFloodStaysBounded(t *testing.T) {
	relay := newRelayTestNode(7002)
	relay.EnableRelayAccept()

	// Drive the epoch from a test-controlled variable so the sweep can be triggered
	// deterministically without a full chain.
	var epoch uint64
	relay.setRelayEpochFnForTest(func() uint64 { return epoch })

	// (1) HARD CAP: flood N >> cap fresh-identity opens within one epoch, none
	// settled. The table must stay at or below the cap the whole time.
	const N = relayMaxLiveSessions * 3
	epoch = 0
	for i := uint64(0); i < N; i++ {
		from, msg := floodOpenMsg(t, i)
		relay.handleRelayOpen(from, msg)
		if got := relay.relayLiveSessionCountForTest(); got > relayMaxLiveSessions {
			t.Fatalf("session table grew to %d after %d wire opens — exceeds the hard cap %d (unbounded-growth leak)", got, i+1, relayMaxLiveSessions)
		}
	}
	atCap := relay.relayLiveSessionCountForTest()
	if atCap != relayMaxLiveSessions {
		t.Fatalf("after a flood of %d opens the table holds %d, want it pinned at the cap %d", N, atCap, relayMaxLiveSessions)
	}

	// (2) EPOCH SWEEP: advance the epoch past the retention window. The next open
	// triggers sweepRelaySeen, which must reap the stale (epoch-0) unsettled sessions.
	// After the advance the table drops far below the cap — the pre-advance flood did
	// NOT survive.
	epoch = relayRetentionEpochs + 1 // two epochs on, epoch-0 sessions age out
	from, msg := floodOpenMsg(t, N)  // one fresh open to trigger the lazy sweep
	relay.handleRelayOpen(from, msg)
	if got := relay.relayLiveSessionCountForTest(); got > 1 {
		t.Fatalf("after an epoch advance past retention the table holds %d stale sessions — the epoch sweep did not reap unsettled sessions (leak survives across epochs)", got)
	}
}

// TestRelaySettledSessionRemovedPromptly pins the drain side of the fix: a
// legitimately-settled session is removed from the table immediately at settlement
// (the existing single-settle path), independent of any sweep. This guards against
// a sweep-only fix that would leave settled sessions lingering until the next epoch.
func TestRelaySettledSessionRemovedPromptly(t *testing.T) {
	const S = 4
	fetcher, relay, _, sched := relayPairForTest(t, nil, 1_000)
	chain, _ := relaypay.BuildChain([]byte("prompt-removal-fresh-random-tip!!")[:32], S)

	var handle uint64
	fetcher.OpenRelaySessionRemote(relay.id, chain.Root(), S, FundingEphemeralBlind, func(h uint64, _ error) { handle = h })
	sched.Run()
	if relay.relayLiveSessionCountForTest() != 1 {
		t.Fatalf("after one open the table holds %d sessions, want 1", relay.relayLiveSessionCountForTest())
	}
	relay.SettleRelaySession(handle)
	if got := relay.relayLiveSessionCountForTest(); got != 0 {
		t.Fatalf("after settlement the table holds %d sessions, want 0 — settled sessions must be removed promptly, not left for the sweep", got)
	}
}
