package credit

// R2.9 — byte-denominated per-increment delivery settlement: the LEDGER-tier RED-first
// gates (Tester's list, Researcher certification
// R2.9-build-questions-domain-rescale-guard-RESEARCH-CERTIFICATION-2026-09-04 §5),
// re-expressed at the ratified price (U, p) = (262,144, 1), Dλ = 393,216 (G-R212-7,
// 2026-09-06). Gates B-1, B-2, B-3, B-3b, B-4, B-6 live here; B-14 in
// invariant_a_test.go; B-5 and the literal pins in cmd/silt (the one package that
// imports core/credit and core/relaypay); B-7…B-13 are the node half's.
//
// RULES (cert §5): none of these call fund(); every burn goes through ChargePublish
// against the fetcher's DURABLE identity holding the shipped grant. Σ_L = Σ balances +
// Σ escrow (sumConserved) over a FIXED account set registered before the baseline.
//
// ABLATIONS that must redden (six, run and recorded in the PR): Dλ := U (parity tie ⇒
// B-1 margin ≤ 0); whole-lane reversal on a partial ack (⇒ B-1 partial, B-2); the
// payout reading l.fee (⇒ B-3); the retired "256 serves per block" unit (⇒ B-4);
// remainder → escrow (⇒ B-6); the burn counted per settlement (⇒ G-λ-8-6).

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

const (
	r29U = int64(DeliveryIncrementBytes)
	r29P = int64(DeliveryIncrementCredit)
)

// serveLane serves total bytes on (server, fetcher, root) in chunk-sized object-aware
// calls — the per-chunk shape of the production call site (core/node/node.go).
func serveLane(l *Ledger, server, fetcher ports.NodeID, root ports.Hash, total, chunk int64) {
	var i byte
	for served := int64(0); served < total; served += chunk {
		n := chunk
		if total-served < n {
			n = total - served
		}
		l.RecordServeToObject(server, fetcher, root, ports.ChunkID{i}, n)
		i++
	}
}

// openDeliverySession is the certified open: the durable fetcher buys k anchors through
// the real burn on the server's ledger, the server spends them at open, and the summed
// face is the session budget.
func openDeliverySession(t *testing.T, l *Ledger, server, fetcher ports.NodeID, epoch uint64, from, k int) int64 {
	t.Helper()
	anchors := buyAnchors(t, l, fetcher, epoch, from, k)
	face, reason := l.SpendDeliveryAnchors(server, anchors)
	if face != int64(k)*l.Fee() || reason != "" {
		t.Fatalf("SpendDeliveryAnchors(k=%d) = (%d, %q), want (%d, \"\")", k, face, reason, int64(k)*l.Fee())
	}
	return face
}

func ceilDiv(a, b int64) int64 { return (a + b - 1) / b }

