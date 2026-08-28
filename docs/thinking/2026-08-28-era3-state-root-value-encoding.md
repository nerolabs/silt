# era-3 StateRoot value encoding — the per-field canonical byte encoding (build step 1)

**Date:** 2026-08-28
**Seat:** Builder (BUILD mode — step 1 of the certified sequence)
**Status:** decides the load-bearing encoding both consults flagged highest-severity.
Written BEFORE code, per PACE-BEFORE-CODE. Ships in the step-1 PR.

**Inputs (read, verified against source):**
- Research certification `era3-committed-state-root-format-RESEARCH-CERTIFICATION-2026-08-28.md`
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/`) — Q2
  CERTIFIED, Q6 flag 2, residual R2 (this doc discharges R2's encoding half).
- PE ruling `RULING-era3-committed-state-root-format-2026-08-28.md`
  (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/`) — Q2 highest
  severity: three super-quorum predicates SUM `bonded`/`epochSet` weights, so a
  wrong-value witness is a consensus-SAFETY attack, not a read bug.
- Design options `docs/thinking/2026-08-28-era3-format-design-options.md`.

## Scope of step 1 (the STOP boundary)

This step computes two roots and proves the encoding deterministic. It does NOT add
any `Block` field, does NOT change `Hash()`, does NOT add a validity predicate, does
NOT touch `BlockVersion`/`versionSupported`. Those re-trigger certification and are
later steps. Step 1 lives at the model-check tier: the root-computation machinery goes
behind the keystone oracles so a value-encoding defect is caught as a test failure
BEFORE it can become a signed field.

## Why the encoding is a consensus parameter, not formatting (the frame)

The pokt SMT binds the leaf value into the leaf digest (cert Q2, decisive artifact:
`smt@v1.0.0/proofs.go:428-429`, `hasher.go:111-114` — the value is inside the leaf
preimage, so it propagates to the root). Therefore the root is a function of the exact
leaf BYTES. Two honest nodes that encode the same logical value to different bytes
compute different roots — a consensus split. The width and endianness of every
value-carrying field are consensus parameters (cert Q6 flag 2; a width/endianness
change is a hard fork). This doc PINS them.

## The three field classes (verified against `chain.go:804-930`)

The keystone classification (`modelcheck_state_completeness_test.go:81-96`) enumerates
16 `committedSet` fields. They fall into three encoding classes:

### Class A — set-membership (value = fixed presence marker `[]byte{1}`)

The predicate reads EXISTENCE, never a value. The leaf value carries no information;
security rests on key presence/absence (the SMT serves both inclusion and exclusion
proofs — `internal/smtspike/exclusion_test.go`).

| field | Go type | why membership-only |
|---|---|---|
| `revoked` | `map[Hash]bool` | `validateTakedowns` reads membership (un-revoke target) |
| `spent` | `map[string]bool` | `ValidateEntry` replay-reject reads membership |
| `slashed` | `map[NodeID]bool` | qualification refuses a slashed id — membership |
| `validatorsSeen` | `map[NodeID]bool` | C2Metric enumerates the SET |
| `byRoot` | `map[Hash]Entry` | **existence-only — RESOLVED, see below** |

**`byRoot` hedge, RESOLVED (PE Q2 required this before freeze).** I grepped every
product-code read of `byRoot` (`chain.go:2235, 2282, 3338`). All three validity-path
reads test EXISTENCE (`_, exists := c.byRoot[e.Root]`; `_, ok := c.byRoot[r]`).
`LookupRoot` (`chain.go:3338`) returns the Entry but is a query ACCESSOR, not a
validity predicate. So no predicate reads the Entry body. `byRoot` is Class A
(membership marker) for the state root. **Reopening condition:** if any future validity
predicate reads the `Entry` body (FileSize, ManifestChunks, Publisher, Token) rather
than mere existence, `byRoot` graduates to Class B and must commit a canonical digest
of the Entry. The completeness guard catches a new FIELD but NOT a new READER of
`byRoot`'s body — this is a named residual, mirrored from the cert's `epochStart`
observation.

### Class B — value-carrying (value = fixed canonical encoding of the field's value)

The predicate reads the VALUE. A presence-only marker would let a witness prove
presence without pinning the value a predicate reads — the cert/PE forgery. Each width
below is justified against the field's declared Go type and the domain it must cover.

