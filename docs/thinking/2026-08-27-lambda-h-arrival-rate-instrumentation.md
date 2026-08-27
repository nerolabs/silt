# λ_H arrival-rate instrumentation — the measurement CT-1 is owed

**Date:** 2026-08-27
**Seat:** Builder
**Class:** OBSERVABILITY over the committed ledger. NOT a consensus-rule, validity-predicate,
or security-parameter change. Reads; never changes what the chain accepts.
**Certification this serves:**
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27.md`

---

## 1. What CT-1 owes, in one paragraph

CT-1 lifts C-1 to CERTIFIED-CONDITIONAL: maturity provably precedes capture under an
honest-arrival floor `λ_H > 0` (measured), an adversary budget cap `W_A` (declared), and the
parameter constraint P2 (`M_req > W_A / (2·w_min)`). §6 of the cert names exactly one owed
input silt does not already hold in code: **the live honest-arrival rate `λ_H` at launch**,
plus a floor-exit alarm ("if the live rate drops below the floor the launch was certified
against, the operator is outside the hypothesis and must NOT treat CT-1 as holding — surface
it"). The instrumentation to READ the distinctness state already ships (`C2Metric` over
`c.bonded`); what is owed is RECORDING the arrival-rate and alarming on a floor exit. §6:
"The measurement does not touch consensus rules. It parameterizes the certification, not the
code."

## 2. The λ_H definition — pinned to the shipped shed metric

The cert (§2.1, H) defines the honest-arrival count `A(t)` as **operator-distinct,
domain-distinct bonded provision committed to the ledger by height `t`, where distinctness is
measured by the SHIPPED metric `min(NakamotoOperators, NakamotoDomains)`** (`chain.go:1835–1839`).
This is the exact quantity the maturity shed gates on. So:

- **A(t) := `min(C2Metric().NakamotoOperators, C2Metric().NakamotoDomains)`** — the committed,
  operator-and-domain-distinct bonded-distinctness count at height `t`. This is `k` in
  `matureNow()`. Reusing it (not a second definition) is load-bearing: λ_H must count the SAME
  distinctness the shed fires on, or the floor would parameterize a different quantity than the
  theorem binds (`T_mature ≤ M_req / λ_H`, cert eq. (1)).

- **λ_H over a window `[t₀, t₁]` := `(A(t₁) − A(t₀)) / (t₁ − t₀)`** — operator/domain-distinct
  bonded arrivals per block-height. A rate, in distinct-arrivals per height. The cert's H is a
  floor arrival rate `E[A(t)] ≥ λ_H · t`; the live measurement is the realized slope of `A` over
  a trailing window.

Why a windowed slope and not an instantaneous value: `A(t)` is a step function (integer count);
its instantaneous derivative is 0 almost everywhere and a spike at each arrival. The cert's λ_H
is a *floor RATE over the bootstrap window* (§2.1: "a worst-case lower envelope on honest
arrival"). A trailing window of `W` heights gives the realized average arrival rate — the
comparable quantity. `W` is configurable; the floor is compared against this windowed rate.

Note the metric is monotone-non-decreasing ONLY in expectation, not per-height: a lapsed (TTL)
bond drops out of `C2Metric` (`chain.go:1924`, "a lapsed bond drops out and the coefficient can
fall"). So `A(t₁) − A(t₀)` can be negative (net attrition). λ_H measured this way is a NET
arrival rate. A net rate at or below zero over the window is precisely the floor-exit the cert
wants surfaced (arrivals stalled / reversed → `T_mature → ∞`, theorem vacuous).

## 3. Where the state lives — daemon observer, not the chain (DECISION)

**Decision: the Chain stays a pure reader; the windowing state lives in the daemon's OnCommit
observer, beside the existing C2 alarm.**

Options weighed:

- **Option A — window state in `Chain`.** Add a ring of (height, A) samples to the `Chain`
  struct, update on commit. Cost: puts mutable observability state INTO the consensus object;
  it would have to be reconstructed on reload/reorg to stay a pure function of committed blocks
  (the #572 replay-divergence shape); risks looking load-bearing to a future reader. Rejected —
  it buys nothing and adds a reconstruction obligation to a hot, consensus-critical struct.

- **Option B — pure accessor on `Chain`, window in the daemon (CHOSEN).** Add one pure method
  `MatureCoefficient()` returning `A(t) = min(NakamotoOperators, NakamotoDomains)` — the SAME
  value `matureNow()` computes, extracted so the two cannot drift. The daemon's OnCommit
  callback (which already holds the C2 alarm and prints per-block narration) keeps a small
  trailing ring of `(height, A)` and computes λ_H = ΔA/Δheight over the window. This matches the
  existing C2 alarm exactly: ephemeral observability state in the observer, pure read from the
  chain. The measurement cannot touch a validity predicate because the chain exposes only a
  getter.

Option B is simpler and is the only one that respects the hard boundary structurally: the chain
gains a getter, nothing more.

## 4. The observable + alarm design

Following the exact convention of the C2 line and CONCENTRATION ALARM already in
`daemon.go` OnCommit (narration to the process log via `fmt.Printf`, which the operator
redirects to `silt.log`):

- **Per-commit λ_H line** (objective mode only, like the C2 line — legacy mode has no on-chain
  bonded set): once the trailing window has filled, print the measured λ_H (distinct arrivals
  per height) alongside the current `A` and the window width. Before the window fills, the line
  states it is still filling. This is the RECORDING §6 asks for.

- **Floor-exit alarm.** A configurable floor `-lambda-h-floor` (distinct arrivals per height,
  float; default 0 = disabled, so existing deployments and sims are unaffected and the operator
  opts in with the value they certified `W_A`/`M_req` against). When the measured λ_H falls
  BELOW the floor AND the network has not yet matured (`!EverMature()` — after the one-way latch
  the arrival floor is moot; P4 makes post-maturity concentration harmless), print a LOUD
  `⚠ λ_H FLOOR-EXIT` marker: the launch has left CT-1's hypothesis H; the operator must not
  treat maturity-precedes-capture as holding. This is the §6/§271 owed alarm.

Why gate the alarm on `!EverMature()`: CT-1 only needs to order the PRE-maturity window (cert
P4, §3.3). After the latch trips, a low arrival rate cannot re-arm anchors and does not threaten
the ordering. Alarming post-maturity would be noise. The per-commit λ_H LINE still prints
regardless (it is a measurement); only the ALARM is gated.

## 5. The hard boundary — proof it holds

- `MatureCoefficient()` is a getter returning the value `matureNow()` already computes. Extracting
  the inline `min` into a method and calling it from both sites changes NO value the shed sees —
  the regression test asserts `matureNow`'s decision is byte-identical before/after (behavior
  pinned by the existing maturity tests).
- The daemon change is print-only, inside a callback that already only prints. No block is
  accepted or rejected differently.
- The config flag defaults to 0 (disabled): no sim, e2e, or field topology changes behavior
  unless an operator sets the floor. It parameterizes a certification, per §6 — not the code.
- Touches no I1–I5 invariant, no validity predicate, no security parameter. If it did, this
  would STOP and route to research. It does not: it reads the committed metric and prints.

## 6. Tests

- **Unit (core/chain):** `MatureCoefficient()` returns `min(NakamotoOperators, NakamotoDomains)`
  for a constructed bonded set, and equals the value `matureNow()` gates on (guard against the
  two definitions drifting).
- **Unit (cmd/silt):** the λ_H window helper — given a sequence of (height, A) samples, it
  computes ΔA/Δheight over the trailing window; the floor-exit predicate fires when the rate is
  below the floor and stays quiet at/above it. ABLATION: inject a stalled-arrival sequence
  (A flat or falling) and watch the alarm predicate go true; a healthy climbing sequence keeps
  it false. A green alarm test with no demonstrated red is a comment that compiles (session-7
  lesson).

## 7. CHANGELOG

One line under Added: the λ_H arrival-rate observable + floor-exit alarm, citing the CT-1 cert.
Rebase against main / regenerate HTML if other PRs land first (never hand-edit the generated
HTML).
