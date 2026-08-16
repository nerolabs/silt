# 2026-08-16 — #432: the proposer's self-prepare — and why it must be REQUIRED, not just contributed

Resuming the stage-B checkpoint's one open point (see
`2026-08-16-432-stageB-checkpoint-proposer-equivocation.md`): in era 2 the
double-PROPOSER is no longer slashable, because `consensusSigScopes`
deliberately excludes the bare-hash `ProposerSig` (a cross-round re-proposal is
honest, and the round is not in the hash, so authorship can never be
round-attributed). The two #345/#378 drills are red for exactly this.

## Evidence gathered before deciding (the state of the code)

1. **The proposer-skip is count-safe by construction.** `collectQuorumSigs`
   (chain.go:1739) skips `id == b.ProposerID()` BEFORE the phase/round
   exactness check and BEFORE `verifyAtt` — so a proposer signature placed
   inside `PrepareQC`/`Atts` is invisible to every count: the count floor
   (`RequiredQuorum`), the anchor majority (`countAnchorSupport` adds the
   proposer by AUTHORSHIP, exactly once), and the weight quorum (adds proposer
   weight by authorship). Injecting self-signatures cannot shift any quorum
   arithmetic. The one node-side trap: `gatherTwoPhase`'s early-stop counts raw
   `len(atts) >= quorum`, which WOULD inflate by one — fixed by counting
   non-author signatures only.
2. **"Contribute" alone does not fix the drills.** The drills commit their
   forks through `proposeAndCommitTo` (core/node/adversary.go) — the adversary
   primitive builds the QC from ONLY the target's signatures, and the target
   commits it, because the proposer counts toward the anchor quorum by
   authorship with NO signature of its own in the certificates. A Byzantine
   proposer simply omits its evidence. Making only the honest gather
   self-prepare fixes nothing an adversary does.

## The options weighed

- **(A) Contribute-only** (the checkpoint's literal step 1): honest gather adds
  self-prepare/self-precommit; validation unchanged. Cost: near zero. Benefit:
  none where it matters — the drills stay red (see evidence 2), and I5's
  "every safety violation is attributable" keeps a hole: a double-proposer that
  withholds its self-signatures forks the launch chain (it IS the quorum
  intersection in the 2-2 drill shape) with zero slashable evidence. Era 1 did
  not have this hole (`ProposerSig` is structural); era 2 without a required
  self-prepare is an accountability REGRESSION vs era 1.
- **(B) Contribute + REQUIRE**: `ValidateCommit` (era 2) additionally demands a
  verifying proposer prepare at `(PhasePrepare, round ≤ CommitRound)` inside
  `PrepareQC` — the era-2 analogue of era-1's structural `ProposerSig`, now
  round-stamped. A proposer that wants its block COMMITTED must leave
  round-scoped evidence; two proposals at one `(h, r)` are then two prepares at
  `(h, r, prepare)` over different hashes = the existing era-2 slash rule.
  Liveness check (the reason `≤`, not `=`): a locked value re-proposed at a
  higher round keeps its ORIGINAL author, who may be down — but its
  original-round prepare still endorses the block (the hash excludes the
  round), rides in the carried lock QC, and the proposer-skip makes it exempt
  from the round-exactness rule, so the re-proposal carries it into the fresh
  QC. Every lock descends from a QC assembled by the author's own gather
  (fresh gathers are author-run; forced gathers require a lock), so the
  carried author-prepare always exists.
- **(C) Round-scope the ProposerSig instead.** Refuted by stage A's own
  design: the round is deliberately NOT in the hash (re-proposal keeps block
  identity), so `ProposerSig` is round-blind forever; and `CommitRound` is set
  by whoever completes the gather, unsigned by the author.

## Decision

**(B).** The self-precommit is contributed (extra evidence density on the
honest path, per the checkpoint) but NOT required — the dead-author
re-proposal case cannot produce one (the author may never have locked).

**Process note (research gate):** the checkpoint pre-flagged this as a
research check if the proposer-skip arithmetic proved subtle. It proved clean
(evidence 1, plus an explicit count-neutrality test). The REQUIRE half is a
commit-validity rule addition beyond the certification's literal text, argued
here from the certification's own I5 frame (every safety violation
attributable) and era-1 parity; it is flagged for research eyes in the PR
body, and the branch does not merge before the S1/S2 oracle gate regardless.

Failing-first: the omit-loophole is demonstrated by a chain-level test (a full
two-quorum v2 commit with no author prepare must be REFUSED — red before the
rule, green after), and the #345/#378 drills flip green only once
`proposeAndCommitTo` is forced to staple the adversary's own round-scoped
signatures — the accountability doing its job.

## Post-certification correction (research, 2026-08-16 — rule CERTIFIED as-is)

Certification:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/432-proposer-prepare-required-RESEARCH-CERTIFICATION-2026-08-16.md`.
**One arithmetic correction to this doc's framing (its §2):** my "zero slashable
evidence" claim at the A=4 drill shape was imprecise — two strict-majority anchor
sets at EVEN A must share a non-proposer attester (`2·⌊A/2⌋ > A−1`), who
equivocates and is itself slashable. The FULLY-unattributable two-fork commit
(proposer the sole culprit, zero evidence) exists at **odd A ≥ 5** and in the
**mature regime with a ≥⅓-weight proposer** (two >⅔-weight sets disjoint except
the proposer). At A=4 the real gap is that the PROPOSER escapes attribution and
accountability depends on catching a colluder. Research's read: this makes the
rule's necessity cleaner, not weaker. Follow-ups filed (non-blocking, do not
gate the merge): drill minimal-A fidelity (§5.1) and wire authentication for
the in-transit certificate-strip residual (§5.2, transport-layer).
