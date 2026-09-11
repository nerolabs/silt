# The declared era at start-up — freeze manifest item 19's second clause

Date: 2026-09-11 · Builder · against `origin/main` `d05d053`

Companion to [`2026-09-11-cloud-era-probe-design.md`](2026-09-11-cloud-era-probe-design.md), which
built the observed half and recorded this half as owed (§4: *"`cmd/silt/daemon.go` is untouched …
another builder holds that file this session"*). The file is free now.

## 0. What was owed, and why one number answers nothing

Manifest item 19 has two clauses. #808 shipped the first: what era the CHAIN has reached, on
`silt chain-status` and `GET /api/status`. The second is *"the daemon prints its declared max block
era at start-up"*, and it is not a decoration on the first — it is the other half of a pair that
only discriminates together.

Cloud row `13b-delivery-settlement` has to separate two worlds behind one observation:

| What is observed | Under a build that declares v5 | Under a build that declares v2 |
|---|---|---|
| no v5 block on the chain | a HEALTHY dark network | the WRONG BUILD |

Same chain, same census, opposite verdicts. The observed era alone cannot pick between them, and
the declared era alone says nothing about any chain. So the daemon prints both, from one call, and
the row's deliverable is the DIFFERENCE between the four cells — not the presence of a line.

## 1. The trap this row inherits

`R-CLOUD-ERA-PROBE` absorbs `R-CARRIER-ROLLOUT-SIGNAL`, whose code half was vacuous. The surviving
artifact is `TestG5_StampFiveImpliesTheCarrierIsHashCovered`, which opens with

```go
if r.Version < BlockVersionWitnessable { t.Logf("vacuous by design"); return }
```

The stamp is 3, so the early return always fires, the test asserts nothing, and `go test` prints
`ok` exactly as for a real pass — `t.Logf` is invisible without `-v`. **The mechanism: a gate took
its guard condition from its own subject.**

Transposed into this deliverable, the same shape is a start-up line that is always the same line.
Every gate below therefore asserts that two renders DIFFER; none asserts that a line is present.

## 2. Decisions

### 2.1 Where the declared number comes from

| Option | Cost | Verdict |
|---|---|---|
| (a) A `-max-block-era` flag, or a config field | Free | **REFUTED.** A declared era an operator can type reports a belief. An operator debugging a dark network would read their own input back, and 13b's discrimination would be decided by the wrong party. |
| (b) Derive it at run time from the chain | Free | **REFUTED — it is circular.** The declared number's whole job is to be independent of the chain being judged. |
| (c) A compile-time constant, checked against the build's real ceilings | One constant, one gate | **CHOSEN.** |

`chain.DeclaredMaxBlockVersion = BlockVersionWitnessable`. Two things make (c) more than a literal:

- **It is asserted to be a constant at COMPILE time.** `const declaredIsACompileTimeConstant =
  DeclaredMaxBlockVersion` in `eradeclared_test.go` accepts a constant expression and nothing else,
  so the package stops building if the declaration ever becomes a var, a field or a flag read. No
  runtime assertion can distinguish those from a constant that happens to hold the same value today.
- **The claim it makes about the build is checked, not trusted.**
  `TestDeclaredMaxIsTheBINARYsRealCeiling` drives BOTH ceilings that define it: the decode ceiling
  (`versionSupported` accepts `DeclaredMaxBlockVersion` and rejects `+1` — both sides, because one
  side alone locates nothing) and the mint ceiling (`MintVersion` on a chain whose own readiness tally activated era-4). A
  constant that merely read `5` would keep reading `5` after someone widened `versionSupported` to
  6, and the daemon would then understate what it is.

### 2.2 The era ↔ version mapping is a TABLE, not arithmetic

`EraStatus.EraLine` derives the era name with `version - 1`. That is correct for the only two
versions it renders (v4 → era-3, v5 → era-4) and **wrong below them**: the CHANGELOG calls v1 and v2
chains era-1 and era-2, not era-0 and era-1. The sequence jumps because v3
(`BlockVersionRegGate`) is a readiness STAMP that no block is ever minted with.

`EraNameOf` is therefore a table — 1→1, 2→2, 4→3, 5→4, and *no era* for v3 — and it returns `ok`
false rather than inventing a name. This matters here and not in #808 because this render names the
era of arbitrary versions, including the v2 every live network is minting today.

### 2.3 What the verdict compares

`DeclaredRelation` compares the declared maximum against `Census.MaxVersion` — the highest version
ANYWHERE on the chain, not the head's. The question is whether this build can validate every block
it holds, and a chain can carry a version above its own head if history was reorganised below an
activation boundary.

Four members, a closed string enum with no zero value (the #808 device, for the #808 reason):
`ahead` (healthy dark), `at`, `behind` (wrong build), `unobserved`.

### 2.4 Two states that are easy to get wrong

- **No blocks at all.** A daemon whose store is empty holds a real chain with none — the state every
  fresh node is in until genesis is seeded. Rendering "highest version v0" there is the vacuity trap
  itself: a zero indistinguishable from a measurement. The render names the limit instead and claims
  no verdict. The declaration still prints, because it is a fact about the build and does not depend
  on a chain.
- **Locked in but not yet activated.** A build can be AT or AHEAD of every committed block and still
  be about to be stranded: the readiness tally locks an era in one epoch BEFORE its first block. That
  window is the only moment an operator can upgrade without a stall, and the latch is chain state, so
  it is observable. One warning line covers it. It is NOT printed under `behind`, where it would
  repeat the verdict directly above it in weaker words — a warning that fires on every unhealthy
  state is how an operator learns to skip the block.

### 2.5 Placement

The renderer is a new file, `core/chain/eradeclared.go`, not an addition to `chain.go` or
`erastate.go`: two other seats hold files in those packages this session, and a new file conflicts
with nothing. The daemon's entry point is `eraStartupLines(ch *chain.Chain)` — it takes a chain and
NOTHING else, which is what makes "no flag can reach this number" structural rather than asserted.

The call sits after the replay and after the genesis seed. Ahead of the replay it would report an
empty chain on every restart; ahead of the seed, on every fresh node. Both orderings are gated.

## 3. Gates, and the ablation that made each one red

| Gate | Tier | Drives | RED shown by |
|---|---|---|---|
| G-DE-1 | `core/chain` | the declaration against both real ceilings | A-1: declare v4 → "versionSupported accepts v5, above the declared maximum v4" |
| G-DE-2 | `core/chain` | the four-cell matrix {v5, v2} × {dark chain, era-4 chain} | A-2 (render ignores its argument), A-3 (render ignores the chain), A-8 (`const`→`var`: the package stops compiling) |
| G-DE-3 | `core/chain` | every era knob in `Config` moved; render must not budge | A-4: point `EraState` at `c.cfg.Era4ActivationHeight` → the render prints `999999` |
| G-DE-4 | `core/chain` | an empty chain: no verdict, no zeros | A-5: drop the guard → "highest version v0" |
| G-DE-5 | `core/chain` | every member of `EraRelations`, driven and pairwise distinct | A-6: add an undriven member |
| G-DE-6 | `core/chain` | the stranding warning on a PENDING chain, and its ABSENCE for the shipped build | A-7 |
| G-DE-7 | `cmd/silt` | the daemon's own number under divergent consensus config | A-11 |
| G-DE-8 | `cmd/silt` | the call site, and its position after replay and seed | A-9 (delete it), A-10 (move it ahead of the replay) |
| G-DE-9 | `cmd/silt` | both markers registered in `ObservableContract` | red on first run, before registration |

**Both chains in G-DE-2 are DRIVEN.** `eraProbeChain` is handed four signed bond registrations and
one number — the readiness stamp — and the chain decides the rest: whether the tally latches, at what
height, and what version each block is minted with. No fixture writes a `Version`, a latch or an
activation height. The contrast BUILD (declared v2) is constructed, because this binary is not one;
what it proves is a property of the renderer, and the shipped path's own number is proved separately
at the `cmd` tier.

**G-DE-8's limit, stated rather than glossed.** It is a source gate. It proves the call is in the
start-up path's source, not that it is REACHED at run time — a new early return above it would be
invisible (instance 2 of the observable-log-contract scar). Spawning a daemon to read its stdout is
the e2e tier's job. What this gate makes impossible is the silent deletion of the call while every
unit test stays green; `ObservableContract` covers the deletion of the strings themselves.

## 4. Deliberately NOT done

- **No e2e assertion on a spawned daemon.** Proportionate to a start-up log line, and the two
  registered `ObservableContract` markers already carry the rename half of that scar. Named as a
  limit, not left implied.
- **No new residual, no new prefix** (simplicity rule 4).
- **The row does not close.** Its other owed item is the release-runbook line, and per
  `docs/era4-freeze-what-closes.md` the runbook is due AT THE STAMP RAISE, not at the freeze. Writing
  it now would be guessing at a document nobody has named. The register row stays `ACTIONABLE` — the
  only legal bucket for an open row — with the daemon half recorded as built.
- **No block field, no committed leaf, no validity rule.** This adds a start-up log line. It is not
  a format item and carries no freeze deadline.
