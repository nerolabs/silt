package blindtoken

import (
	"crypto/rsa"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
)

// ErrDoubleSpend is returned when a token serial is presented more than once.
var ErrDoubleSpend = errors.New("blindtoken: token already spent")

// MarshalPub / ParsePub serialize an RSA issuer public key for the wire as
// len(N)‖N‖E — a validator publishes it so peers can verify its token
// signatures. (Manual, not x509, so core keeps a minimal import surface.)
func MarshalPub(pub *rsa.PublicKey) []byte {
	nb := pub.N.Bytes()
	out := make([]byte, 8+len(nb))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(nb)))
	copy(out[4:], nb)
	binary.BigEndian.PutUint32(out[4+len(nb):], uint32(pub.E))
	return out
}

func ParsePub(b []byte) (*rsa.PublicKey, error) {
	if len(b) < 8 {
		return nil, errors.New("blindtoken: issuer key too short")
	}
	n := binary.BigEndian.Uint32(b[:4])
	if uint64(len(b)) < uint64(8)+uint64(n) {
		return nil, errors.New("blindtoken: issuer key truncated")
	}
	pub := &rsa.PublicKey{
		N: new(big.Int).SetBytes(b[4 : 4+n]),
		E: int(binary.BigEndian.Uint32(b[4+n : 8+n])),
	}
	// A DECODED KEY IS UNTRUSTED INPUT, NOT A KEY (red-team re-break F4, 2026-09-03).
	// This used to bound nothing: any N, any E. N = 0 panicked every verifier inside
	// big.Int.Mod (division by zero) — reachable today, a bonded Byzantine issuer
	// crashing every fetcher that transacts with it. N = 1 made s^e mod 1 == 0 == the
	// FDH image, so EVERY (serial, sig) pair verified — a universal forgery. E = 1 made
	// sig := FDH(msg) computable by anyone holding the public key. The consensus
	// commitment cannot catch any of it: it attests sha256(MarshalPub(key)), 32 bytes,
	// which is a binding on WHICH BYTES an issuer serves and never on whether those
	// bytes are an unforgeable signature scheme. RFC 9578 key configurations carry
	// validated key material; this one now does too.
	if err := ValidatePub(pub); err != nil {
		return nil, err
	}
	return pub, nil
}

// Modulus and exponent bounds for a well-formed issuer key.
const (
	// MinModulusBits is the smallest RSA modulus this lane will hold. Every key the
	// tree generates is 2048-bit (adapters/diskissuer), so this rejects only degenerate
	// and deliberately-weak moduli.
	MinModulusBits = 2048
	// MaxModulusBits bounds the COST of the modexp an untrusted key can force. Without
	// it a served 1-Mbit modulus turns one verify into seconds of CPU on the
	// single-threaded node loop — the same lane the N=0 panic reached.
	MaxModulusBits = 8192
	// MaxPubExp is the largest public exponent accepted: the wire encoding is a
	// uint32 (MarshalPub), so anything above it could never have round-tripped anyway,
	// and it bounds the verify exponentiation the same way MaxModulusBits does.
	MaxPubExp = 1<<32 - 1
	// MinPubExp is the FIPS 186-5 App. A.1.1 exponent floor: "the exponent e shall be
	// an odd, positive integer such that 2^16 < e < 2^256". e must be STRICTLY above
	// it. crypto/rsa.GenerateKey uses 65537, so every key this tree mints clears it by
	// exactly one; a smaller e enables the gcd(e, phi(N)) > 1 blindness partition.
	MinPubExp = 1 << 16
)

// ErrBadPubKey rejects an issuer public key that is not a well-formed RSA key.
var ErrBadPubKey = errors.New("blindtoken: malformed issuer public key")

