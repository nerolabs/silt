package demand

// R2.9 — the paid DELIVERY SESSION's wire vocabulary and its signatures (the pure
// half; core/node holds the session table and the handlers). Certified shape:
// silt-reviews/research/research-outcome/R2.9-G-R212-8-delivery-anchor-quantization-RESEARCH-CERTIFICATION-2026-09-06.md
// §3.1 (C1–C10), §6.3 (receipt v3); the domain ruling (no fifth FDH domain — the
// anchor IS the demand token, spent at OPEN instead of at redeem):
// …/R2.9-build-questions-domain-rescale-guard-RESEARCH-CERTIFICATION-2026-09-04.md §2.
//
// Three messages mirror the relay lane's (core/relaypay/wire.go):
//
//   - SessionOpen: the fetcher's DURABLE identity presents k = 1 demand token
//     (bought from this server through the ordinary blind withdrawal) as the
//     session's anchor, signed over the open commitment M. The server verifies the
//     token under its OWN committed key_E in the DEMAND domain, spends it into the
//     shared paid-serial guard, and admits a session whose budget is the face.
//   - SessionFund: a top-up of an admitted session with a fresh anchor (C8), signed
//     over a fund commitment that names the handle.
//   - SessionReceipt (receipt v3): the fetcher's CUMULATIVE acknowledged increment
//     count for one object on one session (C5). It replaces the v2 receipt's bare
//     serial with the session handle and the open commitment M, so a receipt
//     acknowledges exactly one session and a session is exactly the anchors spent at
//     its open (B-8). Its replay defence is the session's monotone counter, not the
//     paid-serial guard (cert §6.3).
//
// Receipts are not committed to the chain, so the v3 bump has no era or freeze
// coupling (the R2.14 argument). The v2 flat path (one token spent at redeem)
// stays callable until its retirement PR (gate B-9); the two never share a message
// kind.

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

// The three commitment domains. Every field under them is fixed-width (a NodeID is
// 32 B, a handle 8 B, a count 8 B, a serial blindtoken.SerialSize — all enforced
// before the hash is recomputed), so each encoding is injective without length
// prefixes (the relayOpenDomain argument).
const (
	sessionOpenDomain = "silt/delivery/open/v1"
	sessionFundDomain = "silt/delivery/fund/v1"
	receiptDomainV3   = "silt/demand/receipt/v3"
)

// MaxAnchorsPerOpen is k_max_delivery, the decode/DoS bound on anchors per open or
// top-up. It is DERIVED from the face in cmd/silt (⌈D_max·p/(U·f)⌉ = 1: one face funds
// the whole 12.21 GiB session, T-RELAY-GRAN one lane over) and pinned to this literal
// by TestDeliverySessionCeilingIsDerivedFromTheFace (G-λ-8-1); core/demand carries
// no dependency on core/credit or core/relaypay.
const MaxAnchorsPerOpen = 1

// CommitmentSize is the width of the open commitment M a receipt carries.
const CommitmentSize = sha256.Size

// SessionOpen is MsgDeliveryOpen's payload.
type SessionOpen struct {
	Anchors []Token `cbor:"1,keyasint"`
	Fetcher []byte  `cbor:"2,keyasint"` // the durable fetcher's ed25519 public key; sha256(Fetcher) MUST equal the authenticated sender
	Sig     []byte  `cbor:"3,keyasint"` // over SessionOpenCommitment(server, Anchors)
}

// SessionFund is MsgDeliveryFund's payload.
type SessionFund struct {
	Handle  uint64  `cbor:"1,keyasint"`
	Anchors []Token `cbor:"2,keyasint"`
	Fetcher []byte  `cbor:"3,keyasint"`
	Sig     []byte  `cbor:"4,keyasint"` // over SessionFundCommitment(server, Handle, Anchors)
}

