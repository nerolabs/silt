# era-4 RegCap instrument options: byte cap (A) vs count cap (B)

Status: SHAPING DRAFT (not canon). Ground: `origin/main` @ `0076337`.
Author seat: Builder (advocates shipping + simplicity). A blind review judges the same
choice; the human ratifies. Certs that settle this choice:
- RULE shape (v5 validity cap):
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-witnessable-transitions-RECERT2-2026-08-29.md`.
- Counting rule = per-block TOTAL (fresh + renewal), fresh-only REFUTED:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-regcap-recert-VERDICT-2026-08-29.md`.
- Instrument = COUNT cap (B), not byte cap (A):
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-era4-regcap-instrument-A-vs-B-2026-08-29.md`.
- Value N = 256 (floor 18 at k=1, 7-determinant re-derivation gate):
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-regcap-VALUE-DERIVATION-VERDICT-2026-08-29.md`.

## The question

era-4 needs a v5 block-validity cap on BondRegs per block so the epoch-boundary witness
read-set is a registry-INDEPENDENT constant that fits the 2 GiB floor box. The RULE shape
is certified: a v5 validity cap replicas enforce on receipt, counting the per-block TOTAL
BondReg count — **fresh AND renewal**, after `canonicalBondRegs`. Renewals are NOT exempt.
Both fresh and renewals write `bondRegHeight[id] = h` at the identical apply site
(`core/chain/chain.go:2995-2996`), so both land in the same TTL due-bucket; #506 rate-limits
renewals per-IDENTITY, not per block, so O(registry) distinct ids can each renew once in one
block and all land in one bucket — the exact wall era-4 removes. A fresh-only cap sits idle
while the renewal term is unbounded (Research REFUTED fresh-only three times:
`era4-regcap-recert-VERDICT-2026-08-29.md`, `era4-regcap-VALUE-DERIVATION-VERDICT-2026-08-29.md`).
The INSTRUMENT and VALUE are open. Two candidate instruments:

- **(A) Per-block BYTE cap** on total BondReg bytes. Value `L`.
- **(B) Per-block COUNT cap** on total BondReg count. Value `N` (= `RegCap`).

Both are evaluated parametrically in the minimum-valid-reg size `M` and the block reg-body
budget `B`, so the analysis holds when the exact `M` lands.

## The measurement that reframes the choice (read this first)

The Tester re-measured `M` on `0076337` (2026-08-29):

| Reg shape | Min VALID size `M` | Honest ceiling `B/M` |
|---|---|---|
| Any reg that passes `VerifySpaceTime` (fresh OR renewal) | `M` = 1,485,573 B | `floor(B/M)` = `floor(2,097,152 / 1,485,573)` = **1** |

