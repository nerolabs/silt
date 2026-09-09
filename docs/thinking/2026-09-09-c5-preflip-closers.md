# 2026-09-09 — Lane C5 pre-flip closers: the bounty truncation gates, and the short final stripe

**Context / trigger:** ROADMAP Lane C5, the two CODE closers among the pre-flip items. `R-BOUNTY-TRUNCATION`
was GATED by the BOULDER-2 residual-closures certification
(`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md`,
§2.6) on two named gates, G-BT-1 and G-BT-2; the accumulator alternative is REFUTED there on build-immutable #8.
`R-SHORT-FINAL-STRIPE` is rider item 2 of the Economist's chunk-size advisory
(`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-default-chunk-size-256KiB-2026-09-06.md`, §4),
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

**The threshold is derived, and the rule has a closed complement.** The certification asked for a threshold
"derived from the arithmetic, never typed". `shippedBountyBase()` is `RepairBountyBase(K, DefaultChunkSize + Overhead)`
— the base the shipped default pays, 10 today. The rule is then: **warn iff the operator SET `-chunk-size` AND
that geometry pays a smaller base than the default's.** Its complement is closed and both sides are driven in
`TestGLambda8PublishWarningFiresOnlyForAnExplicitSmallChunk`: silent for an unset flag at any geometry, silent
at and above the default, ZERO arm below `MinBountyChunkBytesFor`, truncation arm in the band between. The
default is silent *by construction* rather than by a hand-kept number, so a moved default cannot leave the
warning stranded — it moves the threshold with it.

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
| **Re-addresses** | Every object whose content fits in one frame: `size ≤ chunkSize − 8` (262,136 B at the default). Its data and parity chunk IDs, root, link key and manifest all change. |
| **Byte-identical** | Every object with two or more frames — they share a stripe, the tail stays padded, `chunk.Split` does the whole job. Gated by `TestMultiFrameObjectsKeepThePaddedTail` and measured on the 1.5 MB modal object. |
| **Manifest format** | UNCHANGED. No new field, no new CBOR key. The frame length rides in the existing `ChunkSize`. |
| **Erasure geometry** | UNCHANGED for every object. Only the shard LENGTH of a single-frame stripe moves. |
| **An existing store** | Keeps working. Nothing on the read path consults `DefaultChunkSize`; readers take the geometry from the manifest and the frame headers, so old objects still fetch, audit and repair exactly as before under their own committed `ChunkSize`. Re-publishing the same bytes yields a NEW root, so dedup does not span the boundary and the store holds two copies of anything re-published. |
| **The genesis** | **MOVES.** The 2,042-byte manifesto is a single-frame object. root `fce9eeeb…20d6` → `31768fb4…7dd1`, manifest chunk `5478750c…d107` → `f761f80b…fcf6`, block hash `f428d0a8…0951` → `e44344ea…72c0`. Unlike 4′ the ROOT moves too, because this re-frames the data chunks the root is built from. |

The genesis move is the **second** in this window and rests on the same ground the owner accepted for the first
(4′, 2026-09-07): no live network exists and every development chain is wiped on upgrade. It is owed the same
explicit acceptance; `TestGenesisBlockHashIsPinned` holds all three literals so it cannot drift unannounced.

### Two consequences the certification and the advisory did not price

1. **A sub-frame object's repair bounty base becomes ZERO.** Its shard is now ~1 KB, so `k·shardBytes` is far
   below one credit of fetch. The direction is the one Don't #7 asks for — repairing a 1 KB object really does
   cost about 10 KB of fetch, and the old 10-credit payment was a ~250× over-pay funded from that object's own
   escrow — but the integer floor lands it at zero, so sub-frame objects are kept alive by the serve economy
   rather than by bounties. It is **not silent**: the judge counts `Stats.BountyBaseZero` and journals a WARN
   on every such settlement, which is G-λ-8 doing exactly its job, now on a geometry a real publish produces
   (the old fixture's 6,553-byte shard was synthetic — the Economist's `R-SHARD-IS-CHUNK` note; the fixture is
   re-grounded on a real 1 KB object here).
2. **Threat F3 gets slightly worse for sub-frame objects.** `docs/threat-catalog.md` already records
   timing/size fingerprinting as unmitigated ("chunk sizes + timing fingerprint files even encrypted (no
   padding)"), and silt claims no size hiding. Still, the padding *did* blur a sub-frame object's true size to
   "somewhere in one chunk", and after this change its exact byte length is visible to any caretaker (in the
   layout's `ChunkSize`) and to any holder (in the stored shard length). Recorded here rather than decided: it
   is a degradation inside an already-open, already-disclosed threat, and the seat that owns F3 may want to say
   so out loud before the flip.

---

## Gates and their ablations

| Gate | Where | Ablation that must go RED |
|---|---|---|
| G-BT-1, both sides of the rule | `cmd/silt/g_lambda_test.go` `TestGLambda8PublishWarningFiresOnlyForAnExplicitSmallChunk` | Restore the zero-only rule ⇒ `bountyChunkWarning(52_412, true)` returns `""` |
| G-BT-2, the arithmetic | `core/credit/g_bt2_test.go` | Divide before the multiplier ⇒ 4 instead of 7; and the zero-signal case collapses to 0 |
| G-BT-2, at the judge | `core/node/g_bt2_judge_test.go` | Settle with the floor-first price ⇒ the judge pays 4, not 7 |
| The short frame | `core/pipeline/short_final_stripe_test.go` | Split at `opts.ChunkSize` again ⇒ the 1 KB object commits a 262,144-byte geometry |
| The audit coupling | `core/node/g_shortstripe_audit_test.go` | Commit `opts.ChunkSize` instead of the frame used ⇒ the auditor sizes a 1,048-byte shard at 262,160 and every honest holder fails |

## What is NOT in this change

- The accumulator (REFUTED, build-immutable #8).
- `R-ANCHOR-BEARER-TRANSFER`, the third C5 item — a red-team pass, not a build.
- The corrected `pod.md:417-421` / `relayrole.go:34-38` relay-anonymity sentence the same certification owes
  (§4). It belongs to `R-RELAY-ANON-SET`, not to either closer here.
