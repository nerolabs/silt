# State of HEAD — orientation for the fresh-eyes audit (2026-08-19)

**Purpose.** A factual map of where silt is, written to orient a fresh-eyes audit
(non-coding: "where are we / what are the priorities"). It inventories the terrain —
what is shipped, what is designed-not-built, what validations are pending — and
deliberately does **NOT** rank priorities or prescribe next steps: that ranking is the
audit's job, and pre-framing it would defeat the point of fresh eyes. Trust the source
docs over this summary; every claim below points at one.

Builder, PE, and research are standing down for this look.

---

## What silt is (30 seconds)

Hold the **privacy × accountability × Sybil trilemma** without trading a corner
(`docs/TENETS.md` Part 0 = M0, the mission-immutable). Two planes: a storage plane
(content-addressed, erasure-coded, DHT-served chunks) and a trust plane (consensus-secured
registry + work-backed, unlinkable reputation). M0 is *falsifiable*: held iff an EXTERNAL
red-team suite denies all three failure modes (publish→identity link, identity/global
takedown, Sybil standing at a discount = the C1/C2 composition claim).

**Read first:** `docs/TENETS.md` (Part 0 + Part IX immutables), then
`docs/design/consensus-invariants.md` (I1–I5), `docs/design/m0.md`, `docs/network-durability.md`,
`docs/build-process.md`.

---

## What is SHIPPED on main (working, tested)

- **Storage plane:** content-addressed chunks, erasure coding, DHT provider records,
  repair loop, NAT hole-punch + relay fallback (integration/nat, cross-NAT in CI).
- **Consensus (trust plane):** the BFT cluster is literature-faithful and the I1–I5
  invariant set is closed + asserted by a deterministic model-check
  (`docs/design/consensus-model-check.md`). The #357→#402→#432→#441→#451 liveness/safety
  cascade is landed + research-certified. "Consensus is boring by policy" — novelty spent on M0.
