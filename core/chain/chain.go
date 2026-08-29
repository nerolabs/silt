// Package chain is the registry as an append-only block chain,
// maintained by the daemons themselves — the M12 replacement for the
// "single honest instance". The consensus is deliberately NOT
// proof-of-work: blocks commit by reputation-weighted quorum.
//
//   - A block is proposed by a node whose reputation clears
//     MinProposerRep, and commits only with attestations (Ed25519
//     signatures over the block hash) from at least Quorum DISTINCT
//     validators, each clearing MinAttesterRep, none of them the
//     proposer. No single node's say-so writes a block.
//   - Reputation is earned the M7/M9 way: passed storage audits and
//     bytes served (see credit.Reputation). A fresh identity starts at
//     zero and cannot propose or attest; and because NodeID is the hash
//     of the signing key (M10), reputation cannot be transplanted.
//   - Blocks carry registry entries only — root, manifest chunk
//     pointers, size. Manifests stay chunked and sealed off-chain, so
//     the chain stays small and content-blind.
//
// Every validator holds a full replica and validates everything: a
// block is accepted exactly when its hashes, signatures, reputations,
// quorum, and entries all check out against the local replica. Honest
// scope note: this is a quorum chain for a network with an honest
// validator majority, not a fork-choice consensus for adversarial
// partitions — there is no chain reorganization in v1; first valid
// block at a height wins.
package chain

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/core/publishtoken"
	"github.com/nerolabs/silt/core/translog"
	"github.com/nerolabs/silt/ports"
)

// Config sets the consensus thresholds. Zero MinRep values make a
// permissive chain (useful for small trusted deployments and demos);
// the sim exercises the strict settings.
type Config struct {
	MinProposerRep int64
	MinAttesterRep int64
	Quorum         int // attestations required (a FLOOR; see ByzantineQuorum), excluding the proposer
	// ByzantineQuorum sizes the required quorum at the Byzantine threshold
	// (H4 / Memo 05): in OBJECTIVE mode a commit's support set (the proposer plus
	// its attesters) is raised to a supermajority n−f of the qualified bonded set
	// of size N (f = ⌊(N-1)/3⌋), so any two support sets intersect in ≥ f+1 ≥ 1
	// honest validator — the classic quorum-intersection safety, which a FIXED
	// Quorum loses as the set grows (a fixed 3 among 30 validators no longer
	// guarantees two quorums share an honest node). Config.Quorum stays a floor (for
	// tiny trusted deployments); the effective attestation requirement is
	// max(Quorum, N−f−1). It only ever RAISES the bar, so it is safe to leave off
	// for legacy/trusted configs (default) and default-on for an untrusted objective
	// validator. See RequiredQuorum. No effect in legacy (reputation) mode, where the
	// qualified set size is a local, divergent view.
	ByzantineQuorum bool
	// AnchorWeight is the fixed fork-choice weight a zero-bond launch anchor carries
	// during the young window (#357 §1a). It gives the anchor→bonded ramp an
	// always-present, monotone weight signal so heavier() never has to decide a
	// zero-weight tie on the height-blind head-hash (which dropped committed blocks to
	// height 0 in the field). 0 = default to MinBond (see Chain.anchorWeight): an
	// anchor then weighs like a minimally-bonded validator, a real bond of any size
	// still outweighs it, and the weight vanishes at maturity (launchAnchor ⇒ false).
	// Mature-regime fork-choice weight (summed committed bond, the C2 quantity) is
	// unchanged, so this is C1/C2-neutral.
	AnchorWeight int64
	// Launch-window "training wheels" (risk 15): while the network is immature —
	// its NAKAMOTO COEFFICIENT over non-anchor bonded weight is below
	// MatureValidators — a commit ALSO needs AnchorQuorum attestations from Anchors
	// (a declared seed set), so a Sybil quorum can't capture a young network before
	// it decentralizes. The wheels shed MECHANICALLY on measured decentralization,
	// never a flag day, and — unlike a one-way latch — RE-ENGAGE if decentralization
	// later drops (the post-shed escape hatch, H4). Anchors are plural and require a
	// threshold, so no single anchor is load-bearing (cf. R4) — and while they can
	// gate publication on the young network (a transparent, on-chain, time-limited
	// power), they can never do so once mature. Empty Anchors / zero AnchorQuorum =
	// no training wheels (the default; trusted/sim deployments).
	Anchors      map[ports.NodeID]bool
	AnchorQuorum int
	// MatureValidators is the required NAKAMOTO COEFFICIENT (H4 / Memo 05): the
	// minimum number of DISTINCT non-anchor bonds whose combined bonded weight must
	// EXCEED the Byzantine fraction (⌊total/3⌋) of the non-anchor bonded set before
	// the training wheels shed. This is cost-to-corrupt over bond-distinct operators,
	// not a head-count: a network whose weight is dominated by one big bond (Nakamoto
	// 1) is NOT mature no matter how many satellite keys an operator adds, so one
	// operator cannot cheaply trip the wheels off. 0 = no threshold (always mature;
	// the default). (Residual, documented: a determined operator that splits its
	// stake into many EQUAL bonds still inflates the coefficient — the on-chain limit
	// is that stake concentration is invisible — but it pays the full cost-to-corrupt
	// and ByzantineQuorum still bounds it to ≤ ⅓ of weight for safety.)
	MatureValidators int
	// OperatorMargin is M, the conservative keys-per-operator inflation factor the
	// C2 concentration metric (C2Metric / Mature) discounts the bond-distinct
	// Nakamoto coefficient by: since on-chain data carries no operator label, one
	// operator may split its stake across up to ~M keys, so the true operator count
	// is bounded k* ≥ k̂/M and the maturity shed demands k̂ ≥ MatureValidators × M
	// distinct bonds. HEURISTIC by theorem (Kwon, D-C2): only as tight as M is
	// honest. 0/unset = 1 (no discount — legacy/sim/single-operator default,
	// behavior unchanged); a real untrusted deployment sets it > 1 in genesis.
	OperatorMargin int
	// AllowPublisher permits an entry to carry a durable Publisher NodeID.
	// It is FALSE by default because a Publisher→root record is permanent
	// on an append-only chain — the M0 privacy corner silently surrendered
	// in the historical record (F1/#14, #97). The unlinkable path (a
	// blind-signed publish token, or no identity at all) is the default;
	// only an explicitly trusted deployment opts back into Publisher
	// entries. Genesis is exempt (it seeds via AppendGenesis, and its
	// proposer is public by design).
	AllowPublisher bool
	// MinBond turns on OBJECTIVE fork-choice (D2 / red-team F6). When > 0 (and a
	// bond verifier is wired, SetBondVerifier), proposer/attester eligibility,
	// the quorum count, and the fork-choice WEIGHT are all decided by ON-CHAIN
	// bond registrations (Block.BondRegs) — a quantity every replica recomputes
	// identically from the blocks — instead of the local, per-node reputation
	// view. That is what stops two honest replicas with different audited sets
	// from computing different winners and forking permanently. A validator
	// qualifies iff its committed bonded size ≥ MinBond. Zero (default) keeps the
	// legacy reputation-gated path unchanged, so existing deployments and the
	// permissive/sim configs are unaffected.
	MinBond int64
	// MinBondBytes is the OBJECTIVE anti-release floor (retest G4), mirroring the
	// node-side floor the credit ledger already enforces (core/node MinBondBytes,
	// bondaudit): a bond below it earns NO objective standing, because a plot that
	// small can be released and re-plotted inside a challenge window, so its
	// one-time proof does not evidence sustained held space. Distinct from MinBond
	// (the admission size): a deployment can admit at MinBond yet still deny
	// fork-choice standing to sub-floor bonds. Zero (default) = no floor.
	MinBondBytes int64
	// BondTTLBlocks is the OBJECTIVE re-challenge cadence (retest G4): a bonded
	// validator's standing LAPSES this many blocks after the block that carried
	// its latest registration, unless it renews with a FRESH space-time proof
	// (a new BondReg, bound to a recent parent nonce). This enforces the "time"
	// half of proof-of-space-TIME on the objective path — a validator that
	// registers once and then RELEASES its plot cannot answer the fresh challenge
	// to renew, so its vote decays to nothing instead of persisting forever off a
	// single one-time proof. Decay is a deterministic function of block height, so
	// every replica expires standing in lockstep. Zero (default) = no expiry.
	BondTTLBlocks uint64
	// BondRegHeadWindow is how many recent committed heads a bond registration
	// stays valid over (factor ii of the MATURING drain wall): a reg is signed over
	// BondRegNonce(prev), and a strict single-head rule makes it go STALE the instant
	// the head advances — so over a real WAN, where a proposer proposes on head-advance
	// before the resubmission arrives, the drain starves (blocks commit empty below the
	// #286 byte cap; maturity never reaches bar-2 in-window). Accepting a reg over the
	// last K committed head nonces removes the staleness while keeping freshness bounded.
	// K is a C1 security parameter — a reg's space-time proof must still evidence RECENT
	// possession — and is safe because (1) it must be ≪ BondTTLBlocks (which already
	// time-boxes standing to a fresh proof) and (2) continuous bond-audit re-challenges
	// possession with a fresh nonce, catching a release-and-replay within one audit.
	// 0 = the DefaultBondRegHeadWindow. Set 1 to restore the strict single-head rule.
	// The exact bound vs the anti-release/reseal window is research-gated.
	BondRegHeadWindow int
	// Archive makes this node the ARCHIVAL tier (D-TIERING §3/§4, `-archive`):
	// it RETAINS every block's heavy space-time bond proof to genesis instead of
	// shedding it below the rolling retention horizon, so it can serve deep
	// history to a node that fell behind the swarm's prune horizon (the
	// ErrNeedCheckpoint / #559 true-loss case an archive is the answer to).
	//
	// It is a RETENTION choice ONLY — never a validity one. An archival node
	// validates by exactly the same rules as a pruning node (trustFloor and the
	// retention horizon are untouched), so it can never accept a block a pruning
	// peer would reject, and the tiers cannot fork against each other.
	//
	// Costs O(all history) resident heavy payload, which is exactly what
	// build-immutable #8 forbids on the 1 vCPU / 2 GB box — the tier model
	// exists so the edge node does NOT carry this. Off by default.
	Archive bool
	// EpochBlocks freezes the MATURE-phase validator set per epoch (#357 research
	// certification, Condition A): when > 0 in objective mode, the post-handoff
	// finality quorum (validatorSetSize / RequiredQuorum), attester/proposer
	// qualification, and attester fork-choice weight are read from a SNAPSHOT of the
	// committed bonded set taken at the last epoch-boundary block (height % EpochBlocks
	// == 0), never recomputed live from the churning bonded map. Finality is
	// quorum-INTERSECTION safety — two super-quorums are only guaranteed to share an
	// honest validator when both are taken over the SAME set — so bonds that join,
	// renew, or TTL-expire integrate only at the next rotation; the sole live mid-epoch
	// disqualification is a proven slash (shrink-only: N stays frozen, so it can only
	// raise the effective bar). The boundary block is itself super-quorum-final under
	// the §3 gate, so every rotation happens at a finalized checkpoint — and the
	// young→mature handoff (Condition B) is simply the FIRST mature rotation: the
	// anchors keep governing after the everMature latch trips mid-epoch, shedding at
	// the next boundary, so the change in what fork-choice weight MEANS is rooted at an
	// immutable base and can never reach back across it. Consensus-critical: every
	// validator in a swarm must run the same value (like MinBond/Anchors — genesis
	// config discipline). Keep it well below BondTTLBlocks: a frozen epoch extends a
	// mid-epoch-lapsed bond's vote by at most EpochBlocks. 0 (default) = no epochs:
	// the mature phase recomputes live (pre-Condition-A behavior; safe only for
	// trusted/sim deployments — the daemon defaults it ON for untrusted objective
	// validators).
	EpochBlocks uint64
	// RegGateActivationHeight is the PRE-LATCH activation override for the #506
	// reg-inclusion rate bound (the R-rule): when > 0, the rule is enforced on
	// every block of height > this, with no readiness signalling — the trusted
	// launch-anchor set coordinates its upgrade and declares the boundary as
	// genesis config, exactly like EpochBlocks/WSCheckpoint (research
	// certification 2026-08-22, Q1.5). 0 (default) = post-latch behavior: the
	// rule activates only when rule-aware bonded WEIGHT over a frozen epoch
	// crosses the >⅔ finality super-quorum (see rotateEpoch), which never fires
	// without epochs. Consensus-critical genesis config, same discipline as
	// MinBond/Anchors.
	RegGateActivationHeight uint64
	// WSCheckpoint is a WEAK-SUBJECTIVITY checkpoint (F-1): a recent trusted block
	// (height + hash) this replica refuses to reorg AT OR BEFORE, regardless of fork
	// weight. silt is weakly subjective — a node syncing from genesis (or long
	// offline) cannot distinguish the real matured chain from a forged long-range one
	// on chain data alone (Buterin WS; Gaži–Kiayias–Russell stake-bleeding) — so the
	// one-way maturity latch is only safe if a fresh node is ALSO pinned to a recent
	// trusted state out-of-band. Every production PoS chain does exactly this
	// (Ethereum checkpoint-sync, Cosmos ADR-044 unbonding, Casper finality); Bitcoin
	// removed its hardcoded checkpoints once cheaper protections existed. A reorg that
	// rewrites history at/before the checkpoint is rejected as a long-range attack.
	// The trusting window (how recent the checkpoint must be) is the weak-subjectivity
	// period, bounded by BondTTLBlocks + slashing depth (the eviction/unbonding
	// analogue). Zero Height = no checkpoint (genesis-trusting; safe only at launch,
	// on a trusted swarm, or before the network has matured). See docs/design/m0.md §10.
	WSCheckpoint WSCheckpoint
	// LivenessRecoveryHeight is the #535 fix (3) OPERATOR-DIRECTED liveness-floor
	// escape: an epoch-boundary height at which mature-epoch validation re-bases
	// proposer/attester qualification and the >⅔ weight quorum against the LIVE
	// qualified bonded set instead of the frozen epochSet — ONE boundary, then
	// the rotation the recovered block performs governs as normal. It exists for
	// a genuine loss of > ⅓ of the frozen epoch's weight that does NOT return
	// (members whose bonds TTL-lapsed and stayed gone): that state is outside
	// the BFT liveness model, the boundary stalls by design (safety-first), and
	// no automatic rule can make progress safe there — automatic re-basing was
	// refuted (fix (2), modelcheck_535_fix2_rebasing_test.go: excluding
	// possibly-honest lapsed weight raises the Byzantine fraction and reopens
	// I1). So the trust moves to a HUMAN: the operator confirms out-of-band that
	// the loss is a real outage, not a partition or an attack, and every honest
	// operator sets the SAME height — the weak-subjectivity trust class,
	// identical to agreeing on a WSCheckpoint, and the accepted residual (a
	// wrongly-invoked recovery can fork, exactly the fix (2) counterexample).
	// Consensus-critical coordination config, same discipline as WSCheckpoint /
	// RegGateActivationHeight. 0 (default) = off: a bled boundary stalls, which
	// is the certified-correct behavior. A non-boundary value never fires.
	LivenessRecoveryHeight uint64
	// Era3ActivationHeight is the PRE-LATCH activation override for the era-3
	// committed-state-root format (the v4 mint-flip + the v4-required boundary
	// rule): when > 0, era-3 is active on every block of height >= this, with no
	// readiness signalling — a trusted launch-anchor set coordinates its upgrade
	// and declares the boundary as genesis config, exactly like
	// RegGateActivationHeight (research cert Q5, mirroring the #506 pre-latch
	// override). 0 (default) = post-latch behavior: era-3 activates only when
	// era-3-aware bonded WEIGHT over a frozen epoch crosses the >⅔ finality
	// super-quorum (see rotateEpoch, the regVersion >= BlockVersionStateRoot
	// tally), which never fires without epochs. Consensus-critical genesis config,
	// same discipline as MinBond/Anchors/RegGateActivationHeight.
	//
	// NOTE the >= (not >) boundary: era-3 is a MINT/FORMAT boundary, so H_era3 is
	// itself the first v4 height ("at/above H_era3, a block MUST be v4"), unlike
	// the #506 R-rule's height > H_act (where the boundary block is the last
	// old-rules block). Monotonic height-gated activation keyed on finalized
	// history either way (2c deliberation).
	Era3ActivationHeight uint64
}

// WSCheckpoint is a recent trusted (height, hash) a replica will not reorg before.
// See Config.WSCheckpoint.
type WSCheckpoint struct {
	Height uint64
	Hash   ports.Hash
}

func DefaultConfig() Config {
	return Config{MinProposerRep: 100, MinAttesterRep: 100, Quorum: 3}
}

// BlockVersion is the schema/rule era a block is minted under. It is
// committed by Hash and checked at decode, so a block from one era can
// never be silently mis-validated under another era's rules — the
// hard-fork guard the chain needs BEFORE any change to what a block hash
// commits to or how a block validates (real-bond commitments, mandatory
// tokens; #98, prerequisite for #90/#91/#92). Additive field changes stay
// version-compatible via the keyasint tags (the Token addition proved
// this); a version bump is reserved for a change that would otherwise be a
// silent flag-day.
//
// v2 = BlockVersionRounds (#432, the rounds era): consensus signatures become
// two-phase and (height, round, phase)-scoped — Atts hold the PRECOMMIT quorum
// at CommitRound, PrepareQC holds the prepare quorum that justified it, and
// both sign the domain-separated v2 payload instead of the bare hash. A v1
// block keeps validating under v1 rules (era-gated in ValidateCommit /
// VerifyEquivocation), so committed history is never re-interpreted.
// BlockVersion (what the node MINTS) flipped to BlockVersionRounds once the
// propose path gathered two-phase (#432, merged b56f611) — the flip promised
// by the era-2 change, landed as its own behavior-neutral follow-up: the
// propose path already stamped BlockVersionRounds explicitly, so production
// minting is unchanged; only hand-built test blocks tracked this const.
const BlockVersion = BlockVersionRounds

// BlockVersionRounds is the #432 two-phase-rounds rule era.
const BlockVersionRounds = 2

// BlockVersionRegGate is the #506 reg-inclusion-rate-bound rule era. NOTE the
// deliberate deviation from the certification's rule-packaging candidate:
// blocks are NOT minted with this tag (BlockVersion stays BlockVersionRounds),
// because versionSupported on every pre-gate binary is an EXACT set — a v3-tagged
// block is rejected outright at decode, which is a hard fork, not the certified
// soft fork. The R-rule needs no schema change to enforce (it only REJECTS
// payloads), so enforcement keys on HEIGHT relative to the chain-derived
// activation boundary (regGateActive — exactly the certification's Q2 form:
// "apply the rule to every block of height > H_act"), and this constant's job
// is the READINESS threshold: a bond reg signalling Version ≥ BlockVersionRegGate
// counts its validator's frozen-epoch weight as rule-aware. This binary ACCEPTS
// v3-tagged blocks (validated under the ≥-rounds rules), so a future era that
// genuinely diverges the schema can flip minting without stranding it.
const BlockVersionRegGate = 3

// BlockVersionStateRoot is the era-3 committed-state-root rule era (the keystone).
// An era-3 block commits two attester-signed roots — StateRoot (an SMT over the 18
// committedSet validity fields, the original 16 plus the two era-3 activation scalars
// era3LockedIn/era3Height) and LogRoot (the RFC-6962 revocation-log MTH) — so
// every validating node that accepts the block can check the state it commits to.
//
// era-3 is a HARD fork, NOT a soft fork (research cert Q5/Q7): it adds a schema and,
// in step 2b, a validity predicate a pre-era-3 binary cannot evaluate. So era-3 MINTS
// a NEW version (4, not 3) and versionSupported is extended to <= 4 in the SAME
// release that adds the root fields — otherwise a pre-era-3 binary, which already
// decode-accepts v3 (v <= BlockVersionRegGate), would accept an era-3 block and never
// check its roots: a forged root would ride through unvalidated. Minting 3 is refuted
// for exactly that silent-mis-validation reason (RESEARCH-CERTIFICATION-2026-08-28 Q7).
//
// Build order (certified, choice 5): step 2a (THIS) adds the fields, folds them into
// Hash so attesters sign them, and widens versionSupported so a v4 block DECODES and is
// accepted. It does NOT flip minting (BlockVersion stays BlockVersionRounds) and does
// NOT add the root-matches-recomputed-state validity predicate — that is 2b. Minting v4
// is 2c, height-gated on a regVersion >= 4 supermajority, sequenced AFTER 2b so no v4
// block is ever minted before its predicate exists.
const BlockVersionStateRoot = 4

// BlockVersionWitnessable is the era-4 rule era: the two whole-map apply() scans
// (TTL-expiry and epoch rotation) become O(payload)-witnessable by committing two
// new accelerator keyspaces — a due-height bucket index (TTL) and a materialized
// `qualified` set with a frozen `epochStart` marker (rotation) — under the state
// root as v5-only leaves. The design is RATIFIED in
// docs/thinking/2026-08-29-era4-witnessable-transitions-options.md and the ordered
// build in docs/thinking/2026-08-29-era4-build-decomposition-options.md.
//
// Build order (ratified, PREDICATE-FIRST): 4a (THIS) mints this constant and defines
// the three new field tags, but does NOT lift versionSupported to <= 5, does NOT add
// the maintenance maps, and does NOT emit any new leaf — so 4a is INERT on the live
// v4 chain (no v4 block's committed root changes). 4b adds the maintenance spine and
// commits the new leaves as v5-only. 4c adds the v5 validity predicate on every
// disk-write path AND widens versionSupported to <= 5 in the SAME release (so the
// decode ceiling and the predicate ship atomically — no interim decode-without-check
// window). 4d height-gates activation and flips minting to v5.
const BlockVersionWitnessable = 5

// Consensus signature phases (#432 two-phase gather, research-certified).
// PhaseLegacy (0) is the era-1 bare-hash signature — what a pre-rounds
// Attestation decodes as; never minted in era 2.
const (
	PhaseLegacy    uint8 = 0
	PhasePrepare   uint8 = 1
	PhasePrecommit uint8 = 2
)