The load-bearing correction: **a renewal is NOT small.** Every valid reg — fresh and
renewal alike — must pass `verifyBond` (`core/chain/chain.go:1617` → `bond.VerifySpaceTime`),
which requires the full ~1.485 MB space-time Answer. The `225-byte renewal` that motivated
the original byte-cap-over-count-cap argument (`core/node/node.go:74`, "a count cap would
lapse them") was a PHANTOM: an Answer-less reg that `verifyBond` REJECTS. Source:
`.claude/agent-memory/tester/era4-regcap-measurement-2026-08-29.md`.

Consequence for A vs B: **at the deployed verifier, A and B express the SAME honest
ceiling.** With renewal ≈ fresh ≈ `M`, the block holds `floor(B/M)` = 1 valid reg whether
the cap counts bytes or regs. The two instruments only DIVERGE if reg sizes become
heterogeneous — i.e. only if `M` stops being a single number. Hold that thought; it is the
whole decision (see §Divergence).

## Shared implementation surface (both options)

Both instruments land the same way. The differences are one predicate line and one
derivation, called out per-option below.

### Where the predicate lands — v5-gated, era-3-freeze-safe

The era-3 committed-format freeze is immutable. A v5 predicate must be a NO-OP on v2/v3/v4
blocks. The template already exists and is proven: `core/chain/era3validity.go`
`validateEra3Roots`, which returns nil below its version and only fires at/above it.

- Add `validateEra4RegCap(b *Block) error` in a new `core/chain/era4validity.go`, mirroring
  `era3validity.go`. First line is the era gate:
  `if b.Version < BlockVersionWitnessable { return nil }` (v2/v3/v4 skip entirely — the
  era-2/3 rules are byte-for-byte unchanged, exactly as `validateEra3Roots` leaves them).
  `BlockVersionWitnessable = 5` per the ratified era-4 params.
- Call it from `validateBondRegs` (`core/chain/chain.go:1514`), which already runs on the
  ONE validated path `ValidateProposal` invokes (`chain.go:2289`). `validateBondRegs` is
  the natural home: it already walks `b.BondRegs` once and already carries the
  UNCONDITIONAL `seenRoot` dedup (`chain.go:1572`), which is the precedent for "a
  reg-body rule enforced at validity, not just proposer policy." Fold the cap check into
  that existing loop; no second scan.
- `versionSupported` widens `<= 4` → `<= 5` in the SAME release as the predicate
  (PREDICATE-FIRST, ratified). Do NOT widen a release ahead — era-3 did that and left an
  accept-a-wrong-root window (`chain.go:326-338`). No block is minted v5 until 4d.

### How a replica checks it on receipt

`validateBondRegs` runs inside `ValidateProposal`. An honest attester runs `ValidateProposal`
before signing; `ValidateCommit` runs it first. So a block violating the cap is refused at
BOTH consensus-entry points — an attester will not sign it, a commit carrying it is
rejected. This is identical to how `validateEra3Roots` and the `seenRoot` dedup already
propagate. The check is a pure function of `(block, cfg)` — every honest replica computes
the same verdict (I5). No clone, no apply: the cap reads only `b.BondRegs` and a config
scalar, so it is even cheaper than the era-3 root recompute (which clones + applies).

### Count the per-block TOTAL — fresh AND renewal (certified; NO renewal exemption)

The cap counts the per-block TOTAL BondReg count, fresh and renewal alike, after
`canonicalBondRegs`. Renewals are NOT exempt. This is the load-bearing correction Research
made three times (`era4-regcap-recert-VERDICT-2026-08-29.md`,
`era4-regcap-VALUE-DERIVATION-VERDICT-2026-08-29.md`), against an earlier fresh-only draft.

The reason is the TTL due-bucket surface, not the boundary surface. Both fresh and renewal
regs write `bondRegHeight[id] = h` at the identical apply site (`chain.go:2995-2996`, "reset
the TTL clock on every (re)registration"), so both land in the same due-bucket
`D = h + ttl + 1`. #506 rate-limits renewals per-IDENTITY (`chain.go:1587`, `regMinInterval`),
NOT per block: the per-block `seenReg[id]` guard (`chain.go:1583-1585`) only stops one id
appearing twice in one block. So O(registry) distinct existing identities can EACH renew once
in one block, and all land in bucket `D`. At fire-height `D` the TTL read-set is
O(registry) × SProofMax — the exact wall era-4 exists to remove.

A fresh-only cap does NOT bound this. It bounds one of the two committed witness surfaces
(the boundary `epochSet`/`qualified` symmetric difference, which only fresh ids change) but
NOT the other (the TTL `tagDueBucket`, which renewals populate by delete-and-reinsert). era-4
commits BOTH surfaces; RegCap must bound the tighter one. So both instruments count/measure
the per-block TOTAL. The predicate reads `canonicalBondRegs(b.BondRegs)` (already computed in
apply, `chain.go:2969`), which collapses same-id multi-reg to at most one reg per id — so the
count is exactly the number of due-bucket entries this block contributes. No new bookkeeping.

The renewal-exemption argument that motivated fresh-only ("a count cap would lapse honest
renewals") is DEAD: a renewal is not smaller than a fresh reg (both carry the full ~1.485 MB
Answer), so the honest TOTAL ceiling equals the honest fresh ceiling, and the total cap lapses
no honest renewal a fresh-only cap would have admitted (see §Recommendation).

## Option A — per-block BYTE cap (value `L`)

### A.1 Predicate
```
sum := 0
for _, r := range canonicalBondRegs(b.BondRegs):   // TOTAL: fresh + renewal
    sum += encodedBytes(r)          // the bondRegEncode size, chainrole.go:1757
if sum > L: reject (ErrEra4RegCapBytes)
```
`encodedBytes(r)` must be the SAME encoding every replica computes byte-identically. That
already exists (`bondRegEncode`, `core/node/chainrole.go:1757-1759`), used by the proposer
byte budget. A validity rule that sums per-reg encoded bytes is deterministic across replicas.

### A.2 Deriving `L` from `M` and `B`
`L` bounds the witness read-set, which scales with the NUMBER of proofs, not their
bytes. To turn a byte cap into a read-set bound you must divide back out by the minimum
reg size: max regs admitted = `floor(L / M)`. To admit at most `N_witness` regs
you set `L = N_witness × M`. With the witness fit `N_witness = 16,384`
(`2 GiB / (EpochBlocks=8 × SProofMax=16 KiB)`, the desk upper bound) and `M = 1,485,573`:
`L = 16,384 × 1,485,573 ≈ 24.3 TB`. That is absurd as a per-BLOCK byte cap — it is 12
million × the 2 MiB block budget. The honest-ceiling framing is the usable one: to admit
the honest `floor(B/M) = 1`, `L` must sit in `[M, 2M)` = `[1.485 MB, 2.97 MB)`. Any `L`
below `2M` and at/above `M` admits exactly 1 reg — same as today's budget.

### A.3 Re-derivation gate
`L`'s correctness depends on `M`, and `M` is a function of the space-time proof determinants:
`k` (label samples, `bond.go:117`), `Samples` (possession blocks, `bond.go:108`), `BlockSize`
(`bond.go:93`), `MinBond` (`cmd/silt/daemon.go:93`), `BondVDFDelay` (`node.go:291`). If ANY
of these shrinks `M`, the number of regs a fixed `L` admits RISES (`floor(L/M)` grows).
So a byte cap's read-set bound is NOT stable under a proof-size change: the same `L` silently
admits more proofs. The gate must bind `L` to the CURRENT `M`, re-measured whenever any
determinant changes. This is the #299 hazard in a sharper form: under succinct proofs `M`
drops ~1000×, and a fixed `L` admits ~1000× more regs into the witness read-set without
any rule edit — a SILENT read-set blow-up, not a loud rejection.

### A.4 Liveness (I4)
An honest block holds at most `floor(B/M) = 1` reg regardless of `L` (the 2 MiB body
budget binds first). So for any `L ≥ M`, A never rejects an honest reg. A renewal is not
smaller than a fresh reg (both carry the full ~1.485 MB Answer), so the honest total ceiling
equals the honest fresh ceiling — counting the total lapses no honest renewal-heavy block
that the body budget already admits. The liveness margin is entirely governed by the body
budget `B`, not by `L`.

### A.5 Simplicity / failure modes
- Set `L < M`: rejects EVERY reg → no validator can bond or renew → capture-heals and
  genesis-drain both wedge. Loud but catastrophic.
- Set `L` too high (or leave `M` stale after a proof shrink): the read-set bound silently
  fails OPEN — more proofs than the floor box can witness enter the due-bucket / committed
  set, and the witness overruns 2 GiB. This is the SILENT failure, and it is the one the era-4
  witness bound exists to prevent. A byte cap fails open under exactly the change (#299) the
  design must survive.

## Option B — per-block COUNT cap (value `N` = RegCap)

### B.1 Predicate
```
if len(canonicalBondRegs(b.BondRegs)) > N: reject (ErrEra4RegCap)   // TOTAL: fresh + renewal
```
Counting is trivially deterministic — a single integer compare, no encoding dependency at
all. It reads `canonicalBondRegs` (already computed in apply, `chain.go:2969`), which
collapses same-id multi-reg to one reg per id, so the count is exactly the number of
due-bucket entries this block contributes. No renewal exemption: fresh and renewal both land
in the TTL due-bucket and both are counted.

### B.2 Deriving `N` from `M` and `B`
`N` IS the witness read-set bound directly — it is a count of proofs, which is exactly
what the TTL-firing and boundary witness scales with (`witness_bound.go:202`,
`count × SProofMax`, a FLAT per-proof envelope). No divide-back-out. The bracket:
- Lower bound (must admit the honest ceiling at the LOWEST permitted `k`): `N ≥ floor(B/M)`.
  At the deployed k=64 this is 1; at the minimum permitted k=1 (`resolveK`, `bond.go:120-128`)
  it is 18. So `N ≥ 18`.
- Upper bound (witness must fit the box): `N ≤ 2 GiB / (EpochBlocks × SProofMax)`
  = `2^31 / (8 × 16,384)` = **16,384**.
- Ratified value: `N = 256`. Clears the k=1 floor of 18 by 14× (generous liveness headroom
  for any future `M` shrink) and sits 64× below the witness fit. Its worst-case valid block
  is `256 × M` ≈ 363 MiB, bounded by real M0 Sybil cost (256 distinct sealed plots, per-root
  dedup `chain.go:2949-2953`), not a free DoS surface; its witness (256 × 8 × 16 KiB = 32 MiB)
  fits the box 64× over.

### B.3 Re-derivation gate
`N`'s LOWER bound depends on `M` (must stay ≥ `floor(B/M)`); its UPPER bound depends on the
witness parameters (`EpochBlocks`, `SProofMax`, box size), NOT on `M`. So the count cap's
SAFETY (witness fit) is INDEPENDENT of the proof determinants — a proof shrink cannot make a
fixed `N` overrun the box, because `N` bounds the count the witness pays for directly. The
determinant the gate must bind is only the LIVENESS side: if `M` shrinks so that `floor(B/M)`
rises ABOVE `N`, honest regs get rejected. `M` is a function of all SEVEN determinants — `B`
(block reg-body budget), `k` (`BondLabelSamples`), `Samples`, `BlockSize`, `BondVDFDelay`,
`MinBond`, and the proof scheme — so the gate must fire on ANY of them changing, at the next
BlockVersion mint, NOT on #299 alone. #299 is the SHARPEST single determinant: under succinct
proofs `M` drops ~1000× and the honest ceiling rises to ~2,000 regs/block, above 256, so `N`
must be re-measured and re-minted before/with #299
(`.claude/agent-memory/builder/era4-regcap-299-gate.md`). The gate binds `N`'s floor to
`floor(B/M)`; its ceiling never moves under a proof change. Failure direction is LOUD (honest
reg rejected), not silent.

### B.4 Liveness (I4)
An honest block holds `floor(B/M) = 1` reg today (at the deployed k=64), 18 at the minimum
permitted k=1. `N = 256 ≫ 18`, so B never rejects an honest block on any permitted config at
the current `M`. The renewal-lapse concern is DEAD: a renewal is not smaller than a fresh reg
(both carry the full ~1.485 MB Answer), so the honest TOTAL ceiling equals the honest fresh
ceiling — counting fresh + renewal together lapses no honest renewal-heavy block that the
body budget already admits. The only I4 risk is a future `M` shrink lifting `floor(B/M)`
above `N` — caught LOUDLY by B.3's re-derivation gate (all seven determinants) before the
box can be endangered.

### B.5 Simplicity / failure modes
- Set `N < floor(B/M)`: rejects honest regs. LOUD (a bond/renew attempt fails visibly), and
  caught by the ablation: a block with **> 256 TOTAL BondRegs of ANY mix (all-renewal,
  all-fresh, or mixed) REJECTS; ≤ 256 TOTAL ACCEPTS** (see §Ablation spec below).
- Set `N` too high (up to but not over 16,384): still within the witness fit — the box does
  not overrun. The cap fails toward the SAFE side of the witness bound. There is no silent
  read-set blow-up: `N` caps the count the witness pays for, by construction.

## Divergence: when do A and B differ?

At a single `M` they are the same ceiling. They diverge ONLY when reg sizes are
heterogeneous — some regs much smaller than others. That happens under exactly one
foreseeable change: **#299 succinct proofs**, where `M` drops ~1000× and a block can pack
~2,000 small proofs. At that point:

- The BYTE cap `L` admits `floor(L/M_small)` ≈ 2,000 proofs into the witness read-set
  with NO rule edit — it fails OPEN, silently overrunning the box.
- The COUNT cap `N` still admits at most `N` = 256 — it fails toward SAFE, and the honest
  ceiling rising above 256 triggers a LOUD honest-reg rejection that FORCES the re-mint
  (the certified #299 gate) before the box can be endangered.

The instruments encode opposite failure directions under the one change the design is built
to survive. That asymmetry, not the current 1-reg ceiling, is the decision.

## Builder recommendation: (B) the COUNT cap, `N = RegCap = 256`

I advocate shipping and simplicity, and here they point the same way as safety.

1. **B is simpler to get right.** Its predicate is `if len(canonicalBondRegs(b.BondRegs)) > N
   reject` — a single integer compare, no encoding, no divide-back-out. `N` IS the quantity
   the witness bound is stated in (`2 GiB / (EpochBlocks × SProofMax)` is a COUNT). A byte cap
   forces every reader to convert bytes→count through `M` to reason about the actual read-set
   — an extra inference step, and the exact step that goes stale silently.
2. **B fails LOUD; A fails SILENT.** The property era-4 defends is "witness fits the box."
   `N` bounds the witnessed count directly, so it CANNOT be violated by a proof-size change —
   the worst case is a loud honest-reg rejection that forces a re-mint. `L` bounds bytes, and
   a proof shrink makes a fixed `L` admit more proofs into the read-set with no rejection and
   no signal — the box overruns silently. For a consensus safety bound, prefer the instrument
   whose failure is loud.
3. **The historical argument for a byte cap is retracted.** `node.go:74` chose bytes over
   count because "small renewals pack many per block, a count cap would lapse them." The
   Tester's 2026-08-29 re-measurement shows that renewal is a PHANTOM: every valid reg —
   fresh AND renewal — carries the full ~1.485 MB Answer, so no cap of either kind lapses
   renewals. The renewal-exemption argument that motivated a fresh-only count is DEAD: the
   honest TOTAL ceiling equals the honest fresh ceiling, so counting the total (which the
   TTL due-bucket surface REQUIRES) lapses no honest block a fresh-only count would admit.
   The one reason to prefer A no longer holds.
4. **B is already ratified and certified as a security parameter** (`RegCap = 256`, Andrew
   2026-08-29; RECERT2 Q2). This options analysis re-derives that choice from the parametric
   `M`/`B` reasoning independently — it is not merely deferring to the ratification. If the
   blind review surfaces a reason to prefer A, the divergence in §Divergence is where to
   contest it.

Keep the proposer BYTE budget (`MaxBondRegBytesPerBlock`, `node.go:270`) as-is: it stays
PROPOSER POLICY (WAN-gatherability, `chainrole.go:798`), orthogonal to the v5 VALIDITY count
cap. The two coexist — policy bounds block SIZE for the gather; validity bounds TOTAL reg
COUNT for the witness. Do not merge them; do not touch `MaxBondRegBytesPerBlock`.

## Ablation spec (the check 4c must satisfy — inject the defect)

The RegCap ablation is the spec the 4c code must satisfy. It counts the per-block TOTAL, so:

- A block with **> 256 TOTAL BondRegs of ANY mix REJECTS.** All three mixes must redden:
  all-renewal (e.g. 257 renewals), all-fresh (257 fresh), and mixed (e.g. 130 fresh + 127
  renewal = 257).
- A block with **≤ 256 TOTAL BondRegs ACCEPTS**, at the boundary (exactly 256) and below.

The old fresh-only ablation ("300 renewals + 200 fresh ACCEPT; 257 fresh REJECT") is itself a
green check with the defect UNINJECTED: it asserts a 500-reg (300-renewal) block ACCEPTS,
which is the exact O(registry) TTL-firing read-set the total-count rule must be able to
reject. Do not carry it forward. (No test code lands here — 4c builds it; this is the SPEC of
record.)

## Residual for the reviewer / human

- Two separate gates govern `N`. The FLOOR gate binds `N ≥ floor(B/M)` where `M` is a
  function of all SEVEN determinants (`B`, `k`, `Samples`, `BlockSize`, `BondVDFDelay`,
  `MinBond`, proof scheme) — re-derive at the next BlockVersion mint on ANY of them changing.
  The CEILING gate binds `N ≤ 16,384 = 2 GiB / (EpochBlocks × SProofMax)` on the three witness
  params (box size, `EpochBlocks`, `SProofMax`), independent of `M`. The floor moves under the
  seven; the ceiling moves under the three.
- Neither instrument is contested on the era-3 freeze: both are v5-gated no-ops below v5,
  proven by the `era3validity.go` template and pinned by the write-set guard test
  (`TestEveryDiskWritePathRunsTheEra3RootCheck` — add the era-4 analogue).
