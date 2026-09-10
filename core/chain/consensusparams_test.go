package chain

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// G-CFGBIND-1 — MEMBERSHIP IS CLOSED. Every chain.Config field is either CARRIED in
// ConsensusParams or EXPLICITLY EXCLUDED here with its reason. A new Config field fails this gate
// until someone decides which it is.
//
// This is the same closed-complement discipline as the divergence gate, applied to the bind rather
// than to the measurement — and it exists because silt named this class in prose for months with no
// enumeration, so membership was a human remembering to write a sentence. Three parameters slipped
// through that way.
//
// GETTING MEMBERSHIP WRONG IS ASYMMETRIC. Too few and a consensus quantity stays unbound (the
// defect). Too many and honest operators are refused for differing on something that was never
// theirs to agree on — which is why each exclusion below carries its OWN reason and not a shared
// one.
var paramsExcluded = map[string]string{
	"Archive":                "retention policy — whether THIS node keeps full bodies. An operator's storage choice by design (build-immutable #8); it reaches no validity verdict.",
	"WSCheckpoint":           "narrowing-only, and sharing it would DESTROY weak subjectivity. The pin is the operator's OWN trust anchor and silt is weakly subjective by design (TENETS Part 0); a network-wide value would make every node trust the same anchor, which is the opposite of the property.",
	"MinProposerRep":         "binding it would be INEFFECTIVE, not merely unnecessary: the INPUT is the local reputation view, so two nodes sharing a threshold still diverge. The real fix for that leg is objective mode, which replaces the reputation gate with committed bond.",
	"MinAttesterRep":         "same as MinProposerRep — the threshold is not the divergent term, the local view is.",
	"LivenessRecoveryHeight": "STRUCTURALLY UNBINDABLE. It is set AFTER launch, on a chain that by construction cannot commit it — the #535 recovery re-bases one boundary against the LIVE qualified set precisely because the chain is stalled. Rule 8's second arm cannot reach it (R-LIVENESS-RECOVERY-UNBOUND).",
}

func TestConsensusParamsMembershipIsComplete(t *testing.T) {
	carried := map[string]bool{}
	pt := reflect.TypeOf(ConsensusParams{})
	for i := 0; i < pt.NumField(); i++ {
		carried[pt.Field(i).Name] = true
	}
	// The two node.Config members are carried by value and have no chain.Config twin.
	for _, nodeSide := range []string{"BondLabelSamples", "BondVDFDelay"} {
		if !carried[nodeSide] {
			t.Errorf("ConsensusParams must carry %q — it reaches a hard Reject through the bond verifier "+
				"(core/bond compares a proof's label count against the verifier's OWN local value), and it "+
				"lives in node.Config where the divergence gate's reflection never sees it", nodeSide)
		}
	}

	ct := reflect.TypeOf(Config{})
	var undecided []string
	for i := 0; i < ct.NumField(); i++ {
		n := ct.Field(i).Name
		if !ct.Field(i).IsExported() {
			continue
		}
		_, isCarried := carried[n]
		_, isExcluded := paramsExcluded[n]
		switch {
		case isCarried && isExcluded:
			t.Errorf("Config.%s is BOTH carried and excluded — decide", n)
		case !isCarried && !isExcluded:
			undecided = append(undecided, n)
		}
	}
	sort.Strings(undecided)
	if len(undecided) > 0 {
		t.Fatalf("chain.Config has %d field(s) neither carried in ConsensusParams nor excluded with a reason: %v\n"+
			"Canon rule 8: a consensus quantity must be a function of the CHAIN. If the field can move a validity\n"+
			"verdict it belongs in the committed params; if it cannot, say why in paramsExcluded. Do not delete\n"+
			"this check — an unenumerated class is how MinBond stayed a bare flag through three audits.", len(undecided), undecided)
	}
	// A stale exclusion is as bad as a missing decision.
	for n := range paramsExcluded {
		if _, ok := ct.FieldByName(n); !ok {
			t.Errorf("paramsExcluded names %q, which is no longer a chain.Config field — remove the row", n)
		}
	}
}

