package node

// The judge-side half of D-BOUNTY-REPAIR-MECHANISM-GATED-2026-09-12 — the three
// items the mechanism certification discharged the research gate for, and nothing
// else. Each test here was driven RED against the tree that preceded its fix.
//
//	A  The claimed position is screened against the manifest BEFORE any survivor
//	   fetch. A listed position whose committed id disagrees with claim.ShardID is
//	   SLASHED; a position the manifest does not list is DENIED.
//	B  repairproof.VerifyByRecompute counts the implicit-zero padding of a short
//	   final stripe, so a stripe whose every stored shard the judge can see is
//	   judgeable. Its unit gate is core/repairproof; this file carries the wired arm.
//	D  A (root, stripe, pos) already PAID draws nothing on a later claim. Its main
//	   assertion is the CONVERTED RT-RC-2 gate in rt_repairclaim_gates_test.go; what
//	   lives here are the TWO control legs that fix WHERE the record is written — one
//	   for the deny arm and one for the release-that-paid-nothing arm. Two arms,
//	   because the deny one alone leaves the realistic mis-placement uncovered
//	   (measured in blind review, 2026-09-12).
//
// WHAT IS NOT HERE, deliberately: the loss witness. Nothing below asserts that the
// claimed position was ever missing, and nothing below may be read as doing so. The
// witness is GATED behind R-PROBE-FALSE-NEGATIVE-RATE, RT-RC-3 stays open, and the
// honest summary of what these three buy is that the bounty becomes correctly
// METERED and stays MIS-ATTRIBUTABLE.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// shardFetches counts how many of the OBJECT'S OWN shards the judge wrote through
// its store. handleRepairClaim fetches the object's manifest chunks before it can
// judge anything, and that fetch is not a survivor fetch — measured: a claim refused
// at the position screen still writes exactly 1 chunk, the manifest. Counting only
// ids the manifest commits as data or parity isolates the survivor fetch from it, so
// "zero" below means zero SHARDS and not zero bytes of any kind.
func (s *repairAdv) shardFetches(cs *countStore) int {
	shard := map[ports.ChunkID]bool{}
	for _, id := range s.m.ChunkIDs() {
		shard[id] = true
	}
	for _, id := range s.m.ParityIDs() {
		shard[id] = true
	}
	n := 0
	for id := range cs.distinct {
		if shard[id] {
			n++
		}
	}
	return n
}

// ─────────────────────────────────────────────────────────────────────────────
// A — the position screen.
// ─────────────────────────────────────────────────────────────────────────────

