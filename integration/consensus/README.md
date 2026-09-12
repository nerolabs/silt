# Multi-validator consensus + partition-heal harness (Docker)

Field test #1. It stands up **four real `silt daemon -validator` processes** on a
flat Docker network, drives real publishes through consensus, severs the set
with a real network partition, and asserts the **M0 keystone claim**:

> **Objective, bond-weighted commit admission**: a block commits only on an
> intersecting super-quorum of a validator set the chain itself sizes (a strict
> anchor majority at launch, >⅔ of the epoch's frozen on-chain bond once standing
> is earned), so a sub-quorum partition commits nothing, stalls, and catches up to
> the majority's history on heal rather than reorging onto it.

> **⚠ CORRECTED 2026-09-12, AND THIS HARNESS HAS NOT BEEN RE-RUN.** The claim above
> replaces the earlier wording, in which the healed set converged onto whichever
> fork carried more bond.
> Fork choice ranks on height then head hash and has **no bond term**
> (`core/chain/chain.go` `heavier`), and with the finality gate on — every
> `objective() && ByzantineQuorum` posture — `Reconcile` admits only forks
> containing the committed head, so `dropped > 0` is structurally impossible.
> Source: research certification
> `README-BOND-FORKCHOICE-literal-claim-and-equivalence-RESEARCH-CERTIFICATION-2026-09-12`
> (finding R-4).
>
> **PROVENANCE — THE WORDING ABOVE IS CERTIFIED BUT NOT YET RATIFIED.** It is the
> certification's §7 sentence, and the owner has not approved it. It is used here
> anyway because what this file said before was measurably FALSE, and holding a
> false statement in place to wait for a ratification is the worse trade. Three
> shipped sites now carry this replacement wording — this file, `integration/run-all.sh`
> (suite catalog) and `integration/README.md` (suite row; the last two paraphrase
> rather than quote). The verbatim pin that would make a re-wording fail a build
> reads **`README.md` only** and is held with the published-claim half. **So if the
> owner ratifies different words, these three drift and nothing goes red.** Whoever
> ratifies should re-grep for this sentence, not just edit the front page.
>
> **The consequence for this document is in the "Result" section below and is not
> cosmetic:** P2 expects a 2-anchor "lighter group" to commit at height 1, and under
> `requiredLaunchAnchors = ⌊4/2⌋+1 = 3` a 2-anchor island cannot reach the launch
> anchor majority and should commit **nothing**. **Routed to the Tester:**
> `./integration/consensus/run.sh`. Predicted, NOT observed: P2 fails, or P3's reorg
> line is absent. Nobody has run it at this commit — do not read the Result section
> as current evidence.

This is the Docker-real, real-socket counterpart of the in-process
`e2e/partition_test.go` (`TestPartitionHealsToHeavierForkOverTCP`) — same
topology and flags, but over real kernel networking and container isolation, with
the partition driven by silt's own built-in test-harness flag `-block-peers`.

```
  flat network 10.50.0.0/24  (all four can route to each other)

    valA 10.50.0.11  registry ─┐  GROUP 1 (heavier): commits [g, a1, a2]  height 2
    valB 10.50.0.12  attester ─┘  — NOT partitioned; a healed valC reconnects here

    valC 10.50.0.13  registry ─┐  GROUP 2 (lighter): commits [g, c1]       height 1
    valD 10.50.0.14  attester ─┘  — carry -block-peers valA,valB → severed link
```

All four share the **same `-anchors` launch set**, so every replica starts from an
identical genesis config and objective fork-choice weight is computed the same way
everywhere. `run.sh` derives each NodeID from its `-id-seed` up front (via
`silt id`, run inside a throwaway container), so `-anchors` / `-attesters` /
`-block-peers` are all fillable before launch.

## Run it

```sh
./integration/consensus/run.sh          # build, test, tear down; exit 0 = PASS
KEEP=1 ./integration/consensus/run.sh    # leave it up afterward to poke at
```

