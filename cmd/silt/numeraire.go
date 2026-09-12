package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/pipeline"
)

// grantOverSummedPriceBytes is how many bytes one starter grant buys when a NAT'd
// fetcher pays BOTH lane prices at once: g / (1/D_delivery + 1/D_relay) =
// g·D_delivery·D_relay / (D_delivery + D_relay). The 64 GiB grant/r pin is read on this
// SUM (G-R212-6); the priced-lane start-up refusal compares against it (G-λ-3).
func grantOverSummedPriceBytes(grant, deliveryBytesPerCredit, relayBytesPerCredit int64) int64 {
	if deliveryBytesPerCredit <= 0 || relayBytesPerCredit <= 0 {
		return 0
	}
	return grant * deliveryBytesPerCredit * relayBytesPerCredit / (deliveryBytesPerCredit + relayBytesPerCredit)
}

// objectSizeUnknown is the objectBytes a caller passes when it cannot cheaply learn the
// length it is about to publish (a pipe, a stream). The warning then prices the GEOMETRY
// alone, which is the only thing it can honestly say about such a publish.
const objectSizeUnknown = int64(-1)

// fileSizeOrUnknown is the length of an already-open publish source, or objectSizeUnknown
// when it has none a Stat can report (a pipe, a device). It never fails the publish: a
// missing size costs the object arm of the warning, nothing else.
func fileSizeOrUnknown(f *os.File) int64 {
	if f == nil {
		return objectSizeUnknown
	}
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return objectSizeUnknown
	}
	return fi.Size()
}

// warnBountyPrice names, at publish time, a publish whose SHARD short-pays the repair
// bounty. The base is an integer floor of the witnessed fetch price of the one shard the
// payee moves (F1, 2026-09-12), so a shard below one credit of fetch rounds to nothing
// (G-λ-8, G-R212-7) and a shard just above it rounds away up to half the repairer's wage
// (R-BOUNTY-TRUNCATION, G-BT-1).
//
// TWO things can put a publish there, and both are named because a publisher can act on
// neither once the object is stored. (1) A chunk size the operator chose. (2) The OBJECT:
// since R-SHORT-FINAL-STRIPE a single-frame object is stored at its true length, so ITS
// shard is its own bytes and no chunk size changes that — at the shipped default every
// object of 262,119 B or less pays a base of ZERO (26,190 B before F1; the class widened
// 10.008×, R-BOUNTY-ZERO-BELOW-262KB). Case (2) is created by this same change, so leaving
// it silent would ship a warning that misses the class it invented (blind PE B-3,
// 2026-09-09).
//
// The daemon has no publish geometry at start-up to refuse on, and the judge that names a
// zero at settlement is a caretaker the publisher neither runs nor sees, so this is the
// one place the publisher is told.
func warnBountyPrice(chunkBytes int, objectBytes int64, w io.Writer) {
	if msg := bountyPriceWarning(int64(chunkBytes), objectBytes); msg != "" {
		fmt.Fprintln(w, msg)
	}
}

// minBountyChunkBytes is the publish-time ZERO threshold at the shipped erasure geometry.
func minBountyChunkBytes() int64 {
	return credit.MinBountyChunkBytesFor(erasure.DefaultParams.K, crypto.Overhead)
}

// shippedBountyBase is the repair-bounty base the SHIPPED publish default pays on a
// full-frame object: the warning's threshold, DERIVED from the two shipped constants and
// never typed. It is 1 today (a 262,160-byte shard, exact 1.00006 — it was 10 before F1
// re-based the price on the one shard the payee moves), which is what makes the rule below
// have a closed complement — warn iff this publish pays a smaller base than the shipped
// default's — so a default publish of a full-frame object is silent by construction rather
// than by a hand-kept number.
//
// The coupling to watch: if DefaultChunkSize ever dropped below minBountyChunkBytes this
// would be 0 and the whole warning, ZERO arm included, would go permanently silent. That
// was a comment until 2026-09-12; F1 compressed the margin from 235,945 B to 16 B, so it is
// now an assertion — checkBountyDisclosureHeadroom, a refuse-to-start (canon rule 8, first
// arm: locally checkable ⇒ refuse to start). The gate also asserts this reads 1 and says to
// re-read G-BT-1 on any move of the default (blind PE N-7).
func shippedBountyBase() int64 {
	return credit.RepairBountyBase(erasure.DefaultParams.K, int64(pipeline.DefaultChunkSize)+crypto.Overhead)
}

