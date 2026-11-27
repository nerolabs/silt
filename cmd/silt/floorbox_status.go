package main

import (
	"sync"
	"time"
)

// floorBoxState is a -floor-box daemon's audit progress, written by its audit cycle and read by
// the web UI: the operator's checkpoint, how far the box has followed the chain from it, and its
// latest verdict. It holds consensus facts only — heights and verdicts — and nothing about who
// fetched what.
type floorBoxState struct {
	mu  sync.Mutex
	cur floorBoxInfo
	on  bool
}

// floorBoxInfo is the status page's floorBox block.
type floorBoxInfo struct {
	Checkpoint  uint64 `json:"checkpoint"`  // the operator's -ws-checkpoint height
	Following   uint64 `json:"following"`   // the pin: the newest block this box has accepted
	LastVerdict string `json:"lastVerdict"` // ACCEPTED, REFUSED, STALL or WAITING
	LastHeight  uint64 `json:"lastHeight"`
	Accepted    int    `json:"accepted"`
	Refused     int    `json:"refused"`
	Stalls      int    `json:"stalls"`
	Reason      string `json:"reason,omitempty"` // the last refusal's or stall's reason
	AtUnix      int64  `json:"atUnix"`
}

func (f *floorBoxState) start(checkpoint uint64) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.on = true
	f.cur.Checkpoint, f.cur.Following = checkpoint, checkpoint
}

func (f *floorBoxState) record(verdict string, height, pin uint64, reason string) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cur.LastVerdict, f.cur.LastHeight, f.cur.Following = verdict, height, pin
	f.cur.Reason = reason
	f.cur.AtUnix = time.Now().Unix()
	switch verdict {
	case "ACCEPTED":
		f.cur.Accepted++
	case "REFUSED":
		f.cur.Refused++
	case "STALL":
		f.cur.Stalls++
	}
}

// info is a copy of the current progress, or nil when this daemon is not a floor box or has not
// started its audit.
func (f *floorBoxState) info() *floorBoxInfo {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.on {
		return nil
	}
	out := f.cur
	return &out
}
