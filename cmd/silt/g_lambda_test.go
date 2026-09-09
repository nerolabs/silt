package main

import (
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/pipeline"
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

// TestGLambda8PublishWarningFiresOnlyWhenThePublishShortPaysTheRepairer drives BOTH sides
// of the publish warning's rule and BOTH causes (G-λ-8 for the zero arm, G-BT-1 for the
// truncation arm — BOULDER2 residual-closures certification 2026-09-07 §2.6; the object
// cause added on the blind PE's B-3, 2026-09-09). The rule: warn iff THIS PUBLISH's real
// repair-bounty base is below the base the shipped default pays on a full frame. The
// shard is the object's own bytes when the object fits in one frame, so the size is part
// of the price. Every side is RUN here rather than described; SOURCE GATE below.
//
// Ablations that must go RED: (a) price on the chunk size alone (drop the objectBytes
// term) ⇒ a 1 KB object at the default says nothing while paying zero; (b) restore the
// pre-G-BT-1 zero-only rule ⇒ bountyPriceWarning(52_412, unknown) returns "".
func TestGLambda8PublishWarningFiresOnlyWhenThePublishShortPaysTheRepairer(t *testing.T) {
	// The threshold is DERIVED from the shipped default, not typed: the default pays 10.
	// N-7 tripwire: if this ever reads 0 the whole warning goes silent, ZERO arm included.
	if got := shippedBountyBase(); got != 10 {
		t.Fatalf("the shipped default pays a base of %d, want 10 — the warning's threshold moved with it; re-read G-BT-1 before re-pinning", got)
	}
	// SILENT, and it is the only silent case: a base at or above the default's. That
	// covers every full-frame object at the default and everything larger. An UNSET
	// -chunk-size is DefaultChunkSize (pinned by the source gate below), so it can only
	// land here — which is why no explicit-flag test is needed any more.
	for _, c := range []struct {
		chunk  int64
		object int64
	}{
		{int64(pipeline.DefaultChunkSize), objectSizeUnknown},
		{int64(pipeline.DefaultChunkSize), 1_500_000},
		{int64(pipeline.DefaultChunkSize), 262_136}, // exactly one FULL frame: unchanged
		{int64(pipeline.DefaultChunkSize), 262_120}, // the largest short frame that still pays 10
		{1 << 20, objectSizeUnknown},
		{1 << 20, 1 << 22},
	} {
		if msg := bountyPriceWarning(c.chunk, c.object); msg != "" {
			t.Fatalf("-chunk-size %d, object %d (base %d >= the default's %d) warned: %q", c.chunk, c.object, credit.RepairBountyBase(erasure.DefaultParams.K, publishShardBytes(c.chunk, c.object)), shippedBountyBase(), msg)
		}
	}
	// CAUSE 1, the GEOMETRY. The zero arm below the derived minimum...
	min := minBountyChunkBytes()
	if min != 26_199 {
		t.Fatalf("derived threshold %d, want 26,199 at k=10 with a 16-byte tag", min)
	}
	if msg := bountyPriceWarning(16_384, objectSizeUnknown); !strings.Contains(msg, "ZERO") || !strings.Contains(msg, "26199") {
		t.Fatalf("explicit 16 KiB chunk: no ZERO warning naming the threshold (%q)", msg)
	}
	if msg := bountyPriceWarning(min-1, objectSizeUnknown); !strings.Contains(msg, "ZERO") {
		t.Fatalf("a chunk one byte below the minimum did not warn ZERO: %q", msg)
	}
	// ... and the truncation arm at the certification's worst un-warned case. A
	// 52,412-byte chunk is a 52,428-byte shard: k·shardBytes = 524,280 B, an exact price
	// of 1.99996 credits paid as 1. Every figure must be in the sentence.
	msg := bountyPriceWarning(52_412, objectSizeUnknown)
	for _, want := range []string{"TRUNCATES", "52412", "52428", "1.99996", " 1 ", "50.0%"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("the 52,412-byte truncation warning does not name %q: %q", want, msg)
		}
	}
	if strings.Contains(msg, "ZERO") {
		t.Fatalf("a geometry paying a base of 1 was called ZERO: %q", msg)
	}
	if m := bountyPriceWarning(min, objectSizeUnknown); !strings.Contains(m, "TRUNCATES") {
		t.Fatalf("a chunk AT the zero minimum (base 1) did not take the truncation arm: %q", m)
	}
	if m := bountyPriceWarning(65_536, objectSizeUnknown); !strings.Contains(m, "TRUNCATES") || !strings.Contains(m, "20.0%") {
		t.Fatalf("the former 64 KiB default (base 2 of an exact 2.5006) did not warn at 20.0%%: %q", m)
	}
	// CAUSE 2, the OBJECT — the class R-SHORT-FINAL-STRIPE creates, at the SHIPPED
	// DEFAULT chunk size, which the geometry arm can never see. 1 KB and 10 KB pay a base
	// of ZERO; 100 KB pays 3 of an exact 3.81561, a 21.4 % cut.
	for _, size := range []int64{1_024, 10_240, 26_190} {
		m := bountyPriceWarning(int64(pipeline.DefaultChunkSize), size)
		if !strings.Contains(m, "ZERO") || !strings.Contains(m, "NO chunk size changes this") || !strings.Contains(m, "prepay-only") {
			t.Fatalf("a %d B object at the shipped default: want a ZERO warning naming the object cause and prepay-only durability, got %q", size, m)
		}
		if strings.Contains(m, "use -chunk-size >=") {
			t.Fatalf("a %d B object was told to raise -chunk-size, which cannot help it: %q", size, m)
		}
	}
	if m := bountyPriceWarning(int64(pipeline.DefaultChunkSize), 100_000); !strings.Contains(m, "TRUNCATES") || !strings.Contains(m, "100024") || !strings.Contains(m, "3.81561") || !strings.Contains(m, "21.4%") {
		t.Fatalf("a 100 KB object at the shipped default did not warn with its own figures: %q", m)
	}
	// The two boundaries of the object arm at the shipped default, both sides, derived:
	// base 0 at or below 26,190 B, and below the default's 10 at or below 262,119 B.
	if m := bountyPriceWarning(int64(pipeline.DefaultChunkSize), 26_191); strings.Contains(m, "ZERO") {
		t.Fatalf("26,191 B is one byte above the zero band and was still called ZERO: %q", m)
	}
	if m := bountyPriceWarning(int64(pipeline.DefaultChunkSize), 262_119); !strings.Contains(m, "TRUNCATES") {
		t.Fatalf("the largest short-paying object (262,119 B) did not warn: %q", m)
	}
	// SOURCE GATE — labelled, and its runtime cover is the silent-side table above: the
	// rule's complement is closed over an UNSET -chunk-size only because the flag's
	// default IS pipeline.DefaultChunkSize, and only because both publish paths hand the
	// warning the object's size rather than the geometry alone.
	for _, f := range []string{"main.go", "swarm.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), `warnBountyPrice(*chunkSize, fileSizeOrUnknown(f), os.Stderr)`) {
			t.Fatalf("SOURCE GATE: %s no longer prices the publish with the object's size — the string is gone (cover: the CAUSE 2 rows of this test)", f)
		}
		if !strings.Contains(string(src), `fs.Int("chunk-size", pipeline.DefaultChunkSize,`) {
			t.Fatalf("SOURCE GATE: %s no longer defaults -chunk-size to pipeline.DefaultChunkSize, so an UNSET flag can now reach the warning's firing side (cover: the silent-side table of this test)", f)
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

// TestDefaultChunkIsOneDeliveryCredit — D-R2.9-NODE-HALF-CALLS 4′: the publish default is
// exactly the delivery increment (one chunk = one witnessed credit), and a k = 10 stripe of
// it pays a repair-bounty base of 10 with a truncation under 0.01 %. Ablation: move either
// constant alone ⇒ RED.
func TestDefaultChunkIsOneDeliveryCredit(t *testing.T) {
	if int64(pipeline.DefaultChunkSize) != credit.DeliveryBytesPerCredit {
		t.Fatalf("pipeline.DefaultChunkSize %d != credit.DeliveryBytesPerCredit %d — one chunk must be one delivery credit", pipeline.DefaultChunkSize, credit.DeliveryBytesPerCredit)
	}
	if got := credit.RepairBountyBase(erasure.DefaultParams.K, int64(pipeline.DefaultChunkSize)+crypto.Overhead); got != 10 {
		t.Fatalf("the default geometry pays a base of %d, want 10 (the certified D-S7 threshold of 36 retrievals per repair holds exactly)", got)
	}
	if msg := bountyPriceWarning(int64(pipeline.DefaultChunkSize), objectSizeUnknown); msg != "" {
		t.Fatalf("the shipped default warned: %q", msg)
	}
}
