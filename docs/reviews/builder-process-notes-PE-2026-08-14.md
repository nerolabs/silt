# Notes to the builder — process, from the PE seat (2026-08-14)

Written at the owner's request, to pass along the process feedback from a week of consults. Not a ruling and not canon — notes from someone who has watched the build closely and wants you to have the pattern, not just the per-bug answers. Read it once; it's short on purpose.

---

## First: the discipline is genuinely excellent. Keep all of it.

I audit for a living and I look for problems, so read this plainly: the *way* you work is not the problem. Specifically, keep doing these — they are senior-team habits and they're rare:

- **Root-cause before patch (#6).** Your attributions have been *real*, not guessed — captured journals, deterministic failing-first repros, a mechanism paragraph that holds against the code. The #397 consult (missing `n.attested` write, with the repro that isolates attest-after-propose) is a model of it.
- **Repro at the tier the bug lives, failing-first.** You keep landing the RED test on the right tier before the fix. That's what makes a fix *stick*.
- **Consult before touching a claim or a consensus rule.** Every single time, you stopped at exactly the claim-touching line — token-gather privacy, quorum sizing — and routed it. That restraint is the mark of the team, not a weakness.
- **Honest attribution even when the answer is "no defect."** The P0-2 call — "the publish path is already durable-or-loud; do not build a fix for a non-bug" — was exactly right and took discipline to *not* ship a fix. A week earlier that loop would have "fixed" it three times.
- **You turn lessons into enforcement.** The wanguard ledger, the #6 pre-flight gate on billable runs — you don't just learn a lesson, you make it un-forgettable in code.
- **The blind field test earned its keep immediately** (it found #357, #397, #402 before any external party could). Keep running *discovery* blind — a fresh executor with no builder-knowledge is your best bug-finder.

None of that changes. Everything below is one specific gap, not a critique of the craft.

---

## The reframe: you are not chasing your tail

It *feels* endless because #357, the B2 handoff issue, #397, and #402 look like four different bugs. **They are one bug in four costumes:** a quorum that doesn't *intersect*, or doesn't *hold still*, admits a conflicting commit or a fork. You've been meeting the oldest theorem in Byzantine consensus at four different doorways.

That feeling has a name and it's diagnostic, not fatal: it's what **re-deriving a known theory one edge at a time** feels like. You've been walking the perimeter of BFT correctness, and you only find each edge when a multi-region field run trips it — so it feels like a circle. It's a **spiral**: dead → oscillating → live-under-hostile-load. Each lap is at a deeper correctness level; the bugs *rhyme* because they're the same theorem, not because you're going in circles.

And the class is **finite and closed**. Everything you've hit lives under five invariants (`docs/design/consensus-invariants.md`). You're most of the way through the list. There is an end, and you can now see it.

---

## The three anti-patterns to retire (each drawn from a real loss)

**1. Don't discover consensus correctness with a field run.** #286 taught "don't spend a billable run to *discover* a cause." The deeper version: **don't spend a field run to discover a consensus *invariant*.** A multi-region run is the most expensive, slowest, least-deterministic possible fuzzer for a spec that was never written down. The fix is `docs/design/consensus-model-check.md` — a laptop property test that asserts all five invariants under an adversary in seconds. Field run's job goes back to *confirming* what the model proved.

**2. Don't spend the novelty budget on consensus (B8).** BFT is solved — Tendermint/CometBFT, HotStuff, Casper/Gasper settled this cluster a decade ago. Every consensus bug so far was a place silt rolled its own corner instead of importing the settled one. *Boring parts, novel car* — and the novel car is **M0**, which is in genuinely good shape (the crypto is built, the honesty markers hold). Make the consensus layer as boring and literature-faithful as you can; save the invention for the part that deserves it.

**3. Don't reclassify a security GAP to make a run "pass."** The "GAP (not FAIL) when the attack can't be DRIVEN" pattern is scoreboard management. An undrivable *security* drill is a **RED to fix on a deterministic harness**, never a GAP to accept. If the harness can't force equivocation, the harness is broken — fix it where you can schedule the attack (netem / model-check), and let the field prove liveness, which is the thing only it can prove.

**And the meta-lesson that ties them together: don't fix the instance when it's the class.** #402 was the *third* sighting of "non-intersecting quorum admits a fork" (after #397 finality and B2 handoff). Patching each instance one field run at a time *is* the tail-chasing. Closing the class — write the invariant, assert it in the model-check — is what ends it.

---

## One gentle watch-out

The discipline can tip into **processing pain by writing.** The canon is at 2,900+ lines, and the amendment log started reading as real-time self-therapy during the WAN thrash; the buildlog going silent in that same stretch was the tell. Write the invariant down **once** (the map), not the same lesson across five amendment-log entries. When you feel the urge to document a lesson a third time, that's the signal to instead encode it as a *test* — a guard the next person can't skip. You already do this well (wanguard); lean on it harder than on prose.

---

## The two tools that end this flavor of pain

1. **`docs/design/consensus-invariants.md`** — the map. Five invariants (quorum intersection everywhere; never-sign-twice persisted; set-changes-at-finalized-boundaries-by-weight; commit ≠ final; deterministic fork-choice / honest-never-slashed), each with the scar that proved it and the code site it governs. **Working rule:** every consensus-touching PR states which invariants it touches and how it preserves each.
2. **`docs/design/consensus-model-check.md`** — the harness. A deterministic adversarial property test over the *real* core (reusing `simclock`/`simnet` — not new infra). Its acceptance test is the proof it's worth building: **checked out before each fix, it must go RED on #357/B2/#397/#402** — i.e., it would have caught all four on a laptop in an afternoon. Then it becomes the *first* consensus gate (unit → **model-check** → sim → netem → field) and a red-team entry criterion.

If you build one thing next on the process side, build the **N=4 launch tier of the model-check first** and prove the four failing-first replays. That single deliverable validates the map *and* retires the whole bug class before the external red team — the highest-leverage move available.

---

## Bottom line

The craft is right. The pain was a specific, fixable gap: consensus correctness was *emergent* — a pile of individually-correct rules with no written statement of the closed set they must satisfy — and it was being fuzzed by the most expensive fuzzer money can buy. Write the five invariants down, assert them on a laptop, and this specific flavor of surprise stops. What's left after that is the genuinely novel M0 work — which is exactly where a project like this is *supposed* to be hard.

You've built something that's live under hostile load with a real, honest Sybil-resistance composition underneath it. That's further than most attempts at this get. Give yourself the map, and finish walking the fence.

— PE (audit & rescue seat)
