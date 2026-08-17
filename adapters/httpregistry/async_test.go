package httpregistry_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/httpregistry"
	"github.com/nerolabs/silt/ports"
)

// asyncReg is a registry that implements the async publish capability (PublishAsync +
// PublishStatus) — so httpregistry.Serve routes /publish through the 202 + poll path. Its
// background "gather" either commits an accepted entry after commitDelay OR (gatherFail set)
// records a terminal failure; it can also refuse synchronously (refuse != nil). Those are the
// three behaviours the async path must preserve: accept→commit, terminal fast-fail, refusal.
type asyncReg struct {
	mu          sync.Mutex
	committed   map[ports.Hash]ports.Entry
	failed      map[ports.Hash]error
	refuse      error         // if set, PublishAsync returns it synchronously (a refusal)
	gatherFail  error         // if set, the background gather records a TERMINAL failure (no commit)
	commitDelay time.Duration // simulate the async gather
}

func (r *asyncReg) PublishAsync(_ context.Context, e ports.Entry) error {
	if r.refuse != nil {
		return r.refuse // synchronous refusal — the client must get this, not a 202+timeout
	}
	r.mu.Lock()
	delete(r.failed, e.Root) // a retry clears the prior attempt's terminal failure
	r.mu.Unlock()
	go func() { // accepted: resolve in the background, like the real gather
		time.Sleep(r.commitDelay)
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.gatherFail != nil {
			r.failed[e.Root] = r.gatherFail
			return
		}
		r.committed[e.Root] = e
	}()
	return nil
}

func (r *asyncReg) PublishStatus(_ context.Context, root ports.Hash) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.committed[root]; ok {
		return true, nil
	}
	if e, ok := r.failed[root]; ok {
		return false, e
	}
	return false, nil
}

// Publish must NOT be reached when PublishAsync is present (the handler prefers async).
func (r *asyncReg) Publish(context.Context, ports.Entry) error {
	return errors.New("sync Publish should not be called on an async registry")
}

func (r *asyncReg) Lookup(_ context.Context, root ports.Hash) (ports.Entry, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.committed[root]
	return e, ok, nil
}

func (r *asyncReg) All(context.Context) ([]ports.Entry, error) { return nil, nil }

// The async happy path: a publish is ACCEPTED (202) and commits in the background; the
// client's Publish blocks until it commits (via polling Lookup), not on a held connection.
// This is what removes the flat 10s guillotine on the ~1.5 MB genesis gather (#286 Layer 1).
func TestAsyncPublish_AcceptThenPollCommits(t *testing.T) {
	reg := &asyncReg{committed: map[ports.Hash]ports.Entry{}, failed: map[ports.Hash]error{}, commitDelay: 400 * time.Millisecond}
	addr, shutdown, err := httpregistry.Serve("127.0.0.1:0", reg)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()
	c := httpregistry.NewClient("http://" + addr)
	ctx := context.Background()
	e := ports.Entry{Root: ports.HashBytes([]byte("async-ok")), FileSize: 1}

	// Publish blocks until the background commit lands (poll), then returns nil.
	if err := c.Publish(ctx, e); err != nil {
		t.Fatalf("async publish should commit via poll, got %v", err)
	}
	if _, ok, _ := c.Lookup(ctx, e.Root); !ok {
		t.Fatal("entry was not committed after a successful async publish")
	}
}

