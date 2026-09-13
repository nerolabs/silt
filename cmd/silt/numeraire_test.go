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

// The numéraire gates that need BOTH core/credit and core/relaypay. T-NUMERAIRE: U/p < Dλ ≤
// RelayIncrementBytes/RelayIncrementCredit.

// TestStrictParityHoldsByConstruction: Dλ > U/p from the shipped
// constants. Ablation: Dλ = U/p ⇒ RED.
func TestStrictParityHoldsByConstruction(t *testing.T) {
	if credit.ServeMintBytesPerCredit <= credit.DeliveryBytesPerCredit {
		t.Fatalf("Dλ %d ≤ U/p %d — STRICT parity (decision 3) is broken: a self-mint would pay at least what a witnessed delivery pays", credit.ServeMintBytesPerCredit, credit.DeliveryBytesPerCredit)
	}
	// The derivation, pinned as INDEPENDENT literals: a moved constant
	// must be re-check, not re-derived silently.
	if credit.DeliveryIncrementBytes != 262_144 || credit.DeliveryIncrementCredit != 1 || credit.ServeMintBytesPerCredit != 393_216 {
		t.Fatalf("(U, p, Dλ) = (%d, %d, %d), want (262,144, 1, 393,216) — the settled numéraire moved; re-check parity, Don't #7 and the pin together", credit.DeliveryIncrementBytes, credit.DeliveryIncrementCredit, credit.ServeMintBytesPerCredit)
	}
	if want := (3*credit.DeliveryIncrementBytes + 2*credit.DeliveryIncrementCredit - 1) / (2 * credit.DeliveryIncrementCredit); credit.ServeMintBytesPerCredit != want {
		t.Fatalf("Dλ %d is not ⌈3·U/(2·p)⌉ = %d — it must DERIVE from the price, never be pinned", credit.ServeMintBytesPerCredit, want)
	}
}

// TestDontSevenCeilingAgainstTheRelayPrice: Dλ ≤ the relay lane's bytes per credit under
// BOTH readings of Don't #7 — gross (the skim counts as reward to the content lane, the
// reading the project settled) — or a relay that stores nothing out-earns the node that
// served the bytes. The ceil in the derivation rounds TOWARD this bound, so it is
// asserted, not assumed. Ablation: Dλ = 524,289 ⇒ RED.
func TestDontSevenCeilingAgainstTheRelayPrice(t *testing.T) {
	relay := int64(relaypay.RelayIncrementBytes / relaypay.RelayIncrementCredit)
	if credit.ServeMintBytesPerCredit > relay {
		t.Fatalf("Dλ %d > relay bytes/credit %d — Don't #7 (gross reading): the relay out-earns the server per byte", credit.ServeMintBytesPerCredit, relay)
	}
	// The NET reading (the server's 7/8 take-home, Dλ ≤ 458,752) is NOT asserted: the
	// project settled the GROSS reading. The value clears both; a future re-tune the
	// gross reading admits and the net one refuses is the project call, not this
	// gate's.
}

// TestGrantOverSummedPriceHoldsThePin: one starter grant buys ≥ 64 GiB across the
// summed delivery + relay prices at the shipped constants; a delivery increment of 128
// KiB would NOT (the ablation), and the pin itself sits above the structural floor
// S_max × N/K derived from erasure.DefaultParams (never transcribed).
func TestGrantOverSummedPriceHoldsThePin(t *testing.T) {
	const grant = int64(500_000)
	relay := int64(relaypay.RelayIncrementBytes / relaypay.RelayIncrementCredit)
	got := grantOverSummedPriceBytes(grant, credit.DeliveryBytesPerCredit, relay)
	if got < credit.GrantOverRPinBytes {
		t.Fatalf("one grant buys %d bytes across the summed prices, below the 64 GiB pin %d", got, credit.GrantOverRPinBytes)
	}
	if got != 87_381_333_333 { // 81.38 GiB: 27 % above the pin
		t.Fatalf("summed-price purchasing power %d, want 87,381,333,333 (81.38 GiB) — a price moved", got)
	}
	if below := grantOverSummedPriceBytes(grant, 131_072, relay); below >= credit.GrantOverRPinBytes {
		t.Fatalf("U = 128 KiB buys %d bytes, expected BELOW the pin — the refusal would not fire on the ablation", below)
	}
	// The structural floor beneath the pin: the largest common object (owner, 2026-09-05:
	// movies up to ~30 GB) times the parity amplification N/K of a whole-object fetch.
	const maxCommonObjectBytes = int64(30_000_000_000)
	floor := maxCommonObjectBytes * int64(erasure.DefaultParams.N) / int64(erasure.DefaultParams.K)
	if credit.GrantOverRPinBytes < floor {
		t.Fatalf("the 64 GiB pin %d is below the structural floor S_max·N/K = %d — re-ratify the pin", credit.GrantOverRPinBytes, floor)
	}
}

