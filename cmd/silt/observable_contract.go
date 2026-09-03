package main

// ObservableContract is the S5 set: the announced operator strings an operator, a
// script, or an acceptance test depends on. "An announced log line is an OBSERVABLE
// CONTRACT" (S5). Changing one is a BREAKING CHANGE to the operator interface: add an
// entry when you add an announced marker; never delete one to make a build green.
//
// Why this exists (the Tester's scar `scar-observable-log-contract`, THIRD-TIME RULE
// fired 2026-09-03): three times an upstream change broke an announced string while every
// unit test stayed green, because only the spawned-process tier (e2e, which `-short`
// skips) ever looked at what the binary printed. This registry moves the RENAME half of
// that scar to the unit tier: TestObservableContractStringsAreStillEmitted asserts each
// literal is still in its emitting source.
//
// What it does NOT cover, so nobody over-trusts it: it proves the string is still IN THE
// SOURCE, never that it is still REACHABLE. A new early return upstream of the emit site
// (instance 2 of the scar) is invisible here. Therefore: keep the e2e assertion for every
// registered marker (Asserter), and when a diff inserts a new precondition on a CLI or
// daemon path, ask which registered markers sit downstream of it. UNGATED: the
// `%!w(<nil>)`-family formatting defect (a `%w` on a variable the guard allows to be nil)
// is not statically detectable and is not checked here.
type ContractedString struct {
	Marker   string // the literal substring that must appear in the source (the SOURCE form, e.g. a format verb)
	File     string // repo-relative path of the emitting file
	Why      string // who depends on it
	Asserter string // the test that observes it at runtime (a `func TestX(` in the tree), or "" if none
}

// ObservableContract — seed entries verified present at origin/main 748594f (2026-09-03).
var ObservableContract = []ContractedString{
	{"freeload: ON", "cmd/silt/daemon.go", "e2e reFreeload; the role announcement (instance 1 of the scar)", "TestFreeloadRoleSeparation"},
	{"archive: ON", "cmd/silt/daemon.go", "e2e reArchive", "TestArchiveTierAnnouncesRetention"},
	{"NOT banked", "cmd/silt/swarm.go", "e2e: the lane-off refusal must be legible to the caller (instances 2 and 3 of the scar)", "TestDeliveryReceiptRefusedWhenLaneOff"},
	{"delivery receipt banked", "cmd/silt/swarm.go", "e2e parses credit=(\\d+) from the client line", ""},
	{"delivery receipt banked", "core/node/demandrole.go", "e2e findInLog on the daemon log file", ""},
	{"delivery receipts: ACCEPTING", "cmd/silt/daemon.go", "e2e waits for the lane announcement", ""},
	{"log: %s and above → ", "cmd/silt/daemon.go", "e2e PARSES the log path out of this line", ""},
	{"chain: committed block ", "cmd/silt/daemon.go", "e2e reCommitted; partition / equivocation / coldstart harnesses", "TestObjectiveConsensusCommitsOverTCP"},
	{"chain: restored ", "cmd/silt/daemon.go", "the depth-war regime instrumentation — PERMANENT (chain.Regime)", ""},
	{"chain: saved ", "cmd/silt/daemon.go", "the depth-war regime instrumentation — PERMANENT (chain.Regime)", ""},
	{"chain: slashed equivocator ", "cmd/silt/daemon.go", "e2e equivocation harness", "TestEquivocatorSlashedOverTCP"},
	{"adversary: equivocation complete (double-signed height ", "cmd/silt/daemon.go", "e2e equivocation harness", "TestEquivocatorSlashedOverTCP"},
	{"refusing to start", "cmd/silt/daemon.go", "e2e reRefuse", ""},
	{"contradict each other", "cmd/silt/daemon.go", "e2e contradictory-flags refusal", "TestContradictoryContentFlagsRefused"},
	{"-care needs a registry", "cmd/silt/daemon.go", "e2e economy_repair", "TestCareWithoutRegistryRefusesToStart"},
	{"re-bootstrapped: recovered from an empty routing table", "cmd/silt/daemon.go", "e2e bootstrap self-heal (#281)", "TestBootstrapRetryRecoversColdStartRace"},
	{"bootstrapped (", "cmd/silt/daemon.go", "e2e reBootstrapped", ""},
	{"registry: chain-backed, serving ", "cmd/silt/daemon.go", "e2e reRegistry parses the registry address", ""},
	{"registry-only: ", "cmd/silt/daemon.go", "e2e registry-only mode", "TestRegistryOnlyMode"},
	{"peer: ", "cmd/silt/daemon.go", "e2e rePeer parses the peer id@addr", ""},
	{"silt:v1:", "core/link/link.go", "the PRODUCT's link scheme; asserted across e2e", "TestPublishCommitFetchOverTCP"},
	{"siltcare:", "core/link/link.go", "the care-link scheme", "TestRepairBountyPaysOnTheWire"},
	{"column %d", "cmd/silt/swarm.go", "e2e reHolderCol parses per-column holders", "TestSwarmHoldersReportsPerColumnPlacement"},
	{"stripe repair pending confirmation", "core/node/repair.go", "e2e economy_repair", "TestRepairBountyPaysOnTheWire"},
	{"proposed on-chain revocation of ", "cmd/silt/daemon.go", "e2e reRevoked", "TestChainRevocationCommitsOverTCP"},
	{" proposal correctly REJECTED by ", "cmd/silt/daemon.go", "e2e proposal_reject (forge-block / lowbond-propose)", "TestForgedBlockRejectedOverTCP"},
	{"ui: http://", "cmd/silt/daemon.go", "e2e publishflood parses the UI URL", "TestConcurrentUIPublishesAllSucceed"},
	{"delivery receipt paid NO credit", "core/node/demandrole.go", "the only signal an operator gets when a banked receipt settles nothing", "TestBankedButUnpaidReceiptLogsTheWarnLine"},
}
