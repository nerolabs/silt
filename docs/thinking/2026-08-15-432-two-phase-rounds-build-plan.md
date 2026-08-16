# 2026-08-15 — #432 build plan: two-phase rounds (plain T1), per the research certification

**Certified design** (research: `432-rounds-locking-liveness-RESEARCH-CERTIFICATION-2026-08-15`
in the review archive; PE concurrence chain in `principle-engineer/`): two-phase gather
(prepare→precommit) in BOTH regimes, lock on the highest-round prepare-QC, lock-change only
with a POL, view-change quorum carries the highest prepare-QC forward, deterministic
per-height sweep-count round advance, watermark `{Height, Round, Phase, Hash, prepareQC}`,
equivocation = same-(h, r, phase) different hashes. Merge gate: S1 + S2 oracles, both
regimes, RED against a lock-free baseline, GREEN with the prepare phase. Owner latency call
(§6): plain T1, no fast path (ratification pending; reversible).

## Design decisions (deliberated here, built below)

1. **Signature domain.** Today `Attest`/`Sign` sign the bare block hash. Certified: prepare
   and precommit sign `(hash(B), h, r)` with the PHASE domain-separated. Encoding: a signed
   payload `"silt/consensus/v2" ‖ phase ‖ round ‖ blockHash` (height rides inside the hash).
   The Attestation struct gains `Round uint64` + `Phase uint8` (cbor additive, omitempty) so
   a legacy attestation decodes as `(r=0, prepare)`.
2. **The block does NOT carry its round in the hash.** Tendermint separates the vote's
   `(h, r, block-id)` from the block content precisely so the SAME value can be re-proposed
   at a higher round without changing identity — the view-change's "re-propose the locked
   value" needs value-identity stable across rounds. The PROPOSAL message envelope carries
   the round; the COMMITTED block records its commit round + certificate in new additive
   fields (`CommitRound`, and `Atts` holding the precommit-QC; `PrepareQC` alongside for
   auditability), which are cbor-additive/omitempty → a legacy block (r=0) hashes
   identically. **BlockVersion bumps**: blocks minted under the new rules declare the new
   era; validation is era-gated so committed legacy chains stay valid (R2: no silent
   re-interpretation of history).
3. **QC = the existing []Attestation shape at threshold**, phase/round-tagged. The
   thresholds are EXACTLY the existing commit thresholds (launch ⌊A/2⌋+1 derived from
   len(Anchors) per #411; mature >⅔ frozen epoch weight per #389) — the certification's "POL
   threshold = commit threshold, no new arithmetic" — so `requiredLaunchAnchors`/
   `countAnchorSupport`/weight-quorum code is REUSED, never re-derived (the #402 lesson).
   Sybils excluded from BOTH phases in launch (certification §5.5).
4. **Wire protocol (node layer).** The gather becomes: propose(B, h, r) → collect prepare
   sigs → assemble prepare-QC → LOCK (durable) → broadcast the QC + collect precommits →
   precommit-QC = commit → commit broadcast as today. New message kinds: prepare-reply
   (reuses MsgAttestReply semantics), MsgPrepareQC (proposer→attesters, carries the QC),
   precommit reply, MsgRoundChange (carries the sender's lock: value hash, round,
   prepare-QC), with the (h, r+1) designated proposer assembling the new-view. Round
   advance: a per-height sweep counter in the existing chainSyncTick cadence (pure function
   of deliveries — B2), quorum-observable before a new round proposes (anti-grief).
5. **Locking + watermark.** `ports.SignMark` grows `{Round, Phase, PrepareQC}` (cbor
   additive; absent loads as r0/prepare — I2's restart guarantee preserved per-(h,r,phase);
   mark-before-sign for BOTH phases). The lock = highest-round prepare-QC witnessed,
   persisted with the mark so a restarted validator re-presents it in round-change.
6. **Equivocation (I5).** `Equivocation` proof gains round+phase; `VerifyEquivocation`
   requires same-(h, r, phase), different hashes. A cross-round different-hash signature is
   honest (the POL justifies it); the slash detector must never fire on one. The #397
   honest-slash oracle extends to round schedules.
7. **Failing-first strategy.** Full-mechanism-then-controlled-revert (the I5-oracle
   precedent): S2 must go RED with the prepare phase disabled (a test-only lock-free mode or
   a documented revert diff) and GREEN with it — proving the prepare phase is load-bearing.
   The #432 wedge oracle (branch `oracle/i4-liveness-wedge`) merges GREEN in this PR.

## Build order (each step suite-green before the next)

- **A. ports + chain layer:** Attestation round/phase; signed-payload v2; Block additive
  fields (CommitRound, PrepareQC); era-gated ValidateCommit requiring a same-(h,r)
  precommit-QC built on a verified prepare-QC; equivocation round-scoping. Exhaustive unit
  tests at A∈{3,4,5} + weight-regime variants.
- **B. node layer:** two-phase gather in proposeBlock; lock persistence; round-change /
  new-view; sweep-count advance; drain + publish integration (both paths propose through
  the SAME two-phase machinery — the uncoordinated-paths race stays possible and must now
  be SAFE and RECOVERABLE, which is the point).
- **C. oracles:** S1 (delayed lower-round quorum, crash-only) + S2
  (equivocate-at-r0-then-misreport, f=1) over held delivery, both regimes;
  the wedge oracle GREEN; I2 restart oracle extended per-(h,r,phase); I5 honest-never-
  slashed under round schedules incl. POL-carrying lock-changes.
- **D. canon:** consensus-invariants I4 updated (mechanism SHIPPED), model-check doc, the
  certification folded into decisions.md; CHANGELOG; ROADMAP note.

Risks watched: (i) the do-not-hash-the-round subtlety — any accidental round-in-hash breaks
re-proposal identity (unit-pinned); (ii) legacy-era validation drift (era-gated tests both
ways); (iii) the drain's designated-proposer interaction with rounds (the designated rule
now applies per-(h, r): props[(h+r) mod n] so a wedged designated proposer rotates away —
liveness under a crashed proposer; pinned by an oracle).
