package chain

import (
	"crypto/ed25519"
	"crypto/sha256"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nerolabs/silt/ports"
)

// #563 memory oracle — the deep OOM's Reconcile spike, measured deterministically.
//
// FIELD OBSERVATION (a434494-deep): val-d kernel-OOM ×2 on the 2 GB box during
// post-drill cold-sync at depth (RSS 1.43 GiB sampled 2 min pre-kill; the terminal
// spike fell between 30 s samples). The issue's hypothesis was "fork + throwaway
// replica ≈ 2–3× chain resident at once." That multiple was unmeasured — and Go's
// []Block appends copy the struct SHALLOWLY (the multi-MB Answer backings are
// shared pointers), so the sync path's three concurrent slices (served[] →
// reconstructFork → tmp.blocks) may hold ONE copy of the payload bytes, not three.
//
// THE ORACLE: during a wire-faithful cold-sync Reconcile of an n-block chain whose
// blocks carry realistic multi-MB reg Answers, the EXTRA live heap over the
// already-resident decoded fork must stay a small fraction of the fork's payload —
// the throwaway replica and its derived maps may cost headers and map entries,
// never a second copy of the payload bytes. A change that deep-copies block
// payloads on the Reconcile path (or holds encode buffers across it) sends the
// extra to ≥1× fork payload and turns this RED.
//
// Deliberation + the outcome tree decided in advance:
// docs/thinking/2026-08-25-563-reconcile-memory-bench-deliberation.md

// verifyBigPrefix accepts any Answer carrying the sentinel prefix, so minted regs
// can carry field-realistic payload sizes (objectiveVerify demands the exact
// sentinel, which pins Answers at 5 bytes).
func verifyBigPrefix(_ []byte, _ ports.Hash, _ int64, _ uint64, answer []byte) bool {
	return len(answer) >= 5 && string(answer[:5]) == "valid"
}

// bondRegBig is bondReg with `pad` bytes of Answer payload after the sentinel —
// the shape of a real ~1.5 MiB PoST reg proof on the wire.
func bondRegBig(priv ed25519.PrivateKey, size int64, prev ports.Hash, pad int) BondReg {
	pub := priv.Public().(ed25519.PublicKey)
	answer := make([]byte, 5+pad)
	copy(answer, "valid")
	r := BondReg{
		Validator: append([]byte(nil), pub...),
		Root:      ports.HashBytes(pub),
		Size:      size,
		Answer:    answer,
	}
	r.Sig = ed25519.Sign(priv, r.signingBytes(BondRegNonce(prev)))
	return r
}

