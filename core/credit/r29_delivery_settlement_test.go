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
	"reflect"
	"strings"
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
				_, paid, why = l.SettleDelivery(server, fetcher, root, count, budget, 0)
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
	_, paid, why := l.SettleDelivery(server, fetcher, root, j, 1<<40, 0)
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
	_, paid2, _ := l.SettleDelivery(server, fetcher, root, 1<<30, 1<<40, 0)
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
	l2.SettleDelivery(server, fetcher, root, 1<<30, 8, 0)
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
	_, paid, why := l.SettleDelivery(server, fetcher, root, budget/r29P+1_000, budget, 0)
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
	l.SettleDelivery(server, fetcher, root, 4*mintUnit/r29U, budget, 0) // fully acknowledged
	if d := sumConserved(l) - base; d != 4*mintUnit/r29U*r29P-budget || d >= 0 {
		t.Fatalf("Δ Σ_L = %d, want j·p − Σ face = %d < 0", d, 4*mintUnit/r29U*r29P-budget)
	}
	// (c) Unanchored: pays 0, touches nothing, the self-mint stays.
	l = newL()
	serveLane(l, server, fetcher, root, 4*mintUnit, mintUnit)
	before := l.Balance(server)
	if _, p, why := l.SettleDelivery(server, fetcher, root, 1<<20, 0, 0); p != 0 || why != ReasonNoAnchor || l.Balance(server) != before {
		t.Fatalf("unanchored settle (%d, %q), balance %d→%d — must pay 0 and leave the bilateral fallback", p, why, before, l.Balance(server))
	}
	if _, p, why := l.SettleDelivery(server, fetcher, root, 0, fee, 0); p != 0 || why != ReasonNoIncrement || l.Balance(server) != before {
		t.Fatalf("zero-count settle (%d, %q), balance %d→%d", p, why, before, l.Balance(server))
	}
	// (d) The payout tracks the budget it is handed — a budget that is no multiple of the
	// fee pays exactly itself; the fee is never read on the payout path.
	l = newL()
	const odd = int64(12_345)
	if _, p, _ := l.SettleDelivery(server, fetcher, root, 1<<20, odd, 0); p != odd-odd*SkimNum/SkimDen {
		t.Fatalf("odd budget %d paid %d, want %d — the payout read something other than the budget", odd, p, odd-odd*SkimNum/SkimDen)
	}
	if _, p, _ := l.SettleDelivery(server, fetcher, root, 3, 1<<40, 0); p != 3*r29P-3*r29P*SkimNum/SkimDen {
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

// TestDeliveryRemainderIsNeverRoutedToEscrow — B-6, re-expressed under the ratified deposit
// (D-R2.9-NODE-HALF-CALLS 1′). Settling j·p < Σ face: the object's escrow rises by skim(j·p)
// only; no account and no escrow receives the remainder AT CLOSE — it is booked as a
// pending deposit (Σ_L is down by exactly the remainder until the anchor expires) and
// returns to the fetcher's EXISTING account, whole, when the anchor leaves the window.
func TestDeliveryRemainderIsNeverRoutedToEscrow(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	l := New(fee, grant)
	src := &mockEpochSource{}
	l.SetEpochSource(src)
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
	_, paid, _ := l.SettleDelivery(server, fetcher, root, j, budget, 0)
	value := j * r29P
	if paid != value-value*SkimNum/SkimDen || l.EscrowBalance(root) != value*SkimNum/SkimDen {
		t.Fatalf("paid %d, escrow %d, want %d and skim(j·p) = %d only", paid, l.EscrowBalance(root), value-value*SkimNum/SkimDen, value*SkimNum/SkimDen)
	}
	remainder := budget - value
	if booked := l.CloseDeliverySession(fetcher, remainder, 0); booked != remainder {
		t.Fatalf("close booked %d, want the remainder %d as a deposit", booked, remainder)
	}
	// At close: Σ_L is down by the remainder (it is pending), no account or escrow received it.
	if d := sumConserved(l) - base; d != -remainder {
		t.Fatalf("Δ Σ_L = %d at close, want −remainder = %d: the remainder went somewhere", d, -remainder)
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
				t.Fatalf("fetcher %d → %d at close, want −face (the deposit is not yet spendable)", b, after)
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
	st := l.DeliverySettlementStats()
	if st.Settlements != 1 || st.SettledCredits != value || st.SessionsClosed != 1 || st.PendingRefundCredits != remainder || st.BurnedCredits != 0 || st.SettledIncrements != j {
		t.Fatalf("telemetry %+v, want 1 settlement, %d settled, 1 closed, %d pending, 0 burned, %d increments", st, value, remainder, j)
	}
	// The anchor (epoch 0) leaves the window at W+1: the deposit returns to the fetcher whole.
	src.e = uint64(PaidSerialWindow) + 1
	l.ReleaseDueRefunds()
	if l.Balance(fetcher) != balances[fetcher]-fee+remainder || sumConserved(l)-base != -value+value {
		t.Fatalf("after release: fetcher %d (want %d), Δ Σ_L %d (want 0 — the face's legs cancel exactly)", l.Balance(fetcher), balances[fetcher]-fee+remainder, sumConserved(l)-base)
	}
	if st := l.DeliverySettlementStats(); st.RefundedCredits != remainder || st.PendingRefundCredits != 0 {
		t.Fatalf("after release: %+v", st)
	}
}

// TestRemainderIsAccountedOnceAtCloseNotPerSettlement — G-λ-8-6 (G-R212-8 cert §8), under
// the ratified deposit. A session settles in DELTAS; the remainder is accounted once at
// close (booked pending — not burned, not routed): no account and no escrow rises at the
// close, Σ_L is down by exactly the remainder, and PendingRefundCredits rises ONCE.
// Ablation: account `budget − value` on the per-settlement path and settle in three deltas
// — the pending counter over-reports and this catches it.
func TestRemainderIsAccountedOnceAtCloseNotPerSettlement(t *testing.T) {
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
		_, paid, why := l.SettleDelivery(server, fetcher, root, 8, remaining, settled)
		if why != ReasonPaid || paid != 8-8*SkimNum/SkimDen {
			t.Fatalf("delta %d: (%d, %q)", delta, paid, why)
		}
		settled += 8 * r29P
		remaining = budget - settled
		if st := l.DeliverySettlementStats(); st.PendingRefundCredits != 0 || st.BurnedCredits != 0 || st.SessionsClosed != 0 {
			t.Fatalf("delta %d: pending %d / burned %d / closed %d before any close — the remainder is accounted per settlement", delta, st.PendingRefundCredits, st.BurnedCredits, st.SessionsClosed)
		}
	}
	balBefore, escBefore := l.Balance(server), l.EscrowBalance(root)
	l.CloseDeliverySession(fetcher, remaining, 0)
	if l.Balance(server) != balBefore || l.EscrowBalance(root) != escBefore || l.Balance(fetcher) != grant-fee {
		t.Fatal("a close moved an account or an escrow — the remainder must be booked, not routed")
	}
	if d := sumConserved(l) - base; d != -remaining {
		t.Fatalf("Δ Σ_L = %d over the cycle, want −remainder = %d", d, -remaining)
	}
	st := l.DeliverySettlementStats()
	if st.PendingRefundCredits != remaining || st.SessionsClosed != 1 || st.Settlements != 3 || st.SettledCredits != settled || st.BurnedCredits != 0 {
		t.Fatalf("telemetry %+v, want pending exactly once = %d, 1 closed, 3 settlements, %d settled, 0 burned", st, remaining, settled)
	}
}

// ---- the settlement skim under fetcher-chosen deltas (G-SKIM-1, -2, -5; certification
// R2.9-settlement-skim-under-fetcher-chosen-deltas-2026-09-06 §7). Ablation for all three:
// restore `skim := value*SkimNum/SkimDen` (the per-settlement floor).

// settleSession drives one face to exhaustion at a fixed delta, passing the cumulative
// settled as prior exactly as the node does.
func settleSession(t *testing.T, l *Ledger, server, fetcher ports.NodeID, root ports.Hash, budget, delta int64) {
	t.Helper()
	var settled int64
	for settled < budget {
		s, _, why := l.SettleDelivery(server, fetcher, root, delta, budget-settled, settled)
		if why != ReasonPaid {
			t.Fatalf("delta %d at settled %d: %q", delta, settled, why)
		}
		settled += s
	}
}

// TestSkimIsExactOverTheSessionNotPerSettlement — G-SKIM-1: the PE's measured table as a
// permanent gate. One whole face at deltas 1, 7, 8, 5,000 and 50,000: escrow 6,250 and
// server 43,750 in every arm.
func TestSkimIsExactOverTheSessionNotPerSettlement(t *testing.T) {
	for _, delta := range []int64{1, 7, 8, 5_000, 50_000} {
		l := New(50_000, 0)
		server, fetcher := id(1), id(2)
		root := ports.HashBytes([]byte("g-skim-1"))
		l.Register(server)
		settleSession(t, l, server, fetcher, root, 50_000, delta)
		if l.EscrowBalance(root) != 6_250 || l.Balance(server) != 43_750 {
			t.Fatalf("delta %d: escrow %d, server %d — want 6,250 / 43,750 at EVERY granularity (a payer-chosen delta must not move the split)", delta, l.EscrowBalance(root), l.Balance(server))
		}
	}
}

// TestSettlementSkimNeverExceedsItsOwnValue — G-SKIM-2: 0 ≤ skim_i ≤ value_i, so the payout
// is never negative. The binding case: prior 7, value 1 ⇒ skim 1, paid 0.
func TestSettlementSkimNeverExceedsItsOwnValue(t *testing.T) {
	l := New(50_000, 0)
	server, fetcher := id(1), id(2)
	root := ports.HashBytes([]byte("g-skim-2"))
	l.Register(server)
	base := sumConserved(l)
	settled, paid, why := l.SettleDelivery(server, fetcher, root, 1, 1<<20, 7)
	if why != ReasonPaid || settled != 1 || paid != 0 || l.EscrowBalance(root) != 1 || l.Balance(server) != 0 {
		t.Fatalf("(settled %d, paid %d, %q), escrow %d, server %d — want the boundary credit to go whole to the escrow and the server paid 0, never negative", settled, paid, why, l.EscrowBalance(root), l.Balance(server))
	}
	if sumConserved(l)-base != 1 {
		t.Fatalf("Σ_L moved by %d, want exactly the settled credit", sumConserved(l)-base)
	}
	for prior := int64(0); prior < 64; prior++ {
		for value := int64(1); value < 20; value++ {
			skim := (prior+value)*SkimNum/SkimDen - prior*SkimNum/SkimDen
			if skim < 0 || skim > value {
				t.Fatalf("prior %d value %d: skim %d outside [0, value]", prior, value, skim)
			}
		}
	}
}

// TestWitnessingNeverDefundsTheEscrow — G-SKIM-5, the composed claim: serve 64 MiB (the
// unwitnessed leg deposits its skim), then witness 256 increments in deltas of 1. The
// escrow ends STRICTLY higher than after the serve alone, and the witnessed deposit is at
// least PF × the unwitnessed one (32 vs 21). Ablation: the per-settlement floor ⇒ the
// serve-time skim is clawed back and nothing replaces it — the escrow ends at 0, net −21.
func TestWitnessingNeverDefundsTheEscrow(t *testing.T) {
	l := New(50_000, 0)
	server, fetcher := id(1), id(2)
	root := ports.HashBytes([]byte("g-skim-5"))
	l.Register(server)
	const B = int64(64 << 20)
	serveLane(l, server, fetcher, root, B, 512<<10)
	afterServe := l.EscrowBalance(root)
	if afterServe != objSkim(B) || afterServe != 21 {
		t.Fatalf("setup: the unwitnessed leg deposited %d, want 21", afterServe)
	}
	settleSession(t, l, server, fetcher, root, 256, 1) // 256 increments of 256 KiB = the 64 MiB, one at a time
	if got := l.EscrowBalance(root); got <= afterServe || got != 256*SkimNum/SkimDen {
		t.Fatalf("escrow after witnessing %d (after serve alone %d) — witnessing must ADD funding (want 32 = ⌊256/8⌋), never claw it back and replace it with nothing", got, afterServe)
	}
	if _, live := l.ProvisionalLaneForTest(server, fetcher, root); live {
		t.Fatal("the fully witnessed lane survived")
	}
}

// ---- the deposit released at anchor expiry (G-6R-1…6, 8, 9, 10; certification
// R2.9-session-remainder-refund-and-live-anchor-cap-2026-09-06 §7; D-R2.9-NODE-HALF-CALLS 1′).
// Ablations: release at close (releaseEpoch := watermark) ⇒ G-6R-3, G-6R-4 RED; credit
// through acct() ⇒ G-6R-2 RED; refund `budget` instead of `budget − settled` ⇒ G-6R-1 RED;
// unbounded pending table ⇒ G-6R-6 RED.

// TestRemainderIsConservedAcrossCloseAndRelease — G-6R-1. Withdrawal → open → serve → settle →
// close → release: Σ_L returns to its pre-withdrawal value plus nothing but the
// un-acknowledged tail mint; remainder = refunded + pending + burned at every point.
func TestRemainderIsConservedAcrossCloseAndRelease(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	l := New(fee, grant)
	src := &mockEpochSource{}
	l.SetEpochSource(src)
	server, fetcher := id(1), id(2)
	root := ports.HashBytes([]byte("g6r-1"))
	l.Register(server)
	l.Register(fetcher)
	base := sumConserved(l)
	budget := openDeliverySession(t, l, server, fetcher, 0, 0, 1)
	const B = int64(40 * DeliveryIncrementBytes)
	serveLane(l, server, fetcher, root, B, 512<<10)
	settled, _, _ := l.SettleDelivery(server, fetcher, root, 32, budget, 0) // 32 of 40 increments acknowledged
	remainder := budget - settled
	tail := objNet(B-32*r29U) + objSkim(B-32*r29U) // the un-acknowledged 8 increments' self-mint stays (the bilateral fallback)
	if booked := l.CloseDeliverySession(fetcher, remainder, 0); booked != remainder {
		t.Fatalf("booked %d, want %d", booked, remainder)
	}
	check := func(when string) {
		st := l.DeliverySettlementStats()
		if st.RefundedCredits+st.PendingRefundCredits+st.BurnedCredits != remainder {
			t.Fatalf("%s: refunded %d + pending %d + burned %d != remainder %d", when, st.RefundedCredits, st.PendingRefundCredits, st.BurnedCredits, remainder)
		}
	}
	check("at close")
	if d := sumConserved(l) - base; d != -remainder+tail {
		t.Fatalf("Δ Σ_L at close %d, want −remainder + tail mint = %d", d, -remainder+tail)
	}
	src.e = uint64(PaidSerialWindow) + 1
	l.ReleaseDueRefunds()
	check("after release")
	if d := sumConserved(l) - base; d != tail {
		t.Fatalf("Δ Σ_L after release %d, want exactly the un-acknowledged tail mint %d — the face's legs cancel", d, tail)
	}
	if l.Balance(fetcher) != grant-fee+remainder {
		t.Fatalf("fetcher %d, want grant − face + remainder = %d", l.Balance(fetcher), grant-fee+remainder)
	}
}

// TestReleaseNeverRegistersAnAccount — G-6R-2 (M1). A deposit whose fetcher has no account
// here is burned and counted; no account is registered, no grant minted.
func TestReleaseNeverRegistersAnAccount(t *testing.T) {
	l := New(50_000, 500_000)
	src := &mockEpochSource{}
	l.SetEpochSource(src)
	server, stranger := id(1), id(9)
	l.Register(server)
	base := sumConserved(l)
	accounts := len(l.Balances())
	l.CloseDeliverySession(stranger, 12_345, 0)
	src.e = uint64(PaidSerialWindow) + 1
	l.ReleaseDueRefunds()
	st := l.DeliverySettlementStats()
	if len(l.Balances()) != accounts || sumConserved(l) != base {
		t.Fatalf("release registered an account (%d → %d) or moved Σ_L by %d — acct() minted a grant (the RT-RELAY-1 phantom)", accounts, len(l.Balances()), sumConserved(l)-base)
	}
	if st.BurnedCredits != 12_345 || st.RefundsBurnedNoAccount != 1 || st.PendingRefundCredits != 0 || st.RefundedCredits != 0 {
		t.Fatalf("telemetry %+v, want the remainder burned once as no-account", st)
	}
}

// TestRemainderIsNotSpendableUntilTheAnchorExpires — G-6R-3 (M2). Close at epoch E with an
// anchor issued at epoch A: at every watermark ≤ A + W the fetcher's balance is unchanged
// and a withdrawal still refuses; at A + W + 1 the balance rises by exactly the remainder.
func TestRemainderIsNotSpendableUntilTheAnchorExpires(t *testing.T) {
	const fee = int64(50_000)
	l := New(fee, 0)
	src := &mockEpochSource{}
	l.SetEpochSource(src)
	server, fetcher := id(1), id(2)
	l.Register(server)
	l.Register(fetcher)
	l.acct(fetcher).balance = fee // exactly one face: the whole balance goes into the anchor
	src.e = 3
	budget := openDeliverySession(t, l, server, fetcher, 3, 0, 1)
	l.CloseDeliverySession(fetcher, budget, 3) // settled nothing: the whole face is the remainder
	for e := uint64(3); e <= 3+uint64(PaidSerialWindow); e++ {
		src.e = e
		l.ReleaseDueRefunds()
		if l.Balance(fetcher) != 0 || l.ChargePublish(fetcher) == nil {
			t.Fatalf("epoch %d: the deposit was spendable before the anchor left the window (balance %d)", e, l.Balance(fetcher))
		}
	}
	src.e = 3 + uint64(PaidSerialWindow) + 1
	l.ReleaseDueRefunds()
	if l.Balance(fetcher) != budget {
		t.Fatalf("at A+W+1 the fetcher holds %d, want the whole face %d back", l.Balance(fetcher), budget)
	}
}

// TestGuardOccupancyIsBoundedByTheCreditStock — G-6R-4, THE REFUTATION ENCODED (cert §4.2):
// fresh identities pass bearer anchors down a chain, each opening, closing and re-withdrawing
// inside one window. Live guard entries must never exceed ⌊stock/f⌋. Under release-at-close
// the chain funds itself forever and fills the guard for zero net credits; under release-at-
// anchor-expiry the second identity's deposit is locked and the chain stops at the stock.
func TestGuardOccupancyIsBoundedByTheCreditStock(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	l := New(fee, grant)
	src := &mockEpochSource{}
	l.SetEpochSource(src)
	server := id(1)
	l.Register(server)
	stock := grant // one funded identity is the whole credit stock in this world
	x1 := id(10)
	l.Register(x1) // receives the grant
	serial := 0
	// X1 spends its whole stock on anchors and hands them, bearer, down a chain of fresh
	// identities. Each recipient opens, closes at once, and — if its deposit is spendable —
	// buys a new anchor for the next identity.
	holders := []ports.NodeID{}
	anchors := [][]RelayAnchor{}
	for i := 0; i < int(stock/fee); i++ {
		anchors = append(anchors, buyAnchors(t, l, x1, 0, serial, 1))
		serial++
	}
	next := byte(20)
	for round := 0; round < 200; round++ {
		if len(anchors) == 0 {
			break
		}
		a := anchors[0]
		anchors = anchors[1:]
		xk := id(next)
		next++
		l.Register(xk) // a fresh identity; the unconfigured faucet grants it — model the grant as NOT part of this stock
		l.acct(xk).balance = 0
		face, why := l.SpendDeliveryAnchors(server, a)
		if face != fee {
			t.Fatalf("round %d: open refused: %q", round, why)
		}
		holders = append(holders, xk)
		l.CloseDeliverySession(xk, face, 0) // closes at once: the whole face is the remainder
		if live := int64(len(l.paidSerial)); live > stock/fee {
			t.Fatalf("round %d: %d live guard entries > ⌊stock/f⌋ = %d — a bearer-anchor chain filled the guard beyond the credit stock (release-at-close)", round, live, stock/fee)
		}
		// Can the recipient fund the next link? Only if its deposit was released.
		if err := l.ChargePublish(xk); err == nil {
			anchors = append(anchors, anchorsAt(0, serial, 1))
			serial++
		}
	}
	if live := int64(len(l.paidSerial)); live != stock/fee {
		t.Fatalf("live entries %d, want exactly ⌊stock/f⌋ = %d", live, stock/fee)
	}
	// After the window the deposits release and the stock is whole again — in the holders'
	// hands, not destroyed (T-DEPOSIT: the face bounds concurrency, never capacity).
	src.e = uint64(PaidSerialWindow) + 1
	l.ReleaseDueRefunds()
	var held int64
	for _, h := range holders {
		held += l.Balance(h)
	}
	if held != stock {
		t.Fatalf("after expiry the chain's holders hold %d, want the whole stock %d back", held, stock)
	}
}

// TestZeroSettleSessionReturnsItsWholeFace — G-6R-5 (G-λ-8-10 lift (b), re-worded): a
// session that settles nothing returns its WHOLE face at close or at anchor expiry, whichever
// is later; nothing is burned. A close-only assertion must NOT pass a correct build.
func TestZeroSettleSessionReturnsItsWholeFace(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	l := New(fee, grant)
	src := &mockEpochSource{}
	l.SetEpochSource(src)
	server, fetcher := id(1), id(2)
	l.Register(server)
	l.Register(fetcher)
	budget := openDeliverySession(t, l, server, fetcher, 0, 0, 1)
	l.CloseDeliverySession(fetcher, budget, 0)
	if l.Balance(fetcher) != grant-fee {
		t.Fatal("the face came back AT CLOSE — release-at-close is the refuted rule")
	}
	src.e = uint64(PaidSerialWindow) + 1
	l.ReleaseDueRefunds()
	if l.Balance(fetcher) != grant || l.DeliverySettlementStats().BurnedCredits != 0 {
		t.Fatalf("after expiry: fetcher %d (want the whole grant back), burned %d (want 0)", l.Balance(fetcher), l.DeliverySettlementStats().BurnedCredits)
	}
	// "Whichever is LATER": a session whose anchor already expired at close releases at once.
	src.e = 20
	budget2 := openDeliverySession(t, l, server, fetcher, 20, 50, 1)
	src.e = 20 + uint64(PaidSerialWindow) + 1
	l.CloseDeliverySession(fetcher, budget2, 20)
	if l.Balance(fetcher) != grant {
		t.Fatalf("an already-expired anchor's deposit did not release at close: %d", l.Balance(fetcher))
	}
}

// TestPendingRefundTableIsBoundedAndRefusesNeverEvicts — G-6R-6 (build-immutable #8). The
// pending table is capped at the guard cap; at the cap the remainder is BURNED and counted,
// never a live record dropped.
func TestPendingRefundTableIsBoundedAndRefusesNeverEvicts(t *testing.T) {
	l := New(50_000, 0)
	src := &mockEpochSource{}
	l.SetEpochSource(src)
	server, fetcher := id(1), id(2)
	l.Register(server)
	l.Register(fetcher)
	for i := 0; i < maxPaidSerial; i++ {
		l.CloseDeliverySession(fetcher, 1, 100) // far-future anchors: nothing releases
	}
	if len(l.pendingRefunds) != maxPaidSerial || l.DeliverySettlementStats().PendingRefundCredits != int64(maxPaidSerial) {
		t.Fatalf("pending table %d, want exactly the cap %d", len(l.pendingRefunds), maxPaidSerial)
	}
	l.CloseDeliverySession(fetcher, 7, 100)
	st := l.DeliverySettlementStats()
	if len(l.pendingRefunds) != maxPaidSerial || st.RefundsBurnedAtCap != 1 || st.BurnedCredits != 7 || st.PendingRefundCredits != int64(maxPaidSerial) {
		t.Fatalf("at cap: table %d, %+v — the remainder must be burned and counted, never a live record evicted", len(l.pendingRefunds), st)
	}
	if l.pendingRefunds[0].amount != 1 {
		t.Fatal("the oldest live record was evicted")
	}
}

// TestNoIdentityIsJoinedToAnAnchorSerialInTheDurableStore — G-6R-8 (Don't #3, cert §6.2). The
// guard entry and the durable store's record carry no FETCHER identity; a durable join
// between a durable identity and an anchor serial is the refused shape.
func TestNoIdentityIsJoinedToAnAnchorSerialInTheDurableStore(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(paidSerialEntry{}), reflect.TypeOf(ports.PaidSerial{})} {
		for i := 0; i < typ.NumField(); i++ {
			n := strings.ToLower(typ.Field(i).Name)
			if strings.Contains(n, "fetcher") || strings.Contains(n, "requester") || strings.Contains(n, "buyer") || strings.Contains(n, "payer") {
				t.Fatalf("%s.%s joins a fetcher identity to an anchor serial — retained W+1 epochs on disk, the refused shape", typ.Name(), typ.Field(i).Name)
			}
		}
	}
	if f, ok := reflect.TypeOf(pendingRefund{}).FieldByName("fetcher"); !ok || f.Type != reflect.TypeOf(ports.NodeID{}) {
		t.Fatal("the pending record keys on the durable fetcher — that is the PERMITTED place (process memory, deleted at release)")
	}
}

