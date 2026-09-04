package tcpnet

// R4.3b (2026-09-04) — the transport half of observed-address keying. The transport is
// the ONLY place an IP exists; it exports an opaque per-process-salted (class, group)
// through ports.PeerClassifier. Gates G-2 (address book never classifies), G-3 (relayed
// conns present the relay's group in ONE namespace), G-5 (DIRECT never downgraded), G-8
// (exemption scope: loopback + link-local only; RFC1918 CLASSIFIED — the cloudtest plan
// is 10.20.0.x, cert §6.3 trap), G-9 (no group on the wire, on disk or in a log line;
// per-process salt). Spec: silt-reviews/research/research-outcome/R4.3b-relayed-class-
// and-observed-address-keying-RESEARCH-CERTIFICATION-2026-09-04.md §6–§8.

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/relay"
	"github.com/nerolabs/silt/ports"
)

func r43bLoop(t *testing.T) *eventloop.Loop {
	t.Helper()
	loop := eventloop.New()
	go loop.Run()
	t.Cleanup(loop.Stop)
	return loop
}

func r43bTransport(t *testing.T, seed int64) (*Transport, *identity.Identity) {
	t.Helper()
	ident := identity.FromSeed(seed)
	tr, err := New(r43bLoop(t), ident, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	return tr, ident
}

func tcpAddr(ip string) net.Addr { return &net.TCPAddr{IP: net.ParseIP(ip), Port: 4001} }

// TestR43b_G8_ExemptionScopeIsLoopbackAndLinkLocalOnly — G-8. Loopback, link-local and
// unspecified are exempt (group 0); EVERYTHING else is classified, RFC1918 included —
// a geth-style LAN exemption would classify every cloudtest node as exempt and the
// shadow run would measure nothing. v4 keys by /24, v6 by /32.
func TestR43b_G8_ExemptionScopeIsLoopbackAndLinkLocalOnly(t *testing.T) {
	var salt [16]byte
	copy(salt[:], "r43b-fixed-salt!")
	group := func(ip string) (uint64, bool) {
		return addrGroup(salt, net.ParseIP(ip), 24, 32)
	}
	for _, ip := range []string{"127.0.0.1", "127.0.0.9", "::1", "169.254.1.1", "fe80::1", "0.0.0.0", "::"} {
		if g, exempt := group(ip); !exempt || g != 0 {
			t.Fatalf("G-8 RED: %s classified (group %#x, exempt %v); loopback / link-local / unspecified must be exempt with group 0", ip, g, exempt)
		}
	}
	for _, ip := range []string{"10.20.0.11", "10.30.0.11", "192.168.1.1", "172.16.0.1", "203.0.113.5", "2001:db8::1", "100.64.0.1"} {
		if g, exempt := group(ip); exempt || g == 0 {
			t.Fatalf("G-8 RED: %s EXEMPT (group %#x, exempt %v) — RFC1918/CGNAT must be CLASSIFIED: the cloudtest plan is 10.20.0.x/10.30.0.x and an exempt private range measures nothing (cert §6.3)", ip, g, exempt)
		}
	}
	same := func(a, b string) bool {
		ga, _ := group(a)
		gb, _ := group(b)
		return ga == gb
	}
	if !same("10.20.0.11", "10.20.0.99") {
		t.Fatalf("G-8 RED: 10.20.0.11 and 10.20.0.99 are in different groups; v4 keys by /24")
	}
	if same("10.20.0.11", "10.20.1.11") || same("10.20.0.11", "10.30.0.11") {
		t.Fatalf("G-8 RED: distinct /24s (10.20.0.x vs 10.20.1.x / 10.30.0.x) share a group; v4 keys by /24")
	}
	if !same("2001:db8:1::1", "2001:db8:ffff::1") {
		t.Fatalf("G-8 RED: two addresses in one v6 /32 are in different groups; v6 keys by /32 (an IPv6 /64 is free — red-team)")
	}
	if same("2001:db8::1", "2001:db9::1") {
		t.Fatalf("G-8 RED: distinct v6 /32s share a group")
	}
	if same("10.20.0.11", "2001:db8::1") {
		t.Fatalf("G-8 RED: a v4 /24 and a v6 /32 collide")
	}
	// The width is a flag: /16 must merge what /24 splits.
	if g16a, _ := addrGroup(salt, net.ParseIP("10.20.0.11"), 16, 32); g16a != func() uint64 { g, _ := addrGroup(salt, net.ParseIP("10.20.1.11"), 16, 32); return g }() {
		t.Fatalf("G-8 RED: width 16 does not merge 10.20.0.x and 10.20.1.x")
	}
}

// TestR43b_G2_AddressBookNeverClassifies — G-2 at the transport: an address learned by
// AddPeer (bootstrap / persisted peers) or by envelope gossip is NOT a conversation; the
// peer stays unclassified until a handshake completes at that address.
func TestR43b_G2_AddressBookNeverClassifies(t *testing.T) {
	tr, _ := r43bTransport(t, 61)
	peer := identity.FromSeed(62).NodeID()
	tr.AddPeer(peer, "203.0.113.5:4001")
	if _, _, known := tr.ClassOf(peer); known {
		t.Fatalf("G-2 RED: AddPeer classified a peer that never answered — the address book is a declared value, not an observation")
	}
	tr.learnGossip(peer, "198.51.100.7:4001", true)
	if _, _, known := tr.ClassOf(peer); known {
		t.Fatalf("G-2 RED: envelope gossip classified a peer that never answered")
	}
	tr.observeConn(peer, tcpAddr("203.0.113.5"), false)
	c, g, known := tr.ClassOf(peer)
	if !known || c != ports.ClassDirect || g == 0 {
		t.Fatalf("G-2 RED: after a completed direct conversation ClassOf = (class %d, group %#x, known %v); want (DIRECT, non-zero, true)", c, g, known)
	}
}

// TestR43b_G3_RelayedConnPresentsTheRelaysGroupInOneNamespace — G-3 / C-1. A relayed
// conversation is keyed on the RELAY's address (the socket is to the relay) as class
// RELAYED; the relay's own direct conversation from the same /24 lands in the SAME group.
func TestR43b_G3_RelayedConnPresentsTheRelaysGroupInOneNamespace(t *testing.T) {
	tr, _ := r43bTransport(t, 63)
	relayID := identity.FromSeed(64).NodeID()
	tr.observeConn(relayID, tcpAddr("203.0.113.40"), false)
	rc, rg, rknown := tr.ClassOf(relayID)
	if !rknown || rc != ports.ClassDirect || rg == 0 {
		t.Fatalf("G-3 fixture: the relay's own direct conversation is (class %d, group %#x, known %v)", rc, rg, rknown)
	}
	for i := int64(65); i < 70; i++ {
		client := identity.FromSeed(i).NodeID()
		tr.observeConn(client, tcpAddr("203.0.113.40"), true) // the splice: the socket's remote is the relay
		c, g, known := tr.ClassOf(client)
		if !known || c != ports.ClassRelayed {
			t.Fatalf("G-3 RED: a relay-spliced conversation classified as (class %d, known %v); want RELAYED — the relay's address is being read as the peer's own (rule (c), REFUTED) or exempted (rule (b), REFUTED)", c, known)
		}
		if g != rg {
			t.Fatalf("G-3 RED (C-1): relayed client group %#x ≠ the relay's own direct group %#x — the group namespace is split by class, so a relay /24 yields cap_direct + cap_relay slots", g, rg)
		}
	}
}

// TestR43b_G5_TransportNeverDowngradesDirect — G-5 / C-3 at the transport: once DIRECT at
// a /24, a later relayed conversation leaves the class alone; a later DIRECT conversation
// at another /24 re-keys.
func TestR43b_G5_TransportNeverDowngradesDirect(t *testing.T) {
	tr, _ := r43bTransport(t, 71)
	pony := identity.FromSeed(72).NodeID()
	tr.observeConn(pony, tcpAddr("198.51.100.20"), false)
	_, gA, _ := tr.ClassOf(pony)
	tr.observeConn(pony, tcpAddr("203.0.113.40"), true) // re-reached through a relay
	c, g, known := tr.ClassOf(pony)
	if !known || c != ports.ClassDirect || g != gA {
		t.Fatalf("G-5 RED (C-3): after a later relayed conversation ClassOf = (class %d, group %#x); want the earlier (DIRECT, %#x) kept — the adopt-newest-wins oscillation flips honest ponies into the reserve", c, g, gA)
	}
	tr.observeConn(pony, tcpAddr("198.51.101.20"), false) // a new /24, directly
	c, g, _ = tr.ClassOf(pony)
	if c != ports.ClassDirect || g == gA || g == 0 {
		t.Fatalf("G-5 RED: a later DIRECT conversation at a new /24 did not re-key: (class %d, group %#x), old %#x", c, g, gA)
	}
}

// TestR43b_G8_LoopbackConversationsClassifyExempt — G-8 on REAL sockets: two on-host
// transports converse over 127.0.0.1 and each classifies the other as exempt (group 0)
// but known — the hook is wired into readLoop for a direct accept and a direct dial.
func TestR43b_G8_LoopbackConversationsClassifyExempt(t *testing.T) {
	trA, identA := r43bTransport(t, 73)
	trB, identB := r43bTransport(t, 74)
	got := make(chan struct{}, 1)
	trA.SetHandler(func(from ports.NodeID, msg ports.Message) {
		trA.Send(from, ports.Message{Kind: ports.MsgFindNodeReply, RID: msg.RID})
	})
	trB.SetHandler(func(from ports.NodeID, msg ports.Message) {
		if from == identA.NodeID() {
			got <- struct{}{}
		}
	})
	trB.AddPeer(identA.NodeID(), trA.Addr())
	if err := trB.Send(identA.NodeID(), ports.Message{Kind: ports.MsgFindNode, RID: 1}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("no reply")
	}
	for _, side := range []struct {
		name string
		tr   *Transport
		peer ports.NodeID
	}{{"dialer", trB, identA.NodeID()}, {"acceptor", trA, identB.NodeID()}} {
		c, g, known := side.tr.ClassOf(side.peer)
		if !known || c != ports.ClassDirect || g != 0 {
			t.Fatalf("G-8 RED (%s): a completed loopback conversation is (class %d, group %#x, known %v); want (DIRECT, 0 = exempt, true) — an on-host swarm must classify as exempt, and the hook must fire on both a dial and an accept", side.name, c, g, known)
		}
	}
}

// TestR43b_G3_RelayedRoundTripClassifiesRelayedOnBothEnds — G-3 on REAL sockets through a
// real relay on loopback: the dialer's relay-form conn AND the registrant's RelayInbound
// conn both reach the hook with viaRelay=true (group 0: the relay is on loopback).
func TestR43b_G3_RelayedRoundTripClassifiesRelayedOnBothEnds(t *testing.T) {
	identR := identity.FromSeed(75)
	srv, err := relay.Serve("127.0.0.1:0", identR, relay.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	trA, identA := r43bTransport(t, 76)
	trB, identB := r43bTransport(t, 77)
	rc, err := relay.NewClient(identB, identR.NodeID(), srv.Addr(), trB.RelayInbound, nil)
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan error, 1)
	go rc.Run(func(err error) { ready <- err })
	t.Cleanup(rc.Close)
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("relay registration timed out")
	}
	trB.SetAdvertise(rc.Addr())
	trB.SetHandler(func(from ports.NodeID, msg ports.Message) {
		trB.Send(from, ports.Message{Kind: ports.MsgFindNodeReply, RID: msg.RID})
	})
	reply := make(chan struct{}, 1)
	trA.SetHandler(func(from ports.NodeID, msg ports.Message) {
		if from == identB.NodeID() {
			reply <- struct{}{}
		}
	})
	trA.AddPeer(identB.NodeID(), rc.Addr())
	if err := trA.Send(identB.NodeID(), ports.Message{Kind: ports.MsgFindNode, RID: 7}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reply:
	case <-time.After(10 * time.Second):
		t.Fatal("no reply through the relay")
	}
	if c, _, known := trA.ClassOf(identB.NodeID()); !known || c != ports.ClassRelayed {
		t.Fatalf("G-3 RED (dialer side): a relay-form dial classified (class %d, known %v); want RELAYED", c, known)
	}
	if c, _, known := trB.ClassOf(identA.NodeID()); !known || c != ports.ClassRelayed {
		t.Fatalf("G-3 RED (RelayInbound side): a registrant's spliced inbound classified (class %d, known %v); want RELAYED", c, known)
	}
}

