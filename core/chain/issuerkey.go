package chain

// R0.4b — the CONSENSUS-ATTESTED per-epoch demand-issuer key binding (E ↦ key_E).
//
// WHY THIS IS IN CONSENSUS STATE AT ALL. The R0.4b expiry construction gives the
// demand token a validity window by putting the issuance epoch in the ISSUER KEY
// (core/demand/keyset.go). That buys expiry with no new token field — but it
// manufactures a linkability lever the field already knows: an issuer that serves a
// DISTINCT key_E to a small cohort turns "which key verified you" into a
// fingerprint, collapsing the epoch anonymity set. Privacy Pass names this failure
// mode; silt inherits it exactly.
//
// The lighter mitigation — "the redeemer pins the keyset it fetched and refuses a
// rotation it cannot cross-check" — is REFUTED as sufficient (research certification
// 2026-09-02, Verdict 2). Cross-check against WHAT? Pinning the key you fetched
// first detects a change over time for ONE redeemer, never a key that DIFFERS across
// redeemers; and an issuer willing to equivocate keys will equivocate its published
// key LIST too, serving list A to cohort A and list B to cohort B. There is no
// consensus anchor forcing one list. So soundness REQUIRES the redeemer to resolve
// key_E against state every honest node agrees on. That is this file.
//
// WHAT IS COMMITTED: (epoch, issuer) ↦ sha256(blindtoken.MarshalPub(key_E)), the
// 32-byte key fingerprint. A redeemer holds a fetched key_E only if its fingerprint
// EQUALS the committed one, so an off-commitment (targeted) key is rejectable by
// construction — the property TestIssuerKey_OffCommitmentKeyIsRefused pins.
//
// APPEND-ONLY IS THE ANTI-EQUIVOCATION PROPERTY — and only that. A commitment for
// (epoch, issuer) is written ONCE and never overwritten (first-write-wins, the same
// dedup discipline bondRootOwner ships). An issuer that could rewrite key_E after
// seeing who redeems under it would have the equivocation channel back. apply()
// therefore SKIPS a duplicate rather than replacing it, and a duplicate is not a
// validity error — a racing re-submission must not be able to wedge block production.
//
// WHAT APPEND-ONLY DOES NOT BUY (corrected 2026-09-02, red-team break 1). It does NOT
// make a token's issuing epoch well-defined. Nothing here forbids ONE fingerprint from
// being committed for SEVERAL epochs, and an ordinary restart does exactly that: the
// persisted key is re-installed for the new boot epoch. Under a key bound to two
// epochs, a token that carried its epoch only in the KEY could be re-dated to the
// newer one — guard entries expired while tokens did not, and the cross-server
// double-redeem pump re-opened. The close is at the token, not here: the issue epoch
// is inside the blind-signed message (core/blindtoken, the RFC 9578 token_key_id
// schema), so a token verifies under exactly one (key_E, E) pair.
//
// A DISTINCTNESS VALIDITY RULE IS REFUTED, not merely unbuilt (converged verdict
// §2.3). A rule rejecting a fingerprint already bound to another epoch could only see
// the RETAINED BAND — pruneIssuerKeyCommit narrows to [cur−W, cur+prePublish] at each
// registration-carrying block — so a few epochs on the old commitment is gone and the
// same fingerprint is registrable again, lengthening the pump's period rather than
// closing it.
// Remembering every fingerprint forever is unbounded committed state, which
// build-immutable #8 forbids. It would also convert the honest restart into a
// self-invalid proposal. Do not add it.
//
// BACKDATING IS REJECTED. A registration may name its OWN epoch or one up to W
// epochs AHEAD (a pre-published key SCHEDULE, which is what lets a token issued at
// an epoch boundary find its key already committed). It may NEVER name a past
// epoch: committing key_E after epoch E has run is precisely the "choose the key
// once I know who is watching" move.
//
// INERT TO CONSENSUS (the F1 STOP boundary, restated for this keyspace). No validity
// predicate, no quorum, no fork-choice, and no floor-box recompute READS this
// keyspace. It is committed so that honest nodes AGREE on it, not so that consensus
// depends on it. The only reader is the demand-lane redeemer, out of band.
//
// v5-ONLY (era-3 is FROZEN). The leaf is emitted only by stateRootLeavesV5 and a
// registration is valid only in a v5 block, so a v4 block's committed root stays
// byte-identical to era-3 (immutable F / #632). Without the version gate a v4 block
// could write committed state the era-3 leaf set does not cover — a silent
// divergence. See docs/thinking/2026-09-02-r0.4b-per-epoch-key-expiry-design.md §5.

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// issuerKeyRegDomain separates the registration signature from every other ed25519
// signature a validator makes, so a block signature or an attestation can never be
// replayed as a key registration.
const issuerKeyRegDomain = "silt/demand/issuerkey/v1"

