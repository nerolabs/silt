# One bug in four costumes

No code tonight. Tonight was for reading two reviews that arrived the same
day — one from the principal-engineer seat, one from the research team —
and discovering they had converged, independently, on the same sentence:
the four consensus bugs that have eaten the last two weeks were not four
bugs. They were one.

Each had looked different in the field. A fork-choice that oscillated
between committed chains. A maturity handoff where cheap identities
head-counted their way into a quorum. Two honest proposers who signed at
the same height and slashed each other. A launch gate that let one free
anchor bless a competing block. Four multi-region field runs, four
attributions, four fixes — and each time, the uneasy feeling of walking in
a circle.

The reviews named the circle. Every one of those defects was a finality
quorum that failed to *intersect* over the validator set it claimed to
represent. That is the oldest safety theorem in Byzantine consensus:
two quorums that share an honest member cannot finalize conflicting
blocks, because an honest member never signs twice. Sized against a
shifting set, counted by heads instead of weight, signed without writing
the ledger, or filled from a larger pool than it was sized over — four
doorways into the same room. The feeling of circling wasn't failure; it
was what re-deriving a known theory one edge at a time feels like from
the inside. The perimeter is finite. We had been discovering it with the
most expensive, slowest, least deterministic fuzzer available: a
multi-region network under real WAN weather.

So tonight's decisions are about never doing that again. The invariant
set — five statements, each annotated with the scar that proved it and
the code it governs — is now canon, and every consensus-touching change
must say which invariants it touches and why it preserves them. Alongside
it, a new first gate: a deterministic model-checker that drives the real
consensus core through adversarial schedules — delays, partitions,
crash-restarts, equivocators — and asserts all five invariants after
every step, on a laptop, in seconds. Its acceptance test is honest: run
against the code as it stood before each of the four fixes, it must go
red on all four. The proof it would have caught them is the proof it was
worth building. Field runs return to the job only they can do — proving
liveness on a real, hostile internet — and stop being the place safety
theorems are discovered.

The other decision was about restraint. The research certification for
the fourth bug corrected our proposed threshold by exactly one: a launch
block now needs a strict majority of anchors behind it, because the
clever-sounding half-rounded-up version admits a split that is not merely
a fork but a permanent partition. One-off-by-one, at the heart of a
quorum rule, found by arithmetic instead of by a field run — the whole
evening's argument in miniature. Consensus is where this project spends
no novelty at all. The invention budget belongs to the part of silt that
is genuinely new; the consensus layer's highest ambition is to be
indistinguishable from the literature.

It was also a night for admitting what the documentation had become: one
viewpoint per bug arc, accreted, with the current plan smeared across
consult files and issue threads. The closed arcs move to the archive;
the live tree gets rewritten to say one thing. A project should be
readable the way its chain is supposed to be — a single head, with
history behind it, not four competing tips.
