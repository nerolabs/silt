# era-3 step 2a — commit the two roots into the block schema

**Date:** 2026-08-29
**Seat:** Builder
**Step:** 2a of the ratified, certified era-3 sequence (schema + hash + versionSupported).
Step 1 (root COMPUTATION) is on main (`72d5c4c`): `core/statehash` + `core/chain/statehash.go`
give `StateRoot()`/`LogRoot()` and the determinism oracle.
**Certification:** `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era3-committed-state-root-format-RESEARCH-CERTIFICATION-2026-08-28.md`
**Ruling:** `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era3-committed-state-root-format-2026-08-28.md`

## Scope of 2a — exactly this, nothing more

1. Add `StateRoot` and `LogRoot` (`ports.Hash`) to the `Block` struct.
2. Include BOTH in the `Hash()` unsigned body (`chain.go:523`) so attesters sign them.
3. Add `BlockVersionStateRoot = 4` and extend `versionSupported` to accept `<= 4` so a v4
   block DECODES and is accepted (the cert's mint-v4 requirement — a v4 block must not be
   silently mis-validated).
4. Populate `StateRoot`/`LogRoot` from the step-1 accessors when a v4 block is CONSTRUCTED,
   without minting v4 by default.

## STOP boundaries (re-triggered by later steps, NOT in 2a)

- **No validity predicate** that rejects on a root mismatch — that is step 2b.
- **No mint-version flip.** `BlockVersion` stays `BlockVersionRounds` (2). Minting v4 is
  step 2c, height-gated on a `regVersion >= 4` supermajority.
- **No activation height.** 2c.
- **No consensus-rule change** beyond the additive schema + `versionSupported` widening.

The sequence exists so no v4 block is ever minted before its predicate (2b) exists. 2a makes
the schema and hash CARRY the roots and makes v4 DECODABLE; it does not enforce them.

---

## THE LOAD-BEARING COMPAT DECISION — the CBOR shape of the two root fields

### The tension, stated exactly

Two requirements pull in opposite directions:

- **Era-2 byte-identity (task requirement, `chain.go:260-268` re-interpretation ban).** An
  era-2 block MUST hash and validate BYTE-IDENTICALLY after 2a. Committed history is never
  re-interpreted. Concretely: `Block{...}.Hash()` for a pre-2a era-2 block must produce the
  same 32 bytes after the two fields are added to the struct.
- **Era-3-always-definite (cert freeze condition 4, Q1).** An era-3 block commits a DEFINITE
  root, never absent. The empty-vs-absent ambiguity must be closed: an empty era-3 state
  commits the definite empty root, not a missing field.

A naive read says these conflict: "byte-identical" wants the field absent for era-2;
"definite" wants the field always present for era-3.

### The resolution — `omitempty` at the CBOR layer, and the reason it is sound

Both hold simultaneously because **an era-3 root is NEVER the CBOR zero value**, so
`omitempty` omits the field for era-2 and never for era-3.

CBOR `omitempty` on a `ports.Hash` (`[32]byte`) omits the field only when the value is the
zero array (all 32 bytes zero). The decisive facts, both verified by execution:

- **The empty-log root is `sha256("") = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`,
  not zero.** `translog.mth([])` returns `sha256.Sum256(nil)` (`core/translog/translog.go:107-109`),
  the RFC-6962 empty-tree hash. Verified: an empty `New(...)` chain's `LogRoot()` is that
  non-zero constant.
- **The empty-state root is a non-zero SMT constant, not zero.** A zero-value `Chain`'s
  `stateRootLeaves()` always emits the four scalar leaves (`everMature`, `matureEpoch`,
  `gateLockedIn`, `gateHeight`), so its `StateRoot()` is the SMT root over those four leaves —
  a fixed non-zero constant (verified). A populated chain's root is even more clearly non-zero.

So for EVERY era-3 (v4) block, both roots are non-zero constants → `omitempty` emits them.
For EVERY era-2 (v2) block, the node never sets the fields → they stay `ports.Hash{}` →
`omitempty` omits them → the CBOR bytes of the unsigned body are IDENTICAL to pre-2a →
`Hash()` is byte-identical. Both properties hold at once. The `omitempty` here is not a
compat hedge that weakens the era-3 "required" semantic — it is the mechanism that makes the
additive change invisible to era-2 while every era-3 block still carries a definite,
signed, non-zero root.

