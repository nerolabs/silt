# The keystone SMT library call — answered, and it corrects the certification's caveat

**Date:** 2026-08-26. **The question:** the state-root certification left the SMT
library choice as a builder's call, gated on one verification:

> `pokt-network/smt` — the leading candidate: maintained (2025), **audited**
> (Thesis Defense 2024-06), pure-Go, SHA-256, in production (Pocket Network
> mainnet). **Caveat to confirm before adopting:** its exclusion is
> *closest-proof* (present nearest key ≠ yours), a **different verifier shape**
> than the textbook empty-subtree absence — the builder must verify that shape
> supports silt's exclusion consumers (a specific-key-absent proof, and the
> sharded omission proof). If closest-proof fits, this is the adopt-audited
> choice.

## The finding: the caveat's premise is wrong, in silt's favour

**`pokt-network/smt` has BOTH mechanisms, and they are separate.** The library
does not force silt through the closest-proof shape:

1. **Standard non-membership**, which is what silt needs, rides the ordinary
   `SparseMerkleProof` / `VerifyProof(proof, root, key, value, spec)` path with
   an **empty value**. That is a specific-key-absent proof, bound to the queried
   key.
2. **`SparseMerkleClosestProof`** is an *additional, separate* type with its own
   verifier (`VerifyClosestProof(proof, root, spec)`). It proves *which* key
   lies nearest a target — Pocket uses it for relay-mining sampling. It is not
   the library's exclusion mechanism, and silt need not touch it.

The certification appears to have read the closest-proof feature as *the*
exclusion path. It isn't.

## The evidence (source, quoted — not the README)

From `proofs.go`, the verifier's non-membership branch:

```go
if bytes.Equal(value, defaultEmptyValue) {
    // Non-membership proof if `value` is empty.
```

The position is either an empty placeholder or an unrelated leaf:

```go
if proof.NonMembershipLeafData == nil {
    currentHash = spec.placeholder()          // empty subtree
} else {
    actualPath, valueHash = spec.parseLeafNode(proof.NonMembershipLeafData)
```

And — the load-bearing line — **the queried key's path is checked against the
leaf actually found there**:

```go
if bytes.Equal(actualPath, path) {
    // This is not an unrelated leaf; non-membership proof failed.
    return false, nil, errors.Join(ErrBadProof,
        errors.New("non-membership proof on related leaf"))
}
```

So a proof that "key K is absent" cannot be forged for a key that is in fact
present: if a leaf sits at K's path, verification rejects. The root is then
recomputed up the sibling path and compared against the trusted root, exactly as
an inclusion proof is. This is textbook empty-subtree/unrelated-leaf absence —
the shape the certification wanted.

## What this unblocks, per consumer

| silt exclusion consumer | Served by |
|---|---|
| Duplicate-publish reject over `byRoot` ("root X is not committed") | `VerifyProof` with empty value at `H(field-tag‖root)` |
| Serial-replay reject over `spent` ("this serial was never spent") | same |
| Sharded registry, can't-lie-by-omission ("not in my slice") | same, per queried key — the slice-holder cannot deny a key it cannot produce an absence proof for |

The third is the one the certification called "the load-bearing security
question" for sharding (§11.2, still its own consult). A key-bound absence proof
is the right primitive for it; the open part is the *protocol* around which keys
a slice-holder is obliged to answer, not the proof shape.

## Recommendation

**Adopt `pokt-network/smt`** — audited, maintained, pure-Go, in production, and
its exclusion shape fits silt's consumers directly. The alternative the
certification named (a JMT port from Rust) carries a real build cost under
build-immutable #8 and buys nothing that this library lacks.

## Gate before the dependency lands (do not skip)

This finding is **documentary evidence — quoted source, not executed code.**
Build-immutable #7 wants the artifact, so adoption is gated on a local spike,
which is the keystone build's first task:

1. Add the dependency behind a spike test only.
2. Prove a **specific key absent** against a root: build a tree with keys A and
   B, then verify absence of C — and assert that an absence proof for A
   **fails**. The second half is the one that matters; a library that "passes"
   absence for a present key would be silently unsound for every consumer above.
3. Measure produce + verify on the floor box (build-immutable #8) before
   committing to it — the certification's Q4 obligation.

If step 2 does not behave as the quoted source says, this recommendation is void
and the JMT port returns to the table.

## Also worth carrying into the build

The certification's own implementation note stands and is independent of the
library: **hash the key into the leaf position (`H(field-tag ‖ key)`), never use
a content root directly as the position** — silt's keys are adversary-influenced
(a `byRoot` key is `H(content)`), so an unhashed position lets an attacker grind
content to force pathological depth.
