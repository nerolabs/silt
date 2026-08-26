# The keystone SMT spike — executed. The library holds; the residency number is the finding.

**Date:** 2026-08-26. **What this closes:** the build-immutable #7 gate that
[the library call](2026-08-26-keystone-smt-library-call.md) left open. That
document recommended `pokt-network/smt` on **documentary evidence — quoted
source, not executed code** — and bound the recommendation to a spike:

> 2. Prove a **specific key absent** against a root … and assert that an absence
>    proof for A **fails**. The second half is the one that matters …
>    If step 2 does not behave as the quoted source says, this recommendation is
>    void and the JMT port returns to the table.

**Verdict: step 2 behaves as the quoted source says. The recommendation stands.**
The spike lives at `internal/smtspike/`, imports nothing from the product, and is
imported by nothing.

## What was proven, not read

The verifier source at the resolved version (`v1.0.0`) matches the quotes in the
library call verbatim, including the load-bearing guard at `proofs.go:422`. But
matching source is still documentary. The tests execute it:

| Assertion | Test | Result |
|---|---|---|
| A specific absent key verifies as absent against a committed root | `TestAbsentKeyProvesAbsent` | PASS |
| An honest **membership** proof, re-read as an absence claim for the same present key, is rejected | `TestAbsenceProofForPresentKeyFails/membership…` | PASS (`ok=false`) |
| Another key's absence proof, replayed against a **present** key, is rejected | `…/another key's absence proof replayed` | PASS, 256/256 candidates |
| The `"non-membership proof on related leaf"` guard **actually fires** | same | PASS — fired on candidate 5 |
| Membership still verifies; wrong value and wrong key do not | `TestMembershipProofStillVerifies` | PASS |
| The same raw key under two field tags is two independent entries | `TestFieldTagSeparatesKeySpaces` | PASS |

The fourth row is the one worth calling out. It would have been easy to write a
test that passes because *every* replayed proof fails the root recomputation,
never reaching the related-leaf branch at all — the branch the soundness argument
actually rests on would then be asserted by hope. The test sweeps candidate keys
until one lands on the present key's leaf, and **fails if the guard never fires**.
It fires.

### A correction carried forward, in the library's favour

The library call's implementation note said to hash the key into the leaf
position because silt's keys are adversary-influenced. That is right, but the
default `pathHasher` **already** digests the key — position is `SHA-256(key)`, so
grinding content buys no control over trie shape. `TestAdversarialKeysDoNotDeepenTheTrie`
confirms it behaviourally: 4096 keys sharing a fixed 32-byte prefix reach max
proof depth **23**, against **24** for spread keys.

So the field tag's job is **domain separation** (a `byRoot` key must not collide
with a `spent` key), not grind resistance. The only way to forfeit grind
resistance is to opt out via `WithPathHasher` and a nil path hasher — which silt
must not do. That is a sharper rule than "hash the key," and it is now pinned by
a test.

Incidental: max depth lands near **2·log₂(n)**, not log₂(n) — the expected
maximum common prefix among n random paths. Proof sizes should be budgeted
against that, not against log₂(n).

## The measurement, and the finding it produced

Gate step 3 asked for produce + verify cost. Measured on an Apple M4 (**not the
floor box** — see the honest gap below), the profile is stable across four orders
of magnitude:

| n | build | per-key | heap MB | nodes/key | store B/key | prove | verify | proof B |
|---|---|---|---|---|---|---|---|---|
| 1 000 | 0.00 s | 3.5 µs | 0.7 | 2.23 | 217 | 2.1 µs | 5.7 µs | 571 |
| 10 000 | 0.02 s | 2.4 µs | 7.1 | 2.25 | 218 | 1.0 µs | 3.6 µs | 679 |
| 100 000 | 0.18 s | 1.8 µs | 71.5 | 2.25 | 218 | 1.7 µs | 3.5 µs | 791 |
| 1 000 000 | 2.43 s | 2.4 µs | 751.7 | 2.24 | 218 | 2.4 µs | 6.3 µs | 900 |

The incremental claim holds. `BenchmarkApplyBlock` applies 100 changed keys to a
tree of size n: **367 µs at n=1 000, 526 µs at n=10 000, 1 076 µs at n=100 000.**
A 100× growth in state costs under 3× in apply time — the cost tracks `changed`
and grows logarithmically in n, which is the certification's Q4 shape
(`O(changed · log n)`) and not the #555 `O(state)` scar.

**The finding is the residency split.** Two columns say different things:

- **218 bytes/key** is payload — what a store must actually hold. Constant across
  scales, at a steady 2.24 nodes per key.
- **752 bytes/key** is Go heap with the reference in-memory backend
  (`simplemap`). **Overhead is 3.4× the payload.**

At 1M registry entries that is **752 MB resident** against a 2 GB box, versus
**218 MB on disk**. And `byRoot` is the "∀ content ever" term — the dominant
forever term in the certification's own §Q3 table.

This sharpens the certification's Q6. Q6 framed the choice as rebuild-vs-persist
for **durability** ("the tree is a derived cache; prefer rebuilding at boot"). The
numbers add a second, independent constraint that bites first: **residency**. An
in-memory trie is not viable at registry scale on the floor box regardless of what
we decide about durability, so the trie needs a **disk-backed `MapStore`** either
way. Q6's durability answer is unaffected and still right — never trust a torn
tree, the chain wins — but "don't persist" cannot mean "keep it all in RAM."

The good news is that this is cheap. `kvstore.MapStore` is **five methods**
(`Get`/`Set`/`Delete`/`Len`/`ClearAll`). Backing it with silt's own store is an
adapter, not a project; `internal/smtspike/store_test.go` is a worked example.

## Dependency footprint

Checked, because silt's dependency profile is deliberately lean:

- `pokt-network/smt v1.0.0`'s only requirement is `testify` — **test-only**.
  After `go mod tidy` the module adds **zero new indirect dependencies**.
- `go list -deps` over `./cmd/... ./core/... ./client/...` confirms **no product
  package imports it**. The dependency is behind the spike, as the gate required.
- Pure Go, SHA-256, and the Thesis Defense audit PDF ships inside the module
  (`audits/240612_Thesis_Defense-…pdf`). This satisfies the certification's
  "Pure Go, SHA-256 only" constraint directly.

## The honest gap — what is NOT yet measured

**These numbers are from an Apple M4 dev laptop, not the 1 vCPU / 2 GB floor
box.** Build-immutable #8 wants the floor-box number *before commit*, and the
certification repeats it ("measure produce-and-rebuild cost, not just verify
cost — the #299 scar"). Splitting what transfers from what does not:

- **Transfers.** Bytes are bytes on any 64-bit target: 218 B/key stored,
  752 B/key resident, 2.24 nodes/key, ~900 B proofs, ~2·log₂(n) depth. The
  residency finding above is therefore already binding, and it is the one that
  changes a design decision.
- **Does not transfer.** Every wall-clock number. A 1 vCPU x86 box will be
  materially slower, and the build column is worse than it looks: it was measured
  against an **in-memory** store, so a disk-backed rebuild is dominated by I/O the
  profile never paid. **Treat the 2.43 s/1M build as a lower bound, not an
  estimate.**

**`silt-dev` is not available as the floor box.** It is the correct spec
(1 vCPU / 1909 MB), but it is running the **flixz production daemon** — 1060 MB
RSS, up since 2026-08-19, with the box already 880 MB into swap. Running a
1M-key profile there would thrash a live production node. It was not run there,
and it should not be.

That leaves a clean 1 vCPU / 2 GB instance as the way to close step 3 — a
billable launch, so it is the owner's call, not the builder's.

Worth noting on its own: **flixz already sits at ~1 GB RSS on a 2 GB box.** The
keystone's resident cost lands on top of a real deployment that is already tight.
That is independent evidence for the disk-backed store, from the field rather
than from a benchmark.

## Status

- **Gate steps 1 and 2: CLOSED.** The recommendation is no longer documentary.
  `pokt-network/smt` is adoptable; the JMT port stays off the table.
- **Gate step 3: PARTIAL.** The scale-invariant costs are measured and the
  residency conclusion is firm. The floor-box wall-clock — boot rebuild in
  particular, which is what Q6 turns on — is still owed.
- **Carried into the build:** back the trie with a disk-backed `MapStore`; keep
  the default path hasher (never `WithPathHasher(nil…)`); field-tag every key for
  domain separation; budget proof depth at 2·log₂(n).
