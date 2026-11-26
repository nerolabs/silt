package main

import (
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nerolabs/silt/adapters/chainstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// cmdTakedownReceipt derives the receipt for a root's on-chain takedown from a
// replica's own blocks: the auditor's way to obtain the proof for any removal,
// without asking the operator who honoured it.
func cmdTakedownReceipt(args []string) error {
	fs := flag.NewFlagSet("takedown-receipt", flag.ExitOnError)
	storeDir := fs.String("store", ".silt-daemon", "store directory holding chain.cbor (a validator's, or any replica's)")
	rootHex := fs.String("root", "", "the revoked root, hex")
	curatorHex := fs.String("curator", "", "the curator whose listing to prove, hex (as the honouring node's log names it); empty for a takedown from before curators")
	from := fs.Uint64("from", 0, "a checkpoint log size to prove consistency from (an earlier receipt's size); 0 proves from the empty log")
	fs.Parse(args)

	root, err := parseRootHex(*rootHex)
	if err != nil {
		return fmt.Errorf("takedown-receipt: %w", err)
	}
	blocks, err := chainstore.Load(filepath.Join(*storeDir, "chain.cbor"))
	if err != nil {
		return fmt.Errorf("takedown-receipt: %w", err)
	}
	l := chain.Listing{Root: root}
	if *curatorHex != "" {
		if l.Curator, err = ports.ParseHash(strings.TrimSpace(*curatorHex)); err != nil {
			return fmt.Errorf("takedown-receipt: -curator: %w", err)
		}
	}
	r, err := chain.ReceiptFromBlocks(blocks, l, *from)
	if err != nil {
		return fmt.Errorf("takedown-receipt: %w", err)
	}
	printReceipt(r)
	fmt.Printf("receipt: %s\n", base64.RawURLEncoding.EncodeToString(chain.EncodeReceipt(r)))
	return nil
}

// cmdTakedownVerify checks a receipt someone published, typically an operator's
// "takedown: honoured" line, against this replica: the anchoring block must be the
// one this replica committed at that height, and the receipt's proofs must hold
// against its committed log root.
func cmdTakedownVerify(args []string) error {
	fs := flag.NewFlagSet("takedown-verify", flag.ExitOnError)
	storeDir := fs.String("store", ".silt-daemon", "store directory holding this replica's chain.cbor")
	raw := fs.String("receipt", "", "the receipt, base64url as printed by an honouring node or takedown-receipt")
	fs.Parse(args)

	b, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(*raw))
	if err != nil {
		return fmt.Errorf("takedown-verify: receipt is not base64url: %w", err)
	}
	r, err := chain.DecodeReceipt(b)
	if err != nil {
		return fmt.Errorf("takedown-verify: %w", err)
	}
	blocks, err := chainstore.Load(filepath.Join(*storeDir, "chain.cbor"))
	if err != nil {
		return fmt.Errorf("takedown-verify: %w", err)
	}
	if r.At >= uint64(len(blocks)) {
		return fmt.Errorf("takedown-verify: NOT VERIFIED — the receipt is anchored at height %d and this replica's head is %d; sync it and retry", r.At, len(blocks)-1)
	}
	if err := chain.VerifyRevocationReceipt(r, blocks[r.At]); err != nil {
		return fmt.Errorf("takedown-verify: NOT VERIFIED — %w", err)
	}
	printReceipt(r)
	fmt.Printf("VERIFIED: the takedown of %s (curator %s) at height %d is leaf %d of the revocation log block %d commits (size %d), and that log extends the checkpoint at size %d\n",
		r.Root, r.Curator, r.Height, r.Index, r.At, r.Size, r.PrevSize)
	return nil
}

func printReceipt(r chain.RevocationReceipt) {
	fmt.Printf("takedown receipt:\n")
	fmt.Printf("  root:        %s (one root; a takedown names exactly one)\n", r.Root)
	if r.Curator != (ports.NodeID{}) {
		fmt.Printf("  curator:     %s (who listed it; the log leaf names it)\n", r.Curator)
	}
	fmt.Printf("  revoked at:  height %d, log leaf %d\n", r.Height, r.Index)
	fmt.Printf("  anchored at: block %d %s, log root %s, size %d\n", r.At, r.BlockHash, r.LogRoot, r.Size)
	fmt.Printf("  extends:     the log at size %d (root %s)\n", r.PrevSize, r.PrevLogRoot)
}

func parseRootHex(s string) (ports.Hash, error) {
	var h ports.Hash
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != len(h) {
		return h, fmt.Errorf("-root must be a %d-byte hex hash", len(h))
	}
	copy(h[:], b)
	return h, nil
}
