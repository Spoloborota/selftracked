package load

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	sqlite "modernc.org/sqlite"

	"github.com/Spoloborota/selftracked/internal/rules"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// Build constructs a fresh database from a parsed dump in a temp file
// inside dir and returns its path. The §8.5 sequence: hardened
// connection → DDL → parameterized inserts (literals never spliced back
// into SQL) → required-meta check → version stamp → load guard → Stage-0
// checks plus the DB-only §7 rules → close. The caller renames — an
// interrupted build never lands.
func Build(ctx context.Context, dir string, d *Dump) (string, error) {
	tmp, err := os.CreateTemp(dir, "db.sqlite.load-*")
	if err != nil {
		return "", fmt.Errorf("load: temp file: %w", err)
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("load: temp file: %w", err)
	}
	// CreateTemp made an empty file; SQLite wants to create its own.
	_ = os.Remove(path)

	db, err := openHardened(path)
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := build(ctx, db, d); err != nil {
		_ = db.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := db.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("load: close built db: %w", err)
	}
	return path, nil
}

// openHardened opens the build connection with §8.5's posture — the
// schema package owns the DSN grammar, so the hardening pragmas live
// there (OpenLoadBuild) and sqlite3_limit caps are applied per connection
// here.
//
// SQLITE_DBCONFIG_DEFENSIVE is deliberately NOT set: it is unreachable
// through modernc.org/sqlite (its only sqlite3_db_config binding is an
// unexported method on an unexported type — verified against v1.54.0), so
// the whitelist parser, not a defensive-mode flag, is the primary defense
// against a malicious dump (§8.5).
func openHardened(path string) (*sql.DB, error) {
	db, err := schema.OpenLoadBuild(path)
	if err != nil {
		return nil, fmt.Errorf("load: open build db: %w", err)
	}
	return db, nil
}

// sqlite3_limit ids (sqlite3.h). Set defensively low-ish: dump values are
// bounded strings and integers, never megabyte SQL statements.
const (
	limitLength    = 0       // SQLITE_LIMIT_LENGTH
	limitSQLLength = 1       // SQLITE_LIMIT_SQL_LENGTH
	maxValueBytes  = 1 << 26 // 64 MiB per value: essay notes fit, bombs do not
	maxSQLBytes    = 1 << 20 // our own prepared statements are tiny
)

func applyLimits(ctx context.Context, db *sql.DB) (*sql.Conn, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("load: pin connection: %w", err)
	}
	for _, l := range []struct{ id, val int }{
		{limitLength, maxValueBytes},
		{limitSQLLength, maxSQLBytes},
	} {
		if _, err := sqlite.Limit(conn, l.id, l.val); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("load: set limit %d: %w", l.id, err)
		}
	}
	return conn, nil
}

func build(ctx context.Context, db *sql.DB, d *Dump) error {
	conn, err := applyLimits(ctx, db)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, schema.DDL()); err != nil {
		return fmt.Errorf("load: apply DDL: %w", err)
	}
	for _, ins := range d.Inserts {
		// Defense in depth: Parse already rejects unknown tables, but Build
		// must not trust a *Dump it did not parse (a future rebuild path
		// could hand-build one). An unknown table has no canonical columns.
		cols, known := expectedColumns[ins.Table]
		if !known {
			return fmt.Errorf("%w: unknown table %q", ErrRefused, ins.Table)
		}
		params := strings.TrimSuffix(strings.Repeat("?, ", len(ins.Values)), ", ")
		//nolint:gosec // table and column names come from the compiled-in whitelist, values ride as parameters
		q := "INSERT INTO " + ins.Table + " (" + cols + ") VALUES (" + params + ")"
		args := make([]any, len(ins.Values))
		for i, v := range ins.Values {
			args[i] = v
		}
		if _, err := conn.ExecContext(ctx, q, args...); err != nil {
			//nolint:errorlint // the family is ErrRefused; the driver error is context
			return fmt.Errorf("%w: %s row: %v", ErrRefused, ins.Table, err)
		}
	}

	if err := requireMetaRows(ctx, conn, d.Version); err != nil {
		return err
	}
	// The dump carries no PRAGMAs, and a freshly built DB defaults to
	// user_version 0 — stamp both identity values from the meta row's
	// version so the next verb does not spuriously re-migrate (§8.5).
	stamp := fmt.Sprintf("PRAGMA application_id = %d; PRAGMA user_version = %d;",
		schema.ApplicationID, d.Version)
	if _, err := conn.ExecContext(ctx, stamp); err != nil {
		return fmt.Errorf("load: stamp identity: %w", err)
	}
	if err := seedEventsHighWater(ctx, conn); err != nil {
		return err
	}
	return finalChecks(ctx, db, conn)
}