// TestR43b_G9_SaltIsPerProcessAndGroupNeverLeavesTheProcess — G-9. Two transports
// classify the same /24 to different groups (the salt is per transport instance, drawn
// at New); the wire envelope and the persisted peer record carry no class/group field
// (a positive pin of the CURRENT key sets: envelope keys 1..5, wireMsg keys 1..28,
// tcpnet.Peer = {ID, Addr}).
func TestR43b_G9_SaltIsPerProcessAndGroupNeverLeavesTheProcess(t *testing.T) {
	trA, _ := r43bTransport(t, 81)
	trB, _ := r43bTransport(t, 82)
	peer := identity.FromSeed(83).NodeID()
	trA.observeConn(peer, tcpAddr("203.0.113.5"), false)
	trB.observeConn(peer, tcpAddr("203.0.113.5"), false)
	_, gA, _ := trA.ClassOf(peer)
	_, gB, _ := trB.ClassOf(peer)
	if gA == gB {
		t.Fatalf("G-9 RED: two transports classify 203.0.113.0/24 to the SAME group %#x — the salt is not per process (a group would be comparable across nodes; leak 4)", gA)
	}

	for _, typ := range []reflect.Type{reflect.TypeOf(envelope{}), reflect.TypeOf(wireMsg{}), reflect.TypeOf(Peer{})} {
		for i := 0; i < typ.NumField(); i++ {
			name := strings.ToLower(typ.Field(i).Name)
			if strings.Contains(name, "class") || strings.Contains(name, "group") || strings.Contains(name, "salt") {
				t.Fatalf("G-9 RED: %s has a field %q — no class/group/salt may exist on the wire or on disk", typ.Name(), typ.Field(i).Name)
			}
		}
	}
	if got := reflect.TypeOf(Peer{}).NumField(); got != 2 {
		t.Fatalf("G-9 RED: tcpnet.Peer has %d fields; the persisted peers file carries exactly {ID, Addr}", got)
	}
	env := envelope{From: peer[:], Addr: "203.0.113.5:4001", Relay: "203.0.113.5:4002",
		Contacts: map[string]string{peer.String(): "203.0.113.6:4001"},
		Msg: toWire(ports.Message{Kind: ports.MsgFindNodeReply, RID: 9, Nodes: []ports.NodeID{peer}, Domain: 7,
			BondRoot: ports.HashBytes([]byte("b")), BondSize: 1, CapTotal: 2, Ephemeral: true})}
	frame, err := encMode.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var top map[uint64]cbor.RawMessage
	if err := cbor.Unmarshal(frame, &top); err != nil {
		t.Fatal(err)
	}
	for k := range top {
		if k < 1 || k > 5 {
			t.Fatalf("G-9 RED: envelope carries CBOR key %d outside the pinned set 1..5 — a new wire field; if intentional, re-pin here AND prove it is not a class/group", k)
		}
	}
	var msg map[uint64]cbor.RawMessage
	if err := cbor.Unmarshal(top[4], &msg); err != nil {
		t.Fatal(err)
	}
	for k := range msg {
		if k < 1 || k > 28 {
			t.Fatalf("G-9 RED: wireMsg carries CBOR key %d outside the pinned set 1..28", k)
		}
	}
}

