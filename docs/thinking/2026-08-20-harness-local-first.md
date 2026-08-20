# 2026-08-20 — Local-first harness: LOCAL=1 mode, sever race, chaos premise, re-drive

**Owner directive (Andrew, 2026-08-20):** implement the local-first package from the
harness parity analysis, in one session. This authorizes the multi-item scope; each
item below still gets its own evidence line (#7).

**The diagnosis being acted on (from the parity report):** 20 archived cloud runs,
zero fully green; the recurring red concentrates in harness-quality rows (partition
sever 18 GAPs, chaos FAIL-vs-GAP 15 FAILs, setup-publish GAP cascades) — and the
harness itself is the one major component that never executes locally.

## Item 1 — LOCAL=1: the cloud harness runs against local docker nodes

**Decision: docker containers, not bare processes.** The flows' commands embed
absolute paths (`/usr/local/bin/silt`, `/var/lib/silt`, `/etc/systemd/system/
silt.service`) and root-isms (`sudo`, `systemctl`, `journalctl`, `pkill`). Options:
(a) rewrite every flow to be path-abstract — touches 1,600 lines of graded drive
logic, exactly what we must NOT churn; (b) processes + PATH shims — cannot shim
absolute paths; (c) containers with tiny shims at the real paths — zero flow
changes, real per-node 127.0.0.1 (the UI flows curl `127.0.0.1:8098` *on the
node*), and the topology's static IPs (10.20.0.x) reproduce on a docker bridge.
(c) wins. Precedent: the NAT/adversarial suites are already docker-first.

**The entire remote surface is three seams** (verified by reading lib.sh whole):
`ssh_node` (every remote op funnels through it — jlog/dlog/svc/relaunch/capture/
mem-sampler), the `NODES_JSON` metadata readers, and one GCP metadata curl in
`restore_argv`. LOCAL backend = `ssh_node → docker exec`, a locally-written
nodes.json, and `restore_argv` reading a baked-argv file. `scenarios.sh` is not
touched by the backend at all — which is the point: the SAME graded bash executes.

**In-container shims** (`integration/cloudtest/local/`): `systemctl`
(start/stop/restart/is-active/status/daemon-reload/show -p MemoryCurrent) managing
the daemon from the real unit file via pidfile; `journalctl` (-u/-n/--since @epoch/
-b) over an epoch-prefixed `/var/log/silt.log` with boot markers; `sudo` passthrough
(already root). `relaunch_with`'s sed of the unit file works unchanged.

**v1 scope cuts, stated:** natgw/nat-1/nat-2 excluded (9-cross-nat SKIPs cleanly;
real NAT is `integration/nat`'s job and stays a cloud residue); no TTL self-destruct
(containers die with `down`); the preflight billable gate is bypassed with a logged
notice — it exists to guard money, and LOCAL spends none.

**What LOCAL=1 is for:** executing the drive logic (severs, kill selection,
relaunch/restore, verdict grading) before it ever runs against a billable VM — the
class that produced this week's three scenario defects and most of the 20-run red.
It does NOT replace the cloud run's WAN residue (latency asymmetry, real NAT, scale,
clocks, R1).

## Item 2 — The partition sever's 18 GAPs: a baseline-before-sever race, not sever width

**Attribution (mechanism paragraph, #6).** The sever was already widened to ALL
validator-role peers (validator/adversary/maturer/sybil) after runs 1ebd487-* — the
in-code comment documents that fix. Yet 2323b09 still GAPed "val-c ADVANCED during
the partition (h27→h29)". Mechanism: `adv_partition` reads val-c's baseline height
BEFORE `relaunch_with` applies the blocklist; the relaunch takes seconds (sed +
daemon-reload + restart + chain reload), and on a chain committing drain blocks
near-continuously (Run B: every height), val-c legitimately commits 1–2 more blocks
in that window. The "advance during partition" is an advance during the UNSEVERED
seconds. **Fix: confirm the sever is live (wait for the post-restart `⚠ PARTITION`
banner), read the baseline AFTER it, then drive.** The stall assertion then measures
what it claims to.

## Item 3 — chaos-fetch / durability fetch: classify the unmet premise (roadmap 2a)

Run B's two FAILs were `chaos-fetch` "root not in registry" — the publish premise
(#441-family accepted-not-committed) broke upstream, so crash-recovery was UNTESTED,
not failed. Fix: on fetch failure in flow_chaos_crash and flow_durability_turnover,
classify the client error — `root not in registry` / no manifest ⇒ GAP ("publish
premise broke, property untested"), anything else (hash mismatch, partial bytes,
timeout with the entry present) stays FAIL. Mirrors require_link's existing
philosophy one layer deeper.

## Item 4 — Per-flow LOCAL_PROOF annotations, linted

Every `flow_*`/`adv_*` in scenarios.sh carries a machine-readable line:
`# LOCAL_PROOF: <command>` or `# LOCAL_PROOF: n/a — <WAN-only reason>`.
`check_local_proofs.sh` fails if any flow lacks one (wired into CI beside the
multibyte lint). This turns the parity table from a report into a standing
invariant, and the `n/a` set IS the owned cloud-only residue, reviewable in one
grep. Extends #490's per-run principle to per-flow.

## Item 5 — Re-drive loop: TEARDOWN=0 / reuse / FLOWS= / dual commit stamp

- `TEARDOWN=0` (alias of KEEP_UP for the `all` path) leaves the fleet standing.
- `run` against a standing fleet already works; add `FLOWS="11-economy-repair …"`
  to run a named subset (run_all_scenarios dispatches through a filter). Flows with
  one-way state (the maturing latch) are excluded from re-drive by a marker.
- Reports stamp BOTH commits: product (`RUN_ID` today) + harness
  (`git log -1 --format=%h -- integration/cloudtest`), so a harness-only re-drive
  against the same binaries is attributable as exactly that.
- The bright line stays: a re-driven flow supersedes a GAP for convergence; the
  exit-gate/RC artifact is one clean uninterrupted sheet on one commit pair.

## Item 6 — Nightly netem CI

Scheduled workflow running `integration/adversarial/run.sh` (+ flakynet control)
— the adverse-network tier currently has no standing gate, and the cloud's clean
GCP fabric never exercises it. Nightly, not per-merge: minutes-long and
loss-injection makes it too jittery for a merge gate.

## Item 7 — Parity-gap e2e tests

- **Anchor-stop drill (local 5-sybil-no-capture analogue):** objective net, 2
  anchors + bonded non-anchor validators; baseline publish commits; STOP both
  anchors → publish must refuse / heads must not advance; restart anchors → publish
  commits again. Real daemons, same pattern as the economy e2e.
- **Maturing latch (local 10-handoff analogue), timeboxed:** 1 anchor + 3 maturers,
  `-mature-validators 2`; latch trips ("wheels shed permanently"); post-shed a
  publish commits with the anchor STOPPED (3/4 weight > ⅔); daemon restart keeps the
  latch (F-1). If the wall-clock or flake budget blows, ship the anchor-stop drill
  alone and record the latch e2e as the named residual.

## Outcomes (same day)

- **LOCAL=1 shipped and self-certified**: first full local SMOKE sheet graded
  **10 pass / 1 gap / 0 fail (REVIEW) in ~8 min, $0** — and its own first RED was
  a perfect specimen of the class this mode hunts: a bind-mounted shim edited on
  the host mid-run tore the container's view of the file ("syntax error line 59"
  while the daemons were healthy). Fix: COPY binary+shims at provision, never
  bind-mount. The remaining GAP (8-takedown "served=0") is the SMOKE topology
  lacking store-2, not a LOCAL defect.
- **Anchor-stop drill green in 60 s** (`e2e/anchorstop_test.go`,
  TestAnchorStopHaltsBondedNonAnchors): its own first RED re-derived #402's
  arithmetic the hard way — A=2 anchors leaves one counting non-proposer attester,
  so `-quorum 2` makes the BASELINE uncommittable; 3 anchors + 2 bonded
  non-anchors is the minimal shape. Baseline commits → all anchors killed → zero
  fresh commits on the bonded survivors → restart → resumes.
- **Full base-topology LOCAL sheet (ECONOMY=1, no sybils): 19 pass / 1 gap /
  0 fail.** `184-partition` PASSED with the baseline-after-sever fix — the flow
  that GAPed 18 consecutive cloud runs. The 1 gap was the LOCAL mode earning its
  keep: the economy flow's selection stage measured **0 of 16 columns
  all-killable at the publish's default replication 3** — the 4th latent defect
  in that flow, invisible to every cloud run (they all died earlier in the
  chain). Fixed: the economy publish is `-replication 1` (parity is the
  redundancy; same shape as the e2e proof).
- **SYBILS=8 LOCALLY oversubscribes a laptop — a documented limit, not a
  target.** The 19-container sheet degraded exactly as CPU contention predicts
  (publisher warm missed 240 s → the dependent flows GAPed HONESTLY through the
  new premise classifier, which is itself the fix working; chaos-reprovide
  missed its 300 s envelope — journals captured, NOT assumed benign, rule 7),
  and the sheet overran a 58-minute outer timeout mid-sybil-drill. The base
  13-container topology is LOCAL's sweet spot; the sybil cohort's computed WAN
  bounds don't fit a 5-way-oversubscribed docker VM. The 5-sybil drill's local
  coverage is the deterministic e2e twin (TestAnchorStopHaltsBondedNonAnchors),
  not a 19-container laptop sheet.
- **Targeted `FLOWS="economy"` re-drive (base topology): the previously-fatal
  stages are live-verified.** The filter ran exactly one flow; the economy drive
  cleared publish → caretaker relaunch WITH `-registry` (defect 3) → 400k fund
  on BOTH caretakers (defects 1–2) and reached selection, finding **2 of 3**
  all-killable columns — up from 0/16 pre-fix, and exactly the pool math: the
  base topology has 2 killable nodes, 16 columns × 2/11 ≈ 3. The cloud
  confirming run's SYBILS=8 pool (10 killable) predicts ≈7.6 qualifying columns.
  The full payout drive stays locally covered by `TestRepairBountyPaysOnTheWire`
  (a 12-node killable pool by construction) — that division of labor is the
  design, not a shortfall.
- **The skim leg closed on the wire (owner push: "economic testing covers more
  than 1 flow").** Andrew's challenge surfaced a real hole: the sheet's economy
  grade covered prepay→bounty but S7's sentence is prepay→SKIM→bounty, and the
  skim had no wire grade anywhere. Shipped `11b-economy-skim` (a zero-prepay
  shard-holder caretaker; replication 1 makes fetches route through it;
  `funded>0` = pure skim) — **first wire PASS: funded=98310 on store-1** — and
  `11c-economy-horizon` (the per-run `g` sample, observational). Pool
  interaction noted: the skim observer leaves the killable pool (base topology
  dropped 2→1 qualifying columns; the SYBILS=8 cloud pool drops 10→9, ~7
  expected qualifying columns — comfortable).
- **Maturing-latch e2e: parked as the named residual** (per the timebox above).
  The honest reason: a post-shed weight-quorum drill needs the maturers to hold
  >⅔ of frozen epoch WEIGHT (bond-size asymmetry or a 7-maturer fleet), a regime
  design worth doing deliberately, not at the end of a long session. In-process
  coverage stands (sim latch + mature model-check fixtures); the LOCAL_PROOF
  annotation on flow_maturing_handoff records it.

## The economy #441 publish latency (attributed 2026-08-20 on run 9b5d3f4-30907)

**Mechanism (build-immutable #6):** the economy setup publish POST failed with
`context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
**because** `adapters/httpregistry` `Client` hard-codes `http.Client{Timeout:
10*time.Second}`, and a chain-backed publish holds the connection open until the
consensus commit completes — which under SYBILS=8 launch load exceeds 10s. The
caller passes `context.Background()` (no deadline), so the fixed 10s is the ONLY
cap, and it **shadows the caller's context** — a build-immutable #5 magic-constant
violation (a fixed transport deadline on a load-varying operation).

Compounding it, a **harness bug**: the economy flow regenerated random content
*inside* the retry loop, minting a new root every attempt, so a slow-but-eventual
server-side commit could never be picked up by a retry.

**Two fixes:**
1. **Harness (SHIPPED):** generate the payload ONCE before the retry loop
   (mirrors ft_publish), so a same-root retry picks up an entry that committed
   server-side after the client's 10s gave up. **Honesty caveat:** LOCAL cannot
   reproduce the 10s-under-load timeout (fast local commits), so this fix's effect
   on the cloud latency is REASONED (same-root retry → server-side-committed entry),
   not locally proven. The next cloud run is its test.
2. **Product (PROPOSED, owner/research call — NOT shipped here):** the fixed 10s
   client timeout on the publish path is the root. Options: (a) a generous,
   caller-honored publish deadline separate from the 10s lookup timeout; (b) the
   async-publish (202 + poll) path the network-durability doc already describes as
   the #441 answer — which is the more architecturally faithful fix but a bigger
   change. Deliberately not decided mid-session on consensus-adjacent code.

## Randomization's first finding: val-b order-sensitive post-restart stall (2026-08-20)

Seeded randomization immediately earned its keep. Two seeds, same 18 flows:
- `SEED=coupling-test` → **20 pass / 0 fail** (and it placed `takedown`/`restart-content`
  BEFORE `publish-fetch`, proving the self-containment fix).
- `SEED=clean-integration` → val-b **falls behind and never recovers** (h5 vs h11):
  `7-restart-standing` FAIL, `5-convergence` FAIL, `2-publish-fetch` FAIL.

**Attribution (evidence, not guess):** it reproduces at 10 containers WITHOUT the
island (so it is NOT the island's 4 extra nodes), and it is **order-sensitive** —
the IDENTICAL val-b restart recovers under `coupling-test` and stalls under
`clean-integration`. A deterministic product bug would fail in BOTH orders; an
identical operation passing or failing by surrounding load is the signature of
**resource/timing contention** — val-b's post-restart re-bond racing the 60s SLO,
won or lost by how CPU-heavy the concurrently-graded flows happen to be. Best read:
**LOCAL laptop starvation, not a product bug** — but NOT certified benign; real
per-node CPU (cloud) is the instrument that disambiguates.

**Consequences banked:**
1. Randomization WORKS — it surfaced an order-sensitivity fixed order hid.
2. A full randomized sheet is **not reliably green on a resource-constrained
   laptop** — LOCAL's sweet spot is individual-flow + small-sheet verification
   (where every fix this session DID verify green); a full 18-flow randomized sheet
   wants real resources. LOCAL users can set `EQV_ISLAND=0` for a lighter sheet.
3. The fresh-session CLOUD run is the disambiguator: if val-b recovers there across
   seeds, it was laptop contention; if it stalls, it is a real post-restart
   convergence finding to attribute (and randomization found it).

## Deferred, with reasons (owner can re-order)

- **2b dedicated registry node:** repointing REGREF to the existing `registry` node
  changes publish semantics — it serves a FILE registry, not the chain-backed one, so
  publishes would stop driving consensus. The honest fix needs a topology decision
  (non-anchor chain-serving validator-registry vs #466's pagination relief) — a
  deliberate design choice, not a harness patch. Written here so it isn't lost.
- **2d parallel read-only flows:** the flows share FT_LAST_LINK and relaunch state;
  parallelizing without a state audit risks exactly the cross-flow races this
  session exists to remove. Sequenced after LOCAL=1 exists to test it for free.

## Born-RED, generalized — proposed as mechanism, not amendment

The consensus discipline's wedge-oracle rule ("a field finding becomes a
deterministic local RED before its fix ships") generalizes to all tiers: a cloud
FAIL/GAP may not be closed without a local failing-first test for the same
mechanism. Per discipline rule 5 (canon grows by mechanism), this ships as the
LOCAL_PROOF lint + this doc, and the canon-text amendment is left for Andrew/PE to
ratify rather than self-amended here.
