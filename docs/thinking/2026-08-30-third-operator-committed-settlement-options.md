# Third-operator committed settlement — a DESIGN-OPTIONS strawman

> **Status: DEFINITION ONLY — NOT a decision, NOT a mechanism, NO production code.**
> This maps the design space for a **gated economic mechanism** so Research can certify
> soundness, the PE can rule on correctness/severity, and the owner can decide. It
> recommends **no single option**. Every option carries its own firewall/conservation
> analysis and its own gated surfaces, so the space can be evaluated on the merits.
>
> **Author seat:** Builder (advises + shapes the question; does not decide a gated
> mechanism — silt `.claude/CLAUDE.md` research gate). This document is the shape of the
> question, not an answer.

## 0. Why this document exists now

Relay compensation shipped (`docs/design/pod.md` §7.3, the §7.3 transport Batches 1–3,
CERTIFIED 2026-08-30). It settles **bilaterally and in-memory**: a relay redeems a
PayWord chain into its **own node's** `credit.Ledger` operator balance
(`core/credit/delivery.go` `RedeemDeliveryCredit`; the balance economy in
`core/credit/credit.go`). That closes the case the §7.3 mechanism was scoped to — a
fetcher paying the relay it is *directly connected to*.

The next PoD frontier is the case §7.3 explicitly deferred (D-POD-KNOBS knob 2 / PoD
§5 Q5): **a credit that a THIRD operator — not the fetcher's own node, not the fetcher's
direct counterparty — must honor.** Q5 named the answer's *home* ("the D-TIERING registry
state root, committed at coarse granularity — epoch net-settlement, never per-serve") but
NOT its *mechanism*. This document opens that mechanism's design space.

**The frontier in one sentence.** Today every balance-lane credit lives in a per-node
in-memory ledger that no other node is obligated to honor; committed settlement lets an
operator carry a redeemable balance in **chain-committed state** that any validator agrees
on, so value can settle across operators who never had a direct session.

## 1. The load-bearing constraints (cite these — they are the frame)

These are HARD. Any option that violates one is dead on arrival, not a tradeoff.

### 1.1 The firewall immutable — γ→1/N, Invariant A (`docs/TENETS.md`, m0 §10, #182)

Delivery/relay/durability credits fund **compensation and durability, NEVER consensus
standing**. Structurally: `core/credit/credit.go:291-298` `Reputation()` reads bond-derived
terms and slash counters ONLY — never `balance`, never `servedBytes`, never any delivery
field. `docs/design/pod.md` §41-43 and §7.3.3 restate it: relay credit "moves `balance` and
nothing else — it cannot enter `Reputation()`." **One Invariant-A regime already covers
delivery + relay credit** (D-POD-KNOBS coupling 2). Committed settlement must extend that
regime to committed state WITHOUT letting any settled value become a `Reputation()` input.

The deep open problem this fences (`docs/decisions.md:1077-1082`, m0 §10): fusing served
content into standing without leaking γ→1/N is UNSOLVED. Committed settlement must not open
a back door to it — a committed balance that a validator could read as standing is exactly
the leak.

### 1.2 The conservation rule — no network-minted per-receipt subsidy (PoD §4.1)

