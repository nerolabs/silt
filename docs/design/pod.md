# Proof-of-Delivery (Phase 4) — the neutral lane, specified

> **Status: DRAFT — research-gated.** This is the Phase 4 opening move the
> ROADMAP orders: spec + consult **before** any build. The consult is filed at
> `silt-reviews/research/PoD-neutral-lane-B3-close-CONSULT-2026-08-26.md`
> (Q1–Q5). Nothing below is wired until the consult certifies §5.
> Relates to: D-DEMAND ([decisions.md](../decisions.md)), D-TIERING §8 (the PE
> direction memo, 2026-08-25), owned residual **B3**
> ([design/owned-residuals.md](owned-residuals.md) §B3), the γ→1/N firewall
> (#182, [design/m0.md](m0.md) §10), deliberation
> [`thinking/2026-08-26-phase4-pod-opening.md`](../thinking/2026-08-26-phase4-pod-opening.md).

## 1. Purpose

Storage is priced; bandwidth is not. An operator who relays or gateways for the
network today is a free rider's choke point — the recentralization failure mode
Phase 4 exists to close. PoD prices delivered bytes so that serving strangers is
compensated, cheaply enough to run on a transient edge box.

**Two forms, one firewall (D-DEMAND / D-TIERING §8):**

| Form | What a receipt buys | Gate |
|---|---|---|
| **Neutral** (this spec) | Balance-lane credit — bandwidth/relay compensation | This consult |
| **Strong** (out of scope) | Consensus standing | Verifiable-escrow crypto **and** #182 sealing — both open |

Delivery credits fund durability and compensation, **never consensus standing**.
The firewall is structural (`core/credit/credit.go:282`, Invariant A) and this
spec does not touch it.

## 2. What exists (verified at HEAD `d9635c4`)

| Piece | Where | State |
|---|---|---|
| Receipt engine: blind withdraw → PoR-bound ack → bank → redeem | `core/demand/demand.go` | Built (#181), inert — `EnableDemandBank` has no production caller |
| Wire messages `MsgDeliveryReceipt`/`Ack` | `ports/net.go:149`, dispatched `core/node/node.go:1567` | Wired |
| Per-byte serve credit (1 credit/byte, 1/8 skim to the object's escrow) | `core/node/node.go:1543-1545`, `core/credit/escrow.go:117-135` | Live — but **self-recorded**, per-node ledger |
| Self-serve guard | `core/credit/credit.go:131-134` (`server == requester` earns nothing) | Live |
| Cost-to-wash levers: fee at withdrawal (P3a), bonded-fetcher credential (P3b) | `demand.go:82-87`, `Bank.RequireBondedFetcher` | Built |

The neutral lane is therefore **not a new payment**. It is the *witnessed* form
of the existing serve credit: today a node credits itself when it serves;
nothing attests it. Witnessing matters exactly where the payer and payee are
different operators — relay compensation, edge tit-for-tat, and any credit
another node must honor.

## 3. The design (neutral lane)

The flow reuses the built engine end-to-end; the only new element is the
consumer and its invariant.

1. **Withdraw.** A fetcher blind-withdraws a retrieval token from an issuer,
   paying the retrieval fee at withdrawal (`SignWithdrawal`'s documented
   caller obligation). The issuer never learns the serial.
2. **Fetch + ack.** The fetcher retrieves the object, re-verifies it against
   its content address (tenet B3, the existing read path), and sends the
   server a `DeliveryReceipt`: token spend + fetcher signature + the
   (serial‖object‖server)-bound proof.
3. **Bank + redeem.** The server banks the receipt and redeems it for a
   **balance-lane delivery credit**.
4. **The conservation invariant (the new rule, load-bearing):**

   > **No credit is minted by a receipt.** A redeemed receipt only *moves*
   > value that the fetcher already paid in (the burned/escrowed retrieval
   > fee funds the compensation pool; the credit to the server is drawn from
   > it, less the durability skim). The network pays no per-receipt subsidy.

## 4. Closing B3 on paper (the prerequisite)

**The residual (B3):** a receipt's PoR leg is forgeable with zero object bytes,
because the per-object PoR key seed is public (`demand.go:104-110` — the proof
certifies "bytes consistent with C's demand key," not "the unique bytes of C").
B3 is inert today only because demand has no consumer. §3 wires a consumer, so
B3 must close first. The close has two legs.

### 4.1 The economic leg (primary): conservation makes forgery a self-payment

What can an adversary mint with a zero-byte forged receipt? The other two
bindings still hold: the receipt requires a real token (fee paid at
withdrawal) and a fetcher signature (only the token's spender can mint it,
`demand.go:145-149`). So the forger is a colluding fetcher+server pair that
paid a real fee and skips only the byte transfer. Under the §3 invariant the
pair's best outcome is moving its own fee back to itself **minus the skim** —
a strict loss per loop, identical to the honest self-serve case the ledger
already blocks (`credit.go:131-134`). Forgery is not free minting; it is
buying your own money back at a discount to yourself of `SkimNum/SkimDen`.

The exposure this leg does **not** cover, and therefore the spec **bans**: any
network-minted per-receipt subsidy (a reward funded by anyone other than the
fetcher's own payment). A subsidy converts the forgery into a money pump. If a
future design wants delivery subsidies (e.g. repair-pool-funded cold-content
serving), that is a new consult, not a parameter tweak.

### 4.2 The crypto leg (secondary): is the PoR binding load-bearing at all?

The fetcher verified the bytes against the content address before acking (the
existing read path re-verifies every fetch). Its signature is already an
attestation of correct delivery by the party best placed to know. The PoR
proof adds "the fetcher held the bytes at ack time" — but a colluding fetcher
signs anything, so the PoR leg deters no collusion; and an honest fetcher's
signature needs no PoR to back it. The spec's working position: **in the
neutral lane the PoR leg is belt-and-suspenders, not load-bearing; the
load-bearing bindings are token spend + fetcher signature + conservation.**

If research disagrees, the named close is the H7-style **content-committed
recompute floor**: bind the ack challenge to Merkle samples verified against
the object's content address (no secret key; unforgeable without the bytes;
pure Go). Secret-keyed tags are the fallback, with the key-custody question
that made H7 choose recompute. Either way, per build-immutable #8, the chosen
mechanism's produce+verify cost is measured on the floor box before commit.

### 4.3 What stays open (unchanged by this spec)

Demand **authenticity** is a Douceur limit (owned residual B2): a self-fetch is
a real delivery, and no receipt proves the counterparty was independent. The
neutral lane does not need authenticity — it needs wash to be unprofitable
(§4.1) — and the strong form remains gated on #182 regardless.

## 5. What the consult must certify (Q1–Q5)

Filed as `PoD-neutral-lane-B3-close-CONSULT-2026-08-26.md`; summarized:

1. **Q1 — the conservation close.** Is §4.1 sound as the B3 economic close at
   stated parameters (fee routing, skim), including the interaction with the
   existing per-byte serve credit (double-payment risk: a witnessed receipt
   *and* a self-recorded `RecordServe` for the same bytes)?
2. **Q2 — the PoR leg.** Is §4.2's working position right? If the leg is
   load-bearing, which close (recompute floor vs secret-keyed tags) and at
   what sampled-block parameters?
3. **Q3 — relay compensation.** A relay is content-blind: it cannot verify
   against a content address, so §4.2's fetcher-verification argument does not
   transfer. What receipt shape prices relay bytes without teaching the relay
   what it carries (T3/B4)?
4. **Q4 — the strong-form desk study** (the PE-recommended 1–2 day study,
   folded in): is there an adoptable, audited, pure-Go-or-acceptably-vendored
   verifiable-escrow primitive (Camenisch–Shoup class)? Verdict on strong-form
   tractability; no code dependency.
5. **Q5 — settlement consistency.** The balance lane is per-node bookkeeping
   (the #586 arming question is field evidence of divergent views). Does
   tit-for-tat compensation need chain-committed settlement, or does per-node
   suffice — and how does this interact with the state-root keystone consult
   (registry state root), which is the natural home for any committed balance?

## 6. Non-goals

- **No standing fusion.** Strong-form PoD stays double-gated (Q4 crypto and
  #182). Coupling D-TIERING §5.3 holds: contribution scales publishing
  allowance and compensation, never consensus weight except through the bond.
- **No new crypto in the neutral lane** unless Q2 forces the recompute floor.
- **No delivery subsidies** (§4.1 ban) — conserved transfers only.

## 7. Build order after certification (for scale, not commitment)

1. Wire `EnableDemandBank` + the balance-lane consumer under the certified
   invariant, with the failing-first firewall test (a big deliverer's
   `Reputation()` is unchanged across the reward — the Phase 2 Invariant-A
   guard pattern).
2. The D-TIERING near-term flags (`--serve-content`, `--archive`) — build-gated
   only, unblocked once this spec is certified.
3. Relay compensation per Q3's answer.
