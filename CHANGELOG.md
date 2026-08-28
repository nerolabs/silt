# Changelog

All notable changes to Silt are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and the project aims to
follow [Semantic Versioning](https://semver.org/).

This log is published at [silthq.com/changelog](https://silthq.com/changelog.html).

## [Unreleased]

### Changed
- **CONSENSUS-RULE: reject a block carrying two bond registrations from distinct
  identities on the same root** (2026-08-28; certified + human-ratified). This is a
  validity-layer consensus-rule change. `validateBondRegs` (chain.go) deduped only
  per-ValidatorID (`seenReg`, gate-gated) and never per-root, so a block with two
  PROVEN registrations from DISTINCT identities on the SAME root was ADMITTED;
  `apply()` (chain.go:2780-2790) then resolved the winner by intra-block SLICE ORDER.
  Two honest replicas applying the identical block in a different `BondReg` order
  committed a DIFFERENT `bonded`/`bondRootOwner` state — an order-dependent commit the
  era-3 SMT state root cannot tolerate. The fix adds a `seenRoot` dedup, a sibling of
  `seenReg`, that rejects such a block with `ErrSharedRootInBlock`. It runs
  **UNCONDITIONALLY** (NOT behind the #506 `regGateActive` gate — the freeze seam must
  close in every regime), and dedups on **(root × distinct-ID)** only: a validator
  re-registering its OWN root (same ID: renew/resize) is legal (F1) and stays admitted.
  A pure validity tightening — removes an admissible block class, never admits a new
  commit; I1–I5 and history-independence preserved; `apply()`'s tie-break, the weight
  sum, the epoch freeze, and the quorum threshold are untouched. Covered by a
  RED-then-GREEN model-check probe (`redteam_verify_sameroot-intrablock_test.go`):
  pre-fix the block is admitted and the committed state diverges across intra-block
  orderings; post-fix it is rejected in both orderings; a negative control confirms
  same-ID renew/resize is still admitted. Closes residual R2 of the certification.
  Research certification: `same-root-intrablock-bondreg-contention-RESEARCH-CERTIFICATION-2026-08-28`
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/same-root-intrablock-bondreg-contention-RESEARCH-CERTIFICATION-2026-08-28.md`);
  PE ruling `RULING-618-bond-registration-order-independence-2026-08-28`. The prior
  #618 `TestSharedRootDeniedViaValidatedBlock` was updated: the validated path now
  REJECTS the shared-root block rather than admitting it and deduping in `apply()` —
  strictly stronger, and order-free by construction.

### Added
- **Named the genesis same-root premise (residual R-G) — no genesis validity change**
  (2026-08-28). PR #618's `seenRoot` per-root distinct-ID dedup lives in
  `validateBondRegs`, which `AppendGenesis` (chain.go) does NOT run — it goes straight
  to `apply()`. Genesis `apply()` IS order-dependent for two distinct-ID **unproven**
  same-root regs (confirmed by execution: slice `[A,B]`→owner=A, `[B,A]`→owner=B). It is
  safe TODAY only by an EXTERNAL invariant, not by the guard: the production genesis is a
  byte-identical shared constant carrying **NO BondRegs** (`genesis.Build`,
  core/genesis/genesis.go), so there is no per-node slice order to diverge on. The era-3
  SMT freeze's unconditional order-independence claim silently leans on this premise. This
  change NAMES it as a pinned, executable fact so it cannot break silently: a named anchor
  at `AppendGenesis` plus two guard tests — `TestGenesisSameRootApplyIsOrderDependent`
  (core/chain, pins the un-guarded order-dependence; flips RED if genesis is made
  order-independent) and `TestProductionGenesisCarriesNoBondRegs` (core/genesis, pins
  byte-identity + zero BondRegs; flips RED if the production genesis ever carries BondRegs
  or goes per-node). Both ablations verified. **No change to genesis validity rules** —
  this is the record-it half of the PE's "close it OR record it," inside the already-
  certified envelope. Making genesis order-independent by rejection would be a
  consensus-rule change to genesis validity (research-gated); see
  `docs/thinking/2026-08-28-genesis-sameroot-residual.md` option (b). PE ruling
  `RULING-618-updated-sameroot-dedup-fix-2026-08-28`
  (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-618-updated-sameroot-dedup-fix-2026-08-28.md`),
  residual R-G.
- **Order-independence coverage for the mature-epoch family — the `orderVacuous` debt,
  next increment** (2026-08-28). The order-independence model-check oracle
  (`modelcheck_order_independence_test.go`) declared the mature-epoch family
  (`everMature`, `matureEpoch`, `epochSet`) as `orderVacuous`: the launch-anchor
  `twoOrderings` world never matures (`MatureValidators=99`), so those fields were
  compared over ∅ and their order-independence was unproven. A new `matureOrderings`
  fixture brings an ANCHORLESS objective world (epochs on, `MatureValidators=2`) to
  maturity over two OPPOSITE-order histories: a bonded non-quorum victim is slashed at
  height 1 in one ordering and height 3 in the other, so the `(bonded, slashed)` maps
  are built by two genuinely different histories (per the #618 lesson that a commutative
  fixture is a decoration). Both freeze the SAME four-member `epochSet` at the height-4
  boundary (`liveQualifiedSet` excludes the slashed victim), and both latch `everMature`
  / set `matureEpoch`. All three fields are non-empty and byte-identical across the two
  orderings — the maturity latch, the #357 Cond-B handoff, and the epoch freeze are
  order-INDEPENDENT (the consensus-correctness trip-wire did not trip; no rule touched).
  `TestCommittedSetFieldsAreOrderIndependent` now pairs each committed field with its
  populating world (the union-of-worlds pattern the snapshot oracle already uses); a
  dedicated `TestMatureEpochFamilyIsOrderIndependent` asserts the latch/handoff/freeze
  ACTUALLY FIRED (coverage is not vacuous) and the two orderings reached identical state.
  Three ablations verified RED-then-GREEN (drop a governor from one frozen `epochSet`;
  flip `matureEpoch`/`everMature` in one chain — each names the field). Three fields
  removed from `orderVacuous`. Two-list union: `epochSet` already had a leave-one-out
  snapshot probe (#604), so it now clears BOTH oracle lists (freeze-ready);
  `everMature`/`matureEpoch` clear the order-independence list only and remain
  `probeUncovered`-owed. The #506-gate family (`gateLockedIn`, `gateHeight`) is left for
  the next increment. Deliberation: `docs/thinking/2026-08-28-orderVacuous-mature-epoch.md`.
  Test/fixture only; no consensus rule changed.
- **Order-independence coverage for the bond-registration family — the #617 debt,
  first increment** (2026-08-28). PR #617 declared six committed bond-registration
  fields (`bonded`, `bondRootOwner`, `bondRootProven`, `bondRegHeight`, `regVersion`,
  `bondDomain`) as `orderVacuous`: the order-independence model-check oracle compared
  them over ∅ in every ordering, so their order-independence was unproven. The
  `twoOrderings` fixture now commits a height-5 bond block whose BondReg slice order
  flips between the two orderings — including a **G3 proof-beats-declaration
  displacement** of a genesis squatter (chain.go:2780-2794), the one bond rule whose
  intra-block order could genuinely matter. All six fields are non-empty in both
  orderings and byte-identical across them for this DISJOINT-ROOT construction:
  G3/bond-root ownership is order-independent for admissible blocks (the
  consensus-correctness trip-wire did not trip). A dedicated
  `TestBondRegG3DisplacementIsOrderIndependent` asserts the displacement actually
  FIRED (coverage is not vacuous) and both orderings reached identical bond-root state.
  **Scope correction (certified 2026-08-28, see the CONSENSUS-RULE entry under
  Changed):** this disjoint-root fixture does NOT cover two distinct-ID proven claims
  on the SAME root in one block — that case IS order-dependent in `apply()` and is
  now rejected at the validity layer. The order-independence claim holds for every
  ADMISSIBLE block precisely because that collision can no longer be admitted.
  Six fields removed from `orderVacuous`. For the two-list union rule, a covering
  leave-one-out probe was added for `bondRootProven` (a proven owner must not be
  displaced by a later proven claim; a snapshot that lost the field wrongly allows it)
  and it was removed from `probeUncovered`. **Fields now clearing BOTH oracle lists:**
  `bonded`, `bondRootOwner`, `bondRootProven`. Fields clearing the order-independence
  list only (their snapshot-equivalence coverage is #506-gated or metric-only, tracked
  in `probeUncovered`): `bondRegHeight`, `regVersion`, `bondDomain`. The mature-epoch
  and #506-gate `orderVacuous` families are left for later increments. Test/fixture
  only; no consensus rule changed.
- **Fixed a state-aliasing hazard in the snapshot-equivalence oracle's `snapshotBoot`**
  (2026-08-28). `snapshotBoot` carried committed maps into a replica by REFERENCE, so a
  mutating leave-one-out probe (one that calls `apply()`) wrote through the shared map
  header into `src` and every sibling replica. The leave-one-out loop ablates one field
  at a time off the same `src`, so a mutating probe on the k-th ablation silently
  poisoned the (k+1)-th — this masked `bondRootProven`'s verdict flip entirely (the
  `bondRootOwner` F1 probe displaced `src`'s shared `bonded`/owner maps before
  `bondRootProven` was ever ablated, so its ablation saw already-corrupted state and
  changed no verdict). `snapshotBoot` now deep-copies carried maps/slices so each
  replica owns its state. Test-only.
- **The O(depth) CI gate — a standing pass/fail check for the memory-accumulation
  subset of the depth-war class** (2026-08-27). The lineage #528/#535/#549/#555/#556/
  #558/#560/#561/#562/#563/#572 is one class: per-height cost that grows with chain
  depth. A unit test at a fixed height is always green for such a bug (canonical #555:
  `AllEntries` built an O(n) slice per block — green in every constant-height test,
  catastrophic at real depth). `sim/TestPerHeightCostLinear` turns the standing
  memory-growth diagnostic into an assertion: it drives the mature-epoch consensus
  network up a height ladder and fails if baseline-subtracted `HeapObjects` grows
  super-linearly. **Scope: this gate is MEMORY-only** (baseline-subtracted
  `HeapObjects`). It catches the allocation-shaped depth blow-ups (#555 `AllEntries`),
  the OOM-producing subset that crash-looped the field cohort. It does NOT catch a
  CPU-time O(depth) scan that allocates little (e.g. #528's per-height CPU burn); that
  dimension needs its own noise study and is tracked as a follow-on. **The gate that
  protects `main` on each PR is the SHORT ladder** — every PR and push runs
  `go test -short` (`ci.yml:44`) and `go test -race -short` (`ci.yml:65`), which drive
  the `250→500→1000` ladder, still spanning two doublings. The wide `500→1000→2000`
  ladder runs on `release.yml` only. The bound is a two-stage doubling test —
  `growth(2H)/growth(H) < 2.6` (measured linear baseline 1.998; a super-linear O(n²)
  regression ≈ 4.0; 2.6 sits 30% above baseline and 35% below the defect signal).
  HeapObjects is deterministic across runs (seeded sim + forced GC), so the gate does
  not flake on legitimate linear growth or GC noise; HeapInuse is logged but not
  asserted on (its arena-granular steps are too noisy). Proven failing-first: a
  synthetic O(n)-per-block accumulator (`SILT_ODEPTH_INJECT=1`) drives both doublings
  red (ratios 3.03/3.36). Runs in the default `go test` job, ~6s to h=2000, no new CI
  job. Shares the drive-and-measure helper with the OOM diagnostic so the measurement
  has one source of truth. Derivation and false-positive analysis:
  `docs/thinking/2026-08-27-o-depth-ci-gate.md`.
- **Order-independence + leave-one-out coverage for the `spent` and `slashed`
  SMT leaves, and a permanent vacuous-∅ guard** (2026-08-28). The keystone
  order-independence oracle reported "16/16 committedSet fields identical" while
  its fixture left `spent` and `slashed` empty, so two of the sixteen comparisons
  were `DeepEqual(∅, ∅)` — vacuous (PE ruling
  `silt-reviews/.../RULING-keystone-spent-slashed-classification-2026-08-28.md`).
  `twoOrderings` now commits two blind-signed publish-token spends and two
  committed equivocation slashes across two opposite orderings, so both fields are
  NON-EMPTY and byte-identical across order. A new fixture-side guard fails the
  order-independence test if any committedSet field it compares is empty in both
  orderings and not declared in `orderVacuous` (a shrinking debt with reasons), so
  "all N identical" can never again read as coverage over an empty map. The
  snapshot-equivalence / leave-one-out oracle gains `spent` and `slashed` probes
  (each flips a real verdict on omission), and both leave `probeUncovered`. A new
  `TestBondedOrderFreeUnderSlashInteraction` traces the PE's flagged residual —
  apply()'s `delete(c.bonded, culprit)` paired with `slashed[culprit]=true` — and
  proves `bonded` byte-identical across two opposite slash orderings. Test-only;
  no consensus rule moved. Each new probe was ablated (injected order-dependence →
  RED, reverted → green).
- **λ_H arrival-rate instrumentation — the one measurement the CT-1 conditional
  theorem is owed** (2026-08-27). The C-1 lift to CERTIFIED-CONDITIONAL
  (`silt-reviews/.../C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27.md`)
  proves maturity precedes capture under an honest-arrival floor `λ_H > 0`
  (measured), an adversary budget `W_A` (declared), and P2 (`M_req > W_A/(2·w_min)`).
  §6 names exactly one input silt did not hold in code: the **live honest-arrival
  rate at launch**, plus a **floor-exit alarm**. This ships both. λ_H is defined
  as the **operator/domain-distinct bonded-arrival rate per block-height** — the
  arrival COUNT `A(t)` is the SAME shed metric `min(NakamotoOperators,
  NakamotoDomains)`, exposed as the pure getter `chain.MatureCoefficient()` so the
  floor and the shed cannot count different quantities (the theorem binds
  `T_mature ≤ M_req/λ_H`). The daemon's commit observer trails `A(t)` over a
  configurable window (`-lambda-h-window`, default 20 heights) and narrates
  `λ_H = ΔA/Δheight` to the log beside the C2 concentration line. `-lambda-h-floor`
  (distinct arrivals/height; **default 0 = disabled**, so sims and existing
  deployments are unaffected) sets the certified floor: when the measured rate
  falls below it **while the network is still young** (pre-maturity latch — after
  the one-way `EverMature` latch the floor is moot, P4), a LOUD `λ_H FLOOR-EXIT`
  marker surfaces that the launch has left CT-1's hypothesis H and
  maturity-precedes-capture is no longer proven. **OBSERVABILITY ONLY**: it reads
  the committed C2 metric and narrates — it changes no validity predicate, no
  consensus rule (I1–I5), no security parameter. It parameterizes the
  certification, not the code (cert §6). Design:
  `docs/thinking/2026-08-27-lambda-h-arrival-rate-instrumentation.md`.
- **Keystone weight-discriminator probe — the committed per-member WEIGHT bytes
  of `epochSet` proven load-bearing** (2026-08-27, closes issue #603, the era-3
  format-freeze gate). The membership probes prove `epochSet` MEMBERSHIP is
  load-bearing but would still pass if the field stored membership with all weights
  set to a constant, because omission empties frozen membership and rejects via the
  COUNT floor (`ErrNoQuorum`) — the ⅔-weight predicate never fires. `TestEpochWeight
  BytesAreLoadBearing` closes that gap. It builds a mature-epoch world with UNEQUAL
  frozen weights and a block whose support coalition (proposer + one attester,
  concentrated real weight) clears the count floor (`Quorum: 1`, `seen=1`) but whose
  verdict is carried by `requireEpochWeightQuorum`. Full case (true weights): the
  coalition holds 10 of 12 MiB → `3·10 > 2·12` → ACCEPT. Ablated case: membership
  held fixed, weights FLATTENED to a constant → support/total collapses to
  `2/4 = ½ < ⅔` for any constant → REJECT with **`ErrNoQuorumWeight`**, the weight
  predicate as the discriminator — not `ErrNoQuorum`. The rules are used as written
  (`chain.go:2443-2464`); no summation, freeze timing, or boundary was moved.
  Ablation-proven (the session scar): injecting no-blinding makes the ablated case
  ACCEPT (RED), and injecting the membership ablation (empty `epochSet`) flips via
  `ErrNoQuorum: 0 qualified` with `seen=0` (RED) — so the probe rejects a
  membership-flip masquerading as a weight-flip. This is the load-bearing weight
  claim the era-3 committed-root format may now freeze on. Certified by C-7
  (`../silt-reviews/research/research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`,
  the witness path needs per-field load-bearing state) and by the blind PE ruling's
  fix 2 (`../silt-reviews/principle-engineer/RULING-keystone-probes-bonded-epochset-2026-08-27.md`).
  Deliberation: `docs/thinking/2026-08-27-keystone-weight-discriminator-probe.md`.
- **Keystone leave-one-out — `bonded` and `epochSet` MEMBERSHIP proven
  load-bearing** (2026-08-27, PE-gated on the era-3 format freeze). The
  snapshot-boot-equivalence oracle's sharp half now probes two more committed
  fields out of `probeUncovered` and into `probes()` (coverage 3/16 → 5/16):
  omitting `bonded` from the snapshot rejects a commit its bonded quorum should
  accept, and omitting `epochSet` rejects a mature-epoch commit its frozen
  membership should accept. Both flips run the real qualification+quorum predicate
  (`collectQuorumSigs` → `requireQuorumStack`), using the rules as written — no
  rule was tuned. The flip in each case is carried by **membership**
  (qualification), NOT the ⅔-weight predicate: omitting `bonded` disqualifies the
  attesters in the objective regime, and omitting `epochSet` empties the frozen set
  so its members fail membership. In both cases the verified RED is `ErrNoQuorum`
  (the count floor); `requireEpochWeightQuorum` never fires (with `epochSet` empty
  its `total <= 0` branch short-circuits). Because `bonded` gates a verdict only
  where qualification reads the live bonded map (a non-epoch objective regime) and
  `epochSet` only governs a mature epoch, the two are load-bearing in mutually
  exclusive regimes, so the leave-one-out harness now ablates each field on the
  world where it flips. Ablation-proven: the leave-one-out goes RED ("changed NO
  verdict") when the probe is made field-blind, and each rejection is the
  frozen-set/bonded qualification error, not an unrelated panic.
  **Owed (issue #603, era-3 format-freeze gate):** these probes prove MEMBERSHIP is
  load-bearing, not the committed per-member WEIGHT bytes — a leave-one-out that
  flips via `requireEpochWeightQuorum` specifically (a coalition clearing the count
  floor but below ⅔ of frozen weight → `ErrNoQuorumWeight`) is still owed before
  the era-3 format freezes. Do not freeze era-3 on the weight claim until #603
  lands. Blind-PE-reviewed
  (`../silt-reviews/principle-engineer/RULING-keystone-probes-bonded-epochset-2026-08-27.md`),
  Tester-confirmed injected RED. Deliberation:
  `docs/thinking/2026-08-27-keystone-probes-bonded-epochset.md`.
- **The disk-backed node-store spike — a batching bbolt `MapStore`, proven
  correct locally before any billable run** (2026-08-27, PE-ordered). PR #596
  disqualified the in-memory SMT backend by kernel OOM, so the keystone needs a
  disk-backed store, and the certification's owed boot-rebuild measurement cannot
  re-run until one exists. `internal/smtspike/` now carries `boltStore` — a
  batching `kvstore.MapStore` over bbolt, test-only, importable by nothing. The
  load-bearing design point is **write batching**: the SMT calls `Set()` once per
  dirty node during `Commit()`, so a naive one-transaction-per-`Set` adapter would
  fsync per node and make the measurement meaningless; instead `Set()` buffers and
  `Flush()` commits a whole block in one transaction. **Correctness is proven
  before cost** (the local-proof-before-billable rule): the disk-backed trie
  produces byte-identical roots to the in-memory reference across every block,
  survives close+reopen while still serving membership proofs, and handles the
  delete/tombstone path. The measurement harness (`SILT_STORE_PROFILE=1`) reports
  **RSS, not just Go heap** — bbolt is mmap'd, so its residency lives in the page
  cache outside the Go heap, which is exactly the OOM-relevant number the earlier
  heap-only draft would have missed. New evidence surfaced for the backend
  decision: the LSM candidate (pebble) pulls in **127 modules** vs bbolt's ~1, so
  the recommended sequence is bbolt-alone on the floor box first, adding the LSM
  only if bbolt's write cost proves binding — evidence-driven rather than
  prior-driven. The floor-box run itself is billable and awaits explicit
  authorization. The floor-box run
  is now DONE (both backends, 1M keys, fair SSD box, no OOM): the unevictable Go
  heap ties at ~305 MB, so both fit the box; pebble is smaller on disk (234 vs
  418 MB) and lower RSS, but that RSS gap is mostly kernel-evictable page cache,
  not the must-hold floor. Builder recommendation is **bbolt** — pebble's
  127-module supply-chain surface outweighs its evictable-memory edge once heap
  and cache are separated — and **Q6 resolves to persist** (reopen is 7 ms vs an
  18-min rebuild). The one owed follow-up is a memory-pressure coexistence test
  against a ~1 GB daemon. Reasoning:
  `docs/thinking/2026-08-27-disk-backed-mapstore-options.md`.
- **The hexagonal guard now walks TRANSITIVE imports — and it found the gap was
  already live** (2026-08-27, PE-ruled). `internal/depcheck` inspected **direct**
  imports only, which was honest while `core/` imported nothing third-party. The
  keystone node-store decision makes that untrue, so the PE ruled the gap closed
  in the same change that opens it: a third-party package reaching the
  filesystem, clock, network or ambient randomness could otherwise enter `core`
  without tripping the guard, because the forbidden import sits **one hop away** —
  green check, vanished property. The new guard walks the module-aware closure
  (`go list -deps`) and checks **third-party purity**, deliberately skipping
  stdlib (`fmt` imports `os` by design; flagging stdlib would make it
  unrunnable). **It fired immediately on four pre-existing effects**, so this was
  never hypothetical. Each is now a reviewed, falsifiable claim rather than a
  blanket pass — most usefully: cbor's `math/rand` is **verified unreachable** in
  silt's configuration (its only use is `SortFastShuffle` in `encodeStruct`, and
  silt encodes with `CanonicalEncOptions`), so the entry doubles as a **tripwire**
  — switching encode modes would make block encoding nondeterministic and break
  consensus. The `cpuid`/`os` entry records the assumption it rests on out loud:
  the SIMD and generic Reed-Solomon paths must produce identical bytes, or
  erasure output would vary by host. Proven by ablation: an unlisted third-party
  effect fails the build. This is a **ratchet, not a proof** — it establishes
  that the next effect cannot arrive silently, which is the property that was
  missing.
- **Both certifications landed — canon amended, and the order-varying oracle that
  enforces the refinement** (2026-08-27). Research answered both open consults.
  **#597 (revLog):** the conflict was a *category error*, not a contradiction —
  the SMT choice stands, and `revLog` gets its **own append-only root** rather
  than becoming an SMT leaf. Canon's "one root over all committed state" is
  refined to **one history-independent SMT over set-valued validity state PLUS a
  separate RFC-6962 root for any committed ordered log** (the Ethereum shape:
  stateRoot + receiptsRoot + txRoot); the snapshot carries the **full** revLog
  entry list, preserving H9 proofs for snapshot-booted nodes at the cost of the
  smallest forever term. `epochStart` is reclassified as an **observable** —
  reorg-swapped but under no committed root. **Relay (knob 2):** amended from
  "dispute-only quorum-TTP" to **no TTP at all** — the relay leg is
  *self-enforcing*, so there is no adjudicable dispute; a quorum-TTP could not
  remedy the one-increment stiff anyway, because forwarding is unprovable by any
  mechanism. PayWord chains, ~1–64 KiB increments pinned by a floor-box
  measurement, relay credit = operator balance (no new keystone field), one
  Invariant-A firewall regime. The feared privacy vector (a public dispute naming
  a fetcher key) **dissolves** with the dispute itself. The classification now
  carries a **three-way taxonomy** (`committedSet` / `committedLog` /
  `observable`), and `core/chain/modelcheck_order_independence_test.go` enforces
  the certification's Q4 mandate that the oracle **vary append order**:
  classification alone cannot catch a purely order-derived value. Two histories
  reaching the same final state agree on all **16** set-valued fields and produce
  **different** log roots — the certified resolution, asserted. Proven by
  ablation: reclassifying `revLog` as set-valued makes the oracle name it, i.e.
  it reproduces #597 mechanically.
- **The era-boundary Reload oracle — the keystone's RED home #3, shipped ahead
  of era-3** (2026-08-27). The certification requires this test to land
  **before** the change it governs, and forbids the shape of the mistake:
  *extend the shared era-aware verification path (`verifyAtt`) — never fork a
  parallel era-3 path*, and a failed replay must be a **loud rebuild, never a
  genesis fallback**. Era-3 blocks are not minted yet, so
  `core/chain/reload_era3_boundary_test.go` pins the two properties era-3 will
  need, in a form that extends by one block when it arrives: **(1)** a history
  spanning an era boundary replays through the single `verifyAtt` dispatcher —
  a forked path works fine on a single-era history and breaks exactly at a
  boundary, which is what every real chain is at an activation height; **(2)** a
  block from a **future** era — precisely what era-3 activation creates for
  every un-upgraded node — is rejected **loudly** and reports the honest count
  of what it restored, so no caller can mistake a truncated replay for a
  complete one. That second property is #558 carried forward: the damage there
  was never the rejection, it was the silent fallback that discarded finalized
  history while reporting health. Both are proven RED by ablation (drop the
  `PhaseLegacy` branch → (1) fails; make the `default` branch accept unknown
  eras → (2) fails), and a positive control asserts the rejection names a
  signature/attestation failure so it cannot pass because the forged block was
  malformed for an unrelated reason.
- **The incremental-cost oracle — the keystone's RED home #2**
  (2026-08-27). The certification's Q4 gate: *count actual hash computes per
  block; RED = O(state), GREEN = O(changed·log n) with an explicit budget* —
  guarding the #555 scar (`Hash()` re-marshaling the world on the hot path),
  which would surface not as a correctness failure but as a node quietly falling
  over on the floor box. `internal/smtspike/incremental_cost_test.go` counts
  **digests, not wall-clock**: shared cloud hardware carries ~2× timing
  variance, so a time budget would be either too loose to catch a regression or
  flaky enough to get disabled, whereas a digest count is exact and identical on
  a laptop and a 1 vCPU box. Measured: applying 64 changed keys costs
  **544 / 780 / 978** digests at 1k / 10k / 100k state — **1.80× growth for 100×
  the state**, and 6.78× for 8× the changed keys, so cost tracks `changed` and
  not state size. `budgetK` is set from measurement (0.85–1.61 digests per
  changed key per log₂n, so 3 gives ~1.9× headroom) rather than guessed, and the
  budget constrains the **shape**, not just the constant. The RED case is
  demonstrated rather than asserted: a full-tree recompute costs **44,733
  digests, 18× over budget** — and that test fails if the budget is ever loosened
  enough to admit it, so the constant cannot be quietly inflated to hide a
  regression.
- **Snapshot-boot equivalence — the keystone's RED home #1, both halves**
  (2026-08-27). Part 2 is the differential oracle
  (`core/chain/modelcheck_snapshot_equivalence_test.go`): a validator booted
  from committed state alone — never having replayed the history — must reach
  the same verdicts as one that replayed, which is the property the whole
  keystone rests on. The snapshot-booted replica is built **by reflection over
  the classification**, and one detail falls out for free: `blocks` is classified
  `input` rather than `committed`, so "copy every committed field" yields a
  replica with no history by construction. The **leave-one-out** half is the
  sharp one — omitting a committed field must change a verdict, and the passing
  output is evidence rather than an assertion: dropping `byRoot` turns
  dup-publish from `reject` into **`accept`**; dropping `revoked` breaks
  un-revocation; dropping `bondRootOwner` turns a second identity's claim on an
  already-owned bond root from `claim-blocked` into **`claim-succeeded`** — one
  plot backing two identities, a direct **C1 no-discount** break. The oracle also
  corrected its own probes: it first flagged three fields as not load-bearing,
  and attribution showed the probes were wrong — F1 dedup lives in `apply()`,
  not in a validate predicate, and `bondRegHeight`'s min-interval is gated behind
  `regGateActive` (#506). Probe coverage is **3 of 18** committed fields; the
  rest are declared in `probeUncovered` with what each would need, and the test
  **fails if a committed field is neither probed nor declared**, so the debt
  cannot grow silently. Consensus engine untouched; I1–I5 untouched.
- **State-field completeness ratchet — the keystone's RED home #1, part 1**
  (2026-08-27). The state-root certification makes one obligation load-bearing:
  the field enumeration must be proven complete **by an oracle, not by
  inspection**, because inspection already missed fields. The tempting test —
  capture the listed fields, restore, compare — is inspection wearing a test
  costume: it can only test the list it was handed, so field #17 lands green and
  silent (the #558 class). Instead
  `core/chain/modelcheck_state_completeness_test.go` cross-binds **three**
  enumerations by reflecting over the live `Chain` struct: the classification of
  every field, the populate helper, and **`adopt` — product code on the reorg
  path**. A new field fails classification; once classified it fails populate;
  then it fails `adopt`. Proven failing-first by three ablations (add an
  unclassified field / drop a field from `adopt` / drop one from populate), each
  RED then reverted. **It found a live disagreement:** `Chain` has 25 fields,
  the certification enumerates 16, and `adopt` already copies 19 — the extra
  `revLog` and `epochStart` are written from block history but absent from the
  certified set. `revLog` is the sharp one: it is a **history-dependent**
  append-only transparency log backing the H9 non-globality proofs, so a
  snapshot-booted node cannot rebuild it from set-valued state — which the
  certification's history-independence argument for choosing the SMT does not
  address. Consensus engine untouched; I1–I5 untouched (the tests only read
  state). Reasoning, findings and the still-owed differential half:
  `docs/thinking/2026-08-27-snapshot-boot-equivalence-oracle-design.md`.
- **The keystone SMT spike — `pokt-network/smt` proven, not just read**
  (2026-08-26). The state-root library recommendation rested on quoted source
  rather than executed code, and was explicitly void if a spike disagreed.
  `internal/smtspike/` is that spike, and it agrees. The assertion the gate
  turned on — **an absence proof for a PRESENT key must FAIL** — holds against
  all three adversary shapes, and the test fails if the library's
  `"non-membership proof on related leaf"` guard never fires, so the branch the
  soundness rests on is exercised rather than assumed. Measured cost is stable
  across 1k–1M keys: **2.24 nodes and 218 stored bytes per key**, ~900-byte
  proofs, and applying 100 changed keys costs **2.37 → 3.01 → 3.80 ms** across
  1k → 100k state — 1.27× per 10× of state against the 1.25–1.33× `log n`
  predicts, so the certified `O(changed·log n)` shape holds and the #555
  `O(state)` scar does not return. **Measured on a real 1 vCPU / 2 GB floor box**
  (dedicated `e2-custom-1-2048`, no swap), which confirmed the laptop projection
  to the digit: 751.6 MB heap at 1M vs 751.7 MB on the M4. **The finding: the
  in-memory backend is disqualified.** Residency is 752 B/key — 3.4× the 218 B
  stored payload — and the floor box **OOM-killed** the trie at 2M entries
  (anon-rss 1.68 GiB); even 1M would not fit alongside the flixz daemon's
  1060 MB. The library is adopted, the reference in-memory backend is rejected:
  the trie needs a disk-backed `MapStore` (five methods), which is the only
  configuration that survives build-immutable #8. Boot rebuild measured at ~22 s
  per 1M, a **lower bound** pending the real disk store. The dependency adds
  **zero new indirect dependencies** and no product package imports it.
  Reasoning and numbers:
  `docs/thinking/2026-08-26-keystone-smt-spike-results.md`.
- **PoD neutral lane runs in a real daemon — `-accept-delivery-receipts` +
  `silt swarm receipt`** (2026-08-26). This closes the e2e-tier gap #590 reported
  honestly: the lane was built and proven at unit and sim tiers, but no daemon
  ran it. Now both halves exist. **Server:** a validator started with
  `-accept-delivery-receipts` banks receipts against its own token-issuer key
  (the bilateral issuer==server shape the certification's settlement answer
  covers) and settles the conserved delivery credit. **Fetcher:** `silt swarm
  receipt <root> -peers ID@ADDR` blind-withdraws a retrieval token, signs the
  delivery receipt, and submits it — the fetcher half of the lane, which the CLI
  previously had no surface for. The e2e asserts the full path over real TCP and
  goes past "banked" to assert the **settled credit is non-zero**, so a lane that
  banked a neutral observable while paying nothing would fail rather than pass
  quietly. The lane is off by default; a daemon without the flag refuses the
  receipt and the client reports the refusal instead of a success it did not get.
  Delivery credit remains balance-only and can never reach standing.
- **D-TIERING mode flags — `-archive` and `-serve-content`** (2026-08-26, the
  near-term build-gated items of the tier model). **`-archive` is a genuinely new
  capability:** an archival node retains every block's heavy space-time bond proof
  to genesis instead of shedding it below the rolling retention horizon, so it can
  serve the deep history a pruning swarm has already dropped — the answer a node
  stranded past the prune horizon needs (`ErrNeedCheckpoint`, #559's true-loss
  residual). It is **retention-only, never validity**: the trust floor and
  retention horizon are untouched, pinned by a test asserting an archival and a
  pruning chain agree on both, so the tiers cannot fork against each other. Costs
  O(all history) resident payload — off by default, and build-immutable #8 forbids
  it on the 1 vCPU / 2 GB box, which is the reason the tier model exists.
  **`-serve-content`** is the positive spelling of the content axis (default ON),
  so an edge profile composes as `-serve-content -archive=false -validator=false`
  rather than as a double negative; the legacy `-freeload` is unchanged and
  remains its inverse. A contradictory pair (`-freeload -serve-content=true`) is
  **refused loudly** rather than silently resolved (S3). The announced line
  carries BOTH spellings (`serve-content: OFF (freeload: ON)`) because
  `freeload: ON` is a stable marker the e2e harness and operator tooling grep —
  an announced line is an observable contract (S5), and the first cut of this
  change broke `TestFreeloadRoleSeparation` by renaming it.
- **D-POD-KNOBS — the three Phase-4 economy/state knobs, DECIDED** (2026-08-26,
  owner ratification of the PE recommendations). (1) The delivery-credit **skim
  routes to the object's durability escrow**, not burn — the deciding reason is
  a cross-tier funding loop (edge delivery skim funds that content's durability
  on the persistent tier); conservation carries soundness independently, so the
  skim stays a deterrent knob and must not be raised for anti-wash reasons.
  (2) Relay compensation resolves disputes through a **dispute-only quorum-TTP**,
  under the load-bearing scope condition that a dispute adjudicates **the payment
  chain only, never transit** — which is what keeps the resolution
  signature-verifiable and keeps the verifiable-escrow unknown confined to
  strong-form PoD. (3) A bond root's ownership record **follows current
  possession (TTL-lapse)**, required so the keystone's committed state stays
  bounded; lifetime provenance survives in the archival tier's chain history.
  Recorded in `docs/decisions.md`; (1) is shipped, (2) lands with relay
  compensation after its consult certifies the scope condition, (3) freezes into
  the keystone field set (live ledger behavior unchanged for now).
- **Owner-knob guards + the relay dispute gate answered** (2026-08-26, per
  `silt-reviews/principle-engineer/RULING-PoD-keystone-owner-knobs-2026-08-26.md`,
  which concurs on all three knobs) — two regression locks the PE prescribed.
  `TestPaidBountyIsNotRecoverableBySupersede`: escrow skim-routing is sound
  *because* the supersede reversal floors at the remaining reserve, so a bounty
  already paid for real repair work is never clawed back — if that floor
  regressed, escrow would begin minting recoverable balance and burn-routing
  would become the correct choice instead. `TestRootOwnerFeedsOnlyTheDedup`:
  `rootOwner` feeds the F1 dedup and nothing else (both slash paths dock by
  identity regardless of root ownership), which is the property that makes the
  keystone's TTL-lapse option safe; the companion anti-griefing property is
  already pinned by `core/bond TestRedteamG2_PlotBoundToClaimedIdentity`.
  Recorded with them: the relay-dispute gate is **signature-verifiable** — the
  certification forbids adjudicating transit (no transit proof exists), so the
  only adjudicable quantity is the self-verifying payment chain, and the
  quorum-TTP direction does not reactivate the verifiable-escrow unknown so long
  as relay disputes stay scoped to payment. Builder evidence, routed to the
  relay consult; the three knobs remain the owner's calls.
- **PoD neutral lane BUILT — the witnessed delivery credit, conserved
  (Phase 4 §7.1)** (2026-08-26, per the certified `docs/design/pod.md`) — a
  banked delivery receipt now settles a conserved balance credit: the fetcher's
  withdrawal fee, less the 1/8 durability skim routed to the object's escrow
  (`Ledger.RedeemDeliveryCredit`, wired in `handleDeliveryReceipt`). The
  certified supersede rule ships with it: every object-aware serve's self-credit
  is tracked provisionally per (requester, root) and a witnessed receipt
  REVERSES it before paying — a delivery is never paid twice (the banned
  subsidy). The receipt itself sheds its PoR leg (certification Q2: a
  public-seed proof deters no collusion and cost 128 SW samples/delivery on the
  floor box) — the neutral receipt is token + fetcher signature + the
  (serial‖object‖server) binding, domain-bumped to v2. Firewall pinned at both
  tiers: the Invariant-A guard classifies the new press neutral, a dedicated
  heavy-deliverer test asserts `Reputation()` unchanged, and the sim closes the
  bilateral loop over the wire (pair net = exactly −skim; wash is a strict
  loss). Skim routing defaults to escrow pending the owner-knobs PE consult.
- **PHASE 3 BANKED — the deep-heights exit gate is MET** (2026-08-26, run
  `fe2376a-deep`: 30 pass / 1 gap / 0 fail) — `12-deep-heights` drove h78→h132
  (target 128) at ~48 s/height, the #549 Q4 barrier stabilized in 215 s, the
  retention prune engaged on every validator at depth and the pruned chain
  converged (12b/12c), worst RSS 0.65 GiB, zero OOM, and the S7 economy closed on
  the wire for the third consecutive sheet. Field-confirms and closes the entire
  depth-stall lineage (#549, #560, #561, #572, #573); #183's close condition is
  met (issue held open by owner directive). ROADMAP updated: Phase 3 ✅, the
  publish-bound re-derivation carried as the owed gate clause, Phase 4 (PoD
  spec-first) is the next phase. Evidence: `integration/cloudtest/report-fe2376a-deep.md`
  (artifacts PR #585).
- **#572: save-side regime line — restore/save PAIRS make the next under-latch
  self-locating** (2026-08-26) — every chain persist (commit / catch-up / takedown)
  now prints the same regime snapshot as the restore line plus the head it went down
  with (`chain: saved N block(s) [why] head=H:hash (everMature=… …)`). Paired with
  the restore-time line, one diff decides the remaining #572 premises: last-save ≠
  restore ⇒ store/replay layer; equal-but-wedged ⇒ downstream of restore; the head
  hash pins content. The legacy `restored N block(s) from disk` prefix is preserved
  (integration/consensus greps it).
- **#572 round 3: chain replay proven pure; the divergence hunt moves to the daemon
  layer, instrumented** (2026-08-26) — the write-site audit (every latch/regime map
  writes only in `apply`/`adopt`) plus a full FIELD-SHAPE oracle
  (`TestLatchReplayFieldShape_572`: organic 12-seat gather, renewal treadmill at the
  R-rule-legal cadence, TTL lapse + re-entry, latch + handoff + three rotations,
  wire-faithful Reload) are both GREEN — replaying identical blocks provably
  reproduces the latch at chain level. So 474718e-deep's under-latch must arise
  outside pure replay: the daemon now prints the full regime state at every restore
  (`everMature/matureEpoch/seen/bonded/epochStart/epochSet` via `chain.Regime()`), so
  the next occurrence names the map that failed to rebuild. Also: `ValidateProposal`'s
  objective rejection now names the ACTUAL disqualifying branch (slashed / not in the
  frozen epoch set / not a launch anchor / under-bonded) — the field's misleading
  "bonded 1048576, needs 1048576" was a frozen-set refusal wearing a bond-size
  costume. The #563 memory bench now skips under `-race` (shadow memory + pool
  suppression inflate the live-heap peak ~10×; the budget is only meaningful
  uninstrumented).
- **#572 round 2: latch-replay determinism oracle** (2026-08-26) — the 474718e-deep
  diagnostic named the stall branch (val-d refused every mature commit with
  `ErrAnchorRequired need=3` at h32 after a drill restart restored the 32 blocks that
  had latched `everMature` live at ~h14): replay of a latch-producing history did not
  reproduce the latch. `TestLatchSurvivesReplay_572` asserts latch-replay determinism
  (wire-faithful roundtrip + fresh `Reload`); GREEN on matureWorld12's shape, which
  bounds the remaining search to the field's latch dynamics (validatorsSeen/C2
  accumulation under the renewal/TTL cadence). Full attribution on issue #572.
- **#572 sync-stall repro guards + per-sweep catch-up diagnostic** (2026-08-26) — the
  027c354-deep val-c stall (100+ min of no-progress sweeps) was unattributable because
  every failure branch of the SyncChain walk logs at debug or below. The deterministic
  repro (`core/node/syncstall_572_test.go`, exact 12-seat topology, epoch rotation in
  the gap) EXONERATED the leading suspects: a healed behind seat catches up in one
  sweep, and the field's chain-behind-ahead-mark shape (chain h24 / mark h33 — the
  markstore is atomic, chain.cbor save cadence lagged) neither blocks adoption nor
  moves the I2 mark. Both stay as regression guards. Since the mechanism is still
  unnamed, `SyncChain` now emits ONE warn when a sweep ends with zero adopted blocks
  while demonstrably behind — our-next, max-peer-head, probe/window/append/reconcile
  counters, last branch error — so the next occurrence carries its mechanism
  (RED-proven oracle: `TestSyncStall_572_NoProgressSweepWarns`). No behavior change on
  any sync path. Record: `docs/thinking/2026-08-26-572-sync-stall-attribution.md`.
- **#570 archival-format golden-fixture suite — committed chains every future HEAD must
  replay** (2026-08-25) — four write-once serialized fixtures
  (`core/chain/testdata/archival/`: era-1, era-2, era-2-pruned, mixed era-1→era-2, the
  exact bytes `chainstore.Save` persists) replayed at HEAD against pinned head-hash and
  derived-state constants. Asserts the property no HEAD-minted test can: bytes written by
  an older binary must replay today — the #558 class (era-2 replay silently falling to
  genesis from #432 until last week) is now caught locally, RED-proven by reinstating the
  #558 bare-hash verification (era-2/pruned/mixed fail at block 1; era-1 stays green).
  Head-hash pins also catch silent hash-computation changes over committed bytes. Era-3
  (the D-TIERING state root) must ADD a fixture here — this suite is its standing RED
  home (consult Q5). Record: `docs/thinking/2026-08-25-570-archival-fixture-suite.md`.

### Changed
- **Quarantined the `TestRepairBountyPaysOnTheWire` e2e (#514)** (2026-08-27). The
  repair-bounty wire proof carries a PREMISE-ARMING flake: the kill-selector's holders-view
  can diverge from byte-reality, so the stripe is not always armed the way the test assumes.
  The test is `t.Skip`-ped at the top to unblock the verified era-3 keystone probe work
  (#604/#606) whose e2e job was catching this unrelated flake. This is a TOP-PRIORITY fix,
  not an accepted state — un-skip when #514 is proven closed by stress.

### Docs
- **#600 ratified — the floor box is a semi-stateless witness-validating full validator**
  (2026-08-28). Andrew ratified the direction: witness-validation is the floor box's primary
  validation posture; holding the whole registry tree is a bigger-box opt-in behind
  `ports.NodeStore`, never the 2 GB-floor default. Recorded in `docs/decisions.md` (D-TIERING,
  dated entry) with five consequences: (1) same security, narrower self-sufficiency; (2) a HARD
  REQUIREMENT that witness-serving stay open + multi-provider (`TENETS.md:557` — a permissioned
  availability choke is the banned load-bearing-centralized dependency); (3) the ≥1-honest-provider
  liveness assumption promoted optional → load-bearing (safety unaffected — a witness-less floor
  box stalls, never accepts), a new #183-sibling seam; (4) bbolt NOT reopened (pebble ties its
  heap); (5) an HONEST evidence basis — the billable coexistence run captured zero rssMB rows (killed
  by `-timeout 60m` mid-build) and showed severe memory pressure, so the decision rests on C-7
  soundness + no-owed-measurement + hold-tree-unproven-to-fit + the pressure signal, NOT a
  conclusive OOM. `docs/VISION.md` marks the witness floor RATIFIED and adds the decentralized-liveness
  posture; `ROADMAP.md` re-sequences the era-3 format freeze to critical-path (witness is vacuous
  until the `Block` commits both roots). PE ruling
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-600-floor-box-direction-2026-08-28.md`;
  research note
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/600-floor-box-direction-post-coexistence-RESEARCH-NOTE-2026-08-28.md`.
- **VISION + canon honesty pass — C-1 lift to a conditional theorem, C-5 operator-economics
  true-up, and C-2/C-3 register fixes** (2026-08-27). One coherent pass re-anchoring the north
  star to ratified canon. **C-1 (maturity before capture)** lifts GATED → **CERTIFIED-CONDITIONAL**
  (conditional-theorem lift): maturity provably precedes capture as **Theorem CT-1** under an
  honest-arrival floor (H), a declared adversary budget (B), and a parameter constraint (P), with
  the falsifiable crossing inequality **`W_A < 2·w_min·M_req`** — still not unconditional (the
  weak-subjectivity wall). `docs/VISION.md`, `docs/decisions.md` (supersedes the prior GATED entry),
  `docs/design/m0.md` §10 (CT-1), and `docs/design/owned-residuals.md` E3 trued up; the #183 brief
  re-prices R1 to the inequality, names R5's attack region `W_A ≥ 2·w_min·M_req`, and opens **R6 —
  the H⊥B independence break** (an adversary's staged bonds count in both the honest-arrival floor
  and the capture weight, so `λ_H` must be measured as address-diverse arrival). **C-5 (honest
  operator composed economics)** ratified **GATED**: the γ→1/N firewall and conservation hold under
  the composition and no defense prices out the small operator; a FACTUAL VISION correction — the
  repair bounty pays the **new holder** of a rebuilt shard (custody rent), not the reconstructor
  (unpaid caretaker duty) — plus the hot/cold scope (repair self-funds hot objects; cold rides a
  funded horizon). The floor-box reconstruction RAM (G2) is owed before the economy-ON field run
  (`owned-residuals.md` D6). **C-2/C-3 register fix:** VISION's multiplicative-interlock paragraph
  now carries m0.md's own `C_honest ≈ D`-today and declaration-cheap-A-axis qualifiers
  (target-not-yet-live). Distinguishes factual errors (C-5 G1 — fixed) from register drift (C-1,
  C-2/C-3 — qualified); VISION stays a north star, not a status report. Certifications:
  `silt-reviews/research/research-outcome/C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27.md`,
  `silt-reviews/research/research-outcome/C5-honest-operator-economics-composition-RESEARCH-CERTIFICATION-2026-08-27.md`.
- **Canon true-up recording two ratified research certifications** (2026-08-27). C-7
  (witness-based floor-box validation) is CERTIFIED sound + complete: soundness no longer
  blocks the #600 direction, and the era-3 format now carries a HARD freeze prerequisite —
  the `Block` must commit both the state SMT root and the append-only transparency-log root
  over the completeness- and order-independence-proven field set, and the floor-box verifier
  must hold the invariant "no witness → never accept (stall)" (the block commits neither root
  today, `core/chain/chain.go:311-405`). C-1 ("maturity before capture") is ratified as a
  safe-parameterization, not a theorem, confirming the canon: the `everMature` latch is
  certified one-way — it bounds the consequence of a lost bet, not the reachability of
  pre-maturity capture. `docs/VISION.md` §108 trued up to carry the qualifier; two
  `docs/decisions.md` entries; C-7 residuals added to `docs/design/owned-residuals.md` (E6)
  and the C-1 R6 doc-register residual closed (E3); the #183 red-team brief sharpened (R1 is
  the live seam, R2 and R3-safety are CLOSED). Certifications:
  `silt-reviews/research/research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`,
  `silt-reviews/research/research-outcome/C1-maturity-before-capture-RESEARCH-CERTIFICATION-2026-08-27.md`.

### Fixed
- **#514 ROOT CAUSE — the repair-bounty flake: the premise killed BEFORE DHT
  convergence, so publish-time lost-ack extra copies re-converged and healed the
  loss within slack** (2026-08-27) — `TestRepairBountyPaysOnTheWire` failed ~20% of
  runs with a premise defeat: the kill-selector killed a column's holders but the
  caretaker's byte-confirmed sweep saw `missing ≤ slack` and never armed repair.
  Mechanism, pinned by the caretaker's own sweep trace: the object carries
  publish-time lost-ack extra copies (#497 — `-replication 1` does NOT mean one
  holder, a lost ack mints a silent extra copy) whose provider records converge a
  sweep or two AFTER the kill. The caretaker's first post-kill sweep DID see the
  loss over slack, but `reachable` then climbed as the hidden copies surfaced, the
  loss healed within slack, and the #517 two-sweep confirmation gate reset. Neither
  the record view nor a byte-confirmed selector view could see the hidden copies at
  kill time. **Two parts.** (1) HARNESS — the premise is now deterministic:
  STABILIZE (wait until the byte-confirmed `swarm holders` view stops changing, so
  every real byte-holder including the lost-ack copies is listed), SELECT within
  `(slack, n−k]` (killing a node removes every column it holds, so bound the loss so
  a stripe stays ≥ k and the bounty can pay), KILL ALL byte-holders of the target
  columns, then CONFIRM on the caretaker's OWN sweep (a stripe over slack),
  re-killing any surfaced copy (the caretaker's own DHT vantage can resolve a copy
  the selector could not) and re-publishing under a fresh root if placement
  concentrates all columns onto 2-3 nodes (the cloud grade records that as "economy
  UNTESTED, not failed" — the e2e re-rolls it instead). (2) PRODUCT — `ColumnHolders` (`swarm
  holders`) byte-confirms each column's provider records with `MsgHasChunk`
  (`confirmColumnHolders`), so the operator/selector view no longer reports phantom
  holders, corpse-gated exactly like `probeShard` (`repair.go:479`) so a stale
  record to a departed holder costs one `HolderDialTimeout` for the whole walk
  instead of one per shard — closing the dead-holder dial-storm PR #607's ungated
  all-shards walk re-introduced (the #226/#277/#501 class). RED-proven at the node
  tier (`core/node/column_holders_bytes_514_test.go`): a phantom record-holder with
  no bytes is dropped by the byte-confirmed view (ablate the confirm ⇒ lists the
  phantom); a dead record-holder is dialed exactly once, not once per shard (ablate
  the corpse-gate ⇒ 5 dials on a 5-shard column). The invariant the e2e proves (a
  verified reconstruction PAYS) is untouched — only the premise arming is made
  deterministic. Extends PR #607 (its byte-confirm direction was sound; its scoping
  and its selector-only premise were not). Evidence: 50/50 green serial iterations
  (`docs/thinking/2026-08-27-514-repair-bounty-50x-evidence.txt`), where the flake
  reproduced ~20% pre-#607 and ~5% post-#607.
- **#572 ROOT CAUSE — the restore under-latch: the daemon replayed history before
  wiring the bond verifier** (2026-08-26) — `objective()` is `MinBond>0 AND
  verifyBond!=nil`, and `EnableObjectiveChain` ran ~80 lines after
  `chainstore.Replay`, so every restore replayed under the LEGACY rep-gated
  qualification with an empty boot ledger: `validatorsSeen` rebuilt EMPTY, the
  `everMature` latch was silently lost, and the restored validator demanded
  launch-rule anchors for mature commits forever — the 474718e-deep/8a52aba-deep
  drill-restart wedge, proven by the save/restore regime pairs (saved
  `seen=12 everMature=true` → restored `seen=0 everMature=false` over identical
  blocks). Fix: the daemon wires `node.SpaceTimeBondVerifier` (factored from
  `EnableObjectiveChain`) BEFORE Replay, and `Reload` now REFUSES an
  objective-config replay with no verifier — the ordering can never regress
  silently. RED-proven (guard removed → the replay proceeds and, at the field's
  `-min-rep`, under-latches); the field-shape oracle asserts both the refusal and
  the with-verifier latch. `Reconcile`'s tmp replica already inherited the
  verifier (chain.go:3089) — that doorway was never open.
- **#563 cold-sync Reconcile OOM on the 2 GB box — the hypothesis was garbage, literally**
  (2026-08-25) — the deep-run kernel-OOM (a434494-deep, val-d ×2) was attributed by a new
  deterministic memory oracle (`core/chain/reconcile_mem_563_test.go`, born RED): there is
  NO 2–3× resident fork copy (retained-after-GC is negative — adoption shares payload
  backing); the spike is ~1× fork-bytes of transient CBOR garbage from each decoded
  block's first `Block.Hash()` materializing its full multi-MB body, on top of a measured
  2.35× decode inflation and GOGC=100's heap-doubling headroom. Two-leg fix: `Hash()` now
  marshals into a pooled buffer (`UserBufferEncMode.MarshalToBuffer`, byte-identity with
  the reference encoder asserted by `TestHashPooledBufferIdentity_563` — same bytes, same
  hashes, no consensus surface), dropping the Reconcile peak extra 69→6 MiB; and the
  cloudtest fleet now DEFAULTS `MEM_LIMIT=1500M` (the a9cfc06-proven GOMEMLIMIT guard the
  OOM'd run had silently dropped — `console-a434494-deep.log` carries no `-mem-limit`).
  Deliberation + outcome: `docs/thinking/2026-08-25-563-reconcile-memory-bench-deliberation.md`.
- **#562 renewal jitter grid clamped to the #506 R-rule — no more refused-renewal sweeps**
  (2026-08-25) — the #555 phase-jitter's nearest-grid rounding reaches down to TTL/4 blocks
  after the last committed reg, but the reg-inclusion rate bound R = K+2 can exceed TTL/4
  (10 vs 8 at the field TTL=32), so a colliding identity's renewal was refused every sweep
  ("re-registering 9 blocks after its last reg (R=10)", a434494-deep) until the chain
  outran R. `renewalDueHeight` (factored out of `BondRenewalDue` for direct testability)
  now clamps the grid point to the rate bound: at most one off-grid cycle (the next due
  point re-aligns to the grid), steady-state periods stay exactly TTL/2 (the #313/#556
  property), and unlike jumping to the next grid point it can never overshoot the TTL at
  small TTLs. Client-side pacing only. RED-proven:
  `TestRenewalDueClearsRegRateBound_562` (colliding phases 12/13 land at +8/+9 < R
  pre-clamp; on-grid steady state asserted inert).
- **#561 round-escape decoupled from the chain-sync peer walk — dead peers can no longer
  stall the view-change** (2026-08-25) — `maybeAdvanceRound` (the #432 escape counter) ran
  inside `SyncChain`'s completion callback, which fires only after the sequential ask-walk
  over every peer completes; dead peers stretch that walk by their full retry budgets. In
  the a434494-deep 10a stall-drill (4 of 12 peers stopped), the honest cohort's first
  round-change came ~8 minutes after the stall against a 430 s bound built on 30 s sweeps —
  while renewals (tick-driven) flowed on schedule the whole time. The escape now runs on
  the TICK: it needs only local state (pending work + the sweep count), tick cadence is the
  #549 Q3 skew-bound premise, and the new-view proposal path was already message-driven at
  the designee. The drain keeps its freshest-head property (#338) by staying in the
  callback. RED-proven: `core/node/roundescape_tick_561_test.go` (held delivery models the
  never-completing walk; pre-fix the node never leaves round 0).
- **#558 era-2 chain replay always failed — silent genesis fallback on every validator
  restart, exposed as a stranding at depth** (2026-08-25) — `validateStructural` (the
  `Reload` path a restarted daemon replays its own chain.cbor through) verified attester
  signatures over the bare block hash — the era-1 form. Era-2 (#432) attestations sign the
  domain-separated `consensusSigBytes(phase, round, hash)`, so replay of any era-2 chain
  failed at its first non-genesis block with `bad signature`, and the daemon silently fell
  back to genesis. Invisible until now: peer catch-up re-fetched the whole chain after
  every restart (an expensive hidden full Reconcile — a #555-adjacent load source). In the
  a434494-deep run the retention prune removed that mask: OOM-restarted val-d could not
  re-sync below the prune horizon and was stranded at genesis with its intact h83 store on
  disk (not a torn write — `chainstore.Save` is atomic). Fix: `validateStructural` uses the
  shared era-aware `verifyAtt` (the live commit path's arithmetic); `Reload` keeps the
  longest valid prefix; the daemon names a replay failure loudly (prefix kept, prune-horizon
  consequence, #559 pointer) instead of a one-line stderr note. RED-proven:
  `core/chain/reload_era2_558_test.go` reproduces the exact field failure (era-2 + pruned
  block replay through the persisted representation) against the pre-fix code.
- **#555 deep-drive crawl attributed and fixed — `Block.Hash` memoized; the crawl was
  hash-work saturation, not gather latency** (2026-08-25) — The 95d39e8-deep field log
  produced the measurement the #555 certification held for: the intrinsic two-phase gather
  is ~10 s at 12-seat WAN (h74: new-view → commit in 9.6 s), well inside the 60 s round-0
  base — the apparent 90–150 s "gather" was event-loop saturation. ChainReply processing
  blocked the single node thread 16–86 s per reply (cost growing 2.4 s → 42 s with depth),
  stretching the sweep timers (waited p50 18 s, p90 146 s) and starving the gather; the
  watchdog stacks pin the work to `Reconcile → recentBondRegNonces → blockByHash →
  Block.Hash → sha256` — the hash re-marshaled the full block body (~1.5 MB per reg proof)
  on every call, recomputed per scan step, K=8 lookups per validated block: O(depth ×
  window × scan) per full fetch, self-sustained by probe timeouts forcing more full
  fetches. Fix: memoize `Block.Hash` (decode-fresh on the wire; `Sign` invalidates; pruned
  branch keeps priority) — one hash per block per lifetime; no consensus rule, no wire
  change, no timing constant (`roundAdvanceSweeps` stays 2 — the #549 Q3 skew derivation
  remains the binding lower bound). RED-proven hash-work oracle
  `core/chain/reconcile_hashwork_555_test.go`: 798 → 25 computations for a 24-block
  cold-sync reconcile; `Head()` now does zero hash work. Attribution:
  `docs/thinking/2026-08-25-555-crawl-attribution.md`.

### Added
- **#549 in-process repro of the DEEP-run h68 stall — the field cause is not synchronizer
  logic** (2026-08-24) — `core/node/modelcheck_549_scatter_test.go` models the field's
  distinguishing dimension (sybils active in the frozen set inflating the low-round
  round-change head-count while the heavy validator weight is mass-scattered across rounds).
  It is GREEN even under 50% sustained round-change loss (commits at round 2): because
  `RoundCatchupMet` is weight-based, sybil head-count cannot dilute convergence, so the h68
  stall is not a synchronizer-logic defect but a real-WAN wall-clock timer-skew + scale
  dimension the untimed model cannot reproduce. Feeds the research consult
  (`silt-reviews/research/549-h68-view-synchronization-stall-CONSULT.md`); a RED here would
  be the home for any logic fix.
- **#183 red-team coverage caveats C-1 + C-2 closed — I5/I2 exhaustive oracles + disk-backed
  I2 durability** (2026-08-24) — The external red-team verdict (M0 HOLDS) noted two harness
  assurance gaps, not protocol defects; both are now closed. **C-1:** the I5 and I2 oracles,
  previously covered by scenario tests, are promoted into the exhaustive-enumeration tier —
  `core/chain/modelcheck_i5_accountable_test.go` drives the real `VerifyEquivocation` over
  the full 2^4×2^4×2 space of same-height signature schedules (honest-never-slashed AND
  completeness, both directions), sweeps fork-choice determinism over all 24 permutations of
  a fork set (up from 3 hand-picked orders), and pins the bare-ProposerSig-is-not-a-vote
  exemption; `core/node/modelcheck_i2_exhaustive_test.go` drives the real `signAllowedAt`
  watermark against a mark reloaded across a restart over every (signed slot)×(competitor
  slot) pair in the {height,round,phase,hash} space. **C-2:** the I2 fsync-before-the-wire
  durability, previously verified only by inspection (all restart tests used
  `markstore.NewMem`), now has a disk-backed home — `adapters/markstore/markstore_test.go`
  (the first tests for that package: Save/Load survives a fresh-instance restart, atomic
  overwrite leaves no torn file, missing-vs-corrupt is start-vs-refuse) and
  `core/node/i2_disk_durability_183_test.go` (a real `Disk`-backed node refuses a same-slot
  competitor across restart; `recordSign` withholds the signature when the store cannot
  persist the mark — the fail-safe against an honest self-slash). All RED-proven against
  reverted mechanisms.
- **#535 fix (3): the operator-directed weak-subjectivity liveness-floor escape**
  (2026-08-24) — The remaining layer of the certified recovery stack, built per the
  ratified design (PR #544). `Config.LivenessRecoveryHeight` (daemon
  `-liveness-recovery-height`, 0 = off) names ONE epoch-boundary height at which
  mature-epoch validation re-bases proposer/attester qualification and the >⅔ weight
  quorum against the LIVE qualified bonded set instead of the frozen `epochSet` — a
  single `effectiveEpochSet(h)` consulted by all three predicates (and the gather's
  `SupportMeetsQuorum`/solicitation, now height-threaded), so the set a quorum is
  sized over and the set it is filled from can never differ (the #402 law). After the
  recovered boundary commits, the normal rotation freezes the same live set and the
  chain resumes. NEVER automatic: a genuine >⅓-of-frozen-weight loss is outside the
  BFT liveness model (automatic re-basing was refuted — fix (2)), so a bled boundary
  stalls by default and the trust moves to a HUMAN who confirms the loss is a real
  outage and coordinates the SAME height on every honest node — the WSCheckpoint
  trust class, with the wrongly-invoked-recovery fork as the documented residual.
  Operator visibility (S5): `Chain.BoundaryLivenessFloorLost` diagnoses the wedge
  (live-bonded frozen weight ≤ ⅔ bar), the round-change path logs
  `stalled-at-boundary` naming the recovery, and `chain-status` flags a
  next-height-is-a-boundary head. Model-check
  (`core/chain/modelcheck_535_fix3_recovery_test.go`, RED-proven against the pre-fix
  rule): recovery-when-invoked commits the wedge topology and resumes; off/wrong/
  non-boundary directives still stall (`ErrNoQuorumWeight`); replicas replaying the
  same directive reach the identical head. Detail:
  [docs/thinking/2026-08-24-535-fix3-built.md](docs/thinking/2026-08-24-535-fix3-built.md).
- **#535 fix (2) refuted by the proof-first model-check — the recovery stack is (4) + (3)**
  (2026-08-24) — The certification adopted boundary-local quorum re-basing (`old∩next`)
  *conditional* on a model-checked #402 handoff-intersection proof for a bled set.
  `core/chain/modelcheck_535_fix2_rebasing_test.go` discharges that obligation and finds
  the naive form **unsafe**: a boundary block finalizes a fork iff Byzantine weight exceeds
  ⅓ of the *re-based* total, and excluding possibly-honest lapsed weight raises that fraction
  — at the field numbers, 171 MiB Byzantine is safe against the full 516 MiB frozen set but
  breaks I1 over the re-based 324 MiB. This is the same fault-tolerance wall that sank fix (1).
  Automatic denominator re-basing is not safely realizable; the certification pre-ruled the
  fallback (fix (3), the operator-signaled weak-subjectivity escape, is the guaranteed-safe
  recovery). The model-check stands as the permanent evidence + regression (it asserts the
  shipped no-re-basing behavior — a bled boundary STALLS rather than forks — is the safe one).
  Detail: [docs/thinking/2026-08-24-535-fix2-refuted-stack-is-4-plus-3.md](docs/thinking/2026-08-24-535-fix2-refuted-stack-is-4-plus-3.md).

### Fixed
- **#555 fix (b): bond-renewal phase-jitter — spread renewals so ~1 reg lands per block**
  (2026-08-25, research-certified) — The deep-drive crawl (#555) was inflated by heavy blocks:
  validators that all registered near genesis hit the TTL/2 renewal point together, so 5–7
  ~1.5 MB space-time proofs landed in one block, and each attester verifies every proof on the
  two-phase gather's critical path before signing (1 vCPU box) — inflating the gather latency
  that drives the crawl. `BondRenewalDue` now places each identity's renewal on a per-identity
  ABSOLUTE grid (period TTL/2, phase = a deterministic offset), rounded to the nearest grid
  point within ±TTL/4, so the genesis-aligned fleet's first renewal spreads across
  [TTL/4, 3·TTL/4) (≈1 reg/block) while the PERIOD stays exactly TTL/2 on every later cycle —
  keeping the #313 re-registration-frequency bound and the ≥TTL/4 renewal margin intact. It is
  client-side pacing (BondRenewalDue gates the node's own drain/submit, never block validation),
  so it changes WHEN an identity re-proves, never a consensus rule; the TTL denomination and its
  #503 couplings are untouched. This is the certified "lighter blocks first" half of #555;
  fix (a), sizing the round base to the (now reduced) gather latency, follows. Tests:
  `core/chain/renewal_jitter_555_test.go` (spread + ≥TTL/4 margin + determinism; RED-proven by
  neutering the offset → clustering).
- **cloudtest: 5-convergence grades with a bounded wait-for-convergence** (2026-08-24) — The
  flow-5 convergence check took a single point-in-time sample immediately after the
  6-fault-tolerance drill stops/restarts val-d, so it could read a spurious catch-up lag as a
  FAIL (observed on run eb510a7-deep: val-a=33 but val-d=27 right after the restart). It now
  polls until the validators converge (within 2 of tip + shared tip hash) or `CONVERGE_WAIT_S`
  (default 120s) expires — the same grade-after-stabilization lesson as the #549 Q4 deep-heights
  barrier, applied to flow 5. A genuine non-convergence or a persistent fork still FAILs; only a
  transient post-drill lag (or a fork fork-choice is still resolving) is given time to settle.
- **#549 Q3 companion: round-duration base derived + guarded (no numeric change)** (2026-08-24)
  — Assessed the certification's Q3 (round-duration base tune). Measurement: the cross-region
  sweep-timer skew is structurally bounded by `ChainSyncInterval` (each node sweeps once per
  interval at an arbitrary phase, so two nodes' round-change timeouts differ by < 30s; WAN
  delivery ~80ms is negligible). So `roundAdvanceSweeps = 2` (base = 60s = 2× the skew) is
  already the **smallest** value that reliably outruns it — `1` (= 30s = the skew) has zero
  overlap margin, `3+` is "larger than necessary" (slower recovery + churn on the 2 GB box,
  against the cert's M1 guidance). No numeric change; instead the derivation is made explicit
  (the constant is now derived-not-magic, build-immutable #5) and pinned by
  `TestRoundBaseOutrunsSkew`, which fails if the base ever drops to the skew or drifts above
  the certified minimum. Deliberation: `docs/thinking/2026-08-24-549-q3-round-duration.md`.
  This closes the #549 certified companions; the finding now needs only a clean DEEP
  field-confirm.
- **#549 Q4 companion: cloudtest deep-heights stabilization barrier** (2026-08-24) — The
  Phase-3 deep-heights flow graded liveness immediately after the maturing drills
  (10a/10b/10c) mass-restart 8 of 12 seats, so it measured post-restart CHURN rather than
  steady state (the certification's Q4). The harness now requires the network to reach GST
  before the drive grades — all validators converged on ONE head AND one fresh commit under
  normal conditions — via a bounded barrier (`STABILIZE_S`, default two per-height
  worst-cases). A network that cannot re-stabilize after the drills is reported as a degraded
  PREMISE (GAP), never a deep FAIL. This is the harness half of the #549 fix: the
  catch-up-target change makes convergence sound, and this barrier stops the harness from
  grading before it completes.
- **#549: the h68 view-synchronization stall — catch-up jumps to the highest qualifying
  round, not the smallest of the union** (2026-08-24, research-certified) — The DEEP-run
  Phase-3 exit gate stalled at h68 for ~26 minutes (r1-congestion, no prepare-QC) after the
  drill sequence mass-restarted 8 of 12 seats. Root cause (certification
  `silt-reviews/research/research-outcome/549-h68-view-synchronization-stall-RESEARCH-CERTIFICATION-2026-08-24.md`):
  `maybeCatchUpRound` unioned round-change senders across ALL rounds above the current one,
  checked the weight threshold on that union, then jumped to the SMALLEST such round — a round
  that may carry only a fraction of the union's weight (structurally unable to form a QC).
  Because `duration(r)=base+r(r+1)/2` is keyed to the round number, targeting the smallest
  pinned the effective round low, so the increasing-duration ladder never outran 3-region WAN
  + 30s timer skew and the after-GST convergence guarantee never engaged. Fix: jump to the
  HIGHEST round that INDIVIDUALLY meets the catch-up weight threshold — coalesce the weight at
  the leading edge and let the ladder climb. Safety untouched (I1/locking): it changes only
  WHEN a node changes round, never which value it may sign, and is still gated on >⅓ weight
  (a Byzantine minority cannot drag the round forward), so the anti-overshoot property PBFT's
  "smallest view" rule sought is preserved — evaluated per round rather than on the union.
  Deterministic RED/GREEN home: `core/node/modelcheck_549_catchup_test.go` (drives the real
  `maybeCatchUpRound` over a low-weight-trailing + quorum-weight-leading smear — RED jumps to
  the sub-threshold trailing round, GREEN to the qualifying leading round). Companions (not in
  this change): a harness post-mass-restart stabilization barrier and a round-duration base
  tune sized to measured cross-region skew.
- **#183 red-team F-1: the `MsgSubmitEntry` CPU-DoS gap — per-sender rate gate + cheap
  replay reject** (2026-08-24) — The first external red-team engagement (verdict: **M0
  HOLDS** at the shipped defaults) handed back one real, bounded liveness finding: under
  `-require-tokens` (publisher privacy, off by default), the entry-submit path had none of
  the #424/Phase-1.2 CPU hardening its structurally-identical sibling `MsgSubmitBondReg`
  has. A single peer could harvest a public committed token, pair it with a novel `Root`,
  and flood `MsgSubmitEntry` — `ValidateEntry` ran `publishtoken.Verify` (an RSA modexp per
  signature) to completion on the single consensus loop *before* a spent-serial check placed
  after it caught the replay. Two fixes, both admission-order, no consensus rule touched
  (build-immutable #3 intact): (1) `allowEntrySubmit(from)` — a per-sender window burst gate
  charged BEFORE decode+validate, mirroring `allowBondSubmit`; (2) `ValidateEntry` now checks
  `c.spent[serial]` BEFORE `publishtoken.Verify`, so a replayed (already-spent) token fails on
  an O(1) map lookup instead of N modexps. Failing-first regressions, both RED-proven against
  the pre-fix code: `core/chain/entry_replay_cpu_183_test.go` (a spent token with tampered
  sigs must fail `ErrTokenSpent`, proving the verify was skipped) and
  `core/node/entrysubmit_gate_183_test.go` (a 100-message single-sender flood queues at most
  `entrySubmitBurst`; a second sender is not starved; the window refills). Bounded/conditional
  — absent at the shipped default (tokens off), where the residual is the already-documented,
  shelved byte-flood E5.
- **#535 fix (4): the R-gate restore exemption — a returning frozen member can re-bond to
  heal a stalled boundary** (2026-08-23, research-certified) — The h64 epoch-boundary wedge's
  non-recovery was compounded by #506: a member whose standing lapsed was R-refused when it
  tried to re-register (`re-registering 1 block after its last reg, R=10`), so it could not
  restore its weight to help the boundary commit. Fix: a re-registration that RESTORES standing
  the identity already held — a current frozen-epoch member (`epochSet`) re-proving a `Root` it
  already owns (`bondRootOwner`, which survives a lapse) while its standing has lapsed
  (`bonded < MinBond`) — is exempt from the R interval. Safe by construction: it can only
  restore weight the honest set already trusted for the epoch, never admit new weight, so it
  cannot cheapen capture (unlike shrinking the quorum denominator — the certification's rejected
  fix (1), which cheapened cost-to-corrupt 344→216 MiB). Narrow: a still-bonded member re-proving
  its root is the #506 storm and stays refused (flood protection intact). This is layer (4) of the
  certified recovery stack; (2) boundary-local re-basing and (3) the weak-subjectivity liveness
  escape are the remaining layers. Cert:
  `silt-reviews/research/research-outcome/535-epoch-boundary-liveness-cliff-RESEARCH-CERTIFICATION-2026-08-23.md`.
- **#536 cloudtest: the escape fingerprint read round-changes from the wrong channel →
  a manufactured WEDGE FAIL** (2026-08-23) — `ft_escape_progress` counted `round-change`
  from journald (`jlog_since`), but that structured `n.logf` line is written to
  `$STORE/debug.log` only, never journald (`cmd/silt/daemon.go` openLog) — so `rc` read 0
  on every sample regardless of ladder activity, and a LIVE ladder (114 round-change lines
  at h64 r1→r5 in run 45da13c-17686's captured debug.log) fingerprinted as FROZEN, grading
  the down-designee flow a WEDGE FAIL instead of the honest "advancing-but-uncommitted"
  out-of-model GAP. Fix: read round-changes from `debug.log` (`dlog`), time-scoped by the
  ISO-timestamp column ≥ the kill instant (new portable `epoch_to_iso`); and — the #525
  lesson extended to the empty-read case — an UNREADABLE source now yields `?`, never `0`,
  and the WEDGE-FAIL branch requires a readable (no-`?`) frozen fingerprint. Shipped with an
  offline RED/GREEN self-test (`integration/cloudtest/check_escape_fingerprint.sh`, wired
  into CI) per the third-time rule.

### Added
- **#535 deterministic repro: the epoch-boundary liveness cliff (consensus model-check)**
  (2026-08-23) — `core/chain/modelcheck_535_boundary_wedge_test.go` pins the mechanism the
  first Phase 3 deep field run (45da13c-17686) wedged on: the mature-epoch finality quorum
  needs signers holding > ⅔ of the FROZEN epoch weight, but a member that lapses or goes
  offline mid-epoch keeps its frozen weight in the denominator for the whole epoch — so once
  > ⅓ of frozen weight cannot sign, no block reaches the super-quorum, INCLUDING the boundary
  block whose commit is the only event that rotates the snapshot to a lighter set (a permanent
  stall that cannot self-heal; #506's R-gate compounds the non-recovery). The repro reproduces
  the field arithmetic exactly (a 9-of-12 live coalition at 324 MiB refused; a 10-of-12 control
  at 388 MiB commits). This is a consensus-rule question (whether the frozen denominator should
  exclude provably-lapsed members) — research-gated, consult filed at
  `silt-reviews/research/535-epoch-boundary-liveness-cliff-CONSULT.md`; **no unilateral fix**.
  Attribution: [docs/thinking/2026-08-23-535-boundary-wedge-attribution.md](docs/thinking/2026-08-23-535-boundary-wedge-attribution.md).
- **The DEEP=1 exit-gate flow + chain-status prune visibility (ROADMAP Phase 3)**
  (2026-08-23) — `flow_deep_heights` (opt-in `DEEP=1`, `DEEP_TARGET=128` default) drives
  the chain past the maturing drills to depth with three graded rows: the honest
  ceiling reaches the target inside a wall bound with the #525 freeze early-exit
  (a crawl/stall at depth is itself the Phase 3 finding, reported with measured
  cadence); the retention prune is confirmed ENGAGED on every validator from real
  persisted state; and the flow-5 convergence probe re-runs on the pruned chain (the
  slice-5 suffix-sync-around-the-gap property at depth, on the #528 suffix-append
  path). Supporting product change: `silt chain-status` now prints the
  payload-stripped block count, so an operator (and the harness) confirms the prune
  from `chain.cbor` rather than a debug log line. Per-height bounds reuse the
  #451/#525 topology-aware arithmetic (610 s at 12 seats, verified against flow 10's
  certified value). Design deliberation:
  [docs/thinking/2026-08-23-deep-heights-exit-gate-design.md](docs/thinking/2026-08-23-deep-heights-exit-gate-design.md).
- **#299 measured: the 1.5 MB bond answer is a parameter question, not an encoding one**
  (2026-08-23) — Committed measurements (`core/bond/answer_size_measure_test.go`,
  `verify_cpu_measure_test.go`) decompose the answer: label-open blocks are 1264 KiB of
  the 1513 KiB total (64 opens x 5 x 4 KiB raw plot bytes); cross-open duplicate leaves
  are 0.9% at 64 MiB (the issue's dedup interim is refuted at scale); the Merkle
  multiproof union floor saves ~6% of the total; verify CPU is 1.8 ms/answer (batch
  verification not a cost center). The 10x levers (`DefaultLabelSamples`, `BlockSize`)
  are soundness parameters -> research consult filed
  (`silt-reviews/research/299-label-samples-answer-size-CONSULT.md`); Phase 3's
  multiproof/batch-verify tiers are deliberately NOT built on this evidence. Deliberation:
  [docs/thinking/2026-08-23-299-answer-size-evidence.md](docs/thinking/2026-08-23-299-answer-size-evidence.md).

- **The #506 version gate: the per-identity reg-inclusion rate bound ships as a validity
  rule behind a BFT-native activation** (2026-08-22) — The #503-certified R-rule is now
  enforceable: past the activation boundary, a bond registration is a valid block payload
  only if its identity is unslashed (R∞ — the Defect-A commit path closed structurally,
  beyond the #508 proposer filter) and its last committed reg is ≥ R blocks old
  (R = max(TTL/4, K+2), derived; first registrations exempt; one identity cannot register
  twice in one block; `ValidateBondRegErr` pre-filters submissions so an honest proposer
  never mints a block its own rule rejects). Activation follows the research certification
  (2026-08-22, BIP9 schema re-based on silt's primitives): each bond reg carries a signed
  readiness byte (`BondReg.Version`, conditionally signed like `Domain`, hash-committed,
  prune-surviving — deliberately NOT on the attestation, which `Block.Hash` does not
  commit and any re-serving peer could strip); at each mature epoch boundary the frozen
  set's rule-aware WEIGHT (never heads — a cheap-bond cohort cannot fake-signal an
  activation) is tallied against the same >⅔ super-quorum finality uses; the first
  boundary that clears it locks in one-way and enforcement begins at the NEXT boundary
  (`H_act`, chain-derived, replay-identical on every replica, reorg-immune per #357
  finality). Monotonic: a later ready-weight collapse stalls, never forks. The trusted
  pre-latch fleet declares the boundary as genesis config instead
  (`Config.RegGateActivationHeight`). Deliberate deviation from the certification's
  packaging: no v3 block tag is minted — `versionSupported` on pre-gate binaries is an
  exact set, so a v3-tagged block would hard-fork them at decode; enforcement is
  height-keyed (the certification's own Q2 form) and `BlockVersionRegGate` serves as the
  readiness threshold. Stated residual: if the fleet never crosses ⅔ ready weight the
  rule never activates and the #503 interim remains the fallback — there is no safe
  force-activation. Regressions (`core/chain/reggate_506_test.go`, ablation-verified RED
  three ways): the three-clause R-rule at a pre-latch boundary, weight-not-heads lock-in,
  boundary-exact enforcement (storm accepted at `H_act`, refused at `H_act`+1),
  signature-binding of the signal, prune survival, replay-derived `H_act`, monotonicity.
  Build record + deviations:
  [docs/thinking/2026-08-22-506-reg-gate-build.md](docs/thinking/2026-08-22-506-reg-gate-build.md).
- **Chunk write-path debug narration (`chunk stored` / `chunk pulled` / `place attempt`) — the
  #497 records-vs-bytes attribution instrument** (2026-08-21) — Every path that writes chunk
  bytes into a node's store now names itself at `-log debug`: the `MsgStoreChunk` receiver logs
  `chunk stored` (chunk, sender, placement key, lease), a fetch-pull logs `chunk pulled` (chunk,
  provider — these copies mint NO provider record, the exact records-vs-bytes signature), and the
  placement client logs each `place attempt` outcome (a delivered-but-unacked store would be a
  silent extra copy; `SILT_SWARM_DEBUG=1` narrates the `swarm add`/`get` ephemeral client to
  stderr). With it a disk census is attributable line-for-line to its writers — the instrument
  that attributed #497 in one LOCAL run: the publish is clean (30 files for 30 acks,
  records==bytes) and the "extra copies" are the repair sweep's transient survivor fetches plus
  retained NetGet pulls, none of which announce. The economy drill's premise is fixed on the
  same evidence: the pay window now covers the MEASURED repair cycle (sweep duration under dead
  holders is ~3-4 min — `-repair-interval 2s` bounds only the idle gap between sweeps), sized by
  `ECONOMY_REPAIR_WINDOW_S` (default 600s) with one journal-driven grace extension
  (`ECONOMY_REPAIR_GRACE_S`, default 300s) when the cycle is visibly in flight at expiry, and
  every verdict carries the post-kill sweep/repair evidence — a premise defeat (dead shards
  still "reachable") is now named as such instead of GAPing as a timing miss. Trail:
  [docs/thinking/2026-08-21-497-records-vs-bytes-attribution.md](docs/thinking/2026-08-21-497-records-vs-bytes-attribution.md).
- **Cloudtest harness: contained equivocation island (runs every sheet), seeded flow
  randomization, LOCAL/cloud state separation, empty-response honesty, idempotent economy retry**
  (2026-08-20) — A batch of harness-trust fixes, several provoked by a real self-inflicted incident
  this session (a LOCAL verification clobbered a live cloud run's shared `nodes.json` — see below).
  Design: [docs/thinking/2026-08-20-equivocation-island-design.md](docs/thinking/2026-08-20-equivocation-island-design.md)
  + the local-first doc.
  - **Equivocation island (`flow_equivocation_island`, `184-equivocation-island`):** the one
    destructive drill (a proven double-sign is a permanent F2 eviction) now runs on EVERY sheet, in
    a fully-contained separate consensus universe — 4 island anchors naming only each other, own
    genesis, NO external IP (Cloud NAT egress → zero `IN_USE_ADDRESSES` quota). Its slash consumes
    only the island's fault tolerance, never the main sheet's (the PE 2026-08-17 zero-FT-tail
    objection made structurally impossible), so it closes the skip-is-a-blind-spot gap the ruling
    left. LOCAL-verified green (real slash on the wire, height 1); terraform validated.
  - **Seeded flow randomization (`RANDOMIZE=1` default, `SEED=` to replay):** the order-independent
    flows run in a seeded-shuffled order so no flow can free-ride on state a fixed predecessor left
    behind (the hidden coupling that shared `FT_LAST_LINK` hid). `takedown` and `restart-content`
    now self-publish (like `chaos`/`durability` already did), so they're truly order-independent.
    Fixed points pinned: warm-up first, destructive flows (soak/maturing) last. First fully-green
    LOCAL sheet (20/0/0) ran on a random order that placed `takedown` and `restart-content` BEFORE
    `publish-fetch` — the proof.
  - **LOCAL/cloud state-file separation:** LOCAL writes `nodes.local.json`/`topology.local.json`
    (never the cloud's `nodes.json`/`topology.json`), so a LOCAL run can NEVER corrupt a live cloud
    run's node map. Verified: a LOCAL sheet leaves a planted cloud sentinel untouched. This is the
    root-cause fix for the incident where LOCAL island verifications overwrote a running cloud
    sheet's map, breaking its `ssh_node` on `zone=local` and masking the real verdicts.
  - **Empty-response honesty:** an `ssh_node` that returns NOTHING (node unreachable / wrong map) is
    now flagged LOUDLY as a PLUMBING failure and scored GAP, in both `ft_publish` and the economy
    flow — instead of an empty parenthetical that read as benign latency (which masked the clobber
    for a whole cloud run).
  - **Idempotent economy retry:** the economy setup publish generates its payload ONCE before the
    retry loop (mirroring `ft_publish`), so a retry re-publishes the SAME root and picks up an entry
    that committed server-side after the client's fixed 10s registry timeout gave up (#441). The
    fixed 10s `httpregistry` client timeout itself is an owned product finding (build-immutable #5),
    proposed not shipped — see the thinking doc.
  LOCAL_PROOF parity is linted; the sever race and chaos premise-grading are fixed; re-drive
  loop (TEARDOWN=0 / FLOWS=) + dual commit stamp; nightly netem CI** (2026-08-20) — The
  local-first package (design + attributions:
  [docs/thinking/2026-08-20-harness-local-first.md](docs/thinking/2026-08-20-harness-local-first.md)).
  Motivation: 20 archived cloud runs, zero fully green, with the recurring red concentrated in
  harness-quality rows — and the 1,600-line graded drive logic never executed anywhere but against
  billable VMs.
  - **`LOCAL=1` backend:** one container per topology node (shims provide the
    systemctl/journalctl/sudo surface; binary + shims COPIED, never bind-mounted — a host edit of a
    mounted shim tears the container's view, which LOCAL's own first run caught), same static IPs on
    a docker bridge, `ssh_node → docker exec`. The SAME scenarios.sh executes: first full local
    SMOKE sheet graded **10 pass / 1 gap / 0 fail in ~8 min for $0**, RSS telemetry included.
  - **184-partition sever race attributed + fixed:** the sever was already correctly widened to all
    validator-role peers; the surviving GAP class was the BASELINE being read before the sever
    relaunch lands (seconds) on a chain committing drain blocks continuously — val-c "advanced
    during the partition" by committing in the unsevered window (run 2323b09: h27→h29). The flow now
    confirms the post-restart PARTITION banner, then baselines.
  - **chaos-fetch / durability-turnover premise classifier (roadmap 2a):** a fetch failing with
    `root not in registry` means the publish premise broke upstream (#441-family) — now GAP
    (UNTESTED), never FAIL; real mismatches still FAIL. Run B's two chaos FAILs were this shape.
  - **Per-flow `# LOCAL_PROOF:` annotations + `check_local_proofs.sh` in CI:** every graded flow
    names the local test that proves the same property, or an explicit `n/a — <WAN-only reason>`;
    the n/a set (2 flows) IS the owned cloud-only residue. Extends the #490 per-run gate per-flow.
  - **Re-drive loop:** `TEARDOWN=0` keeps the fleet standing; `FLOWS="…" ./cloudtest.sh run`
    re-runs a named subset; reports stamp product AND harness commits so a harness-only re-drive is
    attributable. Convergence aid only — a grade stays one clean uninterrupted sheet.
  - **Nightly netem workflow** (`.github/workflows/nightly-netem.yml`): the adversarial + flakynet
    tiers get a standing gate — merge CI is clean-network and the GCP fabric is cleaner than the
    adverse internet these suites inject (build-immutable #5).
  - **The economy wire grade now covers S7's FULL sentence — prepay → SKIM → bounty + the `g`
    sample** (owner-directed, same day): flow `11b-economy-skim` arms one shard-holder as a
    zero-prepay caretaker (the skim lands on the SERVING holder's per-node ledger, and the UI
    surfaces only cared roots), drives fetches that must route through it (replication 1 → sole
    holder of its column), and asserts `funded > 0` — pure skim, unmistakable. **First wire PASS:
    funded=98310 with zero prepay** (previously the skim leg was sim-only). Flow
    `11c-economy-horizon` records the payer's reserve/horizonSec/cost-per-repair as an
    observational row per graded run — the S7 `g` instrumentation trail ("the one number to
    instrument"), a series no single run can grade.
  - **`e2e/anchorstop_test.go` (TestAnchorStopHaltsBondedNonAnchors)** — the local twin of the cloud
    5-sybil-no-capture flow: 3 anchors + 2 bonded non-anchor validators; baseline commits; ALL
    anchors killed → the bonded survivors commit nothing (the launch anchor gate, #402); anchors
    restart → the chain resumes. Green in ~60 s. (Its own failing first cut re-proved the #402
    arithmetic: with A=2, `-quorum 2` leaves one counting non-proposer attester and the baseline
    can never commit.) The maturing-latch e2e twin is the named residual (needs a >⅔-weight
    maturer regime design — docs/thinking/2026-08-20-harness-local-first.md).
- **The LOCAL proof of the full S7 economy loop (`e2e/economy_repair_test.go`) + `-repair-interval`**
  (2026-08-20) — `TestRepairBountyPaysOnTheWire` runs the whole Phase 2 Slice 4 integration on real
  daemons over real TCP: publish erasure-coded (one k=10/n=16 stripe at the cloud's 256 KiB chunk) →
  `swarm holders` → kill the holders of 3 columns (> `RepairSlack`) → a caretaker reconstructs from
  parity → a peer caretaker-judge verifies both legs and the bounty draws the object's escrow
  (`paid > 0`) → the file still fetches bit-perfect. This is the enforced `RUN_LOCAL_PROOF` for the
  confirming `ECONOMY=1` cloud run (the integration run 2323b09-20931 GAPed on was never proven
  locally — build-immutable #7). Building it found **three latent cloud-scenario defects** that would
  have GAPed the re-run even with the publish fix (#489): the scenario armed ONE caretaker, but the
  paramedic never judges its own claim and credit is per-node-local, so `paid` lands on the OTHER
  caretaker's ledger; it funded 2,000,000 against the 500,000 starter grant (`FundEscrow` refuses);
  and its relaunched caretaker had no `-registry`, which silently disabled the care loop entirely.
  `flow_economy_repair` now arms two caretakers (the judge is the relay — outside the killable role
  set), funds both within grant, and polls both. New daemon flag **`-repair-interval`** (default 60 s,
  unchanged) mirrors `-bond-audit` so a local swarm's repair sweep — and this proof — fires in
  seconds. Design + the run-1 attribution honesty note + the one-shot-claim residual:
  [docs/thinking/2026-08-20-economy-local-loop-design.md](docs/thinking/2026-08-20-economy-local-loop-design.md).

### Fixed
- **#528 — the h≈56 liveness knee: catch-up sync validates only the new suffix, never
  the whole chain** (2026-08-23) — The RC run `0de4b96-64567` wedged at h57: every
  catch-up reconcile re-validated the ENTIRE chain from genesis in a throwaway replica
  (~1s per 1.5 MB reg block, on the event loop), so at accumulated MATURING reg weight
  one reconcile outlasted the round durations, starved sweep and round-change
  processing, and h57 never committed (198 `ChainReply` watchdog HANGs; deterministic
  2/2 with run `94ef1e8-36901`). Fix: a served window that provably EXTENDS the local
  committed head (finality active; the first new block's `Prev` chains from our head
  hash, which transitively commits our entire already-validated history) is adopted
  per window through the normal `Append` commit path — O(delta) validation, loop
  occupancy bounded by the #466 window byte budget — instead of `reconstructFork` +
  `Reconcile`'s O(height) genesis replay. Every other shape (divergent fork,
  equal-height fork, legacy no-finality config, pruned gap) keeps the unchanged slow
  path with all reorg / equivocation-scan / finality-gate / `ErrNeedCheckpoint`
  guarantees. No consensus rule changes: adoption without a `heavier()` pass is sound
  exactly because the finality gate makes an extension the only adoptable shape and
  each appended block re-proves a super-quorum commit. Measured locally: near-head
  catch-up on a 60-block heavy-reg chain fell from 280 ms (full replay) to 4.7 ms
  (suffix append), 60×; at field bond-verify cost that is the 40–60 s loop pin
  removed. New cost gauges `ChainSyncSuffixAppends` / `ChainSyncFullReconciles` split
  the two routes; deliberation in
  [docs/thinking/2026-08-23-528-suffix-append-catchup.md](docs/thinking/2026-08-23-528-suffix-append-catchup.md).
- **Cloudtest harness: the #525 trio — a false-wedge fingerprint read, base-topology
  bounds graded onto a 12-seat rotation, and the re-drive clobbering the sheet**
  (2026-08-22, all three evidenced by coverage run 94ef1e8-36901) — (1) The #509 escape
  fingerprint is now TIME-SCOPED (`jlog_since`, `journalctl --since @kill-t0`), never a
  last-600-line window: economy sweep narration scrolled the survivors' round-change
  lines out of the window, so both samples read rc=0 ("frozen") while the journals
  showed the ladder advancing h38 r1→r3 — a manufactured WEDGE FAIL (the run's true
  verdict was #509's out-of-model GAP). Same unscoped-read class audit #303 closed for
  matches. (2) The 6-fault-tolerance tiers and the maturing-handoff drive bound are
  TOPOLOGY-AWARE: the certified 260s/575s and 220s-per-height figures price the 4-seat
  base rotation, but pre-epoch the (h+r) mod N designee rotation spans every bonded
  seat. Policy: one extra #451-priced escape rung (dur(r) = 2 + r(r+1)/2 sweeps × 30s)
  per 4 rotation seats beyond the base, added to the base constants — an N=4 sheet
  computes exactly the certified figures; the 12-seat MATURING sheet computes 650s/1445s
  and 610s/height (the run missed h57 by ONE block inside 9×220s while the latch itself
  tripped). The handoff drive also exits early once the ceiling freezes for a full
  per-height bound, so a real stall grades without burning the whole window. (3)
  `./cloudtest.sh run` no longer truncates `results.jsonl`: `run` is the documented
  re-drive entry, and the truncation destroyed every previously graded verdict (the
  run's archived sheet held only the 3-row re-drive pass; the 14 first-pass verdicts
  survived only in the console log). The sheet now clears where a NEW sheet begins
  (`all` and `up`); a re-drive appends, and the report shows each pass's verdict.
  Rider: the `PERSIST_NET` marker `.persist_net` is git-ignored.
- **Chainless registry lookups no longer block the event loop (#473) — the async pass**
  (2026-08-22) — The remaining face of the concurrent-publish 502 class: on a chainless
  node (client mode, or a daemon on a remote registry) six loop-driven sweeps — `Care`,
  `repairRoot`, `netGet`, `Audit`, repair-claim judging, `ColumnHolders` — called
  `ports.Registry.Lookup` inline on the event loop, and against `httpregistry` that is a
  blocking HTTP round-trip holding the node's single thread for up to the HTTP timeout,
  per call, per sweep. New optional capability `ports.AsyncRegistry`: `httpregistry`
  runs the round-trip on its own goroutine and the node marshals the continuation back
  through the loop (`AfterFunc(0)` — walltime posts, sim enqueues); in-memory registries
  keep their sync `Lookup` via a fallback that still defers completion (the #467
  contract: a continuation never runs on the stack that initiated it, error paths
  included). Regressions (`core/node/registry_async_473_test.go`, ablation-verified
  RED): a chainless `Audit` against an async-capable registry makes zero blocking
  lookups, the loop stays live while the round-trip is in flight (a timer fires
  mid-lookup), and the sync fallback defers.
- **Cloudtest harness: persistent VPC, canonical region octets, and the preflight
  counts the natgw's address** (2026-08-22, harness-hardening items from the 2026-08-19
  audit) — (1) `./cloudtest.sh net-up` creates a long-lived network
  (`terraform/network`: VPC, canonical subnets, firewalls, Cloud NAT — own state, $0
  idle) and `PERSIST_NET=1` runs attach to it instead of creating and destroying a
  per-run VPC, saving those minutes every run; the run's `destroy` never touches it.
  (2) topology.py's region→octet assignment is now CANONICAL — a function of the region
  alone, never of which regions a topology subset uses (the old subset-relative
  numbering gave us-east1 octet 21 in a SMOKE but 22 in a full sheet — fatal for
  persistent subnets; the full-sheet assignment is byte-identical). (3) The IP-quota
  preflight now counts the natgw — its interface holds the masquerade external IP but
  the counter skipped it, undercounting its region by one; at the full
  SYBILS+MATURING+ECONOMY sheet's exact 8/8 us-west1 fit, that hidden margin was the
  whole margin. Verified with generated topologies against live quotas: the full
  coverage sheet fits every region's default 8-IP quota as-is (us-west1 8, europe-west1
  6, us-east1 5) — no quota increase needed.

- **Chain serve is windowed — a validator no longer marshals its whole chain into one
  buffer to answer `MsgGetChain` (#466)** (2026-08-22) — The serve-side OOM driver
  measured on the 2 GB box (`chain.EncodeBlocks` marshaling the full bond-reg-laden
  suffix: 144 MB live / 98 MB retained / ~310 MB per encode at cloud heights) is closed:
  the server now replies with the longest block prefix that fits `maxChainReplyBytes`
  (`EncodeBlocksUpTo`, always ≥ 1 block so an oversized block still moves), and the
  requester (`SyncChain`'s `fetchFull`) loops windows — advancing from each window's last
  decoded height, releasing each raw reply buffer after decode, terminating on the head
  probe's height (or the first empty window against a pre-window peer) — then reconciles
  the reassembled suffix through the unchanged validation tail, so a spliced or truncated
  window fails closed exactly as before. Two rulings from the PE approach review
  (RULING-466, 2026-08-22) shaped the shipped form: the window is **derived, not a
  literal** — `RequestSizeFloorBytesPerSec × requestSizeExtensionCap × ½` = 3.75 MiB at
  daemon defaults — and the requester arms a **reply-sized deadline** for `MsgGetChain`
  (the 30 s size extension keyed off the outbound payload, so a windowed reply was
  otherwise bounded by the base 2 s `RequestTimeout`, which no window near the floor can
  meet). Rollout is atomic with no capability negotiation: an old requester against a
  windowed server converges sweep-by-sweep (head-match termination makes silent
  under-sync structurally impossible; measured in the mixed-fleet drill), and a new
  requester accepts an old server's whole-suffix reply. Regressions:
  `core/node/chainsync_window_466_test.go` (deadline coupling, window derivation, serve
  bound, multi-window convergence, both mixed-fleet directions, splice fail-closed —
  ablation-verified RED). Requester-side chain retention is explicitly NOT closed by this
  (the #299/pruning axis). Design + ruling trail:
  [docs/thinking/2026-08-18-paginate-chain-sync-design.md](docs/thinking/2026-08-18-paginate-chain-sync-design.md).
- **The #467 recursion audit — five sibling continuation chains bounded, closing the PE's
  audit extension** (2026-08-22) — The flixz stack-overflow crash itself was fixed by #471's
  walk-terminal trampoline, but the ruled follow-up scan ("no unbounded recursion / no
  re-entrant cycle between subsystems") had never run. Run now, it found five chains that
  still advance INLINE when a fast path completes synchronously: the `repairStripes`
  healthy-stripe walk (O(stripes) frames + an O(stripes × refs) rescan monopolizing the
  loop each sweep), `FetchChunk`'s already-held fast path and `fetchFrom`'s no-provider
  exit (fetch chains recurse O(ids) over fully-held or unsettled lists), `repairTick`'s
  root walk over synchronously-skipped roots, `distribute`'s dedup member skip, and —
  the class enabler — `request` running its callback inline on a synchronous send
  failure, which re-armed every "safe because it crosses a request" chain against a dead
  transport. All six sites now post their continuation through the loop (`AfterFunc(0)`,
  the #471 contract: completion never runs on the stack that initiated it), and the
  stripe walk groups refs by stripe once, taking a large file's sweep CPU from
  O(stripes × refs) to O(refs). Failing-first regressions:
  `core/node/recursion_audit_test.go` (all five RED before the fix). Audit record, with
  the bounded-chain inventory so the next audit doesn't re-derive it:
  [docs/thinking/2026-08-22-467-recursion-audit.md](docs/thinking/2026-08-22-467-recursion-audit.md).
- **`TestMeasure_StoreChunkDrainRate` no longer races itself (#507) — local `-race ./core/node/`
  runs clean with no skip** (2026-08-22) — The measurement's ack counter was written by the
  tcpnet readLoop callback and read by the test body unsynchronized; every local race run
  needed `-skip TestMeasure_StoreChunkDrainRate`, masking real signal for consensus-touching
  changes (build-immutable #2). Now `atomic.Int64`; the full `core/node` race suite passes
  unskipped, and the E5 measurement still reads clean (268 MB/s, cap/drain 0.955s under the
  2.0s bound — consistent with the shelve verdict).
- **The cloudtest fault-tolerance bound models escapes that START under load (#509) — two
  computed tiers with a progress-graded extension, and the wedge signature is now a FAIL**
  (2026-08-22) — Seed `f35a0f9-76780` GAPped `6-fault-tolerance` when a healthy, advancing
  round escape (already at r1 pre-kill, sweeps stretched by the economy triple) outran the
  flat 260s bound, which models a 2-round escape from idle. The bound is now two-tier, both
  from the #451 arithmetic: the 260s expected tier stays the first check; a miss extends —
  while the survivors' escape fingerprint (round-change count + max height) demonstrably
  advances — to the r≤3 hard cap (dur(0..3)=18 sweeps ≈ 575s). Grades sharpen both ways: a
  commit inside the cap PASSES with the slow escape narrated; a FROZEN fingerprint across
  the extension is the wedge signature and now FAILS (previously an unattributable GAP);
  only advancing-but-uncommitted-at-cap remains a GAP, marked out-of-model. The knob was
  not raised — the model was completed (build-process rule 7).
- **A repair bounty can no longer starve for want of a judge — rebuilt shards prefer
  holders outside the caretaker-judge quorum, and every verdict is evidence-carrying
  (#518)** (2026-08-22) — A repair claim excludes both the paramedic and the named holder
  from judging, so a two-caretaker deployment whose rebuilt shard landed ON the other
  caretaker had ZERO eligible judges: the claim died silently and the bounty never paid
  (captured with the #519 narration: all four of a repair's claims naming the other
  caretaker as holder, `quorum=2` — the last e2e flake mode, made likelier by #517
  synchronizing the two caretakers' confirming sweeps). Placement now stably prefers
  civilian holders (`preferNonJudges` — preference, never veto: a shard on a judge still
  beats a shard nowhere; the self-hold path is exempt since claimant==holder is excluded
  once and the other judges still judge). A second captured sub-mode is fixed with it:
  a claim arriving moments after the repair-time fetch storm found the judge's survivor
  fetches transiently short (`survivors fetched=2..5 of k=10`, live-but-slow holders
  freshly negative-cached) and was denied TERMINALLY — emission is one-shot, so a 30s
  condition silently cost the bounty forever. The judge now DEFERS a transiently
  unjudgeable claim and re-judges after `HolderCooldown` (the duration of the very
  transient being waited out), bounded at 3 attempts, denying with the reason only when
  they exhaust. The verdict path also narrates end to end: every `repair claim denied`
  carries a `reason=`, deferrals name themselves, the holder retrievability challenge
  logs its outcome, a release that pays nothing warns `escrow empty on this judge`, and
  a verified-but-not-released verdict names itself. The sim bounty test's shared-ledger endowment is recalibrated:
  with starvation fixed every judge settles claims that previously died, and on the rig's
  one-shared-ledger wiring that double-draws the escrow the 5M prepay sat knife-edge at
  storm exhaustion (production per-node ledgers each pay once, by design). Regressions:
  `core/node/prefer_nonjudges_518_test.go` + `core/node/judge_defer_518_test.go` (a
  staged transient — survivors down at claim time, revived mid-retry-schedule — must
  end in a paid bounty; RED with the defer disabled); the capture arc rides
  [docs/thinking/2026-08-22-517-repair-confirmation-gate.md](docs/thinking/2026-08-22-517-repair-confirmation-gate.md).
- **The repair trigger is minimum-filtered — one noisy probe sample can no longer fire a
  false repair (#517; roots the #514 e2e flake)** (2026-08-22) — A caretaker's FIRST sweep
  after arming races record propagation: the captured #514 run read three never-lost shards
  as missing (`reachable=18` while every shard had a live holder), "repaired" them from
  parity, and placed the rebuilds at the daemon's replication 3 — persistent, record-backed
  duplicates nobody paid to place (a third source of the #497 extra-copies census, after
  #500/#502 closed the other two), which then defeated the e2e kill-selector's premise: the
  "doomed" columns survived their holder's death, the caretakers correctly watched
  `missing ≤ slack`, and no bounty could ever pay (#514, ~2/10 under load). Per
  `network-durability.md` §3 (*minimum-filter a noisy signal — never trust one sample*) the
  repair and dispersion-re-spread triggers now require the over-slack observation to persist
  across TWO consecutive sweeps (a clean sweep resets; a firing that fails below-k retries
  every sweep without re-confirming), narrated as `stripe repair pending confirmation`. Costs
  one repair interval on a true loss. `probeShard` gains `shard confirmed by=` debug
  narration so every reachable verdict names its confirmer, `emitRepairClaim` warns
  `repair claim found no eligible judge` when a claim has nowhere to go (the newly-filed
  #518 judge-starvation corner: a 2-caretaker quorum whose rebuilt shard landed on the other
  caretaker), and the e2e gains a premise fast-fail (no over-slack observation within 60s of
  the kill → loud premise-defeat failure, not a silent 180s timeout). Regressions:
  `core/node/repair_confirm_517_test.go`.
  Capture story + attribution: [docs/thinking/2026-08-22-517-repair-confirmation-gate.md](docs/thinking/2026-08-22-517-repair-confirmation-gate.md).
- **A restart mid-repair no longer orphans the survivor working set — Care reconciles it at
  boot (#502)** (2026-08-22) — A repairing caretaker (or judging caretaker-judge) pulls up to
  k×stripes survivor chunks and drops them only in the post-reconstruction cleanup
  continuation; a restart in that window (operator, crash, or the harness's
  `relaunch_with`/`econ_restore`) killed the chain and nothing at boot reconciled — the pulls
  sat in the store forever: record-less bytes counting against the pledge and read as local by
  the next sweep's `includeLocal` probe (the plausible source of the persistent ~3× disk
  census on re-driven fleets that motivated #497). Care's warm-start continuation now runs
  `reconcileWorkingSet`: a LEAF of the cared root, present in the store, with no proof in the
  persisted backing is definitively an orphan (every legitimate leaf holding carries a
  persisted proof — `MsgStoreChunk` refuses proof-less shards, the repair self-hold and
  `NetGetRetain` mint one, and plain `NetGet` drops its working set — which is why #502 was
  sequenced after #500) and is dropped, narrated as `repair working set reconciled`.
  Warm-start manifest copies (held bare by design) are exempt; legacy proof-less NetGet
  leftovers from pre-#500 fleets are cleaned by the same sweep at their next boot. Regression
  with a REAL injected crash — an actual sweep stepped to mid-window between fetch and drop,
  then a fresh Node incarnation booted on the same chunk+proof stores:
  `core/node/repair_orphan_502_test.go`. Deliberation:
  [docs/thinking/2026-08-22-502-working-set-boot-reconciliation.md](docs/thinking/2026-08-22-502-working-set-boot-reconciliation.md).
- **NetGet's pulls are an explicit working set, and the UI consumer==provider promise is
  wired (#500) — a fetch either drops what it pulled or retains it as REAL, discoverable,
  audit-answerable hosting** (2026-08-22) — `fetchFrom` writes bytes with no provider record
  (deliberate), and two callers retained what they pulled: `NetGet` kept the whole object
  forever — undiscoverable bytes counting against the capacity pledge (the #497
  records-vs-bytes divergence; 55 `chunk pulled` vs 30 `chunk stored` in one economy drive,
  none discoverable) — and the UI `/api/fetch` consumer==provider path retained on purpose but
  never announced, so no fetcher could ever find the "provider". Now: **`NetGet` drops its
  working set** after assembly, success or failure (the repair-path paramedic discipline;
  chunks the node already hosted are never touched), and **`NetGetRetain`** converts the pulls
  into full hosting — each shard's StorageProof + PoR tags are minted from the manifest tree
  and the link's layout key (the retainer can defend an audit exactly like a `MsgStoreChunk`
  recipient — never host what a later audit can't defend), registered under its placement key
  via the existing repair self-hold primitive, and ANNOUNCED to the nodes near the key.
  Retained copies then ride the normal reprovide lifecycle. The UI fetch uses retain: a
  daemon that consumes a link comes out the other side visible in `swarm holders` and able to
  serve the object after every original holder dies (both asserted:
  `core/node/netget_retention_500_test.go` sim tier incl. serve-after-holder-death,
  `e2e/netget_retention_500_test.go` on real daemons over TCP). `swarm get` and the netcheck
  self-test keep the drop default. `includeLocal` durability semantics are now honest: the
  only record-less local copies left are in-flight working sets. Deliberation:
  [docs/thinking/2026-08-22-500-netget-retention-semantics.md](docs/thinking/2026-08-22-500-netget-retention-semantics.md).
- **The repair sweep is BOUNDED under dead holders (#501) — sweep-scoped corpse gating + a
  decaying dead-peer cooldown; the measured 3–4 min sweep drops to ~72s worst-case first
  discovery and the wire repair cycle to ~28s** (2026-08-22) — A sweep's discovery walks paid a
  full retry ladder (RequestTimeout × 4 attempts ≈ 22s at daemon defaults) to every freshly dead
  peer, per phase, re-paying it mid-sweep each time the flat 30s `HolderCooldown` lapsed — a
  deterministic sim reproduction (daemon-faithful transport, field-faithful replication-1
  placement) measured 159s for the first post-kill sweep and a 45s full-re-discovery sweep
  recurring every ~30s FOREVER, even on a fully healed object (corpses re-enter lookups via other
  peers' `FindNode` replies). Two cache-scope changes, no transport deadline / retry / eviction
  semantics touched: (1) a peer whose ladder exhausts anywhere in the current repair tick is
  skipped by every gated leg (walk, probe, fetch, announce) for the tick's remainder — one sweep
  pays at most one discovery ladder per corpse; (2) each successive exhaustion doubles the
  corpse's cooldown (30s → 60 → 120 → 240 → capped 480s, under the reprovide period), so the
  recurring re-discovery tax decays geometrically — any inbound message still clears the entry
  instantly (proof of life, #69), and the sole-candidate guards keep a lone holder probeable.
  Sweeps now narrate their phase timings (`repair sweep complete … manifest-heal-ms probe-ms`,
  `repair pass complete … repair-ms total-ms`), making any future slow sweep self-attributing in
  a run journal. The e2e bounty window re-tightens 600s → 180s (the fixed cycle measures ~28s on
  the wire; the deadline is the #501 regression signal again). Regression + measurement:
  `core/node/repair_sweep_duration_501_test.go`; deliberation + measured tables:
  [docs/thinking/2026-08-22-501-sweep-duration-bound.md](docs/thinking/2026-08-22-501-sweep-duration-bound.md).
- **An F2-evicted identity no longer re-registers forever — the bond-renewal storm behind the
  island OOM is suppressed at every honest layer (#503, Q1 of the research certification)**
  (2026-08-21) — A slash deletes `bonded[id]`, which made `BondRenewalDue` read true FOREVER for
  the evicted identity: its daemon re-broadcast the full ~1.5 MB space-time proof every ~30 s
  sweep, no layer consulted `slashed`, and honest proposers committed the banned identity's
  registration as a fresh block each time, unbounded — ~35 MB/min of chain growth on the cloud
  sheet's equivocation island until the 2 GB box OOM'd (build-immutable #8; the fa501cc-56689
  sheet's one FAIL). Certified fix, zero block-validity change (a mixed-version swarm cannot
  fork on it): the client backs off permanently once it observes its own slash and logs
  "permanently evicted" once instead of silently retrying (B5); a receiver refuses a slashed
  identity's submitted reg at arrival, before decode, by a map lookup; and the proposer's fold
  re-filters the queue so a reg that raced in before the slash landed is dropped as policy, not
  validity. The honest renewal loop then decays to quiescence on its own (~0.19 renewals per
  block-of-aging < 1, certified) — the TTL's height denomination and all four coupled security
  parameters (WS period, safetyDepth = 2·TTL, EpochBlocks ≪ TTL, BondRegHeadWindow ≪ TTL) are
  deliberately untouched (Q2 certified contraindicated). The structural close against an
  adversarial re-registrant — a per-identity reg-inclusion VALIDITY rule — is #506
  (version-gated). Certification:
  `silt-reviews/research/research-outcome/503-bond-renewal-storm-RESEARCH-CERTIFICATION-2026-08-21.md`;
  deliberation: [docs/thinking/2026-08-21-503-q1-fix-deliberation.md](docs/thinking/2026-08-21-503-q1-fix-deliberation.md).
- **A prepare-only equivocator is now selected for slashing — equivocation candidate-selection
  enumerates every signing role the verifier checks (#496)** (2026-08-21) — `signers()`, which feeds
  `FindEquivocations` its candidate set, read proposer + `Atts` (precommit) but never `PrepareQC`
  (prepare), while `VerifyEquivocation` scans both. An era-2 double-signer whose signature in the
  honest canonical block sat only in the prepare certificate — the objective-mode equivocator at the
  genesis child, where the culprit is reliably prepare-only — was therefore never even tested by the
  verifier that would have convicted it: a real I5-accountability hole an adversary could aim at any
  height by choosing to be prepare-only. Found by the randomized field sheet (a height-1 double-sign
  went unslashed for a whole drill window while height 2 was always caught, run 1642465-57233);
  mechanism research-certified by execution before the fix (candidate-selection asymmetry — not a
  height floor, not delivery, not fork-choice; widening selection cannot manufacture a false slash
  because `VerifyEquivocation` remains the gate with its honest exemptions intact). Invariants: I5
  strengthened, I1–I4 untouched (no commit/quorum rule changes — local detection only). Regression
  pinned born-RED by `core/chain/TestFindEquivocations_PrepareOnlyCulprit`.
- **`-care` with no registry now refuses to start instead of silently never caretaking**
  (2026-08-20) — The care loop requires a registry to resolve the cared entry; without `-registry` or
  `-serve-registry` the daemon came up looking healthy while `-care` did nothing (the #235
  silent-skip shape, one layer up — and exactly the no-op caretaker the cloud economy scenario
  armed in run 2323b09-20931). Regression pinned by `e2e/TestCareWithoutRegistryRefusesToStart`.

### Added
- **`silt swarm holders <link>`: object shard placement made observable** (2026-08-19) — Prints, per
  erasure column, the NodeIDs that claim to hold that column's shards (their DHT provider records
  under `colKey`). An operator uses it to see *where* an object lives; a test harness uses it to force
  a *controlled* reconstruction — killing every holder of more than `RepairSlack` columns drops that
  many shards from every stripe, so the caretaker must rebuild from parity (the deterministic trigger
  the cloud economy grade needs, which the sim had via `KillColumns` but the cloud lacked). New
  read-only node accessor `ColumnHolders` (resolves each column's providers via the same DHT walk
  `NetGet` uses; plants and stores nothing); uncoded objects report their per-chunk holders under
  `uncoded`. Tested over real TCP (`e2e/holders_test.go` — 16 columns, 48 holder entries resolved).
- **§0.1 repair-path memory footprint measured locally (`core/erasure/reconstruct_mem_test.go`)**
  (2026-08-19) — The research cert's §0.1 gate ("measure repair RAM at production chunk size before
  the economy-ON grade") is a single-node property, so it is measured locally for $0 rather than with
  a billable cloud run (build-immutables #6/#7: reproduce locally first; the cloud harness publishes
  at the 64 KiB sim size and would hide the spike ~1000× anyway). Result: reconstructing one
  `DefaultParams` (k=10, n=16) stripe holds **1.0 MiB resident at 64 KiB vs 1.0 GiB at the 64 MiB
  production minimum**. On a 2 GB floor box that leaves ~1 GB, and the daemon baseline is 0.5–1.25 GiB
  (measured in field run 6a38d7b-42691) — so **production-scale repair + baseline can exceed 2 GB →
  OOM**. Consequence: the economy-ON field grade (Slice 4) must run on a larger box or land a
  streaming/column-wise decode mitigation first (build-immutable #8). Plan:
  [docs/thinking/2026-08-19-cloudtest-harness-improvement-plan.md](docs/thinking/2026-08-19-cloudtest-harness-improvement-plan.md).
- **`-economy`: the S7 repair-bounty payout enable — the keystone (Phase 2, Slice 1)**
  (2026-08-19) — Turns the half-open economy fully on: an opt-in `-economy` flag
  (**default OFF**) under which a verified repair PAYS for a rebuilt shard from the
  object's own escrow. Per the PE ruling + research certification:
  - **Protocol price, never an operator amount** (an operator-set base is a lottery, not
    a price — it undefines S7's equilibrium and opens a censorship-via-underfunding lever),
    and **relative to the erasure geometry**: `base = c × (k × shardBytes)`
    (`credit.RepairBountyBase`), so re-tuning Evolving-tier erasure params re-prices repair
    automatically. **`c = 1` is research-certified** (decode <0.1% of the fetch cost so no
    upward pressure; `g`-neutral; smallest floor-honest value; self-funds hot data, cold
    stays prepay-dependent per D-S7). Config `RepairBountyBase int64` (absolute, test-only)
    → `RepairEconomy bool`; the settle path threads the repaired shard's byte size and is a
    true no-op when off.
  - **Payee: (a-domain-fresh)** — the paramedic that reconstructs a shard KEEPS it
    (becoming the paid holder) iff its own failure domain is unused by the stripe, funding
    the node that bore the reconstruction cost + the ~640 MiB–1 GiB RAM peak WITHOUT
    reducing failure-domain diversity (`node.selfHoldEligible`/`hostShardLocally`); else it
    places remote exactly as before. Chosen over paying the cheap holder (mis-attributes
    the price) and over unconstrained self-hold (trades away S2 dispersal).
  - **Invariant A holds** — the bounty moves *balance* only, never standing — asserted by
    the failing-first merge gate (`core/node`: release-pays-holder-never-standing,
    **economy-OFF-is-a-true-no-op**, **default-OFF**), plus the S2-safety gate
    (`selfHoldEligible`: self-hold only when the economy is on and the domain is fresh).
  - **Owned residual:** the reconstruction step is unfunded in the non-fresh-domain fraction
    (fork (c) split-pay is the evidence-gated fast-follow); and the repair-path RAM at
    production chunk size must be measured before the economy-ON field grade (build-immutable
    #8 — the 64 KiB sim hides the spike ~1000×). Rulings + cert:
    [docs/thinking/2026-08-19-phase2-economy-on-deliberation.md](docs/thinking/2026-08-19-phase2-economy-on-deliberation.md).
- **`POST /api/fund`: the durability endowment path (Phase 2, Slice 3)** (2026-08-19) —
  A publisher/operator can now prepay an object's repair reserve from the daemon's own
  earned credit balance, so content outlives churn before it is popular enough to
  self-fund via the serve auto-skim. Wires the built-but-uncallable `FundDurability` to a
  token-gated endpoint that accepts a `silt:`/`siltcare:` link **or** a bare root hash plus
  an `amount` (credits), and returns the object's new reserve and the node's remaining
  balance. Status contract: 200 endowed, **402** insufficient credit (a client-correctable
  condition, not a server fault), 400 bad input, 401 without the bearer token. Standing is
  untouched — the credits come from serving and fund durability only (Invariant A). Tests:
  `cmd/silt/fund_test.go` (parse link/hash, endow-debits-balance, 402/400/401). Decision-
  independent of the pending `RepairBountyBase` ruling that gates Slice 1 (the payout
  enable). Deliberation: [docs/thinking/2026-08-19-phase2-economy-on-deliberation.md](docs/thinking/2026-08-19-phase2-economy-on-deliberation.md).
- **Durability telemetry: the S7 repair economy made observable on `/api/status`
  (Phase 2, Slice 2)** (2026-08-19) — The economy runs *half-open* on a live daemon
  today: the serve auto-skim (1/8) already fills each object's durability escrow
  (`node.go` records `RecordServeToObject`), but the funded reserve, lifetime skim/pay,
  the funded horizon, and whether bounties actually **disburse** were invisible
  (`credit.G`/`Horizon` were computed only for a local repair decision, never surfaced).
  `/api/status` now carries a `durability` block: the node's credit **balance** (what
  serving earned), a `bountyOn` flag (whether `RepairBountyBase > 0` so verified repairs
  actually pay — false by default, the half-open state named honestly), and per cared
  object its reserve / lifetime funded / paid / repair-count / projected funded-horizon
  seconds. New node accessors `CreditBalance`, `CaredDurability`, `RepairBountyEnabled`
  (loop-owned, read-only). Standing is never in this block — Invariant A holds (credits
  fund durability, never consensus weight). This is the prerequisite for *watching* `g`
  once the economy is switched on (Slice 1). Tests: `core/node/durability_telemetry_test.go`.
  Deliberation + the full Phase 2 slicing (and the one open decision — is `RepairBountyBase`
  a protocol constant or an operator flag?): [docs/thinking/2026-08-19-phase2-economy-on-deliberation.md](docs/thinking/2026-08-19-phase2-economy-on-deliberation.md).
- **RSS/memory-envelope telemetry in `integration/cloudtest` (Phase 1.3, evidence
  hygiene)** (2026-08-19) — The field harness detected memory only as a binary crash
  signal (`scan_node_liveness` greps journals for OOM-kill / Go-fatal) plus an on-demand
  heap profile; there was **no continuous RSS series**, so the MATURING OOM
  "return-to-2GB" headline rested on the *absence* of a crash, not a *measured* ceiling
  (the fresh-eyes audit's finding — no committed RSS artifact backed the claim). Now every
  run samples each node's cgroup memory (`systemctl … MemoryCurrent` — the exact quantity
  `GOMEMLIMIT` and the OOM-killer act against) every `MEM_SAMPLE_INTERVAL` (default 30 s)
  into a `rss-<RUN_ID>.jsonl`, and `scan_node_memory` records an `infra-node-memory`
  finding with per-node **peak / final** RSS. Strictly additive and failure-tolerant (a
  missed read never affects a verdict); purely observational (S5 — it reports the envelope,
  the crash verdict stays with `infra-node-liveness`). The series is git-ignored by default
  and force-committed for any run cited as evidence, matching the console-log convention.
  Sampler + summary logic unit-verified locally; the artifact itself lands on the next real
  run. Design: [docs/thinking/2026-08-19-cloudtest-rss-telemetry.md](docs/thinking/2026-08-19-cloudtest-rss-telemetry.md).
- **The `MsgSubmitBondReg` CPU gate — per-sender submit budget + sender binding
  (Phase 1.2, the pre-#183 DoS floor)** (2026-08-19) — The bond-renewal submit path
  had no rate limit: every well-formed, self-signed registration forces up to one
  `VerifySpaceTime` on the node's single loop (measured ~2–3 ms valid / ~0.5 ms
  garbage-until-reject at the field config on an M4 core — `core/bond/`
  `verifycost_bench_test.go`, the new sizing benchmark), so one authenticated
  identity holding a pipe could keep the loop at a permanent duty cycle for free —
  the #424 bond-challenge CPU-DoS, one message kind over. Two cheap gates now run
  first: **(a)** `allowBondSubmit` — submits examined per sender per
  `ChainSyncInterval` window (burst 8; honest cadence is one submit per sweep while
  `BondRenewalDue`, so the budget clears honest traffic with wide headroom), charged
  BEFORE decode so a refusal costs a map lookup and a flooder gains no amplification;
  **(b)** sender binding — a submit is always the sender's OWN renewal
  (`SubmitBondRenewal` self-submits), so a reg whose `ValidatorID` differs from the
  authenticated transport sender is refused before any signature/proof work, closing
  the third-party replay hole (queue dedup sat AFTER the expensive verify, so
  replaying one captured valid ~1.5 MB reg re-paid full verification per message).
  Refusals are logged, never silent (B5); a refused honest submit heals by the
  existing resubmit-next-sweep retry, the same recovery as a WAN-skew refusal.
  Failing-first regressions: `core/node/bondsubmit_gate_test.go` (flood bounded to
  the budget, second sender not starved, budget refills on window turnover,
  third-party relay refused pre-crypto). Deliberation:
  [docs/thinking/2026-08-19-bondreg-submit-cpu-gate.md](docs/thinking/2026-08-19-bondreg-submit-cpu-gate.md)
  — which also records the verify-cost measurement correcting E5's drain folklore
  (the ~100 ms figure was the *prover*; the *verify* is ms-scale by design), making
  the E5 drain-rate measurement rider the next step.

### Changed
- **`-inbound-cap` sizing made two-axis legible; the v2b consensus-priority lane
  sequenced behind the bond-reg CPU gate as owned residual E5** (2026-08-19) — A
  PE-mandated timed drill (parked RED on `drill/v2b-gate-starvation`) showed that under
  a within-share sybil-cohort bulk flood, consensus-frame starvation lives in the single
  loop's FIFO **drain** (latency ≈ cap/drain, measured within 1% of the analytic
  prediction), not in gate admission — so the planned admission-side reserve alone is
  insufficient, and the severe regime is exactly the bond-reg/VDF slow-drain that the
  Phase 1.2 CPU gate bounds. Per the PE drain ruling the lane is sequenced, not shelved:
  1.2 first, then a measured-drain re-run of the drill as the go/no-go for a single
  two-class priority-drain mechanism (with a bounded-priority second oracle so bulk/repair
  never starves — I4's storage-plane face). Recorded as owned residual **E5**
  (`docs/design/owned-residuals.md`) with the reach-recipe; the `-inbound-cap` flag help
  now states the two-axis trade (OOM headroom vs worst-case ~cap/drain latency — a
  saturated 256M draining at 2 MiB/s is ~128s) so operators size for their expected-worst
  drain. Drill design + verdict:
  [docs/thinking/2026-08-19-v2b-gate-starvation-drill-design.md](docs/thinking/2026-08-19-v2b-gate-starvation-drill-design.md).
  **Resolved 2026-08-19 (the E5 drain rider):** the measured real single-loop drain for
  the cheapest bulk a flood rides (MsgStoreChunk, real hash-verify + store handler over a
  real TLS transport — `core/node/draindrate_measure_test.go`) is **~1227 MB/s on an M4
  core**, ~600× the drill's hypothetical 2 MiB/s, so `cap/drain ≈ 0.21s` at the shipped
  256M — well under the 2s bound. Both legs of the RED drill's slow-drain premise are gone
  (the bond-reg/VDF flood is now rate-gated by the Phase 1.2 CPU gate, and its cost was
  ms-scale to begin with), so the two-class priority drain is **shelved** (owned residual
  E5 updated; the drill stays parked as its merge oracle, #183 has the named target). One
  caveat owed: measured on M4, not the ~1-vCPU floor box — SHA-256 is hardware-accelerated
  on cheap ARM so the shelve is expected to hold, flagged as expectation-not-measurement.

### Fixed
- **The concurrent-publish 502: a Care/NetGet event-loop self-deadlock — NOT the
  inbound cap** (2026-08-19) — Under concurrent UI ingest (the first production
  workload's 4-worker segment flood) a validator daemon hard-failed publishes with
  502 while surviving. Local repro + a control run attributed it: the inbound cap was
  innocent (unbounded cap fails identically; the always-on loop-saturation telemetry
  stayed silent). The 15s hang watchdog caught the real mechanism: every successful
  publish's `-care-published` auto-caretake ran `Node.Care` ON the daemon's event
  loop, and Care's synchronous `Registry.Lookup` — chainhost on a validator —
  marshals BACK onto the same loop and blocks awaiting a task the wedged loop can
  never run: a reentrant post-and-wait self-deadlock, 30s per publish (the chainhost
  timeout), starving every queued message behind it (placements blew their 4×2s
  attempts → "placed on no node"; entries outlived the 30s commit poll). Five core
  doorways shared the class (`Care`, `NetGet` via apiFetch, the repair sweep, the
  audit sweep, repair-claim verification). Fix: `node.lookupEntry` — a node holding a
  chain replica answers these lookups from its own committed chain (the very read
  chainhost performs), so no loop-context registry read round-trips through the
  adapter; chainless nodes fall through to the registry port unchanged. Failing-first
  at two tiers: the loop-reentry integration pair (`adapters/chainhost/loopreentry_test.go`)
  and the concurrent-UI-publish e2e (`e2e/publishflood_test.go`), which also covers
  the drive-by fix that `-inbound-cap 0` (the documented "unbounded" sentinel) was
  rejected by the size parser. Mechanism record:
  [docs/thinking/2026-08-19-publish-502-attribution-care-self-deadlock.md](docs/thinking/2026-08-19-publish-502-attribution-care-self-deadlock.md).
  The roadmap's Phase 1.1 note that the fairness/priority-lane work "also fixes" this
  502 is corrected — that work stands on the PE ruling's adversarial case alone.
- **P2P robustness under real streaming load — DHT recursion crash, reprovide (#69),
  fetch/discovery, cold-start** (2026-08-19) — Surfaced running silt under a real
  HTTP-streaming workload (a 14 GB / 381 K-file store on a 2 GB box; the #464/#465 OOM fix
  independently confirmed to hold there). **DHT walk recursion → stack overflow:** the
  provider-resolution/repair continuation chain (`announceAll`,
  `resolveProviders⇄probeShard⇄sweepProviders`) fired its terminal callback INLINE when a
  walk converged synchronously (no live peers), piling up O(keys) frames over a large held
  set → `fatal error: stack overflow` (watchdog kill, blocked large-file publish); the walk
  terminal is now trampolined through the loop → O(1) depth. **Periodic full reprovide
  (#69):** provider records lease out at `ProviderRecordTTL` and `AnnounceHeld` ran once at
  startup, so a holder went undiscoverable ~TTL after boot; `StartReprovide` re-announces on
  a TTL/2 timer (a full re-announce, stack-safe atop the trampoline). **Fetch/discovery:**
  `apiFetch` serves locally-held content before the swarm and pulls drawn content onto the
  main node (consumer==provider, no ephemeral-node loopback leak); a public node drops
  loopback peer addresses learned via gossip (`selfPublic()`-gated) so its book/resolution
  don't rot. **Cold-start:** the relay + registry listeners bind before the O(store) proof
  maturation scan (they need no proofs), so a public node's resolver-facing services answer
  immediately instead of being connection-refused for the minutes the scan takes; the scan
  logs progress.
- **Storage-node cold start: the O(store) proof scan goes fully async — startup never
  waits on it** (2026-08-18) — On a real 14 GB / 381K-file store the startup proof reload
  (`LoadProofs`) scanned the WHOLE store (~8m45s) to rebuild its resident `proofMeta`
  index, blocking the daemon's startup sequence — growing with store size (a TB-scale
  durability node → tens of minutes). This is an availability regression introduced by
  the O(hot) proof paging (#464/#465): paging traded resident RAM for a startup scan.
  The listener-bind reordering above removed the scan from in front of the relay/registry;
  this completes the fix: `Node.StartProofReload` schedules the scan onto the event loop
  in bounded batches (`clock.AfterFunc`, 128 proofs/task) instead of running it inline, so
  the WHOLE daemon starts and serves immediately while `proofMeta` matures lazily in the
  background; the full proofs already page on demand (#464), so serving never waits for
  the scan, and an announce that races the scan self-corrects on the next reprovide sweep
  (#69). Every `proofMeta` write stays on the single event loop (no new locking).
  Surfaced by flixz's public-node load test. A persisted `proofMeta` sidecar (cold start
  O(delta), not O(store)) is the tracked fast-follow.
- **The MATURING daemon OOM — an unbounded inbound message queue (a resource-exhaustion
  DoS), fixed with read backpressure (`-inbound-cap`)** (2026-08-18) — Heap-profiled on
  the wire (`e03f80d-heapprof`): consensus nodes at ~1 GB RSS held ~500 MB of *live*
  decoded CBOR on the path `tcpnet.readLoop → eventloop.Post → node.handle` (252 MB in
  `cbor.fillByteString` alone; only 35 goroutines — no goroutine leak). Mechanism:
  `readLoop` decodes every inbound frame and `Post`s a closure **capturing the payload**
  onto the event loop's **unbounded** queue (`Post` "never blocks"); under load (big
  bond-reg blocks, gather storms, 20 peers) inbound decode outruns the single serialized
  loop, the queue backs up, decoded messages pin RAM → OOM-crash-loop. This is
  availability-under-adversary (a flood OOMs an honest node — the memory twin of the #424
  CPU-flood), a **security floor**, not efficiency: *bounded-then-fast*. Fix: a bounded
  inbound-bytes admission gate (`adapters/tcpnet/inbound.go`) — the per-connection reader
  acquires a frame's bytes **before** decoding and releases when the loop finishes the
  message; over the cap the reader stops draining the socket → TCP flow-control pushes
  back on the sender. A fatal OOM becomes a survivable throughput limit (alive > crashed);
  the loop only releases, never acquires, so no deadlock; a lone oversized-but-legal frame
  is still admitted. `-inbound-cap` (default 256M; 0 = unbounded/legacy). **v1 is a global
  budget that stops the OOM; per-peer fairness + a consensus-priority lane are the pending
  hardening before the red team (a flood could otherwise stall consensus behind the cap) —
  PE ruling.** Plan + ruling: [docs/thinking/2026-08-17-inbound-backpressure-fix-plan.md](docs/thinking/2026-08-17-inbound-backpressure-fix-plan.md).

### Added
- **`-allow-web-origin`: opt-in browser transport onto the network** (2026-08-19) — An
  off-by-default, exact-match CORS + Private Network Access allowance so a hosted resolver
  page can draw content from a viewer's LOCAL silt node (a cross-origin HTTPS→localhost
  request the browser otherwise blocks). Secure by default (empty list ⇒ localhost-only);
  the operator opts a specific origin in. One browser-facing transport; more are expected.
- **The MATURING OOM return-to-2GB: rolling retention horizon, payload-selective pruning, and
  suffix-sync (H2 slices 1–5 — now ENABLED)** (2026-08-18) — A
  validator's chain grows O(all history) in the ~1.5 MB space-time bond proof
  (`BondReg.Answer`) carried by every registration, OOMing a 2 GB box (build-immutable #8,
  the hobbyist floor). The fix bounds the RESIDENT heavy payload to a recent finalized
  window. Slice 1: `RetentionHorizon() = finalizedHead − 2·BondTTL`, epoch-floored
  (research-certified safetyDepth). Slice 2: Opt-1 pruned-block representation (`Block.Prune()`
  drops the heavy `Answer`, keeps header + consensus sigs, stores the pre-prune hash so a
  pruned block still hash-links and stays valid late-reveal slashing evidence). Slice 3
  (this change): the **Q2 gate** — a pruned (Answer-less) block is trusted, and its
  space-time re-verify skipped, ONLY strictly below the node's OWN finalized/checkpoint
  anchor (`trustFloor`); at/above it is rejected (`ErrPrunedAboveHorizon`), and a pruned
  block still carrying an `Answer` is rejected (`ErrMalformedPruned`). During a `Reconcile`
  replay the floor is pinned to the RECEIVER's anchor (threaded into the throwaway replica),
  never the peer's fork — so a peer cannot skip verification to forge standing (a C1/M0
  no-discount break). The Reload/own-disk path needs no gate change (it never re-verifies
  bonds; the stored hash covers the sig). Consensus-invariants: preserves I5 (a pruned block
  is still slashable), reads I3/I4's finalized anchor; no quorum-sizing/signing/fork-choice
  change. Slice 4: `pruneBelowHorizon()` — the actual in-place shed of `BondReg.Answer` from
  finalized blocks strictly below the prune floor (`max(2·BondTTL, BondRegHeadWindow+margin)`
  below the finalized head, epoch-aligned; 0 without finality or with a degenerate `BondTTL`,
  the PE-acked over-prune guard). Because the durable store and serve path both read `c.blocks`,
  one in-place shed bounds resident, on-disk, AND served heavy payload. **Still DORMANT — no
  production path calls it:** enabling it changes how nodes sync (mesh catch-up is a full-genesis
  `Reconcile`, which the Q2 gate rejects against a pruned peer), so the PE gated enablement on the
  safe sync redirect. Slice 5 (this change) is that redirect, which **enables the prune**: mesh
  catch-up now suffix-syncs from a node's OWN finalized head (`{Height: FinalizedHeight()}` instead of
  `{Height:0}`), prepending its own verified prefix so the existing genesis-rooted `Reconcile` (with its
  slice-3 `trustFloorOverride` pinned to the node's own anchor) accepts its own pruned history but never
  trusts a peer-served head — the C1/long-range guard the PE ruled (peer-served-head trust rejected as a
  Sybil break). A node behind by less than the weak-subjectivity window catches up around the pruned gap;
  a deep-cold node beyond it gets `ErrNeedCheckpoint` (obtain a recent `-ws-checkpoint` out-of-band or use
  an archive node) — surfaced, never silent (I4/S5). `pruneBelowHorizon()` is wired into the commit path,
  so a validator sheds heavy proofs below its floor as finality advances — the line that returns the
  MATURING box to 2 GB. Consensus-invariants: I4 (catch-up-or-signal, both asserted), I3 (trusted set from
  the node's own finalized snapshot), I1/I5 preserved (no quorum re-sizing; equivocation still caught in
  the suffix, at/above the finalized head where forks can exist). Ablation-verified load-bearing; full
  core suite green. Plans + rulings:
  [slice3](docs/thinking/2026-08-18-slice3-q2-gate-plan.md),
  [slice4](docs/thinking/2026-08-18-slice4-prune-blocked-on-sync-redirect.md),
  [slice5](docs/thinking/2026-08-18-slice5-sync-redirect-plan.md).
- **Daemon memory controls: `-mem-limit` (soft heap ceiling) + `-debug-addr` (pprof)**
  (2026-08-17) — The MATURING field cohort OOM-crash-loops on 2 GB nodes, and it is
  NOT the PoR proof map (#464 shipped without moving it; the crash-looping nodes hold
  ~no chunks). Local + in-process probes show no leak — the signature is a
  large-but-bounded working set colliding with Go's default 2×-heap GC target on a
  small box. `-mem-limit <size>` (e.g. `1500M`) sets `runtime/debug.SetMemoryLimit`
  so the GC reclaims before the kernel OOM-kills (equivalent to `GOMEMLIMIT`; the flag
  wins) — a memory-bounded head, not a hard cap. `-debug-addr <addr>` serves Go pprof
  (heap/goroutine) + dumps a heap profile to `<store>/heap-<pid>.pprof` on `SIGUSR1`,
  so the true consensus-node footprint can finally be ATTRIBUTED (the daemon had no
  heap profiling, which is why the wrong structure was first blamed). Both off by
  default. `integration/cloudtest` gains `DEBUG_PROFILE=1`/`MEM_LIMIT=` knobs and a
  `./cloudtest.sh heap <node>` capture. Deliberation:
  [docs/thinking/2026-08-17-oom-not-the-proof-map-attribution.md](docs/thinking/2026-08-17-oom-not-the-proof-map-attribution.md).
- **#184 adversarial drills made DRIVABLE on the wire under the objective BFT model
  — equivocation (slash-on-detection) and partition-heal (stall-then-catch-up)**
  (2026-08-17) — Both marquee attacks GAPped on every field sheet because the drills
  imported legacy-mode (`-objective=false`, quorum 1) assumptions that objective
  3-of-4 correctly forbids. Fixed per three PE rulings, mechanism proven at the code
  level first (`core/node/modelcheck_184_equivocation_objective_test.go`, failing-first).
  **Equivocation:** a fork can never be COMMITTED onto a target under a BFT quorum
  (2-attestation single-target commit is quorum-short; a minority fork is an I1
  violation), so the crime is *signing* two conflicting blocks at one height, not
  *committing* two forks. New `PlaceConflictingSigned` adversary primitive: a
  consensus-set validator participates honestly (its era-2 `(round, prepare)`
  signature lands on-chain), then SERVES a conflicting signed block at that slot
  (crafted `GetChain`/head-probe response); an honest peer fetches it on sync and
  `FindEquivocations` slashes the same-slot cross-fork prepare pair unaided,
  pre-Reconcile, never adopting the quorum-short loser. It runs on its OWN dedicated
  ephemeral net (`e2e/equivocation_test.go`, objective 4-anchor, over real TCP;
  netem via `integration/adversarial`) — the one destructive drill (a proven
  double-sign is a permanent F2 eviction) is isolated from the shared sheet, whose
  mid-sheet eviction would pin the commit requirement at 3-of-4 against 3 live
  anchors (zero fault tolerance). **Partition-heal:** a severed sub-quorum minority
  cannot commit, so on heal it CATCHES UP (a forward sync, `dropped=0`), it does not
  reorg — a droppable reorg would require a minority to commit a conflicting fork
  (the I1 violation model B forbids; the absence of a reorg line IS the safety
  property). The drill (`integration/cloudtest` + `e2e/partition_test.go`) severs the
  minority from the whole > ⅔ majority, drives the majority to commit a heavier
  chain, asserts the minority STALLED (anti-vacuity), then reconverges to the
  majority head (height + hash) on heal. Deliberation +
  the three rulings: `docs/thinking/2026-08-17-drill-drivability.md`.
- **Publish client: the accept→commit poll re-derived for the #451 round durations
  (180s→360s) and made self-healing — the poll loop now RE-SUBMITS the entry every
  30s** (2026-08-17) — Run 82bcd2b-39478's durability-turnover GAP ("accepted but not
  committed within 3m0s") was honestly unpinned between discovery (#351) and a #441
  mature-quorum residual. The pin, per the PE work order, came from two new
  deterministic model-check oracles over the 12-member mature fixture
  (`core/node/modelcheck_441_publish_bound_test.go`): (A) under steady renewal
  contention with delivery intact, an accepted entry rides the VERY NEXT committed
  block — three consecutive publishes — refuting the fold-starvation residual at the
  model tier; (B) with the fire-and-forget submit burst dropped, the entry strands in
  the accepting validator's mempool for a MEASURED designee-rotation wait (8 chain
  heights in the oracle schedule ≈ tens of minutes at the field's 220s/height escape
  bound) — unreachable by any single-shot poll. So the failure class is CLIENT-side
  liveness, not a consensus defect: `publishPollTimeout` was still the genesis-era
  180s, BELOW the in-spec per-height worst case the #451 synchronizer durations imply
  (H_ESCAPE 220s; the harness's own re-derived PUBLISH_RETRY_S is 360s — same
  derivation, now one number), and the certified #441 design's drop-recovery lever
  ("the client's retry loop re-sends") never fired inside the window because the
  client submitted once and only polled. The re-submit is a mempool-dedup no-op on
  the happy path and the recovery lever on the lossy one (failing-first regression:
  a stranded entry that only a re-submission lands). Harness: a failed `ft_publish`
  now captures the VALIDATOR journals with the GAP verdict (82bcd2b's capture had
  only client-side nodes — the accept→commit window was unattributable, #7's
  capture-first rule), and the verdict text decomposes by the captured error instead
  of the #351-or-#441 disjunction. M1 note: the 360s bound shrinks by shrinking
  round durations (batching, #299), never by the client under-reporting them.
- **#456 concurrent gather: prepares and precommits are broadcast-and-collected — a
  dead epoch member no longer taxes every proposal its full retry timeout**
  (2026-08-16) — The two-phase gather asked attesters strictly sequentially, so each
  unreachable member's full transport retry budget (~34s at field config) sat on the
  critical path before the next attester was even asked: with a third of the epoch
  silent, every proposal paid ~270s (the B2 drill's twice-field-reproduced stall —
  runs ce15a80/1eded27 — after the #453 synchronizer was confirmed working via the
  catch-up telemetry). Per the research certification (456-gather-serialization):
  BFT tolerates f faults BY CONSTRUCTION — a correct protocol never waits for the
  faulty — and broadcast-and-collect-until-quorum is the universal shape; the
  sequential chain inverted it into f×timeout on every proposal. Both phases now
  send to every attester at once and complete on the SAME quorum predicate
  ValidateCommit demands (SupportMeetsQuorum: count + anchor majority + frozen
  weight); assembled certificates are copied at capture so a late reply can never
  mutate them. No certified property is order-sensitive (one gatherer per proposal;
  the QC is carried in the block, never re-derived) — #432/#402/#397/#389 untouched,
  the full oracle set green. Per-peer patient retry (network-durability §2) is kept;
  only its SERIALIZATION across peers — the flat-aggregate anti-pattern one level up
  — is removed: patient AND concurrent. Failing-first via the model-check's first
  COARSELY-TIMED oracle (the sim clock as the cost model, the certification's method
  fix): dead-first ask order, RED sequential (4.02s simulated = 4 dead × timeout × 2
  phases), GREEN concurrent (0 simulated).
- **`syncTargets` returns a deterministic (ID-sorted) list — a B2-determinism leak
  caught by the #451 fixture flaking under `-count=10`** (2026-08-16) — The list that
  drives gather ask-order, round-change broadcast order, and (in the model-check) the
  entire schedule was returned in raw map-iteration order: every gather's QC
  composition was a per-call dice-roll, flaking the #451 freeze-frame fixture 2-in-10
  (the sybil author's prepare-QC landed on order-lucky subsets) and adding silent
  run-to-run variance to every field gather. ID-sort is safe here — ask order is not
  an inclusion-fairness surface (any assembled quorum is valid), unlike the reg/entry
  queues which stay FIFO per #448/#441; `EligibleProposers` already sorts for the
  same reason. The #451 oracles now pass 10× under the race detector.
- **#451 view synchronizer: increasing round duration + responsive catch-up — round-based
  liveness gets its missing second half** (2026-08-16) — The clean re-run's B2 stall
  drill re-proposed a carried lock across rounds 4–15 for 370 s with 8 of 12 members
  live holding >99% of the weight: silt had #432's locking (safety under round
  disagreement) but no SYNCHRONIZER (convergence into a shared round) — a fixed
  `roundAdvanceSweeps` let independently-skewed sweep timers smear the members across
  rounds forever, and a round-change recorded at a receiver never pulled it forward.
  Per the research certification (451-view-synchronization, adopting Tendermint/PBFT
  whole): **(a)** round duration now grows `dur(r)=dur(r−1)+r·k` in deterministic sweep
  counts — after GST the round eventually outlasts any timer skew or adversarial
  round-change smearing (red-team seam #7, closed as a bounded residual), the
  load-bearing guarantee; **(b)** a straggler JUMPS to the smallest round proven ahead
  by f+1 anchors (launch) / >⅓ frozen weight (mature) of recorded round-changes, or by
  a valid higher-round new-view certificate — catch-up at message speed. Neither
  ingredient changes which value a node may sign: locking, the proposer-prepare rule,
  and the #402 arithmetic are untouched (the I1/S1/S2 oracle set stays green).
  Failing-first per the certification's merge gate: the staggered-sweep oracle
  (pre-GST chaos — skewed sweeps, per-target prepare delivery, driver-fired request
  timeouts scattering the sign marks; post-GST — the certified convergence bound) is
  RED against locking-without-a-synchronizer and GREEN with both ingredients. Method
  fix recorded in the model-check canon: per-node round-advance skew is a first-class
  adversarial schedule dimension.
- **Bond-reg queue goes FIFO-by-arrival — the ID-sort starvation behind the confirm
  run's 22-minute maturity stall** (2026-08-16) — Run 54003f7-91159's latch missed its
  computed bound because the 3rd maturer's first-time registration sat in the designees'
  queues for 22 minutes: the reg fold sorted pending regs by validator ID, and with the
  byte budget admitting ~one plot-sized reg per block, ID order is a strict priority —
  the highest-ID submitter loses to ANY lower-ID renewal, every block, for as long as
  renewal traffic flows (the census: 49 ahead-skew refusals were the visible symptom;
  the queue acceptance was fine, the fold order was the starvation). This is the exact
  class the #441 certification closed for entries with FIFO (Addition 2: no fees ⇒ no
  priority order that can defer indefinitely), still live on the reg side — #429 had
  named it ("ID-sorted packing makes order seed-luck"). The queue is now FIFO by
  arrival with replace-in-place renewal updates (a resubmission refreshes bytes but
  keeps its position: renewing cannot queue-jump, waiting cannot lose seniority), and
  an over-budget reg keeps its seniority for the next block instead of being dropped.
  Proposer-side inclusion policy only — no validity or quorum rule changes (block-byte
  determinism needs only the single designee's own order). Failing-first:
  `TestBondRegFIFONoIDSortStarvation` (RED under the ID-sort — the high-ID first-timer
  never banks across four one-reg blocks — GREEN under FIFO).
- **#441 publish starvation FIXED: entries are mempool content the designee's block
  carries — the certified operation-liveness mechanism** (2026-08-16) — The first
  mature-regime field run committed ZERO publish entries post-latch across 33+ heights
  while the chain stayed live on drain blocks, and the first launch soak stalled a
  publish-contended height 361s past the computed escape bound: one root, the #432
  round machinery's new-view seat AND its escape arming belonged exclusively to the
  bond-reg drain path, so an entry proposal could win no round of any height. Per the
  research certification (441-publish-starvation, 2026-08-16, direction A): a publish
  now SUBMITS its entry (`MsgSubmitEntry`, the `MsgSubmitBondReg` mirror —
  validate-on-arrival with synchronous refusal reasons, FIFO mempool dedup'd by root)
  and polls for finality; the single (h, r) designee's block folds pending entries
  under a byte budget SEPARATE from the reg budget (`-max-entry-bytes-per-block` —
  neither stream can starve the other), and pending entries arm the round escape
  alongside regs. Entries are content, never a competing value: locks/POL, the
  proposer-prepare rule, and the #402 arithmetic are untouched, and a forced
  (lock-carrying) re-proposal never folds. The born-RED starvation oracle plus five
  certification siblings (launch-face entry-only arming, adversarial-designee drop
  within the O(f+1) fairness bound — an owned residual, no *permanent minority*
  censorship — FIFO no-internal-starvation, entry-flood-vs-renewal budget isolation,
  and S1-with-entries lock-never-displaced) are all RED under the recorded fold+arming
  revert and GREEN with the fix. Bonus, pinned by the certification §7 discriminator:
  drain-alone commits at r0, so the field's every-height r1 escape (~95–155s/height)
  was entry contention — the fix recovers the r0 happy path. Legacy (-objective=false)
  deployments keep the direct propose path (no rounds machinery exists there to drive
  a mempool, and no drain contention either). I4's full statement is now
  operation-liveness — no legitimately submitted operation is permanently starved —
  recorded in the invariant map.
- **`BlockVersion` (the mint era) flips to 2 — the era-2 follow-through promised by the
  #432 change** (2026-08-16) — The rounds era shipped with the propose path explicitly
  stamping `BlockVersionRounds`, deferring the const flip as behavior-neutral test churn.
  Landed: production minting is byte-identical (the propose path already stamped v2;
  no non-test site builds genesis from the const), the att/bond-reg wire wrappers now
  carry version 2 (accepted by every era-2-capable binary — a pre-#432 binary cannot
  participate in an era-2 network regardless), and the 63 test files that hand-build
  era-1 blocks now declare `Version: 1` explicitly — the honest form, since they
  exercise the still-supported era-1 validation rules rather than tracking the mint
  const. Full suite + race green.
- **Node-level mature-epoch model-check fixture + the dynamic mature S1/S2 oracles**
  (2026-08-16) — Closes the #432 certification's named residual (1): the S1/S2
  merge-gate oracles ran the round machinery over the real node loop only in the launch
  regime. `matureWorld` (core/node) now drives a real 8-node network — 4 launch anchors
  + 4 bonded 64 MiB distinct-domain maturers, the field re-split shape — to a governed
  mature epoch entirely over held delivery (drain banks the maturers, they qualify by
  attesting, the everMature latch trips, the epoch boundary freezes the maturer
  snapshot), verifies the premise first (latch on every replica, anchors shed from
  eligibility, an anchors-only proposal REFUSED in the governed epoch, a 3-of-4
  weight-quorum commit tracked everywhere), and then runs the certification schedules
  dynamically: S1 (a delayed >⅔-weight round-0 quorum must be carried forward by the
  lock rule) and S2 (a Byzantine maturer's weight-short forged lock must die at
  round-change verification) — both anti-vacuity-pinned (CommitRound == 1) and both
  RED under the recorded lock-free revert. This world is also the deterministic home
  for the open mature-regime r0-contention observation from run 09fbe60-84613.
- **Drain-curve observability: commit lines carry the bond-reg count, and a refused
  renewal submit is attributable to WAN head-skew instead of reading as forgery**
  (2026-08-16) — The committed-block lines (daemon banner and the node's structured
  `block committed`) printed entries and attestations but NOT how many bond registrations
  the block banked, so a field journal could not answer the drain curve's decisive
  question — which blocks carried which regs (the interrupted confirm run 09fbe60-84613
  was misread as "blocks committing empty" while regs were in fact landing). All three
  commit lines now print the reg count. And the census of that run's 54
  `bond-reg submit REFUSED` lines — every one a bare "signature" error across ≥7 honest
  validators — is the AHEAD-skew face of the #427 K-head window: a renewal is signed over
  the submitter's head, a receiver accepts only nonces of its own last-K COMMITTED heads,
  so a reg arriving ahead of the receiver's commit fails every window nonce and heals on
  the next resubmit sweep. The refusal line now logs the receiver's next height, the
  submit side logs the height it signed over (`bond renewal submitted`), and
  `TestBondRegAheadOfReceiverWindow_refusedThenHeals` pins the mechanism — refused while
  the receiver trails, the same bytes validate once it commits the head — so the field
  signature-refusal census reads as commit-propagation skew, not attack.
- **#432 rounds + locking: the two-phase (prepare→precommit) gather with a lock-carrying
  view-change — the I4 liveness escape, research-certified, era-gated as block version 2**
  (2026-08-16) — The #397 height-only never-sign-twice watermark made a crossed 2-2 proposer
  race a PERMANENT stall of a connected, all-honest launch network (the field wedge in runs
  9c3777d/8ae8326; `TestModelCheck_I4_WedgedHeightMustRecover`, born RED). Per the research
  certification (432-rounds-locking-liveness, 2026-08-15): consensus signatures are now
  (height, ROUND, phase)-scoped; a commit carries TWO quorum certificates at one round (the
  prepare-QC that justified precommitting, and the precommit certificate), each held to the
  full commit threshold in both regimes (launch strict anchor majority / mature >⅔ frozen
  weight — the POL threshold IS the commit threshold); validators LOCK on the highest-round
  prepare-QC (durable, mark-before-sign, restart-rehydrated); round advance is a deterministic
  sweep count (never wall-clock), and the view-change quorum carries the highest lock forward,
  forcing the next proposer to re-propose any potentially-committed value. Equivocation is
  round-scoped (same-(h, r, phase) double-sign slashes; a POL-justified cross-round re-sign is
  honest), and a committed era-2 block REQUIRES its author's round-scoped prepare — the
  structural ProposerSig's era-2 analogue, which is what keeps a double-proposal attributable
  (I5) while staying count-neutral in every quorum (#402 arithmetic untouched, tested
  byte-identical). Merge-gate oracles per the certification: S1 (delayed lower-round quorum)
  and S2 (equivocate-then-misreport, forged-lock injection) both RED against a recorded
  lock-free revert and GREEN with the prepare phase, plus per-(h, r, phase) restart I2 and
  §5.3 lock re-presentation. Era 1 blocks keep validating under era-1 rules — committed
  history is never re-interpreted.
- **Never refuse silently: the two consensus refusal sites that hid the #432 wedge now log
  their reason** (2026-08-15) — A peer-submitted bond registration that fails validation was
  dropped on arrival with no line (`MsgSubmitBondReg` receipt), and a drain proposal blocked at
  the proposer's own #397 sign watermark returned silently — so the #432 wedged-height liveness
  defect (chain stalled, cohort regs arriving-verifying-vanishing) was mis-attributed across
  three field runs (discovery/#351, CPU, staleness). The receipt path now logs
  `bond-reg submit REFUSED` with the exact `ValidateBondRegErr` reason (decode failures too),
  and the drain logs `bond-reg drain BLOCKED at own sign watermark` — at most once per 30s
  sweep, only while pending work is actually blocked, which is precisely the #432 wedge
  signature. One field observation now names this class (B5; build-immutable #7's
  instrument-first). The wedge itself is #432 (rounds+locking, research certification in
  progress) — this ships the observability ahead of the rule change because it is not one.
- **MATURING topology re-split: 4 honest maturers + 4 Sybils, with COMPUTED harness windows**
  (2026-08-15, per the PE concurrence on the latch-premise finding) — The `MATURING=1 SYBILS=8` field
  topology re-splits its 8 cohort slots into **4 honest maturers** (non-anchor validators, full 64M
  bond, UNSET `-domain` — each an independent address-diversity group) + 4 single-domain MinBond
  Sybils, making the drill the on-the-wire confirmation of the I3 oracle's certified shape: the
  bar-2 latch is now REACHABLE (`min(NakamotoOperators, NakamotoDomains) = 2` at full drain) while
  the cheap cohort alone still cannot mature the network — both pinned deterministically by
  `TestMaturingResplitTopologyLatchReachable`. `MATURING=0` topologies (the P1 launch gate) are
  byte-identical. The harness windows are now **computed bounds, not arbitrary wall-clocks** (PE
  cadence ruling §4): the latch window derives from worst-case drain order (ID-sorted packing ⇒
  9 reg-blocks × 64s worst-case block time + a submission leg ≈ 630s), the publish window from the
  per-leg retry budget (8s × 4 attempts + backoff ≈ 34s/leg × the fresh-publisher leg census ≈
  240s) — and a latch miss inside the computed bound with the maturer cohort live now records
  **FAIL (a finding), never a re-graded GAP** (maturers down ⇒ honest GAP, preemption-shaped). The
  flow also records the drain curve (val-a's per-commit C2 lines: 64MiB jumps = maturer bonds) so
  the bound is checked against the real drain order, and the B2 capture drill now stops the
  maturers with the anchors (leaving honest weight up would fake a capture). Also fixes a second
  latent premise bug the finding exposed: the cohort-seated gate required `participants ≥ 4+n_syb`
  (=12) but the C2 participant count is NON-anchor bonds only (max 8 here) — now `n_mat + n_syb`.
- **MATURING-drill premise repro: the 10-maturing-handoff latch is unreachable in the field topology
  as parameterized** (2026-08-15) — `TestMaturingFieldTopologyLatchUnreachable` (core/chain) constructs
  the exact MATURING=1 SYBILS=8 parameterization at FULL drain — every bond banked, every participant
  generously granted attester status — and measures `min(NakamotoOperators=3, NakamotoDomains=1) = 1 < 2`,
  so `Mature()` is false no matter how many regs commit: `C2Metric` excludes anchors **by design**
  (counting the scaffolding's own bonds to shed the scaffolding would be circular, immutable #3) and the
  topology's 4 validators are all anchors, while the only non-anchor cohort — the 8 single-`-domain`
  Sybils — aggregates to ONE address-diversity group, which is the certified C2 discount doing its job.
  The two field GAPs ("latch never tripped in 420s") therefore had two stacked causes: the bond-reg drain
  staleness race (fixed) **and** this premise defect — the drill asked the maturity metric to be tripped
  by exactly the cohort it exists to refuse. Core behavior is correct and now pinned; the fix is a
  harness topology re-split (honest non-anchor distinct-domain maturers, the I3 oracle's certified
  shape), routed through a PE concurrence note before the re-run. Found on a laptop while deriving the
  principled maturity bound — before the billable run, not after.
- **`-loop-budget` — emit the event-loop goroutine-budget decomposition at INFO for a diagnostic run**
  (2026-08-15) — The per-window per-handler breakdown (from the eventloop instrumentation) is debug-gated
  by default so it's silent at steady state; `-loop-budget` raises just that summary to INFO so a
  load/diagnostic run captures the full per-handler CPU breakdown **without** the `-log debug` firehose —
  which, logged synchronously on the loop goroutine, would itself skew the very measurement. Slow-task,
  queue-wait, and hang lines are always-on regardless. The cloudtest harness threads it via `LOOP_BUDGET=1`.

### Fixed
- **Daemon OOM: bound resident PoR-proof RAM to O(hot), not O(total held chunks)**
  (2026-08-17) — A node kept the full `StorageProof` (Merkle `Path` + per-block PoR
  `PorTags`, ~5.4 KB each) resident for EVERY hosted chunk, never evicted — so a disk
  full of content pinned proof RAM at O(total held) and crash-looped the whole
  MATURING cohort (field-corroborated). The full proof now lives in the durable proof
  store; the node keeps only ~80–100 B of resident METADATA per chunk (`Root, Index,
  Total, Column` — everything the existence checks, iterate-all sweeps, re-announce
  and denylist sites read without paging), and a new bounded LRU (`adapters/proofcache`,
  mirroring `cachestore`: byte-budget, write-through-no-warm scan resistance) pages the
  big fields in only to SERVE or AUDIT a proof. Resident proof RAM is now O(hot). A
  daemon wires `proofcache` over `diskproofs` (new `-proof-cache` budget, default 64M);
  sims keep an in-core in-memory backing (hexagonal core takes no adapter dependency,
  B1). The audit answer is byte-identical whether the proof was resident or cold-paged
  — a where-it-LIVES change, not a where-it-VERIFIES change (PoR verification, I1–I5
  untouched). `ports.ProofStore` gains `Get`/`Keys` (per-id paging + one-proof-at-a-time
  startup reload, so `LoadProofs` is O(N) I/O but O(hot) RAM). Design + the two build
  refinements: `docs/thinking/2026-08-17-proof-map-oom-fix-plan.md`,
  `docs/thinking/2026-08-17-proof-map-oom-build-refinements.md`.
- **Bond-reg drain staleness (factor ii of the MATURING cadence wall): accept a reg over the last K
  committed heads** (2026-08-15) — A bond registration is signed over `BondRegNonce(prev)` and was validated
  only against the **current** head, so the instant the head advanced a reg in flight went stale and was
  refused. Over a real WAN (a proposer proposes on head-advance before the resubmission arrives) this starved
  the drain — blocks committed empty below the #286 byte cap and maturity never reached bar-2 in-window (the
  instrumented run measured the goroutine ≤7% busy, so it was never CPU; this staleness race was the cause).
  `validateBondRegs`/`ValidateBondReg` now accept a reg whose proof validates against any of the last
  `BondRegHeadWindow` committed heads (default 8, deterministic walk), removing the one-head brittleness while
  keeping freshness **bounded** (K ≪ `BondTTLBlocks`, and continuous bond-audit re-challenges possession — so
  a released-and-replayed old reg still decays out; pinned by a beyond-window-rejected test). Paired with a
  **durable pending queue** — the proposer now keeps valid regs that didn't fit the byte cap instead of wiping
  the whole pending set and relying on a re-broadcast. Failing-first + the #406 model-check tier (I1–I5) green.
  The K-vs-anti-release bound is a C1 parameter flagged for a research security-check.
- **#424 — bond-audit answer path was a remote-triggered CPU-DoS; add a per-challenger rate-limit**
  (2026-08-15) — Answering a bond challenge forces a fresh sequential VDF-eval (an unpredictable nonce, so
  it can't be precomputed) on the node's single event-loop goroutine, and `answerBondChallenge` served one
  on **every** incoming `MsgBondChallenge` with no per-challenger limit — so one peer flooding challenges
  could pin a validator's thread and starve its token-issue/commit/sync handlers (red-team seam #7).
  `allowBondChallenge` now caps evals served to a single challenger per `BondAuditInterval` window
  (`bondChallengeBurst`=8), refusing the excess **before** the costly eval so a flood gains no
  amplification. The cap is **per-challenger** (not global) so a flooder cannot starve honest challengers
  of their own budget; honest cadence is one challenge per peer per window, well under the cap. This also
  caps the O(n) audit fan-out that is a suspect for the MATURING single-goroutine saturation wall (PR #423
  instrumentation will name the dominant term). The exact cap borders the audit path — flagged in #424 for
  a research/PE confirm.

### Added
- **Event-loop latency instrumentation — name the slow (or hung) handler from real load** (2026-08-15,
  Andrew's timing-for-evidence idea) — The single event-loop goroutine (`adapters/eventloop`) is the one
  serialization point, so it is the one place to see where the node's thread actually goes. Each task now
  carries a label (inbound deliveries by `msg.Kind`; timers/commit/api by a constant) and the loop
  optionally reports: **slow tasks** (`SlowThreshold`/`OnSlow` — a single task blocking the thread), a
  **hang watchdog** (`HangThreshold`/`OnHang` — a task still in-flight past a deadline, reported once with
  an all-goroutine stack dump of exactly where it is stuck), **queue-wait** (`QueueWaitThreshold`/
  `OnQueueWait` — the causal signal: a task that executes fast can still blow a downstream deadline by
  *waiting* behind a saturated thread, so this ties saturation to the 8s request-timeout that
  execution-time alone would miss), and a **per-window budget summary** (`SummaryEvery`/`OnSummary` —
  count/total/max execution AND queue-wait per label = the goroutine-budget decomposition, cause and
  effect in one window, so the dominant handler is named from real execution, not a reconstruction; the
  summary is debug-gated, slow/hang/queue-wait are always-on). Adapter-only, zero-value = off
  (the sim's own scheduler is untouched); wired in the daemon via the existing logger. This is the
  observability the starved MATURING field run lacked (its per-issuer gather legs were debug-gated).
- **#406 — consensus model-check, tier 2: the I5/#397 honest-never-slashed oracle** (2026-08-15) — Over
  the real node loop + held-delivery: two proposers cross-attest-race one height in a WEAK config where
  both forks can commit (objective, no anchors, `ByzantineQuorum` off, `Quorum 1`, N=4 → no finality
  gate, no anchor gate — the pre-#402 baseline), and **no honest validator is slashed**: the #397
  propose-time never-sign-twice watermark makes each proposer refuse the rival, so no honest node
  double-signs. Proven **failing-first** by controlled revert (removing the propose-time `recordSign`
  lets each proposer cross-attest → both forks commit → sync slashes both honest proposers via `OnSlash`
  — RED; with the watermark — GREEN). Resolves the deferral from the substrate PR: #402's I1 shields the
  both-commit fork in the normal objective tree, so the weak config is what makes it reachable.
  `core/node/modelcheck_tier2_test.go`. Test-only. **With this + the #357 launch replay, the launch tier
  now has all four scars (#357/B2/#397/#402) failing-first — the P1-run gate criterion.**
- **#406 — consensus model-check, tier 1: the #357 launch replay (no reorg of a finalized block +
  fork-choice determinism)** (2026-08-15) — `core/chain/modelcheck_i5_357_test.go` asserts the invariant
  that closes #357: over adversarial competing forks (bare-genesis, shorter, equal-height, and even
  **taller** conflicting), a super-quorum-finalized launch block is **never reorged** (D-1), and
  fork-choice is **order-independent** (reconciling the same fork set in any order yields the same head
  — a pure function, never the height-blind hash tiebreak that dropped committed blocks to height 0).
  Proven **failing-first** by a controlled revert: forcing `finalityQuorumActive` false (the pre-#357
  no-gate state) lets a taller fork reorg the finalized chain — RED; with the shipped gate — GREEN. Also
  asserts the #357 weight face (a committed anchor-attested chain carries nonzero fork-choice weight).
  Test-only.
- **#406 — consensus model-check, tier 2: the I2-across-restart oracle** (2026-08-15) — On the
  held-delivery substrate: a validator signs a block at height h through a REAL gather, then
  "crashes and restarts" (a fresh `Node`+chain on the same endpoint), and must **refuse a competitor
  at h** — the never-sign-twice watermark must survive restart (#397 Q1b; Tendermint
  `priv_validator_state`). Failing-first **by construction**: the identical scenario run with a
  NON-persisted mark (the pre-#397 crash-wipe) *attests* the competitor, so the in-test control proves
  the persistence is load-bearing and the assertion is not vacuous. `core/node/modelcheck_tier2_test.go`.
  Test-only.
- **#406 — consensus model-check, tier 2: the simnet held-delivery substrate (adversarial delivery over
  the REAL node loop)** (2026-08-15) — `adapters/simnet` gains a test-only **held-delivery mode**
  (`EnableHeldDelivery` + `Pending`/`Deliver`/`DropPending`): `Send` parks each message so a driver
  fires them in an order IT chooses — the "adversarial delivery scheduler" the design specifies —
  off by default so every existing sim/e2e path is untouched, conformance-tested
  (`adapters/simnet/helddelivery_test.go`). `core/node/modelcheck_tier2_test.go` proves the substrate
  drives the real loop: a real proposer gathers a real quorum and commits + broadcasts to every replica
  **entirely over driver-controlled delivery** (no clock advance; request timeouts sit on the clock,
  which the driver never advances, so a gather completes purely by delivered replies). The adversarial
  invariant oracles that build on it (I2-across-restart; the genuine I5/#397 catch) are the documented
  next increment — the first I5 draft was **withheld because it could not be shown failing-first**:
  #402's I1 structurally prevents the both-commit fork the #397 slash needs, so the #397 fix is shielded
  by a different fix in the current codebase, and a genuine I5 catch needs a pre-#402 baseline
  (`docs/thinking/2026-08-15-406-tier2-substrate-and-the-i5-honesty-catch.md`). The only production code
  is the `adapters/simnet` held-delivery mode, which is inert unless `EnableHeldDelivery()` is called
  (no existing path calls it), so all shipping behavior is unchanged.
- **#406 — consensus model-check, tier 1: the I3 mature weight-quorum oracle (the B2 catch)** (2026-08-15)
  — `core/chain/modelcheck_i3_test.go` drives the REAL `Chain` to a **mature epoch** whose frozen
  `epochSet` holds the B2 imbalance — 3 real validators (distinct domains, 20 MiB) + **8 sybils** (one
  shared domain, 2 MiB) — and asserts **I3**: a coalition finalizes IFF it carries a **>⅔ WEIGHT**
  super-majority, never a head-count one. The 8-sybil cohort is a head-count quorum (7 non-proposer
  attesters = `bftThreshold(11)`) but a weight minority (16 ≪ ⅔·76), so it is refused. Exhaustive over
  the finality-relevant space by equivalence class (finality is a pure function of proposer-type +
  #honest/#sybil attesters, so ~72 representatives cover all 2^10·11 cases). The **setup is verified
  first** (`TestModelCheck_I3_SetupReachesMatureWeightedEpoch` asserts the mature-epoch state + exact
  frozen weights before any oracle trusts it — the anti-#303 discipline; it caught a real
  proposer-never-seen setup bug during the build). Proven **failing-first** by a controlled revert of
  `requireEpochWeightQuorum` to head-counting → RED. Test-only; no production change.
- **#406 — consensus model-check, tier 1: the exhaustive I1 launch oracle** (2026-08-15) — The first
  rung of the deterministic adversarial consensus harness (`docs/design/consensus-model-check.md`):
  `core/chain/modelcheck_test.go` drives the REAL `Chain` finality predicate over an **exhaustive**
  enumeration of adversarial anchor coalitions (N∈{3,4,5} launch regime, +8 sybils) and asserts **I1** —
  no two *disjoint* anchor coalitions may both finalize a block at one height (the invariant all four
  scars #357/B2/#397/#402 violated at their core). Proven **failing-first**: reverting the launch rule to
  the pre-#402 `AnchorQuorum=1` makes it go RED ("disjoint coalitions [0] and [1] both finalize" — the
  exact #402 defect) in milliseconds on a laptop, GREEN under the derived `⌊A/2⌋+1`. This is the
  cheapest-tier catch the testing-tiers assessment calls for; it begins the gate that ends "discover a
  consensus invariant by billable field run" (D-CONSENSUS). WIP — the mature/I3/I5 oracles and the
  simnet tier-2 (I2-restart) layer are the documented next steps; scope is stated honestly in the file
  header (S5). Test-only; no production change.
- **#402 — chain-tier repro attributing the launch anchor-gate fork (the field "CAPTURE" was a fork,
  not a Sybil capture)** (2026-08-14) — Field run `4faaee8-22913` graded a flow-5 "CAPTURE"; the
  captured evidence + `core/chain/fork_anchor_gate_402_test.go` show it was a **fork**. The
  wheels-engaged commit gate requires only `AnchorQuorum=1` distinct anchor attester, and the honest
  side commits at the bare count quorum (proposer + 2), leaving one **free** anchor; a Sybil-proposed
  competitor attested by that one free anchor passes the gate — while a **zero-anchor** Sybil quorum is
  still refused (`ErrAnchorRequired`), so **C2 holds** (no quiet capture). The residual is a
  launch-phase fork-creation / liveness vector. A second test shows `AnchorQuorum=2` closes the same
  fork (the honest commit then holds 2 non-proposer anchors, leaving `<2` free), naming the fix
  direction `AnchorQuorum ≥ ⌈#anchors/2⌉` and its cost (launch commits need 3-of-4 anchors up). The fix
  is a consensus-rule change, routed to research (`docs/reviews/fork-anchor-gate-402-RESEARCH-CONSULT-2026-08-14.md`);
  tests-only here, no product change.

### Fixed
- **cloudtest: the C2 no-capture flow's PASS verdict crashed on an unbound variable (P1 run
  b525b0b-87478), dropping an otherwise-green result** (2026-08-15) — `scenarios.sh:832` wrote the
  no-quiet-capture PASS with `($h1→$h2)` — an UNBRACED `$h1` immediately before the multibyte `→`.
  macOS `/bin/bash` 3.2.57 under `set -u` absorbs a byte of the multibyte char into the identifier
  (parsing `h1<0xe2>`), which is unbound → the `record()` crashed BEFORE writing, so the C2 drill —
  which **behaviorally passed** (sybils couldn't advance with anchors down; a driven block committed +
  synced when they returned) — recorded no verdict and dropped out of `results.jsonl`. Root-caused by
  reproducing the exact `bash: h1�: unbound variable` on bash 3.2. Fix: brace the vars (`${h1}→${h2}`);
  swept + fixed the one other instance (`${cp}…` at the WS cold-sync line). Regression guard
  `integration/cloudtest/check_shell_multibyte.sh` (a STATIC lint — the bug is bash-3.2-specific, so a
  Linux/bash-5 runtime test can't catch it) is wired into CI. Harness-only; no product change (the C2
  property held on WAN).
- **#402 — the launch anchor gate is now a DERIVED strict anchor majority `⌊A/2⌋+1`, structural in
  objective mode (research-certified consensus fix; encoding B)** (2026-08-14) — Closes the
  launch face of the intersecting-quorum invariant (I1). Two parts: (1) **anchor-only launch
  proposing** — during the young window only anchors propose; a bonded sybil drains via
  `MsgSubmitBondReg` (submit-don't-propose, #397), removing the sybil-proposed fork at its source.
  (2) The commit gate now requires a **strict anchor majority `⌊A/2⌋+1`** (=3 of 4) counting the
  proposer-if-anchor, **derived from `len(Anchors)` in objective mode independent of the
  `-anchor-quorum` knob** — so a missing/low config can never disable quorum intersection. Attribution
  correction that drove the structural choice: the field run (`4faaee8-22913`) left `-anchor-quorum`
  unset (flag default 0 → gate inert), so the fork needed no free anchor — a two-sybil-signature
  quorum committed it. The consult's proposed `AnchorQuorum=⌈A/2⌉` (=2) was insufficient: a
  both-sybil-proposed 2-2 anchor split satisfies it and the finality gate then *cements* a permanent
  conflicting-finalization partition. Legacy (non-objective) mode is unchanged (configured `AnchorQuorum`
  capture-prevention floor, no finality gate). Fault tolerance is the same 3-of-4 (1-fault-tolerant),
  now uniform and intersecting. Failing-first repros in `core/chain/fork_anchor_gate_402_test.go`
  (the 2-2 split; sybil-can't-propose; derived-ignores-config; 3-of-4 liveness); seam-7 equivocation
  test re-expressed at A=3 to keep its lone-culprit property under the majority rule. Certification:
  `silt-reviews/.../fork-anchor-gate-402-RESEARCH-CERTIFICATION-2026-08-14.md`; deliberation:
  `docs/thinking/2026-08-14-402-anchor-gate-encoding-and-derived-threshold.md`. Invariants: I1 (launch
  intersection), I3 (set = membership).
- **cloudtest flow 5: the C2 resume clincher now DRIVES a block after restoring the anchors instead
  of waiting for a spontaneous one** (2026-08-14) — Three runs GAPed "chain did NOT resume within
  180s" after the anchors returned. Attributed from the captured journals (run `9b2198e-67673`, the
  #396 evidence harness): the restored anchors were fully healthy — bootstrapped in seconds, standing
  back, bond challenges passing — but the chain is **reactive (B6)**: every due renewal was drained
  into blocks before the stop, and renewal-due is HEIGHT-based so a frozen chain mints no new ones —
  the restored network was legitimately QUIESCENT, and the clincher's wait mis-graded healthy idleness
  as a liveness gap. (The pre-#397 drain over-proposed own renewals — an accidental heartbeat that
  masked this.) The clincher now restores the anchors and then **drives a publish until it commits and
  the Sybil syncs it** (the same drive-then-verify pattern as flow 10's B2 drills), turning the
  verdict into a driven verification. Harness-only; no product change.
- **#397 — an honest proposer can no longer be slashed for a protocol-manufactured double-sign
  (research-certified consensus signing fix + the certified race closures)** (2026-08-14) — The first
  evidence-instrumented field run (`b88245d-3496`) wedged at a 2-2 fork at height 6 with BOTH racing
  anchors permanently slashed as equivocators: `proposeBlock` signed a proposal **without recording it
  in the never-sign-twice ledger** (only attestations were recorded), so when two renewal-clock-aligned
  anchors proposed the same height, each found an empty ledger and honestly **attested the competitor's
  block** — signing two different blocks at one height; the cross-fork scan then correctly slashed both
  honest anchors on both branches and the anchor-quorum chain died (publishes failed with validators
  4/4 reachable; restarting the anchors could not undo the committed slash). Research certification
  (`silt-reviews/…/honest-proposer-cross-attest-RESEARCH-CERTIFICATION-2026-08-14.md`) established the
  launch finality quorum was **already intersecting** (support-3-of-4; the double-commit was the
  bug-manufactured >f break, not a quorum-design flaw) and certified this fix set, all shipped here:
  **(Q1)** a proposal now enters the same never-sign-twice ledger as an attestation at sign time —
  **(Q1b)** replaced by a **persisted monotonic `{height, hash}` watermark fsync'd BEFORE any consensus
  signature is released** (`adapters/markstore`, the Tendermint `priv_validator_state` pattern; wired
  refuse-to-start in the daemon), so a crash/restart can no longer wipe the mark and let a validator
  contradict a signature it already shipped — permanent slashing (Q3, unchanged) is sound only with
  this; **(Q2b)** the two certified liveness race-closures: the drain takeover fallback now readmits
  **one proposer per sweep by rank** (was: every eligible proposer at once after 3 idle sweeps), and a
  non-designated proposer whose only pending work is its **own due renewal submits it**
  (`SubmitBondRenewal`, already broadcast every sweep) **instead of proposing** — removing the
  genesis-aligned renewal-clock collision that drove the field race; **(Q4)** two detection bugs: the
  local ledger slash is now **idempotent-once per culprit** (it re-applied and re-logged every ~2s
  reconcile sweep for as long as the fork stayed live) and a pending **on-chain slash record now
  requeues until a commit confirms it** (the requeue list was built but never appended to, so a slash
  whose carrier proposal failed to gather quorum was silently dropped). Failing-first regressions at
  unit + sim tiers: proposer-refuses-competitor, restarted-validator-refuses (crash variant),
  slash-idempotency, slash-requeue, submit-don't-propose, staggered takeover; the sim training-wheels
  test's doomed no-anchor attempt now uses an expendable proposer, mirroring its throwaway-attester
  convention (its old shape had the proposer double-signing height 1 — the exact #397 hazard).

### Added
- **cloudtest: a scenario-level FAIL/GAP now captures its evidence before teardown
  (build-immutable #7)** (2026-08-14) — Run `beb3628-95860` (the P1 all-corners run) ended with two
  FAILs (`9-cross-nat`, `chaos-reprovide`) and a new sybil-resume GAP that were **unattributable**:
  journals were only captured for nodes that never came ready, the scenario console died with the
  terminal, `ft_publish`'s per-call gap signal was lost in its command-substitution subshell (so
  `9-cross-nat` graded FAIL where the honest verdict may have been a #351 GAP, with no recorded
  error), and the next run overwrites `results.jsonl`/`report.md`. Now: `record()` snapshots the
  flow's involved nodes' service state + journal + `debug.log` into `flow-evidence-<run>.log` the
  moment a non-green verdict lands (nodes stashed by `require_nodes`/`require_live` or explicit
  `flow_evidence_nodes`); `ft_publish` hands its gap signal and last captured error across the
  subshell boundary by file, so `publish_verdict` grades honestly and the report names the mechanism;
  `9-cross-nat` attributes WHICH leg died (publish vs fetch); the scenario console is tee'd to
  `console-<run>.log`; and each run's `results.jsonl` + `report.md` are archived under `archive/` so
  a regression is distinguishable from a never-passed flow. Harness-only; no product change.
- **#390 — the cross-NAT restart re-fetch (`integration/nat`, #69 phase) no longer flakes on a loaded
  CI runner** (2026-08-14) — The `RESTART=1` cross-NAT job REDed bimodally (~1-in-4, on docs-only
  commits too) at "content undiscoverable after restart". Attributed: reprovide-after-restart is a
  real, working product path (proofs persist to the proof store, reload on restart —
  `reloaded storage proofs count=N` — and `AnnounceHeld` re-plants provider records after
  re-bootstrap), but the harness waited for it with a fixed `sleep 6` and then re-fetched ONCE. That
  is the magic-constant anti-pattern (build-immutable #5: wait for the condition, never a constant):
  under a loaded runner the re-announce + DHT propagation sometimes took longer than 6s, so the
  single re-fetch found no provider and false-FAILed. Fixed harness-side by confirming the reload
  then RETRYING the re-fetch on a ~60s bounded deadline — a genuine reprovide gap still fails after it
  (never masks a real #69 break; it only rides out a slow re-announce), the same "retry, don't guess a
  timeout" discipline the product uses. No product change. Green 5/5 locally (was ~1-in-4).
- **#378 — the `-equivocate` red-team drill is now deterministically drivable under WAN delay
  (the local adversarial gate's entry criterion for the external red team)** (2026-08-14) —
  The netem adversarial gate (`integration/adversarial`, `delay 80ms 20ms`) REDed BIMODALLY on
  `TestEquivocatorSlashedOverTCP` — the drill that proves a double-sign is caught and slashed over
  real TCP (#184 accountability). The property itself always held (the slash fired); the *drill
  driver* wedged, so a required gate flapped ~1-in-4 — corrosive (it trains "re-run until green").
  Root-caused to THREE distinct placement wedges under warm-up jitter, each closed and each guarded
  failing-first: **(1) ErrDupRoot re-placement** — the old driver rebuilt the conflicting blocks at
  the LIVE head every retry, so once a leg committed and the culprit synced it back (head advanced),
  later attempts re-proposed the same deterministic root at a new height and were refused forever;
  fixed by PINNING the fork base + blocks on the first attempt and LATCHING each placed leg (the
  #345 live-tip win is preserved — the base is the tip AT PIN TIME). **(2) Attested-but-not-
  committed** — a target attests on the proposer's standing but commits on its OWN attestation's
  qualification, which warms up later, so a leg could attest yet ack the commit not-OK; latching on
  the round-trip alone then stranded the next block chasing an uncommitted parent. Fixed by latching
  a leg only on a CONFIRMED commit (`proposeAndCommitTo` now reports `committed`) and retrying an
  attested-but-uncommitted one. **(3) Detection disqualifies the proposer mid-drill** (found via
  wire diagnostics) — the moment an honest node holds BOTH forks it slashes the culprit, removing it
  from that node's qualified-proposer set, so any REMAINING placement there is refused "not yet
  standing" forever (the property firing wedged the drill). Fixed by placing the COMPLETE heavier
  fork [Y,Z] FIRST and the conflicting X LAST, so no honest node can see both forks until every leg
  is down; the slash then propagates asynchronously (the detector reconciles the heavier fork), which
  is the property under test. Harness-only (`core/node/adversary.go`); no honest consensus path
  changes. Regressions: two failing-first unit repros (`equivocation_resumable_test.go` for the pin,
  `equivocation_uncommitted_test.go` for the commit-confirmed latch — both red-verified against the
  old code) plus the netem gate itself, now **10/10 consecutive PASS** under the exact bimodal-RED
  condition (was ~1-in-4 FAIL). This clears the external red team's entry criterion.

### Added
- **MATURING cloud topology + field flow 10: the handoff/post-shed regime is now field-exercisable
  (§4 of the PE ruling; gates the external red team)** (2026-08-14) —
  The base cloud topology never matures BY DESIGN (4 equal validator bonds → Nakamoto coefficient
  2 < `-mature-validators 4`), so every prior field run exercised only the YOUNG anchor-gated
  regime — while the red team's sharpest target (brief seam #8) is the handoff and what follows.
  `MATURING=1 SYBILS=8 ./cloudtest.sh` now runs the topology that hands off on the wire: the
  maturity bar is set to the coefficient the 4 distinct-operator validators actually reach (2, at
  an explicit `-operator-margin 1` — deliberate and disclosed, uniform across every consensus
  role) and the Sybil cohort bonds the MINIMUM (1M vs the validators' 64M) so the B2 drills price
  per-head cheapness the way the research certification does. The new `flow_maturing_handoff`
  (flow 10) grades, outcome-first: **(10)** the `everMature` latch trips on the wire (`wheels shed
  permanently`), commits cross the epoch boundary into the governed mature snapshot, and no
  anchor-required refusal appears post-shed; **(10a) the B2 stall drill** — the cheap cohort
  declines to attest (stopped) and the honest >⅔-weight coalition must still commit (head-counted
  quorum left this exact network born-unable-to-commit); **(10b) the B2 capture drill** — the
  cohort alone must not advance past the honest ceiling (ceiling read from the honest validators
  before stopping them, the #383 catch-up lesson; a real capture also requires a fresh cohort
  commit log; the `frozen-weight super-majority` refusal corroborates), with the clincher that the
  chain resumes when honest weight returns; **(10c) WS cold-sync under the latch** — a validator
  restarts pinned to a peer-published `checkpoint: H:HASH`, catches up, and comes back with the
  wheels STILL shed (a restart must never re-arm the anchors, F-1). Preconditions grade as honest
  GAPs (latch never tripped, cohort never banked), never fake passes; flow 5 (anchor-gate
  no-capture) self-skips under MATURING=1 since its premise — a network that never sheds — is
  deliberately absent. Runs LAST (it stops validators). Both topology modes dry-run-validated;
  the base topology is unchanged byte-for-byte with MATURING unset.

### Fixed
- **Mature-phase quorum is now WEIGHT-counted — closing a cheap-member stall/capture seam at the
  handoff (B2, research-certified consensus change)** (2026-08-13) —
  The research certification of the token-gather consult (Item B2) refused to confirm the drafted
  mature-phase residual and instead **found a real break**: post-handoff, commit quorum was sized by
  **member count** (`bftThreshold(len(epochSet))`, `ValidateCommit` counting distinct attesters)
  while epoch admission is deliberately unfiltered (#357 Condition A seats every qualified bond).
  Every MinBond identity riding an honest handoff therefore weighed one head: **8 cheap members
  among 4 honest validators made the mature phase born unable to commit** (stall at 8×MinBond,
  nothing slashable — the cohort just declines to attest), and **9 made a cohort-only commit valid
  with zero honest attestation** (capture at 9×MinBond, persisting into full maturity) — a
  C1-discount + C2-quiet-capture break, caught by the PE addendum + research escalation rather than
  the external red team. The fix adopts the settled pattern (B8 — Tendermint voting power, Casper
  FFG ⅔-of-stake): a mature-epoch commit's coalition (proposer + distinct qualified attesters) must
  now carry **strictly >⅔ of the frozen epoch's bonded weight** (`requireEpochWeightQuorum`; new
  `ErrNoQuorumWeight`), replacing the head-count escalation (`RequiredQuorum` keeps only the
  `Config.Quorum` count floor there). Quorum weight and fork-choice weight are now the **same
  frozen quantity** (`epochSet`). The §3 finality gate stays engaged under the weight rule
  (`finalityQuorumActive`: two >⅔-weight coalitions intersect in >⅓ weight, hence honest bond),
  and the proposer's gather asks the chain "is this support enough" (`SupportMeetsQuorum`) instead
  of counting heads. Launch phase is UNTOUCHED (fixed anchor set; the §2 repro stays green).
  Failing-first drills verified red against the head-counted code (capture committed, honest 97%
  weight coalition refused "3 qualified, need 8"): `core/chain/quorum_weight_test.go` — cohort-only
  capture refused `ErrNoQuorumWeight`; honest weight commits through a declining cohort; strict-⅔
  boundary; plus the epoch Condition-A suites re-expressed in weight (frozen denominator across
  mid-epoch join and slash). The E4 owned-residual (bonded-minority stall) is now priced truthfully
  — its ⅓/⅔ claims were only true under weight counting, which research held it on. The §4
  maturing-topology field flow gains the stall + capture drills as its sharpest red-team target
  (seam #8).

### Added
- **Parallel publish-token gather — transport concurrent, signer selection unchanged (research-stamped)** (2026-08-13) —
  The flagship privacy flow (a fresh ephemeral client acquiring a `-token-quorum k` blind-signed
  publish token) was failing over real WAN under load: every leg was **sequential** — issuer-key
  fetches, per-issuer credit mints, canonical-set discovery, and the k blind-sign round-trips —
  ~`2·V+k` WAN round-trips end to end (`ft_publish FAILED after 120s` in both SYBILS=8 runs).
  All four legs now **overlap**, collapsing the gather to ~3 round-trip times. **The privacy
  boundary is unchanged and research-certified** (`token-gather-privacy-and-fault-tolerance-`
  `RESEARCH-CERTIFICATION-2026-08-13.md`, Item A1): requests fire concurrently at the **fixed
  network-canonical top-k signers** and wait for that exact set — acceptance is a function of
  canonical rank + liveness, with per-issuer failures **falling forward in canonical order**,
  never first-k-of-N-to-reply (the forbidden variant that would stamp the publisher's network
  position into the token's revealed signer set, re-opening R-3); token `Sigs` are assembled in
  canonical order so the token cannot leak the arrival permutation structurally. Canonical-set
  discovery (`FetchCanonicalIssuersFromAny`) races all validators for a **deterministic** answer
  (privacy-neutral: who answers first changes nothing about what is answered). **Issuance is now
  idempotent under transport retries** (certification Item A2): a retry re-presents the SAME
  blinded serial (deterministic RSA-FDH ⇒ identical signature) and the issuer **dedups both the
  signature and the charge/credit-spend** keyed on the blinded-serial hash within the transport
  retry window — before this, a lost *reply* double-charged the legacy fee and was **refused as a
  credit double-spend** on the prepaid-credit path, failing the whole gather. Instrumented per-leg
  (`token gather leg` debug logs with per-issuer elapsed). Failing-first regressions at unit
  (issuer dedup: single charge, identical sig, credit-retry accepted, TTL-bounded) and sim tier
  (canonical set accepted under heavy reply-order jitter across seeds; canonical fall-forward past
  a dead signer; round-trips proven to overlap on the virtual clock; 25% loss ridden out at
  exactly k fees). The named prediction for the next field run: `ft_publish` and the ~6 cascading
  flows flip to PASS, and `6-fault-tolerance` recovers (research Item B1's conditional close —
  if it does not, the latency attribution reopens).
- **Attribution repro for the SYBILS=8 `6-fault-tolerance` GAP: quorum sizing is correct; the GAP is gather-latency under load, not a consensus bug** (2026-08-13) —
  `core/chain/TestFaultToleranceBranch_SybilBondsDoNotInflateLaunchQuorum` reproduces the exact
  committed state (4 anchors + 8 banked single-domain sybil bonds, pre-maturity, objective, epochs
  on) and names the branch behind the field GAP: `validatorSetSize=4` (the launch anchor branch
  fires — NOT the `qualifiedCount=12` fall-through), `RequiredQuorum=2`, and a 3-of-4 anchor commit
  with one validator down **passes in-process**. So banked sybil bonds do **not** inflate the launch
  quorum, and the cloud GAP is a gather-**latency** effect under the 8-sybil load — not a
  quorum-sizing bug, no consensus rule change. Test-only; routed to research for concurrence
  (`archive/reviews/token-gather-privacy-and-fault-tolerance-RESEARCH-CONSULT-2026-08-13.md`).

### Changed
- **#382 (M1) — chain-sync elides the whole-chain re-fetch when peers already agree (cheap head probe)** (2026-08-13) —
  The first M1 efficiency change under the standing rule "trust stays green while cost drops." `SyncChain`
  used to fetch and re-validate every peer's ENTIRE chain every 30s sweep — O(chain × peers) bytes and CPU
  even when the whole network agreed (the dominant cost behind the participating-Sybil slowness, #382).
  Now each peer is first sent a cheap **head probe** (`MsgGetChainHead` → height + head hash); an identical
  head hash proves an identical committed history (a block hash commits its whole ancestry), so the full
  fetch is **elided**. It runs only on a real head difference — catch-up, reorg, or an old peer that can't
  answer the probe — where the unchanged full fetch + `Reconcile` + equivocation scan still fire. **Trust-
  neutral by construction:** every validity, reorg-detection, and slashing guarantee is unchanged; the probe
  only skips provably-redundant work. Backward-compatible (a peer too old to answer the probe falls back to
  the full fetch). M1 cost gauges added (`Stats.ChainSyncHeadMatches` / `ChainSyncFullFetches` — the
  chain-bytes-per-sweep signal, near-0 in agreement). Failing-first regressions: 5 sweeps against an
  agreeing peer do zero full fetches; a head difference still triggers exactly one full fetch and catches
  up; heads re-agree → elision resumes. Full node/chain/sim/e2e consensus+equivocation suites + `-race`
  green (trust-neutrality verified). A genesis-to-head block *diff* inside `Reconcile` is a further
  follow-up; this closes the dominant no-op-sweep cost.

### Fixed
- **#303 — test-honesty audit: closed the 7 still-live positive-control gaps / confounds before the blind field test** (2026-08-13) —
  Most of the 27 audited items were already fixed (PRs #304–312, #339); this closes the 7 that were still
  live, all in the integration harnesses (shell/compose only — no product code, `go test ./...` unaffected).
  Each fix makes the test FAIL if the property it claims to check is actually broken: the `upgrade` #69
  finding is now gated on real reload evidence (`reloaded storage proofs` count + V1-had-no-persisted-proofs)
  so a HEAD-reload regression or mesh/decode confound can no longer masquerade as the ancient-V1 format
  boundary; `consensus` P2 replaces a vacuous "no reorg" grep (which passes trivially when the local fork is
  already heavier) with a real positive control (valA's own committed head byte-unchanged) plus an honest
  SCOPE note that inbound-fork *receipt* is not CLI-observable; `cloudtest` chaos-reprovide and
  restart-standing are scoped to the post-crash/post-restart boot via a new `waitfor_since` (`--since @t0`)
  so a stale pre-crash log line can't satisfy them; `bond` PHASE 2 gates the low-bond reject on an honest
  node first ACCEPTING a well-bonded proposal (`-goodpropose`), so a reject-everything node can't false-pass;
  `retrieval` adds a baseline cold-fetch positive control so a seed/registry saturation isn't misattributed
  to the #43 routing finding. Every newly-asserted string was grep-verified as real product output.
- **C2 field flow reported a FALSE capture — a lagging Sybil catching up read as an "advance"; the property itself HELD** (2026-08-13) —
  The SYBILS=8 run FAILed `5-sybil-no-capture` ("the Sybil cohort advanced the chain 26→37 with all anchors
  down"). Root-caused: NOT a capture. The flow anchored its "ceiling" to **sybil-1's own head** (h0), and
  under the run's load (see the participating-Sybil slowness, #382) sybil-1 lagged ~11 blocks behind the true
  committed tip; when the anchors stopped, sybil-1 **caught up via normal SyncChain** to the anchors'
  already-committed blocks (26→37) and the flow misread that as a Sybil advance — the same catch-up
  false-positive class the *local* harness already fixed, never ported to the cloud flow. Chain-level proof
  the property holds in this exact topology: `core/chain/TestC2SingleDomainSybilsDoNotMature` — 8 equal
  single-domain bonds cap NakamotoBonds at 3 (< MatureValidators 4) **regardless of domain or margin**, so
  the network cannot mature, the launch-anchor gate cannot shed, and a no-anchor Sybil quorum is refused
  with `ErrAnchorRequired` (verified). Fix: the flow now (a) anchors the ceiling to the **true committed
  tip** read from the anchors *before* stopping them (a catch-up caps at that tip, so it can't be misread as
  an advance), and (b) requires a **fresh Sybil `committed block` log** for a CAPTURE verdict (a catch-up
  logs `chain reconciled`, never `committed block`) — a height rise past the ceiling with no fresh Sybil
  commit is now correctly classed as catch-up (GAP: property held, drivability masked by lag), not a FAIL.
- **#338 cloud follow-up — the Sybil cohort ran a divergent quorum FLOOR that stranded it at genesis, so the C2 capture drill still could not be driven** (2026-08-13) —
  The SYBILS=8 field run confirmed the drain + static-tier sync fix works (base validators reached tip 8,
  up from 3; every product corner green), but `5-sybil-no-capture` still GAPed with the same signature
  ("sybil-1 never synced a committed chain, head 0"). Root cause, reproduced deterministically in-process
  (`TestDivergentQuorumFloorStrandsSyncingNode338`): the harness gave the Sybils `-quorum 5` (a "self-majority"),
  while the anchors commit blocks at quorum 2. `Config.Quorum` is a hard FLOOR on `ValidateCommit`
  (`max(Quorum, bftThreshold)`), so when a Sybil re-validates the anchors' honestly-committed 2-attestation
  blocks inside `Reconcile` under its own floor of 5, every block fails `ErrNoQuorum`, the whole fork is
  rejected, and the Sybil is stranded at genesis — regardless of correct transport, static peers, and sync
  targets. In OBJECTIVE mode the "self-majority capture" is sized by `bftThreshold` over committed bond, not
  a config knob, so the fix is a **uniform quorum floor** across the objective swarm (`topology.py`: the
  Sybil role now runs the network `-quorum`, not `n_syb//2+1`). This both lets the Sybils sync (capture
  precondition met) and makes their capture attempt reach the real **anchor gate** (`ErrAnchorRequired`)
  rather than dying on a quorum count. The objective-mode quorum-floor footgun (a per-node floor above the
  network's `bftThreshold` silently breaks replica sync — arguably objective mode should ignore the local
  floor when validating committed blocks) is filed separately as a consensus-rule question, not changed here.
- **#338 — an idle young objective network never drained its deferred bond registrations; a non-attester validator had no path to the committed chain** (2026-08-13) —
  Two structural gaps behind the local `nakamoto 0 bonds` state and the SYBILS=8 field GAP ("sybil-1
  never synced a committed chain — anchors hadn't banked the Sybil bonds; capture precondition unmet").
  **(1) Reactive bond-registration drain.** Pending registrations (#336-deferred off the lean genesis,
  peer-submitted via the H2 renewal path) were only ever folded into publish/revocation proposals — on
  a young network with no content traffic nothing proposed, so no validator (anchors included) ever
  earned committed standing and maturity was unreachable. Now a proposer-eligible validator holding
  pending registrations (or whose own is due) proposes a **BondRegs-only block** on the chain-sync
  sweep — reactive (B6: fires on pending state, quiesces empty), budget-bounded (#286 L2b), guarded by
  **never-sign-twice-at-a-height** (the failing-first repro caught two anchors drain-racing one height,
  cross-attesting, and equivocation-slashing EACH OTHER into a wedged chain) and a **deterministic
  designated proposer per height** derived from committed state (`chain.EligibleProposers`,
  `ids[height % n]`, absent-proposer fallback after 3 sweeps). **(2) The configured persistent-peer
  tier is now a chain-sync target** (`syncTargets`): a validator whose attester seed holds no
  chain-carrying peer (the cloud sybil cohort — attesters are only other sybils) and whose bond gossip
  hasn't warmed otherwise had NO path to sync or submit (configure-not-discover,
  `network-durability.md` §8); the cloudtest sybil role now configures `-persistent-peers` over the
  validator set. The local `integration/sybil` harness graduates from its scoped-down standing-gate
  form to the REAL C2 property: it now asserts the autonomous drain commits, the sybil syncs, a bonded
  sybil **publishes through its own registry with the anchors present** (positive control), and the
  capture attempt without anchors is refused by the **anchor co-sign gate** on a genuinely-bonded
  Sybil set — previously cloud-scoped, now local. Failing-first regressions at the node tier
  (`TestIdleYoungNetworkDrainsPendingBondRegs338`, `TestSyncTargetsIncludeStaticPeers338`).

### Added
- **#357 Conditions A+B — the mature phase epoch-snapshots its validator set, and the young→mature handoff lands at a finalized boundary (research-certified)** (2026-08-13) —
  Completes the #357 arc: the research certification conditioned its C1/C2 soundness ruling on two
  pieces of mature-phase machinery, both now built. **Condition A** — finality is quorum-INTERSECTION
  safety, which only holds when every super-quorum is taken over the SAME set; recomputing
  `validatorSetSize`/qualification live from the churning bond ledger (joins, renewals, TTL expiry)
  could let two conflicting commits each "finalize" against two different sets. Post-handoff consensus
  (quorum size N, attester/proposer qualification, attester fork-choice weight) now reads a per-epoch
  FROZEN snapshot of the committed bonded set (`Config.EpochBlocks`), rotated only at epoch-boundary
  blocks — each itself super-quorum-final under the §3 gate. Churn integrates at the next rotation
  (bounded by one epoch); the one live mid-epoch disqualification is a proven slash, which is
  shrink-only against a frozen N (can only raise the effective bar). **Condition B** — the anchor→bond
  weight-meaning transition is now the FIRST mature rotation: after the `everMature` latch trips
  mid-epoch, the anchors keep governing (eligibility, weight, anchor sign-off) for at most one more
  epoch until the finalized boundary sheds them, so bond-weighted fork-choice is rooted at an
  immutable base and can never reach back across the boundary. One-way both ways (F-1: neither the
  latch nor the handoff ever re-arms). Daemon: `-epoch-blocks` (consensus-critical, set identically
  across the swarm), **safe-by-default** for an untrusted objective validator (`DerivedEpochBlocks` 8
  — ≤ ¼ of the derived bond TTL, so a mid-epoch-lapsed bond's vote outlives its TTL by at most an
  epoch); explicit 0 opts a trusted/demo swarm out (pre-epoch live recompute, unchanged). Failing-first
  regressions at the chain tier: mid-epoch join/TTL-expiry cannot move N, RequiredQuorum, or
  qualification; the handoff waits for the boundary (anchor sign-off still required between latch and
  boundary); a mid-epoch slash disqualifies immediately with N frozen; plus the certification's
  repro-ladder step-2 drain-window ordering test (a lagging replica converges by catch-up; a
  conflicting, heavier drain ordering is refused without dropping committed height; weight strictly
  monotone across the drain). Full `go test ./...` + `-race` (chain/node/sim) green.

### Fixed
- **Field-test harness: the 8-takedown probe tested the wrong surface; the equivocation drill's refusals were silent** (2026-08-13) —
  Two test-harness observability fixes from the M0-candidate field run's GAPs. (1) `flow_takedown`
  GAP'd on every run (`denied= served=1`) because its denial leg ran `swarm get` ON store-1 and
  grepped for a refusal — but `swarm get` is a short-lived CLIENT node that never consults the
  daemon's denylist and fetches from any other holder, so the grep could never match (audit-#303
  class). The denial leg now asserts the surface `-denylist` actually gates: the daemon's own
  enforcement narration ("denylist: N root(s) denied…"), with the no-global-switch leg (store-2
  serves bit-perfect) unchanged. (2) The `-equivocate` drill's retry loop swallowed every refusal,
  hiding a real wedge (#345 family, caught while certifying under netem): after a PARTIAL placement
  (X committed on the first target while the second hadn't yet qualified the adversary), every
  later attempt rebuilds the same adversarial entry root, which the first target refuses forever
  (root already registered) — the drill starves permanently and bimodally under WAN-ish delay.
  Each refused attempt is now narrated so a wedge is distinguishable from warm-up; the drill-design
  fix (atomic or resumable placement) is tracked separately.
- **#357 §3 — the quorum-finality gate: a super-quorum-committed objective block is irreversible (research-certified, owner D-1)** (2026-08-13) —
  Completes the ratified bond-weighted BFT model (research certification 2026-08-13). §1+§2 stopped the
  oscillation; §3 adds the *safety* guarantee: `Reconcile` refuses any fork that would revert our
  committed head (`ErrPreFinalityReorg`), so heaviest-weight fork-choice only ever adjudicates among
  DESCENDANTS of the finalized head — "reorg to height 0" is structurally impossible, and under a >⅓
  partition a node STALLS rather than reorg committed history (owner decision **D-1**; the storage plane
  keeps serving throughout, **D-2**, so durability is unaffected). **Finality is quorum-INTERSECTION,
  never bare depth** (a depth cap lets two partitions finalize conflicting blocks — worse than a reorg),
  so the gate is **gated on a real super-quorum**: it engages only when `RequiredQuorum ≥ bftThreshold`
  (always true with ByzantineQuorum, the untrusted default). A trusted weak config (Quorum=1) has no
  quorum intersection — a lone equivocator can split the honest set onto two committed forks — so it
  keeps heaviest-chain reorg and its equivocation slash heals by adopting the heavier fork (this is why
  the Quorum=1 live-tip equivocation-slash path is unaffected). Realized on the existing WS-checkpoint
  machinery (a rolling finalized floor). The red-team fork-choice tests that encoded the *old* Nakamoto
  healing are rewritten to the BFT model: **F6** now proves convergence by CATCH-UP (a behind replica
  adopts a longer chain that EXTENDS its finalized prefix; a conflicting heal required equivocation,
  which B slashes); **F7** proves a cross-height double-backer is neutralized by FINALITY (the finalized
  fork stands, the conflicting heavier fork is refused). Ramp repro extended to assert the gate. Full
  `go test ./...` + `-race` (chain/node) green. **Staged next (mature-phase machinery, research
  Conditions A+B):** epoch-snapshot the mature validator set + a finalized young→mature handoff; the
  drain-window sim (repro-ladder step 2) and a BFT e2e partition-heal rewrite (supermajority commits /
  minority catches up).
- **Fork-choice oscillation during the anchor→bonded ramp — §1 convergent weight + §2 stable quorum (#357)** (2026-08-13) —
  The blind multi-region field run committed blocks then reorged them back to height 0; the
  local `consensus` sim passed (it only ever tested the *mature* regime). Research root-caused it
  (`silt-reviews/research/.../357-...-RESEARCH-RESPONSE.md`) and the owner **ratified the
  bond-weighted BFT model (B)**. Two of the three ranked defects are fixed here (the third, the
  finality floor, is a staged follow-up):
  **§1** — during bootstrap anchors are `attesterQualified` but contribute `bonded[id]=0`, so every
  fork's `Weight()` was ≈0 and `heavier()` fell through to its **height-blind head-hash tiebreak** —
  a genesis fork whose hash sorted lower thus displaced a committed chain. Fixed by (1a) crediting a
  qualified launch anchor a fixed bootstrap weight (`Config.AnchorWeight`, default `MinBond`) so an
  anchor-attested chain carries real, height-growing weight from block 1, and (1b) making the
  tiebreak **height-aware** (equal weight ⇒ the taller chain wins; head-hash only breaks a
  weight+height tie). Both are **C1/C2-neutral**: the weight is the sanctioned immutable-#3
  training-wheels trust, vanishes at maturity (`launchAnchor ⇒ false` once `everMature`), and the
  mature-regime quantity (summed committed bond) is unchanged.
  **§2** — `RequiredQuorum` was sized against the **live-moving** `qualifiedCount`, so it shifted
  block-to-block as registrations drained in and no fork held a quorum of a consistent set.
  Fixed by sizing against a **stable validator set** (`validatorSetSize`): the fixed anchor set
  during the young window (seeded at genesis — `bftThreshold(4)=2`), transitioning to the committed
  bonded set at maturity. Deterministic bootstrap-ramp repro
  (`core/chain` `TestForkChoiceRampCommittedChainOutweighsGenesis357`, research Ask 4) that FAILS
  before §1 (`Weight()==0`) and passes after; full `go test ./...` + `-race` on chain/node green.
  **Staged follow-up (owner review):** §3, the rolling BFT finality floor (refuse to reorg below a
  quorum-committed block), completes model B but rewrites objective fork-choice semantics
  (Nakamoto-heal → BFT-final) and the red-team tests that encode the old model — held for review.

### Added
- **Regression guard for the #288 evict-on-one-miss anti-pattern (P0-4)** (2026-08-13) —
  `core/node` `TestLivePeerIsRetriedNotEvictedOnOneMiss` locks the build-immutable #5 rule that a
  live peer must be *retried*, not evicted, on a single slow/dropped packet: a dead peer given
  `RequestRetries=2` must be dialed 3 times (initial + 2 retries) before eviction, so re-introducing
  evict-on-the-first-miss (the shape that starved consensus under loss) turns this test red.
  Counting dials over a run-to-completion is timing-independent, so it can't flake. The
  `internal/wanguard` scope note now records that retry/evict are guarded by routing + this behavior
  test (they are semantic policy, not an AST-lintable construct). No product behavior change.

### Fixed
- **Canonical issuer-set discovery falls through an un-synced validator (#351, P0-2 residual)** (2026-08-13) —
  A chainless publisher (`silt swarm add`) picks its publish-token signers by a *canonical*,
  ledger-ranked ordering it fetches from a validator, so the signer subset isn't a
  per-publisher quasi-identifier (R-3 / seam-4). It asked only `validators[0]`, so a single
  un-synced or transiently-unreachable validator — e.g. one that just **restarted mid-run**
  (#351) — dropped the publisher into the `-peers` fallback, which *narrows the publisher
  anonymity set*. Added `Node.FetchCanonicalIssuersFromAny`, which tries each validator in
  order until one serves the set (`cmd/silt/swarm.go` now uses it). The canonical ranking is
  **deterministic** — every chain-holder computes the same bond-ranked order — so asking a
  different validator returns the same answer: pure liveness/anonymity robustness, **no change
  to selection, consensus, or the privacy claim**. Regression: `core/node`
  `TestFetchCanonicalIssuers_ReturnsLedgerRankedSet` now also asserts FromAny skips a chainless
  validator and returns the chain-holder's ranked set. **Scope (V4):** this closes the
  canonical-set half of #351 only; the token-*acquisition*-after-restart path (reaching enough
  signers for the token-quorum when one is down) is a privacy-sensitive residual that needs a
  deterministic repro to pin the failing stage — tracked, not addressed here.
- **Provider-record lifecycle — age out departed holders so the repair/fetch loop stops re-dialing corpses (#277, P0-1)** (2026-08-12) —
  The principal-engineer substrate plan (P0-1) targets the dominant durability/retrieval
  wound: the #277 dial-storm. Attribution first (build-immutable #6), which *corrected* the
  audit's hypothesis: the DHT walk is **already** `deadUntil`-gated (PR #355) and signed
  provider records are **already** expiry-filtered on read (`acceptedProviderIDs`→`Verify`).
  The genuine residuals were three: **(1)** a confirmed-dead holder's provider record is never
  removed, so `deadUntil` only *rate-limits* the re-dial to one full `RequestTimeout` per
  `HolderCooldown` (30s) **forever** — the persistent dial-storm floor; **(2)** the re-serve
  path (`MsgGetProviders`) handed out stale records with a raw `Get`, propagating the corpse to
  whoever asked; **(3)** the announce/re-announce path was the last provider-record consumer not
  gated on `deadUntil`. *The loop drowns in dials to departed holders **because** their records
  outlive them in every consumer; **fixed by** giving the store a lifecycle and gating the last
  consumer.* Adds `dht.Providers.Live` (filter expired on read — used by the re-serve path),
  `Evict` (per-`RepairInterval` age-out sweep in `repairTick`), and `RemoveIfNotSole` (prune a
  confirmed-dead holder from every key with a live alternative, called when retries exhaust and
  the peer is negative-cached) — the last **keeps a SOLE holder's record** so its content stays
  discoverable and re-probeable (#69/#226) rather than orphaned. Also gates the announce path on
  `deadUntil`. **M1 baseline instrumented now** (the efficiency gate starts in P0): two gauges,
  `Stats.HolderDialsSkipped` (dials avoided by the negative cache — the dials-per-fetch/repair
  bound) and `Stats.DeadProviderRecordsPruned` (records aged out). Failing-first regressions:
  `core/dht` unit tests for `Live`/`Evict`/`RemoveIfNotSole` (incl. the sole-holder-kept case),
  and `core/node` `TestConfirmedDeadHolderPrunedFromReplicatedKeptForSole` (verified fail-before /
  pass-after). Full `go test ./...` + `-race` on `core/node`/`core/dht` green.

### Changed
- **Corrected two materially-false public claims flagged by the principal-engineer rescue audit** (2026-08-12) —
  A read-only fresh-eyes audit found the public site overclaiming two M0 corners beyond what the
  canon and code support. (1) `marketing/anchor-launch.html` claimed a published root is unlinkable
  to its authorizing identity *"at any layer an observer can watch"* — the exact transport/metadata
  layer the code does **not** cover (and it contradicted `website/index.html`, which already admits a
  transport IP+timing link remains until issuance-mixing ships, Pre-V1). Softened to the
  refuse-to-surveil / access-held-in-tension wording already ratified in TENETS immutable #4. (2)
  `README.md`, `ROADMAP.md` (→ generated `website/roadmap.html`), and `website/index.html` claimed the
  storage plane is *"field-proven at scale"* — but "at scale" is the deterministic in-process
  simulation; no warm multi-region cloud run has graded a full suite end-to-end. Changed to
  "sim-proven at scale, field-proven cross-network at small scale," keeping every true claim (the CI
  Docker NAT/hole-punch evidence is genuine). Docs-and-copy only; no behavior change.

### Removed
- **Deleted a stale 13 MB prebuilt binary committed to git and a dead export** (2026-08-12) —
  `integration/flakynet/silt` (a committed aarch64 binary) is removed and gitignored — the flakynet
  harness already `go build`s the daemon from source at run time, so the checked-in binary was a
  supply-chain smell and a risk of "field-proven" silently certifying a stale build. Also dropped the
  fully-dead `core/bond/bond.go` `PlotSeed` export (zero callers). No behavior change.

### Fixed
- **Provider diversity sweep honors the dead-peer negative cache — closes the last ungated resolve leg (#277)** (2026-08-12) —
  The `deadUntil` negative cache that stops silt re-dialing a just-timed-out peer was consulted on the DHT
  distance walk (`node.go`), the fetch path (`file.go`), and the repair probe (`repair.go`) — but **not** on
  the domain-diversity sweep (`sweepProviders`, `dht_diversity.go`), the second leg `resolveProviders` runs
  whenever `DHTDomainCap > 0`. Since the daemon and client both set `DHTDomainCap = 2`, the sweep runs on
  **every** provider resolution, so under churn a departed holder still in the routing table was re-dialed at
  a full `RequestTimeout` on every resolve — a contributor to the #277 repair/retrieval dial-storm (a
  caretaker sweeping a churny swarm "drowns" and never finishes a sweep). Fixed by gating `deadUntil` in
  `sweepProviders` exactly as the other three paths do (a sweep is breadth discovery, so a cooled peer is
  simply skipped — no sole-holder concern like the fetch path's `#69` `anyLive` guard). Surfaced by the
  2026-08-12 blind field test, which also exposed a unit-test blind spot: the existing dead-cache tests set
  `DHTDomainCap = 0`, isolating away the exact leg the daemon always runs — so a new failing-first regression
  (`TestProviderDiversitySweepSkipsCooledPeer`) engages `DHTDomainCap = 2` (FAILs "got 1 dials, want 0"
  before, passes after). Closes one named, verified leak on the resolve side; not the whole #277 envelope.
  Follow-up: gating the sweep would have blacklisted a *recovered* holder still inside the 30 s cooldown
  (breaking `#69` cross-NAT restart survival — the `deadUntil` cache was only cleared on a successful
  bootstrap dial). So a **message is now proof of life**: receiving any message from a non-ephemeral peer
  clears its `deadUntil` entry, cleanly separating a departed holder (sends nothing → stays gated, dial-storm
  fix intact) from a recovered one (restart + reprovide, or a NATed peer now reachable via the relay →
  un-gated at once). Guarded by `TestInboundMessageClearsDeadCache` and a green `RESTART=1 integration/nat`.
- **Equivocation red-team drill double-signs at the live tip, not a stale height 1 (#345)** (2026-08-12) —
  The `184-equivocation` field-test drill FAILed on the #286 GCP re-cert ("no slash line within 120s"),
  while the same property passes in-process (#204). Root cause: `Node.Equivocate` (the `-equivocate`
  red-team harness) hardcoded the double-sign at **height 1**. On a fresh chain that is the live tip and
  slashing fires; but the cloud drill runs *after* the warm-up has committed several blocks, so a height-1
  double-sign is **stale** — an honest target refuses to attest a proposal that is not at `head+1`
  (`ValidateProposal`), so `proposeAndCommitTo` fails, the conflicting forks are never placed, they never
  enter fork reconciliation, and nothing is slashed (the adversary never even logged "equivocation
  complete"). Fixed to double-sign at the **current uncommitted tip** via `chain.Head()` — backward-
  compatible on a fresh chain (tip = genesis ⇒ height 1, so the in-process slash test is unchanged) and
  live on an advanced chain. Also pointed the cloud drill at the **direct detector** (val-b, which holds
  fork X and catches the double-sign the instant it syncs val-c's heavier fork) instead of val-a, which
  only sees the slash after on-chain propagation. Guard: `TestEquivocateAtLiveTipSlashesOnAdvancedChain345`
  advances past height 1, runs the real `Equivocate` path, and asserts the culprit is detected + evicted —
  it FAILS on the old height-1 code ("target refused the proposal at height 1"). This is a red-team
  *harness* fix; the product's equivocation detection + on-chain eviction was already correct.
- **Distribute/repair proofs are O(log n) too — same cached Merkle tree (#340)** (2026-08-12) —
  The sibling of the bond fix, on the storage plane: `distributeFrom` (`file.go`) and `repairStripe`
  (`repair.go`) build a Merkle inclusion proof **per shard** so hosts can answer storage challenges — each
  via the standalone `manifest.Prove`, which is O(n) (it rehashes subtrees). Proving S shards over an
  n-leaf manifest was therefore **O(S·n) ≈ O(n²)** on the node loop — for a large file (thousands of
  leaves) that is seconds of on-loop proof work during distribution or a repair sweep, exactly the kind of
  loop stall the bond compute layer exposed (#286). Fixed by building one **`manifest.Tree`** per
  distribution/repair and drawing every per-shard proof from it (O(log n) each); it also replaces the
  redundant `m.Root()` (a second full O(n) `MerkleRoot`) with `tree.Root()`. Proofs are **byte-identical**
  (`TestTreeMatchesStandaloneProve`), so the storage-proof and PoR-tag paths are unchanged — verified by
  the full node suite (distribute + proof-verify + repair) staying green under `-race`.
- **Bond proof answers are O(log n) again — cached Merkle tree (#286 compute layer, #340)** (2026-08-12) —
  The confirming 3-region GCP run reopened #286: after the network layers (L1/L2a/L2b) genesis *still*
  wedged at height 0 because bond proof-of-space-time compute saturated the single consensus loop (B2) —
  decisive proof: bond 64M→2M dropped CPU ~90%→~3% and genesis committed instantly. A pprof isolation
  pass (build-immutable #6, root-cause before you patch) found that the per-challenge answer *also* scaled
  with plot size, contradicting the assumed size-independence — and pinned why: `manifest.Prove` was
  **O(n), not O(log n)**, because `auditPath` recomputes `merkleTreeHash` over half the leaves on **every**
  call, and `AnswerSpaceTime` draws O(k) proofs, so each answer cost O(k·n) and grew with the bond. On a
  64 MiB plot one answer took ~743 ms (and RegisterBondReg rebuilt proofs on every propose retry). Fixed
  with a precomputed **`manifest.Tree`** cached on the bond `Commitment` (built once in `Seal`/`Reconstruct`),
  so each inclusion proof reads cached subtree hashes in O(log n): the same construction, **byte-identical
  root and proofs** (guarded by `TestTreeMatchesStandaloneProve` across every leaf count) — no proof-param
  change, C1-neutral. Measured effect: a 64 MiB answer drops **743 ms → ~8 ms (~95×)** and is now flat
  across plot sizes. This removes the recurring per-audit-epoch scaling; the remaining Ω(size) on-loop cost
  is the one-time `Seal()` at onboarding, moved off-loop next (research Option A, #340).
- **Genesis commits small — bond registrations spread across blocks (#286 Layer 2b)** (2026-08-12) —
  A real 3-region GCP cert run (with the #331 persistent-peers fix + #327/#332 `-log debug`) proved
  address convergence was fixed but genesis *still* didn't commit, and pinned the true final blocker:
  `gather: starting height=1 bytes=7866154 regs=5` — the proposer piled **every** founding validator's
  ~1.5 MB space-time bond registration into the single genesis block (~8 MB), which the quorum gather
  can't move + re-verify over a WAN before the round churns, and because it never commits the validators
  re-submit forever (308×), pinning the block huge. Root cause and fix confirmed by research against the
  code: the founding set are **anchors** (`chain.launchAnchor`), so genesis quorum bootstraps from anchor
  eligibility at *zero committed bond* (`qualifiedCount=0` keeps `RequiredQuorum=Quorum`) — the block does
  not need the bonds in it. Fixed with a **byte budget, `Config.MaxBondRegBytesPerBlock`** (flag
  `-max-bondreg-bytes-per-block`, default ~2 MiB): the proposer embeds bond registrations per block up to
  that byte budget (`chainrole.go`), so genesis commits small on the anchor bootstrap and the deferred
  registrations **drain over the next blocks** — each validator still gains real bonded weight and the set
  reaches `MatureValidators`. A **byte** budget, not a count, is the right lever because the blocker is
  *size*, not number: at genesis one full ~1.5 MB proof fits per block, but small steady-state renewals
  pack many per block — so an attest-only validator's renewals are never starved under a tight TTL (a
  count cap lapsed them; `sim/bond_renewal` proved it). Anchors grant eligibility, never fork-choice
  weight, so nothing is fabricated. Reproduced deterministically in-process (no WAN needed) and guarded by
  `TestGenesisBondRegsSpreadAcrossBlocks286L2b` (4 anchor validators, quorum-2: genesis commits with one
  ~1.5 MB reg and all four bonds drain by block 4). The structural close remains #299 (succinct +
  aggregated bond proof).
- **Registry client rides out transient loss on its reads (#329, durable-WAN audit)** (2026-08-12) —
  The HTTP registry client's idempotent GET reads (`Lookup`, `/publish-status`, `All`) were single-shot:
  a single dropped packet or a transient 5xx failed a `swarm get` / root resolution outright. This was the
  one client path NOT behind the consensus layer's retry — the lone true violation of build-immutable #5
  found in the durable-WAN audit (#329). Fixed with a **bounded exponential-backoff retry** (3 attempts,
  200 ms base) on those GETs — ride out transient loss instead of deciding on a single sample
  (`docs/network-durability.md` §1 "modest initial + retry", §2). A definitive `4xx` (e.g. a `404`
  not-found) is returned at once, never retried; `POST /publish` is deliberately NOT retried here (its
  commit durability comes from the async 202 + poll). Guarded by `TestClientGetRetry_RecoversTransient`
  / `_BoundedGivesUp` / `_NoRetryOn404`.

### Added
- **Handshake attribution instrumentation (#286 Layer 2 Q3)** (2026-08-12) — To attribute the
  cross-region inbound `err=EOF` before tuning any deadline (the research team's "instrument first"
  discipline), tcpnet now logs, on every failing handshake, the numbers that separate the candidate
  causes: the **inbound** side logs the **concurrent in-flight handshake count** (the hub-stampede gauge)
  and the **elapsed** time before failure; the **dialer** side logs the dial **budget** and the **elapsed**
  spent. On the next run, a clustered EOF with high concurrency and elapsed ≈ the dialer budget attributes
  it to the hub-stampede / dialer-deadline variant (which the `-persistent-peers` fix independently
  relieves by spreading load and letting the proposer dial out); an instant failure points instead at a
  pin-rejection / teardown. Logging-only; `docs/network-durability.md` §8.
- **`-persistent-peers`: a static, never-evicted consensus-peer tier (#286 Layer 2, dominant fix)**
  (2026-08-12) — The blind field tester root-caused the cross-region genesis stall (with the #327
  `-log debug`): it is a **mesh address-convergence** bug, not a timeout. A proposer had `send with no
  known address` for the other validators, so it could not initiate the attestation gather → no quorum →
  no genesis. Cause: at genesis there is no chain (no discoverable validator registry), all validators
  bootstrapped to ONE seed, and silt's routing table holds bare NodeIDs (addresses live in the transport
  layer, learned only from inbound frames/gossip) — so hub-and-spoke never converges addresses across a
  fresh WAN, and the proposer cannot dial out. Fix (research-directed, `docs/network-durability.md` §8;
  the settled BFT/PoS practice — Tendermint `persistent_peers`, Ethereum static peers, libp2p peerstore):
  **configure the validator/anchor set** as `-persistent-peers ID@HOST:PORT,…` in every validator. Those
  peers are `AddPeer`'d at boot (their address is known up front, no dependence on inbound learning) and
  marked a **never-evicted** tier — a transient WAN miss is a retry, never an eviction, so a proposer can't
  lose an attester mid-formation and stall quorum (Q4). The cloudtest/awstest topologies now pass the whole
  validator set as persistent-peers. Guarded by `TestStaticPeerSurvivesReachabilityEviction286` (a static
  peer survives retry-exhaustion eviction; a normal discovered peer does not). WAN-certified on the next
  GCP run.
- **Gather-path debug logging for the #286 Layer-2 root-cause** (2026-08-11) — The consensus
  propose→gather→attest path previously logged ONLY on a successful commit ("block committed"), so a
  quorum-2 genesis gather that starts and never completes over the WAN (the #286 Layer-2 blocker) was
  invisible — a silent `ValidateProposal` reject of the ~1.5 MB first block looked identical to "never
  received". Added `-log debug` lines on both sides (`core/node/chainrole.go`): the PROPOSER logs
  `gather: starting` (with block SIZE), each `gather: requesting attestation`, each collected/refused/
  failed reply, and `gather: NO QUORUM`; the ATTESTER logs receive → `REJECTED (ValidateProposal)` **with
  the reason** / `REFUSED (already attested)` / `ATTESTED`. So the next multi-region GCP run with
  `-log debug` shows exactly where the round stalls. Debug-level only — no info-level noise, no behaviour
  change (all consensus tests unchanged).

### Fixed
- **Publish no longer dies on a flat wall-clock deadline — async accept + poll (#286 Layer 1)** (2026-08-11)
  — The binding failure in the quorum-2 genesis stall was on the HTTP **publish** path, not the
  validator↔validator RPC path #318 tuned: `silt swarm add` held the HTTP connection open for the whole
  consensus gather under **three stacked flat deadlines** (10 s `http.Client.Timeout`, 30 s server
  `WriteTimeout`, 30 s `chainhost` loop timeout), any of which guillotined the one-time ~1.5 MB genesis
  gather over a real WAN before quorum could form — a flat transport deadline is a category error
  (build-immutable #5; `docs/network-durability.md`). Fixed by making publish **asynchronous**:
  `chainhost.PublishAsync` runs the LOCAL validation synchronously — so every refusal (no publish token
  when required, a durable Publisher identity the refuse-to-surveil chain rejects, a double-spent token, a
  duplicate root) still surfaces at once — then kicks off the commit gather in the background and replies
  **202 Accepted**; the client polls a new `GET /publish-status` until the entry commits or the gather
  reaches a terminal no-quorum. No connection is held open for the gather (no flat guillotine on the
  commit, no slowloris — #48), yet `Publish` still BLOCKS the caller until commit so sequencing
  (double-spend replay, dup) is preserved exactly as the sync path. `/publish-status` exposes the gather's
  terminal outcome, so a not-yet-committable publish (no quorum before mutual standing is earned)
  **fast-fails** instead of polling out the budget — preserving the publish-retry-until-standing semantics
  the bond earned-standing and on-chain revocation e2e flows depend on. Guarded by
  `TestAsyncPublish_AcceptThenPollCommits` / `_RefusalIsSynchronous` / `_TerminalFailureFastFails` and the
  full e2e suite over real TCP. This removes the Layer-1 transport guillotines; the deeper WAN-only genesis
  gather (Layer 2) is still open pending a `-log debug` cloud run (#327) and the structural ~1.5 MB → succinct
  bond proof (#299).
- **Objective chain wedged after the first bond, cross-region (#313)** (2026-08-11) — Found on the GCP
  cert field test: a 3-region objective validator set committed block 1, then every publish hung and the
  chain stalled at height 1. Root cause: **F6 "proposing IS registering" re-embedded the proposer's full
  space-time bond proof in EVERY block** (and the H2 non-proposer path re-submitted it every sweep). Once
  a validator's bond sealed, each subsequent block carried the ~1.5 MB proof (#299); over real
  cross-region links attestation could not carry the bloated block in time, so the proposer's quorum
  gather stalled and the chain wedged. Re-registering an already-committed bond bought nothing — the
  latest registration already stands. Fixed with **`Chain.BondRenewalDue(id)`**: the proposer attaches
  (and a peer submits) a bond registration ONLY when not-yet-bonded or past the TTL renewal point (half
  the TTL, leaving margin), so ordinary blocks stay lean while the release-and-coast defense (RT-2/G4:
  a released plot still decays out on the TTL cadence) is preserved. Also lowers the per-block bandwidth
  residual (#299) and the participation floor (build-immutable #4). Reproduced and guarded in-process at
  the exact cert parameters (4 objective validators, quorum 2, Byzantine-ON, no genesis bonds):
  `TestWedge313_ObjectiveByzantineMultiBlock` (exactly one registration block, not every block) and
  `TestWedge313_RenewalStillHappensUnderTTL` (renewals still fire on the TTL cadence).
- **Size-aware consensus transport deadline (network durability)** (2026-08-11) — The per-attempt DHT/
  consensus RPC deadline (`Config.RequestSizeFloorBytesPerSec`, default 256 KB/s) now scales with the
  OUTBOUND payload: a request gains `len(payload)/floor` of transfer headroom (capped 30 s), so a large
  one-time block (a validator's ~1.5 MB bond registration) gets WAN margin while lean blocks are
  unaffected and holder-fetch dials keep their tighter, non-extended deadline (#277). A textbook-correct
  application of the #289 tenet (transport deadlines must be generous; security lives in the proof, not
  the clock — `docs/network-durability.md`). Guards: `TestRequestTimeoutFor_SizeAware286` +
  `TestRequestTimeoutFor_ExtensionOptOut286`. **This is a genuine durability improvement — but it does
  NOT fix #286 (see Known issues); the GCP re-run proved it tuned a non-binding path.**

### Known issues
- **#286 — quorum-2 objective chain never commits genesis, cross-region (STILL OPEN; RC-gate blocker).**
  The first full 13-node multi-region GCP run and a re-run both FAIL: a fresh 4-validator, **quorum-2**,
  3-region objective chain never commits a genesis block (0 blocks on all four; publishes time out), while
  a single-zone **quorum-1** SMOKE commits in 5 s. The GCP re-run of the size-aware-deadline change above
  showed it does **not** fix this — it tuned the validator↔validator RPC path, not the binding one. Real
  two-layer cause, diagnosed live on GCP: **Layer 1 — the HTTP publish path guillotines the gather** with
  three stacked FLAT deadlines (10 s `http.Client.Timeout` in `adapters/httpregistry`, 30 s server
  `WriteTimeout`, 30 s `chainhost.Host.Timeout`); the 10 s client fires first → `context deadline
  exceeded` → chain never commits. **Layer 2 — a deeper WAN-only genesis-gather defect**: with ALL
  deadlines set to 300 s and full attester reachability, the ~1.5 MB first block STILL doesn't gather its
  2 attestations even given minutes (attesters show zero receive activity at `-log info`). Layer 2 does
  not reproduce in the no-latency sim (`TestWedge313_*` commit these exact params), so it is genuinely
  WAN/scale-specific and needs `-log debug` from boot on a real multi-region run to root-cause. The
  structural close is a **succinct bond proof (#299)** — shrink the 1.5 MB genesis block so the whole
  class dissolves. Fix-in-progress: async publish (remove the Layer-1 guillotines) + gather-path debug
  logging (enable the Layer-2 root-cause). Credit: blind field test #2 supplemental2 re-validation.

### Security
- **seam-5: A-axis truth-in-labelling + a count/entropy signal for the equal-bond split** (2026-08-09) —
  Two red-team hardening findings on the operator-clustering heuristic. **(F3, truth-in-labelling)** two
  `core/chain` comments claimed the declared failure-domain is "transport-cross-checked at H5-B / refuses
  to route to a validator whose declared domain does not match its observed /24" — but `handle()` learns
  `peerDomains` from gossip **verbatim, with no /24 cross-check**. The comments are corrected to say the
  domain is **self-asserted, not transport-verified**; the composition never relied on the cross-check
  (the shed gates on `min(NakamotoOperators, NakamotoDomains)`, so free domains can only *lower* the min,
  never trip the wheels off early), so this is a labelling fix, not a mechanism change. **(F1, new signal)**
  an equal-bond **split** — one operator posting N identical min-bonds across N keys — drives HHI→1/n,
  Gini→0, TopShare→1/n, so it reads *maximally decentralized* on every weight-concentration signal and the
  ⅓ whale alarm never fires. Added **`C2.WeightUniformity`** (effective participants `1/HHI` over actual,
  →1 for perfectly uniform) — the count/entropy companion that exposes the "many atoms, implausibly
  uniform" fingerprint the weight signals miss, surfaced in the daemon C2 status with an *atomization
  note* when a many-bond set reads implausibly uniform with no whale. Necessary-not-sufficient (a
  size-varying splitter evades it, and healthy decentralization is also uniform), so it does not close the
  honest-whale / M_est residue (#182) — it makes the naive split *legible* for out-of-band verification.
  Regression: `TestC2Metric_WeightUniformityCatchesEqualBondSplit`.
- **seam-2: an untrusted objective validator refuses to start without cold-start scaffolding** (2026-08-09)
  — The same blind red-team pass found that a stock untrusted objective validator (the default M0 path)
  shipped with `-anchors`/`-mature-validators` unset, so `Mature()` returns true at genesis
  (`MatureValidators<=0`), the node **latches `everMature` at the first block**, and the anchor co-sign
  the young regime relies on **never engages** — a young or Sybil quorum could self-certify mature and
  capture. Only a *liveness* WARNING guarded it. This is the "fixed but off by default" meta-pattern
  Invariant B exists to catch (the enumeration had **no cold-start row**). Fixed: the daemon now
  **refuses to start** (like the existing `-min-bond<=0` hard failure) unless the operator supplies
  either the anchor **launch set** (`-anchors …` + `-mature-validators N`, to bootstrap a fresh network)
  or a **weak-subjectivity checkpoint** (`-ws-checkpoint HEIGHT:HASH`, to safely *join* an already-mature
  one). Refuse-to-start is *forced*, not merely prudent: there is no sound synthesizable anchor set
  (weak-subjectivity irreducibility — you cannot bootstrap trust in the validator set from the validator
  set). Locked by a new **Invariant-B S6 row** (`coldStartScaffoldOK`). Off the untrusted objective path
  (trusted `-min-rep 0` / legacy `-objective=false`) nothing changes.
- **BREAK 2: the shipped desktop client now defaults its eclipse defenses ON** (2026-08-09) — A second
  blind red-team pass found that `silt client` built its node from raw `node.DefaultConfig()`, where the
  H5-B eclipse-resistance defenses ship OFF (`DHTDomainCap = 0`, `RequireSignedProviders = false`) —
  even though the daemon and the `swarm add`/`get` fetcher both default them on. So a routing-layer
  censor owning the NodeIDs closest to a root's keys but sitting in one failure domain (a ~$4 /24
  key-surround) could make that root **undiscoverable** for a client user who consented to *no* takedown
  — a discovery-layer route to the "make a specific root unfetchable" outcome immutable #5 forbids at the
  denial layer. Fixed: the client now builds from `clientNodeConfig()` (domain-diversity cap on, signed
  provider records required, freshness TTL) and signs its own records. The safe config is now the DEFAULT
  for the untrusted client posture — locked by a new **Invariant-B S5 row** (`invariant_b_test.go`) so it
  can't silently regress. The eclipse mechanism itself was already proven in `redteam_h5b_test.go`; this
  closes the shipped-default gap. (The *multi-domain* surround residual — a censor spread across enough
  failure domains — remains the owned survivor-Nakamoto/#180 residual, tracked separately.)
- **seam-7: equivocation is slashed on DETECTION, not only on adoption** (2026-08-09) — The red team
  found a validator could double-sign onto a *losing* fork — attesting the canonical head AND signing a
  conflicting block on a doomed/lighter fork (to confuse late joiners, split gossip, or bait a partition)
  — at **zero standing cost**, because `slashEquivocators` ran only when a node RECONCILED ONTO a heavier
  competing fork. A fork nobody adopts was never scanned. Fixed in `SyncChain`: every fetched peer chain is
  now scanned against the local one for cross-fork double-signs **before** the heavier test and regardless
  of whether we adopt it — a provably-guilty signer is slashed even if its fork loses. The evidence is
  self-verifying (`chain.VerifyEquivocation`), so an honest sequential signer is never caught; the change
  subsumes the old adopted-branch scan. Regression: `TestSeam7_LosingForkEquivocatorIsSlashedOnDetection`
  (A holds a heavier chain, B serves a lighter fork carrying the culprit's conflicting signature; A does
  not adopt but slashes). *(The companion F2 — applying the eviction to the local objective set on a
  gossiped proof before a slash block commits — touches objective fork-weight uniformity between replicas
  and is deferred as a separate, carefully-scoped change.)*
- **R-2: the SurvivorNakamoto non-globality scalar ships (raw), doc corrected** (2026-08-09) — The
  red team found `docs/safety-denylist.md` read as if the non-globality metric were shipped ("a
  *checkable quantity*"), but no such computation existed in `core/` — only the raw CT log. Per a
  research consult (R-2), the **raw scalar** is a build target (the data — signed provider records +
  gossiped failure-domain labels — already exists); only the ZK/PIR privacy wrapper is post-M0. Built
  **`Node.SurvivorNakamoto(key)`**: the survivor Nakamoto-coefficient over a key's live, accepted
  provider set = the number of DISTINCT failure domains those providers sit in — how many independent
  domains a censor must eclipse to make the content undiscoverable. A set spread across N domains reads
  N; the same providers collapsed into ONE read 1 (one key-surround from dark) — the censor fingerprint
  the raw provider *count* hides. The provider-resolution path now **logs a collapse** (several providers
  all in one declared domain) so silent routing censorship (the BREAK 2 residual) becomes a measurable
  event. Corrected `safety-denylist.md` to state exactly what ships (raw scalar in M0) vs. what is post-M0
  (the certified, domain-hiding ZK-threshold + PIR-probe wrapper, H9/#180). Necessary-not-sufficient
  observability, never enforcement. Regression: `TestSurvivorNakamoto_CountsDistinctFailureDomains`.
- **seam-6: the on-chain bond-renewal nonce is documented as predictable (bounded elsewhere)** (2026-08-09)
  — The red team noted (a *note*, not a break) that `BondRegNonce = H(prev_block_hash)` is predictable:
  once `prev` commits a validator knows its next on-chain renewal challenge, so the on-chain path alone
  doesn't bound release-and-recompute-just-in-time. It **cannot** be made unpredictable without a
  randomness beacon (M0 has none) and **must** stay a pure function of committed history so every replica
  re-derives it identically for objective verification — so this is truth-in-labelling, not a mechanism
  change. The `BondRegNonce` comment now names the weakness and where it is bounded: the parallel **live
  peer-audit** issues an *unpredictable* nonce at random, and that audit now carries the
  **`BondMaxAnswerLatency` reply-deadline** (BREAK 1 / owned-residuals A5), so a released prover that must
  recompute past the ~0.25 knee fails it. (The demand→standing **firewall tripwire** the same pass asked
  to preserve is already regression-locked — `sim/demand_costtowash_test.go` + `sim/demand_bonded_test.go`
  assert standing is byte-identical under wash/self-dealt demand, and `core/credit/invariant_a_test.go`'s
  reflection guard fails the build on any unclassified standing press.)
- **BREAK 1: C1 restated to `(1−ε*)` with an enforcing bond-answer-latency gate** (2026-08-09) — A blind
  red-team pass found a **partial-storage recompute** discount on C1: on silt's single-layer DRSample
  bond graph a prover can delete a fraction ε of its plot (keeping the 32-byte leaves) and **recompute**
  any challenged block on demand, passing the exact `bond.VerifySpaceTime` the live wire runs while
  holding only `(1−ε)` of the disk. Recomputed bytes are **content-identical** to stored ones, so no
  content check can catch it (verified in code) — enforcement is necessarily the **time** leg. Measured
  on the shipped graph: recompute is ~free at ε≤0.10 and its work explodes past the **~0.25 knee**. Per a
  research consult (web-verified against the proofs-of-space literature), the tight small-`ε*` close is
  **H-track** (stacked tight-PoS + a Groth16 SNARK over a ~100 MB witness → a trusted setup), so M0 ships
  the honest **Option B**:
  - **C1 restated** from `(1 − o(1))·q·C_honest` to **`(1 − ε*)·q·C_honest`, `ε*=0.20` disclosed**
    (`m0.md`, `owned-residuals.md` A5, `m0-sybil-rebind §8.1`).
  - **Enforcement:** a reply-latency gate on the live bond challenge — `node.Config.BondMaxAnswerLatency`
    / daemon `-bond-answer-latency` (default 1.5 s) — earns no standing for a reply slower than the
    (generously-margined) deadline; past the ~0.25 knee the recompute blows it. **Soft** (wall-clock ⇒
    fastest-evaluator-sensitive), off in the sim (tick clock), on in the daemon.
  - **Honest residual (A5):** it deters the rational **serial** disk-saver; a **parallel** adversary can
    hold less disk but **re-pays the recompute every audit, per identity** (compute-for-storage
    re-pricing, not a free discount), with the parallelism required growing **super-exponentially** in ε
    (Brent: ~10² cores at ε≈0.25, ~10⁵ at 0.30, ~10¹³ near 0.5) — so realistic parallel exposure ≈ ε0.30,
    and **audit frequency** (`-bond-audit`) is a free tightening lever. Composes with BondTTL so the gate
    bounds even on-chain objective weight over time. The tight close is Option A (H-track).
  - Regressions: `TestBreak1_LateBondAnswerEarnsNoStanding` (a late answer earns no standing; the gate is
    off at deadline 0). No deterministic content check exists — do not add one.
- **R-3: publish-token signers are chosen by a network-canonical ledger ordering** (2026-08-09) — The
  red team found `swarm add -token-quorum` signed a publish token from an **arbitrary** subset of the
  caller's `-peers`, and since the committed `PublishToken.Sigs` records each signer's NodeID, a
  distinctive subset could collapse a publisher's anonymity set toward a singleton (full deanonymization,
  no broken crypto). Per research consult R-3, the fix is a **canonical, ledger-derived** signer set —
  the SAME for every publisher. Added `MsgGetCanonicalIssuers`: a chain-holding validator serves its
  deterministic canonical issuer ordering (validators ranked by committed bond, `chain.CanonicalIssuers`,
  which existed but was unwired). `swarm add` now fetches it and ranks its reachable validators by it
  (`rankByCanonical`), so the signer subset is no longer a per-publisher choice; falls back to `-peers`
  with an honest warning if no peer serves a chain. Locked by **Invariant-B S7**
  (`TestInvariantB_S7_PublishSignerSetIsCanonical` — the selection is order-independent and follows the
  canonical ranking). **Owned caveats (owned-residuals B4):** holds *subset-anonymity only* (the fetcher
  IP/timing channel is the separate D-PRIV residual); a canonical quorum is a mild publish-liveness
  surface (mitigated by rotation-by-bond); and a chainless publisher ranks its *reachable* peers, so the
  hold is fully global only when publishers connect to the canonical set. The full crypto close (the
  issuer signs without learning which root) is the **B2 blind publish token**, H8/#179.
- **F-3: the whole-registry `GET /all` dump is off the public mux** (2026-08-08) — Completes the
  red-team F-3 fix. `/all` serialized the entire registry O(N) with no pagination — an unbounded
  per-request cost. An interim change priced it by work, but that only bounds cost *per source*; a
  **distributed** dump (one request per source IP) no per-IP counter can touch. Since `/all` is used
  only by an operator's own CLI/UI — which reads the registry **in-process** (the daemon's local
  `chainhost`/`fileregistry`), never over this wire — it is now simply **not served on the public
  mux**: a remote client gets `404` / `ErrAllNotServed` and degrades to per-root `/lookup`; an
  operator listing its *own* registry is unaffected. This deletes both the amplification and the
  distributed variant. Also hardened the rate-limiter's per-IP **bucket map with a hard size cap +
  sampled-LRU eviction** so a source-IP-cycling flood can't grow it without bound (it would otherwise
  be its own cost vector). Regressions: `TestBucketMapIsBounded` + the round-trip test now asserts
  `/all` is not served. (The interim work-pricing `charge()` is removed as superseded.)
- **F-1: the maturity shed is now a genuine one-way ratchet (anchors never re-arm)** (2026-08-08) —
  A blind red-team pass (re-)found that the launch-anchor "training wheels" were gated on the **live**
  `Mature()`, which recomputes decentralization from the *current* bonded set — so an honest whale
  growing **real** bond past ⌊total/3⌋ could flip a matured chain back to immature and **re-arm the
  zero-bond anchors**, either halting the chain (if the anchors were gone) or handing them permanent,
  standing-free power (contradicting immutable #3, *no permanent center*). Fixed as a bundle:
  - **One-way `everMature` latch** — the anchor requirement (and anchors' bond-free eligibility) is now
    gated on whether the network has *ever* matured, a replay-derived **consensus fact** (latched in
    `apply`, re-derived on reload, carried across a reorg). Once matured, the anchors never re-arm.
  - **Real-bond super-quorum de-maturation fallback** — if a matured network later drops below the
    decentralization bar, a commit needs a **real-bond super-majority** (≥⅔ of live bonded weight, no
    anchor sign-off) instead of the retired anchors — center-less liveness that preserves accountable
    safety (`ErrDeMatureQuorum`).
  - **Weak-subjectivity checkpoint** — silt is now explicitly weakly subjective; a fresh/long-offline
    node pins a recent trusted block with `-ws-checkpoint HEIGHT:HASH` and **refuses any reorg at or
    before it** (`ErrPreCheckpointReorg`), the long-range-attack defense that makes the latch safe. The
    daemon prints `checkpoint: HEIGHT:HASH` for its committed head so operators can publish/cross-check it.
  - The two residuals are **owned, not hidden** (`docs/design/m0.md` §10): a bounded, socially-recoverable
    re-centralization risk (the honest whale — the same trade Ethereum/Cosmos/Bitcoin made) and the
    weak-subjectivity dependency itself. Regressions invert the red-team PoC (both halt and
    permanent-center horns killed; super-quorum enforced; long-range reorg refused).
- **C2 concentration: the address-diversity (A) axis + an out-of-band honest-whale alarm** (2026-08-08) —
  Two follow-ups to F-1, hardening the residual it deliberately leaves open (the honest whale — real bond
  concentrated by a real operator, unclosable on-chain by theorem, Kwon):
  - **A axis wired into the shed.** A validator's failure-domain (`-domain`) is now **committed in its
    bond** (`BondReg.Domain`, backward-compatibly signed) so the concentration metric counts
    **address-diverse** participants: bonds sharing a declared domain aggregate into one group
    (`NakamotoDomains`), and the maturity shed gates on `min(NakamotoOperators, NakamotoDomains)` — so a
    stake split across many keys in ONE domain cannot fake decentralization; retiring the launch anchors
    needs distinct domains, not just distinct keys. Turns the flat operator-margin `M` into a
    per-network-position cost. Honestly **weak** (a domain is declared, transport-cross-checked, not
    proven; /24s are rentable) — it *prices* concentration, it does not *close* it. With no `-domain` set,
    behavior is identical to before.
  - **Concentration alarm.** `C2Metric` now also reports **HHI**, the **Gini** coefficient, and the
    top bond's share; the daemon narrates them and raises a `⚠ CONCENTRATION ALARM` when one bond holds
    ≥ ⅓ of bonded weight — a social/operational veto, explicitly not on-chain enforcement.

  Regressions: `TestC2Metric_AddressDiversityGate` (same-domain splitting doesn't shed; distinct domains
  do; unset domains unchanged), `TestBondRegDomainSignatureBackwardCompatible`, `TestC2Metric_ConcentrationSignals`.

### Changed
- **`-revoke` now tells the operator it is waiting, instead of looking silently inert** (2026-08-10) —
  Field-test finding #235. `-revoke <root>` does not act immediately: it polls until the named root is
  committed on-chain and this validator has earned standing to gather a takedown quorum. With no output, a
  bogus or not-yet-committed root left the daemon looking hung. It now prints, at start, `revoke: target
  <root> — waiting until it is committed on-chain and this validator has standing to gather a takedown
  quorum`, and a one-time `revoke: <root> is committed — gathering a takedown quorum` on the transition
  (then the existing `takedown: proposed …` on success). (#235's repair-sweep item was already covered by
  the `repair sweep complete` line added with the field tests; the pre-format-store item is folded into the
  migration-policy work, #237.)
- **`integration/fieldtest/` → `integration/cloudtest/` — honest naming for the cloud substrate** (2026-08-09)
  — The GCP harness is *the cloud variant* of the field test, so the directory now says so: renamed to
  `integration/cloudtest/`, its orchestrator `fieldtest.sh` → `cloudtest.sh`, and the per-run GCP resource
  label `fieldtest=<run_id>` → `cloudtest=<run_id>` (nuke-by-label filters updated in lockstep, so teardown
  safety is unchanged). "Field test" remains the umbrella concept — local Docker is the free fast net,
  `cloudtest/` is where GCP is the judge (field-test immutable #5). Docs, `.gitignore`, and cross-references
  updated; the stale "lands with PR #209 on a separate branch" note is dropped now that the harness is on
  `main`. No behaviour change — a rename + reference sweep. The `silt_{local,cloud}_fieldtest_<date>.md`
  operator-report names (both substrates are field tests) are intentionally unchanged.
- **Truth-in-labelling sweep + split-defense safe-default — remediating the M0 blind
  red-team + acceptance passes** (2026-08-08) — The reviews found the composition sound (no C1
  discount, no C2 capture, demand→standing firewall holds both directions) but flagged a cluster of
  *docs-ahead-of-code* overclaims and two documentation gaps. Corrected:
  - **The time (T) axis is relabelled as retention-only.** `Reputation()` has no acquisition-time
    term (`firstSeenTick` is recorded but read by no standing calc), so full standing is granted on
    the first passing bond challenge and acquisition is priced by **D alone**. The docs (`m0.md` §3
    & §4, `TENETS.md`, `core/credit/credit.go`) previously asserted T was a live acquisition factor
    ("cannot buy last month's uptime"); they now state T ships for *retention only* (decay/TTL) and
    that a time-acquisition ramp is deferred (a bare age gate is pre-farmable — the coin-age
    anti-pattern; the only sound form is a continuous bond-anchored VDF, M1+).
  - **`GET /all` registry read-cost is now priced by work, not per request** (F-3). A per-IP token
    bucket that charges one token regardless of endpoint metered the wrong quantity — `/all`
    serializes the whole registry O(N) for the same token as a 183-byte `/lookup` (~20,000×
    amplification at N=20k). `/all` now additionally charges ~one token per 64 entries served,
    draining the source's bucket into bounded debt, so a single caller can't repeatedly amplify one
    token into a full-registry dump. Regression: `TestChargePricesAllByWork`. (A *distributed* `/all`
    flood and full cursor pagination remain post-launch — the #48 entry now says so.)
  - **The C2 operator margin M is safe-by-default.** `-operator-margin` now auto-arms to a
    conservative `M>1` for an untrusted (objective) validator — exactly as `-min-bond-floor` and
    `-byzantine-quorum` already do — instead of shipping `M=1` (zero protection against one operator
    splitting real stake across NodeIDs to fake the decentralization that sheds the launch anchors).
    An explicit `-operator-margin 1` still opts out for a trusted/single-operator swarm. M stays an
    honest heuristic (unverifiable on-chain, #182). Regression:
    `TestOperatorMarginDefaultsAboveOneForUntrustedValidator`.
  - **The seam-4 demand-receipt one-liner (`m0.md`) no longer reads as closed.** Two residual leaks
    (a receipt is forgeable with zero object bytes; a bonded-mode receipt links fetch→standing key)
    are neutralized *today* by the firewall (demand has no consensus consumer) but must be closed
    before any demand→standing fusion — now stated as such.

### Fixed
- **Holder-fetch dials fail fast again — no retry/backoff regression on the dial-storm** (2026-08-10) —
  Follow-up to the adverse-network hardening above. That change applied the new 5 s timeout + retries to
  *every* RPC, including speculative holder-fetch dials (`MsgFetchChunk`/`MsgHasChunk`), so a fetch from a
  dead holder cost up to `(retries+1)·5 s ≈ 20 s` (was ~½ s) — deepening the dead-holder dial-storm (#277)
  exactly where content lives on churning holders. Fixed: holder-fetch RPCs are **not retried** (the fetch
  loop already retries at a higher level via `FetchAttempts` and skips known-dead holders via `deadUntil`)
  and use a **tighter `-holder-dial-timeout`** (default 2 s) so a dead holder fails fast, while mesh/
  consensus RPCs keep the generous timeout + retries needed to ride out jitter. Restores the pre-hardening
  fetch responsiveness without giving up the jitter durability.
- **Consensus now bootstraps under a jittery network (adverse-network durability)** (2026-08-10) —
  New `integration/flakynet` harness (4 objective validators behind `tc netem`) reproduced a real
  durability collapse the clean local tests never showed: under a mild, realistic 80 ms ± 20 ms **jitter**
  the network committed in 6 s on a clean link but **never** committed jittered (the root of the flaky-GCP
  `#286` symptom). Three causes, fixed: **(1)** `RequestTimeout` was **500 ms** — LAN-tight for a global
  P2P RPC (TCP connect + query routinely exceeds it on a jittery path); the daemon default is now **5 s**
  (`-request-timeout`). **(2)** A *single* timed-out RPC evicted the peer from the routing table
  (`table.Remove`), so one slow/dropped packet tore a good peer out of everyone's mesh; timed-out RPCs are
  now **retried with exponential backoff** (`-request-retries`, default 3; `-request-backoff` 250 ms) and
  the peer is only given up after retries are exhausted. **(3)** A `BondChallenge` reply carries a large,
  slow space-time proof; under adversity these time out in droves and (2) then evicted the peer — starving
  consensus of the very standing it was establishing. A bond-challenge timeout now **never evicts** from
  routing (it is a *standing* signal, not a *reachability* one — standing lapses and re-audits on its own).
  `DefaultConfig` keeps `RequestRetries=0` so the deterministic sim/tests are unchanged; the daemon opts in.
  KNOWN HARDER GAP (filed): jitter **+ packet loss** still does not reliably bootstrap — loss on the large
  proof/chunk replies plus the C1 reply-latency gate (`-bond-answer-latency`) reading network latency as a
  short-storage cheat; the latter is a security tradeoff needing a deliberate call.
- **A node that joins before its bootstrap peer is listening now recovers on its own** (2026-08-10) —
  Field-test finding #281, found on the first real 13-node cross-region GCP run. silt's Kademlia join
  (`Node.Bootstrap`) was **one-shot**: on a multi-node cold start with no ordering guarantee, three
  validators started their `FIND_NODE` before the boot validator's listener was up, landed with an
  **empty routing table**, and — with no re-bootstrap — never tried again, even though the target became
  reachable seconds later. The network never meshed, the chain stayed at height 0, and every publish
  timed out (reachability was fine; consensus never formed). Added a **periodic self-heal**
  (`core/node/bootstrap.go`, `StartBootstrapRetry`): while `Table().Size() == 0`, re-run the join against
  the original `-bootstrap` seeds every `BootstrapRetryInterval` (new `-bootstrap-retry` flag, default
  15s; 0 disables). The retry first clears the seeds from the `deadUntil` negative cache (their failed
  initial dial had marked them dead for `HolderCooldown`), so it re-dials immediately instead of waiting
  out the cooldown; it is a no-op once any peer is in the table. Recovery logs
  `re-bootstrapped: recovered from an empty routing table (N table entries)`. Covered by unit tests
  (`simnet` Kill/Restart race) and a real-process e2e (`TestBootstrapRetryRecoversColdStartRace`).
- **A sparse cold-start mesh now converges (periodic bucket refresh)** (2026-08-10) — follow-up to #281.
  Recovering an EMPTY routing table is necessary but not sufficient: a node can re-bootstrap to just one
  or two peers and stall there, and the seedless boot validator (no `-bootstrap`) never re-looks-up at
  all — so it stays stuck at the single entry an incoming dial gave it and can't discover the rest of the
  validator set. The bootstrap-retry loop now also does a Kademlia self-lookup while the table is below
  `BootstrapWellConnected` (default 8), which discovers more peers and converges the mesh. This reaches
  the boot node too — it queries the peers that dialed IN. Covered by a `simnet` convergence unit test
  (a boot-like node with one entry converges via refresh). NOTE: this fixes the DHT-mesh convergence; a
  *separate* consensus-bootstrap gap remains where a fully-simultaneous cold start of an objective
  validator set does not establish anchor standing even once connectivity is fine (filed separately).
- **Repair no longer starves on stale records to dead holders** (2026-08-09) — Field-test finding F2
  (`integration/churn/`): the repair fetch loop (`fetchStripeByColumn` → `fetchFrom`) dials providers
  serially, each dead holder costing a full `RequestTimeout`, and a single timeout re-sweeps the whole
  provider list up to `FetchAttempts` times — with nothing skipping a holder we just failed to reach
  (`n.reachable` was write-only). Routing-table eviction doesn't remove the *provider record* other
  nodes still hold, so the next lookup resurfaces the same corpse and re-dials it at full timeout every
  sweep; under churn one stripe could exceed `RepairInterval` on timeouts alone. Added a **failed-holder
  negative cache**: a request timeout stamps `deadUntil[peer] = now + HolderCooldown` (30s), and the
  fetch/repair dial path skips a holder still in cooldown — but **only when a live alternative exists**
  (`anyLive`), so the cache can never be the reason a fetch fails (a sole provider that timed out
  transiently and has since re-announced is still dialed, preserving cross-NAT reprovide, #69). Stamped
  centrally (so `NetGet` benefits too) but consulted only in the fetch path, leaving consensus/DHT
  re-probes untouched; the map is bounded (`maxDeadHolders`). Regressions in `fetch_deadcache_test.go`.
- **The dead-holder negative cache now also covers the repair PROBE path and the DHT walk** (2026-08-09) —
  Follow-on to the fetch-path fix above, which stamped `deadUntil` centrally but consulted it only in the
  fetch dial path, leaving the two other places a repair sweep dials stale records exposed: (1) the
  dispersion audit's `probeShard` `MsgHasChunk` loop and (2) the provider-discovery **DHT walk**
  (`walk.step`, `MsgGetProviders`), which a caretaker runs for *every* key it cares about. Under churn a
  caretaker with many stale routing/provider records to `docker kill`ed holders spent a full
  `RequestTimeout` per dead record, so a single sweep never completed — the caretaker never registered the
  loss and never repaired (surfaced by `integration/churn/`, which stalls on small swarms; the true
  degradation happens at GCP scale where k=10 actually strands stripes). Both paths now skip a holder still
  in cooldown — the walk fails it in the Kademlia lookup without a dial, the probe skips it — each **only
  when a live alternative exists** (`anyLive`), so a sole transiently-timed-out holder that has since
  recovered is still dialed (#69 preserved). Regressions in `repair_deadcache_test.go` (walk-skips,
  walk-without-cooldown control, probe-skips). A dial-storm reduction — necessary but, as the next entry
  found, not the whole churn story.
- **Repair-under-churn now actually completes: parallel shard probing + a visible sweep (#235)** (2026-08-09) —
  The `integration/churn` field test stalled: a caretaker killed holders never repaired. Root cause, found
  by instrumenting the sweep: `repairRootWithLayout` probed every shard **strictly serially**
  (`probeNext(i+1)` inside each probe's callback), so once holders die each dead-holder dial costs a full
  `RequestTimeout` **in series** — a large file's sweep can't finish within a `RepairInterval`, the
  caretaker never reaches `repairStripes`, and nothing is ever repaired. The healthy first sweep completed
  only because every dial returned in ~ms. Two fixes: **(1)** the probe phase now fans out with bounded
  concurrency (`repairProbeConcurrency`), so dead-holder timeouts overlap and a sweep completes in seconds
  under churn (safe on the single-threaded event loop — probe callbacks mutate sweep state on the loop, the
  same model as the DHT walk's in-flight fan-out); **(2)** the sweep is no longer **silent** — it logs
  `repair sweep complete` with the reachable-shard count, and its previously-invisible no-op early-returns
  (`registry lookup failed`, `manifest not yet reassembled`, `layout not loadable`) now log why, so a
  caretaker that can't sweep is diagnosable instead of looking identical to a healthy one (**#235**). With
  this, the caretaker reconstructs stranded stripes from parity and re-scatters, and `integration/churn`
  passes honestly (reachable drops after a kill → `stripe repaired` → bit-perfect re-fetch).
- **`silt swarm add -replication N`** (2026-08-09) — Expose the placement replication factor (previously a
  compiled-in 3) as a publisher flag; parity across holders backstops copies, so even 1 is viable. Lets a
  small-swarm test strand a column with a single kill, which makes caretaker repair **deterministically**
  reproducible on a laptop: `integration/churn` now runs at `REPLICATION=1` as a fast, deterministic gate
  and documents `REPLICATION=3 HOLDERS>=50` on the GCP harness as the faithful, shipped-default variant.
- **Acceptance-pass documentation gaps** (2026-08-08) — From the fresh-operator acceptance pass (all
  nine flows worked; these were doc/test issues, not broken capabilities):
  - `docs/user-seam.md` Role 4 "become a validator" walkthrough errored as written — the default
    objective fork-choice path needs `-anchors` to bootstrap a young network's on-chain bonded weight,
    which the walkthrough omitted (`bonded 0, needs …`). It now passes `-objective=false` to match the
    cited test `TestBondEarnedStandingCommitsOverTCP`, with a note on the objective/anchor launch path.
  - `README.md` said the default add mode was "convergent"; the default is `-mode private` (H6). Fixed.
  - `examples/flow8-takedown.sh` published with the (now private) default, giving two *different* roots
    so the takedown test denied one root and confirmed an unrelated one still served. It now publishes
    `-mode convergent` so both operators hold the **same** root, actually demonstrating per-operator
    takedown of a shared root.

### Changed
- **The C1 bond reply-latency gate is now SOFT, not a standing gate (build-immutable #3; #289)** (2026-08-10) —
  A live bond challenge no longer denies standing for a slow reply. Reply-latency is transport (RTT + jitter +
  loss) **plus** compute, and gating security on the sum is unsound on the open internet — it read network
  jitter/loss as a partial-storage cheat and starved durability under adverse networks. Standing now rests on
  the sound signals only (anti-release floor + identity binding + the space/labeling proof `VerifySpaceTime`);
  a valid answer earns standing however slowly it arrives. The partial-storage timing deterrent becomes a
  **soft, disclosed** signal: the node tracks the windowed-MINIMUM (low quantile) of each peer's bond-challenge
  reply latencies — which filters the one-sided network noise — and raises a non-gating suspicion only when
  that floor is SUSTAINED above `-bond-answer-latency` (a partial-storage prover recomputes on every challenge
  so its floor stays elevated; an honest node on a bad path is only randomly slow). New read-only accessor
  `Node.BondLatencyFloor`. The old hard-gate regression test is inverted to assert the sound behavior
  (`TestC1TimingIsSoftNotAHardGate`). The hard structural close remains tight-PoS (H-track), owned as residual
  A5. Following the 2026-08-10 network-durability research opinion.
- **Anti-release bond floor decoupled from the transport timeout (build-immutables #3/#4)** (2026-08-10) —
  Following the network-durability-vs-space-time research opinion, the anti-release floor `MinBondBytes` is now
  sized explicitly against a named **compute** window (`AntiReleaseComputeWindow`, ~2s) times the measured seal
  rate (`bond.PlotSealThroughput`, ~270 MB/s), *not* the transport `-request-timeout`. `DerivedBondFloor` is a
  derivation (2× margin over window×throughput ≈ 1 GiB) rather than a magic constant, and a regression test
  (`TestAntiReleaseFloorIsComputeSourcedNotTransport`) locks it to the compute arithmetic. This means raising
  `-request-timeout` for durability under adverse networks (#288) can never balloon the floor toward multi-GiB
  and price out small validators (immutable #4), and the anti-release argument no longer rests on a network
  reply deadline (immutable #3; enforcement is the floor + bond-audit statistics-over-history, since a full
  re-seal is a multi-second cost). No change to the default floor value; flag help and comments corrected.
### Added
- **`silt daemon -goodpropose <peerID>` — a POSITIVE CONTROL for the `-forge-block`/`-lowbond-propose` rejections** (2026-08-11) —
  The `integration/redteam` forged-block and low-bond scenarios asserted only that H3 replied `OK:false`, but a
  validator replies `OK:false` for many unrelated reasons (chain role disabled, `ErrWrongParent`,
  already-attested), so a target that refuses **every** proposal would false-pass both — "reject the good one
  too" is indistinguishable from "reject the bad one" (audit #303). New test-harness flag `-goodpropose <peerID>`
  (sibling to `-forge-block`/`-lowbond-propose`), backed by `core/node.ProposeGoodBlock`, sends a **well-formed,
  properly-bonded** proposal and asserts the target **ACCEPTS** it — logging `goodpropose proposal ACCEPTED by
  <id>` on attest, `goodpropose proposal UNEXPECTEDLY REJECTED by <id>` otherwise (it retries until its bond
  earns standing). The redteam harness now gates SCENARIO 2 & 3 on this positive control, so a rejection is
  attributed to the real defence, not a dead/wedged target. (#303)
- **PoR audit seam on `silt daemon` — `-liar` + `-audit` make the verify-without-fetch catch+slash wire-driveable** (2026-08-10) —
  The `silt sim run audit` headline (a verify-WITHOUT-fetch PoR challenge catches a storage node that kept
  its proof tags but dropped the bytes, and **slashes its standing**) existed only in-process: `Node.SetLiar`
  and `Node.Audit` were unit/sim-tested, but nothing in `cmd/silt` toggled the liar or invoked the sweep, so
  over the wire a liar was caught only *indirectly* (it answers `MsgHasChunk=false` once its bytes are gone).
  Two new flags (siblings to the consensus red-team flags `-equivocate`/`-forge-block`/`-lowbond-propose`):
  **`-liar`** (keep the tags, advertise as a provider, drop the bytes, prove over data it no longer holds)
  and **`-audit <interval>`** (a `-care`-ing caretaker runs `Node.Audit` on every cared root — challenge each
  shard's providers, grade the proofs against the care-link key with no ground-truth fetch, and slash the
  liar). `integration/audit` now gates the literal claim over the wire (honest holders pass, the liar is
  caught and slashed) and demonstrates *why* it is needed — the liar's `MsgHasChunk` lie fools the
  availability probe but not the audit. (#232)
- **`swarm add` token-replay seam (`-save-token` / `-use-token`) drives the double-spend rejection over the wire** (2026-08-10) —
  The publish-token double-spend guard (`core/chain` `ErrTokenSpent` + the online issuer's spent-set) was
  real and unit-tested but had no CLI/wire seam: every `swarm add -token-quorum N` minted a fresh random
  serial, so two publishes never collided. `swarm add` now takes **`-save-token <file>`** (write the acquired
  token, CBOR) and **`-use-token <file>`** (publish carrying that saved token instead of minting a fresh one).
  Re-presenting the same token for a second file re-uses its already-committed serial, which the registry's
  pre-check refuses with the exact `ErrTokenSpent` reason and never commits. `integration/economy` now gates
  the double-spend over the wire (a fresh-token control still commits) and ties its unlinkability assertion to
  the tokened entry's *own* zero-NodeID Publisher. (#233)
- **Cloud variants of the field-test series in `integration/cloudtest`** (2026-08-10) — Four new GCP
  scenarios carry the local suites' properties onto real VMs / real regions, mapped onto the existing 13-node
  topology with **no topology change**: `flow_publisher_unlinkability` (privacy #3 — a durable-`Publisher`
  publish is refused by the default chain over the real wire), `flow_durability_turnover` (durability #2 —
  content survives a **permanent** storage-node departure, fetched bit-perfect from a survivor),
  `flow_chaos_crash` (chaos #7 — a **SIGKILL**ed storage node re-announces its chunks (#69) via
  `Restart=on-failure` and content stays fetchable), and `flow_web_ui_guard` (client/UI #4 — the #89 web-UI
  guard holds on a real VM: no-token→401, DNS-rebinding→403, read→200). Wired into `run_all_scenarios`;
  shell/topology dry-validated (no billable run). **C2-Sybil (#5) has no cloud flow yet** — it needs
  non-anchor Sybil validator VMs (a `topology.py` addition) so the Sybils' bonds bank over a longer cloud run
  and the pure `ErrAnchorRequired` gate + ≥8-bond atomization note become assertable; recorded as a `skip`
  until then.
- **New field test: chaos / crash-recovery (`integration/chaos`)** (2026-08-10) — Tests whether the system
  survives **hard crashes**: a `SIGKILL` (abrupt process death, no graceful shutdown), then a restart of the
  *same* node — same identity, same IP, same on-disk store (`docker start` on an un-removed container, unlike
  `durability`'s `docker rm`). **WAVE 1 (default, the gate):** `SIGKILL` **every** holder, restart, and assert
  each re-bootstraps AND logs `re-announced N held chunks` (#69) so content stays discoverable, then a fresh
  client cold-fetches it back **bit-perfect** — with no crash-loop. Validated locally (PASS: 6/6 holders
  re-announced, bit-perfect after a full holder crash). **WAVE 2 (opt-in, `WAVES=2`):** also `SIGKILL`s the
  **sole** seed/registry/bootstrap; this surfaces a discoverability gap — the content stays on disk but a
  fresh client can't rediscover the providers in the window, and a single holder re-announce doesn't restore
  it — recorded **honestly as an observation to root-cause** (entangled with the single-bootstrap SPOF a real
  deployment avoids; retest with a redundant-bootstrap topology + on the cloud test), **not** a verified
  product defect, which is why it is off by default. Wired into `run-all.sh` (gate tier).
- **New field test: C2 "no quiet capture" under a Sybil validator set (`integration/sybil`)** (2026-08-10) —
  Tests the M0 systemic C2 claim cynically, as an OUTCOME: can a bonded **Sybil validator set** — many
  identities, real bonds, its own quorum — **quietly capture** a young objective network? Two honest anchors
  (`a1` proposer, `a2` co-signer, since one anchor can't co-sign its own block under `-anchor-quorum 1`) plus
  a Sybil set. **C2-a**: with the anchors present the chain commits a real block and the daemon prints
  `wheels engaged (young network — anchor quorum still required)` — the chain is live and the training wheels
  are on. **C2-b**: stop **both** anchors and the Sybil set (s1 proposes, s2 attests) **cannot advance the
  chain — no new block**; the test reports which training-wheels layer refused it (locally the standing gate —
  a young Sybil set can't even earn committed bonded standing without an anchor-proposed block; behind it the
  anchor co-sign gate, `ErrAnchorRequired`). A chain advancing for the Sybils would be a hard FAIL. **C2-c
  (bonus)**: with ≥8 committed equal bonds the C2 metric's atomization note fires (an equal-bond split reads
  as a fingerprint, not real decentralization). Honest scope (immutable #5): on one host the Sybils' bonds
  don't reliably bank on-chain, so the standing gate usually fires first (itself a faithful no-capture
  outcome); the pure anchor-co-sign gate with pre-banked bonds and the ≥8-bond atomization signal are
  exercised at scale on the cloud test. Validated locally (PASS). Wired into `run-all.sh` (gate tier).
- **New field test: the client / web-UI path under an adversary (`integration/client`)** (2026-08-10) —
  Tests the path a real **user** takes — run a daemon, open its web UI, drop a file in, get a link, someone
  fetches it back — over the daemon's **HTTP API**, not the `silt swarm` CLI; and the path a real **attacker**
  takes against that same local API. A `ui` node (storage + its own registry + `-ui`) plus `N` holders; the
  test drives the UI with `curl` from inside the operator's container (the realistic browser-on-the-same-box
  model, and the only way the guard's local-`Host` rule is met). **U1–U3**: `POST /api/publish` (multipart +
  the bearer token grabbed from the daemon's own `ui: …?token=` line) scatters the file, `/api/roots`
  reflects it, and `GET /api/fetch?link=…` returns the bytes **bit-perfect** — the real end-to-end round-trip
  over HTTP. **U4–U8** attack the guard (#89): a no-token and a wrong-token `POST` are refused **401**, a
  DNS-rebinding request (non-local `Host`) and a cross-origin drive-by (evil `Origin`) are refused **403**,
  and a token-free localhost read still returns **200** (ergonomics preserved). Every assertion is a real
  HTTP status code / real SHA-256, never an echoed string; any failure is a hard FAIL (the user path and the
  local-security guard are both load-bearing). Validated locally (PASS, all eight). Wired into `run-all.sh`
  (gate tier).
- **New field test: publisher unlinkability under an adversary (`integration/privacy`)** (2026-08-10) —
  Tests immutable #4 (refuse-to-surveil) cynically, as an OUTCOME: *can an adversary get the network to
  record who published a given file?* A silt registry entry may carry a durable `Publisher` NodeID — a
  permanent file→publisher link on the append-only chain (the #14/F1 privacy corner) — and the M0 default
  refuses it. Two real `silt -validator` daemons form a committing chain; the test asserts on real
  `chain-status` commit counts and the validator's rejection line, never an echoed string. **P0** (positive
  control) shows a chain run with `-allow-publisher=true` *commits* a Publisher entry — so the refusal below
  is real policy, not a broken publish. **P1** shows the default chain commits a normal unlinkable publish
  and fetches it back bit-perfect (privacy isn't a broken product). **P2** (the crux) shows the same default
  chain *refuses* an `-allow-publisher` publish — the real error surfaces over the wire (`chain: entry
  carries a durable Publisher (records permanent linkage; publish unlinkably…)`) and no new block commits.
  **P3** shows a `-token-quorum` publish commits with a *blind* validator credential and no Publisher —
  authorized yet unlinkable (the F1 fix). A privacy regression (the default chain committing a Publisher
  entry, or the private path failing) is a hard FAIL; the metadata-correlation layer is a stated M0 tradeoff,
  not covered. Validated locally (PASS, all four). Wired into `run-all.sh` (gate tier).
- **New field test: durability under permanent holder loss (`integration/durability`)** (2026-08-10) —
  Tests the core promise cynically — *does content outlive the nodes that held it?* A `seed + 16 holders +
  caretaker` swarm publishes a file at `replication=1` (every column single-copy, the honest stress), then
  **permanently kills holders one at a time and never replaces them** so the pool shrinks onto ever-fewer
  survivors. Crucially it kills **without** re-scaling: an earlier `rm -f`+`--scale` design let Docker
  recycle a dead holder's IP to a fresh **empty** identity, and the caretaker dialing `old-NodeID@that-IP`
  hit a TLS-pin *impostor* — a Docker artifact (real infra doesn't recycle IPs in seconds) that masqueraded
  as "content lost." Shrinking the swarm removes the artifact and isolates the real mechanic:
  reconstruct-from-parity (`k=10` of `n=16`) + re-scatter onto survivors. It reads **two separate oracles** —
  the caretaker's `repair below k` log (authoritative, unrecoverable content loss) for **durability**, and a
  warm-peer `swarm get` (every survivor handed as a direct peer) for **retrieval**, so a discovery flake is
  never miscounted as loss. **Finding:** durability **held** — across 10 permanent departures (16→6 holders)
  no stripe ever fell below `k` and the caretaker performed **13 reconstructions**, so the bytes provably
  survived — but a fresh client's end-to-end **retrieval** stayed bit-perfect only down to ~11 survivors,
  then degraded (the **#43 retrieval surface under permanent loss**: *durable is not the same as
  retrievable*). Exits 0 as a FINDING (`EXPECT=pass` to hard-fail; `MIN_SURVIVORS=11` for the
  retrieval-healthy clean PASS). True membership **rotation** (fresh VMs = fresh IPs) is the cloud test's
  job. Wired into `run-all.sh` (slow tier).
- **New field test: retrieval / discoverability at scale (`integration/retrieval`)** (2026-08-09) —
  Measures the most basic user promise — *can I get my content back?* — on a real multi-holder swarm as
  short-lived publisher/fetcher identities churn the DHT (#43/#60). It publishes files, optionally pollutes
  routing with throwaway ephemeral publishes, then measures the **bit-perfect cold-fetch success rate** from
  fresh ephemeral clients and gates it on a floor. Validated locally, it cleanly isolates the cause: raw
  scale (24 holders, no churn) fetches **100%**, but adding 40 churning ephemeral identities drops cold
  discovery to **85%** — a real, honest reproduction of the #43 ephemeral-identity routing degradation
  (`POLLUTERS=0` vs `POLLUTERS=40`). A sub-floor rate is a **FINDING** (exits 0 like `upgrade` reproducing
  #237; `EXPECT=pass` flips it to a hard fail), never a faked green. Wired into `run-all.sh` (slow tier).
- **Blind-session ease: an "LLM instructions" README section + interactive GCP setup** (2026-08-09) —
  The field-test scripts stay simple and emit clean per-test output; the dated `silt_local_fieldtest_<date>.md`
  / `silt_cloud_fieldtest_<date>.md` roll-ups + per-test detail reports are written by the operating agent,
  guided by a new **"For an LLM/agent operator"** section in `integration/README.md` (plus a Quick-start).
  `./integration/cloudtest/cloudtest.sh setup` is now an **interactive** step — it asks for the GCP project,
  walks the user through `gcloud auth` (login + the application-default creds Terraform needs), enables the
  required APIs, and writes `config.env` — so spinning up the cloud test needs no hand-editing.
- **`integration/run-all.sh` + shared `integration/lib.sh` — the clone-and-run field-test experience**
  (2026-08-09) — One driver runs the local Docker suites in sequence (each owns its topology, so they run
  one at a time), captures each suite's real `RESULT:` line + duration, and writes a shareable consolidated
  `report.md` plus a terminal summary; exit 0 iff every suite that ran is PASS or a deliberately-reproduced
  FINDING. Runs the fast gate set by default (`FULL=1` adds the slow `soak`/`upgrade` suites; `SUITES="…"`
  picks a subset). `lib.sh` factors the shared shell — RESULT classification, timing, prereq checks — that
  each suite otherwise re-implements; the suites still stand alone. Suite → M0-claim mapping lives in the
  driver's catalog.
- **GCP field test — an automated multi-machine RC gate** (2026-08-08,
  [#52](https://github.com/nerolabs/silt/issues/52)) — `integration/cloudtest/` spins up a real
  ~13-node silt network across three GCP regions (validators, storage, a `-registry-only` node, a
  relay, a fetcher, a NAT gateway + natted peers, and an adversary), runs the acceptance flows
  (publish → commit → fetch bit-perfect, earned-standing validator onboarding, multi-validator
  convergence, f=1 fault tolerance, restart survival, per-hash takedown, cross-NAT via the relay) plus
  the [#184](https://github.com/nerolabs/silt/issues/184) adversarial consensus drills
  (equivocation→slash, partition→heal, forged/low-bond→reject) **over the real wire**, emits a
  shareable report (`report.md` / `report.html`), and tears the whole network down. Deterministic and
  self-configuring — every peer/anchor/attester reference is computed from `silt id -id-seed` + static
  internal IPs before any VM boots, so there is no discovery wait. Cost-bounded four ways (SPOT
  instances + per-VM TTL self-destruct + destroy-on-exit + optional budget alarm, with a
  nuke-by-label fallback), reusable by outside developers against their own GCP project, and
  `SMOKE=1` trims it to a 4-node run for a pennies-scale plumbing check. This automates roadmap
  [#52](https://github.com/nerolabs/silt/issues/52) and is the standing field-test gate for every
  release candidate.
- **`-registry-only` — the leanest public-registry role** (2026-08-08,
  [#47](https://github.com/nerolabs/silt/issues/47)) — A daemon started with `-registry-only` serves a
  file-backed registry over HTTPS and constructs **no storage node at all** — no DHT, chunk store,
  chain, or caretaker. It sits below `-freeload` (which is still a full routing node that merely refuses
  to host content): a public-infrastructure operator can now run a rendezvous registry at minimal cost.
  The daemon returns to a tiny registry-serving path before any of the node machinery is built. Proven
  over real TLS (`e2e/registry_only_test.go`: a pinned client publishes + looks up an entry, and the
  daemon never announces a routing peer). With `-freeload` (PR #201) this completes #47.
- **Registry read-cost bounding — keep public registries cheap to run** (2026-08-08,
  [#48](https://github.com/nerolabs/silt/issues/48)) — A registry is a costless public good only if a
  single caller can't drive unbounded cost, so the registry HTTP server now enforces a **per-client-IP
  token-bucket rate limit** (generous defaults — 20 req/s, burst 40 — so normal clients never notice;
  sustained floods from one source get `429`) plus **server timeouts** (read-header / read / write /
  idle) against slowloris. Idle rate buckets are pruned on a timer so a caller cycling source IPs
  can't grow the bucket map without bound (the map would be its own cost vector). `GET /all` is
  additionally **priced by work** (see the Fixed entry below) — a flat token bucket alone meters
  request *count*, not the O(N) serialization `/all` does. Covers the read-cost-bounding lever of #48
  for a **single source**; a distributed `/all` flood, full cursor pagination, liveness-pruning of
  dead entries, and federation/sharding remain as post-launch work.

### Fixed
- **Prepaid publish credits were silently dropped over real TCP** (2026-08-08,
  [#179](https://github.com/nerolabs/silt/issues/179)) — `tcpnet`'s hand-rolled wire codec
  (`toWire`/`fromWire`) never mapped the `Credit` field of a `MsgTokenRequest`, so the F4/D3 **fee
  decoupling** (paying for a token with a prepaid blind credit instead of charging the durable
  identity) only ever worked in the in-process sim — over real sockets the credit vanished and the
  issuer fell back to charging the requester. Added `Credit` to the wire struct + `toWire`/`fromWire`,
  with a `TestCreditSurvivesWire` round-trip guard (the exact `#65`-class silent-drop bug
  `wire_por_test.go` warns about). This repairs the publish-token credit path (F4) as well as enabling
  D3 below.

### Added
- **D3 issuance-mixing, slices 1 & 2 — ephemeral-identity + relay-routed token withdrawal** (2026-08-08,
  [#179](https://github.com/nerolabs/silt/issues/179)) — Closes the remaining fetcher-unlinkability
  links in the demand receipt. The token issuer authenticates whoever dials it via the end-to-end TLS
  handshake, so a withdrawal made over a fetcher's durable identity tied that withdrawal to the fetcher
  (the blind signature hid only the token serial, not the network identity). Now `client.
  WithdrawDemandTokenPrivately` performs the withdrawal over a **fresh ephemeral identity** — a one-off
  keypair + transport, torn down on return — paying with a **prepaid blind credit** (`Node.
  AcquireDemandTokenWithCredit`) rather than a durable account, so the issuer authenticates only an
  unlinkable ephemeral key and charges nothing it can tie to the fetcher (**slice 1 — the identity
  link**). Given a **relay-form** issuer address (`relay:R@host:port`) the ephemeral transport dials the
  issuer THROUGH a content-blind relay, so the issuer's inbound connection is from the relay, not the
  fetcher — hiding the fetcher's IP as well (**slice 2 — the IP link**); the end-to-end TLS still
  authenticates the ephemeral key across the relay pipe. Proven over real TCP (`client/privissue_test.go`:
  a mock issuer records the authenticated identity — it is the ephemeral one, never the fetcher's, direct
  and relay-routed; a withdrawal with no valid credit is refused). *Fetcher-unlinkability is now
  cryptographic + identity + IP; timing-correlation (epoch-batching) is deferred to the post-M0 H8
  mixnet.* Whole suite green with \`-race\`; full e2e suite green over real TCP.
- **#184 verify — forged-block→reject and low-bond→reject over the REAL WIRE** (2026-08-08,
  [#184](https://github.com/nerolabs/silt/issues/184)) — The two `ValidateProposal` defences, proven
  against real daemons over TCP: an honest validator refuses to attest a proposal whose **proposer
  signature is forged** (corrupted after signing) or whose **proposer lacks a qualifying bond**. A
  single red-team primitive (`Node.ProposeBadBlock`, behind the daemon's `-forge-block` /
  `-lowbond-propose` flags) sends one crafted proposal to a peer and reports whether it was refused;
  the block is otherwise valid and built at the target's head, so the only reason for refusal is the
  fault under test (a bad signature → `ErrBadSignature`, or an under-bonded proposer →
  `ErrLowReputation`). `TestForgedBlockRejectedOverTCP` and `TestLowBondProposerRejectedOverTCP`.
  These complete #184's four consensus-safety cases over the real wire (with equivocation→slash and
  partition→heal). Whole suite green with \`-race\`; full e2e suite green over real TCP.
- **#184 verify — partition→heal proven over the REAL WIRE (partition control + reorg observability)** (2026-08-07,
  [#184](https://github.com/nerolabs/silt/issues/184)) — The M0 consensus denial "honest replicas cannot
  permanently diverge under a partition" now runs against real daemons over real TCP, not only the
  in-process sim (`sim/reorg_test.go`). A new **`-block-peers` daemon flag** (`Node.SetBlockedPeers`)
  simulates a severed link — the node drops all traffic to/from the listed peers — so a validator can be
  partitioned, each side can make its own progress, and then the link can HEAL (restart without the
  flag; the persisted chain reloads and reconciles). The e2e (`TestPartitionHealsToHeavierForkOverTCP`)
  splits two committing groups into divergent forks (a heavier two-block fork and a lighter one-block
  fork), then heals the lighter side, which **reorgs onto the heavier fork** — consensus reconverges on
  one history. Also adds a `Node.OnReorg` callback so the daemon surfaces a reorg on stdout
  (`chain: reorged onto a heavier fork (dropped N block(s), …)`) — a significant, operator-visible
  consensus event, and the precise signal the e2e asserts. `-block-peers` models a transport fault, not
  Byzantine behaviour (the node stays honest); a real deployment never sets it. Second of #184's four
  consensus-safety cases (after equivocation→slash); low-bond→reject and forged-block→reject follow.
  Whole suite green with \`-race\`; the e2e passes over real sockets.
- **#184 verify — equivocation→slash proven over the REAL WIRE (adversarial-daemon harness)** (2026-08-07,
  [#184](https://github.com/nerolabs/silt/issues/184)) — The accountability property "a proven
  double-sign costs standing" (D2) is now exercised against real daemons over real TCP, not only in the
  in-process sim. A new **`-equivocate` red-team daemon flag** (quarantined in `core/node/adversary.go`,
  loudly announced, reached by no honest path) makes a validator DELIBERATELY double-sign: it places
  block X at height 1 on one honest peer and a heavier conflicting fork (Y, Z) on another. The honest
  detector syncs the heavier fork, reconciles across the two histories, and `chain.FindEquivocations`
  catches the adversary signing two different blocks at the same height — slashing it. Because
  fork-choice is summed qualified-attester weight, the heavier fork is deterministic, so the e2e
  (`TestEquivocatorSlashedOverTCP`) is not a timing race. Also adds a `Node.OnSlash` callback so the
  daemon surfaces slashing on stdout (`chain: slashed equivocator …`) — a real operator-visible
  accountability event, not only a debug-log line. Keeping the adversary in the shipped binary (behind
  the flag) means it runs in CI *and* lets an external red-team drive the same attack against a
  deployment to confirm the defence holds. First of #184's four consensus-safety cases; partition→heal,
  low-bond→reject, and forged-block→reject follow. Whole suite green with \`-race\`; the e2e passes over
  real sockets.
- **D-DEMAND P2 — the optimistic fair-exchange abort-safety floor** (2026-08-07,
  [#181](https://github.com/nerolabs/silt/issues/181)) — The demand exchange is content C (server →
  fetcher) ⟷ a delivery receipt (fetcher → server). Fair exchange provably needs a TTP (Pagnia–
  Gärtner); silt's is the validator quorum as a threshold-distributed TTP (Asokan–Shoup–Waidner),
  invoked only on dispute. This ships the **optimistic phase + both abort-SAFETY properties**, which
  hold structurally today: **(1) fetcher-side** — an aborted exchange never *consumes* the token (a
  serial is spent only by a completed `Redeem`), so a server that takes the commitment and delivers
  nothing leaves the paid token reusable at another server (the fetcher can't be robbed of its token);
  **(2) server-side** — a fetcher's pre-release `ExchangeCommitment` (a signed promise made before
  content release, domain-separated from the receipt and carrying no PoR proof) can *never* redeem as
  demand, so a server can't turn "the fetcher engaged" into a fake completed delivery — the
  unforgeability bound `#receipts(C) ≤ #completed correct deliveries` survives the abort path.
  Regression-locked (token-reusable-after-abort, commitment-is-not-a-receipt, domain separation,
  optimistic path still credits). *Gated, deliberately not built: the dispute-RESOLUTION half — turning
  a server-held commitment into a TTP-affidavit on fetcher default requires the quorum to verify
  delivery completed without the fetcher — i.e. verifiable escrow of the content key (Camenisch–Shoup)
  + threshold decryption t-of-n across validators. The threshold-decryption/DKG half is available in Go
  (dedis/kyber, drand-grade); the wall is the verifiable-escrow primitive (no adoptable pure-Go impl)
  plus the large new crypto trust surface — disproportionate to a neutral observable. Same strategy as
  H7 (floor now, heavy crypto as a fast-follow), different primitive. Demand-neutrality keeps this
  low-stakes: an unresolved abort only undercounts a neutral observable, never standing.
  `ExchangeCommitment` is the seam the future resolver consumes.* Whole
  suite green with \`-race\`.
- **D-DEMAND P3b — the bonded-fetcher credential (second cost-to-wash lever)** (2026-08-07,
  [#181](https://github.com/nerolabs/silt/issues/181)) — Witnessed demand can now be gated on a
  **bond-distinct fetcher credential**: with `demand.Bank.RequireBondedFetcher` (wired on the daemon
  as `Node.RequireBondedFetchers`), a delivery receipt counts toward an object's demand only if the
  fetcher's signing key is a bond-distinct identity in the **committed on-chain bond ledger**
  (`chain.IsBonded` — the same Sybil-priced, deduped supply the C2 metric measures), and demand then
  counts **distinct bonded fetchers per object**. So a self-dealer running one bonded identity can
  still mint N perfectly valid receipts (a self-fetch *is* a real paid delivery — Douceur is
  unbeaten), but witnessed demand rises by **1, not N** — re-pricing wash to *one real storage bond
  per faked unit of demand*, the best achievable under no-center. This is the second lever alongside
  the already-shipped **P3a fee-burn** (each wash burns a real retrieval fee). Demand stays a
  **neutral observable** throughout — the gate changes what *counts* as demand, never whether demand
  touches consensus standing (it never does; the γ→1/N firewall is intact). Off by default (raw
  count, unchanged). Self-dealing red-team locked at both the pure layer (`core/demand`: unbonded →
  0, one bonded identity washing 6 → demand 1, 4 distinct bonded → demand 4) and the real node wire
  (`sim`: one bonded identity washes 5 → demand 1, unbonded delivery → 0, a distinct bonded identity
  → +1). *Residual: the credential shows the bonded key in the clear, so fetcher-unlinkability stays
  nominal until D3/H8 — the `demand.BondCheck` doc marks the exact seam for a blind bond-distinctness
  proof.* Whole suite green with \`-race\`.
- **Registry economics — `-freeload` role separation for the daemon** (2026-08-09,
  [#47](https://github.com/nerolabs/silt/issues/47)) — A daemon can now be started with `-freeload`
  to serve the **registry / relay / routing** role while **refusing to store or serve content** — so
  a public-infrastructure operator can run a rendezvous registry without being conscripted into
  hosting arbitrary content (the conflation that caps how many public registries the network can
  attract, which bootstrap/NAT-traversal depend on). The mechanism (`node.SetFreeload`, honored by
  the serve paths) already existed and was sim-only; this exposes it on the real daemon and announces
  the role. The node still carries DHT routing. *(The leaner `-registry-only` mode — no storage node
  constructed at all — is the follow-up.)* Covered by a real-TCP e2e; whole suite green with \`-race\`.
- **H9 takedown transparency — the CT-style append-only log** (2026-08-09,
  [#180](https://github.com/nerolabs/silt/issues/180)) — New `core/translog`: an RFC 6962
  (Certificate Transparency) append-only Merkle log — adopted, not invented (B8) — for honored
  revocations. It offers the two proofs that make a takedown **auditable and non-silent**:
  **inclusion** ("revocation R is entry i of the log at size N") so a specific takedown is provably
  recorded, and **consistency** ("the log at size M is a prefix of size N") so an operator can't
  quietly rewrite history — a dropped or back-dated revocation breaks the consistency proof.
  Exhaustively tested: prover-generated inclusion paths (every leaf × every size) and consistency
  proofs (every prefix pair) cross-check against the recomputed roots, and a tampered history fails.
  This is the M0-honest core of pluralistic takedown; the ZK non-globality predicate + PIR-routed
  probes on top are post-M0. **Wired into the chain:** every honored revocation/un-revocation is
  appended to the log in `Chain.apply` (a deterministic function of the committed blocks, rebuilt
  identically on replay), and the chain exposes `RevocationLogRoot` + inclusion/consistency proofs +
  the exported `RevocationLeaf` so an auditor can reconstruct a leaf from public block data — so silt
  can now *prove* a takedown was recorded and that its takedown history was never silently rewritten.
  Whole suite green with \`-race\`.
- **D-DEMAND — the delivery receipt goes live on the wire** (2026-08-09,
  [#181](https://github.com/nerolabs/silt/issues/181)) — The `core/demand` primitive is now a real
  node capability. A fetcher `AcquireDemandToken` blind-withdraws a retrieval token from an issuer over
  the existing token-request wire (no new issuance path — the blind signature is domain-agnostic), then
  `SubmitDeliveryReceipt` sends the server a `MsgDeliveryReceipt` (the token + a PoR-bound, signed ack
  over the received bytes). The server verifies it against the issuer key it trusts and banks it into a
  **neutral witnessed-demand observable** (`Node.WitnessedDemand`) — **never wired to consensus
  standing**, so a forged or self-dealt receipt buys zero standing (the γ→1/N firewall). Only receipts
  naming this server are banked; replays (double-spent serial) and forged/mis-issued tokens are rejected
  over the wire. Integration sim covers the honest flow + both rejections. Whole suite green with
  \`-race\`. *(Fee-burn cost-to-wash is P3; fetcher-unlinkability needs D3.)*
- **D-DEMAND P1 — blind-withdrawn retrieval token** (2026-08-08,
  [#181](https://github.com/nerolabs/silt/issues/181)) — The demand token is now **blind-withdrawn**:
  `core/blindtoken` gains a domain-separated demand variant (`BlindDemand`/`VerifyDemand` — a demand
  token can't be presented as a publish token or credit under the same key), and `core/demand` upgrades
  the token from a placeholder issuer-signed serial to `Withdraw → SignWithdrawal → Unblind`. The issuer
  blind-signs the token **without learning its serial**, so the token that later redeems is
  cryptographically unlinkable to its withdrawal. Fetcher-unlinkability stays **nominal until D3
  issuance-mixing** (H8) closes the IP/timing channel — the blind signature hides the serial, not the
  withdrawer's network identity. The P0 unforgeability red-team carries over (now over blind tokens),
  plus an unlinkability regression. Whole suite green with \`-race\`.
- **D-DEMAND P0 — the blind demand receipt primitive (witnessed delivery, unforgeable-at-the-token-level)** (2026-08-08,
  [#181](https://github.com/nerolabs/silt/issues/181)) — First phase of the B axis (served-demand) of the
  systemic claim. New pure `core/demand`: an issuer-signed retrieval **token**, a **PoR-bound
  delivery-ack** (the fetcher signs a Shacham–Waters proof over the delivered bytes, with the challenge
  bound to `serial‖object‖server`), and a **bank/redeem** that credits a per-object *witnessed-demand*
  counter once per token. It proves exactly one thing — **`#receipts(C) ≤ #issued-tokens-spent-on-a-signed-C-delivery`** —
  and, per the decision's doc-truth rule, deliberately does **not** prove demand *authenticity* (a
  self-fetch is a real paid delivery; a Douceur limit, re-priced by cost-to-wash in P3, never proven).
  - **NEUTRAL by construction.** A redeemed receipt is an *observable* (`Bank.Demand`) that is never
    wired to consensus standing — so even a forged or self-dealt receipt buys **zero** standing, keeping
    the γ→1/N shared-content firewall intact (fusing demand into standing stays gated on #182). Standing
    is bond-only today.
  - **Unforgeability red-team** (each a permanent regression): a token not issuer-signed, a
    tampered/lifted receipt (server/object/fetcher/sig), a receipt claiming object C' while holding C's
    bytes (the PoR binding, not just the signature), a data-less "delivery", and a double-spent serial —
    all rejected; only an honest signed delivery credits demand. The public-per-object-key tag-forgery
    residual is documented (H7 precedent; inert because demand is neutral).
  - P1 blind withdrawal, fetcher-unlinkability (needs D3/H8), P2 fair-exchange dispute, and P3
    cost-to-wash economics remain. Whole suite green with \`-race\`.
- **C2 metric wiring — cost-to-corrupt from the committed bond ledger, split-resistant shed** (2026-08-08,
  [#185](https://github.com/nerolabs/silt/issues/185)) — The "no quiet capture" axis (C2 / D-C2) gets a
  first-class, published concentration measurement. `chain.C2Metric()` computes
  `{NakamotoBonds, NakamotoOperators, CostToCorruptBytes, TotalBondedBytes, Margin}` over the
  **committed on-chain `BondReg` ledger** — never gossip, which kills the "lie about your size" *skew*
  half of the skew+split attack outright. It was previously a private helper that only gated the
  training-wheels shed; now it is a single measurement consumed by the shed and surfaced for operators.
  - **Split-half defense via an operator margin.** A `BondReg` carries no operator label, so real
    key→operator clustering is impossible on-chain; instead a config **`OperatorMargin M`** discounts the
    bond-distinct coefficient to `NakamotoOperators = ⌊k̂/M⌋`, and `Mature()` sheds the anchor
    training-wheels only when `k̂ ≥ MatureValidators × M` — so a stake split across many keys must clear
    `k·M` distinct bonds. `M=1` (default) is the legacy/single-operator behavior, unchanged; the daemon
    exposes `-operator-margin` and narrates the metric on every commit (`nakamoto N bonds → M operators |
    cost-to-corrupt … | wheels shed/engaged`).
  - **Honest residuals (D-C2, unchanged):** operator clustering is heuristic *by theorem* (Kwon) — `M`
    only bounds it; `M_est` under adversarial NodeID placement is unquantified; the honest-whale / real
    cartel is outside C2. Byzantine-robust *sampling* and the private-lookup committee-certification
    consumer (H8/#179) are future. Unblocks the external C2 red-team (#183). Full unit coverage of the
    metric arithmetic + the split-resistant shed; whole suite green with \`-race\`.
- **H7 proof-of-correct-repair — the false-repair red-team (acceptance gate)** (2026-08-08,
  [#95](https://github.com/nerolabs/silt/issues/95)) — The self-dealing adversary is driven against
  the **wired** verification handler over a live network (`core/node/redteam_repair_claim_test.go`),
  proving the crypto's verdict actually reaches the ledger, each case a permanent regression:
  - **(a) garbage claim → slash.** A claim naming a real position but a bogus shard id: the judge
    recomputes the position from the manifest-anchored survivors, sees the mismatch (a self-attributing
    fraud proof), and **slashes the claimant** — no bounty.
  - **(c) compute-but-don't-store → denied, never slashed.** A correct shard id on a data-less liar
    holder (keeps the proof + PoR tags, drops the bytes) fails its identity-bound retrievability
    challenge → **denied** but not punished (a shortfall may be transient). This also pins the
    **(b) anti-double-count** property: retrievability binds to the *named* holder, so "the correct
    bytes exist on the survivors" does not pay.
  - **Positive control** — an honest claim on a real holder clears both legs, the holder is paid, and
    **no standing moves** — so the deny/slash cases are discriminating, not blanket rejection.
  - **(d) quorum discovery** — every caretaker announcing under the `careKey` rendezvous is found, so
    none is silently excluded from the vote. *(Domain-diverse quorum SELECTION — refusing a
    single-domain quorum — is explicit deferred hardening, tracked with caretaker-selection work.)*
  Whole suite green with `-race`.
- **H7 finite-but-renewable durability — instrument `g` + the funded horizon (slice 3)** (2026-08-08,
  [#95](https://github.com/nerolabs/silt/issues/95)) — silt does not *promise* perpetual cold-data
  solvency (that promise is the Arweave endowment identity in credits, and it holds only while the
  credit-denominated cost of storage keeps falling — which 2020s hardware evidence questions). So
  durability ships as an explicit **finite-but-renewable** contract, and this slice makes where an
  object sits on it **measurable** (decision D-S7):
  - The escrow now tracks a **repair count** (`PayBounty` increments it), and a per-object
    `ports.DurabilitySnapshot` (reserve, lifetime funded/paid, repairs) crosses the `CreditLedger`
    interface — read-only, classified `neutral` under the Invariant-A guard.
  - New pure instruments in `core/credit` read a snapshot: **`CostPerRepair`** (realised credits per
    shard-repair), **`Horizon`** (how long the reserve lasts at the *observed* burn — returning a
    `finite` flag so "no burn yet" reads as *unproven*, never *perpetual achieved*), and **`G`** —
    instrument *g*, the annualized trend of cost-per-repair, signed so `g > 0` means cost is
    **declining** (the condition under which "perpetual" becomes earnable). `g` stays **measured**,
    never assumed.
  - A bounty payment now narrates the drawn-down reserve and cost-per-repair (`Node.DurabilitySnapshot`
    exposes the accounting for the observatory). Full unit coverage of the instruments + a repair-loop
    sim asserting the snapshot's repair count matches bounties released and the funded horizon is a
    positive finite runway; whole suite green with `-race`.
- **H7 self-funding durability — the serve auto-skim goes live** (2026-08-08,
  [#95](https://github.com/nerolabs/silt/issues/95)) — The escrow that pays repair bounties is now
  topped up by the object's own traffic. The `MsgFetchChunk` serve path resolves each coded shard's
  object root from its storage proof and routes the serve through `RecordServeToObject`, which
  diverts a protocol-fixed slice (`SkimNum/SkimDen`, 1/8) of the serve revenue into *that object's*
  durability reserve — so **popular data pays for its own repair** while the server keeps the net.
  Shards with no proof-anchored root (manifest chunks, uncoded files) keep the plain serve.
  - **Publisher/operator funding API** — `Node.FundDurability(root, amount)` prepays an object's
    reserve from the node's own balance (a pure balance move, never standing), so cold data can be
    endowed to outlive churn before it is popular enough to self-fund; `Node.DurabilityReserve(root)`
    reads the remaining horizon. `ports.CreditLedger` gains `RecordServeToObject`. *(A publisher-side
    CLI subcommand waits on the client credit-balance model; the node API is the entry point today.)*
  - **The invariant holds.** Serve income funds the balance economy and the durability reserve —
    **never** standing. The integration sim retrieves a whole file, watches the reserve fill from the
    serves (a *slice* of the bytes, not the whole thing), and asserts no node's `Reputation` moves.
    Full unit + sim coverage; whole suite green with `-race`.
- **H7 proof-of-correct-repair — the node/network quorum wiring** (2026-08-07,
  [#95](https://github.com/nerolabs/silt/issues/95)) — The `core/repairproof` gate is now wired into
  the live repair loop, so a durability bounty actually flows on a *verified* repair. When a
  caretaker rebuilds a lost shard and places it on a fresh holder (`repairStripe`), it emits a
  `MsgRepairClaim` naming that holder; the object's other caretakers — reached through a new
  **`careKey` rendezvous** (`hash(root ‖ "silt/care/v1")`, announced on `Care`), since only a
  care-link holder has the layout key needed to judge — each independently run both legs:
  - **Verify** (`handleRepairClaim`, `core/node/repairclaim.go`) — reload the layout, fetch k
    survivors *by column* (verifying each against its committed id, dropping what it didn't already
    host — a paramedic, not a hoarder), `VerifyByRecompute` the claimed position, then challenge the
    holder's retrievability under the identity-bound `RepairChallengeSeed`, and `Decide`.
  - **Settle on the LOCAL ledger** — release pays the *new holder* from the object's escrow
    (`PayBounty`, capped by the rarest-shard `BountyFor` multiplier); a self-attributing correctness
    lie slashes the *claimant* (`SlashFalseRepair`). Credit is per-node-local accounting, so each
    caretaker-judge settles independently and the τ-of-q quorum is the emergent property that τ
    honest judges reach release — no on-chain bounty transaction.
  - **The invariant holds through the wire.** A bounty is a pure *balance* motion — the integration
    sim churns a stripe, watches a peer caretaker verify and release the reserve, and asserts **no
    node's consensus standing moves at all**, so the γ→1/N shared-content hole stays shut.
    `ports.CreditLedger` gains `PayBounty`/`SlashFalseRepair`/`FundEscrow`/`EscrowBalance`; new
    `RepairBountyBase`/`RepairQuorumTau` config (bounty economy off by default). Full unit coverage
    (settlement truth table) + happy-path sim; whole suite green with `-race`. *(The full
    self-dealing red-team — garbage claim → slash, relay double-count → denied, compute-but-don't-store
    → denied, quorum domain-packing — and the caretaker-discovery hardening land next.)*
- **H7 proof-of-correct-repair — the verification layer, slice 2 (logic + wire)** (2026-08-07,
  [#95](https://github.com/nerolabs/silt/issues/95)) — A repair bounty must pay only for a *real,
  correct* repair, never a bare claim. New `core/repairproof` composes the gate, unit-tested end to
  end short of the network wiring:
  - **Correctness leg** (`VerifyByRecompute`) — reconstruct the lost shard from k survivors and check
    it is byte-identical to the manifest-committed shard ID. Sound, pure-Go, publicly checkable,
    content-blind. *(A soundness pressure-test proved the plaintext-blind homomorphic-commitment path
    impossible in pure Go over silt's GF(2⁸) storage — there is no ring homomorphism GF(2⁸)→F_r — so
    M0 ships this recompute floor; the blind upgrade is a documented fast-follow. See
    [`docs/design/h7-proof-of-repair.md`](docs/design/h7-proof-of-repair.md) §3, §13.)*
  - **Retrievability leg** (`VerifyRetrievability` + `RepairChallengeSeed`) — a Shacham–Waters PoR
    challenge bound to the holder's own node identity, closing the relay/double-count attack (reuses
    `core/por`).
  - **Release/slash gate** (`Decide`) — release iff correctness holds *and* a τ-of-q retrievability
    quorum confirms; a failing correctness recompute is self-attributing fraud → slash. Backed by a
    new `credit.SlashFalseRepair` press (classified `reduces` under the Invariant-A guard: it can
    only ever *lower* standing).
  - **Repair-role model** decided from the real code: silt's repair is a *paramedic split* (the
    caretaker reconstructs but keeps nothing), so the bounty pays the **new holder** of the rebuilt
    shard (§8b). Wire types (`MsgRepairClaim`/`MsgRepairVote`, `RepairClaim`) landed; the node quorum
    handler + hot-path hook + sim/e2e are the next slice.
- **H7 durability-escrow primitives — the S7 funding layer, slice 1** (2026-08-06,
  [#95](https://github.com/nerolabs/silt/issues/95)) — The repair loop that keeps content alive
  under churn must be paid in equilibrium, not charity (the wound that killed Freenet/GNUnet).
  New in `core/credit/escrow.go`: a **per-object durability reserve** (`FundEscrow`), keyed by an
  object's root, that pays repair bounties; an **auto-skim** (`RecordServeToObject`) that routes a
  protocol-fixed fraction — `SkimNum/SkimDen`, 1/8 — of each object's serving revenue back into
  *that object's* reserve, so popular data self-funds its durability while cold data draws down
  what it prepaid; a **rarest-shard bounty multiplier** (`BountyFor`) that scales the payout by how
  under-replicated a stripe is, so repairing the last spare before data loss pays the most; and a
  `PayBounty` draw-down that pays what the reserve can cover (a short reserve = the object's funded
  horizon running out, *finite-but-renewable*, not an overdraft).
  - **The one load-bearing invariant is enforced structurally.** The durability budget lives in the
    *balance* economy and confers **zero** consensus standing — a durability credit that bought
    standing would re-open the shared-content γ→1/N hole (one physical copy of an erasure-coded
    shard answering for N pledges). The `Invariant-A` reflection guard (`invariant_a_test.go`) now
    classifies every escrow press `neutral` and fails the build if a new one ships unclassified;
    the behavioral half fires funding, skimming, and bounty-payout against a bondless identity and
    asserts `Reputation` never rises above zero. Standing is still minted by the bond press alone.
  - Prototype-first: these are the ledger primitives. Wiring the auto-skim into the live serve path
    and gating `PayBounty` on a verified proof-of-repair transcript are later H7 slices (2 and 3).
    Full unit coverage; whole suite green with `-race`.

### Changed
- **External-audit honesty propagation: held-in-tension residuals carried from the spec down
  to the tenets, risk surface, and public site** (2026-08-06) — Two independent audits of the
  docs pass (a research *comprehension* audit + a red-team *intention* audit) found
  comprehension faithful but a **propagation gap**: the honesty that was correct in `m0.md §10`
  / issue #182 hadn't reached the tenet layer, the risk-tracking surface, or the public pages,
  so three things read as *achieved* that are deliberately *open*. No code changed. Fixes:
  - **The S7 "one ledger" fusion** (served-content ⇄ standing) reworded across `TENETS.md` S7 +
    `m0.md §5` from an achieved fact to the **design goal** — today standing comes **only** from
    the dedicated identity-keyed bond plot, **separate** from served content, gated on the
    γ→1/N problem (#182). A builder implementing the old wording would have re-opened the Sybil
    break the separation prevents.
  - **`C_honest = D×A×T×B`** marked *target composition vs. shipped subset* (`m0.md §3`, TENETS
    Part 0): today standing is gated by the **bond (D) axis alone** — B (served demand) is
    unbuilt (#181), A (address diversity) is at the DHT layer, not in the standing number — so
    C1 is a *conditional* claim. Added the missing served-demand row to the `m0.md §6` as-built
    map (NOT SHIPPED → #181).
  - **γ→1/N** added as an explicit open-risk row in `risk-register.md` + `threat-catalog.md`;
    the "proof-of-repair now EXISTS" durability headline softened to *construction designed,
    not yet built (H7/#95)* across `threat-catalog.md`, `TENETS.md`, `decisions.md`.
  - **D-PRIV propagation:** `TENETS.md` Part VIII table row "Privacy of *access* is absolute"
    corrected to the refusal-to-surveil form; `decisions.md` "publish-unlinkability is delivered"
    → *chain-layer only; transport IP+timing OPEN until D3 (H8/#179)*.
  - **Public site regenerated** (`index.html`/`node.html`/`docs.html`): the Sybil-standing copy
    ("reputation = audits + bytes served / bandwidth counts toward reputation") corrected to
    bond-backed standing; the unlinkability hero requalified (opt-in blind tokens + IP+timing
    caveat); "alive forever" → finite-but-renewable; "no token" → "no *speculative external*
    token"; "private by architecture" → content-blind.
  - **C2 "no quiet capture"** promoted to a first-class decision entry (`k*≥k̂/M`, Kwon floor,
    honest-whale + adversarial-placement residues); added risk rows for `g≤0` and CPR under
    adversarial NodeID placement; reconciled the `threat-model.md` BFT self-contradiction.
- **Full non-code file audit + remediation; stray binary removed** (2026-08-06) — Audited
  all 106 tracked non-code files (purpose · last-updated · needed? · safe-to-remove/archive ·
  staleness). Findings actioned; no Go behavior changed except one web-UI default (below).
  - **Stray removed:** `shardnet` — a 5.1 MB Mach-O binary committed under the project's old
    name — deleted and gitignored. No other committed strays or dead files found.
  - **The one factual contradiction fixed:** `docs/risk-register.md` still said center-less
    proof-of-repair was "routed to research"; it's **delivered** (D-S7) — corrected, plus
    finite-but-renewable durability and D-DEMAND (cost-to-wash pricing).
  - **`docs/threat-model.md` reconciled** (public disclosure doc): the Sybil/eclipse, PoR,
    free-rider/wash, colluding-quorum, and trust-assumption sections rewritten from the old
    "reputation quorum / storage bond / DHT eclipse unhardened / Gate 4" framing to the
    current **C1 + C2 composition** (objective bonded fork-choice, H5 eclipse hardening,
    D-DEMAND wash re-pricing, private-by-default).
  - **`website/docs.html`** consensus section + meta refreshed from "reputation-quorum" to
    the objective bonded-quorum / C1·C2 framing; link-format copy corrected for
    private-by-default.
  - **H6 behavioral gap closed:** `cmd/silt/ui/publish.html` defaulted the web publish mode
    to `convergent`; now defaults to **private** (matching the CLI), with the confirmation-
    attack caveat.
  - **Staleness sweep:** `docs/math/02` + `docs/math/07` (convergent-as-default → private),
    `docs/math/05` (retired "Gate 4 #90" citation), `docs/math/08` (H4 Byzantine quorum
    note), `docs/design/cross-network.md` (relay incentives → D-DEMAND), `docs/threat-catalog.md`
    + `docs/safety-denylist.md` (backfilled the 08-06 commission facts).
  - **Archive hygiene:** `docs/fresh-eyes-council.md` archived (a new council brief added at
    `docs/reviews/fresh-eyes-council-brief.md`); `docs/design/bond-audit.md` archived with a
    live wire-protocol stub left in place; 6 broken intra-archive relative links and 2 stale
    "LIVING/current" banners fixed; `archive/README.md` index updated. `.gitignore` deduped.
- **ROADMAP + BACKLOG reconciled to the current strategy; the retired Gate 0→6 spine
  removed** (2026-08-06) — Both planning docs still narrated the old builder-phase spine
  ("V1 = Gate 0→6, **Gate 4 is the M0 mechanism to build**"), which predates the mechanism
  being built, the composition reset, and the research commission. `ROADMAP.md` rewritten to
  the honest current status (storage plane field-proven; M0 mechanism BUILT + H1–H6 hardened;
  mission reframed as **C1 + C2 held in tension**; commission answered) and the **forward
  tracks** that replace the gate spine: **build** (H7 durability/proof-of-repair — next; H8
  metadata privacy/D3; H9 takedown CT-log + non-globality metric; D-DEMAND blind receipt; the
  C2-metric-from-ledger wiring; registry economics), **verify** (multi-machine field test +
  external red-team vs C1/C2 — the gate to "M0 held"), and the **research frontier**
  (shared-content sealing boundary; MSR proof-of-repair; CPR under adversarial placement).
  `BACKLOG.md` slimmed to genuinely-open captured ideas + repointed at
  `docs/design/m0.md` / `docs/decisions.md` as the source of truth (shipped placement /
  networking / observability / fresh-eyes work moved out — it lives in git + buildlog). No
  code changed. GitHub issues reconciled in the same pass (Gate-4 mechanism issues closed as
  built; new build/verify/research-frontier tracks filed).
- **Research commission answers folded into the decision ledger; the two routed-to-research
  constructions now EXIST** (2026-08-06) — The follow-up research commission
  (`silt-reviews/research/research-outcome/commission/`, eight footnoted memos) answered the
  questions `archive/reviews/research-brief.md` had routed out. Recorded across
  `docs/decisions.md`, `docs/design/m0.md`, and `docs/TENETS.md`; no code behavior changed.
  - **D-S7 — construction DELIVERED + durability restated finite-but-renewable.** Center-less
    **proof-of-correct-repair now exists** as a composition of proven parts (a transparent
    binary-field polynomial commitment [FRI-Binius, no trusted setup] for *correctness* +
    Shacham–Waters PoR for *retrievability* + a DAS quorum for *center-less checking*) — ~100 B
    proof, no plaintext seen, no new primitive for the plain-RS case → build track **H7**.
    Durability ships as an explicit **finite-but-renewable** contract, not "perpetual":
    perpetual cold-data solvency is the Arweave endowment identity in credits and holds only
    while a positive credit-denominated cost decline `g > 0` (which 2020s hardware no longer
    guarantees), so silt funds a renewable horizon and **instruments `g`** as the number that
    decides perpetual-vs-finite. (MSR/regenerating-code proof-of-repair stays genuinely open,
    off the critical path.)
  - **D-TAKEDOWN — non-globality metric CONSTRUCTED.** A *survivor Nakamoto coefficient over
    failure domains*, published as a certified lower bound `≥ t` via a **ZK threshold
    predicate** that reveals only the scalar `t` (defeating the discovery-oracle) — as real as
    the (non-cryptographic) independence oracle. Stays low-urgency → H9.
  - **D-DEMAND (new decision).** Standing is priced on **cost-to-wash, never receipt count**.
    The blind demand receipt (Chaumian token + PoR-bound delivery-ack + quorum-as-TTP fair
    exchange) delivers unforgeable-delivery + fetcher-unlinkability, but **demand *authenticity*
    is a Douceur limit** — self-dealing is uncloseable by any receipt; wash is re-priced (burned
    fee + bonded-fetcher credential), not proven away.
  - **The core open problem, named precisely.** `B5` proves **C1 (no discount) is a theorem
    under H1–H3**; the single surviving economy of scale is the **shared-content sealing
    boundary** (plain PoR over shared erasure-coded shards leaks γ→1/N, closed only by
    identity-keyed PoRep sealing). silt is **not exposed today** — standing uses a dedicated
    identity-keyed bond plot, not the shared shards — but fusing served content into standing
    without leaking γ→1/N is the open, academic-collaborator task (`docs/design/m0.md` §10).
    Cross-cutting engineering find (`B1`): compute the C2 concentration metric's weight from the
    **committed on-chain bond ledger, not gossip** — one measurement feeds three seams.
- **M0 reframed as a systemic composition (not a Sybil-proof primitive); tenets amended and
  docs reset** (2026-08-05) — Adopting the research capstone (`09-m0-as-composition.md`), M0's
  Sybil corner is now stated as a **systemic** claim — **C1 (no discount) + C2 (no quiet
  capture)**, held in tension — rather than a per-primitive "Sybil-proof" claim that is false by
  theorem (Douceur: no single primitive prevents Sybils under free identity + no permanent
  center). This changes what "done" means: a primitive failing a standalone is-it-Sybil-proof
  test is *expected*, not an M0 failure; the verdict target is the composition and its seams.
  - **`docs/TENETS.md` amended** (see the amendment log). Decisions derived from the accepted
    research package and recorded: **D-PRIV** — immutable #4 requalified from an absolute
    ("access never observable") to *refuse-to-surveil* (absolute) + *access-unobservability held
    in tension* at the metadata layer (the anonymity trilemma is a hard wall). **D-S7** — S7 now
    states the durability funding model (internal escrowable credit reserve; **no *speculative
    external* token**); center-less proof-of-repair is the open construction, routed to research.
    **D-TAKEDOWN** — immutable #5 commits every honored revocation to a CT-style transparency log
    toward a formal non-globality guarantee. **D-DISCLOSURE** — new Don't #8 (no decryption
    backdoor at core). B8/S7/immutable-#3 threaded with the composition thesis (C1/C2; the
    one-ledger S7↔Sybil-budget fusion; the young→mature maturation bet).
  - **New `docs/design/m0.md`** — the single M0 spec (thesis + interlock + surface map S1–S8 +
    the 7 composition seams = the red-team/build target + open decisions + open problems).
  - **New `docs/decisions.md`** — the decision ledger, each entry splitting derived direction
    from deferred construction.
  - **New `archive/reviews/research-brief.md`** — open questions for the research team (the two
    constructions the memos self-flagged non-existent — center-less proof-of-repair and the
    non-globality metric — plus the seam stress-tests).
  - **`/archive/`** — the finding-by-finding history moved out of the live tree (5 M0 design
    notes, 5 red-team/acceptance/audit reports, the genesis handoff) behind an index README;
    nothing deleted. The live tree now carries one current (composition) viewpoint.
  - **Every remaining non-code doc reconciled** to the composition framing (README, ROADMAP,
    threat-catalog, the 3 review briefs, and 10 others). No code behavior changed.

### Fixed
- **H6 (privacy, Memo 02): default publish is `private` — no existence oracle for guessable
  content** (2026-08-05) — convergent encryption derives the key from the plaintext, so the
  content address is a deterministic function of the plaintext: anyone who GUESSES it can
  compute the root and look it up to confirm you stored it (the confirmation attack), and
  it shipped as the DEFAULT. H6 flips the default publish mode to `private` (a random
  per-file key) across every publish path — `silt add`, `swarm add`, and the web UI — so
  identical content encrypts differently each time and can't be probed for; convergent is
  now explicit opt-in and prints a confirmation-attack warning. Regression:
  `core/pipeline/redteam_h6_test.go` — the attacker computes the convergent root of a
  guessed plaintext; under convergent a registry probe HITS (the oracle, documented), under
  the private default it MISSES, and two private uploads of identical content don't even
  collide. The Memo 02 "Proof-of-Ownership" idea was deliberately not added: a PoW-to-serve
  gate contradicts silt's capability model (the link/manifest IS the read capability) and
  possession is already gated by store-time hash verification + PoR audits, so private-by-
  default is the substantive fix (reasoning recorded in the strategy doc §7 H6).
- **H5-B (DHT eclipse, Memo 08): failure-domain diversity — a single-domain key-surround
  can't suppress discovery** (2026-08-05) — H5-A stopped provider records being *forged*;
  this stops them being *suppressed*. An adversary that grinds the NodeIDs closest to a
  content key (a ~$4 /24 key-surround) could hold every slot a lookup converges on and
  simply return nothing. Fix, reusing the gossiped failure-domain (`Domain`) signal as the
  diversity dimension: (1) the routing table caps same-domain peers **per bucket**
  (`dht.Table.SetDiversity`), so a one-domain Sybil cluster can't fill the buckets near a
  key and evict honest peers; (2) provider records are announced to a domain-**spread**
  near set, not just the NodeID-closest (`announceTargets`/`diverseNear`), so honest nodes
  in other domains hold the record; (3) after the distance walk converges onto the
  surrounding NodeIDs, resolution **sweeps** that domain-spread set (`sweepProviders`), so
  the honest holders are actually queried. `DHTDomainCap` gates it (0 = off); default
  `-dht-domain-cap 2` for the daemon and the ephemeral fetcher. Regression:
  `core/node/redteam_h5b_test.go` — an adversary grinds the 10 closest NodeIDs to a key
  (one domain, suppressing); with diversity OFF the key is undiscoverable, with it ON
  discovery succeeds through honest other-domain nodes; plus unit tests for the
  domain-spread near set and the per-bucket routing cap. Residual: `Domain` is
  self-reported — binding it to the transport-observed /24 (or per-AS) is the full-strength
  hardening. Real-TCP `e2e` green; this completes surface S5 (with H5-A).
- **H5-A (DHT eclipse, Memo 08): self-certifying provider records — records can't be
  silently forged** (2026-08-05) — DHT provider records were unsigned NodeIDs: a node
  holding the k-closest slots to a content key could fabricate provider records for
  identities that never announced, or inject fake providers into the records it re-serves
  on lookup (the forgery half of the ~$4 key-surround). Fix: `ports.ProviderRecord` is a
  signed "I hold content under key K" claim bound to the provider's identity
  (`sha256(pubkey) == ID`) and the key, with an optional expiry. A node signs its own
  announcements with its identity key (`SetSigner`), the store path (`acceptAnnounce`)
  rejects any record that isn't a valid self-announce for the queried key,
  `MsgGetProvidersReply` re-serves the signed records, and a fetcher
  (`acceptedProviderIDs`) drops any record not signed-for-this-key-and-fresh — so a
  forged, mis-signed, expired, or cross-key-replayed record is silently discarded, while a
  fetcher still hash-verifies chunk bytes on receipt. `RequireSignedProviders` is on by
  default for the daemon (`-signed-providers`); unsigned legacy records still flow when
  it's off (sim/trusted). Wire: new `ProviderRecord` type + `Provider`/`ProviderRecs`
  message fields, mirrored in the tcpnet CBOR frame. Regressions: `core/node/redteam_h5_test.go`
  (a signed record binds to identity+key; a third-party or mis-signed announce is rejected
  at the store; a fetcher drops injected forged / cross-key records; unsigned records flow
  only in non-strict mode), real-TCP `e2e` green under strict signing. **Follow-up (H5-B):**
  the *suppression* half of key-surround (prefix-diversity routing + disjoint-path/wide-
  region announce, so a key stays discoverable when one /24 owns the k-closest NodeIDs).
- **H4 (consensus safety, Memo 05): Byzantine quorum sizing + Nakamoto-coefficient shed
  metric** (2026-08-05) — consensus safety rested on a FIXED quorum (default 3) and a
  training-wheels shed triggered by a HEAD-COUNT of distinct validators. Both are Sybil-
  fragile: a fixed 3 among 30 validators no longer guarantees two quorums share an honest
  node (quorum-intersection safety is lost as the set grows), and one operator spinning up
  many keys could trip the head-count maturity, then capture consensus once the anchors
  shed. Fix, per Memo 05 (*safety is quorum arithmetic at the Byzantine threshold, not
  reputation weight*): (1) `Config.ByzantineQuorum` sizes a commit's support set (proposer
  + attesters) at a supermajority **n−f** of the qualified bonded set (f = ⌊(n−1)/3⌋), so
  any two quorums intersect in ≥ f+1 ≥ 1 honest validator; the proposer gathers
  `max(floor, RequiredQuorum())` and `ValidateCommit` enforces it. (2) `Mature()` now
  measures the **Nakamoto coefficient** over the participating non-anchor bonded set
  (`validatorsSeen ∩ current bond`) — the min number of bond-distinct operators needed to
  reach ⅓ of the weight — which is participation-gated (no fake-genesis decentralization),
  weight-aware (a set dominated by one bond has coefficient 1 → stays immature no matter
  how many satellite keys), and revertible (a lapsed bond drops out → the wheels re-engage,
  the post-shed escape hatch). Both default-on for the untrusted objective posture
  (`effectiveByzantineQuorum`, `-byzantine-quorum`). Regressions: `core/chain/h4_consensus_test.go`
  (`TestBFTQuorumIntersectionAboveFaultBound` proves two quorums always intersect above the
  fault bound for every set size; `TestByzantineQuorumScalesWithValidatorSet` +
  `TestFixedQuorumUnsafeWithoutByzantineSizing`; `TestMaturityNakamotoResistsOneOperator`
  shows one operator's many keys can't trip the wheels), `cmd/silt/invariant_b_test.go`
  (S4 default-on). Residual (documented): an operator that splits stake into many EQUAL
  bonds still inflates the coefficient — stake concentration is invisible on-chain — but it
  pays the full cost-to-corrupt and the Byzantine quorum bounds it to ≤ ⅓ of weight.
- **H2 / RT-2 (Sybil, High): bond standing decays across time by default — release-and-
  coast denied** (2026-08-05) — the blind red team broke the Sybil corner (over the G2
  fix) through the *time* axis: a validator registered a genuine bond once, **released
  the plot**, and kept voting forever off that single one-time proof, because the bond
  TTL (`BondTTLBlocks`) shipped **off by default** — the third "fixed but off by default"
  instance. It could not simply be flipped on: renewal happened only when a validator
  *proposed*, so an attest-only validator would never renew and would lapse, costing the
  quorum its weight (a liveness trap). Fix: a **non-proposer renewal path** —
  `node.SubmitBondRenewal` broadcasts a fresh self-signed `BondReg` (new
  `MsgSubmitBondReg`); a receiver re-verifies it for the current head
  (`chain.ValidateBondReg`) and queues it (`pendingBondRegs`); the next proposer folds the
  queued peer regs (deterministically ordered, head-filtered so one stale reg can't poison
  the block) into its block, mirroring `pendingSlashes`. The chain-sync sweep drives
  renewal, so an attest-only validator renews without ever proposing. **Only then** is the
  TTL made safe-by-default on the untrusted objective posture (`effectiveBondTTL`, mirroring
  the anti-release floor; explicit `-bond-ttl 0` is the trusted opt-out). Regressions: sim
  `TestObjectiveBondRenewalSustainsAttestOnlyValidator` (attest-only validator sustains
  standing across many TTL windows via the wire renewal path while a released validator is
  pruned — no liveness regression), `core/node/redteam_rt2_test.go` (TTL off ⇒ coast
  survives, the vuln; TTL on ⇒ released plot decays out), `cmd/silt/invariant_b_test.go`
  (the untrusted default turns the TTL on).
- **H3 (Sybil, systemic): Invariant-A/B guardrails so a standing press or an off-by-default
  mechanism cannot ship unaudited** (2026-08-05) — the strategy doc's two meta-patterns
  ("we fix instances, not classes" and "fixed but off by default") each bit us three-plus
  times (F1→G2→RT-1; F6→F4→G4→RT-2). Turned both classes into compile-and-test obligations:
  `core/credit/invariant_a_test.go` enumerates every standing-granting press (a reflection
  guard forces each `*Ledger` method to be classified `mints`/`reduces`/`neutral`; a
  behavioral guard proves no non-`mints` press lifts a bondless identity; the sole `mints`
  press — the bond — is asserted identity-bound + deduped + bond-gated), and
  `cmd/silt/invariant_b_test.go` builds the default untrusted-validator config and asserts
  it denies the attack per mechanism (S1 anti-release floor on, S3 bond-TTL on). A new
  press that skips classification or a mechanism that ships off-by-default now fails loudly.
- **H1 / RT-1 (Sybil, Critical): PoR audits no longer mint consensus standing —
  a disk-less relay farm earns nothing** (2026-08-05) — a fresh blind red-team broke
  the Sybil corner (over the G2 fix) via the proof-of-retrievability audit press:
  `credit.Reputation` added `auditsPassed·25` with **no bond gate**, and the PoR proof
  was a pure function of `(chunkID, challenge, data)` — not bound to the prover, and
  challenged with a shared, publicly-derivable seed. So a data-less identity could
  **relay** an honest holder's aggregated `(μ, σ)`, pass, and reach propose/attest
  eligibility (100 rep) with **zero storage** — the code's own "a liar without the
  bytes cannot answer" comment was false (relay doesn't need the bytes, only a holder
  that has them). Fix (architectural, per `docs/design/m0-hardening-strategy.md`
  Invariant A + research memo 03: *plain PoR over shared content is not Sybil-
  resistant*): **PoR audits grant no Sybil-resistant standing** — removed the mint, so
  standing rests on the identity-bound storage bond alone; audits now fund only the
  balance economy and remain a *negative* integrity signal (a failed audit still
  subtracts, and can never be Sybil-amplified). Defense-in-depth: the challenge is now
  **identity-bound** (`porProverSeed = H(base‖proverID)`), so a relayed proof for one
  identity fails another's verify. Regressions: `core/credit/redteam_rt1_test.go` (audit
  passes grant 0 standing without a bond; an **Invariant-A property test** that no press
  mints standing without a bond; failed audits still penalize), `core/node/redteam_rt1_test.go`
  (relayed proof denied), `sim/por_standing_test.go` (holder passing audits over the wire
  earns 0 standing without a bond). Standing-granting sims/tests updated to earn standing
  via the bond press. **Residual (tracked):** a *colluding bonded holder* can still
  recompute a proof per Sybil to farm *balance* (not standing) — closing that needs
  sealed real-content replicas (backlog H7). **Honest status: built + covered, awaiting
  external re-verification** (B8).
- **G2 (Sybil, Critical): the storage bond is now a VERIFIED proof-of-space —
  prefix plots can no longer back N standings from one disk** (2026-08-05) — the
  fix-verification red-team broke the Sybil corner a second time, over the F1 fix
  code, with **prefix plots**: `plotBlock`/`parentIndices` keyed only on
  `(secret, i)` and never on the total block count `n`, so blocks `0..m-1` of an
  `n`-block plot were **byte-identical** to a standalone `m`-block plot with its
  OWN distinct Merkle root — and `VerifySpaceTime` only checked Merkle *inclusion*,
  never recomputed a *label*. Per-root dedup (F1) keys on *equal* roots, so it was
  structurally unable to catch a family of *distinct* prefix roots: the scheme was
  proof-of-STORAGE, not proof-of-SPACE, and one physical plot backed ~`N`
  standings (marginal cost of one more Sybil ≈ one 4 KiB block). The fix (a
  graph-labeling proof-of-space over silt's existing DRSample graph — DFKP CRYPTO'15
  / ABH CCS'17, adopted not invented) seals the plot from a **public, identity- and
  size-bound seed** `H("silt/bond/plot/v3" ‖ pk ‖ n)` folded into both the labels
  and the parent draws, and adds a **labeling-consistency challenge**: the answer
  opens `k` challenged nodes with their predecessor and DRSample parents (Merkle-
  proven), and the verifier **recomputes** each label from the opened parent bytes
  under `H(pk, n)` and requires a match. Because the seed is public the verifier can
  do this without holding the plot, so identity and size become **checked** properties
  of the plot, not claimed ones: a prefix, a foreign-identity plot, or arbitrary
  committed bytes all fail the recompute. **N standings now require N plots.** `k` is
  a per-network knob (`-bond-label-k`, `Config.BondLabelSamples`, default 64;
  soundness error ≤ `(1-ε)^k` against an ε-short prover); leaving it unset resolves
  to 64 inside `core/bond`, so the check is never silently disabled. The seed and the
  labeling check ship **together** (a public seed without the check would regress
  griefing), and G3's "proof beats declaration" rule is load-bearing for the public
  seed's griefing-safety. Plot format **v2 → v3** — a one-time fleet re-plot; the
  disk version guard forces it so a restart never reloads an insecure v2 plot.
  Regressions: `core/bond/redteam_g2_test.go` (a prefix passes possession but fails
  the labeling check; a plot for one key fails under another; arbitrary bytes fail;
  a prefix *family* forges **zero** standings; k unset still denies),
  `sim/bond_sybil_g2_test.go` (a Sybil pointing at another node's plot earns no
  standing over the live-audit wire), `adapters/diskplot` (a v2 file loads as
  absent → re-plot), and the objective/audit e2e paths carry the ~1.5 MB label proof
  over TCP and on-chain. Design: [docs/design/m0-sybil-rebind.md](docs/design/m0-sybil-rebind.md).
  **Honest status: built + covered across all three tiers, awaiting external
  red-team re-verification — not self-certified held** (immutable B8: the tight
  `ε→k` constant and the on-chain proof-size / asymmetric-`k` mitigation are the
  carried open risks in the design note §8).
- **Retest G4-residual: the anti-release floor is now ON BY DEFAULT for an untrusted
  validator** (2026-08-05) — `#163` shipped the floor + re-challenge mechanism but
  defaulted both knobs to `0`, and the daemon did not auto-enable them on the
  earned-standing M0 path (unlike `-objective`, which *is* auto-on when `-min-rep > 0`).
  So a stock, doc-following open validator still admitted a sub-floor, releasable bond
  to full objective standing — **fixed but off by default is not fixed.** The
  anti-release floor now gets the same treatment `-objective` has: it defaults to a
  **derived 1 GiB** for an untrusted validator (`-validator` + `-min-rep > 0` +
  objective), the value the flag's own arithmetic implies (~270 MB/s plot throughput ×
  the ~2 s challenge window ≈ 540 MiB, with ~2× margin). The daemon **fails closed** if
  `-bond` is under the floor — an actionable refusal beats running a validator that
  silently earns nothing — and an operator can still opt out **explicitly** with
  `-min-bond-floor 0` for a trusted/demo swarm. A non-validator is unaffected.
  Regression: `cmd/silt/bondfloor_default_test.go` (the derived floor exists, exceeds
  what re-plots inside a challenge window, denies the default 64M bond, and an explicit
  choice — including `0` — always wins). Docs + the local walkthroughs opt out
  explicitly and now document the floor.
  **Known gap, deliberately NOT defaulted on:** `-bond-ttl` (the objective re-challenge
  cadence) stays off, because bond renewal currently happens only when a validator
  *proposes* (`chainrole.go`), and proposing is event-driven — an attest-only validator
  would never renew and would lapse, costing the quorum its standing. Defaulting the TTL
  on requires a renewal path for non-proposers first; tracked as follow-up.
- **Retest G4 (Sybil/time, High): the objective validator set now enforces an
  anti-release floor and re-challenges bonds on a cadence** (2026-08-04) — the fresh
  pass found the "time" half of proof-of-space-TIME was not enforced on the OBJECTIVE
  fork-choice path: `c.bonded` was set once at registration on a one-time proof and
  never decayed or re-challenged, and `chain.Config` had no anti-release floor at all
  (only `MinBond`). So (a) a sub-floor bond — small enough to release and re-plot
  inside a challenge window — earned full objective standing, and (c) a validator
  could prove once, RELEASE its plot, and keep voting forever with zero resident
  storage (the node-side floor + live re-challenge lived only in the credit ledger the
  objective set never reads). Two additive, deterministic knobs close it:
  `Config.MinBondBytes` (an objective anti-release floor — a bond below it earns no
  standing, rejected on the normal path and uncredited at genesis) and
  `Config.BondTTLBlocks` (objective standing LAPSES this many blocks after a
  validator's latest registration unless it renews with a FRESH space-time proof —
  height-driven, so every replica decays in lockstep). A validator that releases its
  plot cannot answer the fresh challenge to renew, so its vote decays to nothing. The
  daemon wires both: the existing `-min-bond-floor` now also feeds the chain floor,
  and a new `-bond-ttl` sets the cadence. Both default to 0 (off), so legacy/sim
  configs are unchanged. Regressions: `core/chain/redteam_verify_objective-antirelease_g4_test.go`
  (sub-floor bonds earn zero standing / are rejected; standing decays without renewal
  and persists with it) and `core/node/redteam_verify_objective-antirelease_g4_test.go`
  (through the real `bond.VerifySpaceTime`: a validator that stops renewing lapses; a
  continuously-renewing one keeps standing).
- **Retest G3 (Accountability, High regression): a genesis bond-squat can no longer
  lock out an honest validator** (2026-08-04) — the fresh pass found the F1 per-root
  dedup (`#158`) became a griefing lever when combined with the pre-existing
  unvalidated genesis `BondRegs`: a malicious genesis pre-squats an honest
  validator's real plot root under an attacker key (no space-time proof — genesis
  regs are declared), so when the true holder later registers that root on the
  normal path with a REAL, verifier-accepted proof, `apply()`'s first-owner dedup
  sees the root already claimed and drops the honest credit — the holder earns 0,
  the squatter keeps unbacked standing. Fix: **proof beats declaration.** `apply()`
  now tracks whether a root's owner claimed it with a verified proof (a height>0
  registration, gated by `validateBondRegs`) or a mere declared genesis reg
  (`bondRootProven`); a verified registration DISPLACES an unproven declared claim
  (stripping the squatter's standing), while every other collision still earns
  nothing — so once proven, first-proven-owner wins and F1 is preserved. Regressions:
  `core/chain/redteam_verify_genesis-bondsquat_g3_test.go` (inverted PoC: V's proof
  displaces the squat; a second identity still can't share the proven root) and
  `core/node/redteam_verify_genesis-bondsquat_g3_test.go` (a real live bond
  registration displaces a genesis squat through the objective space-time verifier).
- **Retest G1 (Accountability, Critical regression): a genesis block can no longer
  carry an equivocation Slash** (2026-08-04) — the fix-verification red-team's fresh
  pass over the F1/F2 code found that `#158`'s on-chain `Block.Slashes` reopened, for
  a stronger lever, exactly the door `#159` (F3) closed for `Revocations`.
  `AppendGenesis` skips `validateSlashes`, and `apply()` unconditionally evicts every
  `Slashes` culprit (`slashed[id]=true`, dropped from `bonded`, barred from
  re-earning, carried through `adopt()`), so a genesis carrying an **unverified**
  Slash was a proof-free, pre-emptive, identity-level kill switch — a fortiori what
  immutable #5 forbids. A slash is only meaningful against equivocation within a
  chain's own history, of which a genesis has none, so `AppendGenesis` now **rejects**
  any genesis carrying `Slashes` (`ErrGenesisTakedown`), symmetric with the F3 guard;
  a slash must go through the normal path where `validateSlashes` → `VerifyEquivocation`
  gates it on a real double-sign proof. Regressions: `core/chain/redteam_verify_genesis-slash_g1_test.go`
  (genesis slash denied, victim keeps standing, normal-path slash still fires on a real
  proof) and `core/node/redteam_verify_genesis-slash_g1_test.go` (a node in objective
  mode never establishes a genesis that evicts an honest bonded validator).
- **Blind red-team F4 (integrity, S1): the auditor no longer trusts a prover's
  self-reported PoR block count on the file's last shard** (2026-08-04) — the audit
  graded every leaf but the last against a block count it recomputed itself, while
  the LAST leaf took a lenient "tail" branch that accepted any `1..wantFull`. Since
  `porChallenge` clamps the sample space to the prover's reported count, a liar
  holding only block 0 of an N-block shard could report `PorBlocks=1`, be challenged
  on block 0 alone, and pass — earning rent while holding ~1/N of the shard, with no
  slash and no repair. The premise behind the leniency was wrong: `chunk.Split`
  zero-pads the last frame up to `ChunkSize` (the true length rides in the frame
  header) and erasure pads short stripes, so **every stored shard is full-size on
  the wire** — there is no short tail to accommodate. The auditor now demands the
  same recomputed full block count for **every** leaf, so a prover can never shrink
  its own challenge. Regressions: `core/node/redteam_verify_liar-por_0_test.go`
  (inverted PoC — the shrink liar's grading predicate now fails, an honest holder
  still passes) and `sim/audit_tailshrink_test.go` (integration: a shrink liar on a
  single-chunk file's sole — previously lenient — leaf is slashed into debt while
  honest holders pass).
- **Blind red-team F3 (Accountability): genesis can no longer pre-emptively revoke
  a never-published root** (2026-08-04) — `AppendGenesis` calls `apply()` directly
  and skips `validateTakedowns`, so a genesis block could carry `Revocations`
  naming a root never published — a pre-emptive takedown, exactly what immutable #5
  forbids ("a takedown is never pre-emptive"), honored forever by any node running
  `-honor-chain-revocations`. `AppendGenesis` now **rejects** any genesis carrying
  `Revocations` or `Unrevocations` (`ErrGenesisTakedown`): a genesis seeds entries
  and declared launch bonds only; a takedown must go through the governed normal
  path where `ErrRevokeUnknownRoot` enforces that the root already exists.
  Regression `core/chain/redteam_verify_censor_0_test.go` (inverted PoC: genesis
  takedown rejected; the normal-path existence guard still fires).
- **Blind red-team F1 (Sybil, Critical) + F2 (equivocation slash inert): the
  objective validator set now honors the two defenses it was bypassing**
  (2026-08-04) — a second, blind red-team pass (`ae005e9`) found that promoting
  objective on-chain-bond fork-choice to the M0 default (#154) made it authoritative
  for standing, but it skipped two defenses that lived only in the
  non-authoritative `core/credit` reputation ledger. Both are now carried into
  `core/chain`. **F1 — per-root bond dedup:** the objective set never checked that
  a bond `Root` was unclaimed, and the space-time proof is not identity-bound, so N
  cheap identities could register the *same* plot's root+answer and each earn full
  `MinBond` fork-choice weight — one 4 MiB disk buying a whole write quorum.
  `apply`/`validateBondRegs` now enforce a `bondRootOwner` map (a root credits AT
  MOST ONE identity, the first to claim it; the owner may renew), so N Sybils cost
  N independent bonds again. **F2 — on-chain equivocation slash:** `SlashEquivocation`
  only mutated the reputation ledger, which objective mode never reads, so a proven
  double-signer kept full eligibility and weight. Slashing is now an **on-chain
  record** (`Block.Slashes`, a self-verifying equivocation proof) that on commit
  **evicts** the culprit from `c.bonded` and bars it from re-earning standing —
  applied in lockstep on every replica; a forged slash is rejected
  (`ErrBadSlash`), so forged-slash griefing stays denied. The node records
  detected equivocations on-chain in the next block it proposes. Regressions:
  `core/chain/redteam_verify_*` (shared-root denied, slash evicts, forged slash
  rejected) and `core/node/objective_slash_test.go` (over the loop: a node detects,
  records, and every replica evicts).

### Security
- **M0 composition: every red-team finding (F1–F7) fixed and covered by tests —
  awaiting external re-verification** (2026-08-04) — following the red-team break
  below, all seven findings now have a shipped fix with unit + in-process
  simulation coverage, and real-TCP e2e where a daemon surface exists. Sybil
  (byte-binding over a depth-robust graph + read-bound VDF + anti-release floor),
  Privacy (ephemeral publish identity + prepaid Chaumian credits + canonical
  issuer set), Accountability (existence-checked, per-operator, reversible
  takedowns), Consensus (objective on-chain-bond fork-choice with an anchor
  cold-start; F7 resolved by F6 + sound same-height slashing). **The per-finding
  fix + how-to-verify guide for the next reviewer is
  `docs/reviews/M0-REDTEAM-VERIFICATION.md`.** This is the builder's response, NOT
  a self-certification: M0 is *held* only when a fresh external red-team denies all
  three failure modes. Deliberately deferred residuals (honestly recorded): the
  public-IP issuance IP+timing refinement (F4; the stronger NodeID/fee/subset links
  are severed and NATed clients already relay), and flipping the objective-mode
  default (a launch-config decision).
- **M0 external red-team: primitives real, composition unproven, M0 not yet
  held** (2026-08-04) — the independent M0 red-team ran against shipped code
  (`c1397e0`) and **broke all three corners in the novel composition**. The
  adopted primitives held (the Wesolowski VDF and the Shacham–Waters PoR were
  attacked and denied). Full report: `docs/reviews/M0-REDTEAM-REPORT.md`;
  live status carried in `docs/design/gate4-m0-mechanism.md`. This supersedes
  earlier changelog language that presented the corners as resolved.
  - **Accountability** — 🟢 **FIXED (below, #136).**
  - **Sybil** — 🔴 **BROKEN (F1/F2/F3):** the PoST plot binds only the 32-byte
    block *leaves*, not the block bytes, so a prover holds ~1/128 of the storage
    it is charged for (→0 for small bonds, re-plotted inside the VDF window); and
    the VDF "time" half gates nothing because its challenge input is public.
    Earlier entries claiming "N distinct blobs of real storage" and "cannot
    release the space and re-plot" are **false against this attack** and are
    corrected in-code (`core/bond/bond.go`). Fix = bind to block bytes
    (memory-hard/DRG) + a pre-VDF plot read; mechanism design turn.
  - **Privacy** — 🔴 **BROKEN (F4):** the D3 issuance-mixing layer was never
    shipped, so `AcquireToken` de-anonymizes the publisher at token acquisition
    by IP+timing (and the fee debit). The residual was previously described as a
    "narrowed anonymity set"; in shipped code it is a **singleton** (direct
    de-anonymization). Fix = route issuance over the content-blind relay, epoch
    batch, decouple the fee; privacy design turn.
  - **Consensus (D2)** — 🔴 **BROKEN (F6/F7):** fork-choice weight is the
    subjective local reputation view, not objective on-chain bond, so two honest
    replicas diverge permanently; and cross-height double-backing evades the
    equivocation slash. Fix = objective bond-weighted fork-choice (depends on the
    Sybil fix) + slashing that distinguishes malicious double-backing from honest
    reorg-following; consensus design turn.

### Docs
- **M0 mechanism design turn: per-corner fix write-ups** (2026-08-04) — the three
  broken corners each get a skeptic-readable design doc that names the exact
  break (`file:line`), the adopt-don't-invent fix, the composition, the schema
  touch, and a falsifiable denial with the red-team's own PoC inverted as
  regression. **Sybil (F1/F2/F3)** — `docs/design/m0-sybil-bond.md`: a proven
  depth-robust graph over full-byte labels (closes the 1/128 gap) + a pre-VDF
  plot-read seed (releasing the space forfeits the answer). **Privacy (F4)** —
  `docs/design/m0-privacy-issuance.md`: D3 issuance-mixing — relay + ephemeral
  transport, epoch batching, canonical validator set, and a prepaid blinded-credit
  fee decoupling. **Consensus (F6/F7)** — `docs/design/m0-consensus.md`: objective
  on-chain PoST-bond fork-choice weight + Casper-FFG-style surround-vote slashing
  that spares honest reorg-followers. The Sybil bond is the keystone (consensus
  depends on it); privacy is independent. Linked from
  `docs/design/gate4-m0-mechanism.md`. Design only — no code changed.

### Fixed
- **Consensus (red-team F7): cross-height double-backing resolved — by F6 plus
  sound same-height slashing, without slashing honest reorg-followers**
  (2026-08-04) — the report's F7 (sign fork A@1, sit out B@1, sign B@2 — never the
  same height on both, evading the same-height equivocation slash) is now resolved,
  and the resolution is the honest one rather than a wrong slashing rule. Worked
  through precisely and locked in `core/chain/redteam_f7_test.go`: **(1)**
  same-height double-signing is still slashed (`FindEquivocations`, the
  distinguishable misbehavior); **(2)** cross-height double-backing is *provably
  indistinguishable* from an honest reorg-follow from the blocks alone (a validator
  that attested A@1 then followed a heavier fork to attest B@2 produced identical
  evidence), so any rule slashing "signed two incompatible forks" would slash
  honest validators — a regression — and detection correctly does not flag it (the
  guard test); **(3)** objective fork-choice (F6) neutralizes it anyway — the
  double-backer cannot make both histories stand, the heavier-bond fork wins on
  every replica. The pre-F6 design had planned Casper-FFG surround-vote slashing;
  the analysis shows it is unnecessary here (F6 neutralizes) and, for this exact
  pattern, ineffective (the spans do not surround), so a finality gadget is not
  added for M0. `docs/design/m0-consensus.md` §2b carries the reasoning.

### Changed
- **The default `-token-quorum` publish now uses the prepaid-credit path (closes
  red-team re-verification #4)** (2026-08-04) — the re-verifier confirmed the
  fee-decoupling credit mechanism works but flagged that `cmd/silt/swarm.go` still
  acquired tokens via the legacy `AcquireToken`, so a default token-quorum publish
  still hit `ChargePublish(from)` per publish. `acquirePublishToken` now **mints
  one prepaid credit per validator** (the fee is charged at mint) and **spends
  them** for the k blind signatures, so the publish itself records no per-publish
  fee debit — the credit path the mechanism was built for is now the default
  publish path, exercised end-to-end over real TCP
  (`e2e/TestUnlinkablePublishOverTCP`). The whole flow runs from the swarm client's
  already-ephemeral identity. Residual (deliberately deferred, option B): the
  IP+timing transport link (relay-forced issuance + epoch batching) — NATed clients
  already relay; a public-IP client's issuance IP/timing is the last D3 piece.
- **Objective fork-choice is now the DEFAULT for an untrusted validator (closes
  red-team re-verification #6/#7)** (2026-08-04) — the fix re-verifier confirmed
  objective mode heals divergent replicas but flagged that it was **off by
  default**, so a stock validator swarm still ran the legacy subjective path that
  diverges under partition. `silt daemon -objective` now **defaults to `true`** and
  is active for any untrusted validator (`-min-rep > 0`); a trusted swarm
  (`-min-rep 0`, self-commit) auto-disables it, and the legacy subjective path is
  now an explicit, labeled opt-out (`-objective=false`, which prints that it does
  NOT hold the M0 denial under an adversarial partition). A multi-validator quorum
  still bootstraps from the declared launch `-anchors` (the honest trustless-
  cold-start boundary); without them the daemon warns and a multi-validator swarm
  will not commit, rather than silently running the divergent path. Verified e2e:
  `e2e/TestObjectiveConsensusCommitsOverTCP` now runs with **no `-objective` flag**,
  proving the default path is objective; the legacy-path e2e/example flows opt in
  with `-objective=false`. This makes "two histories both stand" unreachable with
  stock validator flags — the residual the re-verifier asked to close.

### Added
- **Design note: rebinding the storage bond to identity and size (M0 Sybil / G2)**
  (2026-08-05) — `docs/design/m0-sybil-rebind.md`. The Sybil corner is **open**: a
  red-team pass over the F1 fix code broke it again via **prefix plots** (blocks
  `0..m-1` of an `n`-block plot are byte-identical to a standalone `m`-block plot, and
  each prefix has its own distinct Merkle root, so per-root dedup never fires). The
  root cause is that `VerifySpaceTime` checks only Merkle **inclusion** and never
  recomputes a **label** — proof-of-storage, not proof-of-space — while identity is
  asserted by a signature over an attacker-chosen root rather than verified. The note
  specifies the fix (a public, identity- and size-bound plot seed plus a
  labeling-consistency challenge the verifier recomputes without holding the plot),
  its soundness parameters (`k ≥ λ·ln2/ε`), the wire format, the build sequence, and
  the ordering constraints — including that the public seed must **never** land before
  the labeling check, and that the G3 "proof beats declaration" fix is load-bearing for
  its griefing safety. Derived by an independent researcher pass with no build context.
  **Not yet built; M0 is not held.**
- **`silt daemon -honor-chain-revocations` and `-revoke`: operate on-chain
  takedowns, with an e2e proof (F5)** (2026-08-04) — the accountability fix's
  per-operator honoring and quorum-gated, existence-checked revocation are now
  operable from the binary. `-honor-chain-revocations` **subscribes** this
  operator to on-chain takedowns (default OFF — following the chain never imposes
  someone else's takedowns; the operator-local `-denylist` is always honored).
  `-revoke <root>` makes a validator **propose** an on-chain takedown of a root
  once it has earned standing and the root is committed (retried on the loop-safe
  clock; the chain enforces existence + quorum). This completes the F5 test pyramid
  with the **e2e tier** (`e2e/TestChainRevocationCommitsOverTCP`): a validator
  drives a quorum revocation of a published root over real TCP and it commits. The
  per-operator honoring is covered at integration (`sim/revocation_test.go`).
- **Anti-release bond floor (`-min-bond-floor` / `Node.MinBondBytes`): a bond too
  small to be safe against release + re-plot earns no standing (M0 Sybil F1/F2)**
  (2026-08-04) — the byte-binding + read-bound-VDF plot makes a released prover
  recompute (memory-hard) before it can answer, but that only bites if re-plotting
  the pledged size takes LONGER than the challenge window. At the measured plot
  throughput (~270 MB/s, `bond.BenchmarkSeal`) a 500 ms window re-plots ~135 MiB
  and this daemon's ~2 s window ~540 MiB — so a bond at or below that could be
  released and recomputed just-in-time. A bond below `Node.MinBondBytes` now earns
  **no standing**, self or peer, at the live audit: `bondAuditOnce` gates both the
  self-credit and the peer-credit on the floor, so a valid answer for a sub-floor
  plot proves nothing about sustained possession. Exposed as `silt daemon
  -min-bond-floor` (default `0` = off, since every fast test/demo/NAT config uses
  tiny bonds; an open deployment sets it above window × throughput, e.g. `1G`, and
  the daemon warns if `-bond` is below it). Coverage: **unit**
  (`core/node/bondfloor_test.go` — a sub-floor bond earns 0, an at-floor bond earns
  standing) and **integration** (`sim/bond_floor_test.go` — a sub-floor validator
  is denied standing over the live audit wire while an at-floor one earns it).
  `BondVDFDelay` remains the complementary time-floor knob. See
  `docs/design/m0-sybil-bond.md`.
- **`silt daemon -objective`: run consensus on objective on-chain-bond fork-choice
  (F6), with an e2e proof** (2026-08-04) — a validator can now enable objective
  mode from the binary: `-objective` (with `-min-bond`, and requiring `-anchors` +
  `-mature-validators > 0` for the cold-start) wires the on-chain-bond verifier and
  makes the validator register its real bond live as it proposes, so eligibility,
  quorum, and fork-choice weight come from verifiable on-chain bonds instead of the
  local reputation view. This completes the F6 test pyramid with the **e2e tier**
  (`e2e/TestObjectiveConsensusCommitsOverTCP`): two `-objective` daemons bootstrap
  via anchors and drive a real objective quorum commit over real TCP, and the file
  round-trips bit-perfect — the bond-registration-and-verification protocol works
  end to end, not just in the sim. Objective mode remains opt-in at the daemon
  (the default stays the legacy reputation path); flipping the shipped default is
  the remaining step, tracked in `docs/design/m0-consensus.md`.

### Fixed
- **Consensus (F6): the objective-fork-choice cold-start — an anchor-bootstrapped
  validator set that builds itself from real bonds** (2026-08-04) — objective mode
  had a chicken-and-egg: a validator must be bonded ON CHAIN to propose/attest, but
  the first block that records bonds must itself be proposed and attested. It is
  now solved with the existing training-wheels anchors: in objective mode a
  declared anchor is eligible to propose/attest **while the network is immature**
  (`Chain.launchAnchor`), so the declared launch set commits the early blocks;
  validators register their real bonds **live** as they propose
  (`Node.RegisterBondReg`, attached by `proposeBlock`; `Chain.Objective` /
  `NewBondReg` / `BondRegNonce` are the seam); and the anchor eligibility **sheds
  mechanically at maturity** (`Mature()`). It grants **eligibility, never
  fork-choice weight** — weight is always summed real bond, so a declared anchor
  can never outweigh a proven one, and a network that never decentralizes simply
  keeps its training wheels. Coverage: **unit**
  (`core/chain/objective_coldstart_test.go` — an anchor bootstraps an empty
  objective set then sheds at maturity) and **integration**
  (`sim/objective_coldstart_test.go` — an anchor-only network with a separate empty
  ledger per node bootstraps consensus, and proposers become really bonded
  on-chain by self-registration, agreed across replicas). **Residual:** the daemon
  `-objective` flag wiring + an e2e run over real daemons. See
  `docs/design/m0-consensus.md`.
- **Test coverage backfill (build-immutable): the Accountability fix (F5) now has
  the integration tier** (2026-08-04) — the F5 fix (on-chain revocation is
  existence-checked, per-operator opt-in, reversible) had unit + node-white-box
  coverage; this adds the integration tier over the full node loop
  (`sim/revocation_test.go`): a bonded quorum publishes a root, then commits an
  on-chain revocation of it over the wire, and — the load-bearing property — the
  takedown is honored **per operator**: a subscribing node
  (`SetHonorChainRevocations`) denies the root while a node on the **identical
  chain** that did not subscribe does **not** (never a global switch); and a
  quorum cannot revoke a root the chain never committed (`ErrRevokeUnknownRoot`).
  Adds `Node.WouldDeny` — operator-facing observability for the effective,
  per-operator takedown decision. **e2e is explicitly deferred:** the daemon does
  not yet expose chain-revocation *proposing* (no revoke command / auto-propose)
  or the honor-subscription flag, so the full quorum-revocation-honoring flow is
  not drivable end-to-end; it lands when those daemon features do. A stated tier
  choice, not a silent gap.
- **Test coverage backfill (build-immutable): the Privacy fixes (F4) now have the
  integration tier** (2026-08-04) — the fee-decoupling and canonical-issuer-set
  fixes shipped unit-only; this adds the outcome-driven integration tier.
  **Fee decoupling** (`sim/credit_fee_test.go`): a publisher mints prepaid credits
  over the real node loop (charged in bulk at mint), then publishes by SPENDING a
  credit over the wire — and its durable standing key balance is **unchanged by
  the publish** (the ledger-level link severed end-to-end), the token verifies,
  and re-spending a credit over the wire is refused (double-spend). **Canonical
  issuer set** (`core/node/objectivechain_test.go`): two nodes on the same
  objective genesis surface the IDENTICAL deterministic on-chain-bonded issuer
  set, and a node with no chain surfaces none. e2e for the transport-layer parts
  (relay + ephemeral + epoch) lands with those parts, which are not yet built.
- **Test coverage backfill (build-immutable): the Sybil fix (F1/F2) now has the
  integration tier it was missing** (2026-08-04) — the shipped Sybil fix carried
  only its unit tier (the red-team PoC inverted in `core/bond`); the build-immutable
  rule (V5) requires unit + integration + e2e. Added the **integration** tier:
  `sim/bond_release_test.go` drives the property through the live audit wire
  (gossip → `MsgBondChallenge` → answer → `VerifySpaceTime` → ledger) — a
  validator that pledges a bond, advertises it, then RELEASES the resident bytes
  (holding at most the 32-byte leaves, the attacker that frees the space to save
  disk) FAILS the live audit and earns ZERO standing, while an honest full-plot
  validator earns it. A `bond.Commitment.ReleaseBlocks` / `Node.ReleaseBond`
  adversary seam (cf. `SetLiar` for PoR) models the release. **e2e** is already
  covered by `e2e/TestBondEarnedStandingCommitsOverTCP` (two real daemons proving
  bonds to each other over real TCP, exercising the fixed read-bound-seed
  protocol); the released/leaves-only adversary is proven at unit+integration
  rather than e2e because forcing it end-to-end would mean shipping attack
  behavior in the production binary — an explicit, stated tier choice, not a
  silent gap.
- **Consensus (red-team F6): objective fork-choice is now wired into the node,
  with integration + unit coverage** (2026-08-04) — the F6 objective-weight
  mechanism (on-chain `BondRegs`) previously existed only in `core/chain` behind a
  verifier a caller had to supply. A node now wires it in one call:
  `Node.EnableObjectiveChain` injects the real space-time bond verifier
  (`bond.VerifySpaceTime`, the same check the audit loop runs), and
  `Node.RegisterBondReg` mints a signed registration from the node's held bond for
  live entry into the objective set (`chain.NewBondReg` / exported
  `chain.BondRegNonce`; `EnableBond` now records the identity signer so a bonded
  node can register before it joins consensus). Coverage now spans all three
  tiers per the build-immutable rule: **unit** — a live registration round-trips
  through the real verifier and a tampered space-time proof is rejected
  (`core/node/objectivechain_test.go`); **integration** — the red-team's
  non-healing-partition scenario inverted, with a **separate empty ledger per
  node** (so the local reputation view is useless, unlike `sim/reorg_test.go`'s
  shared ledger): the partition still commits and heals to the heavier-bond fork
  on every replica (`sim/objective_consensus_test.go`). **Residual:** turning
  objective mode on by default in the daemon (a genesis/anchor-seeded validator
  cold-start plus a live-registration submission path), and an e2e multi-process
  run, remain; see `docs/design/m0-consensus.md`.
- **Privacy (red-team F4 §2c): a canonical, on-chain issuer set so the validator
  subset a publisher asks leaks nothing** (2026-08-04) — a publisher previously
  acquired publish tokens from whatever validator subset its `-peers` gave it, so
  a colluding issuer minority could narrow the anonymity set by *which* validators
  a given publish asked. `Chain.CanonicalIssuers` (and `Node.CanonicalIssuers`)
  now derives a **deterministic** issuer set from the **on-chain bond** (the same
  objective `bonded` map that heals fork-choice, F6): bonded validators ordered by
  size then NodeID, identical on every replica. Every publisher asks the same
  validators, so the subset choice carries no signal. Regression
  (`core/chain/redteam_consensus_test.go`) proves two maximally-divergent replicas
  produce the identical ordered set. This is one of the three network-layer parts
  of D3; the transport parts (routing issuance over the content-blind relay from
  an ephemeral identity, epoch batching) are still pending, so IP+timing
  correlation remains until they land. See `docs/design/m0-privacy-issuance.md`.
- **Privacy corner (red-team F4): the per-publish fee no longer links a publish
  to its standing key** (2026-08-04) — token issuance de-anonymized the publisher
  two independent ways: over a non-anonymous transport (IP+timing) and via
  `ChargePublish(from)`, a **per-request debit of the durable standing account**.
  This lands the fee decoupling — **prepaid publish credits** (online Chaumian
  e-cash, Chaum 1982). A credit is a blind signature under the issuer's key but in
  a **separate FDH domain** (`blindtoken.BlindCredit`/`VerifyCredit`), so a credit
  can never be presented as a publish token or vice versa even under one key. The
  fee is charged **in bulk at mint** (a normal, charged token request blinded in
  the credit domain); at publish the requester **spends a credit** (`Message.Credit`,
  verified and marked spent in an online double-spend set) and the issuer does
  **not** charge the durable identity — severing the ledger-level link. The change
  is purely additive: a request with a credit spends it (no debit), a request with
  none takes the legacy charged path, so existing token flows are unchanged
  (whole suite + vet + `-race` green). New helpers `Node.AcquireCredits` (bulk
  mint) and `Node.AcquireTokenWithCredits` (spend). Regressions in
  `core/node/redteam_privacy_test.go` show a mint charging once, a publish
  charging nothing more, a spent credit refused (double-spend), a forged credit
  refused, and the credit/token domains proven non-interchangeable. **Residual
  (honest):** the **network-layer link** — routing issuance over the content-blind
  relay from an ephemeral identity, epoch batching, and a canonical validator set
  — is **not yet built**, so a colluding issuer minority can still correlate by
  IP+timing; the privacy corner does not fully hold until that lands. See
  `docs/design/m0-privacy-issuance.md`.
- **Consensus corner (red-team F6): fork-choice is now objective — honest
  replicas stop diverging** (2026-08-04) — fork-choice weight, the quorum count,
  and proposer/attester eligibility used the **local reputation view**
  (`c.rep(id)`), so two honest validators that had audited different peers
  computed different weights and forked permanently (the partition never healed).
  Fork-choice is now driven by **on-chain PoST-bond registrations**
  (`Block.BondRegs`): a validator records its bonded size with a fresh
  space-time proof any replica re-verifies (`SetBondVerifier`), bound to the
  block's parent so it can't be replayed to another height/fork and signed so it
  can't be claimed by a non-holder. Weight becomes the summed on-chain bond of a
  block's distinct attesters — a quantity **every replica recomputes identically
  from the chain** — so divergent local views can no longer disagree on which
  fork is heavier, and a lighter fork reorgs onto the heavier one on every honest
  node. The mechanism is **additive and opt-in** (`Config.MinBond > 0`): the field
  is `omitempty` so a block with no registrations hashes exactly as before (no
  `BlockVersion` bump), and the default path is unchanged — the legacy
  reputation-gated behavior and every existing test/sim are untouched. Regressions
  in `core/chain/redteam_consensus_test.go` show two maximally-divergent replicas
  computing the same weight, a partition healing to the heavier-bond fork, and a
  forged registration (bad proof or bad signature) denied. **Residual (honest):**
  the objective-mode wiring in the node/daemon (validators emitting registrations,
  a genesis-seeded validator cold-start, enabling `MinBond` in production) is a
  follow-up, and **F7** — cross-height double-backing evading the same-height-only
  equivocation slash — is not yet fixed (it needs Casper-FFG-style surround-vote
  slashing that spares honest reorg-followers). See `docs/design/m0-consensus.md`.
- **Sybil corner (red-team F1/F2/F3): the PoST bond now binds the bytes it
  charges for, and the VDF is bound to a plot read** (2026-08-04) — the external
  M0 red-team broke the Sybil corner three ways; the first two are now fixed at
  the mechanism level (`core/bond`), per `docs/design/m0-sybil-bond.md`.
  **(F1)** `plotBlock` derived each 4 KiB block from only the 32-byte *leaves* of
  its predecessor and parents, so a prover could store just the leaves (1/128 of
  the bond) and recompute any probed block on demand. Each block now depends on
  the **full bytes** of its predecessor and its parents, selected over a **proven
  depth-robust graph** (DRSample, Alwen–Blocki–Harsha CCS'17) instead of the old
  flat-uniform parents — so reconstructing a block requires the parents' bytes
  recursively and the pebbling cost is Ω(n); the rational strategy is to store
  the S bytes, and the charged size equals the resident footprint. `Verify` never
  recomputes a block, so it stays O(log n). **(F2)** `AnswerSpaceTime` seeded the
  VDF from the *public* `challengeSeed(root, nonce)`, so a zero-resident prover
  ran the VDF and then re-derived the sampled blocks — releasing the space
  forfeited nothing. The VDF is now seeded from a plot block **read before the
  VDF** (`seedIndex` → `challengeSeedST`): the answer carries that block plus its
  inclusion proof, the verifier recomputes the seed index and checks the proof,
  so a prover that released the space cannot produce the seed without the Ω(n)
  recompute. **(F3)** root-owner dedup is documented as only a same-root tiebreak;
  Sybil cost now lives in the byte-bound proof, and distinct identities still
  produce distinct plots. The plot on-disk format (`adapters/diskplot`) bumps to
  **version 2** so a restart re-plots rather than reloading the old, insecure
  labeling (one-time re-plot on upgrade). The red-team PoCs are adopted inverted
  as regressions (`core/bond/redteam_sybil_test.go`), and `BenchmarkSeal` records
  the plot/re-plot constant (~270 MB/s) behind the "re-plot ≫ epoch" tuning.
  **Residual (honest):** the *structural* anti-release binding is in; the
  *quantitative* floor — a minimum bond size and `BondVDFDelay` such that even the
  smallest allowed bond cannot re-plot within one challenge window — is a
  deployment-tuning follow-up, and consensus fork-choice weight (F6) still depends
  on this bond being real. See the design doc's open-risks section.
- **Accountability corner (red-team F5): on-chain revocation is no longer a
  global switch** (2026-08-04) — the external M0 red-team broke the
  accountability tenet three ways through the chain's takedown path: a quorum
  could revoke a root it never published (no ownership or existence check); the
  takedown was honored by **every** chain-follower with no opt-out — a global
  switch the tenets say cannot exist; and it was irreversible. All three are
  fixed. **(1)** `ValidateProposal` and the commit path now reject a block whose
  `Revocations` name a root never committed on this chain (`ErrRevokeUnknownRoot`)
  — a quorum cannot censor content that isn't on the ledger, nor a competitor's
  unpublished hash. **(2)** Honoring on-chain revocations is now a **per-operator
  subscription** — `ReplicaRegistry.HonorRevocations` and
  `node.SetHonorChainRevocations`, both default **off** — so following the chain
  never silently imposes someone else's takedowns; the effect is "proportional to
  who trusts you" (TENETS §9), the same voluntary stance as the operator-local
  denylist, never a universal switch. **(3)** Added an **un-revoke** record
  (`Block.Unrevocations`, quorum-gated and committed in the block hash) so a
  takedown is reversible by the same governance that imposed it, not a permanent
  asymmetry. The red-teamer's own PoC now fails at its ownership check; adopted
  inverted as `core/chain/redteam_f5_accountability_test.go` and
  `core/node/redteam_f5_subscription_test.go` (unit + node-integration; the
  operator-local takedown sim remains the e2e). Traces to immutable #5, Don't #2,
  S4. **The other red-team breaks (Sybil bond F1/F2, privacy issuance F4,
  subjective fork-choice F6, cross-height equivocation F7) remain open — see the
  M0 status note below — this fix closes the accountability corner only.**
- **Doc-truth reconciliation + a token round-trip playbook (acceptance round 3)**
  (2026-08-03) — the third acceptance re-run PASSED again (all 9 flows, all 8
  tenets, **zero code defects**); its findings were stale docs and one
  discoverability gap, several making the product look *worse* than it is. **(F1)**
  `risk-register.md` row 14 claimed a default publish still writes a permanent
  `Publisher→root` map — but the chain default now REJECTS Publisher entries
  (`-allow-publisher=false`), so a default publish records no author; updated to
  CLOSED-by-default, with blind tokens as the additional opt-in for full
  unlinkability. **(F2)** `threat-catalog.md` F1 still said "the RSA issuer key is
  in-RAM (persistence is a follow-up)"; it persists now (#126, `adapters/diskissuer`)
  — corrected. **(F3)** the website said publishing is "cryptographically
  unlinkable" as an unqualified property; qualified to "names no author by default —
  with opt-in blind tokens, cryptographically unlinkable," matching the honest
  in-repo docs. **(F4)** the headline walkthrough (`local-test-network.md`) never
  reached the trust-plane flows 4–7; added a "Tier 4 — become a validator" section
  pointing at `examples/` and `user-seam.md` §Role 4. **(F5, doc-note only per
  decision)** documented that a denied root reads to a fetcher as ordinary
  data-loss (compliant nodes answer "not found" rather than advertising a refusal —
  deliberate; the fetcher retrieves from another operator). **(F6)** the F7
  sub-claim "the tokens it issued stay valid across a restart" had no operator-level
  repro; added **`examples/flow-tokens-issuer-restart.sh`** — validators require
  blind tokens, a tokened publish commits (no Publisher), the issuer is restarted
  (its `issuer.key` reloads byte-identical, no re-mint), and a token issued by the
  restarted issuer still commits (peers accept it), with a token-less-publish-refused
  negative control. Also made `silt chain-status`'s hint line un-ambiguous to grep.
  No mechanism changed. Traces to **S5**.
- **Docs & UX polish (acceptance re-run new-F3/F4/F5/F6)** (2026-08-03) — four
  minor/cosmetic gaps the passing re-run surfaced, each a small correctness or
  clarity fix, no mechanism change. **(F3)** the Tier-1 "erasure by hand"
  walkthrough listed objects as flat under `.silt/objects/` and told you to
  `rm .silt/objects/<a-few>` — but objects nest one level under a 2-hex prefix
  (`.silt/objects/<xx>/<hash>`), so that command targets a whole prefix
  directory, and it could delete the single-copy manifest chunk and brick `get`;
  rewritten to use `silt info … -shards` to pick real data/parity shard hashes,
  delete them by their true path, and warn the manifest is single-copy on one
  node (`README.md`, `docs/local-test-network.md`). **(F4)** `silt daemon -h`
  described `-registry` as `http://host:port` — the exact form the key-pinning
  contract *refuses*; the flag help now reads `ID@https://host:port (key-pinned —
  copy the daemon's 'registry:' line verbatim)`. **(F5)** the website's feature
  list didn't mention NAT traversal (thoroughly documented in the repo but
  invisible to a site visitor); added a "Reaches across NATs" card. **(F6)**
  `silt get <siltcare:…>` refused with `link: not a silt:v1: link`, which reads
  like a typo rather than an intentional capability boundary; `link.Parse` now
  recognises a care link and says so, and `silt get` points to `silt info` /
  `silt daemon -care` and the full link (unit test pins the clearer error).
  Traces to **S5**. See the M0 acceptance re-run report.
- **Gate 4 (#52, acceptance F1): a restarted validator rejoins the chain instead
  of being stranded at its pre-restart height** (2026-08-03, D2) — the M0
  acceptance field test found the one blocker: kill a validator, let the network
  commit a block without it, restart it on the same `-store`, and it never caught
  up — it sat at its old height forever while the live set advanced, so over time
  the validator set could only shrink. Two compounding causes, both rooted in the
  same mistake — treating *reputation* (a live, local, NON-persisted view, re-earned
  by bond audits) as if it were a property of a *persisted* block. **(1) Reloading
  our own chain** re-ran every block — including the genesis — through the full
  commit gate (`chainstore.Replay` called `chain.Append`), so at boot, before any
  bond audit had run, the empty reputation view failed the very first block:
  `reputation below threshold: proposer <genesis-id> has 0, needs 100`. The genesis
  is designed to *bypass* that gate (`AppendGenesis`); replaying it through the gate
  cannot work. **(2) Catching up on missed blocks** fired `SyncChain` exactly once,
  at boot, gated on `-attesters`, and BEFORE `StartBondAudit` — so it ran against an
  empty reputation view (adopting nothing, since it can't yet tell which fork carries
  real standing) and then never retried. The in-process `consensus` sim hid both
  because it PRE-POPULATES reputation before the latecomer syncs. **The fix draws the
  trust boundary at whose disk it is.** Our OWN committed history is reloaded by
  `Chain.Reload`, which re-verifies each block's cryptographic integrity — hash
  ancestry, the proposer signature, and a quorum of distinct verifying non-proposer
  attester signatures (so bit-rot, truncation, or tampering is still caught, B7) —
  but NOT the time-varying reputation gate, which a validator already satisfied when
  it committed the block live; genesis reloads via `AppendGenesis` as it always
  should have. A PEER's fork is a different trust class and still goes through
  `Reconcile` with full reputation re-validation. Catch-up is now a periodic,
  retrying `StartChainSync` loop (`ChainSyncInterval`, default 30s), UNGATED on
  `-attesters` (it targets the explicit set plus every validator learned from a
  gossiped bond, so a node restarted with only `-bootstrap` still rejoins), and the
  daemon runs it AFTER `StartBondAudit` so peer standing is being re-earned — a later
  sweep, once audits land, adopts the missed blocks and persists them. Tested (V5):
  unit — replaying our own `[genesis, block1]` with an EMPTY ledger now rejoins at
  height, while a tampered block is still rejected (`ErrBadSignature`); node — a
  restarted validator adopts NOTHING while its standing view is empty and catches up
  the instant bond audits restore peer standing, and `syncTargets` includes a
  bond-learned validator with no `-attesters` given. Honestly labelled: fork-choice
  weight is still the locally-qualified reputation view (fully-objective,
  partition-independent on-chain PoST-bond weight remains the recorded D2 hardening),
  and a bespoke multi-daemon restart harness is deferred to the acceptance re-run —
  the field test roadmap #52 exists to prove. Traces to **M0**, **B7**, **D2**,
  **#52**. See `docs/design/gate4-m0-mechanism.md` §3e.
- **Gate 4 (acceptance F2/F7): the trust plane narrates itself — an operator can
  SEE standing, bond reload, and caretaker sweeps** (2026-08-03, S5) — the M0
  mechanisms worked but ran silent, so the acceptance operator had to read source
  to confirm the earned-standing and self-heal claims. Four honest-observability
  fixes, all at `-log info`: **(standing)** a validator now narrates its own
  consensus standing every bond-audit sweep and the verdict of every peer bond
  challenge (`standing`, `bond challenge`), so the earned-standing mechanism the
  whole of M0 rests on is visible rising and decaying rather than inferred from a
  diffed `chain.cbor`; **(bond reload)** a restart that RELOADS its plot now says
  `reloaded the … bond (no re-plot)` instead of the identical `sealed …` wording a
  first-time plot uses — the "no re-plot" guarantee held, but the log had actively
  suggested the expensive path ran (`EnableBond` now reports reloaded-vs-sealed);
  **(caretaker)** the repair sweep logs `stripe degraded, within repair slack —
  watching` when it sees a loss that parity/replication still covers, so an
  operator who kills a holder sees the caretaker NOTICE rather than apparent
  silence — repair fires (`stripe repaired`) only once losses exceed the slack,
  which with the default replication takes more than "a couple" of deaths, and
  `repair below k` already marks the can't-yet-reconstruct case; **(default on)** a
  validator with no `-log` flag now defaults to `-log info` — the M0 stakes mean
  the normal path should narrate itself in the field, not stay dark until someone
  knows to ask (non-validators are unchanged: logging stays off). The flagship
  self-heal walkthrough (`docs/local-test-network.md`) is rewritten to set honest
  expectations (why killing "a couple" of holders correctly heals nothing visible,
  how to actually strand a stripe, and `silt sim run churn` for the dense version).
  A read-only `Reputation` accessor was added to the `CreditLedger` port for the
  narration. No mechanism changed — this is pure observability. Traces to **M0**,
  **S5** (honest observability), **B5**. See the M0 acceptance report.
- **Docs (acceptance F4/F5/F6/F8): the getting-started guides match reality**
  (2026-08-03) — the acceptance operator hit four first-five-minutes doc snags,
  none breaking the product but each eroding "every step works / every counter
  reproduces": **(F4)** three guides (`README.md`, `docs/local-test-network.md`,
  `docs/v1-test.md`) said `add` "prints the root hash" / a "64-char hex string"
  then told you to `get <root>` — but `add` prints a full `silt:` **link** and
  `get`/`info`/`swarm get` need that whole link, so a literal newcomer hit an
  error; every such placeholder is now `<silt-link>` with the output described as
  a link (the top-level `silt` usage block was already correct). **(F5)** the
  quoted `sim run economy -seed 21` figures were stale — refreshed to the actual
  deterministic output (Gini 0.00 → 0.63, top earner ~1.25 MB, freeloader ~444 KB,
  20/36 second-round publishes ok). **(F6)** the `silt sim run` usage error listed
  only `scatter` and the top-level usage omitted half the scenarios — both now list
  all eight (`scatter, churn, economy, audit, capacity, consensus, bondstanding,
  takedown`), including the previously undocumented `bondstanding`. **(F8)** the
  `user-seam.md` store-layout table listed `chain/` (a directory); the committed
  history is a single `chain.cbor` file. Traces to **S5** (honest observability
  extends to the docs). See the M0 acceptance report.

### Added
- **Validator onboarding (acceptance re-run new-F1/new-F2): `silt id`, `silt
  chain-status`, and a runnable `examples/` playbook** (2026-08-03) — the M0
  acceptance re-run PASSED (all 9 flows, all 8 tenets, zero `broken`), leaving
  two "major" gaps that both blocked a literal newcomer from the validator flow
  without changing any mechanism. **(new-F1)** Role-4 setup was chicken-and-egg:
  `-attesters <ID_B>` needs B's NodeID, but nothing told you how to learn it
  before launch (the acceptance script resorted to booting a throwaway daemon to
  read its `peer:` line). New `silt id [-id-seed N | -store DIR] [-listen ADDR]`
  prints the NodeID a daemon *would* use without launching one — resolving the
  identity exactly as the daemon does — so the topology is wireable up front.
  **(new-F2)** there was no operator playbook for the multi-validator flows 5–7
  and no way to confirm convergence except hashing `chain.cbor` by hand. New
  read-only `silt chain-status [-store DIR]` prints a replica's head height, head
  hash, and block/entry counts — identical head height AND hash across replicas
  proves they agree; a rising head after a restart proves catch-up. And a new
  top-level **`examples/`** directory ships four bash playbooks
  (`flow2-publish-fetch`, `flow4-earned-standing`,
  `flows567-convergence-fault-restart`, `flow8-takedown`) — the flows-5–7 script
  IS the field test roadmap #52 owes itself, now runnable in one command. The
  playbooks track only the PIDs they start (no blanket `pkill`) and use both new
  commands. `docs/user-seam.md` Role 4 gains a concrete `silt id`-based recipe
  and points at `examples/`. All four playbooks pass end to end locally
  (including the restarted-validator chain catch-up on real daemons — the
  daemon-level confirmation of the F1 restart fix). Traces to **S5** (an operator
  can see and reproduce what's true), **#52**. Adopted from the M0 acceptance
  reproduction scripts.
- **Gate 4d (#93): the publish-token issuer key persists across restarts**
  (2026-08-03) — a validator that issues blind-signed publish tokens generated a
  FRESH RSA key on every daemon start, which orphaned every token it had already
  FRESH RSA key on every daemon start, which orphaned every token it had already
  signed (they no longer verify) and staled every issuer public key its peers had
  cached. A new `adapters/diskissuer` persists the key (PKCS#1 DER, written
  atomically with `0600`), and the daemon **loads-or-creates** it: first run mints
  the issuer identity, every restart keeps it — so outstanding tokens stay
  verifiable and the distributed issuer set is stable. A corrupt or foreign key
  file is a hard error, never silently overwritten with a new identity. Tested
  (V5): the restart property is pinned (two `LoadOrCreate`s over the same dir
  return the same key), plus save/load round-trip, clean-absent, and
  corrupt-file handling; the real daemon (e2e + Docker NAT) starts and persists
  the key. Honestly labelled: this is the issuer-key half of §3d's "issuer
  survives restart"; **on-chain issuer registration** (so the qualified issuer
  set is chain-verifiable rather than fetched ad-hoc) is the remaining §3d piece,
  and it pairs with the deferred D3 canonical-validator-set work. Traces to
  **M0** (the unlinkable-publish path stays live across restarts), **B7**. See
  `docs/design/gate4-m0-mechanism.md` §3d.
- **Gate 4f (#100): equivocation is provable and slashable — double-signing
  costs standing** (2026-08-03, D2) — the consensus analogue of a storage liar:
  a validator that signs two DIFFERENT blocks at the SAME height (trying to make
  two competing histories both look supported) is now caught and penalised. Two
  parts: **(prevention)** an honest validator records the block hash it signed at
  each height and REFUSES to sign a different block there — it never equivocates,
  even if two competing proposals reach it before either commits; **(penalty)** a
  `chain.Equivocation` is a compact, self-verifying proof (the two conflicting
  blocks; any node recomputes their hashes, confirms same height + different
  block, and that the culprit's signature — as proposer OR attester — verifies in
  both), and `chain.FindEquivocations` extracts every cross-fork double-signer
  from two competing histories. When a node reconciles across a fork it slashes
  each proven equivocator in its local ledger (`credit.SlashEquivocation`), a
  crushing, permanent reputation penalty that buries the culprit below any
  threshold — so its proposals are refused and its attestations stop counting
  toward any fork's weight. An honest validator signing sequential heights is
  never implicated (the heights differ) and a forged accusation fails (the
  signatures won't verify). Tested (V5): unit — a double-sign is provable, a
  sequential signer and an unsigned accusation are not, the same block is not a
  conflict, and every cross-fork culprit is found while one-fork signers are
  spared; node — a validator REFUSES a second block at a height it attested, and
  reconciling across a fork slashes the double-signer below zero. Honestly
  labelled: strict lock-on-attest can stall a height's liveness if a proposal
  fails and its attesters are needed again there — proper resolution is
  round-based unlocking (Tendermint POLC), a recorded 4f hardening; on-chain
  equivocation records so every replica slashes in lockstep (vs. each acting on
  what it observes) is the other recorded follow-up. Traces to **M0** (a
  double-signing proposer cannot stand two histories AND keep its standing),
  **D2**. See `docs/design/gate4-m0-mechanism.md` §3e.
- **Gate 4f (#100): the chain can reconcile forks — reorg to the heavier
  history** (2026-08-03, D2) — the registry chain was append-only with no
  reorganisation ("first valid block at a height wins"), and `SyncChain`
  silently `break`ed on divergence, so a partitioned or diverged validator
  stayed forked forever. It now heals: `Chain.Reconcile` re-validates a peer's
  full chain end to end in a throwaway replica and, iff that history is strictly
  heavier (ties broken by the lower head hash, so every honest node picks the
  same winner), **adopts it** — rolling state back to the shared genesis and
  forward onto the heavier fork. Because all derived state (`byRoot`, `spent`,
  `revoked`, `validatorsSeen`) is a pure function of the blocks, the reorg is a
  whole-state swap, not fragile per-record undo. Fork-choice weight is the
  cumulative count of DISTINCT qualified non-proposer attestations across the
  chain — the heaviest history is the one the most *earned standing* has
  committed to, not merely the longest (which a fast Sybil could extend);
  signatures are objective, the qualification bar is the local reputation view
  (which converges among honest replicas). The fork is genesis-anchored, so a
  peer cannot swap in a heavier FOREIGN chain, and every block is re-validated,
  so a lying peer wastes time but cannot feed an invalid history. `SyncChain`
  now reconciles against each peer's full chain — one uniform path for catch-up,
  fork-heal, and no-op (an equal-length fork is invisible to "give me blocks
  above my head", which is why it compares whole chains). Tested (V5): unit —
  a heavier fork is adopted, a lighter one rejected, ties break deterministically
  by hash, a foreign genesis is refused, an under-quorum fork is re-validated and
  rejected; integration — a 10-node network **partitions, each side commits its
  own history, then heals and the lighter side reorgs onto the heavier fork over
  the wire while the heavier side does not budge**. Honestly labelled:
  fully-objective, partition-independent on-chain PoST-bond weight is the
  recorded D2 hardening (a self-asserted or locally-qualified weight can diverge
  under an adversarial partition); equivocation evidence + slashing is the next
  4f increment; genesis-to-head diffs (vs. whole-chain fetch) are the scaling
  follow-up. Traces to **M0** (consensus can't be captured by an off-head or
  partitioning proposer), **D2**. See `docs/design/gate4-m0-mechanism.md` §3e.

### Changed
- **Gate 4b (#91): bind the bond plot to its identity — close the
  plot-amortisation gap** (2026-08-03) — the Sybil cost only holds if each
  identity holds its OWN distinct plot; previously nothing stopped a single
  operator from pointing N node identities at ONE shared plot (all advertising
  the same root, answering from one copy on disk), collapsing the per-identity
  cost from S to S/N. Two changes close it, together: **(C)** the plot is now
  sealed from a per-identity **secret** derived from the node's signing key
  (`EnableBond` takes the signer; `bond.Seal` takes the secret) rather than the
  public NodeID — so only an identity's owner can generate its plot, and an
  outsider cannot precompute a *victim's* root to grief it; and **(A)** the
  ledger binds each bond root to the first identity that proves it
  (`RecordBondChallenge` gains a `root`; a per-root owner map), so a root builds
  standing for **at most one identity** — N identities sharing one plot earn one
  bond's worth of standing, not N, forcing N distinct plots = N×disk. Honest
  identities never collide (distinct secret ⇒ distinct root), so the dedup only
  ever bites deliberate sharing. This upgrades design §6's open amortisation
  question from "hand it to the red-team" to a built defence — noting it is
  still not a proof of *correct* plotting (no PoRep/SNARK); the secret + dedup
  make sharing a root un-grief-able and uneconomical rather than impossible.
  Tested (V5): the M0 outcome is pinned — three identities proving one shared
  root leave only the first with standing while a distinct plot earns normally
  (failing-first: without dedup all three would clear the bar); distinct
  secrets yield distinct roots; and the over-the-wire bond audit + restart
  reload paths stay green under the new derivation. Traces to **M0** (the Sybil
  corner), **D1**. See `docs/design/gate4-m0-mechanism.md` §3b/§6.
- **Gate 4b (#91): the bond is now proof-of-space-TIME — the VDF is wired into
  the live bond audit** (2026-08-02) — completes the mechanism: standing is
  backed not just by held space (the plot) but by space held *across time*. A
  bond challenge now answers with a `core/vdf` proof over the fresh
  `(root ‖ nonce)` challenge, and the probed plot-block indices are derived from
  the *VDF output* — so a prover cannot know which blocks to keep ready until it
  has done `BondVDFDelay` sequential squarings, and therefore cannot release the
  pledged space and re-plot just-in-time, nor parallelise its way out of the
  elapsed-time floor. Verification stays O(log n) (checking a VDF is fast even
  though producing it was slow) plus the existing Merkle checks, so consensus
  cost on the core loop is unchanged. `core/bond` gains `AnswerSpaceTime` /
  `VerifySpaceTime` (additive — the space-only `Answer`/`Verify` remain), the
  answer carries the VDF proof inside the existing CBOR `Answer` (so no wire
  format change), and `core/vdf` gains `Default()` — the RSA-2048 challenge
  modulus, an unknown-order group needing no fresh trusted setup (a documented
  launch anchor; class groups are the setup-free upgrade). `BondVDFDelay` is a
  new node-config tuning knob (Evolving): a modest default keeps the
  deterministic sim fast, a real deployment raises it for a stronger time floor;
  `0` disables the time binding. The daemon inherits it from `DefaultConfig`
  (the #65 dropped-field discipline), and the `bondstanding` sim now exercises
  the whole space-time path over the wire. Tested (V5): held bonds answer, a
  space-only answer / wrong-delay / forged-VDF-output all fail, and the probed
  blocks provably derive from the work not the raw nonce. Honestly labelled:
  producing the VDF currently runs on the audit path; moving the heavy work
  fully off the core loop and persisting the plot across restarts (B2 / #93) is
  the next 4b step. Traces to **M0** (Sybil corner: space held across time),
  **D1**, **B2**. See `docs/design/gate4-m0-mechanism.md` §3b.
- **Gate 4b (#91): the bond is now a real space-hard plot, not independent
  blocks** (2026-08-02) — replaces the honestly-labelled placeholder in
  `core/bond` (each block was cheap iterated SHA-256 over `id‖index`, so an
  attacker could recompute any block on demand and store nothing) with a
  **sequential labeling plot**: block `i` depends on its identity, index,
  immediate predecessor, and a few pseudo-random *earlier* blocks (a chain plus
  long-range parents — a DAG). Because a block depends on earlier ones,
  recomputing a single probed block forces recomputing its whole dependency
  subgraph, and the long-range parents defeat cheap checkpointing — so the
  rational strategy becomes to **store the S bytes**, which is exactly the space
  being charged for. This makes N Sybil identities cost N distinct blobs of real
  disk, the property the reputation→quorum path always assumed but never charged.
  The challenge/answer/verify seam is untouched — `bond.Verify(root, size,
  nonce, Answer)` stays a stateless O(log n) Merkle check — so only *what fills
  the blocks* changed. Honestly labelled: space-hardness is heuristic (not yet a
  formally depth-robust graph or a memory-hard label function — the hardening
  path), and the *time* half (binding a fresh epoch challenge to the `core/vdf`
  delay so the space must be held across time and the challenge can't be
  precomputed) is the next 4b step. Tested (V5): determinism + identity-binding,
  the dependency lever (perturbing a predecessor or long-range parent changes
  the block — the space-hardness property the old independent blocks lacked),
  and parent indices are always earlier + deterministic. Traces to **M0**
  (Sybil corner), **D1**. See `docs/design/gate4-m0-mechanism.md` §3b.

### Added
- **Gate 4b (#93): the bond plot persists — a restart reloads it, never
  re-plots** (2026-08-03) — plotting the identity bond is deliberately expensive
  (that expense is the Sybil cost), so paying it again on every daemon restart
  would be wasteful and, for a large pledge, a long stall before a validator can
  prove standing. A new `adapters/diskplot` store persists the plot (one atomic
  file per identity: a small header with the block geometry and committed root,
  then the raw blocks), and `EnableBond` now **loads-or-plots**: if a persisted
  plot exists it is reloaded and its Merkle root **re-derived from the bytes and
  checked against the committed root** (B7 — persisted state is re-verified, not
  trusted), so a restart skips plotting entirely; a corrupt, truncated, or stale
  plot is detected and cleanly re-plotted. `core/bond` gains `Reconstruct` (rebuild
  a commitment from persisted blocks) and `Blocks()`; a new `ports.PlotStore`
  seam keeps the node pure (nil = memory-only, fine for sims). The daemon wires
  it alongside the proof store (inheriting the #69/#93 restart discipline).
  Tested (V5): the adapter round-trips and flags truncated/foreign files; a
  reloaded bond answers a space-time challenge; and the node-level restart
  outcome is pinned — a second start with the same identity **reloads instead of
  re-plotting** (asserted via plot/reload counters), while a corrupted plot
  re-plots to the correct identity-bound root. Traces to **M0**, **D1**, **B7**.
  See `docs/design/gate4-m0-mechanism.md` §3b/§3d.
- **Gate 4b (#91): verifiable delay function primitive (`core/vdf`)** (2026-08-02)
  — the sequential-work core of the proof-of-space-*time* bond, and the first
  4b construction piece. A VDF evaluates in a prescribed number of *inherently
  sequential* steps (you cannot parallelise your way to the answer) yet emits a
  short proof anyone verifies almost instantly — exactly what a bond needs to
  bind a fresh epoch challenge to real elapsed, non-parallelisable time, so a
  Sybil can neither retroactively fake having held its pledged space across the
  epoch nor buy its way out of the wall clock with more cores. The construction
  is Wesolowski's VDF (EUROCRYPT 2019), adopted not invented (B8): over a group
  of unknown order (`Z_N^*` for an RSA modulus `N`), `y = x^(2^T) mod N` by `T`
  sequential squarings, with `π = x^(⌊2^T/ℓ⌋)` for a Fiat–Shamir prime `ℓ`
  computed in `T` steps via long division (never materialising the `T`-bit
  exponent), and verify `π^ℓ·x^r ≟ y` for `r = 2^T mod ℓ` in O(log ℓ + log T) —
  cheap enough to stay on the core loop. Security rests on `N`'s factorisation
  being unknown (a documented trust anchor; the class-group variant removes it
  and is the noted upgrade path). Pure package (big integers and bytes only).
  Adversarially tested: relabelling a shorter computation as a longer one, a
  trivial `π=1`, tampered `y`/`π`, wrong-challenge, wrong-`T`, and non-canonical
  elements all fail; the delay loop is pinned against a direct `x^(2^T)`
  reference. Wiring the plot + epoch proof off-loop behind the existing
  `bond.Verify` seam is the next 4b change. Traces to **M0** (the Sybil corner:
  space-time held, not asserted), **D1**, and **B2** (the heavy work runs off
  the core loop). See `docs/design/gate4-m0-mechanism.md` §3b.
- **Gate 4a (#90): wire the real proof-of-retrieval into the live audit path**
  (2026-08-02) — the `core/por` primitive now *replaces* the toy scheme in the
  running node. An auditor verifies that a peer still holds a shard **without
  fetching the bytes**: at distribute time the publisher computes each shard's
  per-block authenticators under a key derived from the file's layout key
  (`node.DerivePorKey`, mirroring the link key hierarchy) and ships them beside
  the Merkle proof (`StorageProof.PorTags`); the storage node keeps them with
  the chunk; on challenge the prover aggregates its bytes + tags into a compact
  `(μ, σ)` response; the auditor derives the *same* key from its care-link and
  checks the response touching no data. `gradeAnswers` **loses its ground-truth
  fetch entirely** — a `liar` node that kept its tags but dropped the bytes now
  fails an audit that never fetches, and is slashed via `credit.RecordAudit`.
  The auditor recomputes each full shard's expected block count from the layout
  `ChunkSize` and rejects any prover under-reporting it (soundness against
  partial deletion for every full shard; the single short tail shard is the one
  documented residue for the V3 red-team). The key never crosses the wire and a
  storage node — lacking the layout key — cannot forge. Two hand-rolled codecs
  were extended so the tags don't vanish in the field (a #65-class trap): the
  TCP wire codec (`adapters/tcpnet`) and the on-disk proof store
  (`adapters/diskproofs`, so a restarted host can still prove what it
  re-announces, #69). Repaired/re-seeded shards are re-tagged from the
  caretaker's care-link. Coverage (V5): unit (deterministic key derivation +
  cross-capability agreement, GCM-overhead guard, wire + persistence
  round-trips), sim (liars slashed with **zero** ground-truth fetches during the
  sweep — proven by a per-kind message counter), and the real-daemon TCP + cross
  -NAT (incl. full-swarm restart) harnesses stay green carrying the enlarged
  proofs. Traces to **M0** (presence proven, not asserted), **B8**, and
  **B7/V3**. See `docs/design/gate4-m0-mechanism.md` §3a.
- **Gate 4a (#90): real proof-of-retrieval primitive (`core/por`)** (2026-08-02)
  — the first Gate-4 construction piece. A verifier holding a small secret key
  can now check that a prover still holds a chunk's bytes *without fetching
  them* — the property the toy scheme (`core/node/por.go`, which grades against
  ground truth it fetches itself) deliberately lacked. The construction is the
  private-verification Compact Proof of Retrievability of Shacham & Waters
  (ASIACRYPT 2008) — a homomorphic linear authenticator over the Curve25519
  field prime: per-block tags `σᵢ = f_k(i) + Σⱼ αⱼ·mᵢⱼ`, a seed-expanded
  challenge, and an O(s) aggregated response `(μ, σ)` whose size is independent
  of the chunk. A prover that deleted or altered any sampled block cannot make
  the verification equation hold without the secret αⱼ, which the tags do not
  reveal. The verify key is designed to ride the care-link, so caretakers audit
  over ciphertext while storage-node provers cannot forge. Pure package (bytes
  and keys only); wiring it into the manifest, node audit loop, and credit
  ledger is the next 4a change. Adversarially tested: tampered/deleted-block,
  key-less forgery, wrong-key, and wrong-unit proofs all fail. Traces to **M0**
  (the Sybil corner: presence proven, not asserted), **B8** (adopt the proven
  primitive), and **B7/V3** (a non-holder fails the challenge). See
  `docs/design/gate4-m0-mechanism.md` §3a.

### Fixed
- **Gates 1–3 completeness audit: closed missing regressions in the floors**
  (2026-08-02) — a pre-Gate-4 audit verified the landed floors (Gate 1),
  register-after-distribute (Gate 2, #65), and NAT traversal (Gate 3, #27/#111)
  are whole at all three test tiers, and fixed the coverage gaps it found. The
  register-after-distribute *failure* outcome had no regression: the one sim
  test touching an unplaceable scatter used the old `Add` (publish-up-front)
  path, so it couldn't catch a dangling entry. The gate is now a single tested
  helper, `pipeline.RegisterAfterDistribute` (publish iff the scatter
  confirmed), that both the `swarm add` and daemon-UI publish paths call
  instead of hand-rolling "publish iff `derr == nil`" — covered by a pipeline
  unit test (both branches) and a sim test that drives the real `node.Distribute`
  failure and asserts the registry is left empty (S5). The relay's per-target
  session cap (`PerPeerSessions`, the #65 knob) gained an isolation test proving
  one target's fan-out can't be throttled by — or monopolise beyond its slot —
  another's; previously only the global `MaxSessions` branch was exercised. The
  default `-dns-seed` is documented as a *deliberate* empty (neutral
  infrastructure, community-run seeds — #27 Part A), not an unfinished hole.
- **Transport frame cap was smaller than the minimum production chunk** (2026-08-02)
  — a whole chunk rides in one length-prefixed frame, but the inbound read
  loop's cap was 32 MiB while the *minimum* production chunk is 64 MiB, so every
  production-sized chunk was dropped on receipt; the swarm could only move
  sim-sized (64 KiB) chunks. The cap is now derived from the manifest chunk-size
  ceiling plus envelope overhead (`maxFrame = manifest.MaxChunkSize +
  frameOverhead`), so the wire can always carry a chunk the manifest layer
  accepts and the two limits can't drift. `Send` now also rejects an over-cap
  frame with an explicit error instead of emitting one the peer silently drops
  (S1/S3). Traces to S1/S3 and anti-persona #14. Closes #104.

### Security
- **Gate 1 (A5): panic-recover + fuzz the decode surface** (2026-08-02) — a
  daemon that crashes on a malformed frame can't be field-tested and can't
  carry the "credible from day one" claim, so every untrusted-input decoder is
  now proven not to panic and is caught if it ever does. New Go fuzz targets
  cover the whole decode surface — the manifest CBOR decoder, the chunk-frame
  length header (plus a Split/Join round-trip), `silt:`/`siltcare:` link
  parsing, chain block/blocks decoders, the tcpnet wire envelope, and the relay
  control frame; their seed corpora run as a smoke test on every push/PR and a
  new nightly workflow mutates each for a real time budget (millions of execs,
  zero panics found). Underneath that proof sits a defence-in-depth recovery
  net (`internal/safe`): the tcpnet read loop and the relay client/server frame
  loops drop the *connection* on any panic, and the node's event loop contains
  a panicking task so one bad frame fails the *request*, not the *process* — an
  event-loop panic is logged at error level (a top-severity bug until fixed),
  never silent. Traces to tenets S1/S3 and anti-persona #14. Closes #87.
- **Gate 1 (A6): bound the declared manifest chunk count + size** (2026-08-02) —
  a manifest arrives as reassembled chunk data and *declares* its own chunk
  count and sizes; a declared number is a claim, not a fact (tenet B7), so a
  tiny manifest that declares a huge chunk array was a cheap memory-exhaustion
  vector (anti-persona #14). The manifest CBOR decoder is now bounded
  (`MaxArrayElements = MaxChunks`) so an over-declared array is refused as its
  header is read — *before* the slice is allocated — across both the plain and
  the sealed (layout/secrets) decode paths. `Validate` and `OpenLayout` add
  semantic checks that reject an oversize declared chunk size or count cleanly,
  per request, with the node still up. Bounds are exported and documented
  (`MaxChunks`, `MaxChunkSize`), sized with headroom over the 64 MiB production
  chunk. Traces to tenets B7 and S1/S3. Closes #88.
### Security
- **Gate 1 (I1): lock the local UI / JSON API** (2026-08-02) — the daemon's
  local HTTP API sent CORS `*`, so any web page the operator visited could
  enumerate or drive their node. It is now locked: every request must carry a
  **localhost `Host`** (a DNS-rebinding page arrives as `evil.com` and is
  refused), any **cross-origin request from a non-localhost page** is rejected
  outright (localhost origins are *reflected*, not blanket-allowed, so the
  observatory still aggregates sibling daemons), and every **state-changing
  call requires a per-daemon bearer token** minted on first run
  (`<store>/ui-token`, 0600) and handed to the operator's browser on the UI URL
  (`/?token=…`). Reads keep their no-token localhost ergonomics. CORS `*` is
  gone. Traces to Don't #3 (access-unsurveilled), B4 (privacy by construction),
  and S4 (no seizable single point). Closes #89.
### Security
- **Chain permanence: version the Block schema before any Gate-4 record change**
  (2026-08-02) — `Block` carried no version, so any future change to *what the
  block hash commits to* or to *validation semantics* (real-bond commitments,
  mandatory tokens) would be a hard fork with nothing to gate the eras:
  `Decode`/`DecodeBlocks` would happily decode an old block and mis-validate it
  under new rules. Blocks now carry a `Version` (era) that `Hash` commits to and
  `Decode`/`DecodeBlocks` require — a version mismatch is an explicit
  `ErrBlockVersion`, never silent mis-validation, and because the hash covers it
  the era can't be swapped under a valid signature. Landed while the chain is
  still throwaway, so it costs nothing now and prevents a flag-day later; it is
  the prerequisite for the Gate-4 record-format changes (#90/#91/#92). Entry
  versioning is deliberately deferred: entries are always validated within a
  block whose version gates their rules, and standalone-registry entry
  semantics are what the tokened-publish design turn (#97) will settle. Closes
  #98.
- **Register-after-distribute: a failed scatter no longer leaves a dangling
  registry entry** (2026-08-02, Gate 2, #65) — `pipeline.Add` published the registry entry
  as its final step, *before* the caller distributed the chunks to peers, so a
  loud placement failure left an entry pointing at content that never landed
  (no link reaches the user, but the registry — and network-size estimates —
  count phantom content; tenet S5). Publishing is now split from staging: a new
  `pipeline.Stage` stores the chunks and sealed manifest and returns the entry
  *without* registering it; the networked publish paths (`swarm add`, web-UI
  publish) register **only after** distribution is confirmed. `Add` still
  stages-and-publishes in one shot for callers that don't distribute separately
  (local `add`, genesis, sim). Fetch-side retry and raised relay session limits
  (the rest of #65) already landed. Closes #65.

### Security
- **Unlinkable publish is now the default; the Gated registry is fenced off** (2026-08-02)
  (M0 privacy, #97/#99) — publishing recorded a permanent `Publisher → root`
  link on the append-only chain because the publish clients attached the node's
  durable identity by default. The chain never *required* it; it was being
  written gratuitously and can never be undone. Now: the `swarm add` and web-UI
  publish paths **attach no Publisher by default** (publish is unlinkable —
  carry a blind-signed token, or nothing), and the chain **refuses a
  Publisher-bearing entry** unless the deployment is explicitly trusted
  (`chain.Config.AllowPublisher`, daemon `-allow-publisher`; `swarm add
  -allow-publisher` to opt a single publish back in). Genesis is exempt (it
  seeds via `AppendGenesis` and its proposer is public by design). Tokens stay
  an orthogonal opt-in (`-token-quorum`/`-require-tokens`) for a *paid*
  unlinkable publish, so earned-standing commit without tokens still works. The
  credit-**Gated** registry — which hard-requires a Publisher and has no token
  path — is documented sim/test-only and **fenced off**: an `internal/depcheck`
  architecture test fails the build if any `cmd/` entry point constructs it (it
  is used only by the sim today). Traces to **M0** (privacy corner), **F1 /
  risk #14**, immutable #3 (no permanent linkage). Closes #97 and #99.
- **Hole-punch now actually fires end-to-end: two NATed daemons upgrade the relay
  path to a direct connection** (2026-08-02, Gate 3, #27/#111) — the Phase-3 wiring existed
  but never worked, and CI never caught it because it only ran the standalone
  probe, never the integrated daemons. Two bugs, both found locally via the
  Docker NAT harness (build-immutable V5): (1) the punch was only *requested* on
  a fresh relay **dial**, but a relay conn is reused for every subsequent frame,
  so a steady-state relay path never tried to go direct — now a reused
  relay-backed conn also (cooldown-gated) requests the punch; (2) the punch was
  requested but never **bound** — the relay control conn was dialed without
  `SO_REUSEPORT`, so the punch dial couldn't re-bind that port to reuse the NAT
  mapping the relay observed, so every attempt failed. The reuseport dial hook
  now lives in a shared `internal/reuseport` package used by both the transport
  and the relay client. Proven locally: cone punches (both daemons log a direct
  connection), symmetric correctly stays on the relay. `integration/nat/
  holepunch.sh` (cone + symmetric) is now wired into the `nat-holepunch` CI job
  so this can never silently regress again. Closes #111.

### Docs
- **Build-immutable: a bug fixed once stays fixed, caught locally** (2026-08-02)
  — added tenet **V5** and a new **build-immutable** category to `docs/TENETS.md`.
  Product-immutables define *what silt is*; build-immutables define *how we
  build* and are held at the same amendment bar. V5: every discovered defect
  ships in the same change as a failing-first regression test at its tier(s)
  (unit / integration-sim / e2e), runnable on a contributor's own machine, so a
  re-break surfaces locally in seconds — CI is the backstop, never the first line
  of defense. The three-tier Definition of Done (V1/V2) is elevated alongside it.
  Prompted by catching the integrated hole-punch gap (#27 Phase 3) locally via
  the Docker NAT harness rather than at CI.
- **Intention review actioned: M0 sharpened, S7 added, the V1 gate spine put
  on the board** (2026-08-02) — a docs/canon + tracker pass, no code or
  behavior change, acting on an intent-level fresh-eyes review. **M0** is
  requalified from "*resolve*" the trilemma to "***hold*** it — refuse to
  trade any corner away," and bound to a falsifiable test (held iff an
  *external* red-team suite denies all three failure modes); privacy and
  accountability hold from day one while **Sybil-resistance is the corner that
  bootstraps**. "No center" becomes **"no *permanent* center"** (immutable #3
  and T1), reconciling the invariant with the time-boxed launch-window anchors.
  A new tenet **S7 — "durability must pay for itself"** names the repair-loop
  economics that killed Freenet/GNUnet. **B8 and V3** now require the adversary
  that certifies a novel composition to be *external*, not self-graded. On the
  tracker, the **V1 gate spine is materialized** as GitHub labels + issues
  (gates 0→6, critical path 1→4→6, pinned epic #94): the previously
  prose-only Gate 1 floors (#87/#88/#89) and Gate 4 "the car" (#90–#93, the
  real M0 mechanism) and Gate 5 durability economics (#95) are now filed and
  traced to their tenet. The site's roadmap/changelog generators gain relative-
  link and blockquote rendering so the volatile pages stay generated, never
  hand-edited.
- **Canon reconciled: mission, mechanisms, and a single roadmap spine**
  (2026-08-02) — a docs/canon pass, no code or behavior change.
  `TENETS.md` is restructured into three tiers: a new mission-immutable
  **M0** (silt exists to *hold* the privacy × accountability × Sybil
  trilemma — unlinkable publishing, content-level accountability, and
  Sybil-resistance held together without trading any corner away), six
  mechanism-immutables, and the build tenets,
  which gain **B8** (use best-in-class, proven components; be novel only
  in how they are composed). `ROADMAP.md` is slimmed to a single GitHub
  `V1`-milestone spine: the retired M/Wave/Tier markers are dropped in
  favor of a "learning phase" framing, the 0.1.x/0.2.x line is relabeled
  experimental/learning, and the cadence is stated as 0.9.0 then 1.0.0.
  The issue tracker is reconciled (#78 and #79 closed as shipped, the
  `V1` milestone created), the website roadmap is regenerated from
  source, and a sensitive term was removed from the public docs. The
  math notes on proof-of-retrieval (05) and quorum chains (08) are
  reconciled to match: the current PoR is labeled a challenge-time toy
  with a real published-scheme PoR as the V1 target, and consensus
  standing is described as bond-gated challenged storage on a labeled
  placeholder seal being hardened for V1.

### Security
- **Publisher privacy: quorum-issued blind publish tokens** (2026-08-01) (#14 / F1): the
  chain recorded a Publisher NodeID per root, letting an observer map a durable
  reputation key to every root it published (silt protects who-READS far better
  than who-WRITES). A publish is now authorized by a **publish token** — a
  random serial blind-signed by a QUORUM of distinct validators (a k-of-n
  Chaumian blind multisignature: no single issuer, no trusted-dealer/DKG). The
  publisher pays the fee with its durable identity to acquire the token, but the
  issuers never see the serial, so the committed entry carries the token and
  **NO Publisher identity**, and each serial spends exactly once (chain-wide
  double-spend rejection). Daemon: `-require-tokens N` makes the chain accept
  only token-carrying entries and validators issue; `swarm add -token-quorum N`
  acquires one over the wire. Proven at three tiers: unit (blind sig, quorum
  bundle, chain enforcement), sim (acquire-then-publish through the node loop),
  e2e (three validators, a 2-of-3 token over real TCP). Honest residuals
  (labeled): each signature is unlinkable (Chaum), but a colluding validator set
  narrows the anonymity *set* to same-epoch requesters of the same subset (use a
  canonical validator set); the RSA issuer key is in-RAM (cross-restart
  persistence is a follow-up).
- **Launch-window training wheels** (2026-08-01) (#79, risk 15): a young network is the
  easiest to capture — a Sybil quorum is cheap before the network has
  decentralized. A validator set may now declare **anchors** (`-anchors`,
  `-anchor-quorum`): while the network is immature, a commit ALSO requires
  anchor sign-off, so a Sybil quorum cannot write to a young registry. The
  requirement **sheds mechanically** once `-mature-validators` distinct
  non-anchor validators have attested a committed block — measured
  decentralization, never a flag day. Because attesting requires earned bond
  standing (#78), the maturity metric can't be cheaply inflated by Sybils.
  Anchors are plural (a threshold; no single anchor is load-bearing, cf. R4)
  and their power is transparent, on-chain, and time-limited — they can never
  gate a *mature* network. Off by default (empty anchors). Proven for the
  OUTCOME at unit (`TestTrainingWheelsGateYoungNetworkThenShed`) and sim
  (`TestTrainingWheelsShedThroughTheNodeLoop` — the shed through the real
  propose/attest/commit loop); e2e deliberately skipped and recorded (the shed
  is deterministic chain logic covered at unit+sim, and the `-anchors` wiring
  is confirmed by a daemon smoke check — a bespoke multi-daemon shed e2e is
  high-cost/low-value).
- **Identity costs storage: bond-gated consensus standing** (2026-08-01) (#78): reputation —
  the number the chain gates writes on — is no longer dominated by
  self-reported serving (which two colluding nodes could wash-mint for free,
  threat-catalog D1/D3). Standing now costs **real, challenged, held storage**:
  a validator seals an identity-bound storage bond (`core/bond`, `-bond`), and
  validators challenge each other's bonds over the wire (`MsgBondChallenge`/
  `MsgBondReply`), verifying against only the committed Merkle root — no
  ground-truth fetch. Standing must be *sustained* (it decays if a bond stops
  being re-proven), so N Sybil identities cost N distinct bonds on N disks.
  Proven for the OUTCOME at three tiers: unit (`core/bond`), sim
  (`TestBondAuditEarnsStandingOverTheNetwork` — a no-bond node is refused,
  decay retires unsustained standing), and e2e
  (`TestBondEarnedStandingCommitsOverTCP` — two bonded validators earn standing
  over real TCP and commit on `-min-rep 100`). Honest limit: the bond is held
  in RAM and the seal is not yet memory-hard (proof-of-*space*-lite, labeled);
  disk-persistence + a memory-hard seal are tracked follow-ups. Design:
  `docs/design/bond-audit.md`.
- **Safe consensus defaults** (2026-08-01) (#79): `silt daemon -validator` now defaults to
  `-quorum 3 -min-rep 100` (was `-quorum 1 -min-rep 0`), so a lone or fresh
  node can no longer rubber-stamp the registry — writing requires earned
  standing and a real quorum. A trusted one-box swarm opts into self-commit
  explicitly (`-quorum 0 -min-rep 0`), which now prints a loud
  trusted-deployment warning rather than being the silent default. Outcome
  proven end-to-end: e2e `TestDefaultsRefuseRubberStampCommit` asserts the
  default refuses a lone commit, with `TestPublishCommitFetchOverTCP` (explicit
  `-quorum 0`) as the positive control.

### Added
- **Deterministic NAT/relay/hole-punch in the sim** (2026-08-01) (#27): the in-process
  network (`simnet`) now models a home router — a NATed node dials out freely
  (each outbound opening the conntrack reverse mapping so replies get back in)
  but is un-dialable cold from off its LAN. Two NATed nodes on different LANs
  therefore meet through a designated relay (counted in `Stats.Relayed`), or
  `HolePunch` opens a direct path for cone NATs and correctly falls back to the
  relay for symmetric ones. A relayed delivery pointedly does *not* open a
  direct mapping, so a later direct dial still needs a punch. This is the
  tier-1, seed-reproducible mirror of the `integration/nat` Docker harness; it
  is zero-overhead and byte-identical for every existing scenario (no NAT
  configured → the fast path short-circuits and draws no extra randomness).
- **Hole-punching: relay paths upgrade to direct connections** (2026-08-01) (#27): when two
  NATed daemons talk through a relay, the relay now *coordinates* a
  hole-punch — it tells each the other's observed endpoint, and both dial it
  from their relay-registration port at once (`SO_REUSEPORT`, TCP
  simultaneous-open). Through a cone NAT the crossing SYNs establish a direct
  link, which the transport adopts so the bulk traffic leaves the relay; on
  symmetric NAT it simply fails and the relay path stays. The relay forwards no
  bytes for the direct path — it only swaps addresses. The punch **primitive is
  proven end-to-end against real kernel NAT** by the `integration/nat` harness,
  CI-gated (cone → direct connection, symmetric → relay); the relay
  coordination is unit-tested. This demotes the relay from every-byte carrier
  to rendezvous, the big cost win for cheap public infrastructure (S6). (The
  live two-daemon upgrade has a harness scenario in progress — the caretaker
  traffic-trigger needs the minimal-network provider resolution sorted.)
- **NATed nodes learn their public endpoint, STUN-style** (2026-08-01) (#27, the groundwork
  for hole-punching): when a node registers with a relay, the relay reports the
  `host:port` it observed the registration coming from — the node's NAT mapping.
  A node behind NAT cannot otherwise know its own public address, and
  hole-punching needs it (it's the endpoint a peer aims a simultaneous-open at).
  Surfaced as `relay.Client.Observed()` / `node.ObservedAddr()` and logged by
  the daemon. This is phase 1 of #27; the relay-coordinated punch, port-reuse
  dial, and relay→direct upgrade follow. The `integration/nat` harness asserts
  a NATed node learns its *mapped* public IP (the gateway's), not its LAN
  address.
- **Automated cross-NAT integration harness** (2026-08-01) (`integration/nat/`, and a
  `nat-integration` CI job): stands up two genuinely-NATed daemons plus a
  public relay in real container networks (real kernel NAT via iptables
  MASQUERADE, real TLS over real sockets), publishes from behind one NAT and
  fetches from behind another, and asserts the bytes come back bit-perfect
  having crossed the relay (verified by counting relay splices). This is the
  automatable replacement for the manual two-machine (Mac A ↔ Mac B) rig — the
  NAT/relay path that the in-process sim and flat-localhost e2e can't reach —
  and the seed harness for hole-punching (#27) and restart/re-provide (#69)
  scenarios. Runs on one host (CI, a dev box, or Docker Desktop); no second
  machine.

### Fixed
- **The daemon no longer silently drops config fields** (2026-08-01) (#71): `cmd/silt` built
  `node.Config` field-by-field, so any field added to `DefaultConfig` defaulted
  to its zero value in the real binary — how the #65 fetch-retry shipped inert
  and demand-responsive dispersion was off in the daemon while the roadmap
  listed it as done. The daemon and the ephemeral swarm add/get client now
  start from `node.DefaultConfig()` and override only what genuinely differs
  (the daemon's 2s `RequestTimeout`), so new fields are inherited by default.
- **A restarted daemon's content stays discoverable** (2026-08-01) (#69, found in the #65
  field test): provider records live only in peers' memory and die with the
  process, so a daemon re-announces everything on its disk at startup
  (`AnnounceHeld`) — but a coded shard must be announced under its *column
  key* `hash(root‖column)`, where readers look, and that key is derived from
  the shard's storage proof. Proofs were kept only in memory, so after a
  restart the re-announce fell back to the bare chunk id and a disk full of
  intact content was invisible until it happened to be re-hosted. Storage
  proofs are now **persisted alongside the chunks** (`adapters/diskproofs`) and
  reloaded on startup, so the re-announce lands on the right key again — and
  the node can still answer storage-audit challenges after a restart. The
  `integration/nat` harness gained a `RESTART=1` scenario that restarts the
  whole swarm and re-fetches to prove it.
- **Fetches survive a saturated relay** (2026-08-01) (#65): once the public rendezvous
  node hits its capacity cap, every byte to a NATed provider funnels through
  the relay, whose per-peer splice slots saturate under concurrent fan-out
  and return "relay at capacity" — and the fetch path had **no retry**, so a
  transiently-refused chunk was reported unreachable (the tail-of-sweep
  fetch failures seen from a second network). A chunk fetch now **re-sweeps
  its providers with a backoff** when every provider failed *transiently* (a
  timeout or relay refusal, not a clean "don't have it") — the freed slots
  make the retry succeed — the fetch-side analogue of the #63 placement
  retry (`FetchAttempts`/`FetchBackoff`, default 3× / 200 ms). A clean miss
  (nobody has the chunk) still returns after a single pass. The relay's
  concurrency defaults are also raised from **64/8 to 128/16**
  (global/per-peer): splices are short-lived, so this is realistic headroom
  for a rendezvous node while staying a bounded, operator-tunable cost (each
  splice is still byte-capped). Remaining, tracked in #65: register-after-
  distribute (a loud placement failure still leaves a dangling registry
  entry), and hole-punching (the structural fix that moves bulk bytes off
  the relay entirely).
- **Publish no longer returns a link for a file the swarm can't rebuild** (2026-08-01)
  (#64, the data-shard twin of #60): placement verified that *manifest*
  chunks landed durably, but **data and parity shards were placed
  optimistically** — a column that no node accepted was ignored, so under
  load a stripe could silently erode below its erasure threshold `k` and the
  publish still returned a valid-looking link (in the field, f123 came back
  `stripe 0: only 9 of 16 shards, need k=10, unrecoverable`). Distribute now
  tracks per-shard placement and, before returning a link, **verifies every
  stripe kept enough placed shards to reconstruct** (accounting for the
  known-zero padding of a short final stripe); a column that lands nowhere is
  **retried with a fresh lookup** (as manifest chunks already were), and if a
  stripe still can't be made recoverable the publisher **fails loudly**
  instead of handing back an unrebuildable link. The same check closes the
  identical silent-loss on **uncoded files** (which carry no parity, so every
  chunk is required). Extends tenet **B7 — trust but verify; no optimistic
  operations** from the manifest path to all of publish.
- **Publish no longer returns a link for content it never stored** (2026-07-30) (#60,
  found in the 300-file scaling re-test): under load, once the network
  passed its capacity cap, a manifest chunk could be placed on *no* node
  (all candidates full or unreachable) yet publish still registered the
  root and returned a valid-looking link — ~14% of files were stranded
  behind dangling links (fetch failed with "manifest chunks unreachable").
  A manifest chunk that lands nowhere is now **retried with a fresh lookup**
  (these misses are usually transient — a relay hiccup once the nearest
  nodes cap out and load shifts onto NATed hosts), so publishes that used to
  strand now succeed; if it still can't be placed after several tries the
  publisher **fails loudly** instead of handing back an unretrievable link.
  This makes publish honor the new tenet **B7 — trust but verify; no
  optimistic operations.**
- **Ghost routing entries no longer break discovery at scale** (2026-07-30) (found in
  the 300-file scaling test, #43): every `swarm add`/`swarm get` ran as a
  short-lived client with a fresh identity, and nodes both routed to those
  clients and persisted them to `peers.json` — so a busy node's routing
  table filled with dead entries (in the test: 327 entries, 2 live, ~75%
  query timeouts), which broke provider discovery and made most fetches
  fail. Fixed at both ends: nodes persist only peers they have actually
  reached, and a short-lived client stamps its messages so peers never
  route to it.
- **Re-publishing identical content is idempotent** (2026-07-30) (#46): a failed
  publish could leave a root registered but return no link, and a retry
  then hit "root already published with different entry" — because
  idempotency compared the whole entry, including the per-invocation
  publisher identity. It now dedups on content, so a retry (or a second
  person adding the same file) succeeds instead of colliding.
- **NATed peers can actually converse** (2026-07-26) (found in the first real
  cross-network test, #27): the transport dialed a fresh connection per
  message, so a reply required dialing *into* the requester — impossible
  behind NAT, and bootstrap came back with zero table entries. Replies
  (and all traffic) now ride the live connection the peer opened, and
  dialed connections are kept and reused. Two corollaries: a wildcard
  bind (`0.0.0.0`/`[::]`) is never stamped on outgoing messages (it used
  to poison peers' address books with an undialable address — a new
  `-advertise HOST:PORT` flag lets a public box say what to gossip), and
  a daemon that registers with a relay now **re-bootstraps** through it,
  since its first join attempt may have been unanswerable. The
  reachability dial-back deliberately never reuses a connection — its
  meaning is "a fresh inbound dial landed" — so AutoNAT stays honest.
- **Relay-form addresses survive `-bootstrap`, DNS seeds, and
  peers.json** (2026-07-26) — peer strings split on the first `@`, not the last, so
  `ID@relay:RID@host:port` parses instead of being silently dropped.

### Added
- **Opt-in in-RAM read cache for hot chunks** (2026-07-30) (`-cache SIZE`, default off;
  #42): a cache hit serves trusted bytes from memory, skipping both the
  disk read and the per-read hash re-verification. Read-through LRU,
  cache-on-read only, and Delete evicts so purged content is never served.
- **The daemon caretakes content published through its own UI** (2026-07-30) by default
  (`-care-published`, #44): without a caretaker a published file's
  redundancy only decays as nodes churn — now the publishing daemon
  repairs its own roots, and both the UI and CLI say whether a caretaker
  is running.
- **Paginated, shard-sorted roots list in the daemon UI** (2026-07-30) (#45): the
  "identifiers this daemon holds shards of" table now paginates and sorts
  by shards held, instead of rendering every row (unusable at hundreds).
- **A public build log** (2026-07-27) — a chronological "how it was built and why"
  narrative under `docs/buildlog/` (dated Markdown entries), rendered to
  `website/buildlog.html` by `scripts/gen_buildlog.py` on the same
  source-of-truth pipeline as the changelog and roadmap (CI fails if the
  page drifts). It's the *reasoning* behind the design — the forks, the
  dead ends, the decisions — distinct from the changelog (what shipped)
  and the roadmap (what's next), and strictly about building the
  infrastructure. Seeded with three entries: the one-process/ports-and-
  adapters prime directive, the placement spectrum, and cross-network
  reachability. Linked from the site's docs and footer.
- **`-log LEVEL` — narrate the normal path, not just failures** (2026-07-27) — both
  `silt daemon` and `silt client` take `-log error|warn|info|debug`,
  opening the `debug.log` sink at that threshold; `-debug` is now
  shorthand for `-log debug`. At `info` the happy path narrates —
  `file distributed` (chunks placed), `block committed` (quorum reached,
  by proposal or broadcast), `file retrieved`, alongside the existing
  `stripe repaired`, `dispersion re-spread`, and `reachability verdict`
  — so a real-world run can be checked against how the system is
  *supposed* to behave, not only when something breaks, and without the
  debug firehose. Free when off and off the hot path (per-chunk store
  events stay at debug); core still logs through the `ports.Logger` port
  and imports nothing new.
- **Multi-process end-to-end tests over real TCP** (2026-07-27) (CI hardening,
  BACKLOG Phase 2) — a new `e2e/` suite builds the `silt` binary and
  runs three daemons as separate OS processes, publishes a 1 MiB file
  through the chain-backed registry over pinned HTTPS (driving a real
  consensus round to a committed block), then fetches it back across the
  swarm and asserts it returns bit-perfect. This exercises the whole
  wire path the in-process sim deliberately bypasses — exactly where
  #36's "a reply can never reach a NATed peer" bug hid until real
  sockets carried it. It runs as its own CI job; the unit and race jobs
  pass `-short` to skip the process spawning.
- **Relay discovery by gossip** (2026-07-27) (#27 polish) — a daemon offering `-relay`
  now stamps the service's dialable `host:port` on every outgoing
  envelope (borrowing the `-advertise` host when the relay listener is
  bound to a wildcard). Peers record these first-hand — a node only ever
  announces its *own* relay, and dialing pins the relay's identity, so
  gossip can direct but never impersonate. A daemon whose reachability
  verdict is NATed and that has no `-relay-via` adopts the first
  discovered relay automatically (and keeps watching until one appears):
  the two-Macs runbook now works with nothing but `-bootstrap`.
- **Two-slot address book: direct preferred, relay fallback** (2026-07-27) (#27
  polish) — the transport now remembers up to two addresses per peer,
  one direct `host:port` and one `relay:R@host:port`, instead of one
  slot the two forms fought over (an mDNS-learned LAN address used to be
  clobbered by the peer's relay stamp, sending house-mates through a
  relay on another continent). Dials try direct first — no third hop —
  and fall back to the relay within the same delivery; a direct address
  is dropped only when the relay fallback *reaches* the peer, which
  proves the address stale rather than the peer down. Contact gossip
  passes on the relay form when one is known (a relay-advertising peer
  is NATed, so its direct address is LAN-scoped hearsay); `peers.json`
  persists both slots. The reachability dial-back ignores relay
  addresses outright: reachable-through-a-relay is exactly what "public"
  must not mean.

- **Relay** (2026-07-26) (#27, step 3 — the universal NAT fallback) — a NATed daemon can
  now be reached across networks through any reachable node running
  `-relay ADDR`. The shape is libp2p Circuit-Relay-v2's, without the
  dependency: the NATed node keeps one registered outbound connection to
  the relay (`-relay-via RELAYID@HOST:PORT`, taken up automatically when
  the reachability verdict says NATed) and advertises `relay:R@host:port`
  as its address; a sender dials the relay, the target dials back, and the
  relay splices the two streams. Crucially, the sender then runs its
  normal pinned **end-to-end TLS handshake with the target through the
  splice** — the relay moves opaque bytes it cannot read, alter, or forge,
  so "a frame's sender is whoever the handshake authenticated" holds
  unchanged across a relay. Relaying is a capability, not infrastructure:
  opt-in, capped (concurrent sessions, per-peer sessions, per-session
  bytes), no relay baked into the binary, and the relay-operator metadata
  exposure is documented in the threat model. CI proves the full path on
  localhost — including both-peers-NATed, every byte relayed — because
  "NATed" is modeled honestly as "accepts no inbound connections".
- **`-debug` flag → `debug.log`** (2026-07-26) on both `silt daemon` and `silt client` —
  a leveled logger behind a new `ports.Logger` interface (core stays pure;
  the file sink is `adapters/logfile`). One grep-able line per event:
  transport failures (dials, handshakes, forged frames), node events
  (request timeouts, repairs, dispersion re-spreads, the reachability
  verdict), and daemon milestones (discovery, bootstrap). Quiet by default
  and free when disabled; with `-debug`, a failure in the field leaves an
  artifact that can be attached to a bug report. Groundwork for testing
  cross-network reachability (#27) on real networks, where failures are
  one-shot and remote instead of deterministic and replayable.
- **Zero-config LAN discovery** (2026-07-26) (#27, first rung of cross-network
  reachability) — `silt daemon` now announces itself on the local network
  and folds any peer it hears into the routing table, so two nodes in the
  same house find each other with no `-bootstrap`, no DNS seed, and no
  infrastructure. It's link-local multicast (the same idea as mDNS, scoped
  to the LAN by TTL), and self-authenticating: an announcement carries a
  peer's `ID@host:port`, and the TLS handshake still must present a key
  hashing to that ID, so a rogue beacon can misdirect a dial but never
  impersonate a node. On by default; `-mdns=false` opts out, and a
  loopback-only `-listen` disables it with a note (nothing on the LAN could
  reach a loopback address anyway). See
  [docs/design/cross-network.md](docs/design/cross-network.md).
- **Reachability check** (2026-07-26) (#27, our AutoNAT) — after bootstrap, a daemon asks
  a couple of known peers to dial it back at its advertised address. A
  landed dial-back both proves and delivers the verdict "public"; silence
  within a timeout is read, conservatively, as "behind NAT" (which only ever
  costs a relay we might not have needed, never a false claim of being
  reachable). The daemon logs the result and the dashboard shows it; the
  relay step will key its advertise-direct-vs-via-relay decision off it. No
  new message plumbing beyond two wire kinds; the pure core stays
  NodeID-only — reachability is simply whether the transport can deliver.

### Changed
- **The field-test publish bound re-derived downward, 360 → 300 s** (2026-08-27,
  owed Phase-3 gate clause). The `12-deep-heights` deep drive measured ~48 s/height
  steady cadence (`integration/cloudtest/results-fe2376a-deep.jsonl:29`, was ~390 s
  at the depth-war start), so the publish retry budget in
  `integration/cloudtest/scenarios.sh` (`PUBLISH_RETRY_S`, and its sibling
  `ECONOMY_PUBLISH_RETRY_S`) re-derives from the measured number, not a guess
  (#549-Q3 discipline). The load-bearing finding: only the gather-leg term is
  cadence-free request-timeout arithmetic; the commit-wait leg is the #451
  synchronizer 2-round escape FLOOR (`dur(0)+dur(1)` = 150 s, counted in fixed 30 s
  sweeps — a consensus-liveness parameter, **left untouched**). The 60 s shed is the
  historical escape-rounding cushion (220 → 184) plus stale slow-height straddle
  padding the cheap cadence retires. 300 s keeps the full 150 s escape window inside
  the bound (6.25× the measured cadence, 1.76× the 170 s per-height worst case at
  e2fab4b), above the too-tight 240 s scar. Config + docs only; no billable run.
  Derivation: `docs/thinking/2026-08-27-publish-bound-rederivation.md`.

## [0.1.1] — 2026-07-26

Still early, experimental, and unaudited (see the
[threat model](https://github.com/nerolabs/silt/blob/main/docs/threat-model.md)).
This release is the first round of first-production-user feedback from 0.1.0,
fixed:

### Changed
- **Swarm registry docs & error messages** (#17) — the registry is
  *key-pinned HTTPS*, and now everything says so. The README swarm recipe
  and `silt daemon -registry` help use the `<ID>@https://host:port` form the
  daemon prints; passing a bare `https://` or an `http://` URL to a pinned
  registry returns a message that names the fix instead of a raw TLS error.
- **`silt info` summarizes by default** (#18) — root, mode, size, chunk and
  stripe counts, erasure params; the full per-shard dump moved behind
  `-shards`. It was a wall of hashes on any real-sized file.
- **`silt add` leads with the share link** (#19), labelled, and prints the
  care link after with a "repair only, cannot decrypt" caveat. The bare
  link stays on stdout so `silt add file` remains pipeable.
- **`silt daemon` pledges 5G by default** (#21), matching `silt client`, so a
  fresh daemon contributes measurable, countable storage instead of an
  unlimited pledge that read as 0 B of network storage. `-capacity ""` still
  means unlimited.
- **Shorter, easier-to-copy links** (#20) — a link now encodes its two
  32-byte values in compact base64url (43 chars each) instead of 64-char
  hex, so a share link is ~30% shorter (137 → 95 chars). Old hex links still
  parse.
- **Observatory** (#22) explains it shows only the daemons you list that run
  `-ui` (no swarm auto-discovery), that "daemons observed" is not the peer
  count, and now displays the swarm's self-estimate ("~N peers") right beside
  it so the two numbers reconcile.

### Added
- [**Build your own Silt test network**](https://github.com/nerolabs/silt/blob/main/docs/local-test-network.md) —
  a public, end-to-end local walkthrough (sims → a real multi-node swarm that
  survives a node death), with all of the above fixes baked in.

## [0.1.0] — 2026-07-25

**The first release — early, experimental, and unaudited.** Silt 0.1.0 is
published to get technical feedback, not to be trusted with data you can't
afford to lose. Please read the
**[threat model](https://github.com/nerolabs/silt/blob/main/docs/threat-model.md)** —
it names the weak parts on purpose (a toy proof-of-retrieval, unhardened
Sybil/eclipse, a quorum-not-BFT chain, and more) — and help us break it.
Binaries are **not** code-signed; verify them against the attached
`SHA256SUMS`.

### Added
- **Content-addressed storage** — every fragment is named by the SHA-256
  of its bytes; verification is intrinsic, so hosts are never trusted.
- **Erasure coding** — Reed-Solomon stripes (default any 10 of 16 rebuild
  the file); a repair loop restores redundancy as machines fail, and — like
  the initial placement — keeps each stripe's shards spread across distinct
  hosts as it rebuilds, so one machine's death never costs a stripe more
  than a single shard.
- **Encryption at every level** — chunks and manifests are both
  ciphertext; a file's share handle is a *link* (`silt:v1:root:key`)
  whose one-way key hierarchy also yields *care links* that grant repair
  and audit without the ability to decrypt.
- **The swarm** — Kademlia routing, provider records, and multi-node
  fetch over a deterministic simulator or real mutual-TLS sockets;
  identity is a keypair and a node's ID is the hash of its public key.
- **Column placement** — an erasure-coded file is placed by *column* (one
  shard position across every stripe), keyed by `hash(root‖col)`, so a
  whole column lands together: one host holds one shard of each stripe,
  a reader finds a column in a single lookup, and losing a host costs a
  stripe exactly one shard (up to n−k columns can go and the file still
  rebuilds). Placement, retrieval, repair, and audits all speak columns.
- **Failure-domain-aware placement** — a node can declare a failure-domain
  label (AS / rack / geo / operator) and gossips it; placement and repair
  spread a file's columns across distinct domains, so an entire domain
  going dark costs a stripe as little as possible — not just distinct node
  IDs, but distinct *domains*.
- **Dispersion audit** — a caretaker doesn't just keep a stripe *alive*, it
  keeps it *spread*: each sweep it confirms which domains actually hold each
  column, and if any one domain holds enough of a stripe that losing it
  would drop below the recovery threshold, it seeds extra copies into other
  domains until no single domain failure could break the file.
- **Demand-responsive dispersion** — storage flexes with popularity. A node
  that finds itself serving a chunk hard pushes leased cache copies to more
  hosts (spread across domains) so readers divide across more sources; when
  the reads cool off, the copies expire and the file contracts back to its
  baseline. A flash-popular file fans out without permanently hoarding
  capacity.
- **Capacity** — nodes pledge a fixed budget (`-capacity 2G`); placement
  spills over as nodes fill, and every node estimates the whole network's
  size from local gossip alone.
- **Proof-of-retrieval audits** — hosts are challenged to prove
  possession with a fresh nonce; those that keep the proof but drop the
  data are slashed.
- **The registry chain** — an append-only chain kept by the operators;
  blocks commit only with a quorum of attestations from validators whose
  reputation (audits + serving) is earned, not bought.
- **Genesis** — every fresh network is born carrying a founding manifesto
  in block 0, declared identically on every node.
- **Takedown by revocation** — illegal or unwanted content is removed at
  the availability layer, not the ledger: an append-only revocation
  record, committed by the same reputation quorum, makes compliant nodes
  no-op on a denied opaque root (refusing to store, serve, prove,
  announce, or repair it) and purge what they hold — never decrypting
  anything. Operators may also load a local denylist they choose to honor
  (`silt daemon -denylist`). The project ships the mechanism and no list;
  it operates neither the network nor the policy.
- **Web UI** — an embedded dashboard, publish/fetch pages, and a network
  observatory, served by the daemon.
- **Desktop client** — one binary that consumes and serves at once, keeps
  a link-book library, and runs on macOS, Windows, and Linux.
- **Public website** (silthq.com) with brand, docs, operator guide, and
  build-from-source instructions.
- **Continuous delivery** — PR previews, a `staging` environment, and
  production deploys from `main`; a public changelog rendered from this
  file.
- **Governance & strategy docs** — the fresh-eyes council, risk register,
  launch plan, safety/takedown model, and `GOVERNANCE.md`.

[0.1.1]: https://github.com/nerolabs/silt/releases/tag/v0.1.1
[0.1.0]: https://github.com/nerolabs/silt/releases/tag/v0.1.0
