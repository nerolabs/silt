# Witness floor-box validation mechanism — design options

Status: PACE deliberation (options only, no code). Date: 2026-08-29. Author: Builder.
Scope: the THREE open construction items the era-3 format freeze deliberately left open —
R3 (witness size / DoS bound), Delivery (generation + carry/gossip), R4 (omission vs.
proven-exclusion accessor). The DIRECTION is ratified and NOT re-litigated here: the floor
box validates by witness against the committed roots, hold-the-tree is a bigger-box opt-in
behind `ports.NodeStore`, witness-serving is open + multi-provider, and "no witness → stall,
never accept" is already built.

## Hard constraints this doc operates under

- **The era-3 format is FROZEN (immutable as of `3af40bc`, ratified 2026-08-29).** No option
  here may change `Block.StateRoot`/`LogRoot`, the 18-field `committedSet`, the field-tag key
  encoding, the value encodings, or the v4 activation. Any option that would require a format
  change is OUT — it would need a NEW era (a new `BlockVersion`), which is a separate ratified
  decision, not a build. `docs/decisions.md` FROZEN entry §598-633.
- **Witness soundness is CERTIFIED (C-7).** The verifier reproduces every validity predicate
  from membership + non-membership proofs against the committed root; the three forgeries
  (partial / replayed / stale) are rejected. This doc does NOT re-derive soundness; it
  designs the delivery/DoS/accessor layer AROUND the certified verifier.
- **The banned move stays banned.** "No witness supplied for a key a predicate reads → never
  accept (reject / stall)." Every option preserves this; none may weaken it for liveness.
- **Consensus engine untouched (D-CONSENSUS §5).** Witness handling is a validation-input and
  gossip/transport concern. It adds no round, no quorum rule, no fork-choice input. Anything
  that would touch I1-I5 is flagged as a hard stop and routed to Research, not built.
- **The 2 GB / 1-vCPU floor-box promise** (VISION L23, L154-169) is the sizing budget every
  option is scored against: the floor box holds the two roots and a bounded per-block witness,
  never the tree.

## What the predicates actually read (the witness contents, grounded)

Verified against `core/chain/chain.go:2338` (`ValidateEntry`) and the C-7 predicate table.
A block's witness must carry a proof for every committed-set key each of its transitions
reads. The reads are bounded by the block payload, not by the registry size:

| Transition in the block | Predicate read | Proof class | Committed field |
|---|---|---|---|
| each `Entry.Root` | `byRoot[Root]` must be ABSENT | non-membership | `byRoot` |
| each `Entry.Token.Serial` | `spent[Serial]` must be ABSENT | non-membership | `spent` |
| each `Revocation` root `r` | `byRoot[r]` must be PRESENT | membership | `byRoot` |
| each `Unrevocation` root `r` | `revoked[r]` must be PRESENT | membership | `revoked` |
| each `BondReg` / `Slash` | bonded/epochSet/regVersion reads for weight + gating | membership + value | `bonded`, `epochSet`, `regVersion`, … |

Key fact for sizing: the number of proofs in a witness is `O(payload transitions in the
block)`, and each proof is `O(log n)` sibling hashes where `n` is the committed-set size.
The pokt SMT already bounds a single proof's side-node count to `PathSize()*8 = 256`
(`proofs.go:55-58` `validateBasic`, "cause a CPU DoS attack" is the library's own words).
So the per-KEY witness is already bounded; the open item is the per-BLOCK aggregate.

---

# Item R3 — witness per-block size bound / DoS budget

## The adversary, named precisely

Two distinct adversaries, different bounds:

- **A-produce (malicious block-producer):** proposes a v4 block and attaches (or induces the
  fetch of) a witness far larger than the block's transitions justify — padded proofs,
  duplicate proofs, proofs for keys no predicate reads, or maximally-deep side-node arrays.
  Goal: exhaust an attester's or floor box's memory/CPU during validation, or inflate gossip.
- **A-serve (malicious witness provider, on-demand path):** when the floor box FETCHES
  witnesses rather than receiving them in-block, a hostile provider returns an oversized or
  slow-to-verify blob. Goal: same exhaustion, plus a slow-loris stall.

The bound must be **checkable before the expensive work** (before allocating the full witness,
before verifying every proof). A bound checked only after parsing is not a DoS bound.

## The bound, precisely

Worst-case legal witness bytes per block:

```
maxWitnessBytes(block) = Σ over the keys the block's transitions read of
                         (perProofCeiling)
perProofCeiling        = 256 side-nodes × 32 bytes  +  NonMembershipLeafData ceiling
                       + gob/marshal envelope
```

The number of keys is itself bounded by the block's transition count, which is bounded by the
existing block payload caps. This is the I4 #441 discipline (entry budget separated from reg
budget) applied to witnesses: the witness budget is DERIVED from the payload budget, never a
standalone number that can drift. It is the same "derive, don't drift" rule the transport
already uses — `maxFrame = manifest.MaxChunkSize + frameOverhead` (`tcpnet.go:72`, the #104
lesson: a standalone cap silently dropped every production chunk).

