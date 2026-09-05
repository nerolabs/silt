//go:build !bbootstrap

package credit

// D-BB-BUILD-TAG — the DEFAULT-BUILD half. Everything here asserts an ABSENCE, so it can
// only compile and run without the `bbootstrap` tag.

import (
	"reflect"
	"testing"
)

// hasBBootstrap is false in a default build. Its tagged twin is in
// r29a_build_tag_present_test.go; together they let a shared test assert on whichever
// build it is running in without repeating a tag.
const hasBBootstrap = false

// TestR29aDefaultBuildHasNoCensusReaderOnTheLedger is the compiler-level claim, checked
// by reflection so it fails with a name rather than a build error if someone re-exports
// a census reader from an untagged file.
//
// The two methods are the instrument's whole exported ledger surface: the setter that
// injects the age clock, and the only route the histogram takes out of this package.
// Neither may exist in a default silt binary.
func TestR29aDefaultBuildHasNoCensusReaderOnTheLedger(t *testing.T) {
	typ := reflect.TypeOf(&Ledger{})
	for _, name := range []string{"BBootstrapPublish", "SetObservabilityClock"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Fatalf("*Ledger.%s exists in a DEFAULT build. The B_bootstrap instrument compiles only under the `bbootstrap` build tag (D-BB-BUILD-TAG, docs/decisions.md); an untagged file re-declaring it puts the mechanism back into every shipped binary", name)
		}
	}
}
