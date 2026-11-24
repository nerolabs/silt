package e2e

// On the wire: the UI fetch, end to end. A daemon that fetches a link through its
// own /api/fetch with -serve-fetched must come out the other side as a
// DISCOVERABLE provider of what it consumed — visible to `swarm holders` — not
// a silent hoarder whose bytes count against the pledge while no fetcher can
// find them. On the default it must come out as neither: no provider record,
// because announcing is publishing a (fetcher, content) pair, and no bytes kept. The sim tier (core/node/netget_retention_test.go) proves the
// drop/retain semantics and serve-after-holder-death; this proves the wire
// path: real daemons, real TCP, the real UI handler.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUIFetchAnnouncesOnlyWhenTheConsumerOptsIn(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}

	// A: lone trusted validator + registry + UI (the publisher).
	a := startDaemon(t, "A",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0",
		"-validator", "-quorum", "0", "-min-rep", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "9501",
		"-ui", "127.0.0.1:0")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	idA, addrA := peer[1], peer[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]
	bootstrapA := idA + "@" + addrA
	uiA := a.waitFor(t, reUI, 20*time.Second)
	baseA, tokenA := "http://"+uiA[1], uiA[2]

	// Storage pool so the publish scatters off the validator.
	for i := 0; i < 6; i++ {
		d := startDaemon(t, fmt.Sprintf("S%d", i+1),
			"-listen", "127.0.0.1:0", "-store", t.TempDir(),
			"-bootstrap", bootstrapA,
			"-capacity", "1G", "-mdns=false", "-id-seed", fmt.Sprint(9601+i))
		d.waitFor(t, rePeer, 20*time.Second)
		d.waitFor(t, reBootstrap, 20*time.Second)
	}

	// Publish through A's UI.
	payload := make([]byte, 256<<10)
	for i := range payload {
		payload[i] = byte(i*13 + 1)
	}
	pubStart := time.Now()
	status, body, err := uiPublish(baseA, tokenA, "consume.bin", payload)
	t.Logf("publish took %s", time.Since(pubStart).Round(time.Millisecond))
	if err != nil || status != http.StatusOK {
		t.Fatalf("publish: status=%d err=%v body=%s", status, err, body)
	}
	var pub struct {
		Link string `json:"link"`
	}
	_ = json.Unmarshal([]byte(body), &pub)
	if pub.Link == "" {
		t.Fatalf("publish returned no link: %s", body)
	}

	// Two consumer daemons, both joining AFTER the publish (the realistic consumer
	// shape), so neither can have been a placement target: any holder listing of
	// either below is retention, not luck. B opts in with -serve-fetched; C runs the
	// default, under which a fetch must keep nothing and announce nothing.
	consumer := func(name, seed string, extra ...string) (id, base, store string) {
		store = t.TempDir()
		args := append([]string{"-listen", "127.0.0.1:0", "-store", store,
			"-bootstrap", bootstrapA, "-registry", regRef,
			"-capacity", "1G", "-mdns=false", "-id-seed", seed,
			"-ui", "127.0.0.1:0"}, extra...)
		d := startDaemon(t, name, args...)
		id = d.waitFor(t, rePeer, 20*time.Second)[1]
		d.waitFor(t, reBootstrap, 20*time.Second)
		return id, "http://" + d.waitFor(t, reUI, 20*time.Second)[1], store
	}
	idB, baseB, _ := consumer("B", "9701", "-serve-fetched=true")
	idC, baseC, storeC := consumer("C", "9702")
	holders := func() string {
		return runClient(t, "swarm", "holders", pub.Link, "-peers", bootstrapA, "-registry", regRef)
	}
	if h := holders(); strings.Contains(h, idB) || strings.Contains(h, idC) {
		t.Fatalf("rig: a consumer is already listed as a holder before fetching:\n%s", h)
	}
	// A consumer reads the link through its own UI (a read: token-free on localhost).
	fetch := func(name, base string) {
		began := time.Now()
		defer func() { t.Logf("%s /api/fetch took %s", name, time.Since(began).Round(time.Millisecond)) }()
		resp, err := (&http.Client{Timeout: 3 * time.Minute}).Get(
			base + "/api/fetch?link=" + url.QueryEscape(pub.Link))
		if err != nil {
			t.Fatalf("%s api/fetch: %v", name, err)
		}
		got, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s api/fetch: status=%d body=%s", name, resp.StatusCode, got[:min(len(got), 300)])
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("%s api/fetch returned wrong bytes: %d vs %d", name, len(got), len(payload))
		}
	}
	fetch("C", baseC)
	fetch("B", baseB)

	// B, which opted in, must appear in the swarm's provider view: that is the
	// control that the walk below CAN see a consumer that announced. The announce
	// completes before /api/fetch returns, but give the walk a few polls for
	// record propagation across the pool.
	deadline := time.Now().Add(60 * time.Second)
	var h string
	for {
		h = holders()
		if strings.Contains(h, idB) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("consumer B (-serve-fetched) never became a discoverable provider after /api/fetch:\n%s", h)
		}
		time.Sleep(2 * time.Second)
	}
	// C, on the default, fetched FIRST, so it has had longer to announce than B had.
	if strings.Contains(h, idC) {
		t.Fatalf("consumer C fetched on the default and is listed as a provider — the fetch published a (fetcher, content) pair:\n%s", h)
	}
	// And it kept nothing: no chunk file is left in its store.
	if n := chunkFilesUnder(t, storeC); n != 0 {
		t.Fatalf("consumer C fetched on the default and kept %d chunk files", n)
	}
}

// chunkFilesUnder counts the chunk files a daemon's disk store holds
// (<store>/objects/xx/<id>).
func chunkFilesUnder(t *testing.T, store string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(filepath.Join(store, "objects"), func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("walking %s: %v", store, err)
	}
	return n
}
