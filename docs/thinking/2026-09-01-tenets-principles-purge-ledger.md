# TENETS.md principles-purge — relocation ledger

**Date:** 2026-09-01
**Author seat:** Builder
**Status:** OWNER-RATIFIED 2026-09-01 (edits the supreme/immutable doc — PR to main, owner merges).

## Purpose

`TENETS.md` was restructured into a document of **pure guiding principles**
(owner-ratified governing rule, 2026-09-01): the tenets state what "good" looks
like as an *outcome*, abstracted to what each principle *represents*, and carry
no mechanism detail, ship-status, proof machinery, format specifics, or
issue/PR war-stories. Build details look *up* to the tenets; the tenets never
look *down* at the build.

This ledger is the audit trail for the **hard guardrail: relocate, never lose.**
Every block of product/mechanism/history detail removed from `TENETS.md` is
listed below with (a) what it was, (b) where its content now lives, and (c) the
abstracted principle that replaced it. Every "lives now in" target was confirmed
present by grep before the source block was cut; where it was not already
present, it was moved into a companion (noted as **MOVED**).

Companions referenced: `docs/design/m0.md`, `docs/decisions.md`,
`docs/network-durability.md`, `docs/build-process.md`, `docs/network-protection.md`,
`docs/design/owned-residuals.md`, `ROADMAP.md`, `VISION.md`, and the new
`docs/tenets-history.md` (the extracted amendment log + war-stories).

---

## Relocation table

