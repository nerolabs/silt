//go:build !bbootstrap

package main

// THE DEFAULT BUILD. This file is the WHOLE of the R2.9a B_bootstrap instrument in a
// silt binary built without the `bbootstrap` tag (D-BB-BUILD-TAG, ratified 2026-09-05;
// docs/decisions.md). There is no -bbootstrap flag, no observability clock injection, no
// histogram type and no /api/status renderer in the binary.
//
// WHAT `silt daemon -bbootstrap` DOES ON A DEFAULT BINARY: it fails at flag parse with
// "flag provided but not defined: -bbootstrap". That is the intended answer. The
// mechanism is not disabled, it is absent, and there is nothing to enable.
//
// WHY THESE FOUR DECLARATIONS SURVIVE. A build tag cannot delete a struct field, a
// composite-literal member or a call from an untagged function, so the untagged tree
// keeps exactly four references — one type and three one-line calls, all inert:
//
//   - statusExtras, embedded in the GET /api/status payload. Empty here, so
//     encoding/json promotes nothing and the emitted bytes carry no extra key
//     (TestR29aDefaultBuildStatusHasNoBBootstrapKey).
//   - registerBBootstrapFlag, called where daemon.go declares its flags. Declares none.
//   - bbootstrapInject, called where daemon.go builds the ledger. Injects nothing, so
//     credit.Ledger.stampFirstFetch has no clock and recordFetched writes no
//     first-fetch time.
//   - bbootstrapWireUI, called where daemon.go builds the UI server. Wires nothing, so
//     uiServer.statusExtra stays nil.
//
// Each takes the same arguments as its tagged twin so the daemon's call sites are
// identical in both builds and cannot drift apart silently.

import (
	"flag"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// statusExtras is empty in a default build: no extra key on GET /api/status.
type statusExtras struct{}

// registerBBootstrapFlag declares no flag in a default build, and reports the
// instrument permanently off.
func registerBBootstrapFlag(*flag.FlagSet) func() bool { return func() bool { return false } }

// bbootstrapInject injects nothing in a default build. No observability clock reaches
// the ledger, so no account is stamped with a first-touch time.
func bbootstrapInject(*credit.Ledger, ports.Clock, bool) {}

// bbootstrapWireUI wires nothing in a default build. uiServer.statusExtra stays nil.
func bbootstrapWireUI(*uiServer, bool) {}
