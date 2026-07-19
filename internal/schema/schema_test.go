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

	declared := schema.Objects()
	if len(declared) == 0 {
		t.Fatal("the DDL declares no objects; the parser or the file is wrong")
	}
	for _, name := range declared {
		if !live[name] {
			t.Errorf("the DDL declares %q but a fresh database does not have it", name)
		}
	}
}

func TestAFreshDatabaseHasNothingTheDDLDidNotAskFor(t *testing.T) {
	t.Parallel()

	live := liveObjects(t, fresh(t))

	declared := map[string]bool{}
	for _, n := range schema.Objects() {
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
		t.Errorf("a fresh database has %q, which the DDL does not declare", name)
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