// checkBountyDisclosureHeadroom refuses a build whose publish default pays a ZERO repair
// bounty. It is canon rule 8's first arm: the relation is pure local arithmetic over two
// compile-time constants, so it is checkable at start-up and never needs distributed
// agreement (`c` is not a consensus quantity — PayBounty is classified `neutral` and the
// γ→1/N firewall is build-enforced in core/credit/invariant_a_test.go).
//
// WHAT IT PROTECTS, precisely. bountyPriceWarning's rule is "warn iff this publish's base
// is below the shipped default's". If the shipped default itself paid 0, every publish
// would be at or above the threshold and the warning — ZERO arm included — would go
// permanently silent, which is the one way a publisher can never learn that its object's
// repairs pay nothing. The margin used to be 235,945 B and nobody could plausibly cross it;
// since F1 it is 16 B (crypto.Overhead), and the two constants either side are Evolving-tier.
//
// It is deliberately NOT gated on -economy: the disclosure is what the publisher needs
// whether or not THIS node pays bounties, and the publish path runs in `silt add` with no
// economy flag at all. Raising pipeline.DefaultChunkSize to widen the margin is REFUSED —
// it moves blocks[0].Hash() (see that constant's own doc, and core/genesis
// TestGenesisBlockHashIsPinned). The admissible move is the PRICE, which is a separate
// certification because U/p carries GrantOverRPinBytes and the G-λ-3 refusal.
//
// Driven both ways by TestF1RefusesAPublishDefaultThatPaysNoBounty.
func checkBountyDisclosureHeadroom(defaultChunk, minChunk int64) error {
	if defaultChunk >= minChunk {
		return nil
	}
	return fmt.Errorf("the publish default pays a ZERO repair bounty: pipeline.DefaultChunkSize is %d B but a non-zero base needs at least %d B (one shard of witnessed fetch, %d B, less the %d-byte tag — F1, D-BOUNTY-PRICE-F1-2026-09-12). At a zero shipped base the publish warning's threshold is 0, so every publish reads as at-or-above it and the ZERO warning never fires again — a publisher would have no way to learn its object's repairs pay nothing. Re-derive the delivery price (credit.DeliveryBytesPerCredit); do NOT raise the chunk default, which moves the height-0 block hash",
		defaultChunk, minChunk, int64(credit.DeliveryBytesPerCredit), int64(crypto.Overhead))
}

// publishShardBytes is the ciphertext shard a repair of this publish will actually pull:
// pipeline.DataFrameSize decides the frame — the SAME rule pipeline.splitFile implements,
// read from the same two inputs rather than restated — plus the GCM tag. An unknown
// object length prices the chunk geometry alone.
func publishShardBytes(chunkBytes, objectBytes int64) int64 {
	frame := int(chunkBytes)
	if objectBytes >= 0 {
		frame = pipeline.DataFrameSize(int(objectBytes), int(chunkBytes))
	}
	return int64(frame) + crypto.Overhead
}

