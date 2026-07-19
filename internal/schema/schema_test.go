package schema_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Spoloborota/selftracked/internal/schema"
)

// fresh builds a database in a temp directory and returns it ready to query.
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

// liveObjects reads the names a real database actually carries.
func liveObjects(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()

	rows, err := db.QueryContext(t.Context(),
		`SELECT name FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("read sqlite_schema: %v", err)
	}
	defer func() { _ = rows.Close() }()

	live := map[string]bool{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		live[name] = true
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("iterate: %v", err)
	}
	return live
}

func TestCreateBuildsEveryObjectTheDDLDeclares(t *testing.T) {
	t.Parallel()

	live := liveObjects(t, fresh(t))

	for _, name := range wantObjects {
		if !live[name] {
			t.Errorf("the schema must contain %q; a fresh database does not have it", name)
		}
	}

	// The DDL parser and the golden list must agree too, or Objects() could
	// drift away from the thing the serializer will rely on.
	declared := map[string]bool{}
	for _, n := range schema.Objects() {
		declared[n] = true
	}
	for _, name := range wantObjects {
		if !declared[name] {
			t.Errorf("Objects() does not report %q, which the schema must contain", name)
		}
	}
}

func TestAFreshDatabaseHasNothingTheDDLDidNotAskFor(t *testing.T) {
	t.Parallel()

	live := liveObjects(t, fresh(t))

	declared := map[string]bool{}
	for _, n := range wantObjects {
		declared[n] = true
	}

	var extra []string
	for name := range live {
		if !declared[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		t.Errorf("a fresh database has %q, which the schema does not declare", name)
	}
}

func TestFreshDatabaseCarriesItsIdentityAndVersion(t *testing.T) {
	t.Parallel()

	db := fresh(t)

	var appID, userVersion int
	if err := db.QueryRowContext(t.Context(), `PRAGMA application_id`).Scan(&appID); err != nil {
		t.Fatalf("read application_id: %v", err)
	}
	if err := db.QueryRowContext(t.Context(), `PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if appID == 0 {
		t.Error("application_id is unset; a foreign database would be indistinguishable from ours")
	}
	if userVersion != schema.Version {
		t.Errorf("user_version = %d, want %d", userVersion, schema.Version)
	}
}

func TestStampMatchesTheConstants(t *testing.T) {
	t.Parallel()

	// The stamp is a literal so that no SQL is assembled at runtime; this
	// test is what stops the literal and the constants from drifting apart.
	db := fresh(t)

	var appID, userVersion int
	if err := db.QueryRowContext(t.Context(), `PRAGMA application_id`).Scan(&appID); err != nil {
		t.Fatalf("read application_id: %v", err)
	}
	if err := db.QueryRowContext(t.Context(), `PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if want := 0x5354524B; appID != want {
		t.Errorf("application_id = %d, want %d — the stamp literal and the constant have drifted", appID, want)
	}
	if userVersion != schema.Version {
		t.Errorf("user_version = %d but schema.Version = %d — the stamp literal is stale", userVersion, schema.Version)
	}
}

func TestSeededMetaRowsArePresent(t *testing.T) {
	t.Parallel()

	db := fresh(t)

	for key, want := range map[string]string{
		"schema_version":          "1",
		"events_archived_through": "0",
		"idle_days":               "14",
		"prime_cap":               "20",
	} {
		var got string
		err := db.QueryRowContext(t.Context(),
			`SELECT value FROM meta WHERE key = ?`, key).Scan(&got)
		if err != nil {
			t.Errorf("meta[%q]: %v", key, err)
			continue
		}
		if got != want {
			t.Errorf("meta[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestForeignKeysAreOnForEveryPooledConnection(t *testing.T) {
	t.Parallel()

	db := fresh(t)

	// Ask several times: the setting travels in the DSN precisely so that a
	// connection the pool opens later is not born without it.
	for i := range 5 {
		var on int
		if err := db.QueryRowContext(t.Context(), `PRAGMA foreign_keys`).Scan(&on); err != nil {
			t.Fatalf("read foreign_keys: %v", err)
		}
		if on != 1 {
			t.Fatalf("connection %d has foreign_keys off", i)
		}
	}
}

func TestCreateRefusesADatabaseThatAlreadyHasObjects(t *testing.T) {
	t.Parallel()

	db := fresh(t)

	if err := schema.Create(t.Context(), db); err == nil {
		t.Fatal("Create succeeded on an already-populated database; it must refuse")
	}
}

func TestOpenReportsAnUnusablePath(t *testing.T) {
	t.Parallel()

	db, err := schema.Open(filepath.Join(t.TempDir(), "no-such-dir", "db.sqlite"))
	if err == nil {
		defer func() { _ = db.Close() }()
		if err = db.PingContext(context.Background()); err == nil {
			t.Fatal("opening a database under a missing directory reported no error")
		}
	}
}

// pragmaOnSecondConnection forces the pool to hand out a connection other
// than the first, because the failure being guarded against is precisely a
// later connection born without the settings.
func pragmaOnSecondConnection(t *testing.T, db *sql.DB, pragma string) string {
	t.Helper()

	held, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("hold the first connection: %v", err)
	}
	defer func() { _ = held.Close() }()

	second, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("open a second connection: %v", err)
	}
	defer func() { _ = second.Close() }()

	var value string
	if err = second.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&value); err != nil {
		t.Fatalf("read PRAGMA %s: %v", pragma, err)
	}
	return value
}

func TestEveryConnectionCarriesTheRequiredPragmas(t *testing.T) {
	t.Parallel()

	db := fresh(t)

	for pragma, want := range map[string]string{
		"foreign_keys":       "1",
		"recursive_triggers": "1",
		"trusted_schema":     "0",
		"busy_timeout":       "5000",
	} {
		if got := pragmaOnSecondConnection(t, db, pragma); got != want {
			t.Errorf("second pooled connection: %s = %q, want %q", pragma, got, want)
		}
	}
}

func TestJournalModeIsNotWAL(t *testing.T) {
	t.Parallel()

	// WAL is ruled out deliberately: it is a persistent flag, it leaves -wal
	// and -shm files beside the database, and it does not work on network
	// filesystems. Setting it would be silent and hard to undo.
	if got := pragmaOnSecondConnection(t, fresh(t), "journal_mode"); got == "wal" {
		t.Errorf("journal_mode = %q; the specification rules WAL out", got)
	}
}

func TestWriteConnectionsLockExclusivelyAndSyncFully(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "db.sqlite")
	db, err := schema.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err = schema.Create(t.Context(), db); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = db.Close()

	w, err := schema.OpenWrite(path)
	if err != nil {
		t.Fatalf("OpenWrite: %v", err)
	}
	defer func() { _ = w.Close() }()

	if got := pragmaOnSecondConnection(t, w, "locking_mode"); got != "exclusive" {
		t.Errorf("write connection: locking_mode = %q, want exclusive", got)
	}
	if got := pragmaOnSecondConnection(t, w, "synchronous"); got != "2" {
		t.Errorf("write connection: synchronous = %q, want 2 (FULL)", got)
	}
}

func TestReadConnectionsRefuseToWrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "db.sqlite")
	db, err := schema.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err = schema.Create(t.Context(), db); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = db.Close()

	r, err := schema.OpenRead(path)
	if err != nil {
		t.Fatalf("OpenRead: %v", err)
	}
	defer func() { _ = r.Close() }()

	_, err = r.ExecContext(t.Context(),
		`INSERT INTO meta (key, value) VALUES ('x', 'y')`)
	if err == nil {
		t.Fatal("a read connection accepted a write; query_only is not in force")
	}
}

func TestStrictTablesRejectAWrongTypedValue(t *testing.T) {
	t.Parallel()

	db := fresh(t)

	// STRICT binds any process, not just the verbs — that is the point of
	// putting the constraint in the schema rather than in the code above it.
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO tasks (id, title, status, created_at, updated_at)
		 VALUES ('not-an-integer', 't', 'OPEN', 'now', 'now')`)
	if err == nil {
		t.Fatal("a text value was accepted into an INTEGER column; STRICT is not in force")
	}
}

func TestCloseStampIsAnAccidentGuardNotAnAdversaryGuard(t *testing.T) {
	t.Parallel()

	db := fresh(t)
	ctx := t.Context()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO epics (slug, goal, created_at)
		 VALUES ('e', 'a goal', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed an epic: %v", err)
	}

	// An epic that never ran cannot be closed — the matrix says dissolve it
	// instead — so activate it first; that transition is legal and needs no
	// marker, which is itself worth seeing.
	if _, err := db.ExecContext(ctx,
		`UPDATE epics SET status = 'ACTIVE' WHERE slug = 'e'`); err != nil {
		t.Fatalf("activate the epic: %v", err)
	}

	// Closing means status and stamp together — the table's own CHECK ties
	// them — so the update below is the shape the verb would write, and the
	// only thing standing between it and success is the close gate.
	const closeIt = `UPDATE epics
	                 SET status = 'CLOSED', close_sweep = '2026-01-01'
	                 WHERE slug = 'e'`

	// The accident it stops: closing without going through the verb.
	_, err := db.ExecContext(ctx, closeIt)
	if err == nil {
		t.Fatal("an epic was closed without the verb; the close gate did not fire")
	}

	// The adversary it does not stop, stated honestly: a writer that sets the
	// same marker the verb sets walks straight through. Detection of that is
	// R12's job, not this trigger's.
	if _, err = db.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES ('active_verb', 'epic-close')`); err != nil {
		t.Fatalf("set the verb marker: %v", err)
	}
	if _, err = db.ExecContext(ctx, closeIt); err != nil {
		t.Fatalf("the gate blocked a writer holding the marker, which it cannot do: %v", err)
	}
}

func TestNoColumnStoresNullOutsideTheThreeThatMayNot(t *testing.T) {
	t.Parallel()

	db := fresh(t)
	ctx := t.Context()

	// The claim is about stored values, not column declarations, so the check
	// reads rows rather than metadata — which is why it needs no exception
	// list for the rowid aliases that accept NULL on the way in.
	allowed := map[string]bool{
		"tasks.dup_of":     true,
		"tasks.epic":       true,
		"worklog.corrects": true,
	}

	tables, err := db.QueryContext(ctx,
		`SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer func() { _ = tables.Close() }()

	var names []string
	for tables.Next() {
		var name string
		if err = tables.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		names = append(names, name)
	}
	if err = tables.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}

	for _, table := range names {
		for _, col := range nullableColumns(t, db, table) {
			qualified := table + "." + col
			if allowed[qualified] {
				continue
			}
			// The column may accept NULL; the claim is that no row carries one.
			var stored int
			q := `SELECT count(*) FROM "` + table + `" WHERE "` + col + `" IS NULL`
			if err = db.QueryRowContext(ctx, q).Scan(&stored); err != nil {
				t.Errorf("count nulls in %s: %v", qualified, err)
				continue
			}
			if stored != 0 {
				t.Errorf("%s stores %d NULLs; only tasks.dup_of, tasks.epic and worklog.corrects may", qualified, stored)
			}
		}
	}
}

// nullableColumns reports the columns of one table that accept NULL.
func nullableColumns(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()

	rows, err := db.QueryContext(t.Context(),
		`SELECT name FROM pragma_table_info(?) WHERE "notnull" = 0`, table)
	if err != nil {
		t.Fatalf("read %s columns: %v", table, err)
	}
	defer func() { _ = rows.Close() }()

	var cols []string
	for rows.Next() {
		var col string
		if err = rows.Scan(&col); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		cols = append(cols, col)
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("iterate %s columns: %v", table, err)
	}
	return cols
}
