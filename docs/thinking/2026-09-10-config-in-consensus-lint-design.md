# The config-in-consensus lint — options, decision, and what it deliberately does NOT claim

**Date:** 2026-09-10 · **Seat:** Builder · **Canon:** `docs/build-process.md` rule 8 (owner ruling,
2026-09-10), simplicity rules 1/7/8, `docs/decisions.md` `D-PREIMAGE-CERT-2026-09-10`.

## The problem, measured not assumed

`Params` embeds `Config` verbatim (`core/chain/stateview_live_v5.go:26`) and every `Config` field is
a command-line flag (`cmd/silt/daemon.go:905-916`). So a consensus validity predicate can read local
config, and three times it has: **#380** (`RequiredQuorum()` read `cfg.Quorum` on the objective path),
**`SlashesBytesCap`** (an invariant derived from two proposer-side flag defaults), and **`MinBond`**
(a validity threshold that is a bare flag — found by a driven probe this session).

silt names the class in prose — *"consensus-critical genesis config"* (`core/chain/chain.go:190`,
`:253`) — but only in scattered doc comments. **There is no enumeration and no enforcement, so
membership is a human remembering to write the sentence.** That is the root cause, and it is the same
"repeated hand-enumeration misses" that `docs/decisions.md:962` prescribes a mechanical sweep for.

## Options

**Option A — grep-style source lint** (flag `c.cfg.` / `p.<Field>` reads inside validity files).
*Cost:* cheap. *Benefit:* none that survives review. `chain.go` alone has 78 `c.cfg.` reads across
proposer, gather and retention paths; the signal-to-noise is hopeless, and it pattern-matches syntax
rather than the property. It would also have MISSED `SlashesBytesCap` entirely (a `const`, not a
field read). **REJECTED.**

**Option B — hand-maintained list of consensus-critical fields, checked for doc annotation.**
*Cost:* cheap. *Benefit:* small. It re-encodes the exact failure mode being fixed: the list is a human
enumeration, so a new field is a new miss. **REJECTED** — it is the disease, formalised.

**Option C — driven divergence sweep + a reflective declaration table.** Two honest replicas whose
`Config` differs in exactly ONE field must reach the SAME validity verdict on the SAME block. Every
`Config` field is reached by REFLECTION, so a newly added field with no declaration is RED. **CHOSEN.**

## The decision

Option C, because it checks the PRINCIPLE (canon rule 8: a consensus quantity is a function of the
chain) rather than the three instances already found — which is exactly what the owner directed when
he put the rule in the canon *before* the lint was written.

**The invariant, stated so it has a closed complement:** every field of `chain.Config` is declared
either `LOCAL` (free to differ between honest replicas) or `CONSENSUS-CRITICAL` (must be swarm-uniform,
and canon rule 8 says how to bind it). Reflection over the struct makes the complement closed: there
is no third state, and no field can be silently absent.

**RED conditions:**

1. A field declared `LOCAL` whose perturbation CHANGES a validity verdict. (This is the defect class.)
2. A field with NO declaration at all. (A new `Config` field forces the decision at review time.)
3. A declaration whose stated regime was never actually entered by the fixture — see below.

**The ablation, run BEFORE the gate was written** (`docs/decisions.md` `D-GATES-HOLD-THE-SEAM`
discipline): reverting the #380 fix so `RequiredQuorum()` returns `cfg.Quorum` unconditionally makes
`Quorum` diverge on the objective path again — the sweep goes 2 → 3 diverging fields. The gate reddens
on the real historical defect, so it is not decoration.

## What this deliberately does NOT claim — the rule-7 discipline

Simplicity rule 7: *"a green gate with no demonstrated red is decoration… a field classified 'safe' in
any coverage table must be a DRIVEN probe."* A height-1 block in a four-anchor swarm cannot exercise
epochs, maturity, the anchor launch window or retention. **A field that does not diverge in a regime
that could never have exercised it has not been shown safe — it has been shown untested.**

So the gate reports **three** states, not two, and the difference is load-bearing:

- **DIVERGES** — measured, and the field must be declared `CONSENSUS-CRITICAL` with its binding status.
- **DRIVEN-SAFE** — the fixture provably entered a regime where this field is read, and the verdict
  still did not move.
- **UNPROVEN** — no driven regime exercises it yet. **Reported as unproven, never as safe**, and the
  gate names which regimes ran.

`UNPROVEN` is the honest state for most fields on day one. Regimes get added over time and fields
migrate `UNPROVEN → DRIVEN-SAFE`. A gate that called all fourteen quiet fields "safe" today would be
the decoration failure in a new place — the same failure the source-gate lint
(`scripts/check_source_gates.py`) was built to stop.

## Why this is a Go test and not a `scripts/*.py` lint

`check_source_gates.py` reads source TEXT because its property is textual. This property is a RUNTIME
verdict: only executing `ValidateCommit` against two differently-configured replicas can observe it.
A source-text gate here would promise a runtime property it cannot see — the exact scar
`check_source_gates.py` exists to prevent. The reflective half (every field declared) is structural
and rides in the same test, where the runtime half can cover it.
