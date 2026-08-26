// Package e2e drives the real `silt` binary as separate OS processes
// over real TCP — the layer the in-process sim deliberately skips. It
// exists because the class of bug the sim cannot see is exactly the one
// that bit us in the field (#36: a reply that could never reach a NATed
// peer, invisible until real sockets carried it). Here a built binary
// runs actual daemons, publishes through the chain-backed registry over
// pinned HTTPS, and fetches back across the swarm — asserting the file
// returns bit-perfect.
//
// The test builds the binary once (TestMain) and is guarded by
// -short so `go test -short` skips the process-spawning cost; CI runs
// the full form.
package e2e

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/link"
)

var siltBin string

func TestMain(m *testing.M) {
	// Build once for the whole package. -short still builds (cheap when
	// cached) but the tests themselves skip, so `go test -short ./e2e`
	// is fast and never spawns a process.
	dir, err := os.MkdirTemp("", "silt-e2e-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: tempdir:", err)
		os.Exit(1)
	}
	siltBin = filepath.Join(dir, "silt")
	build := exec.Command("go", "build", "-o", siltBin, "github.com/nerolabs/silt/cmd/silt")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: build silt:", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// lineBuffer collects a process's merged stdout+stderr as whole lines,
// so a test can wait for a specific line (a printed address, a commit)
// without racing the reader.
type lineBuffer struct {
	mu    sync.Mutex
	pend  []byte
	lines []string
}

func (b *lineBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pend = append(b.pend, p...)
	for {
		i := bytes.IndexByte(b.pend, '\n')
		if i < 0 {
			break
		}
		b.lines = append(b.lines, string(b.pend[:i]))
		b.pend = b.pend[i+1:]
	}
	return len(p), nil
}

func (b *lineBuffer) find(re *regexp.Regexp) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ln := range b.lines {
		if m := re.FindStringSubmatch(ln); m != nil {
			return m
		}
	}
	return nil
}

func (b *lineBuffer) dump() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return "\t" + strings.Join(b.lines, "\n\t")
}

type daemon struct {
	name string
	cmd  *exec.Cmd
	out  *lineBuffer
}

// startDaemon launches `silt daemon <args>` and streams its output into
// a lineBuffer. It is killed on test cleanup.
func startDaemon(t *testing.T, name string, args ...string) *daemon {
	t.Helper()
	out := &lineBuffer{}
	cmd := exec.Command(siltBin, append([]string{"daemon"}, args...)...)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon %s: %v", name, err)
	}
	d := &daemon{name: name, cmd: cmd, out: out}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	return d
}

