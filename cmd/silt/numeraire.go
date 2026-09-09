package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/pipeline"
)

// grantOverSummedPriceBytes is how many bytes one starter grant buys when a NAT'd
// fetcher pays BOTH lane prices at once: g / (1/D_delivery + 1/D_relay) =
// g·D_delivery·D_relay / (D_delivery + D_relay). The 64 GiB grant/r pin is read on this
// SUM (G-R212-6); the priced-lane start-up refusal compares against it (G-λ-3).
func grantOverSummedPriceBytes(grant, deliveryBytesPerCredit, relayBytesPerCredit int64) int64 {
	if deliveryBytesPerCredit <= 0 || relayBytesPerCredit <= 0 {
		return 0
	}
	return grant * deliveryBytesPerCredit * relayBytesPerCredit / (deliveryBytesPerCredit + relayBytesPerCredit)
}

// objectSizeUnknown is the objectBytes a caller passes when it cannot cheaply learn the
// length it is about to publish (a pipe, a stream). The warning then prices the GEOMETRY
// alone, which is the only thing it can honestly say about such a publish.
const objectSizeUnknown = int64(-1)

// fileSizeOrUnknown is the length of an already-open publish source, or objectSizeUnknown
// when it has none a Stat can report (a pipe, a device). It never fails the publish: a
// missing size costs the object arm of the warning, nothing else.
func fileSizeOrUnknown(f *os.File) int64 {
	if f == nil {
		return objectSizeUnknown
	}
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return objectSizeUnknown
	}
	return fi.Size()
}

// warnBountyPrice names, at publish time, a publish whose stripe SHORT-PAYS the repair
// bounty. The base is an integer floor of the witnessed fetch price, so k × shardBytes
// below one credit of fetch rounds to nothing (G-λ-8, G-R212-7) and k × shardBytes just
// above it rounds away up to half the repairer's wage (R-BOUNTY-TRUNCATION, G-BT-1).
//
// TWO things can put a publish there, and both are named because a publisher can act on
// neither once the object is stored. (1) A chunk size the operator chose. (2) The OBJECT:
// since R-SHORT-FINAL-STRIPE a single-frame object is stored at its true length, so ITS
// shard is its own bytes and no chunk size changes that — at the shipped default every
// object of 26,190 B or less pays a base of ZERO. Case (2) is created by this same
// change, so leaving it silent would ship a warning that misses the class it invented
// (blind PE B-3, 2026-09-09).
//
// The daemon has no publish geometry at start-up to refuse on, and the judge that names a
// zero at settlement is a caretaker the publisher neither runs nor sees, so this is the
// one place the publisher is told.
func warnBountyPrice(chunkBytes int, objectBytes int64, w io.Writer) {
	if msg := bountyPriceWarning(int64(chunkBytes), objectBytes); msg != "" {
		fmt.Fprintln(w, msg)
	}
}

// minBountyChunkBytes is the publish-time ZERO threshold at the shipped erasure geometry.
func minBountyChunkBytes() int64 {
	return credit.MinBountyChunkBytesFor(erasure.DefaultParams.K, crypto.Overhead)
}

// shippedBountyBase is the repair-bounty base the SHIPPED publish default pays on a
// full-frame object: the warning's threshold, DERIVED from the two shipped constants and
// never typed. It is 10 today (a 262,160-byte shard, exact 10.00061), which is what makes
// the rule below have a closed complement — warn iff this publish pays a smaller base
// than the shipped default's — so a default publish of a full-frame object is silent by
// construction rather than by a hand-kept number.
//
// The coupling to watch: if DefaultChunkSize ever dropped below minBountyChunkBytes this
// would be 0 and the whole warning, ZERO arm included, would go permanently silent. The
// tripwire is in the gate, which asserts this is 10 and says to re-read G-BT-1 on any
// move of the default (blind PE N-7).
func shippedBountyBase() int64 {
	return credit.RepairBountyBase(erasure.DefaultParams.K, int64(pipeline.DefaultChunkSize)+crypto.Overhead)
}

// publishShardBytes is the ciphertext shard a repair of this publish will actually pull:
// pipeline.DataFrameSize decides the frame — the SAME rule pipeline.splitFile implements,
// read from the same two inputs rather than restated — plus the GCM tag. An unknown
// object length prices the chunk geometry alone.
func publishShardBytes(chunkBytes, objectBytes int64) int64 {
	frame := int(chunkBytes)
	if objectBytes >= 0 {
		frame = pipeline.DataFrameSize(int(objectBytes), int(chunkBytes))
	}
	return int64(frame) + crypto.Overhead
}

