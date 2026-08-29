# Orchestra coordination — silt

The seats in `.claude/agents/` operate the orchestra way (below) while building the silt
way (silt's own canon). This file is the bridge. It loads every session in the silt repo.

## The project constitution — silt's canon is the law here

The seats build under silt's existing rules. They are not overridden by anything here:

- **The mission (immutable):** `docs/TENETS.md` Part 0 — M0, the privacy × accountability ×
  Sybil trilemma. Everything defers to it.
- **The three tiers:** `docs/TENETS.md` Part IX — immutable / tenet / evolving. Know which
  tier a decision lives in before you make it. Immutables are never traded by an agent.
- **The bright lines:** `docs/TENETS.md` Part VI — the don'ts.
- **Decisions + roadmap:** `docs/decisions.md`, `ROADMAP.md` — the ledger and the order.
- **Build discipline:** `docs/build-process.md` — #6 root-cause-before-you-patch,
  #7 evidence-or-nothing, and the consensus-correctness discipline (model-check before
  field; a field run confirms, never discovers).
- **Consensus invariants:** `docs/design/consensus-invariants.md` (I1–I5), the closed set.

## Coordination rules — the orchestra way (reusable methodology)

1. **Structural tension.** Each seat is the uncompromising advocate for one value
   (Builder: shipping/simplicity · PE: correctness/severity · Researcher: soundness ·
   Tester: does-it-work-under-stress · Planner: vision/sequencing). Push back when another
   seat's proposal costs your value. Never win by attrition; never cave to keep the peace.
   Unresolved tension is surfaced by the Planner, not papered over.
2. **Adversarial seats judge blind.** The PE and any reviewer get the artifact + the
   question — NOT the builder's rationale. Pass the code and the criteria; withhold the
   "here's why it's fine."
3. **Verify before you assert.** A seat relying on another's claim for a load-bearing
   decision verifies it against the source itself — read the code, confirm the `file:line`.
4. **Evidence or nothing.** Name the specific artifact (a failing test, a log line, a
   measured number) that justifies any step. "I think / probably / let me just try" is the
   signal to go get evidence, not to act.
5. **Ground truth is external to the agents.** Correctness is decided by silt's real
   deterministic tiers (below), a blind external review, or the human — never by agents
   agreeing with each other.
6. **Scars survive pruning.** The Tester owns scar-counting and the third-time rule; a
   lesson is encoded as a gate/test before context is cleared.

## Research gate — what a seat may NOT decide alone (silt-specific)

Route to the Researcher for certification; do not build or assert on these:

- **Consensus-rule changes** (anything touching I1–I5, the block/validity rules,
  fork-choice, epochs, slashing).
- **Published-claim changes** (M0 / C1 / C2, the Sybil composition).
- **Economic-mechanism changes** (D-S7 durability economy, D-DEMAND, escrow / skim /
  bounty, the γ→1/N firewall).
- **Security parameters** a proof depends on (recall `build-process.md` — a durability knob
  was twice also a security parameter).

The Builder advises and shapes the question; the Researcher certifies; the human ratifies.

## Where things go

- **PE rulings** → `../silt-reviews/principle-engineer/` (this seat's own directory).
  NEVER the builder's tree (`docs/reviews/`). Always reply with the full path.
- **Research certifications** → `../silt-reviews/research/research-outcome/`.
- **Code** → the silt repo (Builder only).
- **Scars / run evidence** → the Tester's memory (the scar ledger), cited.
- **Live agent memory** → `.claude/agent-memory`, a symlink to the shared external store
  `~/.claude/silt-agent-memory`. It is gitignored and lives OUTSIDE git. NEVER `git add` it,
  commit it, or open a PR with it. Seat DEFINITIONS (`.claude/agents/`) stay tracked; live
  memory does not. New checkout or worktree: run `.claude/setup-agent-memory.sh` once to
  establish the symlink. (History: committing live memory caused per-pull conflicts, #636/#638.)

## The Tester's ground truth on silt

Correctness is a command result on silt's real tiers, in order:
`unit → consensus model-check → integration/sim → e2e/netem (integration/nat, flakynet) →
field (integration/cloudtest deep runs)`. Capture evidence FIRST (crash journals, logs)
before teardown — a non-reproducible failure is instrumented, not re-tried. A field run
confirms a fix; it never discovers a consensus invariant.

## Escalation to the human

- **Veto gate (STOP, get ratification):** any trade touching an immutable / M0, a scope
  change, or ratifying a research verdict.
- **Checkpoint (report + confirm direction):** material progress — a phase gate met, a
  milestone banked.
- Status to the human at least every two hours, and immediately on a veto-gate escalation.

## Rollout safety (while the orchestra is unproven on silt)

- **Read-only first.** Run the PE and Tester seats (read-only; the PE files to its own dir,
  the Tester only observes) before the Builder edits anything.
- **The human stays on every immutable-trade.** The orchestra augments silt's working
  process; it does not replace it, and it never decides an immutable.
- **Off the critical path first.** A full loop runs on a small, non-RC task before anything
  that matters.

## Source of truth — do not fork the seats

- The orchestra project (`../agent-orchestra/`) is the SOURCE OF TRUTH for the seat
  personas and the coordination rules. What lives here in `silt/.claude/` is a deployed
  SNAPSHOT.
- To change a seat, iterate in the orchestra sandbox, prove the change there, then re-copy
  into `silt/.claude/agents/`. **Never edit the seats in `silt/.claude/` directly** — that
  forks the two sets and the snapshot silently drifts from the source.
- The full deploy runbook (prerequisites, rollout steps) lives at
  `../agent-orchestra/deploy/silt/DEPLOY.md`, not here.
- When the design stabilizes and moves to the Claude Agent SDK, the personas port verbatim
  and this copy step goes away (see `../agent-orchestra/SDK-MIGRATION.md`).
