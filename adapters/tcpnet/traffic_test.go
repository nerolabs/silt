package tcpnet_test

import (
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/ports"
)

// TestTrafficMeterCountsBothEndsByKind: a frame is counted once on the sender as
// sent and once on the receiver as received, under its kind, with the same byte count
// on both ends; kinds that never crossed are absent.
func TestTrafficMeterCountsBothEndsByKind(t *testing.T) {
	start := func(seed int64) (*tcpnet.Transport, *identity.Identity) {
		loop := eventloop.New()
		go loop.Run()
		t.Cleanup(loop.Stop)
		id := identity.FromSeed(seed)
		tr, err := tcpnet.New(loop, id, "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { tr.Close() })
		return tr, id
	}
	trA, _ := start(220)
	trB, idB := start(221)
	got := make(chan struct{}, 4)
	trB.SetHandler(func(ports.NodeID, ports.Message) { got <- struct{}{} })
	trA.AddPeer(idB.NodeID(), trB.Addr())

	payload := make([]byte, 5000)
	for i := 0; i < 3; i++ {
		if err := trA.Send(idB.NodeID(), ports.Message{Kind: ports.MsgStoreChunk, RID: uint64(i + 1), Data: payload}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-got:
		case <-time.After(3 * time.Second):
			t.Fatal("frame never arrived")
		}
	}

	a, b := trA.Traffic()["StoreChunk"], trB.Traffic()["StoreChunk"]
	if a.OutFrames != 3 || b.InFrames != 3 {
		t.Fatalf("frames: sender out=%d, receiver in=%d, want 3 and 3", a.OutFrames, b.InFrames)
	}
	if a.OutBytes != b.InBytes || a.OutBytes < 3*int64(len(payload)) {
		t.Fatalf("bytes: sender out=%d, receiver in=%d — want equal and at least the 3 payloads (%d)", a.OutBytes, b.InBytes, 3*len(payload))
	}
	if a.InFrames != 0 || b.OutFrames != 0 {
		t.Fatalf("one-way traffic counted the wrong way: sender in=%d, receiver out=%d", a.InFrames, b.OutFrames)
	}
	if _, ok := trA.Traffic()["CommitBlock"]; ok {
		t.Fatal("a kind that never crossed is reported")
	}
}
