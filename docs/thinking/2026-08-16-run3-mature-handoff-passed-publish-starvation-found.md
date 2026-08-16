# 2026-08-16 — run 3 (a56ac10-42834): the handoff PASSES on the wire; the mature steady state starves the PUBLISH path

The ONE follow-up instrumented MATURING run (serial queue item 3), launched detached from
main @ a56ac10 (`MATURING=1 SYBILS=8 LOG_LEVEL=debug LOOP_BUDGET=1`, all on-demand,
double-fork per the discipline — note darwin has no setsid(1); the working detach is a
subshell-orphaned nohup). Result: **18 pass / 3 gap / 2 fail / 1 skip.**

## What the run settled (the goals it was launched for)

1. **Flow 10 formal grade: PASS — the first field-exercised young→mature handoff.**
   Latch tripped at h26 (`wheels shed permanently`, nakamoto 2, **260 MiB across 8 — the
   whole cohort seated**, so the #439 seated-wait never even engaged); commits crossed
   the epoch boundary into the governed mature snapshot (h52→57); no anchor-required
   refusal post-shed. **10b capture drill PASS** (the 4 MinBond members alone could not
   advance the mature chain; resumed once honest weight returned — post-shed capture is
   weight-priced). **10c PASS** (WS cold-sync under the latch; F-1 held across restart).
2. **C2 evidence on the wire** end-to-end via the new regs= commit lines.
3. **strand-(a) re-framed by direct capture** (below) — it was never a discovery
   problem this run.

## The headline finding: ZERO publish entries commit after the latch

Census over every captured journal (readable only because #438's `regs=`/`entries=`
line shipped): entry-carrying blocks committed at h4, h5, h12, h15, h23 — **all
pre-latch** — and **not one of the 52 post-latch blocks (h26→h59+) carries an entry**;
they are all reg-only/empty. The chain is LIVE (renewal drains commit every height via
r1 new-views) but the **product path — publish — is systematically starved in the
mature steady state.**

The capture that broke it open (taken live from the still-up network after the
harness's own error capture came back empty): a fetch-1 warm attempt's client error —

```
silt: httpregistry publish: propose height 45 round 1:
  chain: insufficient valid attestations: 2 prepares of 2 gathered
```

Decoded against `chainrole.go:838`: the count floor (2) was met but `supportMet`
(the >⅔ mature WEIGHT) failed with the attester list exhausted — **at round 1**, i.e.
the publish proposal lost even the escape round. Correlations:

- fetch-1 warmed in **19 s pre-latch** and then failed every warm attempt (40+ over two
  240 s windows) **post-latch** — the "intermittency" is regime-correlated, not random.
- Every steady-state height burns r0 and commits at r1 (h57's full cycle in the
  captured evidence: drain blocked at own slot → round-change 03:31:56 → new-view
  03:32:12 → prepare-QC prepares=8 03:32:23 → commit 03:32:29 — **~95 s/height**).

**Leading mechanism candidate (code-cited, NOT yet proven):** the #432 view-change
machinery is **drain-only** — `recordRoundChange` (rounds.go) "fires the drain
proposal at that round" when the designated (h, r) proposer's quorum forms. The
publish path (`httpregistry publish` → `proposeBlock`) has no round-aware retry and no
new-view seat: at steady state there are ALWAYS pending renewals arming the drain, so
each height's rounds are owned by drain proposals, attesters' `(h, r, prepare)` slots
are taken by the time the publish prepare arrives (never-sign-twice → refusal → weight
short), and the publish loses every round of every height. Pre-latch it worked because
anchor-only proposing + lighter contention left r0 free often enough.

**Deterministic home:** the node-level mature-epoch fixture (PR #440's `matureWorld`)
was built for exactly this — the next schedule there is: registry-anchor publish
proposal vs an armed drain designee at mature steady state; RED = publish starves
forever, GREEN = whatever fix ships. **Research/PE consult required before any fix**
(consensus-adjacent: proposal scheduling/fairness touches the certified round
machinery). Issue filed.

## 10a-stall-drill FAIL — re-attributed (two confounds + the real finding)

The drill: stop the 4 sybils, ONE `mh_drive_block`, 90 s window. It reads "the honest
>⅔-weight coalition still commits" but actually measures "does a **publish-driven**
commit land within 90 s":

1. `mh_drive_block` IS a publish (`swarm add` via val-a) — the starved path above; its
   "successes" during the handoff drive were riding drain commits of the same window.
2. **90 s < the computed per-height escape bound** (~154 s: 2 sweeps × 30 s + gather;
   the observed steady-state cycle is ~95–155 s even healthy).
3. With sybils down, heights whose drain designee is a down sybil add the staggered
   takeover ladder (3+dist sweeps) before anything can commit.

The property itself (honest coalition can commit with the cohort declining) was
demonstrated minutes later by **10b's resume phase** (chain advanced past h58 once
honest weight returned, sybils still down at first). So: 10a = drill fidelity (window
below the computed bound + publish-path dependence) **compounded by** the real
publish-starvation finding. Fix the drill to (a) wait ≥ the computed escape bound and
(b) accept any honest commit (drain included) as the stall-refutation — then it grades
the intended B2 property.

## chaos-fetch FAIL — honestly UNATTRIBUTED (the harness discarded the evidence)

`swarm get` on store-1 produced no file (`got=<none>`) for a pre-latch link after
store-2's SIGKILL+recovery (reprovide PASSED at 217 s *before* the fetch). The get's
stderr goes to `/dev/null` in the flow, so the deciding client error was **discarded**
— the third instance of the same harness pattern this run (`wait_publisher_warm`
drops `$out`; `ft_publish` logged "last silt error: <none captured>"). No attribution
is possible from what was kept; anything more is a guess (#7). Fix the harness first,
attribute on the next run.

## Harness obs fixes queued (fold into the item-4 PR)

1. `wait_publisher_warm`: keep and print the LAST failed attempt's client output.
2. `ft_publish`: actually capture the silt client error it claims to capture.
3. `flow_chaos_crash`'s get: capture stderr on failure.
4. `10a`: window ≥ computed escape bound; stall-refutation = any honest commit.
5. The durability-turnover GAP text hard-codes "#351 issuer-set discovery" — this
   run's capture proves the failure can be post-latch quorum starvation instead;
   soften to "publish subsystem degraded (see captured client error)".

## Verdict on the PE gate sequence

Flow-10/B2/C2/WS: field-confirmed (10a pending the drill fix). The red-team release
remains gated on: (1) the launch-topology publish/drain soak (item 4, next run);
(2) **the new publish-starvation finding** — a mature-regime product-liveness break
that a red team would hit immediately; it needs its deterministic repro + a
research-consulted fix before #183 ships against the mature regime.
