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

// Consensus model-check — TIER 2 (over the REAL node loop, via simnet held-delivery).
//
// Tier 1 (core/chain/modelcheck*_test.go) asserts I1/I3 over the Chain finality
// predicate directly. Tier 2 drives the REAL propose→gather→attest→commit→broadcast
// loop of several `*Node`s over the held-delivery network, where the DRIVER chooses
// the delivery order — so it reaches what tier 1 can't: the invariants that only exist
// once messages race through the actual gather and persist across a restart (I2, and
// I5's honest-never-slashed / the #397 delivery-race face).
//
// This file holds: (1) the SUBSTRATE proof — a real round commits + broadcasts entirely
// over driver-controlled held-delivery; (2) the I2-across-restart oracle — a validator
// that signed at a height, after a crash+restart, refuses a competitor there (the #397
// watermark survives restart), failing-first BY CONSTRUCTION via a non-persisted-mark
// control; (3) the I5/#397 honest-never-slashed oracle — two proposers cross-attest-race a
// height in a WEAK config where both forks can commit (the pre-#402 baseline: objective,
// no anchors, ByzantineQuorum off, Quorum 1, N=4 → no finality gate, no anchor gate), and
// no honest validator is slashed. #402's I1 shields this fork in the normal objective
// tree, so the weak config is what makes it reachable — the resolution of the deferral
// noted in docs/thinking/2026-08-15-406-tier2-substrate-and-the-i5-honesty-catch.md.

// mcStubVerify makes objective() true without real space-time proofs — anchors qualify
// via launchAnchor (zero-bond, pre-handoff), so the model-check needs no VDF work.
func mcStubVerify(_ []byte, _ ports.Hash, _ int64, _ uint64, _ []byte) bool { return true }

// tier2Net wires n anchor Nodes to a HELD-DELIVERY network sharing one genesis, each
// with its own persisted sign-mark store. Returns the nodes and the network. No
// StartChainSync (its self-rescheduling timers would never quiesce); the driver calls
// proposeBlock directly and moves the resulting messages by hand.
func tier2AnchorNet(t *testing.T, nAnchors int) ([]*Node, []*identity.Identity, *simnet.Network, *chain.Block, chain.Config) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids := make([]*identity.Identity, nAnchors)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(8500 + i))
		anchors[ids[i].NodeID()] = true
	}
	g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, MatureValidators: 99}

	nodes := make([]*Node, nAnchors)
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
	return nodes, ids, net, g, cfg
}

// drainHeld delivers parked messages until the network is quiescent (no message in
// flight) or a step bound is hit, picking the next message via pick(pending)→index.
// Returns the number of deliveries. Since the driver never advances the clock, request
// timeouts never fire — a gather completes purely by delivered replies.
func drainHeld(t *testing.T, net *simnet.Network, pick func([]simnet.HeldMsg) int) int {
	t.Helper()
	const bound = 10000
	steps := 0
	for {
		p := net.Pending()
		if len(p) == 0 {
			return steps
		}
		if steps++; steps > bound {
			t.Fatalf("held-delivery did not quiesce within %d steps (livelock?)", bound)
		}
		net.Deliver(p[pick(p)].ID)
	}
}

func fifo(p []simnet.HeldMsg) int { return 0 }

// TestModelCheckTier2_RoundCommitsOverHeldDelivery is the substrate proof: a real
// proposer gathers a real quorum and commits a block entirely over driver-controlled
// delivery (no clock advance, no auto-timers), and the commit broadcasts to every
// replica. If this works, the held-delivery layer genuinely drives the node loop —
// the precondition for every adversarial tier-2 schedule.
func TestModelCheckTier2_RoundCommitsOverHeldDelivery(t *testing.T) {
	nodes, ids, net, g, _ := tier2AnchorNet(t, 4)
	attesters := []ports.NodeID{ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}
	all := []ports.NodeID{ids[0].NodeID(), ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}

	b := &chain.Block{Version: chain.BlockVersion, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("blk1")}}
	var done bool
	var commitErr error
	nodes[0].proposeBlock(b, attesters, all, 1, func(err error) { done, commitErr = true, err })

	drainHeld(t, net, fifo)

	if !done {
		t.Fatal("proposeBlock never completed over held-delivery (gather did not finish)")
	}
	if commitErr != nil {
		t.Fatalf("round must commit over held-delivery: %v", commitErr)
	}
	for i, nd := range nodes {
		// Head() returns the NEXT height, so a committed height-1 block ⇒ Head()==2.
		if hh, h := nd.Chain().Head(); h != 2 || hh != b.Hash() {
			t.Fatalf("node %d must hold the committed block-1 (broadcast): head height=%d hash=%x want height 2 hash %x", i, h, hh, b.Hash())
		}
	}
}

