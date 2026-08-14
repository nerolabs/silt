# Research consult — decisions raised by the M0 red-team (blind pass, 2026-08-08)

**From:** silt build team
**To:** research team
**Re:** the second external blind red-team pass
(`silt-reviews/redteam-august-8th/findings/`, target `main @ bd22f31`, verified
against current `main @ f952d10`).

**Why this doc exists.** The red team found **two P0 breaks + one silently-assumed-
closed seam**, plus several P1 residuals. Most are mechanical fixes the build team
will land directly (see §6, "building without consult"). This brief carries only the
subset that needs a **research decision or advice** — because it touches a **claim we
publish** (C1's `(1−o(1))`), a **primitive soundness argument you own** (the labeling
opens, `m0-sybil-rebind §8.1`), a **roadmap gate** (the non-globality metric →H9),
or a **privacy model** (publisher anonymity set). For each, we give the mechanism with
code refs, why it is research's call and not a build call, the options, our
recommendation, and the specific ask.

**Ground truth first (so you calibrate the stakes).** The composition's load-bearing
severance held under every attack: the **demand→standing firewall is airtight**
(confirmed three independent ways — 50 self-dealt receipts and 8 zero-byte forged
receipts both moved standing by exactly 0), **C2 held** (concentrating past capture
cost 21 distinct real min-bonds = `q·C_honest`, no shortcut), and prefix/shared-plot
Sybils, γ→1/N-on-the-bond, identity binding, and fork-choice convergence are all
genuinely closed. The breaks below are **not** cryptographic collapses — they are a
**bounded constant-factor discount** and a set of **off-by-default shipped configs**.
Calibrate accordingly: we are asking you to help us close a ~15–20% seam and decide a
claim-wording question, not to rescue a broken design.

---

## Decisions requested (summary)

| # | Decision | Touches | Our lean |
|---|---|---|---|
| **R-1** | How to close BREAK 1 — the partial-storage recompute discount on C1 | the **published C1 claim** + labeling-open soundness (`m0-sybil-rebind §8.1`, your owned external-derivation item) | Pursue the pebbling-path harden if tractable for M0; else own `ε*` **with enforcement** and restate C1. **We need you to tell us which.** |
| **R-2** | Ship the SurvivorNakamoto non-globality metric in M0, or defer to H9? | immutable-#5 availability promise + roadmap gate (#180/H9) | Ship the raw scalar in M0 (you already CONSTRUCTED the metric, research-brief A2); keep the ZK/PIR wrapper for H9. |
| **R-3** | Is a canonical validator signer-set enough for publisher privacy, or is the B2 blind publish-token needed for M0? | D-PRIV publisher-anonymity model | Canonical-set default closes the reported collapse for M0; B2 blind token stays the H8 target. Confirm. |
| **R-4** | Is "refuse-to-start" the correct safe default for cold-start, or is a synthesizable bootstrap anchor default sound? | seam-2 cold-start / immutable-#3 scaffolding model | Refuse-to-start (mirror `-min-bond<=0`). Light-touch confirm. |

Everything else from the pass (§6) we build without a consult.

---

## R-1 (primary) — BREAK 1: C1 admits a bounded ~15–20% partial-storage recompute discount

**Severity high, confidence high, independently found by two personas (Sybil-farm,
Liar-prover), re-verified by us against live code.**

### The finding
A prover seals the honest v3 plot, **deletes an ε fraction of the blocks** (keeping the
32-byte Merkle leaves, which fix the committed root), and on challenge **recomputes**
any dropped block on demand from its predecessor + 3 DRSample parents. It then answers
the possession opens, seed-block read, VDF, and the `k=64` labeling opens, and passes
the exact `bond.VerifySpaceTime` the live wire calls. `RecordBondChallenge` mints
standing at the **advertised** size with no residence check
(`core/credit/credit.go:189` — verified: `a.bondedBytes = provenBytes`), and objective
weight `c.bonded[id] = r.Size`. So the prover holds ~0.80–0.85 of the real disk and
collects the full `q` standing → cost ≈ `0.80–0.85·q·C_honest`. C1 requires
`≥(1−o(1))·q·C_honest`; a fixed ~15–20% constant is **not** `o(1)`, so C1 *as literally
written* is false. It is **bounded** (collapses past ε≈0.25) and does **not** amplify
(not one-disk-many-identities) — hence high, not critical — but it sits on the only
axis gating consensus today, and every Sybil in a farm takes the same cut.

