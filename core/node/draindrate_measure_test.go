package node

// The E5 drain-rate measurement (PE drain ruling 2026-08-19, the go/no-go rider
// on Phase 1.2): the v2b starvation latency is ≈ cap/drain, so the question that
// decides whether the two-class priority drain is ever built is the REAL
// single-loop drain rate for the cheapest bulk a flood can ride — measured on a
// real node handler over a real transport, not the v2b drill's hypothetical
// 2 MiB/s. MsgStoreChunk is that bulk (the drill's own flood kind; bond-reg
// submits are now rate-gated, Phase 1.2). This is a MEASUREMENT harness, not a
// pass/fail gate — it prints the number and the derived cap/drain at the shipped
// 256M against the 2 s saturation bound, so the verdict is evidence, not a guess.
// Records into docs/thinking/2026-08-19-bondreg-submit-cpu-gate.md's E5 rider.
//
// Run explicitly:
//   go test ./core/node -run TestMeasure_StoreChunkDrainRate -v -count=1
// Skipped in the normal suite (it is a benchmark-shaped measurement, timing-
// sensitive, and asserts only a sanity floor).

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/adapters/walltime"
	"github.com/nerolabs/silt/ports"
)

func TestMeasure_StoreChunkDrainRate(t *testing.T) {
	if testing.Short() {
		t.Skip("drain-rate measurement (~10s); explicit-run only")
	}

	// Receiver: a real node over a real TLS transport with the real MsgStoreChunk
	// handler (hash-verify + store) — the actual production drain path.
	rLoop := eventloop.New()
	go rLoop.Run()
	defer rLoop.Stop()
	rID := identity.FromSeed(9600)
	rTr, err := tcpnet.New(rLoop, rID, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer rTr.Close()
	recv := New(rID.NodeID(), DefaultConfig(), walltime.New(rLoop), rTr, memstore.New())
	_ = recv

	// Sender floods valid, self-consistent chunks (c.Verify() passes → the full
	// store path runs; a flood of garbage would fail the hash cheaply and measure
	// the wrong, faster path).
	sLoop := eventloop.New()
	go sLoop.Run()
	defer sLoop.Stop()
	sID := identity.FromSeed(9601)
	sTr, err := tcpnet.New(sLoop, sID, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sTr.Close()

	// acks is written by the transport's readLoop goroutine and read by the
	// test body after the window — atomic, or the measurement itself races
	// (#507: the one test that forced -race runs to skip it).
	var acks atomic.Int64
	sTr.SetHandler(func(_ ports.NodeID, m ports.Message) {
		if m.Kind == ports.MsgStoreChunkAck {
			acks.Add(1)
		}
	})
	sTr.AddPeer(rID.NodeID(), rTr.Addr())

	const chunkSize = 256 << 10 // 256 KiB — realistic coded-shard bulk
	data := make([]byte, chunkSize)
	for i := range data {
		data[i] = byte(i * 131)
	}
	id := ports.HashBytes(data) // the id must hash the data or c.Verify() rejects

	// Warm the connection, then measure sustained ack throughput over a window.
	const window = 5 * time.Second
	const inflight = 64 // enough to keep the receiver's loop saturated
	sendOne := func() { _ = sTr.Send(rID.NodeID(), ports.Message{Kind: ports.MsgStoreChunk, ChunkID: id, Data: data}) }
	for i := 0; i < inflight; i++ {
		sendOne()
	}
	start := time.Now()
	deadline := time.After(window)
	ticker := time.NewTicker(200 * time.Microsecond)
	defer ticker.Stop()
loop:
	for {
		select {
		case <-deadline:
			break loop
		case <-ticker.C:
			sendOne() // keep the pipe full
		}
	}
	elapsed := time.Since(start)

	got := acks.Load()
	bytes := got * chunkSize
	mbps := float64(bytes) / elapsed.Seconds() / (1 << 20)
	const shippedCap = 256 << 20
	capOverDrain := float64(shippedCap) / (mbps * (1 << 20))

	t.Logf("drain measurement: %d chunks acked in %v = %.0f MB/s (%d KiB chunks, real store handler)",
		got, elapsed.Round(time.Millisecond), mbps, chunkSize>>10)
	t.Logf("→ at the shipped 256M cap, cap/drain = %.3f s (v2b saturation bound = 2.0 s)", capOverDrain)
	if capOverDrain > 2.0 {
		t.Logf("→ VERDICT: cap/drain EXCEEDS the bound at this drain — v2b starvation is REACHABLE; re-run the parked drill re-parameterized")
	} else {
		t.Logf("→ VERDICT: cap/drain is UNDER the bound at this drain — starvation not reachable via cheap bulk on this hardware; SHELVE per E5 (floor-box confirmation still owed)")
	}
	if mbps < 1.0 {
		t.Fatalf("drain measurement implausibly low (%.2f MB/s) — harness not actually draining", mbps)
	}
}
