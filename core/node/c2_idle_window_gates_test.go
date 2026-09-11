package node

// Lane C2 — the DELIVERY IDLE WINDOW (`R-REAPER-FORFEIT`, ROADMAP row C2; owner call 4
// of `D-TRUE-UP-CALLS-2026-09-07`, which keeps `-delivery-idle-window` REFUSE-UNTIL-SET
// until Lane A's liveness bound is field-confirmed and then sets the default ABOVE it).
//
// These gates pin the PROPERTY, never a number: the reaper's GUARANTEED survival — the
// shortest gap since a real settlement at which a reap can fire — must dominate the worst
// stall the published liveness model admits. The number itself is bound to the shipped
// flag default by the cmd/silt half (c2_idle_window_default_test.go, RED until a Builder
// sets it).
//
// EVERY input is named and sourced:
//
//	c2BoundGoverning       430 s  docs/decisions.md D-H43-WORKLESS-DESIGNEE (21): a LOST
//	                              entry forward is bounded by the re-keyed takeover at
//	                              ≤ (N+2)·ChainSyncInterval + G = 14·30 + 10 at N = 12.
//	c2BoundModal           190 s  D-CONSENSUS-ARMING (19): ≤ f′+1 rounds at f = 1, N = 12
//	                              = (2+3)·30 + 30 + 10; the arithmetic is in the tree at
//	                              integration/cloudtest/scenarios.sh:512.
//	c2BoundHarnessHardCap  380 s  the cloudtest 6-fault-tolerance hard cap = 2× the tier
//	                              (scenarios.sh:526-527), for sweep-phase/timeout noise.
//	c2FieldStallObserved  1040 s  the ONE stall the field produced: run c450985-deep,
//	                              block 43 committed 17 min 20 s after block 42. That is
//	                              the DEFECT the A1 fix closed, not a bound — it is
//	                              carried as the margin datum, never as the estimand.
//	deliveryStampDivisor       4  core/node/deliverysession.go — the reaper's stamp
//	                              coarsening, pinned by TestC2StampDivisorIsFour.
//
// The field confirmation the ratified sentence waits on: integration/cloudtest/
// report-97e3101-deep.md, row `6-fault-tolerance` ("within the computed 190s down-designee
// escape bound") and row `10a-stall-drill` ("within the computed 430s bound").

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

const (
	c2BoundGoverning      = 430 * ports.Second
	c2BoundModal          = 190 * ports.Second
	c2BoundHarnessHardCap = 380 * ports.Second
	c2FieldStallObserved  = 1040 * ports.Second

	// c2EpochBlocks mirrors cmd/silt DerivedEpochBlocks (pinned there by
	// TestC2DerivedEpochBlocksIsEight); core/node cannot import package main.
	c2EpochBlocks = 8
	// c2MeasuredTbMillis is the MEASURED block interval, in milliseconds:
	// integration/cloudtest/report-97e3101-deep.md row 12-deep-heights, h93 → h130
	// (37 heights) in 1697 s = 45.865 s/height. A measurement on a 4-validator GCP
	// cohort, not a constant of the protocol — every figure derived from it is a figure
	// about that cohort.
	c2MeasuredTbMillis = 45865

	// The two candidate defaults the derivation lands on. Both are DURATIONS, because
	// the flag is a duration and the liveness bound is denominated in seconds
	// (ChainSyncInterval), not in blocks: an epoch-denominated default would drift with
	// T_b while the bound it must dominate would not.
	c2CandidateTight = 600 * ports.Second // 10m — the smallest round duration whose
	//                                        guaranteed survival (450 s) clears 430 s.
	c2CandidateMargin = 1440 * ports.Second // 24m — clears 430 s at 2.51× and also
	//                                         dominates the 1040 s observed field stall.
)

