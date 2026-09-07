package node

// B-9 (2026-09-07): the v2 flat receipt is retired, so every node-tier gate that drove a
// delivery through MsgDeliveryReceipt now drives it through the SESSION lane — open with
// the token as the anchor, settle one increment, close. The properties the gates pin are
// unchanged; only the driver is.

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

// sessionPresent presents tok at server as a session anchor from fetcher's DURABLE
// identity, settles ONE increment for object and closes the session (so the same fetcher
// may present again). It reports whether the delivery was banked — the open admitted and
// the settlement paid — the same yes/no the flat `present` reported.
func sessionPresent(t *testing.T, server *Node, fetcher *identity.Identity, tok demand.Token, object ports.Hash) bool {
	t.Helper()
	s, err := server.OpenDeliverySession(fetcher.NodeID(), demand.SignSessionOpen(fetcher.Signer(), server.id, []demand.Token{tok}))
	if err != nil {
		return false
	}
	settled, err := server.SettleDeliveryReceipt(fetcher.NodeID(), demand.AckSession(fetcher.Signer(), s.handle, s.commitment, object, server.id, 1))
	server.closeDeliverySession(s.handle, "test")
	return err == nil && settled > 0
}
