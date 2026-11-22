package chain

import (
	"math/rand"
	"reflect"
	"testing"
)

// pmapContents is m's entries as a Go map.
func pmapContents[K comparable, V any](m *pmap[K, V]) map[K]V {
	out := map[K]V{}
	m.Range(func(k K, v V) bool {
		out[k] = v
		return true
	})
	return out
}

// A pmap behaves as a Go map under any sequence of sets, overwrites and deletes,
// and every earlier version of it is unchanged by what came after.
func TestPmapMatchesAMapAndKeepsEveryVersion(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	var m pmap[int, int]
	ref := map[int]int{}
	type snapshot struct {
		m   pmap[int, int]
		ref map[int]int
	}
	var versions []snapshot
	for step := 0; step < 20000; step++ {
		k := rng.Intn(3000)
		if rng.Intn(4) == 0 {
			m.Delete(k)
			delete(ref, k)
		} else {
			m.Set(k, step)
			ref[k] = step
		}
		want, wantOK := ref[k]
		if v, ok := m.Get(k); ok != wantOK || v != want {
			t.Fatalf("step %d: Get(%d) = %d,%v; map holds %d,%v", step, k, v, ok, want, wantOK)
		}
		if step%997 == 0 {
			cp := map[int]int{}
			for a, b := range ref {
				cp[a] = b
			}
			versions = append(versions, snapshot{m, cp})
		}
	}
	if m.Len() != len(ref) || !reflect.DeepEqual(pmapContents(&m), ref) {
		t.Fatalf("pmap holds %d entries, map %d, or they differ", m.Len(), len(ref))
	}
	for i, v := range versions {
		if v.m.Len() != len(v.ref) || !reflect.DeepEqual(pmapContents(&v.m), v.ref) {
			t.Fatalf("version %d changed after later writes", i)
		}
		for k, want := range v.ref {
			if got, ok := v.m.Get(k); !ok || got != want {
				t.Fatalf("version %d: Get(%d) = %d,%v, want %d", i, k, got, ok, want)
			}
		}
	}
}

// The trie is a function of its key set: the same entries inserted in different
// orders, or reached after deleting others, build equal nodes.
func TestPmapShapeIsHistoryIndependent(t *testing.T) {
	keys := rand.New(rand.NewSource(5)).Perm(2000)
	var a, b pmap[int, bool]
	for _, k := range keys {
		a.Set(k, true)
	}
	for i := len(keys) - 1; i >= 0; i-- {
		b.Set(keys[i], true)
	}
	for k := 2000; k < 2600; k++ {
		b.Set(k, true)
	}
	for k := 2000; k < 2600; k++ {
		b.Delete(k)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("equal key sets built different tries")
	}
	for _, k := range keys {
		a.Delete(k)
	}
	if a.root != nil || a.Len() != 0 {
		t.Fatalf("deleting every key left %d entries or a non-empty root", a.Len())
	}
}
