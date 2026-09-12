package node

// RT-RC — three confirmed breaks on the inbound repair-claim path, landed as
// PINNED_DEFECT gates under D-REPAIR-CLAIM-GATES-PINNED-2026-09-12.
//
// ⚠ READ THE POLARITY BEFORE YOU READ A RESULT.
//
// These three tests assert the tree's CURRENT, BROKEN behaviour. They are GREEN
// while the defect is present and go RED when it is FIXED. A RED here is not a
// regression; it is the pin doing its job. The ratified posture is that a pin
// "asserts current broken behaviour, named so that a future fix REDDENS the pin
// and forces the record to be updated" — skip-until-fixed was explicitly
// REFUSED, because a t.Skip costs the same lines and reports green either way
// (scar:short-run-is-zero-execution). The mechanism and the precedent are
// core/pipeline's TestRT_SFO_4_..._PINNED_DEFECT and
// TestRT_SFO_5_..._PINNED_DEFECT, shipped in #817.
//
// A pin records behaviour. It does NOT ratify that behaviour as correct, and it
// does not price a fix. The remedies routed for RT-RC-1 are REFUTED on both arms
// (D-REPAIR-RATE-LIMIT-REFUTED-2026-09-12): a per-sender rate budget breaks the
// healing precondition every other allowWindowed budget in silt rests on,
// because emitRepairClaim binds an empty reply callback and a refused claim is
// therefore lost forever (T-RETRY-IS-THE-PRECONDITION); and the check-ordering
// hoist cannot preserve the slash. RT-RC-1 stays pinned with no remedy owner.
//
// The three findings, each confirmed independently by two seats:
//
//	RT-RC-1  handleRepairClaim applies no per-sender rate limit, and the economy
//	         gate (`if !n.cfg.RepairEconomy`) sits inside settleRepairVerdict,
//	         which runs only AFTER fetchSurvivors. An unbounded stream of small
//	         claims from ONE sender therefore buys an unbounded stream of stripe
//	         fetches on the judge. THIS ONE FIRES ON THE SHIPPED DEFAULT.
//
//	RT-RC-2  credit.Ledger.PayBounty keys only on (root, repairer, amount). It
//	         carries no (root, stripe, pos) dedup and the judge keeps no record
//	         of positions already paid, so the IDENTICAL claim replayed pays
//	         AGAIN out of the same escrow.
//
//	RT-RC-3  Nothing on the judge's path ever checks that the claimed shard was
//	         EVER MISSING. Correctness recomputes the position from survivors and
//	         retrievability challenges the named holder; both pass for a shard
//	         that was never lost and was merely COPIED onto a second holder.
//
// ⚠ THE FETCH IS n−1, NOT k — and there is no k anywhere in that path. An
// earlier revision of this file, of the ratified decision entry, and of the
// judge's own source comment all said "k survivors". All three were false in the
// SAME direction: they understated the amplification this pin exists to hold.
// judgeRepairClaim builds survivorRefs as the COMPLEMENT of claim.ShardPos over
// the stripe's manifest-listed positions, and fetchSurvivors hands the whole
// slice to fetchStripeByColumn, which walks it to the end with no early exit once
// k shards are in hand. So an honest in-range claim costs n−1 fetches (15 at the
// shipped k=10/n=16), and an out-of-range claim.ShardPos — which nothing
// validates before the loop — excludes nothing and costs all n (16), one MORE
// than an honest claim, before VerifyByRecompute rejects it. The requirement is
// k, because repairproof.VerifyByRecompute genuinely needs k survivors; the
// FETCH is not budgeted to it. That gap is the whole of RT-RC-1, and the pin's
// mechanism arms below assert it directly. Corrected in canon by #844.
//
// CONTRADICTION OF RECORD — do not resolve it in this file. The shipped positive
// control TestRedteamRepair_HonestClaimIsPaid (redteam_repair_claim_test.go)
// stages a shard with the SAME helper RT-RC-3 uses (stageShardOn: fetch the live
// shard from its providers, re-place a copy on a fresh holder) and asserts the
// bounty IS paid. RT-RC-3 pins the same outcome for the opposite reason: the
// shipped test reads it as the intended rule, RT-RC-3 reads it as the defect.
// Both are left standing on purpose. Which one states the intended rule is the
// owner's call, not a seat's.

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
// RT-RC-1 — the survivor fetch is UNBOUNDED per sender, and it is n−1 wide.
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
// ─────────────────────────────────────────────────────────────────────────────
// The RT-RC-1 pin, in three independently reddenable arms. Each returns "" while the
// pinned behaviour is present and the instruction once it is not.
// TEETH: TestRTRC_PinsFireOnTheirRemediations.