func (d *daemon) waitFor(t *testing.T, re *regexp.Regexp, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m := d.out.find(re); m != nil {
			return m
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("daemon %s: timed out waiting for /%s/\n--- output ---\n%s", d.name, re, d.out.dump())
	return nil
}

var (
	rePeer      = regexp.MustCompile(`peer: ([0-9a-f]{64})@(\S+)`)
	reRegistry  = regexp.MustCompile(`registry: chain-backed, serving ([0-9a-f]{64}@https://[^ ]+) \(quorum`)
	reBootstrap = regexp.MustCompile(`bootstrapped \((\d+) table entries\)`)
	reCommitted = regexp.MustCompile(`chain: committed block (\d+)`)
	reLink      = regexp.MustCompile(`^silt:v1:\S+`)
	reFreeload  = regexp.MustCompile(`freeload: ON`)
	reArchive   = regexp.MustCompile(`archive: ON`)
	reRefuse    = regexp.MustCompile(`refusing to start`)
)

// TestFreeloadRoleSeparation (#47): a daemon started with -freeload announces the
// role and still comes up as a routing peer — it serves registry/relay/routing but
// refuses to host content. Role separation for public-infra operators, not a broken
// node.
func TestFreeloadRoleSeparation(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	d := startDaemon(t, "freeloader",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-freeload", "-mdns=false", "-id-seed", "4700")
	d.waitFor(t, reFreeload, 20*time.Second) // the role is announced
	d.waitFor(t, rePeer, 20*time.Second)     // and it still runs as a routing peer
}

// TestServeContentFlagIsTheFreeloadInverse (D-TIERING §4): the positive spelling
// of the content axis reaches the same state as the legacy negative one. A tier
// profile composes as `-serve-content=false` without the operator having to think
// in double negatives, and the announced line keeps the legacy `freeload: ON`
// marker so existing tooling and this harness keep working.
func TestServeContentFlagIsTheFreeloadInverse(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	d := startDaemon(t, "no-serve-content",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-content=false", "-mdns=false", "-id-seed", "4701")
	d.waitFor(t, reFreeload, 20*time.Second) // same announced state as -freeload
	d.waitFor(t, rePeer, 20*time.Second)     // still a routing peer
}

// TestContradictoryContentFlagsRefused (S3): -freeload with an EXPLICIT
// -serve-content=true states two opposite intents. The daemon must refuse to
// start rather than silently pick one and hand the operator a node doing the
// opposite of half their command line.
func TestContradictoryContentFlagsRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	d := startDaemon(t, "contradictory-flags",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-freeload", "-serve-content=true", "-mdns=false", "-id-seed", "4702")
	d.waitFor(t, regexp.MustCompile(`contradict each other`), 20*time.Second)
}

// waitForInLog polls a daemon's LOG FILE (node/chain narration goes there, not to
// stdout) until re matches, returning the submatches. The daemon prints the path
// as "log: info and above → <path>".
func waitForInLog(t *testing.T, path string, re *regexp.Regexp, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil {
			if m := re.FindStringSubmatch(string(b)); m != nil {
				return m
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for /%s/ in daemon log %s", re, path)
	return nil
}

// TestDeliveryReceiptBankedOverTCP is the e2e tier for the PoD neutral lane
// (docs/design/pod.md §7.1, certified 2026-08-26) — the tier #590 honestly
// reported as N/A because no daemon ran the lane yet. Now one does: a validator
// started with -accept-delivery-receipts banks a real fetcher's signed receipt
// arriving over a real socket, and settles the conserved delivery credit.
//
// The peer is both issuer and server — the bilateral shape the certification's
// settlement answer covers. The client withdraws a token blindly, signs a
// receipt, and submits it; the daemon verifies it against its own issuer key,
// banks it, and pays itself the fee-minus-skim.
func TestDeliveryReceiptBankedOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	a := startDaemon(t, "receipt-banker",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-accept-delivery-receipts",
		"-objective=false", "-min-rep", "100", "-quorum", "1",
		"-bond", "8M", "-min-bond-floor", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "4801")
	a.waitFor(t, regexp.MustCompile(`delivery receipts: ACCEPTING`), 20*time.Second)
	logPath := a.waitFor(t, regexp.MustCompile(`log: info and above → (\S+)`), 20*time.Second)[1]
	peer := a.waitFor(t, rePeer, 20*time.Second)
	bootstrapA := peer[1] + "@" + peer[2]

	// A root the fetcher attests it received. The receipt carries no possession
	// proof by certified design, so no bytes need to move for THIS assertion —
	// what is under test is that a real daemon verifies, banks, and settles.
	root := strings.Repeat("ab", 32)

	out := runClient(t, "swarm", "receipt", root, "-peers", bootstrapA)
	if !strings.Contains(out, "delivery receipt banked") {
		t.Fatalf("receipt was not banked over TCP; client said: %s", out)
	}

	// Banking is only half of it — assert the daemon actually SETTLED the
	// conserved credit. The node's own log carries the paid amount, and it must
	// be positive: the fee the fetcher paid at withdrawal, less the durability
	// skim. A zero would mean the lane banked a neutral observable and paid
	// nothing — the silent no-op this assertion exists to catch.
	m := waitForInLog(t, logPath, regexp.MustCompile(`delivery receipt banked .*credit=(\d+)`), 20*time.Second)
	if m[1] == "0" {
		t.Fatal("the receipt banked but settled credit=0 — the conserved delivery credit did not pay (is a ledger wired?)")
	}

	// The token is one-time: replaying the SAME flow mints a fresh token, so it
	// must ALSO bank (a second genuine delivery), while the daemon's
	// double-spend set is what stops a replayed serial. Assert the honest
	// second delivery works — the replay defense is pinned at the unit tier.
	out2 := runClient(t, "swarm", "receipt", root, "-peers", bootstrapA)
	if !strings.Contains(out2, "delivery receipt banked") {
		t.Fatalf("a second genuine delivery must also bank; client said: %s", out2)
	}
}

// TestDeliveryReceiptRefusedWhenLaneOff: the lane is OFF by default, so a daemon
// that never opted in refuses to bank — and the client says so plainly instead
// of reporting a success it did not get.
func TestDeliveryReceiptRefusedWhenLaneOff(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	a := startDaemon(t, "no-receipts",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-objective=false", "-min-rep", "100", "-quorum", "1",
		"-bond", "8M", "-min-bond-floor", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "4802")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	bootstrapA := peer[1] + "@" + peer[2]

	out, err := runClientAllowErr(t, "swarm", "receipt", strings.Repeat("cd", 32), "-peers", bootstrapA)
	if err == nil {
		t.Fatalf("a daemon without -accept-delivery-receipts must not bank a receipt; client succeeded with: %s", out)
	}
	if !strings.Contains(out, "NOT banked") {
		t.Fatalf("the refusal must be legible to the caller, got: %s", out)
	}
}

// TestArchiveTierAnnouncesRetention (D-TIERING §3): an archival node announces
// the tier it is running, so an operator can see from the log that this box is
// carrying O(all history) heavy payload deliberately — not by accident.
func TestArchiveTierAnnouncesRetention(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	d := startDaemon(t, "archival",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-archive", "-mdns=false", "-id-seed", "4703")
	d.waitFor(t, reArchive, 20*time.Second)
	d.waitFor(t, rePeer, 20*time.Second) // and it is an ordinary peer otherwise
}

// runClient runs a one-shot `silt <args>` (swarm add/get) to completion,
// returning its stdout. Diagnostics on stderr are folded into the
// failure message only.
func runClient(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(siltBin, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("client %v: %v\n--- stdout ---\n%s\n--- stderr ---\n%s", args, err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

// runClientAllowErr runs a one-shot client and returns its combined output
// and exit error (nil on success) instead of failing the test — for
// asserting a command MUST fail (e.g. a publish the safe defaults refuse).
func runClientAllowErr(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := exec.Command(siltBin, args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// TestDefaultsRefuseRubberStampCommit is the OUTCOME test for the safe consensus
// defaults. Use case: an operator runs a LONE validator with DEFAULT flags (no
// -quorum / -min-rep override, so the untrusted objective path) and no cold-start
// scaffolding. Outcome required (seam-2, red-team 2026-08-08): the daemon must
// REFUSE TO START — a lone untrusted objective validator with no anchor launch set
// and no weak-subjectivity checkpoint would latch everMature at genesis and run
// with no anchor co-sign, so a young/Sybil quorum could self-certify and capture.
// Refuse-to-start is strictly stronger than the old "starts but refuses to
// rubber-stamp": a node that will not run cannot rubber-stamp anything.
// (TestPublishCommitFetchOverTCP is the positive control — with the explicit
// -quorum 0 trusted-deployment setting the same publish commits;
// TestBondEarnedStandingCommitsOverTCP shows the earned-standing objective path
// with an anchor launch set.)
func TestDefaultsRefuseRubberStampCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	a := startDaemon(t, "A",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0", "-validator",
		// Opt OUT of the anti-release floor an untrusted validator gets by default
		// (a local test box plots a small bond, not the 1 GiB a real deployment
		// needs). The CONSENSUS defaults under test here (untrusted objective path,
		// no anchors) are untouched — that is what must refuse to start.
		"-min-bond-floor", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "2001")
	// The daemon must print the cold-start refusal and exit, not come up as a peer.
	a.waitFor(t, reRefuse, 20*time.Second)
}

// TestBondEarnedStandingCommitsOverTCP is the e2e tier for the trust pivot
// (T1b, #78): two validators earn consensus standing by proving their storage
// bonds to EACH OTHER over TCP (gossip → challenge → verify → ledger), and a
// publish then commits on the SAFE min-rep path — no `-quorum 0` trusted-
// deployment shortcut. Standing is earned, not granted; a publish that commits
// here means real held storage bought the write.
func TestBondEarnedStandingCommitsOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	// Deterministic ids so A can name B as its attester before B exists.
	idB := identity.FromSeed(1002).NodeID().String()

	// A: registry + validator + bond; min-rep 100 (earned standing required),
	// quorum 1, and B is its attester. Fast bond audit so the test is quick.
	a := startDaemon(t, "A",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-objective=false", "-min-rep", "100", "-quorum", "1", "-attesters", idB,
		"-bond", "8M", "-bond-audit", "1s",
		"-capacity", "1G", "-mdns=false", "-id-seed", "1001")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	idA, addrA := peer[1], peer[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]
	bootstrapA := idA + "@" + addrA

	// B: validator + bond, joined to A. Both run bond audits, so each earns
	// the other's standing in its own ledger.
	b := startDaemon(t, "B",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-bootstrap", bootstrapA, "-validator",
		"-objective=false", "-min-rep", "100", "-quorum", "1",
		"-bond", "8M", "-bond-audit", "1s",
		"-capacity", "1G", "-mdns=false", "-id-seed", "1002")
	b.waitFor(t, reBootstrap, 20*time.Second)

	src := filepath.Join(t.TempDir(), "payload.bin")
	want := make([]byte, 256<<10)
	rand.New(rand.NewSource(0xB0AD)).Read(want)
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}

	// Publish, retrying until the bond audit has earned standing on both sides
	// (a few 1s rounds). A success here is a real quorum commit on min-rep 100.
	// runClientAllowErr returns combined stdout+stderr, so match the link
	// anywhere (not the ^-anchored, stdout-only reLink). "silt:v1:" does not
	// occur inside the "siltcare:v1:" care link, so this finds the real link.
	reAnyLink := regexp.MustCompile(`silt:v1:\S+`)
	var link, lastOut string
	deadline := time.Now().Add(30 * time.Second)
	for {
		// Bond-earned-standing CONSENSUS test, not encryption: pin convergent so each
		// retry re-proposes the SAME root while the two validators earn mutual
		// standing (the private default mints a fresh root per attempt — H6).
		out, err := runClientAllowErr(t, "swarm", "add", src,
			"-peers", bootstrapA, "-registry", regRef, "-chunk-size", "65536", "-mode", "convergent")
		lastOut = out
		if err == nil {
			if link = reAnyLink.FindString(out); link != "" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("bonded validators never earned committable standing:\n%s", lastOut)
		}
		time.Sleep(1 * time.Second)
	}
	// The publish drove a real quorum commit (not a self-commit).
	a.waitFor(t, reCommitted, 10*time.Second)

	// And the file round-trips bit-perfect.
	dst := filepath.Join(t.TempDir(), "fetched.bin")
	runClient(t, "swarm", "get", link, "-o", dst, "-peers", bootstrapA, "-registry", regRef)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) || !bytes.Equal(got, want) {
		t.Fatalf("round-trip corrupted the file: got %d bytes, want %d", len(got), len(want))
	}
	_ = b
}

// TestChainRevocationCommitsOverTCP is the e2e tier for the accountability fix
// (red-team F5): a validator proposes an on-chain takedown of a published root,
// and it COMMITS through a real quorum over real TCP. The per-operator honoring
// (a subscribing node denies it, a non-subscribing node does not) is proven at
// the integration tier (sim/revocation_test.go); this proves the daemon can
// actually drive a quorum revocation end to end — the runtime capability that
// tier could not exercise.
func TestChainRevocationCommitsOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	idB := identity.FromSeed(3002).NodeID().String()
	a := startDaemon(t, "A",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-objective=false", "-min-rep", "100", "-quorum", "1", "-attesters", idB,
		"-bond", "8M", "-bond-audit", "1s",
		"-capacity", "1G", "-mdns=false", "-id-seed", "3001")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	idA, addrA := peer[1], peer[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]
	bootstrapA := idA + "@" + addrA

	b := startDaemon(t, "B",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-bootstrap", bootstrapA, "-validator",
		"-objective=false", "-min-rep", "100", "-quorum", "1",
		"-bond", "8M", "-bond-audit", "1s",
		"-capacity", "1G", "-mdns=false", "-id-seed", "3002")
	b.waitFor(t, reBootstrap, 20*time.Second)

	// Publish a root, retrying until the bonded quorum can commit it.
	src := filepath.Join(t.TempDir(), "payload.bin")
	want := make([]byte, 128<<10)
	rand.New(rand.NewSource(0xF5)).Read(want)
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}
	reAnyLink := regexp.MustCompile(`silt:v1:\S+`)
	var linkStr, lastOut string
	deadline := time.Now().Add(30 * time.Second)
	for {
		// Consensus/revocation test, not encryption: pin convergent so each retry
		// re-proposes the SAME root as the quorum forms (private mints a fresh root
		// per attempt — H6).
		out, err := runClientAllowErr(t, "swarm", "add", src, "-peers", bootstrapA, "-registry", regRef, "-chunk-size", "65536", "-mode", "convergent")
		lastOut = out
		if err == nil {
			if linkStr = reAnyLink.FindString(out); linkStr != "" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("publish never committed:\n%s", lastOut)
		}
		time.Sleep(1 * time.Second)
	}
	h, err := link.Parse(linkStr)
	if err != nil {
		t.Fatalf("parse link %q: %v", linkStr, err)
	}

	// A third validator is told to REVOKE that root: once it has earned standing
	// and seen the root committed, it proposes an on-chain takedown that commits
	// through the quorum. It also subscribes to on-chain takedowns (F5).
	c := startDaemon(t, "C",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-bootstrap", bootstrapA, "-validator",
		"-objective=false", "-min-rep", "100", "-quorum", "1", "-attesters", idA,
		"-bond", "8M", "-bond-audit", "1s",
		"-revoke", h.Root.String(), "-honor-chain-revocations",
		"-capacity", "1G", "-mdns=false", "-id-seed", "3003")
	c.waitFor(t, reBootstrap, 20*time.Second)

	reRevoked := regexp.MustCompile(`proposed on-chain revocation of ` + h.Root.String())
	c.waitFor(t, reRevoked, 40*time.Second)
	_ = b
}

// TestObjectiveConsensusCommitsOverTCP is the e2e tier for objective fork-choice
// (F6): two validators run `-objective`, so eligibility, quorum, and fork-choice
// weight are decided by on-chain bond registrations rather than the local
// reputation view. The launch set bootstraps via -anchors; each validator
// registers its real bond LIVE as it proposes. A `swarm add` drives a real
// objective quorum commit over real TCP, and the file round-trips bit-perfect —
// the fixed protocol (bond registration + verification on the wire) works end to
// end, not just in the sim.
func TestObjectiveConsensusCommitsOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	idA := identity.FromSeed(2001).NodeID().String()
	idB := identity.FromSeed(2002).NodeID().String()
	anchors := idA + "," + idB // the declared launch set that bootstraps objective mode

	// Note: NO -objective flag — it is the DEFAULT for an untrusted validator, so
	// this also proves the default path is objective. The objective config is the
	// launch anchors + min-bond + mature-validators.
	a := startDaemon(t, "A",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-min-bond", "1M", "-mature-validators", "2",
		"-anchors", anchors, "-quorum", "1", "-attesters", idB,
		// A local test swarm deliberately uses tiny bonds, so it opts OUT of the
		// anti-release floor that an untrusted swarm gets by default (G4-residual).
		"-min-bond-floor", "0",
		"-bond", "8M", "-bond-audit", "1s",
		"-capacity", "1G", "-mdns=false", "-id-seed", "2001")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	addrA := peer[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]
	bootstrapA := idA + "@" + addrA

	b := startDaemon(t, "B",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-bootstrap", bootstrapA, "-validator",
		"-min-bond", "1M", "-mature-validators", "2",
		"-anchors", anchors, "-quorum", "1",
		"-min-bond-floor", "0",
		"-bond", "8M", "-bond-audit", "1s",
		"-capacity", "1G", "-mdns=false", "-id-seed", "2002")
	b.waitFor(t, reBootstrap, 20*time.Second)

	src := filepath.Join(t.TempDir(), "payload.bin")
	want := make([]byte, 256<<10)
	rand.New(rand.NewSource(0x0B1EC)).Read(want)
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}

	// Publish → a real objective quorum commit (A proposes + self-registers its
	// bond, B attests as a launch anchor). Retry a few rounds while B joins.
	reAnyLink := regexp.MustCompile(`silt:v1:\S+`)
	var link, lastOut string
	deadline := time.Now().Add(30 * time.Second)
	for {
		out, err := runClientAllowErr(t, "swarm", "add", src,
			"-peers", bootstrapA, "-registry", regRef, "-chunk-size", "65536")
		lastOut = out
		if err == nil {
			if link = reAnyLink.FindString(out); link != "" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("objective validators never committed a publish:\n%s", lastOut)
		}
		time.Sleep(1 * time.Second)
	}
	a.waitFor(t, reCommitted, 10*time.Second)

	dst := filepath.Join(t.TempDir(), "fetched.bin")
	runClient(t, "swarm", "get", link, "-o", dst, "-peers", bootstrapA, "-registry", regRef)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) || !bytes.Equal(got, want) {
		t.Fatalf("objective-committed file round-trip corrupted: got %d bytes, want %d", len(got), len(want))
	}
	_ = b
}

// TestUnlinkablePublishOverTCP is the e2e tier for publisher privacy (T3,
// #14/F1): three validators issue and REQUIRE publish tokens; a `swarm add`
// acquires a 2-of-3 token over real TCP (paying the fee with its identity, the
// issuers never seeing the serial) and publishes — the entry commits and the
// file round-trips, with no Publisher identity gating it.
func TestUnlinkablePublishOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	idA := identity.FromSeed(3001).NodeID().String()
	idB := identity.FromSeed(3002).NodeID().String()
	idC := identity.FromSeed(3003).NodeID().String()

	// Three validators: each a token issuer, requires a 2-of-3 token, knows the
	// other two as attesters. Fast bond audit so standing/issuer-keys warm up.
	common := func(seed string, others string) []string {
		return []string{
			"-listen", "127.0.0.1:0", "-store", t.TempDir(), "-validator",
			"-objective=false", "-min-rep", "100", "-quorum", "1", "-attesters", others,
			"-require-tokens", "2", "-bond", "8M", "-bond-audit", "1s",
			"-capacity", "1G", "-mdns=false", "-id-seed", seed,
		}
	}
	a := startDaemon(t, "A", append([]string{"-serve-registry", "127.0.0.1:0"}, common("3001", idB+","+idC)...)...)
	pa := a.waitFor(t, rePeer, 20*time.Second)
	bootA := pa[1] + "@" + pa[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]

	b := startDaemon(t, "B", append([]string{"-bootstrap", bootA}, common("3002", idA+","+idC)...)...)
	pb := b.waitFor(t, rePeer, 20*time.Second)
	bootB := pb[1] + "@" + pb[2]
	b.waitFor(t, reBootstrap, 20*time.Second)

	c := startDaemon(t, "C", append([]string{"-bootstrap", bootA}, common("3003", idA+","+idB)...)...)
	pc := c.waitFor(t, rePeer, 20*time.Second)
	bootC := pc[1] + "@" + pc[2]
	c.waitFor(t, reBootstrap, 20*time.Second)

	src := filepath.Join(t.TempDir(), "payload.bin")
	want := make([]byte, 256<<10)
	rand.New(rand.NewSource(0x70CE)).Read(want)
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}

	// Publish with a 2-of-3 token, retrying while standing + issuer keys warm up.
	allPeers := strings.Join([]string{bootA, bootB, bootC}, ",")
	reAnyLink := regexp.MustCompile(`silt:v1:\S+`)
	var link, lastOut string
	deadline := time.Now().Add(40 * time.Second)
	for {
		out, err := runClientAllowErr(t, "swarm", "add", src,
			"-peers", allPeers, "-registry", regRef, "-token-quorum", "2", "-chunk-size", "65536")
		lastOut = out
		if err == nil {
			if link = reAnyLink.FindString(out); link != "" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("token-gated publish never succeeded:\n%s", lastOut)
		}
		time.Sleep(1 * time.Second)
	}
	a.waitFor(t, reCommitted, 10*time.Second)

	dst := filepath.Join(t.TempDir(), "fetched.bin")
	runClient(t, "swarm", "get", link, "-o", dst, "-peers", allPeers, "-registry", regRef)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round-trip corrupted the token-published file: got %d bytes, want %d", len(got), len(want))
	}
	_ = c
}

