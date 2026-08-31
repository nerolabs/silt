# Phase 5 scoping — the operational floor (a node a person can run)

**Date:** 2026-08-31
**Author:** Builder seat
**Status:** SCOPING / DEFINITION ONLY — no code. Decomposes ROADMAP.md Phase 5 into
sized, ordered increments with per-increment gated-surface flags.
**Branch:** `docs/phase5-operational-floor-scoping` off `origin/main` `61c75eb`.

---

## 0. What Phase 5 is, and its exit gate

ROADMAP.md Phase 5 (`ROADMAP.md:141-148`) has one job: turn silt from a binary a
developer runs into **a node a non-developer installs and forgets**. Two workstreams,
distinct in kind:

1. **Operability (pure-ops).** Per-platform service packaging (launchd / systemd /
   Windows service), signed installers, and operator-consented self-update per R4.
2. **The S6 scaling kills (proof machinery).** Two steady-state costs today scale with
   the *whole held set*, not with churn:
   - the **O(store) restart scan** — proof-maturation rebuilds the resident `proofMeta`
     index by reading every proof in the store at boot;
   - the **O(held) reprovide re-sign** — every reprovide interval re-signs a provider
     record for every held placement key, changed or not.

**Exit gate (verbatim, `ROADMAP.md:146-148`):** a non-developer installs a node that
**survives reboot and returns to serving in seconds**, and **steady-state cost no longer
scales with the whole held set (S6)**.

That gate decomposes into two independently-checkable demos:

- **Operability demo:** on a clean machine, a non-developer runs one installer, the node
  registers as a platform service, survives a reboot, and is back serving within seconds
  (not the ~9 min of F3 downtime).
- **S6 flatness demo:** on a large store (target the flixz F3 scale — 14 GB / ~381K
  files), measured restart-to-serving time and per-interval reprovide CPU are **flat in
  held-set size** — they scale with *what changed since last boot / last interval*, not
  with the whole catalog.

---

## 1. The evidence — the two hot paths, grounded in code

### 1a. The O(store) restart scan (proof maturation)

`Node.StartProofReload` (`core/node/node.go:980-1002`) and `reloadProofBatch`
(`core/node/node.go:1004-1028`) rebuild the resident `proofMeta` map at boot by listing
**every** key in the proof store (`n.proofs.Keys()`) and `Get`-ing each proof in
128-key batches (`proofReloadBatch = 128`, `core/node/node.go:978`).

History matters here, because it bounds what is left to do:

- The **F3 fix already shipped**: the scan was moved from a *synchronous, startup-blocking*
  `LoadProofs` onto the event loop in bounded async batches, so the daemon binds its
  relay/registry listeners immediately (`cmd/silt/daemon.go:489-495`,
  `core/node/proof_reload_test.go:13-29`). Evidence for the original defect: flixz F3 —
  a real 14 GB / 381K-file store scanned synchronously for **~8m45s** before the relay
  (4002) and registry (4003) listeners bound, i.e. ~9 min of downtime per restart, growing
  with store size (`core/node/node.go:984`, `cmd/silt/daemon.go:472`).
- **What F3 did NOT fix:** the scan is still **O(store) total work**. It is merely no
  longer on the critical path. The daemon serves during the scan, but the *whole* store
  is still read on every boot. The code says so in its own words: *"A metadata sidecar
  (O(delta) cold start) is the tracked fast-follow that makes the background scan itself
  cheap."* (`core/node/node.go:990-991`). That fast-follow is Phase 5 increment S1.

So the S6-relevant residual is: **total work per restart is O(store), and the reload does
real disk reads + decode for every proof even when almost nothing changed since the last
clean shutdown.**

### 1b. The O(held) reprovide re-sign

`Node.StartReprovide` (`core/node/repair.go:134-162`) schedules `AnnounceHeld`
(`core/node/repair.go:99-132`) every `ProviderRecordTTL/2`, floored at 60 s. Default
`ProviderRecordTTL` = 30 min (`cmd/silt/daemon.go:111`, `:311`, `:1614`), so **a full
re-announce fires every ~15 min**.

