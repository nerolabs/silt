package relaypay

// PoD §7.3 transport Batch 2 — the on-wire session vocabulary (step 3, design §1).
//
// Three request/reply kinds mirror the delivery-receipt lane exactly (design §1):
// RelayOpen (fetcher commits the chain root + S + the prepayment anchors), RelayPay
// (a preimage reveal that authorizes the next increment(s)), and their acks.
// Settlement needs NO wire message — the relay settles LOCALLY at close by
// redeeming its highest held preimage (design §1, §5). These are the CBOR payloads
// that ride ports.Message Data, exactly as demand.SubmittedReceipt rides
// MsgDeliveryReceipt.
//
// R2.14 (2026-09-04) makes RelayOpen v2: the chain root is ANCHORED to k blind-signed
// prepayment credentials the fetcher's durable identity bought from this relay, and
// the whole commitment is signed by the session's ephemeral key — Rivest–Shamir's
// M = {vendor, C_U, w_0, …}_SK_U with C_U the blind credentials. No consensus
// coupling: nothing in core/chain reads this payload (cert §8).

import (
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/core/blindtoken"
)

// ShippedAnchorFace is the face of one anchor at the shipped fee: cmd/silt's default
// --fee (50,000 credits), the value the relay's ledger charged for the blind
// withdrawal (core/credit: face = Fee(), an identity with the burn). Pinned as a
// literal here because core/relaypay carries no dependency on core/credit; the
// derivation below is checked against a second, independent literal in
// TestRelayMaxAnchorsPerSessionCoversTheSessionCeiling.
const ShippedAnchorFace = 50_000

// MaxAnchorsPerSession is the decode/DoS bound on anchors per RelayOpen — DERIVED,
// never a bare number (cert §5): the fewest anchors whose summed face covers the
// longest session a relay accepts, ⌈S_max × RelayIncrementCredit / face⌉ = 6 at the
// shipped fee. One anchor funds 195.3 MiB of forwarding; six cover the 1 GiB
// MaxSessionBytes. A lower fee raises it (granularity / liveness, never soundness);
// slack above the ceiling would only let an attacker pad an open with garbage
// anchors that cost the relay a modexp each.
const MaxAnchorsPerSession = (MaxChainLength*RelayIncrementCredit + ShippedAnchorFace - 1) / ShippedAnchorFace

// Anchor is one relay prepayment credential as presented at session open: the
// serial and the relay's blind signature over (issue epoch, serial) in the
// relay-anchor domain (blindtoken.BlindRelayAnchor). The issue epoch rides no field —
// the relay discovers it by verifying under its held per-epoch keys, newest first
// (demand.Keyset.VerifyAnchorInWindow), exactly as a demand token does.
type Anchor struct {
	Serial []byte `cbor:"1,keyasint"`
	Sig    []byte `cbor:"2,keyasint"`
}

// RelayOpen is the fetcher's session-open commitment (MsgRelayOpen).
//
// v1 fields: the committed chain root x_0, the chain length S, and the
// funding-source discriminator (an int mirroring node.FundingSource so the wire
// layer stays free of a node import). Funding is the M0 guard (i) TEST OBJECT — the
// value a caller presents to be refused; it enforces nothing by itself. What
// enforces guard (i) is the anchor: a blind bearer credential verified under the
// relay's chain-committed key, bought by a durable identity the relay cannot link to
// this session (docs/design/pod.md §7.3.4).
//
// v2 fields (R2.14): Anchors, the k ≤ MaxAnchorsPerSession prepayment credentials
// the root is committed under; Fetcher, the session ephemeral's ed25519 public key
// (sha256(Fetcher) MUST equal the authenticated sender); and Sig, its signature over
// the commitment M = sha256("silt/relay/open/v1" ‖ relayID ‖ Root ‖ uint32BE(S) ‖
// uint32BE(k) ‖ serial_1 ‖ … ‖ serial_k) (core/node relayOpenCommitment). M binds the
// root, the length, the anchor set and the relay the commitment was made to, so a
// tampered field or a replay at another relay fails one ed25519 verify before any
// RSA work.
//
// Version skew fails safe both ways (cert §8): a v1 open at a v2 relay decodes with
// no anchors and is refused with a named reason; a v2 open at a v1 relay is admitted
// unanchored and pays 0.
type RelayOpen struct {
	Root    []byte   `cbor:"1,keyasint"`
	S       int      `cbor:"2,keyasint"`
	Funding int      `cbor:"3,keyasint"`
	Anchors []Anchor `cbor:"4,keyasint,omitempty"`
	Fetcher []byte   `cbor:"5,keyasint,omitempty"`
	Sig     []byte   `cbor:"6,keyasint,omitempty"`
}

// Marshal encodes a RelayOpen for MsgRelayOpen.Data.
func (o RelayOpen) Marshal() ([]byte, error) { return cbor.Marshal(o) }

// ErrRelayOpenBounds is the decode refusal for an over-sized v2 field.
var ErrRelayOpenBounds = errors.New("relaypay: RelayOpen field exceeds its bound")

// The v2 field bounds, checked at decode BEFORE any map write or modexp (the F5
// amplifier shape, demand.go's decode bounds): an attacker-sized field must die in
// the codec, the cheapest refusal there is.
const (
	// maxAnchorSigBytes bounds an anchor signature to the largest modulus the lane
	// will ever verify under (blindtoken.MaxModulusBits / 8 = 1,024 B).
	maxAnchorSigBytes = blindtoken.MaxModulusBits / 8
	// fetcherPubBytes / openSigBytes are the fixed ed25519 widths.
	fetcherPubBytes = 32
	openSigBytes    = 64
)

// UnmarshalRelayOpen decodes a RelayOpen from MsgRelayOpen.Data and enforces the
// v2 bounds: len(Anchors) ≤ MaxAnchorsPerSession, each Serial ≤
// blindtoken.SerialSize, each Sig ≤ MaxModulusBits/8, Fetcher == 32 and Sig == 64
// when present. A v1 payload (no v2 fields) decodes and is refused at
// OpenRelaySession with a named reason, not here.
func UnmarshalRelayOpen(b []byte) (RelayOpen, error) {
	var o RelayOpen
	if err := cbor.Unmarshal(b, &o); err != nil {
		return RelayOpen{}, err
	}
	if len(o.Anchors) > MaxAnchorsPerSession {
		return RelayOpen{}, fmt.Errorf("%w: %d anchors > %d", ErrRelayOpenBounds, len(o.Anchors), MaxAnchorsPerSession)
	}
	for i, a := range o.Anchors {
		if len(a.Serial) > blindtoken.SerialSize {
			return RelayOpen{}, fmt.Errorf("%w: anchor %d serial %d B > %d", ErrRelayOpenBounds, i, len(a.Serial), blindtoken.SerialSize)
		}
		if len(a.Sig) > maxAnchorSigBytes {
			return RelayOpen{}, fmt.Errorf("%w: anchor %d sig %d B > %d", ErrRelayOpenBounds, i, len(a.Sig), maxAnchorSigBytes)
		}
	}
	if len(o.Fetcher) != 0 && len(o.Fetcher) != fetcherPubBytes {
		return RelayOpen{}, fmt.Errorf("%w: fetcher key %d B != %d", ErrRelayOpenBounds, len(o.Fetcher), fetcherPubBytes)
	}
	if len(o.Sig) != 0 && len(o.Sig) != openSigBytes {
		return RelayOpen{}, fmt.Errorf("%w: open signature %d B != %d", ErrRelayOpenBounds, len(o.Sig), openSigBytes)
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
