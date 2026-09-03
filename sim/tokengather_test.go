package sim

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	mrand "math/rand"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// The parallel token-gather (research stamp 2026-08-13, A1): transport is
// concurrent, but the accepted signer set must remain a function of CANONICAL
// RANK + liveness only — never of which issuer replied first. These tests pin
// that property against the live simnet, plus the wall-clock win itself.

// tokenNet builds V issuer nodes plus a publisher (nodes[0], also an issuer but
// never asked to self-sign), a shared ledger with plenty of credit, and the
// issuer key registry. Latency/loss shaped by cfg; deterministic per seed.
func tokenNet(t *testing.T, seed int64, V int, cfg simnet.Config) (
	*simclock.Scheduler, *simnet.Network, []*node.Node, []ports.NodeID, func(ports.NodeID) *rsa.PublicKey) {
	t.Helper()
	ledger := credit.New(100, 1_000_000)
	issuerReg := map[ports.NodeID]*rsa.PublicKey{}
	issuerPub := func(id ports.NodeID) *rsa.PublicKey { return issuerReg[id] }

	sched := simclock.New()
	net := simnet.New(sched, seed, cfg)
	ids := make([]ports.NodeID, V)
	nodes := make([]*node.Node, V)
	for i := 0; i < V; i++ {
		ident := identity.FromSeed(seed*1000 + int64(i))
		ids[i] = ident.NodeID()
		nd := node.New(ids[i], node.DefaultConfig(), sched, net.Endpoint(ids[i]), memstore.New())
		nd.SetLedger(ledger)
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		nd.EnableTokenIssuer(rand.Reader, key)
		issuerReg[ids[i]] = &key.PublicKey
		nodes[i] = nd
	}
	ledger.Register(ids[0])
	for i := 1; i < V; i++ {
		nodes[i].Bootstrap([]ports.NodeID{ids[0]}, func() {})
	}
	sched.Run()
	return sched, net, nodes, ids, issuerPub
}

// Under heavy latency jitter the reply order is arbitrary — but the signer set
// must be EXACTLY the canonical top-k, in canonical order, every time. A
// first-k-to-reply collector (the forbidden variant, R-3) would instead admit
// whichever issuers happened to be fast, failing this over the seed sweep.
func TestTokenGatherAcceptsCanonicalSetNotFastestRepliers(t *testing.T) {
	const V, k = 6, 2
	for seed := int64(1); seed <= 8; seed++ {
		// Jitter wide enough that reply order shuffles freely, but RTT stays
		// below the transport deadline — every signer is genuinely LIVE, so
		// liveness can't excuse a non-canonical set.
		cfg := simnet.Config{LatencyMin: 5 * ports.Millisecond, LatencyMax: 200 * ports.Millisecond}
		sched, _, nodes, ids, issuerPub := tokenNet(t, seed, V, cfg)
		rng := mrand.New(mrand.NewSource(seed))
		serial, _ := blindtoken.NewSerial(rng)

		canonical := ids[1:] // the caller's list IS the canonical ranking
		var tok *ports.PublishToken
		var acqErr error
		nodes[0].AcquireToken(rng, serial, canonical, issuerPub, k, func(tk *ports.PublishToken, err error) {
			tok, acqErr = tk, err
		})
		sched.Run()
		if acqErr != nil || tok == nil || len(tok.Sigs) != k {
			t.Fatalf("seed %d: acquisition failed: err=%v tok=%v", seed, acqErr, tok)
		}
		for i := 0; i < k; i++ {
			if tok.Sigs[i].Validator != canonical[i] {
				t.Fatalf("seed %d: signer %d is %s, want canonical rank %d (%s) — the accepted set must be canonical rank + liveness, never reply order",
					seed, i, tok.Sigs[i].Validator, i, canonical[i])
			}
		}
	}
}

// A dead canonical signer is FALLEN FORWARD from in canonical order: with rank 0
// down, the set is ranks {1,2} — never a lower rank promoted by reply speed.
func TestTokenGatherFallsForwardCanonicallyOnDeadSigner(t *testing.T) {
	const V, k = 6, 2
	cfg := simnet.Config{LatencyMin: 5 * ports.Millisecond, LatencyMax: 50 * ports.Millisecond}
	sched, net, nodes, ids, issuerPub := tokenNet(t, 42, V, cfg)
	rng := mrand.New(mrand.NewSource(42))
	serial, _ := blindtoken.NewSerial(rng)

	canonical := ids[1:]
	net.Kill(canonical[0])

	var tok *ports.PublishToken
	var acqErr error
	nodes[0].AcquireToken(rng, serial, canonical, issuerPub, k, func(tk *ports.PublishToken, err error) {
		tok, acqErr = tk, err
	})
	sched.Run()
	if acqErr != nil || tok == nil || len(tok.Sigs) != k {
		t.Fatalf("acquisition should survive a dead canonical signer: err=%v tok=%v", acqErr, tok)
	}
	want := []ports.NodeID{canonical[1], canonical[2]}
	for i := range want {
		if tok.Sigs[i].Validator != want[i] {
			t.Fatalf("signer %d is %s, want %s (canonical fall-forward past the dead rank-0)",
				i, tok.Sigs[i].Validator, want[i])
		}
	}
}

