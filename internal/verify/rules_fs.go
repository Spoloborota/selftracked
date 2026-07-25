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
	"slices"
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
			if strings.HasPrefix(ref, "-") {
				// git reads a leading-dash argv as an option, not a revision,
				// so such a token would reach cat-file as a flag. Nothing
				// legitimate has this shape — git itself refuses a branch or
				// tag starting with '-' — so the cell is malformed or planted.
				out = append(out, rules.Violation{
					Rule:    "R5",
					Message: fmt.Sprintf("worklog %s/%d cites %q, which is not a revision", epic, seq, ref),
				})
				continue
			}
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

// r10 (advisory) is §7's two-trigger report on an ACTIVE epic. PAUSED and
// BACKLOG epics are silent by design — they are intentional states.
//
//	(a) No home, no window (amendment
//	    `r10-sees-the-window-it-was-meant-to-watch`): no story in a
//	    NON-TERMINAL status, i.e. every story terminal or no story at all.
//	    Reported at once, independent of idle_days, because that is the
//	    state in which work can be recorded nowhere. The predicate is
//	    rules.EpicsWithoutWorkableStory — the one definition, shared with
//	    `prime`.
//	(b) Idle: no READY/IN-PROGRESS story and no non-correction worklog
//	    append in idle_days. An epic with no such append at all counts as
//	    idle (COALESCE to the empty string, which sorts before any
//	    timestamp). Correction rows are excluded so an unrelated historical
//	    correction cannot reset a neglected epic's clock (§5.7, INV-117).
//
// The two story clauses are DELIBERATELY different sets and the asymmetry
// is load-bearing, not drift: (a) asks "can this epic receive work at all",
// where a PLANNED story is a home (`story ready` → `story start` reaches it
// with no scope change) and a BLOCKED one is the sanctioned PO-absent state
// (§11.3); (b) asks "has this epic gone quiet", where neither counts.
//
// ONE EPIC YIELDS ONE LINE, stating whichever facts hold — an epic that is
// both homeless and idle is one finding, not two.
//
// (b)'s comparison is lexical, which is sound because every verb writes
// dates in one canonical form (now(): ISO-8601 UTC, ...Z). A non-canonical
// stored date (only reachable by raw SQL, or by an importer that fails to
// normalise — an S9 obligation) would compare wrongly; R10 is advisory and
// assumes the canonical storage the write path guarantees.
func r10(ctx context.Context, db *sql.DB) ([]rules.Violation, error) {
	homeless, err := rules.EpicsWithoutWorkableStory(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("R10: %w", err)
	}
	idleDays, err := r10IdleDays(ctx, db)
	if err != nil {
		return nil, err
	}
	idle, err := r10IdleEpics(ctx, db, idleDays)
	if err != nil {
		return nil, err
	}
	return r10Findings(homeless, idle, idleDays), nil
}

// r10IdleDays reads the configured window. A value outside `config set`'s
// own validation is a raw-SQL tamper: an infrastructure failure, not a rule
// finding, because the rule cannot be evaluated at all.
func r10IdleDays(ctx context.Context, db *sql.DB) (int, error) {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'idle_days'`).Scan(&raw); err != nil {
		return 0, fmt.Errorf("R10: read idle_days: %w", err)
	}
	idleDays, err := strconv.Atoi(raw)
	if err != nil || idleDays <= 0 {
		return 0, fmt.Errorf("R10: %w: idle_days %q is not a positive integer", errTamperedConfig, raw)
	}
	return idleDays, nil
}

// r10IdleEpics is trigger (b), in slug order. Its story clause is the
// READY/IN-PROGRESS pair and stays inline: it is not the shared
// non-terminal vocabulary and must not be generated from it — see r10's
// note on the asymmetry.
func r10IdleEpics(ctx context.Context, db *sql.DB, idleDays int) ([]string, error) {
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
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("R10: %w", err)
		}
		out = append(out, slug)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("R10: %w", err)
	}
	return out, nil
}

// r10Findings merges the two trigger sets into one finding per epic. Both
// inputs arrive in slug order; the union is re-sorted so the report is
// deterministic whichever trigger contributed a slug.
func r10Findings(homeless, idle []string, idleDays int) []rules.Violation {
	isHomeless := make(map[string]bool, len(homeless))
	for _, slug := range homeless {
		isHomeless[slug] = true
	}
	isIdle := make(map[string]bool, len(idle))
	for _, slug := range idle {
		isIdle[slug] = true
	}
	union := make([]string, 0, len(homeless)+len(idle))
	union = append(union, homeless...)
	union = append(union, idle...)
	slices.Sort(union)
	union = slices.Compact(union)
	out := make([]rules.Violation, 0, len(union))
	for _, slug := range union {
		var msg string
		switch {
		case isHomeless[slug] && isIdle[slug]:
			// Both facts, one line. The idle half drops "no active story":
			// the homeless clause already says something stronger.
			msg = fmt.Sprintf("epic %s has no story that can receive work; new work has no home; "+
				"and it is idle (no append in %d days)", slug, idleDays)
		case isHomeless[slug]:
			msg = fmt.Sprintf("epic %s has no story that can receive work; new work has no home", slug)
		default:
			msg = fmt.Sprintf("epic %s is idle (no active story, no append in %d days)", slug, idleDays)
		}
		out = append(out, rules.Violation{Rule: "R10", Message: msg})
	}
	return out
}

// r13 (advisory): OPEN tasks with no LIVE `home` artifact link (§5.8;
// amendment `r13-counts-live-homes`). An archived home is history, not a
// home: R3 stops checking an archived artifact's existence, so counting
// one here would let an OPEN task hold a home that points at a deleted
// file with no signal from any surface.
func r13(ctx context.Context, db *sql.DB) ([]rules.Violation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.id FROM tasks t
		WHERE t.status = 'OPEN' AND NOT EXISTS (
			SELECT 1 FROM task_artifacts ta JOIN artifacts a ON a.id = ta.artifact
			WHERE ta.task = t.id AND ta.role = 'home' AND a.archived = 0)
		ORDER BY t.id`)
	if err != nil {
		return nil, fmt.Errorf("R13: %w", err)
	}
	var out []rules.Violation
	out, err = drain(rows, out, "R13", "OPEN task #%v has no home link")
	return out, err
}

