# The client / web-UI path under an adversary — field test

**Outcome under test (cynical, two-sided):**
- what a real **user** experiences: run a daemon, open its web UI, drop a file in,
  get a link, someone fetches it back — all over the daemon's **HTTP API**, not the
  `silt swarm` CLI; and
- what a real **attacker** experiences: a malicious web page or a LAN neighbour
  trying to drive that same local API must be **refused** (the guard, #89).

Both are asserted against the live HTTP surface with `curl` — real status codes and
real bytes — never a string the harness echoes.

## Topology

`ui` (the operator's node: storage + its own chain-backed registry + `-ui`) plus
`N` plain `holder`s bootstrapped to it, so a UI publish genuinely **scatters across
the swarm** and a UI fetch genuinely **pulls it back over the wire** (not a
local-store shortcut). The UI is driven from **inside** the `ui` container — the
realistic model (a browser on the same machine) and the only way the guard's
local-`Host` rule is satisfied. The attacks then spoof `Host` / `Origin` / token.

## What it asserts

| | check | expected |
|---|---|---|
| **U1** | `POST /api/publish` (multipart + bearer token) | a link, `placed>=2`, **and >=2 distinct holders each report >0 held chunks** via their own `GET /api/status` |
| **U2** | `GET /api/roots` | the roots count grew **and** the U1 holder floor held |
| **U3** | `GET /api/fetch?link=…` | the bytes back **bit-perfect** (a real over-the-wire round-trip; the U1 holder floor held) |
| **U4** | `POST /api/publish` with **no** token | **401** |
| **U5** | `POST /api/publish` with a **wrong** token | **401** |
| **U6** | any request with a non-local `Host` | **403** (DNS-rebinding defense) |
| **U7** | any request with an evil `Origin` | **403** (cross-origin drive-by) |
| **U8** | `GET /api/status` with no token | **200** (reads stay frictionless) |

The token is grabbed from the daemon's own `ui: http://…?token=…` line — the test
never fabricates it. A failure on any check is a **FAIL**: the user path and the
guard are both load-bearing.

**Positive control (why U1–U3 can't false-pass).** A UI publish goes through an
ephemeral swarm client that could land every shard on the `ui` daemon itself — a
valid remote node — with **zero holders participating**, and a bit-perfect fetch
served entirely by that one node would still satisfy a naive "over the wire" claim.
So the holders are started with `-ui` too, and U1 reads each holder's **own**
`GET /api/status` held-chunk count (dialing the holder's swarm IP with a spoofed
local `Host`, since reads need no token) and requires **>=2 distinct holders**
actually holding shards. U2 and U3 also carry that floor, so neither a single-node
loopback nor a lost swarm can masquerade as a green end-to-end round-trip.

## Run

```sh
./run.sh                 # build, run U1–U8, tear down; exit 0 = PASS
HOLDERS=8 ./run.sh
KEEP=1 ./run.sh          # leave the topology up (the UI is on the ui container's :8081)
```

The GCP judge (`integration/cloudtest`) exercises the same publish→fetch path across
real machines; this local suite owns the **web-UI HTTP surface + its local-security
guard**, which a single host models faithfully.
