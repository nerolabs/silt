//go:build bbootstrap

package main

// R2.9a — G-BB-12′ / G-BB-13′ Part A: the code, not a handoff note, keeps the B_bootstrap
// block off every reader that is not the operator. This file holds the (a) half — the
// startup refusal of a routable bind, owner-ratified 2026-09-05 ("refuse at startup",
// docs/decisions.md D-R2.9a-RUN-CALLS item 4) — and the two source gates that keep the
// refusal where it is and keep the Red-team's F6 unreachable on a daemon.

import (
	"os"
	"strings"
	"testing"
)

// TestR29aG12RoutableBindWithBBootstrapIsRefusedAtStartup pins the predicate on the
// operator's own flag string: loopback literals and "localhost" pass, everything that
// could put the block on a network — all-interfaces, a LAN or public literal, a hostname
// the node cannot vouch for — is refused, and the refusal names both flags so the
// operator knows which combination to change. With the flag off nothing is refused: a
// tagged binary run without -bbootstrap is a default daemon.
func TestR29aG12RoutableBindWithBBootstrapIsRefusedAtStartup(t *testing.T) {
	accept := []string{"127.0.0.1:8081", "localhost:8081", "[::1]:8081", "127.0.0.2:9000", "127.255.255.254:1"}
	refuse := []string{"0.0.0.0:8081", "[::]:8081", ":8081", "192.168.1.5:8081", "10.0.0.7:8081", "203.0.113.7:8081", "silt.example.org:8081", "[2001:db8::1]:8081"}
	for _, a := range accept {
		if err := bbootstrapRefuseRoutableBind(a, true); err != nil {
			t.Fatalf("-ui %q -bbootstrap was REFUSED; a loopback bind is the one configuration the run is allowed: %v", a, err)
		}
	}
	for _, a := range refuse {
		err := bbootstrapRefuseRoutableBind(a, true)
		if err == nil {
			t.Fatalf("-ui %q -bbootstrap was ACCEPTED: the histogram would be published on a routable bind (G-BB-13′ Part A refuses this at startup)", a)
		}
		for _, must := range []string{"-bbootstrap", "-ui", a, "loopback"} {
			if !strings.Contains(err.Error(), must) {
				t.Fatalf("refusal for %q does not name %q — the operator must be told which flags and which fix: %v", a, must, err)
			}
		}
		// The same bind with the flag OFF is an ordinary daemon and is not refused.
		if err := bbootstrapRefuseRoutableBind(a, false); err != nil {
			t.Fatalf("-ui %q WITHOUT -bbootstrap was refused; the refusal must bind only the flag combination: %v", a, err)
		}
	}
	// No UI at all: nothing is published, nothing to refuse.
	if err := bbootstrapRefuseRoutableBind("", true); err != nil {
		t.Fatalf("-bbootstrap with no -ui was refused: %v", err)
	}
}

// TestR29aG12DaemonRefusesBeforeItBindsOrMintsAToken is a source gate on daemon.go: the
// refusal runs inside the -ui block, BEFORE loadOrCreateUIToken and BEFORE ui.serve, so a
// refused combination binds no socket and leaves no token file behind. Order is the
// property; a call that moved after the bind would refuse a daemon that is already
// listening.
func TestR29aG12DaemonRefusesBeforeItBindsOrMintsAToken(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	refuse := strings.Index(s, "bbootstrapRefuseRoutableBind(*uiAddr, bbootstrapOn())")
	if refuse < 0 {
		t.Fatalf("SOURCE GATE: daemon.go does not call bbootstrapRefuseRoutableBind(*uiAddr, bbootstrapOn()) — G-BB-13′ Part A's startup refusal is gone")
	}
	mint := strings.Index(s, "loadOrCreateUIToken(*storeDir)")
	serve := strings.Index(s, "ui.serve(*uiAddr)")
	if mint < 0 || serve < 0 {
		t.Fatalf("SOURCE GATE: daemon.go lost the literal loadOrCreateUIToken(*storeDir) or ui.serve(*uiAddr) this gate anchors on")
	}
	if refuse > mint || refuse > serve {
		t.Fatalf("SOURCE GATE: the refusal (byte %d) must run BEFORE the token is minted (byte %d) and BEFORE the UI is bound (byte %d)", refuse, mint, serve)
	}
}

// TestR29aG12ClientSubcommandWiresNoInstrument keeps the Red-team's F6 unreachable on the
// tree: -allow-web-origin (a page on an allow-listed web origin reads the local API with
// no token) exists ONLY on the `client` subcommand, and the client wires neither the flag
// nor the renderer, so no build and no flag combination puts the histogram behind an
// allow-listed origin. Two literals must stay absent from client.go and one must stay
// present, so a future "let the client publish it too" is a deliberate, reviewed change.
func TestR29aG12ClientSubcommandWiresNoInstrument(t *testing.T) {
	src, err := os.ReadFile("client.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, `"allow-web-origin"`) {
		t.Fatalf("SOURCE GATE: client.go no longer declares -allow-web-origin; if it moved to the daemon, the F6 exposure is live there and this gate must be rewritten, not deleted")
	}
	for _, forbidden := range []string{"registerBBootstrapFlag(", "bbootstrapWireUI(", "bbootstrapInject("} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("SOURCE GATE: client.go calls %s — the client subcommand carries -allow-web-origin, so wiring the instrument there publishes the histogram to an allow-listed web origin with no token (Red-team F6)", forbidden)
		}
	}
	dsrc, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dsrc), `"allow-web-origin"`) {
		t.Fatalf("SOURCE GATE: daemon.go declares -allow-web-origin; the daemon is the one place the instrument is wired, so the F6 origin bypass is now reachable there and needs its own gate before this one is relaxed")
	}
}
