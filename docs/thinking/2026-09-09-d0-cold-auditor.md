# D0 — the cold auditor: options and the decision (2026-09-09)

Lane D row D0. The Release Candidate's ONLY floor-box requirement. Owner call 2 of
`D-TRUE-UP-CALLS-2026-09-07`, on direction (a′) of
`R-membership-unbounded-sets-and-recovery-boundary-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03`
(Part 2, four conditions H-1…H-4, plus the advisory H-5). `D-RECOMPUTE-FREEZE` cut this row's
dependency on the recompute spine and made it the whole floor-box gate, under ten standing
simplicity rules — of which rule 7 (*a green gate with no demonstrated red is decoration*), rule 4
(no new `R-*` names) and rule 8 (add no concepts) govern here.

## What the row asks for

Six deliverables. Five are subtractions; the sixth is the one that makes the row real.

1. Unconditional loud stall at an ambiguous recovery boundary.
2. Delete `RecoveryDirective.Heights` and `LiveFollower`.
3. Refuse pruned blocks.
4. Take `trustFloor` off the contract surface.
5. Document the `-ws-checkpoint` re-anchor as irrecoverable-if-unreachable.
6. ONE DRIVEN never-Accept suite: for every v5 block class, forge it, commit the divergent root,
   assert a stall.

## The measurement this deliberation rests on

Before choosing anything I drove one v5 block of each class through `(*Box).Validate` on the
existing `structFixture` (four bonded anchors, objective, mature-from-genesis) and recorded the
verdict verbatim. Every class reached the door and produced a DISTINCT named stall; none bounced
early; none Accepted:

| class | node's own verdict | door verdict | the name it stalls on |
|---|---|---|---|
| entries (E) | accepts | INDETERMINATE | `ErrRecomputeGated` — reached the R1.8 downgrade |
| revocations (R) | accepts | INDETERMINATE | `tagRevLogSize` parent-log-size stall |
| un-revocations (R) | accepts | INDETERMINATE | `tagRevLogSize` parent-log-size stall |
| bond regs (B) | accepts | INDETERMINATE | no digest witness for touched `bondedRoot` |
| slashes (S) | accepts | INDETERMINATE | no digest witness for touched `slashedRoot` |
| issuer keys | accepts | INDETERMINATE | out-of-P1-a-scope, `issuerKeyCommit` named |
| carrier (`LastCommit`, A) | accepts | INDETERMINATE | no digest witness for `validatorsSeenRoot` |
| pruned | — | INDETERMINATE | `ErrPrunedBlockUnreproducible` |

(The un-revocations row is the CORRECTED one. The first probe named a root nothing had revoked, so
the node refused the block and the arm was measuring a malformed block rather than the class. The
shipped fixture commits a revocation at h2 first, and the per-arm node ORACLE — which asserts the
node's own verdict before the door is driven — is what forces that: an arm whose block the node
rejects cannot quietly stand in for the class it claims.)

That table is the reason the suite is worth writing and the reason it is cheap: the classes are
already distinguishable at the door, so a table-driven suite over them is not decoration — each
arm exercises a different refusal with a different name.

## The decisions

### (1) The unconditional stall, and what happens to the knob

`recoveryBoundaryDecision` had three ways past an ambiguous boundary: a height in
`RecoveryDirective.Heights`, `LiveFollower`, and the fall-through. (a′) removes all three, so the
function collapses to the boundary predicate itself.

- **Chosen:** `recoveryBoundaryDecision(h)` returns `(false, ErrRecoveryBoundaryStall)` iff
  `isAmbiguousRecoveryBoundary(h)`. The `RecoveryDirective` type, `hasDirective`, `BoxConfig.Recovery`
  and `Box.recovery` are deleted outright, and `WitnessValidateV5` loses its third parameter.
