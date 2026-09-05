//go:build bbootstrap

package credit

// Invariant A for the R2.9a B_bootstrap instrument's two exported ledger methods.
//
// THEY ARE CLASSIFIED HERE RATHER THAN IN invariant_a_test.go BECAUSE THEY ONLY EXIST
// HERE. The instrument compiles only under the `bbootstrap` build tag
// (D-BB-BUILD-TAG, 2026-09-05), and TestInvariantA_EveryLedgerMethodClassified checks
// standingClassification in both directions — every *Ledger method needs an entry, and
// every entry needs a method — so an untagged entry would fail the reverse check in a
// default build. This init() adds them exactly when the methods are present.
//
// THE CLASSIFICATION ITSELF IS UNCHANGED. The instrument is a COUNT histogram over
// (age × log2 bytes) and nothing reads it but an operator: no accounting rule, no
// screen, no conservation rule, and certainly no standing calculation. The clock
// setter's only effect is that Register stamps account.firstSeenTick — a field the
// T-axis note has always said no standing calc reads, and that is still true; the
// export reads it for observability only. The snapshot is a PURE reader and must stay
// one: TestR29aBBootstrapSnapshotWritesNothing deep-compares the account map across it,
// because the sibling defect in this family is a reader that goes through acct() →
// Register and MINTS a 500,000 grant for every id it touches.
func init() {
	standingClassification["SetObservabilityClock"] = neutral
	standingClassification["BBootstrapPublish"] = neutral
}
