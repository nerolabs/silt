package chain

import (
	"crypto/ed25519"
	"errors"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) version-boundary rule — the WRITE-PATH guard era3validity.go's
// validateEra4Version comment cites.
//
// The comment claims validateEra4Version "runs on the SAME write paths as
// validateEra3Version ... so 'every disk-write path enforces the era boundary' is uniform
// across era-3 AND era-4". This file is that claim, tested. It has three legs, because no
// single one covers the property:
//
//  1. STRUCTURAL — every Chain method that writes a block to the live committed history
//     (discovered by scanning for c.apply, not a hand list) runs the era-4 rule. This is
//     the leg that catches a FUTURE write path (fast-sync, import) written years from now.
//  2. SCANNER COMPLETENESS — the structural scan reads chain.go, so it is only total if
//     chain.go is where the live-receiver applies are. Pinned here, so moving a write path
//     into another file cannot silently escape leg 1.
//  3. BEHAVIORAL — each real entry point is DRIVEN with a signature-valid sub-v5 block at
//     the era-4 boundary and must reject it with ErrEra4VersionRequired, leaving nothing
//     applied. Leg 1 proves a validator is CALLED; leg 3 proves the call actually rejects
//     and rejects for the era reason, not a signature failure.
//
// Overlap with TestEveryDiskWritePathRunsTheEra3RootCheck is deliberate and partial: that
// guard's rule list already includes validateEra4Version, which gives leg 1 for free but
// gives neither leg 2 nor leg 3 — and it is named and motivated for era-3, so an era-4
// regression reads there as an era-3 test failing. This is the era-4 guard by name.
//
// RED (demonstrated, 2026-09-02): delete the `validateEra4Version` call from
// appendStructural and leg 1 names appendStructural while leg 3's Reload subtest shows the
// forged v4 boundary block being persisted.
func TestEveryDiskWritePathRunsTheEra4VersionCheck(t *testing.T) {
	t.Run("structural/every-apply-caller-runs-the-rule", func(t *testing.T) {
		src := readChainSource(t)
		methods := methodsCallingApply(t, src)
		if len(methods) == 0 {
			t.Fatal("found NO methods calling c.apply — the scanner is broken (it must find at " +
				"least Append/appendStructural/AppendGenesis), so this guard would pass vacuously")
		}

		// Validators that themselves reach validateEra4Version. ValidateProposal calls it
		// directly; ValidateCommit reaches it through ValidateProposal. Both are verified
		// below rather than assumed, so the allowance cannot rot into a fiction.
		transitiveGuards := []string{"ValidateProposal", "ValidateCommit"}
		// AppendGenesis is the only exemption: a v1 genesis is declared-not-agreed, sits
		// below any era-4 boundary and commits no maintenance-spine roots by construction.
		// Adding to this map is a reviewed decision, never a silent default.
		genesisAllowlist := map[string]bool{"AppendGenesis": true}

		for _, g := range transitiveGuards {
			body := methodBody(t, src, g)
			reaches := callsFn(body, "validateEra4Version")
			for _, other := range transitiveGuards {
				if other != g && callsFn(body, other) {
					reaches = true
				}
			}
			if !reaches {
				t.Fatalf("transitive guard %q no longer reaches validateEra4Version — the "+
					"coverage assumption rotted, and every disk-write path relying on it is now "+
					"unguarded for the era-4 boundary", g)
			}
		}

		for _, m := range methods {
			if genesisAllowlist[m] {
				continue
			}
			body := methodBody(t, src, m)
			guarded := callsFn(body, "validateEra4Version")
			for _, g := range transitiveGuards {
				if callsFn(body, g) {
					guarded = true
				}
			}
			if !guarded {
				t.Errorf("method %q writes a block to the committed history (calls c.apply) but "+
					"does NOT run validateEra4Version (neither directly nor via %v) and is not on "+
					"the genesis allowlist.\nA disk-write path that skips the era-4 version rule "+
					"persists a v4 block at/above H_era4 — a block that commits none of the "+
					"maintenance-spine keyspaces the era-4 witnesses read, silently dropping the "+
					"witnessable-transition commitments. Route the block through "+
					"validateEra4Version BEFORE apply, so a rejection leaves nothing applied.",
					m, transitiveGuards)
			}
		}
	})

	t.Run("structural/scanner-covers-the-whole-package", func(t *testing.T) {
		// Leg 1 scans chain.go only. That is total only while chain.go holds every apply on
		// a LIVE chain. The one apply outside it is postApplyRoots' `scratch.apply` — a
		// throwaway clone, not a disk write. Any OTHER receiver calling .apply in a non-test
		// file is either a new write path (which must move into leg 1's scan) or a new clone
		// (which must be named here), and either way is a reviewed change, not a default.
		cloneReceivers := map[string]bool{"scratch": true}
		for file, src := range packageSources(t) {
			if file == "chain.go" {
				continue
			}
			for _, recv := range applyReceivers(src) {
				if cloneReceivers[recv] {
					continue
				}
				t.Errorf("%s calls %s.apply(...) but leg 1's structural scan reads chain.go "+
					"ONLY — this call site is invisible to the era-4 write-path guard.\nIf it "+
					"applies to a LIVE chain it is an unguarded disk-write path: move it into "+
					"chain.go or widen the scan. If it is a dry-run clone, name its receiver in "+
					"cloneReceivers here.", file, recv)
			}
		}
	})

	t.Run("behavioral/Append", func(t *testing.T) {
		c, _, bad := era4BoundaryFixture(t)
		if err := c.Append(*bad); !errors.Is(err, ErrEra4VersionRequired) {
			t.Fatalf("Append of a signature-valid v4 block AT H_era4: got %v, want "+
				"ErrEra4VersionRequired — the commit path must enforce the era-4 boundary", err)
		}
		if _, h := c.Head(); h != 4 {
			t.Fatalf("after the rejected Append the next height is %d; want 4 — a rejected "+
				"block must not be left applied (the version check runs BEFORE apply)", h)
		}
	})

	t.Run("behavioral/Reload", func(t *testing.T) {
		// The own-disk path: Reload routes every non-genesis block through appendStructural,
		// which calls validateEra4Version directly (the commit path's ValidateProposal is
		// deliberately NOT run on our own history — see appendStructural).
		c, _, bad := era4BoundaryFixture(t)
		fresh, _ := era4AnchorChain(t, 2, 4) // same cfg and same deterministic genesis
		history := append(c.Blocks(1), *bad) // heights 1..3 good, height 4 sub-v5

		n, err := fresh.Reload(history)
		if !errors.Is(err, ErrEra4VersionRequired) {
			t.Fatalf("Reload of own history ending in a signature-valid v4 block AT H_era4: got "+
				"%v, want ErrEra4VersionRequired.\nThe own-disk Reload path must run the era-4 "+
				"version rule exactly as the commit path does — otherwise a v4 block at/above "+
				"H_era4 on disk is replayed unvalidated, and the replica rebuilds from a history "+
				"missing the maintenance-spine commitments.", err)
		}
		if n != 3 {
			t.Fatalf("Reload restored %d blocks; want 3 — it must restore the valid prefix and "+
				"stop AT the offending block (longest-valid-prefix contract)", n)
		}
		if _, h := fresh.Head(); h != 4 {
			t.Fatalf("after the rejected Reload the next height is %d; want 4 — the rejected v4 "+
				"boundary block must not be left applied", h)
		}
	})

	t.Run("behavioral/Reconcile", func(t *testing.T) {
		// The peer-fork path: Reconcile replays the candidate history into a fresh replica
		// via Append, so the era-4 rule reaches it there. A fork whose boundary block is
		// sub-v5 must be refused, and this replica must be left untouched.
		c, _, bad := era4BoundaryFixture(t)
		fork := append(c.Blocks(0), *bad) // our genesis + heights 1..3 + the sub-v5 height 4

		ok, err := c.Reconcile(fork)
		if ok {
			t.Fatal("Reconcile ADOPTED a fork whose boundary block is a v4 at/above H_era4 — a " +
				"peer fork must clear the era-4 version rule before it can replace our history")
		}
		if !errors.Is(err, ErrEra4VersionRequired) {
			t.Fatalf("Reconcile of a fork ending in a v4 block AT H_era4: got %v, want an error "+
				"wrapping ErrEra4VersionRequired", err)
		}
		if _, h := c.Head(); h != 4 {
			t.Fatalf("after the rejected Reconcile the next height is %d; want 4 — a refused "+
				"fork must leave this replica's committed history untouched", h)
		}
	})

	t.Run("adopt-is-not-a-write-path", func(t *testing.T) {
		// adopt swaps in state a fresh replica already built by replaying the fork through
		// Append, so it applies no block itself and correctly runs no era-4 rule. That is
		// only sound while it stays that way: if adopt ever applies a block directly it
		// appears in leg 1's scan and must run the rule. This pins the premise.
		src := readChainSource(t)
		if strings.Contains(methodBody(t, src, "adopt"), "c.apply(") {
			t.Error("adopt now calls c.apply — it has become a disk-write path and must run " +
				"validateEra4Version (or the block it applies must have cleared it already)")
		}
		if !callsFn(methodBody(t, src, "Reconcile"), "Append") {
			t.Error("Reconcile no longer replays the fork through Append — its era-4 coverage " +
				"was transitive through Append, so that coverage is now gone")
		}
	})
}