// Block is one link of the registry chain.
type Block struct {
	Height      uint64        `cbor:"1,keyasint"`
	Prev        ports.Hash    `cbor:"2,keyasint"`
	Entries     []ports.Entry `cbor:"3,keyasint"`
	Proposer    []byte        `cbor:"4,keyasint"` // Ed25519 public key
	ProposerSig []byte        `cbor:"5,keyasint,omitempty"`
	Atts        []Attestation `cbor:"6,keyasint,omitempty"`
	// Revocations are append-only takedown records: opaque roots that a
	// SUBSCRIBING node no-ops on (honoring is per-operator — see
	// node.SetHonorChainRevocations / ReplicaRegistry.HonorRevocations — not a
	// global switch). Each named root must already be committed on this chain
	// (ValidateProposal enforces existence), so a quorum cannot revoke content
	// it never published. Deletion is impossible on an immutable chain, so a
	// takedown is an ADDITION — a tombstone — that replicates and is
	// tamper-evident like any other block.
	Revocations []ports.Hash `cbor:"7,keyasint,omitempty"`
	// Version is the block's rule era (see BlockVersion). Committed by Hash
	// and required at decode; every minted block sets it.
	Version uint64 `cbor:"8,keyasint"`
	// Unrevocations reverse a prior takedown: each names a currently-revoked
	// root and clears it on commit (apply). This is the governance undo the
	// tenets require — takedown must not be a one-way, permanent asymmetry —
	// and, like a revocation, it is quorum-gated and replicated.
	Unrevocations []ports.Hash `cbor:"9,keyasint,omitempty"`
	// BondRegs are on-chain PoST-bond registrations that make fork-choice
	// OBJECTIVE (D2 / red-team F6): each records a validator's bonded size with a
	// fresh space-time proof any replica re-verifies, so "who is a qualified
	// validator, and how heavy is their attestation" is a function of the chain,
	// not of the local reputation view. Only meaningful when Config.MinBond > 0;
	// omitempty keeps this additive (a block with no registrations hashes exactly
	// as before, so no BlockVersion bump). Committed by Hash so attesters sign
	// over them.
	BondRegs []BondReg `cbor:"10,keyasint,omitempty"`
	// Slashes are on-chain equivocation records (red-team F2): a self-verifying
	// proof that a validator double-signed. On commit, the culprit is EVICTED from
	// the objective bonded set (its `c.bonded` weight → 0) and permanently barred
	// from re-earning it — so a proven double-sign costs standing in objective
	// mode, not only in the reputation ledger the objective set never reads.
	// Committed by Hash (attesters sign over them); omitempty keeps it additive.
	Slashes []Equivocation `cbor:"11,keyasint,omitempty"`
	// CommitRound is the round this block committed at (#432 rounds era). The
	// round is NOT part of the value's identity — the SAME block re-proposed at
	// a higher round after a view-change must hash identically (Tendermint
	// separates the vote's (h, r, block-id) from the block for exactly this) —
	// so CommitRound and the certificates below are set on the COMMITTED copy,
	// after Hash-identity has done its work, and are excluded from Hash (see
	// Hash). 0 for era-1 blocks and for round-0 commits.
	CommitRound uint64 `cbor:"12,keyasint,omitempty"`
	// PrepareQC is the prepare-phase quorum certificate that justified the
	// precommit phase (#432): the quorum of PhasePrepare attestations at
	// (Height, CommitRound). ValidateCommit (era 2) requires it at the SAME
	// thresholds as the commit itself — ⌊A/2⌋+1 anchors in launch, >⅔ frozen
	// epoch weight in mature — because the POL threshold IS the commit
	// threshold (certification §4). Excluded from Hash like Atts.
	PrepareQC []Attestation `cbor:"13,keyasint,omitempty"`
	// Pruned, when set, is the block's pre-prune Hash. A payload-selectively pruned
	// block (heavy BondReg.Answer proofs dropped below the retention horizon — the H2
	// OOM fix) can no longer recompute its own hash because Hash commits BondRegs, so
	// it carries the hash it had when full. EXCLUDED from Hash (like the QCs): a full
	// block never sets it and hashes exactly as before (no BlockVersion bump, additive).
	// Hash() returns it for a pruned block. The value is trusted ONLY when authenticated
	// by chain Prev-linkage to the node's own trusted anchor, and a pruned block is
	// accepted ONLY strictly below that finalized anchor (the Q2 gate in Reconcile) —
	// never on this field alone. See retention.go + docs/thinking/2026-08-18-serve-retain-from-checkpoint-oom-fix.md.
	Pruned ports.Hash `cbor:"14,keyasint,omitempty"`

	// StateRoot and LogRoot are the era-3 committed roots (the keystone, #603/#597).
	// StateRoot is the SMT over the 18 committedSet validity fields (StateRoot());
	// LogRoot is the RFC-6962 MTH over the revocation transparency log (LogRoot() =
	// RevocationLogRoot()). BOTH are folded into Hash below — unlike the QCs and
	// CommitRound, attesters SIGN them, so a forged root cannot ride a valid signature.
	// This is the Ethereum stateRoot/receiptsRoot shape: two committed roots of two
	// kinds (an order-invariant authenticated map + an append-only log, #597), one
	// signature covering both.
	//
	// COMPAT (step 2a — the load-bearing decision, see
	// docs/thinking/2026-08-29-era3-step2a-commit-roots-schema.md): these are POINTERS so
	// omitempty keeps the change INVISIBLE to era-2. An era-2 block leaves both nil, and
	// omitempty omits a nil pointer — the unsigned body is byte-identical to pre-2a and
	// Hash() is unchanged (committed history is never re-interpreted, chain.go:260-268).
	// A plain ports.Hash ([32]byte) would NOT work: omitempty never omits a fixed-size
	// ARRAY (it is never "empty"), so a zero [32]byte would be emitted as 32 zero bytes
	// and change every era-2 hash. The byte-identity oracle caught exactly that; the
	// pointer is the fix that keeps the fixed-32-byte type AND omits cleanly. An era-3
	// block sets both to the non-zero committed roots (empty-state SMT / sha256("") log —
	// fixed non-zero constants), so they are emitted and signed. nil means unambiguously
	// absent; a set pointer means present-with-this-value. 2a adds NO predicate that
	// rejects a v4 block with a nil root — that belongs to 2b.
	StateRoot *ports.Hash `cbor:"15,keyasint,omitempty"`
	LogRoot   *ports.Hash `cbor:"16,keyasint,omitempty"`

	// hashMemo caches Hash() (#555). A block's hashed content is immutable once
	// minted (Sign computes the hash it signs) or decoded, but Hash() re-marshaled
	// the whole body — BondRegs' ~1.5 MB proofs included — and re-hashed it on
	// EVERY call. blockByHash recomputes per scan step and recentBondRegNonces does
	// up to K=8 such lookups per validated block, so a deep-chain Reconcile paid
	// O(depth × K × scan) full-body hashes on the node thread: the 16–86 s
	// ChainReply stalls that saturated the event loop, stretched the sweep timers,
	// and starved the two-phase gather in the 95d39e8-deep field run. Unexported:
	// never on the wire (cbor skips it), zero on decode, travels with value copies.
	// Sign invalidates it (the one place hashed content mutates after a possible
	// Hash call); everything else constructs before hashing.
	hashMemo    ports.Hash
	hashMemoSet bool
}

// Attestation is a validator's consensus signature over a block. The public
// key rides along because a NodeID (its hash) can't be inverted.
//
// Era 1 (Round=0, Phase=PhaseLegacy): Sig is over the bare block hash.
// Era 2 (#432 rounds): Sig is over the domain-separated payload
// consensusSigBytes(phase, round, hash) — so a prepare can never be replayed
// as a precommit, and a signature at one round can never complete a quorum at
// another (the delayed-quorum schedule S1). Round/Phase are cbor-additive:
// a legacy attestation decodes as (0, PhaseLegacy) and hashes identically.
type Attestation struct {
	PubKey []byte `cbor:"1,keyasint"`
	Sig    []byte `cbor:"2,keyasint"`
	Round  uint64 `cbor:"3,keyasint,omitempty"`
	Phase  uint8  `cbor:"4,keyasint,omitempty"`
}

// BondReg registers (or renews) a validator's on-chain PoST bond for objective
// fork-choice (F6). Validator is the ed25519 public key; Root/Size are the bond
// commitment; Answer is a CBOR-encoded bond space-time answer for the fresh
// nonce derived from the block's parent (BondRegNonce), so it cannot be replayed
// to another height or fork; Sig is the validator's signature binding the claim
// to its identity. A non-genesis registration is accepted only if Sig verifies
// and the injected bond verifier accepts (Root, Size, nonce, Answer) — i.e. the
// validator PROVED it holds the bond NOW. Genesis registrations are declared
// (like the genesis block itself), seeding the launch validator set.
type BondReg struct {
	Validator []byte     `cbor:"1,keyasint"`
	Root      ports.Hash `cbor:"2,keyasint"`
	Size      int64      `cbor:"3,keyasint"`
	Answer    []byte     `cbor:"4,keyasint,omitempty"`
	Sig       []byte     `cbor:"5,keyasint,omitempty"`
	// Domain is the validator's committed failure-domain label (A axis, D-C2): a
	// self-declared AS/rack/geo hash (the same domainID gossiped for DHT diversity,
	// H5-B), now COMMITTED in the bond so the concentration metric can count
	// address-diverse participants deterministically (C2Metric NakamotoDomains).
	// Signed (see signingBytes) so it binds to the validator. 0 = unset (treated as
	// independent — behavior identical to pre-A-axis chains). HONESTLY WEAK, and
	// self-asserted: a declared domain is gossiped and trusted VERBATIM (core/node
	// learns peerDomains straight off the wire with NO /24 cross-check) — it is NOT
	// transport-verified against the peer's observed address. So it PRICES a single-
	// /24 split (equal-domain bonds aggregate into one group) but a splitter that
	// simply DECLARES distinct domains gets distinct groups for free; it does not
	// CLOSE the honest-whale residue (Kwon — m0.md §10, #182). The composition does
	// not rely on any cross-check: the shed gates on min(NakamotoOperators,
	// NakamotoDomains), so free domains can only LOWER the min, never trip the wheels
	// off early (verified — see the colluding-validator red-team, seam-5).
	Domain uint64 `cbor:"6,keyasint,omitempty"`
	// Version is the highest block-rule era this validator's software validates —
	// the #506 readiness signal. It rides the bond reg (not the attestation)
	// because a reg is hash-covered (Block.Hash commits BondRegs; Atts are NOT
	// committed and are strippable by any re-serving peer), validator-signed
	// (signingBytes, conditionally — the Domain idiom), renewed every ≤ TTL/2,
	// and held by every frozen-epoch member by definition — so rule-aware WEIGHT
	// is countable identically by every replica from committed history alone.
	// 0 (absent — a pre-gate binary's reg) reads as NOT rule-aware, the safe
	// default. Kept by Prune (a light field, like Domain).
	Version uint8 `cbor:"7,keyasint,omitempty"`
}

// ValidatorID is the NodeID (hash of the public key) that a registration bonds.
func (r BondReg) ValidatorID() ports.NodeID { return sha256.Sum256(r.Validator) }

// signingBytes is the message a validator signs to claim a registration: the
// root, size, and fresh nonce, domain-separated. Binding the nonce stops a
// signature made for one position being replayed at another.
func (r BondReg) signingBytes(nonce uint64) []byte {
	b := make([]byte, 0, 32+len(r.Root)+8+8)
	b = append(b, []byte("silt/chain/bondreg/v1")...)
	b = append(b, r.Root[:]...)
	var sz [16]byte
	binary.BigEndian.PutUint64(sz[:8], uint64(r.Size))
	binary.BigEndian.PutUint64(sz[8:], nonce)
	b = append(b, sz[:]...)
	// Bind the committed domain (A axis) — but ONLY when set, so a domain-0 bond
	// signs the exact pre-A-axis message and every existing/genesis signature still
	// verifies (backward-compatible; no BlockVersion bump needed).
	if r.Domain != 0 {
		var d [8]byte
		binary.BigEndian.PutUint64(d[:], r.Domain)
		b = append(b, []byte("silt/chain/bondreg/domain/v1")...)
		b = append(b, d[:]...)
	}
	// Bind the #506 readiness signal the same conditional way: a version-less reg
	// signs the exact pre-gate message (existing signatures verify unchanged), and
	// a signalling reg's Version cannot be flipped without breaking Sig.
	if r.Version != 0 {
		b = append(b, []byte("silt/chain/bondreg/version/v1")...)
		b = append(b, r.Version)
	}
	return b
}

var encMode cbor.EncMode

// encModeBuf is encMode's buffer-reusing twin (identical CanonicalEncOptions,
// so identical bytes — asserted by TestHashPooledBufferIdentity_563). Hash()
// marshals a block's FULL body to hash it; on the cold-sync Reconcile path that
// is one multi-MB transient per decoded block, and the resulting O(fork-bytes)
// garbage burst is what crossed the 2 GB box's envelope in the field (#563).
// MarshalToBuffer into a pooled buffer keeps the churn at ~one resident buffer.
var encModeBuf cbor.UserBufferEncMode

var hashBufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func init() {
	var err error
	encMode, err = cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		panic(err)
	}
	encModeBuf, err = cbor.CanonicalEncOptions().UserBufferEncMode()
	if err != nil {
		panic(err)
	}
}

// Hash covers everything except signatures: height, ancestry, entries,
// proposer, and both takedown and undo records. Signing the hash therefore
// signs the block's entire content and its place in history.
func (b *Block) Hash() ports.Hash {
	// A pruned block's heavy body (BondReg.Answer) is gone, so its hash cannot be
	// recomputed — it carries the pre-prune value. Authenticity is established by
	// chain Prev-linkage to the trusted anchor + the Q2 gate (Reconcile), never by
	// this field alone; a forged Pruned fails the proposer/attester signature check
	// (sigs are over the real hash) and the decode invariant (Pruned ⟹ no Answer).
	if b.IsPruned() {
		return b.Pruned
	}
	if b.hashMemoSet {
		return b.hashMemo
	}
	// StateRoot/LogRoot are folded in so attesters sign the era-3 committed roots. For
	// an era-2 block both are zero and omitempty omits them, so the marshalled body — and
	// thus the hash — is byte-identical to pre-2a (the compat property, see the field doc).
	unsigned := Block{Version: b.Version, Height: b.Height, Prev: b.Prev, Entries: b.Entries, Proposer: b.Proposer, Revocations: b.Revocations, Unrevocations: b.Unrevocations, BondRegs: b.BondRegs, Slashes: b.Slashes, StateRoot: b.StateRoot, LogRoot: b.LogRoot}
	buf := hashBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	if err := encModeBuf.MarshalToBuffer(&unsigned, buf); err != nil {
		panic(err) // canonical encoding of our own struct cannot fail
	}
	blockHashComputes.Add(1)
	b.hashMemo = sha256.Sum256(buf.Bytes())
	hashBufPool.Put(buf)
	b.hashMemoSet = true
	return b.hashMemo
}

// blockHashComputes counts actual (non-memoized) Hash computations — the
// deterministic oracle for the #555 hash-work bound (a wall-clock assertion
// would be flaky; the WORK count is what the memo bounds). Atomic only so a
// concurrent test cannot trip the race detector; the node loop is
// single-threaded.
var blockHashComputes atomic.Uint64

// IsPruned reports whether this block has been payload-selectively pruned — its heavy
// BondReg.Answer proofs dropped and its pre-prune Hash stored in Pruned.
func (b *Block) IsPruned() bool { return b.Pruned != (ports.Hash{}) }

// Prune returns a payload-selective pruned copy of a FULL block: the heavy space-time
// proofs (BondReg.Answer, ~1.5 MB each) are dropped and the pre-prune hash is stored,
// so the block still hash-links and still carries its consensus signatures (unbounded
// late-reveal slashing evidence) while shedding what OOMs a small box (build-immutable
// #8). The header, signatures, and the light BondReg fields the STATE/slashing paths
// read (Validator/Root/Size/Sig/Domain) are kept. Call ONLY on a finalized block
// strictly below the retention horizon — the caller enforces that gate. Idempotent.
func (b Block) Prune() Block {
	if b.IsPruned() {
		return b
	}
	h := b.Hash() // over the FULL body, before dropping anything
	out := b
	if len(b.BondRegs) > 0 {
		out.BondRegs = make([]BondReg, len(b.BondRegs))
		for i, r := range b.BondRegs {
			r.Answer = nil // drop the heavy proof; keep the light fields
			out.BondRegs[i] = r
		}
	}
	out.Pruned = h
	return out
}

// hasHeavyBondProof reports whether any of the block's bond registrations still carries
// its heavy space-time Answer (the ~1.5 MB payload the retention prune sheds) — so an
// entry-only or already-pruned block is skipped rather than needlessly marked pruned.
func (b *Block) hasHeavyBondProof() bool {
	for i := range b.BondRegs {
		if b.BondRegs[i].Answer != nil {
			return true
		}
	}
	return false
}

// ProposerID is the proposer's NodeID: the hash of its key (M10).
func (b *Block) ProposerID() ports.NodeID { return sha256.Sum256(b.Proposer) }

func (a Attestation) AttesterID() ports.NodeID { return sha256.Sum256(a.PubKey) }

// Sign fills in the proposer key and signature. Setting Proposer mutates
// hashed content, so the memo is invalidated first — Sign must never sign a
// stale hash (#555).
func Sign(b *Block, priv ed25519.PrivateKey) {
	b.Proposer = append([]byte(nil), priv.Public().(ed25519.PublicKey)...)
	b.hashMemoSet = false
	h := b.Hash()
	b.ProposerSig = ed25519.Sign(priv, h[:])
}

// Attest produces a validator's era-1 (legacy, bare-hash) attestation for b.
// Era-2 consensus uses AttestAt with an explicit round and phase.
func Attest(b *Block, priv ed25519.PrivateKey) Attestation {
	h := b.Hash()
	return Attestation{
		PubKey: append([]byte(nil), priv.Public().(ed25519.PublicKey)...),
		Sig:    ed25519.Sign(priv, h[:]),
	}
}

// consensusSigBytes is the era-2 signed payload: domain tag ‖ phase ‖ round ‖
// block hash. The phase byte means a prepare can never be replayed as a
// precommit; the round means a signature at one round can never complete a
// quorum at another (schedule S1); the height rides inside the hash. The
// domain tag keeps these signatures disjoint from every other signature an
// identity key ever makes.
func consensusSigBytes(phase uint8, round uint64, h ports.Hash) []byte {
	buf := make([]byte, 0, len(consensusSigDomain)+1+8+len(h))
	buf = append(buf, consensusSigDomain...)
	buf = append(buf, phase)
	var r [8]byte
	binary.LittleEndian.PutUint64(r[:], round)
	buf = append(buf, r[:]...)
	buf = append(buf, h[:]...)
	return buf
}

const consensusSigDomain = "silt/consensus/v2\x00"

// AttestAt produces a validator's era-2 phase/round-scoped consensus signature
// for b (#432 two-phase gather).
func AttestAt(b *Block, priv ed25519.PrivateKey, round uint64, phase uint8) Attestation {
	return Attestation{
		PubKey: append([]byte(nil), priv.Public().(ed25519.PublicKey)...),
		Sig:    ed25519.Sign(priv, consensusSigBytes(phase, round, b.Hash())),
		Round:  round,
		Phase:  phase,
	}
}

// verifyAtt verifies one attestation against h under its own declared era:
// PhaseLegacy verifies the bare hash; PhasePrepare/PhasePrecommit verify the
// era-2 payload at the attestation's declared (round, phase). The caller
// enforces WHICH phase/round it will accept — this only answers "is the
// signature genuine for what it claims to be".
func verifyAtt(a Attestation, h ports.Hash) bool {
	if len(a.PubKey) != ed25519.PublicKeySize {
		return false
	}
	switch a.Phase {
	case PhaseLegacy:
		return a.Round == 0 && ed25519.Verify(ed25519.PublicKey(a.PubKey), h[:], a.Sig)
	case PhasePrepare, PhasePrecommit:
		return ed25519.Verify(ed25519.PublicKey(a.PubKey), consensusSigBytes(a.Phase, a.Round, h), a.Sig)
	default:
		return false
	}
}

func Encode(b *Block) []byte {
	raw, err := encMode.Marshal(b)
	if err != nil {
		panic(err)
	}
	return raw
}

// versionSupported: v1 (legacy single-phase), v2 (#432 rounds), v3 (#506 reg-gate
// readiness tag), and v4 (era-3 committed state root) all decode; each validates under
// ITS OWN era's rules (era-gated in ValidateCommit / VerifyEquivocation) — committed
// history is never re-interpreted. The ceiling is BlockVersionStateRoot: a v4 block is
// accepted at decode (step 2a), and its root-matches-recomputed-state predicate lands
// in step 2b. A version BEYOND 4 is refused loudly with ErrBlockVersion — the hard-fork
// guard: a block from a not-yet-known era is never silently mis-validated.
func versionSupported(v uint64) bool { return v >= 1 && v <= BlockVersionStateRoot }

func Decode(raw []byte) (*Block, error) {
	var b Block
	if err := cbor.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("chain: decode block: %w", err)
	}
	if !versionSupported(b.Version) {
		return nil, fmt.Errorf("%w: got %d, want 1..%d", ErrBlockVersion, b.Version, BlockVersionStateRoot)
	}
	return &b, nil
}

func EncodeBlocks(bs []Block) []byte {
	raw, err := encMode.Marshal(bs)
	if err != nil {
		panic(err)
	}
	return raw
}

// EncodeBlocksUpTo encodes the longest PREFIX of bs whose CBOR encoding fits within
// maxBytes, always at least ONE block (a lone oversized block still moves — never
// stall). It sizes blocks one at a time and stops early, so it never marshals more
// than the returned window — the point is to bound the chain-SERVE buffer (a node
// serving its whole chain into one buffer was the 144 MB `EncodeBlocks` OOM driver,
// docs/thinking/2026-08-18-oom-is-multi-driver-chain-serve.md), NOT to marshal the
// full chain. A syncing peer requests successive windows (Height:0, then last+1, …)
// until it reaches the server's head; Reconcile validates the reassembled linkage,
// so a windowed fetch cannot corrupt the chain. maxBytes <= 0 means unbounded (the
// legacy whole-chain encode, for callers/tests that want it).
func EncodeBlocksUpTo(bs []Block, maxBytes int) []byte {
	if maxBytes <= 0 || len(bs) == 0 {
		return EncodeBlocks(bs)
	}
	total := 3 // approx CBOR array-header framing
	k := 0
	for k < len(bs) {
		one, err := encMode.Marshal(bs[k])
		if err != nil {
			panic(err)
		}
		if k > 0 && total+len(one) > maxBytes {
			break // adding this block would exceed the window; send [0,k)
		}
		total += len(one)
		k++
	}
	return EncodeBlocks(bs[:k])
}

func DecodeBlocks(raw []byte) ([]Block, error) {
	var bs []Block
	if err := cbor.Unmarshal(raw, &bs); err != nil {
		return nil, fmt.Errorf("chain: decode blocks: %w", err)
	}
	for i := range bs {
		if !versionSupported(bs[i].Version) {
			return nil, fmt.Errorf("%w: block %d got %d, want 1..%d", ErrBlockVersion, i, bs[i].Version, BlockVersionStateRoot)
		}
	}
	return bs, nil
}

