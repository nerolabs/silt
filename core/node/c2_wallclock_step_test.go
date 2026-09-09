package node

// Lane C2 — the half of `R-SESSION-WALLCLOCK-STEP` that SURVIVES the premise correction.
//
// The residual as written in docs/design/m0.md §10 made two claims: that a forward
// wall-clock STEP reaps every live session, and that a CHAIN STALL does the same. The
// second is refuted by measurement (TestC2FrozenChainDoesNotStarveTheDeliveryLane: with
// the chain frozen for longer than any admitted stall the lane still admits, settles and
// funds, because nothing in the settle path reads the chain). The first is not refuted,
// and it is the reason the disclosure stays. It had never been driven either, so it is
// driven here: a claim about what a mechanism does to live sessions is a MEASUREMENT.

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

// G-C2-16 — a forward wall-clock step reaps EVERY live session at once, at any stamp
// phase, however recently each one settled; and the step is the cause, since the same
// sessions live through a step one nanosecond short of the guaranteed survival.
// ABLATION: key the reaper on anything but the injected clock (e.g. never advance
// n.deliveryIdle's comparison) ⇒ the reaped arm goes RED.
func TestC2ForwardWallClockStepReapsEveryLiveSession(t *testing.T) {
	const E = 1
	const idle = c2CandidateMargin // the SHIPPED default (cmd/silt deliveryIdleDefault)
	g := ports.Duration(idle / deliveryStampDivisor)

	for _, tc := range []struct {
		name     string
		step     ports.Duration
		wantLive bool
	}{
		{"one-ns-short-of-the-guaranteed-survival", c2GuaranteedSurvival(idle) - 1, true},
		{"a-step-of-the-whole-window", idle, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nd, keyE, _, ledger, sched := c2Server(t, idle, E)
			handles := make([]uint64, 0, 3)
			var earliest, latest ports.Time
			for i, phase := range []ports.Duration{0, g / 2, g - 1} {
				h, at := c2OpenAndSettleAt(t, nd, keyE, ledger, sched, E, int64(7910+i), phase)
				handles = append(handles, h)
				if earliest == 0 || at < earliest {
					earliest = at
				}
				if at > latest {
					latest = at
				}
			}
			if n := nd.LiveDeliverySessions(); n != 3 {
				t.Fatalf("fixture: %d live sessions, want 3", n)
			}
			// The STEP: one jump of the node's clock, then one sweep. Measured from the
			// LATEST settlement on the reaped arm (every session is gapped at least this
			// far) and from the EARLIEST on the surviving arm (none is gapped further).
			from := latest
			if tc.wantLive {
				from = earliest
			}
			sched.RunUntil(from.Add(tc.step))
			nd.sweepDeliverySessions(sched.Now())
			live := 0
			for _, h := range handles {
				if _, ok := nd.DeliverySessionForTest(h); ok {
					live++
				}
			}
			want := 0
			if tc.wantLive {
				want = 3
			}
			if live != want {
				t.Fatalf("%d of 3 sessions alive after a forward step of %v on a %v window (guaranteed survival %v), want %d",
					live, tc.step, idle, c2GuaranteedSurvival(idle), want)
			}
			t.Logf("G-C2-16 step=%-12v window=%v guaranteed=%v → %d of 3 live", tc.step, idle, c2GuaranteedSurvival(idle), live)
		})
	}
}

// G-C2-17 — the step reaps a session that settled ONE SECOND before it. This is the
// sentence the disclosure actually rests on: a jump does not spare the busiest session,
// so the exposure is real, and it is a LATENCY loss (the remainder is booked as a
// deposit returned at anchor expiry), never a byte loss.
func TestC2ForwardStepDoesNotSpareTheBusiestSession(t *testing.T) {
	const E = 1
	const idle = c2CandidateMargin
	nd, keyE, _, ledger, sched := c2Server(t, idle, E)
	fID := identity.FromSeed(7930)
	ledger.Register(fID.NodeID())
	obj := ports.HashBytes([]byte("c2-step"))
	sess, err := nd.OpenDeliverySession(fID.NodeID(), demand.SignSessionOpen(fID.Signer(), nd.id, []demand.Token{mintDemandTokenUnder(t, keyE, E)}))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	settle := func(count uint64) {
		t.Helper()
		if _, err := nd.SettleDeliveryReceipt(fID.NodeID(), demand.AckSession(fID.Signer(), sess.handle, sess.commitment, obj, nd.id, count)); err != nil {
			t.Fatalf("settle %d: %v", count, err)
		}
	}
	settle(1)
	// Keep it busy for a while, then settle ONE SECOND before the step: as recently
	// settled as a session realistically gets.
	for i := uint64(2); i <= 6; i++ {
		sched.RunUntil(sched.Now().Add(30 * ports.Second))
		settle(i)
	}
	sched.RunUntil(sched.Now().Add(ports.Second))
	settle(7)
	nd.sweepDeliverySessions(sched.Now())
	if _, ok := nd.DeliverySessionForTest(sess.handle); !ok {
		t.Fatal("fixture: reaped while settling every 30 s — the sweep is not keyed on the last settlement")
	}
	// THE STEP: the clock jumps a whole window forward with no settlement in between.
	sched.RunUntil(sched.Now().Add(idle))
	nd.sweepDeliverySessions(sched.Now())
	if _, ok := nd.DeliverySessionForTest(sess.handle); ok {
		t.Fatal("the session survived a forward step of the whole window one second after settling — R-SESSION-WALLCLOCK-STEP's surviving claim does not hold on the shipped reaper")
	}
	st := ledger.DeliverySettlementStats()
	if st.BurnedCredits != 0 || st.PendingRefundCredits <= 0 {
		t.Fatalf("after the step: pending %d / burned %d — the step must cost LATENCY (a deposit returned at anchor expiry), never bytes",
			st.PendingRefundCredits, st.BurnedCredits)
	}
	t.Logf("G-C2-17 RESULT: a session that settled 1 s before a %v forward step is reaped; %d credits booked as a DEPOSIT, %d burned", idle, st.PendingRefundCredits, st.BurnedCredits)
}