Needs Docker and a Go toolchain. The `silt` binary is compiled **on the host**
(CGO off → trivial cross-compile to the container's arch) and copied into a slim
image, one image for all four roles — same approach as `integration/nat/`.

Isolation (shared machine): image tag `silt-consensus`, compose project
`consensus` (run from this dir), subnet `10.50.0.0/24`, host binary at
`integration/consensus/silt` (gitignored, rebuilt by `run.sh`).

## What each piece is

| file | role |
|------|------|
| `docker-compose.yml` | the topology: one flat network, 4 validators, named `/data` volumes so the chain persists across a recreate |
| `docker-compose.heal.yml` | heal override: recreate valC WITHOUT `-block-peers`, bootstrapped to valA — reload the persisted fork and reconcile |
| `Dockerfile` | slim runtime image carrying the prebuilt `silt` binary |
| `run.sh` | the driver: build → bring up → P0…P3 assertions → tear down |

## The assertions (each keys off a REAL observed CLI flag / log line / `chain-status` field)

- **P0 — negative control.** A lone objective validator with no bonded, qualified
  attester is asked to commit a publish. Assert **no `chain: committed block`**
  appears — the write path is *earned* standing, not a rubber-stamp (mirrors
  `examples/flow4-earned-standing.sh`).
- **P1 — convergence (pre-partition).** Group 1 commits a publish; assert its two
  replicas report an **identical `silt chain-status` head hash** (real command,
  real field: `head hash:`).
- **P2 — partition.** Assert the partition is in effect (`⚠ PARTITION: -block-peers
  set` on valC/valD — a real daemon stdout line), then that the two groups commit
  **different heads**: heavier group 1 reaches `chain: committed block 2`
  (height 2), lighter group 2 `chain: committed block 1` (height 1), distinct head
  hashes, and group 1 (which never blocked anyone) shows **no cross-partition
  reorg**. Honest sides forked; they did not diverge onto one conflicting head.
- **P3 — heal → converge.** Recreate valC WITHOUT `-block-peers`, bootstrapped to
  valA, on its **persisted `/data`**. It reloads `[g, c1]`, reconciles against group
  1's longer history, and its `chain-status` head becomes **identical to group 1's**
  (`a93…` at height 2). The head-hash equality is the ground truth and is
  posture-independent; the reorg narration is supporting evidence only.

## Result — SUPERSEDED, NOT RE-RUN

**Everything below is the run record from the pre-BFT posture.** It is retained
because it is the only field observation this harness has, and deleting it would
hide that the harness's expectations are the ones under question. It is **not**
evidence about the current rules, for the reason in the warning at the top: a
2-anchor island cannot reach a 3-anchor launch majority, so P2's "lighter group
commits at height 1" and P3's reorg line both describe behaviour the shipped rules
forbid. A companion change also retires the daemon's reorg narration (`chain:
reorged onto a heavier fork` -> `chain: adopted a competing fork`); once that
lands, the literal quoted below is not a string any binary emits. The two changes
are independent and may land in either order, so this note is written to be true
before and after.

*Historical record follows.* All four gates PASSED on real Docker (Desktop,
darwin/arm64), `run.sh` exited 0, under the rules of that day: the two groups
forked under the partition, and on heal the lighter side reloaded its persisted
fork from disk and reorged — narrated by the daemon of the time as, in one run:

```
valC on heal:
  chain: restored 2 block(s) from disk
  chain: reorged onto a heavier fork (dropped 1 block(s), new head height 2)
after heal — valA head: height=2 hash=88046cb2b9bbe1a8…
after heal — valC head: height=2 hash=88046cb2b9bbe1a8…   ← identical → converged
```

**No product deficiency was found *at that time*, against the claim as it was then
worded.** That finding does not carry: the claim has been corrected, and
`e2e/partition_test.go` now asserts the opposite phenomenology — a severed minority
that commits NOTHING and does a forward catch-up with `dropped = 0`. The absence of
a reorg line is the safety property there, not a gap.

## Notes for the builder (harness bugs found and fixed while building)

- **Persistent store is load-bearing for the heal.** A first cut mounted no volume,
  so recreating valC gave it a fresh empty `/data`; it then *caught up* into the
  heavier fork without a reorg (nothing to drop), and the `reorged onto a heavier
  fork` line never fired — which looked like a product finding but was purely the
  harness losing the persisted `[g,c1]`. Fixed with named `/data` volumes so the
  heal is a genuine reload-and-reconcile; the reorg line then fires as expected.
  P3 still keys its PASS on the ground-truth `chain-status` head-hash equality and
  treats the reorg line as supporting evidence (robust to either code path).
- `^`-anchored log regexes never matched `docker compose logs`' `svc | ` line
  prefix — fixed (match `peer: <hex>@` unanchored).
- The `silt` binary is built for linux (the container), so `silt id` is run inside
  a throwaway container, not on the host.
- Single-project isolation: run only one invocation of `run.sh` at a time — its
  cleanup trap does `compose down -v` on the shared `consensus` project, so an
  overlapping second run can tear down the first's containers.

## Poking at a running topology (`KEEP=1`)

```sh
KEEP=1 ./integration/consensus/run.sh
cd integration/consensus
docker compose ps
docker compose exec valA silt chain-status -store /data     # head height + hash
docker compose logs valC | grep -E 'reorg|caught up|committed'
docker compose -f docker-compose.yml -f docker-compose.heal.yml down -v   # tear down
```
