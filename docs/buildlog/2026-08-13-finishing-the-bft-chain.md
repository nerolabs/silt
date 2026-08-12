# Finishing the BFT chain

The one corner the substrate night couldn't green was the chain reorging
its own committed blocks. Research answered it, and the answer was not a
new mechanism — it was *finish the one you already have*.

The diagnosis was elegant and a little humbling. Silt's objective chain is
ninety percent a bond-weighted BFT chain: it gathers a Byzantine quorum,
it locks validators to the first block they sign, it calls the result
"committed." But bolted on top was a Nakamoto heaviest-chain fork-choice —
follow the heaviest fork — and during the first few blocks of a fresh
network that rule is fed a weight of *zero*. The launch anchors, the
training wheels a young network leans on, were deliberately given
eligibility but no weight, on the principle that only real staked bond
should ever move fork-choice. Correct in the mature regime; catastrophic
in the bootstrap, because a heaviest-chain protocol whose weight is zero
has, for those blocks, no fork-choice at all. It fell through to a tiebreak
that compared *head hashes* — and a genesis block whose hash happened to
sort low would win against a two-block committed chain. That is how
"committed height two" became "new head height zero." Not a heavier fork
winning; a coin-flip promoted to a rule.

The local simulation had passed the whole time, and that too was the
two-substrate discipline earning its keep: the sim only ever tested an
established chain with real, settled weights, where the tiebreak is the
rare symmetry-breaker it's supposed to be. It never modeled the ramp. So
the first thing built was not a fix but the missing scenario — a
deterministic bootstrap test with zero-bond anchors that reproduces the
reorg-to-zero on a laptop, in a millisecond, red before the fix. Only then
the fix.

Two of the three defects landed tonight, and they are the ones that
restore *convergence*. Anchors now carry a fixed bootstrap weight — their
eligibility *is* their weight during the young window, which is exactly
what the training-wheels principle always meant; it vanishes the moment the
network matures, so nothing about the mature security argument changes. And
the tiebreak now respects height: a shorter fork can never win a weight
tie against a taller one. With those two, a committed chain genuinely
outweighs a genesis fork and the oscillation stops. Alongside them, the
Byzantine quorum is now sized against a *stable* validator set — the fixed
anchor set during the ramp — instead of a live count that climbed
block-by-block as bond registrations drained in and left no fork ever
holding a quorum of a consistent set.

The third defect is the interesting one, and it is deliberately *not* here
yet. The owner ratified the bond-weighted BFT reading of the chain, which
means a quorum-committed block is *final* — never reverted, the way
Tendermint means it. Enforcing that is a small change, a rolling finality
floor generalizing the checkpoint machinery the chain already ships. But it
rewrites what objective fork-choice *means* — from "heal to the heavier
fork" to "a committed block is final" — and the red-team tests that encode
the old healing behavior are, on inspection, built on validators
double-signing, which under finality is a *slashing* event, not a heal.
Changing consensus safety tests because the model changed is legitimate,
but it is not something to do quietly at two in the morning while the owner
sleeps. So it waits for daylight and a second pair of eyes. The chain
converges tonight; it becomes provably final in the morning.
