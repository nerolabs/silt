# provOrder eviction front-drop desync — the cursor fix (Boulder 0)

Date: 2026-09-01
Seat: Builder
Scope: `core/credit/delivery.go`, `core/credit/credit.go`, test harness
`core/credit/compaction_fuzz_test.go`. Conservation-shape-NEUTRAL. `provKey`
shape UNCHANGED (RT-DELIV-3 stays separately cert-gated).

## The failure (evidence)

`TestCompactionTombstoneFuzz/eviction-dominant` FAILS at step 13154, seed
`0xdeadbeef0002`, run WITHOUT `-short` (the 10K `-short` op count does not reach
the failure). The assertion:

```
provIndex[...]=3777 points to wrong key {...} — provIndex/provOrder desync
```

Deterministic and citable.

## The mechanism — the failure is X because Y

The eviction loop in `trackProvisional` dropped the front of `provOrder` by
re-slicing: `l.provOrder = l.provOrder[1:]`. Re-slicing shifts every surviving
entry down one physical position. But `provIndex` (the per-lane position map that
gives O(1) tombstoning) was NOT updated for the survivors. After one or more
front-drops, `provIndex[k]` held a STALE position — off by the number of entries
dropped from the front.

Downstream, `removeFromProvOrder` reads `provIndex[k]` and nils
`provOrder[staleIdx]` — the WRONG, still-LIVE slot. The intended lane stays
un-tombstoned. Compaction then drops the live lane's order entry, and a ghost
index entry can later drive `reverseProvisional` against a live re-served lane's
self-mint. The fuzz's integrity check catches the first symptom (index → wrong
key) at step 13154.

This is a real product defect. The A4 conservation fix (R0.4a) and the earlier
redeem-side desync fix (R0.4) both left the EVICTION-side front-drop unsynced —
it only bites when the eviction loop fires repeatedly (poolSize > maxProvisional),
which the `-short` path never exercises.

## Options considered

1. **Rebuild `provIndex` after each front re-slice.** O(live) per eviction. On
   the eviction-dominant path eviction fires nearly every serve, so this is
   O(maxProvisional) per op — exactly the RT-DELIV-1b serialized-loop stall the
   tombstone design exists to avoid. REJECTED.

2. **Decrement every survivor's `provIndex` by the drop count on each re-slice.**
   Same O(n)-per-op cost as (1). REJECTED.

3. **Logical head cursor (`provHead`) — CHOSEN.** Do not re-slice on a
   front-drop. Advance a `provHead` cursor marking the logical FIFO front and nil
   the dropped slot. `provIndex` stores ABSOLUTE positions into the never-shifted
   slice, so a front-drop touches no survivor's index. Eviction stays amortized
   O(1) (cursor advance, one nil write, each entry dropped once). Compaction —
   already the once-per-~maxProvisional O(n) rebuild — rebuilds the live entries,
   repoints `provIndex`, and resets `provHead` to 0 (the dead prefix is gone, no
   logical front left to skip). Boundedness (`len(provOrder) <= 2*maxProvisional`,
   build-immutable #8) is preserved because compaction still fires on physical
   length.

## Why this is conservation-shape-neutral (R0.4 cert stays valid)

The change touches ONLY the order-slice/index bookkeeping. Untouched:
`reverseProvisional`, the fee/skim arithmetic, the terminal-reversal call sites
(redeem + eviction), and the `provKey` shape. Nothing about WHAT is minted or
reversed changes — only the mechanics of tracking FIFO position. `TestA4MoneyPump
Conservation` stays GREEN across the change, confirming eviction reversal still
conserves.

## The harness gap the fix uncovered (test-only)

With the desync fixed, `eviction-dominant` then failed a CONSERVATION check
(delta negative — ledger short). Root cause is in the FUZZ DRIVER, not the
product: the redeem-branch fallback-serve (`case action<8` when the lane is not
live) performs a real `RecordServeToObject` that can force an eviction at cap, but
— unlike the `action<5` and `default` serve branches — it did NOT predict that
eviction, so `expectedTotal` never subtracted the reversed self-mint. Evidence:
`TestA4MoneyPumpConservation` (independent product conservation gate) stays GREEN;
a probe counted the product performing more evictions than the harness predicted,
in exactly the fallback branch. Fix: add the identical eviction-prediction block
already present in the other two serve branches. No product accounting changed.

## The `-race` scenario-C timeout — harness, not product

The Tester noted a `-race` full-ops timeout on scenario C. It is the fuzz's own
per-step `verifyProvIndexIntegrity` (O(provIndex + provOrder) ≈ O(maxProvisional)
per step, ×150K steps), not a product-path O(n). Proof: driving the PRODUCT path
alone at full ops under `-race`, with no per-step integrity check, completes in
~145 ms for both B (100K) and C (150K). The product path is amortized O(1); the
RT-DELIV-1b stall is not reintroduced.

## Evidence of green

- `TestCompactionTombstoneFuzz` all three scenarios GREEN at full op counts
  (200K/100K/150K), without `-short`.
- Boulder-0 gates GREEN: `TestA4MoneyPumpConservation`,
  `TestProvOrderStaysBoundedAcrossRedeems`,
  `TestRedeemDoesNotLeaveDuplicateOrderEntry`,
  `TestProvisionalCapIsBoundedAndDeterministic`,
  `core/node` `TestR05NodePathConservation`.
- Full `go test ./... -short` GREEN; `core/credit -race -short` GREEN.
- Ablation: restoring the `provOrder[1:]` re-slice reddens the fuzz at exactly
  step 13154 again; restoring the cursor turns it GREEN.