// runI2Restart drives the I2-across-restart scenario over the real loop + held-delivery
// and reports whether the restarted validator REFUSED a competitor at a height it had
// already signed. anchor[1] attests block A at height 1 through a real gather, then
// "crashes and restarts" (a fresh Node + chain on the same endpoint). If persist is
// true the restart reloads the SAME sign-mark store (the shipped behavior); if false it
// gets a fresh store (the pre-#397 in-memory-only behavior). A competitor B at height 1,
// proposed by the one anchor that never signed height 1, is then presented for
// attestation. Returns whether it was refused.
func runI2Restart(t *testing.T, persist bool) bool {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids := make([]*identity.Identity, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(8600 + i))
		anchors[ids[i].NodeID()] = true
	}
	id := func(i int) ports.NodeID { return ids[i].NodeID() }
	g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, MatureValidators: 99}

	marks := make([]ports.SignMarkStore, 4)
	mkNode := func(i int, mark ports.SignMarkStore) *Node {
		nd := New(id(i), DefaultConfig(), sched, net.Endpoint(id(i)), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, ids[i].Signer())
		if err := nd.SetSignMarkStore(mark); err != nil {
			t.Fatalf("mark store: %v", err)
		}
		return nd
	}
	nodes := make([]*Node, 4)
	for i := range nodes {
		marks[i] = markstore.NewMem()
		nodes[i] = mkNode(i, marks[i])
	}

	// Real gather: anchor[0] proposes A at h1, attested by 1 and 2 (proposer+2 = the
	// strict anchor majority). anchor[1] thereby signs A at height 1 (mark persisted);
	// anchor[3] stays free (never signs h1) so it can later propose the competitor.
	blkA := &chain.Block{Version: chain.BlockVersion, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("A")}}
	nodes[0].proposeBlock(blkA, []ports.NodeID{id(1), id(2)}, []ports.NodeID{id(1), id(2), id(3)}, 1, func(error) {})
	drainHeld(t, net, fifo)
	if _, h := nodes[1].Chain().Head(); h != 2 {
		t.Fatalf("setup: anchor[1] must have committed A at h1 (so it signed height 1), head=%d", h)
	}

	// Crash + restart anchor[1]: fresh Node/chain on the same endpoint. persist ⇒ reuse
	// the durable mark store; !persist ⇒ a fresh one (the never-#397 crash-wipes case).
	restartMark := marks[1]
	if !persist {
		restartMark = markstore.NewMem()
	}
	nodes[1] = mkNode(1, restartMark)

	// Competitor B at h1, proposed by anchor[3] (which never signed h1, so it may
	// propose). Present it to the restarted anchor[1] for attestation and read the reply.
	blkB := &chain.Block{Version: chain.BlockVersion, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("B")}}
	chain.Sign(blkB, ids[3].Signer())
	probeID := identity.FromSeed(8699).NodeID()
	probe := net.Endpoint(probeID)
	var replies []bool
	probe.SetHandler(func(_ ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgAttestReply {
			replies = append(replies, msg.OK)
		}
	})
	probe.Send(id(1), ports.Message{Kind: ports.MsgProposeBlock, Data: chain.Encode(blkB)})
	drainHeld(t, net, fifo)

	if len(replies) != 1 {
		t.Fatalf("expected exactly one attest reply from the restarted validator, got %v", replies)
	}
	return !replies[0] // refused == !OK
}

// TestModelCheckTier2_I2_RestartRefusesContradiction is the I2 oracle over the real
// loop: a validator that signed a block at a height, after a crash+restart, must refuse
// a competitor at that height — the never-sign-twice watermark must survive restart
// (#397 Q1b; Tendermint priv_validator_state). Failing-first BY CONSTRUCTION: the same
// scenario with a NON-persisted mark (the pre-#397 crash-wipe) attests the competitor,
// so the control proves the persistence is what does the work — this test is not
// vacuously green.
func TestModelCheckTier2_I2_RestartRefusesContradiction(t *testing.T) {
	if refused := runI2Restart(t, true); !refused {
		t.Fatal("I2 VIOLATION — a restarted validator attested a block contradicting its pre-crash signature; the persisted sign-mark must survive restart (#397 Q1b)")
	}
	// Control (failing-first witness): without persistence the restart WIPES the mark
	// and the validator attests the competitor — proving the assertion above is
	// load-bearing, not vacuous.
	if refused := runI2Restart(t, false); refused {
		t.Fatal("control broken: with a NON-persisted mark the restart should have attested the competitor (the mark is lost) — if it refused, the test is not actually exercising persistence")
	}
}

