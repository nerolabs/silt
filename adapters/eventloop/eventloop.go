// Package eventloop serializes work onto a single goroutine — the real
// world's stand-in for the sim scheduler's single-threadedness. Core
// node code is written lock-free on the assumption that all its entry
// points (transport deliveries, timer callbacks, API calls) run one at
// a time; in the sim the scheduler guarantees that, and over real
// sockets this loop does.
//
// Because it is the ONE serialization point, it is also the one place to
// measure where the node's single thread actually goes. Andrew's idea
// (2026-08-15): rather than reconstruct a suspected hot path in a synthetic
// harness, TIME THE REAL TASKS HERE and let the evidence name the function
// that eats the thread — or, worst case, hangs it. This is legitimate in the
// adapter layer (real wall-clock is fine here); the deterministic core stays
// free of ambient time. Every task carries a label (inbound deliveries by
// message kind), so the per-label latency IS a goroutine-budget decomposition:
// which handler dominates, and a watchdog that names a task still in-flight
// past a deadline (a hang), with a stack dump of exactly where it is stuck.
// All instrumentation is optional — the zero value is off.
package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type task struct {
	label string
	fn    func()
}

type labelStat struct {
	count int64
	total time.Duration
	max   time.Duration
}

type Loop struct {
	mu      sync.Mutex
	cond    *sync.Cond
	queue   []task
	stopped bool
	// OnPanic, if set, is called when a posted task panics. The loop
	// recovers and keeps running, so one bad task — e.g. handling a
	// malformed frame that reaches a node-side decoder — fails that task
	// rather than killing the node's single thread (Gate 1 / anti-persona
	// #14). Set it before Run. If nil the panic is still contained but
	// silent; wire it to a logger in production so the drop is observable
	// (tenets S3/V4). Set-once before Run, so no lock is needed.
	OnPanic func(r any)

	// ── Instrumentation (Andrew's timing-for-evidence idea). Set before Run;
	// zero value = off. Timing uses Now (real wall-clock — adapter layer only). ──

	// Now returns the real time; defaults to time.Now when nil.
	Now func() time.Time
	// SlowThreshold, if > 0, logs any single task that runs at least this long
	// via OnSlow — the "function taking unreasonable time" case.
	SlowThreshold time.Duration
	OnSlow        func(label string, d time.Duration)
	// HangThreshold, if > 0, arms a watchdog: a task still in-flight this long
	// is reported ONCE via OnHang with an all-goroutine stack dump (the loop's
	// Run goroutine shows exactly where it is blocked) — the "hangs completely"
	// case. Needs Run to have been called (the watchdog rides its lifetime).
	HangThreshold time.Duration
	OnHang        func(label string, d time.Duration, stack []byte)
	// SummaryEvery, if > 0, emits a per-label {count,total,max} snapshot via
	// OnSummary every interval and resets — the goroutine-budget decomposition
	// over each window (name the dominant atom from real load).
	SummaryEvery time.Duration
	OnSummary    func(window time.Duration, stats map[string]LabelSummary)

	// watchdog / accumulator state
	stats map[string]labelStat // guarded by mu; updated on the loop goroutine
	// in-flight task, published for the watchdog goroutine via atomics so the
	// loop never holds mu while running a task.
	curStart atomic.Int64  // task start UnixNano; 0 = idle
	curSeq   atomic.Uint64 // bumped per task, so a hang is reported once
	curLabel atomic.Pointer[string]
	reported atomic.Uint64 // last curSeq reported as a hang
	watchdog chan struct{} // closed on Stop to end the background goroutine
}

// LabelSummary is one line of the per-window decomposition.
type LabelSummary struct {
	Count int64
	Total time.Duration
	Max   time.Duration
}

func New() *Loop {
	l := &Loop{stats: make(map[string]labelStat)}
	l.cond = sync.NewCond(&l.mu)
	return l
}

func (l *Loop) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

// Post enqueues fn from any goroutine, tagged with label for the latency
// instrumentation (use the message kind for inbound deliveries; a short
// constant like "timer"/"commit"/"api" otherwise). Never blocks.
func (l *Loop) Post(label string, fn func()) {
	l.mu.Lock()
	if !l.stopped {
		l.queue = append(l.queue, task{label: label, fn: fn})
		l.cond.Signal()
	}
	l.mu.Unlock()
}

