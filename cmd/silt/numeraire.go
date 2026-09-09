package main

import (
	"flag"
	"fmt"
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

// warnBountyChunk names, at publish time, a chunk size whose stripe SHORT-PAYS the
// repair bounty: the base is an integer floor of the witnessed fetch price, so
// k × shardBytes below one credit of fetch rounds to nothing (G-λ-8, G-R212-7) and
// k × shardBytes just above it rounds away up to half the repairer's wage
// (R-BOUNTY-TRUNCATION, G-BT-1). A shard is a whole ciphertext chunk (chunk +
// crypto.Overhead). The daemon has no chunk geometry at start-up to refuse on, so the
// publisher is told here and the judge names a zero again at settlement.
//
// It fires only when the operator SET -chunk-size; a warning on every default publish
// is noise nobody reads (blind PE M6), and the shipped default is the silent case by
// construction — see bountyChunkWarning.
func warnBountyChunk(chunkBytes int, explicit bool) {
	if msg := bountyChunkWarning(int64(chunkBytes), explicit); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
}

// minBountyChunkBytes is the publish-time ZERO threshold at the shipped erasure geometry.
func minBountyChunkBytes() int64 {
	return credit.MinBountyChunkBytesFor(erasure.DefaultParams.K, crypto.Overhead)
}

// shippedBountyBase is the repair-bounty base the SHIPPED publish default pays: the
// warning's threshold, DERIVED from the two shipped constants and never typed. It is 10
// today (a 262,160-byte shard, exact 10.00061), which is what makes the rule below have a
// closed complement: warn iff the operator's geometry pays a smaller base than the
// default's, so the default is silent by construction rather than by a hand-kept number.
func shippedBountyBase() int64 {
	return credit.RepairBountyBase(erasure.DefaultParams.K, int64(pipeline.DefaultChunkSize)+crypto.Overhead)
}

// bountyChunkWarning is the pure form of warnBountyChunk: the warning text, or "" when
// nothing should be said. Every number is the judge's own arithmetic on the runtime
// geometry (credit.RepairBountyBase / credit.RepairBountyTruncation), never a duplicated
// threshold.
//
// THE RULE, with its complement: the warning fires iff the operator SET -chunk-size AND
// that geometry's base is below the base the shipped default pays. It is silent in
// exactly two cases — an unset -chunk-size, or a base at or above the default's — and
// TestGLambda8PublishWarningFiresOnlyForAnExplicitSmallChunk drives both sides.
func bountyChunkWarning(chunkBytes int64, explicit bool) string {
	if !explicit {
		return ""
	}
	k := erasure.DefaultParams.K
	shardBytes := chunkBytes + crypto.Overhead
	base := credit.RepairBountyBase(k, shardBytes)
	if base >= shippedBountyBase() {
		return ""
	}
	if base == 0 {
		return fmt.Sprintf("warning: -chunk-size %d is below %d bytes: under a repair economy (-economy) this object's repair bounty base is ZERO (k·shardBytes = %d < one credit of fetch = %d, G-λ-8; a shard is a whole ciphertext chunk) and a repair of it pays nothing; use -chunk-size >= %d",
			chunkBytes, minBountyChunkBytes(), int64(k)*shardBytes, int64(credit.DeliveryBytesPerCredit), minBountyChunkBytes())
	}
	exactE5, lossTenths := credit.RepairBountyTruncation(k, shardBytes)
	_, dLossTenths := credit.RepairBountyTruncation(k, int64(pipeline.DefaultChunkSize)+crypto.Overhead)
	return fmt.Sprintf("warning: -chunk-size %d TRUNCATES the repair bounty: a k=%d stripe of %d-byte shards is worth %s credits and pays %d, so a repair of this object short-pays the repairer by %s%% of the price (R-BOUNTY-TRUNCATION, G-BT-1); the shipped default -chunk-size %d pays %d and short-pays %s%%",
		chunkBytes, k, shardBytes, creditsE5(exactE5), base, tenthsPct(lossTenths),
		pipeline.DefaultChunkSize, shippedBountyBase(), tenthsPct(dLossTenths))
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

// flagWasSet reports whether the operator passed name on the command line.
func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// ---- R2.9 delivery session: the derived ceiling, the quantized pin, the S5 line.

// deliveryIdleFloor is the smallest -delivery-idle-window the daemon accepts: the reaper
// ticks at idle/2 on a wall-clock ticker, and a sub-second window would make that ticker
// interval round to zero (time.NewTicker panics on a non-positive interval — blind PE
// item 3). The floor is an engineering bound, not the certified VALUE, which is still
// refuse-until-set above it.
const deliveryIdleFloor = time.Second

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
