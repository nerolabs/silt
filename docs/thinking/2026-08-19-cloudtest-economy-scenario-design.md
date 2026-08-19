# Step 3 design — the cloudtest economy-repair scenario (Phase 2 exit gate on the wire)

**Date:** 2026-08-19 · **Plan:** step 3 of the harness improvement plan
(`2026-08-19-cloudtest-harness-improvement-plan.md`). Pace-before-code for the largest remaining
build. **Purpose:** exercise the S7 repair economy end-to-end on the cloud — a caretaker
reconstructs a lost stripe, the bounty pays the new holder, and the durability instruments move —
so the Phase 2 exit gate (**`g`/economy on the wire**) finally has a home. Collapses coverage gaps
C1 (economy untestable), C2 (repair loop never reconstructs), F1/F2 (no caretaker, sim chunks) into
ONE scenario.

## What the current harness lacks (verified)
- **No caretaker reconstructs.** `flow_durability_turnover` only fetches from a *surviving replica*
  (`store-2`) after killing `store-1` — no shard is ever rebuilt from parity. The repair loop, the
  whole reason S2 holds, is unexercised on the cloud.
- **`-economy` unwired** in `topology.py`; no fund→bounty→`g` flow.
- **64 KiB sim chunks** everywhere.

## Building blocks that already exist (so this is assembly, not new machinery)
- `relaunch_with NAME "-flags"` (`lib.sh`) — append `-care <link> -economy -ui 127.0.0.1:8081` to a
  node's ExecStart and restart; `restore_argv NAME` resets. The web-ui-guard flow already uses this
  pattern + `curl http://127.0.0.1:8081/api/status`, so hitting the API from a node is proven.
- `/api/fund` (Slice 3) — prepay the object's escrow from the node's own grant balance
  (`credit.New(50_000,…)` gives every daemon a starter balance).
- `/api/status` `durability` block (Slice 2) — `bountyOn`, per-object `reserve/funded/paid/repairs/
  horizonSec`. The assertion surface.
- The Phase 1.3 RSS sampler — captures the caretaker's reconstruction RAM automatically.

## The scenario — `flow_economy_repair`
1. **Publish an erasure-coded object** from `fetch-1` (`silt swarm add`, a chunk size that reliably
   stripes — see "chunk size" below), capture the `silt:` link, its `siltcare:` link, and the sha.
2. **Make a node a caretaker with the economy on:** pick a caretaker node (a storage node NOT holding
   the object's only copies — e.g. a fresh `care-1`, or reuse `store-2`), then
   `relaunch_with care "-care <careLink> -economy -ui 127.0.0.1:8081"`. It now runs repair sweeps for
   the root and will PAY bounties.
3. **Fund the reserve:** `curl -X POST http://127.0.0.1:8081/api/fund -d 'root=<link>&amount=…'` on the
   caretaker (token from `<store>/ui-token`). Explicit fund makes the bounty deterministic (vs relying
   on serve-skim, which needs fetch traffic first). *(A second variant later can prove skim self-funds;
   v1 funds explicitly to grade the payout path.)*
4. **Force reconstruction:** kill enough column-holders that a stripe loses `> RepairSlack` (=2) shards
   — `svc <holder> stop` on 3 holders of one column set (mirror `KillColumns`), so the caretaker's next
   sweep must REBUILD (not just re-fetch a survivor). This is the step that both (a) exercises the
   repair loop and (b) triggers the §0.1 RAM spike the RSS sampler records.
5. **Wait for the economy to close** (bounded, ~RepairInterval × a few): poll the caretaker's
   `/api/status` durability block until `repairs > 0` and `paid > 0` for the root, or the window
   expires.
6. **Assert (SLO):** the reserve **drew down** (`paid > 0`, `repairs ≥ 1`) — a verified repair was
   funded on a real network — AND `infra-node-liveness` still PASS (no OOM from the reconstruction).
   GAP honestly if the publish/fund setup didn't land (untested ≠ failed), the same discipline the
   other flows use.
7. **`restore_argv care`** on the way out (leave the topology as found for later flows).

## The `g` question (exit gate) — staged
A single repair gives `CostPerRepair` + a finite `Horizon` (Slice 2 already surfaces these), but `g`
(the cost *trend*) needs **two snapshots over time across ≥2 repairs**. v1 asserts the loop CLOSES
(bounty pays a verified repair on the wire — the keystone confirmation). A v2 can drive a second
kill→repair cycle and read `credit.G` from two `/api/status` snapshots to put `g` on the wire. Staging
this keeps v1 shippable and the exit gate explicit about what's measured.

## Chunk size & box size (the §0.1 constraint, now MEASURED)
The local §0.1 benchmark proved a 64 MiB-chunk stripe reconstructs **1.0 GiB resident** — it will
**OOM a 2 GB box** alongside the daemon baseline (0.5–1.25 GiB). So this scenario must NOT publish at
full production chunk size on the default `e2-small` (2 GB), or the caretaker OOMs mid-reconstruction
(the run would then measure the OOM, not the economy). Two honest options:
- **(A) Moderate chunk (1–4 MiB) on the default box.** Stripes reliably, reconstruction ≈ 10–40 MiB
  (fits e2-small), the economy loop grades cleanly. §0.1 (full prod) is already measured *locally*, so
  the cloud run need not re-measure it — it grades the *economy*, which is chunk-size-independent.
  **Recommended for v1.**
- **(B) Production chunk (64 MiB) on a bigger box** (`MACHINE_TYPE=e2-medium`/`standard-2`, 4–8 GB).
  Field-confirms §0.1 at scale + grades the economy, at higher cost. A worthwhile *later* run once v1
  proves the loop; not needed to close the exit gate.

**v1 = option (A):** moderate chunk, default box, grade the economy loop. Cheapest run that closes the
gate, and §0.1 is already answered locally.

## Topology change
Add `-economy` to the caretaker's baked argv is NOT needed (we `relaunch_with -economy` at runtime), so
**topology.py may not need to change at all** for v1 — the caretaker is any existing storage node,
reconfigured at runtime. Minimal blast radius. (If a dedicated `care-1` node is cleaner, add one
storage-role node; optional.)

## Cost/verification
- **Locally validate** the flow's shell (`bash -n`) and the `/api/fund` + `/api/status` curl shapes
  against a local daemon before spending a run.
- The run rides the existing full topology (no new consensus regime → D-CONSENSUS already green).
- Gated behind an opt-in (`ECONOMY=1`) like the other heavy flows, so it doesn't lengthen the base run.

## Deliverable sequence
1. `flow_economy_repair` in `scenarios.sh` (option A), opt-in `ECONOMY=1`, wired into
   `run_all_scenarios`.
2. Local shell validation + a local single-daemon smoke of the `/api/fund`+`/api/status` path.
3. The billable **Run B** (`ECONOMY=1`) — the Phase 2 economy-on-the-wire confirmation.
4. (Later) v2 for `g` (two repair cycles), and option (B) for the §0.1 field-confirm at prod chunk.
