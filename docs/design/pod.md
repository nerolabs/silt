# Proof-of-Delivery (Phase 4) — the neutral lane, specified

> **Status: CERTIFIED — 2026-08-26, with amendments.** Research answered all
> of Q1–Q5
> (`silt-reviews/research/research-outcome/PoD-neutral-lane-B3-close-RESEARCH-CERTIFICATION-2026-08-26.md`):
> the conservation close is sound and is the *only structural* wash defense
> the literature knows (sender-funded transfer, never a mint). Three
> amendments are folded into the text below and marked **[CERT]**:
> 1. **The supersede rule is load-bearing, not additive** (§3.5): the
>    existing `RecordServe` per-byte credit is an *unfunded self-mint* — the
>    exact subsidy §4.1 bans — so the witnessed receipt must supersede it.
>    Do not ship the consumer without this.
> 2. **The PoR leg is DROPPED in the neutral lane** (§4.2): a forgeable
>    proof deters no collusion and costs 128 SW samples per delivery on the
>    floor box. It re-enters (as the content-committed recompute floor) only
>    where loss-deterrence stops covering: strong form and relay.
> 3. **Strong-form Camenisch–Shoup is NOT adoptable** (§6): the only pure-Go
>    implementation is archived and unaudited. The silt-native strong-form
>    path, if ever pursued, is quorum-TTP adjudication + threshold
>    decryption (drand/kyber, audited-grade) — no new hardness assumption.
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
The firewall is structural (`core/credit/credit.go:290`, `bondUnit`/`Reputation`,
Invariant A) and this spec does not touch it.

## 2. What exists (verified at HEAD `d9635c4`)

