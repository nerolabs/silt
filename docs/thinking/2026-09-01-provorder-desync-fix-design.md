# provOrder desync fix — keep the FIFO order slice in lockstep with the map

- Date: 2026-09-01
- Boulder 0, follow-on to the A4 money-pump fix (`2026-09-01-a4-provisional-eviction-fix-design.md`).
- Driver: blind red-team finding, `silt-reviews/red-team/RED-TEAM-FINDINGS-delivery-credit-provorder-2026-09-01.md`
  (RT-DELIV-1 HIGH, RT-DELIV-1b MEDIUM-HIGH, RT-DELIV-2 MEDIUM).

## The break (mechanism, with evidence)

`RedeemDeliveryCredit` deleted the redeemed lane from `l.provisional` but never
removed its key from `l.provOrder`. `provOrder` had one append site
(`trackProvisional`, new lane) and one pop site (the eviction loop front-shift,
gated on `len(l.provisional) >= maxProvisional`). Redeem bypassed the pop, so on
the redeem-heavy path — where the map stays small and the eviction loop never
fires — `provOrder` grew one entry per witnessed delivery, forever.

Three consequences the red-team drove against the real `credit.Ledger` at
`9d50437`:

- RT-DELIV-1 (HIGH, build-immutable #8): `provOrder` unbounded on the honest
  path. Repro reddened `TestProvOrderStaysBoundedAcrossRedeems` at
  `provOrder == 50,000` (cap 8192, 6.1x over).
- RT-DELIV-1b (MEDIUM-HIGH, liveness/B2): a dead prefix makes one serve's
  eviction loop scan the whole prefix on the serialized consensus loop.
- RT-DELIV-2 (MEDIUM, correctness/grief): a stale front entry for a re-served
  key reverses the LIVE re-served lane's mint before any redeem. Reddened
  `TestRedeemDoesNotLeaveDuplicateOrderEntry`.

Not a mint (the red-team's 8000-step conservation fuzz refuted the double-pay);
a durability/DoS + grief break. So this is a Builder fix gated by the Tester's
regression, not a Research certification.

## Options considered

| Option | Redeem cost | Eviction cost | Kills RT-DELIV-2? | Verdict |
|--------|-------------|---------------|-------------------|---------|
| 1. Scan-and-remove from `provOrder` on redeem | O(n) | O(1) | Yes | Rejected — reintroduces RT-DELIV-1b on the hot redeem path |
| 2. Tombstone slot in O(1) + amortized compaction | O(1) | O(1) amortized | Yes | CHOSEN |
| 3. Put the server in `provKey` (+ generation) | O(1) | O(1) | Partly | Out of scope — changes the conserved-lane key shape; cert-gated (RT-DELIV-3), routed separately |

## The fix (option 2, conservation-shape-neutral)

`provOrder` becomes `[]*provKey`; a companion `provIndex map[provKey]int` gives
each live lane's current slot. Every removal site keeps the two in lockstep:

- Redeem: `removeFromProvOrder(k)` tombstones the slot (`nil`) in O(1) via
  `provIndex`, alongside the existing map delete. No slice scan.
- Eviction: the loop skips leading tombstones, then pops the oldest LIVE lane
  and drops its index entry. A tombstone carries no lane, so it is never counted
  as an eviction — the map bound is unchanged.
- `compactProvOrder` rebuilds the slice with tombstones dropped when it grows
  past `2*maxProvisional`. It touches at most `len(provOrder)` entries but runs
  only once every ~`maxProvisional` appends, amortizing to O(1) per serve and
  never scanning on the redeem path. The slice is thereby capped at
  `2*maxProvisional` — bounded state on the floor box (build-immutable #8).

RT-DELIV-2 is closed because the redeem tombstone means no stale key of a
re-served lane can sit in front of it: the tombstone is `nil`, not the key, so
the eviction loop cannot resolve it to the live re-served lane.

## Conservation is untouched (R0.4 cert stays valid)

No change to what is minted or reversed, or when the balance/escrow legs move.
`reverseProvisional`, the fee/skim arithmetic, and both terminal-reversal call
sites are byte-for-byte unchanged. The ONLY change is WHEN and HOW the
order-slice key is removed (a data-structure bookkeeping change). The
conserved-lane key shape (`provKey{requester, root}`) is unchanged — RT-DELIV-3
is explicitly left for a separate, cert-gated shape decision.

## Proof

- `TestProvOrderStaysBoundedAcrossRedeems`, `TestRedeemDoesNotLeaveDuplicateOrderEntry`:
  RED at `9d50437`, GREEN after.
- `TestA4MoneyPumpConservation`, `TestProvisionalCapIsBoundedAndDeterministic`:
  stay GREEN.
- Full `go test ./...` green; `core/credit -race` clean.
- Ablation: backing out `removeFromProvOrder` reddens both new gates (and the
  bound test now takes ~23s — the RT-DELIV-1b stall shape made visible).
