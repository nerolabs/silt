package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/chainstore"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

var (
	reSignRecorded   = regexp.MustCompile(`sign: mark recorded height=(\d+) round=(\d+) phase=(\d+) hash=([0-9a-f]{64})`)
	reSavedBlocks    = regexp.MustCompile(`chain: saved (\d+) block\(s\)`)
	reRestoredBlocks = regexp.MustCompile(`chain: restored (\d+) block\(s\) from disk`)
	reSlashedCount   = regexp.MustCompile(`slashed:\s+(\d+) identities`)
)

// last returns the submatches of the most recent line matching re.
func (b *lineBuffer) last(re *regexp.Regexp) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := len(b.lines) - 1; i >= 0; i-- {
		if m := re.FindStringSubmatch(b.lines[i]); m != nil {
			return m
		}
	}
	return nil
}

func (b *lineBuffer) count(re *regexp.Regexp) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, ln := range b.lines {
		if re.MatchString(ln) {
			n++
		}
	}
	return n
}

// A validator SIGKILLed while it holds a signature for a height its chain has not
// committed restarts with the same command line, no human step, from its own disk:
// it restores at least the blocks it had saved, reloads its sign mark, rejoins, and
// never contradicts a signature it made before the crash.
//
// The kill is aimed: the victim is killed the moment its on-disk mark is its own
// prepare for a block it proposed, at a height above its committed head. Each crash
// lands between that signature and its commit, at the one slot where a restarted
// node with a new mempool would build a different block.
// A second validator is stopped across the crash, so that height cannot commit
// without the victim and the restarted process must sign at it again. A drill whose
// restarts never re-signed at the crash height has not driven the property and fails.
//
// Graded on the victim's own audit trail: every signature it releases is logged
// after its mark is durable, across every process lifetime in one file. No slot
// (height, round, phase) may carry two hashes, counting each crash mark as signed.
// Also: no validator slashes anyone, and the four replicas agree.
func TestValidatorKilledMidConsensusRestartsWithoutContradictingItself(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	// The victim hosts the registry, so it is the node publishes reach first.
	const N, victim, staller, kills = 4, 0, 3, 2
	const rejoinBound = 480 * time.Second
	ids := make([]string, N)
	port := make([]string, N)
	stores := make([]string, N)
	for i := 0; i < N; i++ {
		id := identity.FromSeed(int64(7400 + i))
		ids[i] = id.NodeID().String()
		port[i] = reservePort(t)
		stores[i] = t.TempDir()
	}
	anchors := strings.Join(ids, ",")
	regPort := reservePort(t)
	argsFor := func(i int) []string {
		att := make([]string, 0, N-1)
		persistent := make([]string, 0, N-1)
		for j := 0; j < N; j++ {
			if j != i {
				att = append(att, ids[j])
				persistent = append(persistent, ids[j]+"@"+port[j])
			}
		}
		args := []string{
			"-id-seed", fmt.Sprintf("%d", 7400+i),
			"-listen", port[i], "-advertise", port[i],
			"-store", stores[i],
			"-validator", "-objective",
			"-min-bond", "1M", "-min-bond-floor", "0", "-bond", "8M",
			"-mature-validators", fmt.Sprintf("%d", N),
			"-anchors", anchors,
			"-attesters", strings.Join(att, ","),
			"-persistent-peers", strings.Join(persistent, ","),
			"-quorum", "2", "-bond-audit", "2s",
			"-capacity", "1G", "-mdns=false",
		}
		if i == 0 {
			args = append(args, "-serve-registry", regPort)
		} else {
			args = append(args, "-bootstrap", ids[0]+"@"+port[0])
		}
		if i == victim {
			args = append(args, "-log", "debug")
		}
		return args
	}
	daemons := make([]*daemon, N)
	for i := 0; i < N; i++ {
		daemons[i] = startDaemon(t, fmt.Sprintf("val%d", i), argsFor(i)...)
	}
	regRef := daemons[0].waitFor(t, reRegistry, 30*time.Second)[1]
	peerList := make([]string, N)
	for i := range ids {
		peerList[i] = ids[i] + "@" + port[i]
	}
	peers := strings.Join(peerList, ",")

	// Keep consensus busy: a publisher that commits an entry about once a second.
	var stop atomic.Bool
	pubDone := make(chan struct{})
	go func() {
		defer close(pubDone)
		for k := 0; !stop.Load(); k++ {
			src := filepath.Join(t.TempDir(), fmt.Sprintf("p%d.bin", k))
			_ = os.WriteFile(src, []byte(fmt.Sprintf("crash-mid-consensus payload %d", k)), 0o644)
			_, _ = runClientAllowErr(t, "swarm", "add", src, "-peers", peers, "-registry", regRef,
				"-token-quorum", "2", "-chunk-size", "65536", "-mode", "convergent")
			time.Sleep(300 * time.Millisecond)
		}
	}()
	t.Cleanup(func() { stop.Store(true); <-pubDone })
	daemons[0].waitFor(t, reCommitted, 90*time.Second)

	markPath := filepath.Join(stores[victim], "signmark.json")
	logPath := filepath.Join(stores[victim], "debug.log")
	// ownProposal: the mark is the victim's prepare for a block it proposed. After a
	// restart its mempool is new, so a node that forgot this mark would build and
	// sign a different block in the same slot: the contradiction this drill grades.
	ownProposal := func(m ports.SignMark) bool {
		b, _ := os.ReadFile(logPath)
		want := fmt.Sprintf("propose: signed own proposal height=%d round=%d hash=%s ", m.Height, m.Round, m.Hash)
		return strings.Contains(string(b), want)
	}
	signLines := func() [][]string {
		b, _ := os.ReadFile(logPath)
		return reSignRecorded.FindAllStringSubmatch(string(b), -1)
	}
	type crash struct {
		mark  ports.SignMark
		saved int
		lines int // sign lines in the victim's log at the kill
	}
	var crashes []crash
	for k := 0; k < kills; k++ {
		v := daemons[victim]
		// Hold the staller still across the crash: with it and the victim both down,
		// two of four cannot commit, so the crash height waits for the victim.
		if err := daemons[staller].cmd.Process.Signal(syscall.SIGSTOP); err != nil {
			t.Fatal(err)
		}
		// Aim: wait until the victim's durable mark names a height its chain has not
		// committed, then kill it at once.
		var hit *crash
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) && hit == nil {
			m, ok, err := markstore.New(markPath).Load()
			s := v.out.last(reSavedBlocks)
			if err == nil && ok && s != nil {
				saved, _ := strconv.Atoi(s[1])
				if m.Height >= uint64(saved) && ownProposal(m) {
					if kerr := v.cmd.Process.Kill(); kerr != nil {
						t.Fatal(kerr)
					}
					_ = v.cmd.Wait()
					// Read back what survived the kill: the mark on disk is the
					// signature the restarted process must not contradict.
					m2, ok2, err2 := markstore.New(markPath).Load()
					if err2 != nil || !ok2 {
						t.Fatalf("crash %d: the sign mark is gone after SIGKILL (ok=%v err=%v)", k, ok2, err2)
					}
					hit = &crash{mark: m2, saved: saved, lines: len(signLines())}
				}
			}
			if hit == nil {
				time.Sleep(5 * time.Millisecond)
			}
		}
		if hit == nil {
			t.Fatalf("crash %d: the victim never held its own proposal's prepare above its committed head within 90s — the kill could not be aimed", k)
		}
		crashes = append(crashes, *hit)
		t.Logf("crash %d: killed holding a mark at h=%d r=%d phase=%d with %d block(s) saved",
			k, hit.mark.Height, hit.mark.Round, hit.mark.Phase, hit.saved)

		// Restart with the identical command line.
		daemons[victim] = startDaemon(t, fmt.Sprintf("val%d-restart%d", victim, k), argsFor(victim)...)
		r := daemons[victim].waitFor(t, reRestoredBlocks, 30*time.Second)
		if got, _ := strconv.Atoi(r[1]); got < hit.saved {
			t.Fatalf("crash %d: the restart restored %d block(s), fewer than the %d it had saved — it did not re-pin from its own disk", k, got, hit.saved)
		}
		if m := daemons[victim].out.find(reRefuse); m != nil {
			t.Fatalf("crash %d: the restart needed a human: %s", k, m[0])
		}
		// Rejoined: it commits again past the height it crashed holding. The
		// staller resumes once the restarted victim has signed at that height.
		want := hit.mark.Height + 1
		resumed := false
		restartedAt := time.Now()
		deadline = time.Now().Add(rejoinBound)
		for {
			if !resumed {
				for _, l := range signLines()[hit.lines:] {
					if h, _ := strconv.ParseUint(l[1], 10, 64); h >= hit.mark.Height {
						_ = daemons[staller].cmd.Process.Signal(syscall.SIGCONT)
						resumed = true
						break
					}
				}
			}
			if s := daemons[victim].out.last(reSavedBlocks); s != nil {
				if n, _ := strconv.Atoi(s[1]); uint64(n) > want {
					break
				}
			}
			if time.Now().After(deadline) {
				_ = daemons[staller].cmd.Process.Signal(syscall.SIGCONT)
				if b, err := os.ReadFile(logPath); err == nil {
					_ = os.WriteFile(filepath.Join(os.TempDir(), "crash-drill-victim-debug.log"), b, 0o644)
				}
				t.Fatalf("crash %d: the restarted victim never advanced past h=%d within %s (resigned at it: %v):\n%s", k, want, rejoinBound, resumed, daemons[victim].out.dump())
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Logf("crash %d: rejoined and committed past h=%d %s after the restart", k, hit.mark.Height, time.Since(restartedAt).Round(time.Second))
	}

	stop.Store(true)
	<-pubDone
	time.Sleep(3 * time.Second)
	for i, d := range daemons {
		if m := d.out.find(regexp.MustCompile(`chain: slashed equivocator \S+`)); m != nil {
			t.Fatalf("val%d slashed an equivocator during a crash drill with no Byzantine node: %s", i, m[0])
		}
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	}

	// Read every replica off its own disk.
	chains := make([][]chain.Block, N)
	for i := 0; i < N; i++ {
		bs, err := chainstore.Load(filepath.Join(stores[i], "chain.cbor"))
		if err != nil {
			t.Fatalf("val%d chain: %v", i, err)
		}
		chains[i] = bs
		out, _ := runClientAllowErr(t, "chain-status", "-store", stores[i])
		if m := reSlashedCount.FindStringSubmatch(out); m == nil || m[1] != "0" {
			t.Fatalf("val%d's committed history carries a slash (or reports none): %v\n%s", i, m, out)
		}
	}
	longest := chains[0]
	for _, c := range chains[1:] {
		if len(c) > len(longest) {
			longest = c
		}
	}
	for i, c := range chains {
		for h := range c {
			if c[h].Hash() != longest[h].Hash() {
				t.Fatalf("val%d disagrees with the longest replica at h=%d", i, h)
			}
		}
	}

	// The audit trail: every slot the victim ever signed, across all lifetimes.
	type slot struct {
		h, r uint64
		p    uint8
	}
	signed := map[slot]string{}
	for _, c := range crashes {
		signed[slot{c.mark.Height, c.mark.Round, c.mark.Phase}] = c.mark.Hash.String()
	}
	all := signLines()
	for _, l := range all {
		h, _ := strconv.ParseUint(l[1], 10, 64)
		r, _ := strconv.ParseUint(l[2], 10, 64)
		p, _ := strconv.ParseUint(l[3], 10, 8)
		k := slot{h, r, uint8(p)}
		if prev, ok := signed[k]; ok && prev != l[4] {
			t.Fatalf("the victim signed h=%d r=%d phase=%d TWICE: %s and %s", h, r, p, prev, l[4])
		}
		signed[k] = l[4]
	}
	for k, c := range crashes {
		next := len(all)
		if k+1 < len(crashes) {
			next = crashes[k+1].lines
		}
		again := 0
		for _, l := range all[c.lines:next] {
			if h, _ := strconv.ParseUint(l[1], 10, 64); h == c.mark.Height {
				again++
			}
		}
		if again == 0 {
			t.Fatalf("crash %d: the restarted victim never signed at the crash height %d — the drill did not drive a re-sign", k, c.mark.Height)
		}
		t.Logf("crash %d: after the restart the victim signed %d time(s) at h=%d, none contradicting", k, again, c.mark.Height)
	}
	t.Logf("%d crash(es), %d signatures audited across %d process lifetimes; %d blocks agreed on four replicas",
		len(crashes), len(all), len(crashes)+1, len(longest))
}
