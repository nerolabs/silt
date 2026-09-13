# silt — proposed release candidate list

A proposal, not a verdict. Derived from `docs/VISION.md` and `docs/TENETS.md`, plus a set of
defects confirmed in the code. Nothing here is graded and nothing here is final. Whoever
picks this up should expect to cut items, merge items, and rewrite pass conditions.

**The line this list draws:** a release candidate is the point at which handing silt to an
adversary is a responsible act. Not the point at which it is pleasant to use.

---

## The twenty

**1 — Publishing is unlinkable.** Colluding token issuers who also control the validators,
the chain and the wire cannot link a committed root to the identity that paid to publish it.
Measured as a matching score against chance, not as the absence of a struct field.

**2 — No surveillance mechanism exists.** After a fetch-heavy run, seize every node's disk
and every byte it emitted. No artifact holds a (fetcher, content) pair. Published metadata
leaks no content shape — today the exact plaintext byte count is public, which makes the
padding defence a no-op.

**3 — Takedown bites, cannot go global, and is provable.** Operators who honour a denylist
stop serving; operators who don't, keep serving. No accepted operation removes more than one
named root, and none works by identity. Every honoured removal carries an inclusion proof
and a consistency proof from an append-only log.

**4 — Forging N standings costs N times the real work.** Shared plots, data-less possession,
self-dealt demand and massed keys each earn zero. An honest control in the same run must
earn non-zero, or eight zeros just means the harness granted nothing to anyone.

**5 — No quiet capture, and the scaffolding sheds one way.** A bonded Sybil quorum with real
committed standing cannot advance a young chain once the anchors stop. Once the network
matures and the launch scaffolding sheds, no later collapse in decentralization re-arms it —
including after a full replay from genesis.

**6 — An external adversary denies all three failure modes.** A party other than the author,
given the artifact and the claims but not the rationale, runs its own suite and returns
DENIED on publish-to-identity linkage, on identity-level or global takedown, and on
Sybil-farmed standing at a discount. **This one is not the builder's to turn green.**

**7 — Publish and fetch work on the internet as it is.** A NATed publisher in one region, a
cold fetcher in another, bit-perfect bytes inside a bound derived from the deployed
configuration rather than a chosen constant. The chain keeps committing under sustained load
and under injected latency, jitter, loss and reordering.

**8 — Bit-perfect or an explicit failure.** Never silently wrong. A publish that cannot be
placed returns an error naming what could not be placed, returns no link, and leaves no
registry entry. A transient failure still succeeds on retry, so the loud failure isn't
bought by breaking retries.

**9 — Crash recovery with no human in the loop.** A validator killed mid-consensus restarts,
re-pins from its own last finalized checkpoint, and never contradicts a signature it made
before the crash. A fresh node cold-starts from a checkpoint and converges.

**10 — Content outlives the nodes that held it.** Holders depart permanently and are never
replaced; repair outruns loss and fetches stay bit-perfect. Including against a provider that
simply lies about what it holds — today a self-reported boolean is the only check that runs
on shipped defaults.

**11 — The floor box holds.** A validator on the declared floor spec validates against
witnesses without holding the tree, stays under its memory ceiling on adversarial input, and
stalls rather than accepting when no witness provider is reachable. A hostile witness is
rejected — today it is not; see the defect note below.

**12 — The economy pays for durability and mints nothing.** Repair is funded from the
object's own escrow. A colluding pair strictly loses. Balance-lane credit never becomes
consensus standing. A false repair claim is slashed. The edge tier that does most of the work
ends a run net-positive.

**13 — The consensus denials hold, and no honest node is ever slashed.** Equivocation is
detected and attributed; forged and under-bonded proposals are rejected before attestation; a
partition heals to one order. Asserted under adversarial scheduling before any expensive run.
The honest-never-slashed part is a property of the whole run's slash set, not of each attack.

**14 — An un-upgraded node stalls; the network never updates itself.** A node that cannot
validate a shipped format stops rather than accepting it, and says so while staying alive. A
node whose consensus-reaching configuration diverges from what the chain committed refuses to
start. No version-floor advisory below the signing threshold changes anything, and no node
ever replaces its own binary.

**15 — Core carries nothing.** Hosts hold ciphertext they cannot read and did not choose by
content. Core holds no capability to decrypt it and resolves hashes, never names. Proven by
seizing a holder and failing to recover known plaintext — with a key-holder succeeding on the
same objects in the same run.

