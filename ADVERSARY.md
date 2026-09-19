# silt — the adversary's brief

You are the external adversary. silt makes three claims. Your job is to **break them**, and to
return a verdict on each: **DENIED** (you broke it) or **UPHELD** (you tried and could not).

These are claims **under test**, not established facts. Nothing here asks you to confirm them.
A DENIED is the more useful result, and it is the result this document exists to make reachable:
the project's own governing text says its mission is held *if and only if* a party other than the
author fails to break all three. That party is you.

You are expected to read the source. Everything you need is in this repository; nothing is
withheld, and no part of the system is off-limits to attack.

---

## The system, in one paragraph

silt is a content-addressed storage and distribution network with two planes. The **storage
plane** holds erasure-coded, encrypted chunks addressed by the hash of their bytes, served
peer-to-peer. The **trust plane** is a BFT proof-of-stake-class consensus over a registry, where
influence is earned by **bonded proof-of-space-time** rather than by a coin: a validator seals a
plot to disk, registers it on chain, and must re-prove it on a short clock or its standing lapses.
Publishing spends a blind-signed token, so the network can confirm a publish was paid for without
learning who published. Content is removed by acting on a **hash**, through per-operator lists,
never by identity and never through a global switch. The whole thing is designed to run on a
1-core / 2 GiB / 10 GiB box.

Entry points: `cmd/silt/` (the daemon and CLI), `core/chain/` (consensus, the registry, the
committed state tree), `core/node/` (the node loop), `core/credit/` (the economy),
`core/blindtoken/` (the unlinkable tokens), `adapters/` (disk, network, clock).

---

## Claim 1 — Privacy: a publish cannot be linked to the identity that earns standing

**The claim.** An act of publishing is cryptographically unlinkable to the durable, bonded
identity that performs it. Identity creation is free and pseudonymous; standing costs work; and
the two are not joinable. Separately, silt builds **no mechanism** to observe or record
who-fetched-what: the refusal to surveil is absolute.

The access-privacy half is bounded and the bound is part of the claim: unobservability is pursued
at the **metadata layer**, to the limit the anonymity trilemma allows. A node necessarily sees the
keys it routes and serves, and a global passive adversary watching every wire defeats any
low-latency network. silt does not claim otherwise. What it claims is that it never *builds* the
surveillance itself.

**You have denied it if you can:**

- link a publish to the bonded identity that paid for it, at better than chance, from anything the
  network emits or stores — chain data, wire bytes, or a seized disk;
- recover a `(fetcher, content)` pair from any artifact silt writes, after a fetch-heavy run;
- show that some field silt commits or transmits carries a durable identifier of the publisher;
- show that the blind-token construction admits forgery, double-spend, or de-blinding by the
  issuer.

**Where the mechanism lives.** `core/blindtoken/` (the blind signature and its domain separation),
the publish path in `cmd/silt/` and `core/node/`, the committed leaf set in `core/chain/statehash.go`
(what the chain records about a publish at all).

---

## Claim 2 — Accountability: removal is real, and it is neither global nor identity-level

**The claim.** Genuinely harmful content can be removed, by a hash, with real effect — and no
accepted operation removes content everywhere, and none acts on an identity. Takedown is
consent-based and plural: operators subscribe to the curators they choose, so the effect of a list
is proportional to who trusts it. Every honoured removal is committed to an append-only
transparency log carrying inclusion and consistency proofs, so the non-globality of a removal is a
provable quantity rather than a promise.

**You have denied it if you can:**

- construct an operation the network accepts that removes more than one named root;
- construct one that acts by identity rather than by hash;
- cause content to become unavailable on operators that did **not** honour the removal;
- show an honoured removal that leaves no transparency-log entry, or a log whose inclusion or
  consistency proofs can be forged or made to disagree with the log itself;
- show that a curator can suppress content without it being attributable to them.

