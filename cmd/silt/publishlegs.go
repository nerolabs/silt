package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// publishLegs times the legs of one `swarm add`, in the order they run:
//
//	join     dial the -peers and bootstrap the client node
//	issuers  fetch the canonical issuer ranking from a chain-holding peer
//	keys     fetch the issuer keys not already cached
//	credits  mint one prepaid publish credit per validator
//	sign     gather the k blind signatures for the publish token
//	stage    encrypt and chunk the object, build its manifest
//	place    scatter the chunks to their holders and confirm placement
//	submit   POST the entry until the registry accepts it
//	commit   wait for the entry to land in a committed block
//
// Each leg is the time since the previous mark, so the legs sum to the total. A
// leg that did not run (no -token-quorum, or a publish that failed early) is
// absent rather than zero.
//
// Marks land on the client node's loop while String runs on the caller's
// goroutine, which can still be racing the loop after an operation timeout; the
// mutex keeps that read whole.
type publishLegs struct {
	mu          sync.Mutex
	start, last time.Time
	legs        []publishLeg
}

type publishLeg struct {
	name string
	d    time.Duration
}

func newPublishLegs(start time.Time) *publishLegs {
	return &publishLegs{start: start, last: start}
}

func (p *publishLegs) mark(name string) { p.at(name, time.Now()) }

func (p *publishLegs) at(name string, t time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.legs = append(p.legs, publishLeg{name, t.Sub(p.last)})
	p.last = t
}

// String renders "join 0.312s issuers 0.101s … total 6.004s". The total runs to
// the last mark, so it is the sum of the legs shown.
func (p *publishLegs) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var b strings.Builder
	for _, l := range p.legs {
		fmt.Fprintf(&b, "%s %.3fs ", l.name, l.d.Seconds())
	}
	fmt.Fprintf(&b, "total %.3fs", p.last.Sub(p.start).Seconds())
	return b.String()
}
