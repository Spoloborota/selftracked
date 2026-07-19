// S4: the loader fixtures. Subtest names are the fixture slugs of
// docs/stage-openings/s4.md; each row's verification command is
// `go test ./internal/load -run 'TestLoad/<slug>'`.
package load_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/load"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// goodDump builds a real database, seeds it coherently (events trails for
// every terminal state, so the R-rules pass), and serializes it — the
// loader's happy-path input comes from the actual serializer, never from
// hand-typed text that could drift.
func goodDump(t *testing.T) []byte {
	t.Helper()
	db, err := schema.Open(filepath.Join(t.TempDir(), "src.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := schema.Create(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'goal', 'ACTIVE', 'd')`,
		`INSERT INTO tasks (title, epic, created_at, updated_at) VALUES ('it''s quoted', 'e', 'd', 'd')`,
		`INSERT INTO stories (epic, id, title) VALUES ('e', 'S1', 't')`,
		`INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S1', 'd', 'IN-PROGRESS')`,
		`INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	text, err := dump.Serialize(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// wantRefused parses (and, if parsing succeeds, builds) and demands the
// ErrRefused family.
func wantRefused(t *testing.T, text []byte, wantSub string) {
	t.Helper()
	d, err := load.Parse(text)
	if err == nil {
		_, err = load.Build(t.Context(), t.TempDir(), d)
	}
	if err == nil {
		t.Fatalf("the dump was accepted; want a refusal containing %q", wantSub)
	}
	if !strings.Contains(err.Error(), wantSub) {
		t.Fatalf("refused with %q; want substring %q", err, wantSub)
	}
}

func buildFrom(t *testing.T, text []byte) *sql.DB {
	t.Helper()
	d, err := load.Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	dir := t.TempDir()
	path, err := load.Build(t.Context(), dir, d)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	db, err := schema.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestLoad(t *testing.T) {
	t.Parallel()

	t.Run("roundtrip-good-dump-loads-and-redumps-byte-equal", func(t *testing.T) {
		t.Parallel()
		text := goodDump(t)
		db := buildFrom(t, text)
		again, err := dump.Serialize(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(text) {
			t.Fatal("load → re-dump is not byte-identical")
		}
	})

	t.Run("tampered-ddl-one-byte-refused", func(t *testing.T) {
		t.Parallel()
		text := string(goodDump(t))
		// One byte inside a trigger body: the gate's message text.
		tampered := strings.Replace(text, "'worklog is append-only'", "'worklog is append-onlY'", 1)
		if tampered == text {
			t.Fatal("tamper target not found")
		}
		wantRefused(t, []byte(tampered), "byte-equal")
	})

	t.Run("non-whitelisted-statements-refused", func(t *testing.T) {
		t.Parallel()
		text := string(goodDump(t))
		//nolint:dupword // adjacent NULL, NULL columns are the dump's real shape
		for _, evil := range []string{
			"PRAGMA journal_mode=WAL;",
			"ATTACH DATABASE '/tmp/x' AS x;",
			`INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at)` +
				` VALUES ((SELECT 1), 't', 'OPEN', '', '', NULL, NULL, 'd', 'd');`,
			`INSERT INTO sqlite_master (type) VALUES ('x');`,
			`INSERT INTO tasks (id) VALUES (1);`,
			`DELETE FROM tasks;`,
			`INSERT INTO tasks (id, title, status, status_note, parked, dup_of, epic, created_at, updated_at)` +
				` VALUES (1, 0x41, 'OPEN', '', '', NULL, NULL, 'd', 'd');`,
		} {
			wantRefused(t, []byte(text+evil+"\n"), "refused")
		}
	})

	t.Run("header-version-selects-parser-before-meta", func(t *testing.T) {
		t.Parallel()
		// A header claiming v2 must refuse BEFORE any data parsing — the
		// meta row deeper in the file never gets a say. The refusal message
		// is the forward-only one, not a meta mismatch, which proves order.
		text := string(goodDump(t))
		bumped := strings.Replace(text, "schema_version=1", "schema_version=2", 1)
		wantRefused(t, []byte(bumped), "requires a newer binary")
	})

	t.Run("header-meta-version-mismatch-refused", func(t *testing.T) {
		t.Parallel()
		// Header says v1; the meta ROW says v2. Grammar-valid, must refuse.
		text := string(goodDump(t))
		mismatched := strings.Replace(text,
			`INSERT INTO meta (key, value) VALUES ('schema_version', '1');`,
			`INSERT INTO meta (key, value) VALUES ('schema_version', '2');`, 1)
		if mismatched == text {
			t.Fatal("meta row not found to tamper")
		}
		wantRefused(t, []byte(mismatched), "meta row says 2")
	})

	t.Run("missing-required-meta-row-refused", func(t *testing.T) {
		t.Parallel()
		text := string(goodDump(t))
		gone := strings.Replace(text,
			`INSERT INTO meta (key, value) VALUES ('events_archived_through', '0');`+"\n", "", 1)
		if gone == text {
			t.Fatal("boundary meta row not found to remove")
		}
		wantRefused(t, []byte(gone), "events_archived_through")
	})

	t.Run("loaded-db-carries-stamped-version", func(t *testing.T) {
		t.Parallel()
		db := buildFrom(t, goodDump(t))
		var userVersion, appID int64
		if err := db.QueryRowContext(t.Context(), `PRAGMA user_version`).Scan(&userVersion); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(t.Context(), `PRAGMA application_id`).Scan(&appID); err != nil {
			t.Fatal(err)
		}
		if userVersion != schema.Version || appID != schema.ApplicationID {
			t.Fatalf("stamp: user_version=%d application_id=%d; want %d/%d",
				userVersion, appID, schema.Version, schema.ApplicationID)
		}
	})

	t.Run("events-highwater-seeded-max-boundary-liveseq", func(t *testing.T) {
		t.Parallel()
		// A dump whose only event is seq 7 (a gap below it): after load,
		// the next event must take seq 8, never reuse 1–6.
		text := string(goodDump(t))
		gapped := strings.Replace(text,
			`INSERT INTO events (seq, at, entity, event, detail) VALUES (1, 'd', '#1', 'create', '');`,
			`INSERT INTO events (seq, at, entity, event, detail) VALUES (7, 'd', '#1', 'create', '');`, 1)
		if gapped == text {
			t.Fatal("events row not found")
		}
		db := buildFrom(t, []byte(gapped))
		if _, err := db.ExecContext(t.Context(),
			`INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`); err != nil {
			t.Fatal(err)
		}
		var next int64
		if err := db.QueryRowContext(t.Context(), `SELECT MAX(seq) FROM events`).Scan(&next); err != nil {
			t.Fatal(err)
		}
		if next != 8 {
			t.Fatalf("next event seq = %d; want 8 (no reuse below the high-water)", next)
		}
	})

	t.Run("forged-boundary-aborts-before-rename", func(t *testing.T) {
		t.Parallel()
		// events_archived_through=5 is grammar-valid and internally
		// consistent — only R9 knows no v0 verb can write it.
		text := string(goodDump(t))
		forged := strings.Replace(text,
			`INSERT INTO meta (key, value) VALUES ('events_archived_through', '0');`,
			`INSERT INTO meta (key, value) VALUES ('events_archived_through', '5');`, 1)
		wantRefused(t, []byte(forged), "R9")
	})

	t.Run("trailless-terminal-aborts-before-rename", func(t *testing.T) {
		t.Parallel()
		// A DONE task with no events trail: R12's forgery signature.
		text := string(goodDump(t))
		forged := strings.Replace(text,
			`VALUES (1, 'it''s quoted', 'NEEDS-TRIAGE',`,
			`VALUES (1, 'it''s quoted', 'DONE',`, 1)
		forged = strings.Replace(forged, `'DONE', '', ''`, `'DONE', 'n', ''`, 1)
		if forged == text {
			t.Fatal("task row not found to forge")
		}
		wantRefused(t, []byte(forged), "R12")
	})

	t.Run("load-lands-by-rename", func(t *testing.T) {
		t.Parallel()
		// Every refusal above must leave no database and no temp litter —
		// the target appears only via rename of a fully verified build.
		dir := t.TempDir()
		text := string(goodDump(t))
		forged := strings.Replace(text,
			`INSERT INTO meta (key, value) VALUES ('events_archived_through', '0');`,
			`INSERT INTO meta (key, value) VALUES ('events_archived_through', '5');`, 1)
		d, err := load.Parse([]byte(forged))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := load.Build(t.Context(), dir, d); err == nil {
			t.Fatal("forged dump built successfully")
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			t.Errorf("aborted build left litter: %s", e.Name())
		}
	})

	t.Run("load-connection-hardening-active", func(t *testing.T) {
		t.Parallel()
		db, err := schema.OpenLoadBuild(filepath.Join(t.TempDir(), "probe.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		for pragma, want := range map[string]string{
			"cell_size_check": "1",
			"mmap_size":       "0",
			"trusted_schema":  "0",
		} {
			var got string
			if err := db.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("PRAGMA %s = %s; want %s", pragma, got, want)
			}
		}
	})
}
