package main

// The per-store operator remedies for a guard-store failure (C1 / B1).
//
// adapters/guardstore is one adapter opened on TWO files, and its refusals — the format
// bump's ErrLegacyFormat, ErrCorrupt, a load error — are refuse-to-start for both. The
// refusal is the SAFE posture in both cases. What is not shared is the REMEDY, and
// getting that wrong on one of the two files is a money pump:
//
// - paidserials.log guards delivery/relay payouts. Through the RC the credit ledger is
// EPHEMERAL: balances reset at the same restart, so the file guards
// payouts whose credits no longer exist and removing it costs nothing.
// - creditspent.log guards spent PUBLISH credits. The publish issuer key persists
// (adapters/diskissuer, <store>/issuer/issuer.key), so a credit it signed stays
// spendable across every restart. Clearing that guard on its own re-opens every held
// credit for a second spend — the pump, measured at
// core/node/creditspent_test.go: one 50,000-credit publish redeemed 43,750 + 43,750
// against one burn. The rule is to rotate the publish key AND clear creditspent.log
// TOGETHER, the same recovery core/node's logCreditGuardRefusal already prints for
// a full guard.
//
// So the adapter's sentinel states the CONDITION and names no remedy; the remedy is
// attached here, at the two open sites, where the operator reads it. Pinned by
// TestGuardStoreRemedyTextIsSafePerStore (the text) and
// TestDaemonPairsEachGuardStoreWithItsOwnRemedy (the file-to-remedy pairing).
const (
	remedyPaidSerials = "remedy for paidserials.log: stop the daemon, remove the file named above, and " +
		"restart. Through the RC the credit ledger is ephemeral, so balances reset at the " +
		"same restart and this guard protects payouts whose credits no longer exist: clearing it costs " +
		"nothing. That stops being true the moment the ledger persists."

	remedyCreditSpent = "remedy for creditspent.log: do NOT clear this file on its own. The publish issuer " +
		"key persists across restarts, so every credit it signed stays spendable and an empty guard " +
		"re-opens each held credit for a second spend. Rotate the publish key AND clear creditspent.log " +
		"together, in one stop: delete <store-dir>/issuer/issuer.key and the file named above, then " +
		"restart."
)
