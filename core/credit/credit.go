// Package credit is the v1 ledger. It runs TWO economies that earlier
// versions conflated — and the conflation was the wash-serving hole:
//
//   - BALANCES (RecordServe → 1 byte served = 1 credit, minus a publish
//     fee). Still self-reported and DELIBERATELY GAMEABLE: two colluding
//     nodes can ping-pong a chunk to mint credit and nothing here
//     notices. That is fine — balances fund the anti-spam publish fee and
//     drive the observatory's who-earns/who-freeloads metrics. They are
//     NOT a security boundary and never gate consensus.
//
//   - STANDING (Reputation → the number the chain gates writes on). This
//     is NO LONGER self-reported. It is built only on evidence a Sybil
//     cannot fabricate: challenged, identity-bound held storage
//     (RecordBondChallenge, backed by core/bond) and passed storage
//     audits (core/node/por.go). Wash-serving moves balances but buys
//     ZERO standing — which is what closes the reputation→quorum-capture
//     path (threat-catalog D1/D3): standing now costs real disk, not
//     chatter. See Reputation and RecordBondChallenge.
package credit

import (
	"sort"

	"github.com/nerolabs/silt/ports"
)

type account struct {
	balance      int64
	servedBytes  int64
	fetchedBytes int64
	auditsPassed int
	auditsFailed int
	// Storage-bond standing — the Sybil cost. bondedBytes is the size of
	// the identity-bound bond (core/bond) this node last PROVED it holds
	// under a random challenge; it is the only large, unforgeable input to
	// Reputation, so N identities cost N real bonds on real disk. bondFails
	// counts challenges it could not answer.
	//
	// TIME (T axis), stated honestly: the T factor is shipped for standing
	// RETENTION only — DecayStale + BondMaxAge (below) retire standing that is
	// not *continuously* re-proven, so a released plot cannot coast. There is
	// NO acquisition-time accrual: full standing is granted on the first passing
	// bond challenge, so acquisition is priced by D (real bonded bytes) alone —
	// a zero-age identity that proves the bytes reaches full standing at once.
	// A time-of-ACQUISITION ramp is deferred: a bare age gate is pre-farmable
	// (the coin-age anti-pattern), and the only sound form is a continuous
	// bond-anchored VDF (M1+). lastBondTick drives the retention decay.
	//
	// THE BOND PATH WRITES NO FIRST-TOUCH STAMP. Until 2026-09-05 RecordBondChallenge
	// also wrote a firstSeenTick: the wall-clock nanosecond of the first challenge an
	// identity answered. Nothing read it in any build. DecayStale reads lastBondTick,
	// Reputation reads neither, and the census reads firstFetchTick. A retained `when`
	// that no decided function needs is SURPLUS under T-DONT3 prong (a)
	// (D-DONT3-READING, docs/decisions.md), so the write and the field are deleted
	// (G-BB-28; residual R-BB-BOND-STAMP-TUPLE CLOSED). Gate:
	// TestR29aBondChallengeStampsNoFirstTouch.
	//
	// lastBondTick is the ONE tick a bond challenge writes, and it is a WALL-CLOCK
	// NANOSECOND: core/node/bondaudit.go passes uint64(n.clock.Now())+1 over the
	// daemon's adapters/walltime clock. The unit is load-bearing. DecayStale subtracts
	// it from a `now` off the same clock and compares the difference against
	// BondMaxAge = 300 * ports.Second (core/node/node.go); a counter here would never
	// exceed that age and would silently disable retention. Gate:
	// TestR29aRetentionReadsLastBondTickInNanoseconds.
	//
	// firstFetchTick is the account's FIRST-FETCH stamp (R2.9a; bbootstrap.go),
	// written from the injected observability clock at the one place fetchedBytes is
	// written. It is read for OBSERVABILITY ONLY — the B_bootstrap histogram's age
	// axis — and by NO standing calculation: it is not an acquisition-age gate, and
	// the T-axis note above stands unchanged. It reaches the wire only as a coarse
	// age bucket, never as a value.
	//
	// IN A DEFAULT BUILD NOTHING WRITES IT AT ALL. recordFetched calls
	// stampFirstFetch, which is a no-op declared in bbootstrap_off.go unless the
	// binary was built with the `bbootstrap` tag AND the operator passed -bbootstrap
	// (D-BB-BUILD-TAG, docs/decisions.md). So a default silt node stamps nothing on
	// the SERVE path: no fetcher is given a first-fetch time by having fetched.
	//
	// IT IS STAMPED ON THE FETCH PATH, NOT IN Register (G-BB-24,
	// R-BB-STAMP-BY-ANY-PATH). Register is reached through acct() by every ledger
	// path — bond audit, PoR grading, bounty payment, false-repair slash — so a stamp
	// there recorded first touch by ANY path and over-stated the age of every
	// identity that is also a DHT participant. recordFetched is the one place
	// fetchedBytes is written, so it is the one place a first-FETCH stamp belongs.
	bondedBytes    int64
	bondFails      int
	firstFetchTick uint64 // first FETCH; read by the B_bootstrap export ONLY, never by a standing calc
	lastBondTick   uint64
	// equivocations counts PROVEN consensus double-signs (core/chain). It is
	// the gravest offense — an attack on consensus itself — so it does not
	// merely dent standing, it buries it below any threshold forever.
	equivocations int
	// falseRepairs counts PROVEN false repair claims (core/repairproof): a
	// caretaker claimed a durability bounty for a shard that fails the public
	// correctness recompute. Deliberate but bounded fraud — a large finite dent,
	// not the permanent burial equivocation earns.
	falseRepairs int
	// repairsDone counts the shard-repairs THIS node was paid a bounty for as the
	// repairer (incremented in PayBounty where the repairer's balance is credited).
	// The per-OBJECT repair count lives on objectEscrow.repairs; this is the
	// per-NODE dual the repair-work observability needs (the escrow count cannot
	// tell you WHO did the work). bountyEarned is the lifetime credits earned that
	// way, so an operator can separate serve revenue from repair revenue in the
	// margin panel. Both are observability only — read-side balance-economy
	// accounting, NEVER an input to Reputation or any standing/conservation rule.
	repairsDone  int64
	bountyEarned int64
}