| field | Go type | encoding | width justification |
|---|---|---|---|
| `bonded` | `map[NodeID]int64` | 8-byte big-endian, two's-complement | int64 is 8 bytes; **summed in 3 super-quorum tallies** (`chain.go:2527-2536, 2593-2604, 3150-3152`) — a truncated width diverges the leaf from the summed value. See sign note. |
| `epochSet` | `map[NodeID]int64` | 8-byte big-endian, two's-complement | same as `bonded` — frozen quorum weights, same summing hazard |
| `bondRegHeight` | `map[NodeID]uint64` | 8-byte big-endian | uint64 is 8 bytes; TTL clock + #506 R-rule distance reads the exact height |
| `bondDomain` | `map[NodeID]uint64` | 8-byte big-endian | uint64 is 8 bytes; C2Metric A-axis label; `0 = unset` is a legal in-domain value (see zero note) |
| `bondRootOwner` | `map[Hash]NodeID` | raw 32 bytes | NodeID = Hash = `[32]byte` (`ports/net.go:82`, `ports/ports.go:17`); the value IS the owner identity (F1 dedup reads BY WHOM); no transform, no length prefix |
| `bondRootProven` | `map[Hash]bool` | 1 byte, `0x00`/`0x01` | the bool distinguishes proven from declared (G3); a presence marker collapses that distinction — commit the bool |
| `regVersion` | `map[NodeID]uint8` | 1 byte | uint8 is 1 byte; #506 lock-in tally reads the exact version |

**Sign note (PE Q2, `bonded`/`epochSet`).** These are `int64` and the code treats them
as non-negative weights, but the encoding must be TOTAL over the declared int64 domain.
Two's-complement big-endian is total and order-agnostic here (we commit the value, we do
not sort by it). The encoder does NOT reject or canonicalize negatives — it encodes the
raw int64 bits, so a negative weight (which apply() never produces for a valid bond, but
which is representable) maps to a distinct, unambiguous leaf. This is stricter than
"reject negatives at the encoder": encoding the raw bits means the leaf is a faithful
1:1 image of the in-memory int64, so the root can never disagree with the value a
predicate sums. Rejecting at the encoder would instead be a validity concern (a later
step), not an encoding concern; step 1 must not smuggle a validity rule into the encoder.

**REOPENING CONDITION — the negative-weight safety is a THREE-LAYER property.** The
total encoder is safe to ship WITHOUT a negative-value reject only because two other
layers already keep a negative weight out of the committed set. All three must hold:

1. **The total encoder** (this doc): a negative int64 maps to a distinct, unambiguous
   leaf. It faithfully commits whatever value the map holds — it does not police it.
2. **The `Size < MinBond` reject at `core/chain/chain.go:1521`** — a bond registration
   with `r.Size` below `MinBond` (which is `> 0`) is rejected at validity, so no
   sub-`MinBond` (and a fortiori no negative) weight enters `bonded`/`epochSet` by the
   registration path.
3. **`objective()` requiring `MinBond > 0`** (`core/chain/chain.go:967`,
   `objective() = c.cfg.MinBond > 0 && c.verifyBond != nil`) — the weight-gated regime
   is only ARMED when `MinBond > 0`, which is what makes layer 2's floor a positive
   number rather than a no-op at 0.

Anyone who touches `objective()` or the `Size < MinBond` gate at chain.go:1521 MUST
re-open this encoder ruling: weakening either layer removes the guarantee that the
committed weights are non-negative, at which point the encoder's decision not to reject
negatives has to be re-certified (or a reject added at the encoder or a validity step).
This is recorded so the encoder's simplicity is not silently invalidated by a change
two files away.

**Zero note (PE Q4, `bondDomain`).** `0 = unset` is a legal, meaningful value. The state
root commits present-with-value-0 as the 8-byte encoding of 0; a key ABSENT from the map
has no leaf at all (proven by an exclusion proof). Present-zero and proven-absent are
therefore DISTINCT leaf states under the root — exactly the distinction PE Q4 requires
the eventual witness accessor to preserve. Step 1's encoding makes the distinction
EXPRESSIBLE (it does not conflate them); the accessor branch is the certified follow-on.

### Class C — scalars (one leaf at a reserved key, value = canonical encoding)

Not maps: a single value each. One leaf per scalar at a reserved key `tag ‖ ""` (empty
raw key), value = the scalar's canonical encoding.

| field | Go type | encoding |
|---|---|---|
| `everMature` | `bool` | 1 byte, `0x00`/`0x01` |
| `matureEpoch` | `bool` | 1 byte, `0x00`/`0x01` |
| `gateLockedIn` | `bool` | 1 byte, `0x00`/`0x01` |
| `gateHeight` | `uint64` | 8-byte big-endian |