var (
	ErrLowReputation      = errors.New("chain: reputation below threshold")
	ErrNoQuorum           = errors.New("chain: insufficient valid attestations")
	ErrNoQuorumWeight     = errors.New("chain: mature-epoch commit lacks the frozen-weight super-majority (>⅔ of epoch bonded weight — B2, research certification 2026-08-13)")
	ErrRegGate            = errors.New("chain: bond registration violates the active reg-inclusion rate bound (#506 R-rule — slashed identity, or re-registered within R of its last committed reg)")
	ErrBadSignature       = errors.New("chain: bad signature")
	ErrWrongParent        = errors.New("chain: block does not extend the local head")
	ErrDupRoot            = errors.New("chain: root already registered")
	ErrUseConsensus       = errors.New("chain: replica is read-only; entries are committed via consensus")
	ErrAnchorRequired     = errors.New("chain: immature network requires anchor attestations (training wheels)")
	ErrDeMatureQuorum     = errors.New("chain: de-matured network requires a real-bond super-quorum (≥⅔ of live bonded weight)")
	ErrPreCheckpointReorg = errors.New("chain: fork rewrites history at or before the weak-subjectivity checkpoint (long-range reorg refused)")
	ErrPreFinalityReorg   = errors.New("chain: fork would revert a finalized (super-quorum-committed) objective block (BFT finality gate — reorg refused; prefer stall to reorg, D-1, #357 §3)")
	ErrTokenRequired      = errors.New("chain: entry has no publish token (required)")
	ErrTokenSpent         = errors.New("chain: publish token serial already spent (double-spend)")
	ErrBlockVersion       = errors.New("chain: unsupported block version")
	ErrProposerPrepare    = errors.New("chain: era-2 commit lacks the proposer's round-scoped prepare (the authorship vote that makes a double-proposal attributable — #432/I5)")
	ErrPublisherEntry     = errors.New("chain: entry carries a durable Publisher (records permanent linkage; publish unlinkably or run an explicitly trusted deployment)")
	ErrEmptyFork          = errors.New("chain: cannot reconcile an empty fork")
	ErrNoGenesis          = errors.New("chain: local replica has no genesis to anchor a reconcile")
	ErrForeignGenesis     = errors.New("chain: fork does not share our genesis (refusing to swap chains)")
	// ErrRevokeUnknownRoot rejects a takedown that names a root the chain has
	// never committed. Without this a quorum could revoke a competitor's
	// unpublished hash, or a hash that never existed — arbitrary censorship of
	// content that isn't on the ledger (red-team F5). A revocation must point
	// at a real prior publication record.
	ErrRevokeUnknownRoot = errors.New("chain: revocation names a root never published on this chain")
	// ErrUnrevokeNotRevoked rejects an un-revoke of a root that is not
	// currently revoked — the reversibility record only clears a live takedown.
	ErrUnrevokeNotRevoked = errors.New("chain: un-revocation names a root that is not currently revoked")
	// ErrBadBondReg rejects an on-chain bond registration whose validator
	// signature or space-time proof does not verify (objective fork-choice, F6):
	// a forged registration cannot buy objective weight.
	ErrBadBondReg = errors.New("chain: invalid on-chain bond registration")

	// ErrSharedRootInBlock rejects a block that carries two bond registrations
	// from DISTINCT validator IDs on the SAME root (CONSENSUS-RULE, certified
	// 2026-08-28 same-root-intrablock-bondreg-contention, resolution (a)).
	// apply()'s per-root dedup resolves such a collision by intra-block SLICE
	// ORDER (chain.go:2780-2790), so two honest replicas applying the identical
	// block in a different BondReg order commit a different bonded/bondRootOwner
	// state — an order-dependent commit the era-3 SMT root cannot tolerate. The
	// validity layer rejects the collision so it can never commit. Runs
	// UNCONDITIONALLY (not behind regGateActive); a validator re-registering its
	// OWN root (same ID) is NOT a collision (renew/resize is legal, F1). Dedup is
	// on (root × distinct-ID), never on root alone.
	ErrSharedRootInBlock = errors.New("chain: block carries two bond registrations from distinct identities on the same root (order-dependent commit — refused)")

	// ErrPrunedAboveHorizon rejects a payload-pruned (Answer-less) block presented
	// at or above the node's OWN trust floor (max WS-checkpoint / rolling retention
	// horizon). A pruned block cannot have its space-time proof re-verified, so
	// trusting one at/above the finalized anchor would let a peer strip Answer to
	// skip verification and forge standing — a C1 (no-discount) break. Trusted only
	// strictly below the floor, where finality already makes the reg irreversible
	// (Q2 gate, slice 3; PE ruling pruned-block-representation-2026-08-18).
	ErrPrunedAboveHorizon = errors.New("chain: pruned block at/above the node's trust floor (would skip space-time verification — refused)")

	// ErrMalformedPruned rejects a block marked pruned (Pruned set) that still
	// carries a BondReg.Answer — a full block cannot smuggle a forged stored-hash
	// past the Q2 skip. The two are mutually exclusive by construction (Prune()
	// drops every Answer); a hybrid is hand-crafted and refused (decode-invariant belt).
	ErrMalformedPruned = errors.New("chain: block is marked pruned but carries a BondReg.Answer (malformed)")
	// ErrBadSlash rejects an on-chain equivocation record that is not a valid,
	// self-verifying double-sign proof — so a forged slash cannot evict an honest
	// validator (F2; forged-slash griefing stays denied).
	ErrBadSlash = errors.New("chain: invalid equivocation slash proof")
	// ErrGenesisTakedown rejects a genesis block carrying revocation/un-revocation
	// records — a genesis has no prior history to check a takedown against, so
	// allowing one would be a pre-emptive takedown of never-published content (F3).
	ErrGenesisTakedown = errors.New("chain: genesis must not carry takedowns (pre-emptive revocation forbidden)")
)

// Chain is a validator's replica plus the rules for growing it.
type Chain struct {
	cfg     Config
	rep     func(ports.NodeID) int64
	blocks  []Block
	byRoot  map[ports.Hash]ports.Entry
	revoked map[ports.Hash]bool
	// revLog is the CT-style transparency log (core/translog): every honored
	// revocation and un-revocation is appended as an event, in commit order, so a
	// takedown can be PROVEN recorded (inclusion) and history can't be silently
	// rewritten (consistency). A pure, deterministic function of the committed
	// blocks — rebuilt identically on replay — so it needs no separate persistence.
	revLog *translog.Log
	// validatorsSeen is the set of distinct qualified attesters that have
	// ever committed a block — the monotonic decentralization signal the
	// training wheels shed on (see Mature).
	validatorsSeen map[ports.NodeID]bool
	// everMature is the ONE-WAY MATURITY LATCH (F-1): true once Mature() has held
	// at ANY committed height. The launch anchors are load-bearing only while this
	// is FALSE, so once a network is first certified decentralized the zero-bond
	// anchors NEVER become load-bearing again — the one-way ratchet immutable #3
	// promises. Like validatorsSeen/bonded it is a pure, monotonic function of the
	// committed block sequence (latched in apply, re-derived on Reload, carried
	// across a reorg by adopt), so every replica agrees on it as a CONSENSUS fact.
	// It is never reset. The old code gated anchors on the LIVE Mature(), which
	// re-armed the wheels whenever concentration rose — even from one honest whale
	// growing REAL bond — handing a zero-bond anchor permanent power, or halting the
	// chain if the anchors were gone. De-maturation liveness AFTER the latch is
	// carried by the real-bond super-quorum fallback (RequiredQuorum), never by
	// re-arming anchors. See docs/design/m0.md §10 (F-1).
	everMature bool
	// Publisher-privacy publish tokens (F1): when tokenQuorum > 0 every entry
	// must carry a PublishToken blind-signed by that many distinct qualified
	// validators (issuer keys from issuerKey), and spent records each serial so
	// it can be spent only once (double-spend rejected chain-wide).
	tokenQuorum int
	issuerKey   func(ports.NodeID) *rsa.PublicKey
	spent       map[string]bool
	// bonded is the OBJECTIVE validator set for fork-choice (F6): NodeID → the
	// bonded size from its latest on-chain BondReg. A pure function of the blocks,
	// so every replica computes it identically — which is the whole point.
	// Populated only when Config.MinBond > 0. verifyBond re-checks a registration's
	// space-time proof (injected so core/chain stays decoupled from core/bond).
	bonded     map[ports.NodeID]int64
	verifyBond func(pk []byte, root ports.Hash, size int64, nonce uint64, answer []byte) bool
	// bondRootOwner enforces, in the OBJECTIVE set, the same per-root dedup the
	// credit ledger has (credit.rootOwner): a bond Root builds standing for AT
	// MOST ONE identity, so a colluding operator pointing N identities at one
	// shared plot earns one bond's standing, not N (red-team F1). The space-time
	// proof is not identity-bound, so without this a single plot's answer would
	// verify — and be credited — for every identity that copies it.
	bondRootOwner map[ports.Hash]ports.NodeID
	// bondRootProven records whether a root's current owner claimed it with a
	// VERIFIED space-time proof (a height>0 registration, gated by
	// validateBondRegs) rather than a mere DECLARED genesis registration
	// (applied without a proof). A proof of possession displaces a declaration,
	// so a malicious genesis cannot pre-squat an honest validator's real plot
	// root and, via the F1 first-owner dedup, lock the true holder out when it
	// later registers with a genuine proof (retest G3). Once proven, a root
	// stays proven, so first-proven-owner still wins among real bonds (F1).
	bondRootProven map[ports.Hash]bool
	// bondRegHeight records the height of the block carrying each validator's
	// LATEST bond registration, so objective standing can expire on a cadence
	// (Config.BondTTLBlocks, retest G4): a bond not renewed with a fresh proof
	// within the TTL window is pruned from `bonded`. Deterministic (a function of
	// block height), so every replica decays standing identically.
	bondRegHeight map[ports.NodeID]uint64
	// regVersion records the #506 readiness signal from each validator's LATEST
	// bond registration (0 = a pre-gate binary, not rule-aware). Derived state,
	// a pure function of the blocks like bondRegHeight; expires with it.
	regVersion map[ports.NodeID]uint8
	// gateLockedIn/gateHeight are the #506 activation state (post-latch path):
	// at the first mature epoch boundary where rule-aware frozen weight clears
	// the >⅔ super-quorum, the gate locks in ONE-WAY (monotonic — un-tightening
	// would itself be a hard fork; a later ready-weight collapse stalls commits,
	// it never forks, per the certification Q3) and enforcement begins at the
	// NEXT boundary: gateHeight = H_act, the R-rule applies to every block of
	// height > gateHeight. Derived deterministically from committed history in
	// rotateEpoch, so every replica — live or replaying — computes the identical
	// H_act; epoch boundaries are super-quorum-final (#357 Condition A), so
	// H_act cannot be reorged out from under enforcement (certification Q2/I3).
	gateLockedIn bool
	gateHeight   uint64
	// era3LockedIn/era3Height are the era-3 (v4) committed-state-root activation
	// state, the #506 gateLockedIn/gateHeight mechanism reused one readiness level
	// up (research cert Q5/Q7). At the first mature epoch boundary where
	// era-3-AWARE frozen weight (regVersion >= BlockVersionStateRoot, == 4) clears
	// the same >⅔ super-quorum, era-3 locks in ONE-WAY (monotonic — un-tightening
	// would itself be a hard fork; a later ready-weight collapse stalls, never
	// un-flips) and activation begins at the NEXT boundary: era3Height = H_era3,
	// and v4 is REQUIRED (mint + validity) for every block of height >= era3Height.
	// The >= (vs #506's >) is because era-3 is a MINT/FORMAT boundary: H_era3 is
	// itself the first v4 height. Derived deterministically from committed history
	// in rotateEpoch, so every replica — live or replaying — computes the identical
	// H_era3; epoch boundaries are super-quorum-final (#357 Condition A), so H_era3
	// cannot be reorged out from under enforcement (certification Q5/I3). A DISTINCT
	// readiness level from the #506 gate (>= 3): a node signals 4 only when it can
	// enforce the R-rule AND validate committed roots — the two are different
	// software states, so one const cannot gate both (cert Q7).
	era3LockedIn bool
	era3Height   uint64
	// bondDomain records the committed A-axis failure-domain label from each
	// validator's LATEST bond registration (0 = unset). A pure function of the
	// committed blocks, so C2Metric can count address-diverse participants
	// deterministically (NakamotoDomains). See BondReg.Domain.
	bondDomain map[ports.NodeID]uint64
	// slashed is the set of validators evicted for a proven equivocation (F2). A
	// slashed id is disqualified and cannot re-earn bonded standing, so a proven
	// double-sign costs standing in the OBJECTIVE set, not only the rep ledger.
	slashed map[ports.NodeID]bool
	// epochSet is the FROZEN mature-phase validator set (#357 Condition A):
	// NodeID → bonded size, snapshotted from the committed bonded ledger at the
	// last epoch-boundary block (rotateEpoch). While a mature epoch is in force
	// (matureEpoch, EpochBlocks > 0), qualification, the finality quorum size N,
	// and attester fork-choice weight all read THIS set — never the live-churning
	// `bonded` map — because quorum-intersection safety (what makes a §3
	// super-quorum FINAL) only holds when every quorum is taken over the same set.
	// nil during the launch phase (the fixed anchor set governs) and when epochs
	// are disabled. Like every other derived field it is a pure function of the
	// committed blocks: re-derived by apply on Reload/replay, carried by adopt.
	epochSet map[ports.NodeID]int64
	// epochStart is the height of the boundary block that began the current epoch
	// (observability; rotation cadence is Config.EpochBlocks).
	epochStart uint64
	// matureEpoch is the ONE-WAY handoff flag (#357 Condition B): set at the first
	// epoch rotation at-or-after the everMature latch trips, never cleared. The
	// latch (everMature) records the consensus FACT of maturity the moment it
	// holds; the HANDOFF — anchors shed, weight meaning flips to committed bond,
	// quorum re-sizes onto the bonded snapshot — waits for the next epoch
	// boundary, a block that is itself super-quorum-final under the §3 gate. That
	// roots the change in what fork-choice weight MEANS at an immutable base, so
	// bond-weighted fork-choice can never reach back across the boundary (the
	// residual non-monotonicity Condition B exists to close). With epochs disabled
	// the handoff degenerates to the raw latch (pre-Condition-B behavior).
	matureEpoch bool
	// trustFloorOverride, when non-nil, pins the pruned-tolerance floor to the
	// RECEIVING node's own anchor during a Reconcile replay, so the throwaway `tmp`
	// replica trusts a payload-pruned block only strictly below the RECEIVER's
	// finalized/checkpoint anchor — never a height derived from the (attacker-
	// supplied) fork under replay. nil on a live chain, where trustFloor() computes
	// from the node's own state. This is the C1 gate's load-bearing edge: without
	// it a peer could inflate the acceptance floor by presenting a fork with a high
	// finalized head. (Q2 gate, slice 3.)
	trustFloorOverride *uint64
}

// New starts an empty replica. rep is the local reputation view —
// validators judge proposals by what THEY have observed (audits run,
// serves seen), which is the sense in which trust here is earned, not
// declared.
func New(cfg Config, rep func(ports.NodeID) int64) *Chain {
	return &Chain{cfg: cfg, rep: rep,
		byRoot:         make(map[ports.Hash]ports.Entry),
		revoked:        make(map[ports.Hash]bool),
		revLog:         translog.New(),
		validatorsSeen: make(map[ports.NodeID]bool),
		spent:          make(map[string]bool),
		bonded:         make(map[ports.NodeID]int64),
		bondRootOwner:  make(map[ports.Hash]ports.NodeID),
		bondRootProven: make(map[ports.Hash]bool),
		bondRegHeight:  make(map[ports.NodeID]uint64),
		regVersion:     make(map[ports.NodeID]uint8),
		bondDomain:     make(map[ports.NodeID]uint64),
		slashed:        make(map[ports.NodeID]bool)}
}

// SetBondVerifier wires the objective-fork-choice bond check (F6): given a
// registration's (pk, root, size, nonce, answer), it re-verifies the space-time
// proof (typically bond.VerifySpaceTime with the node's VDF params). pk is the
// registrant's ed25519 public key (BondReg.Validator) — the labeling check (G2)
// recomputes labels from H(pk, n), so the plot is bound to this identity and
// size, not to an attacker-chosen root. Required for Config.MinBond > 0 to take
// effect; injected so core/chain does not depend on core/bond or core/vdf.
func (c *Chain) SetBondVerifier(f func(pk []byte, root ports.Hash, size int64, nonce uint64, answer []byte) bool) {
	c.verifyBond = f
}

// objective reports whether fork-choice runs on on-chain bonds (F6) rather than
// the local reputation view. It needs both a positive MinBond and a wired
// verifier — without the verifier we could not re-check a registration, so we
// fall back to the legacy rep path rather than trust an unproven bond.
func (c *Chain) objective() bool { return c.cfg.MinBond > 0 && c.verifyBond != nil }

// epochsEnabled reports whether the mature phase runs on per-epoch validator-set
// snapshots (#357 Condition A). Objective-mode only: the legacy reputation path
// has no committed bonded set to snapshot.
func (c *Chain) epochsEnabled() bool { return c.cfg.EpochBlocks > 0 && c.objective() }

// handedOff reports whether the young→mature handoff has occurred — the moment
// the anchors shed and consensus (eligibility, quorum sizing, fork-choice weight)
// moves onto the committed bonded set. With epochs enabled this is the FIRST
// mature epoch rotation (#357 Condition B: a finalized boundary block), which may
// trail the everMature latch by up to EpochBlocks; with epochs disabled it is the
// raw latch (the pre-Condition-B behavior, safe only for trusted/sim configs).
// One-way either way (F-1): matureEpoch is never cleared and everMature never
// resets, so the anchors can never re-arm.
func (c *Chain) handedOff() bool {
	if c.epochsEnabled() {
		return c.matureEpoch
	}
	return c.everMature
}

// Objective reports whether this replica runs objective (on-chain bond)
// fork-choice (F6) rather than the local reputation view — so a proposer knows
// to attach its live bond registration.
func (c *Chain) Objective() bool { return c.objective() }

// launchAnchor reports whether id bootstraps the objective validator set: a
// declared training-wheels anchor, but ONLY while the network is immature. It
// breaks the objective-mode cold-start chicken-and-egg (you must be bonded on
// chain to propose/attest, but the first block that records bonds must itself be
// proposed and attested) by letting the declared launch set commit the early
// blocks — the same plural, threshold-gated set the training wheels already
// trust. It sheds MECHANICALLY at maturity (Mature()), after which only real
// on-chain bonds qualify. Anchors are expected to register their OWN real bonds
// early (live self-registration), so this is a launch crutch, not a standing
// exemption. It grants ELIGIBILITY plus a FIXED bootstrap fork-choice weight during
// the young window (#357 §1a: Config.AnchorWeight, defaulting to MinBond) — the
// sanctioned training-wheels trust that gives the anchor→bonded ramp a present,
// monotone weight signal (without it every fresh fork weighed ≈0 and a height-blind
// tiebreak dropped committed blocks to height 0). A real bond of any size still
// outweighs a min-weight anchor, and the anchor weight VANISHES at maturity (this
// returns false once everMature), so it is not a standing exemption and the
// mature-regime fork-choice quantity (summed committed bond) is unchanged.
func (c *Chain) launchAnchor(id ports.NodeID) bool {
	// Gated on the one-way handoff, not the live Mature(): once the network has
	// handed off, anchors lose bond-free eligibility FOREVER (F-1). An anchor that
	// registered its own real bond stays a normal validator on that real weight.
	// With epochs enabled the handoff is the first mature epoch ROTATION (#357
	// Condition B), so after the everMature latch trips mid-epoch the anchors keep
	// governing — deterministically, for at most EpochBlocks more blocks — until
	// the finalized boundary sheds them; without epochs it is the raw latch.
	return len(c.cfg.Anchors) > 0 && c.cfg.Anchors[id] && !c.handedOff()
}

// attesterQualified reports whether id may have its attestation counted toward
// quorum (and, if it has a real bond, weight), OUTSIDE any specific block's
// validation — the frozen-set rule, with no #535 recovery awareness. Height-
// keyed validation paths use attesterQualifiedAt so the one directed recovery
// boundary consults the re-based set; every height-less probe (solicitation,
// token issuer quorum, the maturity metric) stays on the frozen rule.
func (c *Chain) attesterQualified(id ports.NodeID) bool {
	return c.attesterQualifiedAt(id, 0) // height 0 is genesis — never a recovery boundary
}

// attesterQualifiedAt is attesterQualified for the block at height h. Objective
// mode: membership in the epoch set GOVERNING h during a mature epoch (#357
// Condition A — the frozen snapshot everywhere except the operator-directed
// #535 recovery boundary, effectiveEpochSet), otherwise its committed bonded
// size clears MinBond OR it is a launch anchor bootstrapping an immature
// network. Legacy mode: the local reputation view.
func (c *Chain) attesterQualifiedAt(id ports.NodeID, h uint64) bool {
	if c.slashed[id] {
		return false // evicted for a proven equivocation (F2) — the ONE live mid-epoch disqualification
	}
	if c.objective() {
		if c.epochsEnabled() && c.matureEpoch {
			// Condition A: qualification is FROZEN for the epoch. A bond that
			// joins, renews, or TTL-expires mid-epoch integrates at the next
			// rotation — a live recompute here is exactly the churning-set
			// finalization unsoundness the snapshot exists to close. (A lapsed
			// member therefore keeps its vote for ≤ EpochBlocks: bounded,
			// deliberate — a protocol-forced mid-epoch disqualification would
			// shrink the attester supply below the frozen N and could stall the
			// chain before it ever reaches the boundary that rotates it out.)
			// The ONE exception is the #535 operator-directed recovery boundary,
			// where the governing set is the live re-base — the attester filter
			// must draw from the SAME set the weight quorum sums, or the quorum
			// is sized over one set and filled from another (the #402 trap).
			_, ok := c.effectiveEpochSet(h)[id]
			return ok
		}
		return c.bonded[id] >= c.cfg.MinBond || c.launchAnchor(id)
	}
	return c.rep(id) >= c.cfg.MinAttesterRep
}

// proposerQualified reports whether id may propose, outside any specific
// block's validation — the frozen-set rule (see attesterQualified).
func (c *Chain) proposerQualified(id ports.NodeID) bool {
	return c.proposerQualifiedAt(id, 0)
}

// proposerQualifiedAt is proposerQualified for the block at height h. Objective
// mode: membership in the epoch set governing h during a mature epoch
// (Condition A; the #535 recovery boundary re-bases — same rule as attesters),
// otherwise a bonded validator or a launch anchor while the network is
// immature. Legacy mode uses MinProposerRep.
func (c *Chain) proposerQualifiedAt(id ports.NodeID, h uint64) bool {
	if c.slashed[id] {
		return false // evicted for a proven equivocation (F2)
	}
	if c.objective() {
		if c.epochsEnabled() && c.matureEpoch {
			_, ok := c.effectiveEpochSet(h)[id] // governing set for h, same rule as attesters
			return ok
		}
		// LAUNCH WINDOW — ANCHOR-ONLY PROPOSING (#402 encoding B; research
		// certification 2026-08-14). While the network is young, ONLY anchors
		// propose; a bonded sybil drains its standing via MsgSubmitBondReg
		// (submit-don't-propose, #397), never by proposing. This removes the
		// sybil-proposed launch fork at its source — the both-sybil-proposed 2-2
		// anchor split the intersecting-quorum invariant (I1) must otherwise refuse
		// — and composes with the derived strict-anchor-majority gate in
		// ValidateCommit. Post-handoff (launchAnchor ⇒ false, F-1) this falls through
		// to the bonded rule, so a matured validator proposes on its real weight.
		if len(c.cfg.Anchors) > 0 && !c.handedOff() {
			return c.launchAnchor(id)
		}
		return c.bonded[id] >= c.cfg.MinBond || c.launchAnchor(id)
	}
	return c.rep(id) >= c.cfg.MinProposerRep
}

