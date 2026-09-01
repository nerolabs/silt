# Economy-observability design — Boulder 2, R2.1/R2.2 (node-local, no aggregator)

**Date:** 2026-09-01 · **Seat:** Builder · **Type:** DESIGN ONLY (no code this pass) ·
**Sources this deliverable is built on:**
- Economist spec: `/Users/andrewedmond/Claude/claude/silt-reviews/economist/2026-09-01-economy-observability-and-roadmap-proposal.md`
- Economist audit: `/Users/andrewedmond/Claude/claude/silt-reviews/economist/2026-09-01-tiered-edge-economy-sustainability-audit.md`
- Existing telemetry surface: `cmd/silt/ui.go` (`/api/status` durability block, `/api/fund`).

**The requirement (owner, one line):** any operator sees the economy's health/solvency —
their OWN and the reachable NETWORK's — from ONE node, no central aggregator, and sees
recentralization / insolvency / a funding cliff BEFORE it bites.

**This design needs NO Researcher cert.** It is observability: it surfaces existing
accounting; it changes no mechanism, no validity rule, no economic knob. The two places it
touches state (a per-node repair counter, a per-node work-Gini series) are ADDITIVE
read-side accounting, not a change to any conservation or standing rule. Called out
explicitly in §7 so review can confirm the no-cert claim.

---

## 0. The one honesty rule this whole design turns on

Every panel and every endpoint field carries a **knowability tier**. No number is published
without it. The four tiers, hardest-honest to softest:

| Tier | Meaning | Trust |
|---|---|---|
| **local-exact** | read from THIS node's own `Ledger` / `Chain` / `care` state | exact |
| **committed-global** | read from the committed chain every node holds (`C2Metric`, bond ledger) | exact as-of-my-head |
| **gossip-estimated** | a local peer SAMPLE extrapolated by the DHT crowd-estimator | estimate ± stated sample size/error |
| **not-knowable** | off-ledger, private, or unprovable from one node | never publish as fact |

The template is already in the tree: `EstimateNetworkSize` (`core/dht/estimate.go:24`) counts
a crowd it cannot see from local XOR distances, accurate to a constant factor. Economic
network-awareness is the same move. `NetEstimate` (`core/node/capacity.go:14`) already
extrapolates network bytes this way. We reuse both, and we never let a gossip-estimated
number render without its sample size next to it.

---

## 1. The node-local API surface (extends `/api/status` + `/api/fund`)

The existing seed we extend, not replace:
- `/api/status` carries `durability` (per cared object: `reserve`/`funded`/`paid`/`repairs`/
  `horizonSec`, plus `balance` and `bountyOn`) and a `network` `NetEstimate`
  (`ui.go:314-362`, `capacity.go`).
- `/api/fund` re-endows an object's reserve from this node's balance (`ui.go:567`).

New endpoints, grouped SELF (about this node — local-exact) vs NETWORK (about the reachable
crowd — committed-global or gossip-estimated). Every one is a GET, read-only, no token
(reading moves nothing); `/api/fund` remains the only mutator and is untouched.

### SELF — local-exact (the MVP, ships with economy-ON, needs no gossip)

| Endpoint | Exposes | Source in code |
|---|---|---|
| `GET /api/economy/self` | my balance; my bond (pts + bytes); my capability class; my lifetime served-bytes + fetched-bytes; my repairs-done; my rolling skim-in vs bounty-out; my modeled/entered cost + margin | `Ledger.CreditBalance`, `ServedBytes`/`FetchedBytes` (`credit.go:303-304`), bond ledger, `-serve-content`/`-validator`/`-archive` flags (`daemon.go:69-72`), the new per-node repair counter (§4), operator-cost config |
| `GET /api/economy/objects` | the existing durability block PLUS `epochsToExpiry` and a `cliff` bool per cared object | `CaredDurability` (`node.go:852`) → `credit.Horizon` (`instruments.go:52`); cliff = horizon within the warning window |
| `GET /api/economy/flows` | skim-in deltas vs bounty-out deltas over a rolling window, per object and pooled | escrow `funded` deltas (skim-in, `escrow.go:134`) vs `paid` deltas (bounty-out, `escrow.go:166`); a small ring buffer of `DurabilitySnapshot`s |
| `GET /api/economy/suspicion` | MY own wash-SHAPE self-check (serve/fetch symmetry near 1.0 + net-negative balance churn), as a traffic light, labeled "suspected" never "detected" | `ServedBytes`/`FetchedBytes` ratio + balance-churn ring; `not-knowable` for authenticity |

### NETWORK — committed-global + gossip-estimated (the full set, follows MVP)

