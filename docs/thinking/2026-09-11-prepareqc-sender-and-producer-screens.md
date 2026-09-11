# The prepare-QC sender screen and the producer screen — steps 0 and 1

*2026-09-11 · builder · `R-CARRIER-ATTS-PREPAREQC` steps 0 and 1 · certification verdict GATED*

Source:
`~/.claude/silt-agent-memory/researcher/reviews/research-outcome/R-CARRIER-ATTS-PREPAREQC-wire-bound-on-env-QC-RESEARCH-CERTIFICATION-2026-09-10.md`

## The mechanism, in one paragraph

`(*Node).handleChain`'s `ports.MsgPrepareQC` arm passes the sender's `env.QC` from
`cbor.Unmarshal` into `(*Chain).VerifyPrepareQC` → `(*Chain).collectQuorumSigs`, which runs one
`ed25519.Verify` per entry. `collectQuorumSigs` sets `seen[id]` **after** the qualification test,
so byte-identical entries are never deduplicated. One offline `ed25519.Sign` over a block the
attacker chooses itself, replayed to the CBOR decoder's `defaultMaxArrayElements` ceiling of
131,072, buys 131,072 verifies — 4.19–6.89 s of one core for 13.1 MiB of wire, from **any** peer,
at line rate. The arm has no sender screen, no rate budget and no length check. Two changes
address it: a **sender screen** at the arm (step 0) and a **producer screen** in
`(*Node).gatherTwoPhase` (step 1), which is the prerequisite for any bound expressed over
committed quantities.

## What ships, and what deliberately does not

| | ships | why |
|---|---|---|
| sender screen at `ports.MsgPrepareQC` | **yes** | free, honest-safe, reduces the attacker set from *any peer* to *a bonded, slashable validator* — the load-bearing half (certification CORRECTION 3) |
| `allowWindowed` budget at that arm | **no** | see below |
| producer screen in `gatherTwoPhase` | **yes** | one commit discharges G-QC-1 and `R-CB-ATTS-UNBOUNDED`'s G-ATTS-1 |
| the cap inside `VerifyPrepareQC` | **no** | step 2; must land ≥ one deployment window after step 1 (§8) |
| `(*Node).adoptLock` normalization | **no** | step 2 |

### Why the rate budget is not in this commit

The certification's step 0 is "sender screen **+** `allowWindowed` budget". The screen ships;
the budget does not. `allowWindowed` cannot be wired without a burst constant, and the same
certification marks that constant **UNSETTLED**:

> **G-QC-6.** `prepareQCBurst` must be justified against a **measured** honest cadence […]
> **UNSETTLED — needs a run.** Do not transcribe `roundCertBurst`; that constant's derivation
> ("one certificate per round, rounds ≥ 2 sweeps apart") **does not hold here**, because one
> proposer can gather many heights per window.

Residual `R-CARRIER-QC-BURST-VALUE` names it a **security parameter**, owner: tester, closer: a
cadence probe. Security parameters a proof depends on are behind the research gate; a builder does
not mint one. I confirmed the derivation genuinely does not transfer: `(*Node).ProposeEntry` has no
in-flight latch, so a busy publisher legitimately emits many prepare-QCs per `ChainSyncInterval`.
Shipping a guessed burst would be a throughput cliff on exactly the client-publish path this work
is trying to make boundable.

**Consequence, stated plainly:** after this commit the flood is still available to a bonded
validator. What is closed is the *unbonded* flood, which was the whole primitive's value — no
keypair farm, no bond, no coalition, nothing staked.

## Step 0 — the sender screen

```go
if n.chain.Objective() && !n.chain.AttesterEligibleAt(from, n.roundsFor().Height) { refuse }
```

Placed before `cbor.Unmarshal`, so a refusal costs a map lookup. Same shape as G-H43-12's rate
gate on the `ports.MsgRoundCert` arm one case-label below; same predicate
`(*Node).broadcastRoundCert` already applies on the send side.

**Honest-safety.** An honest prepare-QC arrives from the block's own author, who has already
passed this receiver's *stronger* `proposerQualifiedAt` test at the prepare phase — in objective
mode `proposerQualifiedAt ⇒ attesterQualifiedAt` branch by branch. The exception is a forced
re-proposal, where the sender is the round's designee and the author a third party; the designee
is drawn from `EligibleProposers()` by construction. Driven as **G-QC-4**.

**The refusal coupling, checked before wiring anything.** The PE ruling that `allowWindowed` on
`MsgChallenge` would defund honest holders does **not** transfer. Traced at the artifact:
`ports.MsgPrecommitReply` is produced only by this arm and consumed only by
`(*Node).gatherTwoPhase`'s `gatherPrecommits` callback, whose `case !resp.OK:` arm logs at
`LogDebug` and does nothing else. `credit.(*Ledger).RecordAudit` — the −25,000 / `auditsFailed`
path — has exactly one non-test caller, `core/node/por.go`'s `gradeAnswers`. No reputation setter
exists on the node or chain for peer behaviour. **A refused prepare-QC costs the sender no
standing, no credit and no reputation.** The only cost is liveness: the gatherer loses that
precommit, which is why the screen must be honest-safe, not why it must be avoided.

