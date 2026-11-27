package tcpnet

import (
	"net"
	"sync"

	"github.com/nerolabs/silt/ports"
)

// writerPolicy is -writers: a peer whose connection comes from a listed address may send
// anything; every other peer may send only the OPEN kinds — the reads a joiner needs
// (lookups, fetches, chain sync, witnesses, issuer keys, reachability), the replies to this
// node's own requests, and the few submissions that carry a self-verifying proof. Everything
// else from a non-writer is dropped: storing a piece, minting a publish token, registering a
// bond or issuer key, submitting an entry, the credit lanes, repair claims, consensus. It is
// an ALLOW-list, so a message kind added later is closed to non-writers until it is listed.
type writerPolicy struct {
	nets []*net.IPNet
	open map[ports.MsgKind]bool
	// pinned kinds also need the sender's identity in ids (SetWriterIDs); nil: no pinning.
	pinned map[ports.MsgKind]bool
	ids    map[ports.NodeID]bool

	mu    sync.Mutex
	named map[ports.NodeID]bool // peers whose first refusal was logged at INFO
}

// maxNamedRefusals bounds how many peers' first refusals are logged at INFO; past it, every
// refusal is a DEBUG line, so a stream of new identities cannot flood the operator's log.
const maxNamedRefusals = 1024

func (p *writerPolicy) firstRefusal(id ports.NodeID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.named[id] || len(p.named) >= maxNamedRefusals {
		return false
	}
	p.named[id] = true
	return true
}

// SetWriters lets peers connecting from nets send anything, and everyone else only open.
// Call it before the node starts; with no nets every peer may send anything (the default).
// A RELAYED connection is never a writer: its address is the relay's, not the sender's.
func (t *Transport) SetWriters(nets []*net.IPNet, open []ports.MsgKind) {
	if len(nets) == 0 {
		t.writers.Store(nil)
		return
	}
	p := &writerPolicy{nets: nets, open: map[ports.MsgKind]bool{}, named: map[ports.NodeID]bool{}}
	for _, k := range open {
		p.open[k] = true
	}
	t.writers.Store(p)
}

// SetWriterIDs additionally requires, for the pinned kinds, that the sender be one of ids —
// the operator's own fixed node identities. Call it after SetWriters and before the node
// starts; it does nothing without SetWriters.
func (t *Transport) SetWriterIDs(ids []ports.NodeID, pinned []ports.MsgKind) {
	p := t.writers.Load()
	if p == nil || len(ids) == 0 {
		return
	}
	np := &writerPolicy{nets: p.nets, open: p.open, named: p.named,
		pinned: map[ports.MsgKind]bool{}, ids: map[ports.NodeID]bool{}}
	for _, k := range pinned {
		np.pinned[k] = true
	}
	for _, id := range ids {
		np.ids[id] = true
	}
	t.writers.Store(np)
}

// WritesRefused counts inbound messages dropped because a non-writer may not send them.
func (t *Transport) WritesRefused() int64 { return t.writesRefused.Load() }

// IsWriter reports whether id's most recent connection came from a listed writer address
// (ports.WriterOracle). With no -writers every peer is a writer; a peer never seen is not.
func (t *Transport) IsWriter(id ports.NodeID) bool {
	if t.writers.Load() == nil {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.writerIDs[id]
}

var _ ports.WriterOracle = (*Transport)(nil)

// writerConn reports whether a connection from remote may write.
func (t *Transport) writerConn(remote net.Addr, viaRelay bool) bool {
	p := t.writers.Load()
	if p == nil {
		return true
	}
	if viaRelay || remote == nil {
		return false
	}
	ip := remoteIP(remote)
	if ip == nil {
		return false
	}
	for _, n := range p.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// noteWriter records whether id's current connection is a writer's, for IsWriter.
func (t *Transport) noteWriter(id ports.NodeID, writer bool) {
	if t.writers.Load() == nil {
		return
	}
	t.mu.Lock()
	if t.writerIDs == nil {
		t.writerIDs = map[ports.NodeID]bool{}
	}
	t.writerIDs[id] = writer
	t.mu.Unlock()
}

// refusesWrite reports whether a message of kind from `from` must be dropped: a non-writer
// may send only open kinds, and a pinned kind also needs a listed identity.
func (t *Transport) refusesWrite(kind ports.MsgKind, writer bool, from ports.NodeID) bool {
	p := t.writers.Load()
	if p == nil {
		return false
	}
	if p.pinned != nil && p.pinned[kind] && !(writer && p.ids[from]) {
		return true
	}
	return !writer && !p.open[kind]
}

// noteRefusal counts a refused message and logs it: at INFO the first time a peer is
// refused, so an operator sees who tried, and at DEBUG after that.
func (t *Transport) noteRefusal(from ports.NodeID, kind ports.MsgKind) {
	t.writesRefused.Add(1)
	level := ports.LogDebug
	if p := t.writers.Load(); p != nil && p.firstRefusal(from) {
		level = ports.LogInfo
	}
	t.logf(level, "write refused: not from a listed writer address", "from", from, "kind", kind)
}
