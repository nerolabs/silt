package node

import (
	"fmt"

	"github.com/nerolabs/silt/ports"
)

// SwarmClientConfig is the network posture of a publish/fetch CLIENT — the
// short-lived, ephemeral node a `silt swarm add` or `silt swarm get` joins the
// swarm as, stages one object through, and leaves.
//
// It differs from a daemon's posture in what it is FOR, not in how much of the
// adverse internet it expects. A client:
//
//   - is EPHEMERAL: it keeps nothing and peers must not route to it, so it
//     never reprovides and never caretakes;
//   - holds ONE route in: the bootstrap peers named on its command line are its
//     entire routing table at the moment it starts, where a daemon has a mesh it
//     has been growing for hours;
//   - is the only silt process an ordinary user ever runs, so its failures are
//     the ones a publisher or fetcher actually sees.
//
// All three make the adverse-internet discipline MORE load-bearing here, not
// less: a single dropped packet that evicts a daemon's peer costs that daemon
// one entry out of many, while the same packet costs a client its only route
// into the network and fails the whole publish or fetch. So the client carries
// the same retry-don't-evict posture a daemon runs with.
//
// HolderDialTimeout is deliberately left at zero. It only bites when it is
// strictly below RequestTimeout, and the client's RequestTimeout is already the
// 2 s a holder dial would get; setting it would change nothing and would imply a
// tighter dial that does not exist.
func SwarmClientConfig() Config {
	cfg := DefaultConfig()
	// A client crosses a real WAN to reach peers it has never spoken to, so it
	// is more patient per attempt than the sim default.
	cfg.RequestTimeout = 2 * ports.Second
	// Ride out a lost or slow packet instead of tearing the peer out of the
	// table: the decaying backoff covers an impairment of unknown duration
	// without guessing one large timeout.
	cfg.RequestRetries = 3
	cfg.RequestBackoff = 250 * ports.Millisecond
	cfg.RequireSignedProviders = true // reject forged/unsigned provider records on fetch
	cfg.ProviderRecordTTL = 30 * 60 * ports.Second
	cfg.DHTDomainCap = 2 // resolve providers from a domain-spread set — eclipse resistance
	return cfg
}

// SwarmClientOperationCeiling is the whole-operation cap on one client publish
// or retrieval: past it the client gives up on itself and reports a timeout
// rather than waiting on a swarm that is not going to answer. It is the outer
// bound on every number FetchPosture reports — a per-attempt budget that does
// not fit inside it cannot be spent.
const SwarmClientOperationCeiling = 5 * 60 * ports.Second

// FetchPosture is the deadline structure a client is actually running under,
// derived from its configuration. A retrieval that fails today says only which
// chunk had no reachable provider; it does not say how patient the client was
// being, so an operator cannot tell a swarm that has lost the content from a
// path too slow for the deadlines. This is that missing half, and it is what a
// bound on a retrieval is derived FROM: every term is a configured value, so a
// deployment that widens its deadlines for a worse path widens the bound with
// them instead of being graded against a number someone typed.
type FetchPosture struct {
	// PerRPC is the worst case for ONE discovery RPC — a provider or node
	// lookup — with its retries and their decaying backoff spent in full.
	PerRPC ports.Duration
	// PerDial is the deadline on one speculative holder dial. These are not
	// retried at the transport: the fetch loop re-sweeps instead.
	PerDial ports.Duration
	// Backoff is the retry delay inside PerRPC, reported separately because it
	// is the term that grows fastest as retries are raised.
	Backoff ports.Duration
	// Attempts is how many times one discovery RPC is sent before its peer is
	// given up: retries plus the original.
	Attempts int
	// Sweeps is how many times a chunk's provider set is re-swept before the
	// chunk is called unreachable, and SweepBackoff the delay between sweeps.
	Sweeps       int
	SweepBackoff ports.Duration
	// Providers is how many holders one sweep may dial, Alpha how many lookup
	// queries may be in flight at once, and K the lookup's convergence width.
	Providers, Alpha, K int
	// Ceiling is the whole-operation cap (SwarmClientOperationCeiling).
	Ceiling ports.Duration
}

// DeriveFetchPosture reads the deadline structure out of a configuration.
//
// PerRPC follows requestAttempt: the RPC is sent once and re-sent
// RequestRetries times, each attempt waiting RequestTimeout, with the backoff
// DOUBLING between attempts — so the backoff term is RequestBackoff × (2^r − 1),
// not r × RequestBackoff.
//
// PerDial follows requestTimeoutFor: a holder-fetch dial takes the tighter
// HolderDialTimeout only when one is configured AND it is below RequestTimeout;
// otherwise it takes RequestTimeout, which is why a client that sets no tighter
// dial is not thereby dialing without a deadline.
func DeriveFetchPosture(cfg Config) FetchPosture {
	backoff := ports.Duration(0)
	for a := 0; a < cfg.RequestRetries; a++ {
		backoff += cfg.RequestBackoff << a
	}
	dial := cfg.RequestTimeout
	if cfg.HolderDialTimeout > 0 && cfg.HolderDialTimeout < cfg.RequestTimeout {
		dial = cfg.HolderDialTimeout
	}
	sweepBackoff := ports.Duration(0)
	for a := 1; a < cfg.FetchAttempts; a++ {
		sweepBackoff += ports.Duration(a) * cfg.FetchBackoff
	}
	return FetchPosture{
		PerRPC:       cfg.RequestTimeout*ports.Duration(cfg.RequestRetries+1) + backoff,
		PerDial:      dial,
		Backoff:      backoff,
		Attempts:     cfg.RequestRetries + 1,
		Sweeps:       cfg.FetchAttempts,
		SweepBackoff: sweepBackoff,
		Providers:    cfg.Replication,
		Alpha:        cfg.Alpha,
		K:            cfg.K,
		Ceiling:      SwarmClientOperationCeiling,
	}
}

// secs renders a Duration in seconds. core holds no wall clock and does not
// import time (B1), so the formatting is here rather than borrowed.
func secs(d ports.Duration) string {
	return fmt.Sprintf("%.3fs", float64(d)/float64(ports.Second))
}

// String renders the posture as the one operator-facing line the client prints
// before it goes to the network. Every number is configured, none is a literal.
func (p FetchPosture) String() string {
	return fmt.Sprintf(
		"per-RPC <= %s (%d attempts + %s backoff) · per-dial %s · %d sweeps + %s backoff · replication %d · lookup k=%d alpha=%d · operation ceiling %s",
		secs(p.PerRPC), p.Attempts, secs(p.Backoff), secs(p.PerDial),
		p.Sweeps, secs(p.SweepBackoff), p.Providers, p.K, p.Alpha, secs(p.Ceiling))
}
