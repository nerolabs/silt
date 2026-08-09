package main

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// M0 hardening H3 — the Invariant-B structural guardrail.
//
// Invariant B (docs/design/m0-hardening-strategy.md §2): a security mechanism is
// not "shipped" until its SAFE configuration is the DEFAULT for the untrusted
// (open-M0) posture. Enforcement: for EVERY security mechanism, a test that
// builds the DEFAULT config an untrusted validator gets and asserts that default
// DENIES the attack.
//
// This is the guardrail for meta-pattern #2 — "fixed but off by default." F6
// (objective mode) → F4 (credits) → G4-residual (floor) → RT-2 (bond-TTL) were
// four re-instances of ONE class: a mechanism shipped correct but inert, so a
// doc-following open validator stayed exploitable until a red team noticed. This
// file enumerates the standing surfaces (§4 S1–S3) and asserts each one's SAFE
// value is what an operator who passes no flags actually gets.
//
// Coverage map (§4):
//   S1 anti-release floor  → enforced-by-default, asserted here (via effectiveBondFloor)
//   S2 PoR-audit standing  → denied STRUCTURALLY (grants no standing at all — Invariant A,
//                            core/credit/invariant_a_test.go); no default to flip, nothing to test here
//   S3 bond-TTL (RT-2)     → enforced-by-default (H2), asserted here (via effectiveBondTTL);
//                            the release-and-coast prune behavior itself is proven over the
//                            wire in sim TestObjectiveBondRenewalSustainsAttestOnlyValidator
//   S4 Byzantine quorum    → enforced-by-default (H4), asserted here (via effectiveByzantineQuorum);
//                            the quorum-intersection safety itself is proven in
//                            core/chain TestBFTQuorumIntersectionAboveFaultBound
//   S5 client eclipse cap  → enforced-by-default in the SHIPPED CLIENT (BREAK 2), asserted here
//                            (via clientNodeConfig): the desktop client resolves/announces
//                            provider records over a domain-spread, signed set so a ~$4 /24
//                            key-surround can't censor a root at the routing layer for a user
//                            who opted into no takedown. The daemon and swarm fetcher already
//                            defaulted these on; the client path had shipped them off.
//   S6 cold-start scaffold → REFUSE-BY-DEFAULT (seam-2), asserted here (via coldStartScaffoldOK):
//                            an untrusted objective validator with no anchor launch set and no
//                            weak-subjectivity checkpoint would latch everMature at genesis and
//                            run with no anchor co-sign — a young/Sybil quorum could self-certify
//                            mature and capture. No sound synthesizable anchor set exists
//                            (weak-subjectivity irreducibility), so the safe default is to refuse
//                            to start (like -min-bond<=0), not warn.
//   S7 publish signer-set → CANONICAL-BY-DEFAULT (seam-4/R-3), asserted here (via rankByCanonical):
//                            `swarm add -token-quorum` picks its publish-token signers by a
//                            network-canonical ledger ordering (heaviest bond first), the SAME for
//                            every publisher, not an arbitrary subset of -peers — so the signer
//                            subset can't collapse the publisher anonymity set. Deterministic:
//                            input peer order does not change the selection.

// TestInvariantB_S1_AntiReleaseFloorOnByDefault asserts the untrusted-validator
// DEFAULT (no -min-bond-floor passed, objective path) imposes the anti-release
// floor, so a sub-floor releasable plot is denied objective standing without the
// operator having to know the flag exists. (Mechanism internals — that the
// derived floor exceeds a challenge-window re-plot — are covered in
// bondfloor_default_test.go; this row is the Invariant-B enumeration's S1 entry.)
func TestInvariantB_S1_AntiReleaseFloorOnByDefault(t *testing.T) {
	floor, defaulted := effectiveBondFloor(false /*floorSet*/, 0 /*explicit*/, true /*objectivePath*/)
	if !defaulted || floor != DerivedBondFloor {
		t.Fatalf("Invariant B (S1) violated: the untrusted default must impose the anti-release floor, "+
			"got floor=%d defaulted=%v want %d/true — a mechanism shipped off-by-default is not shipped", floor, defaulted, DerivedBondFloor)
	}
	// The default must deny the daemon's own stock 64M bond — the exact posture the
	// red team's release-and-replot PoC exercised.
	if int64(64)<<20 >= floor {
		t.Fatal("Invariant B (S1): the default floor must deny the stock 64M bond on an untrusted swarm")
	}
}

