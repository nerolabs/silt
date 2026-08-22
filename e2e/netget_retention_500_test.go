package e2e

// #500 on the wire: the UI consumer==provider promise, end to end. A daemon
// that fetches a link through its own /api/fetch must come out the other side
// as a DISCOVERABLE provider of what it consumed — visible to `swarm holders`
// — not a silent hoarder whose bytes count against the pledge while no fetcher
// can find them. The sim tier (core/node/netget_retention_500_test.go) proves
// the drop/retain semantics and serve-after-holder-death; this proves the wire
// path: real daemons, real TCP, the real UI handler.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestUIFetchMakesTheConsumerAProvider(t *testing.T) {
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
	status, body, err := uiPublish(baseA, tokenA, "consume.bin", payload)
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

	// B: the consumer daemon — its /api/fetch is the path under test. It joins
	// AFTER the publish (the realistic consumer shape), so it cannot have been
	// a placement target: any holder listing of B below is retention, not luck.
	b := startDaemon(t, "B",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-bootstrap", bootstrapA, "-registry", regRef,
		"-capacity", "1G", "-mdns=false", "-id-seed", "9701",
		"-ui", "127.0.0.1:0")
	peerB := b.waitFor(t, rePeer, 20*time.Second)
	idB := peerB[1]
	b.waitFor(t, reBootstrap, 20*time.Second)
	uiB := b.waitFor(t, reUI, 20*time.Second)
	baseB := "http://" + uiB[1]

	// Sanity: B is not a holder before it consumes.
	holdersOut := runClient(t, "swarm", "holders", pub.Link, "-peers", bootstrapA, "-registry", regRef)
	if strings.Contains(holdersOut, idB) {
		t.Fatalf("rig: consumer B already listed as a holder before fetching:\n%s", holdersOut)
	}

	// B consumes the link through its own UI (a read: token-free on localhost).
	resp, err := (&http.Client{Timeout: 3 * time.Minute}).Get(
		baseB + "/api/fetch?link=" + url.QueryEscape(pub.Link))
	if err != nil {
		t.Fatalf("api/fetch: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("api/fetch: status=%d body=%s", resp.StatusCode, got[:min(len(got), 300)])
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("api/fetch returned wrong bytes: %d vs %d", len(got), len(payload))
	}

	// The promise (#500): B now appears in the swarm's provider view. The
	// announce completes before /api/fetch returns, but give the walk a few
	// polls for record propagation across the pool.
	deadline := time.Now().Add(60 * time.Second)
	for {
		holdersOut = runClient(t, "swarm", "holders", pub.Link, "-peers", bootstrapA, "-registry", regRef)
		if strings.Contains(holdersOut, idB) {
			t.Logf("consumer B is a discoverable provider (consumer==provider wired, #500)")
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("#500: consumer B never became a discoverable provider after /api/fetch:\n%s", holdersOut)
		}
		time.Sleep(2 * time.Second)
	}
}
