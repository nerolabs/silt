// Package demand is the blind demand receipt (D-DEMAND, issue #181): the
// interlock between the Sybil corner (standing should track WITNESSED demand, not
// self-declared popularity) and privacy (who-fetches-what stays unlinkable). It
// covers blind-withdraw → anchored session open → signed delivery-ack → witness,
// and it delivers exactly one provable property:
//
//	UNFORGEABILITY AT THE TOKEN LEVEL: a server cannot bank more receipts for an
//	object C than there were issued tokens spent on a fetcher-signed delivery ack
//	of C. #receipts(C) ≤ #issued-tokens-spent-on-a-signed-C-delivery-ack.
//
//	R2.9 (2026-09-06) — ON THE ANCHORED SESSION LANE (session.go) that property is
//	restated at the CREDIT level, P-SESSION: one token (face f) is spent at session
//	OPEN and funds up to ⌊f/p⌋ acknowledged increments, so
//	demand_S(C)·p ≤ Σ credits settled at S on fetcher-signed acks naming C ≤ Σ face
//	spent into S's guard, and per fetcher Σ_C demand·p ≤ its grant (T-QUANT). The v2
//	token-level form above stays true on its own (retiring) lane. Certification:
//	silt-agent-memory/researcher/reviews/research-outcome/R2.9-witnessed-demand-observable-under-sessions-RESEARCH-CERTIFICATION-2026-09-06.md;
//	ratification of the restatement is the owner's (D-DEMAND's doc-truth rule).
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
// NEUTRAL BY CONSTRUCTION. A settled receipt records witnessed demand as an
// OBSERVABLE (Bank.WitnessedIncrements) that is NOT wired to consensus standing — so even a
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
	"crypto/rsa"
	"io"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

// Token is an issuer-authorized retrieval credit, BLIND-WITHDRAWN (P1): a serial
// carrying an issuer blind signature in the demand domain. Because it was withdrawn
// blindly (Withdraw → SignWithdrawal → Unblind), the issuer signed the token
// WITHOUT learning its serial, so the redeemed token is cryptographically unlinkable
// to the withdrawal at issuance time. (Residual: the IP/timing channel is closed
// only by D3 issuance-mixing, shared with H8 — property (b) unlinkability is nominal
// until then.) A fetcher spends the token by presenting it as a session ANCHOR at
// open (session.go); deliveries on that session are acknowledged by SessionReceipt.
type Token struct {
	Serial []byte // unique; the double-spend key
	Sig    []byte // issuer blind signature over the serial (blindtoken demand domain)
}

// Withdraw is the fetcher side of a blind retrieval-token withdrawal FOR ISSUE
// EPOCH epoch: it blinds a fresh serial (see blindtoken.NewSerial) so the issuer
// signs it without learning the serial. It returns the blinded value to send the
// issuer and the secret to unblind the reply.
//
// chainID is THE WITHDRAWER'S OWN NETWORK — (*Node).chainID(), the genesis hash — bound
// into the same blind-signed message (M3, 2026-09-11), so a token withdrawn on network X
// verifies nowhere else. It is a REQUIRED PARAMETER and it is NOT a field of the request:
// the issuer never sees it (it signs a blinded value) and no attacker can name it. On an
// honest network the withdrawer's chain id and the redeeming server's are the same genesis
// hash, so the honest path is unchanged. The ZERO hash — a node holding no chain — is
// refused outright (blindtoken.ErrZeroChainID): a node that cannot know its network must
// not mint a token that every other chainless node would honour.
//
// The withdrawer CHOOSES epoch and binds it into the blind-signed message (R0.4b
// (b1)). The issuer signs under key_epoch only if it holds that key and epoch is in
// its own window, so a requester can never name an epoch that outlives the honest
// one; naming an EARLIER epoch only shortens its own token's life. Pass the epoch
// the withdrawer's chain-resolved keyset supplied the key for — see
// Node.AcquireDemandTokenInWindow, the only sound acquisition path.
func Withdraw(rng io.Reader, issuerPub *rsa.PublicKey, chainID ports.Hash, epoch uint64, serial []byte) (blinded, secret []byte, err error) {
	return blindtoken.BlindDemand(rng, issuerPub, chainID, epoch, serial)
}

// SignWithdrawal is the issuer side: it blind-signs the withdrawal, learning nothing
// about the serial. chainID is the ISSUER'S OWN network and a REQUIRED PARAMETER; a zero
// one is a refusal (nil), the same arm blindtoken.Issuer.Issue takes on the live issuance
// path. The issuer cannot check WHICH network the requester bound — it signs a blinded
// value — so the only issuer-side rule available is "do not be a signing oracle while you
// do not know your own network", and that is the rule. Charging or burning the fetch fee against the withdrawal is the
// caller's job (the cost-to-wash knob is P3).
//
// rng is the injected randomness the private-key operation blinds with (advisory C-2;
// see blindtoken.SignBlinded). A nil return is a refusal — a non-canonical blinded
// value, or a signature that failed verify-after-sign — and the wire already treats an
// empty reply as "no token issued".
func SignWithdrawal(rng io.Reader, issuerPriv *rsa.PrivateKey, chainID ports.Hash, blinded []byte) []byte {
	if chainID == (ports.Hash{}) {
		return nil // the issuer does not know its network: no signature (blindtoken.ErrZeroChainID)
	}
	sig, err := blindtoken.SignBlinded(rng, issuerPriv, blinded)
	if err != nil {
		return nil
	}
	return sig
}

