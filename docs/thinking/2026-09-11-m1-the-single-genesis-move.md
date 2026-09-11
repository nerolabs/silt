# M1 — the single genesis move: the network's name, the era floor it commits, and two gates that contradicted each other

**Date:** 2026-09-11
**Seat:** Builder
**Branch:** `builder/m1-genesis-move`, based on `builder/owner-call-A-v5-preimage` @ `a67a8d4`
rebased onto `origin/main` @ `a28a5b5`.
**Binding certification:**
`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/NETWORK-IDENTITY-BINDING-THREE-LAYER-RESEARCH-CERTIFICATION-2026-09-11.md`
(all three layers GATED; §2.2 states the freeze read-set does NOT move, the genesis hash does).

## The frame, stated before anything else

There is no live network. The genesis exists only on the owner's machine. Era 4 is open and
fully malleable until the first RC. The freeze is a gate the team opens when the work is done,
not a deadline. Every cost below is priced in re-runs and calendar, never in permanence.

Say **era 4**. The code's `V5` identifiers are era 4's implementation. There is no era 5 here.

## Why one commit and not four

`Block.Hash()` covers `Params`, and `(*Chain).ChainID` is `blocks[0].Hash()`. Every change that
adds a committed field re-mints the genesis and re-runs the graded set. The certification's §4
landing order makes M1 the *only* genesis move: owner call A's preimage, owner call F's bind
(already merged), `ConsensusParams.NetworkName`, and the two era-activation daemon flags land
together or the re-run set is paid once per move.

---

## Part 1 — the two gates that could not both be right

### The finding, measured

The certification derived a contradiction between `G-PRE-6` and `G-PRE-7` from source and marked
it UNSETTLED-2, needing execution. I executed it. The probe:

- builds the victim's era-2-form precommit at (h=12, r=1) under chain id **Y**, and the same
  precommit under chain id **X**, and asserts they are **bit-identical**. They are. `AttPhase`
  returns `PhasePrecommit` for a sub-era-4 block, so `AttestAt` never enters the era-4 branch and
  never reads `chainID`;
- pairs that harvested leg with the victim's honest era-4-form precommit on network X;
- asserts `G-PRE-6`'s stated property — *"an honest validator running on two silt networks must
  NOT be slashable"*.

It **failed**. `CheckEquivocation` returns `nil`: convict. An honest validator that precommitted
once on network X is slashed on network X using bytes harvested from network Y. That is I5, the
#397 shape — the penalty hits the honest.

`G-PRE-7`'s fourth subtest demanded exactly that verdict, under the title *"a v2-form and a
v5-form signature at ONE slot DO convict"*, with a failure message calling the refusal an
**ACCOUNTABILITY REGRESSION**. It asserted an I5 violation as required behaviour.

### What the mechanism actually is

`sigScope` keys on `canonicalStep(phase)`, which collapses `PhasePrecommitV5` → `PhasePrecommit`
and `PhasePrepareV5` → `PhasePrepare`. On the slash path that collapse changes the verdict in
**exactly one case**: a same-round, same-step pair whose two legs carry *different wire forms*.
Every other pair — same form, or different steps — reaches the same verdict with or without it.

So the only thing `canonicalStep` buys on the slash path is the mixed-form convict, and the only
*reachable* instance of the mixed-form convict is the cross-network harvest. On a chain that
commits `Era4ActivationHeight = 1` there is no height above 0 at which an honest validator can
have produced an era-2-form precommit at all, so the on-network boundary double-signer the
subtest describes cannot exist there.

### Options

**(1) Land the era-floor rule now.** Refuse evidence whose block version is below
`(*Chain).MintVersion(h)`, at `validateSlashes`, `FindEquivocations` and `slashEquivocators` in
one commit. This is the certification's M2. It is a **consensus-rule change** — it widens block
invalidity — and the research gate says a builder does not decide one. It also needs a `*Chain`
threaded into `CheckEquivocation`, which M1 does not touch. **Rejected for M1: gated, and out of
this move's scope.**

**(2) Remove `canonicalStep` from `consensusSigScopes`.** Strictly narrowing (the accept set only
shrinks, so it can never manufacture a slash), needs no chain, no floor, no threading, and closes
the mixed-form face at every height. Tempting, and simpler than (1). But it is still a change to
`CheckEquivocation`'s accept set, therefore to `validateSlashes`, therefore to block validity —
a consensus-rule change, research-gated, and it would be a *second* mechanism competing with the
certified era floor. Simplicity rule 1: do not mint a second rule where a certified one is
already routed. **Rejected, and surfaced to the planner as an option M2 should price.**

**(3) Retire the demand; record the open face; machine-check the scope.** Change no production
code. Stop requiring the I5 violation, drive the fact that makes it reachable (the era-2 form is
chain-blind), record the current verdict as the OPEN face with a trip that goes RED the moment
M2 closes it, and bind both gates' era-2-form arms to an explicit, machine-checked pre-era-4
floor. **Taken.**

