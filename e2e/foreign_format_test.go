package e2e

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

var reForeignFormat = regexp.MustCompile(`foreign-format: refused a block in a format this build cannot validate.*format=(\d+)`)

// A running validator handed a block from an era after its own, over real TCP, by a
// peer it has never seen: it refuses the block, says so in its log at warn, keeps its
// head, and goes on answering. This is the stall an un-upgraded node meets at an era
// boundary, driven in a real process rather than asserted in a library call.
func TestRunningValidatorStallsLoudlyOnALaterEraBlock(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	store := t.TempDir()
	d := startDaemon(t, "foreign-format", cfgBindArgs(store, "4851")...)
	gm := d.waitFor(t, reGenesisBlock, 30*time.Second)
	pm := d.waitFor(t, rePeer, 30*time.Second)
	var daemonID, genesis ports.Hash
	if _, err := hex.Decode(daemonID[:], []byte(pm[1])); err != nil {
		t.Fatal(err)
	}
	if _, err := hex.Decode(genesis[:], []byte(gm[1])); err != nil {
		t.Fatal(err)
	}

	// The later build: a peer with its own identity and a block one format past
	// this binary's, linked to the daemon's own genesis.
	loop := eventloop.New()
	go loop.Run()
	t.Cleanup(loop.Stop)
	id := identity.FromSeed(9851)
	tr, err := tcpnet.New(loop, id, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	replies := make(chan ports.Message, 16)
	tr.SetHandler(func(_ ports.NodeID, m ports.Message) { replies <- m })
	tr.AddPeer(daemonID, pm[2])

	future := chain.Block{Version: chain.BlockVersionWitnessable + 1, Height: 1, Prev: genesis,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("next-era-entry"))}}}
	chain.Sign(&future, id.Signer())
	if err := tr.Send(daemonID, ports.Message{Kind: ports.MsgCommitBlock, RID: 1, Data: chain.Encode(&future)}); err != nil {
		t.Fatalf("send the later-era block: %v", err)
	}
	ack := await(t, replies, ports.MsgCommitAck)
	if ack.OK {
		t.Fatal("the validator ACKNOWLEDGED a block in a format it cannot validate")
	}

	logPath := filepath.Join(store, "debug.log")
	m := waitForInLog(t, logPath, reForeignFormat, 15*time.Second)
	if want := fmt.Sprint(chain.BlockVersionWitnessable + 1); m[1] != want {
		t.Fatalf("the warn line names format %s, want %s", m[1], want)
	}

	// Still alive, still serving, head unmoved.
	if err := d.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("the validator exited after the refusal (%v):\n%s", err, d.out.dump())
	}
	if err := tr.Send(daemonID, ports.Message{Kind: ports.MsgGetChainHead, RID: 2}); err != nil {
		t.Fatalf("probe the head: %v", err)
	}
	head := await(t, replies, ports.MsgChainHeadReply)
	if !head.OK || head.Height != 0 || string(head.Data) != string(genesis[:]) {
		t.Fatalf("after the refusal the validator's head must still be its genesis: ok=%v height=%d head=%x",
			head.OK, head.Height, head.Data)
	}
}

func await(t *testing.T, ch <-chan ports.Message, kind ports.MsgKind) ports.Message {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case m := <-ch:
			if m.Kind == kind {
				return m
			}
		case <-deadline:
			t.Fatalf("no %v reply within 15s", kind)
		}
	}
}
