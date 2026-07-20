package verb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/schema"
	"github.com/Spoloborota/selftracked/internal/state"
)

// statePath is STATE.md at the repo root — a sibling of .selftracked.
func statePath(dir string) string { return filepath.Join(dir, stateFile) }

// renderExpected returns the bytes state.Render produces for the seeded DB —
// the ground truth STATE.md must byte-equal (INV-274, R1 check 3).
func renderExpected(t *testing.T, dir string) []byte {
	t.Helper()
	db, err := schema.OpenRead(filepath.Join(dir, instanceDir, dbFile))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	want, err := state.Render(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return want
}

// TestStateWritesRenderByteEqual is INV-274: `state` regenerates STATE.md
// byte-equal to the deterministic render over the live DB.
func TestStateWritesRenderByteEqual(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)

	if err := stateRun(&cli.Env{Stdout: os.Stderr, Stderr: os.Stderr}); err != nil {
		t.Fatalf("state: %v", err)
	}
	got, err := os.ReadFile(statePath(dir))
	if err != nil {
		t.Fatalf("STATE.md not written: %v", err)
	}
	if want := renderExpected(t, dir); string(got) != string(want) {
		t.Fatalf("STATE.md does not byte-equal its render:\n got %q\nwant %q", got, want)
	}
}

// TestStateTracksDBChange: a DB mutation followed by `state` updates STATE.md
// to the new render — the file is a projection, not a snapshot.
func TestStateTracksDBChange(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)

	// A write verb through the pipeline already refreshes STATE.md (the wired
	// stub); here assert the standalone `state` verb tracks a change too.
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		if _, err := tx.ExecContext(context.Background(),
			`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'ship it', 'ACTIVE', 'd')`); err != nil {
			return nil, err
		}
		return []Event{{Entity: "e", Event: evEpic, Detail: "create"}}, nil
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := stateRun(&cli.Env{Stdout: os.Stderr, Stderr: os.Stderr}); err != nil {
		t.Fatalf("state: %v", err)
	}
	got, err := os.ReadFile(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if want := renderExpected(t, dir); string(got) != string(want) {
		t.Fatalf("STATE.md stale after a change:\n got %q\nwant %q", got, want)
	}
}

// TestWriteVerbRefreshesState is the pipeline-wiring assertion: a write verb
// alone (no `state` call) leaves STATE.md byte-equal to its render, because
// the stub is now wired to renderState.
func TestWriteVerbRefreshesState(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedInstance(t, dir)

	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		if _, err := tx.ExecContext(context.Background(),
			`INSERT INTO tasks (title, created_at, updated_at) VALUES ('t','d','d')`); err != nil {
			return nil, err
		}
		return []Event{{Entity: "#1", Event: evCreate, Detail: "t"}}, nil
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(statePath(dir))
	if err != nil {
		t.Fatalf("write did not refresh STATE.md: %v", err)
	}
	if want := renderExpected(t, dir); string(got) != string(want) {
		t.Fatalf("write left STATE.md stale:\n got %q\nwant %q", got, want)
	}
}

// TestStateRefusesOutsideTracker: `state` needs a database; outside one it
// refuses rather than writing a stray STATE.md.
func TestStateRefusesOutsideTracker(t *testing.T) {
	t.Chdir(t.TempDir())
	err := stateRun(&cli.Env{Stdout: os.Stderr, Stderr: os.Stderr})
	if err == nil {
		t.Fatal("expected a refusal outside a tracker")
	}
	var coded *cli.CodedError
	if !errors.As(err, &coded) {
		t.Fatalf("want a coded refusal, got %v", err)
	}
}
