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
`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/NOTE-to-silt-team-simplicity-and-roadmap-reorder-2026-09-08.md`).
The external B8 pass attacks the frozen artifact; `1.0.0` follows a green multi-machine field grade
of the same artifact and the two S6 scaling kills. Ratified sequencing that still governs: nothing
turns the economy on over a live mint (Boulder 0 is DONE); a graded field run is gated on the
model-check tier covering its regime — a field run confirms, it never discovers
(`docs/build-process.md`).

**▶▶ THE SEQUENCE, RATIFIED 2026-09-12 (`D-STRUCTURAL-GATES-2026-09-12`). It supersedes the order
given in "The order of active work" below, and the difference is deliberate: the two structural
gates go FIRST, ahead of the individual defects they would have caught.**

1. **The two structural gates, in order — G-2 (the ADVERSARY-SHAPE gate) then G-1 (a GRADED LANE ON
   THE SHIPPED DEFAULT POSTURE).** Lane F, rows F1 and F2. They rank ahead of the amplification fix,
   the no-loss bounty and the defaults raise, all of which stay filed and stay owed. **The cost is
   real and is accepted rather than presented as free: known defects sit a little longer so that the
   mechanism which finds the next unknown one exists.**
2. **Make the first run clean** — rows F3, F4, F5: the `-bond` default that refuses to start, the
   docs naming defaults that are not the shipped ones, and the restart error text. **The reason it
   has leverage nothing else has: the owner's own acceptance cycles are now the longest-lead item on
   the project**, so work that lets him start sooner buys calendar directly. *That reclassification
   is RATIFIED and in the ledger: `D-B8-AI-ADVERSARY-2026-09-12` (owner: "yes land the B8
   classification"). It no longer sits in conversation only. The `-bond` default half is ratified
   separately (`D-BOND-DEFAULT-CLEARS-FLOOR-2026-09-12`); F4 and F5 are ordinary lane work, which
   needs no call.*
3. **In parallel, needing no owner input:** the O-8 measurement set (row F6) and the repair-claim
   rate-limit certification (row F7). Neither waits on 1 or 2.
4. **Then:** the owner's acceptance cycles · D3 the freeze · Astra / B8, the external pass (E4) ·
   the RC field grade (E5) · `1.0.0` (E6).

**★ THE GATE LETTERS COLLIDE, AND THE COLLISION IS REAL — QUALIFY EVERY USE.** The decision entry
names its two gates G-2 and G-1. This file already uses G-1, G-2 and G-3 for the `R-membership` /
box-entry family: **G-1 is the `objective()` box entry (#797, CLOSED)**, **G-2 was WITHDRAWN with the
digest retirement**, and **G-3 is the witness id-list size gate (KEPT AND RESCOPED post-RC)**. They
are different gates that share a letter. Every use of the new pair in this file is written out —
*G-2 (adversary-shape)* and *G-1 (shipped-default graded lane)* — and a bare letter in this file
means the older family (`silt-agent-memory` scar: a bare gate letter names at least six gates).

**Where the critical path stands (2026-09-10).** Lane A is CLOSED and field-confirmed. Boulder 2's
RC-relevant work is merged — the flat delivery path retired, the R2.7 detectors, the observability set
with per-tier work totals, the pre-flip code closers, and the idle window (value RATIFIED at 24m).
**Boulder 3 is the whole remaining critical path: D1 → D2 → D3 (the owner's freeze), then E4 (the
external pass) and E5 (the field grade).** Nothing in Boulder 2 blocks the freeze; C6 the flip is a
`0.9.x` release AFTER it.

**What the 2026-09-09/10 session changed, and it is mostly one thing.** A question the owner asked
about ONE ratified number — *the idle window's magnitude survived but its reasoning changed; was the
original derivation wrong, and what else was derived the same way?* — was answered by a blind
derivation-route audit, and the answer cascaded:

- **`SlashesBytesCap`'s CONFIGURATION route is CLOSED** (#795, `D-SLASHCAP-ROUTE`). The value stays
  16 MiB; a consensus validity rule no longer rests on the DEFAULTS of two proposer-side-only flags.
  The owner's framing is the durable half: this is the **#380 class** — a consensus quantity that is a
  function of local config rather than of the chain — and the second instance inside one week.
- **A THIRD face on that cap is now MEASURED, not disclosed-in-theory** (folded 2026-09-10 into
  `R-BIG-EVIDENCE-UNSLASHABLE`, which now carries all three faces because all three close on one act).
  `cap ≥ 2 × body + overhead` is an unsatisfiable fixed point because
  evidence nests. **A 687 B throwaway-key proof about never-committed blocks is accepted; 24,456 of
  them make a block VALID under `ValidateProposal` at shipped defaults; a legitimate proof about it is
  16,777,941 B over cap and is REJECTED — the equivocator keeps its seat, with no bond, no coalition
  and no misconfiguration.** Four certifications carry sentences that fall; one is inside an
  OWNER-RATIFIED sentence, annotated in place rather than rewritten.
- **`R-membership`'s digest 5 → 3 retirement is DROPPED** (`D-MEMBERSHIP-KEEP-FIVE-2026-09-11`, owner,
  REVERSING owner call 1 of 2026-09-07). The v5 digest set freezes at FIVE; ~~the residual's closer is
  **G-3, unbuilt**~~ — **SUPERSEDED 2026-09-11 by the G-3 scope ruling, left visible so the correction
  reads as one: `R-membership G-3` is KEPT AND RESCOPED post-RC, it bounds what one BOX accepts as a
  witness and bounds neither grow-only set, and this residual's closer is UNNAMED**; **G-2 comes out**,
  so `D-RECOMPUTE-FREEZE` is untouched. G-1 (#797) remains closed
  and remains worth having — it is the `objective()` entry assertion, not the retirement.
- **Owner call F is DELIVERED, and the node-scope config-gate residual is CLOSED and off the register**
  (its name is removed from the plan per the filing rule; the audit trail is git)
  (`D-CFGBIND-MEMBERSHIP-RULE-2026-09-10`). The genesis-config bind had shipped as a **schema with no
  wiring**: `genesis.Build` minted a paramless block, `omitempty` dropped cbor key 20, and
  `CheckConsensusParams` had zero non-test callers — so every network computed the same genesis hash
  whatever its flags. Five green `G-CFGBIND-*` gates missed it because each hand-built
  `Block{… Params: &p}` in its own fixture. **A gate that constructs the exact state whose production
  absence is the defect cannot detect that defect** — and the contrast is inside the same commit
  (`82fe56d`), where (d-3) was wired end to end and this half was inert, with nothing telling them
  apart. What replaces the "17 fields" list is the ratified **membership RULE**: *every field that can
  change a validity verdict is bound to the chain, or explicitly excluded with a recorded reason.*
  The reflective gate now covers **both** `chain.Config` and `node.Config`, and the two declaration
  tables form a **checked bijection** onto `ConsensusParams` — which caught `MinBondBytes`
  double-claimed the moment it was written. **17 is now the rule's output, not a number to remember.**
- **Three findings were written up and deliberately NOT acted on**: the FT tier priced below its own
  published bound (#796), the relay lane's edge-tier unfitness (row C11), and the PoP bound
  (CERTIFIED at 4,096 B, not built — a format surface).

**The pattern worth carrying into the next session:** three times in that session a NUMBER survived
review and its ROUTE did not — the idle window, `SlashesBytesCap`, and the self-armor threshold the
certification itself derived 5 % wrong. Derive, then DRIVE, in that order; never let the derivation be
the citation.

**The order of active work — SUPERSEDED 2026-09-12 by the ratified sequence above, and kept as
written so the correction reads as one.** What changed: the two structural gates and the first-run
clean-up now come first, and Lane A is closed. The lane-to-Boulder mapping below is unchanged and
still governs everything after step 4. *The superseded sentence:* Lane A (consensus liveness) → Boulder 2 / Lane C (the economy — the
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
| A1 | The h43 round-ladder desync — the round clock armed on LOCAL mempool content (`rounds.go:306`), so the round number was a function of unreplicated private state | **FIXED and MERGED (PR #772), FIELD-CONFIRMED on `2633a11-deep`:** the arming rule (a replicated condition), the relayable round certificate with suffix-semantics round-changes, the designee proposing at the certificate's round, and the workless-designee closer (entries forwarded to the round's designee, cap 4) — `D-CONSENSUS-ARMING` + `D-H43-WORKLESS-DESIGNEE`. Published bound: ≤ f′+1 rounds after GST = 190 s at f = 1, N = 12. Field: 39 s/height on the deep drive (44 before); h30→h40 with a validator down at ~50 s/height. Certifications: `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md` and `…/CONSENSUS-LIVENESS-h43-COMPOSED-DIFF-c2a476a-RESEARCH-CERTIFICATION-2026-09-07.md`; the #441 mature-regime PUBLISH starvation shares this root and is closed by the same fix | nothing but A3: the graded run at the re-priced tiers closes the register row | Tester |
| A2 | `#380` objective-mode `Config.Quorum` floor divergence — `RequiredQuorum()` returned the LOCAL `cfg.Quorum` verbatim in the mature regime (I1), and `SupportMeetsQuorum` gates `newViewFor`, so a divergent floor was also a permanently dead designee (liveness) | **DONE — MERGED 2026-09-08 (PR #779, `fa854d7`; the merge RATIFIED the amended `D-CONSENSUS-ARMING` (20) sentence, G-380-B):** direction (1) as SPECIFIED and CERTIFIED by the one certification for the change (`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/CONSENSUS-380-quorum-floor-direction-1-PREDICATE-AND-CERTIFICATION-2026-09-08.md`, §1 the predicate per regime, §6 the composed diff CERTIFIED): objective + Byzantine sizing ⇒ `bftThreshold(validatorSetSize())` in the young window and **0** in the mature epoch (the >⅔ frozen-weight rule is the whole bar; head-counting the epoch set stays REFUTED per B2); the trusted opt-out (`-byzantine-quorum=false`) and legacy keep `cfg.Quorum`; `Quorum` survives as the proposer-side gather target on EVERY proposal path (`gatherTwoPhase`, the one choke point all four paths share, raises to `max(caller floor, ConfigQuorum(), RequiredQuorum())`; the PE found the new-view and bond-drain paths had passed 0 and would have gathered less than before, and the Researcher's G-380-A found the FORCED new-view leg bypassing the first fix — a uniform swarm's blocks now carry the same count as before, so the change adds accepts only; rollout note in the CHANGELOG: upgrade validators first). Three merge conditions closed in the same PR: Reload counts the same predicate (M-380-1), `newViewFor` refuses a certificate with no round-change from anyone but the designee (M-380-2, wire-reachable via the attester path; the PE caught the designee-only walk-around of the first `len == 0` guard), `RequiredQuorum` pinned by G-D13 through a helper row (M-380-3). Gate G-H43-8 = five arms RED-first (8a the #338 fixture inverted under Byzantine sizing; 8b the higher-floor designee assembles the certificate; 8c mature floor 0 accepts with the weight bar still refusing by name; 8d Reload symmetry; 8e the gather-target control) + the regime-(c) stranding control kept; three ablations RED on the named mechanism; the v5 mirror exact with the mature regime added to the parity oracle | nothing — merged with its five G-H43-8 arms, the regime-(c) control and the three ablations; **OWNER (batched, owed):** the `-quorum 3` default (a four-validator swarm on defaults tolerates f = 0; the field topology sets 2 — remedy is a derived default on the untrusted objective path, the `effectiveByzantineQuorum` pattern) and whether a single-anchor objective launch stays supported (at A = 1 no count gate holds anything; the PE recommends requiring ≥ 2) | Tester · Builder · Researcher · blind PE (all done) |
| A3 | The next graded GCP deep run | **DONE 2026-09-09 — `97e3101-deep`: 31 pass / 1 gap / 0 fail / 3 skip** (`integration/cloudtest/report-97e3101-deep.md`). The first fleet carrying #380 direction (1), the v2 flat-delivery retirement and the R2.7 detectors. **`6-fault-tolerance` PASSES within the computed 190 s down-designee escape bound** — the re-priced tier met in the field on #774's head-reading wait, where the previous run GAPped on the harness artifact; gaps 2 → 1, passes 30 → 31. The derived quorum floor changed nothing a uniform swarm commits: the whole consensus sheet passes (stall and capture drills, partition heal, equivocation island, forged-block refusal, WS cold-sync). Deep drive h93 → h130 at ~45 s/height; prune engaged and all validators converged on the pruned chain. The S7 repair economy closed on the wire with the flat path GONE, and the dark delivery lane still refuses at the withdrawal. No OOM, no crash-loop; teardown verified (40 destroyed, nothing left running) | the one gap is the known `184-low-bond` premise (#350) — the adversary holds a QUALIFYING bond and is correctly accepted, so the under-bond rejection property needs a dedicated sub-min-bond identity in the harness; the property is certified in-process. E5, the RC field grade, runs against the FROZEN artifact after D3 | Tester; owner's go (billable) |

The null proposal (PBFT's null request, the literature's closer for a workless leader) is held in
tension in `docs/design/m0.md` §10.1: it was routed to "era 5" on 2026-09-07, and under simplicity rule 6
no era is pre-approved — it waits for a ratified reason of its own.

#### Lane F — the structural gates, the first clean run, and the parallel work (NEW 2026-09-12) · FIRST among active work

**Everything in this lane comes out of the 2026-09-12 whole-project audit, and every row traces to a
`docs/decisions.md` entry dated that day.** Read the scope honestly before reading the rows:
**no consensus rule, no format surface and no economic mechanism changed on 2026-09-12.** What
landed was documentation, gates and defaults. Rows F8 and F10 are the only two that touch a rule at
all, and F8 is research-gated with nothing built.

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| F1 | **G-2, the ADVERSARY-SHAPE gate. FIRST.** For every defence silt claims, a fixture must exist in which the adversary HAS the capability the defence assumes it lacks. Today the shipped shape is the opposite: every storage adversary in the tree holds strictly LESS than a legitimate participant, and ~~no fixture has a prover that is a key holder~~ — **CORRECTED 2026-09-12: the SAME commit that landed the gate also landed the first fixtures whose prover IS a key holder** (`TestRT_POR_1_CareLinkHolderForgesWithZeroBytes_PINNED_DEFECT` is handed the layout key and holds zero bytes; `TestRT_POR_2_ChallengeProxyPassesAudit_PINNED_DEFECT` is granted an honest holder as an oracle). The premise this row was written against held at `b870ade` and no longer holds | **RATIFIED 2026-09-12 as a first-class roadmap item, ahead of the individual defects it would have caught** (`D-STRUCTURAL-GATES-2026-09-12`). ~~Nothing built.~~ **BUILT AND LANDED 2026-09-12 as `scripts/check_adversary_shape.py`, TRACKED but deliberately NOT WIRED TO CI** (owner-ratified disposition): all 24 in-scope defence claims are declared in one pass and the gate STILL EXITS 1, because a declared-uncovered claim is a RECORD, not an exemption. ~~No `fixture=` is used anywhere: no capability-holding fixture exists in the tree, and that absence is the finding this row names.~~ **CORRECTED 2026-09-12 before merge, by running the gate: 4 claims carry `fixture=`, 19 are `UNCOVERED:` and 1 is `NOT-A-DEFENCE:`.** The four are the two `LayoutKey` claims (`core/node/por.go` `porKeyDomain`, `core/por/por.go` `DeriveKey`), `HonestHolderAsOracle` (`core/node/por.go` `porChallengeSeed`) and the `core/por` package header, all covered by the two RT-POR fixtures landing in this same commit — which is also why the Item column's premise is struck above. **`SectorSecretsAlpha` is a DIFFERENT capability and the gate has NEVER SEEN IT**: the forger sets every `mu_j` to zero so the `alpha_j` terms vanish, and it never touches an alpha. It is recorded in PROSE ONLY, inside `core/por/por.go`'s package header — **no declaration names it, so it is not one of the 19 and `UNCOVERED` (this gate's term of art) does not apply.** An earlier draft of this row said it "stays UNCOVERED", which sent a reader grepping the 19 reasons for a string that is not there. Wiring the gate to CI named "the first such fixture" as its precondition; that precondition is now met, but the gate exits 1 by design, so wiring it remains a separate owner decision and nothing here presumes it. **★ RATCHET MODE LANDED 2026-09-12** (`D-ADVERSARY-SHAPE-RATCHET-2026-09-12`), and it changes what this row can ask for. ~~the gate exits 1 by design, so wiring it remains a separate owner decision~~ — **CORRECTED: the default mode now EXITS 0 and `--strict` exits 1 on the same tree.** The gate reddens on a claim that is NEW to it, not on a claim that is uncovered; the 19 `UNCOVERED:` records are grandfathered on an explicit allow-list in the gate's own source, keyed `(path, capability, digest-of-the-claim-sentence)` so a REWORD is reported on both sides (a fresh claim AND a stale entry) instead of silently falling out. **The countdown premise this row was written on is gone for good:** `R-ADVERSARY-SHAPE-CONTROL-NEEDS-A-BROKEN-DEFENCE` makes `fixture=` reachable only for BROKEN defences, so the 19 cannot be driven to zero by covering and are a RECORD, never a work queue. **THE ACCEPTED RISK IS RECORDED AND IS NOT SMALL: a grandfathered allow-list is how a backlog becomes permanent, and 19 entries that "must shrink" have NO forcing function** — growth and shrinkage cost the identical two edits and only review tells them apart. **THE FILE-SCOPE LIMIT IS FIXED in the same commit:** `marker_scopes` binds each `ADVERSARY-HOLDS` / `CAPABILITY-CONTROL` marker to ONE test, and the miss this row recorded — re-pointing `capability=LayoutKey` at `TestRT_POR_2_ChallengeProxyPassesAudit_PINNED_DEFECT`, measured at 19 problems NOT CAUGHT — now reports 20 and names the fixture (pre-fix gate 19, post-fix gate 20, same patched tree). **STILL NOT WIRED TO CI, deliberately, and that is still a separate owner decision** — what ratchet mode removes is the blocker that the gate could never go green; what remains is the judgement about putting a SOURCE-TEXT gate on a required check. It goes first because it has an ALREADY-MEASURED failure to validate against — which is the only thing that stops a new gate being vacuous on the day it lands | Build it against the storage-proof break two seats confirmed blind to each other: over 100 sweeps the zero-byte attacker's ledger row is **bit-identical** to the honest holder's, and the shipped `-liar` control is annihilated in the same run. Drive the gate RED on that measurement before believing it (`silt-ablation-noop-guard`). The same shape produced the session's vacuous fixtures, the inverted low-bond drill and the bystander gates, which is why it is a gate and not a fix | Tester → Builder |
| F2 | **G-1, a GRADED LANE ON THE SHIPPED DEFAULT POSTURE. SECOND.** At least one graded lane runs the configuration a stock operator gets, with nothing turned on or off to make the test convenient | **RATIFIED 2026-09-12** (`D-STRUCTURAL-GATES-2026-09-12`). Nothing built. **Seven defects in this session exist ONLY because of a default, and every existing gate configures its way OUT of the posture it should be testing** — row F3 is the plainest instance | Build the lane. **Why SECOND and not first, stated as a sequencing judgement rather than a ranking of value:** it is the larger build (a lane, not an assertion) and its validating failures are already captured as individual defects, so the cost of it landing a week later is bounded | Tester |
| F3 | **The `-bond` default does not clear the derived anti-release floor, so `silt daemon -validator` on pure defaults REFUSES TO START** | **RATIFIED AS A DIRECTION 2026-09-12, build owed** (`D-BOND-DEFAULT-CLEARS-FLOOR-2026-09-12`). Measured: `-validator` alone leaves `-objective` true and `-min-rep` 100, so the objective path is taken; with no explicit `-min-bond-floor` the floor defaults on to `DerivedBondFloor = 2 × (2 s × 270 MB/s)` = 1,080,000,000 B ≈ 1030 MiB; **`-bond` defaults to `64M` = 67,108,864 B** (`cmd/silt/daemon.go:116`), below it, and the daemon exits. **A defaults defect, not a security defect** — the floor is correct and deliberately default-on (*"fixed but off by default" is not fixed*); the two defaults were set in different places and never composed | **★ THE GOAL IS AMENDED 2026-09-12** (`D-BOND-DEFAULT-GOAL-AMENDED-2026-09-12`). ~~a gate asserts the shipped defaults admit a working validator~~ — **MEASURED: UNACHIEVABLE by any default.** Raising `-bond` removes a SPURIOUS refusal (the two defaults set in different places and never composed) and leaves a SUBSTANTIVE one: `coldStartScaffoldOK` refuses an untrusted objective validator with no cold-start scaffolding, which is the CORRECT M0 cold-start capture defence, and its two exits — `-anchors` + `-mature-validators`, or `-ws-checkpoint HEIGHT:HASH` — are **NETWORK-SPECIFIC**, so no compiled-in value can ever satisfy it. **NEW GOAL: the shipped defaults produce EXACTLY ONE refusal, and it names a remedy the operator can act on.** The gate may assert the count and the actionability; it may NOT assert that the daemon starts. **ACCEPTED RISK, recorded: amending a goal because it proved hard is how goals get weakened, and *"a stock operator cannot run a validator at all"* remains a real product problem this does NOT fix.** **MEASURED DETAIL — the daemon prints `1029` MiB, not 1030:** both the defaulted-floor notice and the refusal render `effFloor>>20`, and 1,080,000,000 B = 1029.968… MiB, which the shift TRUNCATES; the "≈ 1030 MiB" above is the correct rounding of the byte count and NOT the string an operator sees, so a doc or gate grepping for 1030 finds nothing. Raise the `-bond` default so the stock untrusted-validator posture starts and earns standing. **The floor does not move** (build-immutables #3/#4 forbid sourcing it from a transport deadline). **The number is NOT ratified**: it must clear the derived floor with margin and be defensible as a real operator's smallest sensible plot, and if it turns out to be a security parameter in disguise it routes to the Researcher. **★ THE GATE MUST READ THE DEFAULT, NOT A COPY OF IT** — and the census is by PATH: **TWO sites hard-code `int64(64)<<20`** beside a comment calling it *"the daemon's own default"*, `cmd/silt/bondfloor_default_test.go:48` (`TestAntiReleaseFloorDefaultsOnForUntrustedValidator`, the one the decision entry names) **and `cmd/silt/invariant_b_test.go:68` (`TestInvariantB_S1_AntiReleaseFloorOnByDefault`, which it does not).** Raise the default and both stay GREEN while both sentences go false. Re-point each at the flag's own `DefValue` and the daemon's `effFloor` arithmetic, or re-state it as the historical PoC value it actually is | Builder; **OWNER** ratifies the value |
| F4 | Docs naming defaults that are not the shipped ones | OPEN. Surfaced by the 2026-09-12 audit alongside F3; **no decision entry, and none is needed** — it is a documentation correction, not a call | One pass that re-derives each quoted default from the flag declaration rather than from prose. Every number re-driven at source, never copied | Builder |
| F5 | The restart error text | OPEN. Same origin as F4, same class, **no decision entry needed** | Make the message name the remedy the operator can act on | Builder |
| F6 | **The O-8 measurement set — the work the owed re-decision needs, and none of it is the owner's** | OPEN, and **deliberately NOT scheduled as an O-8 build** — `D-O8-BASIS-CHANGED-2026-09-12` is not a live ratification (see the owner-owned block below) | Three measurable pieces, none needing an owner: **(i) G-O8-D, the call-site census** — every non-test site that derives or checks a chunk ID from data, read in full; three hand-rolled sites in three files were already found, so the census is not a formality. **(ii) G-O8-E, a RECORD fix rather than an answer** — the certification calls the height-0 genesis-hash freeze-surface residual an OPEN precondition, having read `core/genesis/genesis.go`'s comment; **the record DISPOSED that residual on 2026-09-07 as CLOSED-BY-BOUND** (the height-0 hash is network identity, not a consensus format; the migration rule at a format boundary is refuse-to-start, #237 / owner call 9), so the comment is a **dead premise that manufactured a gate**. *Its name is deliberately not repeated here: the row is closed, and under the filing rule a closed residual's name leaves the plan — writing it back would resurrect a row the record already disposed, which is the failure this paragraph is about.* If the 2026-09-07 disposal stands, **G-O8-E is already discharged and O-8 is one gate lighter**; re-opening the disposal is its own decision and not a by-product of O-8. Either way this is a **contradiction of record, not a fresh open question**, and it must not be closed by assertion in either direction. **(iii) the detection and wire figures carried WITH their operating points**, which is the only honest way to carry either: today's scheme samples every block, so its detection against any deletion is 1.0 and its forgery cost is zero bytes; O-8 at one index detects a fraction ε with probability ε and costs 1.6 % more wire; at equal detection O-8 costs 63.5× the wire | Researcher / Tester (measurement); Builder (census) |
| F7 | **The three repair-claim gates, and the rate-limit certification** | **⚠ TWO OF THE PINS ARE REDEEMED, 2026-09-12** (`D-BOUNTY-REPAIR-BUILT-2026-09-12`): **RT-RC-2** is no longer a pin — the `(root, stripe, pos)` dedup landed, reddened it, and it is now `TestRTRC2_ReplayedClaimForAPaidPositionDrawsNothing`, a positive assertion; and **RT-RC-1 arm (c)** is no longer a pin — the position screen landed, reddened it, and `rtRC1OutOfRangeRefused` now asserts that an out-of-range position reaches ZERO of the object's shards. **RT-RC-1 arms (a) and (b) are UNCHANGED and still pinned** (no per-sender bound exists, and the fetch is still not budgeted to k), and **RT-RC-3 is UNCHANGED and did not redden** — that is the evidence nothing behind the loss-witness gate was built. No test was deleted; each redeemed pin was replaced by the positive assertion of the rule that reddened it, which is the ratified route. **One measured correction to this row's own record:** the in-range pin's *"15 distinct shards"* is really **14 survivor shards plus the manifest chunk** — the judge already hosted one of the 15 survivor refs, and two off-by-ones cancel. The assertion is correct about store writes and is left at its ratified value; the sentence that named it is corrected and the finding is filed as `R-RTRC1-COUNTS-CHUNKS-NOT-SHARDS`. **RATIFIED 2026-09-12** (`D-REPAIR-CLAIM-GATES-PINNED-2026-09-12`): the three gates land as `PINNED_DEFECT` — they assert current BROKEN behaviour, named so that a future fix REDDENS the pin and forces the record to be updated. **Skip-until-fixed was explicitly REFUSED**: a `t.Skip` is a dark test, costs the same lines, reports green, and `scar:short-run-is-zero-execution` exists because exactly that shape hid a whole tier from CI. The mechanism and the precedent already exist (#817, and the two live `core/pipeline` RT-SFO pins). **Authored by the Tester at `75c0f89`; ~~NOT yet in the tree~~ LANDED 2026-09-12** (the earlier check stands as a dated record: verified at `cc0cb35`, `core/node/rt_repairclaim_gates_test.go` did not exist on `main`). **All three were RED as authored** — they asserted the INTENDED rule, not current behaviour — and were converted to the ratified pin polarity before landing, so they are now GREEN and redden on a fix. **RT-RC-1 is re-authored to the `#844` correction** and its mechanism arms assert n−1 and n as DERIVATIONS from the fixture's own manifest (measured: 15 and 16 at k=10/n=16). The four-round retry multiplication is deliberately NOT encoded — an unratified severity claim is routed separately, and a pin asserting one would over-claim | Land the three pins — **RT-RC-1** unbudgeted survivor-fetch amplification (`handleRepairClaim` has no per-sender rate limit and the `RepairEconomy` gate sits inside `settleRepairVerdict`, which runs only AFTER `fetchSurvivors`; **this one fires on the SHIPPED DEFAULT**), **RT-RC-2** a replayed claim pays again (`PayBounty` keys on `(root, repairer, amount)` with no `(root, stripe, position)` dedup), **RT-RC-3** a claim with no loss pays. ~~Then the remedy: **a per-sender rate budget's burst value is a security parameter and is research-gated**, exactly as `R-CARRIER-QC-BURST-VALUE` already records.~~ **⚠ THE REMEDY IS REFUTED ON BOTH ARMS AND THE OWNER RATIFIED THE REFUTATION, 2026-09-12** (`D-REPAIR-RATE-LIMIT-REFUTED-2026-09-12`): the rate budget fails on a **precondition, not a value** — every `allowWindowed` budget in silt states a HEALING property, and `emitRepairClaim` binds an **empty reply callback**, so a refused claim is invisible and lost forever (new standing theorem **T-RETRY-IS-THE-PRECONDITION**; the judge's own comment already said *"Claim emission is one-shot, so a terminal deny here loses the bounty FOREVER"*). **No burst value exists** — one honest paramedic legitimately emits ~2,460 claims per sweep on a 1 GiB object — and it is filed as **"does not exist", NEVER as "measure it"**, because the estimand would be publisher-chosen object size and build-immutable #3 forbids resting on a steerable one. The check-ordering hoist is refuted separately and for a different reason: **`settleRepairVerdict` runs the SLASH before the economy gate**, and the slash cannot be preserved across the hoist because the verdict depends on the recompute, which depends on the fetch output. **A pin buys visibility, not a repair, and does not ratify the behaviour as correct.** **⚠ RECORD CORRECTION 2026-09-12 — the severity sentence below understates the drain, and `D-REPAIR-CLAIM-GATES-PINNED-2026-09-12`'s mechanism sentence understated the fetch.** Confirmed by a Researcher certification and a blind PE verification, re-derived at source: the judge fetches **n−1** survivors, not k — `judgeRepairClaim` builds `survivorRefs` as the **complement of one position** and `fetchSurvivors` walks it to the end with **no early exit at k** (15 at the shipped k=10/n=16), and an **out-of-range** `claim.ShardPos` excludes nothing so it fetches **all n = 16**, one MORE than an honest claim. And the claim body is **unsigned** with the payee taken from `claim.Holder`, a field of the message, so the drain is **DIRECTED**, not diffuse. ~~Severity stated so the row does not over-read: the γ→1/N firewall HOLDS across all three — `PayBounty` is `neutral` and `Reputation()` reads neither escrow nor bounty, so this is a durability DoS and an escrow drain, **not a mint**~~ — **the "not a mint" half STANDS and is not disturbed; what is corrected is that the drain is directed** | Tester (gates) → ~~Researcher (the burst value)~~ **no remedy owner: the routed remedy is refuted and a replacement direction is a new question** |
| F8 | **The durability bounty must be paid for REPAIR, not for holding** | **★ THE THREE AUTHORISED ITEMS ARE BUILT, 2026-09-12** (`D-BOUNTY-REPAIR-BUILT-2026-09-12`): the position screen (deny for a position the manifest does not list, **SLASH** for a listed position whose committed id disagrees with `claim.ShardID`), the `present`-count fix in `VerifyByRecompute` only, and node-side `(root, stripe, pos)` dedup on **PAID**. `erasure.ReconstructStripe` was not touched, the `present` pre-check was not deleted, `ports.CreditLedger` did not move, and `core/credit/escrow.go` was not edited. The `present` fix also closes the four-round retry amplification's structural half: with the screen ahead of the fetch and padding counted, **every `cerr` that still reaches the deferral path is a genuine short-survivor fetch**, which is the transient the deferral was built for. **⚠ A PREMISE THIS ROW AND THE CERTIFICATION BOTH CARRIED IS REFUTED BY MEASUREMENT:** `newRepairAdv` does **not** stage *"exactly one full k=10 stripe"* — `splitFile` reserves a frame header, so 10·512 KiB yields **ELEVEN** chunks over **TWO** stripes, and **stripe 1 is already short** (`realData = 1`, 7 stored refs). The conclusion stands — every pre-existing test claims stripe 0 — but no harness geometry change was needed, and the wired arm claims the short stripe that was already there. **★ STILL TRUE AND STILL UNWRITABLE AS SOLVED:** the bounty is correctly **METERED** and stays **MIS-ATTRIBUTABLE**; **RT-RC-3 is OPEN** and the loss witness is GATED behind `R-PROBE-FALSE-NEGATIVE-RATE`. | ~~**INTENT RATIFIED 2026-09-12; MECHANISM RESEARCH-GATED**~~ → **INTENT RATIFIED AND THE MECHANISM CERTIFICATION RETURNED GATED, RATIFIED AT THAT STRENGTH 2026-09-12** (`D-BOUNTY-PAYS-FOR-REPAIR-2026-09-12` → `D-BOUNTY-REPAIR-MECHANISM-GATED-2026-09-12`). ~~**Nothing changes yet: no code, no price, no ledger motion.**~~ **The gate is now discharged for THREE named items and for nothing else. ★ AUTHORISED:** direction A (compare the claimed position's manifest-committed id against `claim.ShardID`) — **and A MUST SLASH** a well-formed position whose claimed id disagrees with the manifest, or an earlier deny silently RETIRES an existing punishment and leaves silt strictly weaker; the **`present`-count fix in `VerifyByRecompute` ONLY**; and node-side `(root, stripe, pos)` dedup keyed on **PAID, never on judged** (judged-but-unpaid must stay payable — empty escrow, `BountyBaseZero`, a deferred re-judgment). **⚠ TWO PLACEMENTS REFUTED:** deleting the `present` pre-check routes a short-survivor **transient** into a bond-slash of an **honest paramedic**; and `erasure.ReconstructStripe` is **already correct** and sits on the **genesis path**. **STILL GATED:** the loss witness, behind `R-PROBE-FALSE-NEGATIVE-RATE` — a repair **erases its own evidence** (**T-LOSS-IS-A-TRANSIENT**) and a witness must be produced on a prompt that is **never itself the evidence** (**T-WITNESS-NEEDS-A-PROMPT**). **★ THE "~4 IN 10 OBJECTS" FIGURE IS DECLINED** — the honest form is 4 of 10 **residue classes**, a rate over objects only under a publisher-steerable size distribution (build-immutable #3); assumption-free: **every object of 4 chunks or fewer — 1 MiB at the default chunk size — is entirely unjudgeable**. **★ NOBODY MAY WRITE THAT THIS IS SOLVED:** the mechanism makes the bounty correctly **METERED** and leaves it **MIS-ATTRIBUTABLE**, held in tension at `owned-residuals.md` D7. On `main` today the judge's two legs — recompute the claimed position (`VerifyByRecompute` REQUIRES k survivors; **the judge FETCHES n−1**, see the record correction on F7), and challenge the named holder — are **both true for a shard that was merely COPIED**; nothing on the judge's path checks that the position was ever lost, and `settleRepairVerdict` pays `PayBounty(claim.Root, claim.Holder, bounty)` through a ledger parameter named `repairer`. The name and the argument disagree, and the argument is what pays. **Reached blind by two seats** (Economist from incentive analysis, red-team from attack surface) | **★ THE COST: THREE TESTS ARE ONE UNIT OF WORK, NOT THREE.** `TestRedteamRepair_HonestClaimIsPaid` is the shipped POSITIVE control; it stages a real shard with `stageShardOn` and asserts `BountiesReleased == 1` — **under the ratified intent that arrangement must pay NOTHING**, so a test named for the red team encodes the defect as correct behaviour and has been green. `TestRedteamRepair_GarbageClaimIsSlashed` and `TestRedteamRepair_ComputeButDontStoreIsDenied` are the two NEGATIVE controls, and their non-vacuity rests on that positive control — stated in the positive control's own doc comment. **Correct it and both negative controls lose their witness in the same commit.** So: build a positive control over a REAL loss (a stripe position actually missing, then rebuilt) and re-point both negative controls at it. **Do not resolve the contradiction by deleting a test.** **★ RATIFIED 2026-09-12** (`D-RTRC3-INTENT-STANDS-2026-09-12`): **RT-RC-3 states the intended rule**, and `TestRedteamRepair_HonestClaimIsPaid` — the SHIPPED positive control — encodes the defect as correct and has been green throughout. The contradiction the pin refused to reconcile is resolved in the pin's favour, and **the three tests remain ONE unit of work**: correcting the positive control costs both negative controls their witness in the same commit, which the positive control's own doc comment states. **NOT AUTHORISED TO BUILD, and not in the PR that ratifies it.** **ACCEPTED RISK:** the closer sits behind the GATED loss witness (`R-PROBE-FALSE-NEGATIVE-RATE`, `T-LOSS-IS-A-TRANSIENT`, `T-WITNESS-NEEDS-A-PROMPT`), so building it early lands a fixture asserting an intent the mechanism cannot yet deliver — a test that passes by arrangement rather than by mechanism. Sequence it THROUGH that row, not around it. | Researcher certifies → **OWNER** ratifies the mechanism → Builder |
| F9 | The three website pages stop being committed and are generated at deploy | **RATIFIED CONDITIONALLY 2026-09-12** (`D-WEBSITE-HTML-AT-DEPLOY-2026-09-12`), scope **all three together** — `website/changelog.html`, `website/roadmap.html`, `website/buildlog.html`. Doing one is worse than doing none. **The condition was partly discharged by measurement and Netlify BUILDS:** `netlify.toml` carries `command = "python3 scripts/gen_changelog.py"` / `publish = "website"`, so `changelog.html` is already regenerated at deploy and the other two are served from their committed copies. **So this is a config flip, not a migration** | **The condition is NOT fully discharged, and the remainder is named: a Netlify site's dashboard build settings can override `netlify.toml`, and that surface is not readable from inside the repo.** Before the three files are deleted, someone confirms from the dashboard that the repo's build command is the one that runs, and that one deploy produces all three pages. **Deleting first and checking after is how a public site goes blank.** Then: two more commands in the build step, three files deleted and gitignored, and the three *"Fail if … page is stale"* CI steps retired with them. `ROADMAP.md`, `CHANGELOG.md` and `docs/buildlog/*.md` stay the single sources of truth — this removes a copy, not a source | Builder, after the dashboard check |
| F10 | **The VDF minimal-encoding rejection** — `Answer.VDFY` has two different equivalence relations imposed on it by two lines four apart, and the prover chooses which one it exploits | **RATIFIED 2026-09-12, TAKEN** (`D-VDF-MINIMAL-ENCODING-2026-09-12`, certification finding Q1′). **REFUTED as a format item on all four doors**: no block field, no cbor key, no `Hash()` preimage change, no committed leaf, no era-activation rule — **its deadline is the readiness stamp raise, not the freeze.** `vdf.Verify` judges the INTEGER (`big.Int.SetBytes` absorbs leading zeros); `bond`'s effective-nonce derivation judges the BYTES. Prepend *j* zero bytes and π is unchanged, `Verify` still accepts, and the prover draws a different possession challenge **at zero VDF cost**. Priced and bounded: it converts *"shed ~0.2 % of a plot"* into *"~8 % at a 2⁴⁰ grind"*, capping near 12 % even at an absurd 2⁶⁴. **NOT an M0 break** — N identities still cost ~0.9 × N × real storage — but ~8 % lands inside the band the code's own measurement calls *"~free"*, where the complementary answer-latency signal is also blind | Ship it. **It is free**: honest provers already emit minimal encodings, so requiring minimality in `Verify` is purely NARROWING and changes zero honest bytes — no re-mint, no re-run set, no fixture movement. **It must NOT ride the next genesis move**; attaching a free fix to a gated one delays the free fix behind the gated one. The discipline is one package over: copy `core/blindtoken`'s canonical-representation check (range PLUS minimal byte encoding). Gate seen RED first on the pre-fix tree, with the patched file diffed against its original before the run is believed. **It closes ONE of the two spellings defects**; the ±1 coset is a different defect, it DOES change honest bytes, and it is not taken here — `R-VDF-COSET` below | Builder |
| F11 | **`BondVDFDelay` KEEPS its shipped value of 1000** | **RATIFIED 2026-09-12** (`D-BONDVDFDELAY-KEEP-1000-2026-09-12`, certification finding Q2.4 GATED). **The route to 1000 does not exist, and no route to ANY value exists, because the delay property has no consumer:** the only stated rationale is a TEST-SPEED argument for a value that is now genesis-bound; a design doc's claim that raising the delay *"widens W"* has **no implementation** (the anti-release window is a bare literal and the derived floor is `2 × (window × plot seal throughput)`); all three latency comparisons in the bond-audit path are UPPER bounds, so nothing reads a lower bound; and the delay CANCELS in the one argument that looks like it should depend on it. **KEEP is the honest act**: `BondVDFDelay` rides `ConsensusParams`, so moving it moves the genesis hash, and substituting one underived constant for another costs a re-mint and buys nothing certifiable. **This is not a finding that 1000 is right — it is a finding that changing it is not currently justifiable**, which is weaker on purpose | The two residual rows are FILED below (`R-VDF-DELAY-INERT`, `R-VDF-DELAY-VS-A5`), each naming what would make 1000 wrong rather than merely saying the constant is underived. **Owed and unrun: the three benchmarks the certification gates on, named in it with their exact commands — NONE has been run.** The existing VDF test modulus is far smaller than the field modulus, so any timing taken on it is wrong by roughly 3×, and the shipped evaluation runs its squarings and its proving pass serially, so a whole-evaluation figure double-counts. Also owed, as a doc correction: `R-CFGBIND-SEVENTH` | Tester (the three benchmarks); Builder (the doc correction) |
| F12 | **An unauthenticated GET with write side effects** — `cmd/silt/ui.go`'s guard gates on the HTTP METHOD, and the route table contains a GET that mutates | **DIRECTION RATIFIED 2026-09-12, build owed** (`D-UI-EFFECT-GATE-2026-09-12`). **Reach stated without inflation:** the UI is off by default (`-ui`), the guard already refuses a non-local `Host` and a non-allow-listed `Origin`, and the token is required for every mutating method — what is open is a same-origin or allow-listed cross-origin GET carrying no token, so this is **CSRF-reachable, not internet-reachable.** `GET /api/fetch` falls through to `NetGetRetain` on the main node: it pulls missing shards into this node's store, mints storage proofs, registers them under their placement keys and **announces**. Every one is a write, and the handler is reached with no token | **Gate on EFFECT, not on method** — the route table declares whether a route has side effects and the guard demands the token from the declaration. A route that mutates and forgets to say so should be the case that FAILS, not the case that passes. **Do not fix this by making `/api/fetch` a POST**: that moves one route across a wrong predicate and leaves the predicate wrong for the next one. **★ AND FIX THE TESTS, WHICH CHECK DISCLOSURE AND NOT SIDE EFFECTS:** `ui_guard_test.go` wraps a trivial handler, so *"reached"* is its only observable and `TestReadNeedsNoToken` **asserts the defect as correct behaviour**; `ui_privacy_test.go` routes real handlers but asserts which JSON KEYS appear, and a handler can withhold every private field and still retain, register and announce. The gate to add is an EFFECT assertion — drive an unauthenticated GET at a routed side-effecting handler and assert the store, the registry and the announce path are untouched. Seen RED first | Builder |
| F13 | **The certified fork-choice sentence is rendered as a STANDALONE sentence at every site** | **RATIFIED 2026-09-12** (`D-FORKCHOICE-RENDER-2026-09-12`), closing the one call `D-FORKCHOICE-CLAIM-2026-09-12` left open. **The certified bytes do not change and nothing here re-opens the certification.** Today the sentence is spliced into a comma list of built primitives, so its first word parses as a list item of its own and a reader meets a published claim mid-enumeration | Re-render at each ratified site, and **only then** widen `TestO3T_CertifiedForkChoiceSentenceIsPresent` from `README.md` to the ratified site set — **widening the pin before the rendering lands goes red on six files by construction.** The pin is scoped to one file today because one file is the only place a verbatim paste is grammatical; a standalone rendering removes that obstruction and gives the published claim one machine-checked face everywhere rather than in the front door only. **NOT ratified: any edit to the sentence** — not to fix a typo, not to fit a line, and not to make a test go green. Editing `o3tCertifiedForkChoiceSentence` to match a page inverts the gate and re-opens the certification silently | Builder |
| F14 | **D3 fetcher privacy is RE-LABELLED now and WIRED post-RC; and the reachability gate's scope extends to `docs/decisions.md`** | **RATIFIED 2026-09-12** (`D-D3-RELABEL-2026-09-12`). **Nothing shipped changes: no code, no format surface, no consensus rule.** ⚠ **This "D3" is `D-DEMAND`'s D3 issuance-mixing slice, NOT Lane D row D3 (the freeze act). They are different things that share a label; do not merge the rows.** `D-DEMAND` books D3 as *"◑ slices 1+2 BUILT"*. The code exists, is correct as far as it goes, and is proven over real TCP — **it is on no production path.** Verified: `client.WithdrawDemandTokenPrivately` has exactly three call sites, all in its own test; **nothing in the tree imports `github.com/nerolabs/silt/client` at all**; and `core/node/relayrole.go` names the DURABLE identity as the funding that settles. So the ledger books a privacy property as partially built while the shipped binary funds a session through the identity the property exists to unlink. **A record defect, not a regression** — `D-DEMAND`'s blind-signed-serial and demand-neutrality clauses are unaffected | Two edits, neither done in the entry: **(i)** re-label `D-DEMAND`'s D3 clause as built-but-inert with its production route named as owed; **(ii)** the register row naming the inert route — **FILED below as `R-D3-ROUTE-INERT`.** Wiring is post-RC and is not a freeze item (no block field, no cbor key, no committed leaf, no validity rule). **The second half: a reachability lane record may now anchor its `label` in `docs/decisions.md` as well as in `docs/release-checklist.md`** — this ledger is the one place a BUILT claim carries weight and cannot be machine-checked, which is the same shape the first half corrects. `scripts/check_reachability.py` still resolves against `docs/release-checklist.md` only (verified at `cc0cb35`), so the widening is OWED, not built. **Two limits ratified with it, neither optional: reachability is NECESSARY AND NEVER SUFFICIENT** (a green run proves a symbol survived linking, nothing about correctness — the gate's own table passes a gutted `v5ValidateSlashes`), and **extending the scope is not building it** (the D3 route has no symbol a record could name until it has a production caller) | Builder |

#### Lane C — Boulder 2: the economy · the RC's SUBSTANCE (promoted 2026-09-08)

The economy is the M1 keystone and is independent of the floor-box recompute. Every paid lane is
built and dark; the RC ships it default-OFF (`D-RC-SCOPE-S1`) and the B8 pass attacks it ON in the
harness. The cross-server double-redeem money pump is CLOSED on main by R0.4b (`2ad9bd5`, per-epoch
issuer-key expiry; open-break gate PR #700 GREEN) — the per-serial delivery guard `fcbab7e` the PE
note cites is an unmerged alternative on a worktree branch, not owed.

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| C1 | R2.9 tail — retire the `core/demand` v2 flat primitive and the ledger's flat delivery leg, leaving the anchored session lane (open → settle) as the ONLY paid-delivery path; the durable paid-serial guard record gains its lane byte | **MERGED 2026-09-08 (#780)** (`builder/c1-demand-v2-retirement` @ `a290bff`): production shrinks ~580 lines (`Bank.Redeem` + the v2 spent set + `DeliveryReceipt`/`SubmittedReceipt`; `RedeemDeliveryCredit[Reason]`); 22 test files re-homed onto the lane, each property with a driven ablation, every red-team scar (cross-server double redeem, the A4 money pump, the self-financing eviction pump, the epoch binding) re-derived RED at its historical broken value; the guard record bumps to format v2 (magic + version header, lane byte last) and REFUSES TO START on a pre-bump file rather than guessing the lane — the guess is the mis-count being closed, and it would persist into the file at the next compaction. The blind PE (`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-c1-demand-v2-and-flat-leg-retirement-a290bff-2026-09-08.md`) ruled the retirement clean (no property lost; every scar re-derived RED at its original broken number; the supersede rewrite holds its property) and found the format bump's one real defect: the store type has TWO consumers, and the refusal told the operator that removing the file costs nothing — true for the paid-serial log (the ledger is ephemeral pre-RC), FALSE for the credit-spent log, whose publish key persists, so clearing it alone re-opens every held credit for a second spend against ratified owner call 6. Closed at `ee81518`: the adapter's refusal names NO remedy and the daemon attaches the per-store one (rotate the key AND clear the log, together, for the credit-spent store), driven by a runtime arm, a source arm and a controlled swap; an empty pre-bump file upgrades silently, a written one refuses, and the refusal leaves the file byte-for-byte intact so the ratified remedy stays available | PE re-check of the delta, then merge; the design record's six drifted lines are corrected in the same PR | Builder (done); blind PE |
| C2 | The delivery idle window + the wall-clock reaper (`R-REAPER-FORFEIT`) | the five R2.9 owner calls DECIDED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (4)–(8)): REFUSE-UNTIL-SET stays until the ≤ f+1 bound is field-confirmed at the re-priced tiers; the disclosed v1 residuals (the wall-clock step, the anchor stall, the operator-managed `creditspent` ceiling with its rotate-and-clear rule, the relay-wash position) are DISCLOSED in `docs/design/m0.md` §10 (folded 2026-09-08 — the doc PR this row owed is done) | **BUILT 2026-09-09** (`builder/c2-idle-window` @ `11aba7b`, PR pending; blind PE MERGEABLE-WITH-ONE-CHANGE, closed): the window ships as a DURATION of **24m** with a start-up floor of `bound × 4/3` = 9m33.33s, so the daemon now REFUSES a window that cannot survive the worst stall the model admits. The Tester's derivation, re-derived independently by the PE: **430 s governs, not 190 s** (the re-keyed takeover bound — a defensive window dominates the worst admitted case, not the modal one), and the last-settle stamp is floored into `idle/4` buckets, so GUARANTEED survival is 0.75 × idle — sizing at the bound would have left it a quarter short. The floor is tight to the nanosecond. **VALUE RATIFIED 2026-09-09 at 24m** (`D-C2-IDLE-WINDOW-VALUE`; cost verified ZERO — the deposit release epoch, 30m35s, binds at both candidate windows). Call (4)'s arithmetic was epoch-denominated against 190 s while this ships a duration against 430 s: same magnitude, different reasoning — so the owner opened a second question, **what ELSE was derived by that route**, and a blind PE audit of silt's derived parameters is running against three faces (modal-vs-worst bound, an uncarried quantization step, an undriven coupling premise), partitioned by whether the parameter FREEZES at D3. Its findings are an input to D1. **Three findings the row did not anticipate:** (i) the graded harness had been running the paid lane at 90 s — a guaranteed survival of 67.5 s, under even the 190 s tier the same run confirmed — fixed at both sites; (ii) a harness scenario polled for a session-close marker that is OBSERVABLE at 90 s and IMPOSSIBLE at a correctly-sized window, so the default alone would have left the sheet requiring a line that can never appear (the row read *"inert while era-4 is dark, a FAIL at the stamp raise"*; **the era-4 half of that premise is VOID as of 2026-09-11** — era-4 is live from height 1 on the shipped default — and the disposition survives on the ground that actually carries it: the line is impossible at a correctly-sized window, whatever the era) — requirement dropped, the property re-homed to e2e, and row 13b's M0 audit made conditional so it is no longer vacuous; (iii) **a premise in canon is REFUTED by a driven run** — the wall-clock-step residual said a chain STALL reaps every live session and both this row and ratified call (4) were sized on it, but with the chain frozen 1040 s the lane admitted a session, settled 104 receipts, took a top-up and the session LIVED; nothing on the settle or fetch path reads the chain. The number survives, its STATUS does not: 430 s is a conservative envelope adopted because call (4) instructs a window above the bound, not a bound the reaper races. The surviving half (a forward wall-clock STEP does reap) was itself unmeasured and is now driven | Tester (derivation + gates, done) → Builder (done) → blind PE (done) → **OWNER ratifies the value** |
| C3 | R2.2 full observability set (serve-work AND repair-work Gini, per-tier margin, live `g`, funded-horizon, wash detection; network panels via the DHT crowd-estimator with knowability tiers) | **MERGED 2026-09-09 (#784)** (`builder/c3-r22-observability` @ `f03ab50`): 15 of the 17 rows built, 1 already on main (row 3, the prepay/skim split, shipped as C4's A4-1), 1 ruled NOT KNOWABLE (row 5, below). Four read-only routes in `apiRoutes()` (`/api/economy/flows`, `/g`, `/concentration`, `/network`), the four panels, a bounded flow ring, live `g` per cared object with `known`/`reason`, serve-work Gini over the whole sample and repair-work Gini WITHIN the repair-capable subset, the C2 metric beside them, and the observed tier ratio. Two gossip fields (`servedBytes`, `repairsDone`) ride the EXISTING capacity pledge under the same condition, so the bound is the existing peer-info eviction — two int64s per existing entry, NO new peer-keyed map — and the work sample IS the capacity sample, which is what lets row 10 derive the tier class instead of gossiping a third field. Bands published with their source: pony < 16 GiB, horse < 1 TiB, archival ≥ 1 TiB. `minGossipSample = 3` is derived, not chosen: at n = 1 a Gini is 0 by construction and at n = 2 it inverts to the ratio of two named peers' counters, so the floor is an honesty floor and a privacy floor at once. The advisory's vacuity correction is MEASURED, not quoted: network-wide repair Gini 0.9091 against 0.0000 within the capable subset on the same healthy shape. 13 ablations RED — including one the Builder caught vacuous in its own gate (it compared encoded frame LENGTHS, which grow equally under the revert, so it stayed green; replaced with an exact key scan) | **Row 5 (network aggregate `g`) is DECIDED BY THE DESIGN, not owed as an owner call:** `g` is cost-per-repair, rows 8–9 carry the denominator (repairs-done) and nothing carries the numerator (credits paid out), and row 10 forbids a third gossip field — so the figure is NOT KNOWABLE from the surface this row may build, and §0's own rule ('never publish as fact') plus row 13's ('field absent is a legitimate rendering; a guess is not') settle it. It ships as an explicit not-knowable carrying its derivation on the wire, never a `network` block. Re-opening it means a design case for a third gossip field, which is a new scoped item, not an escalation. **Both blind seats independently CONFIRMED a privacy break, and the gossip half is GATED at the research gate — OWNER RATIFICATION REQUIRED before it merges.** The break (blind PE `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-c3-r22-observability-f03ab50-2026-09-09.md`, red-team `/Users/andrewedmond/.claude/silt-agent-memory/red-team/reviews/REDTEAM-c3-gossip-disclosure-f03ab50-2026-09-09.md`): identity is free, so an attacker furnishes n−1 of the n sample terms and the published Gini plus its sample size is ONE EQUATION IN ONE UNKNOWN — a working PoC recovered a planted 7,777,777 exactly, and the recovered term is the counter `-privacy=on` (the compiled default) withholds from that reader. The sample-size floor cannot fix it: it bounds sample SIZE, not terms unknown to the reader, and the attacker sets the size. It amplifies to a PEER ORACLE (the recovered term need not be self), a per-node activity TIMELINE (the counters are monotone, so resampling yields a rate), and a repairer FINGERPRINT for eclipse targeting. Certification `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/C3-GOSSIP-DISCLOSURE-vs-D-UI-PRIVACY-FLAG-RESEARCH-CERTIFICATION-2026-09-09.md`: **GATED**, certifiable only as alternative A (gossip the two fields ONLY under `-privacy=off`) under three conditions — a wire gate ablated with an exact CBOR key scan (never a frame-length compare), non-reporting read as UNKNOWN and never as zero (both fields are `omitempty`, so withheld and idle are identical bytes and today's zero drives the serve Gini toward 1.0, a FALSE total-capture reading on the default posture), and self's own withheld counters kept off the open route. The ratified `D-UI-PRIVACY-FLAG` containment survives on the letter and is voided in substance: the gossiped value is the SAME INTEGER as the withheld one, `D-STATUS-SNAPSHOT-INTERVAL` ratified the 5 s snapshot as a security parameter precisely because a reader picks its own poll rate, and the audience widens to nodes with no HTTP surface at all — the exact node the flag was ratified to protect. A coarsened band or rate is REFUTED (a band still yields a monotone step sequence under probing; a rate publishes the derivative the attack must work for). Under alternative A the panels are NOT empty: the C2 metric, the tier mix, the bands and the target ratio survive; only the two Ginis become a named absence. The owner ruled the product half in advance — privacy default WINS, empty panels preferred — and the exact sentence to ratify is §7 of the certification. **The concentration gate is now a THREE-VALUED statistic, and two of the advisory's own thresholds are WITHDRAWN by the Economist that wrote them** (`/Users/andrewedmond/.claude/silt-agent-memory/economist/reviews/ADVISORY-c3-concentration-gate-thresholds-redderived-2026-09-09.md`): the Tester's fixtures showed a bare `serveGini ≤ 0.15` PASSES total capture, because the certified privacy exclusion drops silent peers from the series — five nodes serving 100 % with 1006 silent publishes `0.0000, known:true`. The fix is one adjusted statistic `G_adj = (1−c) + c·G_pub`, EXACT on that fixture (0.9951 = the true Gini), with the coverage floors derived as a theorem of the tolerance (`c ≥ 1−T`, searched over 101,000 pairs) rather than chosen; a separate coverage clause would still admit a true Gini of 0.2775 serve / 0.6400 repair. The verdict is PASS / INDETERMINATE / CONCENTRATED, because reporting "I cannot see" as "you are captured" is its own defect. `repairGini ≤ 0.40` is WITHDRAWN as a field threshold and encoded as a live pin instead: under holdings-proportional repair the honest null is 0.7424 at 11 capable nodes and 0.6282 at the canary's own minimum, so **an honest canary would abort** — and 0.7424 is indistinguishable from the 0.7505 the advisory had labelled CAPTURE. `serveGini ≤ 0.15` is demoted to a fixture constant: its provenance came from a 10,101-node distribution while the sample is capped at 4,096, and the falling curve is not a sample-size effect at all — remove the single archival node and the honest figure is scale-invariant at 0.0810 across two orders of magnitude, so the alarm wants to be MIX-aware. Two clauses are withdrawn as unbuildable: `ponyShareOfServedBytes` has no source (the build item is a per-tier work total; the trap is that the tier mix's `share` is a share of NODE COUNT and reads 0.9891 where the true byte share is 0.1998, so wiring it to the T-AR floor passes total capture — pinned), and `caretakerSetSize ≥ 3` has no published surface and no definition of a cold root. Left: the Tester lifting the red-team PoC as a permanent regression gate — DONE, the recovery solve is now VALUE-shaped (it walks every numeric leaf, after the blind PE republished the same figure under a different name and recovered the secret while the suite stayed green) — then **the owner ratifies §7 of the certification and it merges** **The per-tier work totals — the item that re-founds the two withdrawn thresholds — are BUILT** (`builder/per-tier-work-totals` @ `12cda11`, PR pending): per-tier serve/repair/pledge/reporter counts on the sample and an edge share of SERVE WORK on the concentration document, so the edge-majority tenet finally has a source. On the concentrated fixture the two candidate numbers give OPPOSITE verdicts off the same product — the tier mix's node-count share reads 0.9891 and PASSES a majority floor while the true work share reads 0.1998 and VIOLATES it, a 4.95× inversion. Both floors are DERIVED, not chosen: the per-tier coverage floor falls out as a theorem (with φ the least per-tier coverage, `φ·s_obs ≤ s_true ≤ s_obs/φ` and `s_obs ≤ 1`, so a PASS implies `φ ≥ F` — the minimum coverage a PASS needs IS the tenet floor), and the per-tier reporters floor is the SAME constant as `minGossipSample` reaching a population it had not been applied to. The old sample-wide coverage clause was REMOVED rather than kept as a belt: it is dominated and wrong where the two disagree. **Everything is stated as PRICED, not CLOSED** — the reporters floor buys parity with the Gini beside it rather than closure, and the coverage interval PRICES the silent-concentrator attack at one decoy node per silenced band rather than defeating it (measured: φ 0.5000 → 0.5455 and the floor 0.5000 → 0.4969, and the gate passes a network whose edge tier truly serves 0.1997) | Builder (done) → blind PE + red-team → Tester |
| C4 | R2.7 blocking telemetry — the A2 (supersede-suppression) and A4 (money-pump) detectors | **MERGED 2026-09-08 (#781)** (`builder/c4-r27-telemetry` @ `bc7050a`): three detector families, each with a driven ablation — A2 the served-byte partition across terminal states with the unwitnessed residual DERIVED, never stored (one write path per state; the coverage denominator excludes chunks that can never be witnessed, so a node serving manifests is not penalised); A4 the three-part replacement (the literal `bountyPaidToEscrowFunder` is DEGENERATE — every escrow funder on a per-node ledger is that node), namely the prepay/skim split of `funded`, `BountyPaidToSelf` keyed on the claim holder, and the prior-fetcher payment flag; the affordability floor at BOTH spend gates with distinct identities counted by one bool on the account, never a side set, surfaced beside `grantsDenied` on `/api/status` including the no-faucet branch. Two calls beyond the advisory, recorded in `docs/thinking/2026-09-08-c4-r27-blocking-telemetry.md`: the withheld document rebuilds its revenue block with the two new fields zeroed (the allow-list passes the block through by pointer, so a field added inside it would have shipped open — an F2 join with `/api/roots` on a one-root node), and the floor counts on the unconfigured-faucet branch too (the literal placement would have dropped a real refusal count on the DEFAULT posture — silent loss). C1 removed one terminal state the advisory assumed, so the identity has three terms, not four; a fourth counter would be a permanent zero | both reviews IN: blind PE MERGEABLE-WITH-CHANGES (`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-c4-r27-blocking-telemetry-93deb56-2026-09-08.md`) — the privacy fix was the one thing in the diff with NO gate (reverting it left the whole `cmd/silt` suite green: the existing wire scan compares VALUES and is blind to any field its fixture leaves at zero), and the floor was counted at 2 of 3 spend gates; the Economist found the same third gate independently and ruled the set SUFFICIENT WITH GAPS NAMED (`/Users/andrewedmond/.claude/silt-agent-memory/economist/reviews/ADVISORY-c4-r27-telemetry-as-built-93deb56-2026-09-08.md`). All four items closed at `bc7050a`: a sibling whole-surface scan whose fixture pays a bounty (the existing scan is NOT re-pointed — at `paid == 0` three per-object figures are aliases of `funded`, so driving a bounty through it would trade three-quarters of its coverage for this one field); the third gate counted at the refusal DECISION in `registry.Gated.Publish`, never inside the `CanPublish` predicate, which is read for display (the rejected design is itself an ablation arm); and the delivery-lane state published beside the coverage figure, so a machine-read abort can tell an honest RC-default node from a suppressing one. Delta re-check, then merge | Economist specified → Builder (done) → blind PE |
| C5 | Pre-flip closers — the two CODE closers are MERGED (PR #787, with `D-GENESIS-MOVE-2` ratified); the bearer-anchor red-team pass is what is left. (The two residual names left the register with their closure, per the filing rule.) | the family CERTIFIED 2026-09-07 (`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md`): the λ-dust bound CLOSED (24.00 GiB escrow leg, exact); parity amplification DISCHARGED (the per-server floor corrects DOWN to 27.94 GiB, so the 64 GiB `grant/r` pin gains margin 1.43× → 2.29×); the refuse-and-self-spend and relay-anonymity faces are held in tension (`m0.md` §10); the price-level and relay-wash positions DECIDED 2026-09-07 (owner call 13) and carried INTO the flip — re-confirmed at R2.4 only if C7 contradicts them | **BUILT 2026-09-09** (`builder/c5-preflip-closers` @ `a68701c`, MERGED as PR #787; blind PE MERGEABLE-WITH-THREE-CHANGES at `399f842`, all three taken — `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-c5-preflip-closers-399f842-2026-09-09.md`). The bounty division now happens AFTER the rarest-shard multiplier at the one path that pays, the second pricing function is deleted rather than left beside it, and the publish warning prices the OBJECT rather than the flag — which made the rule smaller, deleting the flag-was-set parameter, since an unset chunk size IS the default and can only land silent. A single-frame object's parity is computed at its TRUE length. **The finding neither source named:** the PoR auditor sizes its challenge from the committed chunk size and demands it EXACTLY of every leaf (deliberately — that exactness IS red-team finding F4), so a short shard under an unchanged committed size makes the auditor refuse every HONEST holder and every sub-frame object silently read as LOST; closed by carrying the frame size actually used in the EXISTING committed field, no format change and F4 intact. **OWNER: the genesis moves a second time and the ROOT moves with it** (4′ re-framed only the manifest, which the root does not cover) — root, manifest chunk and block hash all re-pinned; the first break was accepted explicitly and this needs the same. The blind PE re-ruled twice more; both further findings were the SAME shape and both times the CODE was right — a published sentence overreaching the code's actual boundary while every gate stayed green (the re-addressing boundary was stated one byte wide, and the warning's silence rule claimed a default publish can never warn when in fact every object ≤ 262,119 B does — the concern is TRADED, not satisfied). Both corrected and re-measured independently. A real bug fell out of the second: an EMPTY file printed a 24-byte shard that does not exist, and the gate had EXCUSED size 0 rather than covering it — the row is now driven. Left: **the OWNER accepts the second genesis move** (the root moves this time), then merge | Builder / red-team |
| C6 | R2.4 the economy-ON default flip — phased: correctness gates → economy-OFF baselines over ≥ 3 epochs → canary (window = 1 epoch ≈ 5.9 min at the measured `T_b`, hysteresis 3 windows; HARD aborts: any `guardFullRefusals`, caretakers < 3, `BountyPaidToSelf` > 0, any rise in `spendRefusersDistinct`; soft aborts: coverage < 0.75, pony serve-share < 0.50 or −20 pp, `serveGini` > baseline + 0.10, `repairGini` > 0.40, funded horizon < 600 s, evicted/object-aware > 0.01) → default | DEFINED as a CHECKLIST 2026-09-07 (the advisory, §3) **⚠ TWO OF THE CANARY'S ABORTS DO NOT WORK AS WRITTEN (Economist, 2026-09-09, `/Users/andrewedmond/.claude/silt-agent-memory/economist/reviews/ADVISORY-c3-concentration-gate-thresholds-redderived-2026-09-09.md`).** (i) `repairGini > 0.40` is WITHDRAWN as a field threshold: under holdings-proportional repair the honest null is 0.7424 at 11 capable nodes and 0.6282 at the canary's own minimum topology, so the abort kills the FIRST HONEST CANARY — and 0.7424 is indistinguishable from the 0.7505 the advisory had labelled CAPTURE, because its "midpoint" spanned two healthy shapes rather than a healthy and a captured one. `serveGini > baseline + 0.10` is demoted the same way. (ii) **On the SHIPPED DEFAULT no node gossips work counters at all** (`cmd/silt/daemon.go` welds the publish flag to the privacy default, which is withheld), so under the certified alternative A both concentration series are EMPTY and every concentration-based abort is inoperative — silt can see who is PRESENT and never who does the WORK. The owner chose empty panels over the disclosure knowingly; the consequence for this row is that a canary graded on a default fleet has no decentralization signal, and every concentration baseline to date is measured on an opted-out topology. **DECIDED 2026-09-09 (`D-WORK-VISIBILITY`): decentralization is graded in the HARNESS for the RC — no production concentration alarm is claimed, because none can fire on a default fleet. This row's concentration-based aborts are RE-POINTED at the harness and must stop implying a production abort that cannot fire; the withdrawn `repairGini` threshold is not reinstated. The committed-ledger route (concentration over committed state rather than gossip — the strongest answer, since it removes the disclosure AND the self-reporting) is DEFERRED to as late as possible before the cut: it is research territory that may spin, and defined work comes first** | after C1–C5; the flip is the FIRST of the three `D-FP2-SCOPE` re-arm triggers (the ephemeral-ledger items re-open with it); OUTSIDE the RC by `D-RC-SCOPE-S1` — a `0.9.x` release after C7's clean verdict, before `1.0.0` | Builder; Economist |
| C7 | R2.7 economy-ON adversarial-solvency verdict (five inequalities × seven attacks; A2 supersede-suppression and A5 cold-start capture unpriced) | SCOPED; after C3 + C4 + C6. **Scoped down 2026-09-08 by the Economist's as-built read of C4** (`/Users/andrewedmond/.claude/silt-agent-memory/economist/reviews/ADVISORY-c4-r27-telemetry-as-built-93deb56-2026-09-08.md`): the five solvency inequalities ARE gradable on C4's counters (A4-1's prepay/skim split is the two-sided one that grades S5, bounding recoverable money at the skim leg); **A4-2/A4-3 are FLOOR detectors defeated by one extra keypair costing zero credits** — no counter without an identity axis closes that, and the axis is the access record Don't #3 forbids, so R2.7 must state that limit rather than grade past it; **A5 (cold-start capture) is half-instrumented** — it needs a cold-vs-exhausted split on the refusal (one more bool); **A6 (the edge legitimately refuses cold content) has NO instrument in any shipped row** and must be SCOPED OUT of the verdict until C3 lands rather than graded on nothing. Coverage is a solvency estimand, never an attribution instrument — churn and suppression read identically because economically they ARE identical — so it is read as a band with the abort on the upper bound | Researcher NEW-CERT + red-team pass; feeds the R4.4 brief | Researcher; red-team |
| C8 | R2.8 cold-repair funding path (reserve-aware scheduling + early cliff disclosure to ANY funder + R2.9's expiring remainder; no network pool, never a mint) | DEFINED 2026-09-03; after C3 | one builder session; five inputs stay ASSUMPTION until live data | Builder |
| C9 | Measurements owed: `B_bootstrap` + the honest arrival rate on REAL traffic (the flixz export; the commissionable handoff paragraph is the 2026-09-07 certification's §7 — cumulative per-server draw, one-sided falsifier: only a measurement ABOVE 64 GiB is decisive) · `M_seen` pony-class (the class-M streaming verifier's TIME ceiling) · G-λ-11 the receipt-lane operator cost before `PF` may move | OPEN | the flixz handoff (T-1 first, G-BB-5 answered first); two Tester measurements | Tester / external |
| C10 | Small owed PRs: `R-COMPACT-ORPHAN` WARN line (surface `CompactFailures` / `LastCompactError` on the banked/status path; WIP `builder/c10-compact-orphan-warn` @ `49eb5d1` — the WARN line with both ablations RED, suites not run) · `R-PRIVACY-OPERATOR-TAB-TOKEN` (a persistent operator-token route before the `-privacy` default reaches real operators) · R2.14b `MsgRelayFund` (top-up with fresh anchors on an admitted session) | OPEN, LOW | one small PR each | Builder |
| C11 | **Relay settlement is all-or-nothing at session close — the successor mechanism** (`R-REAPER-FORFEIT` relay half). The lane settles ONCE at close and `sweepRelaySeen` drops a session at `admitEpoch+2` UNSETTLED, so an over-running session forfeits 100 % of earned credit with the fetcher's face already spent at open; clearing one 24.41 GiB face inside a 413-734 s session needs 286-508 Mbit/s sustained, so at a 100 Mbit/s edge uplink a node moves 4.81 GiB and is paid nothing | **DESIGN DEBT, OWNED 2026-09-09** (`D-RC-POSTURE-2026-09-09` (2)). The v1 POSTURE is decided and shipped: default-OFF at every tier with the settlement math disclosed in `docs/design/pod.md` §7.3 and in the flag's own help, framed as *the settlement model is unfit for the edge tier* — because a lane that only settles horse-and-above concentrates revenue on the few and punishes the pony tier the 10000/100/1 thesis rests on, which is an economic-recentralization vector against build-immutable #3, not a bad default. The owner affirmed it was correct NOT to build the fix inside C2 | Choose and certify the mechanism: incremental settlement (pay per forwarded increment rather than at close) OR a periodic relay sweep (the delivery lane has `SweepDeliverySessions` plus a daemon ticker; the relay reap is lazy with ONE production caller, `OpenRelaySession`). It moves an economic rule, so it is research-gated: Economist advises, **Researcher certifies, OWNER ratifies**. Until it lands, relay is horse-and-above by construction | Economist → Researcher → owner |

#### Lane D — Boulder 3: the freeze (era-4/v5) = the RC · its dependency on the recompute spine is CUT

The only floor-box property the freeze needs is "it stalls and never accepts" — one driven suite,
not the R1.x ladder. **The v5 digest leaves freeze at FIVE, not three** (`D-MEMBERSHIP-KEEP-FIVE-2026-09-11`
reverses the retirement; *the superseded sentence: "the v5 digest leaves freeze at three"*), and the
format is not touched again: **the FORMAT set is CLOSED as of 2026-09-11 and the OWED-and-FORMAT list
is EMPTY.**

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| D0 | **The cold auditor — the RC's ONLY floor-box requirement** (owner call 2, RATIFIED 2026-09-07, `D-TRUE-UP-CALLS-2026-09-07` (2)): the floor box is a COLD AUDITOR — unconditional loud stall, never Accept | **MERGED 2026-09-09 (#786)** (`builder/d0-cold-auditor` @ `8c414e7`): the box is a cold auditor — the recovery directive type, its heights list and live-follower opt-in, the box config knob, the `trustFloor` scalar on the contract surface and a loose boundary predicate are all DELETED, and the second exported box entry (`WitnessValidateV5`, zero non-test callers) is deleted too — a coordinator scope call OFF the ratified owner-call-2 list, taken because keeping it ships TWO entries with TWO recovery semantics on the RC's last floor-box item. In their place: ONE DRIVEN never-Accept suite over nine block classes — for each, build a block the NODE's own commit path ACCEPTS, forge a divergent `StateRoot`, re-sign, re-certify with the full quorum, assert the box refuses with a NAMED reason — plus a reflection completeness meta-test that reddens if a `Block` field is added and driven by nothing | blind PE MERGEABLE at `72be1f3` (`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-d0-cold-auditor-3539b2a-2026-09-09.md`); its one blocker was that the "unconditional" stall keyed on the block's DECLARED height, so a box at head 2 emitted a TERMINAL re-anchor stall for a block merely CLAIMING the boundary — and the suite could not see it because the fixture collapsed the two quantities. **The coverage hole was inside the completeness test's own excuse row**, whose written reason was measurably false. Closed, both directions independently RED. Three non-blocking findings taken rather than shipped past, all one class — a comment claiming more than the code delivers: the cited-test lint scanned COMMENTS but not STRING LITERALS, so every excuse row and every observable-contract Asserter entry in the repo was unprotected (fixed in the LINT, +91 citations now covered, zero phantoms); the state-view scalar rule was a pattern that was escaped once per form, now a PARTITION with a closed complement (allow-list with an asserted reason, or a result list ending in `Availability`); and a dead conjunct in the boundary predicate. **A FOURTH false claim was found by running the correction rather than reading it** — dropping `LivenessRecoveryHeight != 0` would have made every box stall at genesis forever. Left: merge | Builder; blind PE; Tester (every gate RED first) |
| D1 | **The era-4/v5 freeze manifest** — the ONE ordered list of what the stamp-raising release contains or decides | **RE-AUDIT 2026-09-11 — THE OWED-AND-FORMAT LIST IS EMPTY. D1's FORMAT CONTENT IS DELIVERED.** All 22 items re-classified at `f826c72`: **9 BUILT · 5 DROPPED/DECLINED · 2 PARTIAL · 6 OWED, and none of the six is on the freeze surface** (`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-era4-freeze-manifest-re-audit-f826c72-2026-09-11.md`). Item 1 merged (#819), item 2 DROPPED by owner reversal (#820), item 3 already built (#800), items 14 and 19 built (#816, #808 + #812), item 6 discharged and owner calls A and F both landed inside M1 (#818). What blocks D3 is item 21 (the freeze ENTRY, the owner's act), not a format item. *The superseded count, kept so the correction reads as one: the 2026-09-10 final-content audit at `0ed3b92` held **the remaining FORMAT set is TWO items** — item 1 (`tagRevLogSize`) and item 2 (the digest 5 -> 3 retirement) (`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-era4-freeze-manifest-final-content-audit-0ed3b92-2026-09-10.md`); item 2 was then reversed rather than built, so the train closed at one.* **⚠ MANIFEST ITEM 8 HAS NOW BEEN DELETED TWICE ON WRONG GROUNDS, AND IS RESTORED AS A LIVE REGISTER ROW — `R-CARRIER-BYTES`.** First it left the manifest on a refuted label: `ROADMAP.md` recorded the box witness/frame byte ceiling as leaving because *"it served the recompute"*, while the certification's own §4.10 says the opposite in terms (*"the box is not the binding constraint at any admissible value; the node's CPU is"*). The repair then re-homed it onto **a different field**: this row said the exposure *"is now tracked as `R-CARRIER-ATTS-NORMALIZE` and `R-CARRIER-ATTS-PREPAREQC`"*, and **that is wrong at source.** Item 8 is a ceiling on `Block.LastCommit`, which appears in **both** `bodyHash` preimage literals (`core/chain/chain.go`); `Block.Atts` appears in **neither** — this file says so itself on the `R-CARRIER-ATTS-BLOCKS-CEILING` row. The certified `Atts` fix is **acceptance-time normalization**, and normalizing a hash-covered field changes the block hash and invalidates the block, so that mechanism **cannot apply to `LastCommit` at all.** They are not one defect. `validateCarrier` (`core/chain/carrier.go`) still enforces phase, `verifyAtt` over `b.Prev` and per-id distinctness and **has no count cap, no byte cap and no qualification screen** — verified at HEAD. Five production comments cite `R-CARRIER-BYTES` *"in ROADMAP.md"* (`carrier.go`, `readset_v5.go`, `stateview_v5.go`, `floorbox_recompute_stateroot_v5.go`, `floorbox_recompute_stateroot_atts_v5.go`) and `ErrWitnessBudgetExceeded`'s failure text names it; under simplicity rule 4 the thing they cite did not exist. **That is repaired: #824 restored the row, so all six citations resolve to a destination that exists.** Two residues stay, both in `core/chain` and neither touched here: three of the five name the destination *"Boulder 1 carry-list"* while the row is homed to Lane D2, and `ErrWitnessBudgetExceeded` says the ceiling *"is not built yet"* — a future tense that the 2026-09-11 decline makes wrong, since for the RC it is not built AT ALL. **The register row is restored and the disposition is an OWNER call (below): in the RC, or explicitly declined and disclosed to B8.** Deleting it by re-label is neither shipping it nor declining it. *The prior status, kept:* CERTIFIED 2026-09-07 as a 22-item manifest in four classes with three deadlines (`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md`); the nine §8 sentences **RATIFIED 2026-09-07** (owner call 9): `tagRevLogSize` BOUGHT (a safety leaf — a wrong `m` is a WRONG-ACCEPT; it also lifts the floor box's takedown stall, CLOSED by #819); (d-3) `AnswerDigest` BOUGHT (closes the late-reveal face, CLOSED by #800, and the `Slashes` cap's completeness face); the PoP slot RESERVED inert — **DROPPED 2026-09-10, owner call D**; the `IssuerKeys` cap 4,096 with its packing budget + `(issuer, epoch)` distinctness; `SerialSize` 32; #237 refuse-to-start across a format boundary; the genesis hash is network identity; the tally stays behind `everMature`; no activation mechanism beyond the tally. FORMAT items freeze at the freeze; VALIDITY items land at the stamp raise. **G-1 CLOSED 2026-09-10** — the certified precondition on item 2 (the digest 5 -> 3 retirement), so `R-membership` is UNBLOCKED: the box entry now asserts `objective()` rather than `verifyBond != nil`, covering BOTH arms. The narrower check let a box with a WIRED verifier and `cfg.MinBond == 0` past the entry, after which the maturity recompute reproduced the OBJECTIVE branch unconditionally while a full node at the SAME config took `matureNow`'s LEGACY branch — one config, two verdicts, silently. Driven red-first; the fold-state pin's site allowance MOVED with the assertion and the entry-still-exists property is re-homed to the driven gate (a runtime observation where the pin could only see a read site). Not itself a format change — a stall-more assertion on a never-Accept box. **Deltas of the 2026-09-08 reorder (the Researcher is told in one doc line; the unchanged items are not re-certified):** (i) **SUPERSEDED TWICE OVER — kept as written so the correction reads as one.** *The delta said: "the digest set freezes at THREE leaves — `R-membership` (D-V5-WHOLESET-ROOTS five → three, a hard fork at activation, free while era-4 is dark, with the `objective()` guard that also covers `MinBond`) is the LAST format touch."* **The ruling is REVERSED** — the set freezes at FIVE and G-3 is `R-membership`'s closer (`D-MEMBERSHIP-KEEP-FIVE-2026-09-11`). **And the ground it stood on is separately FALSE**: *"free while era-4 is dark"* is void, because `-era4-activation-height` defaults to 1 and every fresh network mints era-4 from height 1. Neither correction rescues the other; the item is out on the first and its pricing was wrong on the second; (ii) the box witness/frame byte ceiling LEAVES the manifest — it served the recompute and goes to the re-scoped Boulder 1's design consult; (iii) the `proof.Unmarshal` decoder bound (`R-R3-GOB-ALLOC-AMPLIFICATION`) moves its deadline from "the flip" to the stamp-raise train, because the flip is frozen | Builder lands the FORMAT items in one stamp-raising PR train against the manifest's gates; the B8 green list is the manifest's §6. **TWO owner conditions added 2026-09-09** (`D-RC-POSTURE-2026-09-09` (6)-(7)): (i) delegation does NOT cover the format surface — every FORMAT item comes to the owner individually before it merges, green and reviewed or not, because this is the LAST format touch and after D3 anything not frozen correctly costs a NEW ERA; **⚠ ITEM 4 (the inert PoP slot) IS DROPPED 2026-09-10 — owner call D, `D-ITEM4-DROPPED-2026-09-10`. The manifest loses one FORMAT item and gains none. Everything in the rest of this paragraph is retained as the RECORD of what was weighed and declined, not as work owed; `IssuerKeyPoPMaxBytes` is NOT ratified and NOT reserved, and becomes an input to the post-RC D-DEMAND work instead of a frozen constant.** **`IssuerKeyPoPMaxBytes` CERTIFIED 2026-09-10** at **4,096 bytes, written as `4 x (blindtoken.MaxModulusBits / 8)`** and INADMISSIBLE ALONE (it needs manifest item 7's count cap + the `(issuer, epoch)` distinctness clause): `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R-ISSUERKEY-POP-MAXBYTES-VALUE-RESEARCH-CERTIFICATION-2026-09-10.md` (**the value is NOT ratified and NOT reserved — it is now an input to the post-RC DSKS work**). Derived at the modulus CEILING, not the 2048-bit floor — a floor-derived bound is a widening rule waiting for the first large key. **2,048 is refuted by eight bytes:** `MarshalPub` is `uint32BE(len(N)) || N || uint32BE(E)`, so the obvious consensus-verified PoP is 2,056 B. **THREE THINGS THE OWNER WEIGHS BEFORE RESERVING:** (a) `Prune()` drops only `BondReg.Answer`, so `IssuerKeys` is UNPRUNABLE like `Slashes` — at the count cap the slot adds `4,096 x 4,096` = 16 MiB per block, a second permanent surface EQUAL to `SlashesBytesCap`; (b) the cert SELF-CORRECTS freeze-manifest 4.6 twice — the RFC 9578 close consumes ZERO bytes of this slot, so the hedge covers only one of the two named closes, and the PoP need not be self-authenticating; (c) options alpha (a free proposer filter, recommended) and beta (`PoPDigest` folded into (d-3), which would make the bytes PRUNABLE) are priced in the cert's section 9. Merge conditions M-1..M-5; residual R-1 is a consensus-verified PoP costing 32.5 % of `T_b` at the count cap. NOT BUILT — a FORMAT surface, the **OWNER** ratifies. | (ii) D1 also delivers **one page in plain English — *what is frozen, and what can never change without a new era*, the doors that close, not the 22-item manifest** — which the owner reads BEFORE signing the freeze act; **DRAFTED 2026-09-09 as [`docs/era4-freeze-what-closes.md`](docs/era4-freeze-what-closes.md)**, to be re-checked against the manifest's final content before D3. The blind PE derivation-route audit (`D-C2-IDLE-WINDOW-VALUE`) is an input: any FORMAT parameter it names as reached by a bad route is re-derived BEFORE the freeze, because a wrong value that survives D3 is a new-era fix. **NEW 2026-09-10 — the v5 SIGNATURE-PREIMAGE item (`D-PREIMAGE-BUY-2026-09-10`, owner call 1, BOUGHT).** `consensusSigBytes` (`core/chain/chain.go`) gains **Height**: the v5 preimage is `(height, round, phase)` after CometBFT's `CanonicalVote`, making equivocation evidence **`O(1)` (~251 B derived; unmeasured until G-PRE-9, NOT the ~200 B first cited)** instead of two full `Block` bodies and killing the nesting fixed point at the root (folded into `R-BIG-EVIDENCE-UNSLASHABLE`). Bought as the **second occurrence of the #397 watermark scar** (`docs/build-process.md:204-213`), not as an optimization. FORMAT + WIDENING, so it rides D1 or costs an era; a FORMAT item, so it comes to the **OWNER** individually. GATED on its delta cert (commissioned 2026-09-10). Three binding conditions: the cert is not routed around if it refutes; it COMPLEMENTS (d-3), which still ships; and **no PR may shrink `SlashesBytesCap` in the same breath** (narrowing, cheap after the freeze — land the preimage, re-derive the cap after). **Named gate on the cert:** there are NINE non-test `verifyAtt` sites, not eight, and `core/chain/carrier.go:135` does NOT have the attested block in scope (it verifies precommits over `b.Prev`, so the signing height is the DERIVED `b.Height - 1`) — the #397 off-by-one class at a carrier seam. Also inside this row: **C-4 of `R-CERT-REDERIVE`**, the manifest's own §4.3 sentence, whose spec repair (a single `SlashesDigest`, never a `Slashes'` copy) is a FORMAT question. **THE DELTA CERT RETURNED GATED 2026-09-10** (`D-PREIMAGE-CERT-2026-09-10`): direction CERTIFIED, five build conditions, **ten RED-first gates G-PRE-1..10**. Two findings correct the change as bought — (i) `carrier.go:135`'s DERIVED height is SOUND (P1 parent binding, strictly narrowing; my earlier "#397 class" reading is REFUTED), but the #397 class is one line over: `era4Active` is `>=` so `H_era4`'s PARENT is v4, and a v5 phase dispatch keyed on `b.Version` wedges the chain permanently at exactly `H_era4` (**G-PRE-1**, dispatch on the attestation's own `Phase` instead); (ii) a SECOND seam nobody routed — the durable `ports.SignMark.Phase` is compared NUMERICALLY (`core/node/chainrole.go:87-91`), so a v5 constant of 3 probed against a stored `PhasePrecommit=2` does not block and **the node self-equivocates on upgrade** (**G-PRE-3**; the watermark records the STEP, the attestation the ERA-FORM). Also here: **`R-CONSENSUS-CONFIG-UNBOUND`** (the `MinBond` bind, owner-ruled a defect and D1 scope) | Researcher (done); Builder → OWNER per item |
| D2 | Stamp-raise test deliverables (same release): `R-CARRIER-PRUNED-HASH` (prove the seen-fold never depends on a pruned body; G-D11 two-sided shipped in Round 1A, bounded not eliminated — the descendant catch is the only defence, the proof is owed) · `R-E2E-ERA4-FIXTURE` (objective + bonded + epoch-enabled; the e2e cost ACCEPTED, owner call 14) · `R-CLOUD-ERA-PROBE` (the block era on a CLI/status surface so cloud row 13b can tell a DARK chain from "keys off-commitment" — still a real discrimination, because `-era4-activation-height=0` selects the tally branch and a chain can be dark on that branch; absorbs the rollout-signal item, whose code half was vacuous) | **RE-CLASSIFIED 2026-09-11** against the re-audit. **Item 14, the carrier-seating AGREEMENT property — SETTLED-GREEN 2026-09-11 at `c4da469`, DONE, and its register row is retired per the residual filing rule.** The six model-checks ran individually, 6/6 EXIT 0, each confirmed by its own `=== RUN` line rather than by an exit code; two ablations of the product reducer drove them RED as predicted. The evidence and the one named coverage limit (the oracle shares `attesterQualified` with the replica path, so it is independent on the reducer but not on that screen) are in the CHANGELOG entry for 2026-09-11 — the audit trail is git and the CHANGELOG, not the register. **Item 15 `R-H43-ROUND-LADDER-DESYNC` — the test did NOT cover its regime, MEASURED, and this row and the register row CONTRADICTED each other about the closer.** THE CONTRADICTION, stated rather than quietly resolved: this row gave the closer as running the model-check; the `R-H43-ROUND-LADDER-DESYNC` register row gave it as the next graded field run grading `6-fault-tolerance`. Those name DIFFERENT TIERS, and under `docs/build-process.md`'s consensus-correctness discipline they are not alternatives — the model-check covering the regime comes FIRST and the field run confirms it, never discovers it. **Resolved as an ORDERING:** the model-check gates the graded run, and the graded run then closes the register row. **Why it mattered:** `TestModelCheck_H43_RoundLadderDesyncMustStillConverge` was GREEN at `8b467e1` — its own commit, where the whole #772 fix is absent — and GREEN at `c4da469`, and GREEN under a direct ablation of the arming rule at the IDENTICAL virtual time. It housed the shape without reproducing the mechanism, so running it green discharged nothing while the record read as gated. **LEG (i) IS NOW DISCHARGED (2026-09-11):** the model-check was rebuilt to drive NON-UNIFORM pending work, and it discriminates — EXIT 0 at `c4da469`, EXIT 1 at `8b467e1`, EXIT 1 under the arm-A ablation. **Leg (ii), the graded run, is what is left.** **Item 16 `R-CARRIER-PRUNED-HASH` — OWED, and now SMALLER:** item 3 is built, so per cert §4.13 only the pre-era-4 half survives; `core/chain/chain.go` still carries the in-source *"OPEN, bounded not eliminated"* marker. **Item 17 `R-E2E-ERA4-FIXTURE` — POSTURE HALF BUILT 2026-09-11 (`core/node TestIssuerKeyBindingResolvesInTheObjectiveBondedEpochPosture`, three arms on one objective + bonded + epoch-enabled v5 fixture, each refusal clause ablated separately to exit 1); the remaining half is HELD on the new `R-CLIENT-HAS-NO-CHAIN` owner call, because the shipped one-shot client carries no chain and the OS-process positive arm cannot be asserted without changing that. The original PARTIAL note, and the way it went partial, is the tell and is kept:** the era-4 FORMAT half closed as a SIDE EFFECT of the activation-flag default, not because anyone built it — every e2e daemon now mints era-4 from height 1, so the block format, `validateCarrier`, `validateIssuerKeys`' era gate, the (d-3) preimage and `tagRevLogSize` are all exercised on the existing fixtures. The POSTURE half (objective + bonded + epoch-enabled) is still OWED: the paid-lane and consensus e2e files still launch `-objective=false`. **Item 18 `R-R3-GOB-ALLOC-AMPLIFICATION` — OWED** (no leading gob message-length check before `proof.Unmarshal` in `IngestBlockWitnesses`), **and RE-PRICED 2026-09-11: the host function is not in the shipped binary.** `IngestBlockWitnesses` is the R3 witness-bound gate, the compiler prices it at 478 against a budget of 80, it has **zero non-test callers**, and it is **ABSENT from the linked `./cmd/silt`** — so the owed decoder bound would ship inert. Item 18 is therefore excluded from the reachability gate because its DELIVERABLE does not exist, not because its surface is missing, and the surface being unlinked is its own row: `R-INGEST-WITNESSES-INERT`, whose disposition is an owner call (wire it — a research-gated consensus-rule change — or decline and disclose). **Item 19 — BUILT, both code halves** (#808, #812); the release-runbook line is the only piece left. | Tester + Builder inside the stamp-raising PR train; #237 = REFUSE TO START across a format boundary | Tester |
| D3 | R3.4 the freeze decision itself — the owner freezes the format at the RC on D1's manifest, and the readiness stamp goes 3 → 5 (no release ever stamps 4) | ratified as a principle 2026-09-03; the nine manifest sentences RATIFIED 2026-09-07; the ACT is owed. **TWO NEW OWNER CONDITIONS 2026-09-10 (`D-PREIMAGE-BUY-2026-09-10`, call 4): (i) the plain-English one-pager is re-checked against the manifest's FINAL content; (ii) D3 CANNOT BE SIGNED until the v5 signature-preimage change has either LANDED or been explicitly DECLINED with the consequence recorded in the decision entry** — *"Calls 1-3 all change what D1 contains, and the freeze is the door that closes on all of them. I'm not signing a freeze whose contents were still moving the day before."* A decline is not silence: it is a ledger entry naming what silt keeps instead. **A THIRD BLOCKER 2026-09-10 (`D-PREIMAGE-CERT-2026-09-10`): `R-CONSENSUS-CONFIG-UNBOUND`.** `MinBond`/`MinBondBytes` are bare flags that move the v5 validity verdict — I1, the #380 class, third instance. **REASON CORRECTED 2026-09-10 (`D-FREEZE-REPRICE-2026-09-10`):** not because the freeze locks the repair (a SOFT freeze does not), but because **D3 gates the B8 engagement and silt does not spend the longest-lead item on the roadmap attacking an artifact with a known I1 divergence.** Exploitability is LOW — someone must actually set a different value — so it is D1 scope, not an emergency. **▶ TRUED UP 2026-09-11 — D3 IS SCHEDULABLE ON FORMAT GROUNDS. Three things still block it, and none is a format item.** **(1) Manifest item 21 — the `docs/decisions.md` era-4 freeze entry, in the era-3 entry's four-part shape. It does not exist, and per `docs/TENETS.md` Part IX a freeze with no entry is not a freeze, so the ACT is the blocker rather than any remaining build.** It is the owner's to write and this true-up deliberately does not pre-empt it. **(2) The one-pager re-check — DONE in this true-up** (`docs/era4-freeze-what-closes.md`), against the manifest's final content, discharging `D-PREIMAGE-BUY-2026-09-10` call 4 condition (i). **(3) D3's own sentence *"the readiness stamp goes 3 → 5"* now needs a disposition, not code.** `NewBondReg` still stamps `BlockVersionRegGate` (3), and with `-era4-activation-height` defaulting to 1 the readiness tally is **bypassed entirely on the shipped posture** — `era4Active` takes the config branch and never consults it. The owner is otherwise being asked to sign a sentence about a mechanism the default configuration does not use. **And the framing, standing: D3 is a gate silt opens when the work is done. It is never a deadline** (`D-FREEZE-REPRICE-2026-09-10`) | **OWNER** freezes at the RC after D0 + D1 + D2 (`#558` is DONE and merged; the owner's ratify-or-narrow on its refusal surface is owed) — NO LONGER after the recompute spine — and commissions the external seat then (owner call 12) | owner |

#### Lane E — Boulder 4: B8 external red team on the frozen artifact + the M0 endgame

Immediately after the freeze. The flagship claim ("M0 held") is independent of the recompute. The
M0 status stays *"built + internally-clean"* until the external pass returns.

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| E1 | R4.3b-pre-on — the eight certified preconditions for `-dht-address-cap=on` (de-herd + PE O-1/O-2; the pony's own direct dial (v) with G-14; `cap_relay ∈ [2, K−R]`, `R ≥ K/2`; the v6 width flag with the `/32` floor; the exempt gauge + warning; PE O-3; one ≥3-relay cloudtest shadow run + one at `NAT_MODE=symmetric`; gates G-14…G-19 + the red-team's six) | DEFINED 2026-09-04 (certified); shadow mode MERGED (#725) | one builder session for (1)–(6) + (8); the shadow run rides A3; then **OWNER** ratifies R = 4, `cap_relay` = 4 with the printed floor and flips `on` (owner call 10, after the run reports series A < 5 % and series B < 20 %) | Builder + Tester → owner |
| E2 | R4.2 the A-axis — measure / publish / hand to B8 as-is; A3 NOT wired | measure/publish SHIPPED (#715); the re-scope RATIFIED 2026-09-07 (owner call 11) | put the bonded-adversary domain-collision finding in the R4.4 brief (one doc edit) | Builder (doc) |
| E3 | R4.3 the continuous internal red-team hunt | standing. **FIRST target, owner-scheduled 2026-09-09** (`D-RC-POSTURE-2026-09-09` (8)): the sub-frame privacy surface `R-SUBFRAME-SIZE-ORACLE` — an exact byte-length oracle plus the loss of chunk size as salt against the confirmation attack — taken BEFORE the C6 economy flip, because privacy is a Part-0 corner and the fix window narrows once the format freezes. Other named targets: the O-2 "pruned genesis" probe (`R-CARRIER-GENESIS-DISPOSAL` — a "pruned" genesis served to a fresh-sync victim with our hash and an attacker-chosen body, carrying the PE's composition: attacker-declared `BondRegs` qualifying its own keys under a kept `Pruned`; if it lands, the Tester encodes it and the Researcher certifies the refusal) · the bearer-anchor transfer surface (C5) · class-P compound ordering; bondreg full path; DHT/eclipse; long-range/WS checkpoint; relay/PayWord; churn/restart `everMature` | runs opportunistically between builds; every confirmed break → Tester gate | red-team |
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

### ▶ OPEN QUESTIONS FOR THE OWNER — the authoritative list

Simplicity rule 5 sets NO cap on how many calls may be open (owner direction, 2026-09-12,
`D-OWNER-CALL-CAP-REMOVED-2026-09-12`). What it bounds is the PRESENTATION: calls are put to the
owner **five at a time**, each with its context, its reasoning, its impact, a recommendation, and the
risk of that recommendation. **A call appears here ONLY when its evidence
exists** (owner direction, 2026-09-09, `D-RC-POSTURE-2026-09-09` (4)): a list carrying not-yet-ready
items trains the reader to skim, and then a real call gets waved through. Work whose call is not yet
earned is tracked as ordinary lane work below, and the call re-appears when the evidence lands.
Nothing else in this file is owed to the owner.

**THE FOUR CALLS OF 2026-09-10 ARE DECIDED** (`docs/decisions.md` `D-PREIMAGE-BUY-2026-09-10`).
**THREE NEW CALLS — A, B, C — OPENED THE SAME DAY when call 1's delta cert returned GATED**
(`D-PREIMAGE-CERT-2026-09-10`;
`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/V5-SIGNATURE-PREIMAGE-HEIGHT-DELTA-RESEARCH-CERTIFICATION-2026-09-10.md`).
The DIRECTION is certified and the BUY stands; the build is gated on five conditions, two of which
correct the change as it was bought.

**⚠ ALL SIX CALLS (A-F) ARE DECIDED AND ALL ARE BUILT AND MERGED.** C / D / F landed in #800 (`82fe56d`);
**A and F's wiring landed in #818** (`26b2f69`), and E's gate is in (#799). *The superseded sentence: "C / D / F are
built and merged (PR #800, main `82fe56d`)" — A was still owed when it was written.* **What the owner is owed is the
one-page *what is frozen, and what can never change without a new era*, re-checked against the manifest's FINAL
content — that re-check is DONE as of 2026-09-11** and the page is ready to read (`docs/era4-freeze-what-closes.md`).

**▶▶ THE AUTHORITATIVE LIST AT THE SESSION-27 CLOSE (2026-09-11): FIVE CALLS WERE OPEN, 5 THROUGH 9.
THERE IS NO CAP — the five-per-true-up bound was removed on 2026-09-12
(`D-OWNER-CALL-CAP-REMOVED-2026-09-12`), so a call joins this list when its evidence exists, never
when a slot frees. Call 10 was admitted that day on exactly that basis.** The four calls carried below
as 1-4 are kept as filed, and **all four are answered**: call 1 by `D-CFGBIND-TIER-PROMOTION-2026-09-11`
(the promotion is accepted, not narrowed), call 3 by `D-CARRIER-BYTES-DECLINED-2026-09-11`, and calls
2 and 4 inside the owner's four-call batch of 2026-09-11 — call 2 as **PLACEMENT, not a decision**,
which is exactly why its residue re-opens as call 9, and call 4 by shipping RT-SFO-5 disclosed and
routing RT-SFO-4 to the Researcher, whose verdict is call 5. **None of the five blocks a build today.
All five are owed before D3 signs.** Do not read "five open" as regression: three of the four prior
calls closed and two of the five below are new questions raised by their answers.

5. **RT-SFO-4 — ratify the REFUTED verdict, and pick the route.** THE EVIDENCE: the Researcher
   returned **REFUTED** on *"silt's published durability holds for every stripe configuration
   reachable in production"*, with three independent refuting instances, two of them reachable
   through **shipped defaults**
   (`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/RT-SFO-4-ERASURE-SHARD-COLLAPSE-DURABILITY-RESEARCH-CERTIFICATION-2026-09-11.md`).
   **Say the distinction in the cert's own words or this reads as a data-loss finding, which it is
   not:** the refutation is of the PUBLISHED BOUND and of the ACCOUNTING that reads it, not of
   recoverability — in the honest-failure model the bytes still come back. WHY IT IS THE OWNER'S:
   ratifying a research verdict is a veto-gate act, and the verdict falsifies a **published claim**.
   THE ROUTES, and they are not equivalent: **R-B (fix the accounting)** is free and closes five
   defects; **R-A (a Cauchy generator)** moves the genesis hash; **a `k >= 2` floor** is a read-path
   change. The pinned gate `core/pipeline TestRT_SFO_4_SingleDataShardStripeHasSixDistinctShards_PINNED_DEFECT`
   banks the finding whichever route is taken.
6. **The genesis batch — what rides the next re-mint?** THE EVIDENCE: genesis has been re-minted
   **twice already** this train, and **there is no reserved move left to ride**. WHY IT IS A CALL:
   a re-mint is scarce by CALENDAR, not by permanence (`D-FREEZE-REPRICE-2026-09-10`) — each one
   costs the graded re-run set, so batching is a sequencing trade the owner makes, not a seat.
7. **ANSWERED 2026-09-12 — YES, and a chainless client carries 32 BYTES**
   (`D-TOKEN-DOMAIN-CHAINLESS-CLIENT-2026-09-12`). The token domain is **GENESIS-COVERED**:
   `(*Chain).ChainID()` IS the height-0 block's `Hash()` — write-once, refused on mismatch
   (`ErrForeignGenesis`), derived from committed history, **time-invariant**, which is the property
   that makes it carriable. The 32 bytes arrive by **operator flag** (the daemon already prints the
   genesis hash at start-up, the same out-of-band route as `-ws-checkpoint`); a **peer fetch is
   admissible ONLY with UNANIMITY** — **the first-success form must NOT ship**, because one stale or
   lying peer would then deny every publish with no signal, a 1-of-n liveness dependency strictly
   worse than `FetchCanonicalIssuersFromAny`. **THE BREAK IS `creditDomain`, THE PREPAID CREDIT —
   NOT `fdhDomain`**, which is deliberately left unbound because a publish token is verified INSIDE
   block validity (`v5ValidateEntry`, `(*Chain).ValidateEntry`) against a third party's key, so
   binding it is a **consensus-rule change**; **the verdict's "no consensus rule" holds only while
   `fdhDomain` stays unbound, and nothing here licenses binding it.** ⚠ **WHAT THIS DOES NOT CLOSE:**
   `R-CLIENT-HAS-NO-CHAIN` is a **DIFFERENT QUANTITY on a different lane** and must NOT be closed on
   this verdict — the client's nil chain is untouched by the answer — and neither may
   `R-E2E-ERA4-FIXTURE`, whose posture half rides that row rather than this call. **ACCEPTED RISK:**
   it rests on `T-ONE-VALUE-CANNOT-TAG`, certified the SAME DAY and never adversarially tested — **if
   any verifier ever holds chain ids as a SET and SEARCHES it, "can only deny" inverts into a tagging
   surface.** **M5 (JOIN/START) is still owed the same answer ONCE**, because a separate answer for
   `joinSwarm` forks the rule. Cert:
   `/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/TOKEN-DOMAIN-GENESIS-COVERAGE-AND-THE-CHAINLESS-CLIENT-RESEARCH-CERTIFICATION-2026-09-12.md`
8. **Does the acceptance surface actually block anything?** THE EVIDENCE, two halves folded into one
   call because answering either alone leaves the surface advisory: (i) the fixture census gates —
   **25 of 40 fixtures go RED** if they are built; and (ii) the reachability job **`Go — every
   claimed lane is IN the linked binary` is NOT a required status check.** Read back from the
   ruleset at the session close, the six required contexts are `Go — vet, fmt, test`, `Website —
   changelog + links`, `Go — race detector`, `Docs ship with code`, `Go — the bbootstrap build tag
   (D-BB-BUILD-TAG)` and `Go — multi-process e2e (real TCP)`. The reachability lane gate can
   therefore go RED and a PR still merges. WHY IT IS A CALL: making it required is a **process**
   change with a real cost — it is the gate that refuses "a mechanism shipped inert", and a required
   gate that cries wolf gets disabled.
9. **`LockQC`'s route — and if the second closer is taken, the item is never built.** THE EVIDENCE:
   the Researcher classified it (`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/LOCKQC-UNDER-SIGBYTES-DEADLINE-CLASSIFICATION-2026-09-11.md`):
   the change touches a SIGNED preimage — `(*roundChangeEnv).sigBytes` — but **NOT a format
   preimage**; `(*Block).bodyHash` is unaltered and none of the four doors moves. **Deadline class:
   WIRE COMPATIBILITY**, so it sorts with its family at launch, and missing it costs a coordinated
   fleet upgrade, never an era. **It is not a format item and does not come to the owner as one.**
   What IS owed is the route: the classification names an undecided design decision (residual R-1)
   that governs whether the item is built **at all**. This is the residue of answered call 2, which
   was carried as placement.
10. **The branch-cleanup working session — what is owed is SCHEDULING, not a decision.** ADMITTED
    2026-09-12, when the rule-5 cap was removed (`D-OWNER-CALL-CAP-REMOVED-2026-09-12`); the cap was
    this item's ONLY reason for sitting outside the list. The shape is already decided
    (`D-BRANCH-CLEANUP-SESSION-2026-09-12`) — what the owner is asked for is a slot in his calendar.
    THE EVIDENCE: the inventory prerequisite is **SATISFIED** — **412 branch rows** at
    `/Users/andrewedmond/.claude/silt-agent-memory/tester/evidence/2026-09-12-session29-audit/branch-inventory.tsv`,
    carrying per branch whether it is merged-by-content, the PR it landed as if any, its last-commit
    date, and whether a live worktree holds it. It runs as **one dedicated interactive session with
    the owner present** — not queued behind other work, not bundled with hygiene tasks, not delegated
    to an unattended run; **an inventory read rather than derived live** is the point of generating it
    beforehand. Three reasons it is a session: a bulk delete is destructive and irreversible in
    practice and was **classifier-blocked** when bundled with two benign items — **owner
    authorization does not clear a classifier block**, and the safe half went down with the dangerous
    half; **"merged" is not a property `git branch --merged` can be trusted for here** (silt
    squash-merges, so ancestry lies, and a whole-diff reverse-apply is too strict — measured, it
    rejected 6 of 8 genuinely landed branches); and a worktree is LIVE if it is **locked or holds a
    running seat's cwd**, never by mtime. **One brief, one risk class** is the standing rule this
    records | **OWNER** schedules; the Builder has generated the inventory

**▶ OWNER-OWNED AND OPEN, BUT NOT FILED AS A NUMBERED CALL (2026-09-12; revised the same day when the
rule-5 cap was removed). ONE item, and it belongs to the owner.** The cap is gone
(`D-OWNER-CALL-CAP-REMOVED-2026-09-12`), so **"the list is full" is no longer a reason for anything**
and no item sits here for want of a slot. **This one sits outside the list for its OWN reason, and
that reason is a TRIGGER rather than a bound: the O-8 re-decision returns as a numbered call when the
F6 measurement set lands** (owner call, 2026-09-12; lane F row F6 names the three measurable pieces).
Filing it today would assert that the owner owes a decision he deliberately deferred, which is the
opposite of true. **The item that used to share this block — the branch-cleanup working session — has
moved UP into the numbered list as call 10**, because the cap was its only reason for being out of it
and its inventory prerequisite is now satisfied. The two no longer share a reason, so they no longer
share a block.

- **THE O-8 RE-DECISION IS OWED, AND O-8 IS NOT SCHEDULED.** `D-O8-BASIS-CHANGED-2026-09-12` is
  explicitly ⚠ **NOT A LIVE RATIFICATION TO BUILD.** A direction was ratified conditional on its own
  certification; **the certification returned GATED**, with five gates standing, and it **REFUTED the
  headline number the ratification was given on** — *"at one sampled index the unforgeable byte leg
  costs 1.6 % more wire"* held nothing else fixed. Today's scheme samples every block, so **its
  detection against any deletion is 1.0 and its forgery cost is zero bytes**; O-8 at one index
  detects a fraction ε with probability ε, and at equal detection O-8 costs **63.5× the wire.** The
  two rows are 67× apart in what they buy and were put side by side as if they were not. **The
  economics are INVERTED relative to the ratification and the re-decision IS that trade.** Nobody has
  taken it and the entry does not take it. **What it is not:** O-8 is not dead — the scheme it
  replaces is not cheap-and-sound, it is cheap-and-forgeable. And **O-8 does not restore
  retrievability**; it gives sampled possession at the opened indices, not extractability. Two
  further things bind if it is ever taken: the **leg-1 verifier fix** (`R-POR-MERKLE-TAUTOLOGY`) and
  the **seed-derivation fix B-1** are **PRECONDITIONS, not separable cheap wins** — ship O-8 over the
  current verifier and O-8 is vacuous; and O-8 must be **IN** the batched re-mint or abandoned,
  because it moves both the shard derivation and the manifest frames at once. **Nothing in the
  storage-proof area is booked as fixed: the break it responds to is LIVE.** Lane F row F6 carries
  the measurement work this decision needs, and none of that work is the owner's | **OWNER**
- **THE BRANCH-CLEANUP WORKING SESSION IS SCHEDULED, NOT QUEUED.** `D-BRANCH-CLEANUP-SESSION-2026-09-12`:
  it runs as **one dedicated interactive session with the owner present** — not queued behind other
  work, not bundled with hygiene tasks, not delegated to an unattended run. **An inventory is
  generated BEFOREHAND so the session reads measurements rather than deriving them live**, owing per
  branch: merged-by-content or not, the PR it landed as if any, last-commit date, and whether any
  live worktree holds it. Three reasons it is a session: a bulk delete is destructive and
  irreversible in practice and was **classifier-blocked** when bundled with two benign items —
  **owner authorization does not clear a classifier block**, and the safe half went down with the
  dangerous half; **"merged" is not a property `git branch --merged` can be trusted for here**
  (silt squash-merges, so ancestry lies, and a whole-diff reverse-apply is too strict — measured, it
  rejected 6 of 8 genuinely landed branches); and a worktree is LIVE if it is **locked or holds a
  running seat's cwd**, never by mtime. **One brief, one risk class** is the standing rule this
  records | **OWNER** schedules; Builder generates the inventory beforehand


**▶ ANSWERED — the two calls opened 2026-09-10 (the docs true-up), kept as filed so each answer reads
against its question.** Both are now decided; see the authoritative list above.

1. **ANSWERED 2026-09-11 — the promotion is ACCEPTED, not narrowed** (`D-CFGBIND-TIER-PROMOTION-2026-09-11`).
   The owner ratified the six as knowingly frozen; none is re-classed out of the bound family.
   ORIGINAL CALL: **The genesis bind froze six compile-time DEFAULTS. Was that bought?** The bind is correct under
   canon rule 8 — a consensus quantity must be a function of the chain. But the six values that reach
   the chain are compile-time defaults, so binding them promotes six **Evolving-tier** knobs to frozen
   per-network constants, and it makes any future default change a **fleet-wide brick on upgrade**.
   **Driven, not argued:** move the `-quorum` flag default 3 → 2, rebuild, and an unchanged-argv
   restart refuses on the same chain. `DerivedBondTTL`'s own doc comment — *"a tuning knob (Evolving),
   not a fixed law; a real deployment can tighten it"* — becomes false for a launched network the
   moment this binds. **`DerivedBondFloor` is CLEARED and is not part of the question:** it is a
   compile-time constant with no hardware-dependent divergence. The call is whether the six are
   knowingly frozen, or whether some are re-classed OUT of the bound family before D3.
2. **ANSWERED 2026-09-11 — carried as PLACEMENT, not decided.** The RC-blocking boundary stays open and
   its live residue is open call 9 (`LockQC`'s route), which the classification has since sorted into the
   WIRE-COMPATIBILITY class. ORIGINAL CALL: **How much of the `Atts` / QC program is RC-blocking?** The certification's landing order has five
   steps, and **step 2 must land at least one deployment window after step 1** (version-skew stall).
   It therefore **cannot ship in one release**, and it cannot all be RC-blocking without splitting the
   RC. Steps 0 and 1 are cheap and are the natural answer: step 0 needs nothing, and step 1 discharges
   two gates (G-CB-1 and G-ATTS-1, with G-QC-1) in **one commit**. The rows are
   `R-CARRIER-QC-NESTED-ROUNDCHANGE` (routes ahead of the cap, and it is a liveness veto, not only a
   CPU cost), `R-CARRIER-ATTS-PREPAREQC`, `R-CARRIER-ATTS-NORMALIZE`, and — explicitly **POST-RC** —
   `R-CARRIER-ATTS-BLOCKS-CEILING`.

**▶ ANSWERED — the two calls opened 2026-09-11 (the freeze-manifest re-audit). BOTH are now decided;**
call 3 the same day, call 4 in the owner's four-call batch. Kept as filed.

3. **ANSWERED 2026-09-11 — DECLINED FOR THE RC, DISCLOSED TO B8, ROW KEPT** (`D-CARRIER-BYTES-DECLINED-2026-09-11`). The owner took
   the recommendation on the record. The exposure rides into the B8 engagement as manifest item 22, the register row says so, and the
   cap remains wanted at the stamp raise — it is a narrowing validity rule, so declining it forecloses nothing and costs no era. The
   pony-class honest-maximum measurement is still the precondition for ever setting a value. **The call below is kept as filed, so the
   answer reads against the question it answered.** ORIGINAL CALL: **Manifest item 8 (`R-CARRIER-BYTES`) — in the RC, or explicitly
   declined and disclosed to B8?** THE
   EVIDENCE: `validateCarrier` (`core/chain/carrier.go`) enforces phase, `verifyAtt` over `b.Prev` and per-id
   distinctness, and has **no count cap, no byte cap and no qualification screen**; there is no
   `CarrierBytesCap` or `G-CB-1` symbol in the tree. `Block.LastCommit` is in **both** `bodyHash` preimage
   literals, so the surface is hash-covered, and with `-era4-activation-height` defaulting to 1 it is reachable
   on the RC's own field network rather than on a hypothetical future one. It is a narrowing validity rule, so
   its deadline is the stamp raise, **not the freeze** — this call does not gate D3. WHY IT IS A CALL AND NOT
   LANE WORK: it is a scope trade between build-immutable #8 (the 2 GiB box) and chain liveness, and it turns
   on a pony-class honest-maximum measurement nobody has run — a cap set without that measurement risks landing
   far below the honest floor, which is worse than no cap. THE RECOMMENDATION ON THE RECORD (PE, 2026-09-11):
   **decline it explicitly for the RC, disclose it in the B8 brief (manifest item 22), and keep the register
   row that says so.** The reason the recommendation is "decline explicitly" rather than "defer" is that the
   item has now been **deleted twice on wrong grounds** — once on a refuted *"it served the recompute"* label,
   once by re-homing it onto `Block.Atts`, a field that is in neither preimage literal and whose certified fix
   cannot apply to a hash-covered field. Deleting it by re-label is neither shipping it nor declining it.

4. **ANSWERED 2026-09-11 — SHIP RT-SFO-5 DISCLOSED; ROUTE RT-SFO-4 TO THE RESEARCHER.** The owner took the
   recommendation: annotate the two false doc claims WITH the measurement rather than restate the promise, and
   send the parity-collapse half out as a durability question. **That verdict came back REFUTED and is open
   call 5.** ORIGINAL CALL: **`R-SUBFRAME-SIZE-ORACLE`'s two LIVE pins — does the RC ship with them?** THE EVIDENCE: the red-team pass
   is encoded as ten permanent gates (#817), and **two are PINS rather than assertions because the defect is
   RED on `main`** — `core/pipeline TestRT_SFO_4_SingleDataShardStripeHasSixDistinctShards_PINNED_DEFECT` pins a
   one-data-shard stripe whose parity shards collide, and
   `core/pipeline TestRT_SFO_5_EntryFileSizeIsTheExactByteCount_PINNED_DEFECT` pins `ports.Entry.FileSize` as
   the exact plaintext byte count. Each has a companion that REDDENS when the defect is fixed
   (`TestRT_SFO_4_PinRedensWhenTheSixthParityBecomesDistinct`, `TestRT_SFO_5_PinRedensWhenFileSizeIsBlinded`). A pin goes RED when the defect is FIXED,
   which is what makes them a decision and not a backlog. The third finding, the sealed-manifest length oracle,
   was FIXED in #821 and its gate is now a drift check. WHY IT IS THE OWNER'S: this is a **Part-0 privacy
   corner**, an immutable rather than a tunable, so the trade is his and not a seat's. THE ONE ARGUMENT TO
   DISCOUNT: *"fix it now because the window narrows once the format freezes"* is exactly the era-cost pressure
   `D-FREEZE-REPRICE-2026-09-10` withdraws — neither fix is a format item, so the freeze does not close on
   either. The honest frame is that the gates BANK the finding either way, and the question is whether an RC
   ships with two known-live privacy pins.

**The frame that decided them: the freeze deadline is SOFT** (`D-FREEZE-REPRICE-2026-09-10`) — genesis
and format can both still move pre-launch. Its real currency is re-running graded field runs and
delaying the external B8 engagement: days and cloud spend, not permanence. ***"Or it costs an era" is
WITHDRAWN as a reason to buy anything.***

- **A — the chain id in the v5 preimage: BOUGHT**, on the SCHEMA leg alone. CometBFT's
  `CanonicalVote` was named as the settled corner and carries `ChainID`; dropping it repeats the #397
  scar inside the very fix meant to pay it back. The I5 break is **live on the two test networks
  today**. One class-3 (box-owned) `HeadRef` field; the freeze read-set does not move.
- **B — the pricing ask: WITHDRAWN.** What remains is small: make the standing predicate correct
  (**"was bonded at that height,"** late-reveal preserved).
- **✅ C — (d-3): BUILT AND MERGED** (`D-D3-BUILT-2026-09-10`). A pruned v5 block now recomputes its
  own hash, so its retained body is SELF-COVERING and `Pruned` is retired for era-4 — the defect
  `Block.Hash()`'s own comment records as having shipped false three times, now driven both ways by
  G-D3-7. **cbor key 19.** A signal was split (`HeavyProofsShed()` vs `IsPruned()`) without which the
  trust-floor refusal would have gone SILENTLY DEAD on v5. **The v4/v5 parity contract is NOT
  amended** — the malformed-pruned arm needed TWO registrations, not an era-specific expectation.
  *The purchase record, kept:*
- **C (as bought) — (d-3) `AnswerDigest`: BOUGHT, and the delta cert came back GATED — ⛔ STOPPED AND REPORTED**
  (`D-D3-CERT-REFUTATION-2026-09-10`). **The DIRECTION is certified; the SPECIFICATION is refuted in
  two clauses.** (i) §4.3's `AnswerDigest ports.Hash` would break the **frozen-format immutable on
  LIVE v2/v4 history** — `omitempty` never omits a fixed-size array (`Pruned` is the living proof,
  key 14 in every encoded block), and `bodyHash()` folds `BondRegs` with **no version branch**
  (`chain.go:902`); repair certified as `AnswerDigest *ports.Hash`, the proven era-3 step-2a form.
  (ii) The recursively-reduced `Slashes'` is REFUTED — `Prune()` never touches `Slashes`
  (`chain.go:982-984`) — and adds a per-hash 16 MiB deep copy re-opening #563; **`SlashesDigest` is
  the certified form**, which closes C-4 of `R-CERT-REDERIVE`. **And the ratified sentence is wrong a
  THIRD time:** *"shrinks the face roughly 40×"* is ZERO, because the cap measures ENCODED bytes
  (`SlashesEncodedSize`, `chain.go`) while (d-3) reduces the PREIMAGE. **Two new owner calls and
  nothing built.** Bought on the **third-time rule**, not on
  elegance: *a proof that a property holds today is not a structure that makes violating it
  impossible.* `R-CARRIER-PRUNED-HASH` would prove the discipline holds now; any later change can
  silently violate it. (d-3) makes the retained body **self-covering**, so violation becomes
  impossible rather than absent. `Block.Hash()`'s own comment records this safety claim shipping
  false **three times**. The dual-era branch survives: it keys on `BlockVersion`, which already
  discriminates eras everywhere.
- **✅ D — manifest item 4: DROPPED AND RECORDED** (`D-ITEM4-DROPPED-2026-09-10`). Verified never
  built; removes **16 MiB per block of permanent unprunable surface**. *The record, kept:*
- **D (as decided) — manifest item 4 (the inert PoP slot): DROPPED.** Reserving an unpopulated field bought only
  "not paying an era later", and that cost is void. Keeping it costs **16 MiB per block of permanent
  unprunable surface** — the exact surface the `R-BIG-EVIDENCE-UNSLASHABLE` self-armor face measured
  being weaponised. **Sequenced AFTER
  C**, so option beta's prunable-`PoPDigest` fallback exists. If `demandMsg` proves insufficient
  post-freeze, add the field then and pay in re-runs and calendar.
- **E — the config-in-consensus gate: MERGE**, with one condition drawn from silt's own lint. Every
  declaration citing a `validate_v5_*` file carries **`UNGATED: R-CONFIG-GATE-V5-REGIME`** in the
  DECLARATION and the FAILURE TEXT, not only the header (`scripts/check_source_gates.py`'s
  convention). **Built and gated** — a v5 citation with no marker is RED (ablation A9). The gate's
  limit is regime coverage, not efficacy: it found the third and fifth unbound fields, and its
  reflective half caught two `Config` fields a hand-read had missed.
- **⚠ F — the genesis-config family: SCHEMA MERGED, NOT BOUND** (`D-CFGBIND-BUILT-2026-09-10`).
  **CORRECTED 2026-09-10, third site of the same claim; the original wording follows so the
  correction reads as one.** `core/genesis/genesis.go` mints genesis with **no `Params` field**,
  so `omitempty` drops key 20 and the production genesis hash is unchanged; `CheckConsensusParams`
  has zero non-test callers. The schema is real and green; nothing populates it, so no node yet
  refuses on a config divergence. See the correction block on `D-CFGBIND-BUILT-2026-09-10` and the
  `R-CONSENSUS-CONFIG-UNBOUND` register row, which stays OPEN. ORIGINAL CLAIM:
  `ConsensusParams`, **17 fields by VALUE**, committed on genesis at **cbor key 20** — a
  differently-configured node computes a different genesis hash and **cannot join**. Rule 8's two
  arms COMPOSE: committing the values manufactures the referent a local assertion lacked, so
  `CheckConsensusParams` catches the one case joining cannot — an operator editing a flag and
  restarting on a chain already joined. The foreign-genesis refusal is now LOUD at `ports.LogWarn`
  in `(*Node).SyncChain` (was `LogDebug`); `docs/decisions.md` `D-CFGBIND-CERT` still says `LogDebug`
  and carries a dated correction rather than a rewrite.
  **⚠ ORDERING — CORRECTED 2026-09-10, AND IT STILL BINDS FOR A DIFFERENT REASON.** As written above
  the constraint was VOID: F as merged moved no genesis hash (schema only), and F's wiring does not
  move the *paramless* hash either, because `Block.Params` is a pointer with `omitempty`, so key 20
  drops, `e44344ea…72c0` is unchanged and the ~250 `AppendGenesis` fixtures are untouched. What moves
  is any genesis a **daemon** mints. **The corrected constraint: call A joins F's WIRING in one
  network re-seed, not one fixture re-pin.** The currency is graded field re-runs and calendar. *The
  original sentence, kept so the correction reads as one: "call A moves the signature preimage and F
  moved the genesis hash — they must land in ONE genesis move, or the graded re-run set is paid
  twice."* *The ruling, kept:*
- **F (as ruled) — the genesis-config family: ALL FIVE FIELDS, ONE CHANGE, BEFORE D3.** The scope growth is
  **nominal, not material** — the genesis-hash-covered shape binds a FAMILY, so two fields to five is
  adding entries to a digest. **And the two that arrived after the ruling are the SHARPEST of the
  set:** an activation-height divergence needs **no malice at all** — two honest operators with
  different values flip eras at different heights and validate different blocks under different
  rules. *That is not a hole waiting for an attacker; it is a scheduled fork that fires on a date.*
  **You cannot freeze an era boundary whose value nothing binds across replicas.**

**Also DECIDED 2026-09-10 (`D-PREIMAGE-CERT-2026-09-10`), needing nothing further from the owner:**
`MinBond` is a **defect to fix**, bound to the CHAIN rather than to a local assertion, and it should
land **before D3**. **The reason was corrected the same day** (`D-FREEZE-REPRICE-2026-09-10`): NOT
because the freeze locks the repair — a soft freeze does not — but because **D3 gates the B8
engagement, and silt does not spend the longest-lead item on the roadmap attacking an artifact with a
known I1 divergence.** The **canon rule** the three instances taught is now `docs/build-process.md`
rule 8.

1. **The signature-preimage change at D1 is BOUGHT** (`R-BIG-EVIDENCE-UNSLASHABLE`), conditional on
   its delta cert. The owner's decisive argument was the SCAR, not the optimization: dropping
   **Height** from `consensusSigBytes` (`core/chain/chain.go`) is the **second occurrence of the
   #397 watermark scar** — `docs/build-process.md:204-213`, rule 6, the PE ruling on #432 — the same
   `(height, round, step)` schema family, the same dropped field, and the dropped field is again the
   thing that broke. Three binding conditions: **(a)** the delta cert lands and is not routed around
   if it refutes; **(b)** it COMPLEMENTS (d-3), which still ships; **(c)** **no PR may shrink
   `SlashesBytesCap` in the same breath** — shrinking a cap is narrowing and stays cheap after the
   freeze, so the preimage lands first and the cap is re-derived afterward.
   **⚠ ONE CORRECTION TO THIS ROW'S OWN PRIOR TEXT.** It sold the change as *"eight non-test
   `verifyAtt` call sites, all with the block in scope."* Verified at source there are **nine**, and
   `core/chain/carrier.go:135` does **not** have the attested block in scope — it verifies precommits
   over `b.Prev`, the PARENT hash, so the signing height is `b.Height - 1`, derived rather than read.
   That is the #397 off-by-one class the call is paying back, so it is a **named gate on the delta
   cert**. The BUY is unaffected: it rests on the scar argument, which is independent and verified.
2. **The `SlashesBytesCap` disclosure sentence is UNRATIFIED and REPLACED.** The 16 MiB VALUE does
   not move. The replacement, in the owner's words: ***"(d-3) shrinks the face roughly 40× but does
   not remove it; what removes it is the v5 signature-preimage change, which makes evidence O(1)."***
   The unratification is recorded **in place**, and the owner ratified that handling as standing
   practice for all ratified text — *"the correction should be visible as a correction, not laundered
   into the original."* The four remaining fallen certification sentences (C-1…C-4) are now
   **tracked** with per-sentence re-derivation closers as `R-CERT-REDERIVE`, not merely annotated;
   **C-4 is inside the freeze manifest, so it is D1 content.**
3. **Unbounded state growth is REFUSED; the face is bounded by STANDING.** The disposition: *a slash
   mints no permanent leaf for an identity that had no bonded standing at the height in question* —
   slashing an identity with no standing achieves nothing legitimate. The predicate **cannot** be
   "currently bonded" (late-reveal evidence about a since-lapsed bond is legitimate and must record);
   it must be **"was bonded at that height,"** which the chain can check. The exact predicate is
   research-gated. **Deliberately NOT commissioned until call 1's cert returns** — with `O(1)`
   evidence the cap can shrink as a narrowing change and bound this from the other side, so deciding
   it now prices it against a world call 1 is about to change.
4. **D3 is NOT signed. Two new conditions.** (i) the plain-English one-pager is re-checked against
   the manifest's FINAL content; (ii) **D3 cannot be signed until the preimage change has either
   LANDED or been explicitly DECLINED with the consequence recorded in the decision entry** — *"I'm
   not signing a freeze whose contents were still moving the day before."* A decline is not silence.

**The audit's other three suspects are lane work, not owner calls** (their evidence exists but the
disposition is a seat's): `relayRetentionEpochs` = 1 (`core/node/relayrole.go:148`) — derived purely
as a seen-map MEMORY bound and later welded on as the relay lane's PAYMENT DEADLINE at `:233-237`,
with the comment at `:229-231` asserting a reaped session "forfeits no owed credit" which the C2 gate
measures FALSE; this is **the third occurrence of silt's own scar — a durability/memory knob that is
also a security or economic parameter** (`docs/build-process.md` records the first two), so the
third-time rule fires and the Tester owns the gate. It also sharpens **row C11**: the relay payment
deadline was never designed as one. Then the cloudtest FT hard cap of 380 s versus the 430 s envelope
(`integration/cloudtest/scenarios.sh:512`), and `DerivedBondFloor` (`cmd/silt/daemon.go:2104`, weak —
mitigated by `Seal` being sequential). Eight parameters were CLEARED as documented and driven and are
not to be re-opened: `RegCap` 256, the `IssuerKeys` cap 4,096, `SProofMax`, `DefaultChunkSize`,
`h43ForwardEntries` 4, `capBlockIntervalBoundSec` 3600, `DerivedEpochBlocks` 8, and R = 4 /
`cap_relay` = 4.

**Decided 2026-09-09 and recorded — do not re-open** (`docs/decisions.md`):

- The delivery idle window is **24m**, cost verified zero (`D-C2-IDLE-WINDOW-VALUE`). The owner's
  second question — whether the ORIGINAL derivation was wrong, and *what else was derived the same
  way* — commissioned a blind PE audit of silt's derived parameters against three faces (modal-vs-worst
  bound; an uncarried quantization step; an undriven coupling premise), partitioned by whether the
  parameter FREEZES at D3. **Its findings are an input to D1's content.**
- The relay lane is **default-OFF at every tier**, disclosed as *"the settlement model is unfit for the
  edge tier"* rather than as a disabled feature, because a lane needing 286–508 Mbit/s sustained to
  settle concentrates revenue on horses and punishes pony participation — an economic-recentralization
  vector, not a bad default (`D-RC-POSTURE-2026-09-09` (1)). The real fix (incremental settlement or a
  periodic sweep) is research-gated design debt, tracked as **row C11**.
- The RC **labels every lane that has never run in the field**, as a release-checklist RULE rather than
  a per-lane judgment (`D-RC-POSTURE-2026-09-09` (3), in
  [`docs/release-checklist.md`](docs/release-checklist.md)). Open at the RC: the paid delivery lane,
  graded at E5 after the stamp raise.
- **`SlashesBytesCap` keeps its value (16 MiB) and LOSES its route** (`D-SLASHCAP-ROUTE`, owner:
  *"CLOSE THE ROUTE. Not a re-ratification."*). A consensus validity rule whose invariant was computed
  from the DEFAULTS of two proposer-side-only flags is now bound to the values IN FORCE:
  `core/node.CheckSlashEvidenceHeadroom` and a `cmd/silt` refuse-to-start, driven by G-SLASHCAP-1..4
  with the ablation red first. The owner's weighting: it is the **#380 class** — a consensus quantity
  that is a function of local config rather than of the chain, the second instance in one week — and
  the severity is the **accountability** face, since past the boundary a real double-signer's evidence
  is rejected by the cap and the equivocator keeps its seat. Deadline was the STAMP RAISE (a validity
  rule, manifest item 10), not deferrable past it because RAISING a cap is a WIDENING change outside
  the narrowing exemption. Residual filed, not folded in: `R-BONDREG-SINGLE-OVERSIZE`.
- **Delegation excludes the format surface.** Green-and-reviewed merges proceed without the owner
  EXCEPT on D1's FORMAT items, which come to him individually (`D-RC-POSTURE-2026-09-09` (6)).
- **Sequencing, owner-directed:** the sub-frame privacy surface (`R-SUBFRAME-SIZE-ORACLE`) is
  red-teamed BEFORE the economy flip — privacy is a Part-0 corner, an immutable rather than a tunable,
  and the fix window narrows once the format freezes. The repair statistic's normalisation and
  node-level granularity WAIT: a measurement does not hold a corner (`D-RC-POSTURE-2026-09-09` (8)).

**⚠ CORRECTED 2026-09-12 — B8 IS NO LONGER A SCHEDULING CONSTRAINT, AND THE SENTENCE BELOW IS FALSE.**
The external pass is an **AI** red team (already recorded in the session-27 close below), so the
engagement is **minutes to hours, not weeks**, and it is **not** the longest-lead item on the
project. **The longest-lead item is the owner's own acceptance cycles**, which is why the first-run
clean-up (Lane F rows F3–F5) now has leverage nothing else has. *Provenance: this reclassification is
RATIFIED in the ledger as **`D-B8-AI-ADVERSARY-2026-09-12`** — it is no longer owner direction living
only in a conversation, and that entry is what every correction here cites.* Three further sites
state the old reason inside a recorded 2026-09-10 rationale — the D3 row, the four-calls block and the
`R-CONSENSUS-CONFIG-UNBOUND` register row. **Those are left as written**, because each is a quotation
of why a decision was taken at the time, and the standing practice is that a correction is visible as
a correction rather than laundered into the original. The ORDERING they justify — the `MinBond` bind
lands before D3 — is unaffected: it rests on not attacking an artifact with a known I1 divergence,
which is true whatever the engagement costs in calendar. *The superseded sentence:*

**In flight, needing nothing from the owner until D3:** finding and contracting the external B8 seat
(`D-RC-POSTURE-2026-09-09` (5)). It has no code dependency and is the longest-lead item on the
roadmap — weeks of calendar — and it is what lifts M0 from *built + internally-clean* to *held*, so it
sits on the CALENDAR critical path even though it is not on the code one. Only the ARTIFACT it attacks
waits for the freeze.

The 23-call block of 2026-09-07 is decided and archived; its decisions live in `docs/decisions.md`
(`D-TRUE-UP-CALLS-2026-09-07`, `D-CONSENSUS-ARMING`, `D-H43-WORKLESS-DESIGNEE`). The three calls
delegated on 2026-09-09 are in `D-DELEGATED-CALLS-2026-09-09`.

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
- **DONE 2026-09-10:** the `SlashesBytesCap` configuration route close + the self-armor gate (#795), the
  owner-call records and the relay edge-unfitness disclosure (#791), and the FT-tier finding (#796,
  write-up only). C2's idle-window VALUE is RATIFIED at 24m.
- **DONE 2026-09-08/09:** C1 the v2 flat primitive + the ledger's flat leg retired (#780) · C3 R2.2 the
  observability set (#784) with the per-tier work totals that re-found the withdrawn thresholds (#788) ·
  C4 the R2.7 blocking detectors (#781) · C5's two CODE closers (#787, with `D-GENESIS-MOVE-2` ratified) ·
  C2 the delivery idle window (#789, value ratification owed).
- **OPEN (Lane C):** C5's bearer-anchor red-team pass · C6 R2.4 (two of its aborts re-pointed at the
  harness by `D-WORK-VISIBILITY`) · C7 R2.7 (scoped down: A4-2/A4-3 are floor detectors, A5 is
  half-instrumented, A6 has no instrument and is scoped OUT until C3's successors land) · C8 R2.8 ·
  C9 measurements · C10 three small PRs.
- Design records: [`docs/thinking/2026-09-01-economy-observability-design.md`](docs/thinking/2026-09-01-economy-observability-design.md)
  and the dated R2.x deliberations in `docs/thinking/`.

#### Boulder 3 — The freeze (era-4/v5) · the RC release
- **DONE:** R3.1 the SMT domain-separation residual (#731, #749, #754) · R3.3 (doc-only; re-derive
  only if #299 moves) · `#558` the chain-store refusal (2026-09-07).
- **DONE 2026-09-09:** D0 the cold auditor (#786) — **the RC's only floor-box requirement, closed.**
- **DONE 2026-09-10:** **G-1** (#797) — the box entry asserts BOTH arms of `objective()`. It was the
  certified precondition on item 2; item 2 is now DROPPED, and G-1 stands on its own merit (it caught a
  wired verifier with `cfg.MinBond == 0` reaching two verdicts at one config). Not itself a format
  change. Owed alongside it and filed: `R-G1-POSTLATCH-DRIVEN` (the post-latch maturity row is not yet
  driven).
- **DONE 2026-09-11:** the D1 FORMAT train is **CLOSED** — `tagRevLogSize` (#819, manifest item 1) and
  owner call A's preimage inside M1 (#818) merged, and **manifest item 2 (the digest 5 → 3 retirement)
  is DROPPED** (`D-MEMBERSHIP-KEEP-FIVE-2026-09-11`). The digest set freezes at FIVE.
- **DONE 2026-09-11 (the same day, after the train closed):** the second deliberate genesis re-seed
  (#821, `D-MODE-ORACLE-2026-09-11` — the keyless encryption-mode oracle; not a format item by the
  four-door test) · **M2, the era floor** (#822) — slash evidence whose FORM is below the version this
  chain requires at the evidence's own height is not evidence here, closing the only I5 face the M1
  test fix left open. Narrowing, so its deadline was the stamp raise and it landed early.
- **OPEN (Lane D) — this is the WHOLE remaining critical path to the RC, and it is SHORTER than the
  row above it reads:** D1's FORMAT content is delivered and the **OWED-and-FORMAT list is EMPTY**; the
  one-page *what closes* is re-checked (2026-09-11) · D2 the stamp-raise test deliverables, of which
  items 14 and 15 are BUILT-with-an-UNSETTLED-verdict and need a RUN, not a build · D3 the freeze act
  (**OWNER**), whose first blocker is manifest item 21 — the `docs/decisions.md` freeze ENTRY, which
  does not exist and which Part IX makes constitutive.
- **PLACED 2026-09-11, so the plan carries them somewhere:** **M3** (the FDH bindings) is STAMP-RAISE
  and **fuses with manifest item 11**, whose gate is now **BUILT** — `TestFDHDomainSetIsPairwisePrefixFree`
  (`core/blindtoken/fdh_domain_prefix_free_test.go`) derives the domain set by a `go/ast` walk of
  `core/blindtoken`, so a fifth domain is covered the day it is declared rather than needing the gate
  to land with it. The set is FOUR (`fdhDomain`, `creditDomain`, `demandDomain`, `relayAnchorDomain`),
  all six pairs differ at byte index 10, and the walk is held non-vacuous by
  `TestFDHDomainSetSourceWalkFindsTheRealDeclarations`. **The mechanism the item was written with is
  corrected in the same change:** a prefix violation is a STRUCTURAL defect, not a live forgery, because
  `ctr` sits between the domain and the message and changes per block — measured by
  `TestAPrefixPairFailsToCollideOnlyBecauseTheCounterVaries`, which sees the crafted shift collide on
  SHA-256 block 0 and fail on block 1. **M5** (JOIN/START + the `ch.Len() == 0`
  audit) stays IN the RC as the only closer for the silent singleton, per the owner's M-series trim.
  **`R-membership G-3`**, the witness id-list size gate, is UNBUILT and is **KEPT BUT RESCOPED**
  (`silt-agent-memory/researcher/reviews/research-outcome/2026-09-11-G3-witness-id-list-size-gate-SCOPE-RULING.md`).
  It is cited by its residual because **`G-3` denotes at least six distinct gates in this repo** — one of
  them, `TestG3_ParentBindingPrecedesTheCarrierLeg`, sits in the same file
  (`core/chain/floorbox_box_v5_test.go`) as `TestG7_BoxBudgetCoversTheWitness`, the BG-3 budget leg that
  refutes this one's stated failure mode. **Two corrections.** ~~It is `R-membership`'s closer~~ — it
  bounds what ONE BOX accepts as a witness and bounds neither `slashed` nor `validatorsSeen`, so the
  residual's closer is UNNAMED (register row below). ~~Whether it belongs inside D1 at all is a question
  for the Researcher~~ — **ANSWERED: it does not.** It is not a format item (it adds no leaf and changes
  no required root), and it is not a stamp-raise item either: it is box-local admission on a door that
  returns no verdict, since `(*Box).Validate` downgrades to `IndeterminateTrustlessly,
  ErrRecomputeGated` unconditionally. Its real deadline is whichever of the accept flip (R1.8 / #657) or
  the witness-transport merge lands first — **POST-RC, and off the RC critical path.** Cost of keeping it
  as rescoped: zero engineering now; nothing in the tree decodes a witness from the wire to build it
  against.
- **OWNER-DEFERRED, POST-RC — do not re-open these as RC scope:** M4 (ALPN) and the DNS-seed work
  (`D-MODE-ORACLE-2026-09-11`) · `Atts`/QC steps 2–4, with steps 0–1 the RC-blocking part in flight ·
  RT-SFO-6 · the v5 digest retirement (reversed, `D-MEMBERSHIP-KEEP-FIVE-2026-09-11`) · Boulder 1 ·
  Boulder 5 · the C6 economy flip. **One item in that family is UNCLASSIFIED and should not be
  scheduled until it is:** bringing `LockQC` under `(*roundChangeEnv).sigBytes` is the only member of
  the `Atts`/QC family that could touch a SIGNED preimage, which makes it a wire-format change with a
  rolling-upgrade break — nobody has assigned it a deadline class.

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

### ▶ WHERE WE ARE, AND WHERE WE ARE GOING — read this first (2026-09-12 true-up)

**▶▶ THE SESSION-28 CLOSE (2026-09-12). `main` = `cc0cb35`.** **FIVE PRs merged**, counted off the
merge record rather than recalled: **#835** (`e547d6a`, the memory-index lint asserts the descriptive
hook), **#836** (`605ce1d`, the playbooks rebuild unconditionally and four dead premises are trued
up), **#838** (`14f794f`, the bond-term fork-choice wording retired from `integration/`), **#837**
(`ed6c9e2`, the published fork-choice claim ratified at full scope) and **#839** (`cc0cb35`, the
ratifications ledger). *#838 merged before #837 — it was the parent of a stacked pair, and the
numbers do not order the merges.* **#828 is now the ONLY open PR**, a deliberate draft held with its
e2e RED on open call 7, the chainless client.

**▶ WHAT THE AUDIT FOUND, IN THE SHAPE IT ACTUALLY HAS: the product works — established by RUNNING
it, not by reading about it — and the record describing it did not.** That is the whole finding, and
both halves are load-bearing. Nothing about the shipped system was discovered to be broken by
reading; the defects that matter were found by putting hands on the thing.

**Two of them are veto gates and are stated first so nothing below softens them:**

- **The storage proof passes with ZERO BYTES.** Over 100 sweeps the zero-byte attacker's ledger row
  is **bit-identical** to the honest holder's, and the shipped `-liar` control is annihilated in the
  same run. Confirmed by two seats blind to each other. **The break is LIVE**, it is the measured
  failure that Lane F row F1 (the adversary-shape gate) is built against, and **nothing on
  2026-09-12 fixed it** — `D-O8-BASIS-CHANGED-2026-09-12` records that the remediation's basis
  changed and that a re-decision is owed, not that anything is repaired.
- **D3 fetcher privacy is booked BUILT and is INERT.** `D-DEMAND` books it as *"◑ slices 1+2 BUILT"*;
  nothing in the tree imports the package, the function's only three call sites are its own test, and
  production names the DURABLE identity as the funding that settles. **Re-labelled now, wired
  post-RC** (`D-D3-RELABEL-2026-09-12`, Lane F row F14). ⚠ This is `D-DEMAND`'s D3, **not** Lane D
  row D3, the freeze act.

**▶ DO NOT READ THIS AS A CONSENSUS REPAIR. MOST OF WHAT LANDED IS DOCUMENTATION, GATES AND
DEFAULTS.** Across all five PRs and all fourteen `docs/decisions.md` entries dated 2026-09-12:
**no consensus rule changed, no format surface changed, and no economic mechanism changed.** The
fork-choice work (`D-FORKCHOICE-CLAIM-2026-09-12`, #837) is the sharpest case and says so in terms —
**the PROPERTY was sound and the DOCUMENTATION was false**; the certification found height-then-head-hash
selection composed with the finality gate delivers what the old sentence was reaching for, **no
consensus defect was found and none is fixed.** `D-BOUNTY-PAYS-FOR-REPAIR-2026-09-12` ratifies an
INTENT and authorises no build; the mechanism is research-gated. **A roadmap that reads as though a
consensus defect was repaired is its own defect.**

**▶ THE STANDING PREMISE THAT PRICES SEVERAL OF THE REST — and it is time-boxed.**
`D-NO-EXTERNAL-USERS-2026-09-12`, in the owner's words: *"Assume silt does not exist outside of this
box **for now**."* **Migration cost for published objects is ZERO**, so any argument of the form
*"existing published data would break"* is **void until further notice**, and the options it ranked
must be **RE-PRICED, not re-read** — a conclusion whose premise has died is not evidence. It also
re-frames flixz: a relaunch-and-republish is not a cost to minimise, it is **an acceptance test silt
wants to run.** **What it does NOT delete:** eras 2 and 3 stay FROZEN formats, the genesis re-mint
budget is unchanged, and it is **not** a licence for *"or it costs an era."* **★ THE EXPIRY IS HALF
THE DECISION.** The premise dies at **LAUNCH** — not at the era freeze, which is a different and
earlier event — so **anything leaning on it must carry a NAMED RESIDUAL at the RC**, or it becomes
the next premise that died in the docs and lived on in the code. The honest classification, stated
rather than papered over: **the expiry can be made LOUD but not ENFORCED**, because no test can know
a launch date and *"a live network exists"* is not a locally-checkable predicate.

**▶ WHAT MOVES NEXT** is the ratified sequence at the top of this file: the two structural gates
(G-2 adversary-shape, then G-1 shipped-default graded lane) → the first clean run → the owner's
acceptance cycles → D3 → Astra / B8 → the field grade → `1.0.0`, with the O-8 measurement set and
the repair-claim rate-limit certification running in parallel and needing no owner input.

**▶▶ THE SESSION-27 CLOSE (2026-09-11). `main` = `c911127`.** Nine PRs merged in this stretch,
verified against the merge record rather than recalled: **#824** (manifest item 8 restored as a
register row), **#825** (*"G-3 is built"* was wrong — `R-membership`'s closer does not exist),
**#826** (the source comments that ASSERTED "era 4 is dark" trued up), **#827** (item 8 DECLINED for
the RC and disclosed; the bonded prepare-QC flood gets a row), **#829** (every reason string in the
state table re-derived), **#830** (item 11 — the FDH domain set gated pairwise prefix-free),
**#831** (the h43 round-ladder model-check now DISCRIMINATES — RED without the #772 fix, GREEN with
it), **#832** (the reachability lint watches the freeze-manifest mechanisms) and **#833** (item 17,
the posture half, as a two-armed gate). **#828 is NOT in that list: it is a deliberate DRAFT, held
with its e2e RED on open call 7** (the chainless client) — all three e2e failures reduce to the one
mechanism, a requester with no chain, so the lane is OFF rather than green.

**D1's FORMAT set is CLOSED and the owed-and-format list is EMPTY.** That is the headline above, and
nothing merged in this stretch reopened it: every one of the nine is a test, a lint, a register row
or a documentation correction. **The three D3 blockers are unchanged and NOT ONE IS A FORMAT ITEM** —
manifest item 21 (the freeze entry, which does not exist and is the OWNER's act to write), the
one-pager re-check (DONE), and a disposition for D3's own sentence *"the readiness stamp goes 3 → 5"*,
which is owed because on the shipped default the readiness tally is bypassed entirely.

**NEXT SESSION IS A WHOLE-PROJECT AUDIT, planner-led** — five seats, fresh eyes, not a continuation
of the freeze train. The reason it is scheduled now rather than after D3: B8 is an **AI** red team,
and an AI adversary reads every stale comment in the tree. The repo clean is therefore load-bearing
evidence, not hygiene. **⚠ CORRECTED 2026-09-12 — the sentence that stood here, *"B8 contracting has
not moved all session, and it is the only item whose clock ignores our work"*, is FALSE.**
`D-B8-AI-ADVERSARY-2026-09-12` **retires contracting as a task outright**: there is no procurement
clock to start, so nothing about B8 can fail to move. Commissioning the pass is unchanged — it stays
the owner's act at D3. **No item's clock now ignores our work**; the longest-lead item is the owner's
own acceptance cycles, which our work shortens directly. The audit's reason is the one stated above —
an AI adversary reads the tree — and it does not need a lead-time argument.


**▶ THE HEADLINE — THE OWED-AND-FORMAT LIST IS EMPTY. D3 IS SCHEDULABLE ON FORMAT GROUNDS TODAY.**
A blind PE re-audit of all 22 freeze-manifest items classifies them **9 BUILT · 5 DROPPED/DECLINED ·
2 PARTIAL · 6 OWED**, and **none of the six is on the freeze surface**
(`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-era4-freeze-manifest-re-audit-f826c72-2026-09-11.md`).
Applying the four-door test — a block field / cbor key / `Hash()` preimage; a committed fact or its
canonical encoding; the era activation rule; which roots are required on which paths — **no OWED or
PARTIAL item satisfies any clause.** Items 7, 8, 9 and 11 add no field, no leaf and no encoding; they
only reject more, which `D-F2-EVIDENCE-RECOMPUTE` settles as outside the freeze surface. Items 15–18
are tests or a node-local parse guard, item 20 is a node-local start-up refusal, items 21–22 are
documents.

**Three things still block D3, and NOT ONE OF THEM IS A FORMAT ITEM.** Name them separately or this
true-up reads as "nothing left": **(i) manifest item 21**, the `docs/decisions.md` era-4 freeze
entry — constitutive under `docs/TENETS.md` Part IX, so a signed D3 with no entry is a null act, and
it is the **owner's** act to write; **(ii)** the one-pager re-check against the manifest's final
content (`D-PREIMAGE-BUY-2026-09-10` call 4 condition (i)) — **done in this true-up**; **(iii)** a
disposition for D3's own sentence *"the readiness stamp goes 3 → 5"*, because on the shipped default
the readiness tally is bypassed entirely (below). **D3 is a gate silt opens when the work is done. It
is never a deadline.**

**▶ THE FINDING THAT REORDERS THE PLAN — ERA 4 IS NOT DARK. IT IS LIVE FROM HEIGHT 1 ON THE SHIPPED
DEFAULT.** `-era4-activation-height` defaults to **1** (`cmd/silt/daemon.go`), and
`(*Chain).era4Active` takes the config branch whenever that value is non-zero
(`core/chain/chain.go`), so **every freshly seeded network — every e2e daemon and every graded cloud
run — mints era-4 blocks from height 1.** The manifest sorts its 22 items by DEADLINE, and the
VALIDITY class's sorting rests on one sentence: *"free while era-4 is dark."* **That premise is FALSE
on main.** Measured, not derived: #818's own commit message records `TestEquivocatorSlashedOverTCP`
timing out because every committed block above the genesis of a fresh network now carries the era-4
form.

**What it changes, and what it does not.** Deadline CLASSES do not move — items 7, 8 and 9 are still
narrowing rules, still outside the four doors, still owed at the stamp raise. **Their position in the
BUILD ORDER does:** three uncapped or unvalidated hash-covered surfaces are now reachable on the RC's
own field network, so they gate the graded run rather than a hypothetical future one. And the
freeze's own safety argument narrows to the honest form: nothing has committed under era-4 because
there is **no live network**, not because the format is unreachable. That is the weaker of the two
grounds and it is the one that is true. **Every booking in this file that deferred work because
era-4 was dark is re-stated below on the reason that actually survives, or dropped** — `R-membership`
(reversed on other grounds), `R-E2E-ERA4-FIXTURE` (its format half closed by accident, which is the
tell), cloud row 13b's SKIP (`D-TRUE-UP-CALLS-2026-09-07` (8), premise void), the R2.9 node-half
scope call (`D-R2.9-NODE-HALF-CALLS` (3), premise void, ruling intact) and the C2 harness
requirement (dropped on a second ground that survives).

**▶ CORRECTION 2026-09-11 — M1, the single genesis move, IS MERGED** as PR **#818** (`26b2f69`).
*The superseded sentence, kept so the correction reads as one: "M1 ... is BUILT AND HELD on
`builder/m1-genesis-move`, not merged: it is a FORMAT item and the owner reviews those
individually."* The owner's individual review happened and the branch landed; this block's headline
had gone stale against its own file, which contradicted itself 40 lines down. It carries owner call
A's preimage, owner call F's bind, `ConsensusParams.NetworkName` at cbor key 18, and the two
era-activation daemon flags defaulting to 1/1 (`D-M1-GENESIS-MOVE-2026-09-11`).

- **The genesis hash moved `4a305b96…9c46` → `b862f16b…3b57`.** The **paramless** pin
  `e44344ea…72c0` is UNCHANGED, and no `Block` cbor key was added — key 18 is inside
  `ConsensusParams`. **The freeze read-set does not move.** Say "the genesis re-mints", never "the
  format changes": they are different quantities with different prices, and the second sentence is
  what invites the era-cost argument.
- **The ordering constraint is now DISCHARGED, not merely restated.** Call A, F's wiring, the name
  and the era flags are in ONE re-seed. Anything further that moves the genesis hash pays the
  graded re-run set again.
- **Two era-4 gates were found demanding opposite verdicts on identical bytes**, and the one that
  won required an I5 violation: `G-PRE-7`'s fourth subtest demanded CONVICT on a pair whose
  era-2-form leg is bit-identical to one harvested from another silt network. Driven, not derived.
  M1 fixes the TEST and scopes both gates to an explicit, machine-checked pre-era-4 floor; the
  RULE's closer is the era-floor evidence refusal, which is **research-gated and lands with or
  before M1 reaches a graded run** — it is the only move that closes an I5 face.
- **The membership doctrine gained a second CLOSED category** (owner ruling): a field is bound, or
  it is an identity property of the network (moves no verdict AND is genesis-covered), or it is
  excluded with a reason. Both arms of the new category are machine-checked from opposite sides, so
  "it's an identity field" cannot become an escape hatch. Field count 17 → 18, still the rule's
  OUTPUT.

**▶ UPDATE 2026-09-11 — A SECOND, DELIBERATE GENESIS RE-SEED, and the M-series is TRIMMED.**
Owner-ratified in full on the mode-oracle certification (`D-MODE-ORACLE-2026-09-11`).

- **The genesis hash moves again** — paramless `e44344ea…72c0` → `fdfb676c…eb58`, params-carrying
  `b862f16b…3b57` → `af49c742…95c8`. `manifest.secretsPlainLen` pads the sealed secrets box, so the
  manifesto's manifest chunk ID moves and `Entry.ManifestChunks` with it. The **root does not move**
  and no `Block` cbor key, committed leaf or validity rule is touched: **not a format item** by the
  four-door test. This pays the graded re-run set a second time, knowingly. It closes the keyless
  encryption-mode oracle (threat-catalog **F8**), which was the one finding on that docket outside
  every existing disclosure and the one with a settled corner (LHAE / RFC 8446 §5.4) at 0.013 % of
  object bytes.
- **Two remedies were DECLINED and the boundary is recorded** so it is not re-opened: a `FileSize`
  ladder (novel under simplicity rule 1, and outside M0) and re-padding the data frame (reverts a
  measured 250× storage win against a vantage the tenets already disclaim). *"We are moving genesis
  anyway"* is the scope magnet, and the answer to it is in `D-MODE-ORACLE-2026-09-11`.
- **THE M-SERIES SCOPE TRIM (owner-ratified).** **M5 (JOIN/START mode + the `ch.Len() == 0` audit)
  STAYS IN THE RC.** It is the only thing that closes the **silent singleton**: a typo'd flag founds
  a network of one that reports healthy, because `cmd/silt/daemon.go` mints genesis and then checks
  the params it just minted **from the same config**. A check whose referent it produced itself
  cannot fail. **M4 (ALPN set-and-assert on both ends + the LAN beacon tag + the DNS TXT prefix) and
  the DNS-seed work DEFER TO POST-RC.** ALPN is a wire demux and never a boundary — it is
  unauthenticated, cleartext in the ClientHello, and covered by no certificate under
  `InsecureSkipVerify` + pinning — so it hardens a surface the genesis bind already partitions. The
  single-DNS-seed eclipse surface is a real open row and is not closed by any network tagging.

**Boulder 3 is the whole remaining critical path: D1 → D2 → D3.** Everything else is either done,
post-RC, or standing work that runs beside it.

**D1's FORMAT set is CLOSED as of 2026-09-11 — item 1 merged (#819), call A merged inside M1 (#818),
and item 2 DROPPED** (`D-MEMBERSHIP-KEEP-FIVE-2026-09-11`: the digest set freezes at FIVE, and the
retirement was never `R-membership`'s closer — ~~G-3 is~~; **CORRECTED 2026-09-11: neither is
`R-membership G-3`, which bounds witness admission and not set growth. That residual's closer is
UNNAMED**). The audit that sized the set is kept below as
the record of how it was sized. A blind
audit classified all 22 manifest items against `0ed3b92`: ten are BUILT or disposed, ten are owed at
the stamp raise or later, and **exactly two carry a freeze deadline — item 1 (`tagRevLogSize`) and
item 2 (the digest 5 → 3 retirement)**
(`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-era4-freeze-manifest-final-content-audit-0ed3b92-2026-09-10.md`).
The freeze is a smaller act than the 22-item list makes it look.

**⚠ THE ORDERING CONSTRAINT WAS VOID AS STATED, AND IS CORRECTED — IT STILL BINDS, FOR A DIFFERENT
REASON.** `docs/decisions.md` `D-CFGBIND-CERT` says *"F moves the genesis hash; call A consumes it."*
F as merged moved nothing — it shipped schema only. And F's wiring, now built, does **not** move the
*paramless* genesis hash either: `Block.Params` is a pointer with `omitempty`, so a genesis minted
without params drops cbor key 20, `e44344ea…72c0` is unchanged, and the ~250 `AppendGenesis` fixtures
are untouched. What moves is any genesis a **daemon** mints. So the constraint stands: **call A joins
F's wiring in ONE network re-seed**, not one fixture re-pin. The currency is graded field re-runs and
calendar, which is exactly what `D-FREEZE-REPRICE-2026-09-10` says the freeze costs.

**The reachability headline is two numbers, not one.** A sweep of 1,784 exported declaration sites
across `core/` and `adapters/` found **117 inert sites**, of which **71 (61 %) are the floor-box /
recompute keystone — inert by ratified owner direction** (`D-RECOMPUTE-FREEZE`), not silently. **46**
is the non-frozen remainder
(`/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/2026-09-10-inert-mechanism-sweep-core-adapters-0ed3b92.md`).
Three of the 46 are cases where **the record claims delivered**, and all three are already tracked or
corrected — `Block.Params` / `CheckConsensusParams` (`R-CONSENSUS-CONFIG-UNBOUND`, being fixed),
`RequireBondedFetchers` (D-DEMAND P3b, already disclosed in `docs/design/m0.md` §10.1 as off into
the flip; the record correction ships with this true-up) and `BBootstrapRunPrecondition` (BB-14; same). **The other 43 get NO rows.**
A lint regenerates that list on demand, and 43 rows is 43 things a human must read forever — the
precise cost simplicity rules 3 and 4 exist to stop paying.

**⚠ THE LINT DID NOT WATCH THE SURFACE WHERE THE INERT SHIPPING HAPPENED. BUILT 2026-09-11 — THE
FREEZE-MANIFEST MECHANISMS ARE IN THE LANE SET.** `scripts/check_reachability.py` read
`scripts/reachability_lanes.txt`, whose lane set was exactly three — `paid-delivery`,
`paid-delivery-topup`, `paid-relay-client` — with **no freeze-manifest mechanism in it**, so the gate built
after owner call F shipped inert did not cover the class of thing F was. **The 22 items were resolved as a
MAPPING, not a sample: SIX name a mechanism that lives in production Go** (items 1, 3, 6, 10, 19, 20), and
all six are now lane records — nine of them, because items 3, 19 and 20 each have two independently
droppable halves — anchored on a new posture section in `docs/release-checklist.md`. **The other sixteen
have no symbol for a gate to hold:** 2, 4, 5, 12 and 13 are dropped or refuted, 8 is declined for the RC,
7, 9, 16, 18, 21 and 22 are owed with nothing in the tree, and 11, 14, 15 and 17 are delivered as TESTS.
**Every one of the nine is PRESENT in the linked `./cmd/silt`** — measured, not assumed — and the gate is
green at 15 records. The ablation that proves it bites removes the sole production call site of
`CheckConsensusParams`, reproducing the F failure exactly: RED, exit 1.

**Two clauses of the filing above were wrong at source, and are corrected rather than quietly dropped.**
(1) *"Two live cases sit outside it today"* named one case and then named the one that had already been
fixed: `CheckConsensusParams` was wired on 2026-09-11 by #807 (`6eb42d2`) and is reachable. The two
mechanisms that ARE inert today with the record reading delivered are `(*Node).RequireBondedFetchers` and
`credit.BBootstrapRunPrecondition`, both verified at `820fd6f` with zero non-test callers — and **neither
is a freeze-manifest item**, so neither gets a lane record here. Wiring a mechanism to make a gate green is
a consensus-surface change; the finding is reported, not patched. (2) The header bound *"lanes the release
checklist makes a public claim about"* was sound and is preserved: the checklist now makes that claim, one
bullet per mechanism, so the set stayed bounded rather than becoming a sweep.

**The substantiality test moved from a proxy to the compiler, because the proxy refused the truth.** The
gate required a symbol whose body held a callback closure. **The first filing said that rule refused FOUR
manifest mechanisms; running the retired rule against the nine freeze-manifest records refuses SEVEN** —
`setD3Digests` (cost 172), `validateD3Digests` (1010), `CheckConsensusParams` (278), `genesis.Build` (421),
`printEraObservable` (721), `StartupEraLines` (906) and `chainstore.Recover` (802), every one against a
budget of 80. A closure is a property of a client entry point handing work to a transport, not of a
consensus validator. The gate now reads `go build -gcflags=-m=2` per lane package and requires
`cannot inline`. **The same measurement found a live false-RED in the old gate:**
`(*Chain).stateRootLeavesV5` declares a closure, is present in the binary, and has zero `.funcN` rows
because the compiler inlined the closure into its own parent — the retired `.funcN` witness would have
reported that live mechanism as hollowed.

**NEITHER TEST DOMINATES THE OTHER, and the first filing's "strictly better" was wrong.** The inline
verdict catches a hollowing that collapses the body's cost — a gutted `setD3Digests` draws `can inline` and
the record is REFUSED (driven, exit 1). It does NOT catch a hollowing that leaves the cost above budget:
gut `v5ValidateSlashes` — delete the `SlashesBytesCap` check, the M2 era floor and the culprit walk, keep
one used closure — and the compiler still says `cannot inline` at cost 263, so the gate runs green while
the retired `.funcN` witness goes red on the same binary. That case and the `stateRootLeavesV5` false-RED
are the same compiler event read two ways, which is why the retired rule was a closure-inlining detector
rather than a hollowing detector, and why the trade is still right. The two-way table is in
`scripts/check_reachability.py`, where the next person to gut a validator will read it.

**Do not book a manifest item as BUILT on the strength of a green reachability run.** That sentence now has
**seven standing homes** — the lane file twice (its header and the manifest section), the gate's docstring,
the line the gate PRINTS on every green run, the checklist section, this entry, and `docs/decisions.md` —
because reachability is necessary and never sufficient: it proves a symbol survived linking, not that the
mechanism is correct or that it computes what its record claims. Item 20 is the standing example — two of
its three arms have records, the third has no mechanism in the tree, and no green run distinguishes
*"the third arm is owed"* from *"the item is done"*.

**The gate's red does not block a merge today.** The CI job `Go — every claimed lane is IN the linked
binary` is not one of the `Protect main and staging` ruleset's six required status checks, so a red is
visible and advisory rather than merge-blocking. `docs/release-checklist.md` says so where it cites the
gate. Making the job required is an owner act, owed before the RC.

**The production comments carrying the dead premise are CORRECTED (2026-09-11).** The filing here read
that `core/chain/validate_v5.go`'s BG-1 note — *"The proposer mints v2, the readiness stamp is 3,
`Era4ActivationHeight` has no operator surface — so on every chain that exists,
`ValidateProposal`/`ValidateCommit` never enter these functions"* — had **all three clauses false**.
**That count was wrong, and the correction records it:** clause 2 is TRUE at source. `NewBondReg` stamps
`BlockVersionRegGate` = 3, so the readiness stamp IS 3. What changed is that the stamp stopped being
load-bearing — the tally consuming it is evaluated inside `rotateEpoch` under
`cfg.Era4ActivationHeight == 0`, so the shipped default of 1 never consults it. Clauses 1 and 3 and the
conclusion are false: `-era4-activation-height` IS an operator surface, it defaults to 1, `MintVersion`
returns v5 at every height above the genesis (height 0 stays v2), and the v5 composition is on the live
path from height 1. **The sweep found eight assertion sites, not one** — `equivocation.go`,
`m2_era_floor_gates_test.go`, `lastcommit_carrier_pins_test.go`, `rt_r04b_c3_nonobjective_era4_test.go`,
`e2e/e2e_test.go`, `e2e/delivery_lane_harness_posture_test.go`, and the three `integration/cloudtest`
harness files — plus four QUOTATION sites (`eradeclared.go`, `erastate.go`, `eradeclared_test.go`,
`cmd/silt/erasurface_test.go`) that quote the phrase as a start-up string being disambiguated and are
correct as written. **And the premise was masking a second wrong mechanism:** `e2e/e2e_test.go` explained
`TestPaidDeliveryLaneRefusesWithoutACommittedKeyBinding`'s refusal by the readiness tally; measured on that
exact argv with the store kept, `silt chain-status` reports *"no chain yet (0 blocks)"*, still 0 after a
75 s settle. **That fixture commits nothing**, so this row's "closed half" claim below — that every e2e
daemon *"mints era-4 from height 1"*, exercising the era-4 format on the existing fixtures — does NOT hold
for the paid-lane fixtures, which mint no block of any version. The format half is **not** closed by
accident there; that needs re-checking per fixture. A sentence describing what a gate covers decays exactly
like a cited test name.

**Two clusters get a one-line disclosure in `docs/design/m0.md` §10.1 rather than a row**, because
both have a published claim and neither had a not-yet-wired note: **takedown transparency** (the log
IS written at `(*Chain).apply` and `translog.MTH` is live in `core/chain/statehash.go`, but
`translog.VerifyInclusion` / `translog.VerifyConsistency` and the proof accessors have no callers, so
**no shipped surface serves an inclusion proof to a third party**) and **demand / cost-to-wash**.

**`docs/TENETS.md` needs no change** — checked, not assumed. Its corner-5 factual clause is true and
its guarantee is explicitly framed as a goal.

**The register got smaller, on purpose.** Three certifications filed on 2026-09-10 proposed **24** new
residual names under two unratified prefixes. Four rows survive, five existing rows retired into their
one true closer or into `m0.md` §10.1, and the register went **35 → 34**. The reasoning is in
[`docs/thinking/2026-09-10-session-docs-true-up-residual-triage.md`](docs/thinking/2026-09-10-session-docs-true-up-residual-triage.md).


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
  pruned to actionable rows, and ten simplicity rules are standing — canon since 2026-09-10 in
  [`docs/build-process.md`](docs/build-process.md), filed in `.claude/CLAUDE.md` until then
  (`D-RECOMPUTE-FREEZE`; the note: `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/NOTE-to-silt-team-simplicity-and-roadmap-reorder-2026-09-08.md`).
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
> track (the archive file). `Source` names a `silt-agent-memory/…` path or a line of this file at filing time.

### Residual register

| Name | Bucket | Closer | Source | Duplicate-of |
|---|---|---|---|---|
| `R-H43-ROUND-LADDER-DESYNC` | ACTIONABLE | Lane A3 · Tester; freeze-manifest item 15. **RECORD CORRECTION 2026-09-11: this row and the D2 row named DIFFERENT closers and neither said so.** This row said the closer was the next graded field run; the D2 row said it was running the model-check. Different tiers, and `docs/build-process.md`'s consensus-correctness discipline makes them SEQUENTIAL, not alternative: the model-check covering the regime comes first, the field run confirms. **Resolved as an ordering — (i) the model-check must cover the regime, then (ii) the graded run closes this row.** Leg (i) was NOT discharged as the record implied: `TestModelCheck_H43_RoundLadderDesyncMustStillConverge` was GREEN at `8b467e1` (its own commit, the whole fix absent), GREEN at `c4da469`, and GREEN under a direct ablation of the arming rule at the identical virtual time — it housed the shape, it did not reproduce the mechanism. **Leg (i) IS NOW DISCHARGED (2026-09-11):** the same test drives NON-UNIFORM pending work and discriminates — EXIT 0 at `c4da469`, EXIT 1 at `8b467e1`, EXIT 1 under the arm-A ablation, each confirmed by `=== RUN` and bit-identical across `-count=3`. The fix is merged (#772) and field-confirmed (`2633a11-deep`). **Closer, unchanged in substance:** the next graded run grades `6-fault-tolerance` at the re-priced 190 / 380 s tiers with the head-reading wait (#774), gated on leg (i) | silt-agent-memory/researcher/reviews/research-outcome/CONSENSUS-LIVENESS-h43-COMPOSED-DIFF-c2a476a-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-REAPER-FORFEIT` | ACTIONABLE | **Delivery half CLOSED 2026-09-09; the RELAY half is open and re-priced WORSE.** **Delivery half — CLOSED.** The window is DERIVED, not chosen: the stamp is coarsened to `idle/4`, so a window guarantees only `0.75 × idle` of survival since a real settlement (MEASURED at 0.751× — `core/node/TestC2GuaranteedSurvivalIsThreeQuartersOfTheWindow`), and the governing stall is the 430 s re-keyed takeover of `D-H43-WORKLESS-DESIGNEE` (21), not the 190 s modal tier. Floor = `430 × 4/3` = 9m33.33s (`cmd/silt/numeraire.go` `deliveryIdleFloor`, raised from 1 s, which enforced nothing); shipped default 24m (`deliveryIdleDefault`), chosen over 10m because 10m's margin is 4.7 % and 10m does not survive a repeat of the 1040 s stall the field produced (both candidates driven both ways). **THE VALUE IS THE OWNER'S TO RATIFY** (`docs/decisions.md` reserves "the delivery idle-window VALUE (owed after A3)"): the precondition is met by the graded run `report-97e3101-deep.md` (`6-fault-tolerance` at 190 s, `10a-stall-drill` at 430 s), but call (4)'s ratified arithmetic was EPOCH-denominated against the 190 s tier while this ships a DURATION against 430 s — the same magnitude on different reasoning, and call (4)'s own premise ("a chain stall reaps live sessions") is refuted below. The blind PE recommends ratifying 24m and verified the cost is zero: `CloseDeliverySession` releases at `max(close, maxAnchorEpoch + W + 1)` with `W = 4`, = 1834 s = 30m35s > 24m, so the release epoch binds at BOTH candidate windows and the longer window adds no deposit-lock latency. **The 430 s figure is now a conservative ENVELOPE, not a bound the reaper races** — the causal path it was sized on is withdrawn (the wall-clock-step disclosure in `docs/design/m0.md` §10, corrected 2026-09-09 by a driven run). The graded harness had been running the paid lane at 90 s — a 67.5 s guarantee, under even the 190 s tier the same sheet confirms — and is corrected to the shipped default. **Relay half — the owed measurement is TAKEN and it is worse than this row assumed: not "keeps the burn", but settles ZERO.** The relay lane settles ONCE AT CLOSE and `sweepRelaySeen` drops a session at `admitEpoch+2` UNSETTLED, so an over-running session forfeits 100 % of the credit it earned while the fetcher's face was already spent at open (driven: 8 increments forwarded, paid 0 — `core/node/TestC2RelaySessionReapedByTheEpochSweepSettlesZero`). Session lifetime is 9–16 blocks = 413–734 s at the measured `T_b` = 45.865 s/height, so settling one whole face (24.41 GiB) inside it needs **508 Mbit/s sustained (worst) / 286 Mbit/s (best) on ONE session**; at a 100 Mbit/s edge uplink a session moves 4.81 of 24.41 GiB and is paid **nothing**. The reap is also LAZY with exactly one production caller (`OpenRelaySession`) — there is no periodic relay sweep, where the delivery lane has `SweepDeliverySessions` plus a daemon ticker — so on a quiet relay the epoch passes and nothing fires. **On these numbers `-accept-relay-payments` should NOT be enabled at edge tiers** (pony/horse), where the uplink cannot clear the ratio; it is not a knob an edge operator can turn on and be paid for. A periodic relay sweep, or settling incrementally rather than at close, is a DESIGN change (it moves an economic rule) and is routed separately, not built here. **ROUTED 2026-09-09 as row C11** — the owner ruled the lane default-OFF at EVERY tier (not merely at the edge) with the settlement math disclosed as *the settlement model is unfit for the edge tier*, and the mechanism tracked as owned design debt: Economist advises, Researcher certifies, owner ratifies (`D-RC-POSTURE-2026-09-09` (1)-(2)) | silt-agent-memory/principal-engineer/reviews/RULING-residual-register-true-up-e963034-2026-09-07.md; measurement `core/node/c2_idle_window_gates_test.go` G-C2-7 | — |
| `R-DELIVERY-SWEEP-TICKER-UNFIRED` | ACTIONABLE | The daemon's PERIODIC delivery sweep — the goroutine at `cmd/silt/daemon.go` ticking at `deliverySweepInterval(installedIdle)` and posting `delivery-sweep` — is observed at NO tier. Every test caller of `SweepDeliverySessions` invokes it directly (`e2e/delivery_paid_test.go`, `core/node/r29_delivery_session_test.go`, `sim/demand_conserved_test.go`); none exercises the ticker. Before Lane C2 the cloudtest row-13b close poll was the only observation that a shipped daemon ever swept at all, and at the shipped 24m window the ticker fires at 12m — longer than any graded cloud flow lives — so no graded run can observe it again. What IS gated is the arithmetic (`cmd/silt/TestC2SweepIntervalIsHalfTheInstalledWindow`: half the INSTALLED window, strictly positive at every acceptable window) and the wiring by source (`TestR29DaemonRefusalsAreWiredAtStartup_Source`, G-C2-18). The lazy sweep on activity is a live second path, so an unfired ticker degrades reclamation on a SILENT server only. Closer: a daemon-tier test that drives the loop with an injected clock, or an e2e booted at a window short enough to tick — which the floor now forbids, so the honest closer is the injected-clock one | blind PE `silt-agent-memory/principal-engineer/reviews/RULING-c2-idle-window-09a3da4-2026-09-09.md` non-blocking finding 1 | — |
| `R-SUBFRAME-PREPAY-ONLY` | ACTIONABLE | Lane C7 (research-gated, a precondition of the C6 flip, NOT of the C5 merge — the economy ships default-OFF) · MEASURED by the blind PE and encoded on the branch: serve revenue accumulates per `(server, requester, root)` lane (`core/credit/escrow.go:265`), so 5,000 serves over 250 distinct fetchers skim **250 credits at a 262,160 B shard and 0 at a 1,048 B shard** — the first credit on one lane costs 3,002 serves against 12. Sub-frame objects therefore do NOT become serve-funded when their bounty base falls to zero; they become **prepay-only** for durability. The coupling that produced it: C5's storage half pushes an object class into precisely the integer-truncation regime its pricing half warns about, where the accumulator mitigation is already REFUTED on build-immutable #8. Closer: the Researcher prices sub-frame durability before the flip | silt-agent-memory/principal-engineer/reviews/RULING-c5-preflip-closers-399f842-2026-09-09.md | — |
| `R-SUBFRAME-SIZE-ORACLE` | ACTIONABLE | **Lane E3 (red-team) — SCHEDULED AHEAD OF THE C6 FLIP by the owner, 2026-09-09** (`D-RC-POSTURE-2026-09-09` (8)): privacy is a Part-0 corner, an immutable rather than a tunable, and the fix window NARROWS once the format freezes, so this pass runs before the economy flip and before D3 wherever a fix would touch format. (The repair statistic's normalisation and node-level granularity WAIT, by the same call — a measurement does not hold a corner.) · TWO privacy reductions in the same direction from one change, both measured and both recorded in `docs/threat-catalog.md` under the existing F3 entry as a CATALOG UPDATE, explicitly not a mitigation: (i) padding previously blurred a sub-frame object's size to "somewhere in one chunk" and its exact byte length is now an oracle to any caretaker (layout) and any holder (stored shard length) over the whole class ≤ `chunkSize − 9`; (ii) convergent dedup for sub-frame objects now SPANS `-chunk-size` — the same payload yields ONE root at 64 KiB / 256 KiB / 1 MiB (a multi-frame object still yields two, which bounds it), so chunk size was an accidental salt against the confirmation attack and no longer is. **THE RED-TEAM PASS RAN 2026-09-11 (#817) AND THE ROW IS NOW AN OWNER CALL.** Ten permanent gates across `core/node/rt_sfo_column_oracle_test.go` and `core/pipeline/rt_sfo_*_test.go`; **two are PINS, not assertions, because the defect is RED on `main`** — `TestRT_SFO_4_SingleDataShardStripeHasSixDistinctShards_PINNED_DEFECT` and `TestRT_SFO_5_EntryFileSizeIsTheExactByteCount_PINNED_DEFECT`, each with a companion that reddens when the defect is fixed. The sealed-manifest LENGTH oracle (threat-catalog F8) was the one finding with a settled corner and it is **FIXED in #821** (`manifest.secretsPlainLen` pads the inner secrets box; its gate is now a drift check). RT-SFO-6 is owner-deferred POST-RC. **Closer: the owner rules whether the RC ships with the two live pins** (open call 4 above). Neither remaining fix is a format item, so *"the window narrows at the freeze"* is NOT available as a reason to rush either — that is the era-cost pressure `D-FREEZE-REPRICE-2026-09-10` withdraws, and the sentence above that invokes it is superseded on that point while the SCHEDULING it justified (before the C6 flip) stands on the Part-0 ground alone | silt-agent-memory/principal-engineer/reviews/RULING-c5-preflip-closers-399f842-2026-09-09.md | — |
| `R-ANCHOR-BEARER-TRANSFER` | ACTIONABLE | Lane C5 · red-team: a demand anchor is a bearer instrument — its presenter need not be its payer (the refund therefore pays only an existing account); one pass on the transfer surface before R2.4 | silt-agent-memory/researcher/reviews/research-outcome/R2.9-session-remainder-refund-and-live-anchor-cap-RESEARCH-CERTIFICATION-2026-09-06.md | — |
| `R-SLASH-CULPRIT-ADMISSIBILITY` | ACTIONABLE | Lane D1 · Researcher -> **OWNER** (call B): `validateSlashes` (`core/chain/chain.go:2200-2213`) has NO chain-membership and NO culprit-standing check, so `apply()` mints a permanent SMT leaf for any 32-byte key named in an accepted proof -- no bond, no coalition. Route (C) (`O(1)` evidence) makes this **~2.8x CHEAPER**: ~23,857 -> ~66,842 permanent leaves per 16 MiB block. **This RE-PRICES owner call 3** (state growth bounded by standing), which was deliberately sequenced to wait for exactly this number -- and it runs AGAINST the expectation, since `O(1)` evidence bounds the face from the cap side while making each junk leaf cheaper. CometBFT and Ethereum both gate evidence on set membership; silt gates on nothing. Closer: the culprit-admissibility predicate, research-gated -- and NOT bolted on blind, since a naive `bonded > 0` makes an unbonded double-signer unslashable (the owner's own trap: **"was bonded at that height"**, never "currently bonded"). MEASURE before designing (gate G-PRE-9 also converts the ~251 B derived evidence size) | silt-agent-memory/researcher/reviews/research-outcome/V5-SIGNATURE-PREIMAGE-HEIGHT-DELTA-RESEARCH-CERTIFICATION-2026-09-10.md | — |
| `R-CONFIG-GATE-V5-REGIME` | ACTIONABLE | Lane D1 · Builder: `TestConsensusVerdictIsNotAFunctionOfLocalConfig` validates a **`Version: 1`** block (`core/chain/redteam_consensus_test.go:61`) and `ValidateCommit` dispatches to the v5 composition only at `Version >= 5` (`core/chain/chain.go:3020`). Measured statement coverage under that test ALONE: `validate_v5_predicates.go` **0/178**, `validate_v5_quorum.go` **0/229**, `validate_v5.go` **0/53**, `stateview_live_v5.go` **0/64** — so the gate pins the ERA-1 twin `RequiredQuorum`, and restoring the same #380 defect in `v5RequiredQuorum` (`validate_v5_quorum.go:358`) **alone leaves it GREEN**. Found by blind PE review (F-1), which also verified the v5 twin is NOT unguarded: `TestM1A3_V4V5ParityOracle` covers it. **MERGED with the owner's condition 2026-09-10**: every declaration citing a `validate_v5_*` file now carries an `UNGATED: R-CONFIG-GATE-V5-REGIME` marker in the DECLARATION and the FAILURE TEXT (`scripts/check_source_gates.py`'s convention), enforced RED and driven by ablation A9. Closer: a v5 regime that **REPLACES** the era-1 one (the reviewer's explicit advice is to replace, not to add regimes six through nine). Couples to `R-CONSENSUS-CONFIG-UNBOUND`: `Era3/Era4ActivationHeight` are simultaneously the fields whose probes were dead by one height, the fields that escaped the old prose-keyed pin, and the boundary the D1 freeze is about. **SECOND FACE FOLDED IN 2026-09-10 — same test, same closer.** The gate also reflects over **`chain.Config`**, so its closed complement is closed over the WRONG SET: the consensus-reaching fields in **`node.Config`** are unaudited. Proven by `BondLabelSamples` — `core/bond/bond.go` `VerifyPlot` compares `len(a.LabelIndices)` against the verifier's OWN local value, wired from the daemon's `-bond-label-k`, reaching a hard Reject in `core/chain/validate_v5_quorum.go`, so a `k=32` node rejects EVERY bond reg a `k=64` swarm accepts. `BondVDFDelay` is the same shape, latent. **SECOND FACE CLOSED 2026-09-11 by owner call F's delivery** (`D-CFGBIND-MEMBERSHIP-RULE-2026-09-10`): the second gate exists in `core/node` under the same declaration discipline (`TestNodeConsensusVerdictIsNotAFunctionOfLocalConfig`), driving a real sealed plot and a real space-time answer through the production verifier closure and pinning the measured divergence set; both named fields are now bound to the chain in `ConsensusParams`. **The FIRST face is what this row still tracks**: the closer is a v5 regime that REPLACES the era-1 one | silt-agent-memory/principal-engineer/reviews/ruling-config-in-consensus-gate-05b527c.md + silt-agent-memory/researcher/reviews/research-outcome/GENESIS-CONFIG-FAMILY-BIND-RESEARCH-CERTIFICATION-2026-09-10.md | — |
| `R-CONSENSUS-CONFIG-UNBOUND` | ACTIONABLE | **WIRED ON THE PRODUCTION PATH 2026-09-11 by owner call F's DELIVERY** (`D-CFGBIND-MEMBERSHIP-RULE-2026-09-10`) — the owner ratified the FORMAT act that the correction below records as pending. `genesis.Build` now REQUIRES a `*ConsensusParams` argument (a `BuildWithParams` variant beside a paramless `Build` would have left the same shape available to the next call site); the daemon projects the params off the chain via `Chain.ConsensusParams`, so the arm that WRITES genesis and the arm that CHECKS it read one source; and `CheckConsensusParams` has its production caller as a refuse-to-start, AFTER the REPLAY — the load-bearing boundary, since the check reads `blocks[0]` and is meaningful only against a chain loaded from disk (an earlier draft named the genesis seed and a blind review measured that FALSE: relative to the mint the check is a tautology on a fresh node) — and before `nd.EnableChain`, the upper half of the sandwich. Pinned by `TestG_CFGBIND_6_BuildCarriesAndHashCoversTheParams`, `TestG_CFGBIND_6b_TheParamsCarryingGenesisHashIsPinned` and two source gates, and DRIVEN by G-CFGBIND-10/11 in `e2e` — the instrument of record, added after the review measured the source gates GREEN over a tree where the refusal was dead. **LEG ONE RETIRED 2026-09-11; LEG TWO STANDS.** `Go — multi-process e2e (real TCP)` was not among ruleset `19729396`'s five required contexts, and both e2e tests skip under the `-short` the required job runs, so the MERGE-BLOCKING coverage of the refusal arm was the source gate alone -- which a second blind review measured GREEN over a closure defined between the two landmarks and never invoked (`_ = checkParams`), with the mechanism entirely dead. No stronger lexical gate closes that: a call-string check locates a STRING and an uninvoked closure supplies it by construction. Making that job required was a repo-wide CI-policy call the OWNER holds; **he made it on 2026-09-11 and it is DONE** -- ruleset `19729396` read back **six** required contexts after the write, the five prior ones unchanged and `strict_required_status_checks_policy` still OFF. The flip was applied AFTER the merge that landed the gate, deliberately, because a job must not be made required while the change it was added to protect is still in flight. **THE `UNGATED:` MARKERS STAY, on leg two**: a call-string gate is green over an uninvoked closure whatever CI requires, so a green in `cmd/silt` means "the strings are present and in this order" and never "the mechanism is live". Do not delete the admission on the strength of the six contexts -- that would remove the half that never retires. None of them construct a `Block{Params: ...}` literal, which is exactly how the five earlier gates missed the inert schema. The **membership RULE** replaces the field list: *every field that can change a validity verdict is bound to the chain, or is explicitly excluded with a recorded reason*; 17 is the rule's OUTPUT, and the two declaration tables are a checked bijection onto `ConsensusParams`. **ROW STAYS OPEN** for the surviving paramless path (a pre-bind genesis carries nil and still starts) and the legacy-leg subjectivity no bind can reach. THE CORRECTION THAT PRECEDED THE DELIVERY, kept because a correction stays visible as a correction: **⚠ CORRECTED 2026-09-10 — SCHEMA ONLY; NOT BOUND ON THE PRODUCTION PATH, AND THIS ROW'S "BOUND" CLAIM IS FALSE ON `main`.** Left in place rather than rewritten, per the standing practice that a correction is visible as a correction. Verified at source: `core/genesis/genesis.go` mints genesis as `chain.Block{Version, Height: 0, Entries}` with **no `Params` field**, so cbor `omitempty` drops key 20 and the production genesis hash is unchanged; the only `Params:` literals on a `Block` in the whole tree are in `core/chain/consensusparams_test.go`; and `CheckConsensusParams` has **zero non-test callers**. The field, the key, the comparator and the tests exist and are green — **nothing populates them**. The divergence this row names is still LIVE, and the row's ACTIONABLE state is correct for that reason. Wiring the production mint is a separate task pending an owner call (it moves the genesis hash, so it is a FORMAT act). ORIGINAL CLAIM BELOW: **BOUND ON THE PRODUCTION PATH 2026-09-10** (`D-CFGBIND-BUILT-2026-09-10`): `ConsensusParams`, 17 fields carried by VALUE, committed on the genesis block at cbor key 20 so the genesis hash covers them — a divergent node computes a different genesis and CANNOT JOIN (`ErrForeignGenesis`), which is the right failure surface. Rule 8's two arms COMPOSE: committing the values manufactures the referent a local assertion lacked, so `CheckConsensusParams` is now REQUIRED to catch the one case joining cannot — an operator editing a flag and restarting on a chain already joined. The foreign-genesis refusal is LOUD (was `LogDebug`, now `LogWarn` naming the flags to check, with a `ChainSyncForeignGenesis` stat). Gates G-CFGBIND-1..5, ablations B0-B6 by exit code. **ROW STAYS OPEN** for: the surviving paramless path (a pre-bind genesis carries nil and still starts — disclosed, asserted by G-CFGBIND-5), and the legacy-leg subjectivity no bind can reach. ORIGINAL FINDING BELOW Lane D1 · Builder + Researcher -> **OWNER** ratifies the shape: `MinBond` and `MinBondBytes` change the v5 VALIDITY VERDICT (`validate_v5_predicates.go:246-249`, `validate_v5_quorum.go:222`/`:225`/`:522`, all verified) while being bare command-line flags (`cmd/silt/daemon.go:905-916`) -- two honest replicas with different `-min-bond` disagree on the same block. **I1, the #380 class, third instance**, MEASURED by a driven divergence probe in both quorum regimes (`Quorum` is CLOSED on the objective path; ablating the #380 fix reddens it 2 -> 3 fields). silt names the class in prose as *"consensus-critical genesis config"* (`core/chain/chain.go:190`, `:253`) with **no enumeration and no enforcement**. **The `SlashesBytesCap` refuse-to-start does NOT transfer** -- that invariant is locally checkable arithmetic; `MinBond` divergence is not locally observable, so a start-up assertion has nothing to assert against and would ship a green gate that enforces nothing. **GATE LANDED 2026-09-10**: `core/chain/TestConsensusVerdictIsNotAFunctionOfLocalConfig` -- reflective over every `chain.Config` field (an undeclared field is RED, which caught `Era3ActivationHeight`/`Era4ActivationHeight` that a hand-read had missed), five driven regimes, four-ablation battery verified by exit code. It measured a THIRD unbound field, **`Anchors`** (diverges in the young-anchors launch window), and PINS the unbound set so a fourth instance fails immediately. `Quorum` diverges only on the sanctioned legacy/trusted-opt-out legs, so the #380 fix is now held by a driven assertion rather than prose. Fourteen fields report UNPROVEN, never safe (simplicity rule 7). Closer: bind it to the chain -- (1) commit the genesis-config family into genesis-hash-covered state so divergence is detected at HANDSHAKE, (2) derive it from committed state, or (3) a network-identity digest checked at peering; research prices the three. **MEMBERSHIP CORRECTED UPWARD 2026-09-10 to 17 IN / 5 OUT** (`D-CFGBIND-CERT-2026-09-10`): the 15 `chain.Config` fields that reach a verdict PLUS `BondLabelSamples` and `BondVDFDelay` from `node.Config`. Absorbed by the owner's ruling (a family bind is entries in a digest). OUT for four DIFFERENT reasons: `Archive` (retention only), `WSCheckpoint` (sharing it destroys weak subjectivity), the two rep thresholds (binding is INEFFECTIVE -- the input is the local view), `LivenessRecoveryHeight` (structurally unbindable, so it is a DISCLOSURE and not a row -- `docs/design/m0.md` 10.1). Mechanism CERTIFIED: carry VALUES not a digest, `Params *ConsensusParams` at cbor key 19, pointer per the omitempty-array rule. **ORDERING: land F before or with call A in ONE genesis move, or the graded re-run set is paid twice.** **LANDS BEFORE D3** — reason corrected 2026-09-10 (`D-FREEZE-REPRICE-2026-09-10`): not because a soft freeze locks the repair, but because **D3 gates the B8 engagement and the longest-lead item on the roadmap must not be spent attacking an artifact with a known I1 divergence.** Exploitability LOW (someone must actually set a different value), so it is D1 scope, not an emergency. Canon: `docs/build-process.md` rule 8 | silt-agent-memory/researcher/reviews/research-outcome/V5-SIGNATURE-PREIMAGE-HEIGHT-DELTA-RESEARCH-CERTIFICATION-2026-09-10.md | — |
| `R-BIG-EVIDENCE-UNSLASHABLE` | ACTIONABLE | Lane D1 · Builder -> **OWNER** (a FORMAT item): a double-signer whose evidence pair exceeds `SlashesBytesCap` keeps its on-chain seat, so accountable safety degrades to plain safety -- attribution survives, eviction is lost. Named in the R0.6 delta and the freeze-manifest certifications since 2026-09-03 but **never given a register row until 2026-09-10**, which is why its status kept being asserted from prose rather than tracked. Its reachability is now MEASURED, not theorized (the self-armor face): a lone proposer with throwaway keys arms it at shipped `DefaultConfig` -- no bond, no coalition, no misconfiguration. **CLOSER: the v5 signature-preimage change** (`(height, round, phase)` in `consensusSigBytes`), which makes evidence `O(1)` (~251 B derived, unmeasured until G-PRE-9) so no legitimate proof can be priced out of the block. BOUGHT by the owner 2026-09-10 (`D-PREIMAGE-BUY-2026-09-10`, call 1), GATED on its delta cert. **(d-3) does NOT close it** -- that claim is C-3/C-4 of `R-CERT-REDERIVE` and it fell; (d-3) is a ~40x shrink. Row closes when the preimage change lands and the self-armor gate flips from reporting to asserting. **TWO MEASURED FACES FOLDED IN 2026-09-10 — three rows collapse to one, because all three end in the same sentence (the equivocator keeps its seat) and all three close on ONE act: the `(height, round, phase)` signature preimage making evidence `O(1)`.** **Face 2, the honest-path nesting fixed point.** `Equivocation` carries two FULL `Block`s and a `Block` carries its own `Slashes`, so `cap >= 2 x body + overhead` with `body` including `Slashes <= cap` has NO positive solution at any cap. MEASURED on signature-valid fixtures at the SHIPPED defaults, no coalition and no misconfiguration: a block committing two ordinary 4.14 MiB proofs is VALID and a LEGITIMATE proof about it is 17,373,935 B, 596 KB OVER CAP. This is the only face reachable on the honest path. **Face 3, the self-armor, DRIVEN AND CONFIRMED on real production code (#795).** A 687 B throwaway-key proof about never-committed blocks is ACCEPTED as evidence; 24,456 of them pack `Slashes` to 16,776,819 B (397 B under cap) and that block is VALID under `v5ValidateSlashes` AND `ValidateProposal` at shipped `DefaultConfig`; a legitimate proof about it is 33,555,157 B, REJECTED. Measured armor threshold **~7.9998 MiB/side** (12,227 admissible, 12,228 over), **CORRECTING the certification's derived ~8.39 MiB by ~5 % -- do not re-cite 8.39.** The cheap route is HEADER-ONLY proofs, not the reg-laden ~4.14 MiB proofs both the PE table and the cert modelled. The landed gate REPORTS rather than asserts (no repo convention exists for a hard-skip-until-fixed) and its doc names exactly what turns its `t.Logf` into `t.Fatalf`: the preimage change at D1. **Closer for all three: the preimage change lands and the gate flips to asserting**; the fallback is a validity rule bounding the encoded block body (it binds PEERS, which no start-up check can) or fixed-size evidence via the v5 two-level block hash (d-3). May partly refute the R0.6 value certification; the OWNER ratifies | silt-agent-memory/researcher/reviews/research-outcome/SLASHCAP-NESTED-EVIDENCE-FIXED-POINT-RESEARCH-CERTIFICATION-2026-09-10.md + silt-agent-memory/principal-engineer/reviews/RULING-slashcap-config-route-close-CODE-2026-09-10.md + silt-agent-memory/researcher/reviews/research-outcome/SLASHCAP-NESTED-EVIDENCE-FIXED-POINT-RESEARCH-CERTIFICATION-2026-09-10.md | — |
| `R-CERT-REDERIVE` | ACTIONABLE | Lane D1 · Researcher (owner) → the four sentences close individually: the 2026-09-10 nested-evidence certification's §8 fells FIVE prior sentences (C-1..C-5). C-5 was the OWNER-RATIFIED one and is CLOSED by owner call 2 (`D-PREIMAGE-BUY-2026-09-10`). The other four are certification sentences whose CLAIMS now have no support, and the reviewing engineer's instruction is that annotating them is NOT re-deriving them. **C-1** (I5-cross-height cert §7): the two constraints do not bracket -- the bracket is EMPTY; 16 MiB was chosen against a RESTRICTED floor (largest legitimate pair among blocks whose `Slashes` is empty) that was never stated, so re-derive the floor the value actually satisfies and state the restriction. **C-2** (R0.6 delta V-2): "an honest-shaped pair is always slashable" is gone; the inequality survives only as arithmetic about the NON-`Slashes` body -- scope every citation to that (ledger half done in `D-SLASHCAP-ROUTE`'s SUPERSESSION note; `core/chain/chain.go:412-414` already corrected). **C-3** (R0.6 delta §5.2): re-derive what (d-3) actually delivers, on the HEADER-ONLY proof route, since the measurement refuted the reg-laden route both the PE table and the cert modelled. **C-4** (freeze-manifest cert §4.3): falls in its first third -- the late-reveal face (CLOSED 2026-09-11 with (d-3), #800) and `R-CARRIER-PRUNED-HASH` close as stated, `R-BIG-EVIDENCE-UNSLASHABLE` does not; **this one is INSIDE THE FREEZE MANIFEST so it is D1 content, not bookkeeping**, and the spec repair (the v5 preimage carries a single `SlashesDigest`, never a `Slashes'` copy) is a FORMAT question the owner takes individually. Item 3 stays BOUGHT on its surviving two-thirds. **A FOURTH ITEM ADDED 2026-09-10 — and this one is about the METHOD, not a sentence.** The (d-3) delta certification asserted *"omitempty already gives that property for every additive field"* while the repo carries a comment saying it does NOT (`core/chain/chain.go:569-572`) and a LIVING TEST proving it (`core/chain/lastcommit_carrier_pins_test.go:35-37`, `Pruned` at key 14 of every encoded block). **A certification contradicting an in-repo pin is the THIRD instance of certification-MODELLING failure**, after the wrong attacker route (header-only, not reg-laden) and the mis-attributed value (the armor threshold derived ~5% off). Filed here as evidence the METHOD needs review rather than as a fourth sentence to re-derive — owner direction, 2026-09-10: *"Don't open a new row; add it to the existing one."* Row closes when all four sentences have a re-derived replacement on record AND the modelling-failure pattern has a disposition | silt-agent-memory/researcher/reviews/research-outcome/SLASHCAP-NESTED-EVIDENCE-FIXED-POINT-RESEARCH-CERTIFICATION-2026-09-10.md | — |
| `R-G1-POSTLATCH-DRIVEN` | ACTIONABLE | Lane D1 · Tester: G-1's entry assertion is CLOSED and driven (`core/chain/TestColdBox_G1_WiredVerifierWithZeroMinBondStallsAtEntry`, red-first), and because the entry returns before any screen it covers every path into the box. What is NOT yet DRIVEN is the maturity recompute's own legacy/objective divergence on a POST-LATCH block — the shape the freeze-manifest cert 4.11 names as the live risk. The ablation showed the class-A screen caught the mid-epoch fixture, so that row proves the ENTRY, not the maturity path. Closer: one driven post-latch row; filed rather than claimed, because asserting more than the fixture exercises is the failure this repo lints for | silt-agent-memory/researcher/reviews/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md 4.11 | — |
| `R-BONDREG-SINGLE-OVERSIZE` | ACTIONABLE | Lane D2 (stamp raise) · Builder → Researcher: silt has NO per-reg byte cap, and `core/node/chainrole.go:902` embeds the FIRST fresh registration unconditionally when the block carries no regs yet ("never stall the queue on a single oversized proof"), so one arbitrarily large registration exceeds the configured budget and re-opens the gap `D-SLASHCAP-ROUTE` closed for the budget itself — the headroom check bounds the BUDGET, not this overflow. Closer: a per-reg byte ceiling, which is a validity rule of its own and rides the stamp-raise train (a WIDENING change later is outside the narrowing exemption) | silt-agent-memory/principal-engineer/reviews/RULING-derivation-route-audit-pre-freeze-2026-09-09.md | — |
| `R-COMPACT-ORPHAN` | ACTIONABLE | Lane C10 · Builder: the BENIGN compaction-failure class has no daemon WARN line — surface `CompactFailures` / `LastCompactError` on the banked/status path (WIP `builder/c10-compact-orphan-warn` @ `49eb5d1`: the line + both ablations RED; suites not run) | silt-agent-memory/principal-engineer/reviews/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-PRIVACY-OPERATOR-TAB-TOKEN` | ACTIONABLE | Lane C10 · Builder: a persistent operator-token route is the UX follow-on before the `-privacy` default reaches real operators (`D-UI-PRIVACY-FLAG`) | silt-agent-memory/principal-engineer/reviews/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-membership` | ACTIONABLE | Lane TAIL (POST-RC) · Builder: **the digest retirement is DROPPED — `D-MEMBERSHIP-KEEP-FIVE-2026-09-11` REVERSES owner call 1 of `D-TRUE-UP-CALLS-2026-09-07`. The v5 digest set freezes at FIVE.** ~~The closer is **G-3, the witness id-list size gate, still UNBUILT** — that is the mechanism that bounds the two grow-only sets~~ — **SUPERSEDED 2026-09-11 by the scope ruling (`silt-agent-memory/researcher/reviews/research-outcome/2026-09-11-G3-witness-id-list-size-gate-SCOPE-RULING.md`), left visible so the correction reads as one. `R-membership G-3` caps what ONE BOX accepts as a witness; it bounds neither `len(c.slashed)` nor `len(c.validatorsSeen)`, which `(*Chain).apply` writes and which the gate has no contact with. THIS RESIDUAL'S CLOSER IS UNNAMED** — what closes it is a bound on set growth, and no such instrument is on the plan. Under simplicity rule 4 a residual with no named closer is a disclosure rather than a row; **that demotion is the OWNER's call and is reserved to him**, so the row stands ACTIONABLE with its closer recorded as unnamed rather than being demoted or handed an invented one. Retiring `slashedRoot` / `validatorsSeenRoot` bounded nothing either and was assigned to a residual it does not close. **`R-membership G-3` itself is KEPT AND RESCOPED, post-RC:** its deadline is whichever of the accept flip (R1.8 / #657) or the witness-transport merge lands first — neither a FORMAT item nor a stamp-raise item, but box-local admission on a door that returns no verdict. Half A of it (the O(N) fold over `w.PreIDs` in `anchoredPreSet` / `digestFoldOp`, both still ungated) is **held in tension, not closed**: `(*Box).Validate`'s FIRST statement is `s.budget.Check(len(Encode(&b))+witnessBytes(w), "frame+witness")` and `recomputeStateRootEntriesRevocations` has exactly ONE non-test caller — that door — so BG-3 fires first, but that is a single-caller property of an unexported function and `floorbox_door_inventory_v5_test.go` pins only the exported surface, so one new in-package caller re-opens it. Half B (the allocation BEFORE any measurement) is open and unreachable: `witnessBytes` walks an already-materialized struct, no file under `adapters/` or `ports/` names `StateRootWitness`, and `NewBox` has zero non-test callers. Half B shares ONE instrument with `R-R3-GOB-ALLOC-AMPLIFICATION`'s decoder limit — do not build two. **G-2 is dropped with it, so `D-RECOMPUTE-FREEZE` is untouched.** Driven: the two roots are the only set-completeness anchor a root-only holder has (`provenView.members`, `anchoredPreSet(byTag, tagSlashedRoot)`, `recomputeMatureNowStreaming`'s `validatorsSeenRoot` proof — the C2 quantity the anchor shed gates on); removing the two emits turns 70 top-level tests and 31 subtests RED in `core/chain`; keeping them costs 2 leaves / 95 bytes, fixed and independent of N. **No longer a FORMAT item and no longer in the D1 train — and the lane tag said otherwise.** RECORD CORRECTION 2026-09-11: this row's tag read `Lane D1` while this same body said the row is out of the D1 train, so the row CONTRADICTED ITSELF; the tag is what moved, to `Lane TAIL (POST-RC)`, because the body's claim is the one the scope ruling confirms. | silt-agent-memory/researcher/reviews/research-outcome/R-membership-unbounded-sets-and-recovery-boundary-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md | — |
| `R-R3-GOB-ALLOC-AMPLIFICATION` | ACTIONABLE | Lane D1 · Builder: a 5-byte hand-crafted gob length prefix forces a ~10 MB allocation in `proof.Unmarshal` — `SProofMax` bounds ENCODED bytes, not parse memory (`core/statehash/witness_bound.go:78`); bound parse memory with a decoder limit in the stamp-raise train (deadline moved 2026-09-08 from the frozen flip). **RE-PRICED 2026-09-11: the host function is not in the shipped binary** — `IngestBlockWitnesses` has zero non-test callers and is ABSENT from `./cmd/silt`, so this bound would ship inert. See `R-INGEST-WITNESSES-INERT`; that disposition comes first. **ONE BUILD WITH `R-membership G-3`'s HALF B** (scope ruling 2026-09-11): the id-list allocation that precedes any measurement is this same class in this same seam, and the decoder-level parse-memory limit is the single instrument for both. Do not build two. | silt-agent-memory/principal-engineer/reviews/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-CARRIER-BYTES` | ACTIONABLE | **DECLINED FOR THE RC 2026-09-11, DISCLOSED, ROW KEPT** (`D-CARRIER-BYTES-DECLINED-2026-09-11`, owner call 3 answered) — the cap is NOT built before the RC and the exposure is carried knowingly into the B8 engagement as freeze-manifest item 22. **Declined, not deferred: the difference is that the exposure is now written down as accepted rather than as owed.** WHAT IS ACCEPTED, in one sentence: on the RC's own field network `validateCarrier` admits a `Block.LastCommit` of any size the transport frame allows (~1.3M entries in 132 MiB), each entry costing one `ed25519.Verify` on every replica that validates or reloads the block, and that cost is permanent because the field is hash-covered. **WHY IT IS DECLINED rather than built:** the cap turns on a pony-class honest-maximum measurement nobody has run, and a cap set below the honest floor is a liveness wedge — worse than no cap. Building it without the measurement would trade a CPU-exhaustion nuisance for a publish-path stall (the same G-CB-1 / G-QC-1 / G-ATTS-1 defect). **WHAT THE DECLINE DOES NOT COST:** it is a narrowing validity rule, outside the four doors, so its deadline is the stamp raise and it is NOT a freeze item — declining it forecloses nothing and costs no era. **THE ROW STAYS OPEN** because the rule is still wanted: the decline is scoped to the RC, and the closer below is unchanged. **Lane D2 (stamp raise) · Builder → **OWNER**.** *Filed 2026-09-11 as restored — freeze manifest item 8, deleted twice on wrong grounds and a row again:* The per-block ceiling on `Block.LastCommit`. `validateCarrier` (`core/chain/carrier.go`) enforces phase, `verifyAtt` over `b.Prev` and per-id distinctness and has **no count cap, no byte cap and no qualification screen** — verified at HEAD, and there is no `CarrierBytesCap` or `G-CB-1` symbol in the tree. **Why the two prior deletions were wrong.** (1) It left the manifest labelled *"it served the recompute"*, which the certification's §4.10 refutes in terms (*"the box is not the binding constraint at any admissible value; the node's CPU is"*). (2) It was then re-homed onto `R-CARRIER-ATTS-NORMALIZE` / `R-CARRIER-ATTS-PREPAREQC`, **a different field**: `Block.LastCommit` is in **both** `bodyHash` preimage literals and `Block.Atts` is in **neither**, and the certified `Atts` fix is acceptance-time normalization, which cannot apply to a hash-covered field without changing the block hash. **Re-priced by the era-4 flag default:** the surface is now reachable on the RC's own field network, since every fresh network mints era-4 from height 1 — the deadline class is unchanged (a narrowing validity rule, outside the four doors) but the build order is not. **The row exists so this is DECIDED rather than deleted a third time.** Five production comments cite it *"in ROADMAP.md"* (`carrier.go`, `readset_v5.go`, `stateview_v5.go`, `floorbox_recompute_stateroot_v5.go`, `floorbox_recompute_stateroot_atts_v5.go`) and `ErrWitnessBudgetExceeded`'s failure text names it. **Closer:** the owner's call — IN the RC (which needs the pony-class honest-maximum measurement first; a cap set without it risks landing far below the honest floor, which is worse than no cap), or an explicit DECLINE recorded here and disclosed in the B8 brief (manifest item 22). The bound's three standing tensions — producer discretion, the class split across `LastCommit`/`Slashes`/`IssuerKeys`, and registry order — stay as one disclosed tension line in [`docs/design/m0.md`](docs/design/m0.md), not as a second row | silt-agent-memory/principal-engineer/reviews/RULING-era4-freeze-manifest-re-audit-f826c72-2026-09-11.md | — |
| `R-CARRIER-QC-NESTED-ROUNDCHANGE` | ACTIONABLE | Lane D1 · **OWNER** (a routed direction) — **ROUTE THIS AHEAD OF THE CAP**; the certification calls it the most severe item in the QC family and it is in SHIPPED code on main. `(*roundChangeEnv).sigBytes` signs `domain, Height, NewRound, LockRound, HashBytes(LockBlock)` and **omits `LockQC`**, so a replayed genuine envelope from a qualified sender may be padded in flight and still pass both the signature check and `(*Chain).AttesterEligible`. `(*Node).acceptRoundCert`'s shipped G-H43-12 guard bounds the OUTER list only — **the gate whose own doc says it bounds the work before any signature does not bound the work.** Two consequences: (a) 131,072 verifies per admitted envelope INSIDE the shipped cap, closed by G-QC-5; (b) **a liveness veto** — one padded entry makes `(*Node).verifyRoundChange` fail and `(*Node).newViewFor` hard-fails the WHOLE new-view certificate on its first bad envelope, so any peer can veto any new-view certificate it can observe. (b) closes only by bringing `LockQC` under `sigBytes` (a wire-format change with a rolling-upgrade break) or by making `newViewFor` skip a bad envelope instead of hard-failing — a `#432` safety question the certification explicitly declines to decide. **Absorbs the filed sign-mark face (LOW), which is this fix's migration tail, one PR:** sign marks persisted BEFORE the normalization keep their padding and are re-broadcast after a restart, so `(*Node).roundsFor`'s re-hydration must trim or the first `(*Node).advanceToRound` after an upgrade re-emits the old padding once | silt-agent-memory/researcher/reviews/research-outcome/R-CARRIER-ATTS-PREPAREQC-wire-bound-on-env-QC-RESEARCH-CERTIFICATION-2026-09-10.md | — |
| `R-CARRIER-ATTS-PREPAREQC` | ACTIONABLE | Lane D1 · Builder → Researcher (direction CERTIFIED, verdict **GATED**): `(*Chain).VerifyPrepareQC` runs one `ed25519.Verify` per entry of an attacker-supplied `env.QC` with no cap, and `(*Chain).collectQuorumSigs` sets `seen[id]` only for QUALIFIED ids, so identical entries are NOT deduplicated — **one replayed signature buys 131,072 verifies**, no keypairs and no storage step required. The bound `len(qc) > c.GoverningSetCap() + 2` refuse-before-the-first-verify is certified, applied INSIDE `(*Chain).VerifyPrepareQC` (which has ZERO block-validity callers, so the rule is pure wire admission and forks nothing) under the `c.objective()` guard; the `+ 2` is not a chosen constant, it is the seed list `(*Node).gatherTwoPhase` builds before it solicits anybody. **GATED on G-QC-1:** the shipped honest maximum for `len(env.QC)` is `chainhost.Host.Attesters`, the `-attesters` flag unfiltered — a consensus-adjacent quantity read from local config (`#380` class, fourth instance) — so landing the cap without the paired producer change turns a CPU-exhaustion nuisance into a **publish-path liveness wedge** for any operator with a wide `-attesters` list. Same defect as G-CB-1 and G-ATTS-1; ONE commit discharges all three. **Absorbs the block-validity face:** `b.PrepareQC` at `(*Chain).ValidateCommit` is uncapped and rides the carrier's commit, never this wire step. **The cap is not the load-bearing instrument** — on a saturated registry it buys 7.8x, not the ~8,700x it buys on a 13-seat field network; the sender screen this arm does not have is what makes a per-sender rate budget mean anything, and `bondSubmitBurst` and `entrySubmitBurst` have no screen either | silt-agent-memory/researcher/reviews/research-outcome/R-CARRIER-ATTS-PREPAREQC-wire-bound-on-env-QC-RESEARCH-CERTIFICATION-2026-09-10.md | — |
| `R-CARRIER-QC-BURST-VALUE` | ACTIONABLE | **NEW 2026-09-11 — the arm #823 left open.** Lane D1 · **Tester** (the honest-cadence measurement) → **Researcher** (certifies the value) → **OWNER** ratifies — a **security parameter**, so it is behind the research gate and #823 declined to guess a number rather than ship one. #823 shipped the sender screen on the `ports.MsgPrepareQC` arm (`(*Node).handleChain`), which closes the flood for an UNBONDED sender. **A sender already in the governing set passes that screen and the flood survives intact.** `(*Chain).AttesterEligibleAt` is a property of the SENDER, never of the list it carries, so a bonded validator reaches `(*Chain).VerifyPrepareQC` with an `env.QC` of any length the CBOR decoder admits — the fxamacker default `MaxArrayElements` of 131,072, which is the only ceiling in the path; there is no length check, no count cap and no rate budget between the decode and the verifier. **THE FAILURE MODE, STATED EXACTLY, BECAUSE THE OBVIOUS STATEMENT OF IT IS FALSE.** The attacker does NOT replay its own bonded signature: `(*Chain).collectQuorumSigs` sets `seen[id]` at the BOTTOM of its loop, after the qualification test, so the `if seen[id]` short-circuit fires **only for ids that already qualified** — repeats of the attacker's own bonded key are deduplicated before `verifyAtt` and buy exactly ONE verify. The dedup is ASYMMETRIC, and the open half is the other one: repeats of an UNQUALIFIED id are never deduplicated, so each pays a full `verifyAtt`. **The bond therefore buys PASSAGE THROUGH THE SCREEN, not the payload** — the flood entries are signed by an identity that is in no governing set, which costs one offline `ed25519.Sign` and no bond. Both halves are DRIVEN by `core/node` `TestQC_BurstValue_DedupIsAsymmetricAcrossQualification` (a corrupt entry placed last; `ErrBadSignature` is a positive observation that the loop walked to it) and the screen's boundary by `TestQC_BurstValue_TheSenderScreenAdmitsAQualifiedFlooder`; moving `seen[id] = true` above the qualification test reddens the first, verified by ablation. **WHAT FIRES FIRST — checked, because a residual's failure mode is its own claim.** In arm order: the sender screen (passed), `cbor.Unmarshal`, `chain.Decode`, then `VerifyPrepareQC`. The sign-mark watermark `(*Node).signAllowedAt` runs **after** `VerifyPrepareQC`, so it does not protect — the CPU is spent before any watermark is consulted. **COST TO THE ATTACKER: one bond, and nothing else.** The refusal is a `ports.MsgPrecommitReply` `OK=false`, consumed only by `(*Node).gatherTwoPhase`'s precommit callback, which logs it — no ledger entry, no audit, no standing loss, and nothing slashable, since a padded list is not equivocation. The list never reaches quorum (`ErrNoQuorum`), so the block is refused and the work is pure loss for the victim. At the 131,072 ceiling and the PE's measured 52.6 us/op that is ~6.89 s of one core per message, for ~13.1 MiB of wire. **Closer:** the per-sender rate budget the certification pairs with the screen — its burst constant derived from a MEASURED honest prepare-QC cadence, then owner-ratified (gate G-QC-6). `roundCertBurst = 4` (`core/node/bondaudit.go`) does **not** transfer as a value: one proposer legitimately gathers many heights per window. The bound is a wire-admission rule with zero block-validity callers, so it forks nothing and is not a freeze item. The QC family's standing tension in [`docs/design/m0.md`](docs/design/m0.md) §10.1 is now half-discharged here: a rate budget without a sender screen is re-pricing rather than elimination, and #823 supplied the screen this arm was missing — which is what makes a budget worth sizing at all | silt-agent-memory/researcher/reviews/research-outcome/R-CARRIER-ATTS-PREPAREQC-wire-bound-on-env-QC-RESEARCH-CERTIFICATION-2026-09-10.md | — |
| `R-CARRIER-ATTS-NORMALIZE` | ACTIONABLE | Lane D1 · Builder → **OWNER** (it touches I5): `(*Chain).validateStructural` verifies **every** entry of `b.Atts` with no cap and no qualification screen on the reload path, and `Atts` is **not hash-covered**, so any relay can pad any committed block in transit and every receiving node stores and re-verifies the padding on every restart, forever. `(Block).Prune` keeps `Atts` at every depth. **A narrowing cap at `validateStructural` is REFUTED (T-DISPOSAL)** — the fix is acceptance-time normalization plus a wire cap, and ONE fix closes all three faces filed. **Face 2, quadratic equivocation:** `(*Chain).FindEquivocations` iterates `signers(ab)`, which is `1 + len(PrepareQC) + len(Atts)` of our own STORED block, and `CheckEquivocation` recomputes both `bodyHash()`es on every call, deliberately bypassing the memo (R0.6 G-6) — on a poisoned block that is 262,144 full-body marshals per fork-detection sweep, called from `(*Node).slashEquivocators` on **every fetched peer chain**, so a poisoned node goes CPU-dead on sync, not merely slow on restart. **Face 3, evidence narrowing (the I5 clause, and the reason the OWNER decides):** normalization drops entries from ids unqualified at that height while `(*Chain).signers` reads `b.Atts` as equivocation evidence — an unqualified signer's vote moves no quorum, so it narrows the slashable set in the SAFE direction and partly closes `R-SLASH-CULPRIT-ADMISSIBILITY`. **⚠ THE ROUTED EXPOSURE FIGURE IS WRONG BY 10x AND IS CORRECTED HERE, NOT LAUNDERED.** 1,318,209 entries / 132 MiB / 42-68 s is UNREACHABLE: `chain.Decode` calls `cbor.Unmarshal` on the package-default `DecMode` and `core/chain` sets `MaxArrayElements` nowhere, so fxamacker/cbor v2.9.2's `defaultMaxArrayElements` of 131,072 is enforced BEFORE allocation. True ceiling **131,072 entries / 13.1 MiB / 4.2-6.9 s**, amplification **15.5x, not 156x**. **The finding survives in a WORSE dimension:** the cost is cumulative and permanent, so a poisoned 1,000-block history is **13.1 GiB on disk and 70-115 minutes per daemon start, forever** | `silt-agent-memory/researcher/reviews/research-outcome/R-CB-ATTS-UNBOUNDED-validateStructural-reachability-and-bound-RESEARCH-CERTIFICATION-2026-09-10.md` | — |
| `R-CARRIER-ATTS-BLOCKS-CEILING` | ACTIONABLE | Lane TAIL → **Boulder 5 (POST-RC), and it is a launch blocker when reached** · Builder: `chain.DecodeBlocks` decodes the whole `chain.cbor` as ONE cbor array under the same package-default 131,072-element ceiling, so **a chain longer than 131,072 blocks cannot be loaded by `chainstore.Load` at all**. Not urgent; genuinely terminal when reached, and it arrives on a calendar, not on an attack. Closer: a chain-load `DecMode` with a raised `MaxArrayElements`, or a streaming load. Neither is a consensus rule, so this does not ride the freeze | `silt-agent-memory/researcher/reviews/research-outcome/R-CB-ATTS-UNBOUNDED-validateStructural-reachability-and-bound-RESEARCH-CERTIFICATION-2026-09-10.md` | — |
| `R-CARRIER-PRUNED-HASH` | ACTIONABLE | Lane D2 · Tester: prove the seen-fold never depends on a pruned body and, end-to-end, that the first non-pruned descendant's root check catches a rewritten ancestor; G-D11 (two-sided) shipped in Round 1A, bounded not eliminated; deadline the stamp raise. **SCOPE HALVED 2026-09-11:** (d-3) is built (#800), so a pruned era-4 block recomputes its own hash and its retained body is self-covering — per the manifest cert §4.13 only the **pre-era-4** half of this obligation survives. `core/chain/chain.go` still carries the in-source marker *"OPEN, bounded not eliminated, deadline the stamp raise"*; scope the remaining half against the pre-era-4 legs only | silt-agent-memory/researcher/reviews/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md | — |
| `R-E2E-ERA4-FIXTURE` | ACTIONABLE | **PARTIAL 2026-09-11 — the FORMAT half closed by accident, and that is the tell.** Lane D2 · Tester + Builder; freeze manifest item 17. **Closed half:** no e2e or cloudtest harness passes `-era3/-era4-activation-height`, so every e2e daemon runs the binary default of **1** and mints era-4 from height 1 — the era-4 block format, `validateCarrier`, `validateIssuerKeys`' era gate, the (d-3) preimage and `tagRevLogSize` are now exercised on the EXISTING fixtures, with no fixture work done. Source-pinned: `core/chain/equivocation.go` calls `Era4ActivationHeight = 1` *"the source-pinned default"* and `core/node/adversary.go` says so too. **Owed half — the POSTURE:** objective + bonded + epoch-enabled. Every paid-lane and consensus e2e still launches `-objective=false`, so the paid lane's POSITIVE arm still has no e2e coverage; the e2e cost was ACCEPTED (owner call 14). **The row's premise is CORRECTED, not rewritten.** It read: *"the e2e delivery-receipt daemon runs `-objective=false`, so the era-4 readiness tally can never latch on that fixture … (never expose `Config.Era4ActivationHeight` to a harness)."* The tally is not what gates era-4 any more: `(*Chain).era4Active` takes the config branch whenever the height is non-zero and never consults the tally, so a non-objective topology latching nothing is beside the point. The pin `core/chain TestGateF_NonObjectiveTopologyCanNeverLatchEra4` still holds — it drives the `Era4ActivationHeight = 0` LATCH branch, which is now the non-default route. **Do not close this row on the format half.** **POSTURE HALF BUILT 2026-09-11; what remains is ONE named blocker, and it is not a fixture problem.** The posture gate is `core/node TestIssuerKeyBindingResolvesInTheObjectiveBondedEpochPosture`: objective (`MinBond` > 0 + a wired bond verifier) + bonded (the issuer's `BondReg` must COMMIT before `IssuerKeyRegAdmissible` lets the registration ride) + epoch-enabled (`EpochBlocks` = 4, crossed by real blocks) + era-4 — the binding for **epoch 1** commits in a **v5 block at height 4, block epoch 1**, through the real proposer fold and the real `validateIssuerKeys`, never the genesis door — and it drives ACCEPT (pins 1), REFUSE-ABSENT (pins 0) and REFUSE-MISMATCH (pins 0) on that ONE fixture with a CHAIN under the fetcher in every arm. Each refusal clause has its own arm, ablated one condition at a time: dropping the whole commitment check reddens the ABSENT arm (exit 1); dropping only the fingerprint equality reddens the MISMATCH arm (exit 1) while ABSENT stays green. **THE ROW'S FORMAT-HALF SENTENCE IS CORRECTED BY MEASUREMENT, not rewritten:** the activation default exercises era-4 only on fixtures that COMMIT ABOVE GENESIS. On a committing topology (the `TestBondEarnedStandingCommitsOverTCP` argv, stores kept) `silt chain-status` reports `head version: v5` and `era-4 (v5): ACTIVE — first v5 block at height 1`; on the paid-lane fixture's exact argv it reports `no chain yet (0 blocks)`, the daemon banner reports `head v2 at height 0` and `era-4 (v5): DARK`, and NOTHING era-4 is exercised there. **STILL OWED — the OS-PROCESS POSITIVE ARM, blocked on `R-CLIENT-HAS-NO-CHAIN` (below).** MEASURED 2026-09-11: `TestPaidDeliveryLaneRefusesWithoutACommittedKeyBinding`'s refusal is NOT attributable to the cause it names — `silt swarm receipt` resolves on the chain-less ephemeral node `joinSwarm` builds, so `pinDemandIssuerKey` refuses at `n.chain == nil` first, and deleting the commitment check outright leaves that test AND `TestPaidDeliveryLaneArmsInTheHarnessPosture` GREEN at exit 0. Making those daemons commit is necessary and NOT sufficient; the client still holds no chain. **⚠ OWNER CALL 7 DOES NOT RELEASE THIS ROW** (`D-TOKEN-DOMAIN-CHAINLESS-CLIENT-2026-09-12`): the posture half rides `R-CLIENT-HAS-NO-CHAIN`, a different quantity on a different lane, and it is still blocked after call 7 is answered. | silt-agent-memory/researcher/reviews/research-outcome/R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md | — |
| `R-CLIENT-HAS-NO-CHAIN` | ACTIONABLE | **NEW 2026-09-11 — the named blocker under `R-E2E-ERA4-FIXTURE`'s remaining half, and an OWNER call because it moves the CLIENT posture, not a fixture.** Lane D2 · **OWNER** → Builder. No shipped `silt` process is both chain-bearing and a paid-lane fetcher: `joinSwarm` (`cmd/silt/daemon.go`) builds the one-shot client's node and never calls `EnableChain`, `cmd/silt/client.go` enables none either, and the daemon's is the ONLY `EnableChain` call site outside tests in `cmd/`. `cmd/silt/swarm.go`'s `swarm receipt` is the only production caller of `FetchDemandIssuerKeys` / `AcquireDemandTokenInWindow` / `OpenDeliverySessionRemote` / `SubmitDeliverySettle`. **CONSEQUENCE, MEASURED 2026-09-11:** `pinDemandIssuerKey` refuses at `n.chain == nil` before its committed-binding check on every shipped withdrawal, so the paid lane's POSITIVE arm is unreachable at the OS-process tier and both paid-lane e2e refusals stay GREEN with that check deleted outright. **This is NOT the same claim as "the fixture commits nothing"** — making the daemons commit (measured: a committing topology reports `head version: v5`, `era-4 (v5): ACTIVE`) leaves the client's nil chain untouched. **THE DECISION IS THE OWNER'S:** the D3 architecture already names the answer — a DURABLE PARENT resolves against ITS chain and hands the (key, epoch) pair to the chain-less ephemeral (`Node.ResolvedDemandIssuerKey` → `AcquireDemandTokenWithCredit`, `core/node/demandrole.go`) — but `swarm receipt` does not use it, and wiring one changes what a shipped client posture is. Not designed around; `R-E2E-ERA4-FIXTURE` is held on it rather than closed with a fixture that cannot assert the arm. **⚠ THIS ROW IS NOT CLOSED BY OWNER CALL 7** (`D-TOKEN-DOMAIN-CHAINLESS-CLIENT-2026-09-12`, ANSWERED 2026-09-12). The two were FUSED and are now separated: call 7 asks whether a chainless client can derive a TOKEN DOMAIN — answer yes, it carries 32 bytes — while this row asks whether any shipped `silt` process is both chain-bearing and a paid-lane fetcher. **The client's nil chain is untouched by that answer**, so `pinDemandIssuerKey` still refuses at `n.chain == nil` and both paid-lane e2e refusals still stay GREEN with the commitment check deleted. **Nobody may close this row, or `R-E2E-ERA4-FIXTURE`, on the call-7 verdict.** | silt-agent-memory/builder/fixture-posture-vs-predicate.md; measurement: the ONE non-test `EnableChain` call site in `cmd/` is the daemon's, not `joinSwarm`'s | — |
| `R-CLOUD-ERA-PROBE` | ACTIONABLE | **BOTH CODE HALVES BUILT 2026-09-11 — the row stays ACTIONABLE for the release-runbook line alone.** Lane D2 · Builder: expose the chain's block era on a CLI/status surface so the cloud sheet's `13b-delivery-settlement` can tell "era-4 dark" from "keys off-commitment" (freeze manifest item 19). **BUILT:** `silt chain-status` and `GET /api/status` `.chain.era` now carry the head `Version`, the per-version census with each version's first height, the two era statuses and `max_h len(blocks[h].Atts)` with the height attaining it — the figure two certifications name as unmeasured. `chain.EraState` reads `Config` NOWHERE: the era override would be a `#380`-class local read, and rebuilding the readiness latch inside `chain-status` would make the reported activation height a function of a CLI flag, so ACTIVE is defined from the committed blocks instead (activation is a MINT boundary, so a committed v5 block and era-4 activation are the same fact). **The predecessor's vacuity is guarded, not just noted:** `EraPhase` is a closed string enum with no zero value, optional heights are `*uint64` so a present 0 cannot pass for an absence, and a measured zero carrier is narrated. Seven gates, FIVE ablations RED first (config-read, wrong reducer, PENDING collapsed into DARK, the offline path asserting a tally fact it cannot see, and absent-heights-as-zero); all three phases DRIVEN by a real readiness tally. Not a format item. **THE DAEMON HALF IS NOW BUILT** (2026-09-11, second PR): at start-up the daemon prints `chain.DeclaredMaxBlockVersion` — a COMPILE-TIME constant, reachable by no flag, asserted as a constant at compile time and checked against this build's real decode and mint ceilings — beside the era state of the chain it just loaded, because a chain carrying no v5 block is a HEALTHY DARK network under an era-4 build and the WRONG BUILD under an era-2 one, and neither number discriminates alone. Nine gates, ELEVEN ablations RED first, the four-cell {declared v5, declared v2} x {dark chain, era-4 chain} matrix driven on chains that mint their own v5 blocks and latch their own tally. **OWED — the row does NOT close yet:** the release-runbook line, which `docs/era4-freeze-what-closes.md` places at the STAMP RAISE rather than at the freeze; no runbook document is named for it, and inventing one would be a guess. | silt-agent-memory/principal-engineer/reviews/RULING-cloudtest-delivery-lane-flow-aab3626-2026-09-07.md · `docs/thinking/2026-09-11-cloud-era-probe-design.md` · `docs/thinking/2026-09-11-era-declared-startup.md` | — |
| `R-CARRIER-GENESIS-DISPOSAL` | ACTIONABLE | Lane E3 · red-team: both code halves SHIPPED (#720, #723); the O-2 probe is owed — a "pruned" genesis served to a fresh-sync victim with our hash and an attacker-chosen body (`AppendGenesis` never checks `IsPruned()`), carrying the PE's composition (attacker-declared `BondRegs` qualifying its own keys under a kept `Pruned`); if it lands, the Tester encodes it and the Researcher certifies the refusal | /archive/roadmap-boulders-detail-2026-09-07.md L681–734 (the O1/O2 carrier ratification and the O-2 probe) | — |
| `R-CARRIER-DOUBLESIGN-SLOT` | ACTIONABLE | Lane TAIL (LOW) · Builder + Tester: the evidence producer LIFTS a carried precommit onto the evidence copy of the parent's `Atts` (hash unchanged, no format, no era gate; NOT a consensus-rule change), with the SILENT-GREEN and FALSE-SLASH traps as Tester gates | silt-agent-memory/researcher/reviews/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md | — |
| `R-CARRIER-ORDER-ORACLE` | ACTIONABLE | Lane TAIL (LOW, gate quality) · Tester: rewrite the call-site gate on the behavioural oracle in the PRE-MATURITY branch | silt-agent-memory/researcher/reviews/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md | — |
| `R-SWARM-NOTBANKED-DEAD` | ACTIONABLE | Lane TAIL (LOW) · Builder: a reachability argument or removal of the not-banked dead branch, one small PR | silt-agent-memory/researcher/reviews/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md | — |
| `R-BLIND-BIND-IS-REQUESTER-CHOSEN` | ACTIONABLE | **FILED 2026-09-12 as an UNASKED finding of the token-domain certification** (`D-TOKEN-DOMAIN-CHAINLESS-CLIENT-2026-09-12`) · Lane D2 · Researcher → **OWNER**. `blindtoken.SignBlinded(rng, priv, blinded)` takes **only the blinded representative**, so the issuer never sees which FDH domain the requester hashed under. **The network bind is therefore REQUESTER-CHOSEN, not issuer-enforced** — a bind nobody checks at issuance is a self-declaration. **Where it is VOID: two concurrently-live networks sharing an issuer key.** A requester on network A can bind to network B's chain id and obtain a signature that verifies on B. **This does NOT change the call-7 answer**, which rests on genesis-coverage of the chain id and not on issuer enforcement; it is filed rather than folded in silently because it prices any future claim that a token is bound to a network. Closer: an owner call on whether the issuing surface must carry the domain (which changes the blind-signing API and its privacy argument) or whether one-network-per-issuer-key is stated as a standing assumption | silt-agent-memory/researcher/reviews/research-outcome/TOKEN-DOMAIN-GENESIS-COVERAGE-AND-THE-CHAINLESS-CLIENT-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-INGEST-WITNESSES-INERT` | ACTIONABLE | Lane D2 · Builder files the measurement; the DISPOSITION is an **owner call** — either `core/statehash.IngestBlockWitnesses` (the R3 witness-bound gate) is wired into v5 block acceptance, which is a consensus-rule change and therefore research-gated, or it is explicitly DECLINED for the RC and disclosed to B8. Measured at `cb491ec`: `cannot inline` at cost 478 against a budget of 80, **zero non-test callers**, and **ABSENT from the linked `./cmd/silt`**. This is the founding scar of the reachability gate (`scar:mechanism-shipped-inert-2026-09-10`) sitting outside the lane set, and it re-prices `R-R3-GOB-ALLOC-AMPLIFICATION`: a decoder bound built inside this function ships inert. No lane record is added and nothing is wired here — wiring a mechanism to make a gate green is the over-claim the gate exists to refuse | silt-agent-memory/principal-engineer/reviews/RULING-reachability-substantiality-test-pr832-cb491ec-2026-09-11.md | — |
| `R-SPARSE-COLUMN-PROVIDER` | ACTIONABLE | Lane TAIL · Builder: cap the per-provider shard probe in `confirmColumnHolders` (`core/node/file.go:1041-1064`) — a live 1-of-T parity-column holder costs up to T round-trips and the corpse gate trips only on proven-dead; does NOT fool the durability audit | silt-agent-memory/principal-engineer/reviews/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-PS-LOCAL-ROT-NOT-HEALED` | ACTIONABLE | Lane TAIL · Builder + Tester (owner call 17 ACCEPTED 2026-09-07): evict a verified-rotten shard the node still hosts and announces so the fetch path replaces it (no scrub exists; the sweep is stat-gated, `core/node/repair.go:85,457,678`); one PR with a Tester gate RED first; research-gated if it reaches bounty or escrow | silt-agent-memory/principal-engineer/reviews/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-LANEOFF-ROTATION-RUNTIME` | ACTIONABLE | Lane TAIL (low, test debt) · Tester: that no demand-key rotation goroutine RUNS after a failed boot install is pinned only by a source-order gate (`cmd/silt TestDaemonArmsTheRotatorOnlyAfterABootInstall`); the runtime observer needs an unwritable issuer directory whose publish-token key exists and an epoch crossing | silt-agent-memory/principal-engineer/reviews/RULING-R0.4b-C3-close-271ab81-final-2026-09-03.md | — |
| `R-VDF-DELAY-INERT` | ACTIONABLE | **FILED 2026-09-12** against the D1 derivation-route audit · Researcher → **OWNER**. Lane F row F11 (`D-BONDVDFDELAY-KEEP-1000-2026-09-12`). **`BondVDFDelay` keeps 1000 and the value has no derivation, because the delay property has no CONSUMER.** The declaration's only rationale is *"modest; a real deployment raises it"*, elaborated in the field doc as *"the modest default keeps the deterministic sim fast"* — a TEST-SPEED argument for a value that is genesis-bound via `ConsensusParams` and consensus-critical. **★ THE ROW'S JOB IS TO NAME WHAT WOULD MAKE 1000 WRONG**, because a residual that says only *"this constant is underived"* is unactionable and the next seat re-derives it from scratch. **Three falsifiers: (1) a CONSUMER of the lower bound is built** — the moment any code refuses an answer for arriving too fast, the delay stops being inert and 1000 must be derived from that consumer's detection target rather than chosen; **(2) the design doc's coupling is implemented** — if the anti-release window is ever made a function of `BondVDFDelay`, 1000 becomes load-bearing on the bond floor and the floor's own derivation (build-immutables #3/#4, compute-sourced and decoupled from any transport deadline) re-opens with it; **(3) measurement shows the honest answer cost is not what the keep assumes** — the three benchmarks the certification gates on are named in it with their exact commands and **NONE has been run**; the existing VDF test modulus is far smaller than the field modulus, so any timing on it is wrong by roughly 3×, and the shipped evaluation runs its squarings and its proving pass serially, so a whole-evaluation figure double-counts. **Today's design claim that raising the delay *"widens W and lowers the required floor"* has NO implementation** — `W` is a bare literal and the derived floor is `2 × (window × plot seal throughput)`; the VDF delay does not enter that arithmetic at all. **Closer: run the three benchmarks, then either derive 1000 from a consumer or record it as knowingly underived at the freeze.** Moving the value costs a re-mint, so a named residual beats a re-mint that improves nothing | silt-agent-memory/researcher/reviews/research-outcome/VDF-GROUP-AND-BONDVDFDELAY-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-VDF-DELAY-VS-A5` | ACTIONABLE | **FILED 2026-09-12** · Builder (a doc correction) → Researcher if the coupling is ever built. Lane F row F11. **A TENSION rather than a falsifier of 1000, and carried as one.** `BondVDFDelay` is a common additive term in every honest answer, and the per-node answer-latency deadline is a **MUTABLE FLAG** while `BondVDFDelay` is **FROZEN PER NETWORK**. Raising the frozen side without raising the mutable one pushes honest nodes past the deadline and **degrades an observability signal ONE-WAY**. The two are linked by no code and the margin a raise would eat is small. **Consequence the declaration gets wrong today:** its own advice — *"a real deployment raises it"* — is advice an operator **cannot safely take after launch**, and the comment does not say so. **Closer: correct the declaration comment to state the post-launch constraint, and either couple the two quantities in code or record that they are deliberately uncoupled.** Not a format item and not RC-blocking | silt-agent-memory/researcher/reviews/research-outcome/VDF-GROUP-AND-BONDVDFDELAY-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-CFGBIND-SEVENTH` | ACTIONABLE | **FILED 2026-09-12** · Builder (a doc correction), owed to whichever PR next touches `D-CFGBIND-TIER-PROMOTION-2026-09-11`. Lane F row F11. **The tier taxonomy does not cover `BondVDFDelay`.** That decision names the build-is-the-operator values and says the rest are flag-supplied; `BondVDFDelay` is **NEITHER** — `cmd/silt` declares `-bond-label-k` and **nothing at all for the VDF delay**. So the release-checklist rule that decision creates (*changing any of these compile-time defaults is a breaking change requiring a new network*) **does not reach the one value with no operator surface**, which is also the one value now genesis-bound with no derivation (`R-VDF-DELAY-INERT`). **Recorded as a CORRECTION, not a new rule.** Closer: the taxonomy gains its seventh case — a genesis-bound compile-time constant with no flag — and the checklist rule is restated to reach it | silt-agent-memory/researcher/reviews/research-outcome/VDF-GROUP-AND-BONDVDFDELAY-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-VDF-COSET` | ACTIONABLE | **FILED 2026-09-12** · Researcher → **OWNER** (it changes honest bytes). Lane F row F10 takes the OTHER half (`D-VDF-MINIMAL-ENCODING-2026-09-12`). **The second spelling defect, and it is a DIFFERENT defect from the minimal-encoding one.** The VDF works in `(Z/N)*` rather than the quotient group, so **`N − y` is a second accepted output**. Unlike the minimal-encoding fix — which is purely narrowing and changes zero honest bytes — **this one DOES change honest bytes**, which is why it was not taken with F10. **The property worth encoding is the one with a closed complement: for a fixed challenge, the set of accepted `VDFY` byte strings has cardinality exactly ONE.** *"A valid output has exactly one wire spelling"* needs BOTH halves, so F10 alone does not deliver it and must not be reported as if it did. **Closer: price the honest-byte change, then a narrowing rule that admits one representative of the coset**, gated RED-first on the pre-fix tree. Its deadline class follows the same four-door test F10 passed — it adds no field and no key — so it sorts to the stamp raise, not the freeze | silt-agent-memory/researcher/reviews/research-outcome/VDF-GROUP-AND-BONDVDFDELAY-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-POR-MERKLE-TAUTOLOGY` | ACTIONABLE | **FILED 2026-09-12** · Builder; **PROMOTED to a gate by the O-8 certification** (G-O8-B / assumption A2). Owner-owned block above (the O-8 re-decision). **LEG 1 OF TODAY'S AUDIT IS A TAUTOLOGY.** `verifyStorageProof` verifies an inclusion proof against a root taken **FROM THE RESPONSE**, and neither call site compares that root to the audited one — so the prover supplies both the value and the standard it is judged by. **This is not hypothetical and it is the sharpest finding in the O-8 document**: the identical laxity sits four lines from where O-8 would land, and **repeating the shape one level down would make O-8 vacuous in one line.** ★ **It is a PRECONDITION of O-8, not a separable cheap win** — A2 is the whole of O-8's security, so shipping O-8 over the current verifier ships nothing. **Closer: one comparison — bind the inclusion-proof root to the root the AUDITOR holds, never the one the response supplies.** The certification names its own falsifier: if the path already rejects, this row closes and G-O8-B is half lifted. Free to close and independent of whether O-8 is ever taken | silt-agent-memory/researcher/reviews/research-outcome/O8-CHUNK-ID-MERKLE-ROOT-MECHANISM-14f794f-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-POR-OUTSOURCE` | ACTIONABLE | **FILED 2026-09-12** · Researcher → Builder. **PROMOTED 2026-09-12 from held-in-tension to a PRECONDITION**, which is what makes it a row rather than an `m0.md` §10 disclosure — the certification's ruling is flat: **O-8 must ship with B-1, or it must not ship.** Today a data-less identity **forwards the identical challenge to a real holder and relays the answer**, because the prover takes the challenge seed verbatim off the message rather than deriving it from its own identity. **That lane is UNCHANGED by O-8, at equal cost — but O-8 changes what closing it is WORTH**: under O-8 the response is a value **identical for every prover, with no prover-bound term at all**, so the relay lane stops being inherited and becomes the only thing between an audit and a pure lookup service. **Closer: B-1 — the prover derives its challenge seed from its OWN identity.** With B-1 the cheapest outsourcing is a full shard fetch per audit, a fetch-to-store ratio of 1.0; without it, a small fraction of that. **Residual falsifier, named and owed: any RPC anywhere that returns a caller-chosen SUB-RANGE of a chunk.** The ordinary serve path serves whole chunks; **not every adapter was read.** The remainder after B-1 is the colluding-holder case, which is `#182` | silt-agent-memory/researcher/reviews/research-outcome/O8-CHUNK-ID-MERKLE-ROOT-MECHANISM-14f794f-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-D3-ROUTE-INERT` | ACTIONABLE | **FILED 2026-09-12**, discharging the register row `D-D3-RELABEL-2026-09-12` explicitly owes · Builder, **POST-RC**. Lane F row F14. **The D3 issuance-mixing route has no production caller, so the property `D-DEMAND` books as *"◑ slices 1+2 BUILT"* is built-but-INERT.** Verified: `client.WithdrawDemandTokenPrivately` (`client/privissue.go`) has exactly **three** call sites, all inside `client/privissue_test.go`; **nothing in the tree imports `github.com/nerolabs/silt/client`**, whose only two files are that function and its test; and `core/node/relayrole.go` names the **DURABLE** identity as the funding that settles, with the D3 path explicitly **not anchor-eligible**. So the shipped binary funds a session through the identity the property exists to unlink. **A record defect, not a regression** — the blind-signed serial and demand-neutrality clauses are unaffected; what over-claims is the *slices 1+2* line. **Closer: wire the production route** (post-RC; it adds no block field, no cbor key, no committed leaf and no validity rule, so it is not a freeze item), **and until then `D-DEMAND`'s D3 clause reads built-but-inert with this row named as its owed route.** ⚠ **This is `D-DEMAND`'s D3, NOT Lane D row D3 (the freeze act).** **The reachability gate cannot hold this claim**: it has no symbol a lane record could name until a production caller exists, and inventing a posture line for one is the over-claim that gate refuses in the other direction | silt-agent-memory/planner/veto-gate-d3-fetcher-privacy-inert.md | — |
| `R-PROBE-FALSE-NEGATIVE-RATE` | ACTIONABLE | **FILED 2026-09-12** by the bounty-for-repair mechanism certification, ratified GATED at that strength (`D-BOUNTY-REPAIR-MECHANISM-GATED-2026-09-12`, ROADMAP row F8). Lane F · **Researcher** (measure) → **OWNER** (ratify the mechanism the measurement unblocks). The loss witness — the input the whole bounty-for-repair intent turns on — is held behind it: a repair **erases its own evidence** (**T-LOSS-IS-A-TRANSIENT**), so only a witness recorded BEFORE the repair can carry the loss, and `probeShard`'s false-negative rate prices whether such a witness can be trusted. The prompt that produces the witness must **never itself be the evidence** (**T-WITNESS-NEEDS-A-PROMPT**) — a witness a claimant can cause to exist is one a claimant can manufacture. **Not authorised to build**; the three items the certification DOES authorise are listed on F8 and do not depend on this row | silt-agent-memory/researcher/reviews/research-outcome/D-BOUNTY-PAYS-FOR-REPAIR-mechanism-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-ADVERSARY-SHAPE-CONTROL-NEEDS-A-BROKEN-DEFENCE` | ACTIONABLE | **FILED 2026-09-12. ★ STRUCTURAL — a property of the gate's DESIGN, not a defect in any declaration** · Lane F (row F1) · Builder → **OWNER**, because it re-prices F1's CI-wiring question. `scripts/check_adversary_shape.py` accepts `fixture=` only when the named fixture carries BOTH `ADVERSARY-HOLDS: <cap>` and `CAPABILITY-CONTROL: <cap>`, and the control leg is defined as *"the same attack WITHOUT `<cap>` is asserted to FAIL"*. **That control is only well defined when the attack SUCCEEDS with the capability — that is, when the defence is BROKEN.** Where the defence HOLDS, the without-capability variant fails for every capability, and a control that cannot discriminate is `scar-gate-passes-on-a-bystander` (count=3, third-time fired). **Measured consequence: `fixture=` is reachable ONLY for broken defences, so row F1's countdown to zero `UNCOVERED` cannot be reached by covering.** At least three of the 19 are permanently stuck for this reason and not for any gap in the tree — `ForeignSeedProof`, `ClaimantChosenSurvivorSet` and `UntrustedClaimFields` each have a fixture that GRANTS the capability while the defence HOLDS. `ForeignSeedProof` is the sharpest: RT-POR-2's `relayOnly` arm grants a proof aggregated under another identity's seed and asserts `Passed == 0`, which makes it the best-witnessed claim in the whole set, and the gate structurally cannot record it as covered. **The builder's refusal to declare those three as covered was DISCIPLINE, not an excuse row — the reviewer says so in terms; declaring them would have been the excuse row.** Closer: an OWNER call on whether the gate gains a third accepting form (a `WITNESSED:` declaration for a granted-and-holding fixture, distinct from both `fixture=` and `UNCOVERED:`), or whether the 19 are ratified as a RECORD and never read as a work queue. **Until that call lands, nobody may read the 19 as a countdown.** **★ PARTLY ANSWERED 2026-09-12** (`D-ADVERSARY-SHAPE-RATCHET-2026-09-12`): the second limb IS taken — **the 19 are ratified as a RECORD and are never a work queue**, and the gate now says so in its own output on every green run. **The first limb is NOT taken and stays open:** the gate gained no `WITNESSED:` form, so the three granted-and-holding claims are still recorded as `UNCOVERED`, which under-reports them. What ratchet mode changes is only the EXIT CODE — the gate reddens on a claim NEW to it rather than on an uncovered one — so this row is re-priced, not closed | silt-agent-memory/principal-engineer/reviews/ruling-pr845-foldin-verification-5c7fb67-2026-09-12.md | — |
| `R-RTPOR2-PIN-BLIND-TO-A-HANDLER-FIX` | ACTIONABLE | **FILED 2026-09-12** · Lane F (row F1) · Builder. `TestRT_POR_2_ChallengeProxyPassesAudit_PINNED_DEFECT` calls `holderA.answerChallenge(challenge)` directly and `Node.answerChallenge` takes only `msg`, so the only remediation that reddens this pin is one that changes the method signature — and it reddens as a **COMPILE BREAK**, not as an assertion. A remediation placed instead in `Node.handle`'s `MsgChallenge` case, which already has `from` in scope, denies outsourcing while leaving the pin **GREEN**. **Severity LOW-MEDIUM, and the polarity is the safe one: a stale pin OVER-REPORTS the defect — it makes the tree look WORSE than it is, never safer.** That is the opposite polarity from the same review's blocking finding (an arm that screamed a false M0 ESCALATE on the FIX path), which is why that one blocked and this does not. Closer: one sentence in `rtPOR2Pin`'s FIX CASE — *"a handler-layer fix leaves this pin green; read `Node.handle`'s `MsgChallenge` case before concluding outsourcing is still open"* — or, at more cost, a fixture that drives the handler rather than the method | silt-agent-memory/principal-engineer/reviews/ruling-pr845-foldin-verification-5c7fb67-2026-09-12.md | — |
| `R-RTRC1-COUNTS-CHUNKS-NOT-SHARDS` | ACTIONABLE | **FILED 2026-09-12** · Lane F (row F7) · Builder. **The RT-RC-1 in-range pin's number is right for a reason its sentence does not state.** `rtRC1InRange` asserts `reachedOne == n−1 == 15` and passes, but `reachedOne` is `len(countStore.distinct)` — **DISTINCT CHUNKS WRITTEN**, which on this fixture is **14 survivor shards plus the object's manifest chunk**, because `fetchSurvivors` does not re-`Put` the one survivor ref the judge already hosted (`heldBefore`). Two off-by-ones cancel. **Severity LOW and the polarity is safe:** the assertion is true about store writes, and the *"the fetch is n−1"* claim is anchored in `judgeRepairClaim`'s complement-of-one-position construction, not in this count. The exposure is a fixture whose placement changes, which would move the number for a reason unrelated to the fetch width. **Closer: assert the shard-only count** (`repairAdv.shardFetches`, added 2026-09-12 and already used by RT-RC-1 arm (c)) against a derivation that accounts for `heldBefore`. **Filed rather than folded in because changing the arm changes a RATIFIED measured number** (`D-REPAIR-CLAIM-GATES-PINNED-2026-09-12`) | silt-agent-memory/researcher/reviews/research-outcome/D-BOUNTY-PAYS-FOR-REPAIR-mechanism-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-DEDUP-NOT-PERSISTED` | ACTIONABLE | **FILED 2026-09-12**, held in tension by the bounty-for-repair mechanism certification and **live from the build** (`D-BOUNTY-REPAIR-BUILT-2026-09-12`) · Lane F (row F8) · Builder, **POST-RC**. `Node.bountyPaid` is node-local and in-memory, exactly as the credit ledger is under `D-FP2-SCOPE`. Both die at restart. **CORRECTION 2026-09-12** (blind review of the build PR): this row used to call that *"at least consistent — a restarted judge denies rather than double-pays"*. **That is FALSE, and false in the direction that gets a residual closed early.** `credit.Ledger.RecordServeToObject` skims into the object's escrow on EVERY serve, so **the escrow refills from ordinary delivery traffic while the paid set does not**. The restart window is shut for exactly as long as the escrow is empty and **reopens with the first skim**, at which point the restarted judge pays a position it already paid for — no re-endowment and no persistence required. **The real reason it is inert today is that `cfg.RepairEconomy` is default OFF** (`-economy=false`), which stops holding the day the flag flips. Persisting the ledger makes it worse, not new: a persisted escrow with an ephemeral paid set re-pays every position after every restart. Closer: persist the set with the ledger, in the same change that persists the ledger, never before | silt-agent-memory/researcher/reviews/research-outcome/D-BOUNTY-PAYS-FOR-REPAIR-mechanism-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-SLASH-RUNS-BEFORE-THE-ECONOMY-GATE` | ACTIONABLE | **FILED 2026-09-12** by the blind review of the judge's three fixes · Lane F (row F8) · **Builder → PE** for the placement call, then a one-line move. **`settleRepairVerdict` runs `n.ledger.SlashFalseRepair(claimant)` in its FIRST branch, BEFORE the `if !n.cfg.RepairEconomy` return** — so on the SHIPPED default (economy OFF) the OFF path is a no-op for the bounty and **not** for the slash: an inbound repair claim still moves standing. **This PR did not change that effect; it lowered the ENTRY COST.** The position screen adds a SECOND, zero-network route into the same slash — previously a lying `ShardID` was slashable only after an n−1 survivor fetch, now it slashes at zero fetches. **Not a third-party weapon:** the target is `from`, the TLS-authenticated sender, so it is self-slash only. Closer: a PE ruling on whether a STANDING motion belongs inside the economy PARTICIPATION switch at all, then either the one-line move or a comment stating why it is deliberately outside — plus a gate that reddens if the answer ever silently changes | silt-agent-memory/principal-engineer/reviews/RULING-pr847-repair-judge-three-fixes-6d1991b-2026-09-12.md | — |
| `R-SHARDID-ALIASES-POSITION` | ACTIONABLE | **FILED 2026-09-12** by the bounty-for-repair mechanism certification · Lane F (row F8) · Builder. **Two stripe positions carrying byte-identical shards collapse to ONE content id**, and `heldBefore`, `missing` and `reachable` all key by id today. **CONFIRMED IN THE SHIPPED TEST FIXTURE, not hypothetical:** `newRepairAdv`'s final stripe has positions **12 and 14 sharing id `fbd7593f…`** (a short stripe's parity over mostly-zero data). The 2026-09-12 dedup deliberately keys on `(root, stripe, pos)` and **never on `claim.ShardID`** for exactly this reason — an id key would let one payment permanently block a different, legitimate position. The three id-keyed maps above are **not** fixed by that choice and are the residual | silt-agent-memory/researcher/reviews/research-outcome/D-BOUNTY-PAYS-FOR-REPAIR-mechanism-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-CERT-FIXTURE-STRIPE-COUNT` | ACTIONABLE | **FILED 2026-09-12** · Lane F (row F8) · Builder (a doc correction, already applied in-tree). **The bounty-for-repair mechanism certification §3.3 and `newRepairAdv`'s own comment both assert the fixture stages *"exactly one full k=10 stripe, realData = 10"*. MEASURED FALSE:** `splitFile` reserves `chunk.HeaderSize` bytes per frame, so 10·(512<<10) bytes yield **11** chunks over **2** stripes — stripe 0 full (16 refs, `realData = 10`) and stripe 1 **short** (7 refs, `realData = 1`). **The conclusion the premise supported survives** (every pre-existing repair-claim test claims stripe 0, so none measured a short stripe), which is why this is a correction and not a refutation of the certification. The in-tree comment is corrected; **the certification document itself is the Researcher's artifact and is NOT edited by the Builder** | silt-agent-memory/researcher/reviews/research-outcome/D-BOUNTY-PAYS-FOR-REPAIR-mechanism-RESEARCH-CERTIFICATION-2026-09-12.md | — |
| `R-RTRC3-PREMISE-CHECKS-A-RECORD` | ACTIONABLE | **FILED 2026-09-12** · Lane F (row F1) · Builder. RT-RC-3's premise guard sets `reachable` from `probe.resolveProviders(colKey(...))` — a provider **RECORD** — while its failure message says *"answers"*. A record is not retrievability. **Severity LOW: the premise is true in fact** (nothing in the fixture deletes the shard), so the guard is weak evidence for a correct claim rather than a wrong claim. The exposure is a future fixture change that loses the shard while the provider record survives; the pin would then label correct behaviour a defect. Closer: a real `fetchFrom` on the probe node in place of the `resolveProviders` check — a real fixture change with its own cost, which is why it is filed rather than folded in. **Do NOT bundle this with the RT-RC-3 versus `TestRedteamRepair_HonestClaimIsPaid` label contradiction: that is a different question and it is the owner call at row F8** | silt-agent-memory/principal-engineer/reviews/ruling-pr845-foldin-verification-5c7fb67-2026-09-12.md | — |
| `R-BOUNTY-ZERO-BELOW-262KB` | ACTIONABLE | **FILED 2026-09-12** · Boulder 2 (the economy) · Economist, with the owner on the product question. The F1 re-pricing (`D-BOUNTY-PRICE-F1-2026-09-12`) widens the class of objects whose BASE repair bounty is zero from **≤ 26,190 B to ≤ 262,119 B** — 10.008×, re-derived at source and run in `core/credit` `TestF1ZeroClassIsTheAcceptedCost`. A single-frame object is stored at its true length (the short-final-stripe framing), so for a small object the shard IS the object and NO chunk size changes it; its durability is prepay-only. Three things bound the cost: the publish warning's boundary 262,119/262,120 is INVARIANT (only the arm changes, TRUNCATES → ZERO), `shippedBountyBase()` is DERIVED so the shipped default stays silent, and the rarest-shard multiplier still funds a sub-default object's repair near the cliff (a 100,000 B object pays 0 at `mult = 1` and 2 at `mult = 6`). **This is a PRODUCT question, not a soundness one** — it raises no cost floor, so build-immutable #4 does not bite, but it is adjacent to S6 and to silt's edge-participation story. Closer: an owner call on whether a durability economy that funds base repairs only above ~262 KB is the shape silt wants; the admissible lever if it is not is the PRICE (`U/p`), never `pipeline.DefaultChunkSize`, which moves `blocks[0].Hash()` | `silt-agent-memory/researcher/reviews/research-outcome/R-HOLDER-PARTICIPATION-CONSTRAINT-structural-floor-RESEARCH-CERTIFICATION-2026-09-12.md` | — |
| `R-F1-FLOOR-FAILS-ON-SELF-HOLD` | ACTIONABLE | **FILED 2026-09-12 by the blind PE on PR #848 (F-1); the certification that F1 cites had already found it** · Boulder 2 (the economy) · **Researcher -> owner, under the D-S7 research gate (escrow/skim/bounty) — NOT the builder's to settle, and deliberately not settled in the code comment.** F1's floor is *"the protocol's witnessed price of the bytes the PAYEE moves"*, and the payee is normally the remote new holder of the rebuilt shard, whose act is ONE shard inbound. It is not always remote: `core/node` `(*Node).selfHoldEligible` (`RepairEconomy` on, `domainOf(n.id) != 0`, `usedDomains[sd] == 0`) lets the paramedic KEEP the shard it rebuilt and name ITSELF the payee, and that path is tried BEFORE remote placement (the `(a-domain-fresh)` block created by the 2026-08-19 PE ruling precisely to fund reconstruction). **On it the payee moved `k` survivor shards inbound and F1 pays it for ONE — F1's own floor fails by a factor of `k`, 10x at the shipped geometry, the mis-price F1 exists to remove with the sign reversed.** `-domain` is a shipped flag, set in `integration/sybil/docker-compose.yml` and `integration/awstest/topology.py`; the self-hold rate is a deployment property (~`((D-1)/D)^(16-m)`) with no instrument, so it is not rare by construction. **Inert on `main` today only because `-economy` defaults OFF — a FLAG DEFAULT, which is a weaker reason than "by design" and is recorded as such.** Coupled to the metered-but-not-attributed residual held in tension at `docs/design/owned-residuals.md` D7 (it carries no register row by design, being an owned residual rather than a plan item): the meters cannot tell the two payees apart, so the self-hold rate cannot be measured before it is priced. Closer: a certification answering *does F1's floor hold on the self-hold path, and if not, is a two-rate price, a self-hold exclusion, or an accepted under-pay the answer* | `silt-agent-memory/principal-engineer/reviews/ruling-pr848-f1-bounty-repricing-7dc7462.md` | — |
| `R-PREF1-PRICE-COMMENTS-IN-CORE-NODE` | ACTIONABLE | **FILED 2026-09-12 (blind PE F-5 on PR #848)** · Boulder 2 (the economy) · Builder. Six production comments in `core/node` still state the pre-F1 repair price `c·k·shardBytes` — four in `repairclaim.go` (including the one that describes the division the judge actually EXECUTES, which is the worst of them) and two in `node.go` (`Config.RepairEconomy` and the `BountyBaseZero` stat doc). They are not corrected in PR #848 because both files were open in PR #847 at the time and a deferred sweep must not fight it; the seventh site, `cmd/silt/ui.go`, IS corrected there. **#847 MERGED at 2026-09-12 19:31Z (`317a4c3`) and the six sites survive it unchanged — re-counted at source after the rebase — so the sweep is UNBLOCKED.** Closer: one comment-only sweep of `core/node/{repairclaim.go,node.go}`, no behaviour change, on the next branch | `silt-agent-memory/principal-engineer/reviews/ruling-pr848-f1-bounty-repricing-7dc7462.md` | — |
| `R-MULT-RACES-THE-PLACEMENT` | ACTIONABLE | **Row added 2026-09-12 (blind PE F-6 on PR #848): the residual was cited in `core/credit/escrow.go` and in `docs/decisions.md` with no register row, so the filing rule's own lint could not see it** (`check_residual_register.py` scans `ROADMAP.md` only) · Boulder 2 (the economy) · Economist, with the Researcher on the published band. `reachable` is the judge's POST-repair count (`fetchSurvivors` counts positions fetched AFTER placement), so the rarest-shard multiplier `m̄` is decided by a race between placement convergence and judge scheduling. The consequence is that the D-S7 self-funding figure is a BRACKET, never a point: `[12.0, 36.0]` pre-F1 and `[1.20, 3.60]` under F1, and the published `S/R >= 36` was the top of that bracket presented as a point (withdrawn in `D-BOUNTY-PRICE-F1-2026-09-12` §8). Closer: an instrument for the realised multiplier distribution, or an owner call to publish the band rather than a point | `silt-agent-memory/principal-engineer/reviews/ruling-pr848-f1-bounty-repricing-7dc7462.md` | — |
| `R-TRUNCATION-DISCLOSURE-NARROWS` | ACTIONABLE | **FILED 2026-09-12 by the builder, NOT named in either certification** · Boulder 2 (the economy) · Builder. F1 drops the publish warning's threshold from 10 to 1, and the rule fires iff `base < shippedBountyBase()`, so **a firing publish now always has `base == 0` and the TRUNCATES arm is unreachable through `cmd/silt` `bountyPriceWarning`**. The cost is real and is RUN in `TestGLambda8PublishWarningFiresOnlyWhenThePublishShortPaysTheRepairer`: a publish worth an exact 1.99996 credits (`-chunk-size 524264`) pays 1 and is **SILENT**, a 50 % short-pay the publisher is never told about. **RE-FILED 2026-09-12 at the MEASURED cost, and the first filing's reason is WITHDRAWN as refuted (blind PE F-4).** The cost is not the 1.99996 anecdote: the maximum SILENT repair-wage short-pay rises **5.5×, from 9.09 % to 50.0 %** (pre-F1 `base >= 10` bounds the loss at 1/11; post-F1 `base >= 1` bounds it at 1/2), and it is reachable at ordinary operator choices — `-chunk-size 393216` (384 KiB) goes **0.00 % -> 33.3 %**, 327,680 goes 4.00 % -> 20.0 %, 524,264 goes 5.00 % -> 50.0 % (measured through `credit.RepairBountyTruncation`). Pre-F1 the warning covered exactly the large-loss region; post-F1 that whole region is silent. **The withdrawn reason:** the first filing said widening "would speak on the shipped default itself, the finding blind PE M6 closed". The shipped function refutes it — `RepairBountyTruncation` returns **0 tenths of a percent at BOTH 262,144 B and 262,128 B** against 200/333/500 at the rows above, so an OR of `lossTenths >= 10` (1 %) is silent on the default and on the minimum chunk. **The real reason the rule is not widened here** is the one structural property it has: "warn iff `base < shippedBountyBase()`" has a CLOSED COMPLEMENT — silent in exactly one stated case — and an OR-clause breaks that. Widening is therefore a DECISION, not a defect fix. The arm's arithmetic is kept and driven directly in `core/credit` `TestRepairBountyTruncationIsExactIntegerArithmetic`, and the threshold is DERIVED, so a re-tune of `U/p` or the publish default revives the arm. Closer: an owner call between the loss-fraction OR-clause (measured to be silent on the default) and accepting the 50 % silent band | `silt-agent-memory/researcher/reviews/research-outcome/R-HOLDER-PARTICIPATION-CONSTRAINT-structural-floor-RESEARCH-CERTIFICATION-2026-09-12.md` | — |

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
  research-gate + owner ratification. **The format SLOT that was to be reserved inert at the stamp
  raise is DROPPED** (owner call D, 2026-09-10, `docs/decisions.md` `D-ITEM4-DROPPED-2026-09-10`):
  reserving an inert field bought only "not paying an era later", and the freeze deadline is SOFT.
  This close is unaffected and stays post-RC; if it lands and needs committed bytes, (d-3)'s option
  beta folds a `PoPDigest` in and makes them PRUNABLE. Source: crypto-specialist advisory C-4,
  `/Users/andrewedmond/.claude/silt-agent-memory/crypto-specialist/reviews/ADVISORY-R0.4b-C3-blind-RSA-epoch-binding-2026-09-03.md`.
- **FDH domain-separation-tag length prefix + 128-bit reduction slack — R0.4b-FDH.** The **four**
  FDH domains are not length-prefixed and `fullDomainHashD` expands to only `nLen + 8` bytes. Sound as
  built (all four domain constants differ at byte index 10) but sound *by accident of the constants*.
  The count read "three" from the advisory's own 2026-09-03 wording, which predates `relayAnchorDomain`.
  The publish and credit domains are BYTE-FROZEN against chain replay, so the change was DECLINED at
  the freeze in favour of a prefix-freeness gate (freeze manifest, 2026-09-07), and that gate is BUILT
  (`TestFDHDomainSetIsPairwisePrefixFree`). The versioned length-prefix change itself stays OPEN here.
  Source: advisory C-8.
- **Blinding-factor sampling: mod-reduction, not rejection sampling — R0.4b-BLIND-SAMPLING.**
  `blindtoken.randInt` reduces `(bitlen(N) + 64)` random bits mod `N`; RFC 9474 §4.2's MUST is met
  only statistically (within `2^-64` of uniform). A conformance gap, not a break; changes no committed
  byte. Work: rejection-sample into `[1, N)` and **bound the retry** — the two loops calling `randInt`
  spin forever on a reader that yields zeros. Declared at `core/blindtoken/blindtoken.go` and
  `docs/thinking/2026-09-02-r0.4b-c3-close-design.md` §12. Source: crypto advisory R4,
  `/Users/andrewedmond/.claude/silt-agent-memory/crypto-specialist/reviews/ADVISORY-R0.4b-C3-crypto-items-as-built-01bf8e9-2026-09-03.md`.
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
  `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-b8-558-chainstore-refuse-to-start-2026-09-07.md`.
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
