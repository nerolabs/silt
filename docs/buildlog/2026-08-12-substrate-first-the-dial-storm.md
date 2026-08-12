# Substrate first: starving the dial-storm

After the rescue, the plan changed shape. The instinct all along had been
to keep re-running the multi-region cloud test until the trust plane's
adversarial drills finally graded green. The principal engineer's reading
was sharper: the drills weren't failing on the cryptography. They were
failing because the *substrate underneath them* — the plain business of
publishing a file, repairing it, and finding who holds it — was too flaky
over a real wide-area network to even schedule an attack. Prove the
substrate first, and the rest becomes drivable. So the first forward work
is not consensus. It is durability plumbing.

The dominant wound there has a number, #277, and a shape everyone who has
run a peer-to-peer network will recognise: the dial-storm. A holder leaves
— a laptop closes, a node dies — but the rest of the network keeps a
little note saying "that machine has a copy of this chunk." Nobody tears
the note up. So every time someone wants the chunk, they dial the machine
that is no longer there, wait out a full timeout, and only then try
someone else. Under churn a caretaker sweeping thousands of chunks spends
all its time dialing corpses and never gets far enough to notice the real
loss, let alone repair it.

The honest part of this fix was the part before any code. The audit's
best guess was that the discovery walk re-dialed dead holders. But reading
the code showed that walk had *already* been fixed weeks earlier — it
skips a peer it recently failed to reach. And signed provider records
already carry an expiry that the fetch path already honours. So the
hypothesis was half-stale, and building against it would have fixed
nothing. What was actually still broken was subtler and worse: the
negative cache that suppresses a dead holder only holds for thirty
seconds. After that the holder is dialed again — because its note in the
provider store is never removed. The cache wasn't stopping the storm; it
was *rate-limiting* it, to one wasted timeout per holder every thirty
seconds, forever. And two other paths — the one that hands provider
records to other nodes, and the one that re-announces them — never checked
the cache at all, so the network kept teaching itself about holders that
were already gone.

The fix gives a provider record a life, and an end to it. A departed
holder's record now ages out on a lease it must keep renewing by
re-announcing; a node stops serving records it knows are stale; and the
moment a holder is *confirmed* dead — not merely slow, but past all its
retries — its record is pruned from every chunk that still has another
live holder. That last clause is the whole art of it. You cannot simply
delete a dead holder everywhere, because sometimes it is the *only* holder
a chunk has, and orphaning that record would make the content
undiscoverable the instant the machine blinks — which is exactly the
restart-survival bug this project has fought before. So a sole holder's
record is kept and kept being re-probed, patiently, in case it comes back;
only where a live sibling already exists is the corpse dropped. Replicated
content, which is the norm, sheds its dead weight; the fragile
single-copy case keeps its lifeline.

And because the next milestone after "does it work" is "does it work
*efficiently*," the plan says to start counting now, before any of it is
tuned. So two gauges went in with the fix: how many dead-holder dials the
cache avoided, and how many stale records were aged out. They are the
early-warning lights for the day some future change quietly reopens the
leak — the number climbs, and you know where to look. You cannot make
efficient later what you never measured now.

One small scar worth recording, in the spirit of this log: mid-change, a
stray command to undo a throwaway edit reverted a whole file of real work
along with it. The failing-first test had already done its job — it went
red exactly where it should when the fix was absent — so the loss was
visible immediately and cost minutes, not hours. That is the entire case
for writing the test first, made in miniature.
