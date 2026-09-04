# O3 Direction T — retire the fork-choice weight term; `heavier` becomes height → head-hash

**Date:** 2026-09-04 · **Seat:** PLANNER (deliberation) → TESTER (gates) → BUILDER · **Status:** certified; build queued (overnight lane C)

**Binding inputs:** owner-RATIFIED Direction T (2026-09-03: "Direction T"); the PE ruling
`RULING-O3-fork-choice-weight-R-vs-T-2026-09-03.md`; the Researcher's recommendation
`O3-fork-choice-weight-R-vs-T-RESEARCH-RECOMMENDATION-2026-09-03.md`; and the build-gating certification
`O3-Direction-T-I5-restatement-and-divergence-RESEARCH-CERTIFICATION-2026-09-04.md` — **CERTIFIED**, conditional
on the four gates landing in the same commit as the retirement.

## 1. The mechanism, in one paragraph

The fork-choice weight term is a dead consensus quantity **because** silt has no production posture without
BFT finality (`FinalizedHeight()` returns `len-1` iff the gate is on, so it structurally cannot lag `Head()`),
and `blockWeight` verifies the bare hash while era-2 attestations sign `consensusSigBytes`, so by the code
every era-2+ block already weighs 0 and `heavier` already falls to height → hash; a dead term that survives is
exactly how the third bare-hash verify site happened (#558). This change **retires the term** — delete
`Weight()` / `blockWeight()` / `anchorWeight()` / `Config.AnchorWeight`, state `heavier` as height →
head-hash among descendants of the finalized head — and ships the four gates the cert names in the SAME
commit so the retirement cannot leave a hole: the verifier inventory (no attestation verified outside
`verifyAtt`, with `signedBlock` allowlisted as the era-1-gated fourth site), the interlock (the `heavier`
purity pin + the certificate-variant determinism oracle + fast/slow-path equivalence), the ramp guard's second
half (the twin `Weight() <= 0` at `modelcheck_i5_357_test.go:92`), and the I5 text + claims-ledger edits
(widen I5; no I6).

## 2. Options (settled by ratification; recorded)

| Direction | Verdict |
|---|---|
| T — retire the term | **RATIFIED; CERTIFIED to build** |
| R — repair `blockWeight` to verify `consensusSigBytes` | refused by the owner; both seats recommended T |

Reopening condition (narrow, named): a shipping posture in which `FinalizedHeight()` lags `Head()` — a code
change to that function, not a config posture.

## 3. Shapes that are the Builder's
- One commit: the deletion + the four gates + the two doc edits (I5 Statement/Assert per cert §3; the R0.6
  scar untouched; `claims-ledger.md:47` per cert §7) + the three legacy `Reconcile` fixtures re-grounded on
  height → hash (the cert's six fixture dispositions).
- `heavier` reads ONLY `blocks[len-1].Height` and the head `Hash()`; the AST purity pin forbids anything else.
- `check_cited_tests.py` is strict on `docs/**`: the Assert may name only tests that exist at the merge commit.

## 4. Gates (cert §8) — Tester encodes RED-first
(a1) AST pin over `ed25519.Verify` with the 8-row classified allowlist + teeth; (a2) one `roundsWorld` era-2
certificate accepted by every era-2-reachable attestation verifier. (b1) `heavier` purity pin; (b2)
certificate-variant determinism oracle scoped to forks both replicas admit, RED under controlled revert;
(b3) fast/slow-path equivalence. (c) the twin `Weight() <= 0` deleted. (d) I5 text equals the cert text;
`grep -c 'weight → height → hash'` = 0; `check_cited_tests.py` and `check_claims.py` exit 0.
