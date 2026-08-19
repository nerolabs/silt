# Fresh-eyes audit → the M1 pivot and the ordered roadmap (2026-08-19)

**What this is.** The output of the fresh-eyes audit (non-coding; builder/PE/research stood
down). Method: read the canon, then **verify every "shipped" claim against code and tests**
— the model-checks and adversarial wire drills were *run*, the economy and operational
surfaces *inspected*, with file:line evidence. Nothing below is taken from a summary doc.
The audit's verdict produced an owner decision (the M1 pivot, D-M1-PIVOT in
[`../decisions.md`](../decisions.md)) and an **ordered roadmap**
([`../../ROADMAP.md`](../../ROADMAP.md) "The ordered path"), which this entry records the
reasoning for.

---

## 1. The audit verdict, in one paragraph

**silt is M0-strong and M1-hollow, and the imbalance is now the project's story.** The
trust plane verifies end-to-end: every claimed M0 mechanism is real, wired, and green
(bond, weight-quorum, finality gate, blind tokens over TCP, slashing, C2 metric + latch,
epoch freeze, WS checkpoint); the model-check is genuine (exhaustive coalition enumeration,
failing-first provenance, seconds on a laptop); all four marquee adversarial drills now run
over real TCP (#462 retired the PE's "undrivable" residual). But the durability economy —
the thing the tenets call the wound that killed Freenet/GNUnet (S7) — is **built,
test-proven, and switched off in every shipped node with no enable path**; bandwidth is
unpriced; a hobbyist cannot keep a node alive through a reboot; and the deliberation record
since 2026-08-10 (~27 dated entries) is entirely consensus/OOM with **zero** entries on
economy, packaging, or adoption. **M1 is the binding constraint.**

## 2. Verified findings (evidence-cited)

### M0 — near its gate; the remaining tail is small and enumerated

- All M0 mechanisms confirmed shipped, none stubbed (`core/bond/bond.go` v3 seed;
  `requireEpochWeightQuorum` + `ErrPreFinalityReorg` enforced in `core/chain/chain.go`;
  `core/blindtoken` RSA-FDH + `TestUnlinkablePublishOverTCP`; `SlashEquivocation`;
  `C2Metric()` + `everMature`; `-ws-checkpoint` refuse-to-start). Invariant-A/B guardrails
  are real (reflection guard; safe-defaults test). All 21 `core/...` packages green.
- The four adversarial wire drills PASS over real TCP (equivocation-slash, partition-heal,
  forged-block, low-bond). Prose caveat: "the #184 adversarial drills" is issue-number
  phrasing — the corpus is 4 wire drills + ~30 model-check oracles, and should be described
  that way.
- Remaining before #183: the two DoS gates (inbound-cap consensus-priority lane +
  per-peer fairness — PE ruling in the #465 CHANGELOG entry; `MsgSubmitBondReg` CPU gate),
  a mature-regime confirming run at depth, and **the engagement itself** — #183 is a
  procurement action with no owner, no candidate, and the longest lead time of anything on
  the board.

### M1 — structural holes, zero recent effort

- **The economy is default-off with no enable path.** H7 proof-of-correct-repair is real
  and genuinely adversarially tested (`core/node/redteam_repair_claim_test.go`: garbage
  claim → slash, data-less → deny, honest → paid), and the serve auto-skim is live-wired
  (`core/node/node.go` `RecordServeToObject`). But the entire bounty path gates on
  `RepairBountyBase > 0` and **the default is zero, set only in tests**; `FundDurability`
  and `EnableDemandBank` have no non-test callers; the prepay→skim→bounty loop closes in
  exactly one sim test. **`credit.G` — "the one number to instrument" (S7) — has never
  been computed on live data** (unit-test-only caller). "Durability is funded" is true of
  the code and false of the network.
- **Bandwidth is unpriced** — no proof-of-delivery, no bandwidth→credit path; the relay
  code's own comments name the open-relay "free bandwidth faucet"; the NAT load test
  saturates at N=24 concurrent fetches. The first production deployment hit exactly this.
- **Operational floor is missing.** A raw unsigned `go build` binary, no service unit, no
  installer, no updates (release workflow dormant since the July 0.1.x pre-releases).
  Cold-start rescans the whole store (`LoadProofs` is O(store), minutes on a large node —
  only the listener-bind was deferred). Reprovide re-signs **every held chunk** and walks
  the DHT per unique key each interval (O(held) — only the stack-safety was fixed in #471).
  Publish carries a 360 s bound on a mature network.
- **Evidence hygiene.** The "return-to-2GB, field-confirmed" headline rests on
  **untracked local logs**; no RSS telemetry exists in the harness; sibling runs of the
  same commit produced both PASS and FAIL; the committed `report.md` is a pre-fix run; and
  the retention prune has **never engaged at production parameters** (engages >h64; runs
  reached h63). For a project whose build-immutable #7 is "evidence or nothing," the
  flagship claim resting on uncommitted prose is a self-discipline breach — named here so
  it gets fixed, not repeated.

## 3. The owner decision: pivot to M1, on an ordered roadmap

**Hypothesis (owner):** M1 completeness — the live economy + cheaper heights — will itself
drive better field tests and a more valid red team, and some standing M0 gaps (e.g. the
h64 prune-depth gate) are stalling **because heights are too expensive to accrue** inside
a run's budget (1.5 MB bond proofs per reg, synchronizer round durations, the 360 s
publish bound). The audit supports this transmission mechanism: cheaper heights → deeper
runs → the prune field-exercised → the M0 field confirmation falls out as a byproduct.

**The stronger form, which the roadmap leans on:** red-teaming today's HEAD would certify
a **de-fanged system** — bounties off, demand minting nothing. #183 against the
economy-off config validates a network nobody will run. Economy-first makes the M0
certification *more valid*, not just more convenient.

**Three corrections folded into the decision:**

1. **The receipt-forgeability landmine.** The demand receipt is forgeable with zero object
   bytes (the per-object PoR seed is public — owned-residuals B3), safe today *only*
   because demand has no consumer. The moment Proof-of-Delivery converts receipts into
   credits, forgeability goes live. So PoD is **not** flip-the-switch enablement like the
   storage economy: it has a crypto prerequisite (bind receipts to served bytes) and a
   research consult before code. Sequenced after economy-ON, never before.
2. **The firewall is non-negotiable.** Delivery credits fund durability and relay
   compensation, **never consensus standing** — fusing delivery into standing is the
   γ→1/N minefield (#182). D-S7's coin-free-standing posture is unchanged by the pivot.
3. **Backward attribution corrected.** The consensus/OOM months were not over-investment —
   a crash-looping, wedging chain moots everything, and that work is why the floor now
   verifies. The correct lesson is: *the investment completed and we didn't pivot.* From
   here, marginal trust-plane effort is polish; marginal M1 effort is structural.

**On roadmap looseness (owner):** "tenets are the roadmap" has been too loose — it let a
month of effort pool on one axis without a forcing function to rebalance. The tenets stay
the *destination*; the roadmap now carries an explicit **ordered path with phase gates**
(ROADMAP.md "The ordered path"), and the prior sequencing rule "M1 opens only after the M0
gate" is **superseded** by D-M1-PIVOT (the M0 tail is small and enumerated; the economy
and height-cost work now interleave, for the reasons above).

## 4. Intake: the second private downstream handoff (off-repo)

The first production deployment (a streaming workload; private handoff, kept off public
repos per standing practice) delivered two more artifacts. Disposition:

- **A working signed-installer + background-service reference (macOS).** Developer
  ID-signed, notarized, stapled `.pkg`; launchd LaunchAgent (`RunAtLoad` + `KeepAlive`);
  a service-wrapper CLI; the OpenSSL-3.x p12 `-legacy` gotcha and the notarization
  requirements documented. **silt should own the generic version** (Phase 5): per-platform
  service packaging + operator-consented self-update (R4: never silent — signed manifests,
  operator-controlled; a node pulling its own signed updates over the swarm is the
  dogfooding endgame). The downstream POC is arm64/macOS-only with deployment-specific
  identity baked in — a reference, not a vendorable artifact.
- **A large-file publish crash report (sole-holder daemon, ~3,400-segment ingest →
  `fatal error: stack overflow`).** **Verified already fixed on HEAD:** same
  inline-recursive walk-continuation root cause as #471's Finding-2; the trampoline
  (`core/node/node.go` walk terminal, posted through the loop) covers the publish-placement
  trigger, and the failing-first guards (`core/node/repair_recursion_test.go`,
  `TestAnnounceHeldTrampolinesWalkTerminal` + `TestResolveProvidersTrampolinesWalkTerminal`)
  pin it. The downstream fork should rebase onto main. No new build item.
- **One genuinely open finding: concurrent publish → HTTP 502 from the inbound cap.** A
  local API publish flood trips the global `-inbound-cap` budget and **fails the caller
  mid-ingest** instead of backpressuring gracefully. This is the same v1-global-budget
  coarseness the PE ruling already flagged (a flood can stall consensus behind the cap) —
  the fix belongs in the **same Phase-1 work item**: per-peer fairness + a
  consensus-priority lane + local-API publishes shed load by slowing, not by 502.

## 5. Where the next sessions start

Read ROADMAP.md "The ordered path" and execute Phase 1 top-down. Standing parallel lane:
start the #183 procurement search immediately (longest lead, zero code dependency), and
commit field-run artifacts + RSS telemetry so every headline claim has a citable,
in-repo artifact (build-immutable #7 applied to our own claims).
