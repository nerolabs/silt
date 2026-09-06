package main

import (
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
func warnBountyChunk(chunkBytes int) {
	if int64(chunkBytes) < credit.MinBountyChunkBytes {
		fmt.Fprintf(os.Stderr, "warning: -chunk-size %d is below %d bytes: under a repair economy (-economy) this object's repair bounty base is ZERO (k·shardBytes < one credit of fetch, G-λ-8) and a repair of it pays nothing; use -chunk-size >= %d\n",
			chunkBytes, int64(credit.MinBountyChunkBytes), int64(credit.MinBountyChunkBytes))
	}
}
