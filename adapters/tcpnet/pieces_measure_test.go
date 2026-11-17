package tcpnet

import (
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/ports"
)

// A SMALL FRAME BEHIND A LARGE ONE ON ONE ORDERED CONNECTION, with and without
// pieced framing.
//
// The control lane removes head-of-line blocking only where a second connection can
// be dialled. A peer reached through a relay, or one that cannot be dialled at all,
// keeps one ordered stream, and on it a small frame waits behind the whole payload
// ahead of it (TestAControlFrameWaitsBehindBulkOnOneOrderedConnection). This measures
// what pieced framing recovers on that stream.

// onePipeReader accepts conns as `seed`'s identity, refuses the control lane, and
// drains the one bulk conn at a fixed byte rate — pieced if the dialer offered it and
// pieced is true, unpieced otherwise — reporting when each whole frame completes.
type onePipeReader struct {
	id       ports.NodeID
	addr     string
	arrivals chan arrival
	lanes    chan string
}

func newOnePipeReader(t *testing.T, seed int64, bytesPerSec int, pieced bool) *onePipeReader {
	t.Helper()
	ident := identity.FromSeed(seed)
	cert, err := ident.Certificate()
	if err != nil {
		t.Fatal(err)
	}
	base := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAnyClientCert,
		MinVersion:   tls.VersionTLS13,
	}
	// The control-lane dial negotiates NOTHING, which the transport reads as a peer
	// without the lane: it closes that conn and sends every frame on the bulk one.
	// That is the single-stream peer this file is about.
	cfg := base.Clone()
	cfg.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		if slices.Contains(hello.SupportedProtos, alpnCtrl) {
			return base, nil
		}
		c := base.Clone()
		if pieced {
			c.NextProtos = []string{alpnBulkPieced, alpnBulk}
		} else {
			c.NextProtos = []string{alpnBulk}
		}
		return c, nil
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := &onePipeReader{id: ident.NodeID(), addr: ln.Addr().String(),
		arrivals: make(chan arrival, 64), lanes: make(chan string, 8)}
	done := make(chan struct{})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				go func() { <-done; c.Close() }()
				tc := c.(*tls.Conn)
				if err := tc.Handshake(); err != nil {
					return
				}
				lane := tc.ConnectionState().NegotiatedProtocol
				select {
				case r.lanes <- lane:
				default:
				}
				const slice = 4 << 10
				pause := time.Duration(int64(time.Second) * int64(slice) / int64(bytesPerSec))
				buf := make([]byte, slice)
				drain := func(n int) bool {
					for left := n; left > 0; {
						take := min(slice, left)
						if _, err := io.ReadFull(tc, buf[:take]); err != nil {
							return false
						}
						left -= take
						time.Sleep(pause)
					}
					return true
				}
				var want, got int
				for {
					var hdr [4]byte
					if _, err := io.ReadFull(tc, hdr[:]); err != nil {
						return
					}
					h := binary.BigEndian.Uint32(hdr[:])
					n := int(h &^ pieceFlags)
					if lane != alpnBulkPieced {
						n = int(h)
						h = 0
					}
					switch h & pieceFlags {
					case pieceStart:
						want, got = n, 0
						continue
					case pieceNext:
						if !drain(n) {
							return
						}
						if got += n; got < want {
							continue
						}
						n = want
					default:
						if !drain(n) {
							return
						}
					}
					select {
					case r.arrivals <- arrival{size: n, at: time.Now()}:
					default:
					}
				}
			}(c)
		}
	}()
	t.Cleanup(func() { close(done); ln.Close() })
	return r
}

// smallBehindBulk reports how long a small frame took to arrive with nothing ahead
// of it and with a bulk frame ahead of it, on one connection to a peer that does or
// does not negotiate pieced framing, and which framing was negotiated.
func smallBehindBulk(t *testing.T, seed int64, rate, bulk int, pieced bool) (alone, behind time.Duration, lane string) {
	t.Helper()
	const small = 121
	wait := 10*time.Duration(int64(time.Second)*int64(bulk)/int64(rate)) + 30*time.Second

	tr, loop := newTransport(t, seed)
	defer func() { tr.Close(); loop.Stop() }()
	r := newOnePipeReader(t, seed+1, rate, pieced)
	tr.AddPeer(r.id, r.addr)

	await := func(pred func(arrival) bool) time.Time {
		for deadline := time.After(wait); ; {
			select {
			case a := <-r.arrivals:
				if pred(a) {
					return a.at
				}
			case <-deadline:
				t.Fatal("the frame never arrived inside the window")
			}
		}
	}
	isSmall := func(a arrival) bool { return a.size < bulk/2 }

	// Open the conversation so the handshake is outside both measurements.
	if err := tr.Send(r.id, ports.Message{Kind: ports.MsgGetChainHead}); err != nil {
		t.Fatalf("opening send: %v", err)
	}
	await(isSmall)
	lane = <-r.lanes

	start := time.Now()
	if err := tr.Send(r.id, ports.Message{Kind: ports.MsgGetChainHead, Data: make([]byte, small)}); err != nil {
		t.Fatalf("small send: %v", err)
	}
	alone = await(isSmall).Sub(start)

	if err := tr.Send(r.id, ports.Message{Kind: ports.MsgStoreChunk, Data: make([]byte, bulk)}); err != nil {
		t.Fatalf("bulk send: %v", err)
	}
	// Let the bulk write take the connection and back up behind the slow reader, so
	// the small frame is offered behind it rather than racing it.
	time.Sleep(500 * time.Millisecond)
	start = time.Now()
	if err := tr.Send(r.id, ports.Message{Kind: ports.MsgGetChainHead, Data: make([]byte, small)}); err != nil {
		t.Fatalf("small send: %v", err)
	}
	// Wait for BOTH. The small frame's time is the measurement; the bulk frame must
	// also arrive whole, because pieces that interleave but do not reassemble would
	// make the small frame fast by losing the payload.
	var gotSmall, gotBulk bool
	for deadline := time.After(wait); !gotSmall || !gotBulk; {
		select {
		case a := <-r.arrivals:
			switch {
			case isSmall(a):
				behind, gotSmall = a.at.Sub(start), true
			case a.size >= bulk:
				gotBulk = true
			}
		case <-deadline:
			t.Fatalf("inside the window: small frame arrived %v, bulk frame arrived whole %v", gotSmall, gotBulk)
		}
	}
	return alone, behind, lane
}

