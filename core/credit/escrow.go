package credit

// Durability escrow — the H7/S7 funding layer (issue #95, docs/decisions.md
// D-S7). The repair loop that keeps content alive under churn must be paid in
// equilibrium, not charity — the wound that killed Freenet/GNUnet. This file is
// the accounting for that: a per-object credit reserve that pays repair
// bounties, kept solvent by an auto-skim of the object's own serving revenue,
// with a rarest-shard multiplier that pays most to repair the stripe closest to
// data loss.
//
// THE ONE LOAD-BEARING INVARIANT. Escrow lives in the BALANCE economy: it moves
// the credit unit between node balances and object reserves, and that is ALL it
// does. No field here ever feeds Reputation (core/credit.go). The durability
// budget confers ZERO consensus standing. If it did, the shared-content sealing
// hole (γ→1/N) would re-open: one physical copy of an erasure-coded shard could
// answer for N pledges and buy N nodes' standing. Standing is minted by the bond
// press alone (RecordBondChallenge). Every method in this file is classified
// `neutral` by the Invariant-A guard (invariant_a_test.go), and that guard fails
// the build if a new escrow method ships unclassified — so the firewall cannot
// silently erode.
//
// Prototype-first (this is H7 slice 1): these are the ledger primitives. Wiring
// the auto-skim into the live serve path (node.go) and gating PayBounty on a
// verified proof-of-repair transcript are later slices; here PayBounty trusts
// its caller, and RecordServeToObject is the object-aware serve the wiring will
// call.

import "github.com/nerolabs/silt/ports"

// objectEscrow is one object's durability reserve. balance is the credit
// currently available to pay repair bounties; funded and paid are lifetime
// totals kept for the horizon math (H7 slice 3, instrument g) and the
// observatory. None of these is ever read by Reputation.
type objectEscrow struct {
	balance int64 // credits available now to pay repair bounties
	// The two funding legs (R2.7 detector A4-1, Economist advisory §1.2). Lifetime
	// credits deposited, SPLIT by where they came from, because the wash loop's
	// recoverable money is the skim and not the prepay: with one combined `funded`
	// there is no denominator for "escrow recovered by self-repair" and the S5
	// qualifier cannot be evaluated at all.
	//
	// fundedPrepay is what an operator deposited through FundEscrow. It is NEVER
	// clawed back: a reversal only ever undoes a serve's own auto-skim.
	fundedPrepay int64
	// fundedSkim is what the serve auto-skim routed in (RecordServeToObject,
	// SettleDelivery). reverseLane subtracts from THIS leg only, floored at zero.
	fundedSkim int64
	paid       int64 // lifetime bounties paid out to repairers
	repairs    int64 // count of bounty payments — the denominator of cost-per-repair (instrument g)
}

// funded is the lifetime credits deposited (prepay + auto-skim) — the number
// EscrowFunded and DurabilitySnapshot have always published, unchanged. It is DERIVED
// from the two legs and stored nowhere: a third accumulator kept in step with two write
// paths is exactly the drift the A4-1 split exists to remove.
//
// The published figure does not move. The old combined counter's floor-at-zero sat on
// the total, so had it ever fired it would have eaten PREPAY; it cannot fire, because a
// reversal's claw-back is bounded by the lane's own recorded skim and therefore never
// exceeds the outstanding skim leg. The split makes "a reversal never touches a
// prepay" structural instead of incidental.
func (e *objectEscrow) funded() int64 { return e.fundedPrepay + e.fundedSkim }

// Skim is the protocol-fixed fraction of an object's serving revenue that routes
// back into that object's durability escrow, expressed as SkimNum/SkimDen. This
// is what makes popular data self-fund its own durability (every fetch tops up
// the reserve) while cold data draws down what it prepaid. It is a tuning
// parameter (Evolving, per the tenets), not a fixed law — 1/8 is a starting
// point; the value that actually matters is g (slice 3), the credit-cost trend
// of a shard-repair, which decides perpetual-vs-finite.
const (
	SkimNum = 1
	SkimDen = 8
)

