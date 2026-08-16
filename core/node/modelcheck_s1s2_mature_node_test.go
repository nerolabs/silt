package node

import (
	"crypto/ed25519"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// The node-level MATURE-epoch fixture — the #435 certification's named residual
// (1): the S1/S2 merge-gate oracles ran the round machinery over the real node
// loop only in the LAUNCH regime, and the mature-regime faces were pinned at
// chain level (core/chain/modelcheck_s1s2_mature_test.go) because no node-level
// world reached a governed mature epoch. This fixture builds one, entirely over
// held delivery: 4 launch anchors drain 4 bonded, distinct-domain maturers
// on-chain, the everMature latch trips (F-1), commits cross the epoch boundary,
// and the frozen mature snapshot GOVERNS — the anchors are shed, the >⅔-weight
// quorum is live, and the dynamic S1/S2 schedules then run against it.
//
// The premise is verified FIRST (the #303 verify-the-setup discipline, which
// caught four false-green oracles in the #406 build): the fixture test asserts
// the latch on every replica, the anchors' post-shed ineligibility, a NEGATIVE
// control (an anchors-only proposal must be refused in the governed epoch), and
// a maturer commit at the weight quorum — before any schedule leans on it.
//
// This world is also the deterministic home for the OPEN mature-regime anomaly
// from run 09fbe60-84613 (docs/thinking/2026-08-16-latch-tripped-record-
// correction-and-drain-obs.md): r0 failed at EVERY steady-state height h42–h46,
// each commit arriving only via an r1 new-view. A schedule that reproduces that
// contention belongs here.

// matureWorld builds the governed-mature-epoch world: nodes[0..3] are the
// launch anchors (no bonds — pure training wheels), nodes[4..7] the bonded
// 64 MiB distinct-domain maturers (the field re-split shape, #429). Returns the
// world with the chain already inside the FIRST GOVERNED MATURE EPOCH: head at
// h9 next (boundary h8 frozen the maturer snapshot), latch tripped everywhere.
func matureWorld(t *testing.T) (nodes []*Node, ids []*identity.Identity, net *simnet.Network, refill func()) {
	t.Helper()
	const nAnchors, nMaturers = 4, 4
	sched := simclock.New()
	net = simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids = make([]*identity.Identity, nAnchors+nMaturers)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(8700 + i))
		if i < nAnchors {
			anchors[ids[i].NodeID()] = true
		}
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	chain.Sign(g, ids[0].Signer())
	// MatureValidators=2 with margin 1: four equal 64 MiB distinct-domain bonds
	// give nakamoto-operators 2 and nakamoto-domains 2 — the bar trips exactly as
	// in the field topology. EpochBlocks=8 mirrors the deployed handoff cadence.
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors,
		MatureValidators: 2, OperatorMargin: 1, EpochBlocks: 8}

	nodes = make([]*Node, nAnchors+nMaturers)
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
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
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

	// The maturers' first-time registrations, signed over genesis, one distinct
	// domain each (the A-axis shape that makes NakamotoDomains reach the bar).
	regs := make([]chain.BondReg, nMaturers)
	for i := 0; i < nMaturers; i++ {
		sgn := ids[nAnchors+i].Signer()
		pub := append([]byte(nil), sgn.Public().(ed25519.PublicKey)...)
		regs[i] = chain.NewBondReg(sgn, ports.HashBytes(pub), 64<<20, []byte("stub"), g.Hash(), uint64(i+1))
	}

	anchorIDs := all[:nAnchors]
	// h1: anchor-0 drains all four maturer regs in one block (stub answers are
	// bytes-cheap, far under the per-block reg budget). The latch premise —
	// min(nakamoto-operators, nakamoto-domains) ≥ 2 — holds from this commit.
	nodes[0].pendingBondRegs = map[ports.NodeID]chain.BondReg{}
	for _, r := range regs {
		nodes[0].pendingBondRegs[r.ValidatorID()] = r
	}
	drive := func(proposer int, h uint64, tag string, att []ports.NodeID) {
		prev, next := nodes[proposer].chain.Head()
		if next != h {
			t.Fatalf("drive %s: proposer head next=%d want %d", tag, next, h)
		}
		b := &chain.Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry(tag)}}
		var done bool
		var perr error
		nodes[proposer].proposeBlock(b, att, all, 0, func(err error) { done, perr = true, err })
		drainHeld(t, net, fifo)
		if !done || perr != nil {
			t.Fatalf("drive %s (h%d): done=%v err=%v", tag, h, done, perr)
		}
	}
	// h1 (regs): anchors only — the maturers are not yet on-chain. h2…h8: the
	// banked maturers attest alongside the anchors, which is what QUALIFIES them
	// (C2Metric counts validatorsSeen — a bond participates only once its holder
	// has attested a committed block; the field maturers attest the same way).
	// Maturers are listed FIRST: the gather early-stops at quorum, and an
	// anchors-first order would satisfy it before a maturer is ever asked —
	// leaving every maturer out of the committed Atts and the latch unreachable
	// (the gather still proceeds to the anchors for the strict anchor majority).
	// Distinct entry payload per height — a content root registers once (ErrDupRoot).
	drive(0, 1, "adv-a", anchorIDs[1:])
	matFirst := append(append([]ports.NodeID{}, all[nAnchors:]...), anchorIDs[1:]...)
	for h := uint64(2); h <= 8; h++ {
		drive(0, h, "adv-"+string(rune('a'+h)), matFirst)
	}

	// A 9th identity's registration is the "pending work" that arms the
	// sweep-based round advance in the schedules — refilled per sweep to model
	// the registrant's continuous resubmission (the s1s2World pattern).
	reg9 := identity.FromSeed(8990)
	reg9Pub := append([]byte(nil), reg9.Signer().Public().(ed25519.PublicKey)...)
	head9, _ := nodes[0].chain.Head()
	pend := chain.NewBondReg(reg9.Signer(), ports.HashBytes(reg9Pub), 2<<20, []byte("stub"), head9, 0)
	refill = func() {
		for _, nd := range nodes {
			nd.pendingBondRegs = map[ports.NodeID]chain.BondReg{pend.ValidatorID(): pend}
		}
	}
	refill()
	return nodes, ids, net, refill
}

