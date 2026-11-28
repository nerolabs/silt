package tcpnet_test

import (
	"net"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/ports"
)

func writersPair(t *testing.T, seed int64, writers string) (*tcpnet.Transport, *tcpnet.Transport, chan ports.MsgKind) {
	t.Helper()
	mk := func(s int64) *tcpnet.Transport {
		loop := eventloop.New()
		go loop.Run()
		t.Cleanup(loop.Stop)
		tr, err := tcpnet.New(loop, identity.FromSeed(s), "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { tr.Close() })
		return tr
	}
	sender, receiver := mk(seed), mk(seed+1)
	_, n, err := net.ParseCIDR(writers)
	if err != nil {
		t.Fatal(err)
	}
	receiver.SetWriters([]*net.IPNet{n}, []ports.MsgKind{ports.MsgFetchChunk, ports.MsgFetchChunkReply})
	got := make(chan ports.MsgKind, 8)
	receiver.SetHandler(func(_ ports.NodeID, m ports.Message) { got <- m.Kind })
	sender.AddPeer(identity.FromSeed(seed+1).NodeID(), receiver.Addr())
	return sender, receiver, got
}

func expect(t *testing.T, got chan ports.MsgKind, want ports.MsgKind) {
	t.Helper()
	select {
	case k := <-got:
		if k != want {
			t.Fatalf("delivered %v, want %v", k, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%v was not delivered", want)
	}
}

// TestANonWriterCanReadButNotWrite: from an address outside the writer list, a store, a
// token request, a bond registration and a credit-lane message are dropped, while a fetch on
// the same connection is delivered — a joiner reads, and cannot write. The gate is an
// allow-list, so the credit lane is closed without being named.
func TestANonWriterCanReadButNotWrite(t *testing.T) {
	sender, receiver, got := writersPair(t, 700, "192.0.2.0/24") // TEST-NET-1: not loopback
	to := identity.FromSeed(701).NodeID()
	for _, k := range []ports.MsgKind{ports.MsgStoreChunk, ports.MsgTokenRequest, ports.MsgSubmitBondReg, ports.MsgDeliveryFund, ports.MsgFetchChunk} {
		if err := sender.Send(to, ports.Message{Kind: k}); err != nil {
			t.Fatal(err)
		}
	}
	expect(t, got, ports.MsgFetchChunk)
	select {
	case k := <-got:
		t.Fatalf("a write (%v) from a non-writer address was delivered", k)
	case <-time.After(500 * time.Millisecond):
	}
	if n := receiver.WritesRefused(); n != 4 {
		t.Fatalf("WritesRefused = %d, want 4", n)
	}
	if receiver.IsWriter(identity.FromSeed(700).NodeID()) {
		t.Fatal("IsWriter is true for a peer connecting from an unlisted address")
	}
}

// TestAListedWriterCanWrite: the same writes from a listed address are delivered.
func TestAListedWriterCanWrite(t *testing.T) {
	sender, receiver, got := writersPair(t, 710, "127.0.0.0/8")
	to := identity.FromSeed(711).NodeID()
	if err := sender.Send(to, ports.Message{Kind: ports.MsgStoreChunk}); err != nil {
		t.Fatal(err)
	}
	expect(t, got, ports.MsgStoreChunk)
	if n := receiver.WritesRefused(); n != 0 {
		t.Fatalf("WritesRefused = %d for a listed writer", n)
	}
	if !receiver.IsWriter(identity.FromSeed(710).NodeID()) {
		t.Fatal("IsWriter is false for a peer connecting from a listed address")
	}
}

// TestPinnedKindsNeedAListedIdentity: at a writer address, an unlisted identity — a publish
// client's throwaway one — may still store a piece, but may not submit a bond registration;
// a listed identity may do both.
func TestPinnedKindsNeedAListedIdentity(t *testing.T) {
	for _, listed := range []bool{false, true} {
		seed := int64(730)
		if listed {
			seed = 740
		}
		sender, receiver, got := writersPair(t, seed, "127.0.0.0/8")
		var ids []ports.NodeID
		if listed {
			ids = append(ids, identity.FromSeed(seed).NodeID())
		} else {
			ids = append(ids, identity.FromSeed(999).NodeID())
		}
		receiver.SetWriterIDs(ids, []ports.MsgKind{ports.MsgSubmitBondReg})
		to := identity.FromSeed(seed + 1).NodeID()
		sender.Send(to, ports.Message{Kind: ports.MsgSubmitBondReg})
		sender.Send(to, ports.Message{Kind: ports.MsgStoreChunk})
		seen := map[ports.MsgKind]bool{} // the two may arrive on different lanes, in either order
		deadline := time.After(3 * time.Second)
	collect:
		for {
			select {
			case k := <-got:
				seen[k] = true
			case <-deadline:
				break collect
			}
		}
		if !seen[ports.MsgStoreChunk] {
			t.Fatalf("listed=%v: a store from a writer address was refused", listed)
		}
		if seen[ports.MsgSubmitBondReg] != listed {
			t.Fatalf("listed=%v: bond registration delivered=%v", listed, seen[ports.MsgSubmitBondReg])
		}
		if want := map[bool]int64{false: 1, true: 0}[listed]; receiver.WritesRefused() != want {
			t.Fatalf("listed=%v: WritesRefused = %d, want %d", listed, receiver.WritesRefused(), want)
		}
	}
}
