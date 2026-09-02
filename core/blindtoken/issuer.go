package blindtoken

import (
	"crypto/rsa"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
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
)

// ErrBadPubKey rejects an issuer public key that is not a well-formed RSA key.
var ErrBadPubKey = errors.New("blindtoken: malformed issuer public key")

// ValidatePub is the ONE definition of "this is an RSA public key" for every lane
// that holds an issuer key. It is called at the wire boundary (ParsePub), at the
// keyset door (demand.Keyset.Put), and defensively before every modexp, so a
// degenerate key is a legible refusal at the earliest boundary that sees it and can
// never be a panic at the latest.
//
// N must be odd (every RSA modulus is a product of odd primes; this alone rejects
// 0 and every even degenerate), at least MinModulusBits and at most MaxModulusBits.
// E must be odd (an even exponent is not invertible mod φ(N), so no signature could
// ever verify), strictly greater than 1 (E == 1 makes the "signature" the message
// itself), and at most MaxPubExp.
func ValidatePub(pub *rsa.PublicKey) error {
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
	if pub.E <= 1 || pub.E%2 == 0 || int64(pub.E) > MaxPubExp {
		return fmt.Errorf("%w: public exponent %d is not an odd integer in (1, %d]",
			ErrBadPubKey, pub.E, int64(MaxPubExp))
	}
	return nil
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
	key   *rsa.PrivateKey
	spent map[string]bool
}

func NewIssuer(key *rsa.PrivateKey) *Issuer {
	return &Issuer{key: key, spent: make(map[string]bool)}
}

// Public is the issuer verification key — published so anyone can check a token.
func (i *Issuer) Public() *rsa.PublicKey { return &i.key.PublicKey }

// Issue charges the fee (via charge) to the requesting identity, then
// blind-signs the token — learning nothing about its serial. If the charge
// fails (e.g. insufficient credit), no token is minted.
func (i *Issuer) Issue(charge func() error, blinded []byte) ([]byte, error) {
	if err := charge(); err != nil {
		return nil, err
	}
	return SignBlinded(i.key, blinded), nil
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
