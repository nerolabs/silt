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
//   - recordFetched calls l.stampFirstFetch(a) — the one place account.fetchedBytes is
//     written, and therefore the one place a first-FETCH stamp could be written
//     (G-BB-24; the stamp used to sit in Register, which every ledger path reaches
//     through acct()). Untagged it is an empty body: the compiler inlines it away and
//     both Register and recordFetched are byte-for-byte their pre-R2.9a selves.
//
// THE CONSEQUENCE, STATED HONESTLY. Before R2.9a the ledger held
// (identity, cumulative bytes) for each requester. R2.9a added the WHEN on the SERVE
// path, for every requester, unconditionally. In a default build that serve-path when is
// gone: TestR29aDefaultBuildStampsNoFirstTouchOnRegister pins it in BOTH builds, because
// it asserts on the un-injected state that is the only state a default build can reach.
//
// AND THE BOND PATH WRITES NO `WHEN` EITHER, since 2026-09-05. This comment used to say
// that account.firstSeenTick was "preserved, deliberately" as RecordBondChallenge's own
// mechanism, first calling its tick a request counter (false — it is a wall-clock
// nanosecond, uint64(n.clock.Now())+1 over adapters/walltime) and then filing the
// resulting (identity, cumulative fetched bytes, first-seen wall-clock nanosecond) tuple
// on bonded validator peers as residual R-BB-BOND-STAMP-TUPLE. Nothing read that field in
// any build: DecayStale reads lastBondTick, Reputation reads neither, the census reads
// firstFetchTick. A retained `when` no decided function needs is SURPLUS under T-DONT3
// prong (a) (D-DONT3-READING), so the write and the field are deleted (G-BB-28) and the
// residual is CLOSED. Gates: TestR29aBondChallengeStampsNoFirstTouch (the inversion) and
// TestR29aRetentionReadsLastBondTickInNanoseconds (retention untouched, and in the unit
// BondMaxAge needs). core/node's TestR29aBondAuditStampsAWallClockNanosecondNotACounter
// still measures the auditor's tick, because lastBondTick's unit is what retention
// depends on.

// bbootstrapState is empty in a default build. See the tagged declaration in
// bbootstrap.go for the two injected time sources it holds under the tag.
type bbootstrapState struct{}

// stampFirstFetch does nothing in a default build. There is no observability clock to
// read and no age axis to place an identity on, so account.firstFetchTick stays 0
// ("unset") for every requester however much it fetches.
func (l *Ledger) stampFirstFetch(*account) {}