Measured (256 MiB plot, `BondVDFDelay=1000`, `k=64`), re-run by the red-team lead:

| ε | disk saved | recompute/answer | wall-clock | fits 500 ms window? |
|---|---|---|---|---|
| 0.05 | 12.8 MiB | 20 blocks | 0.30 ms | yes |
| 0.10 | 25.7 MiB | 52 blocks | 0.71 ms | yes |
| 0.15 | 38.2 MiB | 137 blocks | 1.9 ms | yes |
| 0.20 | 51.5 MiB | 394 blocks | 5.1 ms | yes |

Recompute depth: ε=0.10 → avg 1.3; ε=0.30 → avg 32; ε=0.50 → avg ~1800. Knee ≈ ε=0.25.

### Why it defeats the shipped defenses (the part that is yours)
This lands **directly on `m0-sybil-rebind §8.1`**, which you already own as "tight
`ε→k` … needs the DFKP'15/Fisch'19 pebbling reduction instantiated for silt's exact
graph. **External derivation required (B8) before fixing production `k`.**" The red
team sharpens the open item into a *live consequence*:

1. **The §3 soundness premise is empirically false at the ε that matters.** §3 argues
   "recomputing a random missing label costs `Ω(N)` sequential pebbling — which the
   read-bound VDF window forbids." Measured over silt's actual indegree-4 DRSample +
   chain graph, at ε≤0.10 nearly every missing block's predecessor+parents are still
   resident, so recompute terminates at **depth ≈1**. The `Ω(N)` regime only appears
   near ε≈0.5.
2. **The `1−(1−ε)^k` catch bound is vacuous against a recompute-capable prover.** It
   bounds the probability an open lands on a *missing* node — but a recompute prover
   *regenerates* the dropped block and produces a **correct, Merkle-valid** open;
   `verifyLabels` checks label *consistency* (a public predicate), which recomputed
   bytes satisfy perfectly. `k=64` catches only a prover committing *wrong* bytes.
3. **The anti-release floor (`§4`, `MinBondBytes ≳ c·B·W`) prices only a *full*
   re-plot.** A partial prover never re-plots the whole size — it recomputes the
   challenged closure, O(1)–O(100) blocks at low ε, independent of plot size.

So the soundness argument's own two halves (labeling opens + anti-release floor) do
not bound *partial* release-and-recompute. §8.1 flagged the constant; it did not flag
that the Ω(N) premise fails at low ε or that the floor's threat model misses partial
recompute.

### The decision (why it is yours, not ours)
Two fix families, and picking between them **changes a claim we publish**:

- **Option A — restore the intended space-time semantics (harden the opens).**
  Challenge, for each sampled node, a **full pebbling path to a stored checkpoint**, so
  a dropped block forces `Ω(d)` sequential recompute the read-bound VDF window can
  bound. This keeps C1 as `(1−o(1))`. **The open question is whether this is
  instantiable for M0 in pure Go on silt's exact indegree-4 DRSample+chain graph, with
  a *sound, derived* `ε*→k→d` relationship** — i.e. the very DFKP'15/Fisch'19 reduction
  §8.1 says needs external derivation. If it is tractable and cheap enough on-chain
  (proof size is already "heavy on-chain" at `k=64`, §6/§8.2 asymmetric-`k`), this is
  the clean fix.

- **Option B — own a bounded slack honestly, *with enforcement*.** Pin a documented
  storage-slack `ε*`, restate C1 as `≥(1−ε*)·q·C_honest`, **and** add an enforcing
  residence check so a prover beyond `ε*` actually fails the audit (restating the claim
  without enforcement does **not** close the break). This is analogous to the
  "finite-but-renewable `g`" move on durability: convert a silent gap into a legible,
  parameterized residual. It is shippable now; it concedes a bounded discount into the
  claim.

**Our recommendation:** prefer A **if** you judge the pebbling-path challenge derivable
and on-chain-affordable for M0; otherwise B with a research-blessed `ε*` and an
enforcing check. We can build the labeling-open **cost probe** on silt's real graph to
give you exact depth/proof-size numbers before you decide — say the word.

**Specific asks:**
1. Is Option A's pebbling-path challenge instantiable for M0 (sound reduction +
   pure-Go + affordable on-chain proof), or is it an H-track research construction?
2. If B: what is a defensible `ε*` at the shipped parameters, and do you bless
   restating C1 as `(1−ε*)`? (You own the composition proof; this is a claim edit.)
