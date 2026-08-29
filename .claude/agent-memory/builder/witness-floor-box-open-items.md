---
name: witness-floor-box-open-items
description: The three open witness floor-box construction items (R3 size-DoS, Delivery, R4 omission) and how the frozen era-3 format constrains each — the non-obvious downstream consequences of the freeze.
metadata:
  type: project
---

The era-3 committed state-root format is FROZEN (immutable, `3af40bc`, ratified 2026-08-29).
The witness floor-box VALIDATION MECHANISM was deliberately left open as the next keystone
track. Options deliberation: `docs/thinking/2026-08-29-witness-floor-box-validation-mechanism-options.md`.

**Why (the non-obvious part):** the freeze commits ONLY `StateRoot`/`LogRoot` (cbor 15/16) and
the 18-field committedSet under one field-tagged keyspace. It commits NO witness field. That one
fact drives all three open items — the freeze is the anchor, not an obstacle:

- **Delivery: in-block witness carry (D-1) is OUT.** Adding an in-block witness field is a format
  change = a NEW era. The freeze FORCES witnesses to travel outside the block (on-demand fetch or
  separate gossip). Recommended floor default: D-2 (on-demand, any-of-N, no-permission) — defends
  TENETS:557 (open/multi-provider, never a privileged provider). A witness self-verifies against
  the committed root (C-7), so trusting the source is both unnecessary and forbidden.
- **R3 (size/DoS): derive the per-block witness ceiling from the payload caps, check pre-verify.**
  Per-KEY is already bounded by the pokt SMT `validateBasic` (256 side-nodes, its own CPU-DoS
  guard). The OPEN item is the per-BLOCK aggregate. Derive-don't-drift (#104 scar: a standalone
  transport cap silently dropped every prod chunk) — witness ceiling = f(payload caps), never a
  flat constant. The NUMERIC constants are a SECURITY PARAMETER → Research-gated; the derivation
  shape is a build decision. Adversaries: A-produce (malicious proposer) + A-serve (hostile
  on-demand provider, slow-loris).
- **R4 (omission): a THREE-valued accessor — PROVEN_PRESENT / PROVEN_ABSENT / NO_WITNESS.** A
  two-valued `(value, ok)` bool is the banned footgun (ok=false doubles as "absent" and "unknown"
  = the "no witness → accept" move C-7 §Q2 bans). NO_WITNESS forces stall/re-fetch, never accept.
  The frozen SINGLE-root format DISSOLVES the C-7 §11.2 sharded-omission problem: exclusion is
  proven against the one committed root, so "not in my shard" is never accepted as exclusion —
  sharding is a storage concern below the root, not soundness above it.

**SHIPPED — increment 1 (R4-a accessor, #633, `8bc7e79`) + increment 2 (R3 DoS bound, #634,
`0984db4` on main):** R3 is the pre-verify resource gate in `core/statehash/witness_bound.go`.
Three gates, all pre-parse: (1) per-proof byte cap `S_proof_max` = 16 KiB on the ENCODED size
before unmarshal (a byte cap, NOT a side-node count — pokt leaves `NonMembershipLeafData`
byte-unbounded, so a count cap ships and lies); (2) per-block ceiling `C_block = len(read-set)·
S_proof_max`, DERIVED per block (the #104 derive-don't-drift scar honored, no flat constant, no
per-block transition cap = no consensus change); (3) read-set shape gate (exactly the block's
read-set, no unread/dup/missing key). SAFETY WIRING: every rejection → R4 `NoWitness`, NEVER
`ProvenAbsent` — the C-7 §104 banned move. Params certified by `witness-floor-box-dos-bound-
RESEARCH-CERTIFICATION-2026-08-29`; mechanism by `RULING-witness-floor-box-mechanism-2026-08-29`
(both in silt-reviews). Blind-PE fix applied: a `QueryPresent` entry with an EMPTY `Value`
silently routed to `ProvenAbsent` (`Resolve` sends len-0 value to the non-membership branch —
same class as the R4 empty-value scar below); ingest now rejects empty-`Value` presence queries
to `NoWitness` before `Resolve` (`Kind` is authoritative). STILL OPEN for increment 3: the
`Block → read-set` DERIVATION, D-2 on-demand delivery, and the A-serve SLOW-LORIS read deadline
(a TIME attack the byte ceiling does NOT close).