| Endpoint | Exposes | Tier + source |
|---|---|---|
| `GET /api/economy/network` | estimated node count + tier mix (real ratio vs 10000:100:1) with sample size | gossip-estimated: `EstimateNetwork` (`capacity.go:27`) × peer capability-class sample (needs a new gossip field, §4) |
| `GET /api/economy/concentration` | serve-work Gini AND repair-work Gini over the local peer sample (SEPARATE), plus committed-exact `C2Metric` standing concentration; sample size on the Ginis | gossip-estimated Ginis (`credit.Gini`, `credit.go:334`, over the peer sample); committed-global `C2Metric` (`chain.go:2300`) |
| `GET /api/economy/g` | live `g` (annualized trend of cost-per-repair) with the `g>0` / `g<=0` threshold, once computable | local-exact per MY objects (`credit.G`, `instruments.go:84`); gossip-estimated for the network aggregate |

---

## 2. The MVP — 4 local-exact panels (concrete metric defs + code source)

These are the minimum to see the three failures (recentralization proxy, insolvency,
funding cliff) and they need NO gossip and NO new mechanism. They ship the moment the
economy is ON.

### Panel 1 — My solvency (the "your content dies on DATE" warning)
- **Metric:** per cared object, `epochsToExpiry = Horizon(snapshot, uptime)` converted to
  epochs; `cliff = finite && epochsToExpiry <= warningWindow`. Balance shown as the reserve
  bar; RED zone when `cliff`.
