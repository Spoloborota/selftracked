package scaffold

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/schema"
)

var updateGolden = flag.Bool("update-golden", false, "regenerate the init golden files")

const goldenDir = "testdata/golden"

// dbFileRel is the one generated file excluded from golden comparison:
// db.sqlite's bytes are not deterministic across runs.
const dbFileRel = ".selftracked/db.sqlite"

// TestInitGolden regenerates every deterministic artifact `init` writes and
// compares it byte-for-byte to a tracked golden copy. The golden files ARE
// the review surface for what a fresh tracker ships; -update-golden
// regenerates them FROM the code (never hand-edit them to match).
func TestInitGolden(t *testing.T) {
	root := t.TempDir()
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	got := walkFiles(t, root)

	if *updateGolden {
		regenGolden(t, root, got)
		return
	}
	// Every generated file (bar the binary DB) must match its golden.
	for _, rel := range got {
		if rel == dbFileRel {
			continue
		}
		gp := filepath.Join(goldenDir, rel)
		want, err := os.ReadFile(gp)
		if err != nil {
			t.Fatalf("no golden for %s (run: go test ./internal/scaffold -update-golden): %v", rel, err)
		}
		have, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if string(have) != string(want) {
			t.Errorf("%s differs from its golden; regenerate with -update-golden if intended", rel)
		}
	}
	// No orphan goldens: a golden with no matching generated file means init
	// stopped writing something.
	assertNoOrphanGoldens(t, got)
}

func walkFiles(t *testing.T, root string) []string {
	t.Helper()
	var rels []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(rels)
	return rels
}

func regenGolden(t *testing.T, root string, rels []string) {
	t.Helper()
	if err := os.RemoveAll(goldenDir); err != nil {
		t.Fatal(err)
	}
	for _, rel := range rels {
		if rel == dbFileRel {
			continue
		}
		src := filepath.Join(root, rel)
		dst := filepath.Join(goldenDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("regenerated %d golden files under %s", len(rels)-1, goldenDir)
}

func assertNoOrphanGoldens(t *testing.T, got []string) {
	t.Helper()
	present := map[string]bool{}
	for _, r := range got {
		present[r] = true
	}
	err := filepath.WalkDir(goldenDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(goldenDir, p)
		if err != nil {
			return err
		}
		if rel = filepath.ToSlash(rel); !present[rel] {
			t.Errorf("orphan golden %s: init no longer writes it", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestInitSeedsMetaAndPaths covers INV-418 (meta system rows) and INV-064
// (the default path dictionary), read from the DB init built.
func TestInitSeedsMetaAndPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	db, err := schema.Open(filepath.Join(root, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var version, boundary string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='events_archived_through'`).Scan(&boundary); err != nil {
		t.Fatal(err)
	}
	if version == "" || boundary != "0" {
		t.Fatalf("meta seed wrong: schema_version=%q events_archived_through=%q", version, boundary)
	}
	var paths int
	if err := db.QueryRow(`SELECT COUNT(*) FROM path_dictionary`).Scan(&paths); err != nil {
		t.Fatal(err)
	}
	if paths != len(defaultRoots) {
		t.Fatalf("expected %d seeded path rows, got %d", len(defaultRoots), paths)
	}
}

// TestInitForce covers INV-188's --force: a second init refuses without it
// and rebuilds with it.
func TestInitForce(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx := context.Background()
	if err := writeScaffold(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	var exists *existsError
	if err := writeScaffold(ctx, root, false); err == nil || !errors.As(err, &exists) {
		t.Fatalf("second init without --force must refuse with existsError, got %v", err)
	}
	if err := writeScaffold(ctx, root, true); err != nil {
		t.Fatalf("second init with --force must succeed, got %v", err)
	}
}

// TestInitDoesNotClobberAdopterFiles covers the adoption posture: an
// existing AGENTS.md / .claude settings and a populated .gitignore survive.
func TestInitDoesNotClobberAdopterFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "MY OWN AGENTS FILE\n")
	writeFile(t, root, ".gitignore", "node_modules/\n")
	if err := writeScaffold(context.Background(), root, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, root, "AGENTS.md"); got != "MY OWN AGENTS FILE\n" {
		t.Fatalf("init clobbered the adopter's AGENTS.md: %q", got)
	}
	gi := readFile(t, root, ".gitignore")
	if !strings.Contains(gi, "node_modules/") || !strings.Contains(gi, ".selftracked/db.sqlite") {
		t.Fatalf(".gitignore merge dropped a line: %q", gi)
	}
	// Idempotent merge: a second (forced) init adds no duplicate entries.
	if err := writeScaffold(context.Background(), root, true); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(readFile(t, root, ".gitignore"), ".selftracked/db.sqlite"); n != 1 {
		t.Fatalf("gitignore entry duplicated on re-init: %d occurrences", n)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
