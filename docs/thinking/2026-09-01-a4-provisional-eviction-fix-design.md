# A4 provisional-eviction money-pump — fix design (Boulder 0)

Date: 2026-09-01
Author: Builder seat
Status: DESIGN ONLY. No `.go` file is touched by this document.
Gate: this edits the B3 conservation mechanism (an economic-mechanism change).
It needs a Researcher certification before it lands. See "Research gate" below.

## Decision input (carried, not re-litigated here)

Rule ratified by two independent advisories (Researcher confirmation pending):

> **(b) — one delivery, one payment.** An evicted provisional lane must NEVER
> cause a re-mint, and a legitimate late receipt must NEVER be denied.

Rejected alternatives, and why:

- **(a) deny / claw-back.** Denying a redeem whose provisional lane was evicted
  punishes honest high-volume edge nodes (the pony/horse tier the economy relies
  on) and hands an attacker a griefing lever: flood a victim's lanes out of the
  window, then the victim's legitimate receipts get denied. Rejected.
- **(c) pay-anyway.** The current LIVE behavior. The evicted lane keeps its eager
  self-mint AND the redeem pays the conserved leg. One delivery paid twice. This
  is the money pump. Rejected.

## The bug — verified against the code (evidence)

The failure is a double-pay of one delivery because the eager self-mint is
retained after eviction while the redeem still pays the conserved leg.
Attributed mechanism, with `file:line`:

1. `core/credit/escrow.go:130` — `RecordServeToObject` eagerly self-credits the
   server `s.balance += bytes - skim`. This is a MINT: no counterparty balance is
   debited. It is the pre-PoD "1 credit/byte" self-record.
2. `core/credit/escrow.go:139` — the serve records the lane via
   `trackProvisional(requester, root, bytes-skim, skim)`, storing the minted
   `net` and the escrow `skim` so a later receipt can reverse them.
