# Silt integration & field tests

Silt's behavior is proven at three tiers: fast **unit tests** (`go test ./...`),
a deterministic **in-process sim** (`silt sim run …`), and — here — **integration
field tests** that exercise the real daemon over real processes, disk, sockets,
and (on GCP) real machines and real inter-region links.

There are **two substrates for the same properties**, and when you run a field
test you pick one:

| | **Local (Docker)** | **GCP (real machines)** |
|---|---|---|
| Where | `integration/<name>/` (this dir) | `integration/cloudtest/` |
| Shape | **per-test** harnesses, one topology each | **one combined** multi-machine acceptance run |
| Substrate | containers on **one host**, one Docker bridge | **real VMs across 3 regions**, real VPC/Cloud NAT |
| Speed / cost | seconds–minutes, free | minutes, a few cents (SPOT), auto-torn-down |
| Best for | fast iteration, CI gates, the NAT matrix, protocol-logic tests | real crashes/clocks, real WAN latency, real NAT, scale, the **RC gate** |
| Fidelity it adds | real processes/disk/sockets/TLS | **+** independent machines, inter-region internet, real hardware NAT |

Neither replaces the other. Local owns fast, cheap, deterministic per-test
coverage; GCP owns the things a single host physically cannot model. **Deciding
which to run is part of scoping a field test** — see "Choosing a substrate" below.

---

## Field-test immutables

Unit, integration, and e2e tests check that **mechanisms work**. Field tests are a
harsher instrument — the **ultimate judge of whether the whole thing actually
delivers**, with the GCP runs (real machines, real networks, real scale) as the
**gold standard**. These five are non-negotiable for every field test in this tree,
on the same footing as the project's design immutables. A test that violates one is
broken, however green it looks.

