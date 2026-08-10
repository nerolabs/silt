# C2 — no quiet capture, under a Sybil validator set — field test

**Outcome under test (cynical):** can a bonded **Sybil validator set** — many
identities, real bonds, its own quorum — **quietly capture** a young objective
network? The M0 composition says no: while a network has never matured, an
objective commit needs an honest launch **anchor's** co-sign (the training wheels,
H4/D-C2). The Sybils are not anchors, so the network will not run for them alone.

## Topology

Two honest anchors (`a1` registry+proposer, `a2` co-signer — a single anchor can't
co-sign its own block, so `-anchor-quorum 1` needs a second) plus a Sybil set
(`s1` registry, `s2`, and `-sybil` extras: equal bond, one domain). All objective
validators; the anchors have genesis standing, the Sybils do not.

## What it asserts (real `chain-status` / `committed block` / refusal lines)

- **C2-a — the anchored young network is live.** With the anchors present the chain
  commits a real block and the daemon prints the C2 status line with
  **`wheels engaged (young network — anchor quorum still required)`**. Proves the
  chain works *and* the wheels are on (so C2-b's refusal is the gate, not a stall).
- **C2-b — no quiet capture.** Stop **both** anchors. The Sybil set (`s1` proposes,
  `s2` attests) tries to commit alone and **cannot advance the chain — no new
  block.** The test reports *which* training-wheels layer stopped it:
  - the **standing** gate — a young Sybil set can't even earn committed bonded
    standing without an anchor-proposed block to register its bonds; and behind it
  - the **anchor co-sign** gate (`ErrAnchorRequired`) — even *with* standing, a
    young commit needs an anchor.

  Both refuse the capture; the **outcome is the same**. A chain that advanced for
  the Sybils would be a hard **FAIL** (quiet capture).
- **C2-c (bonus) — the split is legible.** With ≥8 committed equal bonds the C2
  metric's **atomization note** fires (one operator across many uniform keys reads
  as a fingerprint, not real decentralization). Reported if seen.

## Honest scope (immutable #5: say what a laptop can only approximate)

On one host the Sybils' bonds don't reliably *bank* on-chain (a young network's
bond-registration needs anchor-proposed blocks), so locally the **standing** gate
usually fires first — which is itself a faithful no-capture outcome. The
**pure anchor-co-sign gate** (pre-banked Sybil bonds → `ErrAnchorRequired`) and the
**≥8-bond atomization** signal are exercised at scale on the **cloud test**
(`integration/cloudtest`), where a real multi-machine, longer-running network banks
the bonds. The core property — *the young network refuses to run for a Sybil set
without the honest anchors* — is asserted here, locally, on real chain state.

## Run

```sh
./run.sh              # build, run C2-a/b/c, tear down; exit 0 = PASS
SYBILS=0 ./run.sh     # core only (a1,a2,s1,s2) — fastest
KEEP=1 ./run.sh
```
