# C4 — the R2.7 blocking telemetry: the six calls the advisory left open

**Date:** 2026-09-08 · **Lane:** C4 (ROADMAP) · **Seat:** Builder
**Spec:** `silt-agent-memory/economist/reviews/ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07` §1.0–§1.4
**Built against:** `origin/main` `c849d15` (Lane C1 merged)

The design was specified counter-by-counter and site-by-site, so this is not a design
record. It records the six places where the advisory read `e963034` and the code has since
moved, or where following the spec's letter would have produced a worse artifact. Each is
a call I made; each is reversible.

## 1. Detector A2 has THREE terminal terms, not four

The advisory's fourth counter, `serveBytesSupersededFlat`, lived in
`RedeemDeliveryCreditReason`'s supersede block, and the same table row says *"It is deleted
with the leg."* Lane C1 deleted the leg: `RedeemDeliveryCreditReason` and the `core/demand`
v2 primitive are gone from `origin/main`, and `grep` finds no supersede block to count in.

**Call: build three, not four.** A fourth counter would be a permanent zero with no
increment site — a green with no possible red, which the Simplicity rules call decoration
(rule 7) and a concept added for nothing (rule 8). The conservation identity becomes
`objectAware == witnessed + laneEvicted + inFlight`, which is exactly as exact.

**What this costs:** the advisory wanted the fourth counter to double as an *"is the
retired flat leg still firing?"* alarm. That alarm is now structural instead — the leg does
not exist, so it cannot fire, and C1's own deletion gates hold that property.

## 2. `objectEscrow.funded` is DERIVED, not a third accumulator

The advisory asks for `fundedPrepay` and `fundedSkim` and for a test asserting
`fundedPrepay + fundedSkim == funded` at every step. That assertion is a drift check on
three accumulators kept in step by two write paths — the same shape §1.0 point 1 refuses
for `servedBytesUnwitnessed`. Applying the advisory's own reasoning one section later:
store the legs, derive the total.

**Call: delete the `funded` field; `funded()` returns `fundedPrepay + fundedSkim`.**
`EscrowFunded` and `DurabilitySnapshot.Funded` publish the same number they always did.

**The one behavioural question this raises, answered:** `reverseLane` previously subtracted
the claw-back from the combined total and floored it at zero; it now subtracts from the
skim leg and floors that. Those diverge only if a claw-back exceeds the outstanding skim,
and it cannot: `r` is bounded by the lane's own recorded `p.skim`, and the skim leg holds
every skim ever credited to that root less what earlier reversals took. So the published
value does not move, and the property *"a reversal never touches an operator's prepay"*
goes from incidental to structural. The existing floored-claw-back gate
(`r29_delivery_settlement_test.go`, `funded == 17`) is untouched and green.

**What the test asserts instead:** because the sum is definitional, the teeth are the two
legs individually plus the reversal landing on the skim leg only. The ablation is
`reverseLane` clawing back the prepay leg, and it reddens.

## 3. A4-3 rides `economyRevenue`, which is NOT token-gated — so `Revenue` is rebuilt

The advisory says A4-3 rides `economyRevenue` and that all three A4 parts are token-gated.
Those two statements conflict on current code: `withheldEconomySelf` is an allow-list at
the `economySelf` FIELD level and it passes `full.Revenue` through by pointer, so anything
added inside `economyRevenue` ships OPEN to an unauthenticated reader. The function's own
doc-comment warns about exactly this shape one level up.

It matters concretely. `bountyPaidToPriorFetcherCredits` is a bounty-out figure; on a node
caretaking one root it is that root's `objects[].bountyOut`, which is withheld, while
`/api/roots` names the root. That is the red-team F2 join, and there is a shipped gate
scanning unauthenticated bodies for exactly that quantity.

**Call: `withheldEconomySelf` builds a fresh `economyRevenue` with the two A4-3 fields
zeroed and the note replaced by `"withheld: token-gated (F2)"`.** The cached document's
`Revenue` pointer is never written through — the clause-assigns-never-mutates rule the
`readerView` comment states.

## 4. The affordability floor ships on BOTH branches of the faucet block

§1.3 puts the two counters in `faucetInfo` beside `grantsDenied`. But `FaucetStats()`
returns a zero struct when no bucket is configured, and `computeStatus` writes
`&faucetInfo{}` on that branch — so the spec's placement, followed literally, would drop a
real refusal count on the default posture (no `-faucet`), which is a silent loss (Don't #4).

**Call: report them on both branches, and say in both doc-comments that these are the only
two fields in the block that are NOT faucet counters** — they count refusals for want of
CREDIT, not for want of a token, and are meaningful with the bucket absent.

The alternative — a new top-level `spend` block — adds a document section and a concept for
one pair of numbers (rule 8), and diverges from the spec's stated reader. Rejected.

`spendRefusalNote` ships beside the numbers, always, including at zero: the honest limit is
that a zero certifies nothing, and a zero that travels without its caveat is exactly how it
would be misread.

## 5. A4-2 counts self-PAYMENTS, not self-releases

§1.2 says *"at `repairclaim.go:223` where `paid` returns"*. `settleRepairVerdict` returns
early when `paid == 0` (an empty escrow on this judge). Counting at the call site would
include releases that moved no money.

**Call: increment inside the `paid > 0` block, beside `BountiesReleased`.** The abort
threshold C-7 is *"an honest judge never pays itself"*; a release that pays nothing is not
a payment. Both count and credits move together, so a reader can always divide.

## 6. No new announced log marker — the WARN line was written and then removed

I first added `n.logf(ports.LogWarn, "repair bounty paid to SELF", …)` at the A4-2 site.
§1.4 forbids it: everything here reaches an operator through documents already registered
in `cmd/silt/observable_contract.go`, and a marker with no reader is an S5 contract with no
reader. Removed. The counter is on the `stats` block, which is where the canary reads it.

## What I did NOT build, and why

- **No `(fetcher × object)` join anywhere.** Every counter is node-wide with no identity
  axis, or per-object with no fetcher axis. A4-3 reads `account.fetchedBytes`, a scalar
  already on the account; it never learns *which* object the repairer fetched. Don't #3
  holds by construction, not by review.
- **No network-wide coverage or network-wide escrow recovery** (§1.4) — gossip-estimated,
  and they belong to C3.
- **No `bountyPaidToEscrowFunder`** — §1.0 point 2 shows it is degenerate.