- **M0 mechanisms (shipped SUBSET, be honest which):** identity-keyed **bond** (disk axis,
  space-time PoR), objective bond-weighted fork-choice, retention decay/TTL, blind-signed
  publish tokens (F1 unlinkability), equivocation slashing, the C2 concentration metric.
  **The composition is D-axis (disk) only today:** served-demand (B, #181) unbuilt, address
  diversity (A) at the DHT layer not yet in the standing number, time (T) is retention-only
  (no acquisition ramp). So the C1 "no discount" claim is *conditional* — see `m0.md` §3/§7/§10.
- **Memory-safety / boundedness:** the MATURING consensus OOM is fixed and **field-confirmed
  on e2-small (2 GB)** — inbound-queue backpressure (#465), PoR-proof RAM O(hot) (#464), and
  the **H2 rolling-retention-horizon + suffix-sync** arc (#470, this week): pruning the
  ~1.5 MB bond proof to a recent finalized window, with a safe behind-peer catch-up.
- **P2P robustness (tonight, PR #471):** a DHT provider-resolution/repair **recursion stack
  overflow** fixed (trampoline), **periodic reprovide (#69)** so holders don't go dark after
  the record TTL, apiFetch serves held content + consumer==provider seeding, public nodes
  drop loopback gossip, cold-start binds relay/registry before the O(store) proof scan, and
  an opt-in `-allow-web-origin` browser transport. Surfaced by a real streaming load test.
- **Field harness:** `integration/cloudtest` — a one-command multi-region GCP grade with a
  flow sheet + `infra-node-liveness` crash gate (now discriminates real OOM/Go-fatal from a
  deliberate chaos SIGKILL). The RC gate (R1).

## What is DESIGNED but NOT built (named, deliberate)

- **The core M0 research problem — shared-content sealing (γ→1/N, #182):** fusing *served*
  content into standing (the S7 "one ledger" goal) is blocked until identity-keyed PoRep
  sealing exists; today standing comes from a *dedicated* bond plot, separate from served
  content. This is where the novel contribution still concentrates (`m0.md` §10).
- **Served-demand axis (B, #181)** and the **demand-attestation** wash-pricing.
- **Succinct bond proofs (#299):** parked — full version is a sealing re-architecture,
  B8-gated on an external memory-hardness proof. Near-term tiers (Merkle multiproof
  compression, batch-verify) are on-tenet but not built.
- **Durability economics (S7):** the per-repair game (proof-of-correct-repair, H7) is BUILT;
  perpetual solvency (`g > 0`) is instrumented-not-guaranteed. The credit escrow/auto-skim
  funding model is decided (D-S7) but the economy is not wired end-to-end.
- **Takedown transparency log** (CT-style non-globality, immutable #5) — construction designed.

## Pending VALIDATIONS (the open gates)

- **★ The external M0 red-team (#183) — the load-bearing one.** M0 is held iff an *outside*
  party's V3 suite denies the three failure modes. This has NOT run against current HEAD.
  PE ruled HEAD "not red-team-ready" until the 184 adversarial drills are all DRIVABLE on
  the wire (some are; equivocation/partition landed). This gate is what turns "we think we
  hold M0" into "an outsider couldn't break it."
- **A clean multi-region field grade at the hobbyist box.** The e2-small return-to-2GB is
  confirmed for the consensus cohort; a fully-green sheet (no residual chaos/timing FAILs)
  on a pruned MATURING net at depth >2·BondTTL is the next confirming run.
- **The prune at depth:** slices proven-correct locally + wire-exercised, but field runs
  reached only ~h63 (< the 64-height engage threshold), so the retention prune itself is not
  yet field-exercised at depth.

## Real-world findings from running silt for real (flixz streaming load test)

Offered as INPUT, not prescriptions (`private/` handoff; keep flixz strategy off public repos).
The **code** fixes landed publicly tonight (PR #471, generic-framed). The **design** gaps:
- **Bandwidth is unpriced** — silt prices storage (PoR) but not bandwidth, so a gateway
  becomes a centralized egress choke (IPFS free-rider recentralization risk). Proposed
  primitive: **Proof-of-Delivery**. M0/S7-adjacent; arguably the biggest strategic gap.
- **NAT traversal as software** — a NATed holder can retrieve but can't be reached to *serve*
  without traversal (registry-coordinated hole-punch + relay is the natural home).
- **Desktop packaging / background service / code-signing / OTA auto-update** — "run the
  client" must mean a signed, always-on consumer install, not `nohup ./silt`. Adoption-critical.

## Open residuals / follow-ups (small, tracked)

- Cold-start: the *deeper* fix (incremental/checkpointed proof maturation so the node's own
  DHT serves during the scan; O(delta) not O(store)) — tonight only recovered relay/registry.
- Reprovide cost at scale (O(held) re-sign each interval — wants dirty-tracking/batching).
- Pre-existing field residuals: chaos-reprovide/#69 hard-crash timing, 10a-stall B2 timing.
- Pre-#183 DoS gates: v2b inbound consensus-priority reserve, MsgSubmitBondReg CPU gate.
- ~110 stale local git branches (history clutter; not pruned — some may be unmerged).

## Suggested reading order for the audit

1. `docs/TENETS.md` (Part 0 + Part IX) — the mission + the immutables that gate every call.
2. `docs/design/m0.md` (esp. §3, §7, §10) — the shipped-subset-vs-target honesty; #182.
3. `docs/design/consensus-invariants.md` — the closed I1–I5 set.
4. `CHANGELOG.md` `[Unreleased]` — what shipped recently, in the builder's words.
5. The memory index `.../memory/MEMORY.md` — the builder's working ledger (dense, historical;
   treat as background, verify against source).
6. `silt-reviews/principle-engineer/state-of-head-PE-2026-08-17.md` — the last PE state read.

## What this doc does NOT do

It does not prioritize, and it does not claim M0 is held (that is #183's verdict, not the
builder's). Where it says "shipped," it means code + tests on main; where it says "designed,"
it means a spec exists and the code does not. The audit's job is to weigh these and set the
priority stack with fresh eyes.
