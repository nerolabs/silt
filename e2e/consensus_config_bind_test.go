package e2e

// Owner call F, THE RUNTIME COVER — the genesis-config bind, driven in REAL PROCESSES.
//
// WHY THIS FILE EXISTS AND WHY IT IS NOT IN cmd/silt. Both halves of the bind are
// properties of a DAEMON START, not of a library call: (1) the genesis a daemon MINTS
// commits this network's consensus config, so height-0 identity moves with the flags;
// (2) a daemon that RESTARTS on a chain it has already joined, with a consensus flag
// edited, REFUSES to start. The cmd/silt gates for both are source-text gates — they
// read daemon.go as a string — and a blind review measured the exact failure that shape
// invites: the check lifted into a helper defined later in daemon.go and called BEFORE
// chainstore.Recover left every source gate GREEN while the mechanism was completely
// dead (the chain is empty before the replay, and CheckConsensusParams returns nil on an
// empty chain). The binary served under a divergent -bond-label-k with zero refusal
// lines. Only a driven start can see that, so this is the gate that holds the seam.
//
// THE CONTROL for "a daemon that always refuses would pass this file": every other test
// in this package starts a daemon and asserts it comes up. A binary that refused
// unconditionally reddens the package, so the negative control is the suite, not a
// duplicated arm here.

import (
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/chainstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/genesis"
)

// cfgBindArgs is the smallest validator that reaches the chain-setup block in daemon.go
// (the mint and the refuse-to-start arm both live inside `if *validator`).
func cfgBindArgs(store, seed string, extra ...string) []string {
	args := []string{
		"-listen", "127.0.0.1:0", "-store", store,
		"-validator", "-objective=false", "-min-rep", "100", "-quorum", "1",
		"-bond", "8M", "-min-bond-floor", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", seed, // seeds 4841-4844: unique across e2e
	}
	return append(args, extra...)
}

var reGenesisBlock = regexp.MustCompile(`genesis: block ([0-9a-f]{64}) — height 0 COMMITS this network's consensus config \(([^)]*)\)`)

// G-CFGBIND-10 — THE MINT: height-0 identity is a function of the consensus config.
//
// This is the runtime cover for the source gate G-CFGBIND-7. The pre-bind shape
// (`genesis.Build(store, nil)`) compiles, prints the same line, and is silent — what it
// cannot do is produce two DIFFERENT hashes for two different configs. Two daemons that
// differ only in -bond-label-k must mint different genesis blocks; the same config must
// mint the same one, or the bind would brick an honest restart instead of a divergent
// one.
func TestGenesisHashMovesWithTheConsensusConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	mint := func(name, seed, k string) (hash, cfgText string) {
		d := startDaemon(t, name, cfgBindArgs(t.TempDir(), seed, "-bond-label-k", k)...)
		m := d.waitFor(t, reGenesisBlock, 30*time.Second)
		return m[1], m[2]
	}
	h64, cfg64 := mint("cfgbind-mint-64", "4841", "64")
	h32, cfg32 := mint("cfgbind-mint-32", "4842", "32")
	h64again, _ := mint("cfgbind-mint-64-again", "4843", "64")

	if h64 == h32 {
		t.Fatalf("THE BIND IS DECORATION: -bond-label-k 64 and 32 minted the SAME genesis block %s. "+
			"Height-0 identity does not cover the consensus config, so two operators who differ on it join "+
			"one network and reach different validity verdicts on the same block (I1). This is the state the "+
			"schema shipped in: a Block literal with no Params, cbor omitempty dropping the key.\n  k=64: %s\n  k=32: %s",
			h64, cfg64, cfg32)
	}
	if h64 != h64again {
		t.Fatalf("HEIGHT-0 IDENTITY IS NOT DETERMINISTIC: two daemons with the same consensus config minted "+
			"%s and %s. The bind would then refuse honest restarts and honest peers, which is worse than not "+
			"binding at all.", h64, h64again)
	}
	if !regexp.MustCompile(`-bond-label-k=64\b`).MatchString(cfg64) || !regexp.MustCompile(`-bond-label-k=32\b`).MatchString(cfg32) {
		t.Fatalf("the startup line must report the values it actually committed; got %q and %q", cfg64, cfg32)
	}
}

// G-CFGBIND-11 — THE REFUSAL: a daemon restarting on a chain it has already joined, with
// a consensus flag edited, EXITS instead of applying different rules to that history.
//
// This is the runtime cover for the source gate G-CFGBIND-8, and it is the arm the blind
// review measured DEAD behind green source gates. It is also why the order in daemon.go
// is load-bearing: the check reads blocks[0], so it is meaningful only against a chain
// LOADED FROM DISK — it must run after chainstore.Recover. Placed before the replay it
// passes on an empty chain and this test times out waiting for a refusal that never
// comes.
//
// The chain is written here rather than by a first daemon run on purpose: chain.cbor is
// persisted only by saveChain("commit"/"catch-up"/"takedown"), so a validator that never
// commits a block re-mints genesis from local flags on every restart and this arm is
// unreachable below height 1.
func TestDaemonRefusesToStartOnADivergentConsensusConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	store := t.TempDir()
	// A committed genesis for a network whose bond verifier uses k=64. Every other field
	// is left at zero: this test asserts on the -bond-label-k line specifically, and
	// pinning the rest would only re-encode the daemon's flag defaults in a second place.
	committed := chain.ConsensusParams{BondLabelSamples: 64}
	gb, _, _, err := genesis.Build(memstore.New(), &committed)
	if err != nil {
		t.Fatalf("build the committed genesis: %v", err)
	}
	chainPath := filepath.Join(store, "chain.cbor")
	if err := chainstore.Save(chainPath, []chain.Block{gb}); err != nil {
		t.Fatalf("persist the committed genesis: %v", err)
	}

	d := startDaemon(t, "cfgbind-refuse", cfgBindArgs(store, "4844", "-bond-label-k", "32")...)
	d.waitFor(t, regexp.MustCompile(`consensus config: REFUSING TO START`), 30*time.Second)
	d.waitFor(t, regexp.MustCompile(`^\s*-bond-label-k: this node has 32, the chain's genesis commits 64$`), 10*time.Second)
	err = d.cmd.Wait()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Fatalf("the daemon must EXIT on a divergent consensus config, not warn and serve: err=%v\n--- output ---\n%s", err, d.out.dump())
	}
	if m := d.out.find(rePeer); m != nil {
		t.Fatalf("the daemon became a peer under a consensus config the chain contradicts: %q", m[0])
	}
}
