---
name: pr618-bond-reg-oracle-verification-2026-08-28
description: PR #618 (order-independence-bond-registration-family) verified 2026-08-28 — all ablations RED (both oracle round and sameroot-intrablock round), both rounds full suite green under -race
metadata:
  type: project
---

PR #618 verified by the Tester seat on 2026-08-28. Branch: `order-independence-bond-registration-family`.

## Round 1 (HEAD 4111313) — oracle and order-independence verification

### Baseline

All 8 targeted tests PASS: `TestCommittedSetFieldsAreOrderIndependent`, `TestCommittedLogFieldsAreGenuinelyOrderDependent`, `TestBondRegG3DisplacementIsOrderIndependent`, `TestRevLogRootIsOrderDependent`, `TestBondedOrderFreeUnderSlashInteraction`, `TestEpochWeightBytesAreLoadBearing`, `TestSnapshotBootMatchesReplayBoot`, `TestLeaveOneOutProvesEachFieldLoadBearing`.

### Ablation (a) — first-BondReg-only injection

Injection: replaced apply()'s BondReg loop at `chain.go:2769` to process only `BondRegs[:1]`.

RED line — `TestCommittedSetFieldsAreOrderIndependent`:
> `6 field(s) classified committedSet DIFFER between two histories: [bonded bondRootOwner bondRootProven bondRegHeight regVersion bondDomain]`

RED line — `TestBondRegG3DisplacementIsOrderIndependent`:
> `[hClaimFirst] validatorX not bonded on its own root (bonded=0 ...)`

Revert: both tests return GREEN.

### Ablation (b) — bondRootProven leave-one-out probe

Both probes flip on bondRootProven omission (no nil-panic):
- bondRootOwner probe: `full=claim-blocked ablated=claim-succeeded`
- bondRootProven probe: `full=proven-owner-held ablated=displaced-the-proven-owner`

Non-adversarial injection: neutralized both mutating probes (bondRootOwner and bondRootProven) — oracle goes RED:
> `omitting committed field "bondRootOwner" changed NO verdict in any world.`
> `omitting committed field "bondRootProven" changed NO verdict in any world.`

NOTE: `bondRootProven` is detected FIRST by the `bondRootOwner` probe (probe index 0 runs before the bondRootProven-specific probe). Both probes cover bondRootProven. Neutralizing only one leaves the oracle GREEN via redundant coverage. Neutralizing both triggers RED. Coverage is redundant, not vacuous.

Revert: oracle returns GREEN.

### Ablation (c) — G3 order-dependence injection

Injection: removed F1/G3 guard at `chain.go:2780-2790` (last-writer-wins).

RED line — `TestBondRegG3DisplacementIsOrderIndependent`:
> `[hClaimFirst] G3 did NOT fire: squatter still bonded (2097152) — the coverage is vacuous, the displacement branch was never taken`

NOTE: `TestCommittedSetFieldsAreOrderIndependent` still PASSES with this injection, because with last-writer-wins, the hClaim (the only claimant for rootShared at height 5) wins rootShared in both orderings — the final fields are identical across orderings even though the squatter was not displaced. The G3-specific test is the sharper oracle for this invariant.

Revert: GREEN.

### Masked-green fix (task 3)

Deep-copy confirmed present at `modelcheck_snapshot_equivalence_test.go:125`:
```go
setField(dst, name, deepCopyValue(fieldValue(src, name)))
```

Leakage test: ran all 5 probes against snapshotBoot replicas; `src.bonded` unchanged before and after. Two independent replicas (r1, r2) — r1.apply() did not affect r2.

Ordering test: 90 probe/field combinations identical in forward and reverse probe evaluation order.

**Verdict: deep-copy fix is COMPLETE. No cross-probe state leakage detectable.**

### Final clean green (Round 1)

- `go test ./core/chain/... -count=1`: PASS, 2.119s
- `go test ./core/chain/... -count=1 -race`: PASS, 9.363s
- `git status`: working tree clean (only `.claude/` untracked)

---