// G-C2-18 (core half) — DeliveryIdleWindow reports the window the REAPER runs on, not a
// remembered input. The daemon announces this value, so it is the tie between the window
// the floor check judged and the window actually installed: cmd/silt reads it back out
// (`installedIdle := time.Duration(nd.DeliveryIdleWindow())`) and prints it, and e2e
// TestDeliveryIdleWindowDefaultBootsThePaidLane asserts the printed figure.
//
// Three polarities: off reports zero, an install reports itself, and the number reported
// is the one the reap boundary is computed from (a session settled on a bucket boundary
// lives one nanosecond short of it and dies at it).
func TestC2DeliveryIdleWindowReportsWhatTheReaperRunsOn(t *testing.T) {
	const E = 1
	sched := simclock.New()
	net := simnet.New(sched, 2, simnet.DefaultConfig())
	ident := identity.FromSeed(7940)
	off := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	if got := off.DeliveryIdleWindow(); got != 0 {
		t.Fatalf("a node with the delivery lane OFF reports an idle window of %v, want 0 — a daemon would announce a window it is not running", got)
	}

	for _, w := range []ports.Duration{c2CandidateTight, c2CandidateMargin, 90 * ports.Second} {
		nd, keyE, _, ledger, sched := c2Server(t, w, E)
		if got := nd.DeliveryIdleWindow(); got != w {
			t.Fatalf("EnableDeliverySessions(%v) then DeliveryIdleWindow() = %v — the announced window would not be the installed one", w, got)
		}
		// And the reported number IS the reap boundary: at stamp phase 0 the stamp is the
		// settlement itself, so the session dies at exactly the reported window.
		reported := nd.DeliveryIdleWindow()
		h, at := c2OpenAndSettleAt(t, nd, keyE, ledger, sched, E, 7941+int64(w%83), 0)
		if !c2AliveAfter(nd, sched, h, at, reported-1) {
			t.Fatalf("window %v: reaped one nanosecond before the REPORTED window elapsed — the reaper is not running on the number the daemon announces", w)
		}
		if c2AliveAfter(nd, sched, h, at, reported) {
			t.Fatalf("window %v: still alive AT the reported window — the reaper is not running on the number the daemon announces", w)
		}
	}
}

// G-C2-20 — the twin of cmd/silt's deliveryIdleFieldStall pin. The 1040 s margin datum
// lives as a literal in both packages (cmd/silt cannot import core/node's test constants
// and core/node cannot import package main), so each side pins its own copy to the
// evidence and names the other. Zeroing either is RED here or there.
func TestC2FieldStallDatumMatchesTheEvidence(t *testing.T) {
	if c2FieldStallObserved != 1040*ports.Second {
		t.Fatalf("c2FieldStallObserved = %v, want 1040 s = 17m20s — block 43 committed 17 min 20 s after block 42 on run c450985-deep "+
			"(evidence: integration/cloudtest/h43-stall-evidence-c450985-deep/README.md). cmd/silt's deliveryIdleFieldStall is the same figure and is pinned there",
			c2FieldStallObserved)
	}
	// The datum's job, driven rather than asserted: the tight candidate is reaped by a gap
	// this long and the shipped one is not. That is the whole reason 24m was chosen.
	if c2GuaranteedSurvival(c2CandidateTight) >= c2FieldStallObserved {
		t.Fatalf("the 10m candidate guarantees %v, which no longer falls short of the %v field stall — the choice of 24m over 10m rests on this comparison",
			c2GuaranteedSurvival(c2CandidateTight), c2FieldStallObserved)
	}
	if c2GuaranteedSurvival(c2CandidateMargin) < c2FieldStallObserved {
		t.Fatalf("the shipped 24m window guarantees %v, under the %v field stall", c2GuaranteedSurvival(c2CandidateMargin), c2FieldStallObserved)
	}
}