// TestDaemonRefusesPricedLaneBelowThePinSourceGate: the daemon's priced-lane block calls the
// pin helper against credit.GrantOverRPinBytes. SOURCE GATE: a string in daemon.go.
// RUNTIME GATE: TestGrantOverSummedPriceHoldsThePin exercises the helper it names on the shipped
// constants and on the below-pin ablation.
func TestDaemonRefusesPricedLaneBelowThePinSourceGate(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "grantOverSummedPriceBytes(ledger.Grant(), credit.DeliveryBytesPerCredit, relaypay.RelayIncrementBytes/relaypay.RelayIncrementCredit); got < credit.GrantOverRPinBytes") {
		t.Fatal("SOURCE GATE: daemon.go no longer contains the priced-lane refusal below the grant/r pin — the string is gone")
	}
}

// TestPublishWarningFiresOnlyWhenThePublishShortPaysTheRepairer drives BOTH sides of the
// publish warning's rule and BOTH causes (for the zero arm for the truncation
// arm — BOULDER2 residual-closures research 2026-09-07 §2.6; the object cause added on
// the, 2026-09-09). The rule: warn iff THIS PUBLISH's real repair-bounty base is
// below the base the shipped default pays on a full frame. The shard is the object's own
// bytes when the object fits in one frame, so the size is part of the price. Every side
// is RUN here rather than described; SOURCE GATE below.
//
// Ablations that must go RED: (a) price on the chunk size alone (drop the objectBytes
// term) ⇒ a 1 KB object at the default says nothing while paying zero; (b) restore the
// earlier zero-only rule ⇒ bountyPriceWarning(52_412, unknown) returns "".
func TestPublishWarningFiresOnlyWhenThePublishShortPaysTheRepairer(t *testing.T) {
	// The threshold is DERIVED from the shipped default, not typed: the default pays 1
	// since F1 re-based the price on the one shard the payee moves (it paid 10 before).
	// tripwire: if this ever reads 0 the whole warning goes silent, ZERO arm included
	// — which is now also a refuse-to-start, TestRefusesAPublishDefaultThatPaysNoBounty.
	if got := shippedBountyBase(); got != 1 {
		t.Fatalf("the shipped default pays a base of %d, want 1 — the warning's threshold moved with it; re-read before re-pinning", got)
	}
	// SILENT, and it is the only silent case: a base at or above the default's. An UNSET
	// -chunk-size is DefaultChunkSize (pinned by the source gate below), so the GEOMETRY
	// cause cannot fire without a flag — but the OBJECT cause can and does, so this
	// table is NOT "everything published at the default". 262,120 B is the first
	// silent size; the firing side below drives 262,119.
	for _, c := range []struct {
		chunk  int64
		object int64
	}{
		{int64(pipeline.DefaultChunkSize), objectSizeUnknown},
		{int64(pipeline.DefaultChunkSize), 1_500_000},
		{int64(pipeline.DefaultChunkSize), 262_136}, // exactly one FULL frame: unchanged
		{int64(pipeline.DefaultChunkSize), 262_135}, // the largest object that re-addresses
		{int64(pipeline.DefaultChunkSize), 262_120}, // the FIRST silent size
		{int64(pipeline.DefaultChunkSize), 0},       // an EMPTY file stores no shard at all
		{1 << 20, objectSizeUnknown},
		{1 << 20, 1 << 22},
	} {
		if msg := bountyPriceWarning(c.chunk, c.object); msg != "" {
			t.Fatalf("-chunk-size %d, object %d (base %d >= the default's %d) warned: %q", c.chunk, c.object, credit.RepairBountyBase(erasure.DefaultParams.K, publishShardBytes(c.chunk, c.object)), shippedBountyBase(), msg)
		}
	}
	// CAUSE 1, the GEOMETRY. The zero arm below the derived
	// minimum.
	min := minBountyChunkBytes()
	if min != 262_128 {
		t.Fatalf("derived threshold %d, want 262,128 (one shard of witnessed fetch less the 16-byte tag)", min)
	}
	if msg := bountyPriceWarning(16_384, objectSizeUnknown); !strings.Contains(msg, "ZERO") || !strings.Contains(msg, "262128") {
		t.Fatalf("explicit 16 KiB chunk: no ZERO warning naming the threshold (%q)", msg)
	}
	if msg := bountyPriceWarning(min-1, objectSizeUnknown); !strings.Contains(msg, "ZERO") {
		t.Fatalf("a chunk one byte below the minimum did not warn ZERO: %q", msg)
	}
	if m := bountyPriceWarning(min, objectSizeUnknown); m != "" {
		t.Fatalf("a chunk AT the derived minimum pays a base of 1 and must be silent: %q", m)
	}
	// And the 64 KiB FORMER default, which used to warn TRUNCATES at
	// 20.0 % and now pays nothing at all: a 65,552-byte shard is a
	// quarter of one credit of fetch.
	if m := bountyPriceWarning(65_536, objectSizeUnknown); !strings.Contains(m, "ZERO") {
		t.Fatalf("the former 64 KiB default (a 65,552 B shard, 0.25006 credits) did not warn ZERO: %q", m)
	}
	// THE TRUNCATES ARM IS UNREACHABLE THROUGH THIS FUNCTION SINCE F1, and that is
	// run here rather than asserted in prose: the rule fires iff base <
	// shippedBountyBase, which is 1, so every firing publish has base 0 and takes the
	// ZERO arm. Sweep both causes across the whole firing range.
	for _, c := range []struct{ chunk, object int64 }{
		{16_384, objectSizeUnknown}, {65_536, objectSizeUnknown}, {min - 1, objectSizeUnknown},
		{int64(pipeline.DefaultChunkSize), 1_024}, {int64(pipeline.DefaultChunkSize), 100_000},
		{int64(pipeline.DefaultChunkSize), 262_119},
	} {
		m := bountyPriceWarning(c.chunk, c.object)
		if m == "" || strings.Contains(m, "TRUNCATES") || !strings.Contains(m, "ZERO") {
			t.Fatalf("-chunk-size %d, object %d: every firing publish must take the ZERO arm at a threshold of 1, got %q", c.chunk, c.object, m)
		}
	}
	// AND THE COST OF THAT, run: a publish worth an exact 1.99996 credits
	// pays 1 and is SILENT, a 50 % short-pay the publisher is never told
	// about. NARROWS. Widening the rule is refused here — it would speak on
	// the shipped default itself, the finding that closed it — so the
	// gap is disclosed, not closed.
	if m := bountyPriceWarning(524_264, objectSizeUnknown); m != "" {
		t.Fatalf("fixture: the 50 %% truncating geometry is expected SILENT at a threshold of 1, got %q", m)
	}
	if e5, loss := credit.RepairBountyTruncation(erasure.DefaultParams.K, publishShardBytes(524_264, objectSizeUnknown)); e5 != 199_996 || loss != 500 {
		t.Fatalf("the silent short-pay is %d/1e5 credits at %d tenths of a percent, want 199996 and 500", e5, loss)
	}
	// CAUSE 2, the OBJECT — the class the re-pricing creates, at the SHIPPED
	// DEFAULT chunk size, which the geometry arm can never see. Under F1 the whole
	// class pays a base of ZERO up to 262,119 B (26,190 B before; the 10.008×
	// widening is the accepted cost).
	for _, size := range []int64{1_024, 10_240, 26_190, 26_191, 100_000, 262_119} {
		m := bountyPriceWarning(int64(pipeline.DefaultChunkSize), size)
		if !strings.Contains(m, "ZERO") || !strings.Contains(m, "NO chunk size changes this") || !strings.Contains(m, "prepay-only") {
			t.Fatalf("a %d B object at the shipped default: want a ZERO warning naming the object cause and prepay-only durability, got %q", size, m)
		}
		if strings.Contains(m, "use -chunk-size >=") {
			t.Fatalf("a %d B object was told to raise -chunk-size, which cannot help it: %q", size, m)
		}
	}
	// The boundary itself did NOT move with F1 — only the arm did. 262,119 fires and
	// 262,120 is silent, before and after; it is in the silent table above too.
	if m := bountyPriceWarning(int64(pipeline.DefaultChunkSize), 262_120); m != "" {
		t.Fatalf("262,120 B is the first silent size and it warned: %q", m)
	}
	// SOURCE GATE — labelled. Its runtime cover is the silent-side table above, which
	// passes pipeline.DefaultChunkSize as a LITERAL and never reads the flag: the source
	// string is therefore the only thing connecting "the flag's default" to "the GEOMETRY
	// cause cannot fire on an unset -chunk-size". Move the flag default and no runtime
	// gate would see it. The second string is what makes the OBJECT cause reachable at all.
	for _, f := range []string{"main.go", "swarm.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), `warnBountyPrice(*chunkSize, fileSizeOrUnknown(f), os.Stderr)`) {
			t.Fatalf("SOURCE GATE: %s no longer prices the publish with the object's size — the string is gone (cover: the CAUSE 2 rows of this test)", f)
		}
		if !strings.Contains(string(src), `fs.Int("chunk-size", pipeline.DefaultChunkSize,`) {
			t.Fatalf("SOURCE GATE: %s no longer defaults -chunk-size to pipeline.DefaultChunkSize, so the GEOMETRY cause can now fire on an UNSET flag — every publish would warn on its chunk size, not just on its object (cover: the silent-side table of this test)", f)
		}
	}
}