// liveQualifiedSet is the CURRENT qualified committed bonded set — every
// identity whose committed bond clears MinBond and is not slashed, with its
// weight. It is the set a rotation freezes for the next epoch (rotateEpoch)
// and the set the #535 recovery boundary re-bases against (effectiveEpochSet)
// — ONE computation, shared, so the recovered boundary's governing set is
// byte-identical to the snapshot its own commit then freezes. A pure function
// of committed state: every replica computes it identically.
func (c *Chain) liveQualifiedSet() map[ports.NodeID]int64 {
	set := make(map[ports.NodeID]int64, len(c.bonded))
	for id, sz := range c.bonded {
		if sz >= c.cfg.MinBond && !c.slashed[id] {
			set[id] = sz
		}
	}
	return set
}

// BoundaryLivenessFloorLost reports whether the chain is in the #535 wedge
// state at height h: a mature-epoch boundary whose frozen members still
// holding live qualified bonds carry at most ⅔ of the frozen weight — so NO
// coalition of live members can commit h, and the rotation that would shed the
// lapsed weight is gated behind the very quorum the lapse denies (the
// certified-correct, safety-first stall). Exposed for OPERATOR VISIBILITY
// (S5 — never silently fail): the node logs this state and chain-status names
// it, so the operator knows a coordinated -liveness-recovery-height (the #535
// fix (3) escape) may be required. Diagnosis only — it changes no rule.
func (c *Chain) BoundaryLivenessFloorLost(h uint64) bool {
	if !c.objective() || !c.epochsEnabled() || !c.matureEpoch || h%c.cfg.EpochBlocks != 0 {
		return false
	}
	var total, live int64
	for id, w := range c.epochSet {
		total += w
		if !c.slashed[id] && c.bonded[id] >= c.cfg.MinBond {
			live += w
		}
	}
	return total > 0 && 3*live <= 2*total
}

// effectiveEpochSet is the validator set GOVERNING the block at height h in a
// mature epoch: the frozen epochSet everywhere, except the one operator-
// directed #535 liveness-recovery boundary (Config.LivenessRecoveryHeight),
// where it is the live qualified bonded set. All three mature-epoch validation
// predicates — attester qualification, proposer qualification, and the >⅔
// weight quorum — consult THIS function, so the set a quorum is sized over and
// the set it is filled from can never differ (I1; the #402 checklist: the
// arithmetic intersects because both coalitions of the h block are >⅔ of the
// SAME set). Boundary-only by construction: a non-boundary directive never
// fires, so a mid-epoch set change (the I3 churning-set unsoundness) is
// impossible. Off (0) by default — a bled boundary stalls, the certified
// safety-first behavior; see Config.LivenessRecoveryHeight for the trust model.
func (c *Chain) effectiveEpochSet(h uint64) map[ports.NodeID]int64 {
	if c.cfg.LivenessRecoveryHeight != 0 && h == c.cfg.LivenessRecoveryHeight &&
		c.epochsEnabled() && h%c.cfg.EpochBlocks == 0 {
		return c.liveQualifiedSet()
	}
	return c.epochSet
}

// qualifiedCount is the number of distinct on-chain validators currently eligible
// to attest in objective mode: a committed bond ≥ MinBond, not slashed. It is the
// N the Byzantine quorum is sized against, recomputed identically by every replica
// from the chain (so RequiredQuorum never diverges). Anchors that also hold a real
// bond are counted; a pure training-wheels anchor with no bond is not (it is a
// bootstrap backstop, not part of the decentralized fault budget).
func (c *Chain) qualifiedCount() int {
	n := 0
	for id, sz := range c.bonded {
		if sz >= c.cfg.MinBond && !c.slashed[id] {
			n++
		}
	}
	return n
}

// bftThreshold is the number of NON-PROPOSER attestations a commit needs for
// Byzantine quorum-intersection safety over n validators. The tolerated fault
// count is f = ⌊(n-1)/3⌋, and the total support set (the proposer, always a
// qualified signer, PLUS its attesters) must be a supermajority of n−f. So the
// attestation count returned is (n−f)−1. Any two such support sets of size n−f
// intersect in 2(n−f)−n = n−2f ≥ f+1 validators (since n ≥ 3f+1), so — with ≤ f
// faulty — at least one HONEST validator is in both, which stops two conflicting
// blocks from each gathering a quorum (safety). n ≤ 0 → 0.
func bftThreshold(n int) int {
	if n <= 0 {
		return 0
	}
	f := (n - 1) / 3
	q := n - f - 1
	if q < 0 {
		return 0
	}
	return q
}

// RequiredQuorum is the number of distinct qualified non-proposer attestations a
// commit needs. It is Config.Quorum by default, but with ByzantineQuorum set in
// objective mode it rises to the Byzantine threshold 2f+1 over the qualified set —
// max(Quorum, bftThreshold(N)) — so quorum-intersection safety is preserved as the
// validator set grows. Exposed so a proposer gathers enough attestations to match
// what ValidateCommit will demand.
//
// MATURE EPOCH: the Byzantine escalation is WEIGHT-counted, not head-counted
// (requireEpochWeightQuorum; research certification 2026-08-13 B2). Counting
// members here let every MinBond identity in the epoch snapshot raise the bar
// one head — so a cohort of cheap bonds could push bftThreshold(len(epochSet))
// beyond what the honest validators could ever attest (stall at 8×MinBond), and
// one head past that, commit ALONE with zero honest attestation (capture) — a
// C1-discount + C2-quiet-capture break, since control priced per-head at MinBond
// never has to pay real weight. In a mature epoch this returns only the
// Config.Quorum count floor; the super-majority that makes a commit final is
// the >⅔ FROZEN-WEIGHT rule (Tendermint/Casper count stake, never heads — B8).
func (c *Chain) RequiredQuorum() int {
	q := c.cfg.Quorum
	if c.cfg.ByzantineQuorum && c.objective() {
		if c.epochsEnabled() && c.matureEpoch {
			return q // weight rule carries the Byzantine bar (ValidateCommit)
		}
		if bq := bftThreshold(c.validatorSetSize()); bq > q {
			q = bq
		}
	}
	return q
}

// validatorSetSize is N, the set the Byzantine quorum is sized against (#357 §2).
// It must be STABLE across a ramp: sizing against the live `qualifiedCount` made
// RequiredQuorum shift block-to-block as ~1.5 MB bond registrations drained in, so
// no fork ever held a quorum of a consistent set (quorum-intersection safety needs a
// FIXED n) — the "0 of 2 gathered" stall. During the young window the set is the
// fixed anchor set (seeded at genesis — the sanctioned trust, immutable #3); after
// the handoff it is the per-epoch FROZEN bonded snapshot (#357 Condition A —
// Tendermint/Casper both fix the set to buy finality), falling back to the live
// qualifiedCount only when epochs are explicitly disabled (trusted/demo). A
// bootstrap 4-anchor network gets bftThreshold(4)=2, matching the "2 attestations"
// the field logs show.
func (c *Chain) validatorSetSize() int {
	if c.objective() && !c.handedOff() && len(c.cfg.Anchors) > 0 {
		return len(c.cfg.Anchors)
	}
	if c.epochsEnabled() && c.matureEpoch {
		// #357 Condition A: post-handoff, N is the FROZEN epoch snapshot — never
		// the live qualifiedCount, whose churn (joins, renewals, TTL expiry) would
		// let two conflicting commits each gather a "super-quorum" of two different
		// sets with no guaranteed honest intersection. N holds even as members are
		// slashed mid-epoch (shrink-only: a smaller live set against a frozen N can
		// only RAISE the effective bar, never weaken intersection).
		return len(c.epochSet)
	}
	return c.qualifiedCount()
}

// finalityQuorumActive reports whether the quorum rule currently in force gives
// genuine quorum-intersection — the precondition for the §3 finality gate. Two
// legs: the count rule at bftThreshold over the pinned set (launch window /
// epochs-off), or the mature-epoch >⅔ frozen-weight rule (B2), which intersects
// in weight instead of heads. A trusted weak config (low Quorum, no
// ByzantineQuorum) has neither, and keeps heaviest-chain reorg.
func (c *Chain) finalityQuorumActive() bool {
	if !c.objective() {
		return false
	}
	if c.cfg.ByzantineQuorum && c.epochsEnabled() && c.matureEpoch {
		return true // weight-counted super-majority (requireEpochWeightQuorum)
	}
	return c.RequiredQuorum() >= bftThreshold(c.validatorSetSize())
}

// requiredLaunchAnchors is the number of anchor signatures a commit needs during
// the launch window — the launch face of quorum-intersection (I1, #402). In
// OBJECTIVE mode it is DERIVED: a STRICT ANCHOR MAJORITY ⌊A/2⌋+1 over the configured
// anchor set, independent of the AnchorQuorum knob. Deriving it is load-bearing: the
// field run left `-anchor-quorum` unset (default 0) so the gate went inert and a
// two-sybil-signature quorum forked the launch chain (#402); config can no longer
// disable intersection. Two ⌊A/2⌋+1 anchor sets over A anchors share ≥1 anchor, which
// never signs twice at a height (#397) → at most one block per height finalizes. In
// LEGACY (non-objective) mode there is no finality gate, so this is the configured
// AnchorQuorum capture-prevention floor (unchanged pre-#402 behavior). 0 = no gate
// (handed off, no anchors, or legacy with AnchorQuorum unset).
//
// Quorum-intersection checklist (consensus-invariants.md): (1) finalizes? yes — a
// passing commit is reorg-refused by the finality gate. (2) N/membership? N =
// len(Anchors); countAnchorSupport draws ONLY from Anchors — size-set == membership-
// set, the #402 trap closed. (3) intersects? 2·(⌊A/2⌋+1) > A ∀A≥1 (⌈A/2⌉ is the even-A
// off-by-one that admitted the 2-2 split). (4) non-members excluded? sybils never
// count. (5) basis? anchors are the pinned, Sybil-resistant-equal launch set, so
// head-count is sound (weight is the mature phase's job). (6) phase boundary? gated on
// !handedOff(); post-handoff the >⅔ frozen-weight rule takes over.
func (c *Chain) requiredLaunchAnchors() int {
	if len(c.cfg.Anchors) == 0 || c.handedOff() {
		return 0
	}
	if c.objective() {
		return len(c.cfg.Anchors)/2 + 1
	}
	return c.cfg.AnchorQuorum
}

// countAnchorSupport counts the anchors in a commit's support coalition: the distinct
// anchor attesters in `seen`, plus the proposer if it is an anchor (objective mode —
// the certified #402 rule counts the proposer-if-anchor toward the intersecting set;
// legacy counts non-proposer anchors only, as it always has). Shared by ValidateCommit
// (validation) and SupportMeetsQuorum (the proposer's gather target) so the two cannot
// drift — a gather that stopped short of what validation demands is exactly the
// under-gather → self-Append-failure bug this centralization prevents.
func (c *Chain) countAnchorSupport(proposer ports.NodeID, seen map[ports.NodeID]bool) int {
	n := 0
	for id := range seen {
		if c.cfg.Anchors[id] {
			n++
		}
	}
	if c.objective() && c.cfg.Anchors[proposer] {
		n++
	}
	return n
}

// BondRegNonce is the fresh challenge a non-genesis bond registration must
// answer, derived from the parent hash the block extends — so a registration
// proves possession AT this position and cannot be replayed to another height
// or fork. A registrant (see NewBondReg) computes its space-time answer for this
// nonce; the chain re-derives it identically at validation.
//
// HONESTLY WEAK, by necessity (seam-6, red-team 2026-08-08): this nonce is
// deterministic and therefore PREDICTABLE — once `prev` commits, a validator knows
// the exact challenge for its next on-chain renewal, so the on-chain path alone does
// not bound release-and-recompute-just-in-time. It cannot be made unpredictable
// without a randomness beacon (M0 has none) and MUST stay a pure function of
// committed history so every replica re-derives it identically for objective
// verification. The coast it would otherwise permit is bounded ELSEWHERE: (1) the
// parallel LIVE peer-audit (core/node/bondaudit.go) issues an UNPREDICTABLE nonce
// (n.rid, peer-initiated) at random, and (2) that live audit now carries the
// BondMaxAnswerLatency reply-deadline (BREAK 1 / owned-residuals A5), so a prover
// that released and must recompute past the ~0.25 knee fails it. So the predictable
// on-chain nonce is a documented weakness held by the live-audit path, not a hole.
func BondRegNonce(prev ports.Hash) uint64 {
	h := sha256.Sum256(append([]byte("silt/chain/bondreg/nonce/v1"), prev[:]...))
	return binary.BigEndian.Uint64(h[:8])
}

// NewBondReg builds a signed registration for a validator's bond at the position
// following prev. answer is the CBOR-encoded space-time proof (bond.EncodeAnswer)
// for BondRegNonce(prev); the chain re-verifies it via the injected bond verifier
// (SetBondVerifier). The signature binds the (root, size, nonce) claim to the
// validator's key so a non-holder cannot register a bond it does not own.
func NewBondReg(signer ed25519.PrivateKey, root ports.Hash, size int64, answer []byte, prev ports.Hash, domain uint64) BondReg {
	r := BondReg{
		Validator: append([]byte(nil), signer.Public().(ed25519.PublicKey)...),
		Root:      root,
		Size:      size,
		Answer:    answer,
		Domain:    domain, // committed A-axis label (0 = unset); signed via signingBytes
		// The #506 readiness signal is a property of the BINARY, not a choice:
		// software that mints regs through this constructor validates the gate
		// era's rules, so every reg it produces says so (signed via signingBytes).
		Version: BlockVersionRegGate,
	}
	r.Sig = ed25519.Sign(signer, r.signingBytes(BondRegNonce(prev)))
	return r
}

// validateBondRegs verifies a non-genesis block's bond registrations: each must
// carry a validator signature over its (root, size, nonce) and a space-time
// proof the injected verifier accepts for the fresh per-position nonce. Only
// enforced in objective mode; a legacy chain ignores BondRegs entirely.
// DefaultBondRegHeadWindow is the K used when Config.BondRegHeadWindow is 0: a bond
// reg stays valid over the last 8 committed heads. Covers WAN propagation + one
// proposer rotation (the factor-ii staleness window) while remaining ≪ any real
// BondTTLBlocks and re-challenged continuously by bond-audit. See Config.BondRegHeadWindow.
const DefaultBondRegHeadWindow = 8

// recentBondRegNonces returns BondRegNonce for `prev` and up to K-1 committed heads
// before it (K = BondRegHeadWindow) — the window over which a bond reg is accepted
// (fix for factor ii). Deterministic: every replica walks the same committed block
// links, so the accepted-nonce set is identical everywhere.
func (c *Chain) recentBondRegNonces(prev ports.Hash) []uint64 {
	k := c.cfg.BondRegHeadWindow
	if k <= 0 {
		k = DefaultBondRegHeadWindow
	}
	nonces := make([]uint64, 0, k)
	cur := prev
	for len(nonces) < k {
		nonces = append(nonces, BondRegNonce(cur))
		blk, ok := c.blockByHash(cur)
		if !ok || blk.Height == 0 {
			break // genesis or not-yet-committed parent: the window ends here
		}
		cur = blk.Prev
	}
	return nonces
}

// blockByHash finds a committed block by its hash, scanning from the tip (recent
// blocks — the drain window — are found in the first few steps).
func (c *Chain) blockByHash(h ports.Hash) (Block, bool) {
	for i := len(c.blocks) - 1; i >= 0; i-- {
		if c.blocks[i].Hash() == h {
			return c.blocks[i], true
		}
	}
	return Block{}, false
}

// validateBondRegWindow accepts a reg if it validates against ANY nonce in the
// recent-head window; on failure it returns the CURRENT-head error (the most
// informative for a genuinely-bad reg, not merely a stale one).
func (c *Chain) validateBondRegWindow(r BondReg, nonces []uint64) error {
	var firstErr error
	for i, n := range nonces {
		err := c.validateBondReg(r, n)
		if err == nil {
			return nil
		}
		if i == 0 {
			firstErr = err
		}
	}
	return firstErr
}

func (c *Chain) validateBondRegs(b *Block) error {
	if !c.objective() {
		return nil
	}
	// Q2 pruned-tolerance gate (slice 3; C1/M0 merge gate). A payload-pruned block has
	// its heavy BondReg.Answer dropped, so its space-time proof cannot be re-verified.
	// Trust it ONLY strictly below this node's OWN finalized/checkpoint anchor
	// (trustFloor) — where finality already makes the reg irreversible and re-verification
	// is neither possible nor needed. At/above the floor, trusting an Answer-less block
	// would let a peer skip the proof and forge standing (a no-discount break), so REJECT.
	// During a Reconcile replay trustFloor() reflects the RECEIVER's anchor (threaded via
	// trustFloorOverride), never the attacker-supplied fork's. A full block (Answer
	// present) falls through to full verification at any height, unchanged.
	if b.IsPruned() {
		if b.Height >= c.trustFloor() {
			return fmt.Errorf("%w: pruned block at height %d, floor %d", ErrPrunedAboveHorizon, b.Height, c.trustFloor())
		}
		// Below the anchor: skip the space-time re-verify. Belt (decode-invariant): a
		// pruned block must not also carry an Answer — a full block cannot smuggle a
		// forged stored-hash past the skip. Structural + proposer/attester-sig checks
		// still run elsewhere, against the stored Hash() (slice 2).
		for _, r := range b.BondRegs {
			if r.Answer != nil {
				return fmt.Errorf("%w: validator %s", ErrMalformedPruned, r.ValidatorID())
			}
		}
		return nil
	}
	nonces := c.recentBondRegNonces(b.Prev)
	// #506 R-rule (VALIDITY, active only past the gate — regGateActive): a bond
	// reg for identity X is a valid payload only if X is not slashed (R∞ — the
	// #503 Defect-A commit path, closed structurally) and X's last committed reg
	// is ≥ R blocks old (first registrations are exempt: bondRegHeight unset).
	// This is what bounds reg-BLOCK volume — the ~1.5 MB Answer per committed
	// reg was the #503 OOM driver; the proposer-side filter (#508) keeps honest
	// proposers clean, this makes a storm block INVALID so it cannot commit.
	// Validation runs against the PARENT state, so a block carrying two regs for
	// the same identity needs its own in-block check (seenReg) or a one-block
	// storm would slip the distance rule entirely.
	gate := c.regGateActive(b.Height)
	var seenReg map[ports.NodeID]bool
	if gate {
		seenReg = make(map[ports.NodeID]bool, len(b.BondRegs))
	}
	// PER-ROOT DEDUP (CONSENSUS-RULE, certified 2026-08-28
	// same-root-intrablock-bondreg-contention, resolution (a)): reject a block
	// carrying two registrations from DISTINCT identities on the SAME root. apply()
	// would resolve such a collision by intra-block slice order (chain.go:2780-2790),
	// so two honest replicas applying the identical block in a different BondReg
	// order commit a different bonded/bondRootOwner state — the order-dependent
	// commit the era-3 SMT root cannot tolerate. Rejecting it at validity means the
	// divergent input never commits, so nothing order-dependent is left to hash.
	//
	// UNCONDITIONAL, by certified caveat: seenRoot is NOT gate-gated (seenReg is).
	// The freeze seam must be closed in EVERY regime, including pre-gate genesis-
	// adjacent heights on this validated path. And it dedups on (root × DISTINCT-ID)
	// only — a validator re-registering its OWN root (same ID: renew/resize) is
	// legal (F1) and stays admitted.
	seenRoot := make(map[ports.Hash]ports.NodeID, len(b.BondRegs))
	for _, r := range b.BondRegs {
		id := r.ValidatorID()
		if prev, ok := seenRoot[r.Root]; ok && prev != id {
			return fmt.Errorf("%w: root claimed by both %s and %s in one block", ErrSharedRootInBlock, prev, id)
		}
		seenRoot[r.Root] = id
		if gate {
			if c.slashed[id] {
				return fmt.Errorf("%w: validator %s is slashed", ErrRegGate, id)
			}
			if seenReg[id] {
				return fmt.Errorf("%w: validator %s registered twice in one block", ErrRegGate, id)
			}
			seenReg[id] = true
			if regH, ok := c.bondRegHeight[id]; ok && b.Height-regH < c.regMinInterval() && !c.restoresHeldStanding(id, r.Root) {
				return fmt.Errorf("%w: validator %s re-registered %d blocks after its last reg (R=%d)",
					ErrRegGate, id, b.Height-regH, c.regMinInterval())
			}
		}
		if err := c.validateBondRegWindow(r, nonces); err != nil {
			return err
		}
	}
	return nil
}

// validateBondReg runs the per-registration objective checks for a given
// per-position nonce: a valid validator key, a signature over (root,size,nonce)
// binding the claim to that identity, a size clearing MinBond and the anti-
// release floor, and a space-time proof the injected verifier accepts. Shared by
// validateBondRegs (whole block) and ValidateBondReg (one peer-submitted renewal).
func (c *Chain) validateBondReg(r BondReg, nonce uint64) error {
	if len(r.Validator) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: bond registration has no valid validator key", ErrBadBondReg)
	}
	if !ed25519.Verify(ed25519.PublicKey(r.Validator), r.signingBytes(nonce), r.Sig) {
		return fmt.Errorf("%w: validator %s signature", ErrBadBondReg, r.ValidatorID())
	}
	if r.Size < c.cfg.MinBond {
		return fmt.Errorf("%w: validator %s size %d below MinBond %d", ErrBadBondReg, r.ValidatorID(), r.Size, c.cfg.MinBond)
	}
	if r.Size < c.cfg.MinBondBytes {
		return fmt.Errorf("%w: validator %s size %d below anti-release floor %d", ErrBadBondReg, r.ValidatorID(), r.Size, c.cfg.MinBondBytes)
	}
	if !c.verifyBond(r.Validator, r.Root, r.Size, nonce, r.Answer) {
		return fmt.Errorf("%w: validator %s space-time proof", ErrBadBondReg, r.ValidatorID())
	}
	return nil
}

// ValidateBondReg reports whether a single peer-submitted bond renewal would be
// accepted in a block extending the CURRENT head (H2 non-proposer renewal path).
// A proposer filters the renewals it received through this before including them,
// so one stale or forged submission can't poison the whole block; a receiver uses
// it to drop junk on arrival. False off the objective path (legacy ignores regs).
func (c *Chain) ValidateBondReg(r BondReg) bool {
	return c.ValidateBondRegErr(r) == nil
}

// ValidateBondRegErr is ValidateBondReg with the refusal REASON. A refused
// peer-submitted reg must be attributable from one field observation — the
// #432 wedge hid for three billable runs partly because the receipt path
// dropped refusals silently (chainrole.go MsgSubmitBondReg), so the drop
// looked like discovery/egress from the outside. Never refuse silently (B5).
func (c *Chain) ValidateBondRegErr(r BondReg) error {
	if !c.objective() {
		return fmt.Errorf("bond reg refused: chain is not objective")
	}
	head, next := c.Head()
	// #506 pre-filter: refuse a submission the R-rule would refuse in the block
	// it would ride (height next). Without this an honest proposer includes it,
	// mints a block its OWN validity check rejects, and burns its proposer turn —
	// the gate must fail the reg at receipt, attributably, not the block at
	// commit. (Same reason the #508 slashed filter lives proposer-side.)
	if c.regGateActive(next) {
		id := r.ValidatorID()
		if c.slashed[id] {
			return fmt.Errorf("%w: validator %s is slashed", ErrRegGate, id)
		}
		if regH, ok := c.bondRegHeight[id]; ok && next-regH < c.regMinInterval() {
			return fmt.Errorf("%w: validator %s re-registering %d blocks after its last reg (R=%d)",
				ErrRegGate, id, next-regH, c.regMinInterval())
		}
	}
	return c.validateBondRegWindow(r, c.recentBondRegNonces(head))
}