// TestInvariantB_S3_BondTTLPrunesReleasedBondByDefault closes the RT-2 gap — the
// live "release-and-coast" break the blind red team found over our own G2 merge:
// a validator registers a bond once, releases the plot, and votes forever off one
// proof. The decay mechanism (chain.Config.BondTTLBlocks) existed but shipped OFF
// by default. H2 made it SAFE-BY-DEFAULT on the objective path, unblocked by the
// non-proposer renewal path (node.SubmitBondRenewal) that lets an attest-only
// validator renew without proposing — so the default costs no liveness (§3 trap).
//
// This asserts the DEFAULT WIRING (Invariant B). The end-to-end prune-vs-sustain
// behavior over the wire is proven in sim
// TestObjectiveBondRenewalSustainsAttestOnlyValidator.
func TestInvariantB_S3_BondTTLPrunesReleasedBondByDefault(t *testing.T) {
	ttl, defaulted := effectiveBondTTL(false /*ttlSet*/, 0 /*explicit*/, true /*objectivePath*/)
	if !defaulted || ttl == 0 {
		t.Fatalf("Invariant B (S3) violated: the untrusted default must turn the bond TTL ON, "+
			"got ttl=%d defaulted=%v — release-and-coast stays live while the mechanism sits off-by-default", ttl, defaulted)
	}
	if ttl != DerivedBondTTL {
		t.Fatalf("Invariant B (S3): the default TTL must be the derived value %d, got %d", DerivedBondTTL, ttl)
	}
}

// TestBondTTLDefaultsOnForUntrustedValidator pins the effectiveBondTTL contract,
// mirroring the anti-release floor's TestAntiReleaseFloorDefaultsOnForUntrustedValidator:
// on-by-default for the untrusted objective posture, an explicit choice (including
// the 0 opt-out) always wins, and a trusted/non-objective node is unaffected.
func TestBondTTLDefaultsOnForUntrustedValidator(t *testing.T) {
	// Stock untrusted validator (no -bond-ttl): the TTL is derived, not left off.
	if got, defaulted := effectiveBondTTL(false, 0, true); !defaulted || got != DerivedBondTTL {
		t.Fatalf("an untrusted validator that sets no TTL must GET one: got %d (defaulted=%v), want %d", got, defaulted, DerivedBondTTL)
	}
	// An explicit -bond-ttl 0 is the trusted/demo opt-out and must win.
	if got, defaulted := effectiveBondTTL(true, 0, true); got != 0 || defaulted {
		t.Fatalf("an explicit -bond-ttl 0 must opt out: got %d (defaulted=%v)", got, defaulted)
	}
	// An explicit non-zero TTL is honored verbatim.
	if got, _ := effectiveBondTTL(true, 7, true); got != 7 {
		t.Fatalf("an explicit TTL must be honored verbatim: got %d", got)
	}
	// A trusted/non-objective node gets no imposed TTL, so demos keep working.
	if got, defaulted := effectiveBondTTL(false, 0, false); got != 0 || defaulted {
		t.Fatalf("a trusted/non-objective node must get no imposed TTL: got %d (defaulted=%v)", got, defaulted)
	}
}

// TestInvariantB_S4_ByzantineQuorumOnByDefault asserts the untrusted-validator
// DEFAULT sizes the commit quorum at the Byzantine threshold, so a fixed quorum
// can't silently lose quorum-intersection safety as the set grows — without the
// operator having to know the flag exists.
func TestInvariantB_S4_ByzantineQuorumOnByDefault(t *testing.T) {
	on, defaulted := effectiveByzantineQuorum(false /*byzSet*/, false /*explicit*/, true /*objectivePath*/)
	if !on || !defaulted {
		t.Fatalf("Invariant B (S4) violated: an untrusted objective validator must size the quorum at the Byzantine threshold by default, got on=%v defaulted=%v", on, defaulted)
	}
}

// TestByzantineQuorumDefaultsOnForUntrustedValidator pins the effectiveByzantineQuorum
// contract: on-by-default for the untrusted objective posture, an explicit choice
// (including the =false opt-out) always wins, and a trusted/non-objective node is off.
func TestByzantineQuorumDefaultsOnForUntrustedValidator(t *testing.T) {
	if on, defaulted := effectiveByzantineQuorum(false, false, true); !on || !defaulted {
		t.Fatalf("untrusted objective validator must default ON: got on=%v defaulted=%v", on, defaulted)
	}
	if on, defaulted := effectiveByzantineQuorum(true, false, true); on || defaulted {
		t.Fatalf("an explicit -byzantine-quorum=false must opt out: got on=%v defaulted=%v", on, defaulted)
	}
	if on, _ := effectiveByzantineQuorum(true, true, false); !on {
		t.Fatalf("an explicit -byzantine-quorum=true must be honored even off the objective path")
	}
	if on, defaulted := effectiveByzantineQuorum(false, false, false); on || defaulted {
		t.Fatalf("a trusted/non-objective node must get no imposed Byzantine sizing: got on=%v defaulted=%v", on, defaulted)
	}
}

// TestInvariantB_S5_ClientEclipseDefenseOnByDefault asserts the SHIPPED desktop
// client (clientNodeConfig) defaults the H5-B eclipse-resistance defenses ON — the
// DHT failure-domain cap and signed provider records — so a routing-layer censor
// (a ~$4 /24 key-surround) cannot make a root undiscoverable for a user who opted
// into no takedown. This is the red-team BREAK 2 (2026-08-08) surface: the daemon
// and swarm fetcher already defaulted these on; the client had shipped them off,
// re-instancing the "fixed but off by default" meta-pattern this file guards.
func TestInvariantB_S5_ClientEclipseDefenseOnByDefault(t *testing.T) {
	cfg := clientNodeConfig()
	if cfg.DHTDomainCap <= 0 {
		t.Fatalf("Invariant B (S5/BREAK 2) violated: the shipped client must default the DHT failure-domain cap ON for eclipse resistance, got DHTDomainCap=%d", cfg.DHTDomainCap)
	}
	if !cfg.RequireSignedProviders {
		t.Fatal("Invariant B (S5/BREAK 2) violated: the shipped client must reject forged/unsigned provider records by default")
	}
}

