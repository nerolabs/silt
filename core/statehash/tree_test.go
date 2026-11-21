package statehash

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

func leavesOf(m map[string][]byte) []Leaf {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Leaf, 0, len(keys))
	for _, k := range keys {
		out = append(out, Leaf{Key: []byte(k), Value: m[k]})
	}
	return out
}

// After any sequence of inserts, overwrites and removals, the tree's root is the
// root Root computes from scratch over the resulting leaf set, and RootWith
// predicts each step's root without moving the tree.
func TestTreeMatchesRebuildAcrossChanges(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	state := map[string][]byte{}
	for i := 0; i < 300; i++ {
		state[fmt.Sprintf("k%d", i)] = []byte{byte(i), 1}
	}
	tree, err := NewTree(leavesOf(state))
	if err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 40; step++ {
		var changes []Change
		for j := 0; j < 1+rng.Intn(12); j++ {
			k := fmt.Sprintf("k%d", rng.Intn(400))
			switch rng.Intn(3) {
			case 0:
				changes = append(changes, Change{Key: []byte(k)}) // removal, present or not
				delete(state, k)
			default:
				v := []byte{byte(rng.Intn(256)), byte(step)}
				changes = append(changes, Change{Key: []byte(k), Value: v})
				state[k] = v
			}
		}
		before := tree.Root()
		want, err := Root(leavesOf(state))
		if err != nil {
			t.Fatal(err)
		}
		predicted, err := tree.RootWith(changes)
		if err != nil {
			t.Fatal(err)
		}
		if tree.Root() != before {
			t.Fatalf("step %d: RootWith moved the committed root", step)
		}
		if predicted != want {
			t.Fatalf("step %d: RootWith %x, rebuild %x", step, predicted, want)
		}
		if err := tree.Apply(changes); err != nil {
			t.Fatal(err)
		}
		if tree.Root() != want {
			t.Fatalf("step %d: applied root %x, rebuild %x", step, tree.Root(), want)
		}
	}
}

func TestTreeEmptyAndRefusals(t *testing.T) {
	empty, err := NewTree(nil)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Root(nil)
	if empty.Root() != want {
		t.Fatalf("empty tree root %x, Root %x", empty.Root(), want)
	}
	one := []Change{{Key: []byte("a"), Value: []byte{1}}}
	got, err := empty.RootWith(one)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := Root([]Leaf{{Key: []byte("a"), Value: []byte{1}}}); got != want {
		t.Fatalf("one leaf over the empty tree: %x, want %x", got, want)
	}
	if _, err := NewTree([]Leaf{{Key: []byte("a"), Value: []byte{1}}, {Key: []byte("a"), Value: []byte{2}}}); err == nil {
		t.Fatal("a duplicate key must be refused")
	}
	if err := empty.Apply([]Change{{Key: []byte("a"), Value: []byte{}}}); err == nil {
		t.Fatal("an empty value must be refused")
	}
}

// Updating the kept tree costs one path per changed leaf, so a small change
// is cheap however many leaves the tree holds.
func BenchmarkTreeRootWith(b *testing.B) {
	for _, n := range []int{1_000, 10_000, 40_000} {
		leaves := make([]Leaf, n)
		for i := range leaves {
			leaves[i] = Leaf{Key: []byte(fmt.Sprintf("leaf-%d", i)), Value: []byte{1}}
		}
		tree, err := NewTree(leaves)
		if err != nil {
			b.Fatal(err)
		}
		changes := []Change{{Key: []byte("new-a"), Value: []byte{1}}, {Key: []byte("new-b"), Value: []byte{1}}}
		b.Run(fmt.Sprintf("leaves=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := tree.RootWith(changes); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