**R4-a accessor built + hardened (`e6712a0`, branch worktree-agent-ab1d67cf5d9637f5d):** the
three-valued `Resolve` spine ships in `core/statehash/witness.go`. SCAR (blind PE LOW): the
membership/non-membership branch MUST key on `len(value) == 0`, NOT `value == nil`. The pokt
`smt.VerifyProof` selects on `bytes.Equal(value, defaultEmptyValue)` where `defaultEmptyValue`
is a nil `[]byte`, and `bytes.Equal` treats `nil` and `[]byte{}` as equal — so an empty-but-non-nil
`[]byte{}` takes the library's NON-membership branch. Keying on `value == nil` routed that same
`[]byte{}` to `ProvenPresent`, yielding a false PRESENCE off a valid ABSENCE proof (the MIRROR of
the C-7 §104 banned move). Rule for any future accessor over pokt-smt: match the library's
`defaultEmptyValue`/`bytes.Equal` convention, never a bare `== nil`. The source-scan test guards
exactly-one construction site for BOTH `ProvenAbsent` AND `ProvenPresent`.

**Increment 3 (delivery) — DESIGN OPTIONS filed (uncommitted), `docs/thinking/2026-08-29-witness-floor-box-delivery-increment3-options.md`:** Part A (Block→read-set derivation) + Part B (D-2 on-demand delivery). THREE non-obvious source findings drive it: (1) **`apply()` reads committed state too**, not just the validity predicates — it branches on `slashed[id]`/`bondRootOwner[root]`/`bondRootProven[root]` (chain.go:2977/2980/2986) to decide what it writes, so the read-set = union of validity-path reads AND apply-branch reads, or the box computes a wrong post-state root even when validity passed. (2) The **bond-reg reads are `map[k],ok` idioms** (`bondRegHeight`/`bondRootOwner`/`bonded`/`epochSet`) — both present-with-value AND absent are acceptance-relevant, which a single `QueryKind` (present XOR absent) can't model → a real gap in increment 2's `ReadEntry` for the bond family. (3) The **quorum-stack reads (`attesterQualifiedAt`/`requireEpochWeightQuorum`) read NON-committed observables** (`epochStart`/`effectiveEpochSet` #535 recovery boundary), which have no committed root — so their witness story can't close with the frozen roots alone; flagged OUT of increment-3 scope. The complete per-transition read-set: publish→`byRoot[root]`absent(+`spent[serial]`absent iff tokenQuorum>0); revoke→`byRoot[root]`present; unrevoke→`revoked[root]`present; bond-reg→{slashed,bondRegHeight,bondRootOwner,bondRootProven,bonded,epochSet}; **slash→∅ (empty, the clean case — VerifyEquivocation is self-contained crypto)**. Recommended A2: derivation in `core/chain` (only it reads unexported committed fields, like `stateRootLeaves`) + a **recording drift-guard** (wrap the committed maps, run real ValidateCommit+apply over a branch-covering corpus, assert recorded⊆derived, ABLATE). Recommended B2: any-of-N first-correct-wins reusing `FetchAttempts`+`RequestTimeout`+`RequestSizeFloorBytesPerSec` (no new knob — derive-don't-drift), `MsgGetWitness{root,key}` side-channel RPC (frozen block format UNTOUCHED, confirmed). GATING: read-set completeness is RESEARCH-gated (precondition of the C-7 soundness invariant — an omitted key defeats stall-not-accept silently, no missing witness to stall on); B2 no-permission delivery is TENET-gated by `:557` (satisfied; a permissioned provider = human veto gate). No new security param, no I1-I5/frozen-format touch.

**How to apply:** when this track moves to BUILD, R3 numbers route to Research before pinning;
the Delivery any-of-N-no-permission rule is a hard invariant (weakening it = human veto gate); the
R4 NO_WITNESS→stall accessor is the direct encoding of the C-7 banned-move invariant and its
regression test must ablate the green (inject NO_WITNESS, confirm stall not accept). No recommended
option touches I1-I5 or the frozen format. C-7 cert:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C7-witness-based-floor-box-validation-RESEARCH-CERTIFICATION-2026-08-27.md`.
Couplings: R3↔Delivery (bound lives at the delivery boundary), Delivery↔R4 (any-of-N re-fetch is
R4's liveness handler), R3↔R4 (an over-budget witness → NO_WITNESS, never PROVEN_ABSENT).
See [[keystone-leave-one-out-probes]] and [[era3-reload-root-check-gap]].
