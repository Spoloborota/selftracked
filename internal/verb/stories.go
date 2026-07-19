package verb

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"

	"github.com/Spoloborota/selftracked/internal/cli"
)

const (
	// storyArity: SLUG SID.
	storyArity = 2

	storyPlanned    = "PLANNED"
	storyReady      = "READY"
	storyBlocked    = "BLOCKED"
	storyInProgress = "IN-PROGRESS"
	storyDone       = "DONE"

	evStory   = "story"
	evWorklog = "worklog"
)

// StoryVerbs returns the S6 `story` and `worklog` catalog entries.
func StoryVerbs() []cli.Verb {
	var title, reason, resolution, commits, gate, review, why string
	return []cli.Verb{{Name: "story", Subs: []cli.Sub{
		{
			Name: subAdd, Arity: 1, Usage: "story add SLUG --title T [--json]",
			Flags: func(fs *flag.FlagSet) { fs.StringVar(&title, "title", "", "the story title (required)") },
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyAdd(e, pos[0], title)
			},
		},
		{
			Name: subReady, Arity: storyArity, Usage: "story ready SLUG SID [--json]",
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyReadyVerb(e, pos[0], pos[1])
			},
		},
		{
			Name: "start", Arity: storyArity, Usage: "story start SLUG SID [--json]",
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyStart(e, pos[0], pos[1])
			},
		},
		{
			Name: "block", Arity: storyArity, Usage: "story block SLUG SID --reason TEXT [--json]",
			Flags: func(fs *flag.FlagSet) { fs.StringVar(&reason, "reason", "", "what blocks it (required)") },
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyBlock(e, pos[0], pos[1], reason)
			},
		},
		{
			Name: "unblock", Arity: storyArity, Usage: "story unblock SLUG SID --resolution TEXT [--json]",
			Flags: func(fs *flag.FlagSet) {
				fs.StringVar(&resolution, "resolution", "", "what cleared the block (required)")
			},
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyUnblock(e, pos[0], pos[1], resolution)
			},
		},
		{
			Name: "done", Arity: storyArity,
			Usage: "story done SLUG SID --commits RANGE --gate G [--review R] [--json]",
			Flags: func(fs *flag.FlagSet) {
				fs.StringVar(&commits, "commits", "", "the proving commit range (required)")
				fs.StringVar(&gate, "gate", "", "the gate that went green (required)")
				fs.StringVar(&review, "review", "", "review reference")
			},
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyDoneVerb(e, pos[0], pos[1], commits, gate, review)
			},
		},
		{
			Name: "dissolve", Arity: storyArity, Usage: "story dissolve SLUG SID --why TEXT [--json]",
			Flags: func(fs *flag.FlagSet) { fs.StringVar(&why, "why", "", "the dissolve reason (required)") },
			Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
				return storyDissolve(e, pos[0], pos[1], why)
			},
		},
	}}, worklogVerb()}
}

// appendWorklog writes one episode row at the epic's next contiguous seq.
func appendWorklog(ctx context.Context, tx *sql.Tx,
	slug, story, state, commits, gate, review, note string,
) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, note)
		VALUES (?1, COALESCE((SELECT MAX(seq) FROM worklog WHERE epic = ?1), 0) + 1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)`,
		slug, story, now(), state, commits, gate, review, note); err != nil {
		return err //nolint:wrapcheck // the contiguity trigger and CHECKs speak through the mapper
	}
	return nil
}

func storyEntity(slug, sid string) string { return epicPrefix + slug + "/" + sid }

func storyAdd(e *cli.Env, slug, title string) error {
	if title == "" {
		return refuse("usage", "story add requires --title")
	}
	if err := validateText("--title", title); err != nil {
		return err
	}
	var sid string
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		var next int64
		err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(CAST(substr(id, 2) AS INTEGER)), 0) + 1
			FROM stories WHERE epic = ?`, slug).Scan(&next)
		if err != nil {
			return nil, fmt.Errorf("story add: %w", err)
		}
		sid = fmt.Sprintf("S%d", next)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO stories (epic, id, title) VALUES (?, ?, ?)`, slug, sid, title); err != nil {
			return nil, err //nolint:wrapcheck // FK and CHECKs speak through the mapper
		}
		return []Event{{Entity: storyEntity(slug, sid), Event: evStory, Detail: "add: " + title}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s [PLANNED]\n", slug, sid)
	return nil
}

// storyTransition updates status (+blocked) — the §5.7 matrix trigger is
// the arbiter — and optionally appends the episode row.
func storyTransition(slug, sid, target, blocked string, episode *Event, worklogState, note string) error {
	return Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		res, err := tx.ExecContext(ctx,
			`UPDATE stories SET status = ?, blocked = ? WHERE epic = ? AND id = ?`,
			target, blocked, slug, sid)
		if err != nil {
			return nil, err //nolint:wrapcheck // the matrix trigger speaks through the mapper
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, refuse("not-found", "no story %s in epic %q", sid, slug)
		}
		if worklogState != "" {
			if err := appendWorklog(ctx, tx, slug, sid, worklogState, "", "", "", note); err != nil {
				return nil, err
			}
		}
		if episode != nil {
			return []Event{*episode}, nil
		}
		return nil, nil
	})
}

// storyReadyVerb: PLANNED → READY. The DoD-is-a-command rule (§2) gets
// its tooth here: a story with an empty dod field is not Ready — there
// is nothing executable to be ready FOR.
func storyReadyVerb(e *cli.Env, slug, sid string) error {
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		var dod string
		err := tx.QueryRowContext(ctx,
			`SELECT dod FROM stories WHERE epic = ? AND id = ?`, slug, sid).Scan(&dod)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, refuse("not-found", "no story %s in epic %q", sid, slug)
		}
		if err != nil {
			return nil, fmt.Errorf("story ready: %w", err)
		}
		if dod == "" {
			return nil, refuse("no-dod",
				"story %s has no DoD; set one with edit --dod (a command or invariant, never prose)", sid)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE stories SET status = 'READY' WHERE epic = ? AND id = ?`, slug, sid); err != nil {
			return nil, err //nolint:wrapcheck // the matrix trigger speaks through the mapper
		}
		// No worklog row: no worklog state exists for this transition (§6.2).
		return []Event{{Entity: storyEntity(slug, sid), Event: evStory, Detail: "ready"}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> READY\n", slug, sid)
	return nil
}

