# era-3 step 2b — the v4 validity predicate (PACE deliberation)

**Date:** 2026-08-29
**Seat:** Builder
**Step:** 2b of the ratified certified era-3 sequence. 2a (schema + Hash + `versionSupported<=4`)
is on main (`84dcee0`). This step adds the validity predicate that reads the roots.
**STOP boundary held:** no mint-version flip, no activation height (those are 2c). No existing
consensus rule is altered — this step ADDS a v4-gated rejection only.

**Certs / rulings this step discharges:**
- Format cert: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era3-committed-state-root-format-RESEARCH-CERTIFICATION-2026-08-28.md`
  (Q5: era-3 is a HARD fork; the predicate is `StateRoot == recompute` AND `LogRoot == RevocationLogRoot()`).
- 2a ruling (the binding carry-forward): `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era3-step2a-commit-roots-schema-2026-08-29.md`
  — §"What 2b MUST carry": (1) reject nil-root v4 explicitly, (2) reject root ≠ recompute,
  (3) value-encoding is load-bearing (rely on the step-1 determinism oracle for byte-identity),
  (4) do NOT route the omitempty amendment myself (carried into the composed re-cert before freeze).

---

## The problem, stated as a mechanism

The 2a→2b window (2a ruling, "highest-severity residual"): a v4 block currently decode-accepts
and validates under the era-2 rounds path (`ValidateProposal` chain.go:2208, `ValidateCommit`
chain.go:2366). **Neither path reads `StateRoot` or `LogRoot`.** So today a v4 block with a wrong
or nil root passes validation on its era-2 merits alone. That is benign only while nothing mints
v4. 2b closes the window by making the roots a validity condition, so 2c can safely mint v4.

The failure this predicate prevents (once v4 is minted, 2c): a Byzantine proposer commits a v4
block whose `StateRoot` leaf encodes a WRONG `bonded`/`epochSet` weight. Those weights are summed
in three super-quorum predicates (chain.go weight-quorum sites named in the 2a ruling §3). A
wrong-value leaf is therefore a consensus SAFETY attack on the weight sums, not a read bug. The
predicate makes any such block invalid: an honest attester refuses to sign it, and a commit
carrying it is rejected.

---

## Decision 1 — WHERE the predicate hooks

**Decision: hook in BOTH `ValidateProposal` and `ValidateCommit`, via one shared helper
`validateEra3Roots(b)`, gated on `b.Version >= BlockVersionStateRoot`.**

- `ValidateProposal` (chain.go:2208) is what an honest attester runs before signing
  (chainrole.go:262). If the predicate is only in `ValidateCommit`, an honest attester would SIGN
  a v4 block with a forged root, and only the commit path would reject it — too late, the
  signature is already on the wire. The roots must be checked wherever a block's validity is first
  decided, which is the proposal path.
- `ValidateCommit` (chain.go:2366) calls `ValidateProposal` first (chain.go:2363), so the check
  fires there transitively. I do NOT add a second, independent call in `ValidateCommit` — that
  would run the dry-run recompute twice per commit. The single call in `ValidateProposal` covers
  both entry points, and I add a comment at the `ValidateCommit` site recording that the root
  check rides in via `ValidateProposal`.
- `validateStructural` (chain.go:2726, the Reload/own-disk replay path) does NOT call
  `ValidateProposal`.

  **~~2b claim (REFUTED 2026-08-29, corrected in the reload-root-check follow-on):~~** *2b
  argued the reload path needed no root check because "the proposer/attester signatures cover
  the whole block hash, which now includes the roots, so `validateStructural`'s signature verify
  rejects a bit-flipped root at load." That argument is WRONG.* A signature over a root proves
  only that the signer committed to THAT byte string — it does NOT prove the root equals the
  post-apply state. **Integrity ≠ root-correctness.** A v4 block whose `StateRoot` is a wrong
  value but is **re-signed with the proposer key** (a corrupt/tampered own-disk block, or a
  future fast-sync/import path feeding attacker-supplied bytes) passes every signature check in
  `validateStructural`. The commit path recomputes and rejects such a block; the reload path
  accepted it — an unenforced root on the own-disk path. This was proven with a failing-first
  regression: `TestReloadRejectsResignedWrongStateRootV4` (RED on `a0f8839`, GREEN after the
  fix), with an ablation subtest showing `validateStructural` ACCEPTS the re-signed block so the
  reject is caused by the root check, not the signature.

  **Corrected decision (A-bare, blind-PE ruled — `RULING-era3-reload-root-check-2026-08-29.md`,
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era3-reload-root-check-2026-08-29.md`):**
  `appendStructural` now calls `validateEra3Roots(&b)` directly — the SAME clone-recompute
  predicate the commit path uses — **BEFORE `c.apply(b)`**. Before-apply, not after: a
  post-apply-then-reject would leave the bad block applied to the live chain (head advanced,
  `byRoot` mutated), which breaks Reload's load-bearing "keep the longest VALID prefix" contract
  that the daemon relies on (`cmd/silt/daemon.go`: a replay failure continues with the prefix
  chain, so a partially-applied bad block would poison a running node). *(The PE ruling's
  "no clone needed, compare against live post-apply `c.StateRoot()`" is an efficiency note that
  assumed apply-then-read is safe; validating on the clone before apply is the exact same verdict
  at O(state)/block and preserves the prefix contract — recorded here as the one refinement over
  the ruling's suggested encoding, surfaced to the PE/Tester in review.)* The named errors are
  shared with the commit path, so proposer, validator, and reload agree or all fail. A v2/v3
  block skips it (era-gated, Decision 4), UNCHANGED. A **structural write-set guard**
  (`TestEveryDiskWritePathRunsTheEra3RootCheck`) now enumerates every `c.apply(b)` caller and
  fails if any (except the genesis allowlist) skips the root check — so a FUTURE fast-sync/import
  path cannot silently re-open this hole.

  **Residual (the hold-the-tree bridge, named as a reopening item — see the "Reopening /
  optimization" section below):** A-bare re-validates each v4 block's root on boot at
  O(state)/block ⇒ O(depth²) over a full Reload. This is the correct BRIDGE until the
  incremental-SMT / `ports.NodeStore` keystone-store workstream (#600) makes `StateRoot()`
  O(depth) on every path, at which point the SAME `validateEra3Roots` call site becomes linear
  with zero change. A-bare is the strictly-dominant bridge to incremental (PE ruling Q1).

Placement inside `ValidateProposal`: AFTER the existing structural/quorum/entry checks return
clean, at the end (a v4 block must first BE a valid era-2 block, then additionally satisfy the
roots). This keeps era-2 rules untouched and adds the v4 gate as a strict additional condition.

## Decision 2 — the POST-APPLY timing, and how to recompute without mutating live state

The committed `StateRoot` is the SMT over the committedSet state AFTER this block's effects are
applied (an entry it publishes is in `byRoot`, a bond it registers is in `bonded`, etc.).
`ValidateProposal`/`ValidateCommit` run BEFORE `apply(b)` (Append: validate then apply,
chain.go:2665-2669). So the predicate must compute the post-apply state itself.

**Decision: clone the accumulating state into a scratch `*Chain`, `apply(b)` on the scratch, then
read `scratch.StateRoot()` / `scratch.LogRoot()`.** Compare to the block's committed roots.

- `apply()` mutates in place (maps, `revLog.Append`, and via `rotateEpoch`: `epochSet`,
  `epochStart`, `matureEpoch`, gate fields). It must NOT run on the live chain during validation.
- The clone copies EXACTLY the fields `apply()` (and its callees `rotateEpoch`, `liveQualifiedSet`,
  `Mature`) read or write. This is the same "replay into a fresh replica" shape `Reconcile` already
  uses (chain.go:3305, `tmp := New(...)` then `Append` each fork block) — reused for one block,
  cloning the current committed state instead of replaying from genesis (O(state), not O(history)).
- **Drift protection (the #558 class the completeness guard exists to catch).** A new committed
  field that the clone forgets would make the dry-run apply diverge and the recompute wrong — a
  silent consensus bug. So the clone is guarded by `TestDryRunCloneCopiesEveryAppliedField`, which
  populates a chain, clones, and asserts (by the same reflection the completeness guard uses) that
  every `committedSet`/`committedLog`/`observable` field is copied non-zero. A new field on `Chain`
  fails this test until the clone copies it, exactly as it fails `TestStateFieldsAreClassified`
  until classified. The clone is a fourth enumeration; the guard is what keeps it honest.

Why clone-and-apply, not "apply then undo" or "diff the block": undo is the fragile per-record
reversal the `adopt` doc explicitly rejects; diffing the block re-derives apply()'s canonicalize/
TTL/rotate logic and would drift from it. Cloning and calling the REAL `apply()` means the
recompute uses the one authoritative state-transition function — a value-encoding or apply bug
surfaces identically on both the proposer's and the validator's side, so they agree or both fail.

`LogRoot()` after the dry-run apply: `apply()` appends this block's revocations/unrevocations to
the scratch `revLog`, so `scratch.LogRoot()` is the post-apply RFC-6962 MTH. That is what the
committed `LogRoot` must equal (2a ruling requirement 2: `committed LogRoot == RevocationLogRoot()`
at the post-apply state).

## Decision 3 — the three rejections (2a ruling's four MUSTs)

The helper enforces, for a `b.Version >= BlockVersionStateRoot` block only:

1. **nil-root reject (MUST 1).** `b.StateRoot == nil || b.LogRoot == nil ⟹ ErrEra3RootMissing`.
   This is the check the cert wanted the SCHEMA to enforce; 2a's omitempty placement moved it here
   (2a ruling Q3). It is EXPLICIT, not implicit in the equality test — a nil pointer is not "a
   wrong value", it is "no root", and it must be named as such.
2. **StateRoot mismatch reject (MUST 2).** `*b.StateRoot != recompute(post-apply) ⟹
   ErrEra3StateRootMismatch`.
3. **LogRoot mismatch reject (MUST 2).** `*b.LogRoot != scratch.LogRoot() ⟹ ErrEra3LogRootMismatch`.
4. **Value-encoding (MUST 3).** The recompute calls `scratch.StateRoot()`, which is the step-1
   canonical encoding (`core/chain/statehash.go`, `core/statehash`). Cross-node byte-identity is
   the property the step-1 determinism oracle proves
   (`modelcheck_stateroot_determinism_test.go`, cert residual R2) — I rely on it, I do not
   re-prove it here. A wrong-value leaf therefore produces a different recomputed root and is
   caught by MUST 2. This is why the value encoding being load-bearing is discharged by this
   predicate: the equality check IS the enforcement of the encoding.
5. **omitempty amendment (MUST 4).** Not routed by me. Noted: the 2a ruling defers the
   "omitempty satisfies required" soundness call to the composed re-cert before freeze. 2b does not
   change the schema; it enforces the nil-reject the placement obliges. No new research gate opened
   by this step.

## Decision 4 — ERA-GATING

The predicate fires ONLY for `b.Version >= BlockVersionStateRoot` (== 4). A v2/v3 block skips
`validateEra3Roots` entirely and validates under era-2 rules UNCHANGED. This is the additive,
strict-superset-rejection shape: era-3 blocks face one MORE condition; no era-2 block's verdict
changes. The gate is a single version comparison at the top of the helper (return nil for
sub-v4), mirroring the existing `b.Version >= BlockVersionRounds` era-gate in `ValidateCommit`
(chain.go:2366) and `VerifyEquivocation` (chain.go:2313).

---

## Which of I1–I5 this touches

**It ADDS a validity rejection gated to v4; it preserves all five and alters none.**

- **I1 (quorum intersection), I3 (set changes at finalized boundary), I4 (commit≠final), I2
  (never sign twice):** UNTOUCHED. The predicate does not read or change any quorum size, the
  weight-sum seam, `epochSet` freeze/`rotateEpoch`, the `⌈A/2⌉`/`⌊A/2⌋+1` threshold, the
  never-sign ledger, or fork-choice. It runs AFTER the existing checks and only ADDS a reject.
- **I5 (accountable safety / deterministic fork-choice):** PRESERVED and reinforced. The
  predicate is a PURE FUNCTION of (block, committed state) — the recompute clones the committed
  state and calls the deterministic `apply()`+`StateRoot()`. Every honest replica computes the
  same verdict, so no honest node is ever induced to reject a block another honest node accepts
  (which would be the fork-choice-divergence failure). It ADDS no slashable event.

The dry-run clone reads `epochSet`/`epochStart`/`matureEpoch`/gate fields but only to feed
`apply()`'s rotate logic on the SCRATCH copy; the live chain's epoch machinery is never touched.
The STOP boundary (do not touch the weight-sum seam / `rotateEpoch` / threshold / value encoding)
holds: `rotateEpoch` runs on the clone, computing the SAME rotation the proposer computed, never
on the live chain.

---

## Model-check tier — every green has a demonstrated RED

In `core/chain/modelcheck_era3_validity_test.go`:

- `TestEra3ValidV4BlockAccepted` — a v4 block whose roots equal the recompute is ACCEPTED by
  `ValidateProposal`. RED if the recompute is wrong (e.g. computed pre-apply instead of post-apply).
- `TestEra3WrongStateRootRejected` — a v4 block with a perturbed `StateRoot` → `ErrEra3StateRootMismatch`.
  RED: remove the StateRoot comparison → the wrong-root block is accepted.
- `TestEra3WrongLogRootRejected` — perturbed `LogRoot` → `ErrEra3LogRootMismatch`. Paired RED.
- `TestEra3NilStateRootRejected` / `TestEra3NilLogRootRejected` — a v4 block with a nil root →
  `ErrEra3RootMissing`. RED: rely on the equality check alone → the dedicated nil test pins the
  explicit named-nil reject the ruling requires.
- `TestEra3PredicateDoesNotFireForV2` — a valid v2 block still passes; a v2 block carrying a
  set-but-wrong StateRoot pointer is STILL ignored (era-2 does not read roots). RED: drop the
  `Version >= BlockVersionStateRoot` gate → the v2 test fails.
- `TestDryRunCloneCopiesEveryAppliedField` — the drift guard (Decision 2).

The ACCEPT/REJECT tests drive the predicate through the real `ValidateProposal` entry point on a
chain with populated committed state, building the v4 block's roots from the post-apply recompute
(the honest proposer's computation) and perturbing for the RED cases.

In `core/chain/reload_era3_boundary_test.go` (the reload-root-check follow-on, Decision 1
correction):

- `TestReloadRejectsResignedWrongStateRootV4` — a v4 block with a wrong `StateRoot` **re-signed
  with the proposer key** is fed to a fresh replica via `Reload`. RED on `a0f8839` (Reload ACCEPTS,
  `err=<nil>`, restores 4); GREEN after the fix with `ErrEra3StateRootMismatch`. The RED is the
  acceptance, and the reject cause is pinned to the root check (not a signature error, not a panic).
- `TestReloadWrongStateRootIsCaughtByTheRootCheckNotTheSignature` — the ablation: it proves the
  re-signed block is signature-VALID (`validateStructural` accepts it) and that the untampered
  history Reloads cleanly (positive control), so the reject is caused by the wrong root, not by an
  unrelated malformation. RED on `a0f8839`.
- `TestReloadRejectsResignedWrongLogRootV4` — the LogRoot sibling → `ErrEra3LogRootMismatch`.
- `TestEveryDiskWritePathRunsTheEra3RootCheck` — the STRUCTURAL write-set guard: it scans
  `chain.go` for every method calling `c.apply(b)` and asserts each runs `validateEra3Roots`
  (directly or via the commit family), except the genesis allowlist. RED on `a0f8839` (it names
  `appendStructural` as an unguarded disk-write path). A future fast-sync/import path that skips the
  check fails this guard structurally, not by a hand-maintained list.

## Reopening / optimization — the hold-the-tree bridge (A-bare → incremental-SMT, #600)

A-bare re-validates each v4 block's committed root on boot by recomputing the SMT over the whole
committed set: O(state)/block, hence **O(depth²) over a full Reload**. This is accepted as the
correct BRIDGE, not the end state (blind-PE ruling `RULING-era3-reload-root-check-2026-08-29.md`,
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era3-reload-root-check-2026-08-29.md`, Q2):

- It does NOT trip the depth-war gate: `TestPerHeightCostLinear` lives in `sim/`, drives live
  commits, and never calls `Reload`. The cost lands on a boot-time path the gate does not measure,
  and it is unreachable until 2c mints v4.
- The linear/stateless end-state is the **incremental-SMT / `ports.NodeStore`** keystone-store
  workstream (#600): maintaining the state tree incrementally per `apply(b)` makes `StateRoot()`
  O(depth) on EVERY path. When it lands, the SAME `validateEra3Roots` call site in
  `appendStructural` becomes linear with **zero change** to the reload check — A-bare is the
  strictly-dominant, forward-clean bridge (PE ruling Q1).
- Tracking target (not a freeze gate): a future `TestReloadCostLinear` that is RED on A-bare and
  GREEN on incremental gives the optimization a red-to-green home.

**Reopening condition:** when `NodeStore` + the incremental tree land (#600), replace the
per-block from-scratch recompute in `validateEra3Roots`/`postApplyRoots` with the incremental
root; the equality check and the named errors stay identical.
