package demand

// R0.4b — the per-epoch issuer keyset and the token validity WINDOW.
//
// THE CONSTRUCTION (certified 2026-09-02, R0.4b-per-epoch-issuer-key-expiry, as
// amended by the red-team reconciliation verdict of the same date, §2.4 fix (b1)):
// the issuer holds one RSA key per epoch and blind-signs an epoch-E withdrawal under
// key_E OVER A MESSAGE THAT BINDS E. A token verifies under exactly the PAIR
// (key_E, E).
//
// THE EPOCH IS NOT A TOKEN FIELD AND NOT A RECEIPT FIELD — Token{Serial,Sig} and the
// signed receiptMsg are byte-identical to the pre-R0.4b shape, so the change adds
// ZERO new receipt quasi-identifier (cert Verdict 3; the epoch is a CONSENSUS epoch
// index, not wall-clock, so the R0.4 Q1 refutation of a wall-clock stamp does not
// reach it). At redemption the epoch is DISCOVERED by trying the held (key_e, e)
// pairs; the issuer keeps no serial↔epoch map, so serial→withdrawer stays blind.
//
// WHY THE EPOCH MUST BE IN THE SIGNED MESSAGE, not only in the key. The first
// R0.4b build put the epoch ONLY in the key. Then issuedEpoch(token) was a function
// of the token AND the verifier's current keyset: one RSA key bound to two epochs —
// which an ordinary RESTART produces, since the persisted key was re-registered for
// the new boot epoch, and which no validity rule forbids — made an old token verify
// under the NEWEST epoch that key is held for. Guard entries at the credit layer
// then expired while the tokens did not, and the cross-server double-redeem pump
// re-opened (red-team probes G and I, 2026-09-02). Binding E into the blind-signed
// message makes issuedEpoch a PURE FUNCTION OF THE TOKEN for ANY key schedule,
// which is the coupling condition "evicted ⇒ expired ⇒ un-redeemable" needs. The MOVE
// is Privacy Pass's — put the key's identity inside the signed message so a token
// cannot be re-dated (tenet B8, adopt the analogue's schema).
//
// WHAT SILT ACTUALLY SIGNS, STATED EXACTLY (crypto-specialist advisory C-4,
// 2026-09-03). RFC 9578 §5.3 signs token_input = 0x0002 ‖ nonce ‖ challenge_digest ‖
// token_key_id, where token_key_id = SHA256(DER SubjectPublicKeyInfo of pk_I) — a hash
// of the WHOLE issuer public key, identifying the issuer AND the key. silt signs an
// EPOCH INDEX (8-byte BE ‖ serial; see blindtoken.demandMsg), which identifies
// NEITHER. That is enough for the re-dating property, which is what R0.4b needed, and
// it is NOT the RFC 9578 binding. Do not describe it as one.
//
// THE GAP THIS LEAVES, named: validateIssuerKeys (core/chain/issuerkey.go) requires a
// verifying ed25519 self-signature, an in-range epoch and a bond — but NO
// proof-of-possession of the RSA key. Nothing stops bonded issuer B from registering
// issuer A's public-key fingerprint for epoch E (A's public key is served publicly);
// a redeemer resolving against B then pins A's key, and a token A signed verifies
// under B's keyset. That is Duplicate-Signature Key Selection (Blake-Wilson & Menezes
// 1999). It is NOT live today — handleDeliveryReceipt resolves ONE configured issuer
// and ledgers are per-node — so it is latent, and the close (an issuer-key
// proof-of-possession, and/or binding keyFingerprint into demandMsg) is a VALIDITY-RULE
// change: research-gated, owner-ratified, tracked as a ROADMAP Rock before the stamp
// raise. Not built here.
//
// ROTATION IS THEREFORE LIVENESS, NOT SOUNDNESS. Something must be committed for the
// current epoch or the lane cannot issue; but soundness holds even if the issuer
// re-registers one key forever. Fresh keys per epoch remain defence in depth (a
// discarded private key cannot backdate-sign) and are what the daemon schedules.
//
// EXPIRY IS THE HELD KEYSET, NOT AN ARITHMETIC CHECK. A verifier holds keys only
// for epochs in [current−W, current]. A token verifies iff its issuing epoch's key
// is still held; when the chain advances and key_{current−W−1} is dropped, every
// token from that epoch stops verifying AT ONCE. There is no per-token expiry field
// to forge or mis-read.
//
// WHY THIS EXISTS: it is the second half of the ratified (b)-prunable rule. It makes
// "evicted ⇒ expired ⇒ un-redeemable" TRUE at the credit layer, which is what closes
// the self-financing eviction pump a bounded FIFO paid-serial guard could not close
// (see core/credit/delivery.go's paidSerial note and the red-team eviction gates).
//
// THE MANDATORY COMPANION (cert Verdict 2): per-epoch keys manufacture a linkable
// tag — "which key verified you" — if a Byzantine issuer serves a distinct key_E to
// a small cohort. A pinned or published keyset does NOT close that (an issuer that
// equivocates keys equivocates the published list too). Soundness REQUIRES the
// redeemer to resolve key_E against a CONSENSUS-ATTESTED binding, which is why
// chain.IssuerKeyCommitment exists and why core/node pins this keyset against it.
// A Keyset assembled WITHOUT that cross-check is certified-unsafe for the neutral
// demand lane: per-epoch keys can then be worse for privacy than no epoch at all.
//
// The anonymity set coarsens to epoch width (cert residual R1) — the accepted,
// on-tenet price of any epoch-granular expiry, and the same coarse granularity
// pod.md:213 already commits demand at.

