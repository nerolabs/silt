package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/chunk"
	"github.com/nerolabs/silt/core/crypto"
)

// seizeAll reads every file under every store, as an adversary holding the disks
// would: the stores are read whole, with no assumption about which files matter.
func seizeAll(t *testing.T, stores []string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, s := range stores {
		err := filepath.WalkDir(s, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			out[p] = b
			return nil
		})
		if err != nil {
			t.Fatalf("seize %s: %v", s, err)
		}
	}
	return out
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// knownPlaintext is a file whose every line carries the canary, so a byte search
// for any part of it has thousands of places to land.
func knownPlaintext(tag string, size int) []byte {
	var b bytes.Buffer
	for i := 0; b.Len() < size; i++ {
		fmt.Fprintf(&b, "SILT-SEIZURE-CANARY-%s line %07d: the quick brown fox jumps over the lazy dog\n", tag, i)
	}
	return b.Bytes()[:size]
}

// Seize every disk in the network, the validator's included, and fail to recover a
// known plaintext published in the default private mode; then, in the same run and
// off the same disks, a key-holder fetches it bit-perfect. Without the control arm a
// seizure that recovers nothing would only show the seizure was done badly.
//
// The seizer is given everything short of the key: every byte of every store, the
// plaintext itself, the published file's name, and its care link. Three attacks,
// each with the outcome the product documents:
//   - search the disks for any 32-byte window of the plaintext, or its name: nothing;
//   - re-encrypt the known plaintext and look for the ciphertext (the confirmation
//     attack): fails on the private publish, and SUCCEEDS on a convergent one, which
//     is the disclosed cost of that mode (-mode help text);
//   - fetch with the care link: refused.
func TestSeizedNetworkYieldsNoPlaintextWhileTheKeyHolderReadsIt(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	tagBytes := make([]byte, 8)
	_, _ = rand.Read(tagBytes)
	tag := hex.EncodeToString(tagBytes)
	const chunkSize = 65536

	ports := []string{reservePort(t), reservePort(t), reservePort(t), reservePort(t)}
	regPort := reservePort(t)
	stores := []string{t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()}
	seeds := []string{"4871", "4872", "4873", "4874"}
	a := startDaemon(t, "seize-0", "-listen", ports[0], "-advertise", ports[0], "-store", stores[0],
		"-mdns=false", "-id-seed", seeds[0],
		"-serve-registry", regPort, "-validator", "-quorum", "0", "-min-rep", "0", "-capacity", "1")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	bootstrap := peer[1] + "@" + ports[0]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]
	holder := func(i int, name string) *daemon {
		d := startDaemon(t, name, "-listen", ports[i], "-advertise", ports[i], "-store", stores[i],
			"-mdns=false", "-id-seed", seeds[i], "-bootstrap", bootstrap, "-capacity", "1G")
		d.waitFor(t, reBootstrap, 20*time.Second)
		return d
	}
	all := []*daemon{a}
	for i := 1; i < 4; i++ {
		all = append(all, holder(i, fmt.Sprintf("seize-%d", i)))
	}

	dir := t.TempDir()
	name := "seizure-canary-" + tag + ".txt"
	priv := knownPlaintext(tag+"-P", 300<<10)
	conv := knownPlaintext(tag+"-C", 300<<10)
	privPath, convPath := filepath.Join(dir, name), filepath.Join(dir, "convergent-"+name)
	if err := os.WriteFile(privPath, priv, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(convPath, conv, 0o644); err != nil {
		t.Fatal(err)
	}
	publish := func(path string, extra ...string) (link, care string) {
		args := append([]string{"swarm", "add", path, "-peers", bootstrap, "-registry", regRef,
			"-chunk-size", fmt.Sprint(chunkSize)}, extra...)
		out, err := runClientAllowErr(t, args...)
		if err != nil {
			t.Fatalf("publish %s: %v\n%s", filepath.Base(path), err, out)
		}
		l := regexp.MustCompile(`silt:v1:\S+`).FindString(out)
		c := reCareLink.FindString(out)
		if l == "" || c == "" {
			t.Fatalf("publish printed no link or care link:\n%s", out)
		}
		return l, c
	}
	privLink, privCare := publish(privPath)
	_, _ = publish(convPath, "-mode", "convergent")

	// SEIZE: every process stops, every disk is read.
	for _, d := range all {
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	}
	seized := seizeAll(t, stores)
	var total int
	for _, b := range seized {
		total += len(b)
	}
	t.Logf("seized %d files, %d bytes, from four stores", len(seized), total)
	if total < len(priv) {
		t.Fatalf("the seized disks hold %d bytes, fewer than the %d-byte file: the publish did not land, so the seizure tests nothing", total, len(priv))
	}

	// Attack 1: any 32-byte window of either plaintext, the canary, or the name.
	needles := [][]byte{[]byte("SILT-SEIZURE-CANARY-" + tag), []byte(name)}
	for off := 0; off+32 <= len(priv); off += 997 {
		needles = append(needles, priv[off:off+32], conv[off:off+32])
	}
	for p, b := range seized {
		for _, n := range needles {
			if bytes.Contains(b, n) {
				t.Fatalf("SEIZED PLAINTEXT: %s contains %q", p, n)
			}
		}
	}

	// Attack 2, the confirmation attack: encrypt the known plaintext's first chunk
	// the way a convergent publish does and look for the ciphertext on the disks.
	confirm := func(plain []byte) bool {
		frames, err := chunk.Split(bytes.NewReader(plain), chunkSize)
		if err != nil {
			t.Fatal(err)
		}
		ct, _, err := crypto.ConvergentEncrypt(frames[0])
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range seized {
			if bytes.Contains(b, ct[:64]) {
				return true
			}
		}
		return false
	}
	if confirm(priv) {
		t.Fatal("the confirmation attack found the PRIVATE publish's ciphertext: private mode is probeable")
	}
	if !confirm(conv) {
		t.Fatal("the confirmation attack did not find the CONVERGENT publish, which its help text says it is open to: the attack is not reaching the data, so its failure on the private publish proves nothing")
	}

	// Restart the same disks for the remaining arms.
	a = restartDaemon(t, "seize-0-again", "-listen", ports[0], "-advertise", ports[0], "-store", stores[0],
		"-mdns=false", "-id-seed", seeds[0],
		"-serve-registry", regPort, "-validator", "-quorum", "0", "-min-rep", "0", "-capacity", "1")
	a.waitFor(t, reRegistry, 20*time.Second)
	for i := 1; i < 4; i++ {
		holder(i, fmt.Sprintf("seize-%d-again", i))
	}

	// Attack 3: the care link is a repair capability, not a read one.
	careOut := filepath.Join(t.TempDir(), "care.bin")
	careRes, careErr := runClientAllowErr(t, "swarm", "get", privCare, "-o", careOut, "-peers", bootstrap, "-registry", regRef)
	if careErr == nil {
		t.Fatalf("a fetch with the care link SUCCEEDED:\n%s", careRes)
	}
	t.Logf("the care-link fetch was refused: %s", strings.TrimSpace(lastLine(careRes)))
	if got, err := os.ReadFile(careOut); err == nil && bytes.Contains(got, []byte("SILT-SEIZURE-CANARY-")) {
		t.Fatal("the care link wrote plaintext to disk")
	}

	// Control: a wrong key on the same disks fails, and the key-holder reads the file.
	wrong := privLink[:len(privLink)-1] + map[bool]string{true: "A", false: "B"}[!strings.HasSuffix(privLink, "A")]
	wrongOut := filepath.Join(t.TempDir(), "wrong.bin")
	if _, err := runClientAllowErr(t, "swarm", "get", wrong, "-o", wrongOut, "-peers", bootstrap, "-registry", regRef); err == nil {
		if got, _ := os.ReadFile(wrongOut); bytes.Equal(got, priv) {
			t.Fatal("a link with the wrong key read the file")
		}
	}
	dst := filepath.Join(t.TempDir(), "keyholder.bin")
	runClientStderr(t, "swarm", "get", privLink, "-o", dst, "-peers", bootstrap, "-registry", regRef)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, priv) {
		t.Fatalf("the key-holder could not read the seized network's copy: got %d bytes, want %d", len(got), len(priv))
	}
}
