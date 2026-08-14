# 2026-08-14 — #402 launch anchor gate: encode the certified strict-anchor-majority rule, and make it structural (derived, not a knob)

**Context / trigger:** Building the research-certified #402 fix
(`silt-reviews/.../fork-anchor-gate-402-RESEARCH-CERTIFICATION-2026-08-14.md`): launch
finality must require a **strict anchor majority `⌊A/2⌋+1`** (=3 for A=4), counting
proposer-if-anchor, sybils excluded from the launch finality count. The certification
assigns the *threshold* to research and the *encoding* to build. Paced before coding per
the standing rule — and the pacing surfaced a premise correction that reshaped the fix.

**Evidence (build-immutable #7 — cited artifacts):**
- `git show 4faaee8:integration/cloudtest/topology.py` — the validator command line passes
  `-anchors`, `-attesters`, `-quorum`; it passes **no `-anchor-quorum`**.
- `cmd/silt/daemon.go:72` (same at 4faaee8) — `-anchor-quorum` flag **defaults to 0**,
  documented "0 = off"; passed unmodified into `chain.Config` at `daemon.go:492`.
- `core/chain/chain.go:1472` — the anchor gate engages only
  `if len(c.cfg.Anchors) > 0 && c.cfg.AnchorQuorum > 0 && !c.handedOff()`; at
  `AnchorQuorum == 0` it is **inert**.
- `cmd/silt/daemon.go:592` — the status line **"wheels engaged (young network — anchor
  quorum still required)"** derives from maturity state alone; it never checks whether an
  anchor quorum is configured. So every field daemon logged the gate as required while it
  was disabled (an S5 honest-observability defect).
- `core/chain/chain.go:657,675` — `attesterQualified`/`proposerQualified` admit any bonded
  validator (incl. sybils) or a launch anchor during the young window.

**Premise correction this evidence forces.** Issue #402, my consult, and the certification's
field framing all assumed the young-phase gate required ≥1 anchor (`AnchorQuorum=1`). The
code shows it was **0 = off** in run 4faaee8-22913. Consequences:
- Sybil blocks 11'/12/13 committed on 2 sybil attestations meeting `RequiredQuorum=2` — no
  free anchor needed. Q-A's "whose signatures are on the sybil fork" has a boring answer:
  sybils'. The planned in-process repro of the cloud attester wiring is likely unnecessary.
- Field C2 was held by the **finality gate** (`ErrPreFinalityReorg`, anchors refusing to
  reorg their finalized head), **not** by the anchor gate. The repro's "zero-anchor quorum
  refused" was true only because that test sets `AnchorQuorum=1` explicitly.
- The certified rule is **unaffected and strengthened**: the §1 "deeper root" (quorum sized
  over anchors but fillable from all bonded) wasn't merely deeper — it was the whole
  mechanism. And a new requirement appears: a safety invariant a missing default flag
  silently disables is the #380 class at its worst (nobody misconfigured — the *default*
  was unsafe).

**Options weighed:**

*D1 — how to encode the certified threshold.*
- **(A) Certification's literal encoding B:** anchor-only launch proposing +
  `AnchorQuorum = ⌊A/2⌋` non-proposer anchors (anchor-proposer + 2 = 3). Cost: correct only
  while proposing is truly anchor-restricted; a non-anchor-proposed block reaching a replica
  by another path isn't re-checked for the majority. Benefit: minimal; removes the
  sybil-proposed fork at the source.
- **(B) Encoding B + count proposer-if-anchor, require total anchor support `≥ ⌊A/2⌋+1`.**
  Cost: one extra term in the gate. Benefit: matches the certified statement verbatim, and
  stays intersecting even against an adversary-crafted non-anchor-proposed block —
  defense-in-depth at **zero** liveness cost (both are 3-of-4, 1-fault-tolerant).

