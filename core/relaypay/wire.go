package relaypay

// PoD §7.3 transport Batch 2 — the on-wire session vocabulary (step 3, design §1).
//
// Three request/reply kinds mirror the delivery-receipt lane exactly (design §1):
// RelayOpen (fetcher commits the chain root + funding + S), RelayPay (a preimage
// reveal that authorizes the next increment(s)), and their acks. Settlement needs
// NO wire message — the relay settles LOCALLY at close by redeeming its highest
// held preimage (design §1, §5). These are the CBOR payloads that ride ports.Message
// Data, exactly as demand.SubmittedReceipt rides MsgDeliveryReceipt.

import "github.com/fxamacker/cbor/v2"

// RelayOpen is the fetcher's session-open commitment (MsgRelayOpen). It carries the
// committed chain root, the chain length S, and the funding-source discriminator.
// The relay verifies all of it at OpenRelaySession — the M0 guards ((i) funding is
// an ephemeral blind credit, (ii) fresh eph + fresh root) and the #644 S-clamp all
// fire there. Funding is an int mirroring node.FundingSource; the wire layer stays
// free of a node import, and the node maps it back on receipt.
type RelayOpen struct {
	Root    []byte `cbor:"1,keyasint"`
	S       int    `cbor:"2,keyasint"`
	Funding int    `cbor:"3,keyasint"`
}

// Marshal encodes a RelayOpen for MsgRelayOpen.Data.
func (o RelayOpen) Marshal() ([]byte, error) { return cbor.Marshal(o) }

// UnmarshalRelayOpen decodes a RelayOpen from MsgRelayOpen.Data.
func UnmarshalRelayOpen(b []byte) (RelayOpen, error) {
	var o RelayOpen
	if err := cbor.Unmarshal(b, &o); err != nil {
		return RelayOpen{}, err
	}
	return o, nil
}

// RelayPay is a preimage reveal (MsgRelayPay): the fetcher reveals x_Count to
// authorize the relay to advance to increment Count. Handle names the live session
// the relay returned in MsgRelayOpenAck. The relay drives the carried-S Verifier
// with this (AdvanceTo), which is bounded to at most S hashes (the #644 clamp).
type RelayPay struct {
	Handle   uint64 `cbor:"1,keyasint"`
	Preimage []byte `cbor:"2,keyasint"`
	Count    int    `cbor:"3,keyasint"`
}

// Marshal encodes a RelayPay for MsgRelayPay.Data.
func (p RelayPay) Marshal() ([]byte, error) { return cbor.Marshal(p) }

// UnmarshalRelayPay decodes a RelayPay from MsgRelayPay.Data.
func UnmarshalRelayPay(b []byte) (RelayPay, error) {
	var p RelayPay
	if err := cbor.Unmarshal(b, &p); err != nil {
		return RelayPay{}, err
	}
	return p, nil
}