// ValidatePub is the ONE definition of "this is an RSA public key" for every lane
// that holds an issuer key. It is called at the wire boundary (ParsePub), at the
// keyset door (demand.Keyset.Put), and defensively before every modexp, so a
// degenerate key is a legible refusal at the earliest boundary that sees it and can
// never be a panic at the latest.
//
// SHAPE, THEN HARDNESS (crypto-specialist advisory C-3, 2026-09-03). The first four
// checks bound the key's SHAPE. They are necessary and were not sufficient: the seat's
// spike fed four moduli through the shape-only bound set and ALL FOUR passed —
//
//	N = one 2048-bit PRIME       → φ(N) = N−1 is public, so ANY node computes d
//	N = 122 distinct 17-bit primes → any node that trial-divides N computes d
//	N = p² (p 1024-bit)          → a perfect power, factorable
//	E = 3                        → below FIPS 186-5's 2^16 floor
//
// and the first two are outright universal forgery by any observer. The consensus
// commitment cannot catch any of it: it attests sha256(MarshalPub(key)), a binding on
// WHICH BYTES an issuer serves and never on whether those bytes are an unforgeable
// signature scheme. So a Byzantine bonded issuer could commit a deliberately weak
// modulus, every redeemer would pin it (the fingerprint matches), and "a committed key
// means tokens were paid for" — the premise the credit layer's conservation argument
// rests on — would be false.
//
// The four hardness checks below are the ACME/Boulder GoodKey schema and NIST SP
// 800-56B partial public-key validation, adopted rather than invented (tenet B8). They
// do NOT make N provably a semiprime; that needs a ZK proof of correct key generation,
// which is out of scope. State the residual honestly: BLINDNESS AGAINST A MALICIOUS
// ISSUER IS BOUNDED BY KEY-CORRECTNESS ASSUMPTIONS, NOT PROVEN, and the on-chain
// commitment bounds EQUIVOCATION, not soundness.
//
// Shape:
//   - N odd (every RSA modulus is a product of odd primes; this alone rejects 0 and
//     every even degenerate), in [MinModulusBits, MaxModulusBits].
//   - E odd (an even exponent is not invertible mod φ(N), so no signature could ever
//     verify) and at most MaxPubExp.
//
// Hardness:
//   - E > 65536 (FIPS 186-5 App. A.1.1: "2^16 < e < 2^256"). Every key this tree
//     generates uses e = 65537, so this rejects nothing honest.
//   - no prime factor below SmallFactorBound (one gcd against the primorial) — kills
//     the smooth modulus. See SmallFactorBound for exactly what that bar buys.
//   - N is not a perfect power p^k — kills the p² case.
//   - N is not itself prime (Miller-Rabin) — kills the single-prime universal forgery.
//
// COST, measured (TestValidatePubHardnessCostBudget): well under the 5 ms per-pin
// budget, and it runs at most W+1 = 5 times per issuer per window.
func ValidatePub(pub *rsa.PublicKey) error {
	if err := validateShape(pub); err != nil {
		return err
	}
	if g := smallFactor(pub.N); g != nil {
		return fmt.Errorf("%w: modulus shares a %d-bit factor with the primorial below %d "+
			"— a smooth modulus is factorable by any verifier, so anyone can compute d "+
			"and mint tokens", ErrBadPubKey, g.BitLen(), SmallFactorBound)
	}
	if k := perfectPower(pub.N); k != 0 {
		return fmt.Errorf("%w: modulus is a perfect %d-th power, which is factorable",
			ErrBadPubKey, k)
	}
	// ProbablyPrime(0) is already a Baillie-PSW test in Go (Miller-Rabin bases plus a
	// Lucas test) and no composite is known to pass it; the argument only adds extra
	// RANDOM Miller-Rabin rounds. One is what the advisory priced (2.2 ms measured) and
	// it is the dominant term in this function's cost, so it stays at one.
	if pub.N.ProbablyPrime(1) {
		return fmt.Errorf("%w: modulus is PRIME, so phi(N) = N-1 is public and any "+
			"holder of the public key can compute d — a universal forgery", ErrBadPubKey)
	}
	return nil
}