// c2GuaranteedSurvival is the shortest gap since a REAL settlement at which the reaper can
// fire, for a configured window. Derived from the shipped coarsening, never assumed:
// deliveryStamp floors the stamp into buckets of g = idle/deliveryStampDivisor, so a
// settlement landing just under a bucket boundary is stamped almost a whole g in the past.
func c2GuaranteedSurvival(idle ports.Duration) ports.Duration {
	return idle - idle/deliveryStampDivisor
}

// c2Server: a server node with a REAL epoch clock, a chain-committed demand key at epoch 0
// and at `epoch`, the delivery lane on with window `idle`, and the scheduler that IS its
// clock. The chain does not advance unless a test advances it — that is the STALL.
func c2Server(t *testing.T, idle ports.Duration, epoch int) (*Node, *rsa.PrivateKey, *chain.Chain, *credit.Ledger, *simclock.Scheduler) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 2, simnet.DefaultConfig())
	ident := identity.FromSeed(7401)
	nd := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	nd.SetSigner(ident.Signer())
	signer := ident.Signer()
	key0, keyE := cachedRSAKey(t, 3), cachedRSAKey(t, 4)
	c := c3Chain(t, 1, signer,
		chain.SignIssuerKeyReg(signer, 0, demand.KeyFingerprint(&key0.PublicKey)),
		chain.SignIssuerKeyReg(signer, uint64(epoch), demand.KeyFingerprint(&keyE.PublicKey)))
	ledger := credit.New(50_000, 0)
	src := &settableEpoch{e: uint64(epoch)}
	ledger.SetEpochSource(src)
	nd.SetLedger(ledger)
	nd.EnableChain(c, signer)
	nd.SetDemandIssuerKey(rand.Reader, 0, key0)
	c3Advance(t, c, []ed25519.PrivateKey{signer}, epoch)
	nd.SetDemandIssuerKey(rand.Reader, uint64(epoch), keyE)
	nd.EnableDemandBank(nd.id)
	nd.EnableDeliverySessions(idle)
	ledger.Register(nd.id)
	return nd, keyE, c, ledger, sched
}

// c2OpenAndSettleAt opens a session for a fresh fetcher, drives the clock so the REAL
// settlement lands at `phase` inside a stamp bucket, settles once, and returns the handle
// and the wall time of that real settlement.
func c2OpenAndSettleAt(t *testing.T, nd *Node, keyE *rsa.PrivateKey, ledger *credit.Ledger, sched *simclock.Scheduler, epoch uint64, seed int64, phase ports.Duration) (uint64, ports.Time) {
	t.Helper()
	g := ports.Duration(nd.deliveryIdle / deliveryStampDivisor)
	fID := identity.FromSeed(seed)
	ledger.Register(fID.NodeID())
	base := sched.Now()
	bucket := base - base%ports.Time(g) + ports.Time(g)
	sched.RunUntil(bucket.Add(phase))
	s, err := nd.OpenDeliverySession(fID.NodeID(), demand.SignSessionOpen(fID.Signer(), nd.id, []demand.Token{mintDemandTokenUnder(t, keyE, nd.chainID(), epoch)}))
	if err != nil {
		t.Fatalf("open (seed %d, phase %d ns): %v", seed, phase, err)
	}
	obj := ports.HashBytes([]byte("c2-gate"))
	if _, err := nd.SettleDeliveryReceipt(fID.NodeID(), demand.AckSession(fID.Signer(), s.handle, s.commitment, obj, nd.id, 1)); err != nil {
		t.Fatalf("settle (seed %d): %v", seed, err)
	}
	return s.handle, sched.Now()
}

// c2AliveAfter reports whether the session is still live once the wall clock has advanced
// `gap` past its real settlement, with the reaper swept at the end of the gap.
func c2AliveAfter(nd *Node, sched *simclock.Scheduler, handle uint64, settledAt ports.Time, gap ports.Duration) bool {
	sched.RunUntil(settledAt.Add(gap))
	nd.sweepDeliverySessions(sched.Now())
	_, alive := nd.DeliverySessionForTest(handle)
	return alive
}