// Run executes posted functions in order until Stop. Call from exactly
// one goroutine; that goroutine becomes "the node's thread".
func (l *Loop) Run() {
	l.startWatchdog()
	for {
		l.mu.Lock()
		for len(l.queue) == 0 && !l.stopped {
			l.cond.Wait()
		}
		if len(l.queue) == 0 && l.stopped {
			l.mu.Unlock()
			return
		}
		t := l.queue[0]
		l.queue = l.queue[1:]
		l.mu.Unlock()
		l.run(t)
	}
}

// run executes one task under panic recovery so a panicking task cannot
// unwind through Run and stop the node's thread, timing it for the
// instrumentation hooks.
func (l *Loop) run(t task) {
	instr := l.SlowThreshold > 0 || l.HangThreshold > 0 || l.SummaryEvery > 0
	var start time.Time
	if instr {
		start = l.now()
		l.curLabel.Store(&t.label)
		l.curStart.Store(start.UnixNano())
		l.curSeq.Add(1)
	}
	defer func() {
		if instr {
			l.curStart.Store(0)
			d := l.now().Sub(start)
			if l.SlowThreshold > 0 && d >= l.SlowThreshold && l.OnSlow != nil {
				l.OnSlow(t.label, d)
			}
			if l.SummaryEvery > 0 {
				l.mu.Lock()
				s := l.stats[t.label]
				s.count++
				s.total += d
				if d > s.max {
					s.max = d
				}
				l.stats[t.label] = s
				l.mu.Unlock()
			}
		}
		if r := recover(); r != nil && l.OnPanic != nil {
			l.OnPanic(r)
		}
	}()
	t.fn()
}

// startWatchdog spawns the background hang-watcher + summary emitter if either
// is armed. It ends when Stop closes l.watchdog.
func (l *Loop) startWatchdog() {
	if l.HangThreshold <= 0 && l.SummaryEvery <= 0 {
		return
	}
	l.mu.Lock()
	l.watchdog = make(chan struct{})
	done := l.watchdog
	l.mu.Unlock()

	tick := l.HangThreshold
	if l.SummaryEvery > 0 && (tick <= 0 || l.SummaryEvery < tick) {
		tick = l.SummaryEvery
	}
	if tick <= 0 {
		tick = time.Second
	}
	// Poll a few times per threshold so a hang is caught promptly, not a full
	// interval late.
	poll := tick / 4
	if poll <= 0 {
		poll = tick
	}
	go func() {
		t := time.NewTicker(poll)
		defer t.Stop()
		var lastSummary time.Time
		for {
			select {
			case <-done:
				return
			case <-t.C:
				now := l.now()
				if l.HangThreshold > 0 && l.OnHang != nil {
					if startNanos := l.curStart.Load(); startNanos != 0 {
						d := now.Sub(time.Unix(0, startNanos))
						seq := l.curSeq.Load()
						if d >= l.HangThreshold && l.reported.Load() != seq {
							l.reported.Store(seq)
							label := ""
							if p := l.curLabel.Load(); p != nil {
								label = *p
							}
							buf := make([]byte, 1<<16)
							buf = buf[:runtime.Stack(buf, true)]
							l.OnHang(label, d, buf)
						}
					}
				}
				if l.SummaryEvery > 0 && l.OnSummary != nil {
					if lastSummary.IsZero() {
						lastSummary = now
					}
					if now.Sub(lastSummary) >= l.SummaryEvery {
						l.emitSummary(now.Sub(lastSummary))
						lastSummary = now
					}
				}
			}
		}
	}()
}

func (l *Loop) emitSummary(window time.Duration) {
	l.mu.Lock()
	if len(l.stats) == 0 {
		l.mu.Unlock()
		return
	}
	out := make(map[string]LabelSummary, len(l.stats))
	for k, s := range l.stats {
		out[k] = LabelSummary{Count: s.count, Total: s.total, Max: s.max}
	}
	l.stats = make(map[string]labelStat)
	l.mu.Unlock()
	l.OnSummary(window, out)
}

// Stop drains the queue and then lets Run return.
func (l *Loop) Stop() {
	l.mu.Lock()
	l.stopped = true
	if l.watchdog != nil {
		close(l.watchdog)
		l.watchdog = nil
	}
	l.cond.Broadcast()
	l.mu.Unlock()
}
