# Witness floor-box — increment 3 (delivery layer) options

Date: 2026-08-29
Status: DESIGN OPTIONS ONLY. No code. Routes through blind PE + Researcher before build.
Author seat: Builder

## What this increment is

Increments 1 and 2 built the floor box's *consumer* interface. Increment 1 (`core/statehash/witness.go`, PR #633) is the three-valued `Resolve` accessor. Increment 2 (`core/statehash/witness_bound.go`, PR #634) is `IngestBlockWitnesses`, which gates a witness bundle against a read-set and returns `map[key]Result`. Both consume a `[]ReadEntry` (tagged key + `QueryKind` + expected value) and raw witnesses.

Neither increment produces the read-set, and neither fetches a witness. Increment 3 is the *producer* and the *transport*: turn a real block into its exact `[]ReadEntry`, and deliver the witnesses for those keys. It gives the merged accessor + bound their first production caller.

Two of its parts are gated, so this is a design pass, not a build.

- **Part A — Block → read-set derivation.** Soundness-load-bearing. Enumerate, per transition type, exactly which committed-set keys each predicate reads, the query kind, and the expected value.
- **Part B — D-2 on-demand delivery.** Tenet-gated. Fetch the witnesses for a block's read-set from any-of-N unprivileged providers, with a per-provider read deadline that closes the slow-loris residual.

## The one-paragraph mechanism (attribution)

A floor box holds the two committed roots, not the SMT tree. To validate a block it must (1) know exactly which committed-set keys the block's predicates read, so it can demand a proof for each, and (2) obtain those proofs from some node that does hold the tree at the parent root. The failure this increment prevents is *silent under-demand*: if the derived read-set omits a key some predicate reads, `IngestBlockWitnesses` never gets a `ReadEntry` for it, never demands a witness, and the predicate is evaluated against un-witnessed state — the floor box accepts a transition it never verified. Part A addresses this by deriving the read-set from the *same* predicate paths the applier runs, guarded so a predicate change cannot silently desync it. Part B addresses delivery by fetching each read-set key's proof from any-of-N providers, first-correct-wins, with every fetch failure feeding the accessor's `NoWitness` arm (stall), never `ProvenAbsent`.

---

## Part A — Block → read-set derivation

### The completeness property, stated precisely

For a block `b` validated against parent state `S` (committed root `R`), let `Reads(b, S)` be the set of `(key, kind, value)` triples such that evaluating `b`'s acceptance against `S` observes committed-set key `key`. The derived read-set `D(b, S)` is **complete** iff `Reads(b, S) ⊆ D(b, S)`, and **faithful** iff for every triple in `D`, the `kind` and `value` match what the predicate actually checks.

Completeness is the soundness surface. An omitted key is silently un-witnessed and lets an unverified transition through. Over-inclusion (a key in `D` that no predicate reads) is *not* a soundness bug — it costs one wasted witness fetch and is caught by increment 2's shape gate as a bundle the honest provider simply supplies. So the derivation must err toward over-inclusion, never under-inclusion.

### The non-obvious finding: acceptance reads live in TWO places, not one

The task framed the read-set as "which keys each predicate reads." Reading the source, acceptance of a block observes committed-set keys in **both** the validity predicates **and** `apply()`. This is the central completeness trap.

`apply()` (`core/chain/chain.go:2926`) is not a blind writer. It branches on committed state to decide what it writes:

- `c.slashed[id]` (`:2977`) — a slashed id's registration is skipped (writes nothing).
- `c.bondRootOwner[r.Root]` + `c.bondRootProven[r.Root]` (`:2980`, `:2986`) — the proof-beats-declaration displacement rule; determines whether `bonded`/`bondRootOwner` are written and whether the prior owner's `bonded` is stripped.

These reads change the committed post-state, hence the next `StateRoot`. A floor box that witnesses only the validity-predicate reads can compute a *wrong post-state root* even when every validity predicate passed, because it branched `apply` on un-witnessed `bondRootProven`. **The read-set must be the union of the keys read on the validity path AND the keys `apply` branches on.** Any option that scopes A to `ValidateEntry`/`validateBondRegs`/`validateSlashes` alone is incomplete by construction.