// bountyPriceWarning is the pure form of warnBountyPrice: the warning text, or "" when
// nothing should be said. Every number is the judge's own arithmetic on the shard this
// publish will really store (credit.RepairBountyBase / credit.RepairBountyTruncation),
// never a duplicated threshold.
//
// THE RULE, with its complement: warn iff this publish's real repair-bounty base is below
// the base the shipped default pays on a full frame. It is silent in exactly one case —
// a base at or above that.
//
// WHAT AN UNSET -chunk-size DOES, precisely, because the first version of this comment
// overstated it (blind PE R-1, 2026-09-09). An unset flag IS DefaultChunkSize, so the
// GEOMETRY cause can never fire without one; the OBJECT cause can, and is meant to.
// Measured at the shipped default with no flag set: 1,024 B fires (ZERO), 100,000 B fires
// (ZERO), 262,119 B fires (ZERO), and 262,120 B is the first silent size. So a default
// publish is silent for a FULL-FRAME object and speaks for a short-framed one — every
// object of 262,119 B or less warns.
//
// THE BOUNDARY DID NOT MOVE WITH F1 (2026-09-12); THE ARM DID. 262,119/262,120 is the same
// pair before and after, because both the base and the threshold fell by the same factor.
// What changed is that the sizes below it used to warn TRUNCATES and now warn ZERO — they
// pay nothing rather than paying short. That IS the accepted cost of the re-pricing
// (R-BOUNTY-ZERO-BELOW-262KB), and the shortFrame fix text below already said the right
// thing for it.
//
// AND THE TRUNCATES ARM IS NOW UNREACHABLE THROUGH THIS FUNCTION, disclosed rather than
// deleted. The rule fires iff base < shippedBountyBase(), which is 1, so a firing publish
// has base == 0 and always takes the ZERO arm.
//
// THE COST IS MEASURED, and it is larger than the anecdote first filed for it. The maximum
// SILENT repair-wage short-pay rises 5.5×, from 9.09 % pre-F1 (base ≥ 10 ⇒ loss ≤ 1/11) to
// 50.0 % (base ≥ 1 ⇒ loss ≤ 1/2), and it is reachable at ordinary operator choices, not
// only at the 524,264 corner: -chunk-size 393216 (384 KiB) goes 0.00 % → 33.3 %, 327,680
// goes 4.00 % → 20.0 %, 524,264 goes 5.00 % → 50.0 %. Pre-F1 the warning covered exactly
// the large-loss region; post-F1 that whole region is silent.
//
// THE RULE IS NOT WIDENED HERE, AND THE REASON IS THE CLOSED COMPLEMENT. The earlier
// record gave a different reason — that a loss-fraction clause "would speak on the shipped
// default itself, the finding blind PE M6 closed" — and THE SHIPPED CODE REFUTES IT:
// credit.RepairBountyTruncation returns 0 tenths of a percent at both 262,144 B (the
// default) and 262,128 B (the minimum chunk), against 200 / 333 / 500 at the rows above,
// so an OR of `lossTenths >= 10` (1 %) is silent on both. What such a clause really costs
// is this rule's one structural property: "warn iff base < shippedBountyBase()" has a
// CLOSED COMPLEMENT — silent in exactly one case, and that case is stated — and an
// OR-clause breaks it. Taking the widening is therefore a DECISION, not a defect fix, and
// it is filed with its measured cost as R-TRUNCATION-DISCLOSURE-NARROWS. The arm's arithmetic stays and is
// driven directly in core/credit TestRepairBountyTruncationIsExactIntegerArithmetic; the
// threshold is DERIVED, so a re-tune of U/p or the publish default revives the arm.
//
// That is a deliberate TRADE against the earlier "don't warn on every default publish"
// finding (blind PE M6), not a way of satisfying it: after R-SHORT-FINAL-STRIPE the object
// is what pays, and a publisher who is told nothing has no other way to learn that this
// object's repairs pay nothing. TestGLambda8PublishWarningFiresOnlyWhenThePublishShort
// PaysTheRepairer drives both sides and both causes, including the first silent size.
func bountyPriceWarning(chunkBytes, objectBytes int64) string {
	k := erasure.DefaultParams.K
	shardBytes := publishShardBytes(chunkBytes, objectBytes)
	base := credit.RepairBountyBase(k, shardBytes)
	if base >= shippedBountyBase() {
		return ""
	}
	// WHICH cause, and therefore which fix. A short frame means the shard IS the object.
	shortFrame := objectBytes >= 0 && shardBytes < chunkBytes+crypto.Overhead
	cause := fmt.Sprintf("-chunk-size %d gives a %d-byte shard", chunkBytes, shardBytes)
	fix := fmt.Sprintf("use -chunk-size >= %d for a non-zero bounty; the shipped default %d pays %d", minBountyChunkBytes(), pipeline.DefaultChunkSize, shippedBountyBase())
	if shortFrame {
		cause = fmt.Sprintf("this object is %d B, which fits in ONE frame, so it is stored at its true length and its shard is %d B (R-SHORT-FINAL-STRIPE)", objectBytes, shardBytes)
		fix = "NO chunk size changes this — the shard IS the object; under a repair economy this object's durability is prepay-only (fund its escrow), because its serves are also too small to skim a credit"
	}
	if base == 0 {
		return fmt.Sprintf("warning: this publish pays a ZERO repair bounty: %s, which is below one credit of fetch (%d B), so under a repair economy (-economy) a repair of it pays NOTHING (G-λ-8); %s",
			cause, int64(credit.DeliveryBytesPerCredit), fix)
	}
	exactE5, lossTenths := credit.RepairBountyTruncation(k, shardBytes)
	return fmt.Sprintf("warning: this publish TRUNCATES the repair bounty: %s, worth %s credits but paying %d — a repair of this object short-pays the repairer by %s%% of the price (R-BOUNTY-TRUNCATION, G-BT-1); %s",
		cause, creditsE5(exactE5), base, tenthsPct(lossTenths), fix)
}