// storyStart: READY + DoR (empty blocked, schema-checked) + WIP slot
// free (the partial unique index is the arbiter; the verb pre-checks for
// a named message).
func storyStart(e *cli.Env, slug, sid string) error {
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		var wip string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM stories WHERE epic = ? AND status = 'IN-PROGRESS'`, slug).Scan(&wip)
		if err == nil {
			return nil, refuse("wip", "WIP limit: story %s is IN-PROGRESS in epic %q — finish or block it first", wip, slug)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("story start: %w", err)
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE stories SET status = 'IN-PROGRESS' WHERE epic = ? AND id = ?`, slug, sid)
		if err != nil {
			return nil, err //nolint:wrapcheck // matrix/DoR CHECKs speak through the mapper
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, refuse("not-found", "no story %s in epic %q", sid, slug)
		}
		if err := appendWorklog(ctx, tx, slug, sid, storyInProgress, "", "", "", ""); err != nil {
			return nil, err
		}
		return []Event{{Entity: storyEntity(slug, sid), Event: evStory, Detail: "start"}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> IN-PROGRESS\n", slug, sid)
	return nil
}

func storyBlock(e *cli.Env, slug, sid, reason string) error {
	if reason == "" {
		return refuse("usage", "story block requires --reason")
	}
	if err := validateText("--reason", reason); err != nil {
		return err
	}
	ev := Event{Entity: storyEntity(slug, sid), Event: evStory, Detail: "block: " + reason}
	if err := storyTransition(slug, sid, storyBlocked, reason, &ev, "BLOCKED-ON-OWNER", reason); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> BLOCKED\n", slug, sid)
	return nil
}

// storyUnblock: BLOCKED → READY. No worklog row — the durable record is
// the events row carrying the resolution VERBATIM (§6.2): when the block
// was an owner question, the resolution starts with the literal `PO:`
// prefix and quotes the verdict; presence-of-text is verb-enforced only.
func storyUnblock(e *cli.Env, slug, sid, resolution string) error {
	if resolution == "" {
		return refuse("usage", "story unblock requires --resolution (what cleared the block)")
	}
	if err := validateText("--resolution", resolution); err != nil {
		return err
	}
	ev := Event{Entity: storyEntity(slug, sid), Event: evStory, Detail: resolution}
	if err := storyTransition(slug, sid, storyReady, "", &ev, "", ""); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> READY\n", slug, sid)
	return nil
}

// storyDone: the status flip and the DONE worklog row are one
// transaction — no non-atomic path exists (§6.2).
func storyDoneVerb(e *cli.Env, slug, sid, commits, gate, review string) error {
	if commits == "" || gate == "" {
		return refuse("usage", "story done requires --commits and --gate")
	}
	for _, f := range [][2]string{{"--commits", commits}, {"--gate", gate}, {"--review", review}} {
		if err := validateText(f[0], f[1]); err != nil {
			return err
		}
	}
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		res, err := tx.ExecContext(ctx,
			`UPDATE stories SET status = 'DONE' WHERE epic = ? AND id = ?`, slug, sid)
		if err != nil {
			return nil, err //nolint:wrapcheck // the matrix trigger speaks through the mapper
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, refuse("not-found", "no story %s in epic %q", sid, slug)
		}
		if err := appendWorklog(ctx, tx, slug, sid, storyDone, commits, gate, review, ""); err != nil {
			return nil, err
		}
		return []Event{{
			Entity: storyEntity(slug, sid), Event: evStory,
			Detail: "done: " + commits + "; gate: " + gate,
		}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> DONE\n", slug, sid)
	return nil
}

func storyDissolve(e *cli.Env, slug, sid, why string) error {
	if why == "" {
		return refuse("usage", "story dissolve requires --why")
	}
	if err := validateText("--why", why); err != nil {
		return err
	}
	ev := Event{Entity: storyEntity(slug, sid), Event: evStory, Detail: "dissolve: " + why}
	if err := storyTransition(slug, sid, "DISSOLVED", "", &ev, "DISSOLVED", why); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "%s/%s -> DISSOLVED\n", slug, sid)
	return nil
}