// RepairBountyCoeffNum/Den is the dimensionless coefficient `c` in the repair-bounty
// price `base = c × shardBytes` (PE ruling 2026-08-19 Q3, RE-DERIVED 2026-09-12 —
// D-BOUNTY-PRICE-F1-2026-09-12): the base is RELATIVE to the shard the payee moves,
// never an absolute constant, because shardBytes is Evolving-tier and an absolute base
// would silently mis-price a repair the moment it re-tunes.
//
// `k` IS NOT IN THIS PRICE, and removing it is the 2026-09-12 re-pricing. The bounty is
// paid to the NEW HOLDER of the rebuilt shard (settleRepairVerdict → PayBounty); the
// reconstruction as such earns nothing by ratified design (docs/design/h7-proof-of-
// repair.md §8b; C-5 G1 2026-08-27). The holder's marginal act is ONE shard moved
// inbound, so the coherent basis is one shard of witnessed fetch price:
//
//	F1, the coherence floor:  bounty(one shard-repair, mult = 1) ≥ shardBytes/(U/p)  ⇔  c·k ≥ 1
//	the over-pay ceiling:     the payee is not paid more than the bytes it moved      ⇔  c·k ≤ 1
//	⇒ c·k = 1 EXACTLY — floor and ceiling coincide, so no value is chosen.
//
// THAT DETERMINACY IS THE BASE'S, and nothing above it. The disbursed price is
// base × RarestShardMultiplier, which runs to n−k+1: measured against the bytes the payee
// itself moved, 7× at the shipped k = 10, n = 16 and 33× at k = 1, n = 33. F1 makes it
// strictly better — the same path reached 70× at the default before — but the multiplier
// is separately ratified and unchanged here, so "no value is chosen" must never be
// published without this qualifier (blind PE, 2026-09-12).
//
// ★ THE HOLDER IS NOT ALWAYS A DIFFERENT NODE, AND F1 DOES NOT HOLD WHERE IT IS NOT.
// The paramedic KEEPS the shard it rebuilt and names ITSELF the payee whenever
// core/node (*Node).selfHoldEligible passes — the repair economy on, its own failure
// domain non-zero, and that domain unused by this stripe — and that path is tried BEFORE
// remote placement (core/node/repair.go, the (a-domain-fresh) block, PE ruling
// 2026-08-19, which created it specifically to fund reconstruction). `-domain` is a
// shipped flag and is set in integration/sybil/docker-compose.yml and
// integration/awstest/topology.py, so this is not a hypothetical path. On it the payee
// moved k survivor shards inbound to rebuild and this price pays it for ONE, so F1's own
// floor fails there by a factor of k — 10× at the shipped geometry, the mis-price F1
// exists to remove with the sign reversed. How often it is taken is a deployment
// property (~((D−1)/D)^(16−m) over D distinct domains) with no instrument, so it is not
// rare by construction.
//
// It is FILED, NOT SETTLED HERE: R-F1-FLOOR-FAILS-ON-SELF-HOLD, coupled to
// R-BOUNTY-METERS-BUT-DOES-NOT-ATTRIBUTE, which is why the meters cannot tell the two
// payees apart today. The price is deliberately NOT changed for it — a two-rate price, a
// self-hold exclusion and an accepted under-pay are all economic-mechanism changes under
// the research gate (D-S7 escrow/skim/bounty), and a code comment must not settle one by
// assertion. Nothing disburses on main today because `-economy` defaults OFF. That is a
// WEAKER reason than "by ratified design" and is written as such on purpose: the gap is
// held inert by a FLAG DEFAULT, not by a proof, and flipping that flag is a supported
// operator action.
//
// Until 2026-09-12 the basis was `k × shardBytes`, the reconstructor's survivor fetch.
// That is an act the price does not pay for on the remote-placement path, and the
// 2026-08-19 certification made
// re-deriving `c` off the payee's own basis a CONDITION of keeping the split; the split
// was ratified and the re-derivation was never performed. Measured on the shipped code
// it over-paid the reconstruction 2.31×–36.0× over the admissible loss range and the
// ratified payee's own basis 10×–60×.
//
// The SHAPE is load-bearing, not only the number. c·k = 1 is encoded by taking `k` OUT
// of the byte quantity and leaving c = 1. It must NOT be encoded as Num/Den = 1/10:
// that re-couples the price to a hard-coded k, and k is Evolving-tier — at k = 12 the
// price would silently over-pay 20 %, measured in TestF1PriceCarriesNoK. (The record
// also offered "1/10 adds a second integer floor before the last division"; that reason
// is WRONG and the gate's third arm drives why — for positive integers ⌊⌊x/a⌋/b⌋ =
// ⌊x/(a·b)⌋, so nested integer division loses nothing. The k-coupling refuses 1/10 on
// its own.) Gates: TestF1PriceIsOneShardOfWitnessedFetch, TestF1PriceCarriesNoK.
//
// Solvency is a (c, skim) PAIR — `base × m̄ × R ≤ V × skim`. Re-derived at this price:
// income per stripe-retrieval is k·shardBytes/(SkimDen·Dλ), outflow per shard-repair is
// shardBytes/(U/p), and **shardBytes CANCELS** in the ratio, so self-funding needs
//
//	S/R ≥ m̄ · SkimDen·Dλ / (k · U/p) = m̄ · 3,145,728 / 2,621,440 = 1.2·m̄ EXACTLY
//
// — 3.60 stripe-retrievals per shard-repair at m̄ = 3, against 12·m̄ = 36.00 before. Both
// are exact: the certification's 1.20007 and 12.0007 carry a spurious 0.006 % from pricing
// income at a 262,144 B shard and outflow at a 262,160 B one. The cancellation is what
// makes the threshold dimensionless, which is the same property that clears
// build-immutable #3's steerable-estimand rule. Pinned in TestF1SolvencyBandIsExact. The prior
// published single number 36 is WITHDRAWN as a point: `reachable` is the judge's own
// POST-repair count, so m̄ is bracketed [1, m] and the honest band was [12.0, 36.0] and
// is now [1.20, 3.60] (R-MULT-RACES-THE-PLACEMENT). Hot data self-funds inside the band;
// cold data stays prepay-dependent (D-S7 finite horizon), the mechanism's honest scope.
// Evolving-tier: re-tune only on field g. Certs, in order:
// silt-agent-memory/researcher/reviews/research-outcome/repair-bounty-coefficient-c-RESEARCH-CERTIFICATION-2026-08-19.md,
// .../escrow-price-repair-cost-model-RESEARCH-CERTIFICATION-2026-09-12.md,
// .../R-HOLDER-PARTICIPATION-CONSTRAINT-structural-floor-RESEARCH-CERTIFICATION-2026-09-12.md.
const (
	RepairBountyCoeffNum = 1
	RepairBountyCoeffDen = 1
)

