# silt — mothballed 2026-09-22

> **Status: paused, not abandoned.** Work stopped on 2026-09-22 by the owner's decision.
> The project resumes at an unset later date. Nothing here is canon; the canon is
> [`VISION.md`](VISION.md) and [`TENETS.md`](TENETS.md) and it did not change.
> This document is the handover — what is true, what is not, where the work stopped, and
> what the next person should not spend a week rediscovering.

The release-candidate date of 2026-09-27 named in
[`integration/rc/RC-PROPOSED.md`](../integration/rc/RC-PROPOSED.md) **lapsed unmet**. That
list stays in the repository as the honest record of what was and was not closed. Read it
as a snapshot dated 2026-09-22, not as a plan.

---

## Why it stopped

The storage plane works. The trust plane has one open mechanism that resisted roughly two
months of calendar time, and the owner judged the cost of closing it too high for now.

Stated once, plainly: **the consensus critical path shares one ordered per-peer connection
with the payload that congests it.** A validator must republish a ~1.5 MB space-time
possession proof every few minutes, every other validator must receive and verify it, and
that traffic rides the same connection as the consensus frames whose deadlines it blows. On
a link slower than the deadline assumes, the chain stops committing.

That sentence took six field reproductions to reach. Everything above the transport that was
tried moved its own number and did not move the wedge, because each still needed a round trip
on the connection the payload owns.

---

## What is true, by tier

The project's own rule is that correctness is a command result, in this order: unit →
consensus model-check → integration → end-to-end under network impairment → field. A claim
is only as strong as the highest tier that drove it.

| | state |
|---|---|
| **Storage plane** | publish, fetch, erasure coding, repair, NAT traversal — driven to the field tier and holding |
| **Consensus safety** | the five invariants asserted under adversarial scheduling; model-checked |
| **Sybil composition** | **three of five axes deny** (bond size, possession, retention). Demand and diversity are *unwired* and report themselves as unwired rather than passing vacuously. Until 2026-11-18 the bond-size and possession axes rested on a plot that was 128× compressible; see "Re-measured 2026-11-18" |
| **Trust plane liveness on an adverse network** | **open.** This is the wedge. On 2026-11-18 one paired field run passed with the label-node plot and failed on `main` (812 s); see "Re-measured 2026-11-18". One pair is not a distribution |
| **The M0 verdict** | **never commissioned.** It belongs to an external red team and no external party has run it |

Six RC items carry **no verdict at all** — 11, 12, 13, 17, 18, 19. A suite passing nearby is
not evidence for them. Item 18's seizure drill is cheap and needs a paired key-holder arm or
it is vacuous. Item 19's suite exists and was never run, and it tests rolling binary upgrade
rather than the four clauses item 19 actually names.

**Nothing in this repository has been audited, and the mission claim is unproven by the only
standard the project accepts for it.** `README.md` says 0.x and unaudited. That is accurate
and should stay.

---

## The hazard this project kept hitting

A claim written confidently, cited by later documents, and wrong. It happened often enough
to be a property of the work rather than bad luck, and the next person will hit it too.

The pattern: a measurement is taken, a conclusion is drawn one step beyond what the
measurement supports, the conclusion is written into a document, and later work reasons from
the document instead of the measurement. By then the original number is three citations away.

Three recorded instances, kept because the method that caught each is the transferable part:

1. **"The proposer's own registration reaches no attester."** True of the function read in
   isolation; false of the path, which already broadcast it on every sweep. Caught by driving
   the path instead of reading it. The fix turned out to be an *ordering* change, not the new
   delivery mechanism the document had specified.
2. **"Item 21's second claim is held."** One pass at 68 s. The second drive of the *same
   binary* went 404 s without a commit. Caught by re-driving. A pass is not a result until it
   repeats.
3. **"The congested arm is a ceiling — a relay cannot shed bytes a peer has never received."**
   The simulator counted *attempted* sends and that was read as deliveries. Counted at
   arrival: 860 of 876 payloads had arrived and been queued. The gap was a lost receipt, not
   an undelivered proof, and one constant caused it. Caught by adding an arrival counter —
   the blind spot was in the measuring instrument, not the system. Fixed 2026-09-22
   (`core/node/peerrate.go`); the congested arm went from wedged to committing.

**The lesson, if there is one:** before believing any claim in the written record here, find
the command that produces the number and run it. The tests are the honest record; the prose
is a summary of what someone believed at the time.

