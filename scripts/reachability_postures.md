# Lane postures — the posture source for scripts/check_reachability.py

THIS FILE IS GATE INPUT, NOT NARRATION. `scripts/check_reachability.py` reads it:
for every record in `scripts/reachability_lanes.txt`, it finds that record's `label`
substring here, reads the enclosing bullet, and asks whether that bullet says the lane
**cannot be exercised** and NAMES the missing client entry symbol. The gate is
bidirectional — a bullet claiming `cannot be exercised` for a symbol the linker DID keep
goes RED, and an absent symbol without such a bullet goes RED too.

Four of the fifteen lane records are excused ONLY by the prose below. Every `label`
value in `reachability_lanes.txt` must occur EXACTLY ONCE in this file.

## Lane postures

  - **Open at the RC: the paid delivery lane — built, sim-proven, and it has not
    been exercised on a real network.** `OpenDeliverySessionRemote` and
    `SubmitDeliverySettle` are both linked into `./cmd/silt`, so an operator could
    open and settle a delivery session tomorrow; none ever has, which is why every
    C2 number is sim-driven. 
  - **Open at the RC: the delivery top-up — it cannot be exercised, and the
    missing client entry point is `FundDeliverySessionRemote`.** Raising a live
    session's budget after admission has no client half in the shipped binary:
    `FundDeliverySessionRemote` has zero non-test callers, so the linker drops it
    out of `./cmd/silt`. The lane opens and settles but cannot top up, and that is
    a strictly stronger claim than the bullet above — do not merge the two into
    one sentence.
  - **Open at the RC: the paid relay client — it cannot be exercised, and the
    missing client entry points are `AcquireRelayAnchors`,
    `OpenRelaySessionRemote` and `SubmitRelayPay`.** The standard sentence
    over-claims here, because it implies an operator could exercise this lane and
    simply has not. Verified by `nm` on a fresh `./cmd/silt` and confirmed by
    call-graph read: all three, plus `DialThroughPaid`, are absent from the linked
    binary (zero non-test callers), while the server half is present and
    dispatched — which is exactly how the lane came to read as delivered.
    In one sentence: the paid relay lane ships server-only, is e2e-proven in
    process, and no operator can open a paid relay session from a stock build. **Keep the qualifier, because
    over-correcting here is the same failure with the sign flipped:** the server
    accepts `MsgRelayOpen` from any peer, so a third party could hand-write a
    client. The accurate scope is *"the shipped binary has no client,"* never
    *"the protocol is unreachable."*
  - **OWED — one site still carries the looser sentence.** `cmd/silt/daemon.go`'s
    `-accept-relay-payments` flag help ends on *"built, sim-proven, never exercised
    on a real network"*; it must take the paid-relay label above before the RC is cut.

## Freeze-manifest mechanisms in the shipped binary

  - **Item 1 — the committed revocation-log size.** `(*Chain).stateRootLeavesV5` is the only
    production writer of the `tagRevLogSize` leaf, and it emits unconditionally on the v5
    branch. It is linked into `./cmd/silt`.
  - **Item 3 — the two-level v5 block hash.** `setBlockDigests` writes `AnswerDigest` and
    `SlashesDigest` on the mint path; `validateBlockDigests` refuses a block whose digests
    disagree with its body, from all three admission paths. Both are linked into
    `./cmd/silt`. A field nothing writes is not in the format, and a field nothing checks is
    not self-covering, so this lane holds both halves and each is its own record.
  - **Item 6 — the genesis mint that binds the config.** `genesis.Build` takes the
    `ConsensusParams` as a REQUIRED argument and stamps them onto height 0. It is linked into
    `./cmd/silt`, and the daemon's seed path passes real params rather than `nil`.
  - **Item 10 — the slashing-evidence byte ceiling.** `v5ValidateSlashes` enforces
    `SlashesBytesCap` against the encoded size before any signature work. It is linked into
    `./cmd/silt`.
  - **Item 19 — the era pair an operator reads at start-up.** `printEraObservable` backs
    `silt chain-status`; `chain.StartupEraLines` composes the declared ceiling beside the
    loaded chain's era, and the daemon boot path prints it through the one-line
    `eraStartupLines` wrapper. Both are linked into `./cmd/silt`. The lane record names the
    reducer, not the wrapper: the wrapper's inline margin is two call charges rather than
    body substance, so merging its two calls would refuse the record with nothing about
    reachability changed.
  - **Item 20 — the two refuse-to-start arms that are built.** `chainstore.Recover` refuses a
    torn or block-0-missing replay; `(*Chain).CheckConsensusParams` refuses a start whose
    argv contradicts the config its own genesis committed. Both are linked into `./cmd/silt`,
    and both return rather than warn. The third arm — comparing the persisted `blocks[0]`
    hash against the one this binary's `genesis.Build` produces — has no mechanism in the
    tree, so it has no record here and no green run covers it.
- [ ] **No compile-time default that the genesis commits has moved since the last