## Options

| Option | Where enforced | Soundness | 2GB fit | DoS surface | Complexity |
|---|---|---|---|---|---|
| **R3-a: derived per-block ceiling, checked pre-verify** | at witness ingest, before proof verification, in the floor-box/attester validation path | strong — rejects an over-budget witness before allocating/verifying it | strong — bound is `O(payload)` × `O(log n)`, kilobytes not megabytes | small — over-budget = reject, checked cheaply from counts | low — a counting check + a size compare |
| **R3-b: per-proof cap only (rely on the library's 256 side-node guard)** | inside the SMT verify (`validateBasic`) | per-key sound, but NO aggregate bound | weak — a block with a legal-but-huge transition count could still aggregate a large witness | medium — aggregate exhaustion still reachable | none (already exists) |
| **R3-c: a single flat "max witness bytes" constant** | at ingest | sound | ok, but arbitrary | small | low, but VIOLATES derive-don't-drift — a constant drifts from the payload caps (#104 scar) |
| **R3-d: streaming/lazy verify, no upfront size cap** | verify proofs one at a time, abort on first over-budget cumulative cost | sound | ok | medium — a slow-loris on the on-demand path can hold the connection | medium — stateful streaming accounting |

## Recommendation — R3-a, layered on top of R3-b

Adopt **R3-a (derived per-block ceiling, checked pre-verify)** as the primary bound, and KEEP
**R3-b** (the library's 256-side-node per-proof guard) as the inner defense. Rationale:

- The two-layer shape matches the certified structure: per-KEY boundedness is already proven
  and library-enforced (R3-b); the OPEN item is exactly the per-BLOCK aggregate the C-7
  residual named. R3-a closes precisely that gap and nothing more.
- Deriving the ceiling from the payload caps (not a flat constant) obeys the #104
  derive-don't-drift scar and the I4 #441 separated-budget discipline. This is the difference
  between R3-a and R3-c; R3-c is the tempting shortcut that the transport-cap scar says not to
  take.
- Checked pre-verify defeats A-produce cheaply: an attester counts the block's transitions,
  computes the ceiling, and rejects an over-budget witness before verifying a single proof.
- The check lives in the floor-box/attester VALIDATION path, NOT in consensus. It rejects a
  block's witness as malformed input; it does not change any I1-I5 rule.

## Security-parameter flag (routes to Research, does NOT block this doc)

The **numeric ceiling constant(s)** — the per-proof envelope ceiling and the payload-to-witness
multiplier — are a SECURITY PARAMETER a DoS bound rests on. Per the CLAUDE.md research gate and
the build-process "a durability knob was twice a security parameter" scar, the Builder proposes
the DERIVATION (witness ≤ f(payload caps)); the Researcher certifies the constants are
conservative against A-produce and A-serve before they are pinned. The DERIVATION SHAPE
(derived-not-flat, checked-pre-verify) is a build decision; the NUMBERS are gated.

---

# Item Delivery — witness generation + carry/gossip

## The tenet this item defends

`docs/TENETS.md:557` (decentralization may be convenient, never load-bearing) + VISION L163-166:
witness-serving is an OPEN, UN-PERMISSIONED responsibility of the tiers above, so the floor
box's liveness dependency stays decentralized, never a single-provider choke. Any option that
introduces a privileged or permissioned witness provider is OUT — it is a bright-line violation,
not a trade.

## Who generates

Grounded fact: a witness is a set of proofs against the block's committed `StateRoot`, over the
PRE-transition state (the state the predicate reads). Any node that holds the registry tree at
the parent block's state root can generate the witness. That is the tier-above set: **archival
nodes (full history) and pruning nodes (rolling horizon, but they hold the current tree).** The
proposer holds the tree while proposing, so the proposer is the natural first generator. But the
scheme must NOT depend on the proposer being the only source, or the proposer becomes a
privileged provider (tenet violation) and a liveness choke.

## Options

| Option | Open/multi-provider | Liveness | DoS surface | 2GB fit (floor box) | Complexity | Format touch? |
|---|---|---|---|---|---|---|
| **D-1: witness carried IN-BLOCK** | any tier-above node can re-derive and re-gossip the block+witness; no privileged fetch | strong — witness arrives with the block, no second round-trip | larger gossip payload (mitigated by R3-a); a bad witness = reject the block | floor box never fetches; validates on receipt | low | **YES — OUT.** the frozen format has no witness field; carrying in-block is a NEW era. |
| **D-2: witness served ON-DEMAND, any-of-N providers** | strong — floor box asks any of N tier-above peers; no permissioned provider | good — depends on ≥1 reachable honest provider (the certified irreducible residual E6) | on-demand path exposes A-serve (slow-loris); bounded by R3-a + a fetch timeout | floor box holds roots, fetches bounded witness per block, discards after | medium — a fetch protocol + provider selection | **NO** — witness travels outside the block, over gossip/RPC, not in the committed format |
| **D-3: witness GOSSIPED separately, keyed to block hash** | strong — decoupled from proposer; any holder gossips | good — but a floor box may receive the block before its witness (transient stall) | gossip amplification; dedup by (blockHash, keyset) | floor box buffers block until witness arrives, bounded | medium-high — a second gossip topic + correlation | **NO** — separate channel, not a committed field |

## The couplings that decide this

- **D-1 is foreclosed by the freeze.** The frozen block schema commits `StateRoot`/`LogRoot`
  and nothing else witness-related. Adding an in-block witness field is a format change → a NEW
  era. So in-block carry is not available to this build. This is the single most important
  constraint on Delivery: **the frozen format forces witnesses to travel OUTSIDE the block.**
  That is not a defect — the format deliberately commits the ROOT (the trust anchor) and leaves
  the witness (untrusted, self-verifying, discardable) to the delivery layer, which is exactly
  the stateless-client shape (the root is consensus, the witness is transport).
- **D-2 and D-3 both preserve the tenet** iff provider selection is any-of-N with no permission
  bit. The design rule: the floor box MUST accept a valid witness from ANY peer that offers one,
  selected by liveness (first correct response wins), never by identity/allowlist. A witness is
  self-verifying against the committed root (C-7), so trusting the source is unnecessary AND
  forbidden — trusting a source would be the load-bearing-centralization the tenet bans.

## Recommendation — D-2 (on-demand, any-of-N) as the floor default, with D-3 as an opt-in acceleration

- **D-2 is the floor default.** It is the minimum mechanism that ships the ratified posture:
  the floor box holds the roots, fetches a bounded witness per block from any-of-N tier-above
  providers, verifies it against the committed root, and discards it. It does not touch the
  frozen format (witness rides an RPC/gossip response, not the block). It realizes the certified
  E6 liveness residual honestly: liveness depends on ≥1 reachable honest provider, priced
  correctly (stall-not-accept on failure).
- **D-3 (separate gossip) is a later acceleration, not the floor.** Push-gossiping witnesses
  reduces the block-then-fetch round-trip, but adds a second gossip topic and block/witness
  correlation buffering. Ship D-2 first (simplest thing that realizes the posture); add D-3 only
  if measured floor-box stall time on the fetch round-trip justifies it. Do not gold-plate D-3
  into the first cut.
- **Reject any privileged-provider shortcut.** A "designated witness server" or a
  proposer-must-serve rule would be simpler to implement but violates TENETS:557. The any-of-N
  selection is the non-negotiable core; it is cheap (first-correct-wins) and it is the whole
  point of the open-multi-provider requirement.

## Consensus-rule flag

Delivery is a transport/gossip concern and touches NO consensus rule — the block, its roots,
and their attestation are unchanged; the witness is validation INPUT the verifier checks against
the already-committed root. No I1-I5 impact. This is buildable without a Research gate, PROVIDED
the any-of-N-no-permission rule is held as a hard invariant (it defends TENETS:557, an immutable
tenet; if a future option ever proposes weakening it, THAT routes to the human veto gate).

---

# Item R4 — omission vs. proven-exclusion accessor

## The exact hazard

The floor box must distinguish two states that look similar at the call site but must NEVER be
conflated:

- **"K is verifiably ABSENT from the committed set"** — a valid non-membership proof of K
  against the committed root. This is a PROVEN EXCLUSION. The predicate that wants K absent
  (e.g. `spent[Serial]` must be absent for a valid publish) is SATISFIED.
- **"I have NO witness for K"** — no proof was supplied/fetched for K. This is OMISSION. The
  predicate CANNOT be evaluated. The floor box must STALL, never read this as "K is absent."

The banned bug: an accessor that returns "absent / not-found / false" for BOTH cases. That
turns a missing witness into a verified exclusion and inverts the safe-degradation proof — it is
the exact "no witness → accept" move C-7 §Q2 names as the one implementation error that breaks
soundness. This is item R4: the accessor's TYPE and semantics must make the two cases
un-conflatable.

## The three-valued accessor (the core of every option)

The accessor over a committed field for key K must return one of THREE outcomes, never two:

```
lookup(field, K) ->  PROVEN_PRESENT(value)   // valid membership proof verified
                 |   PROVEN_ABSENT           // valid non-membership proof verified
                 |   NO_WITNESS              // no proof, or proof failed to verify -> STALL
```

A two-valued (bool / found-or-not) accessor is the bug. The design must force the caller to
handle NO_WITNESS explicitly — a stall, never a silent false. In Go terms this is an explicit
third state (an enum/sentinel error), not a `(value, bool)` where false doubles as both absent
and unknown.

## The sharded-registry sub-problem (C-7 §11.2)

When the tier-above registry is sharded across providers, PROVEN_ABSENT gets harder: "not in
my shard" is not "not in the committed set." A floor box that accepts one provider's "not in my
shard" as PROVEN_ABSENT can be lied to by omission. Because silt commits a SINGLE state SMT root
over the WHOLE committed set (the frozen format: one keyspace, field-tagged, `statehash.go`), the
authoritative exclusion is a non-membership proof against THAT single root — which any provider
holding the tree at that root can produce, and which is complete by construction (the SMT's
non-membership is over the whole keyspace, not per-shard). So the frozen single-root format
DISSOLVES the cross-shard omission problem for the exclusion PROOF: a valid non-membership proof
against the committed root IS a whole-set exclusion, regardless of how providers shard their
STORAGE. Sharding is a storage/serving concern below the root, not a soundness concern above it.

## Options

| Option | Conflation-proof | Soundness | Complexity | Notes |
|---|---|---|---|---|
| **R4-a: three-valued accessor (PROVEN_PRESENT / PROVEN_ABSENT / NO_WITNESS)** | strong — the third state is explicit and un-skippable | strong — NO_WITNESS forces stall, matches C-7 banned-move invariant | low | the certified shape; the missing-witness state is a first-class value |
| **R4-b: two-valued (value, ok) with a SEPARATE "have I a witness" flag** | weak — two flags the caller can mis-combine; the bug is one `&&` away | sound only if every caller checks both | low code, HIGH footgun | rejected — reintroduces exactly the conflation R4 exists to prevent |
| **R4-c: exception/panic on NO_WITNESS** | strong — impossible to ignore | sound but conflates "stall" (recoverable, retry the fetch) with "invalid" (reject) | low | too blunt: a missing witness is a STALL-and-retry, not a fatal — panicking loses the safe-degradation nuance |

## Recommendation — R4-a (three-valued accessor)

- **R4-a is the certified shape and the simplest thing that is correct.** The NO_WITNESS state
  is a first-class return the caller cannot silently treat as absent. It directly encodes the
  C-7 invariant "no witness for a read key → stall, never accept."
- **R4-b is rejected** despite being marginally less code: a `(value, ok)` bool where `ok=false`
  serves double duty as both "proven absent" and "unknown" is the precise footgun R4 exists to
  eliminate. The build-immutable evidence discipline says name the failure mechanism — the
  mechanism here is exactly a two-valued accessor read as three-valued by an inattentive caller.
- **R4-c is rejected** because it flattens stall (retry the fetch from another of the N
  providers — the D-2 liveness path) into reject (the block is invalid). Those are different
  outcomes; a missing witness is a liveness event handled by re-fetch, not a validity verdict.
- **The sharded-omission concern is closed BY THE FROZEN SINGLE-ROOT FORMAT**, not by a new
  cross-shard protocol: exclusion is proven against the one committed root, so "not in my shard"
  is never accepted as exclusion — only a whole-keyspace non-membership proof is. This is a
  benefit the freeze already banked; R4 must not reintroduce per-shard exclusion.

## Consensus-rule flag

R4-a is a validation-path accessor, not a consensus rule. But it is the DIRECT encoding of the
certified banned-move invariant, so the accessor's NO_WITNESS→stall semantics are effectively a
soundness-load-bearing contract. Recommend the accessor's three-valued contract and its
regression test (a NO_WITNESS injection that must produce stall, never accept — ablate the
green) be reviewed against C-7 §Q2/§constraints before it ships, so the built accessor matches
the certified invariant exactly. No I1-I5 change; the review is a soundness-fidelity check, not
a new consensus gate.

---

# Couplings across the three items

1. **R3 ↔ Delivery.** The per-block witness byte ceiling (R3-a) is what makes BOTH the in-block
   (foreclosed) and on-demand (D-2) paths DoS-safe. Because the freeze forces witnesses outside
   the block (D-1 is out), R3-a must also bound the ON-DEMAND fetch (A-serve slow-loris), i.e.
   R3-a applies at fetch-ingest with a fetch timeout, not only at block-ingest. R3 and Delivery
   are co-designed: the bound lives at the delivery boundary.

2. **Delivery ↔ R4.** D-2's any-of-N re-fetch is the LIVENESS handler for R4's NO_WITNESS state.
   When the accessor returns NO_WITNESS, the floor box does not reject — it re-fetches from
   another of the N providers (D-2) and stalls only if all fail. R4's third state and D-2's
   provider selection are the two halves of one safe-degradation loop: NO_WITNESS → re-fetch →
   stall-if-exhausted, never → accept.

3. **R3 ↔ R4.** A witness that FAILS the R3 size bound must map to NO_WITNESS (stall/re-fetch),
   NOT to PROVEN_ABSENT. An over-budget or malformed witness is an absent witness, not an
   exclusion proof. The R3 rejection path must feed the R4 three-valued accessor's NO_WITNESS
   arm, never its PROVEN_ABSENT arm. This is the subtle conflation to guard: "the witness was
   too big so I dropped it" must never read downstream as "the key is absent."

4. **All three ↔ the frozen format (hard constraint).** The freeze commits the ROOT and forces
   the WITNESS outside the block. That single fact drives: D-1 is out (Delivery), the bound lives
   at the delivery boundary not a block field (R3), and exclusion is proven against the one
   committed root so sharding is a storage concern not a soundness one (R4). The freeze is not
   an obstacle to these items — it is the anchor that makes all three clean: consensus commits
   the trust anchor, the delivery layer moves the untrusted self-verifying witness.

---

# Summary of recommendations

| Item | Recommendation | One-line rationale |
|---|---|---|
| R3 (size/DoS) | **R3-a** (derived per-block ceiling, checked pre-verify) over **R3-b** (library per-proof guard) | derive-don't-drift (#104) + check before the expensive verify; numbers gated to Research |
| Delivery | **D-2** (on-demand, any-of-N, no-permission) floor default; **D-3** a later acceleration | the frozen format forces witnesses outside the block; any-of-N defends TENETS:557; D-1 is out |
| R4 (omission) | **R4-a** (three-valued PROVEN_PRESENT / PROVEN_ABSENT / NO_WITNESS accessor) | the third state makes "missing witness" un-conflatable with "proven absent"; single-root freeze dissolves the shard problem |

# Flags

- **Security parameter (Research gate):** the R3 numeric ceiling constants. Builder proposes the
  derivation shape (witness ≤ f(payload caps), checked pre-verify); Researcher certifies the
  numbers are conservative against A-produce and A-serve before they are pinned.
- **Immutable tenet (human veto gate if ever weakened):** Delivery's any-of-N-no-permission rule
  defends TENETS:557. It is buildable now as-is; any future proposal to permission the provider
  set routes to the human.
- **Soundness-fidelity review (C-7):** the R4 accessor's NO_WITNESS→stall contract is the direct
  encoding of C-7's banned-move invariant. Its regression test must ablate the green (inject a
  NO_WITNESS and confirm stall, never accept) and be checked against C-7 §Q2 before it ships.
- **No consensus-rule change in any recommended option.** All three live in the validation-input
  / delivery layer; I1-I5 and the frozen format are untouched. The format cannot change; only a
  new era could — and none of these items need one.