---

## Where the work stopped

The last change made the per-attempt request deadline a function of the peer's observed path
rather than a 256 KiB/s constant. It moved the congested arm from wedged at height 1 to
committing, and left the constant's cousins in place.

**The named next terms, in the order the evidence points:**

1. **`requestSizeExtensionCap` (30 s)** — a constant of exactly the species that was just
   removed one layer down. It is now the binding limit on the congested arm.
2. **`maxChainReplyBytes`** — still derived from the *global* assumed floor, so a node serves
   chain-sync windows sized for a link its peer does not have. Shrinking it produced a large
   measured improvement, confounded with the deadline change and never separated.
3. **The renewal submit itself** — 5.0× the gather legs on a fast wire. Reducing it means
   touching `SubmitBondRenewal`'s head gate, which is deliberate and load-bearing: with a TTL
   tighter than the head window a kept copy stays window-valid after the standing it defends
   has decayed. Shipped TTL is 32 and the window is 8, so they do not conflict at shipped
   values. This is a design decision, not a task.

**Do not re-open these.** Each was built and eliminated, with the evidence under RC item 21:
anything above the transport that still needs a round trip on the congested connection;
raising the outbound cap; the control lane's three invariants; a one-in-flight-per-peer
submit rule (built, fired zero times, removed).

**The separate control connection is already built, and did not end the wedge.** It landed
on 2026-09-21 (`adapters/tcpnet/ctrllane.go`): frames at or under 64 KiB ride a second TLS
connection negotiated by ALPN (`silt-ctrl/1`). On the field run that followed, the lane was
live on all four seats with zero lane failures and zero starved control frames, and the
chain still went 418 s without a commit (RC item 21). An earlier draft of this section
listed it as an unbuilt option; that was wrong.

The likely reason, not yet driven in the field: the field shaped each seat with one netem
band for all swarm traffic, so both connections share one FIFO queue at the slow link. The
control connection escapes TCP's in-order delivery, not the bytes queued ahead of it at the
bottleneck. Only sending fewer bulk bytes, or not sending them on the critical path,
addresses that.

**The structural answers still open, both large:** QUIC (a new transport adapter, and
subject to the same shared-bottleneck limit), or succinct proofs (which replace the 1.5 MB
payload with a small one, and are a **new era** by the frozen-format rule).

Succinct proofs are the one that dissolves the problem rather than managing it. A resumption
with materially better tooling should probably start there and treat the transport work as
what it is — mitigation of a payload that should not be on the critical path at all.

---

## Re-measured 2026-11-17 (branch `builder/have-bondreg-and-paced-bulk`)

Three fixes suggested by an outside reader were built and measured below the field tier.
None has been driven in the field. Re-run the commands before citing a number.

| Change | What it does |
|---|---|
| `MsgHaveBondReg` | A receiver that queues a renewal sends the digest back unsolicited, so the receipt is recorded even after the submit request's deadline expired. The submitter stops re-sending a proof the peer holds. |
| Pieced framing (`silt/2`) | On a negotiated bulk connection, frames over 16 KiB go as 16 KiB pieces, and a small frame goes at the next piece boundary. `TCP_NOTSENT_LOWAT` bounds the kernel's unsent queue. Older peers negotiate `silt/1` and the old format. |
| `simnet.Config.CtrlLaneBytes` | Optional second queue per pair in the simulator, modelling a bottleneck that queues each connection separately. |

The suggested acknowledgement window for pieces was not built. It caps throughput at
window ÷ RTT on every link, about 320 KiB/s at 32 KiB and 100 ms. The suggested backoff
change in `peerrate.go` was not built either; it was described as following from the pieces.

**Coverage on the congested arm, 64 KiB/s, 72 sweeps, eight seeds.** Mean, with range. One
queue per pair is the closer model of the field's netem shaping.

| | Height | Shed | Renewal submits |
|---|---|---|---|
| One queue, before | 2.4 (2–3) | 47% | 470 |
| One queue, with `MsgHaveBondReg` | 5.1 (4–7) | 71% | 36 |
| Separate queues, before | 8.0 (6–9) | 74% | 81 (60–210) |
| Separate queues, with `MsgHaveBondReg` | 8.0 (6–9) | 72% | 41 |
| Fast wire, for reference | 11 | 93–95% | 30 |

