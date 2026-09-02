# Class-M maturity recompute: streaming proof verifier (cut resident witness O(N·depth) → O(depth))

Date: 2026-09-02
Seat: Builder
Status: implementation design (ships with the code in this PR)

## The one-paragraph mechanism (attribute before patch)

The class-M maturity fold (`RecomputeMatureNow`, `core/chain/floorbox_recompute_maturity_v5.go`)
is slow at network scale because the caller materializes ALL N members' SMT proofs into
`SeenSetWitness.Members` (a `map[NodeID]MemberStateWitness`, ~20 KB/member) BEFORE the fold
runs. At N=100k that resident map is ~470 MB (measured `setupLiveHeap`); extrapolated ~2.3 GB
at 500k, ~4.7 GB at 1M. The fold loop itself already processes one member at a time and does
NOT retain proofs past each iteration (`statehash.Resolve` consumes `mw.*Proof` and the pokt
`VerifyProof` discards its per-call `updates` graph, `proofs.go:323`). So the cost is not
accumulated verify state; it is the resident INPUT witness the GC scans on every cycle. The
fix is to change the member-proof DELIVERY from "all N resident in a map" to a pull provider:
the box requests one member's `MemberStateWitness`, verifies its three proofs against the
committed root, folds the scalar, and drops it before requesting the next. Resident drops from
O(N·depth) to O(depth). This changes only HOW memory is managed; WHAT is verified (the anchor
`committedStateRoot`, the completeness MTH over the full id-list, the per-member predicate, the
own-config screens) is byte-identical. It is the certified soundness-neutral refactor
(Candidate 1, CERTIFIED 2026-09-02; PE Option 1, BUILD).

## Evidence base

- Baseline measurement (`floorbox_recompute_maturity_fold_cost_test.go`, this branch, BEFORE
  the refactor, `-short`): N=100k `setupLiveHeap`=470 MB, fold=1173 ms; N=10k
  `setupLiveHeap`=36 MB, fold=99 ms. The resident cost is the materialized `Members` map
  (setupLiveHeap), not the fold-local heap (the bench reads foldLiveHeap ≈ 0 because the
  witness is pre-materialized and held alive by the caller across the timed section).
- PE ruling `ruling-classM-maturity-fold-pony-fit-2026-09-02.md`: "the win is not 'discard
  accumulated verify state' (already discarded); the win is 'stream the witness so all N
  members are never resident at once.'"
- Research cert `classM-maturity-recompute-cheaper-fold-RESEARCH-CERTIFICATION-2026-09-02.md`,
  Candidate 1 CERTIFIED soundness-neutral, with residual R-M-STREAM-COMPLETENESS.

## Hard constraints honored

1. SAME `committedStateRoot` anchor, SAME per-member predicate, SAME completeness MTH. Pure
   memory/processing refactor. No change to WHAT is verified.
2. **R-M-STREAM-COMPLETENESS.** The completeness check `nodeSetMTH(w.IDs)` still consumes the
   FULL id-list. The id-list stays resident (small: 32 B/id, ~32 MB at 1M). Only the per-member
   PROOF heaps stream. A short/truncated id-list must still stall — RED ablation test added.
3. **No wire/format change.** `SeenSetWitness` / `MemberStateWitness` are in-memory Go structs,
   never serialized (grep: no CBOR/gob/json/Marshal on them). The streaming provider is a Go
   function parameter, not a wire contract. No v5 leaf added, no committed object changed. The
   pure-memory-refactor assumption the certs made HOLDS. (Confirmed: no format change needed.)

## Design

### Streaming entry point (additive, box-side only)

Add `RecomputeMatureNowStreaming(committedStateRoot, w SeenSetStreamWitness)` where the member
proofs are pulled on demand:

    type SeenSetStreamWitness struct {
        IDs             []ports.NodeID          // full list — completeness MTH consumes it whole
        SeenRootWitness statehash.Witness
        SeenRootValue   []byte
        Member          func(ports.NodeID) (MemberStateWitness, bool) // pull one, then free it
    }

The fold:
1. Completeness: `nodeSetMTH(w.IDs)` over the FULL id-list == committed `validatorsSeenRoot`
   (byte-identical to today, line 172). Unchanged.
2. Per-member: for each `id`, `mw, ok := w.Member(id)`; verify its slashed/bonded/bondDomain
   proofs against `committedStateRoot`; fold the scalar; the loop drops `mw` (and its proof
   heaps) at the end of the iteration. Never holds N members resident.
3. Coefficient + threshold: byte-identical to today.

### `RecomputeMatureNow` becomes a thin adapter (backward compatible)

The existing `RecomputeMatureNow(root, w SeenSetWitness)` stays — it builds a `Member` closure
over the resident `w.Members` map and delegates to the streaming path. This keeps every existing
caller (`maturityLatchOps`, `RecomputeDeMatureSuperQuorum`) and all existing tests green with no
signature churn. The fold logic lives in ONE place (the streaming path), so the two cannot drift.

The verdict is byte-identical between the two paths (fold-equivalence test asserts
streamed == whole-map == full-node `matureNow`).