import (
	"crypto/rsa"
	"crypto/sha256"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

// DefaultWindow is W: how many epochs PAST its issuing epoch a demand token stays
// redeemable. A token issued in epoch E verifies while current − E <= W.
//
// EVOLVING-TIER KNOB (TENETS Part IX), DENOMINATED IN EPOCHS — never wall-clock.
// silt's block time is emergent (reputation-weighted quorum, no PoW target), so an
// epoch-denominated window auto-scales with the swarm's real cadence: when blocks
// are slow the network is stressed, which is exactly when redemption is slow too,
// and the window stretches WITH it. A wall-clock window would decouple from the very
// cadence that drives the latency it must cover (economist advisory §2), and a
// wall-clock stamp on the receipt is separately REFUTED as a new timing
// quasi-identifier (R0.4 cert Q1).
//
// THE VALUE IS ASSUMPTION-BASED, not measured (cert residual R0.4b-2). The floor is
// the honest serve→bank redemption-latency tail, which is UNMEASURED: the
// tail-setting path is abort-at-A / re-complete-at-B, whose token must still be
// in-window when B banks. 4 epochs (~32 blocks at DerivedEpochBlocks = 8) is the
// deliberately generous starting value — being too long costs a few MB of guard
// state, being too short costs an honest pony an unpaid delivery, and the ponies
// most likely to hit the tail are the small, distant, churny ones we federate work
// TO. The sim that replaces this assumption with a measurement (p99.9 of the
// serve→bank latency, in epochs) is named in the economist advisory §5 and is owed
// to the Tester. W and EpochBlocks are COUPLED knobs: re-check the tail before
// changing either.
const DefaultWindow = uint64(4)

// Keyset is a redeemer's WINDOW of per-epoch issuer public keys — the demand lane's
// analogue of a Privacy Pass key configuration. The held set IS the validity window:
// a token verifies iff some held key_E verifies it and E is in window.
//
// A Keyset is assembled by the redeemer from keys it fetched AND cross-checked
// against the committed E ↦ key_E binding (core/node). This type deliberately does
// NOT reach for the chain itself: it is the pure primitive, and keeping the
// consensus resolution at the caller is what lets core/demand stay free of a chain
// dependency — the same layering core/blindtoken keeps.
type Keyset struct {
	window uint64
	keys   map[uint64]*rsa.PublicKey
}

// NewKeyset returns an empty keyset with validity window w (in epochs). A zero w is
// legal and means "the issuing epoch only" — the tightest window, no late grace.
func NewKeyset(w uint64) *Keyset {
	return &Keyset{window: w, keys: map[uint64]*rsa.PublicKey{}}
}

// Window is W, the number of epochs past issuance a token stays redeemable.
func (k *Keyset) Window() uint64 { return k.window }

// Put binds pub as the issuer key for epoch. The caller is responsible for having
// resolved pub against the committed E ↦ key_E binding FIRST — an unchecked Put is
// exactly the targeted-key fingerprinting channel the cert gates on (see the package
// note above). A nil pub is ignored.
//
// A MALFORMED KEY IS ALSO IGNORED (red-team re-break F4, 2026-09-03). Resolving
// against the commitment proves WHICH BYTES the issuer serves; it proves nothing about
// whether those bytes are an RSA key. A held N = 0 panicked the verifier and a held
// N = 1 verified an arbitrary (serial, sig) pair — a universal forgery behind a
// perfectly valid consensus pin. blindtoken.ValidatePub is the single definition of
// well-formedness and this is the door it guards; Held reports what actually went in.
func (k *Keyset) Put(epoch uint64, pub *rsa.PublicKey) {
	if pub == nil || blindtoken.ValidatePub(pub) != nil {
		return
	}
	k.keys[epoch] = pub
}

// Key returns the held key for epoch, or nil.
func (k *Keyset) Key(epoch uint64) *rsa.PublicKey { return k.keys[epoch] }

// Len is how many epoch keys are held. Bounded by the caller to W+1 via Prune.
func (k *Keyset) Len() int { return len(k.keys) }

// Retain drops every held key for which keep reports false. It is how a redeemer
// makes its keyset FOLLOW THE CHAIN: the pin is a CACHE of the chain's committed
// E ↦ key_E binding, not an independent record, so after a reorg (or any adoption
// that re-points a commitment) a held key whose fingerprint no longer matches the
// CURRENT commitment must be dropped rather than kept and verified against
// (red-team probe H, 2026-09-02 — the redeemer otherwise accepts the abandoned
// fork's key and refuses the canonical one for W+1 epochs). Iteration order does not
// matter: keep is a pure predicate on one (epoch, key) pair.
func (k *Keyset) Retain(keep func(epoch uint64, pub *rsa.PublicKey) bool) {
	for e, pub := range k.keys {
		if !keep(e, pub) {
			delete(k.keys, e)
		}
	}
}

// Prune drops every key whose epoch has left the window at current — the operation
// that ENFORCES expiry. After Prune the held set is exactly {key_E : current−W <= E
// <= current} for the keys the caller supplied, so every token from a dropped epoch
// stops verifying at once. Keys for FUTURE epochs (E > current) are also dropped:
// holding one would accept a token the issuer cannot yet have signed honestly.
func (k *Keyset) Prune(current uint64) {
	for e := range k.keys {
		if e > current || current-e > k.window {
			delete(k.keys, e)
		}
	}
}

// VerifyInWindow reports the issuing epoch of t, if t verifies under some held
// (key_e, e) PAIR whose epoch is within the window at current. It tries at most W+1
// pairs, newest first (the common case is a token from the current or previous
// epoch, so the expected cost is one RSA verify, and the worst case is W+1 — 5 at
// W=4, each a single sub-millisecond modexp on the floor box).
//
// AT MOST ONE PAIR CAN MATCH, whatever keys are held: the epoch is inside the
// signed message, so a signature made for epoch E fails the check at every e != E
// even under the identical key (R0.4b (b1)). That is what makes the returned
// issuedEpoch a pure function of the token — the property the credit layer's expiry
// guard rests on.
//
// A token whose issuing epoch has left the window has NO held key that verifies it,
// so this returns ok=false and the caller rejects it as "token expired or not
// issued" — the rejection happens BEFORE any credit path, which is what makes the
// close purely subtractive (nothing is minted; see Bank.Redeem).
func (k *Keyset) VerifyInWindow(current uint64, t Token) (epoch uint64, ok bool) {
	if len(t.Serial) == 0 {
		return 0, false
	}
	e := current
	for {
		if pub := k.keys[e]; pub != nil && blindtoken.VerifyDemand(pub, e, t.Serial, t.Sig) {
			return e, true
		}
		if e == 0 || current-e >= k.window {
			return 0, false
		}
		e--
	}
}

// KeyFingerprint is the 32-byte commitment to an issuer public key: sha256 over the
// canonical wire encoding (blindtoken.MarshalPub). This is the value committed to
// consensus state as key_E's binding for epoch E, and the value a redeemer compares
// a fetched key against before it will hold that key. Committing the fingerprint
// rather than the key itself keeps the committed leaf a fixed 32 bytes — the same
// width every other value-carrying leaf uses — while binding the key exactly:
// finding a second key with the same fingerprint is a sha256 collision.
func KeyFingerprint(pub *rsa.PublicKey) ports.Hash {
	if pub == nil {
		return ports.Hash{}
	}
	return ports.Hash(sha256.Sum256(blindtoken.MarshalPub(pub)))
}