// TestMatureWorldPremise is the fixture's own gate — the world is usable by a
// schedule ONLY if every claim below holds (verify the setup FIRST).
func TestMatureWorldPremise(t *testing.T) {
	nodes, ids, net, _ := matureWorld(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}

	// 1) The one-way latch tripped on EVERY replica (anchors and maturers — it
	//    is a pure function of the committed blocks they all share).
	for i, nd := range nodes {
		if !nd.chain.EverMature() {
			t.Fatalf("premise: node %d has not latched everMature (head-committed maturity must be replica-independent)", i)
		}
	}

	// 2) The governed epoch's eligible proposers are EXACTLY the four maturers —
	//    the unbonded anchors are shed (T1: scaffolding retires itself).
	elig := nodes[0].chain.EligibleProposers()
	if len(elig) != 4 {
		t.Fatalf("premise: want the 4 bonded maturers eligible post-shed, got %d: %v", len(elig), elig)
	}
	matSet := map[ports.NodeID]bool{}
	for _, id := range all[4:] {
		matSet[id] = true
	}
	for _, id := range elig {
		if !matSet[id] {
			t.Fatalf("premise: post-shed eligible proposer %s is not a maturer (an anchor survived the shed)", id)
		}
	}

	// 3) NEGATIVE control (non-vacuity): an anchors-only proposal in the
	//    governed epoch must FAIL — the anchors hold no weight in the frozen
	//    snapshot, so their coalition can never reach the >⅔-weight quorum.
	prev, h := nodes[0].chain.Head()
	bad := &chain.Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry("anchors-alone")}}
	var badDone bool
	var badErr error
	nodes[0].proposeBlock(bad, []ports.NodeID{all[1], all[2], all[3]}, all, 0, func(err error) { badDone, badErr = true, err })
	drainHeld(t, net, fifo)
	if badDone && badErr == nil {
		t.Fatal("NEGATIVE CONTROL FAILED: an anchors-only coalition committed a block inside the governed mature epoch — the shed did not shed")
	}

	// 4) POSITIVE: a maturer proposal gathering two maturer peers (3 of 4 equal
	//    weights = 192 of 256 MiB > ⅔) commits at the weight quorum.
	prev, h = nodes[4].chain.Head()
	good := &chain.Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry("mature-commit")}}
	var goodDone bool
	var goodErr error
	nodes[4].proposeBlock(good, []ports.NodeID{all[5], all[6]}, all, 0, func(err error) { goodDone, goodErr = true, err })
	drainHeld(t, net, fifo)
	if !goodDone || goodErr != nil {
		t.Fatalf("premise: a 3-of-4 maturer weight quorum must commit in the governed epoch (done=%v err=%v)", goodDone, goodErr)
	}
	// And the commit reached every replica (broadcast path live for the world).
	for i, nd := range nodes {
		if _, hh := nd.chain.Head(); hh != h+1 {
			t.Fatalf("premise: node %d did not track the mature commit (head next=%d want %d)", i, hh, h+1)
		}
	}
}