## Round 2 (HEAD 4c10525) — same-root intra-block consensus rule verification

New commit: `fix(chain): CONSENSUS-RULE — reject same-root distinct-ID bond regs in one block (#618)`

Key addition: `validateBondRegs` gains a `seenRoot` map (unconditional, not gate-gated) that fires `ErrSharedRootInBlock` when two DISTINCT-ID registrations claim the same root in one block. Covering probe: `redteam_verify_sameroot-intrablock_test.go`.

### Step 1 — baseline

`go test ./core/... -race -count=1 -timeout=300s`: ALL 21 packages PASS. `core/chain` 14.2s.

### Step 2a — GREEN (as shipped)

`go test ./core/chain -run "TestSameRoot|TestSharedRootDeniedViaValidatedBlock" -v -race`:

- `TestSameRootDistinctIDIntraBlockRejected/claimantA-first`: PASS. Error: `chain: block carries two bond registrations from distinct identities on the same root (order-dependent commit — refused): root claimed by both 454ff875079f... and 9f14649924... in one block`
- `TestSameRootDistinctIDIntraBlockRejected/claimantB-first`: PASS. Error: `chain: block carries two bond registrations from distinct identities on the same root (order-dependent commit — refused): root claimed by both 9f14649924... and 454ff875079f... in one block`
- Rejection is NOT vacuous: the claimant order in the error string flips between subtests (first-seen vs second-seen), confirming the guard evaluates actual slice order.
- `TestSameRootDistinctIDNoDivergentCommit`: PASS
- `TestSharedRootDeniedViaValidatedBlock`: PASS — 4 distinct-ID regs on one root, `ErrSharedRootInBlock` returned.

### Step 2b — RED (ablation: return on seenRoot collision commented out)

Injection: comment out `chain.go:1486` — the `return fmt.Errorf("%w: root claimed by both..."` in `validateBondRegs`.

- `TestSameRootDistinctIDIntraBlockRejected`: FAIL both subtests. "block with two distinct-ID proven regs on ONE root was ADMITTED"
- `TestSameRootDistinctIDNoDivergentCommit`: FAIL. "a same-root distinct-ID block committed (errA=<nil> errB=<nil>)"
- `TestSharedRootDeniedViaValidatedBlock`: FAIL. "F1 regression: a block with 4 distinct-ID regs on ONE root committed"

**Divergence values captured (ablation active, probe `TestCaptureAblationDivergence`):**

Chain A (claimantA-first, Append err=nil):
- bonded[idA]=2097152 bonded[idB]=0
- bondRootOwner[rootShared]=`3435...34` (hex-encoded idA)

Chain B (claimantB-first, Append err=nil):
- bonded[idA]=0 bonded[idB]=2097152
- bondRootOwner[rootShared]=`3966...63` (hex-encoded idB)

DIVERGENCE CONFIRMED: bondRootOwner differs by slice order. idA prefix: `454ff875079f`, idB prefix: `9f1464992465`.

Revert: chain.go restored from backup. `grep -n "ABLATION"` returned no hits.

### Step 3 — negative control

`TestSameRootSameIDRenewAdmitted`: PASS (under -race). A same-ID double registration (renew + resize) on own root is admitted. `bondRootOwner[rootOwn] = ownerID`, `bonded[ownerID] = minBond*2` (resize took effect). The seenRoot guard correctly fires only on DISTINCT-ID collisions.

### Step 4 — flipped-test verdict

`TestSharedRootDeniedViaValidatedBlock` now asserts `ErrSharedRootInBlock` is returned. Under ablation (Step 2b) it went RED with "a block with 4 distinct-ID regs on ONE root committed". Under the fix it is GREEN. Not vacuous.

### Step 5 — final clean green

- working tree: only `?? .claude/` untracked (orchestration config). No ablation residue.
- `go test ./core/chain -race -count=1 -timeout=120s`: ALL PASS, 19.674s.

**VERDICT: PR #618 HEAD 4c10525 is correct. All ablations go RED. All controls hold. Working tree clean.**