## What this does NOT do (verify, don't assert — from the PE ruling)

- Does NOT fix the O(N log N) compute floor (two `nakamotoCoefficient` sorts + the `nodeSetMTH`
  sort still touch all N). At N=1M the sorts + ~3M verifies + 1M-leaf MTH may stay over the
  time budget regardless of RSS. The RE-MEASURE decides whether 1M is reachable on TIME.
- Does NOT remove super-linearity; it removes the multi-GB resident set (the RSS slope and the
  GC constant). The measurement decides the M_seen ceiling.

## Test plan

- Fold-equivalence: streamed path == resident-map path == full-node `matureNow()` at several N.
- RED ablation (R-M-STREAM-COMPLETENESS): feed a SHORT id-list to the streaming path → assert it
  still stalls `ErrRecomputeSeenSetIncomplete`. This proves the completeness check still bites
  under streaming.
- RED ablation: a forged per-member value pulled by the provider still fails its Resolve and
  stalls `ErrRecomputeMemberStateUnproven` (streaming does not weaken the anchor).
- Re-measure: fold time + peak resident witness at N ∈ {1e4, 1e5, 5e5, 1e6}, streamed vs
  resident, with a heap-profile / cumulative-alloc peak. Derive the pony ceiling (fits 60 s AND
  ~1 GiB RSS-under-repair). This is the load-bearing output that sets the M_seen cap value.

## Measured results (2026-09-02, `TestMeasureRecomputeMatureNowStreamingWin`, this machine)

Box-side witness = what the FLOOR BOX holds (net of the provider-side prover a box never holds).
Budget: fold must fit BOTH 60 s (10% of the 10-min mature cadence) AND ~1 GiB RSS-under-repair
(2 GiB pony − ~1 GiB concurrent repair, R2.5).

| N | proofDepth | resident witness (box) | streaming witness (box) | resident fold | streaming fold |
|---|-----------|------------------------|-------------------------|---------------|----------------|
| 1e4 | 16 | 20.0 MiB | 314 KiB | 103 ms | 124 ms |
| 1e5 | 20 | 310.5 MiB | 3.05 MiB | 1173 ms | 1612 ms |
| 5e5 | 21 | 1432.5 MiB (EXCEEDS) | 15.26 MiB | 12019 ms | 10046 ms |
| 1e6 | 22 | 2928.7 MiB (EXCEEDS) | 30.52 MiB | 94714 ms | 24571 ms |

Reading:

- **RSS ceiling stops binding under streaming.** The resident-map witness the box must hold grows
  O(N): it EXCEEDS the ~1 GiB headroom at N=5e5 (1.43 GiB) and N=1e6 (2.93 GiB) — that is what
  forced the low pony ceiling. The streaming witness is the id-list (32 B/id) + ONE in-flight
  member (~2 KiB, O(depth)): 314 KiB → 30.5 MiB across the whole range, « 1 GiB at every N.
- **The remaining ceiling is TIME.** Streaming fold: 124 ms → 1.6 s → 10.0 s → 24.6 s. All under
  60 s, INCLUDING N=1M at ~24.6 s. At N=1M the streaming fold (24.6 s) is ~3.9× faster than the
  resident fold (94.7 s ≈ the PE-cited ~96 s), confirming the resident cost was GC over the
  multi-GB set, not arithmetic. The residual slope is the O(N·log N) compute floor (two
  `nakamotoCoefficient` sorts + `nodeSetMTH` sort + ~3N verifies) streaming does not remove.
- **Derived pony ceiling (this harness):** with streaming, N=1M FITS on BOTH axes (24.6 s < 60 s;
  31 MiB « 1 GiB). The pony-fitting ceiling the PE put at ~50k resident-map is lifted past 1M on
  RSS and to ~1M+ on time in this measurement. This is the load-bearing input to the M_seen cap
  VALUE — which stays research-gated (a security parameter) and owner-ratified (the finite-cap
  vision call). The measurement supports setting M_seen at or above the projected honest-network
  upper bound (1e6), making the cap the tolerable non-binding evolving-tier knob, not an M0 trade.

Caveats (do not over-claim — this is a white-box in-package harness, not a field run):

- The white-box fixture injects members directly into the committed maps; it does not pay block
  application or real wire delivery. The provider-side proof issuance and network fetch are NOT
  in the streaming fold time. A field/integration measurement under real witness delivery is the
  Tester's blind re-measure gate before the cap value is set.
- `residentWitness` is a `HeapInuse` delta (map over baseline); it carries GC-timing noise but the
  O(N) slope and the >1 GiB crossings at 5e5/1e6 are unambiguous. `streamWitness` is the analytic
  box floor (id-list + one member proof), the exact bytes the box holds.
- **No format change was needed** — confirmed. The refactor is pure in-memory/streaming-decode;
  `SeenSetWitness`/`SeenSetStreamWitness`/`MemberStateWitness` are never serialized, so the certs'
  pure-memory-refactor assumption holds and the soundness cert is not re-opened by a wire change.