// creditsE5 renders a price that credit.RepairBountyTruncation floored at 1e-5 credits:
// 199_996 reads "1.99996". Integer formatting, so the printed figure is the computed one.
func creditsE5(e5 int64) string {
	return fmt.Sprintf("%d.%05d", e5/100_000, e5%100_000)
}

// tenthsPct renders tenths of one percent: 500 reads "50.0".
func tenthsPct(t int64) string {
	return fmt.Sprintf("%d.%d", t/10, t%10)
}

// ---- R2.9 delivery session: the derived ceiling, the quantized pin, the S5 line.

// ---- the delivery idle window: the bound, the floor, the shipped default.
//
// The window is a LIVENESS choice: how long a paid delivery session survives a gap in its
// fetcher's settlements. An honest fetcher goes quiet while the chain it needs is stalled,
// so the window must dominate the worst stall the published liveness model admits. All
// three constants below are DERIVED from that one sentence; none is chosen.

// deliveryIdleBound is a CONSERVATIVE ENVELOPE, not a mechanism the reaper is racing. Its
// status changed on 2026-09-09: the window was originally sized on the sentence "a chain
// stall reaps every live session", and that sentence is refuted (`R-SESSION-WALLCLOCK-STEP`
// in docs/design/m0.md — nothing in the settle or fetch path reads the chain, driven). With
// the causal path withdrawn, this number is adopted because ratified call 4 of
// `D-TRUE-UP-CALLS-2026-09-07` instructs a window ABOVE the bound, and because it is a safe
// envelope on how long an honest fetcher may be gapped for reasons of its own. It is the
// worst stall the model admits: a LOST entry forward is bounded
// by the re-keyed takeover at ≤ (N+2)·ChainSyncInterval + G = 14·30 + 10 s at N = 12
// (docs/decisions.md D-H43-WORKLESS-DESIGNEE (21), ratified 2026-09-07). It DOMINATES the
// 190 s modal tier of D-CONSENSUS-ARMING (19), which is why a defensive window is sized
// against it and not against the tier: a window that only clears the modal case is broken
// by the worst case the same document publishes.
//
// FIELD STATUS, stated precisely (corrected 2026-09-09 by the pre-freeze derivation-route
// audit; the prior sentence here claimed BOTH numbers were field-confirmed and that is
// FALSE). The 190 s modal tier IS field-confirmed: report-97e3101-deep.md row
// 6-fault-tolerance drives exactly it. The 430 s figure is NOT. Row 10a-stall-drill also
// computes 430, but from an UNRELATED formula — (3+n_syb)*30 + 220 at n_syb = 4
// (integration/cloudtest/scenarios.sh, the staggered-takeover ladder for DECLINING
// ATTESTERS) — which merely collides numerically with (N+2)*30 + G = 430 at N = 12. A
// coincident total is not a measurement of this bound: 10a never exercises a lost entry
// forward. So the number governing this floor rests on the ratified MODEL
// (D-H43-WORKLESS-DESIGNEE (21)) and on call 4's instruction to size above the bound, and
// it is a conservative envelope rather than a confirmed quantity. Call 4's own stated
// release precondition named the 190 s bound and IS met; do not read that as covering this
// one. Driving the lost-forward path in the field is owed (Lane E5, at the RC grade).
const deliveryIdleBound = 430 * time.Second

// deliveryIdleStampDivisor mirrors core/node's unexported deliveryStampDivisor, which
// coarsens the last-settle stamp into buckets of window/divisor (a fine per-identity
// activity timestamp would be a finer access record than the reaper needs). cmd/silt
// cannot import it; core/node TestC2StampDivisorIsFour pins the original and cmd/silt
// TestC2ShippedFloorIsDerivedFromTheBound pins this mirror against it.
const deliveryIdleStampDivisor = 4