Command: `go test ./core/node/ -run TestTheDigestRelaysCoverageOnAHealthyWireAndACongestedOne -v`
(seed 17 only; the eight-seed sweep used `driveCoverageSeeded`).

**A 121-byte frame behind bulk on one connection, macOS loopback.**

| Rate, bulk | Unpieced | Pieced |
|---|---|---|
| 8 MiB/s, 16 MiB | 1.81 s | 0.07–0.22 s across runs |
| 64 KiB/s, 1.5 MB | 24.0 s | 12.6 s |
| 64 KiB/s, 1.5 MB, Linux (CI, one run) | 23.9 s | 2.3 s |

Darwin accepts `TCP_NOTSENT_LOWAT` but does not enforce it on write, so about 800 KB of the
proof was still ahead of the small frame at 64 KiB/s; disabling the option changed nothing.
Linux enforces it in `sendmsg`, and there the pieced wait fell to a tenth of the unpieced
one. That figure is one CI run on GitHub's Ubuntu runner over loopback; CI's unshortened
step reruns `TestPiecedFramingAtTheCongestedFieldRate` on every pull request.

**What this says.** `MsgHaveBondReg` removes most of the renewal traffic and roughly doubles
progress where small frames share the bulk queue. On a separately queued control lane it
adds nothing to height, because the head-probe inventory already carries the same evidence.
Neither fix reaches the fast wire's 11 heights, and neither is evidence about the field.

**A silent loss found on the way, and fixed.** `TestOutboundBoundDropsNothingWhenThePeerDrains`
failed intermittently on macOS on `main`: 5 of 6 batches of 15 runs. Two hundred sends to a
peer with no live connection each dialled it; macOS's accept backlog is 128
(`kern.ipc.somaxconn`), so some dials were reset in `connect`, and `deliver` dropped those
frames without an error. Senders now share one dial per peer, and a frame dropped because
every dial failed is logged. Same conditions after the fix: 0 of 6 batches failed.

---

## Re-measured 2026-11-18 (branch `builder/bond-plot-label-nodes`)

The field rows at the end of this section are one paired run per arm. Re-run the commands
before citing a number.

**A soundness flaw in the bond plot, found and fixed.** Each 4 KiB plot block was its 32-byte
label expanded by chained SHA-256, so the label alone determined the block. A prover could keep
only the labels, 1/128 of the claimed plot, regenerate any block on demand, rebuild the
committed root and pass `VerifySpaceTime`. The existing regression covered a prover that kept
block hashes, not one that kept labels. The bond-size and possession axes were therefore about
128× weaker than claimed.

The fix makes each graph node its 32-byte label, so every stored byte is a hash output.
`TestSparseLabelProverRecomputesMostOfThePlot` keeps every 128th label and counts what one
challenge forces it to recompute: 97% of the plot. The disk format moved to v4, so an older
plot re-plots.

**The proof is about 12× smaller.** The Merkle leaves are the labels, and an answer is the
opened labels plus one multiproof that sends each shared path node once. The verifier derives
every index, so none are sent.

| Bond | Old answer | New answer |
|---|---|---|
| 64 MiB | ~1.46 MB | 92 KB |
| 1.08 GB (the floor) | ~1.5 MB | ~121 KB |
| 5 GiB | — | ~135 KB |

Command: `go test ./core/bond -run TestMeasure_AnswerSizeBreakdown -v`. The floor and 5 GiB
rows are computed from the index sets without sealing a plot that size.

**Sealing costs more time and stays inside the floor box's memory.** Sealing runs at about
40% of the old per-byte rate (78 vs 185 MB/s, both measured under background QoS on the same
machine). `SealInto` keeps the most recent 256 MiB of labels in memory and reads older parents
back from disk. A 5 GiB plot on the 2 GiB floor box needs about 336 MiB
(`TestSealFitsTheFloorBoxMemoryBudget`). `PlotSealThroughput` was left at 270 MB/s, so the
derived floor stays at 1.08 GB. Re-deriving the floor at the new rate would lower it to about
460 MB, and that is an owner decision.

**The local impairment harness passes with the new proof.** `LOCAL=1` docker harness, flow
`21-impaired-commit` (80 ms ± 20 ms, 1% loss, 25% reorder on all four validators, three heights,
220 s escape bound), 64 MiB bonds:

| Build | Runs | Worst gap | Heights | Bond regs per block |
|---|---|---|---|---|
| `main` | 2 | 270–374 s | 2 of 3 | 1 |
| `main` + catch-up window fix (PR #864) | 2 | 371–623 s | 2–3 of 3 | 1 |
| Label-node plot | 2 | 35 s, 68 s | 3 of 3 | 4–5 |

The old proof filled the 2 MiB per-block registration budget alone, so the founding validators'
registrations drained one per block through the impaired window. The new one fits four or five
per block. In the second label-node run the window still carried registrations (8
registration-bearing gather legs, 6 carrying the proof) and the chain kept committing; flow
`4-become-validator` passed in the same run. The first run carried none in its window, so it
alone shows less. Two runs are not a distribution, and the local harness has no real WAN and
no NAT.

Command: `LOCAL=1 EQV_ISLAND=0 FLOWS="first_run become_validator impaired" ./cloudtest.sh` from
`integration/cloudtest`.

**The field, paired.** GCP, 13 nodes, four validators in us-west1, us-east1 and europe-west1,
the same impairment on all four, 64 MiB bonds, one harness. `IMPAIR_HEIGHTS=16` stretches the
window past the first renewals, so both arms carry proof-bearing blocks.

| Build | Run | Heights | Worst gap (220 s bound) | Registration-bearing legs | Validator peak RSS |
|---|---|---|---|---|---|
| `main` | `main16c-11c56276` | 8, then stalled | **812 s** | 84, 65 carried | 0.75 GiB |
| Label-node plot | `lbl16-d1e8cae9` | **16 of 16** | **69 s** | 96, 29 carried | 0.17 GiB |

The control reproduces September's field wedge (794–843 s). The label-node arm commits through
the renewals with the proof on the critical path. A three-height run of the label-node build
(`full-d1e8cae9`) also passed, at 61 s, but carried no registration in its window. One paired
run is the field tier's first evidence for this change, not a distribution.

Command: `EQV_ISLAND=0 IMPAIR_HEIGHTS=16 FLOWS="first_run become_validator impaired"
./cloudtest.sh up`, then `run`, then `down`, with the pre-flight gate's `RUN_*` set and
`RUN_LOCAL_PROOF="SUITE=all ./integration/adversarial/run.sh"`.

---

## Restarting

```sh
go build ./cmd/silt          # single binary at ./silt
go test -short ./...         # the local suite
python3 scripts/check_*.py   # seven repo lints, all currently green
```

Read in this order: `docs/VISION.md`, `docs/TENETS.md`, then
`integration/rc/RC-PROPOSED.md` from *"The order of work"*, then `git log`.

**Environment constraints that are real and cost time if ignored.** The development machine
is shared and memory-bound, not CPU-bound. Heavy commands run throttled
(`taskpolicy -c background nice -n 19 go test …`), three heavy jobs maximum, and the box
reaps background tasks at around five minutes. Full suites are CI's job. Cloud harness seats
were SPOT and preemption *deletes* the instance, so a vanished seat must be attributed as a
lost measurement rather than a product failure.

**Things that will have rotted by the time this resumes:** the Go toolchain pin (1.26.5), the
cloud harness credentials and any standing infrastructure, Netlify for silthq.com (already
out of the CI loop before the mothball), and every dated claim in the RC list. Re-run before
believing.

---

## What was built

1,190 commits across 60 working days, 2026-07-24 to 2026-09-22. 858 Go files, roughly 217,000
lines. No cgo. One self-contained binary with the web UI embedded.

The parts worth keeping, in rough order of how hard they were to get right: the deterministic
simulator and the single-loop lock-free core that makes it deterministic; the erasure-coded
storage plane with repair; the space-time possession proof and its audit path; the objective
bond-weighted consensus and its five invariants; the transparency-log takedown path; and the
test discipline itself — every gate proven red before its fix, with a vacuity guard proving
the fixture could fail.

That last one is the thing most worth preserving. A large fraction of this repository is
tests that would catch a regression on a developer's machine in seconds, and several of the
defects found in the final weeks were caught by oracles written months earlier for unrelated
reasons. A consensus model-check caught a *transport* change on the last working day.

---

*Mothballed with the tree clean, CI green, and every local gate passing. The failures are
recorded where they happened rather than summarized away, because the next attempt starts
from the evidence, not from the confidence.*
