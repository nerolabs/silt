package e2e

// The concurrent-publish 502 (2026-08-19, found by silt's first production
// workload): every successful UI publish ran its -care-published auto-caretake
// ON the daemon's event loop, where Node.Care's synchronous registry Lookup
// marshalled back onto the same loop through chainhost — a reentrant
// post-and-wait self-deadlock that wedged the node's single thread for the 30s
// chainhost timeout per publish. Under concurrent ingest the wedge starved every
// queued message: placements blew their 4×2s attempts ("placed on no node"),
// entries outlived the 30s commit poll, and the caller got hard 502s while the
// daemon survived. Fixed by node.lookupEntry (a validator answers loop-context
// registry reads from its own committed chain); this test drives the real field
// shape — concurrent multipart publishes against a validator daemon's UI API —
// and requires every publish to succeed. The daemon runs -inbound-cap 0, which
// also regression-covers the documented "0 = unbounded" sentinel (previously
// rejected by parseSize).
// Mechanism record: docs/thinking/2026-08-19-publish-502-attribution-care-self-deadlock.md

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"regexp"
	"sync"
	"testing"
	"time"
)

var reUI = regexp.MustCompile(`ui: http://(\S+)/\?token=([0-9a-f]+)`)

func TestConcurrentUIPublishesAllSucceed(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}

	a := startDaemon(t, "A",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0",
		"-validator", "-quorum", "0", "-min-rep", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "1001",
		"-ui", "127.0.0.1:0", "-inbound-cap", "0")
	ui := a.waitFor(t, reUI, 20*time.Second)
	base, token := "http://"+ui[1], ui[2]

	// The field shape: parallel workers each pushing sequential segment
	// publishes. Pre-fix, the first success wedges the loop 30s and the rest
	// collapse into 502s; post-fix every publish must return 200.
	const workers, segs = 4, 2
	payload := make([]byte, 512<<10)
	rand.New(rand.NewSource(0x502)).Read(payload)

	var mu sync.Mutex
	var failures []string
	var wg sync.WaitGroup
	for w := 1; w <= workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for s := 1; s <= segs; s++ {
				status, body, err := uiPublish(base, token, fmt.Sprintf("w%ds%d.bin", w, s), payload)
				if err != nil || status != http.StatusOK {
					mu.Lock()
					failures = append(failures, fmt.Sprintf("worker %d seg %d: status=%d err=%v body=%s", w, s, status, err, body))
					mu.Unlock()
					return
				}
			}
		}(w)
	}
	wg.Wait()
	if len(failures) > 0 {
		t.Fatalf("concurrent UI publishes failed (the Care loop-deadlock face):\n%s\n--- daemon ---\n%s",
			failures, a.out.dump())
	}
}

// uiPublish drives POST /api/publish exactly as a UI/ingest client does:
// multipart file upload with the bearer token.
func uiPublish(base, token, name string, payload []byte) (int, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		return 0, "", err
	}
	if _, err := fw.Write(payload); err != nil {
		return 0, "", err
	}
	if err := mw.Close(); err != nil {
		return 0, "", err
	}
	req, err := http.NewRequest("POST", base+"/api/publish", &buf)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	return resp.StatusCode, string(body), nil
}