Each `AnnounceHeld` walks the entire store (`n.store.List`), and for every held placement
key calls `n.providerRecord(key)` (`core/node/repair.go:124`), which performs an
`ed25519.Sign` (`core/node/provider_records.go:30-40`) when signed provider records are on
(the untrusted-swarm default). On the F3-scale store that is **~381K ed25519 signatures
every ~15 minutes, forever, regardless of how many records actually needed refreshing.**
Nothing tracks *which* records are near expiry or *which* changed; every interval re-signs
the whole held set. That is the O(held) per-interval re-sign the S6 kill targets.

Note a subtlety that shapes the increment: reprovide is not pure waste. A full
`AnnounceHeld` re-stamps this node's own records **and re-plants them on near-nodes**
(`core/node/repair.go:142-144`), so multi-holder discovery survives peer churn. Dirty-
tracking must preserve the "re-plant so others can find me" property, not just the "re-sign
my own record" property. This is the load-bearing correctness constraint on S2 (see §5).

---

## 2. flixz context — why this phase is now, not later

flixz.com is silt's first real production user (see `docs/thinking/` flixz references;
the F3 finding itself came from a flixz store). A production operator needs the node to:
(a) install without a Go toolchain, (b) survive reboots as a managed service, and (c) not
pay a ~9-min downtime tax + a full-store re-sign tax on a large catalog. Both Phase 5
workstreams trace directly to a real operator failure, not to speculative polish. That is
the justification for building them; the pure-ops half is otherwise the kind of gold-
plating the Builder seat is supposed to resist. It clears the bar because flixz F3 is a
measured production incident, not a hypothetical.

---

## 3. The increments

Grouped as the task requires: (a) pure-ops packaging/installers/service integration,
(b) the S6 scaling kills, (c) R4 signed self-update. Sizes are S/M/L. Every increment
names its exit condition (how an *outsider* checks it is done — a passing test, a measured
number, or a demo an operator can run).

### Group A — pure-ops (packaging / installers / service integration). UNGATED.

These touch no proof, no consensus rule, no economic mechanism, no security parameter, and
no published claim. They ship the same tested binary under a service manager. **Safe to
build without owner ratification.** The one seam that is NOT purely ops — the code-signing
/ trust chain for installers — is deliberately factored into Group C, because it shares the
R4 trust-root question.

**A1 — Service unit definitions (launchd + systemd).**
*What:* a launchd plist for macOS and a systemd unit for Linux that run `silt daemon`
with a durable store path, `Restart=on-failure`-class supervision, and correct
network-online ordering. A starting template already exists but is explicitly *throwaway
dev infra, not operator-facing* (`deploy/silt-dev-relay.service:1-2`); this increment
produces the real operator unit (correct user, store dir, restart policy, log routing —
recall node/chain narration goes to the log file, not stdout).
*Exit condition:* on a Linux box and a macOS box, `enable --now` (systemd) /
`launchctl load` (launchd) starts the daemon, it serves, and **it comes back after a
reboot** without manual intervention. Demonstrated on a real reboot, not asserted.
*Size:* **S.** *Depends on:* nothing. First increment.
*Gated surface:* **none (pure-ops).**

**A2 — Windows service integration.**
*What:* register `silt daemon` as a Windows service (SCM). Windows is the platform silt
has never targeted; expect the real cost in path/permission/console-handling differences,
not in the service registration itself.
*Exit condition:* on a Windows host, the service installs, starts, serves, and survives a
reboot.
*Size:* **M** (new platform surface, likely small daemon-side portability fixes).
*Depends on:* A1 (reuse the supervision/store-path conventions). Can slip after the
Linux/macOS demo without blocking the exit gate on those two platforms.
*Gated surface:* **none (pure-ops)** — unless a portability fix touches the proof/store
paths, in which case that fix is scoped and reviewed on its own merits.