// SessionReceipt is receipt v3, MsgDeliverySettle's payload: the fetcher acknowledges
// that, on session Handle (opened with commitment M), it has content-verified Count
// increments of DeliveryIncrementBytes of Object from Server IN TOTAL (cumulative).
type SessionReceipt struct {
	Handle     uint64       `cbor:"1,keyasint"`
	Commitment []byte       `cbor:"2,keyasint"` // M of the open (CommitmentSize)
	Object     ports.Hash   `cbor:"3,keyasint"`
	Server     ports.NodeID `cbor:"4,keyasint"`
	Fetcher    []byte       `cbor:"5,keyasint"`
	Count      uint64       `cbor:"6,keyasint"`
	Sig        []byte       `cbor:"7,keyasint"`
}

// SessionOpenCommitment is M = H(open-domain ‖ serverID ‖ uint32BE(k) ‖ serial_1 ‖ …):
// the vendor, the credentials and the payer's key are what the signature binds
// (Rivest–Shamir's authorization half, one lane over from relayOpenCommitment).
func SessionOpenCommitment(server ports.NodeID, anchors []Token) []byte {
	h := sha256.New()
	h.Write([]byte(sessionOpenDomain))
	h.Write(server[:])
	writeSerials(h, anchors)
	return h.Sum(nil)
}

// SessionFundCommitment is H(fund-domain ‖ serverID ‖ uint64BE(handle) ‖ uint32BE(k) ‖
// serials): a top-up is bound to the session it funds, so a captured fund message
// cannot be replayed into another session.
func SessionFundCommitment(server ports.NodeID, handle uint64, anchors []Token) []byte {
	h := sha256.New()
	h.Write([]byte(sessionFundDomain))
	h.Write(server[:])
	var b8 [8]byte
	binary.BigEndian.PutUint64(b8[:], handle)
	h.Write(b8[:])
	writeSerials(h, anchors)
	return h.Sum(nil)
}

func writeSerials(h interface{ Write([]byte) (int, error) }, anchors []Token) {
	var b4 [4]byte
	binary.BigEndian.PutUint32(b4[:], uint32(len(anchors)))
	h.Write(b4[:])
	for _, a := range anchors {
		h.Write(a.Serial)
	}
}

// receiptMsgV3 is what the fetcher signs: every field that fixes which session,
// which object, which server, whom-for, and how much — tampering any of them breaks
// the signature (B-8).
func (r SessionReceipt) receiptMsgV3() []byte {
	h := sha256.New()
	h.Write([]byte(receiptDomainV3))
	var b8 [8]byte
	binary.BigEndian.PutUint64(b8[:], r.Handle)
	h.Write(b8[:])
	h.Write(r.Commitment)
	h.Write(r.Object[:])
	h.Write(r.Server[:])
	h.Write(r.Fetcher)
	binary.BigEndian.PutUint64(b8[:], r.Count)
	h.Write(b8[:])
	return h.Sum(nil)
}

// SignSessionOpen is the fetcher side of an open: the durable identity signs M for
// server over the anchors it bought there.
func SignSessionOpen(fetcher ed25519.PrivateKey, server ports.NodeID, anchors []Token) SessionOpen {
	pub := fetcher.Public().(ed25519.PublicKey)
	return SessionOpen{Anchors: anchors, Fetcher: append([]byte(nil), pub...),
		Sig: ed25519.Sign(fetcher, SessionOpenCommitment(server, anchors))}
}

// SignSessionFund is the fetcher side of a top-up.
func SignSessionFund(fetcher ed25519.PrivateKey, server ports.NodeID, handle uint64, anchors []Token) SessionFund {
	pub := fetcher.Public().(ed25519.PublicKey)
	return SessionFund{Handle: handle, Anchors: anchors, Fetcher: append([]byte(nil), pub...),
		Sig: ed25519.Sign(fetcher, SessionFundCommitment(server, handle, anchors))}
}

