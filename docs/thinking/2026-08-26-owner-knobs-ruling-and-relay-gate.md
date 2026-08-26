# The PE ruling on the three owner knobs — guards attached, and the relay gate answered

**Date:** 2026-08-26. **Ruling:**
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-PoD-keystone-owner-knobs-2026-08-26.md`
(answers `PoD-keystone-owner-knobs-CONSULT-2026-08-26.md`). The PE concurs with
the builder's read on all three knobs, verifies the load-bearing premises
against HEAD itself, and attaches guards. **The knobs remain Andrew's calls** —
this note records what shipped in response and answers the one open question
the ruling routed back.

## What the ruling changed in my understanding

Two reframings worth keeping:

1. **Knob 3 is not a tidiness win, it is load-bearing for the keystone.**
   I argued TTL-lapse as "not paying rent on an empty room." The PE is
   sharper: lifetime-owner puts a *forever-growing term in every snapshot*,
   which directly defeats the bounded-committed-state property the keystone
   exists to deliver. Lifetime-owner quietly reintroduces the unbounded-state
   problem the whole D-TIERING pivot is meant to solve. That moves knob 3 from
   "optional bounding opportunity" to "required for coherence."
2. **Knob 1's positive case is a cross-tier funding loop, not just consistency.**
   Escrow-routing means hot content served on the edge generates skim that
   funds *that content's* durability on the persistent tier — edge delivery
   financing persistent retention. Burn destroys that flow. I had defended
   escrow on "one mechanism"; the alignment argument is stronger.

## Guards shipped this change (build-immutable #2)

- **Knob 1 — `TestPaidBountyIsNotRecoverableBySupersede`** (`core/credit/delivery_test.go`).
  Escrow-routing is safe *because* the supersede reversal floors at the
  remaining reserve, so a bounty already paid for real repair work is never
  clawed back. The PE's point: if that floor silently regressed, escrow would
  begin minting recoverable balance and burn would become the correct routing.
  Now it cannot regress silently.
- **Knob 3 — `TestRootOwnerFeedsOnlyTheDedup`** (`core/credit/bond_reputation_test.go`).
  Pins that `rootOwner` feeds the F1 dedup and nothing else: both slash paths
  dock the identity they name, identically whether or not it owns a bond root.
  A future reader that assumes lifetime binding now fails loudly here.
- **Knob 3(b) — already pinned, cited not duplicated.** The anti-griefing
  property TTL-lapse rests on (an outsider cannot answer challenges on a root
  it did not seal) is `core/bond` `TestRedteamG2_PlotBoundToClaimedIdentity`.
  Verified present and green.

## The relay gate (knob 2) — answered with evidence, routed for certification

The PE made knob 2 contingent on one question and said it decides the knob more
than the trust-surface debate does: **is the relay-increment dispute
signature-verifiable (→ do the quorum-TTP) or does it need verifiable-escrow
crypto (→ defer, the calculus flips)?**

**The evidence says signature-verifiable, because the certification already
forbids the protocol from adjudicating the other thing.**

The heavy-crypto requirement documented in `core/demand/fairexchange.go` is for
a specific dispute: proving *the correct content bytes genuinely reached the
fetcher* without the fetcher's cooperation. That is why it needs verifiable
escrow of the content key plus threshold decryption — the disputed fact is an
unwitnessed physical delivery.

The relay dispute cannot be that dispute. The PoD certification (Q3) already
rules that **no transit proof exists to buy** — Tor's proof-of-bandwidth line
failed at exactly this, and endpoint attestation collapses under endpoint
collusion. So a relay protocol may never adjudicate "did you actually
forward?"; the certified direction is a sender-funded incremental
micropayment where the only adjudicable quantity is **the payment chain
itself** — "the fetcher stopped paying after increment N." A hash-chain
preimage (PayWord) or a signed per-increment token is *self-verifying*: cheap,
pure-Go, and mechanical in exactly the way the PE's sharpening B describes
(verify self-verifying evidence, like equivocation slashing — not a
discretionary judgment).

**So the calculus does not flip, and the roadmap's unbounded crypto unknown
is NOT reactivated by choosing the quorum-TTP direction** — provided the relay
protocol keeps its dispute scoped to payment, never to transit. That scoping
is not a nice-to-have; it is what keeps the cheap answer true.

**Status: builder evidence, not certified.** This is the input the relay
follow-on consult must carry as its first question, exactly as the PE
prescribed (coupling 3: do not let relay compensation quietly commit the
project to Camenisch–Shoup-class crypto). Research certifies; this note names
the finding and its reasoning so the consult starts from evidence.

## Coupling 2, noted for the next slice

The PE asks that the relay credit, when it lands, join the *same* firewall test
regime as the delivery credit rather than growing a second one — one
failing-first firewall regime for the whole neutral lane. Recorded here so the
relay slice inherits it.