**A3 — Installer packaging (unsigned artifacts first).**
*What:* a per-platform installer that drops the binary, installs the A1/A2 service unit,
and creates the store dir with correct ownership. macOS `.pkg` / launchd, a Linux
package or install script, a Windows installer. **Unsigned in this increment** — signing
is A4, gated by the trust-root decision.
*Exit condition:* a non-developer, on a clean machine, runs one artifact and ends with a
running, reboot-surviving node — no Go toolchain, no manual unit editing. This is half the
exit-gate demo (operability).
*Size:* **M.** *Depends on:* A1 (Linux/macOS), A2 (Windows).
*Gated surface:* **none (pure-ops)** for the packaging mechanics. The *signing* of these
artifacts is A4.

**A4 — Signed + notarized installers.**
*What:* code-sign and (macOS) notarize the A3 artifacts so an OS gatekeeper accepts them
without a scary-warning bypass. ROADMAP notes a working downstream macOS
signed+notarized+launchd reference exists and generalizes (`ROADMAP.md:143-144`).
*Exit condition:* a signed installer runs on a stock macOS / Windows machine **without a
Gatekeeper/SmartScreen override**, and the signature verifies.
*Size:* **M** (mostly signing-key logistics + CI, not silt code).
*Depends on:* A3.
*Gated surface:* **trust-surface, adjacent to R4.** Installer signing establishes the
*release trust root* — the key operators implicitly trust when they install. This is the
SAME trust question as R4 self-update (Group C). It is **not** a consensus/economic/
published-claim gate, but the choice of signing key custody, key rotation, and whether the
installer signature and the R4 update-manifest signature share a root is a **security
decision that needs owner ratification** (and should be decided *together* with C1). Flag:
**owner-ratification required for the key-custody / trust-root model**, not for the
packaging mechanics.

### Group B — the S6 scaling kills (proof machinery). PERF, assess-gated.

These are the substance of the S6 half of the exit gate. Both touch the proof machinery,
so each is assessed for whether it changes **what is proven** (gated) or only **how often /
how incrementally the same thing is computed** (ungated perf). My read on both: **ungated
perf, provided the assessment below holds** — but the assessment itself is a claim the PE
should confirm before build, because "a durability knob was twice also a security
parameter" (`docs/build-process.md`) is a live scar here.

