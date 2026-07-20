package verify

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/load"
	"github.com/Spoloborota/selftracked/internal/rules"
	"github.com/Spoloborota/selftracked/internal/schema"
	"github.com/Spoloborota/selftracked/internal/state"
)

// errTamperedConfig marks a config value no verb could have written — a
// raw-SQL tamper surfaced while a rule tried to read it.
var errTamperedConfig = errors.New("config value is not verb-writable")

// r1 is the serialization rule (§7), checks 1–3. Check 3 (STATE.md
// byte-equals its render, the folded R14) lands here at S8c with the
// renderer (amendment r14-rides-its-renderer-at-s8c).
//
//  1. The dump regenerated from the live DB byte-equals tracked dump.sql
//     — a mismatch is a dirty dump (INV-350).
//  2. The tracked dump, loaded into a fresh database and re-serialized,
//     byte-equals itself — proving the tracked bytes round-trip through
//     the real §8.5 loader, not a second parser.
//  3. STATE.md byte-equals its render from the live DB — the former
//     standalone R14, folded into R1 (INV-275/293). A committed STATE.md
//     that drifts from the DB (a hand-edit, a `commit -n` bypass) is caught
//     here; `load` faithfully rebuilds the DB and does NOT paper over the
//     drift, so this check remains the surface that surfaces it.
//
// dbOnlyClean says whether the live DB passed the DB-only rules (R6–R9,
// R12). When it did NOT, check 2 is skipped: load.Build re-runs those same
// rules and would refuse, surfacing the ALREADY-reported violation a second
// time mislabelled as an R1 "does not rebuild" failure (found by the S7
// close review). Checks 1 and 3 run regardless — each is orthogonal to the
// data's rule-cleanliness.
func r1(ctx context.Context, db *sql.DB, dir string, dbOnlyClean bool) ([]rules.Violation, error) {
	var out []rules.Violation
	regen, err := dump.Serialize(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("R1: serialize: %w", err)
	}
	//nolint:gosec // dir/dump.sql is the tracker's own review surface, not user input
	tracked, err := os.ReadFile(filepath.Join(dir, dumpFile))
	if err != nil {
		// An unreadable tracked dump is a rule VIOLATION (the review surface
		// is gone), not an infrastructure failure — hence a nil error here.
		//nolint:nilerr // the missing dump is data in the report, not a run failure
		return []rules.Violation{{Rule: "R1", Message: "tracked dump.sql is unreadable: " + err.Error()}}, nil
	}
	if !bytes.Equal(regen, tracked) {
		out = append(out, rules.Violation{
			Rule:    "R1",
			Message: "dump.sql does not match the database (dirty dump); run selftracked dump",
		})
	}
	stateV, err := r1StateCheck(ctx, db, dir)
	if err != nil {
		return nil, err
	}
	if stateV != nil {
		out = append(out, *stateV)
	}
	if !dbOnlyClean {
		return out, nil // check 2 would only re-surface the DB-only violation
	}
	v, err := reloadRedump(ctx, tracked)
	if err != nil {
		return nil, err
	}
	if v != nil {
		out = append(out, *v)
	}
	return out, nil
}

// r1StateCheck is R1 check 3 (the folded R14): STATE.md at the repo root
// byte-equals its render from the live DB. An unreadable STATE.md is a
// violation (the tracked projection is gone), not an infrastructure failure.
func r1StateCheck(ctx context.Context, db *sql.DB, dir string) (*rules.Violation, error) {
	rendered, err := state.Render(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("R1 check 3: render STATE.md: %w", err)
	}
	tracked, err := os.ReadFile(filepath.Join(filepath.Dir(dir), stateFile))
	if err != nil {
		// An unreadable STATE.md is a rule VIOLATION (the projection is gone),
		// not an infrastructure failure — the missing file is report data.
		//nolint:nilerr // the missing STATE.md is data in the report, not a run failure
		return &rules.Violation{Rule: "R1", Message: "STATE.md is unreadable: " + err.Error()}, nil
	}
	if !bytes.Equal(rendered, tracked) {
		return &rules.Violation{
			Rule:    "R1",
			Message: "STATE.md does not match the database (stale projection); run selftracked state",
		}, nil
	}
	//nolint:nilnil // (no violation, no error) is the clean result
	return nil, nil
}