- **CORRECTED after the blind PE review (B-1,
  `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-d0-cold-auditor-3539b2a-2026-09-09.md`):
  the height that decision reads is the BOX'S OWN (`head.NextHeight`), never `b.Height`.** The first
  cut deleted the knob and then handed the same authority to the block's author. Measured both ways:
  a box at height 2 with the boundary at 100 emitted the stall for a block that merely *declared*
  Height=100 — and that stall's text invokes a contract (E2a clause 3) whose operator response is to
  treat an unreachable pin as a critical and irrecoverable failure, so one integer from any peer was
  a remote liveness kill; and a box AT the boundary handed a block declaring Height+1 answered
  `Reject`/`ErrWrongParent`, the reason it gives ordinary stale traffic, so "the stall propagates to
  every descendant" was true only by the accident that the box cannot advance its head. The lesson
  is narrower than "bind the height": **deleting a knob does not make a decision unconditional if the
  replacement input is still someone else's.** The knob's authority has to land on something the box
  owns, and the box owned `head.NextHeight` the whole time.
- **Rejected — keep the type, ignore the fields.** It leaves a struct whose whole meaning is a
  policy that no longer exists, and the next reader has to prove it inert. The certification's
  §2.1 already had to prove exactly that (the knob was inert DOUBLY and the ruling named only one
  level); leaving it is how that cost recurs.
- **The error is RENAMED**, `ErrRecoveryDirectiveAbsent` → `ErrRecoveryBoundaryStall`. The old name
  asserts the absence of a directive, and a directive is what this row deletes; a name that
  describes a deleted concept is worse than no name. This is not an `R-*` residual name, so rule 4
  is not engaged.

### (2) H-1 — unify the boundary predicate on the STRICTER form

Two copies of one consensus-adjacent predicate exist:
`isAmbiguousRecoveryBoundary` (`floorbox_v5.go`) requires
`LivenessRecoveryHeight ≠ 0 ∧ h = it ∧ epochsEnabled ∧ EpochBlocks ≠ 0 ∧ h mod EpochBlocks = 0`;
`rotateOps` (`floorbox_recompute_stateroot_rotate_v5.go`) requires only the first two.

- **Chosen:** `rotateOps` calls `isAmbiguousRecoveryBoundary`. Behaviour at that site is unchanged
  — `rotateOps` is reached only on an epoch boundary of an epochs-enabled chain, so the three extra
  conjuncts are already implied there — and the drift hazard is gone.
- **Rejected — adopt `rotateOps`'s looser form at both sites.** The certification names the cost:
  the box would newly stall at non-boundary heights. A liveness regression, not a safety one, but
  a real one, and bought for nothing.

### (3) H-4 — `trustFloor` off the contract surface

The certification REFUTED its own first draft here. The decisive artifact is the pruned leg of
`validateBondRegs`, `core/chain/chain.go:1991-2001` (the certification cites it as `1801-1815`, which
is where it sat in the tree that certification read; at this SHA that range is `finalityQuorumActive`
— verified, and re-cited to the live lines rather than carried forward): a pruned block *below*
the floor skips the space-time re-verify, so a caller who supplies a RAISED floor makes the box
skip proof verification for everything under it. The floor is not a benign contract parameter; it
is itself a wrong-accept vector.

Today the composition's `StateView` carries `TrustFloor() uint64`. `liveView` answers the node's
real floor; `provenView` answers a hard-coded `0`, which makes `v5ValidateBondRegs` *Reject* every
pruned block. So the box does not currently take a floor from anyone — but a floor-shaped hole is
on the interface, and a `0` that means "reject everything" is a value standing in for a refusal.

- **Chosen:** delete `TrustFloor() uint64` from `StateView` and replace it with
  `PrunedTolerated(h uint64) (bool, Availability)` — the QUESTION the one caller asks, never the
  scalar. `liveView` answers `(h < c.trustFloor(), Present)`, the node's rule unchanged.
  `provenView` answers `(false, NoWitness)` and the composition STALLS rather than Rejects. After
  this there is no floor VALUE anywhere on the box's contract surface, so a raised floor is not
  expressible rather than merely unused. `Availability` / `Present` / `NoWitness` are the
  interface's existing three-valued vocabulary and "pruned-tolerance" is `chain.go`'s own name for
  this gate, so rule 8 is not engaged: nothing new is named.
- **Rejected — hoist the pruned leg out of the composition to its two callers.** It puts a third
  copy of one validity rule in the tree (the era-3 `validateBondRegs` already holds one). That is
  the #402 trap the composition exists to close, and `stateview_v5.go` says so in its own comment.
