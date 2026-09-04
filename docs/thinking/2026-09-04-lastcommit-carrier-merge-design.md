# The `LastCommit` carrier — bringing the ratified branch to main behind its hard gates

**Date:** 2026-09-04 · **Seat:** PLANNER (deliberation) → TESTER (gates) → BUILDER (rebase + MG-C) · **Status:** BUILT — rebased, every gate green; PR pending

**Binding inputs:** O1 and O2 are OWNER-RATIFIED (2026-09-03): the carrier is an additive open-era format
field, folded into `Hash()`, with the stated validity and transition rules; the stamp goes 3 → 5.
`R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md` §10; the composed-direction cert
(`LASTCOMMIT-CARRIER-residuals-composed-direction-RESEARCH-CERTIFICATION-2026-09-03.md`, CD-0…CD-4); the
delta cert (`LASTCOMMIT-CARRIER-26977a4-DELTA-CERTIFICATION-2026-09-03.md`, MG-C §6); the owner's
definition-program item 4: "rebase the carrier branch onto main NOW behind the two hard merge gates".

## 1. The mechanism, in one paragraph

The carrier branch (`builder/lastcommit-carrier`, base `1adca0f`, 3 commits, +3,123 lines) has fallen 43
main commits behind **because** every session since 2026-09-02 merged to main while it waited on its
certification residuals; the longer it waits the more the three shared files (`chain.go`,
`floorbox_recompute_stateroot_v5.go`, `chainrole.go`) diverge, and the one hazard that cannot be reviewed
by eye is CD-0: main's `Hash()` literal folds `IssuerKeys`, the carrier's folds `LastCommit`, and a naive
merge silently drops one from the signed body. This session addresses that **by encoding the gates FIRST
on main** (the `Hash()` literal reflection pin; the v5 tag-set equality; the `CheckEquivocation` golden
accept set; the MG-C genesis rule) and rebasing the carrier onto a main that already carries them, so the
merged literal is held to "both fields, no hole" by a test rather than by attention.

## 2. Options

| Option | Cost | Benefit | Call |
|---|---|---|---|
| Rebase the carrier onto main with the gates already on main | one conflict-heavy rebase (3 files); the gates decide correctness | the ratified design lands; the freeze carry-list unblocks | **TAKE** |
| Re-implement the carrier fresh on main from the certified spec | days; loses the Tester's verified pins on the branch | cleaner history | REFUSED (the branch is certified as built; re-doing it re-opens the certs) |
| Wait for the box-entry round-A branch (held) first | indefinite; the held branch rewrites the same box-entry file | fewer conflicts later | REFUSED (the hold is ratified; the carrier does not depend on it) |

## 3. Shapes that are the Builder's

1. Rebase, not merge: three commits replayed onto main; resolve `chain.go` so the `unsigned` literal lists
   BOTH `IssuerKeys` and `LastCommit` (the pin decides); `Prune()` keeps `LastCommit`; `bodyHash` (R0.6)
   is the recompute path the carrier's hash rule rides on.
2. MG-C, the hash-covered half only: `AppendGenesis` REFUSES `LastCommit` (inside the preimage: authored
   content; `ErrGenesisLastCommit`). **Correction during the build (coordinator, 2026-09-04):** the `Atts`
   STRIP the gates commit introduced is reverted — CI showed it breaks the anchor bootstrap (four
   `core/node` fixtures fail with an era-3 root mismatch: they seed a VERIFIED genesis att by convention;
   production genesis carries no `Atts` at all and anchors seat at height ≥ 1 — the Researcher corrected
   the "anchor bootstrap" premise the same night). Genesis `Atts` keep main's pre-carrier behaviour
   (neither stripped nor refused); the disposal (strip-all vs seat-only-verified) is research-gated and
   not the Builder's. The three `Atts` gates in `genesis_stub_atts_test.go` are removed;
   `TestGenesisLastCommitIsRefused` stays and is armed.
3. The carrier fold runs before this block's bond regs / TTL / slashes; pinned like rotate-LAST.
4. Not in this PR (owed before the stamp raise, tracked on their Rocks): R-CARRIER-BYTES value (pony
   measurement), R-CARRIER-PRUNED-HASH end-to-end test, R-CARRIER-MODELCHECK (seating agreement),
   R-CARRIER-PARENT-BINDING (`HeadRef`), R-LOGROOT-FORMAT-SCOPE. The carrier merging does not activate
   era 4 on any network (eras are dark; the stamp is not raised here).