// The point of the change: k round-trips overlap. With a FIXED one-way latency L
// (RTT = 2L) and k=3, the sequential gather needed ≥ 3·RTT of virtual time; the
// parallel gather must finish in ~1·RTT. Asserted with a 2·RTT ceiling so the
// test pins parallelism without being brittle about scheduling epsilon.
func TestTokenGatherRoundTripsOverlap(t *testing.T) {
	const V, k = 6, 3
	const L = 100 * ports.Millisecond
	cfg := simnet.Config{LatencyMin: L, LatencyMax: L}
	sched, _, nodes, ids, issuerPub := tokenNet(t, 7, V, cfg)
	rng := mrand.New(mrand.NewSource(7))
	serial, _ := blindtoken.NewSerial(rng)

	start := sched.Now()
	var elapsed ports.Duration
	var acqErr error
	acqDone := false
	nodes[0].AcquireToken(rng, serial, ids[1:], issuerPub, k, func(tk *ports.PublishToken, err error) {
		acqErr, acqDone = err, true
		elapsed = ports.Duration(sched.Now() - start)
	})
	sched.Run()
	if !acqDone || acqErr != nil {
		t.Fatalf("acquisition failed: done=%v err=%v", acqDone, acqErr)
	}
	if rtt := 2 * L; elapsed > 2*rtt {
		t.Fatalf("gather took %v of virtual time — the k=%d round-trips did not overlap (sequential floor is %v)",
			elapsed, k, 3*rtt)
	}
}

// Lossy-path liveness + cost honesty: with per-message loss, transport retries
// re-present the SAME blinded serial and the issuer dedups — so the gather still
// completes and the publisher is charged exactly k fees, never more. (Retries
// are enabled as the daemon does; without the issuer dedup this test double-
// charges whenever a REPLY, rather than a request, is the packet lost.)
func TestTokenGatherLossyPathChargesExactlyK(t *testing.T) {
	const V, k = 6, 2
	const fee = 100
	for seed := int64(1); seed <= 6; seed++ {
		seed := seed
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			cfg := simnet.Config{LatencyMin: 5 * ports.Millisecond, LatencyMax: 50 * ports.Millisecond, Loss: 0.25}
			ledger := credit.New(fee, 1_000_000)
			issuerReg := map[ports.NodeID]*rsa.PublicKey{}
			issuerPub := func(id ports.NodeID) *rsa.PublicKey { return issuerReg[id] }
			sched := simclock.New()
			net := simnet.New(sched, seed, cfg)
			ncfg := node.DefaultConfig()
			ncfg.RequestRetries = 6 // ride out the loss (docs/network-durability.md §2)
			ids := make([]ports.NodeID, V)
			nodes := make([]*node.Node, V)
			for i := 0; i < V; i++ {
				ident := identity.FromSeed(seed*1000 + int64(i))
				ids[i] = ident.NodeID()
				nd := node.New(ids[i], ncfg, sched, net.Endpoint(ids[i]), memstore.New())
				nd.SetLedger(ledger)
				key, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatal(err)
				}
				nd.EnableTokenIssuer(rand.Reader, key)
				issuerReg[ids[i]] = &key.PublicKey
				nodes[i] = nd
			}
			ledger.Register(ids[0])
			start := ledger.Balance(ids[0])
			for i := 1; i < V; i++ {
				nodes[i].Bootstrap([]ports.NodeID{ids[0]}, func() {})
			}
			sched.Run()

			rng := mrand.New(mrand.NewSource(seed))
			serial, _ := blindtoken.NewSerial(rng)
			var tok *ports.PublishToken
			var acqErr error
			nodes[0].AcquireToken(rng, serial, ids[1:], issuerPub, k, func(tk *ports.PublishToken, err error) {
				tok, acqErr = tk, err
			})
			sched.Run()
			if acqErr != nil || tok == nil || len(tok.Sigs) != k {
				t.Fatalf("gather should ride out 25%% loss: err=%v tok=%v", acqErr, tok)
			}
			if charged := start - ledger.Balance(ids[0]); charged != k*fee {
				t.Fatalf("charged %d for k=%d signatures (fee %d): retries must never double-charge (issuer dedup, A2)",
					charged, k, fee)
			}
		})
	}
}
