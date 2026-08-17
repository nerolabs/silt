# Making 184-equivocation and 184-partition drivable on the wire

**Date:** 2026-08-17 · **Trigger:** PE work-order item 2 — an undrivable SECURITY
drill is a RED to fix on the deterministic tier, not a GAP to accept (the
red-team entry criterion). Both marquee attacks GAP-because-undrivable on every
field sheet, including the 0-fail 82bcd2b. The safety property is certified
in-process (#204, #345/#350 oracles); what is missing is driving the attack
*live*, which is the engagement's whole point.

## The mechanism, each drill (evidence)

### 184-equivocation — the double-sign can't be PLACED

`adv_equivocation` (scenarios.sh) has two gates, both currently GAP:

- **Gate 1 (standing):** the adversary must reach qualified-proposer standing
  (`-goodpropose` ACCEPTED) before it can place a fork. A warm-up race; passed
  this run.
- **Gate 2 (placement) — the blocker:** `Node.Equivocate` (`adversary.go:226`)
  pins the fork base at the current head's NEXT height H and drives
  `proposeAndCommitTo` to commit fork Y on val-c and fork X on val-b at H. But
  `proposeAndCommitTo` (`adversary.go:49`) needs each target to ATTEST then
  COMMIT the fork — and on a live chain the honest designee for H has *already*
  proposed and val-b/val-c have *already* attested H (I2 never-sign-twice
  refuses the conflicting proposal: "already attested a different block at
  height"). The honest chain races ahead of the adversary every height, so the
  double-sign never lands. Field verdict verbatim (console-82bcd2b:38).

The in-process oracles drive it because the driver *controls the schedule* — it
quiesces the honest proposers and lets the adversary place first. The live WAN
chain has no such pause.

### 184-partition — no heavier fork FORMS

`adv_partition` blocks val-c from val-b for ~30 s, then heals and waits for a
reorg line. But a validator only reorgs onto a heavier fork if one *formed*
while it was away — i.e. the majority side committed ≥1 block in the window. On
an idle chain (no publishes during the 30 s) both sides stay at the same height,
nothing is heavier, no reorg is emitted. The GAP is a missing DRIVER: the
majority side must be *made* to commit during the partition.

## The common root

Both drills need the honest chain's *timing* controlled, which the field harness
can't do the way an in-process driver can. But each needs a different, small
thing, and both compose EXISTING primitives (`-block-peers`, `-equivocate`, and
`ft_publish` as a commit driver):

- **partition**: DRIVE commits on the majority side during the window — submit
  publishes to val-b (reachable) while val-c is blocked, so a heavier fork
  actually forms. Then the heal has something to reorg onto. This is a pure
  harness change — no product change — and it's the honest fix: the drill was
  under-driven, not the product under-capable.

- **equivocation**: create a placement window where val-b and val-c have NOT
  attested the target height. The minimal mechanism is to **partition the honest
  designee(s) away from val-b/val-c** for the placement (reuse `-block-peers` on
  the honest proposer, or on the two targets, so the honest H-proposal doesn't
  reach them first), THEN fire `-equivocate`. The adversary places X on val-b and
  Y on val-c into the gap; on heal, the double-sign is caught and slashed. Also
  harness-composed from shipped primitives.

## Options

- **(a) Harness-only drivers, composing shipped primitives** (partition drives
  publishes; equivocation adds a placement-window partition of the honest
  proposer). Both attacks then drive live with no product change. Cost: harness
  design + a KEEP_UP local dry-run to prove the drivers before a cloud spend.
  Benefit: honest (the drills were under-driven), cheap, no new adversary
  surface in the product.
- **(b) A product knob to pause honest proposing** (a `-drill-quiesce` flag).
  Rejected: adds an adversary-adjacent knob to the product for a test-harness
  need that (a) already covers with partition; violates R3 (throwaway staying
  out of the product).
- **(c) Accept the GAP, drive only in-process.** Rejected by the PE ruling —
  the whole point of #183 is the WIRE drill driving.

## Decision (2026-08-17) — build HELD, PE consult filed

Builder lean is **(a)** (harness-composed drivers, no product change, proven on a
local KEEP_UP net before any cloud spend). But the equivocation placement-window
carries a security-judgment call that belongs to the ruling author, not the
builder: partitioning the honest proposer so both targets attest the adversary's
fork is *close to staging* the double-sign rather than the adversary earning it.
The PE's bar is "the adversary must PLACE the double-sign"; a harness-created
window is legitimate iff the SLASH still fires from the honest detection path
unaided. That is exactly the B8 external-adversary / self-marked-homework line.

Per Andrew's direction (2026-08-17), the PE is consulted on BOTH drills before any
build — the partition majority-side driver's severing shape included. Consult:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/184-drill-drivability-CONSULT.md`.
**Nothing ships until it returns.** This deliberation doc ships in the same PR as
the eventual fix.

## PE RULING RETURNED (2026-08-17) — build (A), both drills

`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/184-drill-drivability-ruling-PE-2026-08-17.md`.
The governing line: **the harness may create the adversary's OPPORTUNITY (any
*real* adversary capability — a partition is one: eclipse / BGP hijack / link DoS
/ just waiting for a natural WAN split); the adversary takes its OWN malicious
action; the DEFENSE fires unaided from the product's path on real evidence.
Setup is red-teaming; result is homework; the B8 line sits at the defense.** So
(A) is not bar-lowering — the field drill via a real network event is a STRONGER,
more realistic test than the schedule-pausing in-process oracle.

**Equivocation — legitimate, three BINDING conditions to assert:**
1. Real self-incrimination — show the adversary authored BOTH prepares (X and Y
   at `(H, r)`); `requireProposerPrepare` makes it un-bypassable, assert it.
2. Detection + slash are 100% the product's — assert the real
   `slashed equivocator … double-signed at height H` line from the honest
   reconcile path (`FindEquivocations` on val-b's sync), never a harness-echoed
   string (#7).
3. Transparent narration — the drill text says what the ADVERSARY did (induced
   the placement-window partition, authored both forks) vs what the PRODUCT did
   (reconciled, detected, slashed).

**Partition — legitimate, B8-free, but SHAPE CORRECTION (the builder's own worry,
confirmed):** a one-sided val-c↔val-b cut may leave val-c current via val-a/val-d
(quorum intact) → no heavier fork → still GAP. The fix: sever val-c (plus enough
weight) into a genuine **< ⅓-weight sub-quorum MINORITY that CANNOT commit**, cut
from a **> ⅔-weight MAJORITY that CAN** and is driven publishes; heal; assert
val-c's real reorg line. Confirm the sever puts val-c below the commit threshold,
or it silently under-drives again.

Both: **red-team READINESS, not the M0 verdict** (the verdict stays #183's, the
external red team driving their own attacks). Prove both on a **local KEEP_UP net
before any cloud spend** (#6). Decision: build (A) now.

## BUILD-TIME FINDINGS (2026-08-17) — two objective-mode obstacles the ruling's mechanisms hit

Implementing (A) against the ACTUAL cloud config surfaced two mechanism facts the
ruling (reasoning from the general principle + the legacy-mode e2e references)
didn't account for. Both are evidence-backed, not guesses — config math + the
already-obsolete e2e tests confirm each.

**The base cloud topology is OBJECTIVE, 4 validators, `quorum = max(1, n_val-2) =
2`, ByzantineQuorum ON → commit floor `RequiredQuorum = max(2, bftThreshold(4)=3)
= 3-of-4 support** (`topology.py:240`, `chain.go:1828 requireQuorumStack`). Every
commit — of any block, by anyone — needs 3 of the 4 anchors.

### Finding 1 — partition: the "reorg line" cannot fire; a BFT minority CATCHES UP

The current drill asserts `chain: reorged onto a heavier fork (dropped N…)`. That
line fires only when val-c DROPS a locally-committed block (`chainrole.go:1103`,
`dropped > 0`). But under 3-of-4, a severed val-c (≤ 2 of 4) **cannot commit
anything** — so on heal it is a PURE CATCH-UP (`dropped = 0`), never a reorg.
`e2e/partition_test.go:37-47` is SKIPPED as obsolete for exactly this: *"a 2-of-4
minority correctly CANNOT commit a conflicting fork… this is why the cloud's
184-partition now GAPs… A BFT partition-heal test — a supermajority commits, a
stalled minority CATCHES UP on heal — replaces it."* So the honest wire signal is
not a reorg-drop; it is: val-c **stalls** at its pre-partition height during the
window (anti-vacuity: proof it was a real < ⅓ minority that couldn't commit),
then on heal **advances to the majority's heavier head (height AND hash match)**.
This is a **refinement of the ruling's "assert the reorg line"** to the signal the
BFT model actually produces — same substance (val-c returns to the heavier
chain), truthful log. **Buildable now**, harness-only, no consensus change.

### Finding 2 — equivocation: objective 3-of-4 blocks single-target PLACEMENT

The ruling's placement-window partition solves the ATTEST gate (honest hasn't
attested H yet). But there is a SECOND gate it doesn't reach: `Equivocate` →
`proposeAndCommitTo` (`adversary.go:49-109`) drives a real COMMIT on each target,
assembling a QC of only **{adversary, target} = 2 attestations**; the target's
`ValidateCommit` → `requireQuorumStack` needs **3**, so the target refuses to
commit (`ack.OK=false`) and `Equivocate` loops "attested but not yet committed"
forever. The working `e2e/equivocation_test.go` uses **LEGACY** mode
(`-objective=false`, `-quorum 1`) where a single target commits — it does not
translate to the objective cloud. The honest objective-mode path is the
**slash-on-DETECTION** route the product already has (`chainrole.go:1085` "Slash
on DETECTION, not on adoption") — place two conflicting SIGNED blocks (X on val-b,
Y on val-c) that need NOT commit; when val-b syncs val-c's fork carrying the
adversary's Y-signature at H, `FindEquivocations` catches it against val-b's held
X. But the current adversary primitive INSISTS on a commit before it marks a leg
placed, so it can't drive the detection path as-is. Making it place
conflicting-signed-blocks-for-detection (no commit) is a change to the adversary's
consensus interaction — **research/PE-gated (#6), not a build-alone call.** This
is the deeper root of the equivocation GAP: not just "honest already attested,"
but "no minority can commit and a single-target commit needs 3-of-4."

**Split decision:** partition is buildable now (Finding 1's catch-up signal, a
faithful implementation of the ruling's substance). Equivocation needs a focused
PE/research follow-up on the objective-mode placement mechanism (Finding 2) before
its harness can be built. Reported to Andrew for the proceed call.

## PE CONFIRMED both (2026-08-17) + oracle proven — a THIRD constraint surfaced

PE confirmation
(`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/184-drill-drivability-objective-mode-CONFIRM-PE-2026-08-17.md`):
both corrections CONFIRMED, the PE owns the legacy-mode-reference error. Partition =
stall-then-catch-up (the absence of a reorg line IS the I1 safety property).
Equivocation = `PlaceConflictingSigned` slash-on-detection ("the crime is SIGNING
two conflicting blocks at one height, not COMMITTING two forks"), likely NO
partition needed.

**Mechanism now PROVEN at the code level** — oracle
`TestModelCheck_184_ObjectiveEquivocationSlashedOnDetection` (core/node),
objective era-2 A=4 3-of-4 (the cloud regime), GREEN, failing-first confirmed (a
slot-mismatched prepare does NOT slash). It pins the exact rule:

- **Era-2 slashability = a shared (round, phase) CONSENSUS signature**
  (`VerifyEquivocation` → `consensusSigScopes`: PrepareQC/Atts vote slots; the
  bare-hash ProposerSig is authorship, excluded). The equivocator must have
  ATTESTED (prepared) both conflicting blocks at the same slot.
- **Detection needs the honest side to HOLD an adversary-signed block at H** — i.e.
  the equivocator's prepare must be IN the honest committed block. The served
  losing fork carries the adversary's prepare over a different block at the same
  slot; `slashEquivocators` fires pre-Reconcile, never adopts the loser.

**THE THIRD CONSTRAINT (new, decision needed):** because detection requires the
adversary's own prepare to be in the honest committed block, **the equivocator
must be a member of the CONSENSUS SET whose attestations are gathered into honest
commits — not the current drill's outside non-anchor that only earns *proposer*
standing** (`topology.py`: the `adversary` node is a non-anchor that `-goodpropose`
warms to proposer-eligibility). Options for the wire drill:
- **(A) Reuse an existing anchor** (e.g. val-d) as the Byzantine equivocator for
  the drill window — already in the consensus set, its prepares are in commits; no
  topology/C2-math change. Caveat: slashing a live anchor mid-sheet leaves 3
  anchors for later flows — run it LATE or restore val-d after.
- **(B) Make the `adversary` node an anchor** (4→5 anchors) — changes every
  quorum/maturity/C2 number; risky, rejected unless (A) proves unworkable.
- **(C) Keep it a non-anchor but force honest proposers to include its
  attestation** — fragile (inclusion isn't guaranteed under the 3-of-4 gather);
  rejected.
Builder lean: **(A)**, run the equivocation drill late (or with val-d
restore-after), consistent with a real equivocation coming from a validator WITH
standing. Surfaced to Andrew because slashing a live anchor mid-sheet has
sheet-wide consequences.

## PE TOPOLOGY RULING → (D) + BUILD COMPLETE (2026-08-17)

`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/184-equivocation-topology-ruling-PE-2026-08-17.md`:
**(D) — a dedicated, ephemeral equivocation network. Not (A).** Verified: a
mid-sheet slash leaves the requirement pinned at `⌊4/2⌋+1 = 3` over the CONFIGURED
anchors while only 3 stay live → every later commit needs all 3 unanimously (zero
fault tolerance, spurious end-of-sheet flakes). Reusable rule banked: **classify
drills by recoverability — recoverable ones (partition, restart, stalled publish)
run in sequence on the shared sheet; the one irreversible drill (permanent F2
eviction) runs on its own throwaway net.** (C) liveness-after-eviction is an
optional MATURING bonus, gated on verifying post-latch maturer-gather + residual
weight-quorum — deferred.

**Built and proven (all green, failing-first where a defect would hide):**
- **Mechanism oracle** `core/node/modelcheck_184_equivocation_objective_test.go`
  (objective era-2 A=4 3-of-4): detection on a served losing fork + the
  `PlaceConflictingSigned` primitive slashed over a real `SyncChain`. Failing-first:
  a slot-mismatched prepare does NOT slash; skipping the Byzantine act does NOT
  slash.
- **Primitive** `Node.PlaceConflictingSigned` + the crafted-fork serve override
  (`GetChain`/`GetChainHead`) + daemon `-equivocate` objective-mode branch.
- **Equivocation drill (D)** `e2e/equivocation_test.go` — objective 4-anchor
  dedicated net over real TCP (GREEN, ~32s); the cloudtest main-sheet flow is now a
  skip-with-pointer to this home (the destructive drill is not mid-sheet).
- **Partition drill (Finding-1)** `integration/cloudtest` `adv_partition` +
  `e2e/partition_test.go` — sever the minority from the whole majority, drive the
  majority, assert STALL then catch-up to the majority head (height+hash); GREEN
  e2e (~68s), replacing the obsolete pre-BFT reorg test.
- Full `go test ./...` GREEN incl. both e2e drills together (~99s); shell lint clean.

Red-team readiness (item 2 of the state-of-HEAD work order): both marquee attacks
now DRIVE and their defenses demonstrably fire — no longer GAP-because-undrivable.
Optional follow-ups noted, not built: (C) the MATURING liveness-after-eviction
drill; a dedicated CLOUD equivocation net (the e2e/netem dedicated net satisfies
the readiness gate per the ruling).

## WIRE VALIDATION (HEAD 1ebd487, PE next-steps step 1) — 2026-08-17

Two clean-HEAD cloud runs (base `1ebd487-73707`, MATURING `1ebd487-7457`):

- **Base: 16 pass / 2 gap / 0 FAIL.** durability-turnover **PASS** (#461 confirmed
  on WAN), 184-equivocation **SKIP→dedicated net** (renders right), 6-fault-tolerance
  GAP (down-designee 260s ladder, M1-adjacent — PASSED in MATURING, so a load flake
  not a break), 184-partition GAP (below).
- **MATURING (salvaged; hung in 10b — see below): 16 pass / 1 gap / 2 skip / 2 FAIL.**
  durability-turnover **PASS under mature contention** (the regime the residual was
  born in), 10-maturing-handoff **PASS** (latch on the wire, h66 in the 1980s bound),
  10a-stall-drill **PASS** (B2 weight-quorum), 184-equivocation SKIP, latch tripped
  h21. Two chaos crash-recovery FAILs (chaos-reprovide, chaos-fetch) — MATURING-only,
  passed in base; storage/registry under heavier load; attribution PENDING (not the
  fixes under test — those passed).

**Headline: both red-team-gate fixes CONFIRMED on the wire.** #461 publish
reliability and #462 equivocation-drill isolation both hold on real multi-region WAN.

**184-partition — root-caused from the wire evidence + FIXED (harness).** The drill
GAPped honestly ("val-c ADVANCED … sever did not isolate it"): the anchors-only
sever missed the OTHER validator-role chain-holders — val-c synced the majority's
committed chain THROUGH the bonded `adversary` node (base: h14→h16) and the maturers
+ sybils (MATURING: h25→h37), logging "reconciled" not "committed block". Fix
(`scenarios.sh adv_partition`): sever val-c from EVERY validator-role peer
(validator/adversary/maturer/sybil), built from topology.json. Needs a re-run to
confirm the wire drive.

**The MATURING run HUNG — harness robustness bug FIXED.** `ssh_node` wrapped
`gcloud compute ssh` over IAP with NO timeout; a stalled tunnel to maturer-2 during
10b's stop-loop wedged the whole run ~1h (no verdict, VMs burning until a manual
kill + teardown). Fix (`lib.sh`): a hard `timeout ${SSH_NODE_TIMEOUT:-90}` on every
remote call, so a stalled node degrades that one call (callers already tolerate a
non-zero ssh exit), never the whole run. Both runs torn down verified-clean (0
instances/networks/firewalls).
