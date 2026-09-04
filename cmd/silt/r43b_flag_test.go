package main

// R4.3b (2026-09-04) — the daemon surface: -dht-address-cap=off|shadow|on (default
// SHADOW: count, never refuse, until the owner enables `on` against the economist's
// thresholds), the de-herd relay selection (G-11) and the series A/B/E status keys.

import (
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/core/dht"
	"github.com/nerolabs/silt/ports"
)

// TestR43b_AddressCapFlagParsesOffShadowOnDefaultShadow — the parse and the default.
func TestR43b_AddressCapFlagParsesOffShadowOnDefaultShadow(t *testing.T) {
	want := map[string]dht.AddressMode{"off": dht.AddressCapOff, "shadow": dht.AddressCapShadow, "on": dht.AddressCapOn}
	for s, m := range want {
		got, err := parseAddressCapMode(s)
		if err != nil || got != m {
			t.Fatalf("-dht-address-cap=%s parsed to (%d, %v); want %d", s, got, err, m)
		}
	}
	if _, err := parseAddressCapMode("banana"); err == nil {
		t.Fatal("-dht-address-cap=banana was accepted; only off|shadow|on are modes")
	}
	if defaultAddressCapMode != "shadow" {
		t.Fatalf("default -dht-address-cap is %q; want shadow — the veto is enabled by an owner call after the shadow run, never by default (cert §6.3)", defaultAddressCapMode)
	}
}

// TestR43b_AddressCapFlagIsDefinedInDaemon is a SOURCE gate: the flag literal exists in
// daemon.go. Checked: text only.
// RUNTIME GATE: TestR43b_AddressCapFlagParsesOffShadowOnDefaultShadow (the parse). The
// daemon's wiring of the parsed mode into node.SetAddressDiversity is UNGATED at
// runtime here (G-13, the cloudtest shadow run, observes it).
func TestR43b_AddressCapFlagIsDefinedInDaemon(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `"dht-address-cap"`) {
		t.Fatal("SOURCE GATE: the literal `\"dht-address-cap\"` is absent from daemon.go — the flag is not defined")
	}
}

// TestR43b_G11_DaemonAdoptsViaPickRelayNotKnownRelaysZero is a SOURCE gate on the
// gossiped-relay adoption path in daemon.go. Checked: text only — the literal
// `r := rs[0]` is gone and `pickRelay(` is called.
// RUNTIME GATE: TestR43b_G11_RelaySelectionSpreadsAndFailsOver (the selection). The
// daemon goroutine that adopts the pick and re-picks on a registration failure is
// UNGATED at runtime here (G-13 series B, top-relay share, observes the outcome).
func TestR43b_G11_DaemonAdoptsViaPickRelayNotKnownRelaysZero(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "r := rs[0]") {
		t.Fatal("SOURCE GATE: the literal `r := rs[0]` is still in daemon.go — every NATed pony adopts the lowest-ID relay for life (the herd the economist named as a precondition of `on`)")
	}
	if !strings.Contains(body, "pickRelay(") {
		t.Fatal("SOURCE GATE: no call to `pickRelay(` in daemon.go — the gossiped-relay adoption path does not use the de-herd selection")
	}
}

// TestR43b_G11_RelaySelectionSpreadsAndFailsOver — G-11. The de-herd: relay selection
// spreads NATed nodes across the gossiped relays (deterministic per node) and fails over
// past a relay that refused registration.
func TestR43b_G11_RelaySelectionSpreadsAndFailsOver(t *testing.T) {
	relays := []tcpnet.Peer{
		{ID: identity.FromSeed(1).NodeID(), Addr: "198.51.100.1:4002"},
		{ID: identity.FromSeed(2).NodeID(), Addr: "198.51.100.2:4002"},
		{ID: identity.FromSeed(3).NodeID(), Addr: "198.51.100.3:4002"},
	}
	load := map[ports.NodeID]int{}
	for i := int64(100); i < 130; i++ { // 30 NATed ponies
		self := identity.FromSeed(i).NodeID()
		p, ok := pickRelay(self, relays, nil)
		if !ok {
			t.Fatalf("pickRelay refused with 3 candidates")
		}
		if q, _ := pickRelay(self, relays, nil); q.ID != p.ID {
			t.Fatalf("G-11 RED: pickRelay is not deterministic per node (%s then %s)", p.ID.String()[:8], q.ID.String()[:8])
		}
		load[p.ID]++
	}
	for id, k := range load {
		if k > 15 {
			t.Fatalf("G-11 RED: relay %s carries %d of 30 NATed ponies (> 50%%) — the selection herds (loads %v)", id.String()[:8], k, load)
		}
	}
	self := identity.FromSeed(100).NodeID()
	first, _ := pickRelay(self, relays, nil)
	second, ok := pickRelay(self, relays, map[ports.NodeID]bool{first.ID: true})
	if !ok || second.ID == first.ID {
		t.Fatalf("G-11 RED: after relay %s refused registration pickRelay returned (%s, ok %v) — no fail-over", first.ID.String()[:8], second.ID.String()[:8], ok)
	}
	if _, ok := pickRelay(self, relays, map[ports.NodeID]bool{relays[0].ID: true, relays[1].ID: true, relays[2].ID: true}); ok {
		t.Fatal("G-11 RED: every relay refused, yet pickRelay returned one")
	}
}

// TestR43b_StatusExposesSeriesABE is a SOURCE gate on the /api/status JSON keys the
// economist's shadow decision reads: series A (would-refuse per width, direct/relayed),
// B (relay fan-in), E (group-density census). Checked: text only, the json tag literals
// in ui.go.
// UNGATED: the values themselves — G-13 (the cloudtest shadow run) is the runtime observer.
func TestR43b_StatusExposesSeriesABE(t *testing.T) {
	src, err := os.ReadFile("ui.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{`json:"addressCap"`, `json:"wouldRefuse"`, `json:"relayFanIn"`, `json:"groupCensus"`} {
		if !strings.Contains(string(src), tag) {
			t.Fatalf("SOURCE GATE: the json tag literal %s is absent from ui.go — /api/status does not expose that series; without A+B+E the owner cannot enable `on` (cert §6.3)", tag)
		}
	}
}
