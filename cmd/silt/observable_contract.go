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
	{"serves no demand issuer key", "cmd/silt/swarm.go", "the lane-OFF refusal's distinguishing sentence (a server running no -accept-delivery-receipts), DISTINCT from the committed-binding refusal; the cloud sheet's flow_delivery_lane grades its lane-off control on it (blind PE re-review 2026-09-07)", "TestDeliveryReceiptRefusedWhenLaneOff"},
	{"NOT banked", "cmd/silt/swarm.go", "e2e: the lane-off refusal must be legible to the caller (instances 2 and 3 of the scar)", "TestDeliveryReceiptRefusedWhenLaneOff"},
	{"delivery receipt banked", "cmd/silt/swarm.go", "e2e parses credit=(\\d+) from the client line", ""},
	{"delivery receipt banked", "core/node/deliverysession.go", "R2.9: the session lane emits the SAME marker per settled receipt; e2e TestPaidDeliverySessionEndToEnd finds it", "TestPaidDeliverySessionEndToEnd"},
	{"delivery session closed", "core/node/deliverysession.go", "R2.9: the close line (reason idle/exhausted/disabled, per-session numbers only — no identity, no object; M0 log audit)", "TestPaidDeliverySessionEndToEnd"},
	{"delivery settlement: p=", "cmd/silt/numeraire.go", "R2.9 gate B-11: the S5 affordability line, every number computed from the constants", "TestAffordabilityLineIsAnnounced"},
	{"delivery receipts: ACCEPTING", "cmd/silt/daemon.go", "e2e waits for the lane announcement", ""},
	{"log: %s and above → ", "cmd/silt/daemon.go", "e2e PARSES the log path out of this line", ""},
	{"chain: committed block ", "cmd/silt/daemon.go", "e2e reCommitted; partition / equivocation / coldstart harnesses", "TestObjectiveConsensusCommitsOverTCP"},
	{"chain: restored ", "cmd/silt/daemon.go", "the depth-war regime instrumentation — PERMANENT (chain.Regime)", ""},
	{"chain: saved ", "cmd/silt/daemon.go", "the depth-war regime instrumentation — PERMANENT (chain.Regime)", ""},
	{"chain: slashed equivocator ", "cmd/silt/daemon.go", "e2e equivocation harness", "TestEquivocatorSlashedOverTCP"},
	{"adversary: equivocation complete (double-signed height ", "cmd/silt/daemon.go", "e2e equivocation harness", "TestEquivocatorSlashedOverTCP"},
	{"refusing to start", "cmd/silt/daemon.go", "e2e reRefuse", ""},
	{"need an epoch clock", "cmd/silt/daemon.go", "R2.10 / F8 (R-F8-DISABLED): the paid-lane refusal at effective EpochBlocks == 0 must name both lane flags and -epoch-blocks; e2e reRefuseLine", "TestF8_PaidLanesRefuseToStartWithoutAnEpochClock"},
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
	{"delivery anchor refused: guard full", "core/node/deliverysession.go", "the operator's WARN when an open/fund is refused at a paid-serial guard full of LIVE entries — a serve rate above the bound the cap was derived against; carries serial_guard_refusals (blind PE, 2026-09-07: guard-full arises only at open/fund, never at settle)", "TestGuardFullOpenLogsTheWarnMarker"},
	{"delivery receipt paid NO credit", "core/node/deliverysession.go", "the only signal an operator gets when a receipt settles nothing (B-9: re-homed from the retired flat lane to the session lane's refused settlement)", "TestBankedButUnpaidReceiptLogsTheWarnLine"},
	{"relay session settled", "core/node/relaytransport.go", "e2e TestPaidRelaySessionEndToEnd and the M0 log audit (TestRelaySettlementLogCarriesNoDurableField) find the settlement line by it", "TestPaidRelaySessionEndToEnd"},
	{"anchored", "core/node/relaytransport.go", "R2.14: the settlement line's reason field says the paid session was anchored (min(count, Σ face) settled); the R0.7 interim's no-anchor value is retired — an unanchored open is refused and never settles", "TestRelayAnchorsAreBoughtOnTheRelaysOwnLedger"},
	// The era observable (R-CLOUD-ERA-PROBE, freeze manifest item 19). These are the strings the
	// cloud sheet's 13b-delivery-settlement row reads to tell "era-4 dark" from "keys
	// off-commitment". Both the ACTIVE and the NOT-ON-THIS-CHAIN form are registered: the row's
	// whole content is that the two RENDER DIFFERENTLY, so deleting either half to make a build
	// green would restore exactly the ambiguity the row closes.
	{"head version: v%d", "cmd/silt/chainstatus.go", "the head block's rule era; a Tester reading a live net's chain.cbor could not see it before this row", "TestChainStatusDistinguishesAnEra4ChainFromADarkOne"},
	{"era-%d (v%d): ACTIVE", "core/chain/erastate.go", "the era is live on this chain, and the height its first block landed at", "TestChainStatusDistinguishesAnEra4ChainFromADarkOne"},
	{"era-%d (v%d): NOT ON THIS CHAIN", "core/chain/erastate.go", "the OFFLINE dark form: it names the state without claiming a readiness-tally fact chain-status cannot observe", "TestChainStatusDistinguishesAnEra4ChainFromADarkOne"},
	{"era-%d (v%d): PENDING", "core/chain/erastate.go", "the one-epoch-of-notice window — the tally locked in, no block of the era yet; visible only to a caller holding a chain.Chain", "TestEraStateDrivesEveryPhaseFromCommittedState"},
	{"era-%d (v%d): DARK", "core/chain/erastate.go", "the dark form for a caller that CAN see the tally, distinct from the offline form above", "TestEraLineDistinguishesAnUnobservableTallyFromADarkOne"},
	{"max atts:     %d, first at height %d", "cmd/silt/chainstatus.go", "max_h len(blocks[h].Atts), the live attestation-carrier width two certifications name as unmeasured", "TestCensusMeasuresMaxAttsOnANonUniformChain"},
	{"max atts:     0 — measured across every block", "cmd/silt/chainstatus.go", "the MEASURED zero: a bare 0 would be unreadable against a figure nobody computed", "TestChainStatusNarratesAMeasuredZeroCarrier"},
	// The DECLARED half of the era pair (item 19's second clause). 13b reads these off the
	// daemon's start-up log, and it needs BOTH: the declaration alone cannot say whether this
	// chain is dark, and the observed era alone cannot say whether a dark chain is healthy. Both
	// the declaration and the healthy-dark verdict are registered for that reason — deleting
	// either restores the ambiguity the row closes.
	{"chain: era support — this BUILD declares", "core/chain/eradeclared.go", "the daemon's declared max block era: 13b tells \"era-4 dark, binary fine\" from \"wrong build\" by reading it beside the observed era", "TestEraStartupLinesDeclareTheBUILDNotTheFlags"},
	{"AHEAD — this chain has not activated", "core/chain/eradeclared.go", "the verdict that a dark chain under an era-4 build is HEALTHY — the false alarm this row exists to prevent", "TestStartupLinesSeparateAHealthyDarkChainFromAWrongBuild"},
}