// bountyPriceWarning is the pure form of warnBountyPrice: the warning text, or "" when
// nothing should be said. Every number is the judge's own arithmetic on the shard this
// publish will really store (credit.RepairBountyBase / credit.RepairBountyTruncation),
// never a duplicated threshold.
//
// THE RULE, with its complement: warn iff this publish's real repair-bounty base is below
// the base the shipped default pays on a full frame. It is silent in exactly one case —
// a base at or above that.
//
// WHAT AN UNSET -chunk-size DOES, precisely, because the first version of this comment
// overstated it (blind PE R-1, 2026-09-09). An unset flag IS DefaultChunkSize, so the
// GEOMETRY cause can never fire without one; the OBJECT cause can, and is meant to.
// Measured at the shipped default with no flag set: 1,024 B fires (ZERO), 100,000 B fires
// (TRUNCATES 21.4 %), 262,119 B fires (TRUNCATES 10.0 %), and 262,120 B is the first
// silent size. So a default publish is silent for a FULL-FRAME object and speaks for a
// short-framed one — every object of 262,119 B or less warns.
//
// That is a deliberate TRADE against the earlier "don't warn on every default publish"
// finding (blind PE M6), not a way of satisfying it: after R-SHORT-FINAL-STRIPE the object
// is what pays, and a publisher who is told nothing has no other way to learn that this
// object's repairs pay nothing. TestGLambda8PublishWarningFiresOnlyWhenThePublishShort
// PaysTheRepairer drives both sides and both causes, including the first silent size.
func bountyPriceWarning(chunkBytes, objectBytes int64) string {
	k := erasure.DefaultParams.K
	shardBytes := publishShardBytes(chunkBytes, objectBytes)
	base := credit.RepairBountyBase(k, shardBytes)
	if base >= shippedBountyBase() {
		return ""
	}
	// WHICH cause, and therefore which fix. A short frame means the shard IS the object.
	shortFrame := objectBytes >= 0 && shardBytes < chunkBytes+crypto.Overhead
	cause := fmt.Sprintf("-chunk-size %d gives a %d-byte shard", chunkBytes, shardBytes)
	fix := fmt.Sprintf("use -chunk-size >= %d for a non-zero bounty; the shipped default %d pays %d", minBountyChunkBytes(), pipeline.DefaultChunkSize, shippedBountyBase())
	if shortFrame {
		cause = fmt.Sprintf("this object is %d B, which fits in ONE frame, so it is stored at its true length and its shard is %d B (R-SHORT-FINAL-STRIPE)", objectBytes, shardBytes)
		fix = "NO chunk size changes this — the shard IS the object; under a repair economy this object's durability is prepay-only (fund its escrow), because its serves are also too small to skim a credit"
	}
	if base == 0 {
		return fmt.Sprintf("warning: this publish pays a ZERO repair bounty: %s, and a k=%d stripe of those is %d B — below one credit of fetch (%d B), so under a repair economy (-economy) a repair of it pays NOTHING (G-λ-8); %s",
			cause, k, int64(k)*shardBytes, int64(credit.DeliveryBytesPerCredit), fix)
	}
	exactE5, lossTenths := credit.RepairBountyTruncation(k, shardBytes)
	return fmt.Sprintf("warning: this publish TRUNCATES the repair bounty: %s, and a k=%d stripe of those is worth %s credits but pays %d — a repair of this object short-pays the repairer by %s%% of the price (R-BOUNTY-TRUNCATION, G-BT-1); %s",
		cause, k, creditsE5(exactE5), base, tenthsPct(lossTenths), fix)
}

// creditsE5 renders a price that credit.RepairBountyTruncation floored at 1e-5 credits:
// 199_996 reads "1.99996". Integer formatting, so the printed figure is the computed one.
func creditsE5(e5 int64) string {
	return fmt.Sprintf("%d.%05d", e5/100_000, e5%100_000)
}

// tenthsPct renders tenths of one percent: 500 reads "50.0".
func tenthsPct(t int64) string {
	return fmt.Sprintf("%d.%d", t/10, t%10)
}

// ---- R2.9 delivery session: the derived ceiling, the quantized pin, the S5 line.

