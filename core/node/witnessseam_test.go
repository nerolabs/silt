package node

import (
	"bytes"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// The witness seam over the real transport.
//
// WHAT THESE GATES COVER, precisely, so a green run is not read as more than it is: the
// SERVING side and the wire. A request crosses the transport, a real provider answers from a
// real committed chain, the reply decodes, and the proof it carries resolves against the
// committed root. That is the half an operator's floor box depends on and the half no other
// gate in the tree touches — the box's own gates drive an in-process source, and the
// provider's gates compare the production provider to that source without a wire between them.
//
// WHAT THEY DO NOT COVER: a full box-through-the-wire validation. That needs a parent block
// carrying a committed v5 StateRoot, because the view resolves every committed read against
// one and returns NoWitness before it ever consults a source when there is none. This
// package's validator fixtures commit genesis only, so a box anchored there stalls for want of
// a state root rather than for want of a witness — a stall that would make a transport gate
// pass while proving nothing about the transport. Building that fixture is the remaining work
// on this seam and it is named here rather than papered over.

// idBytes views a NodeID as the raw key bytes a leaf key is built from.
func idBytes(id ports.NodeID) []byte { return id[:] }

// TestWitnessServerAnswersOverTheWire drives one leaf request across the transport and checks
// the answer against the committed root — the property the whole seam rests on.
func TestWitnessServerAnswersOverTheWire(t *testing.T) {
	server, client, _, _, _, sched := twoAgreeingValidators(t)

	// The key the box reads first on any block: is the proposer slashed? It is a real
	// committed keyspace, so an honest server owes a proof either way — present or absent.
	sc := server.Chain()
	head, _ := sc.Head()
	prov, ok := chain.NewWitnessProvider(sc)
	if !ok {
		t.Fatal("the serving node must be able to build a provider over its own committed chain")
	}
	key := statehash.Key("slashed\x00", idBytes(server.ID()))
	req, err := cbor.Marshal(witnessReq{Call: witnessCallLeaf, Head: head, Key: key})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}

	var got witnessResp
	var gotErr error
	fired := 0
	client.request(server.ID(), ports.Message{Kind: ports.MsgGetWitness, Data: req},
		func(resp ports.Message, rerr error) {
			fired++
			if rerr != nil {
				gotErr = rerr
				return
			}
			got, gotErr = decodeWitnessResp(resp.Data, head)
		})
	sched.Run()

	if fired != 1 {
		t.Fatalf("the reply callback must fire exactly once; fired %d", fired)
	}
	if gotErr != nil {
		t.Fatalf("a live serving node must answer a witness request: %v", gotErr)
	}
	if !got.OK {
		t.Fatal("the server refused a request for a REAL committed keyspace — an honest provider owes a " +
			"proof here either way, because absence is itself a proven fact and not a refusal")
	}
	if got.Head != head {
		t.Fatalf("the server answered from head %x, not the head asked about (%x) — answers from two heads "+
			"mixed into one judgement is the seam the head field exists to prevent", got.Head[:8], head[:8])
	}

	// The decoded proof must be usable: it has to resolve against the root the asker already
	// holds. A reply that decodes but does not resolve would be a transport that silently
	// corrupts evidence, which is worse than one that drops it.
	w, err := witnessFromProof(got.Proof)
	if err != nil {
		t.Fatalf("the proof the server sent does not decode: %v", err)
	}
	if w.IsNil() {
		t.Fatal("the server sent no proof for a real committed keyspace — a box cannot tell a proven " +
			"absence from a missing answer, which is exactly the forgery it exists to refuse")
	}

	// And it must be the SAME proof the local provider produces. A transport that reshaped it
	// would make every gate that passed against the in-process provider evidence about a
	// server no deployment uses.
	localV, localW, localOK := prov.Leaf(key)
	if !localOK {
		t.Fatal("the local provider refused a key the wire answered — the two disagree about what is servable")
	}
	localB, err := localW.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal local proof: %v", err)
	}
	if !bytes.Equal(localB, got.Proof) {
		t.Fatalf("the wire reshaped the proof: %d bytes local, %d bytes over the wire", len(localB), len(got.Proof))
	}
	if !bytes.Equal(localV, got.Value) {
		t.Fatalf("the wire reshaped the value: local %x, wire %x", localV, got.Value)
	}
}

// TestWitnessServerRefusesAHeadItDoesNotHold: a provider is a snapshot, so a request naming a
// head this node has moved past must be REFUSED rather than answered from the current one.
// Answering would hand the box leaves from one state and leaves from another, and the
// composition would stall on the seam with nothing to name.
func TestWitnessServerRefusesAHeadItDoesNotHold(t *testing.T) {
	server, client, _, _, _, sched := twoAgreeingValidators(t)

	var stranger ports.Hash
	stranger[0] = 0x5A
	req, err := cbor.Marshal(witnessReq{
		Call: witnessCallLeaf, Head: stranger,
		Key: statehash.Key("slashed\x00", idBytes(server.ID())),
	})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}

	var okFlag bool
	fired := 0
	client.request(server.ID(), ports.Message{Kind: ports.MsgGetWitness, Data: req},
		func(resp ports.Message, rerr error) {
			fired++
			if rerr == nil {
				okFlag = resp.OK
			}
		})
	sched.Run()

	if fired != 1 {
		t.Fatalf("the reply callback must fire exactly once; fired %d", fired)
	}
	if okFlag {
		t.Fatal("the server ANSWERED a request naming a head it does not hold — a box would mix two states " +
			"into one judgement and stall on the seam with nothing to name")
	}
}

// TestWitnessDriverReportsExactlyOnceWhenNothingAnswers: whatever the verdict, the floor box's
// callback fires exactly once and never Accepts when the provider is unreachable. A box whose
// callback never fires has stopped auditing without saying so — indistinguishable from a slow
// one, and the worst of the three outcomes.
func TestWitnessDriverReportsExactlyOnceWhenNothingAnswers(t *testing.T) {
	_, client, a1, _, _, sched := twoAgreeingValidators(t)

	c := client.Chain()
	parent := c.Blocks(0)[0]
	prev, next := c.Head()
	b := chain.Block{Version: 5, Height: next, Prev: prev, Entries: []ports.Entry{mkEntry("witness-once")}}
	chain.Sign(&b, a1.Signer())

	var ghost ports.NodeID
	ghost[0], ghost[1] = 0xDE, 0xAD

	var gotOut chain.FloorBoxOutcome
	fired := 0
	client.ValidateWithWitnesses(
		func(src chain.WitnessSource) (*chain.Box, error) {
			return chain.NewBox(c, parent, chain.BoxConfig{BudgetBytes: 1 << 22, ChainID: c.ChainID()}, src)
		},
		b, chain.StateRootWitness{}, ghost, ports.Hash{},
		func(o chain.FloorBoxOutcome, _ error) { fired++; gotOut = o })
	sched.Run()

	if fired != 1 {
		t.Fatalf("the verdict callback must fire EXACTLY once; fired %d", fired)
	}
	if gotOut == chain.Accept {
		t.Fatal("ACCEPTED with no witness provider reachable — the box signed off on a block it never checked")
	}
}