// TestDeliveryAcceptStrictlyDominatesSuppressionAtEverySize — B-1 (G-1 strict at every
// size). For each B the server that BANKS the receipt ends strictly richer than the one
// that suppresses it, by exactly (j − ⌊j/8⌋) − (the self-mint reversed). At 64 MiB the
// margin is +75 credits — the R-FLAT-FEE break (+58,676,506 for suppression at λ = 1)
// closed structurally, not by the numéraire alone. The partial ack (j − 1) still pays
// for the acknowledged part and keeps the tail's bytes on the lane.
func TestDeliveryAcceptStrictlyDominatesSuppressionAtEverySize(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	const chunk = int64(64 << 10) // the shipped default publish geometry
	sizes := []int64{1 << 10, 4 << 10, 64 << 10, 1 << 20, 6_710_886, 64 << 20}
	server, fetcher := id(1), id(2)
	root := ports.HashBytes([]byte("r29-b1"))
	for _, B := range sizes {
		j := ceilDiv(B, r29U)
		k := int(ceilDiv(B, DeliveryBytesPerAnchor))
		run := func(count int64) (*Ledger, int64) {
			l := New(fee, grant)
			l.Register(server)
			l.Register(fetcher)
			budget := openDeliverySession(t, l, server, fetcher, 0, 0, k)
			serveLane(l, server, fetcher, root, B, chunk)
			paid := int64(0)
			if count >= 0 {
				var why string
				paid, why = l.SettleDelivery(server, fetcher, root, count, budget)
				if why != ReasonPaid {
					t.Fatalf("B=%d count=%d: reason %q", B, count, why)
				}
			}
			return l, paid
		}
		suppress, _ := run(-1)
		bank, paid := run(j)
		margin := bank.Balance(server) - suppress.Balance(server)
		if margin <= 0 {
			t.Fatalf("B=%d: bank − suppress = %d, want STRICTLY > 0 — a server prefers suppressing the receipt (G-1 broken)", B, margin)
		}
		value := j * r29P
		wantPaid := value - value*SkimNum/SkimDen
		if paid != wantPaid || margin != wantPaid-objNet(B) {
			t.Fatalf("B=%d: paid %d (want %d), margin %d, want paid − reversed self-mint = %d", B, paid, wantPaid, margin, wantPaid-objNet(B))
		}
		if _, live := bank.provisional[provKey{server: server, requester: fetcher, root: root}]; live {
			t.Fatalf("B=%d: a fully acknowledged lane survived the settlement", B)
		}
		if B == 64<<20 && margin != 75 {
			t.Fatalf("64 MiB margin %d, want +75 (224 paid − 149 reversed at PF 1.5) — the certified accept margin moved", margin)
		}
		// The partial ack: one increment short, when there is one to be short of.
		if j >= 2 {
			partial, ppaid := run(j - 1)
			pm := partial.Balance(server) - suppress.Balance(server)
			if pm <= 0 {
				t.Fatalf("B=%d partial j−1=%d: bank − suppress = %d, want > 0 for the acknowledged part", B, j-1, pm)
			}
			tail := B - (j-1)*r29U
			p, live := partial.provisional[provKey{server: server, requester: fetcher, root: root}]
			if !live || p.bytes != tail || p.net != objNet(tail) || p.skim != objSkim(tail) {
				t.Fatalf("B=%d partial: lane %+v, want the un-acknowledged tail of %d bytes with its own floors (%d, %d) retained", B, p, tail, objNet(tail), objSkim(tail))
			}
			if want := ((j-1)*r29P - (j-1)*r29P*SkimNum/SkimDen) - (objNet(B) - objNet(tail)); pm != want || ppaid+objNet(tail)-objNet(B) != pm {
				t.Fatalf("B=%d partial: margin %d, want %d (paid for j−1 − self-mint reversed for the acknowledged bytes)", B, pm, want)
			}
		}
	}
}

