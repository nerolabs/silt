# SMT second-preimage / domain-separation close — design (Boulder 3, R3.1 / audit finding B2)

Status: DESIGN / ADVISORY (crypto-specialist seat). No code changed. The Researcher certifies;
this seat advises. Build-day plan only.

Date: 2026-09-01

## The residual, precisely

silt commits its era-3 validity state as a single `pokt-network/smt` v1.0.0 SMT root
(`core/statehash/statehash.go`). The fold-based floor box (`core/statehash/fold.go`) and the
witness accessor (`core/statehash/witness.go`) both re-derive that root from proofs and require
the derived root to equal the quorum-committed `StateRoot`. The whole safety spine — the
fold's `OldValue` soundness, the accessor's `PROVEN_ABSENT`-only-from-a-verified-proof invariant
— rests on one assumption: **an attacker cannot construct a proof that verifies against the
committed root for a `(key, value)` pair that is not the committed one.**

The `pokt-network/smt` audit (Thesis Defense, initial 2024-02-20, final 2024-06-12, shipped in
the module cache at
`.../smt@v1.0.0/audits/240612_Thesis_Defense-Pokt_Network_Sparse_Merkel_Tree_Security_Audit_Report.pdf`)
addresses exactly this class in its "Security by Design" section, subsection **"Protection
Against Second Preimage Attacks and Shortened Proof Attacks"** (audit PDF page 5). Its two
statements, verbatim in substance:

1. Second-preimage: "Currently, the SMT uses node-type specific prefixes (0 for leaf nodes, 1
   for inner nodes, and 2 for extension nodes) to protect against second preimage attacks, which
   is a valid strategy commonly utilized in SMT implementations."
2. Shortened-proof: "Using a different hash function for hashing leaves than for hashing internal
   nodes helps mitigate the risk of both attacks. **We recommend the POKT Network team explore
   utilizing such a strategy** to further enforce domain separation, which provides protection
   against both second preimage and shortened proof attacks."

The residual is #2: the auditors **recommended but did not confirm** the stronger
different-hash-per-node-type domain separation. The library ships the prefix-byte scheme only.
silt inherited this UNOWNED. This document determines whether the prefix scheme closes the
attack **for silt's exact leaf encoding**, and specifies the close before the era-4/v5 freeze (a
hard-fork boundary where changing the hashing scheme later becomes expensive).

## Part 1 — Does the node-prefix scheme close shortened-proof for silt's leaves?

### What a shortened-proof / second-preimage attack actually is here

Every node digest in the tree is `SHA256(preimage)`. The attacker's goal is to find a single
preimage (or a short proof whose recomputed nodes collide) that the verifier will accept as
proving a `(key, value)` that was never committed. Two sub-cases:

- **Type-confusion (the "shortened proof"):** feed the verifier a preimage that it interprets as
  a *leaf* while the honest tree hashed the *same bytes* as an *inner node* (or an extension
  node), or vice versa. If leaf-bytes and inner-bytes can ever be equal, `SHA256` gives them the
  same digest, and a proof path can be truncated/reinterpreted so a node that is really an inner
  node is accepted as the leaf being proven (a "shortened" proof).
- **Cross-structure second-preimage:** find any two distinct node preimages with the same digest.
  With `SHA256` this reduces to a generic collision unless structure forbids it; the defense is
  to make the preimage byte-strings of the two node types *disjoint sets*, so no single preimage
  is a valid member of both.

The prefix byte is the defense against type-confusion. The question is whether it is airtight
**given silt's encoding**, or whether a residual length/parse ambiguity remains.

### The library's exact encodings (primary source, module cache)

From `.../smt@v1.0.0/node_encoders.go`:

```
leafNodePrefix  = []byte{0}    (line 23)
innerNodePrefix = []byte{1}    (line 24)
extNodePrefix   = []byte{2}    (line 25)

encodeLeafNode(path, leafData)      = 0x00 || path || leafData          (lines 57-62)
encodeInnerNode(leftData, rightData)= 0x01 || leftData || rightData     (lines 65-70)
encodeExtensionNode(bounds, path, child) = 0x02 || bounds(2) || path || child  (lines 73-79)
```

Digest of every node = `SHA256(preimage)` (`hasher.go:103` `digestData`, called from
`digestLeafNode`/`digestInnerNode` at `hasher.go:111,118`). Verification recomputes:

- leaf digest at `proofs.go:424,429` via `spec.digestLeaf` → `digestLeafNode`;
- inner digest at `proofs.go:442,444` via `spec.digestInnerNode` → `digestInnerNode`.

### silt's exact leaf encoding pins the widths — this is the load-bearing fact

