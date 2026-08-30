# Lane-1 Part A — the execution-derived v5 witness read-set producer: options

Date: 2026-08-30
Author: Builder
Certification governing this build:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witness-floor-box-readset-v5-RESEARCH-CERTIFICATION-2026-08-30.md`

## What this increment ships

A producer that, for a v5 block, emits the WITNESS read-set as a `[]statehash.ReadEntry`
(the type shipped in #633): the set of committed-state keys the v5 WITNESSABLE RECOMPUTE
reads. Plus the R3 drift guard that re-reddens if the producer ever diverges from the keys
the recompute actually reads.

The certified read-set identity (do NOT re-derive it):

> validity reads ∪ `apply()` branch reads (`slashed`/`bondRootOwner`/`bondRootProven`) ∪
> era-4 accelerator reads (the single `dueBucket[h]` NON-MEMBERSHIP leaf + the touched
> `qualified`/`epochSet` delta).

For three block classes: ordinary (O(payload)), TTL-firing (O(payload), includes the
empty-`dueBucket[h]` non-membership case), epoch-boundary (O(boundary-delta), heavier).

## The single sharpest hazard (certified §"Sub-question 2", §"What would lift…")

The read-set is the keyset of the **witnessable recompute**, NOT of `apply()`. `apply()`
still scans the whole `bondRegHeight` map every block (the TTL sweep, `chain.go:3272`), so
instrumenting `apply()`'s literal reads yields the O(registry) set and DEFEATS era-4. The
producer must target the bounded witnessable recompute:

- the TTL "nothing else expired" claim collapses to ONE `dueBucket[h]` non-membership leaf
  (empty bucket) or the bucket's committed member list (non-empty) — it does NOT read every
  id in `bondRegHeight`;
- the boundary changed-leaf set is O(boundary-delta), a distinct heavier class.

## The design question: HOW to derive the read-set

The producer must (a) target the bounded witnessable recompute, and (b) be provably in sync
with what that recompute actually reads (R3). Two sub-decisions: (1) how to ENUMERATE the
read-set, (2) how to build the R3 guard that keeps the enumeration honest.

### Sub-decision 1 — how to enumerate the read-set

**Option A — instrument `apply()` and record every committed-map access.** Wrap the live
committed maps in a recording accessor, run `apply()`, collect the touched keys.

- Cost: DIRECTLY DEFEATED by the certified hazard. `apply()`'s TTL sweep ranges the whole
  `bondRegHeight` map (`chain.go:3272`) and the boundary tallies range the whole frozen set.
  The recorded set is O(registry). This is the exact "build that instruments apply() derives
  the O(registry) set and defeats era-4" the cert names as the single sharpest hazard.
- Verdict: REJECTED. It rebuilds the wall era-4 removes.

**Option B — a payload-driven enumerator that walks the block's transitions.** For a v5
block, walk its payload (`Entries`, `Revocations`, `Unrevocations`, `BondRegs`, `Slashes`)
and emit the committed key each transition reads, PLUS the bounded era-4 accelerator keys
(`dueBucket[h]` for a TTL-firing height, and the touched `qualified`/`epochSet` delta). Each
transition contributes O(1) keys; the accelerator contributes one `dueBucket[h]` leaf +
the bucket members (RegCap-bounded) + the boundary delta. No whole-map scan.

- Benefit: bounded by construction. Ordinary/TTL blocks are O(payload); the boundary is
  O(boundary-delta). This is the recompute's OWN read-set — the certified identity, member
  for member. The producer's keys are derivable purely from the block + the committed state
  it must witness against, which is exactly what a floor box holds.
- Cost: it is a HAND-DIRECTED enumeration of the recompute's reads. If a future refactor of
  the recompute changes what it reads, the enumerator silently desyncs. This is precisely
  the R2/R3 residual the cert flags: "the derivation MUST be execution-derived, not
  hand-written … it must be the recorded touch-set of the actual v5 witnessable recompute
  over a branch-covering corpus, ablated."
- Verdict: CHOSEN for the enumerator, PAIRED WITH the R3 guard below so it is not merely
  hand-written — it is hand-written AND cross-checked against an execution recording.

**Option C — rewrite `apply()`'s TTL/boundary branches to read via the accelerator, then
instrument THAT.** Make `apply()` itself bounded (read `dueBucket[h]` instead of scanning
`bondRegHeight`), then record its reads.

- Cost: this is a CONSENSUS-RULE change to the one authoritative state-transition function.
  The cert is explicit that the full-node recompute stays O(registry) BY DESIGN
  (`chain.go:378`, RECERT2 Q1); the O(payload) property belongs to the WITNESS recompute, a
  DIFFERENT computation. Changing `apply()` is a gated change beyond this increment's scope
  ("If you find yourself changing a consensus rule or validity predicate, STOP").
- Verdict: REJECTED. Out of scope and gated.

**Decision: Option B** — a payload-driven enumerator, bounded by construction, targeting the
witnessable recompute's read-set. The soundness of the enumeration (that it reads EXACTLY
what the recompute reads) is defended by the R3 guard, not by inspection.

### Sub-decision 2 — how to build the R3 drift guard

The cert (R3, §"The decisive completeness artifact") is explicit: the guard must
EXECUTION-DERIVE the recompute's actual reads and redden if the producer's enumeration
diverges. This is the load-bearing defense (treat it like the #654 vacuity guard).

**Option R3-A — record the reads of the ACTUAL witnessable recompute and compare.** Build a
recording accessor over the committed state the witnessable recompute reads, run the
recompute (the bounded verification: read `dueBucket[h]`, the touched deltas, the branch
reads — NOT the `apply()` scan), collect the recorded key-set, assert it EQUALS the
producer's enumerated key-set. Then ablate: drop a `dueBucket` key from the producer and
watch RED; drop a `qualified`/`epochSet` delta key and watch RED; restore GREEN.

- Cost: requires a second, execution-recording implementation of the recompute's reads. But
  the cert demands exactly this — "the recorded touch-set of the actual v5 witnessable
  recompute."
- Verdict: CHOSEN. This is the R3 obligation stated verbatim.

**Option R3-B — assert the producer's set is a subset of `apply()`'s touch-set.** Cheaper,
but `apply()`'s touch-set is O(registry) and includes keys the witnessable recompute does
NOT read. A subset check passes a producer that OMITS an accelerator key (the empty-bucket
non-membership leaf is not in `apply()`'s touch-set at all — `apply()` reads
`bondRegHeight`, not `dueBucket[h]`, on the sweep). It would certify nothing.

- Verdict: REJECTED. It checks against the wrong set (the O(registry) apply reads), the
  exact conflation the cert forbids.

**Decision: Option R3-A** — the guard records the WITNESSABLE recompute's reads over a
branch-covering corpus and asserts set-equality with the producer, ablated red on a dropped
`dueBucket` key AND on a dropped `qualified`/`epochSet` delta key.

Since the witnessable recompute is itself the thing being built (there is no separate v5
verifier binary yet — that is Part B), the R3 guard's "recompute reads" side is derived by a
recording enumerator that mirrors the recompute's DEFINED read-set (the certified identity),
built INDEPENDENTLY of the producer's own construction (a distinct code path over the block
+ committed state). Divergence between the two independent enumerations reddens. This is the
same dual-source discipline the #654 vacuity guard and the RECERT2 maintenance drift guards
use: two independent computations of the same set, asserted equal, ablated red.

## The corpus (certified R3, §"The decisive completeness artifact")

MUST include, or the guard certifies nothing:

1. an ordinary block (publish + bond-reg, no TTL firing, non-boundary) — O(payload);
2. a TTL-firing block with a NON-EMPTY `dueBucket[h]` (the member-list path);
3. a TTL-firing block with an EMPTY `dueBucket[h]` — the single-non-membership-proof path,
   THE WHOLE ERA-4 WIN. This is the height where nothing is due; the read-set carries the
   one `dueBucket[h]` QueryAbsent leaf and nothing from a `bondRegHeight` scan;
4. a renew that moves a bucket (`D_old`→`D_new`);
5. a slash-before-due block;
6. an epoch-boundary block (incl. the young→mature handoff) — O(boundary-delta).

The guard ablates red on a dropped `dueBucket` key and a dropped `qualified`/`epochSet`
delta key (the session-7 "green while covering nothing" scar: the empty-bucket path must be
covered or the guard is vacuous).

## Scope guard (obeyed)

This is the read-set PRODUCER only. NOT the #535 boundary policy (Part B), NOT wiring
`IngestBlockWitnesses` into acceptance (Part B). No consensus rule or validity predicate is
changed. The R2 #535 recovery-boundary observable (`cfg.LivenessRecoveryHeight` branch
selection) is NOT closed here and is NOT this producer's job — the producer emits the read-set
for the committed state; the recovery-branch SELECTION is a Part-B/operator-directive residual
the cert scopes out (§"Sub-question 3").

## Conditional bounds (inherited, not re-opened)

The O(payload)/O(boundary-delta) bounds are CONDITIONAL on RegCap bounding the per-block reg
count and per-height bucket (cert R1, standing gate). This producer does not pin RegCap; it
inherits the gate. The bound is O(payload) IF RegCap bounds the bucket, and RegCap's value is
owed elsewhere (`era4-regcap-VALUE-DERIVATION-VERDICT-2026-08-29.md`).
</content>
