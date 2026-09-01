# Issue → ROADMAP migration map (2026-09-01)

**Purpose.** ROADMAP.md is being established as the single source of truth (SSOT) for tasks;
the GitHub issue tracker is being retired as a task driver. This table records, for every open
issue, its disposition and a close-recommendation. It **drives the later close step** (a
separate action — this migration does NOT close any issue).

**Method.** The 40 open issues come from the Researcher's read-only triage
(`/tmp/docs-trueup-issues.md`). Every issue proposed for PORT was re-verified with
`gh issue view <n>` at HEAD `dfeb1d5` before its content was ported. Disposition values:
- **PORTED-to-<file:section>** — unique content migrated into the SSOT; safe to close after review.
- **ALREADY-IN-ROADMAP-<where>** — the issue duplicates a ROADMAP line; close, no port needed.
- **DONE-already** — resolved per ROADMAP/CHANGELOG; verify the merge, then close.
- **STALE-CLOSE-no-port** — depth-war-era / superseded / low-value; close without porting.
- **KEEP-open** — a live evidence anchor the ROADMAP points at (not a task); keep open.

**Close-recommendation** is a proposal only; the owner confirms each against the issue itself.

## Mapping table

| # | Title (short) | Disposition | Close-recommendation |
|---|---|---|---|
| 651 | paid-relay resolver stopped-loop hardening | ALREADY-IN-ROADMAP (archive scheme-2 §2 PoD follow-on; tracked, not a blocker) | KEEP-open (tracked follow-on, deferred to graceful-shutdown) |
| 621 | pin rotateEpoch-is-last-in-apply | ALREADY-IN-ROADMAP (Boulder 1 R1.0 / R-CARRIER-REFLECTION area) | KEEP-open until R1.0 pin lands, then close |
| 616 | CPU-time O(depth) regression gate | ALREADY-IN-ROADMAP (test-gate, Boulder 1/3 oracle area) | KEEP-open (a test gate to land) |
| 597 | keystone revLog history-dependence | DONE-already (CERTIFIED-answered, two-root refinement merged) | Verify cert merged, then close |
| 586 | economy 11b skim-observer arming | PORTED-to-docs/thinking/2026-09-01-residual-defect-repro-recipes.md + ROADMAP Residual backlog | Close after review (recipe ported) |
| 574 | publish clients unreachable mid-run | PORTED-to-docs/thinking/2026-09-01-residual-defect-repro-recipes.md + ROADMAP Residual backlog | Close after review (recipe ported) |
| 562 | renewal phase-jitter grid vs #506 bound | STALE-CLOSE-no-port (depth-war lineage, mostly closed) | Close |
| 558 | torn chain.cbor → silent genesis fallback | PORTED-to-docs/thinking/2026-09-01-residual-defect-repro-recipes.md + ROADMAP Residual backlog (crash-safety) | Close after review (recipe + residual ported) |
| 536 | flow-6 verdict fingerprint rc=0 h=0 | STALE-CLOSE-no-port (cloudtest harness, depth-war era) | Close |
| 535 | h64 epoch-boundary liveness wedge | PORTED-to-docs/thinking/2026-09-01-residual-defect-repro-recipes.md (recipe); gates Boulder 1 R1.8 (#535 decision) | KEEP-open (gates R1.8) OR close-after-recipe-ported per owner |
| 530 | Docker field test stalls pre-genesis | PORTED-to-docs/thinking/2026-09-01-residual-defect-repro-recipes.md + ROADMAP Residual backlog | Close after review (recipe ported) |
| 504 | liveness-FAIL tears down nodes without journals | STALE-CLOSE-no-port (harness; build-immutable #7 lesson banked) | Close (verify the capture-first fix landed) |
| 441 | mature-regime PUBLISH starvation post-latch | STALE-CLOSE-no-port (depth-war era; verify vs current) | Close after verify |
| 437 | transport TLS/Noise (cert-strip residual) | PORTED-to-ROADMAP Residual backlog (security residual) | Close after review (residual ported) |
| 436 | equivocation drill minimal-A fidelity | STALE-CLOSE-no-port (test fidelity, depth-war era) | Close |
| 407 | docs reconciliation to D-CONSENSUS plan | STALE-CLOSE-no-port (SUPERSEDED — roadmap reorganized twice since; this migration supersedes it) | Close (superseded by this work) |
| 406 | consensus model-check (I1–I5) harness | ALREADY-IN-ROADMAP (Boulder 1 R1.5) | KEEP-open as evidence anchor OR close as duplicate per owner |
| 402 | h11 stall / sybil fork / C2 false-positive | DONE-already (fix merged + field-confirmed; R1.0 cites "#402 lesson") | Verify merge, then close |
| 399 | WS-checkpoint recovery drill (flow 10) | ALREADY-IN-ROADMAP (Verify tracks / R1 sequence) | KEEP-open (named verify drill) |
| 380 | Objective-mode Config.Quorum floor divergence | STALE-CLOSE-no-port (verify vs I1–I5 close) | Close after verify |
| 360 | WAN certification: one clean warm cloud run | ALREADY-IN-ROADMAP (R1 / #52 / Boulder 4) | Close as duplicate |
| 329 | consolidate WAN connections into one lib | STALE-CLOSE-no-port (refactor epic, immutable #5) | Close |
| 303 | introspective suite audit: 27 test-honesty items | PORTED-to-ROADMAP Residual backlog (test/harness debt) | Close after review (open QA debt ported) |
| 299 | bond proof reply ~1.5 MB / N² cost | PORTED-to-ROADMAP Residual backlog (measured numbers home); ROADMAP R3.3 keys re-derivation to it | KEEP-open (R3.3 references it) OR close-after-port per owner |
| 277 | repair-sweep dial-storm to dead holders | PORTED-to-docs/thinking/2026-09-01-residual-defect-repro-recipes.md + ROADMAP Residual backlog | Close after review (recipe + residual ported) |
| 264 | wire demand P2/P3 into daemon fetch | PORTED-to-ROADMAP Residual backlog (PoD demand wiring) | Close after review (residual ported) |
| 239 | field-test build-to-defects backlog (aggregator) | STALE-CLOSE-no-port (backlog-of-backlogs, superseded by ROADMAP) | Close |
| 237 | on-disk format migration policy | PORTED-to-ROADMAP Residual backlog (data-safety residual) | Close after review (residual ported) |
| 236 | fetch retry budget too shallow | STALE-CLOSE-no-port (fetch tuning) | Close |
| 235 | Theme B observability: silent sweep/-revoke/pre-format | PORTED-to-ROADMAP Residual backlog (silent-behavior observability) | Close after review (residual ported) |
| 233 | token double-spend + per-byte credit live seam | ALREADY-IN-ROADMAP (Boulder 2 economy-ON) | KEEP-open until Boulder 2, then close |
| 227 | churn repair-then-fetch DHT walk | STALE-CLOSE-no-port (refuted root cause) | Close |
| 207 | post-launch registry pruning + federation | PORTED-to-ROADMAP Residual backlog (post-launch, not V1) | Close after review (residual ported) |
| 183 | external red-team vs C1+C2 + §7 seams | ALREADY-IN-ROADMAP (Boulder 4 endgame; close condition MET, owner holds) | KEEP-open (evidence anchor; owner deliberately holds close) |
| 182 | shared-content sealing boundary (γ→1/N) | ALREADY-IN-ROADMAP (Research frontier / R4.1) | KEEP-open (research frontier, milestoned) |
| 180 | H9 pluralistic takedown / non-globality | ALREADY-IN-ROADMAP (Residual backlog H9; build track) | KEEP-open (milestoned build track) |
| 179 | H8 metadata-layer privacy | ALREADY-IN-ROADMAP (Residual backlog H8; build track) | KEEP-open (milestoned build track) |
| 115 | NAT traversal UPnP/NAT-PMP auto port-mapping | STALE-CLOSE-no-port (optimization-only, oldest) | Close |
| 94 | V1 — the forward tracks (epic) | ALREADY-IN-ROADMAP (the roadmap's own structure) | Close as duplicate |
| 52 | R1 verify gate — multi-machine field test (epic) | ALREADY-IN-ROADMAP (Boulder 4 / R1 field grade) | KEEP-open as R1 epic anchor OR close as duplicate per owner |

## Tally

- **PORTED (unique content migrated):** 11 — #558, #535, #530, #574, #586, #277 (recipes) +
  #437, #237, #235, #303, #264, #207, #299 (residual-backlog lines). *(Some issues appear in
  both the recipe doc and the backlog; counted once as PORTED.)*
- **ALREADY-IN-ROADMAP:** #651, #621, #616, #406, #399, #360, #233, #183, #182, #180, #179,
  #94, #52.
- **DONE-already (verify then close):** #597, #402.
- **STALE-CLOSE-no-port:** #562, #536, #504, #441, #436, #407, #380, #329, #239, #236, #227, #115.
- **KEEP-open (evidence anchor / live gate):** #183, #182, #180, #179, #52, #406, #399, #651,
  #621, #616, #535, #299, #233 (several overlap ALREADY-IN-ROADMAP — kept open because the
  ROADMAP points AT them or they gate a live Rock).

## Where ported content landed (files this migration created/edited)

- `ROADMAP.md` — SSOT note (top), "Residual backlog" section (#437, #237, #235, #303, #264,
  #299, #207, operational-floor Phase-5 content, and one-line pointers for the recipe issues).
- `docs/thinking/2026-09-01-residual-defect-repro-recipes.md` — the field-defect repro recipes
  (#558, #535, #530, #574, #586, #277).
- `archive/roadmap-history-2026-09-01.md` — the three retired ROADMAP organizing schemes
  (ordered-path preamble + Phases 1–6; Immediate-next-work; forward-tracks Build-tracks list).

## Caveats

- Close-recommendations are proposals. The owner confirms each against the issue itself before
  closing (WebFetch/gh summaries are leads, not rulings).
- #402 "DONE" rests on ROADMAP's "fix is the next build item" note plus R1.0's "#402 lesson";
  verify the actual merge before closing.
- #535 and #299 are marked KEEP-open despite being ported because a live Rock references them
  (R1.8's #535 recovery-boundary decision; R3.3's "#299 moves" trigger). Owner may prefer to
  close-after-port; flagged for the decision.
