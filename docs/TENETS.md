# silt — Tenets

> Status: **canon.** These tenets are the supreme, mostly-immutable statement of
> what silt *is* and what "good" looks like. Changing an **Immutable** (Part 0 +
> the frozen-format principle + the eight build-immutables in Part IX) requires
> deliberate, reviewed consensus and is close to redefining the project.
> **Tenets** are canon too, amendable with reviewed consensus and evidence.
> **Evolving** parameters are expected to change as we learn.
>
> **These tenets are principles, not the build.** They state what "good" looks
> like as an *outcome* — what each principle *represents* — abstracted away from
> any mechanism, ship-status, or product state. The build looks *up* to the
> tenets for guidance; the tenets never look *down* at the build, and they do not
> change when the product ships. Mechanism, current state, and the dated history
> of how these tenets were reached live in the companions: the mission spec in
> [`design/m0.md`](design/m0.md), the decisions in [`decisions.md`](decisions.md),
> the networking discipline in [`network-durability.md`](network-durability.md),
> the build discipline in [`build-process.md`](build-process.md), the update
> mechanism in [`network-protection.md`](network-protection.md), the order of work
> in [`ROADMAP.md`](../ROADMAP.md), and the amendment log in
> [`tenets-history.md`](tenets-history.md).
>
> **The finished picture these tenets define lives in [`VISION.md`](VISION.md)** —
> silt as it *is* when done, told through the people it serves. The tenets are the
> stance and the rules; the vision is the destination they point at.
>
> Format: each tenet is a **stance**, stated plainly. Personas are defined by
> their **desired outcomes** (what "good" looks like from their seat) and the
> **promise** silt makes to them. Where two personas' outcomes collide, the tenet
> is our **stance on the tradeoff**.

---

## The Immutable Register

The immutables, one line each, with a pointer to where each is elaborated below.
Amending any of these is close to redefining the project.

| # | Immutable | In one line | Elaborated |
|---|---|---|---|
| **M0** | The mission | *Hold* the privacy × accountability × Sybil trilemma — refuse to trade any corner away. | Part 0 |
| **1** | Content-blind by construction | Hosts store ciphertext they cannot read or choose. | B4, Part IX |
| **2** | The bytes are the truth | Content-addressed, re-verified, bit-perfect or an explicit failure. | S1/B3, Part IX |
| **3** | No *permanent* center | Nothing permanently load-bearing; bootstrap scaffolding is explicit, time-boxed, sheds. | T1/S4, Part IX |
| **4** | Access is unsurveilled | silt refuses to surveil, and pursues access-privacy to the trilemma's limit. | Don't #3, Part IX |
| **5** | No silent or global censorship | Takedown is transparent, consensual, plural — never one switch. | Don't #2, Part IX |
| **6** | Core carries zero meaning, forever | Core resolves hashes, never names (the Aslan boundary). | T3, Part IX |
| **F** | Frozen consensus formats | A shipped, ratified consensus format is immutable — amend only via a NEW ERA. | Part IX |
| **B1–B8** | Build-immutables | How we build, so the project does not silently rot as it grows. | Part IX |

---

## Part 0 — Why silt exists (the mission-immutable)

**M0 — silt exists to *hold* the privacy × accountability × Sybil trilemma —
to refuse to trade any corner away.** Three properties every prior system in
this space has been forced to trade off against each other, held together
without sacrificing one for the other two:

- **Privacy** — publishing is *unlinkable* to a durable identity; and silt
  *refuses to surveil* who fetches what, pursuing access-privacy to the
  metadata-layer limit the anonymity trilemma allows (not an absolute blob-layer
  guarantee).
- **Accountability** — genuinely harmful content can be removed, curators are
  themselves accountable, and takedown is **pluralistic** — never a global
  switch.
- **Sybil-resistance** — standing cannot be cheaply forged; no actor can spin up
  identities to capture consensus, wash reputation, or flood a denylist.

The incumbents each pick two corners and surrender the third. **silt's reason to
exist is to hold all three** — and **abandoning any corner is abandoning the
project.**

**This is "hold," not "resolve."** It is not a claim to have *solved* a research
problem all at once; it is the refusal to trade a corner away, and a design in
which the corners *co-mature* rather than arriving finished:

- **Privacy is architectural** — private-by-default encryption, opaque
  ciphertext, a refuse-to-surveil access posture, unlinkable publish tokens.
- **Accountability is content-level and reactive** — takedown acts on a *hash*,
  pluralistically and after the fact; never identity-level, never pre-emptive,
  never a global switch.
- **Sybil-resistance is the corner that bootstraps.** It is *weakest on a young
  network* — a small network is cheapest to flood — and it *strengthens as real,
  sustained work accrues*. During bootstrap, explicit, time-boxed scaffolding
  may anchor the network, shedding on measured decentralization via a **one-way
  latch** that, once tripped, never re-arms — so a later drop in decentralization
  cannot hand the bootstrap anchors permanent power (immutable #3: *no permanent
  center*). A fresh node cold-syncs from a recent **checkpoint**, because silt is
  weakly subjective — like any system whose history a newcomer cannot re-derive
  from work alone, and so is exposed to the **costless-simulation / long-range**
  attack class that a checkpoint pins against. This is the live edge where the
  novel contribution concentrates.

