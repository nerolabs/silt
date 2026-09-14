package node

import (
	"errors"
	"fmt"
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

// THE FLOOR BOX, END TO END OVER THE WIRE.
//
// The claim these gates hold: a validator that holds NO chain — no blocks, no registry, no tree —
// judges a block by proof, against witnesses it pulls from a node that does hold one, and stalls
// rather than accepts when it can reach no such node. That is the deployment shape, not a
// simplification of it: the box's chain below is config-bearing and empty, and every committed
// value the composition reads crosses the transport.
//
// WHY THIS NEEDS A FIXTURE OF ITS OWN. The view resolves every committed read against a parent
// StateRoot and answers NoWitness before it consults any source when there is none. A fixture
// whose chain stops at genesis therefore stalls a box for want of a state root rather than for
// want of a witness — and a transport gate over such a fixture is green while proving nothing
// about the transport. So this fixture drives the node's OWN consensus path until a v5 block
// carrying a real StateRoot is committed, and the gates assert that before anything else.
//
// WHAT THE GREEN VERDICT IS. It is the DOWNGRADE — IndeterminateTrustlessly /
// ErrRecomputeGated — not Accept. (*chain.Box).Validate maps Accept to that one line deliberately
// (floorbox_box_v5.go): flipping it is a consensus-rule change and is not this seam's to make.
// Reaching the downgrade means the whole composition ran to a verdict of Accept over witnesses
// and was downgraded at the door — which is exactly "validates against witnesses". The forged
// twin in each gate is what keeps that reading honest: a box that stalled on everything would
// reach the downgrade on nothing.

// witnessableValidators builds a swarm whose genesis declares era-4 live from height 1, drives its
// own consensus path to a committed v5 block, and returns a serving validator, a floor box that
// holds no chain at all, and the cold config-bearing chain the box audits through.
//
// THE CONFIGURATION IS STATED, not inherited, because two of its values decide whether these
// gates mean anything:
//
//   - MatureValidators 99 — the network never matures, which is the LAUNCH regime and the state a
//     floor box actually meets first. The maturity latch is false in committed state, so the
//     class-M witness carries the whole validatorsSeen set proven against the block's own
//     post-apply root. Gating on the latched regime instead would hold the easier half of the
//     claim and leave the case every network starts in untested.
//   - EpochBlocks 0 and BondTTLBlocks 0 — no epoch rotation and no TTL sweep, the two transition
//     classes the provider does not yet build a witness for.
//
// Both are asserted below rather than merely commented: a config drift would otherwise turn these
// gates red in a way that reads as a transport regression.
type witnessableSwarm struct {
	server *Node          // holds the tree and serves witnesses
	second *Node          // a second, equally un-permissioned provider
	vals   []ports.NodeID // every validator, for driving the chain forward
	box    *Node          // the floor box: no chain, no blocks, no tree
	cold   *chain.Chain   // the box's config-bearing chain — it holds NO blocks
	parent chain.Block    // the committed v5 block the box anchors on
	cand   chain.Block    // the candidate the box judges
	head   ports.Hash
	net    *simnet.Network
	sched  *simclock.Scheduler
}

func witnessableValidators(t *testing.T) witnessableSwarm {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	const nVals = 4
	ids := make([]*identity.Identity, nVals)
	anchors := map[ports.NodeID]bool{}
	var regs []chain.BondReg
	for i := range ids {
		ids[i] = identity.FromSeed(int64(9400 + i))
		anchors[ids[i].NodeID()] = true
		regs = append(regs, chain.BondReg{Validator: pubOf(ids[i]), Root: ports.HashBytes(pubOf(ids[i])), Size: 2 << 20})
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g-floorbox")}, BondRegs: regs}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, AnchorQuorum: 1,
		MatureValidators: 99, EpochBlocks: 0, BondTTLBlocks: 0,
		Era3ActivationHeight: 1, Era4ActivationHeight: 1,
	}
	if cfg.MatureValidators == 0 || cfg.EpochBlocks != 0 || cfg.BondTTLBlocks != 0 {
		t.Fatal("FIXTURE: the three stated configuration values decide which transition classes the " +
			"witness bundle has to carry; a drift here fails these gates for a reason that is not the transport")
	}

	nodes := make([]*Node, nVals)
	for i, id := range ids {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		if ch.MintVersion(1) != chain.BlockVersionWitnessable {
			t.Fatalf("FIXTURE VACUOUS: height 1 must mint v%d, mints v%d — a sub-v5 chain never enters "+
				"the witnessable composition and every gate over it passes for the wrong reason",
				chain.BlockVersionWitnessable, ch.MintVersion(1))
		}
		nd.EnableChain(ch, id.Signer())
		if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
			t.Fatalf("sign-mark store: %v", err)
		}
		nodes[i] = nd
	}

	// Two committed heights, driven through the node's own propose/gather/commit path. Two and
	// not one: the block the box judges sits above a parent that is itself a committed v5 block
	// with a real carrier, which is the shape every block above genesis has.
	all := make([]ports.NodeID, nVals)
	for i, id := range ids {
		all[i] = id.NodeID()
	}
	for h := uint64(1); h <= 2; h++ {
		prev, next := nodes[0].chain.Head()
		if next != h {
			t.Fatalf("FIXTURE: the chain wants height %d, the driver is at %d", next, h)
		}
		b := &chain.Block{Height: h, Prev: prev, Entries: []ports.Entry{mkEntry(fmt.Sprintf("floorbox-%d", h))}}
		var done bool
		var commitErr error
		nodes[0].proposeBlock(b, all[1:], all, 1, func(err error) { done, commitErr = true, err })
		drainHeld(t, net, fifo)
		if !done {
			t.Fatalf("FIXTURE: the gather at height %d never completed", h)
		}
		if commitErr != nil {
			t.Fatalf("FIXTURE: height %d did not commit: %v", h, commitErr)
		}
	}
	net.DisableHeldDelivery() // the world is built; the witness exchange runs under the clock

	c := nodes[0].chain
	committed := c.Blocks(2)
	if len(committed) == 0 {
		t.Fatal("FIXTURE: nothing committed at height 2")
	}
	parent := committed[len(committed)-1]
	if parent.Version != chain.BlockVersionWitnessable {
		t.Fatalf("FIXTURE VACUOUS: the parent is v%d, want v%d", parent.Version, chain.BlockVersionWitnessable)
	}
	if parent.StateRoot == nil {
		t.Fatal("FIXTURE VACUOUS: the parent carries NO committed StateRoot. A box anchored here stalls for " +
			"want of a state root BEFORE it consults any source, so a green transport gate over it would be " +
			"evidence about nothing — this is the precise failure this fixture exists to end")
	}
	if len(parent.LastCommit) == 0 {
		t.Fatal("FIXTURE VACUOUS: the parent carries no attestation carrier, so the class-A screen the " +
			"bundle serves is never exercised")
	}
	head, _ := c.Head()

	// The floor box: a node on the same transport with NO chain enabled at all, auditing through a
	// chain that carries the network's configuration and holds no blocks. Its ChainID cannot come
	// from that chain (it has no genesis to hash) — it is operator configuration, which is what
	// BoxConfig.ChainID is.
	boxID := identity.FromSeed(9499)
	boxNode := New(boxID.NodeID(), DefaultConfig(), sched, net.Endpoint(boxID.NodeID()), memstore.New())
	boxNode.SetLedger(credit.New(50_000, 0))
	cold := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
	cold.SetBondVerifier(mcStubVerify)
	if n := len(cold.Blocks(0)) + len(cold.Blocks(1)) + len(cold.Blocks(2)); n != 0 {
		t.Fatalf("FIXTURE: the box's chain must hold NO blocks — that IS the deployment shape; it holds %d", n)
	}

	return witnessableSwarm{
		server: nodes[0], second: nodes[1], vals: all, box: boxNode, cold: cold,
		parent: parent, cand: mintCandidate(t, c, ids, "the-candidate"),
		head: head, net: net, sched: sched,
	}
}

