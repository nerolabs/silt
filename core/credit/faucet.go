package credit

// R2.12 — the faucet rate limit: a lazily-refilled token bucket over an INJECTED
// monotonic time source. It is LOCAL ADMISSION CONTROL on the starter grant, not a
// consensus rule: no other node's validity depends on this node's grant rate, and two
// honest nodes may run different rates forever without diverging (economist advisory
// ADVISORY-boulder2-economy-definitions-2026-09-03 §2.1).
//
// WHAT IT KEYS ON. An injected ports.MonotonicNanos — never the ledger's epoch watermark
// (peer-influenced, F8) and never chainEpoch() (identically 0 on a chainless node, which
// would deny the faucet forever). The daemon injects Go's monotonic reading via
// time.Since; a sim injects its virtual clock. core/ imports no clock (internal/depcheck).
//
// WHAT IT IS NOT. Not a goroutine, not a timer: the level is recomputed from the source
// on every take, so it is deterministic under a fake source and costs nothing idle.
type faucet struct {
	capacity int64 // bucket capacity, in grants
	refill   int64 // grants added per interval
	interval int64 // nanoseconds
	now      func() int64

	level    int64 // current tokens, in [0, capacity]
	lastFill int64 // the source reading the level was last brought up to date at
}

func newFaucet(capacity, refill, intervalNanos int64, now func() int64) *faucet {
	f := &faucet{capacity: capacity, refill: refill, interval: intervalNanos, now: now}
	f.level = capacity // a fresh bucket is FULL: the burst an honest cohort join needs
	f.lastFill = now()
	return f
}

// fill brings the level up to date. Refill is granted per WHOLE elapsed interval, and
// the reference point advances by whole intervals only, so partial intervals are never
// lost and never double-counted; the level saturates at capacity.
func (f *faucet) fill() {
	t := f.now()
	if t <= f.lastFill || f.interval <= 0 {
		return
	}
	intervals := (t - f.lastFill) / f.interval
	if intervals <= 0 {
		return
	}
	f.level += intervals * f.refill
	if f.level > f.capacity {
		f.level = f.capacity
	}
	f.lastFill += intervals * f.interval
}

// take consumes one token if one is available. It reports whether the grant may be
// applied; a false is a RETRYABLE denial (the caller keeps the identity grant-pending).
func (f *faucet) take() bool {
	f.fill()
	if f.level <= 0 {
		return false
	}
	f.level--
	return true
}

// Level reports the current token count after bringing it up to date (telemetry).
func (f *faucet) Level() int64 {
	f.fill()
	return f.level
}
