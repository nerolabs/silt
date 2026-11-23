package e2e

import (
	"bytes"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	reUnplaced     = regexp.MustCompile(`(chunk \S+ placed on no node after \d+ attempts|stripe \d+ unrecoverable: only \d+ of \d+ shards placed, need \d+)`)
	reEntriesCount = regexp.MustCompile(`entries:\s+(\d+) committed`)
)

// committedEntries reads a validator's committed entry count off its store, the
// registry's own record, so "no registry entry" is read from the chain and not
// inferred from the client's exit code.
func committedEntries(t *testing.T, store string) string {
	t.Helper()
	out, err := exec.Command(siltBin, "chain-status", "-store", store).CombinedOutput()
	if err != nil {
		t.Fatalf("chain-status: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "no chain yet") {
		return "0"
	}
	m := reEntriesCount.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatalf("chain-status reported no entry count:\n%s", out)
	}
	return m[1]
}

// A publish into a swarm with no room fails explicitly: it names what could not be
// placed, prints no link, and commits no registry entry. Once the swarm has room,
// the same file publishes and fetches bit-perfect. Bit-perfect or an explicit
// failure, never a link to content the swarm did not take.
func TestUnplaceablePublishFailsExplicitlyThenSucceedsOnRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	// The validator and registry host refuses content (a one-byte pledge), so
	// it cannot quietly absorb what the holders refuse.
	storeA := t.TempDir()
	a := startDaemon(t, "unplaceable-A",
		"-listen", "127.0.0.1:0", "-store", storeA,
		"-serve-registry", "127.0.0.1:0",
		"-validator", "-quorum", "0", "-min-rep", "0",
		"-capacity", "1", "-mdns=false", "-id-seed", "4861")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	bootstrapA := peer[1] + "@" + peer[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]

	// Two holders that are full: a one-byte pledge refuses every chunk.
	for _, seed := range []string{"4862", "4863"} {
		d := startDaemon(t, "unplaceable-full-"+seed,
			"-listen", "127.0.0.1:0", "-store", t.TempDir(), "-bootstrap", bootstrapA,
			"-capacity", "1", "-mdns=false", "-id-seed", seed)
		d.waitFor(t, reBootstrap, 20*time.Second)
	}

	src := filepath.Join(t.TempDir(), "payload.bin")
	want := make([]byte, 512<<10)
	rand.New(rand.NewSource(0x13a)).Read(want)
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}
	add := []string{"swarm", "add", src, "-peers", bootstrapA, "-registry", regRef, "-chunk-size", "65536"}

	before := committedEntries(t, storeA)
	out, err := runClientAllowErr(t, add...)
	if err == nil {
		t.Fatalf("a publish into a swarm with no room SUCCEEDED:\n%s", out)
	}
	if !reUnplaced.MatchString(out) {
		t.Fatalf("the failed publish does not name what could not be placed:\n%s", out)
	}
	t.Logf("the failed publish said: %s", reUnplaced.FindString(out))
	if l := regexp.MustCompile(`silt:v1:\S+`).FindString(out); l != "" {
		t.Fatalf("the failed publish printed a link (%s) to content the swarm never took:\n%s", l, out)
	}
	// Give a commit that should not happen the time a real one takes.
	time.Sleep(2 * time.Second)
	if after := committedEntries(t, storeA); after != before {
		t.Fatalf("the failed publish left a registry entry: committed entries %s -> %s", before, after)
	}

	// Room arrives: two holders with real pledges join.
	for _, seed := range []string{"4864", "4865"} {
		d := startDaemon(t, "unplaceable-roomy-"+seed,
			"-listen", "127.0.0.1:0", "-store", t.TempDir(), "-bootstrap", bootstrapA,
			"-capacity", "1G", "-mdns=false", "-id-seed", seed)
		d.waitFor(t, reBootstrap, 20*time.Second)
	}
	out = runClient(t, add...)
	link := reLink.FindString(strings.TrimSpace(out))
	if link == "" {
		t.Fatalf("the retry printed no link:\n%s", out)
	}
	a.waitFor(t, reCommitted, 20*time.Second)
	if after := committedEntries(t, storeA); after == before {
		t.Fatalf("the retry printed a link but committed no entry (still %s)", after)
	}

	dst := filepath.Join(t.TempDir(), "fetched.bin")
	runClientStderr(t, "swarm", "get", link, "-o", dst, "-peers", bootstrapA, "-registry", regRef)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("the retried publish did not fetch bit-perfect: got %d bytes, want %d", len(got), len(want))
	}
}

