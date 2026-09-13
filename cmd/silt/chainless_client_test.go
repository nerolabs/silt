package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/adapters/walltime"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// TestParseDeclaredChainID pins the -chain-id parse, INCLUDING both refusals. A network identity
// that silently failed to take produces exactly one symptom — a publish that cannot gather
// signatures — so the flag has to fail loudly at parse time or not at all.
func TestParseDeclaredChainID(t *testing.T) {
	want := ports.HashBytes([]byte("a genesis block"))
	cases := []struct {
		name    string
		in      string
		wantID  ports.Hash
		wantErr string // substring; empty = must succeed
	}{
		{"empty is not declared", "", ports.Hash{}, ""},
		{"blank is not declared", "   ", ports.Hash{}, ""},
		{"the genesis hash round-trips", want.String(), want, ""},
		{"surrounding space is trimmed", "  " + want.String() + "\n", want, ""},
		{"too short", "abcd", ports.Hash{}, "-chain-id"},
		{"not hex", strings.Repeat("z", 64), ports.Hash{}, "-chain-id"},
		{"the all-zero hash is refused", strings.Repeat("0", 64), ports.Hash{}, "not a network identity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDeclaredChainID(tc.in)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("parseDeclaredChainID(%q) = error %v, want nil", tc.in, err)
				}
				if got != tc.wantID {
					t.Fatalf("parseDeclaredChainID(%q) = %s, want %s", tc.in, got, tc.wantID)
				}
				return
			}
			if err == nil {
				t.Fatalf("parseDeclaredChainID(%q) accepted it; want an error naming %q", tc.in, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("parseDeclaredChainID(%q) = %v, want an error naming %q", tc.in, err, tc.wantErr)
			}
		})
	}
}

// TestSwarmAddRefusesAMalformedChainIDBeforeItJoins drives the flag through swarmAdd's own argv,
// not through the helper: it pins that -chain-id is REGISTERED on the flag set and that its refusal
// lands before the client touches the network or the file. The file path below does not exist, so
// if the refusal moved after os.Open the error would name the file instead.
func TestSwarmAddRefusesAMalformedChainIDBeforeItJoins(t *testing.T) {
	err := swarmAdd([]string{
		"-peers", "1111111111111111111111111111111111111111111111111111111111111111@127.0.0.1:1",
		"-registry", "http://127.0.0.1:1",
		"-chain-id", "not-a-hash",
		filepath.Join(t.TempDir(), "does-not-exist.bin"),
	})
	if err == nil {
		t.Fatal("swarm add accepted -chain-id not-a-hash")
	}
	if !strings.Contains(err.Error(), "-chain-id") {
		t.Fatalf("swarm add error = %v, want it to name -chain-id (it refused for some other reason)", err)
	}
}

// TestSwarmAddChainIDReachesTheClientNode is the composition the flag exists for: the parsed value
// lands on the node the publish lane reads, and it lands on the REQUESTER side only.
func TestSwarmAddChainIDReachesTheClientNode(t *testing.T) {
	nd, _ := testClientNode(t, 9601)
	declared, err := parseDeclaredChainID(ports.HashBytes([]byte("genesis")).String())
	if err != nil {
		t.Fatal(err)
	}
	if serr := nd.SetNetworkIdentity(declared); serr != nil {
		t.Fatal(serr)
	}
	if got := nd.RequesterChainID(); got != declared {
		t.Fatalf("RequesterChainID = %s, want %s", got, declared)
	}
}

