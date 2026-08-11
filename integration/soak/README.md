# Soak / scale integration harness (Docker)

Field test #10. A longer-duration, sustained-load run over **many real
daemons**, built to catch what a single-shot test never sees: **memory leaks**
and **slow drift**. It stands up a seed/registry, a pool of holder daemons and
a caretaker on one flat network, publishes a batch of multi-MB files, then
keeps continuous fetch traffic going for a fixed DURATION — asserting **every**
fetch across the whole run is bit-perfect — while sampling memory and store
growth to surface leaks or unbounded growth.

```
              flat bridge net 10.120.0.0/24
   seed(+registry) 10.120.0.10   ── holds content, hosts the registry
   holder01..12    10.120.0.2x   ── the replica / erasure substrate
   caretaker       10.120.0.40   ── an extra holder (name is legacy; no -care
                                    link wired, so no active repair loop runs)
   client (ephemeral)            ── fresh, storage-less fetch vantage each time
```

Every daemon binds a wildcard and self-advertises its container IP
(`node.sh` → `-advertise=<ip>:4001`), so it is dialable on the flat net — a
wildcard bind alone is not. The seed boots first; `run.sh` reads its NodeID
from the `peer:` log line, pins it to the seed's static IP, and bootstraps the
pool against it.

## Run it

```sh
./integration/soak/run.sh                 # ~6-min soak, gentle churn, tear down
DURATION=1800 ./integration/soak/run.sh   # 30-minute soak
CHURN=0 ./integration/soak/run.sh         # pure steady-state fetch load, no churn
FILES=16 FILE_BYTES=8000000 ./integration/soak/run.sh
KEEP=1 ./integration/soak/run.sh          # leave it up to poke at
```

exit 0 = PASS (clean bill of health with growth numbers); non-zero = a FAIL or
a FINDING. Needs Docker and a Go toolchain. The `silt` binary is compiled **on
the host** (CGO off → trivial cross-compile) and copied into a slim image, so
the image stays tiny and there's no ~1 GB Go-build memory spike in Docker.

### Knobs (env)

| var | default | meaning |
|-----|---------|---------|
| `DURATION` | `360` | seconds of sustained fetch load |
| `FILES` | `12` | files to publish |
| `FILE_BYTES` | `4000000` | bytes per file (~4 MB) |
| `FETCH_INTERVAL` | `2` | seconds between fetch bursts |
| `CHURN` | `1` | `1` = gentle within-margin churn, `0` = off |
| `CHURN_INTERVAL` | `45` | seconds between churn events |
| `CAPACITY` | `4G` | per-daemon storage pledge |
| `KEEP` | `0` | `1` leaves the topology up |

## What it asserts

- **Bit-perfect throughout.** A single baseline fetch of each link is the
  control; then for DURATION the ephemeral client repeatedly fetches random
  links and every one must equal its recorded SHA-256. Any mismatch fails.
- **No crash-loop.** Container `RestartCount` is sampled at start and end; any
  restart is a finding.
- **Bounded memory.** `docker stats` RSS is sampled at start / mid / end and
  reduced to a **mean per live daemon** — not the raw sum. Churn stops one
  holder at a time, so the running-container count dips 14→13 mid-run; a raw sum
  would swing with that count independently of any leak (false FINDING on the
  dip, or a real per-daemon leak hidden behind the shrunk count). The mean is
  invariant to the churn swing. Monotonic growth start<mid<end that is also >2x
  start **or** >+50 MiB/daemon is flagged a potential leak; bounded monotonic
  growth is noted, not failed.
- **Bounded object store.** Total on-store object count is sampled at start /
  mid / end. Once the published files replicate, it should reach a steady state;
  a monotonic start<mid<end climb that also >2x the start count is flagged as a
  potential disk/object leak (FINDING, not FAIL).

It also **reports** the deltas — mean memory per daemon (MiB), summed store
object counts, and restarts — so a clean run's growth numbers are the deliverable.

## Gentle, within-margin churn

silt replicates content 3x and erasure-codes it k=10/n=16, so losing one of a
dozen holders never strands a column. The churn loop honors that margin
strictly: it stops **exactly one** holder at a time (never the seed or
caretaker), then **restarts it on its persisted store** on the next cycle so
redundancy fully recovers. This measures steady-state health under normal node
comings-and-goings — not catastrophic loss. `CHURN=0` disables it entirely.

## What each piece is

| file | role |
|------|------|
| `docker-compose.yml` | topology: flat net, seed, 12 holders, caretaker, ephemeral client |
| `Dockerfile` | slim runtime image (silt binary + procps), one image for all roles |
| `node.sh` | entrypoint: self-discovers container IP, appends `-advertise`, execs the daemon |
| `run.sh` | the driver: build → bring up → publish → soak+assert+sample → report → tear down |

## Poking at a running topology (`KEEP=1`)

```sh
KEEP=1 DURATION=60 ./integration/soak/run.sh
docker compose -p soak ps
docker compose -p soak logs -f caretaker
docker stats $(docker compose -p soak ps -q)
docker compose -p soak down -v            # tear down when done
```
