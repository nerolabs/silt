package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #560 — the a434494-deep h91 stall, made a TIMED oracle.
//
// FIELD OBSERVATION: after val-d's OOM→genesis-fallback (#558) removed one
// 64 MiB frozen seat, the remaining 11 of 12 (~260 of 324 MiB, comfortably
// above the >⅔ bar) did not commit h91 for 25+ minutes. Rounds climbed
// r1→r4+ purely by timeout; round-changes from all live members flowed but
// SMEARED (r1=33 recorded, r2–r4 only ~5–6 each — under the per-round
// catch-up threshold); no captured node ever fired a new-view proposal.
// Confounded in the field by #561 (walk-delayed sweeps ran the ladder ~2×
// slow) and by the capture window ending mid-ladder.
//
// THE ORACLE (post-#561, tick-cadence escapes): a 12-member mature epoch
// with ONE heavy seat ABSENT (killed — dials fail, nothing is ever heard
// from it again), pending work armed, per-node sweep phases staggered across
// the interval (the field's independent 30 s timers), REAL timed delivery on
// the sim clock. The certified #451 claim: the increasing round durations +
// weight catch-up converge the live weight onto a common round and the
// height COMMITS within a bounded ladder. Bound: the ladder through round 5
// (Σ sweepsForRound(0..4) = 30 sweeps = 900 s) with 2× margin — generous
// against sweep phase + request-timeout noise, tight against the field's
// 25-minute non-commit.
//
// RED here = the field mechanism is in the machinery (one-shot round-change
// delivery never assembles a quorum at any designee, absent-seat rounds
// wasted, …) → the deterministic home for a #560 fix (consensus-adjacent →
// research-gated, build-immutable #6). GREEN = the machinery converges once
// escapes run at tick cadence (#561) — decisive evidence the field stall's
// driver was the walk-starved ladder plus the truncated observation window,
// recorded on #560 (the "GREEN is evidence too" pattern, as with #549's
// scatter oracle).
func TestModelCheck_560_AbsentHeavySeatMustStillCommitInBound(t *testing.T) {
	nodes, ids, net, sched, refill := matureWorld12(t)
	all := make([]ports.NodeID, len(ids))
	byID := map[ports.NodeID]*Node{}
	for i := range ids {
		all[i] = ids[i].NodeID()
		byID[all[i]] = nodes[i]
	}
	for i, nd := range nodes {
		if !nd.chain.EverMature() {
			t.Fatalf("premise: node %d not latched", i)
		}
	}
	var honestSlashed bool
	for _, nd := range nodes {
		nd.OnSlash(func(ports.NodeID, uint64) { honestSlashed = true })
	}

	_, contested := nodes[0].chain.Head()

	// The absent seat: a HEAVY member (the field's val-d was a 64 MiB anchor).
	// Prefer the contested height's r0 designee when it is heavy — the worst
	// case (the first round is guaranteed wasted); otherwise a maturer. The
	// live remainder holds > ⅔ of the frozen weight either way (the field's
	// 260/324 shape) — this oracle asks about CONVERGENCE, not the floor.
	absent := nodes[4] // maturer-1 (64 MiB)
	if d := byID[nodes[0].designatedProposer(contested, 0)]; d != nil {
		for i := 0; i < 8; i++ { // heavy seats are the first 8
			if nodes[i] == d {
				absent = d
				break
			}
		}
	}
	net.Kill(absent.id)
	t.Logf("#560: contested h%d, absent heavy seat %s (r0 designee absent: %v)",
		contested, absent.id, byID[nodes[0].designatedProposer(contested, 0)] == absent)

	// Arm pending work on the LIVE members (the field: entries + renewal regs)
	// and hand the network to the clock: timed delivery from here on.
	refill()
	net.DisableHeldDelivery()

	// Independent sweep phases across the interval — the field's per-node 30 s
	// timers re-randomized by restarts. Deterministic stagger, full spread.
	interval := DefaultConfig().ChainSyncInterval
	live := make([]*Node, 0, 11)
	for _, nd := range nodes {
		if nd == absent {
			continue
		}
		nd := nd
		live = append(live, nd)
		phase := ports.Duration(len(live)-1) * (interval / 11)
		sched.AfterFunc(phase, func() { nd.StartChainSync(nd.chainSyncSeed, nil) })
	}

	committed := func() bool {
		n := 0
		for _, nd := range live {
			if _, h := nd.chain.Head(); h > contested {
				n++
			}
		}
		return n > 0
	}

	// The bound: the #451 ladder through round 5 (30 sweeps = 900 s) with 2×
	// margin for sweep phase + dead-peer request timeouts on the walk.
	deadline := sched.Now().Add(2 * 30 * interval)
	steps := 0
	for sched.Now() < deadline && !committed() {
		if !sched.Step() {
			t.Fatalf("#560: scheduler drained at %v with no commit — the machinery went quiescent with pending work armed", sched.Now())
		}
		steps++
		if steps%64 == 0 {
			refill() // the field's renewal treadmill keeps queues non-empty
		}
	}

	if honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed under the absent-seat run")
	}
	if !committed() {
		rounds := map[uint64]int{}
		for _, nd := range live {
			rounds[nd.roundsFor().Round]++
		}
		t.Fatalf("#560 REPRODUCED IN-PROCESS: with one heavy seat absent and tick-cadence escapes (#561), the live >⅔-weight cohort did not commit h%d within the %v ladder bound (round histogram %v at deadline). This is the deterministic home for the h91 non-commit — a round-change assembly/delivery fix is consensus-adjacent and research-gated (#6).",
			contested, 2*30*interval, rounds)
	}
	// GREEN: name what it proves — and what it does not.
	var blk []ports.Hash
	_ = blk
	r := uint64(0)
	for _, nd := range live {
		if b := nd.Chain().Blocks(contested); len(b) > 0 {
			r = b[0].CommitRound
			break
		}
	}
	t.Logf("#560: h%d committed at round %d in %v virtual with the heavy seat absent — the machinery converges at tick cadence; the field's 25-minute non-commit is attributed to the #561 walk-starved ladder (~2× slow climbs observed) + the truncated capture window, not a convergence defect. Recorded on #560.", contested, r, sched.Now())
}
