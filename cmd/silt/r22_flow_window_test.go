package main

// R2.2 / blind PE ruling B1 — the pooled flow row must be the SUM OF THE ROWS beneath it.
//
// THE DEFECT, as the PE measured it on the real fixture: pooled differenced two whole-sample
// totals while each object row differenced from that root's FIRST APPEARANCE, so a root
// cared for mid-window put its entire pre-window lifetime inflow into the pooled delta.
//
//	"pooled":{"skimIn":5000,"bountyOut":0,"net":5000}
//	"objects":[{"root":"0100…","net":0},{"root":"0200…","net":0}]
//
// It drives Draining, Panel 3's headline alarm: an entry masks a real drain, a departure
// latches a false one.
//
// TWO ARMS, because the node has NO uncare API — a departure cannot be driven through the
// real care path, so it is driven against flowWindowRows, which is the same function the
// handler calls. The entry arm goes end to end over HTTP so the wiring is covered too.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/ports"
)

// b1Ring builds a ring from (root -> funded/paid) maps, one per sample.
func b1Ring(samples ...map[string][2]int64) []flowSample {
	base := time.Unix(1_700_000_000, 0)
	ring := make([]flowSample, 0, len(samples))
	for i, m := range samples {
		fs := flowSample{at: base.Add(time.Duration(i) * flowSampleInterval), obj: map[string]ports.DurabilitySnapshot{}}
		for root, fp := range m {
			fs.obj[root] = ports.DurabilitySnapshot{Funded: fp[0], Paid: fp[1]}
		}
		ring = append(ring, fs)
	}
	return ring
}

func TestR22PooledFlowIsTheSumOfTheRowsBeneathIt(t *testing.T) {
	// SUBTESTS, not straight-line arms: under the controlled revert arm 1 fails first and
	// a straight-line test would leave the other three unproved.
	t.Run("entry", func(t *testing.T) {
		// ---- ARM 1: a root ENTERS mid-window carrying a large pre-window lifetime inflow.
		// This is the PE's measured case, in its own numbers.
		entry := b1Ring(
			map[string][2]int64{"0100": {1000, 0}},
			map[string][2]int64{"0100": {1000, 0}, "0200": {5000, 0}},
		)
		pooled, objects, _ := flowWindowRows(entry)
		var sum int64
		for _, o := range objects {
			sum += o.Net
			if o.Net != 0 {
				t.Fatalf("object %s reports net %d over a window in which nothing moved", o.Root, o.Net)
			}
		}
		if pooled.Net != sum {
			t.Fatalf("pooled net %d but the rows beneath it sum to %d. A root cared for mid-window put its whole PRE-window lifetime inflow into one window (blind PE B1). A pooled figure that is not the sum of the rows printed beneath it is a lie whatever the arithmetic behind it", pooled.Net, sum)
		}

	})

	t.Run("departure", func(t *testing.T) {
		// ---- ARM 2: a root DEPARTS mid-window. Its whole contribution leaves the numerator,
		// so the old pooled arithmetic saw a large negative step that no row explains.
		// Driven here because the node has no uncare API.
		departure := b1Ring(
			map[string][2]int64{"0100": {1000, 0}, "0200": {9000, 0}},
			map[string][2]int64{"0100": {1000, 0}},
			map[string][2]int64{"0100": {1000, 0}},
			map[string][2]int64{"0100": {1000, 0}},
			map[string][2]int64{"0100": {1000, 0}},
		)
		pooled, objects, _ := flowWindowRows(departure)
		var sum int64
		for _, o := range objects {
			sum += o.Net
		}
		if pooled.Net != sum {
			t.Fatalf("after a departure pooled net = %d but the rows sum to %d", pooled.Net, sum)
		}
		if pooled.ConsecutiveNegative != 0 || pooled.Draining {
			t.Fatalf("dropping care on one object LATCHED a drain: consecutiveNegative %d draining %v. Nothing was paid out in this window; the alarm fired on a root leaving the list", pooled.ConsecutiveNegative, pooled.Draining)
		}

	})

	t.Run("entry does not mask a real drain", func(t *testing.T) {
		// ---- ARM 3: an entry must not MASK a real drain. Bounties are paid every step on the
		// long-lived root while a large new root joins. The drain must still latch.
		masking := b1Ring(
			map[string][2]int64{"0100": {1000, 0}},
			map[string][2]int64{"0100": {1000, 100}, "0200": {50_000, 0}},
			map[string][2]int64{"0100": {1000, 200}, "0200": {50_000, 0}},
			map[string][2]int64{"0100": {1000, 300}, "0200": {50_000, 0}},
		)
		pooled, _, _ := flowWindowRows(masking)
		if pooled.ConsecutiveNegative < flowDrainSamples || !pooled.Draining {
			t.Fatalf("a 50,000-credit new root MASKED a real 3-sample drain: consecutiveNegative %d draining %v, want >= %d and true", pooled.ConsecutiveNegative, pooled.Draining, flowDrainSamples)
		}

	})

	t.Run("the undisturbed drain still latches", func(t *testing.T) {
		// ---- ARM 4: the drain still latches with no entry at all, so arm 3 is not vacuous.
		plain := b1Ring(
			map[string][2]int64{"0100": {1000, 0}},
			map[string][2]int64{"0100": {1000, 100}},
			map[string][2]int64{"0100": {1000, 200}},
			map[string][2]int64{"0100": {1000, 300}},
		)
		if pooled, _, _ := flowWindowRows(plain); !pooled.Draining || pooled.Net != -300 {
			t.Fatalf("the undisturbed drain = %+v, want draining with net -300", pooled)
		}
	})
}

