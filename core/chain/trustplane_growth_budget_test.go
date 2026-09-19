package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// The trust plane is only as decentralized as the number of people who can afford to hold
// it. An archival node retains every heavy proof to genesis and is the only tier that can
// serve the deep past a pruning swarm has dropped, so the cost of being archival sets how
// many independent parties can offer that service. If it costs more than a volunteer will
// spend, the deep past has a handful of custodians — a permanent center arrived at through
// a storage bill rather than through anyone's decision.
//
// The comparison that matters is a volunteer-run network that already works: tens of
// thousands of people hold a few hundred gigabytes each, because a few hundred gigabytes
// is what a spare drive costs. That is the budget this gate holds the design to.
const (
	// What a volunteer will plausibly dedicate to holding full history.
	enthusiastArchivalBudgetBytes = 600 << 30 // a spare drive

	// The window that budget has to cover before re-provisioning is a new decision.
	archivalHorizonYears = 5

	// Seconds per committed block on the deployed configuration.
	secondsPerBlock = 45.9

	// Bonded standing lapses after DerivedBondTTL blocks unless the validator
	// re-registers with a fresh space-time proof, and the renewal loop runs at half
	// that, so each bonded validator writes a heavy proof on-chain this often.
	bondRenewalBlocks = 32 / 2

	// Measured encoded size of a bond space-time answer. It is dominated by the
	// labeling opens, so it stays near-constant as the plot grows.
	bondAnswerBytes = 1500 << 10
)

// TestTrustPlaneGrowthFitsAVolunteerArchivalTier prices the archival tier against the
// number of bonded participants, and rejects a growth rate that leaves full history
// affordable only to a few.
//
// Both inputs are the design's own: the attestation traffic every block carries, and the
// heavy possession proof every bonded validator must re-publish to keep its standing. The
// second dominates, and it scales with the very thing the network wants to maximize — the
// number of independent operators holding standing. That is the tension this gate exists
// to keep visible: more participants means a faster-growing history, and a faster-growing
// history means fewer parties able to retain it.
func TestTrustPlaneGrowthFitsAVolunteerArchivalTier(t *testing.T) {
	blocksPerYear := (365.0 * 24 * 3600) / secondsPerBlock

	keys := make([]ed25519.PrivateKey, 128)
	for i := range keys {
		keys[i] = key(int64(20000 + i))
	}
	prev := ports.HashBytes([]byte("parent"))

	// Light block cost: the attestations and carrier a block carries at a given
	// attester count, measured on the shipped encoding.
	lightBlockBytes := func(attesters int) int {
		b := &Block{Version: BlockVersionWitnessable, Height: 1, Prev: prev,
			Entries: []ports.Entry{entry(1)}}
		Sign(b, keys[0])
		for i := 1; i <= attesters; i++ {
			b.Atts = append(b.Atts, Attest(b, keys[i]))
			b.LastCommit = append(b.LastCommit, Attest(b, keys[i]))
		}
		return len(EncodeBlocks([]Block{*b}))
	}

	if lightBlockBytes(100) <= lightBlockBytes(10) {
		t.Fatal("GATE VACUOUS: block size does not grow with attester count — the measurement is not observing the encoding")
	}

	// Priced at the smallest bonded set the design contemplates. A larger one only
	// makes the number worse, so a failure here fails everywhere above it.
	const bondedValidators = 100
	attesters := bondedValidators
	if attesters > 100 {
		attesters = 100
	}

	lightPerYear := float64(lightBlockBytes(attesters)) * blocksPerYear

	// Heavy possession proofs are the dominant payload, and what bounds them is the
	// archival heavy-proof window: past it they are shed, so their contribution is a
	// STANDING VOLUME rather than an annual rate. Without a window this term grows
	// with both time and operator count, and a more decentralized network becomes one
	// fewer parties can archive.
	heavyPerBlock := float64(bondedValidators) / float64(bondRenewalBlocks) * float64(bondAnswerBytes)
	heavyResident := heavyPerBlock * float64(DefaultArchiveProofHeavyWindow)
	heavyUnbounded := heavyPerBlock * blocksPerYear * archivalHorizonYears
	totalOverHorizon := lightPerYear*archivalHorizonYears + heavyResident

	gib := func(b float64) float64 { return b / (1 << 30) }
	t.Logf("at %d bonded validators: light %.1f GiB/yr; heavy %.1f GiB resident over a %d-block window "+
		"(%.1f GiB if never shed); %.1f GiB over %d years",
		bondedValidators, gib(lightPerYear), gib(heavyResident), DefaultArchiveProofHeavyWindow,
		gib(heavyUnbounded), gib(totalOverHorizon), archivalHorizonYears)

	if heavyUnbounded <= heavyResident {
		t.Fatal("GATE VACUOUS: shedding past the window frees nothing — the window is not bounding anything")
	}

	if totalOverHorizon > enthusiastArchivalBudgetBytes {
		t.Fatalf("FULL HISTORY OUTGROWS A VOLUNTEER ARCHIVAL TIER — at %d bonded validators full history reaches "+
			"%.1f GiB over %d years against a %d GiB volunteer budget. Each bonded validator re-publishes a %d KiB "+
			"possession proof every %d blocks to keep its standing, so the stored volume grows with the NUMBER OF "+
			"INDEPENDENT OPERATORS — the quantity decentralization is measured by — and the deep past ends up held "+
			"by whoever can afford terabytes rather than by the many.",
			bondedValidators, gib(totalOverHorizon), archivalHorizonYears,
			enthusiastArchivalBudgetBytes>>30, bondAnswerBytes>>10, bondRenewalBlocks)
	}
	t.Logf("full history stays inside a volunteer's budget: %.1f GiB over %d years, %.0fx less than unshed",
		gib(totalOverHorizon), archivalHorizonYears, heavyUnbounded/heavyResident)
}
