package tcpnet

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/nerolabs/silt/ports"
)

// PIECED FRAMING: one ordered connection that a small frame can still get through.
//
// The control lane (ctrllane.go) takes small frames off the bulk connection, but only
// where a second connection can be dialled. A NATed peer, and any peer reached through
// a relay, has exactly one ordered stream, and on it a 121-byte probe still waits
// behind the whole of a multi-megabyte proof.
//
// On a pieced connection a frame larger than pieceBytes is written as a sequence of
// pieces, and a small frame queued behind it is written at the next piece boundary
// instead of after the last byte. The small frame then waits behind at most one piece
// plus what the kernel already holds, rather than behind the payload.
//
// THE KERNEL HOLDS THE REST OF THE QUEUE, so interleaving in user space alone does
// not reach the wire. A socket send buffer autotunes to hundreds of KiB or more, and
// everything accepted into it is ahead of any frame written later. TCP_NOTSENT_LOWAT
// bounds the bytes the kernel holds that have NOT yet been sent, so the writer blocks
// in user space — where a small frame can still overtake it — instead of in the
// kernel. Unlike shrinking SO_SNDBUF, it does not cap the bytes in flight, so it costs
// a fast link no throughput.
//
// An application-level acknowledgement window would also bound the queue, but it caps
// throughput at window ÷ RTT — 32 KiB over a 100 ms path is 320 KiB/s — on every link,
// including the fast ones the storage plane depends on.
//
// WHAT IT DOES NOT DO. It does not bound bytes already in flight: on a congested path
// those sit in the network's own queues, and a small frame written after them still
// arrives after them. That is what the control lane avoids and this cannot.
//
// THE WIRE FORMAT is the 4-byte length prefix with its top two bits used as flags,
// which maxFrame leaves free:
//
//	len                 a whole frame, exactly as on an unpieced connection
//	pieceStart | total  a pieced frame of total bytes begins; no payload follows
//	pieceNext  | len    the next len bytes of the frame in progress
//
// At most one pieced frame is in progress per connection, so pieces need no stream
// identifier: the sender serializes pieced frames, and the receiver appends every
// piece to the one frame it is assembling.
//
// It is negotiated, never assumed. The bulk dial offers alpnBulkPieced first, and a
// peer that does not know it negotiates alpnBulk and gets the unpieced format.
const (
	alpnBulkPieced = "silt/2"

	pieceStart = uint32(1) << 31
	pieceNext  = uint32(1) << 30
	pieceFlags = pieceStart | pieceNext

	// pieceBytes is the largest frame written whole and the size of each piece of a
	// larger one. It matches a TLS record, so a piece costs no extra record overhead.
	pieceBytes = 16 << 10

	// notSentLowat is the unsent backlog the kernel may hold on a pieced connection:
	// two pieces, so the socket always has the next piece ready while the writer
	// decides what follows it.
	notSentLowat = 2 * pieceBytes
)

// maxFrame must leave the flag bits clear, or a whole frame's length would read as
// a flag.
var _ = [1]struct{}{}[maxFrame>>30]

// isPiecedConn reports whether a completed handshake negotiated pieced framing.
func isPiecedConn(cs tls.ConnectionState) bool { return cs.NegotiatedProtocol == alpnBulkPieced }

// newPeerConn wraps conn for writing, in whichever framing its handshake negotiated.
func newPeerConn(conn *tls.Conn, viaRelay bool) *peerConn {
	pc := &peerConn{conn: conn, viaRelay: viaRelay}
	if isPiecedConn(conn.ConnectionState()) {
		pc.pieced = true
		// Best effort: a platform or socket without the option keeps a larger
		// kernel queue, and pieces still interleave above it.
		setNotSentLowat(conn.NetConn(), notSentLowat)
	}
	return pc
}

// writeTurn decides which writer goes next on a pieced connection. A small frame
// always goes before the next piece of a large one; among small frames, and among
// pieces, the order is whatever the waiters wake in.
//
// Small frames can delay a large one indefinitely only by arriving continuously,
// and the outbound budget bounds how many can be queued (outbound.go).
type writeTurn struct {
	mu           sync.Mutex
	cond         *sync.Cond
	busy         bool
	smallWaiting int
}

