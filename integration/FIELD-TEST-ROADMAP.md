# Field-test roadmap — per-substrate parity + the honest backlog

The `integration/README.md` "Roadmap: per-substrate parity" section points here.
This is the tracked backlog for the field-test tree: where local and GCP coverage
should converge, plus the ranked extension/hardening opportunities surfaced by the
2026-08-10 audit (read alongside [`FIELD-TEST-STATUS.md`](FIELD-TEST-STATUS.md),
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

1. **[coverage] Run the GCP acceptance pass for real.** The billable pass is the
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

6. **[truthfulness] `churn` exit code + status row.** churn self-describes as
   "expected to fail" but exits non-zero (scored FAIL), when a characterized
   shortfall should be a `RESULT: FINDING` (exit 0, `EXPECT=pass` flips it). Split
   the exit like `chaos`/`durability`, and keep its row in `FIELD-TEST-STATUS.md`
   current.

7. **[truthfulness] Automate `bond` C1 "reputation ∝ bond."** Currently
   hand-recorded: the run asserts only reputation ≠ 0 at a single bond size. Seal
   two bonds (e.g. 16M and 64M), read each earned `reputation=` off the real
   `standing self` line, and assert the ratio is roughly linear (≥ ~3.5×) so a
   flat-reputation daemon (breaking no-discount) fails.

8. **[truthfulness] `nat` hole-punch: assert the direct-path outcome.** The direct
   connection is asserted from a log line the daemon emits *before* the TLS/identity
   handshake; assert instead that post-punch traffic bypassed the relay (relay
   `splice` count did not grow) and bytes flowed. Give symmetric-NAT a positive
   control (the relay path was genuinely exercised) so "nothing happened" can't
   score as "correctly fell back to relay."

9. **[truthfulness] `upgrade` chain reload.** `CHAIN_OK` is true-by-absence-of an
   error substring; assert a positive head/height/restored-block count instead, and
   treat empty/failed `chain-status` as a harness error.

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
