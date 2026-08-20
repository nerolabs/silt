# The economy flow's premise needs a killable pool it cannot exhaust (+ the island's missing cloud output)

Date: 2026-08-20. Context: the first full LOCAL sheet at merged main 577f0f1
(run 577f0f1-45838, seed 577f0f1-46567) went 20 PASS / 0 FAIL / 3 expected SKIP
— and GAPed both economy rows. Diagnosed live on the standing fleet; both GAPs
share one root, and the diagnosis surfaced two further latent defects. All
harness-level; no product defect found.

## The mechanism (evidence-backed)

**11-economy-repair GAP.** The econ object erasure-codes into 16 columns.
Observed holders (full `swarm holders` map, nodeids resolved against
nodes.local.json): 4 validators + store-1 + store-2 + relay. fetch-1 (the
publisher) holds zero — self-hold ineligibility, observed. The flow reserves
store-2 (caretaker), relay (judge), store-1 (skim observer); killable roles are
`storage|fetcher|maturer|sybil` minus those reservations. At SYBILS=0 that
leaves fetch-1 alone, which holds nothing. **"0 of 3 columns all-killable" is
deterministic, every seed.** The #494 `-replication 1` fix removed the previous
layer (column concentration); this is the next layer down: role exhaustion.

**11b-economy-skim GAP.** store-1 armed correctly (journal: `caretaking
dd7192…`) but `ChunksServed: 0` since its relaunch — the three driven fetches
sourced k=10 columns from other holders and never asked store-1 for its single
column. Contributing, measured in the same run: the flow warms the observer
with `sleep 15`; chaos-reprovide measured 37 s re-announce after restart. The
wire skim path (`RecordServeToObject`, node.go) was never entered; its only
proof remains the in-process sim test.

**Latent defect A (found by inspection during diagnosis): the unfunded third
caretaker.** The flow arms the skim observer with `-care -economy` — a THIRD
caretaker on the care-link — but funds and polls only care+judge. The recorded
wire mechanic (e2e proof, 2026-08-20) is "fund ALL caretakers, poll all":
which node judges is timing, and PayBounty draws from the judge's OWN escrow.
If the skim observer wins the judge seat, the bounty draws from an unfunded
ledger → paid≈0 → a false GAP/FAIL after everything else worked. Would have
been reachable on the cloud run.

**Latent defect B (found by inspection): island invisible to the cloud
orchestrator.** `terraform/outputs.tf` merges public+natted+natgw into the
`nodes` output; island instances are absent. Cloud nodes.json is `tf output
-json nodes` (cloudtest.sh:190) → island VMs would be provisioned and billed
but unreachable (`ssh_node` empty) → flow_equivocation_island GAPs on every
cloud sheet. LOCAL masked it: the docker backend writes every container into
nodes.local.json. This is exactly the LOCAL/cloud parity seam #493/#494 exist
to close, one file further out.

## Options weighed (repair-leg premise)

1. **Re-use the adversary node as caretaker** — zero topology change, frees
   store-2's columns as killable. Rejected: the adversary daemon carries
   forge/low-bond drill behaviors; in randomized order the caretaker arm can
   interleave with its drills. Cross-contaminates two flows' premises.
2. **Dedicated killable stores (CHOSEN — owner call, 2026-08-20):** add
   store-3/store-4, ECONOMY=1-gated (the SYBILS opt-in pattern), primary zone,
   **no external IP** (island pattern: Cloud NAT egress, IAP reach) because at
   ECONOMY=1 SYBILS=8 every region's default 8-IP IN_USE_ADDRESSES quota is
   already saturated. Two stores, not one: 16 columns over ~9 eligible holders
   ≈ 1.8 columns/holder; one added store expects ~2 killable columns against a
   need of 3 (premise-fragile again); two expect ~3.6 and the flow's honest GAP
   covers the tail. LOCAL cost: two containers.
3. **Shrink chunk size / grow the object for more columns** — rejected: RS
   column count is the code's n=16, not a function of object size; §0.1 sized
   256 KiB deliberately.

## The skim fix (superseded twice — final design in the addendum)

First sketch: announce-verified wait replacing the blind `sleep 15` (15 s <
the measured 37 s re-announce). Superseded by reading the code: the daemon
wires the credit ledger unconditionally, so the observer relaunch itself was
the defect — it races re-announce AND the lazy proofMeta reload, and arming
`-care` created latent defect A. Second design: no relaunch — observe skim on
the CARE node as a delta above its confirmed prepay baseline. The 11364
re-drive then exposed the residual mechanism (a healthy fetch reads DATA
columns; a parity-holding care node is never served), which the addendum's
two-window design closes.

## Ships with

