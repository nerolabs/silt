package chain

import "testing"

// How many independent operators can hold standing at once is not a policy choice — it is
// set by what every validator must receive.
//
// Standing lapses unless a validator re-publishes a possession proof, and that proof goes
// on-chain, so every other validator downloads and verifies it. Each node's ingest is
// therefore the set size times the proof volume per validator, and it grows with the number
// of independent operators — the quantity decentralization is measured by. Retention policy
// does not touch this: shedding old proofs frees disk, not the live wire.
//
// The floor box is the constraint that matters, because the whole point of the tier is that
// a small operator can hold standing on it. If keeping up with consensus needs more
// bandwidth than a home connection spares, the floor tier cannot participate, the bonded set
// collapses to whoever has a datacentre uplink, and consensus weight concentrates there for
// reasons no one chose.
const (
	// What a small operator's box can reasonably give consensus ingest on a home
	// connection — a slice of the uplink, not the whole of it.
	floorBoxConsensusIngestBitsPerSec = 2 << 20 // 2 Mbit/s

	// The bonded set this cadence admits today, measured. It is FAR below the number
	// of floor boxes the design expects to hold standing, and closing that gap needs a
	// smaller proof or a longer renewal cadence — neither of which is a tuning change.
	// Pinned here so the number is visible and so any change that makes it WORSE is
	// caught, rather than discovered when the floor tier quietly stops keeping up.
	measuredAdmittedValidators = 125
)

// TestTheBondedSetFitsTheFloorBoxUplink prices the bonded set against what a floor box can
// receive, and reports the set size the shipped renewal cadence actually admits.
func TestTheBondedSetFitsTheFloorBoxUplink(t *testing.T) {
	secondsPerRenewal := secondsPerBlock * float64(bondRenewalBlocks)
	perValidatorBitsPerSec := float64(bondAnswerBytes) * 8 / secondsPerRenewal
	admits := float64(floorBoxConsensusIngestBitsPerSec) / perValidatorBitsPerSec

	if perValidatorBitsPerSec <= 0 {
		t.Fatal("GATE VACUOUS: the per-validator rate is zero — the measurement is not observing the cadence")
	}
	t.Logf("each bonded validator publishes %d KiB every %.0fs — %.1f kbit/s of ingest at every other node; "+
		"a %d Mbit/s budget admits about %.0f of them",
		bondAnswerBytes>>10, secondsPerRenewal, perValidatorBitsPerSec/1024,
		floorBoxConsensusIngestBitsPerSec>>20, admits)

	if admits < float64(measuredAdmittedValidators) {
		t.Fatalf("THE BONDED SET GOT SMALLER — this cadence now admits about %.0f validators on a %d Mbit/s "+
			"floor-box budget, down from the pinned %d. Standing lapses unless each validator re-publishes a %d KiB "+
			"possession proof every %.0f seconds, and every other validator must receive and verify all of it, so "+
			"ingest scales with the number of independent operators and this number IS the ceiling on how "+
			"decentralized the bonded set can be. It was already far below the floor-box population the design "+
			"expects to hold standing; making it smaller moves the wrong way. This is live traffic — shedding old "+
			"proofs frees disk and changes nothing here. The levers are the proof's size and the renewal cadence.",
			admits, floorBoxConsensusIngestBitsPerSec>>20, measuredAdmittedValidators,
			bondAnswerBytes>>10, secondsPerRenewal)
	}
	if admits > float64(measuredAdmittedValidators)*1.5 {
		t.Fatalf("THE BONDED SET GOT LARGER (%.0f, pinned %d) — good news that must be recorded rather than "+
			"absorbed: re-pin measuredAdmittedValidators so the ratchet keeps holding the new floor.",
			admits, measuredAdmittedValidators)
	}
}
