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
		name     string
		in       string
		wantID   ports.Hash
		wantDecl bool
		wantErr  string // substring; empty = must succeed
	}{
		{"empty is not declared", "", ports.Hash{}, false, ""},
		{"blank is not declared", "   ", ports.Hash{}, false, ""},
		{"the genesis hash round-trips", want.String(), want, true, ""},
		{"surrounding space is trimmed", "  " + want.String() + "\n", want, true, ""},
		{"too short", "abcd", ports.Hash{}, false, "-chain-id"},
		{"not hex", strings.Repeat("z", 64), ports.Hash{}, false, "-chain-id"},
		{"the all-zero hash is refused", strings.Repeat("0", 64), ports.Hash{}, false, "not a network identity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, declared, err := parseDeclaredChainID(tc.in)
			if declared != tc.wantDecl {
				t.Fatalf("parseDeclaredChainID(%q) declared = %v, want %v", tc.in, declared, tc.wantDecl)
			}
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
// lands on the node the publish lane reads, and it lands on the REQUESTER side only. It drives the
// SHIPPED path end to end — parse, then declareNetworkIdentity, the same two calls swarmAdd makes.
func TestSwarmAddChainIDReachesTheClientNode(t *testing.T) {
	nd, loop := testClientNode(t, 9601)
	want, declared, err := parseDeclaredChainID(ports.HashBytes([]byte("genesis")).String())
	if err != nil {
		t.Fatal(err)
	}
	if serr := declareNetworkIdentity(loopRun(loop), nd, want, declared); serr != nil {
		t.Fatal(serr)
	}
	if got := nd.RequesterChainID(); got != want {
		t.Fatalf("RequesterChainID = %s, want %s", got, want)
	}
	if !nd.HasNetworkIdentity() {
		t.Fatal("HasNetworkIdentity is false after a successful declaration")
	}
}

// TestDeclareNetworkIdentityIsBehaviouralAndOnTheLoop is the arm the source gate below CANNOT be:
// it proves the install RUNS, with the right guard, on the node's own goroutine.
//
// It exists because a blind review drove ablation A9 — leave swarmAdd's call in place and make its
// guard impossible — and the entire cmd/silt suite stayed GREEN. swarmAdd builds its client node
// internally and hands it to nobody, so a guard inside swarmAdd is reachable by no test. Moving
// the guard into declareNetworkIdentity is what makes A9's class of defect observable, and these
// are the four arms that observe it.
//
// ABLATIONS, one per arm:
//   - guard impossible (`if !declared || declared`) → arm 1 RED (nothing installs)
//   - guard dropped (always install) → arm 2 RED (ErrNetworkIdentityZero surfaces)
//   - guard reads the HASH (`if id != zero`) instead of `declared` → arm 3 RED
//   - install written off-loop (call nd.SetNetworkIdentity directly, not through run) → arm 4 RED
func TestDeclareNetworkIdentityIsBehaviouralAndOnTheLoop(t *testing.T) {
	want := ports.HashBytes([]byte("a declared network"))

	// Arm 1: a declaration installs.
	t.Run("a declaration installs", func(t *testing.T) {
		nd, loop := testClientNode(t, 9611)
		if err := declareNetworkIdentity(loopRun(loop), nd, want, true); err != nil {
			t.Fatal(err)
		}
		if got := nd.RequesterChainID(); got != want {
			t.Fatalf("RequesterChainID = %s, want %s", got, want)
		}
	})

	// Arm 2: no declaration installs NOTHING, and reports no error. This is the shipped default,
	// and it is what a dropped guard breaks: SetNetworkIdentity refuses the zero hash, so an
	// unguarded install turns every plain `swarm add` into a refusal.
	t.Run("no declaration is silent", func(t *testing.T) {
		nd, loop := testClientNode(t, 9612)
		if err := declareNetworkIdentity(loopRun(loop), nd, ports.Hash{}, false); err != nil {
			t.Fatalf("an undeclared swarm add was refused: %v", err)
		}
		if nd.HasNetworkIdentity() {
			t.Fatal("an undeclared client acquired a network identity")
		}
	})

	// Arm 3: the guard reads DECLARED, never the hash. A hash-valued guard cannot tell
	// not-declared from a refused declaration — the ambiguity HasNetworkIdentity exists to close.
	t.Run("the guard reads declared, not the hash", func(t *testing.T) {
		nd, loop := testClientNode(t, 9613)
		if err := declareNetworkIdentity(loopRun(loop), nd, want, false); err != nil {
			t.Fatal(err)
		}
		if nd.HasNetworkIdentity() {
			t.Fatalf("declared=false still installed %s — the guard is reading the hash", want)
		}
	})

	// Arm 4: the write is POSTED, not made from the calling goroutine. joinSwarm returns after
	// Bootstrap, so the loop is already live when swarmAdd declares; an off-loop write races the
	// first handler that reads the field (#828).
	t.Run("the write is posted onto the loop", func(t *testing.T) {
		nd, loop := testClientNode(t, 9614)
		posts := 0
		counting := func(fn func(done func())) error {
			posts++
			return loopRun(loop)(fn)
		}
		if err := declareNetworkIdentity(counting, nd, want, true); err != nil {
			t.Fatal(err)
		}
		if posts != 1 {
			t.Fatalf("declareNetworkIdentity posted %d times, want exactly 1 — an off-loop write "+
				"races the handler that reads declaredChainID", posts)
		}
		if got := nd.RequesterChainID(); got != want {
			t.Fatalf("the posted write did not land: RequesterChainID = %s, want %s", got, want)
		}
	})
}

// loopRun builds joinSwarm's `run` shape over a test loop: post fn, wait for its done().
func loopRun(loop *eventloop.Loop) func(fn func(done func())) error {
	return func(fn func(done func())) error {
		ch := make(chan struct{})
		loop.Post("test", func() { fn(func() { close(ch) }) })
		select {
		case <-ch:
			return nil
		case <-time.After(30 * time.Second):
			return errors.New("test loop run timed out")
		}
	}
}

// TestPublishTokenFailureNamesTheCreditCause is D-TD-3. acquirePublishToken's stage 2 used to bind
// AcquireCredits' error to `_`, so every credit-lane refusal surfaced as a bare "could not gather
// enough publish-token signatures": acquireToken SKIPS an issuer with no credit rather than
// reporting one, so the real cause never reached the operator. It must now be named.
//
// The fixture is the cheapest total shortfall there is: a validator this client can never reach, so
// no issuer key is cached and AcquireCredits refuses immediately with ErrCreditIssuerKeyUnknown.
//
// THE ANTI-TAUTOLOGY ARM IS THE POINT OF THE LAST TWO CHECKS. As first shipped, the clause read
// "first cause: node: could not gather enough publish-token signatures" — the outer sentence
// repeated — because one sentinel, ErrTokenAcquire, was returned from two unrelated places and both
// legs were the same object. A clause that repeats its own outer error names a COUNT, not a CAUSE,
// and the earlier assertions here could not see it: both matched text the WRAPPER produces.
//
// ABLATIONS: (a) put the callback's second parameter back to `_` in mintCredits → the
// "prepaid publish credit" arm; (b) return ErrTokenAcquire from AcquireCredits' no-issuer-key leg
// again → the cause arm and the anti-tautology arm.
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
	// The count is not a cause. The clause must name the credit lane's OWN refusal.
	if !errors.Is(gotErr, node.ErrCreditIssuerKeyUnknown) {
		t.Fatalf("error = %v, want the first cause to be node.ErrCreditIssuerKeyUnknown", gotErr)
	}
	if !strings.Contains(gotErr.Error(), node.ErrCreditIssuerKeyUnknown.Error()) {
		t.Fatalf("error = %q, want it to SPELL the no-issuer-key cause, not just wrap it", gotErr)
	}
	// ANTI-TAUTOLOGY: the clause must not be the outer sentence repeated back.
	if strings.Contains(gotErr.Error(), "first cause: "+node.ErrTokenAcquire.Error()) {
		t.Fatalf("error = %q: the first-cause clause REPEATS the outer error verbatim, so it names "+
			"a count and no cause — one sentinel is doing two jobs again", gotErr)
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

// TestSwarmAddCallsDeclareNetworkIdentity is a SOURCE gate, and this docstring states its reach
// EXACTLY, because an earlier version of it did not.
//
// WHAT IT PROVES: swarmAdd's own body contains a call named declareNetworkIdentity. That is all.
//
// WHAT IT DOES NOT PROVE, MEASURED: that the call RUNS. A blind review drove ablation A9 — leave
// the call written in swarmAdd and make its guard impossible — and the whole cmd/silt suite stayed
// GREEN. An `ast.Inspect` collecting SelectorExpr names sees a call regardless of reachability, so
// no gate of this shape can close that. The earlier failure message here claimed to catch exactly
// that defect ("the -chain-id flag parses and is then dropped"); it did not, and it no longer says
// it does.
//
// WHAT WAS DONE ABOUT IT INSTEAD OF RE-WORDING ALONE: the guard A9 corrupted was moved OUT of
// swarmAdd and into declareNetworkIdentity, which TestDeclareNetworkIdentityIsBehaviouralAndOnThe
// Loop drives for real. A9's class of defect is now RED at the unit tier. What is left un-gated is
// narrower and louder: swarmAdd's unconditional call being made unreachable (wrapped in a dead
// branch, or the function returning before it). That residual is why this gate still exists, and
// it is filed as R-CHAINID-INSTALL-SOURCE-GATED.
//
// WHY A SOURCE GATE AT ALL: swarmAdd builds its client node internally and exposes it to no
// caller, and no production reader of RequesterChainID exists anywhere in the tree until #828
// lands, so there is no observable at any tier — including e2e — for "swarmAdd reached its
// install". The gate retires the day the credit lane reads the value: a token publish against a
// token-requiring network, with and without -chain-id, dominates it.
//
// It walks swarmAdd's OWN body — a call anywhere else in swarm.go does not satisfy it — and
// carries two anti-vacuity anchors so a rename cannot make it pass by finding nothing.
func TestSwarmAddCallsDeclareNetworkIdentity(t *testing.T) {
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
	// Collect BOTH call shapes: `x.Method(...)` and the bare `f(...)` that declareNetworkIdentity
	// is. Collecting only SelectorExpr would make this gate silently blind to its own target.
	calls := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.SelectorExpr:
			calls[v.Sel.Name] = true
		case *ast.CallExpr:
			if id, ok := v.Fun.(*ast.Ident); ok {
				calls[id.Name] = true
			}
		}
		return true
	})
	// Anchor 2: the walk reaches real calls. `joinSwarm` builds the client node swarmAdd then
	// declares the identity on, so if this is absent the walk is looking at the wrong tree.
	if !calls["declareNetworkIdentity"] {
		t.Fatal("SOURCE GATE: swarmAdd's body contains no declareNetworkIdentity call, so nothing " +
			"installs the parsed -chain-id on the client node. READ THE LIMIT BEFORE ACTING ON A " +
			"GREEN HERE: this gate proves only that the call is WRITTEN in swarmAdd, never that it " +
			"RUNS — a call inside a dead branch passes it (measured, blind-review ablation A9). " +
			"The guard itself is gated behaviourally by TestDeclareNetworkIdentityIsBehaviouralAndOnTheLoop. " +
			"D-TOKEN-DOMAIN-CHAINLESS-CLIENT-2026-09-12, route (a); residual R-CHAINID-INSTALL-SOURCE-GATED.")
	}
	if !calls["FetchCanonicalIssuersFromAny"] {
		t.Fatal("SOURCE GATE is reading the wrong body: swarmAdd's own token block calls " +
			"FetchCanonicalIssuersFromAny and this walk did not see it")
	}
}
