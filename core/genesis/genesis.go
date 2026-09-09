// Package genesis is the network's first block — the founding manifesto
// that every fresh chain is born carrying, the way Bitcoin's genesis
// block hardcodes the Times headline. It is never downloaded or agreed:
// it is EMBEDDED in the binary and declared identically by every node,
// so all replicas share it by construction. Real blocks (earned by
// audit-backed quorum) build on top of it at height 1.
//
// The genesis file is also seeded into each node's store, so the
// manifesto is the one file the whole network always carries and anyone
// can retrieve by its (well-known, deterministic) link.
//
// Determinism is the whole trick: convergent encryption + fixed
// chunking/erasure means running Build on the embedded manifesto yields
// byte-identical chunks, the same root, the same link, and — signed by
// a fixed genesis key — the same block hash, on every machine. Swap the
// manifesto text and everything downstream regenerates; nothing here is
// a magic constant.
package genesis

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/registry"
	"github.com/nerolabs/silt/ports"
)

//go:embed manifesto.txt
var Manifesto []byte

// genesisKeySeed derives the fixed keypair that signs the genesis block.
// It is public knowledge on purpose — the genesis block's authority
// comes from being identical everywhere, not from a secret. No later
// block may use this identity (it earns no reputation).
var genesisKeySeed = sha256.Sum256([]byte("silt/genesis/v1/founding-key"))

// Key returns the fixed genesis signing key.
func Key() ed25519.PrivateKey { return ed25519.NewKeyFromSeed(genesisKeySeed[:]) }

// ID is the genesis proposer's NodeID.
func ID() ports.NodeID {
	return sha256.Sum256(Key().Public().(ed25519.PublicKey))
}

// Options pins the parameters that affect the bytes — chunk size, mode,
// erasure geometry, publisher — so the genesis file is reproducible
// regardless of what the pipeline defaults become. Neither FRAME size is
// pinned: both follow the pipeline's derivation, and because the genesis
// block hashes its entry — manifest chunk IDs included — and the root is
// built from the data and parity chunk IDs, both derivations are part of
// height-0 identity. Two changes have moved it, each owner-accepted on the
// ground that no live network exists to fork:
//
//	4′ 2026-09-07, the true-length MANIFEST frame:
//	  hash 7becf754…32ce → f428d0a8…0951, root unchanged.
//	R-SHORT-FINAL-STRIPE, the true-length DATA frame (the 2,042-byte
//	  manifesto is a single-frame object):
//	  hash f428d0a8…0951 → e44344ea…72c0, and the ROOT moves with it.
//
// TestGenesisBlockHashIsPinned holds all three current literals, so from
// here they move only by an explicit, recorded decision. Whether height-0
// identity sits inside the era-3/4 freeze surface is filed for R3.4
// (R-GENESIS-HASH-FREEZE-SURFACE). ManifestFrameBytes: 64 << 10 still
// reproduces the pre-4′ MANIFEST framing; the pre-short-stripe data
// framing has no knob, because nothing but archaeology wants it.
func Options() pipeline.Options {
	return pipeline.Options{
		ChunkSize: 64 << 10,
		Mode:      crypto.Convergent,
		Erasure:   erasure.Params{K: 10, N: 16},
		Publisher: ID(),
	}
}

// Build deterministically stages the genesis file into store and returns
// the canonical genesis block, the file's link, and its registry entry.
// Every node calls this at startup with its own store; because the
// process is deterministic, every node gets the identical block and
// link, and ends up holding the manifesto's chunks.
func Build(store ports.ChunkStore) (chain.Block, link.Handle, ports.Entry, error) {
	reg := registry.New() // throwaway: the entry goes in the block, not a registry
	h, err := pipeline.Add(context.Background(), store, reg, bytes.NewReader(Manifesto), Options())
	if err != nil {
		return chain.Block{}, link.Handle{}, ports.Entry{}, err
	}
	entry, _, err := reg.Lookup(context.Background(), h.Root)
	if err != nil {
		return chain.Block{}, link.Handle{}, ports.Entry{}, err
	}
	b := chain.Block{Version: chain.BlockVersion, Height: 0, Entries: []ports.Entry{entry}}
	chain.Sign(&b, Key())
	return b, h, entry, nil
}