// Ledger implements ports.CreditLedger plus the observability the sim's
// economy scenario reports on.
type Ledger struct {
	fee      int64
	grant    int64
	accounts map[ports.NodeID]*account
	order    []ports.NodeID // registration order: deterministic iteration
	// rootOwner binds each bond root to the first identity that proved it, so
	// a bond root builds standing for AT MOST ONE identity. A colluding
	// operator pointing N identities at one shared plot therefore earns one
	// bond's standing, not N — each identity needs its own distinct plot
	// (distinct secret ⇒ distinct root). Honest identities never collide.
	rootOwner map[ports.Hash]ports.NodeID

	// escrow holds each object's durability reserve, keyed by the object's
	// root (its Merkle identity). It is the H7/S7 durability budget: prepaid
	// credit that pays repair bounties, topped up by an auto-skim of the
	// object's own serving revenue. It lives in the BALANCE economy — it
	// moves the credit unit between balances and reserves — and NEVER feeds
	// Reputation. That firewall is the one load-bearing H7 invariant (the
	// durability budget must confer ZERO consensus standing, or one physical
	// copy of a shared shard could answer for N pledges); it is asserted
	// structurally by the Invariant-A guard, which classifies every escrow
	// press `neutral`. See escrow.go.
	escrow map[ports.Hash]*objectEscrow

	// provisional tracks each object-aware serve's self-credit per
	// (requester, root) so a later witnessed receipt can SUPERSEDE it
	// instead of stacking on it (the PoD conservation rule — delivery.go).
	// provOrder is the deterministic FIFO for the cap eviction (B2: no map
	// iteration in core). It holds pointers so a lane removed at redeem can be
	// tombstoned in place (nil) in O(1) without a slice scan; provIndex gives
	// each live lane's current position for that O(1) tombstone. Dead tombstones
	// are dropped by an amortized-O(1) compaction (delivery.go), which keeps the
	// slice bounded even on the redeem-heavy path where the eviction loop never
	// runs (RT-DELIV-1/1b/2 fix).
	//
	// provHead is the FIFO front CURSOR: a logical index into provOrder marking
	// the oldest slot not yet dropped by eviction. Eviction advances provHead and
	// nils the dropped slot rather than re-slicing (provOrder[1:]), which would
	// shift every survivor's absolute position and silently invalidate provIndex
	// (the RT-DELIV-3-adjacent desync fuzz found at seed 0xdeadbeef0002 step
	// 13154). With the cursor, provIndex holds absolute positions that a
	// front-drop never touches, so eviction stays amortized O(1) and never
	// rewrites a survivor's index. Compaction rebuilds the slice and resets
	// provHead to 0.
	provisional map[provKey]*provisionalServe
	provOrder   []*provKey
	provIndex   map[provKey]int
	provHead    int

	// paidSerial is the R0.4b CROSS-SERVER double-redeem guard. It records every
	// demand-token SERIAL that has already had a completed, paid delivery redeem on
	// this ledger, together with the server that collected it and the token's ISSUING
	// EPOCH. The conserved leg of RedeemDeliveryCredit pays ONLY the first completed
	// redeem of a serial; any later redeem of the SAME serial — by ANY server on this
	// ledger — mints nothing. One fee (one blind withdrawal, one serial) funds exactly
	// one conserved payout, so K colluding servers sharing one token can no longer
	// mint (K−1)·fee.
	//
	// THE EPOCH IS WHAT MAKES THE BOUND SAFE. A bounded guard must forget entries, and
	// forgetting a STILL-REDEEMABLE serial re-opens the pump — the red-team showed the
	// FIFO version is self-financing, because each flood serial used to evict a victim
	// is itself a paid delivery the colluding operator collects. So eviction here is
	// EXPIRY-ONLY: an entry is dropped only once its issuing epoch has left the
	// demand-token validity window, at which point the demand layer already rejects
	// that token (no in-window key_E verifies it) before any credit path. Evicted ⇒
	// expired ⇒ un-redeemable. See sweepExpiredSerials in delivery.go.
	//
	// BOUNDED (build-immutable #8): capped at maxPaidSerial, a DERIVED cap sized to
	// dominate the honest live set (delivery.go). If the cap is ever reached with
	// nothing expired, RedeemDeliveryCredit REFUSES TO PAY rather than forget a live
	// serial — an under-pay, never an over-pay and never a mint.
	//
	// SCOPE: this closes the SHARED-ledger case only. K truly distinct-owner ledgers
	// do not see each other's paidSerial, so the cross-owner-ledger variant remains
	// the standing Douceur / demand-authenticity limit, neutralized today ONLY by the
	// γ→1/N firewall (delivery credit is a BALANCE observable never wired to
	// Reputation — Invariant-A). This guard touches no field Reputation reads, so the
	// firewall is untouched.
	paidSerial map[string]paidSerialEntry

	// paidStore is the DURABLE half of the guard, and guardLoaded says whether this
	// ledger has read it yet (R0.4b re-break F2, 2026-09-03). Without it a restart is
	// an eviction of EVERY entry, in-window or not — the one eviction mode the design
	// forbids, and the one every node performs. The ledger appends to the store BEFORE
	// it PAYS — the supersede's reversal of the serve's self-mint runs first, which is
	// safe because that reversal is purely subtractive (see delivery.go's ordering
	// invariant) — and it refuses every guarded redeem while a store is attached but
	// unloaded. Nil store = the pre-existing in-memory-only behaviour (the sim and
	// most tests), which is sound exactly as long as the ledger it guards is equally
	// ephemeral — see the delivery.go call site.
	paidStore   ports.PaidSerialStore
	guardLoaded bool

	// epochSrc is the ONE clock this ledger reads its consensus epoch from (R2.10 /
	// F8, rule R-F8-SOURCE, research-certified 2026-09-04). It is injected once
	// (SetEpochSource) and read at the ENTRY of every guarded redeem and anchor spend
	// (advanceEpoch); no port method takes an epoch any more. In production it is
	// the node's chainEpoch() — the same function that prunes the demand keyset,
	// drives the receipt bank and verifies relay anchors — so the guard's expiry
	// predicate and the keyset's validity window are two predicates on ONE clock,
	// which is the coupling condition "evicted ⇒ expired ⇒ un-redeemable" rests on.
	// Nil reads as 0, the value a chain-less node produces; that keeps every
	// in-process fixture that never sets a source at today's epoch-0 behaviour.
	// The finalized-head epoch is REFUTED as a source (permanently 0 without BFT
	// finality; one epoch behind the keyset at every boundary block, which refuses
	// honest relay anchors as ReasonAnchorFuture once per epoch).
	epochSrc ports.EpochSource

	// bb is the R2.9a B_bootstrap instrument's whole ledger-side state, and it is an
	// EMPTY STRUCT in a default build. The instrument compiles only under the
	// `bbootstrap` build tag (D-BB-BUILD-TAG, docs/decisions.md): untagged,
	// bbootstrap_off.go declares `type bbootstrapState struct{}`, this field costs
	// zero bytes, and there is no observability clock, no monotone source and no
	// injection point anywhere in the binary. Tagged, bbootstrap.go declares the two
	// injected time sources and their origins, and SetObservabilityClock fills them —
	// but only when the operator passed -bbootstrap, so recording is gated on the flag
	// too. Nil/absent is the safe state at every layer.
	//
	// It is ONE field rather than four because a build tag cannot split a struct: the
	// fields have to live in a type that has a tagged and an untagged declaration.
	bb bbootstrapState

	// epochWatermark is the HIGHEST epoch this ledger has ever READ from its source:
	// epochWatermark = max(epochWatermark, epochSrc.Epoch()), taken once at the entry
	// of every guarded redeem and anchor spend, BEFORE the sweep and before every
	// screen (R-F8-LATCH). The sweep floor and every admission screen (ReasonBackdated,
	// ReasonAnchorFuture, the cap reserve) run against the watermark, never against
	// the raw source value.
	//
	// WHY A LATCH WHEN THE SOURCE CANNOT FALL. After O3 Direction T the node's
	// chainEpoch() is non-decreasing inside one process lifetime under every shipping
	// posture (`heavier` is height-first; `adopt` only swaps in a taller-or-equal
	// fork), so in production the max() never selects the old value. The latch stays
	// for two other reasons: (1) it is the PORT CONTRACT — EpochSource is an
	// interface, and the ledger cannot verify a mock or an embedder's source is
	// monotone, so it must not depend on it (TestEpochWatermark_IsMonotone and the
	// F8 falling-source gates pin exactly this); (2) it is the value FP-2 must persist
	// at the restore boundary. It is NOT a reorg defence; there is no in-process
	// reorg that lowers the head.
	//
	// AN HONEST-SKEW CORRECTNESS DEVICE, NOT A BYZANTINE DEFENCE (research
	// certification 2026-09-03, item 5). Against a source that lags and then catches
	// up the watermark can only widen what is refused, so the worst case is an
	// UNDER-pay of one server's conserved leg — never an over-pay, never a mint. The
	// former "one call at 2^62 poisons the ledger" exposure (F8) is closed by
	// construction, not by a clamp: no port input moves this clock, only the
	// injected source, and the production source is a pure read of the node's own
	// committed chain.
	//
	// OPEN-INERT, routed to FP-2 as R-F8-RESTART-REWIND: the guard file is durable
	// but the watermark is not, so a node that sweeps at epoch 10, compacts, and
	// restarts on a chain rewound to epoch ≤ 9 has forgotten a serial its keyset
	// still verifies. Self-pay on a private ledger today; real once balances are
	// transferable. Close (R-F8-RESTORE): persist the watermark in the same atom as
	// paidSerial, restore it, then raise to max(restored, epochSrc.Epoch()). The
	// faucet rate limiter must NOT be keyed on this watermark (R2.12).
	epochWatermark uint64

	// sweptEpoch is the last epoch at which sweepExpiredSerials actually ran, and
	// guardFullRefusals counts the redeems refused because the guard set was full of
	// still-live serials. Both are OBSERVABILITY-AND-COST bookkeeping for the R0.4b
	// guard, not part of any accounting rule.
	//
	// SWEEP AT MOST ONCE PER EPOCH (red-team RT-E, measured 1.32 ms per refused redeem
	// at a full live cap): the sweep is a full scan of the guard map, and it was run
	// on EVERY reserve call once the map reached the cap — so a full cap turned each
	// refused receipt into a 65,536-entry scan, an amplifier a griefer gets for free.
	// Nothing can expire twice within one epoch, so one sweep per epoch is exactly as
	// effective and amortizes to O(1). Purely a cost fix: the set of entries swept is
	// identical.
	sweptEpoch        uint64
	guardFullRefusals int64
	sweeps            int64
	// compactFailures / lastCompactErr record a durable-store Compact that returned an
	// error at the sweep (R2.13). Observability, never a refusal: see
	// sweepExpiredSerials for the two-class rule.
	compactFailures int64
	lastCompactErr  error

	// Audit economics: storage that survives a spot-check earns rent;
	// storage that turns out to be a lie is slashed hard. Balances may
	// go negative — debt is the scarlet letter. Exported so scenarios
	// can tune the pain.
	AuditReward int64
	AuditSlash  int64
}

