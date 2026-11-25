package tcpnet

import (
	"sync/atomic"

	"github.com/nerolabs/silt/ports"
)

// The traffic meter: frames and bytes this node sent and received, per message kind,
// since it started. Node-wide totals only, never joined to a peer: it says what a node
// spends its bandwidth on (consensus, the registry, storage, routing), which is what
// pricing a role needs, and nothing about whom it talks to.
//
// Bytes are encoded frame bytes, envelope included, without TLS or TCP overhead. A sent
// frame is counted once it passes outbound admission (a dropped frame cost nothing on
// the wire); a received frame once it decodes.
type kindCounter struct {
	outFrames, outBytes, inFrames, inBytes atomic.Int64
}

type trafficMeter struct {
	byKind [256]kindCounter
}

func (m *trafficMeter) sent(k ports.MsgKind, n int) {
	c := &m.byKind[uint8(k)]
	c.outFrames.Add(1)
	c.outBytes.Add(int64(n))
}

func (m *trafficMeter) received(k ports.MsgKind, n int) {
	c := &m.byKind[uint8(k)]
	c.inFrames.Add(1)
	c.inBytes.Add(int64(n))
}

// KindTraffic is one message kind's lifetime totals.
type KindTraffic struct {
	OutFrames int64 `json:"outFrames"`
	OutBytes  int64 `json:"outBytes"`
	InFrames  int64 `json:"inFrames"`
	InBytes   int64 `json:"inBytes"`
}

// Traffic returns the lifetime totals of every kind this node has sent or received,
// keyed by the kind's name.
func (t *Transport) Traffic() map[string]KindTraffic {
	out := map[string]KindTraffic{}
	for i := range t.traffic.byKind {
		c := &t.traffic.byKind[i]
		kt := KindTraffic{OutFrames: c.outFrames.Load(), OutBytes: c.outBytes.Load(),
			InFrames: c.inFrames.Load(), InBytes: c.inBytes.Load()}
		if kt.OutFrames != 0 || kt.InFrames != 0 {
			out[ports.MsgKind(i).String()] = kt
		}
	}
	return out
}
