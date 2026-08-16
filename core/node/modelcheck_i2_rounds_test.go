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

// Consensus model-check — I2 extended per-(height, ROUND, phase) across
// restart (#432; certification §5.3): the never-sign-twice guarantee is now
// SLOT-scoped, and both halves must survive a crash:
//
//   REFUSE: a validator that signed a block at (h, r, prepare), restarted,
//   must still refuse a competitor at the SAME slot — the #397 guarantee,
//   round-scoped.
//
//   ESCAPE: the same competitor at a HIGHER round, justified by a valid
//   new-view certificate, must be PREPARED — the #432 liveness escape must
//   also survive restart, or a crash re-wedges the height the rounds unwedged.
//
//   RE-PRESENT: a validator that LOCKED (adopted a prepare-QC) and crashed
//   must re-hydrate the lock from its persisted mark and carry it in its next
//   round-change — the certification's "a restarted validator re-presents the
//   same lock it held before the crash, never a blank one" (§5.3). A lost
//   lock is exactly how a delayed lower-round quorum gets orphaned (S1).

// i2RoundsWorld: 4 anchors over held delivery with EXPOSED mark stores (so a
// "restart" can reload the same store), full sync-seed wiring.
func i2RoundsWorld(t *testing.T) ([]*Node, []*identity.Identity, []ports.SignMarkStore, *simnet.Network, *chain.Block, func(i int, mark ports.SignMarkStore) *Node) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids := make([]*identity.Identity, 4)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(9100 + i))
		anchors[ids[i].NodeID()] = true
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, MatureValidators: 99}

	mkNode := func(i int, mark ports.SignMarkStore) *Node {
		nd := New(ids[i].NodeID(), DefaultConfig(), sched, net.Endpoint(ids[i].NodeID()), memstore.New())
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
		seed := make([]ports.NodeID, 0, 3)
		for j := range ids {
			if j != i {
				seed = append(seed, ids[j].NodeID())
			}
		}
		nd.chainSyncSeed = seed
		return nd
	}
	marks := make([]ports.SignMarkStore, 4)
	nodes := make([]*Node, 4)
	for i := range nodes {
		marks[i] = markstore.NewMem()
		nodes[i] = mkNode(i, marks[i])
	}
	return nodes, ids, marks, net, g, mkNode
}

// craftNewView builds a valid new-view certificate for (height 1, round 1):
// signed no-lock round-change envelopes from the given identities — what an
// honest quorum that saw no prepare-QC would emit.
func craftNewView(t *testing.T, signers []*identity.Identity) [][]byte {
	t.Helper()
	var out [][]byte
	for _, id := range signers {
		rc := roundChangeEnv{Height: 1, NewRound: 1,
			Sender: append([]byte(nil), id.Signer().Public().(ed25519.PublicKey)...)}
		rc.Sig = ed25519.Sign(id.Signer(), rc.sigBytes())
		raw, err := cbor.Marshal(rc)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, raw)
	}
	return out
}

// TestModelCheck_I2_RoundScopedRestart drives REFUSE and ESCAPE through the
// wire against a restarted validator, plus the non-persisted control (the
// failing-first-by-construction witness that REFUSE is the mark's work).
func TestModelCheck_I2_RoundScopedRestart(t *testing.T) {
	run := func(persist bool) (refusedSameSlot, preparedHigherRound bool) {
		nodes, ids, marks, net, g, mkNode := i2RoundsWorld(t)
		id := func(i int) ports.NodeID { return ids[i].NodeID() }

		// a0 proposes A gathering ONLY a1 — the gather fails short of the anchor
		// majority, but a1 has SIGNED (h1, r0, prepare, A), mark persisted.
		blkA := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("A")}}
		nodes[0].proposeBlock(blkA, []ports.NodeID{id(1)}, []ports.NodeID{id(1)}, 0, func(error) {})
		drainHeld(t, net, fifo)

		// Crash + restart a1 (fresh Node, same endpoint); persist selects the
		// real store or a blank one (the pre-#397 crash-wipe control).
		restartMark := marks[1]
		if !persist {
			restartMark = markstore.NewMem()
		}
		nodes[1] = mkNode(1, restartMark)

		// Competitor C at h1 from a3 (never signed h1).
		blkC := &chain.Block{Version: chain.BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("C")}}
		chain.Sign(blkC, ids[3].Signer())
		probe := net.Endpoint(identity.FromSeed(9199).NodeID())
		var replies []bool
		probe.SetHandler(func(_ ports.NodeID, msg ports.Message) {
			if msg.Kind == ports.MsgAttestReply {
				replies = append(replies, msg.OK)
			}
		})

		// REFUSE: C at the SAME slot (round 0).
		env0, err := cbor.Marshal(proposeEnv{Raw: chain.Encode(blkC), Round: 0})
		if err != nil {
			t.Fatal(err)
		}
		probe.Send(id(1), ports.Message{Kind: ports.MsgProposeBlock, Data: env0})
		drainHeld(t, net, fifo)

		// ESCAPE: the SAME C at round 1 with a valid new-view certificate
		// (round-changes from a0, a2, a3 — a quorum that saw no lock).
		env1, err := cbor.Marshal(proposeEnv{Raw: chain.Encode(blkC), Round: 1,
			NewView: craftNewView(t, []*identity.Identity{ids[0], ids[2], ids[3]})})
		if err != nil {
			t.Fatal(err)
		}
		probe.Send(id(1), ports.Message{Kind: ports.MsgProposeBlock, Data: env1})
		drainHeld(t, net, fifo)

		if len(replies) != 2 {
			t.Fatalf("expected two attest replies (same-slot, higher-round), got %v", replies)
		}
		return !replies[0], replies[1]
	}

	refused, prepared := run(true)
	if !refused {
		t.Fatal("I2 VIOLATION — a restarted validator prepared a competitor at the SAME (h, r, prepare) slot it signed before the crash; the round-scoped mark must survive restart (#397 Q1b per #432)")
	}
	if !prepared {
		t.Fatal("I4 VIOLATION — a restarted validator refused the HIGHER-round proposal carried by a valid new-view certificate; a crash must not re-wedge the height the rounds unwedged (#432)")
	}
	// Control: with the mark wiped, the same-slot competitor is prepared —
	// proving REFUSE above is the persisted mark's work, not something else.
	if refusedCtl, _ := run(false); refusedCtl {
		t.Fatal("control broken: with a NON-persisted mark the restarted validator should have prepared the same-slot competitor — the refusal is not exercising persistence")
	}
}

