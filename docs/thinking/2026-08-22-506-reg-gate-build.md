# #506 — the version-gated reg-inclusion rate bound: what shipped, and two deviations

Date: 2026-08-22. Certification: `silt-reviews/research/research-outcome/506-reg-inclusion-version-gate-RESEARCH-CERTIFICATION-2026-08-22.md`
(builds on the #503 cert Q3, which certified the R-rule itself).

## What shipped

- **The R-rule as validity** (`validateBondRegs`, active only past the gate): a
  bond reg for identity X is a valid block payload only if X is not slashed
  (R∞ — the #503 Defect-A commit path, closed structurally) and X's last
  committed reg is ≥ R blocks old. First registrations are exempt. A block
  carrying two regs for one identity is refused (validation runs against the
  parent state, so the one-block storm needs its own check). `ValidateBondRegErr`
  pre-filters peer submissions so an honest proposer never mints a block its own
  rule rejects.
- **R derived, never a literal** (`regMinInterval`): `max(TTL/4, K+2)`, capped at
  `TTL/2 − 1` if a pathological config would otherwise block honest renewals
  (renewal is due at TTL/2). At island defaults (TTL 32, K 8): R = 10.
- **The readiness signal** (`BondReg.Version`, keyasint 7): conditionally signed
  exactly like `Domain` (a version-less reg signs the pre-gate message, so every
  existing signature verifies unchanged; a signalling reg's byte cannot be
  flipped without breaking `Sig`), hash-covered (BondRegs are committed by
  `Block.Hash`), kept by `Prune`. `NewBondReg` stamps it unconditionally —
  signalling is a property of the binary, not a choice.
- **Lock-in** (`rotateEpoch`): at each mature epoch boundary, tally the frozen
  set's rule-aware WEIGHT (`regVersion[id] ≥ BlockVersionRegGate`); the first
  time it clears the same >⅔ super-quorum finality uses, lock in one-way and set
  `H_act` = next boundary. Enforcement: every block of height > `H_act`
  (strictly greater; height-keyed, never version-tag-keyed). Monotonic — a later
  ready-weight collapse stalls, never forks. Derived state: replay and `adopt`
  reproduce it identically.
- **Pre-latch override** (`Config.RegGateActivationHeight`): the trusted
  launch-anchor fleet declares the boundary as genesis config — today's
  deployment mode, no signalling needed.

## Deviation 1 — the signal rides the BOND REG, not the Attestation

The certification's letter said "add a Version byte to `Attestation`". Two code
facts break that carrier, verified during the build:

1. **`Block.Hash()` does not commit `Atts`.** An attestation-borne signal is
   strippable by any re-serving peer (the #437 cert-strip class): replicas that
   synced through different peers would tally different readiness histories and
   derive different `H_act` — the exact divergence Q2 forbids.
2. **The consensus signature cannot absorb the byte compatibly.** The era-2
   payload is `domain ‖ phase ‖ round ‖ hash`; folding a version in changes what
   old verifiers reconstruct, so mixed-fleet attestations would fail
   verification — a chain halt, not a soft fork.

The bond reg has none of these problems: hash-covered, validator-signed with the
conditional-signing idiom `Domain` already proved, renewed every ≤ TTL/2, kept
by `Prune`, and held by every frozen-epoch member by definition (a committed
bond is what qualifies them). The certification explicitly delegated "the exact
encoding of the attester version signal" to the builder; this is that call, and
it preserves every certified invariant (per-validator, weight-counted,
unforgeable, chain-derived).

## Deviation 2 — no v3 block tag is minted

The certification's soft-fork framing assumed "an un-upgraded v2 node still
accepts a v3 rule-following block (additive keyasint fields decode fine)". At
the actual site, `versionSupported` is an **exact set** — a pre-gate binary
rejects a `Version: 3` block outright at decode (`want 1..2`). Minting the tag
would strand every un-upgraded node completely: a hard fork, strictly worse
than the storm it prevents.

The rule needs no schema change to enforce (it only REJECTS payloads), and the
certification's own Q2 form is height-keyed ("apply the R-rule to every block of
height > H_act"). So: `BlockVersionRegGate = 3` exists as the READINESS
threshold constant, this binary ACCEPTS v3-tagged blocks (future-proofing the
fleet for an era that genuinely diverges schema), and `BlockVersion` (minting)
stays 2.

## Honesty: the signal field itself is the last free schema change

Adding `BondReg.Version` changes both the block hash and the reg signature for
regs that carry it. A binary WITHOUT the field drops it at decode → different
hash, failed sig → it rejects every block containing a signalling reg. That is
acceptable **today only because silt is pre-launch** and the fleet is the
trusted, coordinated anchor set (the certification's pre-latch regime — the
same regime in which `Domain` and the era-2 flip shipped). **After launch, any
new hash-covered or signed field needs a two-release deploy** (release A: fleet
learns to decode/preserve; release B: fields appear) — and the activation
machinery built here is exactly the tool that measures when release B is safe.
That is the point of building the plumbing now, as tenant #1.

## Certified residual (stated, not solved)

If the fleet never crosses ⅔ rule-aware weight, activation never fires and the
#503 interim (proposer filter #508 + client backoff) remains the indefinite
fallback. There is no safe way to force it (BIP8-style force-activation forks
the minority off — rejected by the certification). The storm is structurally
closed only once ready-weight crosses the threshold on the real fleet.

## Evidence

`core/chain/reggate_506_test.go`, ablation-verified RED three ways:
enforcement disabled → both enforcement tests fail; readiness counted by HEADS
→ the ¾-heads/43%-weight rig locks in when it must not; `H_act` = lock-in
boundary (no next-boundary delay) → the boundary-exact test fails. Plus:
signature-binding (a stripped signal fails `Sig`), prune-survival, replay
determinism (a fresh replica derives the identical `H_act`), monotonicity under
ready-weight collapse, and the full three-clause R-rule at a pre-latch boundary.
The quorum-level "storm block cannot FINALIZE at H_act ± 1" argument rests on
the validity refusal shown here plus the existing I1 intersecting-quorum
machinery (an enforcing validator neither attests nor appends an invalid
block); a dedicated multi-replica finalization drill is future work if the
model-check harness (#406) wants it as a property.
