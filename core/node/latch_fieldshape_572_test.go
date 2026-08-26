package node

import (
	"crypto/ed25519"
	"fmt"
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

// #572 round 3 — the FIELD-SHAPE latch-replay probe.
//
// TestLatchSurvivesReplay_572 is GREEN on matureWorld12, whose drive stops at
// the latch (h8) with no TTL config, no renewals, and no post-latch rotations.
// The field chain that failed to re-latch (474718e-deep val-d, restored 32
// blocks) crossed all of those: renewal-treadmill regs, TTL-bound bonds, the
// h24 rotation, sybil attesters. This world adds every one of those dimensions
// and drives ORGANICALLY (proposeBlock's real gather, not hand-built
// certificates), then kills the ledger view (a fresh replica, empty live rep —
// the restart shape) and replays the committed chain byte-faithfully.
//
// RED here = the field mechanism reproduced locally (replay under-latches).
// GREEN here = the divergence needs yet another dimension; the boot-state
// instrumentation shipped alongside names the diverging map on the next field
// occurrence either way.
func fieldWorld12(t *testing.T) (nodes []*Node, ids []*identity.Identity, net *simnet.Network, sched *simclock.Scheduler) {
	t.Helper()
	const nAnchors, nMaturers, nSybils = 4, 4, 4
	total := nAnchors + nMaturers + nSybils
	sched = simclock.New()
	net = simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids = make([]*identity.Identity, total)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(9900 + i))
		if i < nAnchors {
			anchors[ids[i].NodeID()] = true
		}
	}
	g := &chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{mkEntry("g572")}}
	chain.Sign(g, ids[0].Signer())
	// The field's shape: TTL bounds every bond (field 32; scaled here so the
	// drive crosses lapse+renewal cycles), epochs of 8, latch at 2 operators.
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors,
		MatureValidators: 2, OperatorMargin: 1, EpochBlocks: 8, BondTTLBlocks: 22}

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
	return nodes, ids, net, sched
}

func TestLatchReplayFieldShape_572(t *testing.T) {
	nodes, ids, net, _ := fieldWorld12(t)
	total := len(ids)
	all := make([]ports.NodeID, total)
	for i := range ids {
		all[i] = ids[i].NodeID()
	}

	mkReg := func(i int, size int64, domain uint64) chain.BondReg {
		sgn := ids[i].Signer()
		pub := append([]byte(nil), sgn.Public().(ed25519.PublicKey)...)
		head, _ := nodes[0].chain.Head()
		return chain.NewBondReg(sgn, ports.HashBytes(pub), size, []byte("stub"), head, domain)
	}
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
	nonAnchorsFirst := append(append([]ports.NodeID{}, all[4:]...), all[1], all[2], all[3])

	// h1: anchor-0 drains everyone's founding regs (anchors 64M, maturers 64M
	// with distinct domains, sybils 1M — the field mix).
	for i := 1; i < total; i++ {
		size, dom := int64(64<<20), uint64(0)
		if i >= 8 {
			size = 1 << 20
		} else if i >= 4 {
			dom = uint64(i - 3)
		}
		nodes[0].queuePendingBondReg(mkReg(i, size, dom))
	}
	drive(0, 1, "f572-a", all[1:])
	nodes[1].queuePendingBondReg(mkReg(0, 64<<20, 0))
	drive(1, 2, "f572-b", append(append([]ports.NodeID{}, all[4:]...), all[0], all[2], all[3]))

	// h3..h26: the renewal treadmill at the field's cadence — anchors+maturers
	// renew at TTL/2 (h11, h22 — clear of the #506 R-rule), the sybils skip
	// their renewals so their h1 bonds LAPSE at h24 (TTL 22) and re-register
	// at h25 (the lapse+re-entry churn the field window carried). Crosses the
	// latch (~h2), the HANDOFF rotation at h8, and rotations h16/h24.
	for h := uint64(3); h <= 26; h++ {
		if h == 11 || h == 22 {
			for i := 0; i < 8; i++ {
				size, dom := int64(64<<20), uint64(0)
				if i >= 4 {
					dom = uint64(i - 3)
				}
				nodes[0].queuePendingBondReg(mkReg(i, size, dom))
			}
		}
		if h == 25 {
			for i := 8; i < total; i++ {
				nodes[0].queuePendingBondReg(mkReg(i, 1<<20, 0))
			}
		}
		drive(0, h, fmt.Sprintf("f572-%d", h), nonAnchorsFirst)
	}

	src := nodes[0].Chain()
	if !src.EverMature() {
		t.Fatal("premise: the organic drive must have latched (2 maturer operators committed by h25)")
	}

	// The restart shape: wire-faithful bytes, a FRESH replica (empty live rep —
	// the ledger view a restarted daemon has), full Reload.
	blocks, err := chain.DecodeBlocks(chain.EncodeBlocks(src.Blocks(0)))
	if err != nil {
		t.Fatalf("roundtrip: %v", err)
	}
	anchors := map[ports.NodeID]bool{}
	for i := 0; i < 4; i++ {
		anchors[ids[i].NodeID()] = true
	}
	fresh := chain.New(chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: anchors, MatureValidators: 2, OperatorMargin: 1, EpochBlocks: 8, BondTTLBlocks: 6},
		func(ports.NodeID) int64 { return 0 })
	fresh.SetBondVerifier(mcStubVerify)
	n, err := fresh.Reload(blocks)
	if err != nil || n != len(blocks) {
		t.Fatalf("#572 REPRODUCED (replay refuses the field-shape history): Reload restored %d of %d: %v", n, len(blocks), err)
	}
	if !fresh.EverMature() {
		t.Fatalf("#572 REPRODUCED (field shape): the live drive latched everMature but replaying the same %d committed blocks did NOT — the 474718e-deep val-d under-latch, deterministic", len(blocks))
	}
	if src.EverMature() != fresh.EverMature() {
		t.Fatal("latch divergence")
	}

	// THE FIELD MECHANISM (named by the 8a52aba-deep save/restore regime pair:
	// saved seen=12 everMature=true → restored seen=0 everMature=false over the
	// same 40 blocks): the daemon replayed BEFORE EnableObjectiveChain wired the
	// verifier, and objective() = MinBond>0 && verifyBond!=nil — so the replay
	// ran the LEGACY rep-gated qualification with an empty boot ledger and
	// validatorsSeen rebuilt empty. The Reload guard must REFUSE that replay
	// loudly instead of silently under-latching. RED-provable by removing the
	// guard: this exact call then "succeeds" with EverMature()==false.
	// MinAttesterRep>0 is the field's -min-rep: with the verifier unwired the
	// legacy branch gates on rep()>=MinAttesterRep and the boot ledger is 0 —
	// nobody qualifies, seen rebuilds empty (a 0 threshold would mask this:
	// 0>=0 passes everyone, which is why earlier oracles stayed green).
	unwired := chain.New(chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		MinAttesterRep: 100, MinProposerRep: 100,
		Anchors: anchors, MatureValidators: 2, OperatorMargin: 1, EpochBlocks: 8, BondTTLBlocks: 22},
		func(ports.NodeID) int64 { return 0 })
	if n, err := unwired.Reload(blocks); err == nil {
		t.Fatalf("#572 REPRODUCED: an objective-config replica replayed %d blocks with NO bond verifier and did not refuse (EverMature=%v) — the daemon-boot ordering bug silently demotes qualification to the empty-ledger legacy path and loses the latch", n, unwired.EverMature())
	}
}
