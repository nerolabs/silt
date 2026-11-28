package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/credit"
)

// THE CORE SQUEEZE, MEASURED. The validators (the core) carry consensus, the registry,
// token issuance and bond audits for every storage node (the edge), and are paid only
// for the bytes they serve. With demand held constant, this measures each validator's
// net margin at two edge populations and reports it as a SIGNED number:
//
//	margin = credits earned in the window − bytes moved in the window / Dλ
//
// Dλ is the serve-mint denomination (credit.ServeMintBytesPerCredit): what the network
// pays for an unwitnessed byte served, so it prices the core's bandwidth at the rate the
// edge is paid for its own. Bytes are the transport meter's encoded frames, in and out,
// every kind. The breakdown by role says where the bytes go.
//
// It asserts that the MEASUREMENT is valid (the demand ran identically at both
// populations, every fetch was bit-perfect, every validator moved traffic, both windows
// are the same length) and never the margin's sign: the number is the finding.
// SILT_MARGIN_OUT names a file the two rows are appended to as JSON lines.

var marginRoles = map[string]string{}

func init() {
	for role, kinds := range map[string][]string{
		"consensus": {"ProposeBlock", "AttestReply", "PrepareQC", "PrecommitReply", "RoundChange", "RoundChangeAck",
			"RoundCert", "RoundCertAck", "CommitBlock", "CommitAck", "GetChain", "ChainReply", "GetChainHead",
			"ChainHeadReply", "GetWitness", "WitnessReply", "SubmitSlash", "SubmitSlashAck", "HaveBondReg",
			"SubmitBondReg", "SubmitBondRegAck", "SubmitEntry", "SubmitEntryAck", "SubmitIssuerKeyReg", "SubmitIssuerKeyRegAck"},
		"bond-audit": {"BondChallenge", "BondReply"},
		"issuance": {"TokenRequest", "TokenReply", "GetIssuerKey", "IssuerKeyReply", "GetCanonicalIssuers",
			"CanonicalIssuersReply", "GetDemandIssuerKeys", "DemandIssuerKeysReply", "DemandTokenRequest", "DemandTokenReply"},
		"storage": {"StoreChunk", "StoreChunkAck", "FetchChunk", "FetchChunkReply", "HasChunk", "HasChunkReply",
			"Challenge", "ChallengeReply"},
		"routing": {"FindNode", "FindNodeReply", "GetProviders", "GetProvidersReply", "AddProvider", "AddProviderAck",
			"CheckReachability", "ReachabilityReply"},
	} {
		for _, k := range kinds {
			marginRoles[k] = role
		}
	}
}

type marginRow struct {
	Edge       int              `json:"edgeNodes"`
	WindowSec  int              `json:"windowSec"`
	Validators int              `json:"validators"`
	Bytes      []int64          `json:"bytesPerValidator"`
	Earned     []int64          `json:"earnedPerValidator"`
	Margin     []float64        `json:"marginPerValidator"`
	MeanMargin float64          `json:"meanMarginCredits"`
	ByRole     map[string]int64 `json:"bytesByRoleAllValidators"`
}

func TestCoreMarginAtTwoEdgePopulations(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	rows := []marginRow{measureCoreMargin(t, 3, 0), measureCoreMargin(t, 9, 1)}
	for _, r := range rows {
		t.Logf("CORE MARGIN at %d edge nodes, %ds window: mean %+.2f credits per validator (per validator %v); bytes %v; earned %v; bytes by role %v",
			r.Edge, r.WindowSec, r.MeanMargin, fmtMargins(r.Margin), r.Bytes, r.Earned, r.ByRole)
	}
	t.Logf("CORE SQUEEZE: mean margin moved %+.2f credits per validator from %d to %d edge nodes at held-constant demand",
		rows[1].MeanMargin-rows[0].MeanMargin, rows[0].Edge, rows[1].Edge)
	if out := os.Getenv("SILT_MARGIN_OUT"); out != "" {
		f, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		for _, r := range rows {
			fmt.Fprintln(f, jsonLine(r))
		}
	}
}

