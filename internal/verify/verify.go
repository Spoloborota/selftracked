// Package verify is the §7 integrity engine behind the `verify` verb. It
// opens the tracker read-only and never heals or writes: verify observes,
// the write pipeline heals. Stage 0 is the container check (integrity /
// foreign keys); Stage 1 is the rule battery. Rules split two ways —
// red (R1–R9, R12: a violation fails the run) and advisory (R10, R11, R13,
// R15, R16: reported, exit unaffected) — and the pre-commit `--fast` partition
// runs only the pure-SQL rules (R10 among them) plus R15, skipping
// serialization/filesystem/git work (§7). R14 / R1 check 3 (STATE.md byte-equals its render) lands
// here at S8c with the renderer (amendment r14-rides-its-renderer-at-s8c).
package verify

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Spoloborota/selftracked/internal/rules"
	"github.com/Spoloborota/selftracked/internal/schema"
)

// Instance paths (§9). verify reads them; it writes nothing. STATE.md is a
// sibling of the instance dir at the repo root (dir's parent), not inside it.
const (
	instanceDir = ".selftracked"
	dbFile      = "db.sqlite"
	dumpFile    = "dump.sql"
	stateFile   = "STATE.md"
	skipMarker  = "skip-pending"
)

// advisoryRules are the rule names §7 marks advisory: reported, never
// fatal. Everything else in a Report's findings is red.
var advisoryRules = map[string]bool{"R10": true, "R11": true, "R13": true, "R15": true, "R16": true}

// Report is one verify run's outcome, findings already split by severity.
type Report struct {
	Red      []rules.Violation // R1–R9, R12, Stage 0 — a non-empty Red fails the run
	Advisory []rules.Violation // R10, R11, R13, R15, R16 — reported only
	Fast     bool              // whether this was the --fast partition
}

// route files a batch of findings into Red or Advisory by rule name.
func (r *Report) route(vs []rules.Violation) {
	for _, v := range vs {
		if advisoryRules[v.Rule] {
			r.Advisory = append(r.Advisory, v)
		} else {
			r.Red = append(r.Red, v)
		}
	}
}

// Run executes verify against the tracker rooted at dir. fast selects the
// pre-commit partition. It returns an error only for infrastructure
// failures (no database, a query that could not run); rule VIOLATIONS are
// data in the Report, not errors — the caller decides their exit cost.
func Run(ctx context.Context, dir string, fast bool) (Report, error) {
	rep := Report{Fast: fast}
	dbPath := filepath.Join(dir, dbFile)
	if _, err := os.Stat(dbPath); err != nil {
		return rep, &notFoundError{path: dbPath}
	}
	db, err := schema.OpenRead(dbPath)
	if err != nil {
		return rep, fmt.Errorf("verify: open %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	if err := stage0(ctx, db, &rep, fast); err != nil {
		return rep, err
	}
	if err := stage1(ctx, db, dir, &rep, fast); err != nil {
		return rep, err
	}
	return rep, nil
}

// stage0 is the container check: quick_check (--fast) or integrity_check
// (full) — neither covers foreign keys, so foreign_key_check runs in both
// (§7; INV-010/288/544). A structural failure here is red.
func stage0(ctx context.Context, db *sql.DB, rep *Report, fast bool) error {
	pragma, label := "integrity_check", "integrity_check"
	if fast {
		pragma, label = "quick_check", "quick_check"
	}
	var verdict string
	if err := db.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&verdict); err != nil {
		return fmt.Errorf("verify: %s: %w", label, err)
	}
	if verdict != "ok" {
		rep.Red = append(rep.Red, rules.Violation{Rule: "integrity", Message: label + ": " + verdict})
	}
	fkRows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("verify: foreign_key_check: %w", err)
	}
	defer func() { _ = fkRows.Close() }()
	for fkRows.Next() {
		var table, rowid, parent, fkid sql.NullString
		if err := fkRows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return fmt.Errorf("verify: foreign_key_check: %w", err)
		}
		rep.Red = append(rep.Red, rules.Violation{
			Rule:    "fk",
			Message: fmt.Sprintf("foreign key violation in %s -> %s", table.String, parent.String),
		})
	}
	if err := fkRows.Err(); err != nil {
		return fmt.Errorf("verify: foreign_key_check: %w", err)
	}
	return nil
}

// stage1 runs the rule battery. --fast runs only the pure-SQL rules (R4 +
// DBOnly's R6–R9/R12, plus the pure-SQL advisory R10) and R15; full verify
// adds the serialization (R1), filesystem (R2, R3), git (R5) and the
// remaining advisory (R11, R13, R16) rules.
func stage1(ctx context.Context, db *sql.DB, dir string, rep *Report, fast bool) error {
	r4, err := rules.R4(ctx, db)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	rep.route(r4)
	dbOnly, err := rules.DBOnly(ctx, db)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	rep.route(dbOnly)
	dbOnlyClean := len(dbOnly) == 0 // gates R1 check 2 — see r1's doc

	// The two advisory rules cheap enough for the commit boundary: R15 is a
	// bare file check, and R10 is pure SQL (an idle_days lookup, two epic
	// scans, a clock read) — its former place among the filesystem/git skips
	// never matched what it does (amendment
	// `r10-sees-the-window-it-was-meant-to-watch`).
	rep.route(r15(dir))
	r10v, err := r10(ctx, db)
	if err != nil {
		return err
	}
	rep.route(r10v)
	if fast {
		return nil
	}

	// Full-only rules, each returning its findings or an infra error.
	for _, run := range []func() ([]rules.Violation, error){
		func() ([]rules.Violation, error) { return r1(ctx, db, dir, dbOnlyClean) },
		func() ([]rules.Violation, error) { return r2(ctx, db, dir) },
		func() ([]rules.Violation, error) { return r3(ctx, db, dir) },
		func() ([]rules.Violation, error) { return r5(ctx, db, dir) },
		func() ([]rules.Violation, error) { return r11(ctx, dir) },
		func() ([]rules.Violation, error) { return r13(ctx, db) },
		func() ([]rules.Violation, error) { return r16(ctx, db) },
	} {
		vs, err := run()
		if err != nil {
			return err
		}
		rep.route(vs)
	}
	return nil
}

// notFoundError is verify's one refusal: no tracker here.
type notFoundError struct{ path string }

func (e *notFoundError) Error() string {
	return "no " + e.path + " here; run selftracked init first"
}
