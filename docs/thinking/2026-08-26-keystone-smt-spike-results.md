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

Gate step 3 asked for produce + verify cost. First on an Apple M4 (**not the
floor box** — the floor-box run follows, and confirms every byte of this), where
the profile is stable across four orders of magnitude:

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
(`O(changed · log n)`) and not the #555 `O(state)` scar. (These are single
samples; the floor-box section below min-filters properly, and the tightened
numbers agree.)

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

## The floor-box measurement — run, and it confirms the projection

Gate step 3 is now **closed**. Measured on a dedicated GCP `e2-custom-1-2048`
(**1 vCPU Intel Xeon @ 2.20 GHz, 1976 MB, no swap**, Debian 12). No swap matters:
an OOM here is a true OOM, not the thrash `silt-dev` would have shown.

`e2-small` — what `integration/cloudtest` uses — was deliberately **not** used. It
is a shared-core burstable machine, and burst credits make wall-clock
unrepeatable. A measurement needs a dedicated vCPU.

The correctness suite was re-run on x86_64 first: all five tests pass, including
the related-leaf guard. Soundness is not architecture-specific, but it costs
nothing to confirm rather than assume.

| n | build | per-key | heap MB | nodes/key | store B/key | prove | verify | proof B |
|---|---|---|---|---|---|---|---|---|
| 1 000 | 0.01 s | 11.4 µs | 0.7 | 2.23 | 217 | 14.6 µs | 37.3 µs | 571 |
| 10 000 | 0.15 s | 15.1 µs | 7.1 | 2.25 | 218 | 7.3 µs | 32.7 µs | 679 |
| 100 000 | 1.81 s | 18.1 µs | 70.6 | 2.25 | 218 | 6.8 µs | 34.1 µs | 791 |
| 1 000 000 | **22.42 s** | 22.4 µs | **751.6** | 2.24 | 218 | 12.1 µs | 57.5 µs | 900 |

**The portability claim was correct, and is now confirmed rather than argued.**
Heap at 1M: **751.6 MB on the floor box vs 751.7 MB on the M4.** Stored bytes,
nodes per key, and proof sizes are identical to the digit. Bytes are bytes.

Wall-clock runs **~9× the M4** (22.42 s vs 2.43 s at 1M) — inside the "5–15×"
bracket the laptop write-up guessed, so the earlier lower-bound framing was
honest.

### Q6 gets its number: boot rebuild ≈ 22 s per 1M entries

That is the certification's rebuild-vs-persist input, and it is still a **lower
bound** — measured against an in-memory store, so a disk-backed rebuild pays I/O
this never did. Read as: rebuild-at-boot is comfortable at 1M, questionable at
10M, and the disk-backed store's I/O is the term that decides it.

### Q4's hot path, and a measurement lesson

First sample said 11.04 ms at n=100k. A second run of the identical binary said
4.80 ms. **~2× run-to-run variance on this box** — a single sample would have
shipped a wrong number, and briefly did suggest a superlinear trend that is not
real.

Min-filtered over 5 repeats × 50 iterations (the same discipline
`network-durability.md` §applies to latency signals — take the floor, never one
sample):

| n | apply 100 changed keys (min of 5) | growth per 10× state |
|---|---|---|
| 1 000 | 2.37 ms | — |
| 10 000 | 3.01 ms | 1.27× |
| 100 000 | 3.80 ms | 1.26× |

`log n` predicts 1.33× then 1.25×. Observed 1.27× and 1.26×. **Q4's
`O(changed · log n)` is confirmed on the floor box**, not merely plausible.
A GOGC sweep (off / 100 / 400) put GC at roughly 18% of the cost, so this is
hash-and-cache bound, not GC bound — worth knowing before anyone "optimizes" it.

Hot-path cost to carry into the design: **~4 ms per block for 100 changed keys at
100k entries, on the floor box.**

### The ceiling, as a field number: the in-memory trie is disqualified

