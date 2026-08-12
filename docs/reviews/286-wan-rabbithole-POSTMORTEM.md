# Post-mortem — the #286 WAN rabbit-hole (2026-08-12)

**Status:** the #286 *compute-layer* thread is **CLOSED** (fix confirmed on GCP,
issue closed). This post-mortem exists because closing the bug is not the same as
learning the lesson: the *way* #286 was worked violated the very build-immutables
(#5, then #6) that were being ratified mid-thrash. Written in response to the
principal-engineer rescue audit (`silt-reviews/principle-engineer/RESCUE-AUDIT.md`),
whose one-paragraph verdict was correct: *"You don't have a broken system. You have
a stuck loop and two public overclaims."*

This is a process post-mortem, not a code one. The code fix that mattered was small
and correct (PR #341). The cost was in how many billable cloud runs it took to find
it, and in what did **not** get built while the loop was stuck.

---

## What #286 actually was

A fresh 4-validator, quorum-2, three-region objective chain would not commit its
genesis block on real GCP WAN, while the identical block committed in seconds in
every in-process test and on a single-zone smoke. The chain wedged at height 0.

The **true root cause**, found last, was a compute-layer scaling bug: `manifest.Prove`
was O(n), not O(log n) — `merkle.go auditPath` recomputed subtree hashes on *every*
call, so per-challenge bond answering (`AnswerSpaceTime`, every 30s) and per-propose
`RegisterBondReg` (rebuilt on every propose retry) were O(k·n) and grew with plot
size. On a 2-vCPU e2-small at a 64M bond this saturated both cores and starved the
consensus gather — the loop went silent for 5+ minutes while burning CPU, and every
bond challenge logged `late=true`. **The fix (PR #341): a precomputed `manifest.Tree`
cached on the bond Commitment → O(log n) proofs, byte-identical root/proofs,
C1-neutral.** Measured: a 64 MiB answer went 743ms → ~8ms (~95×), flat across sizes.
Confirmed on GCP re-cert (run d852fe5-21258): genesis committed in 17s, all four
validators at an identical head hash, loop idle.

That is a clean, understandable, one-file fix. The problem is everything that
happened *before* we understood it.

---

## The rabbit-hole (the honest timeline)

#286 was re-diagnosed across **five-plus "layers," each discovered by a fresh billable
cloud run**, over ~11 commits:

| "Layer" | The theory at the time | How it was found | Verdict |
|---|---|---|---|
| L1 | flat publish-path deadlines guillotine the gather | GCP run | real but **not** the binding cause (#328, async publish — a valid fix) |
| — | raise the 2s dial → 10s handshake budget | *guessed* from an EOF | **wrong** (reverted; the EOFs were µs teardowns, #332) |
| L2a | genesis address non-convergence (no persistent peers) | GCP `-log debug` run | real, fixed (#331 persistent-peers) — but genesis **still** wedged |
| L2b | ~8 MB genesis block (all bond regs piled in) un-gatherable over WAN | *another* GCP run to *discover* the block size | real, fixed (#336) — deterministic and **reproducible in-process**, so the billable run was wasted |
| size-aware deadline | a payload-scaled transport deadline is the fix | shipped (#318, `0ffcbe6`) | a **valid durability improvement that did NOT fix #286** (#326) — a billable re-run spent to disprove it |
| compute | per-challenge answering is size-independent; only the one-time `Seal()` starves | routed to research on this premise (`377958e`) | **premise was false** — the O(n) Merkle proof was the starver; research corrected (#342), fix shipped (#341) |

Two anti-patterns recur down that column, and both are now named immutables:

1. **A billable run was spent to *discover* a cause, not to *confirm* an
   understood one.** L2b (the 8 MB block) and the size-aware-deadline disproof were
   both reproducible/deterministic *in-process* — they cost a GCP run to learn what a
   laptop could have shown for free. This is the exact violation build-immutable **#6**
   was written to forbid — and #6 was written *during* this thrash.
2. **A knob was moved before the mechanism was named.** The 2s→10s dial-timeout
   guess, and the size-aware deadline shipped as "the fix," both preceded a
   log/trace/test that named the failure mechanism. This is build-immutable **#5**
   (adverse-internet; consult `network-durability.md` before inventing a timeout) —
   also written mid-thrash.

The tell was visible the whole time and we missed it: **the buildlog went silent
after 2026-07-26.** A subsystem eating dozens of commits in a week with no narrative
entry *is* the signature of a rabbit-hole. The forward V1 spine (H8 privacy, H9
takedown, D-DEMAND, C2 wiring) took **zero** commits across the stretch.

---

## What was salvaged (the thread was not worthless)

The saga did produce durable assets — they just cost far more than they should have:

- **PR #341** — the real O(log n) bond-proof fix. Keep.
- **PR #343** — the storage-plane sibling of the same O(n) Merkle bug (per-shard
  proofs were O(shards·n)). Keep.
- **#328** async publish, **#331** persistent-peers, **#336** genesis bond-reg drain
  — each a real, independently-valid durability fix.
- **Build-immutables #5 and #6** + `docs/network-durability.md` + `docs/build-process.md`
  — the disciplines that forbid the anti-pattern, now canon.
- **`internal/wanguard`** — an AST build-guard that red-builds any new un-ledgered
  transport deadline, so the *next* magic constant is caught at build time.

The rescue's fair criticism: the guardrails **arrived after the horse left**. The
right sequencing is to have them *before* the fifth billable run, not as its scar
tissue.

---

## What changes now (the corrective)

Per the rescue's sequenced steps, and to make the disciplines *executable* rather
than aspirational:

1. **The #286 compute thread is declared CLOSED.** No further cloud run is warranted
   to re-examine it — the mechanism is understood, the fix is confirmed, the
   regression guard (`TestTreeMatchesStandaloneProve`) is in place.
2. **Remaining WAN items are consolidated into one tracking issue with a single
   explicit exit criterion** (#360), so "WAN work" stops being an open-ended
   loop and becomes a gated deliverable. Its exit criterion: one clean warm
   multi-region cloud run grades the full suite green end-to-end, once — with
   adversarial drills certified deterministically off-cloud, and #357 (fork-choice
   oscillation, research-gated) named as its blocking dependency.
3. **Adversarial-consensus certification moves OFF the flaky live-cloud wire onto a
   deterministic local netem harness** that can *force* the attack
   (`integration/adversarial`, reusing the `e2e/` drivers under `tc netem`). An attack
   you cannot schedule is not a test; an undriveable attack is a RED, never a passing
   GAP.
4. **A pre-run gate refuses a billable cloud run** without a written #6 mechanism
   paragraph and a named local repro (`integration/cloudtest` preflight). An expensive
   run *confirms*; it never *discovers*.
5. **The buildlog is treated as a gate, not a nicety** — this post-mortem ships with a
   dated buildlog entry, breaking the silence that was the rabbit-hole's signature.

The one clean warm cloud run — for liveness/timing at scale, the thing a real WAN
uniquely proves — happens **only after** the above, as the R1 gate, not as the place
causes get discovered.

---

## The one-line lesson

**A billable multi-region run confirms an already-understood, locally-reproduced fix.
It never discovers a cause and never tests a guess. When the buildlog goes quiet under
a pile of cloud commits, the loop is stuck — stop, instrument, reduce to a laptop
repro, and if the mechanism is unknown or it touches consensus/a claim, consult
research before spending the run.**

*Refs: #286, #318/#326, #328, #331, #336, #341/#342, #343; build-immutables #5/#6
(TENETS Part IX); `docs/network-durability.md`, `docs/build-process.md`;
rescue audit `silt-reviews/principle-engineer/RESCUE-AUDIT.md`.*
