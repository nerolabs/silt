# F1 — building the repair-bounty re-pricing (2026-09-12)

**Decision record:** `docs/decisions.md` `D-BOUNTY-PRICE-F1-2026-09-12`. This file records the
*build* deliberation: the options I weighed, the three things I refused, and the two places I
corrected the authority rather than transcribing it.

The value was not mine to choose — `c·k = 1` falls out of a floor meeting a ceiling. What was mine
was the shape of the encoding, where the refusal lives, what to do about the machinery the
re-pricing strands, and what to defer.

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
2. **Widen the rule to "warn on any truncation".** Rejected, and not mine: it would speak on the
   shipped default itself (exact 1.00006), which is the finding blind PE M6 closed.
3. **Keep it, run the consequence, file the gap.** Taken.

Option 3 costs one honest sentence in the operator-facing doc and one new residual,
`R-TRUNCATION-DISCLOSURE-NARROWS`. **Neither certification named this.** A publish worth an exact
1.99996 credits now pays 1 and is silent — a 50 % short-pay the publisher is never told about. The
G-λ-8 gate runs both halves: every firing publish takes the ZERO arm, and the 50 % geometry is
silent. A cost that is run cannot rot into a sentence.

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