### The second finding: reads are conditioned on live scalars, some of which are committed and some are NOT

Several committed-set reads are gated by predicates over *other* state:

| Gating condition | Where | Committed under StateRoot? |
|---|---|---|
| `regGateActive(h)` = `gateLockedIn && h > gateHeight` | `:3242` | `gateLockedIn`, `gateHeight` — YES (tags present) |
| `matureEpoch` (frozen-epoch regime) | `:1133`, `:3263` | `matureEpoch` — YES |
| `epochSet[id]` membership | `:1146`, `:3276` | `epochSet` — YES |
| `objective()` = `MinBond>0 && verifyBond!=nil` | `:1057` | config + wiring — NO (node-local, not committed) |
| `epochStart` / recovery boundary (`effectiveEpochSet`) | `:2597` | `epochStart` — NO (observable, excluded by the freeze) |

The rows marked NO matter. `objective()` is a node-config predicate, identical on every honest replica, and is not committed — the floor box knows its own config, so this is fine. `effectiveEpochSet`'s `#535` recovery boundary is an operator-directed observable, not committed. A floor box that must resolve `attesterQualifiedAt` at the recovery boundary reads `epochStart`, which has NO committed root. This is a **flagged residual**, not solved here: the quorum-stack reads (`requireEpochWeightQuorum`, `attesterQualifiedAt`) sit deeper than the transition predicates and depend on non-committed observables. See "Scope boundary" below — increment 3 derives the read-set for the *transition-validity* predicates (publish/takedown/unrevoke/bond-reg/slash), which is what the C-7 reduction (`chain.go:2188-2246`) and increment 2's `ReadEntry` model target. The quorum-weight predicate's witness story is a separate, larger item because its inputs are not all committed.

### THE ARTIFACT — complete per-transition read-set enumeration

Notation: key = `Key(tag, rawKey)` from `core/chain/statehash.go`. Value markers: `Present` = `statehash.Present`; encoders per `statehash.go` (`EncodeInt64`, `EncodeID`, `EncodeBool`, `EncodeUint64`, `EncodeUint8`). "kind" is `QueryAbsent` / `QueryPresent`.

#### 1. Publish / entry (`ValidateEntry`, `:2338`; `apply` entries loop, `:2928`)

| Key | Raw key | Kind | Expected value | Predicate / site | Why |
|---|---|---|---|---|---|
| `tagByRoot` | `e.Root` | `QueryAbsent` | — | `:2339` `if _, exists := c.byRoot[e.Root]` | dup-root reject: the root must NOT already be committed |
| `tagSpent` | `string(e.Token.Serial)` | `QueryAbsent` | — | `:2365` `if c.spent[string(e.Token.Serial)]` | replay reject: the token serial must be unspent. Read ONLY when `tokenQuorum > 0` AND `e.Token != nil` |

Notes. `attesterQualified` is called inside `publishtoken.Verify`'s `qualified` closure (`:2368`) — this reads `slashed`/`bonded`/`epochSet`, the quorum-family reads flagged above. For the transition-validity scope, the publish read-set is `{byRoot[e.Root] absent}` plus, when a token is present, `{spent[serial] absent}`. `apply` writes `byRoot[e.Root]` and `spent[serial]` unconditionally (no committed-state branch), so `apply` adds no reads for publish.

#### 2. Revocation / takedown (`validateTakedowns`, `:2384`; `apply` revocations, `:2934`)

| Key | Raw key | Kind | Expected value | Predicate / site | Why |
|---|---|---|---|---|---|
| `tagByRoot` | `r` (each `b.Revocations`) | `QueryPresent` | `EncodeEntry`-class marker* | `:2386` `if _, ok := c.byRoot[r]; !ok` | may only revoke a root already on the ledger (F5 no-censoring-off-ledger) |

*Value caveat: `byRoot` is a Class-A set-membership field in `statehash.go` — its committed value is `statehash.Present`, NOT the entry struct. So the presence query's expected value is `Present`. `validateTakedowns` checks only *existence* (`_, ok`), so a `QueryPresent` with expected value `Present` is faithful. `apply` writes `revoked[r]` unconditionally; adds no reads.

