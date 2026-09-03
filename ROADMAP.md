# Silt Roadmap

> **Source of truth.** *The finished system we are building toward* is
> [`docs/VISION.md`](docs/VISION.md) (the north star). *What M0 asserts and why*
> lives in [`docs/design/m0.md`](docs/design/m0.md) (the composition spec). *The
> principles that keep it honest* are [`docs/TENETS.md`](docs/TENETS.md). *What the
> owner has decided* lives in [`docs/decisions.md`](docs/decisions.md). This file is
> the **single source of truth for tasks** and the **narrative path**: where we are,
> what the Boulders are, and why they're ordered the way they are. The GitHub issue
> tracker is being retired as a task driver (see the SSOT note below).
>
> The earlier **Gate 0→6 spine** (with Gate 4 as "the M0 mechanism to build") is
> **retired**: that mechanism is built and the mission was reframed (below). The
> `v0.1.x`/`0.2.x` tags are **experimental / learning releases**, not steps on the
> march to V1; that history lives in `docs/buildlog/`.

> **★ Single source of truth for tasks (2026-09-01).** This file — specifically the
> **Boulder/Rock spine** below ("The Boulders (current big-step tracker)") — is the SINGLE
> SOURCE OF TRUTH for live work. The path to the Release Candidate (V1) is the Boulders,
> executed in order under their stated dependencies. The GitHub issue tracker is being
> **retired** as a task driver; all live work lives here. A handful of umbrella/frontier
> issues (#183, #182, #180, #179, #94, #52) remain as evidence anchors the Boulders point
> at — they are not a second task list. Residual/off-critical-path items are enumerated in
> the **Residual backlog** section at the end of this file; repro recipes for named field
> defects live in the cited `docs/design/` and `docs/thinking/` docs.

## Tenets are the destination; the Boulders are the track

**V1 is defined by the tenets, satisfied and field-proven — not by a feature
list.** Every Boulder below advances one or more tenets; if a step serves no tenet,
it doesn't belong here. The relationship, stated once: **the tenets are the
destination; the Boulder/Rock spine is the current track that operationalizes them.**
(The earlier framings — "tenets are the roadmap" and, later, "the ordered path is the
track" — are retired; both are preserved in
[`/archive/roadmap-history-2026-09-01.md`](archive/roadmap-history-2026-09-01.md). The
D-M1-PIVOT *decision* they carried stands — see `docs/decisions.md`.) A tenet gates
V1 as a *principle*, never a *mechanism* — with one deliberate exception, **M0** (the
mission itself), whose *real* mechanism is in V1 by definition. **Release is gated by
proof (R1):** a tenet is "met" only when field-proven multi-machine, not sim- or
single-host-only.

## The launch stance — harden-first

The first public appearance must be **credible and spectacular from day one**. A
half-baked drop on a project this ambitious burns the one first impression we get
with the exact technical audience we need. So the tenet **floors** (integrity,
no-silent-loss, don't-crash, honest observability) *and* the **mission** (M0,
field-proven) are done before any launch. Feedback is sought — on something that
already stands up, not as a substitute for hardening.

**The build principle (B8):** best-in-class *components*, a novel *composition*.
We do not reinvent primitives (crypto, transport, codec); we adopt the strongest
proven ones and reserve novelty for the composition and incentives — where M0
lives — proven by spec + an **external** red-team, never self-graded.

> *The retired "The ordered path" preamble + numbered Phases 1–6 lived here (2026-08-19,
> D-M1-PIVOT framing). Extracted 2026-09-01 to
> [`/archive/roadmap-history-2026-09-01.md`](archive/roadmap-history-2026-09-01.md) — the
> D-M1-PIVOT decision it carried (M1 interleaves with the M0 tail; storage-economy-first;
> firewall unchanged; harness never softens) stands and lives in `docs/decisions.md`; the
> phase packaging is superseded by the Boulders below.*

### The Boulders (current big-step tracker — Rock/Boulder synthesis, 2026-09-01)

**This is the current track.** It replaces the earlier six-Rock overlay (preserved below as
*Superseded Rocks*), reorganizing the same board into **five sequenced Boulders** after a
7-seat audit ([`docs/thinking/2026-09-01-*-design.md`](docs/thinking/), the four PACE
deliberations). **Boulders** are the major arcs; **Rocks** are the ordered deliverables inside
each. This spine is the SSOT for live work; the Residual backlog (end of file) holds the
off-critical-path tail. Items are
marked owner-DECISION, cert-gated, or RED-first as they apply. The overnight owner-ratified
decisions are folded in (R0.2, R1.7, R2.3, R2.5, R2.6, R3.4), and the **2026-09-03 ratifications**
below (the flip/freeze/B8 order; R-BOX-ATTESTS O1/O2/O4).

**Two whole-board sequencing constraints:**
1. **Nothing turns the economy on over a live mint.** Boulder 0 (A4) precedes Boulder 2.
   Boulder 0 is now DONE + MERGED (PR #686); the R0.4 conservation re-cert cleared before any
   economy default flip.
2. **The accept-flip does not close until its whole witness-soundness spine is green, the era-4
   format is FROZEN, and the external pass clears on the frozen artifact.** The flip (R1.8) is a
   consensus-rule change (I1); the old "single remaining step" framing hid 7 predecessors — see the
   flag under *Superseded Rocks*. **RATIFIED 2026-09-03 — the order is now:** internal flip
   preconditions (R1.0–R1.6 + the named residuals) → **era-4/v5 format freeze at the RELEASE
   CANDIDATE, which is the stamp-raising release (R3.4)** → **external B8 pass (R1.7 / R4.4) against
   the FROZEN, still never-Accept artifact** → **R1.8, the flip**. This **reverses** R3.4's earlier
   "the flip proceeds pre-freeze; the freeze re-confirms byte-identity later" clause: a B8 pass
   bought against an unfrozen format would have to be re-bought at the freeze.

**★★ OWNER RATIFICATIONS OWED — the 2026-09-03 definition program.** Eleven seat runs (PE ×5,
Researcher ×6, red-team ×2, crypto-specialist ×2, economist ×1; all read-only) put every undefined Rock
through design and certification in one day. Record and index:
[`docs/thinking/2026-09-03-roadmap-definition-program.md`](docs/thinking/2026-09-03-roadmap-definition-program.md).
Each line below is the ONE sentence the certifying seat asks the owner to ratify; the Rock it lands on
carries the argument and the sources. Two are LIVE BREAKS and come first.
1. **R0.6 (I5 break, LIVE on main) — ✅ RATIFIED 2026-09-03 (owner: "R0.6 ratified") and BUILT the same
   day** (branch `builder/r0.6-i5-evidence-recompute`; PR pending): evidence
   hashes recomputed from full bodies, `Pruned` never read, `Slashes` byte ceiling. The six RED-first gates
   plus G-2/G-4/G-6 and the three-axis I5 model-check are GREEN; G-1 scanned zero hits. **Still owed: the
   owner ratifies the `SlashesBytesCap` VALUE (provisional 16 MiB; G-3 measured — see
   `docs/decisions.md` D-F2-EVIDENCE-RECOMPUTE).**
2. **R0.7 / R2.14 (relay mint, behind a default-off flag):** build the relay prepayment anchor
   (bilateral PayWord, issuer == relay) as a prerequisite of R2.9, with settlement paying 0 and the flag
   default-off until it lands.
3. **R-STATEVIEW-ENUMERATION / R3.4 freeze scope:** the era-4 freeze scope is at most ONE leaf —
   `tagRevLogSize`, bought purely so a floor box survives a takedown block — and no leaf at all is
   needed for safety.
4. **Carrier branch:** rebase `builder/lastcommit-carrier` onto main NOW behind the two hard merge
   gates (the `Hash()` literal reflection pin; v5 tag-set equality); ratify the carrier byte-ceiling
   VALUE only after a pony-class measurement exists; decide whether to buy the multi-block inclusion
   window (R-CARRIER-CREDIT-DENIAL); decline the vector cap unless the M0 claim is re-opened.
5. **R2.9:** adopt PayWord-denominated per-increment delivery settlement under gates G-1…G-6; accept
   the interim exposure (suppression is the shipped default and conservation holds); order R2.14 → R2.9
   → R2.4; choose strict parity or `r = 0` on the witnessed path; authorise the two blocking
   measurements (`B_bootstrap`; the honest arrival rate).
6. **FP-2 / ledger durability:** whether the ledger gets a durable store before the RC at all (PE:
   close FP-2 by scope; land R2.13 and R2.10 regardless).
7. **R4.2:** re-scope the A-axis to measure / publish / fix the DHT domain-0 exemption and the single
   `-domain` flag / hand the A-axis to B8 as-is; do NOT wire A3.
8. **R-ISSUERKEY-POP:** build the `IssuerKeyReg` PoP format slot now, or reserve it inert at the stamp
   raise (the off-chain `demandMsg` binding is the fix either way).
9. **R-E2E-ERA4-FIXTURE:** accept the e2e cost increase at the stamp raise.
10. **R-membership:** close it by retiring `slashedRoot` and `validatorsSeenRoot` from the v5
    committed digest set (D-V5-WHOLESET-ROOTS five → three) rather than by capping seated identities;
    a hard fork at activation; gated on the box's explicit `objective()` guard.
11. **Recovery boundary (#535):** the floor box is a COLD AUDITOR — unconditional loud stall, the two
    directive knobs deleted, pruned blocks refused, `trustFloor` off the contract surface, recovery by
    a fresh `-ws-checkpoint`-class anchor at H+1 treated as irrecoverable if unreachable.
**Not the owner's:** the research-gated pieces are named on each Rock and stay with the Researcher.

**★ Decisions owed to the owner (as of 2026-09-03).** Read these first; each names its source.
- **Floor-box STRUCTURE (Boulder 1, pre-freeze) — RATIFIED 2026-09-03.** Owner: *"I accept the
  recommendation."* All three owner items are DECIDED as recommended: (1) the structure, (2) HOLD the
  box-entry round-A export surface (merge the arithmetic, doors unexported), (3) the state-view
  enumeration is a FREEZE precondition ordered before the structure build. The two research-gated
  items (the one-sided contract form; the `R-CARRIER-BYTES` bound value) stay routed to the
  Researcher. The recommendation, as ratified — build option (E)/(D): ONE accept composition
  `ValidateCommitV5(view, block)` over a three-valued `StateView`
  (`Present`/`ProvenAbsent`/`NoWitness`, where `NoWitness` STALLS), called by both the node and the
  box, with all 11 box doors **unexported** behind a single `WitnessValidateV5` door; the contract
  form becomes one-sided `box.Accept ⇒ node.Accept` (exact box-vs-node equality is REFUTED as a
  certified form). Both seats also recommend **HOLDING** the box-entry round-A exported surface
  (merge the arithmetic, keep the doors unexported — zero production callers today) and name two
  **PRE-FREEZE** items: `tagLastProposer` and the carrier byte ceiling. Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-predicate-rederivation-structure-2026-09-03.md`,
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md`.
- **O3 — the fork-choice weight term — RATIFIED 2026-09-03: Direction T.** Owner: *"Direction T."*
  Both seats independently recommended **Direction T (retire the term; state `heavier` as height →
  head-hash)** over Direction R (repair). Ratifying T settles the one product premise the seats named
  as the owner's: silt supports **no production posture without BFT finality**. The I5 restatement
  and the no-reachable-divergence argument remain research-gated (Researcher certifies before the
  build lands). Reopening condition stands: a shipping posture in which `FinalizedHeight()` lags
  `Head()`. Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-O3-fork-choice-weight-R-vs-T-2026-09-03.md`,
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/O3-fork-choice-weight-R-vs-T-RESEARCH-RECOMMENDATION-2026-09-03.md`.
- **R0.4b C3 (expiry) — four owner calls, ALL RATIFIED 2026-09-03.** Owner: *“merge
  please”*; on the consensus-rule veto gate, *“I accept”*. Each call below is now DECIDED, as
  recommended, and is kept here with its source. **The ratifications:** (1) the payload-driven
  `issuerKeyCommit` prune is ACCEPTED as a consensus rule; (2) the `IssuerKeys` per-block cap is a
  **pre-freeze Rock, NOT a merge gate** — it is carried in R3.4's pre-freeze carry-list; (3)
  `grant = 500_000` plus a faucet rate limit; (4) `RequireBondedFetchers` default **OFF**. (1)
  **Break-1 ratification:** the payload-driven
  `issuerKeyCommit` prune is a **consensus rule** and is research-CERTIFIED
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03.md`
  §1). (2) **The `IssuerKeys` per-block count cap** — a v5 **validity rule** of the class `RegCap`
  already answers for `BondRegs`; measured ~33 s of ed25519 per validator per block at the 128 MiB
  frame ceiling; free now while v5 is dark, a fork after era-4 opens
  (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R0.4b-C3-close-271ab81-final-2026-09-03.md`
  §3, §8). (3) **The production `grant` + faucet rate limit** — keep `grant = 500_000` and
  rate-limit the faucet (structure fixed, rate is an Evolving-tier knob; do NOT key the limiter on
  the ledger watermark — blocked on F8) and (4) **`RequireBondedFetchers` default OFF**
  (`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-R0.4b-cap-griefing-grant-and-bonded-fetchers-2026-09-03.md`,
  certified in the composed cert §6). **G-8 no longer needs an owner call:** both seats converged on
  **(iii) re-scope** the e2e gate to the certified era-4 refusal — never an activation override, in
  any form, in any binary
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md`).
- **R0.6 · I5 CROSS-HEIGHT `Pruned` SLASH FORGERY — CONFIRMED LIVE BREAK ON MAIN (2026-09-03); fix direction CERTIFIED; RATIFIED + BUILT 2026-09-03 (branch `builder/r0.6-i5-evidence-recompute`, PR pending); the cap VALUE ratification is owed.**
  `VerifyEquivocation` reads height from the evidence struct (`equivocation.go:50`) but the signed
  message from `Hash()` (`:53`), which returns attacker-supplied `b.Pruned` (`chain.go:658-660`) for
  the two Blocks inside `Slashes[i]`. Two GENUINE signatures by an honest validator at two DIFFERENT
  heights, re-labelled with one fictitious height, verify as a double-sign; through `Append` the honest
  validator is slashed, evicted from `bonded`, disqualified forever. Era-1 and era-2 both. `Slashes`
  is uncapped. **Needs no Byzantine proposer** — a Byzantine PEER makes an honest node slash and queue
  the forgery. Reproduced end-to-end (red-team probe, re-run by the planner against main: 6/6). The I5
  model-check is era-2-only and fuzzes one height — outside its schedule space by construction.
  **Fix (CERTIFIED, 6 gates, NO era gate):** evidence hashes are ALWAYS recomputed from full bodies;
  `Pruned` is never read for evidence — the rule `equivocation.go:21` already states; strictly
  narrowing, so it can never manufacture a slash; placed in `VerifyEquivocation`, not
  `validateSlashes`. Paired with a per-block encoded-BYTE ceiling on `Slashes` (required: `Prune()`
  never recurses into `Slashes`, so embedded ~1.5 MB `Answer`s pin permanently). Binding height into
  the signature domain REFUTED (era-1 bare-hash verify; cannot rebind minted history). Canon changes:
  `retention.go:17-19`, `chain.go:691-697`. Model-check gains three axes (declared-vs-signed height;
  `Pruned` ∈ {unset, real, forged}; era ∈ {1,2}). Six RED-first Tester gates with `Append` as
  oracle; T-4 supersedes `TestQ2_PrunedBlockStillSlashable`. **Owner ratifies:** narrow F2 so evidence
  hashes are always recomputed and never read from `Pruned`, accepting that a double-sign whose evidence
  is already pruned becomes unslashable, paired with the `Slashes` byte ceiling. Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-accept-chain-state-view-enumeration-2026-09-03.md` (F1),
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/I5-cross-height-pruned-slash-forgery-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md`.
- **R0.7 · RELAY-LANE per-node-ledger MINT — CONFIRMED (2026-09-03), HIGH, live only with `--accept-relay-payments` (default OFF); fix = R2.14.**
  `SettleRelaySession` settles on the RELAY's own ledger (`relaytransport.go:107`); `RedeemRelayCredit`
  debits the fetcher's fresh ephemeral, which on that ledger is a phantom auto-granted 500,000 on first
  touch (`credit.go:247-258`); the relay's balance rises by `chainValue` with nothing binding the chain
  to a real payment. 100 fresh-ephemeral sessions → relay +26,214,400 with zero bytes; with grant = 0
  the relay still gains. Self-deal: the attacker IS the relay. Reputation firewall HOLDS (balance
  economy only). `relay_test.go` pre-funds the fetcher on the SAME ledger and cannot see it;
  `money_pump_test.go` never covers the relay lane. Interim: flag stays OFF and settlement pays 0.
  Sources: `/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-relay-lane-session-grant-and-byte-price-2026-09-03.md`, the R2.14
  cert.
- **`R-CARRIER-BYTES` — the validity bound, with the cost now MEASURED.** 1.3 M carrier entries fit
  a single 132 MiB frame and cost **42.2 s single-core** to validate, **130.18 MiB** on the wire,
  **1.13 GiB** max RSS — and are re-paid on every disk reload, permanently
  (`/Users/andrewedmond/Claude/claude/silt/.claude/agent-memory/tester/pr-lastcommit-carrier-verification-2026-09-03.md`).
  Red-team round 2 adds the box-side face: a **VALID junk carrier** costs the box **2.67 GiB** of
  `AttScreen` witness for one canonical NO-OP block
  (`/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-lastcommit-carrier-26977a4-RE-BREAK-2026-09-03.md`
  RT2-CARRIER-17/17b). The two legs differ in reachability and that matters: the node's 42.2 s leg
  requires a **qualified proposer**; the box's does not
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/LASTCOMMIT-CARRIER-26977a4-DELTA-CERTIFICATION-2026-09-03.md`
  §2.4). A size rule on hash-covered content is a v5 validity rule: research-gated, owner-ratified,
  and it must land **before** the era-4 freeze.

#### Boulder 0 — Stop the live bleed (A4 money-pump) · ✅ DONE + MERGED (PR #686, 2026-09-01)
The only break exploitable on `main` (behind `-accept-delivery-receipts`; `-economy`
default false). Bond-gated so it bought no consensus standing, but it broke the certified B3
conservation close and could fund spam/publish at scale. **CLOSED:** the live money-pump is
dead. **R0.4 CERTIFIED** — conservation (`Σ balances + Σ escrow == grant + legitimate transfers`)
holds across all three terminal lane states (never-redeemed, in-window-redeemed,
evicted-then-redeemed); both firewalls (γ→1/N and standing) untouched. Shipped **(b)-minimal**
(the eviction claw-back), NOT full (b)-prunable — receipt-expiry is parked as R0.4b (below).
Alongside the ledger fix, `provOrder`/`provIndex` integrity landed: a blind red-team pass and a
compaction fuzz each found and closed a FIFO-order desync.

- **R0.1 · A4 conservation regression gate (RED-first)** · ✅ DONE. Asserts
  `Σ balances + Σ escrow == initial grant` across flood→evict→redeem
  (`money_pump_test.go`, `TestA4MoneyPumpConservation`); RED on main → GREEN on the fix.
- **R0.2 · redeem-without-provisional-record semantics — DECISION: (b)-minimal SHIPPED;
  (b)-prunable is the ratified longer-term direction, parked as R0.4b.** (b)-minimal
  reverses the eager self-mint at eviction (one delivery, one payment) and is a
  conservation-correct close on its own. The full (b)-prunable form — couple provisional
  lifetime to an ENFORCED receipt-expiry — is a separate, evidence-gated certification unit
  (R0.4b), not a soundness dependency of the shipped fix.
- **R0.3 · A4 fix (claw back the eager self-mint on eviction)** · ✅ DONE. Shared
  `reverseProvisional` at both eviction and redeem (identical escrow floor at each);
  `provisionalServe` now stores the server identity so eviction reverses the exact credited
  account.
- **R0.4 · Economic re-cert of the B3 conservation close** · ✅ CERTIFIED (2026-09-01)
  (`A4-provisional-eviction-conservation-RESEARCH-CERTIFICATION-2026-09-01.md`). The prior B3
  cert did not model bounded-map eviction; this cert closes exactly that residual and confirms
  no new money pump and no firewall re-open.
- **R0.5 · A4 node-path integration conservation gate** · ✅ DONE. Proves the fix is wired at
  the node path (`node.go` + `demandrole.go`), not just in the ledger.

**Boulder 0 residuals (status 2026-09-03):**
- **RT-DELIV-3 · delivery-credit `provKey` omits the server** · ✅ **DONE + MERGED (PR #699), with
  its open-break gate (PR #700).** DECIDED: fix now. The server identity is in `provKey`, which
  additionally closes a conservation break in the shared-ledger MULTI-SERVER case the prior A4/R0.4
  cert did not model; both firewalls (γ→1/N and standing) stay intact. Research cert (the change
  re-opened and re-closed R0.4):
  `RT-DELIV-3-provkey-server-identity-RESEARCH-CERTIFICATION-2026-09-02.md`.
- **R0.4b · receipt-expiry (the full (b)-prunable form) — ✅ DONE + MERGED (PR #711, 2026-09-03).**
  The two owner calls that gated the build were answered (per-epoch / serial-indexed keys, never a
  wall-clock `NotAfter`). Ships the (b1) FDH epoch binding, the payload-driven `issuerKeyCommit`
  prune (a ratified consensus rule), the token-keyed guards, a persisted paid-serial store, the
  supersede ordering, and the blind-RSA `ValidatePub` hot-path/admission split. The last red CI job
  was a `-race`-inflated timing budget, fixed by gating the wall-clock half only (count gate runs
  under both builds). The `IssuerKeys` per-block cap (H-1) is a pre-freeze Rock in R3.4's carry-list,
  NOT a merge gate (owner call). Verdicts: research `R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03.md` (GATED;
  G-3/G-4/G-D since CLOSED per
  `R0.4b-C3-271ab81-G3-G4-GD-DELTA-CERTIFICATION-2026-09-03.md`), PE
  `RULING-R0.4b-C3-close-271ab81-final-2026-09-03.md`, G-8
  `R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md` (take option (iii)). The four owner calls are in
  the *Decisions owed* block above; the named residuals are Rocks under Boulder 1 and Boulder 2.

#### Boulder 1 — Make the accept-flip safe (floor-box witness-soundness spine) · was Rock 1
Root cause (verified, all seats): classes P/A/B take witness values/screens as `NewValue`s or
branch predicates without `Resolve`-ing against `prevStateRoot`; the attacker controls the
committed root too, so `postRoot==StateRoot` holds by construction. Sound classes route witness
values as VerifyProof'd `OldValue`s — the fix mirrors them. Design:
[`docs/thinking/2026-09-01-floorbox-witness-soundness-fix-design.md`](docs/thinking/2026-09-01-floorbox-witness-soundness-fix-design.md).

**Status (2026-09-03): R1.0 / R1.1 / R1.2 / R1.5 / R1.6 DONE; R1.3 REFUTED; R1.4 CERTIFIED then
RE-OPENED and re-closed by the class-P scalar-anchoring fix (PR #704). The accept-flip (R1.8) is NOT
done** — its precondition list has GROWN this session (the fold-live-state class, the box-entry
round-A findings N1–N8, and the `LastCommit` carrier's parent-anchoring class), and the flip now
lands **after** the era-4 freeze and the external B8 pass (see the sequencing constraint above). The recompute-soundness increment merged as a standalone **never-Accept**
change: the box gained the Resolve-anchoring but still emits `indeterminate-trustlessly`, never
Accept. Certification:
[`floorbox-R1.3-refutation-R1.4-witness-soundness-RESEARCH-CERTIFICATION-2026-09-01.md`](../silt-reviews/research/research-outcome/floorbox-R1.3-refutation-R1.4-witness-soundness-RESEARCH-CERTIFICATION-2026-09-01.md).

- **R1.0 · Pin the held invariants BEFORE the refactor** · ✅ DONE. The shared quorum
  arithmetic (`requireQuorumStack`) does not fork between the live path and the box (the #402
  lesson); class-M is A2-poisoned (inherits the forged `validatorsSeenRoot`).
- **R1.1 · Adversarial-committed-root regression gates for every P/A/B break (RED-first)** ·
  ✅ DONE. Proved **all 11 P/A/B witness fields forgeable**: each gate forges the committed
  root from the forged ops, then asserts non-nil while forgedRoot≠honestRoot. Settled the open
  tension by evidence — class-P `Weight` IS forgeable (`epochSetRoot` is membership-only).
- **R1.2 · Re-anchor P/A/B witness values as Resolved fold OldValues** · ✅ DONE. Every
  untrusted P/A/B value/predicate and the class-A screen read is now `Resolve`d against
  `prevStateRoot` (the one root the attacker does not control); **NoWitness ⇒ stall** (never
  falls through to a false/absent read). The whole-set digest pre-sets are fold-anchored.
- **R1.3 · RE-OPEN + REFUTE the class-A/P/B directional certs** · ✅ REFUTED (2026-09-01).
  The 2026-08-31 class-A/P/B directional certs are **WITHDRAWN**: their "fold-equality
  (`postRoot == StateRoot`) is a universal backstop" premise is FALSIFIED (an attacker who
  forges the block controls the committed root too, so equality holds by construction). Correct
  direction named: **Resolve-anchoring against `prevStateRoot`**.
- **R1.4 · Witness-soundness recompute cert (23-field carrier table, membership-vs-value ×
  source)** · ✅ CERTIFIED (2026-09-01), bounded strictly to **recompute soundness as a
  NEVER-ACCEPT increment**. The 23-field carrier table is complete by reflection with teeth;
  each of the 10 per-field anchors plus the class-M poisoning path has a driven
  adversarial-committed-root gate that wrong-accepts if its anchor is dropped. This is NOT the
  flip's certification — it is its precondition.
- **R1.5 · Accept-flip model-check exercising the NEW Resolve path** · ✅ **DONE (PR #702).**
  A scheduling oracle over the Resolve path: honest-baseline agreement, I1 (disjoint boxes never
  conflict-Accept), I5 (an honest node is never slashed), multi-block Resolve stability under
  reorder, and no forged-witness poisoning of the next `prevStateRoot`; two OPEN-BREAK gates carry
  distinct forged roots, so neither is vacuous. Absorbs old Rock 4 (#406).
- **R1.6 · Oracle coverage: per-field adversarial Resolve-path probes** · ✅ **DONE (PR #701).**
  Surfaced and gated a class-P activation-lock wrong-accept; the three OPEN-BREAK gates were each
  confirmed non-vacuous (forged root ≠ honest committed root). The class-P fix landed separately
  (PR #704, Direction A + B), which is why R1.4 was re-opened and re-closed.
  **Residual:** `probeUncovered` still does not name A1/A2/A3 — that half is carried as R3.2, and
  the class-A probes must be re-pointed if the `LastCommit` carrier lands (they read `b.Atts` today;
  `R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md` §9 stale-list).
- **R1.7 · External red-team pass (B8) — DECISION RATIFIED (re-ratified 2026-09-03): the external
  pass is a HARD precondition of the flip, and it runs at the RELEASE CANDIDATE, AFTER the era-4
  freeze.** Owner milestone-call + external seat; it is the same gate as **R4.4**, not a second one.
  The recompute's OUTPUT is C2's INPUT; internal cert cannot close "no adversary forges a witness the
  recompute accepts." **The artifact it attacks is the FROZEN, still never-Accept box** — attacking
  an unfrozen format would buy a pass that has to be re-bought at the freeze. R1.5 (model-check) and
  R1.6 (per-field probes) are DONE, which is what converts the external pass from discovery to
  bounded confirmation.
- **R1.8 · The flip: wire `WitnessValidateV5` → Accept-iff-all-predicates-pass** · Builder · S.
  **NOT done.** Trivial code; a consensus-rule change (I1). Beyond R1.5/R1.6 green, the flip
  **additionally requires** (per the R1.4 cert §R1.4-FLIP):
  - **R-membership — DEFINED 2026-09-03 (GATED, 7 gates; owner ratification owed; PRE-FREEZE).** Misnamed:
    `bonded` is already bounded by RegCap·(TTL+1)+genesis ≈ 8,448 (three carve-outs incl. the era-4
    activation window), `qualified ⊆ bonded`, `epochSet` is a clone. The real grow-only sets are
    `validatorsSeen` and `slashed`, folded WHOLE on every post-latch block — a terminal-stall time bomb
    for every box, not a DoS. **Direction (not a cap):** retire `slashedRoot` (soundness-neutral: zero
    predicates iterate `slashed`) and `validatorsSeenRoot` (conditional: `{seen ∩ live-bonded}` =
    `{bonded ∧ ≥MinBond ∧ ¬slashed ∧ ¬anchor ∧ ∈seen}` exactly, so enumerating `bonded` is complete)
    from the v5 digest set, amending D-V5-WHOLESET-ROOTS from five roots to three; free today (production
    mints era-2), a hard fork at activation. **G-1 blocks** until the box carries an explicit
    `objective()` guard that stalls instead of silently taking the legacy `matureNow` branch
    (`chain.go:2231-2238` is unreachable by WIRING, the #572 shape). Also: the `PreIDs` gate sits at
    the DECODE boundary; `IngestBlockWitnesses` is structurally inapplicable to the root-only path.
    **Owner sentence:** ratify that R-membership closes by removing the two grow-only sets from the
    floor box's whole-set fold, retiring `slashedRoot` and `validatorsSeenRoot` from the v5 committed
    digest set (D-V5-WHOLESET-ROOTS five → three), rather than by capping seated identities; free today,
    a hard fork at activation, and not certified until the `objective()` guard lands. Sources:
    `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R-membership-and-recovery-boundary-535-2026-09-03.md`,
    `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-membership-unbounded-sets-and-recovery-boundary-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md`.
  - the **EXTERNAL B8 red-team pass** (R1.7) — owner-ratified HARD precondition;
  - the **recovery-boundary decision (formerly #535) — DEFINED 2026-09-03 (CERTIFIED (a′), 4 conditions; owner ratification owed).**
    The shipped directive knob is INERT (`rotateOps` stalls on height alone; no directive in its
    signature, pinned by `rotate_v5_test.go:645`); "commit the recovery height as a leaf" is structurally
    impossible; the stall is TERMINAL, not one block; the ROADMAP-cited repro (the h64 wedge) does not
    cover the question. (a′) is strictly narrowing ⇒ I1/I3/I4 safe. The re-anchor is the Ethereum
    weak-subjectivity checkpoint schema silt already ships (`-ws-checkpoint`; `claims-ledger.md:43`);
    adopt its IRRECOVERABLE-FAILURE clause. New: the pin binds `StateRoot` only for a NON-pruned block
    (`chain.go:658-660` vs `:667`); a caller-supplied raised `trustFloor` skips the space-time re-verify
    (`chain.go:1801-1815`) — so the box REFUSES pruned blocks and `trustFloor` leaves the contract surface.
    **Owner sentence:** ratify that the floor box's role at a #535 recovery boundary is COLD AUDITOR —
    it stalls loudly and unconditionally, `RecoveryDirective.Heights` and `LiveFollower` are deleted,
    the box refuses pruned blocks outright rather than taking a trust floor from its caller, and the
    operator's recovery path is a fresh `-ws-checkpoint`-class anchor at H+1 treated as a critical,
    irrecoverable failure if unreachable. Sources: the ruling and certification above;
    [`docs/thinking/2026-09-01-residual-defect-repro-recipes.md`](docs/thinking/2026-09-01-residual-defect-repro-recipes.md);
  - the **legacy-mode invariant** (the pre-v5 path stays sound under the flip);
  - **R-FOLD-LIVE-STATE-READS — the fold reads NO live box state** (research cert
    `floorbox-R-FOLD-LIVE-STATE-READS-RESEARCH-CERTIFICATION-2026-09-02.md`, GATED). The class-A
    screen selected its qualification branch from `c.matureEpoch` and its anchor eligibility from
    `c.launchAnchor` → `c.handedOff()` — box-own fields written only by `apply→rotateEpoch` and
    `adopt`. The deployment target replays no `apply()`, so a COLD box never set them and screened
    every mature-epoch block under the pre-maturity rule: wrong-accept of a mid-epoch joiner against
    an attacker's root, and false stall on an honest one, with every witness proof passing.
    **Direction A landed (PR #706)** (the branch selector is the Resolved `tagMatureEpoch`
    pre-value anchored against `prevStateRoot`; `launchAnchorGiven` is the one shared predicate)
    together with the
    **R-COLD-BOX-HARNESS** tier and the fold-file live-state allowlist pin. The remaining flip
    obligation is that the cold-box tier stays the tier every new recompute gate runs in.

  **Added to the precondition list 2026-09-02/03** (each is a Rock in the *New Rocks* block below,
  with its source):
  - **R-STRUCTURE-REDERIVATION** — the one-accept-composition build (owner decision pending), and
    its **R-STATEVIEW-ENUMERATION** precursor, which is a FREEZE precondition earlier in the order
    than the flip itself;
  - **R-CARRIER-PARENT-BINDING** — a flip ENTRY blocker: the box reproduces `validateCarrier`
    without the ancestry precondition its node-side caller establishes;
  - **R-CARRIER-PARENTPROPOSER** (`tagLastProposer`) and **R-CARRIER-BYTES**, both also pre-freeze;
  - **R-BOXENTRY-RESIDUALS** — box-entry round-A findings N1–N8 and the §8 residual set;
  - **FP-1** — `Bank.spent` persistence, re-armed the moment witnessed demand confers value.

  **ORDERING — RATIFIED 2026-09-03 (supersedes the earlier "decoupled; the flip proceeds
  pre-freeze" clause):** the flip lands **AFTER** the era-4/v5 format freeze (R3.4) and **AFTER** the
  external B8 pass on the frozen artifact. `WitnessValidateV5` still changes no committed format
  field — the reason for the new order is the B8 pass, not the box: a pass bought against an unfrozen
  format has to be re-bought at the freeze. Two further couplings this session make the order
  load-bearing rather than merely tidy: the state-view enumeration the STRUCTURE build needs is
  itself a **freeze precondition** (it decides which committed leaves the box requires, and a leaf
  discovered after the freeze is a new era), and `R-CARRIER-BYTES` is a v5 **validity rule** that
  cannot land in-era. PE ruling
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-predicate-rederivation-structure-2026-09-03.md`
  §7.

**Carried residuals (worth a line, owed before the flip):**
- **R-CARRIER-REFLECTION — DONE (PR #705, 2026-09-02, test-only).** The fold-input carriers were verified
  by hand; they are now pinned by reflection. `TestFoldInputCarrierCoverageIsComplete`
  (`core/chain/floorbox_recompute_carrier_reflection_v5_test.go`) walks the transitive struct
  closure of the state-root fold's witness bundle (13 carrier types / 74 fields) and requires exact
  equality with the declared coverage (`r12CoverageTable` ∪ `foldInputCoverageTable`), so an added
  carrier type, an added field, or a stale row goes RED. Teeth demonstrated by injection.
  The R-FOLD-LIVE-STATE-READS fix added the `StateRootMaturityWitness.MatureEpoch` carrier field;
  it ships with its own `r12CoverageTable` row, so the pin stays green.
- **R-COLD-BOX-HARNESS — CLOSED as a permanent tier** (`core/chain/floorbox_recompute_coldbox_v5_test.go`).
  Every recompute gate had run on the chain that APPLIED the history, so the test tier shared the
  producer's blind spot — third occurrence in this spine (R1.3 fold-caught premise, class-P
  suppression, live-state reads). The tier drives the real entry on a `New(cfg)` box that never
  applied a block. New recompute gates run in it.
- **R-VERIFYBOND-WIRING — CLOSED.** The injected `verifyBond` is asserted once at the box entry
  (`ErrRecomputeBoxWiring`), so the #572 replay shape fails loud and named instead of as a fold
  mismatch three classes later.
- **R-ROTATE-EPOCH-LAST — DONE (PR #703, test-only): `rotateEpoch`-is-last-in-`apply` is now
  pinned by a drift guard** as load-bearing for `epochSet` order-independence (#621).
  Distinct from R-CARRIER-REFLECTION. `epochSet` order-independence
  (proven in #620) holds only because `rotateEpoch` runs LAST in `apply` (`core/chain/chain.go`),
  reading only the final post-block `bonded`/`slashed` state, so `epochSet =
  liveQualifiedSet(bonded, slashed)` is order-invariant by construction. A refactor that moves
  `rotateEpoch` before slash/bond application, or makes `liveQualifiedSet` read history rather than
  the two final maps, would silently break the SMT history-independence premise for `epochSet` and
  #620 would no longer hold. Owed before the era-4 format freeze (Boulder 3): a drift-guard
  assertion or test that fails if the freeze can observe pre-final state — **shipped**. PE ruling:
  `RULING-620-mature-epoch-order-independence-2026-08-28.md` ("Couplings the consult should carry
  forward").

**New Rocks opened 2026-09-02/03 (Boulder 1 — flip preconditions and pre-freeze items).** Each
carries its source; none is decided by a seat.

*The structure (the shape the box is built in):*
- **R-STRUCTURE-REDERIVATION — RATIFIED 2026-09-03 (owner: "I accept the recommendation"); BUILD
  OPEN, after R-STATEVIEW-ENUMERATION.** Build ONE accept composition
  `ValidateCommitV5(view, block)` over a three-valued `StateView`, both callers, all box doors
  unexported behind `WitnessValidateV5`; contract form becomes `box.Accept ⇒ node.Accept`. Five of
  five box defeats this cycle were **composition** errors (a tail reproduced without the
  precondition its caller established), not missed screens, so a shared predicate *set* does not
  close them. `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-predicate-rederivation-structure-2026-09-03.md`
  (option E), `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md`
  (option D).
- **R-STATEVIEW-ENUMERATION — CLOSED 2026-09-03 (closure GATED G-1…G-5; freeze scope CERTIFIED: ZERO leaves for safety).**
  The exhaustive inventory found the "exactly one non-leaf fact" sketch wrong by five (N1 height axis
  widened; N3 the K=8 ancestor walk; N4 the `LogRoot` half of the roots predicate; N5 `trustFloor`
  node-local; N6 `verifyBond` parameterised by uncommitted node config; plus a fourth class, block-local
  NOT hash-covered: `Atts`/`CommitRound`/`PrepareQC`/`Pruned`). The blind red-team hunt then found
  F2–F15 (13 accepted, F3 rejected) and **F1, a confirmed I5 break (R0.6)**. The closure cert
  REFUTES the four-leaf recommendation: every non-leaf fact closes with zero format change through a
  box-owned head record, because `WitnessValidateV5` already takes `parentStateRoot` as an
  unauthenticated driver parameter. Closure rests on a PARTITION, not a list:
  `modelcheck_state_completeness_test.go:76-151` machine-classifies every `Chain` field, leaving
  exactly two non-leaf containers (`blocks`, `revLog`). Two closure rules: the composition's input
  identity is the WIRE BYTES (F5+F10); `trustFloor` as a caller parameter is REFUTED — a raised floor
  skips the space-time re-verify (`chain.go:1801-1815`), so the box must refuse pruned blocks (F6).
  The one candidate leaf is `tagRevLogSize`, LIVENESS-only (see R3.4). Not walked: `blindtoken.Verify`,
  `vdf.Verify`/`manifest.VerifyProof`, the SMT library, `Reconcile`/`adopt`, `core/genesis`. Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/INVENTORY-accept-chain-state-view-enumeration-2026-09-03.md`,
  `/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-accept-chain-state-view-enumeration-2026-09-03.md`,
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-STATEVIEW-ENUMERATION-closure-and-freeze-scope-RESEARCH-CERTIFICATION-2026-09-03.md`.
  **Also filed from the hunt (definitions, not breaks):** F2 `rotateEpoch`'s activation tallies iterate
  the NEWLY FROZEN set (`chain.go:3494/3496`), not pre-state `epochSet` — a box/node split the
  composition must reproduce from the pre-state; F4 the K=8 nonce walk is TRUNCATED (`[1..8, 8…]`) —
  a fixed-length ancestor list would let a box accept forged bonded standing; F5 `Hash()` writes a
  non-wire memo (the accept chain mutates its input).
- **R-BOXENTRY-RESIDUALS — OPEN (box-entry round A, HELD unmerged; the HOLD is RATIFIED
  2026-09-03).** The exported-door surface is HELD by owner decision (zero production callers today;
  unexporting is free now and migration work once part B starts): merge the arithmetic
  (`quorumWeightTally`, `screenSupportSlashed`), keep every door unexported. Live findings owed with it: **N1** unqualified author credited by the frozen
  weight quorum, **N2** the O2 precedence bypass via the driver-supplied `HasHeldHead`, **N3** the
  backward/off-chain pin walk, **N4** the de-mature super-quorum crediting a mid-epoch joiner,
  **N5** the missed fatal out-of-scope replay leg, **N6** the over-broad O2 refusal, **N7** the zero
  `PinRecord` resetting the forward-progress guard, **N8** the fail-path lazy-fold DoS. Plus the
  §8 residual table's open set (R-SUPPORT-SLASHED-SCREEN, R-MALFORMED-DIVERGENCE,
  R-PIN-LABEL-ESCAPE, R-PIN-REANCHOR-POSITION, R-PIN-BUDGET-ESCAPE, R-FENCE-TABLE-DRIFT,
  R-BOUNDARY-PREDICATE-COVERAGE, R-BOX-ATTESTS invariant II / G-F). Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-floorbox-box-entry-roundA-4a394fd-RE-BREAK-2026-09-03.md`,
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-box-entry-round-A-fee43ba-DELTA-CERTIFICATION-2026-09-03.md`
  §8.

*The `LastCommit` carrier — R-BOX-ATTESTS. **O1, O2 and O4 are OWNER-RATIFIED (2026-09-03);
O3 is pending** (source for all four:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md`
§10). Built on `builder/lastcommit-carrier`, NOT merged.*
- **O1 — RATIFIED: the `LastCommit` carrier is an additive format field of the OPEN era**
  (`LastCommit []Attestation`, additive cbor key, `omitempty`, **folded into `Hash()`**). Validity:
  every entry verifies over `b.Prev`'s hash at `PhasePrecommit` at any single round (`CommitRound` is
  uncovered, so the rule must not bind to it), distinct ids, a pre-v5 block carrying the field is
  invalid, height 1's carrier is empty by rule, genesis carriers refused. Transition: seat each
  carried signer with `id != parent.ProposerID()` and `attesterQualified` evaluated against the
  child's pre-state; the child's own `Atts` write nothing; the frozen era-3 rule is left byte-for-byte.
  The carrier fold runs **before** this block's bond regs / TTL / slashes, pinned the way rotate-LAST
  is pinned. Disclosed: the seat lands one block late; a proposer can DELAY a seating but never FORGE
  one. The smaller same-block `SeatAdds` alternative is explicitly not recommended.
- **O2 — RATIFIED: the rollout rule, both paths.** The readiness stamp goes **3 → 5 directly; no
  release ever stamps 4** — so era-3's frozen format is retired **without ever running**, recorded in
  #632 as *frozen-and-retired-unrun* (a doc note; it edits no format). And: **no mainnet era
  activation, by tally OR by pre-latch genesis override, on a binary without the carrier** — the
  override bypasses `regVersion` entirely, so the stamp rule alone does not cover it. Note the
  enforcement limit found this session: a "refuse activation on a pre-carrier binary" check is
  **vacuous by construction** (any binary carrying the check carries the carrier); the predicate that
  actually binds is *the full v5 carry-list has landed*, which no binary can evaluate about itself
  except as the hard-coded stamp. The mixed-fleet exposure is safety-preserving (a pre-carrier node
  drops cbor key 18, computes a different `Hash()`, and **stalls loudly**; it does not commit
  divergent state) — the residual is `R-CARRIER-ROLLOUT-SIGNAL` below. Sources: §10 (O2) and
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md`
  §2, §4.
- **NOT re-opened (recorded so it is not silently reopened):** option **M** — re-basing the maturity
  metric on `bonded` instead of `validatorsSeen`. It would remove the "attested a committed block"
  anti-declaration property, which makes it a **published-claim** change (Sybil metric semantics),
  not a consensus fix. Declined by default; reopening it is a separate certification. Source: §10.
- **R-CARRIER-PARENT-BINDING — DEFINED 2026-09-03 (CERTIFIED direction; flip-gated; ZERO format change).**
  A silt consensus signature carries NO chain position (`consensusSigBytes = domain ‖ phase ‖ round ‖
  hash`, `chain.go:752-767`), so every consumer must supply position from state it trusts. The box
  binds the carrier to THE PARENT through a **box-owned head record** (`HeadRef`: parent hash, height,
  and — per R-LOGROOT-FORMAT-SCOPE — parent `LogRoot`), the same trust class as the `parentStateRoot`
  the driver already passes (`floorbox_v5.go:223`). Committing parent hash/height as leaves is
  REFUTED (hash circular — `StateRoot` is inside the `Hash()` preimage; height redundant). Bundling
  with `tagLastProposer` is REFUTED: there is no format delta. Also closes the height axis (13c). No
  `HeadRef` symbol exists on any tree yet; `AdoptPin` on the box-entry branch is the evidence the box
  can derive a trustworthy head. Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/LASTCOMMIT-CARRIER-residuals-composed-direction-RESEARCH-CERTIFICATION-2026-09-03.md`,
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-lastcommit-carrier-residuals-direction-review-2026-09-03.md`.
- **R-CARRIER-PARENTPROPOSER / `tagLastProposer` — RE-PRICED 2026-09-03: NOT a leaf, NOT pre-freeze.**
  `parent.ProposerID()` closes with ZERO format change through the box-owned head record (above); the
  enumeration closure cert supersedes the earlier "the ONE hard pre-freeze item" reading (its own §3.2)
  and the R-BOX-ATTESTS §6.2 fix direction. Remains a flip precondition (the class-A carrier fold must
  read it from `HeadRef`, never from a witnessed self-signed pair — ADD is unbounded). Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-STATEVIEW-ENUMERATION-closure-and-freeze-scope-RESEARCH-CERTIFICATION-2026-09-03.md`.
- **R-CARRIER-BYTES — DEFINED 2026-09-03: principle + formula CERTIFIED, VALUE GATED on a pony measurement; owner ratifies.**
  It is **not a security parameter** (the box STALLS on an over-size witness, so safety holds at any
  value); it is a build-immutable-#8 + box-liveness parameter, so the owner ratifies it on immutable
  grounds. Derive the bound from the **box witness cost on unauthenticated input** (the same schema as
  `adapters/tcpnet/tcpnet.go:63-72` and CometBFT `MaxBytes + MaxCommitBytes`), in BYTES not count
  (distinct ids are free). A frame-derived bound is REFUTED (20.7× loose); a state-dependent threshold
  is REFUTED (the #357 shifting-count shape). The PE review adds: the formula needs an **honest-carrier
  LOWER bound** too (the liveness cliff used to refute a minimum applies to the maximum). The ungated
  `anchoredPreSet` fold on `w.PreIDs` and the zero-caller `IngestBlockWitnesses` gate are the SAME
  defect on two more surfaces — one rule, three surfaces. A pony-class measurement does not exist yet.
  Sources: the composed-direction cert and the PE review above;
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R-membership-and-recovery-boundary-535-2026-09-03.md`.
- **R-CARRIER-PRUNED-HASH — OPEN, owed BEFORE the stamp raise (not a flip gate).** `Hash()`
  short-circuits to `b.Pruned`, and `Prune()` keeps `LastCommit` and `StateRoot`, so on a pruned
  block neither is signature-covered: `Pruned` is a linkage token, not a content commitment.
  Pre-existing to the carrier, which is in fact its best-protected member. **Owed:** prove the
  seen-fold never depends on a pruned body, and prove end-to-end that the first non-pruned
  descendant's root recompute catches a rewritten ancestor. Do **not** make `Hash()` cover pruned
  bodies — that defeats pruning (build-immutable #8). Source: the carrier delta cert
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/LASTCOMMIT-CARRIER-26977a4-DELTA-CERTIFICATION-2026-09-03.md`)
  §5.
- **R-CARRIER-GENESIS-DISPOSAL — OPEN, MERGE-BLOCKING on the carrier branch (MG-C).** The rule
  splits: genesis `Atts` must be **STRIPPED**, genesis `LastCommit` must be **REFUSED**. This
  corrects the round-A certification's own §2. Source: the delta cert §6.
- **R-CARRIER-CREDIT-DENIAL — RE-PRICED 2026-09-03: PRE-EXISTING on main; the carrier NARROWS it. GATED (window); minimum REFUTED; vector REFUTED.**
  Main seats from `b.Atts` (`chain.go:3364`), which is outside the `Hash()` preimage, so today ANY relay
  can deny a seating divergently (bounded by the quorum floor, `chain.go:2735`; at v4+ the same trim
  rejects the block, `era3validity.go:131`). The carrier shrinks that to one proposer, agreed. A carrier
  MINIMUM is REFUTED (mintability would depend on the proposer's stored non-hash-covered parent `Atts`
  ⇒ liveness cliff; + #402 forks). A set-indexed VECTOR is REFUTED (needs a validator cap ⇒ ceilings
  C2's measured domain ⇒ an M0 published-claim change; carried as **R-CARRIER-VECTOR-CAP**, owner).
  A multi-block inclusion WINDOW is GATED (owner: whether to buy it). Only dated item: a doc fix. Sources:
  the composed-direction cert and the PE review.
- **R-CARRIER-DOUBLESIGN-SLOT — RE-CLASSIFIED 2026-09-03: NOT a consensus-rule change; severity LOW.**
  Because `Atts` is outside the preimage, the evidence producer LIFTS a carried precommit onto the
  evidence copy of the parent's `Atts` (hash unchanged, `VerifyEquivocation`'s accept set identical,
  no format, no era gate). And `HeadCarrier` iterates `head.Atts` (two non-test callers), so
  `LastCommit ⊆ parent.Atts` by construction — the "seated on both forks, convicts nobody" premise
  cannot be produced by silt's own proposer. Build-spec: the lift must run BEFORE `signers()`;
  `e.A = *ab` aliases the `Atts` backing array; `Slashes` is hash-covered, so this couples to
  R-CARRIER-BYTES. Traps named for the Tester: SILENT-GREEN (adding `LastCommit` to the two loops
  verifies against the carrying block's hash) and FALSE-SLASH (a `(height, phase)` join; height derived
  from the carrying block's label). Corrects the `26977a4` delta cert §8.1. Residual
  R-DOUBLESIGN-TIP-BLIND (LOW). Sources: the composed-direction cert and the PE review.
- **R-CARRIER-ROLLOUT-SIGNAL — DEFINED 2026-09-03: CERTIFIED minimum; no part is a consensus rule; the override is LATENT.**
  The readiness tally already IS BIP-9-shaped; the gap is the flag-day override, and
  `Era4ActivationHeight` has **no `cmd/silt` flag** today. Minimum faithful mechanism (Cosmos
  `x/upgrade` schema): a named upgrade + a declared binary version → startup refusal, plus a
  self-naming stall on unknown cbor key 18. A strict CBOR decoder is REFUTED. **Release-runbook
  precondition:** any future `Era4ActivationHeight` flag lands with the named-upgrade check. Sources:
  the composed-direction cert (§ rollout), the PE review, and
  `/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-lastcommit-carrier-residuals-prior-art-2026-09-03.md`.
- **R-CARRIER-MODELCHECK — GATED, owed BEFORE the stamp raise.** The model-check tier must state and
  drive the seating **AGREEMENT** property under the carrier (every replica seats the same set from
  the same hash-covered carrier), not merely the carrier's own validity. Source: the delta cert §7.
- **R-LOGROOT-FORMAT-SCOPE — NEW 2026-09-03: a flip-gated SAFETY item; closes with ZERO format change.**
  The node requires BOTH roots (`era3validity.go:134-136`); every witness artifact covers only the
  `StateRoot` (`floorbox_recompute_stateroot_v5.go:501`, `readset_v5.go:23-38`, `statehash.go:29-30`),
  and `core/translog` has no frontier/append primitive. A composition that Accepts without reproducing
  `LogRoot` breaks `box.Accept ⇒ node.Accept`. Direction: verify-not-recompute (consistency +
  per-leaf inclusion), carrying `parentLogRoot` in the head record and STALLING on revocation-bearing
  blocks. Liveness needs the authenticated log size — see `tagRevLogSize` under R3.4. Sources: the PE
  inventory (N4), the composed-direction cert (§ corrections), the PE review §5, the closure cert.
- **R-HASH-LITERAL-PIN + R-V5-TAGSET-EQUALITY — NEW 2026-09-03: two HARD merge gates on the carrier branch (CD-0).**
  Main's `Hash()` `unsigned` literal folds `IssuerKeys` not `LastCommit` (`chain.go:667`); the carrier
  branch's folds `LastCommit` not `IssuerKeys`, has no `IssuerKeys` field at all, and is behind main by
  2,907 lines in `core/chain` (base `1adca0f`); both branches rewrite the same box-entry file. A naive
  merge holes the signed body. Gates: (1) a reflection pin that the `Hash()` literal names every
  hash-covered field; (2) v5 tag-set equality against `statehash.go` (`issuerKeyCommit` joined the set
  in #711). **Owner: rebase the carrier branch onto main NOW.** Verified by the planner, the Researcher
  and the PE independently. Sources: the PE inventory, the composed-direction cert (CD-0), the PE review.

*Fork-choice, canon and inventory (O3 / #558 family):*
- **R-FORKCHOICE-WEIGHT (O3) — RATIFIED 2026-09-03: Direction T (owner: "Direction T"). BUILD
  OPEN, research-gated on the I5 restatement cert.** See the *Decisions owed* block for the
  recommendation and its two sources. Direction T means: remove the
  weight comparison from `heavier`, delete `Weight()` / `blockWeight()` / `anchorWeight()` /
  `Config.AnchorWeight` (no operator-visible contract — no CLI flag exists), **preserve and promote
  the §1b height preference**, and re-ground the three legacy `Reconcile` fixtures on height →
  hash. Do not leave the term inert: a dead consensus quantity that survives a retirement is exactly
  how the third bare-hash verify site happened. **The next four Rocks ship in the SAME commit as
  whichever direction the owner picks.** The reopening condition is narrow and named: evidence of a
  shipping posture in which `FinalizedHeight()` lags `Head()`.
- **R-558-VERIFIER-INVENTORY — OPEN (Tester).** A standing grep-shaped gate that **no attestation is
  verified outside `verifyAtt`**. `#558`'s repair swept two of three bare-hash verify sites;
  `blockWeight` is the third and was missed. Under Direction T the gate is satisfiable by
  construction because the third site is deleted, so it ships **with the T commit**. Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/O3-fork-choice-weight-R-vs-T-RESEARCH-RECOMMENDATION-2026-09-03.md`
  O3-R7.
- **R-INTERLOCK-GATE — OPEN.** The binding interlock ("the `blockWeight` signature verify is never
  repaired alone") has **zero test enforcement**: the PE applied exactly the forbidden one-line
  change and `./core/... ./sim/...` was fully GREEN, EXIT=0. A gate is owed in the same commit as
  whichever O3 direction lands. Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-O3-fork-choice-weight-R-vs-T-2026-09-03.md`
  §5.
- **R-FORKCHOICE-RAMP-GUARD — OPEN (Tester), small.** Delete the `Weight() > 0` assertion at
  `core/chain/forkchoice_ramp357_test.go:66-69`; do **not** repair it. It asserts a property
  production does not have and is green only because the fixture hand-builds `Version: 1` blocks
  with era-1 attestations inside an objective config — a guard that passes only under an attestation
  era no node mints is not a guard. Invariant D in the same file is the part that guards #357 and it
  stands. Source: the O3 research recommendation §3.5 item 3 / O3-R3.
- **R-I5-TEXT-AND-CLAIMS-LEDGER — OPEN, RESEARCH-GATED.** Two canon/ledger edits ride the O3
  decision: the I5 restatement in `docs/design/consensus-invariants.md` (editing an invariant's
  stated rule is inside the research gate — the PE explicitly does not assert that certification),
  and `docs/design/claims-ledger.md:47` ("objective fork-choice heals a partition to the
  heavier-standing chain"), which has been **false since 2026-08-16** and is backed by an era-1 unit
  test no production path can produce. Re-word it to name the real mechanism: quorum finality plus
  height. Sources: the O3 PE ruling §6 and the O3 research recommendation §3, O3-R2.
- **R-O4-CANON-HASH-COVERAGE — RATIFIED in substance, NUMBERING PENDING.** Amend canon with the
  hash-coverage rule covering **both** the transition and the fork-choice weight, with both scars and
  both code sites; plus the #632 *frozen-and-retired-unrun* note. Whether it widens **I5** or mints
  **I6** is the owner's; both seats agree the split is non-substantive. Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md`
  §10 (O4).

*Gate quality, coverage debt and small items:*
- **R-AST-PIN-GLOB — OPEN, small.** The R-FOLD-LIVE-STATE-READS AST pin's glob does **not** cover
  the file where three of five box defeats live: `floorbox_recompute_v5.go` and four others are
  outside it. Direct evidence that AST gates fail by scope. Widen the glob and re-run. Source: the
  PE structure ruling §8 item 2.
- **R-INVENTORY-HAND-LIST — OPEN (Tester).** A gate that claims to cover "every X" but enumerates X
  as a hand-written literal is green because the list is shorter than X, and worse than no gate
  because the claim is now published in a test name. Demonstrated on the eleven-door legacy fence
  (which misses `RecomputeMatureNowStreaming`) and mirrored by R-FENCE-TABLE-DRIFT. Derive the
  surface from the code, not from a list. Source:
  `/Users/andrewedmond/Claude/claude/silt/.claude/agent-memory/tester/scar-inventory-gate-is-a-hand-list.md`.
- **R-S5-STRING-REGISTRY — OPEN (Builder + Tester); scar at count 2, THIRD-TIME RULE ONE AWAY.**
  An announced operator string is a contract (S5), and an upstream early-return breaks it while every
  unit test stays green (instance 1: `freeload: ON`; instance 2: the R0.4b `NOT banked` refusal). The
  proposed gate is a registry of contracted operator strings (27 seed entries verified present) plus
  a unit-tier presence test with a teeth test, keeping the e2e assertion for every registered marker
  — the registry proves the string is still in the SOURCE, never that it is still REACHABLE. Full
  buildable spec:
  `/Users/andrewedmond/Claude/claude/silt/.claude/agent-memory/tester/scar-observable-log-contract.md`.
- **R-SWARM-NOTBANKED-DEAD — OPEN, LOW.** After the lane-off legibility fix and PE H-2, the
  `NOT banked` branch in `cmd/silt/swarm.go` may be genuinely unreachable on the shipped daemon. It
  needs either a reachability argument or removal — a dead branch must not carry an S5 contract.
  Source: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md`
  §5.
- **R-E2E-ERA4-FIXTURE — RE-SCOPED 2026-09-03: NOT independently schedulable; a deliverable OF the stamp raise.**
  Era-3 and era-4 are DARK on every real network (`NewBondReg` stamps v3 vs tallies needing 4/5; no
  production activation-height setter; genesis stamps v2), so the fixture cannot green by tally on any
  binary until the stamp raise, and never by an activation override (owner-ratified). The
  "owed" composition gate ALREADY SHIPPED (`sim/demand_composition_test.go:52`, both arms incl. the
  second delivery at `:144-148`); what is missing is its ABLATION twin. Owner: accept the e2e cost
  increase at the stamp raise. Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-three-small-designs-issuerkey-pop-era4-e2e-fixture-smt-oracle-2026-09-03.md`.
- **R-ISSUERKEY-POP — RE-DIRECTED 2026-09-03: the PoP is NOT the fix; bind `issuerID` into `demandMsg` (off-chain); reserve the format slot at the stamp raise.**
  Binding the key FINGERPRINT is VACUOUS (B registers A's actual key bytes, fingerprint identical).
  The sound close is off-chain: sign `issuerID ‖ epoch ‖ serial` in `demandMsg` — no committed byte,
  no consensus cost (research-gated as a D-DEMAND change; PSS-vs-FDH routed). Nothing breaks for
  already-registered keys because none can exist (eras dark). Coupling: (B) closes the attack but a PoP
  is a FORMAT SLOT in `IssuerKeyReg` — reserve it inert at the STAMP RAISE or it freezes with no room.
  Owner: build the slot now vs reserve-only. Until then, correct the two comments claiming RFC 9578
  fidelity. Source: the three-small-designs ruling above;
  `/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R0.4b-C3-blind-RSA-epoch-binding-2026-09-03.md` C-4/Q3.
- **FP-1 · `Bank.spent` persistence — OPEN, a flip precondition.** The in-memory spent guard has
  less durability than the thing it guards; narrowed, not waived, and re-armed the moment witnessed
  demand confers any value. Source: the composed cert Residuals; the FP-1/FP-2 text is CERTIFIED
  faithful in `R0.4b-C3-271ab81-G3-G4-GD-DELTA-CERTIFICATION-2026-09-03.md` §4.
  *(The 2026-09-03 ledger-durability ruling REFUTES the earlier "mirror crash window is a pure
  under-pay, self-heals, owes no gate" reading on all three clauses — see FP-2 below.)*
- **FP-2 · the redeem ATOM (was "write-ahead across the guard file and a ledger store") — RE-DEFINED 2026-09-03; sequenced R2.13 → R2.10 → FP-2.**
  The brief named the wrong invariant: there is NO double-pay (`r04b_c3_crashwindow_test.go:84-89`, Σ
  unmoved). The residual is a **LOST SUPERSEDE**: the reversal at `delivery.go:449-454` sits OUTSIDE
  the certified `{guard entry, payout}` atom, so the server keeps 58,720,256 against an honest 43,750
  (`:97-106`); the same atom reproduces the identical residual. The atom is the WHOLE redeem (five
  mutations). Direction 2 (pay-then-append) stays REFUTED. Coupling: persisting the ledger BEFORE F8
  (R2.10) upgrades the watermark poison from process-lifetime to PERMANENT — F8 gates the BUILD.
  Research-gated: the atom boundary and replay idempotence (conservation). Owner: whether the ledger
  gets a durable store before the RC at all (PE: close FP-2 by scope; land R2.13 and R2.10 anyway).
  Source: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-ledger-durability-family-FP2-R2.13-R2.10-2026-09-03.md`.

- **SMT app-layer keyspace-injectivity oracle — DEFINED 2026-09-03: a decoration, and the invariant its safety rests on is FALSE.**
  `statehash.go:52-55` / `:99-101` claim "map raw keys are never empty"; `c.spent[string(e.Token.Serial)]`
  (`chain.go:3254`) has zero validation, and `validateEntry`'s token branch is gated on `tokenQuorum > 0`
  which no `cmd/` caller sets, so `Key("spent\x00","")` == the scalar form. No live collision; the margin
  is one tag name, and the same serial is a floor-box read-set key (`readset_v5.md:333`, the A2 class).
  Build: a `Serial` non-empty/length validity rule (research-gated: validity + immutable #8) and a
  defect-injected gate (inject the empty serial, watch RED). Freeze-blocker stands. Source: the
  three-small-designs ruling.

#### Boulder 2 — Turn the economy on, prove it under adversary · depends on Boulder 0
All solvency claims are sim-only today (economy default-off, no live enable path). An
economy-off HEAD certifies a network nobody runs. Design:
[`docs/thinking/2026-09-01-economy-observability-design.md`](docs/thinking/2026-09-01-economy-observability-design.md).

- **R2.1 · Economy observability MVP + node-local APIs** · Builder · L (sliceable) ·
  **Slice 6a DONE** (shipped in #689 / commit 94c5c04). 4 local-exact panels (my solvency /
  am-I-profitable / durability self-funding / wash self-check), extending the existing
  `/api/status` durability block; ships cert-free, economy-off. The per-node `repairsDone`
  counter landed with it (`core/credit/credit.go:71`, incremented at
  `core/credit/escrow.go:177`), unblocking the repair-work Gini (R2.2).
- **R2.2 · Full observability set + testable telemetry gate** · Builder + Tester. Serve-work
  Gini AND repair-work Gini (separate), per-tier margin, live `g`, funded-horizon-to-expiry,
  wash-detection; network panels via the DHT crowd-estimator (knowability tiers).
- **R2.3 · A4-fix + economy-ON packaging — DECISION RATIFIED: separate; A4 fix first.** The
  conservation re-cert (R0.4) lands before the economy default flips regardless of packaging.
  ★ The flag that arms A4 is `-accept-delivery-receipts`, not `-economy` — sequence it behind
  the re-cert; do not let it ride silently in an economy-on PR.
- **R2.4 · Economy-ON default flip — DEFINED 2026-09-03: lands AFTER R2.14 and R2.9, phased.** Under
  the flat lane the flip reads to a pony as a 99.93% pay cut on a 64 MiB object. Order: correctness gates
  (incl. FP-1) → economy-OFF baselines incl. the pre-flip Gini → canary → default. After Boulder 0 +
  R0.4 cert. Source: `/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-boulder2-economy-definitions-2026-09-03.md`.
- **R2.5 · C-5 G2 RAM measurement at production chunk — DONE (2026-09-01): 1024 MiB resident.**
  Measured locally at production chunk (16 × 64 MiB), +~512 MiB reclaimable (1536 MiB
  allocation-inclusive peak). Consequence: on a 2 GB pony ONE repair fits, TWO concurrent
  prod-chunk repairs OOM. Build-day: confirm a repair-concurrency limiter in
  `core/node/repair.go` (PE found none); fold into the owed node-store coexistence test.
- **R2.6 · Repair-payee model — DECISION RATIFIED: HOLD the ratified design; convert to a
  G2-gate.** The `selfHold` conditional payee ALREADY pays the reconstructor where safe
  (2026-08-19 ruling); the real surface is only the domain-collision case, and the binding
  constraint is COST (the G2 RAM spike), not incentive. Any re-open needs a cert grounded on
  the measured G2 (**cert-gated**, re-opens D-S7). Real seam: who re-endows cold-escrow + the
  missing per-tier repair-work-Gini telemetry.
- **R2.7 · Economy-ON adversarial-solvency verdict + attack pass — SCOPED 2026-09-03.** Five
  solvency inequalities × seven attacks; the two unpriced today are **A2 supersede-suppression**
  (highest; now CERTIFIED as a live incentive break, R2.9) and **A5 cold-start capture**; S5 must
  include escrow recovered by self-repair. **Blocking telemetry:** `servedBytesWitnessed` /
  `servedBytesUnwitnessed` (the A2 detector) and `bountyPaidToEscrowFunder` (the A4 detector) — neither
  exists; without them R2.7 grades a benign workload. Researcher (NEW-CERT, cert-gated) + red-team,
  after R2.14 + R2.9 + R2.2. Feeds the #183/R4.4 brief. Source: the economist advisory.
- **R2.8 · Cold-repair funding path — DEFINED 2026-09-03.** Reserve-aware repair scheduling + early
  cliff disclosure open to ANY funder + R2.9's expiring remainder. A network pool is REJECTED (a
  censorship lever, drainable); never a mint. Today the caretaker pays 1024 MiB of RAM and THEN learns
  the escrow is dry (`repairclaim.go:216`). Five inputs stay ASSUMPTION until live data: `B_bootstrap`,
  honest arrival rate, object-size distribution, willingness to re-endow, escrow recoverability. After
  R2.2. Source: the economist advisory.

**New Rocks opened 2026-09-03 (Boulder 2 — economy):**
- **R2.9 · D-POD-KNOBS re-pricing — CERTIFIED as a LIVE incentive break; direction GATED (G-1…G-6); owner ratification owed; R2.14 is a PREREQUISITE.**
  **Certified:** a server strictly prefers NEVER banking a witnessed receipt above B = 50,000 bytes —
  payoff `0.875·(B − fee)`: +13,594 at 64 KiB, +58.7 M (1,342×) at 64 MiB — and suppression is one
  default-off flag (`daemon.go:74`). **B3 conservation is INTACT** (conditioned on a banked receipt);
  what breaks is incentive-compatibility of accept; no shipped gate pins it. **Certified with scope:**
  `S/R ≥ 24·(B/fee)` ⇒ 32,212 at 64 MiB on the witnessed lane only; turning the conserved lane ON is a
  1,342× durability DOWNGRADE, voiding knob 1's rationale. **Theorem:** D-S7 holds iff the PRICE is
  byte-proportional; no clamp or ordering fix restores it. **Direction (GATED):** PayWord-denominated
  per-increment delivery settlement (no new primitive class), `Ledger.fee` split into publish anti-spam
  vs delivery settlement. Gates: G-1 parity STRICT (`p > r·U`; 0.763 today, at U = 64 KiB, r = 1 ⇒
  p = 65,536); G-2 the clamp at the CREDIT site (clamping `p.net` re-opens the money pump); G-3
  conservation never rests on a caller-supplied budget; G-4 re-derive `maxPaidSerial`; G-5 the numéraire
  rescale covers all seven balance constants; G-6 remainder-to-escrow out of scope. **Blocking residual
  is AFFORDABILITY:** at parity a 500,000 grant buys 488 KiB of fetch, ever (a build-immutable-#4
  regression introduced by the fix); `r ≤ grant/B_bootstrap`, `B_bootstrap` UNMEASURED. Alternatives
  (flat self-mint; cap the lane; rely on bonded fetchers) all REFUTED. Owner: the direction under
  G-1…G-6; the interim exposure; R2.9 before R2.4; strict parity vs `r = 0` on the witnessed path; the
  two measurements. Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R2.9-D-POD-KNOBS-delivery-settlement-repricing-RESEARCH-CERTIFICATION-2026-09-03.md`,
  the economist advisory.
- **R2.10 · F8 — the ledger must own a CHAIN-ANCHORED epoch — DEFINED 2026-09-03; second in the R2.13 → R2.10 → FP-2 order.**
  "Chain-anchored ≠ monotone": objective mode forbids the head falling (`chain.go:3888-3894`) but
  legacy `heavier` decides on weight first and `adopt` swaps the block slice, so a heavier SHORTER fork
  can lower `chainEpoch()` — F8 must **re-source the monotone latch**, not delete it (moot for the
  weight term once O3-T lands; the height leg stays). Separately `-epoch-blocks 0` (`daemon.go:119`)
  makes the epoch permanently 0, the sweep never fires (`delivery.go:542-544`), and the lane BRICKS at
  65,536 paid deliveries — the epochs-disabled denomination is a security parameter (research-gated).
  F8 gates the BUILD of any persisted ledger, not only its deployment. The faucet limiter keys on the
  node's monotonic clock instead (R2.12), which dissolves the F8 block on it. Sources: the
  ledger-durability ruling; the composed cert §5; the G3/G4/GD delta cert §4.
- **R2.11 · R0.4b-11 — no peer-submit path for an issuer-key registration.** An attest-only
  validator's key is never committed. Fail-closed, liveness only. Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03.md`
  (Residuals → Open).
- **R2.12 · Faucet rate limit — DEFINED 2026-09-03 (owner calls of 2026-09-03 stand: `grant = 500_000`, rate-limit the faucet, `RequireBondedFetchers` default OFF).**
  The limiter is LOCAL ADMISSION CONTROL, not a consensus rule: key it on the node's monotonic clock
  (never the ledger watermark — F8). Gate a separate `Grant(id)`, never `Register` (idempotent ⇒ a
  denial at Register permanently excludes an honest fetcher). Cap binds at 1,311 grants/epoch;
  100/epoch = 0.076× cap; a guard-fill grief costs 6,554 grants = 66 epochs at that rate. Evolving-tier
  by a three-part test the Invariant-A guard checks mechanically. **Composition warning (relay cert):**
  R2.12 plus a non-negative-payer check closes the relay mint but turns the relay lane into a 100%
  denial — there is no funded honest path through it until R2.14. Sources: the economist advisory;
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md`;
  `/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-R0.4b-cap-griefing-grant-and-bonded-fetchers-2026-09-03.md`.
- **R2.13 · `R-COMPACT-ORPHAN` — DEFINED 2026-09-03: real, cheap, NOT research-gated; FIRST in the R2.13 → R2.10 → FP-2 order.**
  Measured by the PE: after a successful rename, a failed post-rename `OpenFile` leaves the append
  handle on the unlinked inode; write AND fsync through the stale handle return success; `Load` never
  sees the record; `Compact`'s error is discarded at the sweep call site. Failure direction is an
  OVER-pay, once per epoch since C-7. Fix: **open-before-rename** (fd swap), plus the missing port
  clause at `ports/ports.go:361-363`. Do NOT blanket fail-closed at `delivery.go:559` — a pre-rename
  failure is benign and over-correcting is a self-inflicted liveness break. Tester gate: inject the
  post-rename open failure, assert the next append is visible to `Load`. Ships with the fix to the false
  ROADMAP FP-1 parenthetical. Source: the ledger-durability ruling.
- **R2.14 · Relay-lane prepayment ANCHOR — NEW 2026-09-03: a PREREQUISITE of R2.9; the fix for R0.7; owner ratification owed.**
  Per-node conservation is always AUTHORIZATION-anchored (privacy guard (ii) makes "debit the payer"
  unimplementable on any topology; `RedeemDeliveryCredit` debits no one either, `delivery.go:512-516`).
  The relay's anchor was specified 2026-08-27 (Q4(a)) and NEVER BUILT: `RelayOpen.Funding` is a bare
  fetcher-set int (`wire.go:20-24`); the shipped PayWord chain is missing two of Rivest–Shamir's four
  steps. **CERTIFIED direction:** anchor the chain to a blind-signed, issuer-verifiable prepayment,
  bilateral form (issuer == relay), gates G-A1…G-A5; zero new primitive class; NO-TTP preserved.
  Escrow binding REFUTED; bonded ephemeral REFUTED; on-chain form contradicts Q5. Invariant to gate
  (INV-RELAY-CONS): `settled(R) ≤ Σ face(spent anchors)`, each verified / spent-once /
  `ChargePublish`-backed on the PAYING ledger; per-session ledger total unchanged. Five RED-first gates,
  none pre-funding the fetcher; the three existing pair-total tests are REWRITTEN. Face value caps a
  session at 195.3 MiB vs `MaxSessionBytes` 1 GiB → allow k ≤ 6 credentials. **Until it lands:** the
  flag stays default-off AND settlement pays 0; correct five false claims in `relay.go:16-61`.
  **RT-RELAY-3 (MsgRelayPay preimage-walk CPU DoS, ~1083× byte→ms) is NOT closed by this** — promote
  `Verifier.walkSteps` (`payword.go:136-144`) to an enforced per-session budget S (Builder + PE).
  Side finding: `creditSpent` (`node.go:626`) has no cap/sweep/eviction on a shipped lane. Sources:
  the relay-lane fix cert above;
  `/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-relay-lane-session-grant-and-byte-price-2026-09-03.md`.
- **M_seen — the pony-class value is still OWED (carried).** The class-M streaming verifier (PR
  #709) removed RSS as the binding ceiling; the remaining ceiling is **TIME**, the O(N·log N)
  compute floor streaming does not remove. The cap value must be derived from a pony-class
  measurement, and the transport budget must be stated with it. Source: `CHANGELOG.md` (Unreleased,
  the class-M streaming verifier entry) and
  `/Users/andrewedmond/Claude/claude/silt/.claude/agent-memory/tester/class-m-maturity-fold-cost-measurement-2026-09-02.md`.

#### Boulder 3 — Freeze prerequisites (era-4/v5) · parallel · freeze-gated · was Rock 3
- **R3.1 · Close/own the SMT second-preimage / domain-separation residual** · Builder (scope to
  hashing first) + crypto/Researcher confirm. Load-bearing: a leaf/internal second-preimage
  would defeat the fold-OldValue soundness Boulder 1 relies on. Design (NO hash change; the
  library domain-separates by construction for silt's fixed-width leaves):
  [`docs/thinking/2026-09-01-smt-domain-separation-close-design.md`](docs/thinking/2026-09-01-smt-domain-separation-close-design.md).
- **R3.2 · Close the oracle probeUncovered debt fully (from R1.6)** · Tester. Freeze gate.
- **R3.3 · Record PayWord/RegCap re-derivation dependencies** · doc-only; re-derive only if
  the bond-proof size / N² cost residual moves (measured numbers in the **Residual backlog**
  below, "Bond-proof reply size / N² cost"; formerly #299).
- **R3.4 · era-4/v5 format freeze — DECISION RATIFIED, REVISED 2026-09-03: still at the RELEASE
  CANDIDATE (not end-of-PoD), and it is now the **stamp-raising** release that the flip and the
  external B8 pass come AFTER, not before.** The earlier "DECOUPLED from the flip; the flip proceeds
  pre-freeze" clause is **superseded**: R1.8 lands after the freeze and after B8 on the frozen
  artifact (see the sequencing constraint at the top of the Boulders). Owner-call, after the field
  set settles (R1.4) + domain-sep owned (R3.1). The prior "freeze at end-of-PoD" framing remains
  superseded.
  **Pre-freeze carry-list — RE-PRICED 2026-09-03 by the enumeration closure cert: ZERO leaves are required for SAFETY.** What remains in freeze scope: **`tagRevLogSize`** (at most ONE leaf, LIVENESS-only — without it a floor box stalls terminally at the first takedown block after any pin; owner ratifies) · the `Block.IssuerKeys` per-block count cap (PE H-1) · the **`Slashes` per-block encoded-BYTE ceiling** (SHIPPED with R0.6, un-era-gated — it is a validity rule in every era, not a v5 leaf; what remains for the freeze is only that its VALUE be owner-ratified before the format is frozen) · **`R-AAXIS-TAG-RESERVE`** (one line: reserve the A-axis tag prefix so a future partition leaf is not a prefix collision — it does NOT avoid the era) · the O3 decision (RATIFIED T). **Dropped from the leaf list:** `tagLastProposer`, the parent hash/height, the K=8 hash window — all close with zero format change through the box-owned head record; R-membership (retire the whole-set folds is a leaf change ONLY if the C2 re-shape needs it — see its Rock). **Owed before the stamp raise, but not before the freeze:** R-CARRIER-PRUNED-HASH's end-to-end test, R-CARRIER-MODELCHECK, the `R-E2E-ERA4-FIXTURE` upgrade, the `IssuerKeyReg` PoP format slot (R-ISSUERKEY-POP).
  Sources: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-predicate-rederivation-structure-2026-09-03.md`
  §7, `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md`
  §9–§10, `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R0.4b-C3-close-271ab81-final-2026-09-03.md`
  §3.

#### Boulder 4 — Standing gates + M0 endgame (post-PoD hardening) · was Rock 6
- **R4.1 · PoD demand→standing bright-line gate (fires per PoD increment)** · Researcher cert
  on trigger (**cert-gated**). Any increment wiring served demand toward standing re-opens the
  C1 discount (γ→1/N, #182). A review gate, not scheduled work.
- **R4.2 · The A-axis (operator/domain) — RE-SCOPED 2026-09-03: MEASURE / PUBLISH / fix two DHT-layer defects / hand to B8 AS-IS; do NOT wire A3. Owner ratification owed.**
  Direction cert: GATED on A3 (observed-address partition ratified by a ⅔-bonded attestation quorum) /
  CERTIFIED on "measure, publish, do not wire." **The dilemma:** additive-only ⇒ `C_honest` unchanged
  (the A-axis has NO reward consumer in silt); changes `C_honest` ⇒ a hard gate on routing reachability
  ⇒ build-immutable #3 (`TENETS.md:646`). A3 occupies no third position, and its quorum is a CENTER
  in two places (a ⅔ adversary attests honest validators into one group and keeps the launch anchors
  permanently). Sealed-plot domain binding is INERT (the seed is already identity-bound,
  `bond.go:164-172`; it also strands committed `bondRootOwner`/`byRoot`). C1 text: NO change; only
  C2 changes, toward honesty. `M_cluster = declared/observed` REFUTED as a metric (B1 defines M as the
  adversary's ratio). **Pre-flip, no-consensus-change program:** print `NakamotoDomains` /
  `DistinctDomains` (computed, not printed, `daemon.go:1029-1031`); four pre-build refutation thresholds
  in the cert. **New live finding (re-priced, → R4.4 brief):** a bonded adversary can DECLARE an honest
  validator's published domain and suppress its maturity at one `MinBond` per collision
  (`chain.go:2364-2365`) — cheaper than anything A3 defends. Era: `bondDomain` is an ERA-3 leaf
  (`statehash.go:174`); `R-AAXIS-TAG-RESERVE` on the R3.4 carry-list. Residuals unverified: ASN price;
  whether a validator endpoint is independently dialable by any bonded peer. Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R4.2-A-axis-operator-diversity-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md`,
  `/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R4.2-A-axis-operator-diversity-prior-art-2026-09-03.md`.
- **R4.3 · Continuous internal red-team hunt on the not-yet-run backlog** · red-team → feeds the
  external seat. Backlog: class-P compound-block ordering; bondreg full path; DHT/eclipse/A-axis
  layer; long-range/weak-subjectivity checkpoint; relay/PayWord economy; churn/restart
  everMature under the R1.2 refactor.
- **R4.3a · DHT domain-0 exemption + the single `-domain` flag — NEW 2026-09-03 (from R4.2; Builder, small; not research-gated).**
  `core/dht/table.go:44-45` never caps domain 0 — an exemption from an eclipse defence that geth and
  Bitcoin Core key on the OBSERVED address; and `daemon.go:316-317` feeds ONE `-domain` flag to both the
  eclipse cap and the consensus metric, whose consumers want opposite defaults — a literal
  build-immutable-#3 violation. Fix both; a distinct R4.3 hunt target. Source: the R4.2 direction cert.
- **R4.4 · External red-team vs the C1 + C2 composition and the seven §7 seams (#183) — THE M0
  close gate.** This is the RC-defining gate, and **it is the same pass as R1.7** — one external B8
  engagement, not two. **RATIFIED 2026-09-03: it runs at the RELEASE CANDIDATE, AFTER the era-4/v5
  format freeze (R3.4), against the frozen, still never-Accept artifact; R1.8 (the accept-flip) lands
  after it.** A fresh, no-memory EXTERNAL red team (self-graded
  does not count — B8 requires the certifying adversary to be external) attacks the BUILT system
  and must find no strategy that (a) earns quorum-controlling standing for less than `q · C_honest`
  (**C1**), (b) concentrates bonded weight past capture under adversarially-skewed measurement
  (**C2**), or (c) breaks one of the seven `docs/design/m0.md` §7 composition seams (re-pricing/wealth
  residue; cold-start scaffolding-capture; real-demand > wash-demand; privacy↔attribution linkage;
  operator-clustering heuristic; time-axis gaming; new liveness/griefing surfaces). The unit of test
  is the SYSTEM, not a primitive — a primitive failing a standalone Sybil-proof test is Douceur
  (expected). A seam held in tension (bounded cost, documented residual) is a PASS; a seam silently
  assumed closed is the failure mode. Findings are triaged/fixed to the build-immutable bar (unit +
  integration + e2e + inverted PoC) and re-attacked until a clean verdict. Brief:
  `docs/reviews/m0-redteam-brief-2026-08.md`. Pairs with the multi-machine field test (#52) to render the
  verdict. This is the gate to declaring M0 *held* (not merely built) and defines the R1 field
  grade → V1 endgame. #183's close condition is MET and the issue carries the evidence — the close
  is the owner's call, deliberately held.
- The self-declared A-axis (R4.2) is an explicit SECOND external B8 target once wired.

**Watch-items / standing gates (not scheduled):** bond-floor vs pony-disk ratio (a dashboard
row); owned-residual doc lines (SHA-256 pinned, store-free verify, R3 16 KiB cap); PayWord
re-derivation. **A4 fix (Boulder 0) is Third-operator settlement's predecessor** — the old Rock 2
(third-operator committed settlement, DEFINITION, #658) is unchanged and attaches its
demand→standing bright-line to R4.1.

<details>
<summary><b>Superseded Rocks (2026-08-19→08-31 overlay — kept as history)</b></summary>

> **⚠ Reconciliation flag (2026-09-01).** The old Rock 1 read *"the accept-flip is the single
> remaining step."* The 7-seat audit found that FALSE: the flip has a witness-soundness spine
> (re-anchor + refute + cert table + gates + external pass — Boulder 1's R1.0–R1.7) it never
> named, and classes P/A/B currently accept forged witness values because they are not
> Resolve-anchored. The flip is now R1.8, gated on that spine. Old Rock 4 (#406) folds in as
> R1.5. This flag preserves the earlier framing and records the correction.

1. **Trustless floor box** (D-TIERING keystone / "lane 1"). **IN PROGRESS — the recompute
   is built and merged; the accept-flip is the single remaining step.** The witness read-set
   producer (Part A, #656) and the additive floor-box v5 validation-mode scaffold (Part B1,
   #657) are merged; the box still holds `indeterminate-trustlessly` (never wrongly accepts).
   **R-boundary DECIDED (owner-ratified 2026-08-31, decisions.md):** the **heavy / fully-trustless**
   posture — the box reproduces every validity predicate, it does not re-derive the state root and
   trust finality. Whole-set committed reads are backed by MTH digest-root leaves; **≥4** carry a
   recompute reader (`bondedRoot`, `epochSetRoot`, `qualifiedRoot`, `validatorsSeenRoot`), and the
   committed v5 format emits all five (adding `slashedRoot`, F1). **The `apply()` transition set is
   reproduced and merged:** the O(payload) HYBRID recompute (payload/`dueBucket`-derived write-set +
   the R-fold over changed paths) covers the E/R spine and classes S/B/T/A/P plus the class-M
   maturity latch — O(payload)+O(registry), via a changed-digest write-set primitive. A
   **write-obligation ledger** (an emission-keyed differential guard) confirms **the committed-leaf
   diff a real `apply()` produces equals the key-set the recompute folds (28/28 committed leaf kinds)**,
   and self-detects a future added or renamed leaf. **Cost:** the witness bundle is measured **flat in
   the total state** — O(payload)+O(log N), not O(whole-state) — so the recompute fits the 1-CPU/2-GB
   "pony" budget. **Remaining: the accept-flip** — wire `WitnessValidateV5` to the merged recompute and
   return Accept-iff-all-predicates-pass. **Owner-ratified (pre-launch)**; gated on the #406
   model-check cert and the accept-flip gates. One step to a validating pony.
   Maps to: **Boulder 1** (the flip is R1.8, not a single step).
2. **Third-operator committed settlement** (the next PoD economic frontier). **DEFINITION.**
   Cross-operator settlement in committed state, replacing today's bilateral in-memory
   ledgers. Design-space strawman filed (#658, DEFINITION only); a **gated economic
   mechanism** (γ→1/N firewall #182 + conservation) under blind Research + PE evaluation.
   Couples to the v5 format. Maps to: the PoD frontier (Boulder 2 economy + Boulder 3 freeze);
   unchanged; attaches to R4.1.
3. **era-4 / v5 format freeze** (the closing act of Proof-of-Delivery; a second practiced
   era freeze). **DEFERRED BY DESIGN.** era-4/v5 is kept OPEN-ENDED — no live chain, and
   PoD may still reshape witnessable state (owner-ratified 2026-08-30). The R-boundary digest
   leaves are ratified and merged (F1, five roots); the hard gate is the COMPLETE exhaustive
   digest-root set — the mechanically-enumerated whole-set reads — plus any third-operator leaves,
   certified before the freeze (decisions.md). Maps to: **Boulder 3** (freeze re-scoped to RC).
4. **#406 consensus model-check (I1–I5).** **IN PROGRESS.** The deterministic adversarial
   property harness that HARD-GATES every graded field run; each invariant ablation-proven.
   Maps to: **Boulder 1 R1.5** (folded into the flip's model-check).
5. **Phase 5 — operational floor** (a node a person can run). **NOT STARTED (needs scoping).**
   Per-platform packaging + signed installers + operator-consented R4 self-update, plus the
   S6 scaling kills (incremental O(delta) proof maturation; reprovide dirty-tracking).
   Maps to: the operational-floor residual (Residual backlog; archived Phase 5).
6. **External red team (#183) → R1 field grade → V1.** **GATED (endgame).** Runs against
   the economy-ON config, then a green multi-region R1 grade, then V1. #183's close condition
   is MET and the issue carries the evidence; the close is the owner's call, deliberately held.
   Maps to: **Boulder 4**.

</details>

> *The retired numbered **Phases 1–6** block (Phases 1–3 COMPLETE; 4–6 the forward plan
> the Boulders replaced) and the dated **"Immediate next work (2026-08-26)"** priority
> snapshot lived here. Extracted 2026-09-01 to
> [`/archive/roadmap-history-2026-09-01.md`](archive/roadmap-history-2026-09-01.md). The
> Boulder spine above is the current packaging of the same board; still-live residuals
> they named (#559, #583, #574, #530, #586, #501/#500/#502, #506, #299) are carried in the
> Boulders and the **Residual backlog** at the end of this file.*

## Where we are now (the honest status)

- **Storage plane — sim-proven at scale, field-proven cross-network at small scale.** Cross-network publish/fetch,
  erasure-coded durability with failure-domain-aware placement + dispersion audit,
  capacity pledging/spill, mutual-TLS pinned identity, encrypted manifests +
  care-links, a quorum chain, web UI/observatory, desktop client. The silent-loss
  floors and the reprovide/config-drift gaps are fixed; scale (bit-perfect retrieval
  under churn) is proven in the deterministic in-process simulation, and
  cross-network hole-punching is proven through cone NAT in an automated Docker
  harness in CI. A warm multi-region cloud run has not yet graded a full suite
  end-to-end.
- **Trust plane — the M0 mechanism is BUILT and internally hardened.** The genuine
  composition shipped (PRs #117–#127): a verify-without-fetch proof-of-retrieval, a
  proof-of-**space-time** bond (an identity-bound sealed plot × a Wesolowski VDF,
  persisted, so N Sybils cost N real disks), standing as the time-integral of bond +
  audit, fork-choice reconciliation so partitions heal, and provable equivocation
  that slashes double-signers. The **H1–H6 systemic hardening pass is complete** —
  every standing/consensus/DHT/privacy surface has a shipped mechanism + inverted-PoC
  regression + the Invariant-A/B guardrails.
- **Durability is funded and verifiable — H7 is BUILT (#95).** The S7 spine ships: a
  per-object credit escrow, a **serve auto-skim** (popular data self-funds its repair),
  a rarest-shard bounty, and a **verified proof-of-correct-repair** — a bounty pays the
  new holder of a rebuilt shard only when correctness (Merkle recompute against the
  manifest-anchored survivors) *and* an identity-bound Shacham–Waters retrievability
  proof both verify, with an attributable false claim bond-slashed. It ships as an
  explicit **finite-but-renewable** contract with the funded horizon and **instrument
  `g`** measured, and the self-dealing red-team (garbage claim → slash, don't-store →
  deny, double-count → deny) is a permanent regression. *The plaintext-blind
  correctness commitment was found a GF(2⁸) dead end in pure Go, so M0 ships the
  Merkle-recompute floor and the bandwidth-blind upgrade is a documented fast-follow.*
- **The mission was reframed — the composition reset.** M0's Sybil corner is a
  **systemic claim held in tension**, not a Sybil-proof primitive (impossible by
  Douceur). It is stated as **C1 (no discount)** — forging a fraction *q* of standing
  costs ≈ *q*·`C_honest`, where `C_honest = disk × address-diversity × time ×
  served-demand` (non-substitutable) — **plus C2 (no quiet capture)** — the
  concentration metric keeps the minimum colluding *operator* set above *k*.
  Durability (S7) is **fused into the same budget** (one ledger). Full spec:
  [`docs/design/m0.md`](docs/design/m0.md).
- **The research commission has been answered.** The two constructions we'd routed
  to research both **exist**: center-less **proof-of-repair** (a composition of
  proven parts — unblocks durability) and a **non-globality metric** (ZK threshold
  predicate). Durability is decided **finite-but-renewable**, not "perpetual." The
  genuinely-hard residue collapsed to **one** named open problem — the shared-content
  sealing boundary (see the research frontier below). Decisions:
  [`docs/decisions.md`](docs/decisions.md).
- **The M0 build backlog is complete and externally re-attacked (2026-08-08→09).**
  All decided directions shipped: D-DEMAND P0–P3 (blind receipt + fee-burn +
  bonded-fetcher), H8 slices 1+2 (ephemeral-identity + relay-routed issuance), the
  CT-style transparency log (#180), the C2 metric wired from the committed ledger
  (#185), registry-only mode (#206), and the real-wire adversarial suite (#204). A
  second blind red-team found 2 shipped-default P0 gaps (not broken crypto); the
  certified remediation (F-1 `everMature` latch bundle, ε* discount close,
  SurvivorNakamoto, canonical signer set, refuse-to-start) is merged.
- **The consensus-correctness arc is closed as a class (2026-08-12→14, canon
  [`D-CONSENSUS`](docs/decisions.md)).** Four multi-region field runs each found an
  RC-blocking consensus bug — #357 (fork-choice oscillation), the B2 handoff
  head-count quorum, #397 (honest-proposer cross-attest), #402 (one-free-anchor
  fork). PE + research converged independently: **all four were one defect** — a
  finality quorum that did not intersect over its phase's real validator set. The
  closed invariant set is now canon
  ([`consensus-invariants.md`](docs/design/consensus-invariants.md), I1–I5), the
  deterministic **consensus model-check** (#406,
  [`consensus-model-check.md`](docs/design/consensus-model-check.md)) becomes the
  *first* consensus gate, and **every graded field run is gated on the model-check
  tier covering its regime** — a field run confirms; it never discovers an
  invariant. Fixes #357/B2/#397 are merged + field-confirmed; the certified #402
  fix (strict anchor majority `⌊A/2⌋+1`, anchor-only launch proposing) is the next
  build item.

- **The fresh-eyes audit (2026-08-19) verified the trust plane and found the economy
  dark.** Every claimed M0 mechanism verifies against code and tests (model-check
  genuine; all four adversarial wire drills pass over real TCP), and the M0 tail to
  #183 is small and enumerated. But the S7 repair economy — built and adversarially
  tested — is **default-off in every shipped node with no enable path**, `g` has never
  been measured live, bandwidth is unpriced, and the operational floor (packaging,
  O(store) cold-start, O(held) reprovide) prices out the honest operator in practice.
  Verdict: **M1 is the binding constraint.** The Boulder spine above is the response
  (D-M1-PIVOT — M1 interleaves with the M0 tail; the Boulders are the current packaging
  of that ordered board); full findings in
  [`docs/thinking/2026-08-19-fresh-eyes-audit-and-the-m1-pivot.md`](docs/thinking/2026-08-19-fresh-eyes-audit-and-the-m1-pivot.md).

## Verifying M0 is held (the Boulder 4 endgame frontier)

Two kinds of work carry the endgame: **verify** is the gate to declaring M0 *held* (not
merely built); **research frontier** is the handful of items that need a new result, not a
decision. Both feed Boulder 4 and the external red team (#183).

> *The retired **Build tracks** list (H8/H9 privacy+takedown, D-DEMAND/C2/registry status —
> a third top-level organizing scheme) was extracted 2026-09-01 to
> [`/archive/roadmap-history-2026-09-01.md`](archive/roadmap-history-2026-09-01.md). Its
> still-live items (H8 #179, H9 #180, post-launch registry #207) are carried in the
> **Residual backlog** below.*

### Verify tracks — the gate to "M0 held"

M0 is *held* only when an **external** party attacks the built composition and it
survives at declared parameters. Self-graded does not count.

**The certified sequence to the gate (D-CONSENSUS, 2026-08-14):** build the
certified #402 fix → **consensus model-check launch tier green** (#406, with the
#357/#397/#402 failing-first replays) → the P1 all-corners field run → model-check
handoff tier + the #399 WS-recovery drill → the MATURING=1 run (field-cert of the
#389 weight-quorum handoff) → model-check full budget + the red-team entry
criteria ([`release-checklist.md`](docs/release-checklist.md)) → **external red
team (#183)**.

- **Consensus model-check (#406).** The deterministic I1–I5 property harness — the
  first consensus gate, and the gate on every graded field run (tier order:
  unit → model-check → sim → netem → field).
- **Multi-machine field test (R1, #52).** Bonds, tokens, and consensus across real
  machines and real NAT — the trust plane earning the rigor the storage plane has.
  Confirms on real WAN what the model-check proved; grades liveness, which only
  the field can.
- **External red-team vs C1/C2.** A fresh, no-memory adversary attacks the
  *systemic* claim and the seven composition **seams** ([`m0.md`](docs/design/m0.md)
  §7), not isolated primitives — a primitive failing a standalone "Sybil-proof" test
  is Douceur, expected, not an M0 failure. A seam *held in tension* (bounded cost,
  documented residual) is a pass; a seam silently assumed closed is the failure mode.

### Research frontier — needs a new result, not a decision

- **⭐ The shared-content sealing boundary — the one surviving economy of scale.**
  Plain PoR over *shared* erasure-coded shards lets one physical copy answer for N
  pledges (γ→1/N); closed only by **identity-keyed PoRep sealing of arbitrary useful
  shared data**, which is not yet publicly-verifiable + timing-free +
  trusted-setup-free. **silt is not exposed today** — standing comes from a dedicated
  identity-keyed bond plot, not the shared shards — but *fusing* served content into
  standing without leaking γ→1/N is the highest-leverage open question. An
  academic-collaborator task (`m0.md` §10).
- **MSR / regenerating-code proof-of-repair.** The A1 composition is airtight for
  plain-RS reconstruction; no published construction specializes it to MSR/Clay. Off
  today's critical path (silt ships plain-RS).
- **Byzantine size-estimation under adversarial NodeID placement.** The C2 sampling
  tolerance is proven for *random* Byzantine placement; a stake-splitter chooses its
  NodeIDs, degrading it by an amount the literature does not quantify.

## What "M0 held" means

M0's Sybil corner is not a primitive to be proven Sybil-proof — that is impossible.
It is **held** when, at the network's declared parameters, no strategy earns
consensus-controlling standing for less than `q · C_honest` (**C1**), and the
concentration metric keeps the minimum colluding operator set above *k* (**C2**),
with the §7 seams either closed or *held in tension with a documented, bounded
residual*. C1 is a **theorem *under* the B5 hypotheses H1–H3** (a direct-product
bound) — *conditional*, not unconditional: the per-identity Alwen–Blocki lift is
unproven, the shared-content H3 gap (γ→1/N, #182) is open, and in shipped code only
the bond (D) axis gates standing so far (§ "where we are"). C2 is a **measurement
bounded by an impossibility result** (Kwon) — held, not closed, by design. The
verdict is rendered by the external red-team + the field test, together.

## Residual backlog (tracked here, not on the Boulder critical path)

These are the off-critical-path residuals migrated from the (retiring) GitHub issue tracker.
They do NOT gate the Boulder spine; they are the honest tail. Each carries its provenance
issue number as an anchor only. Repro recipes for the field defects (#558/#535/#530/#574/#586/#277)
live in [`docs/thinking/2026-09-01-residual-defect-repro-recipes.md`](docs/thinking/2026-09-01-residual-defect-repro-recipes.md).

**Security / data-safety residuals:**
- **Demand issuer-key proof-of-possession (DSKS) — R0.4b-PoP.** `validateIssuerKeys`
  (`core/chain/issuerkey.go`) requires a verifying ed25519 self-signature, an in-range epoch and
  a bond, but **no proof that the registrant holds the RSA private key** whose fingerprint it
  registers. Public keys are served publicly, so bonded issuer B can register issuer A's
  fingerprint for epoch E; a redeemer resolving against B then pins A's key, and a token A
  signed verifies under B's keyset — Duplicate-Signature Key Selection (Blake-Wilson & Menezes
  1999). **Latent, not live:** `handleDeliveryReceipt` resolves ONE configured issuer and ledgers
  are per-node, so the multi-issuer surface is not exercised today. The close is either a PoP in
  the registration, or the faithful RFC 9578 binding `keyFingerprint(32) ‖ epoch(8) ‖ serial` in
  `demandMsg`. Both are **validity-rule changes: research-gate + owner ratification, BEFORE the
  stamp raise.** Source: crypto-specialist advisory C-4,
  `/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R0.4b-C3-blind-RSA-epoch-binding-2026-09-03.md`.
- **FDH domain-separation-tag length prefix + 128-bit reduction slack — R0.4b-FDH.** The three
  FDH domains are not length-prefixed and `fullDomainHashD` expands to only `nLen + 8` bytes
  (64 spare bits against RFC 9380 §5.2's `k = 128`). Both are sound as built — the crypto seat
  verified the three domain constants differ at byte index 10, so no message produces a
  cross-domain collision — but sound *by accident of the constants*. **Not fixed in R0.4b C3
  because either change alters the FDH output, and the publish and credit domains are BYTE-FROZEN
  against chain replay:** committed publish tokens re-verify on every replay, so changing their
  signed bytes invalidates history. Fixing only the demand domain would leave two conventions in
  one file. Do it as one versioned change, with a chain-era gate. Source: advisory C-8.
- **Blinding-factor sampling: mod-reduction, not rejection sampling — R0.4b-BLIND-SAMPLING.**
  `blindtoken.randInt` draws the blinding factor `r` (and the issuer-side blind `u`) by reducing
  `(bitlen(N) + 64)` random bits mod `N`. RFC 9474 §4.2 states a **MUST**: *"The blinding factor r
  MUST be randomly chosen from a uniform distribution. This is typically done via rejection
  sampling."* silt meets it only **statistically** — the distribution is within `2^-64` of uniform.
  **Declared, not fixed, in R0.4b C3:** it is a conformance gap rather than a break (no use of a
  `2^-64` bias is known), and it changes no committed byte, so it neither blocks the close nor
  should slip in unannounced. Work: rejection-sample into `[1, N)` and **bound the retry** — the
  two loops calling `randInt` (`blindD`, `SignBlinded`) currently spin forever on a reader that
  yields zeros. Declared at `core/blindtoken/blindtoken.go` (`randInt`) and in
  `docs/thinking/2026-09-02-r0.4b-c3-close-design.md` §12. Source: crypto advisory R4,
  `/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R0.4b-C3-crypto-items-as-built-01bf8e9-2026-09-03.md`.
- **Transport authentication (TLS or Noise) — #437.** silt's wire is unauthenticated CBOR
  (`adapters/tcpnet/wire.go`), so an on-path MITM can strip certificate signatures in transit.
  Certified NOT a safety break and NOT a wedge (stripped blocks fail `ValidateCommit`, are inert,
  never drive round-advance; only verifying sigs count; ≥⌊A/2⌋+1 full-certificate holders re-serve
  over any honest path; residual = censorship over a controlled link, already tolerated by the
  liveness model). Work: wire authentication/integrity at the transport layer — a pre-existing
  transport property, orthogonal to consensus. Consult `docs/network-durability.md` before design
  (build-immutable #5); research-gate the crypto choice. Cert:
  `432-proposer-prepare-required-RESEARCH-CERTIFICATION-2026-08-16.md` §5.2.
- **Crash-safety: torn `chain.cbor` → silent genesis fallback — #558.** A SIGKILL mid-persist
  can tear the `chain.cbor` write; replay hits the damaged region and the daemon silently discards
  finalized history back to genesis (the markstore is atomic; the chain store is not). Silent loss
  on a common failure (OOM/power). Fix direction + RED home in the repro doc. **Real silent-loss
  residual** (build-immutable: no-silent-loss floor).
- **On-disk format migration policy — #237.** Upgrading a daemon onto a pre-format store is
  silently destructive: the chain is rejected (`unsupported block version`) and silently reseeds
  with only a one-line stderr notice; content is stranded. **Needs a POLICY call before any release
  that crosses a format boundary:** (a) a migration path (read old store, upgrade proofs/chain), or
  (b) refuse to start with a clear message (safe default). Relates to #70 (proof migration) / #98
  (chain format).
- **Silent-behavior observability — #235.** Three silent behaviors reduce field assertability:
  (1) a healthy repair sweep emits no log line at any level ("is the caretaker working?" is
  unanswerable until something breaks) — add `repair: sweep complete, all stripes healthy` and/or
  per-repair counts; (2) `-revoke <bogus-root>` leaves the daemon silently inert — add `revoke:
  root not yet committed, waiting` or an explicit refusal; (3) rolling upgrade across a format
  boundary degrades with only a one-line stderr notice (ties to #237). Partly overlaps #237.

**Test / harness debt:**
- **Cited-test lint + the OWED ledger it opened — PR #708 (lint) and PR #707 (first two payments).**
  `scripts/check_cited_tests.py` fails the build when a `TestXxx` cited in a Go comment, `CHANGELOG.md`,
  `ROADMAP.md` or `docs/**` resolves to no `func TestX(` in the repo — the
  `scar:cited-test-does-not-exist-2026-09-02` class, where a phantom laundered from a production
  comment into a research certification. In-repo citations are STRICT; external review trees are
  ADVISORY (they may legitimately cite a test on an unmerged branch), and `.claude/` is excluded —
  load-bearing, because those worktrees are copies of OTHER branches and scanning them would let a
  phantom resolve. PR #707 wrote the first two cited-but-missing consensus guards (the v5 root
  coverage guard and the era-4 write-path guard) and closed the same worktree-walk unsoundness in
  `scripts/check_claims.py` (measured: 121 test names resolvable only in `.claude/worktrees/`).
  **Open:** the rest of the OWED ledger.
- **e2e paid-delivery-lane fixture is `-objective=false` — R-E2E-ERA4-FIXTURE.** The e2e
  delivery-receipt daemon runs `-objective=false`, so `chain.objective()` is false, so
  `epochsEnabled()` is false, so `apply()` never calls `rotateEpoch` — and the era-4 readiness
  tally lives inside it. **The tally can therefore never latch on that fixture, at any readiness
  stamp, on any binary**, so the paid lane's POSITIVE arm has no e2e coverage:
  `TestPaidDeliveryLaneRefusesWithoutACommittedKeyBinding` asserts the certified refusal
  instead, and `sim TestPaidDeliveryLaneThreeCallComposition` + `core/node
  TestRTC3_RestartDoesNotRePayTheSameWireReceipt` carry the positive arm below e2e. **This is a
  PREREQUISITE of the stamp-raising release, not of the R0.4b merge:** restore the e2e positive
  arm by UPGRADING THE FIXTURE to objective + bonded + epoch-enabled (a topology that reaches
  the `everMature` latch), **never** by exposing `Config.Era4ActivationHeight` to a harness in
  any form — that is the one branch that skips every readiness predicate the tally embodies.
  The trace is pinned by `core/chain TestGateF_NonObjectiveTopologyCanNeverLatchEra4`. Source:
  G-8 convergence,
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md`.
- **LANE-OFF rotation disarm has no runtime observer — R-LANEOFF-ROTATION-RUNTIME.** That no
  demand-key rotation goroutine RUNS after a failed boot install is pinned only by a source-ORDER
  gate (`cmd/silt TestDaemonArmsTheRotatorOnlyAfterABootInstall`). Reaching the branch in a real
  daemon needs an unwritable issuer directory whose publish-token key already exists; OBSERVING
  the difference additionally needs the chain to cross an epoch boundary, which on a lone
  validator needs a driven publish. Low: the fix is structural (the single assignment sits below
  the single failure exit), so there is no branch left to regress into — but the property is
  UNGATED at runtime and says so in the gate's own failure text. Source: PE ruling H-2,
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R0.4b-C3-close-271ab81-final-2026-09-03.md`.
- **A bare `go test ./...` has no margin against the default package timeout.**
  `core/chain TestMeasureRecomputeMatureNowStreamingWin` is a long MEASUREMENT test, and it
  puts the whole `core/chain` package close to Go's 10-minute default. Measured 2026-09-03 on
  this branch's tree: the single test **309 s**, the package **530 s** with an explicit
  `-timeout 40m`; a bare `go test ./...` on `origin/main` `2247235` was reported timing out in
  `core/chain` (~7.5 min for the same test on that run). Nothing is broken — the test has an
  always-on fast structural twin (`..._Structural`) and is skipped under `-short`, which is what
  CI runs — but the documented full-suite command has no margin, and load decides whether it
  passes. **Work:** document `go test -timeout 40m ./...` as the full-suite command, or move the
  measurement behind an opt-in build tag. Do NOT simply shorten the measurement — the number is
  the artifact. Untouched here: it is not this branch's test, and editing another branch's
  measurement inside a receipt-expiry commit is how a regression hides.
- **Test-honesty audit — #303.** 27 adversarially-verified test-honesty issues + 9 product
  findings from a per-harness audit (65 agents) of all 18 integration harnesses against the 5
  field-test immutables — each a way a harness could go GREEN on a broken product (e.g. the
  redteam SCENARIO 2/3 assertions key off a non-specific `!resp.OK`, so a dead/blanket-rejecting
  H3 goes green; fix: add an H3 positive control). Open QA debt; overlaps but is not fully covered
  by the Boulder-1 decoration-oracle gates.
- **Harness reachability + flow-overlap (#574), Docker pre-genesis stall (#530), skim-observer
  arming (#586).** Standing-tail harness defects; repro recipes + fix directions in the repro doc.
  #530 first step is instrumentation (`-log debug`, capture one full client transcript), not a fix
  (build-immutable #7).
- **CPU-time O(depth) regression CI gate — #616.** The shipped O(depth) gate (#613) measures
  baseline-subtracted `runtime.MemStats.HeapObjects`, so it catches allocation-shaped depth
  blow-ups (the #555 `AllEntries` shape) but NOT CPU-time-shaped ones (the #528 per-height CPU
  burn allocates little, so it slips the memory gate). Needed: a companion wall-time/CPU-time
  slope-vs-depth gate analogous to the memory gate's two-stage doubling test. The blocker is
  noise — wall-time is far noisier than a seeded byte-deterministic `HeapObjects` count, so this
  needs its own **noise study first**: measure run-to-run variance of the per-height time cost,
  derive a bound from that measurement (not a guessed constant), and prove it goes RED on an
  injected CPU-time O(depth) defect before it can assert. Companions the Boulder-1/Boulder-3 test
  gates. PE ruling: `RULING-613-odepth-ci-gate-2026-08-28.md` §B; deliberation
  `docs/thinking/2026-08-27-o-depth-ci-gate.md` (lines 183–188).
- **cloudtest infra-liveness-FAIL journal-capture gap — #504.** A distinct evidence-capture
  defect. The flow-level capture-on-fail exists (`flow-evidence-*.log`), but the `infra-node-liveness`
  row's FAIL path has NO capture step: on run fa501cc-56689 it named `island-c×3` crashes then let
  the EXIT-trap teardown destroy them, losing crash-type attribution (OOM vs Go fatal), crash times,
  and the pre-crash log tail — the exact build-immutable #7 canonical loss that ratified
  capture-the-evidence-first (PR #394). Fix (third-time rule — a gate, not prose): when
  `infra-node-liveness` records a FAIL, pull each named node's `journalctl -u silt` (all boots) +
  `dmesg | tail` into `failed-nodes-<run>.log` BEFORE returning, the same capture path the flow
  failures use. Fires only on the FAIL branch (cheap).

**Field-test harness residuals (folded from the retired `integration/FIELD-TEST-ROADMAP.md`, 2026-09-01):**
The RC field-test gate is MET (RC run `585c82a-58990` graded 28 pass / 0 gap / 0 fail /
2 skip-by-design, #532 `eb57d50`; deep lineage `fe2376a`-deep 30P/1G/0F). What remains is
harness truthfulness/coverage/parity hardening — none gates the Boulder spine. The full
list and per-item fix directions live in
[`archive/FIELD-TEST-ROADMAP-2026-09-01.md`](archive/FIELD-TEST-ROADMAP-2026-09-01.md);
the load-bearing still-live items:
- **Harness truthfulness hardening — tracked under #303** (the test-honesty audit). Each
  is a way a green harness could hide a broken property: the consensus P0 negative control
  needs a real quorum + a positive control; refusal reasons should be read from the daemon
  log, not client stdout; `soak` memory-growth and `churn` seeded-placement gates need
  falsifiable oracles; `bond` C1 must assert reputation ∝ bond (two bond sizes, ratio
  roughly linear); `nat` hole-punch should assert the direct path bypassed the relay;
  `redteam` should cross-check the honest target's head height is unchanged. `upgrade`
  chain-reload (CHAIN_OK positive height) is DONE.
- **Demand field test (#264, above).** `integration/demand` becomes real only once the
  demand P2/P3 seam is wired into the daemon fetch path — the same #264 residual listed
  under Durability/repair/demand below.
- **`chaos` WAVE-2 redundant-bootstrap survival — root-cause open.** Does a redundant
  (≥2 seed) bootstrap survive one crashing? Pin it, then fix + assert or document the
  single-bootstrap topology limit.
- **#281 empty-routing-table self-heal wire-certification.** Fixed in-product
  (`Node.StartBootstrapRetry`) but no cloud flow disables the startup TCP-wait to exercise
  the real `re-bootstrapped: recovered from an empty routing table` path over the wire.
- **GCP substrate operability — RC gate MET; quota/preflight hardening still worthwhile.**
  The full 13-node run is completed and graded (the RC sheet above). Separately: a
  full-topology run is still blocked by two ENVIRONMENTAL constraints (not product bugs) —
  a `us-central1-a` E2 capacity shortage and the default `IN_USE_ADDRESSES` = 8/region
  quota (~11 external IPs needed). Worth doing: a pre-flight that checks IP headroom + zone
  capacity before `apply`; shrink the public-IP footprint (IAP-only/bastion) so a
  single-zone full run fits the default quota; make `nuke` sweep the leaked
  VPC/subnets/firewall/routes by label and stop swallowing `terraform destroy` stderr.
- **Per-substrate parity + GCP-only scenarios (parity).** Factor the shared
  `exec-on-node`/`assert-on-log` node abstraction so one scenario targets either substrate,
  then add scale-out churn (50+ nodes), a real firewall partition, `tc` link-shaping, and
  long-haul soak. An **AWS variant + two-cloud** field test is the far end (a fallback
  substrate when GCP capacity/quota blocks, then a GCP+AWS split for real inter-provider WAN).

**Durability / repair / demand residuals:**
- **Repair dial-storm to dead holders — #277.** The DHT walk re-dials dead holders every sweep
  (the `deadUntil` negative cache is consulted on the fetch/repair decision path but not on the
  walk's dials), so under heavy permanent loss a sweep can't finish though ≥k shards survive.
  Scale-independent behavior; part of the pre-gate repair-sweep family (#501 unbounded sweep / #500
  fetch-retained copies never announce / #502 restart orphans the working set). Repro in the doc.
- **Wire demand P2/P3 into the daemon fetch path — #264.** `core/demand` P2 fair-exchange floor
  and P3 cost-to-wash (fee-burn + bonded-fetcher credential) are real and unit-tested but have no
  live daemon-wire seam (no `silt sim run demand` scenario, no CLI flag, no serve/fetch-path
  enforcement), so a real field test can't cynically exercise them. Wire one/both (a sim scenario
  and/or daemon-level demand enforcement), then add `integration/demand` asserting the outcome:
  a freeloader can't fetch without paying; faking demand costs one bonded identity per unit + a real
  fee. Phase-4 (PoD) detail; the firewall (delivery credits never fund standing, γ→1/N #182) is
  immutable.
- **Bond-proof reply size / N² cost — #299.** The encoded bond-challenge answer is ~1.5 MB,
  near-flat in bond size (dominated by 64 label opens), so proofs are loss-sensitive over lossy TCP
  and cost N²×1.5 MB per audit interval as the validator set grows. The sound fixes are structural
  (SNARK-wrapped succinct proof → H-track; fewer samples = a soundness tradeoff, Evolving-tier,
  research-gated; FEC needs QUIC first). An **owned, named residual**, not a stopgap. ROADMAP R3.3
  keys the PayWord/RegCap re-derivation to "only if #299 moves" — this is where #299's measured
  numbers live. Possible cheap interim (unverified): de-duplicate shared DRSample parent blocks in
  the encoding (no soundness change) — measure before any k-reduction.

**Polish & latent wins (low-priority — folded from the retired `BACKLOG.md`, 2026-09-01):**
These are small, still-open captured ideas — polish and opportunistic wins, not RC-critical.
They carry no provenance issue (they never merited one); they gate nothing on the Boulder
lattice. When one matures, promote it to the section it belongs in.

- **Demand-responsive dispersion — pull half (storage placement).** The push half ships (a hot
  holder leases cache copies away from its own failure domain — the dispersion re-spread in
  `core/node/repair.go`). Still open: let a node that had to *fetch* a chunk under load
  opportunistically cache and announce it, decaying when unused — so hot copies also gravitate
  *toward* readers, not just away from hot holders. Kin to the repair-sweep residual #500
  (fetch-retained copies never announce) above; a shared fix could subsume both.
- **Domain-aware placement gaps (storage placement).** Query a candidate's failure domain when
  gossip hasn't reached it yet (today placement spreads only across *learned* peer domains);
  domain-aware capacity spill. Column placement will subsume the per-stripe anti-affinity repair
  path later.
- **Direct IPv6 dial before assuming a relay (networking latent win).** Try a direct IPv6 dial
  before falling back to relayed transport — a cheap latent win before the relay fallback.
- **Relay selection + failover (networking latent win).** A NATed node that discovers relays by
  gossip currently adopts the lowest-ID one and commits to it; if the chosen relay won't
  register, it retries that one forever instead of failing over. Fine while a swarm has one dev
  relay; wants selection + failover once community relays are plural.
- **`docs/` staleness enforcement (observability polish).** The `Docs ship with code` CI job
  (`.github/workflows/ci.yml`) already fails a PR that touches `cmd/`/`core/`/`adapters/`
  without a `CHANGELOG.md` update. Extending the same staleness enforcement to `docs/` is a
  possible later tightening.
- **e2e relay-in-the-middle variant (test/harness polish).** A relay-in-the-middle variant of
  the multi-process e2e suite. (The kill-a-node erasure-resilience variant it was captured
  alongside has since shipped as `e2e/economy_repair_test.go`.)
- **Capacity/scaling shape test — deferred (test/harness polish).** The 3 GB shape test —
  30×100 MB vs 300×10 MB — to characterize manifest/DHT overhead vs chunk-count. Deferred while
  the dev box is RAM-bound.

**Consensus-touching residuals (tracked, NOT resolved — need the research gate when worked):**
- **Objective-mode `Config.Quorum` floor divergence — #380 · tracked, NOT resolved —
  consensus-touching (I1).** In objective mode the effective `ValidateCommit` requirement is
  `max(Quorum, bftThreshold(validatorSetSize))` (`core/chain/chain.go` `RequiredQuorum`). `bftThreshold`
  is a pure function of committed chain state (replica-identical), but the `Quorum` floor is LOCAL
  config, so two honest replicas with different `-quorum` compute different validity for the SAME
  committed block; a node whose floor exceeds a committed block's attestation count rejects it in
  `Reconcile` and is stranded at genesis. Confirmed root cause of the SYBILS=8 field GAP (#338).
  **Workaround shipped:** a uniform `-quorum` across the objective swarm (documented behavior,
  `TestDivergentQuorumFloorStrandsSyncingNode338` passes today). **The product question is a
  consensus-rule change (I1) and needs the research/owner gate before any build** (build-immutable
  #6): should objective mode ignore the local `Quorum` floor in `ValidateCommit` and defer to
  `bftThreshold`, keeping `Quorum` as a proposer-side gather target only? Options: (1) ignore the
  floor in `ValidateCommit`; (2) a cheap non-gated startup warning when `-quorum` is set on an
  objective+byzantine node; (3) leave as-is, documented. Do NOT assert resolved.
- **Mature-regime PUBLISH starvation — #441 · tracked, NOT resolved — consensus-touching
  (I4 liveness).** Field-observed (run a56ac10-42834): entry-carrying blocks committed only
  pre-latch (h4–h23, latch at h26); NONE of the 52 post-latch blocks carries a publish entry,
  though the chain stays live via renewal drains. Consensus SAFETY unaffected (S1/S2/I1–I5 held);
  this is I4's *liveness* face for the ENTRY path. Leading (code-cited, NOT proven) mechanism: the
  #432 view-change machinery is drain-only — `recordRoundChange` (`core/node/rounds.go`) fires the
  drain proposal, the publish path (`httpregistry publish` → `proposeBlock`) has no round-aware
  retry or new-view seat, so at mature steady state pending renewals own every height's rounds and
  the publish loses every round. **A fix touches the certified #432 round machinery, so it needs a
  research/PE consult BEFORE any build** (no unilateral change). Deterministic home: a repro
  schedule in `matureWorld` (PR #440). Attribution: `docs/thinking/2026-08-16-run3-mature-handoff-passed-publish-starvation-found.md`.
  Do NOT assert resolved.

**Operational floor (RC-path, needs scoping — the archived Phase 5):**
- **A node a person can run.** Per-platform service packaging (launchd / systemd / Windows
  service) + signed installers, and operator-consented self-update per R4 (signed manifests,
  never silent). Plus the S6 scaling kills: incremental O(delta) proof maturation (kill the
  O(store) restart scan) and reprovide dirty-tracking (kill the O(held) per-interval re-sign).
  Exit gate: a non-developer installs a node that survives reboot and returns to serving in
  seconds, and steady-state cost no longer scales with the whole held set. NOT STARTED.

**Post-launch (explicitly not V1):**
- **Registry liveness-pruning + federation — #207.** The read-cost-bounding lever of registry
  economics shipped for M0 (per-IP rate limit + timeouts, #206). The remaining cheapness levers —
  liveness-pruning of dead entries (needs a provider-liveness probe; inapplicable to the chain-backed
  append-only registry) and federation/sharding of a large public registry — are post-launch scaling.
  These do NOT gate M0.

**Umbrella/frontier issues kept as evidence anchors (not a second task list):** #183 (external
red team → **Boulder 4 R4.4, the M0 close gate**; close condition MET, owner deliberately holds), #182 (shared-content
sealing frontier → R4.1 / research frontier above), #179 (H8 metadata privacy), #180 (H9 pluralistic
takedown), #94 / #52 (the forward-tracks / R1 verify epics — now the Boulder structure itself),
#406 (consensus model-check = Boulder 1 R1.5).

## The resolver layer ("Aslan" — separate product)

Meaning lives above the infrastructure, in a separate codebase: name/description/
tags → (root, manifest key). Silt ships zero Aslan code, ever. See
`docs/aslan-boundary.md`.

## Release engineering — the march to V1

The `0.1.x` / `0.2.x` tags were **experimental / learning releases**, not steps
toward V1 — treat them as archaeology. The real cadence has three stages:

- **Learning phase (past).** Everything through the experimental 0.x tags:
  proving the architecture. Detail lives in `docs/buildlog/`.
- **Feature-complete → `0.9.0`.** When every V1 build track's *mechanism* is built
  (the floors plus the M0 composition, durability, privacy, takedown) we cut
  `0.9.0` as the release-candidate line and harden it in the field.
- **`1.0.0` = V1.** Cut only once the tenets are field-proven multi-machine (R1)
  **and** the external red-team verdict holds — a *true* release candidate,
  signed/notarized/checksummed. This is the first release we stand behind publicly.

Mechanics when ready: move CHANGELOG "Unreleased" into the version, tag, and the
release workflow builds + publishes binaries; add code-signing/notarization
(macOS) + a checksums file first. See `docs/release-checklist.md`; website/DNS in
`DEPLOYMENT.md`.