| # | Removed block (what it was) | Content now lives at (file:line) | Abstracted principle that replaced it in TENETS |
|---|---|---|---|
| 1 | **M0 proof machinery** — the C1 "no discount" inequality `(1−ε*)·q·C_honest`, C2 "no quiet capture," the `C_honest ∝ D×A×T×B` product, q/k, the systemic composition math. | `docs/design/m0.md:56–120` (the systemic claim, C1/C2, the product) — canonical M0 spec, already the source of truth. | Part 0 keeps M0 as the *mission* and states the falsifiable yes/no bar in prose; points to `design/m0.md` for "the precise composition claim, its inequalities, and the suite." |
| 2 | **M0 "shipped subset vs. target" ship-status** — `C_honest ≈ D` today; B (served demand) unbuilt (#181); A at DHT layer; T retention-only; C1 *conditional*. | `docs/design/m0.md:108–120` (⚠️ Target vs. shipped subset callout); issue trail in `docs/tenets-history.md` (war-stories §, #181/#182). | Removed entirely from the tenet — ship-status is build state. Part 0 says "which axes are wired at any moment … live in `design/m0.md`; those are build state, and build state does not belong in a tenet." |
| 3 | **The external "V3 red-team suite" naming** as the M0 verdict machinery. | `docs/design/m0.md` (the red-team target); V3 tenet in TENETS Part IV retains the *discipline* (external adversary), not the suite-as-machinery framing. | Part 0 / Part IX say M0 is held iff "an external red-team suite denies all three failure modes" — the principle (external, falsifiable), not the named V3 artifact. |
| 4 | **S7 shipped-vs-target fusion caveat** — "⚠️ That fusion is NOT shipped, and deliberately so"; dedicated identity-keyed bond plot; γ→1/N shortcut; "#182"; "gated on this separation holding." | `docs/decisions.md:49–120` (D-S7 economy) + `docs/design/m0.md` §10 (γ→1/N sealing boundary, the core open problem). | S7 keeps the *principle* ("durability must pay for itself"; the one-ledger design goal) and states the fusion "turns on a real sealing problem, owned in `design/m0.md`" — no ship-status, no issue number. |
| 5 | **S7 funding-model mechanism detail** — "per-object durability escrow," "auto-skims a fixed fraction," "Shacham–Waters," "care-link quorum," "H7 / issue #95," "the plaintext-blind PCS is a GF(2⁸) dead end in pure Go," the recompute-floor fast-follow. | `docs/decisions.md:49–120,189` (D-S7: per-object escrow, serve auto-skim, rarest-shard bounty, Shacham–Waters PoR); `docs/design/h7-proof-of-repair.md` (H7 construction). | S7 keeps the funding *stance* in prose (no speculative token; internal escrow; auto-skim; correctness+retrievability verified by a coordinator-less quorum; slashing) and points to `decisions.md` (D-S7). Drops #95, GF(2⁸), the fast-follow. |
| 6 | **S7 "perpetual" mechanism** — the Arweave endowment identity, `g > 0`, "credit-denominated cost-of-storage decline," "publishes the funded horizon per object." | `docs/decisions.md:119` (D-S7 finite-but-renewable horizon contract); `docs/network-durability.md` + `docs/risk-register.md` (`g≤0` risk). | **Lane-5 prior-art precision applied:** S7 now frames it as "an endowment identity **priced in the network's own credit unit, not in fiat**" and states the **adopt-the-schema / reject-the-perpetual-promise** stance. The numéraire distinction is the principle; `g` becomes "the measured credit-denominated decline" without the symbol. |
| 7 | **Frozen-format specifics** — the era-3 SMT spec, `BlockVersion = 4`, `H_era3`, `regVersion >= 4`, the 18 `committedSet` fields, RFC-6962 MTH, `3af40bc`, `ports.NodeStore` / #600 follow-on, the re-certification filename. | `docs/decisions.md:454–628` (D-TIERING + FROZEN 2026-08-29 freeze entry — full spec); cert at `silt-reviews/research/research-outcome/era3-committed-state-root-format-BUILT-RECERTIFICATION-2026-08-29.md`. | Part IX keeps the **frozen-format principle**: "a shipped, ratified consensus format is immutable — amend only via a NEW ERA, never an edit," + the safety-first stall behavior. States the specific formats "live in the `decisions.md` freeze entries, not here, because the *principle* is the immutable, and the individual formats accrete under it." |
| 8 | **R4 update-mechanism specifics** — criticality tiers (Low/Medium/High/Critical = 24–48h), "threshold-signed (m-of-n)," "recallable (monotonic sequence)," "observation-clocked," "fail open," `{advisory-sequence, min-version, …}`. | `docs/network-protection.md:75–119` (Criticality tiers table, threshold-signed, version-floor advisory record). | R4 reduced to its **principle**: operator-autonomous; never silent auto-update; only security is graduated; the last-resort gate can halt the network so **no single key may declare it — the signing threshold is the safety property**; recallable + observation-clocked. Points to `network-protection.md` for the mechanism. |
| 9 | **Immutable #3 mechanism specifics** — the latch name `everMature`, `F-1`, "de-maturation liveness is a real-bond ≥⅔ super-quorum," "weak-subjectivity checkpoint," `design/m0.md §10`. | `docs/design/m0.md` §10 (shed metric, the honest-whale residual, latch); `docs/design/consensus-invariants.md` (quorum invariants). | Part 0 + Part IX #3 keep the **principle**: bootstrap scaffolding is explicit/time-boxed and sheds via a one-way latch that never re-arms; the residual is bounded and owned in `design/m0.md`. Drops the identifier `everMature`, `F-1`, the ≥⅔ figure (a parameter). |
| 10 | **R4/immutable-#5 present-tense criticality tiers** duplicated between Part V (R4) and the transparency-log detail in immutable #5. | R4 mechanism → `docs/network-protection.md`; transparency-log construction (survivor Nakamoto + ZK threshold) → `docs/decisions.md` (D-TAKEDOWN) + `docs/threat-catalog.md`. | Immutable #5 keeps "every honored revocation is committed to an append-only transparency log … toward a formal non-globality guarantee" — the principle, not the CT-style / inclusion-proof mechanism words. |
| 11 | **Issue/PR war-story citations throughout** — #43 (dead-ephemeral network-size), #46 (stranded publish), #47/#48 (registry-as-cache), #44 (caretaker gap), #60 (optimistic ack), #27 (hole-punch), #286/#288/#289 (WAN/flakynet), #299 (Filecoin-PoRep prover), #341, #388–#406, #181/#182. | War-stories collected in `docs/tenets-history.md` (§"build-immutable scars" + §"other product war-stories"); build lessons in `docs/build-process.md:52,148,158,161` (#286, journal capture) and `docs/network-durability.md` (#286/#288); #299/OOM in `docs/design/owned-residuals.md` + `docs/build-process.md`. **MOVED**: the consolidated scar list is newly written into `tenets-history.md`. | Tenets keep the *lesson* in principle form (e.g. B7 "an optimistic ack is a defect," S5 "no dashboards that flatter") with no issue number. |
| 12 | **The ~190-line Amendment log (Part "Amendment log")** — 2026-08-01 … 2026-08-29, every dated amendment with its reviewer/issue provenance. | `docs/tenets-history.md` (§"Amendment log") — **MOVED verbatim**, plus a new 2026-09-01 entry recording this restructure. | TENETS.md leaves a one-line dated pointer at the foot: "The dated amendment log … live in `tenets-history.md`." |
| 13 | **B8 example product names / self-grading detail** — the "single-author crypto graded by its own author," Douceur-by-name framing. | `docs/design/m0.md:18–53` (the primitive-hunt reset, Douceur). | **Lane-5 applied:** B8 keeps the external-adversary principle AND adds the **proven-UNADOPTABLE discipline** ("we also reject a primitive that fails our bar — unaudited, archived, or unproven … even when convenient"), stated as a principle with no product names. |
| 14 | **T0 "BitTorrent + BitCoin" tagline** stated as bare lineage. | — (no relocation needed; a clarification, not a removal). | **Lane-5 applied:** T0 now carries a one-clause **fidelity note** — storage plane behaves like content-addressed swarms; trust plane is BFT / proof-of-stake-class + a transparency log, **not Nakamoto proof-of-work**. |
| 15 | **Part 0 weak-subjectivity framing** — "silt is weakly subjective, like every proof-of-stake-class system." | `docs/design/m0.md` + `docs/decisions.md` (checkpoint / cold-sync). | **Lane-5 applied:** Part 0 now names the **attack class** — "exposed to the **costless-simulation / long-range** attack class that a checkpoint pins against" — instead of "every PoS system." |

---

## Additions (not relocations)

| Item | What was added | Where |
|---|---|---|
| A1 | **Immutable Register** — a top-of-file index table: mission + six corners + frozen-format principle + build-immutables, one line each + a pointer to elaboration (Lane 1b). | `docs/TENETS.md` "The Immutable Register" section. |
| A2 | **T-AR — Anti-recentralization** (Lane 6), NEW **tenet-tier** principle (NOT immutable): "No permanent center of *reward*: the edge tier that does the majority of the work must remain a net-positive place to do it" — the economic dual of immutable #3. **No ratio/parameter baked in** (those stay Evolving). | `docs/TENETS.md` Part IX (Tenet tier) + a Part VIII value-loop tension row. |
| A3 | **Build/test discipline named at principle level** — "unit → consensus model-check → integration → e2e" pipeline, and "a field run confirms a fix; it never discovers a consensus invariant." No test files, no issue numbers. | `docs/TENETS.md` Part IV (V1). |
| A4 | **"A release candidate has these attributes"** — the abstract attribute list a shippable silt must meet (allowed by the governing rule: abstract outcome attributes, not the proof suite). | `docs/TENETS.md` Part 0. |

---

## Dedup (Lane 1c) — principles restated 3+ times collapsed to one home + pointers

Each carries the **union** of its caveats (no hedge dropped to shorten):

- **Refuse-to-surveil / access-unobservability-held-in-tension** appeared in Part 0,
  Don't #3, persona #1, the value-loop table, and immutable #4. Canonical home:
  **immutable #4** (fullest form: refusal absolute + metadata-layer unobservability
  bounded by the anonymity trilemma and anonymity-set size, not a blob-layer
  absolute). Others state it briefly and defer; all caveats preserved in #4.