// TestInvariantB_S6_ColdStartScaffoldRefusedByDefault asserts that an untrusted
// objective validator (the M0 default path) with NO cold-start scaffolding is
// REFUSED, not run — closing the red-team seam-2 (2026-08-08) hole where a stock
// validator latches everMature at genesis and imposes no anchor co-sign, letting a
// young or Sybil quorum self-certify mature and capture. Either the anchor launch
// set (anchors + mature-validators>0) or a weak-subjectivity checkpoint satisfies
// it; off the untrusted objective path there is nothing to gate. Refuse-to-start is
// FORCED by weak-subjectivity irreducibility — there is no sound synthesizable
// anchor set — so it is the correct safe default, not merely prudent.
func TestInvariantB_S6_ColdStartScaffoldRefusedByDefault(t *testing.T) {
	// Stock untrusted objective defaults: no anchors, mature-validators=0, no checkpoint → REFUSE.
	if coldStartScaffoldOK(true /*useObjective*/, 0 /*anchors*/, 0 /*matureValidators*/, "" /*wsCheckpoint*/) {
		t.Fatal("Invariant B (S6/seam-2) violated: an untrusted objective validator with no cold-start scaffolding must be refused (it would latch everMature at genesis with no anchor gate)")
	}
	// Anchor launch set (anchors + mature-validators>0) satisfies it.
	if !coldStartScaffoldOK(true, 2, 2, "") {
		t.Fatal("an anchor launch set (anchors + mature-validators>0) must satisfy cold-start scaffolding")
	}
	// Partial launch set (anchors but mature-validators=0) is NOT enough — the gate never engages.
	if coldStartScaffoldOK(true, 2, 0, "") {
		t.Fatal("anchors without mature-validators>0 must NOT satisfy cold-start (the anchor gate never engages)")
	}
	// A weak-subjectivity checkpoint (the join path) satisfies it.
	if !coldStartScaffoldOK(true, 0, 0, "100:deadbeef") {
		t.Fatal("a weak-subjectivity checkpoint must satisfy cold-start (safely joining a matured network)")
	}
	// Off the untrusted objective path (trusted/legacy) there is nothing to gate.
	if !coldStartScaffoldOK(false, 0, 0, "") {
		t.Fatal("a trusted/non-objective node must not be gated on cold-start scaffolding")
	}
}

// TestInvariantB_S7_PublishSignerSetIsCanonical asserts the publish-token signer
// selection is CANONICAL-by-default (seam-4 / R-3): `swarm add -token-quorum` ranks
// its signers by the network-canonical ledger ordering (heaviest bond first), the
// SAME for every publisher, so the signer subset stops being a per-publisher
// quasi-identifier. The key property is determinism: the same reachable set +
// canonical ordering yields the same selection regardless of the publisher's own
// -peers order — so two publishers can't be told apart by their subset choice.
func TestInvariantB_S7_PublishSignerSetIsCanonical(t *testing.T) {
	id := func(b byte) ports.NodeID { var h ports.NodeID; h[0] = b; return h }
	a, bb, c, d := id(1), id(2), id(3), id(4)
	canon := []ports.NodeID{c, a, d, bb} // ledger ordering (heaviest bond first)

	// Two publishers hold the same reachable validators in DIFFERENT orders.
	pub1 := rankByCanonical([]ports.NodeID{a, bb, c, d}, canon)
	pub2 := rankByCanonical([]ports.NodeID{d, c, bb, a}, canon)
	if len(pub1) != len(pub2) {
		t.Fatalf("length mismatch: %d vs %d", len(pub1), len(pub2))
	}
	for i := range pub1 {
		if pub1[i] != pub2[i] {
			t.Fatalf("Invariant B (S7) violated: signer selection must be canonical (order-independent); pub1=%v pub2=%v", pub1, pub2)
		}
	}
	// And it MUST be the canonical order, not the caller's -peers order.
	for i := range canon {
		if pub1[i] != canon[i] {
			t.Fatalf("Invariant B (S7): the signer set must follow the canonical ledger ordering, got %v want %v", pub1, canon)
		}
	}
	// A validator not reachable by the publisher is dropped (can't sign for a peer
	// you can't dial), the rest stay in canonical order.
	got := rankByCanonical([]ports.NodeID{a, c}, canon) // only a, c reachable
	if len(got) != 2 || got[0] != c || got[1] != a {
		t.Fatalf("unreachable canonical entries must be skipped, keeping canonical order: got %v", got)
	}
}
