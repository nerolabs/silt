//go:build !bbootstrap

package main

// D-BB-BUILD-TAG (ratified 2026-09-05) — the daemon tier's default-build gates. Each one
// asserts an ABSENCE, so the file compiles only without the `bbootstrap` tag.

import (
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestR29aDefaultBuildHasNoBBootstrapFlag asserts a default silt binary does not
// RECOGNISE -bbootstrap. It drives the same seam daemon.go does, on a real FlagSet, so
// it observes the flag surface rather than reading source text.
//
// The consequence for an operator is `silt daemon -bbootstrap` failing at flag parse
// with "flag provided but not defined", which is the intended answer: the mechanism is
// not disabled, it is absent, and there is nothing to enable.
func TestR29aDefaultBuildHasNoBBootstrapFlag(t *testing.T) {
	fs := flag.NewFlagSet("silt", flag.ContinueOnError)
	fs.SetOutput(new(strings.Builder))
	on := registerBBootstrapFlag(fs)
	if f := fs.Lookup("bbootstrap"); f != nil {
		t.Fatalf("-bbootstrap is declared in a DEFAULT build (default %q). The instrument compiles only under the `bbootstrap` build tag (D-BB-BUILD-TAG)", f.DefValue)
	}
	if on() {
		t.Fatalf("registerBBootstrapFlag reports the instrument ON in a default build; it must be permanently false")
	}
	if err := fs.Parse([]string{"-bbootstrap"}); err == nil {
		t.Fatalf("a default build ACCEPTED -bbootstrap. It must be rejected as an unknown flag")
	}
}

// TestR29aDefaultBuildStatusHasNoBBootstrapKey asserts on the BYTES /api/status emits,
// which is the boundary that matters — the same boundary BB-20 asserts the floor's
// property on, one build over.
//
// It drives real requester traffic first, so the absence is not the absence of a census:
// there IS a census on that ledger, and the default binary has no way to render it.
// statusExtras is an empty embedded struct here, so encoding/json promotes no field and
// the key cannot appear.
func TestR29aDefaultBuildStatusHasNoBBootstrapKey(t *testing.T) {
	s, _, _, led := economyServer(t, 0)
	s.peerCount = func() int { return 0 } // nil func in this fixture; /api/status segfaults without it
	server := ports.HashBytes([]byte("srv"))
	// Ten requesters: credit.BBootstrapMinRequesters does not exist in a default build
	// (it is tagged out with the rest of the instrument), so the count is a literal here
	// and its only job is to make the census non-trivial.
	for i := 0; i < 10; i++ {
		led.RecordServe(server, ports.HashBytes([]byte{byte(i), 0x29}), ports.Hash{}, 4096)
	}
	// statusExtras must be EMPTY, not merely un-populated. A field added to it in a
	// default build would put the key back on the wire the moment anything filled it,
	// and the body check below only catches the populated case.
	if n := reflect.TypeOf(statusExtras{}).NumField(); n != 0 {
		t.Fatalf("statusExtras has %d field(s) in a DEFAULT build, want 0. It is embedded in the /api/status payload, so any field here is a key a default silt binary can publish (D-BB-BUILD-TAG)", n)
	}
	if s.statusExtra != nil {
		t.Fatalf("uiServer.statusExtra is non-nil in a DEFAULT build — nothing can set it, because the only implementation is behind the `bbootstrap` build tag")
	}

	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/status", nil)
	w := httptest.NewRecorder()
	s.guard(http.HandlerFunc(s.apiStatus)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status: %d, body %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "bBootstrap") {
		t.Fatalf("GET /api/status carries a bBootstrap key in a DEFAULT build: %s", body)
	}
	// And the payload is still well-formed with the empty extras embedded — an empty
	// embedded struct must contribute no key AND break nothing.
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &top); err != nil {
		t.Fatalf("decode status: %v (body %s)", err, body)
	}
	if _, ok := top["id"]; !ok {
		t.Fatalf("status payload lost its id key — the embedded statusExtras seam broke the rest of the block: %s", body)
	}
}