// validateShape is the CHEAP half: nil, N positive/odd/in-range, E odd/in-range. It is
// the LAST-LINE defence run before every modexp (blindD, Unblind, verifyD), and it must
// stay cheap for exactly that reason — those are hot paths on the single-threaded node
// loop, and an attacker chooses how often they run. It is what closes the N = 0 panic
// and the N = 1 / E = 1 universal forgeries (red-team re-break F4).
//
// The HARDNESS half (primorial gcd, perfect power, primality — ~3.5 ms) runs only where
// a key is ADMITTED: ParsePub at the wire boundary and demand.Keyset.Put at the keyset
// door. Running it per-modexp would turn a 12 µs verify into a 3.5 ms one and hand an
// attacker a 300x CPU amplifier on the node loop, which is the same lane the N=0 panic
// reached. Admission is the right place: a key that cleared the door is already trusted
// to be a key, and this is defence in depth against a bypass, not a second admission.
func validateShape(pub *rsa.PublicKey) error {
	if pub == nil || pub.N == nil {
		return fmt.Errorf("%w: nil", ErrBadPubKey)
	}
	if pub.N.Sign() <= 0 || pub.N.Bit(0) == 0 {
		return fmt.Errorf("%w: modulus is not a positive odd integer", ErrBadPubKey)
	}
	if bits := pub.N.BitLen(); bits < MinModulusBits || bits > MaxModulusBits {
		return fmt.Errorf("%w: modulus is %d bits, want [%d, %d]",
			ErrBadPubKey, bits, MinModulusBits, MaxModulusBits)
	}
	if pub.E%2 == 0 || int64(pub.E) > MaxPubExp {
		return fmt.Errorf("%w: public exponent %d is not an odd integer in (%d, %d]",
			ErrBadPubKey, pub.E, MinPubExp, int64(MaxPubExp))
	}
	if int64(pub.E) <= MinPubExp {
		return fmt.Errorf("%w: public exponent %d is at or below the FIPS 186-5 floor "+
			"2^16 (a small e enables the gcd(e, phi(N)) > 1 blindness partition)",
			ErrBadPubKey, pub.E)
	}
	return nil
}

// SmallFactorBound is the trial-division bar: ValidatePub refuses any modulus with a
// prime factor below it.
//
// IT IS A BAR, NOT A PROOF, and the honest statement is in two halves. What it buys:
// every "product of many small primes" modulus up to 20-bit factors is refused, which
// covers the advisory's 122-times-17-bit shape and everything an attacker could build
// from a sieve that fits in memory. What it does NOT buy: an attacker who uses 21-bit
// factors walks past it. Deciding "N is a product of exactly two large primes" cheaply
// is not possible; that needs a ZK proof of correct key generation, which is out of
// scope (see ValidatePub's residual note).
//
// The bound is 2^20 rather than ACME/Boulder's 2^16 because the primorial-GCD form
// below makes the higher bar CHEAPER than naive trial division at the lower one:
// measured 0.77 ms per key at 2^20 against 0.30 ms of batched modular reductions at
// 2^16, both far under the 5 ms per-pin budget (TestC3_ValidatePubCostBudget).
const SmallFactorBound = 1 << 20

// smallPrimorial is the product of every odd prime below SmallFactorBound, so one
// gcd(N, P) screens them all at once. 2 is omitted: N is already required to be odd.
//
// It is built LAZILY (sync.Once) with a product tree — 1.3 ms to sieve plus 24 ms to
// multiply, measured — so a process that never pins an issuer key pays nothing, and the
// cost lands once rather than on every ValidatePub.
var (
	smallPrimorialOnce sync.Once
	smallPrimorial     *big.Int
)

func primorial() *big.Int {
	smallPrimorialOnce.Do(func() {
		sieve := make([]bool, SmallFactorBound)
		level := make([]*big.Int, 0, 82_024)
		for i := 3; i < SmallFactorBound; i += 2 {
			if sieve[i] {
				continue
			}
			level = append(level, big.NewInt(int64(i)))
			for j := i * i; j < SmallFactorBound && j > 0; j += 2 * i {
				sieve[j] = true
			}
		}
		// Product tree: pairwise multiplication is near-linear, where a left fold is
		// quadratic (282 ms measured at this bound, against 24 ms here).
		for len(level) > 1 {
			next := make([]*big.Int, 0, (len(level)+1)/2)
			for i := 0; i < len(level); i += 2 {
				if i+1 < len(level) {
					next = append(next, new(big.Int).Mul(level[i], level[i+1]))
				} else {
					next = append(next, level[i])
				}
			}
			level = next
		}
		smallPrimorial = level[0]
	})
	return smallPrimorial
}

// smallFactor returns gcd(N, primorial) when it exceeds 1 — i.e. a witness that N has a
// prime factor below SmallFactorBound — and nil otherwise.
func smallFactor(n *big.Int) *big.Int {
	g := new(big.Int).GCD(nil, nil, n, primorial())
	if g.Cmp(bigOne) == 0 {
		return nil
	}
	return g
}

// perfectPower returns k > 1 if n == p^k for some integer p, else 0.
//
// Only PRIME k needs testing: if k = a·b then n = (p^a)^b. And because smallFactor has
// already ruled out every factor below SmallFactorBound = 2^20, any base p here exceeds
// 2^20, so k < BitLen(n)/20 — 102 candidates at the 2048-bit floor, 26 of them prime.
func perfectPower(n *big.Int) int {
	maxK := n.BitLen() / 20
	if maxK < 2 {
		return 0
	}
	for _, k := range []int{2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37, 41, 43, 47,
		53, 59, 61, 67, 71, 73, 79, 83, 89, 97, 101, 103, 107, 109, 113, 127} {
		if k > maxK {
			break
		}
		if isKthPower(n, k) {
			return k
		}
	}
	return 0
}

