---
name: era4-4a-schema-shipped
description: era-4 increment 4a (schema + tag reservation) built; the stateRootTags trap that makes the doc §6 "add to stateRootTags" line un-shippable inertly; where classification actually keys.
metadata:
  type: project
---

era-4 increment **4a** built on branch `era4-4a-schema-classification`, commit `7241f82`
(off `origin/main` @ `0984db4`). Routed to blind PE + Tester at that hash.

**What shipped (exactly 4a):** `BlockVersionWitnessable = 5` const (`core/chain/chain.go`);
three reserved tag strings `tagDueBucket`/`tagQualified`/`tagEpochStart`
(`core/chain/statehash.go`, in the tag const block, NOT in `stateRootTags`, NOT emitted);
CHANGELOG line + regenerated `website/changelog.html`. No struct fields, no apply(), no
predicate, no versionSupported widen, no leaf emission. Inert: no v4 root changes.

**Why:** the maintenance spine (`qualified`/due-bucket maps + hooks) and the v5 predicate
are 4b/4c. 4a is the deliberately-thin schema PR so the tag table is clean before the spine
lands. See [[era4-ratification-and-build-order]].

## THE 4a TRAP — the doc §6 line is un-shippable inertly (evidence-backed)

The build-decomposition doc §6 says "add the three tags to `stateRootTags`." **That cannot
be done in 4a without shipping red.** The coverage guards
(`core/chain/modelcheck_stateroot_determinism_test.go`) enforce that `stateRootTags`
EXACTLY equals `fieldsOfKind(committedSet)` (BOTH directions) AND that every tag emits a
populated leaf. `fieldsOfKind` reflects over LIVE Chain struct fields. So:
- Adding a tag to `stateRootTags` with no matching committedSet struct field → RED
  `extra`/`missing` (demonstrated: `TestStateRootCoversExactlyTheCommittedSetFields` +
  `TestStateRootEmitsALeafForEveryCommittedField` both reddened on injecting `"qualified"`).
- Struct fields + leaf loops are the HARD-boundary 4b work. So the tag→`stateRootTags`
  wiring MUST wait for 4b — that is the only point classification goes red the correct way.

**Where classification actually keys:** `TestStateFieldsAreClassified`
(`modelcheck_state_completeness_test.go`) reflects over `Chain{}` STRUCT FIELDS and checks
each has a `stateClass` entry. It reddens on an unclassified STRUCT FIELD, not on a tag.
4a adds no struct fields, so this guard cannot be the 4a gate the way the doc implies. I
demonstrated its red by injecting a `qualified map[...]` struct field (no stateClass entry)
→ "Chain has 1 unclassified field(s): [qualified]". Reverted.

**Stale doc line refs:** doc §6 pointed at `core/statehash/statehash.go:39-40/:57` for the
tags — WRONG file. Tags live in `core/chain/statehash.go` (the marshaller that reads
unexported Chain fields). `core/statehash/` is the SMT-mechanics package with no chain dep.

**Consequence for 4b:** 4b adds the struct fields + `stateClass` entries + `stateRootTags`
entries + leaf loops together, v5-gated so era-3 roots stay byte-identical. That is the
first point all four guards can be satisfied simultaneously (and the point their ablations
go red correctly).
