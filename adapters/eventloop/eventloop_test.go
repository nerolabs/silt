package eventloop

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// A task that panics must not stop the loop: the panic is reported and the
// next task still runs. This is the "node still up" outcome — one bad task
// (e.g. handling a malformed frame that reaches a decoder) fails that
// task, not the node's single thread.
func TestPanicInTaskDoesNotStopLoop(t *testing.T) {
	l := New()
	var mu sync.Mutex
	var panics []any
	l.OnPanic = func(r any) {
		mu.Lock()
		panics = append(panics, r)
		mu.Unlock()
	}

	go l.Run()
	defer l.Stop()

	done := make(chan struct{})
	l.Post("bad", func() { panic("bad frame") })
	l.Post("next", func() { close(done) }) // must still run after the panic
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(panics) != 1 || panics[0] != "bad frame" {
		t.Fatalf("panic not reported through OnPanic, got %v", panics)
	}
}

// With no OnPanic hook set, a panicking task is still contained (the loop
// keeps running) — it is just silent. Proves the nil-hook path is safe.
func TestPanicContainedWithoutHook(t *testing.T) {
	l := New()
	go l.Run()
	defer l.Stop()

	done := make(chan struct{})
	l.Post("boom", func() { panic("boom") })
	l.Post("next", func() { close(done) })
	<-done
}

// A task slower than SlowThreshold is reported through OnSlow with its label —
// the "function taking unreasonable time" evidence (Andrew's timing idea).
func TestSlowTaskReported(t *testing.T) {
	l := New()
	l.SlowThreshold = 10 * time.Millisecond
	type hit struct {
		label string
		d     time.Duration
	}
	got := make(chan hit, 1)
	l.OnSlow = func(label string, d time.Duration) { got <- hit{label, d} }

	go l.Run()
	defer l.Stop()

	l.Post("BondChallenge", func() { time.Sleep(30 * time.Millisecond) })

	select {
	case h := <-got:
		if h.label != "BondChallenge" {
			t.Fatalf("slow task label = %q, want BondChallenge", h.label)
		}
		if h.d < 10*time.Millisecond {
			t.Fatalf("slow task duration = %v, want >= 10ms", h.d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnSlow never fired for a 30ms task at a 10ms threshold")
	}
}

// A fast task must NOT be reported slow — no false positives below threshold.
func TestFastTaskNotReported(t *testing.T) {
	l := New()
	l.SlowThreshold = 50 * time.Millisecond
	fired := make(chan struct{}, 1)
	l.OnSlow = func(string, time.Duration) { fired <- struct{}{} }

	go l.Run()
	defer l.Stop()

	done := make(chan struct{})
	l.Post("quick", func() {})
	l.Post("marker", func() { close(done) })
	<-done

	select {
	case <-fired:
		t.Fatal("OnSlow fired for a sub-threshold task")
	case <-time.After(50 * time.Millisecond):
	}
}

// A task still in-flight past HangThreshold is reported ONCE via OnHang, with a
// stack dump — the "hangs completely" case, caught while it is still stuck.
func TestHangWatchdogReportsStuckTask(t *testing.T) {
	l := New()
	l.HangThreshold = 40 * time.Millisecond
	type hit struct {
		label string
		stack string
	}
	got := make(chan hit, 4)
	l.OnHang = func(label string, _ time.Duration, stack []byte) {
		got <- hit{label, string(stack)}
	}

	go l.Run()
	defer l.Stop()

	release := make(chan struct{})
	l.Post("StuckHandler", func() { <-release })

	select {
	case h := <-got:
		if h.label != "StuckHandler" {
			t.Fatalf("hang label = %q, want StuckHandler", h.label)
		}
		if !strings.Contains(h.stack, "eventloop") {
			t.Fatalf("hang stack should mention the loop goroutine; got:\n%s", h.stack)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("OnHang never fired for a task blocked past the 40ms threshold")
	}
	close(release)

	// Reported exactly once for the one stuck task, not every poll.
	time.Sleep(120 * time.Millisecond)
	if n := len(got); n != 0 {
		t.Fatalf("hang reported %d extra times; want once total", n)
	}
}

// The per-window summary accumulates count/total/max per label — the
// goroutine-budget decomposition that names the dominant handler.
func TestSummaryDecomposition(t *testing.T) {
	l := New()
	l.SummaryEvery = 30 * time.Millisecond
	got := make(chan map[string]LabelSummary, 1)
	l.OnSummary = func(_ time.Duration, stats map[string]LabelSummary) {
		select {
		case got <- stats:
		default:
		}
	}

	go l.Run()
	defer l.Stop()

	l.Post("BondChallenge", func() { time.Sleep(15 * time.Millisecond) })
	l.Post("TokenRequest", func() {})
	l.Post("TokenRequest", func() {})

	select {
	case stats := <-got:
		if stats["BondChallenge"].Count != 1 {
			t.Fatalf("BondChallenge count = %d, want 1", stats["BondChallenge"].Count)
		}
		if stats["TokenRequest"].Count != 2 {
			t.Fatalf("TokenRequest count = %d, want 2", stats["TokenRequest"].Count)
		}
		if stats["BondChallenge"].Total < 15*time.Millisecond {
			t.Fatalf("BondChallenge total = %v, want >= 15ms", stats["BondChallenge"].Total)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnSummary never fired")
	}
}