// deliveryIdleFloor is the smallest -delivery-idle-window the daemon accepts. The stamp
// coarsening spends up to a whole bucket of the window before the reaper ever looks, so a
// window of D only GUARANTEES D − D/divisor of survival since a real settlement (MEASURED
// at 0.751× on a 1000 s window: core/node TestC2GuaranteedSurvivalIsThreeQuartersOfTheWindow).
// The floor is therefore the bound scaled by divisor/(divisor−1), not the bound itself:
// a daemon started at exactly deliveryIdleBound reaps a session gapped only 322.5 s.
// This REPLACES the old 1 s floor, which existed only to keep the idle/2 ticker interval
// positive (blind PE item 3) and enforced nothing about the bound.
const deliveryIdleFloor = deliveryIdleBound * deliveryIdleStampDivisor / (deliveryIdleStampDivisor - 1)

// deliveryIdleFieldStall is the one stall the field has actually produced: run
// c450985-deep, block 43 committed 17 min 20 s after block 42. That is the DEFECT the A1
// fix closed, so it is not a bound — it is carried here as the margin datum the shipped
// default is required to clear.
const deliveryIdleFieldStall = 1040 * time.Second

// deliveryIdleDefault is the SHIPPED -delivery-idle-window (owner call 4 of
// D-TRUE-UP-CALLS-2026-09-07: refuse-until-set is released once the bound is
// field-confirmed, and the default is then set ABOVE the bound).
//
// 24m, not the tighter 10m that also clears the floor, for two measured reasons:
//   - 10m guarantees 450 s against a 430 s bound: 4.7 % of margin, inside the measurement
//     error of the block interval the bound's inputs are quoted at (45.865 s/height
//     measured vs the 30 s ChainSyncInterval the bound is computed at).
//   - 10m does NOT survive a repeat of the stall the field produced. Driven, both
//     candidates, both directions: core/node TestC2SessionSurvivesTheWorstAdmittedStall
//     rows candidate-tight-10m/1040s (reaped) and candidate-margin-24m/1040s (alive).
//
// It is a DURATION, not an epoch count: the bound is denominated in ChainSyncInterval and
// does not move with the block time, so an epoch-denominated default would drift away from
// the number it has to dominate. The cost of the longer window is a session slot held
// (deliveryMaxLiveSessions = 4096, and filling it is faucet-limited — one demand-domain
// anchor per session), and a deposit returned at the LATER of the anchor's release epoch
// and the close (core/credit/deliveryanchor.go CloseDeliverySession), so the release epoch
// is the binding term at this window on the measured cohort.
const deliveryIdleDefault = 24 * time.Minute

// Compile-time proof of the four sentences above, in the same arithmetic the reaper runs.
// A future edit to the bound, the divisor or the default that breaks one of them fails to
// BUILD: a negative constant does not convert to uint (measured: `deliveryIdleDefault =
// 23m` gives "constant -5000000000 overflows uint").
//
// HOW TO FALSIFY THE GATES BESIDE THESE: a guard PRE-EMPTS its twin test, so an ablation
// that trips a guard never reaches the test and proves nothing about it. To show
// TestC2AcceptedIdleFloorClearsTheLivenessBound and TestC2ShippedFloorIsDerivedFromTheBound
// are not decoration, LIFT the guard first (delete the matching line below), THEN ablate
// the constant; both go RED. The guards cover the axis more strongly than the tests do —
// unbuildable beats red, and it cannot be skipped or -shorted away — but they carry no
// property statement, which is what the tests are for.
const (
	_ = uint(deliveryIdleFloor - deliveryIdleFloor/deliveryIdleStampDivisor - deliveryIdleBound)          // the floor's guaranteed survival clears the bound
	_ = uint(deliveryIdleDefault - deliveryIdleFloor)                                                     // the default is at or above the floor
	_ = uint(deliveryIdleDefault - deliveryIdleDefault/deliveryIdleStampDivisor - deliveryIdleFieldStall) // and clears the observed field stall
	_ = uint(deliveryIdleFloor/2 - time.Second)                                                           // the idle/2 ticker interval stays positive (PE item 3)
)