// reloadRedump runs R1 check 2. It returns a *Violation for a genuine
// round-trip defect (the tracked bytes will not parse, the loader refuses
// them, or the re-dump differs) and an error ONLY for an infrastructure
// failure (no temp dir, an open/serialize failure) — the §7 engine reports
// rule findings as data and reserves errors for the environment.
func reloadRedump(ctx context.Context, tracked []byte) (*rules.Violation, error) {
	parsed, err := load.Parse(tracked)
	if err != nil {
		return &rules.Violation{Rule: "R1", Message: "tracked dump does not reload: " + err.Error()}, nil
	}
	tmp, err := os.MkdirTemp("", "verify-r1-*")
	if err != nil {
		return nil, fmt.Errorf("R1 check 2: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	built, err := load.Build(ctx, tmp, parsed)
	if err != nil {
		if errors.Is(err, load.ErrRefused) {
			// The loader refused the tracked bytes: a whitelist/grammar/DDL
			// rejection — the dump does not round-trip. (A data-rule refusal
			// cannot reach here: r1 skips check 2 unless the DB is clean.)
			return &rules.Violation{Rule: "R1", Message: "tracked dump does not rebuild: " + err.Error()}, nil
		}
		return nil, fmt.Errorf("R1 check 2: rebuild: %w", err)
	}
	rdb, err := schema.OpenRead(built)
	if err != nil {
		return nil, fmt.Errorf("R1 check 2: open rebuilt db: %w", err)
	}
	defer func() { _ = rdb.Close() }()
	redumped, err := dump.Serialize(ctx, rdb)
	if err != nil {
		return nil, fmt.Errorf("R1 check 2: re-dump: %w", err)
	}
	if !bytes.Equal(redumped, tracked) {
		return &rules.Violation{Rule: "R1", Message: "tracked dump does not survive a reload/re-dump round-trip"}, nil
	}
	//nolint:nilnil // (no violation, no error) is exactly the clean round-trip result
	return nil, nil
}

// r2 is §7's path-root rule: every root in the dictionary exists on disk.
func r2(ctx context.Context, db *sql.DB, dir string) ([]rules.Violation, error) {
	base := filepath.Dir(dir) // roots resolve relative to the repo root, dir's parent
	rows, err := db.QueryContext(ctx, `SELECT class, scope, root FROM path_dictionary`)
	if err != nil {
		return nil, fmt.Errorf("R2: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []rules.Violation
	for rows.Next() {
		var class, scope, root string
		if err := rows.Scan(&class, &scope, &root); err != nil {
			return nil, fmt.Errorf("R2: %w", err)
		}
		if _, err := os.Stat(filepath.Join(base, root)); err != nil {
			out = append(out, rules.Violation{
				Rule:    "R2",
				Message: fmt.Sprintf("path root %q (%s) does not exist", root, classScope(class, scope)),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("R2: %w", err)
	}
	return out, nil
}

// r3 is §7's artifact rule: every non-archived artifact of a non-ephemeral
// class resolves to a file under its root. Ephemeral classes (workdir, run)
// are existence-exempt by design (§5.2).
func r3(ctx context.Context, db *sql.DB, dir string) ([]rules.Violation, error) {
	base := filepath.Dir(dir)
	rows, err := db.QueryContext(ctx, `
		SELECT a.class, a.scope, a.relpath, p.root FROM artifacts a
		JOIN path_dictionary p ON p.class = a.class AND p.scope = a.scope
		WHERE a.archived = 0 AND p.ephemeral = 0`)
	if err != nil {
		return nil, fmt.Errorf("R3: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []rules.Violation
	for rows.Next() {
		var class, scope, relpath, root string
		if err := rows.Scan(&class, &scope, &relpath, &root); err != nil {
			return nil, fmt.Errorf("R3: %w", err)
		}
		if _, err := os.Stat(filepath.Join(base, root, relpath)); err != nil {
			out = append(out, rules.Violation{
				Rule:    "R3",
				Message: fmt.Sprintf("artifact %s:%s does not resolve", classScope(class, scope), relpath),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("R3: %w", err)
	}
	return out, nil
}

// r5 is §7's commit rule: every non-`legacy:` commits cell resolves via
// `git cat-file`. A range A..B resolves both endpoints. Runs from the repo
// root, dir's parent.
func r5(ctx context.Context, db *sql.DB, dir string) ([]rules.Violation, error) {
	base := filepath.Dir(dir)
	if err := gitPresent(ctx, base); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT epic, seq, commits FROM worklog
		WHERE commits <> '' AND commits NOT LIKE 'legacy:%'`)
	if err != nil {
		return nil, fmt.Errorf("R5: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []rules.Violation
	for rows.Next() {
		var epic, commits string
		var seq int64
		if err := rows.Scan(&epic, &seq, &commits); err != nil {
			return nil, fmt.Errorf("R5: %w", err)
		}
		for _, ref := range commitRefs(commits) {
			if !commitResolves(ctx, base, ref) {
				out = append(out, rules.Violation{
					Rule:    "R5",
					Message: fmt.Sprintf("worklog %s/%d cites %q, which does not resolve", epic, seq, ref),
				})
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("R5: %w", err)
	}
	return out, nil
}

// r10 (advisory) is the idle report, §7 verbatim: ACTIVE epics with no
// READY/IN-PROGRESS story and no non-correction worklog append in
// idle_days. An epic with no such append at all counts as idle (COALESCE to
// the empty string, which sorts before any timestamp). Correction rows are
// excluded so an unrelated historical correction cannot reset a neglected
// epic's clock (§5.7, INV-117). PAUSED/BACKLOG epics are silent by design.
//
// The comparison is lexical, which is sound because every verb writes dates
// in one canonical form (now(): ISO-8601 UTC, ...Z). A non-canonical stored
// date (only reachable by raw SQL, or by an importer that fails to
// normalise — an S9 obligation) would compare wrongly; R10 is advisory and
// assumes the canonical storage the write path guarantees.
func r10(ctx context.Context, db *sql.DB) ([]rules.Violation, error) {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'idle_days'`).Scan(&raw); err != nil {
		return nil, fmt.Errorf("R10: read idle_days: %w", err)
	}
	idleDays, err := strconv.Atoi(raw)
	if err != nil || idleDays <= 0 {
		// config set validates idle_days as a positive integer, so this is a
		// raw-SQL tamper: an infrastructure failure, not a rule finding.
		return nil, fmt.Errorf("R10: %w: idle_days %q is not a positive integer", errTamperedConfig, raw)
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -idleDays).Format("2006-01-02T15:04:05Z")
	rows, err := db.QueryContext(ctx, `
		SELECT e.slug FROM epics e
		WHERE e.status = 'ACTIVE'
		  AND NOT EXISTS (SELECT 1 FROM stories s WHERE s.epic = e.slug AND s.status IN ('READY','IN-PROGRESS'))
		  AND COALESCE((SELECT MAX(w.date) FROM worklog w
		                WHERE w.epic = e.slug AND w.corrects IS NULL), '') < ?
		ORDER BY e.slug`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("R10: %w", err)
	}
	var out []rules.Violation
	out, err = drain(rows, out, "R10", fmt.Sprintf("epic %%s is idle (no active story, no append in %d days)", idleDays))
	return out, err
}

// r13 (advisory): OPEN tasks with no `home` artifact link (§5.8).
func r13(ctx context.Context, db *sql.DB) ([]rules.Violation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.id FROM tasks t
		WHERE t.status = 'OPEN' AND NOT EXISTS (
			SELECT 1 FROM task_artifacts ta WHERE ta.task = t.id AND ta.role = 'home')
		ORDER BY t.id`)
	if err != nil {
		return nil, fmt.Errorf("R13: %w", err)
	}
	var out []rules.Violation
	out, err = drain(rows, out, "R13", "OPEN task #%v has no home link")
	return out, err
}

// r15 (advisory): a pending skip marker signals an unconverted gate skip.
// It is a bare file check, cheap enough to run on the --fast path (§7).
func r15(dir string) []rules.Violation {
	if _, err := os.Stat(filepath.Join(dir, skipMarker)); err == nil {
		return []rules.Violation{{
			Rule:    "R15",
			Message: "a pending .selftracked/skip-pending marker records an unconverted gate skip",
		}}
	}
	return nil
}

// commitRefs splits a commits cell into the git references it names,
// expanding an A..B range into both endpoints.
func commitRefs(cell string) []string {
	fields := strings.FieldsFunc(cell, func(r rune) bool { return r == ' ' || r == '\t' || r == ',' })
	var refs []string
	for _, f := range fields {
		if before, after, isRange := strings.Cut(f, ".."); isRange {
			// Trim any residual dots so a three-dot range (a...b) yields the
			// two endpoints, not "a" and ".b" — the symmetric-difference form
			// is out of the sanctioned <sha>..<sha> grammar but is a common
			// git idiom, and a leading-dot token would misreport which ref
			// failed to resolve.
			for _, part := range []string{before, after} {
				if p := strings.Trim(part, "."); p != "" {
					refs = append(refs, p)
				}
			}
			continue
		}
		refs = append(refs, f)
	}
	return refs
}

func commitResolves(ctx context.Context, base, ref string) bool {
	// base is the repo root and ref is an opaque token passed as an argument,
	// never interpolated into a shell — cat-file resolves or it does not.
	//nolint:gosec // fixed argv, no shell
	cmd := exec.CommandContext(ctx, "git", "-C", base, "cat-file", "-e", ref+"^{commit}")
	return cmd.Run() == nil
}

// gitPresent confirms base is inside a git work tree, so R5's per-ref
// failures mean "unresolvable object", not "no repository".
func gitPresent(ctx context.Context, base string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", base, "rev-parse", "--git-dir") //nolint:gosec // fixed argv, no shell
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("verify R5: %s is not a git repository: %w", base, err)
	}
	return nil
}

func classScope(class, scope string) string {
	if scope == "" {
		return class
	}
	return class + "@" + scope
}

// drain collects a single-column violation query into messages.
func drain(rows *sql.Rows, out []rules.Violation, rule, format string) ([]rules.Violation, error) {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var v any
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("%s: %w", rule, err)
		}
		out = append(out, rules.Violation{Rule: rule, Message: fmt.Sprintf(format, v)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", rule, err)
	}
	return out, nil
}
