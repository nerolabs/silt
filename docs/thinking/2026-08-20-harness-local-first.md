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
- **Maturing-latch e2e: parked as the named residual** (per the timebox above).
  The honest reason: a post-shed weight-quorum drill needs the maturers to hold
  >⅔ of frozen epoch WEIGHT (bond-size asymmetry or a 7-maturer fleet), a regime
  design worth doing deliberately, not at the end of a long session. In-process
  coverage stands (sim latch + mature model-check fixtures); the LOCAL_PROOF
  annotation on flow_maturing_handoff records it.

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
