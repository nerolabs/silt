# M0 owned residuals — the held-in-tension ledger

**Status: canonical residuals reference + research-collaboration surface.** M0's honesty
rule (`m0.md` §8): a seam *held in tension with a documented residual* is a **pass**; a
seam *silently assumed closed* is the failure. This doc is the single place every owned
residual is named, classified, bounded, and given an open question — so the set is legible
at a glance and the research team has one surface to push on.

Each residual carries:
- **Class** — why it is not closed: **theorem** (an impossibility result forbids closing it
  **in silt's actual threat model** — reserved strictly for that; an over-hard `theorem` label
  is itself a failure mode, a permanent "stop looking" sign on a door the literature leaves
  open), **crypto-gap** (the construction exists but has no adoptable pure-Go impl — see
  [`primitive-availability-gaps.md`](primitive-availability-gaps.md)), **scope** (deliberately deferred past
  M0), or **research-frontier** (open problem, routed out).
- **What it is** · **Why not closed** · **How it's bounded today** · **What would close it** ·
  **Open question for research**.

Nothing here is load-bearing for the *shipped* M0 safety claims on the D-gated path (C1 no
discount, C2 no quiet capture, the demand→standing firewall) — those are held. These bound
*reach and tightness*, and each is labelled at the site it appears (`m0.md`, `TENETS.md`,
`decisions.md`, CHANGELOG).

---

## A. Sybil / concentration (C1 + C2)

### A1. The honest whale (C2 — held, not closed)
- **Class:** **definitional limit + open problem** (anchor: **Douceur**, IPTPS 2002 — a resource
  test *prices* Sybils, it cannot *bound* them without a logically-central authority). **NOT a
  settled impossibility** *(reclassed from `theorem (Kwon)` per the 2026-08-08 research audit —
  Kwon was mis-applied; see below).*
- **What it is:** a single *real* operator who genuinely provisions distinct disk across
  distinct network positions, pays the full `D × A × M` cost per key, and *then chooses to
  collude*. C1 (no discount) still holds — they paid honest cost — but C2 (no quiet capture)
  cannot count them as one operator, because aggregate operator identity is unobservable
  on-chain.
- **Why not closed (precisely):** no anchor-free Sybil cost is *known*, and a TTP identity is
  *sufficient* but **not proven necessary** — Kwon et al. (AFT 2019) explicitly leave *"the
  existence of mechanisms to enforce a Sybil cost in permissionless blockchains"* as an **open
  problem**. Kwon is NOT the load-bearing result: the honest whale pays full cost per key (his
  marginal Sybil cost is C=0, the regime Kwon *assumes*, not one it forbids escaping), and "a
  rich honest actor gets proportional weight" is definitional to any resource-weighted
  consensus, not a Kwon consequence. The honest anchor is **Douceur**; Kwon is a supporting
  "even-distribution is also unreachable without a Sybil cost" cite.
