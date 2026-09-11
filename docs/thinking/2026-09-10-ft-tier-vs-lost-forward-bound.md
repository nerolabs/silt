# The `6-fault-tolerance` hard cap is priced below the bound its own scenario publishes

**Date:** 2026-09-10. **Status:** FINDING ONLY — nothing changed, by owner direction
(2026-09-09: investigate and write it up; changing a standing RC gate's SLO tier is not a
side effect of an audit).
**Origin:** suspect 3 of the pre-freeze derivation-route audit,
`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-derivation-route-audit-pre-freeze-2026-09-09.md`.

## The claim, verified

`integration/cloudtest/scenarios.sh` computes the `6-fault-tolerance` tiers from `f`, the
number of DOWN seats — correctly, and the seat-count rungs of #525 are rightly struck:

    ftsum   = Σ_{r=0}^{f} (2 + r(r+1)/2) · 30       # f=1 → 60 + 90 = 150
    expected = ftsum + skew(30) + G(10)             # f=1 → 190 s
    hard     = 2 × expected                         # f=1 → 380 s

`D-H43-WORKLESS-DESIGNEE` (21) publishes silt's liveness after GST as **≤ f′+1 rounds**, and
states two numbers for this exact configuration: with the entry forward landing, `f′ = f` and
the commit lands in **190 s** at f=1, N=12 — *and* **a lost forward is bounded by the re-keyed
takeover at ≤ (N+2)·ChainSyncInterval + G = 430 s at N=12.**

So the scenario's own published worst case is 430 s, and its hard cap is 380 s.

## Why the two bounds are the same scenario, not different ones

The natural defence is that 190/380 prices the *down-designee* ladder while 430 prices a
*lost entry forward* — different failure modes, so no conflict. That defence does not hold
here, because the flow exercises both:

1. The flow stops one validator and then publishes (`ft_publish fetch-1 262144`).
2. The h43 fix forwards pending ENTRIES to the round's designee on round entry.
3. If that forward is lost, the designee is LIVE but workless. The empty-block refusal is a
   validity rule, so its round is wasted exactly as a down designee's would be, and the
   re-keyed takeover walk fires — the 430 s path.

A commit at, say, 400 s is therefore **legal in the ratified model** and would be graded a
failure by this row. `ft_wait_new_block` would exhaust `FT_DOWN_HARD_S` and the row would go
red with an escape fingerprint, on a network that behaved correctly.

## Why this is the audit's F1 face, not a rounding quibble

The hard cap is not model-derived at all. The comment states its provenance plainly: *"the
hard cap is 2× = 380 s (sweep-phase + request-timeout noise)."* It is the modal bound times
an arbitrary noise multiplier. That is precisely the shape the audit was commissioned to
hunt — a defensive threshold sized against the **modal** number when its purpose requires
the **worst the model admits** — and it is the same shape that produced the delivery idle
window's original derivation.

The tell is that the multiplier and the bound are unrelated quantities. `2×` happens to
clear 430 at f = 2 (`ftsum` = 330 → hard 740) and happens not to at f = 1. Nothing connects
the coefficient to the takeover walk it needs to dominate.

## What a fix would look like — NOT applied

    hard = max( 2 × expected, (N + 2) · ChainSyncInterval + G )

At f=1, N=12 that is `max(380, 430)` = 430 s. It costs nothing on a healthy run: the last
graded run (`97e3101-deep`) committed inside 190 s, so the row passes on the expected tier
and never reaches the cap. What it buys is that the cap stops being able to fail a legal
outcome.

Two reasons to route this deliberately rather than patch it:

- **It changes what a standing RC gate asserts.** `6-fault-tolerance` is the row the h43 arc
  was graded on. Widening its cap is a claim change about the release gate, not a test tweak.
- **The formula needs `N`**, which the tier computation does not currently read — it is
  derived from `f` alone, on purpose. Introducing `N` re-opens a question the #525 strike
  settled (rounds burned scale with `f`, not with `N`). The takeover WALK does scale with
  `N`, so both can be true, but the distinction has to be stated where the strike is
  recorded or the next reader re-litigates it.

## Severity

**Latent, not observed.** No graded run has failed this way: the defect surfaces only when a
forward is actually lost, and the two most recent runs committed on the modal path. The cost
of leaving it is a false red on some future run — expensive in trust and in hours, since the
first response to a red FT row is to hunt a liveness regression that is not there.

Owner's to sequence. It is test-tier work (deadline: the stamp raise), not a format item.