**S1 — O(delta) proof-maturation cold start (kill the O(store) restart scan).**
*What:* a persisted metadata sidecar so boot restores the resident `proofMeta` index from
a compact durable index (written at shutdown / incrementally) plus a **reconciliation of
only what changed** since that index was written, instead of reading every proof in the
store. The named fast-follow in `core/node/node.go:990-991`. The reload path to replace is
`StartProofReload` / `reloadProofBatch` (`core/node/node.go:980-1028`).
*What it must NOT change:* the CONTENTS of `proofMeta` after boot must be byte-identical to
today's full scan. `proofMeta` is the small always-resident projection of a `StorageProof`
(`core/node/proofbacking.go:5-20`, `metaOf`); the sidecar is a cache of that projection,
not a new source of truth. The full proofs still page from disk on demand (#464); this
increment does not touch what a proof *is* or what an audit *checks*.
*Why I assess it UNGATED (perf):* it changes *how often and how incrementally* the
resident index is rebuilt, not *what is proven*. No validity predicate, no audit
challenge/response, no consensus rule, no economic quantity reads the sidecar. It is a
boot-time cache-warming optimization. **The correctness bar is a divergence oracle**, not a
research certification: the sidecar-boot `proofMeta` must equal the full-scan `proofMeta`
for the same store, including under a dirty shutdown (crash mid-write). That oracle is the
exit condition and the regression test.
*The honest unknown / the ablation risk:* the trap is a sidecar that goes stale silently —
a chunk added/deleted/re-proofed while the daemon was down, or a torn write on crash, and
the boot index disagrees with the store. A green "sidecar loads fast" test that never
injects a stale/torn sidecar is decoration (the session-7 lesson: *inject the defect and
watch it go red*). The reconciliation-of-what-changed step is the load-bearing part and the
hardest part; a design that cannot cheaply detect "store changed under me" collapses back
to O(store). **Consult the PE before build** to confirm the ungated-perf classification and
to pressure-test the staleness-detection design.
*Exit condition:* (1) divergence oracle green — sidecar-boot `proofMeta` ≡ full-scan
`proofMeta`, including a crash-mid-write case that must fall back to a correct (possibly
slower) rebuild, not a wrong index; (2) measured restart-to-`proofMeta`-resident time on an
F3-scale store is **flat in store size** given bounded delta (scales with what changed,
not the catalog).
*Size:* **L** (durable sidecar format + incremental write + crash-safe reconciliation +
the divergence oracle).
*Depends on:* nothing in Group A; independent track. Do it early — it is the harder of the
two S6 kills and the exit gate needs it.
*Gated surface:* **proof machinery — assessed UNGATED (perf), PE to confirm.** Does not
change what is proven. Build-after-confirm, not build-freely.

**S2 — Reprovide dirty-tracking (kill the O(held) per-interval re-sign).**
*What:* track which provider records are near expiry / newly held / dropped, and each
reprovide interval refresh **only the dirty subset** instead of re-signing the whole held
set. The path to change is `StartReprovide` → `AnnounceHeld`
(`core/node/repair.go:99-162`); the per-key `ed25519.Sign` is `providerRecord`
(`core/node/provider_records.go:30-40`).
*What it must NOT change:* the discovery guarantee. Two properties AnnounceHeld provides
today and dirty-tracking must preserve: (a) this node's own records stay Live (never lapse
past `ProviderRecordTTL`), and (b) records get **re-planted on near-nodes** so multi-holder
discovery survives peer churn (`core/node/repair.go:142-144`). A naive "only re-sign
records near expiry" that stops re-planting on churned peers would silently regress
discoverability — the #69 dark-node failure mode this whole loop exists to prevent
(`core/node/repair.go:138-141`). So dirty-tracking is over *two* dimensions: record-expiry
AND target-freshness (a peer that newly entered my near set needs my record even if my
record is not near expiry).
*Why I assess it UNGATED (perf):* it changes *how often* a given record is re-signed and
re-planted, not the record's meaning, the signing key, the freshness-lease semantics, or
what a fetcher verifies (`ProviderSigningBytes`, TTL, H5 self-certification all unchanged).
No consensus, economic, or published-claim surface. **But** the discovery-guarantee
constraint above is a real correctness cliff — a wrong dirty-set is a liveness/
discoverability regression, exactly the class the #69 residual and the reprovide loop were
built to close. So: perf, but perf with a sharp correctness edge.
*The honest unknown / the ablation risk:* the defect to inject is a peer that joins my near
set between full sweeps — does dirty-tracking still plant my record on it before a reader
looks there? A green "dirty set skips unchanged records" test that never introduces a
newly-near peer is decoration. The regression test must reproduce #69's dark-node condition
under dirty-tracking and show it stays findable.
*Exit condition:* (1) a test reproducing the #69 dark-node / churned-near-peer condition
stays green under dirty-tracking (findability preserved); (2) measured per-interval
reprovide CPU / signature count on an F3-scale store is **flat in held-set size** — it
scales with churn + expiry, not the catalog.
*Size:* **M–L** (the expiry-index is straightforward; the target-freshness dimension —
detecting a newly-near peer cheaply — is the hard part and may lean on existing routing-
table change signals).
*Depends on:* nothing in Group A. Independent of S1 (different hot path). Can run in
parallel with S1.
*Gated surface:* **proof/discovery machinery — assessed UNGATED (perf), PE to confirm.**
Does not change what a provider record proves or how it verifies. Build-after-confirm.

### Group C — R4 signed self-update. SECURITY / TRUST SURFACE. GATED.

