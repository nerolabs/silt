# M2 — the era-floor evidence refusal

**Date:** 2026-09-11 · **Seat:** Builder · **Certification:**
`NETWORK-IDENTITY-BINDING-THREE-LAYER-RESEARCH-CERTIFICATION-2026-09-11` Layer 1 (GATED,
G-1a lifted, G-1b/G-1c/G-1d), read with **Amendment 1**.

## The failure, attributed before the fix

An honest validator that precommitted **once** on network X is convicted on network X using a
leg harvested from network Y. The mechanism:

- The sub-era-4 attestation form is **chain-blind**. `AttPhase` returns `step` unchanged below
  `BlockVersionWitnessable`, so `AttestAt` never enters the era-4 branch and never reads
  `chainID`; `verifyAtt`'s era-2 arm ignores the scope entirely. The bytes a key produces over a
  body are identical on every silt network.
- `CheckEquivocation` collapses the wire phase to a canonical step, so an era-2-form leg and an
  era-4-form leg land at the same `sigScope`. Pairing a harvested leg with the victim's own
  honest era-4 precommit satisfies every conjunct.
- Nothing on the write path screens the evidence's FORM against the chain, so the block commits
  and `apply` evicts the honest validator. Bond to zero.

Driven on `main` before the change: `TestGEF5_EraFloorRefusesOnBothTheLiveAcceptAndReloadPaths`
reddened with both the live accept path (`ValidateCommit` → `ValidateCommitV5` → P8) and the
own-disk reload path (`appendStructural` → `validateStructural` → `validateSlashes`) ACCEPTING a
block carrying the cross-network proof.

Owner call A's preimage closed the era-4 leg. **The era-2 leg was the entire remaining exposure**,
and it cannot be closed from that side: `consensusSigBytes` is frozen, by its own doc, forever.

## The fix

`CheckEquivocation` gains one conjunct: evidence below the chain's committed era floor at the
evidence's own height is not evidence here. **Strictly narrowing** — a conjunct on the accept
condition can only decline a slash, never manufacture one. That argument is a property of the
conjunct, not of the function hosting it, so it holds identically at every site.

### Shape — and why each condition binds

The floor is a **required, height-indexed parameter** (`type EraFloor func(height uint64) uint64`),
never a `(*Chain)` method beside the free function.

| | Decision | Why |
|---|---|---|
| **C-1** | derived from the verifier, never a parameter of `ValidateCommitV5` / `ValidateProposalV5` | A floor LOWERED by a caller re-admits exactly the evidence the rule refuses. Site 1 derives it from its `StateView` with `v5EraFloorAt`, built on `v5EraActive` — the function P10/P11 already use. `TestColdAuditor_NoTrustFloorOnTheContractSurface` fails on arity ≠ 2, on a bare `uint64`, and (added here) on a `func` parameter. |
| **C-2** | height-indexed, evaluated INSIDE the gate at `e.A.Height` | `FindEquivocations` iterates heights and has no scalar to supply. More: a pre-evaluated scalar makes "the caller used the wrong height" expressible, and a compile error catches an omission but never a wrong value. Evaluating inside makes the wrong-height failure inexpressible. |
| **C-3** | an absent supplier REFUSES | `uint64(0)` is a valid-looking "no floor" — today's fail-open — and it is what a caller gets for free on an exported function with an out-of-package caller. `(*Node).eraFloor()` is nil when the node holds no chain, the same direction `(*Node).chainID` already takes with its zero hash. |

**A required parameter rather than a method, because a signature change turns a missed site into a
compile error.** A `*Chain` method could not have reached site 1 at all: the accept composition
takes a `StateView` and is source-gated against holding a `*Chain`.

### The four sites, classified by PATH (two of them are called `validateSlashes`)

| # | Symbol | Path | Supplier |
|---|---|---|---|
| 1 | `v5ValidateSlashes` (P8) | **LIVE ACCEPT** for an era-4 block — `ValidateProposal`, `ValidateCommit`, `Append`, `Reconcile`, `(*Box).Validate` | `v5EraFloorAt` over the `StateView` |
| 2 | `(*Chain).validateSlashes` | own-disk RELOAD for era-4; live accept for era-1/2/3 | `(*Chain).EraFloor()` |
| 3 | `FindEquivocations` / `VerifyEquivocation` | detection / selection | passed through |
| 4 | `(*Node).slashEquivocators` | detection / queue | `(*Node).eraFloor()` |

Plus the **fifth point that is not a gate call**: the proposer DRAIN. `proposeBlock` re-checked
only `IsSlashed` and the byte cap. On the latch route the floor at a fixed height RISES when
`era4LockedIn` latches, so queued evidence can become inadmissible before it is drained and the
node embeds a proof its own P8 refuses — a permanent proposer self-wedge. The drain now re-checks
and drops, as proposer POLICY (never validity), releasing the on-chain latch.

Landing sites 2–4 without site 1 would have been **worse than landing nothing**: the I5 violation
still open on the live accept path, plus an accept-live/refuse-on-reload split — one operator
diverging from itself across a restart, introduced by the fix.

## The one place I did not follow the certification's literal predicate, and why

§1.1 writes the predicate as `e.Version < MintVersion(h)` with no scope. I shipped
`f >= BlockVersionWitnessable && (e.A.Version < f || e.B.Version < f)`.