// TestPaidSerialCapLiteralsMatchTheirSources: the paid-serial guard's cap is
// derived from the two anchor faces, which core/credit carries as duplicated literals
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

// TestDefaultChunkIsOneDeliveryCredit — 4′: the publish default is exactly the delivery
// increment (one chunk = one witnessed credit), and a full-frame shard of it pays a
// repair-bounty base of 1 with a truncation under 0.01 %. Ablation: move either constant
// alone ⇒ RED.
//
// The base moved 10 → 1 with F1, which re-based the price on
// the ONE shard the payee moves. Ground (i) of the 256 KiB default — one chunk is one
// delivery credit — is unchanged and now TIGHTER: 262,144 B is the exact zero-bounty cliff,
// to within crypto.Overhead. That 16-byte margin is why checkBountyDisclosureHeadroom exists.
func TestDefaultChunkIsOneDeliveryCredit(t *testing.T) {
	if int64(pipeline.DefaultChunkSize) != credit.DeliveryBytesPerCredit {
		t.Fatalf("pipeline.DefaultChunkSize %d != credit.DeliveryBytesPerCredit %d — one chunk must be one delivery credit", pipeline.DefaultChunkSize, credit.DeliveryBytesPerCredit)
	}
	if got := credit.RepairBountyBase(erasure.DefaultParams.K, int64(pipeline.DefaultChunkSize)+crypto.Overhead); got != 1 {
		t.Fatalf("the default geometry pays a base of %d, want 1 (F1: one shard of witnessed fetch; the D-S7 self-funding band is 3.60 stripe-retrievals per shard-repair at m̄ = 3)", got)
	}
	if msg := bountyPriceWarning(int64(pipeline.DefaultChunkSize), objectSizeUnknown); msg != "" {
		t.Fatalf("the shipped default warned: %q", msg)
	}
}

