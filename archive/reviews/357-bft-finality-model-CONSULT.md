# Research consult — #357 §3: which in-band finality method for silt's objective chain?

**From:** silt build team · **Date:** 2026-08-13
**Re:** the #357 follow-up. §1 (convergent ramp weight) + §2 (stable BFT quorum) are **built, merged, and
cloud-confirmed** (a fresh 3-region GCP run converged: all 4 validators at an identical head hash,
durable over a settle window — the exact inverse of the reorg-to-height-0 symptom). Your prior ruling
recommended, and the owner **ratified, the bond-weighted BFT model (B)**. This consult asks you to
weigh the **methods for §3** — the in-band finality floor that *completes* model B — because the owner
wants the choice made on evidence: **highest trust, highest durability, most efficient.** He does not
have a strong prior; neither do we beyond your §3 recommendation, which we want stress-tested against
the alternatives now that §1/§2 are real.

This is a **consensus-rule + published-claim** decision, so per build-immutable #6 it is yours to
adjudicate; we implemented one candidate (B1 below), measured its blast radius, reverted it, and are
bringing you the trade-offs rather than shipping a model choice unreviewed overnight.

---

## What §1/§2 already established (so §3 is scoped correctly)

- **§2 already adopts BFT quorum-intersection safety.** `RequiredQuorum` is now sized against a *stable*
  set (the fixed anchor set during the young window → the committed bonded set at maturity). A direct,
  **already-observed** consequence: **a minority partition cannot commit** — a 2-of-4 anchor group can't
  reach `bftThreshold(4)=2` support, so it stalls rather than committing its own fork. The cloud
  `184-partition` GAPed for exactly this reason ("no heavier fork formed"), and the e2e
  `TestPartitionHealsToHeavierForkOverTCP` (which assumed a 2-of-4 minority commits) is now stale and
  paused. **So the "two conflicting committed forks" world is already gone under §2** — §3 is only about
  what happens to a block that *did* reach quorum.
- **§3's job:** decide whether a quorum-committed objective block is **final** (never reverted) and, if
  so, by what mechanism fork-choice is prevented from reorging below it.

---

## The candidate methods (with our measured findings)

### Method B1 — per-block rolling quorum-finality floor *(your §3 recommendation; we built + reverted it)*
Every quorum-committed objective block is final by quorum intersection (§2 gives the fixed set), so
`Reconcile` refuses any fork that does not contain our committed head — fork-choice's heaviest-weight
rule then only ever adjudicates among **descendants of the finalized head** (Tendermint/Gasper). We
implemented it as a ~10-line stateless guard (the fork must match our head hash at its index) reusing
the existing `WSCheckpoint`/`ErrPreCheckpointReorg` machinery.
- **Trust:** strongest. A committed block is *never* reverted; "reorg to height 0" is structurally
  impossible. Matches the code already calling the result "committed" and the publish semantics that
  treat a committed link as durable.
- **Durability:** strongest. Content whose registry entry committed can never be dropped by a reorg.
- **Efficiency:** cheap at runtime — an O(1) hash comparison per reconcile, no new state, no epoch
  machinery, reuses the checkpoint guard. **This is the most efficient of the three at steady state.**
- **Cost / blast radius (measured):** it rewrites objective fork-choice from "heal to the heavier fork"
  to "a committed block is final," so it breaks the two red-team heal tests
  (`TestRedteamF6_ObjectiveForkChoiceHeals`, `TestF7_ObjectiveForkChoiceNeutralizesDoubleBacker`) —
  which, on inspection, encode the old healing built on validators **double-signing** (3-of-4 attest
  *both* forks). Under finality that is a **slashing** event, not a heal. So the tests aren't a
  regression signal; they're the old model. Still: adopting B1 means committing to "committed = final"
  everywhere and rewriting those tests to assert slash-on-conflict instead of heal-by-weight.