// TestPublishTokenFailureNamesTheCreditCause is D-TD-3. acquirePublishToken's stage 2 used to bind
// AcquireCredits' error to `_`, so every credit-lane refusal surfaced as a bare "could not gather
// enough publish-token signatures": acquireToken SKIPS an issuer with no credit rather than
// reporting one, so the real cause never reached the operator. It must now be named.
//
// The fixture is the cheapest total shortfall there is: a validator this client can never reach, so
// no issuer key is cached and AcquireCredits refuses immediately with ErrTokenAcquire.
//
// ABLATION (drives this RED): put the callback's second parameter back to `_` in mintCredits.
func TestPublishTokenFailureNamesTheCreditCause(t *testing.T) {
	nd, loop := testClientNode(t, 9602)
	unreachable := identity.FromSeed(9603).NodeID()

	var gotErr error
	done := make(chan struct{})
	// Node state is the loop's, so the whole acquisition runs ON the loop — the same way
	// swarmAdd's run(...) drives it.
	loop.Post("test", func() {
		acquirePublishToken(nd, []ports.NodeID{unreachable}, 1, func(tok *ports.PublishToken, err error) {
			gotErr = err
			close(done)
		})
	})
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("acquirePublishToken never called back")
	}

	if gotErr == nil {
		t.Fatal("acquirePublishToken succeeded against an unreachable validator")
	}
	// The underlying error is still the one callers match on — the cause is ADDED, not swapped.
	if !errors.Is(gotErr, node.ErrTokenAcquire) {
		t.Fatalf("error = %v, want it to still wrap node.ErrTokenAcquire", gotErr)
	}
	if !strings.Contains(gotErr.Error(), "prepaid publish credit") {
		t.Fatalf("error = %q, want it to name the prepaid-credit cause (the discarded-error shape is back)", gotErr)
	}
	if !strings.Contains(gotErr.Error(), "1 of 1 validators") {
		t.Fatalf("error = %q, want it to count the validators that minted no credit", gotErr)
	}
}

// testClientNode builds the chainless shape joinSwarm builds, without the swarm.
func testClientNode(t *testing.T, seed int64) (*node.Node, *eventloop.Loop) {
	t.Helper()
	loop := eventloop.New()
	go loop.Run()
	t.Cleanup(loop.Stop)
	id := identity.FromSeed(seed)
	tr, err := tcpnet.New(loop, id, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	cfg := node.DefaultConfig()
	cfg.RequestTimeout = ports.Duration(500 * time.Millisecond)
	nd := node.New(id.NodeID(), cfg, walltime.New(loop), tr, memstore.New())
	nd.SetSigner(id.Signer())
	nd.SetEphemeral(true)
	return nd, loop
}

// TestSwarmAddWiresTheDeclaredChainIDIntoTheClientNode is a SOURCE gate, and it is here because
// `main` has no runtime consumer to observe: the credit lane does not read RequesterChainID until
// M3 (#828) lands, so deleting swarmAdd's SetNetworkIdentity call changes no behaviour any test on
// this branch can see. The gate walks swarmAdd's OWN body — a call anywhere else in swarm.go does
// not satisfy it — and carries two anti-vacuity anchors so a rename cannot make it pass by finding
// nothing.
//
// It retires the day the credit lane reads the value: at that point a behavioural e2e (a token
// publish against a token-requiring network, with and without -chain-id) dominates it.
func TestSwarmAddWiresTheDeclaredChainIDIntoTheClientNode(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "swarm.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body *ast.BlockStmt
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "swarmAdd" {
			body = fd.Body
		}
	}
	// Anchor 1: the function this gate claims to read exists. Without it a rename of swarmAdd
	// turns the whole gate into a no-op that reports GREEN.
	if body == nil {
		t.Fatal("SOURCE GATE is VACUOUS: swarm.go declares no func swarmAdd — this gate read nothing")
	}
	calls := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			calls[sel.Sel.Name] = true
		}
		return true
	})
	// Anchor 2: the walk reaches real calls. `joinSwarm` builds the client node swarmAdd then
	// declares the identity on, so if this is absent the walk is looking at the wrong tree.
	if !calls["SetNetworkIdentity"] {
		t.Fatal("SOURCE GATE: swarmAdd's body has no SetNetworkIdentity call — the -chain-id flag " +
			"parses and is then dropped, so the client publishes under the ZERO network identity " +
			"and the operator is told nothing. D-TOKEN-DOMAIN-CHAINLESS-CLIENT-2026-09-12, route (a).")
	}
	if !calls["FetchCanonicalIssuersFromAny"] {
		t.Fatal("SOURCE GATE is reading the wrong body: swarmAdd's own token block calls " +
			"FetchCanonicalIssuersFromAny and this walk did not see it")
	}
}
