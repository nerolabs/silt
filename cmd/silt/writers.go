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
	// Every reply and acknowledgement: they answer this node's own requests, and one that
	// matches none is ignored by the node.
	for k := ports.MsgKind(1); k != 0; k++ {
		if name := k.String(); strings.HasSuffix(name, "Reply") || strings.HasSuffix(name, "Ack") {
			open = append(open, k)
		}
	}
	return open
}()

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
