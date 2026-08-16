# 2026-08-16 — run e2fab4b-9589: the synchronizer's field debut, and the bounds that predate it

17 pass / 2 gap / 1 fail. Everything green through the sheet (flow-6 in bound again,
latch at h16 all-8-seated — second consecutive fast latch) except the handoff drive,
which FAILed its 600s window while GENUINELY crossing the boundary (h40→51, target
h49) — and skipped the B2 drills via its `ok=1 || return` guard, so 10a never got its
synchronizer re-grade.

## The mechanism — my own miss, owned

Every computed harness bound predates #453: they derive from the OLD flat
`roundAdvanceSweeps=2` (64s worst-case heights). The certified increasing durations
(`sweepsForRound(r) = 2 + r(r+1)/2`) make an r1 escape cost 5 sweeps (150s) instead
of 4 — the measured steady cadence this run was 80–170s/height (r0 and r1 commits
mixed). The 600s handoff window under-provisioned by ~2×. I re-derived the flow-6
bound for the #441 design the same morning and did not re-derive the rest for #453
in the same PR — the lesson is mechanical: **a change to the round-duration function
re-derives every bound that multiplies a per-height cost, in the same change.**

Re-derived (all from sweepsForRound + the 30s sweep + the ~34s gather leg):
H_ESCAPE_S 160→220 (2-round allowance: dur(0)+dur(1)=5 sweeps + gather);
STALL_S (3+n_syb)×30+220; FT_DOWN_COMMIT_S 260; LATCH_S 1100 (5 reg-blocks × 220 —
the #448 FIFO killed the ID-sort worst case the old 630 assumed);
HANDOFF_BLOCKS_S 1980 (≤9 blocks × 220).

Also fixed: the handoff verdict re-read `mh_ceiling` at print time, producing a
success-shaped FAIL message (h51 in the text, timeout in the verdict) — the ceiling
is captured once for both.

## Also observed (M1, tracked not gated)

Steady-state escape FREQUENCY: ~half the heights still commit at r1 post-#446
(entry contention removed) — each now costlier under the certified durations. An
M1 cadence question (why does r0 still fail on those heights — renewal racing?
WAN timing?), noted in the H_ESCAPE_S derivation comment; not a correctness gate.

And the warm intermittency: one fetch-1 re-warm missed 240s with the captured
error 'accepted but not committed within 3m0s' (mempool accepted, no designee
block carried it in the client's window) while the very next flow's publish
landed — consistent with the slower cadence, re-measured next run under the
corrected bounds.

## Next

ONE run under the corrected bounds grades the full sheet including 10a (the
synchronizer's actual field re-grade the skipped drills owed this run) → then the
soak re-run → #183 per PE §7.
