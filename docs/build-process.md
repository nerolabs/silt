# How we build: root-cause first, stop guessing

**Read this before you touch a knob to fix a failure.** It is the working companion
to **build-immutable #6** ("root-cause before you patch — attribute before you ship")
in [`TENETS.md`](TENETS.md) Part IX, and the *sequencing* dual of #3/#4/#5 (which
govern *what* is sound; this governs *the order in which you find the fix*). Source
of record: the research team's process note
`silt-reviews/research/research-outcome/build-process-root-cause-first-ADVICE.md`
(2026-08-12), itself distilled from the network-durability consults.

---

## The one idea

**Attribute before you ship.** If you cannot write the one-paragraph mechanism of a
failure — *the failure is X **because** Y; this change addresses Y **by** Z* — with
evidence, then you are **guessing**. Stop, instrument, reduce to a cheap repro, and
if the mechanism is still unknown (or the change touches security / consensus / a
published claim), **consult research before building or spending a billable run.**
Root-causing first is not slower; it is *cheaper*, because it collapses the
guess → run → fail → consult loop into one pass.

---

## The pattern this exists to break (with receipts)

Across the WAN field-test effort, one shape repeated and cost **days**:

> a failure appears → a **knob gets tuned** (raise a timeout, add a deadline, trade a
> parameter) → a **billable multi-region run** is spent to test the guess → it fails
> or is an anti-pattern → *then* research is consulted → the real cause was
> **structural, one layer down, and often already solved in the code.**

Every time, **the knob under the hand was not the cause.** That is the tell.

| Symptom | The guess that got tried | The actual root cause |
|---|---|---|
| Inbound handshakes EOF over WAN | raise handshake deadline 2 s → 10 s (a fixed-constant **anti-pattern**) | the hub had **no outbound addresses** — an architecture gap, not a timeout |
| Genesis block won't gather | k-for-size / FEC on the block (**band-aids**) | block is ~8 MB because it piles **all** founding bonds into genesis; spread them |
| Genesis can't bootstrap quorum | (about to build new bootstrap machinery) | **`launchAnchor` already existed**, comment describing this exact chicken-and-egg — just unused |
| Peers torn out of the mesh | evict-on-first-timeout | Kademlia's churn discipline (never evict a live peer on one miss) |
| Node brittle on flaky nets | raise `RequestTimeout` | that knob was *also* a security parameter (anti-release/C1) — a durability change silently weakened a proof |

The lesson: the fix was **not** the parameter under the hand. It was structural, and
twice the correct mechanism was **already written**.

---

## The gate — eight rules, all cheap

1. **Instrument → attribute → *then* change.** No knob moves before a log, trace, or
   test **names the mechanism**. The moment `-log debug` was captured on the #286
   round, it pinned the real blocker precisely — make that the **first** step, not the
   step after two guesses.

2. **Write the mechanism paragraph before writing the fix.** One paragraph: *the
   failure is X **because** Y; this fix addresses Y **by** Z.* If you can't write it
   with evidence, you're guessing — and **that is the trigger to consult research,
   before building, not after.** This single sentence is the whole gate.

3. **Cheap deterministic repro before expensive confirmation.** The `netem` /
   `flakynet` / sim harness makes failures cheap and repeatable on a laptop in minutes.
   **Every** WAN failure gets reduced to a local repro **first**; a billable cloud run
   is spent **only to confirm an already-understood fix**, never to *discover* a cause
   or *test* a guess. (Consensus-logic failures are deterministic — reproduce them
   in-process; you do not need the cloud to see an ~8 MB block.)

4. **Known-vs-unknown gate, routed to research.** Classify each blocker before
   touching it: *"I know the mechanism and the fix"* → build; *"I'm tuning a constant /
   trying something to see"* → **consult first.** Tells you're in the second bucket:
   you're adjusting a numeric parameter, you can't write the rule-2 paragraph, or the
   change touches a security property, a published claim, or a consensus rule.

5. **Security-parameter and consensus-rule changes are research-gated, always.**
   Twice the same knob was both a durability lever and a security parameter
   (`RequestTimeout` ↔ anti-release/C1). Any change to a parameter a security argument
   or a published claim depends on is **not** a build-alone decision (this is #3/#4/#5
   in force).

