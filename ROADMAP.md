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
> **critical path to the Release Candidate** and the **Boulder status ledger** below — is the SINGLE
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


### The Release Candidate — what it is, and the critical path to it (reordered 2026-09-08)

**The RC is ONE release: the era-4/v5 stamp-raising release, with the consensus format FROZEN,
the floor box a COLD AUDITOR (never-Accept, unconditional loud stall), and the economy DEFAULT-OFF
with every paid lane built (`D-RC-SCOPE-S1`).** The RC does not need the trustless recompute to be
sound; it needs the floor box to stall correctly — a far smaller property. The trustless-recompute
track is **FROZEN** (`D-RECOMPUTE-FREEZE`, 2026-09-08 — direction from the owner via the PE seat,
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/NOTE-to-silt-team-simplicity-and-roadmap-reorder-2026-09-08.md`).
The external B8 pass attacks the frozen artifact; `1.0.0` follows a green multi-machine field grade
of the same artifact and the two S6 scaling kills. Ratified sequencing that still governs: nothing
turns the economy on over a live mint (Boulder 0 is DONE); a graded field run is gated on the
model-check tier covering its regime — a field run confirms, it never discovers
(`docs/build-process.md`).

**The order of active work:** Lane A (consensus liveness) → Boulder 2 / Lane C (the economy — the
RC's substance) → Boulder 3 / Lane D (the freeze = the RC) → Boulder 4 / Lane E (B8 on the frozen
artifact + the M0 endgame) → Boulder 5 (the operational floor, post-RC) → Boulder 1 re-scoped ("the
cheap validator", post-RC, toward `1.0.0`).

**How to read the tables.** `Status` is trued to main `7f80c0e` (2026-09-08). `What is left` is the
one sentence a builder acts on without a design session; **OWNER** marks a sentence the owner
ratifies; a seat name marks the artifact that seat owes. Rows are ordered within a lane; a row
depends on the rows above it unless it says otherwise. The 2026-09-07 tables, the 23-item owner-call
block and the 125-row register live verbatim in
[`/archive/roadmap-reorder-2026-09-08.md`](archive/roadmap-reorder-2026-09-08.md); the per-Rock
argument before that in
[`/archive/roadmap-boulders-detail-2026-09-07.md`](archive/roadmap-boulders-detail-2026-09-07.md).

#### Lane A — consensus liveness (`R-H43-ROUND-LADDER-DESYNC`) · FIRST among active work

Core, live (block 43 took 17 min under f=1 down on `c450985-deep`), and it gates every graded field
run. Essential complexity — no re-scope.

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| A1 | The h43 round-ladder desync — the round clock armed on LOCAL mempool content (`rounds.go:306`), so the round number was a function of unreplicated private state | **FIXED and MERGED (PR #772), FIELD-CONFIRMED on `2633a11-deep`:** the arming rule (a replicated condition), the relayable round certificate with suffix-semantics round-changes, the designee proposing at the certificate's round, and the workless-designee closer (entries forwarded to the round's designee, cap 4) — `D-CONSENSUS-ARMING` + `D-H43-WORKLESS-DESIGNEE`. Published bound: ≤ f′+1 rounds after GST = 190 s at f = 1, N = 12. Field: 39 s/height on the deep drive (44 before); h30→h40 with a validator down at ~50 s/height. Certifications: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md` and `…/CONSENSUS-LIVENESS-h43-COMPOSED-DIFF-c2a476a-RESEARCH-CERTIFICATION-2026-09-07.md`; the #441 mature-regime PUBLISH starvation shares this root and is closed by the same fix | nothing but A3: the graded run at the re-priced tiers closes the register row | Tester |
| A2 | `#380` objective-mode `Config.Quorum` floor divergence — `RequiredQuorum()` returned the LOCAL `cfg.Quorum` verbatim in the mature regime (I1), and `SupportMeetsQuorum` gates `newViewFor`, so a divergent floor was also a permanently dead designee (liveness) | **DONE — MERGED 2026-09-08 (PR #779, `fa854d7`; the merge RATIFIED the amended `D-CONSENSUS-ARMING` (20) sentence, G-380-B):** direction (1) as SPECIFIED and CERTIFIED by the one certification for the change (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/CONSENSUS-380-quorum-floor-direction-1-PREDICATE-AND-CERTIFICATION-2026-09-08.md`, §1 the predicate per regime, §6 the composed diff CERTIFIED): objective + Byzantine sizing ⇒ `bftThreshold(validatorSetSize())` in the young window and **0** in the mature epoch (the >⅔ frozen-weight rule is the whole bar; head-counting the epoch set stays REFUTED per B2); the trusted opt-out (`-byzantine-quorum=false`) and legacy keep `cfg.Quorum`; `Quorum` survives as the proposer-side gather target on EVERY proposal path (`gatherTwoPhase`, the one choke point all four paths share, raises to `max(caller floor, ConfigQuorum(), RequiredQuorum())`; the PE found the new-view and bond-drain paths had passed 0 and would have gathered less than before, and the Researcher's G-380-A found the FORCED new-view leg bypassing the first fix — a uniform swarm's blocks now carry the same count as before, so the change adds accepts only; rollout note in the CHANGELOG: upgrade validators first). Three merge conditions closed in the same PR: Reload counts the same predicate (M-380-1), `newViewFor` refuses a certificate with no round-change from anyone but the designee (M-380-2, wire-reachable via the attester path; the PE caught the designee-only walk-around of the first `len == 0` guard), `RequiredQuorum` pinned by G-D13 through a helper row (M-380-3). Gate G-H43-8 = five arms RED-first (8a the #338 fixture inverted under Byzantine sizing; 8b the higher-floor designee assembles the certificate; 8c mature floor 0 accepts with the weight bar still refusing by name; 8d Reload symmetry; 8e the gather-target control) + the regime-(c) stranding control kept; three ablations RED on the named mechanism; the v5 mirror exact with the mature regime added to the parity oracle | nothing — merged with its five G-H43-8 arms, the regime-(c) control and the three ablations; **OWNER (batched, owed):** the `-quorum 3` default (a four-validator swarm on defaults tolerates f = 0; the field topology sets 2 — remedy is a derived default on the untrusted objective path, the `effectiveByzantineQuorum` pattern) and whether a single-anchor objective launch stays supported (at A = 1 no count gate holds anything; the PE recommends requiring ≥ 2) | Tester · Builder · Researcher · blind PE (all done) |
| A3 | The next graded GCP deep run | **DONE 2026-09-09 — `97e3101-deep`: 31 pass / 1 gap / 0 fail / 3 skip** (`integration/cloudtest/report-97e3101-deep.md`). The first fleet carrying #380 direction (1), the v2 flat-delivery retirement and the R2.7 detectors. **`6-fault-tolerance` PASSES within the computed 190 s down-designee escape bound** — the re-priced tier met in the field on #774's head-reading wait, where the previous run GAPped on the harness artifact; gaps 2 → 1, passes 30 → 31. The derived quorum floor changed nothing a uniform swarm commits: the whole consensus sheet passes (stall and capture drills, partition heal, equivocation island, forged-block refusal, WS cold-sync). Deep drive h93 → h130 at ~45 s/height; prune engaged and all validators converged on the pruned chain. The S7 repair economy closed on the wire with the flat path GONE, and the dark delivery lane still refuses at the withdrawal. No OOM, no crash-loop; teardown verified (40 destroyed, nothing left running) | the one gap is the known `184-low-bond` premise (#350) — the adversary holds a QUALIFYING bond and is correctly accepted, so the under-bond rejection property needs a dedicated sub-min-bond identity in the harness; the property is certified in-process. E5, the RC field grade, runs against the FROZEN artifact after D3 | Tester; owner's go (billable) |

The null proposal (PBFT's null request, the literature's closer for a workless leader) is held in
tension in `docs/design/m0.md` §10.1: it was routed to "era 5" on 2026-09-07, and under simplicity rule 6
no era is pre-approved — it waits for a ratified reason of its own.

#### Lane C — Boulder 2: the economy · the RC's SUBSTANCE (promoted 2026-09-08)

The economy is the M1 keystone and is independent of the floor-box recompute. Every paid lane is
built and dark; the RC ships it default-OFF (`D-RC-SCOPE-S1`) and the B8 pass attacks it ON in the
harness. The cross-server double-redeem money pump is CLOSED on main by R0.4b (`2ad9bd5`, per-epoch
issuer-key expiry; open-break gate PR #700 GREEN) — the per-serial delivery guard `fcbab7e` the PE
note cites is an unmerged alternative on a worktree branch, not owed.

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| C1 | R2.9 tail — retire the `core/demand` v2 flat primitive and the ledger's flat delivery leg, leaving the anchored session lane (open → settle) as the ONLY paid-delivery path; the durable paid-serial guard record gains its lane byte | **BUILT 2026-09-08** (`builder/c1-demand-v2-retirement` @ `a290bff`, PR pending): production shrinks ~580 lines (`Bank.Redeem` + the v2 spent set + `DeliveryReceipt`/`SubmittedReceipt`; `RedeemDeliveryCredit[Reason]`); 22 test files re-homed onto the lane, each property with a driven ablation, every red-team scar (cross-server double redeem, the A4 money pump, the self-financing eviction pump, the epoch binding) re-derived RED at its historical broken value; the guard record bumps to format v2 (magic + version header, lane byte last) and REFUSES TO START on a pre-bump file rather than guessing the lane — the guess is the mis-count being closed, and it would persist into the file at the next compaction. The blind PE (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-c1-demand-v2-and-flat-leg-retirement-a290bff-2026-09-08.md`) ruled the retirement clean (no property lost; every scar re-derived RED at its original broken number; the supersede rewrite holds its property) and found the format bump's one real defect: the store type has TWO consumers, and the refusal told the operator that removing the file costs nothing — true for the paid-serial log (the ledger is ephemeral pre-RC), FALSE for the credit-spent log, whose publish key persists, so clearing it alone re-opens every held credit for a second spend against ratified owner call 6. Closed at `ee81518`: the adapter's refusal names NO remedy and the daemon attaches the per-store one (rotate the key AND clear the log, together, for the credit-spent store), driven by a runtime arm, a source arm and a controlled swap; an empty pre-bump file upgrades silently, a written one refuses, and the refusal leaves the file byte-for-byte intact so the ratified remedy stays available | PE re-check of the delta, then merge; the design record's six drifted lines are corrected in the same PR | Builder (done); blind PE |
| C2 | The delivery idle window + the wall-clock reaper (`R-REAPER-FORFEIT`) | the five R2.9 owner calls DECIDED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (4)–(8)): REFUSE-UNTIL-SET stays until the ≤ f+1 bound is field-confirmed at the re-priced tiers; the disclosed v1 residuals (the wall-clock step, the anchor stall, the operator-managed `creditspent` ceiling with its rotate-and-clear rule, the relay-wash position) are DISCLOSED in `docs/design/m0.md` §10 (folded 2026-09-08 — the doc PR this row owed is done) | after A3's green run: the idle-window default is a one-line Builder PR sized against the STALL case, not the steady state (Tester sizes it against Lane A's bound — `T_b` = 44 s/height is the input, not the answer); the relay lane's settle-inside-one-epoch measurement is owed before `-accept-relay-payments` is enabled | Tester → Builder |
| C3 | R2.2 full observability set (serve-work AND repair-work Gini, per-tier margin, live `g`, funded-horizon, wash detection; network panels via the DHT crowd-estimator with knowability tiers) | **BUILT 2026-09-09** (`builder/c3-r22-observability` @ `f03ab50`): 15 of the 17 rows built, 1 already on main (row 3, the prepay/skim split, shipped as C4's A4-1), 1 ruled NOT KNOWABLE (row 5, below). Four read-only routes in `apiRoutes()` (`/api/economy/flows`, `/g`, `/concentration`, `/network`), the four panels, a bounded flow ring, live `g` per cared object with `known`/`reason`, serve-work Gini over the whole sample and repair-work Gini WITHIN the repair-capable subset, the C2 metric beside them, and the observed tier ratio. Two gossip fields (`servedBytes`, `repairsDone`) ride the EXISTING capacity pledge under the same condition, so the bound is the existing peer-info eviction — two int64s per existing entry, NO new peer-keyed map — and the work sample IS the capacity sample, which is what lets row 10 derive the tier class instead of gossiping a third field. Bands published with their source: pony < 16 GiB, horse < 1 TiB, archival ≥ 1 TiB. `minGossipSample = 3` is derived, not chosen: at n = 1 a Gini is 0 by construction and at n = 2 it inverts to the ratio of two named peers' counters, so the floor is an honesty floor and a privacy floor at once. The advisory's vacuity correction is MEASURED, not quoted: network-wide repair Gini 0.9091 against 0.0000 within the capable subset on the same healthy shape. 13 ablations RED — including one the Builder caught vacuous in its own gate (it compared encoded frame LENGTHS, which grow equally under the revert, so it stayed green; replaced with an exact key scan) | **Row 5 (network aggregate `g`) is DECIDED BY THE DESIGN, not owed as an owner call:** `g` is cost-per-repair, rows 8–9 carry the denominator (repairs-done) and nothing carries the numerator (credits paid out), and row 10 forbids a third gossip field — so the figure is NOT KNOWABLE from the surface this row may build, and §0's own rule ('never publish as fact') plus row 13's ('field absent is a legitimate rendering; a guess is not') settle it. It ships as an explicit not-knowable carrying its derivation on the wire, never a `network` block. Re-opening it means a design case for a third gossip field, which is a new scoped item, not an escalation. **Both blind seats independently CONFIRMED a privacy break, and the gossip half is GATED at the research gate — OWNER RATIFICATION REQUIRED before it merges.** The break (blind PE `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-c3-r22-observability-f03ab50-2026-09-09.md`, red-team `/Users/andrewedmond/Claude/claude/silt-reviews/red-team/REDTEAM-c3-gossip-disclosure-f03ab50-2026-09-09.md`): identity is free, so an attacker furnishes n−1 of the n sample terms and the published Gini plus its sample size is ONE EQUATION IN ONE UNKNOWN — a working PoC recovered a planted 7,777,777 exactly, and the recovered term is the counter `-privacy=on` (the compiled default) withholds from that reader. The sample-size floor cannot fix it: it bounds sample SIZE, not terms unknown to the reader, and the attacker sets the size. It amplifies to a PEER ORACLE (the recovered term need not be self), a per-node activity TIMELINE (the counters are monotone, so resampling yields a rate), and a repairer FINGERPRINT for eclipse targeting. Certification `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C3-GOSSIP-DISCLOSURE-vs-D-UI-PRIVACY-FLAG-RESEARCH-CERTIFICATION-2026-09-09.md`: **GATED**, certifiable only as alternative A (gossip the two fields ONLY under `-privacy=off`) under three conditions — a wire gate ablated with an exact CBOR key scan (never a frame-length compare), non-reporting read as UNKNOWN and never as zero (both fields are `omitempty`, so withheld and idle are identical bytes and today's zero drives the serve Gini toward 1.0, a FALSE total-capture reading on the default posture), and self's own withheld counters kept off the open route. The ratified `D-UI-PRIVACY-FLAG` containment survives on the letter and is voided in substance: the gossiped value is the SAME INTEGER as the withheld one, `D-STATUS-SNAPSHOT-INTERVAL` ratified the 5 s snapshot as a security parameter precisely because a reader picks its own poll rate, and the audience widens to nodes with no HTTP surface at all — the exact node the flag was ratified to protect. A coarsened band or rate is REFUTED (a band still yields a monotone step sequence under probing; a rate publishes the derivative the attack must work for). Under alternative A the panels are NOT empty: the C2 metric, the tier mix, the bands and the target ratio survive; only the two Ginis become a named absence. The owner ruled the product half in advance — privacy default WINS, empty panels preferred — and the exact sentence to ratify is §7 of the certification. **The concentration gate is now a THREE-VALUED statistic, and two of the advisory's own thresholds are WITHDRAWN by the Economist that wrote them** (`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-c3-concentration-gate-thresholds-redderived-2026-09-09.md`): the Tester's fixtures showed a bare `serveGini ≤ 0.15` PASSES total capture, because the certified privacy exclusion drops silent peers from the series — five nodes serving 100 % with 1006 silent publishes `0.0000, known:true`. The fix is one adjusted statistic `G_adj = (1−c) + c·G_pub`, EXACT on that fixture (0.9951 = the true Gini), with the coverage floors derived as a theorem of the tolerance (`c ≥ 1−T`, searched over 101,000 pairs) rather than chosen; a separate coverage clause would still admit a true Gini of 0.2775 serve / 0.6400 repair. The verdict is PASS / INDETERMINATE / CONCENTRATED, because reporting "I cannot see" as "you are captured" is its own defect. `repairGini ≤ 0.40` is WITHDRAWN as a field threshold and encoded as a live pin instead: under holdings-proportional repair the honest null is 0.7424 at 11 capable nodes and 0.6282 at the canary's own minimum, so **an honest canary would abort** — and 0.7424 is indistinguishable from the 0.7505 the advisory had labelled CAPTURE. `serveGini ≤ 0.15` is demoted to a fixture constant: its provenance came from a 10,101-node distribution while the sample is capped at 4,096, and the falling curve is not a sample-size effect at all — remove the single archival node and the honest figure is scale-invariant at 0.0810 across two orders of magnitude, so the alarm wants to be MIX-aware. Two clauses are withdrawn as unbuildable: `ponyShareOfServedBytes` has no source (the build item is a per-tier work total; the trap is that the tier mix's `share` is a share of NODE COUNT and reads 0.9891 where the true byte share is 0.1998, so wiring it to the T-AR floor passes total capture — pinned), and `caretakerSetSize ≥ 3` has no published surface and no definition of a cold root. Left: the Tester lifting the red-team PoC as a permanent regression gate — DONE, the recovery solve is now VALUE-shaped (it walks every numeric leaf, after the blind PE republished the same figure under a different name and recovered the secret while the suite stayed green) — then **the owner ratifies §7 of the certification and it merges** | Builder (done) → blind PE + red-team → Tester |
| C4 | R2.7 blocking telemetry — the A2 (supersede-suppression) and A4 (money-pump) detectors | **BUILT 2026-09-08** (`builder/c4-r27-telemetry` @ `bc7050a`, PR pending): three detector families, each with a driven ablation — A2 the served-byte partition across terminal states with the unwitnessed residual DERIVED, never stored (one write path per state; the coverage denominator excludes chunks that can never be witnessed, so a node serving manifests is not penalised); A4 the three-part replacement (the literal `bountyPaidToEscrowFunder` is DEGENERATE — every escrow funder on a per-node ledger is that node), namely the prepay/skim split of `funded`, `BountyPaidToSelf` keyed on the claim holder, and the prior-fetcher payment flag; the affordability floor at BOTH spend gates with distinct identities counted by one bool on the account, never a side set, surfaced beside `grantsDenied` on `/api/status` including the no-faucet branch. Two calls beyond the advisory, recorded in `docs/thinking/2026-09-08-c4-r27-blocking-telemetry.md`: the withheld document rebuilds its revenue block with the two new fields zeroed (the allow-list passes the block through by pointer, so a field added inside it would have shipped open — an F2 join with `/api/roots` on a one-root node), and the floor counts on the unconfigured-faucet branch too (the literal placement would have dropped a real refusal count on the DEFAULT posture — silent loss). C1 removed one terminal state the advisory assumed, so the identity has three terms, not four; a fourth counter would be a permanent zero | both reviews IN: blind PE MERGEABLE-WITH-CHANGES (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-c4-r27-blocking-telemetry-93deb56-2026-09-08.md`) — the privacy fix was the one thing in the diff with NO gate (reverting it left the whole `cmd/silt` suite green: the existing wire scan compares VALUES and is blind to any field its fixture leaves at zero), and the floor was counted at 2 of 3 spend gates; the Economist found the same third gate independently and ruled the set SUFFICIENT WITH GAPS NAMED (`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-c4-r27-telemetry-as-built-93deb56-2026-09-08.md`). All four items closed at `bc7050a`: a sibling whole-surface scan whose fixture pays a bounty (the existing scan is NOT re-pointed — at `paid == 0` three per-object figures are aliases of `funded`, so driving a bounty through it would trade three-quarters of its coverage for this one field); the third gate counted at the refusal DECISION in `registry.Gated.Publish`, never inside the `CanPublish` predicate, which is read for display (the rejected design is itself an ablation arm); and the delivery-lane state published beside the coverage figure, so a machine-read abort can tell an honest RC-default node from a suppressing one. Delta re-check, then merge | Economist specified → Builder (done) → blind PE |
| C5 | Pre-flip closers — the two CODE closers are BUILT; the bearer-anchor red-team pass is what is left. (The two residual names left the register with their closure, per the filing rule.) | the family CERTIFIED 2026-09-07 (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md`): the λ-dust bound CLOSED (24.00 GiB escrow leg, exact); parity amplification DISCHARGED (the per-server floor corrects DOWN to 27.94 GiB, so the 64 GiB `grant/r` pin gains margin 1.43× → 2.29×); the refuse-and-self-spend and relay-anonymity faces are held in tension (`m0.md` §10); the price-level and relay-wash positions DECIDED 2026-09-07 (owner call 13) and carried INTO the flip — re-confirmed at R2.4 only if C7 contradicts them | **BUILT 2026-09-09** (`builder/c5-preflip-closers` @ `a68701c`, PR pending; blind PE MERGEABLE-WITH-THREE-CHANGES at `399f842`, all three taken — `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-c5-preflip-closers-399f842-2026-09-09.md`). The bounty division now happens AFTER the rarest-shard multiplier at the one path that pays, the second pricing function is deleted rather than left beside it, and the publish warning prices the OBJECT rather than the flag — which made the rule smaller, deleting the flag-was-set parameter, since an unset chunk size IS the default and can only land silent. A single-frame object's parity is computed at its TRUE length. **The finding neither source named:** the PoR auditor sizes its challenge from the committed chunk size and demands it EXACTLY of every leaf (deliberately — that exactness IS red-team finding F4), so a short shard under an unchanged committed size makes the auditor refuse every HONEST holder and every sub-frame object silently read as LOST; closed by carrying the frame size actually used in the EXISTING committed field, no format change and F4 intact. **OWNER: the genesis moves a second time and the ROOT moves with it** (4′ re-framed only the manifest, which the root does not cover) — root, manifest chunk and block hash all re-pinned; the first break was accepted explicitly and this needs the same. The blind PE re-ruled twice more; both further findings were the SAME shape and both times the CODE was right — a published sentence overreaching the code's actual boundary while every gate stayed green (the re-addressing boundary was stated one byte wide, and the warning's silence rule claimed a default publish can never warn when in fact every object ≤ 262,119 B does — the concern is TRADED, not satisfied). Both corrected and re-measured independently. A real bug fell out of the second: an EMPTY file printed a 24-byte shard that does not exist, and the gate had EXCUSED size 0 rather than covering it — the row is now driven. Left: **the OWNER accepts the second genesis move** (the root moves this time), then merge | Builder / red-team |
| C6 | R2.4 the economy-ON default flip — phased: correctness gates → economy-OFF baselines over ≥ 3 epochs → canary (window = 1 epoch ≈ 5.9 min at the measured `T_b`, hysteresis 3 windows; HARD aborts: any `guardFullRefusals`, caretakers < 3, `BountyPaidToSelf` > 0, any rise in `spendRefusersDistinct`; soft aborts: coverage < 0.75, pony serve-share < 0.50 or −20 pp, `serveGini` > baseline + 0.10, `repairGini` > 0.40, funded horizon < 600 s, evicted/object-aware > 0.01) → default | DEFINED as a CHECKLIST 2026-09-07 (the advisory, §3) **⚠ TWO OF THE CANARY'S ABORTS DO NOT WORK AS WRITTEN (Economist, 2026-09-09, `/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-c3-concentration-gate-thresholds-redderived-2026-09-09.md`).** (i) `repairGini > 0.40` is WITHDRAWN as a field threshold: under holdings-proportional repair the honest null is 0.7424 at 11 capable nodes and 0.6282 at the canary's own minimum topology, so the abort kills the FIRST HONEST CANARY — and 0.7424 is indistinguishable from the 0.7505 the advisory had labelled CAPTURE, because its "midpoint" spanned two healthy shapes rather than a healthy and a captured one. `serveGini > baseline + 0.10` is demoted the same way. (ii) **On the SHIPPED DEFAULT no node gossips work counters at all** (`cmd/silt/daemon.go` welds the publish flag to the privacy default, which is withheld), so under the certified alternative A both concentration series are EMPTY and every concentration-based abort is inoperative — silt can see who is PRESENT and never who does the WORK. The owner chose empty panels over the disclosure knowingly; the consequence for this row is that a canary graded on a default fleet has no decentralization signal, and every concentration baseline to date is measured on an opted-out topology. **DECIDED 2026-09-09 (`D-WORK-VISIBILITY`): decentralization is graded in the HARNESS for the RC — no production concentration alarm is claimed, because none can fire on a default fleet. This row's concentration-based aborts are RE-POINTED at the harness and must stop implying a production abort that cannot fire; the withdrawn `repairGini` threshold is not reinstated. The committed-ledger route (concentration over committed state rather than gossip — the strongest answer, since it removes the disclosure AND the self-reporting) is DEFERRED to as late as possible before the cut: it is research territory that may spin, and defined work comes first** | after C1–C5; the flip is the FIRST of the three `D-FP2-SCOPE` re-arm triggers (the ephemeral-ledger items re-open with it); OUTSIDE the RC by `D-RC-SCOPE-S1` — a `0.9.x` release after C7's clean verdict, before `1.0.0` | Builder; Economist |
| C7 | R2.7 economy-ON adversarial-solvency verdict (five inequalities × seven attacks; A2 supersede-suppression and A5 cold-start capture unpriced) | SCOPED; after C3 + C4 + C6. **Scoped down 2026-09-08 by the Economist's as-built read of C4** (`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-c4-r27-telemetry-as-built-93deb56-2026-09-08.md`): the five solvency inequalities ARE gradable on C4's counters (A4-1's prepay/skim split is the two-sided one that grades S5, bounding recoverable money at the skim leg); **A4-2/A4-3 are FLOOR detectors defeated by one extra keypair costing zero credits** — no counter without an identity axis closes that, and the axis is the access record Don't #3 forbids, so R2.7 must state that limit rather than grade past it; **A5 (cold-start capture) is half-instrumented** — it needs a cold-vs-exhausted split on the refusal (one more bool); **A6 (the edge legitimately refuses cold content) has NO instrument in any shipped row** and must be SCOPED OUT of the verdict until C3 lands rather than graded on nothing. Coverage is a solvency estimand, never an attribution instrument — churn and suppression read identically because economically they ARE identical — so it is read as a band with the abort on the upper bound | Researcher NEW-CERT + red-team pass; feeds the R4.4 brief | Researcher; red-team |
| C8 | R2.8 cold-repair funding path (reserve-aware scheduling + early cliff disclosure to ANY funder + R2.9's expiring remainder; no network pool, never a mint) | DEFINED 2026-09-03; after C3 | one builder session; five inputs stay ASSUMPTION until live data | Builder |
| C9 | Measurements owed: `B_bootstrap` + the honest arrival rate on REAL traffic (the flixz export; the commissionable handoff paragraph is the 2026-09-07 certification's §7 — cumulative per-server draw, one-sided falsifier: only a measurement ABOVE 64 GiB is decisive) · `M_seen` pony-class (the class-M streaming verifier's TIME ceiling) · G-λ-11 the receipt-lane operator cost before `PF` may move | OPEN | the flixz handoff (T-1 first, G-BB-5 answered first); two Tester measurements | Tester / external |
| C10 | Small owed PRs: `R-COMPACT-ORPHAN` WARN line (surface `CompactFailures` / `LastCompactError` on the banked/status path; WIP `builder/c10-compact-orphan-warn` @ `49eb5d1` — the WARN line with both ablations RED, suites not run) · `R-PRIVACY-OPERATOR-TAB-TOKEN` (a persistent operator-token route before the `-privacy` default reaches real operators) · R2.14b `MsgRelayFund` (top-up with fresh anchors on an admitted session) | OPEN, LOW | one small PR each | Builder |

#### Lane D — Boulder 3: the freeze (era-4/v5) = the RC · its dependency on the recompute spine is CUT

The only floor-box property the freeze needs is "it stalls and never accepts" — one driven suite,
not the R1.x ladder. The v5 digest leaves freeze at three and the format is not touched again.

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| D0 | **The cold auditor — the RC's ONLY floor-box requirement** (owner call 2, RATIFIED 2026-09-07, `D-TRUE-UP-CALLS-2026-09-07` (2)): the floor box is a COLD AUDITOR — unconditional loud stall, never Accept | **BUILT 2026-09-09** (`builder/d0-cold-auditor` @ `8c414e7`, PR pending): the box is a cold auditor — the recovery directive type, its heights list and live-follower opt-in, the box config knob, the `trustFloor` scalar on the contract surface and a loose boundary predicate are all DELETED, and the second exported box entry (`WitnessValidateV5`, zero non-test callers) is deleted too — a coordinator scope call OFF the ratified owner-call-2 list, taken because keeping it ships TWO entries with TWO recovery semantics on the RC's last floor-box item. In their place: ONE DRIVEN never-Accept suite over nine block classes — for each, build a block the NODE's own commit path ACCEPTS, forge a divergent `StateRoot`, re-sign, re-certify with the full quorum, assert the box refuses with a NAMED reason — plus a reflection completeness meta-test that reddens if a `Block` field is added and driven by nothing | blind PE MERGEABLE at `72be1f3` (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-d0-cold-auditor-3539b2a-2026-09-09.md`); its one blocker was that the "unconditional" stall keyed on the block's DECLARED height, so a box at head 2 emitted a TERMINAL re-anchor stall for a block merely CLAIMING the boundary — and the suite could not see it because the fixture collapsed the two quantities. **The coverage hole was inside the completeness test's own excuse row**, whose written reason was measurably false. Closed, both directions independently RED. Three non-blocking findings taken rather than shipped past, all one class — a comment claiming more than the code delivers: the cited-test lint scanned COMMENTS but not STRING LITERALS, so every excuse row and every observable-contract Asserter entry in the repo was unprotected (fixed in the LINT, +91 citations now covered, zero phantoms); the state-view scalar rule was a pattern that was escaped once per form, now a PARTITION with a closed complement (allow-list with an asserted reason, or a result list ending in `Availability`); and a dead conjunct in the boundary predicate. **A FOURTH false claim was found by running the correction rather than reading it** — dropping `LivenessRecoveryHeight != 0` would have made every box stall at genesis forever. Left: merge | Builder; blind PE; Tester (every gate RED first) |
| D1 | **The era-4/v5 freeze manifest** — the ONE ordered list of what the stamp-raising release contains or decides | CERTIFIED 2026-09-07 as a 22-item manifest in four classes with three deadlines (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md`); the nine §8 sentences **RATIFIED 2026-09-07** (owner call 9): `tagRevLogSize` BOUGHT (a safety leaf — a wrong `m` is a WRONG-ACCEPT; it also lifts the takedown stall `R-BOX-STALLS-ON-TAKEDOWN`); (d-3) `AnswerDigest` BOUGHT (closes `R-LATE-REVEAL` and the `Slashes` cap's completeness face); the PoP slot RESERVED inert (`R-ISSUERKEY-POP`); the `IssuerKeys` cap 4,096 with its packing budget + `(issuer, epoch)` distinctness; `SerialSize` 32; #237 refuse-to-start across a format boundary; the genesis hash is network identity; the tally stays behind `everMature`; no activation mechanism beyond the tally. FORMAT items freeze at the freeze; VALIDITY items land at the stamp raise. **Deltas of the 2026-09-08 reorder (the Researcher is told in one doc line; the unchanged items are not re-certified):** (i) the digest set freezes at THREE leaves — `R-membership` (D-V5-WHOLESET-ROOTS five → three, a hard fork at activation, free while era-4 is dark, with the `objective()` guard that also covers `MinBond`) is the LAST format touch; (ii) the box witness/frame byte ceiling LEAVES the manifest — it served the recompute and goes to the re-scoped Boulder 1's design consult; (iii) the `proof.Unmarshal` decoder bound (`R-R3-GOB-ALLOC-AMPLIFICATION`) moves its deadline from "the flip" to the stamp-raise train, because the flip is frozen | Builder lands the FORMAT items in one stamp-raising PR train against the manifest's gates; the B8 green list is the manifest's §6 | Researcher (done); Builder |
| D2 | Stamp-raise test deliverables (same release): `R-CARRIER-MODELCHECK` (the seating AGREEMENT property) · `R-CARRIER-PRUNED-HASH` (prove the seen-fold never depends on a pruned body; G-D11 two-sided shipped in Round 1A, bounded not eliminated — the descendant catch is the only defence, the proof is owed) · `R-E2E-ERA4-FIXTURE` (objective + bonded + epoch-enabled; the e2e cost ACCEPTED, owner call 14) · `R-CLOUD-ERA-PROBE` (the block era on a CLI/status surface so cloud row 13b can tell "era-4 dark" from "keys off-commitment"; absorbs the rollout-signal item, whose code half was vacuous) | certified as manifest items 14–19 | Tester + Builder inside the stamp-raising PR train; #237 = REFUSE TO START across a format boundary | Tester |
| D3 | R3.4 the freeze decision itself — the owner freezes the format at the RC on D1's manifest, and the readiness stamp goes 3 → 5 (no release ever stamps 4) | ratified as a principle 2026-09-03; the nine manifest sentences RATIFIED 2026-09-07; the ACT is owed | **OWNER** freezes at the RC after D0 + D1 + D2 (`#558` is DONE and merged; the owner's ratify-or-narrow on its refusal surface is owed) — NO LONGER after the recompute spine — and commissions the external seat then (owner call 12) | owner |

#### Lane E — Boulder 4: B8 external red team on the frozen artifact + the M0 endgame

Immediately after the freeze. The flagship claim ("M0 held") is independent of the recompute. The
M0 status stays *"built + internally-clean"* until the external pass returns.

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| E1 | R4.3b-pre-on — the eight certified preconditions for `-dht-address-cap=on` (de-herd + PE O-1/O-2; the pony's own direct dial (v) with G-14; `cap_relay ∈ [2, K−R]`, `R ≥ K/2`; the v6 width flag with the `/32` floor; the exempt gauge + warning; PE O-3; one ≥3-relay cloudtest shadow run + one at `NAT_MODE=symmetric`; gates G-14…G-19 + the red-team's six) | DEFINED 2026-09-04 (certified); shadow mode MERGED (#725) | one builder session for (1)–(6) + (8); the shadow run rides A3; then **OWNER** ratifies R = 4, `cap_relay` = 4 with the printed floor and flips `on` (owner call 10, after the run reports series A < 5 % and series B < 20 %) | Builder + Tester → owner |
| E2 | R4.2 the A-axis — measure / publish / hand to B8 as-is; A3 NOT wired | measure/publish SHIPPED (#715); the re-scope RATIFIED 2026-09-07 (owner call 11) | put the bonded-adversary domain-collision finding in the R4.4 brief (one doc edit) | Builder (doc) |
| E3 | R4.3 the continuous internal red-team hunt | standing; named targets: the O-2 "pruned genesis" probe (`R-CARRIER-GENESIS-DISPOSAL` — a "pruned" genesis served to a fresh-sync victim with our hash and an attacker-chosen body, carrying the PE's composition: attacker-declared `BondRegs` qualifying its own keys under a kept `Pruned`; if it lands, the Tester encodes it and the Researcher certifies the refusal) · the bearer-anchor transfer surface (C5) · class-P compound ordering; bondreg full path; DHT/eclipse; long-range/WS checkpoint; relay/PayWord; churn/restart `everMature` | runs opportunistically between builds; every confirmed break → Tester gate | red-team |
| E4 | **R4.4 = R1.7 — the external B8 pass** against the FROZEN, never-Accept artifact: C1 (no discount), C2 (no quiet capture), the seven `m0.md` §7 seams, plus the brief items (the `SlashesBytesCap` second face; the bonded-adversary domain collision; the R2.7 verdict) | the M0 close gate; brief `docs/reviews/m0-redteam-brief-2026-08.md` | after D3; **OWNER** commissions the external seat (owner call 12); findings triaged to the build-immutable bar and re-attacked until clean | external |
| E5 | The RC field grade — a green multi-machine GCP deep run of the frozen artifact (the standing release gate) | last graded: `2633a11-deep` 30 pass / 2 gap / 0 fail / 3 skip | after A3 and D3 | Tester |
| E6 | `1.0.0` | — | after E4 + E5 and the two S6 scaling kills (Boulder 5's `1.0.0` gates, scope call S2). The accept-flip (R1.8) is NO LONGER a `1.0.0` gate: it lands only after the re-scoped Boulder 1's simpler design proves out, and only if it is still needed | owner |

#### Boulder 5 — the operational floor · POST-RC, AHEAD of the recompute (scope call S2, 2026-09-07)

"A node a person can run" matters more to adoption and to build-immutable #8 than a pony that
trustlessly recomputes state roots.

- **Post-RC (`1.x`):** per-platform service packaging (launchd / systemd / Windows service) + signed
  installers; operator-consented self-update per R4 (signed manifests, never silent). `#437` transport
  authentication (TLS/Noise) is post-RC by scope call S4, research-gated on the crypto choice.
- **`1.0.0` gates (on the E6 path, not the RC):** the two S6 scaling kills — incremental O(delta)
  proof maturation (kill the O(store) restart scan) and reprovide dirty-tracking (kill the O(held)
  per-interval re-sign) — because they price out the honest operator (build-immutables #4/#8).
- Exit gate: a non-developer installs a node that survives reboot and returns to serving in
  seconds, and steady-state cost no longer scales with the whole held set.

#### Boulder 1 — RE-SCOPED and MOVED post-RC, after Boulder 5, toward `1.0.0`: "the cheap validator"

- **FROZEN 2026-09-08 (`D-RECOMPUTE-FREEZE`):** the trustless changed-path recompute track. No new
  floor-box classes, no Round 1B, no more R1.x rungs, no structure rounds on the keystone, no era whose
  reason is "the recompute needs it". The Round 1B closers, the resident-witness heap, the witness byte
  ceiling, the `provenView` faithfulness hunt and the fold-pin composite-literal gap are frozen with
  it; their names and last state are in the 2026-09-08 archive file. Every gate that exists stays
  GREEN and is not weakened — the era-4 maintenance oracle (`core/chain/modelcheck_era4_maintenance_test.go`)
  in particular, which holds `len(qualified)` as a safety quantity. The state-root commitment (the
  three v5 digest leaves) stays: cheap, standard, already built.
- **What replaces it — a short design consult, not a build.** Re-derive the pony's validation model
  from `docs/TENETS.md` + `docs/VISION.md` with B8 as the hard constraint, naming the settled corner
  it buys: (a) bound the state — prune + shard so a pony holds a bounded slice (Bitcoin pruned nodes;
  D-TIERING §7); (b) optimistic + fraud proofs — ponies accept, any honest full node challenges within
  a window (1-of-N honest watcher); (c) tiered validation — ponies serve / store / audit-and-stall and
  keep bond WEIGHT while delegating the validation DUTY to horses + archival, which fully validate
  (light clients; owner call 2). Expected: (a)+(c). PE + crypto-specialist + researcher advise; the
  Researcher certifies; the **OWNER** ratifies.
- **R1.8 the accept-flip** lands only after the simpler design proves out, and only if it is still
  needed (a consensus-rule change, I1: Researcher re-certifies, owner ratifies).
- **Done history** (R1.0–R1.6, the `LastCommit` carrier, O3 Direction T, the ephemeral-ledger scope close (`D-FP2-SCOPE`), the
  I5 pruned-slash forgery, `#558`, Structure Round 1A — PRs #701–#732, #772–#776): the archive file
  and `docs/thinking/2026-09-01-floorbox-witness-soundness-fix-design.md`.

**Owner calls.** At most five per true-up (simplicity rule 5). Owed today: **10** (R4.3b `on`, after
the shadow run), **12** (commission the external B8 seat, at D3), the `#558` refusal-surface
ratify-or-narrow, the `-quorum` derived default and the single-anchor launch posture (both A2) (the rule refuses on ANY structural-verification failure, a larger surface than
scope call S3's "torn tail" — kept per the PE). The 23-call block of 2026-09-07 is decided and
archived; its decisions live in `docs/decisions.md` (`D-TRUE-UP-CALLS-2026-09-07`,
`D-CONSENSUS-ARMING`, `D-H43-WORKLESS-DESIGNEE`).

**Scope calls — all four DECIDED 2026-09-07** (`D-RC-SCOPE-S1`, `D-RC-SCOPE-S2-S4`): S1 the RC ships
economy default-OFF with every lane built, the flip is a `0.9.x` release before `1.0.0`; S2 the
operational floor is post-RC as Boulder 5, the two S6 kills are `1.0.0` gates; S3 `#558` was
RC-blocking (DONE); S4 `#437` is post-RC (`1.x`), the crypto choice research-gated then.

### The Boulders — status ledger (reordered 2026-09-08)

The organizing spine is unchanged in membership; the ORDER and Boulder 1's scope changed on
2026-09-08. DONE means merged on main with its PR. Everything OPEN appears in the lanes above, by
row id.

#### Boulder 0 — Stop the live bleed (A4 money-pump) · ✅ DONE + MERGED
R0.1–R0.5 (#686); RT-DELIV-3 (#699, gate #700); R0.4b receipt-expiry (#711; the cross-server
double-redeem pump closed by per-epoch issuer-key expiry); R0.6 the I5 cross-height `Pruned` slash
forgery (#714; `SlashesBytesCap` 16 MiB); R0.7 relay-lane mint (interim #718 → closed by R2.14 #721).

#### Lane A — consensus liveness · the h43 fix MERGED (#772) and FIELD-CONFIRMED
- **DONE:** A1 the h43 fix (#772, field-confirmed) · A2 `#380` direction (1) with gate G-H43-8 (#779) · A3 the graded run at the re-priced tiers (`97e3101-deep`, 31/1/0, 2026-09-09).
- **OPEN:** nothing. Lane A is CLOSED; E5 re-grades the frozen artifact after D3.

#### Boulder 2 — Turn the economy on, prove it under adversary · the RC's substance
- **DONE:** R2.1 slice 6a (#689) · R2.3 · R2.5 · R2.6 (hold; G2-gated) · R2.9 (ledger #759, node
  #760, deposit-at-anchor-expiry #763, B-9 at the node #764, the 256 KiB default + true-length
  manifest framing under the NEW genesis `f428d0a8…0951` #765, the cloud delivery-lane flow #766) ·
  R2.9a `B_bootstrap` (#734–#745; `grant/r` = 64 GiB) · R2.10 F8 (#727) · R2.11 (#747) · R2.12 the
  faucet (#746, #754, #755, #758, #761) · R2.13 (#717) · R2.13b (#724) · R2.14 (#721) · the
  per-stripe parity fetch (#751).
- **OPEN (Lane C):** C1 the v2 primitive deletion · C2 the idle-window default after A3 · C3 R2.2 ·
  C4 the R2.7 telemetry · C5 the pre-flip closers · C6 R2.4 · C7 R2.7 · C8 R2.8 · C9 measurements ·
  C10 small PRs.
- Design records: [`docs/thinking/2026-09-01-economy-observability-design.md`](docs/thinking/2026-09-01-economy-observability-design.md)
  and the dated R2.x deliberations in `docs/thinking/`.

#### Boulder 3 — The freeze (era-4/v5) · the RC release
- **DONE:** R3.1 the SMT domain-separation residual (#731, #749, #754) · R3.3 (doc-only; re-derive
  only if #299 moves) · `#558` the chain-store refusal (2026-09-07).
- **OPEN (Lane D):** D0 the cold auditor · D1 the freeze manifest (three digest leaves, the last
  format touch) · D2 the stamp-raise test deliverables · D3 the freeze act.

#### Boulder 4 — B8 on the frozen artifact + the M0 endgame
- **DONE:** R4.3a (#715) · R4.3b shadow mode (#725).
- **OPEN (Lane E):** E1 R4.3b `on` · E2 R4.2 (doc) · E3 the standing hunt · E4 R4.4 = R1.7 the
  external pass · E5 the RC field grade · E6 `1.0.0`. R4.1 is a review gate that fires per PoD
  increment, not scheduled work.

#### Boulder 5 — the operational floor · POST-RC, ahead of the recompute
Packaging, signed installers, R4 self-update; the two S6 kills as `1.0.0` gates; `#437` post-RC.

#### Boulder 1 — "the cheap validator" · POST-RC, after Boulder 5, toward `1.0.0`
- **FROZEN:** the trustless-recompute track (above).
- **OPEN:** the design consult ((a)/(b)/(c) vs B8) → owner ratification → R1.8 only if still needed.

**Watch-items / standing gates (not scheduled):** bond-floor vs pony-disk ratio (a dashboard row);
owned-residual doc lines (SHA-256 pinned, store-free verify, R3 16 KiB cap); PayWord re-derivation
(R3.3). The old Rock 2 (third-operator committed settlement, DEFINITION only, #658) attaches its
demand→standing bright-line to R4.1 and is post-RC.

> *The 2026-08-19→08-31 six-Rock overlay ("Superseded Rocks"), the retired Phases 1–6 and the
> 2026-08-26 priority snapshot live in
> [`/archive/roadmap-history-2026-09-01.md`](archive/roadmap-history-2026-09-01.md); the
> 2026-09-01→07 per-Rock detail lives in
> [`/archive/roadmap-boulders-detail-2026-09-07.md`](archive/roadmap-boulders-detail-2026-09-07.md);
> the 2026-09-07 critical path, owner-call block and 125-row register live in
> [`/archive/roadmap-reorder-2026-09-08.md`](archive/roadmap-reorder-2026-09-08.md).*

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

- **The 2026-09-08 reorder — simplicity, by owner direction via the PE.** The owner read TENETS and
  VISION against what the loop had shipped and asked which team we are: the one that makes a
  complicated thing simple, or the one that makes a simple thing complicated. The measured answer
  (`core/chain` 3,025 → 13,093 non-test LOC in 20 days; 43 % of the last 255 commits bookkeeping
  against 10 % `feat(`; 275 residual names; six block formats in three weeks): the trust-plane CORE
  is essential complexity and stays; the floor-box trustless-recompute keystone added in the last
  three weeks is accidental complexity that violates B8 (a research-frontier answer — stateless
  validation of unbounded state — to a problem with three settled corners) and sat on the RC
  critical path though the RC ships never-Accept. The track is FROZEN, Boulder 1 is re-scoped to
  "the cheap validator" post-RC, the economy is promoted to the RC's substance, the register is
  pruned to actionable rows, and ten simplicity rules are standing in `.claude/CLAUDE.md`
  (`D-RECOMPUTE-FREEZE`; the note: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/NOTE-to-silt-team-simplicity-and-roadmap-reorder-2026-09-08.md`).
- **The 2026-09-01→07 build run closed the live breaks and built the paid economy dark.** Sixty-seven
  PRs (#697–#767): the A4 money-pump (Boulder 0), the I5 pruned-slash forgery, the relay-lane mint, the
  floor-box witness-soundness spine through R1.6, the `LastCommit` carrier, O3 Direction T, the whole R2.9
  paid-delivery economy on PayWord under the numéraire (ledger + node halves, deposit-at-expiry, B-9), the
  `B_bootstrap` instrument behind a build tag, F8, the faucet, R2.14, R4.3b shadow, R3.1 — every one
  blind-reviewed and certified, every gate ablation-proven RED first. The graded run `c450985-deep` closed
  the economy on the wire (29 pass / 2 gap / 0 fail) and DISCOVERED the h43 liveness stall (Lane A). The
  2026-09-07 true-up then rewrote this file around the critical path to the RC.
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

> **Residual filing rule (2026-09-08 — simplicity rule 4, owner direction via the PE).** A residual
> must be ACTIONABLE or it does not exist. A row in the **Residual register** below requires an
> **owner** (a seat), a **closer** (the PR, measurement or owner call) and a **Boulder or lane**.
> Anything else is a sentence in [`docs/design/m0.md`](docs/design/m0.md) §10 (a design residual held
> in tension, bounded and disclosed) or nothing. No new prefixes (`R-BB-`, `G-`, …) without an owner
> ratification. A residual name is any token of the form `R-<UPPER>[-<UPPER|DIGIT>…]`; a name may
> appear in this file ONLY if it has a register row (enforced by `scripts/check_residual_register.py`,
> `scar:residual-backlog-unbucketed-2026-09-06`), so closing a residual REMOVES its name from the plan
> in the closing PR — the audit trail is git and the dated archive files, not the register. `Bucket`
> is kept for the lint and is `ACTIONABLE` on every row by construction.
>
> Pruned 2026-09-08 from 125 rows to the actionable set (24 today; C1 closes the guard-restore row): 60 closed rows deleted (verbatim in
> [`/archive/roadmap-reorder-2026-09-08.md`](archive/roadmap-reorder-2026-09-08.md)); 34 held-in-tension
> and owner-disclosed rows folded into `m0.md` §10; the Round 1B / recompute-track rows frozen with the
> track (the archive file). `Source` names a `silt-reviews/…` path or a line of this file at filing time.

### Residual register

| Name | Bucket | Closer | Source | Duplicate-of |
|---|---|---|---|---|
| `R-H43-ROUND-LADDER-DESYNC` | ACTIONABLE | Lane A3 · Tester: the fix is merged (#772) and field-confirmed (`2633a11-deep`); the row closes when the next graded run grades `6-fault-tolerance` at the re-priced 190 / 380 s tiers with the head-reading wait (#774) | silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-COMPOSED-DIFF-c2a476a-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-REAPER-FORFEIT` | ACTIONABLE | Lane C2 · Tester: the delivery idle reaper runs on the WALL clock (`core/node/deliverysession.go:29-31`), so size the idle window against the STALL case (Lane A's published bound), not the 44 s/height steady state; the relay lane keeps the burn and its settle-inside-one-epoch measurement is owed before `-accept-relay-payments` is enabled; then Builder sets the default (one line) | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-SUBFRAME-PREPAY-ONLY` | ACTIONABLE | Lane C7 (research-gated, a precondition of the C6 flip, NOT of the C5 merge — the economy ships default-OFF) · MEASURED by the blind PE and encoded on the branch: serve revenue accumulates per `(server, requester, root)` lane (`core/credit/escrow.go:265`), so 5,000 serves over 250 distinct fetchers skim **250 credits at a 262,160 B shard and 0 at a 1,048 B shard** — the first credit on one lane costs 3,002 serves against 12. Sub-frame objects therefore do NOT become serve-funded when their bounty base falls to zero; they become **prepay-only** for durability. The coupling that produced it: C5's storage half pushes an object class into precisely the integer-truncation regime its pricing half warns about, where the accumulator mitigation is already REFUTED on build-immutable #8. Closer: the Researcher prices sub-frame durability before the flip | silt-reviews/principle-engineer/RULING-c5-preflip-closers-399f842-2026-09-09.md | — |
| `R-SUBFRAME-SIZE-ORACLE` | ACTIONABLE | Lane E3 (red-team) · TWO privacy reductions in the same direction from one change, both measured and both recorded in `docs/threat-catalog.md` under the existing F3 entry as a CATALOG UPDATE, explicitly not a mitigation: (i) padding previously blurred a sub-frame object's size to "somewhere in one chunk" and its exact byte length is now an oracle to any caretaker (layout) and any holder (stored shard length) over the whole class ≤ `chunkSize − 9`; (ii) convergent dedup for sub-frame objects now SPANS `-chunk-size` — the same payload yields ONE root at 64 KiB / 256 KiB / 1 MiB (a multi-frame object still yields two, which bounds it), so chunk size was an accidental salt against the confirmation attack and no longer is. Closer: a red-team pass on this surface before the flip; inventing a salt without one is the unreviewed novelty B8 forbids | silt-reviews/principle-engineer/RULING-c5-preflip-closers-399f842-2026-09-09.md | — |
| `R-ANCHOR-BEARER-TRANSFER` | ACTIONABLE | Lane C5 · red-team: a demand anchor is a bearer instrument — its presenter need not be its payer (the refund therefore pays only an existing account); one pass on the transfer surface before R2.4 | silt-reviews/research/research-outcome/R2.9-session-remainder-refund-and-live-anchor-cap-RESEARCH-CERTIFICATION-2026-09-06.md | — |
| `R-COMPACT-ORPHAN` | ACTIONABLE | Lane C10 · Builder: the BENIGN compaction-failure class has no daemon WARN line — surface `CompactFailures` / `LastCompactError` on the banked/status path (WIP `builder/c10-compact-orphan-warn` @ `49eb5d1`: the line + both ablations RED; suites not run) | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-PRIVACY-OPERATOR-TAB-TOKEN` | ACTIONABLE | Lane C10 · Builder: a persistent operator-token route is the UX follow-on before the `-privacy` default reaches real operators (`D-UI-PRIVACY-FLAG`) | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-membership` | ACTIONABLE | Lane D1 · Builder: retire `slashedRoot` and `validatorsSeenRoot` from the v5 digest set (D-V5-WHOLESET-ROOTS five → three; owner call 1 RATIFIED 2026-09-07) in one pre-freeze PR with the explicit `objective()` guard that also covers `MinBond` (G-1 is NOT satisfied on main — the box-entry assert covers `verifyBond` only); a hard fork at activation, free while era-4 is dark; the LAST format touch | silt-reviews/research/research-outcome/R-membership-unbounded-sets-and-recovery-boundary-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md | — |
| `R-LATE-REVEAL` | ACTIONABLE | Lane D1 · Builder: closes with the (d-3) two-level block hash `AnswerDigest`, BOUGHT (owner call 9, freeze manifest item 3) — lands in the stamp-raising train | silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-ISSUERKEY-POP` | ACTIONABLE | Lane D1 · Builder: reserve the proof-of-possession format slot INERT at the stamp raise (owner call 14: reserve-only) and correct the two comments; the research-gated D-DEMAND change (the DSKS close — a PoP in the registration, or the RFC 9578 binding) is post-RC | silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-R3-GOB-ALLOC-AMPLIFICATION` | ACTIONABLE | Lane D1 · Builder: a 5-byte hand-crafted gob length prefix forces a ~10 MB allocation in `proof.Unmarshal` — `SProofMax` bounds ENCODED bytes, not parse memory (`core/statehash/witness_bound.go:78`); bound parse memory with a decoder limit in the stamp-raise train (deadline moved 2026-09-08 from the frozen flip) | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-BOX-STALLS-ON-TAKEDOWN` | ACTIONABLE | Lane D1 · Builder: a revocation-bearing v5 block STALLS on `provenView` by name (`ErrRevLogSizeUnauthenticated`, driven by G-D8) until `tagRevLogSize` lands as the safety leaf (BOUGHT, owner call 9); the leaf in the stamp-raise train closes it and the k ≥ 1 `LogRoot` coverage lands with it | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md | — |
| `R-CARRIER-MODELCHECK` | ACTIONABLE | Lane D2 · Tester: the seating AGREEMENT property in the model-check tier, before the stamp raise (freeze manifest item 14) | silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-CARRIER-PRUNED-HASH` | ACTIONABLE | Lane D2 · Tester: prove the seen-fold never depends on a pruned body and, end-to-end, that the first non-pruned descendant's root check catches a rewritten ancestor; G-D11 (two-sided) shipped in Round 1A, bounded not eliminated; deadline the stamp raise (scope depends on the bought (d-3)) | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md | — |
| `R-E2E-ERA4-FIXTURE` | ACTIONABLE | Lane D2 · Tester + Builder: the e2e delivery-receipt daemon runs `-objective=false`, so the era-4 readiness tally can never latch on that fixture and the paid lane's POSITIVE arm has no e2e coverage; upgrade the fixture to objective + bonded + epoch-enabled (never expose `Config.Era4ActivationHeight` to a harness); the e2e cost ACCEPTED (owner call 14); pinned by `core/chain TestGateF_NonObjectiveTopologyCanNeverLatchEra4` | silt-reviews/research/research-outcome/R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md | — |
| `R-CLOUD-ERA-PROBE` | ACTIONABLE | Lane D2 · Builder: expose the chain's block era on a CLI/status surface (`chainstatus.go` prints no era) so the cloud sheet's `13b-delivery-settlement` can tell "era-4 dark" from "keys off-commitment"; one deliverable with the release-runbook line that survives of the rollout-signal item (freeze manifest item 19) | silt-reviews/principle-engineer/RULING-cloudtest-delivery-lane-flow-aab3626-2026-09-07.md | — |
| `R-CARRIER-GENESIS-DISPOSAL` | ACTIONABLE | Lane E3 · red-team: both code halves SHIPPED (#720, #723); the O-2 probe is owed — a "pruned" genesis served to a fresh-sync victim with our hash and an attacker-chosen body (`AppendGenesis` never checks `IsPruned()`), carrying the PE's composition (attacker-declared `BondRegs` qualifying its own keys under a kept `Pruned`); if it lands, the Tester encodes it and the Researcher certifies the refusal | /archive/roadmap-boulders-detail-2026-09-07.md L681–734 (the O1/O2 carrier ratification and the O-2 probe) | — |
| `R-CARRIER-DOUBLESIGN-SLOT` | ACTIONABLE | Lane TAIL (LOW) · Builder + Tester: the evidence producer LIFTS a carried precommit onto the evidence copy of the parent's `Atts` (hash unchanged, no format, no era gate; NOT a consensus-rule change), with the SILENT-GREEN and FALSE-SLASH traps as Tester gates | silt-reviews/research/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md | — |
| `R-CARRIER-ORDER-ORACLE` | ACTIONABLE | Lane TAIL (LOW, gate quality) · Tester: rewrite the call-site gate on the behavioural oracle in the PRE-MATURITY branch | silt-reviews/research/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md | — |
| `R-SWARM-NOTBANKED-DEAD` | ACTIONABLE | Lane TAIL (LOW) · Builder: a reachability argument or removal of the not-banked dead branch, one small PR | silt-reviews/research/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md | — |
| `R-SPARSE-COLUMN-PROVIDER` | ACTIONABLE | Lane TAIL · Builder: cap the per-provider shard probe in `confirmColumnHolders` (`core/node/file.go:1041-1064`) — a live 1-of-T parity-column holder costs up to T round-trips and the corpse gate trips only on proven-dead; does NOT fool the durability audit | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-PS-LOCAL-ROT-NOT-HEALED` | ACTIONABLE | Lane TAIL · Builder + Tester (owner call 17 ACCEPTED 2026-09-07): evict a verified-rotten shard the node still hosts and announces so the fetch path replaces it (no scrub exists; the sweep is stat-gated, `core/node/repair.go:85,457,678`); one PR with a Tester gate RED first; research-gated if it reaches bounty or escrow | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-LANEOFF-ROTATION-RUNTIME` | ACTIONABLE | Lane TAIL (low, test debt) · Tester: that no demand-key rotation goroutine RUNS after a failed boot install is pinned only by a source-order gate (`cmd/silt TestDaemonArmsTheRotatorOnlyAfterABootInstall`); the runtime observer needs an unwritable issuer directory whose publish-token key exists and an epoch crossing | silt-reviews/principle-engineer/RULING-R0.4b-C3-close-271ab81-final-2026-09-03.md | — |
| `R-CARRIER-CREDIT-DENIAL` | ACTIONABLE | Lane TAIL (doc only) · Builder: the multi-block inclusion window DECLINED for v1 (owner call 3, 2026-09-07); what is left is the doc fix the 2026-09-03 re-pricing named (the carrier narrows the pre-existing denial to one proposer; a minimum and a set-indexed vector are REFUTED) | silt-reviews/research/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md | — |

These are the off-critical-path residuals; they do NOT gate the Boulder spine. Repro recipes for
the named field defects (#535/#530/#574/#586/#277) live in
[`docs/thinking/2026-09-01-residual-defect-repro-recipes.md`](docs/thinking/2026-09-01-residual-defect-repro-recipes.md).

**Security / data-safety residuals:**
- **Demand issuer-key proof-of-possession (DSKS) — R0.4b-PoP.** `validateIssuerKeys`
  (`core/chain/issuerkey.go`) requires a verifying ed25519 self-signature, an in-range epoch and
  a bond, but **no proof that the registrant holds the RSA private key** whose fingerprint it
  registers, so bonded issuer B can register issuer A's fingerprint for epoch E — Duplicate-Signature
  Key Selection (Blake-Wilson & Menezes 1999). **Latent, not live:** `handleDeliveryReceipt` resolves
  ONE configured issuer and ledgers are per-node. The close is a PoP in the registration or the RFC
  9578 binding `keyFingerprint(32) ‖ epoch(8) ‖ serial` in `demandMsg` — a validity-rule change,
  research-gate + owner ratification; the format SLOT is reserved inert at the stamp raise (Lane D1,
  `R-ISSUERKEY-POP`). Source: crypto-specialist advisory C-4,
  `/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R0.4b-C3-blind-RSA-epoch-binding-2026-09-03.md`.
- **FDH domain-separation-tag length prefix + 128-bit reduction slack — R0.4b-FDH.** The three
  FDH domains are not length-prefixed and `fullDomainHashD` expands to only `nLen + 8` bytes. Sound as
  built (the three domain constants differ at byte index 10) but sound *by accident of the constants*.
  The publish and credit domains are BYTE-FROZEN against chain replay, so the change was DECLINED at
  the freeze in favour of a prefix-freeness gate (freeze manifest, 2026-09-07). Source: advisory C-8.
- **Blinding-factor sampling: mod-reduction, not rejection sampling — R0.4b-BLIND-SAMPLING.**
  `blindtoken.randInt` reduces `(bitlen(N) + 64)` random bits mod `N`; RFC 9474 §4.2's MUST is met
  only statistically (within `2^-64` of uniform). A conformance gap, not a break; changes no committed
  byte. Work: rejection-sample into `[1, N)` and **bound the retry** — the two loops calling `randInt`
  spin forever on a reader that yields zeros. Declared at `core/blindtoken/blindtoken.go` and
  `docs/thinking/2026-09-02-r0.4b-c3-close-design.md` §12. Source: crypto advisory R4,
  `/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R0.4b-C3-crypto-items-as-built-01bf8e9-2026-09-03.md`.
- **Transport authentication (TLS or Noise) — #437.** **POST-RC (`1.x`) by scope call S4.** silt's
  wire is unauthenticated CBOR (`adapters/tcpnet/wire.go`). Certified NOT a safety break and NOT a
  wedge (stripped blocks fail `ValidateCommit`; residual = censorship over a controlled link, already
  tolerated by the liveness model). Consult `docs/network-durability.md` before design
  (build-immutable #5); research-gate the crypto choice. Cert:
  `432-proposer-prepare-required-RESEARCH-CERTIFICATION-2026-08-16.md` §5.2.
- **Crash-safety: torn `chain.cbor` → silent genesis fallback — #558.** **CLOSED 2026-09-07 (scope
  call S3): fsync'd atomic write + refuse-to-start unless `-accept-chain-loss` (which preserves the
  original as `chain.cbor.rejected-<unix>`).** Attribution corrected by the PE: the field event was an
  INTACT file an era-2 replay bug rejected (`core/chain/reload_era2_558_test.go`), not a torn write.
  **Owner note:** the rule refuses on ANY structural-verification failure, a larger surface than S3's
  "torn tail" — kept per the PE; ratify or narrow. Ruling:
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-b8-558-chainstore-refuse-to-start-2026-09-07.md`.
- **On-disk format migration policy — #237.** DECIDED with the freeze manifest (owner call 9):
  REFUSE TO START across a format boundary, with a clear message. The stamp-raising release carries it.
- **Silent-behavior observability — #235.** (1) healthy-sweep and repair-pass summaries and (2) the
  `-revoke` progress lines are DONE on main; (3) rolling upgrade across a format boundary degrades
  with a one-line stderr notice — closed by #237's refuse-to-start at the stamp raise.

**Test / harness debt:**
- **Cited-test lint + the OWED ledger it opened — PR #708 (lint) and PR #707 (first two payments).**
  `scripts/check_cited_tests.py` fails the build when a `TestXxx` cited in a Go comment, `CHANGELOG.md`,
  `ROADMAP.md` or `docs/**` resolves to no `func TestX(` in the repo (the
  `scar:cited-test-does-not-exist-2026-09-02` class). In-repo citations are STRICT; external review
  trees are ADVISORY; `.claude/` is excluded (those worktrees are copies of OTHER branches). **Open:**
  the rest of the OWED ledger.
- **Test-honesty audit — #303.** 27 adversarially-verified test-honesty issues + 9 product findings
  from a per-harness audit of all 18 integration harnesses against the 5 field-test immutables — each
  a way a harness could go GREEN on a broken product (e.g. the redteam SCENARIO 2/3 assertions key off
  a non-specific `!resp.OK`; fix: add an H3 positive control). Open QA debt.
- **Harness reachability + flow-overlap (#574), Docker pre-genesis stall (#530), skim-observer
  arming (#586).** Standing-tail harness defects; repro recipes + fix directions in the repro doc.
  #530's first step is instrumentation (`-log debug`, one full client transcript), not a fix
  (build-immutable #7).
- **A runtime cover for the derived `-quorum` wiring — owed on the next visit to `cmd/silt/daemon.go`.**
  The wiring of the derived gather target is held by a SOURCE gate (string, order, count) and its VALUE
  at the call site is declared UNGATED in the gate's own doc comment. A blind PE broke the value arm
  three ways in ten minutes, each compiling and each leaving every arm green — `var effByz bool = true`
  in an enclosing block (one token off the gated spelling, and it re-opens a validity-lowering blocker
  verbatim), a plain reassignment, and a widened condition. Which binding an identifier resolves to is a
  scope property and a substring search cannot decide it, so more arms would advertise coverage they
  lack. The cover that kills every spelling at once: drive the daemon and assert the "gather target
  derived to" console line is ABSENT under `-byzantine-quorum=false` and present with sizing on
  (machinery at `e2e/e2e_test.go`; e2e tier, since it is skipped under `-short`). Tester; Lane TAIL.
- **CPU-time O(depth) regression CI gate — #616.** The shipped O(depth) gate (#613) measures
  baseline-subtracted `HeapObjects`, so it catches allocation-shaped depth blow-ups but NOT
  CPU-time-shaped ones (the #528 per-height CPU burn). Needed: a companion wall-time slope-vs-depth
  gate — with its own **noise study first** (measure run-to-run variance, derive the bound from the
  measurement, prove it goes RED on an injected CPU-time O(depth) defect). **A second member of this family,
  measured 2026-09-08 (Lane C4):** `core/node TestC3_InboundReceiptsCostOHardnessChecksNotOPerMessage` (`rt_r04b_c3_hardness_hotpath_test.go`) asserts a WALL-CLOCK per-message budget of 760 µs and reds under load. Matched
  control taken at the time: clean `main` reds it WORSE than the branch under review (10.63 ms vs 1.07 ms per message), so
  it is load-sensitive and pre-existing, not a regression — but a gate that grades the box rather than the code cannot
  attribute a real regression when one lands. Same closer as #616: a noise study first, then a bound derived from the
  measurement (or a re-basing onto an allocation/operation count, which is load-free), proven RED on an injected defect.
  Tester; Lane TAIL. PE ruling
  `RULING-613-odepth-ci-gate-2026-08-28.md` §B; deliberation `docs/thinking/2026-08-27-o-depth-ci-gate.md`.
- **cloudtest infra-liveness-FAIL journal-capture gap — #504.** The `infra-node-liveness` row's FAIL
  path has NO capture step (run `fa501cc-56689` named `island-c×3` crashes then let the EXIT-trap
  teardown destroy them). Fix (third-time rule — a gate, not prose): on FAIL, pull each named node's
  `journalctl -u silt` + `dmesg | tail` into `failed-nodes-<run>.log` BEFORE returning.
- **The full-suite command.** `go test -timeout 40m ./...` is the documented full-suite command
  (DONE 2026-09-06); the release-only measurement family in `core/chain` alone exceeds 40 min under the
  load rule's throttle (measured 2026-09-08) and is skipped under `-short`, which is what CI and local
  runs use. Do NOT shorten a measurement — the number is the artifact.

**Field-test harness residuals (folded from the retired `integration/FIELD-TEST-ROADMAP.md`, 2026-09-01):**
The RC field-test gate is MET (RC run `585c82a-58990` graded 28 pass / 0 gap / 0 fail / 2 skip-by-design;
deep lineage through `2633a11-deep` 30P/2G/0F). What remains is harness truthfulness/coverage/parity
hardening — none gates the Boulder spine. The full list lives in
[`archive/FIELD-TEST-ROADMAP-2026-09-01.md`](archive/FIELD-TEST-ROADMAP-2026-09-01.md); the
load-bearing still-live items:
- **Harness truthfulness hardening — tracked under #303.** The consensus P0 negative control needs a
  real quorum + a positive control; refusal reasons read from the daemon log, not client stdout;
  `soak` memory-growth and `churn` seeded-placement gates need falsifiable oracles; `bond` C1 must
  assert reputation ∝ bond; `nat` hole-punch should assert the direct path bypassed the relay;
  `redteam` should cross-check the honest target's head height is unchanged.
- **Demand field test (#264).** `integration/demand` becomes real only once the demand P2/P3 seam is
  wired into the daemon fetch path (the #264 residual under Durability below).
- **`chaos` WAVE-2 redundant-bootstrap survival — root-cause open.** Pin it, then fix + assert or
  document the single-bootstrap topology limit.
- **#281 empty-routing-table self-heal wire-certification.** Fixed in-product
  (`Node.StartBootstrapRetry`) but no cloud flow disables the startup TCP-wait to exercise the real
  re-bootstrap path over the wire.
- **GCP substrate operability.** A full-topology single-zone run is blocked by two ENVIRONMENTAL
  constraints (a `us-central1-a` E2 capacity shortage; the default `IN_USE_ADDRESSES` = 8/region quota).
  Worth doing: an IP-headroom + zone-capacity pre-flight; shrink the public-IP footprint; make `nuke`
  sweep leaked VPC/subnets/firewall/routes by label and stop swallowing `terraform destroy` stderr.
- **Per-substrate parity + GCP-only scenarios.** Factor the shared `exec-on-node`/`assert-on-log`
  abstraction so one scenario targets either substrate, then scale-out churn (50+ nodes), a real
  firewall partition, `tc` link-shaping, long-haul soak; an AWS variant + two-cloud split is the far end.

**Durability / repair / demand residuals:**
- **Per-stripe parity fetch — MERGED 2026-09-06 (#751).** NetGet's parity fallback is a DEFICIT walk;
  the object-size term of the parity-amplification worst case is gone and the per-server floor was
  corrected DOWN to 27.94 GiB (2026-09-07). What remains: the sparse-column provider probe cap
  (`R-SPARSE-COLUMN-PROVIDER`, register) and local rot healing (`R-PS-LOCAL-ROT-NOT-HEALED`, register);
  the verified presence check (Get + Verify per data shard, 46 ms per 64 MiB chunk vs 2.5 µs for a stat)
  is taken for correctness — a pay-on-failure redesign (trust the stat, verify only when the pipeline
  fails) is the cheaper shape, unscheduled. Ruling `RULING-parity-fetch-per-stripe-design-2026-09-06.md`.
- **Repair dial-storm to dead holders — #277.** The DHT walk re-dials dead holders every sweep (the
  `deadUntil` negative cache is not consulted on the walk's dials), so under heavy permanent loss a
  sweep can't finish though ≥k shards survive. Part of the pre-gate repair-sweep family (#501 / #500 /
  #502). Repro in the doc.
- **Wire demand P2/P3 into the daemon fetch path — #264.** `core/demand` P2 fair-exchange floor and P3
  cost-to-wash are real and unit-tested but have no live daemon-wire seam. Wire one/both, then add
  `integration/demand`. The firewall (delivery credits never fund standing, γ→1/N #182) is immutable.
- **Bond-proof reply size / N² cost — #299.** The encoded bond-challenge answer is ~1.5 MB, near-flat
  in bond size, so proofs cost N²×1.5 MB per audit interval as the validator set grows. The sound fixes
  are structural (SNARK-wrapped succinct proof; fewer samples = a soundness trade-off, research-gated;
  FEC needs QUIC first). An **owned, named residual**. R3.3 keys the PayWord/RegCap re-derivation to
  "only if #299 moves". Possible cheap interim (unverified): de-duplicate shared DRSample parent blocks.

**Polish & latent wins (low-priority — folded from the retired `BACKLOG.md`, 2026-09-01):**
Small captured ideas; they gate nothing. When one matures, promote it to the section it belongs in.
- **Demand-responsive dispersion — pull half.** Let a node that had to *fetch* a chunk under load
  opportunistically cache and announce it, decaying when unused. Kin to #500.
- **Domain-aware placement gaps.** Query a candidate's failure domain when gossip hasn't reached it;
  domain-aware capacity spill.
- **Direct IPv6 dial before assuming a relay.**
- **Relay selection + failover.** A NATed node adopts the lowest-ID relay and retries it forever;
  wants selection + failover once community relays are plural.
- **`docs/` staleness enforcement.** Extend the `Docs ship with code` CI job to `docs/`.
- **e2e relay-in-the-middle variant** of the multi-process e2e suite.
- **Capacity/scaling shape test — deferred** (30×100 MB vs 300×10 MB) while the dev box is RAM-bound.

**Post-launch (explicitly not V1):**
- **Registry liveness-pruning + federation — #207.** Liveness-pruning of dead entries and
  federation/sharding of a large public registry are post-launch scaling. They do NOT gate M0.

**Umbrella/frontier issues kept as evidence anchors (not a second task list):** #183 (external red
team → **Boulder 4 R4.4, the M0 close gate**; close condition MET, owner deliberately holds), #182
(shared-content sealing frontier → R4.1 / research frontier above), #179 (H8 metadata privacy), #180
(H9 pluralistic takedown), #94 / #52 (the forward-tracks / R1 verify epics — now the Boulder structure
itself), #406 (consensus model-check = the first consensus gate).

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
