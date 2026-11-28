package main

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestAClientOperationIsBoundedByStallingNotBySize: an operation still placing shards outlives
// its ceiling, one that stops making progress fails one ceiling later, and a finished one
// returns at once. The fixed ceiling killed every publish of a 2.5 GB film at 6m39s.
func TestAClientOperationIsBoundedByStallingNotBySize(t *testing.T) {
	const ceiling, poll = 120 * time.Millisecond, 10 * time.Millisecond

	var n atomic.Int64
	done := make(chan struct{})
	go func() { // progress for 4 ceilings, then finish
		for i := 0; i < 48; i++ {
			n.Add(1)
			time.Sleep(poll)
		}
		close(done)
	}()
	start := time.Now()
	if err := awaitWithProgress(done, ceiling, poll, n.Load); err != nil {
		t.Fatalf("an operation still making progress was killed after %s: %v", time.Since(start), err)
	}
	if time.Since(start) < 3*ceiling {
		t.Fatal("CONTROL: the operation finished inside one ceiling, so this proved nothing")
	}

	stuck := make(chan struct{})
	start = time.Now()
	err := awaitWithProgress(stuck, ceiling, poll, func() int64 { return 7 })
	if err == nil || !strings.Contains(err.Error(), "no progress") {
		t.Fatalf("a stalled operation: err=%v", err)
	}
	if el := time.Since(start); el < ceiling || el > 4*ceiling {
		t.Fatalf("a stalled operation failed after %s, want about one ceiling (%s)", el, ceiling)
	}

	finished := make(chan struct{})
	close(finished)
	if err := awaitWithProgress(finished, ceiling, poll, func() int64 { return 0 }); err != nil {
		t.Fatal(err)
	}
}
