# Closing `SlashesBytesCap`'s derivation route

**Date:** 2026-09-10 (the owner's call came 2026-09-09, late in the preceding session).
**Status:** BUILT. Gates G-SLASHCAP-1..4.
**Origin:** the pre-freeze derivation-route audit,
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-derivation-route-audit-pre-freeze-2026-09-09.md`,
itself commissioned by `D-C2-IDLE-WINDOW-VALUE` when the owner asked what else had been derived
the way the delivery idle window was.

## The defect, stated once

`chain.SlashesBytesCap` (16 MiB) is a consensus validity rule enforced on every validator
(`core/chain/validate_v5_predicates.go:285`). Its documented invariant is

    SlashesBytesCap ≥ 2 × (honest block) + overhead

and "honest block" was computed from the **default values** of `-max-bondreg-bytes-per-block`
(2 MiB) and `-max-entry-bytes-per-block` (64 KiB). Those two flags are **proposer-side only** —
every non-test read is `core/node/chainrole.go:890` and `core/node/entrypool.go:116` — and the
shipped help documented `0 = unbounded`. Nothing bound the running configuration to the
derivation.

**Consequence.** An operator raising its own bond-reg budget past ~7.9 MiB makes its own
equivocation unprovable: the evidence pair (two full block bodies, since `F2-EVIDENCE-RECOMPUTE`
made full bodies the only admissible evidence) exceeds the cap, the cap rejects it *before*
`CheckEquivocation` runs, and the double-signer keeps its seat. Slashing defeated by making the
evidence too big.

The owner's framing, which is sharper than the audit's: **this is the #380 class**, which silt
paid for in the same week — `RequiredQuorum()` returning the local `cfg.Quorum` in the mature
regime, an I1 divergence. A consensus quantity must be a function of the chain, never of local
config. Two instances of one class inside a week is an unguarded seam, not a coincidence.

## Options considered

| Option | Cost | Verdict |
|---|---|---|
| **A. Re-ratify 16 MiB as-is** | Zero now. Leaves a validator able to make itself un-slashable by editing one local flag. | **Rejected by the owner.** The accountability face is a Part-0 corner. |
| **B. Raise the cap to cover any configurable budget** | No admissible value exists — the budgets are unbounded by construction (`0 = unbounded`). Also *raising* a cap is a WIDENING rule change, so it cannot ride the narrowing exemption. | Rejected: no value closes it. |
| **C. Derive the cap from the chain** | Would make the cap a function of committed state. A format/validity change of real size, and the cap is already chain-uniform (a `const`) — the defect is in the *configuration*, not in the constant. | Rejected: wrong target. |
| **D. Refuse a configuration that violates the invariant** ✅ | One predicate + one startup refusal. Value unchanged, no consensus rule touched, derivation true by construction. | **TAKEN.** The `IssuerKeys`-cap precedent. |

## What was built

`core/node.CheckSlashEvidenceHeadroom(cfg)` expresses the invariant on the values **in force**,
and `cmd/silt/daemon.go` refuses to start on a non-nil return. Three refusal classes:

1. `-max-bondreg-bytes-per-block ≤ 0` — the proposer's guard is `budget > 0`, so zero *and*
   negative both mean unbounded, and an unbounded body makes the invariant unsatisfiable at any
   cap. Refused rather than clamped: clamping silently re-interprets an explicit operator request.
2. `-max-entry-bytes-per-block ≤ 0` — the same door, the other flag. The cap bounds the whole
   encoded body; entries are not exempt.
3. `2 × (regs + entries + 64 KiB slack) > SlashesBytesCap` — the invariant itself.

Every refusal names `SlashesBytesCap`, the flag to change, the admissible ceiling, and what is
lost if it starts anyway. An operator who reads only the error can act on it.

## Why the gate is where it is

The scar discipline says gate where the **runtime** value is read, never on the constants that
feed it (`silt-gates-hold-the-seam`). `CheckSlashEvidenceHeadroom` takes `cfg`, and
`maxHonestBondRegBytes` is a function of the runtime *entry* budget rather than of that flag's
default — the whole defect being closed is a bound computed from a default instead of from the
value in force, so re-introducing a default inside the fix would reproduce it one level down.

The ablation ran **first** and went red: with the predicate stubbed to `return nil`, six refuse
rows failed and both admit rows passed. That is the seam.

## Scope — stated so it is not over-read

This closes the **configuration** route. The cap's disclosed *second face* — that a ≥⅓ coalition
can make every evidence pair over-cap with ~6 of its own valid renewals per block, so accountable
safety degrades to plain safety for fat coalitions — is **untouched**, and no admissible cap value
closes it. Only the v5 two-level block hash (d-3, fixed-size evidence) removes that face, and it
is a FORMAT item in the D1 train.

A second, narrower hole the audit named is **not** closed here and is filed rather than folded in:
`core/node/chainrole.go:902` embeds the first fresh reg unconditionally when the block carries no
regs yet ("never stall the queue on a single oversized proof"), and silt has **no per-reg byte
cap**, so one arbitrarily large registration can exceed the configured budget. The headroom check
bounds the *budget*, not that single-reg overflow. Closing it needs a per-reg ceiling, which is a
validity rule of its own.

## Timing

A validity rule, not a format item — freeze manifest item 10 — so its deadline is the **stamp
raise**, not the freeze. Not deferrable past it: raising a cap later is a *widening* rule change
and is outside the narrowing exemption that makes post-freeze rule changes cheap
(`docs/era4-freeze-what-closes.md`). It is now, or it is a coordinated fleet fork.