// RepairBountyBase is the per-shard base bounty in CREDITS: c × shardBytes / (U/p) —
// ONE shard of witnessed delivery price, which is the act the payee performs (F1,
// D-BOUNTY-PRICE-F1-2026-09-12), denominated in the witnessed delivery price (G-R212-7,
// T-NUMERAIRE; before 2026-09-06 the base was implicitly 1 credit per byte, which would
// have moved D-S7's self-funding threshold by 12.6 million the moment λ moved —
// R-BOUNTY-BASE-DENOMINATION). This replaces the old absolute Config.RepairBountyBase so
// re-tuning shardBytes (Evolving-tier) re-prices repair automatically (PE Q3).
//
// `k` IS NOT IN THE PRICE. It is still a parameter because it names a DEGENERATE
// geometry (k <= 0 pays nothing) and because the judge's call sites read it from the
// erasure params they already hold; it must never re-enter the arithmetic — see
// RepairBountyCoeffNum/Den on why, and TestF1PriceIsOneShardOfWitnessedFetch, which
// drives the price at k = 2…64 and fails if it moves with k.
//
// 0 for a degenerate shard/stripe AND for any shard below one credit's worth of fetch
// (shardBytes < DeliveryBytesPerCredit — a chunk below 262,128 B; the shipped 256 KiB
// default pays 1, the 64 KiB former default pays 0). The caller's base<=0 guard means
// "off", and the judge names a zero base loudly (G-λ-8, core/node/repairclaim.go). That
// zero class WIDENED 10.008× with F1 — an accepted, disclosed cost
// (R-BOUNTY-ZERO-BELOW-262KB); the publish-time warning is what discloses it, and
// cmd/silt refuses to start if the shipped default ever falls into it.
func RepairBountyBase(k int, shardBytes int64) int64 {
	return repairBountyCredits(k, shardBytes, 1)
}

