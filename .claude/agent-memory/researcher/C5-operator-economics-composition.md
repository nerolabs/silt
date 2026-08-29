---
name: c5-operator-economics-composition
description: C-5 verdict — the honest floor-box operator's relay+repair credit clears cost? GATED. Firewall+conservation certified; the "repairs they perform" credit pays the HOLDER not the reconstructor; hot-only self-funding.
metadata:
  type: project
---

# C-5: honest operator economics — relay + repair credit clears cost (2026-08-27)

**Verdict: GATED.** Cert:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/C5-honest-operator-economics-composition-RESEARCH-CERTIFICATION-2026-08-27.md`

**The claim:** VISION.md:52-56,141-151 — operator "earns balance-lane credit for the bandwidth
they relay and the repairs they perform," "no defense prices out the small operator." Composed:
relay+repair credit clears the floor-box operating cost at equilibrium, no subsidy.

**Why GATED (not CERTIFIED, not REFUTED):**
- CLOSED/certified: (1) FIREWALL holds under composition — relay+repair both balance-lane, never
  standing; γ→1/N (#182) not strained (`credit.go:291-298`, `escrow.go:14-20`, invariant_a_test).
  (2) CONSERVATION holds — no leg mints a network subsidy; money-pump does not exist. (3) No
  DEFENSE prices out the small op — min-bond walled from transport/serve economy (network-durability §4).
  (4) Relay income tracks relay cost by construction (sender-funded PayWord, same-byte).
- The GAP (G1, the decisive finding): "the repairs they perform" credit **pays the NEW HOLDER, not
  the paramedic who reconstructs** — RATIFIED design, fork (b), `h7-proof-of-repair.md:414`,
  `TENETS.md:301`, `repair.go:651-670`, `file.go:279`. The reconstructor (fetch k survivors, RS-rebuild,
  640MiB-1GiB RAM spike) is paid ZERO by construction. So the VISION sentence is literally false to the
  mechanism — repair is an unpriced caretaker DUTY, the bounty is custody rent to the holder.
- Scope residuals (held-in-tension): repair self-funds HOT objects only (S/R≥24, escrow.go:62-67);
  cold majority (50-60% one-hit) is D-S7 finite-horizon PREPAY, not self-sustaining. Relay/serve
  income is DEMAND-GATED — idle/unrouted honest node earns ~0.

**Gate lifts by:**
- G1: edit VISION to say what the mechanism PAYS (holder-side custody credit) — one-clause doc fix,
  no code, RECOMMENDED. OR reopen D-S7 payee (fork a, pay reconstructor) = mechanism change, new cert.
- G2: MEASURE floor-box repair RAM at PRODUCTION chunk size (64MiB, not 64KiB sim which hides ~1000×).
  Build-immutable #8, still UNTAKEN. If OOM, reconstruction migrates to large nodes = scale asymmetry.
- G3: state cold-data durability as the finite-but-renewable contract canon already owns, not
  unqualified self-funding.

**Decisive upstream cert this composes:** `repair-bounty-coefficient-c-RESEARCH-CERTIFICATION-2026-08-19.md`
§0 (unpaid reconstructor) + §0.1 (RAM spike) — those two flags ARE the C-5 gate. Relay leg:
`PoD-relay-compensation-followon-...-2026-08-27.md`. No immutable traded; firewall+conservation intact.
Adjacent to [[C1-maturity-before-capture]] (both #183 red-team surfaces).
