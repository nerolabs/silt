# The substrate holds — a night on the P0 floor

The rescue redrew the map: stop re-running the cloud, prove the substrate.
This was the night of proving it. Not the flashy consensus corners — the
plumbing underneath them, the part that has to work before any red-team
can even begin. By morning the floor is solid in every place a laptop can
reach, and honest about the one place it can't.

The night's throughline is a discipline the rescue paid dearly to learn:
*attribute before you build.* Four of the five substrate items turned out
to already exist. The concentration metric that measures whether a Sybil
cohort could quietly capture consensus — built, tested against skew and
split fixtures, printed on every commit from the committed ledger, not
gossip. The publish path everyone feared was silently stranding content —
already durable-or-loud, already confirming that every erasure stripe kept
enough shards to rebuild before it hands back a link, already covered by
five sim tests. The cold-start re-mesh that a joining node needs when it
starts before its seed is listening — already certified over real TCP. Had
we taken the audit's file references as diagnoses rather than hypotheses,
we would have "fixed" three things that were not broken. Reading the code
first is the whole of the method.

What was genuinely missing was smaller and sharper. The dead-holder
lifecycle from the day before got its companion: a way to *certify* the
substrate under an adverse network, deterministically, on the loopback —
so that "the network stays live and serves under jitter and loss" is a
command you run in two minutes, not a hope you hold about a cloud. That
harness immediately earned its keep: it caught a bootstrap test trying to
pass under netem that had no business there — a clean-localhost timing test
with a half-second timeout and no retries, green once by luck, red under
loss. The honest move was to *exclude* it and say why, not to loosen a
threshold until it passed. Never fake green, even against your own harness.

The retry-versus-evict rule got a guard that can't flake: a dead peer must
be dialed as many times as its retry budget before the network gives up on
it. Evicting a good peer on a single dropped packet — the mistake that once
starved consensus under loss — now turns a specific test red. And the last
publish residual, a publisher that lost its anonymity cover when the one
validator it asked had just restarted, learned to ask the next one; the
ranking is the same from everyone who holds the chain, so the fix costs
nothing and closes the gap.

The honesty this log exists for is the ending. One corner is not green, and
no amount of substrate work will make it so tonight: the objective chain,
under real multi-region latency, still reorganizes its own committed blocks
onto a lighter fork. That is a consensus rule and a published claim, and the
discipline is explicit about where those go — to research, not to an
overnight guess at fork-choice weight. It is characterized, consulted, and
detected by the convergence test itself, so the morning's field run will
*see* it rather than paper over it. The floor is ready. The one load-bearing
question left is not ours to answer at two in the morning with a knob.
