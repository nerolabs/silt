# State-root domain separation — the SMT second-preimage / shortened-proof residual (R3.1)

**Status: GATED, not closed (2026-09-06).** The disjoint-preimage argument is Researcher-CERTIFIED on the
`smt.VerifyProof` path and the audit's different-hash recommendation is declined with a certified ground;
two latent defects on the composed fold surface were found and are closed on `main` behind gates; one
scope invariant (SI-6) is an open owner call. Certification:
`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/R3.1-SMT-domain-separation-disjoint-preimage-RESEARCH-CERTIFICATION-2026-09-06.md`.
Design record: [`docs/thinking/2026-09-01-smt-domain-separation-close-design.md`](../thinking/2026-09-01-smt-domain-separation-close-design.md).

## 1. The attack class, and what rests on it

silt commits its era-3/v5 validity state as one `pokt-network/smt` v1.0.0 sparse-Merkle root
(`core/statehash`). The floor box re-derives that root from proofs (`fold.go`) and the witness accessor
(`witness.go`) resolves per-key facts from proofs; both require the derived root to equal the
quorum-committed `StateRoot`. What rests on proof unforgeability is therefore the **fold's `OldValue`
soundness** and the **`ProvenAbsent`-only-from-a-verified-proof** invariant — the floor box's own recompute
path. It is NOT I1–I5: `consensus-invariants.md` does not read `StateRoot`; the committed root is
attester-signed and the fold's defence today rests on that signature (a held-in-tension residual, §7).

A second-preimage / shortened-proof forgery is a proof that verifies against the committed root for a
`(key, value)` pair that was never committed — by getting a preimage of one node TYPE accepted as another
(type confusion), or by finding two distinct node preimages with one digest.

## 2. The library mitigation

Node-type prefixes: `0x00` leaf, `0x01` inner, `0x02` extension (`node_encoders.go:22-27`, with an
`init()` guard that the prefix length is 1, `:32-38`). Every digest is `SHA-256(prefix ‖ body)`; the
verifier recomputes bottom-up and always prepends the type prefix itself (`proofs.go`, `trie_spec.go`).

## 3. The silt-specific argument — prefix-free leading byte, NOT fixed width

The design record argued from fixed-width (65-byte) leaf and inner preimages. **That is wrong on the
verify path**: `NonMembershipLeafData` is bounded only from below (`proofs.go:61-75`), so leaf preimages
are not fixed-width. The argument that holds, and is certified, is stronger: the preimage classes are
**disjoint by their leading byte**, so no byte-string is a valid member of two classes and a
type-confusion forgery needs a SHA-256 collision across the type boundary — a generic second-preimage,
out of reach. Two facts sharpen it (certification §2.3, §2.5): the INNER preimage is fixed at 65 bytes
unconditionally because the verifier copies every side node into a fresh 32-byte buffer
(`proofs.go:437-445`, `make`+`copy` — the library's normalisation, not silt's hasher choice, and not
`validateBasic`, whose side-node size loop an attacker skips with `SiblingData = nil`); and **exactly two
preimage classes enter a root** — `0x00` leaf and `0x01` inner — because an extension node's digest is the
digest of its EXPANDED inner-node chain, so the `0x02` preimage never enters the digest chain on the verify
path. (The `0x02` class matters on the fold surface, §6.1, where witness bytes are seeded as nodes.)

## 4. Scope conditions — each with its gate; violating any re-opens R3.1

| SI | Condition | Gate | Status |
|---|---|---|---|
| SI-1 | Non-sum trie: every SMT silt constructs or imports is `sumTrie=false` | V2 (`internal/depcheck/smt_domain_separation_test.go`) pins one `NewTrieSpec(sha256.New(), false)` site; **G-R31-3** (`smt_r31_pins_test.go`) additionally forbids `NewSparseMerkleSumTrie`/`ImportSparseMerkleSumTrie` (the library sets `sumTrie=true` and `WithValueHasher(nil)` INSIDE those, invisible to V2) and requires `sha256.New()` at every `NewSparseMerkleTrie`/`ImportSparseMerkleTrie` | gated |
| SI-2 | Default value hasher; never `WithValueHasher(nil)` — keeps the leaf's value slot a 32-byte SHA-256 digest, so the leaf preimage's width is what the argument (§3) says it is and a raw value can never be hashed into the leaf slot; certification `R-R31-SI2-WIDTH-NOTE` closed by this restatement | V2 | gated |
| SI-3 | No closest-proof path (`ProveClosest`, `VerifyClosestProof`, `nilPathHasher`) | V2 | gated |
| SI-4 | Key-space injectivity: every tag used with `statehash.Key` ends in exactly one NUL and has no other | **G-R31-4** (`core/chain/r31_key_tag_injective_test.go`, every `tag*` constant by source) | gated |
| SI-5 | No externally-writeable node store: no entry silt did not itself bind to its digest | **G-R31-1** (below) | gated (was VIOLATED on `main` until 2026-09-06) |
| SI-6 | No empty committed leaf value: `statehash.Root` rejects `len(Value) == 0` | **G-R31-5 — OWNER CALL** (a validity-surface tightening on the committed-state builder; one line) | open |
| SI-7 | Library version and audited-commit pin | **G-R31-6** (`smt_r31_pins_test.go`: `go.mod` pins exactly `v1.0.0`) | gated |