### What (3) ships

`G-PRE-7` subtest 4 is re-authored. It no longer demands a verdict. It:

1. drives the chain-blindness of the era-2-form leg (bit-identical under two chain ids) — the
   fact that makes the harvest possible;
2. asserts the fixture height is **at or above** the era-4 floor on an RC-shaped chain
   (`Era4ActivationHeight = 1`), and **below** it on a late-boundary chain — the floor read from
   `(*Chain).MintVersion` on a separately-built chain, never from the evidence blocks;
3. records the verdict today with a RED-on-close trip: if `CheckEquivocation` ever refuses the
   pair, the era floor has landed and the arm must be retired.

`G-PRE-6`'s ablation arm gains the same explicit floor witness and its log line stops calling
itself "the ablation". Under the era-floor rule that arm is only green at a pre-era-4 height, so
it must say which height it is talking about, and the gate must prove the height is in that
interval rather than assert it in prose. A claim about a gate is itself a claim.

**Neither gate's fixture builds a chain that mints the evidence blocks.** The floor witness is a
separate `*Chain` built only to answer `MintVersion(h)`; the evidence blocks are hand-built as
before. The guard condition does not come from the subject.

---

## Part 2 — `ConsensusParams.NetworkName`

**The owner's requirement:** a node reports its network by both a cryptographic identifier and a
canonical text name.

**Why not a flag.** `eradeclared.go` already ruled the vacuity: a number an operator can type
reports a belief, and whoever is debugging reads their own input back. A committed name is read
from the chain. The same argument, one field along.

**Why it is admissible.** cbor key 18 is free. The struct's *no `omitempty` on any field* rule
applies: the empty string is a MEANING (unnamed), not an absence, and the renderer narrates it.

**The display rule is load-bearing and is structural, not a policy sentence.** Collisions are not
preventable and are not meant to be: the hash is the identity, the name is a label. So the name
is never displayed without the tag. The device: there is **no accessor that returns the bare
name**. `(*Chain).NetworkIdentity()` is the only way out of the chain, and it always renders
both. A render site cannot print one without the other without reaching past the accessor.

**The sites.** The certification says three lines is seven sites. It is **nine**, and the two it
did not name are the ones a gate caught rather than a reader:

| # | Site | Symbol | Named by the cert |
|---|---|---|---|
| 1 | the config field | `chain.Config.NetworkName` | yes |
| 2 | the committed field, cbor 18 | `ConsensusParams.NetworkName` | yes |
| 3 | the projection | `ParamsFromConfig` | yes |
| 4 | the diagnosable refusal | `ConsensusParams.Diff` | yes |
| 5 | the closed-complement declaration | `configDecls` | yes |
| 6 | the machine-checked bijection | `configMemberships` | yes |
| 7 | the driven perturbation | `perturbConfig` | yes |
| 8 | the hash-coverage constructor | `setNonZero` (`hash_literal_pin_test.go`) — no `reflect.String` case, so G-CFGBIND-2 fails with *"no non-zero constructor for ConsensusParams.NetworkName"* | **no** |
| 9 | the report | `(*Chain).NetworkIdentity`, `NetworkIdentityLines`, the daemon flag and its wiring | **no** |

Site 8 is the good kind of missing: the gate refuses to guess, so it goes red rather than silently
skipping the new field's hash coverage.

**A tenth candidate was checked and correctly needed nothing.** `paramsExcluded`
(`consensusparams_test.go`) is a *second* membership table, and the obvious worry is that adding a
field to one table and not the other leaves a hole. It does not here: that table enumerates the
fields NOT carried, and `NetworkName` is carried, so `TestConsensusParamsMembershipIsComplete`
passes by reflection with no row. Recorded because "I checked it and it needed no edit" is a
different claim from "I did not think of it".

**And two gates outside the field itself caught real work.** `TestG_CFGBIND_6` refused the fixture
for leaving the new field at its ZERO value — the value-equality blind spot, working — and
`TestG_CFGBIND_6b` refused the moved genesis hash and demanded the re-pin be an explicit act. Both
are the mechanism doing its job on the first field added since it was built.

---

## Part 3 — the two activation flags

`Era3ActivationHeight` / `Era4ActivationHeight` are already committed at cbor keys 13/14 and ride
the refuse-to-start arm. `grep Era[34]ActivationHeight cmd/` returned nothing outside a test:
verified at source. Two daemon flags are the entire missing piece.

**Default 1/1.** The owner ratified starting the RC network at the highest era, and a Tester
confirmed it under the real launch regime. Three consequences, each deliberate:

1. **`rotateEpoch`'s era-4 tally never runs** — it is gated on `cfg.Era4ActivationHeight == 0`.
   There is no latch, so the boundary is a genesis constant: the maximally stable form.
2. **The sub-era-4 interval is empty above height 0.** This is the coupling the certification
   found and the advisory missed: the era-floor evidence rule (M2) is a *total* closure of the
   cross-network false slash **if and only if** the genesis commits `Era4ActivationHeight = 1`.
   The flag and the rule are one decision.