3. `core/credit/delivery.go:67-71` — at `maxProvisional = 8192`
   (`delivery.go:43`) the tracker FIFO-evicts the oldest un-redeemed lane
   (`delete(l.provisional, old)`), by design, to stay bounded (build-immutable
   #8, B2 no-map-iteration). The evicted lane's mint is now UNTRACKED.
4. `core/credit/delivery.go:99` — `RedeemDeliveryCredit` reverses the self-mint
   ONLY `if p, ok := l.provisional[k]; ok`. For an evicted lane the lookup misses,
   so the mint is NOT reversed.
5. `core/credit/delivery.go:116-125` — the redeem then unconditionally pays the
   conserved leg `s.balance += fee - skim` (the fee the fetcher prepaid at
   `ChargePublish`, `credit.go:311`, is a genuine transfer).

Result for an evicted-then-redeemed lane: the server keeps `bytes - skim`
(minted) PLUS receives `fee - skim` (conserved). The conserved leg is honest.
The retained mint is the leak. The current test
`TestProvisionalCapIsBoundedAndDeterministic` (`delivery_test.go:207-238`)
ASSERTS this leak as "the bounded, documented residual" — that test encodes
rule (c) and must be replaced by a rule-(b) assertion.

The escrow-skim half of the mint is separately floored: the redeem reversal of
the skim is clamped to what the reserve still holds (`delivery.go:101-111`),
which is what `TestPaidBountyIsNotRecoverableBySupersede` (`delivery_test.go:169`)
protects. That floor must survive any fix here.

## What "one delivery, one payment" requires, precisely

The mint at serve time is provisional. Exactly one of two terminal states must
hold per lane, and the sum of what the server keeps must equal one payment:

- **Never redeemed (bilateral fallback):** the server keeps the self-mint
  `bytes - skim`. One payment. Unchanged, legitimate.
- **Redeemed (witnessed):** the server keeps the conserved `fee - skim` and the
  self-mint is reversed. One payment.

The bug is a THIRD state — evicted-then-redeemed — where the server keeps both.
Rule (b) says: in that third state, pay the conserved leg and DO NOT re-mint. The
mint is already on the server's balance from serve time; "do not re-mint" means
the redeem must not ADD the conserved leg on top of an unreversed mint. The fix
must make the redeem either (i) reverse the mint before paying conserved, or
(ii) never eager-mint in the first place, so there is nothing to double.

## Candidate shape (b)-minimal — settle evicted lanes to "conserved only"

Keep the map bounded. Make an evicted lane, on redeem, pay the conserved leg but
first neutralize its retained mint — without an unbounded map.

### The distinguishing problem

The redeem must tell three cases apart:

- **In-window lane** (`provisional[k]` present): reverse mint, pay conserved.
  Already correct.
- **Evicted-legit lane** (a serve happened, its lane was FIFO-evicted): the mint
  is on the balance, untracked. Must pay conserved WITHOUT leaving the mint.
- **Never-existed lane** (no serve ever happened for this `(fetcher, root)`): no
  mint to reverse. Pay conserved only. (A receipt with no matching serve is
  possible — the redeem side has no proof a serve occurred; it trusts the banked
  receipt. Today this path pays conserved and reverses nothing, which is correct.)

The evicted-legit and never-existed cases are INDISTINGUISHABLE at redeem time
once the lane is gone, because the only record that a serve happened WAS the
evicted map entry. This is the crux: (b)-minimal cannot recover the per-lane mint
amount after eviction without keeping per-lane state, which is exactly the
unbounded map we are forbidden.

### The move that makes it work: evict by REVERSING, not by forgetting

Change eviction from "forget the lane, leave the mint" to "reverse the lane's
mint at eviction time, then forget it." When a lane is FIFO-evicted
(`delivery.go:67-71`), before `delete`, apply the same reversal the redeem would
apply: debit the server's balance by `p.net`, and reduce the object's escrow by
`p.skim` FLOORED at the reserve (identical to `delivery.go:101-111`).

After eviction the lane is in the SAME accounting state as "never served": the
mint is gone, the skim is floored-reversed, the map entry is gone. Then:

- Evicted-legit redeem == never-existed redeem: both pay conserved only, reverse
  nothing. Indistinguishable AND correct — because eviction already undid the
  mint. One delivery, one payment.
- The bilateral fallback for an UNREDEEMED evicted lane changes: the server no
  longer keeps the serve-time mint after eviction. That is the cost of this shape
  (see trade-off). Under rule (b) it is acceptable: the alternative is the leak.

Eviction reversal is deterministic (it acts on `provOrder[0]`, no map iteration —
B2 preserved). The map stays capped at `maxProvisional` (build-immutable #8
preserved). No new durable state.

### Escrow floor preservation

The eviction reversal reuses the redeem's floored-skim logic verbatim: reduce
`e.balance` by `min(p.skim, e.balance)`, reduce `e.funded` symmetrically, clamp
`funded >= 0`. A bounty paid out between serve and eviction is never clawed back,
same as between serve and redeem. `TestPaidBountyIsNotRecoverableBySupersede`
stays true and non-vacuous (it exercises the redeem path, untouched here). Add a
sibling test that a bounty paid between serve and EVICTION is likewise
non-recoverable, so the new reversal site inherits the same floor guard.

### Cost of (b)-minimal

- One added reversal block at the eviction site, ~10 lines, mirroring the redeem
  reversal. No new fields, no new map, no port change.
- Semantic change to the unredeemed-evicted bilateral fallback: a high-volume
  server whose unwitnessed serve is evicted LOSES its self-record credit for that
  serve. This under-pays honest bilateral serves at extreme volume (>8192
  concurrent un-redeemed lanes on one node). It never over-pays and never denies a
  witnessed receipt, so it satisfies rule (b). The economist should weigh whether
  the bilateral-fallback under-pay at the tail is acceptable; the counters below
  make it observable.

## Candidate shape (b)-full — witness cross-operator serves, no eager mint

The deeper fix the `delivery.go:27-34` comment already names: "witnessing all
cross-operator serves would subsume the self-mint." Remove the eager self-mint
entirely so there is nothing to double-pay.

### Mechanism

`RecordServeToObject` stops crediting `s.balance += bytes - skim` at serve time.
The serve records the delivery as an unpaid, witnessable claim (bytes, requester,
root) — the same lane identity — but mints nothing. Payment happens ONLY on one
of two terminal events:

- **Witnessed:** a banked receipt redeems the lane → pay the conserved
  `fee - skim` (as today). This is the cross-operator case: the fetcher's prepaid
  fee funds it. Conservation holds by construction — the only credit that moves is
  the transfer of the prepaid fee.
- **Unwitnessed bilateral fallback:** if silt still wants to pay an unwitnessed
  serve at all, it needs a FUNDED source for it, not a mint. Options: (a) drop the
  unwitnessed payment entirely (only witnessed deliveries pay — simplest, but
  changes the incentive for serving to peers who never bank a receipt); or (b)
  fund it from a real counterparty debit (the requester pays at serve time),
  which requires the requester to be online and solvent at serve time — a protocol
  change to the serve path.

With no eager mint, eviction can forget freely: an evicted lane has no mint to
retain, so its later redeem pays conserved once and there is nothing to double.
The bounded FIFO map becomes a pure de-dup / supersede aid, not a mint ledger, so
even a fully-forgotten lane is safe.

### Cost of (b)-full

- Touches the serve path and its callers in `core/node` (the wiring the
  escrow.go:22-26 comment defers). Bigger blast radius than (b)-minimal.
- Forces a decision on the unwitnessed bilateral fallback (drop it, or fund it
  from a real debit). That is itself an economic-mechanism decision (it changes
  who pays for a peer-to-peer serve with no banked receipt) → Researcher gate.
- Requires the receipt/witness path to be the PRIMARY settle path in practice, or
  honest serving to peers who never bank goes unpaid. Depends on PoD §7.3
  transport maturity (the receipt-banking path).

## Recommendation

**Land (b)-minimal now; keep (b)-full as the tracked deeper fix.**

Trade-off, stated plainly:

- (b)-minimal closes the LIVE money pump with a ~10-line, self-contained change
  in `core/credit`, no port or serve-path change, and it preserves the escrow
  floor by reusing the proven reversal. It is the simplest thing that makes the
  conservation invariant hold. Its only give is under-paying the unwitnessed
  bilateral fallback at the >8192-lane tail — an under-pay, never an over-pay,
  never a denial — which the exported counters surface.
- (b)-full is the correct end state (no mint to double, ever) but its blast radius
  is the serve path and its callers, and it forces a second economic decision on
  the unwitnessed fallback. Doing it now bundles a mechanism-removal with a
  live-leak fix; that is churn on the critical path. Sequence it after the leak is
  closed and after the PoD §7.3 receipt-banking path is the primary settle route,
  so dropping/re-funding the unwitnessed fallback is safe.

Both directions edit B3 conservation and both need the Researcher cert. File the
cert request for the DIRECTION (b)-minimal-now / (b)-full-later, so the deeper
fix is pre-blessed and does not re-open the question later.

## Exported counters (economist ask)

Two per-node counters, exported through the existing observability surface (same
shape as `Audits`, `servedBytes` accessors — a plain `Ledger` method returning an
int, read-only, never touching Reputation):

- **`evicted_unredeemed_lanes`** — incremented at the eviction site
  (`delivery.go:67-71`) each time a lane is FIFO-evicted before any redeem. Under
  (b)-minimal this counts honest bilateral serves that lost their self-record at
  the tail. It is the "am I running the map too small / is a node under load"
  signal. Expected nonzero only under extreme lane pressure.
- **`redeems_on_evicted_lanes`** — incremented in `RedeemDeliveryCredit` whenever
  a redeem arrives for a `(fetcher, root)` that has no live provisional entry AND
  a serve for that lane was previously evicted. **Under a correct (b) fix this is
  STRUCTURALLY ZERO for the double-pay sense** — because eviction already reversed
  the mint, a redeem on an evicted lane pays conserved-only, minting nothing. A
  NONZERO double-pay value is the regression alarm.

  Implementation note for the counter to be meaningful: distinguishing
  "evicted lane" from "never-existed lane" at redeem time is exactly the
  indistinguishability the fix resolves by reversing-at-eviction. So this counter
  cannot cheaply separate the two after eviction WITHOUT extra state. Recommended:
  count it at the ALARM boundary instead — increment only if a redeem's reversal
  would have found a non-zero retained mint that eviction failed to clear. Under a
  correct fix that condition is unreachable, so the counter is a pure invariant
  tripwire: any increment means the eviction-reversal regressed. This keeps the
  counter's "structurally zero" contract exact without a per-lane audit map.

## Failing-first regression gate (the money-pump conservation gate)

New test in `core/credit/delivery_test.go`. It must go RED on current main and
GREEN after the fix.

Shape:

1. `l := New(fee, grant)` with a known `grant` for a fixed set of server nodes,
   `grant = 0` for the fetcher pool if using prepaid fees (mirror the existing
   tests' setup; fund publishers enough to pay the fee).
2. Record the initial total: `initial = Σ balances + Σ escrow` across all
   accounts and all object reserves. This is the closed-system conservation
   quantity (credit is neither minted nor burned by serve+redeem under rule (b)).
3. Flood distinct lanes past `maxProvisional` so lane 0 (and more) are FIFO-evicted
   (reuse the flood loop from `TestProvisionalCapIsBoundedAndDeterministic`,
   `delivery_test.go:223-226`).
4. Redeem an EVICTED lane (lane 0) with a valid receipt (`RedeemDeliveryCredit`).
5. Assert `Σ balances + Σ escrow == initial` (conservation), MINUS only the
   genuine prepaid-fee transfers that entered from `ChargePublish` — i.e. account
   for the fee legs explicitly so the assertion isolates the mint. Concretely:
   the closed quantity to pin is `Σ balances + Σ escrow` where every credit that
   appears was either granted at `New`/`Register` or moved by a real transfer.
   Under (c) it is `initial + (bytes - skim)` for the evicted redeemed lane (the
   leaked mint). Under (b) it equals `initial` adjusted only by prepaid fee
   transfers.
6. Assert `redeems_on_evicted_lanes == 0` (the structural-zero tripwire).

RED on main: the retained mint from step 4 makes the sum exceed `initial` by
`bytes - skim`. GREEN after (b)-minimal: eviction reversed the mint, so the redeem
pays conserved only and the sum is conserved.

Companion changes to existing tests:

- **Replace** `TestProvisionalCapIsBoundedAndDeterministic`
  (`delivery_test.go:207-238`). Its current assertion (evicted redeem pays
  conserved WITHOUT reversal, leaving the mint) encodes rule (c) and is the
  double-pay. Rewrite it to assert the (b) behavior: evicted redeem pays conserved
  AND the pre-existing mint has been cleared by eviction, so total credit is
  conserved. Keep its bound/determinism assertions (map never exceeds cap; FIFO
  order deterministic).
- **Keep** `TestPaidBountyIsNotRecoverableBySupersede` (`delivery_test.go:169`)
  unchanged; it must stay non-vacuous. Add a sibling asserting the escrow floor
  also holds at the new EVICTION reversal site (bounty paid between serve and
  eviction is not clawed back).

## Constraints check

- Provisional map stays bounded and non-durable (cap `maxProvisional`, FIFO, no
  map iteration). Build-immutable #8 and B2 preserved. No unbounded-state DoS.
- Escrow reversal floor preserved (reused verbatim at the new eviction site;
  `TestPaidBountyIsNotRecoverableBySupersede` stays true and non-vacuous).
- Standing untouched: the fix moves only balance-economy credit; no field it
  touches is read by Reputation (Invariant-A guard remains green).

## Research gate (mandatory before landing)

This changes the B3 conservation mechanism — an economic-mechanism change under
the silt research gate (`.claude/CLAUDE.md`, "Research gate"; MEMORY: "The banned
dual: any network-minted per-receipt subsidy = money pump"). The Builder shapes
the question; the Researcher certifies; the human ratifies. Do NOT land either
shape on a verbal summary.

Cert request should ask the Researcher to certify:

1. That reversing the mint AT EVICTION preserves conservation for all three
   terminal lane states (never-redeemed, in-window-redeemed, evicted-redeemed)
   and introduces no new money pump.
2. That the escrow floor at the new eviction reversal site is sound (no clawback
   of paid bounties, no negative reserve).
3. The DIRECTION verdict: (b)-minimal now, (b)-full later — so the deeper fix is
   pre-blessed and does not re-open the conservation question.