// AckSession is the fetcher side of a settlement: having content-verified the bytes
// (B3), the fetcher signs its CUMULATIVE count for object on the session. Cheap by
// design — one ed25519 sign — so it runs on the floor box at delivery rate.
func AckSession(fetcher ed25519.PrivateKey, handle uint64, commitment []byte, object ports.Hash, server ports.NodeID, count uint64) SessionReceipt {
	r := SessionReceipt{Handle: handle, Commitment: append([]byte(nil), commitment...), Object: object, Server: server,
		Fetcher: append([]byte(nil), fetcher.Public().(ed25519.PublicKey)...), Count: count}
	r.Sig = ed25519.Sign(fetcher, r.receiptMsgV3())
	return r
}

// VerifySig checks the fetcher's signature over the v3 message. Everything else the
// receipt claims (the handle exists, the commitment is that session's, the count
// advances) is the server's session table to check.
func (r SessionReceipt) VerifySig() bool {
	return len(r.Fetcher) == ed25519.PublicKeySize && len(r.Commitment) == CommitmentSize &&
		ed25519.Verify(ed25519.PublicKey(r.Fetcher), r.receiptMsgV3(), r.Sig)
}

// Marshal / Unmarshal — every variable-length field is BOUNDED at decode, before any
// map write or modexp (the F5 amplifier shape, red-team re-break 2026-09-03).
func (o SessionOpen) Marshal() ([]byte, error)    { return cbor.Marshal(o) }
func (f SessionFund) Marshal() ([]byte, error)    { return cbor.Marshal(f) }
func (r SessionReceipt) Marshal() ([]byte, error) { return cbor.Marshal(r) }

// ErrSessionBounds is the decode refusal for an over-sized or mis-sized field.
var ErrSessionBounds = errors.New("demand: session message field exceeds its bound")

func boundAnchors(anchors []Token) error {
	if len(anchors) > MaxAnchorsPerOpen {
		return fmt.Errorf("%w: %d anchors > %d", ErrSessionBounds, len(anchors), MaxAnchorsPerOpen)
	}
	for i, a := range anchors {
		if len(a.Serial) > blindtoken.SerialSize {
			return fmt.Errorf("%w: anchor %d serial %d B > %d", ErrSessionBounds, i, len(a.Serial), blindtoken.SerialSize)
		}
		if len(a.Sig) > maxTokenSigBytes {
			return fmt.Errorf("%w: anchor %d sig %d B > %d", ErrSessionBounds, i, len(a.Sig), maxTokenSigBytes)
		}
	}
	return nil
}

func boundSigner(fetcher, sig []byte) error {
	if len(fetcher) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: fetcher key %d B != %d", ErrSessionBounds, len(fetcher), ed25519.PublicKeySize)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signature %d B != %d", ErrSessionBounds, len(sig), ed25519.SignatureSize)
	}
	return nil
}

// UnmarshalSessionOpen decodes MsgDeliveryOpen.Data under the bounds.
func UnmarshalSessionOpen(b []byte) (SessionOpen, error) {
	var o SessionOpen
	if err := cbor.Unmarshal(b, &o); err != nil {
		return SessionOpen{}, err
	}
	if err := boundAnchors(o.Anchors); err != nil {
		return SessionOpen{}, err
	}
	if err := boundSigner(o.Fetcher, o.Sig); err != nil {
		return SessionOpen{}, err
	}
	return o, nil
}

// UnmarshalSessionFund decodes MsgDeliveryFund.Data under the bounds.
func UnmarshalSessionFund(b []byte) (SessionFund, error) {
	var f SessionFund
	if err := cbor.Unmarshal(b, &f); err != nil {
		return SessionFund{}, err
	}
	if err := boundAnchors(f.Anchors); err != nil {
		return SessionFund{}, err
	}
	if err := boundSigner(f.Fetcher, f.Sig); err != nil {
		return SessionFund{}, err
	}
	return f, nil
}