**C1 — Operator-consented signed self-update (R4).**
*What:* the update mechanism R4 specifies (`docs/TENETS.md:386-394`): the network **never
silently auto-updates**; operators control their own software; only *security* uses
graduated enforcement, set by maintainers, as **criticality-graded** (Low = advisory /
Medium = 30 d / High = 7 d / Critical = 24–48 h) **threshold-signed (m-of-n)**,
**recallable (monotonic sequence)**, **observation-clocked** version-floor advisories that
**fail open**. Signed manifests, never silent (`ROADMAP.md:142-144`).
*Why this is GATED — do NOT build on a Builder decision:* R4 is in the tenet/immutable
material (`docs/TENETS.md:773`, `:813` — B7/Don't #7/R4 tier placement), and the mechanism
is *itself the safety property*: "Whoever can declare Critical can halt the network, so no
single key may — the signing threshold *is* the safety property" (`docs/TENETS.md:392-394`).
That makes every parameter here a **security parameter a proof/claim depends on** — the
exact research-gate category in `.claude/CLAUDE.md` and the scar in `docs/build-process.md`.
The signing threshold `m`, the key custody, the recall/monotonic-sequence rule, the
observation-clock, and the fail-open behavior are **not** Builder calls. They route to the
Researcher for certification and the owner for ratification. The Builder advises and shapes
the question; it does not decide it.
*Exit condition (of the DEFINITION, not the build):* a certified spec for the R4 update
mechanism (threshold, custody, recall, clock, fail-open) exists and is owner-ratified.
*Only then* does an implementation increment open, and its own exit condition is: an
operator receives a signed advisory, is prompted (never silently updated), can decline a
non-Critical advisory, and a forged/single-key advisory is rejected (threshold enforced) —
each with a failing-first test.
*Size:* **L**, and **most of it is upstream of code** — the spec + certification is the
long pole. Implementation is medium once the mechanism is certified.
*Depends on:* the trust-root decision shared with A4 (installer signing). Decide the key
custody / trust-root model **once**, for both A4 and C1, at the research/owner gate.
*Gated surface:* **security parameter + trust surface. GATED. Owner ratification + research
certification REQUIRED before any build.** This is the one Phase 5 workstream the Builder
must not start on its own authority.

---

## 4. Recommended build order + the exit-gate demo

**Ordering rationale:** ship the outsider-checkable operability win first (A1→A3 clears
half the exit gate and unblocks flixz on Linux/macOS with the least risk), run the two S6
kills as an independent parallel track (they gate on a PE classification, not on packaging),
and route the two trust-surface items (A4 signing, C1 self-update) through one shared
research/owner gate rather than deciding a trust root twice.

1. **A1** (launchd + systemd units) — S, ungated. First outsider-visible win.
2. **A3** (unsigned installers, Linux/macOS) — M, ungated. Completes the operability half
   of the exit-gate demo on the two platforms flixz needs.
3. **S1** (O(delta) cold start) and **S2** (reprovide dirty-tracking) — L / M–L, perf,
   PE-classification-gated. **Parallel track**, started early (independent of A1/A3). Get
   the PE's ungated-perf confirmation FIRST, then build with the divergence/findability
   oracles as failing-first tests.
4. **A2** (Windows service) — M, ungated. Broadens platform reach; not on the critical path
   for the Linux/macOS exit-gate demo.
5. **GATE — decide the trust root once.** Route A4-signing + C1-R4 as a single
   security/trust question to the Researcher (certification) and owner (ratification).
   Nothing in Group C, and no *signed* installer (A4), is built before this returns.
6. **A4** (signed/notarized installers) — M, on the ratified trust root.
7. **C1** (R4 self-update) — L, on the certified R4 mechanism.

**The exit-gate demo (two independently-checkable halves, per `ROADMAP.md:146-148`):**

- **Operability:** on a clean Linux and macOS machine, a non-developer runs one installer
  (A3, later A4-signed), the node registers as a launchd/systemd service (A1), and after a
  **real reboot** it returns to serving **within seconds** — not the ~9 min F3 downtime.
  Windows (A2) added when ready.
- **S6 flatness:** on an F3-scale store (target 14 GB / ~381K files), (a) restart-to-serving
  time is **flat in store size** given bounded change since last boot (S1 divergence oracle
  green), and (b) per-interval reprovide signature count / CPU is **flat in held-set size**,
  scaling with churn + expiry only (S2 findability regression test green). Measured numbers,
  derived not guessed, on a fair box — same discipline as the node-store measurement.

The exit gate is met when **both** halves demonstrate on an outsider-runnable artifact.
Consistent with the VISION discipline (`docs/VISION.md`): a step is real when it moves an
outsider's checkable answer, not builder confidence.

---

## 5. Gated-surface summary (the answer the task asks for)

| Increment | Group | Size | Gated surface | Owner ratification? |
|---|---|---|---|---|
| A1 launchd + systemd units | pure-ops | S | none | no — build freely |
| A2 Windows service | pure-ops | M | none | no — build freely |
| A3 unsigned installers | pure-ops | M | none | no — build freely |
| A4 signed/notarized installers | pure-ops + trust | M | **trust root (shared w/ C1)** | **yes — key custody / trust-root model** |
| S1 O(delta) cold start | S6 kill | L | proof machinery — **assessed UNGATED (perf)** | no ratification, but **PE confirms classification first** |
| S2 reprovide dirty-tracking | S6 kill | M–L | proof/discovery machinery — **assessed UNGATED (perf)** | no ratification, but **PE confirms classification first** |
| C1 R4 signed self-update | R4 | L | **security parameter + trust surface** | **YES — research certification + owner ratification REQUIRED before build** |

**Safe to build without owner ratification:** A1, A2, A3 (pure-ops), and S1, S2 **after**
the PE confirms the ungated-perf classification (they touch proof machinery, so the
classification is not the Builder's to assert alone — it is checked, then built with
failing-first divergence/findability oracles).

**Requires a gate:** A4's trust-root/key-custody model (owner), and C1 in full (research
certification of the R4 mechanism + owner ratification). Decide the trust root **once** for
both.

