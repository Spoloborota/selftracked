// S3: the serializer fixtures. Subtest names are the fixture slugs of
// docs/stage-openings/s3.md, so each inventory row's verification command
// is `go test ./internal/dump -run 'TestSerializer/<slug>'`.
package dump_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/schema"
)

func fresh(t *testing.T) *sql.DB {
	t.Helper()
	db, err := schema.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := schema.Create(t.Context(), db); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return db
}

func exec1(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("setup %q: %v", query, err)
	}
}

func serialize(t *testing.T, db *sql.DB) string {
	t.Helper()
	text, err := dump.Serialize(t.Context(), db)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	return string(text)
}

// seed populates one of everything so format fixtures see every table.
func seed(t *testing.T, db *sql.DB) {
	t.Helper()
	exec1(t, db, `INSERT INTO path_dictionary (class, scope, root) VALUES ('research', '', 'docs/research')`)
	exec1(t, db, `INSERT INTO epics (slug, goal, status, created_at) VALUES ('e', 'goal', 'ACTIVE', 'd')`)
	exec1(t, db, `INSERT INTO epic_criteria (epic, seq, criterion) VALUES ('e', 1, '$ true')`)
	exec1(t, db, `INSERT INTO tasks (title, epic, created_at, updated_at) VALUES ('with epic', 'e', 'd', 'd')`)
	exec1(t, db, `INSERT INTO tasks (title, created_at, updated_at) VALUES ('no epic', 'd', 'd')`)
	exec1(t, db, `INSERT INTO task_links (from_task, to_task, type) VALUES (1, 2, 'relates')`)
	exec1(t, db, `INSERT INTO stories (epic, id, title) VALUES ('e', 'S1', 't')`)
	exec1(t, db, `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S1', 'd', 'IN-PROGRESS')`)
	exec1(t, db, `INSERT INTO artifacts (class, scope, relpath) VALUES ('research', '', 'x.md')`)
	exec1(t, db, `INSERT INTO task_artifacts (task, artifact, role) VALUES (1, 1, 'home')`)
	exec1(t, db, `INSERT INTO epic_artifacts (epic, artifact, role) VALUES ('e', 1, 'home')`)
	exec1(t, db, `INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`)
}

