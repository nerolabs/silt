package main

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/nerolabs/silt/adapters/chainstore"
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
