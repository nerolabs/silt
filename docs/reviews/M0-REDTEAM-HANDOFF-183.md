# silt M0 — External Red-Team Handoff (#183)

**Audience:** an external, independent red team engaged to attack silt's M0
Sybil-resistance claim. **Status of this document:** the engagement brief and
rules of engagement. **Read `docs/design/m0.md` §7–§8 and `docs/TENETS.md`
Part 0 first — this handoff points at them, it does not replace them.**

> **⚠ GATE STATUS (updated 2026-08-24; read before scheduling).** The formal
> #183 entry criteria (`docs/release-checklist.md`) are now **ALL GREEN**:
> - ✅ Consensus model-check green (#406) — `go test ./core/{node,chain} -run ModelCheck`.
> - ✅ netem adversarial suite deterministic-green **10/10 consecutive** (`SUITE=all` under `delay 80ms 20ms`).
> - ✅ #399 WS-checkpoint recovery drill — field-confirmed (runs 585c82a, 45da13c).
> - ✅ **#535, the h64 epoch-boundary liveness wedge — recovery stack COMPLETE.**
>   The stall (> ⅓ of *frozen* epoch weight offline wedges the boundary; repro
>   `core/chain/modelcheck_535_boundary_wedge_test.go`) is research-ruled
>   *correct* safety-first BFT (certification:
>   `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/535-epoch-boundary-liveness-cliff-RESEARCH-CERTIFICATION-2026-08-23.md`);
>   the defect was non-recovery, now closed by the certified stack: **layer (4)**
>   the R-gate restore exemption (PR #541, heals a returning member); **layer (2)**
>   automatic boundary re-basing REFUTED by the proof-first model-check and
>   closed (PR #543 — the no-re-basing stall is the safe behavior, permanently
>   pinned); **layer (3)** the operator-directed weak-subjectivity liveness-floor
>   escape (`-liveness-recovery-height`, PR #545) for a genuine loss that does
>   not return, with the extended #535 model-check green
>   (`core/chain/modelcheck_535_fix3_recovery_test.go`: recovery-when-invoked,
>   off-by-default stall, replica determinism). The residual — a wrongly-invoked
>   recovery can fork — is the documented, off-by-default WS operator trust.
>
> The engagement-start decision now rests with the owner (the doc's entry
> criteria are met). The known-open list (§6) keeps you from re-discovering
> what is already on the board.

---

## 1. The one thing to attack

**silt claims to *hold* the privacy × accountability × Sybil trilemma without
trading any corner away.** The live edge — where the novel contribution
concentrates and where your effort belongs — is **Sybil-resistance**, stated as
a **systemic** claim, not a per-primitive one:

- **C1 — no discount:** no strategy earns a fraction *q* of consensus-controlling
  standing for less than ≈ *q* × `C_honest`, where
  `C_honest = disk × address-diversity × time × served-demand` (the
  non-substitutable resources an honest provider pays).
- **C2 — no quiet capture:** the objective concentration metric keeps the minimum
  colluding **operator** set that reaches quorum capture above a target *k*,
  sampled Byzantine-robustly.

**M0 is *held* iff you cannot deny C1 or C2 at the network's declared
parameters.** "Held," not "closed" — some seams are bounded-in-tension by
design, and the spec says which (§6 below, and `m0.md` §7). A seam *held in
tension* with a documented residual is a **pass**; a seam **silently assumed
closed** that you break is the **finding**.

## 2. What NOT to waste time on (retired as units of test)

Per Douceur, no single primitive can be "Sybil-proof" under free identity
minting. So these are **out of scope as findings** (expected-true, not defects):

- "Is this bond / VDF / PoR Sybil-proof in isolation?" — no, by theorem. The
  guarantee is the *composition*, not any part.
- Breaking an adopted primitive's published security (Wesolowski VDF,
  Shacham–Waters PoR, DRSample DRG, Chaumian blind signatures) — if you break the
  *cryptography*, that's a cryptography result, report it, but it is not the
  target. The target is the **composition**.
- Findings against **off-by-default / opt-in** mechanisms that the shipped
  default config does not enable — unless you can show the *default* posture is
  unsafe. (Test the shipped defaults; that is where prior passes found real gaps.)

## 3. The real attack surface — the seven seams (m0.md §7)

Break composed systems **at the seams.** Each is a target with a pass condition;
full text in `docs/design/m0.md` §7. Staged along the axes (`m0.md` §8):

1. **Re-pricing vs. the wealth residue.** C1 does nothing against an actor who
   *honestly* provides ~40% of real storage. Does C2's shed/concentration metric
   bound the colluding-operator set below capture under **adversarially-skewed
   measurement**? *Attack:* skew the size/clustering signals; split real stake
   across NodeIDs to beat the operator-margin. *Note:* the weight numerator is
   computed from the **committed on-chain bond ledger, not gossip** — the
   gossip-skew half is already closed; what remains skewable is the off-chain
   size/clustering signal. **Most likely where M0 is *held*, not closed.**
2. **Cold-start / scaffolding capture.** Can you capture the anchor-scaffolded
   young regime before it matures and sheds (the `everMature` one-way latch)?
   *Pass:* maturity is reached before any feasible capture; anchors are plural +
   threshold so none is load-bearing. **Sharpened by C-1 (research certification
   2026-08-27,
   `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C1-maturity-before-capture-RESEARCH-CERTIFICATION-2026-08-27.md`):**
   the live seam is **R1 — pre-maturity acquisition** (accrue a controlling *real*
   bonded fraction before the latch trips; your own weight then trips `Mature()`).
   Two adjacent seams are **CLOSED — do not re-litigate:** **R2**, handoff-instant
   head-count-quorum capture (8×/9×MinBond), shut by weight-counting
   (`requireEpochWeightQuorum`, `quorum_weight_test.go`); and **R3-safety**, the
   de-maturation super-quorum (safety preserved by quorum intersection). Attack the
   *price* of R1 (can you reach >⅔ live bonded weight for materially less than ⅔ of
   honest, sustained, address-diverse provision, and before honest arrivals dilute
   you?), not the *permanence* — the latch bounds R1's consequence, not its
   reachability.
3. **Self-dealing vs. real demand.** The anti-wash rests on real demand
   outweighing fabricated. *Attack:* the ratio of wash to attested-real demand you
   can manufacture. (Note the firewall — §6 — currently gives demand **no**
   consensus consumer, so this is latent until D-DEMAND fuses; attack the
   *forgeability*, not the price today.)
4. **Privacy ↔ attribution linkage.** Can a colluding validator subset
   de-anonymize a fetcher, **or** mint demand receipts with zero served bytes?
   (Both residuals are known — §6 — and neutralized *today* only by the firewall.)
5. **Operator-clustering heuristic.** Address-diversity counts address groups;
   "5 operators" vs "1 operator, 5 /24s" is heuristic. *Attack:* real cloud/AS
   diversity that evades operator-clustering cheaply.
6. **Time-axis gaming.** Standing-as-time-integral is beatable by banking
   reputation then defecting intermittently. *Attack:* tune to the audit/decay
   window (rolling-window + hysteresis is the defense; no proven-optimal window).
7. **New liveness / griefing surfaces.** Composed rules create new stalls
   (withholding, refusing to attest, boundary races). *Attack:* can a minority
   grief liveness under the fused rules? **This is where #535 lives — and where a
   red team is most likely to land a real, non-adversarial finding fast.**

Also run, as the staging ladder: (i) per-axis reuse attacks (one disk → many ids;
one IP → many nodes; instant standing; self-dealt content) — each should be
denied by its part; (ii) **cross-axis seam attacks** (above — the prize); (iii)
concentration/shed attacks vs C2; (iv) cold-start capture.

## 4. Environment & how to stand up a target

- **Repo:** the full source (Go). Build: `go build ./...`; whole test suite:
  `go test ./...`; race on the consensus core: `go test -race ./core/node ./core/chain`.
- **Deterministic consensus model-check** (`docs/design/consensus-model-check.md`,
  `core/node/modelcheck_*_test.go`, `core/chain/modelcheck_*_test.go`) — the I1–I5
  invariant harness under adversarial scheduling. **This is where you can
  *schedule* an attack deterministically** (equivocation, partition, boundary
  races, weight skews) without a cloud. If you find a safety/liveness break,
  landing it as a failing model-check schedule is the highest-value artifact.
- **In-process simulator** (`silt sim run …`) — many-node behavior, deterministic,
  free.
- **Local Docker field harness** (`integration/*/run.sh`) — real processes, disk,
  sockets, TLS, NAT matrix. `integration/consensus/run.sh` is the partition→heal
  drill. (Note: `integration/consensus/run.sh` currently stalls pre-genesis on this
  host — tracked as #530; the in-process `e2e/partition_test.go` covers the same
  property meanwhile.)
- **GCP field harness** (`integration/cloudtest`) — real multi-region VMs, real
  WAN/NAT/scale. This is the RC gold standard. Run the **economy-ON**
  configuration (`SYBILS=8 MATURING=1 ECONOMY=1`) — the network people will
  actually run. `MATURING=1` sheds the anchors so you attack the **post-shed
  mature regime** (the sharpest seam target). Billable; coordinate before
  spending.
- **The claim surface to read:** `docs/design/m0.md` (as-built map + seams),
  `docs/design/consensus-invariants.md` (I1–I5, the closed BFT-safety set),
  `docs/design/owned-residuals.md` (the named, deliberate residuals), `docs/TENETS.md`
  Part 0 (M0 + C1/C2) and Part IX (the immutables).

## 5. Rules of engagement

- **Attack the systemic claim (§1), at the shipped defaults, at declared
  parameters.** A finding is: *a strategy that earns consensus-controlling
  standing for < q·C_honest, OR concentrates past capture (C2), OR breaks a §7
  seam that the spec assumes closed, OR stalls/forks a connected honest network.*
- **"Held" is a pass.** If a seam is documented as held-in-tension with a bounded
  residual (§6, `owned-residuals.md`), demonstrating the residual is *confirmation*,
  not a finding — unless you exceed the stated bound.
- **Every finding ships a reproduction.** Best: a failing model-check schedule or
  a deterministic sim/netem repro. Acceptable: a scripted field repro with the
  observed on-chain/on-wire state. State the **severity** (does it break C1, C2, a
  safety invariant, or liveness?) and the **seam** it lives at.
- **No destructive action against infrastructure you don't own**; the GCP harness
  is self-contained and auto-torn-down. Coordinate billable runs.
- **Independence:** attack code + public docs + this handoff. You are the external
  check M0's "held" verdict requires (B8) — self-grading does not count.

## 6. Honest current state — what's shipped, held, and already open

**Do not re-discover these.** Report *new* seam breaks or *exceeded* residual
bounds.

**Shipped and internally hardened** (`m0.md` §6): identity-bound
proof-of-space-time bond (sealed plot × Wesolowski VDF, persisted — N Sybils cost
N real disks), verify-without-fetch proof-of-retrieval, standing as the
time-integral of bond + audit, objective on-chain-bond fork-choice with
partition→heal, provable equivocation slashing, the C2 concentration metric
computed from the committed ledger, weight-counted mature-epoch quorum, blind
publish tokens (publisher-unlinkable), the S7 durability economy (escrow + serve
auto-skim + verified proof-of-correct-repair).

**Held in tension, by design** (not defects — `m0.md` §7, `owned-residuals.md`):
- `C_honest ≈ D` (disk axis) is what is *in force today*; `D×A×T×B` is the
  *target*. The served-demand axis (B / D-DEMAND) is **built but firewalled off
  from standing** — demand is a neutral observable with **no consensus consumer**.
  Two receipt residuals (a demand receipt is forgeable with zero object bytes; a
  bonded-mode receipt links fetch→standing key to one validator) are **open and
  must close before any demand→standing fusion**, or they re-open the γ→1/N hole.
- Address-diversity (A) lives in the DHT layer, not yet in the standing number.
- Time (T) ships for **retention only** (decay/TTL); there is **no
  acquisition-time accrual** (a bare age gate is pre-farmable — deliberately not
  shipped).
- The C1 claim is therefore **conditional** (`m0.md` §3, §7, §10). The
  honest-whale wealth residue is **bounded by C2**, not eliminated.

**Known-open findings (already on the board — not fresh territory):**
- **#535 — the h64 epoch-boundary liveness wedge (recovery stack COMPLETE; see
  the gate-status block).** The stall itself is certified-correct safety-first
  BFT; recovery is fix (4) for returning members + the operator-directed
  fix (3) WS escape for genuine loss. The documented residuals — a
  wrongly-invoked recovery can fork (WS trust), and a mid-epoch (non-boundary)
  > ⅓ loss is recovered by a coordinated WSCheckpoint restart, not in-protocol
  — are on the board; probing them is fair game, re-reporting them is not.
- **#530 — the Docker consensus e2e stalls pre-genesis** on some hosts (harness,
  not product; in-process coverage stands in).
