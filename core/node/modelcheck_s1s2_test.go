package node

import (
	"crypto/ed25519"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — the #432 MERGE-GATE oracles: schedules S1 and S2 from
// the research certification (432-rounds-locking-liveness, 2026-08-15 §2/§5.4),
// driven over the REAL node loop via held delivery.
//
//   S1 (delayed lower-round quorum, crash-fault only): X gathers a REAL commit
//   quorum at round 0, but the final precommit reply is delayed so only the
//   proposer could ever complete it; everyone else round-changes. The lock rule
//   must carry X forward — the new round must re-commit X, never a fresh value,
//   or the delayed quorum completing later forks the height.
//
//   S2 (equivocate-then-misreport, one Byzantine anchor — silt's f=1 budget):
//   the Byzantine anchor helps X to a real round-0 quorum AND sends a
//   round-change reporting a lock on its own value Y, presenting a forged
//   "prepare-QC" (its own signature). No prepare-QC for Y can exist (a QC is a
//   quorum, certification §4), so the misreport must die at verification and X
//   must still be carried forward.
//
// THE ORACLE PROPERTY (both schedules, the certification's merge gate): after
// the dust settles, every replica that committed the contested height committed
// the SAME block — the proposed X — the height DID commit (liveness), and no
// honest validator was slashed (I5).
//
// FAILING-FIRST (controlled revert, the pattern of the I5/#397 oracle): the
// lock machinery is not test-toggleable, so the RED is proven by a recorded
// controlled revert — disabling (i) the lock carriage in maybeAdvanceRound and
// (ii) the defensive lock rule in the prepare handler makes the view-change
// LOCK-FREE: the new round then commits a FRESH drain block while the delayed
// round-0 quorum completes X at the proposer → conflicting commits at one
// height on different replicas (I1 broken by the liveness escape). The revert
// diff and RED output are recorded in the commit message and in
// docs/thinking/2026-08-16-432-s1-s2-oracles.md.

// s1s2World: tier2AnchorNet plus the wiring the round machinery needs — every
// node can reach every other (chainSyncSeed → syncTargets, used by both the
// round-change broadcast and the new-view proposal) and holds one pending bond
// registration (the "pending work" that arms the sweep-based round advance;
// a real, valid reg for a 5th non-anchor identity, as in the field wedge).
// The returned refill re-arms every node's pending queue — a failed drain
// proposal EMBEDS (and so consumes) the pending regs even when it never
// commits, and in production the registrant's renewal sweep continuously
// resubmits; the oracles call refill before each sweep to model that.
func s1s2World(t *testing.T) ([]*Node, []*identity.Identity, *simnet.Network, *chain.Block, func()) {
	t.Helper()
	nodes, ids, net, g, _ := tier2AnchorNet(t, 4)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	reg5 := identity.FromSeed(8990)
	regPub := append([]byte(nil), reg5.Signer().Public().(ed25519.PublicKey)...)
	reg := chain.NewBondReg(reg5.Signer(), ports.HashBytes(regPub), 2<<20, []byte("stub"), g.Hash(), 0)
	refill := func() {
		for _, nd := range nodes {
			nd.pendingBondRegs = map[ports.NodeID]chain.BondReg{reg.ValidatorID(): reg}
		}
	}
	for _, nd := range nodes {
		seed := make([]ports.NodeID, 0, len(all)-1)
		for _, id := range all {
			if id != nd.id {
				seed = append(seed, id)
			}
		}
		nd.chainSyncSeed = seed
	}
	refill()
	return nodes, ids, net, g, refill
}

// drainHeldExcept delivers parked messages (FIFO) until nothing deliverable
// remains, HOLDING every message the hold predicate matches — those stay
// parked for the driver to release later (the S1/S2 "delayed delivery").
func drainHeldExcept(t *testing.T, net *simnet.Network, hold func(simnet.HeldMsg) bool) int {
	t.Helper()
	const bound = 20000
	steps := 0
	for {
		p := net.Pending()
		next := -1
		for i, m := range p {
			if !hold(m) {
				next = i
				break
			}
		}
		if next < 0 {
			return steps
		}
		if steps++; steps > bound {
			t.Fatalf("held-delivery did not quiesce within %d steps (livelock?)", bound)
		}
		net.Deliver(p[next].ID)
	}
}

// sweepRounds runs the production sweep body (drain, then round-advance) on
// each given node, draining the network between phases — one call ≈ one
// chain-sync tick for the swept nodes (chainSyncTick's timer never fires under
// the frozen driver clock, so the oracle calls the body directly).
func sweepRounds(t *testing.T, net *simnet.Network, hold func(simnet.HeldMsg) bool, nodes []*Node) {
	t.Helper()
	for _, nd := range nodes {
		nd.maybeProposeBondDrain()
		drainHeldExcept(t, net, hold)
	}
	for _, nd := range nodes {
		nd.maybeAdvanceRound()
		drainHeldExcept(t, net, hold)
	}
}

// assertSingleCommit is the shared oracle assertion: every replica committed
// the contested height (liveness — the #432 wedge is the counterexample), all
// committed the SAME hash (I1 across round boundaries), that hash is want, and
// no honest validator was slashed (I5) — honestSlashed is read after the final
// cross-sync sweeps that would fire any equivocation scan.
//
// ANTI-VACUITY (the #303 discipline): the committed block must record
// CommitRound == 1 — proof the VIEW-CHANGE path did the committing. If the
// sweeps had silently done nothing and the released round-0 reply had simply
// completed the original gather, the height would commit at round 0 and this
// fails — the oracle cannot go green without the lock-carrying view-change
// actually running.
func assertSingleCommit(t *testing.T, nodes []*Node, want ports.Hash, honestSlashed *bool) {
	t.Helper()
	for i, nd := range nodes {
		hh, h := nd.Chain().Head()
		if h < 2 {
			t.Fatalf("LIVENESS: node %d never committed the contested height (head=%d) — the round escape did not run", i, h)
		}
		blk := nd.Chain().Blocks(1)
		if len(blk) == 0 || blk[0].Hash() != want {
			t.Fatalf("I1 VIOLATION: node %d committed a different block at the contested height (got %x, want %x; head %x/%d)", i, blk[0].Hash(), want, hh, h)
		}
		if blk[0].CommitRound != 1 {
			t.Fatalf("VACUOUS: node %d holds the contested height committed at round %d, want 1 — the view-change did not do the committing, so this run does not exercise the lock rule", i, blk[0].CommitRound)
		}
	}
	if *honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed during the round escape")
	}
}

