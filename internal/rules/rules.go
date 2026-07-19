// Package rules holds the §7 integrity rules that need only the database —
// no filesystem, no git, no serialization. Born at S4 because §8.5 makes
// `load` run exactly this subset (R6–R9, R12) before its atomic rename;
// S7 wraps the `verify` verb and its reporting contract around the same
// code. Nothing here prints, exits, or knows about verbs: it returns
// violations and lets the caller decide what they cost.
package rules

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/Spoloborota/selftracked/internal/ref"
)

// Violation is one broken rule instance.
type Violation struct {
	Rule    string // "R6" … "R12"
	Message string
}

// DBOnly runs the pure-SQL Stage-1 subset: R6, R7, R8, R9, R12.
func DBOnly(ctx context.Context, db *sql.DB) ([]Violation, error) {
	var all []Violation
	for _, run := range []func(context.Context, *sql.DB) ([]Violation, error){
		r6, r7, r8, r9, r12,
	} {
		v, err := run(ctx, db)
		if err != nil {
			return nil, err
		}
		all = append(all, v...)
	}
	return all, nil
}

// r6: DONE story ⇔ DONE worklog row with non-empty commits, both
// directions, correction rows excluded from the count (§7).
func r6(ctx context.Context, db *sql.DB) ([]Violation, error) {
	var out []Violation
	rows, err := db.QueryContext(ctx, `
		SELECT s.epic, s.id FROM stories s
		WHERE s.status = 'DONE' AND NOT EXISTS (
			SELECT 1 FROM worklog w
			WHERE w.epic = s.epic AND w.story = s.id AND w.state = 'DONE'
			  AND w.commits <> '' AND w.corrects IS NULL)`)
	if err != nil {
		return nil, fmt.Errorf("R6: %w", err)
	}
	out, err = collect(rows, out, "R6", "DONE story %s/%s has no DONE worklog row with commits")
	if err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, `
		SELECT w.epic, w.story FROM worklog w
		WHERE w.state = 'DONE' AND w.corrects IS NULL AND w.commits <> ''
		  AND w.story GLOB 'S[0-9]*'
		  AND NOT EXISTS (
			SELECT 1 FROM stories s
			WHERE s.epic = w.epic AND s.id = w.story AND s.status = 'DONE')`)
	if err != nil {
		return nil, fmt.Errorf("R6: %w", err)
	}
	return collect(rows, out, "R6", "DONE worklog row %s/%s has no DONE story")
}

// r7: duplicates links ⇔ dup_of, one-to-one; no dup_of target is itself
// DUPLICATE (no chains or cycles).
func r7(ctx context.Context, db *sql.DB) ([]Violation, error) {
	var out []Violation
	rows, err := db.QueryContext(ctx, `
		SELECT t.id, t.dup_of FROM tasks t
		WHERE t.dup_of IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM task_links l
			WHERE l.from_task = t.id AND l.to_task = t.dup_of AND l.type = 'duplicates')`)
	if err != nil {
		return nil, fmt.Errorf("R7: %w", err)
	}
	out, err = collect(rows, out, "R7", "task #%v has dup_of=%v but no duplicates link")
	if err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, `
		SELECT l.from_task, l.to_task FROM task_links l
		WHERE l.type = 'duplicates' AND NOT EXISTS (
			SELECT 1 FROM tasks t WHERE t.id = l.from_task AND t.dup_of = l.to_task)`)
	if err != nil {
		return nil, fmt.Errorf("R7: %w", err)
	}
	out, err = collect(rows, out, "R7", "duplicates link #%v→#%v has no matching dup_of")
	if err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, `
		SELECT t.id, t.dup_of FROM tasks t JOIN tasks c ON c.id = t.dup_of
		WHERE c.status = 'DUPLICATE'`)
	if err != nil {
		return nil, fmt.Errorf("R7: %w", err)
	}
	return collect(rows, out, "R7", "task #%v points dup_of at #%v, itself DUPLICATE (chain)")
}

