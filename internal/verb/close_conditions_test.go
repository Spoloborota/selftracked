//nolint:lll // white-box: conditions (2)/(5) are unreachable through the S6 CLI (verbs write status+worklog atomically, worklog is append-only), so the fixture seeds the pathological state directly; import (S9) reaches it live
package verb

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/schema"
)

// TestCloseConditionsUnreachableViaCLI proves epic-close conditions (2)
// and (5) — INV-282, INV-285 — which the S6 close review showed cannot be
// produced through the verb surface: story done/dissolve always write the
// terminal status and its matching worklog row together, and worklog is
// append-only, so a DONE story with a non-terminal last episode (2) or a
// DONE story with no commits-bearing worklog row (5) only arises from a
// raw INSERT — which import (S9) can do, and which this test simulates.
func TestCloseConditionsUnreachableViaCLI(t *testing.T) {
	t.Parallel()

	t.Run("condition-2-non-terminal-last-episode", func(t *testing.T) {
		t.Parallel()
		db := freshEpicDB(t)
		seedTwoDoneStories(t, db)
		// A raw, contiguous, NON-correction worklog row appended AFTER
		// S1's DONE row, in a non-terminal state — the import-shaped
		// tamper condition (2) exists to catch.
		execSeed(t, db, `INSERT INTO worklog (epic, seq, story, date, state)
		             VALUES ('e', (SELECT MAX(seq)+1 FROM worklog WHERE epic='e'), 'S1', 'd', 'IN-PROGRESS')`)
		blockers := closeNow(t, db, "e")
		assertBlocker(t, blockers, "(2) story S1's last episode is not terminal")
	})

	t.Run("condition-5-done-story-without-commits", func(t *testing.T) {
		t.Parallel()
		db := freshEpicDB(t)
		// A DONE story whose only DONE worklog row carries empty commits —
		// story done requires --commits, so this is only reachable by a
		// raw insert (import).
		execSeed(t, db, `INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S1', 't', 'DONE')`)
		execSeed(t, db, `INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S2', 't', 'DISSOLVED')`)
		execSeed(t, db, `INSERT INTO worklog (epic, seq, story, date, state, commits)
		             VALUES ('e', 1, 'S1', 'd', 'DONE', '')`)
		blockers := closeNow(t, db, "e")
		assertBlocker(t, blockers, "(5) DONE story S1 has no DONE worklog row with commits")
	})

	t.Run("condition-5-legacy-commits-pass", func(t *testing.T) {
		t.Parallel()
		// `legacy: reason` IS non-empty, so it satisfies (5) visibly.
		db := freshEpicDB(t)
		execSeed(t, db, `INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S1', 't', 'DONE')`)
		execSeed(t, db, `INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S2', 't', 'DISSOLVED')`)
		execSeed(t, db, `INSERT INTO worklog (epic, seq, story, date, state, commits)
		             VALUES ('e', 1, 'S1', 'd', 'DONE', 'legacy: imported')`)
		blockers := closeNow(t, db, "e")
		for _, b := range blockers {
			if strings.HasPrefix(b, "(5)") {
				t.Fatalf("legacy: commits should satisfy condition 5, got blocker %q", b)
			}
		}
	})
}

func freshEpicDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := schema.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := schema.Create(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	execSeed(t, db, `INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'g', 'ACTIVE', 'd')`)
	return db
}

func seedTwoDoneStories(t *testing.T, db *sql.DB) {
	t.Helper()
	execSeed(t, db, `INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S1', 't', 'DONE')`)
	execSeed(t, db, `INSERT INTO stories (epic, id, title, status) VALUES ('e', 'S2', 't', 'DONE')`)
	execSeed(t, db, `INSERT INTO worklog (epic, seq, story, date, state, commits)
	             VALUES ('e', 1, 'S1', 'd', 'DONE', 'abc')`)
	execSeed(t, db, `INSERT INTO worklog (epic, seq, story, date, state, commits)
	             VALUES ('e', 2, 'S2', 'd', 'DONE', 'def')`)
}

func execSeed(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), q); err != nil {
		t.Fatalf("seed %q: %v", q, err)
	}
}

// closeNow runs the blocker evaluation inside a transaction (as the verb
// does) and rolls back — the test inspects the blocker list, not a commit.
func closeNow(t *testing.T, db *sql.DB, slug string) []string {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	blockers, _, err := closeBlockers(context.Background(), tx, slug)
	if err != nil {
		t.Fatalf("closeBlockers: %v", err)
	}
	return blockers
}

func assertBlocker(t *testing.T, blockers []string, want string) {
	t.Helper()
	if !slices.Contains(blockers, want) {
		t.Fatalf("blocker %q not found in %v", want, blockers)
	}
}
