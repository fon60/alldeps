package domain

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// importsOf parses every .go file in dir and returns its import paths per
// file. Test files are included: the boundary must hold for them too.
func importsOf(t *testing.T, dir string) map[string][]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	out := map[string][]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s/%s: %v", dir, name, err)
		}
		imps := make([]string, 0, len(f.Imports))
		for _, imp := range f.Imports {
			imps = append(imps, strings.Trim(imp.Path.Value, `"`))
		}
		out[name] = imps
	}
	return out
}

// TestDomainImportBoundary guards the seam (design D6): the domain core is
// dependency-free — it may import nothing under internal/adapter and no TUI
// package.
func TestDomainImportBoundary(t *testing.T) {
	for file, imps := range importsOf(t, ".") {
		for _, imp := range imps {
			if strings.HasPrefix(imp, "npmitude/internal/adapter/") {
				t.Errorf("domain/%s imports adapter package %s", file, imp)
			}
			if strings.HasPrefix(imp, "github.com/charmbracelet/") || strings.HasPrefix(imp, "github.com/muesli/") {
				t.Errorf("domain/%s imports TUI package %s", file, imp)
			}
		}
	}
}

// TestAppImportBoundary guards the seam (design D6): the app consumes only
// the domain core and the ecosystem port — it may import no adapter package
// and no node-ecosystem implementation detail directly.
func TestAppImportBoundary(t *testing.T) {
	for file, imps := range importsOf(t, filepath.Join("..", "app")) {
		for _, imp := range imps {
			if strings.HasPrefix(imp, "npmitude/internal/adapter/") {
				t.Errorf("app/%s imports adapter package %s", file, imp)
			}
			switch imp {
			case "npmitude/internal/npmcmd", "npmitude/internal/registry", "npmitude/internal/prefix":
				t.Errorf("app/%s imports node-ecosystem package %s directly", file, imp)
			}
		}
	}
}
