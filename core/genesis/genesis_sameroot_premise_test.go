package genesis_test

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/genesis"
)

// The other half of the named-premise guard for residual R-G (PE ruling
// RULING-618-updated-sameroot-dedup-fix-2026-08-28; deliberation
// docs/thinking/2026-08-28-genesis-sameroot-residual.md). Its sibling is
// TestGenesisSameRootApplyIsOrderDependent in core/chain (white-box, on
// bondRootOwner). That test could not live here — genesis imports chain, so a
// chain test importing genesis is an import cycle.
//
// The residual: genesis apply() is order-dependent for two distinct-ID unproven
// same-root bond regs, and AppendGenesis does NOT run the seenRoot dedup. It is
// safe TODAY only because the production genesis is a byte-identical shared
// constant that carries NO BondRegs. This test pins BOTH of those facts on the
// real genesis.Build, so the residual cannot become reachable in production
// without tripping a named check.

// TestProductionGenesisCarriesNoBondRegs pins the actual thing that keeps R-G
// unreachable: the production genesis is byte-identical across nodes and carries
// NO BondRegs, so the un-guarded order-dependent apply() path is never exercised
// in production.
//
// Teeth: if a future change lets the production genesis carry BondRegs (a
// config-driven or downloaded genesis, a genesis builder that seeds validators),
// or makes it per-node, the order-dependent path becomes reachable — and this
// test fails, forcing reachability to be re-assessed against option (b) (a
// research-gated genesis-validity change) before it ships.
func TestProductionGenesisCarriesNoBondRegs(t *testing.T) {
	// Byte-identity across nodes: two independent Build calls (distinct stores,
	// standing in for two nodes) must yield the identical block hash. This is the
	// "declared, not agreed" property the whole residual safety rests on.
	gb1, _, _, err1 := genesis.Build(memstore.New())
	gb2, _, _, err2 := genesis.Build(memstore.New())
	if err1 != nil || err2 != nil {
		t.Fatalf("genesis.Build failed: %v / %v", err1, err2)
	}
	if gb1.Hash() != gb2.Hash() {
		t.Fatalf("PREMISE CHANGED: two nodes' genesis.Build produced DIFFERENT block "+
			"hashes (%x vs %x) — genesis is no longer the byte-identical shared constant. "+
			"Residual R-G's safety rests on this identity; a per-node genesis makes the "+
			"un-guarded genesis apply() order-dependence reachable. Route option (b) of "+
			"docs/thinking/2026-08-28-genesis-sameroot-residual.md before this ships.",
			gb1.Hash(), gb2.Hash())
	}

	// No BondRegs: the order-dependent apply() path is not exercised by the real
	// genesis at all (genesis.go builds Entries only).
	if len(gb1.BondRegs) != 0 {
		t.Fatalf("PREMISE CHANGED: the production genesis now carries %d BondReg(s). "+
			"The un-guarded genesis apply() order-dependence (see core/chain "+
			"TestGenesisSameRootApplyIsOrderDependent) is now REACHABLE in production, "+
			"because AppendGenesis does NOT dedup same-root distinct-ID regs. Re-assess "+
			"reachability and route option (b) of "+
			"docs/thinking/2026-08-28-genesis-sameroot-residual.md before shipping a "+
			"BondReg-carrying genesis.", len(gb1.BondRegs))
	}
	t.Logf("premise pinned: production genesis is byte-identical (%x) and carries 0 BondRegs",
		gb1.Hash())
}