var _ ports.CreditLedger = (*Ledger)(nil)

// New creates a ledger with the given publish fee. grant is the
// starting balance handed to each node on Register — the faucet that
// bootstraps a fresh economy (with zero grants, nobody could ever
// publish the first file).
func New(fee, grant int64) *Ledger {
	return &Ledger{
		fee: fee, grant: grant,
		accounts:    make(map[ports.NodeID]*account),
		rootOwner:   make(map[ports.Hash]ports.NodeID),
		escrow:      make(map[ports.Hash]*objectEscrow),
		provisional: make(map[provKey]*provisionalServe),
		provIndex:   make(map[provKey]int),
		paidSerial:  make(map[string]paidSerialEntry),
		AuditReward: 1_000,
		AuditSlash:  25_000,
	}
}

// SetEpochSource injects the ONE clock this ledger reads its consensus epoch from
// (R2.10 / F8, R-F8-SOURCE). Call it once, before any guarded redeem or anchor
// spend; the daemon wires the node's chain epoch right after the chain is enabled.
// A ledger with no source reads epoch 0 — a chain-less node's value — so nothing
// here refuses: the epochs-disabled brick is refused at start-up in cmd/silt
// (R-F8-DISABLED), and core stays permissive for in-process fixtures.
func (l *Ledger) SetEpochSource(src ports.EpochSource) { l.epochSrc = src }

