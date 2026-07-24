// Package dump is the §8.1 canonical serializer and the `dump` verb: one
// deterministic SQL text rendering of the database, byte-stable across
// runs, machines and operating systems, because the committed dump is the
// review surface and the only sync channel. `sqlite3 .dump` was
// disqualified with reasons (research doc); this serializer owns table
// order, row order, column lists, and the one escaping rule.
package dump

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Spoloborota/selftracked/internal/schema"
)

// table describes one serialized table: its explicit column list (§8.1:
// every INSERT names its columns) and the full-PK ORDER BY that fixes row
// order. The slice order below IS the dump's table order — parents before
// children so S4's loader can insert in file order with FKs on, events
// last so the append-only tail is the diff's tail.
type table struct {
	name string
	// columns is the space-separated explicit column list; orderBy is the
	// full-PK ordering. Space-separated so the declaration reads like the
	// schema and the strings stay single-occurrence.
	columns string
	orderBy string
	// where filters rows; only meta (the active_verb exclusion) and
	// events (the archive boundary) use it.
	where string
}

var tables = []table{
	// The serializer excludes active_verb defensively: it is transaction-
	// internal state (§5.1), and even a bug that leaked it must not let it
	// reach a dump.
	{
		name: "meta", columns: "key value", orderBy: "key",
		where: "key <> 'active_verb'",
	},
	{
		name: "path_dictionary", columns: "class scope root ephemeral note",
		orderBy: "class, scope",
	},
	{
		name: "epics", columns: "slug goal status status_note close_sweep created_at",
		orderBy: "slug",
	},
	{
		name: "epic_criteria", columns: "epic seq criterion met evidence",
		orderBy: "epic, seq",
	},
	{
		name: "tasks", columns: "id title status status_note parked dup_of epic created_at updated_at",
		orderBy: "id",
	},
	{
		name: "task_links", columns: "from_task to_task type note",
		orderBy: "from_task, to_task, type",
	},
	{
		name: "stories", columns: "epic id title status dod consumes produces blocked",
		orderBy: "epic, id",
	},
	{
		name: "worklog", columns: "epic seq story date state commits gate review corrects note",
		orderBy: "epic, seq",
	},
	{
		name: "artifacts", columns: "id class scope relpath archived note",
		orderBy: "id",
	},
	{
		name: "task_artifacts", columns: "task artifact role note",
		orderBy: "task, artifact",
	},
	{
		name: "epic_artifacts", columns: "epic artifact role note",
		orderBy: "epic, artifact",
	},
	// events last, bounded below by the archive boundary (§8.2), so
	// activating `events archive` later changes no dump grammar.
	{
		name: "events", columns: "seq at entity event detail",
		orderBy: "seq", where: "seq > ?",
	},
}

// HeaderVersion extracts the schema version from a dump's header line —
// the §8.1 format this package itself writes, parsed strictly: the exact
// prefix, a schema_version field, an integer ≥ 1. One implementation for
// every reader (the §8.6 gate, prime's report, the load preamble), so no
// two surfaces can disagree about what counts as a header.
func HeaderVersion(header string) (int, bool) {
	rest, ok := strings.CutPrefix(header, "-- selftracked dump ")
	if !ok {
		return 0, false
	}
	for f := range strings.FieldsSeq(rest) {
		raw, found := strings.CutPrefix(f, "schema_version=")
		if !found {
			continue
		}
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

// TrackedHeaderVersion reads the tracked dump's header version from an
// instance directory. The read is bounded: a legitimate header line is a
// few hundred bytes, so anything without a newline in the first 4 KiB is
// not a selftracked dump and reports (0, false) — explicitly, not as an
// ignored scanner error. A missing version is not a refusal here: the
// caller's other guards (comparison (ii), the §8.4 matrix, the §8.5
// parser) own every state a headerless file can produce.
func TrackedHeaderVersion(dir string) (int, bool) {
	f, err := os.Open(filepath.Join(dir, dumpFile)) //nolint:gosec // fixed .selftracked path
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()
	const headerCap = 4096
	buf := make([]byte, headerCap)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return 0, false
	}
	line, _, found := strings.Cut(string(buf[:n]), "\n")
	if !found && n == headerCap {
		return 0, false // no newline in the first 4 KiB: not a header
	}
	return HeaderVersion(line)
}

// TableOrder returns the serializer's emission order — parents before
// children, events last. The §8.6 migration engine flattens its rebuilt
// corpus in this order so the load path replays it exactly as it replays
// a dump: one FK-safe order, declared once.
func TableOrder() []string {
	names := make([]string, len(tables))
	for i, t := range tables {
		names[i] = t.name
	}
	return names
}

// Serialize renders the whole database as canonical dump text.
func Serialize(ctx context.Context, db *sql.DB) ([]byte, error) {
	var b strings.Builder

	if err := writeHeader(ctx, db, &b); err != nil {
		return nil, err
	}

	// Full DDL verbatim from the single compiled-in source (§8.1). The
	// embedded file ends with a newline; the byte-equal contract forbids
	// reformatting it here.
	b.WriteString(schema.DDL())

	boundary, err := archiveBoundary(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, t := range tables {
		if err := writeTable(ctx, db, &b, t, boundary); err != nil {
			return nil, err
		}
	}
	return []byte(b.String()), nil
}

// writeHeader emits the single header comment line (§8.1): schema_version
// and the tasks/artifacts id high-water for human readers — never the
// events/worklog seq, which change on every verb and would put the header
// into every diff.
func writeHeader(ctx context.Context, db *sql.DB, b *strings.Builder) error {
	var version string
	err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version)
	if err != nil {
		return fmt.Errorf("dump header: read schema_version: %w", err)
	}
	var tasks, artifacts int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM tasks`).Scan(&tasks); err != nil {
		return fmt.Errorf("dump header: tasks high-water: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM artifacts`).Scan(&artifacts); err != nil {
		return fmt.Errorf("dump header: artifacts high-water: %w", err)
	}
	fmt.Fprintf(b, "-- selftracked dump schema_version=%s tasks=%d artifacts=%d\n", version, tasks, artifacts)
	return nil
}

