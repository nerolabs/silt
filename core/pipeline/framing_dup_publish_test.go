package pipeline_test

// The SECOND break class of the 4′ framing change, named and gated (blind PE, 2026-09-07).
// The manifest frame is not in the root — manifest.Root covers data + parity IDs only — so
// true-length framing changes entry.ManifestChunks under an UNCHANGED root. registry.Publish
// answers ErrDupPublish for exactly that shape: same root, different entry. A re-publish of
// pre-change content at the same explicit chunk size therefore collides at the registry
// after the scatter has shipped the bytes. This is a disclosure of the break, not a fix.

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/registry"
	"github.com/nerolabs/silt/ports"
)

func TestReframedManifestUnderAnUnchangedRootIsADupPublish(t *testing.T) {
	ctx := context.Background()
	store, reg := memstore.New(), registry.New()
	data := bytes.Repeat([]byte("the same bytes, published twice across the framing boundary "), 2048)
	const cs = 64 << 10

	// The pre-4′ publish: the manifest padded to the chunk size.
	old := pipeline.Options{ChunkSize: cs, Mode: crypto.Convergent, ManifestFrameBytes: cs}
	h1, e1, err := pipeline.Stage(ctx, store, bytes.NewReader(data), old)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Publish(ctx, e1); err != nil {
		t.Fatal(err)
	}
	// The same bytes, same explicit chunk size, today's derived (true-length) framing.
	h2, e2, err := pipeline.Stage(ctx, store, bytes.NewReader(data), pipeline.Options{ChunkSize: cs, Mode: crypto.Convergent})
	if err != nil {
		t.Fatal(err)
	}
	// Premises: the root did NOT move; the manifest chunk IDs DID.
	if h1.Root != h2.Root {
		t.Fatalf("premise: the root must be unchanged across the framing boundary (%x vs %x)", h1.Root[:4], h2.Root[:4])
	}
	if len(e1.ManifestChunks) == len(e2.ManifestChunks) && e1.ManifestChunks[0] == e2.ManifestChunks[0] {
		t.Fatal("premise: the framing must move the manifest chunk IDs, or there is no second break class to gate")
	}
	// The collision.
	if err := reg.Publish(ctx, e2); !errors.Is(err, ports.ErrDupPublish) {
		t.Fatalf("re-publishing the same root with a re-framed manifest returned %v, want ErrDupPublish — "+
			"the break class this gate discloses has moved (or the registry now accepts a second entry per root)", err)
	}
}
