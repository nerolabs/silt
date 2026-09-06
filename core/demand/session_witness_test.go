package demand

// G-DEM-1, G-DEM-5 (bank half), G-DEM-6, G-DEM-7 — the witnessed-demand observable under
// R2.9 sessions (certification R2.9-witnessed-demand-observable-under-sessions-2026-09-06
// §7). Ablations that must redden: `b.increments[object]++` (G-DEM-1 batched arm); write
// v3 units into demand[] (G-DEM-6); a map[Hash]map[NodeID]… field on Bank (G-DEM-7).

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

func TestV2AndV3DoNotShareACounter(t *testing.T) {
	obj := ports.HashBytes([]byte("g-dem-6"))
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	b := NewBank()
	b.Witness(obj, pub, 40)
	if b.Demand(obj) != 0 {
		t.Fatalf("a v3 settlement moved the v2 demand counter to %d — a mixed-denomination map is meaningless (rule 6)", b.Demand(obj))
	}
	b.demand[obj]++ // a v2 redeem's bump, in its own unit
	if b.WitnessedIncrements(obj) != 40 || b.Demand(obj) != 1 {
		t.Fatalf("v3 %d / v2 %d after one bump each side, want 40 / 1", b.WitnessedIncrements(obj), b.Demand(obj))
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
