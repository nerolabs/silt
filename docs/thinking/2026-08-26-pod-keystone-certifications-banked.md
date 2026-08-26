# Both Phase 4 gate consults certified — what changed, what builds next

**Date:** 2026-08-26 (same session as the spec). Research answered both open
consults:

- PoD neutral lane (Q1–Q5):
  `silt-reviews/research/research-outcome/PoD-neutral-lane-B3-close-RESEARCH-CERTIFICATION-2026-08-26.md`
- D-TIERING state-root keystone (Q1–Q7):
  `silt-reviews/research/research-outcome/D-TIERING-state-root-keystone-RESEARCH-CERTIFICATION-2026-08-26.md`

This note records what the certifications *changed* relative to the drafts, and
the build sequencing decision. The full verdicts live in the certifications;
the spec amendments are marked **[CERT]** in [`design/pod.md`](../design/pod.md).

## What the PoD certification changed (three amendments, all folded in)

1. **The supersede rule graduated from "reconciliation question" to
   load-bearing correction.** Research identified the existing per-byte serve
   credit (`RecordServe`) as an *unfunded self-mint* — the exact
   "network-minted per-receipt subsidy" the spec bans. The spec's "additive"
   framing under-stated this: wiring the witnessed consumer without the
   supersede rule double-pays and violates conservation. This is the one
   non-additive obligation, and it lands before the firewall test can mean
   what it claims.
2. **The PoR leg is dropped, not kept as belt-and-suspenders.** The
   certification's reasoning is sharper than the spec's: a forgeable belt is
   not a belt, and it costs 128 Shacham–Waters samples per delivery on the
   floor box — pure M1 cost for zero security. The neutral receipt is token +
   fetcher signature + binding.
3. **The strong-form route changed.** The desk study (Q4) came back harder
   than "no library": the only pure-Go Camenisch–Shoup is archived and
   unaudited, and the certified alternative is the quorum-TTP/VSS path silt
   already frames in `fairexchange.go` — a committee-trust design choice
   instead of a new hardness assumption. This retires the roadmap's largest
   unknown-unknown with a concrete answer.

Also settled: relay compensation has a literature-settled shape (sender-funded
incremental micropayment; no transit proof exists to buy; TTP-free atomic
fairness is proven impossible), and per-node settlement suffices for the
neutral lane's bilateral tit-for-tat.

## What the keystone certification settled

Compact SMT (determinism is the consensus-grade discriminator — a
snapshot-booted and a replay-booted validator must compute the identical
root), one root over all 16 fields (re-derivation rejected as a relocated
#357 hazard), era-3 activation as tenant #2 of the built #506 version-gate,
rebuild-tree-at-boot preferred (the tree is a derived cache; the chain wins),
and the Q7 self-checkpoint ruling that closes #559's common crash-reboot
case. The three load-bearing obligations are named in the certification and
now in the ROADMAP: field-completeness proven by the snapshot-boot-equivalence
oracle (inspection already missed fields), the incremental-cost oracle
(the #555 discipline), and the era-2→3 Reload test shipping ahead of the
change (the #558 discipline).

## Sequencing decision

**PoD neutral-lane build first, keystone second.** Both are certified; the
PoD build is small (a consumer + a supersede rule + a firewall test, on seams
verified additive), while the keystone is a consensus-rule change with three
oracle obligations and a library-selection call — a multi-session track. The
PoD build also exercises the balance lane the keystone's Q5 answer
(coarse-granularity committed balances) will later need field data from.
ROADMAP order updated accordingly (items 2 and 3).

## Owner knobs deliberately held (named, not decided)

- Skim routing: burn (airtight deterrent) vs escrow (consistent with the
  existing serve skim; research leans escrow). Conservation is sound either
  way.
- The relay dispute-TTP question: accepting a dispute-only quorum-TTP vs
  refusing any TTP and eating the irreducible one-increment stiffing
  residual. Deferred with relay compensation itself.
- Keystone: lifetime-owner vs TTL-lapse policy for `bondRootOwner` (both
  C2-sound; TTL-lapse bounds the snapshot), and rebuild-vs-persist (decided
  by measurement on the floor box, not by preference).