- **#299 — the ~1.5 MB bond answer is a parameter question** (k label opens), not
  an encoding one; a research consult on the sound k-floor is open. The N²
  bandwidth cost is real and known.
- The **shared-content sealing boundary** (`m0.md` §10, research frontier):
  plain PoR over *shared* erasure-coded shards lets one physical copy answer for N
  pledges (γ→1/N). silt is **not exposed today** (standing comes from a dedicated
  identity-keyed bond plot, not shared shards) — but fusing served content into
  standing without leaking γ→1/N is the highest-leverage open research question.
  If you find a *shipped* path that fuses them, that is a real finding.

## 7. Deliverable

For each finding: **(a)** one-line claim (which of C1 / C2 / a safety invariant /
liveness it breaks); **(b)** the seam (§3) or "new"; **(c)** a reproduction
(model-check schedule preferred; sim/netem/field acceptable) with observed state;
**(d)** severity and the parameters at which it holds; **(e)** whether it exceeds
a documented residual bound or breaks a seam assumed closed. A final summary:
does M0 **hold** at the declared parameters, or not — the yes/no an outsider can
check.

## 8. The loop this terminates

Either a seam falls (real, localized, fixable) → the build team triages and fixes
to the three-tier bar with your PoC inverted as a permanent regression → you
re-verify; or the system holds at stated parameters → **M0 held**. Both are
progress. The engagement is over when a fully-informed pass reaches diminishing
returns against the shipped defaults and the seam residuals are either closed or
explicitly accepted as held.
