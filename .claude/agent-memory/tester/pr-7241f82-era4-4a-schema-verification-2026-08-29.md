---
name: pr-7241f82-era4-4a-schema-verification-2026-08-29
description: era-4 4a schema commit 7241f82: PROMOTED — build green, all 6 inertness guards GREEN, ablation RED confirmed (2 guards fired independently)
metadata:
  type: project
---

# era-4 increment 4a — `7241f82` verification (2026-08-29)

**Branch:** `era4-4a-schema-classification` (off `origin/main @ 0984db4`)
**Verdict: PROMOTED.** All checks passed. Ablation RED independently confirmed.

## (0) HEAD confirmed
`7241f8263fdf4d8ff6ea42a8d87492fbf9709112` — detached HEAD on the target commit.
Working tree clean for `core/` after ablation revert (diff showed only builder/planner memory files, not source).

## (1) Build + suite

- `go build ./...` — exit 0, no output.
- `go vet ./core/chain/` — exit 0, no output.
- `go test ./core/chain/ ./core/statehash/ -count=1` — both packages GREEN.
  - `core/chain`: `ok` in 6.472s
  - `core/statehash`: `ok` in 0.597s

## (2) Inertness verdict — guards named and confirmed GREEN

Six guards explicitly run and passed:

| Guard | Package | Result |
|---|---|---|
| `TestStateFieldsAreClassified` | core/chain | PASS |
| `TestStateRootCoversExactlyTheCommittedSetFields` | core/chain | PASS |
| `TestStateRootEmitsALeafForEveryCommittedField` | core/chain | PASS |
| `TestEra2BlockHashesByteIdenticalAfter2a` | core/chain | PASS |
| `TestEra2GoldenHashUnchanged` | core/chain | PASS |
| `TestEra3RootsAreAttesterSigned` | core/chain | PASS |

The three v5 reserved tags (`tagDueBucket`, `tagQualified`, `tagEpochStart`) are defined as
string constants in `core/chain/statehash.go:68-70` but are NOT in `stateRootTags` (lines 77-83)
and are NOT in the `stateClass` classification map. The guards confirm this gap is the correct
inert state: no leaf is emitted, no struct field is claimed.

Direct byte-identity comparison across commits was not run (requires two checkouts simultaneously
and a deterministic input fixture; the era-2/era-3 golden-hash guards are the functional equivalent
and they passed).

## (3) Ablation — demonstrated RED

**Injection:** appended `"dueBucket"` to `stateRootTags` in `core/chain/statehash.go` (a reserved
v5 tag with no struct field or classification entry). Reverted from backup afterward.

**Red output (exit 1):**
```
--- FAIL: TestStateRootCoversExactlyTheCommittedSetFields (0.00s)
    modelcheck_stateroot_determinism_test.go:58: StateRoot commits tag(s) that are NOT classified committedSet: [dueBucket]
        Either the field was reclassified (drop its tag) or the tag is stale.
--- FAIL: TestStateRootEmitsALeafForEveryCommittedField (0.00s)
    modelcheck_stateroot_determinism_test.go:104: stateRootLeaves emitted NO leaf for committedSet field(s): [dueBucket]
        The field's tag is in stateRootTags but its leaf loop is absent from stateRootLeaves — ...
FAIL    github.com/nerolabs/silt/core/chain    0.335s
```

Two guards fired independently on the same injection. `TestStateFieldsAreClassified` did NOT fire
(no struct field was added — correct, the injected defect was tag-only).

**Revert + green-restored:**
- Restored from backup copy in scratchpad.
- Rerun of all three guards: `ok` 0.195s. Exit 0.
- `git diff HEAD -- core/` empty — tree clean.

## (4) Anomalies

None. The commit is exactly what it claims: a schema reservation that cannot silently enter the
committed state root without reddening at least two independent guards.

**Why:** `stateRootTags` (coverage) and the `stateClass` map (classification) are SEPARATE data
structures that the guards cross-bind. Adding a tag to one without the other fires; adding a
struct field without the classification fires `TestStateFieldsAreClassified`. All three entry
points are guarded.
