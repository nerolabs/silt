# Pinning the durability-turnover GAP: the evidence says accept→commit, not discovery

**Date:** 2026-08-17 · **Trigger:** the PE state-of-HEAD assessment ranks the
run-82bcd2b `durability-turnover` GAP as the load-bearing residual ("mature-regime
publish reliability UNCONFIRMED — #351 or #441-residual; never presume which") and
prescribes pinning it deterministically on the now-timed model-check before any
field spend.

## The evidence (each citable)

- **E1** — `integration/cloudtest/console-82bcd2b-39478.log:28`: the captured
  client error is `httpregistry publish: accepted but not committed within 3m0s
  (root 4cc7de00…) — the consensus gather did not finish`. The publish token was
  gathered and the entry was **accepted** (the #441 submit-then-poll path); the
  failure is in **accept→commit**, not discovery.
- **E2** — same console: publisher→validator reachability 4/4; fetch-1 had
  re-warmed 62 s earlier (a throwaway publish **committed** ~2 min before the
  failing one, on the same matured network).
- **E3** — `flow-evidence-82bcd2b-39478.log:7835`: the GAP's evidence capture
  grabbed **store-1/store-2/fetch-1 only**. No validator journal covers the
  21:06–21:10 publish window — the run cannot show whether the chain was
  committing, whether the entry sat in a mempool, or whether it landed after the
  client quit.
- **E4** — `adapters/httpregistry/httpregistry.go:157`:
  `publishPollTimeout = 180 * time.Second`, its comment derived from the
  *genesis-era* gather. The #451 synchronizer durations re-derived every
  harness bound (`H_ESCAPE` 220 s, `PUBLISH_RETRY_S` 360 s — commit 6fbcf2e)
  but **not this client constant**: a single in-spec escape height (≤220 s) can
  outlast the client's entire poll window. This is the bound-re-derivation miss
  class the 2026-08-16/17 ledger already flagged once (#454→#459).
- **E5** — `core/node/entrypool.go:68-82`: `SubmitEntry` queues locally **only
  if the receiving node is proposer-eligible**, and the peer broadcasts are
  **fire-and-forget** (empty reply callback — no ack check, no retry).
  `adapters/httpregistry/httpregistry.go:337-359`: the client submits **once**
  and then only polls — the certified design's "dropped submission is re-sent by
  the client's retry loop" only fires as a whole fresh publish attempt (in the
  field: the harness's next `swarm add`, once per ~180 s).
- **E6** — `integration/cloudtest/topology.py:291-294`: val-a (the registry the
  client submits through) runs `-bond 64M`, and `chain.go:813` counts
  anchors-with-real-bond, so val-a is *likely* epoch-set-eligible post-latch —
  but if its peer broadcasts drop, the entry commits only when the accepting
  node itself is the designee: a rotation wait of up to |epochSet| heights.
- **E7** — `core/node/modelcheck_441_publish_starvation_test.go` is GREEN with
  the fix: under the WAN-observed contention schedule an entry submitted by an
  eligible node to **all** peers commits within 3 heights. Untimed, loss-free —
  it bounds heights, not time, and models perfect delivery.

## The candidate mechanisms

- **M1 — #351 issuer discovery: REFUTED for this run** by E1+E2. Discovery
  completed (token gathered, entry accepted, reachability 4/4). The sheet's
  "discovery #351 or starvation #441" disjunction was written without E1's
  decomposition.
- **M2 — #441 fold-starvation residual** (the mempool entry losing to drain
  under mature contention): disfavored by E7 but unquantified; needs the timed
  assertion under steady contention. If real → research consult (touches the
  certified round machinery).
- **M3 — under-derived client poll bound**: a **fact** (E4), defect regardless
  of the rest. An in-spec height can exceed the whole client window, so the
  client can manufacture a failure verdict for in-spec behavior — an S5
  honesty bug in the product's own mouth.
- **M4 — delivery fragility**: fire-and-forget submit broadcasts + eligibility-
  gated local queue + submit-once-then-poll client (E5) mean one WAN-dropped
  broadcast burst degrades commit latency to the accepting node's designee
  rotation wait (E6) — plausibly ≫ any client window.

M2/M3/M4 are not mutually exclusive; M3 is certain, M2 and M4's *magnitudes*
are what the deterministic tier must pin.

## Options weighed

- **(a) Timed model-check discrimination first (PE prescription)** — two
  laptop oracles on the existing `matureWorld12` + sim-clock machinery:
  **A** (steady contention, perfect delivery → entry must ride the next
  committed block; RED = M2 real) and **B** (delivery dropped to all but the
  accepting node → measure the rotation-wait bound; the number that sizes any
  honest client window). Cost: hours. Benefit: pins M2 and M4 deterministically,
  produces the derivation for M3's fix, gates the field spend per D-CONSENSUS.
- **(b) Fix M3 + rerun the field test** — rejected alone: a billable run spent
  to *test* a hypothesis (#7); and if M2 or M4 is real the rerun GAPs again
  with the same blind capture (E3).
- **(c) Research consult first** — not yet triggered: no consensus rule or
  security parameter moves in (a); the consult fires only if oracle A goes RED.

## Decision

Sequence **(a)**, then fix what the oracles justify, in one PR:

1. Oracle A (timed steady-state publish bound) — discriminates M2.
2. Oracle B (delivery-drop rotation wait) — quantifies M4.
3. Re-derive `publishPollTimeout` from the oracle numbers × the field per-height
   bound (derivation in the comment), and give the poll loop a periodic
   **re-submit** (mempool-dedup no-op by design — the certified drop-recovery
   lever actually firing within the window). Client-side liveness only; no
   consensus rule, no security parameter.
4. Harness: `ft_publish` failure captures the **validator** journals too (E3 is
   #7's canonical capture-the-evidence-first case recurring).

On the M1 flag (bounds must converge, not creep): widening the client's 180 s is
**not** the escape-window creep the PE warned about — it is the client's honesty
about already-shipped consensus durations. The pressure to *shrink* belongs on
the durations themselves (batching, #299), not on a client window that lies
below the in-spec worst case and manufactures failures.

The next MATURING field run then *confirms* (never discovers) — it was already
owed as the durability-turnover re-grade.

## Outcome (same session)

Both oracles GREEN on first run (`core/node/modelcheck_441_publish_bound_test.go`):

- **A** — under steady contention with delivery intact, the accepted entry rode
  the very next committed block, three consecutive publishes, drain work in every
  carrying block (anti-vacuity held). **M2 refuted at the model tier.**
- **B** — with the submit burst dropped, the entry committed after a measured
  **8-chain-height** rotation wait (h9→h17). At the field's 220 s/height escape
  bound that is ~30 min — no single-shot client poll can cover a lost burst.
  **M4 quantified: real, and client-recoverable.**

Shipped accordingly (no consensus rule touched; I1–I5 untouched):
`publishPollTimeout` 180→360 s with the derivation comment (converges with the
harness's `PUBLISH_RETRY_S` — one number, one derivation); the poll loop
re-submits every 30 s (failing-first regression
`TestAsyncPublish_ResubmitInsidePollWindowRecoversLostBurst` — RED against the
submit-once client, reproducing the field error string verbatim); `ft_publish`
failures now capture the validator cohort's journals with the verdict, and the
GAP text decomposes by the captured error. Full `go test ./...` green.
