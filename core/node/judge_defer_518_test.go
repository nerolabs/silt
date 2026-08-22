package node

// #518 regression (the transient-deny sub-mode): a claim that is unjudgeable
// RIGHT NOW — the judge's survivor fetches come up short, which in the field
// happens moments after the repair-time fetch storm — must be DEFERRED and
// re-judged, not denied forever. Claim emission is one-shot, so before this a
// 30s transient silently cost the bounty for good (captured: "survivors
// fetched=2..5 of k=10 needed", 4ms after the judge's own rebuild).

import (
	"testing"

	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

func TestJudgeDefersTransientlyUnjudgeableClaim(t *testing.T) {
	s := newRepairAdv(t, 46)
	s.fundEscrow(5_000_000)
	judge := s.careJudge()
	claimant := s.nodes[2]

	pos, parityID, leafIdx := s.parityTarget()
	holder := s.nodes[9]
	s.stageShardOn(judge, holder, parityID, pos, leafIdx)

	// The judge is manifest-warm (a caretaker's steady state — Care's warm
	// start): fetch the manifest chunks BEFORE the survivor holders go down,
	// so the claim reaches the SURVIVOR leg — the leg the captured transient
	// hits — rather than dying on the manifest fetch.
	entry, _, _ := s.reg.Lookup(bg(), s.root)
	warmed := false
	judge.fetchAll(entry.ManifestChunks, func(missing []ports.ChunkID) { warmed = len(missing) == 0 })
	s.sched.Run()
	if !warmed {
		t.Fatal("rig: judge manifest warm-up failed")
	}

	// Make every stripe-0 survivor TRANSIENTLY unreachable: kill each node
	// holding any stripe-0 shard other than the claimed one (the judge's own
	// copies are dropped too, so it must fetch), and schedule their revival
	// between the first and second deferred re-judgments. The claim then
	// arrives at a moment when it cannot be judged — the captured field shape.
	stripe0 := map[ports.ChunkID]bool{}
	for i, id := range s.m.ChunkIDs() {
		if i < s.m.K {
			stripe0[id] = true
		}
	}
	for _, id := range s.m.ParityIDs()[:s.m.N-s.m.K] {
		stripe0[id] = true
	}
	delete(stripe0, parityID)
	var downed []ports.NodeID
	for i, nd := range s.nodes {
		if nd == judge || nd == holder || i == 0 {
			continue
		}
		holds := false
		for id := range stripe0 {
			if ok, _ := nd.Store().Has(bg(), id); ok {
				holds = true
				break
			}
		}
		if holds {
			s.net.Kill(nd.ID())
			downed = append(downed, nd.ID())
		}
	}
	for id := range stripe0 {
		judge.dropHosted(id)
	}
	if len(downed) == 0 {
		t.Fatal("rig: no survivor holders found to down")
	}
	// Revive at t+45s: after the first re-judgment (+30s, still down) and
	// before the second (+60s) — the transient clears mid-retry-schedule.
	s.sched.AfterFunc(45*ports.Second, func() {
		for _, id := range downed {
			s.net.Restart(id)
		}
	})

	claim := repairproof.RepairClaim{
		Root: s.root, Stripe: 0, ShardPos: pos, ShardID: parityID, Holder: holder.ID(),
	}
	s.deliverClaim(judge, claimant.ID(), claim)

	if judge.Stats.BountiesReleased != 1 {
		t.Fatalf("#518: a transiently-unjudgeable honest claim was not paid after the transient cleared: BountiesReleased=%d (a terminal deny loses the bounty forever — emission is one-shot)", judge.Stats.BountiesReleased)
	}
	if judge.Stats.FalseRepairSlashes != 0 {
		t.Fatal("#518: the deferred claim was slashed")
	}
}