#### 3. Un-revocation (`validateTakedowns`, `:2390`; `apply` unrevocations, `:2941`)

| Key | Raw key | Kind | Expected value | Predicate / site | Why |
|---|---|---|---|---|---|
| `tagRevoked` | `r` (each `b.Unrevocations`) | `QueryPresent` | `Present` | `:2391` `if !c.revoked[r]` | may only un-revoke a currently-revoked root |

`revoked` is Class-A (value `Present`). `apply` deletes `revoked[r]` unconditionally; adds no reads.

#### 4. Bond registration (`validateBondRegs`, `:1514`; `apply` reg loop, `:2969`)

This is the dense one. Reads come from `validateBondRegs` (gate-active branch), `restoresHeldStanding`, AND `apply`. Enumerated per registration `r` (id = `r.ValidatorID()`), only when `objective()` and (for the gated reads) `regGateActive(b.Height)`:

| Key | Raw key | Kind | Expected value | Site | Why |
|---|---|---|---|---|---|
| `tagSlashed` | `id` | `QueryAbsent` | — | `:1580` (gate) + `:2977` (apply) | a slashed id may not register / earn standing |
| `tagBondRegHeight` | `id` | present-or-absent** | `EncodeUint64(regH)` | `:1587` (gate) `if regH, ok := c.bondRegHeight[id]; ok` | R-interval: distance since last reg |
| `tagBondRootOwner` | `r.Root` | present-or-absent** | `EncodeID(owner)` | `:2980` (apply) + `:3266` (`restoresHeldStanding`) | per-root dedup: who owns this root, if anyone |
| `tagBondRootProven` | `r.Root` | present-or-absent** | `EncodeBool(proven)` | `:2986` (apply) | proof-beats-declaration displacement |
| `tagBonded` | `id` | present-or-absent** | `EncodeInt64(w)` | `:3269` (`restoresHeldStanding`) + `:2989`/`:2995` (apply) | live-standing check + displacement strip |
| `tagEpochSet` | `id` | present-or-absent** | `EncodeInt64(w)` | `:3276` (`restoresHeldStanding`) | lapsed-but-seated frozen-epoch membership |

**The absence/presence duality problem.** Six of these reads are `map[k], ok` idioms: the predicate observes *whether the key exists AND its value if it does*. A single `QueryKind` (present XOR absent) cannot faithfully model `if regH, ok := c.bondRegHeight[id]; ok`. Both outcomes are acceptance-relevant: "absent → first registration, exempt" vs "present with value regH → check the interval." **This is a real gap in increment 2's `ReadEntry` model** for the bond family. Options below (A2/A3) resolve it. `restoresHeldStanding` (`:3262`) is only reached in the mature-epoch regime, so its three reads (`bondRootOwner`, `bonded`, `epochSet`) are conditional; a conservative derivation includes them whenever the block carries a bond-reg past the gate.

`recentBondRegNonces` (`:1468`, via `b.Prev`) reads the block chain, not the committed set — no committed-set read. `verifyBond` (`:1617`) is a node-local injected verifier over the reg's own `Answer` — no committed-set read.

#### 5. Slash / equivocation (`validateSlashes`, `:1665`; `apply` slash loop, `:3017`)

| Key | Raw key | Kind | Expected value | Site | Why |
|---|---|---|---|---|---|
| (none) | — | — | — | `:1667` `VerifyEquivocation` | validity is a self-contained crypto check of the double-sign proof; reads NO committed-set key |

`apply` writes `slashed[culprit]` and deletes `bonded[culprit]` unconditionally — no committed-state branch, so slash adds no reads. **Slash is the clean case: its read-set is empty.** Worth stating because it is the counter-example that proves the enumeration is per-predicate, not a blanket "every block reads everything."

### Completeness summary table

| Transition | Read-set keys (committed-set) | Empty? |
|---|---|---|
| Publish (no token) | `byRoot[root]` | no |
| Publish (token) | `byRoot[root]`, `spent[serial]` | no |
| Revocation | `byRoot[root]` | no |
| Un-revocation | `revoked[root]` | no |
| Bond registration | `slashed[id]`, `bondRegHeight[id]`, `bondRootOwner[root]`, `bondRootProven[root]`, `bonded[id]`, `epochSet[id]` | no |
| Slash | ∅ | YES |