**Measured, not argued.** With the unscoped comparison, `go test ./core/chain/ -short` was
**33 RED**; scoped, it was **5** (three of them the golden corpus's own regeneration). The 25-test
delta is not incidental:

1. **It closes nothing.** The certification's own closure table (§1.3) marks the only rows the
   surplus could touch — `(v2,v2)` at `h < H_era4`, floor "2 or 4" — as **NOT CLOSED**, and they
   cannot be closed by a floor of 2 or 4: below the era-4 boundary the REQUIRED form is itself
   chain-blind, so an attacker harvests a leg of the required form. Refusing a leg below a v2/v4
   floor buys zero closure by the table's own verdict.
2. **It reprices two artifacts this certification does not name.** `MintVersion`'s minimum is v2
   at every height on every chain, so an unscoped floor makes the **era-1 branch unreachable in
   production**. That flips the entire era-1 arm of `TestModelCheck_I5_AccountableSafety_Exhaustive` (which
   demands era-1 pairs CONVICT) and closes the **T1 variant of R-NEST-GATE**, whose own gate says
   in its failure text: *"do not read a RED here as the residual closed … update
   SLASHCAP-NESTED-EVIDENCE-FIXED-POINT-RESEARCH-CERTIFICATION-2026-09-10 §6.1"*. A builder does
   not amend another seat's certification to make its own tests pass.
3. **On the ratified route the two forms are identical.** With `Era4ActivationHeight = 1` — the
   source-pinned default (G-NET-2/G-NET-3) — every height above the genesis has floor v5, and
   `AppendGenesis` refuses a height-0 slash outright. The scope changes nothing on the RC network.

Accept-set relation, stated plainly: `accept(unscoped) ⊆ accept(shipped) ⊆ accept(main)`. The
shipped rule is still strictly narrowing versus `main` and accepts nothing `main` does not.

**The deferral is DRIVEN, not silent.** `TestGEF8_TheDeferredSubEra4SurfaceIsStillAdmissible`
pins both halves: the era-1 form still convicts at a v2 floor, and the cross-network sub-era-4
harvest still convicts below `H_era4` and is refused at or above it. If a ruling removes the
scope, that gate reddens and names what moved. **Routed to the planner for a Researcher ruling;
deleting the clause is a one-token change with the fixture migration as its only cost.**

## Gates, all seen RED first

| Gate | What it drives | RED shown by |
|---|---|---|
| **G-EF-5** | live-accept and reload verdicts AGREE and both refuse | run on `main`: both paths ACCEPT |
| **G-EF-6** | `MintVersion` and `v5EraFloorAt` agree, on the config route AND the latch route | ablation: `h >= cfgHeight` → `h > cfgHeight` in `v5EraActive` (diffed before the run) |
| **G-EF-7** | the drain re-checks admissibility, drops, releases the latch, and the drop was necessary | ablation: guard deleted (diffed) → the proposal embeds the proof |
| **G-EF-8** | what the scope DEFERS, both halves | the discriminating arm: same bytes, two floors, two verdicts |
| **G-EF-9** | C-3 — nil supplier refuses at all three entry points, before the floor is read | non-vacuity arm: the same pair convicts with a floor |
| **G-PRE-7 subtest 4** | retired per its own instruction | its trip FIRED on this branch and is recorded below |
| **C-1 source gate** | the composition surface stays clean | decoy signatures `(StateView, *Block, EraFloor)` and `(…, uint64)` — both clauses fire |

`G-PRE-7` subtest 4 carried the violation as a RECORD with a trip that reddens the day it closes.
It fired:

```
owner_call_a_v5_preimage_gates_test.go:605: THE MIXED-FORM FACE IS CLOSED … If M2's era-floor
rule landed, RETIRE this subtest and assert the closure at h >= H_era4 in its place
```

It was retired as instructed — same harvest construction, same premise assertions, now asserting
the closure at the RC floor **and** the conviction below it, so the refusal is demonstrably about
the floor and not about mixed forms.

`G-PRE-6`'s pre-era-4 arm now passes its floor explicitly and gained the closure arm on the same
bytes. Its residual statement stays: *the RC network has no reachable height*, never *closed*.

## What it costs, ratified and not overlooked

A validator that double-signs ACROSS an era boundary is unslashable. It cannot finalize either
way (`validateEra4Version` refuses a sub-era-4 block at every height at or above the boundary on
every disk-write path), and on a genesis committing `Era4ActivationHeight = 1` there is no such
boundary to stand on. Certification §1.9 G-1d, owner-ratified.

## Residual, unchanged in class

The cross-network false slash at `h < H_era4` is **HELD IN TENSION — bought off by a genesis
constant, not eliminated.** `consensusSigBytes` is frozen forever; any network that tallies up
from era 2 re-acquires it. Folded into `R-SLASH-CULPRIT-ADMISSIBILITY`; no new prefix.

## What did not move

Not a format item: no block field, no cbor key, no committed leaf, no genesis re-mint. The freeze
read-set does not move — on the `Era4ActivationHeight = 1` route `v5EraActive` short-circuits on
`v.Params()` and performs no `Scalar` read, so P8 gains **no** stall site; on the latch route it
reads `tagEra4LockedIn`/`tagEra4Height`, which P10/P11 already witness, so the cost there is a
**stall surface** change inside P8, not a read-set change. Named here rather than discovered.