*D2 — knob vs. derived (forced by today's discovery).*
- **(i) Keep the config knob, set it in topology.py.** Cost: the seam stays one missing
  flag from re-opening; the invariants checklist Q3 (arithmetic visible in code) stays
  unenforceable. Rejected.
- **(ii) Derive the threshold in code:** pre-handoff, objective + ByzantineQuorum + anchors
  configured ⇒ strict anchor majority `⌊len(Anchors)/2⌋+1` in force; config cannot lower it.
  Cost: unit tests that set `Anchors` without `AnchorQuorum` now feel the gate (fallout).
  Benefit: faithful to the certified rule (encoding is build's call); same medicine as the
  cold-start seam-2 refuse-by-default; safe-by-default.
- **(iii) Keep the knob but refuse-to-start below the derived value.** Cost: every deployer
  computes `⌊A/2⌋+1` by hand; legacy 0-configs break at startup anyway. Middle-ground with
  the downsides of both.

*D3 — #380 fold-in scope.* Guard only (refuse-to-start when a local `-quorum` floor exceeds
the launch-canonical requirement), **not** the "ignore the local floor in ValidateCommit"
semantic change — issue #380 explicitly leaves that research-gated.

**Decision + rationale:**
- **D1 → (B).** The verbatim-match plus the defense-in-depth is free (no liveness cost over
  A), and it removes the reliance on "proposing is definitely anchor-only" being airtight at
  every future call site — the gate itself enforces intersection regardless of proposer type.
- **D2 → (ii) derive in code — REFINED after test-fallout evidence.** Today's discovery *is*
  the core argument: the field seam was not a misconfiguration but an unsafe default, and a
  safety invariant must not be a knob a missing flag disables. **But scoping the derivation
  surfaced a distinction I initially missed:** `core/chain/trainingwheels_test.go` runs in
  **legacy (non-objective) mode** (no `SetBondVerifier`, MinBond 0, rep-based) with 2 anchors
  + `AnchorQuorum:1`, and asserts the gate refuses a no-anchor quorum (OUTCOME 1) and admits a
  1-of-2-anchor quorum (OUTCOME 2). The anchor gate therefore bundles **two** properties: (1)
  *capture-prevention* (need **some** anchor sign-off) — meaningful in legacy and objective;
  (2) *intersecting finality* (need a strict anchor **majority** so two forks can't both
  finalize) — meaningful **only in objective mode**, where a finality gate exists
  (`finalityQuorumActive` requires `objective()`; legacy keeps heaviest-chain reorg, forks
  heal). #402 is about (2). Bumping the requirement to `⌊A/2⌋+1` *universally* would silently
  change a legacy capture-prevention behavior that is **not** the bug — scope creep past the
  certified rule.
  - **Refined decision:** the derived strict-anchor-majority `⌊A/2⌋+1` (proposer-if-anchor
    counted) applies **in objective launch mode only, independent of config** — config cannot
    lower it, so `AnchorQuorum=0`/unset (the field footgun) no longer disables intersection.
    **Legacy mode keeps its existing behavior** (configured `AnchorQuorum` floor, non-proposer
    count). So the `AnchorQuorum` config field and `-anchor-quorum` flag **stay** (still
    meaningful for legacy/demo), but the objective safety property **no longer reads them** —
    the footgun is closed by making objective structural, without ripping a field across ~13
    test files.
  - **S5 status line falls out fixed for free:** the `daemon.go:592` "anchor quorum still
    required" line prints under `ch.Objective()` + `!EverMature()`; under the derived rule an
    objective young network *always* has the gate engaged, so the line is now **true** (the old
    lie was that `AnchorQuorum=0` disabled it while the line claimed otherwise). Enrich the
    wording to name the derived `⌊A/2⌋+1`, but no logic change needed.
  - **Objective-mode test fallout is desirable, not a cost:** the objective 2-anchor tests
    (`c2_metric`, `a_axis`, `h4_consensus`) set `AnchorQuorum:1`; the derived rule needs 2
    anchors (proposer-if-anchor + 1). Updating them to supply the real majority is the #303
    test-honesty correction — a green suite modeling a rule the field never ran is the problem,
    not the fallout. Each is verified to be "supply the true majority," not a masked regression.
- **D3 → guard only.** Stay inside the certified/decided scope; don't smuggle an
  undecided consensus-rule change into a fix PR.
- The S5 status line (`daemon.go:592`) is corrected to reflect the *real* gate state.

**What would change my mind:** research or the owner ruling that the encoding should be the
literal (A), or that the threshold should stay a deployer knob. Because this is a
consensus-rule change, the PR body carries the I1/I3 statements + the six-question checklist
at the gate site, and flags the field-attribution correction for research's eyes. No *rule*
changes beyond what was certified, so my read is this does not need a fresh consult before
building — but the correction comment on #402 lands first, and the PR surfaces the shift.

---

## Build-time discoveries (recorded because they were non-obvious)

**Seam-7 interaction (I5 accountable safety) — verified NOT weakened.** The derived majority
made `TestSeam7_LosingForkEquivocatorIsSlashedOnDetection` fail: it installs a losing fork via
`Reconcile`, which now rejects a sub-majority (2-of-4) fork as invalid. Before touching it I
traced the detection path (anti-pattern guard: don't reclassify a red-team test to make it
pass). Finding: `slashEquivocators(old, full)` runs on the **raw fetched blocks BEFORE
`Reconcile`** (`chainrole.go:671`), so equivocation detection is independent of fork validity —
#402 does not weaken seam-7. Only the test's *setup* broke. Faithful fix: an **odd** anchor set
(A=3, majority 2) lets two 2-of-3 anchor sets share *exactly one* anchor, so a lone Byzantine
anchor can still form a valid competing minority fork with honest anchors cleanly split — the
lone-culprit property is preserved. (Even A forces ≥2 overlap → a valid competing fork would
need ≥2 colluding double-signers; that is the real I5 residual, worth its own test later, but
not this test's scenario.) A subtlety surfaced and is owned: `⌊A/2⌋+1` is the *crash/partition*
intersection bar for **trusted** anchors (immutable #3); a **Byzantine** anchor (the seam-7
culprit) can still fork for odd A — the certification's flagged residual ("for Byzantine anchors
use `bftThreshold(A)`"), and exactly why seam-7's slashing accountability matters.

**#380 (divergent `Config.Quorum` floor) — DEFERRED out of this PR (D3 refined).** The plan said
"fold the #380 guard in here." On inspection #380 is about the **general** quorum floor
(`RequiredQuorum` / `max(Quorum, bftThreshold)`), a *different* mechanism from the anchor gate;
my change doesn't touch it. #380 already has a shipped workaround (uniform `-quorum` in
cloudtest, PR #381) and its structural fix is a separate consensus-rule change (research-gated,
per the issue). Folding it here would blur the PR's invariant statements and mix two consensus
concerns. Decision: keep #402 focused; #380's structural fix is a scoped follow-up. Recorded so
the plan's "fold #380" directive is visibly, deliberately overridden — not silently dropped.

**Gather-vs-validate predicate drift (found via a flaky e2e).** The full e2e run flaked on
`TestObjectiveColdStartWithSatelliteValidator` (passed 6/6 standalone at ~2.3s; the failure was
under full-package process contention against the now-stricter 3-anchor bar). Per #7 I did not
re-run-on-a-theory — code inspection found the real gap: the proposer's gather stop-predicate
`SupportMeetsQuorum` (`chain.go`) checked the count quorum + the mature-epoch weight rule but
**not** the launch anchor gate, so it could stop at `RequiredQuorum` heads while `ValidateCommit`
now demands ⌊A/2⌋+1 anchors → the proposer commit-attempts a coalition its own `Append` rejects
(`ErrAnchorRequired`). Latent bug regardless of whether it was this test's exact trigger. Fix:
extracted `requiredLaunchAnchors()` + `countAnchorSupport()`, shared by BOTH `ValidateCommit` and
`SupportMeetsQuorum`, so the gather stops on exactly what validation demands and the two **cannot
drift** (the third-time-rule applied: encode the invariant as one mechanism, not two parallel
checks). Regression test `TestSupportMeetsQuorumRequiresAnchorMajority402` locks it (V5).

**Status:** BUILT & GREEN — full `go test ./...` passes (unit + node + e2e); failing-first RED→GREEN
confirmed; docs pipeline (changelog/roadmap/buildlog/links/claims 32/32) green. Docker integration
suites (sybil et al.) flagged for a confirming run. On branch `fix/402-derived-anchor-majority`.