// repairBountyCredits is the ONE place a repair price is divided into credits:
// ⌊c·shardBytes·mult / (U/p)⌋, with mult the rarest-shard multiplier (1 for the
// base). Every price the ledger pays flows through here, so the floor is applied
// exactly once, at the END — G-BT-2 (BOULDER2 residual-closures certification,
// 2026-09-07, §2.6). Flooring the base FIRST and multiplying after threw away
// mult × frac(x) instead of frac(x·mult): for integer mult ≥ 1 and real a ≥ 0,
// ⌊a⌋·mult ≤ ⌊a·mult⌋ ≤ a·mult, so dividing last is never an over-pay and recovers
// up to (n−k+1)−1 credits — most on the stripe nearest data loss, which is exactly
// the stripe the multiplier exists to prioritise.
//
// `k` is read ONLY as the degenerate-geometry guard. Putting it back into the product
// is the mis-price F1 removed, and encoding c = 1/k in the coefficient instead would
// add a second integer floor here — the exact thing the sentence above forbids.
func repairBountyCredits(k int, shardBytes int64, mult int) int64 {
	if k <= 0 || shardBytes <= 0 || mult <= 0 {
		return 0
	}
	return shardBytes * int64(mult) * RepairBountyCoeffNum / RepairBountyCoeffDen / DeliveryBytesPerCredit
}

// RepairBountyTruncation prices what the floor in repairBountyCredits COSTS the
// repairer at one geometry, in integers — the money path does no floating point.
// It returns the exact price scaled by 1e5 and FLOORED (never over-stated, the same
// direction as the payment itself) and the under-pay as a fraction of the exact
// price in tenths of one percent, rounded to nearest. At a 524,280-byte shard (the
// -chunk-size 524264 geometry) that is 199,996 and 500: an exact price of 1.99996
// credits paid as 1, a 50.0 % wage cut. Nothing disburses on it.
//
// It is what the publish warning (G-BT-1) says out loud on its TRUNCATES arm — and
// since F1 that arm is UNREACHABLE through cmd/silt bountyPriceWarning, because the
// warning's threshold is the shipped default's own base, which is now 1, so "below the
// threshold" means "zero". The arithmetic stays and is driven directly here: the
// threshold is DERIVED and both U/p and the publish default are Evolving-tier, so a
// re-tune that lifts the shipped base above 1 revives the arm. R-TRUNCATION-DISCLOSURE-
// NARROWS records the cost of the gap in the meantime.
func RepairBountyTruncation(k int, shardBytes int64) (exactE5, underpayTenthsPct int64) {
	if k <= 0 || shardBytes <= 0 {
		return 0, 0
	}
	num := shardBytes * RepairBountyCoeffNum                    // the exact price's numerator
	den := int64(DeliveryBytesPerCredit) * RepairBountyCoeffDen // ... over this
	exactE5 = num * 100_000 / den
	rem := num - repairBountyCredits(k, shardBytes, 1)*den
	underpayTenthsPct = (rem*1_000 + num/2) / num
	return exactE5, underpayTenthsPct
}