1. **Be cynical.** Assume it's broken and *try to make it fail*. Attack the
   property and measure what survives — never assert that a function ran or a log
   line merely appeared. (Retrieval doesn't check "the DHT walk returned"; it
   *pollutes* routing with identity churn and measures what fraction of a new
   user's cold fetches actually come back — and found 85%.)
2. **Test the outcome, not the mechanism.** Frame each test around what a real
   **user or adversary** experiences — *can I get my data back? does it stay alive?
   is it private? can consensus be captured?* — and gate it on a hard, honest
   threshold.
3. **Real evidence only.** Assert on real daemon/disk/socket/wire output — real
   `<store>/debug.log` lines, real SHA-256, real on-chain or observed state. Never
   a string the harness itself echoes, and never a pass by construction.
4. **Never fake green.** A property that falls short is a **FINDING** (reported with
   the real number + a reproducer; exit 0 like a known-defect reproducer); a
   regression is a **FAIL**; a capability with no live seam is *stated as a gap*,
   not faked. The failure **is** the deliverable.
5. **Two substrates, one standard.** Every property ships a **local** Docker suite
   (cheap, fast — catches most things before we spend) **and** a **GCP** flow (the
   real verdict — real crashes, clocks, WAN latency, NAT, scale). Where a laptop can
   only approximate (true scale, a real partition), say so and let the GCP flow be
   the one that decides.

Prototype and iterate locally; **certify on GCP.**

> **Extending or auditing these tests?** Read
> [`FIELD-TEST-STATUS.md`](FIELD-TEST-STATUS.md) first — the honest current state of
> every suite: genuine PASS vs reported FINDING vs stated gap, the scoping caveats
> (e.g. `sybil` #5 falls back to the standing gate on a laptop; `chaos` WAVE 2 is an
> unpinned observation; `demand` #6 has no seam yet), and what has actually been
> *run* vs only *dry-validated*.

---

## Quick start

**Local (free, ~15 min for the fast set):** you need Docker running + a Go toolchain.
```sh
./integration/run-all.sh            # the fast gate set → consolidated report + per-suite logs
FULL=1 ./integration/run-all.sh     # + the slow suites (soak, upgrade)
./integration/audit/run.sh          # or run any single suite on its own
```
`run-all.sh` prints a live pass/finding/fail summary, writes `integration/.run-all/report.md`,
and saves each suite's full output to `integration/.run-all/<suite>.log`. Exit 0 iff every
suite that ran is PASS or a deliberately-reproduced FINDING.

**GCP (real VMs, a few cents, auto-torn-down):** you need `gcloud`, `terraform`, Go, a
billing-enabled project.
```sh
cd integration/cloudtest
./cloudtest.sh setup     # interactive: asks for your project + walks you through auth
./cloudtest.sh           # build → provision → run flows → report → DESTROY
./cloudtest.sh nuke      # last-resort teardown by label, if anything is ever left
```
Teardown is guaranteed (destroy-on-exit + a TTL self-destruct on every VM); always confirm
`gcloud compute instances list --filter labels.cloudtest:*` is empty afterward.

---

## For an LLM/agent operator — filing the reports

The **scripts don't emit dated reports; you do.** When you (an agent) run a field test on
someone's behalf, run the harness, then **read the raw output and write the reports** so the
result is a durable, shareable artifact. Use today's date (`YYYY-MM-DD`).

**Local** — after `./integration/run-all.sh` (use `FULL=1` for full coverage), file:
- **`silt_local_fieldtest_<date>.md`** — the consolidated roll-up: one row per suite (PASS /
  FINDING / FAIL), the M0 claim it gates, runtime, and a one-line verdict; then a short
  "notable findings" section for anything that isn't a clean PASS. Base it on
  `integration/.run-all/report.md` plus your reading of the logs.
- **`silt_local_fieldtest_<suite>_<date>.md`** — one per suite: what it proves, the real
  evidence you saw (the asserted `debug.log` lines / SHA / chain-status, quoted from
  `integration/.run-all/<suite>.log`), the verdict, and — for a FINDING/FAIL — the failing
  assertion and the reproducer.

**GCP** — after `./integration/cloudtest/cloudtest.sh`, file the same shape from
`integration/cloudtest/report.md` + `results.jsonl`:
- **`silt_cloud_fieldtest_<date>.md`** (roll-up across the flows + the #184 drills), and
- **`silt_cloud_fieldtest_<flow>_<date>.md`** per flow (with the real over-the-wire evidence).

Report **honestly**: a FINDING (a deliberately-reproduced open defect, e.g. `upgrade` = #237)
is not a failure; a real FAIL is. Quote real evidence, never invent a passing string. If a
capability has no live seam yet, say so — a surfaced gap is a valid result.

---

## Choosing a substrate

Run it **local** when you're testing **protocol logic or a specific mechanism**,
iterating quickly, or gating a PR:
- consensus fork-choice, red-team accountability, bond cost, takedown, economy,
  audit, the NAT matrix (cone/symmetric/hole-punch), rolling-upgrade — all reach
  their verdict fine on one host, in minutes, for free.

Run it on **GCP** when the **answer depends on real machines or the real
network**, or you're cutting a release:
- real inter-region latency and independent clocks (consensus convergence under
  a *real* partition, not an app flag);
- real Cloud NAT (cone vs symmetric decided by an actual NAT gateway);
- **scale** — e.g. the repair-under-churn "coverage cliff" only resolves at
  50+ storage nodes, which a laptop can't host;
- long-haul **soak** on real hardware;
- the **RC gate**: a green multi-machine acceptance pass is required to advance a
  release candidate.

Rule of thumb: **prototype and gate locally; certify on GCP.**

---

## Local — per-test Docker harnesses

**Prereqs:** Docker + a Go toolchain. Each harness builds the `silt` binary on
the host (CGO off), bakes a slim image, stands up its topology, drives it,
asserts on real `<store>/debug.log` lines + SHA-256, and tears down (`trap …
docker compose down -v`).

**Run one:**
```sh
./integration/<name>/run.sh            # build → run → assert → tear down; exit 0 = PASS
KEEP=1 ./integration/<name>/run.sh     # leave the topology up to poke at
```

| harness | what it proves |
|---------|----------------|
| `nat/run.sh` | cross-NAT publish→fetch bit-perfect via the relay (`RESTART=1` adds #69 reprovide) |
| `nat/holepunch.sh` | cone NAT upgrades relay→direct (#27); `NAT_MODE=symmetric` falls back to the relay |
| `nat/loadtest.sh` | fetch-under-load: many concurrent fetches through a bandwidth-capped relay (#65) |
| `churn/run.sh` | repair-under-churn: kill holders, caretaker reconstructs + re-scatters, stays bit-perfect |
| `chaos/run.sh` | crash-recovery: `SIGKILL` every holder, restart, #69 re-announce fires, cold-fetch bit-perfect (`WAVES=2` probes a seed-crash discoverability gap) |
| `durability/run.sh` | durability under permanent loss: shrink the swarm (no replacement), caretaker re-scatters, content outlives the nodes |
| `consensus/run.sh` | objective on-chain-bond fork-choice: partition → heal to the heavier chain |
| `redteam/run.sh` | #184 accountability: equivocator slashed, forged block rejected, low-bond proposer refused |
| `sybil/run.sh` | C2 no quiet capture: a young objective network commits with the honest anchors and refuses to advance for a bonded Sybil set without them |
| `audit/run.sh` | a "liar" deletes data but keeps proofs → the loss is caught and repaired |
| `bond/run.sh` | proof-of-space-time bond cost (C1 no-discount): real plots seal expensive + verify cheap (plot-residency cost gate), one plot cannot back N identities (root-owner dedup), under-bonded proposals rejected. (Reputation *proportionality* — `reputation ∝ bond` — is not yet asserted; ROADMAP #7.) |
| `economy/run.sh` | blind-signed, publisher-unlinkable credits **over the wire**; per-byte earning / freeloader-broke via the in-process economy sim (no daemon credit seam yet — see the suite's FINDING 2) |
| `takedown/run.sh` | per-operator, existence-checked, reversible takedown |
| `privacy/run.sh` | publisher unlinkability: the default chain refuses a durable file→publisher link (refuse-to-surveil), the private path works, `-token-quorum` authorizes without identity |
| `client/run.sh` | web-UI path: publish→list→fetch bit-perfect over the daemon's HTTP API, and the local-security guard holds (no-token/wrong-token→401, DNS-rebinding/cross-origin→403) |
| `soak/run.sh` | sustained load + gentle churn: bit-perfect throughout, bounded memory/disk |
| `upgrade/run.sh` | rolling binary upgrade on persisted stores: reload + fetch bit-perfect |
| `retrieval/run.sh` | retrieval/discoverability at scale + ephemeral-identity churn: cold-fetch success-rate floor (#43) |

Each harness uses a distinct Docker network subnet, image tag, and compose
project, so they don't collide; run them one at a time (each assumes exclusive
use of its topology). Common env: `KEEP=1` to keep the topology up; per-harness
knobs (scale, file size, duration) are documented in each `run.sh` header.

**Run the whole suite** (serially, with per-test logs): see
`integration/run-all.sh` if present, or drive them in a loop.

---

## GCP — real multi-machine acceptance *(`integration/cloudtest/`)*

The cloud substrate — where GCP is the judge (field-test immutable #5). Local
Docker is the free, fast net above; this runs the same claims over real VMs, real
cloud networks, and real multi-region latency.

A **~13-node silt network across 3 regions** on real GCP VMs: 4 validators, 2
storage nodes, a registry, a relay, a fetcher, a NAT gateway + NATed nodes, and
an adversary — provisioned by Terraform (VPC, public + NAT subnets, firewall,
**SPOT** instances), each node booting its full `silt` argv from
`topology.py`-computed static IPs/NodeIDs. It runs the 9 acceptance flows + the
#184 drills over the real wire, writes `report.md` + `report.html`, and tears
everything down.

**Prereqs (once):**
- A **billing-enabled GCP project**; `gcloud auth login` done.
- APIs: `gcloud services enable compute.googleapis.com iap.googleapis.com storage.googleapis.com`.
- IAM on the project: `roles/compute.admin`, `roles/iap.tunnelResourceAccessor`,
  `roles/storage.admin` (**Owner** covers all three).
- Local tools: `terraform`, `gcloud`, `go`, `python3`, `curl`.

**Run it:**
```sh
cd integration/cloudtest
cp config.env.example config.env       # set PROJECT_ID (+ optional knobs)
./cloudtest.sh                          # build → terraform apply → run → report → DESTROY
```

**Cost & safety — cheap-first, and it will not leak resources:**
- SPOT instances; a hard **TTL self-destruct** (`shutdown -h +TTL_MINUTES`) so even
  a crashed orchestrator can't leave VMs running; **nuke-by-label**
  (`./cloudtest.sh nuke`); optional billing-budget alarm.
- Validate with **no spend** first (topology + `terraform validate`), then a
  **4-node SMOKE** run (pennies), then the full 13-node run.
- Iterate for free: bring it up once with `KEEP_UP=1`, re-run scenarios with
  `./cloudtest.sh run`, tear down when done.
- **Always** verify teardown: `gcloud compute instances list --filter labels.cloudtest:*`
  must be empty afterward.

See `integration/cloudtest/README.md` for the full topology, knobs, and the
`HANDOFF.md` first-run guide.

---

## Running a field test (the workflow)

1. **Scope** — what property are you testing, and does the answer depend on real
   machines/network/scale? Pick **local** (fast, per-test) or **GCP** (real,
   combined) accordingly.
2. **Run** — the chosen harness(es). Local is one command per test; GCP is one
   command for the whole acceptance pass.
3. **Collect** — local prints a `RESULT: PASS/FAIL` per test (capture the logs);
   GCP hands back `report.md` + `report.html`.
4. **On GCP, confirm teardown** — no VMs left running.

---

## Roadmap: per-substrate parity

Today the mapping is asymmetric — local is ten focused per-test harnesses; GCP is
one combined acceptance run. The direction is **parity**: factor a shared
node-abstraction (`exec-on-node` + `assert-on-log`, already the shape of both
`docker exec` locally and IAP-SSH on GCP) so the *same* scenario can target
either substrate, and add the GCP scenarios that only real hardware can answer —
**scale-out** repair-under-churn (50+ nodes), a **real firewall partition** for
consensus, **`tc` link shaping** for fetch-under-load, and long-haul **soak**.
The still-live parity/hardening items now live in ROADMAP.md's **Residual backlog**
("Field-test harness residuals"); the retired per-substrate backlog is archived at
`archive/FIELD-TEST-ROADMAP-2026-09-01.md`.