// r16 (advisory): a CLOSED epic no longer satisfies the conditions it was
// closed on (§6.4 conditions 1, 3, 4; amendment
// `terminal-epic-conditions-stay-true`). Scoped to epics THIS tracker
// closed — an `epic` event carrying the close stamp: an epic that arrived
// CLOSED through `import` never passed the gate, so there is no claim to
// re-check. Conditions 2/5 ride R6 and story rows cannot be deleted, so
// condition 6 cannot fall.
//
// It re-executes nothing: `met` is read as stored, the way `epic show`
// reads it. Reusing close condition (3)'s engine would run repo-state
// shell commands inside verify and write their results back — verify's
// connection is query_only and the execution surface is not verify's to
// own.
//
// EVERY FINDING NAMES ITS REPAIR (amendment
// `r16-reports-only-what-a-verb-can-clear`), because two of the three
// clauses used to report a state no verb could clear, and an advisory a
// reader cannot act on is one they learn to scroll past — at the cost of
// R10, R11, R13 and R15, which share the same channel. The story clause
// names the §6.4 carve-out this change opened for exactly it; the criterion
// clause names no verb, because none exists: on a verb-closed epic
// `criteria add`/`criteria met` refuse, `criteria check` is report-only and
// `import` refuses to re-declare an existing epic, so `met = 0` there is a
// raw-SQL signature of R12's family and the message says so.
func r16(ctx context.Context, db *sql.DB) ([]rules.Violation, error) {
	// The provenance predicate has ONE definition, in internal/rules, shared
	// with the §6.4 `story dissolve` carve-out: the repair a finding names
	// must be available exactly where the finding can appear, and two
	// hand-written copies of "closed by verb" is how that agreement breaks.
	closedByVerb := rules.ClosedByVerbSQL(`e.slug`)
	// The concatenated operand is rules.ClosedByVerbSQL applied to a SOURCE
	// LITERAL — no value, no input, nothing derived from either, reaches the
	// SQL text; the fragment binds nothing and the whole query takes no
	// arguments. gosec cannot see that across the call, so the exemption is
	// stated here rather than by inlining a second copy of the predicate,
	// which is the defect this shared fragment exists to prevent.
	//nolint:gosec // G202: operand is a package-level SQL fragment over a source literal
	rows, err := db.QueryContext(ctx, `
		SELECT e.slug || ': story ' || s.id || ' is not terminal; repair: selftracked story dissolve '
		       || e.slug || ' ' || s.id || ' --why TEXT'
		FROM epics e JOIN stories s ON s.epic = e.slug
		WHERE e.status = 'CLOSED' AND s.status NOT IN ('DONE','DISSOLVED') AND `+closedByVerb+`
		UNION ALL
		SELECT e.slug || ': task #' || t.id || ' (' || t.status || ') is homed here; repair: selftracked edit '
		       || t.id || ' --detach, or set it terminal'
		FROM epics e JOIN tasks t ON t.epic = e.slug
		WHERE e.status = 'CLOSED' AND t.status IN ('OPEN','IN-REVIEW','NEEDS-TRIAGE') AND `+closedByVerb+`
		UNION ALL
		SELECT e.slug || ': criterion ' || c.seq
		       || ' is not met; no verb writes this state here — it arrived by raw SQL'
		FROM epics e JOIN epic_criteria c ON c.epic = e.slug
		WHERE e.status = 'CLOSED' AND c.met = 0 AND `+closedByVerb+`
		ORDER BY 1`)
	if err != nil {
		return nil, fmt.Errorf("R16: %w", err)
	}
	var out []rules.Violation
	out, err = drain(rows, out, "R16", "closed epic %v")
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
