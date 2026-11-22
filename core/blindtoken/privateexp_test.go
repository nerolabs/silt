package blindtoken

import (
	"crypto/rand"
	"crypto/rsa"
	"math/big"
	mrand "math/rand"
	"testing"
)

// The CRT path is an optimization, never a different signature: for every key and
// input it returns exactly c^D mod N, and so does SignBlinded, whose bytes are
// committed in publish tokens on chain. A key without CRT values takes the full-size
// path and gets the same number.
func TestPrivateExpIsTheFullExponentiation(t *testing.T) {
	rng := mrand.New(mrand.NewSource(3))
	for k := 0; k < 3; k++ {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		bare := *priv
		bare.Precomputed = rsa.PrecomputedValues{}
		for i := 0; i < 20; i++ {
			c := new(big.Int).Rand(rng, priv.N)
			if i == 0 {
				c.SetInt64(0)
			}
			want := new(big.Int).Exp(c, priv.D, priv.N)
			if got := privateExp(priv, c); got.Cmp(want) != 0 {
				t.Fatalf("key %d input %d: CRT result differs from c^D mod N", k, i)
			}
			if got := privateExp(&bare, c); got.Cmp(want) != 0 {
				t.Fatalf("key %d input %d: the full-size fallback differs from c^D mod N", k, i)
			}
			if c.Sign() == 0 {
				continue
			}
			sig, err := SignBlinded(rand.Reader, priv, c.Bytes())
			if err != nil {
				t.Fatalf("key %d input %d: sign: %v", k, i, err)
			}
			if new(big.Int).SetBytes(sig).Cmp(want) != 0 {
				t.Fatalf("key %d input %d: SignBlinded returned a different signature", k, i)
			}
		}
	}
}

func BenchmarkSignBlinded(b *testing.B) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatal(err)
	}
	c := new(big.Int).Rand(mrand.New(mrand.NewSource(1)), priv.N).Bytes()
	for i := 0; i < b.N; i++ {
		if _, err := SignBlinded(rand.Reader, priv, c); err != nil {
			b.Fatal(err)
		}
	}
}
