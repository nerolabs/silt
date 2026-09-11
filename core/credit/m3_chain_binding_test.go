package credit

// M3 — the network the credit lane's fixtures mint on (research certification 2026-09-11
// §3). core/credit does not verify token signatures itself: it spends serials into guards.
// What it needs from M3 is a chain id to hand the demand primitives its fixtures call, and
// the guarantee that the ledger's arithmetic is indifferent to which network minted the
// token — the refusal happens upstream, at demand.Keyset (see core/demand's M3 gates) and
// at the node's verifyDeliveryAnchors / OpenRelaySession.

import "github.com/nerolabs/silt/ports"

// creditTestChainID is one network, used by every fixture here. Non-uniform bytes: a
// repeated-byte fixture hides a binding that copies only the first byte or only the last.
var creditTestChainID = ports.Hash{
	0x76, 0x00, 0x1d, 0xc4, 0x39, 0x00, 0xab, 0x52,
	0xe8, 0x11, 0x00, 0x6f, 0x94, 0x2b, 0x00, 0xd7,
	0x03, 0x88, 0xfc, 0x00, 0x45, 0x9e, 0x21, 0x00,
	0xb0, 0x5d, 0x00, 0xe3, 0x17, 0xca, 0x60, 0x8a,
}
