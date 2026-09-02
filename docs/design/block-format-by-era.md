# What's in a silt block, by era

Status: reference. Describes the chain block as it exists in `core/chain/chain.go`
today. era-1 and era-2 are shipped. The **era-3 committed state-root format
(`BlockVersion = 4`) is BUILT and FROZEN** (2026-08-29, #632, build `3af40bc`;
`docs/decisions.md` D-TIERING freeze entry, `docs/TENETS.md` Part IX Immutable tier).
The **era-4 witnessable-transitions format (`BlockVersion = 5`) is BUILT and merged**
(2026-08-29, PRs #637/#639/#640/#641) but is deliberately kept **OPEN-ENDED**: its
freeze is deferred to the end of Proof-of-Delivery (owner-ratified 2026-08-30). Note
the version numbering: `BlockVersionRegGate = 3` is an **unminted soft-fork readiness
tag**; the era-3 state-root format shipped at **v4** (`BlockVersionStateRoot`), and
era-4 at **v5** (`BlockVersionWitnessable`).

## The short version

There is **one** `Block` struct (`chain.go:311`). An "era" is not a different struct —
it is a `BlockVersion` (`chain.go:260`): the rule/schema version a block is minted under,
committed inside the block hash and checked at decode. Fields are added *additively*
(CBOR `keyasint,omitempty`), so a block that doesn't use a newer field hashes exactly as
an older block did. A version bump is reserved for a change that would otherwise be a
silent flag-day — a change to **what the hash commits** or **how the block validates**.

| Era | `Version` const | Status | The defining change |
|-----|-----------------|--------|---------------------|
| era-1 | `1` (v1) | shipped, legacy | bare-hash consensus signatures (single-phase) |
| era-2 | `2` = `BlockVersionRounds` (#432) | **shipped — minted today** | two-phase (prepare + precommit) round-scoped signatures |
| (reg-gate) | `3` = `BlockVersionRegGate` (#506) | **unminted soft-fork tag** | the #506 reg-inclusion rate bound — HEIGHT-enforced, never minted (minting it would be a hard fork; the R-rule only rejects payloads, so it stays a soft fork) |
| era-3 | `4` = `BlockVersionStateRoot` (#632) | **BUILT + FROZEN 2026-08-29** | Block commits a **state SMT root** + a **transparency-log root** |
| era-4 | `5` = `BlockVersionWitnessable` (#637/#639/#640/#641) | **BUILT + merged 2026-08-29; format kept OPEN-ENDED** | makes the TTL-expiry and epoch-rotation transitions **witnessable** (`RegCap` count cap, the `qualified`/due-bucket/`epochStart` spine) |

`BlockVersion = BlockVersionRounds` (`chain.go:319`) — nodes mint **era-2** today; the
v4 and v5 formats are built and version-gated but activate at height-gated boundaries
(`MintVersion(h)`, `chain.go:3540`). `BlockVersionRegGate = 3` (`chain.go:337`) is a
*readiness tag*, not a minted version (see the reg-gate row above); the era-3 state-root
format shipped at **v4**, not v3.

## The shared Block struct

Every era uses these fields (`chain.go:311-390`). What changes per era is which are
**populated**, how signatures are **scoped**, and what the **hash commits**.

| Field | CBOR | Purpose | In the hash? |
|-------|------|---------|--------------|
| `Height` | 1 | chain position | yes |
| `Prev` | 2 | parent block hash (the chain link) | yes |
| `Entries` | 3 | the registry payload: content records (`ports.Entry` — `Root`, `ManifestChunks`, `FileSize`, optional `Publisher`, optional blind `Token`) | yes |
| `Proposer` | 4 | proposer ed25519 public key | yes |
| `ProposerSig` | 5 | proposer's signature over the block | no (signs the hash) |
| `Atts` | 6 | attester consensus signatures (the quorum) | **no** — strippable, set on the committed copy |
| `Revocations` | 7 | append-only takedown tombstones (opaque roots, per-operator honored) | yes |
| `Version` | 8 | the block's rule era | yes |
| `Unrevocations` | 9 | quorum-gated reversal of a prior takedown | yes |
| `BondRegs` | 10 | on-chain PoST bond registrations — makes fork-choice OBJECTIVE (who is a qualified validator + attestation weight, as a function of the chain) | **yes** (attesters sign over them) |
| `Slashes` | 11 | on-chain equivocation proofs; on commit the culprit is evicted from the bonded set | **yes** |
| `CommitRound` | 12 | the round this block committed at (era-2) | **no** — excluded so a re-proposed block hashes identically |
| `PrepareQC` | 13 | the prepare-phase quorum certificate justifying the precommit (era-2) | **no** — excluded like `Atts` |
| `Pruned` | 14 | pre-prune hash for a payload-pruned block (heavy proofs dropped) | **no** — excluded |

Key idea: **the hash commits the payload and the rules** (Entries, BondRegs, Slashes,
Revocations, Unrevocations, Version, Height, Prev, Proposer). The **consensus signatures
and post-commit certificates are excluded** (Atts, PrepareQC, CommitRound, Pruned) so the
same block value keeps one identity across rounds and re-serving.

## era-1 — the legacy bare-hash era (v1)

- **Consensus signatures are single-phase.** An `Attestation` (`chain.go:401`) is
  `Sig` over the **bare block hash**, with `Round = 0` and `Phase = PhaseLegacy`
  (`chain.go:305`). No prepare phase, no round scoping.
- `CommitRound`, `PrepareQC` are unset (zero). `Round`/`Phase` decode as `(0, PhaseLegacy)`
  on a legacy attestation and hash identically — that's what makes era-2's fields additive.
- Committed era-1 history keeps validating under era-1 rules forever (era-gated in
  `ValidateCommit` / `VerifyEquivocation`); it is never re-interpreted under a newer era.

## era-2 — the rounds era (v2, `BlockVersionRounds`, #432) — minted today

- **Consensus signatures become two-phase and (height, round, phase)-scoped.** A signature
  is over the domain-separated payload `consensusSigBytes(phase, round, hash)`, so a
  prepare can never be replayed as a precommit and a signature at one round can never
  complete a quorum at another (`chain.go:395-400`).
- `Atts` holds the **precommit** quorum at `CommitRound`. `PrepareQC` holds the **prepare**
  quorum that justified it. `ValidateCommit` (era-2) requires `PrepareQC` at the *same*
  thresholds as the commit (⌊A/2⌋+1 anchors in launch, >⅔ frozen-epoch weight when mature).
- `CommitRound` and both certificates are set on the **committed copy** and excluded from
  the hash, so the same block re-proposed at a higher round after a view-change hashes
  identically.
- **The #506 reg-gate readiness signal lives here already.** `BondReg.Version`
  (`chain.go:448`) is each validator's highest-validatable rule era. It rides the bond reg
  (which *is* hash-covered and validator-signed) so every replica can count rule-aware
  attestation weight from committed history alone. This is "era-3 tenant #1": the R-rule is
  enforced by **height** relative to an activation boundary, needing no schema bump — which
  is why `BlockVersionRegGate` exists as a tag but is **not minted** (minting it would be a
  hard fork; the R-rule only rejects payloads, so it stays a soft fork).

## era-3 — the state-root keystone (v4, `BlockVersionStateRoot`) — BUILT + FROZEN 2026-08-29

Before era-3, the block committed **no state root**. The only `Root` fields were inside
`BondReg` (`chain.go:419`, per-registration bond commitments) — not a root over the
validity state. That is the gap era-3 closes. The format shipped at **`BlockVersion = 4`**
(the unminted `BlockVersionRegGate = 3` tag stayed a soft-fork readiness signal) and was
**FROZEN 2026-08-29** (#632, build `3af40bc`): it is now in the Immutable tier
(`docs/TENETS.md` Part IX), so changing it requires a NEW ERA (a new `BlockVersion`), not
an edit. The full freeze scope — schema, the 18-field committed set, the v4 hard-fork
activation, and the verifier posture — is in the `docs/decisions.md` D-TIERING freeze entry.

The era-3 format (C-7, `docs/decisions.md`) adds **two new hash-covered,
attester-signed roots** to the Block:

1. a **state SMT root** — a history-independent sparse-Merkle root over the set-valued
   validity state (who's bonded, spent serials, slashed, epoch set, gate state, …); and
2. a separate **append-only RFC-6962 transparency-log root** for the ordered log
   (the two-root shape from the #597 certification — state root + log root, like
   Ethereum's stateRoot / receiptsRoot separation).

**Why:** once the block commits a state root, a node can validate a block's state
transition from proposer-supplied **witnesses** (inclusion / exclusion proofs) checked
against the trusted root — the stateless-client "floor box" — instead of holding the whole
registry tree (which grows without bound). This is the decentralization posture behind
#600 / C-7.

**The two hard prerequisites for the freeze (both discharged before 2026-08-29):**

- Every field committed under the state root is proven **load-bearing** (no bloat) and
  **order-independent** (identical content → identical root, no fork), via the keystone
  model-check oracles in `core/chain/modelcheck_*_test.go`.
- The verifier carries the invariant **"no witness supplied for a key a predicate reads →
  never accept (reject / stall)"** — accepting on a missing witness inverts the
  safe-degradation proof.

**Why it is frozen:** the committed field set and its encoding are a permanent consensus
contract. A committed field that buys no verdict is permanent bloat; a non-canonical
encoding forks. So the format was proven complete and fork-free **once**, up front, then
frozen (a research-certified, owner-ratified step).

## era-4 — witnessable state transitions (v5, `BlockVersionWitnessable`) — BUILT + merged, format OPEN-ENDED

era-4 makes two `apply()` transitions **witnessable** so a tree-free floor box can validate
them without holding the whole registry: the TTL-expiry sweep and the epoch-rotation
qualified-set rebuild, both of which previously scanned whole committed maps. The format
shipped at **`BlockVersion = 5`** across four merged increments on 2026-08-29:

- **4a (#637)** — mint `BlockVersion = 5` and reserve the three v5 field tags (inert).
- **4b (#639)** — the maintenance spine: `qualified` + due-bucket + `epochStart`, v5-gated.
- **4c (#640)** — the v5 validity predicate + the `RegCap` per-block count cap + version-widen.
- **4d (#641)** — height-gated activation + the mint-flip to v5.

`RegCap` is a **per-block TOTAL BondReg count** validity ceiling — fresh AND renewal,
counted after `canonicalBondRegs` (same-id fold) — value **256** (`chain.go:404`, enforced
`chain.go:1775`). The earlier "fresh-only" interpretation was REFUTED and corrected: both
fresh and renewal land in the same TTL due-bucket, so a fresh-only cap leaves the read-set
unbounded. Full mechanism and the seven-determinant re-derivation gate are in the
`docs/decisions.md` era-4 entry.

**This is the chain-side witnessable-transitions spine only.** It does not by itself ship
the trustless floor-box (witness) validator, which remains the open C-7 / #600 follow-on.

### The v5 `LastCommit` attestation carrier (additive, 2026-09-03)

| cbor key | field | covered by `Hash()` | rule |
|---|---|---|---|
| `18` | `LastCommit []Attestation` | **YES** | v5-only. Republishes the PARENT block's precommits. |

Added by owner call **O1** of the R-BOX-ATTESTS converged verdict (2026-09-02, ratified
2026-09-03). It is the **only** input to the v5 `validatorsSeen` transition; a v5 block's own
`Atts` write nothing.

**Why it exists.** `Hash()` excludes `Atts`, but pre-carrier `apply()` wrote `validatorsSeen`
from them, and the root predicate re-runs `apply` over the ATTACHED certificate. A proposer
populates its roots BEFORE it gathers, so any certificate that would seat a NEW attester made
the recomputed root differ from the signed one and every replica rejected that block. The
consequences were a permanently frozen decentralization measurement and an intermittent
all-honest stall. The carrier moves the seating input into hash-covered content the proposer
holds before it signs.

**Rules.** Every entry verifies over `b.Prev` at `PhasePrecommit` at its own round (NOT bound
to `CommitRound`, which `Hash()` does not cover); ids are distinct; a sub-v5 block carrying the
field is invalid; height 1's carrier is empty by rule and genesis attestations are refused by
rule. The transition seats each carried signer that is not the parent's proposer and is
`attesterQualified` against the child's pre-state, folded BEFORE the block's bond
registrations / TTL / slashes.

**Compatibility.** `omitempty`, so a block carrying no carrier hashes byte-identically to
pre-carrier code — the frozen era-3 (v4) format is untouched (`TestCarrierHashDriftGuard`).
The frozen sub-v5 `b.Atts` seating rule is left byte-for-byte and now runs only for sub-v5
blocks. cbor key **18**, not 17: 17 is reserved for the R0.4b `IssuerKeys` field.

**Disclosed:** the seat lands one block late (monotone, benign); a proposer can DELAY a seating
by omitting a signer but can never FORGE one (downward-only, unenforceable by rule).

**The v5 format is deliberately kept OPEN-ENDED.** There is no live blockchain, and
Proof-of-Delivery is expected to add or reshape witnessable state, so freezing v5 now would
freeze a format PoD may still move. The **v5 freeze is deferred to the end of PoD**, to be
run as a **second practiced era freeze** (the era-3 freeze being the first) on the same
research-certified, owner-ratified path (owner-ratified 2026-08-30, `docs/decisions.md`).

## How eras coexist without a flag-day

`Version` is committed by the hash and required at decode, so a block from one era can
never be silently mis-validated under another era's rules (`chain.go:260-268`). Validation
is era-gated (`ValidateCommit`, `VerifyEquivocation`): an era-1 block validates under era-1
rules, an era-2 block under era-2 rules, and so on up to v5. Additive fields (via
`keyasint,omitempty`) keep a version bump reserved for real rule/commitment changes — the
era-3 state roots and the era-4 witnessable-transition fields are exactly such changes,
which is why they need a version and a freeze rather than a quiet field add.
