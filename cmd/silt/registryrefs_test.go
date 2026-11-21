package main

import (
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/httpregistry"
)

// One ref opens that registry; several open a set; a list naming nothing, or
// carrying one bad ref, is refused before any publish starts.
func TestOpenRegistries(t *testing.T) {
	if r, err := openRegistries("http://127.0.0.1:1"); err != nil {
		t.Fatal(err)
	} else if _, ok := r.(*httpregistry.Client); !ok {
		t.Fatalf("one ref opened %T, want a single *httpregistry.Client", r)
	}
	if r, err := openRegistries("http://127.0.0.1:1, http://127.0.0.1:2,"); err != nil {
		t.Fatal(err)
	} else if _, ok := r.(*httpregistry.Set); !ok {
		t.Fatalf("two refs opened %T, want an *httpregistry.Set", r)
	}
	if _, err := openRegistries(" , "); err == nil || !strings.Contains(err.Error(), "names no registry") {
		t.Fatalf("an empty list = %v, want it refused", err)
	}
	if _, err := openRegistries("http://127.0.0.1:1,https://127.0.0.1:2"); err == nil || !strings.Contains(err.Error(), "no pinned identity") {
		t.Fatalf("a list with an unpinned https ref = %v, want it refused", err)
	}
}
