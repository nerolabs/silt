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
// written by RecordBondChallenge (credit.go). That writer PREDATES R2.9a entirely, is
// stamped from the bond auditor's own request counter rather than from a wall clock, and
// fires only for a validator answering a storage-bond challenge — never for a fetcher.
// It is not part of this instrument and this decision does not touch it.
//
// THE CONSEQUENCE, WHICH IS THE POINT. Before R2.9a the ledger held
// (identity, cumulative bytes) for each requester. R2.9a added the WHEN. In a default
// build the when is gone again: TestR29aDefaultBuildStampsNoFirstTouchOnRegister pins it
// in BOTH builds, because it asserts on the un-injected state that is the only state a
// default build can reach.

// bbootstrapState is empty in a default build. See the tagged declaration in
// bbootstrap.go for the two injected time sources it holds under the tag.
type bbootstrapState struct{}

// stampFirstTouch does nothing in a default build. There is no observability clock to
// read and no age axis to place an identity on.
func (l *Ledger) stampFirstTouch(*account) {}
