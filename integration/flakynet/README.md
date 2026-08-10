# Flaky-network durability field test

**Outcome under test (cynical):** does silt bootstrap consensus, commit, and serve
a bit-perfect fetch **under a degraded network** — latency, jitter, packet loss —
the way a real P2P node on the public internet actually runs? A clean localhost is
the *easy* case; this is the honest one.

## Why this suite exists

Every clean local test (`e2e`, `integration/consensus`) commits in seconds. But the
first real multi-region GCP runs mostly **failed to bootstrap consensus at all**
(chain stuck at height 0), and the only variable was the *flakiness* of a real
network. This suite reproduces that adversity **locally and deterministically** with
`tc netem`, so durability weaknesses can be found and fixed on a laptop in minutes
instead of chasing them on GCP.

It stands up **4 objective validators** with the exact params that failed on cloud
(`-quorum 2`, Byzantine-quorum default ON, `-mature-validators 4`, all-attest,
`-anchors` = all four), degrades every link, and asserts the chain warms + a fetch
completes.

## What it found — real durability bugs (now fixed)

Under a *mild, entirely realistic* impairment (80 ms ± 20 ms jitter), silt's
consensus bootstrap **collapsed** — clean-network commits in 6 s, jittered network
never committed. Root causes, all fixed:

1. **`RequestTimeout` was 500 ms (LAN-tight).** A global P2P RPC (TCP connect +
   query) routinely exceeds that on a jittery path → requests fail. Raised the
   daemon default to **5 s** (`-request-timeout`).
2. **First miss = eviction.** A *single* timed-out RPC called `table.Remove(peer)`
   — one slow/dropped packet tore a good peer out of everyone's routing table,
   keeping the mesh sparse. Added **retry-with-exponential-backoff**
   (`-request-retries`, default 3; `-request-backoff`) so a peer is only given up
   after the retries are exhausted.
3. **Bond audits evicted peers.** A `BondChallenge` reply carries a large
   space-time proof and is slow by design; under adversity these time out in droves
   and (2) then evicted the peer — starving consensus of the very standing it was
   establishing. A bond-challenge timeout now **never evicts** from routing (it's a
   *standing* signal, not a *reachability* one; standing simply lapses and re-audits).

With those, the **jitter** case (the common cross-region reality) now warms and
fetches bit-perfect.

## Known harder gap (a real FINDING)

**Jitter + packet loss** (e.g. `+ loss 2%`) still does not reliably bootstrap, even
with a generous timeout. Two contributors remain, tracked as a finding:
- loss on the *large* bond-proof / chunk replies (a dropped segment ⇒ TCP retransmit
  ⇒ the whole reply is slow), and
- the **C1 reply-latency gate** (`-bond-answer-latency`, the partial-storage
  deterrent) measures *wall-clock* reply time, which includes network latency — so on
  a lossy link an honest prover's slow reply is falsely read as a short-storage cheat
  and denied standing. Loosening it is a **security tradeoff** (it weakens the C1
  deterrent) and needs a deliberate call, so it is filed, not silently changed.

## Run

```sh
./run.sh                                   # default: 80ms±20ms jitter → PASS
NETEM="" ./run.sh                          # clean-network control → must PASS
NETEM="delay 80ms 20ms distribution normal loss 2%" ./run.sh   # jitter+loss → known FINDING
NETEM="delay 150ms 40ms distribution normal" ./run.sh          # harsher jitter
CHURN=1 ./run.sh                           # also kill+restart the boot validator mid-cold-start
REQ_TIMEOUT=10s BOND_LAT=0 LOGLVL=debug KEEP=1 ./run.sh         # probe knobs; leave it up
```

| knob | default | meaning |
|---|---|---|
| `NETEM` | `delay 80ms 20ms distribution normal` | raw `tc netem` arg; empty = clean control |
| `REQ_TIMEOUT` | `5s` | per-attempt RPC timeout (`-request-timeout`) |
| `BOND_LAT` | `1500ms` | C1 reply-latency gate (`-bond-answer-latency`; 0 = off) |
| `WARM_TIMEOUT` | `180` | seconds to wait for the first committed block |
| `CHURN` | `0` | 1 = kill+restart the boot validator during the cold start |
| `LOGLVL` | `info` | `debug` surfaces the per-RPC retry/timeout lines |

Needs Docker with `NET_ADMIN` (compose sets `cap_add: [NET_ADMIN]` so `netem`
applies); the harness hard-fails if the impairment didn't apply, so a missing
capability can't produce a false clean PASS.

**RESULT semantics** (immutable #4 — never fake green): a real committed block +
bit-perfect fetch under the impairment = `PASS`; a failure to warm/fetch under a
realistic impairment = `FINDING` (exit 0) with the exact `netem` params, a
reproducer; a build/harness error = hard `FAIL`.