**The `Objective()` guard.** `(*Chain).attesterQualifiedAt` falls through to
`rep >= MinAttesterRep` in a trusted/demo posture, which a fresh honest peer fails. An unguarded
screen halts such a network. The guard leaves `R-CARRIER-QC-LEGACY-UNCAPPED` open, which is the
certification's own disposition (§4.3) — and is now a **driven probe**
(`TestQC_LegacyPostureKeepsThePrimitive_Driven`) rather than a sentence, per simplicity rule 7.

## Step 1 — the producer screen

```go
if n.chain.Objective() {
    for _, v := range attesters {
        if v == n.id || n.chain.AttesterEligibleAt(v, b.Height) { screened = append(screened, v) }
    }
    attesters = screened
}
```

A new slice: `chainhost.Host.Attesters` is shared config and must never be mutated.

**Honest-safety is an identity, not an estimate.** The predicate, the height and the node are the
same ones this gather's own stop condition uses. `supportMet` → `(*Chain).SupportMeetsQuorum`
skips every id failing `attesterQualifiedAt(id, b.Height)`, and `(*Chain).collectQuorumSigs`
applies that identical test at `Append`. A peer removed here could never have moved `supportMet`,
and `supportMet` is the conjunct that decides completion.

**`GoverningSetCap() == 0` in a trusted/demo posture — how it is handled.** The screen never reads
`GoverningSetCap()`. It reads `AttesterEligibleAt`, the predicate the cap is an upper bound *of*,
so the zero is not a hazard at this site. The cap's zero is a step-2 problem. The one adjacent
hazard — an objective chain with no anchors and no bonds, where the screen empties the
solicitation set — loses nothing, because with `|Q| = 0` the gather cannot satisfy `supportMet`
today either.

And the doc premise that produced the question is repaired in this commit rather than deferred to
step 2: `(*Chain).GoverningSetCap`'s comment asserted "every round path gates on Objective" and
"no round machinery runs there". Both are false — `(*Node).gatherTwoPhase` has no `Objective()`
guard and `(*Node).proposeBlock` stamps `BlockVersionRounds` unconditionally. A false sentence
about what a gate covers decays exactly like a cited test name; leaving it in for a release while
step 2 waits is how the next caller inherits it.

## Gates, and what each ablation showed

| gate | RED before | ablation that reproduces the RED |
|---|---|---|
| **G-QC-0** | an identity with no seat and no bond made a validator verify a full QC and **adopt the lock** | A (over-broad screen) also reds its control arm |
| **G-QC-1** | prepare-QC of **9** entries against a ceiling of 6 (`1 + 6 outsiders + 2`); after: 3 | C (producer screen disabled) → 9 again |
| **G-QC-2 / G-QC-4** | green at HEAD by construction; A (screen refuses everyone) → **RED** | A |
| **legacy residual probe** | green at HEAD; B (`Objective()` guard removed) → **RED** | B |

Every ablation was `diff`-verified against the pristine file before the run
(`silt-ablation-noop-guard`).

## Three things wrong at source, reported not worked around

1. **The `+2` derivation names the wrong two entries.** The certification says the two exempt
   attestations are "the gatherer's own prepare and the `carried` author self-prepare", both
   skipped by `id == b.ProposerID()`. On a forced re-proposal the gatherer is **not** the author,
   so its own seed **is** counted in `seen`. The two entries actually exempt are the *carried
   author self-prepare* and the *live author's own reply*. The count is still ≤ 2 — `carried`
   lifts at most one (`break` on first match) and the author answers at most one solicitation — so
   `+2` stands, but the reason is different. `G-QC-2` asserts the exempt **count**, which is what
   the bound is made of, rather than the size comparison.

2. **§4.1's arithmetic over-counts.** "1 + 1 + 3 (replies) = 5 > 4" assumes all three
   non-gatherer anchors reply. Driven, the 4-anchor launch network produces **4** — exactly
   `GoverningSetCap()`, not one above it — because `finishPrep` closes on the first reply that
   satisfies `supportMet` (`requiredLaunchAnchors()` = 3 with the author counted, so `|seen| = 2`
   suffices). The conclusion is unchanged and arguably sharper: a raw cap has **zero slack** on
   the launch regime, so the `+2` is required, not prudent.

3. **The certification path in the brief is off by a day.** The file is
   `…-RESEARCH-CERTIFICATION-2026-09-10.md`, not `-2026-09-11.md`.

## Owed, and explicitly not built

- **G-QC-2's refuse arm** (ceiling+1 refused with `ErrPrepareQCOverCap` and zero verifies) — needs
  the cap, which is step 2.
- **G-QC-5** — `R-CARRIER-QC-NESTED-ROUNDCHANGE`, routed **ahead of** the cap and not in steps 0–1.
- **G-QC-6 / `R-CARRIER-QC-BURST-VALUE`** — the rate budget, blocked on a measured cadence.
- **G-QC-3**, `(*Node).adoptLock` normalization — step 2.