// TestProvisionalLaneEqualsCreditedBalanceAndReversesPerIncrement — B-2 (G-2 credit site
// + exact reversal). Σ p.net over a lane equals the balance those serves credited at
// every point; a settlement with count j reverses exactly the two floors over
// min(j·U, B) bytes — never more than served — and the escrow claw-back is floored at
// what the reserve holds.
func TestProvisionalLaneEqualsCreditedBalanceAndReversesPerIncrement(t *testing.T) {
	const fee = int64(50_000)
	l := New(fee, 0)
	server, fetcher := id(1), id(2)
	root := ports.HashBytes([]byte("r29-b2"))
	k := provKey{server: server, requester: fetcher, root: root}
	const B = int64(64 << 20)
	var served int64
	for served < B {
		n := int64(512 << 10)
		l.RecordServeToObject(server, fetcher, root, ports.ChunkID{byte(served >> 19)}, n)
		served += n
		p := l.provisional[k]
		if p.net != l.Balance(server) || p.net != objNet(served) || p.skim != l.EscrowBalance(root) || p.skim != objSkim(served) {
			t.Fatalf("after %d bytes: lane (net %d, skim %d), balance %d, escrow %d, want the two floors (%d, %d) == the credited amounts", served, p.net, p.skim, l.Balance(server), l.EscrowBalance(root), objNet(served), objSkim(served))
		}
	}
	// A partial settlement of j increments reverses exactly the difference of the floors.
	const j = int64(100) // 25 MiB acknowledged of 64 MiB
	balBefore, escBefore := l.Balance(server), l.EscrowBalance(root)
	value := j * r29P
	paid, why := l.SettleDelivery(server, fetcher, root, j, 1<<40)
	if why != ReasonPaid || paid != value-value*SkimNum/SkimDen {
		t.Fatalf("settle: (%d, %q)", paid, why)
	}
	tail := B - j*r29U
	wantNetRev, wantSkimRev := objNet(B)-objNet(tail), objSkim(B)-objSkim(tail)
	if got := balBefore + paid - l.Balance(server); got != wantNetRev {
		t.Fatalf("net reversed %d, want exactly %d (floor over %d bytes − floor over the %d-byte tail)", got, wantNetRev, B, tail)
	}
	if got := escBefore + value*SkimNum/SkimDen - l.EscrowBalance(root); got != wantSkimRev {
		t.Fatalf("skim reversed %d, want exactly %d", got, wantSkimRev)
	}
	p := l.provisional[k]
	if p.bytes != tail || p.net != objNet(tail) || p.skim != objSkim(tail) {
		t.Fatalf("tail lane %+v, want (%d bytes, %d, %d)", p, tail, objNet(tail), objSkim(tail))
	}
	// Never more than served: a count past the tail reverses exactly the tail's floors
	// and deletes the lane.
	balBefore = l.Balance(server)
	paid2, _ := l.SettleDelivery(server, fetcher, root, 1<<30, 1<<40)
	if got := balBefore + paid2 - l.Balance(server); got != objNet(tail) {
		t.Fatalf("over-acknowledged settle reversed %d, want the tail's %d and not one credit more", got, objNet(tail))
	}
	if _, live := l.provisional[k]; live {
		t.Fatal("lane survived a settlement that acknowledged every byte")
	}
	// Escrow floor: the skim claw-back never takes what a bounty already paid out.
	l2 := New(fee, 0)
	l2.RecordServeToObject(server, fetcher, root, ports.ChunkID{1}, 16*mintUnit) // skim 16
	if l2.PayBounty(root, id(9), 16) != 16 {
		t.Fatal("setup: bounty")
	}
	// The reserve is empty: the claw-back of the lane's 16 takes 0 (never negative), and
	// the settlement's own skim(8) = 1 lands on top.
	l2.SettleDelivery(server, fetcher, root, 1<<30, 8)
	if l2.EscrowBalance(root) != 1 || l2.escrow[root].funded != 17 {
		t.Fatalf("escrow after a floored claw-back: balance %d funded %d, want 1 / 17 (the paid-out 16 is real durability work, never recovered)", l2.EscrowBalance(root), l2.escrow[root].funded)
	}
}

