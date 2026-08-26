// Package smtspike is the build-immutable #7 gate on adopting
// github.com/pokt-network/smt as the D-TIERING state-root keystone.
//
// The library call in docs/thinking/2026-08-26-keystone-smt-library-call.md
// recommends the library on DOCUMENTARY evidence — quoted source, not executed
// code. This package is the executed half. It carries no product code and is
// imported by nothing: the dependency lives behind these tests until the
// keystone build lands.
//
// What the tests must establish, from that document's gate:
//
//  1. A specific key can be proven ABSENT against a committed root.
//  2. An absence proof for a PRESENT key FAILS. This is the half that matters —
//     a library that "passes" absence for a present key is silently unsound for
//     every exclusion consumer (duplicate-publish reject, serial-replay reject,
//     the sharded can't-lie-by-omission proof).
//  3. Produce + verify cost on the floor box (build-immutable #8).
//
// If (2) does not hold, the recommendation is void and the JMT port returns.
package smtspike