**M0 is falsifiable, not a slogan.** It is *held* if and only if an adversarial
red-team suite, written by a party *other than the author* (see B8), **denies all
three failure modes**: publish→identity linkage (privacy), identity-level or
global takedown (accountability), and **Sybil-farm standing *at a discount*
(Sybil-resistance)**. The Sybil mode is a *systemic* property of the composed
system, not a property of any single primitive. "Did we hold M0?" therefore has
a yes/no answer an outsider can check — not a victory declared by the builder.
The precise composition claim, its inequalities, and the suite that certifies it
are specified in [`design/m0.md`](design/m0.md).

**A release candidate has these attributes.** The abstract bar a finished,
shippable silt must meet:

- Publishing is cryptographically unlinkable to the standing it earns.
- Access carries no surveillance mechanism, to the trilemma's metadata-layer
  limit.
- Takedown has real teeth on harmful content yet no global switch and no
  identity-level lever.
- Forging *N* standings costs *N* times the real, non-substitutable work an
  honest provider pays — cheap for one honest node, ruinous for a Sybil farm,
  with no coin and no capital lockup.
- No standing dependency on any one node; bootstrap scaffolding has shed.
- All of the above is proven by an *external* adversary, not asserted by the
  builder.

**The bet, stated without a slogan.** Two of the three edges are dissolved by
architecture: privacy-vs-accountability dissolves because we act on **content,
not identity** (deny a *hash*; hosts are content-blind and carry no liability for
what they cannot read). The live edge — where the novel contribution concentrates
— is **privacy vs. Sybil**: we **decouple the cost of *creating* an identity from
the cost of *having standing***. Identity is free and pseudonymous; *influence*
costs sustained, challenged, real work; and the publishing act stays
cryptographically **unlinkable** from that bonded identity. The load-bearing,
field-defining claim is therefore:

> **Token-less, work-backed, identity-bound reputation that publishing stays
> cryptographically unlinkable from** — cheap for one honest node, ruinous for a
> Sybil farm, with no coin and no capital lockup.

**Ruin comes from composition, not a lone primitive.** No single mechanism can
*prevent* Sybils under free minting + no permanent center; the guarantee lives in
the *system*. Each part denies one economy of scale a Sybil relies on. The
composition is *designed so that* every shortcut on one axis trips another axis's
check, and forging standing *would become* indistinguishable from honest
provision. Sybil-resistance is therefore *re-pricing + concentration-bounding*,
not prevention; the composition faces permanent Douceur limits it can bound but
never dissolve:

- **The honest whale** — an actor who genuinely provides that much real,
  non-substitutable work — is *bounded* by the concentration metric, not
  eliminated.
- **Bare age is pre-farmable** — standing cannot accrue from age alone; a bare
  age gate is farmable ahead of time, so the only sound form of the time axis is
  continuous re-proof, never an acquisition-time age credit.
- **Wash is re-priced, not proven away** — a demand receipt can be made
  unforgeable and fetcher-unlinkable, but *unlinkable is not authenticated*:
  self-dealt demand is a Douceur limit, so the demand axis is priced by
  *cost-to-wash*, never proven wash-free.

The mechanism of the interlock, and which axes are wired at any moment, live in
[`design/m0.md`](design/m0.md); those are build state, and build state does not
belong in a tenet.

M0 is the one mechanism deliberately pulled into first-release scope by
definition — it *is* the mission, so it must ship **specified and adversarially
proven**, not asserted. Everything below is either a corner made structural, or
the discipline that keeps the composition honest.

---

## Part I — What silt is

**T0 — Two planes, one substrate.** silt is *BitTorrent + BitCoin*: a **storage
plane** (content-addressed, erasure-coded, peer-served chunks with NAT-traversal)
and a **trust plane** (consensus-secured registry, reputation, and revocation).
The tagline is a fidelity note, not a lineage: the storage plane behaves like
content-addressed swarms (dedup by hash, no trusted server); the trust plane is a
**BFT / proof-of-stake-class** consensus plus a **transparency log** — *not*
Nakamoto proof-of-work. The storage plane stands alone and is the default; the
trust plane is opt-in and secures governance. Neither is the product — silt is the
**substrate** other things are built on, and the trust plane is where M0 lives.