Quorum-stack reads (`attesterQualifiedAt`, `requireEpochWeightQuorum`, `requireDeMatureSuperQuorum`) are OUT of increment 3's scope — see the scope boundary. They read `slashed`, `bonded`, `epochSet` (committed) but also `effectiveEpochSet`/`epochStart` (NOT committed), so their witness story cannot be closed with the frozen roots alone and is a separate item.

### Part A options — where the derivation lives + the drift-guard

| # | Option | Where | Drift-guard | Cost | Verdict |
|---|---|---|---|---|---|
| A1 | Hand-written read-set table in `core/statehash`, keyed by transition type | statehash pkg | none automatic; a code-review checklist | lowest to write | REJECT — no lock-step with predicates; a new predicate read silently desyncs. This is exactly the silent-under-demand failure. |
| A2 | Derivation function in `core/chain` (reads the unexported committed fields, like the keystone oracles + `stateRootLeaves`), producing `[]ReadEntry`, with a **recording-applier drift-guard** | chain pkg | a test-mode `apply`/validate that records every committed-set key it touches; a completeness test asserts the recorded set ⊆ the derived set for a corpus of blocks | medium | RECOMMEND — see below |
| A3 | Instrument the real predicate path to EMIT its reads (a read-recording accessor injected into validate+apply), and DERIVE the read-set by running validation in record mode | chain pkg | the derivation IS the instrumented path, so it cannot desync by construction | highest; changes the hot path shape | Strong on soundness, but couples the floor box to a validate/apply refactor. Hold as the fallback if A2's guard proves insufficient. |

**Recommended: A2**, a static derivation in `core/chain` PLUS a recording drift-guard test.

The derivation must live in `core/chain`, not `core/statehash`: only `chain` can read the unexported committed fields and mirror the exact key construction (`Key(tag, rawKey)`), the same reason `stateRootLeaves` lives there. `statehash` stays chain-independent (it already carries no `chain` import).

The drift-guard is the load-bearing part. A hand-written derivation (A1) is a comment that compiles — it desyncs the first time someone adds a `c.someField[k]` read to a predicate. The guard closes it: a test harness wraps the committed-set maps in a recording accessor, runs the *real* `ValidateCommit` + `apply` over a corpus of blocks (one per transition type, plus the bond displacement/restore branches), records every committed-set key actually touched, and asserts `recorded ⊆ derived`. This is the same discipline as the keystone completeness ratchet and `stateRootLeaves`' coverage assertion — the derivation is proven complete by execution, not by inspection. It MUST ablate: delete one derived key, watch the guard go red (the exact "inject the defect, watch it go red" rule).