// TestModelCheck_S1_DelayedLowerRoundQuorumIsCarriedForward drives schedule S1.
func TestModelCheck_S1_DelayedLowerRoundQuorumIsCarriedForward(t *testing.T) {
	nodes, ids, net, g, refill := s1s2World(t)
	id := func(i int) ports.NodeID { return ids[i].NodeID() }
	all := []ports.NodeID{id(0), id(1), id(2), id(3)}

	var honestSlashed bool
	for _, nd := range nodes {
		nd.OnSlash(func(culprit ports.NodeID, _ uint64) { honestSlashed = true })
	}

	// Round 0: a0 proposes X, gathering a1 and a2 (proposer+2 = the strict
	// anchor majority). HOLD a2's precommit reply — the S1 delay: a real commit
	// quorum exists in flight, but only a0 could ever complete it.
	holdA2Reply := func(m simnet.HeldMsg) bool {
		return m.Kind == ports.MsgPrecommitReply && m.From == id(2) && m.To == id(0)
	}
	blkX := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("X")}}
	var xErr error
	xDone := false
	nodes[0].proposeBlock(blkX, []ports.NodeID{id(1), id(2)}, all, 0, func(err error) { xDone, xErr = true, err })
	drainHeldExcept(t, net, holdA2Reply)
	if xDone {
		t.Fatalf("setup: a0's round-0 gather must be suspended on the held reply (got done, err=%v)", xErr)
	}
	// a0 locked X when it formed the QC; a1 and a2 locked X when they
	// precommitted. Verify the S1 premise: locks exist (this is what the
	// round-change must carry).
	for _, i := range []int{0, 1, 2} {
		if rs := nodes[i].roundsFor(); rs.Lock == nil || rs.Lock.Hash != blkX.Hash() {
			t.Fatalf("setup: node %d must be locked on X at round 0 (S1 premise)", i)
		}
	}

	// Everyone else experiences a stuck height with pending work: two sweeps →
	// round-changes to round 1 carrying the locks → the designated proposer
	// assembles the new-view and MUST re-propose X (the carried lock).
	refill()
	sweepRounds(t, net, holdA2Reply, nodes)
	refill()
	sweepRounds(t, net, holdA2Reply, nodes)

	// Release the delayed round-0 precommit: a0's suspended gather resumes and
	// tries to complete the OLD commit — its own replica must refuse it (the
	// head has moved past the re-committed X) or accept the identical value;
	// either way no second value exists.
	drainHeld(t, net, fifo)

	// Final cross-sync so every replica converges and any cross-fork
	// equivocation scan fires (the I5 face).
	for _, nd := range nodes {
		nd.SyncChain(all, func(int, error) {})
	}
	drainHeld(t, net, fifo)

	assertSingleCommit(t, nodes, blkX.Hash(), &honestSlashed)
}