**The Builder's stance on scope:** Group A is the kind of packaging polish this seat
normally resists as gold-plating. It clears the bar because it traces to a measured
production incident (flixz F3), not to speculative completeness. Groups B and C are load-
bearing: S1/S2 are the literal S6 exit condition, and C1 is a named immutable-tier promise
(R4). Nothing here is built ahead of its evidence, and the two trust-surface items are
explicitly held for the gate rather than pre-guessed.

---

## 6. Honest unknowns

- **S1 staleness detection is the crux and is unproven.** Cheaply detecting "the store
  changed while I was down" (and surviving a torn/crash-mid-write sidecar) is the whole
  increment. If it cannot be made cheap, S1 collapses back to O(store) and the design needs
  rethinking before build. The divergence oracle (including a crash case) is mandatory, not
  optional — a fast-but-wrong index is worse than the slow-but-correct scan we have.
- **S2's target-freshness dimension is harder than the expiry dimension.** Detecting a
  newly-near peer cheaply (without a full sweep) may require hooking existing routing-table
  change signals; if none exist cheaply, the increment may need a bounded periodic
  reconciliation floor to stay correct, which caps how flat it can get. Measure before
  claiming flatness.
- **The A4 / C1 trust root is undecided and is not the Builder's to decide.** Whether the
  installer-signing key and the R4 update-manifest threshold share a root, key custody, and
  rotation are open security questions for the gate.
- **Windows (A2) portability cost is unmeasured.** silt has never targeted Windows; the
  service registration is easy, the path/permission/console-handling fixes are the unknown.
- **The PE classification of S1/S2 as ungated-perf is my assessment, not a verdict.** The
  "durability knob was twice a security parameter" scar means this must be confirmed, not
  assumed, before either is built.

**No code was written. This document is definition only.**