// Epoch is the consensus epoch the NEXT guarded redeem or anchor spend will run
// against: max(epochWatermark, source), the latched read (R-F8-LATCH). It reads the
// source without moving the watermark — a pure observer for tests and operators; the
// watermark advances only at the guarded entry points (advanceEpoch).
//
// CONCURRENCY (PE ruling RULING-R2.10-F8-build-178ff3b F5): the production source reads
// the chain's block slice without a lock, which is safe only because every production
// caller — the guarded redeem, the anchor spend, the status snapshot — runs on the node's
// event loop, the chain's single writer. An off-loop caller of Epoch() (or of any guarded
// method) would race the chain; there is none today, and adding one needs a lock, not a
// second clock.
func (l *Ledger) Epoch() uint64 {
	if l.epochSrc == nil {
		return l.epochWatermark
	}
	if cur := l.epochSrc.Epoch(); cur > l.epochWatermark {
		return cur
	}
	return l.epochWatermark
}

// advanceEpoch is the ONE read of the source per guarded operation (R-F8-LATCH):
// called at the entry of RedeemDeliveryCreditReason's guarded path and of
// SpendRelayAnchors, before the sweep and before every screen. It raises the
// watermark by max and, on a band advance, runs the expiry sweep against the
// watermark — never against the raw source, which a mock or embedder may lower.
func (l *Ledger) advanceEpoch() {
	if l.epochSrc == nil {
		return
	}
	if cur := l.epochSrc.Epoch(); cur > l.epochWatermark {
		l.epochWatermark = cur
		// SWEEP ON THE BAND ADVANCE, NOT ONLY AT THE CAP (crypto-specialist advisory
		// C-7, 2026-09-03). The cap-only trigger meant that on any node below 65,536
		// live serials — which is every node most of the time — expired guard entries
		// were retained ON DISK indefinitely, far past the W-epoch window that is their
		// whole justification. Soundness was never affected (refuse-not-evict is the
		// correct choice and is unchanged), but state whose justification is a 4-epoch
		// window must not outlive it: this is the data-minimisation answer, and it is
		// the shape every epoch-scoped online e-cash since Brands uses — an
		// epoch-partitioned spent list dropped at rollover rather than a database swept
		// under load. It is also CHEAPER than the cap-triggered scan: O(cap) per epoch
		// instead of O(cap) per refused redeem. The sweptEpoch latch makes it at most
		// one scan per epoch however many callers drive it.
		l.sweepIfEpochAdvanced(l.epochWatermark)
	}
}

