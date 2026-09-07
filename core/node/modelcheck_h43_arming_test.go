package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestModelCheck_H43_HeterogeneousArmingMustCommitWithinFPlus1Rounds is
// G-H43-1, decisive, per
// silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md
// §4.2: "matureWorld12 variant with heterogeneous arming: exactly 3 of 12
// seats hold pending work, one heavy seat killed, staggered sweep phases,
// timed delivery on the sim clock. The height commits within
// Σ_{r≤f} sweepsForRound(r)·ChainSyncInterval + skew + G."
//
// THE ROOT (M1, certified): core/node/rounds.go:306-310 arms the round clock
// on LOCAL mempool content — `maybeAdvanceRound` quiesces
// (`rs.Sweeps = 0`) whenever a node's OWN pendingBondRegs/pendingEntries/
// bondDrainInFlight are empty, so a seat that never holds work never runs
// the pacemaker at all. Every EXISTING oracle in this family
// (`TestModelCheck_451_*`, `TestModelCheck_560_*`) calls the shared
// `refill()` from `matureWorld12`, which sets `pendingBondRegs` on EVERY
// node (`modelcheck_451_locked_stall_test.go:143-146`) — so none of them can
// ever exercise this branch. This test is the first in the family that
// does not.
//
// THE SCHEDULE: nodes[0..3] are the four heavy (64 MiB) anchors — the
// field's val-a..d. nodes[0,1,2] are ARMED (continuously re-fed a pending
// bond reg, matching the field's val-a/b/c holding entries+regs);
// nodes[3] is KILLED at the contested height's start (the field's val-d,
// stopped) and RESTARTED at downDuration=540s (~9 min, the field's val-d
// return) — the coordinator's requested "down then back mid-ladder" shape,
// layered onto the cert's "one heavy seat killed". nodes[4..11] (4
// maturers + 4 sybils) are left QUIESCENT — pendingBondRegs/pendingEntries
// forced empty and never touched — the 8 seats the field's rotation held
// silent throughout (`sybil-3.log`/`maturer-2.log`: "recording ... and
// never advancing").
//
// THE BOUND (Q4b, certified): Σ_{r≤f} sweepsForRound(r)·ChainSyncInterval
// + skew + G, at f=1 (one heavy seat down, 2f+1=3 of 4 anchors live):
// (sweepsForRound(0)+sweepsForRound(1))·30s + 30s(skew, the #549 derived
// cross-node bound) + 10s(G, the #555-measured intrinsic gather latency at
// 12-seat WAN) = 5·30 + 30 + 10 = 190s. Measured from the KILL instant (the
// field's own accounting: "996s after the kill" — Q4b/§2), not from the
// return.
//
// RED at HEAD: the 8 quiescent seats never run maybeAdvanceRound's
// non-quiescent branch at all (M1), so the network's pacemaker runs on 3 of
// 12 seats — well under any weight/count quorum threshold the certificate
// path requires — and no round converges within the bound. GREEN only
// after fix direction (A) (arm on a replicated condition, hold Sweeps
// rather than zero it on the disarmed branch) ships.
func TestModelCheck_H43_HeterogeneousArmingMustCommitWithinFPlus1Rounds(t *testing.T) {
	nodes, ids, net, sched, refill := matureWorld12(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
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

	// ARM exactly nodes[0,1,2] (the field's val-a/b/c); KILL nodes[3] (val-d);
	// force nodes[4..11] QUIESCENT — the heterogeneous-arming premise G-H43-6
	// checks the harness CAN express.
	armed := []int{0, 1, 2}
	killedIdx := 3
	arm := func() {
		refill() // seeds pendingBondRegs on EVERY node (the shared, uniform helper)
		armedSet := map[int]bool{}
		for _, i := range armed {
			armedSet[i] = true
		}
		for i, nd := range nodes {
			if i == killedIdx || armedSet[i] {
				continue
			}
			// QUIESCENT: strip what refill() just seeded — this seat never
			// holds pending work, matching the field's 8 silent rotation seats.
			nd.pendingBondRegs = nil
			nd.pendingEntries = nil
		}
	}
	arm()
	for i, nd := range nodes {
		isArmed := i == 0 || i == 1 || i == 2
		isKilled := i == killedIdx
		hasWork := len(nd.pendingBondRegs) > 0 || len(nd.pendingEntries) > 0
		if isArmed && !hasWork {
			t.Fatalf("premise: armed seat %d has no pending work after arm()", i)
		}
		if !isArmed && !isKilled && hasWork {
			t.Fatalf("premise: quiescent seat %d has pending work after arm() — heterogeneous arming not established", i)
		}
	}

	net.Kill(nodes[killedIdx].id)
	net.DisableHeldDelivery()

	// Staggered sweep phases across the interval (the field's independent
	// 30s timers, re-randomized by restart — the #560 convention).
	interval := DefaultConfig().ChainSyncInterval
	live := make([]*Node, 0, 11)
	for i, nd := range nodes {
		if i == killedIdx {
			continue
		}
		nd := nd
		idx := len(live)
		live = append(live, nd)
		phase := ports.Duration(idx) * (interval / 11)
		sched.AfterFunc(phase, func() { nd.StartChainSync(nd.chainSyncSeed, nil) })
	}

	// The field's val-d: down then back mid-ladder (coordinator's requested
	// shape, layered onto the cert's "one heavy seat killed"). 190s < 540s,
	// so the bound below is decided before this fires in the RED run; it
	// exists in the schedule so a future GREEN run (and any resolution of
	// R-H43-RECOVERY-UNATTRIBUTED, §3.2/§9 of the certification) also covers
	// the return dynamic, not just the permanently-down case.
	const downDuration = 9 * 60 * ports.Second
	sched.AfterFunc(downDuration, func() {
		net.Restart(nodes[killedIdx].id)
		nodes[killedIdx].StartChainSync(nodes[killedIdx].chainSyncSeed, nil)
	})

	committed := func() bool {
		for _, nd := range nodes {
			if _, h := nd.chain.Head(); h > contested {
				return true
			}
		}
		return false
	}

	// THE BOUND, at f=1 (Q4b): Σ_{r≤f} sweepsForRound(r)·ChainSyncInterval + skew + G.
	const f = uint64(1)
	var sweepSum uint64
	for r := uint64(0); r <= f; r++ {
		sweepSum += uint64(sweepsForRound(r))
	}
	const skew = 30 * ports.Second   // the #549-derived cross-node skew bound (< ChainSyncInterval)
	const gatherG = 10 * ports.Second // the #555-measured intrinsic gather latency at 12-seat WAN
	bound := ports.Duration(sweepSum)*interval + skew + gatherG
	deadline := sched.Now().Add(bound)

	steps := 0
	for sched.Now() < deadline && !committed() {
		if !sched.Step() {
			break // scheduler drained before the bound — report state below, do not loop forever
		}
		steps++
		if steps%64 == 0 {
			arm() // keep the 3 armed seats' queues topped up; re-strip the quiescent 8
		}
	}

	if honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed under the heterogeneous-arming schedule")
	}
	if !committed() {
		rounds := map[int]uint64{}
		for i, nd := range nodes {
			rounds[i] = nd.roundsFor().Round
		}
		t.Fatalf("G-H43-1 REPRODUCED: with 3 of 12 seats armed (nodes 0,1,2), 8 quiescent (nodes 4-11), "+
			"one heavy seat killed (node 3, restart scheduled at %v uncalled by the bound), height %d "+
			"did not commit within the certified f=1 bound %v of the kill (deadline %v; per-seat rounds "+
			"reached: %v) — the round clock is armed on unreplicated local mempool state (rounds.go:306-310), "+
			"so 8 of 12 seats never ran the pacemaker (M1, certified). Consensus-adjacent, research-gated "+
			"(build-immutable #6); fix direction (A)/(B)/(C) in the certification.",
			downDuration, contested, bound, deadline, rounds)
	}
	var commitRound uint64
	for _, nd := range nodes {
		if b := nd.Chain().Blocks(contested); len(b) > 0 {
			commitRound = b[0].CommitRound
			break
		}
	}
	t.Logf("G-H43-1: h%d committed at round %d, %v (virtual) — within the f=1 bound %v of the kill. The heterogeneous-arming defect (M1) is fixed on this branch.",
		contested, commitRound, sched.Now(), bound)
}