**Reserved-key collision (cert Q3, PE Q2).** A scalar's key is `tag ‖ ""` — the field
tag followed by an EMPTY raw key. This cannot collide with any map keyspace: the map
fields under the SAME kind of tag have non-empty raw keys (`bonded` keys are 32-byte
NodeIDs, never empty; `bondRegHeight` likewise). The NUL-terminated tag is injective
(cert Q3): the first `\x00` terminates the field name, so `everMature\x00` + "" is
uniquely the everMature scalar and can equal no `bonded\x00` + 32-byte-key. Each scalar
has exactly one leaf. The four scalar tags are distinct field names, so they do not
collide with each other either.

**A scalar is ALWAYS committed (no omitempty at the leaf level).** `everMature = false`
commits the leaf `everMature\x00 → 0x00`, NOT an absent leaf. A false scalar and an
un-committed scalar must be distinguishable, so the leaf is always present. This mirrors
the choice-1 "required, not omitempty" decision at the field level, applied at the leaf
level. (Step 1 computes the root over the live struct, which always has these scalars, so
this is automatic; recorded so the property is explicit before the field lands.)

## The keyspace (cert Q3, keep the certified scheme)

One SMT over a field-tagged single keyspace. A leaf key is `tag ‖ rawKey` where
`tag = "fieldname\x00"` (the spike's scheme, `exclusion_test.go:18-25`). The NUL
terminator makes the concatenation injective across all 16 tags plus the scalar reserved
keys (cert Q3 CERTIFIED; the spike proves it for `byRoot`/`spent`, the cert extends the
argument to the full set). I keep the exact scheme — do not reimplement, do not vary the
separator.

Tags, one per field (the field name, NUL-terminated). `revLog` and `epochStart` get NO
tag: `revLog` is the separate LogRoot (`committedLog`), `epochStart` is under no root
(`observable`). The tag set is exactly the 16 `committedSet` field names.

## LogRoot (reuse, do not reimplement)

`LogRoot = RevocationLogRoot()` (`chain.go:2092`) — the existing RFC-6962 MTH over
`revLog`. Step 1 exposes it alongside the StateRoot; it does not recompute it. `revLog`
is history-DEPENDENT (its root varies with append order — proven by
`TestRevLogRootIsOrderDependent`), so it correctly stays an ordered CT root and NEVER
becomes an SMT leaf (the #597 category error).

## Empty-tree / empty-log roots (cert freeze condition 4)

The empty state commits a DEFINITE root, not an absent field. The empty-tree root is the
SMT's root over zero keys (a fixed constant the library derives from its placeholder);
the empty-log root is `translog`'s root over zero events. Step 1's determinism oracle
covers this: an empty committedSet computes the same StateRoot on every node. (The
"required, not omitempty" BLOCK-field choice is a later step; step 1 only needs the empty
root to be well-defined and reproducible, which it is.)

## What step 1 builds

1. `core/statehash` (promoted from `internal/smtspike`, a real package): a
   `StateRoot(*Chain)`-shaped computation over the 16 `committedSet` fields, field-tagged
   single keyspace, the per-field-class value encoding above. Reads the fields by the
   SAME reflection-free accessors the chain already has; no `Block` change.
2. The DETERMINISM ORACLE (cert R2): same logical committedSet ⇒ byte-identical leaves ⇒
   identical StateRoot, independent of construction/insertion order and of node. Ablation:
   perturb one Class-B value → root changes; perturb nothing, reorder insertion → root
   identical.
3. Extend the order-independence and snapshot-equivalence oracles to assert ROOT EQUALITY
   (the computed StateRoot), closing the gap between "16 fields equal" and "root equal."

## The one thing I will NOT do (push-back recorded)

I will not add a negative-weight REJECTION to the encoder, even though PE Q2 mentions
"reject or canonicalize negatives." That is a validity rule; smuggling it into the
step-1 encoder would (a) couple encoding to validity before the validity step is
certified, and (b) make the encoder non-total (a value it refuses to encode has no leaf,
re-opening an absent-vs-present ambiguity). The total two's-complement encoding is
simpler AND stronger: every representable int64 has exactly one leaf, so the root is a
faithful image of the summed value. If a later validity step wants to reject negative
weights, it rejects the BLOCK, not the ENCODING. Recorded so the reviewer sees the
deliberate divergence and its rationale.

## Reopening conditions (carried forward)

- `byRoot` graduates Class A → B if any validity predicate reads the Entry body.
- The value widths are consensus parameters: any width/endianness change is an era bump.
- The scalar reserved-key scheme holds only while map raw keys are non-empty; a future
  string-keyed committed map admitting `""` would collide with its own scalar key (cert
  Q3 / PE Q2 flagged this for `spent` — confirmed: token serials are non-empty).