// Register creates the node's account and applies the starting grant.
// Registering twice is a no-op (no double grants).
//
// IT DOES NOT STAMP (G-BB-24). It used to: this is first touch on this ledger and the
// only place an account is constructed, which made the stamp structural — but "first
// touch" is not "first fetch", and every non-fetch path (bond audit, PoR grading,
// bounty payment, false-repair slash) reaches here through acct(). The R2.9a stamp
// now lives at recordFetched, the one place fetchedBytes is written, which is a
// narrower structural claim and the correct one.
//
// So Register carries no instrument call at all, in EITHER build. The one call a build
// tag could not remove moved with the stamp: recordFetched calls stampFirstFetch, which
// is an empty body in a default build (bbootstrap_off.go) and, under the `bbootstrap`
// tag, still writes nothing until -bbootstrap injects a clock (D-BB-BUILD-TAG).
// Register is byte-for-byte its pre-R2.9a self.
func (l *Ledger) Register(n ports.NodeID) {
	if _, ok := l.accounts[n]; ok {
		return
	}
	l.accounts[n] = &account{balance: l.grant}
	l.order = append(l.order, n)
}

func (l *Ledger) acct(n ports.NodeID) *account {
	l.Register(n)
	return l.accounts[n]
}

func (l *Ledger) RecordServe(server, requester ports.NodeID, _ ports.ChunkID, bytes int64) {
	if bytes <= 0 || server == requester {
		return // self-serving earns nothing (the cheapest gaming blocked)
	}
	s := l.acct(server)
	s.balance += bytes // 1 byte served = 1 credit
	s.servedBytes += bytes
	l.recordFetched(requester, bytes)
}

