# 2026-08-16 — building the certified #441 fix: entries become mempool content

## The gate that cleared

`441-publish-starvation-RESEARCH-CERTIFICATION-2026-08-16.md`: **CERTIFY (A)**, design
pinned. Entries stop being a second proposal stream and become **mempool content** the
single `(h,r)` designee's block folds in — the leader-carries-the-mempool shape every
mature BFT SMR uses. Rejected: (B) alternating seats (keeps two streams), (C) folding
regs into entries (doesn't fix arrival order). Two additions beyond the consult: a
**separate** `MaxEntryBytesPerBlock` (a ~1.5 MB reg fills the whole reg cap — without a
reserved entry slice the starvation reappears one layer down; and the dual: an entry
flood must not crowd out consensus-critical renewals) and **FIFO-by-submit-height** (no
fees ⇒ no priority order that could defer an entry forever).

## Build order chosen (and why)

1. **The §7 discriminator FIRST** (cheap, shapes claims): drain designee at a height
   with no entries contending — commits at r0? (a) ⇒ the field's every-height escape
   was entry contention and (A) recovers the happy path as an M1 bonus; (b) ⇒ separate
   defect, its own issue, never buried under M1.
2. **Core mechanism** (ports + node): `MsgSubmitEntry` mirror — validate-on-arrival
   (dup-root/manifest/publisher/token/serial), never refuse silently, FIFO
   `pendingEntries` dedup'd by root; the designee folds entries under the new separate
   budget; `maybeAdvanceRound` arms on entry work (the launch-face fix).
3. **Oracles** (the merge gate): the born-RED starvation oracle reshaped to the
   certified submit-then-poll client shape → GREEN; the five §6 siblings; all six over
   `matureWorld` / the launch fixtures.
4. **Client flow**: the registry/chainhost publish path switches from
   propose-then-gather to **submit-then-poll-finality** (B7/S3 preserved: the link
   still returns only on finality; the 202+poll HTTP surface already exists from
   #286 L1, so the change is the registry's backend, not the CLI's contract).
5. **Docs**: I4 refined to *operation-liveness* ("no legitimately submitted operation
   is permanently starved") per §8 — the invariant map gains the product-layer
   statement its chain-layer statement hid behind; CHANGELOG.

## Decisions inside the pinned design (the freedom the certification leaves)

- `pendingEntries` is an ordered slice of `{entry, submitHeight}` dedup'd by root
  (a re-submit of the same root refreshes nothing — FIFO position keeps the original
  submit-height, so resubmission cannot queue-jump).
- `MaxEntryBytesPerBlock` default: entries are tens of bytes; 64 KiB admits ~hundreds
  per block while keeping reg+entry sum comfortably WAN-gatherable next to the 2 MiB
  reg cap (#286-L2b). An M1 tuning knob per the certification's caveat (3), wired like
  `MaxBondRegBytesPerBlock` (flag + config), not a consensus rule.
- Submit targets: `syncTargets() ∩ ProposerEligible` — the same reachable-eligible
  intersection the drain uses; fire-and-forget, client polls.
- Mempool TTL: entries whose token serial is spent or whose root committed are dropped
  at fold-time re-validation (the reg queue's discipline); no wall-clock TTL enters
  (B2/#3).

## What is explicitly out of scope

The O(f+1) fairness bound ships as an **owned, bounded residual** with its oracle
(adversarial-designee drop) — "no *permanent minority* censorship", handed to the red
team, per the certification §4. `MaxEntryBytesPerBlock`'s value is M1 tuning. #299
remains the structural shrink of the reg side.