// ---------------------------------------------------------------------------
// G-C2-1 — the stamp divisor is 4. The whole derivation's 3/4 factor rides on it, and
// it lives in a package cmd/silt cannot import, so it is pinned here and cited there.
// ABLATION: change deliveryStampDivisor to 2 or 8 ⇒ RED here AND the cmd/silt floor moves.
func TestC2StampDivisorIsFour(t *testing.T) {
	if deliveryStampDivisor != 4 {
		t.Fatalf("deliveryStampDivisor = %d, want 4 — cmd/silt's c2StampDivisor literal and the derived idle floor both move with it", deliveryStampDivisor)
	}
}

// ---------------------------------------------------------------------------
// G-C2-2 — GUARANTEED SURVIVAL IS 3/4 OF THE CONFIGURED WINDOW, driven at every stamp
// phase. This is the correction the ROADMAP's starting arithmetic does not carry: sizing
// the window AT the bound leaves a quarter of it unavailable, so a window of exactly D
// reaps a session that has been gapped only 0.75·D.
// ABLATION: return `idle` from c2GuaranteedSurvival (drop the coarsening term) ⇒ RED.
func TestC2GuaranteedSurvivalIsThreeQuartersOfTheWindow(t *testing.T) {
	const idle = 1000 * ports.Second
	const E = 1
	nd, keyE, _, ledger, sched := c2Server(t, idle, E)
	g := ports.Duration(idle / deliveryStampDivisor)
	want := c2GuaranteedSurvival(idle)

	worst := ports.Duration(1<<62 - 1)
	for i, phase := range []ports.Duration{0, g / 4, g / 2, 3 * g / 4, g - 1} {
		handle, settledAt := c2OpenAndSettleAt(t, nd, keyE, ledger, sched, E, int64(7600+i), phase)
		// One step INSIDE the guaranteed survival: must be alive at every phase.
		if !c2AliveAfter(nd, sched, handle, settledAt, want-1) {
			t.Fatalf("phase %d ns: reaped at %v after a real settlement — inside the guaranteed survival %v", phase, want-1, want)
		}
		// Walk to the reap and record it.
		var reapedAt ports.Time
		for step := ports.Duration(0); step <= idle; step += ports.Second {
			if !c2AliveAfter(nd, sched, handle, settledAt, want-1+step) {
				reapedAt = sched.Now()
				break
			}
		}
		if reapedAt == 0 {
			t.Fatalf("phase %d ns: never reaped within %v", phase, idle)
		}
		elapsed := ports.Duration(reapedAt - settledAt)
		t.Logf("G-C2-2 phase=%6ds  window=%ds  reaped %ds after the REAL settlement (%.1f%% of the window)",
			phase/ports.Second, idle/ports.Second, elapsed/ports.Second, 100*float64(elapsed)/float64(idle))
		if elapsed < worst {
			worst = elapsed
		}
	}
	// The measured worst case must be the derived one, to within the 1 s walk step.
	if worst < want || worst > want+ports.Second {
		t.Fatalf("measured guaranteed survival %v, derived %v (idle − idle/%d) — the derivation's 3/4 factor does not hold on the shipped reaper",
			worst, want, deliveryStampDivisor)
	}
	t.Logf("G-C2-2 RESULT: guaranteed survival = %ds of a configured %ds window = %.3f×",
		worst/ports.Second, idle/ports.Second, float64(worst)/float64(idle))
}

