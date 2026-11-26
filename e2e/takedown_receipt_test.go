package e2e

import (
	"bytes"
	"encoding/base64"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/ports"
)

// Two takedowns commit on a chain two validators replicate. The operator who
// subscribes to on-chain takedowns drops the held bytes of each at once and records
// each removal with a receipt; the operator who does not keeps serving. An auditor
// holding only the OTHER operator's replica verifies both receipts, the second
// extending the first, and refuses a forged one. Takedown bites, is not global,
// and every honoured removal is provable.
func TestHonouredTakedownCarriesAReceiptAnAuditorVerifies(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	idA := identity.FromSeed(4881).NodeID().String()
	idB := identity.FromSeed(4882).NodeID().String()
	// The curators: each revoker signs its takedown with its own key. B follows two of them; the
	// third (4885) lists a root too, and B must not honour it.
	idCur1 := identity.FromSeed(4883).NodeID().String()
	idCur2 := identity.FromSeed(4884).NodeID().String()
	idUnfollowed := identity.FromSeed(4885).NodeID().String()
	storeA, storeB := t.TempDir(), t.TempDir()
	a := startDaemon(t, "takedown-A",
		"-listen", "127.0.0.1:0", "-store", storeA,
		"-serve-registry", "127.0.0.1:0", "-validator",
		"-objective=false", "-min-rep", "100", "-quorum", "1", "-attesters", idB,
		"-bond", "8M", "-bond-audit", "1s",
		"-capacity", "1G", "-mdns=false", "-id-seed", "4881")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	bootstrapA := peer[1] + "@" + peer[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]
	b := startDaemon(t, "takedown-B",
		"-listen", "127.0.0.1:0", "-store", storeB,
		"-bootstrap", bootstrapA, "-validator",
		"-objective=false", "-min-rep", "100", "-quorum", "1",
		"-bond", "8M", "-bond-audit", "1s", "-honor-curators", idCur1+","+idCur2,
		"-capacity", "1G", "-mdns=false", "-id-seed", "4882")
	b.waitFor(t, reBootstrap, 20*time.Second)

	publish := func(seed int64) (string, []byte) {
		src := filepath.Join(t.TempDir(), "payload.bin")
		want := make([]byte, 192<<10)
		rand.New(rand.NewSource(seed)).Read(want)
		if err := os.WriteFile(src, want, 0o644); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(40 * time.Second)
		for {
			// Convergent, so each retry re-proposes the same root while the quorum forms.
			out, err := runClientAllowErr(t, "swarm", "add", src, "-peers", bootstrapA, "-registry", regRef,
				"-chunk-size", "65536", "-mode", "convergent")
			if err == nil {
				if l := regexp.MustCompile(`silt:v1:\S+`).FindString(out); l != "" {
					return l, want
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("publish never committed:\n%s", out)
			}
			time.Sleep(time.Second)
		}
	}
	link1, want1 := publish(0x7a1)
	link2, _ := publish(0x7a2)
	h1, err := link.Parse(link1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := link.Parse(link2)
	if err != nil {
		t.Fatal(err)
	}

	revoke := func(seed, root string) {
		d := startDaemon(t, "takedown-revoker-"+seed,
			"-listen", "127.0.0.1:0", "-store", t.TempDir(),
			"-bootstrap", bootstrapA, "-validator",
			"-objective=false", "-min-rep", "100", "-quorum", "1", "-attesters", idA,
			"-bond", "8M", "-bond-audit", "1s", "-revoke", root,
			"-capacity", "1G", "-mdns=false", "-id-seed", seed)
		d.waitFor(t, regexp.MustCompile(`proposed on-chain revocation of `+root), 60*time.Second)
	}
	reHonoured := func(root ports.Hash) *regexp.Regexp {
		return regexp.MustCompile(`takedown: honoured root=` + root.String() + ` curator=(\S+) purged=(\d+) .*receipt=([A-Za-z0-9_-]+)`)
	}

	revoke("4883", h1.Root.String())
	m1 := waitForInLog(t, filepath.Join(storeB, "debug.log"), reHonoured(h1.Root), 60*time.Second)
	if m1[2] == "0" {
		t.Fatal("the honouring operator held none of the revoked root, so its removal dropped nothing and the drill tests nothing")
	}
	if m1[1] != idCur1 {
		t.Fatalf("the honoured takedown names curator %s, want %s", m1[1], idCur1)
	}
	// A curator B does not follow lists h2 FIRST; then a followed curator lists it. B honours in
	// log order, so had it honoured the unfollowed listing its line would come first, naming 4885.
	revoke("4885", h2.Root.String())
	revoke("4884", h2.Root.String())
	m2 := waitForInLog(t, filepath.Join(storeB, "debug.log"), reHonoured(h2.Root), 60*time.Second)
	if m2[1] != idCur2 {
		t.Fatalf("B honoured h2 on curator %s's listing; it follows only %s and %s (4885 is %s)", m2[1], idCur1, idCur2, idUnfollowed)
	}
	m1, m2 = m1[1:], m2[1:] // drop the curator: [purged, receipt] as below

	// The auditor holds A's replica, not B's. Wait until it has the anchoring blocks.
	verify := func(receipt string) (string, error) {
		deadline := time.Now().Add(30 * time.Second)
		for {
			out, err := runClientAllowErr(t, "takedown-verify", "-store", storeA, "-receipt", receipt)
			if err == nil || !strings.Contains(out, "sync it and retry") || time.Now().After(deadline) {
				return out, err
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	for i, m := range [][]string{m1, m2} {
		out, err := verify(m[2])
		if err != nil || !strings.Contains(out, "VERIFIED:") {
			t.Fatalf("receipt %d, honoured by B, did not verify against A's replica: %v\n%s", i+1, err, out)
		}
		t.Logf("receipt %d: %s", i+1, regexp.MustCompile(`VERIFIED: .*`).FindString(out))
	}
	r1 := decodeReceipt(t, m1[2])
	r2 := decodeReceipt(t, m2[2])
	if r2.PrevSize != r1.Size || r2.PrevLogRoot != r1.LogRoot {
		t.Fatalf("the second receipt must extend the first: prev (%d, %s), first (%d, %s)", r2.PrevSize, r2.PrevLogRoot, r1.Size, r1.LogRoot)
	}

	// The auditor can also derive the proof itself, and gets the operator's receipt.
	out := runClient(t, "takedown-receipt", "-store", storeA, "-root", h1.Root.String(), "-curator", idCur1)
	if got := regexp.MustCompile(`receipt: (\S+)`).FindStringSubmatch(out); got == nil || got[1] != m1[2] {
		t.Fatalf("the auditor's own derivation differs from the operator's receipt:\n%s", out)
	}

	// A forged receipt: the first takedown's proof, claimed for the second root.
	forged := r1
	forged.Root = h2.Root
	if out, err := verify(base64.RawURLEncoding.EncodeToString(chain.EncodeReceipt(forged))); err == nil {
		t.Fatalf("a forged receipt verified:\n%s", out)
	}

	// Not global: A never subscribed and still serves the first root bit-perfect.
	dst := filepath.Join(t.TempDir(), "after-takedown.bin")
	runClientStderr(t, "swarm", "get", link1, "-o", dst, "-peers", bootstrapA, "-registry", regRef)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want1) {
		t.Fatalf("the operator who did not subscribe stopped serving: got %d bytes, want %d", len(got), len(want1))
	}
}

func decodeReceipt(t *testing.T, s string) chain.RevocationReceipt {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	r, err := chain.DecodeReceipt(raw)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
