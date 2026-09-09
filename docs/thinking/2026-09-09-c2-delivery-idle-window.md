# 2026-09-09 — Lane C2: the delivery idle window, and what a driven run did to its premise

**Context / trigger:** ratified owner call 4 of `D-TRUE-UP-CALLS-2026-09-07` holds
`-delivery-idle-window` at REFUSE-UNTIL-SET "until Lane A1's fix lands and the ≤ f+1 bound
(190 s at f=1) is field-confirmed on a graded run, and then ABOVE that bound (today's
arithmetic: ≥ 4 epochs ≈ 24 min at `T_b` = 44 s)". The graded run
`integration/cloudtest/report-97e3101-deep.md` met the precondition on 2026-09-09. The Tester
derived the value RED-first on `tester/c2-idle-window`; this is the Builder's record of what
was decided while turning that derivation into shipped code.

---

## The mechanism, stated before the fix

**The failure is that the daemon accepted any window ≥ 1 s, and the graded harness set 90 s,
because the floor was an engineering minimum rather than a derived one.** `deliveryIdleFloor`
existed only to keep the `idle/2` sweep ticker interval positive (`time.NewTicker` panics on a
non-positive interval). It said nothing about the property the window exists for: an honest
fetcher that goes quiet for a while must not lose its session. Two inputs the ROADMAP
arithmetic did not carry make the gap larger than it looks.

1. **The governing stall is 430 s, not 190 s.** `D-H43-WORKLESS-DESIGNEE` (21) publishes the
   190 s figure as the modal tier and, in the same sentence, bounds a LOST entry forward by
   the re-keyed takeover at `(N+2)·ChainSyncInterval + G` = 430 s at N = 12. A defensive
   window must dominate the worst the model admits, not the common case.
2. **A window of `D` guarantees only `0.75·D`.** `deliveryStamp` floors the last-settle stamp
   into buckets of `idle/4` (`core/node/deliverysession.go:99,186`), so a settlement landing
   just under a bucket boundary is stamped almost a whole bucket in the past. Measured on the
   shipped reaper at every stamp phase: 0.751× on a 1000 s window.

So the floor is `bound × 4/3` = 9m33.333333333s, and the 90 s the graded sheet ran on
guaranteed 67.5 s — under even the 190 s tier the same sheet confirms.

## The options, and why 24m

| option | guarantee | cost | verdict |
|---|---|---|---|
| keep REFUSE-UNTIL-SET | — | the operator invents a number; the harness already invented 90 s | rejected: the precondition the call named is met |
| 10m (tightest round window over the floor) | 450 s = 1.047× the bound | none | rejected: 4.7 % margin is inside the measurement error of the block interval the bound's inputs are quoted at, and 10m is REAPED by a repeat of the 1040 s stall the field produced (driven) |
| **24m** | **1080 s = 2.51× the bound, and over the 1040 s field stall** | a session slot held longer; no deposit-lock cost (below) | **shipped** |
| an epoch count | drifts with `T_b` | — | rejected: the bound is denominated in `ChainSyncInterval` and does not move with block time, so an epoch-denominated default drifts away from the number it must dominate |

**The deposit-lock cost is zero, and that is not obvious.** `CloseDeliverySession` releases at
`max(close, maxAnchorEpoch + W + 1)` with `W = 4`, i.e. 5 epochs × 8 blocks × 45.865 s =
30m35s. The release epoch binds at BOTH candidate windows, so the longer window adds no
latency to the fetcher's refund. Session-slot cost is bounded by `deliveryMaxLiveSessions`
= 4096 against one live session per fetcher and one grant per fresh identity — faucet-limited,
not window-limited.

## What a driven run did to the premise

The window was sized, in both the ROADMAP row and ratified call (4), on the sentence in
`R-SESSION-WALLCLOCK-STEP`: "a chain stall … reaps every live session". The Tester froze the
chain for 1040 s and the lane admitted a session, settled 104 receipts, took a top-up, and the
session lived. Nothing in `SettleDeliveryReceipt` → `credit.SettleDelivery` reads the chain,
and `MsgFetchChunk` has no chain gate. OPEN and FUND *do* read the chain epoch, and a frozen
chain freezes the epoch — which widens the anchor's validity window, the safe direction.

**The number does not move; its status does.** 430 s stops being a bound the reaper is racing
and becomes a conservative envelope on how long an honest fetcher may be gapped, adopted
because the ratified sentence says to size above the bound. That distinction is written into
`m0.md`, the flag help, the constant's comment and the CHANGELOG, because the VALUE is the
owner's to ratify and they should ratify it on the reasoning that is actually true.

## Decisions taken in the build

- **The floor is EXACT, not a safe over-estimate.** A floor of a year clears the bound too;
  a floor above the derived one refuses windows the model says are safe. `G-C2-13` asserts
  equality, and the ablation that proves it is setting the floor too HIGH.
- **Compile-time guards over tests where the property is arithmetic.** Four
  `const _ = uint(A − B)` guards make the inequalities unbuildable rather than red. They are
  strictly stronger on their axis and strictly weaker at stating a property, so the tests stay
  beside them — with the falsification procedure (lift the guard, then ablate) in the comment,
  because a guard pre-empts its twin test's RED.
- **The announced window is read back out of the reaper.** The floor check and the install were
  two independent reads of one variable and only the check was gated, so the daemon could refuse
  a non-compliant window and install another. `node.DeliveryIdleWindow()` closes it at runtime:
  the banner states what the reaper holds, and the e2e boot arm asserts the banner.
- **The graded sheet's idle-close assertion was RELOCATED, not deleted.** At a correctly-sized
  window no graded flow can observe an idle close, so rows 13/13b would have failed for the
  window being right. The close is asserted at the e2e tier, where the clock is injected and
  the assertion is a strict superset of what the row required. The daemon's periodic sweep
  goroutine is the part that loses its only observation; it is filed as
  `R-DELIVERY-SWEEP-TICKER-UNFIRED` rather than papered over.
- **The relay finding is recorded, not fixed.** An epoch-reaped relay session settles ZERO, not
  a fraction. Building a periodic relay sweep or incremental relay settlement moves an economic
  rule and is research-gated; the register row carries the measurement and the plain
  recommendation that `-accept-relay-payments` not be enabled at edge tiers.

## What is still not observed

No delivery session has ever settled on a real network (row `13b-delivery-settlement` is a SKIP
behind the era-4 probe), so every C2 number is sim-driven. The field run confirmed the CONSENSUS
bound, never the delivery lane's behaviour under it.
