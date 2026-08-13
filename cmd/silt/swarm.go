package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"os"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/adapters/discovery"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/ports"
)

// acquirePublishToken fetches the validators' token-issuer keys, then assembles
// an unlinkable publish token (T3) over the PREPAID-CREDIT path (M0 privacy F4):
// it mints one prepaid credit per validator — the fee is charged HERE, at mint —
// and then SPENDS those credits for the k blind signatures, so the publish itself
// records no per-publish fee debit tying it to the requester. The whole flow runs
// from the swarm client's ephemeral identity. Runs on the node's loop; cont fires
// once with the token or an error.
// rankByCanonical reorders the publisher's reachable validators by a
// network-canonical ordering (ledger-derived, heaviest bond first): canonical
// entries the publisher can reach come first in canonical order, then any remaining
// reachable peers (stable). So two publishers with overlapping reachable sets pick
// the SAME signer subset up to reachability — the subset is no longer an arbitrary
// per-publisher choice that leaks who published (R-3 / seam-4). Deterministic: same
// (reachable, canon) → same order, independent of the input peer order.
func rankByCanonical(reachable, canon []ports.NodeID) []ports.NodeID {
	reach := make(map[ports.NodeID]bool, len(reachable))
	for _, id := range reachable {
		reach[id] = true
	}
	out := make([]ports.NodeID, 0, len(reachable))
	seen := make(map[ports.NodeID]bool, len(reachable))
	for _, id := range canon { // canonical order first, but only what we can dial
		if reach[id] && !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	for _, id := range reachable { // then any reachable non-canonical peers, stable
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out
}

// Each stage OVERLAPS its round-trips (research stamp 2026-08-13, A1): the key
// fetches fire concurrently (skipping already-cached keys), then the per-issuer
// credit mints fire concurrently, then the k blind-sign requests gather in
// parallel inside AcquireTokenWithCredits. Selection is untouched — every stage
// still addresses the same canonically-ranked validator list, and each stage
// waits for ALL its round-trips to resolve before the next, so nothing about
// timing picks the signers. This collapses the gather's wall-clock from
// ~(2·len(validators)+k) sequential WAN round-trips to ~3.
func acquirePublishToken(nd *node.Node, validators []ports.NodeID, k int, cont func(*ports.PublishToken, error)) {
	// Stage 1: issuer keys, concurrent, best-effort; skip keys already cached.
	fetchKeys := func(next func()) {
		pending := 0
		fired := false
		settle := func() {
			if fired && pending == 0 {
				next()
			}
		}
		for _, v := range validators {
			if nd.IssuerKeyOf(v) != nil {
				continue
			}
			pending++
			nd.FetchIssuerKey(v, func(error) { pending--; settle() }) // best-effort
		}
		fired = true
		settle()
	}
	// Stage 2: mint one credit per validator (charged at mint), concurrent.
	mintCredits := func(next func(map[ports.NodeID]ports.PublishCredit)) {
		credits := map[ports.NodeID]ports.PublishCredit{}
		pending := 0
		fired := false
		settle := func() {
			if fired && pending == 0 {
				next(credits)
			}
		}
		for _, v := range validators {
			v := v
			pending++
			nd.AcquireCredits(rand.Reader, v, 1, nd.IssuerKeyOf, func(cs []ports.PublishCredit, _ error) {
				if len(cs) == 1 {
					credits[v] = cs[0] // best-effort per issuer; spend handles a shortfall
				}
				pending--
				settle()
			})
		}
		fired = true
		settle()
	}
	// Stage 3: spend the credits for the k blind signatures (no charge at spend).
	fetchKeys(func() {
		mintCredits(func(credits map[ports.NodeID]ports.PublishCredit) {
			serial, err := blindtoken.NewSerial(rand.Reader)
			if err != nil {
				cont(nil, err)
				return
			}
			nd.AcquireTokenWithCredits(rand.Reader, serial, validators, nd.IssuerKeyOf, credits, k, cont)
		})
	})
}

// cmdSwarm publishes to / retrieves from a running daemon swarm using a
// short-lived client node: join, do the thing, leave. The swarm keeps
// the data; the client keeps nothing.
func cmdSwarm(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: silt swarm add|get ... -peers ID@ADDR[,...] -registry URL")
	}
	switch args[0] {
	case "add":
		return swarmAdd(args[1:])
	case "get":
		return swarmGet(args[1:])
	default:
		return fmt.Errorf("unknown swarm command %q (add, get)", args[0])
	}
}

func swarmAdd(args []string) error {
	fs := flag.NewFlagSet("swarm add", flag.ExitOnError)
	peers := fs.String("peers", "", "bootstrap peers: ID@HOST:PORT[,...] (required)")
	regURL := fs.String("registry", "", "registry URL (required)")
	mode := fs.String("mode", "private", "encryption mode: private (default — random per-file key, unprobeable) or convergent (dedups identical content but is vulnerable to the guessed-plaintext confirmation attack; M0 H6, non-secret data only)")
	chunkSize := fs.Int("chunk-size", pipeline.DefaultChunkSize, "chunk size in bytes")
	tokenQuorum := fs.Int("token-quorum", 0, "publisher privacy: acquire a publish token from this many validators so the publish carries no Publisher identity. The signers are chosen by a NETWORK-CANONICAL ordering (ranked by committed bond, fetched from a chain-holding peer), the SAME for every publisher, so the signer subset can't narrow the publisher's anonymity set (R-3); falls back to -peers order if no peer serves a chain. 0 = off")
	allowPublisher := fs.Bool("allow-publisher", false, "record this node's durable Publisher identity on the entry (permanent linkage; off by default for privacy — prefer -token-quorum or an ungated publish)")
	replication := fs.Int("replication", 0, "how many closest holders receive each chunk (0 = default). Parity across holders backstops copies, so even 1 is viable; a lower factor makes shard loss (and thus caretaker repair) reproducible on a small swarm")
	saveToken := fs.String("save-token", "", "after acquiring a -token-quorum publish token, write it (CBOR) to this file so it can be RE-PRESENTED later with -use-token. A publish-token serial is single-use, so this is the seam that lets a harness drive the DOUBLE-SPEND rejection over the wire (#233)")
	useToken := fs.String("use-token", "", "RED-TEAM / TEST-HARNESS: publish carrying a token previously saved by -save-token, instead of minting a fresh one. Presenting the same token a second time re-uses its already-committed serial, which the chain rejects (ErrTokenSpent, double-spend). Never mint-once/publish-twice on a real network")
	pos := parseFlexible(fs, args)
	if len(pos) != 1 || *peers == "" || *regURL == "" {
		return fmt.Errorf("usage: silt swarm add <file> -peers ID@ADDR -registry URL [flags]")
	}
	m, err := crypto.ParseMode(*mode)
	if err != nil {
		return err
	}
	if m == crypto.Convergent {
		fmt.Fprintln(os.Stderr, "warning: -mode convergent is deterministic — anyone who guesses your exact plaintext can confirm you stored it (confirmation attack). Non-secret/shared data only.")
	}
	f, err := os.Open(pos[0])
	if err != nil {
		return err
	}
	defer f.Close()

	e, run, err := joinSwarm(*peers, *replication)
	if err != nil {
		return err
	}
	defer e.close()
	reg, err := openRegistry(*regURL)
	if err != nil {
		return err
	}

	var validators []ports.NodeID
	if ps, perr := discovery.ParseList(*peers); perr == nil {
		for _, p := range ps {
			validators = append(validators, p.ID)
		}
	}

	var h link.Handle
	var placed int
	err = nil
	if rerr := run(func(done func()) {
		publish := func(tok *ports.PublishToken) {
			opts := pipeline.Options{ChunkSize: *chunkSize, Mode: m, Rand: rand.Reader} // Rand needed for the private default (H6)
			switch {
			case tok != nil:
				opts.Token = tok // unlinkable: no Publisher identity
			case *allowPublisher:
				opts.Publisher = e.nd.ID() // opt-in: records permanent linkage
			}
			// Default (neither): publish carries no durable identity — M0-safe
			// on the chain; a credit-gated registry will refuse it, which is
			// the signal to pass -token-quorum or -allow-publisher.
			// Stage stores the chunks + manifest but does NOT register the
			// entry yet; we publish only after a confirmed scatter, so a
			// placement failure never leaves a dangling registry entry that
			// no link reaches (register-after-distribute, #65).
			var aerr error
			var entry ports.Entry
			h, entry, aerr = pipeline.Stage(context.Background(), e.nd.Store(), f, opts)
			if aerr != nil {
				err = aerr
				done()
				return
			}
			mf, merr := pipeline.LoadFull(context.Background(), e.nd.Store(), entry, h)
			if merr != nil {
				err = merr
				done()
				return
			}
			e.nd.Distribute(entry, mf, false, node.DerivePorKey(h.LayoutKey()), func(p int, derr error) {
				// Publish only on a confirmed scatter; a failed one leaves the
				// registry untouched so no dangling entry survives (#65).
				placed, err = pipeline.RegisterAfterDistribute(context.Background(), reg, entry, p, derr)
				done()
			})
		}
		if *useToken != "" {
			// Re-present a token saved by an earlier -save-token publish. Its serial
			// is already committed, so this SECOND publish is a double-spend the chain
			// must reject (ErrTokenSpent) — the wire seam for #233.
			raw, rerr := os.ReadFile(*useToken)
			if rerr != nil {
				err = rerr
				done()
				return
			}
			var tok ports.PublishToken
			if uerr := cbor.Unmarshal(raw, &tok); uerr != nil {
				err = fmt.Errorf("decode -use-token %q: %w", *useToken, uerr)
				done()
				return
			}
			publish(&tok)
		} else if *tokenQuorum > 0 {
			acquire := func(signers []ports.NodeID) {
				acquirePublishToken(e.nd, signers, *tokenQuorum, func(tok *ports.PublishToken, aerr error) {
					if aerr != nil {
						err = aerr
						done()
						return
					}
					if *saveToken != "" && tok != nil {
						if raw, merr := cbor.Marshal(tok); merr != nil {
							fmt.Fprintln(os.Stderr, "warning: could not encode token for -save-token:", merr)
						} else if werr := os.WriteFile(*saveToken, raw, 0o600); werr != nil {
							fmt.Fprintln(os.Stderr, "warning: could not write -save-token file:", werr)
						}
					}
					publish(tok)
				})
			}
			// R-3 / seam-4: pick the token signers by a NETWORK-CANONICAL ordering
			// (validators ranked by committed bond, fetched from a chain-holding peer)
			// rather than an arbitrary subset of -peers, so the signer subset stops
			// being a per-publisher quasi-identifier that can collapse the publisher
			// anonymity set. Fall back to -peers if no peer serves a chain (with an
			// honest warning). Reachability caveat: a chainless publisher can only sign
			// with validators it can dial, so the ranking is applied to the reachable
			// -peers; connecting to the canonical validator set makes it fully global.
			if len(validators) > 0 {
				// Try EVERY validator for the canonical set, not just validators[0]: a
				// single un-synced/unreachable validator (e.g. one that just restarted,
				// #351) otherwise drops us into the anonymity-narrowing fallback. The
				// ranking is deterministic, so any chain-holder answers the same.
				e.nd.FetchCanonicalIssuersFromAny(validators, func(canon []ports.NodeID, ferr error) {
					if ferr != nil || len(canon) == 0 {
						fmt.Fprintln(os.Stderr, "note: no canonical issuer set from peers; signing from -peers in given order — the signer subset may narrow the publisher anonymity set (connect to canonical validators for full privacy)")
						acquire(validators)
						return
					}
					acquire(rankByCanonical(validators, canon))
				})
			} else {
				acquire(validators)
			}
		} else {
			publish(nil)
		}
	}); rerr != nil {
		return rerr
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "scattered %d chunk replicas into the swarm; client keeps nothing\n", placed)
	fmt.Fprintf(os.Stderr, "care link (repair rights, no decryption): %s\n", h.Care())
	fmt.Fprintln(os.Stderr, "note: no caretaker is running for this content yet — its redundancy will")
	fmt.Fprintln(os.Stderr, "decay as nodes churn. Run a daemon with -care <careLink> to repair it")
	fmt.Fprintln(os.Stderr, "(publishing via a daemon's UI caretakes your content automatically).")
	fmt.Println(h)
	return nil
}

func swarmGet(args []string) error {
	fs := flag.NewFlagSet("swarm get", flag.ExitOnError)
	peers := fs.String("peers", "", "bootstrap peers: ID@HOST:PORT[,...] (required)")
	regURL := fs.String("registry", "", "registry URL (required)")
	out := fs.String("o", "", "output file (required)")
	pos := parseFlexible(fs, args)
	if len(pos) != 1 || *peers == "" || *regURL == "" || *out == "" {
		return fmt.Errorf("usage: silt swarm get <link> -o <out> -peers ID@ADDR -registry URL")
	}
	h, err := link.Parse(pos[0])
	if err != nil {
		return err
	}
	e, run, err := joinSwarm(*peers, 0)
	if err != nil {
		return err
	}
	defer e.close()
	reg, err := openRegistry(*regURL)
	if err != nil {
		return err
	}

	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	var getErr error
	if rerr := run(func(done func()) {
		e.nd.NetGet(reg, h, f, func(err error) { getErr = err; done() })
	}); rerr != nil {
		f.Close()
		os.Remove(*out)
		return rerr
	}
	if getErr != nil {
		f.Close()
		os.Remove(*out)
		return getErr
	}
	return f.Close()
}