3. Either way, confirm the **enforcing residence check** shape so we land the red
   team's `c1_recompute_regression_test.go` (RED today) as a permanent green gate.

**Related owned items:** `m0-sybil-rebind §8.1` (ε→k), §4 (floor), §6/§8.2 (asymmetric
`k` on-chain proof size). Field-team defect **#234** ("bond release-and-recompute
shortcut … + C1 no-discount headline scope") is the same seam, now confirmed and
quantified — this consult supersedes the "no live-daemon seam" framing there.

---

## R-2 — BREAK 2 residual: ship the SurvivorNakamoto non-globality metric in M0, or defer to H9?

**The break itself is a build item** (BREAK 2: `DHTDomainCap=0` in
`node.DefaultConfig()`, set only on the daemon path, so `cmd/silt/client.go:87`'s
desktop client ships the H5-B eclipse defense off → a key-surround censor can make a
root undiscoverable). We will default `DHTDomainCap>0` in the client path (§6). **The
research question is the residual:** even hardened, a censor spread across enough
failure domains still censors — a **PASS under M0 only if the cost is legible.** But
the non-globality metric (`SurvivorNakamoto` over failure domains) is **prose in
`docs/safety-denylist.md`, with no `core/` implementation** (confirmed via grep). The
public doc reads more finished than the code.

Per the accepted research commission (`research-brief.md` A2), the **non-globality
metric is CONSTRUCTED** — so the raw scalar is a build target, and only the ZK/PIR
wrapper is genuinely H9 (`core/translog`'s own comment already disclaims that layer as
post-M0).

**Options:** (i) ship the raw `SurvivorNakamoto(root)` scalar over the live provider
set's failure domains in M0 (data exists: signed provider records carry identities,
domains are gossiped), keeping the ZK/PIR wrapper for H9; (ii) defer the whole metric
to H9 and **soften `safety-denylist.md`** to "designed, not built" so the doc stops
over-claiming.

**Our lean:** (i). The break's own detectability argument depends on it — a root whose
provider set has collapsed to one domain is *visibly* near-global the moment the scalar
is published, turning silent routing censorship into a measurable signal, which is
exactly immutable-#5's "no global switch, and we can prove it" promise.

**Ask:** ship the raw scalar in M0, or defer + soften the doc? If ship: confirm the
metric definition (survivor Nakamoto-coefficient over failure domains) matches the
construction from the commission so we implement the blessed form. Tracks #180 (H9).

---

## R-3 — seam-4: is a canonical signer-set enough for publisher privacy, or is B2 needed for M0?

**Finding (medium):** the committed `PublishToken.Sigs` records **each signing
validator's NodeID**, and the shipped `swarm add` signs from the **caller's own
`-peers`**, so the signer-subset is publisher-chosen and varies per publisher. A rare
subset collapses the publisher anonymity set to a **singleton** (full deanonymization,
no broken crypto — the blind signature still hides the on-chain *serial*; the leak is
the *subset selection*). The red team reproduced canonical-set (advantage 0) vs.
`-peers` (singletons) regimes.

This is **partly owned** — `core/publishtoken.go` names the anonymity-set narrowing and
the "canonical validator set" mitigation as documented; what is *not* owned is that the
shipped CLI **doesn't apply** it. It also intersects the locked-but-unbuilt **B2
blind-signed publish token** direction ([silt-publisher-privacy], risk-14).

**The research question:** for M0, is defaulting `-token-quorum` to a **network-
canonical validator set** (e.g. top-k by committed bond, deterministic from the ledger)
sufficient to hold the publisher-privacy promise at stated parameters — or does M0
require the full **B2 blind-signed publish token** (issuer signs without learning which
root it authorized) to sever the on-chain signer-subset quasi-identifier?

**Our lean:** the canonical-set default closes the *reported* collapse (every publisher
in an epoch shares one subset → advantage 0) and is a small CLI/default change we can
build now; B2 stays the H8 privacy-track target. But the privacy *model* call — whether
canonical-set is a genuine M0 hold or just narrows the set — is yours.

**Ask:** bless canonical-signer-set-by-default as the M0 hold for seam-4 (we build it +
make it the CLI default), or scope B2 blind publish tokens into M0? Tracks #179 (H8).

---

## R-4 — seam-2: is "refuse-to-start" the correct safe default for cold-start?

