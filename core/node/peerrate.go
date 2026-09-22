package node

import "github.com/nerolabs/silt/ports"

// A per-peer estimate of the transfer rate a request deadline is sized against.
//
// WHY THIS EXISTS. requestTimeoutFor extends a request's deadline by
// len(payload)/RequestSizeFloorBytesPerSec — a constant standing in for the path's
// real rate. Build-immutable #5 asks for the opposite of a constant: a per-peer
// deadline sized to the worst real path, scaled to payload size. On a link BELOW that
// constant the difference is not a slow request, it is a request that can never
// succeed: a 1,514,986 B bond registration is given 6.28 s at the 256 KiB/s floor and
// needs 23.1 s at 64 KiB/s, so its reply is declared lost while its bytes are still
// crossing. The bytes arrive and are queued; only the receipt for them is destroyed.
//
// That receipt is what the digest relay sheds on, and what tells a renewal it has been
// delivered. Measured over four validators on a wire a quarter of the assumed floor:
// 860 of 876 registrations arrived and were queued by their receivers, 864
// acknowledgements expired, relay coverage read 33%, and the renewal path re-sent the
// full proof to every peer on every sweep because nothing recorded that it had landed.
// The chain reached height 1 and stayed there. See
// bondreg_relay_coverage_measure_test.go.
//
// ONE SIGNAL, ONE JOB (build-immutable #3), AND THERE ARE TWO SIGNALS HERE. A reply
// that does not arrive has two unrelated causes — a link too slow to carry the payload,
// and a packet or peer that is simply gone — and they want opposite responses. They are
// kept in separate state deliberately:
//
//   - measured is the PATH, sampled only from exchanges that COMPLETED. A completion is
//     the only observation that measures anything, it is min-filtered to its floor, and
//     it persists because a path's rate is a property of the path.
//   - backoff is the response to LOSS. It is not a measurement and it does not pretend
//     to be one: it is bounded, each step is paid for by a reply, and it is released
//     whole by the first completed exchange of comparable size. Kept in the measurement
//     window instead it would be indistinguishable from a measurement, and a peer that
//     had simply gone away would become permanently maximally patient — every later
//     exchange with a corpse waiting the size-extension cap, which spends a consensus
//     backstop's whole budget on a peer that will never answer.
//
// NEITHER SIGNAL EVER REACHES A SECURITY GATE. Both size a transport deadline and
// nothing else — never standing, eligibility, or quorum — and both are kept apart from
// peerBondRTT, which measures a different quantity for a different reader. A peer that
// presents a slow path buys itself patience and nothing more.
//
// AND THE DEADLINE ONLY EVER LENGTHENS. The effective rate is the SMALLER of the
// configured floor and what this peer has shown, so a peer on a fast path is treated
// exactly as it is today. requestSizeExtensionCap still bounds the total, so a peer
// presenting an arbitrarily slow path can hold a request open for the same 30 s it can
// hold one open for now.

// rateWindowSize is how many completion samples a peer's window keeps. Same size and
// same minimum-filter idiom as latWindow: a noisy signal is read at its floor rather
// than trusted on one sample (build-immutable #5). For a RATE the conservative floor is
// the SLOWEST observation, which is the one that produces the most generous deadline.
const rateWindowSize = 8

// rateSampleMinBytes is the payload below which an exchange measures round-trip latency
// rather than transfer rate, and so is not sampled. At the 256 KiB/s floor this is
// 250 ms of transfer — comfortably above a WAN round trip, so a sample is dominated by
// the quantity it claims to measure.
const rateSampleMinBytes = 64 << 10

// maxRateBackoffShifts bounds how far consecutive expiries may halve a peer's assumed
// rate: three shifts, so a deadline may be sized against as little as an eighth of the
// configured floor. That covers the measured wedge with margin — the link there is a
// quarter of the floor — and stops short of a runaway that would make a departed peer
// maximally patient. Past this the size-extension cap is the bound.
const maxRateBackoffShifts = 3

// rateWindow is a small ring of a peer's recent completed transfer rates in bytes per
// second, read at its minimum.
type rateWindow struct {
	samples [rateWindowSize]int64
	n       int // total observed (the ring index is n%size)
}

func (w *rateWindow) observe(bytesPerSec int64) {
	if bytesPerSec < 1 {
		bytesPerSec = 1
	}
	w.samples[w.n%rateWindowSize] = bytesPerSec
	w.n++
}

// min returns the slowest rate in the window — the floor estimate.
func (w *rateWindow) min() int64 {
	lim := rateWindowSize
	if w.n < lim {
		lim = w.n
	}
	if lim == 0 {
		return 0
	}
	m := w.samples[0]
	for i := 1; i < lim; i++ {
		if w.samples[i] < m {
			m = w.samples[i]
		}
	}
	return m
}

// peerPath is what one peer has shown about its path: what it achieved when it
// answered, and how many times running it has not.
type peerPath struct {
	measured rateWindow
	backoff  uint // expiries taken, released by a completed exchange of comparable size
	// paidBy is the reply that bought the current backoff level. Each further step
	// needs a NEWER one, so a peer that has stopped answering cannot keep climbing.
	paidBy ports.Time
}

