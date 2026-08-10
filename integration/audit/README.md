# Proof-of-retrieval (PoR) audit field test — the real "liar" node (Docker)

An automated, over-the-wire test of the storage-honesty story from
`silt sim run audit`. It stands up a **real multi-daemon swarm in containers** —
a registry/bootstrap seed, four honest holders, one **PoR liar** (`-liar`:
keeps its proof tags and advertises as a provider, but drops the bytes), and a
caretaker that runs both the repair loop and a **verify-without-fetch PoR audit
sweep** (`-audit`). It publishes an erasure-coded file (`-replication 2`) and
asserts, against real container logs, that:

- the caretaker's **audit** catches the liar *without fetching its bytes*
  (`FAILED≥1`, slashed) while the honest holders pass (`passed≥1`) — the literal
  sim claim, now wire-driven (#232); and
- after an honest holder's bytes are `rm`'d on top of the liar, the file still
  **survives bit-perfect** (the gated durability outcome), with the caretaker's
  detect/repair activity printed as observability.

```
       seed (registry + bootstrap, 10.60.0.10, -capacity 1 = holds nothing)
         |
  holder1 .21  holder2 .22  holder3 .23  holder5 .25   (honest holders)
       holder4 .24  (-liar: keeps proofs, drops bytes)
                    \        |        /
                     caretaker .30  (-care <careLink> -audit 15s, -log info)
```

Every daemon binds `0.0.0.0` but **self-advertises its own container IP** via
`entry.sh` — a wildcard bind is never advertised on its own (see
`cmd/silt/daemon.go` `-advertise`), so on a flat Docker network each daemon must
stamp its real dialable address itself.

## Run it

```sh
./integration/audit/run.sh          # build, test, tear down; exit 0 = PASS
KEEP=1 ./integration/audit/run.sh   # leave it up afterward to poke at
SWEEP_WAIT=90 ./integration/audit/run.sh   # widen the repair-sweep wait on a slow host
```

Needs Docker and a Go toolchain. The `silt` binary is compiled **on the host**
(CGO off → trivial cross-compile) and copied into a slim image, so the image
stays tiny and there's no Go-build memory spike inside Docker. Takes ~2–3 min:
most of it is one ~80 s wait for a caretaker repair sweep (the repair interval
is 60 s and is not a daemon flag).

## What each piece is

| file | role |
|------|------|
| `docker-compose.yml` | the topology: one `swarm` network, seed, 4 honest holders, 1 `-liar`, caretaker |
| `Dockerfile` | slim runtime image (silt binary), one image for every role |
| `entry.sh` | daemon entrypoint: discovers the container IP, stamps `-advertise` |
| `run.sh` | the driver: build → publish → audit-catch → damage → assert durability → tear down |

## What it asserts (all keyed to real observed behavior)

1. **Placement** — publish (`-replication 2`) scatters shards across the four
   honest holders; the liar advertises but drops its bytes; the seed
   (`-capacity 1`) holds none.
2. **Positive control** — before any damage the caretaker has logged **no**
   `stripe repaired`, and the file is retrievable bit-perfect from the intact
   swarm.
3. **PoR audit catch (GATED, #232)** — the caretaker's `-audit` sweep challenges
   every shard's providers and grades the proofs against the care-link key with
   **no ground-truth fetch**: honest holders `passed≥1`, the liar `FAILED≥1` and
   is slashed. The liar's `MsgHasChunk` lie fools the availability probe but not
   the audit.
4. **The attack** — `rm -rf /data/objects/*` on one honest holder (holder2) on
   top of the liar, while it keeps running (it still holds its persisted proofs —
   "keep the receipt, ditch the goods" — but the bytes are gone).
5. **Durability (GATED, the outcome)** — the file is still bit-perfect after the
   liar + honest loss. The caretaker's `stripe degraded / stripe repaired /
   repair below k` counts and the re-replication onto holder2 are printed as
   **observability** (placement decides which shards truly go missing, so exact
   counts vary; a wedged repair loop is still visible), not a flaky gate.
6. **Regression gate** — `-liar` and `-audit` still exist on `silt daemon`, so
   the #232 assertions above can't be silently skipped by a future build.

Why `-replication 2` + one honest wipe? With two copies of every shard, the
byte-dropping liar never *solely* holds a shard, so it cannot threaten
durability — it is purely an accountability target for the audit. Wiping one
honest holder on top of it then exercises real honest-loss recovery while three
honest survivors keep every stripe far above `k` (no flaky below-`k` stripes).
Active reconstruction is placement-dependent, so the harness gates the durability
**outcome** and merely observes the repair mechanism.

## Findings

### F1 — the literal PoR-audit / standing-slash claim IS now reachable over the real daemon (RESOLVED, #232)

The sim's headline is a **verify-without-fetch** PoR challenge: an auditor
holding the care-link key catches a liar that kept the proof but dropped the
bytes, *without fetching ground truth*, and **slashes its standing** into debt
(`sim/audit.go`, `core/node/por.go` `Node.Audit`, `core/por`).

This test *used* to record F1 as an open gap: over the real daemon that path was
not drivable — `Node.SetLiar` and `Node.Audit` existed and were unit/sim-tested,
but nothing in `cmd/silt` toggled the liar or invoked the sweep, so a liar was
caught only **indirectly** (once its bytes were gone it answered
`MsgHasChunk=false`, the caretaker's availability probe saw the shard vanish, and
repair re-scattered).

**#232 closed the gap.** `cmd/silt daemon` now exposes two seams (siblings to the
consensus red-team flags `-equivocate` / `-forge-block` / `-lowbond-propose`):

- **`-liar`** toggles `Node.SetLiar` — the storage node keeps its proof tags and
  advertises as a provider but drops the bytes, and answers `MsgChallenge` with a
  proof over data it no longer holds.
- **`-audit <interval>`** makes a `-care`-ing caretaker run `Node.Audit` on every
  cared root every interval: challenge each shard's providers and grade their
  proofs against the key derived from the care link — no ground-truth fetch —
  settling rent for the honest and **slashing** the liar (`RecordAudit`).

So the harness now GATES the literal claim: an intact-swarm audit sweep reports
`passed≥1` (honest holders) **and** `FAILED≥1` (the liar caught *without fetch*
and slashed). This also demonstrates *why* the audit is needed — the liar's
`MsgHasChunk` lie ("of course I have it") FOOLS the availability probe, but the
verify-without-fetch audit is un-foolable.

**Repro:** `silt daemon -h` now lists `-liar` and `-audit`; `run.sh` step 4
asserts the catch + slash directly, and step 6 guards that the flags still exist
(so the assertions can't be silently skipped).

### F2 — a healthy repair sweep emits no narration, even at `-log debug` (confidence: medium)

Only the degraded/repaired/below-k branches log; a sweep that finds everything
within slack writes nothing at any level. That is defensible (no news is good
news), but it means "is the caretaker actually sweeping?" is unobservable from
logs until something is wrong — worth a single `-log debug` "sweep complete"
line for field diagnosis. `run.sh` works around it by damaging enough to force a
logged branch.

## Poking at a running topology (`KEEP=1`)

```sh
KEEP=1 ./integration/audit/run.sh
cd integration/audit
docker compose exec caretaker sh -c 'grep -E "stripe (degraded|repaired)|repair below k" /data/debug.log'
docker compose exec holder2 sh -c 'find /data/objects -type f | wc -l'   # re-scattered shards
docker compose down -v
```
