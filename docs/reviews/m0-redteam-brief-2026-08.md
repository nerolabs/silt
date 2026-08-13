# M0 red-team brief (2026-08) — break the systemic claim

> **You are an independent, external adversary.** You have no prior context on
> this project and you are not here to be fair to it. Your job is to **break the
> systemic M0 claim** — to find a concrete way the composition fails a promise it
> makes about itself. The unit of test is **not** "is this primitive
> Sybil-proof?" — a lone primitive failing that test is *expected* (Douceur), not
> an M0 failure. The unit of test is the systemic claim (**C1 + C2**) and the
> seven seams that hold it together. A finding is a path, with steps, to an
> outcome the project says is impossible. Vibes are not findings. **The verdict
> you produce is the M0 verdict** — silt is "held" iff an outside party denies all
> three failure modes below, so grade honestly.

## What you get — and only this

The **public surface**, exactly what a real attacker would have. Start from these
two; everything else is reachable from them:

- **the repository — <https://github.com/nerolabs/silt>** — clone it, read it,
  build it (`go build ./cmd/silt`), run it, run its tests;
- **the website — <https://silthq.com>** — and anything it links.

You get **nothing** from the builders — no design rationale beyond what is in the
repo, no "here's why it's safe," no list of what was already tried. Treat every
reassuring comment in the code and every "held" claim in the docs as a **claim to
falsify**, not a fact. A real attacker reads code comments and design docs; you
may too — but they are targets, and *you* do the finding.

**Read the genuine product docs — not the answer key.** The mechanism and the
falsifiable claims live in:

- `README.md` and `docs/local-test-network.md` — what silt is and how to stand it
  up;
- `docs/threat-model.md` — the builders' own honest account of the weak parts (it
  names weaknesses on purpose — confirm or exceed it);
- `docs/TENETS.md` **Part 0** — the mission (M0) and the exact C1/C2 definitions;
- `docs/design/m0.md` **§3** (the claim), **§7** (the seven seams), **§10** (the
  young→mature / anchor edge).

That is enough. You do not need, and should not rely on, any private builder
notes to find a real break — a break that only exists relative to the builders'
own scratch notes is not a break of the shipped product.

## The one promise to break (M0)

silt does **not** claim any single primitive is Sybil-proof — that claim is false
by theorem (Douceur), and breaking it is not new information. What silt claims is
**systemic**. In short (read `docs/design/m0.md` §3 for the exact statements):

> **C1 (no discount):** a strategy earning a fraction *q* of consensus standing
> must pay ≈ *q · C_honest* — **and C2 (no quiet capture):** the minimum colluding
> **operator** set required to reach quorum capture stays above a target *k* —
> **held in tension.**

where `C_honest = disk × address-diversity × time × served-demand`, a **product
of orthogonal, non-substitutable factors** (you cannot trade surplus disk for
missing address diversity, or buy your way past elapsed time). C1 is the fold:
**forging standing ≡ honestly providing that much real, served, sustained,
address-diverse storage.** C2 is the residue C1 cannot touch (an honest whale who
concentrates *real* standing).

> **Shipped subset — the served-demand (B) axis is NEUTRAL, by design.**
> `D×A×T×B` is the *target* composition; in the shipped M0 subset served-demand
> confers **zero** consensus standing — it is a neutral observable, severed from
> standing by a firewall the docs call γ→1/N. So today standing ≈ `D×A×T`. The
> demand mechanism exists and is blind/unlinkable, but a receipt — forged,
> self-dealt, or washed — buys **nothing** that matters to consensus. Your
> demand/wash attacks are therefore a test of that **firewall**: find any path by
> which served/witnessed demand moves consensus standing. "Demand doesn't move
> standing" is the design's claim — break it, or confirm it held.

**A break is one of three things:** (i) a strategy that earns
consensus-controlling standing for **less than *q · C_honest***; (ii) a strategy
that **concentrates past capture** (violates C2); or (iii) a strategy that
**breaks a §7 seam**. The sharpest targets are the seams — that is where composed
systems break.

## The three deny-able failure modes (state your verdict on each)

M0 is *held* iff you deny all three. In outcome terms you assert — not mechanism:

1. **Privacy — publish→identity linkage.** *Break =* given the public ledger,
   issuer/registry logs, and packet traces you could realistically capture, you
   link a target published root to the durable standing key (or the operator)
   behind it, **better than chance within the epoch anonymity set**. *Held =* you
   cannot do better than chance at the stated anonymity-set size.