- **Source:** `credit.Horizon` (`instruments.go:52`) over `CaredDurability` snapshots
  (`node.go:852`). `finite==false` renders as "horizon not yet measurable" — NOT as
  "perpetual" (the instrument's own contract, `instruments.go:47-49`). Never fake precision.
- **Tier:** local-exact.
- **Endpoint:** `/api/economy/objects`.

### Panel 2 — Am I profitable
- **Metric:** `margin_per_epoch = (serve_credit + relay_payword + bounty_earned) −
  operator_cost`. Revenue is exact from the ledger; cost is operator-supplied (a config
  field) or modeled `bytes × $/GB + storage × $/TB + bond-plot`.
- **Source:** serve credit from `ServedBytes` net of skim (`escrow.go:130`); relay from the
  PayWord settle balance; bounty-earned from `PayBounty` credited to this node
  (`escrow.go:168`); cost from config.
- **Tier:** local-exact for revenue; the COST input is operator-entered, so the margin is
  "exact given your cost number." Never assert a NETWORK-wide margin as fact (§3).
- **Endpoint:** `/api/economy/self` + `/api/economy/flows`.

### Panel 3 — Is durability self-funding
- **Metric:** `skim_in − bounty_out` over a rolling window, per object and pooled. A
  persistent `bounty_out > skim_in` is the drain signal.
- **Source:** escrow `funded` deltas (skim-in, `escrow.go:134`) vs `paid` deltas
  (bounty-out, `escrow.go:166`), from a ring of `DurabilitySnapshot`s.
- **Tier:** local-exact (my cared objects).
- **Endpoint:** `/api/economy/flows`.

### Panel 4 — Am I in a wash-suspicion cluster (self-check)
- **Metric:** a single traffic light on {serve/fetch byte symmetry near 1.0, net-negative
  balance churn}. Lets an HONEST operator see their own shape and prove they are not the
  cluster.
- **Source:** `ServedBytes`/`FetchedBytes` ratio (`credit.go:303-304`) + a balance-churn
  ring.
- **Tier:** the SHAPE is local-exact; AUTHENTICITY is **not-knowable** (Douceur — a node
  cannot prove another identity is a Sybil). Labeled "suspected", never "detected". Never a
  slashing input.
- **Endpoint:** `/api/economy/suspicion`.

**Why panels 1-4 unblock the economist immediately:** they read state that already exists
(escrow accounting, served/fetched bytes, horizon math) plus one new per-node repair
counter. No gossip layer, no research cert, no accept-flip. They are the instrument #183 and
the field runs read to grade solvency.

---

## 3. Knowability tier per metric (honest on every panel)

| Metric | Tier | Why / the honest limit |
|---|---|---|
| my balance, bond, served/fetched bytes, repairs-done | local-exact | my own `Ledger` |
| per-object reserve/funded/paid/repairs/horizon/cliff | local-exact | my `care` + escrow |
| my skim-in vs bounty-out | local-exact | my escrow deltas |
| my margin | local-exact revenue, operator-entered cost | cost is off-ledger and private |
| my `g` (per my objects) | local-exact | `credit.G` over my snapshots |
| C2 standing concentration | committed-global | `C2Metric` — every node holds the chain |
| network node count + tier mix | gossip-estimated | `EstimateNetworkSize` × peer-class sample; **needs a tier-class gossip field** |
| serve-work Gini (network) | gossip-estimated | `Gini` over the peer SAMPLE's served bytes; sample size attached |
| repair-work Gini (network) | gossip-estimated | `Gini` over the peer SAMPLE's repairs-done; **needs a per-node repairs-done gossip field** |
| network aggregate `g` | gossip-estimated | extrapolated from sampled cost-per-repair |
| **true per-tier margin network-wide** | **not-knowable** | cost is off-ledger/private; only proxies (served bytes, bond size) observable |
| **authenticity of a wash cluster** | **not-knowable** | Douceur; surface the shape, human/red-team judges |
| **exact global repair-work Gini** | **not-knowable** | balance-lane data is per-node-private; only a LOCAL-SAMPLE Gini with its sample size is honest |

**The load-bearing honesty gaps (call them out, do not paper over):**
1. **Per-node repairs-done does NOT exist today.** The ledger tracks `Repairs` PER OBJECT
   (escrow, `escrow.go:167`), never per repairer node. `PayBounty` credits the repairer's
   BALANCE but does not count their repair COUNT. The repair-work Gini therefore needs a new
   per-node repair counter (§4). Until it lands, the repair-Gini panel is empty, not faked.
2. **Peer gossip carries only capacity bytes.** `peerCaps` (`capacity.go:37`) gossips
   `used`/`total` bytes — NOT tier class, NOT served bytes, NOT repairs-done. The network
   tier-mix and network work-Ginis need new gossip fields (§4). Until then those panels show
   "sample too small / field absent," not a guess.

---

## 4. The full set — how each series is computed

### Serve-work Gini and repair-work Gini — SEPARATE series
- **Serve-work Gini:** `Gini(served_bytes over the peer sample)`. `ServedBytes` is already
  tracked per node (`credit.go:303`); the network view needs peers to gossip their served
  bytes (new gossip field). Local self-Gini (my objects' server distribution) is available
  now if useful, but the recentralization signal is the CROSS-NODE one.
- **Repair-work Gini:** `Gini(repairs_done over the peer sample)`. Requires (a) a new
  per-node `repairsDone` counter incremented where `PayBounty` credits a repairer, and (b)
  a gossip field so peers report it. This is the finding-1b alarm: repair-Gini climbing while
  balance-Gini looks flat is the exact signature of repair concentrating on the horse tier.
- **Why separate:** balance Gini conflates serving and repair. `credit.Gini(balances)`
  (`credit.go:330`) cannot see that repair is recentralizing while serving federates. The
  economist's whole finding-1b hinges on splitting them.

### Per-tier margin (this node)
- `(serve_credit + relay_payword + bounty_earned) − operator_cost` per epoch, tagged with
  MY capability class (`-serve-content`/`-validator`/`-archive`, `daemon.go:69-72`). Revenue
  exact; cost operator-supplied. The dashboard aggregates MY class only — it never claims to
  know another operator's cost (not-knowable, §3).

### Live `g` (credit-cost of one shard-repair per year)
- `credit.G(old, new, dt)` (`instruments.go:84`) over two `DurabilitySnapshot`s of the same
  object separated by wall time. Signed so `g>0` = cost declining (perpetual earnable),
  `g<=0` = the plateau/inflation regime forcing re-endowment. Returns 0 (flat/unknown) when
  it cannot be computed — never a false signal. Per-object local-exact; network aggregate is
  a gossip-estimated extrapolation. The `g` panel marks the `g>0` / `g<=0` threshold as the
  perpetual-vs-finite verdict line (D-S7).

### Funded-horizon-to-expiry
- `credit.Horizon` (`instruments.go:52`) → `epochsToExpiry` and `cliff`. `finite==false`
  renders "not yet measurable," never "perpetual." The count of objects within N epochs of
  expiry is the cold-data early-warning (Risk #3).

### Wash detection (suspected, never detected)
- Cluster on {serve/fetch byte symmetry near 1.0, net-negative balance churn, standing-vs-
  served divergence}. Local self-check exact; the network cluster view is a gossip-estimated
  SHAPE. Never authenticity, never a slashing input (Douceur; §3).

---

## 5. Sliceability — 6a/6b ship independently, unblock the economist now

The requirement decomposes into four slices with a strict "no downstream dependency"
property on the first two:

- **6a — node-local read APIs (SELF, local-exact).** The four SELF endpoints (§1) + the four
  MVP panels (§2). Depends ONLY on existing accounting plus the one per-node repair counter.
  **Zero dependency on any research cert, on the money-pump fix, or on the accept-flip.**
  Ships the moment the economy is ON. This is what the economist reads first.
- **6b — the two Ginis + per-tier margin.** Serve-work Gini and repair-work Gini as separate
  series (§4) + per-tier margin (my class). The LOCAL/self versions ship with 6a; the
  network versions depend on 6b's new gossip fields (peer served-bytes, repairs-done, tier
  class) but NOT on any cert. **6a and 6b together unblock the economist's whole audit
  instrumentation** without touching a mechanism.
- **6c — live `g` + funded-horizon aggregate + network wash-shape.** The gossip-estimated
  network aggregates. Depends on the 6b gossip layer. Still no cert (observability only).
- **6d — dashboard UX.** The panels 1-8 rendered, each with its knowability tier badge and,
  for gossip-estimated, its sample size. Pure presentation over 6a-6c.

**Independence proof:** 6a reads `Ledger`/`Chain`/`care` state that exists on main today (the
one addition, the per-node repair counter, is additive read-side accounting). It renders
whether or not the economy is ON — with economy OFF it shows the accounting that WOULD flow.
So 6a can merge and be reviewed before the money-pump fix (item 1) or the economy-ON flip
(item 2) land. It does not read `g` disbursement, so it does not wait on the accept-flip.

---

## 6. The testable-telemetry gate (so a field run can't drift into recentralization silently)

The failure mode this closes: a field/sim run recentralizes (repair concentrates on the
horse tier) and NO check goes red, so the drift ships. Per the standing lesson — a green
check with no demonstrated red is a comment that compiles — the gate must be ABLATABLE.

**The gate, at the sim tier (where the work distribution is controllable and deterministic):**
1. Run a topology with a KNOWN work distribution across N nodes (e.g. a federated baseline:
   serving spread evenly; and a concentrated baseline: top-k nodes do most serving/repair).
2. Assert `serveGini <= serveGiniThreshold` and `repairGini <= repairGiniThreshold` on the
   federated baseline (the SEPARATE series from §4, computed by the same `credit.Gini` the
   dashboard uses — the test and the panel share one code path so the test proves the panel).
3. **Ablation (the teeth proof):** feed the CONCENTRATED baseline and assert the SAME gate
   goes RED. A gate that never reddens on a concentrated distribution is decoration. This is
   the injected-defect step: the test is not shipped until the concentrated distribution has
   been watched to fail it.
4. **Threshold provenance:** derive the thresholds from the vision ratio and the federated
   baseline's measured Gini, not a guessed constant. Record the derivation next to the gate
   (the same discipline that fixed the too-loose cost budget in session-7). The threshold is
   an evolving-tier PARAMETER, not a hardcoded magic number.

A future field run then reads the SAME serve/repair-Gini series through the dashboard; if the
sim gate is green and the field series climbs past the threshold, that is a real regression
surfaced, not a silent drift. The gate lives at the sim tier so it catches locally in
seconds; the field run confirms, never discovers (build-immutable #7 / consensus-correctness
discipline).

---

## 7. What this does and does NOT touch (the no-cert claim, for blind review)

**Does NOT touch (no cert required):**
- No validity rule, fork-choice, epoch, slashing, or I1-I5 invariant.
- No M0 / C1 / C2 published claim.
- No economic MECHANISM: no skim fraction, no bond floor, no S/R threshold, no repair payee,
  no conservation rule. `PayBounty`, `RecordServeToObject`, the supersede rule, the
  Invariant-A classification — all read, none changed.
- No security parameter a proof depends on.

**Adds (additive, read-side):**
- New read-only GET endpoints (§1). No mutator except the untouched `/api/fund`.
- A per-node `repairsDone` counter incremented where `PayBounty` credits a repairer
  (`escrow.go:168`). This counts observable work; it feeds no standing, no conservation, no
  slashing. It is classified `neutral`/observability and MUST pass the Invariant-A guard
  (`invariant_a_test.go` reflection-enumerates every Ledger method) — if the counter adds a
  Ledger method, that guard forces it to be classified, which is the correct gate.
- New gossip fields (peer served-bytes, repairs-done, tier class) for the network panels
  (6b+). Gossip is advisory sampling, not consensus input.

**The one review flag:** the per-node repair counter is the only new state. It is
observability, but it lands in `core/credit`, so the Invariant-A guard will require it to be
classified. Confirm at build time that it classifies `neutral` and that no standing path
reads it. That is the single correctness obligation of this whole track, and the existing
guard already enforces it.

---

## Open items handed forward
- **Per-node `repairsDone` counter** (§4) is the one prerequisite for the repair-work Gini;
  it does not exist today. Build it in 6a alongside the SELF endpoints; verify it classifies
  `neutral` under the Invariant-A guard.
- **Tier-class + work gossip fields** (§4, §5-6b) gate the NETWORK panels; the SELF/MVP
  panels do not wait on them.
- **The Gini thresholds** (§6) are evolving-tier parameters to be derived from the vision
  ratio + federated baseline, recorded next to the gate — not guessed.
