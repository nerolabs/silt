# Field-test status — honest handoff (2026-08-10)

For a team **assessing coverage against the immutables and extending the local +
cloud field tests**. This is the *honest* current state — what each suite really
proves, where it is scoped or caveated, what is a genuine PASS vs a reported
FINDING vs a stated gap, and what has actually been *run* vs only *dry-validated*.
Read it alongside `integration/README.md` (the five field-test immutables + the
suite table) and each suite's own `README.md`.

**Guiding principle for extending these:** don't fake green. A property with no
live seam is *stated as a gap*, not tested; a property that falls short is a
*FINDING* (exit 0, with the real number + a reproducer), not hidden; GCP is the
gold-standard judge. If you tighten a test, keep its evidence real (a real daemon
log line / SHA / chain-status field), never a string the harness echoes.

## Local Docker suites — per-property honest status

| suite | verdict today | what it really proves | read-this caveat |
|---|---|---|---|
| `retrieval` (#1) | **FINDING** | cold-fetch success-rate under ephemeral-identity churn | *reproduces* the real #43 degradation (100% raw → ~85% with 40 polluters). The finding **is** the deliverable. |
| `durability` (#2) | **FINDING** by default | content outlives permanent holder loss; caretaker reconstructs from parity + re-scatters | Two oracles: `below k` (durability) vs warm-peer fetch (retrieval). Durability **holds** (no stripe below k; the bytes physically survive on the ≥k survivors). The default FINDING is now root-caused (issue **#277**): under heavy permanent loss on a small swarm the caretaker's repair sweep AND a fresh fetch **drown in a dial-storm** to the departed holders' stale provider records (the DHT walk re-dials dead holders the `deadUntil` cache doesn't gate) — so repair + retrieval degrade even though ≥k shards survive. Reported as a FINDING with the dial-storm diagnostic (a genuine below-k → FAIL; `EXPECT=pass` hard-fails). Heavily small-swarm-amplified; the cloud test judges the true envelope. |
| `privacy` (#3) | **PASS** | publisher unlinkability: the default chain refuses a durable `Publisher` entry (`ErrPublisherEntry` over the wire); token-quorum commits unlinkably | Only the **on-chain linkage** is tested. The **metadata-correlation** layer (an on-path observer correlating publish traffic by timing/volume) is a **stated M0 tradeoff — NOT tested and not claimed**. |
| `client` (#4) | **PASS** | the web-UI HTTP path (publish→roots→fetch bit-perfect) + the #89 guard (no-token/wrong-token→401, DNS-rebinding/cross-origin→403, read→200) | Driven from *inside* the daemon container (the local-`Host` guard requires it). Real HTTP status codes + SHA. |
| `sybil` (#5) | **PASS (scoped)** | C2 no-quiet-capture: a young objective net commits with the anchors (`wheels engaged`) and refuses to advance for a bonded Sybil set without them | **BIGGEST CAVEAT.** On a laptop the Sybils' bonds **don't bank on-chain** (a young network's bond-registration needs anchor-proposed blocks — chicken-and-egg for fresh Sybils), so the **standing gate** fires, not the pure **`ErrAnchorRequired`** gate. Both refuse the capture (the outcome is real), but the *pure anchor-co-sign gate* and the **≥8-bond atomization note** are deferred to the cloud (see #5 extension below). |
| `chaos` (#7) | **PASS** (WAVE 1); **opt-in FINDING** (WAVE 2) | crash-recovery: `SIGKILL` every holder, restart, #69 re-announce fires, cold-fetch bit-perfect | WAVE 1 is the robust gate (default). **WAVE 2 (`WAVES=2`) is an OBSERVATION, not a verified defect:** crashing the *sole* seed/registry/bootstrap breaks discovery, and a holder re-announce did **not** restore it — so the initial "stale provider index" hypothesis was **disproven**, the root cause is **unpinned**, and it's entangled with the single-bootstrap SPOF a real deployment avoids. Off by default. Needs root-causing + a redundant-bootstrap retest. WAVE 1's PASS text now reports the measured reprovide fraction (the gate is ≥1 reprovide + a bit-perfect cold fetch), not "every holder." |
| `churn` | **FINDING** by design (README: "expected to fail against current silt") | repair-under-churn: kill holders, caretaker reconstructs from parity + re-scatters, stays bit-perfect | **Exit-code caveat:** a characterized shortfall (small-swarm coverage cliff / dial-storm) should surface as `RESULT: FINDING` (exit 0), but the harness currently exits non-zero → the roll-up scores it **FAIL**, indistinguishable from a real regression. Split the exit like `chaos`/`durability` (see FIELD-TEST-ROADMAP #6). The repair core itself is genuine (real hash-verified `stripe repaired` + ephemeral-client bit-perfect refetch). |

**Also on main (prior sessions):** `consensus`, `redteam` (#184 accountability),
`bond`, `economy`, `audit`, `takedown`, `nat` (+ hole-punch), `soak`, `upgrade`
(#237 reproducer). `economy` already covers the wire-testable demand outcome
("hosts earn per byte, freeloaders go broke"). Known soft spot from the prior
acceptance pass: `bond`'s "reputation ∝ bond" (C1) is now **automated** — a
plot-SIZE gate parses `-bond` to bytes and requires the on-disk plot to be ≥ 90%
of it (a near-empty/instant plot fails C1), replacing the hand-recorded check.

## Not built — stated gap

- **`demand` (#6) — deliberately NOT built.** The P2 fair-exchange floor + P3
  cost-to-wash / bonded-fetcher credential live in `core/demand` (unit-tested) but
  have **no live daemon-wire seam**: no `silt sim run demand`, no CLI flag, no
  fetch-path enforcement. A "field test" here would just re-run core unit tests
  (violates the real-daemon/wire ethos), so per immutable #4 it is a **stated
  gap**, tracked in **issue #264** (wire demand into the daemon fetch path + add a
  demand sim scenario → then add `integration/demand`).

## Cloud (`integration/cloudtest`) — DRY-VALIDATED ONLY, never run

**Nothing here has touched real GCP. Zero spend so far.** The cloud harness
(Terraform + a deterministic 13-node, 3-region topology + acceptance flows 1–9 +
the #184 drills) plus **four new cloud variants** of the local series were added and
**dry-validated at the shell/topology level only** (`bash -n`, no `apply`):

| cloud flow | mirrors | status |
|---|---|---|
| `flow_publisher_unlinkability` | privacy #3 | authored, dry-validated, **not run** |
| `flow_durability_turnover` | durability #2 | authored, dry-validated, **not run** |
| `flow_chaos_crash` | chaos #7 (SIGKILL + #69, via `Restart=on-failure`) | authored, dry-validated, **not run** |
| `flow_web_ui_guard` | client #4 (guard over a real VM) | authored, dry-validated, **not run** |
| `flow_c2_no_capture` | sybil #5 (**opt-in** `SYBILS=8`: non-anchor Sybil cohort) | authored, dry-validated (topology + `terraform validate`), **not run** |

- **These flows have never executed against a live network** — expect first-run
  bugs (log-regex drift, timing/SLO tuning, publisher→validator egress for
  token-quorum). Treat the first GCP run as a shakedown. `cloudtest.sh` is
  cost-bounded (SPOT + per-VM TTL self-destruct + destroy-on-exit + nuke-by-label);
  `SMOKE=1` trims to ~4 nodes for a pennies-scale plumbing check first.
- **C2-Sybil (#5) now HAS a cloud flow** (`flow_c2_no_capture`), **opt-in** via
  `SYBILS=8` (adds a non-anchor Sybil validator cohort to `topology.py`). Off by
  default (the standard run stays 13 nodes); the flow records a `skip` when the
  cohort is absent. Dry-validated only — a real GCP run is where the pure anchor
  gate is certified. See item 2 below.

## Highest-value extension opportunities (ranked)

1. **Run the cloud harness for real** — the billable GCP pass is the *actual*
   remaining gate. The 4 new flows + the existing 9 + #184 drills need a first
   shakedown (SMOKE=1 → full). This is where the durability *retrieval floor*, the
   pure Sybil *anchor gate*, and real multi-region timing get their true verdict.
2. **#5 C2-Sybil cloud flow — BUILT (opt-in), needs a real run.** A `sybil` role
   (`-validator -objective`, equal `-bond`, one shared `-domain sybilnet`,
   referencing the real anchor set it does NOT control) is now in `topology.py`,
   opt-in via `SYBILS=8` (off by default; adds the cohort on SPOT). `flow_c2_no_capture`
   banks the Sybil bonds during warmup, stops every anchor (Sybil self-majority
   cannot advance the chain — `ErrAnchorRequired`), then restores the anchors (chain
   resumes — the clincher). ≥8 equal single-domain bonds trip the **atomization
   note**. Dry-validated (topology gen + `terraform validate`); the **real GCP run**
   is what certifies the pure anchor gate the laptop can only scope down to the
   standing gate.
3. **Root-cause the chaos WAVE 2 observation** — does a *redundant* bootstrap (≥2
   registry/seed nodes) survive one crashing? Is it provider-record persistence, or
   the restarted sole-bootstrap failing to re-mesh with live holders? Pin it, then
   either fix + assert, or downgrade to a documented topology limitation.
4. **Wire demand (#264)** so #6 becomes a real field test (P2 fair-exchange abort
   ⇒ token reusable; P3 wash ⇒ one bonded identity + a real fee per unit of demand).
5. ~~Automate `bond`'s C1 "reputation ∝ bond"~~ — **DONE** (plot-size gate; see
   the suite table's `bond` caveat).

## What to trust

Every local suite's assertions trace to **real** daemon `debug.log`/stdout lines,
real SHA-256, or real `chain-status` fields — no invented strings, no silt-core Go
modified by a harness, safe trapped teardown. The **scoping caveats above are the
honest boundary of what the local substrate can show**; the cloud pass is what
closes them. If something reads as "too green," check it against the caveat column
before trusting it.
