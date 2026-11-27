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