// Unblind turns the issuer's blind signature into the unlinkable Token on the plain
// serial, using the secret from Withdraw, and VERIFIES it under (key_epoch, epoch)
// before returning (RFC 9474 §4.4 Finalize; advisory C-1). An issuer that returns a
// dud is a legible error at withdrawal instead of a token discovered worthless at
// redemption — which matters here beyond conformance, because an unredeemable token
// leaves the serve's eager self-mint un-reversed (see blindtoken.Unblind).
func Unblind(issuerPub *rsa.PublicKey, chainID ports.Hash, epoch uint64, serial, blindSig, secret []byte) (Token, error) {
	sig, err := blindtoken.UnblindDemand(issuerPub, chainID, epoch, serial, blindSig, secret)
	if err != nil {
		return Token{}, err
	}
	return Token{
		Serial: append([]byte(nil), serial...),
		Sig:    sig,
	}, nil
}

// VerifyToken reports whether t carries a valid issuer signature in the demand
// domain for ISSUE EPOCH epoch ON THE NETWORK chainID NAMES (so a publish token or credit under the same key does
// not pass, and neither does a demand token issued for a different epoch under this
// very key). Redeemers use Keyset.VerifyInWindow, which is this check run over the
// (key_e, e) pairs the window still holds.
func VerifyToken(issuerPub *rsa.PublicKey, chainID ports.Hash, epoch uint64, t Token) bool {
	return len(t.Serial) > 0 && blindtoken.VerifyDemand(issuerPub, chainID, epoch, t.Serial, t.Sig)
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

// Bank is a server's neutral demand ledger: the per-object count of witnessed
// delivery increments settled on anchored sessions, and the P3b bonded-fetcher
// credential. Both are OBSERVABLES — never read by consensus standing.
//
// THE DOUBLE-SPEND GUARD IS NOT HERE (B-9 / C1, 2026-09-08). On the v2 flat lane the
// token was spent at REDEEM, so the bank carried its own spent set. Under R2.9 the
// anchor is spent at session OPEN, into the credit ledger's shared paid-serial guard
// (core/credit, SpendDeliveryAnchors) — one guard, one clock, both anchored lanes.
// The bank's spent set retired with the flat lane; its bounded / expiry-swept /
// keyed-by-token properties are pinned on the guard that survived
// (core/credit/delivery_serial_guard_test.go).
//
// EVERY MAP HERE IS BOUNDED (build-immutable #8; red-team re-break F5, 2026-09-03).
// They were not: the observables had no cap, no sweep and no eviction path of any
// kind. increments and credited are capped by object count, and at the cap each
// REFUSES rather than evicting — forgetting a live entry is the refuted FIFO design,
// and an under-count of a neutral observable is the safe error.
type Bank struct {
	// bonded, when set (RequireBondedFetcher / P3b), gates a settlement on the fetcher
	// showing a bond-distinct credential and counts DISTINCT bonded fetchers per object
	// via credited[object][slot].
	bonded   BondCheck
	credited map[ports.Hash]map[string]bool
	// increments (R2.9, the v3 surface): witnessed increments of DeliveryIncrementBytes
	// per object, settled on session receipts. Bounded by maxDemandObjects.
	increments map[ports.Hash]int64
}

// maxDemandObjects bounds increments[] and credited[]. Each entry costs the attacker a
// real withdrawal fee (a settlement only lands here after its anchor was spent), so
// this is a ceiling on memory, not a rate limit.
const maxDemandObjects = 1 << 20

// NewBank returns an empty demand ledger.
func NewBank() *Bank {
	return &Bank{
		increments: map[ports.Hash]int64{},
		credited:   map[ports.Hash]map[string]bool{},
	}
}

// RequireBondedFetcher turns on the P3b bonded-fetcher credential: from now on a
// receipt counts toward demand only if check reports the fetcher's key bond-distinct,
// and demand counts distinct bonded fetchers per object (one bonded identity per unit
// of fake demand). Off by default (raw witnessed count). See BondCheck.
func (b *Bank) RequireBondedFetcher(check BondCheck) { b.bonded = check }

// maxTokenSigBytes bounds a blind signature on the wire: it is an element of Z_N, so
// it cannot exceed the largest modulus any lane will hold (blindtoken.MaxModulusBits).
// Read by the session decode's anchor bound (session.go, boundAnchors).
const maxTokenSigBytes = blindtoken.MaxModulusBits / 8