// ---------------------------------------------------------------------------
// G-C2-3 — THE PROPERTY. A session that settled once and is then gapped by the WORST stall
// the published model admits must SURVIVE, at every stamp phase — and a window sized naively
// AT the bound must NOT. The stall is DRIVEN, not mocked: the chain is frozen for the whole
// gap while the node's wall clock advances.
// ABLATION: size the window at c2BoundGoverning (the "naive" arm below) ⇒ the survive
// assertion is RED, which is exactly why the arm is here.
func TestC2SessionSurvivesTheWorstAdmittedStall(t *testing.T) {
	const E = 1
	for _, tc := range []struct {
		name   string
		idle   ports.Duration
		gap    ports.Duration
		expect bool
	}{
		{"naive-window-at-the-bound/430s-gap", c2BoundGoverning, c2BoundGoverning, false},
		{"candidate-tight-10m/430s-gap", c2CandidateTight, c2BoundGoverning, true},
		{"candidate-tight-10m/190s-modal-gap", c2CandidateTight, c2BoundModal, true},
		{"candidate-tight-10m/380s-harness-cap-gap", c2CandidateTight, c2BoundHarnessHardCap, true},
		{"candidate-tight-10m/1040s-observed-field-stall", c2CandidateTight, c2FieldStallObserved, false},
		{"candidate-margin-24m/430s-gap", c2CandidateMargin, c2BoundGoverning, true},
		{"candidate-margin-24m/1040s-observed-field-stall", c2CandidateMargin, c2FieldStallObserved, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nd, keyE, c, ledger, sched := c2Server(t, tc.idle, E)
			_, headBefore := c.Head()
			g := ports.Duration(tc.idle / deliveryStampDivisor)
			for i, phase := range []ports.Duration{0, g / 2, g - 1} {
				handle, settledAt := c2OpenAndSettleAt(t, nd, keyE, ledger, sched, E, int64(7700+10*i)+int64(tc.idle%97), phase)
				alive := c2AliveAfter(nd, sched, handle, settledAt, tc.gap)
				if alive != tc.expect {
					t.Fatalf("phase %d ns: alive=%v after a %v gap on a %v window, want alive=%v (guaranteed survival %v)",
						phase, alive, tc.gap, tc.idle, tc.expect, c2GuaranteedSurvival(tc.idle))
				}
			}
			_, headAfter := c.Head()
			if headBefore != headAfter {
				t.Fatalf("the chain advanced during the stall arm (%d → %d) — the gap was not a stall", headBefore, headAfter)
			}
			t.Logf("G-C2-3 %-46s window=%5ds guaranteed=%5ds gap=%5ds → alive=%v (chain frozen at head %d)",
				tc.name, tc.idle/ports.Second, c2GuaranteedSurvival(tc.idle)/ports.Second, tc.gap/ports.Second, tc.expect, headAfter)
		})
	}
}

// ---------------------------------------------------------------------------
// G-C2-4 — THE ENDPOINTS. At each candidate default, one step either side of the reap
// threshold at the WORST stamp phase. A gate that only asserts "survives 430 s" is
// satisfied by a window of a year; these two rows are what make the threshold a threshold.
func TestC2ReapThresholdEndpoints(t *testing.T) {
	const E = 1
	for _, idle := range []ports.Duration{c2CandidateTight, c2CandidateMargin} {
		nd, keyE, _, ledger, sched := c2Server(t, idle, E)
		g := ports.Duration(idle / deliveryStampDivisor)
		want := c2GuaranteedSurvival(idle)
		h1, t1 := c2OpenAndSettleAt(t, nd, keyE, ledger, sched, E, 7800+int64(idle%89), g-1)
		if !c2AliveAfter(nd, sched, h1, t1, want) {
			t.Fatalf("window %v: reaped AT the guaranteed survival %v — the reaper is tighter than the derived floor, so the floor UNDERSTATES the window a bound needs", idle, want)
		}
		h2, t2 := c2OpenAndSettleAt(t, nd, keyE, ledger, sched, E, 7801+int64(idle%89), g-1)
		if c2AliveAfter(nd, sched, h2, t2, want+1) {
			t.Fatalf("window %v: still alive ONE NANOSECOND past the guaranteed survival %v at the worst stamp phase — the endpoint is not where the derivation puts it", idle, want+1)
		}
		// One step either side in the units the operator sets the flag in.
		h3, t3 := c2OpenAndSettleAt(t, nd, keyE, ledger, sched, E, 7802+int64(idle%89), g-1)
		if !c2AliveAfter(nd, sched, h3, t3, want-ports.Second) {
			t.Fatalf("window %v: reaped a second INSIDE the guaranteed survival %v", idle, want)
		}
		h4, t4 := c2OpenAndSettleAt(t, nd, keyE, ledger, sched, E, 7803+int64(idle%89), g-1)
		if c2AliveAfter(nd, sched, h4, t4, want+ports.Second) {
			t.Fatalf("window %v: still alive a second past the guaranteed survival %v", idle, want)
		}
		t.Logf("G-C2-4 window=%5ds worst stamp phase: ALIVE at gap %v and at %v; REAPED at gap %v and at %v",
			idle/ports.Second, want-ports.Second, want, want+1, want+ports.Second)
	}
}