// recordFetched credits bytes to n's FETCHED total. It is the ONE write path for
// account.fetchedBytes — RecordServe above and RecordServeToObject (escrow.go) are its
// only callers — which is what makes the R2.9a age stamp below "written if and only if
// this identity is in the census" structural, rather than a guarded assignment at N
// call sites.
//
// IT IS UNTAGGED, AND THE STAMP IT CALLS IS NOT. Crediting fetched bytes is ordinary
// ledger accounting and predates R2.9a; it must compile in every build. The `when` is
// the instrument: stampFirstFetch has an empty untagged twin in bbootstrap_off.go, so a
// default silt binary walks this path and writes no first-fetch time at all
// (D-BB-BUILD-TAG, docs/decisions.md).
func (l *Ledger) recordFetched(n ports.NodeID, bytes int64) {
	a := l.acct(n)
	a.fetchedBytes += bytes
	if a.fetchedBytes > 0 {
		l.stampFirstFetch(a)
	}
}

func (l *Ledger) RecordAudit(prover ports.NodeID, _ ports.ChunkID, passed bool) {
	a := l.acct(prover)
	if passed {
		a.balance += l.AuditReward
		a.auditsPassed++
	} else {
		a.balance -= l.AuditSlash
		a.auditsFailed++
	}
}

func (l *Ledger) Audits(n ports.NodeID) (passed, failed int) {
	a := l.acct(n)
	return a.auditsPassed, a.auditsFailed
}

