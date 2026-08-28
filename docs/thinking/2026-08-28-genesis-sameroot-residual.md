# Genesis same-root bond-reg residual (R-G) — name the premise or extend the guard

Date: 2026-08-28
Seat: Builder
Context: PR #618 shipped the certified `seenRoot` per-root distinct-ID dedup in
`validateBondRegs`. The PE review recorded one residual (R-G) to close or record
before the era-3 format freeze.
PE ruling: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-618-updated-sameroot-dedup-fix-2026-08-28.md`
Certification (parent): `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/same-root-intrablock-bondreg-contention-RESEARCH-CERTIFICATION-2026-08-28.md`

## The residual in one line

`AppendGenesis` (`core/chain/chain.go:2730`) goes straight to `c.apply(b)` (2768)
and never runs `validateBondRegs`, so the new `seenRoot` dedup does not cover
genesis. Genesis `apply()` IS order-dependent for two distinct-ID **unproven**
same-root bond regs. It is safe TODAY by an EXTERNAL invariant — genesis is a
byte-identical shared constant — not by the guard.

## Step 1 — the facts, with evidence

### Fact 1: genesis apply() is order-dependent for two distinct-ID unproven same-root regs — CONFIRMED by execution

I constructed the case and ran it (scratch test, since removed). A genesis with two
distinct-ID unproven regs on one shared root:

- slice order `[A,B]` → `bondRootOwner[shared]=A`, `bonded{A:2MiB, B:0}`
- slice order `[B,A]` → `bondRootOwner[shared]=B`, `bonded{A:0, B:2MiB}`

Byte-different committed state from an identical genesis, chosen only by slice order.

Mechanism (`chain.go:2800-2831`): `proven = b.Height > 0`, so genesis regs are
`proven=false`. The per-root dedup at 2812 (`owner != id`) admits the FIRST reg on an
unclaimed root and rejects later distinct-ID regs on the now-claimed root
(`continue` at 2819, because the G3 displacement branch at 2818 requires
`proven && !bondRootProven[root]`, and `proven` is false at genesis). "First wins"
over a proposer-ordered slice is order-dependent by definition. This is the same
order-dependence #618 rejected at the validity layer for height>0 blocks; genesis
never reaches that layer.

### Fact 2: genesis is a byte-identical, non-attacker-influenceable shared constant — CONFIRMED by reading the source

- `genesis.Build` (`core/genesis/genesis.go:69-82`) constructs the block
  deterministically from `//go:embed manifesto.txt` (compiled into the binary,
  `genesis.go:36-37`), signed by a fixed key (`genesis.go:43-46`). Every node runs
  the same `Build` on the same embedded bytes → identical block, identical hash.
- The production genesis carries `Entries` ONLY: `chain.Block{Version, Height:0,
  Entries: []ports.Entry{entry}}` (`genesis.go:79`). **`BondRegs` is empty.** The
  order-dependent path is not exercised by the real genesis at all.
- All three non-test callers use `genesis.Build`: `cmd/silt/daemon.go:733`,
  `sim/consensus.go:90`, `sim/bondstanding.go:95`. Only test helpers
  (`orderWorld`, `TestBondedOrderFreeUnderSlashInteraction`) construct a genesis
  carrying `BondRegs`.

So the residual is safe today because (i) genesis is identical on every node, so
there is no per-node choice of slice order to diverge on, and (ii) the real genesis
carries no BondRegs anyway. Both are EXTERNAL to `apply()` and to the dedup. The
PE ruling states exactly this at lines 80-89.

### Reachability of the residual

Not reachable in production today. It becomes reachable only if a future change:

- lets genesis carry `BondRegs` that differ per node or are attacker-influenced
  (e.g. a config-driven or downloaded genesis), OR
- re-derives / re-canonicalizes / re-sorts genesis regs (a genesis builder that
  sorts, a snapshot that reconstructs genesis state, a multi-genesis harness).

The era-3 freeze property is unconditional: "for any block B, `apply(B)` produces
the same state regardless of `B.BondRegs` order." Genesis is a block B that does not
satisfy this in isolation. The freeze reasoning silently leans on the byte-identity
premise. That is the coupling to close or name.

## Step 2 — the two options

### Option (a): NAME the premise (assertion + test + docs). NO genesis validity change.

Encode, as an explicit named invariant, that genesis `apply()` safety for same-root
regs depends on genesis being the byte-identical shared constant. Concretely:

