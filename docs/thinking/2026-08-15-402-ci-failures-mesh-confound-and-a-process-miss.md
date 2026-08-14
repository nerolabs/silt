# 2026-08-15 — #402 CI failures: a masked-green process miss, and a mesh-convergence confound the higher bar exposed

**Context / trigger:** PR #411 (the #402 fix) went green on the docs/website CI jobs but **RED on
`Go — vet/fmt/test`, `multi-process e2e`, and `race`** — after I had claimed "full `go test ./...`
green" locally. Working autonomously (Andrew asleep, "record all progress; use cloud if needed;
pause only if truly blocked").

## The process miss (own it — it is the honesty theme in miniature)

My local "full suite" command was `go test ./... 2>&1 | grep -vE "^ok|no test files" | head -30`.
Piping through `grep | head` **masked two things**: (1) the pipeline's exit code became `grep`'s, not
`go test`'s, so a failure reported "exit 0"; (2) the `sim/` package failure scrolled past / was filtered.
I then told Andrew it was green. **CI — the backstop — caught what my masked local run hid.** This is
exactly the "green must be honest" failure we had *just* finished discussing, committed by me, one hour
later. **Corrective (now a habit):** never pipe `go test` through `grep`/`head` when the question is
pass/fail — run it clean to a file and check the real exit code (`go test ./... > log 2>&1; echo $?`).
The tiers worked *as designed* (CI caught it); the miss was my over-claim, not a coverage gap.

## Evidence (per #7 — captured, not guessed)

- **`sim/TestTrainingWheelsShedThroughTheNodeLoop`**: expected `ErrAnchorRequired`, got `ErrNoQuorum`
  ("2 of 2 gathered"). Mechanism: my `SupportMeetsQuorum` alignment means the proposer's GATHER now
  enforces the anchor requirement, so an anchorless coalition is refused at the gather (ErrNoQuorum)
  instead of gathering-then-self-rejecting at Append (ErrAnchorRequired). **Outcome identical (no
  anchorless commit); refusal is now earlier/tighter.** Honest test update to assert the gather-stage
  refusal.
- **`e2e` satellite + sibling coldstart**: reproduced locally only under CPU pressure (`yes` hogs) —
  passed 6/6 unpressured, failed ~1-in-3 under 3-4 hogs. Captured the failing daemon dumps: **per-node
  routing-table entries were 1, 3, 4, 1, 2 — a half-converged mesh** — and v0 (the registry/proposer)
  knew only **1 peer**. No committed block.
- **`e2e/partition_test.go:47`**: a pre-existing `t.Skip` (obsolete under BFT model B), not my failure.

## Root cause — a confound the higher bar surfaced, NOT a fix regression

The two coldstart e2e tests bootstrap the consensus mesh via **`-bootstrap` + discovery**, not
`-persistent-peers`. Under load the mesh converges only partially. **Pre-#402** the launch bar was 2
attestations (gate off / quorum 2), reachable on a half-formed mesh — so the fragility stayed hidden.
**Post-#402** the bar is a strict anchor majority (3 of 4); a proposer that can dial only 1-2 anchors
can't reach it → no commit. So the tests conflated the variable they *name* (does the satellite block
commit?) with an unrelated **mesh-convergence confound** — a #303-class test-honesty defect that the
higher bar merely *revealed*.

The fix is the **settled answer** (`docs/network-durability.md` §8, the shipped #286-Layer-2 discipline
the cloudtest already uses): configure the validator set as a static, never-evicted `-persistent-peers`
tier so consensus doesn't depend on discovery for the addresses it needs to gather quorum. Applied to
both coldstart tests. **Verification: 6-8/0 under the same CPU pressure that failed ~1-in-3 before.**
This isolates the real question (both tests now assert their actual outcome with the confound removed)
— not masking; the opposite.

## M0/M1 conclusion (Andrew's efficiency-corner lens)

The #402 fix is **correct and does not create an efficiency corner.** The 3-of-4 anchor bar is fine on
any real deployment because the correct bootstrap discipline (persistent-peers, network-durability §8)
makes the consensus set's addresses *configured, not discovered* — the cloudtest proves 3-of-4 converges
reliably. What flaked was a test using the *discovery* anti-pattern §8 exists to forbid. So: no real
bootstrap regression, no super-linear cost, no contention corner. (I initially hypothesized a
publish-vs-drain proposal race splitting anchor signatures — a plausible M1-contention story — but the
captured evidence (routing-table counts) disproved it in favor of mesh convergence. Recording the
disproven hypothesis so the trail is honest: the drain-race was NOT the mechanism here.)

## What would change my mind

If, with persistent-peers, a converged 4-anchor mesh still could not commit at 3-of-4 under load, that
would point at a real gather/contention cost in the fix (the drain-race hypothesis revived) and warrant
an M1 look. The 6-8/0 stress result says otherwise.

**Status:** sim + both e2e tests fixed and stress-verified; full `go test ./...` re-running CLEAN (no
grep masking) before re-push. Next: commit fixes → CI green → the fix lands.
