# Decision ledger

**Status: living record.** The product/strategy decisions silt's owner has made, why,
and what remains open. Each entry separates the **direction** (a decision we can and did
derive) from any **construction** (a primitive that must still be built or researched).
This exists so decisions stop being invisible — a reader (builder, red team, researcher,
user) can see exactly what is settled, what is deferred, and on what basis.

**How these were decided.** The research package (`silt-agent-memory/researcher/reviews/research-outcome/`,
read-only) was written specifically to answer these questions — each memo ends with a
recommendation. So the *directions* below are **derived from the accepted research**, not
re-opened. New research is commissioned only where a memo self-flags a wall (a primitive
that does not yet exist). Where possible, an independent party should *verify* a derived
direction rather than re-author it.

Related: [`TENETS.md`](TENETS.md) (canon), [`design/m0.md`](design/m0.md) §9 (the M0-scoped
subset). Superseded per-finding history: [`/archive/`](../archive/).

---

## D-PRIV — access privacy is a metadata-layer tradeoff, not a blob-layer absolute

- **Status:** ✅ DECIDED (Option A) — 2026-08-05.
- **Research basis:** Memo 01 (private retrieval). The anonymity trilemma (Das et al.,
  IEEE S&P 2018) is a hard wall: against a global adversary you cannot have strong
  anonymity + low bandwidth + low latency at once. A participating node structurally sees
  the keys it routes and serves. Access-unobservability is achievable **at the metadata
  layer** (mixnet transport + private DHT lookup + unlinkable retrieval tokens), **not at
  the blob layer** (PIR/ORAM over multi-GB objects imposes a 10–20× blowup a paid substrate
  cannot pay), and even at the metadata layer it is bounded by anonymity-set size on a
  small network.
- **Direction (decided):** Amend immutable #4 from an absolute ("who fetches what is *never*
  observable") to a **stated, layered tradeoff**: publish-unlinkability is delivered **at the
  chain layer** (the committed record omits the Publisher field by default; opt-in blind
  tokens give cryptographic unlinkability of publish→durable-identity) — **but a residual
  transport IP+timing link stays OPEN until D3 issuance-mixing ships** (H8/#179), so this is
  *not yet* full-stack unlinkability. Access-unobservability is a metadata-layer goal *held in
  tension*, bounded by the trilemma and anonymity-set size, not guaranteed at the blob layer.
  What stays absolute is the **refusal to surveil** — silt builds no mechanism to log or link
  who-fetched-what. Resolves the standing contradiction with `threat-model.md` (which already
  concedes access patterns are correlatable); the who-reads comparative framing in
  `risk-register.md` #14 is requalified to match.
- **Also ship:** the **D3 issuance-mixing** residual (route token issuance over the
  content-blind relay from an ephemeral identity + epoch batching) to close the publisher
  IP+timing link — a build item, not a further decision.
- **Construction deferred:** the full H8 metadata-privacy stack (mixnet + PIR-DHT) is a
  post-M0 build track, not required to make the tenet honest now.

## D-S7 — durability is funded by an internal, non-speculative credit reserve

- **Status:** ✅ DIRECTION DECIDED + **BUILT (H7/#95, merged 2026-08-08).** The durability
  economy (per-object escrow, serve auto-skim, rarest-shard bounty), the verified
  proof-of-correct-repair gate, and the finite-but-renewable instruments (funded horizon +
  instrument `g`) all ship, adversarially verified end to end. **One scope change surfaced by
  the build:** the plaintext-blind homomorphic-commitment correctness leg (the "transparent
  binary-field PCS" the design preferred) is a **GF(2⁸) theorem-level dead end in pure Go**, so
  M0 ships the **Merkle-recompute floor** and the blind/bandwidth-free upgrade is a documented
  fast-follow (see the construction bullet below and
  [`design/h7-proof-of-repair.md`](design/h7-proof-of-repair.md) §3/§7/§13).
- **Research basis:** Memo 07 (durability economics) named the wall; the follow-up
  commission (`research-outcome/commission/A1-*`) **delivered the construction and the
  equilibrium.** Memo 07's finding stands: cold-data repair that is **token-less AND
  center-less had no existence proof** — every deployed survivor uses a crutch silt forbids
  (a central paymaster — Storj, which filed Chapter 11 in July 2026, trapping operator
  balances; an online paying client — Sia; a token block-reward subsidy — Filecoin; or a
  prepaid token endowment betting on falling costs — Arweave). The relaxation to an
  internal, non-speculative, time-shiftable credit reserve is what moved S7 from
  "unsolvable" to "solvable in a checkable region."
- **Direction (derived):** Relax "no token" to **"no *speculative external* token."** Keep
  the internal credit unit, but make credits **durable, escrowable, and forwardable in
  time**, and adopt the memo's triad as the S7 spine:
  1. **Per-object durability escrow** — a prepaid credit reserve that pays repair bounties.
  2. **Auto-skim** — a protocol-fixed fraction of each object's serving revenue routes back
     into *that object's* escrow, so popular data self-funds future repair and cold data
     draws its reserve ("paid by the demand it serves," literally, at the object level).
  3. **Rarest-shard bounty multiplier** — scale the bounty by how under-replicated a stripe
     is, self-healing without a central scheduler.
  This completes the fusion memo 09 already put in the tenets: the durability budget and the
  Sybil budget are **one ledger**. The internal credit reserve is distinct from *standing* —
  standing stays work-backed and coin-free; credits fund *durability*, confer no consensus
  weight.
- **Construction (DELIVERED — `A1-proof-of-repair-construction.md`):** center-less
  proof-of-correct-repair **exists as a composition of proven primitives, no new primitive
  for the plain-RS case.** RS repair is a *public linear combination* of surviving symbols
  and silt's commitments are linearly homomorphic, so the check is "does a public linear
  relation hold over committed values, without seeing the values" — exactly what
  linearly-homomorphic authenticators do. The composition: a polynomial-commitment layer
  (KZG opening, or a BFKW subspace signature) proves **correctness** (the repaired shard is
  the correct codeword coordinate) against the commitment the network already holds;
  **Shacham–Waters PoR** (already in silt) proves **retrievability** (the caretaker actually
  holds the bytes, re-challengeable over time); the **DAS/PeerDAS quorum** pattern supplies
  the **center-less** checking. ~100 B proof, one–two pairings to verify, no plaintext seen,
  bounty releases iff *both* correctness and retrievability verify, a false claim is publicly
  attributable and bond-slashable. **Built as H7.**
  - **B8 / no-trusted-setup — the blind correctness leg is deferred (build outcome).** The design
    preferred a **transparent, binary-field polynomial commitment (FRI-Binius)** over KZG (no SRS,
    matches GF(2⁸) natively). Building against the real code **pressure-tested and rejected the
    pure-field-commitment path for M0**: there is **no ring homomorphism GF(2⁸)→F_r** (characteristic
    2 vs prime `r`), so a prime-field Pedersen/KZG commitment cannot carry silt's GF(2⁸) RS relation
    with a linear homomorphic check (Semi-AVID-PR only works because its code lives *in* the
    commitment field — adopting it faithfully = a storage-format change to F_p), and no mature,
    standalone, pure-Go **characteristic-2-native** commitment (FRI-Binius/lattice-SIS) existed to
    adopt (B8: never hand-roll one). **So M0 ships the `core/repairproof` Merkle-recompute floor:**
    reconstruct the target from k survivors and check it is byte-identical to the manifest-committed
    shard id — sound, pure-Go, publicly checkable, content-blind, but **not bandwidth-blind** (an
    explicit M0 non-goal, not a silently-broken claim). The blind upgrade (F_p re-encode, or a
    char-2-native commitment when a library exists) is a fast-follow.
  - **Genuinely open (off today's critical path, → research frontier):**
    proof-of-correct-repair for **MSR / regenerating codes** (Clay, Product-Matrix) has no
    published construction; silt ships plain-RS reconstruction, so this is a roadmap item,
    not an M0 blocker.
- **Durability CONTRACT — finite-but-renewable, not "perpetual"** (decided 2026-08-06,
  from `A1-cold-repair-equilibrium.md`). The per-repair game is solved unconditionally
  (bounty auto-clears to cost + bond-forfeiture asymmetry defeat the Freenet/GNUnet
  free-rider death). But **perpetual cold-data solvency is the Arweave endowment identity in
  credits** and holds *only if* `g > 0` — a strictly positive **credit-denominated cost
  decline** (`E_o(0) ≥ λ·S·c/g`). 2020s hardware evidence says `g` may be going to zero
  (HDD $/TB plateaued). So silt ships durability as an **explicit finite-but-renewable
  contract** (fund a horizon `T`, auto-skim to extend it, re-endow before expiry, publish
  the funded horizon per object) — solvent for *any* sign of `g` — and treats "perpetual"
  as a claim silt *earns only if measured `g` stays positive*, never an architectural
  promise. **Instrument `g` (credit-cost of one shard-repair, per year) as the single number
  that decides perpetual-vs-finite.** *(Built: the per-object `DurabilitySnapshot` and the pure
  `credit.CostPerRepair` / `Horizon` / `G` instruments — `g > 0` = cost declining = solvency-
  favourable, always measured, never assumed.)* Correlation to watch: the same cost regime that breaks
  cold-data solvency (`g ≤ 0`) also cheapens Sybil standing (one ledger) — provision the two
  as *correlated*, not independent.

## D-TAKEDOWN — provable non-globality via a transparency log

- **Status:** ▶ DIRECTION DECIDED — 2026-08-05 (low urgency); **metric CONSTRUCTED** 2026-08-06;
  **CT-log accumulator BUILT (#180, 2026-08-09)** — `core/translog`, an RFC-6962 append-only
  Merkle log with inclusion + consistency proofs (adopted, not invented), exhaustively tested. It
  is the M0-honest core of the transparency layer (prove a takedown was recorded; prove history was
  never silently rewritten). The ZK non-globality PREDICATE + PIR-routed probes on top of it are
  post-M0. **Wired into the chain (#180):** every honored revocation and un-revocation is appended
  to the log in `Chain.apply` (a deterministic function of the committed blocks, rebuilt identically
  on replay), and the chain exposes `RevocationLogRoot` + inclusion/consistency proofs +
  `RevocationLeaf` so an auditor can reconstruct a leaf from public block data. So silt can now
  *prove* a takedown was recorded and that its takedown history was never silently rewritten.
- **Research basis:** Memo 04 (pluralistic takedown). A mechanism strong enough to
  *guarantee* content is gone everywhere *is* the global kill switch silt outlawed; every
  deployed system resolves this by *not* guaranteeing global removal except a legally-forced
  sliver. Pluralism re-centralizes in practice (one default labeler becomes near-universal).
- **Direction (derived):** Adopt the memo's priority order as the H9 roadmap direction — a
  **signed, subscribable revocation/label layer** as the primary mechanism (the quorum
  chain is one high-weight labeler among several), every honored revocation committed to a
  **Certificate-Transparency-style append-only log** with inclusion/consistency proofs (so
  silt can *prove* it never silently or globally censored), threshold/quorum signing on
  revocations, and a **narrow, opt-in, hash-based denylist** scoped to the legally-forced
  sliver and itself committed to the transparency log. Avoid perceptual hashing as a primary
  filter and any single default labeler as the only trust root.
- **Construction (DELIVERED — `A2-non-globality-metric.md`):** the **formal non-globality
  metric** — a proof/measure that a takedown was *not* global — now has a construction.
  Define **NonGlobality(h, A) := the minimum number of independent failure domains an
  adversary of class A must simultaneously compromise to drive the surviving decodable
  replica set below the RS recovery threshold** (a *survivor Nakamoto coefficient*,
  adversary-relative, correlation-aware, composable with the erasure code). silt publishes a
  *certified lower bound* `NonGlobality(h, A) ≥ t`. The **discovery-oracle problem** (every
  measurement of *where* survivors are is a map that helps the censor finish the job) is
  defeated by a **ZK threshold predicate**: prove "≥ t distinct-domain, PoR-fresh,
  bonded survivors exist" over committed attestations in the CT-style log, revealing **only
  the scalar `t`** — never the survivor set, addresses, or shard indices. Layered with
  anonymous/aggregate attestation, PSI for audit queries, DP for coarse diversity, and
  PIR-routed probes. → H9.
- **Honest limit (carry, don't hide):** `t` is *only as real as the independence oracle* —
  crypto proves *distinct labels*, not *true physical/legal independence* (shared upstream
  transit, one cloud region under two brands, treaty-linked jurisdictions), and that oracle
  (RPKI/whois/geolocation) is non-cryptographic and gameable. The residual leak is
  irreducible: `t`, its trend over time, and ε-noised coarse diversity — the price of a
  *checkable* claim at all. Stays **low urgency** (per D-TAKEDOWN priority).

## D-DEMAND — standing is priced on cost-to-wash, never on receipt count

- **Status:** ▶ DIRECTION DECIDED — 2026-08-06; **P0 + P1 BUILT (#181, 2026-08-08)** — the receipt
  primitive (`core/demand`: issue → PoR-bound delivery-ack → bank → redeem) with the
  unforgeability-at-the-token-level red-team, and **blind token withdrawal** (issuer blind-signs
  the token without seeing its serial → unlinkable to the withdrawal). **NEUTRAL by construction**
  (a redeemed receipt records witnessed demand as an observable, never wired to standing — so even a
  forged/self-dealt receipt buys zero standing; the γ→1/N firewall holds). Fetcher-unlinkability
  stays nominal until D3 (needs H8); P2 (fair-exchange dispute) / P3 (cost-to-wash economics +
  self-dealing red-team) remain.
- **Research basis:** `B2-demand-receipt.md`. The blind demand receipt is the load-bearing
  interlock between the Sybil corner (standing must track *witnessed* demand, not
  self-declared popularity) and privacy (who-fetches-what stays unlinkable). It **splits
  cleanly** into what is achievable and what is not:
  - **Achievable + composable from primitives silt already ships:** an unlinkable delivery
    receipt = blind-withdrawn retrieval token (Chaum / Compact E-Cash) + a PoR-bound
    `delivery-ack` (Shacham–Waters binds it to the *correct object C*) + optimistic fair
    exchange with the **validator quorum as the threshold-distributed TTP** (Asokan–
    Shoup–Waidner; fair exchange provably needs *a* TTP — Pagnia–Gärtner). This gives
    **unforgeability-without-served-bytes** (`#receipts for C ≤ #completed paid correct
    deliveries`) **and fetcher-unlinkability** simultaneously — both provable.
  - **NOT achievable by any receipt — a Douceur limit, not an engineering gap:** **demand
    *authenticity*.** A server can run its own fetchers, pay itself, fetch its own content,
    and mint perfectly valid receipts; a self-fetch *is* a real paid correct delivery.
    Unlinkability makes this *strictly worse* (it hides that one entity is on both ends). No
    cryptographic primitive certifies the counterparty was economically independent (the Tor
    proof-of-bandwidth line failed at exactly this).
- **Direction (derived):** price standing on **cost-to-wash, never on raw receipt count**
  (mirrors the C2 rule "shed on cost-to-corrupt, not head-count"). Since authenticity can't
  be *proven*, **re-price** wash so it stops being free, via two levers:
  1. **Burn/escrow the fetch fee** — pay the retrieval token in a scarce unit that does
     *not* flow back to the server as revenue (burned, or escrowed to the repair pool). Wash
     N times costs N real fees with no offsetting income; wash is loss-making per loop *iff*
     the standing-reward per receipt is priced below the burned fee. **The single most
     important knob** — an economic parameter, not a proof.
  2. **Bonded-fetcher credential** — count a receipt toward demand only if the (unlinkably
     shown) fetcher carries a scarce, bond-distinct reputation credential, pushing wash cost
     onto the *fetcher-identity* supply the G2 bond already prices. Re-prices wash to "one
     bonded fetcher identity per unit of fake demand" — the best achievable under no-center.
- **Doc-truth rule:** any claim that the receipt *proves* real, organic, third-party demand
  is **false and must be struck** — it proves *a paid correct delivery happened*, unlinkably.
- **Build (prototype-first, P0→P3):** ✅ **P0 + P1 built** (`core/demand`). P0: issue → PoR-bound
  delivery-ack → bank → redeem, single object, with the unforgeability red-team (forged token,
  tampered/lifted receipt, wrong-object, data-less delivery, double-spend — each rejected; tag-forgery
  and authenticity residuals documented). P1: the retrieval token is **blind-withdrawn** under an
  issuer blind signature (`blindtoken` demand domain — `Withdraw → SignWithdrawal → Unblind`), so the
  issuer signs it without learning the serial → the redeemed token is cryptographically unlinkable to
  its withdrawal. **Wired into the node** (`core/node/demandrole.go`): a fetcher
  `AcquireDemandTokenInWindow` on the per-epoch demand lane (R0.4b — the original
  `AcquireDemandToken` over the shared token-request wire is deleted: the publish key never
  enters the demand keyset), then `SubmitDeliveryReceipt` (a `MsgDeliveryReceipt` carrying
  the token + PoR-bound ack) to the server, which banks it into a **neutral witnessed-demand
  observable** — never standing; replays and forged/mis-issued tokens are rejected
  over the wire. *(SUPERSEDED 2026-09-08 by R2.9's anchored session lane: the token is spent at
  session OPEN, deliveries are acknowledged by cumulative-count `SessionReceipt`, and the observable
  is `WitnessedIncrements`. B-9 (#764) retired the flat path at the node; C1 deleted the primitive —
  `Bank.Redeem`, `SubmitDeliveryReceipt`, `DeliveryReceipt`, `WitnessedDemand`. The property is
  unchanged and restated at credit level as P-SESSION.)* **◑ P2 optimistic fair exchange — the abort-SAFETY floor is built + regression-locked
  (`core/demand/fairexchange.go`); the dispute-RESOLUTION half is gated on threshold crypto silt does
  not ship.** Built: the ASW optimistic phase (`ExchangeCommitment` — a fetcher's pre-release,
  non-repudiable promise) + both abort-safety properties, which hold structurally today — (1)
  fetcher-side: an aborted exchange never CONSUMES the token (spent only at a completed session open), so a
  non-delivering server leaves the paid token reusable elsewhere; (2) server-side: a pre-release
  commitment is domain-separated from the receipt and carries no PoR, so it can NEVER redeem as demand
  — `#receipts(C) ≤ #completed correct deliveries` survives the abort path. GATED: converting a
  server-held commitment into a TTP-affidavit on fetcher default needs the quorum-TTP to verify
  delivery completed without the fetcher — i.e. **verifiable escrow of the content key (Camenisch–
  Shoup) + threshold decryption t-of-n across the validators**. The threshold-decryption/DKG half IS
  available in Go (dedis/kyber, drand-grade); the wall is the **verifiable-escrow primitive — no
  adoptable audited pure-Go impl — plus the large new crypto trust surface of the whole stack**,
  disproportionate to a NEUTRAL observable. (Same *strategy* as H7 — floor now, heavy crypto as a
  fast-follow — different *primitive*: H7 was a char-2 field-algebra wall, this is a missing
  verifiable-escrow lib.) Demand-NEUTRALITY keeps this low-stakes: an unresolved server-side abort only
  UNDERCOUNTS a neutral observable (never standing), so the missing affidavit path costs no security
  today. Held in tension; `ExchangeCommitment` is the exact seam the future threshold-crypto resolver
  consumes. **✅ P3** — BOTH cost-to-wash levers now built +
  regression-locked. **P3a fee-burn** (a self-dealing sim: a server running its own fetcher mints N
  valid receipts — authenticity is *not* provable, Douceur — but each burns a real retrieval fee, so
  cost-to-wash = N·fee for zero standing, since demand is neutral). **P3b bonded-fetcher credential**
  (`demand.Bank.RequireBondedFetcher` / node `RequireBondedFetchers`): a receipt counts toward demand
  only if the fetcher's key is bond-distinct in the COMMITTED on-chain bond ledger (`chain.IsBonded`,
  the same Sybil-priced supply C2 measures), and demand counts DISTINCT bonded fetchers per object —
  so one bonded identity washing N receipts moves demand by 1, re-pricing wash to *one real storage
  bond per faked unit* (the best achievable under no-center). Self-dealing red-team at both the pure
  layer (`core/demand`) and the real node wire (`sim`: one bonded identity washes N → demand 1;
  unbonded delivery → 0; a distinct bonded identity → +1). **Property (b) fetcher-unlinkability —
  D3 issuance-mixing (H8/#179):** ◑ **slices 1+2 BUILT** — `client.WithdrawDemandTokenPrivately`
  withdraws over a FRESH EPHEMERAL identity paying with a prepaid blind credit (slice 1 — issuer
  authenticates only an unlinkable ephemeral key, not the durable NodeID), and given a relay-form
  issuer address dials the issuer THROUGH a content-blind relay (slice 2 — issuer sees the relay's IP,
  not the fetcher's; end-to-end TLS still authenticates the ephemeral key across the relay pipe). Both
  proven over real TCP (`client/privissue_test.go`). Also fixed a latent bug where `tcpnet` dropped the
  `Credit` field over the wire, so the F4/D3 fee decoupling had only ever worked in the sim. ☐ timing-
  correlation (epoch-batching) deferred to the post-M0 H8 mixnet. The blind signature already hid the serial.

- **R2.9 restatement — ✅ RATIFIED 2026-09-06 (D-R2.9-NODE-HALF-CALLS call 2).** On the anchored session lane the token is
  spent at session OPEN and funds up to ⌊f/p⌋ acknowledged increments, so the token-level property above is
  FALSE there and is restated at the CREDIT level (P-SESSION): `demand_S(C)·p ≤ Σ credits settled at S on
  fetcher-signed acknowledgements naming C ≤ Σ face spent into S's guard`, and per fetcher over the ledger's
  life `Σ_C demand·p ≤ its grant`. Stronger in form (aggregate over all receipts and objects), weaker in level
  (one token buys 50,000 units). The counting rule is CERTIFIED (`demand += settled/p`, two surfaces, v2/v3 never
  shared); the restatement of this decision's published property is ratified. Certification:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R2.9-witnessed-demand-observable-under-sessions-RESEARCH-CERTIFICATION-2026-09-06.md`.

## D-C2 — "no quiet capture" is held in tension, never closed (by theorem)

- **Status:** ▶ DIRECTION DECIDED (held-not-closed) — 2026-08-06; **METRIC WIRED (#185,
  2026-08-08).** Promoted to a first-class entry (was buried in the "not on this ledger" tuning
  list) because it is one of M0's two Sybil corners and the strongest *held-in-tension* result —
  it must be tracked, not assumed. The concentration measurement is now first-class
  (`chain.C2Metric()`), computed from the committed bond ledger and consumed by the shed
  (details in the Direction bullet); it stays *held-in-tension*, not closed.
- **Research basis:** commission memo B1 (C2 / no quiet capture). C1 (no discount) can be a
  theorem; **C2 can never be** — Kwon et al.'s impossibility makes assigning a Sybil cost to
  *identity-splitting* impossible without a trusted authority, so operator-clustering (keys →
  independent operators) is **heuristic at its base, by theorem, not by implementation
  weakness.**
- **Direction (decided):** measure concentration as **cost-to-corrupt / Nakamoto-coefficient
  over bond-distinct *operators*, Byzantine-robustly sampled**, and shed the anchor
  training-wheels only when the measured count clears the target *with margin*. The achievable
  bound is **`k* ≥ k̂ / M`**, where `M = M_cluster · M_est · M_sample` is a
  concentration-underestimate factor (> 1); so **shed only when `k̂ ≥ k · M`.** The single
  sharpest engineering lever: **compute the weight numerator from the committed on-chain bond
  ledger, not gossip** (kills the gossip-skew half of the skew+split attack) — this is the
  measurement filed as the C2-metric-wiring build item, and it is *the same number* consumed
  by the consensus shed, the private-lookup committee certification (H8), and C2.
  - **Built (#185):** `chain.C2Metric()` computes `{NakamotoBonds, NakamotoOperators,
    CostToCorruptBytes, TotalBondedBytes, Margin}` over the participating **committed** bonds
    (weight numerator was already on-chain — this makes it a first-class, published measurement,
    not a private shed-helper). Since a `BondReg` carries **no operator label**, real key→operator
    clustering is impossible on-chain, so the M0 stand-in for `M_cluster` is a **config operator
    margin `M`** (`OperatorMargin`, default 1): the shed gates on the discounted
    `NakamotoOperators = ⌊k̂/M⌋`, i.e. it sheds only when `k̂ ≥ k·M` — exactly the decided rule.
    `Mature()` consumes the same measurement; `chain-status`/daemon publish it. **Still future:**
    the private-lookup committee-certification consumer (lands with H8/#179), and any
    Byzantine-robust *sampling* (`M_sample`) — today the metric is over the whole committed set,
    not a sample.
- **Honest residuals (tracked, not closed):**
  - **The honest whale / real cartel** — an actor who *genuinely* provides φ of the disk
    across infra-independent nodes and then coordinates — is **outside C2 entirely**; bounded
    only by the HHI/Gini concentration veto + the cost-to-corrupt-vs-profit co-trigger + the
    anchor training-wheels, none of which is a Sybil bound. This is the wealth residue C1
    cannot touch.
  - **`M_est` under adversarial NodeID placement is unquantified** — the CPR estimator's
    `O(n^{1−δ})` Byzantine tolerance is proven only for *random* placement; a stake-splitter
    chooses its NodeIDs, degrading it by an amount **the literature does not characterize**
    (a flagged research gap, also a risk-register row).
- **Build items:** the C2-metric-wiring issue (Nakamoto-over-bond-distinct-operators from the
  committed `BondReg` ledger; the external red-team #183 that attacks C2 is blocked-by it).

## D-DISCLOSURE — no decryption backdoor at the core layer

- **Status:** ▶ DIRECTION DERIVED — 2026-08-05.
- **Research basis:** Memo 04 §3.6 (accountable disclosure): threshold decryption gives
  quorum-gated de-blinding but reintroduces capture/coercion risk (*who* holds shares under
  *what* legal process becomes the attack surface). Composed with the immutables — B4
  (content-blind by construction) and T3 (the Aslan naming boundary) — and the fresh-eyes
  legal analysis (the content-blind firewall *is* the operator liability shield): a core
  decryption capability would pierce the exact shield silt exists to hold.
- **Direction (derived):** **Never at core.** Silt core holds no capability to decrypt stored
  content and ships no threshold/quorum decryption of it. Accountable disclosure, if it ever
  exists, is an **Aslan-layer** (application/resolver) choice made by parties who can already
  read, never a core capability. This is a bright line, consistent with the content-blind
  firewall; it is a *values/legal* call, not a research question.

> **See also:** [`design/primitive-availability-gaps.md`](design/primitive-availability-gaps.md) — the
> consolidated index of primitives silt *would* adopt but for which no mature pure-Go
> implementation exists in 2026 (blind proof-of-repair, threshold decryption/DKG, verifiable
> encryption, a ZK threshold predicate, a continuous identity-chained VDF). Each is recorded
> inline in its decision below; that page puts the cryptographic dependency surface in one place.

## D-CRYPTO-AGILITY — a stated post-V1 track, not a V1 gate

- **Status:** ▶ SCOPE DERIVED — 2026-08-05.
- **Basis:** Gap inventory P1 (harvest-now-decrypt-later): durable ciphertext is
  retro-decryptable if SHA-256 / Ed25519 / AES fall; no crypto-agility/migration framework
  is built. No research memo covers this — but the *scope* call is a priorities question,
  not a research one.
- **Direction (derived):** Explicitly **defer to post-V1** and say so, rather than leave it
  silently open. It is not M0-blocking; PQ migration is a known engineering pattern. If we
  later choose to *build* it, the design (agile primitive negotiation, ciphertext
  re-wrapping under churn) would warrant a research pass then.

## D-ANCHORS — launch anchor set is a launch-config decision

- **Status:** ▶ DEFERRED to launch-config — 2026-08-05.
- **Basis:** Memo 05 already gave the *mechanism* (anchors plural + threshold, shedding on
  the Nakamoto/cost-to-corrupt shed metric — shipped as H4) and immutable #3 governs it.
  *Who* the launch anchors are, how many, and the exact threshold depend on who is actually
  running nodes at launch — an operational decision, not a research or architecture one.
- **Direction (derived):** No decision needed now; defer to the launch-config window. Not a
  pre-red-team blocker.

---

## D-C1-TIMING — the partial-storage timing deterrent is soft, never a hard standing gate

- **Status:** ✔ DECIDED + BUILDING — 2026-08-10 (build-immutables #3/#4 ratified; PRs #297 decouple, #298 soft gate).
- **Basis:** An external network-durability-vs-space-time research opinion (provoked by adverse-network
  field-testing, `integration/flakynet`, #289) established that a wall-clock reply-latency **hard gate**
  is a category error on the open internet — reply-latency is transport (RTT + jitter + loss) **plus**
  compute, network delay is one-sided (it can only *add* latency, so "slow ⇒ cheat" is unsound), and
  **no mature PoST network** (Filecoin/Storj/Chia/Arweave/Sia/Spacemesh) reply-latency-gates. It read
  jitter/loss as a partial-storage cheat and starved durability.
- **Direction (derived, now canon as build-immutable #3):** Standing rests on the **sound** signals —
  the anti-release floor (a **compute** window decoupled from the transport timeout, #297), identity
  binding, and the space/labeling proof. The partial-storage timing signal is a **soft, disclosed**
  deterrent: the windowed-**minimum** (low quantile) of each peer's reply latencies, which filters the
  one-sided noise, flagged only when *sustained* above the deadline — never a standing gate (#298). The
  anti-release floor stays **small** — never scaled off a transport timeout (build-immutable #4).
- **What would close it (H-track, not M0):** a stacked tight-PoS + SNARK (owned-residual A5, Option A) —
  the same structural close named there. A companion residual: the ~1.5 MB bond proof reply (loss +
  N² bandwidth, [#299]) whose close is also succinct-proof / H-track.

---

## D-CONSENSUS — consensus is boring and invariant-gated; the novelty budget is spent on M0

- **Status:** ✅ DECIDED — 2026-08-14 (owner ratification, dialogue session).
- **Basis:** the PE process review (`~/.claude/silt-agent-memory/principal-engineer/reviews/builder-process-notes-PE-2026-08-14.md`)
  and the research team's **independent same-day convergence** on the same diagnosis
  (`silt-agent-memory/researcher/reviews/research-outcome/INTERSECTING-QUORUM-INVARIANT-note.md`, plus the
  #402 certification `fork-anchor-gate-402-RESEARCH-CERTIFICATION-2026-08-14.md`): the four
  RC-blocking consensus bugs (#357, B2-handoff, #397, #402) were **one defect — a finality
  quorum that did not intersect over its phase's real validator set** — discovered one
  billable field run at a time because the invariant set was never written down.
- **Direction (decided):**
  1. **`docs/design/consensus-invariants.md` (I1–I5) is ADOPTED canon**; every
     consensus-touching PR states which invariants it touches; every quorum site answers the
     research six-question checklist in its comment.
  2. **The #402 fix is the certified strict anchor majority** — launch finality needs
     `⌊A/2⌋+1` anchors (=3 of 4) counting the proposer-if-anchor, **sybils excluded from the
     launch finality count**; the consult's `⌈A/2⌉` is rejected (off by one for even A —
     admits a both-sybil-proposed 2-2 anchor split that the finality gate then cements into a
     permanent partition). **Encoding (B)** chosen: anchor-only launch proposing, sybils
     drain via `MsgSubmitBondReg` (composes with #397 submit-don't-propose; removes the
     sybil-proposed fork at the source). Fault tolerance unchanged (3-of-4 up).
  3. **The consensus model-check is the first consensus gate**
     (`docs/design/consensus-model-check.md`, ADOPTED): tier order `unit → model-check →
     sim → netem → field`; each graded field run is gated on the tier covering its regime
     (launch tier → P1; handoff tier + #399 → MATURING; full budget → red team #183). v1
     ships seeded replay; auto-shrink is a follow-up.
  4. **I4 (commit ≠ final) is a permission, not a build mandate** — research twice ruled no
     decoupling is needed once the finality quorum intersects; a decoupling is built only on
     model-check evidence of an I4 violation (#7).
  5. **No consensus-engine rewrite.** "Boring" means literature-faithful hardening of the
     existing chain with cited analogues, not a CometBFT swap — the chain is load-bearing
     for the bond/standing composition.
- **Also decided (owner, same session) — the documentation-reconciliation pass:** the doc
  surface accreted one viewpoint per bug arc and no longer reads as one current plan. Before
  the next graded field run: (a) the closed consult arcs move to `/archive/` (16 files now,
  5 on short conditions — list in the session record); (b) **every live doc gets a revise
  pass** against tonight's decisions (`test-topologies.md` and `v1-test.md` explicitly
  flagged — the tier order changed); (c) **ROADMAP, backlog, and GitHub issues are
  reconciled** to reflect the actual sequence (fix #402 → model-check tiers → P1 → MATURING
  + #399 → red team #183). The live tree carries exactly one current viewpoint; history
  lives in `/archive/`.

---

## D-M1-PIVOT — the ordered roadmap; M1 (economy + operability) interleaves with the M0 tail

- **FRAMING SUPERSEDED-BY-Boulders (2026-09-01).** The "ordered path with phase
  gates" / "the ordered path is the track" PACKAGING below is retired: ROADMAP.md
  now carries the task order as the Boulder/Rock lattice (the single task SSOT), and
  the numbered Phases + "the ordered path" phrasing are retired to
  `/archive/roadmap-history-2026-09-01.md`. Only the framing is superseded — the
  DECISION itself stands (M1 economy + operability interleaves with the M0 tail;
  storage-economy-first; the standing-firewall unchanged; the trust harness never
  softens). Read the clauses below that say "the ordered path is the track" as the
  historical 2026-08-19 framing, now expressed as the Boulder spine.
- **Status:** ✅ DECIDED — 2026-08-19 (owner ratification, fresh-eyes-audit session).
- **Basis:** the fresh-eyes audit
  (`docs/thinking/2026-08-19-fresh-eyes-audit-and-the-m1-pivot.md`), which verified every
  "shipped" claim against code and tests: the trust plane verifies end-to-end and the M0
  tail is **small and enumerated** (two DoS gates, a confirming deep run, the #183
  engagement itself), while M1 has **structural holes with zero recent effort** — the
  S7 economy is built + test-proven but **default-off with no enable path** (`RepairBountyBase`
  never set outside tests; `FundDurability`/`EnableDemandBank` have no non-test callers;
  `credit.G` never computed on live data), bandwidth is unpriced, and the operational floor
  (packaging, cold-start, reprovide cost) prices out the honest operator in practice.
- **Direction (decided):**
  1. **ROADMAP.md carries an explicit ORDERED task order** (2026-08-19 framing: an
     "ordered path with phase gates," now the Boulder/Rock spine — see the
     SUPERSEDED-BY-Boulders marker above). "Tenets are the roadmap" was too loose — it
     let effort pool on one axis with no rebalancing force. Tenets remain the
     destination; the ordered task spine is the track.
  2. **The prior sequencing rule "M1 opens only after the M0 gate" is SUPERSEDED.** The
     economy-enablement and height-cost work interleave with the M0 tail, because (a) field
     runs stall short of depth (h64) partly on M1 costs — heavy per-reg proofs, round
     durations, the 360 s publish bound (since re-derived to 300s — see #609) — so cheaper heights are the path TO the M0 field
     confirmation; and (b) **#183 must red-team the economy-ON config** — certifying the
     economy-off HEAD would certify a network nobody will run.
  3. **Storage economy first, Proof-of-Delivery second.** Economy-ON is enablement of
     existing tested code (days). PoD has a crypto prerequisite — the demand receipt is
     forgeable with zero object bytes (owned-residuals B3), inert today only because demand
     has no consumer — so PoD gets a spec + research consult before code, never a switch-flip.
  4. **The standing-firewall is unchanged by the pivot:** delivery/durability credits never
     confer consensus standing (D-S7 coin-free standing; the γ→1/N fence, #182).
  5. **The trust harness never softens** (unchanged from the prior M1 ruling): cost budgets
     overlay the same runs; no security gate is relaxed for M1.
- **Construction (open, named):** the PoD receipt hardening (bind receipts to served
  bytes); wash-pricing parameters (D-DEMAND); relay compensation; the per-platform
  service/installer + R4-compliant self-update.

---

## D-TIERING — heterogeneous node roles, one client; the state commitment is the keystone

- **Status:** ✅ DECIDED (direction) — 2026-08-25 (owner ratification). The consensus-rule
  construction (the state root) is **research-gated** and NOT decided here.
- **Basis:** the PE design direction
  (`silt-agent-memory/principal-engineer/reviews/D-TIERING-design-direction-2026-08-25.md`), verified
  against code at `7089d27`: no state-root field in `Block` (core/chain/chain.go:295), the
  registry is unsharded (`AllEntries`), state is rebuilt by pure replay, and the WS
  checkpoint is a trust anchor only — so cheap-but-correct participation is impossible
  today without holding and replaying everything.
- **Direction (decided):**
  1. **One binary, composable capability flags** (`--serve-content`, `--validate`,
     `--archive`, later `--registry-shard`) across a spectrum from transient hobbyist edge
     box (1 vCPU / 2 GB) to archival trust server. Resources determine role; the mode is a
     flag, not a fork.
  2. **Three couplings are canon:** (a) transient boxes serve but never attest — the
     attesting set is drawn from persistent bonded nodes (the #535 churn-stall class is the
     failure prevented); (b) durability is guaranteed by the persistent tiers plus the D-S7
     economy, never by the transient edge (the cold-content death spiral is the failure
     prevented); (c) contribution scales publishing allowance freely but consensus weight
     only through the bond under the C2 cap — the γ→1/N firewall (Invariant A) is
     untouched and reasserted in a failing-first guard whenever a contribution-unlocks-
     publishing mechanic lands.
  3. **The single new load-bearing build item is a registry state root committed in each
     block** — an additive block field plus a validity check, version-gated (era-3), NOT a
     consensus-engine change (D-CONSENSUS §5 holds). It unlocks, in order: cheap correct
     validation on pruned nodes, snapshot sync (O(live-state) bootstrap), and the sharded
     registry.
  4. **Sequencing:** the Phase-3 deep-heights gate finishes first; the state-root research
     consult runs in parallel
     (`silt-agent-memory/researcher/reviews/D-TIERING-state-root-keystone-CONSULT-2026-08-25.md`); mode
     flags and neutral PoD are build-gated items that start after the deep gate is banked.
     #563 is scoped minimally (RED bench + bounded mitigation) and #559 folds into the
     snapshot-sync design, because snapshot sync is the structural fix for both.
- **Construction (open, research-gated):** the authenticated structure (sparse vs
  sorted-key Merkle) with inclusion AND exclusion proofs; the incremental-update algorithm
  (O(changed × log n) per block, never a per-block recompute — the #555 lesson); the full
  enumeration of validity-relevant committed state (16 fields at this HEAD, incl. the
  regime latches and #506 gate state — see the consult); the era-3 upgrade boundary; the
  sharded-registry can't-lie-by-omission model; the validator churn floor.
- **REFINED 2026-08-27 — TWO roots, not one** (research certification
  `.../research-outcome/597-revlog-history-dependence-RESEARCH-CERTIFICATION-2026-08-27.md`,
  which owns this as a correction to its own round-9 phrasing). "One root over all
  committed state" was right about *scope* and wrong about *structure*: it implied one
  **structure**. The precise rule is **one history-independent SMT over all set-valued
  validity state, PLUS a separate append-only (RFC-6962) root for any committed ordered
  log.** The SMT choice is untouched — `revLog` was never set-valued state; it is a second
  committed root of a second kind (the Ethereum shape: stateRoot + receiptsRoot + txRoot).
  Folding an order-derived value into the state root is a **category error** that would
  make the state root depend on history order and break the very history-independence
  argument that selected the SMT. The era-3 snapshot therefore carries the **full revLog
  entry list**, which preserves H9 inclusion/consistency proofs for snapshot-booted nodes
  at the cost of the smallest forever term. `epochStart` is an **observable** — history-
  derived and reorg-swapped, but read only by `Regime()`, so under no committed root.
  **The general rule this sets:** an observable is committed *iff* you want it
  consensus-anchored, and it is committed **under a root whose structure matches its
  data**. Enforced mechanically by the order-varying oracle
  (`core/chain/modelcheck_order_independence_test.go`), because classification alone
  cannot catch a purely order-derived value.
- **DECIDED 2026-08-27 — the floor-box validator validates BY PROOF, not by holding the
  tree** (PE ruling `.../principle-engineer/RULING-keystone-node-store-dependency-2026-08-27.md`
  Q5; Andrew concurred). This is a **decentralization posture**, deliberately NOT settled by
  a storage default. The keystone's whole purpose is *cheap correct validation by proof*
  (D-TIERING §7); if a 1 vCPU / 2 GB validator had to hold the full state tree, the keystone
  would have failed its own promise — the `AllEntries` OOM simply relocated into the SMT
  (~2.2 GB at 10M entries, growing with all-content-ever). **End state:** the floor-box
  validator is **tree-free and semi-stateless** — it verifies a block's root transition from
  proposer-supplied **witnesses** (inclusion/exclusion proofs) against the root it already
  trusts (the Ethereum stateless-client shape); the disk-backed store is a **tier-above**
  concern (proposer / full-registry / archival), which computes roots and generates
  witnesses. **Near-term bridge, stated honestly:** witness-based validation is NOT built,
  so until it is, a validating node holds the tree and the disk-backed KV store is what makes
  the tree fit the box at all — but shipping that store must **never silently redefine #8
  upward** to "validation requires 2.2 GB of state," which would price the floor box out of
  validation as the registry grows. **Consequence for the era-3 format:** it must be frozen
  knowing this direction, so the block/gossip can carry or reconstruct witnesses; a format
  assuming stateful floor-box validation is the thing to avoid. Witness-based validation is
  opened as the Phase-3+ keystone follow-on (witness soundness, a size bound so a malicious
  proposer cannot DoS an attester with huge witnesses, and who generates witnesses).
- **RATIFIED 2026-08-27 — witness-based floor-box validation is CERTIFIED sound + complete**
  (research certification
  `.../research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`;
  full path
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`).
  **Verdict:** witness-based validation of silt's set-valued validity state against the
  committed state root is sound and complete in the stateless-client sense (membership +
  non-membership proofs against the committed, quorum-attested root; the SMT's exclusion
  proof is the load-bearing piece, proven by execution in
  `internal/smtspike/exclusion_test.go`). **Consequence:** soundness is no longer a reason to
  hesitate on the #600 direction (the floor box validates by proof, not by holding the tree);
  the direction remains Andrew's decentralization-posture call, which C-7 does not decide.
  The one banned implementation move is named as an invariant below.
- **HARD FREEZE PREREQUISITE 2026-08-27 (era-3 format) — from C-7 Q3, ratified.** The era-3
  `Block` MUST commit BOTH roots before the format can freeze: (a) the state SMT root over
  the set-valued validity state AND (b) the separate append-only (RFC-6962) transparency-log
  root — the #597 two-root shape — as Hash-covered, attester-signed block fields. It MUST
  commit the state root over the **completeness- and order-independence-proven field set**;
  the freeze stays hard-gated on the consensus-weight fields (`bonded`/`epochSet`, then
  `spent`/`slashed`) reaching the keystone oracles green (issue #603). AND the floor-box
  verifier MUST carry the invariant **"no witness supplied for a key a predicate reads →
  never accept (reject / stall)"** — accepting on a missing witness is the one move that
  inverts the safe-degradation proof. **State of the block today (per the cert):** it commits
  NEITHER root — no state-root or log-root field exists (`core/chain/chain.go:311-405`; the
  `Root` at `:419` is the bond commitment, not a state root). A sound witness scheme cannot
  exist until the root it verifies against is a committed, attested block field, so this is a
  hard prerequisite for the witness path, not an optimization.
- **RATIFIED 2026-08-28 — the era-3 committed state-root block format** (research certification
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/era3-committed-state-root-format-RESEARCH-CERTIFICATION-2026-08-28.md`;
  PE ruling
  `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-era3-committed-state-root-format-2026-08-28.md`;
  design `docs/thinking/2026-08-28-era3-format-design-options.md`). The composed two-root
  format is **CERTIFIED-WITH-CONDITIONS** and Andrew ratified it with the mint correction. The
  ratified shape: two flat, required, attester-signed block fields — `StateRoot` (a
  history-independent `pokt-network/smt` v1.0.0 SMT over the 16 `committedSet` fields, a
  field-tagged single keyspace with a per-field-class canonical VALUE encoding) and `LogRoot`
  (the existing RFC-6962 MTH over `revLog`) — both inside `Hash()`. **Mint `BlockVersion = 4`,
  not 3** (the certification REFUTED minting 3: `versionSupported` already decode-accepts a v3
  block and would validate an era-3 block under era-2 rules with no state-root predicate,
  silently accepting a forged root; `BlockVersionRegGate = 3` stays the #506 reg-gate readiness
  threshold, era-3 activation gates on a distinct `regVersion >= 4` supermajority). This is a
  **HARD FORK** (an un-upgraded binary rejects a v4 block at decode, LOUDLY; a laggard stalls at
  the boundary rather than accepting unvalidated roots — the safety-first behavior). **The
  value encoding is a CONSENSUS PARAMETER, not formatting** (cert Q2/Q6, PE Q2 highest
  severity): three super-quorum predicates SUM `bonded`/`epochSet` weights, so a
  true-presence/wrong-value witness is a consensus-SAFETY attack; the per-field byte encoding
  (8-byte big-endian for the int64/uint64 weights and heights, raw 32 bytes for `bondRootOwner`,
  one byte for bools/`regVersion`) is pinned in
  `docs/thinking/2026-08-28-era3-state-root-value-encoding.md` and any width/endianness change
  is an era bump. **The five freeze conditions (all must hold before the format freezes):**
  (1) mint v4 + extend `versionSupported` to `<= 4` in the same release;
  (2) #603 green (the `bonded`/`epochSet`/`spent`/`slashed` oracle probes);
  (3) the fixed canonical value encoding pinned + a byte-identical-leaf cross-node determinism
  oracle proving it at the model-check tier BEFORE the root is a signed field (residual R2);
  (4) empty-tree and empty-log roots are fixed constants, both root fields REQUIRED (not
  omitempty); (5) record the two witness freeze constraints (root-is-an-attested-field;
  witness-serving-stays-open-and-multi-provider) so the C-7 witness follow-on is not precluded.
  **The freeze is HELD until the coverage gate (#603) is green** — the trigger is Andrew's to
  pull. **Build order (certified, step 1 landed):** step 1 computes the two roots and proves the
  encoding deterministic behind the keystone oracles (no `Block`/`Hash()`/`BlockVersion` change,
  no validity predicate — `core/statehash` + `core/chain/statehash.go`, the determinism oracle,
  and the order-independence/snapshot-equivalence oracles extended to assert ROOT equality); the
  field addition, the validity predicate, and the height-gated activation are later steps that
  re-trigger certification. This discharges the HARD FREEZE PREREQUISITE above's format half
  once #603 lands.
- **FROZEN 2026-08-29 — the era-3 committed state-root format is IMMUTABLE as of build
  `3af40bc`** (composed re-certification
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/era3-committed-state-root-format-BUILT-RECERTIFICATION-2026-08-29.md`;
  the design certification it re-certifies,
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/era3-committed-state-root-format-RESEARCH-CERTIFICATION-2026-08-28.md`).
  The composed, BUILT two-root format — shipped by PRs #627 (step 1 root computation), #629
  (step 2a schema + v4-accept), #630 (step 2b validity predicate on every path), #631 (step 2c
  activation + mint-flip) — is **CERTIFIED FOR THE FREEZE** and **Andrew ratified the freeze on
  2026-08-29**. What is frozen, precisely:
  - **The block schema.** `Block.StateRoot`/`Block.LogRoot` (`*ports.Hash`, cbor keys 15/16) folded
    into the signed `Hash()` body so attesters sign them; `BlockVersionStateRoot = 4`;
    `versionSupported(v) = v >= 1 && v <= 4`.
  - **The committed field set.** The **18** `committedSet` fields under the state SMT root; the
    field-tagged (`"fieldname\x00" ‖ rawKey`) NUL-terminated single keyspace; and the per-field
    canonical VALUE encoding — the present-marker for the set-membership fields, and the canonical
    value for the value-carrying maps (int64 8-byte big-endian for the `bonded`/`epochSet` weights,
    uint64 8-byte BE for the heights/domains, raw-32 for `bondRootOwner`, one byte for
    bools/`regVersion`), with the six scalars at reserved keys. The separate RFC-6962
    transparency-log root over `revLog` is `LogRoot()` = `RevocationLogRoot()` (the ordered CT root,
    never an SMT leaf — the #597 two-root shape).
  - **The `v4` hard-fork activation.** Height-gated `H_era3`, one-way lock-in tallied by frozen
    WEIGHT at `>⅔` and gated on `regVersion >= 4`, landing on an epoch-final, reorg-stable boundary
    with `>=` (first-v4-height) semantics. At/above `H_era3` a v4 block with valid roots is required;
    a v2 block there is rejected (`ErrEra3VersionRequired`); below `H_era3` validation is unchanged.
  - **The verifier posture.** "No witness supplied for a key a predicate reads → never accept
    (reject/stall)"; the root check AND the version-boundary rule are enforced on EVERY disk-write
    path, including the node's own-disk Reload, check-before-apply.
  **Governance posture — HARD FORK, confirmed by Andrew:** an un-upgraded node rejects a v4 block at
  decode and STALLS at `H_era3` rather than accept an unvalidated root. That stall is the correct
  safety-first behavior; every operator must upgrade before `H_era3`. **The immutability rule, stated
  plainly: changing the frozen era-3 format requires a NEW ERA (a new `BlockVersion`), not an edit.**
  **What is NOT frozen (open follow-on, so the freeze is not over-claimed):** (a) the WITNESS
  floor-box validation mechanism (C-7 / #600 — witness soundness, the R3 witness size-DoS bound, and
  the R4 missing-witness ≠ verified-exclusion accessor), and (b) the incremental-SMT / `ports.NodeStore`
  optimization. Until the witness path ships, the A-bare `O(depth²)`-boot full-tree validator is the
  certified hold-the-tree bridge (a bounded boot-time cost, not a freeze-blocker).
- **RATIFIED 2026-08-29 — era-4 witnessable state transitions (Option B), the format veto-gate**
  (research re-certification RECERT2
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/era4-witnessable-transitions-RECERT2-2026-08-29.md`,
  **CERTIFIED-WITH-CONDITIONS**; prior passes it supersedes:
  `.../era4-witnessable-transitions-EQUIVALENCE-RESEARCH-2026-08-29.md` and
  `.../era4-witnessable-transitions-RECERT-2026-08-29.md`; PE ruling
  `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-era4-witnessable-transitions-2026-08-29.md`;
  design `docs/thinking/2026-08-29-era4-witnessable-transitions-options.md`). Andrew ratified the
  format veto-gate on 2026-08-29. **Why era-4 exists:** two `apply()` operations scan whole
  committed maps and so cannot be witness-validated by an O(payload) floor box — the TTL-expiry
  sweep (`for id, regH := range c.bondRegHeight`, `chain.go:3005-3013`) and the epoch-rotation
  qualified-set rebuild (`liveQualifiedSet` scans all of `c.bonded`, `chain.go:1198-1206`). era-4
  makes both transitions witnessable so the tree-free floor box (C-7 / #600) can validate them.
  **What is ratified, precisely (the four format items):**
  - **`BlockVersion = 5`, `versionSupported(v) = v >= 1 && v <= 5`, PREDICATE-FIRST.** The version
    ceiling widens in the **same release** as the v5 validity predicate, closing the interim
    accept-a-wrong-root window that the era-3 rollout left open (era-3's `versionSupported <= 4`
    landed a release before the predicate). Today `versionSupported` is `<= BlockVersionStateRoot`
    = 4 (`chain.go:339, 740`); era-4 lifts the ceiling to 5 only when the predicate is present.
  - **Three new committed field-tags added to the state SMT keyspace:** `tagDueBucket` (the TTL
    due-height index), `tagQualified` (the live qualified accelerator), and `tagEpochStart`
    (rotation observable, the O-1 commit). `tagEpochSet` is **retained** (the frozen materialized
    era-3 shape, `statehash.go:40`); era-4 does not edit era-3 blocks.
  - **TWO-keyspace layout for the qualified set (RECERT2 Q1, the corrected E-2).** `epochSet`
    stays its own **frozen** materialized committed keyspace (mid-epoch immutable, sizes the
    governing quorum via `effectiveEpochSet`, `chain.go:1243-1248`); a separate live-maintained
    `qualified` keyspace is committed as a **boundary-computation accelerator**, not a pointer
    target. At the boundary `epochSet := qualified` is a copy; the boundary block is a distinct,
    heavier witness class with an O(boundary-delta) changed-leaf set (not O(payload)). This
    preserves the I1 sizing-set≠membership-set and I3 no-mid-epoch-churn invariants (`qualified`
    is a fourth distinct map in `cloneForDryRun`, `era3validity.go:173-175`).
  - **A new `RegCap` per-block TOTAL BondReg count validity rule, value = 256.** This caps the
    total BondRegs per block — **fresh AND renewal**, counted after `canonicalBondRegs` — as a v5
    block-validity rule every replica checks on receipt, bounding the witness read-set so it fits
    the 2 GB floor box. **Renewals are NOT exempt** (Research REFUTED fresh-only three times): both
    fresh and renewal write `bondRegHeight[id]` at the same apply site (`chain.go:2995-2996`) and
    land in the same TTL due-bucket, and #506 rate-limits renewals per-IDENTITY not per block, so
    O(registry) distinct ids can each renew once in one block → an O(registry) TTL read-set, the
    exact wall era-4 removes. A fresh-only cap leaves that term unbounded. The **instrument is a
    COUNT cap, not a byte cap** (PE ruling): the witness is `count × SProofMax` (a flat per-proof
    envelope), so reg bytes do not enter the witness cost — count does — and a byte cap's implied
    count `floor(L/M)` inflates silently ~13× as `k` (`BondLabelSamples`) drops 64→1. A count cap
    is invariant to `M` and is a single integer compare (I5-clean). **Andrew ratified
    `RegCap = 256`.** The RECERT2 certified the upper bound `RegCap ≤ 16,384` at desk
    (`2 GiB / (EpochBlocks=8 × SProofMax=16 KiB)`, tight) but ruled the *value* MEASUREMENT-REQUIRED,
    not desk-pinnable: it is a security parameter of the same class as `SProofMax`, and the honest
    ceiling reduces to `floor(B / min-valid-reg-byte-size under the deployed verifyBond)`, which is
    not a chain constant. The ratified 256 is safe **under the deployed ~1.5 MB genesis-proof
    scheme**, whose measured honest ceiling is ~1 reg/block at k=64 and 18 at the minimum permitted
    k=1; 256 clears the k=1 floor of 18 by 14× and sits 64× below 16,384, with margin on both sides.
    Its worst-case valid block (~363 MiB = 256 × ~1.485 MB) is bounded by real M0 Sybil cost (256
    distinct sealed plots, per-root dedup), not a free DoS surface.
  - **RE-DERIVATION GATE on all SEVEN determinants, not #299 alone.** `RegCap` is a function of
    seven determinants — block budget `B`, `k` (`BondLabelSamples`), `Samples`, `BlockSize`,
    `BondVDFDelay`, `MinBond`, and the proof scheme. Any one changing re-derives the value, gated at
    the **next BlockVersion mint** (a validity-affecting parameter change requires a mint anyway).
    #299 (succinct proofs) is the SHARPEST single determinant: `M` drops ~1000× and the measured
    honest ceiling rises to ~2,000 regs/block, **above 256**, so `RegCap` MUST be re-measured and
    re-minted before or with #299, or honest registrations are rejected. This is a hard dependency
    recorded ON #299 (also in `docs/design/owned-residuals.md`, the RegCap owed-input).
  **SEPARABLE / scoped out of era-4-minimum (Andrew's call, deferred):** the recovery-boundary
  direction — witnessing the `effectiveEpochSet` recovery re-base at `LivenessRecoveryHeight`
  (`chain.go:1243-1248`), which needs either a committed recovery-height (a new consensus-rule
  gate) or the O-2 posture bound. RECERT2 R2 confirms it is cleanly separable; the Q5 recovery
  branch's `liveQualifiedSet()`-must-agree-with-materialized-`qualified` coupling is discharged as
  a **build-time assertion**, not an open soundness gap. **Build-time obligations owed under the
  "inject the defect" rule (each ablation MUST go red before its increment is trusted):** the
  `qualified` maintenance drift-guard (ablated per site 2989/2995/3008/3019/3020, reddening on the
  2989 hook specifically), the T-3 due-bucket dual-source drift-guard (ablated on a missed renew
  old-bucket delete), the T-3 byte-identical post-apply StateRoot replay vs an era-3 replay over a
  corpus, and the Q5 recovery-branch agreement assertion. The ORDERED build decomposition is a
  separate PACE deliberation (`docs/thinking/2026-08-29-era4-build-decomposition-options.md`); no
  era-4 mechanism is built until `RegCap` is pinned — done here at 256 (per-block TOTAL count),
  with the re-derivation gate on all seven determinants at the next mint (#299 the sharpest).
  - **BUILT 2026-08-29 — the era-4 (v5) witnessable-transitions build spine is COMPLETE and merged
    to main.** The four ordered increments landed the chain-side transitions: 4a (#637 — mint
    `BlockVersion = 5` + reserve the three v5 field tags, inert), 4b (#639 — the maintenance spine:
    `qualified` + due-bucket + `epochStart`, v5-gated), 4c (#640 — the v5 validity predicate +
    `RegCap` + version-widen), 4d (#641 — height-gated activation + mint-flip to v5). This is the
    chain-side witnessable-transitions spine only; it does not by itself ship the trustless
    floor-box (witness) validator, which remains the open C-7 / #600 follow-on.
  - **SEQUENCING RATIFIED 2026-08-30 (owner, Andrew) — era-4/v5 is kept OPEN-ENDED; its freeze is
    deferred to the END of Proof-of-Delivery.** *Why:* there is no live blockchain, so nothing is
    minting v5 against an immutable contract yet, and PoD is expected to add or reshape witnessable
    state. Freezing v5 now would freeze a format PoD may still move. *How to apply:* treat the era-4
    format as still-open through the PoD build; run the v5 freeze as a **second practiced era freeze**
    (the era-3 freeze on 2026-08-29 being the first) at the end of PoD, on the same
    research-certified, owner-ratified path. Do not mark era-4/v5 as frozen or immutable until that
    step is run.
- **RATIFIED 2026-08-27 — "maturity before capture" ships as a safe-parameterization, not a
  theorem** (research certification
  `.../research-outcome/C1-maturity-before-capture-RESEARCH-CERTIFICATION-2026-08-27.md`;
  full path
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/C1-maturity-before-capture-RESEARCH-CERTIFICATION-2026-08-27.md`).
  **Verdict: GATED — CERTIFIED-as-a-safe-parameterization, REFUTED-as-a-theorem**, confirming
  the canon (`docs/design/m0.md` §10, `owned-residuals.md` E3). The mechanism — the one-way
  `everMature` latch, plural threshold anchors, the de-maturation super-quorum — is certified
  sound and shipped; the load-bearing *premise* (that maturity is reached before capture) is
  a parameterized bet against live telemetry, not a bound. **Consequence:** the `everMature`
  latch is certified **one-way** — it bounds the *consequence* of a lost bet (no re-arm, no
  permanent center; a lost race becomes a bounded, socially-recoverable re-centralization of
  *real* stake), it does **not** bound the *reachability* of pre-maturity capture. The gate is
  against publishing the sentence as a theorem, not against the design; VISION §108 is trued
  up to carry the qualifier. Sharpens the #183 red-team seam (see `owned-residuals.md` E3 /
  the red-team brief): the live seam is R1 pre-maturity acquisition; R2 (handoff-instant
  head-count capture) and R3-safety (de-maturation super-quorum) are CLOSED.
- **SUPERSEDES the entry above — RATIFIED 2026-08-27 — "maturity before capture" lifts to a
  CONDITIONAL THEOREM (C-1 = CERTIFIED-CONDITIONAL)** (research certification
  `.../research-outcome/C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27.md`;
  full path
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27.md`).
  **Verdict: CERTIFIED-CONDITIONAL — the lift succeeds, with a named boundary.** The prior
  entry's GATED "safe-parameterization, not a theorem" is replaced. "Maturity precedes
  capture" is now a **theorem (CT-1) under three hypotheses**: an honest-arrival floor **H**
  (address-diverse, operator-distinct bonded provision at a measured floor rate `λ_H > 0`), an
  adversary-budget cap **B** (spendable real-bond capital `W_A` acquired at the C1 no-discount
  price), and a parameter constraint **P** (chiefly P2, `M_req > W_A / (2·w_min)`). The
  derivation's decisive artifact: the honest arrival *rate* `λ_H` **cancels** out of the
  capture-vs-maturity order, leaving a pure budget-vs-threshold inequality — **maturity
  provably precedes ⅔-capture iff `W_A < 2·w_min·M_req`** (and precedes ⅓-stall iff below half
  that). It is proven conservatively, without crediting the declaration-cheap A-axis margin
  `M`. The shipped shed trigger (`min(NakamotoOperators, NakamotoDomains) ≥ MatureValidators`,
  `chain.go:1819`) and the one-way `everMature` latch (`chain.go:2820`) bind the proof.
  **Still NOT unconditional:** the honest-arrival floor `λ_H` and the budget cap `W_A` cannot
  be verified from genesis on chain data alone — the weak-subjectivity wall every deployed PoS
  system lives behind. So C-1 moves GATED → **CERTIFIED-CONDITIONAL**, pending the one owed
  measurement — `λ_H` at launch (address-diverse arrival), now instrumented as a separate lane
  (lambda-h-instrumentation) — which parameterizes the certification, not the consensus code.
  VISION §108 and `docs/design/m0.md` §10 (CT-1) are trued up to the conditional-theorem
  register; the #183 brief re-prices R1 to the inequality and opens R6 (the H⊥B independence
  break — see the red-team brief / `owned-residuals.md` E3).
- **RATIFIED 2026-08-27 — C-5 honest-operator composed economics: GATED** (research
  certification
  `.../research-outcome/C5-honest-operator-economics-composition-RESEARCH-CERTIFICATION-2026-08-27.md`;
  full path
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/C5-honest-operator-economics-composition-RESEARCH-CERTIFICATION-2026-08-27.md`).
  Certifies the COMPOSITION of the honest floor-box operator's economics (relay credit +
  repair credit vs. real operating cost), not any leg in isolation. **Verdict: GATED**,
  decomposed by residual class. **CLOSED (certified intact under the composition):** the
  γ→1/N firewall holds — relay and repair credit are both balance-lane, never standing
  (#182 not strained); conservation holds — no leg mints a network subsidy, so the banned
  money-pump does not exist (relay is sender-funded strict-loss under collusion; repair is
  paid only from an object's own prepaid/skimmed escrow); and **no DEFENSE prices out the
  small operator** — the min-bond floor is walled from the transport/serve economy
  (build-immutable #4). **GATED halves (true-up + measurement owed):**
  **G1 — a FACTUAL correction to VISION.** The repair bounty pays the **new holder** of the
  rebuilt shard, a custody rent for a re-challengeable replica — NOT the reconstructor, who
  by ratified design (`h7-proof-of-repair.md:414`, `TENETS.md:301`) is paid **zero**;
  reconstruction is an unpaid caretaker duty. VISION lines ~54 and ~160 are corrected to what
  the mechanism actually pays (holder-side custody credit + a funded durability horizon for
  cold data). **G3 — repair self-funds HOT objects only** (`S/R ≥ 24`); the cold one-hit
  majority (~50–60% of objects) rides D-S7's finite-but-renewable prepay horizon, not a
  self-sustaining earning; VISION carries the scope. **G2 — the floor-box reconstruction RAM
  spike is MEASURED at production chunk size (2026-09-01): 1024 MiB resident + ~512 MiB
  GC-reclaimable = 1536 MiB allocation-inclusive peak** (prod chunk `DefaultParams{K:10,N:16}` ×
  64 MiB = 16 shards materialized by `ReconstructStripe`; `-benchmem` B/op = 1,610,666,010 B,
  237 allocs/op; method `core/erasure/reconstruct_mem_test.go:79` + `BenchmarkReconstructStripe_ProdChunk`,
  Apple M4, git HEAD `d904d21`). **On the 2 GB pony reference box (build-immutable #8) ONE repair
  fits; TWO concurrent production-chunk repairs OOM the box.** This gates the R2.6 repair-payee
  decision and folds into the owed node-store coexistence test (same 2 GB budget). See
  `owned-residuals.md` G2. No immutable is traded; no certified leg or consensus surface is
  reopened.
- **The node store is a 7th dependency behind `ports.NodeStore`** (PE ruling Q1/Q2; Andrew
  concurred). Take an embedded pure-Go KV store — a hand-rolled engine is the textbook B8
  violation (crash consistency and fsync ordering are settled; "consensus is boring" binds
  hardest on what consensus state is stored in). It sits behind a silt-owned `ports.NodeStore`
  (the five `MapStore` methods), never a third-party interface on the core's surface, so the
  SMT/backend choice — each only one spike old — stays a localized swap and preserves the
  option to drop to a simpler backend once sharding shrinks the per-node key count. The
  backend is chosen by a **build-immutable-#8 floor-box measurement of `bbolt` + one tuned
  LSM in one run** (prior: bbolt's mmap page cache is kernel-evictable where an LSM's
  memtables/caches are server-sized heap — the wrong profile for the box — but #596 proved
  reading loses to measurement on this exact workload, so confirm it).
- **RATIFIED 2026-08-28 — #600 DECIDED: the floor box is a semi-stateless witness-validating
  full validator; hold-tree is a bigger-box opt-in, never the floor default** (Andrew ratified
  the direction). Sources: PE ruling
  `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-600-floor-box-direction-2026-08-28.md`;
  research note
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/600-floor-box-direction-post-coexistence-RESEARCH-NOTE-2026-08-28.md`;
  C-7 certification
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`;
  coexistence evidence `integration/cloudtest/coexist-20260827T212244-citev/`. This resolves
  the decentralization-posture question the earlier entries (2026-08-27) left as Andrew's call.
  The consequences, stated for the record:
  1. **Posture:** witness-validation is the floor box's **primary** validation mechanism. The
     floor box is a **semi-stateless witness-validating full validator** — it verifies every
     transition soundly against the committed root from tier-above-supplied witnesses, but does
     not retain the full registry tree. Holding the tree survives ONLY as a **bigger-box opt-in
     behind `ports.NodeStore`** (an archival / full-registry decentralization posture), never as
     the 2 GB-floor default. **Same security, narrower self-sufficiency** — a witness floor box
     cannot be fooled (C-7 soundness), but cannot make progress without a witness source.
  2. **HARD REQUIREMENT (decentralization tenet `TENETS.md:557`):** witness-serving MUST stay
     **open and multi-provider — un-permissioned, servable by any archival / pruning node**, and
     the floor box MUST be able to source from any of them. Trustless verification does NOT
     rescue a permissioned availability choke; a permissioned witness set would be the banned
     **load-bearing centralized** dependency (`:557`: convenience may centralize, load-bearing
     never). This is a hard constraint on the era-3 witness-delivery design, not a later nicety.
  3. **C-7 residual promotion:** the ≥1-honest-provider **liveness** assumption goes from
     optional → **load-bearing**. Before this call a floor box had a self-sufficient fallback
     (hold the tree, depend on no one); that fallback is gone. **Safety is unaffected** — a
     witness-less floor box **STALLS, never accepts** (C-7 Q2, unconditional on provider
     honesty). Record as its own named seam in `docs/design/owned-residuals.md`,
     **cross-referenced to the #183 cold-start seam as a sibling liveness-on-the-tier-above
     dependency, not folded into it** (#183 is bootstrap/maturity liveness; this is the new
     post-maturity seam "can a tree-less floor box keep validating if the witness tier
     degrades").
  4. **Backend NOT reopened — bbolt stays.** pebble's unevictable heap TIES bbolt's at 1M
     (305 vs 304 MB, #601); the thrash spiral is inherent to holding-the-tree-build under
     pressure on a page-cache store, not a bbolt property. The backend-lock ruling
     (`RULING-keystone-node-store-backend-lock-2026-08-27.md`) is unchanged.
  5. **HONEST EVIDENCE BASIS — do not over-claim.** The billable coexistence run did **NOT**
     produce a shed-vs-OOM measurement. It was killed by `-timeout 60m` DURING the 1M
     build-from-empty under a ~1 GB balloon; **zero rssMB rows** were captured (the quantitative
     trace is on the box's inaccessible nohup log; serial carried zero test output). What the run
     DID show: **severe memory pressure** (free -m available fell 48 → 8 MB, sshd could not fork,
     ens4 network-dead at 22:05, zero OOM events in serial) and that the 1M build did not finish
     in 2× the unpressured time. The decision therefore rests on **C-7 certified-sound + no owed
     measurement to ship the witness path + hold-tree-on-floor unproven-to-fit + the
     severe-pressure signal** — NOT on a conclusive coexistence OOM. The run refutes "the floor
     box builds and holds a 1M-key tree beside a real daemon"; it is NOT cited as "bbolt is
     unusable."
- **Immutables preserved:** consensus engine untouched; M0 firewall reinforced; the
  hobbyist box (#8) is the point of the whole direction; content-blind core untouched (the
  root commits locators and status, never content meaning).
- **RATIFIED 2026-08-30 — the trustless floor-box (lane-1, increment 3): the v5 witness
  read-set is bounded, RegCap=256 is measured safe, and #535's recovery boundary is a
  local-only policy flag.** Andrew ratified all three. This is a decision record for the
  lane-1 witness-validating floor box (the #600 posture above), not a mechanism restatement;
  the mechanisms live in the cited certifications and the v5 recompute. The three items:
  1. **Lane-1 witness read-set identity RATIFIED (AMENDED / complete form).** The sound
     floor-box WITNESS read-set for a v5 block is the **23-keyspace** committed read-set
     enumerated in the amended certification, with its per-leaf read-membership table (18
     era-3 committed leaves + 5 v5-only leaves; each row names the recompute read site). It
     is **O(payload)** for ordinary and TTL blocks; the boundary read-set is **O(RegCap)**
     (see boundary bound below). **CORRECTION — the prior identity was INCOMPLETE and is
     SUPERSEDED.** The earlier entry named validity reads ∪ `apply()` branch reads ∪ era-4
     accelerator reads, but OMITTED the attestation-loop committed leaf `validatorsSeen`
     (`apply()`'s per-attester `validatorsSeen[id]` write, gated on committed qualification
     reads), the maturity-latch scalar `everMature`, and eight committed scalar leaves
     (`bondDomain`, `matureEpoch`, `epochStart`, `era4LockedIn`, `era4Height`,
     `gateLockedIn`, `gateHeight`, `era3LockedIn`, `era3Height`). A floor box witnessing only
     the prior read-set could be made to **wrong-accept** a forged block on any omitted leaf.
     The complete identity is the amended one. **Load-bearing:** the FULL-NODE recompute stays
     **O(registry)**; only the WITNESS read-set is bounded. The read-set producer MUST be
     **payload-driven (O(payload))**, and its completeness guard MUST be **EXECUTION-DERIVED**
     — the recorded leaf-touch of the real v5 witnessable recompute (or a pre/post
     `stateRootLeavesV5` leaf diff), ablated red on a dropped read — **never a hand-written
     mirror**. A hand-written guard mirrored the hand-written producer, both inherited the
     blind spot, and the guard stayed green over the accept-a-forgery gap. Cite the amended
     certification by full path:
     `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/era4-witness-floor-box-readset-v5-AMENDED-RESEARCH-CERTIFICATION-2026-08-30.md`
     (supersedes the same-day
     `.../era4-witness-floor-box-readset-v5-RESEARCH-CERTIFICATION-2026-08-30.md` on identity
     and boundary label; that cert's REFUTE of "read-set = full O(payload) apply() touch-set"
     still stands).
  2. **R1 — RegCap=256 MEASURED SAFE (no value change).** The honest per-block ceiling is
     **1 BondReg** at the deployed `k=64` / `MinBond=1 MiB` (a full space-time proof is
     ~1.46 MiB against the 2 MiB block budget), so RegCap=256 carries **255× headroom** and
     rejects no honest block. The bounded `dueBucket` witness at 256 is **32 MiB** at an epoch
     boundary and fits the 2 GiB floor box. Re-derivation is required **only if one of the
     seven determinants changes** (tracked as **E6 on #299**). RegCap stays **256** — no value
     change from the era-4 pin above.
     **Boundary bound corrected (amended cert, Claim 2):** the epoch-boundary WITNESS
     read-set is **O(RegCap)**, NOT O(boundary-delta). O(boundary-delta) is the WRITE-set (the
     changed leaves); the three `rotateEpoch` activation tallies read `regVersion` and sum
     weight over EVERY frozen-set member, and the super-quorum predicate `3*ready > 2*total`
     cannot be computed from a delta. **Box-fits at RegCap=256 is UNCHANGED and NO security
     parameter moves** — only the boundary label / DoS-bound wording was wrong; correct every
     downstream boundary witness-cost statement to `O(RegCap) · S_proof_max`.
  3. **R2 — #535 recovery-boundary disposition DECIDED (one local-only policy flag).** The
     recovery directive is sourced **ONLY from the box's own `-ws-checkpoint`-class config,
     NEVER from the proposer or the block**. Directive present for height h ⇒ validate
     trustlessly against the recomputed witnessable set. Directive absent at an ambiguous
     boundary ⇒ emit a loud `indeterminate-trustlessly`, never trust the proposer. **DEFAULT =
     cold-auditor** (stall-loud; favors full trustlessness); **live-follower** (proceed on the
     full node's existing weak-subjectivity residual) is an **opt-in flip of the same flag**.
     **GATE:** certifiable CLOSURE of this residual is **gated on the #603 `bonded` /
     `epochSet` keystone probes** — do NOT mark this residual closed until those probes are
     green. Cite:
     `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/535-recovery-boundary-disposition-RECONCILIATION-2026-08-30.md`
     and
     `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RECONCILIATION-floorbox-livenessrecovery-boundary-2026-08-30.md`.
- **RATIFIED 2026-08-31 — the v5 floor-box recompute DIRECTION and the R-boundary POSTURE
  (HEAVY / fully-trustless).** Two owner ratifications on the lane-1 floor box: how the recompute
  closes completeness, and how far it validates.
  1. **Floor-box recompute DIRECTION RATIFIED (Andrew, 2026-08-30).** The v5 floor-box recompute
     closes completeness by **MTH-reconstruction** — the `dueBucket` pattern: witness the id-list,
     recompute the Merkle Tree Hash, require it equals the committed digest. **NO new cryptographic
     primitive; NO `dueBucket` format change.** The direction reuses the existing committed-digest /
     recompute machinery. Cite:
     `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/era4-v5-floorbox-bounded-recompute-CRUX-RESEARCH-CERTIFICATION-2026-08-30.md`.
  2. **R-boundary POSTURE RATIFIED HEAVY (Andrew, 2026-08-31).** The floor box reproduces **EVERY
     validity predicate** (Option B, fully-trustless). It does **NOT** merely re-derive the state
     root and trust finality for the accept decision. **Rationale (the owner's "pony" framing):**
     the edge node (1 CPU / 2 GB) contributes as an economical **INDEPENDENT VALIDATOR** — it helps
     build quorum and validates recent/upcoming blocks from bounded witnesses, not as a trusting
     follower and not as an archival workhorse holding the whole state. The committed digest roots
     are what make full validation fit the pony budget (bounded witness, measured to fit 2 GB). This
     is what era-4 was built for.
     - **Consequence — the v5 committed format needs an MTH digest-root leaf for every WHOLE-SET
       committed read the heavy recompute performs — at least** `qualifiedRoot`, `epochSetRoot`,
       `validatorsSeenRoot`, `bondedRoot` (**≥4**). The COMPLETE, exhaustive set is being established
       by a **mechanical / execution-derived enumeration** across `apply ∪ ValidateCommit` (the
       root-cause fix for repeated hand-enumeration misses), then Research-certified.
     - **Blind spot to close:** the merged Part A (#656) producer + guard **do not witness the
       quorum-stack whole-map reads** — to be closed. The **#657 `WitnessValidateV5` seam contract
       WIDENS** from "accept iff root matches" to "**reproduce every validity predicate**."
     - **The v5 format freeze stays DEFERRED** until the complete digest-root set is certified AND
       the format addition owner-ratified (consistent with the 2026-08-30 open-ended-freeze
       sequencing above).
     - Cite the R-boundary reconciliations:
       `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RECONCILIATION-lane1-Rboundary-root-count-2026-08-31.md`
       and
       `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/era4-v5-Rboundary-Rscope-RECONCILIATION-2026-08-31.md`.
- **RATIFIED 2026-08-31 — the O(payload) HYBRID state-root recompute for the trustless floor
  box.** Andrew ratified the design that lets the tree-free **pony** (1 CPU / 2 GB) fully
  validate a block's committed `StateRoot` in **O(payload)**, not O(whole-state) — preserving
  the heavy / fully-trustless posture (the HEAVY R-boundary ratified above) **without** a
  hold-tree box, a light posture, or a new cryptographic primitive. This grounds the node-tier
  ratio vision (ponies : horses : archival ≈ **10000 : 100 : 1** — ponies DOMINATE validation,
  so full validation MUST fit 2 GB).
  1. **The design (HYBRID, class-partitioned).** The box:
     1. **DERIVES the write-set** from the block's payload + the era-4 accelerators
        (`dueBucket` for TTL; the digest roots for whole-set commitments) — running the
        **generator**, not the prover;
     2. **collects each changed leaf's pre-state proof** against `prevStateRoot`;
     3. **FOLDS only those changed paths** to COMPUTE the post-root;
     4. requires it **`== b.StateRoot`**.
     Deriving the write-set closes **completeness**; the fold catches any **discrepancy**.
     Neither verify-only nor fold-only closes; **together they do**. The **write-set derivation
     is the load-bearing object** (same lineage as the read-set completeness / R-boundary work
     above).
  2. **Class-partitioned.** Payload / `dueBucket`-driven classes (**E / R / T**) are
     certified-in-direction **now**; whole-map classes (**B / P / M**) inherit the still-open
     R-boundary write-set completeness.
  3. **GATED-on-build residuals.**
     - **R-fold** — the multi-leaf SMT fold pinned **byte-exact** and ablated per structural
       case (the **largest new burden**).
     - **scope-gate** re-anchored on `dueBucket[h]`.
     - **R-writeset** — B / P / M ⊂ R-boundary.
     - **R-scope** — the box stays **never-Accept** until R-fold is pinned.
     - **R3** — an execution-derived drift guard vs the real `apply()`.
     - Standing **R1** (RegCap) and **R2** (#535) carry forward.
     Cite:
     `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/floorbox-recompute-P1a-Opayload-multileaf-RESEARCH-CERTIFICATION-2026-08-31.md`
     and
     `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-floorbox-recompute-P1a-Opayload-multileaf-2026-08-31.md`.
  4. **CORRECTED 2026-09-08 (`D-RECOMPUTE-FREEZE`, owner direction via the PE).** The sentence
     "O(payload), not O(whole-state)" does not describe the code as built. The digest-class
     recompute files state the honest cost themselves — *"COST — HONEST … NOT O(payload) …
     ≈ O(registry) per touched digest"* (`core/chain/floorbox_recompute_stateroot_slash_v5.go:46`,
     `…_ttl_v5.go:47`, `…_bondreg_v5.go:51`) — and the measurement puts the provider-side SMT
     build at ~980 MB live heap at N = 1M
     (`docs/thinking/2026-08-31-floorbox-wholeset-witness-size-measurement.md`). Item 1 of the
     2026-09-07 true-up (five digests → three) is still O(registry). The ratification above stands
     as history; the track it ratified is FROZEN (below).

---

## D-POD-KNOBS — the three Phase-4 economy/state knobs, decided

- **Status:** ✅ DECIDED — 2026-08-26 (owner ratification: "PE answers are mine as well").
  All three were held open by the two 2026-08-26 certifications as *owner scope*; the PE
  attached an engineering recommendation to each, the owner adopted them.
- **Basis:** the certifications
  (`silt-agent-memory/researcher/reviews/research-outcome/PoD-neutral-lane-B3-close-RESEARCH-CERTIFICATION-2026-08-26.md`,
  `…/D-TIERING-state-root-keystone-RESEARCH-CERTIFICATION-2026-08-26.md`), the consult
  (`silt-agent-memory/principal-engineer/reviews/PoD-keystone-owner-knobs-CONSULT-2026-08-26.md`), and
  the ruling
  (`silt-agent-memory/principal-engineer/reviews/RULING-PoD-keystone-owner-knobs-2026-08-26.md`), whose
  load-bearing premises were verified against code before the recommendations were made.

**1. Delivery-credit skim routes to the object's durability ESCROW (not burn).**
The skim (`SkimNum/SkimDen` = 1/8 of the withdrawal fee) funds the delivered object's
repair reserve, exactly as the serve skim does. The decisive reason is a **cross-tier
funding loop**: hot content served on the edge generates skim that funds *that content's*
durability on the persistent tier — edge delivery financing persistent retention, which is
what D-TIERING's tier split needs. Conservation carries soundness independently
(`credit ≤ fee`), so the skim is a deterrent knob, not a soundness requirement — **do not
raise it for anti-wash reasons** (that would tax honest delivery; build-immutable #4).
*Safety rests on the recovery floor:* the supersede reversal is floored at the remaining
reserve, so a bounty already paid for real repair work is never clawed back — escrow
therefore cannot mint recoverable balance. That floor is regression-locked
(`core/credit` `TestPaidBountyIsNotRecoverableBySupersede`); **if it ever regressed, burn
would become the correct routing.** Burn's only advantage is audit optics ("zero recovery,
ever" in one word) and remains the fallback if an external review ever needs that answer.

**2. Relay compensation needs NO TTP — the relay leg is self-enforcing.**
**AMENDED 2026-08-27 by research certification** (`.../research-outcome/PoD-relay-compensation-followon-RESEARCH-CERTIFICATION-2026-08-27.md`),
which answered the scope condition this knob was conditional on. The original decision was
"dispute-only quorum-TTP, contingent on the relay dispute being signature-verifiable." The
certification found something stronger and simpler: **there is no adjudicable relay dispute
at all**, so the TTP is not needed rather than merely cheap.

The relay leg is **self-enforcing at both ends**. A self-authorizing payment token (PayWord
hash chain) lets the relay redeem only increments the fetcher actually authorized — it
cannot forge a preimage, so the fetcher is fully protected with no dispute. In the other
direction, for the relay to be *owed* increment N+1 it must prove it *forwarded* N+1, which
**no transit proof can establish** (PoD cert Q3: Tor's proof-of-bandwidth line failed here;
endpoint attestation dies under endpoint collusion). Neither side can prove the other
cheated, so a quorum has nothing to adjudicate.

This **refines the PE's Sharpening A with evidence**: the concern that "no TTP" bakes a
one-increment stiff into the relay/backbone tier is valid, but a quorum-TTP **cannot remedy
it** — it can verify the payment chain (which already self-verifies) and cannot verify
forwarding (which nothing can). The stiff is **irreducible** (Pagnia–Gärtner, plus the
unprovability), and its only remedy is to **bound the increment small**, not to adjudicate.

**Certified consequences:** PayWord hash chains (cheapest verify, scales best as increments
shrink); byte-sized ~1–64 KiB increments **pinned by a floor-box measurement**, never a
round figure (#8); relay credit is the **operator balance** at epoch net-settlement — no new
keystone field; **one** Invariant-A firewall regime covering delivery + relay credit (PE
coupling 2); conservative under collusion, and the feared "fabricated dispute mints credit"
vector **does not exist** because no dispute exists.

**Privacy — two constraints that may not be traded** (M0 access-privacy): (i) bind the
PayWord chain root to a **blind credit under a fresh ephemeral identity**, never a durable
one (reuses D3 slice 1, already built); (ii) **fresh ephemeral identity + chain per
session** — reuse would upgrade the relay from a per-session to a **longitudinal** observer,
a real Don't-#3 regression. The consult's worst-case vector (a public dispute naming a
fetcher key, adversarially triggerable) is **dissolved** by the same finding that closes the
gate: no dispute, no public linkage.

**Coupling 3 satisfied decisively:** relay compensation touches no dispute crypto, so it
cannot reactivate the verifiable-escrow (Camenisch–Shoup) unknown, which stays wholly in
strong-form/delivery PoD.

**3. A bond root's ownership record follows current possession (TTL-lapse), not lifetime.**
When a bond lapses, its owner/proven records may be dropped from committed state; a new
owner may re-bond that root with a fresh space-time proof. C2-sound because the F1 dedup
invariant is "one plot ≤ one **active** standing," never one lifetime owner — a lapsed
root backs zero standings. Verified safe: `rootOwner` feeds the F1 dedup and nothing else,
both slash paths dock by identity regardless of root ownership, and the anti-griefing
guarantee comes from per-identity plot sealing (an outsider cannot answer a plot it did not
seal), not from retaining the map. Both properties are regression-locked
(`TestRootOwnerFeedsOnlyTheDedup`; `core/bond` `TestRedteamG2_PlotBoundToClaimedIdentity`).
**This is required, not merely available:** lifetime-owner would put a forever-growing term
in every committed snapshot, defeating the bounded-state property the keystone exists to
deliver. Lifetime provenance is not lost — it survives immutably in the **archival tier's**
chain history; only the *live committed state* forgets, which is exactly D-TIERING's tier
decomposition.

- **Construction (what still has to be built):** (1) is shipped (`core/credit/delivery.go`).
  (2) lands with relay compensation, after its follow-on consult certifies the scope
  condition. (3) freezes into the keystone's committed field set and is implemented on that
  track — the live in-memory ledger's map is unchanged for now, so no consensus-adjacent
  behavior moves ahead of the keystone.
- **Standing rule this sets:** whenever a Phase-4/keystone knob is otherwise balanced,
  favor the option that keeps the **live committed state bounded** — that is the keystone's
  reason to exist.

---

## D-POD-RELAY-COEXIST — paid relay is additive to free relay, under shared caps

- **Status:** ✅ RATIFIED — 2026-08-30 (owner ratification of the certified no-regression
  option). It answers the ONE policy question the §7.3 transport increment (Batch 3)
  surfaces for the first time: what a relay with `--accept-relay-payments` on does when a
  FREE swarm-relay connect arrives at the same listener.
- **Basis:** the research certification
  (`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/PoD-7.3-free-vs-paid-relay-coexistence-RESEARCH-CERTIFICATION-2026-08-30.md`),
  which certified the composed policy against the three gates it touches (economic mechanism
  / D-DEMAND-adjacent, M0 access-privacy / Don't-#3, and Don't-#1 not-forced-to-serve), and
  the builder deliberation
  (`docs/thinking/2026-08-30-pod-7.3-batch3-daemon-binding-design.md` §1).

**The decision: Option B — paid relay is a PARALLEL opt-in path; free relay is UNCHANGED,
and the two share the SAME transport caps** (`MaxSessions`/`PerPeerSessions`/
`MaxSessionBytes`). A connect with a valid, node-owned paid handle runs the paid splice; every
other connect runs the free splice exactly as today. Payment is purely additive — it gives an
operator a reason to carry the paid sessions on top of the free floor, never a reason to
withdraw the free floor.

**Why this and not the alternatives** (the certification's verdicts, not re-derived here):
- **Option A (paid REPLACES free): REFUTED** — an access regression. Coupling "accept payment"
  to "withdraw free relay" strips NAT-fallback reachability from every non-paying peer, trading
  an M0-adjacent connectivity value for a bandwidth-pricing feature. The correct direction is
  additive-never-replacement.
- **Option C (free rate-limited, paid uncapped): GATED, not for v1** — conservation-safe but it
  introduces a NEW economic/security parameter (the free-vs-paid cap relationship) that no proof
  covers, and it silently degrades the free floor when payments turn on. This is exactly the
  "a durability knob was twice also a security parameter" trap (`docs/build-process.md`). It stays
  closed unless a sharper pay-incentive is ever wanted, and only through the full research gate
  with a measured free-reachability floor plus a Don't-#1 human ruling.

**What may NOT be traded** (certified constraints that ride this policy):
- No free-vs-paid cap parameter, no reserved paid headroom, no differential free rate-limit —
  that is the GATED Option-C territory.
- A paid connect whose handle does NOT resolve to a live, node-owned session must be REFUSED,
  never downgraded to free (a free downgrade hands a non-payer an unfunded forward). This is a
  correctness condition on the paid path, regression-locked (`adapters/relay`).
- The paid handle is per-session and non-linking; the settlement log must never carry it in a
  cross-session-correlating way (the M0 residual audit rides Batch 3).
- **Held-in-tension, pre-existing (not introduced here):** free and paid share `MaxSessions`, so
  a free-relay flood can still exhaust the shared fan-out cap. This is UNCHANGED from today (free
  relay abuse is already flagged for launch); Option B does not worsen it. Insulating paid
  sessions from a free flood is a SEPARATE reservation decision that would re-raise the Option-C
  parameter gate — it is not folded into B.

## D-V5-WHOLESET-ROOTS — five whole-set digest-root leaves close the heavy-posture set-completeness gap

- **Status:** ✅ RATIFIED — 2026-08-31 (owner ratification). Andrew ratified adding **five
  v5-only committed MTH digest-root leaves** to the OPEN era-4/v5 committed format:
  `bondedRoot`, `epochSetRoot`, `qualifiedRoot`, `slashedRoot`, `validatorsSeenRoot`. Each is a
  **membership-only digest** (weight/value rides the existing per-member leaves), **always-emit**,
  mirroring the certified `dueBucket` MTH.
- **What they buy:** they let the heavy-posture floor box prove **SET-completeness** for the five
  whole-set committed reads — `bonded`/`slashed`/`epochSet`/`validatorsSeen` in the quorum stack;
  `qualified` in the apply-channel boundary freeze — by **reconstruct-and-compare**, closing the
  whole-set wrong-accept gap. Without them a floor box validating by proof can be fed an
  incomplete set (omitted members) and cannot detect it.
- **Basis:** the research certification
  (`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/v5-wholeset-digest-root-addition-RESEARCH-CERTIFICATION-2026-08-31.md`),
  the PE cert cross-check
  (`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-v5-wholeset-digest-root-cert-crosscheck-2026-08-31.md`),
  and the read-set enumeration
  (`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-readset-v5-quorum-wholeset-enumeration-2026-08-31.md`).

**Additive, immutable preserved.** The addition **appends to `stateRootLeavesV5` only**; the 18
era-3 leaves are untouched, so a **v4 root stays byte-identical** (era-3 is frozen, #632). This is
an addition to the OPEN era-4/v5 format, permitted by the deferred-freeze decision. **It is NOT an
immutable trade.**

**Seven binding build / model-check conditions** (none desk-liftable — each must be met in the
build + model-check tier before the v5 format freezes):
- **C-1 (load-bearing):** the recompute composes `digest ∪ per-member value/inclusion proofs` — the
  digest binds MEMBERSHIP; 4 of 5 folds are weight/predicate sums, so per-member weights MUST be
  verified or the tally is forgeable.
- **C-2:** the #535 cold-auditor directive-trust is PRESERVED — the roots witness set-completeness,
  not directive authenticity (`LivenessRecoveryHeight` stays cfg-carried / uncommitted).
- **C-3:** the corpus-poise caveat travels — the 5-set is proven by the closed-23 schema + the PE
  call-tree trace, not by the perturbation oracle.
- **C-4:** always-emit is mandatory — the keyspaces can be empty; empty = `translog.MTH(nil)`, a
  fixed constant; no absent-vs-empty shortcut.
- **C-5:** the ablation suite — per-keyspace + #535-boundary + empty + forged-weight +
  slashed-omission, each red-before-green.
- **C-6:** the recompute reads genesis-pinned config (`MinBond`, `Anchors`, `OperatorMargin`) from
  its OWN genesis config, NEVER the witness.
- **C-7:** the new tags are `\x00`-terminated / prefix-safe (`Key = tag||rawKey`) and bound into the
  coverage guard.

**The v5 format freeze stays DEFERRED** until the addition is built and model-checked.

---

## What is NOT on this ledger

The following are **build items or tuning knobs**, not owner-level decisions, and live in
their own tracks (`design/m0.md`, ROADMAP, the "evolving" tenet tier):
- bind the DHT `Domain` signal to a transport-observed /24 (H5 residual);
- the D3 issuance-mixing transport (build item under D-PRIV);
- the real-wire adversarial-consensus (D2) test sub-suite;
- **compute the C2 concentration metric's weight numerator from the committed on-chain bond
  ledger, not gossip** (`B1-c2-no-quiet-capture.md`'s sharpest engineering find — kills the
  gossip-skew half of the skew+split attack outright; the objective fork-choice path already
  recomputes weight from on-chain `BondRegs`, so this is a metric-wiring task);
- the private-lookup build-track (`C-privacy-buildtrack.md`): server-held-DB PIR (Peer2PIR
  model) for routing/provider records + epoch-bounded staleness + a rotating sortition
  committee (VRF + beacon) for the ≥2-non-colluding-parties atom — the committee counts as
  "no permanent center" *exactly when the shed metric clears* (same measurement as C2);
- economic parameters held in tension — `C_honest` weights, concentration threshold *k* (and
  its margin *M* = M_cluster·M_est·M_sample), demand-attestation ratio, audit/decay windows,
  fee pricing.

**Research frontier (genuinely open — needs a new result, not a decision).** Tracked in
[`design/m0.md`](design/m0.md) §10:
- **the shared-content sealing boundary** — the one surviving economy of scale; plain PoR
  over shared erasure-coded shards leaks γ→1/N, closed only by identity-keyed PoRep sealing
  of arbitrary useful shared data (not yet publicly-verifiable + timing-free +
  trusted-setup-free). *silt is not exposed today* — standing comes from a dedicated
  identity-keyed bond plot, not the shared shards — but fusing served content into standing
  without leaking γ→1/N is the open problem;
- **proof-of-correct-repair for MSR/regenerating codes** (A1 G1);
- **Byzantine size-estimation under *adversarial* NodeID placement** — the CPR `O(n^{1−δ})`
  fault tolerance is proven for *random* placement; a stake-splitter's chosen NodeIDs
  degrade it by an amount the literature does not quantify (B1's flagged gap).

---

## D-A4-CONSERVATION — the A4 provisional-lane money-pump is closed; B3 conservation re-certified

- **Status:** ✅ RATIFIED — 2026-09-01 (owner ratification; PR #686 merged). The A4
  money-pump — an evicted-then-redeemed provisional delivery lane that kept the eager
  self-mint AND took the conserved leg (the supersede reversal was skipped when the
  provisional record was gone) — is closed on `main`. It was the only break exploitable
  today (behind `-accept-delivery-receipts`; `-economy` default false), bond-gated so it
  bought no consensus standing but broke the certified B3 conservation close and could fund
  spam/publish at scale.
- **What shipped:** **(b)-minimal** — a shared `reverseProvisional` claws back the eager
  self-mint at eviction (one delivery, one payment), with the server identity now stored on
  the provisional so eviction reverses the exact credited account. Alongside it,
  `provOrder`/`provIndex` FIFO-order integrity landed (a blind red-team pass and a compaction
  fuzz each found and closed a desync).
- **Cert:** **R0.4 CERTIFIED** — conservation
  (`Σ balances + Σ escrow == grant + legitimate transfers`) holds across all three terminal
  lane states (never-redeemed, in-window-redeemed, evicted-then-redeemed); no new money pump;
  neither firewall (γ→1/N, standing) re-opened. Basis:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/A4-provisional-eviction-conservation-RESEARCH-CERTIFICATION-2026-09-01.md`.
  This cert supersedes, on the eviction axis, the prior B3 close cert
  (`PoD-neutral-lane-B3-close-RESEARCH-CERTIFICATION-2026-08-26.md`), which did not model
  bounded-map eviction.
- **Direction on (b):** (b)-minimal-now / (b)-full-later is CERTIFIED sound. (b)-minimal is a
  complete, conservation-correct close on its own; (b)-full (receipt-expiry) is an optional
  deeper simplification, not a soundness dependency.
- **R0.4b — receipt-expiry (full (b)-prunable) PARKED, evidence-gated.** Re-open when the
  unwitnessed-bilateral under-pay tail bites (>8192 un-redeemed lanes on one node), and only
  after two owner calls: (1) the **privacy-safe shape** — epoch-granular / serial-indexed,
  NEVER a wall-clock `NotAfter` (which adds a timing quasi-identifier on the D3/H8 channel);
  (2) a **TTL-bounds measurement** (the honest-pony redemption-latency tail at the
  >8192-lane regime is unmeasured; no value may be pinned at desk). It is a **separate**
  certification unit (new receipt-lifetime rule + unlinkability interaction + new floor-box
  state bound + credit-layer eviction wiring), not folded into R0.4. Scoping cert:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R0.4-receipt-expiry-scoping-RESEARCH-CERTIFICATION-2026-09-01.md`.
- **Open residual — RT-DELIV-3:** the delivery-credit `provKey` omits the server, a latent
  conservation break reachable only in the shared-ledger *sim* (not per-node prod). The
  next-session decision: fix now (add the server to `provKey`, which changes the conserved-lane
  key shape → **re-opens this R0.4 cert**) vs track as a sim-only residual.

## D-FLOORBOX-WITNESS-SOUNDNESS — Resolve-anchoring is the correct direction; fold-equality is NOT a universal backstop

- **Status:** ✅ RATIFIED — 2026-09-01 (owner ratification). Two linked research verdicts on
  the floor-box witness-soundness spine (Boulder 1), certified together:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/floorbox-R1.3-refutation-R1.4-witness-soundness-RESEARCH-CERTIFICATION-2026-09-01.md`.
- **R1.3 — REFUTED (certs withdrawn).** The 2026-08-31 class-A / class-P / class-B directional
  certs are **WITHDRAWN**. They rested on the premise that fold-equality
  (`postRoot == StateRoot`) is a universal backstop that catches a forged witness value. That
  premise is FALSE: classes P/A/B read witness VALUES and PREDICATES as fold NewValues or
  branch decisions without resolving them against `prevStateRoot`, and an attacker who forges a
  block controls the committed root too, so `postRoot == StateRoot` holds by construction. The
  withdrawn certs are
  `floorbox-recompute-classA-classP-wholeset-RESEARCH-CERTIFICATION-2026-08-31.md` and
  `floorbox-Rboundary-writeset-digest-reconstruction-RESEARCH-CERTIFICATION-2026-08-31.md`.
  **Correct direction:** every untrusted witness read must be `Resolve`d (present or absent)
  against `prevStateRoot` — the one root the attacker does NOT control — before it is trusted,
  and **NoWitness must stall** (never fall through to a false/absent read).
- **R1.4 — CERTIFIED (recompute soundness, as a NEVER-ACCEPT increment).** At the fixed
  artifact every untrusted predicate/value read in classes P/A/B and the class-A screen is
  Resolve-anchored against `prevStateRoot`; the whole-set digest pre-sets are fold-anchored; the
  23-field carrier table is complete by reflection with teeth; and each of the 10 per-field
  anchors plus the cross-class class-M poisoning path has a driven adversarial-committed-root
  gate that wrong-accepts if its anchor is dropped. The box still **never-Accepts**. This
  merged as a standalone never-Accept increment.
- **Scope boundary — this is NOT the accept-flip.** R1.4 certifies recompute soundness only; it
  is the flip's PRECONDITION, not its grant. The accept-flip (R1.8, a consensus-rule change, I1)
  additionally requires: **R-membership** (OPEN — a set-size bound on qualified / `validatorsSeen`);
  the **EXTERNAL B8 red-team pass** (owner-ratified HARD precondition); the **#535
  recovery-boundary decision**; and the **legacy-mode invariant**.

## D-F2-EVIDENCE-RECOMPUTE — equivocation evidence is recomputed from the body, never read from `Pruned`; `Slashes` gets a byte ceiling

- **Status:** ✅ RATIFIED — 2026-09-03 (owner: *"R0.6 ratified"*). Built the same day on
  `builder/r0.6-i5-evidence-recompute`. Certification:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/I5-cross-height-pruned-slash-forgery-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md`.
  Deliberation: `docs/thinking/2026-09-03-r0.6-i5-evidence-recompute-design.md`.
- **The break (I5, LIVE on main, every era).** `VerifyEquivocation` read the height from a
  struct field but the signed message from `Hash()`, which short-circuits to the
  accuser-supplied `Pruned` for the two blocks inside `Slashes[i]`. Two GENUINE signatures by
  an honest validator at two DIFFERENT heights, re-labelled with one fictitious height,
  verified as a double-sign; through `Append` the honest validator was slashed, evicted and
  disqualified forever. A Byzantine PEER sufficed — an honest node queued the forgery itself.
- **The rule (F2-EVIDENCE-RECOMPUTE, a narrowing consensus-rule change, NO era gate).** An
  equivocation proof's two block hashes are always recomputed from the body (`bodyHash`),
  never read from `Block.Pruned` and never from the hash memo; a pruned evidence block is
  refused outright with `ErrPrunedEvidence`. One hash function serves both candidate
  selection (`FindEquivocations`) and verification (`CheckEquivocation`), so proposer and
  validator close in one edit. Strictly narrowing: it can never manufacture a slash. The
  practical fork set is empty (G-1: every persisted chain fixture scanned, zero committed
  slashes with pruned evidence); no grandfather height.
- **The accepted cost — R-LATE-REVEAL, held in tension.** A double-sign whose evidence
  blocks have BOTH been payload-pruned is unslashable. Bounded to below the prune floor
  (honest detection pairs only heights at or above the node's finalized head, which is
  above `pruneFloor`), where `ErrPreFinalityReorg` already forbids adoption: safety is
  unaffected, only the penalty is lost. Closed only by the long-run form (d-3): a two-level
  block hash (`AnswerDigest` in place of `Answer`) at the next `BlockVersion` mint.
- **The pair — `SlashesBytesCap`, an immutable-#8 RESOURCE CEILING, not a security
  parameter.** Full bodies are now the only admissible evidence and `Prune()` never recurses
  into `Slashes`, so every admitted proof pins two full bodies (`BondReg.Answer` included)
  permanently on every node. The ceiling is on canonically-encoded BYTES (a count bounds
  nothing: one proof spans ~1 KB to hundreds of MB), enforced first on every write path in
  every era; at-cap accepts; the proposer packs `pendingSlashes` under it and carries the
  rest. Raising it never admits a forged slash; lowering it never convicts an honest
  validator — that is the whole argument that it is not a security parameter. **The PE
  review adds the other face (F-2):** the value is also the size above which a
  double-signer's evidence cannot be committed, so an equivocator who makes both its blocks
  over-cap keeps its on-chain seat (the local ledger still penalises it; the class pre-existed
  at the 132 MiB frame). Whether I5's COMPLETENESS half makes the value research-gated is
  routed to the Researcher — **and the Researcher's delta certification REFUTED the "not a
  security parameter" wording as stated: the cap is DUAL-FACE** (a resource ceiling on the
  honest-never-slashed axis; the evidence-size completeness bound on the deterrent axis). The
  local ledger penalty is consensus-inert in objective mode, so an un-evicted equivocator keeps
  its seat; a ≥⅓ coalition can make every pair over-cap with ~6 of its own valid renewals per
  block, so for fat coalitions accountable safety degrades to plain safety (attribution
  survives, eviction is lost). No admissible value closes that face — silt's evidence is the
  whole signed body — so the number is still ratifiable on immutable-#8 grounds with the face
  DISCLOSED; only the v5 two-level block hash (d-3) removes it, which therefore joins the R3.4
  pre-freeze carry-list, and the face goes to the R4.4 external brief. Invariant on the value:
  `SlashesBytesCap ≥ 2 × (default honest block) + overhead`. **⚠ SUPERSEDED 2026-09-10 — this invariant is
  UNSATISFIABLE as written; see the annotation below and `D-SLASHCAP-ROUTE`.** Two interims REFUTED: a
  consensus block byte cap (collides with `RegCap`); an attester-side byte policy (collides with
  the #432 forced-value rule). Source:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R0.6-SlashesBytesCap-value-security-face-DELTA-CERTIFICATION-2026-09-03.md`.
  **Value 16 MiB — ✅ RATIFIED by the owner 2026-09-03 ("2 ratified"), on immutable-#8 grounds, with
  the second face disclosed. The sentence ratified (Researcher Q4):** ratify 16 MiB as the memory ceiling
  knowing it is also the evidence size above which a double-signer keeps its seat with no on-chain
  penalty, including every member of a ≥⅓ coalition that splits finality using ~9 MB valid
  blocks, a face no value of the cap closes and only (d-3) removes.

  > **⚠ THE RATIFIED SENTENCE ABOVE IS LEFT VERBATIM AND IS PARTLY REFUTED — 2026-09-10.** It is not
  > rewritten, because the owner ratified those words and only the owner unratifies them; this note
  > records what a research certification found and what he must now re-ratify. Certification:
  > `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/SLASHCAP-NESTED-EVIDENCE-FIXED-POINT-RESEARCH-CERTIFICATION-2026-09-10.md`.
  > **(i)** *"and only (d-3) removes"* is **FALSE**. §4.3's `Slashes′` is a recursively reduced COPY,
  > not a digest, and it leaves `Entries` and `LastCommit` — neither of which has a validity bound on a
  > peer's block — so (d-3) is a ~40× constant shrink (2 → ~80 committed proofs), not a close.
  > **(ii)** The *"≥⅓ coalition"* clause is true but **materially understates**: `CheckEquivocation`
  > (`core/chain/equivocation.go:68`, verified at source) checks only key SIZE, equal height, differing
  > body, matching era and a shared `(round, phase)`. It never checks that the culprit is a bonded
  > validator, that the evidence blocks belong to this chain, or that a proof is not a duplicate. So a
  > **lone** proposer can armor itself with throwaway-key junk proofs — no bond, no coalition, no
  > misconfiguration. **DRIVEN AND CONFIRMED 2026-09-10** against real production code on `4c330c8` —
  > shipped `CheckEquivocation`, `SlashesEncodedSize`, `v5ValidateSlashes` and the full
  > `Chain.ValidateProposal` path, no mocks, reproduced independently. **A throwaway-key proof about two
  > never-committed blocks is 687 B and is ACCEPTED as evidence.** 24,456 of them pack `Slashes` to
  > 16,776,819 B — 397 B under the cap — and that block is **VALID under `v5ValidateSlashes` AND
  > `ValidateProposal` at shipped `DefaultConfig`**. A legitimate proof about it is 33,555,157 B, i.e.
  > 16,777,941 B over cap, and is REJECTED with `ErrSlashesBytesCapExceeded`: **the equivocator keeps its
  > seat.** Gate: `core/chain/TestRNestGate_SelfArmorMeasurement` + `TestRNestGate_JunkKeyProofIsAcceptedEvidence`
  > (`R-NEST-GATE`).
  > **The measurement CORRECTED the certification's own arithmetic.** §2.2 derived the armor threshold as
  > ≈8.39 MiB per side and flagged it unmeasured; binary search over junk-proof count puts it at
  > **≈7.9998 MiB** (12,227 proofs/side still admissible, 12,228 over) — essentially `SlashesBytesCap/2`
  > less ~1.5 KB of overhead. The derivation **overstated the attacker's cost by ~5 %**. Do not re-cite
  > 8.39 MiB.
  > **And the cheap route is not the one modelled.** Both the PE table and the certification reasoned from
  > two ordinary ~4.14 MiB reg-laden proofs; the driven construction reaches the same armored state with
  > **header-only** 687 B proofs, which is far cheaper for an attacker to build. Anything that prices the
  > attacker's cost must use the header-only route.
  > **(iii)** The close that does exist is not (d-3): `consensusSigBytes` (`core/chain/chain.go:1138`)
  > omits **Height** from the signature preimage — "the height rides inside the hash" — which is the
  > entire reason evidence carries full bodies (`equivocation.go:54-60` says so). Putting
  > `(height, round, phase)` in the v5 preimage makes evidence `O(1)`, ~200 bytes **[the ~200 B figure
  > is SUPERSEDED — ~251 B derived, unmeasured until G-PRE-9; see `D-PREIMAGE-CERT-2026-09-10`]**. That is CometBFT's
  > `CanonicalVote`. It is a **FORMAT** change and a **WIDENING** one, so it rides D1 or it costs an era
  > **[the "costs an era" clause is WITHDRAWN as a reason to buy — `D-FREEZE-REPRICE-2026-09-10`. The
  > freeze deadline is SOFT pre-launch; this change is bought on the #397 schema argument alone]**.
  > **The 16 MiB VALUE does not move.** Three owner calls are in §9 of the certification.
  >
  > **✅ UNRATIFIED AND REPLACED BY THE OWNER — 2026-09-10 (owner call 2).** The owner read the
  > refutation and unratified his own sentence in his own words: *"Those were my words and they're
  > wrong. The ratified text told me that buying (d-3) removes the double-signer face. It doesn't —
  > §4.3's `Slashes'` is a recursively reduced copy, not a digest, and leaves `Entries` and
  > `LastCommit` unbounded. A ~40× shrink (2 → ~80 proofs) is not a removal, and I ratified a sentence
  > that said it was."*
  >
  > **The replacement sentence, ratified 2026-09-10 — this is what the owner is buying:**
  >
  > > **(d-3) shrinks the face roughly 40× but does not remove it; what removes it is the v5
  > > signature-preimage change, which makes evidence O(1).**
  >
  > **⚠ WRONG A THIRD TIME, AND REPLACED BY THE OWNER — 2026-09-10
  > (`D-D3-CERT-REFUTATION-2026-09-10`).** The *"roughly 40×"* figure is refuted. `SlashesBytesCap` is
  > enforced against `SlashesEncodedSize` (`core/chain/chain.go:2571`), which marshals the ACTUAL
  > `[]Equivocation` — real encoded bytes with full `Block` bodies. (d-3) reduces the HASH PREIMAGE,
  > which that function never reads. **As specified, (d-3) shrinks the face by ZERO and is marginally
  > negative.**
  >
  > **THE REPLACEMENT SENTENCE, ratified 2026-09-10 — and it carries NO shrink figure, deliberately:**
  >
  > > **(d-3) does not shrink the evidence face. `SlashesBytesCap` is enforced against
  > > `SlashesEncodedSize`, which reads real encoded bytes, not the hash preimage. What (d-3) buys is
  > > the self-covering pruned body — retiring `Pruned`. What removes the face is the v5
  > > signature-preimage change.**
  >
  > **Why no number appears, recorded so it is not "helpfully" restored:** three shrink figures have
  > now been wrong, and the structural statement needs none. Any future shrink claim requires the
  > **mandatory evidence-strip validity rule**, which nobody has bought and which is not on the table.
  >
  > **The owner's own lesson, in his words — it is about the method, not the number:**
  >
  > > *"Three corrections to one sentence: 'only (d-3) removes' → false. The coalition clause → false.
  > > '40×' → zero. Each time I accepted a number I was handed and wrote it into the ratified record.
  > > I gave you the rule that every value claim must name what it closes independently versus in
  > > combination — and never once applied it to my own ratifications. **I ratified the figure instead
  > > of demanding its derivation.** That's mine, three times."*
  >
  > The 16 MiB VALUE is untouched by this unratification — only the disclosure sentence moved. The
  > owner also ratified the HANDLING as a standing practice: *"the annotate-in-place-rather-than-rewrite
  > handling was exactly right. Keep doing that with ratified text — the correction should be visible
  > as a correction, not laundered into the original."* See `D-PREIMAGE-BUY-2026-09-10`.

  Derived from shipped bounds: one legitimate evidence pair is at most two blocks at the
  default per-block budgets (2 MiB regs + 64 KiB entries) ≈ 4.2 MiB, so 16 MiB admits three
  fat proofs (or ~18k header-only ones) and is 1/8 of the 128 MiB transport frame. G-3
  measured (`TestSlashesBytesCapWorstCaseCost`): 5 reg-laden proofs at 3.00 MiB each fill the
  cap; 15.0 MiB resident after decode; `validateSlashes` 11.5 ms.
- **Gates (all GREEN; controlled revert RED).** T-1/T-2 the era-1/era-2 forgery through
  `Append`; T-3 a genuine double-sign still convicts; T-4 `TestPrunedEvidenceIsRefused`
  (supersedes `TestQ2_PrunedBlockStillSlashable`, which pinned the behaviour removed); T-5
  the honest-node vector (`core/node`, reproduced first); T-6 the cap over/at; G-2 honest
  detection never pairs a pruned block; G-4 memo bypass; G-6 one hash function (source pin);
  the I5 model-check gained three axes (declared-vs-signed height; `Pruned` ∈ {unset, real,
  forged}; era ∈ {1, 2}; 9,792 cases). Removing the recompute turns six gates RED; the
  Tester's independent half-revert matrix shows the refusal half and the recompute half are
  each gated on their own. Packing gate `TestProposerPacksPendingSlashesUnderTheBytesCap`
  (`core/node`): a backlog over the cap is carried, the proposal never exceeds the cap, and a
  proof that alone exceeds it is dropped, never embedded (PE ruling F-1). Researcher V-1 gate
  `TestOverCapProofDoesNotSilenceLaterProofsByTheSameCulprit`: the once-per-culprit LOCAL
  latch is separate from the ON-CHAIN queue latch, so a culprit whose first proof was over the
  cap still gets a later small proof queued; the over-cap WARN line is pinned as an S5 contract
  (V-6).
- **G-1, stated exactly.** Artifacts scanned: the four persisted chain fixtures in the tree
  (`core/chain/testdata/archival/{era1,era2,era2-pruned,mixed-era1-era2}.cbor`), against BOTH
  new invalidation predicates (pruned evidence; `Slashes` over 16 MiB). They carry zero
  `Slashes` at all, so both pass vacuously. No persisted field chain (`chain.cbor`) exists in
  the tree or in the local cloudtest evidence directories; the flixz deployment was not
  scanned. **Accepted explicitly:** no persisted chain silt holds carries a committed slash,
  and no network mints them today; the rule ships unconditional, no grandfather height. If a
  field box with committed slashes is ever restored onto this binary, the reload truncates at
  the first invalidated block and stalls loudly rather than accept it.
- **PE ruling:** MERGE-WITH-CONDITIONS, conditions F-1 and F-4 landed in the same PR
  (`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-R0.6-i5-evidence-recompute-3131d5a-2026-09-03.md`).
  Owed after: F-5 (pin `finalized > pruneFloor`, the G-2 derivation); the F-2 research
  question; a byte-tight at-ceiling fixture (the Tester: the accepted list lands 1.12 MiB
  under the cap).
- **Residuals, OPEN and named (not decided here).** R-EVIDENCE-BYTES (re-priced, bounded by
  the cap; re-opens if `RegCap` rises or #299 lands); R-BIG-EVIDENCE-UNSLASHABLE
  (pre-existing: evidence over the transport frame was never gossipable); **R-BOX** (the
  floor box reconstructs the slash write-set from `b.Slashes` without `CheckEquivocation` —
  the rule must appear on both sides or `box.Accept ⇒ node.Accept` is vacuous over slashes;
  routed to Boulder 1); R-MEMO (F5: `Hash()` writes a non-wire memo; G-4 covers this path
  only); R-RELOAD-RE-VERIFY (own-disk replay re-runs `validateSlashes` on committed history —
  whether to skip it is a separate consensus-rule question, NOT built on this cert).
- **Canon text changed with it.** `docs/design/consensus-invariants.md` I5 (new scar +
  assert); `core/chain/retention.go` (the "slashing window drops out" premise refuted);
  `core/chain/chain.go` `Hash`/`Prune` docs.

## D-GENESIS-ATTS-SEATING — a genesis seats only the attestations that verify over its hash; the rest are stripped, never refused

- **Status:** ✅ RATIFIED — 2026-09-04 (owner: *"I ratify 1"*). Certification:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/genesis-atts-seating-rule-RESEARCH-CERTIFICATION-2026-09-04.md`.
- **The rule.** In `AppendGenesis`, after the proposer-signature check and before apply, `b.Atts` becomes
  exactly the entries with `verifyAtt(a, b.Hash())`; never an error on this account; the committed
  `blocks[0].Atts` is the verified subset (so save / serve / reload are idempotent); a genesis `LastCommit`
  is refused (O1, hash-covered). A height-0 state-transition change (`validatorsSeen` is an era-3 leaf), so
  ratified, not era-gated: a height-0 rule cannot be.
- **Why.** `Atts` sit outside the `Hash()` preimage. A relaying peer could append an unsigned stub the
  proposer signature does not cover; the seating loop never verified attestation signatures, so the stub
  seated a phantom into `validatorsSeen` and diverged the era-3 committed root on a fresh-sync victim.
  Refusing the stub would let the same zero-key-material input wedge fork-adopt and `Reload`.
- **The two refuted alternatives.** *Strip all* (the delta cert's MG-C, built and reverted 2026-09-04):
  discards a real signer's consent and forks the seating predicate into a second copy; four bootstrap
  fixtures that seed a verified genesis attestation caught it. *Refuse present-but-invalid*: a zero-byte
  signature is present-but-invalid; that refusal is the free denial lever.
- **Soundness.** I1/I2 untouched; I3, I4, I5 strengthened; extensionally equal to the old rule on every
  honest history (production genesis carries no attestations — `core/genesis` emits Entries only; anchors
  seat at height ≥ 1 through the founding drain). Cost: |Atts| verifies once at boot, 0 in production.
- **Gates.** G1–G10 (`core/chain/genesis_atts_seating_test.go`, `core/genesis/genesis_atts_test.go`);
  strip-all reddens G2 + G5's verified control + one bootstrap fixture; refuse-invalid reddens G1/G3/G4/G5/G6.
- **Corrections travelling with this decision.** The 2026-09-03 delta certification's MG-C ("strip") is
  superseded; its premise "the launch anchors' genesis attestations seat them" was false.

## D-R2.9-DIRECTION — byte-denominated per-increment delivery settlement; strict parity; the two measurements authorised

- **Status:** ✅ RATIFIED — 2026-09-04 (owner: *"I also accept rulings on R2.9 and the future flixz.com
  measurement"*). Brief: `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R2.9-OWNER-BRIEF-2026-09-04.md`;
  certification: `R2.9-D-POD-KNOBS-delivery-settlement-repricing-RESEARCH-CERTIFICATION-2026-09-03.md`.
- **The break (re-verified on main 2026-09-04):** a server strictly prefers never banking a witnessed
  receipt above B = 50,000 bytes (payoff `0.875·(B − fee)`: +58.7 M, 1,342×, at 64 MiB); suppression is one
  default-off flag. B3 conservation is intact; incentive-compatibility of accept is what breaks.
- **The six rulings.** (1) Direction: PayWord-denominated per-increment delivery settlement under gates
  G-1…G-6; G-3 is satisfied by R2.14's spend-at-open; G-4/G-5 are re-derived for the shared paid-serial guard
  and the single fee constant (`relaypay.ShippedAnchorFace`). (2) The interim exposure is accepted
  (suppression is the shipped default; conservation holds; disbursement is behind `-economy`) and the order is
  R2.14 → R2.9 → R2.4. (3) STRICT parity (`p > r·U`); the unwitnessed bilateral fallback stays; `r = 0` on the
  witnessed-capable path is NOT commissioned in v1. (4) The two measurements are authorised — `B_bootstrap`
  (per-requester fetched bytes vs identity age on real traffic) and the honest arrival rate — and `grant/r`
  is NOT pinned until the first exists: the affordability knob is the RATIO `grant/r` (a total rescale leaves
  it invariant); parity-vs-`r = 0` decides only whether the bilateral fallback survives. (5) D-POD-KNOBS knob 1's
  cross-tier funding loop delivers 1/1,342 of its stated value on the witnessed lane at production sizes;
  escrow-over-burn STANDS on conservation grounds, its "funding loop" rationale is corrected here. (6) No relay
  skim in v1; R-RELAY-WASH-ZERO-LOSS re-opens with R2.12 (a burn-only skim on a content-blind lane does not close
  the faucet route that funds the wash).
- **The instrument.** `B_bootstrap` is USER behaviour: cloudtest measures its own synthetic fetch plan and
  cannot produce it. The raw per-requester byte counter exists on the ledger; the measurement is an export of
  it on real traffic — flixz.com (a private handoff, off public repos) — R2.9a.
- **Not decided here:** the value of `grant/r` (after R2.9a); R2.4's flip.

## D-FP2-SCOPE — the credit ledger stays ephemeral through the release candidate

- **Status:** ✅ RATIFIED — 2026-09-04 (owner: *"scope close it is"*). Brief and certification:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/FP-2-redeem-atom-and-ledger-durability-OWNER-BRIEF-AND-CERTIFICATION-2026-09-04.md`.
- **The decision.** The credit ledger (balances, escrow, provisional lanes) is NOT persisted before the RC.
  This is an explicit, tested posture, not an accident: FP-2, FP-1 (`Bank.spent`) and R-F8-RESTART-REWIND stay
  open and inert, and are re-armed automatically by the first of — the **R2.4 economy-ON default flip**, any
  **shared or multi-operator ledger**, or any **PR that persists any balance the ledger reads**. A Tester pin
  (G-FP2-0) fails when one arrives.
- **Why, in one sentence.** Persisting the ledger opens an uncertified economic question larger than the
  residual it closes: the account, order, escrow and root-owner maps are unbounded with no sound eviction
  rule, and every candidate rule is itself an economic mechanism — evicting a positive balance destroys
  money, evicting a negative one forgives a debt (the grant-refill mint), and refusing at the cap refuses to
  pay a new server — so build-immutable #8 cannot be met without a fresh certification.
- **The accepted cost, stated plainly.** The publish anti-spam fee's budget is per-process-lifetime rather
  than per-identity, and a restart destroys every prepaid durability escrow. The only live mint is the grant
  refill on a fresh account, which is the surface R2.12 already owns. Everything else lost at a restart is a
  forgotten debt or a destroyed observable. Both paid lanes and the repair economy are default OFF, and no RC
  gate names a durable ledger.
- **If it is ever built (`D-FP2-BUILD`, not ratified):** a SINGLE checkpoint-plus-journal store that subsumes
  `paidserials.log` — never two independent stores — in one PR with FP-1 and a certified account/escrow
  eviction rule, persisting no standing field; gates G-FP2-1…7 are entry conditions.
- **Two rulings that outlive the close.** **T-W:** the epoch advance sits OUTSIDE the redeem atom (which is
  the supersede through the payout); refused ⊇ swept iff the redeem's watermark is at least the sweep's — an
  ordering, not a transaction. So a future R-F8-RESTORE must persist the watermark *in the same atomic
  checkpoint as the guard set it swept*, or it opens a second-payout path that does not exist today.
  **Incremental persistence is refuted:** persisting balances without the provisional lanes is a mint, and
  persisting the lanes without conditional replay reverses every re-served lane at every restart.

## D-BB-BUILD-TAG — the `B_bootstrap` instrument moves behind a build tag, and inside it the flag gates the RECORDING

- **Status:** ✅ RATIFIED — 2026-09-05 (owner: *"I'll take your recommendation, proceed"*).
- **Seat reports:**
  `/Users/andrewedmond/.claude/silt-agent-memory/red-team/reviews/RED-TEAM-R2.9a-bbootstrap-instrument-and-containments-2026-09-05.md`;
  `/Users/andrewedmond/.claude/silt-agent-memory/crypto-specialist/reviews/ADVISORY-R2.9a-Bbootstrap-observability-containment-prior-art-2026-09-05.md`;
  `/Users/andrewedmond/.claude/silt-agent-memory/economist/reviews/ADVISORY-R2.9a-grant-over-r-containment-and-pinning-2026-09-05.md`;
  `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-R2.9a-minR-floor-3337e8b-2026-09-05.md`.
- **Certifications this is built on top of, both binding and neither reopened here:**
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R2.9a-Bbootstrap-DELTA-contamination-privacy-floor-clock-RESEARCH-CERTIFICATION-2026-09-04.md`
  (G-BB-11, the minimum-requester floor) and
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R2.9a-minR-floor-RECERT-sybil-pad-and-estimand-steerability-RESEARCH-CERTIFICATION-2026-09-05.md`
  (G-BB-11′, the property; BB-20). The instrument's shape is certified by
  `R2.9a-Bbootstrap-instrument-sufficiency-RESEARCH-CERTIFICATION-2026-09-04.md` and is UNCHANGED —
  no bin, edge, floor value or clock design moves here.
- **The decision, in two sentences.** The instrument compiles only under the `bbootstrap` build tag,
  so a default `go build` produces a binary with no histogram type, no census reader, no age
  stamping and no `-bbootstrap` flag. Inside a tagged build the flag now gates the RECORDING as well
  as the publication: the observability clock is injected only when the operator asked for it.
- **Why a tag rather than a better runtime gate.** Part VI Don't #3 is a claim about what silt
  **builds** — *"silt builds no mechanism to observe or link who-fetches-what… The refusal to build
  surveillance is absolute"* — not a claim about who can currently read the output. The shipped
  binary contained the mechanism and merely declined to print it, which satisfies the second reading
  and not the first. The red-team's F3 is the concrete form: `cmd/silt/daemon.go:673` injected the
  clock **unconditionally**, with a comment saying so deliberately, so every default-flags silt node
  recorded `(identity, cumulative bytes, first-seen wall-clock nanosecond)` for every requester, in
  RAM, with no flag to disable it. The `when` did not exist before R2.9a. Prior art reaches the same
  place: go-ethereum answered the analogous question about its `personal` namespace by **removing
  the capability from the network-facing surface**, not by authenticating it better.
- **The trade this reverses, accepted deliberately.** Unconditional injection bought one property —
  flipping `-bbootstrap` on at the next restart found an already-stamped population. That is gone. A
  tagged operator restarts **with** the flag on and then waits for the population to re-stamp. It is
  affordable because the instrument is run once, for one series, on one deployment, and because
  G-BB-15 already requires monotone uptime ≥ 2× the read bucket's upper edge — the wait is the run's
  own precondition, not a new cost on top. `BBootstrapRunPrecondition` voids a run carrying any
  unstamped account, so the half-stamped population is refused rather than fitted.
  **⚠ CORRECTION 2026-09-10 (docs true-up): it voids nothing — `BBootstrapRunPrecondition` has ZERO
  non-test callers.** `cmd/silt/bbootstrap.go` describes it in a comment and does not call it, so the
  half-stamped population is neither refused nor detected. The sentence above keeps its ratified text
  and carries this dated correction. Measured by the 2026-09-10 inert-mechanism sweep
  (`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/2026-09-10-inert-mechanism-sweep-core-adapters-0ed3b92.md`),
  confirmed here by call-graph read. Wiring it is ordinary lane work; it gets no residual name.
- **Alternatives rejected.**
  - **A token-gated endpoint (containment #2).** REJECTED: silt's status token is a **single
    unscoped secret that also authorises publishing and funding** (`cmd/silt/ui.go` `guard()`), so
    handing it to a monitoring scraper hands over mutation. Red-team F9. It also regulates the
    reader, which is the wrong axis for a claim about the artifact.
  - **A loopback bind check (containment #1).** REJECTED in favour of the tag: the existing guard
    checks a client-controlled `Host` header and **never the connection's `RemoteAddr`**, so a
    reverse proxy — the standard production shape — defeats it (red-team F5, F11; go-ethereum states
    the same limit verbatim for CORS). It remains a sound **deployment posture** and G-BB-12′ still
    stands; it is not a substitute for the artifact-level claim.
  - **Leaving it as-is with a better default.** REJECTED: a default-off flag on a mechanism that is
    present is exactly the shape Don't #3 rules on, and the recording was not behind the flag at all.
  - **Deleting the instrument.** NOT taken: `D-R2.9-DIRECTION` sentence 4 makes the measurement a
    precondition of pinning `grant/r`, and the tag preserves the run at the cost of one build flag.
- **What is explicitly NOT decided here.** The tail-merge of low-count cells (with the Researcher).
  `W`, `q`, the population `P` (G-BB-1, G-BB-9). G-BB-13′ Part A/B, the routable-interface question —
  the tag narrows it to tagged builds but does not answer it. Every open residual stands:
  R-BB-DELTA-TRAJECTORY, R-BB-CENSUS-SYBIL-PAD, R-BB-ANONYMITY-SET-SIZE,
  R-BB-SUPPRESSED-IS-A-DISCLOSURE, R-BB-EXPORT-SCALAR-BYPASS, R-BB-ESTIMAND-STEERABLE.
- **What the tag does NOT close, stated because a build flag reads like more than it is.** Under the
  tag, with the flag on, every red-team finding about the instrument's *contents* is live and
  unchanged — F1 (the attacker mints the anonymity set for $0), F2 (object-level attribution joined
  from the unconditionally published `stats.bytesServed` and `durability.objects[].funded`), F4, F7,
  F8. The tag removes the mechanism from every node that was not asked for it; it does not make the
  measurement safe on the node that runs it. **F2's object half is unflagged, unfloored and
  untouched by this decision** — it predates R2.9a and needs its own.
- **Gates.** `TestR29aTheFlagGatesTheRecordingNotJustThePublication` and
  `TestR29aFlippingTheFlagOnDoesNotRecoverThePastIsTheACCEPTEDCOST` (`cmd/silt`, tagged);
  `TestR29aDefaultBuildStampsNoFirstTouchOnRegister` and
  `TestR29aBondChallengeStillStampsFirstSeenTick` (`core/credit`, BOTH builds; the latter since
  INVERTED and renamed `TestR29aBondChallengeStampsNoFirstTouch` under G-BB-28, see the second
  correction below);
  `TestR29aDefaultBuildHasNoCensusReaderOnTheLedger`,
  `TestR29aDefaultBuildHasNoBBootstrapReaderOnTheNode`,
  `TestR29aDefaultBuildHasNoBBootstrapFlag`, `TestR29aDefaultBuildStatusHasNoBBootstrapKey`
  (untagged only). CI job `bbootstrap` reads the linked default binary for symbols and flag, then
  runs the tagged suite and asserts twelve named gates PASSED and nothing skipped.
- **One thing preserved on purpose.** `account.firstSeenTick`'s other writer, in
  `RecordBondChallenge`, predates R2.9a entirely, is stamped from the bond auditor's request counter
  rather than a wall clock, and fires only for a validator answering a storage-bond challenge. It is
  untouched, and it has its own gate so a later reader cannot mistake it for part of this mechanism.
- **CORRECTION — 2026-09-05, appended not substituted.** The bullet immediately above is **false on
  the fact**, and it is left standing so the record shows the correction rather than hiding it.
  Source: the blind principal-engineer review
  `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-R2.9a-bbootstrap-build-tag-d5099fa-2026-09-05.md`
  §6.2–6.3, which measured it on two real bonded validators.
  - **What the entry said:** `RecordBondChallenge`'s tick "is stamped from the bond auditor's request
    counter rather than a wall clock".
  - **What is true:** it is a **wall clock**. `core/node/bondaudit.go` computes
    `uint64(n.clock.Now()) + 1` (both at `bondAuditTick` and at `AuditBondsOnce`); the daemon builds
    its node clock as `clk := walltime.New(loop)` and hands that same `clk` to `node.New`; and
    `adapters/walltime` returns `time.Now().UnixNano()`. The cited DELTA certification had already
    recorded this — the entry restated the certification's own fact backwards. The writer fires on a
    `-validator` node (`StartBondAudit` is gated on `*validator`) for that node's own id and for
    every **bonded** peer that answers a challenge; it never fires for an unbonded fetcher, and a
    non-validator daemon never calls it.
  - **What follows, and it is the reason this correction is not cosmetic:** an identity that is both
    a bonded peer and a fetcher carries the full `(identity, cumulative fetched bytes, first-seen
    wall-clock nanosecond)` tuple in a **default** build — the exact tuple this decision's texts said
    was gone. The tuple's absence therefore holds on the **serve path**, for the general requester
    population, which is what the tag and the flag actually changed; it does not hold for bonded
    validator peers. Filed as open residual **R-BB-BOND-STAMP-TUPLE** (ROADMAP R2.9a), disclosed
    rather than closed: the residual predates R2.9a and the retention surface it feeds (`DecayStale`,
    `BondMaxAge`) is research-gated and routed separately.
  - **Pinned by a test, not by this paragraph:** `TestR29aBondAuditStampsAWallClockNanosecondNotACounter`
    (`core/node`, untagged) drives two real audit sweeps an hour apart and asserts the ticks differ
    by the elapsed hour rather than by 1, so a request counter — including a high-seeded one — fails
    it. `TestR29aBondChallengeStillStampsFirstSeenTick` now passes a Unix-nanosecond-magnitude tick
    instead of `77`, so the gate itself shows the wall clock.
  - **No behaviour changed.** `RecordBondChallenge`, `DecayStale` and standing retention are
    untouched by this correction; only the texts and the gates moved.
- **CLOSED — 2026-09-05, G-BB-28, appended not substituted.** `R-BB-BOND-STAMP-TUPLE` is closed by
  deleting the stamp, not by re-arguing it. Certification:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R2.9a-DONT3-READING-AND-BOND-STAMP-TUPLE-RESEARCH-CERTIFICATION-2026-09-05.md`
  §2 (Q2), ratified under `D-DONT3-READING`.
  - **The ground:** the field had NO reader in any build configuration. `DecayStale` reads
    `lastBondTick`; `Reputation` reads neither tick; the census reads `firstFetchTick` (since the
    fetch-only stamp of PR #737). A retained `when` no decided function needs is SURPLUS under
    T-DONT3 prong (a). "Predates R2.9a" grants no exemption, and "serves the bond auditor" was false:
    the auditor never read it. The correction bullet above was therefore right on the fact and wrong
    on the disposition — the residual was not research-gated by the retention surface, because the
    retention surface never touched it.
  - **What moved:** `account.firstSeenTick` and its write in `RecordBondChallenge` are deleted
    (`core/credit/credit.go`). `lastBondTick`, `DecayStale` and `BondMaxAge` are untouched.
  - **Gates:** `TestR29aBondChallengeStampsNoFirstTouch` (`core/credit`, untagged; the inversion of
    `TestR29aBondChallengeStillStampsFirstSeenTick`, which asserted the write as "something else's
    mechanism" — there was no other mechanism) reads the `account` type by reflection and asserts
    its set of tick-typed fields (`uint64`, `ports.Time`, `ports.Duration`) is CLOSED — exactly
    `{firstFetchTick, lastBondTick}` — so a tick added under ANY name reddens it with that name in
    the message. It was first shipped as a name match on `firstseen`; the blind review re-added the
    stamp as `bondSeenTick` and every gate stayed green, so the gate was widened to a type whitelist
    (`bondSeenTick uint64` and `bondSeenAt ports.Time` both measured RED). It does not see a `when`
    declared as a bare `int64`. `TestR29aRetentionReadsLastBondTickInNanoseconds`
    (`core/credit`, untagged) is the ablation that proves the deletion was surgical — `lastBondTick`
    still advances on a passing challenge, `DecayStale` still retires a bond one nanosecond past
    `BondMaxAge = 300 * ports.Second`, and a counter-valued tick never lapses, which is why
    `lastBondTick` must NOT be re-denominated. `TestR29aBondAuditStampsAWallClockNanosecondNotACounter`
    (`core/node`) stays green unchanged: it measures the tick the auditor passes, not the stored field.
    Both `core/credit` gates are named anchors in the default-build CI job.
- **CORRECTION — 2026-09-05, G-BB-29, appended not substituted.** The bullet "Why a tag rather than
  a better runtime gate" above is narrowed, not withdrawn. As written its rule — the binary
  "contained the mechanism and merely declined to print it", i.e. recording per se is the break —
  condemns `core/credit/delivery.go`'s `provKey{server, requester, root}` and `core/credit/escrow.go`,
  which are `D-S7` and are not going anywhere. The certification cited at the top of this entry's
  `CLOSED` bullet (§1.5) refutes that generalised rule and keeps the OUTCOME: the instrument must not
  be in a default binary because it records **SURPLUS** under T-DONT3 prong (a) — a per-requester
  `when` that no decided function needs — and is kept for the purpose prong (c) names, relating a
  fetcher to bytes over time. `core/credit/bbootstrap.go`'s header comment carries the same
  narrowing. `D-DONT3-READING` is the ratified reading; this entry is read under it.

---

**Correction, appended 2026-09-05 (G-BB-12′ design review, blind PE ruling
`RULING-R2.9a-G-BB-12-design-2026-09-05.md` S4; the ratified text above is unchanged).** The
"token-gated endpoint … REJECTED" bullet above is scoped exactly as the bind-check bullet
beside it already is: it rejected the token AS A SUBSTITUTE FOR THE ARTIFACT-LEVEL CLAIM (what
silt builds), which the build tag settles. It did not reject the token as the reader-axis
mechanism. Under G-BB-12′ the reader axis is the correct axis — the question there is *who may
read a block a tagged, flag-on node publishes* — and the API token, presented in the
`Authorization` header only, composed with the loopback-bind refusal, is the ratified
mechanism (`D-R2.9a-RUN-CALLS` item 4 delegated the mechanism to that review). The F9 reason
recorded above stands and is answered, not overridden: the bind refusal removes the token's
reason to travel, and a header-only predicate keeps it out of URLs and logs.

## D-DONT3-READING — how the who-fetches-what bright line is read

**Ratified 2026-09-05** by the owner: *"1 yes we can amend vision.md to include only what's
needed and nothing leaves the node. 2 ratified."* Certification:
`silt-agent-memory/researcher/reviews/research-outcome/R2.9a-DONT3-READING-AND-BOND-STAMP-TUPLE-RESEARCH-CERTIFICATION-2026-09-05.md`.

**What was decided.** The recorded reading of `docs/TENETS.md` Part VI Don't #3 is the
three-prong test **T-DONT3**. A record of who-fetched-what is inside the prohibition if any of:

- **(a) SURPLUS** — it holds more than serving the request requires, or holds it longer;
- **(b) REACH** — it leaves the node that produced it, by publication, persistence, or transfer;
- **(c) PURPOSE** — it is kept in order to link a fetcher to content.

The dimension of the record is not the test. A `(who, which object)` tuple can be outside the
line, and a bare `when` can be inside it.

**Two readings refuted, at opposite ends.** The 2026-09-04 rule *"publication is what makes a
record a surveillance mechanism"* is WITHDRAWN: it licenses an unpublished who-fetched-what log,
which is the exact mechanism the line names, and leaves the absolute half of the immutable doing
no work. The generalised rule behind `D-BB-BUILD-TAG`, *"recording per se is the break"*, is
REFUTED by silt's own default build: `core/credit/delivery.go` declares
`provKey{server, requester, root}`, written on every object-aware serve, up to 8,192 live tuples
in memory with no flag and no tag, and required by the D-S7 durability economy. The build tag's
OUTCOME stands; only the generalised rule behind it fails.

**Grounding.** The immutable's own text: *"a participating node sees the keys it routes and
serves."* And the code: `core/credit/credit.go` discards the `ChunkID` at the parameter the
port hands it — minimisation was already in force before this ruling named it.

**The VISION amendment.** `docs/VISION.md` said *"no mechanism, anywhere in the design, that
logs or links who-fetched-what."* Against `provKey` that was false as written, and it is a
published claim. Reworded to what serving requires and nothing leaving the node. The purpose
prong is carried here rather than in VISION.

**Residuals, open.**
- `R-DONT3-PROVLANE` — the provisional-lane tuple is outside the line only by necessity to
  D-S7 and only because it never leaves the node. **The first PR that persists the ledger
  engages prong (b) and moves it inside.** The FP-2 re-arm therefore carries a privacy trigger
  as well as its economic ones.
- `R-BB-BOND-STAMP-TUPLE` — **CLOSED 2026-09-05 (G-BB-28)**, remedy cost zero: `DecayStale` reads
  `lastBondTick`, `Reputation` reads neither, and once the fetch-only stamp landed the
  first-seen stamp was written by the auditor and read by nobody. The write and the field are
  deleted; retention is untouched. `lastBondTick` did NOT change: `DecayStale` compares against
  `BondMaxAge = 300 * ports.Second`, so it needs nanoseconds, and a counter would silently
  disable retention — pinned by `TestR29aRetentionReadsLastBondTickInNanoseconds`. Record: the
  `D-BB-BUILD-TAG` entry's second appended correction.
## D-STATUS-SNAPSHOT-INTERVAL — the `/api/status` recompute interval is 5 seconds

**Ratified 2026-09-05** by the owner: *"I'll ratify the 5 seconds for now. We can always
take user feedback later."*

**The parameter.** `statusSnapshotInterval = 5 * time.Second` (`cmd/silt/ui.go`). Between
recomputes every caller is served the same cached document. It is a **security parameter**,
not a tuning knob: `T` bounds a disclosure rate. An observer gets at most `floor(uptime/T)`
distinct documents however fast it asks, and every bin crossing inside one interval is
unresolvable. `docs/build-process.md` records that a durability knob has twice also turned
out to be a security parameter; this one is a security parameter first.

**Why a cache at all.** Certified REQUIRED on two independent grounds in
`R2.9a-instrument-necessity-geometry-bound-and-tail-merging-RESEARCH-CERTIFICATION-2026-09-05.md`
§3.5. First, the `R-BB-DELTA-TRAJECTORY` residual had been disclosed as "bounded by the poll
rate", and the poll rate is the reader's choice, so that was never a bound. Second, the
handler walked the whole never-evicted account set plus the whole chunk store inside the
node's event loop on every unauthenticated GET, which is a build-immutable #8 finding on its
own; caching caps a GET flood's amplification at one recompute per interval instead of at the
attacker's request rate.

**Why 5 seconds.** Derived from shipped numbers, bounded on both sides. From above by the fit:
the narrowest positive-width age bucket is 60 s, and the candidate windows the edges bracket
run from an hour to a week, so any `T` well inside 60 s is over-sampled by orders of magnitude
and costs the estimate nothing. From above by the operator: the shipped dashboard polls every
3,000 ms, so sitting just above that keeps the operator's view essentially live while making
the recompute rate strictly lower than the request rate. From below by privacy and loop cost.

**The cost, stated rather than buried.** The privacy side wants `T` much larger. At 5 seconds
an observer still collects 17,280 documents a day. The owner took the liveness side knowingly.

**Revisit trigger, named.** Operator feedback on dashboard liveness. The asymmetry matters:
raising `T` costs the estimate nothing until it approaches 60 s, so moving in the privacy
direction stays cheap later, while the liveness direction does not. One named site changes it.

**What this does NOT close.** The cache degrades the per-object join; it does not remove it.
At this deployment's traffic an interval routinely holds a single fetch, so an observer can
still attribute that interval's bytes to a named root. `R-BB-SIBLING-AGGREGATES` stays open,
and the token gate on the per-object detail is what actually closes the red-team's F2.

**Correction, appended 2026-09-05 (the ratified text above is unchanged).** A blind
principal-engineer review of the build measured the bound false as ratified. "At most
`floor(uptime/T)` distinct documents however fast it asks" held for `/api/status` only:
`/api/economy/self` republished `revenue.balance`, `revenue.servedBytes` and the pooled
`selfFunding.*` recomputed per request, so an observer polling it got the same aggregates at its
own rate (measured: a 16,388-credit step in `selfFunding.skimIn` recovered at 330 ms resolution,
131,104 bytes of a root `/api/roots` had named). The closing sentence above was false at the
same time: the token gate withheld `objects[]` but left `selfFunding.skimIn` open, which is
`Σ objects[].funded`, and on a node caretaking one object the sum IS the withheld counter.

As corrected (PR #737, second commit): (1) `/api/economy/self` is served from the SAME snapshot
as `/api/status` — one document, one loop pass, invalidated together — and carries the same
`snapshotTakenAtUnix` / `snapshotAgeSec` / `snapshotIntervalSec` stamps; (2) `selfFunding.*` is
token-gated with `objects[]`, by allow-list. **The bound now reads precisely:** an observer gets
at most `floor(uptime/T)` distinct ledger-derived documents from the two endpoints served off
that snapshot, `GET /api/status` and `GET /api/economy/self`. `snapshotAgeSec` moves per serve by
design, so "distinct" means distinct in what was counted. `/api/roots`, `/api/registry`,
`/api/chain` and `/api/library` are not snapshotted and carry no ledger counter.

**What stays open, named rather than implied.** The node-wide aggregates — `durability.balance`,
`stats.BytesServed`, `revenue.*` — stay unauthenticated on both documents, because the
cross-origin observatory (`cmd/silt/ui/observatory.html`) reads `stats.BytesServed` with no token
by design and no cross-origin consumer reads `selfFunding.*` at all. On a node holding ONE root,
which `/api/roots` names, those totals are that root's counters. That is
`R-BB-SIBLING-AGGREGATES`: still open, now rate-bounded to `floor(uptime/T)`, not closed. Closing
it means gating the observatory's bytes-served panel, which is the owner's trade and is not made
here. Gates: `TestR29aEconomySelfIsServedFromTheStatusSnapshot`,
`TestR29aF2NoUnauthenticatedResponseOnTheWholeSurfaceCarriesTheWithheldCounter`,
`TestR29aOneCacheTwoViewsAnAnonymousReadDoesNotStripTheOperatorsView` (`cmd/silt`), and the
live-daemon arm of `TestEconomyEndToEndOnLiveDaemon` (`e2e`). Source:
`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/2026-09-05-RULING-r2.9a-status-surface-cache-stamp-and-f2-gate.md`.

## D-R2.9a-RUN-CALLS — the seven `B_bootstrap` run-precondition calls, ratified 2026-09-05

**Ratified 2026-09-05** by the owner, in one sitting, after a walk-through of each open call
with its seat record. The five calls that belong to the run are here; the two that belong to
the UI surface (`R-BB-SIBLING-AGGREGATES`, the `/api/library` link key) are `D-UI-PRIVACY-FLAG`
below. ROADMAP item 12 carries the per-call sources.

**1. `grant/r` is re-pinned at 32 GiB, on geometry, before any run.** The provisional
`λ = 1 ⇒ 500,000 bytes` funded 1/134th of one 64 MiB production chunk and was refuted
without measurement (necessity certification §2.1; residuals certification §1.3). The value
follows the Economist's structural derivation `grant/r ≥ S_max / F_min`, never below the
stripe floor `K × chunkSize_max = 640 MiB`, with the two policy inputs the owner supplied:
`S_max` ≈ 28 GiB (*"Some movies can get up to 30GB of data in a single file"*), and
`F_min = 1` assumed conservatively because column placement puts each column on the 3 nodes
closest to its DHT key (`core/node/column.go`), so on a small fleet one host can be among the
closest for every column and serve the whole object. 28 GiB with margin rounds to **32 GiB**.
Owner's scope statement, recorded: *"I do anticipate much larger data sets will be published
to silt, but those will be long tail (research data dumps, etc) without much demand, so for
common / popular use 99.99% of files will be under 32GiB."* Consequence, accepted: an object
above 32 GiB served from one host is not fetchable on the grant alone; it needs column spread
across hosts or a paid balance. **Two things this pin does NOT do.** It covers one object,
not the cumulative draw over the ledger's life: the grant is one-shot per (viewer, server),
never topped up, and a pure viewer has no income path, so a repeat viewer on one server
outruns any finite ratio (`R-BB-GRANT-NOT-RENEWABLE`, on FP-2's re-arm list). And it is a
security parameter (build-immutable #4 calls cheap honest participation a security
constraint), so it ships in code only with the Researcher's G-BB-19 ratification sentence
naming the constraint and TIER each direction lands on, and under the ephemeral-ledger
statement the Economist asked for. G-BB-17 is satisfied by construction: the number is
structural, not a census reading. The Economist's condition stands as a sequencing rule:
**R2.12 (the faucet rate limit) lands at or before priced delivery goes live**; otherwise
the Economist's advice inverts to "do not enable priced delivery".

**2. The population `P` is all honest fetchers, including repairing and judging peers**
(G-BB-9). Owner: *"all honest fetchers, including repairing and judging peers."* Basis: the
residuals certification §3.3 — under the top-bucket reading rule the long-tenured caretaker
sits exactly in the read cell, and this answer puts caretakers inside `P` by definition, so
`C = 0` and `R-BB-CENSUS-MIXTURE` dissolves rather than needing a bracket the handoff might
not be able to form. The D-S7 half points the same way: a caretaker pays the same `r`, so
excluding it risks a ratio that starves repair. The earlier "viewers-only ⇒ Sybil-dearer"
reason was refuted (G-BB-22) and is not the basis.

**3. The run is re-scoped per the Economist's advisory §7.** Owner: *"follow economist
recommendation on scoping run."* `grant/r` is pinned structurally (item 1), and the flixz
census is re-aimed at the honest arrival rate and at falsifying the structural number, not
at pinning it. This drops the 14-day restart-free requirement (G-BB-25's one-shot constraint
still applies to whatever window is run). **`q` is left unpinned**: under the re-scope no
consumer of a quantile level remains, so G-BB-1′ has nothing to pin; the Researcher records
its disposition. The Economist's remaining asks travel with the handoff: answer G-BB-5
(gateway vs per-viewer nodes) first, and build T-1 (the insufficient-balance refusal
histogram) as the direct observable of the build-immutable #4 harm. Source:
`/Users/andrewedmond/.claude/silt-agent-memory/economist/reviews/ADVISORY-R2.9a-grant-over-r-containment-and-pinning-2026-09-05.md`.

**4. G-BB-13′ Part A: silt REFUSES `-ui <routable> -bbootstrap` at startup.** Owner:
*"refuse at startup."* Part B (the Don't #3 veto gate on a routable histogram) is therefore
not reached. Basis: the Economist verified no measurand needs a non-operator reader, so the
containment costs the measurement nothing; G-BB-18's pad screen is produced by polling and is
safe only when the poller is the operator; a containment that lives in an operator's head does
not scale to the pony tier. Refuse at startup, never silently omit the block (the absent-vs-
empty discipline). **What this does NOT decide: the G-BB-12′ mechanism.** The seats did not
converge on it — the Economist and the PE favour the bind refusal, the Red-team demonstrated
that a bind check is not access control (a default nginx `proxy_pass` forwards a loopback
`Host`, and `-allow-web-origin` is a designed bypass) and warned against reusing the shared
write token, and the Crypto-specialist documented Tor's per-connection remote-address policy
as the prior-art family. The Builder's PACE record for G-BB-12′ must address the proxy case,
and the ruling on the mechanism is a PE review, not this entry.

**5. The byte axis moves to 1 bin per doubling** (G-BB-23). Owner: *"1 bin per doubling."*
`BBootstrapBinsPerOctave` 4 → 1, `BBootstrapByteBins` 164 → 41. Basis: the Red-team's F4
measured that at flixz's scale 35–86% of occupied cells hold one identity and the count of
individually pinned identities is constant in the census size; the Researcher certified the
bin count as the ONLY lever that acts on that exposure (merging and rounding refuted at this
scale). The price is resolution, 19% → 2× ("between 4 and 8 GiB" instead of "between 4.0 and
4.75 GiB"), and under item 3 the precision side lost its consumer while Don't #3 stays on the
other side. Builder task under the `bbootstrap` build tag; the residual disclosure in the
code comment updates with it. *Correction appended 2026-09-05 (PR #742's blind PE review,
S1): "the pinned-identity count falls by about the same factor" is true only at large R. The
measured reduction is 1.4× at the census floor R = 10, 2× at R = 25 and 4× only at R ≳ 1,000,
and the limit is 1 at the floor by construction. The ratification stands — 1 bin per doubling
is weakly better than 4 at every R measured — but the floor and the bin count are weakest in
the same band, R = 10–25, and are not independent containments.*

**Builds this ratification creates (ROADMAP item 12):** the bin-count change; the startup
refusal plus the G-BB-12′ PACE; the G-BB-19 sentence from the Researcher before the 32 GiB
ratio ships; the re-aimed handoff.

**Research verdict on item 1, appended 2026-09-05 — GATED; the VALUE 32 GiB is REFUTED and
may not ship; the METHOD is CERTIFIED and G-BB-17 is lifted.**
`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R2.9a-grant-over-r-32GiB-structural-pin-G-BB-19-RESEARCH-CERTIFICATION-2026-09-05.md`.
The decisive artifact was `core/node/file.go`'s parity fallback (the `allData()` / whole-column
`fetchCols(parityCols, …)` block at the time): when ANY data chunk was missing the fetcher pulled
EVERY parity column of the whole object, so the worst-case per-server draw at `F = 1` is
`S_max · (N/K)` = 1.6 × 27.94 GiB = **44.7 GiB**, not 27.94. *Citation corrected 2026-09-06:* that
block is now a per-stripe DEFICIT walk (`docs/thinking/2026-09-06-parity-fetch-per-stripe.md`), so
an honest or WITHHOLDING provider can no longer force the object-size term; the N/K factor SURVIVES
as the worst case because `fetchFrom` transfers a shard's bytes before it verifies them — a
CORRUPTING provider forces `S · N/K` under any fetch policy. The pin's value is untouched; whether
`R-PARITY-AMPLIFICATION` is discharged is research-gated. The corrected formula is
`grant/r ≥ S_max · (N/K) / F_min`; 32 GiB covers 71.6 % of its own floor. This corrects the
Economist's §4.1 and the Researcher's own necessity cert §2.1. Two more corrections: the
"stripe floor `K × chunkSize_max` = 640 MiB" clause above is wrong twice (128 MiB is the
enforced maximum, and that cliff was itself refuted) — **G-BB-32**; and `grant/r = g/λ` is
invariant under a total rescale, so pinning the RATIO decides nothing about WHICH constant
moves (raising `g` inflates the free publish-token count; lowering `λ` makes small serves mint
zero) — **G-BB-30**, a second decision nobody has made. `F_min = 1` is CERTIFIED as the only
value the code supports (placement "prefers", never vetoes, a fresh domain; the fetch path
concentrates); it flips only on a code change, never on fleet size. **OWNER CALL RE-OPENED —
G-BB-31: re-ratify a value ≥ 44.7 GiB; the Researcher's input, not a pin, is 64 GiB.** The
G-BB-19 sentence is DISCHARGED — written in the certification's §4 for 64 GiB and valid for
any `V ≥ 44.7 GiB`. New residuals: `R-PARITY-AMPLIFICATION` (the parity fetch's 1.6×; whole-object at the time, a
per-stripe deficit walk since 2026-09-06, the worst case surviving for a corrupting provider), `R-GRANT-RATIO-NOT-A-CONSTANT` (blocking, G-BB-30),
`R-SMAX-PUBLISHER-CHOSEN`. What can ship before the value: the floor as a constant DERIVED in
code from `erasure.DefaultParams`, with a refuse-below assertion on the eventual `g/λ` (the
`-dht-address-reserve` shape) — an R2.9 build item, since `r` does not exist yet.

**RATIFIED 2026-09-05 (later the same day) — `grant/r` = 64 GiB.** Owner: *"Ratify 64GiB."*
G-BB-31 is CLOSED at the Researcher's input value, which clears the corrected floor
(44.7 GiB = 30 GB × 1.6, `S_max · (N/K)` at `F_min = 1`) with margin to the next binary value.
The G-BB-19 ratification sentence is the certification's §4 text VERBATIM, written for 64 GiB
(`R2.9a-grant-over-r-32GiB-structural-pin-G-BB-19-RESEARCH-CERTIFICATION-2026-09-05.md`);
its conditions bind: ephemeral ledger (any FP-2 re-arm reopens the pin and owes a
grant-renewal path), the R2.12 clause AS RESTATED below (G-BB-19′), `(K, N) = (10, 16)` and
`F_min = 1`. It
covers one object, never the cumulative draw; objects above `S_max` served from one host are
not fetchable on the grant alone. **Still open and NOT closed by this ratification:** G-BB-30
(the ratio is `g/λ` and a total rescale leaves it invariant — WHICH constant moves is a
separate decision with its own tier consequences, taken when R2.9 gives `r` a value), G-BB-32
(the "stripe floor 640 MiB" clause in item 1 above is wrong twice and stands corrected by
this note: 128 MiB is the enforced maximum chunk, and that cliff was itself refuted).
`R-PARITY-AMPLIFICATION` (the parity fetch that produced the 1.6× — whole-object until 2026-09-06, a
per-stripe deficit walk since; the 1.6× survives as the CORRUPTING-provider worst case because
bytes transfer before they verify) is a build residual on the fetch path, not a pin condition.

**G-BB-19′ — the pin's R2.12 clause, RESTATED 2026-09-05 (Researcher,
`R2.12-faucet-rate-tier-and-grant-ratio-composition-RESEARCH-CERTIFICATION-2026-09-05.md`
§3.6; documentary, the 64 GiB value is not reopened):** *R2.12 (the faucet rate limit) is
BUILT and ENFORCED wherever priced delivery is enabled. Write the identity-arrival rate `A`
(never `N`, which in this sentence is the erasure parameter). What the faucet bounds is `A`,
the rate at which fresh identities come to hold a spendable starter balance on one ledger. It
does not bound the byte-axis subsidy, which is bounded by the serving node's own upload
capacity with or without a faucet; and it does not bound the account map, which `Register`
grows on any first touch before the bucket is consulted. It is necessary because guard
occupancy is the product `A · (g/f) · (W+1)` and both factors must be finite. The pin's
realization must therefore satisfy, jointly, `g/r ≥ B_floor` and
`C ≤ maxPaidSerial · (f/r) / ((W+1) · B_floor)` — which is satisfied by lowering the lane
price `r`, and is violated by raising `g`.* Consequences recorded with it: **G-BB-30′** — `r`
is a PRICE a fetcher pays, not the server's self-mint `λ`; the ratio is realized in the price,
never by raising the grant (at 64 GiB via `g`, one grant is ~73× the whole guard cap and no
faucet rate bounds it); **G-R212-2 BLOCKS era-4 activation of the relay lane** — the shipped
relay price `RelayIncrementCredit/RelayIncrementBytes = 1/4096` (`core/relaypay/payword.go`)
puts `g/r_relay` at 1.9 GiB, 23.4× BELOW the certified 44.7 GiB floor; lifted by re-pricing
(raising `RelayIncrementBytes` ≈ 24×, which also shrinks the chain length and the guard
population) or by a certified derivation that the #4 floor does not apply to a relayed fetch.
The daemon now refuses to enable either priced lane with the faucet unconfigured (G-R212-1).

**G-R212-2, CERTIFIED 2026-09-06 — no longer blocks era-4; a re-price is CERTIFIED and awaits the
owner** (`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/G-R212-2-relay-lane-reprice-RESEARCH-CERTIFICATION-2026-09-06.md`).
Route (b) holds: the #4 floor does not bind the relay price, because the free splice is
unconditional (`D-POD-RELAY-COEXIST`) and no production path can open a paid session — a
load-bearing condition; if a free/paid differential or a forced paid path ever opens, #4
re-arms on `r_relay`. The gate's remedy (a) as first written was wrong: per-anchor yield is
`min(f·B/c, MaxSessionBytes)` with the remainder BURNED, so raising the price alone caps the lane
at 10 GiB and burns 95.6 % of every fetcher payment (`T-RELAY-GRAN`: a price change must move
`RelayIncrementBytes` and `MaxSessionBytes` together). **Certified value for ratification (§8 of
the certification, verbatim there): `RelayIncrementBytes = 524_288` (512 KiB), `RelayIncrementCredit
= 1` unchanged ⇒ `g/r_relay` = 244.14 GiB; `MaxChainLength := ShippedAnchorFace /
RelayIncrementCredit = 50,000` and `MaxSessionBytes := MaxChainLength × RelayIncrementBytes =
24.414 GiB` become DERIVED; `MaxAnchorsPerSession` derives to 1; the relay adapter's shared
free/paid per-splice cap takes the same value.** Bounded below by the 64 GiB pin read on the SUM
of the prices a NAT'd fetcher pays and by Don't #7 (at 256 KiB the relay is forced to out-earn the
server per byte); bounded above by T-AR (`B ≤ 1,048,576`). Face-neutral: `ShippedAnchorFace`
does not move, so `g/f` and the guard bound are unchanged and ONE-FACE is not engaged.
**RATIFIED 2026-09-06 by the owner (*"2. ratified"*): the certified value verbatim — `RelayIncrementBytes = 524_288`,
`RelayIncrementCredit = 1`, `MaxChainLength` and `MaxSessionBytes` DERIVED, `MaxAnchorsPerSession` derives
to 1, the relay adapter's shared per-splice cap takes the same value; the nine sites move in ONE PR
(cert §4.1). BUILT 2026-09-06; blind PE code ruling MERGE-AFTER folded in
(`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-G-R212-2-relay-reprice-code-2026-09-06.md`):
the derivation collapsed the face into S_max, so the face is now held by INDEPENDENT literals
(F-1) and the runtime budget `l.fee × k` is gated by a node-tier low-fee test (F-3); the shared cap
has its own gate (F-2). **Scope delta recorded against cert §3.4 (PE N-5):** the `Serve` refusal of a
per-splice cap below the protocol ceiling is UNCONDITIONAL, not scoped to `-accept-relay-payments` —
a free-only relay operator can no longer cap a splice below 24.4 GiB; kept unconditional on the PE's
recommendation (one cap, one constructor; a conditional refusal is a free/paid differential in
waiting). **Residual `R-RELAY-ANON-SET′` (PE N-6, research-gated, not asserted):** the certification's
"IMPROVED" reading is one-sided — `k_max = 1` dissolves the `k` partition, but guard (ii) rotates one
ephemeral per session and the session ceiling rose 24.4×, so ephemeral rotation per relayed byte
falls 24.4×.** **G-R212-7 (NEW):** STRICT parity `p > λ·U` at `λ = 1 credit/byte` and the 64 GiB pin
are two ratified decisions 137,439× apart — R2.9 cannot set its price until the owner resolves which
yields. 2026-09-06: the owner asked for context; the builder's brief recommends route (a), re-denominating
`λ` (a self-mint scaling constant with no external consumer) strictly below `p/U` through a Researcher
certification, over re-opening ruling 3. **Owner chose route (a) on 2026-09-06 (*"take route 1 as
recommended"*).** Consult chain opened the same day: Economist advisory on what re-denominating `λ` does
to the D-S7 escrow auto-skim (the unwitnessed serve feeds it) → Researcher certification of the
admissible `λ` interval strictly below `p/U` with `R-LAMBDA-DUST` disclosed → owner ratifies the value.
Nothing is built until the value is ratified. **CERTIFIED with corrections, 2026-09-06** (`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/G-R212-7-lambda-redenomination-RESEARCH-CERTIFICATION-2026-09-06.md`;
Economist advisory `/Users/andrewedmond/.claude/silt-agent-memory/economist/reviews/ADVISORY-G-R212-7-lambda-redenomination-2026-09-06.md`). Composed claim: re-denominating `λ` to 1 credit per `Dλ` bytes, jointly with a
per-lane byte-remainder accumulator and a re-denominated `RepairBountyBase`, satisfies STRICT parity, the 64 GiB
pin, Don't #7, T-AR, D-S7 and #4 **iff `U/p < Dλ ≤ 524,288` with `U/p ≥ 186,268`** (T-NUMERAIRE: parity bounds
`Dλ` from below; Don't #7 read against the already-ratified relay price bounds it from above). The Economist's
numéraire finding is CERTIFIED (λ alone moves D-S7's self-funding threshold 24 → 12,582,912 retrievals per
shard-repair; `RepairBountyBase` is implicitly 1 credit/byte) and its coupled set CORRECTED (`RelayIncrementBytes`
is the fourth coupled constant, already moved). `(U, p) = (262,144, 1)` CERTIFIED (81.38 GiB, 27 % pin margin);
the provisional `(4,096, 4,097)` REFUTED outright; "integer `PF ≥ 2`" REFUTED (`PF = 2` is the CEILING at
`U = 262,144` under the gross Don't #7 reading, inadmissible under the net one); the accumulator CERTIFIED with the
binding condition that the remainder lives on the provisional LANE, never the account (a supersede would double-pay);
the dust regime is 100 % at the shipped 64 KiB chunk, so the accumulator is REQUIRED. Sequencing: "all in ONE PR"
REFUTED — the certified minimal first step is **`λ` + `RepairBountyBase` + accumulator BEFORE R2.9** (parity is
vacuous today; four strict improvements incl. R-FLAT-FEE flipping +58.7 M → −43,601 credits). **NEW G-R212-8
(blocks R2.9):** one token face funds 12.21 GiB and `B_floor` needs 3.66×, so an object fetch must span ≥ 4 anchor
sessions with no face remainder burned. *[STRUCK 2026-09-07: the "≥ 4 anchor sessions" wording is the wrong estimand and would ship a vacuous gate if marked met; G-R212-8 is DISCHARGED as restated — the surviving form is the concurrency arithmetic `6 + 3 = 9 ≤ 10` gated at `cmd/silt/numeraire.go`; `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md` §8.4.]* Eleven gates G-λ-1…11 (cert §8.1). **Certified for ratification (cert §11,
verbatim there): `ServeMintBytesPerCredit Dλ = 393_216` (λ = 1 credit per 384 KiB, `PF = 1.5`) derived as
`⌈3·U/(2·p)⌉` from `DeliveryIncrementBytes U = 262_144`, `DeliveryIncrementCredit p = 1`; `RepairBountyBase =
c·k·shardBytes/(U/p)`; `f` and `g` do not move.** FIVE OWNER CALLS OPEN (cert §8.2): `Dλ` · `(U, p)` · the bounty
denominator · whether Don't #7 reads GROSS or NET · `AuditReward`/`AuditSlash`. Residuals: `R-NUMERAIRE-SANDWICH`
(held in tension), `R-BOUNTY-BASE-DENOMINATION` (blocking), `R-AUDIT-REWARD-DOMINATES`, `R-LAMBDA-WASH-MINT`,
`R-LAMBDA-DUST` (quantified, closable by build); `R-GRANT-RATIO-NOT-A-CONSTANT` / G-BB-30 CLOSED.

**RATIFIED 2026-09-06 by the owner (*"ratify first option in all, proceed"*) — all five calls at the
Researcher's first option. The certification's §11 sentence, verbatim:**

> **RATIFIED — the serve-mint denomination `λ`, and the delivery price it derives from.**
> `ServeMintBytesPerCredit Dλ = 393_216` — **λ = 1 credit per 384 KiB served** — derived as
> `Dλ = ⌈3·U/(2·p)⌉` from `DeliveryIncrementBytes U = 262_144` and `DeliveryIncrementCredit p = 1`
> (R2.9), never pinned independently. `RepairBountyBase` becomes `c·k·shardBytes/(U/p)`.
> `f = 50,000` and `g = 500,000` **do not move** (ONE FACE; G-BB-30′): the pin is realized in the
> **price**, and `λ` and the bounty base follow the price.
>
> **Bounded BELOW** by STRICT parity (`D-R2.9-DIRECTION` ruling 3, ratified): `Dλ > U/p`, giving an
> accept margin `PF = 1.5×` under R2.9's per-increment settlement. **Bounded ABOVE** by **Don't #7 —
> reward tracks value** (Tenet tier) read against the **ratified relay price**:
> `Dλ ≤ RelayIncrementBytes/RelayIncrementCredit = 524,288`, or a relay that stores nothing
> and bonds nothing out-earns the node that served the bytes. **`U/p` is bounded below** by the
> 64 GiB `grant/r` pin read on the SUM of the prices a NAT'd fetcher pays at once (G-R212-6):
> `U/p ≥ 186,268`; at 262,144 the grant buys **81.38 GiB**, 27 % above the pin.
>
> **A per-lane BYTE-REMAINDER ACCUMULATOR is REQUIRED, not optional**: at the shipped 64 KiB chunk
> a per-call floor mints **zero on 100 % of serves**. The remainder lives on the **provisional
> lane**, never on the account, or the supersede opens a double-pay; the skim derives from the same
> byte accumulator, never from the minted credits.
>
> **What this costs, stated plainly:** D-S7's self-funding threshold moves from the certified 24 to
> **36** object-retrievals per shard-repair; edge serve-to-fetch entitlement is **0.583**; one
> publish token costs **20.93 GiB** of unwitnessed serving instead of 57.1 KiB. **What it buys:**
> R-FLAT-FEE's incentive break flips (+58.7 M → −43,601 credits at 64 MiB) *before* R2.9;
> the wash-mint absolute rate falls **393,216×**; and D-S7's prepay lane becomes usable for the
> first time (one grant prepays **195** shard-repairs where today it prepays **0.00075**).
>
> **Tier: Evolving** for the VALUE. **The derivation shape is not a parameter**: `Dλ` from `(U, p)`,
> and the bounty base from `(U, p)`, are a build invariant.
>
> **What this does NOT decide:** `PF` above 1 is not derivable from any ratified sentence — its floor
> is set by the unmeasured operator cost of running the receipt lane (**G-λ-11**). And R2.9 still
> owes **G-R212-8** — restated 2026-09-06 by its own certification (the per-object face count was the wrong
> estimand): the binding quantum is the SESSION, one indivisible face per (fetcher, server); the composed pin
> is REFUTED while G-6 burns the remainder (T-QUANT), and the ruling that must move is G-6 — an owner call.
> *[Appended 2026-09-07: G-6 moved (the deposit released at anchor expiry, #763; T-DEPOSIT makes the pin TRUE at every φ), and G-R212-8 is DISCHARGED as restated — see the strike above; nothing further is owed under this name.]*

Also ratified at the first option: the bounty denominator `c·k·shardBytes/(U/p)`; Don't #7 read
GROSS (`Dλ ≤ 524,288`; the certified value clears the NET reading too); `AuditReward`/`AuditSlash`
LEFT at 1,000/25,000 with the disclosure that one passed audit is now worth ~375 MiB of gross
unwitnessed serving (`R-AUDIT-REWARD-DOMINATES`). **BUILT 2026-09-06** (`core/credit/numeraire.go`;
the certified first step: `λ` + `RepairBountyBase` + the lane accumulator, BEFORE R2.9). One gate
took a different shape than certified: G-λ-8's "start-up refusal" has no chunk geometry to read at
daemon start, so the built form is a loud judge-side settlement (`Stats.BountyBaseZero` + a WARN
journal line) plus a publish-time warning below `MinBountyChunkBytes`. The serve-mint telemetry
(`serveMint` on `/api/status`) is served to the token holder only: its skim sum reconstructs a lone
object's funded figure (red-team F2's shape).

## D-R2.9-NODE-HALF-CALLS — the five R2.9 node-half calls, ratified 2026-09-06

- **Status:** ✅ RATIFIED — 2026-09-06 (owner: *"I take all recommendations, proceed"*), on the brief given
  in-session against the G-R212-8 certification
  (`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R2.9-G-R212-8-delivery-anchor-quantization-RESEARCH-CERTIFICATION-2026-09-06.md`),
  the witnessed-demand certification (`…/R2.9-witnessed-demand-observable-under-sessions-RESEARCH-CERTIFICATION-2026-09-06.md`),
  the settlement-skim certification (`…/R2.9-settlement-skim-under-fetcher-chosen-deltas-RESEARCH-CERTIFICATION-2026-09-06.md`)
  and the blind PE ruling `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-R2.9-node-half-e3eb273-2026-09-06.md`.
- **(1) G-6, delivery lane — REFUND, not burn.** The unsettled session remainder is REFUNDED to the session's
  durable fetcher at close, with a per-identity cap of ⌊g/f⌋ live anchors replacing the burn's role as the
  paid-serial occupancy rate limiter. The MECHANISM owes its own certification before it is built; the code
  BURNS until then (`core/credit/deliveryanchor.go` `CloseDeliverySession` is the one seam). The relay lane does
  not copy it (its counterparty is an ephemeral). Amends D-R2.9-DIRECTION ruling 1's G-6 for this lane only.
- **(2) P-SESSION — RATIFIED.** D-DEMAND's token-level property is restated at the credit level on the anchored
  session lane (see D-DEMAND, now marked ratified). The price-level residual `R-DEMAND-PRICE-LEVEL` is a separate
  item for the economy-on decision; the bonded-fetcher credential is its lever.
- **(3) Scope — the node half SHIPS under G-6 pending call (1).** PR #760 merges although the G-R212-8 residual
  table reads "still blocks R2.9's node half": the lane is dark until era-4, off by default, the seam is
  isolated and the refutation disclosed. Recorded so a future reader finds the contradiction resolved here.
- **(4) `R-DEFAULT-CHUNK-BOUNTY-ZERO` — raise `pipeline.DefaultChunkSize` to 262,144 B** in its own PR before any
  economy-on flip, after the Economist confirms the census and dedup effects. **Premise corrected 2026-09-07:**
  the default pays a base of 2 (a shard is a whole ciphertext chunk), not zero — the real defect is a 20 %
  truncation under-pay (`R-BOUNTY-TRUNCATION`). The move stands on the Economist's other grounds (the certified
  D-S7 threshold of 36, one chunk = one delivery credit, exhaustive PoR audits) and carries a NEW call the owner
  has not yet made: fold the manifest true-length framing (`R-MANIFEST-PADDING`: 87.6 % of the flixz store is
  1.4 KB manifests padded to the chunk size; 3.9× store growth at 256 KiB otherwise) into the same
  content-addressing break, or defer both. Not built until that call.
- **(1′) AMENDED 2026-09-07 (owner: *"I'll take the recommendations for both"*), on the refund certification
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R2.9-session-remainder-refund-and-live-anchor-cap-RESEARCH-CERTIFICATION-2026-09-06.md`:
  the PAYEE is an account that already exists on the server's ledger, never the registering lookup (M1: the anchor
  is a bearer instrument, the presenting fetcher need not be the payer, and `acct()` conjures a grant); the
  remainder is released at ANCHOR EXPIRY (`maxAnchorEpoch + W + 1`, the guard's own window), not at session close
  (M2); the per-identity live-anchor cap is DROPPED as refuted (inapplicable: one bearer anchor passed down fresh
  keypairs buys unlimited guard slots for zero credits; the R2.12 assertion goes vacuous under it and survives
  verbatim only under M2). T-DEPOSIT: a fully refunded face is accounting-neutral and bounds concurrency, never
  capacity; the 64 GiB pin becomes TRUE at every φ; `R-PIN-VACUOUS-UNDER-QUANTIZATION` closes. Ten gates (cert §7; the
  guard-occupancy-is-bounded-by-the-credit-stock gate is written FIRST and proven RED against the refuted cap). New residuals
  `R-STOCK-RENEWABLE-OCCUPANCY`, `R-ANCHOR-BEARER-TRANSFER` (owed a red-team pass), `R-REFUND-NEEDS-AN-ACCOUNT`.
- **(4′) AMENDED 2026-09-07:** the chunk move to 262,144 B and true-length MANIFEST FRAMING (`R-MANIFEST-PADDING`)
  land in ONE PR — one content-addressing break — before the economy-on flip; `R-BOUNTY-TRUNCATION` stays a
  separate open residual. **BUILT 2026-09-07** (`pipeline.DefaultChunkSize = 262,144`, `pipeline.ManifestFrameSize`; deliberation
  `docs/thinking/2026-09-07-default-chunk-256k-manifest-framing.md`); the MERGE is held for the owner's go — it changes the
  root every NEW publish of already-published bytes produces. **Blind PE fold-in 2026-09-07**
  (`silt-agent-memory/principal-engineer/reviews/RULING-default-chunk-256k-manifest-framing-b365f10-2026-09-07.md`): the framing
  change alone had moved the GENESIS block hash (the entry's manifest chunk IDs are inside the hashed block) — the PE
  recommended pinning the padded frame; **the OWNER RULED 2026-09-07: accept the new genesis** (`f428d0a8…0951`; no live
  network exists, every development chain is wiped on upgrade), gated on the literal in `TestGenesisBlockHashIsPinned`;
  `pipeline.Options.ManifestFrameBytes` reproduces the old framing on demand; the freeze-surface question is filed for
  R3.4 (`R-GENESIS-HASH-FREEZE-SURFACE`); the second break class
  (re-framed manifest under an unchanged root → `ErrDupPublish`) is named and gated; `R-BOUNTY-TRUNCATION` stays OPEN and
  the ROADMAP row is reconciled to this sentence.
- **(5) The delivery idle window stays REFUSE-UNTIL-SET.** The Tester measures the block interval `T_b` on the
  next graded run; the window is then set as a small multiple of one epoch and recorded here. Ten minutes is the
  value for dark-lane testing only. Decide the wall-clock-step exposure (`R-SESSION-WALLCLOCK-STEP`) with (1).
  **`T_b` MEASURED 2026-09-07 (run `c450985-deep`, `integration/cloudtest/report-c450985-deep.md`):** the deep
  drive h64→h128 took 2877 s on the 12-seat rotation with the organic renewal treadmill running — **44 s/height**
  (the harness's own figure; 34 s/height over the 09:27–09:51 sub-window; the prior deep run measured 48 s). One
  derived epoch is 8 blocks ≈ **5.9 min**. The idle window is NOT set by this measurement — it is the owner's call
  (a small multiple of one epoch; 2–3 epochs ≈ 12–18 min is the arithmetic) and the guard cap's
  `capBlockIntervalBoundSec = 3600` bound stands at 82× the measured interval. Caveat: the same run also carried a
  17-minute liveness stall at h43 under f=1 down (`R-H43-ROUND-LADDER-DESYNC`), so the steady-state figure is not
  the worst case.

## D-UI-PRIVACY-FLAG — node-wide counters and the library link key go behind an operator flag; exposed in beta, withheld at release · EXTENDED 2026-09-09 to the WIRE

**Ratified 2026-09-05** by the owner, closing `R-BB-SIBLING-AGGREGATES` and the `/api/library`
link-key flag as decisions (the build is owed). Owner, on the counters: *"I actually am OKAY
with small edge nodes exposing some data in the beginning. This should be flagged as 'DEBUG
whilst in BETA, OFF in RELEASE'. Edge, hobbyist nodes will not need this UX, and it does
expose private information. Additionally we can keep the economist / nerdy information its
own toggle, for instance `-ui -privacy=off`, to make the point really clear."* On the link
key: *"same thing … make it optional. We will default ON through the BETA of flixz (labelled
pre-release information … this data is not exposed in production without the explicit
`-privacy=off` flag)."*

**The decision.** One operator flag on the UI server, `-privacy`, governs whether the
following are served to an UNTOKENED reader: the node-wide `stats.bytesServed`,
`durability.balance` and `revenue.*` on `GET /api/status` and `GET /api/economy/self` (the
counters that on a one-root node are that root's counters, `D-STATUS-SNAPSHOT-INTERVAL`
"What stays open"), and the `link` field of `GET /api/library` (the full `silt:v1:` handle,
which carries the decryption key; `core/link/link.go`, *"Handle is the full capability:
retrieve and decrypt"*). A token-bearing reader always gets them. **Default during the
flixz beta: exposed**, with the documents labelled as pre-release information on the wire.
**Default at release: withheld**; `-privacy=off` is the explicit opt-out for a node whose
operator wants the observatory's served column and bandwidth card, or a hosted resolver that
lists library links. The owner's reason the trade is acceptable in beta: the exposed case is
a single-root node, and flixz's nodes hold a whole catalogue, so on flixz the node-wide
total attributes nothing per title; the edge/hobbyist node, which is the exposed case, gets
the withheld default at release.

**What this resolves between seats.** The Economist wants the fleet metrics visible; the
Red-team's F2 showed the node-wide total is the object half of who-fetches-what on a one-root
node; the `#89` read-only exemption was reasoned for counters and the link is a capability,
not a counter. The flag gives the Economist the data on any node whose operator opts in and
gives the edge node the private default. It is a containment on the READER; it does not
change what silt records (`D-DONT3-READING` prong (a) is untouched), and it takes the
`R-DONT3-OBJECT-HALF` exposure knowingly for the beta window — an owner call on a Don't #3
question, recorded as such.

**Not decided here, for the Builder's PACE and a blind PE review:** the exact mechanism of
the default flip (a build tag as in `D-BB-BUILD-TAG`, or a default that flips on the RC
checklist), the wire form of the pre-release label, whether the observatory sends the token
or requires `-privacy=off` on its targets, and the Red-team's F9 point that the token is an
unscoped write credential (a read-scoped token is a separate follow-on, not a precondition).

**RATIFIED 2026-09-05 (later the same day) — the no-flip default.** Owner: *"Agree with the
privacy change."* The compiled default is WITHHELD in every build; the beta sentence above
("default ON through the BETA") is superseded by this ratification, and the flixz beta nodes
run `-privacy=off` explicitly, labelled PRE-RELEASE. The build note below stands as the record
of how the change was reached.

**One correction to the owner's rationale, recorded so it is not relied on.** The owner
cited large archival nodes having *"access to the takedown list feature that can remove the
ability of theirs to hold or serve that content"*. `D-TAKEDOWN` is a DECISION (a transparency
log for provable non-globality, low urgency); no takedown-list feature is built. The
privacy-flag decision does not depend on it.

**Revisit trigger.** The RC gate flips the default; the flixz beta closing is the named
moment. Any report of per-title attribution from a one-root beta node reopens the beta
default early.

**Build note, appended 2026-09-05 (the ratified text above is unchanged; ONE item is put back
to the owner).** Built on branch `builder/r2.9a-privacy-flag` after a blind PE design review
(`RULING-UI-PRIVACY-FLAG-design-2026-09-05.md`, PROCEED-WITH-CHANGES). (1) **The default is
WITHHELD in every build — there is no beta/release flip in code.** The two sentences above
conflict (*"default ON through the BETA"* vs *"not exposed in production without the explicit
`-privacy=off` flag"*); the PE ruled the second is a guarantee and the first a convenience, and
recommended no flip: a default a human must remember to change at release will one day ship
wrong, and the cost of no flip is one flag on the flixz launch command. Built that way and
asserted on the released artifact in `release.yml`. **This changes the ratified beta default and
is the OWNER's to confirm or revert**; the one-line site is `privacyDefaultWithheld` in
`cmd/silt/ui.go`. (2) The wire name is `stats.BytesServed` (no JSON tags on `node.Stats`), not
`stats.bytesServed`; the WHOLE `stats` block is withheld, deliberately over the letter above,
because `ChunksServed × chunk size` reconstructs the withheld byte count. (3) The link clause is
live on `silt client` only — a plain daemon has no linkbook — while the counters clause is live
on both. (4) `-privacy` is `on|off` as a string; any other value refuses to start. (5) The
pre-release label is a banner on the dashboard and the observatory when `privacy.mode` is
`off`, plus `privacy.{mode,default}` on every status response. (6) Compatibility: an OLDER
observatory page pointed at a NEW `privacy=on` daemon aborts its render (the old inline
dereference); upgrade the observing daemon.


### EXTENSION, ratified 2026-09-09 — the flag governs the WIRE, not only the reader

**Ratified by the owner 2026-09-09**, on the Researcher's certification
`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/C3-GOSSIP-DISCLOSURE-vs-D-UI-PRIVACY-FLAG-RESEARCH-CERTIFICATION-2026-09-09.md`
(verdict GATED, certifiable only as alternative A; ratification required in every alternative).
The sentence ratified, verbatim:

> **`D-UI-PRIVACY-FLAG` is extended from a reader containment to a disclosure containment:
> `-privacy` governs the node-wide serve and repair counters on the WIRE as well as on the
> HTTP surface. With `-privacy` on (the compiled default in every build) a node gossips
> neither `ServedBytes` nor `RepairsDone`; with `-privacy=off` it gossips both, labelled
> pre-release as the HTTP surface already is. The accepted cost, stated: on the default
> posture the network serve- and repair-inequality series render as a named absence
> ("sample too small") on most nodes; the tier mix, the published bands, the target ratio
> and the committed `C2` block are unaffected.**

**Why the extension was needed rather than assumed.** Lane C3 added the two counters to peer
gossip. The Researcher found the original containment survives on the letter and is voided in
substance: the gossiped value is not the same CLASS as the withheld counter, it is the SAME
INTEGER; `D-STATUS-SNAPSHOT-INTERVAL` ratified the 5 s snapshot as a security parameter
precisely because a reader picks its own poll rate, and a reply-carried counter hands that rate
back; and the audience widens to nodes with no HTTP surface at all — the exact node this flag
was ratified to protect. A blind PE and a blind red-team independently confirmed a working
break on the route side (free identities let an attacker supply n−1 of a published aggregate's
terms and solve for the last one exactly), which is fixed separately by the route clause.

**What was refuted, so it is not re-proposed.** A coarsened form — a band, a rate or a rank —
is REFUTED: a band still yields a monotone step sequence under repeated probing, a rate
publishes the derivative the attack has to work for, and a rank cannot be a wire form.

**The consequence, accepted with the ratification and NOT a surprise.** The publish flag is
welded to the privacy default, so on the shipped default no node gossips work counters, the
non-reporting exclusion drops everyone, and both concentration series are structurally empty:
**silt can see who is PRESENT and never who does the WORK.** Every concentration-based abort in
the R2.4 canary is therefore inoperative on a default fleet, and every concentration baseline
measured to date is measured on an opted-out topology. Recorded on `ROADMAP.md` row C6 as an
owner/Researcher call before that canary runs; the alternatives and their costs are laid out in
`docs/thinking/2026-09-09-work-visibility-on-the-default-posture.md`.

## D-R2.12-EMPTY-BUCKET — an empty faucet bucket ADVANCES one publish fee; deny is opt-in

**Ratified 2026-09-06** by the owner (*"4. advance"*), closing the one R2.12 owner call. When the
token bucket is empty at an identity's first spend, the identity receives ONE publish fee now as an
ADVANCE on its starter grant, stays grant-pending, and is topped up to the full grant when a token
later admits it (`completeGrant`, G-R212-3). The advance is never a settlement: a settled floor would
cap an honest identity below the build-immutable #4 affordability cliff forever. Rationale (PE and
Researcher, both recommending the advance): the honest onboarding floor stays structurally non-zero
while a farm's per-identity yield under an exhausted bucket falls tenfold (one fee vs the grant).
Mechanics: `-grant-deny-floor` defaults to `-1`, a sentinel that resolves to the LEDGER's fee (never a
duplicated literal); `0` opts into deny; a positive value is an explicit advance; the default is inert
while the faucet is unconfigured (refuse-until-set stands, the rate is still a security parameter with
no shipped default). Gate: `TestR212FaucetFlagsRefuseHalfAndUnsafeConfigurations`.

## D-R3.1-EMPTY-LEAF — `statehash.Root` refuses an empty leaf value

**Ratified 2026-09-06** by the owner (*"1/ ratified"*), closing G-R31-5 and with it R3.1 as an owned
residual. `Root` returns `EmptyValueError` for a leaf whose value is empty. The SMT library treats an
empty update as a DELETE, so an accepted empty value would silently drop the key from the root; every
committed field encodes to a non-empty value (the `Present` marker, a fixed-width scalar encoding, or a
32-byte set digest, `core/chain/statehash.go`), so no honest root changes and this is a validity-surface
tightening, not a consensus change. Gate: `TestRootRejectsEmptyLeafValue` (with a positive control
showing the one-byte value IS committed).

## D-CONSENSUS-ARMING — the round clock arms on a replicated condition; a relayable round certificate; the published f+1 liveness bound

- **Status:** ✅ RATIFIED — 2026-09-07 (owner: *"agreed with all (ratify)"*), on the certification
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md`
  and the RED gate G-H43-1 (`core/node/modelcheck_h43_arming_test.go`, Tester branch
  `tester/h43-round-ladder-desync-modelcheck` @ `8b1e1ef`). ROADMAP owner calls 18, 19 and 20.
- **The defect (attributed):** `core/node/rounds.go:306-310` arms the round clock on LOCAL mempool content, so the
  round number is a function of unreplicated private state; the quiescent branch zeroes `rs.Sweeps`; `proposeBlock`
  re-derives the round from `rs.Round`; `Changes[r]` is a point record. Field shape: run `c450985-deep`, block 43
  committed 17 min 20 s after block 42 under f=1 down. #441 (publish starvation) is the same arming clause.
- **(18) The rule, a consensus-rule change (I4; I1 preserved by `slotCompare`'s round term, never by the arming
  rule):** (A) arm the round clock on a REPLICATED condition — pending work OR any consensus message for the working
  height (PBFT arming restored to uniformity; Tendermint L21); (B) suffix-semantics round-changes plus a relayable
  round certificate (the DiemBFT timeout-certificate schema); (C) the designee proposes at the certificate's round.
  Built ONLY against the RED gates G-H43-1…8 (Tester, RED-first), in one PR, with the Researcher re-certifying the
  composed change before merge. No graded cloud run until G-H43-1 is GREEN on the fix.
- **(19) The published bound and the harness:** silt publishes liveness under f=1 down as **≤ f+1 rounds**
  (190 s ≈ 4.3 × the measured 44 s block interval); the cloudtest `6-fault-tolerance` tiers are re-priced to
  190 / 380 s and the `ft_publish` empty-string defect (`scenarios.sh:526-541`, which made the 1445 s cap unmeasured)
  is fixed with G-H43-7; graded validators run `-log debug`.
- **(20) #380:** in objective mode `ValidateCommit` ignores the local `Config.Quorum` floor and defers to
  `bftThreshold`; `Quorum` stays a proposer-side gather target only. A separate GATED item (I1) behind G-H43-8.
  - **AMENDED 2026-09-08 (G-380-B; the owner ratifies this sentence at the merge of the A2 PR):** the ratified
    sentence reads, in full, *"in objective mode `ValidateCommit` ignores the local `Config.Quorum` floor and defers
    to the DERIVED Byzantine rule: `bftThreshold(N)` in the launch window and with epochs off, and a count floor of
    0 in a mature epoch, where the >⅔ frozen-weight rule is the bar (research certification
    `CONSENSUS-380-quorum-floor-direction-1-PREDICATE-AND-CERTIFICATION-2026-09-08.md` §1)"*. That certification
    is the ONE certification for the change (rule 9:
    `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/CONSENSUS-380-quorum-floor-direction-1-PREDICATE-AND-CERTIFICATION-2026-09-08.md`,
    §1 the predicate, §6 the composed diff). Head-counting the epoch set stays REFUTED (B2); the trusted opt-out (`-byzantine-quorum=false`) and legacy mode keep `cfg.Quorum`
    unchanged; `cfg.Quorum` stays the gather target on EVERY proposal path (so a uniform swarm's blocks carry the
    same attestation count as before — the change adds accepts only). Three merge conditions rode with it: Reload
    counts the same predicate; `newViewFor` refuses a certificate with no round-change from anyone but the
    designee; `RequiredQuorum` is pinned by G-D13. Owner calls surfaced, batched: the `-quorum 3` default (f = 0 at
    four validators — a derived default on the untrusted objective path) and whether a single-anchor objective
    launch stays supported (at A = 1 no count gate holds anything; the PE recommends requiring ≥ 2).
- **What this does NOT decide:** the delivery idle-window default (owner call 4) — set only after the fix lands and
  the bound is field-confirmed, since the reaper is wall-clock and a stall reaps live sessions.

## D-RC-SCOPE-S1 — the Release Candidate ships with the economy default-OFF; the flip is a `0.9.x` release before `1.0.0`

- **Status:** ✅ RATIFIED — 2026-09-07 (owner: *"agreed with all (ratify)"*), on the Economist's scope advice
  `/Users/andrewedmond/.claude/silt-agent-memory/economist/reviews/ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07.md` §4 (option b) and the planner's recommendation in `ROADMAP.md` (scope call S1).
- **The decision:** the RC (`0.9.0`, the era-4/v5 stamp-raising release, format frozen, floor box never-Accept)
  ships with `-economy` DEFAULT-OFF and every paid lane BUILT, gated and dark. The external B8 pass attacks the
  frozen consensus AND the economy under `-economy` ON in the harness. The default flip (R2.4) is a `0.9.x`
  release inside the RC line, after R2.7's adversarial-solvency verdict is clean and before `1.0.0`.
- **Why (b):** under (a) flip-before-RC a pony eats silent restart loss on the ephemeral ledger (`D-FP2-SCOPE`) and
  the T-AR baseline is lost forever; under (c) flip-after-1.0.0 the edge carries durability's cost for a whole
  major version with the reward half dark — a T-AR violation; under (b) it defers ~33 % of revenue for one release.
  The flip re-arms FP-2 / FP-1 / `R-F8-RESTART-REWIND`; freezing the format does not depend on it.
- **What this does NOT decide:** S2 (the operational floor), S3 (`#558`), S4 (`#437`) — decided the same day, `D-RC-SCOPE-S2-S4` below.

## D-TRUE-UP-CALLS-2026-09-07 — the fifteen true-up owner calls, ratified 2026-09-07

- **Status:** ✅ RATIFIED — 2026-09-07 (owner: *"1 ratify 2 ratify 3 decline, doc fix only, 4 accept recommendation
  5 accept 6 accept 7 accept 8 SKIP 9 ratify all nine 10 owed after the run, 11 ratify 12 owed at D3 13 accept
  recommendation, 14 accept recommendation 15 leave 19 accept recommendation 20 main only first"*), on the owner-call
  list in `ROADMAP.md` as worded by the 2026-09-07 true-up (PR #768) and the certifications and rulings each call
  cites there. Calls 18–20 were ratified earlier the same day (`D-CONSENSUS-ARMING`); the owner's "19" and "20"
  therefore read positionally as calls 17 and 16 (the file lists 18, 19, 20, 17, 16 after 15), and "main only
  first" is call 16's recommendation verbatim. Calls 10 and 12 stay OWED (after the R4.3b shadow run; at D3).
- **(1) `R-membership` (Lane B2):** retire `slashedRoot` and `validatorsSeenRoot` from the v5 committed digest
  set (D-V5-WHOLESET-ROOTS five → three) rather than cap seated identities; a hard fork at activation, free while
  era-4 is dark; not certified until the `objective()` guard (G-1, covering `MinBond` too) lands. PRE-FREEZE.
- **(2) The recovery boundary (Lane B3):** the floor box is a COLD AUDITOR — unconditional loud stall,
  `RecoveryDirective.Heights` and `LiveFollower` deleted, pruned blocks refused, `trustFloor` off the contract
  surface, recovery by a fresh `-ws-checkpoint`-class anchor at H+1, irrecoverable if unreachable.
- **(3) `R-CARRIER-CREDIT-DENIAL` (Lane B6):** DECLINED — no multi-block inclusion window in v1 (the carrier
  already narrows the denial to one proposer; the window is a format change). A doc fix is all that ships.
- **(4) The delivery idle window (Lane C2):** stays REFUSE-UNTIL-SET. The reaper is wall-clock
  (`core/node/deliverysession.go:29-31`), so a chain stall reaps live sessions; the default is set only after Lane
  A1's fix lands and the ≤ f+1 bound (190 s at f=1) is field-confirmed on a graded run, and then ABOVE that
  bound (today's arithmetic: ≥ 4 epochs ≈ 24 min at `T_b` = 44 s).
- **(5) `R-SESSION-WALLCLOCK-STEP` (Lane C2):** ACCEPTED as a disclosed v1 residual — a forward wall-clock step
  (or a stall) reaps every live session; the deposit returns at anchor expiry; only latency is lost.
- **(6) `R-CREDITSPENT-UNBOUNDED` (Lane C2):** ACCEPTED — the 65,536 cap is an OPERATOR-MANAGED ceiling (rotate
  the publish key AND clear `creditspent.log` together) until the epoch-bind lands; the epoch-bind is not a
  consensus-format item (freeze manifest item 12) and stays research-gated.
- **(7) `R-ANCHOR-STALL` (Lane C2):** ACCEPTED as a disclosed v1 residual — ≤ 300,000 credits per 1 GiB relay
  session; R2.14b `MsgRelayFund` is the follow-on.
- **(8) Cloud row 13b (Lane C2):** SKIP, not GAP, while era-4 is dark, behind the era probe.
- **(9) The freeze manifest §8 (Lane D1), all nine sentences:** buy `tagRevLogSize` (a SAFETY leaf, since a
  witness-supplied `m` is a wrong-accept); buy (d-3) `AnswerDigest`; reserve the PoP slot; the `IssuerKeys` cap at
  COUNT 4,096 with its proposer packing budget and `(issuer, epoch)` distinctness clause; `SerialSize` 32;
  refuse-to-start across a format boundary (#237); the genesis hash is network identity, outside the era surface;
  the readiness tally stays behind `everMature`; no activation mechanism beyond the tally. This ratifies the
  manifest's CONTENT. The freeze ACT (D3) is still the owner's, at the RC, after D1 + D2 + B1–B4 + B8.
- **(11) R4.2 the A-axis (Lane E2):** re-scoped to measure / publish / hand to B8 as-is; A3 is NOT wired. The
  bonded-adversary domain-collision finding goes in the R4.4 brief.
- **(13) `R-DEMAND-PRICE-LEVEL` and `R-RELAY-WASH-ZERO-LOSS` (Lane C5):** the Economist's position is adopted as
  the standing one carried into the R2.4 flip — `(U, p)` unmoved, `RequireBondedFetchers = false`, no relay skim
  in v1 (`RelaySkim = 0/1`, a disclosed residual). It is re-opened at the flip only if C7's adversarial-solvency
  verdict contradicts it.
- **(14) `R-ISSUERKEY-POP` and `R-E2E-ERA4-FIXTURE` (Lanes D1/D2):** the PoP slot is RESERVED inert at the stamp
  raise (not built); the e2e cost of the objective + bonded + epoch-enabled fixture is accepted.

  > **⚠ THE PoP HALF IS UNRATIFIED — 2026-09-10, owner call D (`D-ITEM4-DROPPED-2026-09-10`).** The
  > sentence is left verbatim because those are the owner's ratified words; this note records what he
  > has since withdrawn. **The reservation is DROPPED.** It was bought on a cost that no longer
  > exists: reserving an INERT, unpopulated field buys exactly one thing — not paying an era later —
  > and the freeze deadline is SOFT pre-launch (`D-FREEZE-REPRICE-2026-09-10`). **The e2e-fixture half
  > of (14) is untouched and still stands.**
- **(15) The `-grant-capacity` help note:** left as is.
- **(16) Structure Round 1B (Lane B1):** main-only FIRST — Round 1A is the main-only spine per the PE's 12-step
  brief; the five box-entry-dependent closers wait for Round 1B after the HELD `builder/floorbox-structure`
  branch is re-applied file-by-file. Round A is NOT merged first.
- **(17) `R-PS-LOCAL-ROT-NOT-HEALED` (Lane TAIL):** take it — evict a verified-rotten shard the node still hosts
  and announces so the fetch path replaces it, trading one operator's hosting count and D-S7 revenue for truthful
  durability. Research-gated if the fix reaches bounty or escrow.
- **What this does NOT decide:** call 10 (R4.3b `on`: R = 4, `cap_relay` = 4 — owed after the shadow run reports
  series A < 5 % and series B < 20 %); call 12 (commissioning the external B8 seat — owed at D3); the delivery
  idle-window VALUE (owed after A3); the `R-CARRIER-BYTES` value (B4, owed on the Tester's measurement).

## D-RC-SCOPE-S2-S4 — the operational floor is post-RC (Boulder 5); `#558` is RC-blocking; `#437` is post-RC

- **Status:** ✅ RATIFIED — 2026-09-07 (owner: *"S2 - accept recommendation S3 accept recommendation S4 accept
  recommendation"*), on the planner's recommendations in `ROADMAP.md` (scope calls S2–S4) and, for S2, the flixz
  handoff's post-RC placement of packaging. Completes the four scope calls with `D-RC-SCOPE-S1`.
- **S2 — the operational floor:** packaging, signed installers and R4 self-update are POST-RC and define
  **Boulder 5**. The two S6 scaling kills — O(delta) maturation and reprovide dirty-tracking — are `1.0.0` GATES
  (on the E6 path, not the RC), because O(store) cold-start and O(held) reprovide price out the honest operator
  (build-immutables #4 / #8).
- **S3 — `#558` torn `chain.cbor` → silent genesis fallback:** RC-BLOCKING (a real silent-loss residual against
  the no-silent-loss floor). One builder PR, no research gate: an atomic chain-store write plus refuse-to-start on
  a torn tail, with the Tester's torn-tail fixture RED first. Homed as Lane B8, independent of B1–B7.
- **S4 — `#437` transport authentication (TLS / Noise):** POST-RC (`1.x`). Certified NOT a safety break; the
  residual is censorship over a controlled link, already inside the liveness model. The crypto choice is
  research-gated when it is scheduled (`docs/network-durability.md` first, build-immutable #5).
- **What this does NOT decide:** Boulder 5's internal order and its Rocks — defined when the RC ships.

## D-H43-WORKLESS-DESIGNEE — the restated liveness bound, the entry-forward cap, and the null proposal routed to era 5

- **Status:** ✅ RATIFIED — 2026-09-07 (owner: *"1/ ratify, 2 ratify 3/ ratify. please proceed"*), on the blind delta
  certification `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/CONSENSUS-LIVENESS-h43-ABC-ASBUILT-workless-designee-RESEARCH-CERTIFICATION-2026-09-07.md`
  and the composed-diff re-certification `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/CONSENSUS-LIVENESS-h43-COMPOSED-DIFF-c2a476a-RESEARCH-CERTIFICATION-2026-09-07.md`,
  with the blind PE ruling `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-h43-consensus-arming-c2a476a-2026-09-07.md`.
  ROADMAP owner calls 21, 22 and 23; built and merged as PR #772 (`builder/h43-consensus-arming`).
- **The mechanism this amends `D-CONSENSUS-ARMING` for:** `R-H43-WORKLESS-DESIGNEE` (M4) — a round whose designee is LIVE but
  holds none of the height's pending work is wasted exactly like a round on a down designee, because the empty-block refusal
  is a validity rule and the rotation is blind to who holds work. Present at f = 0; masked by M1 until (A) unmasked it. The
  designee has PRIORITY at its round, never exclusivity (the #338 takeover fires at any round; this corrects the #441
  certification's published shape). Closers built: pending ENTRIES are forwarded to the round's designee on round entry
  (registrations stay owner-submitted — forwarding them is REFUTED, the relay refusal is the #424 closer), the takeover walk is
  keyed to the round's designee and kept monotone, the designee attempts once per (h, r).
- **(21) The published bound (published-claim change, amends `D-CONSENSUS-ARMING` (19)):** silt publishes liveness after GST
  as **≤ f′+1 rounds, where f′ counts governing-set seats that are DOWN or that hold none of the height's pending work at the
  round they are designated.** With the entry forward landing, f′ = f and the number stays **190 s at f = 1, N = 12**; a lost
  forward is bounded by the re-keyed takeover at ≤ (N+2)·ChainSyncInterval + G = 430 s at N = 12. The cloudtest FT tiers
  (190 / 380 s, #771) and the delivery idle-window call (owner call 4) ride on this number.
- **(22) The entry-forward cap (security parameter):** `h43ForwardEntries` = **4** — its own named constant, never the client's
  `entrySubmitBurst` (32): up to N−1 forwarders fire at ONE seat on every round entry, and the designee needs one entry to make
  a non-empty block.
- **(23) `R-H43-NULL-PROPOSAL` (scope):** PBFT §4.4's null request — the literature's answer to a workless leader — is a
  verifier-posture change inside the frozen era surface (`ValidateProposal` and `ValidateCommit` refuse an empty block); routed
  to **era 5, outside the RC**, with its own certification and activation height.
- **What this does NOT decide:** the certificate's byte ceiling and eliding `LockBlock` from a round-change
  (`R-H43-CERT-CARRIES-BLOCKS`, an era item); #380 (`R-380-LIVENESS-FACE`, its own gated item).

## D-RECOMPUTE-FREEZE — the trustless-recompute track is frozen; the ROADMAP is reordered to the simplicity spine; ten simplicity rules are standing

- **Status:** ✅ DIRECTED by the owner via the PE seat, 2026-09-08 — read as direction, not a consult. The note:
  `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/NOTE-to-silt-team-simplicity-and-roadmap-reorder-2026-09-08.md`.
- **The verdict the direction rests on.** The trust-plane CORE (I1–I5 consensus and the model-check, PoST bonds, blind
  tokens, the γ→1/N firewall, the economy — what existed at the 2026-08-19 audit) is ESSENTIAL complexity; nothing in
  the 08-19 KEEP ruling is reversed. The floor-box trustless changed-path recompute keystone and its apparatus (era-4/5
  churn, the R1.x ladder, the residual register as a growth medium, the structure rounds) is ACCIDENTAL complexity: it
  chose the research-frontier answer (stateless validation of unbounded state — Ethereum's unshipped stateless-client
  program) to a problem with three settled corners, which B8 says to buy. Evidence: `core/chain` non-test LOC 3,025 →
  13,093 in 20 days (the floor box alone 41 files, ~18K LOC); 109 of the last 255 commits bookkeeping against 26
  `feat(`; 275 distinct `R-*` names; 23 owner calls in one true-up; a sixth block format scheduled inside three weeks;
  the keystone's own code contradicting its ratification's O(payload) claim (the correction above); a classification
  the R1.6 design doc itself called decoration shipped as safe and driven wrong-accept 200/200 by the PE's PoC.
- **The decision.**
  1. **FREEZE the trustless-recompute track now:** no new floor-box classes, no Structure Round 1B, no more R1.x rungs,
     no structure rounds on the keystone, no era whose reason is "the recompute needs it" (era 5 is not pre-approved).
     Every existing gate stays GREEN and is not weakened. The state-root commitment (three v5 digest leaves) stays.
  2. **Reorder the ROADMAP to the note's §5 spine:** Boulder 0 (done) → Lane A consensus liveness (essential, first) →
     Boulder 2 the economy (promoted to the RC's substance) → Boulder 3 the freeze = the RC, its dependency on the
     recompute spine CUT (the RC's only floor-box requirement is owner call 2's cold auditor: never-Accept,
     unconditional loud stall, one DRIVEN suite) → Boulder 4 the external B8 pass on the frozen artifact → Boulder 5
     the operational floor (post-RC, ahead of the recompute) → Boulder 1 re-scoped to "the cheap validator", post-RC,
     toward `1.0.0`: a short design consult (PE + crypto-specialist + researcher) choosing among (a) bound the state,
     (b) optimistic + fraud proofs, (c) tiered validation — expected (a)+(c) — against B8; R1.8 the accept-flip only
     after the simpler design proves out, and only if still needed.
  3. **Freeze-manifest deltas** (the Researcher is told in one line; unchanged items are not re-certified): the digest
     set freezes at three leaves and that is the last format touch; the box witness/frame byte ceiling leaves the
     manifest for the design consult; the `proof.Unmarshal` decoder bound's deadline moves from the frozen flip to the
     stamp-raise train.
  4. **The residual register is pruned to ACTIONABLE rows** with an owner, a closer and a lane (25 rows from 125);
     held-in-tension and owner-disclosed residuals are folded into `docs/design/m0.md` §10.1; closed rows are deleted
     (verbatim in `/archive/roadmap-reorder-2026-09-08.md`). The 23-item owner-call block is archived; owner calls are
     not a standing section of the roadmap.
  5. **Ten simplicity rules are standing** in `silt/.claude/CLAUDE.md` (the B8 gate at every design decision; the PE's
     mandate widened to correctness AND simplicity; the bookkeeping ratio capped at 1:1; a residual must be actionable
     or it does not exist; at most five owner calls per true-up; no new era without a ratified non-recompute reason;
     a green gate with no demonstrated red is decoration — a RULE; structure rounds frozen on the keystone; one cert
     per consensus-rule change and only per consensus-rule change; the planner steps back to TENETS + VISION weekly).
     > **ANNOTATION 2026-09-10 — the location above is superseded; the rules are unchanged.** The ten rules now live
     > in [`build-process.md`](build-process.md), section "The ten simplicity rules", which is their canonical home;
     > `silt/.claude/CLAUDE.md` carries a pointer. **Rules 1–10 keep their original numbers**, so every `simplicity
     > rule N` citation in the repo still resolves to the same rule. Owner-directed: canon that lives in the agent
     > harness travels with the harness — a session run without the usual configuration silently unloads it, and
     > several of the ten were breached while nobody was reading them. The sentence above is left as written rather
     > than rewritten, per the standing practice that a correction is visible as a correction.
- **What this does NOT decide:** the pony's validation model itself — that is the design consult's output, owner-ratified;
  whether R1.8 ever lands. The A1 h43 fix (`D-CONSENSUS-ARMING`, `D-H43-WORKLESS-DESIGNEE`) is untouched — core, live, real.
- **Fact check recorded at the reorder:** the note's "close the cross-server double-redeem (fix `fcbab7e`)" is already
  closed on main by R0.4b (`2ad9bd5`, per-epoch issuer-key expiry; open-break gate PR #700); `fcbab7e` (the per-serial
  delivery guard, option b) is an unmerged alternative on a worktree branch and is not owed.

## D-DELEGATED-CALLS-2026-09-09 — four small calls the owner delegated, and what was decided under each

- **Status:** ✅ DECIDED 2026-09-09 by delegation. The owner delegated all four explicitly before an
  overnight run ("which of these owed calls may I decide myself" → all four), with the recommendation
  on each stated at the time. This entry records what was actually decided and why, so the delegation
  is auditable rather than remembered. Simplicity rule 5 caps owner calls at five per true-up; these
  four were retired here so the owner's queue holds only calls 10 and 12.

- **(1) The `#558` chain-store refusal surface — KEEP THE BROAD REFUSAL.** The shipped rule refuses to
  start on ANY structural-verification failure of `chain.cbor`, which is a larger surface than scope
  call S3's "torn tail". Narrowing it needs a classifier that separates "torn" from "corrupt in some
  other way", and that classifier is exactly the thing that can be wrong — a misclassification
  silently discards finalized history, which is the build-immutable the fix exists to hold. A refusal
  is recoverable by the operator (`-accept-chain-loss` preserves the original as
  `chain.cbor.rejected-<unix>`); a wrong classification is not. The broad surface stands as built
  (PR #772-era work, ruling `RULING-b8-558-chainstore-refuse-to-start-2026-09-07.md`).

- **(2) The `-quorum` default — DERIVE IT on the untrusted objective path.** Built here. Since #380
  (`D-CONSENSUS-ARMING` (20)) `-quorum` is not a validity term on that path, but it is still a floor
  on the proposer's gather, so the shipped literal 3 asked every peer of a four-anchor launch to
  attest and the swarm tolerated **f = 0** — against a published liveness bound stated at **f = 1**
  (`D-CONSENSUS-ARMING` (19), amended (21)). The field topology had already overridden it to 2, which
  is how the published number and the shipped default came apart unnoticed. `effectiveQuorum` now
  derives to `chain.ByzantineThreshold(<launch set>)` when the operator sets none, mirroring the
  bond-floor / TTL / Byzantine / operator-margin derivations — except this one derives DOWNWARD. It
  can only lower the ask: `gatherTwoPhase` gathers `max(caller floor, ConfigQuorum(), RequiredQuorum())`,
  so the derived Byzantine bar sits underneath it whatever the operator sets, and safety is untouched
  because validity reads `RequiredQuorum`, not this. An explicit `-quorum` always wins, including one
  raised above the bar. **The derivation applies ONLY where Byzantine sizing is on**, and the first cut
  of this change got that wrong: with `-byzantine-quorum=false` `RequiredQuorum` returns `cfg.Quorum`
  verbatim, so the local floor IS the validity bar there and deriving it downward would have made the
  node ACCEPT a 2-attestation block it previously refused, on three accept-side predicates. That is
  precisely the boundary #380's own ratification drew — *"the trusted opt-out (`-byzantine-quorum=false`)
  and legacy mode keep `cfg.Quorum` unchanged"* — and the regime is live in-tree
  (`integration/sybil/docker-compose.yml`, safe there only incidentally because it also passes an
  explicit `-quorum`). Caught by the blind PE before merge; the fix moves the change back INSIDE the
  existing certification's predicate, so no new research certification is owed. The two calls are also
  COUPLED, which the consult missed and the review caught: `MinObjectiveAnchors = 2` is what makes
  `effectiveQuorum`'s `anchorCount < 2` guard dead code today (`ByzantineThreshold(n) < 1` exactly when
  `n < 2`), so narrowing the anchor refusal later silently makes that guard load-bearing again — it is
  kept and commented rather than removed.
  `chain.ByzantineThreshold` is the single export of that arithmetic — a daemon
  that re-derived `f = ⌊(n-1)/3⌋` locally would be a duplicated consensus literal, and duplicated
  copies drift (the gather target went path-dependent exactly once already, on this same review).

- **(3) The single-anchor objective launch — REQUIRE TWO (`MinObjectiveAnchors = 2`).** Built here. At
  A = 1 every consensus gate on the launch path is self-satisfied: `bftThreshold(1) = 0`, so the sole
  anchor commits on its own signature with zero attestations; `requiredLaunchAnchors` is ⌊1/2⌋+1 = 1
  and `countAnchorSupport` credits the proposer itself, so the #402 anchor gate never bites; and
  `finalityQuorumActive` is true at 0 >= 0, so those zero-attestation blocks are treated as final.
  What holds at A = 1 is f = 0 plus "one qualified proposer never signs twice at a height" (#397) — a
  property of there being nobody else, not a quorum. Two anchors is the smallest set where the launch
  gate is a gate. This REFUSES a configuration that previously started; that is the intent, because it
  was starting into a posture where the gates were decoration. Every harness in the tree already uses
  three or four anchors, checked before the change.

- **(4) The CHANGELOG truncation lint — NOT YET; wait for a third occurrence.** A blank line inside an
  entry terminates the entry in `scripts/gen_changelog.py`, silently dropping everything after it from
  the published page while the source reads correctly. Two occurrences to date (the Lane C1 guard-store
  compatibility warning, caught by the blind PE; and one older entry in released history, left as
  history). The third-time rule fires at three, and Simplicity rule 3 caps bookkeeping against build —
  so the lesson is carried in the C4 and C3 build briefs instead, and the lint becomes mandatory on a
  third sighting. Recording the count here IS the tripwire: a future seat that hits it makes three.

## D-WORK-VISIBILITY — decentralization is graded in the HARNESS for the RC; the committed-ledger route is deferred to as late as possible before the cut

- **Status:** ✅ RATIFIED 2026-09-09 by the owner. Deliberation with all five alternatives priced:
  [`docs/thinking/2026-09-09-work-visibility-on-the-default-posture.md`](thinking/2026-09-09-work-visibility-on-the-default-posture.md).
  Follows directly from `D-UI-PRIVACY-FLAG`'s extension to the wire, ratified the same day.

- **The fact this decides.** `cmd/silt/daemon.go` welds `PublishWorkCounters` to `-privacy`, which is the
  compiled default, so on the shipped posture no node gossips `ServedBytes` or `RepairsDone`; the certified
  non-reporting exclusion then drops every peer, and both concentration series are structurally EMPTY.
  **silt can see who is PRESENT and never who does the WORK.** Five seats established this independently,
  four of them blind to each other: the PE and the red-team measured the reconstruction break that forced
  the wire containment, the Researcher gated the disclosure and refuted every coarsened form, the Economist
  flagged the consequence as dominating every number in its own advisory, and the Tester found every
  concentration gate returns INDETERMINATE by construction on that posture.

- **The decision.** Decentralization is graded **in the harness** — the deterministic tiers and the graded
  cloud runs, where the topology is known and every node is instrumented by the operator. No production
  concentration alarm is claimed, because none can fire on a default fleet. The R2.4 canary's
  concentration-based aborts are re-pointed at the harness accordingly; they must stop implying a
  production abort that cannot fire (`ROADMAP.md` row C6).

- **The committed-ledger route is DEFERRED to as late as possible before the RC is cut** — deliberately,
  and the reasoning is the owner's, recorded because it is a sequencing principle and not only a
  scheduling preference: computing work concentration from committed state instead of gossip is the
  strongest long-term answer (it removes the disclosure AND the self-reporting at once, and repairs the
  `m0.md` §7 seam-1 objection that a gossip numerator is Sybil-settable) — but it is **research
  territory**, it may spin, and it depends on establishing that the committed ledger even carries a
  per-node work quantity at the needed granularity. Owner, verbatim: *"I fear we will get back to research
  territory trap where we spin for a long time. I'm okay with that at the end, but not when so much other
  well defined, well researched work exists that we can execute on now."* So: execute the defined work
  first; open the committed-ledger question when the defined work is done and the cut is close.

- **What this does NOT defer.** The per-tier work totals build item stands and is NOT
  committed-ledger-dependent: the harness sets the counters directly, so a per-tier serve-byte total on
  `EconomySample` is exactly what a harness gate needs to assert the edge-majority tenet. Today nothing
  carries it, and the tier mix's `share` — a share of NODE COUNT — is the substitution trap, pinned:
  measured 0.9891 where the true byte share was 0.1998, so wiring it to the tenet floor would pass total
  serve capture.

- **What was refuted and must not be re-proposed as a compromise:** untying the publish flag from
  `-privacy` (it undoes the wire containment using the evidence that produced it), a coarsened band or
  rate (a band still yields a monotone step sequence under probing; a rate publishes the derivative the
  attack must compute), and opt-in telemetry (a self-selected sample is biased in the dangerous direction,
  since an operator running a capture would not opt in).

## D-GENESIS-MOVE-2 — the second content-addressing break is accepted; the genesis ROOT moves, not only the block hash

- **Status:** ✅ RATIFIED 2026-09-09 by the owner ("yes, regenerate"), on the Lane C5 pre-flip closers
  (PR #787). The first break was ruled the same way on 2026-09-07 under `D-R2.9-NODE-HALF-CALLS`
  ("accept the new genesis"; no live network exists, every development chain is wiped on upgrade), and
  that ruling explicitly did NOT cover this one — the second break class was filed forward, not
  pre-approved, so this is its own ratification rather than an extension of the first.

- **What moves, and why it is larger than 4′.** The 4′ break re-framed only the manifest, and the
  genesis ROOT does not cover the manifest — so 4′ moved the block hash alone. Computing a
  single-frame object's parity at the shard's TRUE length changes the object's own chunk IDs, so this
  time **all three move**: root `fce9eeeb…20d6` → `31768fb4…7dd1`, manifest chunk `5478750c…d107` →
  `f761f80b…fcf6`, block hash `f428d0a8…0951` → `e44344ea…72c0`. All three are re-pinned in
  `core/genesis/genesis_test.go`, so a future move stays an explicit, reviewed act.

- **The blast radius, measured rather than asserted.** Objects of `chunkSize − 9` bytes or fewer
  (262,135 B at the shipped default) re-address; two-or-more-frame objects are byte-identical, because
  they share a stripe and the tail stays padded. The manifest format is unchanged — no new field, no
  new CBOR key — and the erasure geometry is unchanged for every object; only a single-frame stripe's
  shard length moves. An existing store keeps working: nothing on the read path consults the default,
  so old objects fetch, audit and repair under their own committed geometry. A re-publish of the same
  bytes yields a new root, so dedup does not span the boundary.

- **What the owner accepted WITH it, each filed as its own register row rather than folded into this
  sentence:** sub-frame objects become **prepay-only** for durability (`R-SUBFRAME-PREPAY-ONLY`) —
  their bounty base is zero and serve revenue provably does not cover it, since revenue accumulates
  per `(server, requester, root)` lane, so traffic that skims 250 credits at a full shard skims 0 at a
  1 KB shard; and two privacy reductions (`R-SUBFRAME-SIZE-ORACLE`) — an exact byte-length oracle over
  the whole sub-frame class, and the loss of chunk size as an accidental salt against the confirmation
  attack. Both are threat-catalog updates, explicitly NOT mitigations, with a red-team pass owed.

- **The reason the change could not be declined on its own terms.** The proof-of-retrieval auditor
  sizes its challenge from the committed chunk size and demands it EXACTLY of every leaf — that
  exactness is red-team finding F4 — so a short shard under an unchanged committed size makes the
  auditor refuse every HONEST holder and every sub-frame object silently read as lost. The frame size
  actually used therefore travels in the EXISTING committed field: no format change, F4 intact.

## D-C2-IDLE-WINDOW-VALUE — the delivery idle window is 24m; and the route that reached it is under audit

- **Status:** ✅ RATIFIED — 2026-09-09 (owner: *"C2 idle window — RATIFY at 24m. Agreed. Cost is zero
  (deposit release at 30m35s binds first either way), precondition met, blind PE re-derived
  independently."*). This closes the value `D-TRUE-UP-CALLS-2026-09-07` reserved as *"the delivery
  idle-window VALUE (owed after A3)"*; the precondition was met by the graded run `97e3101-deep`.
- **The value.** `deliveryIdleDefault` = **24m**, with the start-up floor `deliveryIdleFloor` =
  `430 × 4/3` = 9m33.33s (`cmd/silt/numeraire.go`), so the daemon refuses a window that cannot
  survive the worst stall the model admits. Guaranteed survival is `0.75 × idle` because the
  last-settle stamp is floored into `idle/4` buckets (measured at 0.751× —
  `core/node/TestC2GuaranteedSurvivalIsThreeQuartersOfTheWindow`).
- **The cost is zero.** `CloseDeliverySession` releases the deposit at
  `max(close, maxAnchorEpoch + W + 1)`, `W = 4`, = 1834 s = 30m35s. The release epoch binds at both
  candidate windows, so the longer window adds no deposit-lock latency.
- **The owner's second question, recorded because the answer is owed to the freeze.** The magnitude
  did not move but the reasoning did: ratified call (4) was EPOCH-denominated against the 190 s modal
  tier; the shipped value is a DURATION against the 430 s re-keyed-takeover envelope. The owner asked
  whether the original derivation was wrong and, if so, *"what else was derived the same way — right
  answer by the wrong route once is fine; twice is a pattern I want found now, not at the freeze."*
  A blind PE audit of silt's derived parameters was commissioned against three faces: **F1** sized
  against a modal or published bound where the purpose requires the worst bound the model admits;
  **F2** a quantization/flooring step between the knob and the guarantee that the derivation did not
  carry through; **F3** resting on a coupling premise no driven test ever exercised. **It RETURNED
  2026-09-09:** `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-derivation-route-audit-pre-freeze-2026-09-09.md`.
  The exemplar failed all three faces, and the route's output — `4 × EpochBlocks × T_b` — contains a
  term (`T_b`) the guarantee does not, so **it emits a violating value for any block interval below
  17.92 s/height**; the measurement landed at 44 s, a 2.5× accident. The same route applied at an
  unreviewed site produced the graded harness's 90 s window (a 67.5 s guarantee). Eight parameters
  were cleared as documented and driven; four are named as suspects, and the one owner call is
  `SlashesBytesCap`.
- **A false claim in this row's own supporting code, corrected the same day.** `cmd/silt/numeraire.go`
  asserted that BOTH the 190 s tier and the 430 s bound were field-confirmed on `97e3101-deep`. The
  190 s tier is (row `6-fault-tolerance`). **The 430 s figure is NOT.** Row `10a-stall-drill` also
  computes 430, but from an unrelated formula — `(3+n_syb)*30 + 220` at `n_syb = 4`, the
  staggered-takeover ladder for DECLINING ATTESTERS — which merely collides numerically with
  `(N+2)*30 + G = 430` at `N = 12`. A coincident total is not a measurement: 10a never exercises a
  lost entry forward. Call (4)'s stated release precondition named the 190 s bound and IS met; it does
  not cover the number the shipped floor actually derives from, which rests on the ratified MODEL and
  is a conservative envelope, not a confirmed quantity. **This is the session-24 scar recurring**
  (*a claim about a gate is itself a claim; "this defeats X" is a measurement against X and you do not
  have it until you have run X*). Driving the lost-forward path in the field is owed at E5.
- **The "cost is ZERO" finding is `T_b`-conditional.** The deposit release epoch binds first at the
  measured cohort's block interval; at `T_b` = 20 s the idle window would bind instead. The
  ratification stands on the measured network; the conditionality is disclosed here rather than
  carried as an unqualified sentence.
- **What this does NOT decide:** the RELAY half of `R-REAPER-FORFEIT` (see `D-RELAY-EDGE-UNFIT`); the
  disposition of any parameter the audit names — each is its own call at its own tier.

## D-RC-POSTURE-2026-09-09 — relay settlement is unfit for the edge tier; unexercised lanes are labelled as such; and five governance calls

- **Status:** ✅ DECIDED — 2026-09-09 (owner, in one message; quoted per clause below).
- **(1) The relay lane is default-OFF at EVERY tier, and the docs say why.** The owner went further
  than the recommendation (which was "disable at edge tiers"): *"A lane needing 286–508 Mbit/s
  sustained to settle, that pays zero on a reap, is structurally a horse-and-above lane. That means
  relay revenue concentrates on the few while the pony tier — the tier the entire 10000/100/1 thesis
  depends on — gets a lane that punishes participation. That's not a bad default; it's an
  economic-recentralization vector."* So: **built, default-OFF at every tier, with the settlement math
  stated plainly in the operator docs, framed as "the relay settlement model is unfit for the edge
  tier" — not as a disabled feature.** A horse operator with the bandwidth opts in knowingly; a pony
  is never silently enrolled in a losing game. **As-built note:** `-accept-relay-payments` is already
  `false` (`cmd/silt/daemon.go:78`), so this call costs no default flip; what it buys is the
  DISCLOSURE and the routed design debt below.
- **(2) The real fix is design debt with an owner, not a v1 default.** *"While settlement is
  all-or-nothing at session end, relay stays horse-and-above permanently, and I want that tracked as a
  design debt with an owner."* A periodic relay sweep, or settling incrementally rather than at close,
  moves an economic rule and is therefore research-gated. Filed as a named Boulder-2-successor item;
  the owner affirmed it was correct NOT to build it inside C2.
- **(3) The RC labels every lane that has never run in the field — as a checklist rule, not a
  per-lane judgment.** *"Ship the RC stating the lane is built, sim-proven, never exercised on a real
  network, and grade it at E5 after the stamp raise… I don't want it deferred to D3 — omission decides
  these by default, and the default is always the over-claim. We've been here once already with M0."*
  The rule is generalized into [`release-checklist.md`](release-checklist.md) under honest labeling, so
  the next lane needs no fresh honesty call.
- **(4) The owner-call list carries only calls whose evidence exists.** *"Don't list a call again
  until its evidence exists; a list carrying not-yet-ready items trains me to skim, so a real call gets
  waved through."* `ROADMAP.md`'s owner-question block drops its GATED section: a call re-appears when
  its evidence lands, and is tracked meanwhile as ordinary lane work.
- **(5) Call 12 is decoupled — procurement starts now, ratification stays at D3.** *"The procurement
  has no code dependency and is the longest-lead item on the roadmap… Only the artifact they attack has
  to wait for the freeze."* Finding and contracting the external B8 seat begins immediately; the owner
  ratifies the engagement at D3. B8 is what lifts M0 from *built + internally-clean* to *held*, so it
  is on the CALENDAR critical path even though it is not on the code one.
- **(6) Delegation excludes the format surface.** The 2026-09-08 overnight grant is confirmed for this
  session with one carve-out: *"no format-surface change lands under delegation. D1 is a format train —
  every FORMAT item comes to me before it merges, even if it's green and reviewed."*
- **(7) One page in plain English before D3.** The owner reads, before signing the freeze act, a single
  page saying **what is frozen and what can never change without a new era** — the doors that close, not
  the 22-item manifest. Owed by D1, addressed to the owner, not to a seat.
- **(8) A scheduling call made unprompted: the sub-frame privacy surface is red-teamed BEFORE the
  economy flip.** *"Privacy is a Part-0 corner — an immutable, not a tunable — and the fix window
  narrows once the format freezes."* `R-SUBFRAME-SIZE-ORACLE` (an exact byte-length oracle plus the loss
  of chunk size as salt against the confirmation attack) moves ahead of C6. The repair-statistic
  normalization and node-level granularity WAIT: *"it's a measurement, it doesn't hold a corner."*
- **What this does NOT decide:** the relay successor's MECHANISM (periodic sweep vs incremental
  settlement — the Researcher certifies, the owner ratifies); which external party takes the B8
  engagement; the freeze act itself (D3).

## D-SLASHCAP-ROUTE — `SlashesBytesCap` keeps its value and loses its route; a consensus rule may not be a function of local config

- **Status:** ✅ DECIDED — 2026-09-09 (owner: *"The SlashesBytesCap call: CLOSE THE ROUTE. Not a
  re-ratification. The value stays 16 MiB. The route goes."*), on the pre-freeze derivation-route audit
  `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-derivation-route-audit-pre-freeze-2026-09-09.md`.
  Deliberation: [`thinking/2026-09-10-slashcap-route-close-design.md`](thinking/2026-09-10-slashcap-route-close-design.md).
- **The value is unchanged.** `chain.SlashesBytesCap` stays 16 MiB. Nothing about the constant moved.
- **The defect.** The cap is a consensus validity rule enforced on every validator
  (`core/chain/validate_v5_predicates.go:285`), and its invariant `cap ≥ 2 × (honest block) + overhead`
  was computed from the DEFAULTS of `-max-bondreg-bytes-per-block` and `-max-entry-bytes-per-block`.
  **⚠ THE INVARIANT ITSELF IS SUPERSEDED — see the SUPERSESSION note at the end of this entry.**
  Both flags are **proposer-side only** (every non-test read: `core/node/chainrole.go:890`,
  `core/node/entrypool.go:116`) and documented `0 = unbounded`. So an operator could raise its own
  budget past ~7.9 MiB and make its OWN equivocation unprovable — the evidence pair exceeds the cap,
  the cap rejects it before `CheckEquivocation` runs, and the double-signer **keeps its seat**.
- **The owner's three reasons, in his order of weight.** (1) *"It's the same class as #380, which we
  just paid for.* `RequiredQuorum()` *read the local* `cfg.Quorum` *and produced an I1 divergence.
  A consensus rule must be a function of the chain, never of local config. Two instances of one class
  inside a week isn't a coincidence, it's an unguarded seam."* (2) The accountability face is the real
  severity: *"that's slashing defeated by making the evidence too big. Accountability is a Part-0
  corner."* (3) Timing is decisive and the tier correction is accepted — this is a VALIDITY rule
  (freeze manifest item 10), so the deadline is the STAMP RAISE, not the freeze; but *"raising a cap
  later is a widening rule change, outside the narrowing exemption. 'Fix it cheaply later' isn't on
  the menu. It's now, or it's a coordinated fleet fork."*
- **The fix.** `core/node.CheckSlashEvidenceHeadroom(cfg)` expresses the invariant on the values IN
  FORCE; `cmd/silt` refuses to start on a non-nil return. `0` and negative budgets are refused (the
  proposer's guard is `budget > 0`, so both read as unbounded), rather than clamped — clamping would
  silently re-interpret an explicit operator request. Driven by G-SLASHCAP-1..4, ablation red first.
  No consensus rule, block format or published claim changes; this binds a configuration to an
  already-ratified derivation.
- **Scope, so it is not over-read.** This closes the CONFIGURATION route only. The cap's disclosed
  SECOND FACE — a ≥⅓ coalition making every evidence pair over-cap with its own valid renewals, so
  accountable safety degrades to plain safety for fat coalitions — is UNTOUCHED, and no admissible cap
  value closes it; only the v5 two-level block hash (d-3, a FORMAT item in the D1 train) removes it.
- **CORRECTION, 2026-09-10, by the blind PE review of the close itself — the stronger claim is
  REFUTED and the owner must see this.** The first version of this entry and of the code comment said
  the derivation was now "bound by construction". **It is not, and it cannot be.** `Equivocation`
  carries two FULL `Block`s (`core/chain/equivocation.go:26-27`) and a `Block` carries its own
  `Slashes` field (`core/chain/chain.go:518`), bounded only by the cap being defended. So
  `cap ≥ 2 × body + overhead` with `body ⊇ Slashes ≤ cap` **has no positive solution at any cap** — a
  fixed point, not a tuning error. Measured on signature-valid fixtures at the SHIPPED defaults, with
  **no coalition and no misconfiguration**: a block committing two ordinary 4.14 MiB proofs is VALID
  (8.28 MiB of `Slashes` under the 16 MiB cap), and a LEGITIMATE proof about that block is
  **17,373,935 B — 596 KB over cap**. The equivocator keeps its seat.
- **So the cap has a THIRD face: `R-NESTED-EVIDENCE-OVERCAP`.** The two faces already disclosed on the
  constant need a ≥⅓ coalition or (as of this entry) a misconfiguration; this one needs **neither** and
  is reachable on the honest path at shipped defaults. What `D-SLASHCAP-ROUTE` buys is therefore
  stated exactly: it closes the OPERATOR MISCONFIGURATION route — a validator can no longer make its
  own equivocation unprovable by editing a local flag — and it makes the configurable half of the
  derivation true instead of assumed. It is **necessary, not sufficient**. Whether a validity rule
  bounding the encoded block body closes the rest (it would bind PEERS, which no start-up check can),
  or whether fixed-size evidence (d-3) is the only close, is **RESEARCH-GATED and in flight**; the
  verdict may partly refute the R0.6 value certification, and the owner ratifies that.
- **What this does NOT decide:** the single-reg overflow (`R-BONDREG-SINGLE-OVERSIZE`) — silt has no
  per-reg byte cap and `core/node/chainrole.go:902` embeds the first fresh reg unconditionally, so one
  oversized registration can still exceed the configured budget. That needs a validity rule of its own
  and is filed, not folded in. Nor the nested-evidence face above. Nor **the one call the PE routed to
  the owner: retiring the documented `0 = unbounded` posture on two shipped flags pre-RC.** The blast
  radius is empty in-tree (verified: no script, CI workflow, integration topology, cloudtest launcher
  or deploy file sets either flag) and the PE recommends taking it; it ships in this change and the
  owner may reverse it.
- **⚠ SUPERSESSION — the `cap ≥ 2 × body + overhead` INVARIANT is retired, not merely qualified
  (2026-09-10, at the reviewing engineer's own instruction).** The engineer whose ruling prescribed
  that invariant asked that this entry say so in as many words, so that no future reader finds a
  ruling on record that reads as adequate:

  > *"The nest-gate finding supersedes my invariant. My `2 × body + overhead` was necessary, not
  > sufficient."*

  The invariant is not a threshold that was set too low. It **has no positive solution at any cap**,
  because `body ⊇ Slashes ≤ cap` makes it self-referential — a fixed point, not a tuning error
  (`R-NESTED-EVIDENCE-OVERCAP`, measured as `R-NEST-GATE`). Any document, comment or review that
  states the invariant as the cap's sufficient condition is wrong as of this date. The invariant's
  surviving role is narrow and should be cited only as such: it bounds the CONFIGURABLE half of the
  derivation, which is what `CheckSlashEvidenceHeadroom` now enforces. **The sufficient close is the
  v5 signature-preimage change** (`D-PREIMAGE-BUY-2026-09-10`, owner call 1), which makes evidence
  `O(1)` and removes the self-reference entirely rather than bounding its symptom.

---

## D-PREIMAGE-BUY-2026-09-10 — the v5 signature preimage carries Height; the `SlashesBytesCap` disclosure sentence is unratified and replaced; state growth is bounded by standing; D3 stays unsigned

- **Status:** ✅ FOUR OWNER CALLS DECIDED — 2026-09-10, answering the four open questions in
  `ROADMAP.md` "▶ OPEN QUESTIONS FOR THE OWNER". Basis: the nested-evidence certification
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/SLASHCAP-NESTED-EVIDENCE-FIXED-POINT-RESEARCH-CERTIFICATION-2026-09-10.md`
  and the driven measurement `R-NEST-GATE` (#795). Companion: `D-SLASHCAP-ROUTE`.

### Call 1 — BUY the signature-preimage change at D1, conditional on its delta cert

**BOUGHT.** `consensusSigBytes` (`core/chain/chain.go:1138`) omits **Height** from the signed
preimage; the v5 preimage carries `(height, round, phase)`, after CometBFT's `CanonicalVote`.
Evidence then becomes `O(1)` — **~251 B DERIVED, not the ~200 B first cited; unmeasured until gate G-PRE-9** — instead of two full `Block` bodies.

**The decisive argument is the scar, not the optimization — and it is the owner's, not the
write-up's.** In his words:

> *"`consensusSigBytes` omits Height from the signed preimage with the justification 'the height
> rides inside the hash.' CometBFT's `CanonicalVote` signs (height, round, step). That is the #397
> watermark scar, second occurrence — `build-process.md` rule 6, from a PE ruling on #432: 'cite the
> analogue means adopt the SCHEMA and know why each field exists, never just the purpose… every
> field you drop is a claim you can prove you don't need it.' The #397 watermark copied
> `priv_validator_state`'s purpose and dropped its (height, round, step) schema, and the dropped
> field was the liveness. Here we dropped the same field from the same schema family, and the
> dropped field is why evidence must carry two full blocks. The claim 'we don't need height' is
> precisely the claim that failed. So this isn't a clever optimization. It's paying back a known
> scar at the one moment it's still cheap."*

The citation is verified at source: `docs/build-process.md:204-213` states rule 6 and the #397
`(height, ROUND, step)` drop in exactly those terms. **This is the second occurrence of that scar.**

**The corroborating reasons, in the owner's order:** it kills the nesting attack at the root rather
than bounding a symptom (a legitimate proof is ~251 B derived, regardless of how bloated the target block is, so the 687 B × 24,456 route dies completely); it names its settled corner, so the B8 gate is
satisfied; and its blast radius is smaller than (d-3)'s. It is FORMAT + WIDENING, so it rides the D1
train. **The "or it costs an era" clause that originally accompanied this is WITHDRAWN**
(`D-FREEZE-REPRICE-2026-09-10`): the freeze deadline is SOFT pre-launch, and this change stands on
the #397 schema argument by itself — which is the leg that was always load-bearing.

**Three conditions, all binding:**

1. **The delta cert lands.** *"If it refutes, come back to me, don't route around it."* Commissioned
   2026-09-10; verdict filed to
   `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/V5-SIGNATURE-PREIMAGE-HEIGHT-DELTA-RESEARCH-CERTIFICATION-2026-09-10.md`.
2. **It COMPLEMENTS (d-3); it does not replace it.** (d-3) `AnswerDigest` still ships. The delta
   cert is asked to confirm (d-3) retains independent work once evidence is `O(1)`, and to say so
   plainly if it does not — the owner wants to know if he is buying two things where one would do.
3. **Do NOT shrink `SlashesBytesCap` in the same breath.** *"Once evidence is O(1), 16 MiB is
   absurd, but shrinking a cap is a narrowing change and stays cheap after the freeze. Land the
   preimage change, re-derive the cap afterward."* This is a standing prohibition on the D1 train:
   no PR may move the constant and the preimage together.

**⚠ A CORRECTION TO THE CALL'S OWN BLAST-RADIUS SENTENCE, found while acting on it (2026-09-10).**
The roadmap sold this as *"eight non-test `verifyAtt` call sites, all with the block in scope."*
Verified at source, there are **nine**: `core/chain/validate_v5_quorum.go:243,274` ·
`core/chain/carrier.go:135,233` · `core/chain/chain.go:3095,3126,3487,3601` ·
`core/chain/equivocation.go:121`. `carrier.go:135` is the only one whose signing height is DERIVED:
it sits in `validateCarrier(b *Block)` but verifies `PhasePrecommit` attestations over `b.Prev` — the
**parent's** hash — so the height to sign is `b.Height - 1`. The delta certification made the same
error one better: it *listed* nine and *called* them eight in the same sentence whose parenthetical
conceded the exception.

**AND A CORRECTION TO THAT CORRECTION — the delta cert REFUTED my reading of it (2026-09-10).** I
filed `carrier.go:135` as the #397 off-by-one class. **It is not.** The derived height is SOUND, on
an invariant named **P1 PARENT BINDING** (`core/chain/validate_v5.go:211-215`, `chain.go:3446-3450`;
`HeadRef.NextHeight = parent.Height+1` at `stateview_v5.go:83-86`), and the derivation is strictly
**narrowing** — a mis-declared height makes genuine entries fail, never forged ones pass.
`chain.go:3601` is likewise not a derivation problem: the height is read and is 0.

**The #397 class is one line over, and it is worse.** See `D-PREIMAGE-CERT-2026-09-10`.

### Call 2 — UNRATIFY and replace the `SlashesBytesCap` disclosure sentence

**UNRATIFIED.** The owner withdrew his own ratified words. The replacement, ratified 2026-09-10:

> **(d-3) shrinks the face roughly 40× but does not remove it; what removes it is the v5
> signature-preimage change, which makes evidence O(1).**

> ⚠ **SUPERSEDED 2026-09-10 — the replacement sentence quoted immediately above is itself
> REFUTED, and it is left standing so the correction is visible as a correction.** The
> *"roughly 40×"* figure is **ZERO**: `SlashesBytesCap` is enforced against
> `SlashesEncodedSize`, which marshals real encoded `[]Equivocation` bytes, and (d-3) reduces
> the HASH PREIMAGE, which that function never reads. The refutation and the sentence that
> now stands — carrying **no shrink figure, deliberately**, because three shrink figures have
> now been wrong — are recorded in place under `D-F2-EVIDENCE-RECOMPUTE` above, as
> `D-D3-CERT-REFUTATION-2026-09-10`. Read that, not this. *(Pointed by decision ID rather than
> by line number on purpose: an annotation inserted above a coordinate moves it, which is the
> defect `scar:cited-source-coordinate-decayed-2026-09-10` was widened to catch.)*

**The 16 MiB VALUE does not move.** Only the disclosure sentence moved. The unratification is
recorded **in place** at the original ratified text (this file, the R0.6 value entry) rather than by
rewriting it, and the owner ratified that handling as standing practice: *"the correction should be
visible as a correction, not laundered into the original."* Apply it to all ratified text.

**Tracked, not merely annotated (at the reviewing engineer's instruction).** The certification's §8
enumerates five prior sentences that fall (C-1…C-5). C-5 is the owner sentence replaced above. The
remaining four are certification sentences whose **claims now have no support**, and annotating them
is not the same as re-deriving what they asserted. They are tracked as one actionable register row,
`R-CERT-REDERIVE` (`ROADMAP.md`), with a per-sentence closer:

| # | The sentence that fell | What must be RE-DERIVED, not just annotated |
|---|---|---|
| C-1 | I5-cross-height cert §7: *"two constraints bracket it… liveness floor"* | The bracket is EMPTY. 16 MiB was chosen against a **restricted** floor — largest legitimate pair among blocks whose `Slashes` is empty — and that restriction was never stated. Re-derive the floor the value actually satisfies, and state the restriction. |
| C-2 | R0.6 delta V-2: *"so an honest-shaped pair is always slashable"* | The property is gone; the inequality survives only as arithmetic about the **non-`Slashes`** body. Scope every citation to that. Done for the ledger in `D-SLASHCAP-ROUTE`'s SUPERSESSION note; `core/chain/chain.go:412-414` already carries corrected wording. |
| C-3 | R0.6 delta §5.2: *"(d-3) → two headers + digests, ~KB fixed; completeness face gone"* | Falls. Re-derive what (d-3) actually delivers — and do it on the **header-only** proof route, since the measurement refuted the reg-laden route both the PE table and the cert modelled. |
| C-4 | Freeze-manifest cert §4.3: *"what it closes, all three at once"* | Falls in its first third. `R-LATE-REVEAL` and `R-CARRIER-PRUNED-HASH` close as stated; `R-BIG-EVIDENCE-UNSLASHABLE` does not. **This one is inside the FREEZE MANIFEST, so it is D1 content, not bookkeeping.** Manifest item 3 stays BOUGHT on its surviving two-thirds; the spec repair (the v5 preimage carries a single `SlashesDigest`, never a `Slashes'` copy) is a FORMAT question the owner takes individually. |

### Call 3 — do not accept unbounded state growth; bound it by STANDING, and decide it after call 1

**REFUSED as stated; the disposition is bounded rather than accepted.** `apply()` writes
`slashed[culprit] = true` unconditionally and the recompute mints a permanent SMT leaf per culprit,
so a 16 MiB block of proofs implies ~23,800 permanent leaves. The owner:

> *"~23,800 permanent SMT leaves from one block is a state-growth DoS, and the nest measurement
> showed how it's reached: throwaway keys about never-committed blocks. An attacker mints permanent
> committed state for identities that never mattered."*

**The disposition he wants:** *a slash mints no permanent leaf for an identity that had no bonded
standing at the height in question.* Slashing an identity with no standing achieves nothing
legitimate, so nothing real is lost.

**The nuance is load-bearing and the owner named it himself:** the predicate **cannot** be
*"currently bonded"* — late-reveal evidence about a since-lapsed bond is legitimate and must record.
It has to be *"was bonded at that height,"* which the chain can check. **The exact predicate is
routed to research.**

**Sequenced AFTER call 1, deliberately:** *"with O(1) evidence the cap can shrink as a narrowing
change, which bounds this from the other side. Deciding 3 in isolation prices it against a world
call 1 is about to change."* So the research question is NOT commissioned until the call-1 delta
cert returns.

### Call 4 — D3 is not signed today; two conditions before the owner signs

**NOTHING TODAY.** The freeze act waits. Two conditions, both new:

1. The plain-English one-pager (`docs/era4-freeze-what-closes.md`) is re-checked against the
   manifest's **final** content — already owed under `D-RC-POSTURE-2026-09-09` (7).
2. **D3 cannot be signed until the preimage change has either LANDED or been explicitly DECLINED
   with the consequence recorded in the entry.** *"Calls 1–3 all change what D1 contains, and the
   freeze is the door that closes on all of them. I'm not signing a freeze whose contents were still
   moving the day before."*

Condition 2 is a hard gate on row D3: a decline is not silence — it is an entry in this ledger
naming what silt keeps instead.

---

## D-PREIMAGE-CERT-2026-09-10 — the preimage delta cert returns GATED; the consensus quantity rule enters the canon; `MinBond` is a defect to fix, bound to the chain

- **Status:** ✅ CERT RECEIVED + ✅ ONE OWNER RULING — 2026-09-10. Certification:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/V5-SIGNATURE-PREIMAGE-HEIGHT-DELTA-RESEARCH-CERTIFICATION-2026-09-10.md`.
  Follows `D-PREIMAGE-BUY-2026-09-10`. **Verdict: GATED — the DIRECTION is CERTIFIED, the build is
  gated on five conditions, two of which correct the change as the owner bought it.**
- **The BUY stands.** The owner's scar argument is independent of everything the cert found and is
  verified at `docs/build-process.md:204-213`. Nothing below reopens call 1.

### The canon rule — what three instances have been teaching one at a time

**RATIFIED 2026-09-10 by the owner, and written into `docs/build-process.md` as rule 8** (his
direction: it *"belongs in the canon before the config-in-consensus lint gets written — the lint
should check the principle, not just pattern-match the three cases found"*):

> **A consensus quantity must be a function of the chain.** When its invariant is **locally
> checkable**, bind it with a refuse-to-start (`SlashesBytesCap`). When it requires **distributed
> agreement**, bind it to **committed or genesis-covered state** (`MinBond`). **A local assertion
> cannot enforce a distributed agreement.**

The three instances: **#380** `RequiredQuorum()` read `cfg.Quorum` on the objective path (fixed);
**`SlashesBytesCap`** derived its invariant from two proposer-side flag defaults (`D-SLASHCAP-ROUTE`,
refuse-to-start); **`MinBond`** below.

### `MinBond` — a genuine defect, and the `SlashesBytesCap` treatment does NOT transfer

**Measured, not derived.** A driven divergence probe over 17 `Config` fields in both quorum regimes:
`Quorum` is CLOSED on the objective path (the #380 fix holds — ablating it reddens the probe, 2 → 3
diverging fields), but **`MinBond` and `MinBondBytes` still change the validity verdict there.** Two
honest replicas on one network with different `-min-bond` disagree about the validity of the same
block. The owner confirmed three v5 Reject paths at source:
`core/chain/validate_v5_predicates.go:246-249` (proposer qualification),
`core/chain/validate_v5_quorum.go:222` (bond-reg admission, with `MinBondBytes` at `:225`) and
`:522` (the qualification filter). **That is I1, and it is the #380 class, third instance.**

Root cause: `Params` embeds `Config` verbatim (`core/chain/stateview_live_v5.go:26`), and every
`Config` field is a command-line flag (`cmd/silt/daemon.go:905-916`). The v5 floor box is therefore
**not** clean-by-construction. silt already names the class in prose — *"consensus-critical genesis
config"* (`core/chain/chain.go:190`, `:253`, listing `MinBond`/`Anchors`/`EpochBlocks`/
`RegGateActivationHeight`) — but it exists ONLY in scattered doc comments: no enumeration, no
enforcement. Membership is a human remembering to write the sentence.

**The owner's ruling — the disposition is his, the mechanism is ours:**

> *"I'm ruling it's a defect to fix, not a requirement to document harder, and that the fix must bind
> it to the chain rather than to a local assertion. Which of the three shapes, and how, is yours plus
> research."*

**Why the `SlashesBytesCap` fix does not transfer, in his words** — this is the load-bearing part:
that cap's invariant is `2 × (budgets) ≤ cap`, **pure local arithmetic**, so a node can check its own
config and refuse to start. **`MinBond` divergence is not locally observable.** No node can tell from
its own config that a peer set a different value, so *"a refuse-to-start assertion has nothing to
assert against… 'the same treatment' would produce a gate that looks green and enforces nothing — the
decoration failure, in a new place."* `MinBond` lives in `Config` (`core/chain/chain.go:126`) and the
genesis hash covers block content, not `Config`, so the coordination requirement is today
**documented and structurally unbindable**.

**Three candidate shapes, to be PRICED by research rather than picked blind:**

1. Commit the genesis-config family (`MinBond`, `MinBondBytes`, `Anchors`, `EpochBlocks`,
   `RegGateActivationHeight`) into something the **genesis hash covers**. Divergence then becomes
   impossible to *join* with rather than fatal at validation — detected at handshake, *"which is the
   right failure surface."*
2. **Derive `MinBond` from committed state**, if the maintenance claim already implies it.
3. A **network-identity digest** over the consensus-critical config, checked at peering.

### `MinBond` BLOCKS D3 — the freeze locks the repair, not the bug

The owner rejected the builder's "`MinBond` isn't a format item, so the freeze doesn't close the
door" argument, on the fact that makes it wrong:

> *"'MinBond isn't a format item' is true of MinBond as it exists today — a local flag. But every
> viable fix moves the value into committed or genesis-covered state. That's a format change. The
> freeze doesn't close the door on the bug; it closes the door on the repair. After D3, binding
> MinBond costs an era, exactly like the preimage change."*

**⚠ THE OWNER CORRECTED THIS REASONING THE SAME DAY — `D-FREEZE-REPRICE-2026-09-10`.** The freeze
deadline is SOFT pre-launch, so the freeze does NOT lock the repair. **The conclusion is unchanged and
the honest reason is:** D3 gates the B8 engagement, and silt does not spend the longest-lead item on
the roadmap attacking an artifact with a known I1 divergence. The paragraph above is left in place,
per the standing practice on superseded reasoning.

And the severity argument runs the **same** direction, not the opposite one: an I1 divergence is
worse in kind than a format mistake — *"a format mistake costs an era, an I1 divergence costs a fork,
and forks are the thing the whole trust plane exists to prevent"* — so the item with the worse
failure mode is also the one whose fix the freeze locks.

**Proportionality, so this does not become an emergency:** exploitability is LOW. Divergence requires
someone to actually set a different value, and the flag is documented as coordination-required. This
is a **latent structural hole, not a live break** — the same posture as the floor-box wrong-accepts.
**So it is D1 SCOPE: it rides the format train with the preimage change.** If the delta cert on the
chosen shape says it is more than D1 can absorb, that comes back to the owner rather than shipping
the freeze around it.

### The cert's three findings that change the build

1. **`carrier.go:135`'s derived height is SOUND — and the #397 class is one line over.** The
   derivation rests on **P1 PARENT BINDING** and is strictly narrowing. But `era4Active` is `>=`
   (`core/chain/chain.go:3980-3985`, verified), so `H_era4` is the first v5 height and **its parent
   is v4**. `validateCarrier` already keys on `b.Version` (`core/chain/carrier.go:117`, verified).
   If the new v5 phase dispatch is likewise keyed on `b.Version`, then at exactly `H_era4`
   `HeadCarrier` (`carrier.go:233`) filters with the **v4 parent** in scope while `validateCarrier`
   verifies with the **v5 child** in scope, every entry fails, and the honest proposer rejects its
   own block (`core/node/chainrole.go:1076` → `:1090`) — a **permanent liveness wedge at `H_era4`**.
   **Fix: dispatch on the attestation's own `Phase`**, the mechanism `verifyAtt` already uses for
   era-1 vs era-2 (`core/chain/chain.go:951-958`), which is wire-additive and is what the #558
   one-dispatcher pin demands. Gate **G-PRE-1** must be seen RED first.
2. **A second seam nobody routed — the DURABLE `ports.SignMark.Phase`.** It is fsynced to disk
   (`ports/ports.go:322-353`) and `slotCompare` compares it **numerically**
   (`core/node/chainrole.go:87-91`, verified). A mark written `(H, r, PhasePrecommit=2)` probed with
   a v5 constant of 3 compares `3 > 2` → not blocked → **the node proposes a different block at a
   height it already precommitted: a self-manufactured double-sign produced by the upgrade itself.**
   The bitter joke is that `SignMark` IS the #397 artifact — Tendermint's `priv_validator_state`.
   **Fix, no migration: the watermark records the STEP, the attestation records the ERA-FORM.** Gate
   **G-PRE-3** must be seen RED first.
3. **The chain id is not optional; the drop-claim is disprovable.** `consensusSigDomain` is constant
   across every silt network — a domain tag separates message *kinds*, not *networks* — and the
   genesis moved 2026-09-07, so two silt networks exist. One key honestly precommitting at the same
   `(h, r)` on both yields a **valid** equivocation proof on either: `validateSlashes`
   (`core/chain/chain.go:2200-2213`) has no chain-membership check and `apply()` evicts permanently.
   The face exists TODAY with bodies; route (C) drops its price from two megabyte blocks to ~251 B.
   Ethereum's `compute_fork_data_root` names the purpose verbatim.
   **Certified layout — 99 bytes, fixed-width, no length prefix** (a fixed-width concatenation is
   injective by construction; CometBFT length-prefixes only because protobuf is variable-length):
   `"silt/consensus/v5\x00"(18) ‖ genesisHash(32) ‖ phase(1) ‖ height(8 LE) ‖ round(8 LE) ‖ hash(32)`.

**Also corrected: "~200 bytes" is wrong — ~251 B derived** (raw field material alone is 241 B). It
had propagated into this ledger and into `ROADMAP.md`; every site is now annotated. **It is DERIVED
and must be MEASURED before any doc cites it as fact** (gate G-PRE-9). It is not load-bearing for the
verdict; it IS load-bearing for the junk-leaf re-pricing in owner call B.

### THREE NEW OWNER CALLS — A, B, C

**A. The chain id in the preimage is a SCOPE ADD relative to what was bought. The Researcher
recommends BUY; the builder does NOT decide it** (a scope change is a veto-gate item). Cost is one
class-3 (box-owned) `HeadRef` field: no witnessed leaf, no digest, no SMT tag, so **the freeze
read-set does not move**. The tension the owner must weigh: it adds a concept to the floor box, which
**simplicity rule 8 freezes**.

**B. `R-SLASH-CULPRIT-ADMISSIBILITY` — NEW, OPEN, and it re-prices owner call 3.** Route (C) makes
junk-proof state growth **~2.8× cheaper**: ~23,857 → ~66,842 permanent SMT leaves per 16 MiB block,
from any 32-byte key, with no bond. **This is exactly the pricing the owner sequenced call 3 to wait
for** — and it runs against the expectation: `O(1)` evidence bounds the face from the cap side while
making each junk leaf cheaper. CometBFT and Ethereum both gate evidence on set membership; silt gates
on nothing. **Do not bolt a screen on blind** — a naive `bonded > 0` makes an unbonded double-signer
unslashable, which is the same trap the owner already named in call 3 ("was bonded at that height,"
never "currently bonded"). Measure first, then design.

**C. (d-3) now rests on ONE residual, not three — and the owner should hear it plainly.**
`R-BIG-EVIDENCE-UNSLASHABLE` is route (C)'s job; `R-LATE-REVEAL` is double-covered;
**`R-CARRIER-PRUNED-HASH`'s structural half is (d-3)'s SOLE independent work, and route (C) does
nothing for it.** So the answer to call 1's condition (b) is: he is **not** buying two things where
one would do — he is buying a second thing **worth less than the manifest told him**. The Researcher
recommends shipping both. This is the C-3/C-4 re-derivation of `R-CERT-REDERIVE` landing.

---

## D-FREEZE-REPRICE-2026-09-10 — the freeze deadline is SOFT; "or it costs an era" is withdrawn as a reason to buy

- **Status:** ✅ OWNER CORRECTION — 2026-09-10, correcting his own three rulings of the same day
  (`D-PREIMAGE-CERT-2026-09-10`) and the framing of `D-PREIMAGE-BUY-2026-09-10`. **This entry
  governs every open D1 item.**

### The correction

The owner justified several buys with *"it's format and widening, so it rides D1 or it costs an
era."* **Pre-launch that is wrong.** With no live network, genesis and format can both move — silt
already moved genesis once (`D-GENESIS-MOVE-2`).

> *"The freeze's real currency is re-running graded field runs and delaying the external B8
> engagement — days and cloud spend, not permanence. That deadline is real and it's soft."*

**Price the freeze in re-runs and calendar, not in permanence** — *"which is what it actually costs,
and which is a number you can weigh instead of a threat you can't argue with."*

### What the freeze still means, so this is not read as licence to churn

It remains **the forcing function and the B8 gate.** At some point the format stops moving or silt
never ships, and the external engagement cannot meaningfully attack a moving artifact. **Still
freeze, still soon, still the gate on the calendar-critical item.** What is withdrawn is
*permanence* as the reason.

### The governance lesson — the owner's, in his words

> *"Two sessions ago I told you the trust plane was making a complicated thing more complicated, and
> gave you ten rules to stop it. Then I spent two sessions supplying 'now or it costs an era' as a
> reason to buy things. Artificial deadline pressure is one of the most reliable drivers of
> over-buying — it's how a freeze train turns into a scope magnet. I supplied the pressure. That's
> mine."*

**The corrected posture: with the deadline soft, buy LESS and BETTER, not more and faster.** Every
item currently justified partly by *"or it costs an era"* is re-read with that clause **deleted**,
and anything that does not survive on merit **comes out of the train**. This is the operative rule
for the rest of D1, and it composes with simplicity rules 1 and 8.

### The three calls, re-priced

**A — chain id in the v5 preimage: BUY, unchanged, on the leg that was always load-bearing.** The
argument had two legs and only one mattered. **The deadline leg is void.** The **schema leg stands
entirely on its own:** silt named CanonicalVote as its settled corner, `ChainID` is in it, and
dropping it **repeats the #397 scar inside the very fix meant to pay it back** (`build-process.md`
rule 6). And the I5 break is **live on the two test networks today**. Buy it because it is correct.
If genesis can move freely, this gets *easier*, not harder.

**B — the FORMAT-vs-VALIDITY pricing ask is WITHDRAWN.** It was asked because it set a deadline; it
barely does now. **Do not spend a session on it.** What remains is unchanged and small: make the
standing predicate correct — *"was bonded at that height,"* with late-reveal preserved — and let the
planner sequence it against the last graded run. `R-SLASH-CULPRIT-ADMISSIBILITY` keeps its row; it
loses its pricing sub-task.

**C — (d-3) genuinely re-opens, and is RE-JUSTIFIED BELOW on correctness alone.** The owner:
*"I told you 'buy it: it's format, and declining costs an era.' That reason is void. What's left is
a purchase whose advertised value has been re-priced downward twice — first 'removes the face' →
shrinks it 40×, then 'two things' → one thing worth a third. Justify the remaining third on its own
merit, or drop it. Do not buy it because a train is leaving."*

### (d-3) re-justified on merit — RECOMMEND BUY, as a correctness fix, with the value restated

**The advertised value is not the real value.** (d-3) was sold on evidence size, and that leg fell
twice. Read at source, its merit is a different and larger thing: **it retires `Pruned` — the
declared-identity field — for v5.**

**What is actually broken today**, from `Block.Hash()`'s own comment (`core/chain/chain.go:756-765`,
read at source):

> *"THE PRUNED FIELD IS A LINKAGE TOKEN, NOT A CONTENT COMMITMENT… The attack is not forging
> `Pruned`. It is KEEPING `Pruned` and the real signatures while mutating the body: `Hash()` returns
> `b.Pruned` unchanged, so every signature still verifies. `Prune()` drops only `BondReg.Answer` — it
> KEEPS `LastCommit`, `StateRoot`, `Entries`, `Revocations`, `Slashes` and the light `BondReg`
> fields, and **none of them is covered by `Hash()` once the block is pruned.**"*

So **once a block is pruned, none of its retained content is hash-covered.** Today's defences are
(i) the recompute chain to the first non-pruned descendant, (ii) `trustFloor`, and (iii) a
**discipline** — *"NO CONSENSUS DECISION MAY DEPEND ON RE-READING THE BODY OF A PRUNED BLOCK"* —
whose *proof* is the owed `R-CARRIER-PRUNED-HASH`. The same comment notes this is **"the third time
this comment has shipped a false safety claim."**

**What (d-3) changes:** `Prune()` for v5 drops `Answer`, keeps `AnswerDigest`, and **does not set
`Pruned`** — a pruned v5 block **recomputes its own hash from what it retains**. `Pruned` is retired
for v5. That converts a discipline plus an owed proof into a **structural property**, and it is the
root-cause fix for the class R0.6 patched by *refusing* pruned evidence (a narrowing workaround that
costs `R-LATE-REVEAL`).

**What it protects, stated plainly, as the owner asked:** the integrity of a pruned block's retained
body — `LastCommit`, `StateRoot`, `Entries`, `Revocations`, `Slashes` — which today is covered by
nothing.

**The honest cost, not soft-pedalled.** It is not a one-clause change: `bodyHash` becomes
version-dependent, `IsPruned()` is read across six non-test files
(`core/chain/{floorbox_box_v5,validate_v5_quorum,chain,retention,equivocation}.go`,
`cmd/silt/chainstatus.go`), `Reconcile` compares `fork[0].Hash()`, and the carrier's `Hash()` pin
(CD-0) moves with it. **Era-1/2 keep the full-body rule and the pruned-evidence refusal forever**, so
silt carries BOTH paths — the change adds a branch rather than deleting one. The build needs its own
delta certification.

**Verdict on merit:** it removes a class of defect (declared identity in place of recomputed
identity) rather than shrinking a symptom, and it retires a concept plus two downstream defences for
v5. That survives simplicity rule 1 and rule 8 with the era clause deleted. **RECOMMEND BUY as
correctness — and, per rule 2, buy it under the restated justification, not the fallen one.** A
FORMAT item, so the **OWNER** takes it individually.

### MinBond — same conclusion, honest reason

**Withdrawn:** *"it blocks D3 because the freeze locks the fix."* It does not; the fix can land after
a soft freeze. **The honest reason, which reaches the same place:** **D3 gates the B8 engagement, and
silt does not spend the longest-lead item on the roadmap attacking an artifact with a known I1
divergence.** `R-CONSENSUS-CONFIG-UNBOUND` should land before D3 on that ground.

### The manifest audit with the clause deleted — ONE ITEM SHOULD COME OUT

Re-reading the 22-item manifest (§ groups A–D) with *"or it costs an era"* struck:

| Item | Survives on merit? | Why |
|---|---|---|
| 1 `tagRevLogSize` | **YES, strongly** | A wrong `m` is a WRONG-ACCEPT, and every floor box *dies permanently* at the first takedown block after its pin. Safety + liveness, no deadline needed. |
| 2 digest set 5 → 3 (`R-membership`) | **YES** | It is a **removal** — two committed leaves fewer. Its urgency is honest and is not permanence: it is free *while era-4 is dark*, because nothing has committed under it. |
| 3 (d-3) | **YES, re-justified above** | Retires `Pruned` for v5; restores hash coverage to a pruned block's retained body. Bought as correctness, not as evidence size. |
| **4 `IssuerKeyReg` PoP slot, reserved inert** | **NO — RECOMMEND DROP** | See below. |
| 5 `R-AAXIS-TAG-RESERVE` | already REFUTED | — |
| 6 height-0 identity | not in the freeze surface | — |
| 7–13 (validity rules) | unaffected | They are DoS bounds and wrong-accept closes with independent merit, or already DECLINED / NOT-THIS-RELEASE. |

**Item 4 is a pure deadline artifact and should come out of the train.** Reserving an *inert,
unpopulated* field buys exactly one thing: not paying an era later. **Delete the era cost and the
reservation buys nothing at all.** It is the same error the manifest certification itself names
twice — it refutes the A-axis tag reservation on this ground as its own item 5 (already closed), and
says of (d-3) *"Do NOT propose a reserve-only hedge… That is the same error."* Item 4 survived only
because the era clause was still standing.

**And dropping it removes a real surface, not just a line.** Per the 2026-09-10 PoP certification,
`Prune()` drops only `BondReg.Answer`, so `IssuerKeys` is **UNPRUNABLE like `Slashes`**: at the
count cap the reserved slot adds `4,096 × 4,096` = **16 MiB per block, a second permanent surface
EQUAL to `SlashesBytesCap`** — the very surface `R-NEST-GATE` just measured being weaponised. Buying
a 16 MiB permanent attack surface as a hedge against a future era cost, when the era cost is void, is
the scope-magnet failure in its clearest form.

**If a PoP is ever needed, (d-3) makes it cheaper than the reservation does:** option beta in that
certification folds `PoPDigest` into (d-3), which makes the bytes **PRUNABLE**. Buying item 3 is
therefore also the better hedge, and it is bought on correctness.

**Owner call: DROP item 4.** Recommendation only — a FORMAT item is his.

---

## D-FREEZE-CALLS-CDEF-2026-09-10 — (d-3) bought on the third-time rule; item 4 dropped; the config gate merges with its scope marked; the genesis-config family binds as ONE change

- **Status:** ✅ FOUR OWNER CALLS DECIDED — 2026-09-10, closing the docket opened by
  `D-PREIMAGE-CERT-2026-09-10` and re-priced by `D-FREEZE-REPRICE-2026-09-10`. **No further calls go
  to the owner before D1 lands.** What he reads next is the one-page *what is frozen, and what can
  never change without a new era*, re-checked against the manifest's FINAL content.

### Call C — (d-3) `AnswerDigest`: BUY. The argument is the third-time rule, not elegance.

**BOUGHT**, gated on its delta cert (commissioned 2026-09-10). The owner resolved the question the
builder flagged as the strongest objection — *would landing `R-CARRIER-PRUNED-HASH` be enough and
cheaper?* — and the resolution is the governing principle here:

> **A proof that a property holds today is not a structure that makes violating it impossible.**
> `R-CARRIER-PRUNED-HASH` would prove the discipline currently holds; any later change can silently
> violate it. (d-3) makes the retained body self-covering, so **violation becomes impossible rather
> than absent.**

**Why this is the third-time rule (`build-process.md` rule 5) rather than a preference for elegance.**
Documented-discipline-plus-proof has degraded to decoration three times in one month: the coverage
classification that was *"TRUE but un-driven"*; the `SlashesBytesCap` invariant nothing bound; the
`MinBond` coordination requirement nothing enforced. And `Block.Hash()`'s own comment records that
**this specific safety claim has shipped false three times.** The rule fires: **encode it as
structure, not as a fourth proof.**

**On the dual-era branch (the builder's other objection): it survives.** The branch is keyed on
`BlockVersion`, which already discriminates eras everywhere. The alternative is freezing v5 onto a
rule whose own comment documents three false safety claims, purely to avoid a branch — *"that's the
wrong trade, and it isn't elegance-seduction, it's refusing to carry a known defect across the
door."*

### Call D — manifest item 4 (the inert PoP slot): DROP

**DROPPED.** The certification refutes reserve-only hedges twice, and item 4 survived only on the era
clause the owner has since withdrawn. Buying **16 MiB per block of permanent unprunable surface** —
the exact surface `R-NEST-GATE` measured being weaponised — as insurance against a cost that no
longer exists is **negative-value insurance**.

- **Sequencing: C lands first**, so option beta's prunable-`PoPDigest` fallback exists.
- **If the off-chain `demandMsg` binding proves insufficient post-freeze:** add the field then and
  pay in re-runs and calendar, *"which is what the freeze actually costs."*

### Call E — the config-in-consensus gate: MERGE, with one condition drawn from silt's own lint

**MERGE.** The owner accepted the builder's own strongest objection as valid — declarations citing
`validate_v5_*` files the test never executes is the decoration shape — and then named the
distinguishing fact: **this gate has demonstrated efficacy** (it found the third and fifth unbound
fields, and its reflective half caught two `Config` fields a hand-read missed minutes earlier).
*"Its limit is regime coverage, not efficacy."*

**The condition:** apply `scripts/check_source_gates.py`'s own rule to the gate's declaration table —
every declaration citing a `validate_v5_*` file carries **`UNGATED: R-CONFIG-GATE-V5-REGIME`**, in
the **declaration and the failure text**, not only the header. That converts a misleading citation
into a disclosed one using a convention silt already has. **Built and gated:** the marker is
enforced (a v5 citation without one is RED, ablation A9) and travels into both the report and the
violation text. `R-CONFIG-GATE-V5-REGIME` is filed as a **replacement** for the era-1 regime, not a
sixth.

*On blind review breaking the gate twice:* **"evidence the loop works. A gate that survived review
unbroken would worry me more."**

### Call F — the genesis-config family: ALL FIVE FIELDS, ONE CHANGE, BEFORE D3

**The scope growth is nominal, not material, and the ruling absorbs it.** The builder escalated
rather than absorbing it, which was correct; the answer is that **the genesis-hash-covered shape
binds a FAMILY, so going from two fields to five is adding entries to a digest.** The mechanism the
owner already specified covers it. **One change, not five.**

**And the two fields that arrived AFTER the ruling are the SHARPEST of the set — this corrects the
builder's own framing.** The builder asked whether this is latent like the floor-box wrong-accepts.
**No, and `Era3/Era4ActivationHeight` is exactly why:**

> *"The floor-box holes are latent because the code is dead. `MinBond` divergence needs an operator
> to set a divergent value — low exploitability, agreed. But an **activation-height divergence needs
> no malice at all.** Two honest operators with different values flip eras at different heights and
> validate different blocks under different rules. That isn't a hole waiting for an attacker; **it's
> a scheduled fork that fires on a date.**"*

**The governing sentence: you cannot freeze an era boundary whose value nothing binds across
replicas.** All five, before D3. Mechanism is the builder's plus research, as delegated.

### The session scope the owner set, in order

1. **C** — (d-3), gated on its delta cert. **If the cert refutes, STOP and report; do not route
   around it.**
2. **D** — drop manifest item 4, *after* C lands so the fallback exists.
3. **E** — merge the config gate with the `UNGATED` markers; file `R-CONFIG-GATE-V5-REGIME` as a
   replacement.
4. **F** — bind the genesis-config family, all five fields, one genesis-hash-covered bind.

**Net scope moves DOWN:** one drop, one trivial addition, one item already owed, and a five-field
bind that costs the same as two. *"That's buy-less-and-better with a number attached."*

---

## D-D3-CERT-REFUTATION-2026-09-10 — the (d-3) DIRECTION is certified; its SPECIFICATION is refuted in two clauses, one of which would have broken a frozen format on live history

- **Status:** ⛔ STOPPED AND REPORTED — 2026-09-10, per the owner's standing instruction on owner call C:
  *"If the cert refutes, come back to me, don't route around it."* Certification:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/D3-ANSWERDIGEST-TWO-LEVEL-HASH-DELTA-RESEARCH-CERTIFICATION-2026-09-10.md`.
  **Verdict: GATED.** Nothing is built. Owner call D (drop manifest item 4) is sequenced after C and
  therefore also stalls.
- **THE PURCHASE SURVIVES; THE SPECIFICATION DOES NOT.** The owner bought (d-3) for **self-covering**
  — retiring `Pruned` — on the third-time rule, and **that limb is CERTIFIED.** The size leg was
  already withdrawn from his justification before this cert ran. What is refuted is the freeze
  manifest's §4.3 spec text and one surviving size sentence.

### The stop-and-report finding — the spec as written breaks the frozen-format immutable

Freeze-manifest §4.3 item 1 specifies `AnswerDigest ports.Hash \`cbor:"8,keyasint,omitempty"\``, and
item 3 asserts *"`omitempty` already gives that property for every additive field."* **Both are
false, and this repo says so in two places — verified at source:**

- `core/chain/chain.go:569-572`: *"A plain `ports.Hash` (`[32]byte`) would NOT work: `omitempty` never
  omits a fixed-size ARRAY (it is never 'empty'), so a zero `[32]byte` would be emitted as 32 zero
  bytes and change every era-2 hash. The byte-identity oracle caught exactly that; the pointer is the
  fix."*
- `core/chain/lastcommit_carrier_pins_test.go:35-37`: **`Pruned` is the living proof** — a
  `[32]byte`, so *"key 14 is present in EVERY encoded block body, zero-valued for a non-pruned
  block. It is part of the frozen bytes."*

**Why that is a bright-line breach and not a bug.** `bodyHash()` (`core/chain/chain.go:960`, verified)
folds `BondRegs: b.BondRegs` into the unsigned literal with **no version branch at all**. So a
fixed-size field on `BondReg` is emitted into every era's preimage, and **the hash of every v2 and v4
block carrying a bond registration changes — on live history, before era-4 ever activates.** That is
immutable **F** (frozen consensus formats), which is amended only by a new era, never by an edit.

**Repair, CERTIFIED:** `AnswerDigest *ports.Hash` — the pointer form, which is the *exact* era-3
step-2a fix already proven in this repo for `StateRoot`/`LogRoot`. Mechanism, not a new purchase.

### The second refuted clause — `Slashes'` is unnecessary and harmful

§4.3 reduces each embedded evidence block to *"its own era's header form"* — a recursively reduced
COPY. **REFUTED:** `Prune()` never touches `Slashes` (`core/chain/chain.go:990-992`, verified:
*"Note that Prune does NOT recurse into Slashes: evidence bodies embedded in a committed block stay
resident forever, which is why `SlashesBytesCap` bounds that slot"*), so the reduction buys nothing
for the self-covering property — and it adds a **per-hash 16 MiB deep copy**, re-opening `#563`.
**`SlashesDigest` is CERTIFIED as the required form.** This closes C-4 of `R-CERT-REDERIVE`, whose
one-word spec repair was exactly this. Scope add → **owner call 1**.

### THE OWNER'S RATIFIED SENTENCE IS WRONG A THIRD TIME — on the number this time

The replacement he ratified on 2026-09-10 reads: ***"(d-3) shrinks the face roughly 40× but does not
remove it."*** The cert self-corrects its own earlier arithmetic: **as specified, (d-3) shrinks the
face by ZERO, and is marginally negative.**

The mechanism, verified: `SlashesBytesCap` is enforced against `SlashesEncodedSize`
(`core/chain/chain.go:2372`), which **marshals the actual `[]Equivocation`** — real encoded bytes,
full `Block` bodies included. §4.3 reduces the **hash preimage**. Changing what `bodyHash` folds does
**nothing** to what `SlashesEncodedSize` measures.

Recovering any shrink at all needs a **mandatory evidence-strip validity rule that nobody has
bought** → **owner call 2**.

**This is the third correction to one ratified sentence** (*"only (d-3) removes"* → false; the
coalition clause → materially under-disclosing; now *"roughly 40×"* → zero). It is left in place and
annotated, per the standing practice on ratified text.

### The rest of the cert

- **Limb 1 — direction CERTIFIED**, and the close needs no new code: it makes four **existing**
  proposer-signature checks (`chain.go:2835`, `:3454`, `:3524`, `floorbox_box_v5.go:108`) stop being
  vacuous on pruned blocks. The owner's ratio decidendi is upheld.
- **Limb 3 — 16 sites, 12 change.** `Reconcile` needs **no** change, which corrects the blast radius
  routed to the researcher; four sites were missing from it. The **disqualifying widening** to refuse
  is *"a pruned v5 block is self-authenticating, so trust it at any height"* — identity ≠ bond
  possession, and `trustFloor` stays.
- **Limb 4 — NO era-boundary wedge**, and the reason matters: `bodyHash` is **unary**, so `b.Version`
  IS the signed object's version. The carrier wedges because it is **binary** (child verifying a
  parent's attestations). **A builder over-generalising the sibling preimage cert gets this exactly
  backwards.**
- **Limb 6 — CERTIFIED**, with a trap named: the v5 pruned-evidence refusal becomes structurally
  unreachable, but **deleting `ErrPrunedEvidence` re-opens I5 for every pre-v5 height, permanently.**
- **Limb 7 — the witness read-set does NOT move** (a freeze surface). Frame/resident cost
  +≤8.5 KiB per block at `RegCap = 256`.
- **Three gates on `main` would stay GREEN straight through this change** and must be re-pointed
  first: `TestHashLiteralPinsEveryHashCoveredField` (it unions the two literals),
  `TestHashLiteralPinRuntimePair` (fixture is `Version: 1`), `TestCarrierHashDriftGuard` (its v5 case
  carries no `BondRegs`). Gates `G-D3-1 … G-D3-11`, all RED-first.

### Execution caveat, recorded rather than smoothed over

The Researcher ran **no shell commands** this round: every finding is source-read or derived, and
labelled as such in the cert. The builder independently verified the load-bearing ones at source
(`chain.go:569-572`, `:822`, `:852-854`, `:2217`; `lastcommit_carrier_pins_test.go:35-37`). The cert
also could not read the then-unmerged branch; that branch is now `main` @ `76bf707` and touched **no
`.go` file** except the new divergence gate, so its reads stand.

---

## D-CFGBIND-CERT-2026-09-10 — the genesis-config bind is certified; membership corrects UPWARD to 17, and a live unbound consensus parameter was found OUTSIDE the gate's scope

- **Status:** ✅ CERT RECEIVED, ⚠ GATED — 2026-09-10. Certification:
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/GENESIS-CONFIG-FAMILY-BIND-RESEARCH-CERTIFICATION-2026-09-10.md`.
  Answers owner call F (`D-FREEZE-CALLS-CDEF-2026-09-10`). **Direction CERTIFIED, mechanism
  specified, membership corrected upward, four limbs GATED.**

### The finding that changes the work — and it indicts the gate this session shipped

**`-bond-label-k` (`BondLabelSamples`) is a live, shipped, UNBOUND consensus parameter, and it is
sharper than anything in the owner's five.** Verified independently at source:

- `core/bond/bond.go:489` — `if len(a.LabelIndices) != kk || len(a.LabelBundles) != kk { return false }`
  — **exact equality against the verifier's OWN local value.**
- `cmd/silt/daemon.go:930` wires it from local config:
  `SetBondVerifier(node.SpaceTimeBondVerifier(cfg.BondVDFDelay, cfg.BondLabelSamples))`, reaching a
  hard `Reject` at `core/chain/validate_v5_quorum.go:228`.
- `cmd/silt/daemon.go:128` — the flag help states the coordination requirement *"prover and verifier
  must MATCH … set it uniformly across the swarm"* **and in the same breath invites the change:**
  *"Lower it only to shrink on-chain proof size, at a soundness cost."*

**A node with `k = 32` rejects EVERY bond registration a `k = 64` swarm accepts** — not a straddling
bond, all of them. `BondVDFDelay` is the same shape, latent.

**Why the divergence gate missed it: its closed complement is closed over the WRONG SET.**
`TestConsensusVerdictIsNotAFunctionOfLocalConfig` reflects over **`chain.Config`**; these fields live
in **`node.Config`**. The gate's central claim — that a new field cannot be added without forcing a
declaration — holds only inside the package it reflects over. Filed as `R-CONFIG-GATE-NODE-SCOPE`.

### Membership — 17 IN, 5 OUT, and the exclusions each differ

**IN:** the 15 `chain.Config` fields that reach a verdict, plus `BondLabelSamples` and `BondVDFDelay`.
**This is absorbed by the owner's existing ruling** — he ruled the growth *"nominal, not material… a
genesis-hash-covered bind covers a FAMILY, so going from two fields to five is adding entries to a
digest."* The same reasoning carries 5 → 17.

**OUT, each for a different and load-bearing reason:**

| Field | Why out |
|---|---|
| `Archive` | retention only; never reaches a verdict |
| `WSCheckpoint` | narrowing-only, and **sharing it would destroy weak subjectivity** — it is the operator's own trust anchor |
| `MinProposerRep` / `MinAttesterRep` | **binding is INEFFECTIVE** — the input is the local `rep` view, so a shared threshold still diverges |
| `LivenessRecoveryHeight` | **structurally unbindable** — set after launch, on a chain that by construction cannot commit it (`R-LIVENESS-RECOVERY-UNBOUND`, new) |

### The mechanism

- **Shape (1) CERTIFIED and needs no new machinery** — `core/chain/chain.go:4145-4147` already refuses
  a foreign genesis. **Carry VALUES, not a digest:** values reuse `Block.Hash()`'s canonical CBOR (so
  no new injectivity proof is owed) and make the refusal *diagnosable*. `Params *ConsensusParams`,
  cbor key 19, **pointer per the omitempty-array rule**, no `omitempty` inside, `Anchors` as a sorted
  slice. Optional field, so ~250 `AppendGenesis` fixtures stay untouched.
- **Shape (2) REFUTED** as a family mechanism, except the era heights, where the tally branch is
  already bound and certified — F's job there is to close the **override**.
- **Shape (3) REFUTED:** a gossiped digest is an unauthenticated claim; a genesis hash is
  self-authenticating.
- **⚠ "DETECTED AT HANDSHAKE" IS REFUTED AS A DESCRIPTION OF WHAT SHIPS.** The owner called that
  *"the right failure surface"*, and it remains the right one — but **there is no genesis exchange at
  the TLS handshake**, and the refusal that does fire is logged at `LogDebug`
  (`core/node/chainrole.go:1616-1618`), so **the operator sees a node that never syncs and says
  nothing.** Making that refusal loud is part of the build, not a nicety.

  > **⚠ CORRECTION 2026-09-10 (docs true-up) — the sentence above is now FALSE, and is annotated
  > rather than rewritten.** The refusal fires at **`ports.LogWarn`**, inside `(*Node).SyncChain`,
  > naming the likely cause and the flags to check. `#800` made it loud and this certification's text
  > was never annotated. **Name the class, because it is the one the new lint cannot catch:** the
  > coordinate `core/node/chainrole.go:1616-1618` still resolves to the right symbol — the cited-source
  > check shipped in `#803` verifies exactly that and passes — so what rotted was the **claim about
  > the content**, not the pointer to it. A symbol-anchored coordinate proves you are looking in the
  > right place; it cannot prove the sentence about what you find there is still true.
  >
  > **Second stale claim in this entry, same class:** the mechanism section below specifies `Params`
  > at **cbor key 19**. Shipped is **key 20** (`core/chain/chain.go`, `Block.Params`); **19 is
  > `SlashesDigest`**. The build is right and the certification text is stale.
- **T-REFERENT — rule 8's two arms COMPOSE.** Genesis-covering manufactures the referent, which makes
  a refuse-to-start *required* rather than redundant: it catches the case nothing else does — an
  operator editing a flag and restarting on an existing chain.

### Ordering, and the calendar cost

**F moves the genesis hash; call A (the chain id in the preimage) consumes it. Land F before or with
A, in ONE genesis move, or the re-run set is paid twice** — that is the entire calendar cost, which
is the freeze's actual currency (`D-FREEZE-REPRICE-2026-09-10`).

> **⚠ CORRECTION 2026-09-10 (docs true-up) — the constraint above was VOID AS STATED. It still binds,
> for a different reason.** Two errors, both verified at source:
>
> 1. **F as merged moved nothing.** It shipped schema only — `core/genesis/genesis.go` mints genesis
>    with no `Params` field and `(*Chain).CheckConsensusParams` had zero non-test callers.
> 2. **F's wiring, now built, does not move the *paramless* genesis hash either.** `Block.Params` is
>    `*ConsensusParams` with `omitempty`, so a genesis minted without params drops key 20 entirely:
>    `e44344ea…72c0` is unchanged and the ~250 `AppendGenesis` fixtures are untouched. What moves is
>    any genesis a **daemon** mints.
>
> **The corrected constraint: call A joins F's WIRING in ONE network re-seed, not one fixture
> re-pin.** The currency is graded field re-runs and calendar, exactly as
> `D-FREEZE-REPRICE-2026-09-10` prices it. This matters because the constraint as written would have
> been discharged by a fixture edit that buys nothing, and the double payment it exists to prevent
> would have been paid anyway. (d-3) is independent in value but
coupled in build: it splits `bodyHash` into two literals and CD-0's gate unioned them
(`G-CFGBIND-11`) — **already closed this session** by the red-first gate re-point.

### GATED limbs, recorded rather than smoothed

**G-A** — the Researcher had **no shell** and never ran the divergence gate; the map was read from
its `wantDivergence` literal. **G-B** — `-bond-label-k` is proven by source reads, not a driven
verdict (the builder verified the three cited lines independently; a DRIVEN gate is still owed).
**G-C** — "~0 fixtures affected" is a grep claim, not a compile. **G-D** — the graded re-run count is
the Tester's to price.

### What it does NOT close

`R-CONSENSUS-CONFIG-UNBOUND` closes **on the production path only**. Still open:
`R-LIVENESS-RECOVERY-UNBOUND` (new, unbindable), `R-CONFIG-GATE-NODE-SCOPE` (new — `node.Config` is
unaudited beyond two fields), the surviving paramless path, and the legacy-leg subjectivity.

---

## D-D3-BUILT-2026-09-10 — (d-3) is built: `Pruned` is retired for v5, and the parity contract is NOT amended

- **Status:** ✅ BUILT, package green — 2026-09-10. Owner call C (`D-FREEZE-CALLS-CDEF-2026-09-10`)
  delivered against `D3-ANSWERDIGEST-TWO-LEVEL-HASH-DELTA-RESEARCH-CERTIFICATION-2026-09-10.md` and
  the follow-on
  `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/D3-PARITY-MALFORMEDPRUNED-DELTA-RESEARCH-CERTIFICATION-2026-09-10.md`.
- **The purchase, driven not asserted (G-D3-7).** A pruned v5 block recomputes its own hash from
  what it retains, carries no `Pruned` token, and mutating its retained body MOVES the hash. The
  same gate drives the **contrast on v2**, where the identical rewrite is invisible — so the defect
  `Block.Hash()`'s comment records as having "shipped false three times" is demonstrated, not
  described.
- **The certified spec repairs both landed:** `AnswerDigest *ports.Hash` (the pointer form — the
  manifest's bare array would have broken frozen v2/v4 bytes) and `SlashesDigest` replacing the
  recursively-reduced `Slashes'`. **cbor key 19 allocated to `SlashesDigest` after verifying 1..18
  were taken; the genesis-config family bind takes key 20.** Both certs said "next free, verify at
  build", and both were right to — they had each claimed 19.

### The defect this build introduced, and the cert that named it in advance

Retiring `Pruned` for v5 broke a signal that was **serving two masters**: `IsPruned()` meant BOTH
*"identity is declared, not recomputable"* AND *"heavy proofs are shed, so bond verification cannot
be re-run"*. (d-3) changes only the first. Left alone, the trust-floor refusal
(`ErrPrunedAboveHorizon`) would have gone **silently dead for v5** — the exact disqualifying
widening the build cert named: ***identity != bond possession, trustFloor stays.***

Fixed by splitting the signal (build-immutable #3): **`Block.HeavyProofsShed()`** — a registration
carrying no `Answer` but committing a digest that is not `answerDigestOf(nil)`. The bond-possession
sites are re-keyed to it; the identity sites keep `IsPruned()`. The floor-box sites 9 and 10 are
**KEEP, re-keyed** per the cert, which warned that letting site 10 die for v5 makes D0's ablation
vacuous (simplicity rule 7) — which is precisely what began to happen.

### The parity question: the ACCEPTANCE was sound, the REASON was refuted

The builder proposed that (d-3) *deletes* the malformed-pruned category on v5 — *"there is no v5
block that can be marked pruned while carrying an Answer."* **That is FALSE**, and the delta
certification refuted it against a live site: `core/chain/validate_v5_quorum.go:48-52` is reachable,
v5-only, and still returns `ErrMalformedPruned` (verified at source).

**What actually changed is GRANULARITY, not existence.** `Pruned` was a per-BLOCK mark, so ONE
registration could contradict it. `HeavyProofsShed()` is a per-ITEM property lifted by `∃`, so
contradicting it needs a **SECOND** registration. The one-reg fixture could only reconstitute the
byte-identical valid original, which a v5 reader **must** accept. **The fixture failed, not the
contract.**

**So the #572 attribution contract is NOT amended.** The arm now carries two registrations and
restores the `Answer` on one; both twins still want `ErrMalformedPruned`, with identical rendering.
The builder's own proposal — an era-specific expectation — was refuted on two independent grounds,
either sufficient: it is factually wrong, and it would have deleted the **only** driven coverage of
that v5 site.

**A sixth EXCLUSION is added and DRIVEN** (G-PMP-3): a **forged** `Answer` is a genuine by-rule
divergence — v4 reaches `ErrMalformedPruned`, v5 is caught earlier and more specifically by
`ErrD3DigestMismatch`. Driven rather than claimed in the header, per simplicity rule 7.

### What the certification found that nobody routed — Probe C

A block below the trust floor with `reg0` genuinely shed, where an attacker **appends** a `reg1`
carrying `Answer == nil` and `AnswerDigest == answerDigestOf(nil)`. `HeavyProofsShed()` is true from
`reg0`, so the verify loop is skipped and `reg1` would take bonded standing **with no proof ever
verified**. **This works on v4** and is **structurally closed on v5** — the appended registration is
inside the v5 preimage, so the hash moves. That is the (d-3) purchase paying out on a surface nobody
aimed it at, and it is *why* the `answerDigestOf(nil)` carve-out in the predicate is safe rather
than a hole. **Name that precondition wherever the predicate is defended.**

### Corrections to the certifications themselves

1. The build cert's `T-TWO-JOBS` predicate was **under-specified** — without the third conjunct, a
   registration that never had a proof reads as shed. The implemented form is the correct one.
2. **`chain.go`'s `validateBondRegs` is UNREACHABLE for a v5 block on the node path** (its sole
   non-test caller sits inside `ValidateProposal`, which returns for v5 earlier). The re-key stays —
   it is correct and defensive — but **it must not be counted as v5 coverage**. Related: a comment
   claiming v5 `RegCap` is "enforced here, on the commit path" is false.
3. Both delta certifications ran **with no shell**. Every load-bearing claim was re-verified at
   source by the builder before being acted on, and the gates below are what lift them from derived
   to measured.

### Gates

`G-D3-1..7` with a six-ablation battery, and `G-PMP-2` (ablate the v5 malformed-pruned arm → the
parity arm reddens, proving it drives the v5 site and not the v4 one alone). All verified by **exit
code**. One ablation in the battery first reported a **false GREEN**: the patch anchor matched two
sites, so the edit silently no-opped and never ran. **A no-op patch is indistinguishable from a
passing ablation** — verify the source actually changed before believing the result. Recorded in the
gate's own comment, and it is why the D0 ablation instructions that still named `IsPruned()` targets
were corrected in the same commit: an ablation instruction naming a line no longer in the source is
worse than none.

---

## D-ITEM4-DROPPED-2026-09-10 — freeze-manifest item 4, the inert PoP slot, comes out of the train

- **Status:** ✅ DECIDED — 2026-09-10, owner call D (`D-FREEZE-CALLS-CDEF-2026-09-10`), executed after
  call C landed so the fallback below actually exists. Sequenced by the owner, not by convenience.
- **Verified before acting: it was never built.** Zero non-test PoP sites across `core/`, `cmd/` and
  `ports/`; `IssuerKeyReg` (`core/chain/issuerkey.go`) carries four fields — `Pub`, `Epoch`,
  `Fingerprint`, `Sig` — and no slot. **Dropping it is a ledger act with no code change.**

### Why it comes out

**Reserving an inert, unpopulated field buys exactly one thing: not paying an era later.** With the
freeze deadline re-priced as SOFT (`D-FREEZE-REPRICE-2026-09-10`), that cost is void, so the
reservation buys **nothing at all**.

It is the same reserve-only error the freeze-manifest certification names **twice** — it refutes the
A-axis tag reservation on this exact ground as its own item 5, and warns *"do NOT propose a
reserve-only hedge"* about (d-3). **Item 4 survived only because the era clause was still standing.**
It was found by re-reading the manifest with that clause struck, which is the audit the owner
ordered.

### Dropping it removes a real surface, not just a line

`Prune()` drops only `BondReg.Answer`, so `IssuerKeys` is **UNPRUNABLE like `Slashes`**. At the
certified count cap the reserved slot would add `4,096 × 4,096` = **16 MiB per block — a second
permanent surface EQUAL to `SlashesBytesCap`**, which is the surface `R-NEST-GATE` measured being
weaponised by a lone proposer with throwaway keys. **Buying 16 MiB per block of permanent attack
surface as insurance against a cost that no longer exists is negative-value insurance.**

### The fallback now exists, which is why C went first

If the off-chain `demandMsg` binding ever proves insufficient, the answer is **not** a pre-reserved
slot. It is option beta of the PoP certification — fold `PoPDigest` into (d-3), which makes those
bytes **PRUNABLE**. **(d-3) landed today** (`D-D3-BUILT-2026-09-10`), so that route is available.
The owner's instruction if the need lands post-freeze: *"add the field then and pay in re-runs and
calendar — which is what the freeze actually costs."*

### What is NOT decided here

The **research-gated D-DEMAND change** (the DSKS close — a PoP in the registration, or the RFC 9578
binding) is unaffected and stays post-RC. Dropping the reservation removes a *format hedge*, not the
security question it was hedging. `IssuerKeyPoPMaxBytes` (certified 2026-09-10 at 4,096 bytes,
INADMISSIBLE ALONE) is **not ratified and not reserved** — it becomes an input to that post-RC work
rather than a frozen constant, and its self-corrections of freeze-manifest §4.6 travel with it.

**Net effect on D1: the manifest loses one FORMAT item and gains none.**

---

## D-CFGBIND-BUILT-2026-09-10 — the genesis-config family is bound to the chain; canon rule 8's two arms compose

> ⚠ **CORRECTION 2026-09-10, filed the same day and left beside the claim rather than folded
> into it — the SCHEMA shipped, the WIRING did not.** The entry below says the family binds *on
> the production path* and that a differently-configured node "computes a different genesis hash
> and cannot join". **That is false on `main` as built.** Verified at source:
>
> - `core/genesis/genesis.go` mints genesis as `chain.Block{Version: …, Height: 0, Entries: …}`.
>   It sets **no `Params` field**, so cbor `omitempty` drops key 20 and the production genesis
>   hash is **byte-identical to before this change**. Nothing populates the slot.
> - The **only** `Params:` literals on a `Block` anywhere in the tree are in
>   `core/chain/consensusparams_test.go`. (`chain.go`'s two `Params: b.Params` copies are
>   signature-preimage plumbing, not a mint.)
> - `CheckConsensusParams` has **zero non-test callers**, so rule 8's second arm is not armed
>   on any running node either.
>
> What is true today: the `Block.Params *ConsensusParams` field, its cbor key, its
> `CheckConsensusParams` comparator and its tests all exist and are green. What is NOT true is
> that any of it is reached in production. **`R-CONSENSUS-CONFIG-UNBOUND` is therefore NOT
> closed on the production path** — the divergence it names is still live. The wiring is a
> separate task pending an owner call; it is deliberately not built here, because binding the
> production genesis moves the genesis hash and that is a FORMAT act the owner ratifies
> individually. Read every "BOUND"/"cannot join" sentence below as describing the mechanism the
> schema makes POSSIBLE, not the behaviour of a node on `main`.

- **Status:** ✅ BUILT, `core/chain` and `core/node` green — 2026-09-10. Owner call F
  (`D-FREEZE-CALLS-CDEF-2026-09-10`) delivered against
  `GENESIS-CONFIG-FAMILY-BIND-RESEARCH-CERTIFICATION-2026-09-10.md`. Closes
  `R-CONSENSUS-CONFIG-UNBOUND` **on the production path**.
- **The mechanism:** `ConsensusParams` — **17 fields carried by VALUE** — committed on the genesis
  block as `Block.Params *ConsensusParams` at **cbor key 20**, so the genesis hash covers it. A node
  configured differently computes a different genesis hash and **cannot join at all**: `Reconcile`
  refuses the fork with `ErrForeignGenesis` before any validity question arises. Divergence becomes
  *impossible to join with* rather than fatal at validation.
- **Key 20, not 19.** Both the genesis-config and (d-3) certifications proposed key 19 and both said
  *"next free, verify at build"*. Verified: 1..18 were taken, (d-3) landed first and took 19.
- **VALUES, not a digest** — so a mismatch is **diagnosable**: `CheckConsensusParams` names the field
  that differs. A digest would only prove two hashes differ, and a *gossiped* digest was refuted
  separately as an unauthenticated claim where a genesis hash is self-authenticating.

### Rule 8's two arms COMPOSE (T-REFERENT) — this is the part worth carrying forward

A refuse-to-start was **refuted for this class**: `MinBond` divergence is not locally observable, so
a start-up assertion has nothing to assert against. But **committing the values MANUFACTURES the
referent it lacked**, which makes a start-up check *required* rather than redundant — it catches the
one case joining cannot: **an operator editing a flag and restarting on a chain the node has ALREADY
joined.** The genesis on disk is unchanged, so no fork boundary is crossed and nothing else would
notice. `Chain.CheckConsensusParams` is that arm, driven by G-CFGBIND-4.

### Membership: 17 IN, 5 OUT, and the exclusions differ from each other

Getting membership wrong is **asymmetric**: too few leaves a consensus quantity unbound; too many
refuses honest operators for differing on something that was never theirs to agree on. So each
exclusion carries its own reason, and a reflective gate (G-CFGBIND-1) fails on any `Config` field
that is neither carried nor excluded.

| OUT | Why — and they are not the same reason |
|---|---|
| `Archive` | retention only; reaches no verdict |
| `WSCheckpoint` | **sharing it would DESTROY weak subjectivity** — it is the operator's own trust anchor |
| `MinProposerRep` / `MinAttesterRep` | **binding is INEFFECTIVE** — the divergent term is the local reputation *view*, not the threshold |
| `LivenessRecoveryHeight` | **structurally unbindable** — set after launch on a chain that by construction cannot commit it (`R-LIVENESS-RECOVERY-UNBOUND`) |

**The two sharpest members came from `node.Config`, not `chain.Config`:** `BondLabelSamples` and
`BondVDFDelay`. `core/bond` compares a proof's label count against the verifier's **own local** value,
so a `k=32` node rejects **every** bond registration a `k=64` swarm accepts — and the flag help
states the coordination requirement while inviting the change. They are carried by value here to
avoid an import cycle. The divergence gate never saw them because its reflection is closed over
`chain.Config` (`R-CONFIG-GATE-NODE-SCOPE`, still open).

### "Detected at handshake" was refuted as a description of what SHIPPED — and is now fixed

The certification found there is no genesis exchange at the TLS handshake, and the refusal that does
fire logged at **`LogDebug`** (`core/node/chainrole.go`) — so *"the operator sees a node that never
syncs and says nothing."* `ErrForeignGenesis` is now split out of the generic non-adoption branch and
logged at **`LogWarn`**, naming the likely cause and the flags to check, with a
`ChainSyncForeignGenesis` stat. Because the genesis now commits the config, the overwhelmingly likely
cause of a foreign genesis is a **divergent local flag**, not a hostile peer — so the remedy is
nameable, and it is named.

### Gates and ablations

`G-CFGBIND-1..5`, ablation battery **B0–B6**, all verified by exit code: drop `Params` from the
pre-v5 literal → G-CFGBIND-2 RED; drop a field from `ParamsFromConfig` → G-CFGBIND-4 RED; remove a
`Config` field's decision → membership RED; drop the placement rule → G-CFGBIND-3 RED; make the
refusal undiagnosable → G-CFGBIND-4 RED.

**Two of those first reported FALSE GREEN because the patch never applied**, and the battery caught
it: this run carried an explicit no-op guard (`diff` the patched file against its original before
believing the result), added after the (d-3) battery was bitten by exactly this. **A patch that
silently no-ops is indistinguishable from a passing ablation**, and the guard is now the habit.

### What this does NOT close

`R-CONSENSUS-CONFIG-UNBOUND` closes **on the production path only**. Still open:
`R-LIVENESS-RECOVERY-UNBOUND`; `R-CONFIG-GATE-NODE-SCOPE` (the divergence gate still reflects over
`chain.Config` alone); **the surviving paramless path** — a genesis predating the bind carries nil
and still starts, which keeps ~250 fixtures byte-identical and is **disclosed, with G-CFGBIND-5
asserting it deliberately** rather than leaving it to chance; and the legacy-leg subjectivity, which
no bind can reach.

### Ordering that still stands

**F moves the genesis hash; call A (the chain id in the signature preimage) consumes it.** They must
land in ONE genesis move or the graded re-run set is paid twice — which, with the freeze re-priced,
is the actual cost of the freeze.

## D-CFGBIND-MEMBERSHIP-RULE-2026-09-10 — the membership RULE replaces the field list, and owner call F is delivered

**Status:** BUILT. Supersedes the membership half of `D-CFGBIND-BUILT-2026-09-10`, whose "17 IN, 5
OUT" table was a list where the owner wanted a rule, and whose wiring claims were true of the schema
and false of the binary.

### What was actually wrong

`D-CFGBIND-BUILT-2026-09-10` reads as a shipped mechanism. It was a shipped **schema**.
`ConsensusParams` declared 17 fields at cbor key 20 and **nothing populated it**, verified at source
on `0ed3b92`:

- `core/genesis.Build` minted `chain.Block{Version, Height: 0, Entries}` — no `Params`. cbor
  `omitempty` on the pointer dropped the key, so every network on every binary minted the identical
  paramless genesis.
- `Chain.CheckConsensusParams` had **zero non-test callers**. All four call sites were in
  `core/chain/consensusparams_test.go`.
- `ParamsFromConfig` and `ConsensusParams.Diff` were dead by closure.
- The only non-test `Params:` occurrences were the two preimage pass-throughs in `Block.bodyHash`.

So the failure surface the decision described — *a divergently-configured node computes a different
genesis hash and cannot join* — did not exist. Every node computed the same hash regardless of its
`-min-bond`, `-quorum`, `-anchors`, `-epoch-blocks`, `-bond-label-k` or compiled bond-VDF delay
(which has no flag).

**Why five green gates did not notice.** Each of `G-CFGBIND-1..5` hand-constructs
`chain.Block{… Params: &p}` in its own fixture. **A gate that constructs the exact state whose
production absence is the defect cannot detect that defect.** The gates were correct about the
predicates and silent about the wiring, and nothing distinguished the two.

The instructive contrast is *inside the same commit* (`82fe56d`): (d-3) was wired end to end —
`setD3Digests` → `PopulateEra4Roots` → the two `core/node/chainrole.go` call sites — while the
genesis-config bind was inert. **One half of one commit was live and the other was dead, and no gate
told them apart.** That is the generalisable lesson, and it is the reason for the wiring pin below.

### The RULE, which is what is ratified

> **Every field that can change a validity verdict is BOUND TO THE CHAIN, or is EXPLICITLY EXCLUDED
> WITH A RECORDED REASON.**

The owner rejected both "five fields" and "17 fields" as the thing to ratify. A list has to be
remembered; a rule has to be satisfied. **17 is now the rule's OUTPUT** — 15 fields claimed by
`chain.Config`'s table plus 2 claimed by `node.Config`'s — and no one has to hold the number.

### The five exclusions stand, and they are five DIFFERENT arguments

They are recorded as five strings rather than one flag precisely because collapsing them would lose
the reasoning that makes each one safe.

| OUT | The argument, and it is not shared with any other row |
|---|---|
| `Archive` | **Retention only** — it reaches no verdict. Per-node by build-immutable #8; binding it would forbid the heterogeneity durability depends on. |
| `WSCheckpoint` | **Sharing it would DESTROY weak subjectivity.** It is the operator's own trust anchor and MUST vary per node — a checkpoint every operator got from the same place is not an independent anchor. It narrows and can never widen. |
| `MinProposerRep` / `MinAttesterRep` | **Binding is INEFFECTIVE.** The divergent term is the local reputation *view*, not the threshold: two nodes agreeing on the number still disagree, because they compare it against different inputs. Committing it would look like a fix and change nothing. |
| `LivenessRecoveryHeight` | **Structurally unbindable** — set after launch, on a chain that by construction cannot commit it (`R-LIVENESS-RECOVERY-UNBOUND`). Not a deferred decision; a shape the mechanism cannot hold. |

### The gate now covers BOTH structs, and the two tables are a checked bijection

`R-CONFIG-GATE-NODE-SCOPE` is **CLOSED**. `TestConsensusVerdictIsNotAFunctionOfLocalConfig`
reflected over `chain.Config` alone, which is the wrong set — and the cost was measured, not
hypothetical: `BondLabelSamples` and `BondVDFDelay` live in `node.Config`, reach a hard chain
`Reject` through `core/bond`'s verifier, and were invisible to it.

`TestNodeConsensusVerdictIsNotAFunctionOfLocalConfig` (`core/node`) closes the complement over
`node.Config`'s 36 fields. Both tables now name, per field, the `ConsensusParams` field that binds it
or the reason it is not bound, **resolved by reflection against the real struct** — a pin on prose is
not a pin, and blind PE F-4 already broke one binding pin that keyed on free text.

Together they must claim every `ConsensusParams` field **exactly once**:

- claimed by **neither** ⇒ committed but **unowned**: it rides in the genesis hash while binding
  nothing an operator can set;
- claimed by **both** ⇒ **double-counted**: one of the two knobs is unbound while both tables read as
  complete.

The bijection earned its place the moment it was written: it caught `MinBondBytes`, which exists in
both structs, being claimed twice. The daemon copies the `node.Config` value into
`chain.Config.MinBondBytes`, so the binding is real but the claimant is `core/chain`'s table — the
node-side row now records that, transitively, instead of asserting a second claim.

### The driven half, and what it deliberately refuses to claim

Simplicity rule 7 binds here. The `node.Config` gate seals a **real 1 MiB plot**, produces a **real
space-time answer**, and perturbs fields through **the production verifier closure**
(`SpaceTimeBondVerifier`), not a re-implementation. Measured divergence: exactly
`{BondLabelSamples, BondVDFDelay}`, and that set is pinned.

The other **31 fields report UNPROVEN, never "safe"**. Most of `node.Config` is transport, DHT and
repair policy the bond verifier never reads, so no probe here can move it. *A field that does not
diverge in a regime that could never have exercised it has been shown UNTESTED, not shown safe.*
Calling them safe would be the decoration failure in a new place.

### The wiring, and the pin that is the actual deliverable

- **`genesis.Build` REQUIRES the params argument.** A `BuildWithParams` variant beside a paramless
  `Build` would have left exactly the shape the defect shipped in available to the next call site.
  `nil` remains legal and means the pre-bind genesis; every `nil` site in the repo is a test or a sim.
- **The daemon projects params off the CHAIN** (`Chain.ConsensusParams`), not off the `chain.Config`
  literal, so the arm that WRITES genesis and the arm that CHECKS it read one source. Reading a
  second copy would mean the node that founded a network could refuse its own genesis at the next
  restart if `New` ever normalised anything.
- **`CheckConsensusParams` has its production caller**, as a refuse-to-start, placed **after the
  replay** — `chainstore.Recover` is the load-bearing boundary. The check reads `blocks[0]`, so it
  is meaningful only against a chain LOADED FROM DISK; ahead of the replay it reads an empty chain,
  returns nil, and the daemon serves under a config its own chain contradicts.

  **CORRECTED 2026-09-10 — and the correction is the lesson, so the wrong version stays visible.**
  The first draft of this line named the *genesis seed* as the boundary ("wired earlier it would be
  green on every start while checking nothing"). A blind review built that tree and measured it
  FALSE: relative to the mint the check is a **tautology** on a fresh node — both sides are
  `ParamsFromConfig` over one `cfg` in one process — so it refuses identically on either side of the
  seed. The gate that enforced the false boundary was therefore inverted with respect to severity:
  RED on the benign edit (W2) and **GREEN on the fatal one** (S4 — the check lifted into a helper
  defined later in `daemon.go` and called before `Recover`: both source gates green, the binary
  serving under a divergent `-bond-label-k`, zero refusal lines). Ruling:
  `silt-agent-memory/principal-engineer/reviews/RULING-owner-call-F-genesis-params-wiring-CODE-0d99aef-2026-09-10.md`.
- **`silt genesis` prints its hash labelled PARAMLESS.** A daemon-launched network no longer has one
  true genesis hash; the link and the manifesto root remain config-independent and are printed
  unqualified.

**The pin (`G-CFGBIND-6/6b/7/8`) is the part the audit says was missing.** It never constructs a
`Block{Params: …}` literal — it drives `genesis.Build`, and it reads `daemon.go` as text for the
call strings and their order. **For a new field on a consensus type, a pin requires a non-test
WRITER, not merely a reader**, and `G-CFGBIND-7` is that requirement: it fails if `genesis.Build`
stops being handed real params on the daemon path, which is precisely the state the tree was in.

**The instrument of record is `G-CFGBIND-10/11`, in `e2e`, added after the review.** A source gate
can only see strings, and S4 is the proof that a green one can sit over a completely dead mechanism.
`TestGenesisHashMovesWithTheConsensusConfig` starts real daemons and asserts the minted genesis hash
MOVES with `-bond-label-k` and is stable within a config;
`TestDaemonRefusesToStartOnADivergentConsensusConfig` persists a genesis committing `k=64`, starts a
daemon with `32`, and asserts the process exits naming that field. `G-CFGBIND-8`'s order assertion
was re-anchored to a SANDWICH — `chainstore.Recover` < the check < `nd.EnableChain` — which is RED on
S4 and correctly GREEN on W2; it remains a lexical proxy and now says so, and names the e2e as its
runtime cover.

### The genesis hash: what moved and what did not

The **paramless** hash is **unchanged** at `e44344ea…72c0`, and that is asserted, not assumed:
`Block.Params` is a pointer with `omitempty`, so `nil` omits cbor key 20 entirely and every committed
fixture keeps its identity. What moves is the hash of a **daemon-minted** genesis, which is now a
function of the config and therefore has no single value to pin. A representative params set is
pinned instead at `4a305b96…9c46`, so a cbor renumbering or a field reorder — which would change
every network's height-0 identity while leaving a "the hash moved with the params" assertion
perfectly green — turns `G-CFGBIND-6b` red.

### Ablations, each verified by EXIT CODE, each with a no-op guard

A patch that silently fails to apply reports GREEN and is indistinguishable from a passing ablation,
so every patched file was `diff`ed against its original before the result was believed.

| # | Ablation | Result |
|---|---|---|
| W1a | daemon mints with `genesis.Build(store, nil)` | RED (G-CFGBIND-7, 8) |
| W1b | remove the `CheckConsensusParams` caller | RED (G-CFGBIND-8) |
| W2 | wire the check between the replay and the genesis seed | **GREEN, correctly** — re-measured 2026-09-10: the daemon refuses identically, so the old RED was a lexical fact with no behavioural referent |
| **S4** | **lift the check into a helper defined later in `daemon.go`, called BEFORE `chainstore.Recover`** | **RED** (G-CFGBIND-8 upper bound; e2e `G-CFGBIND-11` times out waiting for a refusal that never comes). This is the ablation that was GREEN before the re-anchor |
| W3 | `Build` accepts params and drops them | RED (G-CFGBIND-6, 6b) |
| W4 | drop `Params` from the pre-v5 hash preimage | RED (G-CFGBIND-6, "the commitment is decoration") |
| N1 | **add a fake verdict-reaching field to `node.Config`** | **RED** — the owner's binding condition |
| N2 | stale declaration for a field `node.Config` lacks | RED |
| N3 | re-declare `BondLabelSamples` as local | RED (bijection) |
| N3b | …with the tables made self-consistent, so **only the measurement can catch it** | RED (driven half) |
| N4 | `carriedAs` names a field `ConsensusParams` lacks | RED |
| N6 | verifier ignores `k`, so the probe goes dead | RED (divergence-map pin) |
| C1 | an 18th `ConsensusParams` field claimed by nobody | RED (committed but unowned) |
| C2 | a `chain.Config` field with no membership ruling | RED |

N3b exists because N3 was caught by the *structural* half, which would have left the driven half
unproven. A gate whose two arms are never separated cannot tell you which one is load-bearing.

### What this does NOT close, and one correction to the record

- `R-LIVENESS-RECOVERY-UNBOUND` — structurally unbindable, unchanged; it moved off the register
  2026-09-10 and is a disclosure in [`design/m0.md`](design/m0.md) 10.1, because it has no closer.
- `R-CONFIG-GATE-V5-REGIME` — the chain gate still validates a `Version: 1` block.
- **The carrier bound is NOT built here**, and its `|Anchors|` and `EpochBlocks` terms **depend on
  this wiring landing**: before it, those quantities were not committed anywhere, so a bound derived
  from them had nothing to read. No `Atts` cap is built either. Both are separate changes, separately
  certified.
- **Correction to the brief that ordered this work:** the foreign-genesis refusal was reported as
  still `LogDebug`. It is not. `core/node/chainrole.go` already logs `ErrForeignGenesis` at
  `LogWarn`, names the flags to check, and increments `ChainSyncForeignGenesis` — landed in
  `82fe56d`, confirmed by `git log -L`. Re-verified at source and left alone rather than rebuilt.

### Ordering that still stands

**Call A (the chain id in `consensusSigBytes`) must land in the SAME genesis move as F**, or the
graded re-run set is paid twice. That is the freeze's actual cost now — re-runs and calendar, not
permanence.

---

## D-CFGBIND-TIER-PROMOTION-2026-09-11 — the genesis bind promotes six compile-time defaults out of the Evolving tier, and the owner accepts it explicitly

- **Status:** ✅ RATIFIED — 2026-09-11, owner. The promotion is **accepted**, not narrowed, and the
  constraint it creates is written down here. Filed the same day owner call F's delivery merged, so
  the record and the behaviour land together.
- **Raised by:** the blind PE review of the call-F wiring
  (`silt-agent-memory/principal-engineer/reviews/RULING-owner-call-F-genesis-params-wiring-CODE-0d99aef-2026-09-10.md`),
  as *"the coupling the consult missed"*. It was routed to the owner rather than settled by the
  reviewer or the builder, because a tier reclassification is not a build decision.
- **Canon:** `docs/TENETS.md` Part IX now carries the PRINCIPLE (a value bound into a frozen
  consensus format leaves the Evolving tier for that network's lifetime). This entry is the build
  state that principle points at. `docs/build-process.md` rule 8 is the rule that motivated the bind.

### The finding, DRIVEN — not argued

`ConsensusParams` commits the consensus-critical config **by value** into the genesis block, so the
genesis hash covers it. The reviewer moved **one flag default** — `-quorum` from 3 to 2, no semantics
touched — rebuilt, and restarted the new binary on a chain the previous binary had minted, with the
operator's **argv unchanged**:

```
### v1 binary on the v1-minted chain (control) ###
serving; Ctrl-C to stop

### v2 binary (ONLY the -quorum flag DEFAULT moved 3 -> 2) on the SAME chain ###
silt: consensus config: REFUSING TO START — … 1 field(s) differ:
  -quorum: this node has 2, the chain's genesis commits 3
serving lines: 0
>> EXITED (refused)
```

A **pure binary upgrade** — no configuration change by anyone — refuses to start. That is correct
under canon rule 8: a value that changes a validity verdict must be a function of the chain, and this
is what "a function of the chain" costs.

### Which values are affected

Six inputs reach `ParamsFromConfig` through an *effective-value* helper that supplies a compile-time
default when the operator sets no flag: `effectiveQuorum`, `effectiveByzantineQuorum`,
`effectiveOperatorMargin`, `effectiveBondFloor` (`DerivedBondFloor`), `effectiveBondTTL`
(`DerivedBondTTL`) and `effectiveEpochBlocks`. For these six, the *build* is the operator: change the
default, ship the binary, and every node that upgrades disagrees with the chain it is on.

The other committed values are supplied by a flag the operator actually passes. They are frozen
per-network too, but changing them requires someone to change an argv, which is visible.

**`DerivedBondFloor` is CLEARED as a source of hardware-dependent divergence**, and that is the one
thing that would have made this severe. It is a compile-time constant —
`2 × (AntiReleaseComputeWindow/s × bond.PlotSealThroughput)` — and `bond.PlotSealThroughput` is a
literal in `core/bond/bond.go`, **not** a machine measurement, so two nodes on different hardware
derive the same floor and mint the same genesis. Verified at source, not assumed. It is frozen
per-network like the other five; it does not fork a network at mint time.

### One published sentence this falsifies, corrected in the same commit

`DerivedBondTTL`'s own doc comment read *"A tuning knob (Evolving), not a fixed law; a real deployment
can tighten it."* True of a network that has not launched. **False for one that has** — tightening it
and rebuilding refuses on every existing chain. The comment now says so.

### The two alternatives, both DECLINED, and why

1. **Narrow the committed set to the flags an operator actually sets.** Declined. It re-opens the
   17-in / 5-out membership analysis the certification settled, and it replaces the ratified rule
   (*every field that can change a validity verdict is bound to the chain, or is explicitly excluded
   with a recorded reason*) with an accident of which knobs happen to carry a flag today. Adding a
   flag to a value would then silently change its consensus status.
2. **Require explicit values, with no defaults at all.** Declined. It charges every operator, on
   every launch, to protect a case that **already fails safe**: the node refuses to start, loudly,
   naming the field and both values, instead of diverging silently. Paying a permanent usability cost
   to avoid a loud refusal is the wrong trade, and it does not even remove the class — an operator
   can still pass a different value.

### Why the safe direction is the reason this must be WRITTEN, not discovered

The failure mode of the promotion is a **refusal to start**, never a fork. That is exactly why it
needs a written rule: a loud, correct refusal that nobody expected reads as a bug in the release, and
the tempting fix is to weaken the check. The rule below forecloses that reading before it happens.

**The rule, and it lives in `docs/release-checklist.md`:** changing any of these compile-time defaults
is a **breaking change requiring a new network**, because every upgrading node refuses to start on the
existing chain.
