# 2026-09-09 — Lane C5 pre-flip closers: the bounty truncation gates, and the short final stripe

**Context / trigger:** ROADMAP Lane C5, the two CODE closers among the pre-flip items. `R-BOUNTY-TRUNCATION`
was GATED by the BOULDER-2 residual-closures certification
(`/Users/andrewedmond/.claude/silt-agent-memory/researcher/reviews/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md`,
§2.6) on two named gates, G-BT-1 and G-BT-2; the accumulator alternative is REFUTED there on build-immutable #8.
`R-SHORT-FINAL-STRIPE` is rider item 2 of the Economist's chunk-size advisory
(`/Users/andrewedmond/.claude/silt-agent-memory/economist/reviews/ADVISORY-default-chunk-size-256KiB-2026-09-06.md`, §4),
left open when 4′ folded in rider item 1 (true-length manifest framing, #765). Both land before the economy-ON
flip: the first because the flip arms the repair wage, the second because it changes content addressing.

---

## Item 1 — `R-BOUNTY-TRUNCATION`

**Evidence.** `RepairBountyBase` (`core/credit/escrow.go`) floors `c·k·shardBytes/(U/p)` and the judge then
multiplied the floored base by the rarest-shard multiplier (`core/node/repairclaim.go`). At `-chunk-size 52412`
the exact price is 1.99996 credits and the repairer is paid 1 — a 50 % wage cut — and `bountyChunkWarning`
(`cmd/silt/numeraire.go`) returned `""` for it, because it spoke only when the base hit ZERO. Measured on this
tree: `bountyChunkWarning(52_412, true) == ""` before this change.

**Options weighed.**

- **(A) A per-`(root, repairer)` accumulator, as on the serve lane.** REFUTED by the certification and not
  re-opened here: the serve accumulator was certified against a `mint = 0` on 100 % of serves regime, the
  bounty's failure is bounded by `1/(base+1)`, and the accumulator needs a map that must SURVIVE between repairs
  — an unbounded never-dying ledger surface, build-immutable #8, bought for a tenet-tier wage correction.
- **(B) Warn at publish (G-BT-1) + divide after the multiplier (G-BT-2).** Certified. No new state, no new
  surface. Chosen.
- **(C) Warn only.** Leaves free money on the table: G-BT-2 is strictly dominant by
  `⌊a⌋·m ≤ ⌊a·m⌋ ≤ a·m`, so it is never an over-pay and recovers up to `N−K+1 = 7×` of the truncation, most on
  the stripe nearest data loss. Rejected — a one-line arithmetic fix does not need a second PR.

**Decision: (B), both gates.**

**The threshold is derived, and the rule has a closed complement — over the PUBLISH, not the flag.** The
certification asked for a threshold "derived from the arithmetic, never typed". `shippedBountyBase()` is
`RepairBountyBase(K, DefaultChunkSize + Overhead)` — the base the shipped default pays on a full frame, 10
today. The rule is then: **warn iff this publish's real repair-bounty base is below that.**

The first cut of this scoped the rule to the chunk size and claimed the complement was closed. It was closed
over *geometry* and open over *objects*, and the blind PE caught it (B-3): after half 2 a 1 KB and a 10 KB
object at the shipped default both pay a real base of ZERO and the warning said nothing — the diff's own second
half created an object class its first half could not see. The rule now prices the shard the publish will
really store, via `pipeline.DataFrameSize` (the same rule `splitFile` implements, read rather than restated),
so the two causes are:

| cause | the operator can act on it by | at the shipped default |
|---|---|---|
| the chunk size they chose | raising `-chunk-size` | anything below 262,128 B truncates; below 26,199 B pays zero |
| the OBJECT's own size | nothing — the shard IS the object | every object ≤ 26,190 B pays ZERO; ≤ 262,119 B pays under 10 |

Extending it *simplified* the rule rather than complicating it: the `explicit` flag-was-set parameter is now
DEAD and is deleted, along with `flagWasSet`. An unset `-chunk-size` is `DefaultChunkSize` by construction, so
the GEOMETRY cause cannot fire without a flag — and that is the whole of what "unset" buys. The first version
of this paragraph went on to conclude that the M6 "don't warn on every default publish" concern was therefore
"satisfied structurally", one line below a table that says otherwise. It is **not satisfied. It is TRADED**,
deliberately and on the PE's own B-3, because after half 2 the object is what pays. Measured at the default
with no flag set:

| object | verdict |
|---|---|
| 1,024 B | FIRES (ZERO) |
| 26,190 B | FIRES (ZERO) |
| 100,000 B | FIRES (TRUNCATES 21.4 %) |
| 262,119 B | FIRES (TRUNCATES 10.0 %) |
| **262,120 B** | **first SILENT size** |
| 1,500,000 B | SILENT |

**Every object of 262,119 B or less warns on a default publish.** That is the intended behaviour and it has an
operational cost worth stating: several integration harnesses publish 32–65 KB fixtures at the default and now
emit a line they did not (verified non-verdict-changing — the PE matched all 125 `grep` patterns in
`integration/` and `e2e/` against all ten warning strings; four matched, none in a verdict path).

A source gate pins that the flag's default really is `DefaultChunkSize`, because the runtime table passes the
constant as a literal and never reads the flag, so the source string is the only thing connecting the two.
Both sides and both causes are driven in
`TestGLambda8PublishWarningFiresOnlyWhenThePublishShortPaysTheRepairer`.

**The printed figures are integer arithmetic.** `credit.RepairBountyTruncation` returns the exact price floored
at 1e-5 credits and the under-pay in tenths of one percent, both computed in `int64`. The money path does no
floating point, and the printed price is floored in the same direction as the payment, so it can never
over-state what the repairer is owed. At the certification's case that renders exactly its own figures: base 1,
exact `1.99996`, `50.0 %`.

**One behaviour change worth naming.** Because the division is now last, the rarest-shard multiplier can lift a
ZERO-base geometry to a non-zero payment (`k = 10`, 6,553-byte shards, a stripe at the k-floor: 0 → 1). That is
the honest price of the work, and it is exactly why the G-λ-8 zero-signal must keep reading the UNMULTIPLIED
base: reading the paid price would make the signal vacuous on the rarest stripes, which are the ones that matter
most. `TestGLambda8ZeroBountyBaseIsNamedNotSilent` now drives both arms — a geometry that pays nothing at any
multiplier, and one the multiplier lifts — and asserts the counter fires on both.

**Retired, not kept beside its replacement.** `credit.BountyFor` (multiplier over an already-floored base) is
DELETED rather than left exported next to `credit.RepairBounty`. Two ways to price a repair in one money path is
the "two places, one rule" trap; the design doc sentences that named it (`docs/design/h7-proof-of-repair.md`)
are corrected in the same change.

---

## Item 2 — `R-SHORT-FINAL-STRIPE`, a content-addressing break

**Evidence.** `chunk.Split` frames every chunk to exactly `chunkSize`, so a 1 KB object at the 256 KiB default
stored one 262,160-byte data shard plus six identical parity shards — 1,835,120 bytes of mostly zeros for
1,024 bytes of content. Measured on this tree, whole object including the manifest: **1,835,465 B before,
7,681 B after** (239×). A 100 KB object: 1,835,465 → 700,517 (2.6×). The 1.5 MB flixz modal object:
**3,146,439 B, unchanged.**

**The refutation that had to be revisited.** The 4′ deliberation
(`docs/thinking/2026-09-07-default-chunk-256k-manifest-framing.md`, option C) refused this with "erasure shards
must be equal-length within a stripe". That is true **within a stripe of two or more real chunks** and does not
reach a single-frame object, which is alone in its stripe: `erasure.EncodeStripe` sizes the implicit zero shards
and the parity from `len(data[0])`, whatever that is. The over-broad ground is corrected here.

**The load-bearing constraint the advisory did not state.** The PoR auditor fixes the challenge sample space
itself — `want = por.DefaultParams.Blocks(int(m.ChunkSize) + ctOverhead)`, demanded EXACTLY of every leaf
(`core/node/por.go`, `blocksOK`) — because letting a prover self-report its block count is red-team finding F4
(keep block 0, report `PorBlocks = 1`, pass while holding a sliver). A short shard under an unchanged
`m.ChunkSize` therefore does not fail loudly: the auditor demands 65 blocks of a 1,048-byte shard, every honest
holder is refused, and every sub-frame object silently reads as lost.

**Options weighed.**

- **(A) Carry the true frame length in a NEW committed manifest field.** A manifest FORMAT change, and it would
  move the object's exact size out of the secrets box into the layout. Rejected, and it is also the STOP
  condition this task was given.
- **(B) Let the prover report its own block count for short shards.** Re-opens F4. Refused outright.
- **(C) Commit the frame size that was actually USED, in the existing `manifest.ChunkSize`.** The field is
  already documented as "frame size used at split time", is already inside the layout (so the auditor, the
  repair judge and the reader all read it), and is already authenticated under the layout key. No format change,
  no new field, no new trust. F4 stays closed because the auditor still fixes the sample space from committed
  data. Chosen.

**Decision: (C).** `splitFile` is `chunk.Split` with one exception — an object whose whole content fits in a
single frame is framed at its true length — and `Stage` reads the geometry off the artifact
(`frameBytes = len(frames[0])`) rather than re-deciding it.

### Blast radius, stated exactly

| | |
|---|---|
| **Re-addresses** | Every object whose content fits in one frame: **`size ≤ chunkSize − 9`** (262,135 B at the default). Its data and parity chunk IDs, root, link key and manifest all change. The first statement of this said `− 8` and was wrong by one byte (blind PE B-1, measured at `chunkSize = 4096`: size 4088 is byte-identical, 4087 moves) — an object of exactly `chunkSize − 8` FILLS the first frame, so `io.ReadFull` returns a nil error and `splitFile` takes the pass-through branch. Both sides are now driven by `TestDataFrameSizeIsWhatStageCommits`. |
| **Byte-identical** | Every object with two or more frames — they share a stripe, the tail stays padded, `chunk.Split` does the whole job. Gated by `TestMultiFrameObjectsKeepThePaddedTail` and measured on the 1.5 MB modal object. |
| **Manifest format** | UNCHANGED. No new field, no new CBOR key. The frame length rides in the existing `ChunkSize`. |
| **Erasure geometry** | UNCHANGED for every object. Only the shard LENGTH of a single-frame stripe moves. |
| **An existing store** | Keeps working. Nothing on the read path consults `DefaultChunkSize`; readers take the geometry from the manifest and the frame headers, so old objects still fetch, audit and repair exactly as before under their own committed `ChunkSize`. Re-publishing the same bytes yields a NEW root, so dedup does not span the boundary and the store holds two copies of anything re-published. |
| **The genesis** | **MOVES.** The 2,042-byte manifesto is a single-frame object. root `fce9eeeb…20d6` → `31768fb4…7dd1`, manifest chunk `5478750c…d107` → `f761f80b…fcf6`, block hash `f428d0a8…0951` → `e44344ea…72c0`. Unlike 4′ the ROOT moves too, because this re-frames the data chunks the root is built from. |

The genesis move is the **second** in this window and rests on the same ground the owner accepted for the first
(4′, 2026-09-07): no live network exists and every development chain is wiped on upgrade. It is owed the same
explicit acceptance; `TestGenesisBlockHashIsPinned` holds all three literals so it cannot drift unannounced.

### Four consequences the certification and the advisory did not price

**1. A sub-frame object's durability becomes PREPAY-ONLY. This is the correction that matters most.**
Its repair bounty base is now zero — its shard is ~1 KB, so `k·shardBytes` is far below one credit of fetch.
The first version of this document said such objects are "kept alive by the serve economy rather than by
bounties". That is **measured false** and the blind PE caught it (B-2). `RecordServeToObject` accumulates the
skim on a per-`(server, requester, root)` LANE (`core/credit/escrow.go`), and a published small object is
served once each to many *different* fetchers, so no lane ever reaches a credit:

| shard | same-lane serves to the first escrow credit | escrow after 5,000 serves over 250 distinct fetchers |
|---|---|---|
| 262,160 B (padded, before) | 12 | **250** |
| 1,048 B (short frame, after) | 3,002 | **0** |

Measured on the shipped ledger and now driven by `TestSubFrameObjectDurabilityIsPrepayOnly`, so the sentence
cannot rot back. Zero bounty *and* zero skim: a sub-frame object's durability is funded by publisher prepay
alone. The direction is still defensible — the old inflow billed for moving padding, and repairing a 1 KB
object really does cost about 10 KB of fetch, so the old 10-credit bounty was a ~250× over-pay — but the honest
statement is "prepay-only", not "serve-funded", and it is the statement the owner reads when accepting the
break.

It is **not silent**: the judge counts `Stats.BountyBaseZero` and journals a WARN on every such settlement, and
since B-3 the publisher is told at publish time too, which matters because the judge is a caretaker the
publisher neither runs nor sees.

**And this is the coupling neither the certification nor I saw: half 2 of this diff pushes a whole object class
into exactly the integer-truncation regime half 1 exists to warn about, and on the repair lane the mitigation —
an accumulator — is already REFUTED on build-immutable #8.** That framing, not the storage saving, is what the
Researcher has to price. It gates the economy-ON flip (C6), not this change, because `-economy` is off by
default.

**2. Threat F3 gains an exact byte-length oracle for sub-frame objects.** `docs/threat-catalog.md` already
records timing/size fingerprinting as unmitigated and silt claims no size hiding anywhere. Still, the padding
*did* blur a sub-frame object's true size to "somewhere in one chunk"; now any holder reads its exact length
from the stored shard and any caretaker from the layout's `ChunkSize`. Characterising this as "slight" in the
first cut was wrong: it is an exact oracle over the whole class ≤ `chunkSize − 9`.

**3. Convergent dedup for sub-frame objects now spans `-chunk-size`, and chunk size stops being an accidental
salt.** Measured: the same 4,096-byte payload staged in convergent mode at 64 KiB, 256 KiB and 1 MiB now yields
ONE root; before, the padded ciphertext made the chunk ID — and so the root and the link key — depend on the
publisher's geometry. Dedup improves, and the F6 confirmation attack that `cmd/silt` already warns about gets
cheaper by exactly the same step: a guesser needs the plaintext alone rather than the plaintext AND the
geometry. Bounded to sub-frame objects — a 100,000-byte multi-frame object still yields different roots at
different chunk sizes, and `TestConvergentDedupNowSpansTheChunkSize` drives both sides.

**Disposition of 2 and 3: recorded, not mitigated.** Both are written into `docs/threat-catalog.md` under the
existing F3 entry as a catalog update. No mitigation is attempted and none should be attempted here — a
red-team pass is owed on this surface, and inventing a salt without one would be exactly the unreviewed novelty
B8 forbids.

**4. `manifest.ChunkSize` can now legitimately be as small as 9** (a 1-byte object). Any future consumer that
divides by the field or assumes a floor inherits that; said in `splitFile`'s comment where such a consumer
would read it.

---

## Gates and their ablations

| Gate | Where | Ablation that must go RED |
|---|---|---|
| G-BT-1, both sides and both causes | `cmd/silt/g_lambda_test.go` `TestGLambda8PublishWarningFiresOnlyWhenThePublishShortPaysTheRepairer` | (a) Restore the zero-only rule ⇒ `bountyPriceWarning(52_412, unknown)` returns `""`; (b) price on the chunk size alone ⇒ a 1 KB object at the default says nothing while paying zero |
| The prepay-only consequence | `core/credit/g_bt2_test.go` `TestSubFrameObjectDurabilityIsPrepayOnly` | Skim on a per-account rather than per-lane accumulator ⇒ the 250-fetcher spread stops reading 0 |
| The re-addressing boundary | `core/pipeline/short_final_stripe_test.go` `TestDataFrameSizeIsWhatStageCommits` | Shift the rule one byte (`fs+1 < chunkSize`) ⇒ "size 4087: Stage committed ChunkSize 4096, want 4095" |
| The dedup/salt finding | `core/pipeline/short_final_stripe_test.go` `TestConvergentDedupNowSpansTheChunkSize` | Restore the padded frame ⇒ three chunk sizes give three roots, which is the measurement that the salt WAS there |
| The empty object | both gates above | Restore `DataFrameSize(0) = HeaderSize` ⇒ "size 0: DataFrameSize says 8, Stage commits 4096", and at the CLI seam an empty file warns about a 24-byte shard that does not exist |

**One note on how the boundary is gated, because it is not gated the way it looks.** `fs < chunkSize` and
`fs <= chunkSize` are the SAME function: at `fs == chunkSize` both branches return `chunkSize`. So the rule has
no off-by-one to mutate at its own boundary — the error was never in the code, it was in the sentence, which
nothing executed. The gate is therefore a table of LITERAL expected frame sizes (4087 → 4095, 4088 → 4096) that
both `Stage` and `DataFrameSize` are checked against, rather than the test calling the function under test and
agreeing with itself. That is the standing lesson applied one level up: a published sentence is an assertion,
and the fix is to run it.
| G-BT-2, the arithmetic | `core/credit/g_bt2_test.go` | Divide before the multiplier ⇒ 4 instead of 7; and the zero-signal case collapses to 0 |
| G-BT-2, at the judge | `core/node/g_bt2_judge_test.go` | Settle with the floor-first price ⇒ the judge pays 4, not 7 |
| The short frame | `core/pipeline/short_final_stripe_test.go` | Split at `opts.ChunkSize` again ⇒ the 1 KB object commits a 262,144-byte geometry |
| The audit coupling | `core/node/g_shortstripe_audit_test.go` | Commit `opts.ChunkSize` instead of the frame used ⇒ the auditor sizes a 1,048-byte shard at 262,160 and every honest holder fails |

## What is NOT in this change

- The accumulator (REFUTED, build-immutable #8).
- `R-ANCHOR-BEARER-TRANSFER`, the third C5 item — a red-team pass, not a build.
- The corrected `pod.md:417-421` / `relayrole.go:34-38` relay-anonymity sentence the same certification owes
  (§4). It belongs to `R-RELAY-ANON-SET`, not to either closer here.

---

## Through-line — the same defect, twice, and what it says about where to look

Both review rounds on this branch found the same thing, and neither found it in the code.

| round | the code | the published sentence |
|---|---|---|
| B-1 | correct. `fs < chunkSize` and `fs <= chunkSize` are the same function, so the mutation is a genuine no-op and the off-by-one could not live here | wrong: "`≤ chunkSize − 8`" |
| R-1 | correct. An unset flag really does close the geometry cause | wrong: "so it can only reach the silent side", contradicted by the table one line above it and by the branch's own gate |

Every gate was green through both. That is the signal, not the exception: **a gate checks the code, and
nothing checks the sentence.** The sentence is what the owner reads to accept a content-addressing break, so
on this branch it was the higher-risk artifact of the two.

Three working rules come out of it, and they generalise past this change:

1. **A claim about a boundary is a measurement.** Do not derive it from reading the comparison — publish the
   endpoints you actually ran. Both errors were one step of arithmetic away from the truth and both survived
   re-reading.
2. **Where the code cannot carry the error, the gate must carry the sentence.** A boundary that is a fixed
   point has no mutation to ablate, so the gate is a table of literal expected values (4087 → 4095,
   4088 → 4096, 0 → 4096) that both the helper and the real path are checked against. Never
   `want := TheFunctionUnderTest(...)`.
3. **An excused row is where the hole hides.** `if c.size > 0` skipped the one input on which the helper and
   `Stage` disagreed, and the gate stayed green over a live defect — `silt add` on an empty file priced a
   24-byte shard that is never stored. The row is now driven, not excused. (The same lesson as the previous
   branch's meta-test excuse row, one tier down.)

The related trap this branch also hit, from the same family: a *counter-argument* is a claim too. "Sub-frame
objects are funded by the serve economy instead" was a sentence nobody ran, and running it returned zero.
