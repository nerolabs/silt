package node

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// The #451 repro — the clean re-run ce15a80-89365's 10a stall, made
// deterministic: a 12-member governed mature epoch (4 BONDED anchors + 4
// maturers + 4 minimum-bond sybils — the field topology exactly), a
// SYBIL-authored drain proposal locked at (h, r0), the whole sybil cohort
// then PARTITIONED (the drill's stop), and the live 8 members — holding
// 512/516 MiB, >99% of the frozen weight — sweep the round machinery. The
// field observation: the carried lock was re-proposed forced=true at rounds
// 4..15 and never completed; no commit for 370+ s. This schedule asks the
// same question in-process: does the height commit within a bounded number
// of sweeps once the author cohort is silent?
//
// The oracle property (what a healthy escape must satisfy): the contested
// height commits within the sweep budget, and no honest validator is
// slashed. If this test is RED, it is the deterministic face of #451 and the
// research consult's evidence; the certified machinery under test is the
// carried-lock path (dead-author prepare lift, lock-QC re-verification at
// high rounds, round-change quorum composition with a third of the epoch
// silent) — NO unilateral fix; parked on its branch per the wedge-oracle
// pattern.
func matureWorld12(t *testing.T) (nodes []*Node, ids []*identity.Identity, net *simnet.Network, sched *simclock.Scheduler, refill func()) {
	t.Helper()
	const nAnchors, nMaturers, nSybils = 4, 4, 4
	total := nAnchors + nMaturers + nSybils
	sched = simclock.New()
	net = simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids = make([]*identity.Identity, total)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(8800 + i))
		if i < nAnchors {
			anchors[ids[i].NodeID()] = true
		}
	}
	g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("g12")}}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors,
		MatureValidators: 2, OperatorMargin: 1, EpochBlocks: 8}

	nodes = make([]*Node, total)
	for i, id := range ids {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, id.Signer())
		if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
			t.Fatalf("sign-mark store: %v", err)
		}
		nodes[i] = nd
	}
	all := make([]ports.NodeID, total)
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	for _, nd := range nodes {
		seed := make([]ports.NodeID, 0, total-1)
		for _, id := range all {
			if id != nd.id {
				seed = append(seed, id)
			}
		}
		nd.chainSyncSeed = seed
	}

	// Regs for ALL 12 (the field: anchors bond too). Anchors size 64M no
	// domain; maturers 64M distinct domains; sybils 1M no domain.
	mk := func(i int, size int64, domain uint64) chain.BondReg {
		sgn := ids[i].Signer()
		pub := append([]byte(nil), sgn.Public().(ed25519.PublicKey)...)
		return chain.NewBondReg(sgn, ports.HashBytes(pub), size, []byte("stub"), g.Hash(), domain)
	}
	regs := make([]chain.BondReg, 0, total)
	for i := 0; i < nAnchors; i++ {
		regs = append(regs, mk(i, 64<<20, 0))
	}
	for i := 0; i < nMaturers; i++ {
		regs = append(regs, mk(nAnchors+i, 64<<20, uint64(i+1)))
	}
	for i := 0; i < nSybils; i++ {
		regs = append(regs, mk(nAnchors+nMaturers+i, 1<<20, 0))
	}

	anchorIDs := all[:nAnchors]
	drive := func(proposer int, h uint64, tag string, att []ports.NodeID) {
		prev, next := nodes[proposer].chain.Head()
		if next != h {
			t.Fatalf("drive %s: head next=%d want %d", tag, next, h)
		}
		b := &chain.Block{Version: chain.BlockVersion, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry(tag)}}
		var done bool
		var perr error
		nodes[proposer].proposeBlock(b, att, all, 0, func(err error) { done, perr = true, err })
		drainHeld(t, net, fifo)
		if !done || perr != nil {
			t.Fatalf("drive %s (h%d): done=%v err=%v", tag, h, done, perr)
		}
	}
	// h1 (anchor-0 drains everyone else's regs — the fold skips its own id)
	// then h2 (anchor-1 drains anchor-0's), then attested heights to the h8
	// boundary so all 12 qualify and the maturer snapshot governs.
	for _, r := range regs {
		if r.ValidatorID() != all[0] {
			nodes[0].queuePendingBondReg(r)
		}
	}
	drive(0, 1, "w12-a", anchorIDs[1:])
	// Non-anchors FIRST from h2 on: the gather early-stops at quorum, and an
	// anchors-first order would leave every maturer/sybil out of the committed
	// Atts — unqualified for C2Metric, latch unreachable (the matureWorld
	// build's catch 1, again).
	nonAnchorsFirst := append(append([]ports.NodeID{}, all[nAnchors:]...), anchorIDs[1:]...)
	nodes[1].queuePendingBondReg(regs[0])
	drive(1, 2, "w12-b", append(append([]ports.NodeID{}, all[nAnchors:]...), all[0], all[2], all[3]))
	for h := uint64(3); h <= 8; h++ {
		drive(0, h, "w12-"+string(rune('b'+h)), nonAnchorsFirst)
	}

	reg13 := identity.FromSeed(8990)
	reg13Pub := append([]byte(nil), reg13.Signer().Public().(ed25519.PublicKey)...)
	head, _ := nodes[0].chain.Head()
	pend := chain.NewBondReg(reg13.Signer(), ports.HashBytes(reg13Pub), 2<<20, []byte("stub"), head, 0)
	refill = func() {
		for _, nd := range nodes {
			nd.pendingBondRegs = []pendingBondReg{{R: pend}}
		}
	}
	refill()
	return nodes, ids, net, sched, refill
}

