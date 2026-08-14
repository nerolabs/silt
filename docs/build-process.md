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

## The gate — seven rules, all cheap

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
