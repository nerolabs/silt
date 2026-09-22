package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// THE DETERMINISTIC REPRO of the payload-versus-rate failure, at the tier the bug
// lives at rather than the field.
//
// requestTimeoutFor size-extends a request's per-attempt deadline by
// len(payload) / RequestSizeFloorBytesPerSec — an ASSUMED floor, not a measured
// rate. When the wire delivers below that floor the deadline is under-sized by
// construction, and the bigger the payload the wider the gap. In the field this
// showed up as consensus proposals carrying a ~1.5 MB space-time bond proof timing
// out on every attempt of the retry ladder while every validator was up and
// willing.
//
// It could not be reproduced below the field tier because simnet had no bandwidth:
// delivery was atomic after a latency draw, so payload size cost nothing and the
// arithmetic had nowhere to bite. With a rate on the link it is ordinary unit-tier
// arithmetic, and these two tests are the same request on either side of the floor.
func payloadRepro(t *testing.T, linkRate int64) (*Node, ports.NodeID, *simclock.Scheduler) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.Config{RateBytesPerSec: linkRate})

	me := identity.FromSeed(4001)
	peer := identity.FromSeed(4002)
	cfg := DefaultConfig()
	cfg.RequestTimeout = 8 * ports.Second
	cfg.RequestRetries = 0 // one attempt: this measures the DEADLINE, not the ladder
	cfg.RequestSizeFloorBytesPerSec = 256 << 10

	nd := New(me.NodeID(), cfg, sched, net.Endpoint(me.NodeID()), memstore.New())
	// A peer that answers instantly, so the only cost in play is the wire.
	pe := net.Endpoint(peer.NodeID())
	pe.SetHandler(func(from ports.NodeID, msg ports.Message) {
		pe.Send(from, ports.Message{Kind: ports.MsgAttestReply, RID: msg.RID, OK: true})
	})
	nd.table.Observe(peer.NodeID())
	return nd, peer.NodeID(), sched
}

// payloadBytes is sized so the arithmetic is unambiguous at the shipped floor:
// 1.5 MiB / 256 KiB/s = 6 s of extension on top of the 8 s base, so the deadline is
// 14 s. It is the same order as the real bond proof that produced the field failure.
const payloadBytes = 3 << 19 // 1.5 MiB

// BELOW THE FLOOR. The interesting number is not "below" but HOW FAR below, and
// working it out is the point of the exercise:
//
//	deadline  = base + payload/floor = 8 s + 1.5 MiB / 256 KiB/s = 14 s
//	transfer  = payload / actual rate
//
// so the request fits whenever actual > payload/deadline = 112 KiB/s — which is
// BELOW the floor. The 8 s base absorbs a modest shortfall, and a first cut of this
// test used half the floor (128 KiB/s ⇒ 12 s) and passed for exactly that reason.
// The margin is not constant, though: it is base/(base+extension) of the floor, so
// it SHRINKS as the payload grows. At 1.5 MiB the wire may run 43% slow; at 15 MiB
// the extension caps at 30 s and the tolerance collapses. This drives a quarter of
// the floor — 24 s of transfer against a 14 s deadline, unambiguous.
func TestLargePayloadTimesOutOnALinkBelowTheAssumedFloor(t *testing.T) {
	nd, peer, sched := payloadRepro(t, 64<<10) // a quarter of the 256 KiB/s floor

	var gotErr error
	var fired bool
	nd.request(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payloadBytes)},
		func(_ ports.Message, err error) { fired, gotErr = true, err })
	sched.Run()

	if !fired {
		t.Fatal("the request callback never fired at all — the rig is wrong and neither arm means anything")
	}
	if gotErr == nil {
		t.Fatalf("a %d-byte request crossed a link at a QUARTER of the assumed floor INSIDE its deadline. "+
			"The deadline is 8s + payload/%d B/s = 14s and the wire needs 24s one way, so this cannot fit — "+
			"check whether the link is charging for SIZE before believing the pass.",
			payloadBytes, nd.cfg.RequestSizeFloorBytesPerSec)
	}
}

// THE CONTROL, on the same payload and the same deadline: a link AT the assumed
// floor delivers inside the budget. Without this the arm above would pass on a
// network that simply never delivers anything.
func TestTheSamePayloadSucceedsOnALinkAtTheAssumedFloor(t *testing.T) {
	nd, peer, sched := payloadRepro(t, 256<<10) // exactly the floor

	var gotErr error
	var fired bool
	nd.request(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payloadBytes)},
		func(_ ports.Message, err error) { fired, gotErr = true, err })
	sched.Run()

	if !fired {
		t.Fatal("the request callback never fired at all — the rig is wrong and neither arm means anything")
	}
	if gotErr != nil {
		t.Fatalf("the same %d-byte request FAILED on a link at exactly the assumed floor: %v. The size "+
			"extension exists to make precisely this case fit, so a failure here is the deadline being "+
			"under-sized even when the wire delivers what it assumes.", payloadBytes, gotErr)
	}
}