// deliverySweepInterval is the wall-clock cadence of the daemon's periodic delivery
// sweep: half the INSTALLED window, so a silent server still closes an idle session
// within 1.5× the window. The half is why deliveryIdleFloor may never fall to a
// sub-second value — time.NewTicker panics on a non-positive interval (blind PE item 3),
// and at the derived floor the interval is 4m46.67s.
//
// UNGATED: R-DELIVERY-SWEEP-TICKER-UNFIRED. This function's ARITHMETIC is gated
// (TestC2SweepIntervalIsHalfTheInstalledWindow); the goroutine that FIRES it
// (cmd/silt/daemon.go) is observed at no tier. Every test caller of
// SweepDeliverySessions invokes it directly, and at the shipped 24m window the ticker
// fires at 12m, which no graded cloud flow lives long enough to see — before Lane C2 the
// cloudtest close poll was the only observation that the shipped daemon ever swept.
func deliverySweepInterval(installed time.Duration) time.Duration { return installed / 2 }

// deliverySessionCeiling is C3 (G-R212-8 cert §3.1): D_max = ⌊f/p⌋·U bytes per anchor
// and k_max_delivery = ⌈D_max·p/(U·f)⌉ = 1 anchor per open, DERIVED from the face this
// ledger charges (the ONE fee constant), never pinned. A ceiling pinned independently of
// f re-creates the burn the relay re-price removed (T-RELAY-GRAN; gate G-λ-8-1).
func deliverySessionCeiling(fee int64) (dMax int64, kMax int) {
	if fee <= 0 {
		return 0, 0
	}
	dMax = (fee / credit.DeliveryIncrementCredit) * credit.DeliveryIncrementBytes
	kMax = int((dMax*credit.DeliveryIncrementCredit + credit.DeliveryIncrementBytes*fee - 1) / (credit.DeliveryIncrementBytes * fee))
	return dMax, kMax
}

// grantFundsThePinInWholeFaces is G-λ-8-2, the QUANTIZED form of the grant/r pin: a
// face is indivisible and spent at one server, so the pin is funded iff the faces the
// pin needs on BOTH lanes fit in one grant: ⌈B_pin/D_max⌉ + ⌈B_pin/relayBytesPerAnchor⌉
// ≤ ⌊g/f⌋. Today 6 + 3 = 9 ≤ 10 — one face of margin (cert §8). This is the φ = 1
// reading (every face fully consumed); the certified refutation of the COMPOSED claim
// (φ ≪ 1 on the real path while G-6 burns the remainder) is the owner's call, not this
// gate's — the gate holds the arithmetic that must survive either way.
func grantFundsThePinInWholeFaces(grant, fee, relayBytesPerCredit int64) (need, have int64, ok bool) {
	if fee <= 0 || relayBytesPerCredit <= 0 {
		return 0, 0, false
	}
	dMax, _ := deliverySessionCeiling(fee)
	relayPerAnchor := fee * relayBytesPerCredit
	pin := credit.GrantOverRPinBytes
	need = (pin+dMax-1)/dMax + (pin+relayPerAnchor-1)/relayPerAnchor
	have = grant / fee
	return need, have, need <= have
}

// deliveryAffordabilityLine is the S5 disclosure (R2.9 gate B-11): one announced line,
// every number COMPUTED from the constants and the ledger, never typed. Registered in
// observable_contract.go with TestAffordabilityLineIsAnnounced as its asserter.
func deliveryAffordabilityLine(grant, fee, relayBytesPerCredit int64, idle string) string {
	dMax, kMax := deliverySessionCeiling(fee)
	need, have, _ := grantFundsThePinInWholeFaces(grant, fee, relayBytesPerCredit)
	return fmt.Sprintf("delivery settlement: p=%d credit per %d B (U/p=%d B/credit; self-mint Dλ=%d B/credit, PF %.2f); anchor face %d funds %d increments = %d B (%.2f GiB) per session, k_max=%d; one grant = %d faces, the 64 GiB pin needs %d faces across delivery+relay; idle window %s; unsettled remainder: a DEPOSIT returned to the fetcher's account when its anchor leaves the %d-epoch guard window (D-R2.9-NODE-HALF-CALLS 1′; the relay lane keeps the burn)",
		int64(credit.DeliveryIncrementCredit), int64(credit.DeliveryIncrementBytes), int64(credit.DeliveryBytesPerCredit), int64(credit.ServeMintBytesPerCredit),
		float64(credit.ServeMintBytesPerCredit)/float64(credit.DeliveryBytesPerCredit),
		fee, dMax/credit.DeliveryIncrementBytes, dMax, float64(dMax)/float64(1<<30), kMax, have, need, idle, credit.PaidSerialWindow+1)
}
