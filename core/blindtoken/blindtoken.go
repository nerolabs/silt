// Package blindtoken is a Chaumian blind signature — the crypto behind
// publisher-privacy publish tokens (T3, risk 14 / catalog F1). The problem:
// the chain records a publish's Publisher NodeID, so an observer maps a
// durable reputation key to every root it published (silt protects who-READS
// far better than who-WRITES). A blind token breaks that link while keeping
// the fee/anti-spam economics:
//
//  1. A publisher, using its durable identity, asks the issuer for a token.
//     It BLINDS a random serial and sends only the blinded value; the issuer
//     charges the fee to the durable key and signs the blinded value — WITHOUT
//     ever seeing the serial.
//  2. The publisher UNBLINDS the issuer's signature into a valid signature on
//     the plain serial, and later spends (serial, sig) to publish. The token
//     verifies (a fee was paid) but is unlinkable to the issuance session, so
//     the publish carries no durable identity.
//
// This is textbook RSA-FDH blind signatures (Chaum 1982): sig = FDH(serial)^d
// mod N; blinding multiplies by r^e so the issuer signs FDH(serial)·r, and
// dividing by r recovers FDH(serial)^d. Standard construction, NOT
// independently audited (see the threat model's crypto-composition note).
package blindtoken

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
)

const fdhDomain = "silt/blindtoken/fdh/v1"

// creditDomain is the FDH domain for PREPAID PUBLISH CREDITS (M0 privacy D3,
// red-team F4). A credit is a blind signature under the SAME issuer key as a
// publish token, but over a domain-separated message — so a credit signature can
// never be presented as a publish token (or vice versa): each verifies only
// under its own domain. That lets one issuer key mint both a bulk, fee-charged
// credit (decoupled from any publish) and, spending that credit, an unlinkable
// publish token WITHOUT a second per-publish fee — the ledger link the red-team
// exploited. This is online Chaumian e-cash (Chaum 1982): the coin is a
// blind-signed serial, double-spend is caught by the issuer's spent-set.
const creditDomain = "silt/blindcredit/fdh/v1"

// demandDomain is the FDH domain for BLIND-WITHDRAWN RETRIEVAL TOKENS (D-DEMAND
// P1, issue #181). A retrieval token is a blind signature under an issuer key,
// domain-separated from a publish token and a publish credit — so a demand token
// can never be presented as either (or vice versa) even under one key. The blind
// withdrawal is what makes the token unlinkable to the fetch at issuance time (the
// issuer signs the blinded value, never seeing the serial); the residual IP/timing
// channel is closed only by D3 issuance-mixing (shared with H8).
//
// v2 BINDS THE ISSUE EPOCH INTO THE SIGNED MESSAGE (R0.4b (b1), certified
// 2026-09-02 in the red-team reconciliation verdict §2.4). The MOVE is Privacy Pass's:
// RFC 9578 (Blind RSA, type 0x0002) signs token_input = token_type ‖ nonce ‖
// challenge_digest ‖ token_key_id, i.e. the key's identity is inside the signed
// message, which is why a Privacy Pass token cannot be re-dated. The first R0.4b
// import took Privacy Pass's key-per-period schema but dropped that binding, and
// the omission was the whole break: with the epoch living only in the KEY, one RSA
// key bound to two epochs (which an ordinary restart does) let a verifier re-date
// an old token to the newest epoch that key is held for, so guard entries expired
// while tokens did not, and the cross-server double-redeem pump re-opened.
//
// WHAT SILT SIGNS IS NOT RFC 9578's token_key_id (advisory C-4, 2026-09-03).
// token_key_id is SHA256(DER SPKI of pk_I) — a hash of the whole issuer public key.
// silt signs an EPOCH INDEX. That closes re-dating and nothing more; it does not bind
// the issuer or the key identity, and the DSKS surface that leaves is named and
// tracked in core/demand/keyset.go. Do not call this "the RFC 9578 schema".
//
// Binding E into the FDH input makes issuedEpoch(token) a PURE FUNCTION OF THE
// TOKEN: a signature made over (E, serial) verifies under exactly the pair
// (key_E, E) and under no other epoch, whatever key schedule the issuer runs. The
// v1 domain is retired rather than extended so a v1 (unbound) demand signature can
// never verify as a v2 one.
//
// This adds NO field to the token and NO field to the wire: the epoch is chosen by
// the requester, blinded into the message, and named in the request the issuer
// already answers with an epoch. See core/demand/keyset.go.
const demandDomain = "silt/blinddemand/fdh/v2"

