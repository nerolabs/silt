# Durability under permanent holder loss — field test

**Outcome under test (cynical):** does content *outlive the nodes that held it*? Not
"does a repair fire once" — but, as holders **permanently depart and are never
replaced**, so the pool concentrates onto ever-fewer survivors, does the caretaker
keep every stripe reconstructable (re-scatter from parity faster than loss) and can
a user still **fetch it back, bit-perfect**? (D-S7, the finite-but-renewable
durability contract.)

## Why kill-*without*-replace

An earlier version killed a holder with `docker rm -f` and `--scale`d the pool back
up. Docker recycles a freed container IP to the next container it starts, so the
fresh **empty** replacement tended to inherit the dead holder's IP under a **new
identity**. The caretaker, dialing `old-NodeID@that-IP`, then hit an *impostor* (TLS
pin mismatch) — a Docker artifact real infrastructure does not produce (a terminated
VM's IP is not handed to a stranger seconds later). Those failed dials drowned the
repair sweep and masqueraded as "content lost."

So this local test **shrinks the swarm** — permanent loss with **no IP recycling** —
which isolates the real mechanic: reconstruct-from-parity (any `k=10` of `n=16`
columns) + re-scatter onto survivors. True membership **rotation** (fresh VMs =
genuinely fresh IPs) is the **cloud** test's job — `integration/cloudtest`, the
gold-standard judge (field-test immutable #5).

## Two oracles, kept separate

A durability test must not fail for a *retrieval* reason. So it reads two signals:

- **Durability (authoritative):** the caretaker logs `repair below k` only when it
  tried to reconstruct a stripe and could not gather even `k` shards — genuine,
  unrecoverable content loss. That, not a flaky fetch, ends the durability envelope.
- **Retrieval (the user outcome):** a fresh client `swarm get`, handed **every
  surviving holder as a direct peer** (warm discovery, so the probe is about the
  *bytes* not about re-walking a churn-polluted DHT), retried a few times.

A fetch that flakes while `below k` has *not* fired is transient/discovery noise:
recorded, but the shrink continues (durability is not breached). The run PASSES only
if a real end-to-end fetch after all the loss is bit-perfect — the outcome, not the
mechanism.

## What it does

`seed + N holders + a caretaker + a persistent client`. Publish at `REPLICATION`
copies/column, wait for the caretaker's first sweep, baseline-fetch bit-perfect,
then for `N − MIN_SURVIVORS` cycles: **`docker rm -f` a running holder** (store +
identity gone for good), **do not replace it**, block until the caretaker completes
a **fresh** sweep (so `reachable`/`below-k` are post-kill, never a stale line), and
fetch. Finish with an authoritative confirmation fetch.

## What it found — the durability envelope, and its boundary

Default run (`16 → 6` holders, `replication=1`, one file ~4 MB = 104 shards):

- **Durability HELD.** Across **10 permanent departures** no stripe ever fell below
  `k`. Redundancy erodes as within-slack losses accumulate (`reachable` 104 → 98 →
  91 → 85), then the caretaker reconstructs once a stripe crosses `RepairSlack` and
  re-scatters onto survivors, so `reachable` recovers (→ 99) — **13 stripe
  reconstructions** over the shrink. The content provably still exists, whole, on
  nodes that outlived the ones it was published to.
- **RETRIEVAL degraded at the small-swarm boundary.** A fresh client's fetch stayed
  bit-perfect down to **~11 survivors**, then began failing — *even handed every
  survivor as a direct peer*. The shards are reachable to the long-lived caretaker,
  but a fresh client cannot **discover** enough of the re-scattered shards to
  assemble the file as the swarm shrinks. This is the **#43 retrieval surface under
  permanent loss**: *durable is not the same as retrievable.*

So the default exits as a **FINDING** — durability held, but the end-to-end user
outcome degraded past the retrieval floor. The finding *is* the deliverable (real
evidence of the durability↔retrievability boundary, not a tuned-to-green pass). The
retrieval field test owns #43 head-on; the **cloud** test's larger, real-network
swarms are where the *healthy-retrieval* durability envelope is certified.

`EXPECT=pass` flips any finding to a hard failure (for CI gating on a config you
expect to pass, e.g. a retrieval-healthy `MIN_SURVIVORS`).

## Run

```sh
./run.sh                                 # 16 → 6 holders, replication=1 (default)
MIN_SURVIVORS=11 ./run.sh                # stay above the small-swarm retrieval floor → clean PASS
REPLICATION=3 ./run.sh                   # shipped-default margin (survives more before repair is forced)
HOLDERS=24 MIN_SURVIVORS=12 ./run.sh     # bigger swarm, more departures, retrieval stays healthy
KEEP=1 ./run.sh                          # leave the swarm up to poke at
```

| knob | default | meaning |
|---|---|---|
| `HOLDERS` | 16 | starting storage-pool size |
| `MIN_SURVIVORS` | 6 | shrink down to this many, then a final confirmation fetch |
| `CYCLE_WAIT` | 70 | seconds/cycle; a floor — the run also blocks on a *fresh* sweep (RepairInterval=60s) |
| `REPLICATION` | 1 | copies/column at publish; 1 ⇒ every departure strands columns (honest stress) |
| `FILE_BYTES` | 4000000 | published file size (multiple stripes) |
| `FETCH_RETRIES` | 3 | fetch attempts before a cycle is counted a transient miss |

GCP is the gold-standard judge here — a real long-haul run over independent
machines, real clocks, and true membership rotation is where the finite-but-
renewable contract is certified (`integration/cloudtest`).
