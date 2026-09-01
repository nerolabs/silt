# silt — Tenets history

> Status: **companion to [`TENETS.md`](TENETS.md).** This file holds the
> *amendment log* — the dated record of how the tenets reached their present
> form — plus the product/mechanism war-stories that motivated each build
> principle. It was extracted from `TENETS.md` on **2026-09-01** when the tenets
> were restructured into pure guiding principles (build details look *up* to the
> tenets; the tenets never look *down* at the build). The living principles live
> in `TENETS.md`; the reasons, incidents, and dates live here.

---

## Why this file exists

`TENETS.md` states what "good" looks like as an *outcome*, abstracted to what
each principle *represents*. It deliberately carries no issue/PR citations, no
mechanism detail, no ship-status, and no dated history — a tenet does not change
when the product ships. Everything a reader needs to *audit* the tenets against
their origin lives here and in the design companions (`design/m0.md`,
`decisions.md`, `network-durability.md`, `build-process.md`, `network-protection.md`,
`ROADMAP.md`, `VISION.md`).

Each build-immutable was distilled from a **real, paid-for loss.** The lesson is
canon (in `TENETS.md`); the incident that taught it is recorded below, so the
principle can be judged against the scar that made it.

---

## The build-immutable scars (why each principle exists)

- **#3 — One signal, one job (never fuse transport with security).** Distilled
  2026-08-10 from the external network-durability-vs-space-time research opinion
  (`silt-reviews/research/…network-durability-vs-spacetime…`) that field-testing
  under adverse networks (`integration/flakynet`, #288/#289) provoked. It caught
  two real defects from one rule: the **C1 reply-latency gate** (transport RTT
  fused with the compute security signal) and the **#288 evict-on-one-miss**
  (routing reachability fused with consensus standing). Security lives in the
  proof object or in statistics-over-many, never in a single network number the
  adversary's own path can move — *latency proves proximity, never diligence.*

- **#4 — Cheap honest participation is a security constraint.** Added 2026-08-10
  from the same consult. Promotes silt's "cheap to run" mission into a veto over
  security designs: no fix may raise the floor of honest participation (scaling a
  min-bond off a transport knob, demanding GiB where MiB suffices is a regression
  against silt's reason to exist, even if it closes a real attack).

- **#5 — Build for the adverse internet (durability is the default).** Added
  2026-08-11 with its companion `network-durability.md`. Provoked by **#286** —
  the first full multi-region GCP field test found a fresh quorum-2 objective
  chain wedged at genesis because a flat transport deadline couldn't carry the
  one-time ~1.5 MB bond-registration block across the WAN; the fix (a size-aware
  transport deadline) is exactly the "generous, payload-scaled transport
  deadline" the research prescribes. Both #286 and #288 (a flat deadline +
  evict-on-one-miss starved consensus under loss) were this law unlearned.

- **#6 — Root-cause before you patch (attribute before you ship).** Added
  2026-08-12 with its companion `build-process.md`, from the research team's
  `build-process-root-cause-first-ADVICE.md`. Provoked by the repeated **#286**
  WAN loop: a handshake-EOF "fixed" with a fixed-constant timeout (real cause: no
  outbound addresses), and a billable multi-region run spent to *discover* the
  ~8 MB genesis block that was deterministic and reproducible in-process. The
  correct genesis fix (spread bond regs; commit small via the already-existing
  `launchAnchor` anchor bootstrap) was *unused-correct code* — the grep-first
  poster child.

- **#7 — Evidence or nothing (no forward step on a hypothesis).** Ratified
  2026-08-14 after watching the **guess → act → fail → guess** loop cost hours
  and billable cloud runs across multiple sessions. Provoked by the canonical
  case: a billable P1 cloud run whose sybil cohort **crash-looped**, and the
  harness tore the VMs down *without capturing their journals* — leaving only
  guesses about the cause. The corrective was to make the harness **capture the
  evidence first, then look** (`integration/cloudtest` failed-node journal
  capture, PR #394).

- **#8 — Fits the hobbyist box (~1 vCPU / 2 GB, measured before you commit).**
  Ratified 2026-08-18. Provoked by the most expensive near-miss: **#299**
  succinct bond proofs, which read as ideal at ~192 bytes on the wire until
  research quantified the *prover* at one-Filecoin-PoRep-class (**128–192 GB RAM
  + GPU**) — two orders over the floor; citing the artifact size without the
  production cost nearly bought a week and a datacenter dependency. Reinforced by
  the MATURING OOM arc (unbounded inbound queue → proof map → chain retention),
  the same law unlearned on the *memory* axis.

## Other product war-stories the tenets distilled

- **S3 / Don't #4 (no silent-loss shape)** — **#46:** a publish that
  half-succeeded and stranded content with no retrievable link.
- **B7 (no optimistic operations)** — **#60:** a publish once returned a
  valid-looking link for content it never durably stored.
- **S5 (honest observability)** — **#43:** the network-size estimate counting
  dead ephemeral identities was a dashboard that flattered; it had to be fixed.
- **Registry persona (#5)** — the registry-as-cache-over-DHT stance is the crux
  of **#47/#48**.
- **Caretaker persona (#7) / S7** — the durability-as-a-sellable-service gap is
  **#44**; the γ→1/N shared-content sealing boundary and identity-keyed PoRep are
  the single core research problem, **#182**; the served-demand standing axis is
  **#181 (D-DEMAND)**.

---

## Amendment log

- **2026-08-01** — Ratified as canon (#54). Reviewer questions resolved:
  persona #1 is "Aslan users" (name kept); application-operator (#12) is a
  distinct seat from developer (#11); trust plane is a V1 pillar (Open Q#3
  closed).
- **2026-08-02** — Added **M0** (mission-immutable, the trilemma) as Part 0 and
  the supreme immutable; restructured Part IX into three tiers (immutable /
  tenet / evolving) with six mechanism-immutables; **moved B7, Don't #7, and R4
  out of the immutable set into the tenet tier** (they are disciplines/stances,
  not project identity); added **B8** (best components, novel composition);
  stated the load-bearing novel claim and the principle-not-mechanism exception
  explicitly.
- **2026-08-02 (intention review)** — Acted on the fresh-eyes *intent* review
  (`docs/reviews/fresh-eyes-2026-08-02-intention.md`). **Requalified M0** from
  "resolve the trilemma" to "*hold* it — refuse to trade any corner away," named
  which corner bootstraps (Sybil-resistance is weakest early and co-matures;
  privacy and accountability hold from day one), and **bound M0 to a falsifiable
  test** (held iff the external V3 red-team suite denies all three failure
  modes). **Reconciled "no center" with the training wheels**: immutable #3 and
  T1 now say "no *permanent* center — bootstrap scaffolding is explicit,
  time-boxed, and sheds on measured decentralization." **Added S7 — durability
  must pay for itself** (the repair loop funded in equilibrium, not charity; the
  Freenet/GNUnet wound). **Named the external adversary** in B8 and V3: the suite
  that certifies a novel composition (and M0) must be written by an outside party
  (audit / bounty / independent red-team), not self-graded.
- **2026-08-02 (build-immutables)** — Added **V5 — a bug fixed once stays fixed,
  and breaks are caught locally**: every discovered defect ships in the same
  change as a failing-first regression test at its tier(s), runnable on a
  contributor's own machine, so CI is the backstop and never the first line.
  Introduced the **build-immutable** category in Part IX — distinct in kind from
  the product-immutables (which define *what silt is*) but equal in standing
  (how we build) — and placed the three-tier Definition of Done (V1/V2) and V5
  there. Prompted by finding the integrated hole-punch gap (#27 Phase 3) *locally*
  via the Docker NAT harness, not at CI: the discipline that made that possible is
  now canon (Andrew).
- **2026-08-05 (composition thesis)** — Reframed M0's Sybil corner as a *systemic*
  claim held by **composition**, not a Sybil-proof primitive (per
  `docs/design/m0.md`, from the research capstone `09-m0-as-composition.md`).
  Defined the Sybil failure mode as **C1 (no discount)** + **C2 (no quiet
  capture)** and made it the V3 target; clarified in **B8** that the red-team
  certifies the *composition*, so a primitive failing a standalone Sybil-proof
  test is Douceur, not an M0 failure. **Fused S7 with the Sybil corner** —
  durability budget = Sybil budget = one ledger. Made immutable #3's young→mature
  maturation an explicit *bet* bridged by the shed metric (cost-to-corrupt /
  Nakamoto-over-operators). Named Sybil-resistance as *re-pricing +
  concentration-bounding*, not prevention, with the honest-whale residual
  *bounded* (not eliminated) by C2, and moved its parameters to the evolving tier.
  This is the reset that changes what "done" means for the Sybil corner: from an
  impossibility (a Sybil-proof primitive) to a reachable target (C1 + C2 at
  stated parameters).
- **2026-08-05 (deferred decisions)** — Recorded owner decisions derived from the
  accepted research package (see [decisions.md](decisions.md)). **D-PRIV:** amended
  immutable #4 from an absolute ("never observable") to *refuse-to-surveil*
  (absolute) + *access-unobservability held in tension* at the metadata layer (the
  anonymity trilemma is a hard wall; blob-layer unobservability is not
  guaranteed); Don't #3 and the Part 0 / persona echoes harmonized. **D-S7:**
  relaxed "no token" to "no *speculative external* token" and stated the durability
  funding model in S7 (internal escrowable credit reserve + auto-skim + rarest-shard
  bounty; standing stays coin-free); center-less proof-of-repair routed to research
  as the open construction. **D-TAKEDOWN:** immutable #5 now commits every honored
  revocation to a CT-style transparency log toward a formal non-globality
  guarantee. **D-DISCLOSURE:** added Don't #8 (no decryption backdoor at core).
  D-CRYPTO-AGILITY (post-V1) and D-ANCHORS (launch-config) are recorded in
  decisions.md only.
- **2026-08-06 (commission answers folded in)** — The follow-up research commission
  (`research-outcome/commission/`) answered the routed-to-research constructions.
  **D-S7:** center-less proof-of-repair **now exists** as a composition of proven
  parts (transparent polynomial-commitment correctness + Shacham–Waters
  retrievability + DAS quorum, no new primitive for plain-RS); S7 rewritten to say
  so, and durability restated as **finite-but-renewable, not "perpetual"** —
  perpetual solvency needs a positive credit-denominated cost decline `g` the 2020s
  no longer guarantee, so silt funds a renewable horizon and instruments `g`.
  **D-TAKEDOWN:** the formal non-globality metric now has a construction (survivor
  Nakamoto-coefficient + ZK threshold predicate revealing only `t`). **D-DEMAND
  (new):** standing is priced on **cost-to-wash, never receipt count** — the blind
  demand receipt gives unforgeable-delivery + fetcher-unlinkability but *not* demand
  authenticity (a Douceur limit); wash is re-priced, not proven away. The **core
  open problem** is now named precisely: the **shared-content sealing boundary**
  (plain PoR over shared shards leaks γ→1/N; silt is not exposed today because
  standing uses a dedicated identity-keyed bond plot, not the shared shards). Full
  detail in [decisions.md](decisions.md) and [design/m0.md](design/m0.md) §10.
- **2026-08-06 (external-audit honesty propagation)** — Two independent audits of the docs
  pass (a research *comprehension* audit + a red-team *intention* audit) found comprehension
  excellent but a **propagation gap**: the held-in-tension honesty that was correct in
  `m0.md §10` / issue #182 had not reached the tenet layer, the risk surface, or the public
  site, so three things read as *achieved* that are deliberately *open*. Fixed at canon: **S7
  reworded** — the "one ledger" (served-content ⇄ standing fusion) is the *design goal*, **not
  shipped**; today standing comes only from the dedicated bond plot, separate from served
  content, and fusing them is gated on the γ→1/N problem (#182). **Part 0 C_honest** marked
  *target composition (D×A×T×B) vs. shipped subset (≈ D only)* — B (served demand) unbuilt
  (#181), A (address diversity) at the DHT layer not yet in the standing number — so C1 is a
  *conditional* claim. **Part VIII value-loop table** row corrected from "Privacy of *access*
  is absolute" (the exact absolute D-PRIV retired) to the refusal-to-surveil-absolute /
  access-held-in-tension form. **Proof-of-repair** softened from "now exists" to *construction
  designed, not yet built (H7)*. Companion risk-register / threat-catalog / public-site fixes
  landed the same honesty (γ→1/N as an open-risk row; `g≤0`; CPR-adversarial-placement; the
  public Sybil-standing copy corrected to bond-backed). *(Update 2026-08-08: H7 is now BUILT and
  adversarially verified; the blind-commitment leg deferred as a fast-follow.)* Comprehension
  was faithful; this was a propagation fix, not a re-think.
- **2026-08-10** — Added **build-immutables #3 (one signal, one job — never fuse
  transport with security)** and **#4 (cheap honest participation is a security
  constraint)**, distilled from the external network-durability-vs-space-time
  research opinion (`silt-reviews/research/…network-durability-vs-spacetime…`,
  2026-08-10) that field-testing under adverse networks (`integration/flakynet`,
  #288/#289) provoked. #3 generalizes the two defects that motivated the consult —
  the C1 reply-latency gate (transport fused with the compute security signal) and
  the #288 evict-on-one-miss (routing reachability fused with consensus standing) —
  into one bright line: security lives in the proof or in statistics-over-many,
  never in a single network measurement; a timing signal is a soft disclosed
  deterrent, never a hard gate (the hard version is structural / owned-residual).
  #4 promotes silt's "cheap to run" mission (Part 0) into a veto over security
  designs: no fix may raise the floor of honest participation. Both enforced in
  code by the same-week decouple-anti-release-from-transport + soft-C1-gate work.
- **2026-08-11** — Added **build-immutable #5 (build for the adverse internet —
  durability is the default)** and its companion reference **`docs/network-durability.md`**,
  distilled from the same network-durability research opinion. Where #3/#4 are the
  *security* duals (never gate security on the network; never tax participation),
  #5 is the *liveness* discipline: every networking path must survive jitter,
  latency, loss, and reordering by default, using the settled best practices
  (adaptive per-peer RTO / size-scaled deadlines, retry-not-evict, minimum-filter,
  succinct-payload > FEC > QUIC) rather than a re-invented magic constant.
  Provoked by **#286** — the first full multi-region GCP field test found a fresh
  quorum-2 objective chain wedged at genesis because a flat transport deadline
  couldn't carry the one-time ~1.5 MB bond-registration block across the WAN; the
  fix (a size-aware transport deadline) is exactly the "generous, payload-scaled
  transport deadline" the research prescribes. The doc exists so future builders
  read the settled answer instead of losing days re-deriving it (as #286/#288 did).
- **2026-08-12** — Added **build-immutable #6 (root-cause before you patch —
  attribute before you ship)** and its companion reference **`docs/build-process.md`**,
  distilled from the research team's `build-process-root-cause-first-ADVICE.md`
  (2026-08-12). Where #3/#4/#5 govern *what* is sound, #6 governs the *sequencing*
  of a fix: instrument and name the failure mechanism (a one-paragraph *X because Y;
  fix Y by Z*) before touching a knob; reduce to a cheap local repro before an
  expensive run; grep for existing machinery before building new; and consult
  research when the mechanism is unknown or the change touches security / consensus /
  a claim — not after two guesses. Provoked by the repeated **#286** WAN loop: a
  handshake-EOF "fixed" with a fixed-constant timeout (real cause: no outbound
  addresses), and a billable multi-region run spent to *discover* the ~8 MB genesis
  block that was deterministic and reproducible in-process. The correct genesis fix
  (spread bond regs; commit small via the already-existing `launchAnchor` anchor
  bootstrap) was *unused-correct code* — the rule-6 poster child. Same corrective as
  the comprehension/owned-residuals audits (docs ahead of code there, fixes ahead of
  root-cause here): **verify/attribute before you ship.**
- **2026-08-14** — Added **build-immutable #7 (evidence or nothing — no forward step
  on a hypothesis; when you lack the evidence, gather it)**, ratified by the owner
  after watching the **guess → act → fail → guess** loop cost hours and billable cloud
  runs across multiple sessions. Where #6 attributes **one** failure before **one**
  patch, #7 governs **all** forward motion — a fix, a run, a claim, a "next step" —
  and forbids taking any of it on a hunch: cite the specific log/trace/test/repro that
  justifies the step, or your real next task is to go **gather** that evidence
  (instrument, reproduce, capture), never to guess-and-try. Mandates an **iterative**
  cadence (one evidence-verified step at a time; no batch of hopeful edits, no run to
  "see what happens") and the rule that a **non-locally-reproducible failure is
  instrumented to capture its cause, not re-tried on a theory** — "re-run, probably
  transient" is a guess in a lab coat unless the re-run is itself the instrumented
  observation that captures the missing evidence. Provoked by the canonical case: a
  billable P1 cloud run whose sybil cohort **crash-looped**, and the harness tore the
  VMs down *without capturing their journals* — leaving only guesses about the cause.
  The corrective was to make the harness **capture the evidence first, then look**
  (`integration/cloudtest` failed-node journal capture, PR #394), and to make the
  self-check before every action explicit: **say the evidence out loud; if you can't,
  you've found your real next task.**
- **2026-08-18** — Added **build-immutable #8 (fits the hobbyist box — ~1 vCPU / 2 GB
  is a hard, measured-before-you-commit design gate; measure the cost to PRODUCE, not
  just the artifact size)**, ratified by the owner. The resource dual of #4: where #4
  forbids *raising* the honest floor, #8 makes the floor a **concrete, testable number**
  and moves the check **before** the build — every mechanism's full cost (produce +
  verify + resident + serve) measured on the floor box first, *bounded-then-fast*.
  Provoked by the most expensive near-miss: **#299** succinct bond proofs, which read
  as ideal at ~192 bytes on the wire until research quantified the *prover* at
  one-Filecoin-PoRep-class (**128–192 GB RAM + GPU**) — two orders over the floor;
  citing the artifact size without the production cost nearly bought a week and a
  datacenter dependency. Reinforced by the MATURING OOM arc (unbounded inbound queue →
  proof map → chain retention), the same law unlearned on the *memory* axis — an
  unbounded system on a 2 GB box is not inefficient, it is unsafe. The specific box
  spec stays an *Evolving* parameter (target may tighten toward ~1 GiB); the
  fit-and-measure discipline is immutable.
- **2026-08-29 (frozen consensus format)** — Added the **frozen consensus formats**
  entry to the Immutable tier and recorded the **era-3 committed state-root format** as
  FROZEN (build `3af40bc`), ratified by the owner. A shipped, certified consensus format
  is immutable: its bytes are law, and changing it in place would re-interpret committed
  history or split the network, so amendment is a NEW ERA (a new `BlockVersion`) behind a
  height-gated hard fork, never an edit. The era-3 format commits two attester-signed roots
  (a state SMT over the 18 `committedSet` fields + the RFC-6962 revocation-log MTH); an
  un-upgraded node stalls at `H_era3` rather than accept an unvalidated root. The precise
  frozen spec lives in the `docs/decisions.md` D-TIERING freeze entry; certified by
  `.../research-outcome/era3-committed-state-root-format-BUILT-RECERTIFICATION-2026-08-29.md`.
- **2026-09-01 (restructure into pure principles)** — Restructured `TENETS.md` into a
  document of pure guiding principles (owner-ratified governing rule 2026-09-01). Removed
  all ship-status / shipped-vs-target caveats, M0's proof machinery (C1/C2 inequalities,
  q/k, the external V3 suite naming), the frozen-format specifics (era-3 SMT spec,
  BlockVersion numbers), present-tense mechanism claims (R4's criticality tiers), and all
  issue/PR war-story citations — each relocated to its design companion (see the relocation
  ledger `docs/thinking/2026-09-01-tenets-principles-purge-ledger.md`). Extracted this
  amendment log to this file. Added: a top-of-file Immutable Register (index); Lane-5
  prior-art precision (weak-subjectivity → the costless-simulation / long-range attack
  class; Arweave → the numéraire distinction; B8 → the proven-unadoptable discipline; the
  BitTorrent+BitCoin tagline → a fidelity note); and a NEW **tenet-tier** principle,
  **anti-recentralization** (Lane 6): no permanent center of *reward* — the edge tier that
  does the majority of the work must remain a net-positive place to do it (the economic
  dual of "no permanent center of control"). No immutable was traded — the mission and every
  corner are preserved, abstracted away from mechanism. This is a presentation restructure.
  **OWNER-RATIFIED 2026-09-01.**
