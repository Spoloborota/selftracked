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
	// Populate MULTIPLE rows per section, so the test exercises ordering and
	// the 10-event window, not a trivially-stable single-row result set.
	seed := []string{
		// Two active epics, inserted out of slug order to test ORDER BY slug.
		`INSERT INTO epics (slug, goal, status, created_at) VALUES ('zebra','later','ACTIVE','2020-01-01T00:00:00Z')`,
		`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e','ship it','ACTIVE','2020-01-01T00:00:00Z')`,
		`INSERT INTO epics (slug, goal, status, status_note, created_at) ` +
			`VALUES ('paused','x','PAUSED','waiting','2020-01-01T00:00:00Z')`,
		`INSERT INTO stories (epic, id, title, status) VALUES ('e','S1','a','DONE')`,
		`INSERT INTO stories (epic, id, title, status) VALUES ('e','S2','b','IN-PROGRESS')`,
		`INSERT INTO tasks (title, status, created_at, updated_at) VALUES ('t','OPEN','d','d')`,
	}
	// Thirteen events so the LIMIT 10 window and DESC ordering both bite.
	for i := 1; i <= 13; i++ {
		seed = append(seed, `INSERT INTO events (at, entity, event, detail) VALUES `+
			`('2020-01-02T00:00:00Z','#1','create','made it')`)
	}
	for _, q := range seed {
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
		"# Project state", "## Active epics", "**e** — ship it", "**zebra** — later",
		"1 done, 1 in progress", "## Task queue", "open: 1", "## Recent activity", "#1 create: made it",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("render missing %q; got:\n%s", want, got)
		}
	}
	// Active epics are ordered by slug: 'e' before 'zebra'.
	if strings.Index(got, "**e**") > strings.Index(got, "**zebra**") {
		t.Fatal("active epics must be slug-ordered (e before zebra)")
	}
	// PAUSED epics are silent.
	if strings.Contains(got, "**paused**") {
		t.Fatal("a PAUSED epic must not appear in Active epics")
	}
	// The recent-activity window is capped at 10 even with 13 events seeded.
	if n := strings.Count(got, "#1 create: made it"); n != 10 {
		t.Fatalf("recent activity must cap at 10 events, rendered %d", n)
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
