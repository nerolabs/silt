package credit

// The credit NUMÉRAIRE — every byte→credit conversion in the balance economy, derived
// from ONE price pair (G-R212-7, Researcher certification 2026-09-06, owner-ratified the
// same day: silt-reviews/research/research-outcome/G-R212-7-lambda-redenomination-
// RESEARCH-CERTIFICATION-2026-09-06.md §11; docs/decisions.md D-R2.9a-RUN-CALLS).
//
// T-NUMERAIRE: a constant is coupled iff it converts bytes into credits or is compared
// against one that does. Three such constants must stay strictly ordered,
//
//	U/p  <  Dλ  ≤  RelayIncrementBytes/RelayIncrementCredit
//
// where U/p is the witnessed delivery price (bytes per credit), Dλ the unwitnessed
// serve-mint denomination, and the right-hand side the relay lane's price already
// ratified at 512 KiB per credit. The left inequality is STRICT parity (D-R2.9-DIRECTION
// ruling 3: a witnessed delivery must pay more than the self-mint for the same bytes);
// the right one is Don't #7 (reward tracks value: a relay that stores nothing may not
// out-earn the node that served the bytes). Both are derived here, never pinned twice;
// the constants gates in cmd/silt (which can import core/relaypay) hold the ordering.
//
// The fee f = 50,000 and the grant g = 500,000 do NOT move (ONE FACE; G-BB-30′): the
// 64 GiB grant/r pin is realized in the PRICE, and λ and the bounty base follow it.

// DeliveryIncrementBytes (U) and DeliveryIncrementCredit (p) are R2.9's per-increment
// delivery price: one credit per 256 KiB witnessed. The smallest power of two clearing
// G-R212-6 (U/p ≥ 186,268 bytes per credit, the delivery lane's share of the 64 GiB pin
// after the relay lane's), with a 27 % pin margin: one grant buys 81.38 GiB across the
// SUM of the two prices. R2.9 consumes these; until it lands the witnessed leg is the
// flat fee − skim and parity is vacuous. The provisional (4,096, 4,097) was REFUTED
// outright (186,313× short of the pin).
const (
	DeliveryIncrementBytes  = 262_144
	DeliveryIncrementCredit = 1
)

// DeliveryBytesPerCredit is U/p: the bytes one credit buys on the witnessed lane. It is
// the denominator of the repair-bounty base (RepairBountyBase) — the fetch price a
// repair is worth — and the floor Dλ must sit strictly above.
const DeliveryBytesPerCredit = DeliveryIncrementBytes / DeliveryIncrementCredit

// ServeMintBytesPerCredit is Dλ: the unwitnessed serve mints ONE credit per Dλ bytes
// served (λ = 1/Dλ credits per byte). Derived as ⌈3·U/(2·p)⌉ = 393,216 (384 KiB):
// a 1.5× STRICT-parity margin above U/p, and below the relay lane's 524,288 under
// BOTH readings of Don't #7 (gross ≤ 524,288; net ≤ 458,752). Not a pin — a future
// price re-tune re-derives it; pinning it independently would break parity in one
// direction or Don't #7 in the other on the next move.
//
// Minting is a FLOOR over a per-lane byte accumulator (RecordServe / RecordServeToObject):
// ⌊acc/Dλ⌋, an under-pay, never a mint, remainder < Dλ. At a 64 KiB chunk (the former
// default; the 256 KiB default is still below Dλ) a per-call floor would mint zero on
// 100 % of serves (R-LAMBDA-DUST), so the accumulator
// is REQUIRED; the remainder lives on the provisional LANE for object-aware serves (a
// witnessed redeem deletes the lane, and a remainder that survived on the account would
// later mint for bytes the witnessed leg already paid — the one double-pay path) and on
// the account for the plain path, which has no lane and no supersede.
const ServeMintBytesPerCredit = (3*DeliveryIncrementBytes + 2*DeliveryIncrementCredit - 1) / (2 * DeliveryIncrementCredit)

// GrantOverRPinBytes is the ratified 64 GiB grant/r pin (D-R2.9a-RUN-CALLS, 2026-09-05):
// one starter grant must buy at least this many bytes across the SUM of the prices a
// NAT'd fetcher pays at once (delivery + relay). Read by the priced-lane start-up
// refusal in cmd/silt (G-λ-3). The structural floor beneath it, S_max × N/K ≈ 44.7 GiB,
// is derived in that gate from erasure.DefaultParams, never transcribed.
const GrantOverRPinBytes int64 = 64 << 30

