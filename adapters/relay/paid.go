package relay

// PoD §7.3 transport Batch 2 — the paid forwarding pump (step 4, design §2).
//
// A paid relay session forwards a ≤1 GiB object toward a fetcher as-it-goes: the
// relay forwards increment k only after the fetcher authorizes it (a preimage
// reveal on the out-of-band payment channel, which the NODE verifies and turns
// into a rising authorized-byte ceiling — design §2 Option A: the node drives, the
// adapter gates). This file is the adapter-side byte pump. It knows NOTHING about
// PayWord or preimages; it only asks the authorizer "how many bytes am I cleared to
// forward" and blocks until that ceiling rises. Keeping the verifier in core/node
// keeps the M0 guards and the #644 S-clamp on the tested path (design §2 Option A).
//
// TWO PROPERTIES this pump must hold that the plain splice does NOT:
//
//   - PAY-AS-YOU-GO GATE. The forward direction never delivers more than the
//     authorized ceiling. If the fetcher stops authorizing, the pump blocks; the
//     irreducible stiff is bounded to one increment (the origin may have one
//     increment already buffered in flight).
//
//   - SPLICE-EOF SURVIVAL (the sharpest hazard). The plain splice closes BOTH
//     conns on the FIRST EOF because swarm exchanges are short-lived (server.go).
//     A paid session is NOT short-lived: the reverse direction (fetcher → origin)
//     may EOF while the paid forward still owes authorized bytes. The paid pump
//     must NOT tear down the forward stream on a reverse EOF. The forward stream
//     ends on its OWN completion (origin EOF, the byte cap, or the fetcher hanging
//     up its read side), never on the reverse direction closing.

import (
	"io"
	"net"

	"github.com/nerolabs/silt/ports"
)

// authorizer is the seam the node fills: it reports how many forward bytes the
// relay is cleared to deliver, and lets the pump wait for the ceiling to rise.
// The node raises the ceiling on each verified MsgRelayPay; Done reports the
// session is closing (no more authorization will ever come) so the pump can stop
// blocking and drain to its current ceiling.
type authorizer interface {
	// AuthorizedBytes is the current forward-byte ceiling (count × increment).
	AuthorizedBytes() int64
	// Wait returns a channel signalled when the ceiling MAY have advanced or the
	// session closed. A spurious wake is fine — the pump re-reads AuthorizedBytes.
	Wait() <-chan struct{}
	// Done reports the session is closing: no further authorization will arrive.
	Done() bool
}

// paidPump forwards bytes from src (the origin) to dst (the fetcher), never
// exceeding the authorizer's current ceiling and never exceeding maxBytes (the
// hard 1 GiB session cap that bounds S — design §3). It returns the total bytes
// forwarded (the settlement basis) when src reaches EOF, the cap is hit, or a
// write fails. It does NOT close either conn — the caller owns conn lifetimes so a
// reverse EOF cannot tear down this forward stream.
//
// The gate: the pump reads a chunk from src, then before WRITING it waits until
// the authorized ceiling covers the bytes about to be delivered. So the origin can
// stream ahead into the relay's buffer, but the relay releases bytes to the fetcher
// only up to what the fetcher has paid for. The stiff is bounded to one buffered
// chunk (≤ one increment on a well-sized read).
func paidPump(dst io.Writer, src io.Reader, auth authorizer, maxBytes int64) int64 {
	const chunk = 4096 // one increment; the pump releases at increment granularity
	buf := make([]byte, chunk)
	var forwarded int64
	for forwarded < maxBytes {
		toRead := int64(chunk)
		if rem := maxBytes - forwarded; rem < toRead {
			toRead = rem
		}
		n, rerr := src.Read(buf[:toRead])
		if n > 0 {
			// Gate: do not deliver these n bytes until the ceiling covers them.
			target := forwarded + int64(n)
			for auth.AuthorizedBytes() < target {
				if auth.Done() {
					return forwarded // session closing; deliver no unpaid bytes
				}
				<-auth.Wait()
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return forwarded
			}
			forwarded += int64(n)
		}
		if rerr != nil {
			return forwarded // origin EOF or error: the forward stream is complete
		}
	}
	return forwarded
}

// reversePump forwards the reverse direction (fetcher → origin) UNGATED — it is a
// thin control/ack lane, not a paid stream. Crucially it does NOT close either
// conn on EOF (the splice-EOF gotcha): when the fetcher stops sending on the
// reverse direction, this pump simply returns and the PAID forward stream keeps
// running. This is the exact behavior the plain splice gets wrong.
func reversePump(dst io.Writer, src io.Reader, maxBytes int64) {
	io.Copy(dst, io.LimitReader(src, maxBytes))
}

// Authorizer is the exported seam the node fills to drive a paid splice: it
// reports the forward-byte ceiling the relay is cleared to deliver and lets the
// pump wait for it to rise. node.RelaySession implements it (design §2 Option A:
// the node owns the verifier and the M0 guards; the adapter is a dumb byte pump
// that only knows how many bytes it is cleared to forward).
type Authorizer = authorizer

// SplicePaid runs a paid relay session between the origin conn (a) and the fetcher
// conn (b), gated by auth (the node-owned authorizer). It is the paid sibling of
// splice: the forward direction a→b is pay-as-you-go, and — unlike splice — a
// reverse-direction EOF does NOT tear down the paid forward stream. It returns the
// forward bytes delivered (the settlement basis). The caller settles that count
// once at close (node.SettleRelaySession).
func (s *Server) SplicePaid(target ports.NodeID, a, b net.Conn, auth Authorizer) int64 {
	s.logf(ports.LogInfo, "relay paid splice", "target", target)
	forwarded := paidSession(a, b, auth, s.cfg.MaxSessionBytes)
	s.done(target)
	return forwarded
}

// paidSession runs a full bidirectional paid session between the origin conn (a)
// and the fetcher conn (b): the forward direction a→b is PAID (gated by auth), the
// reverse direction b→a is an ungated control lane. It returns the number of
// forward bytes delivered (the settlement basis).
//
// Unlike splice, this does NOT close both conns on the first EOF. The reverse
// direction ending must not truncate the forward object. The forward pump owns the
// session's end: when it returns (origin EOF, byte cap, or the fetcher's read side
// gone), the session is done and both conns close. A reverse EOF only ends the
// reverse pump.
func paidSession(a, b net.Conn, auth authorizer, maxBytes int64) int64 {
	// Reverse lane: fetcher (b) → origin (a), ungated, does NOT tear down the
	// forward direction when it EOFs.
	go reversePump(a, b, maxBytes)
	// Forward lane: origin (a) → fetcher (b), PAID. This call owns the session end.
	forwarded := paidPump(b, a, auth, maxBytes)
	// The forward stream is complete: now tear the session down. Closing here (not
	// on the reverse EOF) is what makes a paid session survive a reverse EOF.
	a.Close()
	b.Close()
	return forwarded
}
