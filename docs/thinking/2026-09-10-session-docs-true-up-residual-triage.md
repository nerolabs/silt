# Session docs true-up — the residual triage, and the corrections it forced

**Date:** 2026-09-10 · **Seat:** Builder · **Base:** `main` @ `3f4602e` (after #802, #803)
**Owner ask:** *"make sure we are clean and have an understanding on where we are at and where we
are going from here."*

---

## 0. The governing constraint, stated as an arithmetic target

The owner's diagnosis: the decisions got simpler while the bookkeeping got heavier, and the
bookkeeping is what makes the next decision hard. Simplicity rule 3 caps bookkeeping commits against
build commits; rule 4 says a residual must be actionable or it does not exist
(`docs/build-process.md`).

So this true-up has one hard success criterion, decided before any edit:

> **The `ROADMAP.md` residual register must contain FEWER rows after this change than before.**

Not "fewer than the certifications proposed." Fewer than today. A true-up that admits 22 new rows and
congratulates itself for having declined 15 has still made the next decision harder.

| Quantity | Count |
|---|---|
| Register rows on `3f4602e` | **35** |
| New residual names filed by the three 2026-09-10 certifications | **24** (not 22 — §1.1) |
| Rows if every filing were accepted | 59 |
| Rows after this change | **34** |

---

## 1. The triage

### 1.1 What was actually filed — the brief undercounts by two

The planner's brief says "22 new rows from two certifications … a third filed 8." The certifications
filed **24** distinct names. `R-CB-ATTS-UNBOUNDED-…` files **ten**, not eight — the brief's count
misses `R-ATTS-EVIDENCE-NARROWING` and `R-ATTS-REGISTRY-ORDER`, both in its HELD-IN-TENSION block.
Recorded because the input number is the denominator of the reduction claim.

| Certification | Names filed |
|---|---|
| `R-CARRIER-BYTES-lastcommit-legitimate-maximum-…` | 6 |
| `R-CB-ATTS-UNBOUNDED-validateStructural-reachability-and-bound-…` | 10 |
| `R-CARRIER-ATTS-PREPAREQC-wire-bound-on-env-QC-…` | 8 |

All three certification paths are under
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/`.

### 1.2 Both new prefixes die

`R-CB-` and `R-ATTS-` are unratified. The ROADMAP filing rule is explicit: *"No new prefixes … without
an owner ratification."* Everything that survives folds into `R-CARRIER-*`, which already names this
family (`R-CARRIER-BYTES`, `R-CARRIER-PREFIX-ONLY`, `R-CARRIER-BOXSPLIT` …).

### 1.3 Disposition — 24 filed names → 4 register rows

| Filed name(s) | Disposition |
|---|---|
| `R-ATTS-PREPAREQC-UNBOUNDED` | → **`R-CARRIER-ATTS-PREPAREQC`** (row) |
| `R-CARRIER-QC-PREPAREQC-VALIDITY` | folded into the same row — it rides the carrier's commit, not a separate closer |
| `R-CARRIER-QC-NESTED-ROUNDCHANGE`, `R-CARRIER-QC-SIGNMARK-BYTES` | → **`R-CARRIER-QC-NESTED-ROUNDCHANGE`** (row); see §1.4 |
| `R-CB-ATTS-UNBOUNDED`, `R-ATTS-EQUIV-QUADRATIC`, `R-ATTS-EVIDENCE-NARROWING` | → **`R-CARRIER-ATTS-NORMALIZE`** (row) — one fix closes all three |
| `R-ATTS-BLOCKS-131072` | → **`R-CARRIER-ATTS-BLOCKS-CEILING`** (row), post-RC, launch blocker |
| `R-ATTS-ERA3-ROOT-COUPLING` | **DELETED** — collapses entirely (§2) |
| `R-ATTS-WORKTREE-PROVENANCE`, `R-CARRIER-QC-WORKTREE-PROVENANCE` | **DELETED** — a method caveat belongs in a cert's method section, never as a register row |
| `R-CB-PRODUCER-DISCRETION`, `R-CB-CLASS-SPLIT`, `R-CB-REGISTRY-ORDER` + `R-ATTS-REGISTRY-ORDER`, `R-CB-LEGACY-SCREEN` + `R-ATTS-LEGACY` + `R-CARRIER-QC-LEGACY-UNCAPPED`, `R-CARRIER-QC-SYBIL-SENDER`, `R-CARRIER-QC-PRODUCER-LOCAL` | → `docs/design/m0.md` §10.1, **five lines** (the registry-order pair and the legacy trio are each one fact filed under two and three names) |
| `R-CB-PONY-MEASUREMENT` + `R-ATTS-PONY-MEASUREMENT`, `R-ATTS-LIVE-MAX-UNMEASURED`, `R-CARRIER-QC-BURST-VALUE` | → the Tester's measurement queue. Two certifications independently invented the same pony measurement under different names; one survives |

**Two certifications inventing the same residual under two names is the register's failure mode in
miniature.** Neither seat could see the other's filing, so the register would have carried both
forever, and every future reader would have paid to discover they were one thing.

### 1.4 Where I overrode the planner's disposition, and why

The brief lists `R-CARRIER-QC-SIGNMARK-BYTES` as a fifth keep. I folded it into
`R-CARRIER-QC-NESTED-ROUNDCHANGE` instead. Two reasons, in order of weight:

1. **Its closer is the other row's migration tail.** The certification's own text: sign marks
   persisted *before* the §5.3 normalization keep their padding and are re-broadcast after a restart;
   the closer is a trim in `(*Node).roundsFor`. That trim is only reachable — and only necessary —
   once the normalization it names has landed. One owner (builder), one lane, one PR.
2. **The brief could not state the two apart.** Its one-line summary of `R-CARRIER-QC-SIGNMARK-BYTES`
   — *"`(*roundChangeEnv).sigBytes` does not cover `LockQC`, so a genuine envelope can be replayed
   padded"* — is `R-CARRIER-QC-NESTED-ROUNDCHANGE`'s mechanism, not this one's. Two rows whose
   distinguishing content the filer restates as identical are one row.

Surfaced rather than done silently: if the planner wants the split back, it costs one row and the
argument above is the thing to refute.

### 1.5 Five existing rows retired — this is where the register actually shrinks

Admitting 4 rows against a 35-row register is +4. The reduction comes from applying the same triage
to what is already there. Each retirement below names the evidence that permits it.

| Retired | Where it goes | Evidence |
|---|---|---|
| `R-CONFIG-GATE-NODE-SCOPE` | folded into `R-CONFIG-GATE-V5-REGIME` | Both are defects in **the same test function**, `TestConsensusVerdictIsNotAFunctionOfLocalConfig`: one says its *regime* is wrong (it validates a `Version: 1` block), the other says its *reflection set* is wrong (`chain.Config`, not `node.Config`). One PR that re-points the regime and widens the reflection closes both. Same fact family, one closer — the merge criterion the 2026-09-07 triage already used on `R-CARRIER-SIG-COMPOSITION` |
| `R-LIVENESS-RECOVERY-UNBOUND` | `m0.md` §10.1 | The row's own text says **STRUCTURALLY UNBINDABLE**, and its "closer" is *"either an out-of-band coordination protocol … or accept-and-disclose."* That is a disclosure, not a closer. Rule 4 |
| `R-CARRIER-CREDIT-DENIAL` | `m0.md` §10.1 — **and the doc fix ships in this commit** | The row is `Lane TAIL (doc only)` and its entire remaining content is *"the doc fix the 2026-09-03 re-pricing named."* Docs are this seat's to write; the fix is written, so the row is done, not deferred |
| `R-NEST-GATE`, `R-NESTED-EVIDENCE-OVERCAP` | folded into `R-BIG-EVIDENCE-UNSLASHABLE` | **All three end in the identical sentence** — the equivocator keeps its seat because legitimate evidence exceeds `SlashesBytesCap`. They are three measured faces (coalition · honest-path nesting · self-armor) of one defect, and **all three close on one act**: the `(height, round, phase)` signature preimage making evidence `O(1)`. `R-NEST-GATE`'s own closer says so in terms — *"Row closes when that lands."* The surviving row is the owner-facing one and carries both measured numbers, including the **~7.9998 MiB/side** armor threshold that corrects the certification's derived ~8.39 MiB (do not re-cite 8.39) |

**35 − 5 + 4 = 34.**

### 1.6 What deliberately gets NO row

The 2026-09-10 inert-mechanism sweep
(`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/2026-09-10-inert-mechanism-sweep-core-adapters-0ed3b92.md`)
found 117 inert sites, 46 outside the frozen keystone. Three are cases where the record claims
delivered. **None of the three becomes a new register row**, and the other 43 get nothing:

- `Block.Params` / `CheckConsensusParams` — already tracked by `R-CONSENSUS-CONFIG-UNBOUND`.
- `RequireBondedFetchers` (D-DEMAND P3b) — already disclosed by `R-DEMAND-PRICE-LEVEL` in `m0.md`
  §10.1, which says `RequireBondedFetchers = false` into the flip. The sweep sharpens that line
  (nothing can turn it on) rather than opening a row. The record correction ships here.
- `BBootstrapRunPrecondition` (BB-14) — the owed act is the **record correction**, which ships here.
  The build is ordinary lane work with no `R-` name.

A name is a permanent cost: the register lint requires it to exist wherever it is mentioned, so every
name is a thing a human reads forever. **43 rows is 43 things a human reads forever, and a lint
regenerates that list on demand.** That is the whole reason this section exists.

---

## 2. `R-ATTS-ERA3-ROOT-COUPLING` — a "NEW finding" that re-derives a closed one

The certification files it as *"NEW, and it is live today with no attacker … a scheduled fork."* It is
none of those. Verified at source on `3f4602e`:

1. **Mechanism CONFIRMED.** A v4 proposer computes `StateRoot` via `(*Chain).PopulateEra3Roots`
   before the gather; at reload `(*Chain).validateEra3Roots` → `(*Chain).postApplyRoots` re-applies
   over the stored `Atts`. A first-seating entry does move the recomputed root.
2. **Stated failure mode REFUTED.** The check does not fire only at reload. `(*Chain).ValidateProposal`
   calls `(*Chain).validateEra3Roots` on the **commit** path, and `(*Chain).appendStructural` calls it
   again on the own-disk write path. A block that would brick reload cannot commit in the first place.
3. **Field reachability ZERO.** No v4 block can exist. Era-3 lock-in requires
   `c.regVersion[id] >= BlockVersionStateRoot` (= 4) over a ⅔ weight super-quorum, and `chain.NewBondReg`
   hard-codes `Version: BlockVersionRegGate` (= 3). The only other route, `Config.Era3ActivationHeight`,
   is set nowhere outside `_test.go`.
4. **It re-derives a closed defect.** `R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md`
   states the same mechanism verbatim — *"Under the shipped rule the seat never lands. Any precommit
   certificate that would add a new attester makes the recomputed state root differ from the root the
   proposer signed before gathering"* — and it was disposed by owner call **O1** with the era-3 retirement
   ratified under **O2** (*"the stamp goes 3 → 5 directly; no release ever stamps 4 … the era-3 frozen
   format is retired without ever running"*).

No row. The collapse is recorded in `m0.md` §10.1 so the next seat that reads `validateEra3Roots`
does not file it a third time.

**The reusable shape, for the scar ledger:** *a blind reading of live code re-derives a defect the
ledger closed by RETIRING the era it lives in.* The code still contains the mechanism, because
retiring an era does not delete it. A seat with no shell and no ledger access will find it every time.

---

## 3. Corrections — annotated in place, never laundered

Standing owner practice: the correction is visible as a correction.

### 3.1 The `Atts` exposure measurement is wrong by 10×

Cited as 1,318,209 entries / 132 MiB / 42–68 s. **Unreachable.** `chain.Decode` calls
`cbor.Unmarshal`, the package-default `DecMode`; `core/chain` sets `MaxArrayElements` nowhere, so
`defaultMaxArrayElements = 131072` (fxamacker/cbor v2.9.2) is enforced *before* allocation.

| | Cited | True |
|---|---|---|
| Entries | 1,318,209 | **131,072** |
| Resident | 132 MiB | **13.1 MiB** |
| Re-verify per restart | 42–68 s | **4.2–6.9 s** |
| Amplification | 156× | **15.5×** |

**The finding survives in a worse dimension.** The cost is cumulative and permanent, because
`(Block).Prune` never sheds `Atts`: a poisoned 1,000-block history is **13.1 GiB on disk and 70–115
minutes per daemon start, forever.** A one-off 68 s stall is a nuisance; an unbounded per-restart tax
that grows with history is not.

### 3.2 The ordering constraint was void as stated

`D-CFGBIND-CERT` says *"F moves the genesis hash; call A consumes it."* Two errors:

- **F as merged moved nothing** — it shipped schema only.
- **F's wiring, now being built, does not move the *paramless* genesis hash either.** `Block.Params`
  is `*ConsensusParams` with `cbor:"20,keyasint,omitempty"`, so a genesis minted without params drops
  key 20 and `e44344ea…72c0` is unchanged. The ~250 `AppendGenesis` fixtures are untouched.

What moves is any genesis a **daemon** mints. **The constraint therefore stands, for a different
reason:** call A joins F's wiring in **one network re-seed**, not one fixture re-pin. The currency is
graded field re-runs, not fixture churn.

Kept because it is the difference between a cheap sequencing note and a real one, and because the
cert's stated reason would have been discharged by a fixture edit that buys nothing.

### 3.3 `D-CFGBIND-CERT` says the foreign-genesis refusal is `LogDebug`. It is `LogWarn`.

`#800` fixed it and the cert text was never annotated. `(*Node).SyncChain` logs at
`ports.LogWarn`, and the code comment above it reads *"LOUD, not LogDebug."*

**Name the class explicitly, because it is the one the new lint cannot catch.** The cert cites
`core/node/chainrole.go:1616-1618`. That coordinate still resolves to the right symbol — the lint
shipped in `#803` checks exactly that and passes. Only the **claim about the content** rotted. A
symbol-anchored coordinate check proves you are looking at the right place; it cannot prove the
sentence about what you find there is still true.

**Second stale claim in the same entry, same class:** the cert specifies `Params` at **cbor key 19**.
Shipped is **key 20**; 19 is `SlashesDigest`. Same annotation.

---

## 4. The record says delivered where it is not

### 4.1 "settled 104 receipts" is a unit test, not a graded run

`CHANGELOG.md`'s sentence reads as a field observation. It is
`core/node/TestC2FrozenChainDoesNotStarveTheDeliveryLane`, and the `104` is `1040 s / 10 s`.
**No graded run has ever opened a delivery session at all** — row 13 on both reports has the client
refusing at the withdrawal, dark lane — and `FundDeliverySessionRemote` has zero non-test callers, so
the shipped binary contains no delivery-funding client to open one with.

**The canon refutation of `R-SESSION-WALLCLOCK-STEP` still stands**, and this correction does not
touch it: the reaper was armed and swept 104 times, and the chain-freeze arm is *asserted*, not
assumed. The `fund` arm is corroborating only. `m0.md` already cites the test by symbol and needs no
change. Only the CHANGELOG sentence's **tier** was wrong.

### 4.2 The relay lane's honest label

`docs/release-checklist.md`'s rule sentence is *"built, sim-proven, never exercised on a real
network."* For the paid relay lane that is wrong **in the over-claiming direction**: it implies an
operator could exercise it and no one has. Verified by the inert-mechanism sweep and confirmed here by
call-graph read — `OpenRelaySessionRemote`, `AcquireRelayAnchors`, `SubmitRelayPay` and
`DialThroughPaid` all have **zero non-test callers**, while the server half is live
(`(*Node).handleRelayOpen` / `(*Node).handleRelayPay` are dispatched).

The lane is *unexercised because it cannot be reached*, not because no one has tried.

**Keep the honest qualifier:** the server accepts `MsgRelayOpen` from any peer, so a third party could
hand-write a client. The accurate scope is *"the shipped binary has no client,"* not *"the protocol is
unreachable."* Over-correcting here would be the same failure with the sign flipped.

Two further sites carry the same sentence and are **owed to their owners, not edited here**
(file ownership — another builder holds `cmd/silt/daemon.go` this session):

- `cmd/silt/daemon.go`, the `-accept-relay-payments` flag help.
- `docs/design/pod.md` §7.3's **Field status** line.

---

## 5. Where we are, and where we are going

- **Boulder 3 is the whole remaining critical path:** D1 → D2 → D3.
- **D1's real remaining FORMAT set is small** — manifest item 1 (`tagRevLogSize`) and item 2 (the
  digest 5 → 3 retirement), plus F's wiring and call A. A blind audit classified all 22 manifest
  items; everything else is BUILT, DROPPED, or owed at the stamp raise
  (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era4-freeze-manifest-final-content-audit-0ed3b92-2026-09-10.md`).
- **The reachability headline is two numbers, not one:** 117 inert sites, of which **71 (61 %) are the
  floor-box / recompute keystone, inert by ratified owner direction** (`D-RECOMPUTE-FREEZE`). **46** is
  the non-frozen remainder, out of 1,784 exported declaration sites swept.
- **`docs/TENETS.md` needs no change.** Its corner-5 factual clause is true, and its guarantee is
  explicitly framed as a goal. Checked, not assumed; not edited.

---

## 6. Owner calls — two, because two have evidence

The cap is five and the rule is that a call appears **only** when its evidence exists, because a list
carrying not-yet-ready items trains the reader to skim. There are two.

1. **The genesis bind froze six compile-time defaults.** Driven: move the `-quorum` flag default
   3 → 2, rebuild, and an unchanged-argv restart refuses on the same chain. Correct under canon rule
   8 — but it promotes six **Evolving-tier** knobs to frozen per-network constants (`DerivedBondTTL`'s
   own comment, *"a real deployment can tighten it,"* becomes false for a launched network) and makes
   any future default change a **fleet-wide brick on upgrade**. `DerivedBondFloor` is CLEARED: a
   compile-time constant with no hardware-dependent divergence.
2. **How much of the `Atts`/QC program is RC-blocking.** The landing order has five steps and **step 2
   must land ≥ one deployment window after step 1** (version-skew stall), so it cannot ship in one
   release and cannot all be RC-blocking without splitting the RC. Steps 0 and 1 are cheap: step 0
   needs nothing, and step 1 discharges two gates in one commit.

---

## 7. What this change is NOT

No `.go` file, no `scripts/`, no `.github/`, no `.claude/`. No new `R-` prefix. No new era. No
mechanism proposed — three of the four new rows describe work whose *direction* is certified and whose
*build* is not this commit's business.
