# 2026-09-07 — Lane A1: building `D-CONSENSUS-ARMING` (the h43 round-ladder desync fix)

**Status:** deliberation of record for the build PR. Ships in the same PR as the code.
**Governs:** `docs/decisions.md` `D-CONSENSUS-ARMING` (owner calls 18–20, ratified 2026-09-07) on the
certification `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md`.
**Invariants touched:** I4 (operation-liveness; the arming rule and the synchronizer). I1 is preserved by
`slotCompare`'s round term and the lock rule, never by the arming rule (certification §4.1). I2, I3, I5
untouched: no signature site, no set change, no block field, no fork-choice input.

## The evidence this build stands on

- **G-H43-1 is RED on main.** `core/node/modelcheck_h43_arming_test.go`
  `TestModelCheck_H43_HeterogeneousArmingMustCommitWithinFPlus1Rounds`, run at the Tester's branch head
  `8b1e1ef` (= main `c51b97f` + two test-only commits): *"height 9 did not commit within the certified f=1
  bound 190 s of the kill … per-seat rounds reached: map[0:2 1:2 2:2 3:0 4:2 … 11:2]"*. 0.12 s wall.
- **The mechanism, named** (`build-process.md` #6): the height stalls **because** `core/node/rounds.go:306-310`
  arms the round clock on LOCAL mempool content, so a seat holding no work never runs the pacemaker and the
  liveness bound is proved over a population that does not exist on the wire (M1); the disarmed branch
  ZEROES `rs.Sweeps` so intermittent work never accumulates a round (M1b); `proposeBlock` re-derives the round
  from `rs.Round` and refuses the certificate it was handed (M2, `chainrole.go:1040-1047`); and `Changes[r]`
  is a point record, so the catch-up target is structurally the lowest quorum-bearing round (M3). This change
  addresses M1/M1b **by** arming on a replicated condition and holding the counter (A); M3 **by** suffix
  semantics in the catch-up predicate plus a transferable round certificate (B); M2 **by** passing the
  certificate's round through to the gather (C).

## Options weighed

| Option | What it is | Cost | Why not / why |
|---|---|---|---|
| **(A) alone** | replace the local-mempool guard; hold `Sweeps` | smallest diff; turns G-H43-1 GREEN by itself in the model | Leaves M2 and M3 in place: the designee-ahead-of-certificate race and the one-shot, un-relayed round-change stay. Ratified scope is (A)+(B)+(C) in one PR. |
| **(A)+(B)+(C), certificate round-exact** *(chosen)* | suffix semantics in the CATCH-UP predicate; the certificate stays "envelopes for exactly r" (PBFT new-view, DiemBFT TC) and becomes a transferable wire object; designee proposes at the certificate's round | one new wire kind pair, appended; a per-sender declared-round map; a once-per-round certificate broadcast | Faithful to the schema the certification cites: PBFT §4.5.2's suffix is the *catch-up* rule ("smallest view in the set … even if its timer has not expired"); PBFT's new-view and DiemBFT's TC are formed from messages for exactly the target view/round. Under (A) every member arms within one hop, the suffix catch-up coalesces them at the highest round ≥ ⅓ weight has declared, and each catch-up jump broadcasts a round-exact envelope for the target — so the exact certificate forms without loosening it. |
| (A)+(B)+(C), certificate suffix too | `newViewFor` accepts `NewRound ≥ round` | none in code size | A node whose envelope says "at round ≥ 4" would be counted into a certificate for round 1 that it will never act at (its `rs.Round` never moves down). That lets a low-round designee assemble a certificate from members who have left the round — the h43 shape, re-created inside the certificate. Neither PBFT nor DiemBFT forms a view certificate this way. Declined; the Researcher's re-certification is asked to confirm. |
| Tendermint L55 on every message type (B.3) | count proposals/prepares/QCs at a higher round toward catch-up | more handler surface | The certification lists it as *optional, cheap*. Deferred to keep the PR to the ratified three parts; a proposal-carried certificate already jumps the receiver (`chainrole.go:285-287`), and the new standalone certificate covers the rest. Named residual `R-H43-L55-ANY-MESSAGE`. |

## The decision, and the shape of each part

**(A) Arm on a replicated condition; hold the counter.** `heightRounds` gains `Armed bool`, set when this
node VERIFIES any consensus message for the working height: a proposal that passes `ValidateProposal`, a
prepare-QC that passes `VerifyPrepareQC`, a round-change that passes `verifyRoundChange`, or a round
certificate that passes `newViewFor`. `maybeAdvanceRound` runs the clock while `Armed || pending work ||
drain in flight`; when disarmed it returns WITHOUT touching `rs.Sweeps`. `heightRounds` is recreated on every
head move, so a commit disarms every seat — B6 quiescence holds exactly as today when nothing is in flight
anywhere. Arming only on VERIFIED messages means only a qualified validator can start the network's clock
(`verifyRoundChange` requires `AttesterEligible`); the price the certification names — ≤ N round timers per
contested height — is unchanged.

**(B) Suffix catch-up + a transferable certificate.** `heightRounds` gains `Declared map[NodeID]uint64`
(the highest round each verified sender has declared). `maybeCatchUpRound` targets the highest `r > rs.Round`
such that `{id : Declared[id] ≥ r}` meets `RoundCatchupMet` — monotone in r, so the stale-`Changes` pin of
M3 is gone; the #549 target rule (highest qualifying round) is unchanged. `recordRoundChange` now checks
the round-exact quorum at EVERY node, not only the designee; the first time a node holds a quorum-grade
certificate for r it broadcasts `MsgRoundCert{Height, Round, Raws}` once to its sync targets and, if
`r > rs.Round`, enters r (`via=round-cert`) — the same entry rule a proposal-carried certificate already
triggers. A receiver validates the certificate with `newViewFor` (same predicate as an attester applies to a
proposal), records its envelopes, enters r if above, and acks. No re-broadcast: one hop from the assembler
is G-H43-4's requirement, and every jump already gossips a fresh round-change.

**(C) The designee proposes at the certificate's round.** `proposeBlock` becomes a thin wrapper over
`proposeBlockAt(b, …, view *viewAt, done)`; a nil view derives `(round, newView)` from `rs` as today, a
caller-supplied view is used verbatim (still run through `newViewFor` for the forced-value refusal).
`proposeAtNewView`'s fresh leg passes `{round, newView}`.

**Wire.** `MsgRoundCert` / `MsgRoundCertAck` are APPENDED to `ports.MsgKind` (the numbers of every existing
kind are pinned by `TestR211MsgKindNumbersArePinned`). The certificate is a wire object only: it never enters
`Hash()`, `apply`, or `heavier` (I5 by construction). An older peer that does not know the kind never
replies; the sender's callback is empty, so a mixed-version swarm loses nothing but the relay.

## What is NOT in this PR

- #380 (`R-380-LIVENESS-FACE`, G-H43-8): its own GATED item and ratification (`D-CONSENSUS-ARMING` (20)).
- G-H43-7 (the harness honesty rule and the 190 / 380 s tiers in `integration/cloudtest/scenarios.sh`):
  a separate harness PR before Lane A3's run.
- The `consensus-invariants.md` I4 literature line the certification proposes (§8.6) and the
  `consensus-model-check.md` sixth-recurrence amendment: both land HERE as doc edits, since the gates that
  enforce them ship in this PR.

## Gate plan (RED-first, Tester)

G-H43-1 RED at main (above) → GREEN with (A). G-H43-2 RED at HEAD (M2) → GREEN with (C). G-H43-3 RED
(M3) → GREEN with (B). G-H43-4 RED (no certificate object) → GREEN with (B). G-H43-5 GREEN by design (I1
non-regression pin, ablated against a reverted `slotCompare` round term). G-H43-6 GREEN (fixture-blindness
pin, on the branch). The whole `core/node` and `core/chain` suites, then the model-check tier, then
`integration/sim`, before the PR is opened; the blind PE reviews the diff and the Researcher re-certifies the
composed change before merge, per the ratification.
