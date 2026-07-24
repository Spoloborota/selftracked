package verb

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Spoloborota/selftracked/internal/cli"
)

const (
	epicBacklog   = "BACKLOG"
	epicActive    = "ACTIVE"
	epicPaused    = "PAUSED"
	epicClosed    = "CLOSED"
	epicDissolved = "DISSOLVED"

	evEpic = "epic"
)

// EpicVerbs returns the S6 `epic` catalog entry.
func EpicVerbs() []cli.Verb {
	var goal, why string
	var activeOnly bool
	return []cli.Verb{{Name: "epic", Subs: []cli.Sub{
		{
			Name: evCreate, Arity: 1, Usage: "epic create SLUG --goal G [--json]",
			Flags: func(fs *flag.FlagSet) { fs.StringVar(&goal, "goal", "", "the epic's goal (required)") },
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return epicCreate(e, pos[0], goal)
			},
		},
		{
			Name: "activate", Arity: 1, Usage: "epic activate SLUG [--json]",
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return epicTransition(e, pos[0], epicActive, "")
			},
		},
		{
			Name: "pause", Arity: 1, Usage: "epic pause SLUG --why TEXT [--json]",
			Flags: func(fs *flag.FlagSet) { fs.StringVar(&why, "why", "", "the pause reason (required)") },
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				if why == "" {
					return refuse("usage", "epic pause requires --why")
				}
				return epicTransition(e, pos[0], epicPaused, why)
			},
		},
		{
			Name: "dissolve", Arity: 1, Usage: "epic dissolve SLUG --why TEXT [--json]",
			Flags: func(fs *flag.FlagSet) { fs.StringVar(&why, "why", "", "the dissolve reason (required)") },
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				if why == "" {
					return refuse("usage", "epic dissolve requires --why")
				}
				return epicDissolve(e, pos[0], why)
			},
		},
		{
			Name: "show", Arity: 1, Usage: "epic show SLUG [--json]",
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return epicShowFull(e, pos[0])
			},
		},
		{
			Name: "list", Arity: 0, Usage: "epic list [--active] [--json]",
			Flags: func(fs *flag.FlagSet) { fs.BoolVar(&activeOnly, "active", false, "only ACTIVE epics") },
			Run: func(e *cli.Env, _ []string, _ *flag.FlagSet) error {
				return epicList(e, activeOnly)
			},
		},
		{
			Name: "close", Arity: 1, Usage: "epic close SLUG [--json]",
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return epicClose(e, pos[0])
			},
		},
	}}}
}

// slugOK enforces kebab-case, permanent slugs (§4): lowercase words
// joined by single dashes.
var slugShape = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func slugOK(slug string) bool { return slugShape.MatchString(slug) }

func epicCreate(e *cli.Env, slug, goal string) error {
	if !slugOK(slug) {
		return refuse("usage", "%q is not a kebab-case slug", slug)
	}
	if goal == "" {
		return refuse("usage", "epic create requires --goal")
	}
	if err := validateText("--goal", goal); err != nil {
		return err
	}
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		if _, err := tx.ExecContext(context.Background(),
			`INSERT INTO epics (slug, goal, created_at) VALUES (?, ?, ?)`,
			slug, goal, now()); err != nil {
			return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
		}
		return []Event{{Entity: epicPrefix + slug, Event: evEpic, Detail: "create: " + goal}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "epic:%s [BACKLOG]\n", slug)
	return nil
}

func epicTransition(e *cli.Env, slug, target, note string) error {
	if err := validateText("--why", note); err != nil {
		return err
	}
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		if _, err := tx.ExecContext(context.Background(),
			`UPDATE epics SET status = ?, status_note = ? WHERE slug = ?`,
			target, note, slug); err != nil {
			return nil, err //nolint:wrapcheck // the matrix trigger speaks through the mapper
		}
		detail := strings.ToLower(target)
		if note != "" {
			detail += ": " + note
		}
		return []Event{{Entity: epicPrefix + slug, Event: evEpic, Detail: detail}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "epic:%s -> %s\n", slug, target)
	return nil
}

