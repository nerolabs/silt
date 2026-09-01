# silt GCP field test (`#52`)

Spin up a **real, multi-machine silt network** on Google Cloud in a realistic
topology, run a full acceptance pass over the real wire, generate a **shareable
report**, and tear the whole thing down — one command, cost-bounded.

This is the automated form of roadmap **#52** (the multi-machine field test) and
the **RC gate**: a thorough green pass here is required to advance a release
candidate. It is also meant to be run by **outside developers** who want to see
silt operate end-to-end and produce a report of the outcome — it runs against
*their own* GCP project from a clean clone, needs nothing from the authors, and
hands back `report.md` + `report.html`.

> **What this adds over `integration/nat/`** (the local Docker harness): real
> separate machines (independent clocks, real crashes/restarts), real
> inter-region internet latency, and real scale — the things a single-host
> loopback cannot model. The Docker harness still owns fast NAT-matrix testing;
> this complements it, it does not replace it.

## What it exercises

A ~13-node topology across three regions:

| role | count | what it proves |
|------|-------|----------------|
| validators (`val-a..d`) | 4 | earned objective standing, multi-validator convergence, f=1 fault tolerance, Byzantine safety |
| storage (`store-1/2`) | 2 | content scatter, serve, per-operator takedown, restart survival |
| registry | 1 | `-registry-only` role comes up and serves |
| relay | 1 | NAT fallback / hole-punch rendezvous |
| fetcher | 1 | publish → fetch bit-perfect from a different node |
| natgw + natted | 3 | real NAT (cone/symmetric) → cross-NAT file movement via the relay |
| adversary | 1 | the `#184` drills: equivocation→slash, forged/low-bond→reject |

Scenarios map 1:1 onto the acceptance brief (`docs/reviews/m0-acceptance-brief.md`,
flows 1–9) plus the `#184` adversarial consensus-safety cases. Each records a
`pass` / `gap` / `fail` verdict with a severity and elapsed time.

**Cloud variants of the local field-test series** (`integration/{privacy,durability,
chaos,client,sybil}`) run the same properties over real VMs / real regions, mapped
onto the existing 13-node topology with **no topology change**:

| cloud flow | mirrors | asserts |
|---|---|---|
| `flow_publisher_unlinkability` | `privacy` (#3) | a durable-`Publisher` publish is REFUSED by the default chain (refuse-to-surveil) |
| `flow_durability_turnover` | `durability` (#2) | content survives a **permanent** storage-node departure — fetched bit-perfect from a survivor |
| `flow_chaos_crash` | `chaos` (#7) | a **SIGKILL**ed storage node re-announces its chunks (#69) and content stays fetchable |
| `flow_web_ui_guard` | `client` (#4) | the web-UI guard holds on a real VM (no-token→401, DNS-rebinding→403, read→200) |
| `flow_c2_no_capture` | `sybil` (#5) | **opt-in** (`SYBILS=8`): a bonded non-anchor Sybil cohort cannot advance the chain with the anchors down, and it resumes when they return |

**C2-Sybil (#5) — opt-in, `SYBILS=8 ./cloudtest.sh`.** The local `integration/sybil`
suite can only reach the **standing gate** (a laptop's fresh Sybils can't *bank*
bonds — a young network's bond-registration needs anchor-proposed blocks). The
cloud opt-in adds a cohort of **non-anchor Sybil validator VMs** (a `sybil`
`topology.py` role: `-validator -objective`, equal `-bond`, one shared
`-domain sybilnet`, referencing the real anchor set they do **not** control). Over
the warm period the anchors' blocks *bank* the Sybil `BondReg`s, so the flow
certifies the **pure `ErrAnchorRequired` gate**: stop every anchor → a
self-majority of bonded Sybils **cannot** advance the chain (the C2 concentration
metric discounts a single-domain split, so they never reach the bond-distinct
maturity that sheds the anchors); restore the anchors → the chain **resumes**
(proving it was the anchors that were required, not that the Sybils were dead).
≥8 equal single-domain bonds also trip the **atomization note**. The Sybils run on
**SPOT** (cheap); absent the cohort (`SYBILS` unset) the flow records an honest
`skip`. `SYBILS=8 ./cloudtest.sh` adds the 8-VM cohort (21 nodes total) — **off by
default** so the standard run stays 13 nodes.

## How it works (deterministic, self-configuring)

`silt id -id-seed N` is deterministic and the internal IPs are static, so
**every peer / anchor / attester / relay reference is computed before any VM
exists** (`topology.py`). Each node boots with its complete `silt` argv baked
into instance metadata — no discovery wait, no post-apply reconfiguration.

```
topology.py    seeds → NodeIDs → static IPs → the full `silt` argv per node
terraform/     VPC, public + NAT subnets, firewall, SPOT instances, budget alarm
provision/     startup scripts: pull the binary from GCS, run the argv under systemd
lib.sh         SSH-over-IAP, log-wait, SLO assertions, result recording
scenarios.sh   the 9 flows + 3 #184 drills + 4 field-test-series cloud variants
gen_report.sh  results.jsonl → report.md + report.html
cloudtest.sh   the orchestrator: build → apply → run → report → destroy
```

## Prerequisites

- A **GCP project with billing enabled**, and `gcloud auth login` done.
- Enable the APIs once: `gcloud services enable compute.googleapis.com iap.googleapis.com storage.googleapis.com` (+ `cloudbilling.googleapis.com` if you set a budget).
- Your account needs `roles/compute.admin`, `roles/iap.tunnelResourceAccessor`, and `roles/storage.admin` on the project (project **Owner** covers all of it).
- Local tools: `terraform`, `gcloud`, `go`, `python3`, `curl`.

## Run it

```bash
cd integration/cloudtest
cp config.env.example config.env      # set PROJECT_ID (+ optional knobs)
./cloudtest.sh                        # build → apply → run → report → DESTROY
```

At the end you get `report.md` and `report.html`. That HTML file is the artifact
to share / attach to an RC checklist or a GitHub issue.

**Memory envelope (Phase 1.3).** Every run samples each node's cgroup memory
(`systemctl … MemoryCurrent`) every `MEM_SAMPLE_INTERVAL` seconds (default 30)
into `rss-<RUN_ID>.jsonl`, and records an `infra-node-memory` finding with the
per-node **peak / final** RSS — the measured envelope behind any "return-to-2GB"
memory claim (build-immutable #7: a headline needs a citable number, not just the
absence of a crash that `infra-node-liveness` already checks). The series is
git-ignored by default (like the console/flow logs); **force-commit the specific
`rss-<RUN_ID>.jsonl` for any run you cite as evidence** (`git add -f`), same
convention as the tracked console logs. Disable with `MEM_SAMPLE=0`. This is a
coarse envelope, not a profiler — for attribution pull an on-demand heap profile
(`DEBUG_PROFILE=1` at launch, then `./cloudtest.sh heap <node>`).

Other lifecycles:

```bash
SMOKE=1 ./cloudtest.sh   # cheapest 4-node run — validate the plumbing for pennies first
./cloudtest.sh up        # bring the network up and leave it (debugging / iterate on scenarios)
./cloudtest.sh run       # re-run the scenarios against an up network (no new spend)
./cloudtest.sh down      # terraform destroy
./cloudtest.sh nuke      # last resort: delete everything labelled cloudtest=<run>
```

**First time? Follow `HANDOFF.md`** — a step-by-step shakedown runbook (no-spend
validate → SMOKE → full run → confirm teardown) that a fresh session can drive with
you. `SMOKE=1` trims to 4 nodes so you validate apply/provision/SSH/publish before
paying for the full topology; scenarios that need absent nodes skip cleanly.

## Cost model — three independent guards

The default lifecycle **destroys on exit, even on error or Ctrl-C** (`trap … EXIT`).
On top of that:

1. **SPOT/preemptible instances** — cheapest tier; GCP may reclaim them, and they
   never survive 24h.
2. **Per-VM self-destruct** — every VM runs `shutdown -h +TTL_MINUTES` at boot, so
   even a crashed orchestrator cannot leave a VM running past the TTL (default 3h).
3. **Optional budget alarm** — set `BUDGET_AMOUNT_USD` + `BILLING_ACCOUNT` for a
   GCP billing-budget backstop.

If Terraform state is ever lost, `./cloudtest.sh nuke` deletes every resource by
its `cloudtest=<run_id>` label. A full run on `e2-small` nodes for ~30 minutes is
a few dollars; `faithful` bond mode (bigger disks, longer plot time) costs more.

## FAST vs FAITHFUL bonds (read this before trusting a green)

`BOND_MODE` controls what "earned standing" costs in the test:

- **`fast`** (default) — demo-sized bonds (`-bond 64M`, floor 0). Proves the
  **mechanism**: the objective consensus path, convergence, restart survival,
  NAT traversal, and the adversarial drills all run over real machines and real
  latency. It does **not** prove the C1 economic *magnitude*.
- **`faithful`** — real plotted bonds (`-bond 2G`, `-min-bond-floor 1G`). Bonds
  cost real disk + plot time, so standing is economically faithful. Use bigger
  `MACHINE_TYPE` + `BOOT_DISK_GB`. Slower and pricier; this is the mode for an
  economics-faithful RC gate.

A `fast` green means "the system works over the real wire"; a `faithful` green
additionally means "standing cost real resources". Don't conflate them.

## Operational notes

This harness has RUN for real on GCP: the RC run `585c82a-58990` graded
28 pass / 0 gap / 0 fail / 2 skip-by-design (#532, `eb57d50`), on the deep-run
lineage `fe2376a`-deep (30P/1G/0F). `topology.py` generates the real, deterministic
argv for every node (proven against `silt id`), all shell is syntax-checked, and the
cloud path — `terraform apply`, the SSH/journald log matching, and the quorum
arithmetic — is exercised by those graded runs. Two tuning points were resolved on
first contact and are noted here so a future operator knows where they live:

- **Quorum sizing for fault tolerance** — `-quorum` vs. the default-on
  `-byzantine-quorum` interplay for the validator count. The `6-fault-tolerance`
  scenario records the *observed* threshold as a `gap` (not a hard fail); the RC run
  pinned the number.
- **Log-match patterns** — `waitfor` regexes in `scenarios.sh` are matched
  against the daemon's `-log info` output; if a phrasing differs on the live
  build, the check reports `gap`, not a false `pass`.

Nothing here fails *silently*: an un-met SLO is always recorded in the report.