// TestModelCheck_H43_AtLeastOneRoundLivenessOracleArmsNonUniformly is
// G-H43-6 (method), the fixture-blindness pin the certification calls the
// SIXTH recurrence of one blind spot (docs/design/consensus-model-check.md
// amendment log records five: #357, the I5 benign-call, #441 entry-liveness,
// #451 round-advance skew, #456 dead-peer cost).
//
// THE CLAIM UNDER TEST: "at least one round-liveness oracle in this family
// runs with a NON-UNIFORM arming distribution" — i.e. the harness CAN
// express a quiescent seat (a node with permanently empty
// pendingBondRegs/pendingEntries while others hold work), and at least one
// committed oracle exercises that state, rather than every oracle routing
// through the shared `refill()` (`modelcheck_451_locked_stall_test.go:143-146`)
// which arms EVERY node uniformly.
//
// TWO HALVES, both runtime-observed (not source-text inspection — this is
// not a SOURCE GATE per build-process.md rule 8, since it reads live Node
// state, not .go text):
//  1. CAPABILITY: matureWorld12 itself does not force uniform arming — a
//     caller can leave a node's queues empty. Proven by direct field
//     inspection immediately after fixture construction.
//  2. TEETH: the shared `refill()` helper — the one every pre-existing
//     oracle in the family calls — is confirmed, by running it and reading
//     the resulting state, to arm ALL nodes uniformly. This is the exact
//     trap the certification names: a test that reached for `refill()`
//     alone could never have produced heterogeneous arming, so a REGRESSION
//     that silently reverted G-H43-1's fixture to plain `refill()` and
//     nothing else would leave the family with ZERO non-uniform oracles —
//     which is what this pin exists to catch. It does not (and cannot, at
//     the runtime level) certify that G-H43-1 itself does not regress; that
//     is G-H43-1's own premise check
//     (TestModelCheck_H43_HeterogeneousArmingMustCommitWithinFPlus1Rounds's
//     "heterogeneous arming not established" fatal, immediately above it).
func TestModelCheck_H43_AtLeastOneRoundLivenessOracleArmsNonUniformly(t *testing.T) {
	nodes, _, _, _, refill := matureWorld12(t)

	// HALF 1 — capability, with a finding: matureWorld12 ITSELF calls the
	// shared refill() as its own last setup step
	// (modelcheck_451_locked_stall_test.go:147) — so every node ALREADY
	// holds uniform pending work the instant the fixture returns, before any
	// oracle gets a chance to choose otherwise. This is a THIRD layer of the
	// same blind spot the certification names at the caller level (every
	// oracle's OWN refill() loop) — it is baked into the constructor too.
	// Confirm that fact first (evidence, not an assumption), then confirm
	// the harness still PERMITS reaching a non-uniform state by clearing.
	uniformAtReturn := true
	for _, nd := range nodes {
		if len(nd.pendingBondRegs) == 0 && len(nd.pendingEntries) == 0 {
			uniformAtReturn = false
		}
	}
	if !uniformAtReturn {
		t.Fatal("G-H43-6 finding check: matureWorld12 no longer arms every node uniformly at construction — " +
			"re-verify the THIRD-layer uniform-arming claim (constructor-level) before relying on it")
	}
	t.Log("G-H43-6 finding: matureWorld12's OWN constructor arms all 12 nodes uniformly before returning " +
		"(its internal refill() call, modelcheck_451_locked_stall_test.go:147) — a third layer of the same " +
		"blind spot, beneath the caller-level refill() loop the certification names.")

	// CAPABILITY: clear every queue, then arm a strict subset directly — no
	// fixture machinery prevents a non-uniform state; the DEFAULT is uniform,
	// but the STATE SPACE is not restricted to it.
	for _, nd := range nodes {
		nd.pendingBondRegs = nil
		nd.pendingEntries = nil
	}
	nodes[0].pendingBondRegs = []pendingBondReg{{}}
	for i, nd := range nodes {
		hasWork := len(nd.pendingBondRegs) != 0 || len(nd.pendingEntries) != 0
		if i == 0 && !hasWork {
			t.Fatal("G-H43-6 capability check: setting pendingBondRegs on node 0 directly did not take")
		}
		if i != 0 && hasWork {
			t.Fatalf("G-H43-6 capability check: arming node 0 alone leaked pending work onto node %d — "+
				"the fixture does not permit non-uniform arming", i)
		}
	}
	nodes[0].pendingBondRegs = nil // reset before the teeth half

	// HALF 2 — teeth: the shared `refill()` every PRE-EXISTING oracle in this
	// family (#451, #560) calls arms EVERY node — confirmed by reading the
	// live state after calling it, not by trusting the docstring.
	refill()
	uniform := true
	for i, nd := range nodes {
		if len(nd.pendingBondRegs) == 0 && len(nd.pendingEntries) == 0 {
			uniform = false
			t.Logf("teeth check: node %d was NOT armed by refill() — the shared helper is no longer uniform; "+
				"re-verify which pre-existing oracle this changes before treating G-H43-6 as satisfied by them", i)
		}
	}
	if !uniform {
		t.Fatal("G-H43-6 teeth check FAILED: the shared refill() helper no longer arms every node uniformly — " +
			"the premise this pin's SIXTH-recurrence claim rests on (every pre-existing oracle in the family " +
			"arms uniformly via refill()) no longer holds; re-attribute before relying on this pin")
	}
	t.Logf("G-H43-6: capability confirmed (matureWorld12 permits a strict-subset arm), teeth confirmed "+
		"(the shared refill() helper arms all %d nodes uniformly — the exact trap the certification names). "+
		"TestModelCheck_H43_HeterogeneousArmingMustCommitWithinFPlus1Rounds is, as of this commit, the first "+
		"oracle in the family that does not rely on refill() alone.", len(nodes))
}