// TestRepairJudge_WrongIdAtAListedPositionIsSlashedBeforeAnyFetch: the claim names a
// REAL, manifest-listed stripe position and a REAL shard id that the manifest
// commits at a DIFFERENT position. That is a self-attributing lie and it was already
// slashable — through the recompute leg, at the cost of n−1 survivor fetches.
//
// ⚠ THE SLASH IS THE ASSERTION THAT MATTERS. A screen that denied instead would land
// looking like a validation and act as a silent RETIREMENT of an existing
// punishment, leaving silt strictly weaker against the exact adversary the slash was
// built for. So this test asserts the SAME verdict as before the screen, and the
// thing that changed — zero survivor fetches — alongside it.
//
// DRIVEN RED FIRST: before the screen the judge wrote 15 distinct shards through its
// store for this claim.
func TestRepairJudge_WrongIdAtAListedPositionIsSlashedBeforeAnyFetch(t *testing.T) {
	s := newRepairAdv(t, 1901)
	s.fundEscrow(5_000_000)
	judge := s.careJudge()
	cs := newCountStore(judge.store)
	judge.store = cs

	attacker := s.nodes[2]
	baseline := s.bond(attacker)

	pos, committedID, _ := s.parityTarget()
	otherID := s.m.ParityIDs()[1] // a real id, committed at position K+1, not at pos
	if otherID == committedID {
		t.Fatal("PREMISE BROKEN: the fixture's first two parity shards share an id, so this claim does not disagree with the manifest")
	}

	s.deliverClaim(judge, attacker.ID(), repairClaimFor(s.root, 0, pos, otherID, s.nodes[7].ID()))

	if judge.Stats.FalseRepairSlashes != 1 {
		t.Fatalf("a well-formed position claiming an id the manifest does not commit there was NOT slashed: FalseRepairSlashes=%d. "+
			"The screen must SLASH this case; denying it silently retires the punishment the recompute leg already delivered", judge.Stats.FalseRepairSlashes)
	}
	if got := s.ledger.Reputation(attacker.ID()); got >= baseline {
		t.Fatalf("claimant standing not docked: %d >= %d", got, baseline)
	}
	if paid := s.ledger.EscrowPaid(s.root); paid != 0 {
		t.Fatalf("a false claim drew a bounty: %d", paid)
	}
	if got := s.shardFetches(cs); got != 0 {
		t.Fatalf("the judge fetched %d survivor shards for a claim the MANIFEST ALONE refutes, want 0 (%d store writes in total, of which the manifest is one). "+
			"storedShards already carries the committed id for the claimed position; comparing it costs no network at all", got, cs.puts)
	}
	t.Logf("A: wrong id at a listed position — slashed, standing %d -> %d, survivor shard fetches %d over %d total store writes",
		baseline, s.ledger.Reputation(attacker.ID()), s.shardFetches(cs), cs.puts)
}

// TestRepairJudge_UnlistedPositionIsDeniedWithoutFetchOrRetry: a claim.ShardPos
// outside 0..n−1 is STRUCTURALLY impossible. It matches no manifest ref, so before
// the screen it excluded NOTHING from the survivor set and the judge fetched all n —
// one MORE than an honest claim — and then ran that fetch FOUR times, because the
// deferral predicate is `cerr != nil` and cannot tell "too few survivors"
// (transient, heals) from "target out of range" (structural, never heals).
//
// It is a DENY and not a slash: malformed input is treated as malformed input
// everywhere else on this path, and deny is the conservative choice. Either choice
// kills the amplification.
//
// The honest arm in the same test is the anti-vacuity witness: it proves this
// fixture DOES reach fetchSurvivors, so the zero above is a refusal and not a
// harness that never got there.
//
// DRIVEN RED FIRST: before the screen this claim reached 16 distinct shards over 64
// store writes — the four-round ladder, measured.
func TestRepairJudge_UnlistedPositionIsDeniedWithoutFetchOrRetry(t *testing.T) {
	measure := func(seed int64, pos int, id ports.ChunkID) (shards, puts, slashes, released int) {
		s := newRepairAdv(t, seed)
		s.fundEscrow(5_000_000)
		judge := s.careJudge()
		cs := newCountStore(judge.store)
		judge.store = cs
		attacker := s.nodes[2]
		s.bond(attacker)
		// A holder that does not hold the shard: the retrievability leg denies, so
		// the honest arm below measures the FETCH and never pays.
		s.deliverClaim(judge, attacker.ID(), repairClaimFor(s.root, 0, pos, id, s.nodes[7].ID()))
		return s.shardFetches(cs), cs.puts, judge.Stats.FalseRepairSlashes, judge.Stats.BountiesReleased
	}

	geom := newRepairAdv(t, 1902)
	inRange, committedID, _ := geom.parityTarget()
	nShards := geom.m.N

	// Anti-vacuity: an honest, in-range, correctly-identified claim still fetches.
	honestShards, honestPuts, honestSlash, _ := measure(1902, inRange, committedID)
	if honestShards == 0 {
		t.Fatalf("VACUOUS: an honest in-range claim reached %d survivor shards — the fixture never reaches fetchSurvivors, so a zero below proves nothing", honestShards)
	}
	if honestSlash != 0 {
		t.Fatalf("PREMISE BROKEN: the honest arm was slashed (%d) — it names the manifest's own committed id and must clear the screen", honestSlash)
	}

	// Padding positions exist only on a SHORT stripe, so the padding arm names the
	// final stripe. Out-of-range and negative positions are refused on any stripe.
	finalStripe, _, _, _ := geom.finalStripeParityTarget()
	padPos := geom.finalStripeRealData() // the first implicit-zero slot of the final stripe
	if padPos >= geom.m.K {
		t.Fatalf("PREMISE BROKEN: the final stripe has realData=%d of k=%d, so it carries NO padding position and the padding arm is vacuous", padPos, geom.m.K)
	}

	for _, c := range []struct {
		what   string
		stripe int
		pos    int
	}{
		{"out of range", 0, nShards + 5},
		{"negative", 0, -1},
		{"implicit-zero padding", finalStripe, padPos},
	} {
		s := newRepairAdv(t, 1903)
		s.fundEscrow(5_000_000)
		judge := s.careJudge()
		cs := newCountStore(judge.store)
		judge.store = cs
		attacker := s.nodes[2]
		s.bond(attacker)
		s.deliverClaim(judge, attacker.ID(), repairClaimFor(s.root, c.stripe, c.pos, committedID, s.nodes[7].ID()))

		if got := s.shardFetches(cs); got != 0 {
			t.Fatalf("%s position %d of stripe %d reached %d survivor shards over %d store writes, want 0. An honest claim reaches %d over %d, "+
				"so this position used to cost the judge MORE than an honest one, four times over",
				c.what, c.pos, c.stripe, got, cs.puts, honestShards, honestPuts)
		}
		if judge.Stats.FalseRepairSlashes != 0 {
			t.Fatalf("%s position %d was SLASHED (%d) — a structurally impossible position is malformed input and denies; only a well-formed position that lies about its id is punished",
				c.what, c.pos, judge.Stats.FalseRepairSlashes)
		}
		if judge.Stats.BountiesReleased != 0 {
			t.Fatalf("%s position %d released a bounty (%d)", c.what, c.pos, judge.Stats.BountiesReleased)
		}
	}
	t.Logf("A: out-of-range, negative and padding positions each cost 0 survivor shards; an honest in-range claim costs %d over %d store writes (n=%d)",
		honestShards, honestPuts, nShards)
}