func reportPieces(t *testing.T, rate, bulk int, unpieced, pieced [2]time.Duration) {
	t.Helper()
	wire := time.Duration(int64(time.Second) * int64(bulk) / int64(rate))
	t.Logf("MEASURED — a 121-byte frame on ONE connection draining at %d B/s, %d-byte bulk ahead (%v of wire):", rate, bulk, wire)
	t.Logf("  unpieced: alone %v, behind bulk %v", unpieced[0].Round(time.Millisecond), unpieced[1].Round(time.Millisecond))
	t.Logf("  pieced:   alone %v, behind bulk %v", pieced[0].Round(time.Millisecond), pieced[1].Round(time.Millisecond))
	if pieced[1] > 0 {
		t.Logf("  => pieced framing cuts the wait behind bulk %.1fx", float64(unpieced[1])/float64(pieced[1]))
	}
}

// measurePieces runs both arms and requires the pieced wait to be at most
// boundNum/boundDen of the bulk frame's wire time.
func measurePieces(t *testing.T, seed int64, rate, bulk int, boundNum, boundDen int64) {
	var unp, pcd [2]time.Duration
	var lane string
	unp[0], unp[1], lane = smallBehindBulk(t, seed, rate, bulk, false)
	if lane != alpnBulk {
		t.Fatalf("the unpieced arm negotiated %q, want %q — it is not measuring the old framing", lane, alpnBulk)
	}
	pcd[0], pcd[1], lane = smallBehindBulk(t, seed+10, rate, bulk, true)
	if lane != alpnBulkPieced {
		t.Fatalf("the pieced arm negotiated %q, want %q — it is not measuring pieces", lane, alpnBulkPieced)
	}
	reportPieces(t, rate, bulk, unp, pcd)

	wire := time.Duration(int64(time.Second) * int64(bulk) / int64(rate))
	if unp[1] < wire/2 {
		t.Fatalf("VACUOUS: unpieced, the small frame waited only %v behind %v of bulk — the fixture is not "+
			"serializing them, so the pieced arm has nothing to improve on", unp[1], wire)
	}
	if bound := time.Duration(int64(wire) * boundNum / boundDen); pcd[1] > bound {
		t.Fatalf("pieced, the small frame still waited %v behind %v of bulk on one connection, above %v — the "+
			"pieces are not interleaving, or the kernel is holding more of the payload ahead of them than it did",
			pcd[1], wire, bound)
	}
}

func TestPiecedFramingLetsASmallFramePastBulkOnOneConnection(t *testing.T) {
	measurePieces(t, 700, 8<<20, 16<<20, 1, 4)
}

// AT THE FIELD'S RATE AND PAYLOAD: a ~1.5 MB bond proof on a link a quarter of the
// deadline's assumed floor, which is the congested arm of the coverage measurement.
//
// THE BOUND IS LOOSE BECAUSE THE KERNEL DECIDES THE REST. Pieces interleave only
// above what the socket has already accepted. Linux enforces TCP_NOTSENT_LOWAT in
// sendmsg, so the writer blocks with little unsent; Darwin accepts the option and
// does not enforce it on write, so the send buffer autotunes and ~800 KB of the proof
// is still ahead of the small frame — measured at 12.7 s of a 24 s transfer, against
// 24.1 s unpieced. The bound catches pieces that stop interleaving at all, and is
// not a claim about the Linux figure, which has not been measured.
func TestPiecedFramingAtTheCongestedFieldRate(t *testing.T) {
	if testing.Short() {
		t.Skip("drains ~1.5 MB at 64 KiB/s, twice")
	}
	measurePieces(t, 720, 64<<10, 1_574_000, 3, 4)
}
