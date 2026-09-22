package node

// three confirmed breaks on the inbound repair-claim path, landed as
// PINNED_DEFECT gates under
//
// ⚠ READ THE POLARITY BEFORE YOU READ A RESULT, AND IT IS NO LONGER UNIFORM.
//
// A pin asserts the tree's CURRENT, BROKEN behaviour: GREEN while the defect is
// present, RED when it is FIXED. A RED on a pin is not a regression; it is the pin
// doing its job. Skip-until-fixed was explicitly REFUSED, because a t.Skip costs the
// same lines and reports green either way. The mechanism and the precedent are
// core/pipeline's TestSingleDataShardStripeHasSixDistinctShards_PINNED_DEFECT and
// TestEntryFileSizeIsTheExactByteCount_PINNED_DEFECT.
//
// AS OF 2026-09-12, TWO OF THE PINS HAVE BEEN REDEEMED and this file holds a MIX:
//
//	PIN 1 arms (a) and (b) are still PINS. Arm (c) is NOT — it is a positive
//	 assertion that an out-of-range position is refused before any fetch.
//	PIN 2 NOT a pin. It asserts the dedup positively, and its name no longer
//	 carries _PINNED_DEFECT.
//	PIN 3 still a PIN, and it must STAY one. Its closer is the loss witness,
//	 which is GATED behind. A RED here means
//	 something gated was built.
//
// Both conversions followed the same route: the fix reddened the pin, and the
// pin was replaced by the positive assertion of the rule that made it red. No
// test was deleted.
//
// A pin records behaviour. It does NOT ratify that behaviour as correct, and it
// does not price a fix. The remedies routed for this gate are REFUTED on both
// arms: a per-sender rate budget breaks the healing precondition every other
// allowWindowed budget in silt rests on, because emitRepairClaim binds an empty
// reply callback and a refused claim is therefore lost forever
// (T-RETRY-IS-THE-PRECONDITION); and the check-ordering hoist cannot preserve
// the slash. PIN 1 stays pinned with no remedy owner.
//
// The three findings, each confirmed independently by two seats:
//
//	PIN 1 handleRepairClaim applies no per-sender rate limit, and the economy
//	 gate (`if !n.cfg.RepairEconomy`) sits inside settleRepairVerdict,
//	 which runs only AFTER fetchSurvivors. An unbounded stream of small
//	 claims from ONE sender therefore buys an unbounded stream of stripe
//	 fetches on the judge. THIS ONE FIRES ON THE SHIPPED DEFAULT.
//
//	PIN 2 credit.Ledger.PayBounty keys only on the ROOT — (root, repairer,
//	 amount) is its parameter list, not its key, and NO per-position state
//	 existed anywhere in the ledger. The judge kept no record of positions
//	 already paid, so the IDENTICAL claim replayed paid AGAIN out of the same
//	 escrow. ▶ CLOSED 2026-09-12 by node-side (root, stripe, pos) dedup on
//	 PAID; the gate below asserts the rule instead of the defect.
//
//	PIN 3 Nothing on the judge's path ever checks that the claimed shard was
//	 EVER MISSING. Correctness recomputes the position from survivors and
//	 retrievability challenges the named holder; both pass for a shard
//	 that was never lost and was merely COPIED onto a second holder.
//
// ▶ PIN 1 (c) CLOSED by the position screen in judgeRepairClaim: an
// out-of-range claim.ShardPos is judged against the manifest and refused before the
// survivor loop, so it now costs ZERO fetches where it used to cost all n, four
// times over.
//
// ▶ PIN 1 (b) CLOSED: fetchSurvivors passes fetchStripeByColumn a k-budget and the
// walk ends once k shards are in hand, so an honest in-range claim costs the judge
// k rather than the stripe width — 10 rather than 15 at the shipped k=10/n=16. The
// early exit is on SUCCESSFUL fetches, not on the ref list, so a short fetch still
// walks further rather than failing a claim it could have judged. Repair passes a
// nil budget deliberately: usedDomains is a census the re-seed reads, and a partial
// one would place a rebuilt shard into a domain the walk never looked at.
//
// ▶ ARM (a) IS UNTOUCHED AND STILL PINNED. No per-sender bound exists, and the
// remedy is refuted on its own precondition rather than merely unbuilt: claim
// emission binds an empty reply callback, so a claim a budget refuses is lost
// forever (T-RETRY-IS-THE-PRECONDITION). Budgeting the fetch narrows what each
// claim costs; it does not bound how many claims one sender may send.
//
// ⚠ THE FETCH IS n−1, NOT k — and there is no k anywhere in that path. An earlier
// revision of this file and of the judge's own
// source comment all said "k survivors". All three were false in the SAME
// direction: they understated the amplification this pin exists to hold.
// judgeRepairClaim builds survivorRefs as the COMPLEMENT of claim.ShardPos over
// the stripe's manifest-listed positions, and fetchSurvivors hands the whole
// slice to fetchStripeByColumn, which walks it to the end with no early exit once
// k shards are in hand. So an honest in-range claim costs n−1 fetches (15 at the
// shipped k=10/n=16), and an out-of-range claim.ShardPos — which nothing
// validates before the loop — excludes nothing and costs all n (16), one MORE
// than an honest claim, before VerifyByRecompute rejects it. The requirement is
// k, because repairproof.VerifyByRecompute genuinely needs k survivors; the FETCH
// is not budgeted to it. That gap is the whole of, and the pin's mechanism arms
// below assert it directly. Corrected in canon by.
//
// THE CONTRADICTION OF RECORD IS SETTLED — STATES THE INTENDED RULE. F8 ruled that
// a durability bounty pays for a REPAIR, so a shard that was never missing was
// never repaired. The shipped positive control TestRepair_HonestClaimIsPaid
// (repair_claim_adversary_test.go) used to build the own arrangement — the same
// stageShardOn no-loss staging — and assert the bounty IS paid, which made it an
// assertion that the defect was correct behaviour. It was re-derived over a REAL
// loss on 2026-09-13 and no longer agrees with this pin on anything.
//
// ⚠ THE PIN IS UNCHANGED AND STILL RED-WHEN-FIXED. Nothing about that re-derivation
// touched the judge, so a no-loss claim is still paid and the pin still holds.
// Its closer is the loss witness, and that is gated: a repair erases its own
// evidence. Do not read the positive
// control's new loss fixture as that witness — the fixture stages the loss for
// itself, the JUDGE still cannot see one.

