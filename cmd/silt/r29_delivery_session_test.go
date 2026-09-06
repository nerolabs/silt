package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/relaypay"
)

// R2.9 node-half gates that need core/credit, core/demand AND core/relaypay (the
// G-R212-8 certification §8: G-λ-8-1, G-λ-8-2) plus B-11.

// TestDeliverySessionCeilingIsDerivedFromTheFace — G-λ-8-1. D_max == ⌊f/p⌋·U and
// k_max_delivery == 1, computed from relaypay.ShippedAnchorFace and credit.Delivery*;
// the wire bound demand.MaxAnchorsPerOpen equals the derivation. Ablation: pin D_max to
// a literal, or move f to 25,000 and watch D_max fail to follow.
func TestDeliverySessionCeilingIsDerivedFromTheFace(t *testing.T) {
	dMax, kMax := deliverySessionCeiling(relaypay.ShippedAnchorFace)
	if dMax != 13_107_200_000 {
		t.Fatalf("D_max %d, want ⌊f/p⌋·U = 13,107,200,000 (12.21 GiB)", dMax)
	}
	if credit.DeliveryBytesPerAnchor != dMax {
		t.Fatalf("credit.DeliveryBytesPerAnchor %d != the derived D_max %d", credit.DeliveryBytesPerAnchor, dMax)
	}
	if kMax != 1 {
		t.Fatalf("k_max_delivery %d, want 1", kMax)
	}
	if demand.MaxAnchorsPerOpen != kMax {
		t.Fatalf("demand.MaxAnchorsPerOpen %d != the derived k_max %d — the wire bound drifted from the derivation", demand.MaxAnchorsPerOpen, kMax)
	}
	// The derivation FOLLOWS the face: a halved face halves the ceiling and keeps k_max.
	if d2, k2 := deliverySessionCeiling(relaypay.ShippedAnchorFace / 2); d2 != dMax/2 || k2 != 1 {
		t.Fatalf("at half the face: D_max %d (want %d), k_max %d", d2, dMax/2, k2)
	}
}

// TestGrantFundsThePinInWholeFaces — G-λ-8-2. ⌈B_pin/D_max⌉ + ⌈B_pin/relayBytesPerAnchor⌉
// ≤ ⌊g/f⌋ at the runtime constants (6 + 3 = 9 ≤ 10, one face of margin); a raised face, a
// lowered grant, a lowered U or a lowered relay increment must each REFUSE.
func TestGrantFundsThePinInWholeFaces(t *testing.T) {
	const grant = int64(500_000)
	relay := int64(relaypay.RelayIncrementBytes / relaypay.RelayIncrementCredit)
	need, have, ok := grantFundsThePinInWholeFaces(grant, relaypay.ShippedAnchorFace, relay)
	if !ok || need != 9 || have != 10 {
		t.Fatalf("whole-face pin: need %d have %d ok %v, want 9 ≤ 10", need, have, ok)
	}
	// Raising the face lowers BOTH face counts the pin needs and the faces a grant holds;
	// the cliff is a 4× face: 2 faces cannot fund ⌈64/48.8⌉ + ⌈64/97.7⌉ = 3.
	if need, have, ok := grantFundsThePinInWholeFaces(grant, 200_000, relay); ok || need != 3 || have != 2 {
		t.Fatalf("a 4× face must refuse: need %d have %d ok %v", need, have, ok)
	}
	if _, _, ok := grantFundsThePinInWholeFaces(400_000, relaypay.ShippedAnchorFace, relay); ok {
		t.Fatal("a lowered grant (8 faces) must refuse")
	}
	if _, _, ok := grantFundsThePinInWholeFaces(grant, relaypay.ShippedAnchorFace, relay/4); ok {
		t.Fatal("a quartered relay increment (12 relay faces) must refuse")
	}
}

// TestAffordabilityLineIsAnnounced — B-11. The S5 line carries the numbers computed from
// the constants; a hand-typed number cannot drift because there is none.
func TestAffordabilityLineIsAnnounced(t *testing.T) {
	line := deliveryAffordabilityLine(500_000, relaypay.ShippedAnchorFace, relaypay.RelayIncrementBytes/relaypay.RelayIncrementCredit, "1h0m0s")
	for _, want := range []string{
		"delivery settlement: p=1 credit per 262144 B", "U/p=262144 B/credit", "Dλ=393216 B/credit", "PF 1.50",
		"anchor face 50000 funds 50000 increments = 13107200000 B (12.21 GiB) per session, k_max=1",
		"one grant = 10 faces, the 64 GiB pin needs 9 faces", "idle window 1h0m0s", "DEPOSIT returned to the fetcher", "5-epoch guard window",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("affordability line lacks %q:\n%s", want, line)
		}
	}
}

// TestR29DaemonRefusalsAreWiredAtStartup_Source — the two R2.9 start-up refusals, gated
// where the RUNTIME value is read (blind PE item 2, the R2.12 source-gate shape): the
// daemon must (i) refuse -accept-delivery-receipts below the idle-window floor on a line
// naming the flag and the floor, and (ii) CALL grantFundsThePinInWholeFaces on the
// ledger's own grant and fee (never a literal). This gate sees STRINGS and ORDER only.
// RUNTIME GATE: TestDeliveryIdleWindowIsRefuseUntilSet (e2e: the daemon exits with the
// refusal naming the flag, unset and below the floor). The whole-face pin refusal's
// runtime arm is UNGATED: R-G-LAMBDA-8-2-RUNTIME (it cannot fire at the shipped constants
// — 9 of 10 faces fit — so no launch can reach it without moving a ratified price).
func TestR29DaemonRefusalsAreWiredAtStartup_Source(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	body := stripLineComments(string(src))
	if !strings.Contains(body, "*deliveryIdle < deliveryIdleFloor") {
		t.Fatal("SOURCE GATE: daemon.go no longer compares -delivery-idle-window against deliveryIdleFloor before enabling the lane — the refuse-until-set (C9) is gone")
	}
	line := lineContaining(body, "-accept-delivery-receipts: refusing to start — set -delivery-idle-window")
	if line == "" {
		t.Fatal("SOURCE GATE: the idle-window refusal line (naming the flag and 'refusing to start') is gone from daemon.go")
	}
	if !strings.Contains(body, "grantFundsThePinInWholeFaces(ledger.Grant(), ledger.Fee(), relaypay.RelayIncrementBytes/relaypay.RelayIncrementCredit)") {
		t.Fatal("SOURCE GATE: daemon.go does not call grantFundsThePinInWholeFaces on the ledger's grant and fee — G-λ-8-2 is a pure function nobody reads at start-up")
	}
	if deliveryIdleFloor < time.Second {
		t.Fatalf("SOURCE GATE: deliveryIdleFloor %s below one second — the idle/2 wall-clock ticker interval would round to zero and panic (PE item 3); the runtime cover is e2e TestDeliveryIdleWindowIsRefuseUntilSet", deliveryIdleFloor)
	}
}

func lineContaining(body, needle string) string {
	i := strings.Index(body, needle)
	if i < 0 {
		return ""
	}
	lo := strings.LastIndex(body[:i], "\n") + 1
	hi := strings.Index(body[i:], "\n")
	if hi < 0 {
		return body[lo:]
	}
	return body[lo : i+hi]
}
