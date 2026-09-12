// Package shippeddefault is THE SHIPPED-DEFAULT GRADED LANE (ROADMAP row F2,
// ratified `D-STRUCTURAL-GATES-2026-09-12`; canon calls it "G-1" and that letter
// COLLIDES with R-membership's own, different, closed G-1 — this package never
// uses the letter).
//
// THE RULE IT ENFORCES: at least one graded lane runs the configuration a stock
// operator gets, with nothing turned on or off to make the test convenient.
//
// ─────────────────────────────────────────────────────────────────────────────
// WHAT THIS LANE COVERS, AND WHAT IT DOES NOT
//
// COVERS:
//   - The two published `silt daemon` entry points a stock operator actually
//     types — the edge node (`silt daemon`) and the validator (`silt daemon
//     -validator`) — executed as the REAL built binary in a subprocess, with
//     argv carrying no flag beyond the role selector.
//   - The START-UP outcome of each: whether the process comes up or refuses,
//     its exit code, and the operator-visible text it prints.
//   - A structural census of every OTHER tracked integration lane, asserting
//     that none of them runs the anti-release floor DERIVED — which is the
//     measured evidence behind row F2's claim that every existing gate
//     configures its way out of the posture it should be testing.
//
// DOES NOT COVER — say it plainly rather than let the name over-claim:
//   - Any multi-node behaviour. This lane starts ONE process at a time. It has
//     no swarm, no chain, no commit, no quorum, no peer. It cannot see a
//     consensus property and must never be cited for one.
//   - Any property past start-up. The edge node is observed to reach "serving"
//     and is then killed. Nothing about steady state is asserted.
//   - What happens AFTER the bond floor is cleared. MEASURED OUT OF BAND on
//     2026-09-12 at b870ade, and recorded here because it corrects ROADMAP row
//     F3's stated remedy: raising `-bond` past the floor does NOT make the stock
//     validator posture start. A SECOND refusal sits behind the first —
//     `silt daemon -validator -bond 1100M` exits 1 with "consensus: refusing to
//     start — an untrusted objective validator with no cold-start scaffolding
//     would treat itself as mature from genesis". That refusal is CORRECT (it is
//     the M0 cold-start capture defense) and it is already gated behaviourally by
//     cmd/silt TestInvariantB_S6_ColdStartScaffoldRefusedByDefault, which drives
//     coldStartScaffoldOK in both polarities. It cannot be satisfied by any
//     default, because -anchors and -ws-checkpoint carry NETWORK-SPECIFIC values.
//     So "raise the default so the stock validator starts" is not achievable as
//     written; what a raise buys is the removal of a SPURIOUS refusal, leaving
//     the substantive one. This lane does not assert it, because asserting it
//     would require passing `-bond` — the exact behaviour the lane forbids.
//   - Whether the shipped defaults are CORRECT. It asserts what they DO, not
//     what they SHOULD be. The `-bond` default raise is ratified as a direction
//     (`D-BOND-DEFAULT-CLEARS-FLOOR-2026-09-12`) and its VALUE is the owner's.
//   - Any lane that is not a tracked docker-compose or topology.py under
//     integration/. The census walks `git ls-files`, never the filesystem: a
//     gitignored stale artifact once turned local `main` RED on a SHA whose CI
//     was 14/14 (scar: a source gate walks gitignored artifacts).
//
// ─────────────────────────────────────────────────────────────────────────────
// THE ONE JUDGEMENT CALL, MADE VISIBLE
//
// `-validator` IS passed, and it is the only flag this lane may pass. That is
// not a convenience: it is the ROLE SELECTOR, the thing a stock operator types
// to run a validator, and it makes the lane HARDER rather than easier (it is
// what produces the refusal below). Every other flag is forbidden, and
// mustBeStockArgv enforces that at exec time on every single run — not in a
// separate test that could be reordered or deleted on its own.
package shippeddefault

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/bond"
)

// ─────────────────────────────────────────────────────────────────────────────
// THE STOCK POSTURE, DECLARED ONCE