func fmtMargins(m []float64) string {
	var parts []string
	for _, v := range m {
		parts = append(parts, fmt.Sprintf("%+.2f", v))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// measureCoreMargin brings up four anchor validators and `edge` storage nodes, runs the
// fixed demand, and returns the validators' margin over a fixed window.
func measureCoreMargin(t *testing.T, edge, run int) marginRow {
	const (
		nVal    = 4
		objects = 4
		fetches = 3
		objSize = 768 << 10
		window  = 120 * time.Second // two shipped bond-audit periods
	)
	base := 9500 + run*100
	ids := make([]string, nVal)
	ports := make([]string, nVal)
	for i := range ids {
		ids[i] = identity.FromSeed(int64(base + i)).NodeID().String()
		ports[i] = reservePort(t)
	}
	regPort := reservePort(t)
	consensus := []string{"-objective", "-min-bond", "1M", "-min-bond-floor", "0",
		"-mature-validators", "99", "-anchors", strings.Join(ids, ","), "-quorum", "2"}
	type val struct {
		d           *daemon
		base, token string
	}
	var vals []val
	var regRef string
	for i := 0; i < nVal; i++ {
		var att, persistent []string
		for j := 0; j < nVal; j++ {
			if j != i {
				att = append(att, ids[j])
				persistent = append(persistent, ids[j]+"@"+ports[j])
			}
		}
		args := append([]string{"-id-seed", fmt.Sprint(base + i), "-listen", ports[i], "-advertise", ports[i],
			"-store", t.TempDir(), "-validator", "-bond", "8M", // the shipped -bond-audit cadence
			"-attesters", strings.Join(att, ","), "-persistent-peers", strings.Join(persistent, ","),
			"-ui", "127.0.0.1:0", "-capacity", "1G", "-mdns=false"}, consensus...)
		if i == 0 {
			args = append(args, "-serve-registry", regPort)
		} else {
			args = append(args, "-bootstrap", ids[0]+"@"+ports[0])
		}
		d := startDaemon(t, fmt.Sprintf("r%dV%d", run, i), args...)
		ui := d.waitFor(t, reUI, 20*time.Second)
		if i == 0 {
			regRef = d.waitFor(t, reRegistry, 30*time.Second)[1]
		}
		vals = append(vals, val{d: d, base: "http://" + ui[1], token: ui[2]})
	}
	boot := ids[0] + "@" + ports[0]
	for i := 0; i < edge; i++ {
		d := startDaemon(t, fmt.Sprintf("r%dS%d", run, i),
			"-listen", "127.0.0.1:0", "-store", t.TempDir(), "-bootstrap", boot,
			"-capacity", "1G", "-mdns=false", "-id-seed", fmt.Sprint(base+50+i))
		d.waitFor(t, reBootstrap, 20*time.Second)
	}
	var issuers []string
	for i := range ids {
		issuers = append(issuers, ids[i]+"@"+ports[i])
	}

	// Warm-up: the anchors commit their bonds before any publish can gather a token.
	warm := filepath.Join(t.TempDir(), "warm.bin")
	os.WriteFile(warm, []byte("warm"), 0o644)
	for deadline := time.Now().Add(90 * time.Second); ; {
		if _, err := runClientAllowErr(t, "swarm", "add", warm, "-peers", strings.Join(issuers, ","),
			"-registry", regRef, "-token-quorum", "2"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %d: the anchors never became able to issue a publish token", run)
		}
		time.Sleep(2 * time.Second)
	}

	snapshot := func() ([]int64, []int64, map[string]int64) {
		var bytesV, bal []int64
		roles := map[string]int64{}
		for _, v := range vals {
			s := getStatus(t, v.base, v.token)
			var b int64
			for kind, tr := range s.Traffic {
				b += tr.InBytes + tr.OutBytes
				role := marginRoles[kind]
				if role == "" {
					role = "other"
				}
				roles[role] += tr.InBytes + tr.OutBytes
			}
			if len(s.Traffic) == 0 {
				t.Fatalf("run %d: validator %s reports no traffic meter", run, v.d.name)
			}
			bytesV = append(bytesV, b)
			var balance int64
			if s.Durability != nil {
				balance = s.Durability.Balance
			}
			bal = append(bal, balance)
		}
		return bytesV, bal, roles
	}

	b0, bal0, roles0 := snapshot()
	t0 := time.Now()
	// THE DEMAND, identical at every population: the same objects (same seed), each
	// published once and fetched `fetches` times through the validators.
	rng := rand.New(rand.NewSource(0xC0DE))
	for o := 0; o < objects; o++ {
		want := make([]byte, objSize)
		rng.Read(want)
		src := filepath.Join(t.TempDir(), fmt.Sprintf("o%d.bin", o))
		os.WriteFile(src, want, 0o644)
		// A publish right after start can race the validators' issuer-key exchange: the
		// registry's validator has not yet fetched the other signer's key and refuses the
		// token "1 of 2". That one refusal is retried (a node fetches an unknown signer's key
		// on demand, #934); any other failure fails the test. A refused attempt adds a little
		// traffic to the measured window — small beside the demand being measured.
		var out string
		obj := o
		for deadline := time.Now().Add(60 * time.Second); ; {
			o, err := runClientAllowErr(t, "swarm", "add", src, "-peers", strings.Join(issuers, ","),
				"-registry", regRef, "-token-quorum", "2", "-chunk-size", "262144")
			out = o
			if err == nil {
				break
			}
			if !strings.Contains(o, "too few distinct qualified validator signatures") || time.Now().After(deadline) {
				t.Fatalf("run %d: publish %d failed: %v\n%s", run, obj, err, o)
			}
			time.Sleep(2 * time.Second)
		}
		link := regexp.MustCompile(`silt:v1:\S+`).FindString(out)
		if link == "" {
			t.Fatalf("run %d: publish %d printed no link:\n%s", run, o, out)
		}
		for f := 0; f < fetches; f++ {
			dst := filepath.Join(t.TempDir(), "got.bin")
			runClient(t, "swarm", "get", link, "-o", dst, "-peers", boot, "-registry", regRef)
			got, err := os.ReadFile(dst)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("run %d: fetch %d of object %d was not bit-perfect", run, f, o)
			}
		}
	}
	if el := time.Since(t0); el > window {
		t.Fatalf("run %d: the demand took %s, longer than the %s window; the windows would differ", run, el.Round(time.Second), window)
	}
	time.Sleep(window - time.Since(t0))
	b1, bal1, roles1 := snapshot()

	row := marginRow{Edge: edge, WindowSec: int(window / time.Second), Validators: nVal, ByRole: map[string]int64{}}
	var sum float64
	for i := range vals {
		moved, earned := b1[i]-b0[i], bal1[i]-bal0[i]
		if moved <= 0 {
			t.Fatalf("run %d: validator %d moved no bytes in the window", run, i)
		}
		m := float64(earned) - float64(moved)/float64(credit.ServeMintBytesPerCredit)
		row.Bytes, row.Earned, row.Margin = append(row.Bytes, moved), append(row.Earned, earned), append(row.Margin, m)
		sum += m
	}
	row.MeanMargin = sum / float64(nVal)
	keys := make([]string, 0, len(roles1))
	for k := range roles1 {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		row.ByRole[k] = roles1[k] - roles0[k]
	}
	return row
}

func jsonLine(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}
