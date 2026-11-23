package node

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// foreignRig is one validator that runs this build, and a raw peer standing in for
// a node of a later build: it answers chain requests with a block in a format this
// build does not validate, and records every reply the validator sends it.
type foreignRig struct {
	sched   *simclock.Scheduler
	n       *Node
	peer    *simnet.Endpoint
	peerID  ports.NodeID
	genesis chain.Block
	future  chain.Block
	replies []ports.Message
	lg      *levelCaptureLog
}

func newForeignRig(t *testing.T, version uint64) *foreignRig {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 4, simnet.DefaultConfig())
	a1, a2 := identity.FromSeed(51101), identity.FromSeed(51102)
	g := chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-foreign")}}
	chain.Sign(&g, a1.Signer())
	ccfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
		Anchors: map[ports.NodeID]bool{a1.NodeID(): true, a2.NodeID(): true}, AnchorQuorum: 1, MatureValidators: 99}

	nd := New(a1.NodeID(), DefaultConfig(), sched, net.Endpoint(a1.NodeID()), memstore.New())
	nd.SetLedger(credit.New(50_000, 0))
	nd.EnableBond(a1.Signer(), 2<<20)
	ch := chain.New(ccfg, func(ports.NodeID) int64 { return 0 })
	if err := ch.AppendGenesis(g); err != nil {
		t.Fatal(err)
	}
	nd.EnableChain(ch, a1.Signer())
	nd.EnableObjectiveChain()
	lg := &levelCaptureLog{}
	nd.SetLogger(lg)

	// The later build's block: well formed, signed, and linked to this chain's
	// genesis, so its format version is the only thing wrong with it.
	fb := chain.Block{Version: version, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("next-era-entry")}}
	chain.Sign(&fb, a2.Signer())
	fb.Atts = []chain.Attestation{chain.Attest(&fb, a1.Signer())}

	r := &foreignRig{sched: sched, n: nd, peer: net.Endpoint(a2.NodeID()), peerID: a2.NodeID(),
		genesis: g, future: fb, lg: lg}
	r.peer.SetHandler(func(from ports.NodeID, msg ports.Message) {
		switch msg.Kind {
		case ports.MsgGetChainHead:
			h := r.future.Hash()
			_ = r.peer.Send(from, ports.Message{Kind: ports.MsgChainHeadReply, OK: true, RID: msg.RID,
				Height: r.future.Height, Data: h[:]})
		case ports.MsgGetChain:
			_ = r.peer.Send(from, ports.Message{Kind: ports.MsgChainReply, OK: true, RID: msg.RID,
				Data: chain.EncodeBlocks([]chain.Block{r.genesis, r.future})})
		default:
			r.replies = append(r.replies, msg)
		}
	})
	return r
}

// commit hands the validator the later build's block as a committed broadcast.
func (r *foreignRig) commit(rid uint64) {
	_ = r.peer.Send(r.n.ID(), ports.Message{Kind: ports.MsgCommitBlock, RID: rid, Data: chain.Encode(&r.future)})
	r.sched.Run()
}

func (r *foreignRig) warnLines() []map[string]any {
	var out []map[string]any
	for i, ev := range r.lg.events {
		if strings.HasPrefix(ev, "foreign-format:") && r.lg.levels[i] == ports.LogWarn {
			m := map[string]any{}
			for j := 0; j+1 < len(r.lg.kvs[i]); j += 2 {
				m[fmt.Sprint(r.lg.kvs[i][j])] = r.lg.kvs[i][j+1]
			}
			out = append(out, m)
		}
	}
	return out
}

