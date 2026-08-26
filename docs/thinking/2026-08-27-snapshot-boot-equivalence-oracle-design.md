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

## Honest scope — what is NOT yet built

This is **part 1 of RED home #1**. It proves the enumeration cannot silently
drift. It does **not** yet prove the enumerated set is *sufficient* — that is
the differential half, still owed:

- **Replay-boot vs snapshot-boot equivalence**: build a rich history, boot one
  replica by replay and one by restoring the captured state, drive both with an
  identical adversarial schedule, assert identical verdicts and state.
- **Leave-one-out ablation** (option (C) above): for each committed field,
  snapshot everything *except* it and assert the snapshot-booted node
  **diverges**. A field whose omission never diverges is a finding either way —
  either it does not belong in the root, or the schedule is not adversarial
  enough.

Nothing in part 1 touches the consensus engine. **I1–I5 untouched** — the tests
only read state and call the existing `adopt`.
