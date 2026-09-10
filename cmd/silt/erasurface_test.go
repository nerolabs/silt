package main

import (
	"crypto/ed25519"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/chainstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// THE ERA OBSERVABLE, cmd tier (R-CLOUD-ERA-PROBE, freeze manifest item 19).
//
// WHAT THIS TIER TESTS AND WHAT IT DOES NOT. These are RENDERER gates. Their fixtures write blocks
// directly, which does NOT establish that a real chain produces v5 blocks, a real carrier or a real
// readiness latch — that is established one tier down, on a chain that produces all three itself
// (TestEraStateDrivesEveryPhaseFromCommittedState, TestCensusMeasuresMaxAttsOnANonUniformChain).
// The seam between the two tiers is one function, chain.CensusOf, exercised on both sides.
//
// The split is forced, not chosen: chain.NewBondReg hard-codes BlockVersionRegGate, because the
// readiness stamp is a property of the binary rather than a caller's choice, so NO cmd-tier fixture
// can mint a reg stamped 5 and therefore none can drive a real era-4 latch. Said here so the
// boundary is a decision on the record rather than an accident.

// eraStore writes a chain.cbor holding the given blocks and returns its directory.
func eraStore(t *testing.T, blocks []chain.Block) string {
	t.Helper()
	dir := t.TempDir()
	if err := chainstore.Save(filepath.Join(dir, "chain.cbor"), blocks); err != nil {
		t.Fatal(err)
	}
	return dir
}

// eraChainStatus runs `silt chain-status` against a store and returns its whole stdout.
func eraChainStatus(t *testing.T, dir string) string {
	t.Helper()
	return captureStdout(t, func() {
		if err := cmdChainStatus([]string{"-store", dir}); err != nil {
			t.Fatal(err)
		}
	})
}

// TestChainStatusDistinguishesAnEra4ChainFromADarkOne is GATE G-EP-5 and GATE G-EP-6 (cmd tier).
// It is the gate the whole row exists for.
//
// The evidence that this observable was missing: on the tree before this change, `silt chain-status`
// against a store holding a v4 block, two v5 blocks and a 3-wide attestation carrier printed
// head height, head hash, block count, entry count and pruned count — and NOTHING ELSE. That output
// is byte-identical to the output for an all-v2 chain carrying no attestation at all. A Tester's
// verdict had to end "I verified the in-repo configuration, not the live nets' actual chain.cbor",
// and cloud row 13b-delivery-settlement SKIPped with one sentence covering both "era-4 is dark" and
// "the issuer's keys are off-commitment" because nothing separated them.
//
// BOTH ARMS, and the assertion is that they DIFFER. A surface where the two render alike answers
// nothing, which is exactly how this row's predecessor came to be vacuous.
//
// The blocks travel through chainstore.Save and chainstore.Load, so the persisted round trip is
// real: that is the layer the blocked investigation actually hit — a live network's chain.cbor.
func TestChainStatusDistinguishesAnEra4ChainFromADarkOne(t *testing.T) {
	pub := []byte(ed25519.NewKeyFromSeed(make([]byte, 32)).Public().(ed25519.PublicKey))
	att := chain.Attestation{PubKey: pub}
	g := chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{{Root: ports.HashBytes([]byte("g"))}}}

	// ARM A — an era-4 chain: a v4 block, then two v5 blocks, with a NON-UNIFORM carrier.
	a1 := chain.Block{Version: chain.BlockVersionStateRoot, Height: 1, Prev: g.Hash(), Atts: []chain.Attestation{att}}
	a2 := chain.Block{Version: chain.BlockVersionWitnessable, Height: 2, Prev: a1.Hash(),
		Atts: []chain.Attestation{att, att, att}}
	a3 := chain.Block{Version: chain.BlockVersionWitnessable, Height: 3, Prev: a2.Hash(), Atts: []chain.Attestation{att, att}}
	lit := eraChainStatus(t, eraStore(t, []chain.Block{g, a1, a2, a3}))

	// ARM B — a dark chain: same shape, same height, no v4 and no v5 anywhere.
	b1 := chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Atts: []chain.Attestation{att}}
	b2 := chain.Block{Version: 1, Height: 2, Prev: b1.Hash(), Atts: []chain.Attestation{att, att, att}}
	b3 := chain.Block{Version: 1, Height: 3, Prev: b2.Hash(), Atts: []chain.Attestation{att, att}}
	dark := eraChainStatus(t, eraStore(t, []chain.Block{g, b1, b2, b3}))

	t.Logf("ARM A (era-4 chain):\n%s\nARM B (dark chain):\n%s", lit, dark)

	if lit == dark {
		t.Fatal("G-EP-5 RED: an era-4 chain and a DARK chain of the same height render IDENTICALLY. " +
			"That is the state cloud row 13b could not name, reproduced.")
	}
	for _, want := range []string{
		"head version: v5",
		"era-4 (v5): ACTIVE — first v5 block at height 2",
		"era-3 (v4): ACTIVE — first v4 block at height 1",
		"max atts:     3, first at height 2",
	} {
		if !strings.Contains(lit, want) {
			t.Errorf("G-EP-5 RED: the era-4 arm must report %q; whole output:\n%s", want, lit)
		}
	}
	// The dark arm must NAME its state and must NOT claim to know the tally, which no offline
	// caller can see. A "lockedIn: false" here would be a lie dressed as a zero.
	if !strings.Contains(dark, "era-4 (v5): NOT ON THIS CHAIN") {
		t.Errorf("G-EP-5 RED: the dark arm must NAME the state, not print an empty field; whole output:\n%s", dark)
	}
	if !strings.Contains(dark, "/api/status .chain.era") {
		t.Errorf("G-EP-5 RED: the dark arm must say where the tally IS observable; whole output:\n%s", dark)
	}
	if strings.Contains(dark, "lockedIn") || strings.Contains(dark, "DARK") {
		t.Errorf("G-EP-5 RED: the offline path asserted a tally fact it cannot observe; whole output:\n%s", dark)
	}
}