// mintCandidate builds the next v5 block on c's head exactly as the propose path does — carrier
// attached, roots populated over the post-apply state, proposer-signed, with a full prepare and
// precommit certificate — and does NOT commit it. The block the box judges has to be one the full
// node would accept, or a stall proves nothing about the box.
func mintCandidate(t *testing.T, c *chain.Chain, ids []*identity.Identity, label string) chain.Block {
	t.Helper()
	prev, h := c.Head()
	b := &chain.Block{Height: h, Prev: prev, Entries: []ports.Entry{mkEntry(label)}}
	b.LastCommit = c.HeadCarrier()
	if err := c.PopulateEra4Roots(b); err != nil {
		t.Fatalf("populate the candidate's roots: %v", err)
	}
	chain.Sign(b, ids[0].Signer())
	for _, id := range ids {
		b.PrepareQC = append(b.PrepareQC, chain.AttestAt(b, id.Signer(), 0, chain.PhasePrepare, c.ChainID()))
		b.Atts = append(b.Atts, chain.AttestAt(b, id.Signer(), 0, chain.PhasePrecommit, c.ChainID()))
	}
	if err := c.ValidateCommit(b); err != nil {
		t.Fatalf("ORACLE BROKEN: the full node refuses the candidate, so no verdict the box reaches "+
			"means anything: %v", err)
	}
	return *b
}