// rtRC1Amplification: a per-sender per-window bound would hold the window's total
// within a small constant of ONE claim's cost. It does not.
func rtRC1Amplification(claims int, one, many int64, reachedOne int) string {
	limit := 3 * one
	if many > limit {
		return ""
	}
	return fmt.Sprintf("RT-RC-1 PIN IS RED (amplification arm) — THE DEFECT IS FIXED OR HAS MOVED: %d claims from ONE sender cost the judge %d B, "+
		"within the %d B (3x one claim) that a per-sender per-window bound would allow. The pin asserts the UNBOUNDED behaviour, so this RED "+
		"means a bound now exists on the path (an admission check ahead of fetchSurvivors, or a per-sender window in handleRepairClaim).\n"+
		"  BEFORE REPLACING THIS PIN:\n"+
		"    1. Confirm the bound is real and not an artifact of the probe failing to reach fetchSurvivors — %d distinct shards were reached for one claim.\n"+
		"    2. D-REPAIR-RATE-LIMIT-REFUTED-2026-09-12 REFUTED the rate-budget remedy on its precondition: emitRepairClaim binds an EMPTY reply\n"+
		"       callback, so a refused claim is invisible to its sender and lost forever (T-RETRY-IS-THE-PRECONDITION). If a rate budget landed\n"+
		"       anyway, that refutation is now contradicted and the decision entry must be re-opened BEFORE this pin is retired.\n"+
		"    3. Then replace this arm with the positive assertion of whatever bound shipped.",
		claims, many, limit, reachedOne)
}

// rtRC1InRange: survivorRefs is the complement of ONE position over the stripe and
// fetchStripeByColumn walks it to the end, so an honest in-range claim reaches n−1,
// not the k that VerifyByRecompute consumes. This arm holds the #844 correction.
func rtRC1InRange(reachedOne, k, nShards int) string {
	if reachedOne == nShards-1 {
		return ""
	}
	return fmt.Sprintf("RT-RC-1 PIN IS RED (mechanism arm, in-range) — one honest in-range claim reached %d distinct shards, pinned at n−1 = %d (k=%d, n=%d).\n"+
		"  If it is now %d, the fetch has been budgeted to k and the RT-RC-1 mechanism is repaired — retire this arm and assert the budget positively.\n"+
		"  If it is anything else, the complement-of-one-position construction in judgeRepairClaim changed. Re-derive before re-pinning: this number\n"+
		"  is the per-claim cost the amplification arm above multiplies, and #844 corrected canon on exactly this word (it said k; it is n−1).",
		reachedOne, nShards-1, k, nShards, k)
}

// rtRC1OutOfRange: nothing validates claim.ShardPos before the survivor loop, so a
// position outside 0..n−1 excludes NOTHING and costs the judge one MORE fetch than an
// honest claim.
func rtRC1OutOfRange(outOfRangePos, reachedOOR, reachedOne, nShards int) string {
	if reachedOOR == nShards && reachedOOR > reachedOne {
		return ""
	}
	return fmt.Sprintf("RT-RC-1 PIN IS RED (mechanism arm, out-of-range) — a claim naming position %d, outside 0..%d, reached %d distinct shards; "+
		"pinned at n = %d, which is strictly MORE than the %d an in-range claim reaches.\n"+
		"  The pinned defect is that nothing validates claim.ShardPos before the survivor loop, so an invalid position excludes no ref and costs\n"+
		"  the judge one MORE fetch than an honest claim before VerifyByRecompute rejects it. If this is now <= the in-range count, a position\n"+
		"  validation landed ahead of the fetch — good — and this arm must be replaced by the positive assertion that an out-of-range position is\n"+
		"  refused before any fetch. Re-derive before re-pinning.",
		outOfRangePos, nShards-1, reachedOOR, nShards, reachedOne)
}

