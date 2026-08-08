# Byzantine red-team harness (Docker) — validator accountability (#184)

An automated field test that stands up **real silt validators in containers,
talking over real TCP**, and proves an honest validator holds three concrete,
wire-reachable accountability properties against a Byzantine peer. These are the
Dockerized, over-the-wire form of the in-process analogs in
`e2e/equivocation_test.go` and `e2e/proposal_reject_test.go`.

The adversarial behaviours are the repo's own **RED-TEAM / TEST-HARNESS ONLY**
daemon flags (`silt daemon -h`) — a correct node never performs them; they exist
precisely so the defences can be exercised over the real wire:

| flag | Byzantine behaviour | honest-validator defence proven |
|------|--------------------|--------------------------------|
| `-equivocate "idX,idYZ"` | double-sign at height 1 (X→idX, heavier Y,Z→idYZ) | reconciles the fork, catches the double-sign, **slashes** the offender (D2) |
| `-forge-block <peerID>` | proposal with a corrupted proposer signature | `ValidateProposal` verify fails → **rejects** before attesting |
| `-lowbond-propose <peerID>` | well-formed proposal from an under-bonded proposer | proposer not a qualified bonded validator → **refuses** to attest |

## Run it

```sh
./integration/redteam/run.sh          # build, run all scenarios, tear down; exit 0 = PASS
KEEP=1 ./integration/redteam/run.sh   # leave the topology up afterward to poke at
```

Needs Docker and a Go toolchain. The `silt` binary is compiled **on the host**
(CGO off → trivial cross-compile to the container arch) into
`integration/redteam/silt` (gitignored, never committed) and COPYed into a slim
image `silt-redteam` — so the image stays tiny and there's no ~1 GB Go-build
memory spike inside Docker. `silt id` runs **inside a container** (the binary is
linux) so run.sh can learn the deterministic NodeIDs and wire every
`-attesters` / `-equivocate` / `-forge-block` / `-lowbond-propose` target up
front.

## What it asserts (every string is REAL — see the source cited)

1. **Positive control** — two honest validators (`h1`, `h2`) earn mutual bonded
   standing and commit a normal publish through consensus:
   `chain: committed block N (…)` on **both** (`cmd/silt/daemon.go` `OnCommit`).
   This proves the quorum is otherwise healthy, so the rejections below are real
   defences, not a dead swarm.
2. **Equivocation** — the double-signer completes
   `adversary: equivocation complete (double-signed height 1)` and the honest
   detector prints `chain: slashed equivocator <id> (double-signed at height 1)`
   (`cmd/silt/daemon.go` `OnSlash`).
3. **Forged block** — `adversary: forge-block proposal correctly REJECTED by <id>`
   (and it FAILS loudly on `… UNEXPECTEDLY ACCEPTED … (DEFECT)`).
4. **Low-bond propose** — `adversary: lowbond-propose proposal correctly REJECTED by <id>`
   (same DEFECT guard).

Each daemon prints these to **stdout**, so the driver greps `docker compose
logs`, not `<store>/debug.log`.

## Topology

One flat bridge network `10.110.0.0/24` (compose project `redteam`). Services
are brought up **selectively per scenario** via compose profiles so the topologies
don't cross-contaminate:

- **default** — `h1` (also serves the registry) + `h2`: the honest quorum /
  positive control.
- **`equiv` profile** — `equiv-a` (adversary, bootstrap anchor), `equiv-x`
  (honest detector, `-attesters equiv-yz`), `equiv-yz` (honest recipient of the
  heavier fork, no attesters). Mirrors `e2e/equivocation_test.go` exactly: A
  earns standing with both, places X on the detector and the heavier Y,Z on the
  recipient; the detector syncs the heavier fork, reconciles, and slashes A.
- **`propose` profile** — `h3` (fresh honest target), `forger`
  (`-forge-block h3`), `lowbond` (`-bond 128K -lowbond-propose h3`). Each sends
  ONE crafted proposal and reports whether H3 refused it.

The honest validators use the earned-bonded-standing recipe the e2e tests and
`examples/flow4-earned-standing.sh` prove commits over TCP: legacy subjective
consensus (`-objective=false`), `-min-rep 100`, `-quorum 1`, a real `8M` bond,
`-bond-audit 1s` so standing warms in a few seconds.

## What each piece is

| file | role |
|------|------|
| `docker-compose.yml` | the topology: honest quorum + three adversaries, split across compose profiles |
| `Dockerfile` | slim runtime image (silt binary only), one image for all roles |
| `run.sh` | the driver: build → learn IDs → positive control → 3 scenarios → assert → tear down |

`run.sh` traps cleanup and runs `docker compose down -v` on exit (unless
`KEEP=1`), so it leaves nothing running.

## Out of scope (future work)

This test covers the **wire-reachable** accountability defences of #184. The
broader M0 economic red-team — **C1 "no discount"** (an operator cannot earn
standing more cheaply than the honest cost of the bonded space-time) and **C2
"no quiet capture"** (concentration cannot silently cross the capture fraction) —
is an economic/on-chain property, not a single-message wire exchange, and is not
asserted here. The daemon already surfaces the C2 signal on every commit
(`C2: nakamoto … | concentration HHI … | ⚠ CONCENTRATION ALARM …`); a future
field test could drive a multi-operator bond-splitting scenario and assert that
signal, plus a C1 cost-accounting check.
