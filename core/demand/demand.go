// Package demand is the blind demand receipt (D-DEMAND, issue #181): the
// interlock between the Sybil corner (standing should track WITNESSED demand, not
// self-declared popularity) and privacy (who-fetches-what stays unlinkable). It
// covers blind-withdraw → signed delivery-ack → bank → redeem, for a SINGLE
// object, and it delivers exactly one provable property:
//
//	UNFORGEABILITY AT THE TOKEN LEVEL: a server cannot bank more receipts for an
//	object C than there were issued tokens spent on a fetcher-signed delivery ack
//	of C. #receipts(C) ≤ #issued-tokens-spent-on-a-signed-C-delivery-ack.
//
// THE RECEIPT CARRIES NO PoR PROOF (certified 2026-08-26, the PoD neutral-lane
// certification, Q2). The earlier P0 shape bound a Shacham–Waters proof over the
// delivered bytes, but its per-object key seed was public, so the proof was
// forgeable with zero object bytes (owned residual B3) — a forgeable binding
// deters no collusion, and it cost a 128-sample prove+verify per delivery on the
// hobbyist floor box (build-immutable #8). The certified neutral-lane receipt is
// token + fetcher signature + the (serial‖object‖server) binding: an honest
// fetcher signs only after the fetch path re-verified the bytes against the
// content address (tenet B3), and a colluding pair gains nothing a proof would
// deny (conservation makes forgery strictly loss-making — see the certification).
// A possession binding re-enters only where loss-deterrence stops covering
// (receipt→standing, relay), as a content-committed recompute floor — not here.
//
// What it deliberately does NOT prove (a Douceur limit, not an engineering gap, per
// the decision's doc-truth rule): **demand AUTHENTICITY.** A server can run its own
// fetcher, pay itself, fetch its own content, and mint perfectly valid receipts — a
// self-fetch IS a real paid correct delivery, and no cryptographic receipt certifies
// the counterparty was economically independent. Authenticity is re-priced, never
// proven, by cost-to-wash (fee-burn + bonded-fetcher credential — P3). Any claim
// that a receipt proves organic third-party demand is false.
//
// NEUTRAL BY CONSTRUCTION. A redeemed receipt records witnessed demand as an
// OBSERVABLE (Bank.Demand) that is NOT wired to consensus standing — so even a
// forged or self-dealt receipt buys ZERO standing. That is what keeps the γ→1/N
// shared-content sealing firewall intact (fusing demand into standing is gated on
// the open sealing problem, m0.md §10 / #182). Whether/how witnessed demand ever
// feeds standing is a separate, gated decision this package does not make.
//
// BLIND WITHDRAWAL IS BUILT (P1): the retrieval token is withdrawn under an issuer
// blind signature (Withdraw → SignWithdrawal → Unblind, core/blindtoken demand
// domain), so the issuer signs it without learning the serial — the redeemed token
// is cryptographically unlinkable to its withdrawal. But FETCHER-UNLINKABILITY is
// only NOMINAL until D3 issuance-mixing closes the IP/timing channel (shared with
// H8/#179): the blind signature hides the serial, not the network identity of the
// withdrawer. COST-TO-WASH (P3) is priced by two levers: the fee-burn (P3a, a sim
// property — each wash burns a real retrieval fee) and the BONDED-FETCHER CREDENTIAL
// (P3b, built here — Bank.RequireBondedFetcher: demand counts distinct bonded
// fetchers, so wash costs one storage bond per faked unit). LATER PHASE: optimistic
// fair-exchange dispute (P2).
package demand