import (
	"context"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// repairClaimFor is the one place these gates build a claim, so all three name the
// same five fields in the same order.
func repairClaimFor(root ports.Hash, stripe, pos int, shard ports.ChunkID, holder ports.NodeID) repairproof.RepairClaim {
	return repairproof.RepairClaim{Root: root, Stripe: stripe, ShardPos: pos, ShardID: shard, Holder: holder}
}

// ─────────────────────────────────────────────────────────────────────────────
// countStore wraps a ChunkStore and totals what the judge writes through it. The
// judge's survivor fetch lands chunks in its own store (fetchStripeByColumn) and
// then drops them again (fetchSurvivors' paramedic rule), so cumulative Put bytes
// is the honest measure of what one inbound claim costs the judge — a number that
// survives the drop.
//
// It also records the DISTINCT chunk ids written. That count is the one the
// mechanism arms read, and it is deliberately the retry-invariant measure: the
// judge's deferral path can re-judge the same claim, which multiplies bytes and
// Put calls but never widens the SET of shards a single claim reaches. The set
// size is n−1 or n and nothing else, so it isolates the complement-of-one-position
// mechanism from any question about how many rounds run.
// ─────────────────────────────────────────────────────────────────────────────
type countStore struct {
	inner    ports.ChunkStore
	putBytes int64
	puts     int
	distinct map[ports.ChunkID]bool
}

func newCountStore(inner ports.ChunkStore) *countStore {
	return &countStore{inner: inner, distinct: map[ports.ChunkID]bool{}}
}

func (c *countStore) Put(ctx context.Context, ch ports.Chunk) error {
	if err := c.inner.Put(ctx, ch); err != nil {
		return err
	}
	c.putBytes += int64(len(ch.Data))
	c.puts++
	c.distinct[ch.ID] = true
	return nil
}
func (c *countStore) Get(ctx context.Context, id ports.ChunkID) (ports.Chunk, error) {
	return c.inner.Get(ctx, id)
}
func (c *countStore) Has(ctx context.Context, id ports.ChunkID) (bool, error) {
	return c.inner.Has(ctx, id)
}
func (c *countStore) List(ctx context.Context) ([]ports.ChunkID, error) { return c.inner.List(ctx) }
func (c *countStore) Delete(ctx context.Context, id ports.ChunkID) error {
	return c.inner.Delete(ctx, id)
}

// ─────────────────────────────────────────────────────────────────────────────
// PIN 1 — the survivor fetch is UNBOUNDED per sender, and it is n−1 wide.
//
// THE RULE THIS PIN RECORDS THE ABSENCE OF: the work one sender can force out of
// a judge within one window must be BOUNDED, and the fetch should be budgeted to
// the k survivors VerifyByRecompute actually consumes. Neither holds.
//
// The claims here are deliberately NOT punishable: each names the REAL,
// manifest-committed shard id (so the correctness leg passes and nothing is
// slashed) and a holder that does not hold it (so the retrievability leg denies
// and nothing is paid). The sender therefore pays nothing at all — no slash, no
// fee, no standing — while the judge performs a full stripe fetch per claim.
//
// PIN. GREEN today. It goes RED when either half is repaired.
// ───────────────────────────────────────────────────────────────────────────── The
// pin, in three independently reddenable arms. Each returns "" while the pinned
// behaviour is present and the instruction once it is not. TEETH:
// TestRTRC_PinsFireOnTheirRemediations.

// rtRC1Amplification: a per-sender per-window bound would hold the window's total
// within a small constant of ONE claim's cost. It does not.
func rtRC1Amplification(claims int, one, many int64, reachedOne int) string {
	limit := 3 * one
	if many > limit {
		return ""
	}
	return fmt.Sprintf("PIN 1 PIN IS RED (amplification arm) — THE DEFECT IS FIXED OR HAS MOVED: %d claims from ONE sender cost the judge %d B, "+
		"within the %d B (3x one claim) that a per-sender per-window bound would allow. The pin asserts the UNBOUNDED behaviour, so this RED "+
		"means a bound now exists on the path (an admission check ahead of fetchSurvivors, or a per-sender window in handleRepairClaim).\n"+
		"  BEFORE REPLACING THIS PIN:\n"+
		"    1. Confirm the bound is real and not an artifact of the probe failing to reach fetchSurvivors — %d distinct shards were reached for one claim.\n"+
		"    2.  REFUTED the rate-budget remedy on its precondition: emitRepairClaim binds an EMPTY reply\n"+
		"       callback, so a refused claim is invisible to its sender and lost forever (T-RETRY-IS-THE-PRECONDITION). If a rate budget landed\n"+
		"       anyway, that refutation is now contradicted and the decision entry must be re-opened BEFORE this pin is retired.\n"+
		"    3. Then replace this arm with the positive assertion of whatever bound shipped.",
		claims, many, limit, reachedOne)
}

// rc1SurvivorBudget: THE POSITIVE ASSERTION THAT REPLACED PIN ARM (b).
// The pin it replaces recorded that survivorRefs is the complement of ONE position
// over the stripe and fetchStripeByColumn walked it to the end, so an honest
// in-range claim reached n−1 where VerifyByRecompute consumes k. fetchSurvivors now
// passes fetchStripeByColumn a k-budget and the walk ends once k shards are in hand,
// which reddened the pin — which is what a pin is for.
//
// ⚠ WHAT THIS COUNTS, AND THE OFF-BY-ONE IS DELIBERATE — measured 2026-09-12, held
// through the budget. It is DISTINCT CHUNKS WRITTEN, which is one MORE than the
// shards fetched (the object's manifest chunk) and one FEWER (the judge already
// hosted one of the refs, and fetchSurvivors does not re-Put what heldBefore
// holds). The two cancel, so the pinned figure was 15 for an n−1 walk and the
// asserted figure is k for a k-budgeted one. It is a correct assertion about store
// writes and NOT a direct count of survivor fetches; the fetch width itself is
// anchored in fetchSurvivors, where the budget is applied.
//
// THE UPPER BOUND IS ASSERTED SEPARATELY FROM THE EXACT VALUE. k alone would pass if
// the budget were replaced by something that happened to reach k for an unrelated
// reason on this fixture; requiring it to be strictly below the old n−1 width is what
// says the walk actually stopped early. The fixture guards n−1 > k, so the two arms
// cannot collapse into one.
// TEETH: TestRTRC_PinsFireOnTheirRemediations.
func rc1SurvivorBudget(reachedOne, k, nShards int) string {
	if reachedOne == k && reachedOne < nShards-1 {
		return ""
	}
	return fmt.Sprintf("THE SURVIVOR FETCH IS NO LONGER BUDGETED TO k — one honest in-range claim wrote %d distinct CHUNKS, want k = %d and strictly below n−1 = %d (n=%d).\n"+
		"  IF IT IS %d, the k-budget in fetchSurvivors was removed and the walk runs the whole complement again: an unsigned claim costs the judge\n"+
		"  the stripe width for a verification that consumes k, and the per-sender amplification arm above multiplies THAT number.\n"+
		"  IF IT IS BELOW k, the judge can no longer gather enough survivors to recompute and every honest claim now defers — check whether the\n"+
		"  budget is being applied to refs rather than to SUCCESSFUL fetches, which is the failure mode the early exit exists to avoid.\n"+
		"  IF IT IS ANYTHING ELSE, the complement-of-one-position construction in judgeRepairClaim changed; re-derive before re-asserting.",
		reachedOne, k, nShards-1, nShards, nShards-1)
}

// rtRC1OutOfRangeRefused: THE POSITIVE ASSERTION THAT REPLACED PIN ARM (c) ON
// 2026-09-12. The pin it replaces recorded that nothing validated claim.ShardPos
// before the survivor loop, so a position outside 0.n−1 excluded NOTHING and cost
// the judge all n — one MORE fetch than an honest claim — four times over, because
// the deferral predicate could not tell a structurally impossible position from a
// transient short fetch.
//
// judgeRepairClaim now screens the claimed position against the manifest before any
// fetch, direction A, so the cost is ZERO. reachedOne is the anti-vacuity witness:
// an honest in-range claim must still reach shards, or a zero here is a dead
// fixture rather than a screen.
func rtRC1OutOfRangeRefused(outOfRangePos, reachedOOR, reachedOne, nShards int) string {
	if reachedOOR == 0 && reachedOne > 0 {
		return ""
	}
	return fmt.Sprintf("PIN 1 (c) IS RED — a claim naming position %d, outside 0..%d, reached %d distinct shards; the position screen must refuse it at ZERO. "+
		"An honest in-range claim reached %d (n = %d).\n"+
		"  IF reachedOOR IS NOW %d: the screen has been removed or bypassed and the ORIGINAL DEFECT IS BACK — an invalid position excludes no ref, so it costs\n"+
		"  the judge one MORE fetch than an honest claim, and the cerr deferral path multiplies that by four. Restore the screen; do not re-pin.\n"+
		"  IF reachedOne IS 0: the fixture never reached fetchSurvivors at all, so this arm is measuring nothing. Fix the fixture before reading the zero above.",
		outOfRangePos, nShards-1, reachedOOR, reachedOne, nShards, nShards)
}

func TestSurvivorFetchIsUnboundedPerSender_PINNED_DEFECT(t *testing.T) {
	const claims = 8

	measure := func(n, pos int) (fetched int64, reached int, wire int, released, slashes, shards int) {
		s := newRepairAdv(t, 1201)
		s.fundEscrow(5_000_000)
		judge := s.careJudge()
		cs := newCountStore(judge.store)
		judge.store = cs

		_, parityID, _ := s.parityTarget()
		attacker := s.nodes[2]
		s.bond(attacker)
		// A holder that does not hold this parity shard: the retrievability leg
		// denies, so nothing is ever paid and nothing is ever slashed.
		claim := repairClaimFor(s.root, 0, pos, parityID, s.nodes[7].ID())
		data, err := claim.Marshal()
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		for i := 0; i < n; i++ {
			judge.handleRepairClaim(attacker.ID(), ports.Message{Kind: ports.MsgRepairClaim, Data: data})
			s.sched.Run()
		}
		return cs.putBytes, len(cs.distinct), len(data), judge.Stats.BountiesReleased, judge.Stats.FalseRepairSlashes, s.shardFetches(cs)
	}

	// The stripe geometry comes from the fixture's own manifest, so the arms below
	// assert the DERIVATION (n−1, n) and not a transcribed constant.
	geom := newRepairAdv(t, 1201)
	k, nShards := geom.m.K, geom.m.N
	inRangePos, _, _ := geom.parityTarget()
	outOfRangePos := nShards + 5

	one, reachedOne, wire, rel1, sl1, shardsOne := measure(1, inRangePos)
	many, _, _, relN, slN, _ := measure(claims, inRangePos)

	t.Logf("PIN 1 MEASURED: k=%d n=%d; claim wire size = %d B; judge store writes for 1 claim = %d B over %d DISTINCT chunks, of which %d are the OBJECT'S OWN shards; for %d claims = %d B (%.1f x one claim)",
		k, nShards, wire, one, reachedOne, shardsOne, claims, many, float64(many)/float64(one))
	t.Logf("PIN 1 the sender pays NOTHING for either arm: bounties released 1-arm=%d N-arm=%d, false-repair slashes 1-arm=%d N-arm=%d",
		rel1, relN, sl1, slN)

	// ── Preconditions. A pin that measures nothing is worse than no pin. ──
	if one <= 0 {
		t.Fatalf("PIN 1 VACUOUS: one claim caused %d bytes of survivor fetch — the probe never reached fetchSurvivors, so it measures nothing", one)
	}
	if rel1 != 0 || relN != 0 {
		t.Fatalf("PIN 1 PREMISE BROKEN: a bounty was released (%d/%d) — this probe must be the UNPAID arm so the cost is pure amplification", rel1, relN)
	}
	if sl1 != 0 || slN != 0 {
		t.Fatalf("PIN 1 PREMISE BROKEN: the sender was slashed (%d/%d) — this probe must name the REAL shard id so no punishment offsets the cost", sl1, slN)
	}
	if nShards-1 == k {
		t.Fatalf("PIN 1 FIXTURE DEGENERATE: the fixture stripe has k=%d n=%d, so n−1 == k and the mechanism arm below cannot distinguish "+
			"a fetch budgeted to k from one that is not. Re-point the fixture at a stripe with n−1 > k before believing this pin.", k, nShards)
	}

	// ── PIN (a) — the amplification is UNBOUNDED per sender. ──
	// A per-sender per-window cap would hold the window's total within a small
	// constant of ONE claim's cost, however many claims arrive. It does not.
	if msg := rtRC1Amplification(claims, one, many, reachedOne); msg != "" {
		t.Fatal(msg)
	}

	// ── PIN (b) — an in-range claim reaches n−1 shards, NOT k. ──
	// This is the arm that holds the record correction. survivorRefs is the
	// complement of ONE position over the stripe, and fetchStripeByColumn walks it
	// to the end; VerifyByRecompute needs only k.
	if msg := rc1SurvivorBudget(reachedOne, k, nShards); msg != "" {
		t.Fatal(msg)
	}

	// ── (c), NO LONGER A PIN — an OUT-OF-RANGE position is REFUSED BEFORE THE FETCH. ──
	// Converted from a pin to a positive assertion on 2026-09-12 when direction A's
	// position screen landed and reddened it, which is what a pin is for. The screen
	// judges claim.ShardPos against the manifest alone, so an invalid position now
	// matches no ref AND buys no fetch, where it used to buy all n.
	_, reachedOOR, _, relOOR, slOOR, shardsOOR := measure(1, outOfRangePos)
	t.Logf("PIN 1 MEASURED (out-of-range pos=%d): %d of the OBJECT'S OWN shards fetched (%d chunks written in all, the extra one being the manifest); bounties=%d slashes=%d",
		outOfRangePos, shardsOOR, reachedOOR, relOOR, slOOR)
	if msg := rtRC1OutOfRangeRefused(outOfRangePos, shardsOOR, shardsOne, nShards); msg != "" {
		t.Fatal(msg)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PIN 2 — a replay of the SAME claim pays AGAIN.
//
// THE RULE THIS PIN RECORDS THE ABSENCE OF: a (root, stripe, position) is
// repaired once, so the second byte-identical claim for that position should draw
// nothing from the escrow. It draws the full bounty a second time.
//
// PIN. GREEN today. It goes RED when the dedup lands.
// ─────────────────────────────────────────────────────────────────────────────
// pin2Dedup: THE POSITIVE ASSERTION THAT REPLACED PIN 2.
// The pin it replaces recorded that a byte-identical replay drew a SECOND payment of
// the same size out of the same escrow and released a second time. Node-side dedup
// keyed (root, stripe, position) on PAID landed and reddened it, which is what a pin
// is for, direction D.
//
// The rule asserted now: a position this judge has already PAID for draws ZERO on
// every later claim, releases nothing further, and the refusal is COUNTED. The
// counter matters — a silent refusal reads in the journal exactly like a lost claim.
// The record lives on the Node because credit.Ledger keys its escrow on ROOT ALONE
// and carries no per-position state at all.
// TEETH: TestRTRC_PinsFireOnTheirRemediations.
func pin2Dedup(firstPaid, secondPaid int64, firstReleases, secondReleases, duplicates, pos int, rootPfx, shardPfx []byte, holder string) string {
	if secondPaid == firstPaid && secondReleases == firstReleases && duplicates == 1 {
		return ""
	}
	return fmt.Sprintf("PIN 2 IS RED — a replayed claim for an already-PAID position did not draw zero: EscrowPaid %d -> %d (delta %d, want 0), "+
		"BountiesReleased %d -> %d (want unchanged), duplicates refused = %d (want 1). Claim was root=%x stripe=0 pos=%d shard=%x holder=%s.\n"+
		"  IF THE DELTA IS %d, THE ORIGINAL DEFECT IS BACK: the replay paid the full bounty again, and the drain is DIRECTED — repairproof.RepairClaim\n"+
		"  is unsigned and the payee is claim.Holder, a field of the message — so the size of the per-replay draw is the severity.\n"+
		"  IF THE DELTA IS 0 BUT duplicates IS 0: the claim was refused somewhere EARLIER than the dedup, so this gate is green for a cause it never\n"+
		"  reaches. Find what refused it before believing the zero.\n"+
		"  ⚠ IF THE RECORD WAS MOVED FROM PAID TO JUDGED to make this pass: that is REFUTED. A judged-but-unpaid position must stay payable, and\n"+
		"  TestRepairJudge_DeniedPositionStaysPayable is the control that fires on it.",
		firstPaid, secondPaid, secondPaid-firstPaid, firstReleases, secondReleases, duplicates,
		rootPfx, pos, shardPfx, holder, firstPaid)
}

func TestReplayedClaimForAPaidPositionDrawsNothing(t *testing.T) {
	s := newRepairAdv(t, 1202)
	s.fundEscrow(5_000_000)
	judge := s.careJudge()

	pos, parityID, leafIdx := s.parityTarget()
	holder := s.nodes[9]
	s.stageShardOn(judge, holder, parityID, pos, leafIdx)

	claim := repairClaimFor(s.root, 0, pos, parityID, holder.ID())

	s.deliverClaim(judge, s.nodes[2].ID(), claim)
	firstPaid := s.ledger.EscrowPaid(s.root)
	firstReleases := judge.Stats.BountiesReleased
	if firstPaid <= 0 || firstReleases != 1 {
		t.Fatalf("PIN 2 PREMISE BROKEN: the FIRST claim did not pay (EscrowPaid=%d, BountiesReleased=%d) — a replay gate needs a paid first claim to replay",
			firstPaid, firstReleases)
	}

	// The same claim, byte-identical, delivered again by the same sender.
	s.deliverClaim(judge, s.nodes[2].ID(), claim)
	secondPaid := s.ledger.EscrowPaid(s.root)
	secondReleases := judge.Stats.BountiesReleased

	t.Logf("PIN 2 MEASURED: EscrowPaid after 1st claim = %d, after the identical replay = %d (delta %d); BountiesReleased %d -> %d; duplicates refused = %d",
		firstPaid, secondPaid, secondPaid-firstPaid, firstReleases, secondReleases, judge.Stats.BountyDuplicatePosition)

	if msg := pin2Dedup(firstPaid, secondPaid, firstReleases, secondReleases,
		judge.Stats.BountyDuplicatePosition, pos,
		s.root[:4], parityID[:4], holder.ID().String()[:8]); msg != "" {
		t.Fatal(msg)
	}

	// THE KEY IS THE POSITION, NOT THE ROOT. Without this arm a dedup that refused
	// every second claim on the object would pass the assertion above, and a real
	// second repair of a different position would silently stop being paid.
	otherPos, otherID, otherLeaf := s.m.K+1, s.m.ParityIDs()[1], len(s.m.ChunkIDs())+1
	if otherID == parityID {
		t.Fatal("PIN 2 PREMISE BROKEN: the fixture's first two parity shards share an id, so the second arm does not name a different position")
	}
	s.stageShardOn(judge, holder, otherID, otherPos, otherLeaf)
	s.deliverClaim(judge, s.nodes[2].ID(), repairClaimFor(s.root, 0, otherPos, otherID, holder.ID()))
	if judge.Stats.BountiesReleased != firstReleases+1 {
		t.Fatalf("a DIFFERENT position of the same object was refused: BountiesReleased=%d, want %d. The dedup key is (root, stripe, position); "+
			"keying it on the root alone would stop paying every repair after the first",
			judge.Stats.BountiesReleased, firstReleases+1)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// PIN 3 — a claim with NO LOSS is PAID.
//
// THE RULE THIS PIN RECORDS THE ABSENCE OF: a durability bounty pays for a
// REPAIR, so a shard that was never missing was never repaired, and copying it
// onto a second holder should pay nothing. It pays the full bounty.
//
// The arrangement is the no-loss one, and the pin PROVES the no-loss premise
// before it judges: the shard is still retrievable from its ORIGINAL providers at
// the moment the claim is delivered. Nothing was rebuilt; a live copy was moved.
//
// ⚠ THIS PIN USED TO AGREE WITH A SHIPPED TEST, AND THAT AGREEMENT WAS THE
// FINDING. TestRepair_HonestClaimIsPaid (repair_claim_adversary_test.go) built
// the identical arrangement with the same stageShardOn helper and asserted
// BountiesReleased == 1 as the INTENDED rule, while this pin asserted the same
// outcome as a DEFECT. resolved it in THIS pin's favour, and on 2026-09-13 that
// control was re-derived over a REAL loss — a position destroyed across the swarm
// and rebuilt from the survivors — so the two arrangements are now disjoint. This
// one is still the no-loss arrangement and is still the only test in the tree
// that asserts a no-loss claim pays.
//
// PIN. GREEN today. It goes RED when a prior-loss requirement lands.
// ─────────────────────────────────────────────────────────────────────────────
// rtRC3Pin returns "" while a claim for a shard that was NEVER LOST still pays, once,
// and the instruction once it does not.
// TEETH: TestRTRC_PinsFireOnTheirRemediations.
func rtRC3Pin(before, after int64, releases int) string {
	if after > before && releases == 1 {
		return ""
	}
	return fmt.Sprintf("PIN 3 PIN IS RED — a claim for a shard that was NEVER LOST no longer pays as pinned: EscrowPaid %d -> %d, BountiesReleased=%d (pinned at a positive delta and exactly 1).\n"+
		"  THE FIX CASE: no payment and no release means the judge now requires evidence of prior loss for the claimed (root, stripe, position).\n"+
		"  That is the repair this pin was waiting for, and since 2026-09-13 it CAN land alone: TestRepair_HonestClaimIsPaid was\n"+
		"  re-derived over a REAL loss and no longer asserts the opposite on this arrangement, so it\n"+
		"  should stay GREEN in the same run.\n"+
		"  IF IT WENT RED TOO, CHECK THE FIXTURE BEFORE YOU CHECK PAYMENT. Two causes are known and this list is NOT exhaustive:\n"+
		"    (a) the loss witness is shaped as a RESTORATION DIFFERENTIAL — the position unreachable before the claim, reachable after.\n"+
		"        That control cannot satisfy that shape today: rebuildLostShard places through placeAt, which sends MsgStoreChunk and\n"+
		"        announces NOTHING to the nodes near the column key, so after the rebuild the position is not discoverable under\n"+
		"        colKey(root, pos) and lives on ONE node, down from three (measured 2026-09-13). Teach rebuildLostShard to announce\n"+
		"        before you conclude anything about payment.\n"+
		"    (b) payment itself broke.\n"+
		"  Retire this pin by replacing it with the positive assertion of the rule, never by deleting it. The judge's two legs (correctness\n"+
		"  recompute in judgeRepairClaim, retrievability in\n"+
		"  challengeHolderRetrievability) both pass for a live shard merely COPIED to a new holder; nothing on the path asserted prior loss.\n"+
		"  THE OTHER CASE: more than one release, or a payment where the premise arm above did not hold, is a different defect. Re-derive first.",
		before, after, releases)
}

func TestClaimWithNoLossIsPaid_PINNED_DEFECT(t *testing.T) {
	s := newRepairAdv(t, 1203)
	s.fundEscrow(5_000_000)
	judge := s.careJudge()

	pos, parityID, leafIdx := s.parityTarget()
	holder := s.nodes[9]
	s.stageShardOn(judge, holder, parityID, pos, leafIdx)

	// PROVE the no-loss premise: the shard is still live on the swarm, reachable
	// from the column's own providers, independently of the copy just staged.
	probe := s.nodes[3]
	reachable := false
	probe.resolveProviders(colKey(s.root, pos), func(provs []ports.NodeID) {
		for _, p := range provs {
			if p == holder.ID() {
				continue // the freshly-staged copy does not count as a survivor
			}
			reachable = true
		}
	})
	s.sched.Run()
	if !reachable {
		t.Fatalf("PIN 3 PREMISE BROKEN: no provider other than the staged holder answers for column %d — the shard may actually have been lost, so this is not a no-loss arrangement", pos)
	}

	before := s.ledger.EscrowPaid(s.root)
	claim := repairClaimFor(s.root, 0, pos, parityID, holder.ID())
	s.deliverClaim(judge, s.nodes[2].ID(), claim)
	after := s.ledger.EscrowPaid(s.root)

	t.Logf("PIN 3 MEASURED: the shard was NEVER missing (live on its original providers) and a copy was staged on %s; EscrowPaid %d -> %d, BountiesReleased=%d",
		holder.ID().String()[:8], before, after, judge.Stats.BountiesReleased)

	// PIN: the no-loss claim is paid, once.
	if msg := rtRC3Pin(before, after, judge.Stats.BountiesReleased); msg != "" {
		t.Fatal(msg)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// THE TEETH. Each PIN pin above is a pure predicate; this test feeds each one the
// value it must FIRE on and the value it must accept.
//
// A pin that cannot fire reports green forever and is indistinguishable from one that
// works. Ablating the product is the right evidence, but an ablation that lives only in
// prose does not survive a context reset. Same mechanism as
// TestPinRedensWhenFileSizeIsBlinded.
//
// THIS TEST IS NOT A PIN. Its polarity is ordinary: GREEN when each predicate can
// fire. It covers the CONVERTED predicates too — a positive assertion that cannot
// fire is decoration in exactly the way a pin that cannot fire is. Delete it in the
// same commit that retires the last predicate above.
//
// The THREE ARMS ARE EXERCISED SEPARATELY, deliberately. The ablations
// showed each arm is independently reddenable (a k-budget reddens the in-range arm
// alone; a ShardPos validation reddens the out-of-range arm alone), and a teeth test
// that only drove one arm would let the other two rot unobserved.
// ─────────────────────────────────────────────────────────────────────────────

func TestRTRC_PinsFireOnTheirRemediations(t *testing.T) {
	// The shipped geometry PIN 1 measures against: k=10, n=16.
	const k, nShards = 10, 16

	// --- PIN 1 (a), amplification. Pinned: 8 claims cost ~8x one claim. ---
	if msg := rtRC1Amplification(8, 1000, 8000, nShards-1); msg != "" {
		t.Fatalf("rtRC1Amplification fired on the unbounded state it is pinned to accept: %s", msg)
	}
	if rtRC1Amplification(8, 1000, 1000, nShards-1) == "" {
		t.Fatal("rtRC1Amplification stayed silent when 8 claims cost no more than ONE — a per-sender " +
			"per-window bound landing is exactly what this arm exists to detect")
	}
	if rtRC1Amplification(8, 1000, 3000, nShards-1) == "" {
		t.Fatal("rtRC1Amplification stayed silent at exactly the 3x boundary — the arm must fire at the " +
			"limit a bound would allow, not only strictly inside it")
	}

	// --- (b), in-range width. NO LONGER A PIN: asserted at the k budget. ---
	if msg := rc1SurvivorBudget(k, k, nShards); msg != "" {
		t.Fatalf("rc1SurvivorBudget fired on the budgeted width it asserts: %s", msg)
	}
	if rc1SurvivorBudget(nShards-1, k, nShards) == "" {
		t.Fatal("rc1SurvivorBudget stayed silent on the OLD n−1 width — the survivor budget " +
			"landing is what this arm exists to detect, and it is the number corrected")
	}

	// --- PIN 1 (c), out-of-range. NO LONGER A PIN: asserts the screen refuses at ZERO. ---
	if msg := rtRC1OutOfRangeRefused(nShards+5, 0, nShards-1, nShards); msg != "" {
		t.Fatalf("rtRC1OutOfRangeRefused fired on the screened state it asserts: %s", msg)
	}
	if rtRC1OutOfRangeRefused(nShards+5, nShards, nShards-1, nShards) == "" {
		t.Fatal("rtRC1OutOfRangeRefused stayed silent when an invalid position fetched all n — that is " +
			"the ORIGINAL defect returning, and this arm exists to catch the screen being removed")
	}
	if rtRC1OutOfRangeRefused(nShards+5, 1, nShards-1, nShards) == "" {
		t.Fatal("rtRC1OutOfRangeRefused stayed silent at ONE fetch — the assertion is zero fetches, not " +
			"fewer than an honest claim, or a screen that leaked a single fetch would land unnoticed")
	}
	if rtRC1OutOfRangeRefused(nShards+5, 0, 0, nShards) == "" {
		t.Fatal("rtRC1OutOfRangeRefused stayed silent when the HONEST arm also reached zero — that is a " +
			"dead fixture reporting a screen, the vacuity this arm's anti-vacuity witness exists to catch")
	}

	// ---, a replay of a PAID position draws nothing. NO LONGER A PIN. ---
	pfx := []byte{1, 2, 3, 4}
	if msg := pin2Dedup(100, 100, 1, 1, 1, 3, pfx, pfx, "holder00"); msg != "" {
		t.Fatalf("pin2Dedup fired on the deduped state it asserts: %s", msg)
	}
	if pin2Dedup(100, 200, 1, 2, 0, 3, pfx, pfx, "holder00") == "" {
		t.Fatal("pin2Dedup stayed silent when the replay paid the full bounty AGAIN and released a " +
			"second time — that is the original PIN 2 defect returning")
	}
	if pin2Dedup(100, 400, 1, 2, 0, 3, pfx, pfx, "holder00") == "" {
		t.Fatal("pin2Dedup stayed silent when the replay paid MORE than the first claim — the WORSE " +
			"case the message names would land unnoticed")
	}
	if pin2Dedup(100, 100, 1, 1, 0, 3, pfx, pfx, "holder00") == "" {
		t.Fatal("pin2Dedup stayed silent when the replay drew zero but NO duplicate was counted — a " +
			"gate green for a cause it never reaches is the fixture-posture scar, and the counter is " +
			"what distinguishes 'the dedup refused it' from 'something earlier refused it'")
	}

	// ---, a no-loss claim is paid. ---
	if msg := rtRC3Pin(0, 5000, 1); msg != "" {
		t.Fatalf("rtRC3Pin fired on the pinned no-loss payment: %s", msg)
	}
	if rtRC3Pin(0, 0, 0) == "" {
		t.Fatal("rtRC3Pin stayed silent when the no-loss claim paid nothing — a prior-loss requirement " +
			"landing is what this pin exists to detect, and it is the RED that must be read together " +
			"with TestRepair_HonestClaimIsPaid")
	}
	if rtRC3Pin(0, 5000, 2) == "" {
		t.Fatal("rtRC3Pin stayed silent on TWO releases — 'paid, once' is the pinned shape and a " +
			"double release is a different defect")
	}
}