// ---- the delivery idle window: the bound, the floor, the shipped default.
//
// The window is a LIVENESS choice: how long a paid delivery session survives a gap in its
// fetcher's settlements. An honest fetcher goes quiet while the chain it needs is stalled,
// so the window must dominate the worst stall the published liveness model admits. All
// three constants below are DERIVED from that one sentence; none is chosen.

// deliveryIdleBound is the worst stall the model admits: a LOST entry forward is bounded
// by the re-keyed takeover at ≤ (N+2)·ChainSyncInterval + G = 14·30 + 10 s at N = 12
// (docs/decisions.md D-H43-WORKLESS-DESIGNEE (21), ratified 2026-09-07). It DOMINATES the
// 190 s modal tier of D-CONSENSUS-ARMING (19), which is why a defensive window is sized
// against it and not against the tier: a window that only clears the modal case is broken
// by the worst case the same document publishes. Both were field-confirmed on the graded
// run integration/cloudtest/report-97e3101-deep.md (rows 6-fault-tolerance, 10a-stall-drill).
const deliveryIdleBound = 430 * time.Second

// deliveryIdleStampDivisor mirrors core/node's unexported deliveryStampDivisor, which
// coarsens the last-settle stamp into buckets of window/divisor (a fine per-identity
// activity timestamp would be a finer access record than the reaper needs). cmd/silt
// cannot import it; core/node TestC2StampDivisorIsFour pins the original and cmd/silt
// TestC2ShippedFloorIsDerivedFromTheBound pins this mirror against it.
const deliveryIdleStampDivisor = 4

// deliveryIdleFloor is the smallest -delivery-idle-window the daemon accepts. The stamp
// coarsening spends up to a whole bucket of the window before the reaper ever looks, so a
// window of D only GUARANTEES D − D/divisor of survival since a real settlement (MEASURED
// at 0.751× on a 1000 s window: core/node TestC2GuaranteedSurvivalIsThreeQuartersOfTheWindow).
// The floor is therefore the bound scaled by divisor/(divisor−1), not the bound itself:
// a daemon started at exactly deliveryIdleBound reaps a session gapped only 322.5 s.
// This REPLACES the old 1 s floor, which existed only to keep the idle/2 ticker interval
// positive (blind PE item 3) and enforced nothing about the bound.
const deliveryIdleFloor = deliveryIdleBound * deliveryIdleStampDivisor / (deliveryIdleStampDivisor - 1)

// deliveryIdleFieldStall is the one stall the field has actually produced: run
// c450985-deep, block 43 committed 17 min 20 s after block 42. That is the DEFECT the A1
// fix closed, so it is not a bound — it is carried here as the margin datum the shipped
// default is required to clear.
const deliveryIdleFieldStall = 1040 * time.Second

// deliveryIdleDefault is the SHIPPED -delivery-idle-window (owner call 4 of
// D-TRUE-UP-CALLS-2026-09-07: refuse-until-set is released once the bound is
// field-confirmed, and the default is then set ABOVE the bound).
//
// 24m, not the tighter 10m that also clears the floor, for two measured reasons:
//   - 10m guarantees 450 s against a 430 s bound: 4.7 % of margin, inside the measurement
//     error of the block interval the bound's inputs are quoted at (45.865 s/height
//     measured vs the 30 s ChainSyncInterval the bound is computed at).
//   - 10m does NOT survive a repeat of the stall the field produced. Driven, both
//     candidates, both directions: core/node TestC2SessionSurvivesTheWorstAdmittedStall
//     rows candidate-tight-10m/1040s (reaped) and candidate-margin-24m/1040s (alive).
//
// It is a DURATION, not an epoch count: the bound is denominated in ChainSyncInterval and
// does not move with the block time, so an epoch-denominated default would drift away from
// the number it has to dominate. The cost of the longer window is a session slot held
// (deliveryMaxLiveSessions = 4096, and filling it is faucet-limited — one demand-domain
// anchor per session), and a deposit returned at the LATER of the anchor's release epoch
// and the close (core/credit/deliveryanchor.go CloseDeliverySession), so the release epoch
// is the binding term at this window on the measured cohort.
const deliveryIdleDefault = 24 * time.Minute

