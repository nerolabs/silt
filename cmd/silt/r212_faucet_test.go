package main

// R2.12 — the daemon half of the faucet rate limit: flag refusal, the start-up assertion
// that ties capacity × (grant/fee) × (W+1) to the guard cap, the owner grant's place in
// daemon.go, and the telemetry's posture under -privacy.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// TestR212FaucetFlagsRefuseHalfAndUnsafeConfigurations: both rate flags or neither; a
// negative value refuses; a floor without a bucket refuses; a floor above the grant
// refuses; a capacity whose worst-case guard occupancy exceeds a quarter of the paid-serial
// cap refuses and names the arithmetic; a valid configuration configures the ledger.
func TestR212FaucetFlagsRefuseHalfAndUnsafeConfigurations(t *testing.T) {
	const grant, fee = int64(500_000), int64(50_000)
	fresh := func() *credit.Ledger { return credit.New(fee, grant) }
	if err := faucetConfigure(fresh(), 0, 0, 0, grant, fee); err != nil {
		t.Fatalf("both flags at 0 (unlimited) refused: %v", err)
	}
	for _, c := range [][3]int64{{256, 0, 0}, {0, 256, 0}, {-1, 256, 0}, {256, -1, 0}, {0, 0, 50_000}, {256, 256, -1}, {256, 256, grant + 1}} {
		if err := faucetConfigure(fresh(), c[0], c[1], c[2], grant, fee); err == nil {
			t.Fatalf("configuration %v was ACCEPTED", c)
		}
	}
	// The guard assertion: capacity × (grant/fee) × (W+1) ≤ MaxPaidSerial/4.
	// At grant/fee = 10 and W+1 = 5 the largest admissible capacity is MaxPaidSerial/200.
	limitCap := int64(credit.MaxPaidSerial) / 4 / (grant / fee) / int64(credit.PaidSerialWindow+1)
	if err := faucetConfigure(fresh(), limitCap, 1, 0, grant, fee); err != nil {
		t.Fatalf("capacity %d (exactly a quarter of the cap) refused: %v", limitCap, err)
	}
	err := faucetConfigure(fresh(), limitCap+1, 1, 0, grant, fee)
	if err == nil || !strings.Contains(err.Error(), "guard") {
		t.Fatalf("capacity %d (over a quarter of the cap) accepted or refusal does not name the guard: %v", limitCap+1, err)
	}
	// The Economist's recommendation passes with headroom.
	l := fresh()
	if err := faucetConfigure(l, 256, 256, 0, grant, fee); err != nil {
		t.Fatalf("256/256 refused: %v", err)
	}
	st := l.FaucetStats()
	if !st.Configured || st.Capacity != 256 || st.Refill != 256 || st.IntervalNanos != 3_600_000_000_000 || st.Level != 256 {
		t.Fatalf("configured stats: %+v", st)
	}
	// And the degrade shape.
	l2 := fresh()
	if err := faucetConfigure(l2, 256, 256, fee, grant, fee); err != nil || l2.FaucetStats().DenyFloor != fee {
		t.Fatalf("degrade configuration: err %v stats %+v", err, l2.FaucetStats())
	}
}

// TestR212DaemonGrantsTheOwnerBeforeAnyOtherAccount is a source gate: daemon.go configures
// the faucet and grants the node's own id right after the ledger is built, before the
// ledger is attached to the node (nd.SetLedger) and so before any other account can exist.
// RUNTIME GATE: core/credit TestR212OwnerIsGrantedUnmetered observes the grant; this adds
// only the ORDER at the daemon's call site.
func TestR212DaemonGrantsTheOwnerBeforeAnyOtherAccount(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	newL := strings.Index(s, "ledger := credit.New(")
	conf := strings.Index(s, "faucetConfigure(ledger, *grantCapacity, *grantPerHour, *grantDenyFloor")
	owner := strings.Index(s, "ledger.GrantOwner(id)")
	attach := strings.Index(s, "nd.SetLedger(")
	if newL < 0 || conf < 0 || owner < 0 || attach < 0 {
		t.Fatalf("SOURCE GATE: daemon.go lacks a literal: New %d configure %d GrantOwner %d SetLedger %d", newL, conf, owner, attach)
	}
	if !(newL < conf && conf < owner && owner < attach) {
		t.Fatalf("SOURCE GATE: order must be credit.New (%d) → faucetConfigure (%d) → GrantOwner (%d) → nd.SetLedger (%d): the owner grant must land before any other account can be registered", newL, conf, owner, attach)
	}
}

// TestR212FaucetTelemetryIsWithheldWithTheCounters: the faucet block rides GET /api/status
// as a sibling of stats — present (configured or not) for the operator and for any reader
// on a -privacy=off node, ABSENT under countersWithheld for an unauthenticated reader on a
// -privacy=on node.
func TestR212FaucetTelemetryIsWithheldWithTheCounters(t *testing.T) {
	s, led := statusServer(t)
	led.SetFaucet(256, 256, 3_600_000_000_000, 0, func() int64 { return 0 })
	led.GrantOwner(ports.HashBytes([]byte("self")))
	s.privacy = true
	pub := privacyGet(t, s, s.apiStatus, "/api/status", "", "")
	if _, has := pub["faucet"]; has {
		t.Fatalf("privacy=on, untokened: faucet block published: %s", pub["faucet"])
	}
	if string(pub["countersWithheld"]) != "true" {
		t.Fatalf("marker missing")
	}
	op := privacyGet(t, s, s.apiStatus, "/api/status", "Bearer tok", "")
	var f faucetInfo
	if err := json.Unmarshal(op["faucet"], &f); err != nil || !f.Configured || f.Capacity != 256 || f.GrantsIssued != 1 {
		t.Fatalf("operator's faucet block = %s (err %v)", op["faucet"], err)
	}
	// Unconfigured: the block is PRESENT with configured false — "unlimited" is not "withheld".
	s2, _ := statusServer(t)
	s2.privacy = false
	pub2 := privacyGet(t, s2, s2.apiStatus, "/api/status", "", "")
	var f2 faucetInfo
	if err := json.Unmarshal(pub2["faucet"], &f2); err != nil || f2.Configured {
		t.Fatalf("unconfigured faucet block = %s (err %v), want configured:false present", pub2["faucet"], err)
	}
}
