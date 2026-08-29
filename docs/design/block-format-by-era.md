# What's in a silt block, by era

Status: reference. Describes the chain block as it exists in `core/chain/chain.go`
today (era-1 and era-2, both shipped) and the **planned** era-3 state-root format
(designed and ratified in principle, **not yet built**). Era-3 rows are marked
PLANNED so this doc is not mistaken for shipped canon.

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
| era-3 | `3` = `BlockVersionRegGate` (#506) | tag exists; **state-root format PLANNED, not minted** | Block commits a **state SMT root** + a **transparency-log root** |

`BlockVersion = BlockVersionRounds` (`chain.go:281`) — nodes mint **era-2** today.
`BlockVersionRegGate = 3` (`chain.go:299`) already exists but is a *readiness tag*, not a
minted version (see era-3 below).

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

## era-3 — the state-root keystone (v3) — PLANNED, not built

Today the block commits **no state root**. The only `Root` fields are inside `BondReg`
(`chain.go:419`, per-registration bond commitments) — not a root over the validity state.
That is the gap era-3 closes.

The ratified era-3 format (C-7, `docs/decisions.md`) adds **two new hash-covered,
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

**Two hard prerequisites before era-3 can freeze:**

- Every field committed under the state root must be proven **load-bearing** (no bloat) and
  **order-independent** (identical content → identical root, no fork). That proof work is
  the keystone model-check oracles in `core/chain/modelcheck_*_test.go`, in progress.
- The floor-box verifier must carry the invariant **"no witness supplied for a key a
  predicate reads → never accept (reject / stall)"** — accepting on a missing witness
  inverts the safe-degradation proof.

**Why it must be frozen:** the committed field set and its encoding become a permanent
consensus contract. A committed field that buys no verdict is permanent bloat; a
non-canonical encoding forks. A sound witness scheme cannot even exist until the root is a
committed, attested block field. So the format is proven complete and fork-free **once**,
up front, then frozen (a research-certified, human-ratified step).

## How eras coexist without a flag-day

`Version` is committed by the hash and required at decode, so a block from one era can
never be silently mis-validated under another era's rules (`chain.go:260-268`). Validation
is era-gated (`ValidateCommit`, `VerifyEquivocation`): an era-1 block validates under era-1
rules, an era-2 block under era-2 rules. Additive fields (via `keyasint,omitempty`) keep a
version bump reserved for real rule/commitment changes — the era-3 state roots are exactly
such a change, which is why they need a version and a freeze rather than a quiet field add.