2. **Accountability — identity-level or global takedown.** *Break =* you make a
   specific root **globally** unfetchable, or effect a takedown that acts on an
   **identity** rather than a content hash, or flip a single switch that removes
   content network-wide without each operator's independent choice. *Held =* every
   takedown is per-operator, content-scoped, reversible, and no lever removes
   content globally.
3. **Sybil-resistance — standing at a discount, or quiet capture (C1 + C2).**
   *Break =* you obtain a fraction *q* of consensus/denylist standing for
   materially **less than *q · C_honest*** (¬C1), **or** you concentrate to quorum
   capture while staying under the operator-diversity radar (¬C2), **or** you break
   any §7 seam that holds the composition together. *Held =* no such strategy
   exists at the network's declared parameters.

> **"Held," not "closed," is a PASS on some seams — and the spec says so.** A seam
> *held in tension* (bounded cost, an openly documented residual) is a **pass**
> under M0's own definition. The failure mode is a seam **silently assumed
> closed** — a claim the docs make as if settled that in fact admits a cheap
> strategy. Read §7 for which seams the builders themselves flag as *held, not
> closed* (C2, cold-start, and the real-vs-wash-demand ratio are named). Re-deriving
> one of those is a *useful confirmation*; your prize is a break the docs do **not**
> already own, or a "closed" claim you show is actually open.

## Scope — attack the right thing (M0 vs. forward tracks)

**In scope for the M0 verdict:**

- The C1 + C2 composition and the seven §7 seams (below).
- The consensus / fork-choice / finality path (objective, bond-weighted;
  §10) and its young→mature anchor handoff.
- Content-scoped, per-operator takedown (no global switch).
- Publisher unlinkability of the *publish act* from durable standing.

**Out of scope for the M0 verdict (forward tracks — do not grade M0 on these):**

- **Full metadata-layer access privacy (H8).** silt's privacy corner is a
  *refusal to surveil* (absolute — silt builds no who-fetched-what log) **plus
  access-unobservability held in tension** at the metadata layer, explicitly
  bounded by the anonymity trilemma and anonymity-set size. A global passive
  network observer defeating blob-layer unobservability is **not** an M0 break —
  the spec never claims that guarantee. The in-scope privacy target is the
  narrower *publish→identity linkage* above.
- **Pluralistic-takedown formal non-globality metric (H9).** The transparency-log
  non-globality guarantee is a forward construction; the M0 accountability target
  is the concrete "no global / no identity-level takedown" outcome above.
- **The served-demand (B) axis moving standing.** It is firewalled to zero today
  (see the shipped-subset note). Show the firewall leaks and that *is* a break;
  attacks that assume B already feeds standing are attacking an unbuilt target.

If in doubt whether something is in scope, ask: *does it let me deny one of the
three failure modes?* If yes, it counts.

## The seven seams (§7) — suggested attack directions, not solutions

These are named at the level a public reader can see them. They are *where to
look*, not *how it's solved* — the mechanism and pass condition are in
`docs/design/m0.md` §7 for you to falsify.

1. **Re-pricing, not prevention (the wealth residue) → C2.** C1 does nothing
   against an actor who *honestly* provides a large fraction of real storage.
   Does the concentration metric keep the minimum colluding **operator** set above
   *k* under adversarially-skewed measurement? Can you skew the inputs to the
   concentration count?
2. **Cold-start / scaffolding-capture window.** A young network bootstraps from
   explicit, time-boxed **anchor** validators (training wheels) that shed on
   measured decentralization. Can you capture the young, anchor-scaffolded regime
   **before it matures and sheds** — or exploit the maturity latch or the
   de-maturation path (§10) to restore a standing dependency on the anchors after
   they should be gone?
3. **Self-dealing vs. real demand (wash).** The anti-wash argument rests on real
   demand outweighing fabricated demand. At realistic parameters, can you
   manufacture wash demand that dominates real demand? (Remember the firewall:
   first show the receipt buys standing at all.)
4. **Privacy ↔ attribution linkage leak.** If the blind demand-receipt or the
   publish-token flow is imperfect, either privacy leaks or demand is forgeable.
   Can a colluding validator subset **de-anonymize** a fetcher or a publisher, or
   **mint receipts without served bytes**?
5. **Operator clustering is heuristic.** Address-diversity counts address groups;
   "5 operators" vs. "1 operator, 5 subnets" is a heuristic call. Can you present
   real cloud/AS diversity that **evades operator-clustering** and buys diversity
   credit you didn't earn?
6. **Time-axis gaming (on-off / camouflage).** Standing accrues over time. Can you
   **bank reputation then defect intermittently**, tuned to the audit/decay
   window, to hold standing you are not currently backing with real work?