// r8: every events.entity resolves — §4 grammar plus existence.
func r8(ctx context.Context, db *sql.DB) ([]Violation, error) {
	rows, err := db.QueryContext(ctx, `SELECT seq, entity FROM events ORDER BY seq`)
	if err != nil {
		return nil, fmt.Errorf("R8: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Violation
	for rows.Next() {
		var seq int64
		var entity string
		if err := rows.Scan(&seq, &entity); err != nil {
			return nil, fmt.Errorf("R8: %w", err)
		}
		r, err := ref.Parse(entity)
		if err != nil {
			out = append(out, Violation{
				Rule:    "R8",
				Message: fmt.Sprintf("events seq %d: entity %q does not parse", seq, entity),
			})
			continue
		}
		exists, err := refExists(ctx, db, r)
		if err != nil {
			return nil, err
		}
		if !exists {
			out = append(out, Violation{
				Rule:    "R8",
				Message: fmt.Sprintf("events seq %d: entity %q does not exist", seq, entity),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("R8: %w", err)
	}
	return out, nil
}

func refExists(ctx context.Context, db *sql.DB, r ref.Ref) (bool, error) {
	var q string
	var args []any
	switch r.Kind {
	case ref.Task:
		q, args = `SELECT 1 FROM tasks WHERE id = ?`, []any{r.Task}
	case ref.Epic:
		q, args = `SELECT 1 FROM epics WHERE slug = ?`, []any{r.Epic}
	case ref.Story:
		q, args = `SELECT 1 FROM stories WHERE epic = ? AND id = ?`, []any{r.Epic, r.Story}
	case ref.Artifact:
		q, args = `SELECT 1 FROM artifacts WHERE class = ? AND scope = ? AND relpath = ?`,
			[]any{r.Class, r.Scope, r.Rel}
	default:
		return false, nil
	}
	var one int
	err := db.QueryRowContext(ctx, q, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("R8 existence: %w", err)
	}
	return true, nil
}

// r9: sqlite_sequence ≥ MAX(id) per AUTOINCREMENT table (only when the
// table has rows; an absent sequence row counts as 0), and the archive
// boundary is present, a non-negative integer, and exactly 0 in schema v1
// — no verb can write it, so any other value is the raw-SQL tamper
// signature (§8.2).
func r9(ctx context.Context, db *sql.DB) ([]Violation, error) {
	var out []Violation
	for _, tbl := range []struct{ name, id string }{
		{"tasks", "id"}, {"artifacts", "id"}, {"events", "seq"},
	} {
		var maxID, seq int64
		var hasRows bool
		//nolint:gosec // names from the fixed list above
		q := fmt.Sprintf(`SELECT COALESCE(MAX(%s),0), COUNT(*) > 0 FROM %s`, tbl.id, tbl.name)
		if err := db.QueryRowContext(ctx, q).Scan(&maxID, &hasRows); err != nil {
			return nil, fmt.Errorf("R9: %w", err)
		}
		if !hasRows {
			continue
		}
		err := db.QueryRowContext(ctx,
			`SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name = ?), 0)`, tbl.name).Scan(&seq)
		if err != nil {
			return nil, fmt.Errorf("R9: %w", err)
		}
		if seq < maxID {
			out = append(out, Violation{
				Rule:    "R9",
				Message: fmt.Sprintf("%s: sqlite_sequence %d below MAX(%s) %d", tbl.name, seq, tbl.id, maxID),
			})
		}
	}
	boundary, err := boundaryViolation(ctx, db)
	if err != nil {
		return nil, err
	}
	return append(out, boundary...), nil
}

// boundaryViolation is R9's second clause: the archive boundary must be
// present, a non-negative integer, and exactly 0 in schema v1.
func boundaryViolation(ctx context.Context, db *sql.DB) ([]Violation, error) {
	var raw string
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE((SELECT value FROM meta WHERE key = 'events_archived_through'), '')`).Scan(&raw)
	if err != nil {
		return nil, fmt.Errorf("R9: %w", err)
	}
	if raw == "" {
		return []Violation{{Rule: "R9", Message: "meta.events_archived_through is missing"}}, nil
	}
	n, isNonNeg := parseNonNegInt(raw)
	switch {
	case !isNonNeg:
		return []Violation{{
			Rule:    "R9",
			Message: "meta.events_archived_through is not a non-negative integer: " + raw,
		}}, nil
	case n != 0:
		return []Violation{{
			Rule:    "R9",
			Message: "meta.events_archived_through = " + raw + "; must be 0 in schema v1 — raw-SQL tamper signature",
		}}, nil
	}
	return nil, nil
}

// r12: every terminal epic and task has a matching events row (or an
// import one) — a terminal state with no trail is the forgery signature.
func r12(ctx context.Context, db *sql.DB) ([]Violation, error) {
	var out []Violation
	rows, err := db.QueryContext(ctx, `
		SELECT t.id FROM tasks t
		WHERE t.status IN ('DONE','WONT-DO','DUPLICATE') AND NOT EXISTS (
			SELECT 1 FROM events e
			WHERE e.entity = '#' || t.id AND e.event IN ('set-status','import'))`)
	if err != nil {
		return nil, fmt.Errorf("R12: %w", err)
	}
	out, err = collect(rows, out, "R12", "terminal task #%v has no events trail")
	if err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, `
		SELECT e.slug FROM epics e
		WHERE e.status IN ('CLOSED','DISSOLVED') AND NOT EXISTS (
			SELECT 1 FROM events ev
			WHERE ev.entity = 'epic:' || e.slug AND ev.event IN ('epic','import'))`)
	if err != nil {
		return nil, fmt.Errorf("R12: %w", err)
	}
	return collect(rows, out, "R12", "terminal epic %v has no events trail")
}

// collect drains a 1- or 2-column violation query into messages.
func collect(rows *sql.Rows, out []Violation, rule, format string) ([]Violation, error) {
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rule, err)
	}
	for rows.Next() {
		vals := make([]any, len(cols))
		scan := make([]any, len(cols))
		for i := range vals {
			scan[i] = &vals[i]
		}
		if err := rows.Scan(scan...); err != nil {
			return nil, fmt.Errorf("%s: %w", rule, err)
		}
		out = append(out, Violation{Rule: rule, Message: fmt.Sprintf(format, vals...)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", rule, err)
	}
	return out, nil
}

// parseNonNegInt swallows the parse error deliberately: a malformed value
// is a rule VIOLATION result, not an execution failure.
func parseNonNegInt(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