// RarestShardMultiplier is how many base units one shard-repair of a stripe with
// `reachable` of its `n` shards alive is worth: every shard already lost adds one,
// so repairing the last spare before unrecoverable data loss is worth (n−k+1)× a
// top-up on a healthy stripe. That is what steers scarce repair effort at the
// stripes nearest the cliff first. A stripe at or below the k-floor is already
// clamped at the maximum; a degenerate stripe (k <= 0, n < k) is worth 0.
func RarestShardMultiplier(k, n, reachable int) int {
	if k <= 0 || n < k {
		return 0
	}
	lost := n - reachable
	if lost < 0 {
		lost = 0 // a stripe reporting more than n reachable pays the base
	}
	if maxLost := n - k; lost > maxLost {
		lost = maxLost // past the k-floor the multiplier is already maxed
	}
	return lost + 1
}

// RepairBounty is the credits one shard-repair earns: the whole price
// c·shardBytes·(lost+1) divided into credits ONCE, at the end (G-BT-2). It is the
// only pricing entry point the judge calls; RepairBountyBase exists beside it for
// the zero-signal, which must read the UNMULTIPLIED base so a stripe near the cliff
// cannot mask a geometry that pays nothing on a healthy stripe (G-λ-8).
func RepairBounty(k, n, reachable int, shardBytes int64) int64 {
	return repairBountyCredits(k, shardBytes, RarestShardMultiplier(k, n, reachable))
}

// MinBountyShardBytes is the smallest SHARD that pays a non-zero repair bounty: one
// credit of fetch, the bytes the payee moves. It was MinBountyStripeBytes (k × shardBytes)
// until F1 re-derived the price off the payee's own act; the constant's VALUE is unchanged
// and its MEANING moved from a stripe minimum to a shard minimum, which is exactly the
// 10.008× widening of the zero class that F1 costs (R-BOUNTY-ZERO-BELOW-262KB).
//
// A SHARD IS A WHOLE CIPHERTEXT CHUNK — the erasure stage takes k ciphertext chunks as the
// data shards and emits chunk-sized parity (core/pipeline/pipeline.go), and the judge reads
// a survivor's full length (core/node/repairclaim.go) — so at the former 64 KiB default the
// shard was 65,552 B and now pays 0, where under the stripe basis it paid 2. (The G-R212-7
// build and its blind PE stated the geometry as shard = chunk/k and filed
// R-DEFAULT-CHUNK-BOUNTY-ZERO on it; the Economist's 2026-09-06 advisory on the default
// chunk size caught the error.)
const MinBountyShardBytes = DeliveryBytesPerCredit

// MinBountyChunkBytesFor is the smallest plaintext CHUNK whose chunk-sized shard (the
// chunk + overhead bytes of ciphertext expansion) pays a non-zero base:
// ⌈MinBountyShardBytes / c⌉ − overhead. The publish warning and the judge's fix text derive
// their number from it, never type one. At the shipped c that is 262,128 B, against 26,199 B
// under the pre-F1 stripe basis.
//
// `k` is retained as the degenerate-geometry guard ONLY — the threshold does not move with
// it, which is the same property F1 gives the price. Dropping the parameter is owed and
// deferred: core/node/repairclaim.go is the only other caller and it is under review in
// PR #847 (D-BOUNTY-PRICE-F1-2026-09-12 §6).
func MinBountyChunkBytesFor(k int, overhead int64) int64 {
	if k <= 0 {
		return 0
	}
	var perShard int64 = (MinBountyShardBytes*RepairBountyCoeffDen + RepairBountyCoeffNum - 1) / RepairBountyCoeffNum
	if perShard <= overhead {
		return 1
	}
	return perShard - overhead
}

// escrowFor returns the object's reserve, creating an empty one on first touch.
func (l *Ledger) escrowFor(root ports.Hash) *objectEscrow {
	e, ok := l.escrow[root]
	if !ok {
		e = &objectEscrow{}
		l.escrow[root] = e
	}
	return e
}