// runI5HonestSlash drives the #397 honest-slash race over the real loop + held-delivery
// in a WEAK config where two forks CAN both commit — objective but NO anchors,
// ByzantineQuorum off, Quorum 1, N=4 — so RequiredQuorum(1) < bftThreshold(4)=2 leaves
// finalityQuorumActive FALSE (no finality gate) and there is no anchor gate. This is the
// environment the #402/#357 fixes deliberately removed; recreating it here isolates the
// #397 propose-time watermark as the SOLE protection against an honest double-sign.
// The two proposers are each other's ONLY attester, so the #397 cross-attest wedge is
// the clean two-block one: A = P0(prop)+P1(attest), B = P1(prop)+P0(attest). With the
// shipped propose-watermark each proposer refuses the rival, so NEITHER commits (no fork,
// no double-sign — a benign launch stall the designated-proposer rule resolves in
// production, not this oracle's concern). Under the revert each cross-attests → both
// commit → both double-sign → sync slashes both. v2/v3 exist only so N=4 keeps the
// finality gate off (bftThreshold(4)=2 > Quorum 1). Returns whether an honest node was
// slashed. (Non-vacuity is established by the controlled-revert RED, verified in the
// commit message — the #397 watermark is not test-toggleable, so this mirrors the
// controlled-revert failing-first proof used by the I1/I3/#357 oracles.)
func runI5HonestSlash(t *testing.T) (honestSlashed bool) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids := make([]*identity.Identity, 4)
	for i := range ids {
		ids[i] = identity.FromSeed(int64(8700 + i))
	}
	id := func(i int) ports.NodeID { return ids[i].NodeID() }
	all := []ports.NodeID{id(0), id(1), id(2), id(3)}
	pub := func(i int) []byte {
		return append([]byte(nil), ids[i].Signer().Public().(ed25519.PublicKey)...)
	}
	g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	for i := range ids {
		g.BondRegs = append(g.BondRegs, chain.BondReg{Validator: pub(i), Root: ports.HashBytes(pub(i)), Size: 8 << 20})
	}
	chain.Sign(g, ids[0].Signer())
	// Weak config: no Anchors, no ByzantineQuorum, Quorum 1 → no finality gate, no anchor
	// gate (see the finalityQuorumActive arithmetic above).
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, MatureValidators: 0}

	nodes := make([]*Node, 4)
	for i := range nodes {
		nd := New(id(i), DefaultConfig(), sched, net.Endpoint(id(i)), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, ids[i].Signer())
		if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
			t.Fatalf("mark store: %v", err)
		}
		nodes[i] = nd
	}
	// Cross-attest-first delivery: deliver a proposal sent to the OTHER proposer first.
	crossFirst := func(p []simnet.HeldMsg) int {
		for i, m := range p {
			if m.Kind == ports.MsgProposeBlock && (m.To == id(0) || m.To == id(1)) {
				return i
			}
		}
		return 0
	}
	blkA := &chain.Block{Version: chain.BlockVersion, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("A")}}
	blkB := &chain.Block{Version: chain.BlockVersion, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("B")}}
	// P0 and P1 are each other's ONLY attester (the clean cross-attest wedge).
	nodes[0].proposeBlock(blkA, []ports.NodeID{id(1)}, all, 1, func(error) {})
	nodes[1].proposeBlock(blkB, []ports.NodeID{id(0)}, all, 1, func(error) {})
	drainHeld(t, net, crossFirst)

	// Detect slashes via the OnSlash callback — slashEquivocators fires it on DETECTION
	// (fetched chain vs local), before any on-chain slash record. (Checking
	// Chain().IsSlashed would be wrong: that reflects only a COMMITTED slash block, which
	// this scenario never proposes — so the honest slash would go undetected. All four
	// validators are honest, so ANY slash here is an honest one.)
	honest := map[ports.NodeID]bool{id(0): true, id(1): true, id(2): true, id(3): true}
	for _, nd := range nodes {
		nd.OnSlash(func(culprit ports.NodeID, _ uint64) {
			if honest[culprit] {
				honestSlashed = true
			}
		})
	}
	for _, nd := range nodes {
		nd.SyncChain(all, func(int, error) {})
	}
	drainHeld(t, net, crossFirst)
	return honestSlashed
}

// TestModelCheckTier2_I5_HonestNeverSlashed397 is the #397 honest-slash oracle: under a
// two-proposer cross-attest race, in a config where BOTH forks can commit, no honest
// validator is ever slashed — the #397 propose-time never-sign-twice watermark makes each
// proposer refuse to attest the rival's block, so no honest node double-signs.
// FAILING-FIRST (controlled revert): removing the propose-time recordSign in proposeBlock
// lets each proposer cross-attest → both forks commit → sync slashes both honest proposers
// (RED). With the shipped watermark — GREEN.
func TestModelCheckTier2_I5_HonestNeverSlashed397(t *testing.T) {
	if runI5HonestSlash(t) {
		t.Fatal("I5/#397 VIOLATION — an honest validator was slashed under the two-proposer cross-attest race; the propose-time never-sign-twice watermark must stop any honest double-sign")
	}
}