- **Rejected — do nothing, and ship only H-4's literal test** (`ValidateCommitV5` takes no `*Chain`
  receiver and no floor argument, which is already true). It would make the row's fourth
  deliverable a gate with nothing behind it — rule 7 applied to a deletion.

### (4) H-3 — refusing pruned blocks

`(*Box).Validate` already stalls on `b.IsPruned()`, and `NewBox` already refuses a pruned PARENT
(its `Hash()` is a stored token bound to no field, including `StateRoot` — the §2.4 finding).
`WitnessValidateV5`, the other exported box entry, did not. The first cut ADDED the check there so
that "the box refuses pruned blocks" would be true of the box rather than of one of its two
functions. The blind PE named that as the tell: adding a third copy of one check, in the same PR
that deletes a duplicated predicate on drift-hazard grounds, is the signal that the right
subtraction was the ENTRY. The second entry is deleted below and the check exists once, at the
door, plus `NewBox`'s refusal of a pruned PARENT (§2.4 — a pruned parent's hash binds no
`StateRoot`).

**`WitnessValidateV5` is DELETED — reversed after the blind PE review.** The first cut kept it and
flagged the deletion, reasoning that owner call 2 ratified a specific list, that the P-table delta
certification §6 lists the function, and that `TestG6_ExportedBoxDoorInventory` pins it. The PE's
counter-argument is a correctness one and it holds:

1. It holds no head, so B-1 — key the recovery posture on something the box owns — is structurally
   unfixable there. Keeping it ships TWO exported box entries with TWO recovery semantics, one
   fixable and one not, on the RC's last floor-box item.
2. This round made it WORSE, not inert: the pruned refusal was newly ADDED to it to make "the box
   refuses pruned blocks" true of the box. That is a third copy of one check, added in the same PR
   that removes a duplicated boundary predicate at `rotateOps` because "two copies of one
   consensus-adjacent predicate agreeing by reachability is a drift hazard". Both cannot be right,
   and the resolution is to subtract the entry rather than add the check.
3. The door inventory is an in-repo allow-list built to make a NEW door a reviewed event. A list
   that ENUMERATES today's surface does not oblige preserving it; removing an entry is a one-line
   diff and the gate is unharmed. The same reading applies to the certification's §6 line.

Cost, paid: five test call sites. Four were per-increment "STOP boundary" guards that passed
`Block{Version: 5, Height: 3}` — no roots, no signatures — to a function that short-circuits before
reading anything, so they could not have caught a flip in the increment they sat beside; the guard
they stood for is now held once, at the only place a flip can happen, driven on real node-accepted
blocks of every class. The fifth was a tier in the R0.4b split gate whose only assertion was
"did not Accept"; that gate's actual invariant is the box/full-node split, which both remaining
tiers carry. This file's policy tests re-home onto the policy unit itself and onto the door.

The deletion is off the ratified owner-call-2 list. It is recorded here and in the CHANGELOG as a
scope decision taken by the coordinator on the PE's ruling, not as something owner call 2 covered.

### (5) H-2 — the re-anchor contract

All four §2.3 clauses are written down where the residual already lives
(`docs/design/owned-residuals.md`, the `-ws-checkpoint` entry) and at the box's own door. Clause 3
— an unreachable pin is a critical and irrecoverable failure, never a silent degrade to
indeterminate-and-keep-going — is the one silt had not written down, and it is the whole reason
the certification says to adopt the schema rather than just the purpose.

### (6) The driven never-Accept suite

**What "v5 block class" means here.** The Hash-committed payload families a v5 block can carry,
plus the two block shapes that are classes in their own right (pruned; at an ambiguous recovery
boundary). The enumeration is pinned by reflection over `Block`'s own fields with a written
disposition for every field the table does not drive, so a new payload field reddens the coverage
meta-test instead of silently escaping the suite.

**The attack each arm runs.** Build a block of that class that the NODE ITSELF accepts (the
oracle), then commit a DIVERGENT `StateRoot` — the root of a different payload — and re-sign and
re-certify it, so the attacker's block is fully valid on its face and lies only about the
post-state. Assert the door never Accepts.

- **Rejected — forge the WITNESS rather than the block.** That is the recompute's adversarial-root
  ladder, which already exists, is driven per field across classes P/A/B/M, and is FROZEN by
  `D-RECOMPUTE-FREEZE`. Re-deriving it at the door would buy no property and would grow the
  keystone the freeze exists to stop growing.