// RecordBondChallenge settles one storage-bond challenge (core/bond): the
// prover either answered a random challenge on its identity-bound bond of
// provenBytes, or failed to. Passing sets the node's challenged-storage
// standing — the large, unforgeable term Reputation is built on; failing
// zeroes it (a bond you cannot answer buys nothing). tick is a monotonic
// time reading, not a request counter: every caller in the daemon passes
// uint64(clock.Now())+1 (core/node/bondaudit.go), and the daemon's node
// clock is adapters/walltime, so in production this tick IS
// time.Now().UnixNano()+1. DecayStale reads it to retire standing that
// stops being re-proven, and it compares it against a `now` from the same
// clock, which is why the unit has to be time and not a count.
//
// THE UNIT MATTERS, and calling it a counter here is where that got lost: an
// earlier version of this comment said "request counter", four other texts
// inherited it, and a bond-path first-seen stamp was defended on that ground
// until 2026-09-05. That stamp is deleted (G-BB-28; see the account struct):
// a bond challenge writes lastBondTick and nothing else, and lastBondTick stays
// nanoseconds because DecayStale compares it against BondMaxAge. cf. por.go's
// n.rid, which IS a counter — they are not the same thing.
//
// NOTE: intentionally NOT (yet) on ports.CreditLedger. The bond auditor
// reaches it through an optional interface (a type assertion) so this
// lands without touching every CreditLedger implementer; promote it to
// the port once the auditor is wired in core/node/por.go.
func (l *Ledger) RecordBondChallenge(prover ports.NodeID, root ports.Hash, provenBytes int64, passed bool, tick uint64) {
	a := l.acct(prover)
	if passed {
		// Root-owner dedup (see rootOwner): a bond root credits standing to at
		// most one identity, so a colluding operator cannot amortise one plot
		// across N identities. The FIRST identity to prove a root owns it; a
		// later identity advertising the SAME root earns nothing. Only the
		// true owner can produce the plot to answer challenges (core/bond seals
		// from a per-identity secret), so an outsider cannot grief a victim by
		// pre-claiming its root.
		if root != (ports.Hash{}) {
			if owner, ok := l.rootOwner[root]; ok && owner != prover {
				a.bondedBytes = 0 // root already backs another identity's standing
				return
			}
			l.rootOwner[root] = prover
		}
		a.bondedBytes = provenBytes
		a.lastBondTick = tick
		return
	}
	a.bondedBytes = 0
	a.bondFails++
}

// equivocationSlash is the reputation penalty per proven double-sign — large
// enough to bury any conceivable earned standing, so an equivocator is barred
// from proposing and attesting and its attestations stop counting toward any
// fork's weight. Double-signing attacks consensus itself; it costs everything.
const equivocationSlash = 1 << 40

// SlashEquivocation records a proven consensus double-sign against a validator
// (evidence verified by chain.VerifyEquivocation before this is called). It
// zeroes the node's bonded standing and applies a crushing, permanent
// reputation penalty, so the equivocator can no longer influence consensus.
func (l *Ledger) SlashEquivocation(id ports.NodeID) {
	a := l.acct(id)
	a.equivocations++
	a.bondedBytes = 0
}

// falseRepairSlash is the reputation penalty per PROVEN false repair claim. A
// caretaker claimed a durability bounty (core/credit escrow) for a shard that
// fails the public correctness recompute (core/repairproof), and that
// non-verifying transcript is the attributable fraud proof. Unlike an
// equivocation — which buries standing forever because it attacks consensus
// itself — a false repair is deliberate but bounded fraud, so the penalty is
// large but FINITE. The magnitude is a tuning parameter (Evolving, per the
// tenets): it must exceed the standing a typical attester earns and make a false
// claim strictly -EV against the bounty it targets (bounty-relative calibration
// is an open item, docs/design/h7-proof-of-repair.md §12).
const falseRepairSlash = 1_000_000

// SlashFalseRepair records a PROVEN false repair claim against a caretaker
// (the failing correctness transcript verified by core/repairproof before this is
// called). It applies a crushing but finite reputation penalty. Like every slash
// it can only ever LOWER standing — a reduces-class press under Invariant A, never
// Sybil-amplifiable. It does NOT touch bondedBytes: the storage bond is
// independent of the repair lie, so only the earned standing is docked, and an
// honest node that later re-proves its bond is unaffected by the (finite) dent.
func (l *Ledger) SlashFalseRepair(id ports.NodeID) {
	l.acct(id).falseRepairs++
}

// DecayStale zeroes any standing whose last passing bond-challenge is
// older than maxAge, so a node that stops answering loses standing
// without anyone having to catch it lying. The validator/caretaker loop
// calls it with a monotonic now. This is what makes standing an integral
// over *sustained* proof rather than a one-time pass.
func (l *Ledger) DecayStale(now, maxAge uint64) {
	for _, a := range l.accounts {
		if a.bondedBytes > 0 && now > a.lastBondTick && now-a.lastBondTick > maxAge {
			a.bondedBytes = 0
		}
	}
}

// bondUnit converts bonded bytes into standing points: one point per
// 64 KiB of continuously-proven, identity-bound storage. This is the
// exchange rate between real disk and consensus weight — a tuning
// parameter (Evolving, per the tenets), NOT a fixed law.
const bondUnit = 64 << 10