// SerialSize is the length of a token's random serial.
const SerialSize = 32

var (
	ErrBadToken = errors.New("blindtoken: signature does not verify")
	ErrZeroHash = errors.New("blindtoken: serial hashed to zero (retry)")
	// ErrNotCoprime is RFC 9474 §4.2 step 4: gcd(m, N) must be 1 (advisory C-6).
	ErrNotCoprime = errors.New("blindtoken: message is not coprime to the modulus")
	// ErrNotCanonical is the RFC 8017 §5.2.2 range check plus silt's minimal-encoding
	// requirement on any RSA representative crossing the wire (advisory C-5).
	ErrNotCanonical = errors.New("blindtoken: non-canonical RSA representative")
	// ErrSignFault is the verify-after-sign refusal (advisory C-2): the signature the
	// modexp produced does not verify under the issuer's own public key.
	ErrSignFault = errors.New("blindtoken: signature failed verify-after-sign")
)

var bigOne = big.NewInt(1)

// coprime is RFC 9474 §4.2's is_coprime(m, n). Factored out of blindD so it can be
// driven directly: the full path cannot be exercised end to end, because ValidatePub
// now rejects every modulus whose factors a test could actually find (smooth, perfect
// power, prime), and finding m with gcd(m, N) > 1 for an honest semiprime IS factoring.
func coprime(m, n *big.Int) bool {
	return new(big.Int).GCD(nil, nil, m, n).Cmp(bigOne) == 0
}

// NewSerial draws a fresh random token serial.
func NewSerial(r io.Reader) ([]byte, error) {
	s := make([]byte, SerialSize)
	if _, err := io.ReadFull(r, s); err != nil {
		return nil, err
	}
	return s, nil
}

// fullDomainHash maps serial to an element of Z_N (RSA-FDH) under the default
// publish-token domain. See fullDomainHashD for the domain-separated form.
func fullDomainHash(pub *rsa.PublicKey, serial []byte) *big.Int {
	return fullDomainHashD(pub, serial, fdhDomain)
}

// fullDomainHashD maps msg to an element of Z_N (RSA-FDH) under an explicit
// domain: expand SHA-256 in counter mode past the modulus length, then reduce.
// msg is the serial for the publish and credit domains, and the epoch-prefixed
// demandMsg for the demand domain.
// Domain-separated and a few bytes wide of N to keep the mod bias negligible.
// Distinct domains yield unrelated messages under the same key, which is how a
// credit signature and a token signature made by one issuer never interchange.
func fullDomainHashD(pub *rsa.PublicKey, msg []byte, domain string) *big.Int {
	nLen := (pub.N.BitLen() + 7) / 8
	out := make([]byte, 0, nLen+sha256.Size)
	for ctr := uint32(0); len(out) < nLen+8; ctr++ {
		h := sha256.New()
		h.Write([]byte(domain))
		var cb [4]byte
		binary.BigEndian.PutUint32(cb[:], ctr)
		h.Write(cb[:])
		h.Write(msg)
		out = h.Sum(out)
	}
	m := new(big.Int).SetBytes(out)
	return m.Mod(m, pub.N)
}

// randInt draws a random value in [0, max) from the injected reader. Core must
// not touch ambient randomness (crypto/rand) — the adapter passes crypto/rand,
// a sim passes a seeded source. A few extra bytes keep the mod bias negligible;
// uniformity isn't security-critical for the blinding factor anyway.
func randInt(rng io.Reader, max *big.Int) (*big.Int, error) {
	b := make([]byte, (max.BitLen()+7)/8+8)
	if _, err := io.ReadFull(rng, b); err != nil {
		return nil, err
	}
	return new(big.Int).Mod(new(big.Int).SetBytes(b), max), nil
}

// Blind blinds serial for the issuer and returns the blinded value to send and
// the secret (blinding factor) the publisher keeps to unblind the reply. Both
// are big-endian byte strings. rng supplies randomness (injected, not ambient).
func Blind(rng io.Reader, pub *rsa.PublicKey, serial []byte) (blinded, secret []byte, err error) {
	return blindD(rng, pub, serial, fdhDomain)
}

