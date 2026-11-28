package e2e

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"testing"
	"time"
)

// TestAPIFetchServesByteRanges: a live daemon answers an HTTP Range on /api/fetch with
// exactly those bytes (206, Content-Range), refuses a range past the end with 416, and still
// serves the whole file to a plain GET — the contract a video gateway reads a large file by.
func TestAPIFetchServesByteRanges(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	a := startDaemon(t, "range",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(), "-serve-registry", "127.0.0.1:0",
		"-validator", "-quorum", "0", "-min-rep", "0", "-capacity", "1G", "-mdns=false",
		"-id-seed", "1201", "-ui", "127.0.0.1:0")
	ui := a.waitFor(t, reUI, 20*time.Second)
	base, token := "http://"+ui[1], ui[2]

	data := make([]byte, 3<<20+12345) // ~12 chunks at 256 KiB: two stripes, a short last frame
	rand.New(rand.NewSource(77)).Read(data)
	var lnk string
	deadline := time.Now().Add(90 * time.Second)
	for lnk == "" {
		status, body, err := uiPublish(base, token, "r.bin", data)
		if err == nil && status == http.StatusOK {
			lnk = regexp.MustCompile(`silt:v1:[A-Za-z0-9_:-]+`).FindString(body)
		}
		if time.Now().After(deadline) {
			t.Fatalf("publish never succeeded: status=%d err=%v body=%s", status, err, body)
		}
		if lnk == "" {
			time.Sleep(2 * time.Second)
		}
	}
	get := func(rng string) (int, http.Header, []byte) {
		req, _ := http.NewRequest("GET", base+"/api/fetch?link="+url.QueryEscape(lnk), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header, b
	}
	size := int64(len(data))
	for _, c := range []struct{ off, last int64 }{{0, 99}, {262_000, 263_000}, {2_621_000, 2_622_000}, {size - 777, size - 1}} {
		status, hdr, body := get(fmt.Sprintf("bytes=%d-%d", c.off, c.last))
		if status != http.StatusPartialContent {
			t.Fatalf("range %d-%d: status %d: %s", c.off, c.last, status, body)
		}
		if want := fmt.Sprintf("bytes %d-%d/%d", c.off, c.last, size); hdr.Get("Content-Range") != want {
			t.Fatalf("Content-Range %q, want %q", hdr.Get("Content-Range"), want)
		}
		if !bytes.Equal(body, data[c.off:c.last+1]) {
			t.Fatalf("range %d-%d: %d bytes that differ from the file", c.off, c.last, len(body))
		}
	}
	if status, _, body := get("bytes=-500"); status != http.StatusPartialContent || !bytes.Equal(body, data[size-500:]) {
		t.Fatalf("suffix range: status %d, %d bytes", status, len(body))
	}
	if status, hdr, _ := get(fmt.Sprintf("bytes=%d-", size)); status != http.StatusRequestedRangeNotSatisfiable || hdr.Get("Content-Range") != fmt.Sprintf("bytes */%d", size) {
		t.Fatalf("a range past the end: status %d, Content-Range %q", status, hdr.Get("Content-Range"))
	}
	{ // the query form a browser page uses: ?offset=&length=
		req, _ := http.NewRequest("GET", base+"/api/fetch?link="+url.QueryEscape(lnk)+"&offset=262000&length=1001", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusPartialContent || !bytes.Equal(b, data[262000:263001]) {
			t.Fatalf("?offset=&length=: status %d, %d bytes", resp.StatusCode, len(b))
		}
	}
	status, hdr, body := get("")
	if status != http.StatusOK || !bytes.Equal(body, data) || hdr.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("a plain GET: status %d, %d bytes, Accept-Ranges %q", status, len(body), hdr.Get("Accept-Ranges"))
	}
}