6. **Check whether the mechanism already exists before building a new one.**
   `launchAnchor` is the poster child — the machinery to bootstrap genesis without
   in-block bonds was already there, comment and all, just unused. Before building a
   fix, **grep for the intended behavior**; a surprising amount of this is
   *unused-correct* machinery, not missing machinery.

7. **Look one layer beneath the symptom before patching the symptom.** The recurring
   miss is symptom-patching: *EOF → raise the handshake timeout; block won't gather →
   shrink the proof.* The cause was one layer down each time (no outbound addresses;
   propagation design). Ask *"what is the layer beneath this symptom?"* before patching
   the surface.

8. **A gate's failure text may only claim what the gate measures.** A test that reads the
   project's own `.go` source sees strings and order, never behaviour — so its message
   starts `SOURCE GATE:`, says what it checked, and names either the runtime gate that
   observes the behaviour or the residual it leaves `UNGATED:`. Enforced by
   `scripts/check_source_gates.py` (scar count=3, 2026-09-03: the F7 daemon-death was
   reintroduced verbatim with `TestDaemonDegradesTheDemandLaneInsteadOfDying` green and
   `go vet` clean). The same rule generalises: whenever a gate, a comment, or a
   CHANGELOG line says *verifies / equals / matches*, name the axes it compares.

---

## Why it's cheaper

A wrong guess costs three things, not one: a **burned billable run**, an
**anti-pattern that ships and later gets ripped out**, and an **RC date that slips**
while the loop repeats. A research consult costs a few hours. Root-causing first
collapses the loop into a single pass.

---

## What we already do right (keep it)

This is a *sequencing* fix, not a competence critique. The honest `-log debug`
captures, the `netem` / `flakynet` harness, **holding code pending a research
opinion**, and the well-structured consults are all exactly right. The gap is
narrow: **patch-first instead of root-cause-first**, and **consult-when-stuck instead
of consult-when-you-can't-name-the-mechanism.** Close that and the same skill lands
the fix on the first pass instead of the third.

---

## The one line

**Attribute before you ship.** If you can't write the one-paragraph mechanism of the
failure and why your change addresses it, you're guessing — stop, instrument, reduce
to a cheap repro, and if it's still unknown (or it touches security / consensus / a
published claim), consult research *before* you build or spend a cloud run.

---

## Build-immutable #7 — evidence or nothing (the meta-rule over all forward motion)

#6 above governs a **fix**: attribute one failure before one patch. **#7 governs
*every* forward step** — a fix, a cloud run, a claim, a "next step", a "let me try" —
and it is the discipline this project has paid the most to learn. Ratified 2026-08-14
by the owner after the **guess → act → fail → guess** loop ("chasing our tails") burned
hours and billable runs across sessions. `TENETS.md` Part IX has the canonical text;
this is the working checklist.

**The gate — before ANY action that costs time, money, or commits a claim:**

1. **Say the evidence out loud.** Name the *specific* artifact that justifies this
   step: a log line, a trace, a failing test, a reduced reproduction, a measured
   number. Not a category ("the network is flaky") — the actual line.
2. **Catch the guess tell.** If your justification is *"I think / probably / likely /
   it usually / let me just try and see / it's worth a shot"* — **STOP. You are
   guessing.** That feeling is the signal, not a nuisance.
3. **When you have no evidence, your task changes.** The valid next action is to
   **gather** evidence — instrument, reproduce, capture — *not* to do the thing you
   were about to do. Go get the artifact; then decide.
4. **Iterate — one evidence-verified step at a time.** Smallest change a piece of
   evidence justifies → confirm *that* change with evidence → next step. A batch of
   hopeful edits is a batch of guesses. A run launched to "see what happens" is a
   guess with a bill.
5. **A non-locally-reproducible failure is INSTRUMENTED, not re-tried.** Add the
   logging / journal-capture / probe that will record *why*, let **one** instrumented
   observation gather it, then act on what it shows. "Re-run — probably transient" is
   allowed **only** when that re-run is itself the instrumented observation capturing
   the evidence you lack — otherwise it is a guess in a lab coat.

**The one line for #7:** *Say the evidence out loud. If you can't, you've just found
your real next task — go get the evidence — and it is not the thing you were about to
do.*

