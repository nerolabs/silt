# Lane-1 Part A — the execution-derived v5 witness read-set producer: options (REWRITTEN for the AMENDED cert)

Date: 2026-08-30
Author: Builder
Certification governing this build (AMENDED / SUPERSEDING):
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witness-floor-box-readset-v5-AMENDED-RESEARCH-CERTIFICATION-2026-08-30.md`
PE ruling on the prior build (the fixes this rewrite lands):
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-lane1-partA-readset-v5-producer-2026-08-30.md`

## Why this is a REWRITE, not a first draft

The prior build (PR #656 / `dbeccf1`) was SHIP-WITH-FIXES. Two defects, both now
authoritative in the amended cert:

1. **The read-set was INCOMPLETE.** It omitted the attestation-loop reads (per attester:
   `slashed[id]`, the qualification-set membership, the `validatorsSeen[id]` write-target),
   the maturity-latch reads (`everMature` + its `Mature()` inputs `bonded`/`bondDomain`/
   `C2Metric`), and the committed scalar leaves (`epochStart`/`era4LockedIn`/`era4Height`/
   `matureEpoch`/`gateLockedIn`/`gateHeight`/`era3LockedIn`/`era3Height`). `validatorsSeen`
   and `everMature` are consensus-load-bearing (maturity → anchor gating, F-1). A floor box
   handed the incomplete read-set can be made to accept a forged block on any omitted leaf.

2. **The drift guard was BLIND to the gap.** It was a SECOND hand-written enumeration
   (`recomputeWitnessReadsV5`) checked against the producer. Both inherited the cert's blind
   spot, so set-equality was GREEN over a real accept-a-forgery hole. This is the session-7
   "green while covering nothing" scar, one dimension over: a check that passes because both
   sides share the same omission.

The amended cert's per-leaf table (23 committed keyspaces, its §"The CORRECTED, COMPLETE
read-set identity") is now the authoritative checklist. The boundary bound is relabelled
O(RegCap) (it reads the WHOLE frozen set for the three activation tallies), box-fits at 256
unchanged.

## The load-bearing decision — HOW to build the execution-derived completeness guard

The producer enumeration stays payload-driven (bounded by construction — Option B from the
prior doc, unchanged and still correct; instrumenting `apply()`'s literal reads yields the
O(registry) set and defeats era-4, so that stays rejected). What CHANGES is the guard: it
must be rooted in GROUND TRUTH — the recorded leaf-touch of the real v5 recompute — NOT a
second hand-written table. The amended cert makes this MANDATORY (R3), with the decisive
proof: a hand-written mirror shares the producer's blind spots and certifies nothing.

The real v5 recompute is fixed and already in the tree:
`postApplyRoots(b)` → `cloneForDryRun()` → `apply(b)` on the clone → `StateRootForVersion(5)`
→ `stateRootLeavesV5()` (`era3validity.go:145`, `statehash.go:182`). The guard must derive
the "expected read-set" from THAT computation. Two committed-leaf read categories exist:

- **Category 1 — write-target reads.** Every leaf the recompute WRITES it first reads: map
  write-targets need the pre-state to compute the post-value; monotonic scalars gate on
  pre-state (`if !c.everMature`, `if !c.era4LockedIn`, …). These are exactly the leaves the
  prior build dropped (`validatorsSeen`, `everMature`, the scalars are all write-targets).
- **Category 2 — pure gate reads.** Leaves read but NOT written in this block: a `slashed[id]`
  gate where `id` is not slashed, a boundary `regVersion` read for an unchanged frozen member,
  the `bonded`/`bondDomain` maturity inputs. These do not appear in a write-diff.

### Option G-1 — recording accessor over the clone's committed maps

The cert's first-named option. Wrap every committed map on the clone so each `c.bonded[id]`,
`c.slashed[id]`, … read is recorded during the clone's `apply()`.

- Cost: the committed maps are plain Go maps accessed DIRECTLY at hundreds of consensus sites
  (`c.bonded[id]`, `c.slashed[id]`, `c.qualified[id]`, …). Go maps cannot intercept reads. To
  record reads I would have to convert every access site to a method call — that is a
  CONSENSUS-LOGIC change to `apply()`/`attesterQualified`/`matureNow`/`rotateEpoch`/`C2Metric`,
  the exact gated change the scope forbids ("If a production hook is unavoidable, isolate it
  test-only and flag it" — but this is not isolable; it rewrites the hot path).
- Verdict: REJECTED. Not test-only-isolable; it changes consensus logic.

### Option G-2 — pre/post `stateRootLeavesV5` write-diff ALONE

The cert's second-named option, taken literally. Compute `stateRootLeavesV5()` pre-apply and
post-apply on the clone; the leaves whose value CHANGED are the recompute's writes; require
the producer to cover them.

- Benefit: pure ground truth, zero consensus-logic change, catches EVERY Category-1 omission
  — including exactly the prior build's dropped `validatorsSeen`/`everMature`/scalars.
- Cost: a write-diff is only the WRITE-set. It does NOT capture Category-2 pure gate reads
  (`slashed[id]` gate on a non-slashed id, boundary `regVersion` for unchanged members). A
  producer that dropped a pure gate read would still pass. INCOMPLETE as a read-set guard.
- Verdict: NECESSARY but INSUFFICIENT alone. Keep it as the primary source; pair with G-3.

### Option G-3 — leaf-sensitivity perturbation over the real recompute (the Category-2 source)

For each committed leaf, PERTURB its pre-state value on a fresh clone, re-run the real
recompute (`postApplyRoots`), and check whether the output StateRoot CHANGES. A leaf whose
pre-state value affects the recompute's output root is, by definition, a leaf the recompute
READS — the box MUST witness it, else it cannot detect a forged value there. A leaf whose
perturbation leaves the root unchanged is genuinely not read for THIS block (a dead branch),
and the producer correctly need not emit it.

- Benefit: pure ground truth over the REAL recompute (no hand-written mirror, no
  consensus-logic change). It captures BOTH categories: a write-target leaf changes the root
  when perturbed (its post-value depends on its pre-value), AND a pure gate leaf changes the
  root when perturbed (the gate flips a branch that changes some OTHER committed write). It is
  the complement of the write-diff and subsumes it, but the write-diff is cheaper and sharper
  for the common case, so I use both.
- Cost: perturbation is per-leaf, so the probe set must be enumerated. I perturb (a) every
  leaf the pre-apply state already carries, and (b) a representative injected value for each
  of the 23 keyspaces at the payload-named keys (so a leaf ABSENT pre-apply but read as a
  write-target — e.g. `validatorsSeen[attester]` false→true — is probed by injecting it and
  seeing the root move). The probe is O(pre-state + payload), test-only, bounded by the small
  corpus fixtures.
- A subtlety: perturbation detects reads that AFFECT THE OUTPUT. A leaf read into a branch
  that happens not to change any committed write for this block is not witness-necessary for
  this block — which is the CORRECT read-set (a floor box need not witness a leaf whose value
  cannot change the post-state root). So perturbation-sensitivity is precisely the soundness
  criterion: "witness every leaf whose pre-state can change the recompute's committed root."

### DECISION — G-2 (write-diff) ∪ G-3 (perturbation), both over the real `postApplyRoots`

The execution-derived guard's "expected read-set" = the UNION of:
- the pre/post `stateRootLeavesV5` write-diff (Category-1 write-targets), and
- the leaf-sensitivity perturbation set (Category-1 ∪ Category-2, every leaf whose pre-state
  perturbation moves the real recompute's committed root).

Assert the PRODUCER's read-set COVERS (⊇) this ground-truth set. Both sources are computed
from the real `postApplyRoots`/`stateRootLeavesV5`/`apply()` recompute on a clone; NEITHER is
a hand-written table. This is the mechanism the amended cert reaffirms as MANDATORY.

Why COVERS (⊇) not EQUALS: the producer may soundly OVER-witness (emit a leaf the recompute
does not strictly need); over-witnessing costs a little witness bandwidth but never a
wrong-accept. UNDER-witnessing is the soundness hole. So the guard's binding direction is:
the producer must be a SUPERSET of the ground-truth read-set. A separate bound test keeps the
producer from over-emitting to O(registry) (the boundedness ablation, below).

This mechanism is TEST-ONLY. It uses `cloneForDryRun` + `postApplyRoots` + `stateRootLeavesV5`
(all existing) and a test-only leaf-setter that mutates a clone's committed maps between
clone and recompute. NO production hook is added; NO consensus rule or validity predicate is
touched.

## The producer changes (per the amended cert's 23-keyspace table)

Add the omitted reads, keeping the payload-driven / O(payload) shape:

- **The attestation loop** (`apply:3293-3298`), per attester `a` in `b.Atts` with
  `a.AttesterID() != b.ProposerID()`: the `attesterQualified(id)` input reads —
  `slashed[id]`; under objective+matureEpoch the `epochSet`/`effectiveEpochSet` membership;
  else `bonded[id]` — plus the `validatorsSeen[id]` write-target. One read-group per attester,
  bounded by the quorum size (RegCap-bounded). Still O(payload).
- **The maturity latch** (`apply:3303-3305`): `everMature` scalar pre-state, plus the
  `Mature()`→`matureNow()` inputs — legacy mode reads the `validatorsSeen` set, objective mode
  reads `MatureCoefficient`→`C2Metric` over `bonded` + `bondDomain`, and `matureEpoch` selects
  the branch.
- **The committed scalar leaves**: `epochStart`, `era4LockedIn`, `era4Height`, `matureEpoch`,
  `gateLockedIn`, `gateHeight`, `era3LockedIn`, `era3Height`. Each is committed and gated on
  its own pre-state in `apply`/`rotateEpoch`, so the recompute reads it. Emitted as
  scalar-key reads (the reserved-key leaves at `statehash.go:155-160,206,211-212`).
- **`bondDomain`** as a maturity READ input (objective C2 domain-distinct), beyond its
  existing bond-reg write.
- **The boundary read-set relabel**: O(RegCap), not O(boundary-delta). The three activation
  tallies read `regVersion` and weight over EVERY frozen-set member (`rotateEpoch:3442/3465/
  3489`); the producer already ranges the whole `qualified`/`epochSet`, which is correct — the
  comment/label is corrected to O(RegCap).

## The corpus additions (amended cert R3 + PE Q4)

The prior corpus never populated `b.Atts` (`Sign` sets only Proposer, `chain.go:724-729`), so
the attestation write path and maturity latch were DARK. Add:

1. an **attested block** carrying real attestations from a qualified non-proposer attester
   (constructed explicitly, not via `Sign`) so the atts-loop reads fire and `validatorsSeen`
   is written;
2. a **maturity-latch transition** (`everMature` false→true) so the latch read is a witnessed
   change, not a constant;
3. a **standalone slash block at a NON-boundary height** so the slash-path reads are covered
   without the boundary path masking them.

Add `attested`, `maturity-latch`, and `slash` to the required corpus classes in the vacuity
guard so none can silently skip (the session-7 scar: a class the corpus never exercises makes
the guard vacuous for it).

## Regression proof (the whole point — inject the escaped defects, watch RED)

The new execution-derived guard MUST redden on the exact defects that escaped the prior build:

1. **Re-inject the `validatorsSeen` omission** (drop the attestation-loop reads from the
   producer). The write-diff sees `validatorsSeen[attester]` change false→true on the attested
   block; the producer no longer emits it → guard RED. (It was GREEN before, because the mirror
   also omitted it.) Restore → GREEN.
2. **Re-inject the slash-path `qualified[culprit]` drop.** The write-diff / perturbation sees
   `qualified[culprit]` change on the slash block; producer drops it → RED. Restore → GREEN.
3. **Keep the certified boundedness ablation.** Inject an O(registry) `bondRegHeight` scan into
   the producer → the read-set scales with the registry → the bounded-not-registry-sized test
   REDs. (Over-emitting to O(registry) is caught here, not by the coverage guard.)

## Scope guard (obeyed)

Producer + guard/test/corpus only. No consensus rule, no validity predicate, no production
hook. The guard's leaf-perturbation setter is a test-only helper over a `cloneForDryRun`
clone. The #535 recovery-boundary observable (R2) and the Part-B recompute soundness are NOT
closed here and are NOT this producer's job.

## Conditional bounds (inherited, not re-opened)

O(payload) (ordinary/TTL) and O(RegCap) (boundary) are CONDITIONAL on RegCap bounding the
per-block reg count, the per-height bucket, AND now the boundary frozen set and the attester
quorum (amended cert R1). This producer does not pin RegCap; it inherits the standing gate
(`era4-regcap-VALUE-DERIVATION-VERDICT-2026-08-29.md`).