// A refusal must surface SYNCHRONOUSLY — the async path must not swallow it into a 202 that
// the client then poll-times-out on. This is what preserves the token-less / durable-
// Publisher / double-spend refusal controls the privacy & economy flows rely on.
func TestAsyncPublish_RefusalIsSynchronous(t *testing.T) {
	reg := &asyncReg{committed: map[ports.Hash]ports.Entry{}, failed: map[ports.Hash]error{}, refuse: ports.ErrDupPublish}
	addr, shutdown, err := httpregistry.Serve("127.0.0.1:0", reg)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()
	c := httpregistry.NewClient("http://" + addr)
	ctx := context.Background()
	e := ports.Entry{Root: ports.HashBytes([]byte("async-refuse")), FileSize: 1}

	start := time.Now()
	err = c.Publish(ctx, e)
	if !errors.Is(err, ports.ErrDupPublish) {
		t.Fatalf("a refusal must surface synchronously as ErrDupPublish, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("a refusal must be immediate, not a poll-timeout (took %s)", time.Since(start))
	}
}

// lossyReg models the field's dropped fire-and-forget submit burst (run
// 82bcd2b-39478 / the rotation-wait oracle in core/node): the FIRST submission
// is accepted but never commits — the entry is stranded — and only a
// RE-submission of the same root lands it. The pre-fix client submitted once
// and then only polled, so it polled out its whole budget on a stranded entry;
// the fixed client re-submits inside the poll window and recovers. This test is
// the failing-first regression for that re-submit (V5).
type lossyReg struct {
	mu        sync.Mutex
	submits   map[ports.Hash]int
	committed map[ports.Hash]ports.Entry
}

func (r *lossyReg) PublishAsync(_ context.Context, e ports.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.submits[e.Root]++
	if r.submits[e.Root] >= 2 { // only a re-submission lands: the first burst was "lost"
		r.committed[e.Root] = e
	}
	return nil
}

func (r *lossyReg) PublishStatus(_ context.Context, root ports.Hash) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.committed[root]
	return ok, nil
}

func (r *lossyReg) Publish(context.Context, ports.Entry) error {
	return errors.New("sync Publish should not be called on an async registry")
}

func (r *lossyReg) Lookup(_ context.Context, root ports.Hash) (ports.Entry, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.committed[root]
	return e, ok, nil
}

func (r *lossyReg) All(context.Context) ([]ports.Entry, error) { return nil, nil }

func TestAsyncPublish_ResubmitInsidePollWindowRecoversLostBurst(t *testing.T) {
	restore := httpregistry.SetPublishPollKnobsForTest(10*time.Millisecond, 2*time.Second, 100*time.Millisecond)
	defer restore()
	reg := &lossyReg{submits: map[ports.Hash]int{}, committed: map[ports.Hash]ports.Entry{}}
	addr, shutdown, err := httpregistry.Serve("127.0.0.1:0", reg)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()
	c := httpregistry.NewClient("http://" + addr)
	e := ports.Entry{Root: ports.HashBytes([]byte("lost-burst")), FileSize: 1}

	if err := c.Publish(context.Background(), e); err != nil {
		t.Fatalf("a stranded entry must be recovered by the poll loop's re-submit, not polled out to the budget: %v", err)
	}
	reg.mu.Lock()
	n := reg.submits[e.Root]
	reg.mu.Unlock()
	if n < 2 {
		t.Fatalf("commit landed with %d submission(s) — the re-submit never fired and the test proved nothing (anti-vacuity)", n)
	}
}

// A gather that reaches a TERMINAL failure this round (e.g. no quorum, before mutual standing
// is earned) must surface FAST — not poll out the whole accept→commit budget. This is what
// keeps a publish-retry-until-standing caller (bond earned-standing, on-chain revocation)
// retrying promptly; without it those e2e flows stall for the full 180 s budget on every
// pre-standing attempt. The client learns the terminal outcome from PublishStatus, not from a
// silent Lookup miss it cannot tell apart from a still-progressing gather.
func TestAsyncPublish_TerminalFailureFastFails(t *testing.T) {
	reg := &asyncReg{
		committed:   map[ports.Hash]ports.Entry{},
		failed:      map[ports.Hash]error{},
		commitDelay: 200 * time.Millisecond,
		gatherFail:  errors.New("no quorum this round"),
	}
	addr, shutdown, err := httpregistry.Serve("127.0.0.1:0", reg)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()
	c := httpregistry.NewClient("http://" + addr)
	e := ports.Entry{Root: ports.HashBytes([]byte("async-fail")), FileSize: 1}

	start := time.Now()
	err = c.Publish(context.Background(), e)
	if err == nil {
		t.Fatal("a terminal gather failure must surface as an error, not a silent success")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("a terminal gather failure must fast-fail, not poll to the budget (took %s)", time.Since(start))
	}
}
