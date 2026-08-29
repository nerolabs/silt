---
name: pr633-witness-r4a-verification-2026-08-29
description: Witness floor-box R4-a (b6642da + e6712a0 fix) verification 2026-08-29 — all ablations RED, all independent injections SAFE, fix commit confirmed PROMOTED
metadata:
  type: project
---

Run date: 2026-08-29. Commits b6642da (initial) and e6712a0 (empty-value fix) on branch worktree-agent-ab1d67cf5d9637f5d.
Files: core/statehash/witness.go, core/statehash/witness_test.go.

## Baseline at b6642da

- `go test ./core/statehash/...` — PASS (14 tests, 0.54s–0.84s)
- `go vet ./core/statehash/...` — PASS (no output)
- `go build ./...` — PASS (no output)

## Author ablations at b6642da — personally witnessed RED then GREEN

**Ablation A: nil proof → ProvenAbsent injection**
- Defect: replaced `return Result{outcome: NoWitness}` in the nil-proof branch with `return Result{outcome: ProvenAbsent}`.
- RED: TestNoWitnessNeverAbsent (witness_test.go:99 "BANNED MOVE") + TestProvenAbsentHasExactlyOneConstructionSite ("found 2 sites").
- Reverted → GREEN (full suite).

**Ablation B: failed-verify → ProvenAbsent injection**
- Defect: replaced `return Result{outcome: NoWitness}` in the `err != nil || !ok` branch with `return Result{outcome: ProvenAbsent}`.
- RED: all 3 sub-tests of TestFailedVerificationNeverAbsent (wrong-root, tampered, membership-as-absence) + TestProvenAbsentHasExactlyOneConstructionSite ("found 2 sites").
- Reverted → GREEN (full suite).

## Independent injections at b6642da (Tester-authored)

**Injection 1: External struct-literal forge**
- Wrote an external Go module that tried `statehash.Result{outcome: statehash.ProvenAbsent}`.
- Compiler rejected: "cannot refer to unexported field outcome in struct literal of type statehash.Result".
- SAFE: the unexported-field guard is a compile-time impossibility, not a code-review catch.

**Injection 2: verifySpec spec mismatch (sum=true flip)**
- Flipped `smt.NewTrieSpec(sha256.New(), false)` to `true` in verifySpec().
- Proofs built against the non-sum trie failed verification → outcome was NO_WITNESS (SAFE), not ProvenAbsent.
- TestNoWitnessNeverAbsent and TestFailedVerificationNeverAbsent both PASS (safety invariant holds even with broken spec).
- TestResolveVerifiedProofsClassifyCorrectly FAILS (correct: valid proofs now stall the floor box). This is the expected liveness cost of a spec mismatch; the safety cost is zero.
- Reverted → GREEN.

**Injection 3: Cross-root proof (separate module)**
- Built standalone program: honest absence proof from trie A applied to trie B's root.
- Result: NO_WITNESS (SAFE). Output: "PASS: cross-root proof correctly degrades to NO_WITNESS".

---

## Fix commit e6712a0 — empty-value mirror of the banned move

**Fix:** `Resolve` keyed absence on `value == nil`; the pokt-network/smt library keys on
`bytes.Equal(value, defaultEmptyValue)` where `defaultEmptyValue` is nil. `bytes.Equal`
treats nil and `[]byte{}` as equal, so a `[]byte{}` query against a valid absence proof
verified (library: non-membership) but resolved to ProvenPresent here (wrong branch). Fix:
key on `len(value) == 0` to match the library.

### Baseline at e6712a0

- `go test -count=1 -v ./core/statehash/...` — PASS (all named tests GREEN, 0.178s)
- `go vet ./core/statehash/...` — PASS (no output)
- `go build ./...` — PASS (no output)

### Ablation C: revert len(value)==0 to value==nil (personally witnessed RED→GREEN)

- Mutation: sed replaced `if len(value) == 0 {` with `if value == nil {` at witness.go:218.
- RED (exit 1, two failures):
  - TestEmptyValueNeverPresent: `MIRROR OF BANNED MOVE (C-7 §104): an empty-value ([]byte{}) query against a valid absence proof resolved to PROVEN_PRESENT value="" — a false presence off an absence proof`
  - TestOutcomesHaveExactlyOneConstructionSite: `the 'if len(value) == 0 {' non-membership guard is missing — PROVEN_ABSENT must be reachable only through the verified non-membership branch, keyed on len==0 to match the library's empty-value convention`
- Orthogonal tests stayed GREEN: TestNoWitnessNeverAbsent, TestFailedVerificationNeverAbsent.
- Restored → full suite GREEN.

### Ablation D: second ProvenPresent construction site (personally witnessed RED→GREEN)

- Mutation: appended `func _duplicatePresent() Result { return Result{outcome: ProvenPresent} }` to witness.go.
- RED (exit 1): TestOutcomesHaveExactlyOneConstructionSite — `expected EXACTLY ONE construction site for PROVEN_PRESENT, found 2. A second site is the MIRROR banned move...`
- Removed → `git diff` zero; full suite GREEN.

### git diff at e6712a0 after all ablations

`git diff core/statehash/witness.go` → empty (no residual edits confirmed).

## Summary

All ablations (A–D) went RED under injection and GREEN after restore. The fix commit
e6712a0 is PROMOTED. TestOutcomesHaveExactlyOneConstructionSite now guards BOTH
ProvenAbsent and ProvenPresent construction sites — the source-scan gate covers the full
banned-move surface, not just the original absence half.

**Why:** The outcome field is unexported; ProvenAbsent and ProvenPresent are each
constructible in exactly one literal in witness.go; TestOutcomesHaveExactlyOneConstructionSite
is a second-order guard that fires on any new construction site for either. The len(value)==0
guard position assertion also fires if the guard is removed or moved. These are load-bearing
coverage, not comments that compile.

**How to apply:** The R4-a accessor (both commits) is PROMOTED. Callers that add a new
ProvenPresent or ProvenAbsent construction path inside the package will be caught by the
site-counter test; callers outside the package cannot forge one. The remaining gap (not in
scope for R4-a): R3 byte-ceiling gate and D-2 delivery are not built yet; their missing-proof
arm will arrive as NoWitness, which is the correct safe default.