// mkBox returns the constructor ValidateWithWitnesses needs. The source reaches the box through
// its constructor because a Box reads through the source it was CONSTRUCTED with; handing over a
// finished box would leave it reading a source the replay never fills.
func (s witnessableSwarm) mkBox(chainID ports.Hash) func(chain.WitnessSource) (*chain.Box, error) {
	return func(src chain.WitnessSource) (*chain.Box, error) {
		return chain.NewBox(s.cold, s.parent, chain.BoxConfig{BudgetBytes: 1 << 22, ChainID: chainID}, src)
	}
}

// judge drives one floor-box validation to its single verdict and returns it with the number of
// witness requests that crossed the wire.
func (s witnessableSwarm) judge(t *testing.T, b chain.Block, peers []ports.NodeID) (chain.FloorBoxOutcome, error, int) {
	t.Helper()
	before := s.net.Stats.Kinds[ports.MsgGetWitness]
	var out chain.FloorBoxOutcome
	var gotErr error
	fired := 0
	s.box.ValidateWithWitnesses(s.mkBox(s.server.Chain().ChainID()), b, peers, s.head,
		func(o chain.FloorBoxOutcome, e error) { fired++; out, gotErr = o, e })
	s.sched.Run()
	if fired != 1 {
		t.Fatalf("the verdict callback must fire EXACTLY once; fired %d. A box whose callback never "+
			"fires has stopped auditing without saying so", fired)
	}
	return out, gotErr, s.net.Stats.Kinds[ports.MsgGetWitness] - before
}

// TestFloorBoxValidatesOverTheWireWithoutHoldingTheTree is the first of the two legs: a box that
// holds no blocks runs the whole v5 composition to a verdict over witnesses pulled across the
// transport from a node that does.
//
// Ablation: make (*WitnessProvider).Bundle omit one changed leaf, or make serveWitness answer
// OK=false for the leaf reads, and this gate goes red — the box stalls instead of reaching the
// downgrade. The forged arm below is the standing proof that the green arm is not a stall in
// disguise.
func TestFloorBoxValidatesOverTheWireWithoutHoldingTheTree(t *testing.T) {
	s := witnessableValidators(t)

	out, err, wire := s.judge(t, s.cand, []ports.NodeID{s.server.ID()})
	if wire == 0 {
		t.Fatal("NOT A TRANSPORT GATE: no witness request crossed the wire, so whatever the box read, it " +
			"did not read it from the serving node")
	}
	if out != chain.IndeterminateTrustlessly || !errors.Is(err, chain.ErrRecomputeGated) {
		t.Fatalf("the floor box did NOT validate against witnesses: it must run the composition to the "+
			"door's downgrade (IndeterminateTrustlessly / ErrRecomputeGated), which is the verdict Accept "+
			"is mapped to; got %s / %v after %d witness request(s)", out, err, wire)
	}

	// THE FORGED TWIN. Same box, same providers, same transport — a block whose committed StateRoot
	// has been moved. It must NOT reach the downgrade. Without this arm, "reached the downgrade"
	// and "stalled on everything" would be indistinguishable outcomes of a gate that reads only the
	// honest block.
	forged := s.cand
	moved := ports.HashBytes([]byte("a state root the block did not commit"))
	forged.StateRoot = &moved
	if _, fErr, _ := s.judge(t, forged, []ports.NodeID{s.server.ID()}); errors.Is(fErr, chain.ErrRecomputeGated) {
		t.Fatal("VACUITY BROKEN: the box reached the DOWNGRADE on a block whose committed StateRoot was " +
			"moved — it is not reproducing the transition at all, and the honest arm above is green for " +
			"the wrong reason")
	}
}

