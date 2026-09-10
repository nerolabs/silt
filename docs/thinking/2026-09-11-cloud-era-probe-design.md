# R-CLOUD-ERA-PROBE — the era observable (freeze manifest item 19, lane D2)

Date: 2026-09-11 · Builder · against `origin/main` `3a5c8f0`

## 0. The problem, in evidence

Three investigations stalled on the same missing observable.

1. A Tester could not confirm whether the two live test networks carry any v4 block, because
   `silt chain-status` prints no block `Version`. Its verdict had to end *"I verified the
   in-repo configuration, not the live nets' actual `chain.cbor`."*
2. The live maximum `len(b.Atts)` is unmeasured and unmeasurable from any shipped command. It is
   a named open item on two certifications.
3. Cloud row `13b-delivery-settlement` SKIPs with one sentence covering two different worlds
   (`integration/cloudtest/console-2633a11-deep.log`): *"because era-4 is dark … though the same
   sentence would also cover off-commitment keys (no era surface exists to tell them apart)."*

Measured RED, this branch, before any change. `silt chain-status` on a store holding a v4 block,
two v5 blocks and a 3-wide attestation carrier:

```
  head height:  3
  head hash:    9410ceb3c09b6795fcf1a885ba279db1e2357ced5b5ce56523ef133cbd896de3
  blocks:       4 (incl. genesis)
  entries:      1 committed
  pruned:       0 blocks payload-stripped below the retention horizon
```

That output is BYTE-IDENTICAL to the output for an all-v2 chain with no attestations. And
`GET /api/status` `.chain` is, in full, `{"height":1,"entries":1}`.

## 1. The predecessor was vacuous — the mechanism, and what it forbids here

ROADMAP row `R-CLOUD-ERA-PROBE` says it *"absorbs the rollout-signal item, whose code half was
vacuous."* That is `R-CARRIER-ROLLOUT-SIGNAL`, disposed in the freeze-manifest certification §4.12.
The certification names two of its three parts vacuous:

- **The named-upgrade check on the activation flag can never fire.** G-8 option (iii) ratified that
  no activation override exists in any binary. A precondition on a flag that will not exist is a
  note, not a deliverable. (Re-verified independently on this branch: `grep` for
  `Era3ActivationHeight|Era4ActivationHeight` across `cmd/`, `adapters/`, `core/node/` returns zero
  non-test hits. There is no setter.)
- **The self-naming stall on unknown cbor key 18 is unbuildable as specified.** A strict CBOR
  decoder was refuted; without one a pre-carrier binary does not *see* key 18, so the failure is
  safety-preserving and loud but cannot name its cause.

Its code half survives in the tree as `TestG5_StampFiveImpliesTheCarrierIsHashCovered`, and the
shape is worth stating exactly, because it is the shape to avoid:

```go
if r.Version < BlockVersionWitnessable {
    t.Logf("G5 vacuous by design: the stamp is %d, not %d …")
    return                  // <- everything below never runs
}
// ... the only assertion in the test
```

**The mechanism: the gate takes its guard condition from its own subject.** The stamp is 3, so the
early return fires, so the test asserts nothing — and `go test` prints `ok` exactly as it would for
a real pass (`t.Logf` is invisible without `-v`). A PASS from this gate carries zero bits. The
generalisation, and the standing rule this row exists to honour: **a green gate with no demonstrated
red is decoration.**

**The same shape, transposed into an observable, is the trap for this row:** a field that is always
zero, or always absent, is indistinguishable from a field correctly reporting an empty state. If
"era-4 dark" and "the field was never populated" render identically, the observable answers nothing
— and 13b's SKIP sentence would be reproduced verbatim in a new costume.

**So the governing design constraint here is: ABSENT and ZERO must be structurally distinguishable,
and the distinction must be gated.** Section 3 is how.

## 2. Options weighed

### 2.1 Where does the era state come from?

`era3Active` / `era4Active` each consult TWO routes: the genesis activation override
(`Config.Era3ActivationHeight` / `Config.Era4ActivationHeight`) and the readiness-tally latch
(`era3LockedIn`/`era3Height`, `era4LockedIn`/`era4Height`).

| Option | Cost | Verdict |
|---|---|---|
| (a) Report `c.cfg.Era4ActivationHeight` | Free | **REFUTED.** This is the `#380` class: a consensus quantity read from local config. It answers the wrong question — "what did this operator type", not "what does the chain say". |
| (b) Replay the blocks into a fresh `Chain` in the CLI to recover the latch | Needs a `Config` the CLI does not have. `chain-status` would have to *guess* `Quorum`, `MinBond`, `EpochBlocks`… | **REFUTED, and worse than (a).** A guessed `EpochBlocks` produces a latch height that is a function of a CLI flag. It manufactures the `#380` defect inside the tool built to detect era state. |
| (c) Report the latch fields, and derive "active" from the COMMITTED BLOCKS | One accessor, one pure reducer | **CHOSEN.** |

Under (c) the implementation reads `c.cfg` **nowhere at all**, which makes the divergent-config
ablation total rather than targeted: mutate every era-related config field, re-read, assert the
output is byte-identical.

Note the era override is *itself* committed now — `ConsensusParams` carries it at cbor key 20,
fields 13 and 14 (`D-CFGBIND-BUILT-2026-09-10`) — so reading `c.cfg` would not be unsound on a
params-bearing genesis. But `CheckConsensusParams` returns nil when `Params == nil`, so the
paramless path survives, and on that path a `c.cfg` read is a local-config read with no referent.
Not reading it at all costs nothing and closes the case.

