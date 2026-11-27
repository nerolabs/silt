package main

import (
	"sort"
	"strings"
	"testing"
)

// TestOpenKindsAreExactlyTheGoldenList pins what a non-writer may send under -writers. The
// set is partly derived (every reply), so a message kind added later could otherwise open
// silently by being classed a reply; this list must be edited, on purpose, for it to open.
func TestOpenKindsAreExactlyTheGoldenList(t *testing.T) {
	golden := []string{
		// reads a joiner needs
		"FindNode", "GetProviders", "AddProvider", "FetchChunk", "HasChunk", "Challenge",
		"GetChain", "GetChainHead", "GetWitness", "GetIssuerKey", "GetCanonicalIssuers",
		"GetDemandIssuerKeys", "CheckReachability",
		// self-verifying submissions
		"SubmitSlash", "SubmitAdvisory",
		// replies to this node's own requests
		"AddProviderAck", "AttestReply", "BondReply", "CanonicalIssuersReply", "ChainHeadReply",
		"ChainReply", "ChallengeReply", "CommitAck", "DeliveryFundAck", "DeliveryOpenAck",
		"DeliveryReceiptAck", "DeliverySettleAck", "DemandIssuerKeysReply", "DemandTokenReply",
		"FetchChunkReply", "FindNodeReply", "GetProvidersReply", "HasChunkReply", "IssuerKeyReply",
		"PrecommitReply", "ReachabilityReply", "RelayOpenAck", "RelayPayAck", "RoundCertAck",
		"RoundChangeAck", "StoreChunkAck", "SubmitBondRegAck", "SubmitEntryAck",
		"SubmitIssuerKeyRegAck", "SubmitSlashAck", "TokenReply", "WitnessReply",
	}
	var got []string
	for _, k := range openKinds {
		got = append(got, k.String())
	}
	sort.Strings(golden)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(golden, ",") {
		t.Fatalf("openKinds changed.\n got: %v\nwant: %v\nA kind opened to non-writers must be added to this list on purpose.", got, golden)
	}
	for _, closed := range []string{"StoreChunk", "TokenRequest", "SubmitBondReg", "SubmitIssuerKeyReg", "SubmitEntry", "RepairClaim", "RepairVote", "DeliveryFund", "ProposeBlock"} {
		for _, g := range got {
			if g == closed {
				t.Fatalf("%s is open to non-writers", closed)
			}
		}
	}
}
