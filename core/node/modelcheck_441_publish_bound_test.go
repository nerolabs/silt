package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// The durability-turnover discrimination oracles (PE state-of-HEAD 2026-08-17):
// run 82bcd2b-39478's GAP left mature-regime publish reliability honestly
// unpinned between "#351 discovery" and "a #441 mature-quorum residual" — but
// the run's own captured client error (console-82bcd2b-39478.log:28,
// 'accepted but not committed within 3m0s') places the failure in
// ACCEPT→COMMIT: the token was gathered and the entry entered the mempool, so
// discovery (#351) is refuted for that run. What remained unpinned is HOW an
// accepted entry can miss a 180s poll window on a live matured network. These
// two oracles pin the remaining candidates deterministically (the tier order:
// the model discriminates, the field run confirms):
//
//   A — fold starvation under steady mature contention (the #441-residual
//       candidate): with delivery intact, an accepted entry must ride the
//       VERY NEXT committed block. RED here = a real mempool-fold residual →
//       research consult before any fix (certified #432/#441 machinery).
//   B — delivery fragility (the candidate the sheet never named): SubmitEntry's
//       peer broadcasts are fire-and-forget (entrypool.go — empty reply
//       callback, no retry), and the client submits once then only polls
//       (httpregistry.go publishPollTimeout loop). If the broadcast burst is
//       lost, the entry lives ONLY in the accepting node's mempool and commits
//       when the designee rotation reaches that node. B MEASURES that rotation
//       wait. It is a bounded-liveness measurement, not a defect repro: GREEN
//       means the wait is bounded by ~|eligible| heights — which at the field's
//       per-height escape bound (H_ESCAPE 220s, integration/cloudtest
//       scenarios.sh) is tens of minutes, far beyond ANY client poll window.
//       The consequence is client-side: the certified design's drop-recovery
//       lever ("a dropped submission is re-sent by the client's retry loop")
//       must actually fire INSIDE the poll window — periodic re-submit, a
//       mempool-dedup no-op on the happy path — and the poll bound itself must
//       cover at least one in-spec escape height. Full evidence chain:
//       docs/thinking/2026-08-17-durability-turnover-pin.md.

// mcPublishWorld is the shared premise: the 12-member governed mature epoch
// (matureWorld12 — the field topology), latch tripped, plus the I5 guard.
func mcPublishWorld(t *testing.T) (nodes []*Node, all []ports.NodeID, net *simnet.Network, refill func(), honestSlashed *bool) {
	t.Helper()
	nodes, ids, net, _, refill := matureWorld12(t)
	all = make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	for i, nd := range nodes {
		if !nd.chain.EverMature() {
			t.Fatalf("premise: node %d not latched", i)
		}
	}
	if got := len(nodes[0].chain.EligibleProposers()); got != 12 {
		t.Fatalf("premise: want a 12-member epoch, got %d eligible proposers", got)
	}
	var slashed bool
	for _, nd := range nodes {
		nd.OnSlash(func(ports.NodeID, uint64) { slashed = true })
	}
	return nodes, all, net, refill, &slashed
}

// entryHeight returns the height of the committed block carrying root, or
// (0, false) if no committed block carries it.
func entryHeight(ref *Node, root ports.Hash) (uint64, bool) {
	blocks := ref.Chain().Blocks(0)
	for i := range blocks {
		for _, e := range blocks[i].Entries {
			if e.Root == root {
				return blocks[i].Height, true
			}
		}
	}
	return 0, false
}

// driveHeight runs the PRODUCTION sweep cadence (drain, round-advance, sync)
// on every fixture node until the head advances past h0 — the designee rule,
// the staggered takeover for a nodeless designee (the fixture's reg13), and
// the escape ladder are all the real code path, never a test shortcut. hold
// keeps selected messages undelivered across every drain (oracle B's dropped
// submit burst).
func driveHeight(t *testing.T, nodes []*Node, all []ports.NodeID, net *simnet.Network, hold func(simnet.HeldMsg) bool, refill func(), h0 uint64) {
	t.Helper()
	const maxSweeps = 8 // a nodeless designee resolves via the takeover ladder: 3 idle sweeps + rank distance
	for sweep := 0; sweep < maxSweeps; sweep++ {
		refill()
		sweepRounds(t, net, hold, nodes)
		for _, nd := range nodes {
			nd.SyncChain(all, func(int, error) {})
		}
		drainHeldExcept(t, net, hold)
		if _, h := nodes[0].chain.Head(); h > h0 {
			return
		}
	}
	t.Fatalf("h%d never resolved within %d sweeps — the steady-state premise wants every height to commit (drain work is always pending)", h0, maxSweeps)
}