- **Bounded today by:** (1) the real dollar cost of disk × distinct AS positions per key;
  (2) the operator margin **M** (auto-armed > 1 for the untrusted posture); (3) the **A-axis**
  address-diversity gate — the shed counts distinct declared domains, so same-domain key
  splitting doesn't inflate the count — **but the declaration is trusted verbatim (no
  transport /24 cross-check anywhere), so this binds only truthful declarations: a rational
  splitter declaring distinct domains evades it for free, and the binding bound is (1)+(2)
  alone** (2026-08-15 re-price; red-team seam #5); (4) the **HHI / Gini / top-share
  concentration alarm** (out-of-band veto, measurement not enforcement). A dedicated survey found **nothing
  anchor-free beats "/24 diversity + economic bond"** (TEE attestation, proof-of-location,
  social-graph, super-linear bonding, proof-of-personhood all need a trust anchor / a social
  graph silt lacks / are just a price the whale pays) — so the *pessimism is correct*, only the
  old *theorem label* was wrong.
- **What would close it:** unknown — **not proven impossible**, just unsolved anchor-free. The
  one legitimate *mechanism* move (a hardening, not a new uniqueness bound): **PoRep-style slow,
  non-parallelizable sealing** so the per-key linear cost is genuinely real and
  non-compressible (no dedup / compression shortcut). An external proof-of-uniqueness layer
  would close it but is out of scope.
- **Open question (honestly open):** is there a *cheaper-than-personhood* uniqueness signal
  (beyond rentable /24 diversity) that raises the honest-whale cost further without a trust
  anchor? Very hard, nothing beats /24+bond today — but not foreclosed.

### A2. `M` is unverifiable on-chain (#182)
- **Class:** **open problem** (same Douceur root as A1 — an anchor-free Sybil cost is unknown,
  not proven impossible) *(reclassed from `theorem (Kwon)`)*.
- **What it is:** the operator-margin `M` (keys-per-operator inflation) that discounts the
  Nakamoto coefficient to `⌊k̂/M⌋` is a *declared constant*, not a measured quantity.
- **Bounded today by:** conservative default (> 1, auto-armed). (The A-axis does **not**
  make any of it *earned*: a declared domain is free and unverified — see A1's 2026-08-15
  re-price — so `M` stays a declared floor, not a measured one.)
- **Open question:** can telemetry (address/AS distribution, timing) give a *defensible
  lower bound* on `M` without an authority — or is a declared floor the honest ceiling?

### A3. Byzantine size-estimation under **adversarial** NodeID placement (#182)
- **Class:** research-frontier (least-supported load-bearing claim under C2).
- **What it is:** the CPR estimator (Chatterjee–Pandurangan–Robinson, arXiv:2102.09197) behind
  C2's Byzantine-robust sampling tolerates `O(n^{1−δ})` Byzantine nodes *when randomly placed*
  (a Chernoff bound over uniform ID samples); a stake-splitter *chooses* its NodeIDs, degrading
  the bound by an amount **the literature does not quantify**.
- **Not refuted by the 2022 follow-up:** the "adversarial placement" sequel (arXiv:2204.11951)
  models Byzantine *vertices in a fixed graph*, **not chosen NodeIDs clustered in a keyspace** —
  which is exactly silt's threat and remains open. (Guard against a reviewer waving it as a
  refutation.)
- **Bounded today by:** the DHT's `-dht-domain-cap` diversity + the A-axis; but the exact
  degradation is unquantified.
- **Open question:** quantify the CPR / size-estimation bound under *chosen* NodeID
  placement — or find a placement-robust estimator.

### A4. The γ→1/N shared-content sealing boundary (#182 — the single surviving economy of scale)
- **Class:** **research-frontier brushing a theorem** (the crown-jewel open problem — and a
  stated *moat*) *(strengthened per the 2026-08-08 research audit — "does not exist" is accurate
  and stronger than the doc said).*
- **What it is:** C1's direct-product cost bound assumes cross-identity independence (H3). H3
  fails on the disk axis *if* standing were ever granted for "I can prove I hold shard *s*":
  erasure coding means many honest nodes legitimately hold the same shards, so one physical
  copy could answer for N pledges and the disk axis collapses **γ→1/N**.
- **Why silt is NOT exposed today:** standing comes from a *dedicated identity-keyed bond
  plot* (`seed = H("silt/bond/plot/v3" ‖ pk ‖ n)`), so N bonds need N× distinct sealed disk —
  the disk axis and the served-content axis are deliberately **separate**. The bonded disk is
  "wasted" throwaway labels rather than useful served content. C1 is gated on this staying
  separate.
