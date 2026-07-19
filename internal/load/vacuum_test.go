package load_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/schema"
)

// TestVacuumIntoRenameProbe is the §16 re-verification item INV-503: the
// VACUUM INTO + atomic rename flow, exercised on the pinned driver before
// anything relies on it (§8.6's rebuild path is its eventual consumer;
// VACUUM INTO's own docs warn an interrupted run leaves a corrupt target,
// which is why the rename step exists at all).
func TestVacuumIntoRenameProbe(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src, err := schema.Open(filepath.Join(dir, "src.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	if err := schema.Create(t.Context(), src); err != nil {
		t.Fatal(err)
	}
	if _, err := src.ExecContext(t.Context(),
		`INSERT INTO epics (slug, goal, created_at) VALUES ('e', 'g', 'd')`); err != nil {
		t.Fatal(err)
	}

	tmp := filepath.Join(dir, "rebuild.tmp")
	if _, err := src.ExecContext(t.Context(), `VACUUM INTO ?`, tmp); err != nil {
		t.Fatalf("VACUUM INTO on the pinned driver: %v", err)
	}
	final := filepath.Join(dir, "rebuilt.sqlite")
	if err := os.Rename(tmp, final); err != nil {
		t.Fatal(err)
	}

	re, err := schema.Open(final)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = re.Close() }()
	var verdict, goal string
	if err := re.QueryRowContext(t.Context(), `PRAGMA integrity_check`).Scan(&verdict); err != nil {
		t.Fatal(err)
	}
	if verdict != "ok" {
		t.Fatalf("integrity_check on the vacuumed copy: %s", verdict)
	}
	if err := re.QueryRowContext(t.Context(), `SELECT goal FROM epics WHERE slug='e'`).Scan(&goal); err != nil {
		t.Fatal(err)
	}
	if goal != "g" {
		t.Fatalf("data lost in the vacuumed copy: goal=%q", goal)
	}
	// The gates rode along: triggers are schema objects and VACUUM copies
	// the schema whole.
	if _, err := re.ExecContext(t.Context(), `DELETE FROM epics`); err == nil ||
		!strings.Contains(err.Error(), "never deleted") {
		t.Fatalf("the vacuumed copy lost its no-delete trigger: %v", err)
	}
}
