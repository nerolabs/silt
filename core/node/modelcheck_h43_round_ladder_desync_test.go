package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// TestModelCheck_H43_RoundLadderDesyncMustStillConverge — the deterministic
// home for run c450985-deep's height-43 stall (evidence:
// integration/cloudtest/h43-stall-evidence-c450985-deep/README.md + logs).
// Register row R-H43-ROUND-LADDER-DESYNC; freeze-manifest item 15.
//
// FIELD OBSERVATION: with one validator STOPPED at h43, the live members' round
// ladders ran ~77s apart (val-a: r1/r2/r3/r4 at 08:45:10 / 08:46:40 / 08:49:10 /
// 08:53:10; val-b: r1/r2/r3 at 08:46:27 / 08:47:57 / 08:50:27 — each RECORDING
// the others' round-change broadcasts for higher rounds well before its OWN
// local timer reached them), no round ever gathered a new-view proposal that
// committed, and val-d returned after 9 min and started its own ladder cold at
// round 0. Height 43 committed 17m20s after the previous commit; prior deep
// runs cleared the same f=1 drill within 650s.
//
// THE MECHANISM, in one sentence: maybeAdvanceRound armed the round clock on
// LOCAL mempool content alone (pendingBondRegs / pendingEntries /
// bondDrainInFlight) and ZEROED rs.Sweeps on every momentarily-empty sweep, so
// a validator holding no work of its own never laddered — the round number was
// a function of unreplicated private state, so with work held by a minority the
// live members smeared across rounds, no round ever held a round-change quorum,
// and the height fell back to the #338 staggered-takeover rank walk. #772 /
// D-CONSENSUS-ARMING (A) repairs it with the REPLICATED arming term rs.Armed
// (set by storeRoundChange, so one member's first round-change arms every
// recipient within one hop) plus M1b (hold the counter when disarmed, never
// zero it).
//
// THE DEFECT IS A BOUND, NOT A DEADLOCK — this is what the previous version of
// this test got wrong twice over. The field's h43 committed; it took 17m20s
// against a 650s baseline. In-process the pre-fix tree also commits, via the
// rank walk, whose own certified backstop is (N+2)·ChainSyncInterval — the
// D3 comment in rounds.go says so in as many words: "bounded at
// (N+2)·ChainSyncInterval, the certified backstop, but not at the published
// ≤ f+1 rounds". So an oracle that only asks "does it ever commit" is GREEN on
// both trees. Measured: the previous version was GREEN at 8b467e1 (the whole
// fix absent), GREEN at c4da469, and GREEN under a direct ablation of the
// arming rule at the IDENTICAL virtual time — the rule never bound on its
// schedule, because it drove the fixture's refill(), which installs pending
// work on EVERY node. Uniform work satisfies the pre-fix local-only guard at
// every seat at once, so the arming term is never consulted and the ladders
// cannot desynchronize.
//
// THE SCHEDULE. Every step is forced; nothing is sampled.
//
//  1. matureWorld12 — 4 bonded anchors + 4 maturers + 4 minimum-bond sybils,
//     the field's 12-seat rotation, latched into a governed mature epoch.
//  2. Drive the head until the contested height's r0 designee is an ANCHOR.
//     designatedProposer is (height+round) % |EligibleProposers|, so the hop
//     loop is arithmetic, not a search.
//  3. FREEZE-FRAME (the #451 shape): that designee proposes at (contested, r0)
//     with its PRECOMMIT replies held, so every live member locks on the value
//     and burns its own (contested, r0, prepare) sign slot. Round 0 is then
//     dead for the whole network — maybeProposeBondDrain's slotCompare gate
//     refuses a fresh r0 proposal at every seat — which puts the round ladder
//     on the critical path.
//  4. The designee is KILLED: the field's one-down fault. It stays down for the
//     whole budget. That is STRICTLY STRONGER than the field's 9-minute absence
//     (a return only ADDS weight, never subtracts a convergence guarantee), and
//     it keeps the schedule free of an ingredient that would only ever run on
//     the failing path.
//  5. NON-UNIFORM PENDING WORK — the ingredient the previous version lacked and
//     the field's actual condition (three of thirteen seats held entries). Every
//     queue is cleared and the work is re-installed on a two-anchor MINORITY,
//     asserted to sit strictly below chain.RoundCatchupMet so the responsive
//     f+1/weight catch-up cannot rescue the workless majority either. The
//     discrimination then rests on the arming rule alone.
//  6. Delivery is driver-controlled and ONE MESSAGE AT A TIME in FIFO order,
//     with the oracle sampled after every single delivery. The driver clock
//     never advances, so no request timeout fires. Nothing here can turn on
//     probe or message ordering, and the sampler cannot miss the instant a
//     round-change quorum forms — the commit that follows replaces
//     heightRounds, and a coarser sampler reads only the reset state.
//
// THE ORACLE — two arms, because this is a LIVENESS property and a liveness
// property is green under a defect whose whole symptom is that nothing
// happens:
//
//   - BOUND: the contested height commits within the PUBLISHED liveness claim
//     for #772 — "≤ f′+1 rounds after GST", 190s at f = 1, N = 12 with
//     ChainSyncInterval = 30s (ROADMAP Lane A, item A1). One seat is down, so
//     f′ = 1 and the ladder must reach round f′+1 within sweepsForRound(0) +
//     sweepsForRound(1) = 2 + 3 = 5 sweeps = 150s, plus one interval of
//     cross-node skew = 180s, which is that published 190s. The budget below is
//     that number. It is the published claim, NOT a threshold fitted to the
//     measured gap.
//   - MECHANISM: the ROUND LADDER carried the commit — some live member
//     accumulated a round-change set for a round ≥ 1 that meets
//     chain.SupportMeetsQuorum, the same predicate newViewFor applies. This is
//     the control arm. Without it the test could go green on the rank-walk
//     backstop if the backstop ever got faster, and the committed block cannot
//     supply the evidence: a rank-walk proposer stamps CommitRound from its own
//     ladder, so BOTH trees commit this height at CommitRound 1.
//
// MEASURED (each run confirmed by "=== RUN", never by exit code alone):
//
//	c4da469 (fix present)             EXIT 0 — commits in 2 cycles, quorum at r1
//	8b467e1 (the whole fix absent)    EXIT 1 — no quorum at any round, 9 of 11
//	                                  seats pinned at r0 while 2 climb to r2
//	c4da469 + arm-A ablation          EXIT 1 — same signature
//
// Do NOT patch from a RED here. The stall is consensus-adjacent: research-gated
// and owner-ratified (build-immutable #6).
func TestModelCheck_H43_RoundLadderDesyncMustStillConverge(t *testing.T) {
	nodes, ids, net, _, refill := matureWorld12(t)
	all := make([]ports.NodeID, len(ids))
	byID := map[ports.NodeID]*Node{}
	anchor := map[ports.NodeID]bool{}
	for i := range ids {
		all[i] = ids[i].NodeID()
		byID[all[i]] = nodes[i]
		if i < 4 {
			anchor[all[i]] = true
		}
	}
	for i, nd := range nodes {
		if !nd.chain.EverMature() {
			t.Fatalf("premise: node %d not latched", i)
		}
	}
	if got := len(nodes[0].chain.EligibleProposers()); got != 12 {
		t.Fatalf("premise: want a 12-member epoch, got %d eligible proposers", got)
	}
	var honestSlashed bool
	for _, nd := range nodes {
		nd.OnSlash(func(ports.NodeID, uint64) { honestSlashed = true })
	}

	// (2) THE SEAT THAT GOES DOWN OWNS ROUND 0 — forced, not preferred.
	var absent *Node
	for hops := 0; hops < 13; hops++ {
		_, h := nodes[0].chain.Head()
		d := byID[nodes[0].designatedProposer(h, 0)]
		if anchor[d.id] {
			absent = d
			break
		}
		refill()
		d.maybeProposeBondDrain()
		drainHeld(t, net, fifo)
		for _, nd := range nodes {
			nd.SyncChain(all, func(int, error) {})
		}
		drainHeld(t, net, fifo)
	}
	if absent == nil {
		t.Fatal("setup: no anchor-designee height reached within 13 hops")
	}
	_, contested := nodes[0].chain.Head()

	// (3) FREEZE-FRAME: lock the live members at (contested, r0) and burn their
	// r0 prepare slots, so round 0 cannot commit for anyone.
	refill()
	holdAuthorPrecommits := func(m simnet.HeldMsg) bool {
		return m.Kind == ports.MsgPrecommitReply && m.To == absent.id
	}
	absent.maybeProposeBondDrain()
	drainHeldExcept(t, net, holdAuthorPrecommits)
	locked := 0
	for _, nd := range nodes {
		if nd == absent {
			continue
		}
		if rs := nd.roundsFor(); rs.Height == contested && rs.Lock != nil {
			locked++
		}
	}
	if locked == 0 {
		t.Fatalf("setup: no live member locked at (h%d, r0) — the freeze-frame premise did not form, so round 0 is not dead and this schedule would not exercise the ladder", contested)
	}

	// (4) THE ONE-DOWN FAULT. The held precommits die with the endpoint.
	net.Kill(absent.id)

	// (5) NON-UNIFORM PENDING WORK.
	refill()
	pend := append([]pendingBondReg(nil), nodes[0].pendingBondRegs...)
	if len(pend) == 0 {
		t.Fatal("setup: the fixture's refill() left node 0 with no pending registration to redistribute")
	}
	live := make([]*Node, 0, len(nodes)-1)
	for _, nd := range nodes {
		nd.pendingBondRegs = nil
		nd.pendingEntries = nil
		if nd != absent {
			live = append(live, nd)
		}
	}
	holders := make([]*Node, 0, 2)
	holderIDs := map[ports.NodeID]bool{}
	for _, nd := range live {
		if len(holders) == 2 {
			break
		}
		if anchor[nd.id] {
			holders = append(holders, nd)
			holderIDs[nd.id] = true
		}
	}
	if len(holders) != 2 {
		t.Fatalf("setup: want 2 live anchor work-holders, got %d", len(holders))
	}
	// The premise the whole discrimination rests on, asserted against the live
	// chain rather than assumed from the fixture's weights.
	if nodes[0].chain.RoundCatchupMet(holderIDs) {
		t.Fatalf("setup: the 2-seat work-holding cohort MEETS chain.RoundCatchupMet — the workless majority would be rescued by the responsive catch-up path, so this schedule no longer isolates the arming rule. Re-check matureWorld12's epoch weights")
	}
	t.Logf("H43: contested h%d, r0 designee %s down for the whole budget, %d live members locked at r0, pending work on %d of %d live seats (below chain.RoundCatchupMet)",
		contested, absent.id, locked, len(holders), len(live))

	// (6) THE ORACLE'S MECHANISM ARM, sampled after every single delivery.
	maxChanges := map[uint64]int{}
	var quorumRound uint64
	quorumSeen := false
	observe := func() {
		for _, nd := range live {
			rs := nd.rounds // the FIELD, never roundsFor(): reading must not reset it
			if rs == nil || rs.Height != contested {
				continue
			}
			for r, m := range rs.Changes {
				if r == 0 {
					continue
				}
				if len(m) > maxChanges[r] {
					maxChanges[r] = len(m)
				}
				if quorumSeen {
					continue
				}
				senders := make([]ports.NodeID, 0, len(m))
				for id := range m {
					senders = append(senders, id)
				}
				if nd.chain.SupportMeetsQuorum(nd.designatedProposer(contested, r), senders, contested) {
					quorumSeen, quorumRound = true, r
				}
			}
		}
	}
	// deliverAll drains the network ONE message at a time in FIFO order,
	// sampling between every delivery. This is drainHeld(t, net, fifo) with the
	// oracle interleaved — the ordering is fixed, so the verdict cannot depend
	// on it.
	deliverAll := func() {
		const bound = 20000
		steps := 0
		for {
			p := net.Pending()
			if len(p) == 0 {
				return
			}
			if steps++; steps > bound {
				t.Fatalf("held delivery did not quiesce within %d deliveries (livelock?)", bound)
			}
			net.Deliver(p[0].ID)
			observe()
		}
	}

	// THE BUDGET: the published bound, derived above. sweepsForRound(0) +
	// sweepsForRound(1) is the certified ladder to round f′+1 at f′ = 1; the
	// +1 is the one interval of cross-node skew the published 190s carries
	// over the 150s ladder.
	budget := sweepsForRound(0) + sweepsForRound(1) + 1
	liveIDs := make([]ports.NodeID, 0, len(live))
	for _, nd := range live {
		liveIDs = append(liveIDs, nd.id)
	}
	committed := func() bool {
		_, h := nodes[0].chain.Head()
		return h > contested
	}
	ladderAt := make([]map[uint64]int, 0, budget)
	cycles := 0
	for ; cycles < budget && !committed(); cycles++ {
		for _, nd := range holders { // the renewal treadmill runs ONLY on the work-holders
			nd.pendingBondRegs = append([]pendingBondReg(nil), pend...)
		}
		// One chain-sync tick per live seat: the production sweep body, drain
		// then round-advance, exactly as sweepRounds runs it.
		for _, nd := range live {
			nd.maybeProposeBondDrain()
			deliverAll()
		}
		for _, nd := range live {
			nd.maybeAdvanceRound()
			deliverAll()
		}
		for _, nd := range live {
			nd.SyncChain(liveIDs, func(int, error) {})
			deliverAll()
		}
		ladder := map[uint64]int{}
		for _, nd := range live {
			if rs := nd.rounds; rs != nil && rs.Height == contested {
				ladder[rs.Round]++
			}
		}
		ladderAt = append(ladderAt, ladder)
	}

	if honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed under the h43 round-ladder-desync schedule")
	}
	if !committed() {
		t.Fatalf("H43 REPRODUCED IN-PROCESS — BOUND ARM: with the r0 designee down, round 0 burnt at every seat, and pending work on %d of %d live members, height %d did NOT commit within the published budget of %d sweeps (≤ f′+1 rounds, 190s at f = 1, N = 12). Round histogram per sweep: %v; largest round-change set seen per round: %v. The ladders desynchronized exactly as run c450985-deep recorded — the work-holders climbed while the workless majority sat at r0, so no round ever held a round-change quorum. Consensus-adjacent and research-gated (build-immutable #6): do NOT patch from here",
			len(holders), len(live), contested, budget, ladderAt, maxChanges)
	}
	if !quorumSeen {
		t.Fatalf("H43 REPRODUCED IN-PROCESS — MECHANISM ARM: height %d committed within the budget, but NO live member ever held a round-change set meeting chain.SupportMeetsQuorum at any round ≥ 1 (largest set seen per round: %v). The round ladder did not carry this commit; the #338 staggered-takeover rank walk did, and its bound is (N+2)·ChainSyncInterval, not the published ≤ f′+1 rounds. Consensus-adjacent and research-gated (build-immutable #6): do NOT patch from here",
			contested, maxChanges)
	}
	blk := nodes[0].Chain().Blocks(contested)
	if len(blk) == 0 {
		t.Fatalf("H43: head moved past h%d but node 0 holds no block there", contested)
	}
	if blk[0].CommitRound == 0 {
		t.Fatalf("ANTI-VACUITY: h%d committed at round 0, but the freeze-frame burnt every live member's r0 prepare slot — round 0 was supposed to be dead, so this run did not exercise the round ladder and its GREEN means nothing. Re-check the freeze-frame", contested)
	}
	t.Logf("H43: h%d committed at round %d in %d of %d budgeted sweeps; the round-change quorum formed at round %d (largest set per round: %v) — one member's round-change armed the rest and the ladder carried the commit",
		contested, blk[0].CommitRound, cycles, budget, quorumRound, maxChanges)
}
