# silt — the release candidate list

**The line this list draws:** a release candidate is the point at which handing silt to an
adversary is a responsible act. Not the point at which it is pleasant to use.

Every item carries a done-condition, the evidence tiers it must clear, and a gate test that
is proven RED before its fix. A gate that was never red is a comment, not evidence — so
each one also carries a vacuity guard proving its fixture could fail.

**The date: 2026-09-27.** Anything not green on that date ships disclosed rather than fixed.

**The verdict this list ships toward is not on it.** An external adversary — given the
artifact and the claims, but not the rationale — returns DENIED on publish-to-identity
linkage, on identity-level or global takedown, and on Sybil-farmed standing at a discount.
That is the mission, and it is not the builder's to turn green. The items below are the
evidence that makes commissioning it worth doing. They are not a substitute for it.

---

## Tier A — the handoff is not responsible without these

**1. Eligibility resolves from the block's parent state, never the replica's live latch.** ✅ *done*
*Done:* the floor box proves every whole-set pre-state member list against `prevStateRoot`
where it reads it, so an omitted or injected id stalls. It no longer depends on a later fold
op, which was emitted only when the set changed — and the post-set is derived from the
pre-set, so the forgery itself decided whether it would be checked.
*Evidence:* unit → consensus model-check → e2e.
*What it closed:* re-seating a slashed equivocator as bonded and qualified; erasing a bond
registration so its block folds to the root before it; keeping an under-bonded validator's
standing; evicting an honest validator from the frozen epoch set for an epoch; staling the
decentralization digest that decides whether the launch anchors have shed. Each is a gate
that was seen red before the fix and asserts a stall after it.
*Also closed:* the cross-phase seam — quorum intersection holds across the launch→mature
handoff, phase follows applied history rather than delivery order, and the one-way shed does
not re-arm under fork adoption.

**2. Forging N standings costs N×, with no arm passing vacuously.**
*Done:* every arm paired with a positive control **on its own axis**. The bond and
possession arms pass. The demand and diversity arms do not construct, and are reported
**unwired** rather than denied.
*Evidence:* unit → integration → field.
*Ships with:* removing the build-status paragraph from `VISION.md`, whose own header says it
carries none. The two are coupled deliberately — the paragraph is the only place that records
which axes are unwired, so it goes when this item's gate starts reporting that instead.

**3. The shipped default is the defended configuration, and the stock binary runs.**
*Done:* `silt daemon -validator`, no other flags, reaches serving; the possession audit and
the publish-token replay guard are on; every defence this list demonstrates runs flagless.
*Evidence:* integration → e2e → field. Gates four other items.

**4. The repo reads without the record that was deleted.**
*Done:* no milestone, lane, catalog, slice, issue or change-request identifiers in source,
test names or file names. Comments describe the product or cite external work.
*Evidence:* unit — a permanent source gate, kept in the suite so it cannot regress.

**5. One command from a clean clone reproduces every gate.** ⚠ *red — never driven on a box proven clean*
*Done:* fresh clone, no credentials, no committed binary → the suite runner builds from
source and maps each item onto a named suite.
*Evidence:* the run itself, from a scratch clone on a machine that has never built silt.
*State:* the clone is clean — no committed binaries, and the runner builds from source. The
run itself returned rc=137 or rc=124 on all twelve suites. That was attributed to the host
carrying other work; the attribution does not hold. rc=137 is the OOM-killer and rc=124 is the
per-suite cap, and both are also what a run inherits from the PREVIOUS run's leftovers — twelve
containers from an earlier suite were found still up with no driver behind them, one of them
already `Exited (137)`, holding ~950 MiB of the container host's memory. Whether the starvation
was the host or the leftovers is now untested rather than known. The suites start clean and end
clean mechanically (`ft_preflight` / `ft_sweep`, `integration/lib.sh`), so the re-run can tell
the two apart. Until it is driven on a box proven clean at the start, this item is red, because
a demonstration that could not be driven is a failure.

**6. The claims the adversary receives exist as an artifact.**
*Done:* an in-repo statement of the three denials in checkable form, plus what is out of
scope, written so a cold reader can construct attacks from it.
*Evidence:* a cold read.

**7. No consensus state derives from a field the block hash does not cover.** ✅ *gated*
*Done:* the seating map — which sets the maturity coefficient and decides whether the launch
anchors have shed — cannot be changed without changing the block's hash. Below the
witnessable format it can: removing a block's bonded attestations leaves it hashing
identically, so a rewritten history is invisible to the finality gate, fork-choice and the
checkpoint, all of which compare by hash. The witnessable format resolves it by taking the
seating from a carrier of precommits over the parent, which is folded into the hash.
*Evidence:* unit → e2e. The daemon refuses any configuration that would run a height on a
pre-witnessable format.

