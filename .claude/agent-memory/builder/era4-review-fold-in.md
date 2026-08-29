---
name: era4-review-fold-in
description: The five fixes + one scope-out that the PE ruling and Research cert forced into the era-4 witnessable-transitions design doc, with the verified maintenance-site list.
metadata:
  type: project
---

The era-4 design doc (`docs/thinking/2026-08-29-era4-witnessable-transitions-options.md`) was
revised 2026-08-29 to fold in the PE ruling (SHIP-WITH-FIXES) and the Research cert (GATED).

**Why:** the PACE draft had three load-bearing claims that failed verification; the reviews
named the exact fixes and Andrew asked to fold them in (design-doc only, no code).

**How to apply:** when era-4 goes to build, these are the gated conditions — do not ship a
maintenance hook, cap, or boundary without the matching guard/cert below.

The five verified `bonded`/`slashed` maintenance sites in `apply()` (grep-confirmed against
`origin/main` @ `0984db4`, `core/chain/chain.go`): **2989** (`delete(c.bonded, owner)`,
squatter displacement — the one the draft MISSED, an I1 weight-sum attack), **2995**
(`c.bonded[id]=r.Size` register/renew/resize), **3008** (`delete(c.bonded,id)` TTL expiry),
**3019** (`c.slashed[culprit]=true`), **3020** (`delete(c.bonded,culprit)` slash evict). No
sixth per-key site. Two copy sites (not per-key): `cloneForDryRun` and `adopt` (3525-3549)
must copy the `qualified` map — the completeness guards force these.

The fixes folded in:
1. E-2 enumerates all FIVE sites; drift-guard asserts `qualified == filter(bonded,slashed,MinBond)`
   every block, ablated per site, MUST redden on the 2989 hook (Research R1).
2. era-4 is a representation change PLUS one NEW validity rule: a per-block fresh-registration /
   due-bucket cap. #506 bounds per-IDENTITY renewal, NOT distinct-identity regs/block;
   `MaxBondRegBytesPerBlock` is proposer-only (`chainrole.go:798`, 0=unbounded), never validity.
   Cap VALUE is a security parameter, certified numerically before build (SProofMax precedent).
3. Boundary is a POINTER ADVANCE over ONE shared `qualified`/`epochSet` keyspace (zero leaves),
   not a copy into a separate `epochSet` (that gives O(epoch-delta), PE finding 2).
4. New dual-source drift-guard: era-4 keeps BOTH `bondRegHeight` (#506) AND the bucket; assert
   `bucket-membership(id) ⟺ bondRegHeight+ttl+1==D AND bonded present` + byte-identical StateRoot
   vs era-3 replay, ablated on missed old-bucket delete on renew.
5. variant-(b) carried bucket id-list pinned CANONICAL (sorted+dedup+unpadded); shape gate
   rejects non-canonical. Research CERTIFIED Q3 only with this pin.

Scoped OUT (do NOT design in era-4): the `effectiveEpochSet` recovery boundary (`chain.go:1243`,
gated on `c.cfg.LivenessRecoveryHeight`, an operator flag). O-1 (commit `epochStart`) stays —
sound, cheap, no predicate reads it, doubles as the E-2 epoch pointer. Direction of the recovery
boundary (commit recovery-height = trustless/more scope; vs posture-bind the floor box = a trust
surface) is an OPEN decision for Andrew.

Path: revised doc → RE-CERT with Research (lift GATED→CERTIFIED; the one owed numeric is the
cap value) → format veto-gate to Andrew (BlockVersion=5 + new cap rule + one-vs-two keyspace) →
2a→2b→2c behind both drift-guards. Related: [[era4-witnessable-transitions-track]].
