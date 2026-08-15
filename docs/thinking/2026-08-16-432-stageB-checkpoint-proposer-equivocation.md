# 2026-08-16 — #432 stage-B checkpoint: the two-phase gather works; one open semantics point (proposer equivocation in era 2)

## What is built and passing

- **Stage A (chain layer): green.** Phase/round-scoped era-2 signatures, two-quorum
  (prepare-QC + precommit) `ValidateCommit`, round-not-in-hash, round-scoped
  `VerifyEquivocation`, SignMark/markstore migration. `core/chain`, `ports`, `markstore`
  suites all pass. (`core/chain/rounds_test.go`.)
- **Stage B (node layer): two-phase gather runs end to end.** proposeBlock → prepare quorum →
  prepare-QC → durable lock → precommit quorum → commit + broadcast, over the real loop and
  held delivery (verified by a scratch trace: propose→prepare×2→prepareQC→precommit→commit,
  14 deliveries, commits). New wire kinds (MsgPrepareQC/MsgPrecommitReply/MsgRoundChange/Ack)
  routed and reply-correlated. Round machinery (`core/node/rounds.go`): per-height
  lock/round/round-change state, deterministic sweep-count advance (`maybeAdvanceRound`),
  new-view certificate carrying the highest prepare-QC forward, restart lock re-hydration.
  Slot-scoped watermark (`signAllowedAt`/`recordSign`/`recordSignLock`). Drain + view-change
  both propose through the same two-phase path.

## The one open point (STOP-and-verify, not guess — I5 honest-slash trap territory)

Two tests still red — the #345/#378 equivocation drills — for a single, well-understood
reason, and it is a genuine protocol decision, not a bug to patch blind:

**In era 1, a double-proposing equivocator was caught via its bare-hash `ProposerSig`
appearing in two conflicting blocks. In era 2, `consensusSigScopes` (equivocation.go)
deliberately excludes authorship signatures — a proposer re-proposing a fresh value at a
higher round after a view-change is HONEST (the certification's I5 requirement). So the
double-PROPOSER is no longer slashable evidence.**

The question this forces: *in the two-phase era, what makes a Byzantine proposer's
double-proposal slashable?* The literature-faithful answer (Tendermint) is that the proposer
**prepares (prevotes) its own proposal** — a round-scoped consensus signature — so two
different proposals at the same `(h, r)` become two prepares at `(h, r, prepare)` over
different hashes = the existing era-2 slash rule catches it, while a cross-round re-proposal
stays honest. Concretely: the proposer should include its **own** prepare in `PrepareQC` and
its **own** precommit in `Atts`.

**Why I am not just doing that right now:** `collectQuorumSigs` skips `id == b.ProposerID()`
for COUNTING (the proposer is counted separately via `countAnchorSupport(proposer, seen)` /
`SupportMeetsQuorum(proposer, …)`), and `supportMet` in the gather counts around the author.
Injecting the proposer's own signatures into the QCs interacts with that proposer-skipping
arithmetic — the exact quorum-counting the #402 certification hardened against "size-set ≠
membership-set" drift. Getting it a hair wrong risks either a missed equivocation OR a false
honest-slash (I5, the #397 wound). That is a verify-first change, and it is the wrong thing to
land tired at the end of a long session.

## Decision

Checkpoint here on the feature branch (`feat/432-two-phase-rounds`, NOT merging — two drill
tests red by design pending this point). Next session, first task, fresh:

1. Make the proposer contribute its own `(round, prepare)` and `(round, precommit)`
   signatures to its block's certificates; verify `collectQuorumSigs`'s proposer-skip still
   yields identical counts (the anchor-majority and weight-quorum math must be byte-identical
   to today — add an explicit test that a v2 commit's quorum count excludes the proposer).
2. Confirm the #345/#378 drills catch the double-proposer again (now via the round-scoped
   prepare), AND add the mirror I5 assertion: an honest proposer re-proposing across a
   view-change is NOT slashed.
3. Then the S1/S2 oracles (the merge gate) + the wedge oracle turning GREEN.

If step 1's interaction with the proposer-skip arithmetic proves subtle, it is a research
check (it touches the quorum-counting rule), not a build-through.

## Everything else stage-B-remaining (unchanged from the build plan)

- S1 (delayed lower-round quorum) + S2 (equivocate-then-misreport) oracles, both regimes,
  RED against a lock-free baseline, GREEN with the prepare phase — the certification's merge
  gate.
- The #432 wedge oracle (`oracle/i4-liveness-wedge`) merges GREEN into this.
- I2-restart oracle extended per-(h, r, phase); daemon flips `BlockVersion`→2 wiring.
- Canon: invariants I4 "mechanism SHIPPED", CHANGELOG, decisions.md fold-in.