// isKthPower reports whether n is an exact k-th power, by binary search on the root.
func isKthPower(n *big.Int, k int) bool {
	if k == 2 {
		r := new(big.Int).Sqrt(n)
		return new(big.Int).Mul(r, r).Cmp(n) == 0
	}
	bits := (n.BitLen() + k - 1) / k
	lo := new(big.Int).Lsh(bigOne, uint(bits-1))
	hi := new(big.Int).Lsh(bigOne, uint(bits+1))
	mid, pow := new(big.Int), new(big.Int)
	one := big.NewInt(1)
	for lo.Cmp(hi) <= 0 {
		mid.Add(lo, hi).Rsh(mid, 1)
		pow.Exp(mid, big.NewInt(int64(k)), nil)
		switch pow.Cmp(n) {
		case 0:
			return true
		case -1:
			lo = new(big.Int).Add(mid, one)
		default:
			hi = new(big.Int).Sub(mid, one)
		}
	}
	return false
}

// Issuer blind-signs fee-paid publish tokens and later accepts them for spend,
// rejecting double-spends. The RSA key is generated at the edge and injected;
// the fee is charged through a callback, so this stays free of the ledger and
// ports — the token economics without the coupling.
//
// The spent set here is in-memory; the full design records spends ON-CHAIN so
// double-spend is caught network-wide (a publish Entry carries the token, and
// the chain rejects a duplicate serial the way it rejects a duplicate root).
// This is the single-issuer core the wire integration builds on.
type Issuer struct {
	rng   io.Reader
	key   *rsa.PrivateKey
	spent map[string]bool
}

// NewIssuer takes the randomness source the private-key operation blinds with
// (advisory C-2). It is INJECTED, never ambient: core is forbidden crypto/rand
// (internal/depcheck), the adapter passes crypto/rand and a sim passes a seeded
// source. The blinding factor cancels, so the signature this issuer produces is the
// same value for any rng — only the modexp's timing profile changes.
func NewIssuer(rng io.Reader, key *rsa.PrivateKey) *Issuer {
	return &Issuer{rng: rng, key: key, spent: make(map[string]bool)}
}

// Public is the issuer verification key — published so anyone can check a token.
func (i *Issuer) Public() *rsa.PublicKey { return &i.key.PublicKey }

// Issue charges the fee (via charge) to the requesting identity, then
// blind-signs the token — learning nothing about its serial. If the charge
// fails (e.g. insufficient credit), no token is minted.
//
// THE INPUT IS CHECKED BEFORE THE CHARGE, and the order is the property (advisory C-5).
// SignBlinded refuses a non-canonical or out-of-range representative, so without this
// pre-check a rejected spelling would still have taken the requester's fee — a strictly
// worse bug than the dedup bypass the refusal exists to close. The CHARGE still comes
// before the modexp, so an unfunded requester cannot make the issuer do RSA work for
// free. The only remaining charge-without-token path is a verify-after-sign fault,
// which is a hardware fault rather than anything a requester can trigger.
func (i *Issuer) Issue(charge func() error, blinded []byte) ([]byte, error) {
	if _, err := canonicalRep(blinded, i.key.N); err != nil {
		return nil, err
	}
	if err := charge(); err != nil {
		return nil, err
	}
	return SignBlinded(i.rng, i.key, blinded)
}

// Spend accepts a token for a publish: it must verify against the issuer key
// and not have been spent before. The (serial, sig) carries no link to the
// identity that paid at issuance.
func (i *Issuer) Spend(serial, sig []byte) error {
	if !Verify(&i.key.PublicKey, serial, sig) {
		return ErrBadToken
	}
	if i.spent[string(serial)] {
		return ErrDoubleSpend
	}
	i.spent[string(serial)] = true
	return nil
}

// PrewarmValidatePub builds the small-factor primorial ahead of the first key pin. It
// is optional: ValidatePub builds it on demand. Exposed so a measurement can separate
// the ONE-TIME build (~25 ms) from the PER-PIN cost (~1 ms), which are different
// budgets, and so a daemon can pay it at boot rather than on the node loop.
func PrewarmValidatePub() { primorial() }