// Compile-time proof of the three sentences above, in the same arithmetic the reaper runs.
// A future edit to the bound, the divisor or the default that breaks one of them fails to
// BUILD: a negative constant does not convert to uint.
const (
	_ = uint(deliveryIdleFloor - deliveryIdleFloor/deliveryIdleStampDivisor - deliveryIdleBound)          // the floor's guaranteed survival clears the bound
	_ = uint(deliveryIdleDefault - deliveryIdleFloor)                                                     // the default is at or above the floor
	_ = uint(deliveryIdleDefault - deliveryIdleDefault/deliveryIdleStampDivisor - deliveryIdleFieldStall) // and clears the observed field stall
	_ = uint(deliveryIdleFloor/2 - time.Second)                                                           // the idle/2 ticker interval stays positive (PE item 3)
)

// deliverySessionCeiling is C3 (G-R212-8 cert §3.1): D_max = ⌊f/p⌋·U bytes per anchor
// and k_max_delivery = ⌈D_max·p/(U·f)⌉ = 1 anchor per open, DERIVED from the face this
// ledger charges (the ONE fee constant), never pinned. A ceiling pinned independently of
// f re-creates the burn the relay re-price removed (T-RELAY-GRAN; gate G-λ-8-1).
func deliverySessionCeiling(fee int64) (dMax int64, kMax int) {
	if fee <= 0 {
		return 0, 0
	}
	dMax = (fee / credit.DeliveryIncrementCredit) * credit.DeliveryIncrementBytes
	kMax = int((dMax*credit.DeliveryIncrementCredit + credit.DeliveryIncrementBytes*fee - 1) / (credit.DeliveryIncrementBytes * fee))
	return dMax, kMax
}

// grantFundsThePinInWholeFaces is G-λ-8-2, the QUANTIZED form of the grant/r pin: a
// face is indivisible and spent at one server, so the pin is funded iff the faces the
// pin needs on BOTH lanes fit in one grant: ⌈B_pin/D_max⌉ + ⌈B_pin/relayBytesPerAnchor⌉
// ≤ ⌊g/f⌋. Today 6 + 3 = 9 ≤ 10 — one face of margin (cert §8). This is the φ = 1
// reading (every face fully consumed); the certified refutation of the COMPOSED claim
// (φ ≪ 1 on the real path while G-6 burns the remainder) is the owner's call, not this
// gate's — the gate holds the arithmetic that must survive either way.
func grantFundsThePinInWholeFaces(grant, fee, relayBytesPerCredit int64) (need, have int64, ok bool) {
	if fee <= 0 || relayBytesPerCredit <= 0 {
		return 0, 0, false
	}
	dMax, _ := deliverySessionCeiling(fee)
	relayPerAnchor := fee * relayBytesPerCredit
	pin := credit.GrantOverRPinBytes
	need = (pin+dMax-1)/dMax + (pin+relayPerAnchor-1)/relayPerAnchor
	have = grant / fee
	return need, have, need <= have
}

// deliveryAffordabilityLine is the S5 disclosure (R2.9 gate B-11): one announced line,
// every number COMPUTED from the constants and the ledger, never typed. Registered in
// observable_contract.go with TestAffordabilityLineIsAnnounced as its asserter.
func deliveryAffordabilityLine(grant, fee, relayBytesPerCredit int64, idle string) string {
	dMax, kMax := deliverySessionCeiling(fee)
	need, have, _ := grantFundsThePinInWholeFaces(grant, fee, relayBytesPerCredit)
	return fmt.Sprintf("delivery settlement: p=%d credit per %d B (U/p=%d B/credit; self-mint Dλ=%d B/credit, PF %.2f); anchor face %d funds %d increments = %d B (%.2f GiB) per session, k_max=%d; one grant = %d faces, the 64 GiB pin needs %d faces across delivery+relay; idle window %s; unsettled remainder: a DEPOSIT returned to the fetcher's account when its anchor leaves the %d-epoch guard window (D-R2.9-NODE-HALF-CALLS 1′; the relay lane keeps the burn)",
		int64(credit.DeliveryIncrementCredit), int64(credit.DeliveryIncrementBytes), int64(credit.DeliveryBytesPerCredit), int64(credit.ServeMintBytesPerCredit),
		float64(credit.ServeMintBytesPerCredit)/float64(credit.DeliveryBytesPerCredit),
		fee, dMax/credit.DeliveryIncrementBytes, dMax, float64(dMax)/float64(1<<30), kMax, have, need, idle, credit.PaidSerialWindow+1)
}
