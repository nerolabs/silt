# Research consult — #402: the launch anchor gate (AnchorQuorum=1) admits a one-free-anchor FORK; the honest side then partitions and idles

**From:** build (2026-08-14)
**To:** research team
**Re:** issue #402 — field run `4faaee8-22913` recorded a flow-5 "CAPTURE"; the captured evidence + a green chain-tier repro show it was a **fork, not a capture**, and name a **consensus-rule** decision that is yours to rule on.
**Status:** consult, **RC-gating** (blocks the P1 gate → MATURING cert → red team #183). Q-A is fully attributed with a deterministic repro (`core/chain/fork_anchor_gate_402_test.go`, green). Q-B is a grounded hypothesis I am explicitly NOT asserting without your read. Per build-immutable #6 rule 5, no consensus change ships before you certify.
**Provenance:** found by the P1 confirm run for the #397 fix — the fix itself is field-confirmed (run `9b2198e-67673`, 15/0, perfect convergence). This is a *different*, pre-existing seam the #397 watermark makes sharper.

---

## The event (captured evidence)

Run `4faaee8-22913` (4 anchors, 8 bonded single-domain sybils, `AnchorQuorum=1`, `Quorum=2`, wheels engaged the whole run):

- **~15:24 — two competing block-11s.** All four anchors committed `block 11 (1 entries)`, head `c40d4b68` — flow 5-convergence PASSed on it (tip 11, durable). sybil-1 committed a **different** `block 11 (0 entries)`, checkpoint `11:4ddcd792…`, and extended it to `12 (3a759f84…)` and `13 (c9b0447c…)` — each logged with `2 attestations` and, decisively, the daemon's own `wheels engaged (anchor quorum still required)` status line. **No anchor ever held 12/13**: every anchor's post-run restart logged `chain: restored 12 block(s)` (heights 0–11, the honest fork).
- **The honest chain then sat at height 11 for ~26 minutes** (15:24 → the 15:50 flow-5 stop), every publish failing — but the publish failures were **token-gather latency with validators 4/4 reachable and no error text** (#351), not consensus refusals.
- Flow 5's detector misread this: it reads the ceiling (h11) *before* stopping the anchors, saw sybil head already at 13, and its unscoped fresh-commit grep matched the stale 15:27 `committed block` lines → a false **"CAPTURE"** verdict. (Harness fixes for the detector are tracked separately in #402; this consult is only about the product mechanism.)

## Q-A — attributed: this is a FORK the anchor gate admits, not a capture. Confirmed by repro.

`ValidateCommit` (`core/chain/chain.go:1472-1481`) gates a wheels-engaged commit on **`AnchorQuorum` distinct qualified anchor attesters** (non-proposer). With `AnchorQuorum=1`, the honest side commits at the bare count quorum — proposer `a0` + attesters `a1,a2` — which **leaves `a3` free** (never signed honest-11, so under the #397 watermark it is not locked at that height). A competing block proposed by a sybil, attested by the other sybils **plus the one free anchor `a3`**, then satisfies the gate (1 anchor ≥ 1) and commits on the sybil replicas.

`TestForkPassesAnchorGateWithOneFreeAnchor402` (green) builds exactly this and confirms both halves:
- the one-free-anchor fork **passes** the gate, **and**
- the same sybil quorum with **zero** anchor attestation is **refused** (`ErrAnchorRequired`).

So **C2 holds** — a pure sybil quorum cannot capture — and the field verdict was a **fork**, not a capture. The residual is a **launch-phase fork-creation / liveness vector**: one non-participating anchor can co-sign a competitor at a height the honest side already committed.

**How the fork EXTENDS to 12/13 (the head-coupling finding):** `ValidateProposal` (`chain.go:1344`) requires `b.Prev == local head`, so an anchor only attests a block building on **its own** head. The fork reaches 12/13 only on replicas whose head is *on the fork*; the anchors kept head honest-11 (they persisted it), so the network **partitioned** — anchors on honest-11, sybils on the 11'→13' fork, with the free anchor `a3` supplying the one anchor signature each fork block needs (attesting from a head that had adopted the fork, or before it reorged back).

**Q-A1 — the fix direction, quantified (please rule).** `TestIntersectingAnchorQuorumClosesTheFork402` (green) shows **`AnchorQuorum=2` closes this exact fork**: the honest commit must then hold 2 non-proposer anchors (proposer + 2 = **3 anchors up, 1-fault-tolerant**), leaving `<2` free, below the fork's requirement. Generalizing: since each anchor signs at most one block per height (#397), two disjoint blocks cannot both reach `AnchorQuorum` distinct non-proposer anchors when **`2·AnchorQuorum > (#anchors − 1)`** — i.e. `AnchorQuorum ≥ ⌈#anchors / 2⌉`. Questions:
- Is `AnchorQuorum ≥ ⌈A/2⌉` (intersecting anchor sign-off) the right launch-phase rule, replacing the current single-anchor gate?
- Its cost is launch fault-tolerance: at A=4 it needs 3 anchors up to commit (tolerates 1 down) vs today's 2-of-4. Is that acceptable at M0, given immutable #4 is about *participation floor*, not anchor count, and the anchors are explicit time-boxed scaffolding? (My read: yes — a launch net that can *fork* under one stray anchor is worse than one that needs 3-of-4 anchors up.)
- Does this interact with the MATURING handoff (#389)? Post-handoff the anchor gate is off and the >⅔-weight quorum already intersects, so I read this as **launch-phase-only** — confirm.

## Q-B — hypothesis, NOT asserted: the 26-minute honest idle is fork-induced partition + B6 quiescence + #351, not a new consensus stall

What the evidence supports: after the fork, the anchors are partitioned onto honest-11 with **nothing to propose** — all bond registrations drained during warmup (renewal-due is height-based and the honest height was frozen), and every publish that *could* have driven a block failed on **#351 token-gather latency** (reachable, no error). That is the same **B6 quiescence** the flow-5 clincher fix already established (a reactive chain with no pending work does not advance), here *plus* a partition: the only party making blocks was the sybil fork, driven by its own renewal clocks.

What I have NOT confirmed, and want your read on:
- **Q-B1 — partition heal.** When an anchor SyncChains against a sybil and fetches the 11'→13' fork, does the certified bond-weighted fork-choice (#357 model B) correctly **keep** honest-11, or could the 3-block sybil fork (8×64 MiB = 512 MiB of sybil bond vs 4×64 MiB = 256 MiB anchor) be **heavier** and pull an anchor over — which would then reorg committed state (violating D-1 prefer-stall-to-reorg)? The anchors *did* persist honest-11, so it held this run, but is that guaranteed or luck of the weight arithmetic? This is the node-tier fork-choice question I did not build a repro for (I did not want to assert a heal mechanism I could not yet reproduce deterministically — flagging per build-immutable #7).
- **Q-B2 — does the Q-A fix moot Q-B?** If `AnchorQuorum ≥ ⌈A/2⌉` prevents the fork from ever committing, the partition never forms and Q-B's "stall" reduces to ordinary B6-idle under #351 (not a consensus concern). My working hypothesis is **yes — Q-A's fix is also Q-B's fix** — but I want your confirmation before I treat Q-B as closed.

## What I will NOT do without your certification

Ship any change to `AnchorQuorum`, the anchor-gate rule, or fork-choice. The two repros are green and stand as the attribution; the fix is a consensus-rule decision.

## What I will do regardless (harness-only, no product change — tracked in #402)

Fix the flow-5 detector (scope the fresh-commit grep `--since` the anchors-stop; treat a sybil head *above* the ceiling at flow start as a pre-existing-divergence finding, not a "lagging" note), raise capture depth, size the chaos-reprovide window to the measured re-announce latency (linear in held chunks), and widen the flow-6 evidence stash.

## Artifacts

`core/chain/fork_anchor_gate_402_test.go` (both repros, green) · `integration/cloudtest/{flow-evidence,console,publish-diag}-4faaee8-22913.log` · `integration/cloudtest/archive/{results,report}-4faaee8-22913.*`. Related: #397 (the certified fix this binary carries; its Q2a stall prediction is the neighbor of Q-B), #389 (mature weight quorum — the post-handoff intersecting rule this proposes to mirror for launch), #383 (the earlier flow-5 detector false-positive class).