// ---------------------------------------------------------------------------
// G-C2-5 — the window is not merely long: a GENUINELY idle session is reaped, and its
// remainder is booked ONCE as a DEPOSIT, never burned (B-13 / D-R2.9-NODE-HALF-CALLS 1′).
// ABLATION: drop the `now-s.lastSettle >= idle` reap ⇒ RED.
func TestC2GenuinelyIdleSessionIsReapedAsADeposit(t *testing.T) {
	const E = 1
	nd, keyE, _, ledger, sched := c2Server(t, c2CandidateMargin, E)
	fID := identity.FromSeed(7900)
	ledger.Register(fID.NodeID())
	s, err := nd.OpenDeliverySession(fID.NodeID(), demand.SignSessionOpen(fID.Signer(), nd.id, []demand.Token{mintDemandTokenUnder(t, keyE, nd.chainID(), E)}))
	if err != nil {
		t.Fatal(err)
	}
	sched.RunUntil(sched.Now().Add(c2CandidateMargin))
	nd.sweepDeliverySessions(sched.Now())
	if _, alive := nd.DeliverySessionForTest(s.handle); alive {
		t.Fatalf("a session silent for a whole window was NOT reaped — the window does not reclaim")
	}
	st := ledger.DeliverySettlementStats()
	if st.SessionsClosed != 1 || st.PendingRefundCredits != 50_000 || st.BurnedCredits != 0 {
		t.Fatalf("closed %d / pending %d / burned %d, want 1 / 50000 / 0 — the delivery lane's reap is a DEPOSIT, not a forfeit",
			st.SessionsClosed, st.PendingRefundCredits, st.BurnedCredits)
	}
}

// ---------------------------------------------------------------------------
// G-C2-6 — THE PREMISE CORRECTION, driven. `R-SESSION-WALLCLOCK-STEP` (docs/design/m0.md
// §10) states that "a chain stall … reaps every live session". Measured here: it does not.
// With the chain FROZEN for longer than any admitted stall, the delivery lane still admits
// a session, settles receipts and takes a top-up, and a session that keeps settling is
// never reaped. What a stall reaps is a session idle for some OTHER reason; the stall is
// not the cause. The bound therefore acts as a conservative envelope on how long an honest
// fetcher may be gapped, not as a mechanism the reaper is racing.
func TestC2FrozenChainDoesNotStarveTheDeliveryLane(t *testing.T) {
	const E = 1
	nd, keyE, c, ledger, sched := c2Server(t, c2CandidateTight, E)
	_, headBefore := c.Head()
	fID := identity.FromSeed(7902)
	ledger.Register(fID.NodeID())
	obj := ports.HashBytes([]byte("c2-frozen"))
	s, err := nd.OpenDeliverySession(fID.NodeID(), demand.SignSessionOpen(fID.Signer(), nd.id, []demand.Token{mintDemandTokenUnder(t, keyE, nd.chainID(), E)}))
	if err != nil {
		t.Fatalf("open on a frozen chain: %v", err)
	}
	var count uint64
	for elapsed := ports.Duration(0); elapsed < c2FieldStallObserved; elapsed += 10 * ports.Second {
		sched.RunUntil(sched.Now().Add(10 * ports.Second))
		count++
		got, err := nd.SettleDeliveryReceipt(fID.NodeID(), demand.AckSession(fID.Signer(), s.handle, s.commitment, obj, nd.id, count))
		if err != nil || got <= 0 {
			t.Fatalf("settle %d on a frozen chain at t=%ds: paid %d, err %v", count, int64(sched.Now())/int64(ports.Second), got, err)
		}
	}
	if _, alive := nd.DeliverySessionForTest(s.handle); !alive {
		t.Fatalf("a session settling every 10 s was reaped across a %v chain freeze", c2FieldStallObserved)
	}
	fID2 := identity.FromSeed(7903)
	ledger.Register(fID2.NodeID())
	if _, err := nd.FundDeliverySession(fID.NodeID(), demand.SignSessionFund(fID.Signer(), nd.id, s.handle, []demand.Token{mintDemandTokenUnder(t, keyE, nd.chainID(), E)})); err != nil {
		t.Fatalf("fund on a frozen chain: %v", err)
	}
	_, headAfter := c.Head()
	if headBefore != headAfter {
		t.Fatalf("the chain advanced (%d → %d) — this arm did not run against a stall", headBefore, headAfter)
	}
	t.Logf("G-C2-6 RESULT: chain frozen at head %d for %ds — open OK, %d settlements OK, fund OK, session ALIVE. "+
		"The m0.md §10 sentence 'a chain stall reaps every live session' does not hold for a session with bytes in flight.",
		headAfter, c2FieldStallObserved/ports.Second, count)
}

