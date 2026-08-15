# MATURING starvation — attribution CORRECTED: anchor-side throughput wall, not #382, not (this-run) issuer-withholding

**Date:** 2026-08-15 (supersedes the attribution in `2026-08-15-maturing-run-blocked-by-382.md`)
**Run:** `9c3777d-73949` (MATURING=1 SYBILS=8)
**Trigger:** PE ruling `docs/reviews/maturing-run-382-scope-PE-RULING-2026-08-15.md` — "hold the red team; attribution first (#6)."
**Author:** builder. This entry records a double attribution correction — my learning trail, deliberately preserved.

## The trail (this is the point of the doc — how the attribution moved)

1. **My first call: "#382, already M1."** WRONG, and a textbook #6 miss: I symptom-matched
   (sybils + slow + publish-timeout) to a *filed* issue **whose dominant cause already shipped
   fixed** (#384/e2d89ab elided the SyncChain full-refetch). Attributing to a closed issue by
   symptom is exactly the trap.
2. **PE's push (H2): issuer-withholding — the token sibling of #402.** Code-grounded:
   `CanonicalIssuers` (chain.go:1062) builds the publish-token issuer set from every on-chain
   bonded validator, **not phase-gated** — so under MATURING the sybils enter it; if a publisher
   asks withholding sybils it can't reach k signatures. The PE assumed *equal* bonds → a
   sybil-dominated prefix.
3. **My code investigation overturned H2 as this-run's cause — with two facts:**
   - **The topology bonds are 64:1, not equal.** Anchors `-bond 64M`, sybils `-bond 1M`;
     `CanonicalIssuers` ranks bond-descending → the canonical *prefix is anchors*, sybils sit in
     the *tail*. So withholding-sybils only bite if the gather falls forward into the tail.
   - **The publisher never asks the sybils at all.** `ft_peers()` (scenarios.sh:17) returns only
     `role=='validator'` nodes = the **4 anchors** (topology tags the 8 sybils `role=='sybil'`);
     `rankByCanonical` (swarm.go:34) *intersects* the gather to the reachable `-peers`. Diag
     confirms: "4/4 of the -peers set reachable." **The gather set is the 4 honest anchors.**
     H2 (sybil withholding) is structurally *unreachable* in this topology.
4. **What actually starved it — the egress-independent smoking gun:** the console shows
   **`network did not warm within 600s`** — and the network warm-up is a *throwaway publish from
   val-a* (an anchor, on the swarm, gathering tokens from the other anchors). **No external
   egress, no remote issuer-key discovery.** An anchor could not get a publish to commit among
   anchors in 600s. Plus `publisher fetch-1 did not warm within 180s`. The bottleneck is
   **anchor-side, under the 12-participating-validator load.**

## The corrected attribution (this run): H1 — an anchor-side throughput/liveness wall

The publisher (and val-a itself) gathered from **honest, reachable anchors** and still couldn't
assemble a 2-of-4 publish-token quorum + commit within the windows. This is a **throughput/
latency residual**, not a protocol-logic defect and not a griefing seam. #382's *dominant* cause
is fixed, so this needs a **new name** — candidate mechanism: **bond-audit CPU + drain
serialization among 12 participating validators on e2-small** (12×11 bond challenges per 30s
`-bond-audit` cycle, each doing crypto, on a 2-vCPU burstable VM — starving the MsgTokenRequest /
commit handlers). To be confirmed by a load repro (below); not asserted as fact yet.

Note it is **not model-checkable as a protocol bug**: the gather logic is *correct* — given
responsive honest anchors it assembles fine. The defect is that the anchors aren't responsive
*under real CPU load*. So the discriminator is a **perf/load reproduction**, not a logic oracle.

## What H2 still is: a real LATENT M0 seam, worth fixing — but not this run's cause

The PE's structural point stands independent of this run: `CanonicalIssuers` is **not
phase-gated**, and a publisher that dials the *full* validator set (the comment at swarm.go:265
explicitly says connecting to the canonical set is the fully-private mode) *would* ask withholding
sybils. That is the token-issuance analogue of #402 (an un-vetted set doing a launch-phase trusted
job) and it fails the invariant map's phase-gating checklist. **It deserves the #402-sibling fix
(phase-gate the issuer set: launch = anchors, open to the bonded set post-maturity), which is
research-gated because it touches the D3 publisher-independence claim.** But it would NOT have
fixed this run — the harness never exercised it.

## Observability gap (a finding in its own right)

The discriminating diagnostics — per-issuer gather legs (`token gather leg [FAILED]` with
issuer/rank/elapsed_ms) and issuer-key fetches — are **LogDebug**, and the run ran `-log info`.
So slow-vs-silent-per-issuer and key-discovery success were **not captured**, which is *why* both
prior attributions were possible. Worse, the harness verdict text **auto-attributes every publish
failure to "#351 egress / issuer-set discovery over WAN"** — but val-a's warm-up needs no egress
and still failed, so that canned text is misleading. Fixes: promote the gather-leg + key-fetch
summary to info (or a dedicated token-diag), and stop the harness hard-coding #351 as the cause.

## Next steps (attribution-first, per #6 — laptop/deterministic before any field run)

1. **Confirm H1 with a load repro:** stand up the 12-participating-validator regime locally (sim
   or docker) with `-bond-audit 30s`, measure anchor-to-anchor token-gather + commit latency, and
   check whether it blows the window under CPU contention. Profile a single anchor under the audit
   load. This confirms/denies "bond-audit CPU + drain serialization" as the residual name.
2. **Close H2 as class-closure (research-gated):** build the withholding oracle the PE named
   (canonical issuer set = honest + withholding, publisher needs k) so the *latent* seam has a
   deterministic RED/GREEN home, and draft the phase-gate for `CanonicalIssuers` for research
   review. This is decoupled from unblocking this run.
3. **Fix the observability gap** so the next field run can discriminate directly.

## Scope call — maps to the PE's H1 conditional branch, pending PE concurrence

If the PE concurs with H1 (honest anchors, slow under load — the val-a evidence is the crux):
this is a **named M1 efficiency residual** (not #382), and the PE's H1 branch endorses **B + C** —
release the red team against the field-confirmed launch regime + the deterministic mature
certification (disclosed transparently as model-check-not-field for the mature corner), pull the
named residual to top-of-M1, and run an instrumented reduced-load MATURING to field-confirm the
handoff below the throughput wall. **I am NOT acting on this unilaterally** — I've corrected the
attribution twice now; the red-team release is high-stakes and I want the PE to confirm the H1
read (specifically the val-a-self-warm evidence) before we release. Red team stays on standby.