// The same contract when placement fails because the holders are unreachable rather
// than full: both are in the routing table, and both stop answering (SIGSTOP), so
// every store request times out. The publish fails explicitly with no link and no
// entry; once the holders answer again, the retry publishes and fetches bit-perfect.
func TestPublishToUnreachableHoldersFailsExplicitlyThenSucceedsOnRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	storeA := t.TempDir()
	a := startDaemon(t, "unreachable-A",
		"-listen", "127.0.0.1:0", "-store", storeA,
		"-serve-registry", "127.0.0.1:0",
		"-validator", "-quorum", "0", "-min-rep", "0",
		"-capacity", "1", "-mdns=false", "-id-seed", "4866")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	bootstrapA := peer[1] + "@" + peer[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]

	var holders []*daemon
	for _, seed := range []string{"4867", "4868"} {
		d := startDaemon(t, "unreachable-"+seed,
			"-listen", "127.0.0.1:0", "-store", t.TempDir(), "-bootstrap", bootstrapA,
			"-capacity", "1G", "-mdns=false", "-id-seed", seed)
		d.waitFor(t, reBootstrap, 20*time.Second)
		holders = append(holders, d)
	}

	src := filepath.Join(t.TempDir(), "payload.bin")
	want := make([]byte, 512<<10)
	rand.New(rand.NewSource(0x13b)).Read(want)
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}
	add := []string{"swarm", "add", src, "-peers", bootstrapA, "-registry", regRef, "-chunk-size", "65536"}

	for _, d := range holders {
		if err := d.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
			t.Fatal(err)
		}
	}

	before := committedEntries(t, storeA)
	start := time.Now()
	out, err := runClientAllowErr(t, add...)
	t.Logf("the publish to unreachable holders ended after %s", time.Since(start).Round(time.Second))
	if err == nil {
		t.Fatalf("a publish to unreachable holders SUCCEEDED:\n%s", out)
	}
	if !reUnplaced.MatchString(out) {
		t.Fatalf("the failed publish does not name what could not be placed:\n%s", out)
	}
	t.Logf("the failed publish said: %s", reUnplaced.FindString(out))
	if l := regexp.MustCompile(`silt:v1:\S+`).FindString(out); l != "" {
		t.Fatalf("the failed publish printed a link (%s):\n%s", l, out)
	}
	time.Sleep(2 * time.Second)
	if after := committedEntries(t, storeA); after != before {
		t.Fatalf("the failed publish left a registry entry: committed entries %s -> %s", before, after)
	}

	for _, d := range holders {
		if err := d.cmd.Process.Signal(syscall.SIGCONT); err != nil {
			t.Fatal(err)
		}
	}
	out = runClient(t, add...)
	link := reLink.FindString(strings.TrimSpace(out))
	if link == "" {
		t.Fatalf("the retry printed no link:\n%s", out)
	}
	a.waitFor(t, reCommitted, 20*time.Second)
	dst := filepath.Join(t.TempDir(), "fetched.bin")
	runClientStderr(t, "swarm", "get", link, "-o", dst, "-peers", bootstrapA, "-registry", regRef)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("the retried publish did not fetch bit-perfect: got %d bytes, want %d", len(got), len(want))
	}
}