**Finding (high, SILENTLY-ASSUMED-CLOSED):** a stock untrusted objective validator
ships with `-anchors`/`-anchor-quorum`/`-mature-validators` all default 0/empty. With
`MatureValidators<=0`, `Mature()` returns true (`core/chain/chain.go:793`), so the node
**latches `everMature=true` at genesis** and never imposes anchor co-sign — the
cold-start scaffolding the design leans on for the young regime simply never engages.
Guarded only by a *liveness* WARNING. The Invariant-B enumeration
(`cmd/silt/invariant_b_test.go`) has rows for S1/S3/S4 but **no cold-start row**.

(Note: the *prior* red-team F-1 — the two-way maturity gate — is genuinely fixed. The
`everMature` one-way latch and the de-maturation super-quorum are shipped, and this
pass confirms both hold. seam-2 is a *different* angle: the machinery is sound but
off-by-default.)

**The build fix is clear** — refuse to start on the untrusted objective posture when
cold-start params are unset (mirror the existing `-min-bond<=0` hard failure), plus a
cold-start row in the Invariant-B enumeration. **The one design question worth your
confirmation:** the red team argues "there is **no safe synthesizable default** for the
anchor *set*, so the safe default is *refuse*, not *run open*." Do you concur, or is
there a sound synthesizable bootstrap (e.g. a self-anchoring genesis quorum, or a
maturity default keyed off observed bond diversity) that would let an untrusted
validator boot safe *without* an operator-supplied anchor set? This touches immutable-#3
(anchors = time-boxed launch scaffolding).

**Our lean:** refuse-to-start. It is the honest reading of "safe config is the default"
and there is no obviously-sound synthesizable anchor set. We will build it unless you
see a bootstrap default that preserves the cold-start guarantee.

**Ask:** confirm refuse-to-start as the safe default (light-touch — we proceed on this
unless you flag a sound alternative).

---

## 6. Building without consult (scope, for your awareness — no decision needed)

These are mechanical fixes with an unambiguous safe answer; we land them as PRs with
the red team's PoCs inverted as permanent regressions:

- **BREAK 2 client default** — default `DHTDomainCap>0` (and `RequireSignedProviders`)
  in the client path so `silt client` isn't eclipse-open.
- **seam-2 refuse-to-start** — hard-fail the untrusted objective posture with unset
  cold-start params (pending R-4 confirmation it's the right default).
- **Invariant-B enumeration extension** — add rows for cold-start, DHT-domain-cap, and
  publish signer-set to `cmd/silt/invariant_b_test.go` so an off-by-default seam fails
  the build (the systemic root cause the red team names — "off-by-default bit us 4×").
- **seam-5 F3** — implement the domain transport cross-check the `chain.go` comment
  claims, *or* correct the comment to "self-asserted, not transport-verified" (it
  over-credits the A-axis today).
- **seam-5 F1** — add a count/entropy concentration signal so equal-bond splitting
  (which reads HHI/Gini/TopShare ≈ maximally-decentralized) is visible to the alarm.
- **seam-7 F1** — slash equivocation on **detection**, not adoption: scan every fetched
  peer chain against the local one (`FindEquivocations(local, peer)`) before the
  `heavier` test, so a double-sign onto a *losing* fork is still punished.
- **seam-7 F2** — allow local objective-set pre-eviction on a valid gossiped
  `Equivocation` proof (self-verifying → sound), converging via the chain once a slash
  block lands, so an equivocator can't starve its own eviction.
- **seam-6** — make the on-chain bond-renewal nonce unpredictable (not `H(prev_block_hash)`)
  to close the coast-window widening.
- **Permanent tripwires** — keep the red team's demand-firewall and zero-byte-receipt
  tests as regressions (they go RED the instant demand is ever wired to standing), and
  extend Invariant-A's guard to any future *external* demand-reader (today it guards
  only the `*Ledger` mint surface).

**If any item in §6 turns out to hide a design fork once we open the code, we escalate
it into this consult rather than deciding it ourselves.**

---

## Provenance
- Red-team package: `silt-reviews/redteam-august-8th/findings/` — `BUILDER-REPORT.md`,
  `M0-REDTEAM-VERDICT.md`, seven `persona-*.md`, reproducing scripts under `scripts/`.
- 7 independent blind adversaries in isolated worktrees; off-limits to `docs/reviews/`
  and verdict banners; every headline finding re-verified by the lead and, for the two
  breaks + seam-2, re-verified again by the build team against live code (`credit.go:189`,
  `node.go:153/544`, `client.go:87`, `daemon.go:201/1110`, `chain.go:793`,
  `invariant_b_test.go`).