// TestFloorBoxStallsWhenNoProviderIsReachable is the second leg, and it is a SAFETY claim rather
// than a liveness one: a box that cannot see the evidence must not accept the block, and must not
// render its silence as a rejection either. The vision states it plainly — a witness-less floor box
// stalls, never accepts — because safety must never ride on the tier above.
func TestFloorBoxStallsWhenNoProviderIsReachable(t *testing.T) {
	s := witnessableValidators(t)

	var ghost ports.NodeID
	ghost[0], ghost[1] = 0xDE, 0xAD

	out, err, _ := s.judge(t, s.cand, []ports.NodeID{ghost})
	if out == chain.Accept {
		t.Fatal("ACCEPTED with no witness provider reachable — the box signed off on a block it never checked")
	}
	if out != chain.IndeterminateTrustlessly || !errors.Is(err, ErrWitnessUnreachable) {
		t.Fatalf("an unreachable provider must produce a STALL named as one (ErrWitnessUnreachable), not a "+
			"verdict about the block; got %s / %v", out, err)
	}

	// And with NO providers configured at all — the same stall, reached without a single send.
	out, err, wire := s.judge(t, s.cand, nil)
	if out != chain.IndeterminateTrustlessly || !errors.Is(err, ErrWitnessUnreachable) {
		t.Fatalf("a box with an empty provider list must stall by name; got %s / %v", out, err)
	}
	if wire != 0 {
		t.Fatalf("a box with no providers configured sent %d witness request(s)", wire)
	}

	// VACUITY GUARD. The stall above must be the PROVIDER's absence and not a box that stalls on
	// everything: the same box, the same block, one reachable provider, reaches the downgrade.
	if out, err, _ := s.judge(t, s.cand, []ports.NodeID{s.server.ID()}); !errors.Is(err, chain.ErrRecomputeGated) {
		t.Fatalf("VACUITY BROKEN: this box does not reach a verdict even with a live provider (%s / %v), "+
			"so the stall above is evidence about the box, not about the missing provider", out, err)
	}
}

// TestFloorBoxFailsOverToASecondProvider: the box's SAFETY never rests on the tier above, but its
// LIVENESS does, and a box pinned to one provider has handed that provider a switch over its
// ability to audit at all. Witness-serving is an open, un-permissioned duty of any node that holds
// the tree, so the box carries a list and walks it — and mixing providers inside one validation is
// safe precisely because every answer is checked against a root the box already holds.
func TestFloorBoxFailsOverToASecondProvider(t *testing.T) {
	s := witnessableValidators(t)

	var ghost ports.NodeID
	ghost[0], ghost[1] = 0xDE, 0xAD

	// The second validator is an ordinary peer with no relationship to the box: it serves because
	// it holds the tree, not because it was designated.
	if out, err, _ := s.judge(t, s.cand, []ports.NodeID{ghost, s.second.ID()}); !errors.Is(err, chain.ErrRecomputeGated) {
		t.Fatalf("the box did not walk past a dead provider to a live one — liveness rests on a single "+
			"provider, the choke the open multi-provider tier exists to prevent; got %s / %v", out, err)
	}
}