// FundEscrow moves amount credits from funder's BALANCE into object root's
// durability reserve — the publisher (or anyone) prepaying a repair budget. It
// returns ErrInsufficientCredit without side effects if the funder cannot cover
// it, and ignores a non-positive amount. It never touches standing: the funder's
// bonded storage, and therefore its Reputation, is unchanged.
func (l *Ledger) FundEscrow(root ports.Hash, funder ports.NodeID, amount int64) error {
	if amount <= 0 {
		return nil
	}
	a := l.acct(funder)
	if l.faucet != nil {
		l.applyGrant(a) // R2.12: FundEscrow is the third SPEND GATE
	}
	if a.balance < amount {
		l.noteSpendRefused(a) // R2.7 §1.3: symmetric with ChargePublish's refusal
		return ports.ErrInsufficientCredit
	}
	a.balance -= amount
	e := l.escrowFor(root)
	e.balance += amount
	e.fundedPrepay += amount // A4-1: the PREPAY leg — never clawed back by a reversal
	return nil
}

// RecordServeToObject is the object-aware serve: like RecordServe, it credits the
// server for delivering bytes of a chunk to a requester, but it diverts an
// auto-skim (SkimNum/SkimDen) of that revenue into object root's durability
// escrow so popular data pays for its own repair. The server keeps the net; the
// skim funds the reserve. It returns the credits skimmed. Self-serving earns and
// skims nothing (the cheapest gaming, blocked exactly as in RecordServe). Bytes
// served is still counted in full for observability — the work happened; a slice
// of the reward just funds durability. Standing is untouched: serving funds the
// balance economy, never Reputation (wash-serving buys zero standing).
func (l *Ledger) RecordServeToObject(server, requester ports.NodeID, root ports.Hash, id ports.ChunkID, bytes int64) int64 {
	if bytes <= 0 || server == requester {
		return 0
	}
	// The self-credit is PROVISIONAL against a later witnessed receipt for this same
	// delivery lane, which supersedes it (delivery.go — the PoD conservation rule). An
	// unwitnessed serve keeps it: the bilateral fallback. G-R212-7: the mint is TWO
	// FLOORS off the lane's byte accumulator — ⌊(SkimDen−SkimNum)·acc/(SkimDen·Dλ)⌋ to
	// the server and ⌊SkimNum·acc/(SkimDen·Dλ)⌋ to the escrow — each leg an under-pay
	// with its own remainder, so the skim accumulates across serves instead of
	// flooring to zero per call (G-λ-7), and both recorded amounts stay exact for
	// reverseProvisional. The remainder lives on the LANE and dies with it at both
	// terminal sites (redeem, eviction): a remainder on the account would survive a
	// witnessed supersede and later mint for bytes the receipt already paid (G-λ-5).
	p := l.laneFor(server, requester, root)
	p.bytes += bytes
	l.serveBytesObjectAware += bytes // A2: the witnessable denominator (credit.go)
	netTotal := p.bytes * (SkimDen - SkimNum) / (SkimDen * ServeMintBytesPerCredit)
	skimTotal := p.bytes * SkimNum / (SkimDen * ServeMintBytesPerCredit)
	net, skim := netTotal-p.net, skimTotal-p.skim
	p.net, p.skim = netTotal, skimTotal
	s := l.acct(server)
	s.balance += net
	s.servedBytes += bytes // the byte observables stay BYTES (G-λ-9)
	l.recordFetched(requester, bytes)
	e := l.escrowFor(root)
	e.balance += skim
	e.fundedSkim += skim // A4-1: the auto-SKIM leg — the only leg a reversal claws back
	l.noteServeMint(bytes, net, skim)
	return skim
}

