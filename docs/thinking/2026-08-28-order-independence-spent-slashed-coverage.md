# Order-independence + leave-one-out coverage for `spent` and `slashed`

Date: 2026-08-28
Author: Builder seat
Scope: test/fixture-only. No consensus rule is moved.

## Why

The PE ruling `RULING-keystone-spent-slashed-classification-2026-08-28.md` classified
`spent` (spent-serial set, `chain.go:828`) and `slashed` (slashed-validator set,
`chain.go:882`) as living UNDER the era-3 history-independent state root (`committedSet`).
Both must have COVERING order-independence probes before the era-3 format can freeze.

The ruling also found a live defect. The order-independence oracle
(`modelcheck_order_independence_test.go`) reports "16/16 committedSet fields identical
across opposite orderings" but its `twoOrderings` fixture only publishes and revokes roots.
`spent` and `slashed` are both empty in that fixture, so two of the sixteen comparisons are
`DeepEqual(∅, ∅)` — vacuous. The oracle exercises 14 fields while reporting 16. A green
that compares an empty map to an empty map is a comment that compiles, not coverage.

## The mechanism I am covering (attribution)

Both fields are grow-only sets, written unconditionally to `true`:

- `spent`: `apply` sets `c.spent[string(e.Token.Serial)] = true` for every entry that
  carries a publish token (`chain.go:2745`). It is read by `ValidateEntry`
  (`chain.go:2229`, `ErrTokenSpent`) to reject a replayed serial. `tokenQuorum > 0` gates
  the whole path; a token must be a quorum-blind-signed serial.
- `slashed`: `apply` sets `c.slashed[culprit] = true` for every committed equivocation
  proof (`chain.go:2819`). It is read by qualification/quorum predicates (`chain.go:1026`,
  `:1623`, etc.). A slash reaches `apply` only through `validateSlashes` →
  `VerifyEquivocation` (a self-verifying same-height double-sign).

Both are unions. Union is commutative, so the final membership is independent of the order
the serials were spent / the culprits were slashed. That is the property the SMT requires,
and the property the order-independence oracle must EXERCISE, not merely assert over ∅.

## Approach

### 1. A dedicated order-independence world that populates both fields

`twoOrderings` extends to commit, across two opposite orderings:

- Two publish-token entries with serials S1, S2 (drives `spent = {S1, S2}`), and
- Two committed equivocation proofs slashing culprits C1, C2 (drives `slashed = {C1, C2}`).

The chain requires tokens (`RequireTokens(quorum, issuerKey)`) with the anchor keys as
issuers — in the `roundsWorld` launch/objective world the anchors qualify as issuers via
`launchAnchor`. The two culprits are two extra keys that provably double-sign; slashing them
does not disturb the anchor quorum (they are not anchors), so the blocks still commit.

Ordering A commits (spend S1, slash C1) then (spend S2, slash C2). Ordering B commits them
in the opposite order. Both end at `spent = {S1, S2}`, `slashed = {C1, C2}`. The token
entries are also published roots, then revoked at heights 3–4, so `byRoot`/`revoked`/`revLog`
coverage is unchanged and the `committedLog` order-dependence assertion still holds.

**A finding surfaced while building this.** `byRoot` stores the full `Entry`, including its
`Token` (blind-signature bytes). An RSA blind signature is randomized, so minting the same
serial twice yields different `Sig` bytes. Minting a token per-ordering made `byRoot[root]`
DIFFER between the two chains — a FALSE order-dependence, a fixture artifact, not a chain
property. Fix: mint each serial's token ONCE, share the issuer keys across both orderings,
and commit the identical token bytes in both. `byRoot` is then genuinely order-independent.
This is why the two orderings share one `orderIssuers` (keys + mint), created once.

Then `TestCommittedSetFieldsAreOrderIndependent` compares the full committedSet across the
two orderings WITH `spent` and `slashed` non-empty.

### 2. A permanent non-emptiness guard (the durable fix)

Add an assertion to `TestCommittedSetFieldsAreOrderIndependent`: every committedSet field it
compares must be NON-EMPTY in at least one of the two orderings. If a field is empty in both,
the comparison is `DeepEqual(∅, ∅)` and asserts nothing — the test FAILS with a message
naming the vacuous field. This closes the hole for every future grow-only field: "all N
identical" can never again read as coverage while some fraction of it compares ∅ to ∅.

**The guard surfaced a wider truth immediately.** With the non-emptiness assertion added,
ELEVEN other committedSet fields were also empty-vs-empty in this world: the
bond-registration family (`bonded`, `bondRootOwner`, `bondRootProven`, `bondRegHeight`,
`regVersion`, `bondDomain`), the mature-epoch family (`epochSet`, `matureEpoch`,
`everMature`), and the #506 gate family (`gateLockedIn`, `gateHeight`). Their
order-independence was ALSO being asserted over ∅ — the vacuous-green was never limited to
`spent`/`slashed`. These belong to regimes this launch/objective/never-mature/gate-inactive
world does not enter (some are mutually exclusive with each other by design), so forcing them
all into one world is not possible. I declare them in `orderVacuous` — a shrinking debt that
mirrors `probeUncovered`, each entry naming what a covering fixture would need — and the guard
fails on any NEW empty-vs-empty committedSet field not on that list. `validatorsSeen` is NOT
declared: the attesting anchors qualify, so `apply` populates it, and its order-independence
is genuinely exercised here.

### 3. Leave-one-out coverage in the snapshot-equivalence oracle

Add a `spent` probe and a `slashed` probe to the snapshot-equivalence oracle
(`modelcheck_snapshot_equivalence_test.go`), in worlds where each field flips a verdict:

- `spent`: a token-required world where a serial is already spent. `ValidateEntry` on a
  fresh entry carrying that spent serial rejects (`ErrTokenSpent`). A snapshot that lost
  `spent` accepts the replay — the flip.
- `slashed`: a world where a culprit was slashed. A qualification verdict for that culprit
  (or a block it proposes) rejects. A snapshot that lost `slashed` re-admits it — the flip.

Remove both from `probeUncovered` ONLY once genuinely covered (the leave-one-out asserts
the flip).

### 4. The residual: `bonded` under two slash orderings

`apply` pairs `slashed[culprit]=true` with `delete(c.bonded, culprit)` (`chain.go:2819-2820`).
`slashed` is grow-only; `bonded` is mutated in the same step. The `twoOrderings` fixture
slashes NON-bonded culprits, so its `delete` is a no-op and does not exercise the interaction.
A dedicated test (`TestBondedOrderFreeUnderSlashInteraction`) bonds two validators, slashes
both in two OPPOSITE orders, and asserts `bonded` is byte-identical. Result: it is.
Mechanism: `bonded` is keyed by NodeID; deleting two distinct keys commutes, so C1-then-C2
leaves the same map as C2-then-C1. The test is the execution-grade evidence for that algebra.
No order-dependence found — `bonded` stays a clean SMT leaf under the slash interaction.

## Discipline: ablation-first

For each newly-covered field I inject an order-dependence defect (make the set mutation
order-sensitive), paste the FAILING output, then revert and show green. Same for the
non-empty guard: show it goes red when a committedSet field is left empty. A green probe with
no demonstrated red is not accepted.

## Trip-wire

No consensus rule is moved. I do not touch the weight sum (`chain.go:2450-2456`), the
`epochSet` freeze / `rotateEpoch`, the `⌈A/2⌉` quorum threshold, or any validity predicate.
This is fixture + assertion work only. If a probe could only pass by touching a rule, I stop
and report — that is research-gated.
