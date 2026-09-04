package credit

// B_bootstrap — the per-requester fetched-bytes-vs-identity-age series (R2.9a).
//
// WHAT IT IS FOR. R2.9's affordability knob is the RATIO grant/r, and the ratio
// cannot be pinned from a synthetic fetch plan: it needs B_bootstrap, the bytes a
// REAL requester fetches while its identity is still young (D-R2.9-DIRECTION
// sentence 4, ratified 2026-09-04). cloudtest measures its own fetch plan, so the
// only source is real traffic on a real deployment. This file is the export that
// makes that measurable. It is INSTRUMENTATION ONLY: it reads two counters the
// ledger already keeps and moves no credit, no escrow and no standing (Invariant A
// classifies both readers `neutral`).
//
// WHAT IT MUST NEVER BECOME (immutable #4, refuse-to-surveil). A per-requester
// series is one careless field away from a who-fetched-what log, so the shape is
// pinned deliberately narrow:
//
//   - TOTAL bytes and AGE only. No object root, no chunk id, no per-fetch record —
//     the export cannot say WHAT anybody fetched, only how much and how long they
//     have existed.
//   - No timestamp finer than the epoch. The epoch is the coarsest clock the
//     ledger has, and it is the only one here.
//   - The requester id is a per-ledger SALTED hash, never the id. An operator can
//     bucket by age and volume inside ONE snapshot, and can do nothing else: the
//     salt is random, held in memory, never persisted and never shared, so the
//     series cannot be joined across restarts, across nodes, or back to an identity.
//     If the salt cannot be drawn, the export emits NOTHING — it fails closed on
//     privacy rather than emitting a joinable id.
//
// The HTTP surface (/api/status economy.bBootstrap) is narrower still: it drops the
// salted id and publishes (age, bytes) pairs only. See cmd/silt/ui.go.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/nerolabs/silt/ports"
)

// MaxRequesterFetchRows caps the snapshot FetchedBytesByRequester returns
// (build-immutable #8: the export must not grow without bound on a small box).
// 4,096 rows is ~200 KiB of strings and numbers. When more requesters have fetched
// bytes than this, the LARGEST fetchers are retained — they are the rows that
// dominate the grant/r ratio the series exists to pin — and FetchedRequesters
// reports the true total, so the caller can see that a tail was dropped.
const MaxRequesterFetchRows = 4096

// RequesterFetch is one row of the B_bootstrap series: how much one requester has
// fetched, and how old its identity is (via FirstSeenEpoch, against the epoch
// FetchedRequesters reports). It carries NO object root, NO raw identity and no
// clock finer than the epoch — see the privacy note at the top of this file, pinned
// by TestR29a_TheExportCarriesNoRootAndNoRequesterID.
type RequesterFetch struct {
	// SaltedRequester is sha256(per-ledger random salt ‖ requester id), hex, first
	// 16 bytes. It is stable inside one process and meaningless outside it.
	SaltedRequester string
	// FetchedBytes is this requester's LIFETIME fetched bytes on this ledger (the
	// same counter FetchedBytes(n) reads).
	FetchedBytes int64
	// FirstSeenEpoch is the ledger epoch at which this identity first touched the
	// ledger, written ONCE at account creation. age = epoch − FirstSeenEpoch.
	FirstSeenEpoch uint64
}

// FetchedBytesByRequester snapshots the B_bootstrap series: one row per requester
// with fetched bytes, ordered largest-fetcher first (then oldest identity, then
// salted id — a total order, so the snapshot is deterministic), capped at
// MaxRequesterFetchRows. Reading moves nothing.
//
// Iteration is over l.order (registration order), never over the account map: core
// does not iterate maps (B2). The transient copy is O(accounts) — the same order as
// the account map it copies from — while the RETURNED snapshot is capped.
func (l *Ledger) FetchedBytesByRequester() []RequesterFetch {
	salt, ok := l.exportSalt()
	if !ok {
		return nil // no salt, no export: never emit an id an operator could join on
	}
	rows := make([]RequesterFetch, 0, MaxRequesterFetchRows)
	for _, n := range l.order {
		a := l.accounts[n]
		if a == nil || a.fetchedBytes <= 0 {
			continue
		}
		rows = append(rows, RequesterFetch{
			SaltedRequester: saltedRequesterID(salt, n),
			FetchedBytes:    a.fetchedBytes,
			FirstSeenEpoch:  a.firstSeenEpoch,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].FetchedBytes != rows[j].FetchedBytes {
			return rows[i].FetchedBytes > rows[j].FetchedBytes
		}
		if rows[i].FirstSeenEpoch != rows[j].FirstSeenEpoch {
			return rows[i].FirstSeenEpoch < rows[j].FirstSeenEpoch
		}
		return rows[i].SaltedRequester < rows[j].SaltedRequester
	})
	if len(rows) > MaxRequesterFetchRows {
		rows = rows[:MaxRequesterFetchRows]
	}
	return rows
}

// FetchedRequesters reports the TOTAL number of requesters with fetched bytes — so a
// caller can tell that FetchedBytesByRequester truncated — together with the ledger
// epoch the series' ages are measured against. Reading moves nothing.
func (l *Ledger) FetchedRequesters() (requesters int, epoch uint64) {
	for _, n := range l.order {
		if a := l.accounts[n]; a != nil && a.fetchedBytes > 0 {
			requesters++
		}
	}
	return requesters, l.bootstrapEpoch()
}

// bootstrapEpoch is the ledger's OWN epoch — the clock B_bootstrap ages are measured
// against, and the clock stamped into firstSeenEpoch at account creation. It is the
// ledger's monotone epoch, never a caller's view, so an identity's age cannot go
// backwards when a laggard presents an older epoch.
//
// Today that number is epochWatermark, the highest epoch any redeemer has presented
// (see credit.go). Once the ledger OWNS a chain-anchored epoch (R2.10 / FP-2), this
// is l.Epoch() — a one-line change here, and nothing else in the export moves.
func (l *Ledger) bootstrapEpoch() uint64 { return l.Epoch() }

// exportSalt returns this ledger's per-process B_bootstrap salt, drawing it on first
// use. It is random, in-memory, never persisted and never leaves the process, so a
// restart re-salts and no two ledgers agree — which is exactly what stops the series
// being joined across restarts or nodes. false means the salt could not be drawn; the
// caller must then emit nothing.
func (l *Ledger) exportSalt() ([]byte, bool) {
	if l.fetchExportSalt == nil {
		s := make([]byte, 16)
		if _, err := rand.Read(s); err != nil {
			return nil, false
		}
		l.fetchExportSalt = s
	}
	return l.fetchExportSalt, true
}

// saltedRequesterID is the one-way bucket label: sha256(salt ‖ id) truncated to 16
// bytes, hex. Truncation is fine — this is a label, not a commitment — and the salt
// is what makes it un-invertible in practice (the id space is enumerable, so an
// UNsalted hash would be trivially reversible).
func saltedRequesterID(salt []byte, n ports.NodeID) string {
	h := sha256.New()
	h.Write(salt)
	h.Write(n[:])
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}
