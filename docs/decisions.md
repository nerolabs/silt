# Decision ledger

**Status: living record.** The product/strategy decisions silt's owner has made, why,
and what remains open. Each entry separates the **direction** (a decision we can and did
derive) from any **construction** (a primitive that must still be built or researched).
This exists so decisions stop being invisible — a reader (builder, red team, researcher,
user) can see exactly what is settled, what is deferred, and on what basis.

**How these were decided.** The research package (`silt-reviews/research/research-outcome/`,
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
  its withdrawal. **Wired into the node** (`core/node/demandrole.go`): a fetcher `AcquireDemandToken`
  over the existing token-request wire, then `SubmitDeliveryReceipt` (a `MsgDeliveryReceipt` carrying
  the token + PoR-bound ack) to the server, which banks it into a **neutral witnessed-demand
  observable** (`WitnessedDemand`) — never standing; replays and forged/mis-issued tokens are rejected
  over the wire. **◑ P2 optimistic fair exchange — the abort-SAFETY floor is built + regression-locked
  (`core/demand/fairexchange.go`); the dispute-RESOLUTION half is gated on threshold crypto silt does
  not ship.** Built: the ASW optimistic phase (`ExchangeCommitment` — a fetcher's pre-release,
  non-repudiable promise) + both abort-safety properties, which hold structurally today — (1)
  fetcher-side: an aborted exchange never CONSUMES the token (spent only by a completed Redeem), so a
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
- **Basis:** the PE process review (`docs/reviews/builder-process-notes-PE-2026-08-14.md`)
  and the research team's **independent same-day convergence** on the same diagnosis
  (`silt-reviews/research/research-outcome/INTERSECTING-QUORUM-INVARIANT-note.md`, plus the
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
  (`silt-reviews/principle-engineer/D-TIERING-design-direction-2026-08-25.md`), verified
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
     (`silt-reviews/research/D-TIERING-state-root-keystone-CONSULT-2026-08-25.md`); mode
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
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`).
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
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era3-committed-state-root-format-RESEARCH-CERTIFICATION-2026-08-28.md`;
  PE ruling
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era3-committed-state-root-format-2026-08-28.md`;
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
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era3-committed-state-root-format-BUILT-RECERTIFICATION-2026-08-29.md`;
  the design certification it re-certifies,
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era3-committed-state-root-format-RESEARCH-CERTIFICATION-2026-08-28.md`).
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
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witnessable-transitions-RECERT2-2026-08-29.md`,
  **CERTIFIED-WITH-CONDITIONS**; prior passes it supersedes:
  `.../era4-witnessable-transitions-EQUIVALENCE-RESEARCH-2026-08-29.md` and
  `.../era4-witnessable-transitions-RECERT-2026-08-29.md`; PE ruling
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era4-witnessable-transitions-2026-08-29.md`;
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
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C1-maturity-before-capture-RESEARCH-CERTIFICATION-2026-08-27.md`).
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
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27.md`).
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
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C5-honest-operator-economics-composition-RESEARCH-CERTIFICATION-2026-08-27.md`).
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
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-600-floor-box-direction-2026-08-28.md`;
  research note
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/600-floor-box-direction-post-coexistence-RESEARCH-NOTE-2026-08-28.md`;
  C-7 certification
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`;
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
     `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witness-floor-box-readset-v5-AMENDED-RESEARCH-CERTIFICATION-2026-08-30.md`
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
     `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/535-recovery-boundary-disposition-RECONCILIATION-2026-08-30.md`
     and
     `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RECONCILIATION-floorbox-livenessrecovery-boundary-2026-08-30.md`.
- **RATIFIED 2026-08-31 — the v5 floor-box recompute DIRECTION and the R-boundary POSTURE
  (HEAVY / fully-trustless).** Two owner ratifications on the lane-1 floor box: how the recompute
  closes completeness, and how far it validates.
  1. **Floor-box recompute DIRECTION RATIFIED (Andrew, 2026-08-30).** The v5 floor-box recompute
     closes completeness by **MTH-reconstruction** — the `dueBucket` pattern: witness the id-list,
     recompute the Merkle Tree Hash, require it equals the committed digest. **NO new cryptographic
     primitive; NO `dueBucket` format change.** The direction reuses the existing committed-digest /
     recompute machinery. Cite:
     `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-v5-floorbox-bounded-recompute-CRUX-RESEARCH-CERTIFICATION-2026-08-30.md`.
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
       `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RECONCILIATION-lane1-Rboundary-root-count-2026-08-31.md`
       and
       `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-v5-Rboundary-Rscope-RECONCILIATION-2026-08-31.md`.
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
     `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-recompute-P1a-Opayload-multileaf-RESEARCH-CERTIFICATION-2026-08-31.md`
     and
     `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-recompute-P1a-Opayload-multileaf-2026-08-31.md`.

---

## D-POD-KNOBS — the three Phase-4 economy/state knobs, decided

- **Status:** ✅ DECIDED — 2026-08-26 (owner ratification: "PE answers are mine as well").
  All three were held open by the two 2026-08-26 certifications as *owner scope*; the PE
  attached an engineering recommendation to each, the owner adopted them.
- **Basis:** the certifications
  (`silt-reviews/research/research-outcome/PoD-neutral-lane-B3-close-RESEARCH-CERTIFICATION-2026-08-26.md`,
  `…/D-TIERING-state-root-keystone-RESEARCH-CERTIFICATION-2026-08-26.md`), the consult
  (`silt-reviews/principle-engineer/PoD-keystone-owner-knobs-CONSULT-2026-08-26.md`), and
  the ruling
  (`silt-reviews/principle-engineer/RULING-PoD-keystone-owner-knobs-2026-08-26.md`), whose
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
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/PoD-7.3-free-vs-paid-relay-coexistence-RESEARCH-CERTIFICATION-2026-08-30.md`),
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
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/v5-wholeset-digest-root-addition-RESEARCH-CERTIFICATION-2026-08-31.md`),
  the PE cert cross-check
  (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-v5-wholeset-digest-root-cert-crosscheck-2026-08-31.md`),
  and the read-set enumeration
  (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-readset-v5-quorum-wholeset-enumeration-2026-08-31.md`).

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
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/A4-provisional-eviction-conservation-RESEARCH-CERTIFICATION-2026-09-01.md`.
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
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4-receipt-expiry-scoping-RESEARCH-CERTIFICATION-2026-09-01.md`.
- **Open residual — RT-DELIV-3:** the delivery-credit `provKey` omits the server, a latent
  conservation break reachable only in the shared-ledger *sim* (not per-node prod). The
  next-session decision: fix now (add the server to `provKey`, which changes the conserved-lane
  key shape → **re-opens this R0.4 cert**) vs track as a sim-only residual.

## D-FLOORBOX-WITNESS-SOUNDNESS — Resolve-anchoring is the correct direction; fold-equality is NOT a universal backstop

- **Status:** ✅ RATIFIED — 2026-09-01 (owner ratification). Two linked research verdicts on
  the floor-box witness-soundness spine (Boulder 1), certified together:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-R1.3-refutation-R1.4-witness-soundness-RESEARCH-CERTIFICATION-2026-09-01.md`.
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
