# 2026-08-28 — CONSENSUS-RULE: canonicalize same-id intra-block bond regs in apply()

## The decision

Fold each block's `BondRegs` to ONE canonical winner per validator id before the
`apply()` bond loop, chosen by a TOTAL ORDER on content — **largest `Size`, then
`Version`, then `Domain`, then `Sig`** — and apply ALL of that winner's fields. The
committed `regVersion`/`bondDomain`/`bonded` (and the ownership writes) become a pure
function of block content, identical across every intra-block slice order.

This is the certified fix for the finding routed by #622. It is CERTIFIED
(`silt-reviews/research/research-outcome/sameid-twoversion-intrablock-bondreg-contention-RESEARCH-CERTIFICATION-2026-08-28.md`)
and human-ratified (Andrew: canonicalize).

## The finding (what #622 routed)

A block carrying two BondRegs for the SAME validator id (distinct
`Version`/`Domain`/`Size`; any roots), pre-#506-gate, is ADMITTED. `apply()`
(`core/chain/chain.go`) resolved `regVersion`/`bondDomain`/`bonded` LAST-WRITER-WINS
by slice position, so two honest replicas applying the identical block in a different
`BondReg` order committed different state → a different history-independent SMT root
(a #618-class latent fork). `regVersion` feeds the #506 lock-in tally (`rotateEpoch`),
so `gateLockedIn`/`gateHeight` inherited the split when the two-version validator was
the >⅔ ready-weight swing.

Why the current guards let it through (cert §1):
- The ownership-dedup branch (`owner != id`) is FALSE for same-id.
- `seenRoot` (the #618 guard) exempts same-id by construction (`prev != id`).
- `seenReg` is gate-gated — off pre-lock, which is exactly the window in which the
  gate is being TALLIED.

## Why canonicalize, not reject

REJECT was REFUTED (cert §3(b)). The same-id-twice block has a LEGITIMATE form: the
F1 renew/resize, pinned by the shipped negative control `TestSameRootSameIDRenewAdmitted`
(a validator re-registering / resizing its OWN root twice in one block MUST be admitted,
`bonded == minBond*2`). Making `seenReg` unconditional would reject that certified-legal
block. This is the OPPOSITE of #618: distinct-id same-root has NO legitimate form, so
#618 correctly rejected; same-id-twice DOES, so it is canonicalized. The two findings are
siblings in shape (intra-block bond-reg order-dependence feeding the SMT freeze) and
opposite in resolution, decided by whether the contended block class has a legitimate form
(cert §3(c), literature: SMR block-non-determinism — reject when content has no legitimate
form, agree-on-canonical-resolution when it does).

## The total order, and why "largest Size" first

`bondRegLess`: largest `Size`, then `Version`, then `Domain`, then `Sig` bytes. All four
keys are content, so the winner is a pure function of the reg, not slice position. `Sig`
is the last key, so the order is TOTAL — two regs identical in `Size`/`Version`/`Domain`
still have a deterministic winner; there is never a tie the slice order could break.

"Largest size wins" first makes a resize MONOTONE: the intended larger registration takes
regardless of which order the proposer listed the regs. That is the right renew/resize
semantics and it keeps `TestSameRootSameIDRenewAdmitted` green — reg2's `2S` wins in BOTH
orders, which is exactly the outcome that test already asserts (cert §3(a), R3).

## The one place I did NOT guess — and STOPPED

The first implementation returned the per-id winners SORTED BY ID. That made the
DISTINCT-ID same-root case order-independent too, as a side effect — which broke the
named residual R-G premise `TestGenesisSameRootApplyIsOrderDependent` (genesis apply()
is intentionally order-dependent for two distinct-ID unproven same-root regs, safe by
genesis byte-identity, NOT by a guard). Making genesis validity order-independent is a
DIFFERENT, research-gated consensus-rule change (option (b) of
`docs/thinking/2026-08-28-genesis-sameroot-residual.md`).

The cert scoped THIS change to the SAME-ID case only. So I corrected the fold to preserve
FIRST-APPEARANCE order of distinct ids (dedup same-id in place, never re-order distinct
ids). The distinct-id ownership-branch behavior is now byte-for-byte identical to the
pre-fold loop; only same-id multi-reg is collapsed. `TestGenesisSameRootApplyIsOrderDependent`
stays green (order-dependent, as intended). This was a STOP-and-check, not a relax: I did
not touch the premise's assertion.

## Coverage (the two-list union rule)

**Order-independence oracle** (`modelcheck_order_independence_test.go`):
- `TestRegVersionIntraBlockOrderIndependent` — the covering probe. Two same-id regs with
  distinct `Version`/`Domain`/`Size` in one pre-gate block; asserts byte-identical
  `regVersion`/`bondDomain`/`bonded` across both orderings, AND that the winner is the
  largest-Size reg (pins the total order, not just convergence). RED without the fold,
  GREEN with it (ablation-proven).
- `gateSwingOrderings` + `TestGateLockInSwingIsOrderIndependent` — the #506 tally-swing
  fixture. A same-id two-version validator is the EXACT >⅔ ready-weight swing (three equal
  weights, two ready + the swing; proposer rotates so all three are SEEN and the network
  matures). Before the fix, the slice order flips the swing's committed version, hence
  whether the gate locks — `gateLockedIn`/`gateHeight` fork. After, they lock identically.
- `regVersion`, `bondDomain`, `gateLockedIn`, `gateHeight` moved OUT of `orderVacuous`
  (now empty for these). The main oracle checks them via a `gateFields`/`worldOf` routing,
  same union-of-worlds pattern as `matureFields`.
- Corrected the `twoOrderings` over-claim (cert "What I corrected"): its regVersion/bondDomain
  green is distinct-id disjoint-root; it covers NOTHING for the same-id seam. The doc comment
  now names the gap and points at the same-id probe + swing fixture.

**Leave-one-out oracle** (`probeUncovered`, `modelcheck_snapshot_equivalence_test.go`):
`regVersion`/`bondDomain`/`gateLockedIn`/`gateHeight` remain listed. Their ORDER-independence
is now covered on the other list; a leave-one-out (verdict-flip) probe for the gate fields
needs a gate-locked replica plus a nonce-valid post-H_act block whose window check passes on
the ablated path — a full block history a history-less snapshot replica cannot supply cleanly.
`bondDomain` is a metric (C2Metric), not a validity predicate. Notes corrected to say so; not
gold-plated with a fragile probe the cert does not require.

## Which fields clear which list (report)

| Field | order-independence (orderVacuous) | leave-one-out (probeUncovered) |
|---|---|---|
| `regVersion`  | CLEARED (same-id probe + swing) | still listed (verdict-flip probe owed) |
| `bondDomain`  | CLEARED (same-id probe + swing) | still listed (metric, not a predicate) |
| `gateLockedIn`| CLEARED (swing fixture)         | still listed (needs nonce-valid post-H_act block) |
| `gateHeight`  | CLEARED (swing fixture)         | still listed (same) |

None clear BOTH lists yet; all four now clear the order-independence list, which is the list
the cert's gate-lifting evidence names (cert §Verdict "Evidence that lifts the gate").

## Blast radius

`go test ./core/... ./sim/...` green; the model-check tier green under `-race`. Negative
control `TestSameRootSameIDRenewAdmitted` green. Residual R-G premise green (order-dependent,
preserved). No change to the weight sum, `epochSet` freeze, quorum threshold, or
`MatureCoefficient` — only the same-id apply resolution.
