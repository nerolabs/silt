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
func matureWorld12(t *testing.T) (nodes []*Node, ids []*identity.Identity, net *simnet.Network, refill func()) {
	t.Helper()
	const nAnchors, nMaturers, nSybils = 4, 4, 4
	total := nAnchors + nMaturers + nSybils
	sched := simclock.New()
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
	return nodes, ids, net, refill
}

func TestModelCheck_451_SilentAuthorLockedValueMustStillCommit(t *testing.T) {
	nodes, ids, net, refill := matureWorld12(t)
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