// validateSlashes verifies a block's on-chain equivocation records (F2): each
// must be a self-verifying double-sign proof — the culprit's own signatures over
// two DIFFERENT blocks at the SAME height. A forged accusation fails
// VerifyEquivocation, so an honest validator cannot be evicted (forged-slash
// griefing stays denied). Enforced on every write path.
func (c *Chain) validateSlashes(b *Block) error {
	for i := range b.Slashes {
		if !VerifyEquivocation(&b.Slashes[i]) {
			return fmt.Errorf("%w: proof %d", ErrBadSlash, i)
		}
	}
	return nil
}

// BondedSize reports the objective on-chain bonded size for id (0 if none).
// Exposed for observability and tests; it is the fork-choice weight of one of
// id's attestations in objective mode.
func (c *Chain) BondedSize(id ports.NodeID) int64 { return c.bonded[id] }

// ProposerEligible reports whether id currently qualifies to propose — the same
// rule ValidateProposal applies. Read-only probe for the node layer, so a
// validator can decide whether a proposal is worth attempting (the #338
// bond-registration drain) without paying a doomed gather.
func (c *Chain) ProposerEligible(id ports.NodeID) bool { return c.proposerQualified(id) }

// EligibleProposers is the sorted set of validators currently qualified to
// propose, derived ONLY from committed chain state (the anchor set during the
// launch window, the bonded/epoch set after) — so every honest replica computes
// the SAME list. The #338 drain uses it to designate a single proposer per
// height (ids[height % len]): without a deterministic designation two eligible
// proposers drain-race the same height, cross-attest each other's conflicting
// blocks on a small young network, and read each other's signatures as
// equivocation — two honest anchors slashing each other into a wedged chain
// (observed in the failing-first #338 repro).
func (c *Chain) EligibleProposers() []ports.NodeID {
	seen := make(map[ports.NodeID]bool)
	var out []ports.NodeID
	add := func(id ports.NodeID) {
		if !seen[id] && c.proposerQualified(id) {
			seen[id] = true
			out = append(out, id)
		}
	}
	for id := range c.cfg.Anchors {
		add(id)
	}
	if c.epochsEnabled() && c.matureEpoch {
		for id := range c.epochSet {
			add(id)
		}
	} else {
		for id := range c.bonded {
			add(id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return bytesLess(out[i][:], out[j][:]) })
	return out
}

// AttesterEligible reports whether id's attestation would currently count
// toward quorum — the same rule ValidateCommit applies. Read-only probe for the
// node layer: a gather that solicits attestations from unqualified peers
// collects signatures that then don't count, failing its own commit — so the
// #338 drain filters its attester set through this.
func (c *Chain) AttesterEligible(id ports.NodeID) bool { return c.attesterQualified(id) }

// AttesterEligibleAt is AttesterEligible for the block at height h — identical
// everywhere except the #535 operator-directed recovery boundary, where the
// governing set is the live re-base (attesterQualifiedAt). The gather must
// SOLICIT from the same set validation will count: a live-but-unfrozen
// member's weight is in the recovery denominator, so a frozen-set solicitation
// filter could leave the assembling coalition short of the very bar it is
// measured against (the #402 size-set/membership-set law, at the wire).
func (c *Chain) AttesterEligibleAt(id ports.NodeID, h uint64) bool {
	return c.attesterQualifiedAt(id, h)
}

// IsBonded reports whether id is a qualified bond-distinct identity in the COMMITTED
// on-chain bond ledger: its bonded size clears MinBond and it has not been slashed.
// This is the LIVE admission bar (what attesterQualified reads outside a mature
// epoch; within one, qualification is the epoch snapshot — #357 Condition A),
// exposed so the demand bank's P3b bonded-fetcher credential prices fake demand onto
// exactly the Sybil-priced identity supply C2 measures. Always false in legacy mode
// (MinBond 0).
func (c *Chain) IsBonded(id ports.NodeID) bool {
	return c.cfg.MinBond > 0 && !c.slashed[id] && c.bonded[id] >= c.cfg.MinBond
}

// BondRenewalDue reports whether validator id should (re)register its bond in the
// NEXT proposed block. True if it holds no committed bond yet (first registration),
// or — when a TTL is set — its latest registration has passed the renewal point
// (halfway to expiry, leaving margin so a single dropped renewal cannot lapse
// standing). False once bonded and comfortably within the TTL.
//
// This gates F6 "proposing IS registering" (and the H2 non-proposer renewal): a
// bonded validator re-registering on EVERY proposal re-embeds its full space-time
// proof in every block for no gain — the LATEST registration already stands — which
// on a real cross-region network bloated every block past what attestation could
// carry in time and WEDGED the chain after the first bond (#313). It also raised the
// participation floor (build-immutable #4) and the per-block bandwidth (#299). Only
// the not-yet-bonded and the genuinely-due-for-renewal cases justify the proof.
func (c *Chain) BondRenewalDue(id ports.NodeID) bool {
	if !c.objective() {
		return false
	}
	if c.bonded[id] < c.cfg.MinBond {
		return true // not yet in the objective set (or lapsed) — register
	}
	if c.cfg.BondTTLBlocks == 0 {
		return false // no expiry configured — one registration stands
	}
	_, next := c.Head() // height of the block being proposed next
	// #555 renewal PHASE-JITTER (research certification 2026-08-25): validators
	// that all registered near genesis hit TTL/2 together, so 5–7 heavy ~1.5 MB
	// space-time proofs land in ONE block every few heights — and each attester
	// verifies every proof on the gather's critical path before it will sign,
	// inflating the two-phase gather latency on the 1 vCPU box (the deep-drive
	// crawl; real networks spread naturally via organic join-times).
	//
	// Shift the renewal point onto a per-identity ABSOLUTE grid — period TTL/2,
	// phase = offset(id) — rounded to the NEAREST grid point within ±TTL/4 of the
	// plain TTL/2 due point. This spreads the genesis-aligned fleet's FIRST
	// renewal across [TTL/4, 3·TTL/4) (so ~1 reg lands per block), while leaving
	// the PERIOD exactly TTL/2 on every LATER cycle: after a renewal
	// bondRegHeight lands on the grid, so the next due point is bondRegHeight +
	// TTL/2 unchanged. That keeps re-registration frequency (the #313 bloat bound)
	// AND the renewal margin (≥ TTL/4 to expiry) intact — only the phase differs
	// per identity. CLIENT-SIDE PACING ONLY: BondRenewalDue gates the node's own
	// drain/submit decision, never block validation, so this changes WHEN an
	// identity re-proves, never a consensus rule; the TTL denomination and its
	// #503 couplings are untouched. Small-TTL (< 4) falls back to the plain point.
	return next >= c.renewalDueHeight(id)
}

// renewalDueHeight computes the #555 phase-jittered renewal point for id: the
// nearest per-identity grid point (period TTL/2, phase offset(id)) to the plain
// TTL/2 due point, clamped to respect the #506 R-rule (#562). Factored out of
// BondRenewalDue so the phase/clamp arithmetic is directly testable.
func (c *Chain) renewalDueHeight(id ports.NodeID) uint64 {
	period := c.cfg.BondTTLBlocks / 2
	due := c.bondRegHeight[id] + period
	if period >= 2 {
		phase := renewalPhaseOffset(id, period)
		r := (due + period - phase) % period // due's offset above the grid, in [0, period)
		if r <= period/2 {
			due -= r // round down to the nearest grid point
		} else {
			due += period - r // round up
		}
		// #562: the nearest grid point can sit CLOSER to the last committed reg
		// than the #506 R-rule allows (rounding reaches down to TTL/4, and R =
		// K+2 can exceed TTL/4 — 10 vs 8 at the field TTL=32), so the renewal
		// submitted there is refused every sweep ("re-registering 9 blocks
		// after its last reg") until the chain outruns R. Clamp to the rate
		// bound instead: at most ONE off-grid cycle (the next due point
		// re-rounds from the new reg height and re-aligns to the grid), never
		// a shortened steady-state period (an on-grid reg's next due point is
		// a full period away, which the R cap < TTL/2 always clears), and —
		// unlike jumping to the NEXT grid point — never past the TTL at small
		// TTLs (R < TTL/2 ≪ expiry). Client-side pacing only, like the jitter
		// itself: no consensus rule changes.
		if min := c.bondRegHeight[id] + c.regMinInterval(); due < min {
			due = min
		}
	}
	return due
}

// renewalPhaseOffset is a deterministic per-identity offset in [0, window) used
// to spread bond renewals across the TTL window (#555). A pure function of the
// identity, so the pacing is stable and every replica agrees on any node's
// timing (it is not consensus-relevant, but determinism keeps it debuggable).
func renewalPhaseOffset(id ports.NodeID, window uint64) uint64 {
	if window == 0 {
		return 0
	}
	return binary.BigEndian.Uint64(id[:8]) % window
}

// IsSlashed reports whether id has been evicted for a proven equivocation (F2).
func (c *Chain) IsSlashed(id ports.NodeID) bool { return c.slashed[id] }

// CanonicalIssuers returns the objective issuer set for privacy-preserving token
// acquisition (M0 privacy D3 / F4 §2c): the on-chain bonded validators in a
// DETERMINISTIC order — bonded size descending, then NodeID ascending — so every
// publisher asks the SAME validators, and the subset it chose leaks nothing to a
// colluding issuer minority correlating who-asked-whom. Because it reads the
// on-chain bond (not the local reputation view), every replica computes the
// identical set — the same objectivity that heals fork-choice (F6). Returns at
// most max entries (all if max <= 0). Empty when no on-chain bonds are recorded
// (objective mode off, or an immature chain): the caller then falls back to its
// own peer list, which is the pre-D3 behavior.
func (c *Chain) CanonicalIssuers(max int) []ports.NodeID {
	type bonded struct {
		id   ports.NodeID
		size int64
	}
	list := make([]bonded, 0, len(c.bonded))
	for id, size := range c.bonded {
		if size >= c.cfg.MinBond && size > 0 {
			list = append(list, bonded{id, size})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].size != list[j].size {
			return list[i].size > list[j].size // heavier bond first
		}
		return bytesLess(list[i].id[:], list[j].id[:]) // deterministic tiebreak
	})
	if max > 0 && len(list) > max {
		list = list[:max]
	}
	out := make([]ports.NodeID, len(list))
	for i, e := range list {
		out[i] = e.id
	}
	return out
}

// RequireTokens turns on publisher-privacy publish tokens (F1): every entry
// must carry a PublishToken blind-signed by `quorum` distinct qualified
// validators (their issuer keys via issuerKey), and each serial spends exactly
// once (double-spend rejected across the whole chain). Off by default (quorum
// 0) — existing behavior is unchanged, so a Publisher-NodeID entry still works.
func (c *Chain) RequireTokens(quorum int, issuerKey func(ports.NodeID) *rsa.PublicKey) {
	c.tokenQuorum = quorum
	c.issuerKey = issuerKey
}

// Mature reports whether the network has decentralized enough for the launch-
// window anchors to no longer be required: its NAKAMOTO COEFFICIENT over the
// non-anchor bonded set is at least MatureValidators (H4 / Memo 05). Unlike the
// old head-count of distinct validators — which one operator could trip by
// spinning up many minimum bonds, then capture consensus once the wheels shed —
// this is cost-to-corrupt over bond-distinct operators: a set whose weight is
// dominated by a few bonds has a LOW coefficient and stays immature no matter how
// many satellite keys are added. It reads the CURRENT bonded set — the LIVE metric.
// It does NOT gate the anchors: that is the one-way latch EverMature() (F-1), so a
// later drop in decentralization can never re-arm the wheels. Mature() vs
// EverMature() diverge only in the de-maturation window, which the real-bond
// super-quorum handles. Legacy mode has no on-chain bonded set, so it falls back to
// the head count of distinct qualified validators seen.
func (c *Chain) Mature() bool {
	if c.cfg.MatureValidators <= 0 {
		return true
	}
	return c.matureNow()
}

// EverMature reports the one-way maturity LATCH (F-1): whether the network has
// been certified mature at ANY committed height. This — not the live Mature() — is
// what gates the launch anchors, so once a network first decentralizes the anchors
// never re-arm. A pure function of the committed blocks (see the everMature field).
func (c *Chain) EverMature() bool { return c.everMature }

// RegimeState is the derived consensus-regime snapshot a replica holds — the
// state whose live-vs-replay divergence wedged 474718e-deep's val-d (#572: a
// restored replica demanded launch-rule anchors for mature commits, forever).
// Exposed so the daemon can PRINT it at every restore and the next divergence
// names the map that failed to rebuild, instead of hiding behind a validation
// error. Diagnostic only; changes no rule.
type RegimeState struct {
	EverMature, MatureEpoch bool
	ValidatorsSeen, Bonded  int
	EpochStart              uint64
	EpochSetSize            int
}

func (c *Chain) Regime() RegimeState {
	return RegimeState{
		EverMature: c.everMature, MatureEpoch: c.matureEpoch,
		ValidatorsSeen: len(c.validatorsSeen), Bonded: len(c.bonded),
		EpochStart: c.epochStart, EpochSetSize: len(c.epochSet),
	}
}

// matureNow is the LIVE maturity metric over the CURRENT bonded set. Mature()
// wraps it; the latch (everMature) is what gates anchors. The two diverge exactly
// in the de-maturation window (everMature && !matureNow): the network matured, then
// concentration/attrition dropped it back below the bar — handled by the real-bond
// super-quorum, never by re-arming anchors (F-1).
func (c *Chain) matureNow() bool {
	if !c.objective() {
		n := 0
		for id := range c.validatorsSeen {
			if !c.cfg.Anchors[id] {
				n++
			}
		}
		return n >= c.cfg.MatureValidators
	}
	// Objective maturity gates on the OPERATOR-discounted coefficient AND the
	// address-diverse coefficient (A axis, D-C2), whichever is smaller — so a stake
	// split across many keys must clear MatureValidators × M distinct bonds AND
	// MatureValidators distinct declared domains. At M=1 with no domains set this is
	// the plain bond-distinct coefficient (unchanged behavior). min() only ever RAISES
	// the bar to shed, so it can never weaken an existing config.
	return c.MatureCoefficient() >= c.cfg.MatureValidators
}

// MatureCoefficient is the operator-and-domain-distinct bonded-distinctness count over
// the COMMITTED ledger — min(NakamotoOperators, NakamotoDomains) from C2Metric — the
// exact quantity the maturity shed gates on (matureNow). It is the shipped H metric the
// CT-1 conditional theorem measures its honest-arrival count A(t) by (research cert
// C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27, §2.1): the honest
// operator/domain-distinct arrival count at height t. Exposed so an observer can RECORD
// the arrival RATE λ_H = ΔA/Δheight for the floor-exit alarm the cert owes (§6) WITHOUT
// a second, driftable definition of the distinctness metric — the shed and the λ_H floor
// must count the same thing or the floor parameterizes a different quantity than the
// theorem binds (T_mature ≤ M_req / λ_H). Pure read of committed state; changes no rule.
func (c *Chain) MatureCoefficient() int {
	m := c.C2Metric()
	k := m.NakamotoOperators
	if m.NakamotoDomains < k {
		k = m.NakamotoDomains
	}
	return k
}

// C2 is the concentration measurement behind the "no quiet capture" axis (D-C2):
// cost-to-corrupt over bond-distinct participants, computed from the COMMITTED
// on-chain bond ledger (c.bonded) — never gossip, which kills the "lie about your
// size" skew half outright — plus the conservative operator-margin discount that
// stands in for the (impossible-on-chain) key→operator clustering, bounding the
// split half. It is ONE measurement, consumed by the maturity shed (Mature) and
// published for observability (chain-status); the private-lookup committee
// certification (H8/#179) is the third intended consumer.
type C2 struct {
	// NakamotoBonds is the fewest bond-distinct participants whose combined
	// committed weight EXCEEDS the Byzantine fraction (⌊total/3⌋) of the
	// participating weight — the raw cost-to-corrupt coefficient over distinct
	// BONDS (k̂). Weight-aware: a set dominated by one bond yields 1 no matter how
	// many satellite keys are added, so one operator can't cheaply inflate it by size.
	NakamotoBonds int
	// NakamotoOperators is the CONSERVATIVE lower bound on distinct OPERATORS,
	// ⌊NakamotoBonds / M⌋ for operator margin M: since one operator may split a
	// stake across up to ~M keys and on-chain data carries no operator label, the
	// true operator count k* ≥ k̂/M. This is the number the shed actually gates on,
	// so a splitter must clear k·M distinct bonds — the split-half defense. At M=1
	// (default) it equals NakamotoBonds. HEURISTIC by theorem (Kwon): only as tight
	// as M is honest; M_est under adversarial NodeID placement is unquantified
	// (carry margin, D-C2 / #182).
	NakamotoOperators int
	// CostToCorruptBytes is the bonded weight an attacker must control to reach the
	// fault threshold: ⌊total/3⌋ + 1 (one byte past the Byzantine fraction).
	CostToCorruptBytes int64
	// TotalBondedBytes is the participating committed bonded weight the metric is over.
	TotalBondedBytes int64
	// Participants is the number of distinct qualified bonds counted.
	Participants int
	// Margin is the operator margin M the coefficient was discounted by (≥1).
	Margin int
	// NakamotoDomains is the Nakamoto coefficient over ADDRESS-DIVERSE groups (A axis,
	// D-C2): the fewest DISTINCT declared failure-domains whose combined weight exceeds
	// ⌊total/3⌋. Bonds sharing a declared domain aggregate into one group, so a stake
	// split across many keys in one domain does not inflate it — only distinct domains
	// do. With no domains set it equals NakamotoBonds (unchanged). The maturity shed
	// gates on min(NakamotoOperators, NakamotoDomains), so a splitter must clear BOTH
	// k·M distinct bonds AND k distinct domains. Weak signal: a domain is
	// SELF-ASSERTED (declared and gossiped, trusted verbatim — NOT transport-verified
	// against the observed /24), so it prices an equal-/24 split but a splitter that
	// declares distinct domains evades it — pricing, not proof (#182).
	NakamotoDomains int
	// DistinctDomains is the number of address-diversity groups counted (distinct
	// non-zero declared domains + each unset-domain bond as its own group).
	DistinctDomains int
	// HHI is the Herfindahl–Hirschman concentration index over the participating
	// bonds: Σ(share²) ∈ [1/n, 1]. 1 = one bond holds everything; 1/n = perfectly
	// even. A high HHI is the **honest-whale** signal C2 cannot close on-chain (Kwon):
	// surfaced as an out-of-band observability veto — measurement that makes
	// concentration LOUD, never consensus enforcement (D-C2 / F-1 follow-up).
	HHI float64
	// Gini is the Gini coefficient of the bonded-weight distribution ∈ [0,1]:
	// 0 = perfectly even, →1 = one holder. A companion inequality signal to HHI.
	Gini float64
	// TopShare is the largest single bond's fraction of participating weight ∈ [0,1].
	// The most interpretable capture signal: a bond approaching ⅓ is one step from
	// the Byzantine capture fraction (⌊total/3⌋) — the honest-whale alarm threshold.
	TopShare float64
	// WeightUniformity is the evenness of the bonded-weight distribution: the
	// effective participant count (1/HHI, the order-2 Hill number) over the actual
	// count ∈ (0,1]. →1 = every bond identical (perfectly uniform); →1/n = one bond
	// dominates. It is the COUNT/ENTROPY companion the weight-concentration signals
	// (HHI, Gini, TopShare) are BLIND to: an equal-bond SPLIT — one operator posting
	// N identical min-bonds across N keys — drives HHI→1/n, Gini→0, TopShare→1/n
	// (all reading "maximally decentralized") while WeightUniformity→1 with a LARGE
	// Participants count. That "many atoms, implausibly uniform" fingerprint is the
	// naive splitter's tell, invisible to the weight signals (colluding-validator
	// red-team, seam-5). NECESSARY-NOT-SUFFICIENT: healthy decentralization is also
	// uniform, and a splitter that VARIES its bond sizes evades it, so it does NOT
	// close the honest-whale/M_est residue (#182) — it is surfaced so an operator
	// correlating with OUT-OF-BAND address/timing diversity can tell an implausibly
	// perfect split from real decentralization. Observability, never enforcement.
	WeightUniformity float64
}

// C2Metric computes the concentration measurement over the participating,
// qualified, committed bonds — validators that have ATTESTED a committed block
// (validatorsSeen, so a malicious genesis cannot DECLARE a fake-decentralized set)
// AND still hold a live bond ≥ MinBond, minus anchors and slashed keys. It reads
// CURRENT bonds, so a lapsed (TTL) bond drops out and the coefficient can fall —
// re-engaging the wheels (the post-shed escape hatch). Pure function of the
// committed chain state.
func (c *Chain) C2Metric() C2 {
	sizes := make([]int64, 0, len(c.validatorsSeen))
	domainWeight := make(map[uint64]int64) // A axis: non-zero domains aggregated
	var zeroDomainWeights []int64          // domain 0 (unset) → each its own group
	var total int64
	for id := range c.validatorsSeen {
		if c.cfg.Anchors[id] || c.slashed[id] {
			continue
		}
		if sz := c.bonded[id]; sz >= c.cfg.MinBond {
			sizes = append(sizes, sz)
			total += sz
			if d := c.bondDomain[id]; d != 0 {
				domainWeight[d] += sz // same declared domain → one group (no split inflation)
			} else {
				zeroDomainWeights = append(zeroDomainWeights, sz) // unset → independent
			}
		}
	}
	m := C2{TotalBondedBytes: total, Participants: len(sizes), Margin: c.operatorMargin()}
	if total == 0 {
		return m
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] > sizes[j] }) // largest first
	threshold := total / 3
	m.CostToCorruptBytes = threshold + 1
	var cum int64
	m.NakamotoBonds = len(sizes) // all needed unless the loop finds fewer
	for i, sz := range sizes {
		cum += sz
		if cum > threshold {
			m.NakamotoBonds = i + 1
			break
		}
	}
	m.NakamotoOperators = m.NakamotoBonds / m.Margin // ⌊k̂/M⌋, conservative
	// A axis (D-C2): NakamotoDomains is the Nakamoto coefficient over ADDRESS-DIVERSE
	// groups — bonds sharing a declared domain aggregate into one group, so splitting
	// a stake across many keys in ONE domain does NOT inflate the count; only distinct
	// declared domains do (the earned-per-network-position cost the flat margin M only
	// assumes). Domain 0 (unset) is independent, so a chain with no domains set yields
	// NakamotoDomains == NakamotoBonds — behavior identical to a pre-A-axis chain.
	groups := make([]int64, 0, len(domainWeight)+len(zeroDomainWeights))
	for _, w := range domainWeight {
		groups = append(groups, w)
	}
	groups = append(groups, zeroDomainWeights...)
	m.DistinctDomains = len(groups)
	sort.Slice(groups, func(i, j int) bool { return groups[i] > groups[j] })
	var gcum int64
	m.NakamotoDomains = len(groups)
	for i, w := range groups {
		gcum += w
		if gcum > threshold {
			m.NakamotoDomains = i + 1
			break
		}
	}
	// Concentration observability (D-C2 / F-1 follow-up): HHI, Gini, and the top
	// bond's share. C2 cannot CLOSE the honest-whale residue on-chain (Kwon), so this
	// is an out-of-band veto that makes a concentration event LOUD — measurement, not
	// enforcement. sizes is sorted largest-first; ftotal > 0 here.
	n := len(sizes)
	ftotal := float64(total)
	m.TopShare = float64(sizes[0]) / ftotal
	var hhi, giniNum float64
	for i, sz := range sizes {
		share := float64(sz) / ftotal
		hhi += share * share
		// Gini numerator for a DESCENDING-sorted distribution: Σ (n−1−2i)·xᵢ.
		giniNum += float64(n-1-2*i) * float64(sz)
	}
	m.HHI = hhi
	m.Gini = giniNum / (float64(n) * ftotal)
	// WeightUniformity = effective participants (1/HHI, order-2 Hill number) / actual
	// participants. Equal bonds → HHI=1/n → 1/HHI=n → uniformity=1 (the equal-split
	// fingerprint the weight signals read as "decentralized"); concentration pulls it
	// toward 1/n. Companion count/entropy signal (seam-5), observability only.
	if hhi > 0 {
		m.WeightUniformity = (1.0 / hhi) / float64(n)
	}
	return m
}

