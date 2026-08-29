---
name: ruling-r3-witness-bound-review
description: R3 witness floor-box DoS bound (commit 2bafed8) — SHIP-WITH-ONE-FIX; pre-parse caps genuine, banned move unreachable (I ablated), LOW fix = QueryPresent+empty Value silently ProvenAbsent
metadata:
  type: project
---

Ruling: R3 witness floor-box DoS bound, commit `2bafed8`, files
`core/statehash/witness_bound.go` + test. Builds on R4 accessor `witness.go` (8bc7e79).
Ruling at `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R3-bound-review-2026-08-29.md`.

**Verdict: SHIP-WITH-ONE-FIX (LOW).**

**Verified myself:**
- Pre-parse cap is GENUINE: gates 1-3 run on `rw.Encoded` bytes; `Unmarshal` first at
  witness_bound.go:263, inside post-gate loop. Library `validateBasic` runs INSIDE
  `verifyProofWithUpdates` (pokt smt proofs.go:403) = post-parse, useless as DoS gate.
- Byte-cap-not-count-cap justified: `validateBasic` caps side-node COUNT (proofs.go:57)
  but only MIN-checks `NonMembershipLeafData` (proofs.go:63-75), never max → unbounded
  leaf blob. Byte cap closes it. Builder claim exact.
- Banned move (C-7 §104) UNREACHABLE: `allNoWitness` builds only NoWitness zero-values;
  ProvenAbsent constructible only at witness.go:219 after VerifyProof true + len(value)==0,
  reached only post-all-gates. I ran my own ablation probes: membership-proof-for-absence-query
  → NoWitness; wrong-expected-value → NoWitness. Forgery stalls, not accepts.
- C_block = len(readSet)*SProofMax, NOT adversary-inflatable at this layer (adversary owns
  bundle not read-set; padding caught by shape gate).
- Gate 2 NOT redundant with shape gate: it's a cheap early-out for duplicate-key
  count-blowup, runs BEFORE shape maps built (test measures reason=="C_block").
- Scope CLEAN: no I1-I5, no era-3 format, no per-block transition cap, zero external callers.

**The LOW fix (finding 1, I measured it):** `ReadEntry{Kind:QueryPresent, Value:nil}`
→ line 272 leaves value len 0 → Resolve routes to non-membership → ProvenAbsent. Kind
and Value can DISAGREE and Value wins. Doc says "MUST carry non-empty value" but it's
unenforced. Fix = reject/NoWitness when QueryPresent && len(Value)==0.

**The seam call (question asked):** read-set-as-`[]ReadEntry`-input is a clean interface
but security is CONTINGENT on the unbuilt `Block → read-set` derivation. Later increment
MUST guarantee (a) completeness — every read key present, else silent unverified predicate;
(b) faithful (key,kind,value). Shape gate pins bundle-to-read-set, NOT read-set-to-block.
Finding-1 fix partly self-checks (b).

**Contingent on:** the 16 KiB SProofMax + C_block research cert
(witness-floor-box-dos-bound-RESEARCH-CERTIFICATION-2026-08-29) — I did NOT read it
(blind review); value-choice is research-gated + Andrew-ratified. Related:
[[ruling-r4a-witness-accessor-spine]], [[ruling-witness-floor-box-mechanism]].
