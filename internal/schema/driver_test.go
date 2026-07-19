// S1c: driver behaviour — the §16 re-verification items the specification
// refuses to assume, plus the enforcement tiers §1.1 states, each proven
// against the pinned driver rather than trusted from the research document.
// Subtest names are the fixture slugs of docs/stage-openings/s1c.md. Four
// probes assert that a write SUCCEEDS: they demonstrate a stated limitation
// (a bypass or tier boundary the spec documents), and preventing them is
// explicitly not the schema's contract.

package schema_test

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Spoloborota/selftracked/internal/schema"
	sqlite "modernc.org/sqlite"
)

// serializer is the driver-conn surface INV-502 needs; modernc.org/sqlite
// implements it on its unexported conn type, reachable through Raw.
type serializer interface {
	Serialize() ([]byte, error)
	Deserialize(image []byte) error
}

// errNoSerializeAPI reports a driver conn without the Serialize surface —
// the fail-closed path S1c's DoD demands of a probe that cannot run.
var errNoSerializeAPI = errors.New("driver connection does not expose Serialize/Deserialize")

func rawDriverConn(t *testing.T, db *sql.DB, f func(s serializer) error) {
	t.Helper()
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.Raw(func(dc any) error {
		s, ok := dc.(serializer)
		if !ok {
			return errNoSerializeAPI
		}
		return f(s)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDriver(t *testing.T) {
	t.Parallel()

	t.Run("trigger-bypass-vectors-reproducible", func(t *testing.T) {
		t.Parallel()
		// INV-011: both writes must SUCCEED — the limitation is the
		// obligation. Vector (a): the INSERT path carries no transition
		// trigger, so a terminal state lands without ever passing the
		// matrix. Vector (b): a session that unsets recursive_triggers
		// makes INSERT OR REPLACE delete through the no-delete trigger.
		db := fresh(t)
		exec1(t, db, `INSERT INTO tasks (title, status, status_note, created_at, updated_at)
		              VALUES ('t', 'DONE', 'n', 'd', 'd')`)

		conn, err := db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		if _, err := conn.ExecContext(t.Context(), `PRAGMA recursive_triggers = OFF`); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(t.Context(),
			`INSERT OR REPLACE INTO tasks (id, title, status, status_note, created_at, updated_at)
			 VALUES (1, 'replaced', 'DONE', 'n', 'd', 'd')`); err != nil {
			t.Fatalf("REPLACE with recursive_triggers off should bypass the delete trigger: %v", err)
		}
		var title string
		if err := conn.QueryRowContext(t.Context(), `SELECT title FROM tasks WHERE id = 1`).Scan(&title); err != nil {
			t.Fatal(err)
		}
		if title != "replaced" {
			t.Fatalf("title = %q; the REPLACE did not land", title)
		}
	})

	t.Run("recursive-triggers-replace-regression", func(t *testing.T) {
		t.Parallel()
		// INV-505: on the DSN's own connections recursive_triggers is ON,
		// so REPLACE reaches the BEFORE DELETE trigger and is refused —
		// the research doc's regression, re-proven on the pinned driver.
		db := fresh(t)
		exec1(t, db, `INSERT INTO tasks (title, created_at, updated_at) VALUES ('t', 'd', 'd')`)
		refuse(t, db, "never deleted",
			`INSERT OR REPLACE INTO tasks (id, title, created_at, updated_at) VALUES (1, 'x', 'd', 'd')`)
	})

	t.Run("raw-update-leaves-stale-note-passes-trigger", func(t *testing.T) {
		t.Parallel()
		// INV-168: the IN-REVIEW exit trigger checks non-emptiness only. A
		// raw UPDATE that leaves the PREVIOUS verdict in the column passes;
		// only the verb path always rewrites it. The write must SUCCEED.
		db := fresh(t)
		exec1(t, db, `INSERT INTO tasks (title, status, status_note, created_at, updated_at)
		              VALUES ('t', 'IN-REVIEW', 'stale verdict from last round', 'd', 'd')`)
		exec1(t, db, `UPDATE tasks SET status = 'OPEN' WHERE id = 1`)
		var note string
		if err := db.QueryRowContext(t.Context(), `SELECT status_note FROM tasks WHERE id = 1`).Scan(&note); err != nil {
			t.Fatal(err)
		}
		if note != "stale verdict from last round" {
			t.Fatalf("status_note = %q; the stale verdict should have survived", note)
		}
	})

	t.Run("fk-violations-pass-when-pragma-foreign_keys-off", func(t *testing.T) {
		t.Parallel()
		// INV-170: referential integrity binds only connections with the
		// pragma on. A pragma-free connection writes orphans freely — the
		// tier boundary §1.1 states; verify's foreign_key_check (S7) is
		// the compensating detection.
		db, dir := freshAt(t)
		_ = db
		raw := rawConn(t, dir)
		exec1(t, raw, `INSERT INTO tasks (title, epic, created_at, updated_at) VALUES ('t', 'ghost', 'd', 'd')`)
	})

	t.Run("deliberate-raw-insert-bypasses-history-invariants", func(t *testing.T) {
		t.Parallel()
		// INV-173: a deliberate INSERT-path writer plants a terminal state
		// with no events trail — triggers do not and cannot stop it; R12
		// (S7) is the detection. The write must SUCCEED and leave no trail.
		db := fresh(t)
		exec1(t, db, `INSERT INTO tasks (title, status, status_note, created_at, updated_at)
		              VALUES ('forged', 'DONE', 'n', 'd', 'd')`)
		var events int
		if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM events`).Scan(&events); err != nil {
			t.Fatal(err)
		}
		if events != 0 {
			t.Fatalf("events rows = %d; the forgery should leave no trail for R12 to be needed against", events)
		}
	})

	t.Run("dqs-off-rejects-double-quoted-string", func(t *testing.T) {
		t.Parallel()
		// INV-028's fifth setting: _dqs=0 is a connection parameter, not a
		// PRAGMA, so S1a's pragma assertions cannot see it. Behavioural
		// probe: with DQS off, a double-quoted non-identifier is an error,
		// never a silent string literal.
		db := fresh(t)
		refuse(t, db, "no such column", `INSERT INTO tasks (title, created_at, updated_at) VALUES ("not a column", 'd', 'd')`)
	})

	t.Run("returning-via-query-path", func(t *testing.T) {
		t.Parallel()
		// INV-506: RETURNING through QueryRow yields the inserted id on
		// the pinned driver (the ≥v1.48.2 floor exists because Exec used
		// to lose the row; the spec routes all RETURNING through Query).
		db := fresh(t)
		var id int64
		err := db.QueryRowContext(t.Context(),
			`INSERT INTO tasks (title, created_at, updated_at) VALUES ('t', 'd', 'd') RETURNING id`).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		if id != 1 {
			t.Fatalf("RETURNING id = %d on a fresh table; want 1", id)
		}
	})

	t.Run("extended-result-codes-distinguishable", func(t *testing.T) {
		t.Parallel()
		// INV-504: the §6.1 exit-code mapper needs to tell constraint
		// families apart; primary codes collapse them all into
		// SQLITE_CONSTRAINT (19). Prove the driver surfaces EXTENDED codes:
		// FK and CHECK violations must carry distinct codes above the
		// primary range.
		db := fresh(t)
		codeOf := func(query string) int {
			t.Helper()
			_, err := db.ExecContext(t.Context(), query)
			if err == nil {
				t.Fatalf("%q succeeded; want a constraint violation", query)
			}
			var se *sqlite.Error
			if !errors.As(err, &se) {
				t.Fatalf("%q returned %T, not *sqlite.Error: %v", query, err, err)
			}
			return se.Code()
		}
		fk := codeOf(`INSERT INTO tasks (title, epic, created_at, updated_at) VALUES ('t', 'ghost', 'd', 'd')`)
		check := codeOf(`INSERT INTO tasks (title, created_at, updated_at) VALUES ('', 'd', 'd')`)
		const primaryConstraint = 19
		if fk == check {
			t.Fatalf("FK and CHECK violations share code %d; the exit mapper cannot tell them apart", fk)
		}
		if fk <= primaryConstraint || check <= primaryConstraint {
			t.Fatalf("codes fk=%d check=%d; want extended codes above the primary SQLITE_CONSTRAINT (19)", fk, check)
		}
	})

	t.Run("serialize-deserialize-roundtrip-full-schema", func(t *testing.T) {
		t.Parallel()
		// INV-502: the dump↔DB gate (S3/S4) will lean on Serialize and
		// Deserialize working against the FULL schema — triggers, STRICT,
		// WITHOUT ROWID, the partial index. Populate, serialize,
		// deserialize into a second database, and require behavioural and
		// byte identity.
		src := fresh(t)
		addEpic(t, src, "e", "ACTIVE")
		addStory(t, src, "IN-PROGRESS")
		addTask(t, src, "OPEN", nil)
		exec1(t, src, `INSERT INTO worklog (epic, seq, story, date, state) VALUES ('e', 1, 'S1', 'd', 'IN-PROGRESS')`)
		exec1(t, src, `INSERT INTO events (at, entity, event) VALUES ('d', '#1', 'create')`)

		var image []byte
		rawDriverConn(t, src, func(s serializer) error {
			var err error
			if image, err = s.Serialize(); err != nil {
				return fmt.Errorf("serialize: %w", err)
			}
			return nil
		})
		if len(image) == 0 {
			t.Fatal("Serialize returned an empty image")
		}

		dst, err := schema.Open(filepath.Join(t.TempDir(), "dst.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = dst.Close() })
		// Deserialize must happen and be USED on one pinned connection:
		// the restored image lives on that connection's main database.
		conn, err := dst.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		if err := conn.Raw(func(dc any) error {
			s, ok := dc.(serializer)
			if !ok {
				return errNoSerializeAPI
			}
			return s.Deserialize(image)
		}); err != nil {
			t.Fatal(err)
		}

		// Behavioural identity: the restored database carries the schema's
		// objects and the seeded rows, and its gates still bite.
		var stories, objects int
		if err := conn.QueryRowContext(t.Context(), `SELECT count(*) FROM stories`).Scan(&stories); err != nil {
			t.Fatal(err)
		}
		if stories != 1 {
			t.Fatalf("stories after roundtrip = %d; want 1", stories)
		}
		err = conn.QueryRowContext(t.Context(),
			`SELECT count(*) FROM sqlite_master WHERE type IN ('table','trigger','view','index')`).Scan(&objects)
		if err != nil {
			t.Fatal(err)
		}
		if objects == 0 {
			t.Fatal("no schema objects after roundtrip")
		}
		if _, err := conn.ExecContext(t.Context(), `DELETE FROM worklog`); err == nil {
			t.Fatal("the restored database lost its append-only trigger")
		} else if !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("restored trigger refused with %q; want the worklog message", err)
		}

		// Byte identity: serializing the restored image reproduces it.
		var again []byte
		if err := conn.Raw(func(dc any) error {
			s, ok := dc.(serializer)
			if !ok {
				return errNoSerializeAPI
			}
			var err error
			if again, err = s.Serialize(); err != nil {
				return fmt.Errorf("re-serialize: %w", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if string(again) != string(image) {
			t.Fatalf("re-serialized image differs: %d vs %d bytes", len(again), len(image))
		}
	})
}