// operatorMargin is M, the conservative keys-per-operator inflation factor the C2
// coefficient is discounted by. 0/unset resolves to 1 (no discount — the legacy /
// sim / single-operator default), so existing deployments are unaffected; a real
// untrusted deployment sets it > 1 in genesis to demand a splitter clear k·M bonds.
func (c *Chain) operatorMargin() int {
	if c.cfg.OperatorMargin < 1 {
		return 1
	}
	return c.cfg.OperatorMargin
}

// Revoked reports whether root has been taken down by a committed
// revocation record.
func (c *Chain) Revoked(root ports.Hash) bool { return c.revoked[root] }

// The takedown-transparency operations logged as CT events (D-TAKEDOWN / #180).
const (
	RevOp   byte = 'R' // a root was revoked (taken down)
	UnrevOp byte = 'U' // a prior revocation was reversed
)

// RevocationLeaf is the canonical transparency-log entry for one takedown event:
// H("silt/revlog/v1" ‖ op ‖ root ‖ height). Exported so an independent auditor can
// reconstruct the leaf from public block data and check an inclusion proof.
func RevocationLeaf(op byte, root ports.Hash, height uint64) ports.Hash {
	b := make([]byte, 0, 16+1+len(root)+8)
	b = append(b, []byte("silt/revlog/v1")...)
	b = append(b, op)
	b = append(b, root[:]...)
	for i := 0; i < 8; i++ {
		b = append(b, byte(height>>(8*(7-i))))
	}
	return ports.HashBytes(b)
}

// RevocationLogRoot is the current Merkle Tree Head over every honored
// revocation/un-revocation, in commit order — the log's tamper-evident commitment.
func (c *Chain) RevocationLogRoot() ports.Hash { return c.revLog.Root() }

// RevocationLogSize is the number of takedown events logged so far.
func (c *Chain) RevocationLogSize() int { return c.revLog.Size() }

// RevocationLogRootAt is the historical log root at a given event count — what a
// consistency proof is checked against.
func (c *Chain) RevocationLogRootAt(size int) (ports.Hash, error) { return c.revLog.RootAt(size) }

// RevocationInclusionProof proves takedown event `index` is in the log at `size`.
func (c *Chain) RevocationInclusionProof(index, size int) ([]ports.Hash, error) {
	return c.revLog.InclusionProof(index, size)
}

// RevocationConsistencyProof proves the log at size m is a prefix of size n — that
// no logged takedown was ever silently removed or reordered.
func (c *Chain) RevocationConsistencyProof(m, n int) ([]ports.Hash, error) {
	return c.revLog.ConsistencyProof(m, n)
}

// Head returns the current tip hash and the height the NEXT block must
// carry. An empty chain expects height 0 with a zero Prev.
func (c *Chain) Head() (ports.Hash, uint64) {
	if len(c.blocks) == 0 {
		return ports.Hash{}, 0
	}
	last := c.blocks[len(c.blocks)-1]
	return last.Hash(), last.Height + 1
}

func (c *Chain) Len() int { return len(c.blocks) }

// FinalizedHeight is the height this node treats as irreversibly final — the anchor a
// behind peer suffix-syncs FROM (slice 5) and the prune floor derives from. In OBJECTIVE
// mode every committed block is super-quorum-final (the same property the Reconcile
// finality gate rests on — launch: the pinned anchor majority IS the finality quorum;
// mature: the >⅔-frozen-weight commit quorum IS the finality quorum), so the finalized
// head is the committed tip: len(c.blocks)-1. WITHOUT BFT finality (a trusted/demo config)
// there is no immutable anchor, so it returns 0 — sync then falls back to full-genesis
// (ok, ForFinalizedHeight callers request {Height:0}) and nothing prunes (pruneFloor is
// also 0). ok is false when there is no finalized anchor.
func (c *Chain) FinalizedHeight() (height uint64, ok bool) {
	if !c.finalityQuorumActive() || len(c.blocks) == 0 {
		return 0, false
	}
	return uint64(len(c.blocks) - 1), true
}

// Blocks returns the suffix of the chain starting at height from.
func (c *Chain) Blocks(from uint64) []Block {
	if from >= uint64(len(c.blocks)) {
		return nil
	}
	out := make([]Block, len(c.blocks)-int(from))
	copy(out, c.blocks[from:])
	return out
}

// ValidateProposal checks everything an attester must believe before
// signing: ancestry, proposer signature and reputation, and that every
// entry is well-formed and new.
func (c *Chain) ValidateProposal(b *Block) error {
	prev, height := c.Head()
	if b.Height != height || b.Prev != prev {
		return fmt.Errorf("%w: got height %d prev %s, want height %d prev %s",
			ErrWrongParent, b.Height, b.Prev, height, prev)
	}
	if len(b.Proposer) != ed25519.PublicKeySize {
		return ErrBadSignature
	}
	h := b.Hash()
	if !ed25519.Verify(ed25519.PublicKey(b.Proposer), h[:], b.ProposerSig) {
		return fmt.Errorf("%w: proposer", ErrBadSignature)
	}
	if !c.proposerQualifiedAt(b.ProposerID(), b.Height) {
		if c.objective() {
			// Name the ACTUAL disqualifying branch (#572): the 474718e-deep
			// stall printed "bonded 1048576, needs 1048576" — equal-but-failing
			// — because the real refusal was frozen-set membership on a
			// divergent-regime replica, and the bonded/MinBond rendering sent
			// the attribution down a false trail. Mirror proposerQualifiedAt.
			id := b.ProposerID()
			if c.slashed[id] {
				return fmt.Errorf("%w: proposer %s is slashed (F2 eviction)", ErrLowReputation, id)
			}
			if c.epochsEnabled() && c.matureEpoch {
				return fmt.Errorf("%w: proposer %s not in the frozen epoch set governing height %d (mature epoch; bonded %d)",
					ErrLowReputation, id, b.Height, c.bonded[id])
			}
			if len(c.cfg.Anchors) > 0 && !c.handedOff() {
				return fmt.Errorf("%w: proposer %s is not a launch anchor (young network proposes anchor-only, #402; bonded %d)",
					ErrLowReputation, id, c.bonded[id])
			}
			return fmt.Errorf("%w: proposer %s bonded %d, needs %d",
				ErrLowReputation, id, c.bonded[id], c.cfg.MinBond)
		}
		return fmt.Errorf("%w: proposer %s has %d, needs %d",
			ErrLowReputation, b.ProposerID(), c.rep(b.ProposerID()), c.cfg.MinProposerRep)
	}
	if len(b.Entries) == 0 && len(b.Revocations) == 0 && len(b.Unrevocations) == 0 && len(b.BondRegs) == 0 && len(b.Slashes) == 0 {
		return errors.New("chain: empty block")
	}
	if err := c.validateTakedowns(b); err != nil {
		return err
	}
	if err := c.validateBondRegs(b); err != nil {
		return err
	}
	if err := c.validateSlashes(b); err != nil {
		return err
	}
	seen := make(map[ports.Hash]bool)
	seenSerial := make(map[string]bool)
	for _, e := range b.Entries {
		if seen[e.Root] {
			return fmt.Errorf("%w: %s", ErrDupRoot, e.Root)
		}
		if e.Token != nil && seenSerial[string(e.Token.Serial)] {
			return fmt.Errorf("%w: %x", ErrTokenSpent, e.Token.Serial)
		}
		if err := c.ValidateEntry(e); err != nil {
			return err
		}
		if e.Token != nil {
			seenSerial[string(e.Token.Serial)] = true
		}
		seen[e.Root] = true
	}
	// era-3 (v4) version-boundary rule (build step 2c). Checked before the roots
	// predicate so the failure names the version, not a missing root. Extracted into
	// validateEra3Version (era3validity.go) so the OWN-DISK Reload path
	// (appendStructural) runs the identical rule — see that function's doc.
	if err := c.validateEra3Version(b); err != nil {
		return err
	}
	// era-3 (v4) committed-root predicate (build step 2b). A no-op for sub-v4 blocks
	// (era-2 rules unchanged); for a v4 block it rejects a nil root, or a StateRoot/
	// LogRoot that does not equal the post-apply recompute. Placed LAST so a v4 block
	// must first be a valid era-2 block, then additionally satisfy the roots — the
	// additive, strict-superset shape (no era-2 verdict changes). This is the ONE root
	// check site: ValidateCommit calls ValidateProposal first, so the commit path
	// carries it too. See era3validity.go and the 2b deliberation.
	return c.validateEra3Roots(b)
}

// ValidateEntry runs the per-entry checks against the CURRENT chain state —
// dup-root, manifest pointers, the refuse-to-surveil publisher rule, and the
// publish-token verify + chain-wide spent-serial check. Factored out of
// ValidateProposal's loop (byte-identical rules) so the #441 entry mempool can
// validate one peer-submitted entry on arrival and re-validate at fold time —
// a single stale or forged submission never poisons a whole block, exactly the
// reg path's ValidateBondReg discipline. Intra-block dedup (two entries in ONE
// block sharing a root or a serial) stays in ValidateProposal, where the block
// exists.
func (c *Chain) ValidateEntry(e ports.Entry) error {
	if _, exists := c.byRoot[e.Root]; exists {
		return fmt.Errorf("%w: %s", ErrDupRoot, e.Root)
	}
	if len(e.ManifestChunks) == 0 {
		return fmt.Errorf("chain: entry %s has no manifest pointers", e.Root)
	}
	// M0 privacy (#97): a Publisher→root record is permanent on this
	// append-only chain, so the default refuses it. Publish carries no
	// durable identity — a blind-signed token, or nothing — unless the
	// deployment is explicitly trusted (AllowPublisher).
	if !c.cfg.AllowPublisher && e.Publisher != (ports.NodeID{}) {
		return fmt.Errorf("%w: entry %s", ErrPublisherEntry, e.Root)
	}
	if c.tokenQuorum > 0 {
		if e.Token == nil {
			return fmt.Errorf("%w: entry %s", ErrTokenRequired, e.Root)
		}
		// Cheap replay reject BEFORE the RSA work (#183 red-team F-1): a
		// committed token is public on the append-only chain, so an attacker can
		// pair a harvested valid token with a novel Root and flood — every
		// signature is genuine, so publishtoken.Verify would run all N modexps to
		// completion before a spent-check placed after it caught the replay. The
		// serial is an O(1) map lookup and the token is single-use, so a
		// replayed (already-spent) token must fail here, cheaply, not after N
		// verifies. A fresh (unspent) token still pays Verify — but a fresh
		// blind-signed token is issuance-limited, not a free flood amplifier.
		if c.spent[string(e.Token.Serial)] {
			return fmt.Errorf("%w: %x", ErrTokenSpent, e.Token.Serial)
		}
		qualified := func(v ports.NodeID) bool { return c.attesterQualified(v) }
		if err := publishtoken.Verify(*e.Token, c.tokenQuorum, c.issuerKey, qualified); err != nil {
			return fmt.Errorf("chain: entry %s: %w", e.Root, err)
		}
	}
	return nil
}

// validateTakedowns enforces the accountability tenet on a block's revocation
// and un-revocation records (red-team F5): a revocation may only name a root
// this chain has already committed (no censoring content that isn't on the
// ledger), and an un-revocation may only name a root that is currently
// revoked. Called from both the attester pre-check (ValidateProposal) and the
// commit path (validateStructural) so a malicious quorum cannot slip either
// past. Roots published within the SAME block are not yet committed, so
// same-block revoke-what-you-publish is (correctly) refused as nonsensical.
func (c *Chain) validateTakedowns(b *Block) error {
	for _, r := range b.Revocations {
		if _, ok := c.byRoot[r]; !ok {
			return fmt.Errorf("%w: %s", ErrRevokeUnknownRoot, r)
		}
	}
	for _, r := range b.Unrevocations {
		if !c.revoked[r] {
			return fmt.Errorf("%w: %s", ErrUnrevokeNotRevoked, r)
		}
	}
	return nil
}

// ValidateCommit checks a full block under ITS OWN rule era.
//
// Era 1 (legacy): a quorum of distinct, qualified, non-proposer bare-hash
// attestations, then the phase-independent quorum stack.
//
// Era 2 (#432 rounds): TWO quorums at the SAME (Height, CommitRound) — the
// PrepareQC (PhasePrepare) that justified precommitting, and Atts
// (PhasePrecommit), the commit certificate — EACH held to the full quorum
// stack (RequiredQuorum + anchor gate + epoch weight + de-mature), because the
// POL threshold IS the commit threshold (certification §4): at most one value
// can gather a prepare-QC per round (two would share an honest signer), so a
// committed value was uniquely prepared, and any view-change quorum intersects
// its prepare quorum in ≥1 honest carrier. A signature at the wrong phase or
// round is REFUSED (never coerced) — that refusal is what excludes the S1
// delayed-quorum and S2 equivocate-then-misreport schedules.
func (c *Chain) ValidateCommit(b *Block) error {
	if err := c.ValidateProposal(b); err != nil {
		return err // the era-3 (v4) root predicate (step 2b) rides in here, via ValidateProposal
	}
	if b.Version >= BlockVersionRounds {
		if err := c.requireProposerPrepare(b); err != nil {
			return err
		}
		seenPrep, err := c.collectQuorumSigs(b, b.PrepareQC, PhasePrepare, b.CommitRound)
		if err != nil {
			return fmt.Errorf("prepare-QC: %w", err)
		}
		if err := c.requireQuorumStack(b, seenPrep); err != nil {
			return fmt.Errorf("prepare-QC: %w", err)
		}
		seen, err := c.collectQuorumSigs(b, b.Atts, PhasePrecommit, b.CommitRound)
		if err != nil {
			return err
		}
		return c.requireQuorumStack(b, seen)
	}
	// Era 1 (legacy): bare-hash attestations at implicit (r0, PhaseLegacy).
	seen, err := c.collectQuorumSigs(b, b.Atts, PhaseLegacy, 0)
	if err != nil {
		return err
	}
	return c.requireQuorumStack(b, seen)
}

// VerifyPrepareQC reports whether qc is a valid prepare-phase quorum
// certificate for b at round r — the POL the #432 precommit and view-change
// rules lean on. Held to the same thresholds as a commit (certification §4:
// the POL threshold IS the commit threshold, so POL-intersects-commit is the
// same theorem as commit-intersects-commit).
func (c *Chain) VerifyPrepareQC(b *Block, qc []Attestation, round uint64) error {
	seen, err := c.collectQuorumSigs(b, qc, PhasePrepare, round)
	if err != nil {
		return err
	}
	return c.requireQuorumStack(b, seen)
}

// requireProposerPrepare demands the era-2 analogue of the structural
// ProposerSig: a verifying prepare by the block's AUTHOR, at a round ≤ the
// commit round, inside PrepareQC. The bare-hash ProposerSig can never be
// round-attributed (the round is deliberately not in the hash, so re-proposal
// keeps identity — TestRoundNotInBlockIdentity), which is why era 2 excludes
// it from equivocation evidence; without THIS rule a double-proposer that
// withholds its own signatures forks the launch chain with zero slashable
// evidence (it counts toward both anchor quorums by authorship alone) — an
// accountability regression vs era 1 and a hole in I5's "every safety
// violation is attributable". Requiring the round-scoped self-prepare makes
// two proposals at one (h, r) two prepares at (h, r, prepare) over different
// hashes = the existing slash rule.
//
// Round ≤ CommitRound, not ==, is the liveness half: a locked value
// re-proposed at a higher round keeps its ORIGINAL author (possibly down —
// the reason the view changed); its original-round prepare still endorses the
// block (the hash excludes the round) and rides forward in the carried lock
// QC. Count-neutral by construction: collectQuorumSigs skips the author
// before its round-exactness check, so this signature is exempt from the
// per-phase round rule and adds nothing to any quorum count.
func (c *Chain) requireProposerPrepare(b *Block) error {
	h := b.Hash()
	for _, a := range b.PrepareQC {
		if a.Phase == PhasePrepare && a.Round <= b.CommitRound &&
			bytes.Equal(a.PubKey, b.Proposer) && verifyAtt(a, h) {
			return nil
		}
	}
	return ErrProposerPrepare
}

// collectQuorumSigs verifies one phase's signature set for b: every signature
// must be genuine, at EXACTLY the demanded (phase, round), from a distinct
// non-proposer identity; unqualified identities are ignored (not fatal), a bad
// or wrong-scope signature is fatal (a block carrying one is malformed —
// refusing it is what keeps cross-round/cross-phase replay out of quorums).
// The AUTHOR's own signature is skipped BEFORE the scope check — it counts
// toward no quorum (the proposer is counted by authorship: countAnchorSupport
// / requireEpochWeightQuorum) and may legitimately sit at a LOWER round than
// the certificate (a carried self-prepare — see requireProposerPrepare).
func (c *Chain) collectQuorumSigs(b *Block, sigs []Attestation, phase uint8, round uint64) (map[ports.NodeID]bool, error) {
	h := b.Hash()
	seen := make(map[ports.NodeID]bool)
	for _, a := range sigs {
		if len(a.PubKey) != ed25519.PublicKeySize {
			continue
		}
		id := a.AttesterID()
		if seen[id] || id == b.ProposerID() {
			continue // duplicates and self-attestation don't count
		}
		if a.Phase != phase || a.Round != round {
			return nil, fmt.Errorf("%w: attester %s signed (phase %d, round %d), this quorum demands (phase %d, round %d)",
				ErrBadSignature, id, a.Phase, a.Round, phase, round)
		}
		if !verifyAtt(a, h) {
			return nil, fmt.Errorf("%w: attester %s", ErrBadSignature, id)
		}
		if !c.attesterQualifiedAt(id, b.Height) {
			continue // unqualified signatures are ignored, not fatal
		}
		seen[id] = true
	}
	return seen, nil
}

// requireQuorumStack applies the phase-independent quorum requirements to one
// verified signer set: the count quorum, then the regime gates below (anchor
// majority / epoch weight / de-mature super-quorum). Shared by the era-1
// commit, the era-2 prepare-QC, and the era-2 precommit certificate, so the
// three can never drift (the #402 share-the-arithmetic lesson).
func (c *Chain) requireQuorumStack(b *Block, seen map[ports.NodeID]bool) error {
	if req := c.RequiredQuorum(); len(seen) < req {
		return fmt.Errorf("%w: %d qualified, need %d", ErrNoQuorum, len(seen), req)
	}
	// Training wheels: until the network has HANDED OFF, the quorum must ALSO
	// carry anchor sign-off, so a Sybil quorum can't capture a young network
	// before it has decentralized. Gated on the one-way handoff (handedOff —
	// with epochs, the first mature rotation per #357 Condition B; without, the
	// everMature latch), NOT the live Mature() — so a later drop in
	// decentralization (e.g. an honest whale concentrating real bond) can never
	// re-arm the anchors (F-1). The anchors therefore keep their sign-off duty
	// through the (≤ EpochBlocks) tail between the latch and the boundary —
	// coherent with launchAnchor keeping them eligible over the same window.
	// Once handed off, de-maturation liveness is the real-bond super-quorum
	// (requireDeMatureSuperQuorum), not this.
	// Launch anchor gate (#402): a strict anchor majority, derived in objective mode
	// so config can never disable intersection. requiredLaunchAnchors + countAnchorSupport
	// are shared with SupportMeetsQuorum, so the proposer's gather stops on EXACTLY what
	// this validation demands (no under-gather → self-Append failure drift).
	if need := c.requiredLaunchAnchors(); need > 0 {
		if got := c.countAnchorSupport(b.ProposerID(), seen); got < need {
			return fmt.Errorf("%w: %d of required %d", ErrAnchorRequired, got, need)
		}
	}
	// Mature-epoch WEIGHT quorum (research certification 2026-08-13, B2): once
	// the network has handed off, the Byzantine super-majority is counted in
	// FROZEN EPOCH BONDED WEIGHT, never in heads. Membership admission is
	// unfiltered (every qualified bond becomes an epoch member at rotation), so
	// a head-counted threshold handed a MinBond-per-head cohort both a stall
	// lever (ride the snapshot, decline to attest — nothing slashable) and,
	// one head past bftThreshold, outright capture (a cheap-member majority
	// committing with zero honest attestation). Weight-counting prices both at
	// what C1 says they must cost: >⅓ (stall) / >⅔ (capture) of the epoch's
	// REAL bonded weight. This also makes the quorum consistent with fork-choice
	// (blockWeight already sums the same frozen snapshot weights).
	if c.cfg.ByzantineQuorum && c.objective() && c.epochsEnabled() && c.matureEpoch {
		if err := c.requireEpochWeightQuorum(b.ProposerID(), seen, b.Height); err != nil {
			return err
		}
	}
	// De-maturation super-quorum (F-1, ships WITH the latch): once matured, the
	// anchors never re-arm — but if live decentralization has since dropped below the
	// bar (everMature && !matureNow, e.g. an honest whale concentrated real bond or
	// small bonds lapsed), a commit instead needs a real-bond SUPER-MAJORITY: ≥⅔ of
	// the LIVE bonded weight, no anchor sign-off. This is the center-less replacement
	// for the retired anchor net — it keeps liveness for a genuinely-willing real
	// quorum (the HALT horn stays dead) and preserves accountable safety (any two ⅔
	// super-quorums share > ⅓ of the weight, so they intersect in honest bond). In
	// the normal mature-and-still-decentralized case this is a no-op.
	if c.everMature && c.objective() && !c.matureNow() {
		if err := c.requireDeMatureSuperQuorum(b, seen); err != nil {
			return err
		}
	}
	return nil
}

