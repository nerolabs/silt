# Long-horizon durability under membership turnover — field test

**Outcome under test (cynical):** does content *outlive the nodes*? Not "does a
repair fire once" — but, as holders **permanently depart** and fresh **empty**
holders replace them so the swarm's membership fully rotates (size held constant),
does a fetch stay bit-perfect and does redundancy **recover** after each departure?
(D-S7, the finite-but-renewable durability contract.)

## What it does

`seed + N holders + a caretaker`. Publish at `REPLICATION` copies/column, then for
`CYCLES` cycles: **`docker rm -f` a running holder** (its store/identity gone for
good), **re-scale so a fresh empty holder joins** (real turnover, not a restart),
let the caretaker repair, then fetch bit-perfect and read the caretaker's
`repair sweep complete … reachable=N` (the #235 observability) to watch redundancy
heal. Run `CYCLES ≥ HOLDERS` for a full membership rotation — the content that
remains is then held entirely by nodes that joined *after* it was published.

## What it found — the durability envelope

- **`REPLICATION=1` (default, the honest stress):** each permanent departure strands
  columns with no second copy. The caretaker repairs only *past* `RepairSlack`, so
  within-slack column losses accumulate **un-repaired** — redundancy erodes
  (`reachable` 104 → 97 → 91, `repairs=0`) until a stripe crosses below `k` and a
  fetch fails. **FINDING:** at minimal replication, slow turnover slowly kills
  content; the caretaker does not proactively rebuild within-slack losses.
- **`REPLICATION=3` (the shipped default):** the replication margin absorbs a single
  departure, so content survives the rotation. Run it to see the healthy envelope.

A fetch failing is a **FINDING** (content did not survive turnover — reported with
the departure count + reachable trend + the caretaker's repair log; exits 0 like a
known-defect reproducer, `EXPECT=pass` flips it to a hard fail). The failure *is*
the deliverable.

## Run

```sh
./run.sh                                # 12 holders, 14 cycles, replication=1 (the stress)
REPLICATION=3 CYCLES=14 ./run.sh        # the shipped-default margin envelope
HOLDERS=20 CYCLE_WAIT=90 ./run.sh       # bigger swarm, gentler cadence
KEEP=1 ./run.sh
```

| knob | default | meaning |
|---|---|---|
| `HOLDERS` | 12 | pool size, held constant across turnover |
| `CYCLES` | 14 | permanent departures to drive (≥ HOLDERS ⇒ full rotation) |
| `CYCLE_WAIT` | 70 | seconds/cycle for a repair sweep (RepairInterval=60s) to land |
| `REPLICATION` | 1 | copies/column; 1 = every departure strands columns (honest stress) |
| `FILE_BYTES` | 4000000 | published file size |

GCP is the gold-standard judge here — a real long-haul run over independent machines
with real clocks is where the finite-but-renewable contract is truly certified
(`integration/fieldtest`).
