package tcpnet

import (
	"crypto/tls"

	"github.com/nerolabs/silt/ports"
)

// THE CONTROL LANE: a second connection per peer that bulk payload never touches.
//
// The defect it closes is head-of-line blocking on one ordered stream, and it is the
// last cause of the impairment wedge. Consensus, chain-sync and their replies shared
// one TLS connection with the multi-megabyte proofs that congest it, so a 121-byte
// head probe waited behind whatever was already in the stream. Measured on the
// fixture in headofline_measure_test.go: 1 ms alone, 1.927 s behind a 16 MiB frame —
// 2488x — and 1 ms when the two ride separate connections.
//
// It is what the field said and what no admission control could fix. The outbound
// budget stopped small frames being DROPPED (run 1b0b933-52703 dropped zero, against
// twenty the run before) and they still timed out, because a budget decides what to
// hand a socket and cannot reorder what the socket has already accepted.
//
// THE ROLE IS NEGOTIATED IN THE HANDSHAKE, VIA ALPN, and that is forced rather than
// chosen. A conn is adopted as the live conversation with a peer the moment its
// handshake completes — before any frame is read — so nothing a frame could carry is
// available in time. TLS already solves exactly this with protocol negotiation, and
// using it keeps the role a property of the connection rather than of a convention
// about its first message.
//
// THE SPLIT IS BY SIZE, not by message kind, for the same reason the outbound
// reserve is: the transport is an adapter (B1) and must not learn the core's message
// taxonomy to route correctly. The two populations are four orders of magnitude
// apart, so the threshold is not delicate — smallFrameBytes is the same constant
// both mechanisms use, and they are the same judgement made twice.
//
// WHAT IT DOES NOT DO. It does not make the bulk frame arrive any sooner; a carried
// proof still takes its own wire time. It stops that payload from deciding when
// everything else arrives. The remaining cost of a large proof on the critical path
// is the proof's own size, which is succinct-proof work and a new era.
const (
	// alpnBulk and alpnCtrl name the two lanes in the TLS handshake. A peer that
	// offers neither is pre-lane software: NegotiatedProtocol comes back empty, it
	// is adopted as the bulk conversation, and everything it is sent rides one
	// connection exactly as before. That is the mixed-version story and it needs no
	// version check — an old node simply never gets the second lane.
	alpnBulk = "silt/1"
	alpnCtrl = "silt-ctrl/1"
)

// lanesFor returns the ALPN list a dialer offers for a lane.
func lanesFor(ctrl bool) []string {
	if ctrl {
		return []string{alpnCtrl}
	}
	return []string{alpnBulk}
}

// isCtrlConn reports whether a completed handshake negotiated the control lane.
// An empty protocol is a peer that offered none, which is the bulk lane.
func isCtrlConn(cs tls.ConnectionState) bool { return cs.NegotiatedProtocol == alpnCtrl }

// ctrlLaneEligible reports whether a frame of n bytes should ride the control lane.
//
// The rule is deliberately the same size class the outbound reserve protects: a
// frame small enough to be starved is a frame that belongs off the bulk lane. Any
// other threshold would leave a band of frames that the budget treats as control and
// the wire treats as payload.
func ctrlLaneEligible(n int) bool { return int64(n) <= smallFrameBytes }

// noCtrlLane marks a peer that completed a lane dial WITHOUT negotiating it — pre-lane
// software. Without this every small frame to such a peer pays a fresh TCP connect and
// TLS handshake that is then closed, which is a per-message toll on exactly the mixed
// version case that must stay cheap. It is cleared when the bulk conversation with that
// peer drops, so a peer that upgrades is re-probed on its next reconnection rather than
// written off for the process's lifetime.
func (t *Transport) markNoCtrlLane(id ports.NodeID) {
	t.mu.Lock()
	t.noCtrlLane[id] = true
	t.mu.Unlock()
}

func (t *Transport) lacksCtrlLane(id ports.NodeID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.noCtrlLane[id]
}

// ctrlConn returns the live control conversation with id, if there is one.
func (t *Transport) ctrlConn(id ports.NodeID) *peerConn {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ctrlConns[id]
}

// adoptCtrl records a control-lane conn as the control conversation with id. It is
// the ctrlConns twin of adopt, kept separate so the bulk slot — which carries the
// relay and hole-punch state — is never disturbed by a control dial.
func (t *Transport) adoptCtrl(id ports.NodeID, conn *tls.Conn) *peerConn {
	t.mu.Lock()
	defer t.mu.Unlock()
	if pc, ok := t.ctrlConns[id]; ok && pc.conn == conn {
		return pc
	}
	pc := &peerConn{conn: conn}
	t.ctrlConns[id] = pc
	return pc
}

// dropCtrlConn forgets pc if it is still the live control conversation, and closes
// it. A dropped control lane is not an error: the next small frame falls back to the
// bulk conn and the lane is re-dialed on the one after, which is the same
// degradation an old peer lives with permanently.
func (t *Transport) dropCtrlConn(id ports.NodeID, pc *peerConn) {
	t.mu.Lock()
	if t.ctrlConns[id] == pc {
		delete(t.ctrlConns, id)
	}
	t.mu.Unlock()
	pc.conn.Close()
}
