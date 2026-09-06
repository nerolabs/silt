package main

import (
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/relaypay"
)

// The G-R212-7 numéraire gates that need BOTH core/credit and core/relaypay (cert §8.1;
// owner-ratified 2026-09-06). T-NUMERAIRE: U/p < Dλ ≤ RelayIncrementBytes/RelayIncrementCredit.

// TestGLambda1StrictParityHoldsByConstruction: Dλ > U/p from the shipped constants
// (G-λ-1). Ablation: Dλ = U/p ⇒ RED.
func TestGLambda1StrictParityHoldsByConstruction(t *testing.T) {
	if credit.ServeMintBytesPerCredit <= credit.DeliveryBytesPerCredit {
		t.Fatalf("Dλ %d ≤ U/p %d — STRICT parity (D-R2.9-DIRECTION ruling 3) is broken: a self-mint would pay at least what a witnessed delivery pays", credit.ServeMintBytesPerCredit, credit.DeliveryBytesPerCredit)
	}
	// The certified derivation, pinned as INDEPENDENT literals (cert §11): a moved constant
	// must be re-certified, not re-derived silently.
	if credit.DeliveryIncrementBytes != 262_144 || credit.DeliveryIncrementCredit != 1 || credit.ServeMintBytesPerCredit != 393_216 {
		t.Fatalf("(U, p, Dλ) = (%d, %d, %d), want (262,144, 1, 393,216) — the ratified numéraire moved; re-certify parity, Don't #7 and the pin together", credit.DeliveryIncrementBytes, credit.DeliveryIncrementCredit, credit.ServeMintBytesPerCredit)
	}
	if want := (3*credit.DeliveryIncrementBytes + 2*credit.DeliveryIncrementCredit - 1) / (2 * credit.DeliveryIncrementCredit); credit.ServeMintBytesPerCredit != want {
		t.Fatalf("Dλ %d is not ⌈3·U/(2·p)⌉ = %d — it must DERIVE from the price, never be pinned", credit.ServeMintBytesPerCredit, want)
	}
}

// TestGLambda2DontSevenCeilingAgainstTheRelayPrice: Dλ ≤ the relay lane's bytes per
// credit under BOTH readings of Don't #7 — gross (the skim counts as reward to the
// content lane, the reading the owner ratified) — or a relay that stores nothing out-earns
// the node that served the bytes (G-λ-2). The ceil in the derivation rounds TOWARD this
// bound, so it is asserted, not assumed. Ablation: Dλ = 524,289 ⇒ RED.
func TestGLambda2DontSevenCeilingAgainstTheRelayPrice(t *testing.T) {
	relay := int64(relaypay.RelayIncrementBytes / relaypay.RelayIncrementCredit)
	if credit.ServeMintBytesPerCredit > relay {
		t.Fatalf("Dλ %d > relay bytes/credit %d — Don't #7 (gross reading): the relay out-earns the server per byte", credit.ServeMintBytesPerCredit, relay)
	}
	// The NET reading (the server's 7/8 take-home, Dλ ≤ 458,752) is NOT asserted: the owner
	// ratified the GROSS reading (call 4, 2026-09-06). The certified value clears both; a
	// future re-tune the gross reading admits and the net one refuses is the owner's call,
	// not this gate's (blind PE N2).
}

// TestGLambda3GrantOverSummedPriceHoldsThePin: one starter grant buys ≥ 64 GiB across
// the summed delivery + relay prices at the shipped constants; a delivery increment of
// 128 KiB would NOT (the certification's ablation), and the pin itself sits above the
// structural floor S_max × N/K derived from erasure.DefaultParams (never transcribed).
func TestGLambda3GrantOverSummedPriceHoldsThePin(t *testing.T) {
	const grant = int64(500_000)
	relay := int64(relaypay.RelayIncrementBytes / relaypay.RelayIncrementCredit)
	got := grantOverSummedPriceBytes(grant, credit.DeliveryBytesPerCredit, relay)
	if got < credit.GrantOverRPinBytes {
		t.Fatalf("one grant buys %d bytes across the summed prices, below the 64 GiB pin %d", got, credit.GrantOverRPinBytes)
	}
	if got != 87_381_333_333 { // 81.38 GiB, cert §4: 27 % above the pin
		t.Fatalf("summed-price purchasing power %d, want 87,381,333,333 (81.38 GiB) — a price moved", got)
	}
	if below := grantOverSummedPriceBytes(grant, 131_072, relay); below >= credit.GrantOverRPinBytes {
		t.Fatalf("U = 128 KiB buys %d bytes, expected BELOW the pin — the refusal would not fire on the certified ablation", below)
	}
	// The structural floor beneath the pin: the largest common object (owner, 2026-09-05:
	// movies up to ~30 GB) times the parity amplification N/K of a whole-object fetch.
	const maxCommonObjectBytes = int64(30_000_000_000)
	floor := maxCommonObjectBytes * int64(erasure.DefaultParams.N) / int64(erasure.DefaultParams.K)
	if credit.GrantOverRPinBytes < floor {
		t.Fatalf("the 64 GiB pin %d is below the structural floor S_max·N/K = %d — re-ratify the pin", credit.GrantOverRPinBytes, floor)
	}
}