// Oracle A — the #441-residual discriminator. Steady mature contention
// (renewal queues re-armed every sweep), delivery intact: an entry submitted
// through a bonded epoch member (the field's registry validator, val-a's
// shape) must be folded into the VERY NEXT committed block — three times in a
// row. foldPendingEntries is unconditional in every proposal build
// (chainrole.go), so ANY miss is a real fold/scheduling residual of the
// certified machinery: RED routes to research, never a unilateral fix.
func TestModelCheck_441_MatureSteadyState_AcceptedEntryRidesNextBlock(t *testing.T) {
	nodes, all, net, refill, honestSlashed := mcPublishWorld(t)
	noHold := func(simnet.HeldMsg) bool { return false }
	pub := nodes[0] // anchor-0: bonded, epoch-set member — the registry analog

	for i := 0; i < 3; i++ {
		entry := mkEntry("mature-publish-" + string(rune('a'+i)))
		pub.SubmitEntry(entry, all)
		drainHeldExcept(t, net, noHold) // the submit burst lands in every mempool
		_, h0 := nodes[0].chain.Head()

		driveHeight(t, nodes, all, net, noHold, refill, h0)

		at, ok := entryHeight(nodes[0], entry.Root)
		if !ok {
			t.Fatalf("publish %d: entry not committed after the next height resolved — a mempool-held entry missed an unconditional fold (#441 residual: research-gate before touching the round machinery)", i)
		}
		if at != h0 {
			t.Fatalf("publish %d: entry committed at h%d, not the next block h%d — the fold deferred an accepted entry under steady contention (#441 residual)", i, at, h0)
		}
		// Anti-vacuity (#303): the carrying block must ALSO carry drain work —
		// proof the entry rode a CONTENDED block, not an idle chain.
		for _, b := range nodes[0].Chain().Blocks(at) {
			if b.Height == at && len(b.BondRegs) == 0 {
				t.Fatalf("publish %d: the carrying block h%d has no bond regs — the steady-contention premise did not hold, the oracle proved nothing", i, at)
			}
		}
	}
	if *honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed during steady-state publishing")
	}
}

// Oracle B — the delivery-fragility measurement. The submit burst to the other
// eleven members is DROPPED (held forever — the field's fire-and-forget WAN
// loss shape); the entry lives only in the accepting anchor's own mempool and
// can commit only when the designee rotation (or its takeover ladder) hands
// that anchor a block. The oracle asserts the wait is bounded by the rotation
// length and MEASURES it — the number that sizes any honest client poll
// window: waited heights × the field per-height bound (220s) dwarfs the 180s
// single-shot poll, so the client's periodic re-submit is load-bearing, not
// optional.
func TestModelCheck_441_DroppedSubmitBroadcast_RotationWaitBounded(t *testing.T) {
	nodes, all, net, refill, honestSlashed := mcPublishWorld(t)
	holdSubmits := func(m simnet.HeldMsg) bool { return m.Kind == ports.MsgSubmitEntry }
	pub := nodes[0]

	entry := mkEntry("dropped-burst-publish")
	pub.SubmitEntry(entry, all)
	drainHeldExcept(t, net, holdSubmits) // the 11 peer submits stay lost forever

	// Rotation bound: ≤13 eligible proposers once the fixture's pending reg13
	// bond commits (nodeless — its designee heights resolve via the takeover
	// ladder), so the accepting anchor's turn arrives within 13 heights; 16 is
	// the budget with slack. Beyond it the entry is starved outright.
	const heightBudget = 16
	_, start := nodes[0].chain.Head()
	waited := -1
	for i := 0; i < heightBudget; i++ {
		if _, ok := entryHeight(nodes[0], entry.Root); ok {
			waited = i
			break
		}
		_, h0 := nodes[0].chain.Head()
		driveHeight(t, nodes, all, net, holdSubmits, refill, h0)
	}
	if waited < 0 {
		if _, ok := entryHeight(nodes[0], entry.Root); !ok {
			_, head := nodes[0].chain.Head()
			t.Fatalf("STARVED: the solely-held entry never committed within %d heights (h%d→h%d) — rotation/takeover never reached the accepting node's mempool", heightBudget, start, head)
		}
		waited = heightBudget
	}
	if *honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed during the rotation wait")
	}
	at, _ := entryHeight(nodes[0], entry.Root)
	t.Logf("rotation wait: %d chain heights (submitted before h%d, committed at h%d; %d drive iterations) — field worst case ≈ heights × 220s each (H_ESCAPE bound), vs the client's single-shot 180s poll", at-start, start, at, waited)
}
