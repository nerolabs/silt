# Publisher unlinkability under an adversary — field test

**Outcome under test (cynical):** can an adversary get the network to record **who
published a given file?** A silt registry entry may carry a durable `Publisher`
NodeID — a permanent file→publisher link on the append-only chain (the #14 / F1
privacy corner). The M0 default **refuses** to record it. This test attacks that
guarantee over a real committing chain and asserts on real `chain-status`,
`committed block`, and rejection lines — never a string the harness echoes.

Immutable #4 (D-PRIV) is **refuse-to-surveil**: the network won't help build a
publisher↔content map. This test frames the adversary as a would-be surveiller who
*wants* the link recorded, and measures whether the network denies them.

## Topology

Two real `silt daemon -validator` processes (`valA` chain-backed registry, `valB`
attester, `quorum=1`) on a flat Docker network — a minimal chain that actually
commits. `-allow-publisher=${ALLOW_PUB}` lets `run.sh` flip the chain's linkage
policy between phases from one compose file.

## What it asserts

- **P0 — positive control (the gate is real policy).** On a chain run with
  `-allow-publisher=true` (a trusted deployment), an `-allow-publisher` publish
  **commits**. So a Publisher-bearing entry *can* commit — proving the refusal in
  P2 is a deliberate policy gate, with the chain's `AllowPublisher` the only
  variable, not a broken publish path.
- **P1 — the private path works.** On the **default** chain, a normal unlinkable
  publish **commits** and fetches back **bit-perfect**. Privacy isn't bought with a
  broken product.
- **P2 — refuse-to-surveil.** On the *same* default chain, an `-allow-publisher`
  publish is **refused**: the validator logs `chain: ErrPublisherEntry` ("carries a
  durable Publisher") and **no new block commits** (commit + entries counts read
  from `chain-status` are unchanged). The would-be surveiller cannot make the
  network record the link, even asking directly.
- **P3 — authorized yet unlinkable (bonus).** A `-token-quorum` publish gathers a
  **blind** validator credential and commits carrying **no** Publisher — proof of
  authorization without identity (the F1 fix). Not hard-gated: the full quorum
  path is exercised at scale on the cloud test.

A privacy regression — the default chain committing an `-allow-publisher` entry, or
the private path failing to commit/fetch — is a **FAIL** (not a soft finding): the
guarantee is binary. `P3` is advisory.

## Note on observability

The product's query surfaces (`silt ls`, the daemon's `apiRegistry` / `apiChain`)
deliberately **do not expose the `Publisher` field** — itself a privacy property.
So this test reads the guarantee where it is enforced: the chain's **accept/reject**
of a Publisher-bearing entry (commit counts + the rejection log), not by dumping a
field. The metadata layer (an on-path observer correlating publish traffic by
timing/volume) is a **stated tradeoff** of M0, not covered here — silt does not
claim to defeat a global passive adversary.

## Run

```sh
./run.sh            # build, run P0–P3, tear down; exit 0 = PASS
KEEP=1 ./run.sh     # leave the last topology up to poke at
```

The GCP judge (`integration/cloudtest`) runs the same unlinkability claim across
independent machines, where the full multi-validator `-token-quorum` signer path
and real network timing are exercised.