## 4. Gates

R-HASH-LITERAL-PIN and R-V5-TAGSET-EQUALITY (green on main, teeth by injection, must stay green on the
rebased branch); CD-2 golden accept set (green before and after); MG-C, hash-covered half only (see §3 item 2 for the correction): `TestGenesisLastCommitIsRefused` (armed
when the field exists); the branch's own Tester pins (`lastcommit_carrier_pins_test.go`,
`redteam_carrier_boxsplit_gate_test.go`) all green after the rebase; the R0.6, R1.x and model-check
tiers green under `-short`; race on `core/chain` + `core/node`.

## 5. The build — what the rebase actually resolved (Builder, 2026-09-04)

Base: `cf91f18` (= main `b328268` + the four gates). Three carrier commits replayed; one fix commit on
top. Every resolution below was decided by a gate, then read for sense.

| File | Main had | Carrier had | Kept |
|---|---|---|---|
| `core/chain/chain.go` — `Block` fields | `IssuerKeys` (cbor 17, R0.4b) | `LastCommit` (cbor 18) | BOTH; the carrier's field doc now says 17 is `IssuerKeys` on main |
| `core/chain/chain.go` — `bodyHash()` literal | `bodyHash` (R0.6 recompute, `Hash()` memoizes) folding `IssuerKeys` | `Hash()` folding `LastCommit` | main's `bodyHash` shape, literal names BOTH (`TestHashLiteralPinsEveryHashCoveredField` decides) |
| `core/chain/chain.go` — `Hash()` comment | the R0.6 accuser-chosen-digest paragraph | the linkage-token / no-consensus-on-a-pruned-body paragraphs | both, joined (they are complementary) |
| `core/chain/chain.go` — `AppendGenesis` | no `Atts` rule (pre-cf91f18) / `b.Atts = nil` (cf91f18) | `ErrGenesisAtts` refusing `Atts` OR `LastCommit` | refuse `LastCommit` only (`ErrGenesisLastCommit`); `Atts` as main pre-cf91f18 |
| `core/chain/floorbox_recompute_stateroot_v5.go` | `issuerKeyCommit` in the v5 tag set; box-entry shape | `validateCarrier` call at (0a), `hasCarrierSigners` dispatch, `ParentProposer` witness | auto-merged cleanly; tag set intact (`TestV5TagSetEqualityAcrossStatehashAndBox` GREEN) |
| `core/node/chainrole.go` | `pendingSlashes` packing under `SlashesBytesCap` (R0.6) | `b.LastCommit = n.chain.HeadCarrier()` before `PopulateEra4Roots` (two sites) | auto-merged cleanly; both survive (`r06_slashes_cap_packing_test.go` GREEN) |
| `ROADMAP.md` | the definition-program (newer) carrier Rocks | the round-A / boxsplit-era (older) Rocks | main's text; the four branch-only Rocks ported (BOXSPLIT, SIG-COMPOSITION→closed, PREFIX-ONLY, ORDER-ORACLE) |

Two non-conflict edits the gates forced:

- `hash_literal_pin_test.go` teeth-2 injected `Atts` by replacing the verbatim `IssuerKeys: b.IssuerKeys}`
  tail; the literal now ends in `LastCommit`. Injection moved to the literal's head
  (`unsigned := Block{Atts: b.Atts, `) so the fixture does not depend on which field is last.
- `core/node/r04b_c3_gates_test.go` `c3Chain` minted a **v5** genesis with an `Atts` entry and relied
  on it pre-seating `c3Attester`. Under O1 a v5 block's own `Atts` write nothing, so on the one caller
  with a v4 leg (`TestStaleIssuerKeyRegPreFlipBoot`, flip at 24) the height-1 v4 block seated the
  attester AFTER `PopulateEra3Roots` — the pre-existing era-3 seating-after-root property, which the
  fixture had only dodged by pre-seating. The genesis version now follows its era (v5 only where
  `IssuerKeys` ride on it or era-4 is active from height 1; otherwise `chain.BlockVersion`, like the
  production genesis). Not a rule change: the production genesis is sub-v5 with no `Atts`.

Outcome: `go test -short ./core/chain/ ./core/node/` GREEN; `go test -race -short` on the same
packages GREEN (see the PR).
