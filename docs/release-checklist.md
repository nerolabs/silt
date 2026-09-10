# Release checklist — the march to V1

**Three stages, not a step-by-step to a first tag** (see the cadence in
[ROADMAP.md](../ROADMAP.md)):

1. **Learning phase (past).** The experimental `0.1.x` / `0.2.x` tags proved
   the architecture. They were **learning releases, not steps to V1** — ignore
   them as archaeology.
2. **Feature-complete → `0.9.0`.** When every V1 gate's *mechanism* is built
   (the floors plus the M0 mission mechanism), cut `0.9.0` as the
   release-candidate line and harden it in the field.
3. **`1.0.0` = V1.** The true release candidate, cut only once the tenets are
   **field-proven multi-machine** (R1) — floors *and* pillars — per
   [launch-plan.md](launch-plan.md): credible from day one, not an experimental
   drop the community is asked to finish hardening.

**Hard M0 gates (all must pass before `1.0.0`):**
- [ ] **Consensus model-check green on the full schedule budget (#406)** — the
  deterministic I1–I5 harness (`docs/design/consensus-model-check.md`), with the
  four scar replays (#357/B2/#397/#402) proven failing-first. Per D-CONSENSUS,
  each graded field run is itself gated on the model-check tier covering its
  regime — a field run *confirms*, it never discovers a consensus invariant.
- [ ] **Multi-machine field test — the R1 gate (#52)**, including the adversarial-consensus
  SAFETY sub-suite over the real wire (equivocation-slash, partition→heavier-fork heal,
  low-bond reject, forged-block reject), not just liveness.
- [ ] **External red-team verdict against the C1 + C2 composition (#183)** — self-graded
  does not count (B8). M0 is *held* only when an EXTERNAL party (independent audit /
  public bounty / separate red-team) denies C1 (no-discount), C2 (quiet-capture), and
  I1–I5 safety at declared parameters. **The external engagement has NOT yet run — this
  is the still-open M0 close gate (ROADMAP `R4.4` / Boulder 4).** An INTERNAL
  fresh-context red-team seat attacked `5d1e303` on 2026-08-24 as PRE-external assurance
  and found no C1/C2/I1–I5 break at the shipped defaults. Per B8 the internal seat is
  silt's own party, so **self-graded does not count — M0 is BUILT and internally-clean,
  NOT externally certified as held.** The internal verdict is genuine pre-external
  evidence that lowers risk, not the B8 certification:
  `/Users/andrewedmond/Claude/claude/silt-reviews/redteam-august-23/M0-REDTEAM-VERDICT-183.md`
  (INTERNAL red-team seat — see its provenance header).
  One real bounded liveness finding (F-1, `MsgSubmitEntry` CPU gap under the opt-in
  `-require-tokens` mode) — **FIXED (PR #547)**; two harness coverage caveats (C-1
  exhaustive-tier I5/I2 oracles, C-2 disk-backed I2 durability) — **CLOSED (PR #548)**.
  The external pass is the gate (ROADMAP `R4.4`); keep this box UNCHECKED until it runs.

**Red-team entry criteria (#183 does not start until all hold) — ALL MET as of 2026-08-24:**
- [x] **#432 rounds+locking landed and model-check-green across round boundaries** (the
  I4-liveness wedge: a connected all-honest network could permanently stall — a
  catastrophic liveness finding a red team hits immediately, and it is not even
  adversarial. Blocks #183 for BOTH regimes; the P1 launch-liveness claim carries this
  asterisk until it lands. PE ruling 2026-08-15,
  `silt-reviews/principle-engineer/i4-liveness-wedge-rounds-ruling-PE-2026-08-15.md`.)
- [x] **Launch-regime interleaved publish/drain liveness drill green** (P1 confirmed
  safety and observed-run commit-capability, not liveness under the crossed-proposer
  race — the drill closes what P1 didn't cover).
- [x] Model-check green on the full schedule budget (above).
- [x] **The #399 WS-checkpoint recovery drill** built and green (flow 10).
- [x] **The local netem adversarial suite deterministic-green 10 consecutive runs**
  (`integration/adversarial SUITE=all` under `delay 80ms 20ms`) — a bimodally-red
  gate is a backstop you can't trust (PE ruling §5, 2026-08-13).
- [x] **#535 epoch-boundary liveness wedge recovery stack complete** (the blocker the
  #183 handoff was written around): stall is certified-correct safety-first BFT; recovery
  is (4) R-gate restore exemption (PR #541) + (3) operator-directed WS liveness escape
  (PR #545), with (2) automatic re-basing refuted and pinned (PR #543).

Everything below is the mechanics of cutting a tag on the RC line (`0.9.0`)
and, when the field-proof gate is met, V1 (`1.0.0`). The last step — pushing a
tag — is deliberately manual and is Andrew's to pull, *after* the relevant gate
and a personal review.

**Signing:** `1.0.0` is **signed, notarized (macOS), and checksummed** — a
release we stand behind publicly. Every build also ships a `SHA256SUMS` file so
anyone can verify it. (RC-line `0.9.x` builds may ship checksummed-only while
signing is wired up; V1 is not cut until signing/notarization is in place.)

## Before the tag (maintainer)

- [ ] **Personal review + hardening pass.** Run the daemons yourself, try
      to break them, sanity-check the honest labeling reads honestly.
- [ ] **CI green on `main`** — vet, fmt, `-race`, coverage, e2e sims,
      changelog/roadmap freshness, docs-ship, links.
- [ ] **Threat model is published and current**
      ([docs/threat-model.md](threat-model.md)) — it's the centerpiece of
      the feedback ask.
- [ ] **Honest labeling is in place and accurate:**
  - README banner matches the stage being cut (RC on `0.9.x`; a stood-behind
    release on `1.0.0`), links the threat model, and does not overclaim.
  - Website download section + `node.html` operator caution.
  - Checksum/signing claims match reality for the stage: RC builds ship
    checksums; **`1.0.0` is signed + notarized + checksummed** — claim signing
    only once it is actually in place.
  - **Every lane that has never run in the field is labelled as such.** A lane
    whose mechanism is built and proven only in simulation carries the sentence
    *"built, sim-proven, never exercised on a real network"* wherever the release
    claims it — README, website, release notes, and the flag's own help text. This
    is a RULE, not a per-lane judgment call (`D-RC-POSTURE-2026-09-09` (3)):
    omission decides these by default, and the default is always the over-claim.
    A lane earns the sentence's removal by a green graded field run that exercises
    it end to end, not by a passing simulation.
  - **There are THREE postures, not two, and one lane can hold two of them at
    once.** *Exercised* / *has not been exercised* / **cannot be exercised**. The
    middle says an operator could run the lane tomorrow and nobody has; the last
    says the shipped binary contains no client half to run, so nobody can — a
    reader hears a different promise from each, and the standard sentence above is
    only ever the middle one. Which posture a lane gets is not a judgment call:
    `scripts/check_reachability.py` builds `./cmd/silt`, reads its symbol table and
    fails the build when a label below claims a posture the linked binary
    contradicts. A label claiming a lane **cannot be exercised** must NAME the
    client entry symbol that is missing, or the claim cannot be re-checked
    (`scar:mechanism-shipped-inert-2026-09-10`). Each lane's posture is its own
    bullet for the same reason: a shared bullet claims both postures at once and
    so states neither.
  - **Open at the RC: the paid delivery lane — built, sim-proven, and it has not
    been exercised on a real network.** `OpenDeliverySessionRemote` and
    `SubmitDeliverySettle` are both linked into `./cmd/silt`, so an operator could
    open and settle a delivery session tomorrow; none ever has, which is why every
    C2 number is sim-driven. It is graded at E5 after the stamp raise.
  - **Open at the RC: the delivery top-up — it cannot be exercised, and the
    missing client entry point is `FundDeliverySessionRemote`.** Raising a live
    session's budget after admission has no client half in the shipped binary:
    `FundDeliverySessionRemote` has zero non-test callers, so the linker drops it
    out of `./cmd/silt`. The lane opens and settles but cannot top up, and that is
    a strictly stronger claim than the bullet above — do not merge the two into
    one sentence.
  - **Open at the RC: the paid relay client — it cannot be exercised, and the
    missing client entry points are `AcquireRelayAnchors`,
    `OpenRelaySessionRemote` and `SubmitRelayPay`.** The standard sentence
    over-claims here, because it implies an operator could exercise this lane and
    simply has not. Verified by `nm` on a fresh `./cmd/silt` and confirmed by
    call-graph read: all three, plus `DialThroughPaid`, are absent from the linked
    binary (zero non-test callers), while the server half is present and
    dispatched — which is exactly how the lane came to read as delivered.
    In one sentence: the paid relay lane ships server-only, is e2e-proven in
    process, and no operator can open a paid relay session from a stock build. **Keep the qualifier, because
    over-correcting here is the same failure with the sign flipped:** the server
    accepts `MsgRelayOpen` from any peer, so a third party could hand-write a
    client. The accurate scope is *"the shipped binary has no client,"* never
    *"the protocol is unreachable."* Source:
    `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/2026-09-10-inert-mechanism-sweep-core-adapters-0ed3b92.md`.
  - **OWED, not done here — one site still carries the standard sentence.**
    `cmd/silt/daemon.go`'s `-accept-relay-payments` flag help ends on *"built,
    sim-proven, never exercised on a real network"*; that file belongs to another
    seat's open branch, so it must take the paid-relay label above before the RC
    is cut. `docs/design/pod.md` §7.3's **Field status** line carried the same
    sentence and was corrected with this change.
- [ ] **No compile-time default that the genesis commits has moved since the last
      release.** Changing one is a **breaking change requiring a NEW NETWORK**, not a tuning
      change: the genesis block commits the consensus config by value, so every upgrading node
      refuses to start on the existing chain even though nobody touched its argv. This is a RULE,
      not a judgment call (`D-CFGBIND-TIER-PROMOTION-2026-09-11`, ratified by the owner after the
      behaviour was driven). It is stated here rather than left to be discovered because the
      failure is a loud, correct refusal that reads like a release bug — and the tempting fix for
      a refusal nobody expected is to weaken the check. The six defaults are enumerated in that
      decision entry; the principle is `docs/TENETS.md` Part IX.
- [ ] **`CHANGELOG.md` `[Unreleased]` is accurate** — it becomes the release
      notes verbatim. For `1.0.0` it should read like an honest first-release
      summary.

## Cutting the tag (the trigger)

Substitute the version being cut (`0.9.0` for the first RC; `1.0.0` for V1)
for `<version>` below.

1. Move `CHANGELOG.md`'s `## [Unreleased]` to a dated `## [<version>] — <date>`
   section (this is the moment "unreleased" becomes real), then
   `python3 scripts/gen_changelog.py` and commit.
2. Tag and push:
   ```sh
   git tag v<version>
   git push origin v<version>
   ```
3. The **release workflow** (`.github/workflows/release.yml`) fires on the
   tag: it runs `go vet` + `go test`, cross-compiles the Mac / Windows /
   Linux binaries via `build.sh` (which also writes `dist/SHA256SUMS`),
   extracts the notes from `CHANGELOG.md`, and publishes a GitHub Release
   with all of `dist/*` attached. The site's download links resolve once
   the Release exists. **For `1.0.0`, code-signing + macOS notarization must
   run before publish** — V1 binaries are signed, not checksummed-only.

## After the tag

- [ ] **Verify the Release**: all five binaries + `SHA256SUMS` attached;
      download one and check its hash against `SHA256SUMS`.
- [ ] **Verify the site**: the download section points at real files and
      the checksum note is present.
- [ ] **Only then**, begin feedback outreach per
      [launch-plan.md](launch-plan.md) — narrow, technical, "help us break
      this," never "store your data here."

## Rolling back

A tagged Release can be deleted from GitHub, but treat a release as
public and permanent (people may have downloaded it). If something is
wrong, prefer cutting the next patch (e.g. `v1.0.1`) with the fix over
pretending the bad release never happened.