**16 — A prover without the bytes fails the audit and is paid nothing.** Today three separate
paths defeat this: the care link printed on every publish is the storage-proof verification
key, so a zero-byte prover passes; an identity with no data passes by relaying the challenge
to a real holder; and the inclusion proof is checked against a root the prover supplies.

**17 — No unauthenticated frame buys unbounded work.** One small unsigned repair claim
currently makes a judge fetch n−1 survivor shards and run a decode, with no rate limit. One
73-byte challenge frame costs a paged read and an aggregation, with no limit either.

**18 — The stock binary runs.** `silt daemon -validator` with no other flags reaches serving.
Today the default bond sits below the derived floor and the process exits.

**19 — The shipped default is the defended configuration.** Every defence this list
demonstrates is on with no flags passed. Today the possession audit and the publish-token
replay guard both ship off, which means several items above are being demonstrated against a
configuration nobody runs.

**20 — The chain prunes; disk is bounded too.** Retention pruning engages on every validator
at depth, read from persisted state. The list asserts a memory ceiling on the floor box and
currently no storage ceiling anywhere — both come from the same commitment, that
participation stays cheap on a small machine.

---

## What this list deliberately does not cover

Blob-layer unobservability against a global passive adversary — the vision says outright that
silt does not promise the impossible. The full multiplicative Sybil interlock — the vision
calls it the target, not yet the guarantee, and a destination that concedes a gap cannot be
used to manufacture a gate it does not claim. Throughput numbers — bounded first, fast
second. Conformance across multiple implementations, when there is one. Ergonomics, SDKs and
embeddability, which belong to 1.0. Scale beyond three regions.

Two limits worth carrying in writing rather than by implication. The non-globality metric and
the diversity axis both rest on **self-declared** operator domains, so both are claims about
declared diversity. And the profitable-edge commitment is about a trajectory at network
scale; any single run can only measure a point on it.

## Tenets that could not be reduced to a demonstration

Legibility. The hexagonal core and the single lock-free loop — architectural constraints
asserted by structure, whose consequence is testable but whose violation would not
necessarily show. "Never reinvent a primitive," a negative over all future choices. Reactive-
not-eager, which states no threshold. Canon-tracks-behaviour and throwaway-stays-throwaway,
which are disciplines rather than properties of a running system.

And the mission itself. It is falsifiable only as item 6. The other nineteen are the evidence
that makes commissioning item 6 worth doing. **They are not a substitute for it.**

---

## Decisions that live only here

These do not derive from the vision or the tenets. They are the owner's.

- **The date: 2026-09-27.** Anything not green on that date ships disclosed rather than
  fixed.
- **The stopping rule: nineteen green.** Item 6 is carved out and happens after the handoff.
  Nineteen green is not the mission proven.
- **The cut.** That these twenty are exhaustive is a judgement, not a derivation.
- **Two numbers that do not exist yet.** The floor box has no declared memory ceiling and
  nothing anywhere declares a disk ceiling. Items 11, 19 and 20 cannot be graded until both
  are set, and they should be what the machine can afford — not what the daemon happens to
  use today, which would make the bound unfailable.

---

## Two defects worth naming separately

**The recompute does not anchor its pre-state.** Item 11's hostile-witness arm is the only
confirmed consensus-soundness break in the list. The floor box's state-root recompute reads
witnessed pre-state membership without anchoring it, so an attacker chooses the set the box
applies its deltas to — re-bonding a slashed equivocator, or evicting an honest validator.
Two instances, and the second has no non-membership check at all, so removal is free.
Per-member proofs do not close it. Only anchoring the set digest does. **A fix that hardens
the member checks will look correct and leave the break intact.**

**The launch/mature intersection has a hole at the seam.** This is the last thing the deleted
record gave up, and it is recorded nowhere else. Every consensus invariant has at least one
passing test, so none is unguarded. But two gaps sit inside the first invariant, agreement:

- Its direct disjointness oracle is **launch-phase only**, and says so in its own comment. The
  mature phase is covered by a *threshold* test pinning a strict two-thirds boundary, with
  intersection following arithmetically rather than from a direct oracle.
- The **handoff transition has no test at all.** Intersection must hold *across* the boundary,
  not only within each phase. The disjointness check is called at exactly one site in the
  tree — inside the launch oracle. Nothing asserts that a block finalized under launch rules
  and a block finalized under mature rules cannot both finalize at the same height.

That seam is where a fork would live if one lives anywhere.
