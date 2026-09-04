package tcpnet

// R4.3b — the transport is the ONLY place an IP exists. On a completed handshake
// (readLoop, and the dialer's adopt) observeConn records the socket's remote
// address as an opaque, per-process-salted group: DIRECT at the peer's own prefix
// for a direct conversation, RELAYED at the RELAY's prefix for the two spliced
// paths (the socket is to the relay). The core reads (class, group) through
// ClassOf and nothing else; the group never touches the wire, the peers file or
// a log line. Cert: silt-reviews/research/research-outcome/R4.3b-relayed-class-
// and-observed-address-keying-RESEARCH-CERTIFICATION-2026-09-04.md §6–§7.

import (
	"crypto/sha256"
	"encoding/binary"
	"net"

	"github.com/nerolabs/silt/ports"
)

// Default prefix widths: IPv4 /24 (geth's bucketIPLimit granularity), IPv6 /32
// (an IPv6 /64 is free — red-team; /32 is the RIR allocation unit).
const (
	defaultV4Width = 24
	defaultV6Width = 32
)

type peerClass struct {
	class ports.PeerClass
	group uint64
}

// SetAddressWidth sets the prefix widths the group keys on (-dht-address-width).
// Re-keys nothing already classified; call before the first conversation.
func (t *Transport) SetAddressWidth(v4, v6 int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if v4 > 0 && v4 <= 32 {
		t.v4Width = v4
	}
	if v6 > 0 && v6 <= 128 {
		t.v6Width = v6
	}
}

// addrGroup keys an IP to its salted group: v4 by v4Width bits, v6 by v6Width.
// exempt=true (and group 0) for loopback, link-local and unspecified ONLY —
// RFC1918 and CGNAT are CLASSIFIED (the cloudtest plan is 10.20.0.x; a private-
// range exemption would make the shadow run measure nothing, cert §6.3). The
// family is folded into the hash so a v4 /24 and a v6 /32 never collide.
func addrGroup(salt [16]byte, ip net.IP, v4Width, v6Width int) (group uint64, exempt bool) {
	if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return 0, true
	}
	var prefix net.IP
	var tag byte
	if v4 := ip.To4(); v4 != nil {
		prefix, tag = v4.Mask(net.CIDRMask(v4Width, 32)), 4
	} else {
		prefix, tag = ip.To16().Mask(net.CIDRMask(v6Width, 128)), 6
	}
	h := sha256.New()
	h.Write(salt[:])
	h.Write([]byte{tag})
	h.Write(prefix)
	group = binary.BigEndian.Uint64(h.Sum(nil)[:8])
	if group == 0 {
		group = 1 // 0 is the exempt group
	}
	return group, false
}

func remoteIP(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.TCPAddr:
		return v.IP
	case *net.UDPAddr:
		return v.IP
	}
	host, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

// observeConn records a completed conversation with id over a socket whose
// remote is `remote`. viaRelay=false ⇒ ClassDirect at remote's group (the peer's
// own address); viaRelay=true ⇒ ClassRelayed at remote's group (the RELAY's
// address). C-3: a DIRECT classification is never downgraded by a later relayed
// conversation; a later DIRECT conversation at another prefix re-keys it. The map
// is bounded by live conns (dropConn deletes the entry).
func (t *Transport) observeConn(id ports.NodeID, remote net.Addr, viaRelay bool) {
	if remote == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	group, _ := addrGroup(t.salt, remoteIP(remote), t.v4Width, t.v6Width)
	class := ports.ClassDirect
	if viaRelay {
		class = ports.ClassRelayed
	}
	if old, ok := t.classes[id]; ok && old.class == ports.ClassDirect && class != ports.ClassDirect {
		return // C-3
	}
	t.classes[id] = peerClass{class: class, group: group}
}

// ClassOf is the ports.PeerClassifier port. known=false until a conversation
// with id has completed; the address book (AddPeer, gossip) NEVER classifies.
func (t *Transport) ClassOf(id ports.NodeID) (ports.PeerClass, uint64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	c, ok := t.classes[id]
	if !ok {
		return 0, 0, false
	}
	return c.class, c.group, true
}

var _ ports.PeerClassifier = (*Transport)(nil)