// TestRestartLosesTheRemainderAndCountsIt — G-6R-9. A restart (a fresh ledger loading the
// durable guard) has no sessions and no pending deposits: the credits are gone, and the
// restored guard entries — anchors with no session state — are counted so the loss is visible.
func TestRestartLosesTheRemainderAndCountsIt(t *testing.T) {
	store := &memPaidStore{}
	l := New(50_000, 500_000)
	l.SetPaidSerialStore(store)
	if err := l.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	server, fetcher := id(1), id(2)
	l.Register(server)
	l.Register(fetcher)
	budget := openDeliverySession(t, l, server, fetcher, 0, 0, 1)
	l.CloseDeliverySession(fetcher, budget, 0) // a pending deposit of one face
	// RESTART: a fresh ledger over the same durable guard.
	l2 := New(50_000, 500_000)
	l2.SetPaidSerialStore(store)
	if err := l2.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	st := l2.DeliverySettlementStats()
	if st.PendingRefundCredits != 0 || st.RestartOrphanedAnchors != 1 {
		t.Fatalf("after restart: pending %d (want 0 — the deposit is gone), orphaned anchors %d (want 1: the guard entry survived with no session)", st.PendingRefundCredits, st.RestartOrphanedAnchors)
	}
	if _, spent := l2.paidSerial[paidKey(0, anchorSerial(0))]; !spent {
		t.Fatal("the anchor's guard entry did not survive the restart — it could be re-spent")
	}
}