// requireEpochWeightQuorum is the mature-phase Byzantine super-majority, counted
// in weight (B2): the support coalition — proposer + the distinct qualified
// attesters `seen` — must carry STRICTLY MORE THAN ⅔ of the frozen epoch's total
// bonded weight (Σ epochSet). Any two such coalitions overlap in > ⅓ of the
// weight, so with < ⅓ of the WEIGHT Byzantine the overlap contains honest bond —
// the weighted analogue of bftThreshold's count intersection, and the settled
// pattern (Tendermint voting power, Casper FFG ⅔-of-stake; B8: adopt, don't
// invent). Non-members contribute zero (attesterQualified already gated
// membership), so a cheap-member cohort weighs exactly what it paid. A pure
// function of the frozen snapshot — every replica agrees within the epoch.
func (c *Chain) requireEpochWeightQuorum(proposer ports.NodeID, seen map[ports.NodeID]bool, h uint64) error {
	// The governing set for h: the frozen snapshot everywhere but the #535
	// recovery boundary (effectiveEpochSet) — the SAME set the attester filter
	// admits from, so sizing-set == membership-set (the #402 law).
	set := c.effectiveEpochSet(h)
	var total int64
	for _, w := range set {
		total += w
	}
	if total <= 0 {
		return nil // no frozen weight to measure against (degenerate/trusted)
	}
	support := set[proposer]
	for id := range seen {
		support += set[id]
	}
	if 3*support <= 2*total {
		return fmt.Errorf("%w: coalition holds %d of %d bonded weight (need >%d)",
			ErrNoQuorumWeight, support, total, 2*total/3)
	}
	return nil
}

// RoundCatchupMet is the #451 view-synchronizer's catch-up threshold
// (certification §2b, adopted from PBFT's responsive f+1 view-change): the
// smallest set of round-change senders that PROVES at least one honest member
// is ahead, so a straggler may safely jump to their round. Mature epoch:
// >⅓ of the frozen weight (one Byzantine third cannot fake it). Launch:
// f+1 of the anchor set (f = ⌊(A−1)/3⌋). Adversary-robust both ways: a lone
// Byzantine can neither DRAG honest nodes to a fabricated round (the
// threshold needs an honest member) nor stall progress (ingredient (a), the
// increasing round duration, guarantees advance regardless).
func (c *Chain) RoundCatchupMet(senders map[ports.NodeID]bool) bool {
	if !c.objective() {
		return false
	}
	if c.epochsEnabled() && c.matureEpoch {
		var total, support int64
		for _, w := range c.epochSet {
			total += w
		}
		if total <= 0 {
			return false
		}
		for id := range senders {
			support += c.epochSet[id]
		}
		return 3*support > total
	}
	a := len(c.cfg.Anchors)
	if a == 0 {
		return false
	}
	f := (a - 1) / 3
	n := 0
	for id := range senders {
		if c.cfg.Anchors[id] || c.bonded[id] >= c.cfg.MinBond {
			n++
		}
	}
	return n >= f+1
}

// SupportMeetsQuorum reports whether a commit at height h proposed by
// `proposer` and attested by `attesters` would clear ValidateCommit's quorum:
// the distinct qualified non-proposer count floor (RequiredQuorum) plus, in a
// mature epoch, the weight super-majority over the set governing h
// (requireEpochWeightQuorum — the frozen snapshot everywhere but the #535
// recovery boundary). Exposed so a proposer's gather loop can stop asking
// exactly when the coalition it holds would commit — under weight counting,
// "how many attestations" is no longer the question; "whose" is. h is the
// height of the block being gathered, so the gather and the validation judge
// the SAME governing set.
func (c *Chain) SupportMeetsQuorum(proposer ports.NodeID, attesters []ports.NodeID, h uint64) bool {
	seen := make(map[ports.NodeID]bool, len(attesters))
	for _, id := range attesters {
		if id == proposer || seen[id] || !c.attesterQualifiedAt(id, h) {
			continue
		}
		seen[id] = true
	}
	if len(seen) < c.RequiredQuorum() {
		return false
	}
	// The launch anchor gate (#402): the coalition must carry the strict anchor
	// majority ValidateCommit will demand, so the gather stops on enough ANCHORS, not
	// merely enough heads — otherwise the proposer commits-attempts a count-quorum that
	// its own Append then rejects (ErrAnchorRequired).
	if need := c.requiredLaunchAnchors(); need > 0 && c.countAnchorSupport(proposer, seen) < need {
		return false
	}
	if c.cfg.ByzantineQuorum && c.objective() && c.epochsEnabled() && c.matureEpoch {
		return c.requireEpochWeightQuorum(proposer, seen, h) == nil
	}
	return true
}

// requireDeMatureSuperQuorum enforces the F-1 de-maturation rule: the committing
// coalition (proposer + the distinct qualified attesters `seen`) must control ≥⅔ of
// the live bonded weight. Only real committed bond counts (in the de-maturation
// window launchAnchor is false, so `seen` is bonded validators only). A pure function
// of the committed bonded set, so every replica agrees.
func (c *Chain) requireDeMatureSuperQuorum(b *Block, seen map[ports.NodeID]bool) error {
	var total int64
	for _, w := range c.bonded {
		total += w
	}
	if total <= 0 {
		return nil // no bonded weight to measure against (nothing to protect)
	}
	committed := c.bonded[b.ProposerID()]
	for id := range seen {
		committed += c.bonded[id]
	}
	need := (2*total + 2) / 3 // ⌈2·total/3⌉
	if committed < need {
		return fmt.Errorf("%w: coalition holds %d MiB of %d MiB bonded (need ≥%d MiB)",
			ErrDeMatureQuorum, committed>>20, total>>20, need>>20)
	}
	return nil
}

// Append validates and applies a committed block.
func (c *Chain) Append(b Block) error {
	if err := c.ValidateCommit(&b); err != nil {
		return err
	}
	c.apply(b)
	return nil
}

// Reload rebuilds this replica from THIS node's OWN persisted history — the
// genesis first, then each committed block — re-verifying every block's
// cryptographic integrity but NOT the live reputation gate (see
// appendStructural for why). It is how a restarted validator rejoins at its
// persisted height instead of being stranded at genesis by an empty reputation
// view (F1). Returns how many blocks were restored. A PEER's chain is a
// different trust class and still goes through Reconcile, which re-validates
// reputation in full — Reload is only ever fed our own disk.
func (c *Chain) Reload(blocks []Block) (int, error) {
	// #572 guard: an objective-config replica MUST NOT replay history before
	// its bond verifier is wired. objective() is MinBond>0 AND verifyBond!=nil;
	// replaying with a nil verifier silently demotes every qualification check
	// to the LEGACY rep-gated path (empty at boot) — validatorsSeen rebuilds
	// EMPTY, the everMature latch is lost, and the restored validator refuses
	// every mature-regime commit forever (the 474718e-deep restore wedge,
	// proven by the save/restore regime pairs). Refuse loudly instead.
	if len(blocks) > 0 && c.cfg.MinBond > 0 && c.verifyBond == nil {
		return 0, errors.New("chain: objective config (MinBond>0) replayed with NO bond verifier — wire SetBondVerifier before Reload, or the maturity latch is silently lost (#572)")
	}
	for i, b := range blocks {
		var err error
		if i == 0 && b.Height == 0 {
			err = c.AppendGenesis(b)
		} else {
			err = c.appendStructural(b)
		}
		if err != nil {
			return i, err
		}
	}
	return len(blocks), nil
}

// appendStructural re-applies a block from our own persisted history, verifying
// ancestry, the proposer signature, and a quorum of distinct, verifying,
// non-proposer attester signatures — everything a corrupt disk could break —
// but deliberately NOT the reputation gate. Reputation is a live, local,
// time-varying view that is EMPTY at boot (bond audits have not run yet); it is
// not an integrity property of the block, and re-gating our own already-
// committed history on it would strand a restarted validator at genesis (F1).
// Because the proposer and attester signatures cover the whole block hash, any
// tampering, bit-rot, or truncation is still caught (B7 — persisted state is
// re-verified on load, not trusted). What we skip is re-litigating a policy
// decision — proposer/attester qualification, publish-token and Publisher
// policy — that the quorum already made when this node committed the block.
func (c *Chain) appendStructural(b Block) error {
	if err := c.validateStructural(&b); err != nil {
		return err
	}
	// era-3 (v4) version-boundary rule on the OWN-DISK reload path (build step 2c
	// symmetry). The commit path enforces ErrEra3VersionRequired via ValidateProposal;
	// this own-disk path did not call it, so a v2 block at/above H_era3 could be
	// persisted here. Not exploitable today (a valid quorum-signed v2 block cannot exist
	// at/above H_era3 — it committed via ValidateCommit, which rejects it), but the two
	// era-3 consensus-entry rules must be symmetric across write paths, exactly as the
	// root check below now is. A future unguarded disk-write path (fast-sync/import) is
	// precisely where the asymmetry could turn into a hole. This is a PURE HEADER CHECK
	// against era3Active (derived from PRIOR committed state, available BEFORE apply), so
	// it runs BEFORE c.apply(b): a rejected block is not left applied, preserving the
	// longest-valid-prefix contract (same discipline as the root check).
	if err := c.validateEra3Version(&b); err != nil {
		return err
	}
	// era-3 (v4) committed-root re-validation on the OWN-DISK reload path (A-bare).
	//
	// validateStructural verifies the proposer/attester signatures, which cover the
	// block Hash (StateRoot/LogRoot included). But a signature covering a root proves
	// only that the signer committed to THAT byte string — it does NOT prove the root
	// equals the post-apply state. A v4 block whose StateRoot is wrong but is re-signed
	// with the proposer key passes every signature check here. Integrity ≠ root-
	// correctness. The commit paths (Append→ValidateCommit→ValidateProposal→
	// validateEra3Roots) enforce the equality; this own-disk path skipped it, so a
	// corrupt/tampered v4 disk block was accepted with an UNENFORCED root.
	//
	// The check runs BEFORE apply, on the same #558-guarded dry-run clone the commit
	// path uses (validateEra3Roots → postApplyRoots), NOT after apply on live state. A
	// post-apply-then-reject would leave the bad block applied to the live chain — head
	// advanced, byRoot mutated — which breaks Reload's load-bearing "keep the longest
	// VALID prefix" contract (cmd/silt/daemon.go: a replay failure continues with the
	// prefix chain, so a partially-applied bad block would poison a running node). The
	// clone recompute yields the identical verdict at O(state)/block with no live
	// mutation on rejection. The named errors are shared with the commit path so
	// proposer, validator, and reload agree or all fail.
	if err := c.validateEra3Roots(&b); err != nil {
		return err
	}
	c.apply(b)
	return nil
}

func (c *Chain) validateStructural(b *Block) error {
	prev, height := c.Head()
	if b.Height != height || b.Prev != prev {
		return fmt.Errorf("%w: got height %d prev %s, want height %d prev %s",
			ErrWrongParent, b.Height, b.Prev, height, prev)
	}
	if len(b.Proposer) != ed25519.PublicKeySize {
		return ErrBadSignature
	}
	h := b.Hash()
	if !ed25519.Verify(ed25519.PublicKey(b.Proposer), h[:], b.ProposerSig) {
		return fmt.Errorf("%w: proposer", ErrBadSignature)
	}
	if len(b.Entries) == 0 && len(b.Revocations) == 0 && len(b.Unrevocations) == 0 && len(b.BondRegs) == 0 && len(b.Slashes) == 0 {
		return errors.New("chain: empty block")
	}
	if err := c.validateTakedowns(b); err != nil {
		return err
	}
	if err := c.validateSlashes(b); err != nil {
		return err
	}
	seen := make(map[ports.NodeID]bool)
	valid := 0
	for _, a := range b.Atts {
		if len(a.PubKey) != ed25519.PublicKeySize {
			continue
		}
		id := a.AttesterID()
		if seen[id] || id == b.ProposerID() {
			continue // duplicates and self-attestation don't count
		}
		// Era-aware (#558): era-2 attestations sign the domain-separated
		// consensusSigBytes(phase, round, hash), not the bare hash. This path
		// verified the bare hash only, so replay of ANY era-2 chain failed at
		// its first non-genesis block and the daemon silently fell to genesis —
		// masked by peer full-fetch until the retention prune removed the mask
		// (the a434494-deep val-d stranding). verifyAtt is the same arithmetic
		// the live commit path uses.
		if !verifyAtt(a, h) {
			return fmt.Errorf("%w: attester %s", ErrBadSignature, id)
		}
		seen[id] = true
		valid++
	}
	if valid < c.cfg.Quorum {
		return fmt.Errorf("%w: %d valid, need %d", ErrNoQuorum, valid, c.cfg.Quorum)
	}
	return nil
}

// AppendGenesis seeds the height-0 founding block. Unlike every later
// block it needs NO quorum and NO proposer reputation — a genesis is
// accepted because it is identical on every node (declared, not agreed),
// exactly as Bitcoin's genesis has no predecessor to prove work against.
// It must be the first block, at height 0 with a zero parent, and its
// proposer signature must check out (so a corrupted genesis is caught).
func (c *Chain) AppendGenesis(b Block) error {
	if len(c.blocks) != 0 {
		return fmt.Errorf("chain: genesis must be the first block")
	}
	if b.Height != 0 || b.Prev != (ports.Hash{}) {
		return fmt.Errorf("chain: malformed genesis (height %d, non-zero prev?)", b.Height)
	}
	if len(b.Proposer) != ed25519.PublicKeySize {
		return ErrBadSignature
	}
	h := b.Hash()
	if !ed25519.Verify(ed25519.PublicKey(b.Proposer), h[:], b.ProposerSig) {
		return fmt.Errorf("%w: genesis proposer", ErrBadSignature)
	}
	if len(b.Entries) == 0 {
		return fmt.Errorf("chain: empty genesis")
	}
	// A genesis seeds entries (and declared launch bonds) — never takedowns.
	// AppendGenesis skips validateTakedowns (there is no prior history to check a
	// revocation against), so allowing Revocations here would let whoever controls
	// genesis PRE-EMPTIVELY revoke a never-published root, exactly what immutable
	// #5 forbids (red-team F3). Reject them outright; a takedown must go through
	// the governed normal path, where ErrRevokeUnknownRoot enforces existence.
	if len(b.Revocations) > 0 || len(b.Unrevocations) > 0 {
		return ErrGenesisTakedown
	}
	// The same door, for the STRONGER lever (retest G1). AppendGenesis skips
	// validateSlashes, and apply() unconditionally evicts every Slashes culprit
	// (slashed[id]=true, deleted from bonded, barred from re-earning, carried
	// through adopt()). A genesis carrying an UNVERIFIED Slash would therefore be
	// a proof-free, pre-emptive, identity-level kill switch — a fortiori what
	// immutable #5 forbids. A slash is only meaningful against equivocation
	// WITHIN this chain's history, of which a genesis has none, so genesis may
	// never carry one. Reject outright; a slash must go through the normal path,
	// where validateSlashes → VerifyEquivocation gates it on a real proof.
	if len(b.Slashes) > 0 {
		return ErrGenesisTakedown
	}
	// NAMED PREMISE (residual R-G, era-3 freeze coupling): AppendGenesis does NOT
	// run validateBondRegs, so the #618 seenRoot per-root distinct-ID dedup does
	// NOT cover genesis. Genesis apply() IS order-dependent for two distinct-ID
	// UNPROVEN same-root regs (apply() below, proven=false). This is safe ONLY
	// because the production genesis is a byte-identical shared constant carrying
	// NO BondRegs (genesis.Build → core/genesis/genesis.go:79, Entries only), so
	// there is no per-node slice order to diverge on. The era-3 SMT freeze's
	// unconditional order-independence claim leans on THIS premise, not on a guard.
	// The premise is pinned by TestGenesisSameRootApplyIsOrderDependent (this
	// package) + TestProductionGenesisCarriesNoBondRegs (core/genesis). Making
	// genesis order-independent BY REJECTION would be a consensus-rule change to
	// genesis validity (research-gated) — see
	// docs/thinking/2026-08-28-genesis-sameroot-residual.md option (b).
	c.apply(b)
	return nil
}

func (c *Chain) apply(b Block) {
	c.blocks = append(c.blocks, b)
	for _, e := range b.Entries {
		c.byRoot[e.Root] = e
		if e.Token != nil {
			c.spent[string(e.Token.Serial)] = true // serial is now spent chain-wide
		}
	}
	for _, r := range b.Revocations {
		c.revoked[r] = true
		c.revLog.Append(RevocationLeaf(RevOp, r, b.Height)) // append-only transparency record
	}
	// Un-revocations clear a prior takedown (validated as currently-revoked).
	// delete rather than set-false so the map stays a clean set and adopt()'s
	// pure-replay rebuild yields identical state.
	for _, r := range b.Unrevocations {
		delete(c.revoked, r)
		c.revLog.Append(RevocationLeaf(UnrevOp, r, b.Height)) // the reversal is logged too
	}
	// Record on-chain bond registrations (objective validator set, F6). A
	// height>0 registration was already VERIFIED by validateBondRegs (real
	// space-time proof); a genesis (height 0) registration is merely DECLARED.
	// The latest registration wins, so a validator can renew or resize.
	// PER-ROOT DEDUP (red-team F1): a bond Root credits AT MOST ONE identity — the
	// first to claim it. A later registration on an already-claimed root by a
	// DIFFERENT identity earns nothing, so a colluding operator cannot back N
	// Sybil standings off one shared plot. The first owner may re-register (renew
	// or resize) its own root freely.
	proven := b.Height > 0 // genesis regs are declared; height>0 went through validateBondRegs
	// CONSENSUS-RULE (canonicalize same-id intra-block regs, cert
	// sameid-twoversion-intrablock-bondreg-contention 2026-08-28): a block may carry
	// more than one BondReg for the SAME validator id (a legal F1 renew/resize, or a
	// Byzantine same-id-two-version block admissible pre-#506-gate). The old loop
	// resolved these LAST-WRITER-WINS by slice position, so regVersion/bondDomain/
	// bonded committed an order-dependent value → an order-dependent history-
	// independent SMT root (a latent fork; regVersion also feeds the #506 lock-in
	// tally, so gateLockedIn/gateHeight inherited the split). Fold to ONE canonical
	// winner per id by a TOTAL ORDER that is a pure function of content — largest
	// Size, then Version, then Domain, then Sig — and apply ALL of that winner's
	// fields. The result is identical however the proposer ordered the slice. Reject
	// was refuted (it breaks the legal resize); canonicalize is the certified fix.
	// The winner is applied in a deterministic id order so the ownership/displacement
	// writes (bondRootOwner/bondRootProven) are order-free too.
	for _, r := range canonicalBondRegs(b.BondRegs) {
		if len(r.Validator) != ed25519.PublicKeySize {
			continue
		}
		if r.Size < c.cfg.MinBondBytes {
			continue // below the objective anti-release floor → no standing (retest G4)
		}
		id := r.ValidatorID()
		if c.slashed[id] {
			continue // a slashed equivocator cannot re-earn bonded standing (F2)
		}
		if owner, claimed := c.bondRootOwner[r.Root]; claimed && owner != id {
			// PROOF BEATS DECLARATION (retest G3): a verified registration displaces
			// an unproven genesis-DECLARED claim, so a malicious genesis cannot
			// pre-squat an honest validator's real root and lock the true holder out
			// via the dedup above. Any other collision (proven-vs-proven, or an
			// unproven challenger) earns nothing, preserving F1.
			if !(proven && !c.bondRootProven[r.Root]) {
				continue // shared root already backs another identity → no standing
			}
			delete(c.bonded, owner) // strip the displaced squatter's unproven standing
		}
		c.bondRootOwner[r.Root] = id
		if proven {
			c.bondRootProven[r.Root] = true
		}
		c.bonded[id] = r.Size
		c.bondRegHeight[id] = b.Height // reset the TTL clock on every (re)registration (G4)
		c.regVersion[id] = r.Version   // #506 readiness signal; latest committed reg governs
		c.bondDomain[id] = r.Domain    // committed A-axis label (0 = unset); latest wins
	}
	// OBJECTIVE RE-CHALLENGE (retest G4): standing lapses if not renewed with a
	// fresh proof within BondTTLBlocks. A validator that registers once and then
	// releases its plot cannot answer the fresh challenge to renew, so its vote
	// decays to nothing instead of persisting forever off a single one-time proof.
	// Height-driven, so every replica expires standing in lockstep.
	if ttl := c.cfg.BondTTLBlocks; ttl > 0 {
		for id, regH := range c.bondRegHeight {
			if b.Height-regH > ttl {
				delete(c.bonded, id)
				delete(c.bondRegHeight, id)
				delete(c.regVersion, id) // a lapsed bond's readiness signal lapses with it
			}
		}
	}
	// Apply on-chain equivocation slashes (F2): evict the culprit from the
	// objective bonded set and bar it from re-earning standing. Verified already
	// (validateSlashes) on the write paths; genesis slashes are declared.
	for i := range b.Slashes {
		culprit := b.Slashes[i].CulpritID()
		c.slashed[culprit] = true
		delete(c.bonded, culprit)
	}
	// Track distinct qualified validators for the maturity metric — a
	// monotonic, chain-internal, auditable measure of decentralization.
	for _, a := range b.Atts {
		id := a.AttesterID()
		if id != b.ProposerID() && c.attesterQualified(id) {
			c.validatorsSeen[id] = true
		}
	}
	// Latch maturity (F-1): once the network is first certified mature, record it
	// permanently, so the launch anchors never re-arm. Checked AFTER this block's
	// bonds/slashes/TTL are applied, so it reflects the post-block bonded set.
	// Monotonic — only ever set, never cleared.
	if !c.everMature && c.Mature() {
		c.everMature = true
	}
	// Epoch rotation (#357 Conditions A+B), LAST — after this block's bonds, TTL
	// expiries, slashes, and the maturity latch — so a boundary block that also
	// trips maturity hands off in the same commit. The boundary block itself was
	// validated under the OUTGOING epoch's set/quorum; the new snapshot governs
	// from the next block. Under the §3 finality gate a committed boundary block
	// is super-quorum-final, so every rotation — including the young→mature
	// handoff, which is simply the first mature rotation — happens at a finalized
	// checkpoint. Deterministic (a function of height and committed state), so
	// every replica rotates in lockstep.
	if c.epochsEnabled() && b.Height%c.cfg.EpochBlocks == 0 {
		c.rotateEpoch(b.Height)
	}
}

// canonicalBondRegs folds a block's BondRegs to at most one reg per validator id,
// choosing the SAME-ID winner by a TOTAL ORDER on content, and returns one reg per
// id in FIRST-APPEARANCE order. It is the ordering-canonicalization half of the SMR
// block-determinism requirement: same-id multi-reg is legitimate content (a
// renew/resize), so it is CANONICALIZED rather than rejected (the #618 sibling
// rejected distinct-id same-root content, which has no legitimate form). See the
// CONSENSUS-RULE note in apply() and the cert
// sameid-twoversion-intrablock-bondreg-contention 2026-08-28.
//
// The winner rule is largest Size, then Version, then Domain, then Sig bytes — a
// strict total order that is a pure function of the reg's content, independent of
// slice position. "Largest size wins" makes a resize monotone: the intended larger
// registration takes regardless of which order the proposer listed the regs, which
// is the right renew/resize semantics (and keeps TestSameRootSameIDRenewAdmitted
// green — reg2's 2S wins in both orderings).
//
// SCOPE (deliberate, cert §3(a1) + the residual R-G premise): this fold ONLY collapses
// SAME-ID multi-reg. It does NOT re-order distinct ids — the winners are emitted in the
// order their id first appears in the slice. Sorting the winners by id would ALSO make
// the DISTINCT-ID same-root case order-independent, which is a genuinely different rule:
// #618 rejects that collision at height>0 validity, and at genesis it is the intentionally
// order-dependent residual R-G premise (TestGenesisSameRootApplyIsOrderDependent). Both
// are OUT of this cert's scope and research-gated; touching them silently would break a
// named premise. Preserving first-appearance order keeps the distinct-id ownership-branch
// behavior byte-for-byte identical to the pre-fold loop.
func canonicalBondRegs(regs []BondReg) []BondReg {
	if len(regs) <= 1 {
		return regs
	}
	winners := make(map[ports.NodeID]int, len(regs)) // id → index into out
	out := make([]BondReg, 0, len(regs))
	for _, r := range regs {
		if len(r.Validator) != ed25519.PublicKeySize {
			continue // malformed regs are dropped by apply()'s own guard; skip here too
		}
		id := r.ValidatorID()
		if idx, seen := winners[id]; seen {
			if bondRegLess(out[idx], r) { // r beats the incumbent under the total order
				out[idx] = r // canonical winner replaces IN PLACE — first-appearance slot kept
			}
			continue
		}
		winners[id] = len(out)
		out = append(out, r)
	}
	return out
}

