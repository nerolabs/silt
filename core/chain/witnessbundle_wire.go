package chain

import (
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// The witness bundle ON THE WIRE.
//
// statehash.Witness wraps a library proof behind an unexported field, so a bundle cannot be
// handed to a codec as it stands. This file is the shape that crosses: the same fields, with
// every proof carried as the bytes statehash.Witness.MarshalBinary already produces and
// UnmarshalWitness already reads under a ceiling. Nothing is reinterpreted on the way — the
// encoding is a transport, not a second definition of what a witness is.
//
// IT CARRIES EVERY CLASS THE BUNDLE HAS, and the round-trip gate is what keeps that true: a field
// added to StateRootWitness and left out here would hand a box a bundle that is short for a reason
// no error names, and the box's stall would read as a forged root. So the gate compares a real
// bundle field for field across the wire rather than checking that the encoder ran.

var (
	// ErrBundleWireUncarriedClass is the encoder's refusal of a bundle whose own fields disagree —
	// a member set naming an id it carries no witness for. Serving it would produce a stall the box
	// attributes to the block rather than to the bundle.
	ErrBundleWireUncarriedClass = errors.New("chain: witness bundle wire — the bundle names a member it carries no witness for")
	// ErrBundleWireTooLarge is the decoder's refusal of an oversized bundle, before it allocates.
	ErrBundleWireTooLarge = errors.New("chain: witness bundle wire — the encoded bundle exceeds the reader's ceiling")
)

type witnessProofWire = []byte

type changedLeafWire struct {
	Key            []byte                  `cbor:"1,keyasint"`
	OldValue       []byte                  `cbor:"2,keyasint"`
	Proof          witnessProofWire        `cbor:"3,keyasint"`
	DeleteSiblings []statehash.FoldSibling `cbor:"4,keyasint"`
}

type digestPreSetWire struct {
	Tag    string           `cbor:"1,keyasint"`
	PreIDs []ports.NodeID   `cbor:"2,keyasint"`
	Proof  witnessProofWire `cbor:"3,keyasint"`
}

type attScreenWire struct {
	Attester      ports.NodeID     `cbor:"1,keyasint"`
	Slashed       bool             `cbor:"2,keyasint"`
	InEpochSet    bool             `cbor:"3,keyasint"`
	BondedSize    int64            `cbor:"4,keyasint"`
	BondedPresent bool             `cbor:"5,keyasint"`
	SlashedProof  witnessProofWire `cbor:"6,keyasint"`
	EpochSetProof witnessProofWire `cbor:"7,keyasint"`
	EpochSetValue []byte           `cbor:"8,keyasint"`
	BondedProof   witnessProofWire `cbor:"9,keyasint"`
}

type rotateScalarWire struct {
	OldValue []byte           `cbor:"1,keyasint"`
	Proof    witnessProofWire `cbor:"2,keyasint"`
}

type maturityWire struct {
	EverMature  rotateScalarWire `cbor:"1,keyasint"`
	MatureEpoch rotateScalarWire `cbor:"2,keyasint"`
	SeenSet     *seenSetWire     `cbor:"3,keyasint"`
}

// memberStateWire is one validatorsSeen member's committed state and the three proofs that anchor
// it. Carried as a LIST keyed by id rather than a map: the id is a 32-byte array, and a codec that
// has to render an array as a map key is a portability question this format does not need to have.
type memberStateWire struct {
	ID            ports.NodeID     `cbor:"1,keyasint"`
	Bonded        int64            `cbor:"2,keyasint"`
	BondedProof   witnessProofWire `cbor:"3,keyasint"`
	Domain        uint64           `cbor:"4,keyasint"`
	DomainPresent bool             `cbor:"5,keyasint"`
	DomainProof   witnessProofWire `cbor:"6,keyasint"`
	Slashed       bool             `cbor:"7,keyasint"`
	SlashedProof  witnessProofWire `cbor:"8,keyasint"`
}

type seenSetWire struct {
	IDs             []ports.NodeID    `cbor:"1,keyasint"`
	SeenRootWitness witnessProofWire  `cbor:"2,keyasint"`
	SeenRootValue   []byte            `cbor:"3,keyasint"`
	Members         []memberStateWire `cbor:"4,keyasint"`
}

type ttlSweepWire struct {
	Height               uint64                  `cbor:"1,keyasint"`
	Members              []ports.NodeID          `cbor:"2,keyasint"`
	BucketProof          witnessProofWire        `cbor:"3,keyasint"`
	BucketDeleteSiblings []statehash.FoldSibling `cbor:"4,keyasint"`
}

type bondRegScreenWire struct {
	Root        ports.Hash       `cbor:"1,keyasint"`
	PriorOwner  ports.NodeID     `cbor:"2,keyasint"`
	Claimed     bool             `cbor:"3,keyasint"`
	PriorProven bool             `cbor:"4,keyasint"`
	OwnerProof  witnessProofWire `cbor:"5,keyasint"`
	ProvenProof witnessProofWire `cbor:"6,keyasint"`
}

type bucketWire struct {
	DueHeight      uint64                  `cbor:"1,keyasint"`
	PreMembers     []ports.NodeID          `cbor:"2,keyasint"`
	Proof          witnessProofWire        `cbor:"3,keyasint"`
	DeleteSiblings []statehash.FoldSibling `cbor:"4,keyasint"`
}

type rotateMemberWire struct {
	ID                     ports.NodeID            `cbor:"1,keyasint"`
	Weight                 int64                   `cbor:"2,keyasint"`
	RegVersion             uint8                   `cbor:"3,keyasint"`
	RegVersionKnown        bool                    `cbor:"4,keyasint"`
	EpochSetProof          witnessProofWire        `cbor:"5,keyasint"`
	EpochSetOldValue       []byte                  `cbor:"6,keyasint"`
	EpochSetDeleteSiblings []statehash.FoldSibling `cbor:"7,keyasint"`
	QualifiedProof         witnessProofWire        `cbor:"8,keyasint"`
	RegVersionProof        witnessProofWire        `cbor:"9,keyasint"`
}

type rotateWire struct {
	Members       []rotateMemberWire `cbor:"1,keyasint"`
	PriorEpochSet []rotateMemberWire `cbor:"2,keyasint"`
	EpochStart    rotateScalarWire   `cbor:"3,keyasint"`
	MatureEpoch   rotateScalarWire   `cbor:"4,keyasint"`
	GateLockedIn  rotateScalarWire   `cbor:"5,keyasint"`
	GateHeight    rotateScalarWire   `cbor:"6,keyasint"`
	Era3LockedIn  rotateScalarWire   `cbor:"7,keyasint"`
	Era3Height    rotateScalarWire   `cbor:"8,keyasint"`
	Era4LockedIn  rotateScalarWire   `cbor:"9,keyasint"`
	Era4Height    rotateScalarWire   `cbor:"10,keyasint"`
}

// stateRootWitnessWire is the bundle as it crosses. Keys are assigned, never renumbered: a reader
// of an older build must not read one class's field as another's.
type stateRootWitnessWire struct {
	ChangedLeaves  []changedLeafWire   `cbor:"1,keyasint"`
	DueBucketProof witnessProofWire    `cbor:"2,keyasint"`
	DigestPreSets  []digestPreSetWire  `cbor:"3,keyasint"`
	AttScreens     []attScreenWire     `cbor:"4,keyasint"`
	Maturity       *maturityWire       `cbor:"5,keyasint"`
	TTLSweep       *ttlSweepWire       `cbor:"6,keyasint"`
	BondRegScreens []bondRegScreenWire `cbor:"7,keyasint"`
	BondRegBuckets []bucketWire        `cbor:"8,keyasint"`
	Rotate         *rotateWire         `cbor:"9,keyasint"`
}

// EncodeStateRootWitness renders a bundle for the wire, refusing one that carries a class this
// form does not cover.
func EncodeStateRootWitness(w StateRootWitness) ([]byte, error) {
	var firstErr error
	pf := func(pw statehash.Witness) []byte {
		b, err := proofBytes(pw)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		return b
	}
	out := stateRootWitnessWire{DueBucketProof: pf(w.DueBucketProof)}
	for _, cl := range w.ChangedLeaves {
		out.ChangedLeaves = append(out.ChangedLeaves, changedLeafWire{
			Key: cl.Key, OldValue: cl.OldValue, Proof: pf(cl.Proof), DeleteSiblings: cl.DeleteSiblings,
		})
	}
	for _, d := range w.DigestPreSets {
		out.DigestPreSets = append(out.DigestPreSets, digestPreSetWire{
			Tag: d.Tag, PreIDs: d.PreIDs, Proof: pf(d.Proof),
		})
	}
	for _, sc := range w.AttScreens {
		out.AttScreens = append(out.AttScreens, attScreenWire{
			Attester: sc.Attester, Slashed: sc.Slashed, InEpochSet: sc.InEpochSet,
			BondedSize: sc.BondedSize, BondedPresent: sc.BondedPresent,
			SlashedProof: pf(sc.SlashedProof), EpochSetProof: pf(sc.EpochSetProof),
			EpochSetValue: sc.EpochSetValue, BondedProof: pf(sc.BondedProof),
		})
	}
	if w.Maturity != nil {
		out.Maturity = &maturityWire{
			EverMature:  rotateScalarWire{OldValue: w.Maturity.EverMature.OldValue, Proof: pf(w.Maturity.EverMature.Proof)},
			MatureEpoch: rotateScalarWire{OldValue: w.Maturity.MatureEpoch.OldValue, Proof: pf(w.Maturity.MatureEpoch.Proof)},
		}
		if ss := w.Maturity.SeenSet; len(ss.IDs) > 0 {
			sw := &seenSetWire{IDs: ss.IDs, SeenRootWitness: pf(ss.SeenRootWitness), SeenRootValue: ss.SeenRootValue}
			// In the id-list's order, so the encoding is a function of the bundle and not of a map
			// iteration: two providers serving the same committed state serve the same bytes.
			for _, id := range ss.IDs {
				m, held := ss.Members[id]
				if !held {
					return nil, fmt.Errorf("%w: the class-M set names %x with no member witness", ErrBundleWireUncarriedClass, id[:8])
				}
				sw.Members = append(sw.Members, memberStateWire{
					ID: id, Bonded: m.Bonded, BondedProof: pf(m.BondedProof),
					Domain: m.Domain, DomainPresent: m.DomainPresent, DomainProof: pf(m.DomainProof),
					Slashed: m.Slashed, SlashedProof: pf(m.SlashedProof),
				})
			}
			out.Maturity.SeenSet = sw
		}
	}
	if w.TTLSweep != nil {
		out.TTLSweep = &ttlSweepWire{
			Height: w.TTLSweep.Height, Members: w.TTLSweep.Members,
			BucketProof: pf(w.TTLSweep.BucketProof), BucketDeleteSiblings: w.TTLSweep.BucketDeleteSiblings,
		}
	}
	for _, sc := range w.BondRegScreens {
		out.BondRegScreens = append(out.BondRegScreens, bondRegScreenWire{
			Root: sc.Root, PriorOwner: sc.PriorOwner, Claimed: sc.Claimed, PriorProven: sc.PriorProven,
			OwnerProof: pf(sc.OwnerProof), ProvenProof: pf(sc.ProvenProof),
		})
	}
	for _, bk := range w.BondRegBuckets {
		out.BondRegBuckets = append(out.BondRegBuckets, bucketWire{
			DueHeight: bk.DueHeight, PreMembers: bk.PreMembers,
			Proof: pf(bk.Proof), DeleteSiblings: bk.DeleteSiblings,
		})
	}
	if w.Rotate != nil {
		rm := func(ms []StateRootRotateMember) []rotateMemberWire {
			var outM []rotateMemberWire
			for _, m := range ms {
				outM = append(outM, rotateMemberWire{
					ID: m.ID, Weight: m.Weight, RegVersion: m.RegVersion, RegVersionKnown: m.RegVersionKnown,
					EpochSetProof: pf(m.EpochSetProof), EpochSetOldValue: m.EpochSetOldValue,
					EpochSetDeleteSiblings: m.EpochSetDeleteSiblings,
					QualifiedProof:         pf(m.QualifiedProof), RegVersionProof: pf(m.RegVersionProof),
				})
			}
			return outM
		}
		sc := func(s StateRootRotateScalar) rotateScalarWire {
			return rotateScalarWire{OldValue: s.OldValue, Proof: pf(s.Proof)}
		}
		out.Rotate = &rotateWire{
			Members: rm(w.Rotate.Members), PriorEpochSet: rm(w.Rotate.PriorEpochSet),
			EpochStart: sc(w.Rotate.EpochStart), MatureEpoch: sc(w.Rotate.MatureEpoch),
			GateLockedIn: sc(w.Rotate.GateLockedIn), GateHeight: sc(w.Rotate.GateHeight),
			Era3LockedIn: sc(w.Rotate.Era3LockedIn), Era3Height: sc(w.Rotate.Era3Height),
			Era4LockedIn: sc(w.Rotate.Era4LockedIn), Era4Height: sc(w.Rotate.Era4Height),
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return cbor.Marshal(out)
}

// DecodeStateRootWitness reads a bundle from the wire under a byte ceiling, checked BEFORE the
// decode allocates. maxProofBytes bounds each individual proof, so one reply cannot spend a floor
// box's memory ceiling on a single leaf.
func DecodeStateRootWitness(raw []byte, maxBytes, maxProofBytes int) (StateRootWitness, error) {
	var w StateRootWitness
	if maxBytes <= 0 || len(raw) > maxBytes {
		return w, fmt.Errorf("%w: %d bytes against a %d-byte ceiling", ErrBundleWireTooLarge, len(raw), maxBytes)
	}
	var in stateRootWitnessWire
	if err := cbor.Unmarshal(raw, &in); err != nil {
		return w, fmt.Errorf("chain: decode witness bundle: %w", err)
	}
	proof := func(b []byte) (statehash.Witness, error) { return statehash.UnmarshalWitness(b, maxProofBytes) }
	var err error
	if w.DueBucketProof, err = proof(in.DueBucketProof); err != nil {
		return StateRootWitness{}, err
	}
	for _, cl := range in.ChangedLeaves {
		pf, pErr := proof(cl.Proof)
		if pErr != nil {
			return StateRootWitness{}, pErr
		}
		w.ChangedLeaves = append(w.ChangedLeaves, StateRootChangedLeafWitness{
			Key: cl.Key, OldValue: cl.OldValue, Proof: pf, DeleteSiblings: cl.DeleteSiblings,
		})
	}
	for _, d := range in.DigestPreSets {
		pf, pErr := proof(d.Proof)
		if pErr != nil {
			return StateRootWitness{}, pErr
		}
		w.DigestPreSets = append(w.DigestPreSets, StateRootDigestWitness{Tag: d.Tag, PreIDs: d.PreIDs, Proof: pf})
	}
	for _, sc := range in.AttScreens {
		sp, e1 := proof(sc.SlashedProof)
		ep, e2 := proof(sc.EpochSetProof)
		bp, e3 := proof(sc.BondedProof)
		for _, e := range []error{e1, e2, e3} {
			if e != nil {
				return StateRootWitness{}, e
			}
		}
		w.AttScreens = append(w.AttScreens, StateRootAttScreen{
			Attester: sc.Attester, Slashed: sc.Slashed, InEpochSet: sc.InEpochSet,
			BondedSize: sc.BondedSize, BondedPresent: sc.BondedPresent,
			SlashedProof: sp, EpochSetProof: ep, EpochSetValue: sc.EpochSetValue, BondedProof: bp,
		})
	}
	if in.Maturity != nil {
		ev, e1 := proof(in.Maturity.EverMature.Proof)
		me, e2 := proof(in.Maturity.MatureEpoch.Proof)
		for _, e := range []error{e1, e2} {
			if e != nil {
				return StateRootWitness{}, e
			}
		}
		w.Maturity = &StateRootMaturityWitness{
			EverMature:  StateRootRotateScalar{OldValue: in.Maturity.EverMature.OldValue, Proof: ev},
			MatureEpoch: StateRootRotateScalar{OldValue: in.Maturity.MatureEpoch.OldValue, Proof: me},
		}
		if ss := in.Maturity.SeenSet; ss != nil {
			rw, rErr := proof(ss.SeenRootWitness)
			if rErr != nil {
				return StateRootWitness{}, rErr
			}
			out := SeenSetWitness{IDs: ss.IDs, SeenRootWitness: rw, SeenRootValue: ss.SeenRootValue,
				Members: make(map[ports.NodeID]MemberStateWitness, len(ss.Members))}
			for _, m := range ss.Members {
				bp, e1 := proof(m.BondedProof)
				dp, e2 := proof(m.DomainProof)
				sp, e3 := proof(m.SlashedProof)
				for _, e := range []error{e1, e2, e3} {
					if e != nil {
						return StateRootWitness{}, e
					}
				}
				out.Members[m.ID] = MemberStateWitness{
					Bonded: m.Bonded, BondedProof: bp,
					Domain: m.Domain, DomainPresent: m.DomainPresent, DomainProof: dp,
					Slashed: m.Slashed, SlashedProof: sp,
				}
			}
			w.Maturity.SeenSet = out
		}
	}
	if in.TTLSweep != nil {
		bp, bErr := proof(in.TTLSweep.BucketProof)
		if bErr != nil {
			return StateRootWitness{}, bErr
		}
		w.TTLSweep = &StateRootTTLWitness{
			Height: in.TTLSweep.Height, Members: in.TTLSweep.Members,
			BucketProof: bp, BucketDeleteSiblings: in.TTLSweep.BucketDeleteSiblings,
		}
	}
	for _, sc := range in.BondRegScreens {
		op, e1 := proof(sc.OwnerProof)
		pp, e2 := proof(sc.ProvenProof)
		for _, e := range []error{e1, e2} {
			if e != nil {
				return StateRootWitness{}, e
			}
		}
		w.BondRegScreens = append(w.BondRegScreens, StateRootBondRegScreen{
			Root: sc.Root, PriorOwner: sc.PriorOwner, Claimed: sc.Claimed, PriorProven: sc.PriorProven,
			OwnerProof: op, ProvenProof: pp,
		})
	}
	for _, bk := range in.BondRegBuckets {
		bp, bErr := proof(bk.Proof)
		if bErr != nil {
			return StateRootWitness{}, bErr
		}
		w.BondRegBuckets = append(w.BondRegBuckets, StateRootBucketWitness{
			DueHeight: bk.DueHeight, PreMembers: bk.PreMembers, Proof: bp, DeleteSiblings: bk.DeleteSiblings,
		})
	}
	if in.Rotate != nil {
		var memErr error
		rm := func(ms []rotateMemberWire) []StateRootRotateMember {
			var outM []StateRootRotateMember
			for _, m := range ms {
				ep, e1 := proof(m.EpochSetProof)
				qp, e2 := proof(m.QualifiedProof)
				rp, e3 := proof(m.RegVersionProof)
				for _, e := range []error{e1, e2, e3} {
					if e != nil && memErr == nil {
						memErr = e
					}
				}
				outM = append(outM, StateRootRotateMember{
					ID: m.ID, Weight: m.Weight, RegVersion: m.RegVersion, RegVersionKnown: m.RegVersionKnown,
					EpochSetProof: ep, EpochSetOldValue: m.EpochSetOldValue,
					EpochSetDeleteSiblings: m.EpochSetDeleteSiblings,
					QualifiedProof:         qp, RegVersionProof: rp,
				})
			}
			return outM
		}
		sc := func(s rotateScalarWire) StateRootRotateScalar {
			pw, e := proof(s.Proof)
			if e != nil && memErr == nil {
				memErr = e
			}
			return StateRootRotateScalar{OldValue: s.OldValue, Proof: pw}
		}
		r := &StateRootRotateWitness{
			Members: rm(in.Rotate.Members), PriorEpochSet: rm(in.Rotate.PriorEpochSet),
			EpochStart: sc(in.Rotate.EpochStart), MatureEpoch: sc(in.Rotate.MatureEpoch),
			GateLockedIn: sc(in.Rotate.GateLockedIn), GateHeight: sc(in.Rotate.GateHeight),
			Era3LockedIn: sc(in.Rotate.Era3LockedIn), Era3Height: sc(in.Rotate.Era3Height),
			Era4LockedIn: sc(in.Rotate.Era4LockedIn), Era4Height: sc(in.Rotate.Era4Height),
		}
		if memErr != nil {
			return StateRootWitness{}, memErr
		}
		w.Rotate = r
	}
	return w, nil
}

// proofBytes renders one proof. A nil witness is nil bytes, which is what an absent witness looks
// like on the wire and what UnmarshalWitness reads back as absent. A proof that will not marshal is
// an ERROR and never a silent absence: sending it as absent would hand the box a bundle that is
// short for a reason no error names, and its stall would read as a forged root.
func proofBytes(w statehash.Witness) ([]byte, error) {
	b, err := w.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("chain: witness bundle wire: %w", err)
	}
	return b, nil
}