func TestSerializer(t *testing.T) {
	t.Parallel()

	t.Run("dump-utf8-lf-trailing-newline", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		seed(t, db)
		out := serialize(t, db)
		if !utf8.ValidString(out) {
			t.Fatal("dump is not valid UTF-8")
		}
		if strings.Contains(out, "\r") {
			t.Fatal("dump contains a carriage return; LF is the contract")
		}
		if !strings.HasSuffix(out, "\n") {
			t.Fatal("dump lacks the trailing newline")
		}
	})

	t.Run("dump-single-header-comment-only", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		seed(t, db)
		lines := strings.Split(serialize(t, db), "\n")
		if !strings.HasPrefix(lines[0], "-- selftracked dump ") {
			t.Fatalf("first line %q is not the header", lines[0])
		}
		// Every later comment line must come from the DDL block, which is
		// emitted verbatim — no serializer banner beyond line one.
		ddlComments := map[string]bool{}
		for l := range strings.SplitSeq(schema.DDL(), "\n") {
			if strings.HasPrefix(l, "--") {
				ddlComments[l] = true
			}
		}
		for i, l := range lines[1:] {
			if strings.HasPrefix(l, "--") && !ddlComments[l] {
				t.Fatalf("line %d is a banner outside the DDL block: %q", i+2, l)
			}
		}
	})

	t.Run("header-carries-version-and-id-highwater-never-seq", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		seed(t, db)
		before, _, _ := strings.Cut(serialize(t, db), "\n")
		if !strings.Contains(before, "schema_version=1") ||
			!strings.Contains(before, "tasks=2") || !strings.Contains(before, "artifacts=1") {
			t.Fatalf("header %q lacks version or high-water fields", before)
		}
		// Bump ONLY events and worklog seq: the header must not move.
		exec1(t, db, `INSERT INTO events (at, entity, event) VALUES ('d', '#2', 'create')`)
		exec1(t, db, `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 2, 'S1', 'd', 'DONE')`)
		after, _, _ := strings.Cut(serialize(t, db), "\n")
		if before != after {
			t.Fatalf("header changed on a seq-only write:\n%q\n%q", before, after)
		}
	})

	t.Run("dump-ddl-block-byte-equals-compiled-ddl", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		out := serialize(t, db)
		if !strings.Contains(out, schema.DDL()) {
			t.Fatal("the compiled-in DDL does not appear verbatim in the dump")
		}
	})

	t.Run("dump-table-order-fixed-events-last", func(t *testing.T) {
		t.Parallel()
		// Insert in a scrambled order; the dump's table order must not care.
		db := fresh(t)
		exec1(t, db, `INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`)
		exec1(t, db, `INSERT INTO epics (slug, goal, created_at) VALUES ('e', 'g', 'd')`)
		exec1(t, db, `INSERT INTO tasks (title, created_at, updated_at) VALUES ('t', 'd', 'd')`)
		out := serialize(t, db)
		iEpics := strings.Index(out, "INSERT INTO epics ")
		iTasks := strings.Index(out, "INSERT INTO tasks ")
		iEvents := strings.Index(out, "INSERT INTO events ")
		if iEpics == -1 || iTasks == -1 || iEvents == -1 {
			t.Fatal("expected inserts missing")
		}
		if iEpics >= iTasks || iTasks >= iEvents {
			t.Fatalf("table order broken: epics@%d tasks@%d events@%d", iEpics, iTasks, iEvents)
		}
		if last := strings.LastIndex(out, "INSERT INTO "); !strings.HasPrefix(out[last:], "INSERT INTO events ") {
			t.Fatalf("the last INSERT is not events: %q", out[last:last+40])
		}
	})

	t.Run("dump-rows-ordered-by-full-pk-events-bounded", func(t *testing.T) {
		t.Parallel()
		// Explicit ids inserted high-first; the dump must sort by PK.
		db := fresh(t)
		exec1(t, db, `INSERT INTO tasks (id, title, created_at, updated_at) VALUES (5, 'five', 'd', 'd')`)
		exec1(t, db, `INSERT INTO tasks (id, title, created_at, updated_at) VALUES (2, 'two', 'd', 'd')`)
		out := serialize(t, db)
		if strings.Index(out, "'two'") > strings.Index(out, "'five'") {
			t.Fatal("tasks are not in PK order")
		}
	})

	t.Run("dump-every-insert-names-columns", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		seed(t, db)
		for line := range strings.SplitSeq(serialize(t, db), "\n") {
			if strings.HasPrefix(line, "INSERT INTO ") && !strings.Contains(line, ") VALUES (") {
				t.Fatalf("INSERT without a column list: %q", line)
			}
			if strings.HasPrefix(line, "INSERT INTO ") &&
				!strings.Contains(line[len("INSERT INTO "):], "(") {
				t.Fatalf("INSERT without columns: %q", line)
			}
		}
	})

	t.Run("dump-apostrophe-doubles-no-backslash", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		exec1(t, db, `INSERT INTO tasks (title, created_at, updated_at) VALUES ('it''s got ''quotes''', 'd', 'd')`)
		out := serialize(t, db)
		if !strings.Contains(out, `'it''s got ''quotes'''`) {
			t.Fatal("apostrophes are not doubled in the dump")
		}
		if strings.Contains(out, `\'`) {
			t.Fatal("a backslash escape appeared; quote doubling is the only mechanism")
		}
	})

	t.Run("dump-null-only-in-the-three-columns", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		seed(t, db)
		// worklog.corrects NULL rides in seed's row; tasks 'no epic' carries
		// NULL dup_of and epic. Scan every INSERT: NULL may appear only in
		// tasks and worklog lines (their nullable columns).
		for line := range strings.SplitSeq(serialize(t, db), "\n") {
			if !strings.HasPrefix(line, "INSERT INTO ") || !strings.Contains(line, "NULL") {
				continue
			}
			if !strings.HasPrefix(line, "INSERT INTO tasks ") && !strings.HasPrefix(line, "INSERT INTO worklog ") {
				t.Fatalf("NULL outside tasks/worklog: %q", line)
			}
		}
		// And the permitted ones DO serialize as NULL, not as ''.
		//nolint:dupword // two adjacent NULL columns is the point
		if !strings.Contains(serialize(t, db), "NULL, NULL, 'd', 'd');") {
			t.Fatal("the no-epic task's dup_of/epic did not serialize as NULL")
		}
	})

	t.Run("dump-contains-no-pragma", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		seed(t, db)
		for line := range strings.SplitSeq(serialize(t, db), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "PRAGMA") {
				t.Fatalf("a PRAGMA reached the dump: %q", line)
			}
		}
	})

	t.Run("dump-never-contains-active_verb-key", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		// Leak the key the way only a bug could. The DDL comments mention
		// the key by name, so the assertion looks at data lines only.
		exec1(t, db, `INSERT INTO meta (key, value) VALUES ('active_verb', 'epic-close')`)
		for line := range strings.SplitSeq(serialize(t, db), "\n") {
			if strings.HasPrefix(line, "INSERT INTO meta ") && strings.Contains(line, "active_verb") {
				t.Fatalf("active_verb reached the dump: %q", line)
			}
		}
	})

	t.Run("full-new-value-recoverable-from-entity-row-and-dump", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		long := strings.Repeat("the whole new value ", 50)
		exec1(t, db, `INSERT INTO tasks (title, created_at, updated_at) VALUES (?, 'd', 'd')`, long)
		if !strings.Contains(serialize(t, db), long) {
			t.Fatal("the full entity value is not recoverable from the dump")
		}
	})

	t.Run("long-note-one-line-untruncated", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		exec1(t, db, `INSERT INTO epics (slug, goal, created_at) VALUES ('e', 'g', 'd')`)
		essay := strings.Repeat("an essay-sized episode note without a single newline ", 200)
		exec1(t, db, `INSERT INTO worklog (epic, seq, story, date, state, note) VALUES ('e', 1, 'S1', 'd', 'DONE', ?)`, essay)
		var found bool
		for line := range strings.SplitSeq(serialize(t, db), "\n") {
			if strings.Contains(line, essay) {
				found = true // the whole essay on one line, unwrapped
			}
		}
		if !found {
			t.Fatal("the essay note was truncated or wrapped across lines")
		}
	})

	t.Run("boundary-above-zero-omits-archived-events", func(t *testing.T) {
		t.Parallel()
		// Test-harness only: no v0 verb can write the key (R9 guards that);
		// the serializer must already honor it so activating `events
		// archive` later changes no dump grammar.
		db := fresh(t)
		for range 5 {
			exec1(t, db, `INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`)
		}
		exec1(t, db, `UPDATE meta SET value = '3' WHERE key = 'events_archived_through'`)
		out := serialize(t, db)
		if strings.Contains(out, "\nINSERT INTO events (seq, at, entity, event, detail) VALUES (3,") {
			t.Fatal("an archived event (seq <= boundary) was serialized")
		}
		if !strings.Contains(out, "\nINSERT INTO events (seq, at, entity, event, detail) VALUES (4,") {
			t.Fatal("a live event above the boundary is missing")
		}
	})

	t.Run("absent-boundary-row-treated-as-zero", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		exec1(t, db, `INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`)
		// meta has no delete trigger: removing the row simulates the
		// damaged state the rule exists for.
		exec1(t, db, `DELETE FROM meta WHERE key = 'events_archived_through'`)
		if !strings.Contains(serialize(t, db), "INSERT INTO events ") {
			t.Fatal("a missing boundary row dropped events; absent must mean 0")
		}
	})

	t.Run("dump-has-no-sqlite_sequence-inserts", func(t *testing.T) {
		t.Parallel()
		db := fresh(t)
		seed(t, db)
		if strings.Contains(serialize(t, db), "sqlite_sequence") {
			t.Fatal("sqlite_sequence data reached the dump")
		}
	})

	t.Run("determinism-across-runs-and-insert-order", func(t *testing.T) {
		t.Parallel()
		// INV-494/507's local half: the same logical content serializes to
		// the same bytes regardless of arrival order and run count. The
		// cross-OS matrix is the first push's CI gate.
		one := fresh(t)
		exec1(t, one, `INSERT INTO epics (slug, goal, created_at) VALUES ('a', 'g', 'd')`)
		exec1(t, one, `INSERT INTO epics (slug, goal, created_at) VALUES ('b', 'g', 'd')`)
		two := fresh(t)
		exec1(t, two, `INSERT INTO epics (slug, goal, created_at) VALUES ('b', 'g', 'd')`)
		exec1(t, two, `INSERT INTO epics (slug, goal, created_at) VALUES ('a', 'g', 'd')`)
		first, second := serialize(t, one), serialize(t, two)
		if first != second {
			t.Fatal("insert order leaked into the bytes")
		}
		again := serialize(t, one)
		if first != again {
			t.Fatal("two runs over one database differ")
		}
	})
}