// era4BoundaryFixture builds an anchor chain with H_era3=2 and H_era4=4, fills heights
// 1..3 (v2, v4, v4), and forges a SIGNATURE-VALID v4 block at height 4 == H_era4 with
// CORRECT era-3 roots. Correct roots and a full two-phase certificate are what make the
// fixture load-bearing: the only remaining reason any path can reject the block is the
// era-4 version rule, so a rejection cannot be a signature or root failure in disguise.
func era4BoundaryFixture(t *testing.T) (*Chain, []ed25519.PrivateKey, *Block) {
	t.Helper()
	c, keys := era4AnchorChain(t, 2, 4)
	mustAppend(t, c, mintNext4(t, c, keys)) // height 1, v2
	mustAppend(t, c, mintNext4(t, c, keys)) // height 2, v4
	mustAppend(t, c, mintNext4(t, c, keys)) // height 3, v4

	prev, next := c.Head()
	if next != 4 {
		t.Fatalf("fixture: expected next height 4 (H_era4), got %d", next)
	}
	if !c.era4Active(next) {
		t.Fatal("fixture: era4Active must be true at H_era4, or the version rule never fires " +
			"and every assertion below would pass vacuously")
	}
	bad := &Block{Height: next, Prev: prev, Entries: []ports.Entry{entry(byte(next))}}
	if err := c.PopulateEra3Roots(bad); err != nil { // stamps v4 + CORRECT v4 roots
		t.Fatalf("fixture: populate v4 roots: %v", err)
	}
	if bad.Version != BlockVersionStateRoot {
		t.Fatalf("fixture: the forged boundary block must be v4, got v%d", bad.Version)
	}
	twoPhaseSign(bad, keys)

	// Cause pin: the block is structurally VALID. Anything that rejects it below rejects
	// it for the era, not for a malformed forge.
	if err := c.validateStructural(bad); err != nil {
		t.Fatalf("fixture: validateStructural REJECTED the forged v4 boundary block (%v) — the "+
			"forge is malformed for an unrelated reason, so the version-rule assertions would "+
			"be testing the wrong thing", err)
	}
	return c, keys, bad
}

// packageSources returns every non-test .go file in this package, keyed by base name.
func packageSources(t *testing.T) map[string]string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate the source directory")
	}
	dir := thisFile[:strings.LastIndex(thisFile, "/")]
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	out := map[string]string{}
	var names []string
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, err := os.ReadFile(dir + "/" + n)
		if err != nil {
			t.Fatalf("read %s: %v", n, err)
		}
		out[n] = string(b)
		names = append(names, n)
	}
	if len(out) == 0 {
		t.Fatal("no non-test .go files found in the package — the source walk is broken")
	}
	sort.Strings(names)
	return out
}

// applyReceivers returns the distinct receiver identifiers x in every `x.apply(` call in
// src, comments stripped (so a commented-out call is not reported).
func applyReceivers(src string) []string {
	src = stripComments(src)
	seen := map[string]bool{}
	var out []string
	for i := 0; ; {
		j := strings.Index(src[i:], ".apply(")
		if j < 0 {
			break
		}
		end := i + j
		start := end
		for start > 0 && isIdentByte(src[start-1]) {
			start--
		}
		if name := src[start:end]; name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
		i = end + len(".apply(")
	}
	sort.Strings(out)
	return out
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