- **Rejected — build a complete honest witness per class so every arm reaches the R1.8 downgrade.**
  That is the R1.x ladder. The measurement above shows each class already reaches the door and
  names its own stall, which is what makes the arms non-vacuous; a full per-class witness would
  cost the ladder's work to strengthen an arm whose property (never Accept) is already total.

**Non-vacuity, per the repo's own NG-2 rule.** The new file joins `twinGateFiles`, so every test in
it must call an honest-twin helper directly: the fixture's honest block must run the whole
composition through the door to the downgrade. A box that stalled on everything would satisfy
"never Accept" trivially, and that is the failure mode the twin catches.

**Ablations (rule 7).** Each arm names the source edit that reddens it, and each is run once:

| arm | ablation |
|---|---|
| every payload class, never-Accept | remove the door's R1.8 downgrade (`out == Accept` ⇒ Indeterminate) |
| the boundary posture is the box's position | key the door's recovery decision on `b.Height` again (both directions) |
| issuer keys | remove the scope gate's `len(b.IssuerKeys) > 0` clause |
| carrier / bond regs / slashes | remove the digest pre-set requirement |
| recovery boundary | restore an escape past `recoveryBoundaryDecision` |
| pruned block | remove the door's `b.IsPruned()` stall |
| no floor on the surface | re-add a floor-shaped method to `StateView` |
| class coverage | add a payload field to `Block` and leave it undriven |

## The through-line: a comment is an assertion, and only execution keeps it true

Not an incident — a pattern, four times in one branch, each caught by someone RE-RUNNING the claim
rather than reading it. Recording it because the shape is the lesson, not any one instance.

| # | The sentence | What was actually true | Caught by |
|---|---|---|---|
| 1 | `coldAuditorUndriven["Height"]`: *"P1 binds it to the box's OWN head before any other read"* | three block reads precede P1, and the door read `b.Height` under cover of it | blind PE, B-1 |
| 2 | H-4 clause (2): *"any method whose NAME carries 'floor' ⇒ RED"* | name-contains AND arity-1-uint64; `Anchor() uint64` walked past | blind PE, ablation |
| 3 | H-4 clause (2) v2: *"no method returns a lone scalar today, so the rule is exact"* | a fact about the surface, not the rule; three re-spellings walked past | blind PE, ablation |
| 4 | *"each conjunct dropped in turn flips exactly one row"* | one conjunct was dead, and after deleting it a second was still not isolated by any row | blind PE, then me, running it |

Number 4 is the one worth dwelling on. The PE found the dead conjunct; I deleted it and wrote a
corrected sentence saying each of the four survivors flips a row. Then I ran the matrix instead of
trusting my own correction, and `LivenessRecoveryHeight != 0` came back **GREEN** — no row isolated
it, because the only input that does is height 0 on an unconfigured chain (genesis), which the table
did not contain. Dropping it makes genesis read as an ambiguous recovery boundary and every box stall
at height 0 forever. So the corrected sentence would have shipped false too, and the fix is a new
row, not more careful wording.

**The rule this yields.** A sentence describing what a gate checks is itself an assertion, and it
decays exactly like a cited test name: nothing executes it. Three defences, in order of strength:

1. **Prefer a rule with a closed complement to a pattern.** Clause (2)'s first two forms matched a
   name, then a shape; both had unbounded escapes and both were escaped. The partition — *on the
   allow-list, or ends in `Availability`* — has none, because there is no third option to spell.
2. **Run the claim, one condition at a time.** "Each conjunct is load-bearing" is a measurement, not
   a description. Two of five were not, and only the matrix said so.
3. **Put the prose where a machine reads it.** The excuse rows name their gate so
   `check_cited_tests` resolves the name — which is why this round also had to fix that lint, since
   it scanned comments only and every excuse row is a string literal. A claim a tool can check is
   the only kind that stays true without someone choosing to re-derive it.

## What this row does NOT do

It does not flip the box to Accept (R1.8, a consensus-rule change, research-gated and outside the
freeze). It does not add a floor-box class, a rung, or a structure round — `D-RECOMPUTE-FREEZE`
forbids all three. It closes no `R-*` row and opens none.