## 5. The decline of the audit's different-hash-per-node-type recommendation — CERTIFIED

Ground (a), decisive: for prefix-free preimage classes a per-type hash function adds **no security
delta** — the classes are already disjoint. Consequence (c): a hashing change would fork the committed
root for every existing block (a hard fork at the era-4/v5 freeze) and diverge silt from the audited
library. And the recommendation would have closed **neither** defect found below: the panic fires in the
parser before any hashing, and the store poisoning never consults a hashing scheme.

## 6. The two latent defects found on the composed surface, and their closes

**6.1 The fold's writeable node store (SI-5; audit Issue #2, p.8, on the verify side).** `FoldChangedPaths`
seeded the library's node store with witness-supplied `(Digest, Preimage)` sibling pairs and never
checked `Digest == hashPreimage(Preimage)`; the library dispatches node type on `data[0]` and takes the
digest from the LOOKUP KEY without recomputing it. A forged node cost zero hash work. **Close (G-R31-1):**
every delete sibling is bound before seeding by the library's own rule — SHA-256 of the bytes for a leaf or
inner preimage, the EXPANSION root for an extension preimage (`foldDigestMismatch`); the fold stalls with
`ErrFoldSiblingUnbound` otherwise. Gate `TestR31UnboundDeleteSiblingStallsTheFold` with its bound control.

**6.2 The parser panics.** The library enforces node prefixes with `panic` (`node_encoders.go:121-125`) and
slices attacker bytes unbounded: `parseLeafNode` on a `NonMembershipLeafData` whose first byte is not
`0x00` (a 33-byte witness, no siblings, for any absence query), and — the arm the first build missed
(PE code ruling S1) — `hashPreimage(SiblingData)` at `proofs.go:89`, which `validateBasic` bounds nowhere:
an empty `SiblingData`, or a `0x02` `SiblingData` shorter than 35 bytes, panics in `isExtNode` /
`parseExtNode` (a 154-byte gob witness crashed `IngestBlockWitnesses`). Non-test `core/` has no
`recover()`, so each was a remote process crash (safety held; liveness did not). **Close (G-R31-2):**
`proofShapeParsable` refuses both shapes BEFORE `VerifyProof` in `Resolve` (→ `NoWitness`),
`FoldChangedPaths` (→ `ErrFoldProofShape`) and, through `Resolve`, `IngestBlockWitnesses`. Gates
`TestR31MalformedLeafPrefixIsRefusedNotPanicked` and `TestR31MalformedSiblingDataIsRefusedNotPanicked`
capture the raw library panic as their positive controls at every shape. `SideNodes` need no arm
(`make`+`copy`).

## 7. Wiring status, and the residuals

Every defect above was **latent**: the floor box has no production callers and never Accepts (the R1.7
external red-team gate, B8, still holds the accept flip). That is why these are gates and not incidents.
Held in tension: the fold's defence rests on the attester-signed `StateRoot`, which erodes C-7's *safety
needs no trust in the tier above* until G-R31-1/2 are the defence. Bounded, not eliminated: the audit's
scope commit `868237978c…` and verification commit `3981639bd08cf52a7668c3681cbe2243d957e4ee` (audit p.2) are
not the `v1.0.0` tag — SI-7 pins the tag; provenance to the audited commits is carried here. Filed for
Boulder 3, out of R3.1's scope: a hand-crafted 5-byte gob length prefix forces a ~10 MB allocation in
`proof.Unmarshal` — `SProofMax` bounds ENCODED bytes, not parse memory (PE code ruling). Routed elsewhere: `hasher.go:103-108` is not
goroutine-safe (single-loop today). Positive controls for both defects were owed to the Tester by the
certification (it had no shell); they are the four `TestR31*` gates above (two per defect, including the
false-acceptance direction of the binding), executed.

## 8. What lifts the rest

G-R31-5 is the owner's. G-R31-7 requires this record (present, cross-linked) and G-R31-1 and G-R31-2
discharged as BUILT — the binding on all three node types with its false-acceptance gate, and the shape
check on BOTH `NonMembershipLeafData` and `SiblingData` — after which it composes with the R1.7 external
red-team gate for the #657 accept flip. Nothing here is recorded CLOSED; R3.1 stays GATED until G-R31-5.
