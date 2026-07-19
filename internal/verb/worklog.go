package verb

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
)

// worklogVerb is manual `worklog add`, restricted to exactly two forms
// (§6.2): V-N rows on CLOSED epics, and --corrects N correction rows.
// Everything else is written by the story verbs. The story slot rides a
// --story FLAG, deliberately breaking the SLUG SID positional pattern:
// its domain admits V-N forms and correction context (§6.2).
func worklogVerb() cli.Verb {
	var story, state, commits, gate, review, note string
	var corrects int64
	return cli.Verb{Name: "worklog", Subs: []cli.Sub{{
		Name: subAdd, Arity: 1,
		Usage: "worklog add SLUG --story SID|V-N --state ST [--corrects N] [--commits] [--gate] [--review] [--note] [--json]",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&story, "story", "", "story id, or V-<n> on a CLOSED epic")
			fs.StringVar(&state, "state", "", "the episode state")
			fs.Int64Var(&corrects, "corrects", 0, "the ORIGINAL row seq this corrects")
			fs.StringVar(&commits, "commits", "", "commit range")
			fs.StringVar(&gate, "gate", "", "gate")
			fs.StringVar(&review, "review", "", "review")
			fs.StringVar(&note, flagNote, "", "episode note (a corrected fact's true date goes here as content)")
		},
		Run: func(e *cli.Env, pos []string, _ *flag.FlagSet) error {
			return worklogAdd(e, pos[0], story, state, commits, gate, review, note, corrects)
		},
	}}}
}

func worklogAdd(e *cli.Env, slug, story, state, commits, gate, review, note string, corrects int64) error {
	if story == "" || state == "" {
		return refuse("usage", "worklog add requires --story and --state")
	}
	if err := validateWorklogText(story, state, commits, gate, review, note); err != nil {
		return err
	}
	isV := strings.HasPrefix(story, "V-")
	if !isV && corrects == 0 {
		// The two sanctioned forms are the WHOLE surface (§5.7): the
		// self-seeding episode rows belong to the story verbs.
		return refuse("usage",
			"worklog add writes only V-N rows (CLOSED epics) or corrections (--corrects N);"+
				" episode rows belong to the story verbs")
	}
	err := Write(context.Background(), func(tx *sql.Tx) ([]Event, error) {
		ctx := context.Background()
		correctsVal, err := validateForm(ctx, tx, slug, story, state, corrects, isV)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO worklog (epic, seq, story, date, state, commits, gate, review, corrects, note)
			VALUES (?1, COALESCE((SELECT MAX(seq) FROM worklog WHERE epic = ?1), 0) + 1,
			        ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)`,
			slug, story, now(), state, commits, gate, review, correctsVal, note); err != nil {
			return nil, err //nolint:wrapcheck // CHECKs and the contiguity trigger speak through the mapper
		}
		entity := epicPrefix + slug
		if !isV {
			entity = storyEntity(slug, story)
		}
		return []Event{{Entity: entity, Event: evWorklog, Detail: describeAdd(story, state, corrects)}}, nil
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "worklog row appended to %s\n", slug)
	return nil
}

func validateWorklogText(story, state, commits, gate, review, note string) error {
	for _, f := range [][2]string{
		{"--story", story},
		{"--state", state},
		{"--commits", commits},
		{"--gate", gate},
		{"--review", review},
		{flagNoteArg, note},
	} {
		if err := validateText(f[0], f[1]); err != nil {
			return err
		}
	}
	return nil
}

// validateForm checks whichever sanctioned form this add is.
func validateForm(ctx context.Context, tx *sql.Tx, slug, story, state string, corrects int64, isV bool) (any, error) {
	if isV {
		if corrects != 0 {
			return nil, refuse("usage", "a V-row is not a correction; drop --corrects")
		}
		return nil, requireClosedEpic(ctx, tx, slug)
	}
	if err := validateCorrection(ctx, tx, slug, story, state, corrects); err != nil {
		return nil, err
	}
	return corrects, nil
}

func requireClosedEpic(ctx context.Context, tx *sql.Tx, slug string) error {
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status FROM epics WHERE slug = ?`, slug).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse("not-found", "no epic %q", slug)
	}
	if err != nil {
		return fmt.Errorf("worklog add: %w", err)
	}
	if status != epicClosed {
		return refuse("not-closed", "V-rows are post-close validation; epic %q is %s", slug, status)
	}
	return nil
}

// validateCorrection enforces §5.7's correction shape at the verb: the
// target must exist, be an ORIGINAL (non-correction) row of the SAME
// story, and the correction's state must equal the corrected row's state
// — chains cannot form, and "what a correction means" never recurses.
func validateCorrection(ctx context.Context, tx *sql.Tx, slug, story, state string, corrects int64) error {
	var targetStory, targetState string
	var targetCorrects sql.NullInt64
	err := tx.QueryRowContext(ctx,
		`SELECT story, state, corrects FROM worklog WHERE epic = ? AND seq = ?`,
		slug, corrects).Scan(&targetStory, &targetState, &targetCorrects)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse("not-found", "no worklog row %d on epic %q to correct", corrects, slug)
	}
	if err != nil {
		return fmt.Errorf("correction: %w", err)
	}
	if targetCorrects.Valid {
		return refuse("chain",
			"row %d is itself a correction; correcting a correction is a second correction of the SAME original", corrects)
	}
	if targetStory != story {
		return refuse("mismatch", "row %d belongs to story %s, not %s", corrects, targetStory, story)
	}
	if targetState != state {
		return refuse("mismatch",
			"a correction mirrors the corrected row's state: row %d is %s, not %s", corrects, targetState, state)
	}
	return nil
}

func describeAdd(story, state string, corrects int64) string {
	if corrects != 0 {
		return fmt.Sprintf("correction of seq %d (%s %s)", corrects, story, state)
	}
	return "V-row " + story + " " + state
}
