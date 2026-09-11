# Owner call A — the era-4 consensus signature preimage (height + chain id)

**Date:** 2026-09-11
**Seat:** Builder
**Certification (binding, verdict GATED):**
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/V5-SIGNATURE-PREIMAGE-HEIGHT-DELTA-RESEARCH-CERTIFICATION-2026-09-10.md`
**Status:** BUILT AND HELD. Not merged. This is a FORMAT item; the owner reviews each one
individually.

## 1. The mechanism, before the fix

The failure is X because Y; this change addresses Y by Z.

- **X (the failure).** An accuser can convict an honest validator by relabelling the height of a
  genuine signature (I5 / R0.6), and can convict an honest validator who runs one identity key on
  two silt networks.
- **Y (the mechanism).** The era-2 signed payload is `domain ‖ phase ‖ round ‖ hash`
  (`consensusSigBytes`). The height is not in it — it "rides inside the hash", an indirection
  `Block.Pruned` severs, which is exactly what `CheckEquivocation`'s own comment says its height
  check depends on. And `consensusSigDomain` is a *constant across every silt network*: a domain
  tag separates message KINDS, not NETWORKS.
- **Z (the change).** era 4 signs `consensusSigBytesV5` — 99 bytes, fixed-width:

  ```
  "silt/consensus/v5\x00"(18) ‖ chainID(32) ‖ phase(1) ‖ height(8 LE) ‖ round(8 LE) ‖ hash(32)
  ```

  Height inside the message makes the relabel inexpressible: at most one of two signatures verifies
  at one declared height. The chain id makes a signature released on network X unreadable as a vote
  on network Y.

**Bought on the scar, not on an optimisation.** Dropping `Height` is the SECOND occurrence of the
#397 watermark scar (`docs/build-process.md` rule 6) — the same `(height, round, step)` schema
family, the same dropped field, and the dropped field is again the thing that broke. The sharpest
statement of it is silt's own: the durable watermark `ports.SignMark` already carried
`(height, round, step)` while the signature preimage carried `(round, step)`. **Two halves of one
mechanism disagreed about the slot, inside one codebase.**

## 2. The options, and why this shape

| Option | Cost | Verdict |
|---|---|---|
| Leave the preimage; bound evidence some other way | Nothing new to build | REFUTED by the cert: the height is the accuser's degree of freedom, and nothing else removes it |
| Add height only (what was originally bought) | One field | REFUTED at §4.3: the chain-id drop-claim is disprovable, and the construction is given |
| Add height + chain id, NEW domain tag, fixed-width, no length prefix | 40 more signed bytes; nine call sites re-scoped | **CERTIFIED.** Settled corner: CometBFT `CanonicalVote` (carries `ChainID`); Ethereum's `compute_fork_data_root` names the purpose verbatim |
| Length-prefix the fields, as CometBFT does | A prefix on every signature, forever | Not needed. CometBFT prefixes because protobuf is variable-length; a fixed-width concatenation is injective by construction. **Standing constraint recorded in the code: every future field must be fixed-width, or the layout needs a prefix from that day forward.** |

## 3. The two structural decisions this forced

### T-ERA-DISPATCH — the producer keys on the block, the verifier keys on the signature

`era4Active` is at-or-greater, so `H_era4` is the FIRST v5 height and **its parent is v4**.
`HeadCarrier` filters the parent's precommits with the v4 parent in scope (v2-form entries);
`validateCarrier` verifies those same entries with the v5 child in scope. Key that dispatch on the
CONTAINING block's `Version` and every entry fails — the honest proposer rejects its own block, at
one exact height, forever.

So: `AttestAt` keys the FORM on `b.Version` (it holds the block it signs); `verifyAtt` keys the
PREIMAGE on `a.Phase` (it often does not hold the signed object — `validateCarrier` is a pure
function of the child). `AttPhase(version, step)` is the ONE era→form mapping, so the producer and
every phase-exactness check cannot drift. Two new wire constants, `PhasePrepareV5 = 3` and
`PhasePrecommitV5 = 4`; `Attestation.Phase` is already a `uint8` on the wire, so this is additive.

Consequence: `validateCarrier` and `HeadCarrier` accept BOTH precommit forms, through one shared
predicate `isCarrierPrecommit`. Capability-neutral by the cert's §5.3 lemma. Residual
`R-CARRIER-OFFFORM-SEAT` stands.

### T-STEP-VS-FORM — the watermark records the STEP, the attestation records the FORM

`ports.SignMark` is fsynced to disk before any signature is released — it is silt's copy of
Tendermint's `priv_validator_state`, i.e. the #397 artifact itself — and `slotCompare` orders it
NUMERICALLY. A mark written `(H, r, PhasePrecommit = 2)` and probed by a new binary with
`PhasePrepareV5 = 3` compares `3 > 2`, is NOT blocked, and the node signs a different block at a
height it already precommitted. **A self-manufactured double-sign produced by the upgrade itself.**

The fix is discipline, not migration: `recordSign` / `recordSignLock` / `signAllowedAt` /
`slotCompare` keep taking the canonical step at every era. The durable encoding is byte-identical
across the upgrade. `AttestAt` does the mapping, so no caller has to remember.

The rule is held by a SOURCE gate (`TestGPRE3_NoEra4ConstantReachesTheWatermark`), not a comment: a
runtime gate can only observe the sites it happens to drive, and a wedge at an undriven site is how
the h43 drain-slot gate came to be missing.

## 4. One thing the certification did not rule on, and the call made

**The slash slot is `(height, round, STEP)`, not `(height, round, wire phase)`.**

Taking the wire phase would have made a validator that precommitted a v4 block and a v5 block at one
`(height, round)` unslashable — an accountability regression the change did not buy. The golden
corpus caught it: case `v4-vs-v5-both-rounds-era-same-slot-ACCEPT` flipped to `ErrNotEquivocation`.

`sigScope` therefore records the canonical step (`canonicalStep` inverts `AttPhase`). This is
T-STEP-VS-FORM applied to the slash rule, and it is the same argument: the watermark records the
step and is era-independent, so an honest validator releases exactly one signature per
`(height, round, step)` ACROSS the boundary. If the slash rule recorded the form instead, the two
halves would disagree about the slot again.

**This preserves the shipped verdict rather than changing it** — 26/26 golden verdicts identical
before and after — which is why the builder took it rather than routing it. It is flagged for the
Researcher all the same.

## 5. What is NOT in this branch, stated so it cannot be inferred

- **The `EquivV5` fixed-size evidence object (route C).** Owner call A changes the signature
  PREIMAGE. The cert's C1 (exclusivity), C2 (phase whitelist), C3 (canonical ordering) and its
  ~251 B figure are all properties of that object, which does not exist here. `G-PRE-9` measures
  what exists (preimage = **99 B measured**; a minimal body-form v5 proof = **1,041 B measured**)
  and says plainly which figure it cannot measure and why.
- **`SlashesBytesCap`.** Untouched, per owner condition 3. No PR may move the cap and the preimage
  together.
- **`ErrPrunedEvidence` and the `bodyHash` recompute.** Kept verbatim. They are narrowed in scope
  to the body form, not made redundant: era-1/2/3 signatures bind no height and every height below
  `H_era4` exists forever. Named as a DON'T in `CheckEquivocation`'s doc comment.

## 6. Open, routed, not decided here

**`BoxConfig.ChainID` — the BG-2 condition.** The certification asked for the box's chain id to be
"box-owned, derived from its own trust anchor, never driver-declared", and said that if the box
cannot derive it from an anchor it already holds, that is a genuine blocker that comes back.

**It cannot.** The parent block does not carry the genesis hash, and `ch` is the box's
config-bearing chain, not a state source — on the deployment target it holds no blocks, so
`ch.ChainID()` is the zero hash. Reading it off `ch` is also the exact thing the fold-file pin
denies by name (`blocks`: "applied-history chain state — a cold box never has it"). The existing
`TestFoldFilesReadNoLiveBoxState` reddened on the first attempt, which is the evidence.

So it is `BoxConfig.ChainID`: operator configuration, set once at construction, refused when zero,
threaded `BoxConfig.ChainID → HeadRef.ChainID → assembleStateRootRecomputeOps` exactly as
`parentProposer` already is. A wrong value can only make the box refuse MORE (every era-4 signature
fails to verify ⇒ stall); there is no value that makes an invalid block valid, so
`box.Accept ⇒ node.Accept` is preserved. The cold-auditor knob pin was widened from a field COUNT
to a NAMED anchor list carrying that direction argument per field, so a future field cannot ride in
on an arithmetic edit.

**This is the builder's reading of "box-owned". It is routed for confirmation, not settled.**