// TestFloorBoxFinishesAtTheHeadItAnchoredOn. A box names ONE head for the whole of one
// validation, because answers from two committed states mixed into one judgement is the seam the
// head field exists to prevent. The serving node does not stop committing while that validation
// runs — so a provider that only ever answers from its CURRENT head refuses every request after
// the first block lands under it, and a box on a live chain re-anchors and races forever without
// ever finishing. The serving node therefore keeps the snapshot one behind its head.
//
// Ablation: drop the retained snapshot from ProviderCache.For ⇒ the second verdict below becomes
// a stall ⇒ RED.
func TestFloorBoxFinishesAtTheHeadItAnchoredOn(t *testing.T) {
	s := witnessableValidators(t)

	// One full validation at the head the box anchored on, so the serving node has built and
	// cached a provider over it.
	if _, err, _ := s.judge(t, s.cand, []ports.NodeID{s.server.ID()}); !errors.Is(err, chain.ErrRecomputeGated) {
		t.Fatalf("CONTROL BROKEN: the box does not reach a verdict at the undisturbed head: %v", err)
	}

	// The chain moves on. The serving node commits the very block the box was judging, and then
	// answers a request naming no head at all — which is what makes it rebuild over the NEW head
	// and demote the snapshot the box is still anchored on.
	if err := s.server.Chain().Append(s.cand); err != nil {
		t.Fatalf("the serving node must be able to commit its own candidate: %v", err)
	}
	moved, _ := s.server.Chain().Head()
	if moved == s.head {
		t.Fatal("FIXTURE: the serving node's head did not move, so nothing below is under test")
	}
	probe, err := cbor.Marshal(witnessReq{Call: witnessCallMembers, Tag: "bondedRoot\x00"})
	if err != nil {
		t.Fatalf("encode the head probe: %v", err)
	}
	s.box.request(s.server.ID(), ports.Message{Kind: ports.MsgGetWitness, Data: probe},
		func(ports.Message, error) {})
	s.sched.Run()

	// The box's validation, re-run against the head it anchored on. It must still complete.
	if out, err, _ := s.judge(t, s.cand, []ports.NodeID{s.server.ID()}); !errors.Is(err, chain.ErrRecomputeGated) {
		t.Fatalf("the box could not finish at the head it anchored on once the serving node had moved "+
			"past it — a box on a live chain would re-anchor and race forever; got %s / %v", out, err)
	}
}

// commitOneMore drives the serving node's own propose/gather/commit path one height forward, so
// the chain moves under the box exactly as a live chain does.
func (s witnessableSwarm) commitOneMore(t *testing.T, label string) {
	t.Helper()
	prev, h := s.server.Chain().Head()
	b := &chain.Block{Height: h, Prev: prev, Entries: []ports.Entry{mkEntry(label)}}
	var done bool
	var cErr error
	s.server.proposeBlock(b, s.vals[1:], s.vals, 1, func(err error) { done, cErr = true, err })
	s.sched.Run()
	if !done || cErr != nil {
		t.Fatalf("the serving node could not commit at height %d: done=%v err=%v", h, done, cErr)
	}
}

// TestFloorBoxAuditsTheBlockAboveItsPin is the whole entry point, end to end: the box asks a
// provider where it is, pins there, fetches the block that lands above the pin from a peer, and
// judges it against witnesses. Nothing it reads is trusted — the parent is refused unless it
// hashes to the pin, and every witness is checked against the pinned root.
//
// This is the shape a daemon can be driven through, and it is the whole of what a floor box can do
// until the door's Accept downgrade is taken: it audits ONE block above a pin, and never adopts
// it, so it never advances its own head.
func TestFloorBoxAuditsTheBlockAboveItsPin(t *testing.T) {
	s := witnessableValidators(t)

	// 1. Where can this provider answer from? The probe is also what leaves the answering node
	//    holding the snapshot the box is about to ask about.
	var pinHash ports.Hash
	var pinHeight uint64
	var probeErr error
	fired := 0
	s.box.WitnessHead([]ports.NodeID{s.server.ID()}, func(h ports.Hash, height uint64, err error) {
		fired++
		pinHash, pinHeight, probeErr = h, height, err
	})
	s.sched.Run()
	if fired != 1 || probeErr != nil {
		t.Fatalf("the head probe must answer exactly once and succeed; fired=%d err=%v", fired, probeErr)
	}
	if pinHash != s.head || pinHeight != s.parent.Height {
		t.Fatalf("the provider reported head %x@%d, the chain is at %x@%d",
			pinHash[:8], pinHeight, s.head[:8], s.parent.Height)
	}

	// 2. Nothing stands above the pin yet, and the box says so by name rather than returning a
	//    verdict about a block that does not exist.
	pin := FloorBoxPin{Height: pinHeight, Hash: pinHash, ChainID: s.server.Chain().ChainID()}
	if v := s.audit(t, pin, []ports.NodeID{s.server.ID()}); !errors.Is(v.Err, ErrFloorBoxNothingAbovePin) {
		t.Fatalf("with the pin at the head, the box must report that there is nothing to audit yet, "+
			"by name; got %s / %v", v.Outcome, v.Err)
	}

	// 3. The chain moves. The block that lands above the pin is the one the box audits.
	s.commitOneMore(t, "above-the-pin")
	v := s.audit(t, pin, []ports.NodeID{s.server.ID()})
	if v.Height != pin.Height+1 {
		t.Fatalf("the box audited height %d, want %d", v.Height, pin.Height+1)
	}
	if v.Outcome != chain.IndeterminateTrustlessly || !errors.Is(v.Err, chain.ErrRecomputeGated) {
		t.Fatalf("the box did not validate the block above its pin against witnesses: want the door's "+
			"downgrade (IndeterminateTrustlessly / ErrRecomputeGated); got %s / %v", v.Outcome, v.Err)
	}

	// 4. THE PIN IS WHAT MAKES A WRONG HISTORY DETECTABLE. A pin the source does not serve is
	//    refused by name, and the box never begins — it does not fall back on whatever block the
	//    peer happened to have at that height.
	wrong := pin
	wrong.Hash = ports.HashBytes([]byte("a parent this network never committed"))
	if v := s.audit(t, wrong, []ports.NodeID{s.server.ID()}); !errors.Is(v.Err, ErrFloorBoxPinNotServed) {
		t.Fatalf("a pin the source cannot serve must be refused BY NAME before any verdict; got %s / %v",
			v.Outcome, v.Err)
	}

	// 5. And with no provider reachable, the same fetched block yields a STALL, never an accept.
	var ghost ports.NodeID
	ghost[0], ghost[1] = 0xDE, 0xAD
	stalled := s.auditVia(t, pin, []ports.NodeID{s.server.ID()}, []ports.NodeID{ghost})
	if stalled.Outcome == chain.Accept {
		t.Fatal("ACCEPTED with no witness provider reachable")
	}
	if !errors.Is(stalled.Err, ErrWitnessUnreachable) {
		t.Fatalf("an unreachable provider must stall by name; got %s / %v", stalled.Outcome, stalled.Err)
	}
}

