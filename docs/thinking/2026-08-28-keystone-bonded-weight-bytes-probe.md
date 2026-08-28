# bonded weight-bytes sibling probe (the #603 discharge for requireDeMatureSuperQuorum)

- Date: 2026-08-28
- Seat: Builder
- Driver: blind PE ruling `RULING-603-weight-bytes-discharge-2026-08-28.md`
  (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-603-weight-bytes-discharge-2026-08-28.md`)
- Issue class: model-check/unit tier test coverage. Touches NO consensus rule — it
  READS the weight predicate and asserts its bytes are load-bearing.

## The gap the PE named

`TestEpochWeightBytesAreLoadBearing` (`modelcheck_snapshot_equivalence_test.go:938`,
PR #606) proves the committed per-member WEIGHT BYTES of `epochSet` are load-bearing
through `requireEpochWeightQuorum`: membership held byte-identical, weights flattened
to a constant, verdict flips via `ErrNoQuorumWeight`.

`bonded` has no analogous probe. `requireDeMatureSuperQuorum` (`chain.go:2591`) sums
LIVE `bonded` weight the same way the epoch branch sums frozen `epochSet` weight:

```
total    = Σ c.bonded[id]
committed = c.bonded[proposer] + Σ_{id∈seen} c.bonded[id]
need     = ⌈2·total/3⌉
reject (ErrDeMatureQuorum) iff committed < need
```

`bonded`'s existing probe (`everMature`/`deMatureWorld`, `modelcheck_snapshot_equivalence_test.go:520`)
is a PATH-ENTRY flip — dropping the `everMature` bool skips the whole bar. Its own
comment concedes it "changes whether the de-mature bar APPLIES, never how weight is
summed." That is orthogonal to #603. `bonded` is committed under the era-3 root, so
the sibling weight-bytes proof is owed BEFORE the format freezes.

## The mechanism I built

`TestBondedWeightBytesAreLoadBearing` + `bondedWeightBytesWorld`, a clone of the
epoch weight-bytes probe targeting `requireDeMatureSuperQuorum`:

- Hold `bonded` MEMBERSHIP (the id set) byte-identical.
- FLATTEN every per-member `bonded` weight to a constant so support/total collapses
  to |coalition|/|members| and the ⅔-of-live-bonded-weight test FAILS
  (`ErrDeMatureQuorum`), while the true unequal-weight replica PASSES.

The flip goes through the WEIGHT predicate specifically, not membership and not a
path-entry bool.

### The regime, and why it differs from the epoch probe

The de-mature bar fires only when `everMature && objective() && !matureNow()`
(`chain.go:2471`). So the world:

- Ramps 4 equal genesis bonds (MatureValidators=2) so `everMature` LATCHES.
- Resets live `bonded` to an UNEQUAL, whale-dominated split so `MatureCoefficient()`
  drops below MatureValidators → `!matureNow()` (same shape as `deMatureWorld`).
- Epochs DISABLED and NO anchors, so `requireEpochWeightQuorum` never fires
  (needs `epochsEnabled`) and `launchAnchor` is false — the de-mature super-quorum is
  the ONLY regime gate, and qualification reads `bonded[id] >= MinBond` directly
  (`attesterQualifiedAt`, `chain.go:1059`).

**The one seam that differs from `weightBytesWorld`: the count floor.** In a mature
EPOCH, `RequiredQuorum()` returns just `Quorum` (`chain.go:1218` — the weight rule
carries the Byzantine bar), so a Quorum=1 coalition can be a weight-majority but a
HEAD-minority, which is what lets the flatten flip. In the de-mature NON-epoch
objective regime with `ByzantineQuorum: true`, `RequiredQuorum()` escalates to
`bftThreshold(qualifiedCount)` (`chain.go:1220`), which forces the coalition to be
~⅔ of HEADS — and a head-⅔ coalition also clears a FLATTENED weight-⅔, so the flatten
would NOT flip. The count floor would mask the weight rule.

The fix, and the analogue to the epoch probe's mature-epoch bypass: set
`ByzantineQuorum: false` in this world so `RequiredQuorum()` returns `Quorum=1`. The
de-mature branch (`chain.go:2471`) fires regardless of `ByzantineQuorum` — it depends
only on `everMature && objective() && !matureNow()` — so the weight rule under test is
untouched. This lowers the count floor exactly as the mature-epoch path does, so a
small heavy coalition (proposer + 1 attester) is a weight-majority but a head-minority,
and the flatten flips through `ErrDeMatureQuorum`.

### The arithmetic

4 members. True live `bonded`: proposer keys[0]=5 MiB, attester keys[1]=5 MiB, silent
keys[2]=1 MiB, keys[3]=1 MiB. total=12 MiB.

- **Full (true weights):** coalition = keys[0]+keys[1] = 10 MiB. need = ⌈2·12/3⌉ = 8 MiB.
  10 ≥ 8 → ACCEPT.
- **`!matureNow`:** NakamotoOperators = fewest bonds exceeding ⌊12/3⌋=4 MiB → one 5 MiB
  bond clears it → coefficient 1 < MatureValidators 2 → not mature. The de-mature bar
  applies.
- **Flatten to constant k=MinBond:** total=4k, coalition=2k, need=⌈8k/3⌉. For k=1 MiB:
  need=2796203, coalition=2097152 < need → REJECT via ErrDeMatureQuorum. Membership
  byte-identical; only the weight bytes changed.
- **Count floor:** Quorum=1, ByzantineQuorum=false → RequiredQuorum=1. seen={keys[1]}
  clears it on both the full and flattened replicas, so the discriminator is the weight
  rule, not the count floor.

## Ablation — proven RED both directions (the standing rule: inject the defect, watch red)

- **Direction A (no perturbation):** replace the flatten with the TRUE `bonded` (no
  weight change). The ablated replica must now ACCEPT → the `errors.Is(ErrDeMatureQuorum)`
  assertion goes RED (`want ErrDeMatureQuorum ... got <nil>`). Proves the flatten is what
  causes the flip.
- **Direction B (empty membership, a #604-style drop):** substitute an EMPTY `bonded`
  map. This does NOT abort at `collectQuorumSigs` — that call returns `seen={}, err=nil`
  on empty membership, because unqualified attesters (`bonded[id]=0 < MinBond`) are
  silently SKIPPED, not rejected (`chain.go` ~2411, the `continue`). The probe's guard
  fires LATER, at the `errors.Is(stackErr, ErrDeMatureQuorum)` assertion: with `seen`
  empty, `requireQuorumStack` hits the count floor first and returns `ErrNoQuorum`
  ("0 qualified, need 1"), and `ErrNoQuorum != ErrDeMatureQuorum`, so the assertion
  correctly refuses it. (The de-mature branch itself would never fire here anyway — with
  `bonded` empty, `requireDeMatureSuperQuorum` short-circuits at `total <= 0 → nil`,
  `chain.go:2596`.) Proves the probe refuses a membership-omission dressed up as a
  weight-bytes proof.

## STOP boundaries honored

The probe READS the weight predicate. It does NOT touch:
- the weight-sum seam (`chain.go:2591-2609`, `requireDeMatureSuperQuorum`'s summation),
- the `epochSet` freeze / `rotateEpoch` (I3),
- the `⌈2·total/3⌉` de-mature threshold.

Setting `ByzantineQuorum: false` is a per-world CONFIG choice on a throwaway test chain,
not a change to any rule — it selects the count-floor regime so the weight rule is the
isolated discriminator, exactly as the mature-epoch path does for the epoch probe.

## Integrity coupling — do not sever `bondDomain` from `committedSet`

The probe silently depends on `bondDomain` staying classified `committedSet`
(`modelcheck_state_completeness_test.go:91`). The `!matureNow()` half of the regime is
built by declaring a whale-dominated `bondDomain` (`bondedWeightBytesWorld`, line 1084),
and `bondDomain` feeds `MatureCoefficient()` → `matureNow()` (`chain.go:1846`, via
`C2Metric`'s A-axis). If a future refactor demotes `bondDomain` out of `committedSet`,
`snapshotBoot` stops carrying it (`snapshotCarried()` iterates only `committedSet` +
`committedLog` + `observable`), the ablated replica boots with an initialised-but-EMPTY
`bondDomain` map, and its `matureNow()` can flip back to true — which drops the whole
de-mature bar. The probe would then go GREEN for the WRONG reason: a path-entry gate,
not the weight bytes it exists to prove load-bearing. A future editor must not sever
that classification without re-anchoring this probe's `!matureNow` setup.

## Invariants touched (per consensus-invariants.md working rule 1)

- **I1 / I3 (weight, not head-count):** the probe ASSERTS the weight rule discriminates.
  It preserves both by not altering the summation or threshold — it only proves the
  committed bytes the rule reads are load-bearing.