// TestModelCheck_S2_ForgedLockMisreportCannotForkTheHeight drives schedule S2.
func TestModelCheck_S2_ForgedLockMisreportCannotForkTheHeight(t *testing.T) {
	nodes, ids, net, g, refill := s1s2World(t)
	id := func(i int) ports.NodeID { return ids[i].NodeID() }
	all := []ports.NodeID{id(0), id(1), id(2), id(3)}

	// Cast the roles around the deterministic designated proposer for
	// (h=1, r=1): the Byzantine anchor must not be the one assembling the
	// new-view (it refuses nothing from itself), and a0 is the X proposer.
	desig := nodes[0].designatedProposer(1, 1)
	byz := -1
	for _, i := range []int{1, 2, 3} {
		if id(i) != desig {
			byz = i
			break
		}
	}
	var honest []int
	for _, i := range []int{1, 2, 3} {
		if i != byz {
			honest = append(honest, i)
		}
	}
	hAtt, free := honest[0], honest[1] // hAtt joins X's quorum; free never signs r0

	var honestSlashed bool
	for i, nd := range nodes {
		if i == byz {
			continue
		}
		nd.OnSlash(func(culprit ports.NodeID, _ uint64) {
			if culprit != id(byz) {
				honestSlashed = true
			}
		})
	}

	// Round 0: a0 proposes X gathering hAtt and the Byzantine anchor — which
	// behaves HONESTLY on the wire here (prepares, locks, precommits X); its
	// held precommit reply is the delayed completion of X's real quorum.
	holdByzReply := func(m simnet.HeldMsg) bool {
		return m.Kind == ports.MsgPrecommitReply && m.From == id(byz) && m.To == id(0)
	}
	blkX := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("X")}}
	nodes[0].proposeBlock(blkX, []ports.NodeID{id(hAtt), id(byz)}, all, 0, func(error) {})
	drainHeldExcept(t, net, holdByzReply)

	// THE MISREPORT: the Byzantine anchor crafts value Y and a round-change to
	// r1 claiming a round-0 lock on Y, presenting its own prepare as the "QC".
	// (It equivocated: it locked X through the real gather above AND now
	// reports Y.) No quorum ever prepared Y, so the forged QC cannot verify —
	// the envelope must die at verifyRoundChange on every honest receiver.
	blkY := &chain.Block{Version: chain.BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("Y")}}
	chain.Sign(blkY, ids[byz].Signer())
	forged := roundChangeEnv{
		Height:    1,
		NewRound:  1,
		Sender:    append([]byte(nil), ids[byz].Signer().Public().(ed25519.PublicKey)...),
		LockRound: 0,
		LockQC:    []chain.Attestation{chain.AttestAt(blkY, ids[byz].Signer(), 0, chain.PhasePrepare)},
		LockBlock: chain.Encode(blkY),
	}
	forged.Sig = ed25519.Sign(ids[byz].Signer(), forged.sigBytes())
	forgedRaw, err := cbor.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	probe := net.Endpoint(identity.FromSeed(8991).NodeID())
	for _, i := range []int{0, hAtt, free} {
		probe.Send(id(i), ports.Message{Kind: ports.MsgRoundChange, Data: forgedRaw})
	}
	drainHeldExcept(t, net, holdByzReply)

	// Direct mechanism check: no honest node recorded the forged round-change
	// (verifyRoundChange must have refused the unverifiable lock QC).
	for _, i := range []int{0, hAtt, free} {
		if m := nodes[i].roundsFor().Changes[1]; m[id(byz)] != nil {
			t.Fatalf("S2 MECHANISM: node %d recorded the Byzantine round-change carrying a forged Y-lock — verifyRoundChange must refuse a lock whose QC is not a real prepare-QC", i)
		}
	}

	// The honest majority round-changes (a0 and hAtt carry their X locks; free
	// carries none); the designated proposer reaches quorum WITHOUT the
	// Byzantine envelope and must re-propose X.
	honestNodes := []*Node{nodes[0], nodes[hAtt], nodes[free]}
	refill()
	sweepRounds(t, net, holdByzReply, honestNodes)
	refill()
	sweepRounds(t, net, holdByzReply, honestNodes)

	// Release the delayed round-0 precommit (X's old quorum completing late),
	// then converge.
	drainHeld(t, net, fifo)
	for _, nd := range nodes {
		nd.SyncChain(all, func(int, error) {})
	}
	drainHeld(t, net, fifo)

	// Y must exist nowhere; X everywhere; no honest slash. (The Byzantine
	// anchor's Y-prepare lives only in a refused envelope, never in a block, so
	// no slash fires here — the slashable surface is the drills' concern.)
	assertSingleCommit(t, nodes, blkX.Hash(), &honestSlashed)
}