// G-CFGBIND-2 — the genesis hash COVERS the params. This is the whole mechanism: if it did not,
// a divergent node would compute the SAME genesis hash and join happily.
func TestGCFGBIND2_GenesisHashCoversParams(t *testing.T) {
	k := key(70001)
	mk := func(p *ConsensusParams) ports.Hash {
		b := Block{Version: BlockVersionRounds, Height: 0, Entries: []ports.Entry{entry(1)}, Proposer: pubOf(k), Params: p}
		return b.Hash()
	}
	base := ParamsFromConfig(Config{Quorum: 3, MinBond: 1 << 20}, 64, 100)
	h0 := mk(&base)

	// Every carried field must move the genesis hash. A field that does not is bound by nothing.
	pt := reflect.TypeOf(ConsensusParams{})
	for i := 0; i < pt.NumField(); i++ {
		f := pt.Field(i)
		p := base
		fv := reflect.ValueOf(&p).Elem().Field(i)
		if !setNonZero(fv) {
			t.Fatalf("no non-zero constructor for ConsensusParams.%s (%s) — extend setNonZero", f.Name, f.Type)
		}
		if mk(&p) == h0 {
			t.Errorf("ConsensusParams.%s does NOT move the genesis hash — it is carried but bound by nothing, "+
				"so two nodes differing on it would compute the same genesis and join each other", f.Name)
		}
	}
	// And a paramless genesis must be byte-identical to one written before this field existed.
	if mk(nil) == h0 {
		t.Fatal("precondition: a paramless genesis must differ from one carrying params")
	}
}

// G-CFGBIND-3 — only genesis may carry params.
func TestGCFGBIND3_ParamsOnlyOnGenesis(t *testing.T) {
	p := ParamsFromConfig(Config{Quorum: 3}, 64, 100)
	g := &Block{Version: BlockVersionRounds, Height: 0, Params: &p}
	if err := validateParamsPlacement(g); err != nil {
		t.Fatalf("genesis may carry params: %v", err)
	}
	for _, h := range []uint64{1, 2, 99} {
		b := &Block{Version: BlockVersionRounds, Height: h, Params: &p}
		if err := validateParamsPlacement(b); !errors.Is(err, ErrParamsNotOnGenesis) {
			t.Fatalf("height %d carrying params must be refused: got %v", h, err)
		}
		clean := &Block{Version: BlockVersionRounds, Height: h}
		if err := validateParamsPlacement(clean); err != nil {
			t.Fatalf("height %d without params must pass: %v", h, err)
		}
	}
}

// G-CFGBIND-4 — the refuse-to-start arm catches the case JOINING cannot: an operator editing a
// flag and restarting on a chain already joined. And the refusal must be DIAGNOSABLE — naming the
// field — which is the reason values are committed rather than a digest.
func TestGCFGBIND4_RestartWithAnEditedFlagIsRefused(t *testing.T) {
	cfg := Config{Quorum: 3, MinBond: 1 << 20, ByzantineQuorum: true}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	p := ParamsFromConfig(cfg, 64, 100)
	k := key(70002)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}, Params: &p}
	g.BondRegs = append(g.BondRegs, bondReg(k, twoMiB, ports.Hash{}))
	Sign(g, k)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if err := c.CheckConsensusParams(64, 100); err != nil {
		t.Fatalf("matching config must start: %v", err)
	}
	// The edited flag: same chain on disk, different local value.
	c.cfg.MinBond = 4 << 20
	err := c.CheckConsensusParams(64, 100)
	if !errors.Is(err, ErrParamsDiverge) {
		t.Fatalf("an edited consensus flag on an already-joined chain must refuse to start: got %v", err)
	}
	if !contains2(err.Error(), "-min-bond") {
		t.Fatalf("the refusal must NAME the divergent field (this is why values are committed, not a digest): %v", err)
	}
	// The node-side verifier knobs are in the family too — the sharpest members.
	c.cfg.MinBond = 1 << 20
	if err := c.CheckConsensusParams(32, 100); !errors.Is(err, ErrParamsDiverge) {
		t.Fatalf("a divergent -bond-label-k must refuse to start: a k=32 node rejects EVERY bond registration a k=64 swarm accepts; got %v", err)
	}
}

// G-CFGBIND-5 — a genesis predating the bind still starts. The paramless path is a DISCLOSED
// residual, not an accident, so it is asserted rather than left to chance.
func TestGCFGBIND5_ParamlessGenesisStillStarts(t *testing.T) {
	c := New(Config{Quorum: 3}, func(ports.NodeID) int64 { return 0 })
	k := key(70003)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, k)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if err := c.CheckConsensusParams(64, 100); err != nil {
		t.Fatalf("a genesis predating the bind must still start (the surviving paramless path, disclosed): %v", err)
	}
}

func contains2(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