// TestModelCheck_S1_Mature_DelayedWeightQuorumIsCarriedForward is schedule S1
// in the GOVERNED MATURE regime over the real node loop — the dynamic face of
// the chain-level mature S1 (core/chain/modelcheck_s1s2_mature_test.go), and
// the certification residual this fixture exists to close: X gathers a real
// >⅔-WEIGHT commit quorum at round 0 among the maturers, the final precommit
// reply is held, everyone round-changes — the lock rule must carry X into
// round 1 and the height must commit X there, nothing else, no honest slash.
func TestModelCheck_S1_Mature_DelayedWeightQuorumIsCarriedForward(t *testing.T) {
	nodes, ids, net, refill := matureWorld(t)
	id := func(i int) ports.NodeID { return ids[i].NodeID() }
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	maturers := nodes[4:]

	var honestSlashed bool
	for _, nd := range nodes {
		nd.OnSlash(func(culprit ports.NodeID, _ uint64) { honestSlashed = true })
	}

	// Round 0 at the contested height: maturer m0 (nodes[4]) proposes X and
	// gathers m1+m2 — with the proposer that is 3 of 4 equal weights, a REAL
	// >⅔-weight commit quorum. HOLD m2's precommit reply: the quorum exists in
	// flight but only the proposer could ever complete it (the S1 delay).
	holdM2Reply := func(m simnet.HeldMsg) bool {
		return m.Kind == ports.MsgPrecommitReply && m.From == id(6) && m.To == id(4)
	}
	prev, ch := nodes[4].chain.Head()
	blkX := &chain.Block{Version: 1, Height: ch, Prev: prev, Entries: []ports.Entry{mkEntry("X-mature")}}
	var xDone bool
	var xErr error
	nodes[4].proposeBlock(blkX, []ports.NodeID{id(5), id(6)}, all, 0, func(err error) { xDone, xErr = true, err })
	drainHeldExcept(t, net, holdM2Reply)
	if xDone {
		t.Fatalf("setup: m0's round-0 gather must be suspended on the held reply (got done, err=%v)", xErr)
	}
	// S1 premise: the participants hold round-0 locks on X.
	for _, i := range []int{4, 5, 6} {
		if rs := nodes[i].roundsFor(); rs.Lock == nil || rs.Lock.Hash != blkX.Hash() {
			t.Fatalf("setup: maturer node %d must be locked on X at round 0 (S1 premise)", i)
		}
	}

	// The maturers sweep: stuck height + pending work → round-change to r1
	// carrying the locks → the designated (h, r1) proposer re-proposes X.
	refill()
	sweepRounds(t, net, holdM2Reply, maturers)
	refill()
	sweepRounds(t, net, holdM2Reply, maturers)

	// Release the delayed round-0 precommit — the suspended gather resumes and
	// must be refused by (or agree with) the moved head; no second value.
	drainHeld(t, net, fifo)
	for _, nd := range nodes {
		nd.SyncChain(all, func(int, error) {})
	}
	drainHeld(t, net, fifo)

	// The oracle: everyone committed the contested height, all the SAME X, at
	// CommitRound 1 (anti-vacuity: the view-change did the committing), and no
	// honest validator was slashed.
	for i, nd := range nodes {
		_, hh := nd.Chain().Head()
		if hh <= ch {
			t.Fatalf("LIVENESS: node %d never committed the contested mature height (head next=%d, contested=%d)", i, hh, ch)
		}
		blk := nd.Chain().Blocks(ch)
		if len(blk) == 0 || blk[0].Hash() != blkX.Hash() {
			t.Fatalf("I1 VIOLATION (mature): node %d committed a different block at the contested height", i)
		}
		if blk[0].CommitRound != 1 {
			t.Fatalf("VACUOUS: node %d committed the contested height at round %d, want 1 — the mature view-change did not do the committing", i, blk[0].CommitRound)
		}
	}
	if honestSlashed {
		t.Fatal("I5 VIOLATION (mature): an honest validator was slashed during the round escape")
	}
}

