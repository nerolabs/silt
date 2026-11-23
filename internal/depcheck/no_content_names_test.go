package depcheck

import (
	"reflect"
	"sort"
	"testing"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/ports"
)

// Core resolves hashes, never names. Everything the core commits to the chain
// (Block), stores beside the content (the sealed Manifest) or registers (Entry) is
// walked field by field, and the only text it may carry is the pinned set below:
// the network's own label and the encryption mode. A content name, a path or a
// title would have to arrive as a new string field, and it fails here until the
// set is changed on purpose.
//
// The gate is structural. UNGATED: a name smuggled into a byte field. The runtime
// half is TestSeizedNetworkYieldsNoPlaintextWhileTheKeyHolderReadsIt, which seizes
// every disk and finds neither the published file's name nor its bytes.
func TestStoredAndCommittedStructuresCarryNoNames(t *testing.T) {
	allowed := map[string]string{
		"Block.Params.NetworkName": "the network's label, committed at genesis; names the network, never content",
		"Manifest.Mode":            "the encryption mode enum",
	}
	var found []string
	seen := map[reflect.Type]bool{}
	var walk func(tp reflect.Type, path string)
	walk = func(tp reflect.Type, path string) {
		for {
			switch tp.Kind() {
			case reflect.Pointer, reflect.Slice, reflect.Array:
				tp = tp.Elem()
				continue
			case reflect.Map:
				walk(tp.Key(), path+"[key]")
				tp = tp.Elem()
				continue
			}
			break
		}
		switch tp.Kind() {
		case reflect.String:
			found = append(found, path)
		case reflect.Struct:
			if seen[tp] {
				return
			}
			seen[tp] = true
			for i := 0; i < tp.NumField(); i++ {
				walk(tp.Field(i).Type, path+"."+tp.Field(i).Name)
			}
		case reflect.Interface:
			t.Errorf("%s is an interface: it can carry any text, so this walk cannot vouch for it", path)
		}
	}
	walk(reflect.TypeOf(chain.Block{}), "Block")
	walk(reflect.TypeOf(manifest.Manifest{}), "Manifest")
	walk(reflect.TypeOf(ports.Entry{}), "Entry")

	sort.Strings(found)
	for _, f := range found {
		if _, ok := allowed[f]; !ok {
			t.Errorf("%s is a string field in a structure the core stores or commits; a content name must never reach one", f)
		}
	}
	for f := range allowed {
		i := sort.SearchStrings(found, f)
		if i == len(found) || found[i] != f {
			t.Errorf("the pinned field %s no longer exists: shrink the allowed set so it describes the code", f)
		}
	}
}
