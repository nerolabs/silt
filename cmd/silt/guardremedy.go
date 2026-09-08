package main

// The per-store operator remedies for a guard-store failure (C1 / B1, blind PE ruling
// RULING-c1-demand-v2-and-flat-leg-retirement-a290bff-2026-09-08.md).
//
// adapters/guardstore is one adapter opened on TWO files, and its refusals — the format
// bump's ErrLegacyFormat, ErrCorrupt, a load error — are refuse-to-start for both. The
// refusal is the SAFE posture in both cases. What is not shared is the REMEDY, and
// getting that wrong on one of the two files is a money pump:
//
//   - paidserials.log guards delivery/relay payouts. Through the RC the credit ledger is
//     EPHEMERAL (D-FP2-SCOPE): balances reset at the same restart, so the file guards
//     payouts whose credits no longer exist and removing it costs nothing.
//   - creditspent.log guards spent PUBLISH credits. The publish issuer key persists
//     (adapters/diskissuer, <store>/issuer/issuer.key), so a credit it signed stays
//     spendable across every restart. Clearing that guard on its own re-opens every held
//     credit for a second spend — the F-4 pump, measured at
//     core/node/r213b_creditspent_test.go: one 50,000-credit publish redeemed
//     43,750 + 43,750 against one burn. The ratified rule is to rotate the publish key
//     AND clear creditspent.log TOGETHER (R-CREDITSPENT-UNBOUNDED, owner call 6,
//     D-TRUE-UP-CALLS-2026-09-07; docs/design/m0.md §10, docs/decisions.md), the same
//     recovery core/node's logCreditGuardRefusal already prints for a full guard.
//
// So the adapter's sentinel states the CONDITION and names no remedy; the remedy is
// attached here, at the two open sites, where the operator reads it. Pinned by
// TestGuardStoreRemedyTextIsSafePerStore (the text) and
// TestDaemonPairsEachGuardStoreWithItsOwnRemedy (the file-to-remedy pairing).
const (
	remedyPaidSerials = "remedy for paidserials.log: stop the daemon, remove the file named above, and " +
		"restart. Through the RC the credit ledger is ephemeral (D-FP2-SCOPE), so balances reset at the " +
		"same restart and this guard protects payouts whose credits no longer exist: clearing it costs " +
		"nothing. That stops being true the moment the ledger persists."

	remedyCreditSpent = "remedy for creditspent.log: do NOT clear this file on its own. The publish issuer " +
		"key persists across restarts, so every credit it signed stays spendable and an empty guard " +
		"re-opens each held credit for a second spend. Rotate the publish key AND clear creditspent.log " +
		"together, in one stop: delete <store-dir>/issuer/issuer.key and the file named above, then " +
		"restart (R-CREDITSPENT-UNBOUNDED, owner call 6, D-TRUE-UP-CALLS-2026-09-07)."
)