// TestDeliverySettlementIsBoundedByAnchorFaceOnThePayingLedger — B-3 (G-3 spend-at-open,
// budget = Σ face). settled ≤ Σ face recorded at SPEND; a receipt with j·p > Σ face pays
// Σ face; an unanchored receipt pays 0 and leaves the self-mint; Δ Σ_L = settled − Σ face
// ≤ 0 on a fully acknowledged delivery, equality iff j·p == Σ face; the payout tracks
// the budget it is handed, never l.fee.
func TestDeliverySettlementIsBoundedByAnchorFaceOnThePayingLedger(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	server, fetcher := id(1), id(2)
	root := ports.HashBytes([]byte("r29-b3"))
	newL := func() *Ledger {
		l := New(fee, grant)
		l.Register(server)
		l.Register(fetcher)
		return l
	}
	// (a) j·p > Σ face pays Σ face; Δ Σ_L over the whole cycle = 0 (equality: budget consumed).
	l := newL()
	base := sumConserved(l)
	budget := openDeliverySession(t, l, server, fetcher, 0, 0, 2)
	if budget != 2*fee {
		t.Fatalf("budget %d, want 2 × face", budget)
	}
	serveLane(l, server, fetcher, root, 4*mintUnit, mintUnit)
	paid, why := l.SettleDelivery(server, fetcher, root, budget/r29P+1_000, budget)
	if why != ReasonPaid || paid != budget-budget*SkimNum/SkimDen {
		t.Fatalf("over-budget settle paid (%d, %q), want Σ face − skim = %d", paid, why, budget-budget*SkimNum/SkimDen)
	}
	if d := sumConserved(l) - base; d != 0 {
		t.Fatalf("Δ Σ_L = %d over buy → open → serve → full settle, want 0 (settled == Σ face)", d)
	}
	// (b) j·p < Σ face: Δ Σ_L = j·p − Σ face < 0 (the remainder burned, B-6's twin).
	l = newL()
	base = sumConserved(l)
	budget = openDeliverySession(t, l, server, fetcher, 0, 10, 1)
	serveLane(l, server, fetcher, root, 4*mintUnit, mintUnit)
	l.SettleDelivery(server, fetcher, root, 4*mintUnit/r29U, budget) // fully acknowledged
	if d := sumConserved(l) - base; d != 4*mintUnit/r29U*r29P-budget || d >= 0 {
		t.Fatalf("Δ Σ_L = %d, want j·p − Σ face = %d < 0", d, 4*mintUnit/r29U*r29P-budget)
	}
	// (c) Unanchored: pays 0, touches nothing, the self-mint stays.
	l = newL()
	serveLane(l, server, fetcher, root, 4*mintUnit, mintUnit)
	before := l.Balance(server)
	if p, why := l.SettleDelivery(server, fetcher, root, 1<<20, 0); p != 0 || why != ReasonNoAnchor || l.Balance(server) != before {
		t.Fatalf("unanchored settle (%d, %q), balance %d→%d — must pay 0 and leave the bilateral fallback", p, why, before, l.Balance(server))
	}
	if p, why := l.SettleDelivery(server, fetcher, root, 0, fee); p != 0 || why != ReasonNoIncrement || l.Balance(server) != before {
		t.Fatalf("zero-count settle (%d, %q), balance %d→%d", p, why, before, l.Balance(server))
	}
	// (d) The payout tracks the budget it is handed — a budget that is no multiple of the
	// fee pays exactly itself; the fee is never read on the payout path.
	l = newL()
	const odd = int64(12_345)
	if p, _ := l.SettleDelivery(server, fetcher, root, 1<<20, odd); p != odd-odd*SkimNum/SkimDen {
		t.Fatalf("odd budget %d paid %d, want %d — the payout read something other than the budget", odd, p, odd-odd*SkimNum/SkimDen)
	}
	if p, _ := l.SettleDelivery(server, fetcher, root, 3, 1<<40); p != 3*r29P-3*r29P*SkimNum/SkimDen {
		t.Fatalf("count 3 under a huge budget paid %d, want j·p − skim = %d", p, 3*r29P-3*r29P*SkimNum/SkimDen)
	}
}

