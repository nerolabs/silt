# Keystone era-4 leave-one-out probes: `qualified` and `dueBucket` (2026-08-30)

## What and why

Extend the keystone leave-one-out (LOO) snapshot oracle to cover the two era-4
(v5) committed state fields deferred by `probeUncovered`:

- `qualified` — the live qualified-set keyspace (E-2).
- `dueBucket` — the TTL due-height buckets (T-3).

They were deferred because in 4b there was no v5 validity predicate for a
snapshot omission to flip; a snapshot probe would have been vacuous. Build steps
4c (#640, the committed-root predicate) and 4d (#641, activation) landed the v5
committed-root check, so the probes are now buildable and a real validity verdict
flips on the field's absence.

## Mechanism (the attribution)

The v5 committed-root predicate is `validateEra3Roots` (`era3validity.go:114`).
For a v5 block it recomputes `StateRootForVersion(5)` (`era3validity.go:152`) over
the post-apply committed set via `stateRootLeavesV5` (`statehash.go:182`), which
appends the `qualified` leaves (`statehash.go:190`) and `dueBucket` leaves
(`statehash.go:199`) to the 18 era-3 leaves. If the block's committed `StateRoot`
≠ the recompute, it is rejected with `ErrEra3StateRootMismatch`.

The LOO flip is a wrong-ACCEPT, not a panic:

- An honest full node commits a v5 block whose `StateRoot` is the SMT over the
  COMPLETE post-apply set (qualified/dueBucket included).
- An attacker forges a v5 block whose `StateRoot` was computed over a set that
  OMITS the field's leaves.
- An honest validator recomputes WITH the field present → roots disagree → reject.
- A snapshot-booted validator that LOST the field recomputes WITHOUT those leaves
  → its recompute equals the forged root → it ACCEPTS the forged block the honest
  node rejects.

This is `claim-succeeded`: the field's absence lets a wrong-rooted block through.
It obeys the hard scar — model omission as an EMPTY map that lets the claim
succeed wrongly, never a nil-map PANIC.

## Construction (isolation)

`v5RootWorld` is an epochless (no `rotateEpoch` to read `qualified`), TTL-enabled
(so `dueBucket` is live) genesis with four bonds ≥ MinBond. Every bond qualifies,
so `qualified` has four entries and each id sits in a due-height bucket.

`forgedV5Block(c, omit)` builds the attacker's block: an entry-only v5 block at
height 1 (no regs/slashes, so `apply()` runs no qualified/dueBucket maintenance;
TTL=40 so height-1 sweeps nothing; epochs off so no boundary freeze). Its
`StateRoot` is the field-less recompute (`snapshotBoot(c, omit)` then
`postApplyRoots`), byte-identical to what the ablated snapshot recomputes. A
vacuity guard asserts the field-less root ≠ the field-present root, so a world
that failed to populate the field reddens rather than passing a no-op probe.

The two probes drive `validateEra3Roots(forged)` directly — the same
ValidateCommit-free entry point the set-valued probes use (they call
`ValidateEntry` directly because a snapshot-booted node has no head to extend).

## STOP boundary

Oracle/test coverage only. No validity predicate added, no committed format
changed, no consensus rule touched — the probes drive the EXISTING 4c predicate.
Not a gated change.

## Ablation evidence (each probe injected RED, watched, restored GREEN)

- Present: `[v5-qualified-root] omitting qualified → full=reject ablated=accept`
  and `[v5-duebucket-root] omitting dueBucket → full=reject ablated=accept`
  (`..._test.go:2091` log lines). The wrong-ACCEPT flip.
- Injected defect (forge over the field-less set on BOTH sides → field-less root
  == field-present root): the vacuity guard reddens at `..._test.go:1876`
  ("omitting <field> changes no leaf … the probe is vacuous"). Restored → green.
- Neuter guard (`TestNeuteringAnyProbeBreaksCompleteness`) passes: each probe is
  the SOLE discriminator for its field.

## Discipline followed

Ran the full local gate set before declaring green (a fresh scar: `go test
./core/...` alone missed static gates): `go build ./...`, `go test ./core/...
./internal/...`, `go vet ./...`, `gofmt -l .`, plus the depcheck/wanguard internal
gates. `_test.go`-only change still gets a CHANGELOG line.
