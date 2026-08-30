// Package relaypay holds the PayWord hash-chain primitive that funds relay /
// gateway bandwidth compensation (docs/design/pod.md §7.3, certified
// 2026-08-30). It is a sender-funded incremental micropayment: the fetcher
// commits a chain root once, then reveals one preimage per forwarded increment.
// The relay verifies each preimage with a single SHA-256 and redeems the
// highest one it holds. There is no committed state, no TTP, and no new
// dependency — SHA-256 only.
//
// Why PayWord and not per-increment tokens: the certified shape (Q2) is the
// cheapest possible per-increment verify (one hash) and scales as increments
// shrink, which is the whole point of bounding the irreducible one-increment
// stiff small. A blind token per increment would cost an RSA op per KB.
//
// Two M0 invariants live OUTSIDE this primitive, at the wiring layer, because
// they are about identity and funding, not the chain itself: (i) the chain root
// binds to a blind credit under a FRESH EPHEMERAL identity, never a durable one;
// (ii) a fresh ephemeral identity and a fresh chain per session. This file is
// the pure hash-chain math; the session/funding guards are in core/node and
// core/credit.
package relaypay

import (
	"crypto/sha256"
	"errors"
)

// RelayIncrementBytes is the payload size, in bytes, of one PayWord increment —
// the amount of forwarded payload one revealed preimage authorizes.
//
// B = 4,096 bytes (4 KiB), derived analytically from the floor-box-equivalent
// measurement (docs/thinking/2026-08-30-pod-7.3-relay-compensation-design.md
// §5); no billable run was needed. The binding constraint is (b), the chain-
// state memory bound: MaxSessionBytes = 1 GiB (adapters/relay/server.go:40) is
// objectSize_max, so S = 1 GiB / 4 KiB = 262,144 increments and the FETCHER's
// committed chain state = S · 32 B = 8 MB (MB-scale, holds). The relay holds
// only one 32-B preimage; the 8 MB is fetcher-side. Constraint (a), verify
// overhead, is slack: one SHA-256 (order ~100 ns on arm64) is <<1% of the time
// to forward 4 KiB at any real relay speed. 4 KiB is the smallest power-of-two
// ≥ 3.20 KiB on a sub-chunk boundary (64 KiB / 16).
const RelayIncrementBytes = 4096

// MaxSessionBytes is the per-session payload ceiling the relay will ever forward,
// in bytes. It is the protocol floor mirrored by the relay adapter's own config
// default (adapters/relay/server.go:40, 1 GiB); the adapter may lower its own cap,
// but the accepted chain length is clamped against THIS protocol constant so the
// clamp does not depend on an adapter value core/node cannot import. 1 GiB is the
// objectSize_max the increment size B was derived against (§5 constraint (b)).
const MaxSessionBytes = 1 << 30

// MaxChainLength is S_max: the largest committed chain length a relay accepts,
// derived RELAY-SIDE from the relay's own byte ceiling and the protocol increment
// — never trusted from the fetcher. A fetcher cannot open a session claiming a
// chain longer than the relay will ever forward.
//
//	S_max = MaxSessionBytes / RelayIncrementBytes = (1 GiB) / (4 KiB) = 262,144
//
// This bounds the worst-case AdvanceTo walk to ~262K SHA-256 (~23 ms) instead of
// the attacker-chosen millions the unbounded path allowed (#644).
const MaxChainLength = MaxSessionBytes / RelayIncrementBytes

// hashLen is the SHA-256 output length; every chain link is this wide.
const hashLen = sha256.Size

