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
//
// WHAT THIS PAIR DOES NOT HOLD, stated because a second review found both by ablation
// rather than by reading the comment. A source gate on a call STRING sees neither the
// ORDER of the call relative to its consumer nor the VALUE its arguments carry:
//   - move the whole derivation block below `chain.New` and every gate stays green while
//     the config reads the underived literal — and the console still prints "derived to N".
//     That is not hypothetical: the bond verifier in this same file carries the #572 scar
//     for exactly that shape, sixty lines away.
//   - shadow `effByz` just for this call and the original validity-lowering blocker
//     re-opens with both gates green.
//
// So this file asserts the two structural facts that close those: the derivation precedes
// its consumer, and `effByz` is declared exactly once in the file. Anything beyond string,
// order and count here is unobserved, and a future edit should assume so.
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

	// ORDER. The derivation must precede the consumer that snapshots the value into the
	// chain config. Below it, *quorum = q still runs and still prints, but chain.Config
	// already holds the underived literal — the f=0 launch this change exists to fix is
	// not fixed, and the console says it was.
	consumer := strings.Index(s, "ch := chain.New(chain.Config{")
	if consumer < 0 {
		t.Fatal("SOURCE GATE: cannot find the chain.New(chain.Config{...}) consumer in daemon.go, so the ordering below cannot be checked at all")
	}
	if i > consumer {
		t.Fatal("SOURCE GATE: the -quorum derivation sits AFTER chain.New(chain.Config{...}) in daemon.go. The config then snapshots the UNDERIVED literal while the derivation still runs and still prints 'gather target derived to N' — the #572 shape, in this same file")
	}

	// VALUE. Exactly one effByz in the file, so the call cannot be handed a shadowed one.
	// A shadow re-opens the validity-lowering blocker with every behavioural gate green.
	// A bare `effByz := …` in a narrower scope keeps the call string byte-identical, keeps
	// the outer binding used by chain.Config, and compiles — so neither the string arm nor
	// the constructor count above sees it. It is the shape that re-opens the blocker most
	// quietly, which is why it gets its own assertion.
	if strings.Contains(s, "effByz :=") {
		t.Fatal("SOURCE GATE: daemon.go contains a bare `effByz :=` binding. A shadow in a narrower scope hands the -quorum derivation a Byzantine-sizing value that is not the one the daemon computed, re-opening the validity-lowering regime (-byzantine-quorum=false) with every behavioural gate green")
	}
	if n := strings.Count(s, "effByz, byzDefaulted := effectiveByzantineQuorum("); n != 1 {
		t.Fatalf("SOURCE GATE: daemon.go declares effByz %d times, want exactly 1 — a second binding lets the derivation read a SHADOWED Byzantine-sizing value, which re-opens the validity-lowering regime (-byzantine-quorum=false) that this argument exists to exclude", n)
	}
}
