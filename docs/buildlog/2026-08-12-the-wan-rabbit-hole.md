# The WAN rabbit-hole, and climbing back out

For two and a half weeks this build log went quiet. That silence is the
story.

What happened in the gap was a single bug — a fresh three-region chain
that would not commit its first block on a real wide-area network, while
the identical block committed in seconds on a laptop. It has a name,
`#286`, and by the end it had a clean one-file fix: a Merkle proof that
was quietly O(n) instead of O(log n), rebuilding whole subtrees on every
call, so that on a small cloud VM the bond-proof work saturated the CPU
and starved consensus of the cycles it needed to gather a quorum. Cache
the tree once, proofs go O(log n), a 64-megabyte answer drops from 743
milliseconds to eight, and the chain commits in seventeen seconds. Byte
for byte the same proof; the security argument untouched. A good fix.

The trouble was everything before it. That one bug was re-diagnosed
across five different theories, and *each new theory was discovered by
spending a real, billable cloud run.* First it looked like a transport
deadline cutting the gather short. Then like the validators never
learning each other's addresses. Then like an eight-megabyte genesis
block too fat to cross the network. Then a size-aware deadline was shipped
as "the fix" and a fresh cloud run was spent to prove it *wasn't*. Only
last did the profiler show the real culprit, and by then the meter had run
five times over. Worse: at least two of those "layers" were perfectly
reproducible on a single machine — the eight-megabyte block was
deterministic; a laptop would have shown it for free. We paid the cloud to
learn what a local test already knew.

There is a specific discipline this violates, and the sharper irony is
that we were writing that discipline *down* at the same time we were
breaking it. Two build-immutables were added mid-thrash: **build for the
adverse internet** (never invent a timeout out of thin air — the mature
networks solved this decades ago, so read the settled answer first) and
**root-cause before you patch** (name the failure mechanism in one honest
paragraph — *it fails because X; this addresses X by Y* — before touching
a knob, and if you cannot, stop and instrument rather than guess). An
expensive run is for *confirming* a fix you already understand and have
reproduced cheaply. It is never for *discovering* a cause, and never for
testing a guess. We knew this well enough to canonize it; we did not yet
know it well enough to follow it under pressure.

The tell was the silence itself. A subsystem that eats dozens of commits
in a week and produces no build-log entry is not making progress you can
narrate — it is stuck in a loop, and the loop processes its own pain by
re-running the same failing test rather than by stepping back. Meanwhile
the actual forward work — the privacy layer, the takedown layer, the
demand receipt, the concentration metric that is the whole point of the
next phase — got not a single commit.

So this entry is also the climb out. The compute bug is closed and its
regression guard is in place. The remaining wide-area work is folded into
one tracked item with one explicit finish line — a single clean warm run,
once, as a gate — instead of an open-ended invitation to keep poking a
cloud. The adversarial tests that a live cloud kept failing to even *set
up* are being moved onto a deterministic local harness that impairs its
own loopback with the same jitter and loss a real network has, so that an
attack we want to prove denied is one we can *schedule* and run on every
build, not one we hope fires somewhere over a WAN. And a gate now stands
in front of the cloud itself: no billable run without a written mechanism
and a named local reproduction first.

The lesson compresses to one line, and it is worth the two and a half weeks
only if it sticks: when the build log goes quiet under a pile of cloud
commits, the loop is stuck — stop, instrument, reduce it to something a
laptop can reproduce, and spend the expensive run only to confirm what you
already understand.
