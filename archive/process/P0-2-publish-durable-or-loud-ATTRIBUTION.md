# P0-2 attribution — "reliable async publish over WAN" (durable-or-loud)

**From:** silt build team · **Date:** 2026-08-12 · **Re:** principal-engineer `BUILD-NOTES.md` P0-2
**Discipline:** build-immutable #6 — *attribute before you build; the file refs in BUILD-NOTES are
hypotheses, not a diagnosis. Confirm first.* This note is the confirmation step, and it changes the
plan, so it is on the record before any code.

## The hypothesis (BUILD-NOTES P0-2)

> `durability-turnover` → **FAIL: "publish never produced a link."** … an S3/#60-class shape (a
> publish that strands content with no retrievable link) resurfacing over WAN. Fix: the publish path
> must be **durable-or-loud** — a link is returned only once the content is provably retrievable
> (placement confirmed by ≥k independent holders), and a publish that can't confirm fails visibly.

## What the code actually does (traced end to end)

The publish path is **already durable-or-loud, and already confirms ≥k retrievability.** No silent
non-link is possible:

1. **Placement is confirmed against the retrievability threshold before a link exists.**
   `node.distributeFrom` (`core/node/file.go`) tracks `shardPlaced[]` per shard, *requires* every
   redundancy-free chunk (manifest / uncoded) to place (B7), and before completing runs
   `understockedStripe(...)` — *"never register a link for content the swarm can't rebuild"*: if any
   erasure stripe kept fewer than its `k` reconstructable shards, it sets `distErr` and reports it.
   It even **retries** a group that lands nowhere (`placeAttempts = 4`) before failing.
2. **A failed scatter never registers.** `pipeline.RegisterAfterDistribute` publishes to the registry
   **only when `derr == nil`** (#65 register-after-distribute) — an understocked scatter returns the
   error and leaves the registry untouched, so no dangling entry, no link.
3. **The client publish is loud.** `httpregistry` returns `nil` only on commit; every terminal state
   is a typed error *with a reason* — no-quorum `failMsg`, an accepted-but-not-committed timeout that
   names the unfinished gather, conflict, payment (`httpregistry.go`).
4. **The CLI is loud.** `swarm add` prints the `silt:` link (`fmt.Println`, stdout) **only on success**;
   any error → `return err` → printed to stderr, non-zero exit (`cmd/silt/swarm.go`).

And it is **already tested at the sim tier** — `sim/publish_durability_test.go`:
`TestPublishFailsLoudWhenManifestUnplaceable`, `TestPublishFailsLoudWhenStripeUnrecoverable`,
`TestRegisterAfterDistributeLeavesNoDanglingEntry`, `TestManifestPlacementRetriesTransientFailure`,
`TestPublishSucceedsWhenStripeStillRecoverable`.

## Conclusion: P0-2's goal is already met; the hypothesis was wrong

There is **no silent-non-link defect**. "publish never produced a link" over WAN is a **loud** failure
(empty stdout, a reason on stderr, non-zero exit) — the harness greps stdout for the `silt:` link and
sees nothing, which reads as "no link" but is the *durable-or-loud* behavior working as designed.

## What actually causes the loud WAN failure (the real residual)

Two substrate/timing causes, both distinct from the publish path's correctness:

1. **Placement starvation under churn** — `distributeFrom` can't reach `k` live holders, so it
   *correctly* fails loud. This is the **#277 dial-storm**, which **P0-1 (just merged, #364) directly
   improves** (dead holders leave the candidate set, so live-holder dials aren't starved). The next
   field-test run should show whether P0-1 alone moves `durability-turnover` toward PASS.
2. **Issuer-set discovery timing (#351)** — a fresh/ephemeral CLI publisher must discover the
   *canonical issuer set* (`FetchCanonicalIssuers`, `swarm.go:236`) to acquire a publish token; over a
   real multi-region WAN — especially after a mid-run validator restart — that discovery races, and the
   publisher falls back or fails. The cloud harness **already attributes the setup-publish miss to this**
   (`integration/cloudtest/scenarios.sh:552`: *"ephemeral-CLI issuer-set discovery over WAN, #351"*).

## Recommendation

- **Do not build a publish-path "fix"** — it would patch a non-bug (the #6 anti-pattern).
- **Sequence:** let P0-1's placement improvement land in the next field run and measure its effect on
  `durability-turnover` before deciding. If a residual "no link" persists, it is **#351 (issuer-set
  discovery timing)**, which is the true P0-2 successor and deserves its own attribution (deterministic
  repro of the canonical-issuer-set race under netem/restart, then a fix: e.g. bounded discovery retry,
  or gate token acquisition on issuer-set readiness with a loud typed timeout).
- **Optional coverage add** (low urgency): a netem-tier publish guard (adverse loss+latency → link OR
  loud typed error, N seeds) mirroring `integration/adversarial`, to lock the durable-or-loud property
  at the adverse-network tier as well as the sim tier. Not a defect fix; a tier extension.

*Attribution complete; no code changed. The publish path meets the durable-or-loud bar. The forward
lever is P0-1's placement effect, then #351 — not the publish path.*
