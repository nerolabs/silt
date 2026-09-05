//go:build !bbootstrap

package credit

// THE DEFAULT BUILD. This file is the WHOLE of the R2.9a B_bootstrap instrument in a
// silt binary built without the `bbootstrap` tag: an empty struct and an empty method.
// There is no histogram type, no census reader, no clock setter, no age stamping and no
// -bbootstrap flag anywhere in the binary (D-BB-BUILD-TAG, ratified 2026-09-05;
// docs/decisions.md).
//
// WHY THESE TWO DECLARATIONS SURVIVE AND NOTHING ELSE DOES. A build tag cannot split a
// struct's fields or delete a call from an untagged function, so exactly two references
// in credit.go cannot be removed by tagging alone:
//
//   - Ledger holds the instrument's state in ONE field, `bb bbootstrapState`. Untagged
//     that type is empty, so the field costs zero bytes and there is nothing to inject.
//   - Register calls l.stampFirstTouch(a) — the one place an account is constructed, and
//     therefore the one place a first-touch stamp could be written. Untagged it is an
//     empty body: the compiler inlines it away and Register is byte-for-byte its
//     pre-R2.9a self.
//
// WHAT IS PRESERVED, DELIBERATELY. account.firstSeenTick still exists and is still
// written by RecordBondChallenge (credit.go). That writer PREDATES R2.9a entirely and
// this decision does not touch it.
//
// ITS TICK IS A WALL CLOCK. This comment said the opposite until 2026-09-05 — "the bond
// auditor's own request counter rather than a wall clock" — and that was false in four
// places at once. core/node/bondaudit.go stamps uint64(n.clock.Now())+1 and the daemon's
// node clock is adapters/walltime, so the value is time.Now().UnixNano()+1. It fires on
// a -validator node for that node's own id and for every BONDED peer that answers a
// challenge; it never fires for an unbonded fetcher, and a non-validator daemon never
// calls it at all. Measured by core/node's
// TestR29aBondAuditStampsAWallClockNanosecondNotACounter.
//
// THE CONSEQUENCE, STATED HONESTLY. Before R2.9a the ledger held
// (identity, cumulative bytes) for each requester. R2.9a added the WHEN on the SERVE
// path, for every requester, unconditionally. In a default build that serve-path when is
// gone: TestR29aDefaultBuildStampsNoFirstTouchOnRegister pins it in BOTH builds, because
// it asserts on the un-injected state that is the only state a default build can reach.
//
// WHAT IS NOT GONE, and is filed rather than denied: on a -validator node an identity
// that is both a bonded peer and a fetcher still carries
// (identity, cumulative fetched bytes, first-seen WALL-CLOCK nanosecond) in a default
// build, via the bond stamp. Open residual R-BB-BOND-STAMP-TUPLE (ROADMAP R2.9a). It is
// narrow — bonded peers, not the fetcher population — it predates R2.9a, and it is NOT
// closed here: the retention surface it feeds (DecayStale, BondMaxAge) is research-gated.

// bbootstrapState is empty in a default build. See the tagged declaration in
// bbootstrap.go for the two injected time sources it holds under the tag.
type bbootstrapState struct{}

// stampFirstTouch does nothing in a default build. There is no observability clock to
// read and no age axis to place an identity on.
func (l *Ledger) stampFirstTouch(*account) {}