// TestModelCheck_S2_Mature_ForgedLockMisreportCannotForkTheHeight is schedule
// S2 in the governed mature regime: one Byzantine MATURER (64 of 256 MiB = 25%
// weight — inside the <⅓ budget) helps X to a real >⅔-weight round-0 quorum,
// its precommit reply is held (X's delayed completion), and it then misreports
// a round-change claiming a round-0 lock on its own value Y with its lone
// prepare as the "QC". A weight-quorum prepare-QC for Y cannot exist, so every
// honest maturer must refuse the envelope at verifyRoundChange, carry the real
// X locks into round 1, and commit X — nothing else, no honest slash.
func TestModelCheck_S2_Mature_ForgedLockMisreportCannotForkTheHeight(t *testing.T) {
	nodes, ids, net, refill := matureWorld(t)
	id := func(i int) ports.NodeID { return ids[i].NodeID() }
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}

	// Cast around the deterministic (h, r1) designated proposer among the
	// maturers: the Byzantine one must not assemble the new-view, and m0
	// (nodes[4]) proposes X.
	_, ch := nodes[4].chain.Head()
	desig := nodes[4].designatedProposer(ch, 1)
	byz := -1
	for _, i := range []int{5, 6, 7} {
		if id(i) != desig {
			byz = i
			break
		}
	}
	var honest []int
	for _, i := range []int{5, 6, 7} {
		if i != byz {
			honest = append(honest, i)
		}
	}
	hAtt, free := honest[0], honest[1]

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

	// Round 0: m0 proposes X gathering hAtt and the Byzantine maturer (honest
	// on the wire here); the held reply is X's delayed completion.
	holdByzReply := func(m simnet.HeldMsg) bool {
		return m.Kind == ports.MsgPrecommitReply && m.From == id(byz) && m.To == id(4)
	}
	prev, _ := nodes[4].chain.Head()
	blkX := &chain.Block{Version: 1, Height: ch, Prev: prev, Entries: []ports.Entry{mkEntry("X2-mature")}}
	nodes[4].proposeBlock(blkX, []ports.NodeID{id(hAtt), id(byz)}, all, 0, func(error) {})
	drainHeldExcept(t, net, holdByzReply)

	// THE MISREPORT: a round-change to r1 claiming a round-0 lock on Y with the
	// Byzantine maturer's lone prepare as the "QC" — 64 MiB of "support" where
	// the mature rule demands a >⅔-weight prepare-QC.
	blkY := &chain.Block{Version: chain.BlockVersionRounds, Height: ch, Prev: prev, Entries: []ports.Entry{mkEntry("Y2-mature")}}
	chain.Sign(blkY, ids[byz].Signer())
	forged := roundChangeEnv{
		Height:    ch,
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
	probe := net.Endpoint(identity.FromSeed(8992).NodeID())
	for _, i := range []int{4, hAtt, free} {
		probe.Send(id(i), ports.Message{Kind: ports.MsgRoundChange, Data: forgedRaw})
	}
	drainHeldExcept(t, net, holdByzReply)

	// Mechanism check: no honest maturer recorded the forged envelope (the
	// weight-short "QC" must die at verifyRoundChange).
	for _, i := range []int{4, hAtt, free} {
		if m := nodes[i].roundsFor().Changes[1]; m[id(byz)] != nil {
			t.Fatalf("S2-MATURE MECHANISM: node %d recorded the Byzantine round-change carrying a weight-short forged Y-lock", i)
		}
	}

	// The honest maturers sweep to round 1 without the Byzantine envelope; the
	// designated proposer must re-propose the carried X.
	honestNodes := []*Node{nodes[4], nodes[hAtt], nodes[free]}
	refill()
	sweepRounds(t, net, holdByzReply, honestNodes)
	refill()
	sweepRounds(t, net, holdByzReply, honestNodes)

	// Release X's delayed round-0 completion, then converge everyone.
	drainHeld(t, net, fifo)
	for _, nd := range nodes {
		nd.SyncChain(all, func(int, error) {})
	}
	drainHeld(t, net, fifo)

	// Y nowhere; X everywhere at CommitRound 1; no honest slash.
	for i, nd := range nodes {
		_, hh := nd.Chain().Head()
		if hh <= ch {
			t.Fatalf("LIVENESS: node %d never committed the contested mature height (head next=%d, contested=%d)", i, hh, ch)
		}
		blk := nd.Chain().Blocks(ch)
		if len(blk) == 0 || blk[0].Hash() != blkX.Hash() {
			t.Fatalf("I1 VIOLATION (mature S2): node %d committed a different block at the contested height", i)
		}
		if blk[0].CommitRound != 1 {
			t.Fatalf("VACUOUS: node %d committed at round %d, want 1 — the mature view-change did not do the committing", i, blk[0].CommitRound)
		}
	}
	if honestSlashed {
		t.Fatal("I5 VIOLATION (mature S2): an honest validator was slashed")
	}
}