// TestModelCheck_I2_RestartRepresentsLock is the §5.3 RE-PRESENT oracle: a
// validator that LOCKED on a prepare-QC and crashed re-hydrates the lock from
// its persisted mark and its next round-change CARRIES it — never a blank one.
// Failing-first BY CONSTRUCTION: the blank-store control loses the lock.
func TestModelCheck_I2_RestartRepresentsLock(t *testing.T) {
	run := func(persist bool) (rehydrated, carried bool) {
		nodes, ids, marks, net, g, mkNode := i2RoundsWorld(t)
		id := func(i int) ports.NodeID { return ids[i].NodeID() }

		// a0 gathers a REAL prepare-QC for X (a1+a2) and sends MsgPrepareQC;
		// a1 LOCKS (adoptLock persists the lock with its precommit mark). Hold
		// every precommit reply so nothing commits — the height stays open.
		holdReplies := func(m simnet.HeldMsg) bool { return m.Kind == ports.MsgPrecommitReply }
		blkX := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("X")}}
		nodes[0].proposeBlock(blkX, []ports.NodeID{id(1), id(2)}, []ports.NodeID{id(1), id(2), id(3)}, 0, func(error) {})
		drainHeldExcept(t, net, holdReplies)
		if rs := nodes[1].roundsFor(); rs.Lock == nil || rs.Lock.Hash != blkX.Hash() {
			t.Fatal("setup: a1 must be locked on X before the crash")
		}

		// Crash + restart a1.
		restartMark := marks[1]
		if !persist {
			restartMark = markstore.NewMem()
		}
		nodes[1] = mkNode(1, restartMark)

		rs := nodes[1].roundsFor()
		rehydrated = rs.Lock != nil && rs.Lock.Hash == blkX.Hash() && rs.Lock.Round == 0

		// Drive a1's round-change (pending work + two sweeps) and read what a
		// peer RECORDED from it: the envelope must carry the X lock.
		reg5 := identity.FromSeed(9198)
		regPub := append([]byte(nil), reg5.Signer().Public().(ed25519.PublicKey)...)
		reg := chain.NewBondReg(reg5.Signer(), ports.HashBytes(regPub), 2<<20, []byte("stub"), g.Hash(), 0)
		nodes[1].pendingBondRegs = map[ports.NodeID]chain.BondReg{reg.ValidatorID(): reg}
		nodes[1].maybeAdvanceRound()
		drainHeldExcept(t, net, holdReplies)
		nodes[1].pendingBondRegs = map[ports.NodeID]chain.BondReg{reg.ValidatorID(): reg}
		nodes[1].maybeAdvanceRound()
		drainHeldExcept(t, net, holdReplies)

		raw := nodes[3].roundsFor().Changes[1][id(1)]
		if raw == nil {
			t.Fatal("setup: a3 never recorded a1's round-change — the broadcast did not happen")
		}
		var rc roundChangeEnv
		if err := cbor.Unmarshal(raw, &rc); err != nil {
			t.Fatal(err)
		}
		if len(rc.LockQC) > 0 {
			lb, err := chain.Decode(rc.LockBlock)
			carried = err == nil && lb.Hash() == blkX.Hash() && rc.LockRound == 0
		}
		return rehydrated, carried
	}

	rehydrated, carried := run(true)
	if !rehydrated {
		t.Fatal("§5.3 VIOLATION — the restarted validator did not re-hydrate its lock from the persisted mark (a crashed locked validator re-presents a BLANK lock; S1's delayed quorum can then be orphaned)")
	}
	if !carried {
		t.Fatal("§5.3 VIOLATION — the restarted validator's round-change did not CARRY the re-hydrated lock")
	}
	// Control: a blank store loses the lock (proving the assertions above are
	// the persistence's work).
	if rehydratedCtl, carriedCtl := run(false); rehydratedCtl || carriedCtl {
		t.Fatal("control broken: with a NON-persisted mark the lock should be lost after restart")
	}
}