func TestRTRC1_SurvivorFetchIsUnboundedPerSenderAndIsNMinusOneWide_PINNED_DEFECT(t *testing.T) {
	const claims = 8

	measure := func(n, pos int) (fetched int64, reached int, wire int, released, slashes int) {
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
		return cs.putBytes, len(cs.distinct), len(data), judge.Stats.BountiesReleased, judge.Stats.FalseRepairSlashes
	}

	// The stripe geometry comes from the fixture's own manifest, so the arms below
	// assert the DERIVATION (n−1, n) and not a transcribed constant.
	geom := newRepairAdv(t, 1201)
	k, nShards := geom.m.K, geom.m.N
	inRangePos, _, _ := geom.parityTarget()
	outOfRangePos := nShards + 5

	one, reachedOne, wire, rel1, sl1 := measure(1, inRangePos)
	many, _, _, relN, slN := measure(claims, inRangePos)

	t.Logf("RT-RC-1 MEASURED: k=%d n=%d; claim wire size = %d B; judge store writes for 1 claim = %d B over %d DISTINCT shards; for %d claims = %d B (%.1f x one claim)",
		k, nShards, wire, one, reachedOne, claims, many, float64(many)/float64(one))
	t.Logf("RT-RC-1 the sender pays NOTHING for either arm: bounties released 1-arm=%d N-arm=%d, false-repair slashes 1-arm=%d N-arm=%d",
		rel1, relN, sl1, slN)

	// ── Preconditions. A pin that measures nothing is worse than no pin. ──
	if one <= 0 {
		t.Fatalf("RT-RC-1 VACUOUS: one claim caused %d bytes of survivor fetch — the probe never reached fetchSurvivors, so it measures nothing", one)
	}
	if rel1 != 0 || relN != 0 {
		t.Fatalf("RT-RC-1 PREMISE BROKEN: a bounty was released (%d/%d) — this probe must be the UNPAID arm so the cost is pure amplification", rel1, relN)
	}
	if sl1 != 0 || slN != 0 {
		t.Fatalf("RT-RC-1 PREMISE BROKEN: the sender was slashed (%d/%d) — this probe must name the REAL shard id so no punishment offsets the cost", sl1, slN)
	}
	if nShards-1 == k {
		t.Fatalf("RT-RC-1 FIXTURE DEGENERATE: the fixture stripe has k=%d n=%d, so n−1 == k and the mechanism arm below cannot distinguish "+
			"a fetch budgeted to k from one that is not. Re-point the fixture at a stripe with n−1 > k before believing this pin.", k, nShards)
	}

	// ── PIN (a) — the amplification is UNBOUNDED per sender. ──
	// A per-sender per-window cap would hold the window's total within a small
	// constant of ONE claim's cost, however many claims arrive. It does not.
	if msg := rtRC1Amplification(claims, one, many, reachedOne); msg != "" {
		t.Fatal(msg)
	}

	// ── PIN (b) — an in-range claim reaches n−1 shards, NOT k. ──
	// This is the arm that holds the #844 record correction. survivorRefs is the
	// complement of ONE position over the stripe, and fetchStripeByColumn walks it
	// to the end; VerifyByRecompute needs only k.
	if msg := rtRC1InRange(reachedOne, k, nShards); msg != "" {
		t.Fatal(msg)
	}

	// ── PIN (c) — an OUT-OF-RANGE position is WORSE than an honest one. ──
	// Nothing validates claim.ShardPos before the loop, so a position outside
	// 0..n−1 matches no ref, excludes NOTHING, and the judge fetches all n.
	// Distinct shards is the retry-invariant measure, so this arm says nothing
	// about how many judging rounds an unjudgeable claim triggers.
	_, reachedOOR, _, relOOR, slOOR := measure(1, outOfRangePos)
	t.Logf("RT-RC-1 MEASURED (out-of-range pos=%d): reached %d distinct shards; bounties=%d slashes=%d", outOfRangePos, reachedOOR, relOOR, slOOR)
	if msg := rtRC1OutOfRange(outOfRangePos, reachedOOR, reachedOne, nShards); msg != "" {
		t.Fatal(msg)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RT-RC-2 — a replay of the SAME claim pays AGAIN.
//
// THE RULE THIS PIN RECORDS THE ABSENCE OF: a (root, stripe, position) is
// repaired once, so the second byte-identical claim for that position should draw
// nothing from the escrow. It draws the full bounty a second time.
//
// PIN. GREEN today. It goes RED when the dedup lands.
// ─────────────────────────────────────────────────────────────────────────────
// rtRC2Pin returns "" while a byte-identical replay still draws a SECOND payment of
// the same size and a second release, and the instruction once it does not. The delta
// is pinned against the FIRST payment so the assertion stays derived from the run's
// own bounty price rather than a transcribed amount.
// TEETH: TestRTRC_PinsFireOnTheirRemediations.
func rtRC2Pin(firstPaid, secondPaid int64, firstReleases, secondReleases, pos int, rootPfx, shardPfx []byte, holder string) string {
	if secondPaid-firstPaid == firstPaid && secondReleases == firstReleases+1 {
		return ""
	}
	return fmt.Sprintf("RT-RC-2 PIN IS RED — the replay no longer pays the same again: EscrowPaid %d -> %d (delta %d, pinned at %d), "+
		"BountiesReleased %d -> %d (pinned at %d). Claim was root=%x stripe=0 pos=%d shard=%x holder=%s.\n"+
		"  THE FIX CASE: delta 0 and no new release means a (root, stripe, position) dedup landed — retire this pin and assert positively that a\n"+
		"  position already paid on this judge's ledger pays 0 on every later claim. credit.Ledger.PayBounty took only (root, repairer, amount)\n"+
		"  and settleRepairVerdict called it with no dedup of its own; name where the dedup now lives.\n"+
		"  THE WORSE CASE: a delta LARGER than the first payment, or more than one extra release, means the replay now pays MORE than it did.\n"+
		"  Establish which before touching this pin — the escrow drain is DIRECTED (repairproof.RepairClaim is unsigned and the payee is\n"+
		"  claim.Holder, a field of the message), so the size of the per-replay draw is the severity.",
		firstPaid, secondPaid, secondPaid-firstPaid, firstPaid, firstReleases, secondReleases, firstReleases+1,
		rootPfx, pos, shardPfx, holder)
}

func TestRTRC2_ReplayedClaimPaysAgain_PINNED_DEFECT(t *testing.T) {
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
		t.Fatalf("RT-RC-2 PREMISE BROKEN: the FIRST claim did not pay (EscrowPaid=%d, BountiesReleased=%d) — a replay pin needs a paid first claim to replay",
			firstPaid, firstReleases)
	}

	// The same claim, byte-identical, delivered again by the same sender.
	s.deliverClaim(judge, s.nodes[2].ID(), claim)
	secondPaid := s.ledger.EscrowPaid(s.root)
	secondReleases := judge.Stats.BountiesReleased

	t.Logf("RT-RC-2 MEASURED: EscrowPaid after 1st claim = %d, after the identical replay = %d (delta %d); BountiesReleased %d -> %d",
		firstPaid, secondPaid, secondPaid-firstPaid, firstReleases, secondReleases)

	// PIN: the replay draws a SECOND payment of the same size, and a second release.
	// Pinning the delta against the first payment keeps this derived from the run's
	// own bounty price rather than from a transcribed amount.
	if msg := rtRC2Pin(firstPaid, secondPaid, firstReleases, secondReleases, pos,
		s.root[:4], parityID[:4], holder.ID().String()[:8]); msg != "" {
		t.Fatal(msg)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RT-RC-3 — a claim with NO LOSS is PAID.
//
// THE RULE THIS PIN RECORDS THE ABSENCE OF: a durability bounty pays for a
// REPAIR, so a shard that was never missing was never repaired, and copying it
// onto a second holder should pay nothing. It pays the full bounty.
//
// The arrangement is the no-loss one, and the pin PROVES the no-loss premise
// before it judges: the shard is still retrievable from its ORIGINAL providers at
// the moment the claim is delivered. Nothing was rebuilt; a live copy was moved.
//
// ⚠ THIS PIN AGREES WITH A SHIPPED TEST, AND THAT AGREEMENT IS THE FINDING.
// TestRedteamRepair_HonestClaimIsPaid (redteam_repair_claim_test.go) builds the
// identical arrangement with the same stageShardOn helper and asserts
// BountiesReleased == 1 as the INTENDED rule. This pin asserts the same outcome
// as a DEFECT. Exactly one of those readings is right, and which one is the
// owner's call. Do not reconcile them by editing either test.
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
	return fmt.Sprintf("RT-RC-3 PIN IS RED — a claim for a shard that was NEVER LOST no longer pays as pinned: EscrowPaid %d -> %d, BountiesReleased=%d (pinned at a positive delta and exactly 1).\n"+
		"  THE FIX CASE: no payment and no release means the judge now requires evidence of prior loss for the claimed (root, stripe, position).\n"+
		"  That is the repair this pin was waiting for — BUT IT CANNOT LAND ALONE. TestRedteamRepair_HonestClaimIsPaid asserts the OPPOSITE on\n"+
		"  this same arrangement and will have gone RED in the same run. Both tests must be re-decided together, and the owner decides which\n"+
		"  states the intended rule. The judge's two legs (correctness recompute in judgeRepairClaim, retrievability in\n"+
		"  challengeHolderRetrievability) both pass for a live shard merely COPIED to a new holder; nothing on the path asserted prior loss.\n"+
		"  THE OTHER CASE: more than one release, or a payment where the premise arm above did not hold, is a different defect. Re-derive first.",
		before, after, releases)
}

func TestRTRC3_ClaimWithNoLossIsPaid_PINNED_DEFECT(t *testing.T) {
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
		t.Fatalf("RT-RC-3 PREMISE BROKEN: no provider other than the staged holder answers for column %d — the shard may actually have been lost, so this is not a no-loss arrangement", pos)
	}

	before := s.ledger.EscrowPaid(s.root)
	claim := repairClaimFor(s.root, 0, pos, parityID, holder.ID())
	s.deliverClaim(judge, s.nodes[2].ID(), claim)
	after := s.ledger.EscrowPaid(s.root)

	t.Logf("RT-RC-3 MEASURED: the shard was NEVER missing (live on its original providers) and a copy was staged on %s; EscrowPaid %d -> %d, BountiesReleased=%d",
		holder.ID().String()[:8], before, after, judge.Stats.BountiesReleased)

	// PIN: the no-loss claim is paid, once.
	if msg := rtRC3Pin(before, after, judge.Stats.BountiesReleased); msg != "" {
		t.Fatal(msg)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// THE TEETH. Each RT-RC pin above is a pure predicate; this test feeds each one the
// value it must FIRE on and the value it must accept.
//
// A pin that cannot fire reports green forever and is indistinguishable from one that
// works. Ablating the product is the right evidence, but it lives in a review document
// and does not survive a context reset. Same mechanism as #817's
// TestRT_SFO_5_PinRedensWhenFileSizeIsBlinded.
//
// THIS TEST IS NOT A PIN. Its polarity is ordinary: GREEN when the pins can fire.
// Delete it in the same commit that retires the pins above.
//
// RT-RC-1's THREE ARMS ARE EXERCISED SEPARATELY, deliberately. The reviewer's
// ablations showed each arm is independently reddenable (a k-budget reddens the
// in-range arm alone; a ShardPos validation reddens the out-of-range arm alone), and a
// teeth test that only drove one arm would let the other two rot unobserved.
// ─────────────────────────────────────────────────────────────────────────────

func TestRTRC_PinsFireOnTheirRemediations(t *testing.T) {
	// The shipped geometry RT-RC-1 measures against: k=10, n=16.
	const k, nShards = 10, 16

	// --- RT-RC-1 (a), amplification. Pinned: 8 claims cost ~8x one claim. ---
	if msg := rtRC1Amplification(8, 1000, 8000, nShards-1); msg != "" {
		t.Fatalf("rtRC1Amplification fired on the unbounded state it is pinned to accept: %s", msg)
	}
	if rtRC1Amplification(8, 1000, 1000, nShards-1) == "" {
		t.Fatal("rtRC1Amplification stayed silent when 8 claims cost no more than ONE — a per-sender " +
			"per-window bound landing is exactly what this arm exists to detect (simplicity rule 7)")
	}
	if rtRC1Amplification(8, 1000, 3000, nShards-1) == "" {
		t.Fatal("rtRC1Amplification stayed silent at exactly the 3x boundary — the arm must fire at the " +
			"limit a bound would allow, not only strictly inside it")
	}

	// --- RT-RC-1 (b), in-range width. Pinned at n−1. ---
	if msg := rtRC1InRange(nShards-1, k, nShards); msg != "" {
		t.Fatalf("rtRC1InRange fired on the pinned n−1 width: %s", msg)
	}
	if rtRC1InRange(k, k, nShards) == "" {
		t.Fatal("rtRC1InRange stayed silent when the fetch reached exactly k — the survivor budget " +
			"landing is what this arm exists to detect, and it is the number #844 corrected")
	}

	// --- RT-RC-1 (c), out-of-range. Pinned at n, strictly more than in-range. ---
	if msg := rtRC1OutOfRange(nShards+5, nShards, nShards-1, nShards); msg != "" {
		t.Fatalf("rtRC1OutOfRange fired on the pinned n width: %s", msg)
	}
	if rtRC1OutOfRange(nShards+5, 0, nShards-1, nShards) == "" {
		t.Fatal("rtRC1OutOfRange stayed silent when an invalid position reached ZERO shards — a " +
			"ShardPos validation ahead of the fetch is what this arm exists to detect")
	}
	if rtRC1OutOfRange(nShards+5, nShards-1, nShards-1, nShards) == "" {
		t.Fatal("rtRC1OutOfRange stayed silent when the out-of-range claim cost no MORE than an honest " +
			"one — the 'worse than honest' half of the finding would land unnoticed")
	}

	// --- RT-RC-2, replay pays again. ---
	pfx := []byte{1, 2, 3, 4}
	if msg := rtRC2Pin(100, 200, 1, 2, 3, pfx, pfx, "holder00"); msg != "" {
		t.Fatalf("rtRC2Pin fired on the pinned second payment: %s", msg)
	}
	if rtRC2Pin(100, 100, 1, 1, 3, pfx, pfx, "holder00") == "" {
		t.Fatal("rtRC2Pin stayed silent when the replay paid ZERO and released nothing — the " +
			"(root, stripe, position) dedup landing is what this pin exists to detect")
	}
	if rtRC2Pin(100, 400, 1, 2, 3, pfx, pfx, "holder00") == "" {
		t.Fatal("rtRC2Pin stayed silent when the replay paid MORE than the first claim — the WORSE " +
			"case the message names would land unnoticed")
	}

	// --- RT-RC-3, a no-loss claim is paid. ---
	if msg := rtRC3Pin(0, 5000, 1); msg != "" {
		t.Fatalf("rtRC3Pin fired on the pinned no-loss payment: %s", msg)
	}
	if rtRC3Pin(0, 0, 0) == "" {
		t.Fatal("rtRC3Pin stayed silent when the no-loss claim paid nothing — a prior-loss requirement " +
			"landing is what this pin exists to detect, and it is the RED that must be read together " +
			"with TestRedteamRepair_HonestClaimIsPaid")
	}
	if rtRC3Pin(0, 5000, 2) == "" {
		t.Fatal("rtRC3Pin stayed silent on TWO releases — 'paid, once' is the pinned shape and a " +
			"double release is a different defect")
	}
}