func TestModelCheck_451_SilentAuthorLockedValueMustStillCommit(t *testing.T) {
	nodes, ids, net, _, refill := matureWorld12(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
	}
	sybilSet := map[ports.NodeID]bool{}
	for _, id := range all[8:] {
		sybilSet[id] = true
	}

	// Premise: a 12-member governed epoch, latch tripped everywhere.
	for i, nd := range nodes {
		if !nd.chain.EverMature() {
			t.Fatalf("premise: node %d not latched", i)
		}
	}
	if got := len(nodes[0].chain.EligibleProposers()); got != 12 {
		t.Fatalf("premise: want a 12-member epoch, got %d eligible proposers", got)
	}

	var honestSlashed bool
	for i, nd := range nodes {
		if i < 8 {
			nd.OnSlash(func(culprit ports.NodeID, _ uint64) {
				if !sybilSet[culprit] {
					honestSlashed = true
				}
			})
		}
	}

	// Drive to a height whose (h, r0) designee is a SYBIL — the drill's
	// timing: the author about to go silent owns the working height.
	live := nodes[:8]
	var sybAuthor *Node
	for hops := 0; hops < 13; hops++ {
		_, h := nodes[0].chain.Head()
		d := byID[nodes[0].designatedProposer(h, 0)]
		if sybilSet[d.id] {
			sybAuthor = d
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
	if sybAuthor == nil {
		t.Fatal("setup: no sybil designee height reached within 13 hops")
	}
	_, contested := nodes[0].chain.Head()

	// The sybil author proposes its drain at (contested, r0); its PRECOMMIT
	// replies are held so the round cannot complete — but the prepare-QC went
	// out, so the live members LOCK on the sybil-authored value (the drill's
	// exact freeze-frame: locked, then the author cohort stops).
	refill()
	holdAuthorPrecommits := func(m simnet.HeldMsg) bool {
		return m.Kind == ports.MsgPrecommitReply && m.To == sybAuthor.id
	}
	sybAuthor.maybeProposeBondDrain()
	drainHeldExcept(t, net, holdAuthorPrecommits)
	lockedOn := 0
	var lockHash ports.Hash
	for _, nd := range live {
		if rs := nd.roundsFor(); rs.Lock != nil {
			lockedOn++
			lockHash = rs.Lock.Hash
		}
	}
	if lockedOn == 0 {
		t.Fatal("setup: no live member locked on the sybil-authored value — the freeze-frame premise did not form")
	}

	// THE DRILL'S STOP: partition the whole sybil cohort. Every live member
	// drops traffic to/from all four sybils; the held precommits die with it.
	for _, nd := range live {
		nd.SetBlockedPeers(all[8:])
	}

	// The live 8 sweep — refilled pending work arms the escape each round.
	// The field burned 14 rounds in 370s and never committed; give the
	// machinery SIX full sweep cycles (≥3 escape rounds) — a healthy escape
	// needs one.
	liveIDs := all[:8]
	for cycle := 0; cycle < 6; cycle++ {
		refill()
		sweepRounds(t, net, func(m simnet.HeldMsg) bool { return false }, live)
		for _, nd := range live {
			nd.SyncChain(liveIDs, func(int, error) {})
		}
		drainHeld(t, net, fifo)
		if _, h := nodes[0].chain.Head(); h > contested {
			break
		}
	}

	if honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed during the silent-author escape")
	}
	_, h := nodes[0].chain.Head()
	if h <= contested {
		rs := nodes[0].roundsFor()
		t.Fatalf("#451 REPRODUCED: the contested height %d never committed across 6 sweep cycles with 8/12 members live holding >99%% of the weight — the carried sybil-authored lock (hash %x..., locked by %d live members) could not be completed by any forced re-proposal (node0 now at round %d) — the field's 370s/14-round stall, deterministic",
			contested, lockHash[:4], lockedOn, rs.Round)
	}
	// GREEN would mean the fixture cannot reproduce the field mechanism at
	// this shape — itself decisive for the consult (the trigger is elsewhere).
	blk := nodes[0].Chain().Blocks(contested)
	t.Logf("escape completed: h%d committed at round %d (lock carried: %v)", contested, blk[0].CommitRound, blk[0].Hash() == lockHash)
}

// TestModelCheck_451_StaggeredSweepsMustStillConverge is THE #451 oracle — the
// certification's merge gate and the model-check method fix made concrete:
// per-node round-advance SKEW as a first-class adversarial schedule dimension.
// The lockstep freeze-frame above is GREEN because everyone sweeps together;
// the field's 14-round/370s stall came from independent 30s timers with
// arbitrary phase smearing the live members across rounds so that no round
// ever held a quorum of round-changes SIMULTANEOUSLY. This schedule drives
// exactly that smear: after the author cohort goes silent, the live members
// sweep one at a time in rotation — each advancing its own round on its own
// "timer" while the others stand still — the deterministic worst case of
// timer skew.
//
// FAILING-FIRST: RED against locking-without-a-synchronizer (fixed
// roundAdvanceSweeps, no catch-up — the members smear and never converge);
// GREEN with the certified synchronizer (increasing round duration makes
// rounds eventually outlast the smear; f+1/weight catch-up collapses it at
// message speed).
func TestModelCheck_451_StaggeredSweepsMustStillConverge(t *testing.T) {
	nodes, ids, net, sched, refill := matureWorld12(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
	}
	sybilSet := map[ports.NodeID]bool{}
	for _, id := range all[8:] {
		sybilSet[id] = true
	}
	var honestSlashed bool
	for i, nd := range nodes {
		if i < 8 {
			nd.OnSlash(func(culprit ports.NodeID, _ uint64) {
				if !sybilSet[culprit] {
					honestSlashed = true
				}
			})
		}
	}

	// Same freeze-frame as the lockstep test: a sybil-authored value locked at
	// (h, r0), then the whole author cohort partitioned.
	live := nodes[:8]
	var sybAuthor *Node
	for hops := 0; hops < 13; hops++ {
		_, h := nodes[0].chain.Head()
		d := byID[nodes[0].designatedProposer(h, 0)]
		if sybilSet[d.id] {
			sybAuthor = d
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
	if sybAuthor == nil {
		t.Fatal("setup: no sybil designee height reached")
	}
	_, contested := nodes[0].chain.Head()
	refill()
	holdAuthorPrecommits := func(m simnet.HeldMsg) bool {
		return m.Kind == ports.MsgPrecommitReply && m.To == sybAuthor.id
	}
	sybAuthor.maybeProposeBondDrain()
	drainHeldExcept(t, net, holdAuthorPrecommits)
	for _, nd := range live {
		nd.SetBlockedPeers(all[8:])
	}
	liveIDs := all[:8]

	// THE SCHEDULE (the field's asynchrony, deterministic). Phase 1 — pre-GST
	// chaos: skewed sweeps with every new-view prepare HELD and delivered
	// LATE (stale-by-arrival, the WAN reality): each delivery still resolves
	// its gather (refusals/partial accepts — no driver-artifact hangs) while
	// scattering the (h, r, prepare) sign marks across rounds, and the skewed
	// timers smear the members' round clocks apart. Phase 2 — GST: delivery
	// stabilizes (lockstep sweeps, everything delivered promptly). THE ORACLE
	// asserts the CERTIFIED BOUND: after GST, the locked value commits within
	// gstBudget rotations. RED without a synchronizer — the smeared members
	// must climb the round ladder one fixed-duration timeout at a time until
	// some round clears every scattered mark (the field's 14-round/370s
	// stall); GREEN with it — catch-up collapses the smear at message speed
	// and the increasing duration holds rounds open for assembly.
	const chaosSteps = 8
	const gstBudget = 2
	for step := 0; step < chaosSteps; step++ {
		target := live[step%len(live)]
		holdNonTarget := func(m simnet.HeldMsg) bool {
			return m.Kind == ports.MsgProposeBlock && m.To != target.id
		}
		for _, nd := range live {
			refill()
			nd.maybeProposeBondDrain()
			drainHeldExcept(t, net, holdNonTarget)
			nd.maybeAdvanceRound()
			drainHeldExcept(t, net, holdNonTarget)
		}
		// The non-target prepares are LOST ON THE WIRE (pre-GST loss) and the
		// requesters' per-attempt timeout timers fire (the driver advances the
		// sim clock timer-by-timer) — each stranded ask errors through its
		// retries and the gather resolves weight-short (only the step's target
		// could sign), scattering the marks one target at a time. This is the
		// field's loss+timeout dynamics, deterministic — no hung designees.
		for i := 0; i < 400; i++ {
			for _, m := range net.Pending() {
				if m.Kind == ports.MsgProposeBlock && m.To != target.id {
					net.DropPending(m.ID)
				}
			}
			drainHeldExcept(t, net, holdNonTarget)
			if !sched.Step() {
				break
			}
		}
		drainHeldExcept(t, net, holdNonTarget)
		for _, m := range net.Pending() {
			if m.Kind == ports.MsgProposeBlock && m.To != target.id {
				net.DropPending(m.ID)
			}
		}
		drainHeld(t, net, fifo)
	}
	preGSTCommitted := false
	if _, h := nodes[0].chain.Head(); h > contested {
		preGSTCommitted = true // the synchronized system may converge even mid-chaos
	}

	// GST: the network stabilizes. A synchronized escape converges within the
	// certified bound; a synchronizer-less one pays the whole smear climb.
	committed := preGSTCommitted
	for rotation := 0; rotation < gstBudget && !committed; rotation++ {
		refill()
		sweepRounds(t, net, func(m simnet.HeldMsg) bool { return false }, live)
		for _, nd := range live {
			nd.SyncChain(liveIDs, func(int, error) {})
		}
		drainHeld(t, net, fifo)
		if _, h := nodes[0].chain.Head(); h > contested {
			committed = true
			t.Logf("converged within the post-GST budget (rotation %d)", rotation)
		}
	}

	if honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed under the staggered-sweep schedule")
	}
	if !committed {
		maxRound := uint64(0)
		for _, nd := range live {
			if rs := nd.roundsFor(); rs.Round > maxRound {
				maxRound = rs.Round
			}
		}
		t.Fatalf("#451 REPRODUCED: after GST the contested height %d did not commit within %d rotations — the pre-GST smear left the members at rounds/marks up to %d and a synchronizer-less escape must climb the whole ladder one fixed timeout at a time (certification 451-view-synchronization: locking without a SYNCHRONIZER)", contested, gstBudget, maxRound)
	}
}

// TestModelCheck_456_DeadPeersMustNotTaxTheGather is the #456 merge gate — the
// model-check's first COARSELY-TIMED oracle (certification §5: the untimed sim
// models order and Byzantine behavior but charges NOTHING for an unreachable
// peer, so it was structurally blind to bounded liveness — the 5th recurrence
// of the blind spot). Here the sim CLOCK is the cost model: the sybil cohort
// is made UNRESPONSIVE (receiver-side drop — a request is delivered nowhere
// and only the requester's RequestTimeout timer, fired by the driver via
// simclock.Step, resolves it), so every ask to a dead member costs real
// simulated time. The oracle drives the 10a shape (a live designee proposing
// with a third of the epoch silent) and asserts the commit lands within a
// bounded SIMULATED elapsed time.
//
// FAILING-FIRST: RED under the sequential ask-chain — each dead peer's full
// timeout sits ON THE CRITICAL PATH before the next attester is even asked
// (elapsed ≈ the SUM of dead-peer timeouts per phase, twice); GREEN under the
// certified concurrent gather — all asks in flight at once, collection
// completes on the quorum predicate at the speed of the slowest NEEDED (live)
// replier, and the dead peers' timers fire harmlessly off the critical path.
func TestModelCheck_456_DeadPeersMustNotTaxTheGather(t *testing.T) {
	nodes, ids, net, sched, refill := matureWorld12(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
	}
	sybilSet := map[ports.NodeID]bool{}
	for _, id := range all[8:] {
		sybilSet[id] = true
	}
	live := nodes[:8]
	liveIDs := all[:8]

	// THE DEAD COHORT: the sybils drop all inbound (receiver-side), so a
	// request to them is swallowed and only the requester's timeout resolves
	// it — the field's stopped-process shape, with the sim clock charging the
	// cost the untimed driver never did.
	for _, nd := range nodes[8:] {
		nd.SetBlockedPeers(liveIDs)
	}

	// Find a height whose (h, r0) designee is LIVE (the drill's honest-side
	// proposal — the tax is paid soliciting the dead, not being dead). Heights
	// whose designee is dead are driven past by a DIRECT live proposal (valid:
	// the designee rule is drain-path etiquette, not a validity rule), keeping
	// the contested height's marks clean — the oracle measures the GATHER's
	// cost, not setup residue.
	var designee *Node
	liveAtt := append([]ports.NodeID{}, liveIDs[1:]...)
	for hops := 0; hops < 13 && designee == nil; hops++ {
		prev, h := nodes[0].chain.Head()
		d := byID[nodes[0].designatedProposer(h, 0)]
		if !sybilSet[d.id] {
			designee = d
			break
		}
		blk := &chain.Block{Version: chain.BlockVersion, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry("hop-" + string(rune('a'+hops)))}}
		var hopDone bool
		nodes[0].proposeBlock(blk, liveAtt, all, 0, func(err error) { hopDone = err == nil })
		drainHeld(t, net, fifo)
		if !hopDone {
			t.Fatalf("setup hop h%d: live direct proposal failed", h)
		}
	}
	if designee == nil {
		t.Fatal("setup: no live designee height reached")
	}
	_, contested := nodes[0].chain.Head()
	t0 := sched.Now()

	// Drive the live designee's proposal to commit, firing timers as the only
	// clock: elapsed simulated time IS the gather's critical-path cost.
	refill()
	prevC, hC := designee.chain.Head()
	bC := &chain.Block{Version: chain.BlockVersion, Height: hC, Prev: prevC, Entries: []ports.Entry{mkEntry("contested-456")}}
	// The ask ORDER is the oracle's adversarial dimension: the DEAD members
	// first — the order the deterministic ID-sort can and does produce in the
	// field (whose IDs sort where is seed luck). A sequential gather pays every
	// dead timeout before reaching a live attester; a concurrent gather is
	// order-indifferent.
	attC := make([]ports.NodeID, 0, 11)
	attC = append(attC, all[8:]...) // the 4 dead sybils, first
	for _, id := range liveIDs {
		if id != designee.id {
			attC = append(attC, id)
		}
	}
	designee.proposeBlock(bC, attC, all, 0, func(err error) { t.Logf("contested proposal done: err=%v", err) })
	committed := false
	for i := 0; i < 2000 && !committed; i++ {
		drainHeld(t, net, fifo)
		if _, h := nodes[0].chain.Head(); h > contested {
			committed = true
			break
		}
		if !sched.Step() {
			for _, nd := range live {
				nd.maybeProposeBondDrain()
				nd.maybeAdvanceRound()
			}
			drainHeld(t, net, fifo)
			if _, h := nodes[0].chain.Head(); h > contested {
				committed = true
			}
			if !committed && sched.Pending() == 0 {
				t.Fatal("setup: the world quiesced without committing — no timers, no messages, no progress")
			}
		}
	}
	if !committed {
		t.Fatal("setup: the contested height never committed at all")
	}
	elapsed := ports.Duration(sched.Now() - t0)
	// The bound: ONE per-ask timeout unit of tolerance (the concurrent gather
	// pays at most ~one timeout where dead timers overlap the live collection)
	// plus scheduling slack. The sequential chain pays ≈ 4 dead × timeout per
	// PHASE, twice — several times this bound.
	timeout := nodes[0].cfg.RequestTimeout
	bound := 3 * timeout
	if elapsed > bound {
		t.Fatalf("#456 REPRODUCED (coarsely-timed): the live designee's commit took %v of simulated time with 4 dead members — the SEQUENTIAL gather put every dead peer's timeout on the critical path (bound: %v = 3× the per-ask timeout; the concurrent gather pays ~one)", elapsed, bound)
	}
	t.Logf("gather completed in %v simulated (bound %v)", elapsed, bound)
}