// ─────────────────────────────────────────────────────────────────────────────
// B — the short final stripe, wired.
// ─────────────────────────────────────────────────────────────────────────────

// TestRepairJudge_ShortFinalStripeIsJudgeableAndPays is the wired arm of the
// `present`-count fix.
//
// ⚠ THE FIXTURE ALREADY HAD A SHORT STRIPE AND NOBODY HAD CLAIMED IT. newRepairAdv's
// object is not "one full k=10 stripe" — splitFile reserves a header in every frame,
// so 10·512 KiB of input yields ELEVEN chunks over TWO stripes, and the second one
// carries realData = 1. Every pre-existing repair-claim test claims STRIPE 0, the row
// with maximum survivor slack, which is why the whole test surface was blind to this
// defect by construction, for a reason (the bounty base) unrelated to the defect.
//
// At k=10/n=16 with realData=1 that stripe stores 1 + 6 = 7 shards. The judge excludes
// the claimed position, so it can supply at most 6 — four short of k. Before the fix
// this claim deferred four times and then denied, FOREVER, and the same held for every
// object of four chunks or fewer: 1 MiB at the default chunk size.
//
// DRIVEN RED FIRST: before the fix this released 0 bounties.
func TestRepairJudge_ShortFinalStripeIsJudgeableAndPays(t *testing.T) {
	s := newRepairAdv(t, 1904)
	s.fundEscrow(5_000_000)
	judge := s.careJudge()

	// Derived anti-vacuity, from the fixture's own manifest: assert this really is a
	// stripe the OLD predicate could never judge.
	realData, k, nShards := s.finalStripeRealData(), s.m.K, s.m.N
	supplied := realData + (nShards - k) - 1
	if realData >= k {
		t.Fatalf("FIXTURE NOT SHORT: the final stripe has realData=%d of k=%d — this arm would re-measure the full-stripe row", realData, k)
	}
	if supplied >= k {
		t.Fatalf("VACUOUS: a judge can supply %d survivors against k=%d, so this geometry was judgeable before the fix and proves nothing", supplied, k)
	}

	stripe, pos, parityID, leafIdx := s.finalStripeParityTarget()
	holder := s.nodes[9]
	holderBalance := s.ledger.Balance(holder.ID())
	s.stageShardOn(judge, holder, parityID, pos, leafIdx)

	s.deliverClaim(judge, s.nodes[2].ID(), repairClaimFor(s.root, stripe, pos, parityID, holder.ID()))

	if judge.Stats.BountiesReleased != 1 {
		t.Fatalf("the SHORT final stripe (stripe %d, realData=%d of k=%d, %d survivors obtainable) was not judged: BountiesReleased=%d. "+
			"VerifyByRecompute must count the %d implicit-zero padding slots erasure.ReconstructStripe fills, or this position is unjudgeable forever",
			stripe, realData, k, supplied, judge.Stats.BountiesReleased, k-realData)
	}
	if judge.Stats.FalseRepairSlashes != 0 {
		t.Fatalf("an honest short-stripe repair was SLASHED (%d) — that is the REFUTED placement's failure mode: deleting the `present` pre-check routes a "+
			"short-survivor TRANSIENT into Decide(false, …), which bond-slashes an honest paramedic", judge.Stats.FalseRepairSlashes)
	}
	if got := s.ledger.Balance(holder.ID()); got <= holderBalance {
		t.Fatalf("holder balance did not rise: %d <= %d", got, holderBalance)
	}
	t.Logf("B: stripe %d has realData=%d of k=%d (n=%d), %d survivors obtainable — %d short of k — and it now judges and pays",
		stripe, realData, k, nShards, supplied, k-supplied)
}

