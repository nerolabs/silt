package demand

// G-DEM-1, G-DEM-5 (bank half), G-DEM-6, G-DEM-7 — the witnessed-demand observable under
// R2.9 sessions (certification R2.9-witnessed-demand-observable-under-sessions-2026-09-06
// §7). Ablations that must redden: `b.increments[object]++` (G-DEM-1 batched arm); write
// a SECOND map[Hash]int64 on Bank (G-DEM-6); a map[Hash]map[NodeID]… field (G-DEM-7).

import (
	"crypto/ed25519"
	"crypto/rand"
	"reflect"
	"testing"

	"github.com/nerolabs/silt/ports"
)

func TestWitnessedDemandIsDenominatedInSettledIncrements(t *testing.T) {
	obj := ports.HashBytes([]byte("g-dem-1"))
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	batched := NewBank()
	if ok, why := batched.Witness(obj, pub, 256); !ok {
		t.Fatal(why)
	}
	split := NewBank()
	for i := 0; i < 256; i++ {
		split.Witness(obj, pub, 1)
	}
	if batched.WitnessedIncrements(obj) != 256 || split.WitnessedIncrements(obj) != 256 {
		t.Fatalf("batched %d, split %d — both must be 256 (the counter is denominated in settled increments, not receipts)", batched.WitnessedIncrements(obj), split.WitnessedIncrements(obj))
	}
	if ok, _ := batched.Witness(obj, pub, 0); ok {
		t.Fatal("a zero-unit witness credited")
	}
}

func TestBondedDistinctCountIsItsOwnSurface(t *testing.T) {
	obj := ports.HashBytes([]byte("g-dem-5"))
	bonded, _, _ := ed25519.GenerateKey(rand.Reader)
	unbonded, _, _ := ed25519.GenerateKey(rand.Reader)
	b := NewBank()
	b.RequireBondedFetcher(func(pub []byte) (string, bool) {
		if string(pub) == string(bonded) {
			return "slot-1", true
		}
		return "", false
	})
	for i := 0; i < 5; i++ {
		if ok, why := b.Witness(obj, bonded, 3); !ok {
			t.Fatalf("settlement %d of the same bonded fetcher: %s — P3b keeps admission, loses dedup on the increment counter", i, why)
		}
	}
	if b.DistinctBondedFetchers(obj) != 1 || b.WitnessedIncrements(obj) != 15 {
		t.Fatalf("distinct %d (want 1), increments %d (want 15) — two surfaces, never one field", b.DistinctBondedFetchers(obj), b.WitnessedIncrements(obj))
	}
	if ok, _ := b.Witness(obj, unbonded, 3); ok || b.WitnessedIncrements(obj) != 15 || b.DistinctBondedFetchers(obj) != 1 {
		t.Fatal("an unbonded fetcher contributed to a surface with P3b on")
	}
}

// TestTheBankHoldsExactlyOnePerObjectCount is G-DEM-6 after C1. The v2 counter it used
// to be measured against (demand[], one unit per redeemed token, up to 50,000x the v3
// unit) is DELETED, so the property is now structural: the bank may hold exactly ONE
// per-object count, and a second one — of any denomination — cannot reappear without
// this failing. A field, not a call: the defect the original gate caught was two
// counters in one struct, which no call-level assertion can see.
//
// Ablation (run RED 2026-09-08): add `demand map[ports.Hash]int64` back to Bank →
// "Bank holds 2 per-object count fields ([increments demand])".
func TestTheBankHoldsExactlyOnePerObjectCount(t *testing.T) {
	obj := ports.HashBytes([]byte("g-dem-6"))
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	b := NewBank()
	b.Witness(obj, pub, 40)
	if b.WitnessedIncrements(obj) != 40 {
		t.Fatalf("increments %d, want 40", b.WitnessedIncrements(obj))
	}
	typ := reflect.TypeOf(Bank{})
	hashT := reflect.TypeOf(ports.Hash{})
	var counts []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Type.Kind() == reflect.Map && f.Type.Key() == hashT && f.Type.Elem().Kind() == reflect.Int64 {
			counts = append(counts, f.Name)
		}
	}
	if len(counts) != 1 || counts[0] != "increments" {
		t.Fatalf("Bank holds %d per-object count fields (%v), want exactly one named increments — "+
			"a mixed-denomination pair of counters is meaningless to any consumer (rule 6, the v2 demand[] defect)", len(counts), counts)
	}
}

func TestWitnessedDemandCarriesNoIdentityAxis(t *testing.T) {
	typ := reflect.TypeOf(Bank{})
	hashT, nodeT := reflect.TypeOf(ports.Hash{}), reflect.TypeOf(ports.NodeID{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Type.Kind() != reflect.Map || f.Type.Key() != hashT {
			continue
		}
		if e := f.Type.Elem(); e.Kind() == reflect.Map && e.Key() == nodeT {
			t.Fatalf("Bank.%s is object × identity — the Don't #3 defect (G-λ-8-7 one layer down)", f.Name)
		}
	}
}