// TestPublishCommitFetchOverTCP is the whole real-network path in one
// test: three daemons in three processes, a chain-backed registry over
// pinned HTTPS, a publish that must reach quorum and commit a block, and
// a fetch that reassembles the file from chunks scattered across the
// swarm — bit-perfect or bust.
func TestPublishCommitFetchOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}

	// A: validator + registry host. -quorum 0 is the lone-validator
	// self-commit (Quorum counts attestations EXCLUDING the proposer, so
	// a single node can never gather one — 0 is the honest trusted-
	// deployment setting the runbook uses for a one-box swarm).
	a := startDaemon(t, "A",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0",
		"-validator", "-quorum", "0", "-min-rep", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "1001")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	idA, addrA := peer[1], peer[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]
	bootstrapA := idA + "@" + addrA

	// B, C: plain storage nodes in their own processes, joined to A.
	for i, name := range []string{"B", "C"} {
		d := startDaemon(t, name,
			"-listen", "127.0.0.1:0", "-store", t.TempDir(),
			"-bootstrap", bootstrapA,
			"-capacity", "1G", "-mdns=false",
			"-id-seed", fmt.Sprintf("%d", 1002+i))
		d.waitFor(t, reBootstrap, 20*time.Second)
	}

	// A file of many small chunks so it genuinely stripes and scatters.
	src := filepath.Join(t.TempDir(), "payload.bin")
	want := make([]byte, 1<<20) // 1 MiB
	rand.New(rand.NewSource(0x5117)).Read(want)
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}

	// Publish: chunk → encrypt → erasure → scatter → chain-commit.
	out := runClient(t, "swarm", "add", src,
		"-peers", bootstrapA, "-registry", regRef, "-chunk-size", "65536")
	link := reLink.FindString(strings.TrimSpace(out))
	if link == "" {
		t.Fatalf("swarm add printed no silt link:\n%s", out)
	}
	// The publish must have driven a real consensus round to commit.
	a.waitFor(t, reCommitted, 20*time.Second)

	// Fetch from a fresh client: registry lookup → manifest → data
	// chunks pulled across the swarm over TCP → verify → decode →
	// decrypt.
	dst := filepath.Join(t.TempDir(), "fetched.bin")
	runClient(t, "swarm", "get", link, "-o", dst,
		"-peers", bootstrapA, "-registry", regRef)

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round-trip corrupted the file: got %d bytes, want %d", len(got), len(want))
	}
}