// TestDeliveryFundTopUpSpendsFreshAnchorsOnce — B-3b. A top-up is a second all-or-nothing
// spend of FRESH anchors: Σ face rises exactly; a re-presented anchor is refused (T-3 per
// top-up) and a refused batch records nothing (T-10 per top-up).
func TestDeliveryFundTopUpSpendsFreshAnchorsOnce(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	l := New(fee, grant)
	server, fetcher := id(1), id(2)
	l.Register(server)
	l.Register(fetcher)
	budget := openDeliverySession(t, l, server, fetcher, 0, 0, 1)
	budget += openDeliverySession(t, l, server, fetcher, 0, 1, 1) // the top-up
	if budget != 2*fee || len(l.paidSerial) != 2 {
		t.Fatalf("after open + top-up: budget %d, guard %d, want 2 × face and 2 entries", budget, len(l.paidSerial))
	}
	// Re-present anchor 0 alone: refused, nothing recorded.
	if face, why := l.SpendDeliveryAnchors(server, anchorsAt(0, 0, 1)); face != 0 || why != ReasonAlreadyPaid {
		t.Fatalf("re-presented anchor: (%d, %q), want (0, already-paid)", face, why)
	}
	// A batch of one fresh + one spent anchor is refused whole; the fresh one is NOT recorded.
	fresh := buyAnchors(t, l, fetcher, 0, 5, 1)
	batch := append(fresh, anchorsAt(0, 0, 1)...)
	if face, why := l.SpendDeliveryAnchors(server, batch); face != 0 || why != ReasonAlreadyPaid {
		t.Fatalf("mixed batch: (%d, %q), want refused whole", face, why)
	}
	if len(l.paidSerial) != 2 {
		t.Fatalf("guard holds %d after a refused batch, want 2 — a refused top-up recorded an anchor (T-10)", len(l.paidSerial))
	}
	if _, spent := l.paidSerial[paidKey(0, anchorSerial(5))]; spent {
		t.Fatal("the fresh anchor of a refused batch was recorded")
	}
	// The fresh anchor still spends on its own — the fetcher lost nothing to the refusal.
	if face, why := l.SpendDeliveryAnchors(server, fresh); face != fee || why != "" {
		t.Fatalf("fresh anchor after the refusal: (%d, %q)", face, why)
	}
}

// TestPaidSerialCapDominatesBothPopulations — B-4 (G-4, UNIT half; the field half —
// guardFullRefusals == 0 per lane on a graded run, T_b and the resident cost measured —
// is owed to the Tester). The cap is derived from bytes per anchor for both populations,
// (W+1)·E, the target rate and a T_b upper bound, floored at 65,536; the derived φ = 1
// corner is pinned by independent arithmetic so the retired "256 serves per block"
// unit reddens it. It is a CORNER, not a dominance claim (T-QUANT: under spend-at-open
// the binding bound is the session count, enforced at start-up by the R2.12 assertion
// against the runtime faucet capacity). A two-population fill refuses at cap and never
// evicts; each lane's refusal moves ITS counter and the live count per lane is exported.
func TestPaidSerialCapDominatesBothPopulations(t *testing.T) {
	// Independent arithmetic at the runtime fee, never the derivation's own symbols.
	l := New(50_000, 0)
	fee := l.Fee()
	bytesPerDelivery := (fee / int64(DeliveryIncrementCredit)) * int64(DeliveryIncrementBytes)
	bytesPerRelay := fee * 524_288
	window := int64(PaidSerialWindow+1) * 8 * 3600 // (W+1) epochs × E blocks × one hour per block
	rate := int64(131_072_000)                     // 1 Gbit/s
	live := window*rate/bytesPerDelivery + window*rate/bytesPerRelay
	if bytesPerDelivery != 13_107_200_000 || bytesPerRelay != 26_214_400_000 {
		t.Fatalf("bytes per anchor at the runtime fee: delivery %d, relay %d — want 13,107,200,000 (12.21 GiB) and 26,214,400,000 (24.4 GiB)", bytesPerDelivery, bytesPerRelay)
	}
	if DeliveryBytesPerAnchor != bytesPerDelivery || RelayBytesPerAnchor != bytesPerRelay {
		t.Fatalf("derivation constants (%d, %d) disagree with the runtime fee's (%d, %d)", DeliveryBytesPerAnchor, RelayBytesPerAnchor, bytesPerDelivery, bytesPerRelay)
	}
	if live != 2_160 || derivedPaidSerialCap != 4*live {
		t.Fatalf("φ = 1 corner %d (want 2,160 = 1,440 delivery + 720 relay at 125 MiB/s and T_b = 1 h); derived cap %d, want 4 × that — the corner is not derived from bytes per anchor", live, derivedPaidSerialCap)
	}
	if int64(MaxPaidSerial) < 4*live || MaxPaidSerial != 65_536 {
		t.Fatalf("cap %d must be ≥ 4 × the corner %d and is expected to be the 65,536 floor", MaxPaidSerial, live)
	}
	// Two-population fill at one epoch: interleaved delivery and relay anchors to cap.
	server := id(1)
	half := maxPaidSerial / 2
	for i := 0; i < half; i++ {
		if _, why := l.SpendDeliveryAnchors(server, anchorsAt(0, 2*i, 1)); why != "" {
			t.Fatalf("delivery anchor %d refused early: %q", i, why)
		}
		if _, why := l.SpendRelayAnchors(anchorsAt(0, 2*i+1, 1)); why != "" {
			t.Fatalf("relay anchor %d refused early: %q", i, why)
		}
	}
	if len(l.paidSerial) != maxPaidSerial {
		t.Fatalf("guard holds %d after the fill, want the cap %d", len(l.paidSerial), maxPaidSerial)
	}
	if _, why := l.SpendDeliveryAnchors(server, anchorsAt(0, 1<<20, 1)); why != ReasonGuardFull {
		t.Fatalf("delivery open at cap: %q, want guard-full", why)
	}
	if _, why := l.SpendRelayAnchors(anchorsAt(0, 1<<20+1, 1)); why != ReasonGuardFull {
		t.Fatalf("relay open at cap: %q, want guard-full", why)
	}
	if len(l.paidSerial) != maxPaidSerial || l.GuardFullRefusals() != 2 {
		t.Fatalf("after two refusals: guard %d (want %d, nothing evicted), refusals %d (want 2)", len(l.paidSerial), maxPaidSerial, l.GuardFullRefusals())
	}
	if d, r := l.GuardFullRefusalsByLane(); d != 1 || r != 1 {
		t.Fatalf("per-lane refusals (delivery %d, relay %d), want (1, 1) — the split the 2026-09-04 cert §4.4 requires", d, r)
	}
	if d, r := l.LivePaidSerialsByLane(); d != int64(half) || r != int64(half) {
		t.Fatalf("live per lane (delivery %d, relay %d), want (%d, %d)", d, r, half, half)
	}
	for i := 0; i < maxPaidSerial; i++ { // every live entry still there — refuse-never-evict
		if _, ok := l.paidSerial[paidKey(0, anchorSerial(i))]; !ok {
			t.Fatalf("live anchor %d was evicted to admit an open", i)
		}
	}
}

