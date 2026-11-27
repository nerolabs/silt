package e2e

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestAReadOnlyJoinerFetchesButCannotPublish is the test-network posture: anyone may join to
// read and serve, only the operator's addresses may write. The node first runs with the
// local address as a writer, and a file is published. It then restarts on the same store with
// a writer list that excludes the local address: the same client can still fetch the file,
// and can no longer publish one.
func TestAReadOnlyJoinerFetchesButCannotPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	store := t.TempDir()
	args := func(writers string) []string {
		return []string{"-listen", "127.0.0.1:0", "-store", store, "-serve-registry", "127.0.0.1:0",
			"-validator", "-quorum", "0", "-min-rep", "0", "-capacity", "1G", "-mdns=false",
			"-id-seed", "1501", "-writers", writers}
	}
	data := make([]byte, 600<<10)
	rand.New(rand.NewSource(15)).Read(data)
	src := filepath.Join(t.TempDir(), "a.bin")
	os.WriteFile(src, data, 0o644)

	// The operator publishes.
	a := startDaemon(t, "writer", args("127.0.0.1")...)
	peer := a.waitFor(t, rePeer, 30*time.Second)
	reg := a.waitFor(t, reRegistry, 30*time.Second)[1]
	var lnk string
	deadline := time.Now().Add(90 * time.Second)
	for lnk == "" {
		out, err := runClientAllowErr(t, "swarm", "add", src, "-peers", peer[1]+"@"+peer[2], "-registry", reg, "-mode", "convergent")
		lnk = regexp.MustCompile(`silt:v1:[A-Za-z0-9_:-]+`).FindString(out)
		if lnk == "" && time.Now().After(deadline) {
			t.Fatalf("CONTROL: a listed writer could not publish: %v\n%s", err, out)
		}
		if lnk == "" {
			time.Sleep(2 * time.Second)
		}
	}
	a.cmd.Process.Kill()
	a.cmd.Wait()

	// The same node, the local address no longer a writer.
	b := startDaemon(t, "read-only", append(args("192.0.2.1"), "-log", "info")...)
	b.waitFor(t, regexp.MustCompile(`writers: only 192\.0\.2\.1 may write`), 30*time.Second)
	peer = b.waitFor(t, rePeer, 30*time.Second)
	reg = b.waitFor(t, reRegistry, 30*time.Second)[1]
	peers := peer[1] + "@" + peer[2]

	dst := filepath.Join(t.TempDir(), "got.bin")
	var got []byte
	deadline = time.Now().Add(90 * time.Second)
	for {
		out, err := runClientAllowErr(t, "swarm", "get", lnk, "-o", dst, "-peers", peers, "-registry", reg)
		if err == nil {
			got, _ = os.ReadFile(dst)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a non-writer could not FETCH: %v\n%s\n--- daemon ---\n%s", err, out, b.out.dump())
		}
		time.Sleep(2 * time.Second)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("the non-writer's fetch returned the wrong bytes")
	}

	other := filepath.Join(t.TempDir(), "b.bin")
	os.WriteFile(other, []byte(strings.Repeat("not yours ", 5000)), 0o644)
	out, err := runClientAllowErr(t, "swarm", "add", other, "-peers", peers, "-registry", reg, "-mode", "convergent")
	if err == nil || regexp.MustCompile(`silt:v1:`).MatchString(out) {
		t.Fatalf("a non-writer PUBLISHED:\n%s", out)
	}
	logged, _ := os.ReadFile(filepath.Join(store, "debug.log"))
	if !strings.Contains(string(logged), "write refused: not from a listed writer address") &&
		!strings.Contains(out, "accepts publishes only from") {
		t.Fatalf("the publish failed, but not at the writer gate:\n%s\n--- daemon ---\n%s", out, b.out.dump())
	}
}