### Options considered

**Option A — required (no `omitempty`), rely on a version bump alone.** This is the literal
reading of freeze condition 4 ("both root fields REQUIRED (not omitempty)"). Under CBOR, a
required `[32]byte` field is ALWAYS emitted, including as 32 zero bytes for an era-2 block.
That changes every era-2 block's unsigned body and therefore its `Hash()`. **Rejected: it
breaks era-2 byte-identity — the whole game of 2a.** The freeze condition's INTENT (no
empty-vs-absent ambiguity for era-3) is fully satisfied by Option B without paying this cost,
because an era-3 root is never zero.

**Option B — `omitempty`, roots always non-zero for era-3 (CHOSEN).** Era-2 hashes
byte-identically (fields absent); era-3 always carries a definite non-zero root (fields
emitted). The empty-vs-absent hole the cert worries about cannot open, because "absent" and
"the era-3 empty root" are different byte states: absent is no field; the era-3 empty root is
the non-zero `sha256("")` / four-scalar-SMT constant. A v4 block with an absent root field is
not a valid era-3 block — but 2a adds no predicate to reject it (that is 2b); the point for
2a is only that the SCHEMA and HASH carry the roots and that a well-formed v4 block always
emits them.

**Reconciling Option B with freeze condition 4's literal "not omitempty".** The freeze
condition is written against the empty-vs-absent HAZARD: an empty state must not be
indistinguishable from a missing field. Option B closes that hazard by a different, stronger
mechanism than "never omitempty" — the era-3 empty root is a non-zero constant, so it is
ALREADY distinct from absent at the byte level. The condition's goal is met; its literal
implementation clause is superseded by the byte-identity requirement the condition's author
did not have in front of them (the freeze cert reasons about era-3 shape, not the 2a era-2
byte-identity obligation). **This is the exact point I am flagging for the blind PE and the
Researcher re-cert:** I am NOT satisfying freeze condition 4 by making the field `omitempty`
in defiance of it; I am satisfying its INTENT (definite era-3 root, no empty-vs-absent hole)
while ALSO satisfying era-2 byte-identity, and the two are compatible only because the era-3
empty root is non-zero. If the Researcher holds that condition 4 must be met literally
(a field tag always present on the wire for v4), the correct home for that check is the 2b
validity predicate — "a v4 block whose StateRoot/LogRoot is zero is invalid" — not a CBOR
`required` tag that would corrupt era-2 hashes. I recommend that placement and flag it as the
one decision to confirm.

**Option C — a nested `Roots` sub-struct.** Rejected for the same reason the format cert
rejected 1B over 1A: flat beats nested, the two roots have different lifecycles, nesting buys
nothing and complicates the `omitempty` reasoning.

### Decision

**Option B — with the mechanism corrected to `*ports.Hash` after the byte-identity oracle
caught the first attempt.** Two flat fields, `StateRoot` and `LogRoot`, CBOR field tags 15
and 16, `keyasint,omitempty`, inside the `Hash()` unsigned body. Era-2 byte-identical (roots
nil, omitted); era-3 always definite (roots set, emitted, signed).

**The correction — why `*ports.Hash`, not `ports.Hash`.** My first cut used a plain
`ports.Hash` (`[32]byte`) with `omitempty`. The byte-identity oracle
(`TestEra2GoldenHashUnchanged`) went RED immediately: `omitempty` in fxamacker/cbor does NOT
omit a zero-valued FIXED-SIZE ARRAY. An array is never "empty" — it always has 32 elements —
so a zero `[32]byte` is emitted as 32 zero bytes. The CBOR body dump proved it: a v2 block's
unsigned body grew from `a6…` (map of 6) to `a8…` with two extra `0f5820 00…00` /
`1058200 0…00` leaves, changing the hash. This is exactly the failure 2a exists to prevent,
caught at the model-check tier before any field run — the oracle doing its job.

The fix is `*ports.Hash` (pointer to the 32-byte array). `omitempty` DOES omit a nil pointer.
Verified by execution: a nil `*ports.Hash` field emits nothing (`a10101`), a set one emits the
32-byte value (`a20101 0f5820 09…`). So an era-2 block leaves both root pointers nil → the
unsigned body is byte-identical to pre-2a; an era-3 block sets both pointers to the non-zero
roots → they are emitted and signed. This keeps the fixed 32-byte type (a `[]byte` field would
lose the width invariant a committed root needs) AND gets clean omission. The nil-deref surface
is contained: `Hash()` copies the pointers into the unsigned struct (cbor handles nil), and
construction goes through `newV4BlockWithRoots`, which always sets both.