silt builds a **non-sum SHA-256 trie with the default value hasher**:
`smt.NewSparseMerkleTrie(store, sha256.New())` (`statehash.go:123`), and verifies under the
matching `smt.NewTrieSpec(sha256.New(), false)` (`witness.go:174`). Confirmed: the default spec
sets `vh = valueHasher` (`trie_spec.go:25`, `NewSparseMerkleTrie` at `smt.go:37` with no
`WithValueHasher(nil)` override). Therefore, in every silt leaf:

- `path` = `SHA256(key)` → **exactly 32 bytes** (`hasher.go:69` `Path`).
- `leafData` = `valueHash` = `SHA256(value)` → **exactly 32 bytes** (`hasher.go:81` `HashValue`,
  invoked via `spec.valueHash` at `proofs.go:428`). silt never passes raw values into the leaf
  slot; the value hasher is always applied.

So silt's node preimages have fixed shapes:

| node   | preimage bytes                                    | length | byte[0] |
|--------|---------------------------------------------------|--------|---------|
| leaf   | `0x00 ‖ path(32) ‖ valueHash(32)`                 | 65     | `0x00`  |
| inner  | `0x01 ‖ leftDigest(32) ‖ rightDigest(32)`         | 65     | `0x01`  |
| ext    | `0x02 ‖ bounds(2) ‖ path(32) ‖ childDigest(32)`   | 67     | `0x02`  |

(fold.go:50-55 documents the same shapes and pins them byte-exact by
`TestFoldSeedEncodingMatchesLibrary`.)

### The verdict: the prefix scheme DOES close type-confusion for silt's leaves

Leaf and inner preimages are **the same length (65 bytes) but differ in byte[0]** (`0x00` vs
`0x01`). Because the leading byte differs, the two preimage byte-strings are **disjoint sets**:
no byte-string is simultaneously a valid leaf preimage and a valid inner preimage. Under
`SHA256`, disjoint preimage sets mean a type-confusion (shortened-proof) forgery requires a
genuine `SHA256` collision across the type boundary, i.e. it is no easier than a generic
second-preimage on `SHA256` (256-bit, out of reach). The extension node is also disjoint (byte[0]
= `0x02`, and length 67 ≠ 65).

The audit's *stronger* recommendation — a **different hash function** per node type — buys
domain separation even if an implementation had **variable-length or overlapping** preimages,
where a prefix byte alone could be defeated by a length-extension or a parse re-alignment. That
residual attack surface does not exist for silt because:

1. The prefix byte is present and distinct per type (library-enforced, `node_encoders.go:32-38`
   `init()` panics if any prefix length drifts from 1).
2. **silt's widths are fixed** (path 32, valueHash 32) — the verifier's `parseLeafNode`
   (`trie_spec.go:239-246`) slices `path = data[1:33]`, `value = data[33:]` at fixed offsets, and
   the inner parser (`hasher.go:149-153`) slices at fixed `hashSize` offsets. There is no
   length field an attacker can lie about to make one type's bytes re-parse as another's.
3. The verifier recomputes the digest bottom-up (`proofs.go:437-450`) with the type chosen by
   *position in the proof* (leaf at the bottom, inners above), and each recomputation re-applies
   the correct prefix. An attacker cannot get an inner-node preimage accepted in the leaf slot
   because the verifier always prepends `0x00` when it forms the leaf digest and `0x01` when it
   forms an inner digest — it never hashes attacker-supplied bytes without a type prefix.