**T1 — Capabilities, not infrastructure.** Every role (store, relay, registry,
validate, caretake) is a *capability any node can offer*, never a special node
baked into the binary. No node is *permanently* load-bearing; none is
irreplaceable. A young network may lean on explicit, time-boxed scaffolding, but
that scaffolding is designed to shed on measured decentralization — the forbidden
thing is a *standing* dependency on any one node, not the honest scaffolding that
retires itself (immutable #3).

**T2 — The link is the primitive.** A `silt:` link is the whole product surface
for a user: content-addressed identity + the key to read it. Durability,
placement, and repair are the network's job, not the holder's.

**T3 — The naming boundary is immutable (Aslan).** Core resolves *hashes*, never
*names*. Turning an opaque root into human meaning (names, descriptions,
moderation, curation) is a separate resolver layer ("Aslan"), and silt core
**carries zero meaning, forever**. This is a liability firewall as much as an
architecture: the moment core learned to resolve names it would inherit every
takedown, copyright, and safety obligation it exists to shed. Any change that
teaches core about meaning is a top-severity regression. A link guarantees the
*bytes it names*; trusting a *name* is trusting whatever resolver you asked —
like trusting a DNS provider. "File poisoning" (a name resolving to hostile
content) is therefore a resolver-layer concern by design.

**silt is use-agnostic.** Core carries zero meaning, and silt takes zero position
on *use*. We do not enumerate, endorse, or concern ourselves with what flows
through it — archival, a library of record, or anything else users choose is
unenumerated and not silt's business. silt's only ambition is to be the most
trusted, private, secure, scalable, and efficient DFS ever built, chosen for its
feature set. **"Aslan" names any client or application built *on top of* silt** —
the application layer, expected to be richly diverse, is where use lives; silt
below it neither knows nor cares.

---

## Part II — How we build

**B1 — Hexagonal core.** Domain logic is pure and portable; all I/O (disk,
network, clock) lives in adapters at the edge. The seam is where a real
implementation swaps in for a simulated one with zero core changes.

**B2 — Lock-free, single-loop core.** Node logic runs on one serialized loop: no
locks, no goroutines in the core. This is what makes the simulator deterministic
and the real network inherit the same guarantee.

**B3 — Content-addressed, and never trusted blindly.** Identity is the hash of
the bytes. Every read re-verifies against its hash — *disks lie, networks lie*.
Convergent encryption makes identical content converge to identical identity
(dedup), and the publisher's identity is metadata, not part of what content *is*.

**B4 — Privacy by construction.** Hosts store opaque ciphertext they cannot read
and did not choose by content. This is both a user promise (privacy) and an
operator promise (no liability for the unknowable).

**B5 — Legibility.** Code reads like the code around it; a behavior narrates
itself, so the field can see the normal path, not just failures. We optimize for
the next reader.

**B6 — Reactive, not eager.** Data moves when it must (repair on loss, fan-out on
heat), not on a schedule. Idle is cheap; the system quiesces.

**B7 — Trust but verify; no optimistic operations.** B3 generalized from reads to
*every* operation: an operation is not "done" until its effect is **confirmed**,
never assumed. A write is durable only when read back or acknowledged by ≥k
independent parties; a placement is complete only when providers confirm they
hold it; a publish returns a link only once the content is provably retrievable.
We take the **cynical** default — disks, networks, peers, and *our own prior
steps* lie until proven otherwise — so an optimistic ack is a **defect, not an
optimization**. Where verification genuinely conflicts with another tenet (e.g.
latency/cost vs. S6, or eagerness vs. B6), the exception is made **explicit and
discussed**, never taken silently.

**B8 — Best components, novel composition.** We never reinvent a primitive —
cryptography, transport, codec, hash. We adopt the single strongest *proven* one
and treat rolling our own as the amateur tell it is. **We also reject a primitive
that fails our bar** — an unaudited, archived, or unproven construction is not
adoptable *even when it would be convenient*, because shipping an unproven
primitive to the audience we most need to convince is worse than a boring me-too
choice. Novelty is reserved for the **composition and incentive design**, where
the hard problems (M0) actually live — and that novelty must be **specified and
adversarially proven** (a spec a skeptic can read and a red-team suite they can't
break), never hand-waved. **The adversary must be external.** Self-marked
homework is not adversarial proof: the attacks that certify a novel composition
must be written by a party *other than its author* — an independent audit, a
public bounty, or a separate red-team. **The suite tests the composition, not the
parts.** Because the novelty *is* the composition, the certifying attacks target
the system-level claim, not each primitive in isolation — a primitive that fails
a standalone "is-it-Sybil-proof?" test is *expected*, not a defect. Boring parts;
a novel car. Novel-and-unproven is worse than me-too.

---

## Part III — What a successful outcome is (the bar)

**S1 — Integrity is non-negotiable; availability is engineered.** Bytes are
either bit-perfect or an explicit failure — **never silently wrong**. Zero
corruption is the floor, not an achievement. Availability, by contrast, is a
probability we raise with redundancy, placement, and repair.

**S2 — Content outlives any node.** Durability comes from erasure coding ×
cross-node replication × failure-domain spread × an ongoing repair loop that
outruns failure — not from any node being reliable.

**S3 — No silent-loss shapes.** Any operation that can't complete must fail
*visibly and recoverably*. A publish that half-succeeds and strands content with
no retrievable link is a bug of the highest severity.

**S4 — No single point of failure or control.** If one machine dying — or one
operator deciding — can take content down globally, we have failed a core
promise.

**S5 — Honest observability.** An operator can see what is *true*: real peers vs.
ghosts, actual redundancy, caretaker coverage, cache effectiveness. We do not
ship dashboards that flatter.

**S6 — Cheap to participate.** The bar for running *any* role is low enough that
a hobbyist can. Cost that scales unbounded with network activity is a design
defect, not a fact of life. *(This is also the constraint that makes M0's Sybil
corner hard: cheap for the honest, ruinous for the liar — without a capital
lockup that would price out the hobbyist.)*

**S7 — Durability must pay for itself.** The repair loop that keeps content alive
under churn must be **funded in equilibrium, not run on charity.** Reputation
buys *presence*; it does not, by itself, buy *sustained repair bandwidth* when
nodes are leaving faster than they arrive. An economy where storing and repairing
is a net cost with no matching reward decays to zero the moment altruism runs out
— this is the wound that killed the charity-funded DFS generation earlier. So
durability is not "solved" until the caretaker who repairs a stripe they neither
own nor can read is **paid by the demand that content serves**. The *design goal*
is one ledger: the served, sustained real content a node holds would be both what
earns its repair reward *and* what backs its consensus standing (M0). Whether
that fusion is safe to make turns on a real sealing problem, owned in
[`design/m0.md`](design/m0.md); until it is solved, durability budget and Sybil
budget stay two separate ledgers, and treating them as fused would re-open the
exact Sybil break the separation exists to prevent.

**The funding stance.** No *speculative external* token. silt's internal credit
unit is made durable, escrowable, and forwardable in time; durability is funded
by a per-object reserve that auto-skims a fraction of serving revenue back into
that object's reserve, paying repair bounties scaled by how under-replicated a
stripe is. These credits fund *durability* and confer *no* consensus standing —
standing stays work-backed and coin-free. A repair is paid only when its
**correctness** and an identity-bound **retrievability** both verify, checked by
a quorum with no coordinator, and an attributable false claim slashes the bond —
a composition of proven primitives, no new invention required for the base case.
The construction lives in [`decisions.md`](decisions.md) (D-S7).

**Durability is finite-but-renewable, not "perpetual."** Perpetual cold-data
solvency is an endowment identity **priced in the network's own credit unit**,
not in fiat: it holds only while the credit-denominated cost of storage keeps
declining, which modern hardware evidence no longer guarantees. silt therefore
**adopts the endowment schema and rejects the perpetual promise**: it funds a
horizon, auto-skims to extend it, publishes the funded horizon per object, and
asks for re-endowment before expiry. "Perpetual" is a claim silt *earns only if
the measured credit-denominated decline stays positive*, never an architectural
promise; that decline is the one number to instrument.

If we cannot state the equilibrium in which repair funds itself, we have not
decided durability — and it is durability that decides whether files exist in
three years, not whether they exist today.

---

## Part IV — How we test

**V1 — Sim first, field second, both required.** Deterministic simulation proves
logic (including Byzantine and churn cases, reproducibly); a multi-machine field
test proves it against real NAT, real WAN, real timeouts. A behavior is not
"done" until it has cleared *both*. The discipline, stated as a pipeline: **unit
→ consensus model-check → integration → e2e**, with the field run last. A field
run *confirms* a fix; it never *discovers* a consensus invariant.

**V2 — Conformance over trust.** Every implementation of a seam passes one shared
conformance suite, so "however many implementations, one contract."

**V3 — Test the adversary.** We write the attacker's desired outcome as a test
that must fail for them: forgery, tamper, equivocation, freeload, Sybil,
censorship. Security is validated by denial, not assertion. For M0's novel
mechanisms this is the *primary* proof: the red-team suite is the deliverable,
and the M0 verdict is exactly its result (Part 0). The suite that *certifies* M0
must be written by an **external** party (audit / bounty / independent red-team,
per B8) — we may write our own attacks to develop against, but the proof that
ships is the one an outsider could not break.

**V4 — Evidence, not vibes.** A change clears the success bar (Part III) with
observed evidence — a checksum match, a survived kill, a green Byzantine suite —
reported faithfully, including what was skipped.

**V5 — A bug fixed once stays fixed, and breaks are caught locally.** Every
defect we discover ships in the *same change* as a test that **fails before the
fix and passes after**, added at the tier(s) where the bug actually lives (unit
for pure logic; integration/sim for components-in-a-peer and multi-node; e2e for
real daemons over real sockets). A fix without its regression test is not done.
The regression must be catchable by the **local** suite so a re-break surfaces on
the developer's machine in seconds, not at merge or in CI. CI is the backstop; it
is never the *first* place a regression is allowed to appear. This is how a
growing codebase stays fast to change.

---

## Part V — How we release

**R1 — Gated by proof.** A release candidate requires the plane's core promises
*field-proven*: cross-network publish/fetch, scaling under load, crash recovery,
and — because the trust plane is a pillar — the M0 mechanisms proven
**multi-machine**, not sim-or-single-host only. Sim-proven-only work is labeled
as such and is not a release gate on its own.

**R2 — Canon tracks behavior.** Docs and these tenets are updated in the same
change that alters behavior. No silent drift between what the system does and what
we say it does.

**R3 — Throwaway stays throwaway.** Test/dev infrastructure (a dev relay, a demo
registry) is never allowed to quietly become load-bearing.

**R4 — Operator-autonomous updates, security-gated.** Operators control their own
software; the network **never silently auto-updates**. Only security uses
graduated enforcement, and the maintainers set the tier. The gate of last resort
— where patched peers refuse older versions — can halt the network, so **no
single key may declare it**: the signing threshold *is* the safety property.
Enforcement is recallable and clocked on observation of a signed advisory, never
on manipulable system time. The mechanism (criticality tiers, thresholds, the
version-floor advisory) lives in [`network-protection.md`](network-protection.md).

---

## Part VI — The don'ts (bright lines)

1. **Never force a registry or relay operator to also store or serve content.**
2. **Never enable silent or global censorship.** Takedown is transparent,
   consensual, and pluralistic — never a single kill switch.
3. **Never surveil access.** silt builds no mechanism to observe or link
   who-fetches-what, and pursues access-privacy to the anonymity trilemma's
   metadata-layer limit — a goal held in tension (bounded by that wall and by
   anonymity-set size), not an absolute blob-layer guarantee. The refusal to
   build surveillance is absolute.
4. **Never a silent-loss failure shape** (see S3).
5. **Never bake in a special or central node** (see T1).
6. **Never trust bytes without verifying** (see B3).
7. **Never let the economy reward useless or harmful work** — reward tracks value
   delivered to the layer above.
8. **Never build a decryption backdoor.** Core holds no capability to decrypt
   stored content, and silt ships no threshold- or quorum-decryption of it.
   Accountable disclosure, if it ever exists, is an Aslan-layer choice made by
   parties who can already read — never a core capability (reinforces immutable
   #1 content-blind, T3).

---

## Part VII — Per-persona tenets (outcomes → promise)

### A. Value personas

**1. Content consumers (clients — "Aslan users").**
- **Outcomes:** available when asked; authentic; private access; runs no
  infrastructure; a given link keeps working.
- **Promise:** a link is enough — retrieval, verification, and durability are the
  network's job, and silt never surveils what you fetch (access-privacy is
  pursued to the metadata-layer limit the trilemma allows).

**2. Publishers / creators.**
- **Outcomes:** content stays available as long as intended, and they can *tell
  whether it will*; access control + revocation; integrity + attribution; cheap
  publishing; publish without being deanonymized; no silent disappearance.
- **Promise:** you can make content durable and know its durability; you can
  revoke; no one alters it under your name; your publish is unlinkable to your
  standing (M0); taking it down is visible, never silent.

**3. Link recipients (audience).**
- **Outcomes:** the link resolves; bytes are the intended ones; private stays
  private.
- **Promise:** a valid link is a cryptographic guarantee of *what* you'll get.

### B. Infrastructure personas

**4. Storage-node operators.**
- **Outcomes:** fair, predictable reward for disk + bandwidth; legal safety (host
  unknowable ciphertext; refuse lists they choose); free to come and go;
  reputation that accrues and ports.
- **Promise:** contribute capacity, earn proportional standing/credit, carry no
  liability for what you cannot inspect, and never be punished for churn.

**5. Registry operators.**
- **Outcomes:** a public good that *doesn't cost them* (minimal disk/bandwidth);
  never forced to store/serve; not a single point of failure or liability.
- **Promise:** running rendezvous is cheap enough to be near-free; the registry
  is a cache/accelerator over the DHT, so losing one costs latency, not
  availability.

**6. Relay operators.**
- **Outcomes:** content-blind (no liability); bounded, capped cost; rewarded/
  reputed for reachability.
- **Promise:** you forward ciphertext you can't read, within a cap you set, and
  can't be turned into anyone's free CDN.

**7. Caretakers (durability providers).**
- **Outcomes:** rewarded for keeping content alive; can repair content they
  neither own nor can decrypt; clear signal of what needs care and whether
  they're succeeding.
- **Promise:** repair rights are delegable and auditable; durability is a
  *service* a publisher can buy and a caretaker can sell.

### C. Trust & governance personas

**8. Validators / consensus participants.**
- **Outcomes:** honest work grows their standing; Byzantine peers can't cheat
  them; influence is earned, not bought raw; low overhead.
- **Promise:** consensus is deterministic and reputation-gated; standing costs
  sustained, challenged real work (M0), not a stake or a coin; equivocation and
  forgery are detected and cost the actor.

**9. Suppression / takedown / kill-list curators.**
- **Outcomes:** their lists are honored by operators who *choose* to trust them
  (voluntary, pluralistic); real teeth on genuinely harmful content; curation is
  trusted and auditable, with a false entry visible and reputation-costly;
  funded/rewarded for a public-safety service.
- **Promise:** you can publish a takedown list with *effect proportional to who
  trusts you* — never a global switch; accuracy is rewarded, over-reach is
  penalized, and every honored suppression is auditable.
- **Stance:** takedown is **consent-based and plural** — many competing lists,
  operators subscribe to those matching their jurisdiction and values. We build
  the mechanism for *transparent, chosen* suppression, and refuse to build a
  mechanism for *imposed, silent* suppression.

**10. Legal authorities / regulators.**
- **Outcomes:** a real path to remove illegal content within a jurisdiction, with
  accountability.
- **Promise:** jurisdiction-scoped takedown via operators and curators who honor
  it — and *no* lever for global censorship or user surveillance.

### D. Builder personas

**11. Developers / integrators.**
- **Outcomes:** stable, documented APIs/SDKs; the daemon-as-local-service is
  trivial to embed; the link is a clean primitive; predictable, composable
  behavior; deliver utility without running infrastructure.
- **Promise:** silt is a dependency you can reason about — documented seams,
  stable link format, a local API.

**12. Application / utility operators — the infra↔utility bridge.**
- **Outcomes:** rely on the substrate's guarantees (availability, integrity,
  privacy) to build higher-order products, reasoning clearly about what silt
  promises vs. what they must add.
