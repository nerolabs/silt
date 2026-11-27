package main

import (
	"fmt"
	"net"
	"strings"

	"github.com/nerolabs/silt/ports"
)

// writeKinds are the inbound messages that write to a node: they put bytes on its disk, mint
// a publish token, give an identity standing (a bond or issuer-key registration), or submit
// an entry. -writers limits exactly these. Reads — lookups, fetches, provider records, chain
// sync, witnesses — are never limited: a joiner must be able to read and serve.
var writeKinds = []ports.MsgKind{
	ports.MsgStoreChunk,
	ports.MsgTokenRequest,
	ports.MsgSubmitBondReg,
	ports.MsgSubmitIssuerKeyReg,
	ports.MsgSubmitEntry,
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