The guard's corpus must include the branch-triggering blocks, or it certifies nothing:
- a bond-reg that DISPLACES an unproven genesis squatter (exercises `bondRootOwner`/`bondRootProven` reads);
- a lapsed-member restore in a mature epoch (exercises `restoresHeldStanding`'s three reads);
- a token publish AND a no-token publish (the `spent` read is conditional).
A guard whose corpus omits the displacement block is green while covering nothing — the era-boundary-test scar (session-7 lesson (3)).

### Is Part A research-gated?

**Yes — the read-set completeness is a soundness surface and must be Researcher-certified before build.** The argument: the C-7 certification's load-bearing invariant is "a predicate that reads a key with no supplied witness is a REJECT/stall, never an accept" (C-7 §23/§56). That invariant is only meaningful if the floor box KNOWS which keys the predicate reads. The read-set derivation IS the enumeration of "which keys the predicate reads." An incomplete derivation defeats the C-7 invariant silently: the box never demands the witness, so there is no "missing witness" to stall on — the key simply is not on the list. Completeness of `D(b,S) ⊇ Reads(b,S)` is therefore a precondition of the certified soundness result, not an implementation detail below it. It touches the block-validity rules (which keys validity reads), which the research gate reserves. The `apply`-also-reads finding and the quorum-stack scope boundary both need the Researcher's ruling: is the transition-validity read-set (the six-transition table above) the correct closure, or must the quorum-stack reads be witnessed too for a floor box to safely accept a block? That is a soundness question about what "validate a block" means for a semi-stateless box, and it is above the Builder's authority.

Recommended question for the Researcher: *Is the per-transition read-set enumerated here complete for transition-validity, and is it sound for a floor box to accept a block having witnessed only the transition-validity reads while treating the quorum-stack (weight/qualification) reads as a separate, not-yet-witnessed concern?*

---

## Part B — D-2 on-demand delivery

### The mechanism, and the frozen-format confirmation

The floor box holds the roots, not the tree. For each `ReadEntry` in a block's read-set it fetches an SMT proof for that key against the **parent** committed root (the root the predicate evaluates against, `S` above). The witness rides as un-committed side data OUTSIDE the block: the frozen era-3 format commits only `StateRoot`/`LogRoot` (cbor 15/16) and the 18-field set (PR #632, ratified 2026-08-29). It commits NO witness field. **Confirmed: nothing in Part B touches the frozen block format** — a witness is fetched over a side channel, verified against the already-committed root, and never enters `Hash()`. In-block carry (D-1) would be a format change = a new era, and is OUT (`docs/decisions.md` §529-532; the freeze forces witnesses to travel outside the block).

### The `:557` hard requirement — the bright line

`docs/TENETS.md:557`: "Rendezvous may be centralized for convenience but never load-bearing; the decentralized path must always exist." Applied here: **no provider may hold a permission bit.** Any option where the floor box fetches witnesses from a designated/privileged/permissioned provider is a bright-line violation and is named OUT.

| Option | Verdict |
|---|---|
| Designated witness-server role (a permissioned "archival provider" the box trusts) | **OUT — `:557` violation.** A load-bearing privileged provider. Even though the witness self-verifies (so trust is not the issue), making delivery *depend* on a permissioned server makes the decentralized path non-existent. |
| Any-of-N unprivileged providers, first-correct-wins, no permission bit | **IN — the `:557`-compliant path.** |

The soundness reason the open path is safe: a witness self-verifies against the committed root (C-7 §54 — "the provider is untrusted"). So there is no reason to restrict who serves, and `:557` forbids it. Openness costs nothing on soundness and is mandatory on the tenet.

### Who serves

Any node holding the SMT tree at the parent root. Per the #600 ruling, the tree lives a tier ABOVE the floor box — archival or pruning-but-tree-keeping nodes. The floor box sources from ANY of them. There is no registry of "witness servers"; a provider is any peer that answers a witness request correctly, discovered through the existing provider/peer plane. First-correct-wins: the box takes the first proof that verifies against its root and cancels the rest.

### Part B options — the provider fan-out + deadline

| # | Option | Fan-out | Deadline | Slow-loris (R-loris) close | Verdict |
|---|---|---|---|---|---|
| B1 | Serial: ask one provider, on failure ask the next | 1 at a time | per-provider `requestTimeoutFor` | weak — a slow-loris provider burns the full deadline before fallback; N providers = N×deadline worst case | REJECT — the R-loris residual stays open; a hostile provider that trickles bytes stalls the box for the full ladder |
| B2 | Parallel any-of-N sweep, first-correct-wins, per-provider read deadline + `FetchAttempts` re-sweep | K-wide parallel | per-provider deadline via `requestTimeoutFor`, size-extended by `RequestSizeFloorBytesPerSec` | strong — a slow provider is abandoned at its deadline while a parallel honest provider wins; withholding never blocks | RECOMMEND |

**Recommended: B2**, reusing the existing fetch machinery verbatim.

The existing machinery already implements exactly this shape for chunk fetch, and it must be reused rather than reinvented (the derive-don't-drift discipline):

- `FetchAttempts` (`core/node/node.go:143`, default 3) — the higher-level re-sweep across providers, skipping known-dead holders via the negative cache. This is the any-of-N fallback loop.
- `RequestSizeFloorBytesPerSec` (`node.go:61`, default 256 KiB/s) — the size-extended per-request deadline (`requestTimeoutFor`, `:1198`). A witness bundle is small (≤ `C_block` = `len(read-set)·16 KiB`), so it gets ~the base deadline; this is the per-provider read deadline that bounds slow-loris.
- The holder-fetch fail-fast + no-same-holder-retry pattern (`node.go:1255-1263`) — a witness fetch is speculative like a chunk fetch (the provider may be gone or hostile), so it should fail fast on its deadline and fall through to the next provider, NOT retry the same one.

**No new knob.** The R-loris close is `requestTimeoutFor` applied to the witness request kind, with `FetchAttempts` as the fan-out. This is the same deadline+re-sweep that bounds chunk-fetch slow-loris today. Introducing a new witness-specific timeout constant would be the #104/#286 derive-don't-drift scar. Flag: if the build finds the base `RequestTimeout` is wrong for a witness reply (as `MsgGetChain` needed `maxChainReplyBytes` size-extension), the fix is to derive the witness deadline from the SAME symbols, never a fresh literal.

### The wire message shape

A new mesh RPC kind, e.g. `MsgGetWitness{root, key}` → `MsgWitness{key, encoded-proof}`, sitting alongside the existing `MsgGetChain`/`MsgFetchChunk` kinds. The proof bytes are the `RawWitness.Encoded` (gob-marshaled `SparseMerkleProof`) that increment 2's `IngestBlockWitnesses` already consumes. The response is un-committed side data: it is verified against the box's own committed root and discarded if it fails. The request carries the parent root so the provider knows which tree version to prove against.

### The failure-to-`NoWitness` coupling — the load-bearing wiring

Every Part B failure MUST feed increment 1's `NoWitness` arm, NEVER `ProvenAbsent`:

| B failure | Feeds | Never |
|---|---|---|
| Timeout (no provider answered in the deadline) | `NoWitness` (stall, re-fetch next sweep) | `ProvenAbsent` |
| Withholding (provider returns nothing / partial) | `NoWitness` | `ProvenAbsent` |
| Wrong-shape / unparseable proof | `NoWitness` (already handled: `IngestBlockWitnesses` maps an unmarshal failure to `NoWitness`, `witness_bound.go`) | `ProvenAbsent` |
| Proof verifies against a DIFFERENT root (stale provider) | `NoWitness` (`Resolve` returns `NoWitness` on verify-fail) | `ProvenAbsent` |
| Over-budget bundle (R3) | `NoWitness` for every key (`allNoWitness`) | `ProvenAbsent` |

This is the single banned move (C-7 §104). It is already structurally enforced downstream: `Resolve` is the ONLY producer of `ProvenAbsent`, and it produces it only from a verified non-membership proof. Part B's job is to hand a *missing* witness to the accessor as a `nil` proof / absent bundle entry, which resolves to `NoWitness`. Part B must NOT synthesize an "empty" `ReadEntry` value that would route to the absence branch — that is the empty-value scar (the R4-a `len(value)==0` finding and the increment-2 `QueryPresent`-with-empty-`Value` reject). A withheld witness for a `QueryPresent` key is a stall, not a proven absence.

### On-demand vs pre-supplied: who computes the witness

The proposer could pre-compute and gossip witnesses, or each validator fetches on demand. The freeze forces witnesses outside the block either way. On-demand (the box pulls what its read-set needs) is recommended: it is the any-of-N shape, it needs no proposer cooperation (a Byzantine proposer withholding witnesses just means the box fetches from someone else — degrades to stall, never to accept), and it composes with `FetchAttempts` directly. Proposer-push is an optimization that can layer on later without changing the soundness story.

### Is Part B tenet-gated?

**Yes — the any-of-N-no-permission rule is a `:557` invariant, and weakening it is a human veto gate.** The recommended B2 does NOT weaken it (it is the open path). The gate fires only if a future build proposes a permissioned/designated provider for performance. That trade — load-bearing centralization of witness delivery — is an immutable-adjacent decision (`:557` is a tenet-tier line) and STOPs for human ratification. No numeric security parameter is new in Part B: the deadline and fan-out are the existing ratified `RequestTimeout`/`FetchAttempts`/`RequestSizeFloorBytesPerSec`. If the build finds a new constant is needed, that constant routes to Research (a delivery-deadline that a DoS bound depends on).

---

## Couplings (explicit)

1. **A defines what B fetches.** The read-set from Part A is the exact set of keys Part B requests witnesses for. B's fan-out is per read-set key; the honest bundle is exactly `len(read-set)` proofs (increment 2's shape gate already pins this).
2. **B's failures feed R3/R4's `NoWitness`, never `ProvenAbsent`.** Timeout / withholding / wrong-shape all stall. Enforced structurally downstream; B must not fabricate an absence.
3. **A's over-inclusion is B's wasted fetch, not a soundness bug.** If A errs toward including a key no predicate reads, B fetches one extra proof and increment 2's shape gate accepts it (the provider supplies it). This is why A must never under-include and may over-include.
4. **The parent root is the anchor for both.** A derives reads against parent state `S`; B fetches proofs against the SAME parent root `R`. A witness proved against the wrong root fails `Resolve` → `NoWitness`. This ties to the era-3 Reload root-check gap ([[era3-reload-root-check-gap]]): the box must verify against the ANCHOR-bound parent root, never a re-signed wrong root.

---

## Gate classification (the explicit list)

| Item | Gate | Why | Who ratifies |
|---|---|---|---|
| Part A read-set completeness (the six-transition enumeration + the `apply`-also-reads closure) | **RESEARCH-GATED** | it is a precondition of the C-7 certified soundness invariant; touches what "validate a block" reads | Researcher certifies, human ratifies |
| Part A: whether quorum-stack reads must be witnessed (the scope boundary) | **RESEARCH-GATED** | soundness question about semi-stateless block acceptance; involves non-committed observables (`epochStart`) | Researcher |
| Part B any-of-N-no-permission delivery (B2) | **TENET-GATED** (satisfied as recommended) | `:557` — no load-bearing privileged provider. B2 complies; a permissioned variant is a human veto gate | human, only if `:557` is weakened |
| Part A drift-guard test + derivation function (A2) | **BUILDABLE** once A's completeness is certified | mechanical mirror of the certified read-set, guarded by a recording completeness test | Builder |
| Part B wire RPC + fetch fan-out reusing existing knobs (B2) | **BUILDABLE** | reuses ratified `FetchAttempts`/`RequestTimeout`/`RequestSizeFloorBytesPerSec`; no new parameter | Builder |
| The `NoWitness`-not-`ProvenAbsent` failure wiring | **BUILDABLE** (already enforced downstream) | structural in increments 1+2; B just hands a missing witness as absent-entry | Builder |

## Consensus-rule / frozen-format check

- **Nothing proposed touches the frozen era-3 format.** Witnesses ride outside the block; `Hash()` is untouched. Confirmed against `core/chain/statehash.go` (18 tags + two roots) and the freeze (#632).
- **Nothing proposed touches I1–I5.** The read-set derivation READS the validity predicates; it does not change them. Delivery is a transport concern below consensus. The one place that brushes consensus — "must the quorum-stack reads be witnessed" — is flagged OUT of scope and routed to Research precisely because it could touch the block-validity rules.
- **No new numeric security parameter.** R3's caps are ratified (`S_proof_max`, `C_block`). Part B reuses existing ratified deadline/retry knobs. Any new constant that appears in build routes to Research.

## Scope boundary (what increment 3 does NOT do)

- It does NOT witness the quorum-stack reads (`attesterQualifiedAt`, `requireEpochWeightQuorum`, `requireDeMatureSuperQuorum`). Those read non-committed observables (`epochStart`, `effectiveEpochSet` recovery boundary) and are a separate, larger design item.
- It does NOT build in-block witness carry (D-1 — a format change, OUT).
- It does NOT re-open R3's ratified caps or R4's accessor.
- It does NOT add a permissioned provider (`:557`, OUT).

## Recommended path

1. File Part A's six-transition enumeration + the `apply`-also-reads closure + the scope boundary to the Researcher for a completeness certification. Do not build A until certified.
2. In parallel, the PE reviews Part B's B2 (any-of-N, existing-knob deadline, `NoWitness` wiring) blind — it is buildable and tenet-compliant, but the `NoWitness`-not-`ProvenAbsent` wiring and the no-permission rule are the review's load-bearing checks.
3. On certification, build A2 (derivation + recording drift-guard, ablated) then B2 (wire RPC + fan-out reusing `FetchAttempts`), each with a regression test at its tier.
