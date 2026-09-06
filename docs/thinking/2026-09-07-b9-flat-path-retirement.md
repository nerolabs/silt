# 2026-09-07 — B-9: retiring the flat delivery receipt (token spent at redeem) now that the session lane is built

**Context / trigger:** R2.9's node half (PR #760) built the anchored session lane beside the v2 flat path and
left the flat path callable, because nine test files pin red-team scars on it. The build-questions certification
(2026-09-04 §2.2 point 1) requires the flat path RETIRED: "any fetcher left on the flat path re-creates the
break" (a server prefers suppression above B = f). The blind PE on #760 (item 4) measured the coexistence cost:
one face buys 40 v3 increments AND a v2 demand unit if the flat receipt is presented after the v3 open
(`R-V2-V3-DEMAND-DILUTION`); payment is safe either order (the shared guard). `SubmitDeliveryReceipt` has zero
production callers since `swarm receipt` moved to the session flow.

**Evidence:** `core/node/demandrole.go` `handleDeliveryReceipt` (the v2 handler: `Bank.Redeem` then the flat
ledger leg); `core/node/node.go` dispatch of `MsgDeliveryReceipt`; `core/demand/demand.go` `Bank.Redeem`, the
spent set, `SubmittedReceipt`, `Ack` (v2); the seven node test files and one demand test file that drive the
handler or the redeem (inventory below); `cmd/silt/observable_contract.go` markers emitted by the v2 handler
("delivery receipt banked" in demandrole.go, "delivery receipt paid NO credit").

**Options weighed:**
- **(A) Refuse at the handler, keep the primitive and the flat ledger leg.** Smallest diff; leaves
  `Bank.Redeem` and `RedeemDeliveryCreditReason` as production code with no production caller — the exact
  "unused-correct code" that rots (build-immutable #6's grep-first lesson in reverse).
- **(B) Retire the NODE path and the DEMAND primitive in one PR; stage the LEDGER's flat leg for a second PR.**
  Delete `handleDeliveryReceipt`, `SubmitDeliveryReceipt`, `Bank.Redeem` + the spent set + `SubmittedReceipt` +
  v2 `Ack`/`DeliveryReceipt`; `MsgDeliveryReceipt` keeps its kind NUMBER (appended kinds are pinned) and is
  refused with a named reason. Re-home every property the retired code pinned to the open path (table below).
  `RedeemDeliveryCreditReason` stays as the ledger guard's test surface with its production caller gone
  (documented), because ~15 credit test files drive the guard through it and re-homing them is its own PR.
- **(C) Retire everything at once.** Two PRs' worth of scar re-homing in one diff; a blind PE cannot hold it.

**Decision:** (B), STAGED: this PR retires the NODE path (the handler refuses with a named reason; the fetcher-side
submit is deleted; the WARN marker re-homes to the session lane; the node and sim tests that drove the wire path
re-home to open + settle). The `core/demand` primitive (`Bank.Redeem`, `SubmittedReceipt`, v2 `Ack`) stays as a
leaf library with its own unit tests, documented as having no production caller: its ~25 pure tests pin
token-format and spent-set properties, and deleting them unattended at the end of a long session is where the
one accidental test drop of this session already happened. The primitive and the ledger's flat leg
(`RedeemDeliveryCreditReason`, driven by ~15 credit test files) retire together in one attended PR.

**Re-homing table (property → new home; the ablation stays):**
| Retired test | Property | New home |
|---|---|---|
| `TestRTC3_RestartDoesNotRePayTheSameWireReceipt` | a restart is not an eviction: the same token cannot pay twice | the same ANCHOR cannot OPEN twice across a restart (durable guard; `errDeliveryAnchorSpent`) |
| `TestRTC3_DegenerateCommittedKeyIsRefusedByThePinAndTheLane` | a degenerate committed key is refused by the pin and by the lane | pin unchanged; the LANE = the open path refuses (`errDeliveryNoIssuerKey` / `errDeliveryAnchorInvalid`) |
| `TestG4_UnwitnessedReceiptLeavesTheSelfMintAlone` | an unwitnessed/forged receipt reverses nothing | a forged anchor at open, and a settle naming no session, reverse nothing (already `TestReceiptBindsSessionAnchorsAndCount` + a forged-anchor arm) |
| `TestR05NodePathConservation` | conservation through the node handler incl. eviction reversal | the same walk through open + settle |
| `TestBankedButUnpaidReceiptLogsTheWarnLine` | a settled-nothing receipt gets a WARN with the reason | the session lane's refused settlement logs the SAME marker ("delivery receipt paid NO credit") — contract entry re-homed |
| `TestComposedExpiryBoundary_*` (3) | an evicted serial is refused upstream; both layers close together; same fingerprint at two epochs does not re-date | an EXPIRED anchor is refused at open by the keyset AND the guard; the boundary walk on the open path |
| `TestC3_InboundReceiptsCostOHardnessChecksNotOPerMessage`, `TestC3_ADifferentCommittedKeyStillPaysFullAdmission` | hardness at admission, not per message | the same on inbound `MsgDeliveryOpen` (the RSA verify per open is the priced path; hardness memo unchanged) |
| `TestCohortKeyIsADenialOnEveryShippedLane`, `TestDemandLaneOutlivesTheWindowAndARestart` (r04b_c3_gates) | cohort key denial on every lane; the lane outlives the window | the open path IS a lane: the cycle becomes withdraw → open → settle |
| `TestC1_AnIssuerThatReturnsADudIsRefusedAtWithdrawal` | withdrawal-side | unchanged (withdrawal is the anchor purchase) |
| demand: `TestRTC3_BankSpentSetIsBoundedAndExpirySwept`, `TestRTC3_SpentIsKeyedByTheTokenNotTheSerial`, `TestRTC3_OversizedSerialIsRefusedAtTheWireAndAtTheBank` | the spent set's bounds and key | the paid-serial guard already pins bounded/expiry-swept/keyed-by-token (`delivery_serial_guard_test.go`, `paidKey`); the oversized serial is refused at `UnmarshalSessionOpen` and at the ledger (`ReasonAnchorMalformed`) |
| demand: `TestRTC3_EpochBindingHoldsUnderOneKey`, `TestRTC3_DegenerateKeysAreRefusedEverywhere` | token-format | unchanged (Withdraw/Unblind/VerifyInWindow stay) |
| demand: `TestAbortLeavesTokenReusable` (fairexchange) | an aborted exchange leaves the token reusable | B-13's certified rewrite: an aborted SESSION forfeits nothing (deposit) and the retry server is paid from a fresh anchor |

**Pressure-test:** the `R-V2-V3-DEMAND-DILUTION` residual closes by construction (no second surface). The sim
`TestDemandWashCostsRealFees` (v2 cost-to-wash artifact) is retired in favour of the v3 twin that ships. The
e2e refusal tests (`TestPaidDeliveryLaneRefusesWithoutACommittedKeyBinding`, `TestDeliveryReceiptRefusedWhenLaneOff`)
already drive `swarm receipt`, which is v3.
