---
name: ruling-r4a-witness-accessor-spine
description: R4-a witness accessor (core/statehash/witness.go, commit b6642da) — SHIP-WITH-ONE-FIX; spine sound, empty-value mislabel is the LOW fix, I owned an induced flake miss
metadata:
  type: project
---

Ruling on commit `b6642da` — `core/statehash/witness.go`, the three-valued
witness accessor spine (ProvenPresent / ProvenAbsent / NoWitness). Filed:
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R4-accessor-review-2026-08-29.md`

**Verdict: SHIP-WITH-ONE-FIX (LOW).** The safety spine holds. Verified against
the pokt smt library directly, not the builder's comments:
- ProvenAbsent has exactly one construction site (witness.go:202-205), inside the
  `value==nil` branch after `VerifyProof`==(true,nil); `outcome` field unexported.
- NoWitness is the zero value (iota) — safe state is the default.
- All 4 failure classes (wrong root, tampered, membership-as-absence, wrong value)
  + empty/zero proof struct → NO_WITNESS. Empty-proof forgery closed by the
  library's final `bytes.Equal(currentHash, root)` (placeholder != committed root).
- verifySpec = NewTrieSpec(sha256,false) is BYTE-IDENTICAL to what
  NewSparseMerkleTrie builds (smt.go:31). Spec drift fails safe (all→NoWitness,
  never a false read).
- Scope clean: validation-layer only, no I1-I5, era-3 format untouched, NO caller yet.

**THE ONE FIX (LOW):** `Resolve` selects absence-vs-presence on `value == nil`,
but `smt.VerifyProof` selects on `bytes.Equal(value, defaultEmptyValue)` where
defaultEmptyValue is nil — so `[]byte{}` (empty non-nil) takes the library's
NON-membership branch while Resolve routes it to ProvenPresent. Measured:
empty-value query on an absence proof → PROVEN_PRESENT value="". Mirror of the
banned move (false PRESENCE). Unreachable today (all silt encoders non-empty:
Present=[]byte{1}, Encode* fixed-len) but a latent mislabel with no test. Fix:
route on `len(value)==0` or reject empty; add the ablation case. Also widen the
construction-site scan to guard ProvenPresent too (scan only guards ProvenAbsent).

**MISS I OWNED:** captured one full-suite run where the classify test FAILED
(verified proofs → NO_WITNESS). Did NOT reproduce (0/20 plain, 0/10 race+shuffle,
passes isolated). Cause: my own repeated add/remove of zz_* probe test files
polluted that compile. Struck as a code finding — committed code is deterministic.
Lesson: checkpoint/clean probe files before reading a suite-fail as a defect.

Couplings for later increments: R3 byte-ceiling + D-2 fetch-fail both sink into
NoWitness — keep the source-scan gate; the exposure is a future ProvenPresent off
unverified input, which the current scan does NOT cover.