func heapAllocNow() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func TestReconcileMemoryBounded_563(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-hundred-MB allocation bench; skipped under -short")
	}
	if raceEnabled {
		t.Skip("live-heap budget is meaningless under -race: shadow memory + pool-reuse suppression inflate the peak ~10x (observed 60 MiB vs 6 MiB clean)")
	}
	prop := key(1)
	vals := []ed25519.PrivateKey{key(2), key(3), key(4), key(5)}

	// Field-realistic shape: every deep block carries a renewal reg with a
	// ~1.5 MiB proof Answer (the treadmill), n sized so the fork's payload
	// dominates every fixed overhead by orders of magnitude.
	const n = 48
	const pad = 1536 * 1024

	preBuild := heapAllocNow()
	src, g := objectiveChain(prop, vals, func(ports.NodeID) int64 { return 0 })
	src.SetBondVerifier(verifyBigPrefix)
	minted := []Block{*g}
	prev := g.Hash()
	for h := uint64(1); h <= n; h++ {
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		b.BondRegs = append(b.BondRegs, bondRegBig(prop, twoMiB, prev, pad))
		Sign(b, prop)
		for _, v := range vals[:3] {
			b.Atts = append(b.Atts, Attest(b, v))
		}
		if err := src.Append(*b); err != nil {
			t.Fatalf("mint h%d: %v", h, err)
		}
		minted = append(minted, *b)
		prev = b.Hash()
	}

	// The wire path: the cold replica receives BYTES; decoded blocks arrive
	// memo-less with fresh backing arrays, exactly like a real ChainReply.
	wire := EncodeBlocks(minted)
	forkPayload := uint64(len(wire))
	fork, err := DecodeBlocks(wire)
	if err != nil {
		t.Fatalf("roundtrip: %v", err)
	}
	// Release everything except the decoded fork: the minted originals, the
	// source chain, and the wire buffer are NOT resident in the field scenario
	// (the syncing node never held them). What remains over the pre-build
	// baseline is the decoded fork itself — the decode-inflation measurement.
	src, minted, wire = nil, nil, nil
	decodedResident := int64(heapAllocNow()) - int64(preBuild)

	rec, _ := objectiveChain(prop, vals, func(ports.NodeID) int64 { return 0 })
	rec.SetBondVerifier(verifyBigPrefix)

	base := heapAllocNow()
	var peak atomic.Uint64
	peak.Store(base)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var ms runtime.MemStats
		for {
			select {
			case <-stop:
				return
			default:
			}
			runtime.ReadMemStats(&ms)
			if ms.HeapAlloc > peak.Load() {
				peak.Store(ms.HeapAlloc)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	adopted, err := rec.Reconcile(fork)
	close(stop)
	<-done
	if err != nil || !adopted {
		t.Fatalf("reconcile: adopted=%v err=%v", adopted, err)
	}
	extra := peak.Load() - base
	// retained can be negative: adoption swaps the replica in (sharing the
	// fork's payload backing) and frees the old chain state.
	retained := int64(heapAllocNow()) - int64(base)
	t.Logf("#563 decomposition: peak extra %d MiB; retained after GC %d MiB (transient garbage = %d MiB)",
		extra>>20, retained>>20, (int64(extra)-retained)>>20)

	// Budget: the throwaway replica must cost headers + maps, never a second
	// copy of the payload. 1/4 of the fork payload is ~6× the measured green
	// extra (6 MiB at this shape) while the pre-fix per-block Hash() marshal
	// churn blew through it by 2× (69 MiB — the born-RED run) — the defect is
	// an order of magnitude, not a unit, so GC/sampling jitter can't flake it.
	budget := forkPayload/4 + 16<<20
	if extra > budget {
		t.Fatalf("#563 REPRODUCED: Reconcile of a %d MiB fork held %d MiB EXTRA live heap at peak (budget %d MiB) — a second resident copy of the fork payload on the cold-sync path (throwaway replica deep copy, or a held encode/decode buffer). The replica must share payload backing.",
			forkPayload>>20, extra>>20, budget>>20)
	}
	t.Logf("#563: fork payload %d MiB (decode-resident %d MiB, %.2fx wire); Reconcile peak extra %d MiB = %.2fx payload (budget %d MiB)",
		forkPayload>>20, decodedResident>>20, float64(decodedResident)/float64(forkPayload),
		extra>>20, float64(extra)/float64(forkPayload), budget>>20)
}

// The pooled-buffer marshal in Hash() MUST produce byte-identical encoding to
// the reference encMode.Marshal — a divergence would silently change every
// block hash (a consensus catastrophe, #558-class). Asserted over a block
// carrying every hashed field populated, including a multi-MB Answer.
func TestHashPooledBufferIdentity_563(t *testing.T) {
	prop := key(1)
	b := &Block{Version: BlockVersionRounds, Height: 7, Prev: ports.HashBytes([]byte("prev")),
		Entries:       []ports.Entry{entry(1), entry(2)},
		Revocations:   []ports.Hash{ports.HashBytes([]byte("rev"))},
		Unrevocations: []ports.Hash{ports.HashBytes([]byte("unrev"))},
	}
	b.BondRegs = append(b.BondRegs, bondRegBig(prop, twoMiB, b.Prev, 1536*1024))
	Sign(b, prop)

	unsigned := Block{Version: b.Version, Height: b.Height, Prev: b.Prev, Entries: b.Entries, Proposer: b.Proposer, Revocations: b.Revocations, Unrevocations: b.Unrevocations, BondRegs: b.BondRegs, Slashes: b.Slashes}
	ref, err := encMode.Marshal(&unsigned)
	if err != nil {
		t.Fatal(err)
	}
	want := ports.Hash(sha256sum(ref))
	fresh := *b
	fresh.hashMemoSet = false
	if got := fresh.Hash(); got != want {
		t.Fatalf("pooled-buffer Hash() diverged from reference Marshal path: got %x want %x — MarshalToBuffer is NOT byte-identical to Marshal under CanonicalEncOptions", got, want)
	}
}

func sha256sum(b []byte) [32]byte { return sha256.Sum256(b) }
