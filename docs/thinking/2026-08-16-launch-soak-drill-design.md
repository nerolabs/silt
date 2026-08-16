# 2026-08-16 — the launch publish/drain SOAK drill: design before code

## What the PE required, exactly

The #432 ruling's gate (`i4-liveness-wedge-rounds-ruling-PE-2026-08-15.md`): *"re-confirm
the launch regime survives an interleaved publish/drain race (a launch-topology liveness
drill, now that we know P1 didn't cover it)"* — because publish proposals and drain
proposals are uncoordinated in both regimes, the #402-B launch regime proposes
anchor-only, and the wedge needed only one crossed race with a 2-2 anchor split. P1's
clean pass never held the two proposal streams open against each other. The follow-up
framing (owner queue): a **soak** shape, not a scheduled race — roll the dice repeatedly
rather than choreograph one collision.

## Why a soak, and why it cannot share the MATURING confirm run

A scheduled race would drive one crafted collision — but the wedge's discovery history
says the race arms *organically* whenever publish traffic and renewal drains coexist; the
soak holds both streams open and lets the schedule land where it lands, many times. And
the drill is a **launch-regime** property: in a MATURING=1 topology the launch window
ends at the latch (~13 min in on 09fbe60-84613), and consuming it with a soak would
starve the flows the run exists to grade. A MATURING=0 (P1-shape) topology keeps the
launch regime permanent (`MatureValidators` unreachable), so the soak is a separate run.

## The drill (flow `soak-publish-drain`, MATURING=0 only)

Two uncoordinated proposal streams, held open together for a computed window:

- **Publish stream:** a publish loop from the warm publisher (and periodically the fresh
  publisher via the existing helpers) — each publish is a proposal from whichever
  validator carries it.
- **Drain stream:** the sybil cohort's TTL renewals (`SubmitBondReg` → the designated
  drain proposer), running at their natural cadence — no artificial scheduling.

**The oracle, per height:** the chain never *permanently* stalls — every height commits
within the computed per-height escape bound

```
H_BOUND = rounds_allowance × (roundAdvanceSweeps × ChainSyncInterval) + gather_leg
        = 2 × (2 × 30s) + 34s ≈ 154s → 160s
```

(the #432 escape needs `roundAdvanceSweeps` sweeps to fire a round-change; allow two
rounds — the 09fbe60 steady state committed at r1 — plus one computed gather leg. A
height older than H_BOUND with the network live is the wedge signature and a FAIL, not a
window artifact: PE §4, a miss inside a principled bound is a finding.)

**The soak window:** the pre-fix wedge bit within ~6 heights of interleaving in both
MATURING field starves. Soak for `SOAK_HEIGHTS = 20` committed heights (≥3× the observed
exposure) or the wall-clock cap `SOAK_HEIGHTS × 64s ≈ 1280s`, whichever first — bounded,
never "eventual completion".

**Also asserted through the window:** zero honest slashes (I5 — jlog census of `slashed`
lines against the known-honest set); publishes keep completing within `PUBLISH_RETRY_S`;
round-changes MAY fire (they are the escape working, logged not failed).

## What is deliberately NOT in scope

- No mature-regime soak here — the mature steady-state r0-contention question is routed
  to the node-level mature-epoch fixture (PR #440's world), where a deterministic
  schedule can reproduce it; a field soak cannot attribute it.
- No new daemon code, no knobs: the drill drives existing product paths only.

## Sequencing note (owned)

Queue items ran 3 → 5 → 4 rather than 3 → 4 → 5: item 4 edits `scenarios.sh`, which the
in-flight run-3 process is executing (editing a running bash script corrupts execution),
while item 5 was test-only Go, safe during the run. The soak drill code lands after run-3
tears down; its run is the next billable event after that.