**The canonical loss it exists to stop:** a billable P1 cloud run whose sybil cohort
crash-looped, torn down by the harness *without capturing the crash journals* — so the
cause was unknowable and every next move was a guess. The fix was not a smarter guess;
it was to make the harness **capture the evidence first, then look**
(`integration/cloudtest` failed-node journal capture). Instrument, then observe, then
act — in that order, every time.

---

## The consensus-correctness discipline (canon, 2026-08-14)

Ratified after the PE process review (`docs/reviews/builder-process-notes-PE-2026-08-14.md`)
and the research team's independent convergence on the same diagnosis: **#357, B2, #397,
and #402 were one defect in four costumes** — a finality quorum that did not intersect
over its phase's real validator set. The class is finite and closed, and it is asserted
deterministically, not discovered one field run at a time.

1. **The map is canon.** `docs/design/consensus-invariants.md` (I1–I5) names the closed
   invariant set. Every consensus-touching PR states which invariants it touches and how
   it preserves each; every quorum site answers the research checklist in its code comment.
2. **The tier order is `unit → consensus model-check → integration/sim → e2e/netem →
   field`.** The model-check (`docs/design/consensus-model-check.md`) is the *first*
   consensus gate, and each graded field run is gated on the model-check tier covering its
   regime. A field run **confirms**; it never discovers a consensus invariant. (Four
   consecutive runs each discovering a new consensus bug is the base rate this corrects.)
3. **Consensus is boring, by policy (B8).** The novelty budget is spent on M0 only. Every
   consensus mechanism cites its literature analogue (watermark ↔ Tendermint
   `priv_validator_state`; weight quorum ↔ >⅔ voting power; anchor majority ↔ intersecting-
   quorum arithmetic). No consensus-engine rewrite — literature-faithful hardening of the
   existing chain.
4. **Every security property has a deterministic RED/GREEN home** (model-check or netem)
   where the attack can be *scheduled*. A field flow may honestly report GAP when its
   premise is unmet (S5) — but a GAP is acceptable **only because** the deterministic tier
   is the load-bearing grade. An undrivable security drill with no deterministic home is a
   RED to fix, never a GAP to accept.
5. **The third-time rule.** When a lesson is about to be documented a second or third
   time, encode it as a **test or a gate** instead of more prose. The canon grows by
   mechanism, not by amendment-log therapy.
