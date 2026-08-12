package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/ports"
)

// TestLivePeerIsRetriedNotEvictedOnOneMiss is the direct behavior guard for the
// #288 / build-immutable #5 anti-pattern ("retry — don't evict a live peer on a
// single slow/dropped packet"). A full AST guard for retry/evict isn't tractable
// (retry counts and eviction are semantic policy, not a single construct — see the
// SCOPE NOTE in internal/wanguard), so the enforcement lives here, at the tier the
// bug lives: a dead peer given RequestRetries=2 must be DIALED 3 times (the initial
// attempt + 2 retries) before it is finally evicted. If eviction-on-the-first-miss
// ever regresses (the #288 shape that starves consensus under loss), the corpse is
// dialed only ONCE and this goes red. Counting dials over a run-to-completion is
// timing-independent, so unlike a mid-retry window assertion this cannot flake.
// (TestStaticPeerSurvivesReachabilityEviction286 is the post-exhaustion companion:
// after the retries here DO exhaust, a normal peer is correctly evicted.)
func TestLivePeerIsRetriedNotEvictedOnOneMiss(t *testing.T) {
	sched := simclock.New()
	ln := &linkNet{sched: sched, ends: map[ports.NodeID]*linkEnd{}}
	var meID, deadID ports.NodeID
	meID[0], deadID[0] = 1, 3

	dials := 0
	meEnd := &linkEnd{net: ln, id: meID}
	deadEnd := &linkEnd{net: ln, id: deadID}
	deadEnd.h = func(from ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgFindNode {
			dials++ // count each (re)send; never reply → the attempt can only time out
		}
	}
	ln.ends[meID], ln.ends[deadID] = meEnd, deadEnd

	cfg := DefaultConfig()
	cfg.RequestRetries = 2 // two retries remain after the first miss
	me := New(meID, cfg, sched, meEnd, memstore.New())
	me.table.Observe(deadID)

	me.request(deadID, ports.Message{Kind: ports.MsgFindNode, Target: deadID}, func(ports.Message, error) {})
	sched.Run() // run to quiescence: all retries fire, then the eviction branch

	if dials != cfg.RequestRetries+1 {
		t.Fatalf("#288: a good peer must be RETRIED before eviction, not dropped on one miss — "+
			"got %d dials, want %d (initial + %d retries). dials==1 means evict-on-one-miss regressed.",
			dials, cfg.RequestRetries+1, cfg.RequestRetries)
	}
	// Control: once the retries DO exhaust, a dead non-static peer is evicted.
	for _, p := range me.table.Closest(deadID, 20) {
		if p == deadID {
			t.Fatal("after retries exhausted, a dead non-static peer should be evicted from the table")
		}
	}
}
