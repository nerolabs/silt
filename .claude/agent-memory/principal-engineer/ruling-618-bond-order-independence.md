---
name: ruling-618-bond-order-independence
description: PR #618 bond-registration order-independence — SHIP-WITH-CHANGES; "G3 order-independent" is OVERSTATED, same-root two-proven-claim ordering IS order-dependent (I measured it)
metadata:
  type: project
---

# Ruling: PR #618 bond-registration family order-independence (#617 debt)

**Verdict: SHIP-WITH-CHANGES.** Filed
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-618-bond-registration-order-independence-2026-08-28.md`.

**The finding I own (measured, not argued):** The PR claims "G3 is
order-INDEPENDENT" and removes six fields from `orderVacuous`. FALSE as stated. The
fixture's two proven regs target DISJOINT roots (rootShared vs rootX) — independent
map writes, trivially commutative. That is NOT where G3/F1 could be order-sensitive.
The stressing ordering is TWO PROVEN CLAIMS ON THE SAME ROOT in one block. I wrote a
throwaway test in the worktree: genesis squat + height-1 block with claimA/claimB
both on rootShared, order flipped → `bonded` and `bondRootOwner` DIVERGE, block
appends with no validity rejection. So the property the freeze gate needs is unproven
for the case that can actually diverge.

**Why reachable:** No per-root dedup at the validity layer. `validateBondRegs`
(chain.go:1411) / `validateBondReg` (chain.go:1482) check sig/size/proof, and only
`regGateActive` runs a per-VALIDATOR-ID `seenReg` (never per-root). The F1/G3
tie-break lives ONLY in `apply()` (chain.go:2780-2799), which is order-dependent for
same-root contention. Admissible in every regime.

**Verified premises:** deep-copy fix (deepCopyValue, one-level map copy) is correct
AND complete for the current probe set — apply() REPLACES map values
(chain.go:2743), never mutates in place. Residual: `revLog` (*translog.Log pointer)
is carried by snapshotBoot and NOT deep-copied — harmless today (no probe appends an
entry block), a trap for the next mutating probe. bondRootProven snapshot probe is
real (verdict flip proven-owner-held ↔ displaced-the-proven-owner). probeUncovered
declarations honest (bondDomain metric-only; bondRegHeight/regVersion #506-gated).
Q4 clean: test/fixture-only, chain.go unchanged.

**The coupling the consult missed:** the era-3 state-root freeze gate. Freezing
bondRootOwner/bondRootProven/bonded as "order-independent under the
history-independent SMT" on this fixture leaves the same-root contention as a latent
fork (same block contents, different intra-block order → different state root).

**The human's call:** whether same-root two-proven-claim is a real network condition
or proposer-filter-unreachable (#508). My rec: admissible at validity layer, treat as
reachable. Research-adjacent — route the divergence to the Researcher before any
freeze depends on these fields.
