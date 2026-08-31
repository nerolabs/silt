# v5 five whole-set digest-root leaves — increment F1 (format-only)

Date: 2026-08-31
Author: Builder
Base: `origin/main` `ad8effb`
Certified design: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/v5-wholeset-digest-root-addition-RESEARCH-CERTIFICATION-2026-08-31.md`
PE cross-check: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-v5-wholeset-digest-root-cert-crosscheck-2026-08-31.md`

## What this increment is

Add five v5-only committed MTH digest-root leaves — `bondedRoot`, `epochSetRoot`,
`qualifiedRoot`, `slashedRoot`, `validatorsSeenRoot` — to the v5 state root. Each is the
RFC-6962 MTH over the CANONICAL sorted id-list of that keyspace's member set
(membership-only). They are INERT this increment: nothing reads them until F3 wires the
root-only recompute. Same shape as the era-4 4a schema: commit the bytes now, consume
later.

The five roots close set-completeness for the five whole-set committed reads
(`Σ bonded`, `Σ epochSet`, `qualifiedCount`, `C2Metric` over `validatorsSeen`, the
`qualified` freeze). An SMT held by a root-only box can prove inclusion of members it was
given but not the completeness of the set; the MTH over the full id-list closes that gap
(cert "the decisive artifact"). This increment only COMMITS them.

## The mechanism (attribution)

The completeness gap is real *because* an SMT gives a root-only holder no
"enumerate all keys under tag T" primitive; a withholding prover hands a read-set missing
a member and every inclusion proof it hands still verifies (cert lines 82-92). This change
addresses it *by* committing, per keyspace, one scalar MTH leaf over the canonical sorted
id-list, so "recompute to this root" uniquely pins the complete member set — the same
closure `dueBucketMTH` already ships (statehash.go:224-234).

## Decisions

### 1. Tag scheme (C-7 prefix-safe)

Five new scalar tags, each `\x00`-terminated, name = keyspace + `Root`:

```
bondedRoot\x00  epochSetRoot\x00  qualifiedRoot\x00  slashedRoot\x00  validatorsSeenRoot\x00
```

`statehash.Key(tag, rawKey) = tag || rawKey` has no length delimiter (statehash.go:96-101).
The risk C-7 names: a tag that is a byte-prefix of `existingTag || rawKey`. `bondedRoot\x00`
does NOT collide with the per-member `bonded\x00 || id` leaves: after the shared `bonded`
prefix the existing tag has `\x00` (0x00) and the new tag has `R` (0x52) — they diverge
before either tag ends. Each new root is a scalar leaf at `tag || ""`; the per-member
leaves have non-empty 32-byte raw keys, so no scalar-vs-member collision either. Safe by
the chosen name, gated by the coverage guard below.

### 2. Canonical id-list ordering

Ascending by raw NodeID bytes, deduplicated (a set is already unique), unpadded — verbatim
the `dueBucketMTH` canonical order (statehash.go:224-234, RECERT2 pin). This is the ONLY
order that makes "recompute to this root" uniquely identify the set; a map-order encoding
would be malleable. Factor a shared `nodeSetMTH(ids)` helper and reuse it for both the five
new roots and (unchanged behaviour) `dueBucketMTH`.

### 3. C-4 always-emit (hard requirement, not optimization)

Emit all five on every v5 block. An empty keyspace yields `translog.MTH(nil)`, the fixed
empty-MTH constant (confirmed byte-identical to `MTH([])`). NO absent-vs-empty shortcut: a
cold root-only box cannot distinguish "keyspace empty" from "leaf omitted" without an
always-present leaf (cert C-4, lines 230-239). `slashed` and `validatorsSeen` start empty
on a fresh chain, so this is not hypothetical.

### 4. Additive / v5-gated (immutable #632)

Append the five leaves in `stateRootLeavesV5` ONLY, AFTER the untouched era-3 leaves and
after the existing v5 leaves. `stateRootLeaves` (the 18 era-3 leaves) is untouched.
`StateRootForVersion` already gates v4→era-3 vs v5→era-4, so a v4/era-3 root stays
byte-identical. This is the load-bearing immutable proof (ablation 1).

Note on the `bonded`/`epochSet`/`slashed`/`validatorsSeen` keyspaces: their per-member
leaves are already in the ERA-3 leaf set (`stateRootLeaves`). The digest roots are NOT.
Only `qualified` is a v5-only keyspace. So four of the five keyspaces have their members in
the v4 root, but their DIGEST roots are v5-only — which is exactly right: the digest is the
completeness commitment the root-only box needs, and it must not perturb the frozen v4
root.

### 5. Coverage guard (C-7 binding)

The five digest roots are NOT committedSet fields — they are DERIVED digests over existing
keyspaces. So they do NOT go in `stateRootTagsV5` (which binds to the reflected
classification and would then report five "extra" tags, reddening
`TestStateRootV5CoversExactlyTheV5Fields`). Instead add a dedicated list
`stateRootDigestTagsV5` and a new EMIT guard that fails if any of the five is dropped from
`stateRootLeavesV5`. Matched by the `tag\x00` key prefix so a rename cannot mask a drop.

## STOP boundary

No validity predicate reads these roots. That is F3. This increment commits them
inert-by-non-consumption. If a fold/recompute were added here, STOP — out of scope,
re-triggers the recompute certification (C-1/C-6/R-1/R-4 are all F3 build obligations,
not F1).

## Ablations (red-before-green)

1. **v4 byte-identical** — a v4 root is UNCHANGED by this addition (the immutable proof).
   Extend `TestEra3RootByteIdenticalWithV5KeyspacesPresent` so the digest roots must not
   leak into the v4 path.
2. **Each root load-bearing** — perturb a member of each keyspace → its digest root
   changes. New test per keyspace.
3. **Empty keyspace → empty-MTH constant** (C-4). New test.
4. **Coverage guard forces the five tags** — drop one from the marshaller → the new EMIT
   guard reddens.