// requireMetaRows: a missing row passes the grammar whitelist, so §8.5
// demands an explicit check, and the schema_version row must agree with
// the header that steered the parser.
func requireMetaRows(ctx context.Context, conn *sql.Conn, headerVersion int) error {
	var version string
	err := conn.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: meta lacks the schema_version row", ErrRefused)
	}
	if err != nil {
		return fmt.Errorf("load: read meta: %w", err)
	}
	if version != strconv.Itoa(headerVersion) {
		return fmt.Errorf("%w: header schema_version=%d but meta row says %s", ErrRefused, headerVersion, version)
	}
	var one int
	err = conn.QueryRowContext(ctx, `SELECT 1 FROM meta WHERE key = 'events_archived_through'`).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: meta lacks the events_archived_through row", ErrRefused)
	}
	if err != nil {
		return fmt.Errorf("load: read meta: %w", err)
	}
	return nil
}

// seedEventsHighWater applies the §8.2 load guard: the events
// AUTOINCREMENT high-water becomes max(boundary, MAX(live seq)), covering
// a dump with a gap above the boundary and one whose events were all
// archived — zero live rows give the explicit-id mechanism nothing, and a
// fresh clone would otherwise reuse archived seq numbers.
func seedEventsHighWater(ctx context.Context, conn *sql.Conn) error {
	var want int64
	err := conn.QueryRowContext(ctx, `
		SELECT MAX(
			COALESCE((SELECT CAST(value AS INTEGER) FROM meta WHERE key = 'events_archived_through'), 0),
			COALESCE((SELECT MAX(seq) FROM events), 0))`).Scan(&want)
	if err != nil {
		return fmt.Errorf("load guard: %w", err)
	}
	if want == 0 {
		return nil
	}
	res, err := conn.ExecContext(ctx, `UPDATE sqlite_sequence SET seq = ? WHERE name = 'events' AND seq < ?`, want, want)
	if err != nil {
		return fmt.Errorf("load guard: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// No sequence row yet (no events loaded): create it.
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO sqlite_sequence (name, seq)
			 SELECT 'events', ? WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'events')`, want); err != nil {
			return fmt.Errorf("load guard: %w", err)
		}
	}
	return nil
}

// finalChecks is §8.5 step 3's tail: Stage-0 (integrity_check plus
// foreign_key_check — the former does not cover FKs) plus the DB-only §7
// rules, so a boundary forgery or trail-less terminal state never
// survives to the rename.
func finalChecks(ctx context.Context, db *sql.DB, conn *sql.Conn) error {
	var verdict string
	if err := conn.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&verdict); err != nil {
		return fmt.Errorf("load: integrity_check: %w", err)
	}
	if verdict != "ok" {
		return fmt.Errorf("%w: integrity_check: %s", ErrRefused, verdict)
	}
	fkRows, err := conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("load: foreign_key_check: %w", err)
	}
	defer func() { _ = fkRows.Close() }()
	if fkRows.Next() {
		return fmt.Errorf("%w: foreign_key_check found violations", ErrRefused)
	}
	if err := fkRows.Err(); err != nil {
		return fmt.Errorf("load: foreign_key_check: %w", err)
	}
	violations, err := rules.DBOnly(ctx, db)
	if err != nil {
		return fmt.Errorf("load: rules: %w", err)
	}
	if len(violations) > 0 {
		v := violations[0]
		return fmt.Errorf("%w: %s: %s (%d violation(s) total)", ErrRefused, v.Rule, v.Message, len(violations))
	}
	return nil
}