**Where it does NOT hold (the honest caveats, all outside silt's committed-state path):**

- **The sum trie (SMST).** `digestSumLeafNode`/`digestSumInnerNode` (`hasher.go:125-146`) append
  sum+count meta bytes AFTER the digest and the audit's own "False Sum Proof Protection"
  discussion (page 3-4) is about a *different* forgery (weight manipulation) caught by
  `validateBasic`'s sibling-hash check. silt uses `sumTrie=false` everywhere
  (`statehash.go:123`, `witness.go:174`), so the sum-node encodings and their meta-byte parsing
  are **not on any silt path**. This must stay true; a future switch to the sum trie re-opens the
  analysis.
- **`ProveClosest` / `VerifyClosestProof` / the `nilPathHasher`.** The closest-proof path
  (`proofs.go:357-392`) swaps in a `nilPathHasher` that returns the input unchanged
  (`hasher.go:93-95`) and was the audit's primary object (Issue #3). silt uses **none of it** —
  verified by grep (Part 3). Standard `Prove`/`VerifyProof` only.
- **A writeable prover KV store (audit Issue #2).** Out of scope for the hashing residual; silt
  is immune on the verify side (pure `VerifyProof`, no KV store) and uses an in-memory
  `simplemap` on the prove side. Recorded already in the SMT dependency memo; re-check if
  `ports.NodeStore` is ever swapped to a shared backend.

## Part 2 — The CLOSE

**Recommended close: (a) CONFIRM + OWNED-RESIDUAL record. Do NOT change the hashing scheme.**

The prefix-byte scheme, combined with silt's fixed-width leaf encoding, closes type-confusion /
shortened-proof / cross-structure second-preimage for silt's committed-state leaves. A stronger
different-hash-per-type scheme would add no security for silt's fixed-width, disjoint-preimage
encoding, and would be a HASHING-scheme change that:

- forks the committed `StateRoot` for every existing block (a hard fork), and
- diverges silt from the audited, deployed library code — the exact drift this seat exists to
  prevent (silt B8: adopt the analogue's schema, do not fork it).

So the close is a documented, verified confirmation, not a code change. This is the cheaper and
more faithful path, and it respects the era-4/v5 freeze.

### What the OWNED-RESIDUAL record must contain

A short section (proposed home: `docs/design/state-root-domain-separation.md`, referenced from
`docs/design/consensus-invariants.md` near the StateRoot definition, and cross-linked from the
package doc in `statehash.go`). It must state, with the citations above:

1. The attack class (second-preimage / shortened-proof) and why it is load-bearing (a forged
   proof defeats the fold `OldValue` soundness and the `PROVEN_ABSENT` invariant).
2. The library mitigation (node-type prefixes `0x00`/`0x01`/`0x02`, `node_encoders.go:23-25`).
3. The silt-specific argument that closes it: **fixed-width (32/32) leaf and inner preimages that
   are disjoint by their distinct leading prefix byte**, so a type-confusion forgery is no easier
   than a generic SHA-256 second-preimage.
4. The scope conditions that keep the argument valid — the three "must stay true" invariants
   below. If any is violated, the residual re-opens.
5. An explicit statement that silt DECLINED the audit's different-hash-per-type recommendation,
   with the reason (no added security for fixed-width disjoint preimages; a hashing change is a
   hard fork; fidelity to the audited library).

### The three "must stay true" scope invariants (the record's teeth)

These are what the verification in Part 3 pins:

- **SI-1 (non-sum):** silt constructs and verifies with `sumTrie=false`. No SMST path.
- **SI-2 (default value hasher):** silt never passes `WithValueHasher(nil)`; the leaf value slot
  is always a 32-byte SHA-256 valueHash. (This is also what makes leaf and inner both 65 bytes;
  the argument uses the distinct prefix byte, not the length, so even a future value-hasher
  change would not by itself re-open type-confusion — but it WOULD change widths and must be
  re-reviewed.)
- **SI-3 (no closest proof):** no silt path calls `ProveClosest`, `VerifyClosestProof`,
  `SparseMerkleClosestProof`, or the `nilPathHasher`.

## Part 3 — Verification (build-day, for the Tester to own as gates)

Two mechanical checks. Both are cheap, deterministic, and belong in the statehash package test
suite + a repo grep gate. This seat can prototype them as an exploratory spike; the Tester
encodes the load-bearing versions.

### V1 — leaf-domain-byte ≠ internal-domain-byte (a compiled assertion against the library)

A unit test in `core/statehash` that reaches into the library's public encoding behavior and
asserts the domain bytes differ and the preimages are disjoint. Because `leafNodePrefix` etc. are
unexported, assert on OBSERVABLE digests, not the private vars:

- Construct a leaf digest and an inner digest over controlled inputs via the library's own spec
  (`NewTrieSpec(sha256.New(), false)`), using the exported proof/verify path, and assert:
  - the leaf preimage's first byte is `0x00` and the inner preimage's first byte is `0x01` — by
    reconstructing them exactly as `fold.go` does (`foldLeafPreimage`/`foldInnerPreimage`) and
    asserting `foldLeafPrefix[0] != foldInnerPrefix[0]`. This makes the domain-separation
    assumption a RED-on-drift check: if a library bump ever unified the prefixes,
    `TestFoldSeedEncodingMatchesLibrary` (already in `fold_test.go`) reddens because the fold
    seed would stop reconstructing the honest root, and V1 reddens on the prefix equality
    directly.
  - a constructed 65-byte string that is a valid leaf preimage is NOT accepted as an inner node
    and vice versa — i.e. a type-swapped preimage produces a different digest. (Positive control:
    inject the swap, watch it go red — per the "ablate every green check" rule.)

  Ablation (mandatory before shipping): temporarily set `foldInnerPrefix = foldLeafPrefix` in a
  local copy and confirm the test goes RED. A green V1 with no demonstrated red is decoration.

### V2 — no silt path constructs domain-mixing proofs or uses the unaudited closest-proof path

A repo grep gate (extend the existing depcheck / CI lint):

```
grep -rn --include=*.go 'ProveClosest\|VerifyClosestProof\|SparseMerkleClosestProof\|newNilPathHasher\|nilPathHasher\|WithValueHasher\|smt.NewTrieSpec' \
  core/ cmd/ adapters/ internal/ | grep -v '_test.go'
```

Expected result (verified 2026-09-01): the ONLY non-test hits are
`core/statehash/witness.go:174: smt.NewTrieSpec(sha256.New(), false)` — the standard non-sum
SHA-256 spec that MUST match `statehash.Root`'s trie. Zero hits for any closest-proof symbol,
`nilPathHasher`, or `WithValueHasher`. The gate asserts:

- exactly one `NewTrieSpec` construction site, and it is `(sha256.New(), false)` (pins SI-1);
- zero `WithValueHasher` (pins SI-2);
- zero closest-proof / `nilPathHasher` symbols (pins SI-3).

Any new hit fails CI and forces a re-run of this domain-separation analysis before merge. This is
the mechanism that keeps the OWNED-RESIDUAL record honest as the code evolves toward the freeze.

## Part 4 — Researcher cert vs owned-residual documentation

**Needs a Researcher certification (consensus-rule / security-parameter surface):**

- The finding that the prefix scheme + fixed-width encoding CLOSES the residual for silt's
  committed-state leaves. This is a security-parameter argument a proof depends on (the fold
  `OldValue` soundness and the `PROVEN_ABSENT` invariant both rest on proof unforgeability). It
  is squarely inside the research gate ("security parameters a proof depends on"). The Researcher
  should certify: (i) the disjoint-preimage argument, (ii) that the three scope invariants
  (SI-1/2/3) are the complete set of conditions the argument needs, and (iii) that declining the
  audit's different-hash recommendation is sound for silt's encoding.
- Any decision to record this as CLOSED rather than a standing residual before the era-4/v5
  freeze — because the freeze is a hard-fork boundary and locking the hashing scheme as
  "sufficient" is a published-guarantee-adjacent call.

**Owned-residual documentation (this seat advises, Builder writes, no cert needed):**

- The `docs/design/state-root-domain-separation.md` record itself (mechanism description +
  citations), once the Researcher certifies the argument.
- Cross-links from `statehash.go` package doc and `consensus-invariants.md`.

**Tester owns (load-bearing, encoded as permanent gates):**

- V1 (the prefix-inequality + type-swap unit test, with its ablation) and V2 (the grep gate),
  wired into CI. This seat's V1/V2 sketches are exploratory; the Tester grades ground truth.

## Summary of the recommendation

- **Close type = (a) CONFIRM + OWNED-RESIDUAL record.** The node-prefix scheme, plus silt's
  fixed-width (32/32) disjoint-preimage leaf encoding, closes second-preimage / shortened-proof
  for silt's committed-state leaves. No hashing change; no leaf-encoding change.
- **Decline the audit's different-hash-per-type recommendation** for silt — it adds no security
  for a fixed-width disjoint-preimage encoding and would be a hard fork that diverges from the
  audited library.
- **Scope conditions (SI-1 non-sum, SI-2 default value hasher, SI-3 no closest proof)** are the
  teeth; V1 + V2 pin them as CI gates before the era-4/v5 freeze.
- **Researcher certifies** the disjoint-preimage argument and the scope-invariant completeness;
  **Builder writes** the residual record; **Tester encodes** V1/V2.

## Primary sources cited

- `pokt-network/smt` v1.0.0, module cache
  `/Users/andrewedmond/go/pkg/mod/github.com/pokt-network/smt@v1.0.0/`:
  `node_encoders.go` (encodings + prefix `init()` guard), `hasher.go` (digest/path/value
  hashers), `trie_spec.go` (parseLeafNode, valueHash, digestLeaf/InnerNode), `proofs.go`
  (VerifyProof, verifyProofWithUpdates, the closest-proof path), `smt.go` (NewSparseMerkleTrie
  default spec).
- Audit PDF (shipped with the library):
  `.../smt@v1.0.0/audits/240612_Thesis_Defense-Pokt_Network_Sparse_Merkel_Tree_Security_Audit_Report.pdf`,
  page 5, "Protection Against Second Preimage Attacks and Shortened Proof Attacks."
- silt: `core/statehash/statehash.go`, `core/statehash/witness.go`, `core/statehash/fold.go`.