// stockEdgeArgv is what a stock operator types to run an ordinary node.
var stockEdgeArgv = []string{"daemon"}

// stockValidatorArgv is what a stock operator types to run a validator.
var stockValidatorArgv = []string{"daemon", "-validator"}

// allowedFlags is the closed set of flags this lane may ever pass. It has
// exactly one member and the reason is stated in the package comment. Adding to
// it is the failure this gate exists to catch, so the guard names that.
var allowedFlags = map[string]bool{"-validator": true}

// mustBeStockArgv refuses any argv carrying a flag outside allowedFlags. It runs
// on EVERY exec rather than as its own test: a separate assertion can be
// skipped, reordered, or deleted while the lane keeps reporting green.
func mustBeStockArgv(argv []string) error {
	for _, a := range argv {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name := a
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		if !allowedFlags[name] {
			return fmt.Errorf(
				"THE SHIPPED-DEFAULT LANE WAS HANDED A FLAG: %q in argv %v. "+
					"This lane exists to run the posture a stock operator gets. Passing a flag to make it "+
					"start, pass, or run faster is precisely the behaviour ROADMAP row F2 forbids. If the "+
					"shipped default is wrong, change the default (owner's call) and update the pin below — "+
					"do not configure the lane out of the posture", a, argv)
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// THE REAL BINARY. Not a mock, not an in-process call — the shipped entry point.

var siltBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "shipped-default-lane")
	if err != nil {
		fmt.Fprintln(os.Stderr, "shipped-default lane: tempdir:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	siltBin = filepath.Join(dir, "silt")
	build := exec.Command("go", "build", "-o", siltBin, "github.com/nerolabs/silt/cmd/silt")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		// Loud, not skipped. A lane that cannot build the thing it grades has
		// no verdict to report, and `t.Skip` would report green for zero
		// execution (scar: short-run-is-zero-execution).
		fmt.Fprintln(os.Stderr, "shipped-default lane: cannot build cmd/silt, so the lane has NO verdict:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

type runResult struct {
	argv     []string
	exitCode int
	stdout   string
	stderr   string
	started  bool // observed the daemon's own "serving" banner
	timedOut bool
}

// bannerWatcher is the daemon's stdout, watched live. The lane kills the process
// the instant it announces it is serving, rather than waiting out a timeout: a
// lane that costs 45 s per run gets turned off, and a gate that is off is worth
// nothing.
type bannerWatcher struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	want string
	once sync.Once
	hit  chan struct{}
}

func (w *bannerWatcher) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf.Write(p)
	seen := w.want != "" && strings.Contains(w.buf.String(), w.want)
	w.mu.Unlock()
	if seen {
		w.once.Do(func() { close(w.hit) })
	}
	return len(p), nil
}

func (w *bannerWatcher) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// runStockDaemon execs the real binary on a stock argv in an empty working
// directory. The working directory is NOT configuration: `-store` keeps its
// shipped default of ".silt-daemon", which is relative to wherever the operator
// runs the binary. An empty cwd is the honest equivalent of a fresh box.
func runStockDaemon(t *testing.T, argv []string, wait time.Duration, until string) runResult {
	t.Helper()
	if err := mustBeStockArgv(argv); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	cmd := exec.CommandContext(ctx, siltBin, argv...)
	cmd.Dir = t.TempDir()
	w := &bannerWatcher{want: until, hit: make(chan struct{})}
	var errb bytes.Buffer
	cmd.Stdout = w
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %v: %v", argv, err)
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	res := runResult{argv: argv}
	var err error
	select {
	case <-w.hit:
		res.started = true
		_ = cmd.Process.Kill()
		<-waitErr
	case err = <-waitErr:
	case <-ctx.Done():
		res.timedOut = true
		_ = cmd.Process.Kill()
		<-waitErr
	}
	res.stdout, res.stderr = w.String(), errb.String()
	if ee, ok := err.(*exec.ExitError); ok {
		res.exitCode = ee.ExitCode()
	} else if err != nil && !res.started && !res.timedOut {
		t.Fatalf("exec %v: %v\nstdout:\n%s\nstderr:\n%s", argv, err, res.stdout, res.stderr)
	}
	return res
}

// ─────────────────────────────────────────────────────────────────────────────
// THE POSITIVE CONTROL, FIRST.
//
// Without it, the refusal assertion below passes identically on a harness that
// can never start ANY daemon — a wedged box and a correct refusal look the same
// from the outside. That confusion has a scar in this repo (a gate passing on a
// bystander, third-time fired 2026-09-12), so the control is not optional.

func TestShippedDefaultLane_StockEdgeNodeStarts(t *testing.T) {
	res := runStockDaemon(t, stockEdgeArgv, 90*time.Second, "serving; Ctrl-C to stop")
	if !res.started {
		t.Fatalf("POSITIVE CONTROL FAILED: the stock edge posture %v did not reach 'serving'.\n"+
			"Until this passes, the refusal assertions in this file prove nothing — a harness that cannot "+
			"start any daemon reports a refusal identically to a correct one.\n"+
			"exit=%d timedOut=%v\nstdout:\n%s\nstderr:\n%s",
			res.argv, res.exitCode, res.timedOut, res.stdout, res.stderr)
	}
	t.Logf("stock edge posture %v STARTS on shipped defaults (reached 'serving', then killed).", res.argv)
}

// ─────────────────────────────────────────────────────────────────────────────
// THE LANE'S FIRST RESULT, PINNED.
//
// PINNED_DEFECT: on pure defaults the stock VALIDATOR posture REFUSES TO START.
// The `-bond` default (67,108,864 B) does not clear the derived anti-release
// floor (1,080,000,000 B), which defaults ON for an untrusted objective
// validator. This test asserts the defect as the CURRENT measured state, so that
// raising the default — ratified as a direction, value owed to the owner
// (`D-BOND-DEFAULT-CLEARS-FLOOR-2026-09-12`, ROADMAP row F3) — turns this RED
// and forces the record to be updated rather than silently drifting.
//
// The claim and the reference are taken from THREE independent places, never one
// (scar: a verifier that reads the claim and the root it checks against out of
// the same response is a tautology):
//   - the REFUSAL is the daemon's observed run-time behaviour;
//   - the `-bond` default is read from the flag's own declaration via the
//     binary's flag table, never from prose or a copied literal (row F4 exists
//     because docs name defaults that are not the shipped ones);
//   - the floor is re-derived here from core/bond.PlotSealThroughput.
func TestShippedDefaultLane_StockValidatorRefusesToStart_PINNED_DEFECT(t *testing.T) {
	// (a) The independent derivation of the floor. AntiReleaseComputeWindow is
	// 2s and unexported in package main, so the window is restated here as the
	// one literal this test owns; the THROUGHPUT, which is the number that could
	// actually move, is read from its shipped constant.
	const antiReleaseComputeWindowSeconds = 2
	const derivedFloorMargin = 2
	wantFloor := int64(derivedFloorMargin) * (int64(antiReleaseComputeWindowSeconds) * bond.PlotSealThroughput)
	if wantFloor != 1_080_000_000 {
		t.Fatalf("PIN MOVED: the derived anti-release floor re-computes to %d B, pinned at 1,080,000,000 B. "+
			"bond.PlotSealThroughput or the window changed; re-derive row F3 before touching this test", wantFloor)
	}

	// (b) The shipped -bond default, read from the flag declaration itself.
	bondDefault := shippedFlagDefault(t, "bond")
	gotBond, err := parseSiltSize(bondDefault)
	if err != nil {
		t.Fatalf("shipped -bond default %q: %v", bondDefault, err)
	}
	if gotBond != 67_108_864 {
		t.Fatalf("PIN MOVED — and this is the pin doing its job: the shipped -bond default is now %q = %d B, "+
			"pinned at \"64M\" = 67,108,864 B. If the default was raised per D-BOND-DEFAULT-CLEARS-FLOOR-2026-09-12, "+
			"update ROADMAP row F3 and this test TOGETHER", bondDefault, gotBond)
	}

	// (c) The composition that row F3 records, asserted rather than assumed.
	if gotBond >= wantFloor {
		t.Fatalf("PIN MOVED: the shipped -bond default (%d B) now CLEARS the derived floor (%d B). "+
			"Row F3's defect is fixed. Retire this PINNED_DEFECT, keep the positive assertion that the "+
			"stock validator STARTS, and update ROADMAP row F3 and D-BOND-DEFAULT-CLEARS-FLOOR-2026-09-12",
			gotBond, wantFloor)
	}

	// (d) The measured behaviour: the daemon exits rather than running unbonded.
	res := runStockDaemon(t, stockValidatorArgv, 60*time.Second, "serving; Ctrl-C to stop")
	if res.started {
		t.Fatalf("PIN MOVED: the stock validator posture %v STARTED. Row F3 records that it refuses. "+
			"stdout:\n%s", res.argv, res.stdout)
	}
	if res.timedOut {
		t.Fatalf("INDETERMINATE, not a pass: %v neither started nor exited inside the window. "+
			"stdout:\n%s\nstderr:\n%s", res.argv, res.stdout, res.stderr)
	}
	if res.exitCode != 1 {
		t.Fatalf("stock validator posture %v exited %d, pinned at 1\nstdout:\n%s\nstderr:\n%s",
			res.argv, res.exitCode, res.stdout, res.stderr)
	}

	// (e) The operator-visible reason. An announced operator string is a
	// contract (scar, count=2): if this text changes, the change is deliberate
	// and the record moves with it. ROADMAP row F5 queues a rewrite of the
	// RESTART error text, which is a different message from this one.
	const wantReason = "below the anti-release floor"
	if !strings.Contains(res.stderr, wantReason) {
		t.Fatalf("the stock validator refused for a DIFFERENT reason than row F3 records. "+
			"want stderr to contain %q\ngot stderr:\n%s\nstdout:\n%s", wantReason, res.stderr, res.stdout)
	}

	// (f) The floor the daemon ITSELF armed, cross-checked against (a). This is
	// the run-time value, not a constant that feeds it.
	gotFloorMiB := announcedFloorMiB(t, res.stdout)
	if wantMiB := int(wantFloor >> 20); gotFloorMiB != wantMiB {
		t.Fatalf("the daemon armed a floor of %d MiB; the independent derivation says %d MiB "+
			"(%d B >> 20). One of the two is wrong and neither may be trusted until that is settled",
			gotFloorMiB, wantMiB, wantFloor)
	}

	t.Logf("LANE RESULT (pinned defect): `silt daemon -validator` on pure defaults EXITS %d. "+
		"bond default %s = %d B < derived floor %d B (%d MiB armed by the daemon). Reason: %q",
		res.exitCode, bondDefault, gotBond, wantFloor, gotFloorMiB, strings.TrimSpace(res.stderr))
}

// TestShippedDefaultLane_TheStockArgvGuardHasTeeth drives the guard that keeps
// this lane honest. A guard with no demonstrated failure is decoration
// (simplicity rule 7; scar: a pin without teeth).
func TestShippedDefaultLane_TheStockArgvGuardHasTeeth(t *testing.T) {
	for _, argv := range [][]string{
		{"daemon", "-validator", "-min-bond-floor", "0"}, // the door 20 services use today
		{"daemon", "-validator", "-min-bond-floor=0"},    // the same door, = form
		{"daemon", "-validator", "-objective=false"},     // the other door
		{"daemon", "-validator", "-min-rep=0"},           // the other door, second form
		{"daemon", "-validator", "-bond=2G"},             // raising the bond instead of the default
	} {
		if err := mustBeStockArgv(argv); err == nil {
			t.Fatalf("THE GUARD IS VACUOUS: it accepted %v, which configures the lane out of the stock posture", argv)
		}
	}
	for _, argv := range [][]string{stockEdgeArgv, stockValidatorArgv} {
		if err := mustBeStockArgv(argv); err != nil {
			t.Fatalf("the guard rejected a stock argv %v: %v", argv, err)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// THE CENSUS, ENCODED.
//
// This is the measured evidence behind ROADMAP row F2's claim that "every
// existing gate configures its way OUT of the posture it should be testing",
// turned into a standing assertion. Two doors lead out of the stock validator
// posture and every tracked lane uses one of them:
//
//	DOOR A — an explicit `-min-bond-floor` (0 disarms it; any value overrides
//	         the derived one), while the objective path stays on.
//	DOOR B — `-objective=false` or `-min-rep=0`, which makes objectivePath false
//	         so the floor never arms at all (cmd/silt cmdDaemon: objectivePath =
//	         validator && objective && minRep > 0).
//
// The assertion is the INVARIANT, not the counts: every validator service must
// be classifiable into exactly one door, and none may run the floor derived. A
// new lane that runs the derived floor reddens this — which is the desired fix,
// and the record must move with it.

type validatorService struct {
	file, service string
	door          string
	args          []string
}

func TestShippedDefaultLane_NoOtherLaneRunsTheDerivedFloor_PINNED_DEFECT(t *testing.T) {
	root := repoRoot(t)
	files := trackedFiles(t, root)

	var svcs []validatorService
	var composeFiles int
	for _, f := range files {
		if !strings.HasPrefix(f, "integration/") || strings.Contains(f, "/local/") {
			continue
		}
		if !strings.HasSuffix(f, ".yml") && !strings.HasSuffix(f, ".yaml") {
			continue
		}
		composeFiles++
		svcs = append(svcs, parseComposeValidators(t, root, f)...)
	}
	if composeFiles == 0 || len(svcs) == 0 {
		t.Fatalf("CENSUS IS VACUOUS: parsed %d compose files and found %d validator services. "+
			"A census that finds nothing asserts nothing; fix the parser before believing this gate",
			composeFiles, len(svcs))
	}

	var derived, unclassified []validatorService
	byDoor := map[string]int{}
	for _, s := range svcs {
		switch s.door {
		case "A", "B":
			byDoor[s.door]++
		case "DERIVED":
			derived = append(derived, s)
		default:
			unclassified = append(unclassified, s)
		}
	}

	for _, s := range unclassified {
		t.Errorf("UNCLASSIFIABLE validator service %s::%s — args %v. Every validator lane must be "+
			"classifiable as door A (explicit -min-bond-floor), door B (-objective=false or -min-rep=0), "+
			"or DERIVED (neither: it runs the shipped floor). Classify it and update this census",
			s.file, s.service, s.args)
	}

	if len(derived) > 0 {
		var names []string
		for _, s := range derived {
			names = append(names, s.file+"::"+s.service)
		}
		t.Fatalf("PIN MOVED — and this is good news: %d lane validator service(s) now run the DERIVED "+
			"anti-release floor: %v. ROADMAP row F2 records that NONE did. Update row F2 and this pin; "+
			"the shipped-default posture is no longer exclusive to this lane", len(derived), names)
	}

	// The two graded/cloud lanes build their argv by template, so a token scan
	// cannot resolve them. Assert the property that survives templating: every
	// -validator invocation there carries an explicit -min-bond-floor.
	for _, f := range files {
		if f != "integration/cloudtest/topology.py" && f != "integration/awstest/topology.py" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(b)
		valLines, floored := 0, 0
		for _, ln := range strings.Split(src, "\n") {
			if !regexp.MustCompile(`-validator(\s|"|$)`).MatchString(ln) || strings.HasPrefix(strings.TrimSpace(ln), "#") {
				continue
			}
			valLines++
			// the flag may sit on the same or an adjacent f-string fragment
			if strings.Contains(ln, "-min-bond-floor") {
				floored++
			}
		}
		if valLines == 0 {
			t.Errorf("CENSUS IS VACUOUS for %s: found no -validator invocation. The parser, not the lane, "+
				"is what changed — fix it before believing this arm", f)
			continue
		}
		t.Logf("%s: %d -validator invocation lines, %d of them carry -min-bond-floor on the same fragment "+
			"(the remainder continue onto the next f-string fragment; see the resolved values in the run record)",
			f, valLines, floored)
		if !strings.Contains(src, "-min-bond-floor") {
			t.Errorf("PIN MOVED: %s no longer passes -min-bond-floor at all. If it now runs the DERIVED "+
				"floor, that is the fix row F2 wants — update row F2 and this pin", f)
		}
	}

	t.Logf("CENSUS: %d compose files, %d validator services, 0 on the derived floor. "+
		"door A (explicit -min-bond-floor) = %d; door B (-objective=false / -min-rep=0) = %d. "+
		"This lane is the only tracked one running the shipped-default validator posture.",
		composeFiles, len(svcs), byDoor["A"], byDoor["B"])
}

// TestShippedDefaultLane_TheCensusClassifierHasTeeth drives the census's
// classifier on fixtures, including the DERIVED case that does not exist in the
// tree today. Without this, the census's "0 on the derived floor" result is
// indistinguishable from a classifier that can never emit DERIVED at all — a
// gate that passes because it cannot fail (simplicity rule 7).
func TestShippedDefaultLane_TheCensusClassifierHasTeeth(t *testing.T) {
	fixture := func(body string) string {
		dir := t.TempDir()
		p := filepath.Join(dir, "docker-compose.yml")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	cases := []struct {
		name, body, wantDoor string
	}{
		{"doorA_explicit_floor", "services:\n  val:\n    command:\n      - silt\n      - daemon\n      - -validator\n      - -min-bond-floor=0\n", "A"},
		{"doorB_objective_false", "services:\n  val:\n    command:\n      - silt\n      - daemon\n      - -validator\n      - -objective=false\n", "B"},
		{"doorB_minrep_zero", "services:\n  val:\n    command:\n      - silt\n      - daemon\n      - -validator\n      - -min-rep=0\n", "B"},
		{"DERIVED_none_of_the_doors", "services:\n  val:\n    command:\n      - silt\n      - daemon\n      - -validator\n      - -bond=8M\n", "DERIVED"},
	}
	for _, c := range cases {
		got := parseComposeValidators(t, fixture(c.body), "docker-compose.yml")
		if len(got) != 1 {
			t.Fatalf("%s: parsed %d validator services, want 1 — the parser, not the classifier, is broken", c.name, len(got))
		}
		if got[0].door != c.wantDoor {
			t.Fatalf("%s: classified door %q, want %q (args %v)", c.name, got[0].door, c.wantDoor, got[0].args)
		}
	}
	// A service with no -validator must not be counted at all.
	if got := parseComposeValidators(t, fixture("services:\n  edge:\n    command:\n      - silt\n      - daemon\n"), "docker-compose.yml"); len(got) != 0 {
		t.Fatalf("a non-validator service was counted as one: %v", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS

// shippedFlagDefault reads a flag's default from the BINARY'S OWN flag table,
// which the flag package prints from the declaration. Never from a doc, never
// from a copied literal. Two flags carry the words "(default off)" inside their
// prose, so the parse is scoped to the named flag's block only.
func shippedFlagDefault(t *testing.T, flagName string) string {
	t.Helper()
	cmd := exec.Command(siltBin, "daemon", "-h")
	cmd.Dir = t.TempDir()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run() // -h exits non-zero on some flag configurations; the output is what matters
	header := "  -" + flagName + " "
	sc := bufio.NewScanner(&buf)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<22)
	in, block := false, []string{}
	for sc.Scan() {
		ln := sc.Text()
		if strings.HasPrefix(ln, "  -") {
			if in {
				break
			}
			in = strings.HasPrefix(ln, header)
			continue
		}
		if in {
			block = append(block, ln)
		}
	}
	joined := strings.Join(block, " ")
	m := regexp.MustCompile(`\(default ([^)]*)\)`).FindAllStringSubmatch(joined, -1)
	if len(m) != 1 {
		t.Fatalf("flag -%s: want exactly one '(default ...)' in its own block, found %d. "+
			"The flag table is the only source this lane trusts for a default; fix the parse rather than "+
			"hard-coding the value (ROADMAP row F4)", flagName, len(m))
	}
	return strings.Trim(strings.TrimSpace(m[0][1]), `"`)
}

// parseSiltSize mirrors cmd/silt parseSize, which is unexported. It is
// deliberately strict: an unparseable default must fail the test, never default
// to zero and silently satisfy a "below the floor" assertion.
func parseSiltSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "T"):
		mult, s = 1<<40, strings.TrimSuffix(s, "T")
	case strings.HasSuffix(s, "G"):
		mult, s = 1<<30, strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "M"):
		mult, s = 1<<20, strings.TrimSuffix(s, "M")
	case strings.HasSuffix(s, "K"):
		mult, s = 1<<10, strings.TrimSuffix(s, "K")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("bad size %q", s)
	}
	return n * mult, nil
}

var floorLine = regexp.MustCompile(`anti-release floor defaulted to (\d+) MiB`)

func announcedFloorMiB(t *testing.T, stdout string) int {
	t.Helper()
	m := floorLine.FindStringSubmatch(stdout)
	if m == nil {
		t.Fatalf("the daemon never announced the defaulted anti-release floor. Either it did not arm "+
			"(so the refusal came from elsewhere and this test is measuring the wrong thing) or the "+
			"announcement changed.\nstdout:\n%s", stdout)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("floor MiB %q: %v", m[1], err)
	}
	return n
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// trackedFiles walks `git ls-files`, never the filesystem. A filesystem walk
// once made local `main` RED on a gitignored month-old artifact while CI was
// 14/14 green (scar: a source gate walks gitignored artifacts).
func trackedFiles(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	return strings.Fields(string(out))
}

var (
	svcHeader = regexp.MustCompile(`^  ([A-Za-z0-9_.-]+):\s*$`)
	argItem   = regexp.MustCompile(`^\s+-\s+(\S.*?)\s*$`)
	blockKey  = regexp.MustCompile(`^    ([a-z_]+):\s*$`)
)

// parseComposeValidators extracts every service whose `command:` carries
// -validator, and classifies it by which door it takes out of the stock posture.
func parseComposeValidators(t *testing.T, root, rel string) []validatorService {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	var out []validatorService
	svc, inCmd := "", false
	var args []string
	flush := func() {
		if svc == "" || len(args) == 0 {
			return
		}
		has := func(name string) bool {
			for _, a := range args {
				if a == name || strings.HasPrefix(a, name+"=") || strings.HasPrefix(a, name+" ") {
					return true
				}
			}
			return false
		}
		val := func(name string) string {
			for _, a := range args {
				if strings.HasPrefix(a, name+"=") {
					return strings.TrimPrefix(a, name+"=")
				}
				if strings.HasPrefix(a, name+" ") {
					return strings.TrimPrefix(a, name+" ")
				}
			}
			return ""
		}
		if has("-validator") {
			s := validatorService{file: rel, service: svc, args: args}
			switch {
			case has("-min-bond-floor"):
				s.door = "A"
			case val("-objective") == "false", val("-min-rep") == "0":
				s.door = "B"
			default:
				// Neither door: the floor arms derived IF the objective path is
				// on. -objective and -min-rep both default to the objective
				// path, so a service that sets neither runs the derived floor.
				s.door = "DERIVED"
			}
			out = append(out, s)
		}
		args = nil
	}
	for _, ln := range strings.Split(string(b), "\n") {
		if m := svcHeader.FindStringSubmatch(ln); m != nil {
			flush()
			svc, inCmd = m[1], false
			continue
		}
		if blockKey.MatchString(ln) {
			inCmd = strings.TrimSpace(ln) == "command:"
			continue
		}
		if inCmd {
			if m := argItem.FindStringSubmatch(ln); m != nil {
				item := m[1]
				if strings.HasPrefix(item, "-") {
					args = append(args, item)
				} else if len(args) > 0 {
					args[len(args)-1] += " " + item
				}
			}
		}
	}
	flush()
	return out
}