- **Why the separation is a MOAT, not an apology (the strengthening):** an identity-keyed
  sealing of shared useful data that is *simultaneously* transparent + timing/sequential-free +
  publicly-verifiable **does not exist — and the three constraints form a trilemma with an
  impossibility barrier**, so no competitor can fuse served content into standing for free
  either:
  - *transparent (no secret) but timing-free* → **Moran–Wichs (CRYPTO 2020)** prove
    incompressible encodings in the **plain model cannot be secure under any standard
    assumption** (black-box separation); the only transparent escape is the random-oracle model,
    and every ROM construction that also gives replica-distinctness (PIEs) **re-introduces an
    inherently-sequential (timing-flavored) encoding**;
  - *provable + timing-free* → you must add a **secret-key trusted dealer** (Damgård–Ganesh–
    Orlandi's RSA trapdoor; Garg–Lu–Waters) — fatal in a permissionless, content-blind network;
  - *fully public + timing-free* → **Fisch's characterization**: a PoRep "must rely either on
    rational time/space tradeoffs or timing bounds" — i.e. you can't.
  So silt's plot/content separation is the **correct response to a boundary the whole field
  runs into**, not a limitation it is apologizing for.
- **What would close it (and unlock fusing served useful content into standing):**
  identity-keyed **PoRep sealing of arbitrary useful shared data** meeting all three constraints
  — which, per the trilemma above, requires either a trusted replica-dealer or an
  inherently-sequential (timing) encoding.
- **Open question:** the highest-leverage one — a sealing construction that lets one node's
  served useful bytes count toward its identity-keyed standing *without* letting one physical
  copy answer for N identities, i.e. that escapes the Moran–Wichs / Fisch trilemma. (If none
  can, silt's separation is permanently correct — which is itself a publishable result.)

### A5. Partial-storage recompute discount on C1 — the disclosed `ε*` (BREAK 1)
- **Class:** **research-frontier** (Option A is the known close; H-track). Anchors: Fisch,
  *Tight Proofs of Space and Replication* (EUROCRYPT 2019); the DRSample space-time tradeoff.
- **What it is:** a red-team pass (2026-08-08) showed that on silt's **single-layer**
  DRSample+chain bond graph, a prover can **delete a fraction ε of the plot** (keeping the 32-byte
  Merkle leaves) and **recompute** any dropped block on demand from its predecessor + DRSample
  parents, then pass the exact `bond.VerifySpaceTime` the live wire runs — earning full standing
  for the *advertised* size while holding `(1−ε)` of the disk. Measured on the shipped graph:
  recompute is ~free at ε≤0.10 (depth ≈1), and the work explodes past the **~0.25 knee**. So C1
  as originally written (`(1 − o(1))`, a *vanishing* discount) is false; the honest statement is
  `≥ (1 − ε*)·q·C_honest` with a **disclosed `ε*` = 0.20**.
- **Why not closed (precisely):** recomputed bytes are **content-identical** to stored ones, so
  **no content check can distinguish them** (`verifyLabels` is a public, recomputable predicate) —
  verified against the code. Enforcement is therefore inherently the **time/sequential-depth leg**,
  not a content check. A *single-layer* depth-robust graph concedes a constant-fraction shave by
  construction; only a **stacked, multi-layer** tight-PoS construction (Option A) recovers the
  `(1 − o(1))` bound.
- **How it's bounded today (the residual is a gradient + a re-pricing, not a flat give-away):**
  - **Disclosed** `ε*=0.20` — the rational/serial attack point and the floor the graph concedes.
  - **Signalled (not gated)** against a **work-bound (serial/rational) disk-saver** past the ~0.25
    work knee by the **reply-latency signal** on the live bond challenge
    (`node.Config.BondMaxAnswerLatency`, daemon `-bond-answer-latency`, default 1.5 s): past the knee
    the recompute is a large sequential cost that shows up as reply latency. **This is a SOFT,
    disclosed deterrent — it does NOT deny standing** (build-immutable #3; the 2026-08-10
    network-durability research + #289). Gating on a single wall-clock reply is unsound on the open
    internet: reply-latency is transport (RTT + jitter + loss) **plus** compute, network delay is
    one-sided (it can only *add* latency), and gating on the sum read jitter/loss as a cheat and
    starved durability. So the node reads the **windowed-MINIMUM** (low quantile) of each peer's
    reply latencies — which filters the one-sided noise — and raises a **non-gating** suspicion only
    when that floor is *sustained* above the deadline (a partial-storage prover recomputes on every
    challenge → floor stays elevated; an honest bad-path node is only randomly slow). Standing itself
    rests on the sound signals (anti-release floor + identity binding + the space/labeling proof),
    and the anti-release floor is a **compute** window decoupled from the transport timeout (PR1), so
    the serial disk-saver is priced by the floor + audit frequency + the disclosed suspicion, not by
    a hard network deadline. The disclosed `ε*=0.20 ≤` the ~0.25 knee is conceded on the safe side.
  - **Priced (not free) against a parallel adversary.** A parallel prover is bounded by recompute
    **depth** rather than **work**, and can hold *less* disk — but it **re-pays the recompute on
    every audit, per identity, forever** (a compute-for-storage re-pricing, silt's "re-priced, not
    prevented" idiom, *not* a free discount). By Brent's theorem the parallelism required to reach a
    given ε grows **super-exponentially** (`cores ≥ work/depth`: ~10² cores to be depth-bound at
    ε≈0.25, ~10⁵ at ε≈0.30, ~10¹³ to approach ε≈0.5). So the **realistic** parallel exposure is
    ≈ ε0.30 (compute-repriced), and the ε≈0.5 depth knee is a theoretical unbounded-adversary bound,
    not a real attacker.
  - **Audit frequency is the free tightening lever** (`-bond-audit`): the parallel attacker's
    recurring compute bill scales with it, with no construction change.
  - **Composes with BondTTL:** on-chain objective weight lapses without live re-proof, so standing
    over time is bounded by BondTTL + the sound live re-verify (space/labeling proof + anti-release
    floor), not by the (now soft) latency signal (the on-chain verify cannot time a stored proof —
    that tightness is Option A).
- **What would close it:** **Option A** — a stacked multi-layer expander (à la Filecoin Stacked-DRG,
  L≈10) turning depth-robustness into a *global* depth guarantee, made on-chain-succinct by a
  Groth16 SNARK over the ~100 MB witness. It re-imports a **trusted setup** and is far outside an M0
  hotfix ⇒ **H-track**.
- **Open question for research:** a tight (parallel-secure, small-`ε*`) proof-of-space-time
  affordable on-chain in **pure Go without a trusted setup** — i.e. Option A minus the SNARK/setup
  cost. Until then, `ε*=0.20` disclosed + the compute-window anti-release floor + the **soft**
  (non-gating) sustained-latency signal + the audit-frequency lever is the M0 hold. A separate
  finding from the same work: the bond proof reply is ~1.5 MB, a loss-sensitivity (#289) and N²
  bandwidth residual whose structural close (succinct proof) is also H-track — [#299].
  Cross-ref: [`m0-sybil-rebind.md`](m0-sybil-rebind.md) §8.1 (the `ε→k` derivation, confirmed H-track).

---

## B. Time and demand (T + B axes)

### B1. T-axis acquisition (relabelled — retention only ships)
- **Class:** scope (deferred M1+) + design (an age gate is unsound).
- **What it is:** `C_honest ∝ D×A×T×B` wants time to cost. **Retention** ships (decay/TTL forces
  continuous re-proof). **Acquisition-time accrual does not** — standing is granted in full on
  the first passing bond challenge, priced by D alone.
- **Why not built:** a bare `firstSeenTick` age gate is *pre-farmable* (the coin-age
  anti-pattern — Peercoin's CAA attack; NeuCoin removed coin-age). The only sound form is a
  **continuous VDF chained to the bond identity** — an M1+ construction (see
  [`primitive-availability-gaps.md`](primitive-availability-gaps.md) §5).
- **Open question:** is a bond-anchored continuous VDF worth its always-on cost over an
  already-non-substitutable D axis, or is D-only acquisition the right permanent answer?

### B2. Demand authenticity — a Douceur limit (D-DEMAND)
- **Class:** **SPLIT** *(per the 2026-08-08 research audit — Douceur was over-applied to the
  detection half)*: requester-**distinctness** = genuine **theorem (Douceur)**; wash-**detection**
  = **re-pricing** — a monitored economic condition, **not an impossibility**.
- **What it is:** a demand receipt proves *service happened*, not that the requester was a
  distinct honest party (Douceur forbids telling distinct identities from one entity by resource
  testing, no authority). But wash-**detection** — the *same* party on both ends — is a
  payment/collusion problem, **re-priceable and partially monitorable**, not proven-undetectable
  (the doc's own "bounded by cost-to-wash" text *is* a re-pricing argument, which contradicts a
  `theorem` framing on the detection half).
- **Bounded today by:** **cost-to-wash** re-pricing — fee-burn per receipt + a bonded-fetcher
  credential (demand counts distinct bonded fetchers, so washing costs one on-chain storage
  bond per faked unit). Demand is a **neutral** observable (the firewall): forged demand buys
  **zero** consensus standing, so authenticity is not load-bearing for M0 safety.
- **Open question:** the anti-wash inequality `c_wash > c_real` is a monitored economic
  condition, not a proof — is there a tighter, still-permissionless requester-distinctness
  signal?

### B3. Demand-receipt residual leaks (seam-4)
- **Class:** scope (neutralized by the firewall today; must close before any demand→standing fusion).
- **What it is:** a receipt is forgeable with **zero** object bytes (the per-object PoR seed is
  public), and a bonded-mode receipt links fetch→standing key to one validator.
- **Bounded today by:** the firewall — demand has no consensus consumer, so both are inert.
- **What would close it:** needed only *if* B is ever fused into standing (γ→1/N territory, A4).

### B4. Publisher signer-subset (seam-4 — canonical-set holds subset-anonymity)
- **Class:** scope (M0 hold shipped for the reported leak; the stronger crypto close is H8).
- **What it is:** the committed `PublishToken.Sigs` records each signing validator's NodeID, so a
  root's signer subset is a public quasi-identifier. A red-team pass (2026-08-08) showed the shipped
  `swarm add` chose an *arbitrary* subset of the caller's `-peers`, so a distinctive subset could
  collapse a publisher's anonymity set toward a singleton.
- **Bounded today by (R-3, shipped):** `swarm add -token-quorum` now selects signers by a
  **network-canonical ledger ordering** (validators ranked by committed bond, fetched from a
  chain-holding peer via `MsgGetCanonicalIssuers`), the SAME for every publisher — so the subset
  stops being a per-publisher identifier (advantage → 0 for the reported leak at stated parameters).
- **Caveats to own:** (1) it holds **subset-anonymity only** — the fetcher IP/timing channel is the
  separate D-PRIV residual (C1), unchanged; (2) a canonical top-k quorum is a mild **publish-liveness /
  censorship surface** (those validators must be online and willing to sign) — acceptable because the
  set rotates by committed bond, not a fixed cabal, but named honestly; (3) **reachability** — a
  chainless publisher ranks its *reachable* peers by the canonical ordering, so the hold is fully
  global only when publishers connect to the canonical validator set.
- **What would close it (fully):** the **B2 blind-signed publish token** (issuer signs without learning
  which root it authorized), which severs the on-chain signer-subset quasi-identifier at the crypto
  layer so even a non-canonical subset leaks nothing — the H8 privacy-track target (#179).

---

## C. Privacy (D-PRIV)

### C1. Fetcher metadata (IP + timing) unlinkability — D3/H8
- **Class:** scope (post-M0 H8) + theorem **under a global adversary only** *(reclassed from a
  plain `theorem` per the 2026-08-08 research audit — the trilemma is proved against a global
  passive adversary, which silt says it does not face; this was the doc's strongest premature
  surrender).*
- **What it is:** D3 issuance-mixing severs the *identity* + *payment* link (ephemeral key +
  blind credit + relay), verified over real TCP. A residual **transport IP + timing** link
  remains until epoch-batching / a mixnet ships.
- **Bounded today by:** the relay hides the fetcher IP from the issuer; timing-correlation is
  the residual. Deferred to the **H8 mixnet + PIR-DHT** track.
- **Why bounded, not closed — and why the theorem does NOT fully bind silt:** the anonymity
  trilemma (Das et al., IEEE S&P 2018) proves its lower bound specifically against a **global
  passive (network-level) adversary**. silt's stated threat model is a **small paid network, NOT
  a global adversary**, so the trilemma does **not directly bind silt's setting** — the door the
  old `theorem` label marked closed is actually **open**. Against silt's bounded-adversary model
  the achievable anonymity set is an **open, likely-improvable** quantity, and the H8 mixnet/PIR
  track may legitimately target a stronger guarantee than "choose two." (If silt ever concedes a
  global adversary, the full `theorem` label returns.)
- **Open question:** the metadata-layer anonymity-set achievable on a *small* paid network
  (silt's actual model) without a latency/bandwidth tax users won't pay — a genuinely open,
  improvable target, not trilemma-closed.

---

## D. Durability & takedown

### D1. Bandwidth-blind proof-of-repair (H7)
- **Class:** crypto-gap (see [`primitive-availability-gaps.md`](primitive-availability-gaps.md) §1).
- **What it is:** M0 ships the Merkle-recompute floor (fetch k survivors, recompute, compare) —
  sound and content-blind but **not bandwidth-blind**. The blind form needs a char-2-native
  polynomial commitment (FRI-Binius) with no pure-Go impl.
- **Open question:** an F_p storage re-encode vs. waiting for a char-2-native commitment lib.

### D2. Tag-forgery on public per-object PoR keys (H7 — inert)
- **Class:** scope (documented H7 non-goal, inert under neutrality).
- **What it is:** a caretaker holds the layout key and can forge valid SW tags for *wrong*
  bytes; the recompute leg (re-derives correct bytes from survivors) closes it for repair, but
  the tag alone is forgeable.
- **Bounded today by:** the recompute correctness leg; and PoR standing is not fused into
  consensus.

### D3. MSR / regenerating-code proof-of-repair (off the critical path)
- **Class:** research-frontier (off critical path).
- **What it is:** the A1 proof-of-repair composition is airtight for *plain-RS* (what silt
  ships); no published construction specializes it to MSR/Clay regenerating codes.
- **Needed only if:** silt later adopts regenerating codes.

### D4. Cold-data solvency — finite-but-renewable (g > 0)
- **Class:** scope (instrument-first).
- **What it is:** perpetual durability solvency exists only when the cost-trend `g > 0`
  (declining cost per repair). M0 ships **finite-but-renewable** durability and instruments `g`.
- **Open question:** the parameter region where the internal credit reserve stays solvent
  across realistic cost trajectories.

### D5. Provable non-globality of takedown — ZK threshold predicate (D-TAKEDOWN)
- **Class:** **research-frontier + trusted-setup design choice** — **NOT a missing library**
  *(reclassed from `crypto-gap` per the 2026-08-08 research audit; see
  [`primitive-availability-gaps.md`](primitive-availability-gaps.md) §4).*
- **What it is:** M0 ships the CT-style transparency log (provable *recording* of every
  takedown, inclusion + consistency proofs). The stronger *survivor-Nakamoto* metric — a ZK
  predicate "≥ t distinct-domain PoR-fresh replicas are gone".
- **Why not built (precisely, not "no pure-Go ZK"):** **gnark** (Consensys) IS mature pure-Go
  (~9 audits, powers Linea) and *can express* a "≥ t of N" predicate — so this is not a library
  gap. It is blocked by (a) a **trusted-setup design tension**: gnark ships only Groth16 +
  PLONK-KZG, both needing a ceremony, and there is **no audited transparent (STARK/FRI) pure-Go
  backend** — and for a censorship-resistance metric, "who ran the ceremony" is itself a capture
  vector; (b) the genuinely unsolved **circuit/witness design over distributed, freshness-attested
  replica state** — what is the witness for "≥ t replicas are *gone*"? absence/liveness isn't
  locally attestable; freshness must bind to a recent epoch (no stale-proof replay); distinctness
  must be non-Sybil-gameable. **Actionable upside: silt is not blocked by a missing library — it
  could start the circuit/witness design now.**

---

## E. Consensus & bootstrap (F-1 fallout)

### E1. Bounded re-centralization after the maturity latch (F-1)
- **Class:** theorem (weak-subjectivity irreducibility) — the deliberate trade.
- **What it is:** once matured, silt never re-arms the launch anchors; a genuinely
  re-concentrated *real* bonded set keeps committing under the real-bond super-quorum, caught
  only by C2 + the A-axis + the alarm, never by anchors.
- **Why accepted:** it trades the (eliminated) permanent-center and undefined-halt risks for a
  **bounded, socially-recoverable** re-centralization risk — the same trade Ethereum, Cosmos,
  and Bitcoin all made retiring their training wheels. Irreducible for any weakly-subjective
  system.

### E2. Weak-subjectivity dependency (F-1 — newly explicit)
- **Class:** theorem (structural to all proof-of-stake-class systems).
- **What it is:** a node syncing from genesis or long-offline cannot distinguish the real
  matured chain from a forged long-range one on chain data alone, so it **must** be pinned to a
  recent trusted **weak-subjectivity checkpoint** (`-ws-checkpoint`) within the WS period
  (~`BondTTLBlocks` + slashing depth).
- **Open question:** the exact WS period derivation for silt's turnover/eviction dynamics, and
  checkpoint *distribution* tooling (explorer endpoints, client-bundled checkpoints) — post-M0.

### E3. Reachability of maturity (held in tension)
- **Class:** scope (a parameterized bet, not a theorem).
- **What it is:** M0's Sybil soundness is conditional on the mature regime being reached before
  the young, anchor-scaffolded regime is captured. No proof — a safe-parameterization (plural
  threshold anchors + the one-way latch), with levels that must track live telemetry.

### E4. Bonded-minority liveness-denial in the mature phase (the liveness dual of A1)
- **Class:** **held in tension** — the BFT liveness bound every weakly-subjective system lives
  with, *priced and bounded*, not a silt defect. (PE ruling 2026-08-13, §2; research
  certification 2026-08-13 B2, which **held this entry until the pricing below became true in
  code** — see the enforcement leg.)
- **What it is:** in the **mature** phase (bond-weighted BFT, anchors shed), a cohort that
  *honestly* banks ≥⅓ of committed bonded weight **can stall finality** — not capture it. Any
  BFT quorum needs a >⅔ super-majority to commit, so a ≥⅓ bonded minority that refuses to attest
  denies liveness (no new block finalizes) while it holds.
- **How the ⅓/⅔ is enforced — weight, never heads (the B2 catch).** This pricing is only true
  because the mature-phase quorum is counted in **frozen epoch bonded WEIGHT**
  (`chain.requireEpochWeightQuorum`: a commit's coalition must carry >⅔ of Σ `epochSet`).
  Until 2026-08-13 it was **head-counted** (`bftThreshold(len(epochSet))`), and since epoch
  admission is deliberately unfiltered (Condition A), every MinBond identity seated at rotation
  weighed one head: 8 cheap members among 4 honest made the mature phase *born unable to commit*
  (stall at 8×MinBond), and 9 made a cohort-only commit valid with **zero honest attestation**
  (capture at 9×MinBond, persisting into full maturity) — a C1-discount + C2-quiet-capture break
  found by the PE addendum and escalated by research, **not** by the external red team. Both are
  now refused-by-construction and pinned by failing-first drills
  (`core/chain/quorum_weight_test.go`: capture refused `ErrNoQuorumWeight`, honest weight commits
  through a declining cohort, strict->⅔ boundary).
- **Why it is not a break, on three legs:**
  1. **Priced (C1).** The ⅓ is ⅓ of *real, sealed, address-diverse* disk — and it **decays**:
     bonds lapse without continuous re-proof (retention TTL), so the griefing cost is not one-time
     but *recurring*, and the griefer earns nothing while stalling. This is the honest-whale cost
     (A1) applied to liveness rather than capture.
  2. **Bounded + surfaced (C2).** The concentration metric (`C2Metric`: Nakamoto over
     bond-distinct operators/domains, HHI/Gini) makes a ≥⅓ concentration **loud** out-of-band —
     the same alarm that watches for capture watches for stall-capable concentration.
  3. **Safety preserved (D-1).** Under the stall the trust plane **halts rather than reorgs**
     (prefer-stall-to-reorg); no conflicting finalization, and the storage plane keeps serving
     (D-2). A stall heals when the minority re-attests or its bond decays out; a reorg would not.
- **Scope — the load-bearing distinction:** this residual is **mature-phase ONLY**. The **launch
  phase provably has neither capture nor stall from un-matured bonds**: `validatorSetSize()`
  returns the *fixed anchor set* until the finalized handoff, so an un-matured bond **neither
  votes nor counts in the fault budget** — it banks standing while onboarding, nothing more
  (verified: `core/chain/TestFaultToleranceBranch_SybilBondsDoNotInflateLaunchQuorum`, which also
  showed the SYBILS=8 `6-fault-tolerance` GAP is gather-*latency* under load, not a quorum-sizing
  bug). The consult's original "no regime between young and handed-off where an un-matured bond
  acquires stall power" was **half right**: true up to the handoff, **false at the handoff
  instant under head counting** — an un-matured cohort needed only to *ride along* into the first
  epoch snapshot to acquire per-head quorum power (the B2 catch above). With the weight-counted
  quorum the claim holds end-to-end: before handoff a cheap bond neither votes nor counts; after,
  it votes exactly its weight.
- **What the red team should attack:** the *price*, not the possibility (brief seam #8,
  stall-griefing) — can a cohort acquire ≥⅓ bonded weight for materially less than ⅓ of honest,
  sustained, address-diverse provision, or evade the C2 concentration alarm while doing so?

### E5. Consensus-frame FIFO starvation behind the inbound cap under a within-share bulk flood (v2b — sequenced, not shelved)
- **Class:** scope (deliberately sequenced behind Phase 1.2 + a drain-rate measurement; PE-ruled
  2026-08-19). Two rulings govern:
  `silt-reviews/principle-engineer/RULING-v2b-consensus-reserve-approach-2026-08-19.md` and
  `RULING-v2b-drill-RED-drain-not-gate-2026-08-19.md` (full paths under
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/`).
- **What it is:** an authenticated sybil cohort — every member inside its v2a per-peer share —
  fills the global `-inbound-cap` with bulk frames; because the gate releases only when the
  single loop *finishes* a message, gate-full means ≈ cap bytes of bulk already sit in the FIFO
  loop queue, and a validator's consensus frame — however admitted — processes behind
  ≈ `cap/drain` of it. Demonstrated by the committed timed drill (branch
  `drill/v2b-gate-starvation` @ 84b2788, `adapters/tcpnet/reserve_drill_test.go`, parked RED):
  uniform ~4.1 s vs the 2 s saturation bound at a scale-model cap, measured latency matching the
  analytic `cap/drain` within 1%. Design + cost model:
  [`../thinking/2026-08-19-v2b-gate-starvation-drill-design.md`](../thinking/2026-08-19-v2b-gate-starvation-drill-design.md).
- **Why not closed now (the drain ruling):** the drill proved an admission-side reserve is
  **insufficient** (admission ordering cannot fix a processing-ordering problem), and the severe
  regime — `cap/drain ≈ 128 s` at the shipped 256M — **requires drain pinned at ~2 MiB/s, which
  is the bond-reg/VDF CPU-flood regime: Phase 1.2's domain.** The starvation and the CPU gate
  are two faces of one resource (loop drain time); bounding per-message CPU raises the
  denominator and most of the severe case is expected to evaporate with zero event-loop change.
  Building a hottest-path (B2) two-class drain against that unmeasured, about-to-change
  denominator would be measuring on sand.
- **How it's bounded today:** the cap converts the flood to latency, never OOM (alive > crashed);
  v2a confines any single peer to 1/4 of the budget; the `-inbound-cap` sizing note (flag help)
  makes the OOM-headroom vs `cap/drain`-latency trade legible so an operator can size for the
  expected-worst drain; and consensus timeouts/rounds retry — delay, not permanent starvation.
- **What would close it (the reach-recipe + go/no-go, in order):**
  1. Phase 1.2 (`MsgSubmitBondReg` CPU gate) lands — raises the flood-drain floor.
  2. Rider on 1.2 validation: **measure the real saturation drain rate at 256M** under the
     existing flood/soak (a laptop measurement, not a billable run — build-immutable #6), and
     **re-run the committed drill re-parameterized to it**. That re-run is the go/no-go.
  3. Only if still RED: build the **one two-class mechanism** (identity/kind → class →
     **priority drain**; the admission reserve and the drain priority are two faces of the same
     class label — supersedes the admission-only structure). Merge oracle = this drill GREEN
     **plus** a second, non-negotiable oracle: bulk/repair does NOT starve indefinitely under
     sustained consensus load (BOUNDED priority — absolute priority would break I4's
     no-permanent-starvation for the storage plane). Not research-gated (drain order changes
     latency, not consensus outcome — build-immutable #3 untouched), but the full
     sim/model-check must stay green.
- **What the red team should attack (#183):** the reach — can a real cohort pin a production
  node's drain into the slow regime at the shipped cap *after* the Phase 1.2 CPU gate? The
  trigger recipe is above; this residual is pre-designed and can never be a surprise.

---

## F. Primitive-availability gaps (the "would adopt if it existed" set)

Five *ideal* constructions silt would adopt, enumerated with their **true binding constraint**
in [`primitive-availability-gaps.md`](primitive-availability-gaps.md) — and, per that doc's
own correction, **only one or two are actually pure-Go *library* gaps**: (1) char-2-native
polynomial commitment (blind PoR) — *immature everywhere*, B8-blocked in any language;
(2) threshold **decryption** (fair-exchange dispute) — a real library gap, but DKG + threshold
*signing* are mature pure-Go; (3) verifiable encryption — niche everywhere, *and possibly
replaceable* by Paillier range proofs; (4) ZK threshold predicate (= D5) — **NOT a library gap**
(gnark is mature pure-Go); blocked by trusted-setup posture + circuit design; (5) continuous
identity-chained VDF (B1) — deferred on merit, no impl anywhere. Each ships a sound floor.

---

## The through-line

Every residual above is either **impossible to close in silt's actual threat model** (a named
theorem), an **open problem** (winnable-or-milder, not foreclosed), **gated on a primitive with
no adoptable pure-Go impl**, or **deliberately scoped past M0** — and each is *labelled at the
site it appears*, not silently assumed closed. That labelling is the M0 deliverable.

**Theorem-class discipline (research audit, 2026-08-08).** A `theorem` label is a permanent
"stop looking" sign, so it is reserved *strictly* for impossibilities that bind **silt's stated
setting**. The audit found the label had drifted onto **open problems** (A1/A2 — anchor-free
Sybil cost, which Kwon leaves explicitly open; B2's wash-*detection* half) and a **wrong-adversary
result** (C1 — the anonymity trilemma is a *global*-adversary theorem, and silt says it faces a
bounded one). Those are reclassed. Net effect: silt keeps every sound floor **and** stops telling
itself that four winnable-or-milder problems are closed by theorem. Nothing here is under-owned —
no residual is riskier than stated.

**Highest-leverage:** **A4** (γ→1/N sealing) is now stated as a **moat brushing a theorem** — no
competitor can fuse served content into standing for free either (Moran–Wichs + Fisch); if the
trilemma is truly unescapable, silt's plot/content separation is *permanently* correct, a
publishable result. **A1/A2** (a cheaper-than-personhood anchor-free uniqueness signal) is
honestly *open* — very hard, nothing beats /24+bond today, but not proven impossible. Research
team: push hardest there.
