# 2026-09-06 — The credit numéraire: λ re-denominated with the delivery price and the bounty base

**Decision.** Build the Researcher's certified minimal first step for G-R212-7 route (a), all five
owner calls ratified at the first option on 2026-09-06: `λ` = 1 credit per `Dλ = 393,216` bytes,
derived as `⌈3·U/(2·p)⌉` from the pinned delivery price `(U, p) = (262,144, 1)`; `RepairBountyBase =
c·k·shardBytes/(U/p)`; a per-lane byte-remainder accumulator. Certification:
`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/G-R212-7-lambda-redenomination-RESEARCH-CERTIFICATION-2026-09-06.md`. Economist advisory:
`/Users/andrewedmond/.claude/silt-agent-memory/economist/reviews/ADVISORY-G-R212-7-lambda-redenomination-2026-09-06.md`.

## Why this shape

- **One derivation direction.** Every byte→credit constant follows the price pair; nothing is pinned
  twice. The ordering `U/p < Dλ ≤ 524,288` is asserted by gates in `cmd/silt`, the one package
  that imports both `core/credit` and `core/relaypay`.
- **Two floors on the object path.** `net = ⌊7·acc/(8·Dλ)⌋`, `skim = ⌊acc/(8·Dλ)⌋`, each cumulative
  over the lane's bytes, so `reverseProvisional` reverses exact recorded amounts and the skim
  accumulates across serves. Correction recorded against the certification's G-λ-7 wording: the
  distinction "skim from the byte accumulator, not from the minted credits" is arithmetically vacuous
  (`⌊⌊a/Dλ⌋/8⌋ = ⌊a/(8Dλ)⌋`); the substantive requirement is CUMULATIVE accumulation on the lane,
  which is what the gate holds.
- **Remainder placement.** On the provisional lane for object-aware serves (dies at redeem and at
  eviction), on the account for the plain path (no lane, no supersede). G-λ-5 pins the double-pay
  path shut.

## Where the build departed from the certified text, and why

- **G-λ-8 "start-up refusal".** The daemon knows no chunk geometry at start; a refusal keyed on the
  sim default (64 KiB) would refuse every `-economy` daemon forever, including the e2e. Built form:
  the judge names a zero base loudly (`Stats.BountyBaseZero`, a WARN journal line with the fix) and
  the publish commands warn below `MinBountyChunkBytes`. The e2e economy flow publishes 256 KiB
  chunks, which pay a base of 1; the red-team repair fixtures moved from 4 KiB to 512 KiB chunks
  (one full k = 10 stripe) because 4 KiB shards now pay nothing.
- **Telemetry withheld from untokened readers.** `serveMint.skimmedCredits` is the sum of every
  object's funded skim, which on a one-object node IS the per-object figure red-team F2 withholds.
  Served to the token holder only, marker `serveMintWithheld`.

## Test migration

Thirteen credit gates, four node gates and nine command gates pinned one-credit-per-byte
arithmetic. They are re-expressed in **mint units** (`SkimDen·Dλ` bytes = 7 net + 1 skim on a lane;
`Dλ` bytes = 1 credit plain) so their discriminating power is unchanged: every conservation
oracle still counts credits minted and reversed exactly. The G-4 economist's number moved from
+58,676,506 to −43,601 (the R-FLAT-FEE flip the certification predicted), pinned as a literal.

## Owed

G-λ-11: the operator cost of the receipt lane (a measurement, before `PF` may move). G-R212-8 stays
R2.9's gate. `R-RELAY-ANON-SET′`, `R-NUMERAIRE-SANDWICH`, `R-AUDIT-REWARD-DOMINATES`,
`R-LAMBDA-WASH-MINT` in the residual backlog.

## Sim rescale

Three sim scenarios pinned byte-denominated credits or sub-boundary geometry: the freeloader economy
(64 KiB objects at a 50,000 fee — a host now needs 20.93 GiB of serving per token, so the sim uses
4 MiB single-chunk objects and a sim-scale fee of 8), the serve auto-skim (a 256 KiB object at 4 KiB
chunks now skims zero on every lane; it publishes 32 MiB in one chunk so each column lane carries a
3.2 MiB shard), and the repair bounty (4 KiB chunks pay a zero base; 256 KiB chunks at the same 128-chunk
stripe count). None of these is a mechanism change; each is the scenario reaching the boundary the
numéraire moved.

## Blind PE code ruling (MERGE-AFTER, six items) and what it corrected

Ruling: `/Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-G-R212-7-lambda-numeraire-code-2026-09-06.md`.
The arithmetic held under 2,000 randomized trials and eleven of twelve certified ablations reddened a
named gate. The misses were at the edges: the `silt sim run economy` CLI kept the production fee
default and demonstrated the opposite of its claim (fixed: `-fee 0` = scenario default); the
`serveMintWithheld` marker was ungated (gated); the telemetry reported the server leg only and was
silently gross of reversal (both legs + `reversedCredits`, documented); the serve-skim sim had been
rescaled into the one geometry where the accumulator is a no-op (now 512 KiB chunks, the ablation
reddens); eight stale text sites incl. the red-team F2 leak argument (rewritten: coarsened
3,145,728×, the per-object join is still the harm); the publish warning fired on every default
publish (now only for an explicit small `-chunk-size`, tested). Two residuals filed:
`R-DEFAULT-CHUNK-BOUNTY-ZERO` (owner call) and `R-LAMBDA-DUST′` (research-gated: 24 GiB vs 3 GiB).
