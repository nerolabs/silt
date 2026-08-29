---
name: pr-r3-witness-bound-verification-2026-08-29
description: R3 witness DoS bound (commits 2bafed8 + cf22cdd): all ablations RED confirmed, honest-verify CONFIRMED via coverage, fixtures REAL SMT proofs, full -race PASS; weak-test finding RESOLVED by cf22cdd
metadata:
  type: project
---

## Run: 2026-08-29, commit 2bafed8

Package: `core/statehash` (new files: `witness_bound.go`, `witness_bound_test.go`)
Prior worktree: `/Users/andrewedmond/Claude/claude/silt/.claude/worktrees/agent-a6622e9278da9fd9f`
Second scrutiny run: primary working tree, files checked out temporarily from `increment-2-witness-dos-bound` branch

### Baseline

- `go test -v -count=1 ./core/statehash/...`: 24 tests, all PASS in (0.711s total)
- `go test -race -count=1 ./core/statehash/...`: PASS (2.351s)
- `git diff` at end of run: empty (verified)
- `go build ./core/statehash/...`: clean

---

## Addendum scrutiny (2026-08-29, second pass) — timing / fixture realism / honest-verify

### 1. Per-test wall-clock timings

All tests report `(0.00s)` in `-v` output. Total suite: 0.711s.

The zero timings are NOT suspicious — they are explained by the trie scale. `buildTrie` builds a 2–3 key in-memory SMT (no disk, `simplemap.NewSimpleMap()`). SHA-256 proof generation and `smt.VerifyProof` on a 2–3 key trie complete in well under a millisecond. A 0.00s display is the Go test binary rounding anything below 1ms; it does not indicate a no-op.

Evidence: the total suite wall-clock is 0.711s for 24 tests. Sub-ms per-test is consistent with in-memory crypto on trivially small tries.

### 2. Fixture realism — real SMT proofs vs synthetic blobs

**Reject-path tests (over-cap, over-ceiling, shape violations):** mixed, appropriately so.

- `TestOverProofCapRejectedPreParse`: oversized blob is `bytes.Repeat([]byte{0xAB}, SProofMax+1)` — synthetic garbage. Acceptable: the cap fires on `len(Encoded)` before Unmarshal; the content is irrelevant. The test's safety assertion (NoWitness, never ProvenAbsent) is sound.

- `TestOverProofCapIsPreParseNotVerify`: the bloated proof is a **real** `smt.SparseMerkleProof` struct with real `SideNodes` from `trie.Prove(bloatKey)`, plus an enlarged `NonMembershipLeafData` padded to 1.5×SProofMax. This is NOT a synthetic blob — it is a structurally valid absence proof that WOULD verify against the root if decoded. The test verifies this would pass `validateBasic` (confirmed: the pokt library only checks a minimum size for `NonMembershipLeafData` at proofs.go:63-74, no upper bound). The per-proof byte cap is genuinely the only interceptor. Fixture realism confirmed.

- `TestOverBlockCeilingRejectedPreVerify`: both padded proofs are real `smt.SparseMerkleProof` structs built from `trie.Prove(key)` with real `SideNodes`, padded via `NonMembershipLeafData`. Real proofs.

- `TestShapeViolationsRejected`: the `enc` value for the read key is a real absence proof from `absenceProof()`, which calls `trie.Prove(key)` and `proof.Marshal()`. The `unreadEnc` is also a real absence proof.

- `TestOverBudgetAbsenceQueryNeverProvenAbsent`: honest fixture is a real proof from `absenceProof(t, []byte("serial-K"), ...)`.

**Honest-path tests (TestHonestBundleVerifies, non-vacuous subcase):**

Both the presence proof and absence proof are built via `trie.Prove()` on a real committed SMT trie (built with `smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())`). The proofs are marshaled via `proof.Marshal()` and passed through the full `IngestBlockWitnesses` gate chain.

**Verdict: fixtures on the honest path are real SMT proofs, not stubs.**

### 3. Honest-verify execution — does smt.VerifyProof actually run?

**Method:** coverage profiling, isolated to `TestHonestBundleVerifies` alone.