- **Open risk we want your read on:** B1 finalizes *every* committed block immediately. If §2's set were
  ever wrong (a minority somehow committed), B1 would freeze a wrong fork — *unrecoverably*. §2 makes
  that impossible in theory (minority can't reach quorum), but B1 has **zero recovery margin** if that
  assumption is ever violated in practice.

### Method B2 — per-epoch finality *(Casper/Gasper shape)*
Finalize not every block but at **epoch boundaries**: a checkpoint block that gathers quorum of the
epoch's fixed set is final; fork-choice may reorg freely *within* the current (unfinalized) epoch but
never below the last finalized checkpoint. Reorg is bounded to at most one epoch of depth.
- **Trust:** strong, but finality is **delayed by up to an epoch** — a block is committed-but-not-yet-final
  until its epoch checkpoint finalizes. A reorg can still drop a *recently* committed block (within the
  live epoch), so "committed" ≠ "final" for a bounded window.
- **Durability:** strong for anything older than one epoch; a bounded window of recently-committed content
  is still reorg-exposed. This tension with silt's publish semantics (a returned link implies durable)
  is the thing to weigh — an epoch of "committed but reorgable" may need the publish path to wait for
  *finality*, not just commit, which adds latency.
- **Efficiency:** more machinery (epoch counter, per-epoch snapshot, boundary logic, a justified/finalized
  checkpoint pair) but finalization work is **amortized** (once per epoch, not per block). At high block
  rates B2 does less finality bookkeeping than B1; at silt's low objective block rate the difference is
  small and B1's per-block O(1) check is already negligible.
- **Cost:** the most code. Introduces a genuine epoch concept silt does not have today.

### Method A — Nakamoto reorg-depth cap *(stay probabilistic)*
No in-band finality. Keep heaviest-chain healing but refuse to reorg more than **D** blocks below the
head (Bitcoin's "N confirmations," made explicit). §1 already makes depth meaningful (a taller chain
genuinely outweighs a shorter one), so a depth cap kills the height-0 reorg once §1 is in.
- **Trust:** weakest and **probabilistic**. A committed block can still be reverted if a heavier fork
  appears within D; D is a tunable *guess*, not a proof. Sits awkwardly with a chain that already
  gathers 2f+1 and calls the result "committed."
- **Durability:** a committed block within D of the head is reorg-exposed; the guarantee is "very likely
  durable after D blocks," never "durable."
- **Efficiency:** trivial (a depth comparison), and it **preserves** the old healing semantics, so it
  breaks *no* existing tests — the lowest migration cost.
- **Cost:** near-zero code; highest ongoing conceptual cost (you must reason about D, reorg
  probabilities, and "how many confirmations is safe" forever — the thing BFT finality removes).

---

## The efficiency dimension the owner cares about (and where it's subtle)

"Most efficient" splits into two clocks:
- **Runtime cost per reconcile:** B1 (O(1) hash check) ≈ A (depth check) < B2 (epoch bookkeeping). At
  silt's block rate all three are negligible; this is not the deciding axis.
- **System-level efficiency (the one that matters):** BFT finality (B1/B2) lets the publish path treat a
  committed link as *done* with no confirmation wait — the most efficient *product* semantics. A (depth
  cap) forces either a D-block confirmation wait before a link is trustworthy (latency + bandwidth) or a
  standing probabilistic-reversal risk. So **A is cheapest in the consensus code but most expensive in
  the product** (every consumer must reason about confirmations); **B1 is cheapest end-to-end** because
  "committed = final = safe to serve" needs no extra machinery above it.

## Specific asks

1. **Which method** — B1 (per-block finality), B2 (per-epoch finality), or A (depth cap) — best serves
   *highest trust + highest durability + most efficient end-to-end*, given §1/§2 are already in and §2
   already enforces BFT quorum? We lean B1 (your original §3) on the efficiency-and-trust argument above,
   but want your ruling on the **unrecoverable-freeze risk** of per-block finality vs. B2's bounded
   recovery margin.
2. **Publish/durability coupling:** should a returned `silt:` link require **finality** (not just commit)?
   Under B1 they're the same instant; under B2/A there's a window where "committed" content is still
   reorgable. This decides whether the durability claim (a link ⇒ retrievable) is exact or hedged.
3. **Partition liveness under §2 (already live):** a minority partition now *stalls* (can't commit) — is
   that the intended safety/liveness trade (BFT: safety over liveness during partition), or do you want a
   defined recovery/degraded-mode? This is orthogonal to §3's method but part of the same B adoption, and
   it changes what the partition-heal test should assert (a supermajority commits; a minority catches up).
4. **Test/claims migration:** confirm that under finality the two red-team "heal to heavier conflicting
   fork" tests should become **slash-on-conflict** tests (the conflicting fork implies double-signing →
   equivocation → slash), not heal tests — i.e. the old assertions encode the pre-B model, not a property
   B must preserve.
5. **Minimal repro to bless the choice:** we'll extend the deterministic bootstrap-ramp sim
   (`TestForkChoiceRampCommittedChainOutweighsGenesis357`) with the chosen method's invariants
   (no-reorg-below-finalized for B1/B2; no-reorg-below-D for A) before any billable run (#6).

## Provenance
Code at HEAD (post §1/§2 merge): `core/chain/chain.go` — `heavier` (height-aware tiebreak),
`validatorSetSize`/`RequiredQuorum` (§2 stable set), `anchorWeight`/`blockWeight` (§1a), `Reconcile`
(the WSCheckpoint guard we'd generalize for B1/B2), `blocks`/`adopt`. Cloud confirm run 28c9eb6-8609
(16 pass / 0 fail / 3 gap / 1 skip; `5-convergence` durable). Prior ruling:
`research-outcome/357-fork-choice-oscillation-RESEARCH-RESPONSE.md` (§3 recommended B1 + the ordering
warning we honored: §1+§2 before §3). Owner ratified B; this consult refines *which §3 method*.