// audit runs one audit with the same peers as sources and as providers.
func (s witnessableSwarm) audit(t *testing.T, pin FloorBoxPin, peers []ports.NodeID) FloorBoxVerdict {
	t.Helper()
	return s.auditVia(t, pin, peers, peers)
}

func (s witnessableSwarm) auditVia(t *testing.T, pin FloorBoxPin, sources, providers []ports.NodeID) FloorBoxVerdict {
	t.Helper()
	var got FloorBoxVerdict
	fired := 0
	s.box.AuditAbovePin(s.cold, pin, 1<<22, sources, providers, func(v FloorBoxVerdict) { fired++; got = v })
	s.sched.Run()
	if fired != 1 {
		t.Fatalf("the audit callback must fire EXACTLY once; fired %d", fired)
	}
	return got
}

// TestFloorBoxValidatesEveryTransitionClassAliveNetworkProduces is the gate that decides whether a
// floor box is deployable, as opposed to demonstrable. The fixture above holds epochs and the bond
// TTL OFF, which keeps three transition classes out of every block it mints; a real swarm runs with
// both on, so nearly every block registers a bond, one in four turns an epoch, and the TTL sweeps
// regularly. A seam that only carries the quiet blocks would report a green unit tier and stall on
// the network.
//
// So this drives the shape a deployment actually has: bonded validators on a live chain with a
// short TTL and a short epoch, and the box audits EVERY committed block from a pin one height
// behind it — which is the daemon's own cycle. The counters at the end are the non-vacuity: a run
// in which no block registered a bond, no epoch turned and no bond expired would prove nothing
// about the classes it claims to cover, so it fails rather than passing quietly.
func TestFloorBoxValidatesEveryTransitionClassAliveNetworkProduces(t *testing.T) {
	const (
		heights     = 14
		epochBlocks = 4
		bondTTL     = 2
	)
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())

	const nVals = 4
	ids := make([]*identity.Identity, nVals)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(9500 + i))
		anchors[ids[i].NodeID()] = true
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g-live")}}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{
		Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, AnchorQuorum: 1,
		MatureValidators: 99, EpochBlocks: epochBlocks, BondTTLBlocks: bondTTL,
		Era3ActivationHeight: 1, Era4ActivationHeight: 1,
	}
	nodes := make([]*Node, nVals)
	all := make([]ports.NodeID, nVals)
	for i, id := range ids {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), 2<<20) // a bonded validator registers as it proposes
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		ch.SetBondVerifier(mcStubVerify) // the objective wiring installs the real verifier; this fixture proves no plots
		if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
			t.Fatalf("sign-mark store: %v", err)
		}
		all[i], nodes[i] = id.NodeID(), nd
	}

	boxID := identity.FromSeed(9599)
	box := New(boxID.NodeID(), DefaultConfig(), sched, net.Endpoint(boxID.NodeID()), memstore.New())
	box.SetLedger(credit.New(50_000, 0))
	cold := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
	cold.SetBondVerifier(mcStubVerify)
	pinChainID := nodes[0].Chain().ChainID()

	// The sweep needs a bond that actually LAPSES, and a proposer renews its own on every block it
	// mints, so nothing ever expires on a chain the fixture leaves alone. The proposer therefore
	// stops holding a bond for a stretch in the middle of the run: its registration goes stale, the
	// TTL comes due, and the sweep evicts it — which is the class the box has to reproduce.
	const stopRenewingAt, resumeAt = 5, 10
	savedBond := nodes[0].bond

	var withRegs, boundaries, sweeps, judged int
	for h := uint64(1); h <= heights; h++ {
		server := nodes[0]
		switch h {
		case stopRenewingAt:
			server.bond = nil
		case resumeAt:
			server.bond = savedBond
		}
		prevHash, next := server.Chain().Head()
		if next != h {
			t.Fatalf("the chain wants height %d, the driver is at %d", next, h)
		}
		preEpochStart := server.Chain().Regime().EpochStart
		preBonded := server.Chain().Regime().Bonded

		// Warm the provider at the head the box is about to pin on — the box's own first act, and
		// what leaves the serving node holding the state it will be asked about.
		if h > 1 {
			var probeErr error
			box.WitnessHead([]ports.NodeID{server.ID()}, func(_ ports.Hash, _ uint64, err error) { probeErr = err })
			sched.Run()
			if probeErr != nil {
				t.Fatalf("height %d: the head probe failed: %v", h, probeErr)
			}
		}

		b := &chain.Block{Height: h, Prev: prevHash, Entries: []ports.Entry{mkEntry(fmt.Sprintf("live-%d", h))}}
		var done bool
		var cErr error
		server.proposeBlock(b, all[1:], all, 1, func(err error) { done, cErr = true, err })
		sched.Run()
		if !done || cErr != nil {
			t.Fatalf("height %d: done=%v err=%v", h, done, cErr)
		}
		committed := server.Chain().Blocks(h)
		if len(committed) == 0 {
			t.Fatalf("height %d committed nothing", h)
		}
		blk := committed[0]
		if len(blk.BondRegs) > 0 {
			withRegs++
		}
		if server.Chain().Regime().EpochStart != preEpochStart {
			boundaries++
		}
		if server.Chain().Regime().Bonded < preBonded {
			sweeps++
		}
		if h == 1 {
			continue // there is no committed parent below height 1 to pin on
		}

		pin := FloorBoxPin{Height: h - 1, Hash: prevHash, ChainID: pinChainID}
		var v FloorBoxVerdict
		fired := 0
		box.AuditAbovePin(cold, pin, 1<<23, []ports.NodeID{server.ID()}, []ports.NodeID{server.ID()},
			func(got FloorBoxVerdict) { fired++; v = got })
		sched.Run()
		if fired != 1 {
			t.Fatalf("height %d: the audit callback must fire exactly once; fired %d", h, fired)
		}
		if v.Outcome != chain.IndeterminateTrustlessly || !errors.Is(v.Err, chain.ErrRecomputeGated) {
			t.Fatalf("height %d (regs=%d boundary=%v): the floor box could not judge a block the network "+
				"committed; want the door's downgrade, got %s / %v",
				h, len(blk.BondRegs), server.Chain().Regime().EpochStart != preEpochStart, v.Outcome, v.Err)
		}
		judged++
	}

	if judged != heights-1 {
		t.Fatalf("the box judged %d of %d committed blocks", judged, heights-1)
	}
	if withRegs == 0 || boundaries == 0 || sweeps == 0 {
		t.Fatalf("GATE VACUOUS: the run produced regs=%d boundaries=%d sweeps=%d — a run that exercised "+
			"none of the three classes a live network produces says nothing about whether the bundle "+
			"carries them", withRegs, boundaries, sweeps)
	}
	t.Logf("judged %d blocks: %d carried bond registrations, %d turned an epoch, %d swept an expired bond",
		judged, withRegs, boundaries, sweeps)
}
