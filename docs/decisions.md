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
  1. **ROADMAP.md carries an explicit ORDERED path with phase gates** ("The ordered path").
     "Tenets are the roadmap" was too loose — it let effort pool on one axis with no
     rebalancing force. Tenets remain the destination; the ordered path is the track.
  2. **The prior sequencing rule "M1 opens only after the M0 gate" is SUPERSEDED.** The
     economy-enablement and height-cost work interleave with the M0 tail, because (a) field
     runs stall short of depth (h64) partly on M1 costs — heavy per-reg proofs, round
     durations, the 360 s publish bound — so cheaper heights are the path TO the M0 field
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
- **Immutables preserved:** consensus engine untouched; M0 firewall reinforced; the
  hobbyist box (#8) is the point of the whole direction; content-blind core untouched (the
  root commits locators and status, never content meaning).

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