- **Promise:** the substrate's guarantees are explicit and testable, so you know
  exactly which load you're carrying and which you're inheriting.

### E. Meta / systemic

**13. The commons (the network itself).**
- **Outcomes:** no single point of control/failure; self-healing under churn;
  economics where reward tracks useful work; resistant to Sybil, pollution, and
  DoS.

**14. Adversaries (anti-personas — outcomes we DENY).**
- Censor, surveil, forge, free-ride, Sybil the DHT, equivocate in consensus,
  exhaust resources, poison routing. **Each adversary outcome, inverted, is a
  security tenet.** We measure success by how impossible *or unrewarding* we make
  each.

---

## Part VIII — The value loop (how outcomes interlock)

The economy closes as a chain of served guarantees:

```
operators + relays + registries + caretakers  →  a substrate with guarantees
developers + application operators             →  turn guarantees into utility
publishers + consumers                         →  the demand that funds the loop
validators + curators + authorities            →  keep the loop honest and lawful
```

**The through-line:**
- Every persona's incentive is **satisfied by serving the persona above it** —
  reward flows down the stack from the value consumed at the top.
- **No persona's good may require another's harm.**
- Where outcomes genuinely tension, the tenet is our **explicit stance on the
  tradeoff**, held in the open:

| Tension | Our stance |
|---|---|
| Publisher permanence vs. takedown | Permanence by default; takedown only via *chosen, transparent* lists — never silent or global. |
| Consumer privacy vs. legal accountability | The *refusal to surveil* is absolute (silt builds no who-read-what log); access-*unobservability* is a metadata-layer goal held in tension, bounded by the anonymity trilemma — not a blob-layer absolute. Accountability acts on *content* (jurisdiction-scoped), never on who read it. |
| Privacy vs. Sybil-resistance (the M0 edge) | Identity is free and private; *standing* costs sustained challenged work; publishing stays unlinkable from standing. We buy Sybil-resistance with *proven work*, never with a coin, a stake, or deanonymization. |
| Operator freedom vs. availability | Operators are free to leave; availability is the network's job (repair), not a shackle on any operator. |
| Decentralization vs. usability | Rendezvous may be centralized *for convenience* but never *load-bearing*; the decentralized path must always exist. |
| Openness vs. abuse resistance | Open to participate, but reward is gated on delivered value and reputation, so abuse doesn't pay. |
| Reward concentration vs. edge participation | No permanent center of *reward*: the edge tier that does most of the work stays a net-positive place to do it (T-AR). |