// TestR43b_G9_NoLogLineCarriesAGroup is a SOURCE gate: no logging call in the
// transport, the table or the node names a "group" key (the red-team's leak 4: a
// survivor-census log keyed by group). Checked: text only — every `logf(`/`dlog(`/
// `Printf(` call in adapters/tcpnet, core/dht, core/node non-test sources, for a
// string literal "group" argument.
// UNGATED: that no OTHER output path (the /api/status JSON, a metrics line) prints a raw
// group value — series E is a density census by design and must aggregate, not list.
func TestR43b_G9_NoLogLineCarriesAGroup(t *testing.T) {
	var files []string
	for _, dir := range []string{".", "../../core/dht", "../../core/node"} {
		m, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range m {
			if !strings.HasSuffix(f, "_test.go") {
				files = append(files, f)
			}
		}
	}
	if len(files) < 10 {
		t.Fatalf("SOURCE GATE: glob found only %d non-test sources; the scan is not covering the three packages", len(files))
	}
	call := regexp.MustCompile(`(?s)\b(logf|dlog|Printf|Println|Fprintf)\(([^()]|\([^()]*\))*\)`)
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range call.FindAllString(string(src), -1) {
			if strings.Contains(m, `"group"`) || strings.Contains(m, `"groups"`) {
				t.Fatalf("SOURCE GATE: %s has a logging call with a \"group\" key: %s — a salted group must never appear in a log line (G-9)", f, strings.Join(strings.Fields(m), " "))
			}
		}
	}
}
