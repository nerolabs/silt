package chain

// R3.1 gate G-R31-4 (SI-4, key-space injectivity): every field-tag constant used with
// statehash.Key ends in exactly ONE NUL and contains no other NUL, so the first \x00 always
// terminates the tag and no raw key under one tag can equal a key under another.
// TestKeyIsFieldTagPrefixInjective (core/statehash) is a two-case spot check and does not
// discharge this; this gate reads every `tag*` string constant in this package's statehash.go.
// RUNTIME GATE: TestKeyIsFieldTagPrefixInjective observes the property on two tags; this gate
// adds the whole set, by source.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestR31EveryStateHashTagEndsInExactlyOneNUL(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "statehash.go", nil, 0)
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot parse statehash.go: %v", err)
	}
	seen := 0
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range vs.Names {
			if !strings.HasPrefix(name.Name, "tag") || i >= len(vs.Values) {
				continue
			}
			lit, ok := vs.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			val, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("SOURCE GATE: cannot unquote %s: %v", name.Name, err)
			}
			seen++
			if strings.Count(val, "\x00") != 1 || !strings.HasSuffix(val, "\x00") || len(val) < 2 {
				t.Fatalf("SOURCE GATE: tag constant %s = %q must end in exactly one NUL and contain no other — the first NUL is what makes statehash.Key(tag, raw) injective across tags (SI-4)", name.Name, val)
			}
		}
		return true
	})
	if seen < 5 {
		t.Fatalf("SOURCE GATE: found only %d tag* string constants in statehash.go; the inventory anchor moved", seen)
	}
}
