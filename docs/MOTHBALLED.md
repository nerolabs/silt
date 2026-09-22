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
| **Sybil composition** | **three of five axes deny** (bond size, possession, retention). Demand and diversity are *unwired* and report themselves as unwired rather than passing vacuously |
| **Trust plane liveness on an adverse network** | **open.** This is the wedge |
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

**The structural answers, all of which were priced and all of which are large:** a separate
control connection (needs a role negotiated at connection setup, and reaches the relay
splice, the hole-punch upgrade and NATed reply routing), QUIC (a new transport adapter), or
succinct proofs (which replace the 1.5 MB payload with a small one, and are a **new era** by
the frozen-format rule).

Succinct proofs are the one that dissolves the problem rather than managing it. A resumption
with materially better tooling should probably start there and treat the transport work as
what it is — mitigation of a payload that should not be on the critical path at all.

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