// TestRefusesAPublishDefaultThatPaysNoBounty drives the refuse-to-start F1 owes — canon
// rule 8's first arm on the one coupling F1 compresses: the publish default must stay at or
// above the chunk size that pays a non-zero base, or the publish warning's threshold falls
// to 0 and the whole disclosure goes permanently silent.
//
// Both arms, plus the margin. That the DAEMON calls it is a separate source gate,
// TestSourceGateDaemonRefusesTheZeroBountyDefault.
// Ablation: make the check return nil unconditionally ⇒ RED.
func TestRefusesAPublishDefaultThatPaysNoBounty(t *testing.T) {
	min := minBountyChunkBytes()
	if err := checkBountyDisclosureHeadroom(int64(pipeline.DefaultChunkSize), min); err != nil {
		t.Fatalf("the SHIPPED pair must start: %v", err)
	}
	err := checkBountyDisclosureHeadroom(min-1, min)
	if err == nil {
		t.Fatal("a publish default one byte below the minimum was accepted — the ZERO warning would go permanently silent")
	}
	for _, want := range []string{"ZERO repair bounty", "262127", "262128", "262144", "do NOT raise the chunk default"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal does not name %q: %v", want, err)
		}
	}
	// THE MARGIN, measured rather than recalled: 16 B today, against 235,945 B before F1.
	// It is crypto.Overhead exactly, which is why the check is a refusal and not a comment.
	if got := int64(pipeline.DefaultChunkSize) - min; got != crypto.Overhead {
		t.Fatalf("the margin above the zero cliff is %d B, want %d (crypto.Overhead) — re-read  §5 and §7, which CLOSED  with this refusal — the residual carries no register row because it is closed, and a closed residual is removed from the plan, not filed in it", got, int64(crypto.Overhead))
	}
}

// TestSourceGateDaemonRefusesTheZeroBountyDefault reads daemon.go's own text, so it can
// see a STRING and nothing else: that the shipped daemon calls the refusal with the two
// shipped constants. It says nothing about what the refusal decides.
//
// RUNTIME GATE: TestRefusesAPublishDefaultThatPaysNoBounty drives both arms of the
// decision and the 16-byte margin. This gate exists because those arms are pure-function
// cover — nothing else connects the check to the shipped binary, and a refusal nobody calls
// is a refusal that does not exist.
//
// Ablation: perturb the call in daemon.go ⇒ RED.
func TestSourceGateDaemonRefusesTheZeroBountyDefault(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "checkBountyDisclosureHeadroom(int64(pipeline.DefaultChunkSize), minBountyChunkBytes())") {
		t.Fatal("SOURCE GATE: daemon.go no longer contains the call that refuses to start on a publish default paying a zero repair bounty — the string is gone")
	}
}
