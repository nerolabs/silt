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

## Grading — RED, never a passing GAP

`go test`'s exit code *is* the verdict: `0` means every attack was denied under the
impairment. A drill that cannot drive-and-deny its attack makes this script exit
**non-zero** — a hard RED. There is no "GAP because it couldn't be driven": if the
harness can't force the attack, the harness is the bug, and it gets fixed here on the
deterministic substrate. If an impairment was requested but `tc` could not apply it
(missing `--cap-add NET_ADMIN`), the run exits non-zero rather than pass a false
clean cert (build-immutable #4 — never fake green).

## Usage

```sh
./run.sh                                              # default: adversarial drills, ~cross-region impairment
SUITE=substrate ./run.sh                              # P0 substrate liveness under netem (objective commit, bond-standing, publish→fetch)
SUITE=all ./run.sh                                    # the full P0 netem gate (substrate + adversarial) in one run
NETEM="delay 120ms 40ms distribution normal loss 2%" ./run.sh   # crank it up
NETEM="" ./run.sh                                    # clean-network control (must PASS)
TESTS='TestEquivocatorSlashedOverTCP' ./run.sh       # one property by name
```

**Suites.** `adversarial` (default) = the M0 consensus *denial* drills. `substrate` = the P0
*liveness* half the drills ride on — objective quorum commit, bond-earned-standing commit, and
publish→fetch bit-perfect, all over real TCP under impairment. `all` runs both as the single P0
netem gate. (The cold-start re-mesh test is intentionally not in the netem suite — its tight
500ms/0-retry config is a clean-localhost timing test; the fix is certified in the clean e2e suite.)

Requirements: Docker with `--cap-add NET_ADMIN` (works under colima). The silt
source is bind-mounted read-only and the host module cache is reused, so there is
**no** committed binary and the cert always runs the working tree.

## What it does NOT cover

Liveness/timing at scale across a real multi-region WAN — that is `integration/cloudtest`
(the R1 gate, #360). This harness proves the *denials hold under adversity*; the cloud
proves the *network stays live at scale*. Two halves, two substrates (tenet V1).