- A regression test in `core/chain/` that DOCUMENTS and pins the coupling: it
  constructs a genesis with two distinct-ID unproven same-root regs and asserts the
  observed order-dependence (owner differs by slice order). This makes the premise a
  named, executable fact: if a future change ever makes genesis `apply()`
  order-INDEPENDENT for this case (e.g. someone adds a dedup to the genesis path),
  the test flips and forces a conscious re-derivation. If instead someone starts
  letting genesis carry per-node BondRegs, the coupling is right there, named, with
  the reachability spelled out.
- The premise is stated where the freeze gate will inherit it: a named comment/anchor
  at `AppendGenesis` and in the freeze-relevant docs, so the era-3 freeze gate
  inherits a NAMED check rather than an unstated assumption.

Cost: one test + a doc/comment. No change to what genesis accepts. Fully inside the
already-certified envelope — the certification's scope was the proposer-chosen-order
threat, and this changes no validity rule.

Benefit: the freeze's order-independence claim stops silently depending on an
unwritten premise. The premise cannot break without tripping a test. The freeze gate
gets a concrete artifact to point at.

Limit: this does NOT make genesis order-independent. If genesis ever legitimately
needs to carry per-node BondRegs, option (b) is still required then. Option (a) makes
that a loud, conscious decision instead of a silent regression.

### Option (b): EXTEND the guard into the genesis path. CHANGES genesis validity.

Apply the same-root distinct-ID dedup (reject, `ErrSharedRootInBlock`) to the genesis
apply/validation path, so genesis is order-independent BY THE GUARD for every block
class without leaning on byte-identity.

Cost / why this is gated: this CHANGES what genesis accepts. Today genesis validity is
deliberately minimal — genesis is un-validated / a trusted constant ("declared, not
agreed", `chain.go:2724-2727`), and `AppendGenesis` deliberately does NOT run
`validateBondRegs`, `validateTakedowns`, or `validateSlashes`. Adding a rejection into
the genesis path is a consensus-rule change to genesis validity: it changes the set of
genesis blocks a node accepts. That surface is research-gated (consensus-rule /
block-validity) and human-ratified. It also risks interacting with the deliberate
genesis carve-outs (a rejection at genesis that was previously an accept).

Benefit: closes R-G durably for every future genesis shape, byte-identical or not.

## Recommendation: Option (a)

Recommend (a): name the premise with a pinned regression test + a named anchor at
`AppendGenesis`, no change to genesis validity.

Rationale:

1. **Reachability is zero today and the real fix is not needed yet.** Production
   genesis carries no BondRegs and is byte-identical. There is no live divergence to
   fix — the residual is a FUTURE coupling, not a present bug. Shipping a consensus
   validity change now to close a not-yet-reachable path is gold-plating the moment
   does not need, and it burns the research + ratification budget on a hypothetical.

2. **The certified envelope covers (a), not (b).** The certification scoped the
   proposer-chosen-order threat; #618 closed it. A named premise/test changes no
   validity rule, so it ships inside the certified envelope. (b) is a new
   consensus-rule change and must be routed.

3. **(a) buys exactly what the freeze needs: the premise stops being silent.** The one
   thing the PE flagged is that the freeze's order-independence claim "is whole only if
   you ALSO hold 'genesis regs are never re-ordered after canonicalization.'" (a) makes
   that hold-condition an executable, named artifact the freeze gate inherits. That is
   the whole ask.

4. **(a) does not foreclose (b).** If a future era legitimately needs per-node genesis
   BondRegs, the named premise turns that into a loud, conscious "now you need the
   guard" decision — routed through research at the moment it is actually reachable,
   with the real requirement in hand, not speculatively.

The tension I am defending: the PE offered "close it OR record it." Closing it (b) is
the more thorough option and a reviewer could reasonably prefer it. I am pushing back on
doing (b) now because it is a consensus-rule change with zero present reachability — the
cost (research gate + ratification + risk to the deliberate genesis carve-outs) is not
traceable to any real failure today. (a) discharges the freeze's actual dependency at a
fraction of the cost and keeps (b) available for when it is reachable. If the Planner or
human wants genesis order-independence guaranteed by construction regardless of
reachability, that is a legitimate call — it is (b), and it must be routed to research.

## What ships under this recommendation

- A regression test in `core/chain/` pinning the genesis same-root order-dependence as
  a NAMED premise, with the reachability and the freeze coupling documented in-test.
  Ablation-first: the test asserts the premise (order-dependence observed); it flips
  RED if a future change silently alters genesis apply() for this case.
- A named anchor comment at `AppendGenesis` pointing at the premise + this doc, so the
  era-3 freeze gate inherits a named check.
- CHANGELOG line.
- No change to any genesis validity rule. No new rejection at genesis.
