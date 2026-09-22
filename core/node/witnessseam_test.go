package node

import (
	"bytes"
	"errors"
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
// WHAT THEY DO NOT COVER: the box itself. These gates are driven on a genesis-only fixture,
// whose head carries no committed v5 StateRoot — and the view answers NoWitness for every
// committed read before it consults any source when there is none, so a box anchored here
// stalls for want of a state root rather than for want of a witness. Routing a box through
// this fixture would assert a transport property while never reaching the transport. The
// box-through-the-wire legs live in witnessfloorbox_test.go, over a fixture that drives the
// node's own consensus path to a committed v5 block first.

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

// TestWitnessFetchFailsOverPastADeadProvider: a floor box's liveness must not rest on ONE
// provider. Witness-serving is an open, un-permissioned duty of any archival or pruning node,
// so the box carries a list and walks it; a box pinned to a single provider has handed that
// provider an outage switch over its ability to audit at all.
//
// Driven at the FETCH layer, which is where the walk lives: this gate pins the walk itself —
// that an unreachable provider is stepped past and the live one's answer is FILED, so the next
// replay pass does not ask for the same read again. The same property through the box, on a
// chain whose head carries a real state root, is TestFloorBoxFailsOverToASecondProvider.
func TestWitnessFetchFailsOverPastADeadProvider(t *testing.T) {
	server, client, _, _, _, sched := twoAgreeingValidators(t)

	sc := server.Chain()
	head, _ := sc.Head()
	key := statehash.Key("slashed\x00", idBytes(server.ID()))
	req := witnessReq{Call: witnessCallLeaf, Head: head, Key: key}

	var ghost ports.NodeID
	ghost[0], ghost[1] = 0xDE, 0xAD

	run := func(peers []ports.NodeID) (*witnessCache, error) {
		cache := newWitnessCache(head)
		var gotErr error
		fired := 0
		client.fetchWitnesses(peers, cache, []witnessReq{req}, func(e error) { fired++; gotErr = e })
		sched.Run()
		if fired != 1 {
			t.Fatalf("the fetch callback must fire EXACTLY once; fired %d", fired)
		}
		return cache, gotErr
	}

	// Vacuity guard: the ghost alone must genuinely be unreachable, or "failover worked" would
	// be indistinguishable from "the first provider answered all along".
	if _, err := run([]ports.NodeID{ghost}); !errors.Is(err, ErrWitnessUnreachable) {
		t.Fatalf("the ghost must be unreachable on its own, else the failover arm proves nothing; got %v", err)
	}

	// Dead first, live second: the walk must reach the live provider and file its answer.
	cache, err := run([]ports.NodeID{ghost, server.ID()})
	if err != nil {
		t.Fatalf("the fetch did not fail over past a dead provider to a live one — liveness rests on a single "+
			"provider, the choke the open multi-provider tier exists to prevent: %v", err)
	}
	if _, _, ok := cache.Leaf(key); !ok {
		t.Fatal("failover reported success but filed no answer — the cache is empty, so the next replay pass " +
			"would ask for the same read again and the box would never converge")
	}
}