// PayBounty releases up to amount credits from object root's durability escrow to
// a repairer, crediting the repairer's BALANCE — durability work earns spendable
// credit, never standing. It pays min(amount, available): when the reserve is
// short it pays what is left and returns that, which is exactly the object's
// funded horizon running out (finite-but-renewable; re-endow before expiry). It
// returns the credits actually paid.
//
// SLICE-1 CONTRACT: the caller is trusted to invoke this only on a repair it has
// VERIFIED. The proof-of-repair gate — bounty releases iff BOTH correctness and
// retrievability verify, and a false claim is bond-slashed — is H7 slice 2 and
// wraps this primitive; it is not enforced here.
func (l *Ledger) PayBounty(root ports.Hash, repairer ports.NodeID, amount int64) int64 {
	if amount <= 0 {
		return 0
	}
	e, ok := l.escrow[root]
	if !ok || e.balance <= 0 {
		return 0
	}
	if amount > e.balance {
		amount = e.balance // pay what the reserve can cover; the horizon is spent
	}
	e.balance -= amount
	e.paid += amount
	e.repairs++ // one shard-repair funded (a short final payment still counts as one)
	r := l.acct(repairer)
	r.balance += amount
	// Per-NODE repair observability: count the repair against the repairer and
	// accumulate the credits it earned. This is the per-node dual of the
	// per-object e.repairs — the escrow count cannot say WHO did the work, and the
	// repair-work concentration metric (economy observability) needs the per-node
	// series. It feeds no standing, no conservation, no disbursement rule; it only
	// records observable work already paid. Standing stays minted by the bond press
	// alone (Reputation), so this cannot re-open the γ→1/N firewall.
	r.repairsDone++
	r.bountyEarned += amount
	if r.fetchedBytes > 0 {
		// A4-3 (credit.go): the repairer had already fetched from this node when the
		// bounty was released. A SHAPE, never a detection — the payment above is
		// unchanged, and nothing downstream may refuse or slash on this.
		l.bountyToPriorFetcherPayments++
		l.bountyToPriorFetcherCredits += amount
	}
	return amount
}

// BountyToPriorFetcher reports the A4-3 wash SHAPE: how many repair bounties this
// ledger released to a repairer that had already fetched bytes from this node, and
// the credits those payments carried. Node-wide; it names no repairer and no object.
//
// One-sided-informative ONLY. A repairer may legitimately have fetched survivor
// shards from this judge, so a non-zero reading is not evidence of a wash — read it
// beside the wash symmetry, and never as a slashing or disbursement input.
// Observability; reading moves nothing.
func (l *Ledger) BountyToPriorFetcher() (payments, credits int64) {
	return l.bountyToPriorFetcherPayments, l.bountyToPriorFetcherCredits
}

// EscrowBalance is the credit currently available to pay repair bounties for
// object root — its remaining funded horizon in credits. Observability; reading
// it moves nothing.
func (l *Ledger) EscrowBalance(root ports.Hash) int64 {
	if e, ok := l.escrow[root]; ok {
		return e.balance
	}
	return 0
}

// EscrowFunded is the lifetime credits deposited into object root's reserve
// (prepay plus auto-skim) — the numerator the horizon math draws on.
func (l *Ledger) EscrowFunded(root ports.Hash) int64 {
	if e, ok := l.escrow[root]; ok {
		return e.funded()
	}
	return 0
}

// EscrowPaid is the lifetime bounties paid out of object root's reserve — the
// realised cost of keeping it alive so far (the g-instrument reads this).
func (l *Ledger) EscrowPaid(root ports.Hash) int64 {
	if e, ok := l.escrow[root]; ok {
		return e.paid
	}
	return 0
}

// EscrowRepairs is the count of bounty payments object root's reserve has funded —
// the denominator of cost-per-repair.
func (l *Ledger) EscrowRepairs(root ports.Hash) int64 {
	if e, ok := l.escrow[root]; ok {
		return e.repairs
	}
	return 0
}

// DurabilitySnapshot captures object root's whole durability accounting in one
// value, for the finite-but-renewable instruments (see instruments.go). Reading
// it moves nothing; an object with no reserve reports the zero snapshot.
func (l *Ledger) DurabilitySnapshot(root ports.Hash) ports.DurabilitySnapshot {
	e, ok := l.escrow[root]
	if !ok {
		return ports.DurabilitySnapshot{}
	}
	return ports.DurabilitySnapshot{
		Balance:      e.balance,
		Funded:       e.funded(),
		FundedPrepay: e.fundedPrepay,
		FundedSkim:   e.fundedSkim,
		Paid:         e.paid,
		Repairs:      e.repairs,
	}
}
