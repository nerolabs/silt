package main

import (
	"os"
	"strings"
	"testing"
)

// TestDerivedQuorumIsWiredWithBothPreconditions is a SOURCE GATE on the CALL SITE, not on
// the function. It reads daemon.go as TEXT, so it can only ever see a string and its
// position — it observes no behaviour at all and must not be read as if it did.
//
// Why it exists: the blind PE ablated the entire call site out of daemon.go and the whole
// cmd/silt package stayed GREEN, because the behavioural gate
// (TestDerivedGatherTargetTracksTheByzantineBar) exercises the pure function and never
// observes how it is wired. A wrong-argument or deleted-call defect is invisible to it —
// and a wrong argument is precisely the defect that review caught here, the missing
// Byzantine-sizing precondition.
//
// RUNTIME GATE: TestDerivedGatherTargetTracksTheByzantineBar proves the function's
// behaviour in every regime, including the Byzantine-off regime this gate keeps wired.
// This gate proves only that the daemon calls it with both preconditions and assigns the
// result; the two together are the property.
func TestDerivedQuorumIsWiredWithBothPreconditions(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot read daemon.go, so the call site is unobserved: %v", err)
	}
	s := string(src)

	const call = "effectiveQuorum(quorumSet, *quorum, useObjective, effByz, len(anchorSet))"
	if !strings.Contains(s, call) {
		t.Fatalf("SOURCE GATE: daemon.go does not call %s. The derived gather target is either unwired (the shipped literal 3 ships, tolerating f=0 at a four-anchor launch) or wired with different arguments — and dropping the effByz precondition LOWERS a validity bar in the -byzantine-quorum=false regime", call)
	}
	// The assignment must be inside the SAME if-block as the call, not merely somewhere
	// later in the file: scan from the call to the end of that block.
	i := strings.Index(s, call)
	block := s[i:]
	if end := strings.Index(block, "\n\t\t}\n"); end >= 0 {
		block = block[:end]
	}
	if !strings.Contains(block, "*quorum = q") {
		t.Fatal("SOURCE GATE: the derived value is computed but never assigned to *quorum. Every consumer (the chain config, the registry line, the revocation proposer) reads the flag pointer, so an unassigned derivation is a no-op that still prints as if it applied")
	}
}
