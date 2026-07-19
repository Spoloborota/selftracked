// Package schema owns the database's structure: the compiled-in DDL, the
// connection settings the specification requires, and the creation of a
// fresh database.
//
// The DDL is embedded rather than generated. It is the single source the
// serializer emits into every dump and the loader compares a dump against
// byte for byte, so it must be one artifact with one spelling, not a thing
// assembled differently in two places.
package schema

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // registers the CGO-free SQLite driver
)

//go:embed ddl.sql
var ddl string

// Version is the schema version this binary compiles in. A dump declaring a
// higher version is refused rather than guessed at.
const Version = 1

// stampSQL writes the file's identity and schema version. The application id
// spells "STRK", so a database opened by mistake is recognised as foreign
// rather than silently used. Both numbers are literal because PRAGMA takes no
// placeholders and a constant string cannot carry injection;
// TestStampMatchesTheConstants is what keeps the literal from going stale.
const stampSQL = "PRAGMA application_id = 1398035019; PRAGMA user_version = 1;"

// ErrNotEmpty is returned when Create is given a database that already
// carries schema objects — overwriting one silently would destroy data.
var ErrNotEmpty = errors.New("database already has schema objects")

// DDL returns the compiled-in schema definition.
func DDL() string { return ddl }

// basePragmas travel in the DSN so that every connection the pool opens
// carries them — including one opened long after the first. A connection
// that missed them enforces nothing, and SQLite defaults foreign keys to off.
//
// Journal mode is deliberately absent: the specification rules WAL out (it is
// a persistent flag, it litters -wal/-shm files, and it does not work on
// network filesystems), so the default rollback journal stands.
const basePragmas = "?_pragma=foreign_keys(1)" +
	"&_pragma=recursive_triggers(1)" + // without it INSERT OR REPLACE bypasses delete triggers
	"&_pragma=trusted_schema(0)" +
	"&_pragma=busy_timeout(5000)" +
	"&_dqs=0" // a bare string is never silently a string literal

// readPragmas make a connection refuse to write at all, so a read verb that
// tries becomes an error rather than a surprise.
const readPragmas = "&_pragma=query_only(1)"

// writePragmas take the exclusive lock at the connection's first write and
// hold it until close, and make durability the priority over throughput.
const writePragmas = "&_pragma=locking_mode(EXCLUSIVE)" +
	"&_pragma=synchronous(FULL)"

// Open opens the database at path with the settings every connection needs.
// It does not create the schema; see Create. For verbs, prefer OpenRead or
// OpenWrite, which add the half the specification requires of each.
func Open(path string) (*sql.DB, error) { return open(path, "") }

// OpenRead opens a connection that cannot write.
func OpenRead(path string) (*sql.DB, error) { return open(path, readPragmas) }

// OpenWrite opens a connection that takes the exclusive lock on its first
// write and syncs fully.
func OpenWrite(path string) (*sql.DB, error) { return open(path, writePragmas) }

func open(path, extra string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+basePragmas+extra)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, nil
}

// Create builds a fresh database: the schema, the file's identity, and the
// meta rows a new instance is born with. It is an error to call it on a
// database that already carries objects.
func Create(ctx context.Context, db *sql.DB) error {
	var count int
	row := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema`)
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("inspect an empty database: %w", err)
	}
	if count != 0 {
		return fmt.Errorf("%w: found %d; Create expects an empty database", ErrNotEmpty, count)
	}

	if _, err := db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("apply the schema: %w", err)
	}

	// PRAGMA takes no placeholders, so the statement is a literal rather than
	// something assembled at runtime: a constant string cannot carry
	// injection. TestStampMatchesTheConstants keeps the literal honest if
	// either constant ever moves.
	if _, err := db.ExecContext(ctx, stampSQL); err != nil {
		return fmt.Errorf("stamp the file identity: %w", err)
	}

	return seedMeta(ctx, db)
}

// seedRows are the meta rows a new database is born with: the system rows
// the machinery owns, and the configuration keys with their defaults.
var seedRows = [][2]string{
	{"schema_version", strconv.Itoa(Version)},
	{"events_archived_through", "0"},
	{"production_globs", "internal/** cmd/**"},
	{"idle_days", "14"},
	{"prime_cap", "20"},
}

func seedMeta(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin the seeding transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO meta (key, value) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare the meta insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, row := range seedRows {
		if _, err := stmt.ExecContext(ctx, row[0], row[1]); err != nil {
			return fmt.Errorf("seed meta row %q: %w", row[0], err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit the seeding transaction: %w", err)
	}
	return nil
}

// Objects reports every object the compiled-in DDL declares, by name. It
// reads the DDL text rather than a live database, so a test can ask whether
// a database matches the definition without trusting the thing it checks.
func Objects() []string {
	var names []string
	for line := range strings.SplitSeq(ddl, "\n") {
		line = strings.TrimSpace(line)
		for _, kind := range []string{
			"CREATE TABLE ", "CREATE TRIGGER ", "CREATE VIEW ",
			"CREATE INDEX ", "CREATE UNIQUE INDEX ",
		} {
			if rest, ok := strings.CutPrefix(line, kind); ok {
				name, _, _ := strings.Cut(rest, " ")
				names = append(names, strings.TrimSuffix(name, "("))
			}
		}
	}
	return names
}