topology.py (ECONOMY-gated store-3/4, internal_only plumb), terraform
(variables optional bool, public-filter, noip storage resource, NAT condition,
**island added to outputs**), scenarios.sh (require store-3/4 with a premise
GAP message, the delta-baseline two-window skim assert, adversary killable,
link/publish/holders verbosity). Verification: LOCAL re-provision + economy
re-drive, then a full clean LOCAL sheet on the final harness.

## Addendum — the re-drive (run 577f0f1-11364) and the placement-fidelity finding

The re-drive with store-3/4 GAPed both legs again, differently: repair found
1 of 3 killable columns (up from 0), skim stayed flat at the prepay baseline.
Ground truth recovered from per-node object mtimes (journals do not log chunk
receipt at info): the publish placed 28 shards as val-a 7, adversary 7, val-d 6,
val-b 4, store-1 2, store-2 2 — **zero on store-3/store-4/val-c**, though the
same stores had accepted warmup placements a minute earlier.

**Open product question (recorded, not chased):** computing the design's own
argmin-XOR(nodeid, colKey) winner per column from the real root and nodeids
predicts store-3 (col 11), val-c (cols 1,2), fetch-1→store-1 (cols 3,5) —
observed placement contradicts it (adversary took ~3.5 columns against a
predicted 1; predicted winners got zero). A fresh `swarm add` client's
iterative lookup is not converging to the true closest set, or candidate
order/acceptance diverges from XOR order. Needs product-side instrumentation
(a placement trace at debug level) before any conclusion — the harness now
echoes link + publish output + full holders map so the next run carries the
evidence. Skew direction favors long-known nodes; if real, it also shapes the
cloud sheet (sybils are late joiners).

**Harness responses shipped in this addendum's commit:**
- The adversary role joins the killable set (non-anchor, stateless drills,
  restarted by step 7; it held 3-4 columns this run while the fresh stores got
  zero — omitted from the set by history, not constraint).
- The skim verdict gets a second observation window: reconstruction reads k
  surviving columns per stripe, so the repair leg itself is the traffic that
  serves the care node's columns even when a healthy fetch never picks them
  (the run's mechanism: a fetch reads DATA columns; a parity-holding observer
  is never served pre-kill). Baseline before kills; verdict after the repair
  wait, with a fresh final read.
- Verbosity: the flow now echoes the link, the publish client's full output,
  and the full holders map to the console (the old flow discarded all three;
  diagnosing required perturbing dedup re-adds that pollute the map they probe).

## Addendum 2 (2026-08-21, replay sheet 1642465-55153) — the premise's fifth layer is product-level

The replay sheet (same seed as the failed one, fixed shim) went 20 pass /
1 gap / 1 fail: all four harness fixes validated at their exact failure
points (forged-block, partition, restart-content green; economy skim green
again at +98310). The two residuals are product-level, both evidence-complete:

**184-equivocation-island FAIL — a height-1 equivocation went undetected.**
island-b armed and logged `equivocation complete (double-signed height 1)`;
honest anchors reconciled repeatedly and never slashed. Every prior passing
run double-signed at height 2 and was caught. Either the detection path has
a floor at the first post-genesis height (a real accountability seam — an
adversary would aim exactly there) or the conflicting height-1 fork never
reaches an honest reconcile. Consensus-adjacent (I5 accountability face) →
research-gated; the drill's boot-timing premise variance (which height the
baked-in equivocator hits) is what exposed it.

**11-economy-repair GAP — records-vs-bytes divergence defeats the
under-replication premise.** With order A (cold object) and 2s sweeps
running perfectly (28 `repair sweep complete` lines), the sweep found
29/29 reachable with both killed holders PROVEN dead (boot-log gaps
22:41→22:45). Mechanism, disk-verified: a `-replication 1` publish put ~87
files (~3× the 29 shards) across the fleet in the publish second — the
client printed "scattered 30 chunk replicas", so the extra copies come from
beyond the client's own placement — while the provider-record view
(`swarm holders`) shows ONE holder per column. The kill-selection trusts
records, so it kills "sole" holders whose bytes exist elsewhere; and the
sweeping caretaker itself held extra copies of the dead columns, which
`probeShard(includeLocal=true)` legitimately counts. Drive 4 paid only
because the extra copies happened not to land on its caretaker. Open
product questions: (a) why does a replication-1 publish triple on disk, and
why don't the extra copies announce (records-vs-bytes divergence — S5
adjacent); (b) is a caretaker-held-but-unannounced copy "reachable" in the
durability sense (fetchers can't find it)? The wire flow's premise cannot
be made sound until (a) is answered — which also gates the ECONOMY=1 cloud
run's premise.
