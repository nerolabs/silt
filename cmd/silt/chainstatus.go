package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nerolabs/silt/adapters/chainstore"
	"github.com/nerolabs/silt/core/chain"
)

// cmdChainStatus prints a read-only summary of a validator's committed chain
// replica — head height, head hash, and block/entry counts — WITHOUT launching a
// daemon or mutating anything. It reads the same `chain.cbor` the daemon
// persists. This is how an operator confirms the two things flows 5–7 are about
// without hashing files by hand (acceptance new-F2):
//   - CONVERGENCE: run it on each replica; identical head height AND head hash
//     means they agree byte-for-byte on the committed history.
//   - RESTART CATCH-UP: the head height advances after a restarted validator
//     rejoins, instead of staying stuck at its pre-restart height.
func cmdChainStatus(args []string) error {
	fs := flag.NewFlagSet("chain-status", flag.ExitOnError)
	storeDir := fs.String("store", ".silt-daemon", "store directory holding chain.cbor (matches `silt daemon -store`)")
	epochBlocks := fs.Uint64("epoch-blocks", DerivedEpochBlocks, "the swarm's epoch cadence (matches `silt daemon -epoch-blocks`), used only to diagnose a head stuck at an epoch boundary (#535); 0 disables the diagnosis")
	fs.Parse(args)

	path := filepath.Join(*storeDir, "chain.cbor")
	blocks, err := chainstore.Load(path)
	if err != nil {
		return fmt.Errorf("chain-status: %w", err)
	}
	fmt.Printf("chain-status (%s):\n", path)
	if len(blocks) == 0 {
		fmt.Println("  no chain yet (0 blocks) — this node isn't a validator, or nothing has committed")
		return nil
	}
	head := blocks[len(blocks)-1]
	headHash := head.Hash()
	entries, pruned := 0, 0
	for i := range blocks {
		entries += len(blocks[i].Entries)
		if blocks[i].IsPruned() {
			pruned++
		}
	}
	fmt.Printf("  head height:  %d\n", head.Height)
	fmt.Printf("  head hash:    %s\n", headHash)
	fmt.Printf("  blocks:       %d (incl. genesis)\n", len(blocks))
	fmt.Printf("  entries:      %d committed\n", entries)
	// The retention prune sheds the heavy bond proofs of blocks below the
	// rolling horizon while keeping headers + consensus sigs, so on-disk and
	// resident chain weight stays bounded to a recent finalized window. A
	// nonzero count here is how an operator (or the field harness) confirms
	// the prune is engaged from real persisted state, not a log line.
	fmt.Printf("  pruned:       %d blocks payload-stripped below the retention horizon\n", pruned)
	printEraObservable(blocks)
	// #535 diagnosis (S5 — never silently fail): when the NEXT height to commit
	// is an epoch boundary, a head that is not advancing may be the epoch-
	// boundary liveness wedge — members holding > 1/3 of the frozen epoch's
	// weight lapsed, so no live coalition can reach the frozen 2/3 bar, and the
	// rotation that would shed them is gated behind the very quorum they deny.
	// A static snapshot cannot see "not advancing" or the live weight (the
	// daemon's stalled-at-boundary warn log carries the weight evidence), so
	// this names the state and the recovery path rather than asserting it.
	if *epochBlocks > 0 && (head.Height+1)%*epochBlocks == 0 {
		fmt.Printf("  next height:  %d is an EPOCH BOUNDARY — if the head is stuck here, check the daemon log for `stalled-at-boundary` (#535: live-qualified weight below the frozen 2/3 bar; the recovery is a coordinated -liveness-recovery-height, weak-subjectivity trust — see `silt daemon -h`)\n", head.Height+1)
	}
	fmt.Println("  → identical values across replicas mean they agree on the committed history")
	return nil
}

// printEraObservable prints the era half of chain-status: the block-version census and the two
// era lines (R-CLOUD-ERA-PROBE, freeze manifest item 19). It reads the persisted blocks and
// NOTHING ELSE — no config, no flag, no replay. See chain.CensusOf for why.
//
// WHAT THIS PATH CANNOT SEE, AND SAYS SO. chain-status holds a []Block, not a chain.Chain, so the
// readiness-tally latch is out of reach. It must not be rebuilt here: recovering the latch means
// replaying into a fresh Chain with a Config this command does not have, so EpochBlocks would come
// from a CLI flag and the reported activation height would be a function of what the operator
// typed. That is the #380 class, manufactured inside the tool built to observe era state. So
// EraLine is called with tallyVisible=false, and the dark case names the limit and where to get
// the answer instead of printing a false that means "not observable here".
func printEraObservable(blocks []chain.Block) {
	c := chain.CensusOf(blocks)
	fmt.Printf("  head version: v%d\n", c.HeadVersion)
	fmt.Printf("  versions:     %s\n", versionCensusLine(c))
	// The two era lines. Only era-3 and era-4 have activation state; v2 needs none.
	for _, v := range []uint64{chain.BlockVersionStateRoot, chain.BlockVersionWitnessable} {
		s := chain.EraStatus{Version: v, Phase: chain.EraDark}
		if h, ok := c.FirstHeights[v]; ok {
			hh := h
			s.FirstHeight, s.Phase = &hh, chain.EraActive
		}
		fmt.Printf("  %s\n", s.EraLine(false))
	}
	// max_h len(blocks[h].Atts) — the live attestation-carrier width, a figure two certification
	// items name as unmeasured and no shipped command produced before this one. A bare "0" would
	// be unreadable against "never computed", so zero is narrated.
	switch {
	case !c.AttsMeasured:
		fmt.Println("  max atts:     NOT MEASURED — no block was walked")
	case c.MaxAtts == 0:
		fmt.Println("  max atts:     0 — measured across every block; none carries an attestation (a genesis-only or single-signer chain)")
	default:
		fmt.Printf("  max atts:     %d, first at height %d (max_h len(blocks[h].Atts) across all %d blocks)\n",
			c.MaxAtts, c.MaxAttsHeight, c.Blocks)
	}
}

// versionCensusLine renders the per-version block counts in ascending version order, e.g.
// "v2 x 41, v4 x 2, v5 x 3 (highest v5, first at height 43)".
func versionCensusLine(c chain.VersionCensus) string {
	vs := make([]uint64, 0, len(c.Counts))
	for v := range c.Counts {
		vs = append(vs, v)
	}
	sort.Slice(vs, func(i, j int) bool { return vs[i] < vs[j] })
	parts := make([]string, 0, len(vs))
	for _, v := range vs {
		parts = append(parts, fmt.Sprintf("v%d x %d", v, c.Counts[v]))
	}
	return fmt.Sprintf("%s (highest v%d, first at height %d)",
		strings.Join(parts, ", "), c.MaxVersion, c.MaxVersionFirstHeight)
}
