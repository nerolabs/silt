package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// THE DEFECT, AT THE TIER IT LIVES AT. A request's deadline was extended by
// len(payload)/RequestSizeFloorBytesPerSec — a constant. On a path slower than that
// constant the deadline is not merely tight, it is unachievable: the exchange expires
// while its bytes are still crossing, so it can never succeed however many times it
// is retried, and the receipt it would have produced is lost. For a bond registration
// that receipt is what the digest relay sheds on and what tells the renewal path it
// has been delivered, so losing it both carries ~1.5 MB on every gather leg and
// re-sends the same proof to every peer on every sweep.
//
// These pin the four properties the fix rests on. Each fails on the constant-floor
// behaviour: the first two because nothing observed ever changed a deadline, the
// third and fourth because they are the bounds that make widening it safe.
func TestARequestDeadlineIsSizedByThePeersObservedPathNotAConstant(t *testing.T) {
	const payload = 1_572_864

	// answered records one reply from the peer to something else — the independent
	// evidence that it is there and serving while a larger request of ours expires.
	// Each backoff step has to be paid for by a NEWER one, so a live peer supplies a
	// fresh reply per step and a departed one supplies none.
	answered := func(n *Node, peer ports.NodeID, at ports.Time) {
		n.reachable[peer] = at
	}

	t.Run("an expiry teaches the next attempt", func(t *testing.T) {
		n, _ := aloneNode(t, 0)
		peer := ports.NodeID{9}

		before, sized := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		if sized != payload {
			t.Fatalf("the deadline must report the payload it was sized from, got %d", sized)
		}

		// The only sample a failing link ever offers: this payload did not cross inside
		// this deadline, from a peer that answered something else meanwhile. RFC 6298
		// section 5.5 answers "how much slower?" by doubling.
		answered(n, peer, 1)
		n.observeRequestTimeout(peer, sized)

		after, _ := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		if after <= before {
			t.Fatalf("a request that expired must lengthen the next attempt's deadline for that peer: "+
				"%v before, %v after. Without this the estimator cannot bootstrap on the only link that "+
				"needs it — below the assumed floor, the exchanges large enough to measure the path are "+
				"exactly the ones that stop completing.", before, after)
		}
	})

	t.Run("it lengthens for one peer only", func(t *testing.T) {
		n, _ := aloneNode(t, 0)
		slow, fast := ports.NodeID{1}, ports.NodeID{2}

		base, sized := n.requestTimeoutFor(slow, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		answered(n, slow, 1)
		n.observeRequestTimeout(slow, sized)

		got, _ := n.requestTimeoutFor(fast, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		if got != base {
			t.Fatalf("one peer's slow path must not stretch another peer's deadline: %v vs the %v a peer "+
				"with no history gets. A per-peer estimate that leaked between peers would be a global "+
				"deadline raise wearing a per-peer name.", got, base)
		}
	})

	t.Run("it never shortens below the configured floor", func(t *testing.T) {
		n, _ := aloneNode(t, 0)
		peer := ports.NodeID{3}

		base, sized := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		// A path far FASTER than the configured floor, observed repeatedly.
		for i := 0; i < 2*rateWindowSize; i++ {
			n.observeRequestRate(peer, sized, ports.Millisecond)
		}
		got, _ := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		if got != base {
			t.Fatalf("a fast peer must keep the configured floor's deadline (%v), got %v. The estimate is "+
				"allowed to grant more time than the constant and never less: shortening a deadline on "+
				"observed speed would evict a live peer the moment its path hiccupped.", base, got)
		}
	})

	t.Run("a completed exchange of the same size releases the loss backoff", func(t *testing.T) {
		n, _ := aloneNode(t, 0)
		peer := ports.NodeID{6}

		base, sized := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		for i := 0; i < maxRateBackoffShifts; i++ {
			answered(n, peer, ports.Time(i+1))
			n.observeRequestTimeout(peer, sized)
		}
		backedOff, _ := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		if backedOff <= base {
			t.Fatalf("expiries must back the deadline off first: %v then %v", base, backedOff)
		}

		// A COMPLETED exchange of comparable size, faster than the configured floor
		// assumes: the size assumption is confirmed, so the backoff it stood in for is
		// released and nothing is left to lengthen the deadline.
		n.observeRequestRate(peer, sized, base/2)
		got, _ := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		if got != base {
			t.Fatalf("a completed exchange of comparable size must release the loss backoff whole (%v), got %v. An expiry is not a "+
				"measurement — it cannot tell a slow link from a departed peer — so holding it makes a peer "+
				"that merely went quiet permanently maximally patient, and every later exchange with it "+
				"waits the cap. Held as a measurement, this is what turned a consensus liveness oracle red.",
				base, got)
		}
	})

	t.Run("a measured path survives a reply", func(t *testing.T) {
		n, _ := aloneNode(t, 0)
		peer := ports.NodeID{7}

		base, sized := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		// A completion IS a measurement of the path, so unlike a backoff it persists:
		// the exchange took four times as long as the configured floor assumes.
		n.observeRequestRate(peer, sized, 4*base)

		got, _ := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		if got <= base {
			t.Fatalf("a measured slow path must keep lengthening the deadline after a reply (%v), got %v. "+
				"Releasing the measurement along with the backoff would put the deadline straight back "+
				"under the path that was just observed not to fit it.", base, got)
		}
	})

	t.Run("the cap still bounds a peer claiming any slowness", func(t *testing.T) {
		n, _ := aloneNode(t, 0)
		peer := ports.NodeID{4}

		// Expire far past the shift bound AND present a measured path of one byte per
		// second, so both terms push for an unbounded deadline at once.
		for i := 0; i < 40; i++ {
			answered(n, peer, ports.Time(i+1))
			_, sized := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
			n.observeRequestTimeout(peer, sized)
			n.observeRequestRate(peer, sized, ports.Duration(sized)*ports.Second)
		}
		got, _ := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		if want := n.cfg.RequestTimeout + requestSizeExtensionCap; got != want {
			t.Fatalf("a peer presenting an unboundedly slow path must still be bounded by the size-extension "+
				"cap (%v), got %v. The cap is what keeps a peer from holding a round open for longer than "+
				"it already can, and it is the reason widening the deadline buys an adversary nothing.", want, got)
		}
	})

	t.Run("a peer that never answered gets no extra patience", func(t *testing.T) {
		n, _ := aloneNode(t, 0)
		peer := ports.NodeID{8}

		base, sized := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		// Expiry after expiry, and nothing else from this peer the whole time: a corpse
		// looks exactly like a slow link from here, and the two want opposite responses.
		for i := 0; i < 2*maxRateBackoffShifts; i++ {
			n.observeRequestTimeout(peer, sized)
		}
		got, _ := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		if got != base {
			t.Fatalf("a peer that has answered nothing must keep the configured floor's deadline (%v), "+
				"got %v. An expiry on its own cannot separate a slow link from a departed peer; without "+
				"the liveness evidence, backing off makes a corpse maximally patient and spends a "+
				"consensus backstop's whole budget waiting on it. Eviction is the answer to a corpse, "+
				"not patience.", base, got)
		}
	})

	t.Run("a small exchange measures latency, not rate, and is not sampled", func(t *testing.T) {
		n, _ := aloneNode(t, 0)
		peer := ports.NodeID{5}

		base, _ := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		// A tiny exchange that took a long time says the peer was busy, not that the
		// link is slow. Sampling it would let one slow reply stretch every large
		// request to that peer.
		n.observeRequestRate(peer, rateSampleMinBytes-1, 10*ports.Second)
		answered(n, peer, 1)
		n.observeRequestTimeout(peer, rateSampleMinBytes-1)

		got, _ := n.requestTimeoutFor(peer, ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payload)})
		if got != base {
			t.Fatalf("an exchange below %d B measures round-trip latency rather than transfer rate and must "+
				"not enter the estimate: deadline moved %v -> %v", rateSampleMinBytes, base, got)
		}
	})
}