**8. A validator set that cannot grow is refused.** ✅ *gated*
*Done:* a block can seat a validator the chain has not seen. Below the witnessable format it
cannot — the state root commits the seating while the seating is read from the block's own
attestations, which sign over that root — so the set is frozen, the network never matures,
and the anchors never shed.
*Evidence:* unit → e2e, in every mintable format.

## Tier B — reachable, none free

**9. Consensus denials hold, and no honest node is ever slashed.** Equivocation attributed;
forged and under-bonded proposals rejected pre-attestation; a partition heals to one order.
The honest-never-slashed property asserted over the whole run's slash set, not per attack.
*model-check → e2e → field.*

**10. A prover without the bytes fails the audit and is paid nothing.** Three defeats: the
care link printed on every publish is the storage-proof verification key; a data-less
identity passes by relaying the challenge to a real holder; the inclusion proof is checked
against a root the prover supplies. Plus work bounds on the unsigned repair claim and the
challenge frame, neither of which is rate-limited.
*unit → integration → e2e under impairment → field.* Largest code item in scope.

**11. Publishing is unlinkable and no surveillance artifact exists.** A matching score
against chance; seize every disk and emitted byte after a fetch-heavy run and find no
(fetcher, content) pair; published metadata no longer leaks the exact plaintext byte count,
which today makes the padding defence a no-op.
*integration → e2e → field.*

**12. Takedown bites, cannot go global, and is provable.** Honouring operators stop serving
and others keep serving; no accepted operation removes more than one named root or works by
identity; every honoured removal carries inclusion and consistency proofs.
*integration → e2e → field.*

**13. Bit-perfect or an explicit failure, and crash recovery needs no human.** An unplaceable
publish names what could not be placed, returns no link, leaves no registry entry, and still
succeeds on retry. A validator killed mid-consensus re-pins from its own last finalized
checkpoint and never contradicts a signature it made before the crash.
*e2e under impairment → field.*

**14. The floor box holds, and the chain prunes.** ⚠ *red — half demonstrated, half unwired*
A validator on the declared floor spec — one core, 2 GiB, 10 GiB of disk — validates against
witnesses without holding the tree, stays under its memory ceiling on adversarial input, stalls
rather than accepts when no witness provider is reachable, and prunes at depth from persisted
state.
*unit → e2e under impairment → field.*

*State:* the claim has four legs and they are in three different conditions. The `floor` suite
drives what a binary can be made to do; it enforces the spec with a cgroup rather than a flag,
because `-mem-limit` is a SOFT ceiling and a soft ceiling cannot answer "does this survive on a
2 GiB box".

- **Under its memory ceiling — DEMONSTRATED, under HONEST load only.** `memory.max` 2 GiB with
  `memory.swap.max` 0 and `nproc` 1, asserted from inside the container before anything else is
  measured. Peak 291.1 MiB, 14% of the ceiling, read from the cgroup's own high-water mark
  rather than sampled. The box committed to height 22 on the same head hash as two ordinary
  validators, so it was participating and not merely surviving. **The gap:** the load was honest
  consensus traffic. "On adversarial input" is NOT yet shown — that wants the redteam and sybil
  topologies re-pointed at a floor-spec seat, which is the next increment on this item.
- **Prunes at depth from persisted state — DEMONSTRATED.** Seven blocks shed their heavy bond
  proofs at height 22, which is exactly what the retention rule predicts:
  `retain = max(2·BondTTL, BondRegHeadWindow+4) = 12`, `raw = 22-12 = 10`, epoch-aligned to 8,
  leaving heights 1–7 (genesis carries no registration). A restart onto the same volume reloaded
  the already-pruned store, still reported the shed, and committed again to height 24 — so the
  prune is a property of persisted state and did not trade an OOM for a stall.
- **Validates against witnesses without holding the tree — UNWIRED.**
- **Stalls rather than accepts when no witness provider is reachable — UNWIRED.**

*Why UNWIRED and not skipped:* there is no process to drive. `chain.NewBox` has no caller
outside `core/chain`'s own tests, the only implementation of `chain.WitnessSource` in the repo is
the `proverSource` test double, and no daemon flag puts a node in that mode. The mechanism is
thoroughly gated at the unit tier; it has no transport and no entry point. Reporting these green
on the strength of the unit gates would be the exact substitution this list forbids — a
demonstration that could not be driven is a failure, and "unwired" names which failure it is.

*What would turn it green:* a witness server and a floor-box daemon mode, then the two witness
legs added to the same suite. That is product work, not harness work, and it is the item's
critical path — the memory and prune halves are done.