// ServeMintStats is the serve-mint telemetry (Economist §6 items 1–4, endorsed by the
// certification as instrument-class): what the unwitnessed lane minted against what it
// served, and how much served work is still waiting below a mint boundary. Node-wide
// aggregates only — never joined to an identity axis (Don't #3).
type ServeMintStats struct {
	BytesPerCredit int64 // Dλ, so a reader can compute the realized λ
	ServedBytes    int64 // bytes served through both serve paths, lifetime
	// MintedCredits / SkimmedCredits are GROSS of reversal: a witnessed redeem or a lane
	// eviction reverses a lane's mint after the fact and is counted in ReversedCredits, so
	// the realized balance effect of the unwitnessed lane is Minted + Skimmed − Reversed.
	MintedCredits   int64 // credits minted to servers by both paths (net of the skim on the object path), gross of reversal
	SkimmedCredits  int64 // credits skimmed into escrows by the object path, gross of reversal
	ReversedCredits int64 // credits (net + skim) later reversed by a witnessed supersede or a lane eviction
	ZeroMintServes  int64 // serve calls that minted nothing (their bytes are in a remainder)
	// The two legs of the object path floor the same accumulator at different boundaries
	// (8·Dλ/7 for the server, 8·Dλ for the escrow), so each carries its own remainder; the
	// plain path's account remainder is reported on the server leg.
	RemainderBytesServerLeg int64 // bytes waiting below the next SERVER credit, across accounts and live lanes
	RemainderBytesEscrowLeg int64 // bytes waiting below the next ESCROW credit, across live lanes (the leg that funds D-S7)

	// A2 supersede-suppression (R2.7). ObjectAwareBytes / WitnessedBytes /
	// LaneEvictedBytes are the three stored node-wide counters (credit.go);
	// InFlightBytes is the live-lane sum taken by the walk this reader already makes.
	// They satisfy an exact conservation identity, asserted by
	// TestServedByteSplitIsExactAcrossEveryTerminalState:
	//
	//	ObjectAwareBytes == WitnessedBytes + LaneEvictedBytes + InFlightBytes
	//
	// The identity has THREE terminal terms, not four. The Economist's spec named a
	// fourth, serveBytesSupersededFlat, on the flat delivery leg's supersede block and
	// said in the same sentence that it "is deleted with the leg". Lane C1 deleted the
	// leg (RedeemDeliveryCreditReason and the core/demand v2 primitive are gone), so
	// there is no site left to count and a fourth counter would be a permanent zero.
	ObjectAwareBytes int64
	WitnessedBytes   int64
	LaneEvictedBytes int64
	InFlightBytes    int64

	// The three DERIVED read-side numbers. They are computed here on every read and
	// stored nowhere: a second accumulator for a residual invites drift between two
	// write paths, and the states above are the things that actually happen.
	UnwitnessedBytes int64 // ServedBytes − WitnessedBytes: the headline an operator wants
	// UnwitnessableBytes is ServedBytes − ObjectAwareBytes: the plain-path floor (a
	// manifest chunk has no root and can never be witnessed). It is NEVER a suppression
	// signal — a node serving manifests is doing honest work.
	UnwitnessableBytes int64
	// ReceiptCoverage is the A2 number: WitnessedBytes / ObjectAwareBytes. The
	// denominator is object-aware bytes, NOT total served bytes. It is 0 when nothing
	// witnessable has been served yet — read it beside ObjectAwareBytes, because a zero
	// denominator and total suppression print the same number.
	ReceiptCoverage float64
}

// ServeMintStats reads the serve-mint telemetry. Reading moves nothing; the remainder
// walk is bounded by the account map and the live-lane map (the same walk Gini makes).
func (l *Ledger) ServeMintStats() ServeMintStats {
	st := ServeMintStats{BytesPerCredit: ServeMintBytesPerCredit, ServedBytes: l.serveBytes, MintedCredits: l.serveMintCredits,
		SkimmedCredits: l.serveSkimCredits, ReversedCredits: l.serveReversedCredits, ZeroMintServes: l.serveMintZero}
	for _, a := range l.accounts {
		st.RemainderBytesServerLeg += a.serveRemainder
	}
	for _, p := range l.provisional {
		// Two floors off one accumulator: the server's leg has minted ⌊7b/(8Dλ)⌋, so the
		// bytes not yet covered by a server credit are b − net·8Dλ/7; the escrow's leg has
		// minted ⌊b/(8Dλ)⌋, leaving b − skim·8Dλ.
		st.RemainderBytesServerLeg += p.bytes - p.net*SkimDen*ServeMintBytesPerCredit/(SkimDen-SkimNum)
		st.RemainderBytesEscrowLeg += p.bytes - p.skim*SkimDen*ServeMintBytesPerCredit/SkimNum
		st.InFlightBytes += p.bytes // A2: the live term of the conservation identity, on the walk already made
	}
	st.ObjectAwareBytes = l.serveBytesObjectAware
	st.WitnessedBytes = l.serveBytesWitnessed
	st.LaneEvictedBytes = l.serveBytesLaneEvicted
	st.UnwitnessedBytes = st.ServedBytes - st.WitnessedBytes
	st.UnwitnessableBytes = st.ServedBytes - st.ObjectAwareBytes
	if st.ObjectAwareBytes > 0 {
		st.ReceiptCoverage = float64(st.WitnessedBytes) / float64(st.ObjectAwareBytes)
	}
	return st
}