// epicDissolve has close-grade preconditions (§6.2): no IN-PROGRESS
// story, no open task homed here; PLANNED/READY/BLOCKED stories are
// auto-DISSOLVED, each with its own worklog row, in the same transaction.
func epicDissolve(e *cli.Env, slug, why string) error {
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		if blockers, err := dissolveBlockers(ctx, tx, slug); err != nil {
			return nil, err
		} else if len(blockers) > 0 {
			return nil, refuse("blocked", "epic:%s cannot dissolve:\n  %s",
				slug, strings.Join(blockers, "\n  "))
		}
		if err := dissolveStories(ctx, tx, slug, why); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE epics SET status = 'DISSOLVED', status_note = ? WHERE slug = ?`,
			why, slug); err != nil {
			return nil, err //nolint:wrapcheck // constraint codes ride to the mapper
		}
		return []Event{{Entity: epicPrefix + slug, Event: evEpic, Detail: "dissolve: " + why}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "epic:%s -> DISSOLVED\n", slug)
	return nil
}

func dissolveBlockers(ctx context.Context, tx *sql.Tx, slug string) ([]string, error) {
	var blockers []string
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM stories WHERE epic = ? AND status = 'IN-PROGRESS'`, slug)
	if err != nil {
		return nil, fmt.Errorf("dissolve: %w", err)
	}
	blockers, err = drainStrings(rows, blockers, "story %s is IN-PROGRESS")
	if err != nil {
		return nil, err
	}
	trows, err := tx.QueryContext(ctx, `
		SELECT '#' || id || CASE WHEN parked <> ''
			THEN ' is parked here — unpark it or edit --detach'
			ELSE ' is ' || status || ' and homed here' END
		FROM tasks WHERE epic = ? AND status IN ('OPEN','IN-REVIEW','NEEDS-TRIAGE')`, slug)
	if err != nil {
		return nil, fmt.Errorf("dissolve: %w", err)
	}
	return drainStrings(trows, blockers, "task %s")
}

func dissolveStories(ctx context.Context, tx *sql.Tx, slug, why string) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM stories WHERE epic = ? AND status IN ('PLANNED','READY','BLOCKED') ORDER BY id`, slug)
	if err != nil {
		return fmt.Errorf("dissolve: %w", err)
	}
	ids, err := drainStrings(rows, nil, "%s")
	if err != nil {
		return err
	}
	for _, sid := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE stories SET status = 'DISSOLVED', blocked = '' WHERE epic = ? AND id = ?`,
			slug, sid); err != nil {
			return err //nolint:wrapcheck // constraint codes ride to the mapper
		}
		if err := appendWorklog(ctx, tx, slug, sid, "DISSOLVED", "", "", "",
			"auto-dissolved with the epic: "+why); err != nil {
			return err
		}
	}
	return nil
}

func drainStrings(rows *sql.Rows, out []string, format string) ([]string, error) {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, fmt.Sprintf(format, v))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return out, nil
}

func epicShowFull(e *cli.Env, slug string) error {
	return Read(context.Background(), func(ctx context.Context, db *sql.DB) error {
		sections := []struct{ header, q string }{
			{"stories", `SELECT id || ' [' || status || '] ' || title FROM stories WHERE epic = ? ORDER BY id`},
			{"criteria", `SELECT seq || ' [' || CASE met WHEN 1 THEN 'met' ELSE 'open' END || '] ' || criterion
			              FROM epic_criteria WHERE epic = ? ORDER BY seq`},
			{"worklog", `SELECT seq || ' ' || story || ' ' || state || ' ' || commits || ' ' || note
			             FROM worklog WHERE epic = ? ORDER BY seq`},
		}
		if e.JSON {
			// One JSON document (§6.1: --json on every verb means machine-
			// readable output, not a JSON line followed by plain text —
			// found by the S10 dogfood as task #6).
			var goal, status string
			err := db.QueryRowContext(ctx,
				`SELECT goal, status FROM epics WHERE slug = ?`, slug).Scan(&goal, &status)
			if errors.Is(err, sql.ErrNoRows) {
				return refuse("not-found", "no epic %q", slug)
			}
			if err != nil {
				return fmt.Errorf("read epic: %w", err)
			}
			doc := map[string]any{jsonEpicKey: slug, jsonGoalKey: goal, jsonStatusKey: status}
			for _, sec := range sections {
				lines, err := sectionLines(ctx, db, sec.q, slug)
				if err != nil {
					return err
				}
				doc[sec.header] = lines
			}
			return printJSON(e, doc)
		}
		if err := showEpic(ctx, e, db, slug); err != nil {
			return err
		}
		for _, sec := range sections {
			lines, err := sectionLines(ctx, db, sec.q, slug)
			if err != nil {
				return err
			}
			for _, l := range lines {
				_, _ = fmt.Fprintf(e.Stdout, "  %s: %s\n", sec.header, l)
			}
		}
		return nil
	})
}