| Piece | Where | State |
|---|---|---|
| Receipt engine: blind withdraw → PoR-bound ack → bank → redeem | `core/demand/demand.go` | Built (#181), **LIVE** — production caller `cmd/silt/daemon.go:810` (behind `--accept-receipts`); redeem wired `core/node/demandrole.go:190` → `core/credit/delivery.go:89` |
| Wire messages `MsgDeliveryReceipt`/`Ack` | `ports/net.go:149`, dispatched `core/node/node.go:1567` | Wired |
| Per-byte serve credit (1 credit/byte, 1/8 skim to the object's escrow) | `core/node/node.go:1543-1545`, `core/credit/escrow.go:117-135` | Live — but **self-recorded**, per-node ledger |
| Self-serve guard | `core/credit/credit.go:169` (`server == requester` earns nothing) | Live |
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

   **[CERT] Certified with the exact soundness boundary:** the requirement
   is `credit ≤ fee` (conservation), full stop. `fee > 0` and `skim > 0`
   make the wash loss *strict* rather than break-even — deterrent floors,
   not soundness ones. Do not raise the skim for anti-wash reasons; it
   taxes honest delivery (build-immutable #4) and conservation already
   carries soundness. Escrow-routing the skim is certified safe (worst
   case break-even via a real self-repair, never a pump); pure burn is the
   airtight-deterrent option. **Decided: escrow** (D-POD-KNOBS, 2026-08-26);
   its safety rests on the supersede reversal flooring at the remaining
   reserve, regression-locked by
   `TestPaidBountyIsNotRecoverableBySupersede`.

5. **[CERT] The supersede rule (load-bearing, required before the firewall
   test means anything).** The serve path already self-mints 1 credit/byte
   with no debit anywhere (`RecordServe`, `credit.go:168`) — an
   unfunded mint that is precisely the banned per-receipt subsidy once a
   witnessed receipt pays for the same bytes. Certified rule: **a delivery
   paid by a redeemed receipt is never also self-credited**, deduped by the
   delivery identity. Two implementations, (ii) preferred where the node
   knows at serve time a receipt is expected: (i) provisional self-record
   on serve, reversed and replaced by the conserved credit on redeem — the
   robust general form; (ii) no self-record on a witnessed-lane serve;
   self-record remains only as the unwitnessed bilateral fallback. The
   observable has the same split: `bumpDemand`'s self-count and
   `WitnessedDemand` stay separate surfaces, and any consumer reads exactly
   one.

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
already blocks (`credit.go:169`). Forgery is not free minting; it is
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
signature needs no PoR to back it.

**[CERT] Certified, and sharpened: the PoR leg is DROPPED in the neutral
lane.** Research's ruling: a *forgeable* belt is not belt-and-suspenders — a
colluder cuts it for free — and it is not free to wear (`SampleCount = 128`
Shacham–Waters prove per `Ack` + verify per `Redeem`, per delivery, on the
1 vCPU / 2 GB box). The neutral-lane receipt is **token + fetcher signature
+ the (serial‖object‖server) binding**; token-level unforgeability survives,
and conservation makes forgery unprofitable regardless.

The possession binding re-enters exactly where loss-deterrence stops
covering: **(1) the strong form** (a forged receipt would buy standing worth
more than the fee) and **(2) relay** (the relay is content-blind, so the
fetcher-verification argument does not transfer). There, the certified shape
is the H7-style **content-committed recompute floor** (Merkle samples bound
to the content address — no secret key, unforgeable without the bytes, pure
Go) over secret-keyed tags (key custody, the problem H7 already rejected).
Per build-immutable #8, its produce+verify cost is measured on the floor box
before commitment — a strong-form/relay parameter, not a neutral-lane one.

### 4.3 What stays open (unchanged by this spec)

Demand **authenticity** is a Douceur limit (owned residual B2): a self-fetch is
a real delivery, and no receipt proves the counterparty was independent. The
neutral lane does not need authenticity — it needs wash to be unprofitable
(§4.1) — and the strong form remains gated on #182 regardless.

## 5. The consult verdicts (Q1–Q5, certified 2026-08-26)

Consult `PoD-neutral-lane-B3-close-CONSULT-2026-08-26.md`; certification
`research-outcome/PoD-neutral-lane-B3-close-RESEARCH-CERTIFICATION-2026-08-26.md`.

1. **Q1 — the conservation close: SOUND, and the correct primitive** —
   independently corroborated by the wash-trading literature (sender-funded
   transfer flips the attacker's payoff sign; volume-minted rewards
   *incentivize* wash). Soundness = `credit ≤ fee`; `fee > 0` / `skim > 0`
   are deterrent floors. **Completed by the §3.5 supersede rule** — the
   `RecordServe` self-mint is the banned subsidy, so the reconciliation is
   load-bearing, not hygiene. Escrow-routed skim is safe (worst case
   break-even, never a pump). **Skim routing DECIDED 2026-08-26 (D-POD-KNOBS):
   escrow, for the cross-tier funding loop; burn stays the fallback if an
   external audit ever needs "zero recovery, ever" in one word.**
2. **Q2 — the PoR leg: not load-bearing; DROP it in the neutral lane**
   (§4.2). Re-enters at strong form + relay as the content-committed
   recompute floor.
3. **Q3 — relay: the literature settles the shape.** No transit proof
   exists to buy (Tor's line failed; endpoint attestation dies under
   endpoint collusion) and TTP-free atomic fairness is proven impossible
   (Pagnia–Gärtner). Certified direction: **sender-funded, incremental,
   exposure-bounded micropayment** (PayWord/Orchid shape, reusing the
   blind-token machinery). Tit-for-tat is peer *selection*, never the
   payment mechanism. **NO TTP — the relay leg is self-enforcing at both
   ends** (D-POD-KNOBS knob 2, AMENDED 2026-08-27; re-certified 2026-08-30).
   The earlier "dispute-only quorum-TTP, owner's call held" framing is
   RESOLVED and does not survive: there is no adjudicable relay dispute at
   all — a PayWord token self-authorizes (the relay can redeem only
   increments the fetcher revealed) and forwarding is unprovable by any
   mechanism, so a quorum has nothing to adjudicate. The only residual is
   the irreducible one-increment stiff, bounded by making the increment
   small (§7.3). The full mechanism is specified in §7.3.
4. **Q4 — strong-form crypto: NOT adoptable.** The only pure-Go
   Camenisch–Shoup (`coinbase/kryptology` camshoup) is archived since 2022,
   do-not-use flagged, unaudited. If the strong form is ever pursued,
   prefer **quorum-TTP adjudication + threshold decryption** (drand/kyber,
   audited-grade) — a committee-trust design choice instead of a new
   hardness assumption + specialist audit. Strong form stays double-gated
   on #182 and gates nothing near-term.
5. **Q5 — settlement: per-node suffices** for bilateral tit-for-tat (the
   #586 divergence does not bite the neutral lane). Committed state is
   needed only for a credit a *third* operator must honor; its home is the
   D-TIERING registry state root (now separately certified — see the
   keystone certification), committed at coarse granularity (epoch
   net-settlement), never per-serve.

## 6. Non-goals

- **No standing fusion.** Strong-form PoD stays double-gated (Q4 crypto and
  #182). Coupling D-TIERING §5.3 holds: contribution scales publishing
  allowance and compensation, never consensus weight except through the bond.
  **[CERT]** If ever pursued, the strong form's route is quorum-TTP/VSS, not
  Camenisch–Shoup (§5 Q4).
- **No new crypto in the neutral lane** — confirmed; the neutral receipt
  *sheds* crypto (the PoR leg, §4.2).
- **No delivery subsidies** (§4.1 ban) — conserved transfers only. The
  supersede rule (§3.5) is what makes this true against the existing
  self-mint.

## 7. Build order (certified)

1. Wire `EnableDemandBank` + the balance-lane consumer under the certified
   invariant — **with the §3.5 supersede rule and the neutral-lane receipt
   shape (no PoR leg)** — firewall failing-first test leading (a big
   deliverer's `Reputation()` is unchanged across the reward — the Phase 2
   Invariant-A guard pattern).
2. The D-TIERING near-term flags (`--serve-content`, `--archive`) —
   build-gated only, now unblocked.
3. Relay compensation per the Q3 certified direction (sender-funded
   incremental micropayment) — **BUILT and LIVE** (`core/relaypay/payword.go`,
   `core/node/relaytransport.go`; PRs #646–#650). The balance-lane consumer
   landed (`cmd/silt/daemon.go:810`), the follow-on mechanism consult is
   certified (2026-08-30), the mechanism is specified in **§7.3**, and the
   increment is pinned at `RelayIncrementBytes = 4096`.

## 7.3 Relay compensation (the mechanism)

> **Status: BUILT — paid relay is LIVE** (`core/relaypay/payword.go`,
> `core/node/relaytransport.go` — `handleRelayOpen` / `SettleRelaySession` /
> `SplicePaid`; merged PRs #646–#650). The increment size is PINNED:
> `RelayIncrementBytes = 4096` (`core/relaypay/payword.go:40`), so the owed
> measurement of §7.3.5 is discharged. Design basis: **CERTIFIED — 2026-08-30**
> (`silt-reviews/research/research-outcome/PoD-relay-compensation-7.3-mechanism-RESEARCH-CERTIFICATION-2026-08-30.md`),
> ratified basis D-POD-KNOBS knob 2 ([decisions.md](../decisions.md), lines
> 869-907, AMENDED 2026-08-27). Deliberation:
> [`thinking/2026-08-30-pod-7.3-relay-compensation-design.md`](../thinking/2026-08-30-pod-7.3-relay-compensation-design.md).
> The mechanism shape is certified; do not re-derive it.

### 7.3.1 Why relay needs a different mechanism than the neutral lane

The neutral lane (§3) prices a *completed, content-verified* delivery. A
relay/gateway is content-blind: it forwards bytes it cannot verify and never
holds a verifiable object, so there is no completed-delivery receipt to sign.
Two facts bound the design:

- **No transit proof exists.** For the relay to be *owed* payment for forwarding
  increment N it would have to prove it forwarded N — unprovable by any mechanism
  (Tor's proof-of-bandwidth line failed; endpoint attestation dies under endpoint
  collusion).
- **TTP-free atomic fairness is impossible** (Pagnia–Gärtner). No mechanism makes
  forward-then-pay atomic.

The design is therefore **pay-as-you-go in small increments**, not
prove-then-settle. The irreducible one-increment stiff is priced small, not
adjudicated. **NO TTP** — the relay leg is self-enforcing at both ends
(D-POD-KNOBS knob 2).

### 7.3.2 The mechanism — sender-funded PayWord chain

**PayWord hash chain construction.** The fetcher picks a random tip `x_{S+1}` and
computes a chain by hashing:

```
x_S = H(x_{S+1}),  x_{S-1} = H(x_S),  … ,  x_0 = H(x_1)
```

`x_0` is the **root** (the value reached by hashing the most times). The fetcher
commits `x_0` to the relay once, bound to a blind credit worth `S · increment`.
To authorize the k-th increment the fetcher reveals `x_k`; the relay verifies
`H(x_k) = x_{k-1}` against the preimage it currently holds (equivalently
`H^k(x_k) = x_0`). One SHA-256 per increment. The relay redeems the highest
preimage it holds for `k · increment` at settlement.

Self-authorizing at both ends: the relay cannot forge a preimage (one-way hash),
so the fetcher is fully protected with no dispute; the fetcher cannot deny a
revealed preimage. If the fetcher stops revealing, the relay stops forwarding —
the stiff is bounded to one increment.

**Increment / redeem flow:**

1. **Withdraw.** The fetcher blind-withdraws a credit worth `S · increment` under
   a FRESH EPHEMERAL identity (`client.WithdrawDemandTokenPrivately`,
   `client/privissue.go:48` — the D3 slice-1 path, already built). Invariant (i).
2. **Commit the root.** The fetcher builds a FRESH PayWord chain and sends `x_0`
   to the relay in a session-open message, bound to the blind credit. One chain
   per session. Invariant (ii). The relay stores `(x_0, credit, k=0)`.
3. **Pay as bytes forward.** After the relay forwards increment k the fetcher
   reveals `x_k`; the relay checks `H(x_k) = x_{k-1}`, advances its held preimage,
   sets `k`. If the fetcher stops, the relay stops.
4. **Settle.** At epoch net-settlement the relay redeems its highest held preimage
   for `k · increment` into its **operator balance** — the same committed-state
   path delivery credit rides (Q5), **no new keystone field**. Conservation
   holds: the credit is drawn from the fetcher's already-paid blind credit, never
   minted.

**Reuse of the shipped machinery:**

- Blind ephemeral withdrawal: `client/privissue.go` (`WithdrawDemandTokenPrivately`).
- Conserved settlement into balance: sibling of `core/credit/delivery.go`
  `RedeemDeliveryCredit` — moves `balance` only, drawn from the fetcher's paid-in
  credit, never touches `Reputation()` (`core/credit/credit.go:291-298`).
- Consumer gate: mirrors the `--accept-receipts` gate at `cmd/silt/daemon.go:810`;
  a relay opts in under an analogous flag, off by default.
- Role model: the relay banks and settles the way `handleDeliveryReceipt`
  (`core/node/demandrole.go:175`) does, keyed on the chain root and highest
  preimage instead of a completed-delivery receipt.

The only genuinely new code unit is the PayWord primitive (build S+1 hashes;
verify `H(x_k) = x_{k-1}`), SHA-256 only, no new dependency.

### 7.3.3 Conservation and the firewall (inherited, certified)

- **Conservation:** the relay redeems ≤ the chain's value, drawn from the
  fetcher's fee-paid blind credit. A colluding fetcher+relay under one operator
  rebuys its own fee at break-even (or a strict loss with a skim). No new mint
  surface (cert §2a).
- **Firewall (Invariant A):** relay forwarding credit moves `balance` and nothing
  else — structurally identical to delivery credit. It cannot enter
  `Reputation()`. **One Invariant-A regime covers delivery + relay credit**
  (cert §2b, Q6). Failing-first guard: a relay-credit redemption leaves the
  relay's `Reputation()` unchanged (the `core/credit/invariant_a_test.go`
  pattern).
- **Wash / forgery is a strict self-loss:** forgery is cryptographically excluded
  (PayWord self-authorizes), and wash is a strict loss under sender-funded
  conservation (cert §2c).

### 7.3.4 HARD design invariants — M0 access-privacy (IMMUTABLE, Don't-#3)

These two constraints are non-negotiable build constraints, not notes. Violating
either upgrades the relay into a **longitudinal observer** of the fetcher — an M0
access-privacy violation (`docs/TENETS.md` Don't-#3). They are enforced by
construction and each carries a failing-first guard.

**(i) The PayWord chain root MUST bind to a BLIND credit under a FRESH EPHEMERAL
identity, NEVER a durable one.**

- *Enforced by:* funding the chain exclusively through
  `client.WithdrawDemandTokenPrivately` (`client/privissue.go:48`), which
  withdraws over a throwaway keypair and tears it down. The issuer authenticates
  only an ephemeral key and charges no account it can tie to the fetcher. No code
  path binds a chain root to a durable-account credit.
- *Why non-negotiable:* binding a chain to a durable identity lets the relay tie
  the fetcher's durable identity to what it fetched. Not permitted at any
  performance price.
- *Guard:* a chain root funded by a durable-account credit MUST be rejected.

**(ii) A FRESH ephemeral identity AND a FRESH PayWord chain per session.**

- *Enforced by:* one `WithdrawDemandTokenPrivately` call and one freshly-generated
  tip per session-open. No chain or ephemeral identity is cached or reused across
  sessions; the relay stores only `(root, credit, highest-preimage)` for the live
  session and discards it at settlement.
- *Why non-negotiable:* reusing a chain or ephemeral identity lets the relay link
  a fetcher's sessions to each other — upgrading it from a per-session observer
  (which sees only the IP it already routes) to a longitudinal one. A real
  Don't-#3 regression.
- *Guard:* a session-open reusing an ephemeral identity or a chain root MUST be
  rejected.

### 7.3.5 The increment size — MEASUREMENT DISCHARGED, pinned at 4096 B (build-immutable #8)

**Status: DONE.** The increment is pinned at `RelayIncrementBytes = 4096`
(`core/relaypay/payword.go:40`), inside the ~1–64 KiB envelope derived below. The
"one quantitative gate before relay-payment code commits" is met; the paid-relay
code shipped (§7.3 head). The derivation is retained for provenance:

The increment size `B` (payload bytes per increment) is a **floor-box measurement
in BYTES, not a round figure** (the #299/#555 produce-cost scar). `B` is the
smallest value satisfying both:

- **(a)** per-increment verify (one SHA-256 over 32 B + preimage-advance) ≤ ~1% of
  the time to forward `B` payload bytes on the 1 vCPU / 2 GB floor box; and
- **(b)** chain state `S · 32 B` (where `S = objectSize / B`) stays MB-scale for
  the largest object class, aligned to a sub-chunk boundary.

The expected envelope is ~1–64 KiB; the exact value was pinned by the measurement
to **4096 B** (`RelayIncrementBytes`), whose method is defined in the deliberation
note. The measurement artifact was produced and the value chosen; it did not reopen
the design.

### 7.3.6 Residual (disclosed, priced small — not solved)

The irreducible one-increment stiff (either direction) is not eliminable by any
mechanism (Pagnia–Gärtner + forwarding-unprovability). The remedy is to bound the
increment small (§7.3.5), NOT to adjudicate. A **skim on relay payment** is an
engineering knob, conservation-safe either way (composes as the delivery skim,
`delivery.go:120-124`); v1 ships without one for simplicity, since conservation
already makes wash a strict non-gain.
