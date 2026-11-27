package tcpnet

import (
	"net"
	"sync"

	"github.com/nerolabs/silt/ports"
)

// writerPolicy restricts the message kinds that WRITE to a node — store a piece, issue a
// publish token, accept a bond or issuer-key registration, submit an entry — to peers whose
// connection comes from an operator-listed address. Everything a reader needs (lookups,
// fetches, provider records, chain sync, witnesses) stays open to anyone. It is what lets a
// test network accept joiners that read and serve but cannot write: without it, a joined
// node could stash its own files on the operator's disks without ever publishing (a store
// needs no commit), or bond its way into minting its own publish tokens.
type writerPolicy struct {
	nets  []*net.IPNet
	kinds map[ports.MsgKind]bool

	mu    sync.Mutex
	named map[ports.NodeID]bool // peers whose first refusal was logged at INFO
}

// maxNamedRefusals bounds how many peers' first refusals are logged at INFO; past it, every
// refusal is a DEBUG line, so a stream of new identities cannot flood the operator's log.
const maxNamedRefusals = 1024

// firstRefusal reports whether this is the first refused write from id worth an INFO line.
func (p *writerPolicy) firstRefusal(id ports.NodeID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.named[id] || len(p.named) >= maxNamedRefusals {
		return false
	}
	p.named[id] = true
	return true
}

// SetWriters limits kinds to connections from nets. Call it before the node starts; with
// no nets, every peer may write (the default). A RELAYED connection is never a writer: its
// address is the relay's, not the sender's.
func (t *Transport) SetWriters(nets []*net.IPNet, kinds []ports.MsgKind) {
	if len(nets) == 0 {
		t.writers.Store(nil)
		return
	}
	p := &writerPolicy{nets: nets, kinds: map[ports.MsgKind]bool{}, named: map[ports.NodeID]bool{}}
	for _, k := range kinds {
		p.kinds[k] = true
	}
	t.writers.Store(p)
}

// WritesRefused counts inbound write messages dropped because they came from a non-writer.
func (t *Transport) WritesRefused() int64 { return t.writesRefused.Load() }

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

// refusesWrite reports whether msg must be dropped on a connection that is not a writer's.
func (t *Transport) refusesWrite(kind ports.MsgKind, writer bool) bool {
	if writer {
		return false
	}
	p := t.writers.Load()
	return p != nil && p.kinds[kind]
}

// noteRefusal counts a refused write and logs it: at INFO the first time a peer is refused,
// so an operator sees who tried, and at DEBUG after that.
func (t *Transport) noteRefusal(from ports.NodeID, kind ports.MsgKind) {
	t.writesRefused.Add(1)
	level := ports.LogDebug
	if p := t.writers.Load(); p != nil && p.firstRefusal(from) {
		level = ports.LogInfo
	}
	t.logf(level, "write refused: not from a listed writer address", "from", from, "kind", kind)
}
