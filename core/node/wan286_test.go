package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// #286: a fresh 4-validator, quorum-2, 3-region objective chain wedged at genesis on
// GCP — no validator's genesis block ever committed — while the identical block
// committed on a single-zone quorum-1 SMOKE in 5 s. Root cause: the genesis block
// carries the proposer's one-time ~1.5 MB space-time bond registration (#299), and the
// flat RTT-scale RequestTimeout can't cover transferring + re-verifying 1.5 MB across a
// real WAN before the attester replies — a tight wall-clock TRANSPORT deadline is a
// category error (transport deadlines must be generous, the security lives in the proof
// not the clock — docs/network-durability.md). requestTimeoutFor now extends the
// deadline in proportion to the OUTBOUND payload size, so the one-time large block gets
// WAN headroom while lean steady-state blocks and holder fetches are unaffected.
func TestRequestTimeoutFor_SizeAware286(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	id := identity.FromSeed(1)

	cfg := DefaultConfig()
	cfg.RequestTimeout = 2 * ports.Second           // the daemon's base consensus deadline
	cfg.HolderDialTimeout = 500 * ports.Millisecond // the tighter speculative-fetch deadline
	cfg.RequestSizeFloorBytesPerSec = 262144        // 256 KB/s worst-case WAN throughput floor
	nd := New(id.NodeID(), cfg, sched, net.Endpoint(id.NodeID()), memstore.New())

	base := cfg.RequestTimeout

	// A lean steady-state consensus block (few KB) gets ~the base deadline — the fix
	// does NOT loosen the common path.
	lean := nd.requestTimeoutFor(ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, 2<<10)})
	if lean < base || lean > base+100*ports.Millisecond {
		t.Fatalf("a lean consensus block should keep ~the base deadline %v, got %v", base, lean)
	}

	// The one-time ~1.5 MB bond-registration/genesis block gains multi-second transport
	// headroom over the base — enough to cross a real WAN and be re-verified before the
	// attester replies. 1.5 MiB / 256 KB/s ≈ 6 s.
	big := nd.requestTimeoutFor(ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, 1_572_864)})
	if big < base+5*ports.Second {
		t.Fatalf("a ~1.5 MB registration block must gain multi-second WAN headroom over base %v; got %v", base, big)
	}

	// A speculative holder-fetch dial keeps its TIGHTER deadline and is never extended
	// by payload size — a fetch from a dead holder must stay cheap under churn (#277).
	hf := nd.requestTimeoutFor(ports.Message{Kind: ports.MsgFetchChunk, Data: make([]byte, 1_572_864)})
	if hf != cfg.HolderDialTimeout {
		t.Fatalf("a holder-fetch dial must keep its tight %v deadline (no size extension), got %v", cfg.HolderDialTimeout, hf)
	}

	// The extension is capped so a pathological block can't hang the round forever.
	huge := nd.requestTimeoutFor(ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, 100<<20)})
	if huge > base+30*ports.Second {
		t.Fatalf("the size extension must be capped at 30 s over base; got %v", huge)
	}
}

// The size extension must be OFF when the floor is 0 (no assumption about throughput),
// leaving the flat RequestTimeout exactly as before — so deployments that don't opt in
// see no behaviour change.
func TestRequestTimeoutFor_ExtensionOptOut286(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	id := identity.FromSeed(2)

	cfg := DefaultConfig()
	cfg.RequestTimeout = 2 * ports.Second
	cfg.RequestSizeFloorBytesPerSec = 0 // off
	nd := New(id.NodeID(), cfg, sched, net.Endpoint(id.NodeID()), memstore.New())

	got := nd.requestTimeoutFor(ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, 1_572_864)})
	if got != cfg.RequestTimeout {
		t.Fatalf("with the floor off, a large block must keep the flat RequestTimeout %v, got %v", cfg.RequestTimeout, got)
	}
}