// ─────────────────────────────────────────────────────────────────────────────
// D — dedup on PAID.
// ─────────────────────────────────────────────────────────────────────────────

// TestRepairJudge_DeniedPositionStaysPayable is the CONTROL LEG for where the dedup
// record is written, and it is the whole reason the record goes on PAID and never on
// JUDGED.
//
// ⚠ THIS TEST IS NOT RED ON THE DEFECT. It is green before the dedup and green after
// it, and it goes RED when the record is moved to the top of settleRepairVerdict.
// That is what it is for: the defence is breakable, so the control is definable.
//
// ⚠ IT COVERS ONLY HALF THE PROPERTY, and measuring that is what produced the second
// arm below. This test arranges a RETRIEVABILITY DENY, which settleRepairVerdict
// refuses at the PRE-EXISTING `if !d.Release` return — upstream of the record site.
// So it pins the record BELOW `!d.Release`, which was never in doubt, and NOT at
// `paid > 0`, which is the placement the field comment argues for. Measured
// 2026-09-12: moving the record to just after the dedup check leaves this test, and
// the whole package, green. TestRepairJudge_ReleaseThatPaidNothingStaysPayable is
// the arm that reddens there.
//
// The arrangement is the poisoning attack. An attacker claims position P naming a
// holder that does not hold it. The judge judges the claim, denies on retrievability,
// and pays nothing. The honest paramedic then claims P naming the real holder. A
// judged-set would refuse that second claim — and emitRepairClaim binds an EMPTY
// reply callback, so a refused claim is invisible to its sender and lost FOREVER
// (T-RETRY-IS-THE-PRECONDITION). One frame from an attacker would permanently
// destroy a legitimate bounty.
func TestRepairJudge_DeniedPositionStaysPayable(t *testing.T) {
	s := newRepairAdv(t, 1906)
	s.fundEscrow(5_000_000)
	judge := s.careJudge()

	pos, parityID, leafIdx := s.parityTarget()
	attacker := s.nodes[2]
	s.bond(attacker)

	// The attacker names the REAL position and the REAL committed id — so it clears
	// the position screen and the recompute leg, and is not slashed — but names a
	// holder that does not hold the shard, so retrievability denies.
	s.deliverClaim(judge, attacker.ID(), repairClaimFor(s.root, 0, pos, parityID, s.nodes[7].ID()))
	if judge.Stats.BountiesReleased != 0 || s.ledger.EscrowPaid(s.root) != 0 {
		t.Fatalf("PREMISE BROKEN: the attacker's claim PAID (released=%d, EscrowPaid=%d) — this control needs a JUDGED-but-UNPAID first claim",
			judge.Stats.BountiesReleased, s.ledger.EscrowPaid(s.root))
	}
	if judge.Stats.FalseRepairSlashes != 0 {
		t.Fatalf("PREMISE BROKEN: the attacker's claim was slashed (%d) — it must reach the retrievability leg, not die at the screen", judge.Stats.FalseRepairSlashes)
	}

	// The honest repair of the SAME position must still be payable.
	holder := s.nodes[9]
	s.stageShardOn(judge, holder, parityID, pos, leafIdx)
	s.deliverClaim(judge, s.nodes[3].ID(), repairClaimFor(s.root, 0, pos, parityID, holder.ID()))

	if judge.Stats.BountiesReleased != 1 {
		t.Fatalf("an honest claim for a position an ATTACKER had already got JUDGED was refused: BountiesReleased=%d, want 1. "+
			"The (root, stripe, pos) record must be written on PAID and never on judged — a judged set lets one attacker frame destroy a legitimate bounty permanently",
			judge.Stats.BountiesReleased)
	}
	if paid := s.ledger.EscrowPaid(s.root); paid <= 0 {
		t.Fatalf("EscrowPaid = %d, want > 0", paid)
	}
	if judge.Stats.BountyDuplicatePosition != 0 {
		t.Fatalf("the honest claim was counted as a duplicate: BountyDuplicatePosition=%d, want 0", judge.Stats.BountyDuplicatePosition)
	}
	t.Logf("D control: a position judged-and-DENIED stayed payable; the honest claim that followed paid %d", s.ledger.EscrowPaid(s.root))
}