3. **A store whose genesis commits 0/0 now refuses to start** under the default. That is
   `CheckConsensusParams` working exactly as designed, and it costs nothing extra: key 18 moves
   the genesis hash anyway, so every pre-M1 store is already foreign to this build.

**A side effect worth naming.** `Diff` already emits `era3-activation-height` /
`era4-activation-height` as advice to the operator. Until now those named nothing an operator
could set — the same defect the `bond-vdf-delay` row documents in its own comment. The flags make
two of those lines actionable.

**The height-0 residual (certification UNSETTLED-1), settled.** With `Era4ActivationHeight = 1`,
`MintVersion(0)` is still 2, so a `(v2,v2)` pair *at height 0* stays admissible. Reachability:
`AttestAt` has five honest production call sites, all in `core/node/chainrole.go`'s consensus
round path (plus the `adversary.go` red-team harness). `cmd/silt/daemon.go` seeds genesis through
`AppendGenesis` and only then calls `nd.EnableChain`, so the node never holds an empty chain while
a round can run, and `Head()` never returns height 0 to a proposer. **Derived-unreachable on the
daemon path.** Named as still-live for M5: JOIN mode removes the local mint, and a joining node
holds no genesis for a real interval — the certification's §3.7 composition.

---

## Part 4 — the membership doctrine gains a second closed category

`NetworkName` is the first `ConsensusParams` member that reaches **no validity verdict** — and
"it reaches no verdict" is the recorded reason `Archive` is *excluded*. Shipping it under the
existing rule would make a machine-checked doctrine unfalsifiable and admit the next
non-consensus field by precedent instead of by argument.

**The owner's binding condition:** the second category must have a closed complement too.
*"Otherwise 'it's an identity field' becomes the escape hatch that admits anything, and we've
traded a testable doctrine for a rhetorical one."*

**The amended rule.** A `chain.Config` field is exactly one of:

- **(a) bound** — it can change a validity verdict, so it is bound to the chain;
- **(b) an identity property of the network** — it changes **no** validity verdict, and it **is**
  genesis-covered;
- **(c) excluded** — with a recorded reason.

**Why (b) is closed and not an escape hatch.** Both of its arms are machine-checked, and they
close from opposite sides:

- *it changes no validity verdict* is **measured**, not asserted: the field must carry a
  perturbation that actually ran, and it must diverge in **zero** regimes. A field that diverges
  anywhere is (a), and declaring it (b) is RED.
- *it is genesis-covered* is **resolved by reflection** against the real `ConsensusParams`. A
  field that is not carried is (c), and declaring it (b) is RED.

So (b) admits exactly the fields that are in the genesis hash and move no verdict. Such a field
has precisely one observable effect: it partitions networks and names them. That is what
"identity property of the network" means, and nothing else fits through.

The complements stay closed in the other direction too: (a) still requires a binding or an
unbound-residual row, (c) still requires `carriedAs == ""` plus a recorded reason, and a field
declared both carried and not-carried is still RED. The reflective gate over both structs is
untouched — a new field in `chain.Config` or in `ConsensusParams` still fails until someone
declares it. That is ablated, not asserted.

---

---

## The genesis hash, old and new

| Pin | Before | After |
|---|---|---|
| **paramless** genesis | `e44344eafa258c64904d88337e72ad7a904bd3c16a3058abb8d58557740272c0` | **unchanged** |
| **params-carrying** genesis | `4a305b96db986bb73ff571e93f1059bbafd47ce618da6ce0519869e05c8b9c46` | `b862f16b0978d8f563f6ce4d79e5eb5af904c6170d797b85605163a6c3c23b57` |

The paramless pin not moving is the proof that the *format* did not change: `Block.Params` is a
pointer with `omitempty`, so a genesis carrying no params still omits the key entirely and the
~250 `AppendGenesis` fixtures are untouched.

Two things moved the params-carrying pin, and they are one change: cbor key 18, and
`representativeParams` gaining a value for it — because the zero-value sweep in `G-CFGBIND-6`
refuses a fixture that leaves a committed field at zero. (For the record, key 18 alone with the
fixture still at zero gives `dce67c9a708c38f249ef9737e80ca36a7af8ed5f018f6a3689302de779614c99`.
That is a transient state of an incomplete fixture and is deliberately not pinned.)

## What M1 does NOT do, stated so nobody reads it as covered

- **The era-floor evidence rule.** M2. It is the only move that closes an I5 violation, and it
  must land with or before M1 reaches a graded run.
- **The FDH chain bindings and the zero-chain-id refusal.** M3.
- **ALPN, the LAN beacon tag, the DNS TXT prefix.** M4.
- **JOIN/START mode and the `ch.Len() == 0` audit.** M5.
- **`NetworkTag()` as a transport value.** M4. The name's companion here is the chain id itself;
  ALPN is a wire demux and authenticates nothing.