**Where the mechanism lives.** The revocation and un-revocation path in `core/chain/chain.go`,
the transparency log and its committed size leaf, `integration/takedown/`.

---

## Claim 3 — Sybil: standing cannot be farmed at a discount, and the scaffolding cannot be captured

This is the load-bearing claim and the one the project treats as its reason to exist. It has two
halves and **denying either denies the claim**.

**The claim, half one — no discount.** Forging *N* standings costs *N* times the real,
non-substitutable work an honest provider pays. Standing is bonded to sealed, sized, re-challenged
storage, and a shortcut on one axis is meant to trip another axis's check. The claim is about
**re-pricing and concentration-bounding**, not prevention: an honest actor who genuinely provides
that much work legitimately holds that much standing, and that residual is bounded by a
concentration metric rather than eliminated. A cheaper-than-honest path to standing denies it.

**The claim, half two — no quiet capture.** A young network leans on explicit, time-boxed anchor
validators. They shed on a measured decentralization threshold through a **one-way latch** that,
once tripped, never re-arms — so a later dip in decentralization cannot hand the launch anchors
permanent power. The claim is that maturity is reached before the scaffolding can be captured.

**You have denied it if you can:**

- obtain standing for *N* identities at less than *N* times the cost of one honest identity's —
  by re-using one plot, by synthetic or non-unique bytes, by self-dealt demand, by massing
  identities cheaply, or by any route that makes the *N*th standing cheaper than the first;
- hold standing without continuously re-proving the storage that backs it;
- make the decentralization measure report a value the real operator distribution does not
  support, in either direction;
- cause the shed latch to re-arm, or to trip early, or to never trip on a genuinely decentralized
  set;
- capture, stall, or fork consensus with a bonded minority, or cause a validator set to be frozen
  so the network cannot mature.

**Where the mechanism lives.** `adapters/diskplot/` and `core/bond/` (sealing and challenge),
the bond-registration, expiry and slash path in `core/chain/chain.go` (`apply`), and the epoch
freeze, the activation tallies and the one-way maturity latch in `rotateEpoch` in the same file.
The committed leaf set those decisions are recorded in is `core/chain/statehash.go`. The suites
are `integration/sybil/` and `integration/redteam/`.

---

## What counts as a break

**Attack the composition, not the parts.** The novelty here is the way the pieces are combined,
so a primitive that fails a standalone "is this Sybil-proof on its own?" test is *expected* and is
not a finding. A break is a path through the **system** to one of the three outcomes above.

**Show it, do not argue it.** A break is a reproduction: a test, a script, a sequence of commands,
or a trace that another party can run and observe. State what you did, what you expected, and what
happened. A break that only exists as reasoning is a hypothesis, and this brief asks for results.

**Attacks on the honest path count.** If you can make an honest node accept something a correct
node would reject, or reject something it would accept, that is a break whether or not it maps
neatly onto one of the three claims — say which claim it touches and report it.

**A near miss is worth reporting.** If you got close, say where you stopped and why. That is
evidence too, and it is more useful than silence.

---

## Running the evidence that exists

From a clean clone, with no credentials:

```sh
go test ./...                      # the unit and model-check tiers
./integration/run-all.sh           # the fast docker suites
FULL=1 ./integration/run-all.sh    # + the slow ones
```

`integration/run-all.sh` names, for each suite, the claim it is meant to gate, and writes a
consolidated report. Each suite also stands alone at `integration/<name>/run.sh`.

A suite that will not run on your machine is itself worth reporting: evidence you cannot reproduce
is evidence that does not count.

---

## Returning the verdict

For each of the three claims: **DENIED** or **UPHELD**, with your reproduction attached for every
DENIED and your strongest attempt described for every UPHELD.

Open an issue, or a pull request adding a failing test, whichever carries the evidence better. A
failing test in this repository is the strongest form your finding can take: it makes the break
permanent, reproducible, and impossible to argue with.