// memPaidStore is an in-memory durable guard store for the restart gate.
type memPaidStore struct{ entries []ports.PaidSerial }

func (m *memPaidStore) Load() ([]ports.PaidSerial, error) {
	return append([]ports.PaidSerial(nil), m.entries...), nil
}
func (m *memPaidStore) Append(p ports.PaidSerial) error { m.entries = append(m.entries, p); return nil }
func (m *memPaidStore) Compact(live []ports.PaidSerial) error {
	m.entries = append([]ports.PaidSerial(nil), live...)
	return nil
}

// TestR212AssertionStillBoundsOccupancyUnderTheRelease — G-6R-7 (cert §6.3). At the largest
// admissible faucet capacity (MaxPaidSerial/4 ÷ (g/f) ÷ (W+1) identities per bucket-fill)
// every granted identity spends every face, closes at once and re-spends as fast as the
// loop allows, inside one window. Live guard entries stay ≤ MaxPaidSerial/4 — gated on the
// RUNTIME live count. Under release-at-close every identity re-spends its returned face
// without bound and the assertion goes vacuous (RED).
func TestR212AssertionStillBoundsOccupancyUnderTheRelease(t *testing.T) {
	const fee, grant = int64(50_000), int64(500_000)
	l := New(fee, grant)
	src := &mockEpochSource{}
	l.SetEpochSource(src)
	server := id(1)
	l.Register(server)
	capacity := int64(MaxPaidSerial) / 4 / (grant / fee) / int64(PaidSerialWindow+1) // 327 at the shipped constants
	serial := 0
	ids := make([]ports.NodeID, 0, capacity)
	for i := int64(0); i < capacity; i++ {
		f := ports.HashBytes([]byte{byte(i), byte(i >> 8), 'r', '2', '1', '2'})
		l.Register(f) // the grant lands
		ids = append(ids, f)
	}
	for round := 0; round < 3; round++ { // "as fast as the loop allows": three passes inside the window
		for _, f := range ids {
			for l.ChargePublish(f) == nil { // spend every face the identity can
				face, why := l.SpendDeliveryAnchors(server, anchorsAt(0, serial, 1))
				serial++
				if face != fee {
					t.Fatalf("open refused: %q", why)
				}
				l.CloseDeliverySession(f, face, 0) // close at once
			}
		}
		if live := len(l.paidSerial); live > MaxPaidSerial/4 {
			t.Fatalf("round %d: %d live guard entries > MaxPaidSerial/4 = %d — the R2.12 assertion is vacuous (deposits re-spent inside the window)", round, live, MaxPaidSerial/4)
		}
	}
	if live := int64(len(l.paidSerial)); live != capacity*(grant/fee) {
		t.Fatalf("live entries %d, want exactly capacity × (g/f) = %d — (g/f) is the CONCURRENCY term", live, capacity*(grant/fee))
	}
}