// archiveBoundary reads meta.events_archived_through, treating an absent
// row as 0 — a missing row must never drop events (§8.2).
func archiveBoundary(ctx context.Context, db *sql.DB) (int64, error) {
	var boundary int64
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE((SELECT CAST(value AS INTEGER) FROM meta WHERE key = 'events_archived_through'), 0)`).
		Scan(&boundary)
	if err != nil {
		return 0, fmt.Errorf("dump: read archive boundary: %w", err)
	}
	return boundary, nil
}

func writeTable(ctx context.Context, db *sql.DB, b *strings.Builder, t table, boundary int64) error {
	cols := strings.Fields(t.columns)
	colList := strings.Join(cols, ", ")
	// The identifiers come from the compiled-in table above, never input.
	//nolint:gosec // built from package-level constants, no external input
	query := "SELECT " + colList + " FROM " + t.name
	var args []any
	if t.where != "" {
		query += " WHERE " + t.where
		if strings.Contains(t.where, "?") {
			args = append(args, boundary)
		}
	}

	query += " ORDER BY " + t.orderBy

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("dump %s: %w", t.name, err)
	}
	defer func() { _ = rows.Close() }()

	values := make([]any, len(cols))
	scan := make([]any, len(cols))
	for i := range scan {
		scan[i] = &values[i]
	}
	prefix := "INSERT INTO " + t.name + " (" + colList + ") VALUES ("
	for rows.Next() {
		if err := rows.Scan(scan...); err != nil {
			return fmt.Errorf("dump %s: %w", t.name, err)
		}
		b.WriteString(prefix)
		if err := writeRow(b, cols, values); err != nil {
			return fmt.Errorf("dump %s: %w", t.name, err)
		}
		b.WriteString(");\n")
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("dump %s: %w", t.name, err)
	}
	return nil
}

func writeRow(b *strings.Builder, cols []string, values []any) error {
	for i, v := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		if err := writeLiteral(b, v); err != nil {
			return fmt.Errorf("%s: %w", cols[i], err)
		}
	}
	return nil
}

// errUnserializable reports a driver type outside the closed literal
// grammar — a serializer bug, never a value to improvise a spelling for.
var errUnserializable = errors.New("unserializable value type")

// writeLiteral emits one value in the serializer's closed literal grammar:
// decimal integers, single-quoted strings with quote doubling as the ONLY
// escape (§8.1), and the bare NULL that §5 permits in exactly three
// columns. Everything the schema can store is one of these — STRICT tables
// and text columns guarantee it — so any other driver type is a serializer
// error, not a value to improvise a spelling for.
func writeLiteral(b *strings.Builder, v any) error {
	switch val := v.(type) {
	case nil:
		b.WriteString("NULL")
	case int64:
		fmt.Fprintf(b, "%d", val)
	case string:
		b.WriteByte('\'')
		b.WriteString(strings.ReplaceAll(val, "'", "''"))
		b.WriteByte('\'')
	default:
		return fmt.Errorf("%w: %T", errUnserializable, v)
	}
	return nil
}
