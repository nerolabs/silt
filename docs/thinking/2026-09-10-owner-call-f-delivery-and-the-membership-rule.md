# Owner call F's delivery, and the membership rule that governs it

**Date:** 2026-09-10 · **Seat:** Builder · **Base:** `origin/main` `0ed3b92`

## The attribution, before any code

*The failure is that a network minted by any silt binary commits no consensus configuration at
height 0, because `core/genesis.Build` constructs `chain.Block{Version, Height: 0, Entries}` with no
`Params` and cbor `omitempty` then drops key 20 — so the "a divergently-configured node cannot join"
mechanism described in `D-CFGBIND-BUILT-2026-09-10` has no production existence. This change
addresses it by requiring `Build` to take the params, projecting them off the chain in the daemon,
and giving `CheckConsensusParams` its first non-test caller.*

Evidence gathered before writing anything, all on `0ed3b92`:

| Claim | How it was checked | Result |
|---|---|---|
| Nothing populates `Params` | `grep -rn "Params:" --include=*.go core cmd adapters \| grep -v _test` | only the two `bodyHash` pass-throughs |
| `CheckConsensusParams` is dead | `grep -rn "CheckConsensusParams"` | 4 sites, all `consensusparams_test.go` |
| `ParamsFromConfig` is dead | same grep | 3 sites, all in that same test file |
| Genesis mints paramless | read `genesis.Build` | `Block{Version, Height, Entries}` |
| The key is droppable | `grep` the struct tag | `cbor:"20,keyasint,omitempty"` on a pointer |

## Options considered

### Where the params come from

| Option | Cost | Why not / why |
|---|---|---|
| Build them from the `chain.Config` literal at the mint site | free | **Rejected.** `CheckConsensusParams` reads `c.cfg`. Two projections of "the config" can drift — and if they ever do, the node that *founded* the network refuses its own genesis at the next restart. The failure is silent until a restart. |
| **A `Chain.ConsensusParams` method, both arms read it** | one small method | **Chosen.** Makes the drift unrepresentable rather than merely absent today. This is the owner's own standing point: *a proof that a property holds today is not a structure that makes violating it impossible.* |

### `Build`'s signature

| Option | Cost | Why not / why |
|---|---|---|
| `BuildWithParams(store, p)` beside `Build(store)` | zero churn | **Rejected.** It leaves the paramless call available and ergonomic, which is *exactly the shape the defect shipped in*. A future `silt init` would take it without anyone noticing. |
| **`Build(store, params *chain.ConsensusParams)`** | 14 files, 20 call sites, all mechanical `, nil` | **Chosen.** A paramless production genesis becomes something a caller has to *write `nil`* to ask for. One-time churn against a permanent structural property. |

### The scope of the widened gate

The brief asks for `node.Config` coverage. The question I had to answer was how much runtime driving
to buy, given 36 fields of which the bond verifier reads two.

I refused two extremes. **A pure closed-complement gate** would satisfy the letter of the ablation
condition and be decoration on the driven half — and the whole reason this work exists is that five
green gates were decoration. **A probe for all 36 fields** would require standing up transport, DHT
and repair machinery to prove that a fetch backoff does not change a validity verdict, which is
gold-plating a question nobody has evidence to ask.

What I built instead: probes for exactly the fields with a verdict path, a **pinned measured
divergence set**, and the other 31 reported **UNPROVEN — never "safe"**. That is simplicity rule 7
applied honestly in both directions: rule 7 forbids calling an undriven field safe; it does not
require driving every field.

### The bijection — the one thing I added beyond the brief

The brief asks for a rule plus exclusions, with the field count as output. A declaration table alone
makes the count an output *of one table*. It does not stop a `ConsensusParams` field existing that no
table claims, or two tables claiming the same one.

So each gate resolves its `carriedAs` strings against the real struct by reflection and names the
other's residue. This is one assertion per side, not a new mechanism — and it paid for itself
immediately by catching `MinBondBytes` claimed twice, which would have left one of the two knobs
unbound while both tables read as complete.

## What I deliberately did NOT build

- **The carrier bound and any `Atts` cap.** Separate changes, separately certified. Worth recording:
  the carrier bound's `|Anchors|` and `EpochBlocks` terms *depend on this wiring landing* — before
  it, neither quantity was committed anywhere, so a bound derived from them had nothing to read.
- **A `LogWarn` change to the foreign-genesis refusal.** The brief said it was still `LogDebug`. It
  is not: `82fe56d` already made it `LogWarn` with the flag list and a `ChainSyncForeignGenesis`
  stat. Verified with `git log -L` before deciding not to touch it. Building it again would have
  been a change with no mechanism behind it.

## The genesis-hash question, which the brief got half right

The brief says the genesis hash *will* move. Precisely: the **paramless** hash does **not** move
(`e44344ea…72c0` holds), because `Params` is a `nil` pointer with `omitempty`. What moves is a
**daemon-minted** genesis, which is now config-dependent and therefore has no single value to pin.

So `TestGenesisBlockHashIsPinned` needed **no re-pin at all** — and that is an assertion now, not an
accident: `G-CFGBIND-6b` re-states the paramless literal specifically to prove that adding the field
cost no committed fixture its identity. Beside it sits a new literal for a representative params set,
which is what catches a cbor renumbering — a change that would move every network's height-0 identity
while leaving "the hash moves with the params" perfectly green.

## Red-first, because a green gate with no demonstrated red is decoration

Thirteen ablations, each verified by exit code and each `diff`ed against its original before the
result was believed (a patch that silently no-ops reports GREEN and is indistinguishable from a
passing ablation). The table is in `docs/decisions.md`
`D-CFGBIND-MEMBERSHIP-RULE-2026-09-10`.

The one worth naming here is **N3b**. N3 (re-declare `BondLabelSamples` as local) went red via the
*structural* bijection check — which would have left the *driven* half unproven while the battery
read as complete. N3b re-runs the same defect with the tables made self-consistent, so only the
measurement can catch it. It went red on the measurement. **A gate whose two arms are never
separated cannot tell you which one is load-bearing** — the same lesson the F-2 blind review taught
the chain gate about `probedIn`.
