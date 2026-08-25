package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #555 renewal phase-jitter (research certification 2026-08-25): validators that
// all registered near genesis used to hit the TTL/2 renewal point together, so
// 5–7 heavy ~1.5 MB space-time proofs landed in one block — inflating the
// two-phase gather latency on the 1 vCPU box (the deep-drive crawl). BondRenewalDue
// now spreads each identity's renewal across the [TTL/4, 3·TTL/4) window by a
// deterministic per-identity offset, so ~1 reg lands per block, while keeping the
// renewal margin to expiry ≥ TTL/4 (a dropped renewal cannot lapse standing).

// TestRenewalPhaseOffset_SpreadDeterministicBounded pins the offset primitive:
// deterministic, in [0, window), and well-distributed across identities.
func TestRenewalPhaseOffset_SpreadDeterministicBounded(t *testing.T) {
	const window = uint64(16) // TTL/2 for the field's TTL=32
	seen := map[uint64]int{}
	for i := int64(0); i < 64; i++ {
		id := idOf(key(70000 + i))
		off := renewalPhaseOffset(id, window)
		if off >= window {
			t.Fatalf("offset %d out of [0,%d) for id %d", off, window, i)
		}
		if off2 := renewalPhaseOffset(id, window); off2 != off {
			t.Fatalf("offset not deterministic for id %d: %d vs %d", i, off, off2)
		}
		seen[off]++
	}
	// 64 identities over a 16-slot window should touch most slots — a distribution,
	// not a constant (the clustered bug would map everyone to one slot).
	if len(seen) < int(window/2) {
		t.Fatalf("offset poorly distributed: only %d of %d slots used across 64 ids", len(seen), window)
	}
	if renewalPhaseOffset(idOf(key(1)), 0) != 0 {
		t.Fatal("window 0 must yield offset 0 (small-TTL fallback)")
	}
}

