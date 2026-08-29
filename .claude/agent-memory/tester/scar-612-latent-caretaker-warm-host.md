---
name: scar-612-latent-caretaker-warm-host
description: SCAR (latent, count=1): #612 repair-bounty premise determinism rests on caretakers NOT pre-hosting data shards. A future "caretaker warm-hosts data shards" optimization would silently break the confirm loop and fail loud on a healthy system.
metadata:
  type: project
---

# Scar: #612 latent coupling — caretaker warm-host data shards

**Failure class:** Latent coupling. The #612 repair-bounty premise determinism guarantee
depends on a fact outside the changed files. If that fact changes, the confirm loop breaks
on a healthy system.

**Source citation:**
- PE ruling (blind audit), 2026-08-27:
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-PR612-514-deterministic-premise-2026-08-27.md`
  — section "The coupling the consult did not fully surface":

  > "The determinism guarantee is load-bearing on a fact OUTSIDE the changed files: that a
  > killed data column cannot regain a live-node byte-holder except via repair. That rests
  > on the caretaker warm-start hosting ONLY manifest chunks (`repair.go:53`) and NetGet
  > dropping its working set (#500). If a future change made caretakers proactively pre-host
  > data shards of cared objects (a plausible durability optimization), the re-kill loop's
  > termination argument would break — a caretaker could hold a target-column byte the
  > harness cannot kill, and the confirm loop would fail loud on a healthy system."

**The exact code site (verified in PE ruling):**
- `core/node/repair.go:53` — caretaker warm-start fetches ONLY manifest chunks.
- `core/node/repair.go:820` — caretaker holds a data shard ONLY after an actual repair.

**Why it is a scar and not just a note:**

The failure mode is silent at the point of change. A "caretaker pre-hosts cared-object
data shards" PR looks like a durability improvement. Nothing in that PR's diff touches
`economy_repair_test.go`. The test would fail loud on a healthy, correctly-repaired system
— which is worse than a test that fails on a broken system. A developer would debug the
product, find nothing wrong, and be confused.

**Count: 1 confirmed (latent, not yet triggered).**

The PE ruling surfaced this during blind review of #612 (merged 2026-08-27). It has not
occurred as a real test failure. The count is 1 because the coupling is documented and
real; it is not 0 because the risk is not hypothetical.

**What must be encoded before this scar is closed:**

The following check belongs in the review checklist for any PR touching caretaker startup
or the cared-object fetch path:

> "Does this change allow a caretaker to hold a live byte-holder copy of a data shard
> BEFORE a repair event? If yes, re-examine `e2e/economy_repair_test.go`'s confirm loop
> termination argument."

Until that check is encoded in the PE review gate or as a code comment at `repair.go:53`,
this coupling is invisible to any reviewer who did not read this ruling.

**Links:** [[scar-repair-bounty-premise-defeat]]