The projection said an in-memory trie would not survive registry scale. The floor
box was asked directly, and the kernel answered:

| n | outcome |
|---|---|
| 1 000 000 | fits — 751.6 MB resident |
| 2 000 000 | **OOM-killed**, anon-rss 1.68 GiB |
| 3 000 000 | **OOM-killed**, anon-rss 1.69 GiB |

```
Out of memory: Killed process 1218 (smtspike.test) total-vm:3014992kB, anon-rss:1763216kB
```

No swap, so this is a clean kill, not a thrash. **The hard ceiling sits between 1M
and 2M entries.** Worth noting from the same dmesg: the pressure also drove
`google_guest_agent` to invoke the OOM killer — on a real node, the process
competing for that memory is the silt daemon itself.

And 1M "fitting" is misleading in isolation. **flixz already runs at 1060 MB on a
1909 MB box.** 1060 + 752 = 1812 MB against ~1900 MB usable: **even 1M entries
would OOM a box already running the production daemon.**

`byRoot` is the "∀ content ever" term. 2M entries is not a large registry. So
build-immutable #8 decides this in its own words — *"a mechanism whose output is
tiny but whose production blows the floor is disqualified, however elegant"* —
and it disqualifies the **in-memory backend**, not the library. The disk-backed
`MapStore` is not an optimization to consider later; it is the only viable
configuration, and 218 B/key says a 10M-entry registry is 2.2 GB on disk, which
is unremarkable.

## What is still NOT measured

Gate step 3 is closed, but two things remain open and should not be quietly
inherited as settled:

- **The disk-backed rebuild.** Every wall-clock number was measured against an
  **in-memory** store. A disk-backed `MapStore` pays I/O this profile never did,
  so **22 s per 1M is a floor, not a forecast.** Q6's rebuild-vs-persist choice
  needs re-measuring once the real store exists — the residency result forces
  that store, and the store changes the number Q6 turns on. This is the one
  measurement the keystone build must not skip.
- **The sharded omission protocol.** The library gives a key-bound absence proof,
  which is the right primitive. Which keys a slice-holder is *obliged* to answer
  for is a protocol question the certification flagged as its own consult
  (§11.2), and nothing here touches it.

`silt-dev` was rejected as the measurement host and it is worth recording why:
correct spec (1 vCPU / 1909 MB), but it runs the **flixz production daemon** —
1060 MB RSS, up since 2026-08-19, box already 880 MB into swap. Profiling there
would have thrashed a live production node, and swap would have masked the OOM
ceiling that turned out to be the decisive result.

## Status

- **Gate steps 1, 2, and 3: CLOSED.** The recommendation is no longer
  documentary. `pokt-network/smt` is adoptable; the JMT port stays off the table.
- **The library is adopted; the in-memory backend is rejected.** Those are
  separate verdicts and the spike separated them. `simplemap` is a reference
  backend, and it dies at 2M entries on the floor box.
- **Carried into the build:**
  1. Back the trie with a **disk-backed `MapStore`** — five methods, and the only
     configuration that survives build-immutable #8.
  2. Keep the **default path hasher**; never `WithPathHasher(nil…)`.
  3. **Field-tag every key** for domain separation.
  4. Budget proof depth at **2·log₂(n)**, not log₂(n).
  5. Budget the hot path at **~4 ms per block** per 100 changed keys at 100k
     entries, floor box.
  6. **Re-measure boot rebuild** against the real disk store before settling Q6.

## A method note worth keeping

The n=100k hot path measured **11.04 ms** on its first sample and **3.80 ms**
min-filtered over five — a single sample would have shipped a ~3× wrong number
and a superlinear trend that does not exist. `network-durability.md` already says
this about latency signals: *minimum-filter a noisy signal to its floor rather
than trusting one sample.* It applies to benchmarks on shared cloud hardware for
exactly the same reason. Every timing claim above is min-filtered; the one-sample
number is recorded here only as the counterexample.
