# 2026-08-21 — #503 fix deliberation: suppress the evicted identity's re-registration storm (Q1 only)

**Task:** close the dominant driver of the island OOM (#503) — a permanently
F2-evicted identity re-registering every ~30 s sweep, forever, with every layer
letting it through.

**Research gate honored.** The fix directions were certified before any code:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/503-bond-renewal-storm-RESEARCH-CERTIFICATION-2026-08-21.md`
(answers
`/Users/andrewedmond/Claude/claude/silt-reviews/research/503-island-bond-renewal-storm-CONSULT.md`).

## The options weighed (from the consult; decided by the certification)

| Option | Verdict | Why |
|---|---|---|
| Q1(a) proposer/arrival-side filter of slashed regs | **SHIP NOW** | zero block-validity change → zero mixed-version fork risk; drops the dominant OOM driver |
| Q1(c) client-side permanent backoff on own slash | **SHIP NOW** | liveness/UX belt; an honest evicted daemon stops re-broadcasting and says why |
| Q1(b) validation-rule refusal | deferred into Q3 | block-validity change; a mixed-version swarm forks on the very storm it stops |
| Q3 per-identity reg-inclusion rate bound (validity rule, R ≥ K, R ≈ TTL/4, slashed = R∞) | **required structural close, next version boundary** | Q1(a) is honest-majority-dependent; only a validity rule closes the adversarial ~1.5 MB reg flood |
| Q2 re-denominating the TTL ("meaningful height" / wall-clock) | **NOT touched — contraindicated** | Defect B decays on its own (0.19 < 1) once Defect A is gone; "meaningful height" would stretch the retention horizon on reg-heavy chains → prune never fires → worse OOM; four couplings (WS period, safetyDepth=2·TTL, EpochBlocks≪TTL, K≪TTL) stay untouched |

## The mechanism paragraph (build-immutable #6)

The failure is unbounded reg-block minting **because** an F2 slash deletes
`bonded[id]` (`chain.go:2354`), which makes `BondRenewalDue`'s first clause
(`bonded[id] < MinBond`, `chain.go:1328`) true forever; the daemon sweep
re-submits on that signal alone (`objectivechain.go:75`), no validation layer
consults `slashed`, and the apply-time skip (`chain.go:2313`) discards the
standing but not the block. This change addresses it **by** (c) gating the
client's submit and self-embed paths on `IsSlashed(self)` with an explicit
"permanently evicted" log, and (a) refusing a slashed identity's reg at the
receiver's queue (logged, per B5 never-refuse-silently) and filtering it at the
proposer's fold — so an honest evicted daemon stops asking, and honest
proposers stop committing what a dishonest one still asks for.

## Scope discipline

- Consensus rules, quorum arithmetic, TTL denomination: **untouched** (I1–I5
  all preserved by construction — no validity, quorum, set, or fork-choice
  change; statement in the PR body).
- The retention/serve working set (the other ~4× of the OOM) is **attributed
  to the unpaginated chain-serve encode** (evidence:
  `integration/cloudtest/503-retainer-evidence-fa501cc-50820/`) — that is PR
  #466's scope (PE-gated), not this fix.
- Q3 filed as its own issue (version-boundary work).

## Validation plan (Q4 of the certification)

1. Born-RED unit tests, then the fix turns them GREEN:
   - `core/node`: a slashed self does not submit a renewal (`SubmitBondRenewal`
     no-op + the F6 self-embed guard).
   - `core/node`: a proposer does not fold (and a receiver does not queue) a
     slashed identity's submitted reg.
2. Full suite (unit + race) — testing discipline.
3. e2e: the LOCAL island soak — post-slash the island must **quiesce**
   (bounded further commits, RSS flat ≥ 30 min); the pre-fix soak
   (fa501cc-50820) recorded the storm at ~3 blocks/min to h32 as the RED
   baseline.
