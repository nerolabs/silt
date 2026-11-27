package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/release"
)

// floorStatus is the subset of GET /api/status the version-floor gate reads.
type floorStatus struct {
	Stats *struct {
		FloorRefused int
	} `json:"stats"`
	Chain *struct {
		Height       int `json:"height"`
		VersionFloor *struct {
			Seq             uint64 `json:"seq"`
			Floor           uint32 `json:"floor"`
			Tier            string `json:"tier"`
			Enforcing       bool   `json:"enforcing"`
			Release         uint32 `json:"release"`
			BelowFloor      bool   `json:"belowFloor"`
			Validators      int    `json:"validators"`
			ValidatorsBelow int    `json:"validatorsBelow"`
			BondedBelow     int64  `json:"bondedBelowPermille"`
		} `json:"versionFloor"`
	} `json:"chain"`
}

func readFloorStatus(t *testing.T, base, token string) floorStatus {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET /api/status: %v", err)
	}
	defer resp.Body.Close()
	var s floorStatus
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return s
}

var reMaintainerPub = regexp.MustCompile(`maintainer public key: ([0-9a-f]{64})`)

// TestVersionFloorOverTCP is item 19's fourth clause on real processes: four validators on one
// network whose genesis names three maintainer keys at threshold 2. Three present as upgraded to
// release N+1; the fourth runs this build's release N. Three of four equal bonds is above the
// two-thirds the objective quorum needs, so the network stays live once the old node's consensus
// messages stop counting. (With one of three old it would halt: an enforced floor removes the
// old releases' weight from the quorum, which is why R4 puts it behind a signing threshold.)
//
//   - An advisory one maintainer signed is refused by the daemon and nothing changes.
//   - The threshold's enforce-tier advisory commits on all three. The old node warns its operator.
//   - Once the grace period passes, the upgraded validators refuse the old node's consensus
//     messages, and the old node still serves the content it holds.
//   - A recall lifts the floor.
func TestVersionFloorOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	keyDir := t.TempDir()
	var pubs []string
	for i := 0; i < 3; i++ {
		out := runClient(t, "advisory", "keygen", "-out", filepath.Join(keyDir, fmt.Sprintf("m%d.key", i)))
		m := reMaintainerPub.FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("keygen printed no public key:\n%s", out)
		}
		pubs = append(pubs, m[1])
	}
	upgraded := fmt.Sprint(release.Number + 1)

	seeds := []int64{2401, 2402, 2403, 2404}
	var ids []string
	for _, s := range seeds {
		ids = append(ids, identity.FromSeed(s).NodeID().String())
	}
	anchors := strings.Join(ids, ",")
	common := func(name string, seed int64, extra ...string) []string {
		return append([]string{
			"-listen", "127.0.0.1:0", "-store", t.TempDir(), "-validator",
			"-min-bond", "1M", "-mature-validators", "4", "-anchors", anchors, "-quorum", "1",
			"-min-bond-floor", "0", "-bond", "8M", "-bond-audit", "1s",
			"-capacity", "1G", "-mdns=false", "-id-seed", fmt.Sprint(seed),
			"-maintainers", strings.Join(pubs, ","), "-maintainer-threshold", "2",
			"-ui", "127.0.0.1:0",
		}, extra...)
	}

	a := startDaemon(t, "A", common("A", seeds[0], "-serve-registry", "127.0.0.1:0", "-advertise-release", upgraded)...)
	chainID := a.waitFor(t, reGenesisBlock, 30*time.Second)[1]
	addrA := a.waitFor(t, rePeer, 20*time.Second)[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]
	uiA := a.waitFor(t, reUI, 20*time.Second)
	bootstrapA := ids[0] + "@" + addrA
	b := startDaemon(t, "B", common("B", seeds[1], "-bootstrap", bootstrapA, "-advertise-release", upgraded)...)
	d := startDaemon(t, "D", common("D", seeds[3], "-bootstrap", bootstrapA, "-advertise-release", upgraded)...)
	c := startDaemon(t, "C", common("C", seeds[2], "-bootstrap", bootstrapA)...)
	uiB, uiC, uiD := b.waitFor(t, reUI, 20*time.Second), c.waitFor(t, reUI, 20*time.Second), d.waitFor(t, reUI, 20*time.Second)
	addrC := c.waitFor(t, rePeer, 20*time.Second)[2]
	type ui struct{ base, token string }
	// uis[2] is the old node, C.
	uis := []ui{{"http://" + uiA[1], uiA[2]}, {"http://" + uiB[1], uiB[2]}, {"http://" + uiC[1], uiC[2]}, {"http://" + uiD[1], uiD[2]}}
	heights := func() string {
		var hs []string
		for i, u := range uis {
			st := readFloorStatus(t, u.base, u.token)
			h, refused, vf := -1, -1, "none"
			if st.Chain != nil {
				h = st.Chain.Height
				if st.Chain.VersionFloor != nil {
					vf = fmt.Sprintf("%+v", *st.Chain.VersionFloor)
				}
			}
			if st.Stats != nil {
				refused = st.Stats.FloorRefused
			}
			hs = append(hs, fmt.Sprintf("node %d: height %d, refused %d, floor %s", i, h, refused, vf))
		}
		return strings.Join(hs, "\n")
	}

	// Content published while every node is current, so the old node holds some of it.
	src := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(src, []byte(strings.Repeat("the old node still serves this. ", 4096)), 0o644); err != nil {
		t.Fatal(err)
	}
	reLink := regexp.MustCompile(`silt:v1:\S+`)
	publish := func(path string) string {
		deadline := time.Now().Add(60 * time.Second)
		for {
			out, err := runClientAllowErr(t, "swarm", "add", path, "-peers", bootstrapA, "-registry", regRef, "-chunk-size", "65536")
			if err == nil {
				if l := reLink.FindString(out); l != "" {
					return l
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("the validators never committed a publish:\n%s\n%s", out, heights())
			}
			time.Sleep(time.Second)
		}
	}
	link := publish(src)
	work := 0
	more := func() {
		work++
		p := filepath.Join(t.TempDir(), "work.bin")
		if err := os.WriteFile(p, []byte(fmt.Sprintf("work %d %d", work, time.Now().UnixNano())), 0o644); err != nil {
			t.Fatal(err)
		}
		publish(p)
	}

	sign := func(file string, key int, flags ...string) string {
		args := append([]string{"advisory", "sign", "-key", filepath.Join(keyDir, fmt.Sprintf("m%d.key", key)), "-chain-id", chainID, "-file", file}, flags...)
		return runClient(t, args...)
	}
	submit := func(file string) (string, error) {
		return runClientAllowErr(t, "advisory", "submit", "-file", file, "-api", uis[0].base, "-token", uis[0].token)
	}

	// One maintainer's signature: refused, and nothing is in force.
	lone := filepath.Join(keyDir, "lone.adv")
	sign(lone, 0, "-seq", "1", "-floor", upgraded, "-tier", "enforce", "-grace", "3", "-note", "one key")
	if out, err := submit(lone); err == nil || !strings.Contains(out+err.Error(), "threshold") {
		t.Fatalf("an advisory one maintainer signed must be refused by the daemon, got err=%v out=%s", err, out)
	}
	more()
	for i, u := range uis {
		if s := readFloorStatus(t, u.base, u.token); s.Chain == nil || s.Chain.VersionFloor != nil {
			t.Fatalf("node %d: an advisory below the threshold changed something: %+v", i, s.Chain)
		}
	}

	// The threshold's advisory: two maintainers sign the same file.
	adv := filepath.Join(keyDir, "floor.adv")
	sign(adv, 0, "-seq", "1", "-floor", upgraded, "-tier", "enforce", "-grace", "3", "-note", "release N+1 fixes the relay crash")
	sign(adv, 2)
	if out, err := submit(adv); err != nil {
		t.Fatalf("the threshold's advisory was refused: %v %s", err, out)
	}
	waitAll := func(what string, ok func(i int, s floorStatus) bool) {
		t.Helper()
		// Generous for a loaded CI runner: locally the whole test takes about six minutes,
		// and each wait spans several commits.
		deadline := time.Now().Add(240 * time.Second)
		for {
			done := true
			for i, u := range uis {
				if !ok(i, readFloorStatus(t, u.base, u.token)) {
					done = false
				}
			}
			if done {
				return
			}
			if time.Now().After(deadline) {
				var tails []string
				for _, dm := range []*daemon{a, b, c, d} {
					lines := strings.Split(strings.TrimSpace(dm.out.dump()), "\n")
					if len(lines) > 12 {
						lines = lines[len(lines)-12:]
					}
					tails = append(tails, "--- "+dm.name+" ---\n"+strings.Join(lines, "\n"))
				}
				t.Fatalf("timed out waiting for %s:\n%s\n%s", what, heights(), strings.Join(tails, "\n"))
			}
			more()
		}
	}
	waitAll("the advisory to commit on every replica", func(i int, s floorStatus) bool {
		return s.Chain != nil && s.Chain.VersionFloor != nil && s.Chain.VersionFloor.Seq == 1
	})
	for i, u := range uis {
		vf := readFloorStatus(t, u.base, u.token).Chain.VersionFloor
		if vf.BelowFloor != (i == 2) {
			t.Fatalf("node %d: belowFloor %v, release %d, floor %d — only the old node is below", i, vf.BelowFloor, vf.Release, vf.Floor)
		}
	}
	c.waitFor(t, regexp.MustCompile(`version-floor: this node runs a release below the network's floor`), 30*time.Second)
	// The adoption readout an upgraded validator gives the maintainers before enforcement: one
	// validator of four, a quarter of the bond, is below the floor.
	if vf := readFloorStatus(t, uis[0].base, uis[0].token).Chain.VersionFloor; vf.Validators != 4 || vf.ValidatorsBelow != 1 || vf.BondedBelow != 250 {
		t.Fatalf("adoption readout: %d validators, %d below, %d‰ of bond below; want 4, 1, 250‰", vf.Validators, vf.ValidatorsBelow, vf.BondedBelow)
	}

	// Past the grace period: the upgraded validators refuse the old node's consensus messages.
	waitAll("enforcement and a refusal", func(i int, s floorStatus) bool {
		if s.Chain == nil || s.Chain.VersionFloor == nil || !s.Chain.VersionFloor.Enforcing {
			return false
		}
		if i == 2 {
			return true
		}
		refused := 0
		for j, u := range uis {
			if j != 2 {
				refused += readFloorStatus(t, u.base, u.token).Stats.FloorRefused
			}
		}
		return refused > 0
	})
	c.waitFor(t, regexp.MustCompile(`version-floor: ENFORCED — validators no longer count this node's consensus messages`), 30*time.Second)
	if n := readFloorStatus(t, uis[2].base, uis[2].token).Stats.FloorRefused; n != 0 {
		t.Fatalf("the old node refused %d consensus message(s); it enforces nothing against current releases", n)
	}

	// The old node still serves: fetch through it alone.
	dst := filepath.Join(t.TempDir(), "fetched.bin")
	runClient(t, "swarm", "get", link, "-o", dst, "-peers", ids[2]+"@"+addrC, "-registry", regRef)
	got, _ := os.ReadFile(dst)
	want, _ := os.ReadFile(src)
	if string(got) != string(want) {
		t.Fatalf("fetching through the old node returned %d bytes, want %d", len(got), len(want))
	}

	// A recall lifts the floor.
	recall := filepath.Join(keyDir, "recall.adv")
	sign(recall, 1, "-seq", "2", "-tier", "recall")
	sign(recall, 2)
	if out, err := submit(recall); err != nil {
		t.Fatalf("the recall was refused: %v %s", err, out)
	}
	waitAll("the recall on every replica", func(i int, s floorStatus) bool {
		return s.Chain != nil && s.Chain.VersionFloor != nil && s.Chain.VersionFloor.Seq == 2 && !s.Chain.VersionFloor.Enforcing
	})
	t.Logf("held:\n%s", heights())
}
