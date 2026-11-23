package depcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// The network never updates itself. Operators control their own software, so no
// shipped code may locate its own executable, replace the running process image, or
// spawn a process, apart from the one listed use: handing a URL to the desktop's
// browser.
//
// The gate is structural. It sees calls by name in the source, so a binary that
// replaced itself by another route (a shell script it writes, a plugin loader) would
// pass it. UNGATED: self-replacement by a route other than the calls named here; no
// test runs the binary and watches its own file.
func TestNoShippedCodeReplacesItsOwnBinary(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]string{
		"os.Executable":       "locates the running binary, the first step of replacing it",
		"os.StartProcess":     "spawns a process",
		"syscall.Exec":        "replaces the running process image",
		"unix.Exec":           "replaces the running process image",
		"exec.Command":        "spawns a process",
		"exec.CommandContext": "spawns a process",
	}
	// file -> the calls it may make, and why.
	allowed := map[string]map[string]bool{
		"cmd/silt/client.go": {"exec.Command": true}, // openBrowser: hands a URL to the desktop
	}
	skip := map[string]bool{"archive": true, "vendor": true, "testdata": true, ".git": true,
		"integration": true, "e2e": true, "sim": true, "internal": true, "scripts": true}

	fset := token.NewFileSet()
	err = filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != repoRoot && skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, path)
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			name := pkg.Name + "." + sel.Sel.Name
			if why, bad := forbidden[name]; bad && !allowed[filepath.ToSlash(rel)][name] {
				t.Errorf("SOURCE GATE: %s calls %s at %s, which %s; a node never replaces its own binary",
					rel, name, fset.Position(call.Pos()), why)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
