# Floor-box recompute increment 2 — the maturity-latch predicate (matureNow / C2Metric)

Date: 2026-08-31
Author: Builder seat
Status: decided, building
Predecessor: increment 1 (`floorbox_recompute_v5.go`, PR #667) reproduced
`requireEpochWeightQuorum` trustlessly (the C-1 weight-composition pattern).

## The target

Reproduce the MATURITY-LATCH metric `matureNow` (`chain.go:2178`) trustlessly from the
committed StateRoot + witnesses alone, replicating increment 1's structure. `matureNow`
folds `C2Metric` (`chain.go:2300-2382`) and gates the de-mature super-quorum
(`everMature && objective() && !matureNow()`, `chain.go:2827`).

`C2Metric` is a WHOLE-SET fold over `validatorsSeen` (a set-membership keyspace). For each
member id it reads:

| Read | Source | Class |
| --- | --- | --- |
| membership of `validatorsSeen` | committed keyspace (digest root) | set-completeness |
| `c.cfg.Anchors[id]` (skip anchors) | OWN genesis config | C-6 |
| `c.slashed[id]` (skip slashed) | committed keyspace | C-1 (per-member) |
| `c.bonded[id]` (the weight) | committed keyspace | C-1 (per-member) |
| `c.bondDomain[id]` (the A-axis domain) | committed keyspace | C-1 (per-member) |
| `c.cfg.MinBond` (eligibility screen `sz >= MinBond`) | OWN genesis config | C-6 |
| `c.operatorMargin()` = `c.cfg.OperatorMargin` | OWN genesis config | C-6 |
| `c.cfg.MatureValidators` (the threshold) | OWN genesis config | C-6 |

The metric then computes `min(NakamotoOperators, NakamotoDomains)` and `matureNow`
compares it to `MatureValidators`. The non-objective branch (`!objective()`) counts
`validatorsSeen \ Anchors >= MatureValidators` — same set-completeness + own-config shape,
simpler fold; the recompute reproduces the OBJECTIVE branch (the one an untrusted
deployment runs; `objective()` is `MinBond>0 && verifyBond!=nil`, true for any real box).

## Why this predicate is next (the PE's sequencing)

Increment 1 exercised C-1 (per-member value proofs) but its C-6 obligation was VACUOUS —
`requireEpochWeightQuorum` reads only the fixed ⅔ consensus constant, no genesis knob. So
increment 1 could not ship C-6 TEETH: there was no config value an attacker could try to
shift via the witness. `matureNow` is the first predicate that READS genesis config into
the fold (`MinBond` screens members, `OperatorMargin` divides the coefficient,
`MatureValidators` is the threshold). It is the correct next target precisely because it
DISCHARGES the open C-6-teeth gap: a config-from-witness injection now has a live verdict
to flip.

## Options considered

### Option A — reproduce the whole `C2Metric` (all six sub-metrics: Nakamoto bonds/operators/domains, HHI, Gini, TopShare, WeightUniformity)

Rejected. `matureNow` consumes ONLY `min(NakamotoOperators, NakamotoDomains)` via
`MatureCoefficient()`. HHI/Gini/TopShare/WeightUniformity are observability-only
(never enforcement, per the D-C2 comments). Reproducing them is gold-plating a predicate
the latch does not gate on. The recompute reproduces exactly what the latch reads:
`MatureCoefficient() >= MatureValidators`.

### Option B — reproduce `MatureCoefficient()` / `matureNow` objective branch (CHOSEN)

Reproduce the exact quantity the shed gates on. Structure mirrors increment 1:

1. SET-COMPLETENESS: reconstruct `nodeSetMTH(witnessedIDs)`; require it equals the
   committed `validatorsSeenRoot` (proven present against the StateRoot). One omitted or
   injected member ⇒ different MTH ⇒ stall. (The F1 `validatorsSeenRoot` digest, inert
   until now, becomes a genuine read — mirrors increment 1's promotion of `epochSetRoot`.)
2. PER-MEMBER VALUES (C-1): for every member, Resolve `bonded[id]` (present with weight) and
   `bondDomain[id]` (present with domain, or ABSENT for domain 0). `slashed[id]` is resolved
   as a membership proof (present ⇒ skip). A forged weight/domain/slashed-bit fails
   `smt.VerifyProof` ⇒ stall.
3. GENESIS CONFIG (C-6): `MinBond`, `Anchors`, `OperatorMargin`, `MatureValidators` are read
   from the box's OWN `c.cfg`, NEVER the witness. This is the FIRST predicate where C-6 has
   teeth: injecting config-from-witness shifts the eligibility screen / margin / threshold
   and flips the verdict.
4. THE FOLD + THRESHOLD, byte-for-byte `C2Metric` + `MatureCoefficient`: build `sizes` /
   `domainWeight` / `zeroDomainWeights` over the verified members (minus own-Anchors, minus
   slashed), compute NakamotoOperators and NakamotoDomains, take the min, compare to own
   `MatureValidators`.

### The slashed keyspace read — decision

`C2Metric` skips a member if `c.slashed[id]`. A withholding prover cannot simply omit the
slashed proof: every member of the completeness-verified `validatorsSeen` set is folded, and
the recompute Resolves `slashed[id]` for each. Domain-absent (`bondDomain[id]` absent) is a
first-class case (domain 0 → independent group), so the recompute uses `IsProvenAbsent`
there, exactly as the producer emits `addAbsent(tagBondDomain, ...)`. A `bonded[id]` absent
or below `MinBond` is not a member of `C2Metric`'s tally — the member is skipped, matching
the full node's `if sz := c.bonded[id]; sz >= c.cfg.MinBond`.

## STOP boundary

This increment reproduces ONE predicate. It does NOT flip `WitnessValidateV5` to Accept
(the final increment, only after ALL predicates are reproduced). The box still never-Accepts.
No consensus/validity RULE changes — `chain.go` is untouched; this is a SEPARATE root-only
path a semi-stateless box calls INSTEAD of holding the tree.

## The mandatory C-6 ablation (the reason this predicate is next)

Ship a FAILING-FIRST ablation: an injected variant that reads `MinBond` / `OperatorMargin`
/ `MatureValidators` from the WITNESS instead of own config, run against two boxes with
DIFFERENT own config, and watch them DIVERGE (RED). Restore own-config and watch them agree
(GREEN). Report the red→green. This discharges the open C-6-teeth gap from increment 1.

## Hard ablations (C-5, red-before-green)

- Forged per-member `bonded` weight ⇒ REJECT (C-1).
- Forged per-member `bondDomain` ⇒ REJECT (C-1).
- Omitted / injected `validatorsSeen` member ⇒ REJECT (MTH mismatch).
- Config-from-witness (MinBond / OperatorMargin / MatureValidators) ⇒ verdict DIVERGES
  under an injected config-sensitive fold, INVARIANT under the correct own-config fold (C-6).
