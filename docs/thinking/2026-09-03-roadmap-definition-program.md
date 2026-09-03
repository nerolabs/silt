# 2026-09-03 — Roadmap definition program: designing and certifying the undefined Rocks

**Purpose.** On 2026-09-03 the owner asked for every Rock in `ROADMAP.md` that has no chosen
direction, no design, or no certification to be put through the design-and-certification
process, so the roadmap carries definitions rather than open questions. This document is the
PACE record (options → decision → rationale) for that program and the index of the rulings and
certifications it produced. It ships in the same PR as the ROADMAP updates it justifies.

**Discipline.** Seats judged blind (artifact + question, never a rationale). Research-gated
items (consensus rules, published claims, economic mechanisms, security parameters) went to
the Researcher for a CERTIFIED / GATED / REFUTED verdict; the PE ruled on engineering shape
and sequencing; the crypto-specialist and economist advised; the red-team hunted. All seats ran
read-only on the box (no whole-tree Go commands; the one-heavy-agent gate hook and the
`GOFLAGS=-p=2` / `GOMAXPROCS=4` caps were live). The owner ratifies; nothing here decides an
immutable.

## The work packages

| WP | Rocks | Seats (in order) | Status |
|---|---|---|---|
| A | Carrier family: R-CARRIER-PARENT-BINDING, -CREDIT-DENIAL, -DOUBLESIGN-SLOT, -ROLLOUT-SIGNAL, -BYTES | crypto-specialist advisory → Researcher composed cert → PE blind ruling | landed |
| B | R-STATEVIEW-ENUMERATION (freeze precondition) | PE inventory → red-team blind hunt for a missed read → Researcher closure cert | landed |
| C | R-membership; recovery boundary (formerly #535) | PE options ruling → Researcher cert → owner | landed |
| D | FP-2 Direction 1; R2.13 R-COMPACT-ORPHAN; R2.10 F8 | PE design ruling → Researcher on the gated pieces | landed |
| E | Boulder 2: R2.9, R2.12, R2.4, R2.7 (scope), R2.8 | economist advisory → Researcher cert (R2.9 is D-POD-KNOBS, cert-gated) → PE sequencing | landed |
| F | R4.2 A-axis into standing | crypto-specialist prior art → Researcher direction cert → PE | landed |
| G | R-ISSUERKEY-POP; R-E2E-ERA4-FIXTURE; SMT keyspace-injectivity oracle | PE small-designs ruling | landed |

## Outputs (filled in as each seat reports)

Every seat output is listed below in landing order, with the load-bearing findings. The consolidated owner-ratification list lives in `ROADMAP.md` (the ★★ block under *Decisions owed*).

### WP-D — ledger durability family (PE ruling, landed)
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-ledger-durability-family-FP2-R2.13-R2.10-2026-09-03.md`
- Sequence **R2.13 → R2.10 (F8) → FP-2**. Do NOT build FP-2 to the brief as written.
- FP-2's brief names the wrong invariant: there is no double-pay (`r04b_c3_crashwindow_test.go:84-89`, Σ unmoved). The residual is a LOST SUPERSEDE (`delivery.go:449-454` sits outside the `{guard entry, payout}` atom). The atom is the whole redeem (5 mutations).
- "Chain-anchored ≠ monotone": legacy `heavier` can lower `chainEpoch()`; F8 must RE-SOURCE the monotone latch, not delete it. `-epoch-blocks 0` bricks the lane at 65,536 paid deliveries (sweep never fires).
- R2.13: open-before-rename + a missing port clause at `ports/ports.go:361-363`; do NOT blanket fail-closed at `delivery.go:559`.
- Coupling missed by the brief: persisting the ledger BEFORE F8 makes the watermark poison PERMANENT. F8 gates the BUILD.
- Live doc defect: `ROADMAP.md:548-549` ("pure under-pay … owes no gate") is false on all three clauses.
- Research-gated: F8's close; epochs-disabled denomination (security parameter); FP-2 atom boundary + replay idempotence (conservation). Owner call: whether the ledger gets a durable store before RC at all (PE: close FP-2 by scope; land R2.13 and R2.10 anyway).

### WP-E — Boulder 2 economy (economist advisory, landed)
`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-boulder2-economy-definitions-2026-09-03.md`
- **Reorders Boulder 2:** G-4 fixed refuse-beats-accept; SUPPRESS-beats-accept is untouched. Server payoff `0.875·B` for never banking a receipt vs `0.875·fee` for banking — 1,342× at 64 MiB — and suppression is `-accept-delivery-receipts=false`, the shipped default.
- Flat fee flattens the skim: self-funding needs 32,212 retrievals/repair at 64 MiB vs D-S7's `S/R ≥ 24`.
- R2.9 → PayWord-denominated per-increment delivery settlement (relay lane precedent `relay.go:63-77`); invariants INV-P1 `p ≥ r·U`, INV-P2, INV-P4; parity ratio today 0.763, must be ≥1. Split `Ledger.fee` (publish anti-spam vs delivery settlement).
- R2.12 → local admission control keyed on the node's monotonic clock (dissolves the F8 block); gate a separate `Grant(id)`, never `Register` (idempotent → permanent exclusion). Cap binds at 1,311/epoch.
- R2.4 → R2.9 lands FIRST (flip under the flat lane = 99.93% pay cut on a 64 MiB object). Phased: correctness gates incl. FP-1 → economy-OFF baselines incl. pre-flip Gini → canary → default.
- R2.7 → 5 solvency inequalities × 7 attacks; unpriced: A2 supersede-suppression (highest), A5 cold-start capture.
- R2.8 → reserve-aware repair scheduling + early cliff disclosure open to any funder + R2.9's expiring remainder; reject a network pool; never a mint.
- Blocking telemetry: `servedBytesWitnessed`/`Unwitnessed`, `bountyPaidToEscrowFunder` (neither exists).
- Five inputs remain ASSUMPTION (need live data): B_bootstrap, honest arrival rate, object-size distribution, willingness to re-endow, escrow recoverability.

### WP-B — state-view enumeration (PE inventory, landed; red-team hunt + Researcher closure to follow)
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/INVENTORY-accept-chain-state-view-enumeration-2026-09-03.md`
- **"Exactly one non-leaf fact" was wrong by five.** New: **N4** the `LogRoot` half of the roots predicate — every witness artifact is StateRoot-only, `core/translog` has no frontier/append primitive; a composition returning Accept today accepts on HALF the predicate. Sound fix: verify-not-recompute (consistency + inclusion), needing `prevLogSize` (a second missing fact). **N3** the K=8 ancestor hash chain (`recentBondRegNonces`, `chain.go:1742-1757`): naive witness = 7 full ancestor bodies; pruned ancestors unrecomputable. **N1 widened**: the HEIGHT axis is load-bearing beyond the carrier (era3/era4/regGate/blockEpoch/TTL/rotation). **N5** `trustFloor` is node-local by design → a signature parameter. **N6** `verifyBond` is parameterized by uncommitted node config (`BondVDFDelay`/`BondLabelSamples`).
- Taxonomy needs a fourth class **(c′) block-local but NOT hash-covered** (`Atts`/`CommitRound`/`PrepareQC`/`Pruned`); live on main: `apply()` writes `validatorsSeen` from `b.Atts` (`chain.go:3364`).
- Coupling: the four new pre-freeze leaves must land in ONE additive group with `tagLastProposer`.
- **Merge hazard (VERIFIED by the planner, `chain.go:667` vs carrier `chain.go` Hash()):** main's `unsigned` literal has `IssuerKeys` not `LastCommit`; the carrier branch's has `LastCommit` not `IssuerKeys`; the branch is behind main (merge-base 1adca0f). The rebase conflicts on that line → needs a reflection pin that the `Hash()` literal names every hash-covered field.
- Owner call: whether the four leaves enter era-4 freeze scope (PE recommends taking all four) or the freeze ships without them and the box stays permanently gated.
- Not walked: `vdf.Verify`/`manifest.VerifyProof` below `verifyBond`, `blindtoken.Verify`, calls through function values.

### WP-A — carrier family (crypto-specialist advisory, landed; Researcher cert + PE blind ruling to follow)
`/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-lastcommit-carrier-residuals-prior-art-2026-09-03.md`
- PARENT-BINDING, CREDIT-DENIAL and BYTES are three symptoms of ONE schema choice: a free-form signature list verified against a BLOCK-supplied hash vs CometBFT's set-indexed vector verified against a STATE-supplied hash.
- Parent binding → direction (b) (commit parent hash+height as leaves), landed in ONE cert with `tagLastProposer`; also closes the height axis and FP-1's ADD direction.
- Credit denial → advise AGAINST a minimum; the faithful answer is check-all-sigs + explicit `Absent` slots + proposer bonus (CometBFT `types/validation.go:24-28`); Ethereum's is a 32-slot inclusion window — silt's one-block window is the worst censorship geometry.
- Double-sign slot → evidence is defined over the VOTE, not the carrying block; naive addition of `b.LastCommit` to `signers` is a silent green no-op (sigs are over `X.Prev` at `X.Height-1`); joining on `(height, phase)` not `(height, round, phase)` manufactures honest slashes (#397 shape). Third instance after #496.
- Rollout → the tally already IS BIP 9; the gap is the override (flag-day). Faithful minimum: Cosmos `x/upgrade` schema (named upgrade + declared binary version → startup panic) + self-naming stall. Do NOT adopt strict CBOR DecMode.
- Bytes → derive from the BOX witness side (unauthenticated input), same schema as `adapters/tcpnet/tcpnet.go:63-72` and CometBFT `MaxBytes + MaxCommitBytes(MaxVotesCount)`.
- Sequencing: ONE certified format delta, not three or four.

### WP-G — three small designs (PE ruling, landed)
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-three-small-designs-issuerkey-pop-era4-e2e-fixture-smt-oracle-2026-09-03.md`
- **R-ISSUERKEY-POP:** the PoP is NOT the fix. Bind `issuerID` into `demandMsg` (off-chain, no committed byte). The fingerprint-binding alternative is VACUOUS (B registers A's actual key bytes). No already-registered keys can exist: era-3 and era-4 are DARK on every real network (`NewBondReg` stamps v3 vs tallies needing 4/5; no production activation-height setter; genesis stamps v2). Coupling: (B) closes the attack but (A) is a FORMAT SLOT — reserve it inert at the STAMP RAISE or `IssuerKeyReg` freezes with no room for a PoP. Owner: build (A) now vs reserve-only. Research-gated: the `demandMsg` change (D-DEMAND), PSS-vs-FDH, any `Serial` length rule.
- **R-E2E-ERA4-FIXTURE:** not independently schedulable — same stamp blocker; it is a deliverable OF the stamp raise. The "owed" composition gate ALREADY SHIPPED (`sim/demand_composition_test.go:52`, both arms); what is missing is its ablation twin. Owner: accept the e2e cost increase.
- **SMT keyspace-injectivity oracle:** decoration confirmed, and the invariant its safety rests on is FALSE: "map raw keys are never empty" — `c.spent[string(e.Token.Serial)]` (`chain.go:3254`) has zero validation; `validateEntry`'s token branch is gated on `tokenQuorum > 0` which no `cmd/` caller sets; `Key("spent\x00","")` == the scalar form. No live collision; margin is one tag name; the serial is also a floor-box read-set key (A2 class).

### WP-E — R2.9 re-pricing (Researcher certification, landed)
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R2.9-D-POD-KNOBS-delivery-settlement-repricing-RESEARCH-CERTIFICATION-2026-09-03.md`
- **CERTIFIED:** suppression strictly dominates above B = 50,000 bytes (`0.875·(B − fee)`: +13,594 at 64 KiB; +58.7M / 1,342× at 64 MiB); one default-off flag (`daemon.go:74`). **B3 conservation is INTACT** (conditioned on a BANKED receipt); what breaks is incentive-compatibility of accept. No shipped gate pins suppress-vs-accept.
- **CERTIFIED with scope:** `S/R ≥ 24·(B/fee)` ⇒ 32,212 at 64 MiB, witnessed-lane-only (receipts OFF ⇒ byte-proportional skim ⇒ 24 holds). Turning the conserved lane ON is a 1,342× durability DOWNGRADE, voiding D-POD-KNOBS knob 1's rationale.
- **Theorem:** escrow income = skim × price; skim fixed fraction; repair cost byte-derived ⇒ D-S7 holds iff PRICE is byte-proportional. No clamp/ordering fix restores it.
- **Direction GATED** (PayWord-denominated settlement; no new primitive class): G-1 parity STRICT (`p > r·U`); G-2 clamp at the credit site (clamping `p.net` RE-OPENS the money pump); G-3 conservation must not rest on a caller-supplied budget; G-4 re-derive `maxPaidSerial`; G-5 numéraire rescale covers all seven balance constants; G-6 remainder-to-escrow out of scope.
- Parity number 0.763 must be > 1; at U = 64 KiB, r = 1 ⇒ p = 65,536. **Blocking residual is AFFORDABILITY:** at parity a 500,000 grant buys 488 KiB of fetch, ever (build-immutable #4 regression introduced by the fix); `r ≤ grant/B_bootstrap`, B_bootstrap unmeasured.
- Alternatives (a)(b)(c) REFUTED; (c) because `RequireBondedFetchers` returns early when `demandBank == nil` — which IS the suppression lever.
- **Two NEW findings on the shipped relay lane (routed to PE + red-team):** F-1 nothing debits the fetcher at open; settlement debits a fresh ephemeral whose `acct()` grants 500,000 (`relay.go:72` → `credit.go:251`), and the privacy guard forces a fresh identity per session ⇒ the faucet fires once per session by construction. F-2 relay prices a byte 4,096× below serve.
- Owner must ratify: direction under G-1…G-6; interim exposure; R2.9-before-R2.4; strict parity or scope r = 0 on the witnessed path; the two blocking measurements; knob-1 rationale re-pricing.

### WP-A — carrier residuals (Researcher composed certification, landed; PE blind ruling to follow)
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/LASTCOMMIT-CARRIER-residuals-composed-direction-RESEARCH-CERTIFICATION-2026-09-03.md`
- Decisive artifact: `chain.go:752-767` — a silt consensus signature carries NO chain position ("the height rides inside the hash"); every consumer must supply position from state it trusts.
- **PARENT-BINDING — CERTIFIED:** box-owned `HeadRef` (already BG-2). Leaves REFUTED (hash circular; height redundant). Bundling with `tagLastProposer` REFUTED — no format delta exists. Flip-gated.
- **CREDIT-DENIAL — GATED and RE-PRICED:** pre-existing, and the carrier NARROWS it (main `chain.go:3364` seats from `b.Atts`, outside the `Hash()` preimage — any relay can deny divergently today; the carrier shrinks it to one proposer, agreed). Minimum REFUTED (liveness cliff + #402 forks); vector REFUTED (needs a validator cap ⇒ ceilings C2's domain ⇒ M0 published-claim change); window GATED. Only dated item: a doc fix.
- **DOUBLESIGN-SLOT — CERTIFIED, NOT a consensus-rule change:** the producer LIFTS the carried precommit onto the evidence copy of the parent's `Atts` (hash unchanged, accept set identical). Traps: SILENT-GREEN (adding `LastCommit` to the loops verifies against the carrying block's hash); FALSE-SLASH (`(height, phase)` join; deriving height from the carrying block's label).
- **ROLLOUT-SIGNAL — CERTIFIED minimum, no consensus rule.** Override is LATENT: `Era4ActivationHeight` has no `cmd/silt` flag. Strict decoder REFUTED. Release-runbook precondition on any future flag.
- **BYTES — principle + formula CERTIFIED, value GATED.** NOT a security parameter (BG-3 stalls ⇒ safety at any value); it is a build-immutable-#8 + box-liveness parameter ⇒ owner ratifies on immutable grounds after a pony measurement that does not yet exist. Frame-derived bound REFUTED (20.7× loose); state-dependent threshold REFUTED (#357 shape).
- Composed "one schema" claim PARTLY REFUTED: a state-supplied hash cannot exist in silt; the real pairing is 1+3, both zero-format. The proposed one-format-delta is EMPTY.
- **CD-0 (hard gate):** the `Hash()` merge hazard verified on both trees; a naive merge holes the signed body.
- Corrections owned: `26977a4` cert §8.1 and `ROADMAP.md:443-448` mis-classified the double-sign widening; the build-plan over-stated "zero format additions" by disposing of `LogRoot`/`prevLogSize` — filed as **R-LOGROOT-FORMAT-SCOPE** (matches PE inventory N4), open, routed.
- New residuals: R-DOUBLESIGN-TIP-BLIND (LOW), R-CARRIER-VECTOR-CAP (owner, M0), R-LOGROOT-FORMAT-SCOPE.
- Owner must ratify: the byte-ceiling value (after measurement); the vector cap if ever wanted; whether to buy the window; the runbook precondition. Residuals 1, 3, 4 need no further ratification.

### WP-B — red-team hunt on the enumeration (landed) — **CONFIRMED CONSENSUS BREAK ON MAIN**
`/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-accept-chain-state-view-enumeration-2026-09-03.md`
- **F1 CRITICAL, reproduced end-to-end through `Append` (planner re-ran the probe against main's exact `equivocation.go` + `chain.go`: 6/6 PASS, `TestRT_SV1_CrossHeightPrunedSlashForgery_Era1` / `_Era2`).** `VerifyEquivocation` reads height from a struct field (`equivocation.go:50`) but derives the verified message from `Hash()` (`:53`), which returns attacker-chosen `b.Pruned` (`chain.go:658-660`) — the two `Block` values inside `Slashes[i]` are the one place `Pruned` is attacker-supplied on the accept chain. Two GENUINE signatures by an honest validator at two DIFFERENT heights, re-labelled with one fictitious height, verify as a double-sign. Culprit slashed, deleted from `bonded`, disqualified forever. One Byzantine proposer, zero colluders, no key material. Era-1 (`ProposerSig`) and era-2 (same-round precommits) both work. `b.Slashes` is uncapped → the whole validator set in one block. **Breaks I5; I1/liveness at scale.** Missed because the I5 model-check fuzzes schedules at ONE height and pruned-as-evidence is a deliberate tested feature. Fourth instance of "a tail shipped without its precondition". Probe file: `scratchpad/siltcopy/core/chain/zz_rt_stateview_test.go` (session-local; the Tester must encode it as a permanent gate with `Append` as the oracle).
- F2 HIGH box/node split: `rotateEpoch`'s activation tallies iterate the NEWLY FROZEN set (`chain.go:3494/3496`), not pre-state `epochSet` → wrong digest → different StateRoot.
- F3 HIGH: the `dueBucket[h]` substitution makes the TTL sweep a SECOND implementation (node reads only `bondRegHeight`).
- F4 HIGH: N3 is a TRUNCATED walk (`[1 2 3 4 5 6 7 8 8…]`); a fixed-length ancestor list lets a box accept forged bonded standing.
- F5 MED-HIGH: `Hash()` reads AND WRITES a non-wire memo — the accept chain mutates its input.
- F6–F15 enumeration/classification defects (trustFloor reads four committed facts; scalar sites sampled not enumerated; `-bond-label-k` is a CLI-reachable validity oracle; StateRoot does not bind entry values; …).
- HELD: the `dueBucket`/`bondRegHeight` dual source; era-2 round/phase scoping; `collectQuorumSigs`; no clock/rand/fs reached; the `Hash()` literal merge hazard is real as stated.
- Not reached: `blindtoken.Verify`, `vdf.Verify`/`manifest.VerifyProof`, the SMT library, the box side, `Reconcile`/`adopt`, `core/genesis`.

### WP-C — R-membership + recovery boundary (PE ruling, landed)
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R-membership-and-recovery-boundary-535-2026-09-03.md`
- **R-membership is misnamed and half closed:** `bonded` is bounded by RegCap=256 × (TTL+1) + genesis ≈ 8,448 at shipped defaults; `qualified` ⊆ bonded; `epochSet` = clone. The real open sets are **`validatorsSeen` and `slashed`** — grow-only, zero production deletes, and the whole seen set folds on EVERY post-latch block → a TIME BOMB (every box eventually stalls on honest blocks), not a DoS.
- New: `anchoredPreSet` map-inserts + MTH-folds `w.PreIDs` with no length gate before the anchor can reject; the R3 ingest gate `IngestBlockWitnesses` has ZERO production callers; **no per-block count cap on `b.Slashes` or `b.IssuerKeys`** (RegCap's two missing siblings; Slashes on no list — and it is the amplifier for the I5 break).
- Recommendation is NOT a cap: retire the two whole-set folds; `C2Metric` consumes only `validatorsSeen ∩ live-bonded`, so enumerate from the bounded `bondedRoot` and replace the two MTH digests with O(1)-updatable leaves. Pre-freeze; no M0 ceiling; strictly cheaper. (Supersedes the PE's own 2026-09-02 Option-A cap.)
- **#535: the shipped directive knob is INERT** — `rotateOps` stalls on height alone with no directive in its signature; post-flip all three arms stall. Option (c) "commit the height as a leaf" is structurally impossible (unknowable at genesis; proposer-written = the refuted trap). **The stall is TERMINAL, not one block.** Recommend (a′): unconditional loud stall, unify the two forked predicates, delete the inert knob, document a `-ws-checkpoint`-class re-anchor at H+1. The ROADMAP-cited repro (h64 wedge) does not cover the question.
- Couplings: R-membership is downstream of R-STATEVIEW-ENUMERATION; the ingest gap and R-CARRIER-BYTES are one defect on two surfaces; `trustFloor` must be a composition signature parameter.

### WP-E follow-on — relay lane (red-team, landed) — **CONFIRMED MINT, behind a default-OFF flag**
`/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-relay-lane-session-grant-and-byte-price-2026-09-03.md`
- **RT-RELAY-1 HIGH, reproduced with shipped params:** `SettleRelaySession` settles on the RELAY's OWN ledger (`relaytransport.go:107` → `RedeemRelayCredit(n.id, sess.ephID, …)`); `RedeemRelayCredit` (`relay.go:72-73`) unconditionally debits the fetcher; `acct()`→`Register` grants 500,000 on first touch (`credit.go:247-258`); the fetcher is a fresh ephemeral by privacy guard (ii), so on the relay's books it is phantom/grant-funded. Relay balance rises by `chainValue`; nothing binds the chain to a real payment (`RelayOpen.Funding` is a bare declared int). 100 fresh-ephemeral sessions → relay +26,214,400 (262,144/session); with grant=0 the relay STILL gains (phantom stranded negative). Zero bytes needed. No cap, no bond. Self-deal: attacker IS the relay. **Reputation firewall HOLDS** → balance economy only → HIGH not CRITICAL. **Live only with `--accept-relay-payments` (default OFF).** The code comment "drawn from the fetcher's paid-in blind credit" is FALSE.
- RT-RELAY-3 MED-HIGH: MsgRelayPay CPU-DoS (bogus preimage walks full S hashes; ~1083× byte→ms; no rate limit).
- Lead 2: 4096:1 price ratio CONFIRMED exactly; durability-underpay arbitrage REFUTED (origin skim fires on relayed paths too).
- Oracle blind spot: `relay_test.go` pre-funds the fetcher on the SAME ledger (shared-ledger model cannot see the mint); `money_pump_test.go` never covers the relay lane.
- Four regression gates specified; RT-RELAY-1 routed to the Researcher (D-A4-CONSERVATION + per-node-ledger settlement).

### WP-A — PE blind review of the carrier certification (landed) — WP-A CLOSED as definitions
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-lastcommit-carrier-residuals-direction-review-2026-09-03.md`
- Adopt the cert's directions for residuals 1 (parent binding via box-owned head ref, zero format), 4 (rollout minimum), 5 (bytes principle) and its re-pricing of 2 (credit denial pre-existing, carrier narrows it). Decisive premise (signature carries no position) verified byte-for-byte on both trees; the 1+3 regrouping is right over the crypto advisory's 1+2+5.
- **Residual 3 (double-sign slot) DOWNGRADED to LOW:** `HeadCarrier` iterates `head.Atts` so `LastCommit ⊆ parent.Atts` by construction; the ROADMAP:445 premise cannot be produced by silt's own proposer. Build-spec gaps: the lift must run BEFORE `signers()`; `e.A = *ab` aliases the `Atts` array; `Slashes` is hash-covered so the lift couples residual 3 to residual 5.
- No `HeadRef`/`StateView`/`BoxState` symbol exists on any tree yet; the evidence a box can derive a trustworthy head is `AdoptPin` on the box-entry branch. The root-only-box stall applies to hash and height, not only `tagLastProposer`.
- **CD-0 under-scoped:** the carrier tree has no `IssuerKeys` field at all; main moved 2,907 lines in `core/chain` since base `1adca0f`; both branches rewrite the same box-entry file → second gate on v5 TAG-SET equality. **Owner ratification (new): rebase the carrier branch onto main NOW.**
- **`R-LOGROOT-FORMAT-SCOPE` mis-ranked by the cert:** the node requires both roots (`era3validity.go:134`), so a box Accepting without `LogRoot` breaks `box.Accept ⇒ node.Accept` → SAFETY, flip-gated; closable with zero format change (carry `parentLogRoot` in the head ref; stall on revocation-bearing blocks).
- Byte formula has no fixed point and no honest-carrier LOWER bound (the liveness cliff it used to refute a minimum applies to the maximum too). `R-CARRIER-MODELCHECK` was dropped from the cert's table; re-add. Nothing here is live: the node mints v2.

### WP-F — R4.2 A-axis (crypto-specialist advisory, landed; Researcher direction cert to follow)
`/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R4.2-A-axis-operator-diversity-prior-art-2026-09-03.md`
- **Sealed-plot domain binding is INERT:** the plot seed is already identity-bound (`core/bond/bond.go:159-172`), so an N-way splitter already pays N seals; folding the domain in adds zero marginal cost and forces an honest re-plot. Do not pursue.
- **The DHT cap has a free exemption the analogues lack:** `core/dht/table.go:43-46` never caps domain 0; geth/Bitcoin Core key the same defence on the OBSERVED address. At the C2 layer self-declaration is inert; at the DHT layer it is an exemption from an eclipse defence → a distinct R4.3 target.
- **FREEZE-TIMED, unlisted:** `bondDomain` is a committed v5 leaf inside the floor-box read set (`floorbox_recompute_maturity_v5.go:99-115`); any A-axis RULE change after R3.4 forces era-5 unless leaves are reserved at the freeze → belongs on the R3.4 carry-list beside R-STATEVIEW-ENUMERATION.
- Recommended shape **A3**: derive the group from the OBSERVED address; commit only the PARTITION (not addresses; answers Ethereum's SSLE counter-precedent); ratified by a ⅔-bonded-weight attestation quorum (Tor dirauth `Running`-vote schema with the bonded set as authorities). Additive-only (Storj: diversity on reward, not safety). A published-claim change to C1/C2 → full certification.
- Cautionaries: KAX17 (Tor had family + AS-diverse paths and still lost to >900 relays across >50 ASes; remediation was authority fiat, a center silt forbids); Filecoin proves per-identity real cost does not produce operator diversity → R4.2 is not redundant with D.
- Pre-flip, no-consensus-change asks: print `NakamotoDomains`/`DistinctDomains` (computed, not printed at `daemon.go:1029-1031`); measure `M_cluster = declared/observed` — that IS B1's unmeasured M in `k̂ ≥ k·M`. `lambdah.go:9-11`: changing `MatureCoefficient` changes what CT-1's λ_H floor measures.
- Unverified: distinct-/24/ASN market price (Economist); whether a validator's consensus endpoint is independently dialable by any bonded peer (A3 contingent on it).

### I5 break — fix direction (Researcher certification, landed) — OWNER RATIFICATION OWED
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/I5-cross-height-pruned-slash-forgery-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md`
- Root cause: `VerifyEquivocation` quantifies over the release height, which is NOT inside the signed message, and reads it from an accuser-controlled field; `Pruned` severs the hash↔body identity and frees every struct field including the era gate.
- **(a) CERTIFIED (6 gates):** evidence hashes are ALWAYS recomputed from full bodies; `Pruned` is never read for evidence. It is the rule the code's own doc already states (`equivocation.go:21`). Strictly narrowing ⇒ can never manufacture a slash. Place it in `VerifyEquivocation`, NOT `validateSlashes` (else `pendingSlashes` becomes a doomed-proposal loop). **Era gate: NONE** — live in era-1/2 today; an embedded-`Version` gate is attacker-controlled.
- (b) bind height into the signature domain — REFUTED twice (does not touch era-1's bare-hash verify; cannot retroactively bind minted signatures — no format change closes a break built from minted history). (c) REFUTED as posed.
- **(d-2) per-block encoded-BYTE ceiling on `Slashes` CERTIFIED as REQUIRED** (count REFUTED: 200 B entries → 768 MB); class = immutable-#8 resource ceiling, not a security parameter, not RegCap class; value measurement-gated. `Prune()` never recurses into `Slashes` → embedded evidence is never pruned by any path; full bodies pin ~1.5 MB `Answer`s permanently. (d-3) two-level hash CERTIFIED, deferred.
- Cost bounded: honest detection compares only heights ≥ finalized head; the local side of honest evidence is never pruned.
- **Worse than reported:** the attack needs NO Byzantine proposer — a Byzantine PEER makes an honest node slash and queue the forgery on-chain (gate G-5). `modelcheck_i5_accountable_test.go` is era-2 only → RT-SV-1 is outside the tier by construction. Model-check gains three axes: declared-vs-signed height; `Pruned` ∈ {unset, real, forged}; era ∈ {1,2}.
- Canon REFUTED: `retention.go:17-19` and `chain.go:691-697` ("unbounded late-reveal slashing evidence") change. Tester: 6 RED-first gates with `Append` as oracle; T-4 supersedes `TestQ2_PrunedBlockStillSlashable`.
- Residuals: R-LATE-REVEAL (held), R-EVIDENCE-BYTES, R-BIG-EVIDENCE-UNSLASHABLE (pre-existing), R-BOX (no floorbox file calls `VerifyEquivocation`), R-MEMO, R-RELOAD-RE-VERIFY.
- **OWNER RATIFIES:** narrow F2 so evidence hashes are always recomputed and never read from `Pruned`, accepting that a double-sign whose evidence is already pruned becomes unslashable, paired with a per-block encoded-byte ceiling on `Slashes`.

### Relay-lane mint — fix direction (Researcher certification, landed) — OWNER RATIFICATION OWED
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md`
- Root cause: NOT per-node ledgers, NOT the shared-ledger oracle. Privacy guard (ii) makes "debit the payer" UNIMPLEMENTABLE on any topology; per-node conservation is always AUTHORIZATION-anchored (`RedeemDeliveryCredit` debits no one either, `delivery.go:512-516`). The relay's anchor was specified 2026-08-27 Q4(a) and NEVER BUILT (`wire.go:20-24`: `Funding int`, a bare fetcher-set integer). PayWord (Rivest–Shamir 1996) shipped with two of four steps missing (broker cert, user signature, vendor term deleted).
- **(a) CERTIFIED in direction, bilateral form (issuer == relay), gates G-A1…G-A5:** the PayWord chain anchored to a blind-signed, issuer-verifiable prepayment. Zero new primitive class; preserves NO-TTP.
- (b) escrow REFUTED (on-chain contradicts Q5/Don't-#3; local form is empty); (c) bonded ephemeral REFUTED; (d) GATED behind R2.10/FP-2.
- **Composition finding:** R2.12 + a non-negative-payer check closes the exploit but converts the lane into a 100% DENIAL — there is NO funded honest path through the relay lane today.
- Invariant to gate (INV-RELAY-CONS): `settled(R) ≤ Σ face(spent anchors)`, each verified / spent-once / `ChargePublish`-backed ON THE PAYING LEDGER; per-session ledger total unchanged. Five RED-first gates T-1…T-5, none pre-funding the fetcher; three existing pair-total tests must be REWRITTEN.
- Price 4096:1 untouched (R2.9 G-5's total rescale); D-S7 unaffected. Face value caps a session at 195.3 MiB vs `MaxSessionBytes` 1 GiB → allow k ≤ 6 credentials.
- RT-RELAY-3 NOT closed → promote `Verifier.walkSteps` to an enforced per-session budget S (Builder + PE).
- Flag stays default-off AND settlement must pay 0 until the anchor lands; correct five false claims in `relay.go:16-61`.
- **Boulder 2 order changes:** the relay anchor becomes a PREREQUISITE of R2.9 (R2.9 and the economist's Option C both cite the relay lane as "already settled conservatively" — refuted). Side finding: `creditSpent` (`node.go:626`) has no cap/sweep/eviction on a shipped lane.
- **OWNER RATIFIES:** build the relay prepayment anchor (bilateral PayWord, issuer == relay) as a prerequisite of R2.9, with settlement paying 0 until it lands.

### WP-B — enumeration closure + freeze scope (Researcher certification, landed) — WP-B CLOSED; OWNER RATIFICATION OWED
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-STATEVIEW-ENUMERATION-closure-and-freeze-scope-RESEARCH-CERTIFICATION-2026-09-03.md`
- **Freeze scope CERTIFIED: ZERO committed leaves are required for SAFETY.** The PE's four-leaf recommendation is REFUTED: `tagPrevHash` is circular (`StateRoot` is inside the `Hash()` preimage); `tagPrevHeight`, `tagLastProposer`, `tagRecentBlockHashes` all close with zero format change via a box-owned `HeadRef` — `WitnessValidateV5` already takes `parentStateRoot` as an unauthenticated driver parameter (`floorbox_v5.go:223`), so a box-owned record adds zero trust.
- **`tagRevLogSize` is the ONE candidate leaf, LIVENESS-only:** `VerifyConsistency` with m=1 accepts any right-spine extension, so `m` must be authenticated and it is a field of no block. Without it: a TERMINAL stall at the first takedown block after any pin.
- Closure GATED on G-1…G-5, on a stronger footing: `modelcheck_state_completeness_test.go:76-151` machine-classifies every `Chain` field, leaving exactly two non-leaf containers (`blocks`, `revLog`) → closed by PARTITION, not by listing.
- F2–F15: 13 accepted (2 narrowed); F3 rejected (the certified era-4 design). Closure rules: F5+F10 ⇒ the composition's input identity must be the WIRE BYTES; F6 ⇒ "trustFloor as a caller parameter" REFUTED (a raised floor skips the space-time re-verify) — the box must refuse pruned blocks.
- F1 bearing: `Slashes` IS in the `Hash()` preimage → embedded `Pruned` is hash-covered; F1 adds no non-leaf fact; its fix carries an ACTIVATION deadline, not the freeze deadline.
- R-membership answered: `tagSlashedRoot` IS retirable (no accept-path fold); `tagValidatorsSeenRoot` is NOT until `C2Metric` is re-shaped (verdict-identical re-shape).
- Self-correction: the STRUCTURE RESEARCH VIEW §3.2 ("`tagLastProposer` the ONE hard pre-freeze item"; "`recentBondRegNonces` box-reproducible") superseded.
- **OWNER RATIFIES:** the era-4 freeze scope is at most ONE leaf — `tagRevLogSize`, bought purely so a floor box survives a takedown block — and no leaf at all is needed for safety.

### WP-F — R4.2 A-axis (Researcher direction certification, landed) — WP-F CLOSED; OWNER RATIFICATION OWED
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R4.2-A-axis-operator-diversity-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md`
- **DIRECTION: GATED on A3 (5 gates) / CERTIFIED on "measure, publish, do NOT wire."**
- Advisory findings verified: (i) sealed-plot binding inert — CERTIFIED, and worse (strands committed `bondRootOwner`/`byRoot`); (ii) DHT domain-0 exemption — CERTIFIED, plus a missed defect: `daemon.go:316-317` uses ONE `-domain` flag for both the eclipse cap and the consensus metric — a literal build-immutable-#3 violation, opposite defaults wanted; (iii) era — CERTIFIED in fact, RE-PRICED: `bondDomain` is an ERA-3 leaf (`statehash.go:174`), not v5; reserving leaves avoids only a prefix collision, not the era. Carry-list = one line `R-AAXIS-TAG-RESERVE` at the R3.4 decision.
- A3: privacy CERTIFIED (commit the partition); "application-layer vote" REFUTED (feeds `matureNow` → `everMature`, a committed leaf; `Atts` not hash-covered → no carrier); "additive-only is safe" REFUTED (a ⅔ adversary attests honest validators into one group, holds `NakamotoDomains` down, keeps the launch anchors PERMANENTLY — a center in two places); "additive-only changes `C_honest`" REFUTED (no reward consumer exists).
- **Core dilemma:** additive-only ⇒ `C_honest` unchanged; changes `C_honest` ⇒ a hard gate on routing reachability ⇒ immutable #3 (`TENETS.md:646`). A3 occupies no third position.
- **New live finding (re-priced):** a bonded adversary can DECLARE an honest validator's published domain and suppress maturity at one `MinBond` per collision (`chain.go:2364-2365`). Cheaper than anything A3 defends → R4.4 brief.
- C1 text: NO CHANGE. Only C2 changes, toward honesty. Advisory's `M_cluster` REFUTED (B1 defines M as the adversary's ratio). Four pre-build refutation thresholds given.
- Residuals: ASN price; dialable validator endpoint — both unverified.
- **OWNER RATIFIES:** re-scope R4.2 to measure / publish / fix the DHT domain-0 exemption and the single-flag defect / hand the A-axis to B8 (R4.4) as-is; do not wire A3.

### WP-C — R-membership + recovery boundary (Researcher certification, landed) — WP-C CLOSED; OWNER RATIFICATION OWED
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-membership-unbounded-sets-and-recovery-boundary-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md`
- **Def 1 R-membership GATED (7 gates):** retire `slashedRoot` (soundness-neutral) and `validatorsSeenRoot` (conditional on the `{seen ∩ live-bonded}` identity) from the v5 digest set — D-V5-WHOLESET-ROOTS five → three. Free today (era-2 minted), hard fork at activation. **G-1:** the legacy `matureNow` branch is unreachable by WIRING not state (the #572 shape) → an explicit `objective()` guard that stalls.
- **Def 2 #535 (a′) CERTIFIED (4 conditions):** cold auditor; strictly narrowing ⇒ I1/I3/I4 safe; re-anchor = the weak-subjectivity checkpoint schema already shipped; adopt its irrecoverable-failure clause.
- Refuted: the PE's M0-ceiling argument against a cap (right conclusion, wrong argument — a witness-fit cap never binds); "one rule, three surfaces" (hash-covered block content vs box-local witness content are two classes); the Researcher's own H-4 (`trustFloor` as a parameter is a wrong-accept vector: pruned below the floor skips the space-time re-verify) — the box refuses pruned blocks.
- New: the `-ws-checkpoint` pin binds `StateRoot` only for a non-pruned block; `IngestBlockWitnesses` is inapplicable to the root-only path; the `PreIDs` gate sits at the decode boundary; RegCap·(TTL+1) has a third carve-out (era-4 activation window).

## Close (2026-09-03)

All seven work packages landed. Seat runs: PE ×5, Researcher ×6, red-team ×2, crypto-specialist ×2,
economist ×1 — every one read-only, at most four concurrent, zero whole-tree Go commands on the box.
Two confirmed breaks surfaced from the hunts (R0.6 I5 pruned-slash forgery, LIVE on main; R0.7
relay-lane mint, behind a default-off flag), each with a certified fix direction and one owner
sentence. Eleven owner ratifications are consolidated at the top of `ROADMAP.md`'s *Decisions owed*
block. Nothing in this program built code; every build it names waits on its ratification and on a
Tester gate encoded RED-first.
