# 2026-08-28 — order-independence: the #506-gate family (the LAST orderVacuous family)

> **STATUS — SUPERSEDED by the resolution.** This is the finding-era deliberation
> that DISCOVERED the seam and routed it. The finding was certified and FIXED by
> canonicalization (not the reject predicted in the OUTCOME below). The shipped
> decision, the certified fix, and the delivered coverage live in
> `docs/thinking/2026-08-28-sameid-twoversion-canonicalize-apply.md`. Read that for
> what shipped; read this for how the seam was found. The forward-looking "likely
> resolution" in the OUTCOME was WRONG in direction — see the correction note there.

## The goal

Cover the last two `orderVacuous` committedSet fields — `gateLockedIn`,
`gateHeight` — under the order-independence model-check oracle, and settle
whether `regVersion` (the tally input the gate lock-in reads) is order-INDEPENDENT
or order-DEPENDENT. The snapshot-equivalence side (`probeUncovered`) owes
`bondRegHeight`, `regVersion`, `bondDomain`, `gateLockedIn`, `gateHeight` — I cover
what the two-list union rule lets me cover once the order side is settled.

The #620 PE review carried a LOADED WARNING: `regVersion`'s weight tally in
`rotateEpoch` (`chain.go:2922-2934`) is "genuinely order-exposed — which versions
sit in the frozen set at the epoch boundary CAN depend on arrival order." I must
NOT inherit #620's convergent-by-construction assumption. Treat a #618-style
order-DEPENDENCE finding as the EXPECTED outcome. Find the truth; do not produce a
green.

## ★★★ OUTCOME — CONSENSUS FINDING (routed here, since CERTIFIED + FIXED by canonicalization)

> **Resolution (added post-certification):** the finding below was routed to the
> Researcher + human, CERTIFIED, and FIXED by CANONICALIZATION — fold same-id
> intra-block regs to one content-chosen winner in `apply()`. The "likely certified
> resolution" this section predicted (make the `seenReg` guard unconditional, i.e.
> REJECT) was REFUTED: reject breaks the legal F1 renew/resize
> (`TestSameRootSameIDRenewAdmitted`). The fields below are now COVERED on the
> order-independence list. See
> `docs/thinking/2026-08-28-sameid-twoversion-canonicalize-apply.md`. The text below
> is preserved as the finding-era analysis that led to the route.

The warning was RIGHT, and the seam is real. `regVersion` (and `bondDomain`, the
same "latest-wins" slot) is ORDER-DEPENDENT on **intra-block BondReg slice order**
for a single id carrying two regs on its own root. Both orderings are ADMISSIBLE
pre-gate (the same-id-twice guard is gate-gated and the #618 seenRoot guard only
covers DISTINCT-id-same-root), so two honest replicas applying the identical block
in a different `BondReg` order commit different `regVersion`/`bondDomain` — a
different history-independent SMT root — a #618-class fork.

- Repro: `TestRegVersionIntraBlockOrderFinding`
  (`core/chain/modelcheck_regversion_intrablock_finding_test.go`). Passes under
  `-race`; asserts the observed divergence so the suite stays green and the test
  FLIPS when the certified fix lands.
- Exact divergence: `regVersion[v]` = 3 (v3-last) vs 2 (v3-first); `bondDomain[v]`
  = 0x22 vs 0x11. Only the two-version validator's fields differ; governors match.
- STOP per the research gate: this is a consensus-rule / validity-layer change,
  above the Builder seat. NO rule touched. Routed to Researcher + human.
- `gateLockedIn`/`gateHeight` therefore STAY in `orderVacuous` — they cannot be
  covered while their tally input (`regVersion`) is order-dependent. The
  snapshot-side `probeUncovered` entries stay too. Covering them now would either
  paper over the finding or require the very rule change the gate forbids me.
- Likely certified resolution (Researcher's call, not mine): make the
  same-id-twice-in-one-block guard UNCONDITIONAL (drop the `gate` gating on
  `seenReg`, chain.go:1463-1467/1493-1496), mirroring #618's unconditional
  `seenRoot`, so the divergent input never commits. Then the two fields become
  order-independent-by-rejection and this test converts to oracle coverage.

Everything below is the analysis that led here, preserved for the route.

## The mechanism, read from source (not memory)

The #506 gate is DERIVED state. Its two committed fields:

- `gateLockedIn bool`, `gateHeight uint64` (`chain.go:885-886`).
- Set ONLY in `rotateEpoch` (`chain.go:2922-2934`), post-latch path, when
  `RegGateActivationHeight==0 && EpochBlocks>0`.

The lock-in tally (`chain.go:2922-2934`):

```go
if !c.gateLockedIn && c.cfg.RegGateActivationHeight == 0 && c.cfg.EpochBlocks > 0 {
    var total, ready int64
    for id, w := range set {               // set = liveQualifiedSet() = the frozen epochSet
        total += w
        if c.regVersion[id] >= BlockVersionRegGate {   // BlockVersionRegGate = 3
            ready += w
        }
    }
    if total > 0 && 3*ready > 2*total {    // the SAME >⅔ weight bar the finality rule uses
        c.gateLockedIn = true
        c.gateHeight = h + c.cfg.EpochBlocks
    }
}
```

`rotateEpoch` runs LAST in `apply` (`chain.go:2891-2893`), after this block's
bonds, TTL expiries, slashes, and the maturity latch. So the tally reads the
CONVERGED post-block `bonded`/`slashed`/`regVersion` maps.

`regVersion[id]` is written in `apply`'s bond-reg loop (`chain.go:2842`):
`c.regVersion[id] = r.Version` — "latest committed reg governs." The loop iterates
`b.BondRegs` in SLICE ORDER. So within ONE block, if a single id carries two regs
with different Version, the LAST in slice order wins — an intra-block order
exposure of `regVersion[id]`.

### Where the loaded warning bites — and where the validity layer already closed it

The #620 warning is about `regVersion` in the tally. I decompose the exposure:

1. **Across DISTINCT ids.** Each id's `regVersion` is a function of that id's own
   latest committed reg. The tally sums independent per-id contributions into
   `ready`/`total`. Reordering regs of DIFFERENT ids within a block, or across
   blocks that all land before the boundary, does not change any single id's
   *latest* version — the last-writer for id X is still X's highest-height (then
   last-in-block) reg regardless of how X's reg sits relative to Y's. This leg is
   convergent because each id's map entry is independent and the final value is
   the latest reg — NOT "by construction of the tally."