func (w *writeTurn) acquire(small bool) {
	w.mu.Lock()
	if w.cond == nil {
		w.cond = sync.NewCond(&w.mu)
	}
	if small {
		w.smallWaiting++
	}
	for w.busy || (!small && w.smallWaiting > 0) {
		w.cond.Wait()
	}
	if small {
		w.smallWaiting--
	}
	w.busy = true
	w.mu.Unlock()
}

func (w *writeTurn) release() {
	w.mu.Lock()
	w.busy = false
	if w.cond != nil {
		w.cond.Broadcast()
	}
	w.mu.Unlock()
}

// writePieced writes one frame on a pieced connection. A write error leaves the
// stream mid-frame, so the caller must drop the connection, which every caller
// already does on any write error.
func (p *peerConn) writePieced(frame []byte) error {
	if len(frame) <= pieceBytes {
		p.turn.acquire(true)
		defer p.turn.release()
		return p.writeUnit(uint32(len(frame)), frame)
	}
	// One pieced frame at a time per connection, which is what lets the format
	// carry no stream identifier.
	p.bulk.Lock()
	defer p.bulk.Unlock()
	for off := 0; off < len(frame); off += pieceBytes {
		end := min(off+pieceBytes, len(frame))
		p.turn.acquire(false)
		var err error
		if off == 0 {
			err = p.writeUnit(pieceStart|uint32(len(frame)), nil)
		}
		if err == nil {
			err = p.writeUnit(pieceNext|uint32(end-off), frame[off:end])
		}
		p.turn.release()
		if err != nil {
			return err
		}
	}
	return nil
}

// writeUnit writes one header and its payload as a single TLS write, so a unit is
// never split across records for want of a copy.
func (p *peerConn) writeUnit(hdr uint32, payload []byte) error {
	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf, hdr)
	copy(buf[4:], payload)
	_, err := p.conn.Write(buf)
	return err
}

// readPieced reads framed messages off a pieced connection until it closes or
// breaks the format, handing each completed frame to handle. The inbound budget is
// charged for a pieced frame's whole length when it starts, before any of it is
// allocated, exactly as an unpieced frame is charged before it is read.
func (t *Transport) readPieced(conn net.Conn, from ports.NodeID, handle func(frame []byte, release func())) error {
	var (
		partial []byte
		release func()
	)
	defer func() {
		if release != nil {
			release()
		}
	}()
	for {
		var b [4]byte
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			return err
		}
		hdr := binary.BigEndian.Uint32(b[:])
		n := hdr &^ pieceFlags
		switch hdr & pieceFlags {
		case 0:
			if n == 0 || n > maxFrame {
				return fmt.Errorf("frame length %d", n)
			}
			t.inbound.acquire(from, int64(n))
			frame := make([]byte, n)
			rel := t.inboundRelease(from, int64(n))
			if _, err := io.ReadFull(conn, frame); err != nil {
				rel()
				return err
			}
			handle(frame, rel)
		case pieceStart:
			if partial != nil {
				return fmt.Errorf("a pieced frame started inside another")
			}
			if n <= pieceBytes || n > maxFrame {
				return fmt.Errorf("pieced frame length %d", n)
			}
			t.inbound.acquire(from, int64(n))
			release = t.inboundRelease(from, int64(n))
			partial = make([]byte, 0, n)
		case pieceNext:
			if partial == nil {
				return fmt.Errorf("a piece with no frame in progress")
			}
			if n == 0 || n > pieceBytes || len(partial)+int(n) > cap(partial) {
				return fmt.Errorf("piece of %d bytes at %d of %d", n, len(partial), cap(partial))
			}
			at := len(partial)
			partial = partial[:at+int(n)]
			if _, err := io.ReadFull(conn, partial[at:]); err != nil {
				return err
			}
			if len(partial) == cap(partial) {
				frame, rel := partial, release
				partial, release = nil, nil
				handle(frame, rel)
			}
		default:
			return fmt.Errorf("header with both piece flags set")
		}
	}
}

// inboundRelease returns a release for n bytes charged to from that is safe to call
// more than once.
func (t *Transport) inboundRelease(from ports.NodeID, n int64) func() {
	done := false
	return func() {
		if !done {
			done = true
			t.inbound.release(from, n)
		}
	}
}