// sectionLines drains one epic-show section; an empty section marshals
// as [] in JSON, never null.
func sectionLines(ctx context.Context, db *sql.DB, q, slug string) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, slug)
	if err != nil {
		return nil, fmt.Errorf("epic show: %w", err)
	}
	lines, err := drainStrings(rows, nil, "%s")
	if err != nil {
		return nil, err
	}
	if lines == nil {
		lines = []string{}
	}
	return lines, nil
}

func epicList(e *cli.Env, activeOnly bool) error {
	return Read(context.Background(), func(ctx context.Context, db *sql.DB) error {
		q := `SELECT 'epic:' || slug || ' [' || status || '] ' || goal FROM epics`
		if activeOnly {
			q += ` WHERE status = 'ACTIVE'`
		}
		q += ` ORDER BY slug`
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			return fmt.Errorf("epic list: %w", err)
		}
		lines, err := drainStrings(rows, nil, "%s")
		if err != nil {
			return err
		}
		for _, l := range lines {
			_, _ = fmt.Fprintln(e.Stdout, l)
		}
		return nil
	})
}

// closableStatus: close works from ACTIVE or PAUSED; BACKLOG never ran
// and dissolves instead (§6.2).
func closableStatus(ctx context.Context, tx *sql.Tx, slug string) error {
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status FROM epics WHERE slug = ?`, slug).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse("not-found", "no epic %q", slug)
	}
	if err != nil {
		return fmt.Errorf("close: %w", err)
	}
	if status == epicBacklog {
		return refuse("blocked", "epic:%s is BACKLOG — it never ran; dissolve it instead", slug)
	}
	if status != epicActive && status != epicPaused {
		return refuse("blocked", "epic:%s is %s; close works from ACTIVE or PAUSED", slug, status)
	}
	return nil
}

// epicClose is the atomic retro (§6.4): the complete blocker list, then
// one transaction — criteria check, status sweep, dated stamp, events
// row — with active_verb written and deleted INSIDE it, so a crash rolls
// the flag away with everything else.
func epicClose(e *cli.Env, slug string) error {
	var checkOutput []string
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		if err := closableStatus(ctx, tx, slug); err != nil {
			return nil, err
		}
		blockers, output, err := closeBlockers(ctx, tx, slug)
		if err != nil {
			return nil, err
		}
		checkOutput = output
		if len(blockers) > 0 {
			// The COMPLETE blocker list, not the first hit (§6.4).
			return nil, refuse("blocked", "epic:%s cannot close:\n  %s",
				slug, strings.Join(blockers, "\n  "))
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO meta (key, value) VALUES ('active_verb', 'epic-close')`); err != nil {
			return nil, fmt.Errorf("close: %w", err)
		}
		today := time.Now().UTC().Format("2006-01-02")
		if _, err := tx.ExecContext(ctx,
			`UPDATE epics SET status = 'CLOSED', close_sweep = ? WHERE slug = ?`,
			today, slug); err != nil {
			return nil, err //nolint:wrapcheck // the close-gate trigger speaks through the mapper
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM meta WHERE key = 'active_verb'`); err != nil {
			return nil, fmt.Errorf("close: %w", err)
		}
		return []Event{{Entity: epicPrefix + slug, Event: evEpic, Detail: "close: " + today}}, nil
	})
	if err != nil {
		return err
	}
	for _, l := range checkOutput {
		_, _ = fmt.Fprintln(e.Stdout, l)
	}
	_, _ = fmt.Fprintf(e.Stdout, "epic:%s -> CLOSED\n", slug)
	return nil
}