// A node meeting a block from an era after its own refuses it, holds its head, says
// why at warn level on both paths a block arrives by, and goes on answering peers.
func TestBlockFromALaterEraStallsTheNodeLoudlyAndLeavesItServing(t *testing.T) {
	r := newForeignRig(t, chain.BlockVersionWitnessable+1)
	headBefore, _ := r.n.Chain().Head()

	r.commit(101)
	if got := r.n.Chain().Len(); got != 1 {
		t.Fatalf("the node adopted a block in a format it cannot validate: chain length %d, want 1", got)
	}
	if len(r.replies) != 1 || r.replies[0].Kind != ports.MsgCommitAck || r.replies[0].OK {
		t.Fatalf("the commit must be answered with a refusal, got %+v", r.replies)
	}
	ff := r.n.ForeignFormat()
	if ff.Count != 1 || ff.Version != chain.BlockVersionWitnessable+1 || ff.Height != 1 || !ff.Newer ||
		ff.Via != "commit" || ff.From != fmt.Sprintf("%x", r.peerID[:]) || ff.Max != chain.BlockVersionWitnessable {
		t.Fatalf("the refusal is not recorded as what it was: %+v", ff)
	}
	w := r.warnLines()
	if len(w) != 1 {
		t.Fatalf("want one warn line naming the refusal, got %d (events %v)", len(w), r.lg.events)
	}
	if a, _ := w[0]["action"].(string); !strings.Contains(a, "upgrades the binary") {
		t.Fatalf("the warn line must tell the operator the remedy, got action=%q", a)
	}

	// The second path: catching up from the later-era peer.
	done := false
	r.n.SyncChain([]ports.NodeID{r.peerID}, func(int, error) { done = true })
	r.sched.Run()
	if !done {
		t.Fatal("the sync never completed: a foreign-format reply must end the sweep, not hang it")
	}
	if headAfter, _ := r.n.Chain().Head(); headAfter != headBefore {
		t.Fatal("chain sync adopted the later-era block")
	}
	if ff := r.n.ForeignFormat(); ff.Count != 2 || ff.Via != "chain-sync" {
		t.Fatalf("the sync refusal is not recorded: %+v", ff)
	}
	if len(r.warnLines()) != 2 {
		t.Fatalf("the second refusal must log too, got %d warn lines", len(r.warnLines()))
	}

	// Still serving: the stalled node answers a head probe with its own head.
	r.replies = nil
	_ = r.peer.Send(r.n.ID(), ports.Message{Kind: ports.MsgGetChainHead, RID: 202})
	r.sched.Run()
	if len(r.replies) != 1 || r.replies[0].Kind != ports.MsgChainHeadReply || !r.replies[0].OK ||
		string(r.replies[0].Data) != string(headBefore[:]) {
		t.Fatalf("the stalled node stopped answering peers: %+v", r.replies)
	}
}

// A peer that keeps re-sending the block cannot flood the log, and the line keeps
// coming back while the stall lasts.
func TestForeignFormatWarnLineBacksOffWithoutGoingQuiet(t *testing.T) {
	r := newForeignRig(t, chain.BlockVersionWitnessable+1)
	for i := uint64(0); i < 9; i++ {
		r.commit(300 + i)
	}
	if got := r.n.Stats.ForeignFormatBlocks; got != 9 {
		t.Fatalf("every refusal is counted: got %d, want 9", got)
	}
	var at []any
	for _, w := range r.warnLines() {
		at = append(at, w["refused"])
	}
	if fmt.Sprint(at) != "[1 2 4 8]" {
		t.Fatalf("warn lines at refusals %v, want [1 2 4 8]", at)
	}
}

// A pre-format block is refused the same way, and the remedy named is not an upgrade:
// the sender is behind, not this node.
func TestPreFormatBlockIsRefusedWithoutAskingForAnUpgrade(t *testing.T) {
	r := newForeignRig(t, 0)
	r.commit(401)
	ff := r.n.ForeignFormat()
	if ff.Count != 1 || ff.Version != 0 || ff.Newer {
		t.Fatalf("a version-0 block must be recorded as pre-format: %+v", ff)
	}
	w := r.warnLines()
	if len(w) != 1 {
		t.Fatalf("want one warn line, got %d", len(w))
	}
	if a, _ := w[0]["action"].(string); strings.Contains(a, "upgrade") {
		t.Fatalf("a pre-format sender must not be reported as a reason to upgrade, got %q", a)
	}
}

// Control: a block that fails to decode for any other reason is not a format verdict
// and is not counted as one.
func TestGarbledBlockIsNotCountedAsAForeignFormat(t *testing.T) {
	r := newForeignRig(t, chain.BlockVersionWitnessable+1)
	_ = r.peer.Send(r.n.ID(), ports.Message{Kind: ports.MsgCommitBlock, RID: 501, Data: []byte{0xff, 0x00}})
	r.sched.Run()
	if got := r.n.Stats.ForeignFormatBlocks; got != 0 {
		t.Fatalf("a garbled block was counted as a foreign format: %d", got)
	}
	if len(r.warnLines()) != 0 {
		t.Fatal("a garbled block produced a foreign-format warn line")
	}
}
