# Retrieval / discoverability at scale — field test

**M0 claim under test:** the most basic user promise — *can I get my content back?* —
holds as the swarm grows and short-lived publisher/fetcher identities churn the DHT
(the #43 / #60 retrieval-degradation family).

## What it does

On one flat Docker network it stands up a seed/registry + a pool of **N holders**,
publishes a set of files (each from its own **ephemeral** client), optionally
**pollutes routing** with throwaway ephemeral publishes (the #43 churn pressure),
then **measures the bit-perfect fetch success rate** from fresh ephemeral clients
doing cold provider lookups — exactly a new user's path. Every `silt swarm
add`/`swarm get` joins with its own ephemeral identity (`SetEphemeral` — peers must
not route to it, #43), so each measured fetch is a genuinely new fetcher.

## Verdict

- **rate ≥ `FLOOR`% → PASS** — retrieval holds at scale; the #43 mitigations work.
- **rate < `FLOOR`% → FINDING** — real discoverability degradation at scale (the
  failure *is* the deliverable, a product finding to fix, not a harness bug). A
  reproduced FINDING exits 0 like `upgrade`; `EXPECT=pass` flips it to a hard fail.
- A publish that never returns a link, or the swarm not coming up, is **FAIL**.

## Run

```sh
./run.sh                            # 24 holders · 12 files · 40 churn identities · 60 fetches
HOLDERS=40 FETCHES=100 ./run.sh     # crank the scale
POLLUTERS=0 ./run.sh                # baseline: pure scale, no identity churn
FLOOR=95 ./run.sh                   # stricter success-rate gate
KEEP=1 ./run.sh                     # leave the swarm up to poke at
```

| knob | default | meaning |
|---|---|---|
| `HOLDERS` | 24 | storage pool size (the "scale") |
| `FILES` | 12 | distinct files published |
| `FILE_BYTES` | 1000000 | bytes per file |
| `FETCHES` | 60 | measured cold fetches from fresh clients |
| `POLLUTERS` | 40 | throwaway ephemeral publishes (routing churn, #43) |
| `FLOOR` | 90 | required bit-perfect success rate (%) |
| `FETCH_TIMEOUT` | 30 | per-fetch cap — a hung fetch is a fail, never a stall |

Isolate the variable: `POLLUTERS=0` measures raw scale; a high `POLLUTERS` with a
low `HOLDERS` measures churn pressure. On GCP this maps to a large multi-region
swarm where the coverage/discovery cliff is realistic — see `integration/cloudtest`.
