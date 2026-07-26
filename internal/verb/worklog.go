package verb

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/rules"
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
	if err := validateWorklogText(slug, story, state, commits, gate, review, note); err != nil {
		return err
	}
	isV := strings.HasPrefix(story, "V-")
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

// validateWorklogText screens every string this verb stores, the positional
// SLUG included: it is an argument like any other, it reaches `worklog.epic`
// and the refusal messages, and leaving it out was the one hole in §8.1's
// write-time gate on this verb.
func validateWorklogText(slug, story, state, commits, gate, review, note string) error {
	for _, f := range [][2]string{
		{"SLUG", slug},
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

// validateForm checks whichever sanctioned form this add is. The
// non-correction, non-V refusal lives HERE rather than ahead of the write
// (where it sat until the `worklog-refusals-name-the-routes` amendment)
// because its second clause is computed from the epic's story state, which
// needs a handle. Refusing inside the transaction costs nothing: no row
// has been written and the rollback is the caller's normal error path.
func validateForm(ctx context.Context, tx *sql.Tx, slug, story, state string, corrects int64, isV bool) (any, error) {
	if isV {
		if corrects != 0 {
			return nil, refuse("usage", "a V-row is not a correction; drop --corrects")
		}
		return nil, requireClosedEpic(ctx, tx, slug)
	}
	if corrects == 0 {
		// The two sanctioned forms are the WHOLE surface (§5.7): the
		// self-seeding episode rows belong to the story verbs.
		routes, err := routeClause(ctx, tx, slug)
		if err != nil {
			return nil, err
		}
		return nil, refuse("usage",
			"worklog add writes only V-N rows (CLOSED epics) or corrections (--corrects N);"+
				" episode rows belong to the story verbs%s", routes)
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
		// The epic is known to exist on this line, so the state clause is
		// composed without re-asking whether it does.
		routes, err := existingEpicRouteClause(ctx, tx, slug)
		if err != nil {
			return err
		}
		return refuse("not-closed",
			"V-rows are post-close validation; epic %q is %s%s", slug, status, routes)
	}
	return nil
}

// routeClause is the second clause both `worklog add` refusals carry
// (§6.2, amendment `worklog-refusals-name-the-routes`): the routes that
// apply in the epic's CURRENT story state, leading `; ` included.
//
// An epic that does not exist yields the empty clause, so a nonsense slug
// meets the pre-amendment refusal verbatim. That is deliberate: naming
// `story add` for an epic there is nothing to add a story to would be
// advice the agent cannot take, and turning this into a `not-found` would
// change which refusal the form error produces.
func routeClause(ctx context.Context, tx *sql.Tx, slug string) (string, error) {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM epics WHERE slug = ?`, slug).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("worklog add: %w", err)
	}
	return existingEpicRouteClause(ctx, tx, slug)
}

// existingEpicRouteClause composes the clause for an epic already known to
// exist: the transition that reaches the story best placed to receive the
// episode, or — in the dead zone — both sanctioned routes and where the
// authority for each of them sits.
func existingEpicRouteClause(ctx context.Context, q rules.Querier, slug string) (string, error) {
	offer, ok, err := rules.StoryToOffer(ctx, q, slug)
	if err != nil {
		return "", fmt.Errorf("worklog add: %w", err)
	}
	if !ok {
		return deadZoneClause(slug), nil
	}
	return storyRouteClause(slug, offer), nil
}

// deadZoneClause names both sanctioned routes out of the window in which
// an epic's stories are all terminal, and marks which authority each needs
// (§6.2). It is the only branch of the clause that involves the owner.
func deadZoneClause(slug string) string {
	return fmt.Sprintf("; epic %q has no story in a non-terminal status"+
		" — a new story (story add %q --title T, then story ready and story start)"+
		" is a scope change and the owner's call;"+
		" work that does not advance this epic's goal is a standalone task"+
		" (create --title T)", slug, slug)
}

// storyRouteClause names one existing story and the transition that turns
// it into the home for this episode. No owner decision is involved in any
// of these four: the home already exists, and every verb named here is one
// the implementing agent may run itself.
//
// The `then story start` suffix is safe in exactly the three branches that
// carry it. StoryToOffer sorts IN-PROGRESS first, so reaching PLANNED,
// READY or BLOCKED proves no story of this epic holds the WIP slot, and
// the `story start` this clause promises cannot be refused by the WIP
// index. Offering by lowest id instead would break that: a PLANNED S1
// beside an IN-PROGRESS S2 would send the agent through a `story ready`
// that mutates state and into a `wip` refusal.
//
// Every slug and story id is interpolated with %q, including inside the
// suggested commands. Neither is shape-constrained enough to trust at this
// layer: `epics.slug` has no CHECK at all and `stories.id`'s CHECK is `GLOB
// 'S[0-9]*'` (anything after the first digit), so the writing verbs are the
// only thing between a hostile identifier and this message. `epic create`
// and `import` (task #63) both apply §4's grammar now, but `load` replays a
// dump straight into the tables under the schema's CHECKs alone — so an id
// or slug can still carry bidi and format characters into a message another
// agent reads. %q renders those as escapes and still yields a
// copy-pasteable CLI token.
func storyRouteClause(slug string, s rules.Story) string {
	head := fmt.Sprintf("; story %q of epic %q is %s", s.ID, slug, s.Status)
	switch s.Status {
	case rules.StoryPlanned:
		return head + fmt.Sprintf(" — story ready %q %q, then story start %q %q",
			slug, s.ID, slug, s.ID)
	case rules.StoryReady:
		return head + fmt.Sprintf(" — story start %q %q", slug, s.ID)
	case rules.StoryBlocked:
		return head + fmt.Sprintf(" — story unblock %q %q --resolution TEXT, then story start %q %q",
			slug, s.ID, slug, s.ID)
	case rules.StoryInProgress:
		return head + fmt.Sprintf(" and already holds the WIP slot"+
			" — story done %q %q --commits RANGE --gate G writes the episode", slug, s.ID)
	default:
		// Unreachable: StoryToOffer returns only the four above. A status
		// added to the schema without a route lands here, and naming the
		// story without a route beats inventing one.
		return head
	}
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
