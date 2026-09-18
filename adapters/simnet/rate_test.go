package simnet

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/ports"
)

// A LINK WITH A FINITE RATE, because without one a whole failure class cannot
// exist below the field tier.
//
// The mechanism those failures turn on is arithmetic: payload bytes ÷ link rate
// against a deadline. A latency-only network cannot express it — every message
// costs the same whatever its size, so the deadline is met or missed on RTT alone
// and a send queue has nowhere to form. These tests pin the model that makes the
// arithmetic reachable, and the ablation that matters is the default: with no rate
// configured the behaviour must be exactly what it was.

const rateNode = 1 << 20 // 1 MiB/s, so the numbers below are round

func rateWorld(t *testing.T, rate int64) (*simclock.Scheduler, *Network, ports.NodeID, ports.NodeID) {
	t.Helper()
	sched := simclock.New()
	cfg := Config{RateBytesPerSec: rate} // zero latency: the rate is the only term
	net := New(sched, 1, cfg)
	a := ports.HashBytes([]byte("A"))
	b := ports.HashBytes([]byte("B"))
	net.Endpoint(a)
	net.Endpoint(b)
	return sched, net, a, b
}

// One message costs its own transfer time and nothing else.
func TestRateChargesSerializationTime(t *testing.T) {
	sched, net, a, b := rateWorld(t, rateNode)
	var at ports.Time
	net.Endpoint(b).SetHandler(func(ports.NodeID, ports.Message) { at = sched.Now() })

	// Half a MiB at 1 MiB/s is half a second, exactly.
	if err := net.Endpoint(a).Send(b, ports.Message{Data: make([]byte, rateNode/2)}); err != nil {
		t.Fatalf("send: %v", err)
	}
	sched.Run()
	if want := ports.Time(500 * ports.Millisecond); at != want {
		t.Fatalf("a %d-byte message on a %d B/s link arrived at %v, want %v — the link is not charging "+
			"for SIZE, so payload-versus-rate arithmetic cannot be expressed here", rateNode/2, rateNode, at, want)
	}
}

// THE ARM THAT MATTERS: a message waits behind what is already on the link. This
// is head-of-line waiting, and it is the half a size-scaled per-message delay
// would miss — the small message that matters is late because of bulk it has
// nothing to do with.
func TestRateQueuesBehindBulkOnTheSameLink(t *testing.T) {
	sched, net, a, b := rateWorld(t, rateNode)
	var small ports.Time
	net.Endpoint(b).SetHandler(func(_ ports.NodeID, m ports.Message) {
		if len(m.Data) < 100 {
			small = sched.Now()
		}
	})

	// Two MiB of bulk, then a 10-byte message: the small one cannot start until the
	// bulk has drained, so it arrives at ~2s and not at ~0.
	if err := net.Endpoint(a).Send(b, ports.Message{Data: make([]byte, 2*rateNode)}); err != nil {
		t.Fatalf("send bulk: %v", err)
	}
	if err := net.Endpoint(a).Send(b, ports.Message{Data: make([]byte, 10)}); err != nil {
		t.Fatalf("send small: %v", err)
	}
	sched.Run()
	if small < ports.Time(2*ports.Second) {
		t.Fatalf("a 10-byte message behind 2 MiB of bulk on a 1 MiB/s link arrived at %v, before the bulk "+
			"could have finished (2s). The link is charging per message but not QUEUING, so a producer that "+
			"outruns the wire never makes anything wait — which is the whole shape this model exists for", small)
	}
}

// A SEPARATE LINK IS NOT CHARGED. Without this the queue would be a global
// bottleneck rather than a per-link one, and every scenario with unrelated traffic
// would slow down for no modelled reason.
func TestRateQueueIsPerLinkNotGlobal(t *testing.T) {
	sched := simclock.New()
	net := New(sched, 1, Config{RateBytesPerSec: rateNode})
	a := ports.HashBytes([]byte("A"))
	b := ports.HashBytes([]byte("B"))
	c := ports.HashBytes([]byte("C"))
	for _, id := range []ports.NodeID{a, b, c} {
		net.Endpoint(id)
	}
	var onC ports.Time
	net.Endpoint(c).SetHandler(func(ports.NodeID, ports.Message) { onC = sched.Now() })
	net.Endpoint(b).SetHandler(func(ports.NodeID, ports.Message) {})

	if err := net.Endpoint(a).Send(b, ports.Message{Data: make([]byte, 4*rateNode)}); err != nil {
		t.Fatalf("send bulk a→b: %v", err)
	}
	if err := net.Endpoint(a).Send(c, ports.Message{Data: make([]byte, 10)}); err != nil {
		t.Fatalf("send small a→c: %v", err)
	}
	sched.Run()
	if onC > ports.Time(ports.Millisecond) {
		t.Fatalf("a small message on the a→c link arrived at %v, delayed by 4 MiB of bulk on the a→b link — "+
			"the queue is global, not per-link, so unrelated traffic is being made to wait", onC)
	}
}

// THE DEFAULT IS UNCHANGED, and this is the arm that protects every scenario
// written before the model existed: with no rate configured, size costs nothing.
func TestNoRateConfiguredChargesNothingForSize(t *testing.T) {
	sched := simclock.New()
	net := New(sched, 1, Config{}) // no latency, no rate — the historical default
	a := ports.HashBytes([]byte("A"))
	b := ports.HashBytes([]byte("B"))
	net.Endpoint(a)
	net.Endpoint(b)
	var at ports.Time
	net.Endpoint(b).SetHandler(func(ports.NodeID, ports.Message) { at = sched.Now() })

	if err := net.Endpoint(a).Send(b, ports.Message{Data: make([]byte, 64<<20)}); err != nil {
		t.Fatalf("send: %v", err)
	}
	sched.Run()
	if at != 0 {
		t.Fatalf("a 64 MiB message on a network with NO rate configured arrived at %v, want 0 — the rate "+
			"model is not opt-in, and every scenario written against atomic delivery has silently changed", at)
	}
}
