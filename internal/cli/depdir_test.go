package cli_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDependencyDirection audits the §3 layering for the layers that exist
// at S2: integration → CLI → store, arrows never reversed. The store must
// not know the CLI exists, the reference grammar is a leaf, and no library
// imports the command. The integration layer's thinness is S8's audit.
func TestDependencyDirection(t *testing.T) {
	t.Parallel()

	const module = "github.com/Spoloborota/selftracked"
	forbidden := map[string][]string{
		"internal/schema": {module + "/internal/cli", module + "/internal/ref", module + "/cmd"},
		"internal/ref":    {module + "/internal/cli", module + "/internal/schema", module + "/cmd"},
		"internal/cli":    {module + "/cmd"},
	}

	root := repoRoot(t)
	for dir, banned := range forbidden {
		for _, imp := range packageImports(t, filepath.Join(root, dir)) {
			for _, b := range banned {
				if imp == b || strings.HasPrefix(imp, b+"/") {
					t.Errorf("%s imports %s — the dependency arrow points the wrong way", dir, imp)
				}
			}
		}
	}
}

// repoRoot walks up from the test's directory to go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test directory")
		}
		dir = parent
	}
}

// packageImports reads the import paths of every non-test file in dir.
// Production arrows are what the architecture constrains; test files may
// reach across layers to drive them.
func packageImports(t *testing.T, dir string) []string {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var imports []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			imports = append(imports, strings.Trim(imp.Path.Value, `"`))
		}
	}
	return imports
}