// BlindCredit blinds serial as a PREPAID CREDIT (domain-separated from a publish
// token, same key). Used at bulk mint time; the returned credit spends later for
// a token with no per-publish fee (F4).
func BlindCredit(rng io.Reader, pub *rsa.PublicKey, serial []byte) (blinded, secret []byte, err error) {
	return blindD(rng, pub, serial, creditDomain)
}

// BlindDemand blinds serial as a RETRIEVAL TOKEN for ISSUE EPOCH epoch (D-DEMAND,
// domain-separated from a publish token and a credit, same key). The fetcher
// withdraws it blindly so the issuer cannot link the token to a later fetch; it
// spends at delivery-ack time.
//
// epoch is bound INTO the blind-signed message (see demandDomain), so the resulting
// signature verifies under the pair (key_epoch, epoch) and no other — which is what
// makes a token's issue epoch un-re-dateable. The issuer learns only the epoch the
// request names (its own clock ±1); it never learns the serial.
func BlindDemand(rng io.Reader, pub *rsa.PublicKey, epoch uint64, serial []byte) (blinded, secret []byte, err error) {
	return blindD(rng, pub, demandMsg(epoch, serial), demandDomain)
}

// demandMsg is the byte-exact demand FDH input: the 8-byte big-endian issue epoch
// followed by the serial. Big-endian to match every other epoch encoding in the
// codebase (chain.issuerKeyRegMsg, the issuerKeyCommit leaf key, demandDedupKey), so
// there is ONE epoch wire form to reason about. Pinned byte-for-byte by
// TestDemandFDHInputBindsTheEpochByteExactly.
func demandMsg(epoch uint64, serial []byte) []byte {
	m := make([]byte, 8, 8+len(serial))
	binary.BigEndian.PutUint64(m, epoch)
	return append(m, serial...)
}

func blindD(rng io.Reader, pub *rsa.PublicKey, msg []byte, domain string) (blinded, secret []byte, err error) {
	// LAST-LINE KEY VALIDITY (red-team re-break F4). Every modexp below reduces mod
	// pub.N, and big.Int.Mod PANICS on a zero modulus — a committed N = 0 crashed the
	// whole fetcher lane. The key should already have been refused at ParsePub and at
	// Keyset.Put; this makes a bypass a legible error instead of a process death. It is
	// the SHAPE half only — see validateShape for why the hardness checks belong at
	// admission and not on a hot path.
	if err := validateShape(pub); err != nil {
		return nil, nil, err
	}
	m := fullDomainHashD(pub, msg, domain)
	if m.Sign() == 0 {
		return nil, nil, ErrZeroHash
	}
	// RFC 9474 §4.2 step 4: c = is_coprime(m, n); if not, raise an error (C-6).
	// If gcd(m, N) > 1 the blinded value carries the same common factor, so the issuer
	// can compute gcd(b, N) at issuance and gcd(FDH(E‖serial), N) at redemption and
	// LINK them — a blindness break, not a forgery. It is negligible for an honest
	// modulus and NOT negligible for a deliberately smooth one, which is exactly the
	// key shape ValidatePub's hardness checks bound but cannot eliminate.
	if !coprime(m, pub.N) {
		return nil, nil, ErrNotCoprime
	}
	// r random in [1, N), invertible mod N. r = 1 is NO blinding; it is drawn with
	// probability 2^-2048, but the check is free and the comment must not claim a
	// bound the code does not enforce.
	var r, rInv *big.Int
	for {
		r, err = randInt(rng, pub.N)
		if err != nil {
			return nil, nil, err
		}
		if r.Sign() == 0 || r.Cmp(bigOne) == 0 {
			continue
		}
		rInv = new(big.Int).ModInverse(r, pub.N)
		if rInv != nil { // coprime to N (overwhelmingly likely)
			break
		}
	}
	// blinded = m * r^e mod N
	re := new(big.Int).Exp(r, big.NewInt(int64(pub.E)), pub.N)
	b := new(big.Int).Mul(m, re)
	b.Mod(b, pub.N)
	return b.Bytes(), r.Bytes(), nil
}