// TestGLambda3DaemonRefusesPricedLaneBelowThePinSourceGate: the daemon's priced-lane block
// calls the pin helper against credit.GrantOverRPinBytes. SOURCE GATE: a string in daemon.go.
// RUNTIME GATE: TestGLambda3GrantOverSummedPriceHoldsThePin exercises the helper it names
// on the shipped constants and on the certified below-pin ablation.
func TestGLambda3DaemonRefusesPricedLaneBelowThePinSourceGate(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "grantOverSummedPriceBytes(ledger.Grant(), credit.DeliveryBytesPerCredit, relaypay.RelayIncrementBytes/relaypay.RelayIncrementCredit); got < credit.GrantOverRPinBytes") {
		t.Fatal("SOURCE GATE: daemon.go no longer contains the priced-lane refusal below the grant/r pin (G-λ-3) — the string is gone")
	}
}

// TestGLambda8PublishWarningFiresOnlyForAnExplicitSmallChunk (blind PE M6): the publish-time
// warning speaks for an operator-set -chunk-size below the bounty minimum, is silent at or
// above it, and is silent for the shipped default (which is below the minimum — the owner's
// R-DEFAULT-CHUNK-BOUNTY-ZERO call, not a warning on every publish). SOURCE GATE below: both
// publish paths call it. RUNTIME GATE: the pure function arms here.
func TestGLambda8PublishWarningFiresOnlyForAnExplicitSmallChunk(t *testing.T) {
	// The shipped 64 KiB default pays a base of 2 (a shard is a whole ciphertext chunk):
	// NO warning, explicit or not. The earlier gate asserted a warning here on the
	// shard = chunk/k model; the Economist's 2026-09-06 advisory corrected the geometry.
	if msg := bountyChunkWarning(65_536, true); msg != "" {
		t.Fatalf("explicit 64 KiB chunk warned although its stripe pays a base of 2: %q", msg)
	}
	if msg := bountyChunkWarning(65_536, false); msg != "" {
		t.Fatalf("the shipped default fired the warning: %q", msg)
	}
	// An explicit 16 KiB chunk (stripe 10 × 16,400 = 164,000 B < one credit) warns, naming
	// the derived threshold; a chunk AT the threshold does not.
	min := minBountyChunkBytes()
	if min != 26_199 {
		t.Fatalf("derived threshold %d, want 26,199 at k=10 with a 16-byte tag", min)
	}
	if msg := bountyChunkWarning(16_384, true); !strings.Contains(msg, "ZERO") || !strings.Contains(msg, "26199") {
		t.Fatalf("explicit 16 KiB chunk: no warning naming the threshold (%q)", msg)
	}
	if msg := bountyChunkWarning(min, true); msg != "" {
		t.Fatalf("a chunk AT the minimum warned: %q", msg)
	}
	if msg := bountyChunkWarning(min-1, true); msg == "" {
		t.Fatal("a chunk one byte below the minimum did not warn")
	}
	for _, f := range []string{"main.go", "swarm.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), `warnBountyChunk(*chunkSize, flagWasSet(fs, "chunk-size"))`) {
			t.Fatalf("SOURCE GATE: %s no longer calls warnBountyChunk with the explicit-flag check — the string is gone", f)
		}
	}
}

// TestPaidSerialCapLiteralsMatchTheirSources — R2.9 B-4/B-5: the paid-serial guard's cap
// is derived from the two anchor faces, which core/credit carries as duplicated literals
// (it imports neither core/relaypay nor this package). Pinned here to their sources so a
// re-price of either moves the cap derivation or reddens this, never drifts silently.
func TestPaidSerialCapLiteralsMatchTheirSources(t *testing.T) {
	if credit.CapAnchorFace != relaypay.ShippedAnchorFace {
		t.Fatalf("credit.CapAnchorFace %d != relaypay.ShippedAnchorFace %d", credit.CapAnchorFace, relaypay.ShippedAnchorFace)
	}
	if credit.CapRelayBytesPerCredit != relaypay.RelayIncrementBytes/relaypay.RelayIncrementCredit {
		t.Fatalf("credit.CapRelayBytesPerCredit %d != relay price %d", credit.CapRelayBytesPerCredit, relaypay.RelayIncrementBytes/relaypay.RelayIncrementCredit)
	}
	// The ledger the daemon builds charges the same face (the ONE fee constant,
	// TestDaemonFeeIsTheRelayAnchorFace), so the runtime bytes-per-anchor equal the const.
	if got := int64(credit.New(relaypay.ShippedAnchorFace, 0).Fee()/credit.DeliveryIncrementCredit) * credit.DeliveryIncrementBytes; got != credit.DeliveryBytesPerAnchor {
		t.Fatalf("runtime bytes per delivery anchor %d != credit.DeliveryBytesPerAnchor %d", got, credit.DeliveryBytesPerAnchor)
	}
}
