package main

import (
	"fmt"
	"net"
	"strings"

	"github.com/nerolabs/silt/ports"
)

// openKinds are what a peer that is NOT a listed writer may send under -writers; everything
// else from it is dropped. They are the reads a joiner needs — find peers and providers,
// fetch and check pieces, sync the chain, pull witnesses, learn issuer keys, test its own
// reachability — the replies to this node's own requests, a provider record (the node keeps
// a non-writer's only for content it already knows: knownKeyFor), and the two submissions
// that carry a proof anyone can check: equivocation evidence and a signed advisory. It is an
// allow-list: storing a piece, publish tokens, bond and issuer-key registration, entries, the
// credit lanes, repair claims and consensus messages are closed, and so is any kind added
// later until it is listed here.
var openKinds = func() []ports.MsgKind {
	open := []ports.MsgKind{
		ports.MsgFindNode, ports.MsgGetProviders, ports.MsgAddProvider,
		ports.MsgFetchChunk, ports.MsgHasChunk, ports.MsgChallenge,
		ports.MsgGetChain, ports.MsgGetChainHead, ports.MsgGetWitness,
		ports.MsgGetIssuerKey, ports.MsgGetCanonicalIssuers, ports.MsgGetDemandIssuerKeys,
		ports.MsgCheckReachability,
		ports.MsgSubmitSlash, ports.MsgSubmitAdvisory,
	}
	// Every reply: they answer this node's own requests. The set is ports.Message.IsReply —
	// exactly the kinds the node matches against its pending requests (by RID and sender), so
	// one that answers nothing it asked is dropped there. TestOpenKindsAreExactlyTheGoldenList
	// pins the whole set, so a new kind cannot open by being classed a reply unnoticed.
	for k := ports.MsgKind(1); k != 0; k++ {
		// RepairVote counts as a reply (it answers a repair claim) but it decides a bounty
		// payout, so it stays closed to non-writers like every other credit movement.
		if (ports.Message{Kind: k}).IsReply() && k != ports.MsgRepairVote {
			open = append(open, k)
		}
	}
	// The answer to this node's own reachability check: matched by nonce, not request id, so
	// IsReply does not list it — and without it a public node would believe it is unreachable.
	return append(open, ports.MsgReachabilityReply)
}()

// pinnedKinds come only from the operator's own fixed node identities — never from a publish
// client's throwaway identity — so under -writer-ids they need a listed ID as well as a writer
// address: a bond or issuer-key registration, consensus, bond audits, repair claims and votes,
// and the credit lanes. A compromised writer host can then still publish (its throwaway
// identities are indistinguishable), but cannot act as a validator or move credit.
var pinnedKinds = []ports.MsgKind{
	ports.MsgSubmitBondReg, ports.MsgSubmitIssuerKeyReg, ports.MsgHaveBondReg,
	ports.MsgProposeBlock, ports.MsgPrepareQC, ports.MsgCommitBlock, ports.MsgRoundChange, ports.MsgRoundCert,
	ports.MsgBondChallenge, ports.MsgRepairClaim, ports.MsgRepairVote,
	ports.MsgDeliveryReceipt, ports.MsgDeliveryOpen, ports.MsgDeliveryFund, ports.MsgDeliverySettle,
	ports.MsgRelayOpen, ports.MsgRelayPay, ports.MsgDemandTokenRequest,
}

// parseWriterIDs reads -writer-ids: comma-separated node IDs.
func parseWriterIDs(s string) ([]ports.NodeID, error) {
	var out []ports.NodeID
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		id, err := ports.ParseHash(f)
		if err != nil {
			return nil, fmt.Errorf("-writer-ids: %q: %w", f, err)
		}
		out = append(out, id)
	}
	return out, nil
}

// parseWriters reads -writers: comma-separated IPs (one address) or CIDRs. Empty is nil:
// everyone may write.
func parseWriters(s string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !strings.Contains(f, "/") {
			ip := net.ParseIP(f)
			if ip == nil {
				return nil, fmt.Errorf("-writers: %q is not an IP address or CIDR", f)
			}
			if ip.To4() != nil {
				f += "/32"
			} else {
				f += "/128"
			}
		}
		_, n, err := net.ParseCIDR(f)
		if err != nil {
			return nil, fmt.Errorf("-writers: %q: %w", f, err)
		}
		out = append(out, n)
	}
	return out, nil
}
