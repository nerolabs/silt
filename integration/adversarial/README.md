# Deterministic adversarial-consensus certification

Certifies the trust plane's marquee **denials** under adverse-network conditions,
on a laptop, in minutes — deterministically, every run:

| Drill | Attack | Required denial |
|---|---|---|
| `TestEquivocatorSlashedOverTCP` | a validator double-signs at one height | an honest replica reconciles the fork and **slashes** the equivocator |
| `TestPartitionHealsToHeavierForkOverTCP` | the network splits; each side commits its own fork | on heal, the lighter side **reorgs onto the heavier fork** — consensus reconverges |
| `TestForgedBlockRejectedOverTCP` | a proposer forges its block signature | the honest target **refuses to attest** (verify fails) |
| `TestLowBondProposerRejectedOverTCP` | an under-bonded validator proposes a valid block | the honest target **refuses to attest** (not a qualified proposer) |

## Why this exists

The 2026-08 principal-engineer rescue audit (`silt-reviews/principle-engineer/RESCUE-AUDIT.md`,
P2) found these being "certified" on a flaky live GCP wire that kept failing to
even **drive** the attack, then re-grading the miss as a passing **GAP**. That is
backwards. *An attack you cannot schedule is not a test.*

So the certification lives here, on a substrate where the attack is **schedulable**:

- The `e2e/` drivers already run real `silt` daemons in separate processes over
  real TCP, and are deterministic by construction (fork-choice is summed attester
  weight, not a timing race). They are the honest attack drivers.
- They talk over `127.0.0.1`, so applying `tc netem` to the container's **loopback**
  degrades the *actual consensus traffic* with the latency, jitter, and loss a real
  WAN has. The attack still fires; the denial still must hold — now under adversity.

This is the **deterministic** half of the trust-plane certification. The cloud run
(`integration/cloudtest`) is reserved for the one thing only a real multi-region WAN
proves — **liveness + timing at scale** — and is gated so it can never again become
the place attacks are *discovered* instead of *confirmed* (the `cloudtest` pre-flight
gate, build-immutable #6).

## Grading — RED, never a passing GAP, and never a verdict it did not earn

A drill that cannot drive-and-deny its attack makes this script exit **non-zero** — a
hard RED. There is no "GAP because it couldn't be driven": if the harness can't force
the attack, the harness is the bug, and it gets fixed here on the deterministic
substrate.

But a RED must also say *which kind* of RED it is. The harness distinguishes four
outcomes that are not the same finding, and the exit code carries the distinction:

| Exit | Meaning | Is it a property verdict? |
|---|---|---|
| `0` | every **named** drill ran and held under the impairment | yes — a pass |
| `4` | **setup**: the drills never built. Zero drills ran. | **no** |
| `3` | **harness**: an impairment was requested but `tc` could not apply it (missing `--cap-add NET_ADMIN`) | **no** |
| `5` | **unearned**: `go test` exited 0 but fewer than all named drills produced a result (a `-run` regex matching nothing, a `t.Skip`) | **no** |
| else | a real property verdict — a drill failed to deny its attack | yes — a finding |

Codes 3 and 4 are why. `nightly-netem` was red 19 of its first 21 runs; for thirteen
of those nights the `e2e` package had **failed to compile** and not one drill had run,
while the verdict line said *"a property did NOT hold"*. Nobody triaged it, because it
read as a durability finding rather than a broken build. Code 5 closes the mirror hole:
`go test` exits 0 on *"no tests to run"*, so a renamed drill silently leaving the suite
would otherwise certify green over zero execution (build-immutable #4 — never fake green).

## Usage

```sh
./run.sh                                              # default: adversarial drills, ~cross-region impairment
SUITE=substrate ./run.sh                              # P0 substrate liveness under netem (objective commit, bond-standing, publish→fetch)
SUITE=all ./run.sh                                    # the full P0 netem gate (substrate + adversarial) in one run
NETEM="" ./run.sh                                    # clean-network control (must PASS)
TESTS='TestEquivocatorSlashedOverTCP' ./run.sh       # one property by name
```

**The scheduled arms.** Build-immutable #5 names jitter, latency, packet loss **and
reordering** as the everyday case, so `.github/workflows/nightly-netem.yml` runs one
arm per impairment — every arm at `SUITE=all`:

```sh
SUITE=all NETEM="" ./run.sh                                             # clean control
SUITE=all NETEM="delay 80ms 20ms distribution normal" ./run.sh          # jitter
SUITE=all NETEM="delay 120ms 40ms distribution normal loss 2%" ./run.sh # loss
SUITE=all NETEM="delay 20ms reorder 25% 50%" ./run.sh                   # reorder
```

`reorder` **requires** a delay: `tc` rejects it outright otherwise (*"reordering not
possible without specifying some delay"*, exit 1), which this harness renders as a
HARNESS ERROR rather than a clean pass. A malformed reorder arm cannot go green.

Do not weaken an arm to make it pass. A drill tuned down until it goes green is worth
less than no drill, because it also carries a green badge.

**Suites.** `adversarial` (default) = the M0 consensus *denial* drills. `substrate` = the P0
*liveness* half the drills ride on — objective quorum commit, bond-earned-standing commit, and
publish→fetch bit-perfect, all over real TCP under impairment. `all` runs both as the single P0
netem gate. (The cold-start re-mesh test is intentionally not in the netem suite — its tight
500ms/0-retry config is a clean-localhost timing test; the fix is certified in the clean e2e suite.)

Requirements: Docker with `--cap-add NET_ADMIN` (works under colima). The silt
source is bind-mounted read-only and the host module cache is reused, so there is
**no** committed binary and the cert always runs the working tree.

The module cache is mounted **writable**, and that is load-bearing rather than
incidental. Mounted `:ro` it shadowed the container's own writable `/go/pkg/mod`, so
any module the host cache happened to be *missing* became unresolvable and the `e2e`
package would not build. It only bites on a **partial** cache — with no host cache the
mount is skipped and the container downloads freely; with a complete one nothing needs
writing — which is why it survived a month of nightly reds.

## What it does NOT cover

Liveness/timing at scale across a real multi-region WAN — that is `integration/cloudtest`
(the R1 gate, #360). This harness proves the *denials hold under adversity*; the cloud
proves the *network stays live at scale*. Two halves, two substrates (tenet V1).