var (
	// ErrIssuerKeyEra rejects a key registration in a pre-v5 block: the era-3 leaf
	// set does not commit this keyspace, so writing it under a v4 root would put
	// committed state outside the committed root.
	ErrIssuerKeyEra = errors.New("chain: demand-issuer key registration requires a v5 block (the era-3 leaf set does not commit it)")

	// ErrIssuerKeySig rejects a registration whose ed25519 signature does not verify
	// under its own declared public key, or whose key is not a well-formed ed25519
	// key. The record is self-verifying, like an Equivocation.
	ErrIssuerKeySig = errors.New("chain: demand-issuer key registration signature does not verify")

	// ErrIssuerKeyEpoch rejects a BACKDATED registration (an epoch already past) or
	// one further ahead than the pre-publication window allows.
	ErrIssuerKeyEpoch = errors.New("chain: demand-issuer key registration names an out-of-range epoch (backdated, or beyond the pre-publication window)")

	// ErrIssuerKeyUnbonded rejects a registration from an identity with no committed
	// bond while objective mode is on: without it the keyspace is unbounded by
	// anything Sybil-priced.
	ErrIssuerKeyUnbonded = errors.New("chain: demand-issuer key registration from an identity with no committed bond")
)

// IssuerKeyReg is a validator's SELF-VERIFYING commitment of its demand-token issuer
// public key for one epoch. It carries the signing public key rather than just the
// NodeID so the record verifies standalone (NodeID = sha256(pubkey), so the identity
// is derived, never asserted) — the same shape Equivocation uses.
//
// Additive on the wire: a block with no registrations omits the field entirely and
// hashes exactly as before.
type IssuerKeyReg struct {
	// Pub is the issuer's ed25519 IDENTITY public key. IssuerID() = sha256(Pub).
	Pub []byte `cbor:"1,keyasint"`
	// Epoch is the epoch this key signs demand withdrawals for.
	Epoch uint64 `cbor:"2,keyasint"`
	// Fingerprint is sha256(blindtoken.MarshalPub(key_Epoch)) — the RSA blind-signing
	// public key's commitment. See demand.KeyFingerprint.
	Fingerprint ports.Hash `cbor:"3,keyasint"`
	// Sig is Pub's ed25519 signature over issuerKeyRegMsg.
	Sig []byte `cbor:"4,keyasint"`
}

// IssuerID is the registering validator's NodeID, DERIVED from Pub (NodeID =
// sha256(pubkey)) so a record can never claim an identity it cannot sign for.
func (r IssuerKeyReg) IssuerID() ports.NodeID { return ports.HashBytes(r.Pub) }

// issuerKeyRegMsg is the signed bytes: the domain, the derived identity, the epoch,
// and the fingerprint. Every field that fixes WHO commits WHICH key for WHICH epoch
// is inside, so tampering any of them breaks the signature.
func issuerKeyRegMsg(id ports.NodeID, epoch uint64, fp ports.Hash) []byte {
	h := sha256.New()
	h.Write([]byte(issuerKeyRegDomain))
	h.Write(id[:])
	var e [8]byte
	for i := 0; i < 8; i++ {
		e[i] = byte(epoch >> (56 - 8*i)) // big-endian, matching the leaf key encoding
	}
	h.Write(e[:])
	h.Write(fp[:])
	return h.Sum(nil)
}