2. **Same id, two DIFFERENT-version regs in ONE block.** THIS is the genuine
   order exposure: `regVersion[id]` = last-in-slice-order. `bondReg X{v=2}` then
   `bondReg X{v=3}` leaves `regVersion[X]=3`; the reverse leaves `regVersion[X]=2`.
   That flips whether X counts toward `ready`, which can flip `3*ready > 2*total`,
   which flips `gateLockedIn`/`gateHeight`. A real fork.

   BUT: `validateBondRegs` (`chain.go:1489-1496`) rejects a same-id-twice block
   with `ErrRegGate` — **only when `gate` is active** (`seenReg` is gate-gated,
   `chain.go:1464-1467`). The gate is active only AFTER lock-in
   (`regGateActive`, `chain.go:2945-2949`). So at the FIRST lock-in boundary, the
   gate is NOT yet active, and a same-id-two-version block is ADMISSIBLE on the
   validated path. This is the seam the warning points at.

3. **Same id, two different-version regs across DIFFERENT heights (blocks).**
   Both admissible pre-gate. The LATER height's reg governs (last write wins,
   height-ordered — apply processes blocks in height order, which the chain fixes).
   Reordering WITHIN a fixture means reordering which version lands at which
   height. If ordering-A puts v=3 at the LAST pre-boundary height for X and
   ordering-B puts v=2 there, `regVersion[X]` differs at the tally → possible gate
   flip. This is order-DEPENDENCE too, but here "order" means "which version a
   validator last signalled," which is a genuine state difference, not two
   spellings of the same state.

### The critical distinction the oracle must respect (the #618 vs #620 fork)

The oracle's contract: two orderings that reach the **same logical final state**
must reach byte-identical committed state. "Same logical final state" for
`regVersion` means every id ends on the SAME latest-version. If I build two
orderings where id X ends on v=3 in one and v=2 in the other, those are NOT the
same logical state — that is a fixture bug (a false finding), not a consensus
finding. The #618 finding was real because two orderings of the SAME set of
events (same distinct-id regs on the same root) diverged. To make an HONEST
`regVersion` stress, both orderings must commit the SAME multiset of (id, version)
signals reaching the SAME latest-per-id, varying only the ARRIVAL ORDER.

The honest stress for leg 2 (the seam): commit, in ONE pre-gate block, two regs
for the SAME id with DIFFERENT versions, in opposite slice orders across the two
orderings. Same multiset, same block, same height — only slice order differs. If
`regVersion[X]` (and thus the gate) diverges, THAT is a #618-class consensus
finding: an admissible block whose committed state depends on intra-block slice
order. STOP-and-escalate.

## The plan for the stressing fixture (`gateOrderings`)

A NEW fixture, distinct from `twoOrderings` (launch-anchor, MatureValidators=99,
gate never locks) and `matureOrderings` (matures but no versioned regs / no gate).

Regime to lock the gate in the post-latch path:
- `RegGateActivationHeight = 0` (force the derived lock-in path, not the override).
- `EpochBlocks > 0` (say 2), `MatureValidators` small so the latch trips on a real
  bonded set.
- Anchorless objective world (like `matureOrderings`), governors bond at genesis
  carrying `Version = BlockVersionRegGate (3)` so `ready` clears >⅔ and the gate
  locks at the first post-latch boundary.

The order variable — the SHARPEST stress that keeps the multiset identical:

- **Stress (distinct-id versioned regs around the boundary, arrival order).**
  Governors carry v=3 signals that arrive at DIFFERENT pre-boundary heights across
  the two orderings, keeping the latest-per-id identical at the tally height. Both
  orderings must lock the gate at the SAME height with the SAME `gateLockedIn`.
  If they diverge on same-multiset input, that is a #618-class finding.

I assert the direct gate outcome (`gateLockedIn`, `gateHeight` byte-identical) via
the oracle, PLUS a targeted `TestGateLockInIsOrderIndependent` trip-wire that
asserts the gate ACTUALLY LOCKED (not vacuous) and names a divergence as a finding
to route.

## Discipline

- ABLATION-FIRST for `gateLockedIn`/`gateHeight`: inject an order-dependence
  defect, paste the RED naming the field, revert, green.
- Test/fixture-only. If a genuine same-multiset divergence appears, STOP — do not
  touch `rotateEpoch`, the tally, `MatureCoefficient`, the weight sum, the quorum
  threshold, or any validity predicate. Report the two orderings + the exact
  differing values. Route to Researcher + human.
- Snapshot side: cover `gateLockedIn`/`gateHeight` (and `regVersion`/`bondRegHeight`
  where the gate-active world now populates them) with leave-one-out probes,
  remove from `probeUncovered` only where a probe genuinely flips.
- State the safety mechanism PLAINLY per covered field; do not overclaim (#620
  lesson: "convergent-by-construction" was too strong for a per-id latest-wins map;
  the honest claim is "per-id independent latest-write, same multiset ⇒ same map").