"No credit is minted by a receipt" (`core/credit/delivery.go:6-12`). A redeemed
receipt/chain only MOVES value the fetcher already paid in (the burned/escrowed retrieval
fee), less the durability skim. **The banned dual:** any network-minted per-receipt subsidy
is a money pump (PoD §4.1, memory's "banned dual"). Conservation carries soundness
(`credit ≤ fee`); `fee > 0` / `skim > 0` are deterrent floors, not soundness ones. A
committed settlement that let an operator redeem MORE than was paid in — or that let the
same paid-in value be redeemed against TWICE (double-spend across operators) — reintroduces
the pump.

Committed settlement raises conservation's stakes: an in-memory double-spend is bounded to
one node's wrong belief; a **committed** double-spend is agreed-wrong by the whole network.

### 1.3 M0 access-privacy — Don't-#3 (`docs/TENETS.md` Part VI)

The §7.3 privacy invariants (PoD §7.3.4): the PayWord chain root binds to a **blind credit
under a FRESH EPHEMERAL identity**, and there is a **fresh identity + chain per session** —
reuse upgrades a relay from a per-session to a **longitudinal** observer. Committed
settlement writes value into PUBLIC, PERMANENT chain state. Whatever it commits is visible
to every validator forever. So the privacy bar is HIGHER here than in-memory settlement: a
committed record that links a fetcher identity to what/when/from-whom it fetched is a
Don't-#3 regression that no performance argument buys back (PoD §7.3.4 "not permitted at any
performance price").

### 1.4 The keystone bounded-state property (D-POD-KNOBS standing rule)

"Whenever a Phase-4/keystone knob is otherwise balanced, favor the option that keeps the
**live committed state bounded** — that is the keystone's reason to exist"
(`docs/decisions.md:1002-1004`). The whole point of the two-root keystone
(`core/chain/statehash.go`: 18 committedSet leaves + era-4 spine + a separate
RFC-6962 committedLog root) is that the live committed state does not grow without bound.
Committed settlement adds committed state; that state must not be a forever-growing term.

### 1.5 The v5 format is STILL SETTLING — the hard dependency

The committed keystone format is mid-flight. Lane-1 Part B (the witness-validating floor
box) landed only its ADDITIVE slice (`core/chain/floorbox_v5.go`, PR #657); **the
accept-core bounded recompute is ROUTED TO THE RESEARCH GATE, NOT BUILT** (CHANGELOG
Unreleased; PE ruling `RULING-lane1-partA-readset-v5-producer-2026-08-30.md`). The v5
witness read-set is the 23-keyspace set (`docs/decisions.md:853-883`, AMENDED cert
2026-08-30). The era-3 → v5 format is not frozen; the `bonded`/`epochSet` keystone probes
(#603) are the hard gate before the format freezes (memory RESUME block).

**Consequence, stated bluntly:** any option that adds a NEW committed field couples to a
format that is not frozen and to a floor-box validator whose accept-core does not yet exist.
Options that add committed *validity* state cannot be soundly specified until the v5
witnessable recompute is certified. This is the single biggest reason this is
DEFINITION-only work. It is called out per-option in §2.

## 2. What must be committed — options for the carried cross-operator balance

The core question: what committed state carries value that a third operator can redeem?
Four options, ordered from lightest committed footprint to heaviest.

### Option W1 — Epoch net-settlement receipts committed to the transparency LOG only

Commit nothing to the validity SMT. Instead, at each epoch net-settlement, an operator
publishes a **net-settlement entry** into the existing append-only RFC-6962 transparency log
(the `committedLog` / revLog root, `core/chain/statehash.go:29`, its own root by the #597
resolution). The entry is a signed statement "operator X is owed N by the settlement pool
for epoch E, against these redeemed chains/receipts," append-only, never mutated.

- **What is committed:** a log leaf per operator per epoch (or per settlement batch). The
  balance ITSELF stays in per-node ledgers; the log is the cross-operator *evidence* that
  lets any node reconstruct the same net position by replaying the log.
- **Coupling to v5:** LIGHTEST. The committedLog root already exists and is order-committed
  separately (order-independence oracle `modelcheck_order_independence_test.go`). No new SMT
  keyspace. No new validity leaf. Does NOT wait on the accept-core recompute.
- **Bounded state:** the log is append-only and grows forever BY DESIGN — but it lives in
  the ARCHIVAL tier, not the bounded live committed set (mirrors D-POD-KNOBS knob 3: "live
  committed state forgets; provenance survives in the archival tier"). Live state stays
  bounded.
- **The hard question it defers:** a log entry is a CLAIM, not a validated balance. Nothing
  in consensus checks that operator X was actually owed N. Settlement correctness rides on
  operators independently replaying the log and refusing to honor an over-claim — i.e. it is
  still fundamentally BILATERAL/reputational, dressed in a committed audit trail. Whether
  that clears the "third operator MUST honor" bar is the sharp question (§5 Q-W1).

### Option W2 — A committed net-balance leaf per operator in the validity SMT

Add a new committed keyspace `settleBalance[operator] → int64` to the state root (a 24th
keyspace alongside the 23-keyspace witness read-set). `apply()` mutates it at epoch
net-settlement: credit the payee operator, debit the settlement pool. Every validator agrees
on the committed balance.

- **What is committed:** one int64 leaf per operator with a nonzero settled balance.
- **Coupling to v5:** HEAVY and BLOCKING. A new committedSet leaf must be added to
  `stateRootTags` (`statehash.go:91`), enter the witness read-set (making it a 24-keyspace),
  and be covered by the completeness oracle (`modelcheck_state_completeness_test.go`) and the
  floor-box witnessable recompute. **This cannot be soundly specified until the v5
  accept-core recompute is certified** (§1.5). It also freezes into the format the #603
  probes gate.
- **Bounded state:** one leaf per operator-with-balance. Bounded by the operator count, not
  by receipt volume — GOOD, if balances net to zero and dead operators' zero leaves are
  reaped (a TTL-lapse rule like D-POD-KNOBS knob 3). UNBOUNDED if leaves are never reaped.
- **Firewall exposure:** a committed per-operator balance leaf sits RIGHT NEXT TO the
  bond/standing leaves in the same SMT. The Invariant-A structural guarantee is code-level
  (`Reputation()` does not read it), but a committed balance is now a consensus-visible
  quantity — the audit surface for "does any validity rule read it?" is larger. Every
  fork-choice / validity path must be shown blind to it (§3).

### Option W3 — Receipt/chain redemptions committed individually to the SMT

Commit each redeemed cross-operator receipt (or PayWord chain terminal state) as its own
committed leaf `redeemed[receiptID] → payee,amount`, the way `spent[serial]` records a spent
token today (`statehash.go:40,119`).

- **What is committed:** one leaf per redeemed cross-operator receipt/chain.
- **Coupling to v5:** HEAVY and BLOCKING (same as W2) PLUS a volume problem.
- **Bounded state:** UNBOUNDED in receipt volume unless aggressively reaped. This is the
  `spent`-map growth problem at delivery scale. Directly fights §1.4. Likely a non-starter
  on bounded-state grounds alone, but included for completeness because it is the most
  "obviously correct" double-spend defense (each receipt committed once, redeemable once).
- **Double-spend:** STRONGEST — a receipt committed as redeemed cannot be redeemed again by
  any operator. But it buys that with the worst state growth.

### Option W4 — No new committed BALANCE state; commit only a settlement COMMITMENT (Merkle root of an off-chain net-settlement)

Operators run an off-chain net-settlement (bilateral or pool) and commit ONLY a single
Merkle root of the settlement batch to the log or a scalar committed leaf. Redemption is
proven by a Merkle witness against the committed root — the same "validate by proof, not by
holding the state" posture as the floor-box (`docs/decisions.md:513-543`, C-7/#600).

- **What is committed:** one root per settlement batch (scalar or log leaf). O(1) committed
  footprint.
- **Coupling to v5:** LIGHT-to-MEDIUM. A single committed scalar (like the era-4 activation
  scalars, `statehash.go:80-81`) is a smaller format change than a new map keyspace, but it
  is STILL a committed-format change and still couples to the freeze.
- **Bounded state:** BEST — O(1) committed, O(batch) proven off-chain.
- **The hard question:** who computes the off-chain settlement, and what stops the committer
  from committing a root over a settlement that over-credits itself? This pushes the trust
  question to "who may commit a settlement root," which is a consensus-adjacent authority
  question (§5 Q-W4). Without an answer it is a committed root over unvalidated arithmetic.

### Comparison

| Option | Committed footprint | Waits on v5 accept-core? | Bounded state | Double-spend defense | Third-op "must honor" |
|---|---|---|---|---|---|
| W1 log-only | 1 leaf/op/epoch (archival) | No | Yes (live) | Replay-and-refuse (weak) | Reputational, not enforced |
| W2 net-balance leaf | 1 leaf/op | **Yes (blocking)** | If reaped | Committed balance is agreed | Enforced by consensus |
| W3 per-receipt leaf | 1 leaf/receipt | **Yes (blocking)** | No (volume) | Strongest | Enforced by consensus |
| W4 settlement root | 1 root/batch | Partial | Best (O(1)) | Off-chain + witness | Depends on committer authority |

## 3. How settlement ENTERS a block — options + the firewall analysis per option

Two orthogonal questions: (A) what transaction/event triggers a committed settlement, and
(B) how each keeps delivery credits OUT of `Reputation()` and mints no subsidy.

### Entry mechanism E1 — a dedicated settlement transaction type

A new block-carried message `MsgSettle` (sibling of the receipt/bond messages) that `apply()`
processes at an epoch boundary, mutating the committed settlement state (W2/W3/W4).

- **Tradeoff:** explicit, auditable, rate-limitable. But it is a NEW consensus message and a
  NEW `apply()` branch — a consensus-rule change (I1–I5 adjacency), fully gated. It also adds
  a new witness read/write site the floor-box recompute must cover.
- **Firewall:** the `apply()` branch must write ONLY the settlement keyspace and the payee's
  committed balance leaf, never any bond/standing leaf. Failing-first guard: a settlement tx
  leaves every operator's `Reputation()` unchanged (the `invariant_a_test.go` pattern,
  extended to the committed path).
- **Conservation:** the tx must carry, or reference, the paid-in fee/credit it settles
  against, and `apply()` must enforce `settled ≤ paid-in` at commit time. The debit side (the
  settlement pool / the fetcher's escrowed fee) must be committed too, or the network mints.

### Entry mechanism E2 — settlement folded into the existing epoch-rotation apply

No new message. At `rotateEpoch` (the existing epoch boundary, `chain.go` `rotateEpoch`),
`apply()` nets the epoch's committed-log settlement entries (W1) or the accumulated
settlement commitments (W4) and writes the net result.

- **Tradeoff:** no new consensus message, reuses the existing coarse-granularity boundary Q5
  named ("epoch net-settlement, never per-serve"). But it makes the epoch-rotation apply —
  already the heaviest, most consensus-critical branch (the activation tallies,
  `docs/decisions.md:891-897`) — heavier, and the boundary witness read-set is already
  O(RegCap). Adding settlement netting to it enlarges the boundary recompute.
- **Firewall / conservation:** same requirements as E1, but the netting logic lives in the
  most sensitive branch, so the blind-to-standing proof burden is highest here.

### Entry mechanism E3 — redemption proven by witness, committed lazily (pairs with W4)

Settlement value is never eagerly committed as a balance. An operator redeems by presenting
a Merkle witness against a committed settlement root (W4); the committed state only ever
holds roots. Mirrors the floor-box "validate by proof" posture.

- **Tradeoff:** lightest committed mutation, but pushes correctness into witness verification
  and the "who may commit a root" authority question (§5 Q-W4). Redemption-time
  double-spend (redeeming the same witness twice) needs its own committed nullifier — which
  reintroduces a per-redemption committed leaf (W3's growth) unless nullifiers are epoch-scoped
  and reaped.

### The firewall + conservation summary, per what-is-committed

- **W1 (log-only):** firewall is trivially safe — a log leaf is not read by any validity rule
  or by `Reputation()` (the log already has its own root and feeds nothing into the validity
  SMT). Conservation is NOT enforced by consensus; it rides on honest replay. **Money-pump
  risk: LOW structurally, but the pump is not consensus-prevented** — a colluding operator can
  publish an over-claim log entry; other operators must catch it by replay. The defense is
  reputational, not cryptographic.
- **W2 (balance leaf):** firewall requires an explicit proof that no validity/fork-choice path
  reads `settleBalance` (the audit is real work — the leaf is in the same SMT as standing).
  Conservation IS enforceable at `apply()` (`settled ≤ paid-in`), IF the debit side is also
  committed. **Money-pump risk: LOW if the debit is committed and checked; HIGH if the tx
  credits without a committed debit** (the classic half-committed mint).
- **W3 (per-receipt leaf):** same firewall as W2. Conservation strongest (one-receipt-one-
  redeem, committed). **Money-pump risk: LOWEST; state-growth risk HIGHEST.**
- **W4 (settlement root):** firewall safe if the root is a scalar/log leaf feeding no validity
  rule. Conservation depends ENTIRELY on the off-chain settlement arithmetic and the committer
  authority — consensus commits a root, not a checked balance. **Money-pump risk: concentrated
  in "who computes and commits the settlement" — if a self-interested operator commits, it can
  over-credit unless the batch is independently verifiable by witness.**

**The cross-cutting money-pump flag:** every option that commits a CREDIT must commit (or
reference a committed) DEBIT of equal-or-greater value drawn from already-paid-in fees. An
option that commits only the credit side — even "temporarily" — is a money pump the whole
network agrees on. This is the single sharpest correctness question for Research (§5 Q-C1).

## 4. The gated surfaces — exactly what needs certification

Per `.claude/CLAUDE.md` research gate and `docs/build-process.md`. This whole frontier is
gated; the enumeration below is so the certification scope is explicit and nothing ships on a
verbal summary.

### 4.1 Economic-mechanism change (Research certification REQUIRED)

- The settlement mechanism itself is a **D-DEMAND / escrow / skim-adjacent economic
  mechanism** — the exact class the gate names. Conservation across operators, the
  paid-in-vs-redeemed accounting, any settlement pool, and any skim on settlement all need
  certification.
- The conservation soundness boundary across a THIRD party (not the paying fetcher) is a NEW
  soundness question — §7.3's `credit ≤ fee` bound was proven for the bilateral case; a third
  operator redeeming against a pool needs its own proof that the pump does not open.

### 4.2 Committed-format change (Research + PE, and it FREEZES into v5)

- Any option adding a committed keyspace (W2, W3) or a committed scalar (W4) is a
  **committed-format change** that must enter the v5 witness read-set, the completeness
  oracle, and the floor-box witnessable recompute. It is BLOCKED behind the v5 accept-core
  recompute certification (§1.5) and the #603 probes.
- W1 (log-only) is the ONLY option that avoids a validity-format change — it uses the existing
  committedLog root. Even it needs certification that a settlement log leaf is sound to admit
  and cannot be replayed into a validity effect.

### 4.3 Consensus-rule change (Research certification REQUIRED)

- Entry mechanism E1 (a `MsgSettle` transaction type) and E2 (folding netting into
  `rotateEpoch`) are **consensus-rule changes** (new/changed `apply()` behavior, I1–I5
  adjacency). Any change to what a validator accepts, or to the epoch-rotation branch, is
  gated.

### 4.4 Security parameter a proof depends on (Research certification REQUIRED)

- Any settlement rate cap, batch size, nullifier TTL/reap window, or settlement-pool
  parameter is a candidate security parameter (recall build-process.md: "a durability knob was
  twice also a security parameter"). Each must be measured/derived, not guessed (#8), and
  certified if a soundness proof leans on it.

### 4.5 M0 / Don't-#3 access-privacy (certification of the privacy analysis)

- Whatever is committed is PUBLIC and PERMANENT. The certification must show the committed
  settlement record does not link a fetcher identity to what/when/from-whom it fetched, and
  does not let a relay/operator become a longitudinal observer by correlating committed
  entries across epochs. This inherits and TIGHTENS the §7.3.4 invariants.

## 5. Open questions for Research / PE — the sharp ones a certification must answer

These are the questions the strawman cannot answer and must not guess. They are the shape of
the certification.

- **Q-C1 (the pump, cross-operator).** For each committed option, is the paid-in DEBIT
  committed and checked at the same commit as the CREDIT? State the exact invariant that makes
  third-operator settlement conserve (`settled ≤ paid-in`, per operator, per epoch), and prove
  no interleaving of settlements across operators/epochs mints. This is Q5's unanswered half.
- **Q-C2 (double-spend across operators).** What committed nullifier prevents the same
  paid-in credit / receipt / PayWord chain from being redeemed by two operators, or twice by
  one, across epochs? Is it bounded/reaped (§1.4)? If W1, is replay-and-refuse actually
  sufficient, or does "MUST honor" require a committed nullifier?
- **Q-F1 (firewall audit).** For a committed balance leaf (W2/W3) sitting in the validity SMT,
  enumerate EVERY validity and fork-choice read site and prove none reads the settlement
  keyspace. Is the single Invariant-A regime (D-POD-KNOBS coupling 2) sufficient once the
  balance is COMMITTED, or does committed value need a stronger structural fence than the
  in-memory `Reputation()` guard?
- **Q-V1 (v5 coupling / sequencing).** Can any committed-validity option (W2/W3/W4-scalar) be
  soundly specified BEFORE the v5 accept-core recompute is certified and the #603 probes are
  green — or must committed settlement wait for the format freeze? (Builder's read: it must
  wait; Research/PE to confirm or refute.) Is W1 (log-only) a sound INTERIM that ships without
  the freeze and is later upgradable?
- **Q-A1 (settlement authority, for W4/E3).** Who may compute and commit a settlement root/
  batch? Is it a single operator (self-interested — needs witness-verifiable batches), a
  quorum (a new committee-trust surface, the same class as the strong-form quorum-TTP, PoD §5
  Q4), or a deterministic function of already-committed state? What stops a self-crediting
  batch?
- **Q-P1 (committed privacy).** Does the committed settlement record admit a cross-epoch
  linkage of a fetcher to its fetch history? Prove the committed form is at least as private
  as the §7.3.4 in-memory form. What is committed at operator granularity vs. anything finer?
- **Q-B1 (bounded state / reap).** For W2, what is the TTL-lapse reap rule for a zero/dead
  operator balance (mirror of D-POD-KNOBS knob 3)? Prove the live committed settlement state
  is bounded by operator count, not by receipt/epoch volume.
- **Q-S1 (sequencing vs. the roadmap).** Is third-operator committed settlement even NEEDED
  before the durability economy / floor-box work the roadmap orders first, or is bilateral +
  §7.3 sufficient for the flixz-class real user? (Builder pushes back on building committed
  settlement ahead of a demonstrated need — see §6.)

## 6. Builder's honest read — what is unknown, and the simplicity pushback

Stating this in the open, as the seat that defends shipping the simplest thing the evidence
justifies. It is NOT a recommendation among W1–W4.

- **The biggest unknown is sequencing, not mechanism.** Committed settlement is only NEEDED
  when a third operator must honor a credit with no bilateral relationship to fall back on.
  §7.3 relay and the neutral lane are both bilateral and already shipped. The first evidence I
  can name that DEMANDS committed settlement is not yet in hand — no failing test, no measured
  real-user (flixz-class) case where bilateral settlement is insufficient. Per evidence-or-
  nothing (build-immutable #7), **the trigger to build this should be a named artifact showing
  bilateral settlement fails a real case**, not the availability of the keystone.
- **The v5 dependency is real and blocking for the heavy options.** W2/W3 (and W4's scalar)
  cannot be soundly specified until the v5 accept-core recompute is certified and the #603
  probes are green. Specifying a committed field against an unfrozen format is exactly the
  trap §1.5 describes. The lightest option (W1, log-only) is the only one that could ship
  without the freeze — and it is the WEAKEST on "must honor" enforcement.
- **The simplest thing that could work, if a need is demonstrated:** W1 (log-only, archival)
  as a certified INTERIM — it adds no validity-format state, stays bounded in live state, and
  keeps the firewall trivially safe — with W2 (a single reaped net-balance leaf) as the
  upgrade IF and WHEN the "must honor" bar demands consensus enforcement AND the v5 format has
  frozen. This is a hypothesis for Research/PE to test, not a decision.

The tension to surface to the Planner: building committed settlement now would be gold-plating
against a need that is not yet evidenced, AND it couples to a format that is not yet frozen.
The counter-argument (which the PE/Researcher may hold) is that the committed FORMAT decisions
are cheapest to get right BEFORE the v5 freeze, so the settlement field set should be scoped
now even if the mechanism ships later. Both are legitimate; the owner decides. This document
exists so that decision is made on a mapped space, not on the first option that looks correct.

## 7. Provenance

- Constraints cited: `docs/design/pod.md` §3–§4.1 (conservation, the banned subsidy), §5 Q5
  (per-node suffices; committed only for a third-operator credit; epoch net-settlement), §7.3
  (the shipped bilateral relay mechanism), §7.3.4 (the Don't-#3 privacy invariants).
- Decisions cited: D-POD-KNOBS (`docs/decisions.md:914-1004`, esp. knob 2 relay + knob 3
  TTL-lapse bounded-state rule), D-POD-RELAY-COEXIST (`:1008-1052`), the lane-1 floor-box
  ratification (`:853-911`), the witness-validation posture (`:802-850`).
- Code verified at `origin/main` `61c75eb`: `core/credit/delivery.go` (bilateral in-memory
  settlement, conservation + supersede), `core/credit/credit.go:291-298` (`Reputation()` is
  blind to balance — the firewall), `core/chain/statehash.go` (the committed leaf tags; the
  balance ledger is NOT committed), `core/chain/floorbox_v5.go` + CHANGELOG Unreleased (the v5
  accept-core recompute is research-gated, NOT built).
- Immutables/firewall: `docs/TENETS.md` Part VI (Don't-#3), m0 §10 (γ→1/N, the unsolved
  shared-content sealing boundary).

**No option is recommended. This is the map, for Research to certify soundness, the PE to rule
on correctness/severity, and the owner to decide.**