// sizeFloorFor reports the rate to size to's deadline against: the configured floor,
// reduced by whatever this peer has shown — its measured floor when that is slower, and
// its current loss backoff. Never faster than the configured value, so this can only
// ever grant more time than the constant does.
func (n *Node) sizeFloorFor(to ports.NodeID) int64 {
	rate := n.cfg.RequestSizeFloorBytesPerSec
	p := n.peerRate[to]
	if p == nil {
		return rate
	}
	if m := p.measured.min(); m > 0 && m < rate {
		rate = m
	}
	rate >>= p.backoff
	if rate < 1 {
		rate = 1 // the size-extension cap bounds the resulting deadline, not this
	}
	return rate
}

// observeRequestRate records what one COMPLETED exchange achieved: payload bytes over
// the wall time the exchange took. This is the only sample that measures the path.
//
// A LOWER BOUND ON THE LINK, WHICH IS THE SAFE DIRECTION. elapsed also covers the
// peer's own work and the reply's flight, so payload/elapsed understates the link — and
// an understated rate produces a longer deadline, never a shorter one.
func (n *Node) observeRequestRate(to ports.NodeID, payload int64, elapsed ports.Duration) {
	if payload < rateSampleMinBytes || elapsed <= 0 {
		return
	}
	if p := n.pathFor(to); p != nil {
		p.measured.observe(payload * int64(ports.Second) / int64(elapsed))
		// AND THE BACKOFF IS RELEASED HERE, not on any reply at all. The backoff stands
		// in for not knowing whether a payload of this size fits; a completed exchange of
		// this size answers that question directly, and `measured` now carries the
		// answer. Releasing on a small reply instead would be the category error — a
		// 121-byte probe returning says nothing about whether 1.5 MB fits, and on a
		// congested link those small replies arrive often enough to reset the backoff
		// before it can ever reach the path's real rate.
		p.backoff, p.paidBy = 0, 0
	}
}

// observeRequestTimeout records that an exchange EXPIRED, which is what lets this reach
// a link that is already failing.
//
// A completed request measures the path. A timed-out one does not: it proves only that
// the achieved rate was BELOW payload/deadline, with no measurement of how far below and
// no way to tell a slow link from a departed peer. RFC 6298 section 5.5 answers exactly
// that — when a retransmission timer expires, double the timeout — so an expiry halves
// the assumed rate for the next attempt, and the deadline walks up geometrically until
// it covers the path or maxRateBackoffShifts stops it.
//
// WITHOUT THIS THE ESTIMATOR CANNOT BOOTSTRAP where it is needed most: on a link well
// below the assumed floor, the large exchanges that would measure it are precisely the
// ones that never complete, so a completion-only estimator would learn nothing from the
// failure it exists to fix.
//
// IT IS NOT A MEASUREMENT AND IS NOT KEPT AS ONE. observeRequestRate releases it whole
// the next time an exchange of comparable size COMPLETES — RFC 6298 section 5.3's rule
// that a fresh measurement replaces a backed-off timer. By then `measured` carries the
// real figure, so the deadline stays sized to the path rather than snapping back under
// it.
//
// AND EACH STEP MUST BE PAID FOR BY A FRESH REPLY, which is the discriminator that
// stops this from being one signal serving two masters (build-immutable #3). An expiry
// alone cannot tell a slow link from a departed peer, and the two want opposite
// responses: more patience for the first, eviction for the second.
//
// So a step is taken only when the peer has answered something ELSE since the reply
// that bought the current level. A peer that is slow but serving keeps supplying those
// replies and can climb to the bound; a peer that has gone away supplies none, so it
// takes at most one step after its last reply and then stops. The rule is a comparison
// between two local observations rather than a freshness threshold, so it adds no second
// constant standing in for the path.
//
// Unpaid, the backoff climbs to its bound against a peer that has died and then holds
// there: a dead peer completes nothing, so nothing releases it, and every later exchange
// with the corpse waits the size-extension cap. A consensus backstop sized for a retry
// ladder no longer has room for one. Eviction is the answer to a corpse; patience is
// the answer to a slow link, and this rule is what keeps them apart.
func (n *Node) observeRequestTimeout(to ports.NodeID, payload int64) {
	if payload < rateSampleMinBytes {
		return
	}
	answered, ok := n.reachable[to]
	if !ok {
		return // this peer has never answered anything; a corpse is not a slow link
	}
	p := n.pathFor(to)
	if p == nil || p.backoff >= maxRateBackoffShifts {
		return
	}
	if p.backoff > 0 && answered <= p.paidBy {
		return // nothing new from this peer since the step it already bought
	}
	p.backoff++
	p.paidBy = answered
}

func (n *Node) pathFor(to ports.NodeID) *peerPath {
	if n.peerRate == nil {
		return nil
	}
	// Bounded like every other peer-keyed cache: a flood of distinct NodeIDs must not
	// grow this without bound. Soft-cache semantics — an evicted peer's history is
	// re-learned from its next sized exchange, and until then it is sized against the
	// configured floor, which is today's behaviour.
	evictPeerInfoIfFull(n.peerRate, to)
	p := n.peerRate[to]
	if p == nil {
		p = &peerPath{}
		n.peerRate[to] = p
	}
	return p
}