// TestR22PooledFlowOverTheRealCarePathOnAMidWindowEntry is arm 1 end to end: the real
// ledger, the real ring, the real handler. An escrow accrues from a serve whether or not
// this node caretakes the root, so caring for it later brings a fully-funded object into
// the window — exactly the shape the PE measured.
func TestR22PooledFlowOverTheRealCarePathOnAMidWindowEntry(t *testing.T) {
	s, led := statusServer(t)
	early := ports.Hash{0x01, 0xB1}
	late := ports.Hash{0x02, 0xB1}
	s.onLoop(func() {
		s.nd.Care(emptyRegistry{}, link.CareHandle{Root: early})
		led.RecordServeToObject(s.nd.ID(), ports.NodeID{0xC0}, early, ports.ChunkID{0x1}, 40*econMintUnit)
		// The LATE root's escrow fills BEFORE this node caretakes it.
		led.RecordServeToObject(s.nd.ID(), ports.NodeID{0xC0}, late, ports.ChunkID{0x2}, 5000*econMintUnit)
	})
	at := s.started
	economyRouteAt(t, s, "/api/economy/flows", at, true) // sample 0: `early` only
	s.onLoop(func() { s.nd.Care(emptyRegistry{}, link.CareHandle{Root: late}) })
	at = at.Add(flowSampleInterval)

	var doc struct {
		Pooled *struct {
			SkimIn int64 `json:"skimIn"`
			Net    int64 `json:"net"`
		} `json:"pooled"`
		Objects []struct {
			Root   string `json:"root"`
			SkimIn int64  `json:"skimIn"`
			Net    int64  `json:"net"`
		} `json:"objects"`
	}
	if err := json.Unmarshal([]byte(economyRouteAt(t, s, "/api/economy/flows", at, true)), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Pooled == nil || len(doc.Objects) != 2 {
		t.Fatalf("fixture is vacuous: %+v — the window needs both roots present in the last sample", doc)
	}
	var sum int64
	for _, o := range doc.Objects {
		sum += o.Net
	}
	if doc.Pooled.Net != sum {
		t.Fatalf("pooled net %d, rows sum %d. The late root's 5,000 credits of PRE-window inflow landed in this window: %+v", doc.Pooled.Net, sum, doc)
	}
	if doc.Pooled.Net != 0 {
		t.Fatalf("pooled net %d over a window in which no credit moved", doc.Pooled.Net)
	}
}