// TestBondRenewalDue_JitterSpreadsAndKeepsMargin drives a real objective chain:
// 12 validators bonded at genesis (the field topology), TTL=32. It confirms the
// renewal-due heights are SPREAD across [TTL/4, 3·TTL/4) rather than clustered at
// TTL/2, and that every renewal point keeps a ≥ TTL/4 margin to expiry.
func TestBondRenewalDue_JitterSpreadsAndKeepsMargin(t *testing.T) {
	const ttl = uint64(32)
	const bond = int64(64) << 20
	k := make([]ed25519.PrivateKey, 12)
	for i := range k {
		k[i] = key(int64(71000 + i))
	}
	cfg := Config{Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 8, BondTTLBlocks: ttl}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for i, kk := range k {
		g.BondRegs = append(g.BondRegs, bondRegDom(kk, bond, ports.Hash{}, uint64(i+1)))
	}
	Sign(g, k[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	// All 12 registered at genesis (bondRegHeight 0). Find the first height each
	// becomes renewal-due by scanning WITHOUT advancing the chain: BondRenewalDue
	// reads c.Head()'s next, so drive Head by appending plain attested blocks and
	// record, per height, how many validators are newly due.
	dueAt := make(map[ports.NodeID]uint64)
	perHeight := map[uint64]int{}
	prev, _ := c.Head()
	for h := uint64(1); h < ttl; h++ {
		// record due-state at the height about to be proposed (next == h)
		for _, kk := range k {
			id := idOf(kk)
			if _, done := dueAt[id]; !done && c.BondRenewalDue(id) {
				dueAt[id] = h
				perHeight[h]++
			}
		}
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		Sign(b, k[0])
		for _, kk := range k[1:] { // all 11 non-proposers → clears the >⅔ frozen-weight quorum
			b.Atts = append(b.Atts, Attest(b, kk))
		}
		if err := c.Append(*b); err != nil {
			t.Fatalf("append h%d: %v", h, err)
		}
		prev = b.Hash()
	}

	// Every validator must have become due within the certified window, and the
	// margin to expiry (bondRegHeight 0 + TTL) must be ≥ TTL/4.
	if len(dueAt) != 12 {
		t.Fatalf("not all validators became due within TTL: %d of 12", len(dueAt))
	}
	distinct := map[uint64]bool{}
	for id, h := range dueAt {
		if h < ttl/4 || h >= 3*ttl/4 {
			t.Fatalf("renewal for %s at h%d outside the [%d,%d) jitter window", id, h, ttl/4, 3*ttl/4)
		}
		if margin := ttl - h; margin < ttl/4 {
			t.Fatalf("renewal for %s at h%d leaves margin %d < TTL/4 %d", id, h, margin, ttl/4)
		}
		distinct[h] = true
	}
	// THE FIX: renewals are SPREAD, not clustered. Pre-fix, all 12 fired at
	// TTL/2 (one height); now they occupy several distinct heights so ~1 reg
	// lands per block. Require a real spread (≥ 4 distinct heights, and no single
	// height holding more than half the cohort).
	if len(distinct) < 4 {
		t.Fatalf("renewals not spread: 12 validators became due across only %d distinct heights (clustered)", len(distinct))
	}
	maxPerHeight := 0
	for _, n := range perHeight {
		if n > maxPerHeight {
			maxPerHeight = n
		}
	}
	if maxPerHeight > 6 {
		t.Fatalf("renewals still clustered: %d validators due at one height (want the load spread so ~1 reg/block)", maxPerHeight)
	}
}

// #562 — the jitter grid vs the #506 R-rule (a434494-deep field finding): the
// nearest grid point can sit closer to the last committed reg than the
// reg-inclusion rate bound allows (rounding reaches down to TTL/4 = 8; R =
// K+2 = 10 at the field shape), so the renewal submitted there was REFUSED
// every sweep — "re-registering 9 blocks after its last reg (R=10)" — until
// the chain outran R. The due point must clear regMinInterval for EVERY
// identity phase; steady-state (on-grid) periods stay exactly TTL/2 (the
// #313/#556 property — the clamp must be inert there, engaging only on
// colliding first cycles like the genesis-aligned fleet's).
func TestRenewalDueClearsRegRateBound_562(t *testing.T) {
	const ttl = uint64(32)
	cfg := Config{Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 8, BondTTLBlocks: ttl}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	period := ttl / 2
	minDist := c.regMinInterval() // K+2 = 10 > TTL/4 = 8: the collision exists
	if minDist <= ttl/4 {
		t.Fatalf("premise: R=%d does not exceed TTL/4=%d — the collision this guards cannot occur at this shape", minDist, ttl/4)
	}

	const regH = uint64(100)
	collided := false
	for seed := int64(0); seed < 512; seed++ {
		id := idOf(key(90000 + seed))
		c.bondRegHeight[id] = regH
		phase := renewalPhaseOffset(id, period)
		due := c.renewalDueHeight(id)
		if dist := due - regH; dist < minDist {
			t.Fatalf("#562 REPRODUCED: identity with phase %d has renewal due at +%d < R=%d — its renewal submit is refused every sweep until the chain outruns the rate bound", phase, dist, minDist)
		}
		// The un-clamped grid point for phases 12/13 lands at +8/+9 (< R):
		// those are the identities the clamp must move, and the sample must
		// actually contain them or this test is vacuous.
		if phase == 12 || phase == 13 {
			collided = true
			if due-regH != minDist {
				t.Fatalf("colliding phase %d clamped to +%d, want exactly R=%d (the minimal legal point)", phase, due-regH, minDist)
			}
		}
		// Margin to expiry survives the clamp: due stays inside the certified
		// jitter window (≤ 3·TTL/4, so ≥ TTL/4 margin) — the clamp never pushes
		// past it (it only ever RAISES a too-low point to R < TTL/2).
		if due > regH+3*ttl/4 {
			t.Fatalf("phase %d due at +%d — outside the [TTL/4, 3·TTL/4] jitter window, margin eroded", phase, due-regH)
		}
	}
	if !collided {
		t.Fatal("vacuous: no identity in the sample carries a colliding phase (12/13)")
	}

	// Steady state: a reg ON its own grid point renews exactly one period later
	// — the clamp is inert (R < TTL/2), the #556 period property intact.
	for seed := int64(0); seed < 64; seed++ {
		id := idOf(key(91000 + seed))
		phase := renewalPhaseOffset(id, period)
		onGrid := ((regH-phase)/period+1)*period + phase // smallest grid point ≥ regH... next aligned height
		c.bondRegHeight[id] = onGrid
		if due := c.renewalDueHeight(id); due != onGrid+period {
			t.Fatalf("on-grid reg at %d (phase %d): next due %d, want exactly one period later %d (the #556 steady-state property)", onGrid, phase, due, onGrid+period)
		}
	}
}