// Reputation condenses a node's observed history into the number the
// chain consults (M12). It is built ONLY on evidence a node cannot
// fabricate, and — critically — it is minted by exactly ONE press:
//
//   - challenged, identity-bound held storage (bondedBytes, core/bond) —
//     the Sybil cost: N identities need N real bonds on real disk. This is
//     the ONLY term that GRANTS standing; minus
//   - failed audits and failed bond challenges, which bite hard.
//
// PoR audits (por.go) DO NOT grant Sybil-resistant standing (M0 hardening
// H1 / red-team RT-1). Plain proof-of-retrievability over shared, erasure-
// coded content proves POSSESSION of the bytes, not a DISTINCT physical
// replica: a data-less identity can RELAY a real holder's aggregated
// response and pass, because the proof is a pure function of (chunkID,
// challenge, data) — not bound to the prover (see docs/design/
// m0-hardening-strategy.md §4 S2, research memo 03). So a "+25 per pass"
// mint let a disk-less Sybil farm reach propose/attest eligibility with
// ZERO storage. Audits now fund only the BALANCE economy (RecordAudit
// credits balance) and act as a NEGATIVE integrity signal here — a failed
// audit on shards you CLAIMED to hold subtracts standing (it can never be
// Sybil-amplified, since it only ever reduces). Standing that gates
// consensus rests on the bond alone. Self-reported serving (servedBytes)
// is likewise not here: it funds the balance economy, not standing.
//
// Invariant A (docs/design/m0-hardening-strategy.md §2): no standing
// without a verified, identity-bound, deduped, bond-gated proof. Only the
// bond press satisfies it; the audit press is therefore denied a grant.
func (l *Ledger) Reputation(n ports.NodeID) int64 {
	a := l.acct(n)
	return a.bondedBytes/bondUnit -
		int64(a.auditsFailed)*250 -
		int64(a.bondFails)*250 -
		int64(a.falseRepairs)*falseRepairSlash -
		int64(a.equivocations)*equivocationSlash
}

func (l *Ledger) Balance(n ports.NodeID) int64      { return l.acct(n).balance }
func (l *Ledger) CanPublish(n ports.NodeID) bool    { return l.acct(n).balance >= l.fee }
func (l *Ledger) Fee() int64                        { return l.fee }
func (l *Ledger) ServedBytes(n ports.NodeID) int64  { return l.acct(n).servedBytes }
func (l *Ledger) FetchedBytes(n ports.NodeID) int64 { return l.acct(n).fetchedBytes }

// RepairsDone is the count of shard-repairs node n was paid a bounty for as the
// repairer — the per-node repair-work counter (the dual of objectEscrow.repairs,
// which counts repairs PER OBJECT and cannot attribute them to a node). Pure
// observability: it moves no credit and confers no standing. Reading moves nothing.
func (l *Ledger) RepairsDone(n ports.NodeID) int64 { return l.acct(n).repairsDone }

// BountyEarned is the lifetime credits node n earned as a repairer (the bounty
// half of its balance, so an operator can separate serve revenue from repair
// revenue). Pure observability; never a standing input. Reading moves nothing.
func (l *Ledger) BountyEarned(n ports.NodeID) int64 { return l.acct(n).bountyEarned }

func (l *Ledger) ChargePublish(n ports.NodeID) error {
	a := l.acct(n)
	if a.balance < l.fee {
		return ports.ErrInsufficientCredit
	}
	a.balance -= l.fee
	return nil
}

// Balances returns every registered node's balance in registration
// order.
func (l *Ledger) Balances() []int64 {
	out := make([]int64, len(l.order))
	for i, n := range l.order {
		out[i] = l.accounts[n].balance
	}
	return out
}

// Gini computes the Gini coefficient of the current balances: 0 means
// perfect equality, values toward 1 mean a few nodes hold everything.
// (Formula: mean absolute difference between all pairs, divided by
// twice the mean. Computed via the sorted form
// G = Σᵢ (2i − n − 1)·xᵢ / (n·Σ xᵢ) with 1-based ranks i.)
func (l *Ledger) Gini() float64 {
	return Gini(l.Balances())
}

func Gini(values []int64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := int64(len(sorted))
	var sum, weighted int64
	for i, v := range sorted {
		sum += v
		weighted += (2*int64(i+1) - n - 1) * v
	}
	if sum == 0 {
		return 0 // universal poverty is technically equality
	}
	return float64(weighted) / (float64(n) * float64(sum))
}