// TestChainStatusNarratesAMeasuredZeroCarrier is GATE G-EP-5's zero arm.
//
// max_h len(blocks[h].Atts) is legitimately 0 on a genesis-only or single-signer chain. A bare "0"
// would be unreadable against "the field was never computed" — the precise shape of the defect this
// row absorbs. The zero is therefore narrated, and this pins that it is.
func TestChainStatusNarratesAMeasuredZeroCarrier(t *testing.T) {
	g := chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{{Root: ports.HashBytes([]byte("g"))}}}
	out := eraChainStatus(t, eraStore(t, []chain.Block{g}))
	t.Logf("genesis-only chain:\n%s", out)
	if !strings.Contains(out, "max atts:     0 — measured across every block") {
		t.Fatalf("G-EP-5 RED: a measured zero must say it was measured, not print a bare 0:\n%s", out)
	}
}

// TestStatusRouteCarriesTheEraState is GATE G-EP-7 (cmd tier). The programmatic half.
//
// Before this change GET /api/status `.chain` was, in full, {"height":1,"entries":1}. The cloud
// sheet's 13b row reads `.chain.era.era4.phase`; a consumer that cannot see the era has to infer it
// from a client error message, which is what made "era-4 dark" and "keys off-commitment" one
// sentence.
//
// It also pins the ABSENT-versus-ZERO wire contract, which is the anti-vacuity mechanism: the
// optional heights are *uint64 with omitempty, so an era that has not activated carries NO
// firstHeight KEY rather than a firstHeight of 0. Height 0 is a legal height — a genesis block can
// itself be v5 — so a present zero could not have been read as an absence.
func TestStatusRouteCarriesTheEraState(t *testing.T) {
	s, _ := statusServer(t)
	ch := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 0 })
	gk := ed25519.NewKeyFromSeed(make([]byte, 32))
	g := chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{{Root: ports.HashBytes([]byte("g"))}}}
	chain.Sign(&g, gk)
	if err := ch.AppendGenesis(g); err != nil {
		t.Fatal(err)
	}
	s.nd.EnableChain(ch, gk)

	raw := statusKey(t, statusAt(t, s, time.Unix(1_757_000_000, 0), true), "chain")
	t.Logf("GET /api/status .chain = %s", string(raw))

	var doc struct {
		HeadVersion uint64 `json:"headVersion"`
		Era         struct {
			Census map[string]json.RawMessage `json:"census"`
			Era4   map[string]json.RawMessage `json:"era4"`
			Era3   map[string]json.RawMessage `json:"era3"`
		} `json:"era"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode .chain: %v (%s)", err, raw)
	}
	if doc.Era.Era4 == nil || doc.Era.Era3 == nil || doc.Era.Census == nil {
		t.Fatal("G-EP-7 RED: .chain.era is missing an era or the census")
	}
	var phase chain.EraPhase
	if err := json.Unmarshal(doc.Era.Era4["phase"], &phase); err != nil {
		t.Fatalf("G-EP-7 RED: .chain.era.era4.phase is not readable: %v", err)
	}
	if phase != chain.EraDark {
		t.Fatalf("G-EP-7 RED: a v1 genesis-only chain must report era-4 %q, got %q", chain.EraDark, phase)
	}
	if phase == "" {
		t.Fatal("G-EP-7 RED: phase is the empty string — a never-populated field is indistinguishable from it")
	}
	// The wire contract: an era that has not activated carries NO height keys at all.
	for _, k := range []string{"firstHeight", "activationHeight"} {
		if _, present := doc.Era.Era4[k]; present {
			t.Errorf("G-EP-7 RED: era-4 has not activated, so %q must be ABSENT from the wire, not present as 0. "+
				"Height 0 is a legal height, so a zero there could not be read as an absence.", k)
		}
	}
	// attsMeasured is what separates a measured zero from a figure nobody computed.
	var measured bool
	if err := json.Unmarshal(doc.Era.Census["attsMeasured"], &measured); err != nil || !measured {
		t.Errorf("G-EP-7 RED: census.attsMeasured must be true on a walked chain (err %v, value %v)", err, measured)
	}
	if doc.HeadVersion != 1 {
		t.Errorf("G-EP-7 RED: headVersion must be the head block's Version (1), got %d", doc.HeadVersion)
	}
}