// hash returns SHA-256(b). One hash is the per-increment verify cost.
func hash(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

// Chain is a fetcher-side PayWord chain over S increments. It holds the full
// list of preimages x_0 (root) … x_S so the fetcher can reveal x_k on demand.
// The fetcher-side memory cost is S × 32 B; the relay holds only one preimage.
type Chain struct {
	// links[k] is x_k. links[0] is the root (the most-hashed value); links[S]
	// is the value one hash from the random tip. Revealing runs 1 … S.
	links [][]byte
}

// BuildChain builds a PayWord chain of S increments from a random tip. It hashes
// the tip S+1 times; the most-hashed value is the root x_0. tip should be a
// fresh random value (>= 32 bytes of entropy) generated per session — reusing a
// tip across sessions would let a relay link sessions (M0 invariant (ii),
// enforced at the wiring layer, but a fresh tip is the raw material). S must be
// positive.
func BuildChain(tip []byte, S int) (*Chain, error) {
	if S <= 0 {
		return nil, errors.New("relaypay: chain length S must be positive")
	}
	if len(tip) == 0 {
		return nil, errors.New("relaypay: empty tip")
	}
	// links[S] = H(tip); links[k] = H(links[k+1]); links[0] is the root.
	links := make([][]byte, S+1)
	cur := hash(tip)
	links[S] = cur
	for k := S - 1; k >= 0; k-- {
		cur = hash(cur)
		links[k] = cur
	}
	return &Chain{links: links}, nil
}

// Len returns S, the number of increments the chain can authorize.
func (c *Chain) Len() int { return len(c.links) - 1 }

// Root returns x_0, the value committed once to the relay at session open.
func (c *Chain) Root() []byte { return c.links[0] }

// Preimage returns x_k, the value that authorizes increment k (1 <= k <= S).
// H(x_k) = x_{k-1}, so revealing x_k lets the relay advance one hash.
func (c *Chain) Preimage(k int) []byte {
	if k < 1 || k > c.Len() {
		return nil
	}
	return c.links[k]
}

// Verifier is the relay-side state for one PayWord session: the committed root,
// the committed chain length S, the highest preimage revealed so far, and the
// increment count it authorizes. It holds exactly one preimage (32 B) regardless
// of chain length. S is the DoS clamp: the walk in AdvanceTo can never exceed S
// hashes because claimedCount > S is rejected before the walk (#644).
type Verifier struct {
	held  []byte // the highest preimage verified so far; starts as the root x_0
	count int    // increments authorized: held == x_count
	s     int    // the committed chain length; count never exceeds this

	// walkSteps accumulates the SHA-256 walk steps AdvanceTo has executed on this
	// verifier. It is instrumentation for the #644 clamp ablation: a test reads
	// v.walkSteps after an oversized AdvanceTo and asserts the walk ran at most S
	// steps (removing the clamp turns that assertion RED). It is a plain field, not
	// an atomic: AdvanceTo already requires exclusive access to the verifier (it
	// mutates held/count), so the counter carries no synchronization beyond that.
	// One increment per hash is negligible against the SHA-256 itself; the
	// single-hash Advance path is not instrumented.
	walkSteps uint64
}

// NewVerifier starts a relay-side session from the committed root x_0 with the
// committed chain length S. The relay then advances one preimage per forwarded
// increment, and never past S. S must be the relay-clamped chain length (the
// caller clamps S <= S_max = MaxSessionBytes / RelayIncrementBytes before opening
// the session — never a value trusted from the fetcher). See OpenRelaySession.
func NewVerifier(root []byte, S int) *Verifier {
	held := make([]byte, len(root))
	copy(held, root)
	return &Verifier{held: held, count: 0, s: S}
}

// Len returns S, the committed chain length this verifier will authorize up to.
func (v *Verifier) Len() int { return v.s }

// Count returns the number of increments the relay is currently authorized to
// redeem (it holds x_count).
func (v *Verifier) Count() int { return v.count }

// Held returns a copy of the highest preimage the relay currently holds. This is
// the value the relay presents at settlement to redeem count × increment.
func (v *Verifier) Held() []byte {
	out := make([]byte, len(v.held))
	copy(out, v.held)
	return out
}

// Advance verifies the next preimage x_{count+1} and, if it hashes to the held
// preimage, advances one increment. One SHA-256. It rejects any value that does
// not hash to the held preimage — the one-way-hash forgery exclusion. A rejected
// preimage does not move the count.
func (v *Verifier) Advance(preimage []byte) error {
	if len(preimage) != hashLen {
		return errors.New("relaypay: preimage wrong length")
	}
	// The count never exceeds the committed chain length S. After count == S the
	// held preimage is x_S = H(tip); a fetcher who reveals the TIP would hash to
	// x_S and, without this guard, push count to S+1 — an unfunded (S+1)th
	// increment past the committed budget (the settlement cap chain relies on
	// count <= S). Reject the reveal once the chain is exhausted (#644 budget cap).
	if v.count >= v.s {
		return errors.New("relaypay: chain exhausted (count == committed length S)")
	}
	if !bytesEqual(hash(preimage), v.held) {
		return errors.New("relaypay: preimage does not hash to the held value")
	}
	v.held = cloneBytes(preimage)
	v.count++
	return nil
}

// AdvanceTo verifies a preimage claimed to reach increment claimedCount, walking
// the hash chain forward from the revealed preimage to the held value. It lets a
// fetcher pay several increments at once (reveal x_5 to jump from count 0 to 5)
// while the relay still verifies every link. It rejects a backward move and a
// claimed count that the preimage does not actually reach. On success the held
// preimage advances to the revealed value and count = claimedCount.
//
// THE #644 CLAMP: claimedCount > S (the committed chain length carried at
// NewVerifier) is REJECTED BEFORE the walk. The walk is therefore bounded to at
// most (S - count) <= S hashes, and S is itself relay-clamped to
// S_max = MaxSessionBytes / RelayIncrementBytes at OpenRelaySession. claimedCount
// is an attacker int in a bogus MsgRelayPay, but it can no longer drive work past
// S hashes per message. Without this clamp a single bogus preimage with a large
// claimedCount spins the relay for that many hashes (the PE measured ~5M hashes /
// ~0.28s on an M4 from one message). Removing the clamp turns
// TestAdvanceToClampsToChainLength RED.
func (v *Verifier) AdvanceTo(preimage []byte, claimedCount int) error {
	if len(preimage) != hashLen {
		return errors.New("relaypay: preimage wrong length")
	}
	if claimedCount <= v.count {
		return errors.New("relaypay: claimed count is not ahead of the held count")
	}
	// #644 clamp: never walk past the committed chain length. This bounds the walk
	// to at most S hashes regardless of the attacker-chosen claimedCount.
	if claimedCount > v.s {
		return errors.New("relaypay: claimed count exceeds the committed chain length S")
	}
	// Walk H forward (claimedCount - count) times; the result must be the held
	// value. x_{claimedCount} hashed (claimedCount - count) times == x_count.
	steps := claimedCount - v.count
	cur := cloneBytes(preimage)
	for i := 0; i < steps; i++ {
		cur = hash(cur)
		v.walkSteps++
	}
	if !bytesEqual(cur, v.held) {
		return errors.New("relaypay: preimage does not reach the claimed count")
	}
	v.held = cloneBytes(preimage)
	v.count = claimedCount
	return nil
}

func cloneBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// bytesEqual is a length-then-content compare; the values are public preimages,
// so no constant-time requirement (a relay comparing a preimage leaks nothing an
// attacker does not already hold).
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
