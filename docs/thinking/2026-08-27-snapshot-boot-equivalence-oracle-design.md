# 2026-08-27 — The snapshot-boot-equivalence oracle: how to prove field-completeness without inspecting

**Context / trigger:** RED home #1 of the three load-bearing obligations in the
state-root keystone certification
(`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/D-TIERING-state-root-keystone-RESEARCH-CERTIFICATION-2026-08-26.md`).
The library question is closed (PR #596); this is the first oracle. The
certification's wording is unusually strict about *how* the proof may be
obtained:

> The completeness of the 16-field enumeration is the load-bearing obligation,
> and it must be **proven by the snapshot-boot-equivalence oracle, not by
> inspection** (inspection already missed fields — the PE's list was a subset).
> … Treat any field discovered later by that oracle as a soundness bug, not an
> optimization.

**Evidence (build-immutable #7 — artifacts, not vibes):**

- The 16-field enumeration and its read-sites:
  `silt-reviews/research/D-TIERING-state-root-keystone-CONSULT-2026-08-25.md`
  lines 43–72 (the table).
- **Inspection has already missed a field twice, and the second time was in the
  certification itself.** The consult enumerates `revoked` as field #3
  (`chain.go:765`, read by `validateTakedowns`). The certification's own Q3
  growth table lists fifteen fields and **omits `revoked`.** The PE's earlier
  3-field list was the first miss. This is not a hypothetical failure mode — it
  is the observed base rate of the method the oracle is replacing.
- The substrate exists: `core/chain/reload_era2_558_test.go` demonstrates
  build-a-history → `EncodeBlocks`/`DecodeBlocks` → fresh replica → `Reload`,
  all in-process in `package chain` (so unexported state is reachable).

## The trap this design exists to avoid

The obvious oracle is: capture the 16 fields, restore them into a fresh chain,
compare the 16 fields. **That is inspection wearing a test costume.** It is
sound only for fields someone already thought of. When field #17 is added next
year and nobody adds it to the capture list, a 16-field comparison passes,
green and silent — the exact silent-divergence class as #558.

So the design question is not "how do I compare state" but **"what mechanism
fails when a field nobody enumerated exists?"**

## Options weighed

**(A) Enumerated capture + field-by-field equality.**
Cost: low. Benefit: catches a field captured but mis-restored. **Forecloses
nothing, proves nothing about completeness** — it can only ever test the list it
was given. Necessary as substrate, insufficient as the oracle. Rejected *as the
oracle*; kept as its mechanism.

**(B) Reflection guard over the `Chain` struct.**
Walk `reflect.TypeOf(Chain{})` at test time. Every field must be explicitly
classified as either *committed state* or *excluded, with a written reason*
(the certification's correct exclusions: the I2 sign-mark, `rep()`, genesis
constants — plus mutexes, channels, config, and derived caches). **Fail on any
unclassified field.**
Cost: low, deterministic, no new infrastructure. Benefit: this is the only
option that catches the **unknown unknown** — a field added later by someone who
never read this document. It converts "we listed them" into "the compiler and
the test list them, and you cannot add one silently." It also operationalizes
the consensus-correctness discipline's rule 6 verbatim: *every field you drop is
a claim you can prove you don't need it.*
Weakness (owned): it proves the classification is *complete*, not that it is
*correct* — a validity-relevant field could be misclassified as excluded. It
forces that to be a conscious, reviewed, written act rather than an omission.
(C) covers correctness.

**(C) Leave-one-out ablation.**
For **each** enumerated field, snapshot everything *except* that field, boot,
drive both replicas with an identical adversarial schedule, and assert the
snapshot-booted node **diverges**.
Cost: 16 × an in-process differential run — cheap. Benefit: this is the sharp
one. It proves each enumerated field is *actually load-bearing*, and it turns
"we listed 16" into "each of the 16 demonstrably matters, and here is the
schedule that proves it." A field whose omission causes **no** divergence is a
finding either way: either it does not belong in the root (bloat on a forever
term), or the schedule is not adversarial enough to expose it — and per the
consensus discipline's rule 7, an oracle that observes something it cannot
explain **flags, never assumes-benign.**
Weakness: an ablation that fails to diverge needs a judgement call, so the test
must force that judgement rather than pass quietly.

**Decision: (B) + (C), with (A) as their shared mechanism.**
(B) catches the field nobody enumerated. (C) proves the enumerated ones are each
real. (A) alone — the tempting version — is the trap.

## How it ships failing-first

The certification wants a RED home, and this one has a free, historically-honest
RED fixture: **the misses that already happened.**

1. Write the oracle against the **PE's 3-field subset** (the documented first
   miss). Full-snapshot boot must diverge from replay boot → **RED**.
2. Extend to the certification's own Q3 fifteen — the one that drops `revoked`.
   Must still go **RED**, on a takedown schedule.
3. Complete to the consult's sixteen → **GREEN**.

That sequence is not a contrivance. Each RED step is a real enumeration a real
reviewer produced, and step 2 makes the point that the certification's own list
would not have survived its own oracle. That is the argument for the oracle,
executed rather than asserted.

## Scope boundary — what this does NOT build

The capture/restore is **test-only**. No product snapshot code, no SMT, no block
field, no consensus surface. This is deliberate:

- The consensus engine stays untouched (D-CONSENSUS §5). **I1–I5 untouched**;
  the oracle only *reads* state.
- The eventual product snapshot must be built over the same enumeration, and the
  reflection guard (B) is what will bind the two together — it fails if product
  code adds a field the snapshot does not carry.
- Per the #558 discipline the certification cites: the era-2→3 Reload test ships
  **ahead of** the change, extending the shared era-aware path. Same posture
  here — the oracle lands before the thing it will govern.

## What would reopen this

- If (C) finds an enumerated field whose omission never diverges, the Q2
  enumeration is wrong in the other direction (it commits something no validity
  predicate reads) and the field set — and the forever-growth analysis in Q3 —
  must be revisited, not the test relaxed.
- If (B) cannot classify a field without reading product semantics that are
  genuinely local (the sign-mark class), that exclusion is recorded with its
  reason in code, and it is a claim the next reviewer can challenge.

---

## What shipped (part 1) and what it found

`core/chain/modelcheck_state_completeness_test.go` implements (B) — the
completeness ratchet. It cross-binds **three** enumerations with reflection over
the live `Chain` struct, so none can drift:

1. `stateClass` — every field classified `committed` / `input` / `injected` /
   `transient`, each non-committed one carrying **the claim being made**.
2. `populateCommitted` — a distinctive value per committed field.
3. **`adopt` (product code, `chain.go:3172`)** — the reorg-path state swap.

The ratchet: a new `Chain` field fails (1) until classified; classified
`committed`, it fails (2) until populated; then it fails (3) until `adopt`
copies it.

**Proven failing-first by ablation** (each reverted immediately after):

| Ablation | Result |
|---|---|
| Add an unclassified field to `Chain` | **RED** — `Chain has 1 unclassified field(s): [ablationNewState]` |
| Delete `c.bondDomain = t.bondDomain` from `adopt` | **RED** — `adopt() did not transfer 1 committed field(s): [bondDomain]` |
| Drop `epochStart` from `populateCommitted` | **RED** — `populateCommitted left 1 committed field(s) at zero` |

### Finding 1 — product code and the certification already disagree

`Chain` has **25** fields. The certification enumerates **16** as committed
state. `adopt` — which is the existing, load-bearing answer to "what is derived
state?" — copies **19**: the 16, plus `blocks`, plus **`revLog`** and
**`epochStart`**.

So the disagreement predates this test, and it sits in the reorg path. Both
extra fields are written by `apply`/`rotateEpoch` from block history, which is
the certification's own definition of committed state. They are classified
`committed` here, with the discrepancy recorded in the classification itself.

`epochStart` is the milder case: its only reader is `Regime()`, the permanent
save/restore health instrumentation, so losing it misreports restore health
rather than diverging on validity. It still belongs in the snapshot.

### Finding 2 — `revLog` is history-dependent, and that is a keystone problem

This one is not bookkeeping. The certification's **Q1** selected the SMT over a
sorted-key Merkle on one decisive argument:

> the SMT root is "a deterministic root digest, regardless of the order in which
> keys have been inserted or removed … history independence is *not* necessarily
> provided by a sorted Merkle tree" … silt's soundness story *requires* the root
> be identical however the state was reached — a snapshot-booted node never
> replayed the history.

But `revLog` is a **CT-style append-only transparency log** whose root is a
function of **append order**, not of a key→value set (`apply` appends at
`chain.go:2736`/`2743`). It backs `RevocationLogRoot` / `InclusionProof` /
`ConsistencyProof` — the API that immutable #5's *provable non-globality* rests
on. A snapshot-booted validator that never replayed **cannot reconstruct it from
set-valued state.**

So the keystone must choose, and the certification does not address it:

- **(i) carry the whole log in the snapshot** — bounded by revocations, which
  Q3 rates as slow-growing; snapshot-booted nodes keep serving proofs; or
- **(ii) commit only the log root as a scalar leaf** — cheap, but a
  snapshot-booted node can serve the root and *not* inclusion/consistency
  proofs, which weakens the H9 guarantee for exactly the nodes that bootstrap
  fastest.

Per the certification's own instruction — *treat any field discovered later by
that oracle as a soundness bug, not an optimization* — this is filed rather than
absorbed.

## Part 2 — the differential half, shipped

`core/chain/modelcheck_snapshot_equivalence_test.go` implements (C). The
snapshot-booted replica is built **by reflection over the classification**, and
one detail falls out for free: `blocks` is classified `input`, not `committed`,
so "copy every committed field" yields a replica with **no history** — exactly a
snapshot-booted node — without asserting it separately.

**`TestSnapshotBootMatchesReplayBoot`**: with the full committed set restored, a
never-replayed replica answers every probe identically to the replayed one.

**`TestLeaveOneOutProvesEachFieldLoadBearing`**: omit one committed field, and a
verdict must change. Passing output is *evidence*, logged per field:

| Omitted | Probe | Replayed | Snapshot-booted |
|---|---|---|---|
| `byRoot` | dup-publish must be rejected | `reject` | **`accept`** |
| `revoked` | un-revoke a revoked root | `accept` | **`reject`** |
| `bondRootOwner` | second identity takes an owned bond root | `claim-blocked` | **`claim-succeeded`** |

The third is the one that matters: without `bondRootOwner`, one plot backs two
identities — a direct **C1 no-discount** break.

### Two corrections the oracle forced on its own probes

It first reported three fields as "not load-bearing." Attribution showed the
**probes** were wrong, not the fields — which is the oracle working:

- **`bondRootOwner`/`bondRootProven`**: F1 first-owner-wins lives in `apply()`,
  **not** in a validate predicate, so a verdict-only probe is structurally blind
  to it. Fixed by an apply-time probe.
- **`bondRegHeight`**: the min-interval rule is gated behind `regGateActive`
  (#506), inactive in this world, so it never fires. Moved to declared debt with
  that reason.

A third correction was about faithfulness: omitting a map initially left it
`nil`, so `apply` panicked. A panic is divergence, but it **masks the finding** —
the point is to see what a node wrongly *accepts*. Omission now yields an
initialised-but-empty map, which is what a snapshot that failed to carry a field
would actually produce, and that is what turned `panic` into `claim-succeeded`.

### The declared debt

`probeUncovered` names each committed field with no probe yet **and what a probe
would have to construct** (a token replay for `spent`, a committed equivocation
for `slashed`, a gate-active world for `bondRegHeight`, and so on). The test
**fails if a committed field is neither probed nor declared**, so the debt
cannot grow silently. It is a shrinking list, not a permanent excuse.

## Honest scope — what is NOT yet built

Part 1 and part 2 are shipped. It proves the enumeration cannot silently
drift. It does **not** yet prove the enumerated set is *sufficient* — that is
the differential half, still owed:

- **Probe coverage is 3 of 18 committed fields.** The remaining 15 are declared
  in `probeUncovered` with what each would need. The highest-value next ones are
  `spent` (token replay — a double-spend), `slashed`, and `bonded`/`epochSet`
  (quorum sizing), since those carry the most consensus weight.
- **No adversarial *schedule*.** Probes are single validity questions, not a
  partition/restart/reorder schedule of the kind the model-check tier drives.
  Fields whose loss only shows up under a schedule are not yet reachable.

Nothing in part 1 touches the consensus engine. **I1–I5 untouched** — the tests
only read state and call the existing `adopt`.

---

## RED homes #2 and #3 — shipped, with the lesson that mattered most

**#2, the incremental-cost oracle** (`internal/smtspike/incremental_cost_test.go`)
counts **digests, not wall-clock**. The floor-box work measured ~2× timing
variance on shared hardware, so a time budget would be either too loose to catch
a regression or flaky enough to get disabled. `smt`'s `digestData` is
Write→Sum→Reset, so a `hash.Hash` wrapper counting `Sum()` calls counts hash
computations exactly, identically on a laptop and a 1 vCPU box.

Applying 64 changed keys costs 544 / 780 / 978 digests at 1k / 10k / 100k —
**1.80× growth for 100× the state**, and 6.78× for 8× the changed keys. The
budget constant is derived from `TestIncrementalCostReport` (0.85–1.61 measured,
so `budgetK = 3`), not guessed; the first draft used 12, which the report showed
was ~7× too loose to mean anything. A full recompute costs 44,733 digests, 18×
over budget — and **that test fails if the budget is ever loosened enough to
admit it**, so the GREEN assertions cannot be made vacuous by inflating the
constant.

**#3, the era-boundary Reload oracle** (`core/chain/reload_era3_boundary_test.go`)
pins two properties era-3 will need: a mixed-era history must replay through the
single `verifyAtt` dispatcher, and a future-era block must be rejected **loudly**
with an honest restored-prefix count — #558's lesson carried forward, since the
damage there was the silent fallback, not the rejection.

### The lesson: the first version was hollow, and only ablation revealed it

The era-boundary test passed on first run. It was **meaningless**. Its "mixed
era" history was an era-1 *genesis* plus an era-2 block — and genesis carries no
attestations at all, so the `PhaseLegacy` branch of `verifyAtt` was never
executed. Deleting that branch entirely left the test **green**.

That is the whole failure mode this document opened with, reappearing in the
test written to prevent it: an assertion that looks like coverage and is
actually decoration. It was caught only by the standing rule that a passing test
proves nothing until you have watched it fail — the fix was a real era-1
*attested* block at height 1, after which the ablation turns it RED.

Worth generalising, because it recurs across all three oracles in this session:

- Part 2's leave-one-out first "passed" via a **nil-map panic** rather than by
  demonstrating what the node wrongly accepts. A panic is divergence, so the
  test was green — and useless. Modelling omission as an empty map turned
  `panic` into `claim-succeeded`.
- #2's budget was green at `budgetK = 12`, a value that would have admitted
  regressions many times over.
- #3's boundary test was green while covering nothing.

**Every one of the three passed before it was correct.** The green came first
and the meaning came second, each time surfaced by asking what defect should
turn it red and then actually injecting that defect. Ablation is not a
nice-to-have on an oracle; on this evidence it is the only thing separating an
oracle from a comment that compiles.
