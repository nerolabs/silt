# Rolling-upgrade / on-disk-format-stability harness (Docker)

Field test **#11**. Proves — or disproves — that a swarm survives a **binary
upgrade** with its persisted state intact. Two real `silt` binaries are baked
into one image: an **OLD** release (V1) and **HEAD** (V2). The daemons start on
V1 against named-volume stores, publish a real erasure-coded file and commit a
chain block, then the SAME daemons are recreated on V2 against the SAME stores.
The upgrade is a container *recreate* with a different `SILT_VERSION`, reattached
to the unchanged `/data` volume — exactly a binary swap on a persisted store.

```
        ┌──────────── swarm 10.130.0.0/24 ────────────┐
  seed 10.130.0.10  — registry + relay + validator (chain) — seed-data vol
  holderA 10.130.0.11 — EC holder, publishes            — holderA-data vol
  holderB 10.130.0.12 — EC holder                        — holderB-data vol
```

## Run it

```sh
./integration/upgrade/run.sh              # whole-swarm upgrade V1→V2
ROLLING=1 ./integration/upgrade/run.sh    # rolling: upgrade nodes one at a time
KEEP=1 ./integration/upgrade/run.sh       # leave the topology up to poke at
V1_REF=<gitref> ./run.sh                  # choose the OLD version (default v0.1.1)
EXPECT=clean V1_REF=<post-#70/#98 ref> ./run.sh   # demand a clean pass
```

Needs Docker + a Go toolchain. Both binaries are compiled **on the host** (CGO
off → trivial cross-compile) and COPYed into a slim image; neither is committed
(`.gitignore`). V2 is built from the worktree HEAD; V1 is `git archive`-d from
`V1_REF` into a temp tree and built there, so the branch stays on HEAD.

## What each piece is

| file | role |
|------|------|
| `docker-compose.yml` | seed + 2 holders, static IPs, **named volumes** so `/data` survives a recreate; per-service `${*_VER}` selects the binary |
| `Dockerfile` | slim image carrying BOTH `silt-v1` and `silt-v2` |
| `node.sh` | version-selecting entrypoint: assembles the right daemon flags per `SILT_VERSION` (V1 predates `-advertise`/`-mdns`/`-log`) and reattaches to `/data` |
| `run.sh` | driver: build both → up on V1 → publish + control fetch → recreate on V2 on the same stores → assert reload lines + a bit-perfect fetch |

Every assertion keys off a **real** observed line/field — the daemon's
`reloaded storage proofs count=N`, `re-announced N held chunks`,
`chain: restored N block(s) from disk`, `chain replay: …`, `bootstrapped`, real
`silt chain-status`, and SHA-256 bit-perfect equality. No invented strings.

## Versions (V1 / V2)

- **V2 = HEAD** (`origin/main`).
- **V1 = `v0.1.1`** by default — an early tagged release, ~180 commits back.
  Chosen deliberately: it **predates** two on-disk-format changes, so it exposes
  the upgrade path:
  - **#70 / `66454c1`** — persisted storage proofs (`<store>/proofs`). Before it,
    a daemon holds shards on disk but keeps their proofs only in memory.
  - **#98 / `dcfa4f3`** — versioned Block schema (`BlockVersion = 1`), a
    hard-fork guard that **refuses** to decode a block minted before versioning.

## Ground-truth results

Run on `V1 = v0.1.1 (01a6369)` → `V2 = HEAD (4b79842)`, real Docker:

**RESULT: FINDING ⚠ — the v0.1.1→HEAD upgrade is NOT state-preserving.**

- **Content stranded.** The 77 EC shards published on V1 stay on disk across the
  upgrade (`objects: 77 → 77`, data not lost), but `/data/proofs` is empty
  (v0.1.1 wrote none), so V2 logs **no** `reloaded storage proofs` line and can't
  re-key the shards to their placement/column keys. Even the holder that
  physically holds every shard fails to serve the file:
  `get: stripe 0: 10 data shard(s) lost … no shards available at all`.
- **Chain rejected.** V2 refuses the V1-written chain outright —
  `chain replay: chain: unsupported block version: block 0 got 0, want 1` — and
  `silt chain-status` surfaces the same. The daemon then silently reseeds a fresh
  genesis, discarding the V1 chain history.

### Control — HEAD's own upgrade path IS clean

`EXPECT=clean V1_REF=447000c ./run.sh` (V1 = a commit **after** #70 and #98,
43 commits back) → **RESULT: PASS ✅**:

```
  V1 on-disk: holderA objects=77 proofs=64 ; seed chain.cbor bytes=293
  holderA: reloaded storage proofs count=76 ; re-announced 77 held chunks
  seed:    chain: restored 1 block(s) from disk
  post-upgrade self-fetch: bit-perfect ; chain-status: blocks 1 (incl. genesis)
```

So the failure is **specific to upgrading across the #70/#98 format boundary**,
not a harness artifact and not a HEAD restart-durability bug: HEAD reloads its
own persisted proofs and versioned chain cleanly. (An all-V2 publish →
V2-recreate on the same store likewise re-announces its held chunks and self-
fetches bit-perfect — HEAD's own restart durability is intact.)

## Interpreting the finding

This is expected, *designed* behavior at the code level — #98's `BlockVersion`
guard is meant to hard-fork, and #70 added a store layout the old binary never
wrote. The finding is about the **upgrade experience**, not a logic bug:

- there is **no migration path** from a pre-#70/#98 store to HEAD — old content
  is silently stranded (shards on disk, unusable) and the old chain is silently
  discarded (reseeded genesis), with only a one-line stderr notice;
- a real operator upgrading a long-lived v0.1.x node would **lose access to all
  previously-hosted content and their chain history** with no warning that rises
  above a log line and no tool to migrate.

**Recommendation for the builder:** either (a) write a one-time migration that
reconstructs `<store>/proofs` from the on-disk shards + registry manifests on
first V2 boot (the proof is derivable — it's a Merkle inclusion path), and a
chain re-mint/accept path for legacy blocks; or (b) if a hard cutover is
intended, make it **loud and safe** — detect a pre-format store on boot and
refuse to start with an explicit "this store predates format X; run `silt
migrate` or start fresh" rather than silently stranding data and reseeding.

The harness is written so that once a migration exists (or `V1_REF` is a
post-format release), `EXPECT=clean` turns it into a green regression gate.

## Notes / limitations

- The tiny 3-node DHT is unreliable for a **remote** fetcher's provider
  discovery (a small-swarm weakness unrelated to the upgrade), so the content
  assertion drives `swarm get` **from the holder that holds the file** — a
  bit-perfect result there proves the store reloaded and the shards are
  addressable; a decode failure with shards still on disk proves stranding.
- The seed self-commits its chain with `-quorum=0 -min-rep=0` (a trusted
  one-box swarm) purely so a single seed can commit a real, persisted block to
  reload across the upgrade; it is not a consensus test (see `integration/consensus/`).
- NodeIDs are derived once with the V2 binary (`silt id`); V1 has no `silt id`
  subcommand, but ID derivation is identical across versions (verified: the V1
  daemon's logged `peer:` id for `-id-seed=1` matches `silt id -id-seed 1`).
