# Principal-engineer consult — #402: the launch anchor gate admits a one-free-anchor fork. Is `AnchorQuorum ≥ ⌈A/2⌉` the right call, and does it belong ahead of the red team?

**From:** build (2026-08-14)
**To:** principal engineer
**Re:** issue #402 · repro `core/chain/fork_anchor_gate_402_test.go` (green) · research consult `docs/reviews/fork-anchor-gate-402-RESEARCH-CONSULT-2026-08-14.md`
**Why you and not only research:** research owns *whether the construction is sound*; you own *whether this is RC-blocking, what severity it is against M0, and whether we hold the red team for it or ship the corner with an owned residual*. This note asks the engineering-judgment questions, not the crypto ones.

---

## The one-paragraph situation

A P1 field run (`4faaee8-22913`, the confirm run for the now-merged #397 fix) produced a flow-5 **"CAPTURE"** verdict. It is a **false positive** in the detector, but it surfaced a **real, pre-existing launch-phase seam**: with 4 anchors and `AnchorQuorum=1`, an honest commit at the bare count quorum (proposer + 2 attesters) leaves **one free anchor**, and a Sybil-proposed competing block that collects only that one free anchor's signature **passes the wheels-engaged commit gate**. The chain-tier repro confirms it, *and* confirms the boundary: a **zero-anchor** Sybil quorum is still refused, so **C2 (no quiet capture) holds** — this is a **fork-creation / liveness** vector, not a stake capture. `AnchorQuorum=2` (generally `⌈A/2⌉`) closes it, at the cost of needing **3-of-4 anchors up** to commit at launch.

## What is and isn't at stake (so you can size it)

- **NOT a C2 break.** No Sybil quorum without a real anchor ever commits. The mission's Sybil corner is intact; the repro asserts both directions.
- **IS a launch-window integrity/liveness bug.** One stray or adversarial anchor can co-sign a competitor at a height the honest side already committed → two chains at a height → the network partitions (the honest side kept its fork and idled ~26 min in the field). During the launch window the anchors are *our* explicit scaffolding, so "one anchor misbehaves or is simply slow-and-racing" is a within-threat-model event, not an exotic one.
- **The #397 watermark makes it sharper, not softer.** Post-#397 an anchor that signs the fork is *locked* to it for that height — so a fork, once formed, is stickier (the free anchor cannot later also sign the honest block). That is correct behavior; it just means the fork-prevention has to happen at the *gate*, not by hoping the anchor changes its mind.

## The three questions that are yours, not research's

**PE-1 — Severity call: RC-blocker, or owned residual?**
My read: **RC-blocker for the launch window**, because it is a *silent* fork that a red team pointed at seam #8 (the handoff/young regime) will find immediately, and "the launch net can fork under one stray anchor" is a worse story than "the launch net needs 3-of-4 anchors up." But it is *bounded* (no capture, self-heals once the gate is fixed), so if you'd rather ship M0 with `AnchorQuorum=1` + a documented owned-residual and fix post-launch, that is a coherent alternative I want your ruling on. Which?

**PE-2 — The liveness cost of the fix, against immutable #4.**
`AnchorQuorum=⌈A/2⌉` means launch commits need a *majority* of anchors live (3-of-4 at A=4), down from any-2-of-4 today. Immutable #4 is about the *honest-participant floor* (MiB not GiB, cheap to run), not about anchor count — and anchors are time-boxed scaffolding we control — so I read this as **not** a #4 violation. But it does reduce launch fault-tolerance, and I want you to confirm that trade is acceptable at M0 rather than assume it. (If A is chosen larger at launch — say 5 or 7 anchors — `⌈A/2⌉` keeps 1–2-fault-tolerance, which may be the cleaner answer: **pick the anchor count so `⌈A/2⌉` gives the fault-tolerance we want.** Is that a launch-config lever you want to set here?)

**PE-3 — Sequencing against the gate.**
The current plan: research ruling on #402 → build the anchor-gate fix → clean P1 → MATURING=1 field-cert (#389) → external red team (#183). #402 sits *ahead* of the red team by that plan. Do you agree it blocks the red team, or do you want the red team to run *against* the known seam (with #402 disclosed in the brief) as an independent check that we've scoped it correctly? The #397 arc argued for "fix before red team"; this is less severe (no capture), so the answer is genuinely yours.

## What I have already done (so you're ruling on facts, not vibes)

- **Attributed, not guessed:** green chain-tier repro isolating the exact gate behavior + its fix; the mechanism paragraph is in the issue and holds against the code (`ValidateCommit` `chain.go:1472`, `ValidateProposal` head-coupling `chain.go:1344`).
- **Contained the false positive:** the flow-5 detector now GAPs on a pre-existing fork instead of screaming CAPTURE (PR #404, merged), so the next run grades C2 honestly and this event can't masquerade as a capture again.
- **Held the fix:** no consensus code changed. The `AnchorQuorum` change is staged behind your severity/sequencing ruling *and* research's soundness stamp — per build-immutable #6, a consensus-rule change is not a build-alone decision.

## The decision I'm asking you to make

1. **PE-1:** RC-blocker or owned-residual? (my lean: RC-blocker, launch-scoped)
2. **PE-2:** accept 3-of-4 launch liveness for `⌈A/2⌉` — or set the launch anchor count so `⌈A/2⌉` yields the fault-tolerance you want?
3. **PE-3:** does #402 block the red team, or run alongside it as a disclosed target?

Everything else (research soundness of `⌈A/2⌉`, partition-heal/fork-choice weight) is in the research consult; this note is only the judgment calls above.
