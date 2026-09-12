# F1 — building the repair-bounty re-pricing (2026-09-12)

**Decision record:** `docs/decisions.md` `D-BOUNTY-PRICE-F1-2026-09-12`. This file records the
*build* deliberation: the options I weighed, the three things I refused, and the two places I
corrected the authority rather than transcribing it.

The value was not mine to choose — `c·k = 1` falls out of a floor meeting a ceiling. What was mine
was the shape of the encoding, where the refusal lives, what to do about the machinery the
re-pricing strands, and what to defer.

**The qualifier that must travel with that first sentence (added 2026-09-12 after the blind
review):** the floor meets the ceiling **for the BASE**. The disbursed price is
`base × RarestShardMultiplier`, the multiplier runs to `n − k + 1`, and measured against the bytes
the payee itself moves it reaches **7× at the shipped `k = 10, n = 16`** and 33× at `k = 1, n = 33`.
F1 improves that — the same path reached 70× before — and the multiplier is separately ratified and
untouched here. But *"no value is chosen"* is a claim about the base and nothing above it, and it was
about to enter the changelog and the website without saying so.

## 1. The encoding — two candidates, and the cheap one is wrong

| Option | Cost | Verdict |
|---|---|---|
| `RepairBountyCoeffNum/Den = 1/10` | one constant | **REFUSED.** Right today, wrong as a mechanism |
| take `k` out of the byte quantity, keep `c = 1` | one expression + a gate | **TAKEN** |

The certification refuses `1/10` on two grounds. **One of them is false and I did not repeat it.**

- **True and decisive:** `1/10` hard-codes `k = 10` into a price that must not move with `k`, and
  `k` is Evolving-tier. Driven in `TestF1PriceCarriesNoK`: at `k = 12` the `1/10` encoding pays 12
  where F1 pays 10 — a 20 % silent over-pay.
- **False:** *"it adds a SECOND integer floor before the last division … lossless only because `k`
  divides `Den`."* For positive integers `⌊⌊x/a⌋/b⌋ = ⌊x/(a·b)⌋` **always**. Nested integer
  division loses nothing, and the divisibility is irrelevant. The gate's third arm drives the
  identity so the correction is a run, not a sentence.

The directive survives its broken reason, so this is a correction filed with the build, not a stop.

## 2. Where the shape is held: a gate, not a signature

`k` is now inert in the price. The clean expression of that is to drop the parameter — a signature
you cannot re-couple through. **I could not.** `core/node/repairclaim.go` calls both
`RepairBountyBase(p.K, …)` and `MinBountyChunkBytesFor(p.K, …)`, and that file is under review in
PR #847. Changing the signatures would either break the build or fork a file another PR owns.

So the property is held by measurement instead: `TestF1PriceCarriesNoK` sweeps `k = 1…64` and fails
if the price moves. That is weaker against a *deliberate* re-coupling and stronger against an
*accidental* one — it also catches `1/10`, which a signature change would not. The parameter removal
is owed with the doc sweep.

## 3. The machinery the re-pricing strands — disclose, do not delete

The publish warning fires iff `base < shippedBountyBase()`. That threshold is DERIVED and falls
10 → 1 with the price, so a firing publish now always has `base == 0` and **the TRUNCATES arm is
unreachable through `bountyPriceWarning`**. Three options:

1. **Delete the arm and `RepairBountyTruncation`.** Rejected: it removes G-BT-1's arithmetic gate,
   and the threshold is derived from two Evolving-tier constants, either of which revives the arm.
2. **Widen the rule to "warn on any truncation".** Rejected, and not mine.
3. **Keep it, run the consequence, file the gap.** Taken.

**My stated reason for rejecting option 2 was WRONG, and the blind review measured it (2026-09-12).**
I wrote that widening *"would speak on the shipped default itself (exact 1.00006), which is the
finding blind PE M6 closed."* The shipped function refutes that: `RepairBountyTruncation` returns
**0 tenths of a percent at both 262,144 B and 262,128 B**, against 200 / 333 / 500 at the loud
geometries, so a `lossTenths >= 10` (1 %) OR-clause is silent on the default AND on the minimum
chunk. **The verdict survives its broken reason, and the real reason is different:** the rule's one
structural property is a CLOSED COMPLEMENT — silent in exactly one stated case — and an OR-clause
breaks it. That makes widening an owner DECISION, not a defect fix, which is why it is still not
taken here.

