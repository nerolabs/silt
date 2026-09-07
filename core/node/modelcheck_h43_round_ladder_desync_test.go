package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestModelCheck_H43_RoundLadderDesyncMustStillConverge — the deterministic
// home for run c450985-deep's height-43 stall (evidence:
// integration/cloudtest/h43-stall-evidence-c450985-deep/README.md + logs).
//
// FIELD OBSERVATION: with one of four validators (val-d) STOPPED at h43, the
// remaining live members' round ladders ran ~77s apart (val-a: r1/r2/r3/r4 at
// 08:45:10 / 08:46:40 / 08:49:10 / 08:53:10; val-b: r1/r2/r3 at 08:46:27 /
// 08:47:57 / 08:50:27 — each RECORDING the others' round-change broadcasts for
// higher rounds well before its OWN local timer reached them), no round ever
// gathered a new-view proposal that committed, and val-d returned after 9 min
// (08:54:22) and started its own local ladder at round 1 — cold, oblivious to
// how far the live members had already climbed. Height 43 committed at
// 09:01:41, 17m20s after the previous commit; prior deep runs cleared the same
// f=1 drill within 650s. Register row R-H43-ROUND-LADDER-DESYNC.
//
// THIS IS THE SAME FIXTURE FAMILY as the #451/#560 oracles
// (matureWorld12 — 4 bonded anchors + 4 maturers + 4 minimum-bond sybils, the
// field's 12-seat rotation exactly) and the #560 absent-heavy-seat schedule
// (kill one heavy seat, stagger each live member's own chain-sync tick phase
// across the interval, real timed delivery on the sim clock — no more
// lockstep sweeps). NEW ingredient #560 did not drive: the absent seat
// RETURNS mid-ladder (net.Restart + its own StartChainSync, matching the
// field's val-d rejoining cold at round 0) instead of staying dead for the
// whole run.
//
// THE ORACLE PROPERTY (I4 liveness, the #451/#560 certified claim restated
// for a returning member): once ≥ 2f+1 of the four validators are live (three
// of four throughout this schedule — the fourth's absence is bounded, not
// permanent), the contested height commits within a bounded model time of the
// absent seat's return. Bound (Tester's choice, cited): the #451/#560
// certified ladder through round 5 (Σ sweepsForRound(0..4) = 30 sweeps = 900s
// of ChainSyncInterval=30s) with a 2x margin, run from the RETURN instant —
// the same formula TestModelCheck_560_AbsentHeavySeatMustStillCommitInBound
// uses for a seat that never returns, since the return should only ADD
// weight, never subtract a convergence guarantee already proven for a
// permanently-absent seat.
//
// RED here = the field mechanism (a live member's own local round-change
// timer can run for many multiples of the certified bound while it keeps
// RECORDING higher-round broadcasts from its peers without adopting them) is
// in the machinery — the deterministic home for a fix, consensus-adjacent and
// research-gated (build-immutable #6). GREEN = the machinery converges at
// tick cadence once the fourth seat returns — decisive evidence the field
// stall's driver is elsewhere (a Tester-named residual, not a guess).
func TestModelCheck_H43_RoundLadderDesyncMustStillConverge(t *testing.T) {
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

	// THE ABSENT SEAT: one of the FOUR validators (the field's val-d) —
	// preferring the contested height's r0 designee when it is one of the
	// four (the field's worst case: the first round is guaranteed wasted).
	absent := nodes[3]
	if d := byID[nodes[0].designatedProposer(contested, 0)]; d != nil {
		for i := 0; i < 4; i++ {
			if nodes[i] == d {
				absent = d
				break
			}
		}
	}
	net.Kill(absent.id)
	t.Logf("H43: contested h%d, absent validator %s (r0 designee absent: %v)",
		contested, absent.id, byID[nodes[0].designatedProposer(contested, 0)] == absent)

	refill()
	net.DisableHeldDelivery()

	// SKEW: per-node round timers started at DIFFERENT instants, spread of
	// the order of ONE round timeout (the field's ~77s ladder gap; the base
	// round duration is roundAdvanceSweeps(2) x ChainSyncInterval(30s) = 60s
	// — sweepsForRound(1) is 90s). skewSpread=90s spread across the 11 live
	// members is the grid's baseline; varied below if this run is GREEN.
	interval := DefaultConfig().ChainSyncInterval
	const skewSpread = 90 * ports.Second
	live := make([]*Node, 0, 11)
	for _, nd := range nodes {
		if nd == absent {
			continue
		}
		nd := nd
		idx := len(live)
		live = append(live, nd)
		phase := ports.Duration(idx) * (skewSpread / 11)
		sched.AfterFunc(phase, func() { nd.StartChainSync(nd.chainSyncSeed, nil) })
	}

	// THE RETURN: the field's val-d came back after 9 minutes and started its
	// own ladder cold at round 0 — the same net.Restart + StartChainSync path
	// a real daemon restart takes, at a time this node's own heightRounds for
	// the contested height has never been touched.
	const downDuration = 9 * 60 * ports.Second
	sched.AfterFunc(downDuration, func() {
		net.Restart(absent.id)
		absent.StartChainSync(absent.chainSyncSeed, nil)
	})

	committed := func() bool {
		for _, nd := range nodes {
			if _, h := nd.chain.Head(); h > contested {
				return true
			}
		}
		return false
	}

	// bound: the #560 certified ladder budget (2 x 30 sweeps x ChainSyncInterval),
	// run from the RETURN instant.
	bound := downDuration + 2*30*interval
	deadline := sched.Now().Add(bound)
	steps := 0
	for sched.Now() < deadline && !committed() {
		if !sched.Step() {
			t.Fatalf("H43: scheduler drained at %v with no commit — the machinery went quiescent with pending work armed", sched.Now())
		}
		steps++
		if steps%64 == 0 {
			refill() // the field's renewal/entry treadmill keeps queues non-empty
		}
	}

	if honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed under the h43 round-ladder-desync schedule")
	}
	if !committed() {
		rounds := map[uint64]int{}
		sweeps := map[uint64]int{}
		for _, nd := range nodes {
			rs := nd.roundsFor()
			rounds[rs.Round]++
			sweeps[rs.Round] += rs.Sweeps
		}
		t.Fatalf("H43 REPRODUCED IN-PROCESS: with one of four validators stopped for %v then restarted cold at round 0, height %d did not commit within %v of the return (deadline %v; round histogram %v; sweeps-in-round %v) — the field's 17m20s non-commit, deterministic. Consensus-adjacent, research-gated (build-immutable #6).",
			downDuration, contested, bound, deadline, rounds, sweeps)
	}
	var commitRound uint64
	for _, nd := range nodes {
		if b := nd.Chain().Blocks(contested); len(b) > 0 {
			commitRound = b[0].CommitRound
			break
		}
	}
	t.Logf("H43: h%d committed at round %d, %v (virtual) after the return with skewSpread=%v, downDuration=%v — the machinery converges under this shape at this skew/return-instant.",
		contested, commitRound, sched.Now(), skewSpread, downDuration)
}
