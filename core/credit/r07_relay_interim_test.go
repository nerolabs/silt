package credit

// R0.7 relay interim — RED-first gate G-RI-1 (Tester, 2026-09-03).
//
// Binding spec: RELAY-LANE-per-node-ledger-mint-FIX-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md
// §9 step 1 ("make RedeemRelayCredit pay 0 unless an anchor is present — no
// anchor type exists yet, so: pay 0, always"), §7 T-1 (the RT-RELAY-1 core
// gate: use the SHIPPED grant, not grant=0, "using the shipped grant is what
// makes this test detect the faucet-funded phantom; a grant=0 variant would
// pass for the wrong reason once R2.12 lands"). Red-team artifact:
// RED-TEAM-relay-lane-session-grant-and-byte-price-2026-09-03.md RT-RELAY-1
// ("Reproduced" section: credit.New(50_000, 500_000), a fresh ephemeral
// fetcher this ledger has never seen, S=MaxChainLength=262,144).
//
// This is a NARROWING (under-pay only per the certification), so it does not
// need its own economic certification — the design doc
// docs/thinking/2026-09-03-r0.7-relay-interim-design.md records it.

import "testing"

// TestRelayRedeemPaysZeroUntilAnchor is G-RI-1: a full paid relay session
// settled through RedeemRelayCredit pays 0 AND moves no balance on the
// relay's ledger — neither the relay's own balance nor the fetcher's
// (a phantom account auto-granted on first touch, since M0 guard (ii)
// mandates a FRESH EPHEMERAL identity per session, so this ledger has never
// seen it before). Drives the exact RT-RELAY-1 mint shape the red-team
// measured: the shipped grant (500,000), a fresh ephemeral fetcher, and a
// full-length session (S = MaxChainLength = 262,144, chainValue == budget).
//
// TODAY (main, no anchor type exists anywhere in the wire/ledger path):
// relay != fetcher, chainValue > 0, chainValue <= budget, so
// RedeemRelayCredit's three guards all pass and it mints chainValue into the
// relay's balance by auto-registering the fresh ephemeral (500,000 grant)
// and debiting it. RED.
//
// Ablation (both directions must hold once the anchor-check lands):
//   - remove the anchor check entirely -> this test reddens (mints again).
//   - assert paid==0 unconditionally without ALSO asserting no mutation
//     -> would pass a fix that pays 0 but still conjures/debits the phantom,
//     which the design doc calls out as "the same fiction" as the mint
//     itself. Both halves are asserted below for that reason.
func TestRelayRedeemPaysZeroUntilAnchor(t *testing.T) {
	const grant = 500_000 // cmd/silt/daemon.go:622 shipped grant, deliberately not 0
	const fee = 50_000
	l := New(fee, grant)
	relay := id(1)
	freshEphemeral := id(2) // never touched this ledger before — M0 guard (ii)

	// Baseline the relay's balance via the SAME accessor the settlement path
	// itself uses (Balance -> acct -> Register-on-first-touch), so the
	// baseline already includes the relay's own faucet grant.
	relayBefore := l.Balance(relay)

	const chainValue = 262_144 // red-team's measured full-session mint (S=S_max)
	const budget = 262_144     // S == MaxChainLength; chainValue == budget (inclusive cap)

	paid := l.RedeemRelayCredit(relay, freshEphemeral, chainValue, budget)
	if paid != 0 {
		t.Fatalf("RedeemRelayCredit paid %d for a session with no anchor (none exists yet — R2.14), want 0 — the RT-RELAY-1 per-session mint on the relay's own ledger", paid)
	}
	if got := l.Balance(relay); got != relayBefore {
		t.Fatalf("relay balance moved %d -> %d on an unanchored (pay-0) settlement — the interim must move nothing", relayBefore, got)
	}
	// The fetcher's fresh ephemeral must never even be touched: debiting a
	// phantom auto-granted balance is "the same fiction" as the mint
	// (design doc §2 step 1). White-box: inspect the account map directly so
	// this test's OWN accessor call cannot mask a real production Register().
	if _, ok := l.accounts[freshEphemeral]; ok {
		t.Fatalf("RedeemRelayCredit registered/touched the fresh ephemeral fetcher's account on an unanchored (pay-0) settlement — a phantom balance was conjured even though nothing was paid (the RT-RELAY-1 shape, half-fixed)")
	}
}

// TestRelayRedeemPaysZeroEvenWhenFetcherIsFunded is the companion case: even
// when the fetcher DOES carry a real balance on this same ledger (the
// oracle-blind-spot shape relay_test.go's `fund()` helper models), the
// interim still pays 0 and moves nothing — the rule is "no anchor type
// exists, so always pay 0," not "pay 0 only for phantom fetchers." A fix that
// special-cased "only refuse if the fetcher balance was auto-granted" would
// pass G-RI-1 above but fail here.
func TestRelayRedeemPaysZeroEvenWhenFetcherIsFunded(t *testing.T) {
	const chainValue = 30_000
	l := New(50_000, 0)
	relay, fetcher := id(3), id(4)
	fund(l, fetcher, chainValue) // relay_test.go's white-box helper

	relayBefore := l.Balance(relay)
	fetcherBefore := l.Balance(fetcher)

	paid := l.RedeemRelayCredit(relay, fetcher, chainValue, chainValue)
	if paid != 0 {
		t.Fatalf("RedeemRelayCredit paid %d against a funded fetcher with no anchor, want 0 — the interim pays 0 unconditionally until R2.14, not conditionally on fetcher solvency", paid)
	}
	if got := l.Balance(relay); got != relayBefore {
		t.Fatalf("relay balance moved %d -> %d on an unanchored settlement against a funded fetcher", relayBefore, got)
	}
	if got := l.Balance(fetcher); got != fetcherBefore {
		t.Fatalf("fetcher balance moved %d -> %d on an unanchored (pay-0) settlement — nothing may be drawn without an anchor", fetcherBefore, got)
	}
}
