# Retired field-test roadmap — folded into ROADMAP (SSOT), archived 2026-09-01

> ⚠ **HISTORICAL — FROZEN, NOT THE PLAN.** This was a SECOND task roadmap for the
> field-test tree. Its still-live parity/coverage/truthfulness items were **folded into
> `ROADMAP.md`'s "Residual backlog" section** ("Field-test harness residuals") on
> 2026-09-01 as part of the one-task-SSOT consolidation. `ROADMAP.md` is the single
> source of truth for forward work. Read this only for provenance and the per-item fix
> directions. The RC field-test GATE itself is MET — see the current state in
> [`../integration/FIELD-TEST-STATUS.md`](../integration/FIELD-TEST-STATUS.md).
>
> **Fold-map (what became of each ranked item):**
>
> | # | Item | Disposition |
> |---|---|---|
> | 1 | GCP acceptance pass | **DONE** — RC run `585c82a-58990` graded 28P/0G/0F/2-skip (#532, `eb57d50`); deep lineage `fe2376a`-deep 30P/1G/0F. |
> | 2,4,5,6,7,8,10 | harness truthfulness gaps | **Folded** → ROADMAP Residual backlog "Field-test harness residuals" (tracked under #303, the test-honesty audit). |
> | 3 | C2-Sybil cloud flow | **Folded** → ROADMAP Residual backlog (harness coverage; kin to the #303 audit + R4 M0 gates). |
> | 9 | `upgrade` chain reload | **DONE** — CHAIN_OK asserts positive head height (`upgrade/run.sh:239`). |
> | 11 | Wire demand (#264) | **Folded** → ROADMAP Residual backlog #264 (the demand daemon-wire seam). |
> | 12 | `chaos` WAVE-2 redundant bootstrap | **Folded** → ROADMAP Residual backlog "Field-test harness residuals". |
> | 13,16 | parity + AWS/two-cloud | **Folded** → ROADMAP Residual backlog "Field-test harness residuals" (parity + GCP-only + far-end AWS). |
> | 14 | #281 empty-routing-table wire-cert | **Folded** → ROADMAP Residual backlog "Field-test harness residuals". |
> | 15 | GCP substrate operability | **RETITLED — gate MET, quota/preflight hardening still worthwhile.** The full 13-node run completed and graded (the RC sheet); the IP-quota/zone-capacity pre-flight + public-IP-footprint shrink + `nuke` label-sweep are folded → ROADMAP Residual backlog. |

---

**Original content (frozen 2026-09-01) follows.**

# Field-test roadmap — per-substrate parity + the honest backlog

The `integration/README.md` "Roadmap: per-substrate parity" section points here.
This is the tracked backlog for the field-test tree: where local and GCP coverage
should converge, plus the ranked extension/hardening opportunities surfaced by the
2026-08-10 audit (read alongside [`FIELD-TEST-STATUS.md`](../integration/FIELD-TEST-STATUS.md),
the honest current state).

## Direction: one scenario, either substrate

Today the mapping is asymmetric — local is a set of focused per-test Docker
harnesses; GCP is one combined acceptance run. The direction is **parity**: factor
a shared node abstraction (`exec-on-node` + `assert-on-log`, already the shape of
both `docker exec` locally and IAP-SSH on GCP) so the *same* scenario can target
either substrate, then add the GCP-only scenarios real hardware can answer:
scale-out repair-under-churn (50+ nodes), a real firewall partition for consensus,
`tc` link-shaping for fetch-under-load, and long-haul soak.

## Ranked backlog

Severity/verdict tags: **[truthfulness]** = the test could pass while the property
is broken (fix first); **[coverage]** = a real gap to close; **[parity]** = the
local↔cloud convergence work above.

1. **[coverage] Run the GCP acceptance pass for real — DONE.** The RC field-test
   gate is MET: RC run `585c82a-58990` graded **28 pass / 0 gap / 0 fail /
   2 skip-by-design** (#532, `eb57d50`), on the deep-run lineage `fe2376a`-deep
   (30P/1G/0F). The multi-region run graded the durability retrieval floor, the pure
   Sybil anchor gate, and real inter-region timing. (Original open framing preserved
   below for provenance.) The billable pass was the
   actual remaining gate. First-run blockers are now fixed (see the cloudtest
   first-run PR); do `SMOKE=1` → full, tune SLOs/log-regexes on first contact,
   then let the multi-region run decide the durability retrieval floor, the pure
   Sybil anchor gate, and real inter-region timing.

2. **[truthfulness] consensus P0 negative control.** The unbonded-publish "refused"
   check runs a lone daemon whose required attester is unreachable, so it commits
   0 blocks for a reason unrelated to earned-standing enforcement, and has no
   positive control proving the assertion can fire. Give it a real second attester
   (so a quorum is assemblable) and assert on the daemon's real refusal reason
   (`reputation below threshold` / `ErrLowReputation`), plus an inverted control
   that earned standing *does* commit.

3. **[coverage] #5 C2-Sybil cloud flow.** Add non-anchor Sybil validator VMs to
   `topology.py` (a `sybil` role: `-validator` + `-anchors <real anchors>`, not in
   the anchor set, equal `-bond`); over a longer run the anchors' blocks bank the
   Sybils' `BondReg`s so the capture attempt hits the pure `ErrAnchorRequired`
   gate (not the laptop's standing gate) and ≥8 equal bonds trip the atomization
   note. Then a `flow_c2_no_capture` (stop anchors → Sybil quorum refused).

4. **[truthfulness] Read gate reasons from the daemon, not the client.** `sybil`
   C2-b and `privacy`/`consensus` refusal classification should read the *reason*
   from the validator daemon log, not the `swarm add` client stdout (which does not
   carry the async commit-refusal). The pass oracle (frozen height/commit count)
   stays; only the reported reason gets sourced correctly.

5. **[truthfulness] `soak` memory + finding vocabulary.** "bounded memory" is
   gated by a near-unfalsifiable 3-sample monotonic+3× test and prints "memory
   bounded" regardless; sample RSS per burst and gate last-third vs first-third
   growth, failing hard if any sample is 0 (measurement failed ≠ healthy). And let
   soak emit `RESULT: FINDING` (exit 0) for a leak/restart shortfall instead of
   mislabeling it `FAIL`.

6. **[truthfulness] `churn` exit code + status row — EXIT SPLIT DONE; seeded
   placement OPEN.** The exit split is done (`churn/run.sh:257-262`): characterized
   shortfall → `RESULT: FINDING` (exit 0), repaired-but-unfetchable → `RESULT: FAIL`
   (exit 1), `EXPECT=pass` flips the FINDING; the roll-up scores it FINDING. The
   `FIELD-TEST-STATUS.md` row is updated. **Still open:** churn's outcome is sensitive
   to random shard placement — a single run may hit the "coverage held within the
   erasure margin" branch and reconstruct nothing (a weaker demonstration). Seed the
   placement (or force a below-coverage stripe deterministically) so every run
   guarantees a forced repair-and-refetch, keeping the coverage-held case as a
   separate, explicitly weaker signal.

7. **[truthfulness] Automate `bond` C1 "reputation ∝ bond." (source of truth — OPEN.)**
   The suite already gates plot-**residency cost** (plot-size ≥ 90% of `-bond`) and
   PHASE-3 root-owner dedup — the real "no discount" mechanism — but it does **not**
   yet assert reputation **proportionality**: PHASE 1 checks only `reputation=[1-9]` at
   a *single* bond size (`bond/run.sh:144`). Seal two bonds (e.g. 16M and 64M), read
   each earned `reputation=` off the real `standing self` line, and assert the ratio is
   roughly linear (≥ ~3.5×) so a flat-reputation daemon (breaking no-discount) fails.
   `FIELD-TEST-STATUS.md` previously marked this DONE while this item said open — the
   contradiction is resolved in STATUS's favour-of-this-item: it is **not DONE**.

8. **[truthfulness] `nat` hole-punch: assert the direct-path outcome.** The direct
   connection is asserted from a log line the daemon emits *before* the TLS/identity
   handshake; assert instead that post-punch traffic bypassed the relay (relay
   `splice` count did not grow) and bytes flowed. Give symmetric-NAT a positive
   control (the relay path was genuinely exercised) so "nothing happened" can't
   score as "correctly fell back to relay."

9. **[truthfulness] `upgrade` chain reload — DONE.** `CHAIN_OK` now asserts a
   *positive* head/height (`upgrade/run.sh:239`: `head height:` / `blocks: [1-9]` and
   must NOT say `no chain yet`), not the mere absence of an error substring; the "V1
   committed no chain" case is handled explicitly. The blind field test confirmed the
   FINDING it isolates (#237) is real.

10. **[truthfulness] `redteam` honest-target cross-check.** The reject signal comes
    from the adversary's own container; also assert the honest target H3's chain
    head height is unchanged (it committed no adversarial block).

11. **[coverage] Wire demand (#264)** so #6 (`demand`) becomes a real field test:
    P2 fair-exchange abort ⇒ token reusable; P3 wash ⇒ one bonded identity + a real
    fee per unit of demand. Until then it stays a stated gap (no daemon-wire seam).

12. **[coverage] Root-cause the `chaos` WAVE 2 observation.** Does a *redundant*
    bootstrap (≥2 registry/seed nodes) survive one crashing? Pin it, then fix +
    assert or downgrade to a documented single-bootstrap topology limitation.

13. **[parity] Shared node abstraction + GCP-only scenarios** — the convergence
    work described above (scale-out churn, real firewall partition, `tc` shaping,
    long-haul soak).

14. **[truthfulness] Wire-certify #281 (empty-routing-table self-heal).** #281 is
    fixed in-product (`Node.StartBootstrapRetry`, `-bootstrap-retry=15s`, unit-tested)
    and the cloud startup script *also* fastens a TCP-wait belt
    (`provision/silt-startup.sh`) — so no flow exercises an empty-routing-table join
    over the wire, certifying neither the defect nor the fix. Add one cloud flow that
    **disables the startup TCP-wait on a single joining validator** and asserts the
    real `re-bootstrapped: recovered from an empty routing table` line, turning the
    narrated story into a measured property.

15. **[coverage] GCP substrate operability — RC gate MET; quota/preflight hardening
    still worthwhile.** The full 13-node run has since completed and graded (the RC
    sheet in item 1). The remaining work is the environmental-robustness hardening below;
    it does NOT block the gate. The original "not reachable today" framing is preserved
    for provenance: the full 13-node run was blocked by two *environmental* constraints
    (not product
    bugs): a `us-central1-a` E2 capacity shortage (8 of 13 nodes land there; confirmed
    across e2-small and e2-medium) and the default `IN_USE_ADDRESSES` = 8/region quota
    (the topology needs ~11 external IPs, so a single-zone `PIN_ZONE` run blows it, and
    the multi-region spread that fits the quota then depends on us-central1-a). Add a
    **pre-flight** that checks `IN_USE_ADDRESSES` headroom + zone capacity before
    `apply`, and **shrink the public-IP footprint** (IAP-only / bastion) so a
    single-zone full run fits the default quota and can target whichever zone has
    capacity. Also: the `nuke` fallback leaks the (free) VPC/subnets/firewall/routes on
    a partial-apply teardown — make `nuke` sweep those by label too, and stop
    swallowing `terraform destroy` stderr.

16. **[parity] AWS variant + two-cloud field test.** Build an AWS variant of
    `cloudtest` as a fallback substrate when GCP capacity/quota blocks the RC gate
    (mirror the Terraform topology + flows + teardown/cost-safety traps; SSM/bastion in
    place of IAP-SSH). Once both clouds pass independently, add a **two-cloud** field
    test that splits the topology across GCP and AWS to exercise real inter-provider
    WAN latency and distinct network stacks — the closest thing to real-world
    conditions.
