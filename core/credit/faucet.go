package credit

import "github.com/nerolabs/silt/ports"

// R2.12 — the faucet rate limit: a lazily, CONTINUOUSLY accruing token bucket over an
// INJECTED monotonic time source, gating the ledger's starter grant. It is LOCAL ADMISSION
// CONTROL, not a consensus rule: no other node's validity depends on this node's grant rate,
// and two honest nodes may run different rates forever without diverging (economist
// advisory ADVISORY-boulder2-economy-definitions-2026-09-03 §2.1).
//
// WHAT IT KEYS ON. An injected ports.MonotonicNanos — never the ledger's epoch watermark
// (peer-influenced, F8) and never chainEpoch() (identically 0 on a chainless node, which
// would deny the faucet forever). The daemon injects Go's monotonic reading via time.Since;
// a sim injects its virtual clock. core/ imports no clock (internal/depcheck).
//
// CONTINUOUS ACCRUAL, NOT A PERIODIC TOP-UP (economist advisory
// ADVISORY-R2.12-faucet-rate-limit-defaults-2026-09-05 §2.1). Tokens accrue in proportion to
// elapsed time at `refill` per `interval`, with the sub-token remainder carried in integer
// nanoseconds so nothing is lost to truncation. A cohort of 120 arriving at a 100-token
// bucket waits minutes for the next tokens, not until the next hour boundary; the
// sustained rate and the Sybil price are the same either way, the honest experience is not.
//
// WHAT IT IS NOT. Not a goroutine, not a timer: the level is recomputed from the source on
// every take, so it is deterministic under a fake source and costs nothing idle. Not a
// bound on the MINT: a denial is retryable, so a patient farm recovers every deferred grant
// — R2.12 bounds dN/dt, the RATE of fresh grants, never N (blind PE design ruling
// RULING-R2.12-faucet-rate-limit-design-2026-09-05 S2). It is a soft, disclosed deterrent
// standing in for the unbuilt structural cost of identity (docs/network-durability.md
// §4/§5), and it is labelled as such.
type faucet struct {
	capacity int64 // bucket capacity, in grants
	refill   int64 // grants accrued per interval
	interval int64 // nanoseconds
	now      ports.MonotonicNanos

	level    int64 // whole tokens, in [0, capacity]
	carry    int64 // accrued nanoseconds × refill not yet worth a whole token
	lastFill int64 // the source reading the level was last brought up to date at
}

// newFaucet builds a bucket, or returns nil — UNLIMITED — for any non-positive capacity,
// refill or interval. A zero interval or refill would otherwise drain once and never refill
// (the PE measured 104 simulated days at level 0), which is the permanent denial the whole
// design exists to avoid; refusing the configuration belongs to the caller that owns the
// operator's flags (cmd/silt), and the constructor's part is to never build a bucket that
// cannot refill.
func newFaucet(capacity, refill, intervalNanos int64, now ports.MonotonicNanos) *faucet {
	if capacity <= 0 || refill <= 0 || intervalNanos <= 0 || now == nil {
		return nil
	}
	return &faucet{capacity: capacity, refill: refill, interval: intervalNanos, now: now,
		level: capacity, lastFill: now()} // a fresh bucket is FULL: the burst a cohort join needs
}

// fill brings the level up to date. Accrual is `elapsed × refill / interval`, integer, with
// the remainder carried; the level saturates at capacity and the carry is dropped when it
// does (a full bucket does not bank future tokens). A source that does not advance, or
// steps backward, accrues nothing.
func (f *faucet) fill() {
	t := f.now()
	if t <= f.lastFill {
		return
	}
	elapsed := t - f.lastFill
	f.lastFill = t
	if f.level >= f.capacity {
		f.carry = 0
		return
	}
	// Overflow guard: once elapsed × refill would exceed what fills the bucket from empty,
	// the bucket is simply full; no need to multiply large numbers.
	if elapsed/f.interval >= f.capacity/f.refill+1 {
		f.level, f.carry = f.capacity, 0
		return
	}
	acc := f.carry + elapsed*f.refill
	tokens := acc / f.interval
	f.carry = acc - tokens*f.interval
	f.level += tokens
	if f.level >= f.capacity {
		f.level, f.carry = f.capacity, 0
	}
}

// take consumes one token if one is available. A false is a RETRYABLE denial: the caller
// keeps the identity grant-pending and retries on its next spend.
func (f *faucet) take() bool {
	f.fill()
	if f.level <= 0 {
		return false
	}
	f.level--
	return true
}

// Level reports the current whole-token count after bringing it up to date (telemetry).
func (f *faucet) Level() int64 {
	f.fill()
	return f.level
}
