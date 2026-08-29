---
name: verifiable-escrow-primitive-scoping
description: Desk study — the Phase-4 D-DEMAND P2 dispute-resolution verifiable-escrow primitive is SCOPED (not OPEN); quorum-TTP + threshold decryption is the buildable route; demand-neutrality makes the gate cost zero security today.
metadata:
  type: project
---

# Verifiable-escrow primitive (D-DEMAND P2 dispute-resolution) — SCOPED (2026-08-27)

**Verdict: SCOPED with 2 residuals.** Desk study (literature survey, zero code, off critical path):
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/DESK-STUDY-verifiable-escrow-primitive-scoping-2026-08-27.md`

**The question:** PoD's P2 dispute-*resolution* half needs verifiable escrow of the content key so a
quorum-TTP can affidavit that a disputed delivery completed WITHOUT the fetcher. Canon records no
adopted pure-Go impl. SCOPED or OPEN?

**Why SCOPED (the decisive soundness fact):** the protected quantity is a NEUTRAL, never-standing
observable (`fairexchange.go:42-44`, `decisions.md:239-241`). An unresolved abort only UNDERCOUNTS a
neutral quantity → costs ZERO security today. Pagnia-Gärtner (1999) proves a TTP is unavoidable, so
the target was never TTP-free — it was "distributed dispute-only TTP + confirm absence is survivable."
Both hold: abort-safety floor built+locked; resolution-half absence survivable. Classic held-in-tension,
NOT open.

**Candidate space (verified at SOURCE, not summaries):**
- Camenisch-Shoup verifiable encryption = NOT adoptable in ANY language. `coinbase/kryptology` archived
  2022; `hyperledger-labs/agora-verifiable-encryption` (the ONLY new lead) = Rust, unaudited, "use at
  your own risk". Research-grade everywhere. Canon verdict survives fresh check.
- kyber (`go.dedis.ch/kyber/v3`): DKG (`share/dkg/pedersen`) + threshold SIGNING (`sign/tbls`) + PVSS
  mature/audited pure-Go — BUT `encrypt` = ECIES ONLY, NO generic threshold DECRYPTION. Canon claim
  confirmed EXACTLY.
- `niclabs/tcpaillier`: pure-Go threshold Paillier w/ real threshold DECRYPTION + verifiable shares
  (decryption_share.go, zk_proof.go), 44 commits, maintained — but UNAUDITED + TRUSTED-DEALER (no DKG).
  The leading pure-Go decryption slice.
- Ruled out by silt's OWN bright lines (not lib gaps): TTP-free/blockchain-adjudicated fair exchange
  (OptiSwap/FDE eprint 2024/418) needs a smart-contract chain silt lacks; time-lock/TLP family =
  wall-clock security, forbidden by build-immutable #3 (same reason latency-gating rejected, m0.md §3).
- MPC-in-the-Head verifiable encryption (2024) = research-grade, no impl.

**The route canon names (quorum-TTP + threshold decryption) = CONFIRMED buildable, no new hardness
assumption.** DKG+signing free (kyber); decryption = tcpaillier (R1); verifiable-enc = the gap or
design-out via Paillier range proofs (tss-lib precedent, primitive-availability-gaps §3).

**Residuals:**
- R1: no AUDITED pure-Go verifiable-escrow+threshold-decryption stack. Closes via (a) design-out with
  Paillier range proofs [cheapest], (b) wazero-wrap vetted impl, (c) audit tcpaillier + add DKG. Not blocking.
- R2: threshold-decryption trust surface (DKG ceremony + share custody) is a NEW capability,
  disproportionate to a neutral observable, adjacent to D-DISCLOSURE (core holds no decryption of STORED
  content — escrow is per-DELIVERY key only, must stay scoped as invariant). Owner's-call, not research.
- R3 (noted): strong-form fusion promotes R1/R2 to critical path; independently #182-gated, not reopened.

**Recommended posture:** wait-and-floor; resolver is an H7-strategy fast-follow, built only when a
non-neutral consumer makes it load-bearing. Adjacent: [[C5-operator-economics-composition]] (same PoD lane).
