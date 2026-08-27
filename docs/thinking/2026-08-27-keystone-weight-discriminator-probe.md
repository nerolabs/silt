# Keystone weight-discriminator probe (#603) — the era-3 freeze gate

Date: 2026-08-27 · Seat: Builder · Stacked on `keystone-probes-bonded-epochset` (#604)

## The gap this closes

The membership probes (#604) prove `epochSet` / `bonded` **membership** is load-bearing:
omitting either empties frozen membership, the attesters fail `attesterQualifiedAt`, `seen`
collapses to 0, and the **count floor** rejects with `ErrNoQuorum`. The blind PE ruling
(`RULING-keystone-probes-bonded-epochset-2026-08-27.md`, "Coupling the consult did not
name", L98-106) states the exact remaining hole:

> The leave-one-out as written would still pass if `epochSet` stored **membership only with
> all weights set to a constant** — because the flip is membership, not weight. So this
> oracle does not yet prove the *weight bytes* of `epochSet` are load-bearing.

C-7 (`C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`) CERTIFIES
the witness path needs the committed state load-bearing per-field before the era-3 format
freezes. Committing the int64 weight per member is only justified if a probe shows a WRONG
weight flips finality. This doc constructs that probe.

## Why the existing ablation cannot reach it

`snapshotBoot(src, name)` models omission by installing an **empty** map (test file L76-85).
For `epochSet` that empties membership, so `requireEpochWeightQuorum` short-circuits at
`total <= 0` (`chain.go:2452`) and returns nil — the weight predicate never fires. The
verdict flips via the count floor, not the weight rule. Blinding the weight BYTES requires a
different ablation: keep membership, flatten the per-member weights to a constant.

## The rule as written (read, not assumed)

`requireEpochWeightQuorum` (`chain.go:2443-2464`):

```
set := c.effectiveEpochSet(h)          // == epochSet away from the #535 boundary
total := Σ set[id]
if total <= 0 { return nil }           // degenerate — the membership-omission short-circuit
support := set[proposer] + Σ_{id∈seen} set[id]
if 3*support <= 2*total { return ErrNoQuorumWeight }   // the ⅔ predicate, as written
```

In a mature epoch the COUNT floor is just `cfg.Quorum` — `RequiredQuorum()` returns `q`
directly (`chain.go:1204-1205`: "weight rule carries the Byzantine bar"). So a low `Quorum`
lets the coalition clear the count floor while the weight predicate does the work. The
weight predicate fires only when `ByzantineQuorum && objective && epochsEnabled &&
matureEpoch` (`chain.go:2411`) — the same regime `weightWorld` already establishes.

## The coalition — below ⅔ weight, above the count floor

Four members, all bonded ≥ MinBond so all freeze into `epochSet`. The bond size IS the
frozen weight (`liveQualifiedSet`, `chain.go:1097-1099` → `rotateEpoch`, `chain.go:2850`).

| member | role in the probed block | real weight |
|--------|--------------------------|-------------|
| P      | proposer                 | 5 MiB       |
| A      | attester (enters `seen`) | 5 MiB       |
| W      | silent (whale filler)    | 1 MiB       |
| X      | silent                   | 1 MiB       |

- `total_real = 12 MiB`, `support_real = P+A = 10 MiB`.
- `3·10 = 30 > 2·12 = 24` → **weight predicate passes → ACCEPT.**
- Count floor: `seen = {A}`, `len = 1`; `Quorum = 1` → `1 >= 1` passes. The block clears the
  count floor honestly; the verdict is carried by the weight predicate, exactly the
  discriminator the PE named.

Concentrating weight in the coalition {P, A} is what carries the real block over ⅔. This is
the ⅔ rule AS WRITTEN — no boundary is moved.

## The ablation — flatten the weight bytes to a constant

Blind the weight: keep `epochSet` membership identical, set every member's weight to the
SAME constant (`MinBond`). Then support/total = |coalition| / |members| = 2/4 = ½ for ANY
constant `k`:

- `total_flat = 4k`, `support_flat = 2k`.
- `3·2k = 6k ≤ 2·4k = 8k` → **weight predicate fails → REJECT with `ErrNoQuorumWeight`.**

The reject is constant-independent: a validator that lost the true per-member weights and
knew only membership CANNOT reproduce the ⅔ verdict. That is the weight bytes proven
load-bearing. Membership is unchanged, so `seen` is unchanged and the count floor still
passes — the flip is unambiguously the weight rule (`ErrNoQuorumWeight`), not the count
floor (`ErrNoQuorum`). This is the RED the probe must watch.

## Why a dedicated test, not the generic leave-one-out loop

The generic loop (`TestLeaveOneOutProvesEachFieldLoadBearing`) only knows one ablation:
empty the map. The weight claim needs the flatten-to-constant ablation. Shoehorning a second
ablation mode into the shared loop would complicate the membership probes it already carries.
A dedicated `TestEpochWeightBytesAreLoadBearing` states the weight claim directly and keeps
its bespoke ablation local. It reuses the same world regime (mature epoch, `objectiveVerify`,
`ByzantineQuorum`) and the same `quorumVerdict` path as the membership probes, so it isolates
exactly the weight predicate.

## STOP-condition check (from the task ruling)

The HARD STOP fires if constructing the coalition requires: (a) changing how `total`/`support`
are summed (`chain.go:2450-2456`, the #402 seam), (b) moving where/when `epochSet` freezes
(I3, `rotateEpoch`), or (c) DECIDING which side of `⌈A/2⌉` vs `⌊A/2⌋+1` finalizes.

None fires. The construction uses the summation exactly as written, freezes `epochSet` the
same way `weightWorld` already does (genesis mature latch → `rotateEpoch` at height 0), and
the ⌈A/2⌉ launch-anchor boundary is not in play (no anchors; the mature weight rule governs).
Building a test AT the ⅔ boundary is sanctioned; no boundary is MOVED.

## Verdict to watch

- full (real varied weights) → **accept**
- ablated (weights flattened to a constant, membership intact) → **reject with
  `ErrNoQuorumWeight`**

A green check is decoration until its RED is watched. The test asserts the ablated verdict is
`ErrNoQuorumWeight` specifically (via `errors.Is`), and the injection step below confirms the
RED is the weight predicate, not a count-floor or panic artifact.