// TestRepairJudge_ReleaseThatPaidNothingStaysPayable is the SECOND control leg for
// the dedup record, and it is the one that pins the record at `paid > 0` rather than
// merely downstream of `!d.Release`.
//
// ⚠ THE CONTROL ABOVE DOES NOT COVER THIS. TestRepairJudge_DeniedPositionStaysPayable
// arranges a RETRIEVABILITY DENY, which settleRepairVerdict refuses at the
// PRE-EXISTING `if !d.Release` return — upstream of the record site, and never in
// doubt. Measured 2026-09-12 (blind review of this PR): moving the record from the
// `paid > 0` arm up to just after the dedup check leaves the ENTIRE core/node package
// green, that control included. The realistic mis-placement had no gate. This arm is
// the one that reddens.
//
// The arrangement is the case Node.bountyPaid's own doc argues for and the one D-S7
// says happens to every object nobody re-endows: an EMPTY escrow. The claim is honest
// end to end — real position, manifest-committed id, a holder that really holds the
// bytes — so both legs verify and the verdict RELEASES. It pays nothing, because the
// reserve is empty. A release that paid nothing is not a payment, so the position must
// still be claimable once the object is re-endowed; a judged-set would convert the
// funded horizon running out into a PERMANENT loss of a legitimate bounty.
//
// The FUNDED arm is the anti-vacuity witness. The identical arrangement against a
// funded escrow pays on the FIRST claim, so the unfunded arm provably reaches
// PayBounty and is refused THERE, rather than dying earlier at a deny — which is
// exactly the confusion that made the control above half-vacuous.
func TestRepairJudge_ReleaseThatPaidNothingStaysPayable(t *testing.T) {
	// Anti-vacuity witness: same seed, same position, same holder, escrow FUNDED.
	{
		w := newRepairAdv(t, 1907)
		w.fundEscrow(5_000_000)
		judge := w.careJudge()
		pos, parityID, leafIdx := w.parityTarget()
		holder := w.nodes[9]
		w.stageShardOn(judge, holder, parityID, pos, leafIdx)
		w.deliverClaim(judge, w.nodes[3].ID(), repairClaimFor(w.root, 0, pos, parityID, holder.ID()))
		if judge.Stats.BountiesReleased != 1 {
			t.Fatalf("VACUOUS: the witness arrangement did not pay against a FUNDED escrow (BountiesReleased=%d, EscrowPaid=%d). "+
				"Without it, a non-payment in the unfunded arm below would not prove the EMPTY ESCROW was the reason — it could be dying at a deny",
				judge.Stats.BountiesReleased, w.ledger.EscrowPaid(w.root))
		}
	}

	s := newRepairAdv(t, 1907)
	judge := s.careJudge() // NOTE: no fundEscrow — the object's durability reserve is empty.
	pos, parityID, leafIdx := s.parityTarget()
	holder := s.nodes[9]
	holderBalance := s.ledger.Balance(holder.ID())
	s.stageShardOn(judge, holder, parityID, pos, leafIdx)

	if bal := s.ledger.EscrowBalance(s.root); bal != 0 {
		t.Fatalf("PREMISE BROKEN: the escrow holds %d credits before the first claim — this arm needs an EMPTY one, and the serve auto-skim has evidently funded it", bal)
	}
	s.deliverClaim(judge, s.nodes[3].ID(), repairClaimFor(s.root, 0, pos, parityID, holder.ID()))

	if judge.Stats.BountiesReleased != 0 || s.ledger.EscrowPaid(s.root) != 0 {
		t.Fatalf("PREMISE BROKEN: the first claim PAID (BountiesReleased=%d, EscrowPaid=%d) — this control needs a RELEASED-but-UNPAID first claim",
			judge.Stats.BountiesReleased, s.ledger.EscrowPaid(s.root))
	}
	if judge.Stats.FalseRepairSlashes != 0 {
		t.Fatalf("PREMISE BROKEN: the honest first claim was slashed (%d) — it must clear both legs and reach the release arm", judge.Stats.FalseRepairSlashes)
	}

	// Re-endow the object and re-claim the SAME (root, stripe, position).
	s.fundEscrow(5_000_000)
	s.deliverClaim(judge, s.nodes[3].ID(), repairClaimFor(s.root, 0, pos, parityID, holder.ID()))

	if judge.Stats.BountiesReleased != 1 {
		t.Fatalf("a position whose earlier RELEASE paid nothing out of an empty escrow was refused after the object was re-endowed: BountiesReleased=%d, want 1. "+
			"The (root, stripe, pos) record must be written where the payment happens (paid > 0), never on the release verdict — a funded horizon running out would otherwise "+
			"destroy the bounty for that position permanently", judge.Stats.BountiesReleased)
	}
	if paid := s.ledger.EscrowPaid(s.root); paid <= 0 {
		t.Fatalf("EscrowPaid = %d, want > 0", paid)
	}
	if got := s.ledger.Balance(holder.ID()); got <= holderBalance {
		t.Fatalf("holder balance did not rise: %d <= %d", got, holderBalance)
	}
	if judge.Stats.BountyDuplicatePosition != 0 {
		t.Fatalf("the re-endowed claim was counted as a DUPLICATE: BountyDuplicatePosition=%d, want 0 — the record was written on a release that paid nothing",
			judge.Stats.BountyDuplicatePosition)
	}
	t.Logf("D control 2: a release that paid nothing out of an EMPTY escrow left the position payable; after re-endowment it paid %d", s.ledger.EscrowPaid(s.root))
}