import (
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

// Token is an issuer-authorized retrieval credit, BLIND-WITHDRAWN (P1): a serial
// carrying an issuer blind signature in the demand domain. Because it was withdrawn
// blindly (Withdraw → SignWithdrawal → Unblind), the issuer signed the token
// WITHOUT learning its serial, so the redeemed token is cryptographically unlinkable
// to the withdrawal at issuance time. (Residual: the IP/timing channel is closed
// only by D3 issuance-mixing, shared with H8 — property (b) unlinkability is nominal
// until then.) A fetcher spends the token by producing a DeliveryReceipt that
// references its serial.
type Token struct {
	Serial []byte // unique; the double-spend key
	Sig    []byte // issuer blind signature over the serial (blindtoken demand domain)
}

// Withdraw is the fetcher side of a blind retrieval-token withdrawal FOR ISSUE
// EPOCH epoch: it blinds a fresh serial (see blindtoken.NewSerial) so the issuer
// signs it without learning the serial. It returns the blinded value to send the
// issuer and the secret to unblind the reply.
//
// The withdrawer CHOOSES epoch and binds it into the blind-signed message (R0.4b
// (b1)). The issuer signs under key_epoch only if it holds that key and epoch is in
// its own window, so a requester can never name an epoch that outlives the honest
// one; naming an EARLIER epoch only shortens its own token's life. Pass the epoch
// the withdrawer's chain-resolved keyset supplied the key for — see
// Node.AcquireDemandTokenInWindow, the only sound acquisition path.
func Withdraw(rng io.Reader, issuerPub *rsa.PublicKey, epoch uint64, serial []byte) (blinded, secret []byte, err error) {
	return blindtoken.BlindDemand(rng, issuerPub, epoch, serial)
}

// SignWithdrawal is the issuer side: it blind-signs the withdrawal, learning nothing
// about the serial. Charging or burning the fetch fee against the withdrawal is the
// caller's job (the cost-to-wash knob is P3).
func SignWithdrawal(issuerPriv *rsa.PrivateKey, blinded []byte) []byte {
	return blindtoken.SignBlinded(issuerPriv, blinded)
}

// Unblind turns the issuer's blind signature into the unlinkable Token on the plain
// serial, using the secret from Withdraw.
func Unblind(issuerPub *rsa.PublicKey, serial, blindSig, secret []byte) Token {
	return Token{
		Serial: append([]byte(nil), serial...),
		Sig:    blindtoken.Unblind(issuerPub, blindSig, secret),
	}
}

// VerifyToken reports whether t carries a valid issuer signature in the demand
// domain for ISSUE EPOCH epoch (so a publish token or credit under the same key does
// not pass, and neither does a demand token issued for a different epoch under this
// very key). Redeemers use Keyset.VerifyInWindow, which is this check run over the
// (key_e, e) pairs the window still holds.
func VerifyToken(issuerPub *rsa.PublicKey, epoch uint64, t Token) bool {
	return len(t.Serial) > 0 && blindtoken.VerifyDemand(issuerPub, epoch, t.Serial, t.Sig)
}

// DeliveryReceipt is a fetcher's signed acknowledgement that it received the
// correct bytes of Object from Server, spending the token named by Serial. Its
// binding is twofold: the fetcher signature (only the token's spender can mint
// it — and an honest fetcher signs only after the fetch path content-verified
// the bytes, tenet B3) and the (serial‖object‖server) triple inside the signed
// message (a receipt for one delivery cannot be replayed for another object or
// redeemed by another server). No PoR proof rides the receipt — see the package
// comment for the certified reasoning.
type DeliveryReceipt struct {
	Serial  []byte       // the spent token's serial
	Object  ports.Hash   // C — the content-addressed object delivered
	Server  ports.NodeID // the delivering server (the redeemer); binds the ack
	Fetcher []byte       // the fetcher's ed25519 public key
	Sig     []byte       // fetcher's signature over receiptMsg
}

// receiptMsg is the bytes the fetcher signs: every field that fixes what was
// delivered and to whom-for, so tampering any of them breaks the signature.
// The domain is v2: the v1 message shape carried PoR fields, and the bump makes
// a v1 signature unusable on a v2 receipt (and vice versa) by construction.
func (r DeliveryReceipt) receiptMsg() []byte {
	h := sha256.New()
	h.Write([]byte("silt/demand/receipt/v2"))
	h.Write(r.Serial)
	h.Write(r.Object[:])
	h.Write(r.Server[:])
	h.Write(r.Fetcher)
	return h.Sum(nil)
}

// Ack is the fetcher side: having received and content-verified Object's bytes,
// the fetcher spends token by signing a delivery receipt naming this exact
// (serial, object, server). Cheap by design — one ed25519 sign, nothing else —
// so it runs on the floor box at delivery rate.
func Ack(fetcher ed25519.PrivateKey, token Token, object ports.Hash, server ports.NodeID) DeliveryReceipt {
	r := DeliveryReceipt{
		Serial:  append([]byte(nil), token.Serial...),
		Object:  object,
		Server:  server,
		Fetcher: append([]byte(nil), fetcher.Public().(ed25519.PublicKey)...),
	}
	r.Sig = ed25519.Sign(fetcher, r.receiptMsg())
	return r
}

// BondCheck is the P3b bonded-fetcher credential: given a fetcher's receipt-signing
// key, it reports whether that key belongs to a scarce, bond-distinct identity and
// returns an opaque distinctness key (the "slot") for it. The bank counts demand
// PER DISTINCT SLOT, so N receipts from ONE bonded fetcher raise demand by 1 — this
// re-prices wash to "one bonded fetcher identity per unit of fake demand" (the
// D-DEMAND decision's second cost-to-wash lever, alongside the fee-burn). Backed in
// production by the COMMITTED on-chain bond ledger (chain.BondedSize via the node's
// RequireBondedFetchers) — the same Sybil-priced, deduped supply C2 measures — so
// faking U units of demand costs U real storage bonds. Nil ⇒ the gate is off and the
// bank keeps its raw witnessed-delivery count (unchanged behavior).
//
// UNLINKABILITY (M0 residual, not a gap): the fetcher shows its bonded key in the
// clear, so the credential currently LINKS a fetch to the bonded identity — fetcher-
// unlinkability stays NOMINAL (the same D3/H8 channel that leaves the rest of demand
// nominal). This signature is the exact seam where a BLIND bond-distinctness proof
// ("I hold some bonded identity, distinct from my other shows, without revealing
// which") restores unlinkability; it composes with D3 issuance-mixing and is not
// built in M0.
type BondCheck func(fetcherPubKey []byte) (slot string, ok bool)

// Bank is a server's neutral demand ledger: the set of already-redeemed TOKENS
// (double-spend guard) and the per-object count of witnessed deliveries.
// Demand is an OBSERVABLE — it is never read by consensus standing.
//
// EVERY MAP HERE IS BOUNDED (build-immutable #8; red-team re-break F5, 2026-09-03).
// They were not: spent, demand and credited had no cap, no sweep and no eviction path
// of any kind, in the very layer the credit guard's bounded-and-expiry-swept design
// leans on for its "evicted ⇒ expired" coupling. spent is capped and expiry-swept on
// the token validity window; demand and credited are capped by object count. At the
// cap every one of them REFUSES rather than evicting — forgetting a live entry is the
// refuted FIFO design, and an under-count of a neutral observable is the safe error.
type Bank struct {
	// spent is keyed by (ISSUE EPOCH, serial) — the token, not the serial. Keying by
	// serial alone made the entry's expiry epoch the MINIMUM over the tokens sharing a
	// serial (the withdrawer picks both), so the guard forgot a serial while a
	// still-in-window token on it existed. The value is the issue epoch, so the sweep
	// never has to decode a key.
	spent map[string]uint64
	// sweptEpoch is the last epoch sweepExpiredSpent actually ran at: entries expire on
	// the epoch clock, so a second scan inside one epoch can free nothing the first did
	// not, and the amortized cost drops from O(cap) per redeem to O(cap) per epoch.
	sweptEpoch uint64
	demand     map[ports.Hash]int64
	// bonded, when set (RequireBondedFetcher / P3b), gates a receipt on the fetcher
	// showing a bond-distinct credential and makes demand count DISTINCT bonded
	// fetchers per object via credited[object][slot].
	bonded   BondCheck
	credited map[ports.Hash]map[string]bool
}

// The bank's bounds. maxSpentTokens is DERIVED the same way core/credit derives its
// paid-serial cap — W epochs x the epoch cadence x the per-block object-aware serve
// rate, floored generously — so the guard can never deny an honest lane at any serve
// rate the design models. maxDemandObjects bounds the two per-object observables.
const (
	spentEpochBlocks    = 8   // the epoch cadence the cap is derived against
	spentServesPerBlock = 256 // the per-block serve rate the cap is sized for
	maxSpentFloor       = 65_536
	maxSpentTokens      = max(maxSpentFloor, int(DefaultWindow)*spentEpochBlocks*spentServesPerBlock)
	// maxDemandObjects bounds demand[] and credited[]. Each entry costs the attacker a
	// real withdrawal fee (a receipt only lands here after its token verified), so this
	// is a ceiling on memory, not a rate limit.
	maxDemandObjects = 1 << 20
)

// NewBank returns an empty demand ledger.
func NewBank() *Bank {
	return &Bank{
		spent:    map[string]uint64{},
		demand:   map[ports.Hash]int64{},
		credited: map[ports.Hash]map[string]bool{},
	}
}

// spentKey is the (issue epoch, serial) guard key: 8-byte big-endian epoch then the
// serial, the same epoch wire form the FDH message and the issuerKeyCommit leaf use.
func spentKey(epoch uint64, serial []byte) string {
	var k [8]byte
	binary.BigEndian.PutUint64(k[:], epoch)
	return string(k[:]) + string(serial)
}

// sweepExpiredSpent drops every guarded token whose ISSUE EPOCH has left the validity
// window at current. THIS IS THE ONLY EVICTION PATH: a dropped token is one no held
// key_E verifies any more (keyset.Prune has already removed its key), so evicted ⇒
// expired ⇒ un-redeemable, per entry, which is the coupling condition the R0.4b
// certification requires.
func (b *Bank) sweepExpiredSpent(current, window uint64) {
	if current <= window {
		return // nothing can have left the window yet
	}
	floor := current - window
	for k, e := range b.spent {
		if e < floor {
			delete(b.spent, k)
		}
	}
}

// reserveSpent makes room for one more guarded token, or reports that it cannot. It
// sweeps expired entries (at most once per epoch) and only then checks the cap; it
// NEVER evicts a live entry. false ⇒ the caller refuses the receipt.
func (b *Bank) reserveSpent(current, window uint64) bool {
	if len(b.spent) < maxSpentTokens {
		return true
	}
	if current > b.sweptEpoch {
		b.sweptEpoch = current
		b.sweepExpiredSpent(current, window)
	}
	return len(b.spent) < maxSpentTokens
}

// RequireBondedFetcher turns on the P3b bonded-fetcher credential: from now on a
// receipt counts toward demand only if check reports the fetcher's key bond-distinct,
// and demand counts distinct bonded fetchers per object (one bonded identity per unit
// of fake demand). Off by default (raw witnessed count). See BondCheck.
func (b *Bank) RequireBondedFetcher(check BondCheck) { b.bonded = check }

// Redeem verifies a banked receipt against the token that authorized it and, if it
// is a first-seen, validly-issued, IN-WINDOW, correctly-delivered receipt for the
// named server, credits the object's witnessed-demand counter once. It reports
// whether it credited, the token's ISSUING EPOCH (so the caller can carry expiry
// down to the credit layer), and the reason it didn't credit (for
// observability/tests). It NEVER touches standing.
//
// keys is the redeemer's per-epoch issuer keyset and current is the CONSENSUS epoch
// index (head_height / EpochBlocks — deterministic and Byzantine-agreed, never
// wall-clock). The token is accepted only if some held key_E verifies it with
// current − E <= W (R0.4b; see keyset.go). The issuing epoch is DISCOVERED here —
// it rides no field on the token and is never recorded against a withdrawer.
//
// EXPIRY IS PURELY SUBTRACTIVE. It can only ever REJECT a redemption, and a rejected
// redeem credits zero — the return happens before b.demand[...]++ and before the
// serial is marked spent. So conservation (#receipts(C) <= #issued-tokens-spent) is
// preserved a fortiori: a narrower acceptance set cannot exceed the issued-token
// bound. No path here mints.
//
// Rejections (each an unforgeability property): a token no in-window issuer key
// signed (not issued, or EXPIRED — the two are indistinguishable by construction,
// which is the point); a serial already redeemed (double-spend); a receipt whose
// serial ≠ the token's; a fetcher signature that doesn't verify (which also covers
// a receipt tampered toward another object, server, or fetcher — all three are
// inside the signed message).
func (b *Bank) Redeem(keys *Keyset, current uint64, token Token, r DeliveryReceipt) (credited bool, issuedEpoch uint64, reason string) {
	if keys == nil {
		return false, 0, "no issuer keyset"
	}
	issuedEpoch, inWindow := keys.VerifyInWindow(current, token)
	if !inWindow {
		return false, 0, "token expired or not issued"
	}
	if string(token.Serial) != string(r.Serial) {
		return false, issuedEpoch, "receipt serial does not match token"
	}
	// The serial is ATTACKER-CHOSEN and becomes a map key, so its LENGTH is bounded
	// here as well as at the wire decode (red-team re-break F5 amplifier): no honest
	// withdrawal produces anything but a blindtoken.SerialSize serial, and the guards'
	// entry CAP bounds count, never bytes.
	if len(r.Serial) > blindtoken.SerialSize {
		return false, issuedEpoch, "token serial is longer than a serial can be"
	}
	key := spentKey(issuedEpoch, r.Serial)
	if _, done := b.spent[key]; done {
		return false, issuedEpoch, "double-spend: serial already redeemed"
	}
	if len(r.Fetcher) != ed25519.PublicKeySize || !ed25519.Verify(r.Fetcher, r.receiptMsg(), r.Sig) {
		return false, issuedEpoch, "fetcher signature invalid"
	}
	// The guard set is BOUNDED, and at the cap it refuses rather than forgetting a
	// live token (build-immutable #8; the refuted alternative is FIFO eviction, which
	// is self-financing for the flooder). Reserve BEFORE anything is recorded so a
	// refusal leaves no trace.
	if !b.reserveSpent(current, keys.Window()) {
		return false, issuedEpoch, "double-spend guard full of still-redeemable tokens"
	}
	// The token is now consumed — crypto, issuance, and delivery all verified — so
	// mark the token spent BEFORE the bonded-fetcher gate. A receipt rejected only
	// for lacking the credential still burns its one-time token, so an unbonded
	// self-dealer cannot retry the same token to amplify anything.
	b.spent[key] = issuedEpoch
	// P3b bonded-fetcher credential: count toward demand only if the fetcher shows a
	// bond-distinct credential, and count DISTINCT bonded fetchers per object — so a
	// self-dealer running one bonded identity mints N valid receipts but moves demand
	// by 1, not N (Douceur is unbeaten, but wash is re-priced onto the bonded-identity
	// supply). Off (bonded==nil) keeps the raw witnessed count.
	if b.bonded != nil {
		slot, ok := b.bonded(r.Fetcher)
		if !ok {
			return false, issuedEpoch, "fetcher not bond-distinct (bonded-fetcher credential required)"
		}
		seen := b.credited[r.Object]
		if seen == nil {
			if len(b.credited) >= maxDemandObjects {
				return false, issuedEpoch, "bonded-fetcher credit map full"
			}
			seen = map[string]bool{}
			b.credited[r.Object] = seen
		}
		if seen[slot] {
			return false, issuedEpoch, "demand already counted for this bonded fetcher on this object"
		}
		seen[slot] = true
	}
	if _, known := b.demand[r.Object]; !known && len(b.demand) >= maxDemandObjects {
		return false, issuedEpoch, "witnessed-demand map full"
	}
	b.demand[r.Object]++
	return true, issuedEpoch, ""
}

// Demand is the witnessed-delivery count banked for object — an observable, never
// standing. Reading it moves nothing.
func (b *Bank) Demand(object ports.Hash) int64 { return b.demand[object] }

// SubmittedReceipt bundles a delivery receipt with the token that authorized it —
// the pair a fetcher hands the server so the server can bank and redeem in one
// message. Marshal to CBOR for the wire.
type SubmittedReceipt struct {
	Token   Token
	Receipt DeliveryReceipt
}

// Marshal serializes the bundle to CBOR.
func (s SubmittedReceipt) Marshal() ([]byte, error) {
	return cbor.Marshal(s)
}

// ErrOversizedReceipt rejects a wire bundle whose attacker-chosen variable-length
// fields exceed what the lane can ever have issued.
var ErrOversizedReceipt = errors.New("demand: submitted receipt field exceeds its bound")

// maxTokenSigBytes bounds the blind signature on the wire: it is an element of Z_N,
// so it cannot exceed the largest modulus any lane will hold
// (blindtoken.MaxModulusBits).
const maxTokenSigBytes = blindtoken.MaxModulusBits / 8

// UnmarshalSubmittedReceipt parses a wire bundle. Every field is untrusted input to
// be re-verified at Redeem, never established fact.
//
// THE VARIABLE-LENGTH FIELDS ARE BOUNDED HERE (red-team re-break F5 amplifier,
// 2026-09-03). A SubmittedReceipt rides a 132 MiB frame (adapters/tcpnet), and the
// serial becomes a KEY in two long-lived maps — the demand bank's spent set and the
// credit ledger's paid-serial guard — both of which cap COUNT and neither of which
// capped BYTES. A single accepted receipt could therefore pin an attacker-chosen
// number of megabytes. An honest serial is exactly blindtoken.SerialSize; an honest
// signature is at most one modulus wide. Anything else is a decode error, refused
// before it can be stored, counted, or hashed.
func UnmarshalSubmittedReceipt(b []byte) (SubmittedReceipt, error) {
	var s SubmittedReceipt
	if err := cbor.Unmarshal(b, &s); err != nil {
		return SubmittedReceipt{}, err
	}
	if len(s.Token.Serial) > blindtoken.SerialSize || len(s.Receipt.Serial) > blindtoken.SerialSize {
		return SubmittedReceipt{}, fmt.Errorf("%w: serial %d/%d bytes, max %d",
			ErrOversizedReceipt, len(s.Token.Serial), len(s.Receipt.Serial), blindtoken.SerialSize)
	}
	if len(s.Token.Sig) > maxTokenSigBytes {
		return SubmittedReceipt{}, fmt.Errorf("%w: token signature %d bytes, max %d",
			ErrOversizedReceipt, len(s.Token.Sig), maxTokenSigBytes)
	}
	if len(s.Receipt.Sig) > ed25519.SignatureSize || len(s.Receipt.Fetcher) > ed25519.PublicKeySize {
		return SubmittedReceipt{}, fmt.Errorf("%w: receipt signature %d bytes / fetcher key %d bytes",
			ErrOversizedReceipt, len(s.Receipt.Sig), len(s.Receipt.Fetcher))
	}
	return s, nil
}
