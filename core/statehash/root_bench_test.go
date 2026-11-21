package statehash

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"
)

// Root rebuilds the tree from every leaf, so its cost grows with the state:
// one leaf per committed entry and per spent token serial.
func BenchmarkRoot(b *testing.B) {
	for _, n := range []int{1_000, 10_000, 40_000} {
		leaves := make([]Leaf, n)
		for i := range leaves {
			var seed [8]byte
			binary.BigEndian.PutUint64(seed[:], uint64(i))
			k := sha256.Sum256(seed[:])
			leaves[i] = Leaf{Key: k[:], Value: make([]byte, 96)}
			leaves[i].Value[0] = 1
		}
		b.Run(fmt.Sprintf("leaves=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := Root(leaves); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