**And I understated the cost.** I filed the 1.99996 anecdote. The measured band is that the maximum
**silent** repair-wage short-pay rises **5.5×, from 9.09 % to 50.0 %** (pre-F1 `base >= 10` bounds the
loss at 1/11; post-F1 `base >= 1` bounds it at 1/2), reachable at an ordinary operator choice —
`-chunk-size 393216` goes **0.00 % → 33.3 %**, 327,680 goes 4.00 % → 20.0 %. Pre-F1 the warning
covered exactly the large-loss region; post-F1 that whole region is silent.
`R-TRUNCATION-DISCLOSURE-NARROWS` is re-filed at that cost with the broken reason withdrawn. The
G-λ-8 gate runs both halves: every firing publish takes the ZERO arm, and the 50 % geometry is
silent. A cost that is run cannot rot into a sentence — but a REASON that is only reasoned rots
anyway, which is the lesson here.

## 4. The refuse-to-start, and why it is not a compile-time guard

`cmd/silt/numeraire.go` already carries the stronger pattern — a `const _ = uint(...)` block that
fails to BUILD on a violated relation, used for the delivery idle window. I wanted that here. I
could not use it: the threshold comes from `credit.MinBountyChunkBytesFor`, a **function**, so it is
not a constant expression.

The refusal is therefore a runtime check at daemon start, which is canon rule 8's first arm and what
the certification asked for. Three deliberate properties:

- **Unconditional, not gated on `-economy`.** The publisher needs the disclosure whether or not this
  node pays bounties, and `silt add` has no economy flag at all.
- **It takes its two inputs as parameters**, so both arms are drivable. A check that could only be
  called with the shipped constants would be untestable in the direction that matters.
- **A source gate pins the daemon's call**, because the two arms are pure-function cover and nothing
  else connects the check to the shipped binary. Ablations, all three RED independently: put `k`
  back in the product (5 gates RED); neuter the refusal (the refusal arm); perturb the daemon call
  string (the source arm).

## 5. What I deferred, and said so

- The 14 sites carrying the false *"fetches k survivors"* sentence — their own sweep, two of them
  operator-facing.
- `core/node/node.go`'s `Config.RepairEconomy` doc, which still states the pre-F1 formula. It is in
  PR #847's file set. **The operator-facing sentence — the `-economy` flag help — is corrected here**,
  because a stale code comment is a smaller debt than a published price an operator reads.
- The `k` parameter removal (§2).

## 6. Where the record was right and I checked anyway

Every number in the decision entry was re-derived at source rather than copied. All of them held:
the zero-class boundaries (26,190/26,191 and 262,119/262,120), the 10.008× widening, the invariant
warning boundary with a changed arm, the 235,945 B → 16 B margin, and the 36.00 → 3.60 solvency
band. `core/genesis` still has zero references to `core/credit`; `core/chain` reaches it only in
`bond_quorum_test.go`. The genesis hash does not move, and `TestGenesisBlockHashIsPinned` says so.

## 7. What the blind review found that I did not — and the one I most needed told

`docs/decisions.md` `D-BOUNTY-PRICE-F1-2026-09-12` §9 carries the full fold-in. Two of the six are
worth recording as build lessons rather than record corrections.

**I derived a price from a premise I never ran against the code.** I wrote that the bounty goes to
the new holder *"never to the reconstructor, which is unpaid by ratified design"* — and
`core/node/repair.go` says the opposite in its own words, tried BEFORE remote placement:
`selfHoldEligible` lets the paramedic keep the shard it rebuilt and be the payee. On that path the
payee moved `k` survivor shards and F1 pays it for one, so **F1's own floor fails there by a factor
of `k`**. I had read the certification that found this; I quoted its conclusion and not its
exception. **A premise sentence in shipped code is a claim, and it decays exactly like a cited test
name — the fix is to grep the code the premise is about, not to re-read the document that states
it.** The price does not move for it: that is research-gated. What I could do — correct the sentence,
name the mechanism, roster it — is done.

**A gate whose docstring names a property it does not touch is vacuous, and the ablation is the only
way to know.** `TestF1SolvencyBandIsExact` pinned four constants and passed under every price
ablation while claiming the F1 threshold. I ran ablations for the gates I thought were load-bearing
and not for the one I thought was a pin. **The tell was in the gate's own body: it never called the
function whose change the PR is.** It now derives the outflow through `repairBountyCredits` and goes
RED with `k` restored to the product.