**15. The floor box can post the bond its disk allows.** ✅ *done*
*Done:* the plot is sealed to disk block by block and answered by sparse reads, so residency
is the leaves and their tree rather than the plot's size. A 5 GiB plot needs ~247 MiB
against a 1 GiB budget, where it previously needed 6,219 MiB.
*Why it is on this list:* standing is proportional to bonded size. When the largest plot a
node can hold was set by its memory, the biggest bond a small operator could post was a
sixth of the disk they bought, and consensus weight concentrated on larger machines for no
reason anyone chose.
*Evidence:* unit → e2e. A byte-identical-plot test proves the streamed and resident seals
commit the same bond and that a disk-backed plot answers a live challenge that verifies.

**16. Full history fits a volunteer.** ✅ *done*
*Done:* the archival tier keeps every block to genesis and keeps heavy possession proofs for
a bounded window rather than forever. Five years of full history at 100 bonded validators is
~104 GiB, against ~30 TiB unshed.
*Why it is on this list:* a validator republishes a multi-megabyte possession proof every few
minutes to hold its standing, so retained proof volume grows with the number of independent
operators — making a more decentralized network one that fewer parties can archive, and the
deep past the property of whoever can afford terabytes.
*Evidence:* unit → integration.

**17. The economy mints nothing, and the core squeeze is measured.** Balance-lane credit
never becomes consensus standing; a colluding pair strictly loses; repair is funded from the
object's own escrow; a false claim is slashed. Plus core-node net margin at two edge
populations against held-constant demand, reported as a signed number.
*unit → integration → e2e.*

**18. Core carries nothing.** Seize a holder and fail to recover known plaintext, with a
key-holder succeeding on the same objects in the same run; core resolves hashes, never names.
*unit → e2e.*

**19. An un-upgraded node stalls, and the network never updates itself.** A node that cannot
validate a shipped format stops rather than accepting it and says so while staying alive. A
node whose consensus-reaching configuration diverges from what the chain committed refuses to
start. No version-floor advisory below the signing threshold changes anything, and no node
ever replaces its own binary.
*integration → e2e.*

## Tier C — field

**20. Publish and fetch work on the internet as it is.** A NATed publisher in one region, a
cold fetcher in another, bit-perfect bytes inside a bound derived from the deployed
configuration. The chain keeps committing under sustained load with injected latency, jitter,
loss and reordering.
*field.* Run it early: a failure needs time to reduce to a local reproduction.

**21. Every item has a cloud-harness run.** Each item above named in a cloud scenario, with
the gaps written down as decisions rather than left as silence.
*field.*

---

## What this list deliberately does not cover

Blob-layer unobservability against a global passive adversary — the vision says outright that
silt does not promise the impossible. The full multiplicative Sybil interlock: the vision
calls it the target and not yet the guarantee, and a destination that concedes a gap cannot
be used to manufacture a gate it does not claim. Throughput numbers — bounded first, fast
second. Conformance across implementations, when there is one. Ergonomics, SDKs and
embeddability. Cross-cloud field runs: the harness is a scaffold that stops at its first
unbuilt phase, and finishing it costs days that Tier A needs.

Two limits worth carrying in writing rather than by implication. The non-globality metric and
the diversity axis both rest on **self-declared** operator domains, so both are claims about
declared diversity. And the profitable-edge commitment is about a trajectory at network
scale; any single run measures one point on it.

## Known open, carried rather than closed

**A pruned block's body is not bound to its hash.** For a pruned block, the hash is a
stored linkage token rather than a content commitment, so an adversary can keep the token and
the real signatures while rewriting the body — including the carrier that seats validators.
The only defence is the first non-pruned descendant, whose signed state root is recomputed
over the rewritten ancestor state, and the consequence is a silent head truncation at that
descendant with the forged seating live in the replayed state. Bounded, not eliminated.

**The bonded set is capped by bandwidth.** Standing lapses after a short window and renewal
runs at half of it, so each validator republishes a multi-megabyte possession proof every few
minutes, and every other validator must receive and verify all of it. Each node ingests the
set size times that volume. This is live traffic, so retention policy does not touch it, and
it caps the practical bonded set well below the intended participant count.

## Tenets that could not be reduced to a demonstration

Legibility. The hexagonal core and the single lock-free loop — architectural constraints
asserted by structure, whose consequence is testable but whose violation would not
necessarily show. "Never reinvent a primitive," a negative over all future choices.
Reactive-not-eager, which states no threshold. Canon-tracks-behaviour and
throwaway-stays-throwaway, which are disciplines rather than properties of a running system.
