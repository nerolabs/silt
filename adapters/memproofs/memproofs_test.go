package memproofs_test

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memproofs"
	"github.com/nerolabs/silt/adapters/prooftest"
	"github.com/nerolabs/silt/ports"
)

func TestConformance(t *testing.T) {
	prooftest.Run(t, func(t *testing.T) ports.ProofStore { return memproofs.New() })
}