6. **"Cite the analogue" means adopt the SCHEMA and know why each field exists — never
   just the purpose.** (PE ruling, #432, 2026-08-15.) The #397 watermark copied Tendermint
   `priv_validator_state`'s *purpose* (persist the last signature) and dropped its
   `(height, ROUND, step)` schema — and the dropped field *was* the liveness: the
   height-only mark permanently wedged a 2-2 proposer race (the two MATURING field
   starves). The rule cuts recursively: the FIX must adopt rounds *with the locking
   mechanism that makes them safe* (Tendermint lock/POL-change, HotStuff lock-on-QC), not
   "a round counter" — free higher-round re-signing re-opens I1 via a delayed lower-round
   quorum. When importing a literature mechanism, diff your struct against theirs
   field-by-field; every field you drop is a claim you can prove you don't need it.
7. **An oracle that observes an anomaly it cannot explain within its scope FLAGS or
   FAILS — it never assumes-benign.** (PE ruling, #432.) The tier-2 I5 oracle *saw* the
   wedge ("neither commits") and its comment rationalized it as "a benign launch stall the
   designated-proposer rule resolves in production" — an unverified assumption embedded as
   authoritative, and false (the rule serializes drain-vs-drain only; the stall was the
   permanent I4-liveness wedge). Same class as a harness hard-coding "#351 egress" into a
   verdict. If your test's setup or teardown shows behavior you can't prove benign, that
   observation is a finding to route, not a comment to write.
8. **A consensus quantity must be a function of the CHAIN — and how you bind it depends on
   whether its invariant is locally checkable.** (Owner ruling, 2026-09-10,
   `decisions.md` `D-PREIMAGE-CERT-2026-09-10`.) When the invariant is **locally checkable**,
   bind it with a **refuse-to-start** — `SlashesBytesCap`'s `2 × (budgets) ≤ cap` is pure local
   arithmetic, so a node can check its own config and refuse (`D-SLASHCAP-ROUTE`). When it
   requires **distributed agreement**, bind it to **committed or genesis-covered state** —
   `MinBond` divergence is *not locally observable*: no node can tell from its own config that a
   peer set a different value, so a start-up assertion has nothing to assert against and would
   ship a gate that looks green and enforces nothing. **A local assertion cannot enforce a
   distributed agreement.** Three instances taught this one at a time — #380 (`RequiredQuorum()`
   read `cfg.Quorum` on the objective path), `SlashesBytesCap` (an invariant derived from two
   proposer-side flag defaults), and `MinBond` (a validity threshold that is a bare flag) — and
   silt names the class in prose as *"consensus-critical genesis config"* (`core/chain/chain.go:190`,
   `:253`) with **no enumeration and no enforcement**, so membership is a human remembering to
   write the sentence. The corollary for tooling: **a config-in-consensus lint must check this
   PRINCIPLE, not pattern-match the instances already found** — a `Config` field that moves a
   validity verdict is either bound to the chain or it is a defect, and a field claimed safe must
   be DRIVEN into a regime where it could have mattered (simplicity rule 7), never assumed safe
   because an undriven fixture left it quiet.

---

## The ten simplicity rules (canon, owner direction 2026-09-08)

Standing owner direction, given via the PE seat and ratified in
[`decisions.md`](decisions.md) `D-RECOMPUTE-FREEZE` (5). **This is their canonical home.**
They were filed in the agent harness (`.claude/CLAUDE.md`) from 2026-09-08 until 2026-09-10
and moved here because canon that lives in the harness travels with the harness: a session
run without the usual configuration silently unloads it, and several of the ten were
breached while nobody was reading them. `.claude/CLAUDE.md` now carries a pointer.

**The numbering is load-bearing.** Rules 1–10 are cited by number across `docs/`,
`ROADMAP.md`, `CHANGELOG.md` and the `docs/thinking/` deliberations. Do not renumber, do
not insert. A wrong rule number still reads as a sensible sentence, so a renumbering breaks
every citation silently.

The owner asked which team we are: the one that takes a complicated thing and makes it simple, or the
one that takes a simple thing and makes it complicated. The measured answer for the trust plane's
last three weeks was the second (`docs/decisions.md` `D-RECOMPUTE-FREEZE`; the note:
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/NOTE-to-silt-team-simplicity-and-roadmap-reorder-2026-09-08.md`).
These ten rules bind every seat from now on:

1. **The B8 gate, at every design decision.** Before any seat proposes a mechanism, it names the
   settled corner it is buying — or states why none fits. "Novel" is a cost, not a feature, on
   anything outside M0. The planner does not dispatch a build that cannot name its corner.
2. **The PE seat's mandate widens: correctness AND simplicity of approach.** The PE's first question
   on any consult is *"is this the simplest approach the tenets permit?"* — before severity, before
   sequencing. This is the check the PE failed to run on the recompute keystone and now owns.
3. **Cap the bookkeeping ratio.** Bookkeeping commits (structure / register / residual / re-ruling /
   canon) may not exceed the count of `fix(` + `feat(` commits in any week. When they do, the loop
   stops registering and starts closing.
4. **A residual must be actionable or it does not exist.** A new `R-*` row requires an owner, a
   closer, and a Boulder. Otherwise it is a sentence in `docs/design/m0.md` §10 or nothing. No new
   prefixes (`R-BB-`, `G-`, …) without an owner ratification.
5. **Owner calls are batched and bounded.** At most five per true-up. If a true-up needs more than
   five, the loop is deciding by escalation instead of by design — stop and simplify the question.
6. **No new era without a ratified reason that is not "the recompute needs it."** A block-format
   change is the most expensive edit in the system; era 5 is not pre-approved.
7. **"A green gate with no demonstrated red is decoration" is a RULE, not an observation.** A field
   classified "safe" in any coverage table must be a DRIVEN probe (forge it, commit the divergent
   root, assert a stall). The completeness meta-test fails on an un-driven "safe" row, not only a
   missing one.
8. **Structure rounds are frozen on the keystone.** No refactor adds concepts to a component slated
   for re-scope. Rigor applied to accidental complexity produces more of it, beautifully certified.
9. **A cert per consensus-rule change — and only per consensus-rule change.** Not per probe, not per
   register row, not per re-ruling.
10. **Step back to TENETS + VISION once a week, as the planner.** No seat holds that vantage in the
    loop; the planner schedules it.