// ---------------------------------------------------------------------------
// G-C2-7 — THE RELAY LANE'S OWED MEASUREMENT (ROADMAP row C2: "the relay lane keeps the
// burn and its settle-inside-one-epoch measurement is owed before -accept-relay-payments
// is enabled"). Driven, not argued.
//
// The relay lane settles ONCE AT CLOSE (relaytransport.go SettleRelaySession, design §5)
// and its stale-session reaper is EPOCH-keyed, not wall-keyed: sweepRelaySeen drops a
// session whose admitEpoch is below `epoch − relayRetentionEpochs` (= epoch − 1), i.e. at
// epoch admitEpoch+2, WITHOUT settling. So the measurement is not "what fraction settles" —
// it is: a relay session that has not reached its close by epoch E+2 forfeits ONE HUNDRED
// PERCENT of the credit it earned, while the fetcher's face was already spent at open.
func TestC2RelaySessionReapedByTheEpochSweepSettlesZero(t *testing.T) {
	nd := newAnchoredRelayTestNode(t, 1)
	nd.EnableRelayAccept()
	var epoch uint64
	nd.setRelayEpochFnForTest(func() uint64 { return epoch })

	const S = 8
	ch, _ := relaypay.BuildChain([]byte("c2-relay-settle-inside-one-epoch-tip!"), S)
	ch2, _ := relaypay.BuildChain([]byte("c2-relay-the-open-that-triggers-it!!"), S)
	epoch = 0
	sess, err := openAnchored(t, nd, 9, ch.Root(), S)
	if err != nil {
		t.Fatalf("open at epoch 0: %v", err)
	}
	// handleRelayOpen's two lines: OpenRelaySession admits, the WIRE handler assigns the
	// handle and files the session. openAnchored calls the admit half only.
	nd.relaySessionSeq++
	handle := nd.relaySessionSeq
	nd.relaySessions[handle] = sess
	for k := 1; k <= S; k++ {
		if err := sess.Pay(ch.Preimage(k)); err != nil {
			t.Fatalf("increment %d: %v", k, err)
		}
	}
	if sess.Count() != S {
		t.Fatalf("count %d, want %d", sess.Count(), S)
	}
	// Epoch 1: the session survives (retention keeps current + previous).
	epoch = 1
	nd.sweepRelaySeen(epoch)
	if _, ok := nd.RelaySessionForTest(handle); !ok {
		t.Fatalf("the session was reaped one epoch after admission — the retention window is shorter than relayRetentionEpochs claims")
	}
	// Epoch 2: the reap is LAZY. sweepRelaySeen has exactly ONE production call site —
	// inside OpenRelaySession (relayrole.go:366) — and there is no periodic caller: the
	// relay lane has no SweepDeliverySessions twin, and the daemon tickers only the
	// DELIVERY sweep (daemon.go:1160). So on a quiet relay the epoch passes and NOTHING
	// fires. Drive that first, then drive the real trigger: another open.
	epoch = 2
	if _, ok := nd.RelaySessionForTest(handle); !ok {
		t.Fatalf("fixture: the session was already gone before epoch 2 was swept")
	}
	if _, err := openAnchored(t, nd, 11, ch2.Root(), S); err != nil {
		t.Fatalf("the second open (the only production trigger for the relay sweep) failed: %v", err)
	}
	if _, ok := nd.RelaySessionForTest(handle); ok {
		t.Fatalf("the session survived to epoch admitEpoch+2 — the epoch reaper did not fire")
	}
	if paid := nd.SettleRelaySession(handle); paid != 0 {
		t.Fatalf("a reaped relay session settled %d credits; the reap deletes the handle, so the settle is a no-op and the earned credit is FORFEIT", paid)
	}
	// The owed number, computed from SHIPPED constants and the MEASURED block interval,
	// never hand-typed: what sustained relay throughput must a single session hold to
	// settle a whole anchor face before the epoch reap drops it unsettled?
	//
	//   relaypay.MaxSessionBytes           the bytes ONE face funds (S_max × increment bytes)
	//   c2EpochBlocks = 8                  cmd/silt DerivedEpochBlocks, pinned by
	//                                      cmd/silt TestC2DerivedEpochBlocksIsEight
	//   c2MeasuredTbMillis = 45865         report-97e3101-deep.md row 12-deep-heights:
	//                                      h93 → h130 (37 heights) in 1697 s
	//
	// Lifetime in blocks: admitted at block b in [8E, 8E+7], reaped when the head reaches
	// 8(E+2). WORST = 9 blocks (admitted at the last block of E), BEST = 16 blocks.
	worstS := float64(9*c2MeasuredTbMillis) / 1000
	bestS := float64(16*c2MeasuredTbMillis) / 1000
	t.Logf("G-C2-7 RESULT: a relay session admitted at epoch E survives E and E+1 and is reaped UNSETTLED at E+2. "+
		"It forwarded %d increments and was paid 0 — the reap deletes the handle, so 100%% of the earned credit is forfeit "+
		"(the fetcher's face was already spent at open). The reap is LAZY: its only production "+
		"caller is OpenRelaySession, so on a quiet relay it does not fire at all — the relay lane "+
		"has no periodic sweep, unlike the delivery lane's SweepDeliverySessions ticker.", S)
	t.Logf("G-C2-7 SETTLE-INSIDE-ONE-EPOCH (the owed measurement): lifetime = 9 blocks (worst: admitted at the last block "+
		"of E) to 16 blocks (best), = %.0f s to %.0f s at the measured T_b = %.3f s/height. One face funds %d B (%.3f GiB), "+
		"so settling a WHOLE face inside the lifetime needs %.1f Mbit/s sustained (worst) / %.1f Mbit/s (best) on ONE session. "+
		"At a 100 Mbit/s edge uplink a session forwards %.2f GiB of the face's %.2f GiB before the reap — and because the "+
		"settle is single-at-close, an over-running session is paid ZERO, not a fraction.",
		worstS, bestS, float64(c2MeasuredTbMillis)/1000, relaypay.MaxSessionBytes, float64(relaypay.MaxSessionBytes)/(1<<30),
		float64(relaypay.MaxSessionBytes)*8/worstS/1e6, float64(relaypay.MaxSessionBytes)*8/bestS/1e6,
		100e6/8*worstS/(1<<30), float64(relaypay.MaxSessionBytes)/(1<<30))
}