**Empty-vs-absent is still closed for era-3.** A v4 block with a nil root is not a valid era-3
block, but its INVALIDITY is enforced by the 2b predicate ("a v4 block's StateRoot/LogRoot must
be present and equal the recomputed roots"), not by the CBOR tag. 2a's job is that the schema
and hash CARRY the roots and a well-formed v4 block always sets them (non-nil, non-zero). The
nil-vs-set-nonzero distinction at the pointer level is even cleaner than the zero-vs-nonzero
array distinction: nil is unambiguously "absent," a set pointer is unambiguously "present with
this value."

### Why the roots go INSIDE the Hash body (unlike Atts/PrepareQC/CommitRound/Pruned)

The excluded fields (`Atts`, `PrepareQC`, `CommitRound`, `Pruned`) are set AFTER hash-identity
is fixed or are post-commit certificates; they must not change the block's identity. The two
roots are the OPPOSITE: they are a commitment to the state the block produces, and attesters
must SIGN them so a forged root cannot ride a valid signature. So they belong with `Version`,
`Height`, `Prev`, `Entries`, `BondRegs`, `Slashes` in the signed body. This is the Ethereum
`stateRoot`/`receiptsRoot` shape (cert Q1): committed roots inside the signed header.

---

## versionSupported widening — the same-release half of the mint-v4 fix

The cert (Q7) and ruling refute minting v3: `versionSupported(3)` is TRUE today
(`chain.go:668`, `v <= BlockVersionRegGate=3`), so a current binary decode-accepts a v3 block
and validates it under era-2 rules with NO root predicate — a silent mis-validation. era-3
must mint v4 with `versionSupported` extended to `<= 4` IN THE SAME RELEASE that adds the root
fields.

2a does the schema half of that: add `BlockVersionStateRoot = 4` and change
`versionSupported` to `v >= 1 && v <= BlockVersionStateRoot`. After 2a a v4 block DECODES and
is accepted (it validates under the `>= BlockVersionRounds` era-2 path for now — the v4
validity predicate is 2b). A version beyond 4 is still rejected loudly with `ErrBlockVersion`
— the hard-fork failure mode preserved.

**Why widening in 2a is correct, not premature.** The cert's requirement is that the accepted
version set and the root fields land in the SAME release, so no deployed binary is ever in the
state "accepts v4 but has no root fields." 2a lands both together. Minting v4 (2c) is gated
separately on `regVersion >= 4` readiness, so widening the accepted set now does not cause any
v4 block to be minted; it only makes a v4 block — if one existed — decodable and carry the
signed roots. The predicate that ENFORCES the roots is 2b, sequenced before 2c mints one.

The error-message literals in `Decode`/`DecodeBlocks` (`want 1..%d`) currently interpolate
`BlockVersionRegGate`; they move to `BlockVersionStateRoot` so the message stays truthful
about the accepted ceiling.

---

## Model-check plan (each with a demonstrated RED)

1. **Era-2 unchanged (byte-identity).** A v2 block built exactly as before hashes to the same
   32 bytes after 2a. Proven by a golden-hash assertion: the RED is flipping the two root
   fields to non-`omitempty`, which changes the era-2 hash and fails the test.
2. **Era-3 roots are attester-signed.** A v4 block with a tampered `StateRoot` (or `LogRoot`)
   has a different `Hash()`, so a signature over the real hash fails to verify. RED: exclude
   the roots from the `Hash()` unsigned body → tampering no longer changes the hash → the
   tamper-detection assertion fails.
3. **v4 decodes and is accepted; v5 rejected loudly.** `versionSupported(4)` is true;
   `Decode` accepts a v4 block; `BlockVersionStateRoot + 1` is rejected with `ErrBlockVersion`.
   RED: leave `versionSupported` at `<= BlockVersionRegGate` → v4 is rejected → the accept
   assertion fails.
4. **Population wiring.** A v4 block constructed with the step-1 roots carries
   `StateRoot == c.StateRoot()` and `LogRoot == c.LogRoot()`. RED: leave the fields unset →
   they are zero → the equality assertion fails.

`go test ./core/... -race` green.