Results from `go test -coverprofile ... -run TestHonestBundleVerifies`:
- `Resolve` function: **75.0% covered** (the nil-proof branch is not reached by this test, which is correct — the honest path always supplies a proof)
- Line-level HTML coverage for `witness.go:201` (`ok, err := smt.VerifyProof(...)`): `cov8 title="1"` — **executed**
- `return Result{outcome: ProvenAbsent}`: `cov8 title="1"` — **executed** (absent-key subcase)
- `return Result{outcome: ProvenPresent, value: value}`: `cov8 title="1"` — **executed** (present-key subcase)
- `if err != nil || !ok` taken branch (failure): `cov0` — **not executed**, correct (honest proofs verify)

**Verdict: `smt.VerifyProof` genuinely executes on the honest fixture and returns `(true, nil)` for both the membership and non-membership cases. The gate is not decoration.**

### 4. The `buildTrie` helper — is it wired to the real library?

`buildTrie` (in `witness_test.go`) uses:
```
smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())
trie.Update(Key(tagTest, rk), Present)
trie.Commit()
```

The `verifySpec()` in `Resolve` uses `smt.NewTrieSpec(sha256.New(), false)` — the same non-sum SHA-256 spec. The trie and the verifier share the same spec, which is why verification succeeds. This is the correct wiring.

Library confirmed at `/Users/andrewedmond/go/pkg/mod/github.com/pokt-network/smt@v1.0.0` — the adopted v1.0.0 library, not a stub.

---

## Prior ablation results (from first-pass run, same session)

### Ablation 1 — per-proof byte cap disabled (gate 1 commented out)

Defect injected: removed the `if len(rw.Encoded) > SProofMax` return in the gate-1 loop.

RED observed:
- `TestOverProofCapIsPreParseNotVerify`: FAIL — "a bundle with an over-per-proof-cap witness must be rejected" (line 152)

Still GREEN (expected — gate 2 catches it via different path):
- `TestOverProofCapRejectedPreParse`: the single-key oversized blob has total = SProofMax+1 > CBlock(1-key) = SProofMax, so gate 2 fires. Test only checks `Rejected == true` and `MustStall()` — both still true. **This test is a WEAK gate for the per-proof cap specifically** (see finding below).
- `TestOverBudgetAbsenceQueryNeverProvenAbsent/over per-proof cap`: same reason — gate 2 catches it.

Revert: clean.

### Ablation 2 — allNoWitness wired to ProvenAbsent

Defect injected: `Result{outcome: NoWitness}` → `Result{outcome: ProvenAbsent}` in `allNoWitness`.

RED observed (all named the C-7 §104 banned move):
- `TestOverProofCapRejectedPreParse` FAIL
- `TestOverBlockCeilingRejectedPreVerify` FAIL
- `TestShapeViolationsRejected` (all 3 sub-tests) FAIL
- `TestOverBudgetAbsenceQueryNeverProvenAbsent/over per-proof cap` FAIL
- `TestOverBudgetAbsenceQueryNeverProvenAbsent/missing witness` FAIL
- `TestOverBudgetAbsenceQueryNeverProvenAbsent/shape padding (extra unread key)` FAIL

Still GREEN (expected — different code path):
- `TestOverBudgetAbsenceQueryNeverProvenAbsent/malformed_encoding_for_the_read_key`: bundle NOT rejected at bundle level; per-key Unmarshal failure sets NoWitness directly, never goes through `allNoWitness`. Safe under this ablation. Correct.

Revert: clean.

### Independent injections

**Injection A — substituted key (correct count, wrong key):**
Bundle has 1 witness for a substituted key not in the read-set. Shape gate fired:
`"shape gate: witness for a key the block does not read (padding)"`.
Read-set key → NoWitness. Rejected=true. SAFE.

**Injection B — malformed absence proof (under cap, per-key Unmarshal failure):**
99-byte garbage blob (under SProofMax) for a key wanted absent. Bundle passes all 3 gates;
Unmarshal fails → per-key NoWitness. Rejected=false (correctly — only bundle-level gates set Rejected=true).
SAFE.

**Injection C — cap boundary (exact at SProofMax vs one byte over):**
- `SProofMax` bytes: NOT rejected by gate 1 (cap is `>`, not `>=`). Falls to Unmarshal → NoWitness. SAFE.
- `SProofMax+1` bytes: gate 1 fires. Rejected=true. NoWitness. SAFE.
Boundary behaves exactly as specified.