// bondRegLess reports whether a ranks BELOW b under the canonical same-id winner
// order: largest Size, then Version, then Domain, then Sig bytes. All four keys are
// content, so the order is a pure function of the reg and does not depend on slice
// position. The last key (Sig) makes the order TOTAL — two regs identical in
// Size/Version/Domain still have a deterministic winner, so there is never a tie the
// slice order could break.
func bondRegLess(a, b BondReg) bool {
	if a.Size != b.Size {
		return a.Size < b.Size
	}
	if a.Version != b.Version {
		return a.Version < b.Version
	}
	if a.Domain != b.Domain {
		return a.Domain < b.Domain
	}
	return bytes.Compare(a.Sig, b.Sig) < 0
}

// rotateEpoch begins a new epoch at boundary height h. During the launch phase
// (pre-latch) there is nothing to snapshot — the fixed anchor set governs and
// bonds accrue live toward maturity. The first rotation at-or-after the
// everMature latch is the HANDOFF (#357 Condition B): matureEpoch sets one-way,
// and from then on every rotation freezes the qualified committed bonded set —
// membership and size — as the epoch's consensus set (#357 Condition A).
func (c *Chain) rotateEpoch(h uint64) {
	c.epochStart = h
	if !c.everMature {
		return
	}
	c.matureEpoch = true
	set := c.liveQualifiedSet()
	c.epochSet = set

	// #506 lock-in detection (post-latch path; the pre-latch genesis override
	// bypasses signalling entirely). At each boundary, tally the frozen set's
	// rule-aware WEIGHT — weight, never heads, for the same C1/C2 reason the
	// commit quorum is weight-counted (requireEpochWeightQuorum): a cheap-bond
	// cohort must not be able to fake-signal an activation. Lock in the first
	// time it clears the SAME >⅔ super-quorum the finality rule uses; enforce
	// from the NEXT boundary (one finalized epoch of notice; rule changes, like
	// set changes, integrate only at rotations — certification Q1.3/Q1.4).
	// Byzantine signal-inflation is absorbed by the shared threshold: with ≤ f
	// falsely signalling, honest-enforcing weight still exceeds what any storm
	// coalition can gather (certification Q2). Monotonic by the guard.
	if !c.gateLockedIn && c.cfg.RegGateActivationHeight == 0 && c.cfg.EpochBlocks > 0 {
		var total, ready int64
		for id, w := range set {
			total += w
			if c.regVersion[id] >= BlockVersionRegGate {
				ready += w
			}
		}
		if total > 0 && 3*ready > 2*total {
			c.gateLockedIn = true
			c.gateHeight = h + c.cfg.EpochBlocks
		}
	}

	// era-3 (v4) activation lock-in — build step 2c, the #506 tally reused one
	// readiness level up (research cert Q5/Q7). Same frozen-set weight, same >⅔
	// super-quorum, same one-epoch-of-notice and monotonic guard — the ONLY
	// difference is the readiness threshold: regVersion >= BlockVersionStateRoot
	// (== 4), a DISTINCT signal from the #506 gate's >= 3, because a node signals 4
	// only when it can enforce the R-rule AND validate committed roots (cert Q7).
	// Byzantine signal-inflation is absorbed by the shared threshold, identical to
	// #506. Separate from the gate tally above (a node may know the R-rule long
	// before it has the era-3 root software), so the two lock in independently.
	if !c.era3LockedIn && c.cfg.Era3ActivationHeight == 0 && c.cfg.EpochBlocks > 0 {
		var total, ready int64
		for id, w := range set {
			total += w
			if c.regVersion[id] >= BlockVersionStateRoot {
				ready += w
			}
		}
		if total > 0 && 3*ready > 2*total {
			c.era3LockedIn = true
			c.era3Height = h + c.cfg.EpochBlocks
		}
	}
}

// era3Active reports whether the era-3 (v4) committed-state-root format governs a
// block at height h: at/past the genesis-declared boundary (pre-latch override),
// or at/past the chain-derived H_era3 (post-latch lock-in). At-or-greater (unlike
// regGateActive's strictly-greater): era-3 is a MINT/FORMAT boundary, so H_era3 is
// itself the first v4 height ("at/above H_era3, a block MUST be v4"), whereas the
// #506 boundary block is the last OLD-rules block. Derived from committed history,
// so every replica — live or replaying — agrees; epoch-final, so a reorg cannot
// move the boundary to un-enforce it (#357 Condition A, cert Q5).
func (c *Chain) era3Active(h uint64) bool {
	if c.cfg.Era3ActivationHeight > 0 {
		return h >= c.cfg.Era3ActivationHeight
	}
	return c.era3LockedIn && h >= c.era3Height
}

// MintVersion is the BlockVersion a proposer should stamp for a NEW block at
// height h: BlockVersionStateRoot (v4) at/above the era-3 activation boundary,
// BlockVersionRounds (v2) below it. The propose path (core/node) asks the chain,
// which owns the activation state, rather than re-deriving the boundary. Pure
// function of committed state — every honest proposer at the same head mints the
// identical version (I5).
func (c *Chain) MintVersion(h uint64) uint64 {
	if c.era3Active(h) {
		return BlockVersionStateRoot
	}
	return BlockVersionRounds
}

// PopulateEra3Roots stamps b as a v4 block and attaches this chain's committed
// StateRoot/LogRoot over the POST-APPLY state of b — the roots a validator will
// recompute and check (validateEra3Roots). It is called by the propose path AFTER
// all apply-affecting content (BondRegs, entries, slashes) is folded into b, so the
// roots cover the block as it will actually commit. The recompute uses the same
// dry-run apply (postApplyRoots) the 2b predicate uses, so the proposer's root and
// the validator's recompute come from one authoritative state-transition function.
// A no-op below the era-3 boundary: the propose path only calls this when
// MintVersion(h) == BlockVersionStateRoot.
func (c *Chain) PopulateEra3Roots(b *Block) error {
	sr, lr, err := c.postApplyRoots(*b)
	if err != nil {
		return err
	}
	b.Version = BlockVersionStateRoot
	b.StateRoot = &sr
	b.LogRoot = &lr
	return nil
}

// regGateActive reports whether the #506 reg-inclusion rate bound governs a
// block at height h: past the genesis-declared boundary (pre-latch), or past
// the chain-derived H_act (post-latch lock-in). Strictly greater — the boundary
// block itself is the last old-rules block (certification Q2: "apply the R-rule
// to every block of height > H_act"). Enforcement is HEIGHT-keyed, not
// version-tag-keyed, so an un-upgraded proposer's v2-tagged block cannot carry
// a storm reg past the gate; its honest (reg-clean) blocks stay valid, which is
// the bounded-liveness story (≤ ~1.5 rounds via the #451 escape).
func (c *Chain) regGateActive(h uint64) bool {
	if c.cfg.RegGateActivationHeight > 0 {
		return h > c.cfg.RegGateActivationHeight
	}
	return c.gateLockedIn && h > c.gateHeight
}

// restoresHeldStanding reports whether a bond registration (id, root) merely
// RESTORES standing the identity already held — a current frozen-epoch member
// re-proving a Root it already owns — which is EXEMPT from the #506 R interval
// (research certification 2026-08-23, #535 fix (4)). The R interval defends
// against a reg-FLOOD of fresh identities/plots (each carrying a ~1.5 MB Answer,
// the #503 OOM driver); a returning frozen-set member re-proving its OWN plot is
// not that — it can only restore weight the honest set already trusted for this
// epoch, never admit new weight, so it cannot cheapen capture (unlike shrinking
// the quorum denominator — the certification's rejected fix (1)). This removes
// the non-recovery the h64 field wedge showed: a returning member was R-refused
// (`re-registering 1 block after its last reg, R=10`) and so could not re-bond
// to heal the stalled boundary. Narrow + deterministic: same OWNED root
// (bondRootOwner survives a lapse; F1's ownership record is never cleared) AND
// current frozen-epoch membership (≤ EpochBlocks old — the cert's "within ~one
// epoch of its lapse"; a lapsed member keeps its frozen epochSet vote for the
// epoch, chain.go attesterQualified). Mature-epoch only, where the frozen set —
// hence "already held standing" — exists; launch-window R is unchanged.
func (c *Chain) restoresHeldStanding(id ports.NodeID, root ports.Hash) bool {
	if !(c.epochsEnabled() && c.matureEpoch) {
		return false
	}
	if c.bondRootOwner[root] != id {
		return false // not re-proving a root this identity already owns
	}
	if c.bonded[id] >= c.cfg.MinBond {
		// Still holds LIVE standing — a re-reg here is padding volume (the #506
		// storm), not a restore. The exemption is only for a member whose
		// standing has LAPSED (the cert's "within ~one epoch of its lapse"),
		// so #506's storm/flood protection is untouched for bonded members.
		return false
	}
	_, inEpoch := c.epochSet[id]
	return inEpoch // a lapsed member still seated in the current frozen epoch
}

// regMinInterval is R — the minimum block distance between one identity's
// committed bond registrations once the gate is active. Derived, never a
// literal: TTL/4 (an honest renewal is due at TTL/2 > R, so it is never
// blocked; the "single dropped renewal cannot lapse standing" margin holds:
// R < TTL/2 < TTL), floored at the head-freshness window K plus margin so
// re-tuning TTL down never pushes R below the window the reg is fresh over
// (certification Q3 caveat). If a pathological config leaves TTL/2 ≤ R,
// honest-renewal liveness wins: cap at TTL/2 − 1.
func (c *Chain) regMinInterval() uint64 {
	k := uint64(c.cfg.BondRegHeadWindow)
	if k == 0 {
		k = DefaultBondRegHeadWindow
	}
	r := c.cfg.BondTTLBlocks / 4
	if r < k+2 {
		r = k + 2
	}
	if ttl := c.cfg.BondTTLBlocks; ttl > 0 && r >= ttl/2 {
		r = ttl/2 - 1
	}
	return r
}

// Weight is the chain's fork-choice weight: the cumulative count, over every
// block, of DISTINCT qualified non-proposer attestations. More real
// validators standing behind a history makes it heavier — so the heaviest
// chain is the one the most earned standing has committed to, not merely the
// longest (which a fast Sybil could extend). Signatures are objective; the
// qualification bar is the local reputation view, which converges among
// honest replicas. (Making the weight fully partition-independent — objective
// on-chain PoST-bond weight — is the recorded D2 hardening; see §3e.)
func (c *Chain) Weight() int64 {
	var w int64
	for i := range c.blocks {
		w += c.blockWeight(&c.blocks[i])
	}
	return w
}

// blockWeight is the fork-choice weight this block contributes. In OBJECTIVE
// mode (F6) it sums the on-chain bonded SIZE of each distinct, non-proposer
// attester whose signature verifies — a quantity every replica recomputes
// identically from the chain, so honest replicas can never disagree on which
// fork is heavier. In legacy mode it COUNTS those attesters, gated by the local
// reputation view (which is what could diverge under a partition). Either way it
// is the same rule ValidateCommit counts support by.
func (c *Chain) blockWeight(b *Block) int64 {
	h := b.Hash()
	seen := make(map[ports.NodeID]bool)
	var n int64
	for _, a := range b.Atts {
		if len(a.PubKey) != ed25519.PublicKeySize {
			continue
		}
		id := a.AttesterID()
		if seen[id] || id == b.ProposerID() {
			continue
		}
		if !ed25519.Verify(ed25519.PublicKey(a.PubKey), h[:], a.Sig) {
			continue
		}
		if !c.attesterQualified(id) {
			continue
		}
		seen[id] = true
		if c.objective() {
			// #357 §1a — the ramp needs an always-present, monotone weight signal.
			// A really-bonded attester weighs its committed bond (the mature-regime
			// C2 quantity, unchanged). A qualified attester BELOW MinBond can only be
			// a launch anchor (attesterQualified gated it) — during the young window
			// it carries a fixed bootstrap weight (the sanctioned immutable-#3 training
			// wheels), not 0. Without this, a fresh anchor-attested chain has Weight()≈0
			// and heavier() falls through to a height-blind tiebreak that drops committed
			// blocks to height 0. The weight vanishes at maturity (launchAnchor ⇒ false
			// once everMature), so the mature-regime fork-choice quantity is untouched.
			switch {
			case c.epochsEnabled() && c.matureEpoch:
				// #357 Condition A: during a mature epoch an attester weighs its
				// SNAPSHOT bond (membership was already gated by attesterQualified
				// above), so fork-choice weight is stable within the epoch — the
				// same frozen quantity the finality quorum is sized over. Live
				// renewals/growth integrate at the next rotation.
				n += c.epochSet[id]
			case c.bonded[id] >= c.cfg.MinBond:
				n += c.bonded[id]
			case c.launchAnchor(id):
				n += c.anchorWeight()
			}
		} else {
			n++ // legacy weight = count of qualified attesters
		}
	}
	return n
}

// anchorWeight is the fixed fork-choice weight a zero-bond launch anchor carries
// during the young window (#357 §1a). Config.AnchorWeight overrides it; unset it
// defaults to MinBond, so an anchor weighs like a minimally-bonded validator — a
// real bond of any size still outweighs an anchor, and once the network matures
// anchors carry no weight at all (launchAnchor ⇒ false). A nonzero floor guarantees
// the ramp always has a weight signal even if MinBond is 0 (sim/legacy configs).
func (c *Chain) anchorWeight() int64 {
	if c.cfg.AnchorWeight > 0 {
		return c.cfg.AnchorWeight
	}
	if c.cfg.MinBond > 0 {
		return c.cfg.MinBond
	}
	return 1 << 20
}

// Reconcile heals a fork (D2): given a peer's full chain from genesis, it
// re-validates the whole thing in a throwaway replica and, iff that chain is
// strictly heavier than ours (ties broken by the lower head hash), ADOPTS it —
// rolling our replica back to the common genesis and forward onto the heavier
// history. A diverged node therefore stops being forked forever (the old
// SyncChain just `break`ed). The fork must share OUR genesis, so a peer cannot
// swap the chain out from under us with a heavier foreign history; and every
// block is fully re-validated, so a lying peer wastes our time but cannot feed
// us an invalid chain. Returns whether we adopted the fork.
func (c *Chain) Reconcile(fork []Block) (bool, error) {
	if len(fork) == 0 {
		return false, ErrEmptyFork
	}
	if len(c.blocks) == 0 {
		return false, ErrNoGenesis
	}
	if fork[0].Height != 0 || fork[0].Hash() != c.blocks[0].Hash() {
		return false, ErrForeignGenesis // must branch from our own genesis
	}
	// Weak-subjectivity guard (F-1): refuse — regardless of weight — any fork that does
	// not contain the trusted checkpoint block, i.e. that rewrites finalized history at
	// or before it. This is the long-range-attack defense that makes the maturity latch
	// safe for a fresh/long-offline node. Cheap and positional (fork blocks are
	// contiguous from genesis, so index == height; the block hash covers the height, so
	// a match pins the exact checkpoint block). Checked before the replay so a
	// long-range fork is rejected without doing the work.
	if cp := c.cfg.WSCheckpoint; cp.Height > 0 {
		if uint64(len(fork)) <= cp.Height || fork[cp.Height].Hash() != cp.Hash {
			return false, ErrPreCheckpointReorg
		}
	}
	// §3 quorum-finality gate (#357; research certification 2026-08-13; owner decision D-1
	// "prefer stall to reorg"). In OBJECTIVE mode every committed block is super-quorum-final:
	// it met RequiredQuorum, which §2 sizes over the PINNED validator set (the anchor set in
	// the launch window). Quorum-intersection therefore makes it irreversible — so refuse any
	// fork that does not CONTAIN our committed head, i.e. that would revert a finalized block.
	// Fork-choice's heaviest-weight rule (heavier) then only ever adjudicates among DESCENDANTS
	// of the finalized head (the Tendermint/Gasper rule) — "reorg to height 0" is structurally
	// impossible. Finality is quorum-based, NEVER bare depth (a depth cap lets two partitions
	// finalize conflicting blocks — worse than a reorg). Under a >⅓ partition a node simply
	// STALLS (can't gather the super-quorum) rather than reorg committed history (D-1); the
	// storage plane keeps serving throughout (D-2), so durability is unaffected. Fork blocks
	// are contiguous from genesis (index == height) and each hash chains its ancestry, so
	// matching our head hash at its index proves the fork extends our exact finalized history.
	// Legacy (subjective) mode keeps pure heaviest-chain reorg — it has no BFT finality.
	// (Launch-phase: finalized == committed head over the pinned anchor set. The mature
	// phase is sound with epochs enabled: Condition A freezes the finality set per epoch
	// — every quorum is taken over the same snapshot, so super-quorums genuinely
	// intersect — and Condition B roots the handoff at a finalized boundary. With epochs
	// explicitly disabled (trusted/demo), a mature-phase conflict stalls both sides,
	// which is D-1-safe.)
	//
	// GATED ON A REAL SUPER-QUORUM. Finality is quorum-INTERSECTION safety, which only holds
	// when a commit takes ≥ bftThreshold of the validator set — so the gate applies only when
	// RequiredQuorum is a genuine super-quorum. A TRUSTED weak config (e.g. Quorum=1, no
	// ByzantineQuorum) has no quorum intersection: a single attester "commits," so a block is
	// NOT final and a lone equivocator can even split the honest set onto two committed forks.
	// Applying a finality gate there would freeze that split; instead such a config keeps
	// heaviest-chain reorg (and its equivocation slash heals by adopting the heavier fork).
	// With ByzantineQuorum on (the untrusted/production default) RequiredQuorum is already
	// bftThreshold, so the gate always engages there. In a MATURE EPOCH the Byzantine bar
	// is the >⅔ frozen-WEIGHT rule (requireEpochWeightQuorum, B2) rather than a head
	// count — two >⅔-weight coalitions overlap in >⅓ weight, hence honest bond, so
	// weight-committed blocks are exactly as final and the gate engages there too
	// (finalityQuorumActive).
	if c.finalityQuorumActive() {
		if headIdx := len(c.blocks) - 1; headIdx > 0 { // genesis divergence is already ErrForeignGenesis
			if len(fork) <= headIdx || fork[headIdx].Hash() != c.blocks[headIdx].Hash() {
				return false, ErrPreFinalityReorg
			}
		}
	}
	// Re-validate the candidate history end to end in a fresh replica.
	tmp := New(c.cfg, c.rep)
	tmp.tokenQuorum, tmp.issuerKey = c.tokenQuorum, c.issuerKey
	tmp.verifyBond = c.verifyBond // so the fork's bond registrations re-verify (F6)
	// Pin the pruned-tolerance floor to THIS node's own anchor (Q2 gate, slice 3): a
	// pruned block in the fork is trusted only strictly below the RECEIVER's finalized/
	// checkpoint anchor, never a height the fork's own (attacker-supplied) replayed state
	// could inflate. Snapshot so the floor cannot move mid-replay.
	floor := c.trustFloor()
	tmp.trustFloorOverride = &floor
	if err := tmp.AppendGenesis(fork[0]); err != nil {
		return false, err
	}
	for i := 1; i < len(fork); i++ {
		if err := tmp.Append(fork[i]); err != nil {
			return false, fmt.Errorf("reconcile: fork block %d (height %d): %w", i, fork[i].Height, err)
		}
	}
	if !heavier(tmp, c) {
		return false, nil
	}
	c.adopt(tmp)
	return true, nil
}

// heavier reports whether chain a should win fork-choice over b: strictly more
// weight, or equal weight with a deterministic lower-head-hash tiebreak so
// every honest node picks the same winner.
func heavier(a, b *Chain) bool {
	wa, wb := a.Weight(), b.Weight()
	if wa != wb {
		return wa > wb
	}
	// §1b (#357): on equal weight, prefer the TALLER chain. A heaviest-chain protocol
	// must never let a SHORTER fork win a weight tie — the old height-blind head-hash
	// tiebreak is exactly what let a genesis fork (height 0) displace committed blocks.
	// Weight is still decided first, so a genuinely heavier shorter chain still wins;
	// height only breaks a weight tie (monotonicity: never drop committed height for
	// an equal-weight fork).
	ah, bh := a.blocks[len(a.blocks)-1].Height, b.blocks[len(b.blocks)-1].Height
	if ah != bh {
		return ah > bh
	}
	// Same weight AND height ⇒ a deterministic head-hash tiebreak, so honest replicas
	// still converge on one of two genuinely-equivalent forks.
	ha, hb := a.blocks[len(a.blocks)-1].Hash(), b.blocks[len(b.blocks)-1].Hash()
	return bytesLess(ha[:], hb[:])
}

func bytesLess(a, b []byte) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// adopt replaces this replica's state with a reconciled fork's. Because all
// derived state (byRoot, spent, revoked, validatorsSeen) is a pure function of
// the blocks, swapping the whole precomputed set is the reorg — no fragile
// per-record undo.
func (c *Chain) adopt(t *Chain) {
	c.blocks = t.blocks
	c.byRoot = t.byRoot
	c.revoked = t.revoked
	c.revLog = t.revLog
	c.validatorsSeen = t.validatorsSeen
	c.spent = t.spent
	c.bonded = t.bonded
	c.bondRootOwner = t.bondRootOwner
	c.bondRootProven = t.bondRootProven
	c.bondRegHeight = t.bondRegHeight
	c.regVersion = t.regVersion
	c.gateLockedIn = t.gateLockedIn // #506 activation is derived state too: the
	c.gateHeight = t.gateHeight     // replayed fork re-ran every rotation (Q2/I3)
	c.era3LockedIn = t.era3LockedIn // era-3 (v4) activation is derived the same way:
	c.era3Height = t.era3Height     // the replayed fork re-ran every rotation (Q5/I3)
	c.bondDomain = t.bondDomain
	c.slashed = t.slashed
	c.everMature = t.everMature // the maturity latch is a function of the adopted history (F-1)
	// The epoch machinery is derived state like everything above: the replayed
	// fork re-ran every rotation, so its snapshot/handoff ARE the adopted truth.
	c.epochSet = t.epochSet
	c.epochStart = t.epochStart
	c.matureEpoch = t.matureEpoch
}

func (c *Chain) LookupRoot(root ports.Hash) (ports.Entry, bool) {
	e, ok := c.byRoot[root]
	return e, ok
}

func (c *Chain) AllEntries() []ports.Entry {
	var out []ports.Entry
	for _, b := range c.blocks {
		out = append(out, b.Entries...)
	}
	return out
}
