//go:build !bbootstrap

package node

// — the node tier's default-build gate.

import (
	"reflect"
	"testing"
)

// TestDefaultBuildHasNoBBootstrapReaderOnTheNode asserts the node-tier seam is
// absent from a default silt binary. core/node/bbootstrap.go carries the `bbootstrap`
// build tag and has NO untagged stub, because nothing untagged calls it — the only
// caller is cmd/silt's status renderer, tagged the same way.
//
// It is reflection rather than a compile error so that a re-declaration fails with the
// method's name and this decision's ID instead of an unexplained build break.
func TestDefaultBuildHasNoBBootstrapReaderOnTheNode(t *testing.T) {
	if _, ok := reflect.TypeOf(&Node{}).MethodByName("BBootstrap"); ok {
		t.Fatalf("(*Node).BBootstrap exists in a DEFAULT build. The B_bootstrap instrument compiles only under the `bbootstrap` build tag (the design notes); an untagged declaration puts the census reader back into every shipped binary")
	}
}