// TestDeliveryRemainderIsBurnedNotEscrowed — B-6 (G-6). Settling j·p < Σ face: the
// object's escrow rises by skim(j·p) only; no account and no escrow receives the
// remainder; Σ_L falls by exactly the remainder over the cycle; the telemetry counts it.
func TestDeliveryRemainderIsBurnedNotEscrowed(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	l := New(fee, grant)
	server, fetcher, bystander := id(1), id(2), id(3)
	root := ports.HashBytes([]byte("r29-b6"))
	for _, n := range []ports.NodeID{server, fetcher, bystander} {
		l.Register(n)
	}
	base := sumConserved(l)
	balances := map[ports.NodeID]int64{server: l.Balance(server), fetcher: l.Balance(fetcher), bystander: l.Balance(bystander)}
	budget := openDeliverySession(t, l, server, fetcher, 0, 0, 1)
	const B = int64(8 * DeliveryIncrementBytes) // 2 MiB: j = 8, value 8, skim 1; the lane mints 4 net, 0 skim, all reversed at settle
	serveLane(l, server, fetcher, root, B, 64<<10)
	if l.Balance(server) != grant+objNet(B) || l.EscrowBalance(root) != objSkim(B) {
		t.Fatalf("setup: 2 MiB minted (%d, %d), want (%d, %d)", l.Balance(server)-grant, l.EscrowBalance(root), objNet(B), objSkim(B))
	}
	j := B / r29U
	paid, _ := l.SettleDelivery(server, fetcher, root, j, budget)
	value := j * r29P
	if paid != value-value*SkimNum/SkimDen || l.EscrowBalance(root) != value*SkimNum/SkimDen {
		t.Fatalf("paid %d, escrow %d, want %d and skim(j·p) = %d only", paid, l.EscrowBalance(root), value-value*SkimNum/SkimDen, value*SkimNum/SkimDen)
	}
	remainder := budget - value
	if d := sumConserved(l) - base; d != -remainder {
		t.Fatalf("Δ Σ_L = %d, want −remainder = %d: the remainder went somewhere", d, -remainder)
	}
	for n, b := range balances {
		after := l.Balance(n)
		switch n {
		case server:
			if after != b+paid {
				t.Fatalf("server %d → %d, want +%d", b, after, paid)
			}
		case fetcher:
			if after != b-fee {
				t.Fatalf("fetcher %d → %d, want −face", b, after)
			}
		default:
			if after != b {
				t.Fatalf("bystander %d → %d moved", b, after)
			}
		}
	}
	for r, e := range l.escrow {
		if r != root && e.balance != 0 {
			t.Fatalf("a foreign escrow %x holds %d", r[:4], e.balance)
		}
	}
	if burned := l.CloseDeliverySession(remainder); burned != remainder {
		t.Fatalf("close burned %d, want the remainder %d", burned, remainder)
	}
	st := l.DeliverySettlementStats()
	if st.Settlements != 1 || st.SettledCredits != value || st.SessionsClosed != 1 || st.BurnedCredits != remainder || st.SettledIncrements != j {
		t.Fatalf("telemetry %+v, want 1 settlement, %d settled, 1 closed, %d burned, %d increments", st, value, remainder, j)
	}
}