---

## Finding: TestOverProofCapRejectedPreParse was WEAK as a gate-1-specific discriminator (RESOLVED)

**Original finding (first-pass run, 2bafed8):** this test did NOT go RED when gate 1 was disabled, because the test's single-key oversized blob (SProofMax+1 bytes) also breaches the per-block ceiling (C_block = 1·SProofMax), so gate 2 caught it as a fallback. The test confirmed the SAFETY property but did NOT distinguish gate 1 from gate 2 as the interceptor.

**RESOLVED by fix commit cf22cdd.** The test was rewritten: now uses a two-key read-set (C_block = 2·SProofMax) with a small honest proof for the second key, so the over-cap blob + honest proof total stays UNDER C_block. Gate 2 cannot fire. Only gate 1 can catch the oversized blob. The test also asserts `RejectReason` contains `"S_proof_max"` (confirming gate 1, not gate 2).

---

## Fix commit cf22cdd — confirmation run (2026-08-29)

Commit: `cf22cdd` — "fix(statehash): witness floor-box R3 — Kind/Value gate + genuine gate-1 test"
Worktree: `/Users/andrewedmond/Claude/claude/silt/.claude/worktrees/agent-a6622e9278da9fd9f`
HEAD confirmed at `cf22cdd` before run.

### Baseline at cf22cdd

- `go test -count=1 -v ./core/statehash/...`: **30 tests, all PASS** (0.242s)
- `go test -race -count=1 ./core/statehash/...`: **PASS** (1.255s)
- `go vet ./core/statehash/...`: clean
- `go build ./...`: clean
- `git diff HEAD` at end: empty

### Ablation A — Fix 1: Kind/Value gate removed (witness_bound.go lines 270-273)

Gate removed: `if re.Kind == QueryPresent && len(re.Value) == 0 { results[...] = NoWitness; continue }`.

RED observed:
- `TestPresenceQueryEmptyValueNeverProvenAbsent/nil_value`: FAIL — "Kind/Value DISAGREEMENT: a QueryPresent entry with nil value against a valid absence proof resolved to PROVEN_ABSENT — Value won over an authoritative Kind (same class as the R4 empty-value finding)"
- `TestPresenceQueryEmptyValueNeverProvenAbsent/empty_non-nil_value`: FAIL — same message

Exit code 1. Both subtests name the exact defect class.

Restore (`git checkout core/statehash/witness_bound.go`): GREEN. `TestPresenceQueryEmptyValueNeverProvenAbsent` PASS.

### Ablation B — Fix 2: gate-1 (per-proof byte cap) removed (witness_bound.go line 235 block)

Gate removed: `if len(rw.Encoded) > SProofMax { return allNoWitness(..., "per-proof byte cap exceeded (S_proof_max)") }`.

RED observed:
- `TestOverProofCapRejectedPreParse`: FAIL — "an over-S_proof_max witness must reject the bundle pre-parse" (line 109). This is the rewritten test confirming gate-1 specificity — the two-key C_block premise prevents gate 2 from catching the oversized blob.
- `TestOverProofCapIsPreParseNotVerify`: FAIL — "a bundle with an over-per-proof-cap witness must be rejected" (line 182). Also RED as expected (consistent with prior ablation).

Exit code 1.

Restore: GREEN. Both tests PASS.

### Final git diff

```
(empty)
```

Both probes reverted cleanly.

---

## Conclusion

PROMOTED (both fixes confirmed). All properties hold at cf22cdd:
1. Baseline: 30/30 tests PASS, -race PASS, vet clean, build clean.
2. Fix 1 (Kind/Value gate): ablation RED — both subtests name the exact defect (QueryPresent + empty Value → ProvenAbsent); GREEN after restore.
3. Fix 2 (gate-1 test rewrite): ablation RED — `TestOverProofCapRejectedPreParse` now genuinely discriminates gate 1 (two-key C_block premise prevents gate-2 fallback); `TestOverProofCapIsPreParseNotVerify` also RED; GREEN after restore.
4. The weak-test finding from the first pass is resolved. `TestOverProofCapRejectedPreParse` now has a live gate at gate 1 with no gate-2 backstop.
5. Working tree clean throughout (git diff empty at every restore checkpoint).