- **No *permanent* center + shedding scaffolding** appeared in Part 0, T1, and
  immutable #3. Canonical home: **immutable #3** (one-way latch, shed metric,
  bounded honest-whale residual). T1/Part 0 state the principle and defer the
  mechanism to `design/m0.md`.
- **The principle-not-mechanism rule + the M0 exception** appeared in Part 0 and the
  Evolving tier. Canonical home: the **callout box** at the end of Part IX; Part 0
  states the exception once ("the one mechanism deliberately pulled into
  first-release scope").
- **External adversary certifies M0** appeared in Part 0, B8, and V3. Canonical
  home: **B8** (the discipline); Part 0 and V3 assert and defer.

---

## Guardrail verification (grep-confirmed before cutting)

- C1/C2/composition math → present `docs/design/m0.md:56–120`. ✓
- R4 mechanism → present `docs/network-protection.md:75–119`. ✓
- era-3 frozen spec → present `docs/decisions.md:454–628`. ✓
- D-S7 economy (escrow/skim/bounty/Shacham–Waters/horizon) → present `docs/decisions.md:49–189`. ✓
- #286 + journal-capture lesson → present `docs/build-process.md:52,158,161`. ✓
- Amendment log → **MOVED verbatim** to `docs/tenets-history.md`. ✓
- Build-immutable scars (#288/#299/#46/#60/#43/#47/#48/#44/#182/#181) → **MOVED**
  (consolidated) to `docs/tenets-history.md`; disciplines also in
  `build-process.md` / `network-durability.md` / `owned-residuals.md`. ✓

**Correction (fix-cycle, 2026-09-01).** The prior blanket claim here — "nothing
removed from TENETS.md was lost" — was **FALSE**. A blind review found four
principles that were **dropped, not relocated**: the maturation bet (the
young-vs-mature regime split + the named race), and two permanent Douceur
residuals (the pre-farmable bare-age limit and "wash is re-priced, not proven
away"). These are *principles*, not mechanism or ship-status, so their home is
`TENETS.md` itself — a companion pointer does not discharge them. They have been
**RESTORED to TENETS.md** in this fix-cycle (see the fix-cycle table below), not
relocated out. The corrected claim: **every block of mechanism / ship-status /
history was relocated to a companion (confirmed by grep, cited above) or moved to
`tenets-history.md`; every abstracted *principle* was kept in — and the four that
were wrongly dropped are now restored.**

---

## Fix-cycle restorations (2026-09-01) — RESTORED to TENETS.md, not relocated out

| # | Restored principle | Where in TENETS | Why it belongs in the tenet (not a companion) |
|---|---|---|---|
| R1 | **The maturation bet** — soundness holds in the **mature** regime, the anchors scaffold the **young** one, and the bet is that *maturity is reached before the scaffolding can be captured* (a permanent structural race, true for every launch). | Immutable #3 (Part IX). | It is the *conditional* under which M0's Sybil corner is sound — a principle the reader must see at the tenet, not a mechanism. Mechanism (latch name, super-quorum, metric numbers) stays in `design/m0.md`. |
| R2 | **Sybil over-claim → design-target framing** — "the composition is *designed so that* … forging standing *would become* indistinguishable from honest provision," not an achieved Sybil-proof property. | Part 0 ("ruin comes from composition"). | A verb-class fix on a published-claim principle; reading it as achieved would over-state M0. The `≈ D today` ship-status stays in `design/m0.md`. |
| R3 | **Pre-farmable bare-age residual** — standing cannot accrue from age alone; the only sound time axis is continuous re-proof. | Part 0 (the permanent-Douceur-limits list). | A permanent limit on what the composition can promise — a principle, stated with no issue number or mechanism. |
| R4 | **Wash is re-priced, not proven away** — a demand receipt can be unforgeable + fetcher-unlinkable, but *unlinkable ≠ authenticated*; self-dealt demand is a Douceur limit, priced by cost-to-wash. | Part 0 (the permanent-Douceur-limits list). | Same class as R3 — a permanent demand-authenticity limit, stated as principle. |
| R5 | **Build-immutable #3 companion pointer** to `design/owned-residuals.md`, for parity with #5/#6/#7/#8. | Build-immutable #3 (Part IX). | Not a lost principle — a lost *pointer*; #3 names an "owned, named residual" and now links its register, matching its siblings. |

---

## Notes for the ratifier / regen

- No immutable was **traded** — this is a presentation restructure. The one new
  principle (T-AR) is **tenet-tier**, not immutable, per the governing rule.
- `TENETS.md` is referenced by generated site pages and by `.claude/CLAUDE.md`
  (Part 0 / Part IX / Part VI anchors). The Part numbers and the Immutable set are
  preserved, so those anchors still resolve. **Regen note (not run — no
  side-effecting steps taken):** if the docs→site pipeline stamps `website/*.html`
  from these sources, a regen (`scripts/gen_*.py` + `check_links.py`) is owed at
  ratification time; this draft does not run it.
- New internal links added (`tenets-history.md`, `network-protection.md`,
  `design/m0.md`, `decisions.md`, `ROADMAP.md`) should be checked by
  `check_links.py` at ratification.
- **CHANGELOG entry added (fix-cycle, 2026-09-01):** the "Docs" entry the prior
  draft deferred is now in `CHANGELOG.md` under `## [Unreleased] → ### Docs`.
  **`website/changelog.html` regen IS owed at ratification** — `CHANGELOG.md` is
  the source and `scripts/gen_changelog.py` stamps `website/changelog.html` from
  it. This draft does **not** run it (no side-effecting/generated-file writes in a
  pending-ratification draft; the `website/*.html` are generated, never
  hand-edited). Regen command to run at ratification:
  `python3 scripts/gen_changelog.py`.
- **Both `docs/TENETS.md` and `docs/tenets-history.md` are in the worktree draft
  commit and merge together** — confirmed tracked in HEAD, and `TENETS.md` links
  `tenets-history.md` in 3 places (foot pointer + build-immutables provenance +
  register). Neither can merge without the other.