// TestBurnIsCountedOnceAtCloseNotPerSettlement — G-λ-8-6 (G-R212-8 cert §8). Under
// settle-monotone a session settles in DELTAS (delta count, remaining budget); the
// remainder is accounted once at close: no account and no escrow rises at the close,
// Σ_L falls by exactly the remainder over the cycle, and BurnedCredits rises ONCE.
// Ablation: count `budget − value` on the per-settlement path and settle in three
// deltas — the counter over-reports and this catches it.
func TestBurnIsCountedOnceAtCloseNotPerSettlement(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	l := New(fee, grant)
	server, fetcher := id(1), id(2)
	root := ports.HashBytes([]byte("r29-g8-6"))
	l.Register(server)
	l.Register(fetcher)
	base := sumConserved(l)
	budget := openDeliverySession(t, l, server, fetcher, 0, 0, 1)
	const B = int64(3 * 8 * DeliveryIncrementBytes) // 6 MiB served, settled in three deltas of 8 increments
	serveLane(l, server, fetcher, root, B, 64<<10)
	remaining, settled := budget, int64(0)
	for delta := 0; delta < 3; delta++ {
		paid, why := l.SettleDelivery(server, fetcher, root, 8, remaining)
		if why != ReasonPaid || paid != 8-8*SkimNum/SkimDen {
			t.Fatalf("delta %d: (%d, %q)", delta, paid, why)
		}
		settled += 8 * r29P
		remaining = budget - settled
		if st := l.DeliverySettlementStats(); st.BurnedCredits != 0 || st.SessionsClosed != 0 {
			t.Fatalf("delta %d: burned %d / closed %d before any close — the burn is counted per settlement", delta, st.BurnedCredits, st.SessionsClosed)
		}
	}
	balBefore, escBefore := l.Balance(server), l.EscrowBalance(root)
	l.CloseDeliverySession(remaining)
	if l.Balance(server) != balBefore || l.EscrowBalance(root) != escBefore || l.Balance(fetcher) != grant-fee {
		t.Fatal("a close moved an account or an escrow — the remainder must be burned, not routed")
	}
	if d := sumConserved(l) - base; d != -remaining {
		t.Fatalf("Δ Σ_L = %d over the cycle, want −remainder = %d", d, -remaining)
	}
	st := l.DeliverySettlementStats()
	if st.BurnedCredits != remaining || st.SessionsClosed != 1 || st.Settlements != 3 || st.SettledCredits != settled {
		t.Fatalf("telemetry %+v, want burned exactly once = %d, 1 closed, 3 settlements, %d settled", st, remaining, settled)
	}
}
