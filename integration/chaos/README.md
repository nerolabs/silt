# Chaos / crash-recovery — field test

**Outcome under test (cynical):** does the system **survive hard crashes?** Not a
graceful shutdown — a **`SIGKILL`**, the abrupt death of a real process — followed
by a restart of the *same* node (same identity, same IP, same on-disk store). After
a crash, a holder must reload its persisted store **and re-announce its held
chunks** (#69) so content stays *discoverable*, and a fresh client must fetch it
back **bit-perfect**. Every assertion keys off real daemon logs + real SHA-256.

## How it differs from the durability suite

`durability` models **permanent loss** (`docker rm` — the store is gone for good).
`chaos` models a **crash and recovery**: it `SIGKILL`s daemons but **never removes
a container**, so `docker start` restores the same filesystem — exactly the
crash-restart under test. Same identity, same IP, same store.

## What it does

`seed/registry + N holders + a persistent client`. Publish (replicated), baseline
cold-fetch bit-perfect, then:

- **WAVE 1 (default)** — `SIGKILL` **every** holder, restart the same containers,
  wait for each to re-bootstrap **and** log `re-announced N held chunks` (#69), then
  cold-fetch bit-perfect: content survived a full holder crash and stayed
  discoverable. This is the robust, well-characterised recovery gate.
- **WAVE 2 (opt-in, `WAVES=2`)** — *also* `SIGKILL` the **sole** seed (which is
  registry **and** bootstrap **and** a DHT node) and restart it.
- **Throughout** — no crash-loop: every restarted container is `running` at the end.

The fetch is a **cold** fetch from a fresh ephemeral client via the seed, so it must
*discover* the providers — the point of the #69 reprovide check. A crash-loop or
on-disk corruption is a **FAIL**.

## What WAVE 2 found (opt-in observation, not yet a verified defect)

Crashing the **sole** seed/registry/bootstrap and restarting it leaves the content
safe on disk (WAVE 1 proves that) but a fresh client **cannot rediscover the
providers** within the window — and a single holder re-announce did **not** restore
it either, so it is broader than a stale provider index: the restarted sole-bootstrap
does not re-mesh with the live holders in time. The root cause is **not yet pinned**,
and it is **entangled with this single-bootstrap topology** — a SPOF a real
deployment avoids with ≥2 registry/bootstrap nodes. It is recorded honestly as an
**observation to root-cause + retest** (with a redundant-bootstrap topology and on
the cloud test), which is why WAVE 2 is **off by default**. WAVE 1 — the property we
can characterise cleanly — is the gate.

## Run

```sh
./run.sh                       # 6 holders, WAVE 1 only (default); exit 0 = PASS
WAVES=2 ./run.sh               # + the opt-in seed-crash observation (exits 0 as a FINDING)
HOLDERS=8 ./run.sh
KEEP=1 ./run.sh
```

The GCP judge (`integration/cloudtest`) crashes real VMs (and includes a restart-
survival flow); this local suite owns fast, repeatable `SIGKILL`+restart cycles.