// UnmarshalSessionReceipt decodes MsgDeliverySettle.Data under the bounds.
func UnmarshalSessionReceipt(b []byte) (SessionReceipt, error) {
	var r SessionReceipt
	if err := cbor.Unmarshal(b, &r); err != nil {
		return SessionReceipt{}, err
	}
	if len(r.Commitment) != CommitmentSize {
		return SessionReceipt{}, fmt.Errorf("%w: commitment %d B != %d", ErrSessionBounds, len(r.Commitment), CommitmentSize)
	}
	if err := boundSigner(r.Fetcher, r.Sig); err != nil {
		return SessionReceipt{}, err
	}
	return r, nil
}

// Witness records units of witnessed delivery for object from a SESSION receipt the
// ledger settled (R2.9; certification
// silt-reviews/research/research-outcome/R2.9-witnessed-demand-observable-under-sessions-RESEARCH-CERTIFICATION-2026-09-06.md
// §3.3). The rule, clause by clause:
//
//  1. DENOMINATION: units = settled/p, where settled is SettleDelivery's first return
//     (the ledger is the authority on what was settled — never the receipt's Count,
//     never the node's delta). One unit = one DeliveryIncrementBytes witnessed.
//  2. ORDERING: called AFTER a settlement that returned ReasonPaid; the settlement is
//     never gated on this call's result.
//  3. TWO SURFACES, never one field with a flag-dependent unit: increments[object]
//     (this counter) and the distinct bonded fetchers per object (the P3b credited set,
//     read by DistinctBondedFetchers). A consumer reads exactly one.
//  4. P3b keeps its ADMISSION role — an unbonded fetcher contributes nothing to either
//     surface — and loses its dedup role on the increment counter.
//  5. ONE COUNTER, ONE DENOMINATION: the v2 lane's demand[] (one unit per redeemed
//     token, a unit differing by up to 50,000x from this one) is GONE — it retired with
//     the flat lane in C1, 2026-09-08, so there is no second surface to confuse with
//     this one.
//  6. Bounds inherited: maxDemandObjects, refuse-at-cap.
//
// The published claim this restates (P-SESSION, cert §3.4): demand_S(C)·p ≤ Σ credits
// settled at S on fetcher-signed acknowledgements naming C ≤ Σ face spent into S's
// guard; and per fetcher, over the ledger's life, Σ_C demand·p ≤ its grant. Token-level
// unforgeability becomes credit-level. Ratification of that restatement is the owner's
// (D-DEMAND's doc-truth rule); the code carries it because the v3 lane is built.
func (b *Bank) Witness(object ports.Hash, fetcherPub []byte, units int64) (credited bool, reason string) {
	if units <= 0 {
		return false, "no settled increments"
	}
	if b.bonded != nil {
		slot, ok := b.bonded(fetcherPub)
		if !ok {
			return false, "fetcher not bond-distinct (bonded-fetcher credential required)"
		}
		seen := b.credited[object]
		if seen == nil {
			if len(b.credited) >= maxDemandObjects {
				return false, "bonded-fetcher credit map full"
			}
			seen = map[string]bool{}
			b.credited[object] = seen
		}
		seen[slot] = true // the DISTINCT surface: a set, so a repeat bonded fetcher is one
	}
	if _, known := b.increments[object]; !known && len(b.increments) >= maxDemandObjects {
		return false, "witnessed-increments map full"
	}
	b.increments[object] += units
	return true, ""
}

// WitnessedIncrements is the v3 surface: increments of DeliveryIncrementBytes settled on
// fetcher-signed acknowledgements naming object. Observable, never standing. 0 for an
// object no session ever settled.
func (b *Bank) WitnessedIncrements(object ports.Hash) int64 { return b.increments[object] }

// DistinctBondedFetchers is the P3b surface: how many bond-distinct fetchers have been
// credited on object (either lane). 0 with the credential off.
func (b *Bank) DistinctBondedFetchers(object ports.Hash) int64 { return int64(len(b.credited[object])) }