### 2.2 What does "ACTIVE" mean, given two activation routes?

Defining `ActiveAtHead` as `era4Active(head)` would require reading the override. Instead:

> **`ActiveAtHead` for era N ⟺ the head block's `Version >= vN`.**

This is route-independent (it holds under the tally OR an override), it is a pure observation of
committed bytes, and it is exactly what 13b needs. It is sound because activation is a MINT
boundary: at and above `H_era4` a block MUST be v5, which is what `MintVersion` and the era-4
validity rule jointly enforce. So "a v5 block is committed" and "era-4 has activated" are the same
fact, reached without asking any config anything.

The latch (`LockedIn` + `ActivationHeight`) then supplies the one thing blocks alone cannot show:
the **one-epoch-of-notice window** where the tally has locked in but no v5 block exists yet. That
window is the single most useful state for an operator watching a stamp raise land, and it is
precisely the state that a block-only view renders identically to "dark".

Hence three phases, not two.

### 2.3 The CLI cannot see the latch — and must say so rather than print `false`

`cmdChainStatus` loads `chain.cbor` into a `[]Block`. It has no `*Chain`, and per 2.1(b) it must
not build one. So the tally latch is genuinely **not observable** from the offline CLI.

The vacuous move would be to give the CLI an `era4LockedIn: false` line. That is a lie dressed as a
zero — the exact defect. The CLI instead prints what it can prove from the blocks and **names what
it cannot see, and where to get it**. Two named states, no zero render:

- `ACTIVE — first v5 block at height N`
- `NOT ON THIS CHAIN — no v5 block is committed`, plus the sentence that offline this cannot
  separate "the tally has not locked in" from "locked in, activating later", and that a running
  daemon reports the tally at `GET /api/status` `.chain.era`.

The daemon status route, which does have the `*Chain`, reports all three phases.

### 2.4 Shape of the anti-vacuity mechanism

Three devices, each gated:

1. **`EraPhase` is a closed string enum with no zero value.** `""` is not a phase. A field that was
   never populated cannot masquerade as `dark`, because `dark` is the string `"dark"`.
2. **Optional heights are `*uint64`, not `uint64`.** Height `0` is a legal height (a genesis block
   can itself be v5), so `0` cannot mean "none". `nil` means none, `omitempty` drops the JSON key,
   and absent-versus-present-zero is a structural difference rather than a convention two fields
   have to maintain in step. This is the pattern `statusExtras` already documents on this surface
   ("absent and empty stay different objects").
3. **A zero count is narrated, never printed bare.** `max attestations: 0` alone is the trap;
   `max attestations: 0 — no block on this chain carries any attestation` is an answer.

## 3. Gates

Tier placement is forced by a fact worth recording: **`NewBondReg` hard-codes
`Version: BlockVersionRegGate` (3)**, deliberately — the readiness stamp is a property of the
binary, not a caller's choice. So no `cmd/silt`-tier fixture can mint a reg stamped 5, and therefore
no `cmd/silt`-tier test can drive a real era-4 latch. The driven fixture must live in `core/chain`.

| Gate | Tier | What it drives | Anti-vacuity role |
|---|---|---|---|
| G-EP-1 | `core/chain` | The REAL readiness tally: four validators with regs stamped 5, committed as blocks, until `era4LockedIn` flips. Asserts phase `pending`, then `active` after the first v5 block. | The latch is produced by the chain, not by the fixture. |
| G-EP-2 | `core/chain` | Ablation: on an already-latched chain, set `c.cfg.Era3ActivationHeight` / `c.cfg.Era4ActivationHeight` to divergent values and re-read. | The reported value must not move. RED-first by pointing the accessor at `c.cfg`. |
| G-EP-3 | `core/chain` | A NON-UNIFORM `Atts` distribution from real `Attest()` calls, max in the interior. | A uniform fixture would pass under a wrong reducer. |
| G-EP-4 | `core/chain` | All three phases are pairwise-distinct non-empty strings, each PRODUCED by a driven chain — no phase is asserted from a constant. | Simplicity rule 7: every "safe" row is a driven probe. |
| G-EP-5 | `cmd/silt` | Both arms of the CLI: a chain with a v5 block, and one without. Asserts the two renders differ AND that the dark render names what it cannot see. | The absent-vs-zero render distinction. |
| G-EP-6 | `cmd/silt` | The store round-trip: blocks through `chainstore.Save`/`Load` and out of `cmdChainStatus`. | This is the layer the Tester's blocked investigation actually hit — a live net's `chain.cbor`. |
| G-EP-7 | `cmd/silt` | `GET /api/status` `.chain.era` carries the phase for both eras and the census. | The programmatic consumer 13b will use. |

**On "the fixture must not supply the state whose production absence would be the defect."** The
`cmd/silt` gates DO hand-build blocks — they are renderer tests, and the renderer's input is
validated at the `core/chain` tier against a chain that produced its own v5 blocks and its own
latch. The seam between the two tiers is one function, `CensusOf`, exercised on both sides. Stated
here so the split is a decision and not an accident.

## 4. Deliberately NOT done

- **`cmd/silt/daemon.go` is untouched.** Manifest item 19 also asks that *"the daemon prints its
  declared max block era at start-up"*. Another builder holds that file this session. That half of
  item 19 is NOT delivered here and the row must not be closed as if it were; it is recorded as the
  remainder in `ROADMAP.md`.
- **No block field, no committed leaf, no validity rule changes.** This adds an observable and
  nothing else. It is not a format item and carries no freeze deadline.
- **No new residual prefix, no new `R-` row** (simplicity rule 4).