// SignIssuerKeyReg builds a signed registration binding key_epoch's fingerprint to
// priv's identity. The issuer side of the binding; the verifier side is
// VerifyIssuerKeyReg.
func SignIssuerKeyReg(priv ed25519.PrivateKey, epoch uint64, fp ports.Hash) IssuerKeyReg {
	pub := append([]byte(nil), priv.Public().(ed25519.PublicKey)...)
	r := IssuerKeyReg{Pub: pub, Epoch: epoch, Fingerprint: fp}
	r.Sig = ed25519.Sign(priv, issuerKeyRegMsg(r.IssuerID(), epoch, fp))
	return r
}

// VerifyIssuerKeyReg reports whether r is internally well-formed and correctly
// signed. It says nothing about epoch range or bonding — those are chain context and
// live in validateIssuerKeys.
func VerifyIssuerKeyReg(r IssuerKeyReg) bool {
	if len(r.Pub) != ed25519.PublicKeySize || len(r.Sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(r.Pub, issuerKeyRegMsg(r.IssuerID(), r.Epoch, r.Fingerprint), r.Sig)
}

// issuerKeyPrePublish is how many epochs AHEAD of the block's own epoch a
// registration may name. It matches demand.DefaultWindow (W): an issuer publishes
// its key schedule for the whole window it will sign in, so a token withdrawn right
// at an epoch boundary always finds key_E already committed. It is NOT imported from
// core/demand — package chain carries no dependency on the demand lane, and the two
// are coupled by the design doc and by TestIssuerKeyPrePublishMatchesDemandWindow.
const issuerKeyPrePublish = uint64(4)

// IssuerKeyPrePublish exports the window READ-ONLY for core/node's R2.11 arrival gate, which
// evaluates the same clause validateIssuerKeys will apply so a stale or too-far-ahead
// peer-submitted registration never occupies a queue slot.
const IssuerKeyPrePublish = issuerKeyPrePublish

// blockEpoch is the consensus epoch index a block at height h falls in:
// h / EpochBlocks. Deterministic and Byzantine-agreed — the same clock the relay
// lane's seen-map eviction uses. With epochs disabled (EpochBlocks == 0) every
// height is epoch 0, which makes the pre-publication window degenerate but harmless.
func (c *Chain) blockEpoch(h uint64) uint64 {
	if c.cfg.EpochBlocks == 0 {
		return 0
	}
	return h / c.cfg.EpochBlocks
}

// BlockEpoch is the consensus epoch index a block at height h falls in — the same
// clock validateIssuerKeys judges a registration's epoch against. Exported for the
// PROPOSER, which must know the epoch of the block it is about to build in order to
// drop a staged registration that has gone stale (see the pendingIssuerKeys fold in
// core/node/chainrole.go). Read-only; changes no rule.
func (c *Chain) BlockEpoch(h uint64) uint64 { return c.blockEpoch(h) }

// validateIssuerKeys checks a block's demand-issuer key registrations. Every clause
// is a REJECT, never a silent drop: a registration a validator cannot verify must
// not ride into committed state on another node's say-so.
func (c *Chain) validateIssuerKeys(b *Block) error {
	if len(b.IssuerKeys) == 0 {
		return nil
	}
	// Era gate: the era-3 leaf set does not commit this keyspace, so a v4 block
	// carrying registrations would write state its own committed root does not
	// cover. This is the rule that PROTECTS the era-3 byte-identical freeze.
	if b.Version < BlockVersionWitnessable {
		return fmt.Errorf("%w: block version %d", ErrIssuerKeyEra, b.Version)
	}
	cur := c.blockEpoch(b.Height)
	for i := range b.IssuerKeys {
		r := b.IssuerKeys[i]
		if !VerifyIssuerKeyReg(r) {
			return fmt.Errorf("%w (index %d)", ErrIssuerKeySig, i)
		}
		// No backdating, and no publishing further ahead than the window. Backdating
		// is the equivocation move (commit key_E once you know who redeemed under it);
		// unbounded look-ahead is an unbounded-state move.
		if r.Epoch < cur || r.Epoch > cur+issuerKeyPrePublish {
			return fmt.Errorf("%w: epoch %d, block epoch %d, pre-publish window %d",
				ErrIssuerKeyEpoch, r.Epoch, cur, issuerKeyPrePublish)
		}
		// In objective mode the keyspace is bounded to the Sybil-priced bonded set ×
		// the epoch band. Without MinBond there is no committed bond ledger to test
		// against, so the gate is inert (the same condition BondReg qualification uses).
		if c.cfg.MinBond > 0 && c.bonded[r.IssuerID()] <= 0 {
			return fmt.Errorf("%w: %x", ErrIssuerKeyUnbonded, r.IssuerID())
		}
	}
	return nil
}

// validateGenesisIssuerKeys is the genesis door for key registrations. AppendGenesis
// skips validateIssuerKeys (there is no prior history to derive an epoch from), and
// apply() writes whatever it is handed, so the door is checked here explicitly.
//
// Unlike Revocations and Slashes — both REJECTED at genesis because apply() would
// act on them unverified against ANOTHER identity (immutable #5 / retest G1) — a key
// registration is SELF-AUTHORIZING: IssuerID() is derived from Pub, and the signature
// must verify under that same Pub, so a genesis registration can only ever bind the
// registrant's OWN key. It cannot touch a third party. So it is admitted, but only
// under the same three rules the normal path enforces: a v5 block, a verifying
// signature, and an epoch inside the pre-publication window (genesis is epoch 0).
func (c *Chain) validateGenesisIssuerKeys(b *Block) error {
	if len(b.IssuerKeys) == 0 {
		return nil
	}
	if b.Version < BlockVersionWitnessable {
		return fmt.Errorf("%w: genesis version %d", ErrIssuerKeyEra, b.Version)
	}
	for i := range b.IssuerKeys {
		r := b.IssuerKeys[i]
		if !VerifyIssuerKeyReg(r) {
			return fmt.Errorf("%w (genesis index %d)", ErrIssuerKeySig, i)
		}
		if r.Epoch > issuerKeyPrePublish {
			return fmt.Errorf("%w: genesis epoch %d, pre-publish window %d",
				ErrIssuerKeyEpoch, r.Epoch, issuerKeyPrePublish)
		}
	}
	return nil
}

// applyIssuerKeys writes a block's registrations into the committed binding,
// FIRST-WRITE-WINS, then prunes epochs that have left the retention band.
//
// First-write-wins is the append-only property the anti-fingerprinting soundness
// rests on: once (epoch, issuer) is committed it is never replaced, so an issuer
// cannot re-point key_E after the fact. A duplicate is skipped silently — it is not
// an error, so a racing re-submission cannot wedge block production.
//
// PAYLOAD-DRIVEN, NOT HEIGHT-DRIVEN (red-team re-break F1, 2026-09-03). A block that
// carries NO registrations writes NOTHING in this keyspace — it does not even prune.
// This is a consensus rule, not an optimization: the floor box's O(payload) recompute
// derives its write-set from the block PAYLOAD and its scope gate stalls on
// len(b.IssuerKeys) > 0, so any committed write this keyspace makes on an empty
// payload is a write the box can neither fold nor name. It measured as a two-way
// consensus SPLIT at every epoch turn: the box AGREED with a forged root the full
// node rejects, and read an HONEST zero-registration block as a forged root
// (rt_r04b_c3_split_test.go). Keeping the prune here — inside the len>0 branch — is
// what makes the box's premise TRUE rather than merely asserted.
func (c *Chain) applyIssuerKeys(b Block) {
	if len(b.IssuerKeys) == 0 {
		return
	}
	if c.issuerKeyCommit == nil {
		c.issuerKeyCommit = map[uint64]map[ports.NodeID]ports.Hash{}
	}
	for i := range b.IssuerKeys {
		r := b.IssuerKeys[i]
		byIssuer := c.issuerKeyCommit[r.Epoch]
		if byIssuer == nil {
			byIssuer = map[ports.NodeID]ports.Hash{}
			c.issuerKeyCommit[r.Epoch] = byIssuer
		}
		if _, exists := byIssuer[r.IssuerID()]; exists {
			continue // append-only: never overwrite a committed key_E
		}
		byIssuer[r.IssuerID()] = r.Fingerprint
	}
	c.pruneIssuerKeyCommit(b.Height)
}

// pruneIssuerKeyCommit drops epochs that have left the retention band, keeping the
// committed keyspace BOUNDED (build-immutable #8). The band is
// [cur − W, cur + prePublish]: nothing older can be redeemed against (the demand
// keyset has already dropped that key, so a token from that epoch verifies under
// nothing), and nothing newer can be registered.
//
// CALLED ONLY FROM THE REGISTRATION-CARRYING BRANCH of applyIssuerKeys. That is the
// F1 close, and it costs nothing: every ADD is in-band by validity (validateIssuerKeys
// admits only cur <= e <= cur+prePublish), so pruning at each add re-establishes
// "at most 2W+2 epoch buckets" on every block that can grow the keyspace. A run of
// registration-free blocks cannot grow it, so skipping the prune there cannot break
// the bound — it only lets already-committed leaves outlive the band until the next
// registration sweeps them. Nothing reads the band: the demand keyset's own window
// (core/demand/keyset.go) decides what a token can verify against, and
// validateIssuerKeys already rejects a backdated epoch, so a lingering leaf is
// unreachable, not authoritative.
//
// The scan is over EPOCHS, not issuers — at most 2W+2 outer keys survive by
// construction — so this is O(W) per registration-carrying block, not O(state). It is
// a pure function of the block height and the pre-state, so every replica prunes
// identically (which is what keeps the root deterministic across replay and snapshot
// boot).
func (c *Chain) pruneIssuerKeyCommit(h uint64) {
	cur := c.blockEpoch(h)
	for e := range c.issuerKeyCommit {
		if e > cur+issuerKeyPrePublish || (cur > issuerKeyPrePublish && e+issuerKeyPrePublish < cur) {
			delete(c.issuerKeyCommit, e)
		}
	}
}

// IssuerKeyCommitment returns the committed key fingerprint for (issuer, epoch) and
// whether one is committed. This is the resolution a redeemer MUST perform before it
// will hold a fetched key_E: a key whose fingerprint does not equal this value is an
// off-commitment (targeted) key and is refused. Read-only; changes no rule.
func (c *Chain) IssuerKeyCommitment(issuer ports.NodeID, epoch uint64) (ports.Hash, bool) {
	fp, ok := c.issuerKeyCommit[epoch][issuer]
	return fp, ok
}

// IssuerKeyRegAdmissible reports whether a registration by issuer would clear the
// PRE-APPLY bonded gate of validateIssuerKeys at the current head. It mirrors that
// clause exactly and lives beside it so the two cannot drift.
//
// This exists for the PROPOSER, and it is POLICY, not validity. validateIssuerKeys
// reads `c.bonded` BEFORE the block applies, so a proposer that folds its own first
// BondReg and its own staged key registration into the SAME block fails its own local
// pre-check with ErrIssuerKeyUnbonded — and, because a staged registration rides and
// stays queued, re-fails on every later proposal. That is a permanent self-wedge for a
// fresh validator on a current-era objective network. Asking this before folding defers
// the registration to a block after the bond commits. Nothing about block VALIDITY
// changes: an attester still accepts a block carrying a registration this proposer
// chose to defer, so a mixed swarm cannot fork on it. Same shape as the IsSlashed
// filter on pending bond regs.
func (c *Chain) IssuerKeyRegAdmissible(issuer ports.NodeID) bool {
	return c.cfg.MinBond == 0 || c.bonded[issuer] > 0
}

// cloneIssuerKeyCommit deep-copies the committed binding so a dry-run apply mutates
// the copy, never the live chain (the #558 drift class).
func cloneIssuerKeyCommit(m map[uint64]map[ports.NodeID]ports.Hash) map[uint64]map[ports.NodeID]ports.Hash {
	if m == nil {
		return nil // preserve nil-vs-empty so the clone is DeepEqual to its source
	}
	out := make(map[uint64]map[ports.NodeID]ports.Hash, len(m))
	for e, byIssuer := range m {
		cp := make(map[ports.NodeID]ports.Hash, len(byIssuer))
		for id, fp := range byIssuer {
			cp[id] = fp
		}
		out[e] = cp
	}
	return out
}