7. **New liveness / griefing surfaces.** The composed and consensus rules create
   new ways to **stall** — withhold, refuse to attest, equivocate, propose
   off-head, or grief the finality gate. Can a minority halt or oscillate
   liveness, or get **two competing histories to both stand**?

## How to stand up a target network

Everything you need to reproduce a break is a laptop and Docker:

- **`docs/local-test-network.md`** — the README's on-ramp: boot the whole swarm
  in one command, then stand up a real multi-node network on one machine, publish,
  and watch content survive a node death.
- **`docs/user-seam.md`** — the complete set of operations silt exposes, by role:
  your attack-surface map. Subvert any operation to violate a promise above.
- **`docs/test-topologies.md`** — how to build the shapes you'll need (a validator
  swarm, real NATs, a partition).
- **`examples/`** — one-command playbooks that already boot the pieces (publish/
  fetch, earned standing, convergence-under-fault-and-restart, takedown). Extend
  these rather than starting cold.

Two helpers instrument an attack without wasting your budget: `silt id` prints a
peer's NodeID without launching it, and `silt chain-status -store DIR` prints a
replica's head height + head hash — how you tell whether two histories both stand.
Several `silt sim run <name>` models are **pre-wired adversary experiments with
the mechanism's denial already encoded** — extend them: `bondstanding` (a Sybil
quorum is denied), `consensus` (a zero-reputation proposer is refused), `audit`
(a liar-prover is caught), `takedown` (a denied root is blocked per-operator).
`go test ./sim/ -run <name> -v` shows how each is driven. **Don't burn your budget
on setup** — the denial you have to *break* is already staged for you in several
of these.

## Rules of engagement

- **External and blind.** You find the breaks yourself, from the public surface
  only. Do not rely on private builder notes; a finding must reproduce against the
  shipped repo.
- **Concrete or it didn't happen.** Give the steps, the state, and the outcome —
  ideally a failing test, a script, or a precise sequence a builder can reproduce.
  A denial, likewise, carries most when it's a *failing attack* — the mechanism
  visibly refusing — the way `bondstanding` / `consensus` / `audit` already encode
  theirs, not prose alone.
- **Attack the composition, not the isolated primitives.** The primitives are
  adopted from the literature; a lone one failing a standalone Sybil-proof test is
  *expected* (Douceur). The break that matters is a cheap shortcut on one axis
  **not** caught by another axis's check, a capture that concentrates past C2, or a
  §7 seam.
- **Stage along the axes:** (i) per-axis reuse attacks (one disk → many ids; one
  IP → many nodes; instant standing; self-dealt content) — each should be denied by
  its part; (ii) cross-axis **seam** attacks (§7) — the real prize; (iii)
  concentration/shed attacks against C2; (iv) cold-start capture against the young
  regime.
- **State your assumptions.** If a break needs a capability (a global passive
  observer, factoring the VDF modulus, breaking Ed25519), say so — an attack that
  needs a broken primitive is not a break of *this* design.
- **Scriptable → we'll keep it.** If your attack — or your denial — runs as a
  script or a `go test`, hand it over; strong ones become permanent adversarial
  regression tests.

## What to hand back

A single markdown file. Lead with a **verdict table** — one row per unit of test:
the **three failure modes** (privacy linkage, global/identity takedown, C1
discount, C2 quiet capture) **and** each of the **seven §7 seams** — marked
`HELD-IN-TENSION` / `BROKEN` / `SILENTLY-ASSUMED-CLOSED` / `UNCERTAIN`. Then one
section per finding:

```
### <short title>
- unit:          privacy | accountability | C1 | C2 | seam-1..7 (name it)
- adversary:     which mindset (Sybil farm, colluding minority, censor,
                 network observer, wash-trader, liar-prover, equivocator, ...)
- severity:      critical | high | medium | low
- breaks:        privacy-linkage | global/identity-takedown |
                 C1 (cheaper than q·C_honest) | C2 (concentration past capture) |
                 which §7 seam | "hardening"
- confidence:    high | medium | low
- attack:        the concrete steps / state / script
- outcome:       what the attacker achieves that the claim forbids
- suggested fix: optional
```

**The verdict you produce is the M0 verdict.** If you cannot break a unit after a
real effort, say so plainly and give the argument for **why** it held (or why it
holds only in tension) — that is a denial, and under M0's own definition it is
exactly as valuable as a break. A seam held *in tension* is a PASS; a seam
*silently assumed closed* is the failure mode. M0 ships held-at-stated-parameters,
or it does not ship.