// SignBlinded is the ISSUER side: it signs the blinded value, learning nothing
// about the serial. (Charging the fee to the requester is the caller's job.)
//
// THREE DEFENCES AROUND ONE MODEXP (crypto-specialist advisory C-2 / C-5, 2026-09-03).
// This is a network-facing signing oracle over a fully attacker-chosen input, so the
// bare `Exp(b, priv.D, priv.N)` it used to be was the Brumley-Boneh remote-timing
// setting exactly:
//
//  1. RANGE CHECK the input (RFC 8017 §5.2.2 RSAVP1: the representative must be in
//     [0, N-1]) and require a CANONICAL big-endian encoding. The old code silently did
//     `b.Mod(b, N)`, so `blinded` and `blinded + N` — and any zero-padded spelling —
//     signed identically. The issuance dedup cache is keyed on the RAW BLINDED BYTES
//     (core/node/demandkeys.go demandDedupKey), so that was a dedup bypass: a
//     requester could re-present a re-encoded blind and be charged again for an
//     issuance the issuer had already settled.
//  2. RANDOM BLINDING of the private operation. Go's math/big documents that Exp "is
//     not a cryptographically constant-time operation", and Go's own crypto/rsa blinds
//     every private-key op for this reason. CLIENT-side blinding does not help the
//     ISSUER: the client chose r and knows m, so it knows the exact modexp input.
//     rng is injected — core touches no ambient randomness (internal/depcheck).
//  3. VERIFY-AFTER-SIGN. s^e == b mod N before the signature is released, the
//     Boneh-DeMillo-Lipton fault-attack countermeasure Go pairs with CRT in
//     decryptAndCheck. It is also a free correctness check on our own signer.
//
// The WIRE FORMAT is unchanged: the returned bytes are s.Bytes(), exactly as before.
func SignBlinded(rng io.Reader, priv *rsa.PrivateKey, blinded []byte) ([]byte, error) {
	if priv == nil || priv.N == nil {
		return nil, fmt.Errorf("%w: nil private key", ErrBadPubKey)
	}
	b, err := canonicalRep(blinded, priv.N)
	if err != nil {
		return nil, err
	}
	e := big.NewInt(int64(priv.E))
	// Blind: sign b·u^e and divide the result by u. The signature is unchanged
	// (s = (b·u^e)^d · u^-1 = b^d), but the value the modexp actually consumes is
	// unpredictable to the caller, so its timing carries no information about D.
	var u, uInv *big.Int
	for {
		u, err = randInt(rng, priv.N)
		if err != nil {
			return nil, err
		}
		if u.Sign() == 0 {
			continue
		}
		uInv = new(big.Int).ModInverse(u, priv.N)
		if uInv != nil {
			break
		}
	}
	bb := new(big.Int).Mul(b, new(big.Int).Exp(u, e, priv.N))
	bb.Mod(bb, priv.N)
	s := new(big.Int).Exp(bb, priv.D, priv.N)
	s.Mul(s, uInv)
	s.Mod(s, priv.N)
	// Verify-after-sign. A fault in the modexp (or a corrupt key) must not leave the
	// issuer publishing a signature over a value it did not intend.
	if new(big.Int).Exp(s, e, priv.N).Cmp(b) != 0 {
		return nil, ErrSignFault
	}
	return s.Bytes(), nil
}

// canonicalRep decodes a big-endian RSA representative and requires it to be in
// [0, N-1] AND minimally encoded (no leading zero bytes). RFC 8017 §5.2.2 requires the
// range check; the minimality requirement is silt's, and it is what makes a signature
// or a blinded value have exactly ONE valid wire spelling — see SignBlinded (1).
func canonicalRep(b []byte, n *big.Int) (*big.Int, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("%w: empty", ErrNotCanonical)
	}
	if b[0] == 0 {
		return nil, fmt.Errorf("%w: leading zero byte", ErrNotCanonical)
	}
	v := new(big.Int).SetBytes(b)
	if v.Cmp(n) >= 0 {
		return nil, fmt.Errorf("%w: representative out of range", ErrNotCanonical)
	}
	return v, nil
}

// Unblind turns the issuer's blind signature into a signature on the plain serial,
// using the secret from Blind, and VERIFIES it before returning (publish domain).
//
// THE VERIFICATION IS AN RFC MUST (crypto-specialist advisory C-1, 2026-09-03).
// RFC 9474 §4.4 Finalize: "result = RSASSA-PSS-VERIFY(pk, msg, sig); if not valid,
// raise an invalid signature error and stop", so the client learns at WITHDRAWAL that
// the issuer misbehaved rather than at redemption. The composition that makes this
// more than conformance is in silt's own credit economics: a malicious issuer that
// returns a garbage blind signature charges the withdrawal fee, the fetch happens, the
// receipt is banked — and then Bank.Redeem fails at VerifyInWindow, so
// handleDeliveryReceipt never calls the ledger and the serve's eager unwitnessed
// self-mint is NEVER REVERSED. An issuer handing out duds drove its whole cohort onto
// the self-mint path at no cost to itself and with no detection. This closes the entry.
func Unblind(pub *rsa.PublicKey, serial, blindSig, secret []byte) ([]byte, error) {
	return unblindD(pub, serial, blindSig, secret, fdhDomain)
}

