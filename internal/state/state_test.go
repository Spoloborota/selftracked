package state_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/schema"
	"github.com/Spoloborota/selftracked/internal/state"
)

// TestRenderDeterministic is the property R14 rests on: two renders of the
// same database with no intervening writes are byte-identical.
func TestRenderDeterministic(t *testing.T) {
	t.Parallel()
	db, err := schema.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := t.Context()
	if err := schema.Create(ctx, db); err != nil {
		t.Fatal(err)
	}
	// Populate a little so the render is not the empty case.
	for _, q := range []string{
		`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','ship it','ACTIVE','2020-01-01T00:00:00Z')`,
		`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','a','DONE')`,
		`INSERT INTO stories (epic, id, title, status) VALUES ('e','S2','b','IN-PROGRESS')`,
		`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t','OPEN','d','d')`,
		`INSERT INTO events (at, entity, event, detail) VALUES ('2020-01-02T00:00:00Z','#1','create','made it')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	first, err := state.Render(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	second, err := state.Render(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("two renders with no intervening write must be byte-identical")
	}
	got := string(first)
	for _, want := range []string{
		"# Project state", "## Active epics", "**e** — ship it",
		"1 done, 1 in progress", "## Task queue", "open: 1", "## Recent activity", "#1 create: made it",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("render missing %q; got:\n%s", want, got)
		}
	}
}

// TestRenderEmpty covers the fresh-init shape: sections present, empty
// markers where there is no data.
func TestRenderEmpty(t *testing.T) {
	t.Parallel()
	db, err := schema.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := schema.Create(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	out, err := state.Render(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{"## Active epics\n\n_none_", "## Recent activity\n\n_none yet_", "open: 0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("empty render missing %q; got:\n%s", want, got)
		}
	}
	if !bytes.HasSuffix(out, []byte("\n")) {
		t.Fatal("STATE.md must end with a trailing newline")
	}
}
