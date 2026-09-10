package main

import (
	"flag"
	"fmt"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/genesis"
)

// cmdGenesis prints the founding block that every fresh Silt network
// carries: the manifesto, its deterministic link (the same on every
// machine), and the genesis block hash. Swap core/genesis/manifesto.txt
// and all three change together — nothing here is a magic constant.
func cmdGenesis(args []string) error {
	fs := flag.NewFlagSet("genesis", flag.ExitOnError)
	full := fs.Bool("text", false, "also print the full manifesto")
	fs.Parse(args)

	// nil params ON PURPOSE, and the caveat below is why it is not a lie. This
	// command has no network configuration and cannot invent one: since owner call F
	// the genesis a DAEMON mints commits its consensus config (chain.ConsensusParams),
	// so height-0 identity is a function of the manifesto AND the flags. There is no
	// longer one true genesis hash to print. What is still universal — the manifesto,
	// its chunking/erasure geometry, and therefore the link and the root — is printed
	// unqualified; the block hash is printed with what it actually is.
	block, h, entry, err := genesis.Build(memstore.New(), nil)
	if err != nil {
		return err
	}
	bh := block.Hash()
	fmt.Printf("genesis block:  %s (height 0, %d entry) — the PARAMLESS hash\n", bh, len(block.Entries))
	fmt.Printf("genesis link:   %s\n", h)
	fmt.Printf("manifesto root: %s (%d bytes)\n", entry.Root, entry.FileSize)
	fmt.Println("\nNOTE: a daemon-launched network COMMITS its consensus config into height 0 (-quorum,")
	fmt.Println("-min-bond, -min-bond-floor, -anchors, -epoch-blocks, -bond-label-k, ...), so its")
	fmt.Println("genesis block hash DIFFERS from the one above — that is what stops a differently-configured")
	fmt.Println("node from joining. The link and the manifesto root are config-independent and are the same")
	fmt.Println("on every network. To see a real network's genesis hash, read the daemon's own startup line.")
	if *full {
		fmt.Printf("\n%s\n", genesis.Manifesto)
	}
	return nil
}
