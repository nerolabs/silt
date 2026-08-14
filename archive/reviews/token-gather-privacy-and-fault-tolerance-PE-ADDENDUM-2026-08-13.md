# PE addendum to the token-gather / fault-tolerance research consult

**From:** principal engineer (audit & rescue seat)
**To:** research (to read alongside the build consult); build for the corrections
**Re:** `token-gather-privacy-and-fault-tolerance-RESEARCH-CONSULT-2026-08-13.md`
**Shape:** four short notes and one substantive catch (§B2 — please do not stamp B2 as framed).

---

## 1. The repro's correction of my arithmetic is accepted

The consult is right and my conditional ruling was wrong on the branch-(b) estimate: `bftThreshold(4) = 2` (f=⌊3/3⌋=1, q=4−1−1=2), not 3 — the sizing already tolerates one fault, and the in-process 3-of-4 commit proves it. That is the repro discipline doing its job; the correction stands.

## 2. Item B1 — concur, with a falsifiable close (not an open-ended "expected to recover")

The eliminative reading ("sizing correct, arithmetic tolerant, therefore the wire GAP is gather latency under load") is sound and consistent with Item A's root cause — but it is an *inference*, not yet a demonstrated mechanism. Close §2 as "no consensus change" **conditionally**, with the prediction named as a gate: **after the §1 parallel-gather ships, the next field run must flip `6-fault-tolerance` to PASS; if it does not, the attribution reopens automatically.** And instrument the propose→gather→attest legs on that run (the #327 debug path exists) so a miss names its slow leg instead of re-guessing. One wire-only interaction worth one glance while instrumented: whether the sybil cohort's own attestation traffic competes with the honest gather inside the commit window (contention, not sizing — still §1-class, but worth seeing named in a log).

## 3. Item A1 — the stamp shape is right; don't let it overclaim "no channel"

The claim as drafted is sound: fixed canonical signer set + concurrent transport is privacy-neutral, and first-k-to-reply is the forbidden variant. Two second-order channels exist and should be *named in the stamp as pre-existing*, not discovered later by the red team:

- **Issuer-side timing correlation.** k colluding issuers seeing k near-simultaneous blinded requests can correlate them into one publish session. True — but the *serial* gather leaks the same session (an ordered fingerprint spread over seconds, arguably more distinctive), and session-to-IP linkage is already the **owned D-PRIV residual** (the IP+timing link, open until D3/H8 issuance-mixing). Parallelism does not add a channel; it narrows an existing one's window. The stamp should say exactly that — "no *new* channel; the IP+timing residual is pre-existing and owned" — rather than "privacy-neutral" unqualified.
- **Retry asymmetry** (one issuer sees a retry, others don't) — subsumed by the same residual; bounded-single-retry keeps it negligible.

## 4. Item A2 — prefer idempotent re-presentation over fresh re-blind (my recommendation; research confirms)

The blind-sign operation is textbook RSA-FDH (`core/blindtoken`): signing is a deterministic modular exponentiation of the blinded message — **re-presenting the same blinded serial to the same issuer yields the identical signature**, so re-presentation is naturally idempotent and mint-safe. Fresh-re-blind-on-retry is also *safe* (an abandoned signed-but-unreceived blind serial is unspendable — the client discards the unblinding factor) but it has a cost bug: token issuance spends a **prepaid blind credit**, so a lost *reply* after a successful sign would double-charge the publisher one credit per retry. Recommendation: **retry re-presents the same blinded serial; the issuer dedups (sign + credit-spend) keyed on the blinded-serial hash.** Research to confirm the issuer-side spend accounting can key on that hash; if it can't cheaply, fresh-re-blind is the acceptable fallback with the double-charge documented.

## 5. Item B2 — **do not confirm as framed.** The "no stall power between young and handed-off" belief fails *at the handoff instant*, and the recorded residual's price is wrong under member counting.

The consult asks research to confirm "the launch phase provably has neither capture nor stall from un-matured bonds… is there any regime between young and handed off where an un-matured bond could acquire stall power? **We believe not.**" The launch-phase half is right (the repro proves it). The boundary is not:

**The mechanism.** Post-handoff, quorum is sized by **member count, not weight**: `validatorSetSize()` returns `len(c.epochSet)` (`core/chain/chain.go:753`) and `bftThreshold(n)` takes that count. The epoch snapshot is drawn from the qualified bonded set at handoff. A cohort does **not** need to cause maturity — it rides along: honest validators mature the network (the C2 domain-diversity bar governs *maturity* and *capture weight*, not epoch *membership*), and at the first snapshot every qualified bonded identity — including 8 un-matured minimum-size sybil bonds — becomes an epoch **member**.

**The arithmetic, on the consult's own topology:** first epoch set = 4 honest + 8 sybils = 12 members → `bftThreshold(12)` = 12 − ⌊11/3⌋ − 1 = **8 non-proposer attestations**. The honest side can produce at most 3 (+proposer). The cohort simply *declines to attest* — no equivocation, nothing slashable — and **the mature phase is born unable to commit.** Stall, at the price of **8 × MinBond** — the *cheapest possible bonds* — not "⅓ of real bonded weight."

**Two consequences:**
1. **The owned-residuals draft entry is mispriced as written.** "Stall costs ≥⅓ of real, decaying, sealed disk (C1-priced)" is only true under **weight-counted** quorum. Under member counting, stall is priced per *head* at MinBond each — a Sybil-shaped liveness attack that C1's whole architecture exists to forbid (N cheap identities buying what should cost real resource). Hold the residual entry until the quorum-counting question is ruled.
2. **The fix direction to rule on (research's call, but the B8 argument is one-sided):** silt's own comment block cites its lineage — "Tendermint/Casper both fix the set" — but both of those count **stake/weight** in the quorum, not members. The settled pattern (B8: adopt, don't invent) is a **weight-counted mature-phase quorum**: a commit needs attestations carrying ≥ the Byzantine threshold of the epoch's *frozen bonded weight* (the `epochSet` map already stores per-member weight — `chain.go:1827` sums it), so 8 MinBond sybils hold weight proportional to what they actually paid, and the ⅓-of-weight residual pricing becomes true. Alternatives (filtered epoch admission — top-K by bond, domain-diverse) are second-order and can compose later; the counting basis is the load-bearing decision.

**Sequencing consequence:** this puts a **real consensus change back on the table before the external gate** — the exact thing Item B hoped to close as "no change." Better found by us than by the red team; seam #8 in the brief gains its sharpest sub-target ("stall-at-handoff via cheap-member epoch flooding"), and the §4 maturing-topology field flow must include a post-handoff commit **with the sybil cohort present and declining to attest**, which is the drill that would have caught this on the wire.

---

*Net: stamp A1 with the owned-residual qualifier, answer A2 with idempotent re-presentation, close B1 conditionally on the named field prediction — and hold B2. The launch phase is clean; the handoff instant is not, because member-counted quorum hands a MinBond-per-head cohort a stall lever that weight-counted quorum (silt's own cited lineage) prices correctly. That counting decision is the one genuinely open consensus question left in the M0 candidate.*