---

## Part IX — The three tiers (immutable / tenet / evolving)

**Immutable — amending is close to redefining the project; requires deliberate,
reviewed consensus.**

- **M0 — the mission:** *hold* the privacy × accountability × Sybil trilemma —
  refuse to trade any corner away; abandoning any corner abandons the project.
  Held **iff** an external red-team suite denies all three failure modes (Part 0),
  where the Sybil mode is the *systemic* no-discount / no-quiet-capture claim, not
  a per-primitive test. Not a victory claim — a refusal, bound to a falsifiable
  test.
- The six corners made structural:
  1. **Content-blind by construction** (B4) — hosts store ciphertext they cannot
     read or choose.
  2. **The bytes are the truth** (S1/B3) — content-addressed, re-verified,
     bit-perfect or an explicit failure; never silently wrong.
  3. **No *permanent* center** (T1/S4) — nothing *permanently* load-bearing; no
     machine or operator can take content down globally. Bootstrap centralization
     is allowed but must be **explicit, time-boxed, and shed on measured
     decentralization**: the bootstrap anchors are training wheels, not a center,
     and the decentralized path exists from day one. What is forbidden is a
     *standing* dependency on any node — not the honest admission that a young
     network leans on scaffolding while it matures. M0's Sybil soundness is
     *conditional* on this maturation: the composition is sound in the **mature
     regime**, and the anchors scaffold the **young** one — the bet, stated
     plainly, is that **maturity is reached before the scaffolding can be
     captured.** That is a permanent structural race, true for every launch, not
     a phase that ends. It is bridged by a **shed metric** over bond-distinct
     operators; the shed is a **one-way latch** that never re-arms, so no later
     concentration can restore a standing dependency on the anchors.
     The honest cost of "no permanent center" is a bounded, socially-recoverable
     re-centralization residual (the honest whale) — owned in
     [`design/m0.md`](design/m0.md), not a privileged party.
  4. **Access is unsurveilled — silt refuses to surveil, and pursues
     access-privacy to the trilemma's limit** (Don't #3). silt builds *no*
     mechanism to log or link who-fetched-what, and pushes access-privacy as far
     as proven crypto allows at the **metadata layer**. What is *never*
     guaranteed is blob-layer unobservability against a global adversary — the
     anonymity trilemma is a hard wall (strong anonymity + low bandwidth + low
     latency: pick two), and a participating node sees the keys it routes and
     serves. So this corner is the **refusal to surveil** (absolute) plus
     **access-unobservability held in tension** (metadata-layer, bounded by
     anonymity-set size) — not an absolute blob-layer guarantee.
  5. **No silent or global censorship** (Don't #2) — takedown is transparent,
     consensual, and plural; never one switch. Every honored revocation is
     committed to an append-only **transparency log**, so silt can *prove* it
     never flipped a global switch — the goal being a formal **non-globality**
     guarantee (how much survived, on how many independent hosts).
  6. **Core carries zero meaning, forever** (T3) — the Aslan boundary.

**Frozen consensus formats — a shipped-and-ratified wire/validity format is
immutable; changing it requires a NEW ERA, never an edit.** Once a consensus
format is built, certified, and ratified, its bytes are law: an in-place change
would silently re-interpret committed history or split the network. A format is
amended only by minting a new block version (a new era) behind a height-gated
hard fork — the same deliberate, reviewed bar as the corners above. An
un-upgraded node **stalls** at the era boundary rather than accept a format it
cannot validate; that stall is the correct safety-first behavior. The specific
frozen formats, their exact specs, and their activation heights are build state —
they live in the [`decisions.md`](decisions.md) freeze entries, not here, because
the *principle* is the immutable, and the individual formats accrete under it.

**Build-immutables — held at the same amendment bar, but about *how we build*,
not *what silt is*.** The corners above are **product-immutables**: change one and
it is a different project. These are **build-immutables**: change one and the
project silently rots as it grows. They are distinct in kind but equal in
standing. Each was distilled from a real, paid-for loss; the incidents are
recorded in [`tenets-history.md`](tenets-history.md).

  1. **Three-tier Definition of Done** (V1/V2) — no major component is "done"
     until it is proven at unit + integration/sim + e2e; a skipped tier is stated
     with a reason (V4), never silent.
  2. **A bug fixed once stays fixed, caught locally** (V5) — every discovered
     defect ships with a failing-first regression test at its tier(s), runnable
     on a contributor's own machine, so CI is the backstop and never the first
     line of defense.
  3. **One signal, one job — never fuse transport with security.** A single
     measurement must not serve two masters. Reply-latency is *transport* (RTT +
     jitter) **and** *compute* — gating on the sum is unsound; routing
     reachability is not consensus standing; liveness is not correctness.
     Security rests on **structure** (in the proof object) or **statistics** (over
     many independent measurements), never on a single network number the
     adversary's own path can move — *latency proves proximity, never diligence.*
     A timing signal may ship as a **soft, disclosed** deterrent, but a **hard**
     security gate must be structural, and an unbuilt structure is an **owned,
     named residual**, not a wall-clock stopgap. The owned residuals are written
     down — **consult [`design/owned-residuals.md`](design/owned-residuals.md).**
  4. **Cheap honest participation is a security constraint, not a marketing
     feature.** No defense may raise the floor of honest participation. A
     mechanism that prices out the small operator — scaling a min-bond off a
     transport knob, demanding GiB where MiB suffices — is a **regression against
     silt's reason to exist** (Part 0), even if it closes a real attack. Security
     parameters must be **decoupled from performance/transport tuning** so
     hardening one axis never taxes the other.
  5. **Build for the adverse internet — durability is the default, not a
     hardening pass.** silt is network-heavy software whose every path runs on the
     open internet, where **jitter, latency, packet loss, and reordering are the
     everyday case**, not the exception. A network function is not "done" until it
     survives them: **generous, adaptive transport deadlines** (per-peer RTO sized
     to the worst real path, *scaled to payload size*, never a magic constant),
     **retry — don't evict** a live peer on a single slow/dropped packet,
     **minimum-filter** a noisy latency signal to its floor rather than trusting
     one sample, and keep **large payloads off the critical path** (succinct
     proofs > FEC > QUIC). This is the *liveness* dual of #3: #3 forbids gating
     **security** on an optimistic network; this forbids gating **liveness** on
     one. The settled prior art is written down — **consult
     [`network-durability.md`](network-durability.md) BEFORE inventing any
     timeout / retry / eviction / large-payload scheme.**
  6. **Root-cause before you patch — attribute before you ship.** No knob moves
     before a log, trace, or test **names the mechanism** of the failure. Before
     writing a fix, write the one-paragraph mechanism: *the failure is X
     **because** Y; this change addresses Y **by** Z.* If you cannot write it with
     evidence, you are guessing — **stop, instrument, and reduce to a cheap
     deterministic repro**; and if the mechanism is still unknown, **or the change
     touches a security parameter, a consensus rule, or a published claim, consult
     research BEFORE building or spending a billable run.** **Grep for the
     mechanism before building new machinery** — a surprising amount is
     *unused-correct* code, not missing code. An expensive/billable run
     **confirms** an already-understood, locally-reproduced fix; it never
     **discovers** a cause or **tests** a guess. This is the *sequencing* dual of
     #3 and #5. The discipline is written down — **consult
     [`build-process.md`](build-process.md) BEFORE reaching for a knob.**
  7. **Evidence or nothing — no forward step on a hypothesis; when you lack the
     evidence, your job is to GATHER it, not to guess.** Every action that spends
     time, money, or commits a claim — a fix, a cloud run, a "let me try", a
     reported conclusion, a "next step" — must be justified by a **specific piece
     of evidence you can cite**: a log line, a trace, a failing test, a reduced
     reproduction, a measured number. If the honest justification is *"I think /
     probably / likely / let me just try and see"* — **you are guessing. STOP.**
     When you do not have the evidence, the ONLY valid next action is to
     **gather** it — instrument, reproduce, capture — never to guess-and-act. This
     is distinct from #6 (which attributes **one** failure before **one** patch);
     #7 governs **all** forward motion and demands two things of it:
       - **Iterative — one evidence-verified step at a time.** Make the smallest
         change a specific piece of evidence justifies, confirm *that* change with
         evidence, then take the next step. A batch of hopeful edits is a batch of
         guesses; a run launched to "see what happens" is a guess with a bill.
       - **When a failure is not locally reproducible, INSTRUMENT to capture the
         cause — do not re-try on a theory.** Add the logging / journal-capture /
         probe that will record *why*, let **one** instrumented observation gather
         it, then act on what it shows. "Re-run — probably transient" is a guess in
         a lab coat unless the re-run is actually *capturing* the evidence you
         lack.
     The self-check before every action: **say the evidence out loud.** Name the
     artifact that justifies this step. If you cannot, you have just found your
     real next task — go get the evidence.
  8. **Fits the hobbyist box — a small operator's box is a hard design gate,
     measured before you commit.** silt must *run*, and stay network-performant,
     on a small operator's box: bounded memory (**never OOM**) under adversarial
     input first, then fast — *bounded-then-fast*. This is not an aspiration
     checked at the end; it is a **design-time gate**. Before committing to any
     mechanism, measure its **full** resource cost on the floor box — the cost to
     **produce** an artifact, not only to verify, store, or transmit it. A
     mechanism whose output is tiny but whose *production* blows the floor is
     **disqualified**, however elegant. The exact box spec is a named *Evolving*
     parameter; the **fit-and-measure discipline is immutable.** This is the
     resource dual of #4: #4 forbids *raising* the honest floor; #8 makes the
     floor **concrete and testable** and moves the check **before** the build.
     *An unbounded system on a small box is not inefficient, it is unsafe.*

**Tenets — canon, amendable with reviewed consensus and evidence.** Everything
else in Parts I–VIII, including the strong disciplines we hold nearly as firmly as
the immutables but that describe *how we build and govern* rather than *what silt
is*: trust-but-verify / no-optimistic-operations (B7), best-components-novel-
composition (B8), reward-tracks-value (Don't #7), operator-autonomous-security-
gated-updates (R4), the anti-recentralization stance (T-AR, below), and the
per-persona promises and tradeoff stances (Parts VII–VIII).

- **T-AR — Anti-recentralization: no permanent center of *reward*.** The economic
  dual of immutable #3 (no permanent center of *control*). The edge tier that does
  the **majority of the work** must remain a **net-positive place to do it** — if
  the reward structure quietly concentrates so that only a few large operators can
  profitably participate, the network has re-centralized economically even while
  no single node is formally load-bearing. A durable center of reward is as much a
  betrayal of the mission as a durable center of control. This is a *principle*,
  not a parameter: it names the outcome to defend (a profitable edge), and the
  ratios, weights, and thresholds that keep it true at the network's current size
  stay in the Evolving tier.

**Evolving — expected to change as we learn.** Specific algorithms and parameters
(erasure k/n, replication factor, cache policy, DHT constants), the *exact*
economic mechanism, and which roles ship first — including the **Sybil-cost
parameters** that keep the M0 composition true at the network's current size (the
non-substitutable-resource weights, the concentration threshold, the
demand-attestation ratio, the audit/decay windows), the **edge-reward ratios**
that keep T-AR true, and — under build-immutables #4 and #8 — the
**honest-validator hardware floor** and the onboarding budgets it implies. These
are *held in tension* and re-tuned as the network grows — **not closed.** The
concrete current values live in [`decisions.md`](decisions.md) and
[`design/m0.md`](design/m0.md).

> **The principle-not-mechanism rule, and its one exception.** A tenet gates a
> release as a *principle*, never as a *mechanism* — "reward tracks value" is
> canon, but *which* economic mechanism satisfies it, and *when*, is a roadmap
> call (see [`ROADMAP.md`](../ROADMAP.md)). The **single deliberate exception** is
> M0: the trilemma resolution — token-less, work-backed, unlinkable reputation —
> is not a feature that satisfies a principle, it *is* the reason silt exists, so
> its real (not placeholder) mechanism is pulled into first-release scope by
> definition, built from best-in-class components and proven multi-machine.

---

*The dated amendment log and the product war-stories that motivated each build
principle live in [`tenets-history.md`](tenets-history.md).*
