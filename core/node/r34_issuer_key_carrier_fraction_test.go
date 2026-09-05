package node

// R3.4 / #657 input — the IssuerKeys-CARRIER block fraction, measured (PE code ruling on R2.11,
// F6: the unit fixture's "1 of 5" was a demonstration, not a rate). The floor box marks any
// block carrying a demand-issuer key registration Indeterminate (stateRootScopeGate), so the
// accept-flip's witness-path coverage falls with this fraction. Before R2.11 a validator's
// registrations rode ONLY its own proposals; after R2.11 whoever proposes next carries every
// peer's pending registration. This test measures both regimes on the same harness at three
// validator counts and LOGS the table. It asserts only that the measurement ran; the number is
// evidence for R3.4, not a gate on it.

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// carrierFraction runs one regime: V anchors past the v5 boundary, every validator bonded
// (each proposes once, carrying its own bond), every validator staging one issuer key; with
// submit=true every validator submits its registration to every peer first. Then `rounds`
// round-robin proposals. Returns (carrier blocks, total blocks proposed in the measured window).
func carrierFraction(t *testing.T, V, rounds int, submit bool) (carriers, total int) {
	t.Helper()
	nodes, ids, net, _, _ := era4AnchorNet(t, V)
	all := make([]ports.NodeID, V)
	for i, id := range ids {
		all[i] = id.NodeID()
	}
	saturateValidatorsSeen(t, nodes, net, all)
	// Bond everyone: each validator proposes one block, which folds its own bond reg.
	for i, nd := range nodes {
		nd.EnableBond(ids[i].Signer(), 2<<20)
	}
	for i, nd := range nodes {
		if err := proposeOnce(t, nd, net, all, "bond"+string(rune('0'+i))); err != nil {
			t.Fatalf("V=%d bond proposal %d: %v", V, i, err)
		}
	}
	for _, nd := range nodes {
		if !nd.chain.IsBonded(nd.id) {
			t.Fatalf("V=%d: a validator's bond did not commit", V)
		}
	}
	// Stage one key per validator (single epoch: EpochBlocks is unset in this harness, so the
	// epoch is 0 throughout and every registration stays in-window).
	for _, nd := range nodes {
		key, err := rsa.GenerateKey(rand.Reader, 1024) // size is irrelevant to the measurement
		if err != nil {
			t.Fatal(err)
		}
		nd.SetDemandIssuerKey(rand.Reader, nd.DemandEpoch(), key)
	}
	if submit {
		for _, nd := range nodes {
			nd.SubmitIssuerKeyRegs(all)
		}
		drainHeld(t, net, fifo)
	}
	// The measured window: `rounds` proposals, round-robin over the validators.
	startLen := nodes[0].chain.Len()
	for r := 0; r < rounds; r++ {
		p := nodes[r%V]
		if err := proposeOnce(t, p, net, all, "m"+string(rune('a'+r%26))); err != nil {
			t.Fatalf("V=%d measured proposal %d: %v", V, r, err)
		}
	}
	blocks := nodes[0].chain.Blocks(uint64(startLen))
	for _, b := range blocks {
		total++
		if len(b.IssuerKeys) > 0 {
			carriers++
		}
	}
	return carriers, total
}

// TestR34IssuerKeyCarrierFractionMeasurement logs the carrier fraction before and after R2.11
// for V ∈ {2, 4, 8} over 2V round-robin proposals from a cold key schedule (every validator
// has one uncommitted registration when the window opens). Read the table, not the pass.
func TestR34IssuerKeyCarrierFractionMeasurement(t *testing.T) {
	if testing.Short() {
		t.Skip("measurement, not a gate; skipped under -short")
	}
	for _, V := range []int{2, 4, 8} {
		rounds := 2 * V
		bc, bt := carrierFraction(t, V, rounds, false)
		ac, at := carrierFraction(t, V, rounds, true)
		t.Logf("R3.4 carrier fraction | V=%d | %d round-robin blocks from a cold key schedule | BEFORE R2.11 (own proposals only): %d/%d carriers (%.0f%%) | AFTER R2.11 (peer submit): %d/%d carriers (%.0f%%)",
			V, rounds, bc, bt, 100*float64(bc)/float64(bt), ac, at, 100*float64(ac)/float64(at))
		if bt != rounds || at != rounds {
			t.Fatalf("V=%d: measured window did not produce %d blocks (%d / %d)", V, rounds, bt, at)
		}
	}
	t.Log("Interpretation: from a cold schedule every validator's key must ride SOME block. Before R2.11 that is one carrier block per validator (each carries its own on its own turn); after R2.11 the first proposer can carry every peer's pending key at once, so the carrier COUNT per key-schedule turn falls toward one. Steady state is one key-schedule turn per epoch (W+1 registrations per validator), so the per-epoch carrier count is what R3.4's coverage argument needs; this harness has no epoch turns (EpochBlocks unset) and measures one cold turn.")
}