// UnblindCredit is Unblind for a PREPAID CREDIT (credit domain).
func UnblindCredit(pub *rsa.PublicKey, serial, blindSig, secret []byte) ([]byte, error) {
	return unblindD(pub, serial, blindSig, secret, creditDomain)
}

// UnblindDemand is Unblind for a RETRIEVAL TOKEN issued at epoch (demand domain).
// The epoch is part of the signed message, so verifying here also proves the issuer
// signed under key_epoch for the epoch we asked for.
func UnblindDemand(pub *rsa.PublicKey, epoch uint64, serial, blindSig, secret []byte) ([]byte, error) {
	return unblindD(pub, demandMsg(epoch, serial), blindSig, secret, demandDomain)
}

func unblindD(pub *rsa.PublicKey, msg, blindSig, secret []byte, domain string) ([]byte, error) {
	if err := validateShape(pub); err != nil {
		return nil, err // see blindD: a degenerate modulus must not reach the modexp
	}
	// The issuer's reply is untrusted input: range-check it like any other RSA
	// representative (RFC 8017 §5.2.2) before it reaches a modexp.
	bs, err := canonicalRep(blindSig, pub.N)
	if err != nil {
		return nil, err
	}
	r := new(big.Int).SetBytes(secret)
	rInv := new(big.Int).ModInverse(r, pub.N)
	if rInv == nil {
		return nil, ErrBadToken
	}
	sig := new(big.Int).Mul(bs, rInv)
	sig.Mod(sig, pub.N)
	out := sig.Bytes()
	// RFC 9474 §4.4 Finalize.
	if !verifyD(pub, msg, out, domain) {
		return nil, ErrBadToken
	}
	return out, nil
}

// Verify checks that sig is a valid issuer signature on serial: sig^e == FDH(serial).
func Verify(pub *rsa.PublicKey, serial, sig []byte) bool {
	return verifyD(pub, serial, sig, fdhDomain)
}

// VerifyCredit checks that sig is a valid issuer signature on a prepaid CREDIT
// serial (credit domain). A publish-token signature fails this check and vice
// versa, so the two are not interchangeable even under one key.
func VerifyCredit(pub *rsa.PublicKey, serial, sig []byte) bool {
	return verifyD(pub, serial, sig, creditDomain)
}

// VerifyDemand checks that sig is a valid issuer signature on a RETRIEVAL TOKEN
// serial ISSUED AT epoch (demand domain). A publish-token or credit signature fails
// this check and vice versa, so the three token kinds are not interchangeable under
// one key — and a token signed for a DIFFERENT epoch fails too, even under the very
// same key, which is the R0.4b (b1) coupling.
func VerifyDemand(pub *rsa.PublicKey, epoch uint64, serial, sig []byte) bool {
	return verifyD(pub, demandMsg(epoch, serial), sig, demandDomain)
}

func verifyD(pub *rsa.PublicKey, msg, sig []byte, domain string) bool {
	// A malformed key VERIFIES NOTHING (red-team re-break F4). N = 1 made the FDH image
	// 0 and s^e mod 1 == 0, so every (serial, sig) pair verified; E = 1 made the
	// signature the message itself. Both are refused here rather than answered "true".
	if validateShape(pub) != nil {
		return false
	}
	// RFC 8017 §5.2.2: the signature representative must be in [0, N-1], and silt
	// additionally requires it to be MINIMALLY encoded, so a valid signature has
	// exactly one wire spelling (advisory C-5). Without this, `sig` and `sig + N` and
	// any zero-padded spelling all verify — three distinct Token.Sig byte strings for
	// one signature, which is a free re-encoding of anything keyed on the raw bytes.
	s, err := canonicalRep(sig, pub.N)
	if err != nil {
		return false
	}
	m := fullDomainHashD(pub, msg, domain)
	check := new(big.Int).Exp(s, big.NewInt(int64(pub.E)), pub.N)
	return check.Cmp(m) == 0
}
