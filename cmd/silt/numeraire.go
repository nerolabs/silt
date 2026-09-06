package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nerolabs/silt/core/credit"
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

// warnBountyChunk names, at publish time, a chunk size whose stripe pays a ZERO repair
// bounty under a repair economy (G-λ-8, G-R212-7): the base is priced in the witnessed
// fetch price, so k·shardBytes below one credit of fetch rounds to nothing. The daemon
// has no chunk geometry at start-up to refuse on, so the publisher is told here and the
// judge names it again at settlement (core/node/repairclaim.go).
//
// It fires only when the operator SET -chunk-size (below the minimum); the shipped default
// (pipeline.DefaultChunkSize, 64 KiB) is itself below the minimum, and a warning on every
// default publish is noise nobody reads (blind PE M6). Moving the default is a product
// call the owner holds (R-DEFAULT-CHUNK-BOUNTY-ZERO, ROADMAP).
func warnBountyChunk(chunkBytes int, explicit bool) {
	if msg := bountyChunkWarning(int64(chunkBytes), explicit); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
}

// bountyChunkWarning is the pure form of warnBountyChunk: the warning text, or "" when
// nothing should be said.
func bountyChunkWarning(chunkBytes int64, explicit bool) string {
	if !explicit || chunkBytes >= credit.MinBountyChunkBytes {
		return ""
	}
	return fmt.Sprintf("warning: -chunk-size %d is below %d bytes: under a repair economy (-economy) this object's repair bounty base is ZERO (k·shardBytes < one credit of fetch, G-λ-8) and a repair of it pays nothing; use -chunk-size >= %d",
		chunkBytes, int64(credit.MinBountyChunkBytes), int64(credit.MinBountyChunkBytes))
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
	return fmt.Sprintf("delivery settlement: p=%d credit per %d B (U/p=%d B/credit; self-mint Dλ=%d B/credit, PF %.2f); anchor face %d funds %d increments = %d B (%.2f GiB) per session, k_max=%d; one grant = %d faces, the 64 GiB pin needs %d faces across delivery+relay; idle window %s; remainder at close: BURNED (G-6 as ratified; refund is an open owner call, G-R212-8)",
		int64(credit.DeliveryIncrementCredit), int64(credit.DeliveryIncrementBytes), int64(credit.DeliveryBytesPerCredit), int64(credit.ServeMintBytesPerCredit),
		float64(credit.ServeMintBytesPerCredit)/float64(credit.DeliveryBytesPerCredit),
		fee, dMax/credit.DeliveryIncrementBytes, dMax, float64(dMax)/float64(1<<30), kMax, have, need, idle)
}
